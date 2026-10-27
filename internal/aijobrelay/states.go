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

// Typed Waiting reasons (C3-12): a Waiting job always names why.
type WaitingReason string

const (
	WaitBackoffScheduled      WaitingReason = "backoff_scheduled"
	WaitProviderRateLimited   WaitingReason = "provider_rate_limited"
	WaitUnknownSideEffect     WaitingReason = "unknown_side_effect"
	WaitCheckpointResume      WaitingReason = "checkpoint_resume"
	WaitDependencyUnavailable WaitingReason = "dependency_unavailable"
)

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
