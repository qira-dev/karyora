package app

import (
	stdcontext "context"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	workflowdomain "github.com/qira-dev/karyora/internal/domain/workflow"
)

// The B4 Workflow authoring family's service register (DE-0 queue row 6):
// the aggregate-create class (the Core mints the identity — the body never
// carries one), the Core-authoritative typed validation (client checks are
// ADVISORY — the local result is never a truth source; this operation IS
// the Core's authoritative side; it mutates no stored revision and never
// silently saves the draft), the M-21 save class (the NEW revision stated
// on save; the typed rejection preserves the draft; the interim conflict
// rule governs the concurrent-revision outcome), the M-23 AI-diff propose
// member (the AI-PROPOSAL LAW: no Core write, no stored artifact, nothing
// fabricated), the run-history LINKAGE read (the D-40 gate's honest empty
// page), and the authorization boundary through the one decision port.

// newWorkflowEnv is newTestEnv with the workflow port wired to the same
// store (the test-side twin of the composition root's wiring).
func newWorkflowEnv(t testing.TB) (*testEnv, *Slice) {
	t.Helper()
	env := newTestEnv(t)
	cdeps := env.deps
	cdeps.Workflow = env.store
	cslice, err := NewSlice(cdeps)
	if err != nil {
		t.Fatalf("workflow slice: %v", err)
	}
	return env, cslice
}

// workflowFixtureDefinition creates one definition through the service
// (the Core-minted identity is the fixture's anchor).
func workflowFixtureDefinition(t *testing.T, slice *Slice, ctx stdcontext.Context, key string) workflowdomain.Definition {
	t.Helper()
	res, err := slice.CreateWorkflowDefinition(ctx, WorkflowCreateRequest{
		IdempotencyKey: "key-" + key, CorrelationID: "corr-" + key, DisplayName: "the fixture workflow " + key,
	})
	if err != nil {
		t.Fatalf("fixture create: %v", err)
	}
	return res.Definition
}

// workflowValidDraft builds a structurally valid draft over the Core's own
// catalogue types.
func workflowValidDraft() workflowdomain.DraftContent {
	return workflowdomain.DraftContent{
		Nodes: []workflowdomain.DraftNode{
			{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{}},
			{NodeRef: "n2", NodeType: "result", Config: map[string]any{"k": "v"}},
		},
		Edges: []workflowdomain.DraftEdge{
			{EdgeRef: "e1", FromNode: "n1", ToNode: "n2"},
		},
	}
}

// TestWorkflowAuthzBoundary pins the authorization boundary: every B4
// surface refuses the session-less caller with the typed
// AUTHZ_DENIED/SESSION_INVALID envelope through the one decision port —
// and never the data. Local mode stays role-free (no permission policy is
// invented).
func TestWorkflowAuthzBoundary(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	noSession := WithCaller(stdcontext.Background(), "local", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"create", func() error {
			_, err := slice.CreateWorkflowDefinition(noSession, WorkflowCreateRequest{IdempotencyKey: "k", CorrelationID: "c", DisplayName: "d"})
			return err
		}},
		{"validate", func() error {
			_, err := slice.ValidateWorkflowDraft(noSession, WorkflowValidateRequest{CorrelationID: "c", Draft: workflowValidDraft()})
			return err
		}},
		{"ai propose", func() error {
			_, err := slice.ProposeWorkflowAIDiff(noSession, WorkflowAIDiffProposeRequest{CorrelationID: "c", Instruction: "i"})
			return err
		}},
		{"save revision", func() error {
			_, err := slice.SaveWorkflowRevision(noSession, "wfdef-x", WorkflowRevisionSaveRequest{
				IdempotencyKey: "k", CorrelationID: "c", Draft: workflowValidDraft(),
			})
			return err
		}},
		{"list", func() error { _, err := slice.WorkflowDefinitions(noSession, WorkflowListRequest{}); return err }},
		{"inspect", func() error { _, err := slice.WorkflowDefinitionInspect(noSession, "wfdef-x"); return err }},
		{"run history", func() error { _, err := slice.WorkflowRunHistory(noSession, "wfdef-x"); return err }},
		{"state", func() error { _, err := slice.WorkflowDefinitionState(noSession, "wfdef-x", ""); return err }},
	}
	for _, tc := range cases {
		err := tc.call()
		envl := assertKnowledgeEnvelope(t, err, errs.CodeAuthzDenied)
		if envl.Data["reason"] != "SESSION_INVALID" {
			t.Fatalf("%s: the session boundary refusal: %+v", tc.name, envl.Data)
		}
	}
	// One authenticated positive (the state read's typed 404 — the
	// boundary, not the router, answers, and the code is the envelope
	// register's typed carrier).
	_, err := slice.WorkflowDefinitionState(env.ctx(), "wfdef-authz-ok", "")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
}

