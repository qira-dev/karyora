package app

import (
	stdcontext "context"
	"encoding/json"
	"fmt"

	wfengine "github.com/qira-dev/karyora/internal/workflow"

	"github.com/qira-dev/karyora/internal/domain/errs"
	workflowdomain "github.com/qira-dev/karyora/internal/domain/workflow"
	"github.com/qira-dev/karyora/internal/security"
)

// The B4 Workflow authoring family service (DE-0 queue row 6): the
// published /workflows route family's use cases, implemented against the
// canonical contract at the working HEAD — the aggregate-create class (the
// Core mints the definition identity; the body never carries one; the 72 h
// post-terminal keyed replay), the M-21 save class (the NEW immutable
// revision stated on save; no update-in-place; the 1 h post-terminal keyed
// replay; the interim conflict rule on the concurrent-revision outcome),
// the Core-authoritative typed validation (read-like: mutates no stored
// revision and never silently saves the draft; client checks are ADVISORY
// and labeled as such at the point of display — a local validation result
// is never a truth source), the M-23 AI-diff propose member (untrusted
// reviewable TEXT; the approval is draft-local and carries no Core write),
// the run-history LINKAGE read, and the family's authoritative state
// endpoint (a pure query — never a retry; the S0–S4 reconcile anchor).
//
// THE D-40 GATE (carried exactly, binding): THE RUN-EXECUTION OPERATION
// DOES NOT EXIST — no Run submission, no Run start/resume/cancel, no Run
// schema, no Run/step lifecycle event, no second run journal. The
// run-history read's only producer (the Run execution) is gated, so every
// existing definition answers the honest empty page while the gate stands.
// THE IF-MATCH CLAUSE: prose-only — no precondition member exists; the
// client's base_revision_ref asserts no protocol; the interim conflict
// rule governs. THE AI-PROPOSAL LAW (binding): a proposal is untrusted
// reviewable TEXT — it cannot grant permissions, mint capabilities,
// execute, or change system instructions; approval is EXPLICIT and is the
// USER's; AI changes are never auto-saved, never auto-run; the approval
// saves nothing and runs nothing. THE REDACTION LAW: no secret value ever
// enters a Workflow artifact (references only). Every protected effect
// rides the single decision port (security.ResourceWorkflow — the additive
// register extension; Local mode stays role-free: no permission policy is
// invented).

// WorkflowStore is the durable Workflow authoring surface port (B4). The
// storage adapter implements it over the workflow tables (migration 0016);
// the slice consumes only this port. Nil disables the surface — typed
// refusals, fail closed (the KnowledgeStore/MemoryStore/ContextStore
// pattern).
type WorkflowStore interface {
	// SubmitWorkflowCreate applies the aggregate-create class's ruling (72 h
	// post-terminal; a replay returns the recorded Core-minted identity —
	// never a regenerated one).
	SubmitWorkflowCreate(ctx stdcontext.Context, intent workflowdomain.CreateIntent) (workflowdomain.CreateOutcome, error)
	// SubmitWorkflowRevisionSave applies the M-21 class's ruling (1 h
	// post-terminal; the concurrent-revision outcome is the typed
	// WORKFLOW_REVISION_CONFLICT; the NEW revision is an INSERT — the old
	// revision stays immutable and reachable).
	SubmitWorkflowRevisionSave(ctx stdcontext.Context, intent workflowdomain.SaveIntent) (workflowdomain.SaveOutcome, error)
	// LoadWorkflowDefinition resolves one definition's authoritative record;
	// the unknown identity and the out-of-scope identity are the SAME typed
	// 404 (the no-leak rule).
	LoadWorkflowDefinition(ctx stdcontext.Context, definitionID string) (workflowdomain.Definition, error)
	// ListWorkflowDefinitions reads one bounded keyset page (the cursor's
	// absence marks the final page).
	ListWorkflowDefinitions(ctx stdcontext.Context, q workflowdomain.ListQuery) (workflowdomain.DefinitionPage, error)
	// ListWorkflowRevisions reads one definition's revision records — each
	// immutable and reachable.
	ListWorkflowRevisions(ctx stdcontext.Context, definitionID string) ([]workflowdomain.Revision, error)
	// ListWorkflowRunHistory reads one definition's run-history LINKAGE
	// records (a pure read; the D-40 gate stands — the honest empty page
	// while no Run producer exists).
	ListWorkflowRunHistory(ctx stdcontext.Context, definitionID string) ([]workflowdomain.RunHistoryEntry, error)
	// LoadWorkflowDefinitionState resolves the family's authoritative state
	// read (a pure query — never a retry).
	LoadWorkflowDefinitionState(ctx stdcontext.Context, definitionID, revisionID string) (workflowdomain.DefinitionState, error)
}

