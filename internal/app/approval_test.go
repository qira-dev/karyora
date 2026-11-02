package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The approval accept/reject commands (the card-14 working candidate,
// IMPL-7): each of the six recorded requirements pinned at the API surface
// — the scope-bound decision port, the audited fail-closed path, the
// Core-confirmed transitions, the replay-never-double-applies semantics,
// and the pure-read approval query.

// approvalWaitingJob drives one submitted Job to Waiting{reason: approval}
// through the same relay the slice consumes (the real fenced commit path),
// returning the Job and its raised approval.
func approvalWaitingJob(t *testing.T, env *testEnv, id, key string) (aijobrelay.Job, aijobrelay.Approval) {
	t.Helper()
	ctx := context.Background()
	if _, err := env.deps.Relay.SubmitJob(ctx, aijobrelay.Job{
		JobID: id, IdempotencyKey: key, InputDigest: DigestOf([]byte("input " + id)),
		State: aijobrelay.JobCreated, Version: 1, CorrelationID: "corr-" + id,
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claim, ok, err := env.deps.Relay.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w1", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := env.deps.Relay.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: id, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w1", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobWaiting,
		WaitingReason:  string(aijobrelay.WaitApproval),
		AttemptOutcome: aijobrelay.AttemptFailedRetryable,
	}); err != nil {
		t.Fatalf("approval-waiting commit: %v", err)
	}
	approval, err := env.deps.Relay.LoadLatestApprovalForJob(ctx, id)
	if err != nil {
		t.Fatalf("the Waiting{approval} Job must carry its raised approval: %v", err)
	}
	job, err := env.deps.Relay.LoadJob(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return job, approval
}

func decideRequest(jobID, approvalID, decision string) ApprovalDecisionRequest {
	return ApprovalDecisionRequest{JobID: jobID, ApprovalID: approvalID, Decision: decision}
}

// awaitSettlement closes the gap between the bounded driver loop's return
// and the durable settlement (the routine repair the IMPL-7 verification
// surfaced: under the race detector's CI load, RunToSettlement's tick
// budget can expire while the settlement commit is still in flight — the
// loop is a driver, not the settlement's clock). The poll is the system's
// own reconciliation-by-read through the authoritative store, bounded, and
// it WEAKENS nothing: the caller's assertion still requires the terminal
// state it asserts.
func awaitSettlement(t *testing.T, env *testEnv, jobID string, driven <-chan aijobrelay.Job) aijobrelay.Job {
	t.Helper()
	final := <-driven
	if aijobrelay.TerminalJob(final.State) {
		return final
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err := env.deps.Relay.LoadJob(context.Background(), jobID)
		if err != nil {
			t.Fatalf("settlement reload: %v", err)
		}
		if aijobrelay.TerminalJob(job.State) {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	return final
}

// TestApprovalDecisionScopeBound pins requirement (3): the decision port
// authorizes the write on the Job the approval gates — an unauthenticated
// command is the typed AUTHZ_DENIED envelope and nothing changes.
func TestApprovalDecisionScopeBound(t *testing.T) {
	env := newTestEnv(t)
	job, approval := approvalWaitingJob(t, env, "j-appr-auth", "key-appr-auth")

	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	_, err := env.slice.DecideApproval(denied, decideRequest(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	var ee *EnvelopeError
	if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeAuthzDenied {
		t.Fatalf("expected the typed AUTHZ_DENIED envelope, got %v", err)
	}
	waiting, rerr := env.deps.Relay.LoadJob(context.Background(), job.JobID)
	if rerr != nil || waiting.State != aijobrelay.JobWaiting || waiting.Version != job.Version {
		t.Fatalf("the refused command must change nothing: %v", rerr)
	}

	res, err := env.slice.DecideApproval(env.ctx(), decideRequest(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil {
		t.Fatalf("a data-scoped session must decide: %v", err)
	}
	if res.Job.State != aijobrelay.JobQueued || res.Approval.State != aijobrelay.ApprovalAccepted {
		t.Fatalf("the confirmed transition must be recorded, got job=%s approval=%s", res.Job.State, res.Approval.State)
	}
}

// TestApprovalDecisionValidation pins the request-shape law: typed
// VALIDATION_FAILED envelopes for an unregistered decision value and the
// missing identity fields — never silent coercion.
func TestApprovalDecisionValidation(t *testing.T) {
	env := newTestEnv(t)
	job, approval := approvalWaitingJob(t, env, "j-appr-val", "key-appr-val")
	for name, req := range map[string]ApprovalDecisionRequest{
		"empty_job":        {ApprovalID: approval.ApprovalID, Decision: aijobrelay.DecisionAccept},
		"empty_approval":   {JobID: job.JobID, Decision: aijobrelay.DecisionAccept},
		"bad_decision":     {JobID: job.JobID, ApprovalID: approval.ApprovalID, Decision: "approve"},
		"empty_decision":   {JobID: job.JobID, ApprovalID: approval.ApprovalID, Decision: ""},
		"case_variant":     {JobID: job.JobID, ApprovalID: approval.ApprovalID, Decision: "Accept"},
		"unknown_approval": {JobID: job.JobID, ApprovalID: "appr-unknown", Decision: aijobrelay.DecisionAccept},
	} {
		if _, err := env.slice.DecideApproval(env.ctx(), req); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	// The refusals changed nothing: the approval stays pending, the Job waits.
	still, err := env.deps.Relay.LoadJob(context.Background(), job.JobID)
	if err != nil || still.State != aijobrelay.JobWaiting || still.Version != job.Version {
		t.Fatalf("the refused commands must change nothing: %v", err)
	}
	pending, err := env.deps.Relay.LoadApproval(context.Background(), approval.ApprovalID)
	if err != nil || pending.State != aijobrelay.ApprovalPending {
		t.Fatalf("the approval must stay pending after refusals: %v", err)
	}
}

// TestApprovalDecisionAudited pins requirement (5): the decision is an
// audit event of row 10's model (actor, action, decision, outcome) recorded
// through the fail-closed audit path.
func TestApprovalDecisionAudited(t *testing.T) {
	env := newTestEnv(t)
	job, approval := approvalWaitingJob(t, env, "j-appr-audit", "key-appr-audit")

	sink := &memoryAuditStore{}
	env.deps.Guard.Audit = security.NewAuditService(sink, 64, 1000)
	if _, err := env.slice.DecideApproval(env.ctx(), decideRequest(job.JobID, approval.ApprovalID, aijobrelay.DecisionReject)); err != nil {
		t.Fatalf("decide: %v", err)
	}
	records, err := sink.Records()
	if err != nil {
		t.Fatalf("audit records: %v", err)
	}
	found := false
	for _, rec := range records {
		var ar security.AuditRecord
		if err := json.Unmarshal(rec.Payload, &ar); err != nil {
			t.Fatalf("audit payload: %v", err)
		}
		if ar.Action != "approval.decide" {
			continue
		}
		found = true
		if ar.Outcome != aijobrelay.DecisionReject {
			t.Fatalf("the audit record must carry the decision, got outcome %q", ar.Outcome)
		}
		if !strings.Contains(ar.Actor, "local-user:") {
			t.Fatalf("the audit record must carry the actor, got %q", ar.Actor)
		}
		if ar.JobID != job.JobID {
			t.Fatalf("the audit record must carry the job id, got %q", ar.JobID)
		}
		if ar.Scope != "job:"+job.JobID || ar.PolicyDecision != "allow" || ar.PolicyReference != security.ReasonOK {
			t.Fatalf("the audit record must carry the authorization reference, got scope=%q decision=%q ref=%q", ar.Scope, ar.PolicyDecision, ar.PolicyReference)
		}
		if ar.CorrelationID == "" {
			t.Fatal("the audit record must carry the command's correlation id")
		}
	}
	if !found {
		t.Fatalf("the decision must be an audit event among %d records", len(records))
	}
}

// unreachableAuditStore is an audit store whose tail is unreachable: the
// reserved-quota buffer absorbs the first record, and the next record
// denies (the C2-15 §5 fail-closed rule exercised end to end).
type unreachableAuditStore struct{}

func (unreachableAuditStore) Tail() (security.TailState, error) {
	return security.TailState{}, errors.New("scripted audit outage")
}

func (unreachableAuditStore) Append(security.ChainedRecord) error {
	return errors.New("scripted audit outage")
}
func (unreachableAuditStore) Records() ([]security.ChainedRecord, error) {
	return nil, errors.New("scripted audit outage")
}
func (unreachableAuditStore) Anchor(security.Anchor) error {
	return errors.New("scripted audit outage")
}
func (unreachableAuditStore) Anchors() ([]security.Anchor, error) {
	return nil, errors.New("scripted audit outage")
}
func (unreachableAuditStore) SetIntegrityBroken(bool) error  { return nil }
func (unreachableAuditStore) IntegrityBroken() (bool, error) { return false, nil }

// TestApprovalDecisionAuditFailClosed pins the audit-on-critical-path rule:
// a decision whose audit record cannot be written is DENIED — the effect is
// never executed unrecorded, and the Job stays waiting.
func TestApprovalDecisionAuditFailClosed(t *testing.T) {
	env := newTestEnv(t)
	job, approval := approvalWaitingJob(t, env, "j-appr-fc", "key-appr-fc")

	// Quota 1 over the unreachable store: the command's own authorization
	// record absorbs the quota, so the decision record cannot be written.
	env.deps.Guard.Audit = security.NewAuditService(unreachableAuditStore{}, 1, 1000)
	_, err := env.slice.DecideApproval(env.ctx(), decideRequest(job.JobID, approval.ApprovalID, aijobrelay.DecisionAccept))
	if err == nil {
		t.Fatal("a decision whose audit record cannot be written must be denied")
	}
	var unavailable *security.ErrAuditUnavailable
	if !errors.As(err, &unavailable) {
		t.Fatalf("the denial must be the typed audit-unavailable failure, got %v", err)
	}
	waiting, rerr := env.deps.Relay.LoadJob(context.Background(), job.JobID)
	if rerr != nil || waiting.State != aijobrelay.JobWaiting || waiting.Version != job.Version {
		t.Fatalf("the denied command must change nothing: %v", rerr)
	}
	pending, aerr := env.deps.Relay.LoadApproval(context.Background(), approval.ApprovalID)
	if aerr != nil || pending.State != aijobrelay.ApprovalPending {
		t.Fatalf("the denied command must leave the approval pending: %v", aerr)
	}
}

// TestApprovalDecisionConfirmedTransitionsAndReplay pins requirements
// (1)/(2)/(4) at the API surface: the Core-confirmed transitions (accept →
// Queued; reject → the register-consistent Failed), the honest replayed
// flag, and the query-by-identity reconciliation — never a blind
// re-decision.
func TestApprovalDecisionConfirmedTransitionsAndReplay(t *testing.T) {
	env := newTestEnv(t)

	// Both Jobs are driven to Waiting{approval} BEFORE any decision: the
	// FIFO claim would otherwise hand the re-queued accepted Job to the
	// second fixture (the accepted decision's re-queue is real state).
	acceptJob, acceptApproval := approvalWaitingJob(t, env, "j-appr-tr1", "key-appr-tr1")
	rejectJob, rejectApproval := approvalWaitingJob(t, env, "j-appr-tr2", "key-appr-tr2")
	accepted, err := env.slice.DecideApproval(env.ctx(), decideRequest(acceptJob.JobID, acceptApproval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Job.State != aijobrelay.JobQueued || accepted.Approval.State != aijobrelay.ApprovalAccepted || accepted.Replayed {
		t.Fatalf("the accepted transition must be Core-confirmed, got %+v", accepted)
	}
	// The same decision re-presented: the recorded outcome, replayed=true.
	replay, err := env.slice.DecideApproval(env.ctx(), decideRequest(acceptJob.JobID, acceptApproval.ApprovalID, aijobrelay.DecisionAccept))
	if err != nil || !replay.Replayed {
		t.Fatalf("the replay must return the recorded outcome: %v %v", replay.Replayed, err)
	}
	if replay.Job.Version != accepted.Job.Version {
		t.Fatal("the replay must not advance the Job")
	}
	// A contradicting decision is the typed CONFLICT_STATE.
	if _, err := env.slice.DecideApproval(env.ctx(), decideRequest(acceptJob.JobID, acceptApproval.ApprovalID, aijobrelay.DecisionReject)); err == nil || !strings.Contains(err.Error(), string(errs.CodeConflictState)) {
		t.Fatalf("the contradicting decision must be the typed CONFLICT_STATE: %v", err)
	}

	rejected, err := env.slice.DecideApproval(env.ctx(), decideRequest(rejectJob.JobID, rejectApproval.ApprovalID, aijobrelay.DecisionReject))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Job.State != aijobrelay.JobFailed || rejected.Approval.State != aijobrelay.ApprovalRejected {
		t.Fatalf("the rejected transition must be Core-confirmed (the terminal disposition), got job=%s approval=%s", rejected.Job.State, rejected.Approval.State)
	}
	if !aijobrelay.TerminalJob(rejected.Job.State) {
		t.Fatal("the rejected disposition must be terminal")
	}
}

// TestJobApprovalRead pins the queryable identity read: the read is
// authorized like every read, returns the latest raised approval, creates
// nothing, and mutates nothing — a notification read/dismiss never implies
// approval (requirement (6)).
func TestJobApprovalRead(t *testing.T) {
	env := newTestEnv(t)
	job, approval := approvalWaitingJob(t, env, "j-appr-read", "key-appr-read")

	read, err := env.slice.JobApproval(env.ctx(), job.JobID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.ApprovalID != approval.ApprovalID || read.State != aijobrelay.ApprovalPending || read.JobID != job.JobID {
		t.Fatalf("the read must return the raised identity, got %+v", read)
	}
	// The read created nothing: no second approval row exists.
	again, err := env.slice.JobApproval(env.ctx(), job.JobID)
	if err != nil || again.ApprovalID != approval.ApprovalID {
		t.Fatalf("the read must be stable: %v", err)
	}
	// An unauthenticated read is the typed AUTHZ_DENIED envelope.
	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	if _, err := env.slice.JobApproval(denied, job.JobID); !errors.As(err, new(*EnvelopeError)) {
		t.Fatalf("the denied read must be the typed envelope, got %v", err)
	}
	// A Job with no approval read is the typed NOT_FOUND.
	if _, err := env.slice.Submit(env.ctx(), baseRequest()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := env.slice.JobApproval(env.ctx(), "j-never"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("a Job with no approval must be the typed NOT_FOUND: %v", err)
	}
}