// TestWorkflowCreateClass pins the aggregate-create class through the
// service: the Core mints the definition identity (the body never carries
// one); the keyed replay (72 h post-terminal) returns the recorded
// identity, never a second one; the same key with a different request
// identity is the typed CONFLICT_IDEMPOTENCY_KEY; the member validation
// rides the bounded-identity law.
func TestWorkflowCreateClass(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	ctx := env.ctx()

	first, err := slice.CreateWorkflowDefinition(ctx, WorkflowCreateRequest{
		IdempotencyKey: "key-c1", CorrelationID: "corr-c1", DisplayName: "the created workflow",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if first.Replayed || first.Definition.DefinitionID == "" || first.Definition.DisplayName != "the created workflow" {
		t.Fatalf("the Core-confirmed create: %+v", first)
	}
	if first.Definition.CurrentRevisionRef != "" {
		t.Fatalf("a definition with no saved revision carries none: %+v", first.Definition)
	}

	// The keyed replay: the recorded Core-minted identity, never a second
	// one.
	replay, err := slice.CreateWorkflowDefinition(ctx, WorkflowCreateRequest{
		IdempotencyKey: "key-c1", CorrelationID: "corr-c1", DisplayName: "A DIFFERENT body is still the replayed identity",
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replay.Replayed || replay.Definition.DefinitionID != first.Definition.DefinitionID {
		t.Fatalf("the replay returns the recorded identity: %+v", replay)
	}

	// The same key with a different request identity: the typed
	// CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false).
	_, err = slice.CreateWorkflowDefinition(ctx, WorkflowCreateRequest{
		IdempotencyKey: "key-c1", CorrelationID: "corr-OTHER", DisplayName: "d",
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeConflictIdempotencyKey)
	if envl.Data["fingerprintMatch"] != false {
		t.Fatalf("the conflict datum: %+v", envl.Data)
	}

	// The member validation table (the bounded-identity law).
	for _, tc := range []struct {
		name string
		req  WorkflowCreateRequest
	}{
		{"missing key", WorkflowCreateRequest{CorrelationID: "c", DisplayName: "d"}},
		{"missing correlation", WorkflowCreateRequest{IdempotencyKey: "k", DisplayName: "d"}},
		{"missing display name", WorkflowCreateRequest{IdempotencyKey: "k", CorrelationID: "c"}},
		{"oversized display name", WorkflowCreateRequest{IdempotencyKey: "k", CorrelationID: "c", DisplayName: string(make([]byte, 600))}},
	} {
		if _, err := slice.CreateWorkflowDefinition(ctx, tc.req); err == nil {
			t.Fatalf("%s: the bounded-identity law must refuse", tc.name)
		} else if ee, ok := err.(*EnvelopeError); !ok || ee.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: expected VALIDATION_FAILED, got %v", tc.name, err)
		}
	}
}

// TestWorkflowValidateAuthoritative pins the Core-authoritative typed
// validation: the result is Core-owned typed output (the UI invents no
// error category, warning class, or count — client checks are ADVISORY and
// this operation IS the Core's authoritative side); the result labels
// EXACTLY which draft it evaluated (the exact-object labeling law,
// RUN-LINK-2); the operation mutates no stored revision and never silently
// saves the draft (the no-silent-save law); a node type the Core does not
// contract is WORKFLOW_UNKNOWN_NODE_TYPE (the Core's own catalogue — no
// node catalogue is published and none is invented); the un-supplied
// per-node dimensions produce NO fabricated findings.
func TestWorkflowValidateAuthoritative(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	ctx := env.ctx()
	def := workflowFixtureDefinition(t, slice, ctx, "v1")

	// The fresh definition counts (the no-silent-save law's anchor).
	revisionsBefore, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	// The valid draft: the typed result with zero blocking errors; the
	// typed-empty arrays (never fabricated findings); the exact-object
	// label carries the stated definition identity.
	res, err := slice.ValidateWorkflowDraft(ctx, WorkflowValidateRequest{
		CorrelationID: "corr-v1", DefinitionID: def.DefinitionID, Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if res.EvaluatedObjectRef != "draft:"+def.DefinitionID {
		t.Fatalf("the exact-object label: %q", res.EvaluatedObjectRef)
	}
	if len(res.BlockingErrors) != 0 || len(res.Warnings) != 0 || len(res.PermissionGaps) != 0 || len(res.SchemaCompatibility) != 0 {
		t.Fatalf("the valid draft's typed result: %+v", res)
	}

	// The issue table (each typed, each linkable to the offending
	// node/edge).
	cases := []struct {
		name      string
		draft     workflowdomain.DraftContent
		wantNode  string
		wantEdge  string
		wantCode  string
		wantIssue string
	}{
		{
			name: "unknown node type",
			draft: workflowdomain.DraftContent{
				Nodes: []workflowdomain.DraftNode{{NodeRef: "n1", NodeType: "totally-unknown", Config: map[string]any{}}},
				Edges: []workflowdomain.DraftEdge{},
			},
			wantNode: "n1", wantCode: string(errs.CodeWorkflowUnknownNodeType),
		},
		{
			name: "dangling edge source",
			draft: workflowdomain.DraftContent{
				Nodes: []workflowdomain.DraftNode{{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{}}},
				Edges: []workflowdomain.DraftEdge{{EdgeRef: "e1", FromNode: "ghost", ToNode: "n1"}},
			},
			wantEdge: "e1",
		},
		{
			name: "dangling edge target",
			draft: workflowdomain.DraftContent{
				Nodes: []workflowdomain.DraftNode{{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{}}},
				Edges: []workflowdomain.DraftEdge{{EdgeRef: "e1", FromNode: "n1", ToNode: "ghost"}},
			},
			wantEdge: "e1",
		},
		{
			name: "duplicate node_ref",
			draft: workflowdomain.DraftContent{
				Nodes: []workflowdomain.DraftNode{
					{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{}},
					{NodeRef: "n1", NodeType: "result", Config: map[string]any{}},
				},
				Edges: []workflowdomain.DraftEdge{},
			},
			wantNode: "n1",
		},
		{
			name: "self-edge",
			draft: workflowdomain.DraftContent{
				Nodes: []workflowdomain.DraftNode{{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{}}},
				Edges: []workflowdomain.DraftEdge{{EdgeRef: "e1", FromNode: "n1", ToNode: "n1"}},
			},
			wantEdge: "e1",
		},
		{
			name:      "no nodes",
			draft:     workflowdomain.DraftContent{Nodes: []workflowdomain.DraftNode{}, Edges: []workflowdomain.DraftEdge{}},
			wantIssue: "blocking",
		},
	}
	for _, tc := range cases {
		res, err := slice.ValidateWorkflowDraft(ctx, WorkflowValidateRequest{
			CorrelationID: "corr-" + tc.name, DefinitionID: def.DefinitionID, Draft: tc.draft,
		})
		if err != nil {
			t.Fatalf("%s: validate: %v", tc.name, err)
		}
		if len(res.BlockingErrors) == 0 {
			t.Fatalf("%s: the blocking finding must be typed", tc.name)
		}
		found := false
		for _, issue := range res.BlockingErrors {
			if issue.IssueClass != workflowdomain.IssueBlockingError {
				t.Fatalf("%s: the issue class: %+v", tc.name, issue)
			}
			if tc.wantCode != "" && issue.Code == tc.wantCode {
				found = true
			}
			if tc.wantNode != "" && issue.NodeRef == tc.wantNode {
				found = true
			}
			if tc.wantEdge != "" && issue.EdgeRef == tc.wantEdge {
				found = true
			}
			if tc.wantCode == "" && tc.wantNode == "" && tc.wantEdge == "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: the expected typed issue is missing: %+v", tc.name, res.BlockingErrors)
		}
		if len(res.Warnings) != 0 || len(res.PermissionGaps) != 0 || len(res.SchemaCompatibility) != 0 {
			t.Fatalf("%s: the un-supplied dimensions produce NO fabricated findings: %+v", tc.name, res)
		}
	}

	// The exact-object label for a new definition's draft (no Core identity
	// exists yet — the unsaved-draft class, recorded honestly).
	res, err = slice.ValidateWorkflowDraft(ctx, WorkflowValidateRequest{
		CorrelationID: "corr-new", Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("validate new: %v", err)
	}
	if res.EvaluatedObjectRef != "draft:unsaved" {
		t.Fatalf("the unsaved draft's label: %q", res.EvaluatedObjectRef)
	}

	// The stated definition's existence resolves first: the unknown
	// identity is the SAME typed 404 (the no-leak rule).
	_, err = slice.ValidateWorkflowDraft(ctx, WorkflowValidateRequest{
		CorrelationID: "corr-404", DefinitionID: "wfdef-nope", Draft: workflowValidDraft(),
	})
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)

	// THE NO-SILENT-SAVE LAW: validation mutates NO stored revision and
	// never silently saves the draft — the definition's revision record is
	// unchanged after every validation above.
	after, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect after: %v", err)
	}
	if len(after.Revisions) != len(revisionsBefore.Revisions) {
		t.Fatalf("validation saved nothing (the no-silent-save law): %d vs %d", len(after.Revisions), len(revisionsBefore.Revisions))
	}
}

// TestWorkflowSaveRevision pins the M-21 class through the service: the
// save creates a NEW immutable revision with the new reference STATED on
// save; the save's authoritative re-check refuses a draft carrying
// blocking findings (WORKFLOW_VALIDATION_FAILED — the typed rejection
// stores nothing, the draft surviving byte-for-byte client-side); the
// concurrent-revision outcome is the typed WORKFLOW_REVISION_CONFLICT with
// the stored side's REFERENCE only (the interim conflict rule); the keyed
// replay returns the recorded save.
func TestWorkflowSaveRevision(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	ctx := env.ctx()
	def := workflowFixtureDefinition(t, slice, ctx, "s1")

	// Save 1 (the empty base is the client's known base for a definition
	// with no revisions): the NEW revision reference STATED on save.
	save1, err := slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-1", CorrelationID: "corr-s1", Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("save 1: %v", err)
	}
	if save1.Replayed || save1.RevisionRef == "" {
		t.Fatalf("the save states the new revision reference: %+v", save1)
	}
	inspected, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(inspected.Revisions) != 1 || inspected.Definition.CurrentRevisionRef != save1.RevisionRef {
		t.Fatalf("the save created the NEW immutable revision: %+v %+v", inspected.Definition, inspected.Revisions)
	}

	// The authoritative re-check: a draft carrying blocking findings is
	// refused (WORKFLOW_VALIDATION_FAILED) — NOTHING stored (the typed
	// rejection leaves the draft surviving byte-for-byte client-side).
	_, err = slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-2", CorrelationID: "corr-s2", BaseRevisionRef: save1.RevisionRef,
		Draft: workflowdomain.DraftContent{
			Nodes: []workflowdomain.DraftNode{{NodeRef: "n1", NodeType: "unknown-thing", Config: map[string]any{}}},
			Edges: []workflowdomain.DraftEdge{},
		},
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeWorkflowValidationFailed)
	if envl.Retryable {
		t.Fatalf("the blocking-error outcome is never a transient failure")
	}
	after, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect after refusal: %v", err)
	}
	if len(after.Revisions) != 1 || after.Definition.CurrentRevisionRef != save1.RevisionRef {
		t.Fatalf("the refusal stored nothing (the draft survives byte-for-byte): %+v", after.Definition)
	}

	// The concurrent-revision outcome: the stale base — the typed
	// WORKFLOW_REVISION_CONFLICT carrying the stored side's reference; the
	// interim conflict rule governs (nothing overwritten, nothing merged).
	_, err = slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-3", CorrelationID: "corr-s3", BaseRevisionRef: "wfrev-stale",
		Draft: workflowValidDraft(),
	})
	envl = assertKnowledgeEnvelope(t, err, errs.CodeWorkflowRevisionConflict)
	if envl.Retryable {
		t.Fatalf("the conflict outcome is never a transient failure")
	}
	if envl.Data["current_revision_ref"] != save1.RevisionRef {
		t.Fatalf("the conflict carries the stored side's reference: %+v", envl.Data)
	}
	after2, _ := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if len(after2.Revisions) != 1 {
		t.Fatalf("the conflict stored nothing: %d", len(after2.Revisions))
	}

	// Save 2 against the recorded base: a second NEW immutable revision;
	// the old revision preserved immutable and reachable.
	save2, err := slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-4", CorrelationID: "corr-s4", BaseRevisionRef: save1.RevisionRef,
		Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("save 2: %v", err)
	}
	if save2.RevisionRef == save1.RevisionRef {
		t.Fatalf("a save creates a NEW revision, never an update-in-place")
	}
	after3, _ := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if len(after3.Revisions) != 2 {
		t.Fatalf("the revision history: %d", len(after3.Revisions))
	}

	// The keyed replay: the same key + the same correlation + the same
	// prepared-against base replays the recorded save.
	replay, err := slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-1", CorrelationID: "corr-s1", BaseRevisionRef: "",
		Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replay.Replayed || replay.RevisionRef != save1.RevisionRef {
		t.Fatalf("the replay returns the recorded revision, never a second one: %+v", replay)
	}
	after4, _ := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if len(after4.Revisions) != 2 {
		t.Fatalf("a replay never creates a second revision: %d", len(after4.Revisions))
	}

	// The unknown definition: the SAME typed 404 (the no-leak rule).
	_, err = slice.SaveWorkflowRevision(ctx, "wfdef-nope", WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-5", CorrelationID: "corr-s5", Draft: workflowValidDraft(),
	})
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
}

