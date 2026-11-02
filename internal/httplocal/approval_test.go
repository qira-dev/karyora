package httplocal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
)

// The approval command surface over the wire (the card-14 working
// candidate, IMPL-7): the adapter's own non-canonical paths, the typed
// refusals, and the boundary posture — the same patterns every other
// endpoint carries.

// approvalWaitingOverStore drives one submitted Job to Waiting{reason:
// approval} through the fixture store (the real fenced commit path),
// returning the Job and its raised approval.
func approvalWaitingOverStore(t *testing.T, env *testEnv, id, key string) (aijobrelay.Job, aijobrelay.Approval) {
	t.Helper()
	ctx := context.Background()
	if _, err := env.deps.Relay.SubmitJob(ctx, aijobrelay.Job{
		JobID: id, IdempotencyKey: key, InputDigest: app.DigestOf([]byte("input " + id)),
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
		t.Fatalf("the raised approval: %v", err)
	}
	job, err := env.deps.Relay.LoadJob(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return job, approval
}

// TestApprovalEndpoints covers the wire surface: the read returns the
// raised identity inside the approval wrapper; the accept/reject commands
// return the Core-confirmed post-decision state; the status read confirms
// the transition independently.
func TestApprovalEndpoints(t *testing.T) {
	env := newTestEnv(t)
	acceptJob, acceptApproval := approvalWaitingOverStore(t, env, "j-wire-acc", "key-wire-acc")
	rejectJob, rejectApproval := approvalWaitingOverStore(t, env, "j-wire-rej", "key-wire-rej")

	// The approval read (the queryable identity).
	status, body := env.do(t, http.MethodGet, "/v1/jobs/"+acceptJob.JobID+"/approval", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("approval read = %d (%s)", status, body)
	}
	var read struct {
		Approval aijobrelay.Approval `json:"approval"`
	}
	if err := json.Unmarshal(body, &read); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if read.Approval.ApprovalID != acceptApproval.ApprovalID || read.Approval.State != aijobrelay.ApprovalPending || read.Approval.JobID != acceptJob.JobID {
		t.Fatalf("the read must return the raised identity, got %+v", read.Approval)
	}

	// The accept command: the Core-confirmed result on the wire.
	decBody := `{"approval_id":"` + acceptApproval.ApprovalID + `"}`
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+acceptJob.JobID+"/approvals/accept", decBody, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("accept = %d (%s)", status, body)
	}
	var decided struct {
		Approval aijobrelay.Approval `json:"approval"`
		Job      aijobrelay.Job      `json:"job"`
		Replayed bool                `json:"replayed"`
	}
	if err := json.Unmarshal(body, &decided); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if decided.Approval.State != aijobrelay.ApprovalAccepted || decided.Job.State != aijobrelay.JobQueued || decided.Replayed {
		t.Fatalf("the confirmed accept must be on the wire, got %+v", decided)
	}
	// The replayed flag rides the wire honestly.
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+acceptJob.JobID+"/approvals/accept", decBody, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("replay = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !decided.Replayed || decided.Approval.State != aijobrelay.ApprovalAccepted {
		t.Fatalf("the replay must report the recorded outcome, got %+v", decided)
	}

	// The reject command: the terminal disposition confirmed on the wire.
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+rejectJob.JobID+"/approvals/reject", `{"approval_id":"`+rejectApproval.ApprovalID+`"}`, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("reject = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decided.Approval.State != aijobrelay.ApprovalRejected || decided.Job.State != aijobrelay.JobFailed {
		t.Fatalf("the confirmed reject must be on the wire, got %+v", decided)
	}

	// The authoritative status read confirms the transitions independently
	// (the never-from-the-ack-alone discipline rendered server-side).
	status, body = env.do(t, http.MethodGet, "/v1/jobs/"+rejectJob.JobID, "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var st struct {
		Job aijobrelay.Job `json:"job"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Job.State != aijobrelay.JobFailed {
		t.Fatalf("the authoritative state must carry the terminal disposition, got %q", st.Job.State)
	}
}

// TestApprovalRefusals pins the typed refusal paths on the wire: the
// unauthenticated read and decision (403 AUTHZ_DENIED through the decision
// port), the wrong-state decision (409 CONFLICT_STATE), the unknown and
// foreign approval ids (404 NOT_FOUND — indistinguishable), the malformed
// body (400 VALIDATION_FAILED), and the mutating-class boundary posture.
func TestApprovalRefusals(t *testing.T) {
	env := newTestEnv(t)
	waitingJob, approval := approvalWaitingOverStore(t, env, "j-wire-ref", "key-wire-ref")

	// The unauthenticated read.
	status, body := env.do(t, http.MethodGet, "/v1/jobs/"+waitingJob.JobID+"/approval", "", "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("unauthenticated read = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "AUTHZ_DENIED" {
		t.Fatalf("the unauthenticated read must be the typed AUTHZ_DENIED, got %s", env.Code)
	}

	// The unauthenticated decision.
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+waitingJob.JobID+"/approvals/accept", `{"approval_id":"`+approval.ApprovalID+`"}`, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("unauthenticated decision = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "AUTHZ_DENIED" {
		t.Fatalf("the unauthenticated decision must be the typed AUTHZ_DENIED, got %s", env.Code)
	}

	// The wrong-state decision: the Job is paused out of Waiting through the
	// coded exit; a fresh decision refuses with the coded CONFLICT_STATE.
	if err := env.deps.Relay.PauseJob(context.Background(), waitingJob.JobID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+waitingJob.JobID+"/approvals/accept", `{"approval_id":"`+approval.ApprovalID+`"}`, env.token, nil)
	if status != http.StatusConflict {
		t.Fatalf("wrong-state decision = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "CONFLICT_STATE" {
		t.Fatalf("the wrong-state decision must be the typed CONFLICT_STATE, got %s", env.Code)
	}

	// The unknown approval id.
	otherJob, otherApproval := approvalWaitingOverStore(t, env, "j-wire-ref2", "key-wire-ref2")
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+otherJob.JobID+"/approvals/accept", `{"approval_id":"appr-unknown"}`, env.token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown approval id = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "NOT_FOUND" {
		t.Fatalf("the unknown approval id must be the typed NOT_FOUND, got %s", env.Code)
	}
	// The foreign approval id: the same NOT_FOUND shape (the
	// no-existence-leak rule — indistinguishable from unknown).
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+otherJob.JobID+"/approvals/accept", `{"approval_id":"`+approval.ApprovalID+`"}`, env.token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("foreign approval id = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "NOT_FOUND" || !strings.Contains(env.Data["reason"].(string), "is not stored") {
		t.Fatalf("the foreign approval id must be indistinguishable from unknown, got %s (%v)", env.Code, env.Data)
	}

	// The malformed body.
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+otherJob.JobID+"/approvals/accept", `{"approval_id":`, env.token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed body = %d (%s)", status, body)
	}
	if env := decodeEnvelope(t, body); env.Code != "VALIDATION_FAILED" {
		t.Fatalf("the malformed body must be the typed VALIDATION_FAILED, got %s", env.Code)
	}

	// The mutating-class boundary posture: a browser-shaped POST without a
	// session is refused at the CSRF boundary (the same posture every
	// mutation carries).
	resp := env.raw(http.MethodPost, "/v1/jobs/"+otherJob.JobID+"/approvals/accept", `{"approval_id":"x"}`, func(r *http.Request) {
		r.Header.Set("Origin", "http://127.0.0.1:8080")
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("browser-shaped POST = %d", resp.Code)
	}
	if env := decodeEnvelope(t, resp.Body.Bytes()); env.Code != "AUTHZ_DENIED" {
		t.Fatalf("the boundary refusal must be the typed envelope, got %s", env.Code)
	}

	// The remaining approvals stay pending after every refusal.
	for id, want := range map[string]aijobrelay.Approval{approval.ApprovalID: approval, otherApproval.ApprovalID: otherApproval} {
		got, err := env.deps.Relay.LoadApproval(context.Background(), id)
		if err != nil || got.State != aijobrelay.ApprovalPending {
			t.Fatalf("the refusals must leave %s pending: %v", want.ApprovalID, err)
		}
	}
}
