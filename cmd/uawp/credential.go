package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/security/keyring"
)

// The credential surface (D-37 accepted: the OS keyring is the ONLY store).
// `credential store` reads the secret value FROM STDIN — never argv, never
// echoed — and stores it under the exact handle the executors resolve
// ("provider:<id>/<name>"). `status` reports presence only (a reason code,
// never a value); `remove` revokes (idempotent on a missing item). No
// command prints or logs a secret value, and a stored credential alone
// opens nothing: a live call still needs a passing C3-19 gate.

func cmdCredential(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	if len(args) == 0 {
		out.text("usage: uawp credential store|status|remove --provider <id> [--name api-key]")
		return int(runtime.ExitConfigInvalid)
	}
	switch args[0] {
	case "store":
		return cmdCredentialStore(ctx, flags, out, args[1:])
	case "status":
		return cmdCredentialStatus(ctx, flags, out, args[1:])
	case "remove":
		return cmdCredentialRemove(ctx, flags, out, args[1:])
	default:
		out.text("usage: uawp credential store|status|remove --provider <id> [--name api-key]")
		return int(runtime.ExitConfigInvalid)
	}
}

// knownCredentialProviders lists the provider ids the credential commands
// accept (the recorded direct families + custom/local); a typo must never
// plant a credential under a handle nothing will ever resolve.
var knownCredentialProviders = map[string]bool{
	dprovider.ProviderOpenAI:          true,
	dprovider.ProviderAnthropic:       true,
	dprovider.ProviderGemini:          true,
	dprovider.ProviderQwen:            true,
	dprovider.ProviderGLM:             true,
	dprovider.ProviderMoonshot:        true,
	dprovider.ProviderMiniMax:         true,
	dprovider.ProviderDeepSeek:        true,
	dprovider.ProviderCustomOpenAI:    true,
	dprovider.ProviderCustomAnthropic: true,
	dprovider.ProviderLocal:           true,
	dprovider.ProviderMock:            true,
}

type credentialFlags struct {
	provider string
	name     string
}

func parseCredentialArgs(args []string) (credentialFlags, bool) {
	var f credentialFlags
	f.name = "api-key" // the handle name the direct adapters resolve by default
	for i := 0; i < len(args); i++ {
		val := func() (string, bool) {
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case strings.HasPrefix(args[i], "--provider="):
			f.provider = strings.TrimPrefix(args[i], "--provider=")
		case args[i] == "--provider":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.provider = v
		case strings.HasPrefix(args[i], "--name="):
			f.name = strings.TrimPrefix(args[i], "--name=")
		case args[i] == "--name":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.name = v
		}
	}
	if strings.TrimSpace(f.provider) == "" || strings.TrimSpace(f.name) == "" {
		return f, false
	}
	if !knownCredentialProviders[f.provider] {
		return f, false // unknown provider: refuse rather than plant an unreachable handle
	}
	return f, true
}

// credentialHandle forms the SAME handle shape the executor's secret
// boundary resolves ("provider:<id>/<name>" scope/name pair).
func credentialHandle(f credentialFlags) security.Handle {
	return security.Handle{Scope: "provider:" + f.provider, Name: f.name}
}

// credentialStdin is the source `credential store` reads the value from; a
// var so tests can inject stdin without process plumbing. The value never
// passes through argv.
var credentialStdin io.Reader = os.Stdin

func cmdCredentialStore(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	f, ok := parseCredentialArgs(args)
	if !ok {
		out.text("usage: uawp credential store --provider <id> [--name api-key]  (value on stdin)")
		return int(runtime.ExitConfigInvalid)
	}
	raw, err := io.ReadAll(credentialStdin)
	if err != nil {
		env, _ := errs.New(errs.CodeInternal, errs.NewCorrelationID(), map[string]any{"reason": "stdin read failed"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitInternal})
		return int(runtime.ExitInternal)
	}
	value := bytes.TrimSuffix(bytes.TrimSuffix(raw, []byte("\n")), []byte("\r"))
	if len(bytes.TrimSpace(value)) == 0 {
		env, _ := errs.New(errs.CodeValidationFailed, errs.NewCorrelationID(), map[string]any{
			"reason": "empty credential: pass the value on stdin, never as an argument",
		})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitConfigInvalid})
		return int(runtime.ExitConfigInvalid)
	}
	if err := keyring.New().Store(ctx, credentialHandle(f), value); err != nil {
		out.report(map[string]any{"error": boundedDetail(err)})
		return int(runtime.ExitStorage)
	}
	out.report(map[string]any{"provider": f.provider, "name": f.name, "stored": true})
	return int(runtime.ExitOK)
}

func cmdCredentialStatus(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	f, ok := parseCredentialArgs(args)
	if !ok {
		out.text("usage: uawp credential status --provider <id> [--name api-key]")
		return int(runtime.ExitConfigInvalid)
	}
	_, err := keyring.New().Resolve(ctx, credentialHandle(f))
	var se *security.SecretError
	switch {
	case err == nil:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "present": true})
		return int(runtime.ExitOK)
	case errors.As(err, &se) && se.Reason == security.ReasonSecretMissing:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "present": false, "reason": security.ReasonSecretMissing})
		return int(runtime.ExitOK)
	default:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "present": false, "error": boundedDetail(err)})
		return int(runtime.ExitStorage)
	}
}

func cmdCredentialRemove(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	f, ok := parseCredentialArgs(args)
	if !ok {
		out.text("usage: uawp credential remove --provider <id> [--name api-key]")
		return int(runtime.ExitConfigInvalid)
	}
	err := keyring.New().Revoke(ctx, credentialHandle(f))
	var se *security.SecretError
	switch {
	case err == nil:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "removed": true})
		return int(runtime.ExitOK)
	case errors.As(err, &se) && se.Reason == security.ReasonSecretMissing:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "removed": true, "reason": security.ReasonSecretMissing})
		return int(runtime.ExitOK)
	default:
		out.report(map[string]any{"provider": f.provider, "name": f.name, "error": boundedDetail(err)})
		return int(runtime.ExitStorage)
	}
}