// Workflow bounds (the coded bounds class: identifiers ride the 128-char
// bound; labels ride 512; the graph arrays are bounded — bounded, never
// unbounded). The node bound is the Core's own catalogue bound
// (wfengine.MaxNodesPerDefinition); the edge bound is this implementation's
// recorded bound (the MaxContextDecisions class).
const (
	maxWorkflowIDLen          = 128
	maxWorkflowLabelLen       = 512
	maxWorkflowInstructionLen = 1 << 16
	MaxWorkflowEdges          = 512
	workflowDraftField        = "draft"
)

// WorkflowCreateRequest is one create body (the published
// WorkflowCreateRequest members as the service contract).
type WorkflowCreateRequest struct {
	IdempotencyKey string
	CorrelationID  string
	DisplayName    string
}

// WorkflowValidateRequest is one validate body (the published
// WorkflowValidateRequest members as the service contract).
type WorkflowValidateRequest struct {
	CorrelationID string
	DefinitionID  string
	Draft         workflowdomain.DraftContent
}

// WorkflowAIDiffProposeRequest is one AI-diff propose body (the published
// WorkflowAIDiffProposeRequest members as the service contract).
type WorkflowAIDiffProposeRequest struct {
	CorrelationID string
	DefinitionID  string
	Draft         workflowdomain.DraftContent
	Instruction   string
}

// WorkflowRevisionSaveRequest is one save-revision body (the published
// WorkflowRevisionSaveRequest members as the service contract).
type WorkflowRevisionSaveRequest struct {
	IdempotencyKey  string
	CorrelationID   string
	BaseRevisionRef string
	Draft           workflowdomain.DraftContent
}

// WorkflowListRequest is one bounded list request.
type WorkflowListRequest struct {
	Limit  int
	Cursor string
}

// WorkflowListResult is one bounded page (the cursor's absence marks the
// final page).
type WorkflowListResult struct {
	Definitions []workflowdomain.Definition
	Cursor      string
}

// workflowNilPort is the typed refusal for a nil store (fail closed).
func workflowNilPort(corr string) error {
	return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "workflow store is not wired"})
}

// validateWorkflowIdentity validates one bounded identifier member.
func validateWorkflowIdentity(value, field string, corr string) error {
	if value == "" || len(value) > maxWorkflowIDLen {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": field})
	}
	return nil
}

// validateWorkflowDraftBounds enforces the bounded-graph bounds ahead of
// any ruling (bounded, never unbounded).
func validateWorkflowDraftBounds(draft workflowdomain.DraftContent, corr string) error {
	if len(draft.Nodes) > wfengine.MaxNodesPerDefinition {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": workflowDraftField, "reason": "the draft exceeds the node bound",
		})
	}
	if len(draft.Edges) > MaxWorkflowEdges {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": workflowDraftField, "reason": "the draft exceeds the edge bound",
		})
	}
	return nil
}

