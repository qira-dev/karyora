// Package context defines the B3 ContextSnapshot family's wire shapes and
// closed registers (the published canonical contract family of
// contracts/openapi.yaml — the Context* schemas, the ContextErrorCode
// register, the ContextEvent* vocabulary, and the ContextOperationId
// parameter, carried verbatim: never a restatement, never an extension).
// The package is part of the domain leaf: it imports no other internal
// package and carries no transport, database, or OS types.
//
// THE CONDITIONAL COMMANDS (the published semantics carried exactly):
// saveContextSnapshot (the M-19 class) is CONDITIONAL on durable snapshot
// storage existing — the GC-3 un-gate precondition, the owner's PROPOSED
// decision, NOT resolved: while the condition is unmet the command serves
// the typed unmet-condition refusal, no operation row can exist, and no
// fake success path exists anywhere. compareContextSnapshots (the M-20
// class) is CONDITIONAL on stable Core comparison existing — while it
// does not, the family answers the typed CONTEXT_COMPARE_UNSUPPORTED and
// no diff is ever fabricated. setContextInclusion (the M-18 class) is
// UNCONDITIONAL: a request-scoped decision implemented fully.
//
// THE F5 NON-DUPLICATION LAW (BINDING): this family never duplicates,
// redefines, or modifies the served Job-flow snapshot read
// (/jobs/{id}/snapshot → SnapshotView) — the compiled content is never
// carried anywhere; the extension composes ADDITIVELY alongside the
// served read. THE NON-CANONICAL LAW: a saved snapshot is NOT canonical
// knowledge truth. THE TWO CONDITIONAL EVENT LAWS: `snapshot.saved` is
// recorded CONDITIONAL on durable snapshot storage existing and
// `compilation.actual_reported` is recorded CONDITIONAL on the
// provider/Core usage-reporting fields existing — NEITHER event is
// produced while its condition is unmet (the owner's PROPOSED decisions;
// nothing is extrapolated).
package context

// The closed context-operation kind register (the published
// ContextOperationKind register carried verbatim). The compare is the
// read-with-output class and the inclusion decision is request-scoped —
// neither carries a durable operation row; the compilation itself remains
// the served Job flow (F5/getJobStatus) — never duplicated here.
const KindSaveSnapshot = "save_snapshot"

// ValidOperationKind reports membership in the closed kind register.
func ValidOperationKind(s string) bool {
	return s == KindSaveSnapshot
}

// The closed context-operation state register (the published
// ContextOperationState register carried verbatim). `accepted` is the
// submit confirmation (202 = accepted for processing, never completion —
// the F6 law). `saved` is the Core-confirmed terminal state (the snapshot
// durably stored under the selected 72 h post-terminal retention window).
// `failed` carries the explicit failure code and reason — never a
// suppressed warning. `outcome_unknown` is the S0–S4 OUTCOME_UNKNOWN
// surface: a timeout or lost response is never proof of success or safe
// retry — reconcile by the operation ID through the authoritative state
// read.
const (
	OpStateAccepted      = "accepted"
	OpStateSaved         = "saved"
	OpStateFailed        = "failed"
	OpStateOutcomeUnknow = "outcome_unknown"
)

// ValidOperationState reports membership in the closed operation-state
// register.
func ValidOperationState(s string) bool {
	switch s {
	case OpStateAccepted, OpStateSaved, OpStateFailed, OpStateOutcomeUnknow:
		return true
	default:
		return false
	}
}

// The ContextErrorCode register (the published family codes carried
// verbatim — each riding the published ErrorEnvelope with its published
// retryable value and the coded retryClass; the codes join the coded
// register additively at their implementation, the established
// register-extension class). The idempotency/state conflicts ride the
// PUBLISHED codes exactly (CONFLICT_IDEMPOTENCY_KEY / CONFLICT_STATE).
const (
	// FailBudgetExhausted — the budget-exhaustion outcome producing the
	// explicit omission/partial-context record (the no-silent-truncation
	// budget law, MB §18); retryClass never (a repeat with the same
	// budget re-exhausts; the omission record is the explicit outcome).
	FailBudgetExhausted = "CONTEXT_BUDGET_EXHAUSTED"
	// FailOmissionReasonUnavailable — the typed carrier for the honest
	// reason-unavailable state: the Core reports an omission without a
	// reason and the absence is displayed as the absence, never a
	// synthesized reason (P4.6 ST-3); retryClass safe-retry.
	FailOmissionReasonUnavailable = "CONTEXT_OMISSION_REASON_UNAVAILABLE"
	// FailCompilationUnavailable — the compilation/preview cannot be
	// served; the honest unavailable state renders, never a fabricated
	// state (P4.6 ST-9/C7); retryClass safe-retry.
	FailCompilationUnavailable = "CONTEXT_COMPILATION_UNAVAILABLE"
	// FailCompareUnsupported — compare attempted without stable Core
	// comparison support; safe to re-issue as a read-only effect-free
	// read while the outcome persists until support exists (the GC-4
	// law); never a fabricated diff; retryClass safe-retry.
	FailCompareUnsupported = "CONTEXT_COMPARE_UNSUPPORTED"
	// FailSnapshotNotFound — the unknown snapshot/operation identity, the
	// SAME typed 404 for unknown and out-of-scope (the no-leak rule; the
	// typed-404 pattern is the published precedent); retryClass never.
	FailSnapshotNotFound = "CONTEXT_SNAPSHOT_NOT_FOUND"
)

