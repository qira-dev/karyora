// Package workflow defines the B4 Workflow authoring family's wire shapes
// and closed registers (the published canonical contract family of
// contracts/openapi.yaml — the Workflow* schemas, the WorkflowErrorCode
// register, the WorkflowEvent* vocabularies, and the WorkflowDefinitionId
// parameter, carried verbatim: never a restatement, never an extension).
// The package is part of the domain leaf: it imports no other internal
// package and carries no transport, database, or OS types.
//
// THE D-40 GATE (carried exactly, binding): THE RUN-EXECUTION OPERATION
// DOES NOT EXIST — no Run submission, no Run start/resume/cancel, no Run
// schema, no Run/step lifecycle event vocabulary, no second run journal.
// The family's only `run` surface is the pure read listWorkflowRunHistory;
// its entry shape is the LINKAGE record (run_ref / source_revision /
// linkage_state / created_at) carrying NO lifecycle, NO state register,
// and NO cardinality claim. Where a future Run resolves to a durable Job
// submission, the published F6/F7/F9 classes govern at that future act —
// cited, never extended here.
//
// THE IF-MATCH CLAUSE (prose-only, carried verbatim): "RFC 9110 If-Match
// is a possible standard mechanism, not a UI-mandated wire contract"; the
// client does not assert If-Match or another concurrency protocol unless
// Core implements and documents it. ZERO header/parameter/response members
// named If-Match/ETag/412 exist in the published family and none is added
// here; base_revision_ref carries the client's known base and asserts no
// protocol; the interim conflict rule governs instead (no overwrite; no
// merge without an approved policy; the conflict state persists until user
// resolution — never auto-resolved, never timed out).
//
// THE REDACTION LAW (binding): no secret value ever enters a Workflow
// artifact — event payloads carry references only (digests and references,
// the AuditRecord law); AI proposals and model output are untrusted data
// that cannot grant permissions or change system instructions.
//
// THE NO-SILENT-SAVE LAW (binding): validation and run submission never
// silently save a draft unless stated AND Core-confirmed — the
// save-revision member is THE save; no other family operation saves. The
// draft survives a typed rejection byte-for-byte; the dirty state clears
// only on the Core-confirmed confirmation (never a pre-emptive "saved"
// label). THE S0–S4 LAW (binding): a lost response reconciles by the
// authoritative definition/revision-state read BEFORE any retry — never a
// blind retry.
package workflow

// The node vocabulary CLASS (carried in descriptions only — the published
// WorkflowDraftNode description's typed set, Core MB §8.1): Trigger →
// Knowledge/Retrieve → LLM/Tool/Plugin → Transform/Condition →
// Parallel/Join → HumanApproval/Wait → Checkpoint/Subworkflow → Result;
// the compound entries name related classes whose final split into
// distinct Core-owned types the Core's node-schema contract defines. NO
// node catalogue and NO per-node schema is published: every concrete
// per-node value is UNSUPPLIED [B4], config is open-typed because the
// per-node schema is unsupplied, and node_type is validated against the
// CORE'S OWN catalogue — the closed NodeKind set the Core's own
// definition engine carries (internal/workflow). The domain leaf cannot
// import the engine package, so the catalogue check lives in the app
// service layer (the dependency map's own rule).
const NodeVocabularyClassNote = "Trigger -> Knowledge/Retrieve -> LLM/Tool/Plugin -> Transform/Condition -> Parallel/Join -> HumanApproval/Wait -> Checkpoint/Subworkflow -> Result"

// The closed validation issue-class register (the published
// WorkflowValidationIssue.issue_class register carried verbatim — the four
// typed classes CF-4 §3(d) carries).
const (
	IssueBlockingError       = "blocking_error"
	IssueNonBlockingWarning  = "non_blocking_warning"
	IssuePermissionGap       = "permission_gap"
	IssueSchemaCompatibility = "schema_compatibility"
)

// ValidIssueClass reports membership in the closed issue-class register.
func ValidIssueClass(s string) bool {
	switch s {
	case IssueBlockingError, IssueNonBlockingWarning, IssuePermissionGap, IssueSchemaCompatibility:
		return true
	default:
		return false
	}
}

// The closed run-history linkage register (the published
// WorkflowRunHistoryEntry.linkage_state register carried verbatim).
// `unavailable` is the honest state for a missing linkage — NEVER an
// inferred link (RUN-LINK-3; the link is immutable and Core-owned and is
// rendered only as the Core reports it).
const (
	LinkageLinked      = "linked"
	LinkageUnavailable = "unavailable"
)