// workflowDraftIssues runs the Core's own authoritative structural checks
// over a draft and returns the typed issues (each linkable to the
// offending node/edge by its draft-local reference — the exact-object
// law's linkage anchor). The node_type check rides the CORE'S OWN
// catalogue (wfengine.KnownNodeKind — the closed NodeKind set the Core's
// definition engine carries; a type the active Core does not contract is
// WORKFLOW_UNKNOWN_NODE_TYPE; no placeholder node can be saved or run).
// The catalogue is the Core's own — NO node catalogue is published by the
// family (UNSUPPLIED [B4]) and none is invented here. The issue messages
// are the Core's typed statements — they never echo unbounded member
// values (the redaction law). The un-supplied per-node dimensions (the
// per-node schema, capability/permission declarations, egress
// classifications) produce NO fabricated findings: warnings,
// permission_gaps, and schema_compatibility are typed-empty where no
// Core-checkable source exists (recorded honestly).
func workflowDraftIssues(draft workflowdomain.DraftContent) []workflowdomain.ValidationIssue {
	issues := []workflowdomain.ValidationIssue{}
	add := func(nodeRef, edgeRef, code, message string) {
		issues = append(issues, workflowdomain.ValidationIssue{
			IssueClass: workflowdomain.IssueBlockingError,
			NodeRef:    nodeRef,
			EdgeRef:    edgeRef,
			Code:       code,
			Message:    message,
		})
	}
	if len(draft.Nodes) == 0 {
		add("", "", "", "the draft carries no nodes (the Core's definition semantics require at least one node)")
	}
	seenNodes := map[string]bool{}
	for _, n := range draft.Nodes {
		switch {
		case n.NodeRef == "":
			add("", "", "", "a draft node carries no node_ref")
		case len(n.NodeRef) > maxWorkflowIDLen:
			add("", "", "", "a draft node_ref exceeds the identifier bound")
		case seenNodes[n.NodeRef]:
			add(n.NodeRef, "", "", "the draft declares the same node_ref twice")
		default:
			seenNodes[n.NodeRef] = true
		}
		if !wfengine.KnownNodeKind(wfengine.NodeKind(n.NodeType)) {
			ref := n.NodeRef
			if len(ref) > maxWorkflowIDLen {
				ref = ""
			}
			add(ref, "", string(errs.CodeWorkflowUnknownNodeType),
				"the node type is not in the Core's own catalogue; no placeholder node can be saved or run")
		}
	}
	seenEdges := map[string]bool{}
	for _, e := range draft.Edges {
		switch {
		case e.EdgeRef == "":
			add("", "", "", "a draft edge carries no edge_ref")
		case len(e.EdgeRef) > maxWorkflowIDLen:
			add("", "", "", "a draft edge_ref exceeds the identifier bound")
		case seenEdges[e.EdgeRef]:
			add("", e.EdgeRef, "", "the draft declares the same edge_ref twice")
		default:
			seenEdges[e.EdgeRef] = true
		}
		if !seenNodes[e.FromNode] {
			add("", e.EdgeRef, "", "the edge's source node_ref is not a declared node")
		}
		if !seenNodes[e.ToNode] {
			add("", e.EdgeRef, "", "the edge's target node_ref is not a declared node")
		}
		if e.FromNode != "" && e.FromNode == e.ToNode {
			add("", e.EdgeRef, "", "a draft edge cannot join a node to itself")
		}
	}
	return issues
}

// workflowEvaluatedRef builds the exact-object label (the evaluated-object
// labeling law, RUN-LINK-2): the result labels EXACTLY which draft it
// evaluated. A draft under an existing definition is labeled by that
// definition's identity; a new definition's draft carries no Core identity
// yet (the identity mints at create) — its label is the unsaved-draft
// class, recorded honestly as this implementation's labeling choice under
// the law's honest limit.
func workflowEvaluatedRef(definitionID string) string {
	if definitionID != "" {
		return "draft:" + definitionID
	}
	return "draft:unsaved"
}