// TestWorkflowAIDiffProposeNoCoreWrite pins the AI-PROPOSAL LAW at the
// service: the member serves the typed unwired-producer refusal (no
// AI-proposal producer is wired — the provider/model selection and the
// data-handling/egress review surface the published request does not
// carry are UNSUPPLIED [B4]); NO proposal is ever fabricated; NOTHING is
// stored (a draft-local generation with no stored artifact — the approval
// carries no Core write at all); the same request re-issued meets the
// SAME refusal.
func TestWorkflowAIDiffProposeNoCoreWrite(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	ctx := env.ctx()
	def := workflowFixtureDefinition(t, slice, ctx, "ai1")

	inspectedBefore, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	// The propose attempt: the typed refusal — never a fabricated
	// proposal, never a stored artifact.
	req := WorkflowAIDiffProposeRequest{
		CorrelationID: "corr-ai-1", DefinitionID: def.DefinitionID,
		Draft: workflowValidDraft(), Instruction: "add a retry around the retrieval step",
	}
	_, err = slice.ProposeWorkflowAIDiff(ctx, req)
	envl := assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
	if envl.Data["precondition"] != "AI-PROPOSAL-PRODUCER" {
		t.Fatalf("the refusal names its unmet producer condition: %+v", envl.Data)
	}

	// NOTHING stored: the definition, its revisions, and the draft are
	// untouched by the proposal attempt (the no-Core-write law — the
	// approval is draft-local and carries no Core write at all).
	inspectedAfter, err := slice.WorkflowDefinitionInspect(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("inspect after: %v", err)
	}
	if len(inspectedAfter.Revisions) != len(inspectedBefore.Revisions) ||
		inspectedAfter.Definition.CurrentRevisionRef != inspectedBefore.Definition.CurrentRevisionRef {
		t.Fatalf("the proposal attempt wrote nothing (the no-Core-write law): %+v vs %+v", inspectedAfter.Definition, inspectedBefore.Definition)
	}

	// The same request re-issued: the SAME stable refusal.
	_, err = slice.ProposeWorkflowAIDiff(ctx, req)
	assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)

	// The member validation rides ahead of the refusal (the bounded
	// identity and the exact-object existence resolution). The draft
	// content is NOT pre-validated here — an empty draft is a legitimate
	// proposal input (the proposal may build the whole graph); the Core
	// validates the approved result at the subsequent save.
	_, err = slice.ProposeWorkflowAIDiff(ctx, WorkflowAIDiffProposeRequest{
		CorrelationID: "corr-ai-2", DefinitionID: "wfdef-nope", Instruction: "i", Draft: workflowValidDraft(),
	})
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	_, err = slice.ProposeWorkflowAIDiff(ctx, WorkflowAIDiffProposeRequest{CorrelationID: "corr-ai-3"})
	assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
}

