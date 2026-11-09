// Package memory defines the B2 Memory family's wire shapes and closed
// registers (the published canonical contract family of
// contracts/openapi.yaml — the Memory* schemas, the MemoryErrorCode
// register, the MemoryEvent* vocabulary, and the two Memory parameters,
// carried verbatim: never a restatement, never an extension). The package
// is part of the domain leaf: it imports no other internal package and
// carries no transport, database, or OS types. THE SECRETS LAW (BINDING,
// MB §7.5; MB §12.2 law 3): passwords, API keys, private keys, payment
// credentials, and other secrets are NEVER stored in Memory —
// secret-manager/key-store references only; the scanner below refuses a
// write attempting secret storage (MEMORY_SECRETS_REJECTED) and the law
// extends to exports, logs, URLs, and error messages. The Core-owned
// identity members and the authoritative state register live here; the UI
// renders Core-returned truth only.
package memory

// The closed memory-scope register (the published MemoryScope register
// carried verbatim). No Work value exists — DEFERRED per P1.2 until D-12
// approval; scope never silently broadens; cross-user/workspace visibility
// is forbidden by default.
const (
	ScopeUser          = "user"
	ScopeWorkspace     = "workspace"
	ScopeSession       = "session"
	ScopePolicyDefined = "policy-defined"
)

// ValidScope reports membership in the closed scope register.
func ValidScope(s string) bool {
	switch s {
	case ScopeUser, ScopeWorkspace, ScopeSession, ScopePolicyDefined:
		return true
	default:
		return false
	}
}

// The closed memory-origin register (the published MemoryOrigin register
// carried verbatim). A client add is user-entered — the Core's own paths
// set the others; model-extracted origin can only be set by the Core's own
// extraction path (no client body ever claims it).
const (
	OriginUserEntered      = "user-entered"
	OriginApprovedDecision = "approved-decision"
	OriginImported         = "imported"
	OriginModelExtracted   = "model-extracted"
)

// ValidOrigin reports membership in the closed origin register.
func ValidOrigin(s string) bool {
	switch s {
	case OriginUserEntered, OriginApprovedDecision, OriginImported, OriginModelExtracted:
		return true
	default:
		return false
	}
}

// ClientAddableOrigin reports whether a client body may claim the origin
// on the served add member. Only user-entered is a client claim — the
// Core's own paths (import, extraction, approved decisions) set the rest,
// and no such path exists on this surface (recorded honestly).
func ClientAddableOrigin(s string) bool {
	return s == OriginUserEntered
}

// The closed memory lifecycle-state register (the published
// MemoryItemLifecycleState register carried verbatim — the publication
// act's register for MB §7.5's pipeline). Trust state is DISTINCT from
// pending-review: pending review is a lifecycle state, NEVER a trust
// claim. `expired` is the retention/TTL outcome — DISTINCT from forgotten
// and deleted, never UI-computed. THE FORGET/DELETE DISTINCTNESS
// (recorded at the 2026-11-09 resolution act, binding on this family):
// forget and delete are DISTINCT Core semantics at the register level —
// the forgotten and deleted entries are the two DISTINCT lifecycle
// states (the forgotten state; the deleted state), each confirmed by its
// own event. `outcome_unknown` is the S0–S4 OUTCOME_UNKNOWN
// surface.
const (
	StateCaptured       = "captured"
	StateExtracted      = "extracted"
	StatePendingReview  = "pending_review"
	StateApproved       = "approved"
	StateRejected       = "rejected"
	StateActive         = "active"
	StateUpdated        = "updated"
	StateExpired        = "expired"
	StateForgotten      = "forgotten"
	StateDeleted        = "deleted"
	StateOutcomeUnknown = "outcome_unknown"
)

// ValidLifecycleState reports membership in the closed state register.
func ValidLifecycleState(s string) bool {
	switch s {
	case StateCaptured, StateExtracted, StatePendingReview, StateApproved,
		StateRejected, StateActive, StateUpdated, StateExpired, StateForgotten,
		StateDeleted, StateOutcomeUnknown:
		return true
	default:
		return false
	}
}