// CreateWorkflowDefinition is the aggregate-create member (the Core mints
// the definition identity — the body never carries one). The effective
// key is workspace-scoped; the fingerprint is the request identity (the
// correlation ID); the replay window is 72 h post-terminal. An unknown
// outcome is reconciled by the correlation ID before any retry (the S0–S4
// law; the keyed replay returns the recorded outcome) — never a duplicate
// create.
func (s *Slice) CreateWorkflowDefinition(ctx stdcontext.Context, req WorkflowCreateRequest) (workflowdomain.CreateResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.CreateResult{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow}, security.ActionDataWrite); err != nil {
		return workflowdomain.CreateResult{}, err
	}
	if err := validateWorkflowIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return workflowdomain.CreateResult{}, err
	}
	if err := validateWorkflowIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return workflowdomain.CreateResult{}, err
	}
	if req.DisplayName == "" || len(req.DisplayName) > maxWorkflowLabelLen {
		return workflowdomain.CreateResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "display_name"})
	}
	// THE CORE-MINTS-THE-IDENTITY LAW: the identity is minted HERE (the
	// Core) — the body never carries one.
	intent := workflowdomain.CreateIntent{
		Definition: workflowdomain.Definition{
			DefinitionID: "wfdef-" + randomID(),
			DisplayName:  req.DisplayName,
		},
		OperationID:    "wfop-" + randomID(),
		IdempotencyKey: workflowdomain.CreateKey(req.IdempotencyKey),
		Fingerprint:    req.CorrelationID,
	}
	outcome, err := s.deps.Workflow.SubmitWorkflowCreate(ctx, intent)
	if err != nil {
		return workflowdomain.CreateResult{}, err
	}
	return workflowdomain.CreateResult{Definition: outcome.Definition, Replayed: outcome.Replayed}, nil
}

// ValidateWorkflowDraft is the Core-authoritative typed validation (a
// read-like operation producing a Core-owned typed result: it mutates no
// stored revision and NEVER silently saves the draft — the no-silent-save
// law). CLIENT CHECKS ARE ADVISORY and labeled as such at the point of
// display — a local validation result is never a truth source; this
// operation IS the Core's authoritative side. The result labels EXACTLY
// which draft it evaluated (the exact-object labeling law, RUN-LINK-2);
// validation of one object is never presented as another object's result.
// Where the request states a definition identity, its existence resolves
// first — the unknown identity is the SAME typed 404 (the no-leak rule).
func (s *Slice) ValidateWorkflowDraft(ctx stdcontext.Context, req WorkflowValidateRequest) (workflowdomain.ValidationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.ValidationResult{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: req.DefinitionID}, security.ActionDataRead); err != nil {
		return workflowdomain.ValidationResult{}, err
	}
	if err := validateWorkflowIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return workflowdomain.ValidationResult{}, err
	}
	if len(req.DefinitionID) > maxWorkflowIDLen {
		return workflowdomain.ValidationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "definition_id"})
	}
	if err := validateWorkflowDraftBounds(req.Draft, corr); err != nil {
		return workflowdomain.ValidationResult{}, err
	}
	if req.DefinitionID != "" {
		// The stated definition's existence resolves first: the unknown
		// identity is the SAME typed 404 (the no-leak rule).
		if _, err := s.deps.Workflow.LoadWorkflowDefinition(ctx, req.DefinitionID); err != nil {
			return workflowdomain.ValidationResult{}, err
		}
	}
	return workflowdomain.ValidationResult{
		EvaluatedObjectRef:  workflowEvaluatedRef(req.DefinitionID),
		BlockingErrors:      workflowDraftIssues(req.Draft),
		Warnings:            []workflowdomain.ValidationIssue{},
		PermissionGaps:      []workflowdomain.ValidationIssue{},
		SchemaCompatibility: []workflowdomain.ValidationIssue{},
	}, nil
}