// TestWorkflowRunHistoryAndState pins the read surfaces at the service:
// the run-history LINKAGE read's honest empty page while the D-40 gate
// stands (no Run producer exists — never a fabricated record; the unknown
// definition is the SAME typed 404) and the authoritative state endpoint
// (current and revision-specific; the pure query — never a retry).
func TestWorkflowRunHistoryAndState(t *testing.T) {
	env, slice := newWorkflowEnv(t)
	ctx := env.ctx()
	def := workflowFixtureDefinition(t, slice, ctx, "r1")
	save, err := slice.SaveWorkflowRevision(ctx, def.DefinitionID, WorkflowRevisionSaveRequest{
		IdempotencyKey: "skey-r1", CorrelationID: "corr-r1", Draft: workflowValidDraft(),
	})
	if err != nil {
		t.Fatalf("fixture save: %v", err)
	}

	// THE D-40 GATE: the honest empty page — no Run/step lifecycle
	// vocabulary, no run-state register, no cardinality claim exists.
	history, err := slice.WorkflowRunHistory(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("run history: %v", err)
	}
	if len(history.Entries) != 0 {
		t.Fatalf("no Run producer exists while the gate stands: %+v", history.Entries)
	}
	_, err = slice.WorkflowRunHistory(ctx, "wfdef-nope")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)

	// The authoritative state endpoint (the current state).
	state, err := slice.WorkflowDefinitionState(ctx, def.DefinitionID, "")
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.DefinitionID != def.DefinitionID || state.CurrentRevisionRef != save.RevisionRef {
		t.Fatalf("the authoritative state: %+v", state)
	}
	// The revision-specific state.
	revState, err := slice.WorkflowDefinitionState(ctx, def.DefinitionID, save.RevisionRef)
	if err != nil {
		t.Fatalf("revision state: %v", err)
	}
	if revState.CurrentRevisionRef != save.RevisionRef {
		t.Fatalf("the revision-specific state: %+v", revState)
	}
	// An unknown revision identity: the SAME typed 404.
	_, err = slice.WorkflowDefinitionState(ctx, def.DefinitionID, "wfrev-nope")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)

	// The keyset list read (the cursor's absence marks the final page).
	page, err := slice.WorkflowDefinitions(ctx, WorkflowListRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Definitions) == 0 || page.Cursor != "" {
		t.Fatalf("the list page: %+v", page)
	}
}

