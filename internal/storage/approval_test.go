package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The durable approval request store (the card-14 working candidate's
// implementation, IMPL-7): the Core-issued identity minted at the
// Waiting{approval} commit, the fenced decision commands, and the
// replay-never-double-applies semantics — driven through the real fenced
// commit path.

// approvalWaitingFixture submits, claims, and commits one Job to
// Waiting{reason: approval} through the real fenced commit path, returning
// the waiting Job.
func approvalWaitingFixture(t *testing.T, s *Store, id, key string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, submitFixture(id, key, "input "+id)); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: id, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobWaiting,
		WaitingReason:  string(aijobrelay.WaitApproval),
		AttemptOutcome: aijobrelay.AttemptFailedRetryable,
		CorrelationID:  "corr-" + id,
	}); err != nil {
		t.Fatalf("approval-waiting commit: %v", err)
	}
	job, err := s.LoadJob(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return job
}

func decisionFor(jobID, approvalID, decision string) aijobrelay.ApprovalDecision {
	return aijobrelay.ApprovalDecision{
		JobID: jobID, ApprovalID: approvalID, Decision: decision,
		Actor: "local-user:test", CorrelationID: "corr-dec-" + approvalID,
	}
}

// TestApprovalMintedOnApprovalWaitingCommit pins the identity issuance
// (requirement (a)): a Job that durably enters Waiting{approval} carries a
// Core-issued pending approval bound to it; a non-approval Waiting commit
// mints none; a second raised request on the same Job mints a NEW identity.
func TestApprovalMintedOnApprovalWaitingCommit(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	job := approvalWaitingFixture(t, s, "j-appr-1", "key-appr-1")
	approval, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("the Waiting{approval} Job must carry a raised approval: %v", err)
	}
	if approval.State != aijobrelay.ApprovalPending {
		t.Fatalf("the raised approval must be pending, got %q", approval.State)
	}
	if approval.JobID != job.JobID {
		t.Fatalf("the approval must be bound to its Job, got %q", approval.JobID)
	}
	if !strings.HasPrefix(approval.ApprovalID, "appr-") {
		t.Fatalf("the approval id must carry the Core-issued grammar, got %q", approval.ApprovalID)
	}
	loaded, err := s.LoadApproval(ctx, approval.ApprovalID)
	if err != nil || loaded.ApprovalID != approval.ApprovalID {
		t.Fatalf("the identity must be queryable by id: %v", err)
	}
	if _, err := s.LoadApproval(ctx, "appr-nonexistent"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("an unknown approval id must be the typed NOT_FOUND: %v", err)
	}
	if _, err := s.LoadLatestApprovalForJob(ctx, "j-never-submitted"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("a Job with no approval must be the typed NOT_FOUND: %v", err)
	}

	// A non-approval Waiting commit mints nothing.
	if _, err := s.SubmitJob(ctx, submitFixture("j-appr-2", "key-appr-2", "input 2")); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: "j-appr-2", JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobWaiting,
		WaitingReason:  string(aijobrelay.WaitBackoffScheduled),
		AttemptOutcome: aijobrelay.AttemptFailedRetryable,
	}); err != nil {
		t.Fatalf("backoff-waiting commit: %v", err)
	}
	if _, err := s.LoadLatestApprovalForJob(ctx, "j-appr-2"); err == nil {
		t.Fatal("a non-approval Waiting commit must mint no approval")
	}

	// A second raised request on the SAME Job mints a new identity; the
	// first stays decided (a decision never re-opens one).
	if _, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept)); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	secondWaiting := approvalWaitingFixtureForJob(t, s, job.JobID)
	second, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("the second raised request must be queryable: %v", err)
	}
	if second.ApprovalID == approval.ApprovalID {
		t.Fatal("a second raised request must carry a NEW Core-issued identity")
	}
	if second.State != aijobrelay.ApprovalPending {
		t.Fatalf("the second raised request must be pending, got %q", second.State)
	}
	_ = secondWaiting
}

// approvalWaitingFixtureForJob re-runs an already-submitted Job through
// claim + Waiting{approval} commit (the second raised request's path).
func approvalWaitingFixtureForJob(t *testing.T, s *Store, jobID string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if !ok || claim.Job.JobID != jobID {
		t.Fatalf("expected to claim %s, got ok=%v job=%+v", jobID, ok, claim.Job)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: jobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobWaiting,
		WaitingReason:  string(aijobrelay.WaitApproval),
		AttemptOutcome: aijobrelay.AttemptFailedRetryable,
	}); err != nil {
		t.Fatalf("second approval-waiting commit: %v", err)
	}
	return claim.Job
}

