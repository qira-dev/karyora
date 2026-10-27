package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/platform"
)

// harness isolates one test's environment (XDG roots under one temp dir) so
// multiple runs share state, mirroring a real install.
type harness struct {
	t *testing.T
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "rt"))
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	return &harness{t: t}
}

func (h *harness) run(args ...string) (int, string) {
	return h.runCtx(context.Background(), args...)
}

func (h *harness) runCtx(ctx context.Context, args ...string) (int, string) {
	h.t.Helper()
	f, err := os.CreateTemp(h.t.TempDir(), "out-*")
	if err != nil {
		h.t.Fatal(err)
	}
	defer os.Remove(f.Name())
	code := run(ctx, args, f)
	f.Close()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		h.t.Fatal(err)
	}
	return code, string(b)
}

func runCapture(t *testing.T, args ...string) (int, string) {
	t.Helper()
	return newHarness(t).run(args...)
}

func TestParseArgs(t *testing.T) {
	cmd, flags, rest := parseArgs([]string{"status", "--json", "--data-dir", "/tmp/d", "--locale=en"})
	if len(rest) != 0 {
		t.Fatalf("status carries no positional words, got %v", rest)
	}
	if cmd != "status" {
		t.Fatalf("cmd = %q", cmd)
	}
	if !flags.json || flags.dataDir != "/tmp/d" || flags.locale != "en" {
		t.Fatalf("flags = %+v", flags)
	}
	cmd, _, _ = parseArgs([]string{"--json"})
	if cmd != "" {
		t.Fatalf("empty arg list must yield no command: %q", cmd)
	}
}

func TestHelpAndUnknownCommand(t *testing.T) {
	code, out := runCapture(t, "help")
	if code != 0 || !strings.Contains(out, "usage: uawp") {
		t.Fatalf("help: code=%d out=%q", code, out)
	}
	code, _ = runCapture(t, "make-coffee")
	if code != 1 {
		t.Fatalf("unknown command must exit EXIT_FAILURE, got %d", code)
	}
}

func TestVersionCommand(t *testing.T) {
	code, out := runCapture(t, "version")
	if code != 0 || !strings.HasPrefix(out, "karyora core ") {
		t.Fatalf("version: code=%d out=%q", code, out)
	}
}

func TestStatusNotRunning(t *testing.T) {
	code, out := runCapture(t, "status", "--json")
	if code != 0 {
		t.Fatalf("status of a stopped core is a stable report, exit %d", code)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("status must print machine JSON: %q", out)
	}
	if report["live"] != false {
		t.Fatalf("live must be false: %v", report["live"])
	}
}

func TestStatusHumanForm(t *testing.T) {
	code, out := runCapture(t, "status")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, "core is not running") {
		t.Fatalf("human form expected, got %q", out)
	}
}

func TestStopIdempotentWhenNotRunning(t *testing.T) {
	code, out := runCapture(t, "stop", "--json")
	if code != 0 {
		t.Fatalf("stop of a stopped core is the stable no-op, exit %d", code)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stop must print the machine report: %q", out)
	}
}

// TestStartReadyStatusStopCycle is the end-to-end C2-07 lifecycle: start
// reaches Ready with the storage stack wired (migrations, journal, CAS),
// status reports the live ready state without deriving it from liveness,
// the exit is the graceful bounded drain, and a subsequent stop is the
// idempotent no-op.
func TestStartReadyStatusStopCycle(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		code int
		out  string
	}
	done := make(chan result, 1)
	go func() {
		code, out := h.runCtx(ctx, "start", "--json")
		done <- result{code, out}
	}()
	// Wait for the Core to serve its IPC endpoint.
	deadline := time.Now().Add(15 * time.Second)
	live := false
	for time.Now().Before(deadline) {
		code, out := h.run("status", "--json")
		if code == 0 && strings.Contains(out, `"live": true`) {
			live = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !live {
		cancel()
		select {
		case r := <-done:
			t.Fatalf("start never became live: exit=%d out=%s", r.code, r.out)
		case <-time.After(25 * time.Second):
			t.Fatal("start did not exit after cancellation")
		}
	}
	// The live status report carries the ready state and the five groups.
	_, out := h.run("status", "--json")
	var report struct {
		Live   bool `json:"live"`
		Health struct {
			Process struct {
				State             string `json:"state"`
				MigrationsApplied bool   `json:"migrationsApplied"`
				RecoveryComplete  bool   `json:"recoveryComplete"`
			} `json:"process"`
		} `json:"health"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("status report: %v (%s)", err, out)
	}
	if !report.Health.Process.MigrationsApplied || !report.Health.Process.RecoveryComplete || report.Health.Process.State != "ready" {
		t.Fatalf("ready report expected: %s", out)
	}

	// A second start loses the authority race: EXIT_ALREADY_RUNNING.
	code2, out2 := h.run("start", "--json")
	if code2 != 3 {
		t.Fatalf("concurrent start exit = %d, want 3 (out %s)", code2, out2)
	}
	if !strings.Contains(out2, "SINGLE_INSTANCE_CONFLICT") {
		t.Fatalf("conflict report: %s", out2)
	}

	// Cancelling the parent context maps to the graceful-stop request
	// (same path as a signal). The exit is the completed drain.
	cancel()
	select {
	case r := <-done:
		if r.code != 0 {
			t.Fatalf("graceful start exit = %d, want 0 (out %s)", r.code, r.out)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("start did not exit after the stop request")
	}
	// After the drain the next stop is the stable idempotent no-op.
	code3, out3 := h.run("stop", "--json")
	if code3 != 0 {
		t.Fatalf("post-drain stop exit = %d, want 0 (out %s)", code3, out3)
	}
	if strings.Contains(out3, "not_running") == false && !strings.Contains(out3, "false") {
		t.Fatalf("post-drain report should show not running: %s", out3)
	}
}

func TestDoctorReports(t *testing.T) {
	// A started-once install has the class layout; a fresh install reports
	// the missing directories as a real finding (exit 1).
	h := newHarness(t)
	code, out := h.run("doctor", "--json")
	if code == 1 && strings.Contains(out, "missing class directory") {
		// Fresh install: create the layout as start would, then re-run.
		ds, err := platform.Resolve(nil)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := platform.EnsureLayout(ds); err != nil {
			h.t.Fatal(err)
		}
		code, out = h.run("doctor", "--json")
	}
	if code != 0 {
		t.Fatalf("doctor exit = %d (out %q)", code, out)
	}
	if code != 0 {
		t.Fatalf("doctor exit = %d (out %q)", code, out)
	}
	var report struct {
		Overall string `json:"overall"`
		Checks  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor must print machine JSON: %q", out)
	}
	if report.Overall == "" || len(report.Checks) == 0 {
		t.Fatalf("doctor report incomplete: %+v", report)
	}
	// Storage diagnostics are not linked yet: marked unavailable, never
	// silently passed.
	seen := map[string]string{}
	for _, c := range report.Checks {
		seen[c.Name] = c.Status
	}
	if seen["storage"] != "unavailable" {
		t.Fatalf("storage check = %q, want unavailable until block 3 wires it", seen["storage"])
	}
}

func TestDoctorHumanForm(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("doctor")
	if code == 1 && strings.Contains(out, "missing class directory") {
		ds, err := platform.Resolve(nil)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := platform.EnsureLayout(ds); err != nil {
			h.t.Fatal(err)
		}
		code, out = h.run("doctor")
	}
	if code != 0 {
		t.Fatalf("code = %d (out %q)", code, out)
	}
	if !strings.Contains(out, "overall:") {
		t.Fatalf("human form expected, got %q", out)
	}
}
