package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The M-7 retry command's storage pins (IMPL-21, the F9 family's frozen
// semantics): a NEW Attempt of the SAME Job — never a new Job — the
// Supplied-keyed replay ruling (the same reference returns the recorded
// outcome, never regenerated; a contradicting re-issue is the typed
// CONFLICT_STATE — the coded approval-decision precedent's class), and
// retry ≠ reversal (no prior record rewritten). Driven through the real
// fenced commit path.

// terminalFailedFixture submits, claims, and commits one Job to the
// terminal Failed disposition through the real fenced commit path.
func terminalFailedFixture(t *testing.T, s *Store, id, key string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture(id, key, "input "+id)); err != nil {
		t.Fatalf("submit: %v", err)
	}
	return claimAndFail(t, s, id)
}

// claimAndFail claims the Job's next attempt and commits it to the
// terminal Failed disposition (a known policy failure, exactly as the
// CommitAttempt path records one).
func claimAndFail(t *testing.T, s *Store, id string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if claim.Job.JobID != id {
		t.Fatalf("claimed %q, want %q", claim.Job.JobID, id)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: id, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobFailed,
		AttemptOutcome: aijobrelay.AttemptFailedTerminal,
		ReceiptID:      "r-" + claim.Attempt.AttemptID,
		CorrelationID:  "corr-" + id,
	}); err != nil {
		t.Fatalf("failed commit: %v", err)
	}
	job, err := s.LoadJob(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return job
}

// TestRetryJobSuppliedKeyedSemantics pins the Supplied-keyed ruling end to
// end at the store: fresh apply, replay (no double-apply), contradicting
// re-issue, consumed reference, and the new-Attempt continuation.
func TestRetryJobSuppliedKeyedSemantics(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	before := terminalFailedFixture(t, s, "j-retry-1", "key-retry-1")
	if before.State != aijobrelay.JobFailed {
		t.Fatalf("fixture must settle Failed, got %s", before.State)
	}

	// Fresh apply: the terminal Failed Job re-queues under the presented
	// reference — a NEW Attempt of the SAME Job (the same job_id, never a
	// second Job).
	res, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-1", IdempotencyReference: "ref-1"})
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if res.Replayed {
		t.Fatalf("a fresh apply must not report replayed: %+v", res)
	}
	if res.Job.JobID != "j-retry-1" || res.Job.State != aijobrelay.JobQueued {
		t.Fatalf("the retry must re-queue the SAME Job, got %+v", res.Job)
	}
	if res.Job.Version != before.Version+1 {
		t.Fatalf("the re-queue must fence-bump the version: %d vs %d", res.Job.Version, before.Version)
	}

	// The scheduler's next claim creates the NEW Attempt (seq 2 — never a
	// rewrite of the prior attempt).
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w2", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim after retry: %v %v", ok, err)
	}
	if claim.Job.JobID != "j-retry-1" || claim.Attempt.Seq != before.AttemptCount+1 {
		t.Fatalf("the retry must continue with a NEW attempt of the same Job: %+v", claim)
	}

	// Replay: the same reference re-presented while its recorded outcome
	// still stands returns the outcome WITHOUT re-applying — no second
	// re-queue, no state change.
	replay, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-1", IdempotencyReference: "ref-1"})
	if err != nil {
		t.Fatalf("retry replay: %v", err)
	}
	if !replay.Replayed {
		t.Fatalf("the same reference must replay: %+v", replay)
	}
	if replay.Job.Version != claim.Job.Version || replay.Job.State != aijobrelay.JobRunning {
		t.Fatalf("the replay must return the recorded outcome unchanged: %+v", replay.Job)
	}

	// Contradicting re-issue: a different reference against the standing
	// retry is the typed CONFLICT_STATE, never a second re-queue.
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-1", IdempotencyReference: "ref-other"}); err == nil {
		t.Fatal("a contradicting re-issue must refuse")
	} else if codeOfStore(t, err) != errs.CodeConflictState {
		t.Fatalf("the contradicting re-issue must be the typed CONFLICT_STATE, got %v", err)
	}

	// Consumed reference: drive the Job to terminal Failed again (commit
	// the new Attempt's claim); the original reference re-presented cannot
	// start a new cycle (a retry is never regenerated) — a NEW retry is a
	// NEW reference.
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: "j-retry-1", JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w2", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobFailed,
		AttemptOutcome: aijobrelay.AttemptFailedTerminal,
		ReceiptID:      "r-" + claim.Attempt.AttemptID,
		CorrelationID:  "corr-j-retry-1",
	}); err != nil {
		t.Fatalf("re-fail commit: %v", err)
	}
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-1", IdempotencyReference: "ref-1"}); err == nil {
		t.Fatal("a consumed reference must never regenerate")
	} else if codeOfStore(t, err) != errs.CodeConflictState {
		t.Fatalf("the consumed reference must be the typed CONFLICT_STATE, got %v", err)
	}
	again, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-1", IdempotencyReference: "ref-2"})
	if err != nil {
		t.Fatalf("a new reference on a new terminal arrival must apply: %v", err)
	}
	if again.Replayed || again.Job.State != aijobrelay.JobQueued {
		t.Fatalf("the new retry must apply fresh: %+v", again)
	}
}