// ValidLinkageState reports membership in the closed linkage register.
func ValidLinkageState(s string) bool {
	return s == LinkageLinked || s == LinkageUnavailable
}

// The F10-shaped event aggregate types (the published
// WorkflowEventAggregateType register carried verbatim; events ride the
// frozen F10 EventRecord field set and are never the source of truth —
// the authoritative state endpoint is).
const (
	AggregateWorkflowDefinition = "workflowDefinition"
	AggregateWorkflowRevision   = "workflowRevision"
)

// ValidAggregateType reports membership in the closed aggregate register.
func ValidAggregateType(s string) bool {
	return s == AggregateWorkflowDefinition || s == AggregateWorkflowRevision
}

// The F10-shaped event_type vocabulary (the published WorkflowEventType
// register carried verbatim). NO Run/step lifecycle event exists in this
// vocabulary — that vocabulary is a GATED row (D-40 + the stack) and
// nothing drafts one. THE NO-SECOND-RUN-JOURNAL LAW (carried, binding as
// recorded): no second UI-side run journal and no separate Workflow-Run
// event stream exists while the D-40 mapping stands.
//
// `revision.created` records each NEW immutable revision's creation.
// `revision.remote_changed` records the concurrent-revision outcome's
// remote-change signal — the interim conflict rule governs its rendering.
// `validation.completed` records the Core-authoritative typed validation
// result's completion for the exact evaluated object (the exact-object
// labeling law applies to events too).
const (
	EventRevisionCreated       = "revision.created"
	EventRevisionRemoteChanged = "revision.remote_changed"
	EventValidationCompleted   = "validation.completed"
)

// ValidEventType reports membership in the closed event-type register.
func ValidEventType(s string) bool {
	switch s {
	case EventRevisionCreated, EventRevisionRemoteChanged, EventValidationCompleted:
		return true
	default:
		return false
	}
}

// The Workflow family's domain codes (the published WorkflowErrorCode
// register carried verbatim — each riding the published ErrorEnvelope
// with its PUBLISHED retryable value `never` and the coded retryClass;
// the codes join the coded register additively at their implementation,
// the established register-extension class). The idempotency/state
// conflicts ride the PUBLISHED codes exactly (CONFLICT_IDEMPOTENCY_KEY /
// CONFLICT_STATE) — no Workflow-specific conflict code exists.
const (
	// FailValidationFailed — the Core-owned typed blocking-error outcome
	// (the save's authoritative re-check refuses a draft carrying blocking
	// findings); the result is a fact about the evaluated object, not a
	// transient failure; the exact-object labeling law applies.
	FailValidationFailed = "WORKFLOW_VALIDATION_FAILED"
	// FailWarningUnacknowledged — a save or run attempted while identified
	// non-blocking warnings requiring acknowledgment are unacknowledged;
	// the acknowledgment is the user's explicit act — never a timeout,
	// never implicit, never a dismissed toast; a retry without
	// acknowledgment re-fails deterministically.
	FailWarningUnacknowledged = "WORKFLOW_WARNING_UNACKNOWLEDGED"
	// FailRevisionConflict — the concurrent-revision outcome; the interim
	// conflict rule governs: without an approved policy no overwrite and
	// no merge is performed, both sides are shown separately and labeled,
	// the draft is preserved, and the conflict state persists until the
	// user resolves it — never auto-resolved, never timed out into a
	// resolution, never resolved by a refresh.
	FailRevisionConflict = "WORKFLOW_REVISION_CONFLICT"
	// FailUnknownNodeType — a node type the active Core does not contract;
	// the palette source is ONLY the Core-contracted node vocabulary; no
	// placeholder node can be saved or run.
	FailUnknownNodeType = "WORKFLOW_UNKNOWN_NODE_TYPE"
	// FailNodePermissionDenied — a node requiring a capability/permission
	// the current principal lacks; the Core's validation and the single
	// decision port re-check authoritatively — client-side permission
	// filtering is presentation, not enforcement.
	FailNodePermissionDenied = "WORKFLOW_NODE_PERMISSION_DENIED"
	// FailNotSaved — an operation attempted against a draft that is not a
	// Core-confirmed saved revision (the no-silent-save law).
	FailNotSaved = "WORKFLOW_NOT_SAVED"
)

