package workflow

import (
	"errors"
	"fmt"
	"time"
)

// The typed workflow-node vocabulary (C4-03): the smallest closed set the
// Blueprint requires. An unknown kind fails closed everywhere — it can
// never be built, validated, or compiled. This vocabulary separates Core
// contracts from any UI editing surface: a canvas may draw anything, but
// only these kinds exist here.
type NodeKind string

const (
	NodeTrigger       NodeKind = "trigger"
	NodeRetrieve      NodeKind = "retrieve"
	NodeLLM           NodeKind = "llm"
	NodeTool          NodeKind = "tool"
	NodePlugin        NodeKind = "plugin"
	NodeTransform     NodeKind = "transform"
	NodeCondition     NodeKind = "condition"
	NodeParallel      NodeKind = "parallel"
	NodeJoin          NodeKind = "join"
	NodeHumanApproval NodeKind = "human_approval"
	NodeWait          NodeKind = "wait"
	NodeCheckpoint    NodeKind = "checkpoint"
	NodeSubworkflow   NodeKind = "subworkflow"
	NodeResult        NodeKind = "result"
)

// DefinitionSchemaVersion and CompilerVersion pin the contract surface that
// revision identities are computed over (C4-04).
const (
	DefinitionSchemaVersion = "wfdef/v1"
	CompilerVersion         = "wfcompile/v1"
)

// Structural bounds (C4-05): bounded graphs prevent recursion and fan-out
// abuse before anything is built.
const (
	MaxNodesPerDefinition = 64
	MaxSubworkflowDepth   = 4
)

// nodeSchemaVersion pins the wire shape of each node kind's input/output
// contracts. A kind without an entry is unknown and fails closed.
var nodeSchemaVersion = map[NodeKind]string{
	NodeTrigger:       "node/v1",
	NodeRetrieve:      "node/v1",
	NodeLLM:           "node/v1",
	NodeTool:          "node/v1",
	NodePlugin:        "node/v1",
	NodeTransform:     "node/v1",
	NodeCondition:     "node/v1",
	NodeParallel:      "node/v1",
	NodeJoin:          "node/v1",
	NodeHumanApproval: "node/v1",
	NodeWait:          "node/v1",
	NodeCheckpoint:    "node/v1",
	NodeSubworkflow:   "node/v1",
	NodeResult:        "node/v1",
}

// KnownNodeKind reports whether the kind is part of the closed vocabulary.
func KnownNodeKind(k NodeKind) bool {
	_, ok := nodeSchemaVersion[k]
	return ok
}

// NodeSchemaVersion returns the pinned schema version for a kind; an
// unknown kind fails closed with a typed error.
func NodeSchemaVersion(k NodeKind) (string, error) {
	v, ok := nodeSchemaVersion[k]
	if !ok {
		return "", fmt.Errorf("workflow: unknown node kind %q (fail closed)", k)
	}
	return v, nil
}

// EffectClass declares what a node does to the outside world (C4-07).
// Effects are declared by the definition author and re-checked by policy;
// they are never inferred from node names alone.
type EffectClass string

const (
	EffectNone     EffectClass = "none"     // pure computation
	EffectLocal    EffectClass = "local"    // local durable state only
	EffectExternal EffectClass = "external" // reaches outside the core
)

// RetryClass is the safe-retry classification (C4-07). Unknown external
// outcomes are NEVER blindly retried: there is no "always retry" class.
type RetryClass string

const (
	RetrySafe   RetryClass = "safe"   // idempotent by construction
	RetryScoped RetryClass = "scoped" // retryable only under an idempotency scope
	RetryNever  RetryClass = "never"  // at-most-once
)

// UnknownOutcomePolicy pins what happens when an effect's outcome is
// unknown (timeout with no receipt). Reconciliation is the only non-fail
// answer; silent continuation is not an option.
type UnknownOutcomePolicy string

const (
	UnknownFail                UnknownOutcomePolicy = "fail"
	UnknownNeedsReconciliation UnknownOutcomePolicy = "needs_reconciliation"
)

// EffectSpec is the complete per-node effect declaration: class, retry
// safety, idempotency-key scope, and unknown-outcome behavior.
type EffectSpec struct {
	Class            EffectClass          `json:"class"`
	Retry            RetryClass           `json:"retry"`
	IdempotencyScope string               `json:"idempotency_scope,omitempty"`
	UnknownOutcome   UnknownOutcomePolicy `json:"unknown_outcome"`
}