// TestDecideApprovalAccept pins the accepted decision end to end: the
// approval resolves to accepted, the Job re-queues (Waiting → Queued, the
// waiting reason cleared), the version fencing advanced, and the event
// journal carries the decision.
func TestDecideApprovalAccept(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	job := approvalWaitingFixture(t, s, "j-appr-acc", "key-appr-acc")
	approval, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	res, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if res.Replayed {
		t.Fatal("a fresh decision must not report replayed")
	}
	if res.Approval.State != aijobrelay.ApprovalAccepted || res.Approval.Decision != aijobrelay.DecisionAccept {
		t.Fatalf("the approval must record accepted/accept, got %q/%q", res.Approval.State, res.Approval.Decision)
	}
	if res.Approval.DecidedBy != "local-user:test" || res.Approval.DecidedAt.IsZero() {
		t.Fatalf("the approval must record the actor and the decision instant, got %q/%v", res.Approval.DecidedBy, res.Approval.DecidedAt)
	}
	if res.Approval.CorrelationID != "corr-dec-"+approval.ApprovalID {
		t.Fatalf("the decision's correlation must ride the approval record, got %q", res.Approval.CorrelationID)
	}
	if res.Job.State != aijobrelay.JobQueued || res.Job.WaitingReason != "" {
		t.Fatalf("the accepted decision must re-queue the Job with the reason cleared, got %q/%q", res.Job.State, res.Job.WaitingReason)
	}
	if res.Job.Version != job.Version+1 {
		t.Fatalf("the Job version must advance (the coded fencing), got %d want %d", res.Job.Version, job.Version+1)
	}
	events, err := s.JobEvents(ctx, job.JobID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "job.queued" && strings.Contains(string(e.InlinePayload), approval.ApprovalID) {
			found = true
		}
	}
	if !found {
		t.Fatal("the re-queue event must carry the approval identity")
	}
}

// TestDecideApprovalReject pins the rejected decision end to end: the
// approval resolves to rejected and the Job takes its terminal disposition
// (Waiting → Failed — the register-consistent extension), with the decision
// event in the journal.
func TestDecideApprovalReject(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	job := approvalWaitingFixture(t, s, "j-appr-rej", "key-appr-rej")
	approval, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	res, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionReject))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if res.Approval.State != aijobrelay.ApprovalRejected || res.Approval.Decision != aijobrelay.DecisionReject {
		t.Fatalf("the approval must record rejected/reject, got %q/%q", res.Approval.State, res.Approval.Decision)
	}
	if res.Job.State != aijobrelay.JobFailed {
		t.Fatalf("the rejected decision must end the Job Failed (the extension), got %q", res.Job.State)
	}
	if !aijobrelay.TerminalJob(res.Job.State) {
		t.Fatal("the rejected disposition must be terminal")
	}
	events, err := s.JobEvents(ctx, job.JobID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "job.failed" && strings.Contains(string(e.InlinePayload), approval.ApprovalID) {
			found = true
		}
	}
	if !found {
		t.Fatal("the terminal event must carry the approval identity")
	}
}

