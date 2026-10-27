package workflow

import (
	"fmt"
)

// The compilation boundary (C4-10): a validated, policy-resolved
// definition compiles to neutral PROPOSED STEP contracts — the durable
// inputs a future execution engine would submit. Mapping those steps onto
// AIJobRelay Jobs happens only under the proposed D-40 ownership model:
// this package creates NO second scheduler, event journal, or retry
// authority, and no Run schema/API freeze occurs until D-40 and D-35
// evidence is accepted.

// ProposedStep is the compiled form of one node: the pinned revision,
// granted capabilities, effect envelope, and resource declarations —
// everything an executor needs, nothing that grants by itself.
type ProposedStep struct {
	Node             string              `json:"node"`
	Kind             NodeKind            `json:"kind"`
	WorkflowID       string              `json:"workflow_id"`
	RevisionID       string              `json:"revision_id"`
	InputSchema      string              `json:"input_schema"`
	OutputSchema     string              `json:"output_schema"`
	Capabilities     []CapabilityRequest `json:"capabilities,omitempty"`
	Effect           EffectSpec          `json:"effect"`
	CheckpointBefore bool                `json:"checkpoint_before,omitempty"`
	RequiresApproval bool                `json:"requires_approval,omitempty"`
	TimeoutSeconds   int64               `json:"timeout_seconds,omitempty"`
	BudgetTokens     int64               `json:"budget_tokens,omitempty"`
	Sub              SubworkflowRef      `json:"subworkflow,omitempty"`
}

// CompiledPlan is the ordered step set of one compiled revision.
type CompiledPlan struct {
	WorkflowID      string         `json:"workflow_id"`
	RevisionID      string         `json:"revision_id"`
	CompilerVersion string         `json:"compiler_version"`
	Entrypoint      string         `json:"entrypoint"`
	Steps           []ProposedStep `json:"steps"`
}

// Compile validates the definition against the registry, resolves
// capabilities through Core policy (deny-by-default; any denial blocks the
// whole compilation), and emits the proposed steps. The policy approver is
// mandatory so grants can never appear from nowhere.
func Compile(r *Registry, d *Definition, policy PolicyInput) (*CompiledPlan, error) {
	if err := AssertPolicyBoundary(policy); err != nil {
		return nil, err
	}
	if err := Validate(r, d); err != nil {
		return nil, err
	}
	granted, denied := ResolveCapabilities(d, policy)
	if len(denied) > 0 {
		first := denied[0]
		return nil, fmt.Errorf("%w: node %q requests %q: %s (%d denial(s) total)",
			ErrCapabilityDenied, first.Node, first.Capability, first.Reason, len(denied))
	}
	plan := &CompiledPlan{
		WorkflowID:      d.WorkflowID,
		RevisionID:      d.RevisionID,
		CompilerVersion: CompilerVersion,
		Entrypoint:      d.Entrypoint,
	}
	for _, n := range d.Nodes {
		step := ProposedStep{
			Node:             n.Name,
			Kind:             n.Kind,
			WorkflowID:       d.WorkflowID,
			RevisionID:       d.RevisionID,
			InputSchema:      n.InputSchema,
			OutputSchema:     n.OutputSchema,
			Capabilities:     granted[n.Name],
			Effect:           n.Effect,
			CheckpointBefore: n.CheckpointBefore,
			RequiresApproval: n.RequiresApproval || n.Kind == NodeHumanApproval,
			TimeoutSeconds:   int64(n.Timeout / 1e9),
			BudgetTokens:     n.BudgetTokens,
			Sub:              n.Sub,
		}
		plan.Steps = append(plan.Steps, step)
	}
	return plan, nil
}
