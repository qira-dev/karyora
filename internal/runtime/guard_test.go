package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/qira-dev/karyora/internal/platform"
)

// C2-14 §2: the IPC middleware capability check is early rejection only —
// the handler-side decision port is the authoritative check. A wired Guard
// that denies must produce a stable denial response; a wired Guard that
// fails (audit unavailable) must deny state-changing effects.
func TestIPCStopGuardDeny(t *testing.T) {
	ds := testDirs(t)
	r := NewRunner(ds, okPorts(), nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	r.Guard = func(ctx context.Context, action string) error {
		return &authzDenied{code: "AUTHZ_DENIED"}
	}
	closeIPC, err := r.ServeIPC()
	if err != nil {
		t.Fatal(err)
	}
	rec, token, err := platform.OwnerCredentials(ds.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "AUTHZ_DENIED" {
		t.Fatalf("denied stop must return a stable denial: %+v", resp)
	}
	// The machine never left Ready: the effect did not happen.
	if r.Machine().Current() != StateReady {
		t.Fatalf("denied stop must not change state, got %s", r.Machine().Current())
	}
	_ = closeIPC()
	r.Drain(context.Background(), "test-cleanup")
}

func TestIPCStopGuardAuditFailureDeniesEffect(t *testing.T) {
	ds := testDirs(t)
	r := NewRunner(ds, okPorts(), nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	r.Guard = func(ctx context.Context, action string) error {
		// AUDIT_UNAVAILABLE: the stop cannot be recorded, so it is denied
		// rather than executed unrecorded (C2-15 §5).
		return errors.New("security: audit unavailable: store closed")
	}
	closeIPC, err := r.ServeIPC()
	if err != nil {
		t.Fatal(err)
	}
	rec, token, _ := platform.OwnerCredentials(ds.Runtime)
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "AUTHZ_DENIED" {
		t.Fatalf("unrecordable state-changing effect must be denied: %+v", resp)
	}
	if r.Machine().Current() != StateReady {
		t.Fatal("the denied stop must not drain")
	}
	_ = closeIPC()
	r.Drain(context.Background(), "test-cleanup")
}

func TestIPCStopGuardAllows(t *testing.T) {
	ds := testDirs(t)
	r := NewRunner(ds, okPorts(), nil)
	if f := r.Start(context.Background(), "c1", "v"); f != nil {
		t.Fatalf("start: %v", f)
	}
	seen := map[string]bool{}
	r.Guard = func(ctx context.Context, action string) error {
		seen[action] = true
		return nil
	}
	closeIPC, err := r.ServeIPC()
	if err != nil {
		t.Fatal(err)
	}
	rec, token, _ := platform.OwnerCredentials(ds.Runtime)
	if _, err := platform.CallIPC(ds.Runtime, platform.IPCOpStatus, rec.InstanceUUID, token); err != nil {
		t.Fatal(err)
	}
	resp, err := platform.CallIPC(ds.Runtime, platform.IPCOpStop, rec.InstanceUUID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("allowed stop must proceed: %+v", resp)
	}
	if !seen["status.read"] || !seen["lifecycle.stop"] {
		t.Fatalf("both operations must pass the decision port: %v", seen)
	}
	var outcome DrainOutcome
	if err := json.Unmarshal(resp.Body, &outcome); err != nil || !outcome.Completed {
		t.Fatalf("drain outcome: %+v %v", outcome, err)
	}
	_ = closeIPC()
}

// authzDenied is the test stand-in for a policy denial envelope error.
type authzDenied struct{ code string }

func (e *authzDenied) Error() string { return "security: " + e.code }