// The F10-shaped event aggregate types (the published register carried
// verbatim; events ride the frozen F10 EventRecord field set and are never
// the source of truth — the authoritative state endpoint is).
const (
	AggregateContextSnapshot    = "contextSnapshot"
	AggregateContextCompilation = "contextCompilation"
)

// The F10-shaped event_type vocabulary (the published register carried
// verbatim; no type outside this list is ever emitted).
// `compilation.omission_recorded` is the explicit omission record — the
// no-silent-truncation budget law (MB §18). `snapshot.saved` is recorded
// CONDITIONAL on durable snapshot storage existing — the GC-3 un-gate
// precondition is the owner's PROPOSED decision and NO snapshot.saved
// event can exist while it is open. `compilation.actual_reported` is
// recorded CONDITIONAL on the provider/Core usage-reporting fields
// existing — the owner's PROPOSED decision; where unreported, the honest
// "not yet reported" state stands (P4.6 ST-6) and nothing is extrapolated.
const (
	EventOmissionRecorded = "compilation.omission_recorded"
	EventSnapshotSaved    = "snapshot.saved"
	EventActualReported   = "compilation.actual_reported"
)

// The two conditional support gates (the published CONDITIONAL semantics
// carried as the family's own support state — each the owner's PROPOSED
// decision, NOT resolved by any implementation act; flipping either is the
// un-gate act, its own LIVE-gate class, never a side effect).
//
// DurableSnapshotStorageSupported: the GC-3 un-gate precondition for the
// M-19 save-snapshot command. FALSE today — the owner's durable-storage
// decision is PROPOSED and unresolved; while false, the save serves the
// typed unmet-condition refusal, no operation row can exist, and
// snapshot.saved is never emitted.
//
// StableComparisonSupported: the GC-4 un-gate precondition for the M-20
// snapshot compare. FALSE today — no stable Core comparison exists; while
// false, the family answers CONTEXT_COMPARE_UNSUPPORTED and no diff is
// ever fabricated.
const (
	DurableSnapshotStorageSupported = false
	StableComparisonSupported       = false
)

// SnapshotSaveRetentionNote is the save member's selected replay-window
// statement carried in descriptions (the F6-precedent window selected at
// the publication act): 72 h post-terminal. It is carried as the retention
// WINDOW only; no durable snapshot storage exists to which it applies
// while the gate above stands.
const SnapshotSaveReplayWindowNote = "72 h post-terminal (the F6-precedent window selected at the publication act)"

