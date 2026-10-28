package remotestore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Conformance fixtures for the C4-38 implementation remainder (Remote
// Operations Specification §1), executed against a LIVE PostgreSQL server
// via REMOTESTORE_TEST_DSN (skipped — recorded Not Run — when unset): the
// issue/bind/rotate/revoke lifecycle, revoked-credential refusal at the
// claim boundary, the running-Attempt resolution through the fencing
// rules (renewal and authoritative commit refuse so the lease expires
// normally into reconciliation with fresh-epoch recovery), and the
// never-cross-workspace scope rule.

// TestServiceIdentityLifecycleIssueBindRotateRevoke walks the §1
// lifecycle: the issued credential claims; after rotation the OLD
// credential refuses immediately while the new one claims; after
// revocation nothing authorizes.
func TestServiceIdentityLifecycleIssueBindRotateRevoke(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-life"))
	seedJob(t, s, uid("ws-life"), uid("j-life-a"), "alice", "queued")
	seedJob(t, s, uid("ws-life"), uid("j-life-b"), "alice", "queued")

	worker, credential := issueWorker(t, s, "worker-life", uid("ws-life"))
	if _, err := s.ClaimJob(ctx, worker, credential, []string{uid("ws-life")}); err != nil {
		t.Fatalf("claim with the issued credential: %v", err)
	}

	// Rotation: the new version is current, the old version refuses at
	// the next claim — immediately, never at some lagging TTL.
	seedJob(t, s, uid("ws-life"), uid("j-life-c"), "alice", "queued")
	newCredential, err := s.RotateServiceIdentity(ctx, worker)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if newCredential == credential {
		t.Fatal("rotation must issue a NEW credential")
	}
	if _, err := s.ClaimJob(ctx, worker, credential, []string{uid("ws-life")}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("the superseded credential must refuse at claim, got %v", err)
	}
	if _, err := s.ClaimJob(ctx, worker, newCredential, []string{uid("ws-life")}); err != nil {
		t.Fatalf("claim with the rotated credential: %v", err)
	}

	// Revocation: terminal, immediate at the claim boundary.
	seedJob(t, s, uid("ws-life"), uid("j-life-d"), "alice", "queued")
	if err := s.RevokeServiceIdentity(ctx, worker); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.ClaimJob(ctx, worker, newCredential, []string{uid("ws-life")}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("a revoked identity must refuse at claim, got %v", err)
	}
	// Idempotent: revoking the revoked identity stays a no-op.
	if err := s.RevokeServiceIdentity(ctx, worker); err != nil {
		t.Fatalf("re-revoke must stay idempotent: %v", err)
	}
	// Rotation after revocation refuses (the identity is terminal).
	if _, err := s.RotateServiceIdentity(ctx, worker); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("rotating a revoked identity must refuse, got %v", err)
	}
}

// TestRevokedWorkerResolvesThroughFencing covers Remote Operations §1's
// running-Attempt rule: after mid-run revocation the worker's renewal and
// its next authoritative commit refuse even though its fencing tuple is
// still live; the lease expires normally into reconciliation and a fresh
// claim by an ACTIVE worker takes the job over with a bumped epoch.
func TestRevokedWorkerResolvesThroughFencing(t *testing.T) {
	s := testStore(t)
	s.LeaseTTL = 5 * time.Millisecond // the SERVER clock decides expiry
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-rev"))
	seedJob(t, s, uid("ws-rev"), uid("j-rev"), "alice", "queued")

	worker, credential := issueWorker(t, s, "worker-rev", uid("ws-rev"))
	c, err := s.ClaimJob(ctx, worker, credential, []string{uid("ws-rev")})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Mid-run revocation (the operator's typed, immediate action).
	if err := s.RevokeServiceIdentity(ctx, worker); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// The revoked worker cannot extend its own lease…
	if _, err := s.RenewLease(ctx, c.JobID, c.AttemptID, c.WorkerID, c.FencingEpoch); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("a revoked worker's renewal must refuse, got %v", err)
	}
	// …and cannot land its next authoritative commit, even though the
	// fencing tuple is still LIVE (the identity check precedes fencing).
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-rev"), ResultDigest: "sha256-rev", Outcome: "ok",
	}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("a revoked worker's commit must refuse on the live tuple, got %v", err)
	}

	// The lease expires normally into reconciliation; recovery is a FRESH
	// claim by an active worker with a bumped epoch (spec §3). The re-seed
	// stands in for the reconciliation step that re-queues the Job.
	time.Sleep(100 * time.Millisecond)
	recovered := &Store{pool: s.pool, LeaseTTL: DefaultLeaseTTL}
	seedJob(t, s, uid("ws-rev"), uid("j-rev"), "alice", "queued")
	workerB, credB := issueWorker(t, s, "worker-rev-b", uid("ws-rev"))
	fresh, err := recovered.ClaimJob(ctx, workerB, credB, []string{uid("ws-rev")})
	if err != nil {
		t.Fatalf("fresh claim after revocation: %v", err)
	}
	if fresh.FencingEpoch != c.FencingEpoch+1 {
		t.Fatalf("fresh claim epoch = %d, want the bumped %d", fresh.FencingEpoch, c.FencingEpoch+1)
	}
	if err := recovered.CommitAttempt(ctx, AttemptCommit{
		JobID: fresh.JobID, WorkspaceID: fresh.WorkspaceID, AttemptID: fresh.AttemptID,
		WorkerID: fresh.WorkerID, LeaseID: fresh.LeaseID, FencingEpoch: fresh.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-rev-fresh"), ResultDigest: "sha256-rev-fresh", Outcome: "ok",
	}); err != nil {
		t.Fatalf("fresh-epoch commit by the active worker: %v", err)
	}
	// The revoked worker's stale tuple never follows (identity refused
	// before fencing is even consulted).
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 3, JobState: "succeeded",
		ReceiptID: uid("rc-rev-late"), ResultDigest: "sha256-rev-late", Outcome: "ok",
	}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("the revoked worker must still refuse after recovery, got %v", err)
	}
}

