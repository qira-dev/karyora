package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

func testDirs(t *testing.T) platform.DirSet {
	t.Helper()
	tmp := t.TempDir()
	ds := platform.DirSet{
		Data:    filepath.Join(tmp, "data"),
		Config:  filepath.Join(tmp, "config"),
		Runtime: filepath.Join(tmp, "runtime"),
		State:   filepath.Join(tmp, "state"),
	}
	if err := os.MkdirAll(ds.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	return ds
}

func okPorts() Ports {
	return Ports{
		Migrate:        func(ctx context.Context) error { return nil },
		Reconcile:      func(ctx context.Context) error { return nil },
		BumpGeneration: func(ctx context.Context) (uint64, error) { return 7, nil },
		FencePrior:     func(ctx context.Context, generation uint64) error { return nil },
	}
}

func TestStartHappyPathReachesReady(t *testing.T) {
	ds := testDirs(t)
	fenced := uint64(0)
	ports := okPorts()
	ports.FencePrior = func(ctx context.Context, generation uint64) error { fenced = generation; return nil }
	r := NewRunner(ds, ports, nil)
	if f := r.Start(context.Background(), "c1", "test-version"); f != nil {
		t.Fatalf("start failed: %v", f)
	}
	if r.Machine().Current() != StateReady {
		t.Fatalf("state = %s, want ready", r.Machine().Current())
	}
	if r.Generation() != 7 {
		t.Fatalf("generation = %d, want 7", r.Generation())
	}
	if fenced != 7 {
		t.Fatalf("fence generation = %d, want 7", fenced)
	}
	rec, ok := r.Owner()
	if !ok || rec.PID != os.Getpid() {
		t.Fatalf("owner snapshot mismatch: %+v", rec)
	}

	out := r.Drain(context.Background(), "test")
	if !out.Completed {
		t.Fatalf("drain outcome = %+v, want completed", out)
	}
	if r.Machine().Current() != StateStopped {
		t.Fatalf("state after drain = %s, want stopped", r.Machine().Current())
	}
	// Ownership is released: a fresh acquire succeeds.
	if _, _, err := platform.AcquireOwnership(ds, ds.Data, "v"); err != nil {
		t.Fatalf("lock must be free after drain: %v", err)
	}
}

func TestStartLockConflict(t *testing.T) {
	ds := testDirs(t)
	r1 := NewRunner(ds, okPorts(), nil)
	if f := r1.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("first start: %v", f)
	}
	defer r1.Drain(context.Background(), "test")

	r2 := NewRunner(ds, okPorts(), nil)
	f := r2.Start(context.Background(), "c2", "v")
	if f == nil {
		t.Fatal("second start must lose the authority race")
	}
	if f.Exit != ExitAlreadyRunning {
		t.Fatalf("exit = %d, want EXIT_ALREADY_RUNNING", f.Exit)
	}
	if f.Envelope.Code != errs.CodeSingleInstanceConflict {
		t.Fatalf("code = %s, want SINGLE_INSTANCE_CONFLICT", f.Envelope.Code)
	}
}

func TestStartMigrationFailureGatesDispatch(t *testing.T) {
	ds := testDirs(t)
	ports := okPorts()
	ports.Migrate = func(ctx context.Context) error { return errors.New("disk unavailable") }
	r := NewRunner(ds, ports, nil)
	f := r.Start(context.Background(), "c1", "v")
	if f == nil {
		t.Fatal("migrate failure must fail the start")
	}
	if f.Exit != ExitRecoveryRequired {
		t.Fatalf("exit = %d, want EXIT_RECOVERY_REQUIRED", f.Exit)
	}
	if f.Envelope.Code != errs.CodeMigrationFailed {
		t.Fatalf("code = %s, want MIGRATION_FAILED", f.Envelope.Code)
	}
	if r.Machine().Current() != StateFailed {
		t.Fatalf("state = %s, want failed", r.Machine().Current())
	}
	// The lock is released on failure: another start can take over.
	if _, _, err := platform.AcquireOwnership(ds, ds.Data, "v"); err != nil {
		t.Fatalf("lock must be free after failed start: %v", err)
	}
}