// TestRetryJobEligibilityFence pins the same eligibility truth at the
// store: a non-terminal Job without a standing retry refuses (the outcome
// is still pending); Succeeded stays exitless (a re-run is not a retry).
func TestRetryJobEligibilityFence(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// A queued Job: non-terminal, no retry stands — refused.
	if _, err := s.SubmitJob(ctx, submitFixture("j-retry-q", "key-retry-q", "in")); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-q", IdempotencyReference: "ref-q"}); err == nil {
		t.Fatal("a queued job must not be retry-eligible")
	} else if codeOfStore(t, err) != errs.CodeConflictState {
		t.Fatalf("the ineligible retry must be the typed CONFLICT_STATE, got %v", err)
	}

	// A succeeded Job: a re-run is not a retry (the register stays
	// exitless) — refused through the same transition fence.
	if _, err := s.SubmitJob(ctx, submitFixture("j-retry-s", "key-retry-s", "in")); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// The FIFO claim picks the still-queued j-retry-q first; removing it
	// from the queue (claimed → Running) leaves j-retry-s for the next
	// claim.
	if _, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w0", MaxMemoryMB: 1024}, time.Minute); err != nil || !ok {
		t.Fatalf("drain claim: %v %v", ok, err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if claim.Job.JobID != "j-retry-s" {
		t.Fatalf("claimed %q, want j-retry-s", claim.Job.JobID)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: "j-retry-s", JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobSucceeded,
		AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest:   inputDigest("result"),
		ReceiptID:      "r-" + claim.Attempt.AttemptID,
		CorrelationID:  "corr-j-retry-s",
	}); err != nil {
		t.Fatalf("succeeded commit: %v", err)
	}
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-retry-s", IdempotencyReference: "ref-s"}); err == nil {
		t.Fatal("a succeeded job must not be retry-eligible (a re-run is not a retry)")
	} else if codeOfStore(t, err) != errs.CodeConflictState {
		t.Fatalf("the succeeded retry must be the typed CONFLICT_STATE, got %v", err)
	}
}

// TestRetryJobRequestValidation pins the request-shape refusals: the
// Supplied-keyed reference is required and bounded; an unknown identity is
// the typed NOT_FOUND (the no-leak rule).
func TestRetryJobRequestValidation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "j-x", IdempotencyReference: ""}); err == nil {
		t.Fatal("the reference is required (the operation carries it)")
	} else if codeOfStore(t, err) != errs.CodeValidationFailed {
		t.Fatalf("expected VALIDATION_FAILED, got %v", err)
	}
	if _, err := s.RetryJob(ctx, aijobrelay.RetryRequest{JobID: "job-does-not-exist", IdempotencyReference: "ref-1"}); err == nil {
		t.Fatal("an unknown identity must refuse")
	} else if codeOfStore(t, err) != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
}

// codeOfStore extracts the typed code from a storage boundary error.
func codeOfStore(t *testing.T, err error) errs.Code {
	t.Helper()
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("expected a typed boundary error, got %T: %v", err, err)
	}
	return se.Code
}
