package aijobrelay

import (
	"errors"
	"testing"
	"time"
)

// CR-4.2 conformance matrix (C4-18) as executable fixtures: every accepted
// D-40 rule is proven here — transitions, nesting bounds, retry boundary,
// propagation direction, event mapping, and pin/migration rules. This is
// the future durable-Run conformance baseline the schema/API freeze will
// cite.

var runNow = time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)

func baseRun(t *testing.T) *Run {
	t.Helper()
	r, err := NewRun("run-1", "wf", "wfreq-abc", "wfcompile/v1", "job-root", nil, runNow)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	return r
}

func TestRunTransitionMatrix(t *testing.T) {
	allowed := map[string][]string{
		RunRunning:   {RunWaiting, RunPaused, RunSucceeded, RunFailed, RunCancelled},
		RunWaiting:   {RunRunning, RunCancelled},
		RunPaused:    {RunRunning, RunCancelled},
		RunSucceeded: {},
		RunFailed:    {},
		RunCancelled: {},
	}
	for from, targets := range allowed {
		if len(targets) == 0 {
			// terminal: nothing may move it
			run := baseRun(t)
			run.State = from
			for _, to := range []string{RunRunning, RunWaiting, RunPaused, RunSucceeded, RunFailed, RunCancelled} {
				if to == from {
					continue
				}
				if err := TransitionRun(run, to, runNow); err == nil {
					t.Errorf("terminal %s must not transition to %s", from, to)
				}
			}
			continue
		}
		for _, to := range targets {
			r := baseRun(t)
			r.State = from
			v := r.Version
			if err := TransitionRun(r, to, runNow); err != nil {
				t.Errorf("%s -> %s must be allowed: %v", from, to, err)
			}
			if r.State != to || r.Version != v+1 {
				t.Errorf("%s -> %s: state/version not applied: %+v", from, to, r)
			}
		}
	}
	// One illegal move spot-checked with the typed error.
	run := baseRun(t)
	run.State = RunSucceeded
	if err := TransitionRun(run, RunRunning, runNow); !errors.Is(err, ErrRunTransition) {
		t.Fatalf("terminal transition must be typed, got %v", err)
	}
}

func TestNestedRunIdentityAndBounds(t *testing.T) {
	root := baseRun(t)
	child, err := NewRun("run-child", "wf", "wfreq-child", "wfcompile/v1", "job-root", root, runNow)
	if err != nil {
		t.Fatalf("child run: %v", err)
	}
	if child.Depth != 1 || child.ParentRunID != root.RunID || child.RootJobID != root.RootJobID {
		t.Fatalf("child identity mismatch: %+v", child)
	}
	// Depth bound: build a chain to the limit and try one more.
	deep := root
	for i := 0; i < MaxRunNestingDepth; i++ {
		next, err := NewRun("run-d", "wf", "wfreq-d", "wfcompile/v1", "job-root", deep, runNow)
		if err != nil {
			t.Fatalf("depth %d run: %v", i+1, err)
		}
		deep = next
	}
	if deep.Depth != MaxRunNestingDepth {
		t.Fatalf("expected depth %d, got %d", MaxRunNestingDepth, deep.Depth)
	}
	if _, err := NewRun("run-x", "wf", "wfreq-x", "wfcompile/v1", "job-root", deep, runNow); !errors.Is(err, ErrRunBounds) {
		t.Fatalf("nesting beyond the bound must be refused, got %v", err)
	}
	// A child of a terminal parent is refused.
	done := baseRun(t)
	if err := TransitionRun(done, RunSucceeded, runNow); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := NewRun("run-c2", "wf", "wfreq-c2", "wfcompile/v1", "job-root", done, runNow); !errors.Is(err, ErrRunBounds) {
		t.Fatalf("child of a terminal run must be refused, got %v", err)
	}
}