// Removed reports whether the state is a removal outcome (forgotten |
// deleted). A removed item's content is dropped; the identity tombstone
// stays for the reconcile law (the authoritative operation record carries
// the confirmed outcome).
func Removed(s string) bool {
	return s == StateForgotten || s == StateDeleted
}

// Editable reports the eligibility law's served class: the lifecycle
// states whose register carries the correction path (the Core owns
// eligibility — this class is this Core's served policy; the UI never
// decides it).
func Editable(s string) bool {
	return s == StateActive || s == StateUpdated
}

// Reviewable reports the review-transition requirement: only pending
// review items accept the explicit approve/reject actions (pending review
// is a lifecycle state, NEVER a trust claim; approval is always the
// explicit user action — AI-extracted content never becomes user-confirmed
// without it).
func Reviewable(s string) bool {
	return s == StatePendingReview
}

// PinTargetable reports the states whose register carries the pin flag
// (the pin register position is Core-defined per the recorded gap — this
// Core carries pin as a FLAG on the item, distinct from the lifecycle
// state). Removed items accept no pin change.
func PinTargetable(s string) bool {
	switch s {
	case StateCaptured, StateExtracted, StatePendingReview, StateApproved,
		StateRejected, StateActive, StateUpdated:
		return true
	default:
		return false
	}
}

// The closed memory-operation kind register (the published
// MemoryOperationKind register carried verbatim). The list/query and
// inspect reads are correlation-class reads with no operation row.
const (
	KindAdd       = "add"
	KindEdit      = "edit"
	KindApprove   = "approve"
	KindReject    = "reject"
	KindPin       = "pin"
	KindUnpin     = "unpin"
	KindForget    = "forget"
	KindDelete    = "delete"
	KindExport    = "export"
	KindInclusion = "inclusion"
)

// ValidOperationKind reports membership in the closed kind register.
func ValidOperationKind(s string) bool {
	switch s {
	case KindAdd, KindEdit, KindApprove, KindReject, KindPin, KindUnpin,
		KindForget, KindDelete, KindExport, KindInclusion:
		return true
	default:
		return false
	}
}

// The closed memory-operation state register (the published
// MemoryOperationState register carried verbatim). `recorded` carries the
// Core-confirmed outcome in the record's result members;
// `outcome_unknown` is the S0–S4 OUTCOME_UNKNOWN surface — reconcile by
// the operation ID through the operation-status read (or by the item
// identity through the inspect read) BEFORE any retry. A failure is a
// recorded, confirmed outcome.
const (
	OpStateRecorded      = "recorded"
	OpStateOutcomeUnknow = "outcome_unknown"
)

// ValidOperationState reports membership in the closed operation-state
// register.
func ValidOperationState(s string) bool {
	return s == OpStateRecorded || s == OpStateOutcomeUnknow
}

// The MemoryErrorCode register (the published family codes carried
// verbatim — each riding the published ErrorEnvelope with its retryable
// value and the coded retryClass; the codes join the coded register
// additively at their implementation, the established register-extension
// class).
const (
	FailScopeViolation      = "MEMORY_SCOPE_VIOLATION"
	FailSecretsRejected     = "MEMORY_SECRETS_REJECTED"
	FailItemNotFound        = "MEMORY_ITEM_NOT_FOUND"
	FailReviewStateConflict = "MEMORY_REVIEW_STATE_CONFLICT"
	FailConflict            = "MEMORY_CONFLICT"
	FailNotEditable         = "MEMORY_NOT_EDITABLE"
)

// The F10-shaped event aggregate types (the published register carried
// verbatim; events ride the frozen F10 EventRecord field set and are never
// the source of truth — the authoritative state reads are).
const (
	AggregateMemoryItem            = "memoryItem"
	AggregateMemoryReviewOperation = "memoryReviewOperation"
)