func TestStartDefaultMigrateFailsClosed(t *testing.T) {
	ds := testDirs(t)
	// No storage stack wired: the runner never reaches Ready.
	r := NewRunner(ds, Ports{}, nil)
	f := r.Start(context.Background(), "c1", "v")
	if f == nil {
		t.Fatal("unwired storage must fail closed")
	}
	if f.Exit != ExitRecoveryRequired {
		t.Fatalf("exit = %d, want EXIT_RECOVERY_REQUIRED", f.Exit)
	}
	if r.Machine().Current() == StateReady {
		t.Fatal("no dispatch without a storage stack")
	}
}

func TestStartReconcileFailure(t *testing.T) {
	ds := testDirs(t)
	ports := okPorts()
	ports.Reconcile = func(ctx context.Context) error { return errors.New("boom") }
	r := NewRunner(ds, ports, nil)
	f := r.Start(context.Background(), "c1", "v")
	if f == nil || f.Exit != ExitRecoveryRequired {
		t.Fatalf("reconcile failure must map to EXIT_RECOVERY_REQUIRED: %+v", f)
	}
	if f.Envelope.Code != errs.CodeRecoveryRequired {
		t.Fatalf("code = %s, want RECOVERY_REQUIRED", f.Envelope.Code)
	}
}

func TestDrainDeadlineExpiry(t *testing.T) {
	old := drainBudget
	drainBudget = 50 * time.Millisecond
	defer func() { drainBudget = old }()

	ds := testDirs(t)
	ports := okPorts()
	ports.Persist = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}
	r := NewRunner(ds, ports, nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	out := r.Drain(context.Background(), "test")
	if out.Completed || !out.DeadlineExceeded {
		t.Fatalf("drain outcome = %+v, want deadline expiry", out)
	}
	// The process ends in a legal terminal state and the lock is free for
	// the crash-recovery start.
	if r.Machine().Current() != StateStopped {
		t.Fatalf("state = %s, want stopped", r.Machine().Current())
	}
	if _, _, err := platform.AcquireOwnership(ds, ds.Data, "v"); err != nil {
		t.Fatalf("lock must be free after deadline expiry: %v", err)
	}
}

func TestDrainIsIdempotent(t *testing.T) {
	ds := testDirs(t)
	r := NewRunner(ds, okPorts(), nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	out1 := r.Drain(context.Background(), "first")
	out2 := r.Drain(context.Background(), "second")
	if !out1.Completed || !out2.Completed {
		t.Fatalf("repeated drain must be stable: %+v %+v", out1, out2)
	}
}

func TestIPCStatusAndStop(t *testing.T) {
	ds := testDirs(t)
	r := NewRunner(ds, okPorts(), nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	closeIPC, err := r.ServeIPC()
	if err != nil {
		t.Fatal(err)
	}

	rec, token, err := platform.OwnerCredentials(ds.Runtime)
	if err != nil {
		t.Fatal(err)
	}

	// Status round trip returns the live report with the five signal groups.
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStatus, rec.InstanceUUID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("status must pass: %+v", resp)
	}
	var report StatusReport
	if err := json.Unmarshal(resp.Body, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Live || report.Health.Process.State != string(StateReady) {
		t.Fatalf("live ready report expected: %+v", report)
	}
	if !report.Health.Ready() {
		t.Fatalf("readiness gate must hold on the live report: %+v", report.Health)
	}

	// Stop over IPC triggers the graceful drain and reports the outcome.
	resp, err = platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("stop must pass: %+v", resp)
	}
	var outcome DrainOutcome
	if err := json.Unmarshal(resp.Body, &outcome); err != nil {
		t.Fatal(err)
	}
	if !outcome.Completed {
		t.Fatalf("drain outcome = %+v, want completed", outcome)
	}
	if r.Machine().Current() != StateStopped {
		t.Fatalf("state = %s, want stopped", r.Machine().Current())
	}
	_ = closeIPC()

	// After the drain the credentials are gone: the next stop is the
	// idempotent not-running path (C2-07 §2).
	if _, _, err := platform.OwnerCredentials(ds.Runtime); err == nil {
		t.Fatal("owner record must be removed after drain")
	}
}

func TestIPCStopDeadlineExtension(t *testing.T) {
	// The stop response must survive a drain that consumes most of the
	// budget (the connection deadline extends past the drain budget).
	old := drainBudget
	drainBudget = 1500 * time.Millisecond
	defer func() { drainBudget = old }()
	ds := testDirs(t)
	ports := okPorts()
	ports.Persist = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return nil
		}
	}
	r := NewRunner(ds, ports, nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	closeIPC, err := r.ServeIPC()
	if err != nil {
		t.Fatal(err)
	}
	defer closeIPC()
	rec, token, _ := platform.OwnerCredentials(ds.Runtime)
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		t.Fatalf("stop over IPC after a 1s drain: %v", err)
	}
	if !resp.OK {
		t.Fatalf("stop response: %+v", resp)
	}
}