// SubworkflowRef pins a child invocation to an immutable revision (C4-04):
// nested references must resolve to registered revisions, never to
// mutable names.
type SubworkflowRef struct {
	WorkflowID string `json:"workflow_id"`
	RevisionID string `json:"revision_id"`
}

// CapabilityRequest is a node's DECLARED request (C4-06). A request is not
// a grant: execution resolves it against Core policy, deny-by-default.
type CapabilityRequest struct {
	Name               string `json:"name"`
	DataClassification string `json:"data_classification,omitempty"`
}

// NodeSpec is one typed node of a definition.
type NodeSpec struct {
	Kind         NodeKind `json:"kind"`
	Name         string   `json:"name"`
	InputSchema  string   `json:"input_schema"`
	OutputSchema string   `json:"output_schema"`

	RequiredCapabilities []CapabilityRequest `json:"required_capabilities,omitempty"`

	Effect           EffectSpec    `json:"effect"`
	Timeout          time.Duration `json:"timeout,omitempty"`
	BudgetTokens     int64         `json:"budget_tokens,omitempty"`
	CheckpointBefore bool          `json:"checkpoint_before,omitempty"`
	// RequiresApproval is forced true for HumanApproval nodes and is a
	// DECLARATION the runtime must honor; it grants nothing by itself.
	RequiresApproval bool `json:"requires_approval,omitempty"`

	Sub SubworkflowRef `json:"subworkflow,omitempty"`
}

// NewEffect builds a fully-specified effect declaration; the envelope rules
// are enforced at validation, not assumed here.
func NewEffect(class EffectClass, retry RetryClass, idempotencyScope string, unknown UnknownOutcomePolicy) EffectSpec {
	return EffectSpec{Class: class, Retry: retry, IdempotencyScope: idempotencyScope, UnknownOutcome: unknown}
}

// validateNode checks one node's internal invariants (C4-03/C4-07): known
// kind, typed schemas, effect/timeout/budget envelope, and subworkflow ref
// placement. Cross-node and cross-definition rules live in Validate.
func validateNode(n NodeSpec) error {
	class := n.Effect.Class
	if class == "" {
		class = EffectNone // a zero effect declaration means "no effect"
	}
	if !KnownNodeKind(n.Kind) {
		return fmt.Errorf("workflow: node %q: unknown kind %q (fail closed)", n.Name, n.Kind)
	}
	if n.Name == "" {
		return fmt.Errorf("workflow: node of kind %q: empty name", n.Kind)
	}
	if n.InputSchema == "" || n.OutputSchema == "" {
		return fmt.Errorf("workflow: node %q: typed input/output schemas are required", n.Name)
	}
	if class != EffectNone {
		if n.Timeout <= 0 {
			return fmt.Errorf("workflow: node %q: effectful nodes require a timeout", n.Name)
		}
		if n.Effect.Retry == RetryScoped && n.Effect.IdempotencyScope == "" {
			return fmt.Errorf("workflow: node %q: scoped retry requires an idempotency scope", n.Name)
		}
		if n.Effect.UnknownOutcome != UnknownFail && n.Effect.UnknownOutcome != UnknownNeedsReconciliation {
			return fmt.Errorf("workflow: node %q: unknown outcome must be fail or needs_reconciliation", n.Name)
		}
	}
	if n.BudgetTokens < 0 {
		return fmt.Errorf("workflow: node %q: negative budget", n.Name)
	}
	if n.Kind == NodeSubworkflow && (n.Sub.WorkflowID == "" || n.Sub.RevisionID == "") {
		return fmt.Errorf("workflow: node %q: subworkflow requires a pinned workflow+revision", n.Name)
	}
	if n.Kind != NodeSubworkflow && (n.Sub.WorkflowID != "" || n.Sub.RevisionID != "") {
		return fmt.Errorf("workflow: node %q: subworkflow ref is only legal on subworkflow nodes", n.Name)
	}
	return nil
}

// Sentinel errors (typed, inspectable; wrap with context at call sites).
var (
	ErrInvalidDefinition = errors.New("workflow: invalid workflow definition")
	ErrRevisionConflict  = errors.New("workflow: revision identity conflict")
	ErrCapabilityDenied  = errors.New("workflow: capability denied by policy")
)
