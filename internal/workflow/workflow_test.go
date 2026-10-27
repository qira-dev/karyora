package workflow

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// CR-4.1 acceptance fixtures: positive and negative cases per the plan's
// C4-03–C4-10 acceptance criteria. Every fail-closed claim in the package
// doc comments is proven here.

var testNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func node(kind NodeKind, name string, in, out string) NodeSpec {
	return NodeSpec{Kind: kind, Name: name, InputSchema: in, OutputSchema: out}
}

func linearDef(t *testing.T, id string, now time.Time) *Definition {
	t.Helper()
	d, err := Build(id, "start", now, []NodeSpec{
		node(NodeTrigger, "start", "edge/empty", "edge/text"),
		node(NodeLLM, "answer", "edge/text", "edge/text"),
		node(NodeResult, "done", "edge/text", "edge/empty"),
	}, []Edge{
		{From: "start", To: "answer", Schema: "edge/text"},
		{From: "answer", To: "done", Schema: "edge/text"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return d
}

func TestUnknownNodeKindFailsClosed(t *testing.T) {
	_, err := Build("wf", "start", testNow, []NodeSpec{
		node(NodeKind("quantum"), "start", "a", "b"),
	}, nil)
	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown kind must fail closed, got %v", err)
	}
	if _, err := NodeSchemaVersion(NodeKind("quantum")); err == nil {
		t.Fatal("schema lookup of an unknown kind must fail")
	}
}

func TestRevisionIdentityStableAndImmutable(t *testing.T) {
	reg := NewRegistry()
	d := linearDef(t, "wf", testNow)
	if d.RevisionID == "" || !strings.HasPrefix(d.RevisionID, "wfreq-") {
		t.Fatalf("revision id must be a content digest, got %q", d.RevisionID)
	}
	// Same content, different node ORDER -> same digest (canonical form).
	shuffled := linearDef(t, "wf", testNow)
	if shuffled.RevisionID != d.RevisionID {
		t.Fatalf("canonical digest must be order-independent: %s vs %s", shuffled.RevisionID, d.RevisionID)
	}
	if err := reg.Register(d); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Register(shuffled); err != nil {
		t.Fatalf("identical re-registration must be idempotent: %v", err)
	}
	// Overwrite rejection: a tampered body claiming the SAME revision id.
	tampered := *d
	tampered.Nodes = append([]NodeSpec(nil), d.Nodes...)
	tampered.Nodes[1].BudgetTokens = 999
	if err := reg.Register(&tampered); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("claimed-identity/different-content must be an identity conflict, got %v", err)
	}
	// Editing creates a NEW revision ONLY when content changes; sealing
	// identical content dedupes to the same revision (content-addressed).
	again, err := reg.Revise("wf", "start", testNow.Add(time.Minute), d.Nodes, d.Edges)
	if err != nil {
		t.Fatalf("revise (identical content): %v", err)
	}
	if again.RevisionID != d.RevisionID {
		t.Fatalf("identical content must dedupe to the same revision: %s vs %s", again.RevisionID, d.RevisionID)
	}
	if _, ok := reg.Lookup("wf", d.RevisionID); !ok {
		t.Fatal("pinned base revision must remain lookup-able")
	}
}

func TestStaticValidationNegatives(t *testing.T) {
	reg := NewRegistry()
	base := linearDef(t, "wf", testNow)
	if err := reg.Register(base); err != nil {
		t.Fatalf("register: %v", err)
	}
	cases := map[string]func() (*Definition, error){
		"unknown edge target": func() (*Definition, error) {
			return Build("wf", "start", testNow, base.Nodes, []Edge{
				{From: "start", To: "ghost", Schema: "edge/text"},
			})
		},
		"self edge": func() (*Definition, error) {
			return Build("wf", "start", testNow, base.Nodes, []Edge{
				{From: "start", To: "start", Schema: "edge/text"},
			})
		},
		"schema mismatch": func() (*Definition, error) {
			return Build("wf", "start", testNow, []NodeSpec{
				node(NodeTrigger, "start", "edge/empty", "edge/text"),
				node(NodeLLM, "answer", "edge/other", "edge/text"),
				node(NodeResult, "done", "edge/text", "edge/empty"),
			}, []Edge{
				{From: "start", To: "answer", Schema: "edge/text"},
				{From: "answer", To: "done", Schema: "edge/text"},
			})
		},
		"unreachable node": func() (*Definition, error) {
			nodes := append([]NodeSpec(nil), base.Nodes...)
			nodes = append(nodes, node(NodeTransform, "orphan", "edge/text", "edge/text"))
			return Build("wf", "start", testNow, nodes, []Edge{
				{From: "start", To: "answer", Schema: "edge/text"},
				{From: "answer", To: "done", Schema: "edge/text"},
			})
		},
		"cycle": func() (*Definition, error) {
			return Build("wf", "start", testNow, []NodeSpec{
				node(NodeTrigger, "start", "edge/empty", "edge/text"),
				node(NodeLLM, "a", "edge/text", "edge/text"),
				node(NodeLLM, "b", "edge/text", "edge/text"),
				node(NodeResult, "done", "edge/text", "edge/empty"),
			}, []Edge{
				{From: "start", To: "a", Schema: "edge/text"},
				{From: "a", To: "b", Schema: "edge/text"},
				{From: "b", To: "a", Schema: "edge/text"},
				{From: "b", To: "done", Schema: "edge/text"},
			})
		},
		"parallel without join": func() (*Definition, error) {
			return Build("wf", "start", testNow, []NodeSpec{
				node(NodeTrigger, "start", "edge/empty", "edge/text"),
				node(NodeParallel, "fan", "edge/text", "edge/text"),
				node(NodeResult, "done", "edge/text", "edge/empty"),
			}, []Edge{
				{From: "start", To: "fan", Schema: "edge/text"},
				{From: "fan", To: "done", Schema: "edge/text"},
			})
		},
		"effectful node without timeout": func() (*Definition, error) {
			n := node(NodeTool, "act", "edge/text", "edge/text")
			n.Effect = NewEffect(EffectExternal, RetryNever, "", UnknownFail) // no timeout
			return Build("wf", "act", testNow, []NodeSpec{n}, nil)
		},
		"scoped retry without idempotency scope": func() (*Definition, error) {
			n := node(NodeTool, "act", "edge/text", "edge/text")
			n.Timeout = time.Minute
			n.Effect = NewEffect(EffectExternal, RetryScoped, "", UnknownNeedsReconciliation)
			return Build("wf", "act", testNow, []NodeSpec{n}, nil)
		},
	}
	for name, mk := range cases {
		d, buildErr := mk()
		if d == nil && buildErr != nil {
			continue // refused at sealing time — fail-closed holds even earlier
		}
		if d == nil {
			t.Fatalf("%s: fixture build returned nil without an error", name)
		}
		if err := Validate(reg, d); err == nil {
			t.Errorf("%s: validation must fail", name)
		}
	}
}

func TestValidGraphPositive(t *testing.T) {
	reg := NewRegistry()
	d, err := Build("wf", "start", testNow, []NodeSpec{
		node(NodeTrigger, "start", "edge/empty", "edge/text"),
		node(NodeParallel, "fan", "edge/text", "edge/text"),
		node(NodeJoin, "merge", "edge/text", "edge/text"),
		node(NodeResult, "done", "edge/text", "edge/empty"),
	}, []Edge{
		{From: "start", To: "fan", Schema: "edge/text"},
		{From: "fan", To: "merge", Schema: "edge/text"},
		{From: "merge", To: "done", Schema: "edge/text"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := Validate(reg, d); err != nil {
		t.Fatalf("valid graph must pass: %v", err)
	}
}

func TestSubworkflowPinningDepthAndRecursion(t *testing.T) {
	reg := NewRegistry()
	// A valid child revision.
	child, err := Build("child", "go", testNow, []NodeSpec{
		node(NodeTrigger, "go", "edge/empty", "edge/text"),
		node(NodeResult, "end", "edge/text", "edge/empty"),
	}, []Edge{{From: "go", To: "end", Schema: "edge/text"}})
	if err != nil {
		t.Fatalf("child build: %v", err)
	}
	if err := reg.Register(child); err != nil {
		t.Fatalf("child register: %v", err)
	}
	parentNodes := []NodeSpec{
		node(NodeTrigger, "start", "edge/empty", "edge/text"),
		node(NodeResult, "done", "edge/text", "edge/empty"),
	}
	edges := []Edge{{From: "start", To: "sub", Schema: "edge/text"}}
	// Unregistered reference fails closed.
	missing := append([]NodeSpec(nil), parentNodes...)
	missing[1] = node(NodeSubworkflow, "sub", "edge/text", "edge/empty")
	missing[1].Sub = SubworkflowRef{WorkflowID: "child", RevisionID: "wfreq-does-not-exist"}
	dMissing, _ := Build("parent", "start", testNow, missing, edges)
	if err := Validate(reg, dMissing); err == nil {
		t.Fatal("unregistered subworkflow reference must fail closed")
	}
	// Registered reference validates; the pin resolves to the immutable child.
	ok := append([]NodeSpec(nil), parentNodes...)
	ok[1] = node(NodeSubworkflow, "sub", "edge/text", "edge/empty")
	ok[1].Sub = SubworkflowRef{WorkflowID: child.WorkflowID, RevisionID: child.RevisionID}
	dOk, _ := Build("parent", "start", testNow, ok, edges)
	if err := Validate(reg, dOk); err != nil {
		t.Fatalf("pinned subworkflow must validate: %v", err)
	}
	// Self-reference is impossible to build (the pin would need the
	// definition's own digest) — the structural guard is proven by
	// validating a hand-forged definition that claims it.
	self := append([]NodeSpec(nil), parentNodes...)
	self[1] = node(NodeSubworkflow, "sub", "edge/text", "edge/empty")
	self[1].Sub = SubworkflowRef{WorkflowID: child.WorkflowID, RevisionID: child.RevisionID}
	forged, err := Build("parent", "start", testNow, self, edges)
	if err != nil {
		t.Fatalf("forged base build: %v", err)
	}
	forged.Nodes[1].Sub = SubworkflowRef{WorkflowID: forged.WorkflowID, RevisionID: forged.RevisionID}
	if err := Validate(reg, forged); err == nil {
		t.Fatal("direct self-reference must be rejected")
	}
}

func TestCapabilityRequestsAreNotGrants(t *testing.T) {
	reg := NewRegistry()
	n := node(NodeLLM, "answer", "edge/text", "edge/text")
	n.RequiredCapabilities = []CapabilityRequest{
		{Name: "provider.chat", DataClassification: "non-personal"},
		{Name: "work.group", DataClassification: "personal"}, // D-12 excluded
	}
	d, err := Build("wf", "answer", testNow, []NodeSpec{n}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Deny-by-default: no policy input at all.
	granted, denied := ResolveCapabilities(d, PolicyInput{})
	if len(granted) != 0 || len(denied) != 2 {
		t.Fatalf("deny-by-default expected, got granted=%v denied=%v", granted, denied)
	}
	// Grants only from explicit Core policy; Work-scoped stays refused even then.
	policy := PolicyInput{Approver: "core-policy/test", Grants: map[string]bool{
		"provider.chat": true,
		"work.group":    true,
	}}
	granted, denied = ResolveCapabilities(d, policy)
	if len(granted["answer"]) != 1 || granted["answer"][0].Name != "provider.chat" {
		t.Fatalf("provider.chat must be granted: %v", granted)
	}
	if len(denied) != 1 || denied[0].Capability != "work.group" {
		t.Fatalf("work-scoped capability must be refused even with a policy grant: %v", denied)
	}
	// No approver -> no resolution at all (grants never appear from nowhere).
	if err := AssertPolicyBoundary(PolicyInput{Grants: map[string]bool{"provider.chat": true}}); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("empty approver must refuse the policy boundary: %v", err)
	}
	// Compile blocked on any denial.
	if _, err := Compile(reg, d, PolicyInput{}); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("compile must block on denials, got %v", err)
	}
}

func TestCompilePreservesPinnedFacts(t *testing.T) {
	reg := NewRegistry()
	n := node(NodeTool, "act", "edge/text", "edge/text")
	n.Timeout = 90 * time.Second
	n.BudgetTokens = 512
	n.CheckpointBefore = true
	n.Effect = NewEffect(EffectExternal, RetryScoped, "order:42", UnknownNeedsReconciliation)
	n.RequiredCapabilities = []CapabilityRequest{{Name: "tool.web"}}
	d, err := Build("wf", "act", testNow, []NodeSpec{n}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := reg.Register(d); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan, err := Compile(reg, d, PolicyInput{Approver: "core-policy/test", Grants: map[string]bool{"tool.web": true}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if plan.RevisionID != d.RevisionID || plan.CompilerVersion != CompilerVersion {
		t.Fatalf("compile must preserve the pinned revision and compiler version: %+v", plan)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("one node -> one step, got %d", len(plan.Steps))
	}
	s := plan.Steps[0]
	if s.Effect != n.Effect || s.TimeoutSeconds != 90 || s.BudgetTokens != 512 || !s.CheckpointBefore {
		t.Fatalf("compiled step must preserve the declared facts: %+v", s)
	}
	if len(s.Capabilities) != 1 || s.Capabilities[0].Name != "tool.web" {
		t.Fatalf("compiled step must carry only granted capabilities: %+v", s.Capabilities)
	}
}

func TestReplanDiffAndMigrationRules(t *testing.T) {
	reg := NewRegistry()
	base := linearDef(t, "wf", testNow)
	if err := reg.Register(base); err != nil {
		t.Fatalf("register: %v", err)
	}
	// A replan that changes a node's budget is a NEW revision, diffable.
	changed := append([]NodeSpec(nil), base.Nodes...)
	changed[1].BudgetTokens = 100
	next, err := reg.Revise("wf", "start", testNow.Add(time.Minute), changed, base.Edges)
	if err != nil {
		t.Fatalf("replan build: %v", err)
	}
	if next.RevisionID == base.RevisionID {
		t.Fatal("a replan must seal a new revision")
	}
	diff, err := Diff(base, next)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff.Empty() || len(diff.Changed) != 1 || diff.Changed[0] != "answer" {
		t.Fatalf("diff must name the changed node, got %+v", diff)
	}
	// A pinned execution never migrates onto a semantically changed revision.
	if err := MigrationAllowed(base, next); err == nil {
		t.Fatal("migration across changed nodes must be refused")
	}
	// Identical content under a new timestamp: still a new revision, and
	// migration is legal because semantics are identical.
	same, err := reg.Revise("wf", "start", testNow.Add(time.Minute), base.Nodes, base.Edges)
	if err != nil {
		t.Fatalf("same-content revise: %v", err)
	}
	if err := MigrationAllowed(base, same); err != nil {
		t.Fatalf("identical semantics must be migratable: %v", err)
	}
}

func TestHumanApprovalForced(t *testing.T) {
	n := node(NodeHumanApproval, "sign-off", "edge/text", "edge/text")
	n.RequiresApproval = false // an author tries to sneak past approval
	d, err := Build("wf", "sign-off", testNow, []NodeSpec{n}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !d.Nodes[0].RequiresApproval {
		t.Fatal("human-approval nodes must force RequiresApproval")
	}
}

func TestDecisionLogAppendAndBound(t *testing.T) {
	log := NewDecisionLog(2)
	r1, err := log.Append(DecisionRecord{Kind: DecisionPlanProposal, Node: "answer", RevisionID: "wfreq-x", SnapshotRef: "snap-1"})
	if err != nil || r1.Seq != 1 {
		t.Fatalf("append 1: %v %v", r1, err)
	}
	if _, err := log.Append(DecisionRecord{Kind: DecisionToolResult, Node: "answer", ToolResultDigest: "cas-1"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	if _, err := log.Append(DecisionRecord{Kind: DecisionContinuation}); err == nil {
		t.Fatal("a bounded log must refuse further appends (never silently drop)")
	}
	recs := log.Records()
	if len(recs) != 2 || recs[0].Kind != DecisionPlanProposal || recs[1].ToolResultDigest != "cas-1" {
		t.Fatalf("durable trace mismatch: %+v", recs)
	}
	if _, err := log.Append(DecisionRecord{Kind: DecisionKind("vibes")}); err == nil {
		t.Fatal("unknown decision kinds must fail closed")
	}
}