// The F10-shaped event_type vocabulary (the published register carried
// verbatim; no type outside this list is ever emitted).
// `memory_item.forgotten` is DISTINCT from `memory_item.deleted` — THE
// FORGET/DELETE DISTINCTNESS (recorded at the 2026-11-09 resolution act,
// binding on this family): forget and delete are DISTINCT Core semantics
// at the register level — the forgotten lifecycle state and the
// memory_item.forgotten event vs the deleted state and the
// memory_item.deleted event. This Core emits the forgotten event only
// from the forget member's confirmed outcome and the deleted event only
// from the delete member's confirmed outcome — the recorded mechanics
// are this Core's own. `memory_candidate.extracted` is the sensitivity
// check's routing event (auto-save low-risk vs approval-required) — no
// extraction path exists on this surface, so it is never emitted.
const (
	EventCandidateExtracted = "memory_candidate.extracted"
	EventItemAdded          = "memory_item.added"
	EventReviewApproved     = "memory_review.approved"
	EventReviewRejected     = "memory_review.rejected"
	EventItemUpdated        = "memory_item.updated"
	EventItemPinned         = "memory_item.pinned"
	EventItemUnpinned       = "memory_item.unpinned"
	EventItemExpired        = "memory_item.expired"
	EventItemForgotten      = "memory_item.forgotten"
	EventItemDeleted        = "memory_item.deleted"
)

// UntrustedLabel is the one coded value of the published untrusted-content
// label register; imported-origin items ride it and cannot grant
// permissions. No imported-origin item is creatable on this surface (the
// Core's own import path for memory does not exist here — recorded
// honestly), so the label is never set by this implementation.
const UntrustedLabel = "untrusted"

