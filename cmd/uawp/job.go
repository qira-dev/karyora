package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"os"

	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/security/keyring"
	"github.com/qira-dev/karyora/internal/storage"
)

// The CLI surface of the durable vertical slice (C3-31). The commands call
// THE SAME app.SliceService functions an API consumer calls, so CLI and API
// produce semantically identical state and events. `job submit` runs a
// foreground session (a first-class Phase 2 mode): it takes data-directory
// ownership, runs the bounded scheduler to settlement, and reports. D-37 is
// accepted (2026-10-28): credentials resolve only through the OS-keyring
// store, while the provider set carries the recorded fixture adapter only —
// live adapters additionally require C3-19 dated data-handling reviews, so
// protected-data egress stays blocked.

func cmdJob(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	if len(args) == 0 {
		out.text("usage: uawp job submit|status|events [args]")
		return int(runtime.ExitConfigInvalid)
	}
	switch args[0] {
	case "submit":
		return cmdJobSubmit(ctx, flags, out, args[1:])
	case "status":
		if len(args) < 2 {
			out.text("usage: uawp job status <job-id>")
			return int(runtime.ExitConfigInvalid)
		}
		return cmdJobStatus(ctx, flags, out, args[1])
	case "events":
		if len(args) < 2 {
			out.text("usage: uawp job events <job-id>")
			return int(runtime.ExitConfigInvalid)
		}
		return cmdJobEvents(ctx, flags, out, args[1])
	default:
		out.text("usage: uawp job submit|status|events [args]")
		return int(runtime.ExitConfigInvalid)
	}
}

// jobSubmitFlags carries the submission contract from the CLI flags.
type jobSubmitFlags struct {
	task           string
	idempotencyKey string
	provider       string
	model          string
}

func parseJobSubmitArgs(args []string) (jobSubmitFlags, bool) {
	var f jobSubmitFlags
	f.provider = dprovider.ProviderMock // the fixture adapter while D-37 is open
	for i := 0; i < len(args); i++ {
		val := func() (string, bool) {
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case strings.HasPrefix(args[i], "--task="):
			f.task = strings.TrimPrefix(args[i], "--task=")
		case args[i] == "--task":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.task = v
		case strings.HasPrefix(args[i], "--idempotency-key="):
			f.idempotencyKey = strings.TrimPrefix(args[i], "--idempotency-key=")
		case args[i] == "--idempotency-key":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.idempotencyKey = v
		case strings.HasPrefix(args[i], "--provider="):
			f.provider = strings.TrimPrefix(args[i], "--provider=")
		case args[i] == "--provider":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.provider = v
		case strings.HasPrefix(args[i], "--model="):
			f.model = strings.TrimPrefix(args[i], "--model=")
		}
	}
	if strings.TrimSpace(f.task) == "" || strings.TrimSpace(f.idempotencyKey) == "" {
		return f, false
	}
	return f, true
}