func TestStatusReportNotRunning(t *testing.T) {
	rep := NotRunningReport(nil)
	if rep.Live {
		t.Fatal("not-running report must not claim liveness")
	}
	if rep.Health.Ready() {
		t.Fatal("not-running report must never derive readiness")
	}
	b, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"live", "health"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("stable key %q missing from machine form", key)
		}
	}
}

func TestExitCodeRegister(t *testing.T) {
	cases := []struct {
		code errs.Code
		want ExitCode
	}{
		{errs.CodeValidationFailed, ExitConfigInvalid},
		{errs.CodeConfigConflict, ExitConfigInvalid},
		{errs.CodeSingleInstanceConflict, ExitAlreadyRunning},
		{errs.CodeAuthzDenied, ExitAuthority},
		{errs.CodeStorageUnavailable, ExitStorage},
		{errs.CodeStateCorrupt, ExitStorage},
		{errs.CodeMigrationFailed, ExitRecoveryRequired},
		{errs.CodeRecoveryRequired, ExitRecoveryRequired},
		{errs.CodeInternal, ExitInternal},
	}
	for _, c := range cases {
		env, _ := errs.New(c.code, "c1", nil)
		if got := ExitCodeForFailure(env); got != c.want {
			t.Fatalf("%s → %d, want %d", c.code, got, c.want)
		}
	}
	// The register identifiers are stable English.
	if ExitName(ExitOK) != "EXIT_OK" || ExitName(ExitInternal) != "EXIT_INTERNAL" {
		t.Fatal("exit identifiers must stay stable English")
	}
}

func TestExitCodeForUnknownFailsClosed(t *testing.T) {
	env := errs.Envelope{Code: "SOMETHING_NEW", Retryable: false, CorrelationID: "c1"}
	if got := ExitCodeForFailure(env); got != ExitFailure {
		t.Fatalf("unknown code → %d, want EXIT_FAILURE", got)
	}
}

func TestLocaleFallback(t *testing.T) {
	loc := NewLocale("en")
	if loc.Name() != "en" {
		t.Fatal("en resolves itself")
	}
	// A locale without a shipped catalog resolves the English source
	// catalog (visible fallback; the 12-locale matrix is C2-18 evidence).
	loc = NewLocale("de")
	if loc.Text(KeyStatusLive) != NewLocale("en").Text(KeyStatusLive) {
		t.Fatal("missing keys must fall back to English")
	}
	if loc.Text("nonexistent.key") != "" {
		t.Fatal("an unknown key resolves to empty copy, never invented text")
	}
}

func TestLoadEffectiveConfigLayers(t *testing.T) {
	dir := t.TempDir()
	// Missing file: defaults apply.
	cfg, f := LoadEffectiveConfig("c1", dir, nil, func(string) string { return "" })
	if f != nil || cfg.BusyTimeoutMS != 5000 {
		t.Fatalf("defaults expected: %+v %+v", cfg, f)
	}
	// File layer applies.
	content := `{"busy_timeout_ms": 2500}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, f = LoadEffectiveConfig("c1", dir, nil, func(string) string { return "" })
	if f != nil || cfg.BusyTimeoutMS != 2500 {
		t.Fatalf("file layer expected: %+v %+v", cfg, f)
	}
	// Flag layer wins over the file.
	cfg, f = LoadEffectiveConfig("c1", dir, map[string]any{"busy_timeout_ms": 900}, func(string) string { return "" })
	if f != nil || cfg.BusyTimeoutMS != 900 {
		t.Fatalf("flag layer expected: %+v %+v", cfg, f)
	}
	// Unknown key fails closed with EXIT_CONFIG_INVALID.
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(`{"max_jobs_per_tenant": 5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, f = LoadEffectiveConfig("c1", dir, nil, func(string) string { return "" })
	if f == nil || f.Exit != ExitConfigInvalid {
		t.Fatalf("unknown key must fail closed: %+v", f)
	}
}

func TestLoadEffectiveConfigMalformedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, f := LoadEffectiveConfig("c1", dir, nil, func(string) string { return "" })
	if f == nil || f.Exit != ExitConfigInvalid {
		t.Fatalf("malformed file must fail closed: %+v", f)
	}
}
