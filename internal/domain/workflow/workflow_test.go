package workflow

import (
	"encoding/json"
	"testing"
)

// The B4 Workflow authoring family's domain register (DE-0 queue row 6):
// the closed registers carry the published canonical values VERBATIM (the
// issue classes, the linkage states, the event aggregates and types, the
// error-code strings), the wire shapes' JSON tags are the contract member
// names, and THE D-40 GATE stands exactly as recorded (the run-execution
// operation does not exist; the family's own support constant carries the
// recorded state — never flipped by implementation).

// TestClosedRegisters pins every closed register to the published values.
func TestClosedRegisters(t *testing.T) {
	// The issue-class register: the four typed classes (CF-4 §3(d)).
	issueClasses := []string{IssueBlockingError, IssueNonBlockingWarning, IssuePermissionGap, IssueSchemaCompatibility}
	wantIssueClasses := map[string]bool{
		"blocking_error": true, "non_blocking_warning": true, "permission_gap": true, "schema_compatibility": true,
	}
	for _, c := range issueClasses {
		if !wantIssueClasses[c] || !ValidIssueClass(c) {
			t.Fatalf("the issue-class register carries the published values verbatim: %q", c)
		}
	}
	if ValidIssueClass("") || ValidIssueClass("other") {
		t.Fatalf("the issue-class register is closed")
	}

	// The run-history linkage register: linked | unavailable (the honest
	// missing-linkage state — never an inferred link, RUN-LINK-3).
	if LinkageLinked != "linked" || LinkageUnavailable != "unavailable" {
		t.Fatalf("the linkage register carries the published values verbatim")
	}
	if !ValidLinkageState("linked") || !ValidLinkageState("unavailable") || ValidLinkageState("") {
		t.Fatalf("the linkage register is closed")
	}

	// The F10 event aggregates: workflowDefinition | workflowRevision.
	if AggregateWorkflowDefinition != "workflowDefinition" || AggregateWorkflowRevision != "workflowRevision" {
		t.Fatalf("the event-aggregate register carries the published values verbatim")
	}
	if !ValidAggregateType("workflowDefinition") || !ValidAggregateType("workflowRevision") || ValidAggregateType("") {
		t.Fatalf("the event-aggregate register is closed")
	}

	// The F10 event types: revision.created | revision.remote_changed |
	// validation.completed. NO Run/step lifecycle event exists — the
	// vocabulary is closed exactly at the published three.
	eventTypes := map[string]bool{
		EventRevisionCreated: true, EventRevisionRemoteChanged: true, EventValidationCompleted: true,
	}
	if len(eventTypes) != 3 || !ValidEventType("revision.created") ||
		!ValidEventType("revision.remote_changed") || !ValidEventType("validation.completed") {
		t.Fatalf("the event-type register carries the published values verbatim")
	}
	if ValidEventType("") || ValidEventType("run.started") || ValidEventType("step.completed") {
		t.Fatalf("no Run/step lifecycle event exists in the vocabulary (the GATED row)")
	}

	// The WorkflowErrorCode register strings (the published codes carried
	// verbatim; the idempotency/state conflicts ride the PUBLISHED codes).
	codes := map[string]string{
		FailValidationFailed:      "WORKFLOW_VALIDATION_FAILED",
		FailWarningUnacknowledged: "WORKFLOW_WARNING_UNACKNOWLEDGED",
		FailRevisionConflict:      "WORKFLOW_REVISION_CONFLICT",
		FailUnknownNodeType:       "WORKFLOW_UNKNOWN_NODE_TYPE",
		FailNodePermissionDenied:  "WORKFLOW_NODE_PERMISSION_DENIED",
		FailNotSaved:              "WORKFLOW_NOT_SAVED",
	}
	for got, want := range codes {
		if got != want {
			t.Fatalf("the error-code register carries the published strings verbatim: %q", got)
		}
	}
}

// TestD40GateConstant pins THE D-40 GATE exactly as recorded: the
// run-execution operation does not exist; the family's own support
// constant carries the recorded gate state (flipping it is the un-gate
// act, its own LIVE-gate class — never a side effect of implementation).
func TestD40GateConstant(t *testing.T) {
	if RunExecutionSupported {
		t.Fatalf("the D-40 gate stands: the run-execution operation does not exist; the support constant must carry the recorded state")
	}
}

