package workflow

import (
	"fmt"
)

// Static graph and subworkflow validation (C4-05): everything here runs
// BEFORE execution — an invalid or unsupported graph never creates a Job
// or an external effect. Validation errors identify the node by its safe
// declared name; no secret material ever enters a definition, so errors
// cannot leak any.

// Validate applies every cross-node rule to a sealed definition: typed
// edges, acyclicity, reachability, parallel/join pairing, effect
// envelopes, and subworkflow references resolving to registered immutable
// revisions with bounded depth.
func Validate(r *Registry, d *Definition) error {
	if d == nil {
		return fmt.Errorf("%w: nil definition", ErrInvalidDefinition)
	}
	nodes := map[string]NodeSpec{}
	for _, n := range d.Nodes {
		if err := validateNode(n); err != nil {
			return err
		}
		nodes[n.Name] = n
	}
	// Typed edges: both ends exist, no self-edge, and the carried schema
	// must match the source's output and the target's input exactly.
	outgoing := map[string][]string{}
	incoming := map[string][]string{}
	for _, e := range d.Edges {
		from, ok := nodes[e.From]
		if !ok {
			return fmt.Errorf("%w: edge from unknown node %q", ErrInvalidDefinition, e.From)
		}
		to, ok := nodes[e.To]
		if !ok {
			return fmt.Errorf("%w: edge to unknown node %q", ErrInvalidDefinition, e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("%w: node %q has a self-edge", ErrInvalidDefinition, e.From)
		}
		if e.Schema != from.OutputSchema || e.Schema != to.InputSchema {
			return fmt.Errorf("%w: edge %q->%q carries schema %q but nodes declare %q/%q",
				ErrInvalidDefinition, e.From, e.To, e.Schema, from.OutputSchema, to.InputSchema)
		}
		outgoing[e.From] = append(outgoing[e.From], e.To)
		incoming[e.To] = append(incoming[e.To], e.From)
	}
	// Reachability from the entrypoint: an unreachable node can never run
	// and only ever signals authoring mistakes.
	reachable := map[string]bool{d.Entrypoint: true}
	queue := []string{d.Entrypoint}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range outgoing[cur] {
			if !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	for _, n := range d.Nodes {
		if !reachable[n.Name] {
			return fmt.Errorf("%w: node %q is unreachable from the entrypoint", ErrInvalidDefinition, n.Name)
		}
	}
	// Acyclicity: DFS with a path set (reachability alone cannot prove it).
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		color[name] = gray
		for _, next := range outgoing[name] {
			switch color[next] {
			case gray:
				return fmt.Errorf("%w: cycle through node %q", ErrInvalidDefinition, next)
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		color[name] = black
		return nil
	}
	for _, n := range d.Nodes {
		if color[n.Name] == white {
			if err := visit(n.Name); err != nil {
				return err
			}
		}
	}
	// Parallel/join pairing (C4-05): a parallel fan-out must meet a join,
	// and a join must be fed by at least one parallel branch.
	for _, n := range d.Nodes {
		switch n.Kind {
		case NodeParallel:
			found := false
			for _, next := range outgoing[n.Name] {
				if nodes[next].Kind == NodeJoin {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: parallel node %q has no direct join successor", ErrInvalidDefinition, n.Name)
			}
		case NodeJoin:
			found := false
			for _, prev := range incoming[n.Name] {
				if nodes[prev].Kind == NodeParallel {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: join node %q has no parallel input", ErrInvalidDefinition, n.Name)
			}
		}
	}
	// Subworkflow references: resolve to registered immutable revisions,
	// reject direct self-reference, and bound the nesting depth.
	for _, n := range d.Nodes {
		if n.Kind != NodeSubworkflow {
			continue
		}
		if n.Sub.WorkflowID == d.WorkflowID && n.Sub.RevisionID == d.RevisionID {
			return fmt.Errorf("%w: node %q references its own revision (direct recursion)", ErrInvalidDefinition, n.Name)
		}
		child, ok := r.Lookup(n.Sub.WorkflowID, n.Sub.RevisionID)
		if !ok {
			return fmt.Errorf("%w: node %q references unregistered revision %s/%s (fail closed)",
				ErrInvalidDefinition, n.Name, n.Sub.WorkflowID, n.Sub.RevisionID)
		}
		if err := validateSubworkflowDepth(r, child, map[string]bool{key(d.WorkflowID, d.RevisionID): true}, 1); err != nil {
			return err
		}
	}
	return nil
}

// validateSubworkflowDepth walks the referenced chain with a path set so
// indirect recursion is caught, bounded by MaxSubworkflowDepth.
func validateSubworkflowDepth(r *Registry, d *Definition, path map[string]bool, depth int) error {
	if depth > MaxSubworkflowDepth {
		return fmt.Errorf("%w: subworkflow nesting exceeds depth %d", ErrInvalidDefinition, MaxSubworkflowDepth)
	}
	k := key(d.WorkflowID, d.RevisionID)
	if path[k] {
		return fmt.Errorf("%w: subworkflow cycle at %s", ErrInvalidDefinition, k)
	}
	path[k] = true
	defer delete(path, k)
	for _, n := range d.Nodes {
		if n.Kind != NodeSubworkflow {
			continue
		}
		child, ok := r.Lookup(n.Sub.WorkflowID, n.Sub.RevisionID)
		if !ok {
			return fmt.Errorf("%w: subworkflow %s references unregistered revision %s/%s",
				ErrInvalidDefinition, k, n.Sub.WorkflowID, n.Sub.RevisionID)
		}
		if err := validateSubworkflowDepth(r, child, path, depth+1); err != nil {
			return err
		}
	}
	return nil
}
