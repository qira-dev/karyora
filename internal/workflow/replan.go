package workflow

import (
	"fmt"
)

// Replan and active-revision compatibility (C4-08): an AI-proposed replan
// is diffed and sealed as a NEW revision — never a mutation of history —
// and running/paused executions continue against their PINNED revision.
// No claim is made that a model produces deterministic replans; the diff
// exists precisely so a human or policy can review what changed.

// RevisionDiff is the node-level difference between two revisions of one
// workflow, by safe node names.
type RevisionDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// Empty reports whether the two revisions carry identical node content.
func (d RevisionDiff) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Diff compares two revisions of the same workflow. Revisions of different
// workflows are not comparable and yield an error.
func Diff(base, next *Definition) (RevisionDiff, error) {
	if base.WorkflowID != next.WorkflowID {
		return RevisionDiff{}, fmt.Errorf("%w: cannot diff revisions of different workflows (%s vs %s)",
			ErrInvalidDefinition, base.WorkflowID, next.WorkflowID)
	}
	oldNodes := map[string]NodeSpec{}
	for _, n := range base.Nodes {
		oldNodes[n.Name] = n
	}
	var diff RevisionDiff
	for _, n := range next.Nodes {
		old, ok := oldNodes[n.Name]
		if !ok {
			diff.Added = append(diff.Added, n.Name)
			continue
		}
		delete(oldNodes, n.Name)
		if !sameNode(old, n) {
			diff.Changed = append(diff.Changed, n.Name)
		}
	}
	for name := range oldNodes {
		diff.Removed = append(diff.Removed, name)
	}
	return diff, nil
}

// sameNode compares the fields that affect execution semantics: kind,
// schemas, capability requests, effect envelope, timeout/budget,
// checkpoint/approval, and subworkflow pin.
func sameNode(a, b NodeSpec) bool {
	if a.Kind != b.Kind || a.InputSchema != b.InputSchema || a.OutputSchema != b.OutputSchema {
		return false
	}
	if a.Effect != b.Effect || a.Timeout != b.Timeout || a.BudgetTokens != b.BudgetTokens {
		return false
	}
	if a.CheckpointBefore != b.CheckpointBefore || a.RequiresApproval != b.RequiresApproval || a.Sub != b.Sub {
		return false
	}
	if len(a.RequiredCapabilities) != len(b.RequiredCapabilities) {
		return false
	}
	seen := map[CapabilityRequest]bool{}
	for _, r := range a.RequiredCapabilities {
		seen[r] = true
	}
	for _, r := range b.RequiredCapabilities {
		if !seen[r] {
			return false
		}
	}
	return true
}

// MigrationAllowed decides whether an execution may MOVE from a pinned
// revision to a successor revision. The conservative rule (v1): only when
// the workflow, definition schema version, and every SHARED node's entire
// execution semantics are identical — structural changes never migrate;
// they only run in new executions against the new revision. Anything else
// is refused with a typed reason.
func MigrationAllowed(base, next *Definition) error {
	if base.WorkflowID != next.WorkflowID {
		return fmt.Errorf("%w: cannot migrate across workflows", ErrInvalidDefinition)
	}
	if base.SchemaVersion != next.SchemaVersion {
		return fmt.Errorf("%w: definition schema version changed (%s -> %s); migration refused",
			ErrInvalidDefinition, base.SchemaVersion, next.SchemaVersion)
	}
	diff, err := Diff(base, next)
	if err != nil {
		return err
	}
	// Added or removed nodes change the graph a live execution walks; the
	// conservative rule refuses them outright.
	if !diff.Empty() {
		return fmt.Errorf("%w: revision changed nodes (added=%v removed=%v changed=%v); a pinned execution finishes on its own revision and the successor runs only in new executions",
			ErrInvalidDefinition, diff.Added, diff.Removed, diff.Changed)
	}
	return nil
}