// TestDecideApprovalReplayNeverDoubleApplies pins requirement (b): the same
// decision re-presented returns the recorded outcome with NO state change,
// no second event, and Replayed=true; a contradicting decision is the typed
// CONFLICT_STATE; a replay after the Job moved on refuses.
func TestDecideApprovalReplayNeverDoubleApplies(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	job := approvalWaitingFixture(t, s, "j-appr-rep", "key-appr-rep")
	approval, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	first, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	// The same decision re-presented: the recorded outcome, nothing applied.
	replay, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil {
		t.Fatalf("same-decision replay must return the recorded outcome: %v", err)
	}
	if !replay.Replayed {
		t.Fatal("the replay must report replayed")
	}
	if replay.Approval.State != first.Approval.State || replay.Job.Version != first.Job.Version || replay.Job.State != first.Job.State {
		t.Fatal("the replay must not change any state")
	}
	// A contradicting decision is the coded CONFLICT_STATE, never a
	// re-decision.
	if _, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionReject)); err == nil || !strings.Contains(err.Error(), string(errs.CodeConflictState)) {
		t.Fatalf("a contradicting decision must be the typed CONFLICT_STATE: %v", err)
	}
	reloaded, err := s.LoadApproval(ctx, approval.ApprovalID)
	if err != nil || reloaded.State != aijobrelay.ApprovalAccepted {
		t.Fatalf("the contradicting refusal must leave the recorded decision intact: %v", err)
	}
	// The Job's journal carries exactly ONE decision event (no double-apply).
	events, err := s.JobEvents(ctx, job.JobID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	decisionEvents := 0
	for _, e := range events {
		if strings.Contains(string(e.InlinePayload), approval.ApprovalID) {
			decisionEvents++
		}
	}
	if decisionEvents != 1 {
		t.Fatalf("exactly one decision event must exist, got %d", decisionEvents)
	}

	// A replayed decision whose Job moved on (queued → paused) still returns
	// the recorded outcome: the approval record is the replay's authority,
	// not the Job state.
	pausedJob := first.Job
	if err := s.PauseJob(ctx, pausedJob.JobID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	replay2, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil || !replay2.Replayed {
		t.Fatalf("the replay must return the recorded outcome regardless of the Job state: %v %v", replay2.Replayed, err)
	}
}

// TestDecideApprovalRefusals pins the boundary refusals: an unknown or
// foreign approval id is the typed NOT_FOUND (indistinguishable — the
// no-existence-leak shape), a wrong-state Job is the coded CONFLICT_STATE,
// and an unregistered decision value is the typed VALIDATION_FAILED.
func TestDecideApprovalRefusals(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Unknown approval id.
	if _, err := s.DecideApproval(ctx, decisionFor("j-unknown-scope", "appr-unknown", aijobrelay.DecisionAccept)); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("an unknown approval id must be the typed NOT_FOUND: %v", err)
	}

	// A wrong-state Job: the approval is pending but the Job is queued
	// (cancel_requested/queued paths), so a fresh decision refuses with the
	// coded CONFLICT_STATE and leaves the approval pending.
	job := approvalWaitingFixture(t, s, "j-appr-wrong", "key-appr-wrong")
	approval, err := s.LoadLatestApprovalForJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	// Move the Job out of Waiting through the coded pause exit.
	if err := s.PauseJob(ctx, job.JobID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := s.DecideApproval(ctx, decisionFor(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept)); err == nil || !strings.Contains(err.Error(), string(errs.CodeConflictState)) {
		t.Fatalf("a wrong-state Job decision must be the typed CONFLICT_STATE: %v", err)
	}
	pending, err := s.LoadApproval(ctx, approval.ApprovalID)
	if err != nil || pending.State != aijobrelay.ApprovalPending {
		t.Fatalf("the refused decision must leave the approval pending: %v", err)
	}

	// Unregistered decision values.
	job2 := approvalWaitingFixture(t, s, "j-appr-val", "key-appr-val")
	approval2, err := s.LoadLatestApprovalForJob(ctx, job2.JobID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	for _, d := range []string{"", "approve", "deny", "Accept"} {
		bad := decisionFor(job2.JobID, approval2.ApprovalID, d)
		if _, err := s.DecideApproval(ctx, bad); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
			t.Fatalf("decision %q must be the typed VALIDATION_FAILED: %v", d, err)
		}
	}
	if _, err := s.DecideApproval(ctx, aijobrelay.ApprovalDecision{Decision: aijobrelay.DecisionAccept, Actor: "local-user:test"}); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("a decision without its scope and approval id must be the typed VALIDATION_FAILED: %v", err)
	}

	// A foreign approval: an approval bound to another Job presented to this
	// Job's decision is the SAME typed NOT_FOUND shape as an unknown id
	// (the scope-bound law; the no-existence-leak rule — an error never
	// confirms an inaccessible object).
	job3 := approvalWaitingFixture(t, s, "j-appr-foreign", "key-appr-foreign")
	_, ferr := s.DecideApproval(ctx, aijobrelay.ApprovalDecision{
		JobID:      job3.JobID,           // the presented scope
		ApprovalID: approval2.ApprovalID, // bound to j-appr-val
		Decision:   aijobrelay.DecisionAccept,
		Actor:      "local-user:test",
	})
	if ferr == nil || !strings.Contains(ferr.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("a foreign approval binding must be the same NOT_FOUND shape as unknown: %v", ferr)
	}
	if !strings.Contains(ferr.Error(), "is not stored") {
		t.Fatalf("the foreign refusal must be indistinguishable from an unknown identity: %v", ferr)
	}
	// The foreign refusal changed nothing anywhere.
	intact, err := s.LoadApproval(ctx, approval2.ApprovalID)
	if err != nil || intact.State != aijobrelay.ApprovalPending {
		t.Fatalf("the foreign refusal must leave the bound approval intact: %v", err)
	}
	waiting3, err := s.LoadJob(ctx, job3.JobID)
	if err != nil || waiting3.State != aijobrelay.JobWaiting {
		t.Fatalf("the foreign refusal must leave the presented Job waiting: %v", err)
	}
}
