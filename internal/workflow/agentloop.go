package workflow

import (
	"fmt"
	"sync"
	"time"
)

// The durable Agent-loop decision boundary (C4-09): model decisions,
// approved revisions, tool results, continuations, and ContextSnapshot
// references are recorded as Core-owned durable records. Provider-native
// sessions remain optimization metadata — recovery must be possible from
// these records alone when a provider session is unavailable. Prompt or
// retrieved content can never authorize anything: records DESCRIBE what
// happened; authority flows only through the policy boundary (C4-06).
// Exact model-output reproducibility is not promised.

type DecisionKind string

const (
	DecisionPlanProposal DecisionKind = "plan_proposal"
	DecisionToolResult   DecisionKind = "tool_result"
	DecisionPolicy       DecisionKind = "policy_decision"
	DecisionContinuation DecisionKind = "continuation"
)

func (k DecisionKind) valid() bool {
	switch k {
	case DecisionPlanProposal, DecisionToolResult, DecisionPolicy, DecisionContinuation:
		return true
	}
	return false
}

// DecisionRecord is one inspectable boundary event.
type DecisionRecord struct {
	Seq              int64        `json:"seq"`
	Kind             DecisionKind `json:"kind"`
	Node             string       `json:"node"`
	RevisionID       string       `json:"revision_id"`
	SnapshotRef      string       `json:"snapshot_ref,omitempty"`       // durable ContextSnapshot reference
	ToolResultDigest string       `json:"tool_result_digest,omitempty"` // CAS digest of the durable tool result
	PolicyDecision   string       `json:"policy_decision,omitempty"`
	ApprovalRef      string       `json:"approval_ref,omitempty"`
	At               time.Time    `json:"at"`
}

// DecisionLog is the append-only durable record. It is bounded: when the
// bound is reached, appends are refused rather than silently dropped —
// a durable trace never loses records quietly.
type DecisionLog struct {
	mu      sync.Mutex
	records []DecisionRecord
	max     int // 0 = unbounded
}

func NewDecisionLog(max int) *DecisionLog {
	return &DecisionLog{max: max}
}

// Append seals one record with the next sequence number.
func (l *DecisionLog) Append(rec DecisionRecord) (DecisionRecord, error) {
	if !rec.Kind.valid() {
		return DecisionRecord{}, fmt.Errorf("%w: unknown decision kind %q", ErrInvalidDefinition, rec.Kind)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.max > 0 && len(l.records) >= l.max {
		return DecisionRecord{}, fmt.Errorf("%w: decision log reached its bound of %d records", ErrInvalidDefinition, l.max)
	}
	rec.Seq = int64(len(l.records) + 1)
	if rec.At.IsZero() {
		rec.At = time.Now().UTC()
	}
	l.records = append(l.records, rec)
	return rec, nil
}

// Records returns a copy of the durable trace (inspection/recovery).
func (l *DecisionLog) Records() []DecisionRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]DecisionRecord, len(l.records))
	copy(out, l.records)
	return out
}