// ProposeWorkflowAIDiff is the M-23 AI-diff propose member. THE
// AI-PROPOSAL LAW (binding): the proposal generation is a Core call
// producing UNTRUSTED REVIEWABLE TEXT; the approval that follows is
// DRAFT-LOCAL and carries no Core write (the approval saves nothing and
// runs nothing — the subsequent save carries its own row and its own
// class); AI changes are never auto-saved, never auto-run, never executed
// without explicit user approval; Escape, dismissal, navigation, or a
// timeout never approves and never silently discards the DRAFT.
//
// THE HONEST UNMET-PRODUCER STATE (recorded, never a fabricated proposal):
// no AI-proposal producer is wired on this Core — the proposal path
// requires the provider/model selection and the data-handling/egress
// review surface that the published request schema does not carry
// (UNSUPPLIED [B4]: no provider/model member exists on
// WorkflowAIDiffProposeRequest; routing authoring content through any
// wired executor without the published review surface would fabricate).
// The member serves the typed unwired-producer refusal (the register's
// honest-unavailable carrier class — the nil-port/wired-posture refusal
// the B3 unmet-condition treatment rides; the implementation-authored
// typing, never a contract edit), NO proposal is ever fabricated, NOTHING
// is stored (a draft-local generation with no stored artifact), and the
// same request re-issued meets the SAME refusal. Flipping the producer's
// condition is its own LIVE-gate class, never a side effect.
func (s *Slice) ProposeWorkflowAIDiff(ctx stdcontext.Context, req WorkflowAIDiffProposeRequest) (workflowdomain.AIDiffProposal, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.AIDiffProposal{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: req.DefinitionID}, security.ActionDataRead); err != nil {
		return workflowdomain.AIDiffProposal{}, err
	}
	if err := validateWorkflowIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return workflowdomain.AIDiffProposal{}, err
	}
	if len(req.DefinitionID) > maxWorkflowIDLen {
		return workflowdomain.AIDiffProposal{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "definition_id"})
	}
	if req.Instruction == "" || len(req.Instruction) > maxWorkflowInstructionLen {
		return workflowdomain.AIDiffProposal{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "instruction"})
	}
	if err := validateWorkflowDraftBounds(req.Draft, corr); err != nil {
		return workflowdomain.AIDiffProposal{}, err
	}
	if req.DefinitionID != "" {
		// The stated definition's existence resolves first: the unknown
		// identity is the SAME typed 404 (the no-leak rule).
		if _, err := s.deps.Workflow.LoadWorkflowDefinition(ctx, req.DefinitionID); err != nil {
			return workflowdomain.AIDiffProposal{}, err
		}
	}
	// THE UNMET-PRODUCER REFUSAL (the honest unavailable state — never a
	// fabricated proposal, never a stored artifact). THE AI-PROPOSAL LAW
	// rides the refusal exactly as the published descriptions carry it.
	return workflowdomain.AIDiffProposal{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{
		"precondition": "AI-PROPOSAL-PRODUCER",
		"reason":       "the AI-proposal producer is not wired on this Core: the proposal path requires the provider/model selection and the data-handling/egress review surface the published request does not carry (UNSUPPLIED [B4]); no proposal is ever fabricated and nothing is stored (a draft-local generation with no stored artifact)",
	})
}

