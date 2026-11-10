package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/runtime"
)

// cmdSession exposes local session and bootstrap token utilities for the CLI.
func cmdSession(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	if len(args) == 0 {
		out.text("usage: uawp session token [--origin <url>] | status")
		return int(runtime.ExitConfigInvalid)
	}

	switch args[0] {
	case "token", "bootstrap":
		return cmdSessionToken(ctx, flags, out, args[1:])
	case "status":
		return cmdSessionStatus(ctx, flags, out)
	default:
		out.text("usage: uawp session token [--origin <url>] | status")
		return int(runtime.ExitConfigInvalid)
	}
}

func cmdSessionToken(ctx context.Context, flags cliFlags, out *printer, args []string) int {
	origin := "http://localhost:5173"
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--origin=") {
			origin = strings.TrimPrefix(args[i], "--origin=")
		} else if args[i] == "--origin" && i+1 < len(args) {
			origin = args[i+1]
			i++
		}
	}

	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}

	rec, token, err := platform.OwnerCredentials(ds.Runtime)
	if err != nil {
		if flags.json {
			out.report(map[string]any{"error": "CORE_NOT_RUNNING", "detail": "The Core is not running. Start the Core first with: uawp start"})
		} else {
			out.text("The Core is not running. Start the Core first with: uawp start")
		}
		return int(runtime.ExitFailure)
	}

	resp, err := platform.CallIPCWithOrigin(ds.Runtime, platform.IPCOpBootstrapToken, rec.InstanceUUID, token, origin)
	if err != nil {
		if flags.json {
			out.report(map[string]any{"error": "IPC_UNREACHABLE", "detail": err.Error()})
		} else {
			out.text("Could not contact the Core over IPC: " + err.Error())
		}
		return int(runtime.ExitAuthority)
	}

	if !resp.OK {
		env, _ := errs.New(errs.CodeAuthzDenied, errs.NewCorrelationID(), map[string]any{"resource": "ipc", "code": resp.Code})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
		return int(runtime.ExitAuthority)
	}

	var res map[string]string
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		out.text("Invalid IPC response format")
		return int(runtime.ExitInternal)
	}

	if flags.json {
		out.report(res)
	} else {
		out.text(res["credential"])
	}
	return int(runtime.ExitOK)
}

func cmdSessionStatus(ctx context.Context, flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	rec, _, err := platform.OwnerCredentials(ds.Runtime)
	running := err == nil && rec.InstanceUUID != ""
	report := map[string]any{
		"running": running,
	}
	if running {
		report["instance_uuid"] = rec.InstanceUUID
		report["pid"] = rec.PID
	}
	if flags.json {
		out.report(report)
	} else {
		if running {
			out.text("Core session service is active (PID: " + rec.InstanceUUID + ")")
		} else {
			out.text("Core is not running")
		}
	}
	return int(runtime.ExitOK)
}