func TestRetryBoundary(t *testing.T) {
	mk := func(class, scope, unknown string) StepRecord {
		return StepRecord{RunID: "run-1", Node: "act", InvocationID: "inv-1",
			AttemptSeq: 1, State: StepRunning, RetryClass: class,
			IdempotencyScope: scope, UnknownOutcome: unknown}
	}
	// Succeeded is final for every class.
	for _, class := range []string{StepRetrySafe, StepRetryScoped, StepRetryNever} {
		if retry, state, _ := RetryDecision(mk(class, "s", StepUnknownFail), StepSucceeded); retry || state != StepSucceeded {
			t.Fatalf("succeeded must be final for %s", class)
		}
	}
	// Safe retry: allowed on failed-without-effect.
	safe := mk(StepRetrySafe, "", StepUnknownFail)
	if retry, next, _ := RetryDecision(safe, "failed_no_effect"); !retry || next != StepPending {
		t.Fatalf("safe retry must be allowed, got %v %s", retry, next)
	}
	// Scoped retry: allowed ONLY with an idempotency scope.
	scoped := mk(StepRetryScoped, "order:42", StepUnknownFail)
	if retry, _, _ := RetryDecision(scoped, "failed_no_effect"); !retry {
		t.Fatal("scoped retry with a scope must be allowed")
	}
	noScope := mk(StepRetryScoped, "", StepUnknownFail)
	if retry, next, _ := RetryDecision(noScope, "failed_no_effect"); retry || next != StepFailed {
		t.Fatal("scoped retry without a scope must fail the step")
	}
	// Never: at-most-once.
	never := mk(StepRetryNever, "", StepUnknownFail)
	if retry, next, _ := RetryDecision(never, "failed_no_effect"); retry || next != StepFailed {
		t.Fatal("never must be at-most-once")
	}
	// Unknown outcome: NEVER a blind retry — reconciliation or failure.
	unknownRec := mk(StepRetrySafe, "", StepUnknownNeedsReconciliation)
	if retry, next, _ := RetryDecision(unknownRec, "unknown"); retry || next != StepNeedsReconciliation {
		t.Fatalf("unknown outcome must force reconciliation, got %v %s", retry, next)
	}
	unknownFail := mk(StepRetrySafe, "", StepUnknownFail)
	if retry, next, _ := RetryDecision(unknownFail, "unknown"); retry || next != StepFailed {
		t.Fatalf("unknown outcome with fail policy must fail, got %v %s", retry, next)
	}
	// Even a needs_reconciliation record never auto-retries.
	if retry, _, _ := RetryDecision(unknownRec, StepNeedsReconciliation); retry {
		t.Fatal("needs_reconciliation must never auto-retry")
	}
}

func TestPropagationDirection(t *testing.T) {
	root := baseRun(t)
	child, _ := NewRun("run-child", "wf", "wfreq-c", "wfcompile/v1", "job-root", root, runNow)
	unrelated := baseRun(t)
	unrelated.RunID = "run-other"
	if !CanPropagateTo(root, child) {
		t.Fatal("a root must propagate to its own child")
	}
	if CanPropagateTo(child, root) {
		t.Fatal("a child must never propagate upward (D-40)")
	}
	if CanPropagateTo(root, unrelated) {
		t.Fatal("unrelated runs must not propagate")
	}
}

func TestRelinkRevisionRules(t *testing.T) {
	run := baseRun(t)
	// Same execution version + recorded reason: allowed.
	if err := RelinkRevision(run, "wfreq-next", "wfcompile/v1", "semantics unchanged per workflow.MigrationAllowed", runNow); err != nil {
		t.Fatalf("legal relink: %v", err)
	}
	if run.RevisionID != "wfreq-next" {
		t.Fatalf("pin not applied: %s", run.RevisionID)
	}
	// Cross-execution-version relink refused (successor runs in new executions).
	if err := RelinkRevision(run, "wfreq-n2", "wfcompile/v2", "reason", runNow); !errors.Is(err, ErrRunPin) {
		t.Fatalf("cross-version relink must be refused, got %v", err)
	}
	// Empty reason refused (no silent migration).
	if err := RelinkRevision(run, "wfreq-n3", "wfcompile/v1", "", runNow); !errors.Is(err, ErrRunPin) {
		t.Fatalf("reason-less relink must be refused, got %v", err)
	}
	// Terminal runs never relink.
	if err := TransitionRun(run, RunSucceeded, runNow); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := RelinkRevision(run, "wfreq-n4", "wfcompile/v1", "reason", runNow); !errors.Is(err, ErrRunPin) {
		t.Fatalf("terminal relink must be refused, got %v", err)
	}
}

func TestRunEventRecords(t *testing.T) {
	run := baseRun(t)
	rec, err := RunEventRecord(1, runNow, run, EventRunStarted, nil)
	if err != nil {
		t.Fatalf("event build: %v", err)
	}
	if rec.AggregateType != "run" || rec.AggregateID != run.RunID || rec.EventType != EventRunStarted {
		t.Fatalf("event identity mismatch: %+v", rec)
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("event must satisfy the accepted record contract: %v", err)
	}
	if _, err := RunEventRecord(0, runNow, run, EventRunStarted, nil); err == nil {
		t.Fatal("sequence must start at 1")
	}
}