// SaveWorkflowRevision is the M-21 save member: the save creates a NEW
// immutable revision; the response states the new revision reference; the
// old revision is preserved immutable and reachable — there is NO
// update-in-place (the NO-silent-overwrite law). The save's authoritative
// re-check refuses a draft carrying blocking findings
// (WORKFLOW_VALIDATION_FAILED — the typed rejection leaves the draft
// surviving byte-for-byte, visibly so; nothing is overwritten). The
// concurrent-revision outcome (the stated base no longer the current
// stored revision) is the typed WORKFLOW_REVISION_CONFLICT — the interim
// conflict rule governs: no overwrite, no merge without an approved
// policy, both sides preserved, the conflict data carries the stored
// side's REFERENCE only. A lost response reconciles by the authoritative
// definition/revision-state read BEFORE any retry (VRC-LOST-1; the S0–S4
// law) — never a blind retry; the keyed replay (1 h post-terminal) returns
// the recorded save.
func (s *Slice) SaveWorkflowRevision(ctx stdcontext.Context, definitionID string, req WorkflowRevisionSaveRequest) (workflowdomain.RevisionSaveResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.RevisionSaveResult{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: definitionID}, security.ActionDataWrite); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	if err := validateWorkflowIdentity(definitionID, "definition_id", corr); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	if err := validateWorkflowIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	if err := validateWorkflowIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	// base_revision_ref carries the client's known base and asserts no
	// protocol (the If-Match clause: prose-only). An empty base is the
	// client's known base for a definition with no revisions.
	if len(req.BaseRevisionRef) > maxWorkflowIDLen {
		return workflowdomain.RevisionSaveResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_revision_ref"})
	}
	if err := validateWorkflowDraftBounds(req.Draft, corr); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	// The destination identity resolves first: the unknown definition is
	// the SAME typed 404 the inspect read serves (the no-leak rule).
	if _, err := s.deps.Workflow.LoadWorkflowDefinition(ctx, definitionID); err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	// THE SAVE'S AUTHORITATIVE RE-CHECK: the Core refuses a draft carrying
	// blocking findings (no placeholder node can be saved — the register's
	// own law). The typed rejection preserves the draft byte-for-byte
	// client-side; nothing is stored.
	if issues := workflowDraftIssues(req.Draft); len(issues) > 0 {
		return workflowdomain.RevisionSaveResult{}, envelopeErr(errs.CodeWorkflowValidationFailed, corr, map[string]any{
			"field":          workflowDraftField,
			"blocking_count": len(issues),
			"reason":         "the draft carries blocking findings; validate the draft for the typed issues (the exact-object labeling law)",
		})
	}
	outcome, err := s.deps.Workflow.SubmitWorkflowRevisionSave(ctx, workflowdomain.SaveIntent{
		DefinitionID:    definitionID,
		Revision:        workflowdomain.Revision{RevisionRef: "wfrev-" + randomID()},
		OperationID:     "wfop-" + randomID(),
		CorrelationID:   req.CorrelationID,
		IdempotencyKey:  workflowdomain.SaveKey(req.IdempotencyKey, definitionID),
		Fingerprint:     workflowdomain.SaveFingerprint(req.CorrelationID, req.BaseRevisionRef),
		BaseRevisionRef: req.BaseRevisionRef,
	})
	if err != nil {
		return workflowdomain.RevisionSaveResult{}, err
	}
	return workflowdomain.RevisionSaveResult{RevisionRef: outcome.Revision.RevisionRef, Replayed: outcome.Replayed}, nil
}

// WorkflowDefinitions is the keyset list read (the M-1 correlation-only
// read class; safe to re-issue; the cursor's absence marks the final page).
func (s *Slice) WorkflowDefinitions(ctx stdcontext.Context, req WorkflowListRequest) (WorkflowListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return WorkflowListResult{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow}, security.ActionDataRead); err != nil {
		return WorkflowListResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return WorkflowListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	q := workflowdomain.ListQuery{Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeWorkflowCursor(req.Cursor)
		if err != nil {
			return WorkflowListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterID = pos.CreatedAt, pos.ID
	}
	page, err := s.deps.Workflow.ListWorkflowDefinitions(ctx, q)
	if err != nil {
		return WorkflowListResult{}, err
	}
	res := WorkflowListResult{Definitions: page.Definitions}
	if page.More {
		if len(page.Definitions) == 0 {
			return WorkflowListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeWorkflowCursor(page.Definitions[len(page.Definitions)-1])
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return WorkflowListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.Cursor = next
	}
	return res, nil
}

// WorkflowDefinitionInspect is the inspect member: the definition-detail
// read including its revisions — each revision immutable and reachable.
// The read returns exactly the stated definition's record — never another
// object's result (the exact-object labeling law); the unknown identity
// and the out-of-scope identity are the SAME typed 404 (the no-leak rule).
type WorkflowDefinitionInspect struct {
	Definition workflowdomain.Definition
	Revisions  []workflowdomain.Revision
}

func (s *Slice) WorkflowDefinitionInspect(ctx stdcontext.Context, definitionID string) (WorkflowDefinitionInspect, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return WorkflowDefinitionInspect{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: definitionID}, security.ActionDataRead); err != nil {
		return WorkflowDefinitionInspect{}, err
	}
	if err := validateWorkflowIdentity(definitionID, "definition_id", corr); err != nil {
		return WorkflowDefinitionInspect{}, err
	}
	def, err := s.deps.Workflow.LoadWorkflowDefinition(ctx, definitionID)
	if err != nil {
		return WorkflowDefinitionInspect{}, err
	}
	revisions, err := s.deps.Workflow.ListWorkflowRevisions(ctx, definitionID)
	if err != nil {
		return WorkflowDefinitionInspect{}, err
	}
	return WorkflowDefinitionInspect{Definition: def, Revisions: revisions}, nil
}

// WorkflowRunHistory is the run-history query member — a pure read; no
// item-state change. THE D-40 GATE (carried exactly): this read is a
// LINKAGE record, not a Run schema — no Run/step lifecycle vocabulary, no
// run-state register, no Run/Job cardinality claim exists here; the read's
// only producer (the Run execution) is GATED, so every existing definition
// answers the honest empty page while the gate stands. Each entry renders
// beside its Core-reported source revision; missing linkage renders
// `unavailable` — NEVER an inferred link (RUN-LINK-3). The unknown
// definition is the SAME typed 404 (the no-leak rule).
func (s *Slice) WorkflowRunHistory(ctx stdcontext.Context, definitionID string) (workflowdomain.RunHistory, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.RunHistory{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: definitionID}, security.ActionDataRead); err != nil {
		return workflowdomain.RunHistory{}, err
	}
	if err := validateWorkflowIdentity(definitionID, "definition_id", corr); err != nil {
		return workflowdomain.RunHistory{}, err
	}
	entries, err := s.deps.Workflow.ListWorkflowRunHistory(ctx, definitionID)
	if err != nil {
		return workflowdomain.RunHistory{}, err
	}
	return workflowdomain.RunHistory{Entries: entries}, nil
}

