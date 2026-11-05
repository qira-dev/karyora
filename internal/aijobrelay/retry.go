package aijobrelay

// The retry-eligibility read's domain facts (the F9 family): the P3.2
// §3.9 [P] typed-member set as frozen at the S4 §15 freeze act (the
// adopted P-S7-01…06 supply) — a Core, permission-filtered, per-Job read
// returning exactly four typed members: the eligibility verdict, the
// side-effect risk statement, the idempotency guarantee class, and the
// reconciliation requirement. The verdict is the Core's own allow/deny
// for a retry of THIS Job — never a client-side inference from the
// lifecycle state (the M-5 gate's lesson carried: for M-7 no accepting
// state set exists in any payload, which is why this read exists). The
// read is NOT the retry command — the M-7 command's identity stays its
// recorded Open item (the S7 §5 gap 2; O-1), so no eligibility response
// is an authorization to execute anything.

// RetryIdempotencyClass is the idempotency-guarantee class's closed
// three-value register (the recorded vocabulary carried exactly, the
// P-S7-03 draft): `supplied_keyed` (a Supplied-class key — the same key
// returns the original result; never regenerated), `open` (no documented
// idempotency class — the P1.6 §4 law binds the consumption side: no
// re-send except the explicit user act), `none` (at-most-once: the
// operation is not idempotent and must not be repeated). The register's
// own test rule governs any change (pinned by
// TestRetryIdempotencyRegister). The retry path reports `open` until the
// Core/API owner's own recorded act defines the retry command's keying
// (the S7 §5 gap 7) — no class is invented here and no mapping to the
// envelope's RetryClass register or the submit key's replay window is
// implied (the P-S7-03 boundary: the coded registers are precedents,
// never pre-decided mappings).
type RetryIdempotencyClass string

const (
	RetryIdempotencySuppliedKeyed RetryIdempotencyClass = "supplied_keyed"
	RetryIdempotencyOpen          RetryIdempotencyClass = "open"
	RetryIdempotencyNone          RetryIdempotencyClass = "none"
)

// ValidRetryIdempotencyClass reports whether c is a member of the closed
// guarantee-class register.
func ValidRetryIdempotencyClass(c RetryIdempotencyClass) bool {
	switch c {
	case RetryIdempotencySuppliedKeyed, RetryIdempotencyOpen, RetryIdempotencyNone:
		return true
	}
	return false
}

// retryRiskStatement is the Core's own side-effect risk statement — the
// recorded retry laws rendered as the Core's report, in the recorded
// exact terms only (no exactly-once wording anywhere; the P3.7 §7.1
// acceptance law). The statement is part of the read's own surface, not
// a footnote (the plan's acceptance: "retry does not imply safety").
const retryRiskStatement = "A retry creates a new attempt of the same job and does not reverse the prior attempt's effects: retry is not reversal. Declared side effects may run again: at-least-once attempts. Unknown outcomes are never blindly retried: needs reconciliation."

// RetryEligibility is the read's typed result: exactly the four frozen
// members and nothing more (the adopted P-S7-01 supply — "returning at
// minimum the four recorded typed members and nothing presented beyond
// what the Core returns"; no ineligible-reason field exists: the S7 §5
// gap 5 stays its recorded owner's). The JSON tags are the coded wire
// casing (snake_case); the canonical schema resolves at the publication
// act per the O-1 precedent.
type RetryEligibility struct {
	// Eligible is the Core's own allow/deny verdict for a retry of THIS
	// Job (the P-S7-01 member 1): allow only where the Core's own
	// machinery accepts a new Attempt of the same Job — a terminal
	// Failed/TimedOut disposition (the canonical retry case: the typed
	// outcome exists, so outcome reconciliation has its object) — and
	// deny everywhere else: non-terminal states (the current Attempt's
	// outcome is still pending; a Paused Job's exit is the coded Resume,
	// never a retry), NeedsReconciliation (the unknown-outcome law —
	// reconcile first, never a blind retry), Cancelled (the recorded
	// cancellation intent is authoritative; re-running would contradict
	// it), and Succeeded (nothing to recover: a re-run is not a retry).
	Eligible bool `json:"eligible"`
	// RiskStatement is the Core-supplied statement rendered WITH the
	// control (the P-S7-01 member 2 — "the risk statement is part of the
	// control's own surface, not a footnote").
	RiskStatement string `json:"risk_statement"`
	// IdempotencyClass is exactly one of the recorded three classes (the
	// P-S7-01 member 3, the P-S7-03 vocabulary).
	IdempotencyClass RetryIdempotencyClass `json:"idempotency_class"`
	// ReconciliationRequired reports whether a reconciliation requirement
	// stands for THIS Job (the P-S7-01 member 4 — "where the Core reports
	// one, shown with the control; retry ≠ reversal"): true when the
	// Job's state is NeedsReconciliation or any side-effect intent
	// carries an Unknown outcome (the C3-14 fact) — the UI renders the
	// recorded exact terms (`needs reconciliation`) when true, never an
	// invented recovery story (the P-S7-04 boundary).
	ReconciliationRequired bool `json:"reconciliation_required"`
}

// RetryEligibilityFor computes the Core's verdict and members from its
// own recorded facts: the Job's authoritative state and whether any
// side-effect intent of the Job carries an Unknown outcome. No member is
// derived by any client; the read's caller (the app layer) authorizes
// through the one decision port before this computation runs.
func RetryEligibilityFor(state string, unknownSideEffects bool) RetryEligibility {
	eligible := state == JobFailed || state == JobTimedOut
	return RetryEligibility{
		Eligible:               eligible,
		RiskStatement:          retryRiskStatement,
		IdempotencyClass:       RetryIdempotencyOpen,
		ReconciliationRequired: state == JobNeedsReconciliation || unknownSideEffects,
	}
}