// TestUnboundWorkspaceRefusesWholeClaim covers the never-cross-workspace
// rule: one unbound scope in the request refuses the WHOLE claim (fail
// closed — nothing is claimed from the bound workspaces either), and the
// same worker claiming only its bound scope succeeds.
func TestUnboundWorkspaceRefusesWholeClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := uid("ws-scope-a"), uid("ws-scope-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedJob(t, s, wsA, uid("j-scope-a"), "alice", "queued")
	seedJob(t, s, wsB, uid("j-scope-b"), "mallory", "queued")

	worker, credential := issueWorker(t, s, "worker-scope", wsA) // bound to A only
	if _, err := s.ClaimJob(ctx, worker, credential, []string{wsA, wsB}); !errors.Is(err, ErrWorkspaceNotBound) {
		t.Fatalf("an unbound scope must refuse the whole claim, got %v", err)
	}
	// Nothing was claimed from ANY workspace: B's job stayed queued for
	// its own workers and A's job is still queued too.
	var queued int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM jobs WHERE state = 'queued' AND workspace_id = ANY($1)`, []string{wsA, wsB},
	).Scan(&queued); err != nil {
		t.Fatalf("count queued: %v", err)
	}
	if queued != 2 {
		t.Fatalf("queued jobs after refused claim = %d, want 2 (nothing claimed)", queued)
	}
	// The bound scope alone claims normally — the refusal was the scope
	// gate, not the credential.
	if _, err := s.ClaimJob(ctx, worker, credential, []string{wsA}); err != nil {
		t.Fatalf("claim over the bound scope: %v", err)
	}
}

// TestUnknownIdentityAndWrongCredentialRefused pins the fail-closed
// authentication rows: an unknown identity, a wrong credential, and an
// empty credential all refuse identically at the claim boundary.
func TestUnknownIdentityAndWrongCredentialRefused(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-auth"))
	seedJob(t, s, uid("ws-auth"), uid("j-auth"), "alice", "queued")

	worker, credential := issueWorker(t, s, "worker-auth", uid("ws-auth"))
	if _, err := s.ClaimJob(ctx, uid("worker-unknown"), "some-credential", []string{uid("ws-auth")}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("an unknown identity must refuse, got %v", err)
	}
	if _, err := s.ClaimJob(ctx, worker, "wrong-credential", []string{uid("ws-auth")}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("a wrong credential must refuse, got %v", err)
	}
	if _, err := s.ClaimJob(ctx, worker, "", []string{uid("ws-auth")}); err == nil {
		t.Fatal("an empty credential must refuse")
	}
	// The legitimate credential still claims.
	if _, err := s.ClaimJob(ctx, worker, credential, []string{uid("ws-auth")}); err != nil {
		t.Fatalf("claim with the correct credential: %v", err)
	}
}

// TestServiceIdentityInputRefusals pins the issuance guards: a duplicate
// identity id, an empty workspace set, and rotation of an unknown identity
// all fail closed. No credential material may ever surface in an error.
func TestServiceIdentityInputRefusals(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	worker, credential := issueWorker(t, s, "worker-input", uid("ws-input"))
	if _, err := s.IssueServiceIdentity(ctx, worker, []string{uid("ws-input")}); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("a duplicate identity must refuse, got %v", err)
	}
	if _, err := s.IssueServiceIdentity(ctx, uid("worker-empty"), nil); err == nil {
		t.Fatal("an empty workspace set must refuse")
	}
	if _, err := s.RotateServiceIdentity(ctx, uid("worker-ghost")); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("rotating an unknown identity must refuse, got %v", err)
	}
	if err := s.RevokeServiceIdentity(ctx, uid("worker-ghost")); !errors.Is(err, ErrServiceIdentityRefused) {
		t.Fatalf("revoking an unknown identity must refuse, got %v", err)
	}
	// Value-free errors: no credential material ever appears in an error
	// string (the tampered-credential refusal carries the typed reason
	// only).
	if _, err := s.ClaimJob(ctx, worker, credential+"-tampered", []string{uid("ws-input")}); err == nil || strings.Contains(err.Error(), credential[:8]) {
		t.Fatalf("error text must never carry credential material (err=%v)", err)
	}
}
