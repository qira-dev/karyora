package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// Immutable definitions and revision identity (C4-04): the revision ID is
// a content digest over the canonical form of the whole definition.
// Editing or replanning creates a NEW revision; an existing revision can
// never be overwritten; anything that claims a revision ID whose digest
// does not match its content is rejected as an identity conflict. Digests
// pin content; they are NOT described as proof of publisher trust.
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Schema string `json:"schema"` // the typed contract carried on this edge
}

// Definition is one immutable workflow revision.
type Definition struct {
	WorkflowID      string     `json:"workflow_id"`
	RevisionID      string     `json:"revision_id"`
	SchemaVersion   string     `json:"schema_version"`
	CompilerVersion string     `json:"compiler_version"`
	CreatedAt       time.Time  `json:"created_at"`
	Entrypoint      string     `json:"entrypoint"`
	Nodes           []NodeSpec `json:"nodes"`
	Edges           []Edge     `json:"edges"`
}

// canonicalForm returns the deterministic byte form the digest is computed
// over: schema/compiler versions, workflow id, entrypoint, and the nodes
// and edges in a stable (sorted) order so equivalent definitions agree.
func canonicalForm(d Definition) ([]byte, error) {
	nodes := append([]NodeSpec(nil), d.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	edges := append([]Edge(nil), d.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	form := struct {
		SchemaVersion   string     `json:"schema_version"`
		CompilerVersion string     `json:"compiler_version"`
		WorkflowID      string     `json:"workflow_id"`
		Entrypoint      string     `json:"entrypoint"`
		Nodes           []NodeSpec `json:"nodes"`
		Edges           []Edge     `json:"edges"`
	}{d.SchemaVersion, d.CompilerVersion, d.WorkflowID, d.Entrypoint, nodes, edges}
	return events.CanonicalJSON(form)
}

func digestOf(form []byte) string {
	sum := sha256.Sum256(form)
	return "wfreq-" + hex.EncodeToString(sum[:16])
}

// Build assembles and seals one immutable revision: it validates node-level
// invariants and name uniqueness, then computes the content digest. The
// caller still runs Validate (cross-node rules) before compiling.
func Build(workflowID, entrypoint string, createdAt time.Time, nodes []NodeSpec, edges []Edge) (*Definition, error) {
	if workflowID == "" || entrypoint == "" {
		return nil, fmt.Errorf("%w: workflow id and entrypoint are required", ErrInvalidDefinition)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("%w: a definition needs at least one node", ErrInvalidDefinition)
	}
	if len(nodes) > MaxNodesPerDefinition {
		return nil, fmt.Errorf("%w: %d nodes exceeds the bound of %d", ErrInvalidDefinition, len(nodes), MaxNodesPerDefinition)
	}
	seen := map[string]bool{}
	for i := range nodes {
		// Normalize zero effect declarations so equivalent definitions
		// always digest identically (C4-04 determinism).
		if nodes[i].Effect.Class == "" {
			nodes[i].Effect.Class = EffectNone
			if nodes[i].Effect.UnknownOutcome == "" {
				nodes[i].Effect.UnknownOutcome = UnknownFail
			}
		}
	}
	for _, n := range nodes {
		if seen[n.Name] {
			return nil, fmt.Errorf("%w: duplicate node name %q", ErrInvalidDefinition, n.Name)
		}
		seen[n.Name] = true
		if err := validateNode(n); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
		}
	}
	if !seen[entrypoint] {
		return nil, fmt.Errorf("%w: entrypoint %q is not a node", ErrInvalidDefinition, entrypoint)
	}
	d := &Definition{
		WorkflowID:      workflowID,
		SchemaVersion:   DefinitionSchemaVersion,
		CompilerVersion: CompilerVersion,
		CreatedAt:       createdAt.UTC(),
		Entrypoint:      entrypoint,
		Nodes:           append([]NodeSpec(nil), nodes...),
		Edges:           append([]Edge(nil), edges...),
	}
	for i := range d.Nodes {
		if d.Nodes[i].Kind == NodeHumanApproval {
			d.Nodes[i].RequiresApproval = true // forced, never author-set
		}
	}
	form, err := canonicalForm(*d)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical form failed: %v", ErrInvalidDefinition, err)
	}
	d.RevisionID = digestOf(form)
	return d, nil
}

// Node returns the node with the given name.
func (d *Definition) Node(name string) (NodeSpec, bool) {
	for _, n := range d.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return NodeSpec{}, false
}

// Registry stores immutable revisions keyed by (workflow, revision). A
// re-registration of identical content is a no-op; anything that claims an
// existing identity with different content is an identity conflict and is
// rejected — this is the overwrite-rejection guarantee (C4-04).
type Registry struct {
	mu   sync.Mutex
	byID map[string]*Definition
}

func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Definition{}}
}

func key(workflowID, revisionID string) string { return workflowID + "|" + revisionID }

func (r *Registry) Register(d *Definition) error {
	if d == nil || d.RevisionID == "" {
		return fmt.Errorf("%w: nil or unsealed definition", ErrInvalidDefinition)
	}
	form, err := canonicalForm(*d)
	if err != nil {
		return fmt.Errorf("%w: canonical form failed: %v", ErrInvalidDefinition, err)
	}
	if got := digestOf(form); got != d.RevisionID {
		return fmt.Errorf("%w: claimed revision %s does not match content digest %s (overwrite rejected)", ErrRevisionConflict, d.RevisionID, got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(d.WorkflowID, d.RevisionID)
	if _, ok := r.byID[k]; ok {
		return nil // identical content already registered (digest-verified)
	}
	r.byID[k] = d
	return nil
}

// Lookup returns a pinned revision. Active runs pin exactly this identity.
func (r *Registry) Lookup(workflowID, revisionID string) (*Definition, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[key(workflowID, revisionID)]
	return d, ok
}

// Revise seals a NEW immutable revision from modified parts; the base
// revision and every registered revision are untouched (C4-08).
func (r *Registry) Revise(workflowID, entrypoint string, createdAt time.Time, nodes []NodeSpec, edges []Edge) (*Definition, error) {
	return Build(workflowID, entrypoint, createdAt, nodes, edges)
}
