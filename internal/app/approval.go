package app

import (
	"context"
	"fmt"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The approval accept/reject commands (the owner's card-14 working
// candidate of 2026-11-02 — the P3.2 §3.8 six-requirement set, implemented
// in IMPL-7; the adoption covers P-S1-06). Every requirement rides a coded
// surface, and each is recorded in the IMPL-7 record with its code source:
//
//	(1) Core-issued stable queryable approval id — the durable approval
//	    object the Core mints when a Job enters Waiting{reason: approval}
//	    (internal/storage mint hook; queryable by id and by job).
//	(2) correlation/idempotency per the coded command conventions — the
//	    decision carries its correlation id and applies under the fenced
//	    single-writer path; a replayed decision never double-applies (the
//	    same decision returns the recorded outcome; a contradicting decision
//	    is the typed CONFLICT_STATE). The decision is an Open-class operation
//	    per P1.6 §4: a lost response is reconciled by QUERYING the approval
//	    identity (the read below) — never a blind re-decision, never an
//	    auto-retry.
//	(3) scope-bound — the one decision port authorizes the write on the Job
//	    the approval gates (Resource{Type: ResourceJob}, ActionDataWrite, the
//	    code's own register for every Job write); the durable decision acts
//	    on exactly the presented approval identity bound to its own Job.
//	(4) Core-confirmed transition — the result is the durable post-decision
//	    approval + Job (accept → Waiting → Queued; reject → the
//	    register-consistent Waiting → Failed terminal disposition), returned
//	    only after the transaction commits.
//	(5) audited — the decision is an audit event of row 10's model
//	    (actor, action, decision, outcome) recorded on the fail-closed audit
//	    path BEFORE the effect (C2-15 §5): an effect whose audit record
//	    cannot be written is denied, not executed unrecorded. The audit chain
//	    records the security decision; the approval record and the Job event
//	    journal carry the Core-confirmed business outcome.
//	(6) notification-read ≠ approval — nothing here lets a read imply a
//	    decision: the reads below mutate nothing, no notification surface
//	    exists (the recorded card-19 deferral), and the only path that
//	    changes an approval is the explicit decision command.

// ApprovalDecisionRequest is one approval decision command over the typed
// API surface: the Job the approval gates, the Core-issued approval
// identity, and the decision (the closed accept/reject register).
type ApprovalDecisionRequest struct {
	JobID      string
	ApprovalID string
	Decision   string
}

// JobApproval reads one Job's latest raised approval request
// (ActionDataRead through the same decision port every read decides
// through). The read creates nothing and mutates nothing — a notification
// read/dismiss never implies approval (requirement (f) holds structurally:
// this read is the only approval query, and it is a pure read).
func (s *Slice) JobApproval(ctx context.Context, jobID string) (aijobrelay.Approval, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataRead); err != nil {
		return aijobrelay.Approval{}, err
	}
	return s.deps.Relay.LoadLatestApprovalForJob(ctx, jobID)
}

// authorizeDecide runs the one decision port for the approval-decision
// write and returns the decision so the command's own audit record can
// carry the authorization reference (C2-14 §6/C2-15). The same port, the
// same fail-closed rules, and the same envelope mapping as authorize — the
// captured decision is the only difference.
func (s *Slice) authorizeDecide(ctx context.Context, corr string, jobID string) (security.Decision, error) {
	if s.deps.Guard == nil {
		return security.Decision{}, fmt.Errorf("app: guard is not wired")
	}
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: principalID(ctx)}
	pctx := security.PolicyContext{CorrelationID: corr, Credential: credential(ctx)}
	decision, auditErr := s.deps.Guard.Authorize(p, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataWrite, pctx)
	if auditErr != nil {
		return decision, auditErr // the effect is denied, not executed unrecorded (C2-15)
	}
	if !decision.Allow {
		return decision, envelopeErr(errs.CodeAuthzDenied, corr, map[string]any{
			"resource": (security.Resource{Type: security.ResourceJob, ID: jobID}).String(),
			"action":   string(security.ActionDataWrite), "reason": decision.ReasonCode,
		})
	}
	return decision, nil
}

// DecideApproval applies one approval decision command (the card-14 working
// candidate; the six-requirement mapping is this file's header comment).
// The result is the Core-confirmed post-decision state; Replayed reports
// the honest replay (the same decision re-presented returns the recorded
// outcome without re-applying).
func (s *Slice) DecideApproval(ctx context.Context, req ApprovalDecisionRequest) (aijobrelay.ApprovalDecisionResult, error) {
	corr := errs.NewCorrelationID()
	// (3) scope-bound: the decision port authorizes the write on the Job the
	// approval gates — the code's own resource/action register for every Job
	// write.
	decision, err := s.authorizeDecide(ctx, corr, req.JobID)
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	// The request-shape law: typed refusals, never silent coercion.
	if req.JobID == "" {
		return aijobrelay.ApprovalDecisionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "job_id"})
	}
	if req.ApprovalID == "" {
		return aijobrelay.ApprovalDecisionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "approval_id"})
	}
	if !aijobrelay.ValidApprovalDecision(req.Decision) {
		return aijobrelay.ApprovalDecisionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "decision"})
	}
	// (5) audited: the decision is an audit event of row 10's model
	// (actor, action, decision, outcome), recorded BEFORE the effect on the
	// fail-closed audit path (C2-15 §5). The recorded outcome is the
	// decision the actor recorded; the Core-confirmed transition is durable
	// in the approval record and the Job event journal.
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "approval.decide",
		Scope:           (security.Resource{Type: security.ResourceJob, ID: req.JobID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		JobID:           req.JobID,
		Outcome:         req.Decision,
		CorrelationID:   corr,
	}); err != nil {
		// The ErrAuditUnavailable family maps onto the closed register
		// (STORAGE_UNAVAILABLE with the typed reason) through its own
		// Envelope method — the effect is denied, never executed unrecorded.
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	// (1)/(2)/(4): the durable, fenced decision through the relay — the
	// Core-issued identity, the presented job scope, the replay-safe
	// semantics, the confirmed transition.
	res, err := s.deps.Relay.DecideApproval(ctx, aijobrelay.ApprovalDecision{
		JobID:         req.JobID,
		ApprovalID:    req.ApprovalID,
		Decision:      req.Decision,
		Actor:         actor,
		CorrelationID: corr,
	})
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	return res, nil
}