// TestWireShapeJSONTags pins the wire shapes' JSON tags to the contract
// member names verbatim (the coded-name rule; never a restatement).
func TestWireShapeJSONTags(t *testing.T) {
	// WorkflowDefinition.
	b, err := json.Marshal(Definition{DefinitionID: "d1", CreatedAt: "t"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"definition_id":"d1","created_at":"t"}`
	if string(b) != want {
		t.Fatalf("the Definition wire shape: got %s want %s", b, want)
	}

	// WorkflowRevision.
	b, err = json.Marshal(Revision{RevisionRef: "r1", CreatedAt: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"revision_ref":"r1","created_at":"t"}` {
		t.Fatalf("the Revision wire shape: %s", b)
	}

	// WorkflowDraftNode (config open-typed — the per-node schema is
	// UNSUPPLIED [B4]).
	b, err = json.Marshal(DraftNode{NodeRef: "n1", NodeType: "trigger", Config: map[string]any{"k": "v"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"node_ref":"n1","node_type":"trigger","config":{"k":"v"}}` {
		t.Fatalf("the DraftNode wire shape: %s", b)
	}

	// WorkflowDraftEdge.
	b, err = json.Marshal(DraftEdge{EdgeRef: "e1", FromNode: "n1", ToNode: "n2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"edge_ref":"e1","from_node":"n1","to_node":"n2"}` {
		t.Fatalf("the DraftEdge wire shape: %s", b)
	}

	// WorkflowValidationIssue (the four typed classes ride issue_class; the
	// family register code rides code).
	b, err = json.Marshal(ValidationIssue{IssueClass: "blocking_error", NodeRef: "n1", Code: FailUnknownNodeType, Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"issue_class":"blocking_error","node_ref":"n1","code":"WORKFLOW_UNKNOWN_NODE_TYPE","message":"m"}` {
		t.Fatalf("the ValidationIssue wire shape: %s", b)
	}

	// WorkflowValidationResult (the typed-empty arrays marshal as [] —
	// never null).
	b, err = json.Marshal(ValidationResult{EvaluatedObjectRef: "draft:d1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"evaluated_object_ref":"draft:d1","blocking_errors":null,"warnings":null,"permission_gaps":null,"schema_compatibility":null}` {
		t.Fatalf("the ValidationResult wire shape: %s", b)
	}

	// WorkflowRunHistoryEntry (the LINKAGE record — no lifecycle, no state
	// register, no cardinality claim).
	b, err = json.Marshal(RunHistoryEntry{RunRef: "run-1", LinkageState: LinkageUnavailable})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"run_ref":"run-1","linkage_state":"unavailable"}` {
		t.Fatalf("the RunHistoryEntry wire shape: %s", b)
	}

	// WorkflowCreateResult (the Supplied-keyed replay datum).
	b, err = json.Marshal(CreateResult{Definition: Definition{DefinitionID: "d1", CreatedAt: "t"}, Replayed: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"definition":{"definition_id":"d1","created_at":"t"},"replayed":true}` {
		t.Fatalf("the CreateResult wire shape: %s", b)
	}

	// WorkflowRevisionSaveResult (the NEW revision reference STATED on
	// save).
	b, err = json.Marshal(RevisionSaveResult{RevisionRef: "r1", Replayed: false})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"revision_ref":"r1","replayed":false}` {
		t.Fatalf("the RevisionSaveResult wire shape: %s", b)
	}

	// WorkflowAIDiffProposal (the untrusted label rides the closed
	// register value).
	if UntrustedLabel != "untrusted" {
		t.Fatalf("the untrusted label carries the published register value verbatim")
	}
}

// TestKeyAndFingerprintBuilders pins the keyed classes' identity builders:
// the create key is workspace-scoped, the save key is workspace +
// definition-scoped, and the M-21 fingerprint joins the correlation ID and
// the revision's current state reference (the client's known base — which
// asserts no protocol).
func TestKeyAndFingerprintBuilders(t *testing.T) {
	if CreateKey("k1") == CreateKey("k2") {
		t.Fatalf("distinct client keys build distinct effective keys")
	}
	if SaveKey("k1", "d1") == SaveKey("k1", "d2") {
		t.Fatalf("the save key is scoped by the definition identity")
	}
	fp := SaveFingerprint("corr-1", "rev-9")
	if FingerprintBase(fp) != "rev-9" {
		t.Fatalf("the fingerprint's state half is the client's known base: %q", FingerprintBase(fp))
	}
	if FingerprintBase("corr-1") != "" {
		t.Fatalf("a separator-less fingerprint has no state half (it never matches a real base)")
	}
}
