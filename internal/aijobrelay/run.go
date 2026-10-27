package aijobrelay

import (
	"errors"
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// Durable Workflow Runs (CR-4.2, D-40 Option A accepted 2026-10-28): a Run
// is a first-class durable record INSIDE this package — AIJobRelay remains
// the sole execution authority; no second scheduler or journal exists.
// Cardinality: one Run = one root Job (the root Job carries the Run id);
// each workflow node invocation is a child STEP RECORD, never a separate
// Job. Nested Runs arise only from pinned subworkflow revisions with a
// bounded depth. Run schema/API FREEZE waits for CR-4.2 conformance
// evidence (D-34/D-35 discipline); these types are the specified model.

// Run states. Terminal states accept no further transitions.
const (
	RunRunning   = "running"
	RunWaiting   = "waiting"
	RunPaused    = "paused"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunCancelled = "cancelled"
)

// MaxRunNestingDepth mirrors the D-40 decision: nested Runs arise only
// from pinned subworkflow revisions and never exceed the definitions-layer
// bound.
const MaxRunNestingDepth = 4

// runTransitions is the accepted D-40 transition table (C4-15): pause and
// cancel are reachable from every live state; terminal states are final.
var runTransitions = map[string]map[string]bool{
	RunRunning:   {RunWaiting: true, RunPaused: true, RunSucceeded: true, RunFailed: true, RunCancelled: true},
	RunWaiting:   {RunRunning: true, RunCancelled: true},
	RunPaused:    {RunRunning: true, RunCancelled: true},
	RunSucceeded: {},
	RunFailed:    {},
	RunCancelled: {},
}

// Step states (C4-12): the logical node invocation is distinct from its
// executing Attempt; NeedsReconciliation is a first-class state — an
// unknown outcome is never inferred into success or a safe retry.
const (
	StepPending             = "pending"
	StepRunning             = "running"
	StepSucceeded           = "succeeded"
	StepFailed              = "failed"
	StepNeedsReconciliation = "needs_reconciliation"
)

// Mirrored effect vocabulary (C4-14): step records snapshot the node's
// declared effect envelope from the compiled step. The authoritative
// definitions live in the workflow package; the mapping happens at the
// execution wiring, which must copy — never reinterpret — these values.
const (
	StepRetrySafe   = "safe"
	StepRetryScoped = "scoped"
	StepRetryNever  = "never"

	StepUnknownFail                = "fail"
	StepUnknownNeedsReconciliation = "needs_reconciliation"
)

// Run is one durable workflow execution record, pinned to an immutable
// revision and compiler version (C4-17).
type Run struct {
	RunID            string    `json:"run_id"`
	WorkflowID       string    `json:"workflow_id"`
	RevisionID       string    `json:"revision_id"` // the pinned immutable revision
	ExecutionVersion string    `json:"execution_version"`
	RootJobID        string    `json:"root_job_id"`
	ParentRunID      string    `json:"parent_run_id,omitempty"` // empty for root Runs
	Depth            int       `json:"depth"`                   // 0 = root; <= MaxRunNestingDepth
	State            string    `json:"state"`
	DeadlineSecs     int64     `json:"deadline_secs,omitempty"`
	Version          int64     `json:"version"` // optimistic concurrency
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// StepRecord is one logical node invocation inside a Run. Retries create a
// NEW Attempt (AttemptSeq increments) — a record is never rewritten into a
// different attempt.
type StepRecord struct {
	RunID            string `json:"run_id"`
	Node             string `json:"node"`
	InvocationID     string `json:"invocation_id"`
	AttemptSeq       int64  `json:"attempt_seq"`
	State            string `json:"state"`
	RetryClass       string `json:"retry_class"`
	IdempotencyScope string `json:"idempotency_scope,omitempty"`
	UnknownOutcome   string `json:"unknown_outcome"`
	Attempts         int64  `json:"attempts"`
}

// Sentinel errors for the Run model.
var (
	ErrRunTransition  = errors.New("aijobrelay: illegal run transition")
	ErrRunBounds      = errors.New("aijobrelay: run nesting bound violated")
	ErrRunPin         = errors.New("aijobrelay: run revision pin violation")
	ErrRunPropagation = errors.New("aijobrelay: propagation direction violation")
)

// NewRun constructs a Run record with D-40 identity rules (C4-13): root
// Runs carry no parent and depth 0; child Runs name their parent and sit
// exactly one level deeper, never beyond MaxRunNestingDepth.
func NewRun(runID, workflowID, revisionID, executionVersion, rootJobID string, parent *Run, now time.Time) (*Run, error) {
	if runID == "" || workflowID == "" || revisionID == "" || executionVersion == "" || rootJobID == "" {
		return nil, fmt.Errorf("%w: run identity fields are required", ErrRunBounds)
	}
	depth := 0
	parentID := ""
	if parent != nil {
		parentID = parent.RunID
		depth = parent.Depth + 1
		if parent.State != RunRunning && parent.State != RunWaiting && parent.State != RunPaused {
			return nil, fmt.Errorf("%w: child runs require a live parent", ErrRunBounds)
		}
	}
	if depth > MaxRunNestingDepth {
		return nil, fmt.Errorf("%w: nesting depth %d exceeds %d", ErrRunBounds, depth, MaxRunNestingDepth)
	}
	return &Run{
		RunID: runID, WorkflowID: workflowID, RevisionID: revisionID,
		ExecutionVersion: executionVersion, RootJobID: rootJobID,
		ParentRunID: parentID, Depth: depth,
		State: RunRunning, Version: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

// TransitionRun applies one state transition under the accepted table
// (C4-15); the version increments so optimistic concurrency sees the move.
func TransitionRun(run *Run, to string, now time.Time) error {
	allowed, ok := runTransitions[run.State]
	if !ok || !allowed[to] {
		return fmt.Errorf("%w: %s -> %s", ErrRunTransition, run.State, to)
	}
	run.State = to
	run.Version++
	run.UpdatedAt = now.UTC()
	return nil
}

// CanPropagateTo enforces the D-40 direction rule (C4-15): pause, cancel,
// and deadline flow from a Run to its OWN child Runs only. A child can
// never affect its parent; unrelated Runs never propagate to each other.
func CanPropagateTo(from, to *Run) bool {
	return to.ParentRunID == from.RunID
}

// RetryDecision answers whether a failed/unknown step attempt may retry
// (C4-14). Unknown outcomes are NEVER blindly retried: they force
// needs_reconciliation (or failure) per the node's declared policy. Safe
// retries are unbounded by this model (resource caps live in the node
// budget); scoped retries require the idempotency scope; never is
// at-most-once.
func RetryDecision(step StepRecord, outcome string) (retry bool, nextState string, reason string) {
	switch outcome {
	case StepSucceeded:
		return false, StepSucceeded, "succeeded"
	case "failed_no_effect":
		if step.RetryClass == StepRetryNever {
			return false, StepFailed, "retry class never: at-most-once"
		}
		if step.RetryClass == StepRetryScoped && step.IdempotencyScope == "" {
			return false, StepFailed, "scoped retry without idempotency scope cannot retry"
		}
		return true, StepPending, "failed without effect: a new attempt may run"
	case StepNeedsReconciliation, "unknown":
		if step.UnknownOutcome == StepUnknownNeedsReconciliation {
			return false, StepNeedsReconciliation, "unknown outcome: reconciliation required (never blind retry)"
		}
		return false, StepFailed, "unknown outcome with fail policy: terminal failure"
	default:
		return false, StepFailed, "terminal failure"
	}
}

// RelinkRevision moves a LIVE Run onto a successor revision (C4-17): only
// live states, only with a recorded migration reason, and only within the
// SAME execution version — a compiler-version change means new executions
// run the successor; in-flight Runs finish on their pin. The semantic
// equality of the two revisions is attested by the definitions layer
// (workflow.MigrationAllowed) at the call site; the reason must record it.
func RelinkRevision(run *Run, newRevisionID, newExecutionVersion, reason string, now time.Time) error {
	switch run.State {
	case RunSucceeded, RunFailed, RunCancelled:
		return fmt.Errorf("%w: terminal run %s cannot relink", ErrRunPin, run.RunID)
	}
	if newRevisionID == "" || newExecutionVersion == "" {
		return fmt.Errorf("%w: relink requires a revision and execution version", ErrRunPin)
	}
	if newExecutionVersion != run.ExecutionVersion {
		return fmt.Errorf("%w: in-flight relink across execution versions is refused (%s -> %s); the successor runs in new executions",
			ErrRunPin, run.ExecutionVersion, newExecutionVersion)
	}
	if reason == "" {
		return fmt.Errorf("%w: relink requires a recorded migration reason (semantics attestation)", ErrRunPin)
	}
	run.RevisionID = newRevisionID
	run.Version++
	run.UpdatedAt = now.UTC()
	return nil
}

// Run and step event types on the ACCEPTED event/outbox contract (C4-16):
// no new journal; these are ordinary records on the existing stream.
const (
	EventRunStarted     = "run.started"
	EventRunWaiting     = "run.waiting"
	EventRunPaused      = "run.paused"
	EventRunSucceeded   = "run.succeeded"
	EventRunFailed      = "run.failed"
	EventRunCancelled   = "run.cancelled"
	EventRunStepUpdated = "run.step.updated"
)

// RunEventRecord builds one outbox record for a Run transition or step
// update. The payload carries state facts only — never credentials, task
// text, or content (the same redaction discipline as Job events).
func RunEventRecord(sequence uint64, at time.Time, run *Run, eventType string, extra map[string]any) (events.Record, error) {
	if sequence == 0 {
		return events.Record{}, fmt.Errorf("%w: event sequence starts at 1", ErrRunTransition)
	}
	payload := map[string]any{
		"run_id":            run.RunID,
		"workflow_id":       run.WorkflowID,
		"revision_id":       run.RevisionID,
		"execution_version": run.ExecutionVersion,
		"root_job_id":       run.RootJobID,
		"depth":             run.Depth,
		"state":             run.State,
		"version":           run.Version,
	}
	for k, v := range extra {
		payload[k] = v
	}
	raw, err := events.CanonicalJSON(payload)
	if err != nil {
		return events.Record{}, fmt.Errorf("%w: payload canonicalization failed", ErrRunTransition)
	}
	return events.Record{
		EventID:       fmt.Sprintf("evt-run-%s-%d", run.RunID, sequence),
		AggregateType: "run",
		AggregateID:   run.RunID,
		Sequence:      sequence,
		EventType:     eventType,
		SchemaVersion: 1,
		OccurredAt:    at.UTC(),
		Producer:      "aijobrelay",
		InlinePayload: raw,
	}, nil
}
