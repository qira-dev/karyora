package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

func inputDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func submitFixture(id, key, input string) aijobrelay.Job {
	return aijobrelay.Job{
		JobID: id, IdempotencyKey: key, InputDigest: inputDigest(input),
		State: aijobrelay.JobCreated, Version: 1,
		CorrelationID: "corr-" + id,
		CreatedAt:     time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
}

func TestSubmitIdempotencySemantics(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Same key + same input: replay returns the original Job.
	first, err := s.SubmitJob(ctx, submitFixture("j-a", "key-a", "same input"))
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	replay, err := s.SubmitJob(ctx, submitFixture("j-a-2", "key-a", "same input"))
	if err != nil {
		t.Fatalf("replay submit: %v", err)
	}
	if replay.JobID != first.JobID || replay.State != aijobrelay.JobQueued {
		t.Fatalf("replay returned a different job: %+v vs %+v", replay, first)
	}
	// Same key + different input: CONFLICT, never a second Job.
	_, err = s.SubmitJob(ctx, submitFixture("j-a-3", "key-a", "different input"))
	env, ok := err.(*Error)
	if !ok || env.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE for key reuse with different input, got %v", err)
	}
}

func TestClaimIsAtomicAndStaleCommitsAreRefused(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture("j-c", "key-c", "payload")); err != nil {
		t.Fatal(err)
	}
	w1 := aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}
	w2 := aijobrelay.WorkerCapability{WorkerID: "w2", MaxMemoryMB: 1024}
	claim1, ok1, err := s.ClaimNext(ctx, w1, time.Minute)
	if err != nil || !ok1 {
		t.Fatalf("first claim: %v %v", ok1, err)
	}
	// A second worker cannot claim the same (or any other) Job here.
	_, ok2, err := s.ClaimNext(ctx, w2, time.Minute)
	if err != nil || ok2 {
		t.Fatalf("second claim must not succeed: %v %v", ok2, err)
	}
	if claim1.Attempt.LeaseEpoch != 1 || claim1.Job.Version != 2 || claim1.Job.State != aijobrelay.JobRunning {
		t.Fatalf("claim fencing wrong: %+v", claim1)
	}
	// A stale worker (old version/epoch, wrong worker) cannot commit.
	stale := aijobrelay.Commit{JobID: "j-c", JobVersion: 1, AttemptID: claim1.Attempt.AttemptID,
		WorkerID: "w9", LeaseID: claim1.Attempt.LeaseID, LeaseEpoch: 1,
		ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("result"), ReceiptID: "rc-stale"}
	if err := s.CommitAttempt(ctx, stale); err == nil {
		t.Fatal("a stale worker commit was accepted")
	}
	// A stale lease epoch cannot be renewed.
	if err := s.RenewLease(ctx, "j-c", claim1.Attempt.AttemptID, "w1", 99, time.Minute); err == nil {
		t.Fatal("a stale lease epoch was renewed")
	}
	// The current owner commits successfully: state+receipt+event atomically.
	good := aijobrelay.Commit{JobID: "j-c", JobVersion: claim1.Job.Version, AttemptID: claim1.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim1.Attempt.LeaseID, LeaseEpoch: claim1.Attempt.LeaseEpoch,
		ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("result"), ReceiptID: "rc-1", CorrelationID: "corr-j-c"}
	if err := s.CommitAttempt(ctx, good); err != nil {
		t.Fatalf("current owner commit: %v", err)
	}
	job, err := s.LoadJob(ctx, "j-c")
	if err != nil || job.State != aijobrelay.JobSucceeded || job.ReceiptID != "rc-1" {
		t.Fatalf("commit did not land: %+v %v", job, err)
	}
	// The receipt and the event exist (atomic commit evidence).
	var receiptCount, eventCount, pendingOutbox int
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_receipts WHERE receipt_id = 'rc-1'`).Scan(&receiptCount)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_id = 'evt-j-c-succeeded-2'`).Scan(&eventCount)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_outbox WHERE event_id = 'evt-j-c-succeeded-2' AND published = 0`).Scan(&pendingOutbox)
	if receiptCount != 1 || eventCount != 1 || pendingOutbox != 1 {
		t.Fatalf("receipt/event/outbox missing after the atomic commit: %d %d %d", receiptCount, eventCount, pendingOutbox)
	}
}

func TestUnknownSideEffectForcesNeedsReconciliation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture("j-u", "key-u", "payload")); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatal(err)
	}
	intent := aijobrelay.SideEffectIntent{
		IntentID: "int-1", JobID: "j-u", AttemptID: claim.Attempt.AttemptID,
		Operation: "provider.chat", IdempotencyKey: "op-key-1",
		InputDigest: inputDigest("prompt"), ProviderRef: "openai",
	}
	if err := s.PrepareSideEffect(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareSideEffect(ctx, intent); err == nil {
		t.Fatal("a duplicate intent was accepted")
	}
	// Dispatch, then the response is lost: the outcome is Unknown — never
	// a success.
	if err := s.ObserveSideEffect(ctx, "j-u", "int-1", claim.Job.Version, claim.Attempt.LeaseEpoch,
		claim.Attempt.AttemptID, aijobrelay.SideEffectDispatched, ""); err != nil {
		t.Fatalf("observe dispatched: %v", err)
	}
	if err := s.ObserveSideEffect(ctx, "j-u", "int-1", claim.Job.Version, claim.Attempt.LeaseEpoch,
		claim.Attempt.AttemptID, aijobrelay.SideEffectUnknown, ""); err != nil {
		t.Fatalf("observe unknown: %v", err)
	}
	intent2, err := s.LoadIntent(ctx, "int-1")
	if err != nil || intent2.State != aijobrelay.SideEffectUnknown {
		t.Fatalf("unknown outcome not preserved: %+v %v", intent2, err)
	}
	job, err := s.LoadJob(ctx, "j-u")
	if err != nil || job.State != aijobrelay.JobNeedsReconciliation {
		t.Fatalf("unknown effect did not force NeedsReconciliation: %+v %v", job, err)
	}
	// An unfenced observation (stale epoch) is refused.
	if err := s.ObserveSideEffect(ctx, "j-u", "int-1", claim.Job.Version, 99,
		claim.Attempt.AttemptID, aijobrelay.SideEffectFailedNoEff, ""); err == nil {
		t.Fatal("an unfenced observation was accepted")
	}
}

func TestLeaseExpiryRequeuesAndPauseIsHonest(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture("j-l", "key-l", "payload")); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, 30*time.Millisecond)
	if err != nil || !ok {
		t.Fatal(err)
	}
	// The lease is live: renewal works; the worker keeps the authority.
	if err := s.RenewLease(ctx, "j-l", claim.Attempt.AttemptID, "w1", claim.Attempt.LeaseEpoch, time.Minute); err != nil {
		t.Fatalf("live lease renewal: %v", err)
	}
	// Pause releases the lease; expiry recovery cannot resurrect it.
	if err := s.PauseJob(ctx, "j-l"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReleaseExpiredLeases(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	job, err := s.LoadJob(ctx, "j-l")
	if err != nil || job.State != aijobrelay.JobPaused {
		t.Fatalf("paused job was resurrected: %+v %v", job, err)
	}
	// Resume → claim with a short lease → let it expire → the job requeues
	// with a bumped fencing version.
	if err := s.ResumeJob(ctx, "j-l"); err != nil {
		t.Fatal(err)
	}
	claim2, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, 20*time.Millisecond)
	if err != nil || !ok {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	requeued, err := s.ReleaseExpiredLeases(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(requeued) != 1 || requeued[0].JobID != "j-l" || requeued[0].State != aijobrelay.JobQueued {
		t.Fatalf("expired lease did not requeue: %+v", requeued)
	}
	if requeued[0].Version != claim2.Job.Version+1 {
		t.Fatalf("requeue did not bump the fencing version: %+v", requeued[0])
	}
}

func TestSchedulerBoundedAndFenced(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := s.SubmitJob(ctx, submitFixture(
			"j-s"+string(rune('a'+i)), "key-s"+string(rune('a'+i)), "payload")); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	completed := 0
	handler := func(ctx context.Context, claim aijobrelay.Claim) (aijobrelay.Commit, error) {
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		completed++
		mu.Unlock()
		return aijobrelay.Commit{
			JobID: claim.Job.JobID, JobVersion: claim.Job.Version,
			AttemptID: claim.Attempt.AttemptID, WorkerID: claim.Attempt.WorkerID,
			LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
			ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
			ResultDigest: inputDigest("result"), ReceiptID: "rc-" + claim.Job.JobID,
			CorrelationID: claim.Job.CorrelationID,
		}, nil
	}
	sched := aijobrelay.NewScheduler(s, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, handler, 2, time.Minute)
	// One pass dispatches at most maxConcurrent Jobs (bounded goroutines).
	if _, err := sched.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if sched.Running() > 2 {
		t.Fatalf("scheduler exceeded its concurrency bound: %d", sched.Running())
	}
	// Drain the rest in further passes; Stop only after everything landed.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := sched.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if sched.Running() == 0 {
			var done int
			_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE state = ?`, aijobrelay.JobSucceeded).Scan(&done)
			if done == 5 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	sched.Stop()
	mu.Lock()
	c := completed
	mu.Unlock()
	if c < 5 {
		t.Fatalf("only %d of 5 jobs completed", c)
	}
	var succeeded int
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE state = ?`, aijobrelay.JobSucceeded).Scan(&succeeded)
	if succeeded != 5 {
		t.Fatalf("succeeded = %d, want 5", succeeded)
	}
}

func TestCapabilityMatching(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	job := submitFixture("j-cap", "key-cap", "payload")
	job.Requirements = aijobrelay.Requirements{ProviderID: "openai", MinMemoryMB: 4096}
	if _, err := s.SubmitJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	// A worker without the provider capability or the memory budget gets nothing.
	_, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-small", ProviderIDs: []string{"anthropic"}, MaxMemoryMB: 2048}, time.Minute)
	if err != nil || ok {
		t.Fatalf("an incapable worker claimed the job: %v %v", ok, err)
	}
	_, ok, err = s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-fit", ProviderIDs: []string{"openai"}, MaxMemoryMB: 8192}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("a capable worker could not claim: %v %v", ok, err)
	}
}