// WorkflowDefinitionState is the family's authoritative state endpoint (a
// pure query — never a retry): resolved by (workspace, definition identity
// [+ revision identity]). A timeout or lost response is NEVER proof of
// success or safe retry (the S0–S4 model) — reconcile by the authoritative
// definition/revision identity through this pure query BEFORE any retry
// (VRC-LOST-1). The client's unsaved draft is never Core state (the
// no-silent-save law); the current immutable stored revision is what this
// read reports. Events are never the source of truth; this endpoint is.
// The unknown identity and the out-of-scope identity are the SAME typed
// 404 (the no-leak rule).
func (s *Slice) WorkflowDefinitionState(ctx stdcontext.Context, definitionID, revisionID string) (workflowdomain.DefinitionState, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Workflow == nil {
		return workflowdomain.DefinitionState{}, workflowNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceWorkflow, ID: definitionID}, security.ActionDataRead); err != nil {
		return workflowdomain.DefinitionState{}, err
	}
	if err := validateWorkflowIdentity(definitionID, "definition_id", corr); err != nil {
		return workflowdomain.DefinitionState{}, err
	}
	if len(revisionID) > maxWorkflowIDLen {
		return workflowdomain.DefinitionState{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "revision_id"})
	}
	return s.deps.Workflow.LoadWorkflowDefinitionState(ctx, definitionID, revisionID)
}

// workflowCursorPosition is the decoded form behind the opaque list
// cursor: the keyset position (the last page's sort-key values). It
// follows the shared salted-cursor PATTERN — a salt-prefixed hex-JSON
// envelope, never structured data on the wire.
type workflowCursorPosition struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeWorkflowCursor(def workflowdomain.Definition) (string, error) {
	if def.DefinitionID == "" || def.CreatedAt == "" {
		return "", fmt.Errorf("app: workflow list cursor position lacks its keyset values")
	}
	b, err := json.Marshal(workflowCursorPosition{CreatedAt: def.CreatedAt, ID: def.DefinitionID})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

func decodeWorkflowCursor(cursor string) (workflowCursorPosition, error) {
	var pos workflowCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: workflow cursor is malformed")
	}
	if pos.ID == "" || pos.CreatedAt == "" {
		return pos, fmt.Errorf("app: workflow cursor is malformed")
	}
	return pos, nil
}
