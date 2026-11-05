package aijobrelay

import (
	"strings"
	"testing"
)

// TestRetryIdempotencyRegister pins the closed three-value
// guarantee-class register (the P-S7-03 vocabulary carried exactly): any
// change to it is a deliberate, recorded contract change — the register's
// own test rule (the TestWaitingReasonRegister pattern).
func TestRetryIdempotencyRegister(t *testing.T) {
	registered := []RetryIdempotencyClass{
		RetryIdempotencySuppliedKeyed,
		RetryIdempotencyOpen,
		RetryIdempotencyNone,
	}
	seen := map[RetryIdempotencyClass]bool{}
	for _, c := range registered {
		if !ValidRetryIdempotencyClass(c) {
			t.Fatalf("register member %q must validate", c)
		}
		if seen[c] {
			t.Fatalf("duplicate register member %q", c)
		}
		seen[c] = true
	}
	if len(seen) != 3 {
		t.Fatalf("the register carries exactly the three recorded classes, got %d", len(seen))
	}
	for _, absent := range []RetryIdempotencyClass{"", "supplied-keyed", "Open", "at-least-once"} {
		if ValidRetryIdempotencyClass(absent) {
			t.Fatalf("non-member %q must not validate", absent)
		}
	}
}

// TestRetryEligibilityVerdictTable pins the Core's verdict over the coded
// state registry: allow only for the terminal Failed/TimedOut
// dispositions (the canonical retry case — the typed outcome exists, so
// outcome reconciliation has its object); deny for every non-terminal
// state (the current Attempt's outcome is still pending; a Paused Job's
// exit is the coded Resume, never a retry), for Cancelled (the recorded
// cancellation intent is authoritative) and for Succeeded (a re-run is
// not a retry). The reconciliation member rides the state and the
// Unknown-side-effect fact exactly (the C3-14 fact); the class stays
// `open` until the keying act.
func TestRetryEligibilityVerdictTable(t *testing.T) {
	for _, tt := range []struct {
		state        string
		wantEligible bool
	}{
		{JobFailed, true},
		{JobTimedOut, true},
		{JobSucceeded, false},
		{JobCancelled, false},
	} {
		for _, unknown := range []bool{false, true} {
			res := RetryEligibilityFor(tt.state, unknown)
			if res.Eligible != tt.wantEligible {
				t.Fatalf("%s/unknown=%v: eligible=%v, want %v", tt.state, unknown, res.Eligible, tt.wantEligible)
			}
			if res.ReconciliationRequired != unknown {
				t.Fatalf("%s/unknown=%v: reconciliation_required=%v, want %v", tt.state, unknown, res.ReconciliationRequired, unknown)
			}
			if res.IdempotencyClass != RetryIdempotencyOpen {
				t.Fatalf("%s: the class must be `open` until the keying act, got %q", tt.state, res.IdempotencyClass)
			}
			if !ValidRetryIdempotencyClass(res.IdempotencyClass) {
				t.Fatalf("%s: the class must be a register member", tt.state)
			}
		}
	}
	for _, state := range []string{
		JobCreated, JobQueued, JobRunning, JobWaiting, JobPaused,
		JobCancelRequested, JobCancellationPending,
	} {
		res := RetryEligibilityFor(state, false)
		if res.Eligible {
			t.Fatalf("%s: a non-terminal job must never be retry-eligible", state)
		}
		if res.ReconciliationRequired {
			t.Fatalf("%s: no reconciliation requirement without an Unknown side effect", state)
		}
	}
	res := RetryEligibilityFor(JobNeedsReconciliation, false)
	if res.Eligible || !res.ReconciliationRequired {
		t.Fatalf("needs_reconciliation must deny and require reconciliation: %+v", res)
	}
}

// TestRetryRiskStatementExactTerms pins the recorded exact-terms law over
// the Core's own statement: the exact terms only — no exactly-once
// wording anywhere (the P3.7 §7.1 acceptance law; J-5).
func TestRetryRiskStatementExactTerms(t *testing.T) {
	res := RetryEligibilityFor(JobFailed, false)
	for _, term := range []string{
		"new attempt of the same job",
		"retry is not reversal",
		"at-least-once attempts",
		"needs reconciliation",
	} {
		if !strings.Contains(res.RiskStatement, term) {
			t.Fatalf("the risk statement must carry the exact term %q: %q", term, res.RiskStatement)
		}
	}
	if strings.Contains(strings.ToLower(res.RiskStatement), "exactly-once") {
		t.Fatalf("no exactly-once wording anywhere: %q", res.RiskStatement)
	}
}