// THE D-40 GATE CONSTANT (the family's own support state — the recorded
// gate, NOT resolved by any implementation act; flipping it is the un-gate
// act, its own LIVE-gate class, never a side effect). FALSE today: the
// run-execution operation does not exist, no Run schema exists, no Run/step
// lifecycle event exists, and the run-history read's only producer (the
// Run execution) is gated — the linkage read serves the honest empty page
// for every existing definition while the gate stands.
const RunExecutionSupported = false

// The published idempotency/retention selections carried as the recorded
// window statements (stated in the published descriptions; carried here as
// the recorded values only — the windows govern the keyed classes at their
// implementation):
//   - create (the aggregate-create class): 72 h post-terminal (the
//     F6-precedent window selected at the publication act).
//   - save-revision (the M-21 class): 1 h post-terminal (the short-window
//     M-16-class precedent selected at the publication act).
//   - list / inspect / validate / run-history / AI-diff propose: none
//     required (the read classes; the proposal is a draft-local generation
//     with no stored artifact; the approval carries no Core write at all).
const (
	CreateReplayWindowNote = "72 h post-terminal (the F6-precedent window selected at the publication act)"
	SaveReplayWindowNote   = "1 h post-terminal (the short-window M-16-class precedent selected at the publication act)"
)

// Definition is one workflow definition's identity record (the published
// WorkflowDefinition schema carried verbatim — JSON tags are the contract
// member names). Workflow Definitions are immutable and versioned: editing
// creates a NEW immutable revision; each Run keeps its own durable graph
// revision/history; the three states stay distinct (the unsaved draft —
// client-local, never Core state; the current immutable stored revision;
// each Run of a specific revision).
type Definition struct {
	DefinitionID       string `json:"definition_id"`
	DisplayName        string `json:"display_name,omitempty"`
	CurrentRevisionRef string `json:"current_revision_ref,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// DefinitionList is the keyset page (the published WorkflowDefinitionList
// schema carried verbatim — the cursor's absence marks the final page).
type DefinitionList struct {
	Definitions []Definition `json:"definitions"`
	NextCursor  string       `json:"next_cursor,omitempty"`
}

// Revision is one immutable revision record (the published
// WorkflowRevision schema carried verbatim). The revision identity's wire
// form is an opaque Core-owned reference — no digest-keyed identity is
// published (the D-41 digest classes stay gated).
type Revision struct {
	RevisionRef string `json:"revision_ref"`
	CreatedAt   string `json:"created_at"`
}

// DraftNode is one draft node — the client-local authoring unit (the
// published WorkflowDraftNode schema carried verbatim). The node
// vocabulary CLASS rides this type's doc comment (NodeVocabularyClassNote);
// node_type is validated against the Core's own catalogue; config is
// open-typed because the per-node schema is UNSUPPLIED [B4] — the Core
// validates it against its own contract.
type DraftNode struct {
	NodeRef  string         `json:"node_ref"`
	NodeType string         `json:"node_type"`
	Config   map[string]any `json:"config"`
}

// DraftEdge is one draft edge — the client-local connection between two
// draft nodes (the published WorkflowDraftEdge schema carried verbatim).
type DraftEdge struct {
	EdgeRef  string `json:"edge_ref"`
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
}

// DraftContent is the draft's graph content — the client-local authoring
// state the family's write-path operations carry (the published
// WorkflowDraftContent schema carried verbatim). The draft is never Core
// state until a save-revision confirms one (the no-silent-save law).
type DraftContent struct {
	Nodes []DraftNode `json:"nodes"`
	Edges []DraftEdge `json:"edges"`
}

// CreateRequest is the create body (the published WorkflowCreateRequest
// schema carried verbatim) — the aggregate-create class. The Core mints
// the definition identity; the body never carries one. The idempotency
// key is scoped workspace Core-side; its fingerprint is the request
// identity (the correlation ID); the replay window is 72 h post-terminal.
type CreateRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	DisplayName    string `json:"display_name"`
}

// CreateResult is the Core-confirmed created definition and the replay
// datum (the published WorkflowCreateResult schema carried verbatim) —
// the Supplied-keyed replay pattern: a replay returns the recorded
// outcome, never a regenerated identity.
type CreateResult struct {
	Definition Definition `json:"definition"`
	Replayed   bool       `json:"replayed"`
}

// ValidateRequest is the validate body (the published
// WorkflowValidateRequest schema carried verbatim). The evaluated object's
// identity rides the request so the result labels EXACTLY which draft or
// revision it evaluated (the exact-object labeling law, RUN-LINK-2).
type ValidateRequest struct {
	CorrelationID string       `json:"correlation_id"`
	DefinitionID  string       `json:"definition_id,omitempty"`
	Draft         DraftContent `json:"draft"`
}

// ValidationIssue is one Core-owned typed validation issue (the published
// WorkflowValidationIssue schema carried verbatim) — linkable to the
// offending node/edge by its draft-local reference. The UI invents no
// error category, warning class, or count: every member here is the
// Core's typed output.
type ValidationIssue struct {
	IssueClass string `json:"issue_class"`
	NodeRef    string `json:"node_ref,omitempty"`
	EdgeRef    string `json:"edge_ref,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
}