// MetadataExtension is the ADDITIVE metadata extension of the served
// Job-flow snapshot read (F5 /jobs/{id}/snapshot → SnapshotView) toward
// the Core MB §7.3 required-metadata list — the published
// ContextSnapshotMetadataExtension schema's members carried verbatim
// (JSON tags are the contract member names). This type NEVER duplicates,
// redefines, or modifies the served SnapshotView or its member schemas:
// the served members (task_ref, user_input_digest, included, omitted,
// budget, compression_method, retrieval_config_version, estimator_version,
// renderer_version, assembled_digest) are the published forms this
// extension extends — never restated here.
//
// RECORDED GAPS (never fabricated members — the published schema's own
// description carries them): the tool-result/previous-output item-class
// vocabulary is unsupplied (row 3 — no member exists); the provider/Core
// usage-reporting fields are UNSUPPLIED — the owner's PROPOSED decision
// (row 8 — NO field designed; the honest "not yet reported" state applies
// and nothing is extrapolated); the privacy/data-classification decisions
// are the Core's — no UI-minted member exists (row 10); the
// prompt/template/system-instruction and output-schema version refs are
// unsupplied (row 12 — no member exists); the concrete
// pack/claim/evidence/source breakdown classes are the owner's residual
// (row 2's residual — SourceDigests records the digests only).
//
// THE D-41 NOTE rides every digest member: the algorithm-qualified digest
// form `sha256:<lowercase-hex>` is a value FORMAT carried in descriptions
// only — the D-41 portable digest profile stays behind its owner
// technical/security gate; nothing here approves SHA-256 or any algorithm.
//
// Every member is present only where the Core reports it — absence is the
// honest unreported state, never a synthesized value. The P1.5 R5 law:
// model/provider optimization metadata is NEVER canonical context
// identity.
type MetadataExtension struct {
	WorkflowRef         *string        `json:"workflow_ref,omitempty"`
	RunRef              *string        `json:"run_ref,omitempty"`
	SourceDigests       *SourceDigests `json:"source_digests,omitempty"`
	CompressionVersion  *string        `json:"compression_version,omitempty"`
	SourceSnapshotRefs  []string       `json:"source_snapshot_refs,omitempty"`
	UsedBudget          *UsedBudget    `json:"used_budget,omitempty"`
	ModelRef            *string        `json:"model_ref,omitempty"`
	CapabilityProfile   *string        `json:"capability_profile,omitempty"`
	SnapshotDigest      *string        `json:"snapshot_digest,omitempty"`
	PolicySignatureRefs []string       `json:"policy_signature_refs,omitempty"`
	ProviderAdapterVer  *string        `json:"provider_adapter_version,omitempty"`
	RepresentationVer   *string        `json:"representation_version,omitempty"`
	RawPayloadStored    *bool          `json:"raw_payload_stored,omitempty"`
}