// Item is one durable memory item (the published MemoryItem schema's
// members carried verbatim; JSON tags are the contract member names).
// THE SECRETS LAW (BINDING): no member of this schema carries a secret
// value — a write attempting secret storage is refused
// (MEMORY_SECRETS_REJECTED) and a reference renders as a reference.
// THE PROMOTION BOUNDARY: Memory stays separate from canonical
// KnowledgePack truth; promotion is a SEPARATE explicit reviewed
// operation (the recorded sibling gap — NOT part of this family): no
// promotion member exists here and none is invented. Memory deletion
// never auto-deletes KnowledgePack claims.
// Pinned is this Core's served definition of the recorded pin-register
// gap (a flag vs a lifecycle state — Core-defined; P4.5 G-6): a FLAG on
// the item, never a lifecycle state (the closed lifecycle register
// carries no pinned value), riding every served item exactly as the
// pin/unpin member's description requires ("the Core-confirmed pin state
// rides the response's item"). The published MemoryItem schema carries no
// additionalProperties:false, so the additional member is schema-valid;
// the typing is recorded honestly in the implementation record.
type Item struct {
	MemoryItemID    string   `json:"memory_item_id"`
	Scope           string   `json:"scope"`
	Origin          string   `json:"origin"`
	LifecycleState  string   `json:"lifecycle_state"`
	Content         string   `json:"content"`
	Pinned          bool     `json:"pinned"`
	CreationMethod  string   `json:"creation_method,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	OriginatingRefs []string `json:"originating_refs,omitempty"`
	RetentionTTLRef string   `json:"retention_ttl_ref,omitempty"`
	ConflictNote    string   `json:"conflict_note,omitempty"`
	UntrustedLabel  string   `json:"untrusted_label,omitempty"`
	CreatedAt       string   `json:"created_at"`
	LastUsedAt      string   `json:"last_used_at,omitempty"`
	LastConfirmedAt string   `json:"last_confirmed_at,omitempty"`
	UpdatedAt       string   `json:"updated_at"`
}

// ItemList is one bounded memory-items page (the keyset list semantics;
// the cursor's absence marks the final page).
type ItemList struct {
	Items  []Item `json:"items"`
	Cursor string `json:"cursor,omitempty"`
}

// MutationResult is one state-confirmed Memory mutation's outcome (the
// add, edit, approve, reject, pin/unpin, and forget members) — the
// Core-confirmed post-state item and the operation reference. Only a
// confirmed 200 carries the outcome truth (the F11 law); the request is
// never displayed as the confirmation (the J-5 rule). The operation_id is
// the reconciliation identity for a lost response (the S0–S4 law):
// reconcile through the operation-status read or the item's inspect read
// BEFORE any retry — never a blind re-issue.
type MutationResult struct {
	MemoryItem  Item   `json:"memory_item"`
	OperationID string `json:"operation_id"`
}

// DeleteResult is the delete member's Core-confirmed outcome — the
// deletion truth and the retention caveat the Core confirms, never a
// total-erasure promise. Memory deletion never auto-deletes KnowledgePack
// claims.
type DeleteResult struct {
	MemoryItemID  string `json:"memory_item_id"`
	OutcomeState  string `json:"outcome_state"`
	RetentionNote string `json:"retention_note,omitempty"`
	OperationID   string `json:"operation_id"`
}

// InclusionResult is the per-request-inclusion member's Core-confirmed
// outcome — a decision record, not an item-state change (the M-18 class).
type InclusionResult struct {
	MemoryItemID string `json:"memory_item_id"`
	RequestRef   string `json:"request_ref"`
	Included     bool   `json:"included"`
	OperationID  string `json:"operation_id"`
}

// ExportResult is the export document — a read-with-output (no item-state
// change); the honest count, the generation instant, and the honest
// truncation flag. SECRETS ARE EXCLUDED BY DEFAULT (a prohibition, not a
// preference): no secret value ever enters an export artifact.
type ExportResult struct {
	ExportOperationID string `json:"export_operation_id"`
	Count             int    `json:"count"`
	GeneratedAt       string `json:"generated_at"`
	Truncated         bool   `json:"truncated"`
	Items             []Item `json:"items"`
}

// Operation is one memory operation's authoritative record (the published
// MemoryOperation schema's members carried verbatim) — the
// operation-status endpoint's payload (the S0–S4 unknown-outcome lookup's
// operation-ID half).
type Operation struct {
	OperationID   string `json:"operation_id"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	CorrelationID string `json:"correlation_id,omitempty"`
	MemoryItemID  string `json:"memory_item_id,omitempty"`
	OutcomeState  string `json:"outcome_state,omitempty"`
	ExportRef     string `json:"export_ref,omitempty"`
	FailureCode   string `json:"failure_code,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// AddIntent is the add member's durable intent: the Core-minted item to
// store (state per the entry-state policy — the response's lifecycle_state
// carries the truth), the operation record, and the M-17 keying members
// (the effective key is the client key scoped by the stated scope
// Core-side; the fingerprint is the request identity — the correlation
// ID).
type AddIntent struct {
	Item           Item
	Operation      Operation
	IdempotencyKey string
	Fingerprint    string
}

// MutationIntent is one M-16 mutation's durable intent (edit | approve |
// reject | pin | unpin | forget | delete): the operation record and the
// state-checked keying members (the effective key is the client key
// scoped by the item identity + scope Core-side; the fingerprint is the
// correlation ID + the item's current state reference). EditContent rides
// the edit member's corrected content; Pinned rides the pin/unpin member's
// direction; StateRef rides the edit member's prepared-against reference
// (a mismatch is MEMORY_CONFLICT — reconcile by re-reading the item's
// authoritative state, never overwrite).
type MutationIntent struct {
	Kind         string
	Operation    Operation
	MemoryItemID string
	// ClientKey is the caller's RAW key — the store builds the effective
	// key (scoped user + item identity + scope Core-side) INSIDE the
	// transaction where the item's scope is read, so a lost destructive
	// response's keyed retry reaches the replay ruling even after the item
	// is removed (the S0–S4 reconcile law).
	ClientKey   string
	Fingerprint string
	EditContent string
	StateRef    string
	Pinned      bool
}

// InclusionIntent is the inclusion member's durable intent (the M-18
// class): the decision applies ONLY to the stated request/scope, changes
// no item state, and has no global effect. The key scope is the stated
// request/scope ONLY; the fingerprint is the request's correlation ID +
// the included item IDs; the retention is request-lifetime at most
// (implemented as the decision row's own lifetime — recorded honestly).
type InclusionIntent struct {
	Operation      Operation
	MemoryItemID   string
	RequestRef     string
	Included       bool
	IdempotencyKey string
	Fingerprint    string
}

// MutationOutcome is one mutation's store-confirmed outcome: the
// post-state item and the operation reference (replayed reports which
// path served the call — the M-16/M-17 replay law).
type MutationOutcome struct {
	Item        Item
	OperationID string
	Replayed    bool
}

// InclusionOutcome is one inclusion decision's store-confirmed outcome.
type InclusionOutcome struct {
	Result   InclusionResult
	Replayed bool
}

// ExportIntent is the export member's durable record intent (the M-32
// read-with-output class): mutates nothing, no item-state change, safe to
// re-issue — NO idempotency key, NO replay window. ExportRef carries the
// produced document's content digest (the artifact reference — a content
// identity; no artifact retrieval surface exists on this family, recorded
// honestly).
type ExportIntent struct {
	Operation Operation
	ExportRef string
}

// ListQuery is one keyset list request: the bounded page size, the opaque
// cursor position (the last page's sort-key values), and the optional
// scope/state filters (the scope filter is part of the read authorization
// — scope never silently broadens; an unknown reported scope is never
// mapped to a known one).
type ListQuery struct {
	Limit          int
	Scope          string
	State          string
	AfterCreatedAt string
	AfterID        string
}

// ItemPage is one list page: the rows plus whether more exist beyond the
// page (the cursor's mint is the service's; absence marks the final page).
type ItemPage struct {
	Items []Item
	More  bool
}

// StateRef builds the item's current state reference — the M-16
// fingerprint's state half (the lifecycle state + the item's last
// transition instant; this composite is this Core's state-reference
// typing). A client edit prepared against a stale reference is the
// MEMORY_CONFLICT concurrent-edit outcome — reconcile by re-reading the
// item's authoritative state, never overwrite.
func StateRef(lifecycleState, updatedAt string) string {
	return lifecycleState + "@" + updatedAt
}

// The Core-side idempotency key-scope builders (the published key scopes
// carried as the effective-key constructions; the client key is scoped by
// the named dimensions Core-side). The M-17 key is scoped user + scope (+
// session where session-scoped): Local mode's single-owner posture supplies
// the user dimension; the session dimension has no separate producer on
// this surface (recorded honestly). The M-16 key is scoped user + item
// identity + scope. The M-18 key is scoped to the stated request/scope
// ONLY (the route's item identity rides the key; no user or workspace
// dimension).
const (
	keySeparator           = "\x1f"
	CorrelationFingerprint = "\x1f"
	inclusionKeyPrefix     = "memincl:"
)

// ScopeKey builds the M-17 effective key (the client key scoped by the
// stated scope).
func ScopeKey(clientKey, scope string) string {
	return clientKey + keySeparator + "scope:" + scope
}

// ItemKey builds the M-16 effective key (the client key scoped by the item
// identity + scope).
func ItemKey(clientKey, itemID, scope string) string {
	return clientKey + keySeparator + "item:" + itemID + keySeparator + "scope:" + scope
}

// InclusionKey builds the M-18 effective key (scoped to the stated
// request/scope ONLY).
func InclusionKey(requestRef, itemID string) string {
	return inclusionKeyPrefix + requestRef + keySeparator + itemID
}
