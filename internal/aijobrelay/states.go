package aijobrelay

import (
	"fmt"
)

// Job states (C3-12, plan §5): the accepted D-34 state machine mapped onto
// the local durable Job. Terminal states accept no further transitions.
const (
	JobCreated             = "created"
	JobQueued              = "queued"
	JobRunning             = "running"
	JobWaiting             = "waiting"
	JobPaused              = "paused"
	JobCancelRequested     = "cancel_requested"
	JobCancellationPending = "cancellation_pending"
	JobNeedsReconciliation = "needs_reconciliation"
	JobSucceeded           = "succeeded"
	JobFailed              = "failed"
	JobCancelled           = "cancelled"
	JobTimedOut            = "timed_out"
)

// Attempt outcomes (C3-12): a retryable Attempt failure is distinct from a
// terminal Job failure; retries create a NEW Attempt (never a rewrite).
const (
	AttemptRunning         = "running"
	AttemptSucceeded       = "succeeded"
	AttemptFailedRetryable = "failed_retryable"
	AttemptFailedTerminal  = "failed_terminal"
)

// SideEffectIntent states (C3-12/C3-14): Unknown is a first-class state — a
// lost response is never inferred to be a success or a safe retry; it
// forces NeedsReconciliation until the outcome is known or exact-provider
// idempotency proves safety.
const (
	SideEffectPrepared    = "prepared"
	SideEffectDispatched  = "dispatched"
	SideEffectSucceeded   = "succeeded"
	SideEffectFailedNoEff = "failed_no_effect"
	SideEffectUnknown     = "unknown"
	SideEffectCompensated = "compensated"
)

// Typed Waiting reasons (C3-12): a Waiting job always names why. The set is
// a closed register: any change to it is a deliberate, recorded contract
// change (the register's own test rule — pinned by TestWaitingReasonRegister).
type WaitingReason string

const (
	WaitBackoffScheduled      WaitingReason = "backoff_scheduled"
	WaitProviderRateLimited   WaitingReason = "provider_rate_limited"
	WaitUnknownSideEffect     WaitingReason = "unknown_side_effect"
	WaitCheckpointResume      WaitingReason = "checkpoint_resume"
	WaitDependencyUnavailable WaitingReason = "dependency_unavailable"
	// WaitApproval is the recorded C1-10 approval reason — Waiting{reason:
	// approval} — added to this register by the owner's card-17 decision of
	// 2026-11-02 (a deliberate register extension): the state machine can
	// now represent an approval-required wait. The approval command/object
	// itself is its own later milestone and is NOT implemented here.
	WaitApproval WaitingReason = "approval"
)

// ValidWaitingReason reports whether r is a member of the closed Waiting
// register. The fenced-commit path refuses an unregistered reason so the
// register stays enforced, not just pinned.
func ValidWaitingReason(r WaitingReason) bool {
	switch r {
	case WaitBackoffScheduled, WaitProviderRateLimited, WaitUnknownSideEffect,
		WaitCheckpointResume, WaitDependencyUnavailable, WaitApproval:
		return true
	}
	return false
}

// RegisteredJobState reports whether s is a member of the coded Job-state
// registry (the accepted D-34 registry above). The Jobs-list status filter
// accepts only registry members — filter options come from the Core only
// (P1.7 §6.1), never invented client-side.
func RegisteredJobState(s string) bool {
	_, ok := jobTransitions[s]
	return ok
}

// jobTransitions is the legal Job transition table (C3-12 acceptance: only
// legal transitions are accepted; no aggregate's states are overloaded).
var jobTransitions = map[string][]string{
	JobCreated:             {JobQueued, JobCancelled},
	JobQueued:              {JobRunning, JobCancelled, JobPaused},
	JobRunning:             {JobWaiting, JobPaused, JobCancelRequested, JobCancellationPending, JobSucceeded, JobFailed, JobTimedOut},
	JobWaiting:             {JobQueued, JobRunning, JobNeedsReconciliation, JobCancelRequested, JobPaused},
	JobPaused:              {JobQueued, JobCancelled},
	JobCancelRequested:     {JobCancellationPending, JobCancelled, JobFailed},
	JobCancellationPending: {JobCancelled, JobFailed},
	JobNeedsReconciliation: {JobQueued, JobFailed, JobCancelled},
	JobSucceeded:           {},
	JobFailed:              {},
	JobCancelled:           {},
	JobTimedOut:            {},
}

var terminalSet = map[string]bool{
	JobSucceeded: true, JobFailed: true, JobCancelled: true, JobTimedOut: true,
}

// jobEventTypes is the Job transition-event register — the recorded C1-10
// register as the code emits it at its emission sites, WITH `job.submitted`
// added: the owner's card-6 decision of 2026-11-02 (an additive register
// change per P1.5 §4.1; the code already emits `job.submitted` at the
// durable submit). The list is the register pin; any change to it is a
// deliberate, recorded contract change (the register's own test rule —
// pinned by TestJobEventRegister, after errs.go's
// TestRegisterMatchesSpecification pattern).
var jobEventTypes = []string{
	"job.cancel_requested",
	"job.cancellation_pending",
	"job.cancelled",
	"job.claimed",
	"job.failed",
	"job.needs_reconciliation",
	"job.paused",
	"job.queued",
	"job.requeued",
	"job.submitted",
	"job.succeeded",
	"job.timed_out",
	"job.waiting",
}

// TerminalJob reports whether the state is terminal (C3-12).
func TerminalJob(state string) bool {
	return terminalSet[state]
}

// CanTransition reports whether the Job transition is legal.
func CanTransition(from, to string) bool {
	for _, next := range jobTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// sideEffectTransitions is the SideEffectIntent table (C3-12/C3-14): an
// Unknown outcome resolves only through reconciliation or compensation —
// never by a blind retry into Succeeded.
var sideEffectTransitions = map[string][]string{
	SideEffectPrepared:    {SideEffectDispatched},
	SideEffectDispatched:  {SideEffectSucceeded, SideEffectFailedNoEff, SideEffectUnknown},
	SideEffectSucceeded:   {SideEffectCompensated},
	SideEffectFailedNoEff: {},
	SideEffectUnknown:     {SideEffectSucceeded, SideEffectFailedNoEff, SideEffectCompensated},
	SideEffectCompensated: {},
}

// CanTransitionSideEffect reports whether the intent transition is legal.
func CanTransitionSideEffect(from, to string) bool {
	for _, next := range sideEffectTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// CheckTerminalJob rejects any transition out of a terminal state — the
// shared guard for every fenced commit path.
func CheckTerminalJob(state string) error {
	if TerminalJob(state) {
		return fmt.Errorf("aijobrelay: job is already terminal (%s)", state)
	}
	return nil
}