// SourceDigests is the selected pack/claim/evidence/source digests beyond
// the served flat included items (the published
// ContextSnapshotSourceDigests schema carried verbatim) — the served
// per-item form (SnapshotItem) is flat and untouched. The typed
// pack/claim/evidence/source breakdown's concrete classes are the owner's
// residual: this object records the digests the Core reports per class and
// defines no pack/claim/evidence wire shape. An absent member = the Core
// reports no digest for the class — never fabricated.
type SourceDigests struct {
	Packs    []string `json:"packs,omitempty"`
	Claims   []string `json:"claims,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	Sources  []string `json:"sources,omitempty"`
}

// UsedBudget is the used budget and the provider-overhead extension (the
// published ContextSnapshotUsedBudget schema carried verbatim) — the
// served requested form (SnapshotBudgetRequested) and the Core estimate
// (SnapshotBudget) are untouched; reserved output IS already served in the
// requested form. THE USAGE-REPORTING HONESTY LAW: the provider/Core
// usage-reporting fields are UNSUPPLIED — the owner's PROPOSED decision;
// the honest "not yet reported" state applies until they exist (P4.6
// ST-6) — a member is present only where the Core reports it, never
// extrapolated, never synthesized. Pointer members: absent = not yet
// reported.
type UsedBudget struct {
	UsedInputTokens       *int `json:"used_input_tokens,omitempty"`
	UsedOutputTokens      *int `json:"used_output_tokens,omitempty"`
	ProviderOverheadToken *int `json:"provider_overhead_tokens,omitempty"`
}

// SnapshotSaveRequest is the save-snapshot member's body (the published
// ContextSnapshotSaveRequest schema carried verbatim) — the M-19
// Core-confirmed command class. The idempotency key is scoped workspace +
// request context Core-side; the fingerprint is the request identity (the
// correlation ID). CONDITIONAL on durable snapshot storage existing (the
// owner's PROPOSED decision): the typed unmet-condition refusal is served
// while the condition stands — never a fake success.
type SnapshotSaveRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	SourceJobID    string `json:"source_job_id"`
}

// SavedSnapshotResult is the Core-confirmed saved-snapshot reference (the
// published ContextOperation.result members carried verbatim) — present
// only on the Core-confirmed saved outcome.
type SavedSnapshotResult struct {
	SnapshotID    string `json:"snapshot_id,omitempty"`
	RetentionNote string `json:"retention_note,omitempty"`
}

// SnapshotSaveResult is one save's accepted outcome (the published
// ContextSnapshotSaveResult schema carried verbatim) — the operation
// reference (replayed=true on the M-19 same-request-identity replay). 202
// = accepted for processing, never completion (the F6 law); the
// authoritative operation-state read confirms every outcome; the request
// is never displayed as the confirmation (the J-5 rule). No instance of
// this result is produced while the durable-storage gate stands.
type SnapshotSaveResult struct {
	Operation Operation `json:"operation"`
	Replayed  bool      `json:"replayed"`
}

// SnapshotCompareRequest is the compare member's body (the published
// ContextSnapshotCompareRequest schema carried verbatim) — the M-20
// read-only class. READ-ONLY and CONDITIONAL on stable Core comparison
// existing (the GC-4 un-gate precondition) — never a fabricated diff.
type SnapshotCompareRequest struct {
	BaselineSnapshotID  string `json:"baseline_snapshot_id"`
	CandidateSnapshotID string `json:"candidate_snapshot_id"`
}

// SnapshotCompareResult is the compare member's Core-reported output (the
// published ContextSnapshotCompareResult schema carried verbatim) — the
// read-with-output result (mutates nothing). The comparison content is
// the Core's own stable comparison output — no diff/delta vocabulary is
// defined by this family; a compare runs ONLY where stable Core
// comparison exists and no diff is ever fabricated. No instance is
// produced while the comparison gate stands.
type SnapshotCompareResult struct {
	BaselineSnapshotID  string         `json:"baseline_snapshot_id"`
	CandidateSnapshotID string         `json:"candidate_snapshot_id"`
	Comparison          map[string]any `json:"comparison"`
}

// InclusionDecision is one per-item inclusion decision inside the M-18
// request (the published ContextInclusionRequest.decisions items carried
// verbatim).
type InclusionDecision struct {
	MemoryItemID string `json:"memory_item_id"`
	Included     bool   `json:"included"`
}

// InclusionRequest is the per-request memory-inclusion member's body (the
// published ContextInclusionRequest schema carried verbatim) — the M-18
// request-scoped decision. It applies ONLY to the stated request, changes
// no Memory item's lifecycle state, creates no Memory action row (the
// P4.6 §4 boundary), and never widens any scope.
type InclusionRequest struct {
	CorrelationID string              `json:"correlation_id"`
	RequestRef    string              `json:"request_ref"`
	Decisions     []InclusionDecision `json:"decisions"`
}

// InclusionDecisionResult is one decision's Core-confirmed outcome (the
// published ContextInclusionResult.decisions items carried verbatim).
// Reason is the Core-reported outcome reason where the Core's context
// policy reports one — carried verbatim, never synthesized; absence is
// the absence (the Core-provided-only reason law). No reason producer
// exists on this surface today (recorded honestly), so reason is never
// set.
type InclusionDecisionResult struct {
	MemoryItemID string  `json:"memory_item_id"`
	Included     bool    `json:"included"`
	Reason       *string `json:"reason,omitempty"`
}

// InclusionResult is the per-request memory-inclusion member's
// Core-confirmed outcome (the published ContextInclusionResult schema
// carried verbatim) — a decision record for the stated request ONLY,
// never an item-state change and never a Memory action row.
type InclusionResult struct {
	RequestRef string                    `json:"request_ref"`
	Decisions  []InclusionDecisionResult `json:"decisions"`
}

// Operation is one context operation's authoritative record (the
// published ContextOperation schema's members carried verbatim) — the
// state endpoint's payload (the S0–S4 unknown-outcome lookup's
// operation-ID half).
type Operation struct {
	OperationID   string               `json:"operation_id"`
	Kind          string               `json:"kind"`
	State         string               `json:"state"`
	CorrelationID string               `json:"correlation_id"`
	SourceJobID   string               `json:"source_job_id,omitempty"`
	Result        *SavedSnapshotResult `json:"result,omitempty"`
	FailureCode   string               `json:"failure_code,omitempty"`
	FailureReason string               `json:"failure_reason,omitempty"`
	CreatedAt     string               `json:"created_at"`
	UpdatedAt     string               `json:"updated_at"`
}

// InclusionIntent is the inclusion member's durable intent (the M-18
// class): the decision applies ONLY to the stated request/scope, changes
// no item state, and has no global effect. The key scope is the stated
// request/scope ONLY (the decision rows' own (request_ref, item) scope IS
// the key carrier — no operation row and no durable idempotency key); the
// fingerprint is the request's correlation ID + the decided item IDs; the
// retention is request-lifetime at most (implemented as the decision
// row's own lifetime — recorded honestly).
type InclusionIntent struct {
	CorrelationID string
	RequestRef    string
	Decisions     []InclusionDecision
}

// InclusionOutcome is one inclusion call's store-confirmed outcome: the
// Core-confirmed decision record for the stated request.
type InclusionOutcome struct {
	Result InclusionResult
}
