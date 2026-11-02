package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The post-terminal idempotency replay window (the owner's card-8 working
// value: 72 h after the Job reaches a terminal state, finalized at the
// freeze). These tests pin the boundary behavior: inside the window the
// same key + same digest replays the original Job; outside it the replay
// is REFUSED with the coded CONFLICT_STATE — never a silent second Job
// under a spent key, never a fabricated replay. A non-terminal Job always
// replays regardless of its age: the window starts at the terminal
// transition.

// terminalizeFixture submits, claims, and commits one Job to a terminal
// state through the real fenced commit path, returning the committed Job.
func terminalizeFixture(t *testing.T, s *Store, id, key, input string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture(id, key, input)); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	good := aijobrelay.Commit{JobID: id, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("result"), ReceiptID: "rc-" + id, CorrelationID: "corr-" + id}
	if err := s.CommitAttempt(ctx, good); err != nil {
		t.Fatalf("terminal commit: %v", err)
	}
	job, err := s.LoadJob(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return job
}

func TestIdempotencyReplayInsideWindow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	terminal := terminalizeFixture(t, s, "j-w1", "key-w1", "same input")

	// Inside the window (here: immediately after the terminal commit): the
	// same key + same digest replays the original Job — the C3-02
	// semantics unchanged by the window.
	replay, err := s.SubmitJob(ctx, submitFixture("j-w1-2", "key-w1", "same input"))
	if err != nil {
		t.Fatalf("replay inside the window must succeed: %v", err)
	}
	if replay.JobID != terminal.JobID {
		t.Fatalf("replay must return the original Job, got %s", replay.JobID)
	}

	// The exact boundary edge (now == terminal instant + the window) is
	// INSIDE: the window elapsed only AFTER the edge, so the same-key
	// replay still returns the original.
	s.now = func() time.Time { return terminal.UpdatedAt.Add(IdempotencyReplayWindow) }
	replay, err = s.SubmitJob(ctx, submitFixture("j-w1-3", "key-w1", "same input"))
	if err != nil {
		t.Fatalf("replay at the exact window edge must succeed: %v", err)
	}
	if replay.JobID != terminal.JobID {
		t.Fatalf("edge replay must return the original Job, got %s", replay.JobID)
	}
}

func TestIdempotencyReplayOutsideWindowRefused(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	terminal := terminalizeFixture(t, s, "j-w2", "key-w2", "same input")

	// One instant past the window: the replay is REFUSED with the coded
	// CONFLICT_STATE — the same-key re-issue must never silently mint a
	// second Job under a spent key.
	s.now = func() time.Time { return terminal.UpdatedAt.Add(IdempotencyReplayWindow).Add(time.Nanosecond) }
	_, err := s.SubmitJob(ctx, submitFixture("j-w2-2", "key-w2", "same input"))
	env, ok := err.(*Error)
	if !ok || env.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE past the window, got %v", err)
	}
	if !strings.Contains(err.Error(), "replay window has elapsed") {
		t.Fatalf("the refusal must name the elapsed window, got %v", err)
	}

	// The spent key's Job is untouched: the refusal created no second Job.
	job, err := s.LoadJob(ctx, terminal.JobID)
	if err != nil || job.State != aijobrelay.JobSucceeded {
		t.Fatalf("the refused replay must leave the original Job intact: %+v %v", job, err)
	}

	// A different digest past the window is still the digest conflict
	// (the digest rule precedes the window rule).
	_, err = s.SubmitJob(ctx, submitFixture("j-w2-3", "key-w2", "different input"))
	env, ok = err.(*Error)
	if !ok || env.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE for a different digest, got %v", err)
	}
	if strings.Contains(err.Error(), "replay window has elapsed") {
		t.Fatalf("the digest conflict must keep its own reason, got %v", err)
	}
}

func TestIdempotencyWindowStartsAtTerminalNotCreation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// A non-terminal (queued) Job whose stamps are long past: the window
	// has NOT started, so the same-key replay applies regardless of age.
	aged := submitFixture("j-w3", "key-w3", "same input")
	aged.CreatedAt = time.Now().UTC().Add(-200 * 24 * time.Hour)
	first, err := s.SubmitJob(ctx, aged)
	if err != nil {
		t.Fatalf("aged submit: %v", err)
	}
	replay, err := s.SubmitJob(ctx, submitFixture("j-w3-2", "key-w3", "same input"))
	if err != nil {
		t.Fatalf("a non-terminal Job must replay regardless of age: %v", err)
	}
	if replay.JobID != first.JobID {
		t.Fatalf("replay must return the original Job, got %s", replay.JobID)
	}
}
