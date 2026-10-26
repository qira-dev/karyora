package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	h.t.Helper()
	f, err := os.CreateTemp(h.t.TempDir(), "out-*")
	if err != nil {
		h.t.Fatal(err)
	}
	defer os.Remove(f.Name())
	code := run(context.Background(), args, f)
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
	cmd, flags := parseArgs([]string{"status", "--json", "--data-dir", "/tmp/d", "--locale=en"})
	if cmd != "status" {
		t.Fatalf("cmd = %q", cmd)
	}
	if !flags.json || flags.dataDir != "/tmp/d" || flags.locale != "en" {
		t.Fatalf("flags = %+v", flags)
	}
	cmd, _ = parseArgs([]string{"--json"})
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

func TestStartFailsClosedWithoutStorageStack(t *testing.T) {
	// The storage stack is not linked yet: start never reaches Ready and
	// exits EXIT_RECOVERY_REQUIRED (6).
	code, out := runCapture(t, "start", "--json")
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (output %q)", code, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("failure must print the machine envelope: %q", out)
	}
	if env["code"] != "MIGRATION_FAILED" {
		t.Fatalf("code = %v, want MIGRATION_FAILED", env["code"])
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

func TestStartThenConflictThenStop(t *testing.T) {
	// Start would fail closed (no storage stack), so this scenario exercises
	// the conflict/stop paths through the runtime package directly is covered
	// there; here we verify start's failure releases ownership so the next
	// start reports the same stable failure rather than already-running.
	code1, _ := runCapture(t, "start", "--json")
	code2, _ := runCapture(t, "start", "--json")
	if code1 != 6 || code2 != 6 {
		t.Fatalf("both starts must fail with EXIT_RECOVERY_REQUIRED: %d %d", code1, code2)
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
