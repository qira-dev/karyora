package aijobrelay

import "time"

// The approval object (the owner's card-14 working candidate of 2026-11-02,
// the P3.2 §3.8 six-requirement set — implemented in IMPL-7): the Core
// raises one approval request when a Job durably enters Waiting{reason:
// approval}; the request carries a Core-issued, stable, queryable identity
// bound to that raised request. The accept/reject decisions are Core
// commands carrying that identity; the decision result is the
// Core-confirmed transition (S2 only on that transition). A notification
// read/dismiss never implies approval — reads mutate nothing, structurally.
//
// Requirement mapping (each recorded in the IMPL-7 record with its code
// source): (a) the identity is minted by the Core at the Waiting{approval}
// commit and stored durably (queryable by id and by job); (b) the decision
// carries its correlation id and applies under the fenced single-writer
// path — a replayed decision never double-applies (same decision → the
// recorded outcome, no state change; a contradicting decision → the typed
// CONFLICT_STATE); (c) the decision's scope is exactly the presented
// approval identity bound to its own Job — never broader; (d) the result is
// the Core-confirmed transition of the approval state and the Job; (e) the
// decision is an audit event of row 10's model (the app layer records it on
// the fail-closed audit path); (f) nothing in the read surface implies a
// decision.

// Approval is one durable approval request (the card-14 working
// candidate's object schema — recorded exactly for the freeze).
type Approval struct {
	ApprovalID    string    `json:"approval_id"`
	JobID         string    `json:"job_id"`
	State         string    `json:"state"`
	Decision      string    `json:"decision,omitempty"`
	DecidedBy     string    `json:"decided_by,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	DecidedAt     time.Time `json:"decided_at,omitempty"`
}

// Approval states (a closed register; any change to it is a deliberate,
// recorded contract change — the register's own test rule, pinned by
// TestApprovalStateRegister).
const (
	ApprovalPending  = "pending"
	ApprovalAccepted = "accepted"
	ApprovalRejected = "rejected"
)

// Approval decisions — the two Core commands of the card-14 identity
// (a closed register; pinned by TestApprovalDecisionRegister).
const (
	DecisionAccept = "accept"
	DecisionReject = "reject"
)

// approvalTransitions is the legal approval-state table: a pending request
// resolves exactly once to accepted or rejected; a decided approval is
// immutable (a replayed decision never re-applies).
var approvalTransitions = map[string][]string{
	ApprovalPending:  {ApprovalAccepted, ApprovalRejected},
	ApprovalAccepted: {},
	ApprovalRejected: {},
}

// ValidApprovalState reports whether s is a member of the closed approval
// register.
func ValidApprovalState(s string) bool {
	_, ok := approvalTransitions[s]
	return ok
}

// CanTransitionApproval reports whether the approval-state transition is
// legal.
func CanTransitionApproval(from, to string) bool {
	for _, next := range approvalTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// ValidApprovalDecision reports whether d is a member of the closed
// decision register (the accept/reject command vocabulary).
func ValidApprovalDecision(d string) bool {
	return d == DecisionAccept || d == DecisionReject
}

// DecisionForState maps a decided approval state onto the decision command
// that produced it (accepted ← accept; rejected ← reject). A pending
// approval carries no decision.
func DecisionForState(state string) string {
	switch state {
	case ApprovalAccepted:
		return DecisionAccept
	case ApprovalRejected:
		return DecisionReject
	}
	return ""
}

// ApprovalDecision is one Core decision command (requirements (a)/(b)/(c)):
// it carries the presented scope (the Job the approval gates), the
// Core-issued approval identity, the decision, the acting principal
// identifier (bounded, never a credential value), and the command's
// correlation id. The storage adapter applies it under the fenced
// single-writer path so a replayed decision never double-applies, and it
// refuses any approval whose bound Job is not the presented scope (the same
// typed NOT_FOUND shape as an unknown identity — the no-existence-leak
// rule).
type ApprovalDecision struct {
	JobID         string
	ApprovalID    string
	Decision      string
	Actor         string
	CorrelationID string
}

// ApprovalDecisionResult is the Core-confirmed result of one decision
// command (requirement (d)): the recorded approval, the post-decision Job,
// and the honest replayed flag (a replayed command returns the recorded
// outcome without re-applying — never a second transition).
type ApprovalDecisionResult struct {
	Approval Approval `json:"approval"`
	Job      Job      `json:"job"`
	Replayed bool     `json:"replayed"`
}
