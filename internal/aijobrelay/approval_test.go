package aijobrelay

import (
	"testing"
)

// The approval registers and the card-14 transition extension (IMPL-7):
// each closed register is pinned by its own test (the register's own test
// rule, the card-17 precedent — any change to a pinned set is a
// deliberate, recorded contract change).

// TestApprovalStateRegister pins the closed approval-state register: a
// pending request resolves exactly once to accepted or rejected.
func TestApprovalStateRegister(t *testing.T) {
	for _, s := range []string{ApprovalPending, ApprovalAccepted, ApprovalRejected} {
		if !ValidApprovalState(s) {
			t.Fatalf("registered approval state %q must validate", s)
		}
	}
	for _, s := range []string{"", "open", "decided", "Pending", "PENDING", "approve", "unknown"} {
		if ValidApprovalState(s) {
			t.Fatalf("value %q must not be a registered approval state", s)
		}
	}
	if !CanTransitionApproval(ApprovalPending, ApprovalAccepted) {
		t.Fatal("pending → accepted must be legal")
	}
	if !CanTransitionApproval(ApprovalPending, ApprovalRejected) {
		t.Fatal("pending → rejected must be legal")
	}
	for _, tr := range [][2]string{
		{ApprovalAccepted, ApprovalPending}, {ApprovalRejected, ApprovalPending},
		{ApprovalAccepted, ApprovalRejected}, {ApprovalRejected, ApprovalAccepted},
		{ApprovalAccepted, ApprovalAccepted}, {ApprovalRejected, ApprovalRejected},
	} {
		if CanTransitionApproval(tr[0], tr[1]) {
			t.Fatalf("transition %s → %s must be illegal (a decided approval is immutable)", tr[0], tr[1])
		}
	}
}

// TestApprovalDecisionRegister pins the closed decision register — the two
// Core commands of the card-14 identity, nothing invented.
func TestApprovalDecisionRegister(t *testing.T) {
	if DecisionAccept != "accept" || DecisionReject != "reject" {
		t.Fatalf("the decision vocabulary must be the coded accept/reject: %q/%q", DecisionAccept, DecisionReject)
	}
	if !ValidApprovalDecision(DecisionAccept) || !ValidApprovalDecision(DecisionReject) {
		t.Fatal("the registered decisions must validate")
	}
	for _, d := range []string{"", "approve", "deny", "Accept", "REJECT", "allow", "unknown"} {
		if ValidApprovalDecision(d) {
			t.Fatalf("value %q must not be a registered approval decision", d)
		}
	}
	if DecisionForState(ApprovalAccepted) != DecisionAccept {
		t.Fatal("accepted must map to the accept decision")
	}
	if DecisionForState(ApprovalRejected) != DecisionReject {
		t.Fatal("rejected must map to the reject decision")
	}
	if DecisionForState(ApprovalPending) != "" {
		t.Fatal("a pending approval carries no decision")
	}
}

// TestApprovalWaitingRejectedTerminalDisposition pins the card-14 working
// candidate's register-consistent transition extension: the Waiting state's
// previously recorded exits stay open, the accepted approval's re-queue
// (Waiting → Queued) needed no extension, and the rejected approval's
// terminal disposition (Waiting → Failed) is legal. Any change to the
// recorded exits is a deliberate, recorded contract change.
func TestApprovalWaitingRejectedTerminalDisposition(t *testing.T) {
	for _, to := range []string{JobQueued, JobRunning, JobNeedsReconciliation, JobCancelRequested, JobPaused} {
		if !CanTransition(JobWaiting, to) {
			t.Fatalf("Waiting → %s must stay a recorded exit", to)
		}
	}
	if !CanTransition(JobWaiting, JobFailed) {
		t.Fatal("Waiting → Failed must be legal: the rejected approval's terminal disposition")
	}
	if !CanTransition(JobWaiting, JobQueued) {
		t.Fatal("Waiting → Queued must be legal: the accepted approval's re-queue needs no extension")
	}
}
