package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
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
// ownership, runs the bounded scheduler to settlement, and reports. Wiring
// (2026-10-28 milestone): the composition root carries the recorded fixture
// adapter AND the eight direct integrations (C3-21–C3-28), each over the
// OS-keyring credential store (D-37 accepted). Live calls additionally
// require a stored credential and a passing C3-19 gate under the accepted
// egress-classification policy (only Z.AI x `non-personal` passes today);
// unlabeled or `personal` data never egresses to a third-party cloud in v1.

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
	classification string
	baseURL        string
}

func parseJobSubmitArgs(args []string) (jobSubmitFlags, bool) {
	var f jobSubmitFlags
	f.provider = dprovider.ProviderMock // the recorded fixture adapter is the default
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
		case strings.HasPrefix(args[i], "--classification="):
			f.classification = strings.TrimPrefix(args[i], "--classification=")
		case args[i] == "--classification":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.classification = v
		case strings.HasPrefix(args[i], "--base-url="):
			f.baseURL = strings.TrimPrefix(args[i], "--base-url=")
		case args[i] == "--base-url":
			v, ok := val()
			if !ok {
				return f, false
			}
			f.baseURL = v
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
		out.text("usage: uawp job submit --task <text> --idempotency-key <key> [--provider mock] [--model m] [--classification <label>] [--base-url <url>]")
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
		if rec, token, ownerErr := platform.OwnerCredentials(ds.Runtime); ownerErr == nil {
			return submitJobViaRunningDaemon(ctx, flags, out, cfg, ds, sf, corr, rec, token)
		}
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

	slice, runDeps, err := wireJobSlice(lz, bundle)
	if err != nil {
		env, _ := errs.New(errs.CodeInternal, corr, map[string]any{"reason": boundedDetail(err)})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitInternal})
		return int(runtime.ExitInternal)
	}
	// Classification label (accepted egress policy, 2026-10-28): an explicit
	// flag value wins; otherwise the fixture adapter keeps its fixture label
	// and every real provider gets the EMPTY label (protected-unlabeled) —
	// the C3-19 gate then decides, fail-closed.
	classification := sf.classification
	if classification == "" && sf.provider == dprovider.ProviderMock {
		classification = dprovider.ClassificationTestFixture
	}
	sub, err := slice.Submit(callCtx, app.SubmitRequest{
		IdempotencyKey: sf.idempotencyKey,
		Task:           sf.task,
		Provider:       sf.provider,
		Model:          sf.model,
		Classification: classification,
		BaseURL:        sf.baseURL,
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

// wireJobSlice composes the slice over the opened store: the recorded
// fixture adapter PLUS the eight direct integrations (C3-21–C3-28), each as
// its own executor over ONE shared C3-19 registry and the OS-keyring
// credential store (D-37 accepted), PLUS the per-connection custom-executor
// construction (IMPL-18) — the composition-provided factory that builds
// the coded C3-29/C3-30 custom connection-protocol executors from each
// connection record's own non-secret posture over the same policy, store,
// and registry (the app layer cannot construct executors: the dependency
// map's own rule). No credential exists until the user stores one; a
// provider without a stored credential fails typed at resolution time, and
// the registry gate decides classification egress (today only Z.AI x
// non-personal passes — accepted policy, 2026-10-28); a custom endpoint's
// honest unknown record keeps its probe fail-closed exactly like every
// other profile without a current dated review.
func wireJobSlice(lz *lazyStore, bundle *securityBundle) (*app.Slice, app.SliceDeps, error) {
	store := lz.store
	reg := dprovider.NewRegistry(nil)
	adapters := make([]providers.Adapter, 0, 9)
	adapters = append(adapters, providers.NewMockAdapter(providers.MockBehavior{}))
	adapters = append(adapters, providers.NewDirectAdapters()...)
	secrets := keyring.New()
	executors := make(map[string]app.ProviderExecutor, len(adapters))
	for _, a := range adapters {
		reg.Upsert(a.SeedDataHandling(a.DefaultProfile("default")))
		exec, err := providers.NewExecutor(a, providers.DefaultEgressPolicy(providers.CoreLocal), secrets, reg)
		if err != nil {
			return nil, app.SliceDeps{}, err
		}
		executors[a.ID()] = exec
	}
	deps := app.SliceDeps{
		Relay:         store,
		Snapshots:     store,
		Objects:       store,
		Events:        store,
		Packs:         store,
		Guard:         bundle.guard,
		Providers:     executors,
		Backoff:       store,
		Connections:   store,
		Secrets:       secrets,
		Conversations: store,
		// The F4 attachment surface (IMPL-25): the SAME store implements
		// the attachment port (the CAS-backed durable attachment records
		// over migration 0012).
		ConversationAttachments: store,
		Notifications:           store,
		// The B1 KnowledgePack family surface (DE-0 queue row 1): the SAME
		// store implements the knowledge port (the knowledge tables over
		// migration 0013, the M-9 ruling transactionally under the same
		// single-writer gate).
		Knowledge: store,
		// The B2 Memory family surface (DE-0 queue row 3): the SAME store
		// implements the memory port (the memory tables over migration
		// 0014, the M-17/M-16/M-18 rulings transactionally under the same
		// single-writer gate).
		Memory: store,
		// The B3 ContextSnapshot family surface (DE-0 queue row 5): the
		// SAME store implements the context port (the context tables over
		// migration 0015 — the M-18 request-scoped inclusion ruling
		// transactionally under the same single-writer gate; the two
		// conditional commands serve their typed unmet-condition refusals
		// before any store call while their un-gate preconditions stand).
		Context: store,
		// The B4 Workflow authoring family surface (DE-0 queue row 6): the
		// SAME store implements the workflow port (the workflow tables over
		// migration 0016 — the aggregate-create and M-21 keyed rulings
		// transactionally under the same single-writer gate; the immutable
		// revision rows are INSERT-only). THE D-40 GATE: the port carries
		// NO run-execution member — nothing here enables any execution.
		Workflow: store,
		// The B5 Plugin lifecycle family surface (DE-0 queue row 8): the
		// SAME store implements the plugin port (the plugin tables over
		// migration 0017 — the C1-07 install and the short-window keyed
		// rulings transactionally under the same single-writer gate; the
		// M-29 toggle is the Core-recorded state-checked mutation). THE
		// GATED EXECUTION ROWS: the port carries NO invocation member —
		// nothing here enables any execution (D-13/D-14 + the Core MB
		// §10.2 runtime proof keep their recorded states). THE NEVER-GRANT
		// LAW: no member grants a capability.
		Plugin:              store,
		ConnectionExecutors: newCustomExecutorFactory(secrets, reg),
		// The C-4 session-command surface (IMPL-23): the SAME session
		// service the Guard's policy and the local transport already hold
		// — the coded Revoke/RevokeAll classes served through the slice.
		Sessions: bundle.sessions,
		// The F12 audit read surface (IMPL-24): the lazily opened store's
		// audit row query behind the app slice's read port (the
		// lazyAuditStore adaptation precedent).
		AuditReader: auditRowReader{l: lz},
	}
	slice, _ := app.NewSlice(deps)
	return slice, deps, nil
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
	out.report(rec.Events)
	return int(runtime.ExitOK)
}

func submitJobViaRunningDaemon(ctx context.Context, _ cliFlags, out *printer, cfg config.Config, ds platform.DirSet, sf jobSubmitFlags, corr string, rec platform.OwnerRecord, token string) int {
	addr := ""
	if b, err := os.ReadFile(filepath.Join(ds.Runtime, "http_address")); err == nil {
		addr = strings.TrimSpace(string(b))
	}
	if addr == "" && cfg.TransportLocalHTTPPort > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", cfg.TransportLocalHTTPPort)
	}
	if addr == "" {
		addr = "127.0.0.1:27210"
	}

	origin := "http://localhost:5173"
	bootstrapSecret := ""
	if b, err := os.ReadFile(filepath.Join(ds.Runtime, "bootstrap_token")); err == nil {
		bootstrapSecret = strings.TrimSpace(string(b))
	}
	if bootstrapSecret == "" {
		resp, err := platform.CallIPCWithOrigin(ds.Runtime, platform.IPCOpBootstrapToken, rec.InstanceUUID, token, origin)
		if err == nil && resp.OK {
			var bresp struct {
				Secret string `json:"secret"`
			}
			if err := json.Unmarshal(resp.Body, &bresp); err == nil && bresp.Secret != "" {
				bootstrapSecret = bresp.Secret
			} else {
				bootstrapSecret = strings.Trim(string(resp.Body), "\" \n\r")
			}
		}
	}
	if bootstrapSecret == "" {
		env, _ := errs.New(errs.CodeSingleInstanceConflict, corr, map[string]any{
			"reason": fmt.Sprintf("daemon is active (pid %d) but bootstrap authentication token is unavailable", rec.PID),
		})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAlreadyRunning})
		return int(runtime.ExitAlreadyRunning)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := "http://" + addr

	exReq, _ := json.Marshal(map[string]any{
		"credential": bootstrapSecret,
		"scopes":     []string{"data"},
	})
	exHTTPReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/session/exchange", bytes.NewReader(exReq))
	exHTTPReq.Header.Set("Content-Type", "application/json")
	exHTTPReq.Header.Set("Origin", origin)
	httpResp, err := client.Do(exHTTPReq)
	if err != nil {
		env, _ := errs.New(errs.CodeSingleInstanceConflict, corr, map[string]any{
			"reason": fmt.Sprintf("daemon is active (pid %d) but HTTP transport unreachable at %s: %v", rec.PID, addr, err),
		})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAlreadyRunning})
		return int(runtime.ExitAlreadyRunning)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		env, _ := errs.New(errs.CodeAuthzDenied, corr, map[string]any{
			"reason": fmt.Sprintf("session exchange rejected with status %d", httpResp.StatusCode),
		})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitAuthority})
		return int(runtime.ExitAuthority)
	}

	var exRes struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&exRes); err != nil || exRes.Token == "" {
		env, _ := errs.New(errs.CodeInternal, corr, map[string]any{"reason": "failed to decode session token"})
		out.failure(&runtime.Failure{Envelope: env, Exit: runtime.ExitInternal})
		return int(runtime.ExitInternal)
	}
	sessionSecret := exRes.Token

	classification := sf.classification
	if classification == "" && sf.provider == dprovider.ProviderMock {
		classification = dprovider.ClassificationTestFixture
	}

	subPayload, _ := json.Marshal(map[string]any{
		"idempotency_key": sf.idempotencyKey,
		"task":            sf.task,
		"provider":        sf.provider,
		"model":           sf.model,
		"classification":  classification,
		"base_url":        sf.baseURL,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/jobs", bytes.NewReader(subPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionSecret)
	subResp, err := client.Do(req)
	if err != nil {
		out.report(map[string]any{"error": err.Error()})
		return int(runtime.ExitFailure)
	}
	defer subResp.Body.Close()
	if subResp.StatusCode != http.StatusAccepted && subResp.StatusCode != http.StatusOK {
		var errEnv struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(subResp.Body).Decode(&errEnv)
		out.report(map[string]any{"error": fmt.Sprintf("job submission refused (status %d): %s", subResp.StatusCode, errEnv.Code)})
		return int(runtime.ExitFailure)
	}

	var subResult struct {
		Job struct {
			JobID string `json:"job_id"`
			State string `json:"state"`
		} `json:"job"`
	}
	if err := json.NewDecoder(subResp.Body).Decode(&subResult); err != nil || subResult.Job.JobID == "" {
		out.report(map[string]any{"error": "invalid submit response from daemon"})
		return int(runtime.ExitFailure)
	}
	jobID := subResult.Job.JobID

	pollTicker := time.NewTicker(100 * time.Millisecond)
	defer pollTicker.Stop()
	timeout := time.After(60 * time.Second)

	var finalState struct {
		Job struct {
			JobID        string `json:"job_id"`
			State        string `json:"state"`
			AttemptCount int    `json:"attempt_count"`
			ReceiptID    string `json:"receipt_id"`
			ResultDigest string `json:"result_digest"`
		} `json:"job"`
	}

	for {
		select {
		case <-ctx.Done():
			out.report(map[string]any{"error": ctx.Err().Error()})
			return int(runtime.ExitFailure)
		case <-timeout:
			out.report(map[string]any{"error": "timed out waiting for job settlement"})
			return int(runtime.ExitFailure)
		case <-pollTicker.C:
			getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/jobs/"+jobID, nil)
			getReq.Header.Set("Authorization", "Bearer "+sessionSecret)
			getResp, err := client.Do(getReq)
			if err != nil {
				continue
			}
			if getResp.StatusCode == http.StatusOK {
				var statusRes struct {
					Job struct {
						JobID        string `json:"job_id"`
						State        string `json:"state"`
						AttemptCount int    `json:"attempt_count"`
						ReceiptID    string `json:"receipt_id"`
						ResultDigest string `json:"result_digest"`
					} `json:"job"`
				}
				if json.NewDecoder(getResp.Body).Decode(&statusRes) == nil {
					getResp.Body.Close()
					finalState = statusRes
					if statusRes.Job.State == "succeeded" || statusRes.Job.State == "failed" || statusRes.Job.State == "cancelled" {
						goto PollDone
					}
				} else {
					getResp.Body.Close()
				}
			} else {
				getResp.Body.Close()
			}
		}
	}

PollDone:
	report := map[string]any{
		"job_id":         finalState.Job.JobID,
		"state":          finalState.Job.State,
		"attempt_count":  finalState.Job.AttemptCount,
		"receipt_id":     finalState.Job.ReceiptID,
		"result_digest":  finalState.Job.ResultDigest,
		"correlation_id": corr,
	}

	if finalState.Job.State == "succeeded" {
		resReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/jobs/"+jobID+"/result", nil)
		resReq.Header.Set("Authorization", "Bearer "+sessionSecret)
		if resResp, err := client.Do(resReq); err == nil && resResp.StatusCode == http.StatusOK {
			defer resResp.Body.Close()
			if payloadBytes, err := io.ReadAll(resResp.Body); err == nil {
				report["result"] = json.RawMessage(payloadBytes)
			}
		}
	}

	evReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/jobs/"+jobID+"/events", nil)
	evReq.Header.Set("Authorization", "Bearer "+sessionSecret)
	if evResp, err := client.Do(evReq); err == nil && evResp.StatusCode == http.StatusOK {
		defer evResp.Body.Close()
		var evList struct {
			Events []struct {
				EventType     string          `json:"event_type"`
				InlinePayload json.RawMessage `json:"inline_payload,omitempty"`
			} `json:"events"`
		}
		if json.NewDecoder(evResp.Body).Decode(&evList) == nil {
			ids := make([]string, 0, len(evList.Events))
			for _, r := range evList.Events {
				ids = append(ids, r.EventType)
			}
			report["events"] = ids
			if len(evList.Events) > 0 && len(evList.Events[len(evList.Events)-1].InlinePayload) > 0 {
				var lastPayload map[string]any
				if json.Unmarshal(evList.Events[len(evList.Events)-1].InlinePayload, &lastPayload) == nil {
					report["last_event"] = lastPayload
				}
			}
		}
	}

	out.report(report)
	if finalState.Job.State == "succeeded" {
		return int(runtime.ExitOK)
	}
	return int(runtime.ExitFailure)
}