// ValidationResult is the Core-authoritative typed validation result (the
// published WorkflowValidationResult schema carried verbatim). This
// operation is read-like: it mutates no stored revision and NEVER silently
// saves the draft (the no-silent-save law). The exact-object label rides
// evaluated_object_ref.
type ValidationResult struct {
	EvaluatedObjectRef  string            `json:"evaluated_object_ref"`
	BlockingErrors      []ValidationIssue `json:"blocking_errors"`
	Warnings            []ValidationIssue `json:"warnings"`
	PermissionGaps      []ValidationIssue `json:"permission_gaps"`
	SchemaCompatibility []ValidationIssue `json:"schema_compatibility"`
}

// RevisionSaveRequest is the save-revision body (the published
// WorkflowRevisionSaveRequest schema carried verbatim) — the M-21 class.
// base_revision_ref carries the client's known base and asserts no
// protocol (the If-Match clause: prose-only; the interim conflict rule
// governs).
type RevisionSaveRequest struct {
	IdempotencyKey  string       `json:"idempotency_key"`
	CorrelationID   string       `json:"correlation_id"`
	BaseRevisionRef string       `json:"base_revision_ref"`
	Draft           DraftContent `json:"draft"`
}

// RevisionSaveResult is the Core-confirmed save outcome (the published
// WorkflowRevisionSaveResult schema carried verbatim) — the NEW revision
// reference STATED on save; the old revision preserved immutable and
// reachable. A replay returns the recorded save — the same revision,
// never a second one.
type RevisionSaveResult struct {
	RevisionRef string `json:"revision_ref"`
	Replayed    bool   `json:"replayed"`
}

// AIDiffProposeRequest is the AI-diff propose body (the published
// WorkflowAIDiffProposeRequest schema carried verbatim) — the M-23 class.
// The instruction is carried verbatim to the Core's proposal path; the
// proposal output is never treated as a command (the untrusted-input law).
type AIDiffProposeRequest struct {
	CorrelationID string       `json:"correlation_id"`
	DefinitionID  string       `json:"definition_id,omitempty"`
	Draft         DraftContent `json:"draft"`
	Instruction   string       `json:"instruction"`
}

// AIDiffProposal is the untrusted reviewable proposal (the published
// WorkflowAIDiffProposal schema carried verbatim). THE AI-PROPOSAL LAW
// (binding as carried): untrusted reviewable TEXT — it cannot grant
// permissions, mint capabilities, execute, or change system instructions;
// approval is EXPLICIT and is the USER's; AI changes are never auto-saved,
// never auto-run; the approval itself saves nothing and runs nothing —
// the approved change enters the DRAFT and then follows the normal laws.
// The untrusted_label rides the published UntrustedLabel register value
// (`untrusted`).
type AIDiffProposal struct {
	OldRegion      string `json:"old_region"`
	NewRegion      string `json:"new_region"`
	UntrustedLabel string `json:"untrusted_label"`
}

// UntrustedLabel is the published UntrustedLabel register value (the
// closed register `untrusted` — the Layer-1 precedent the proposal rides).
const UntrustedLabel = "untrusted"