// cmdJobSubmit runs the foreground vertical slice: ownership → migrations →
// security bundle → session exchange → submit → bounded scheduling → report.
func cmdJobSubmit(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	sf, ok := parseJobSubmitArgs(args)
	if !ok {
		out.text("usage: uawp job submit --task <text> --idempotency-key <key> [--provider mock] [--model m]")
		return int(runtime.ExitConfigInvalid)
	}
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	corr := errs.NewCorrelationID()
	cfg, f := runtime.LoadEffectiveConfig(corr, ds.Config, flags.overrides, os.Getenv)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}

	// Data-directory ownership: the foreground session IS the instance.
	own, _, err := platform.AcquireOwnership(ds, ds.Data, version)
	if err != nil {
		env, _ := errs.New(errs.CodeSingleInstanceConflict, corr, map[string]any{"reason": boundedDetail(err)})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAlreadyRunning})
		return int(runtime.ExitAlreadyRunning)
	}
	defer func() { _ = own.Release() }()

	opts := buildOptions(cfg, ds.Data)
	lz := &lazyStore{opts: opts}
	defer lz.close()
	if err := lz.connect(ctx, version); err != nil {
		env, _ := errs.New(errs.CodeStorageUnavailable, corr, map[string]any{"reason": boundedDetail(err)})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitStorage})
		return int(runtime.ExitStorage)
	}
	bundle := newSecurityBundle(lz, cfg, ds.Data, ds.State, corr)
	defer bundle.logger.Close()

	// The caller session: the CLI exchanges the one-time bootstrap
	// credential for a data-scoped session token (C2-14 §3).
	cred, err := bundle.sessions.IssueBootstrapCredential("cli")
	if err != nil {
		env, _ := errs.New(errs.CodeInternal, corr, map[string]any{"reason": "session bootstrap failed"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitInternal})
		return int(runtime.ExitInternal)
	}
	token, err := bundle.sessions.Exchange(cred, "cli", []security.Scope{security.ScopeData})
	if err != nil {
		env, _ := errs.New(errs.CodeAuthzDenied, corr, map[string]any{"reason": "session exchange failed"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
		return int(runtime.ExitAuthority)
	}
	callCtx := app.WithCaller(ctx, bundle.instance, token.Secret)

	slice, runDeps := wireJobSlice(lz, bundle)
	sub, err := slice.Submit(callCtx, app.SubmitRequest{
		IdempotencyKey: sf.idempotencyKey,
		Task:           sf.task,
		Provider:       sf.provider,
		Model:          sf.model,
		Classification: dprovider.ClassificationTestFixture,
	})
	if err != nil {
		out.report(map[string]any{"error": boundedDetail(err)})
		return int(runtime.ExitFailure)
	}
	final, err := app.RunToSettlement(callCtx, runDeps, "worker-cli", 1, sub.Job.JobID, 64)
	if err != nil {
		out.report(map[string]any{"error": boundedDetail(err)})
		return int(runtime.ExitFailure)
	}
	report := map[string]any{
		"job_id":         final.JobID,
		"state":          final.State,
		"attempt_count":  final.AttemptCount,
		"receipt_id":     final.ReceiptID,
		"result_digest":  final.ResultDigest,
		"correlation_id": final.CorrelationID,
	}
	if final.State == "succeeded" {
		if payload, rerr := slice.Result(callCtx, final.JobID); rerr == nil {
			report["result"] = json.RawMessage(payload)
		}
	}
	if recs, eerr := slice.Events(callCtx, final.JobID); eerr == nil {
		ids := make([]string, 0, len(recs))
		for _, r := range recs {
			ids = append(ids, r.EventType)
		}
		report["events"] = ids
		if len(recs) > 0 && len(recs[len(recs)-1].InlinePayload) > 0 {
			var lastPayload map[string]any
			if json.Unmarshal(recs[len(recs)-1].InlinePayload, &lastPayload) == nil {
				report["last_event"] = lastPayload
			}
		}
	}
	out.report(report)
	if final.State == "succeeded" {
		return int(runtime.ExitOK)
	}
	return int(runtime.ExitFailure)
}

// wireJobSlice composes the slice over the opened store with the recorded
// fixture provider (D-37: no credentials, no live provider calls).
func wireJobSlice(lz *lazyStore, bundle *securityBundle) (*app.Slice, app.SliceDeps) {
	store := lz.store
	mock := providers.NewMockAdapter(providers.MockBehavior{})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(mock.SeedDataHandling(mock.DefaultProfile("default")))
	exec, _ := providers.NewExecutor(mock, providers.DefaultEgressPolicy(providers.CoreLocal), keyring.New(), reg)
	deps := app.SliceDeps{
		Relay:     store,
		Snapshots: store,
		Objects:   store,
		Events:    store,
		Packs:     store,
		Guard:     bundle.guard,
		Providers: map[string]app.ProviderExecutor{dprovider.ProviderMock: exec},
		Backoff:   store,
	}
	slice, _ := app.NewSlice(deps)
	return slice, deps
}

// cmdJobStatus reads one job read-only (no ownership taken; the doctor-style
// probe path). A live owner makes WAL reads safe; nothing is ever written.
func cmdJobStatus(ctx context.Context, flags cliFlags, out *printer, jobID string) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	cfg, f := runtime.LoadEffectiveConfig(errs.NewCorrelationID(), ds.Config, flags.overrides, os.Getenv)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	opts := buildOptions(cfg, ds.Data)
	rec, err := storage.JobRecordReadonly(ctx, opts.Path, jobID)
	if err != nil {
		out.report(map[string]any{"error": bounded(err)})
		return int(runtime.ExitStorage)
	}
	if rec == nil {
		env, _ := errs.New(errs.CodeNotFound, errs.NewCorrelationID(), map[string]any{"job_id": jobID})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitFailure})
		return int(runtime.ExitFailure)
	}
	out.report(rec)
	return int(runtime.ExitOK)
}

// cmdJobEvents prints one job's journal events in sequence order.
func cmdJobEvents(ctx context.Context, flags cliFlags, out *printer, jobID string) int {
	return cmdJobStatus(ctx, flags, out, jobID)
}