// TestWorkflowNilPortRefusals pins the nil-port refusals (fail closed):
// with the workflow port unwired, every surface answers the typed
// STORAGE_UNAVAILABLE refusal — never a fabricated answer.
func TestWorkflowNilPortRefusals(t *testing.T) {
	env := newTestEnv(t)
	slice := env.slice
	ctx := env.ctx()
	cases := []struct {
		name string
		call func() error
	}{
		{"create", func() error {
			_, err := slice.CreateWorkflowDefinition(ctx, WorkflowCreateRequest{IdempotencyKey: "k", CorrelationID: "c", DisplayName: "d"})
			return err
		}},
		{"validate", func() error {
			_, err := slice.ValidateWorkflowDraft(ctx, WorkflowValidateRequest{CorrelationID: "c", Draft: workflowValidDraft()})
			return err
		}},
		{"ai propose", func() error {
			_, err := slice.ProposeWorkflowAIDiff(ctx, WorkflowAIDiffProposeRequest{CorrelationID: "c", Instruction: "i"})
			return err
		}},
		{"save", func() error {
			_, err := slice.SaveWorkflowRevision(ctx, "wfdef-x", WorkflowRevisionSaveRequest{IdempotencyKey: "k", CorrelationID: "c", Draft: workflowValidDraft()})
			return err
		}},
		{"list", func() error { _, err := slice.WorkflowDefinitions(ctx, WorkflowListRequest{}); return err }},
		{"inspect", func() error { _, err := slice.WorkflowDefinitionInspect(ctx, "wfdef-x"); return err }},
		{"run history", func() error { _, err := slice.WorkflowRunHistory(ctx, "wfdef-x"); return err }},
		{"state", func() error { _, err := slice.WorkflowDefinitionState(ctx, "wfdef-x", ""); return err }},
	}
	for _, tc := range cases {
		assertKnowledgeEnvelope(t, tc.call(), errs.CodeStorageUnavailable)
	}
}