// RunHistoryEntry is one run-history entry — a LINKAGE record, NOT the
// D-40-blocked Run schema (the published WorkflowRunHistoryEntry schema
// carried verbatim): it carries no Run/step lifecycle vocabulary, no
// run-state register, no step model, and no Run/Job cardinality claim.
// The Run ↔ source-revision link is immutable and Core-owned: the entry
// renders the link ONLY as the Core reports it — missing linkage renders
// `unavailable`, NEVER an inferred link (RUN-LINK-3).
type RunHistoryEntry struct {
	RunRef         string `json:"run_ref"`
	SourceRevision string `json:"source_revision,omitempty"`
	LinkageState   string `json:"linkage_state"`
	CreatedAt      string `json:"created_at,omitempty"`
}

// RunHistory is the run-history read's payload (the published
// WorkflowRunHistory schema carried verbatim) — a pure read; each entry
// beside its Core-reported source revision.
type RunHistory struct {
	Entries []RunHistoryEntry `json:"entries"`
}

// DefinitionState is the authoritative definition/revision-state record
// (the published WorkflowDefinitionState schema carried verbatim) — the
// family's authoritative state endpoint's payload (a pure query). The
// client's unsaved draft is never Core state (the no-silent-save law);
// the current immutable stored revision is what this read reports.
type DefinitionState struct {
	DefinitionID       string `json:"definition_id"`
	CurrentRevisionRef string `json:"current_revision_ref,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// The durable intents and outcomes (the store port's contracts — not wire
// shapes; the keyed classes' identity members).

// keySeparator joins the client key and the Core-side scope halves of the
// effective idempotency keys (the shared separator class — references
// never collide across classes).
const keySeparator = "\x1f"

// CreateKey builds the create class's effective key: the client key
// scoped workspace Core-side (the local Core IS the workspace scope; the
// aggregate-create class's key scope).
func CreateKey(clientKey string) string {
	return clientKey + keySeparator + "wf.create"
}

// SaveKey builds the M-21 save class's effective key: the client key
// scoped workspace + definition ID Core-side.
func SaveKey(clientKey, definitionID string) string {
	return clientKey + keySeparator + "wf.save:" + definitionID
}

// SaveFingerprint builds the M-21 fingerprint: the correlation ID + the
// revision's current state reference (the client's known base — the
// fingerprint's other half).
func SaveFingerprint(correlationID, baseRevisionRef string) string {
	return correlationID + keySeparator + baseRevisionRef
}

// FingerprintBase extracts the state-reference half of a recorded save
// fingerprint (the replay ruling's comparison half).
func FingerprintBase(fingerprint string) string {
	for i := len(fingerprint) - 1; i >= 0; i-- {
		if fingerprint[i] == keySeparator[0] {
			return fingerprint[i+len(keySeparator):]
		}
	}
	return ""
}

// CreateIntent is the create class's durable intent: the Core-minted
// definition identity, the operation record's identity members, and the
// effective (workspace-scoped) key with its request-identity fingerprint.
type CreateIntent struct {
	Definition     Definition
	OperationID    string
	IdempotencyKey string // the effective key (CreateKey)
	Fingerprint    string // the request identity (the correlation ID)
}

// CreateOutcome is one create call's store-confirmed outcome: the
// Core-confirmed definition (the recorded one on a replay — never a
// regenerated identity) and the replay datum.
type CreateOutcome struct {
	Definition  Definition
	OperationID string
	Replayed    bool
}

// SaveIntent is the M-21 save class's durable intent: the destination
// definition, the NEW immutable revision's record, the operation record's
// identity members, the effective (workspace + definition-scoped) key,
// and the fingerprint (the correlation ID + the client's known base).
type SaveIntent struct {
	DefinitionID    string
	Revision        Revision
	OperationID     string
	CorrelationID   string // the request identity (the fingerprint's first half)
	IdempotencyKey  string // the effective key (SaveKey)
	Fingerprint     string // SaveFingerprint(correlation, base)
	BaseRevisionRef string // the client's known base (asserts no protocol)
}

// SaveOutcome is one save call's store-confirmed outcome: the NEW
// immutable revision (the recorded one on a replay — never a second one).
type SaveOutcome struct {
	Revision    Revision
	OperationID string
	Replayed    bool
}

// ListQuery is one bounded keyset page request (created_at DESC,
// definition_id DESC — the newest first; the keyset position rides the
// opaque cursor).
type ListQuery struct {
	Limit          int
	AfterCreatedAt string
	AfterID        string
}

// DefinitionPage is one bounded page. More marks a following page (the
// cursor's mint is the app layer's; the cursor's absence on the wire
// marks the final page).
type DefinitionPage struct {
	Definitions []Definition
	More        bool
}
