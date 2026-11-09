package app

import (
	"context"
	"encoding/json"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/security"
)

// The B2 Memory family service (DE-0 queue row 3): the published /memories
// route family's use cases, implemented against the canonical contract at
// the working HEAD — the M-17 creation class (72 h post-terminal), the
// M-16 state-checked class (1 h post-terminal, the destructive members
// included), the M-18 request-scoped inclusion class, the M-32
// read-with-export class (none required), the authoritative state
// endpoint, the S0–S4 reconcile-by-item/operation-ID law, the BINDING
// secrets law (secrets NEVER stored — the refusal scanner; extends to
// exports, logs, URLs, and error messages), the promotion boundary (no
// promotion operation exists or is invented), the scope-never-broadens and
// cross-scope-default laws, the eligibility law (the Core owns
// eligibility), the typed-404 no-leak rule, and the F10-shaped event
// vocabulary through the existing journal machinery. Every protected
// effect rides the single decision port (security.ResourceMemory — the
// additive register extension; Local mode stays role-free: no permission
// policy is invented). The store owns the durable wall-clock values and
// the transactional rulings, exactly like the B1 KnowledgePack precedent.

// MemoryStore is the durable Memory surface port (B2). The storage adapter
// implements it over the memory tables (migration 0014); the slice consumes
// only this port. Nil disables the surface — typed refusals, fail closed
// (the B1 KnowledgeStore pattern).
type MemoryStore interface {
	// SubmitMemoryAdd applies the M-17 ruling and stores the fresh item
	// with its operation record and the memory_item.added event when no
	// replay resolves. replayed reports which path served the call.
	SubmitMemoryAdd(ctx context.Context, intent memory.AddIntent) (memory.MutationOutcome, error)
	// ApplyMemoryMutation applies the M-16 state-checked ruling for one
	// mutation member (edit | approve | reject | pin | unpin | forget |
	// delete) with the per-kind preconditions inside the transaction.
	ApplyMemoryMutation(ctx context.Context, intent memory.MutationIntent) (memory.MutationOutcome, error)
	// SetMemoryInclusion applies the M-18 request-scoped decision (no item
	// state change, no global effect).
	SetMemoryInclusion(ctx context.Context, intent memory.InclusionIntent) (memory.InclusionOutcome, error)
	// SubmitMemoryExport records the export operation (the M-32
	// read-with-output class: mutates nothing, no idempotency key, no
	// replay window).
	SubmitMemoryExport(ctx context.Context, intent memory.ExportIntent) (memory.Operation, error)
	// LoadMemoryItem resolves the authoritative item read; unknown and
	// removed items are the SAME typed MEMORY_ITEM_NOT_FOUND (the no-leak
	// rule).
	LoadMemoryItem(ctx context.Context, itemID string) (memory.Item, error)
	// ListMemoryItems reads one bounded keyset page with the optional
	// scope/state filters.
	ListMemoryItems(ctx context.Context, q memory.ListQuery) (memory.ItemPage, error)
	// MemoryItemsByID loads the named items in the caller's order (the
	// export member's explicit-subset read).
	MemoryItemsByID(ctx context.Context, itemIDs []string) ([]memory.Item, error)
	// LoadMemoryOperation resolves the authoritative state read; an
	// unknown operation is the typed NOT_FOUND (the no-leak rule).
	LoadMemoryOperation(ctx context.Context, opID string) (memory.Operation, error)
}

// Memory list and request bounds (the coded bounds class: identifiers ride
// the 128-char bound; display labels ride 512; content and export pages
// ride their own bounded values — bounded, never unbounded).
const (
	DefaultMemoryListLimit   = 50
	MaxMemoryListLimit       = 1000
	maxMemoryIDLen           = 128
	maxMemoryLabelLen        = 512
	MaxMemoryContentBytes    = 1 << 20
	MaxMemoryOriginatingRefs = 16
	MaxMemoryExportItems     = 500
)

// MemoryListRequest is one bounded list request (the M-1 correlation-only
// read class).
type MemoryListRequest struct {
	Limit  int
	Cursor string
	Scope  string
	State  string
}

// MemoryItemListResult is one bounded item page.
type MemoryItemListResult struct {
	Items  []memory.Item
	Cursor string
}

// MemoryAddRequest is one add (the published MemoryAddRequest members as
// the service contract).
type MemoryAddRequest struct {
	IdempotencyKey  string
	CorrelationID   string
	Scope           string
	Origin          string
	Content         string
	OriginatingRefs []string
}

// MemoryEditRequest is one edit/correct (the published MemoryEditRequest
// members as the service contract).
type MemoryEditRequest struct {
	IdempotencyKey string
	CorrelationID  string
	ItemStateRef   string
	Content        string
}

// MemoryReviewRequest is one review action's body (the published
// MemoryReviewRequest members — the approve and reject members).
type MemoryReviewRequest struct {
	IdempotencyKey string
	CorrelationID  string
}

// MemoryPinRequest is one pin/unpin body (the published MemoryPinRequest
// members — one member operation covering both directions).
type MemoryPinRequest struct {
	IdempotencyKey string
	CorrelationID  string
	Pinned         bool
}

// MemoryInclusionRequest is one per-request inclusion body (the published
// MemoryInclusionRequest members — the M-18 class).
type MemoryInclusionRequest struct {
	CorrelationID string
	RequestRef    string
	Included      bool
}

// MemoryExportRequest is one export body (the published
// MemoryExportRequest members — the M-32 class).
type MemoryExportRequest struct {
	CorrelationID string
	Scope         string
	ItemIDs       []string
	Destination   string
}

// MemoryReviewCommand is one destructive member's command body (the
// PUBLISHED CARRIER GAP, recorded honestly: the published delete and
// forget operations carry NO requestBody and no header parameter while
// their descriptions name the M-16 keying members — the keying members'
// published carrier is missing. This implementation accepts the keying
// members through an OPTIONAL request body (the adapter's /v1 convention)
// and serves the full M-16 ruling when keyed; an unkeyed call records a
// non-replayable destructive operation (no key consumed — the M-16 replay
// is unreachable for it). The gap is a recorded contract finding, never a
// silent edit; changing the canonical contract is a LIVE-gate class).
type MemoryReviewCommand struct {
	IdempotencyKey string
	CorrelationID  string
}

// MemoryDeleteRetentionNote is the delete member's Core-confirmed
// retention caveat (the M-16 retention-caveat rule — the honest statement,
// never a total-erasure promise).
const MemoryDeleteRetentionNote = "the item's content is removed from the active store; the identity tombstone record is retained for the reconcile law; no replica or backup erasure is promised beyond this Core-confirmed scope"

// memoryNilPort is the typed refusal for a nil store (fail closed).
func memoryNilPort(corr string) error {
	return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "memory store is not wired"})
}

// validateMemoryIdentity validates one bounded identifier member.
func validateMemoryIdentity(value, field string, corr string) error {
	if value == "" || len(value) > maxMemoryIDLen {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": field})
	}
	return nil
}

// validateMemoryOriginatingRefs bounds the caller-supplied references.
func validateMemoryOriginatingRefs(refs []string, corr string) error {
	if len(refs) > MaxMemoryOriginatingRefs {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "originating_refs"})
	}
	for _, r := range refs {
		if len(r) > maxMemoryLabelLen {
			return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "originating_refs"})
		}
	}
	return nil
}

// refuseMemorySecrets is the BINDING secrets law's refusal: the matched
// material is NEVER echoed into the envelope (the law extends to error
// messages).
func refuseMemorySecrets(field, corr string) error {
	return envelopeErr(errs.CodeMemorySecretsRejected, corr, map[string]any{
		"field":  field,
		"reason": "the submitted content attempts secret storage; secrets are NEVER stored in Memory — secret-manager/key-store references only",
	})
}

// MemoryItems reads the scope- and review-filtered memory list (the keyset
// list semantics; the cursor's absence marks the final page). The scope
// filter is part of the read authorization: scope never silently broadens,
// and a scope value renders only as the Core reports it — an unknown scope
// value is refused, never mapped to a known one.
func (s *Slice) MemoryItems(ctx context.Context, req MemoryListRequest) (MemoryItemListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return MemoryItemListResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory}, security.ActionDataRead); err != nil {
		return MemoryItemListResult{}, err
	}
	if req.Scope != "" && !memory.ValidScope(req.Scope) {
		return MemoryItemListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "scope"})
	}
	if req.State != "" && !memory.ValidLifecycleState(req.State) {
		return MemoryItemListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "state"})
	}
	limit, err := memoryListLimit(req.Limit, corr)
	if err != nil {
		return MemoryItemListResult{}, err
	}
	q := memory.ListQuery{Limit: limit, Scope: req.Scope, State: req.State}
	if req.Cursor != "" {
		pos, derr := decodeSaltedCursorEnvelope(req.Cursor)
		if derr != nil {
			return MemoryItemListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		var p knowledgeCursorPayload
		if uerr := json.Unmarshal(pos, &p); uerr != nil || p.CreatedAt == "" || p.ID == "" {
			return MemoryItemListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterID = p.CreatedAt, p.ID
	}
	page, err := s.deps.Memory.ListMemoryItems(ctx, q)
	if err != nil {
		return MemoryItemListResult{}, err
	}
	res := MemoryItemListResult{Items: page.Items}
	if page.More {
		next, cerr := encodeKnowledgeCursor(page.Items[len(page.Items)-1].CreatedAt, page.Items[len(page.Items)-1].MemoryItemID)
		if cerr != nil {
			return MemoryItemListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.Cursor = next
	}
	return res, nil
}

func memoryListLimit(limit int, corr string) (int, error) {
	if limit == 0 {
		limit = DefaultMemoryListLimit
	}
	if limit < 1 || limit > MaxMemoryListLimit {
		return 0, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	return limit, nil
}

// AddMemoryItem adds one memory item (the M-17 creation class). The Core
// mints the memory_item_id — the body never carries one (the Core-mints
// law). The entry state is this Core's policy decision under the
// unless-an-accepted-policy-explicitly-permits-another-route clause (the
// recorded OPEN gap): a user-entered add lands `active` — the response's
// lifecycle_state carries the Core-confirmed truth, never an assumption.
// THE SECRETS LAW (BINDING): a body attempting secret storage is refused
// (MEMORY_SECRETS_REJECTED). Origin labeling is honest: a client add is
// `user-entered` — the Core's own paths set the other origin values, and
// no such path exists on this surface (recorded honestly); a client body
// claiming another origin is the typed refusal, never a mislabel.
func (s *Slice) AddMemoryItem(ctx context.Context, req MemoryAddRequest) (memory.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.MutationResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory}, security.ActionDataWrite); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if !memory.ValidScope(req.Scope) {
		return memory.MutationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "scope"})
	}
	if !memory.ValidOrigin(req.Origin) {
		return memory.MutationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "origin"})
	}
	if !memory.ClientAddableOrigin(req.Origin) {
		return memory.MutationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "origin", "reason": "a client add is user-entered; the Core's own paths set the other origin values",
		})
	}
	if req.Content == "" || len(req.Content) > MaxMemoryContentBytes {
		return memory.MutationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content"})
	}
	if err := validateMemoryOriginatingRefs(req.OriginatingRefs, corr); err != nil {
		return memory.MutationResult{}, err
	}
	// THE SECRETS LAW (BINDING): the refusal precedes any accepted
	// operation — nothing is stored, and the matched material is never
	// echoed.
	if memory.ContainsSecretMaterial(req.Content) {
		return memory.MutationResult{}, refuseMemorySecrets("content", corr)
	}
	for _, r := range req.OriginatingRefs {
		if memory.ContainsSecretMaterial(r) {
			return memory.MutationResult{}, refuseMemorySecrets("originating_refs", corr)
		}
	}
	itemID := "mem-" + randomID()
	intent := memory.AddIntent{
		Item: memory.Item{
			MemoryItemID:    itemID,
			Scope:           req.Scope,
			Origin:          req.Origin,
			LifecycleState:  memory.StateActive,
			Content:         req.Content,
			OriginatingRefs: req.OriginatingRefs,
		},
		Operation: memory.Operation{
			OperationID:   "memop-" + randomID(),
			Kind:          memory.KindAdd,
			State:         memory.OpStateRecorded,
			CorrelationID: req.CorrelationID,
			MemoryItemID:  itemID,
			OutcomeState:  memory.StateActive,
		},
		IdempotencyKey: memory.ScopeKey(req.IdempotencyKey, req.Scope),
		Fingerprint:    req.CorrelationID,
	}
	outcome, err := s.deps.Memory.SubmitMemoryAdd(ctx, intent)
	if err != nil {
		return memory.MutationResult{}, err
	}
	return memory.MutationResult{MemoryItem: outcome.Item, OperationID: outcome.OperationID}, nil
}

// MemoryItem reads one item's authoritative state (the inspect member — a
// pure query, never a mutation; the item-state half of the unknown-outcome
// lookup). The unknown item and the removed item are the SAME typed
// MEMORY_ITEM_NOT_FOUND 404 (the no-leak rule).
func (s *Slice) MemoryItem(ctx context.Context, itemID string) (memory.Item, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.Item{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataRead); err != nil {
		return memory.Item{}, err
	}
	if err := validateMemoryIdentity(itemID, "memory_item_id", corr); err != nil {
		return memory.Item{}, err
	}
	return s.deps.Memory.LoadMemoryItem(ctx, itemID)
}

// memoryMutationIntent builds the M-16 intent for one member after the
// shared validation. The destination identity resolves INSIDE the store's
// transaction — the same typed 404 the inspect read serves, and the key
// stays unconsumed by an unresolvable intent (the tx refuses the unknown
// item before any key row exists); the effective key is the client key
// scoped by the item identity + the item's scope Core-side, built where
// the item's scope is read (a lost destructive response's keyed retry
// reaches the replay ruling even after the item is removed — the S0–S4
// reconcile law).
func (s *Slice) memoryMutationIntent(corr, itemID, clientKey, kind string) (memory.MutationIntent, error) {
	intent := memory.MutationIntent{
		Kind: kind,
		Operation: memory.Operation{
			OperationID:   "memop-" + randomID(),
			Kind:          kind,
			State:         memory.OpStateRecorded,
			CorrelationID: corr,
		},
		MemoryItemID: itemID,
		ClientKey:    clientKey,
		Fingerprint:  corr,
	}
	return intent, nil
}

// EditMemoryItem corrects one memory item (the M-16 revision class —
// provenance-preserving: the origin label and the originating references
// are never rewritten, and the edit alone never constitutes review
// approval). Eligibility is Core-owned: editing a non-eligible item is
// MEMORY_NOT_EDITABLE; a stale prepared-against reference is
// MEMORY_CONFLICT (reconcile by re-reading, never overwrite). THE SECRETS
// LAW (BINDING) applies to the corrected content.
func (s *Slice) EditMemoryItem(ctx context.Context, itemID string, req MemoryEditRequest) (memory.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.MutationResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataWrite); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.ItemStateRef, "item_state_ref", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if req.Content == "" || len(req.Content) > MaxMemoryContentBytes {
		return memory.MutationResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content"})
	}
	if memory.ContainsSecretMaterial(req.Content) {
		return memory.MutationResult{}, refuseMemorySecrets("content", corr)
	}
	intent, err := s.memoryMutationIntent(req.CorrelationID, itemID, req.IdempotencyKey, memory.KindEdit)
	if err != nil {
		return memory.MutationResult{}, err
	}
	intent.EditContent = req.Content
	intent.StateRef = req.ItemStateRef
	outcome, err := s.deps.Memory.ApplyMemoryMutation(ctx, intent)
	if err != nil {
		return memory.MutationResult{}, err
	}
	return memory.MutationResult{MemoryItem: outcome.Item, OperationID: outcome.OperationID}, nil
}

// ApproveMemoryItem confirms one item's review (the explicit user action;
// AI-extracted content NEVER becomes user-confirmed without it). The
// request is never displayed as the confirmation (the J-5 rule) — the
// Core-confirmed item state is the truth. A non-pending item is the typed
// MEMORY_REVIEW_STATE_CONFLICT.
func (s *Slice) ApproveMemoryItem(ctx context.Context, itemID string, req MemoryReviewRequest) (memory.MutationResult, error) {
	return s.memoryReview(ctx, itemID, req, memory.KindApprove)
}

// RejectMemoryItem rejects one item's review (the explicit user action;
// the rejected item's disposition follows the Core's semantics — never a
// silent client-side deletion).
func (s *Slice) RejectMemoryItem(ctx context.Context, itemID string, req MemoryReviewRequest) (memory.MutationResult, error) {
	return s.memoryReview(ctx, itemID, req, memory.KindReject)
}

func (s *Slice) memoryReview(ctx context.Context, itemID string, req MemoryReviewRequest, kind string) (memory.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.MutationResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataWrite); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.MutationResult{}, err
	}
	intent, err := s.memoryMutationIntent(req.CorrelationID, itemID, req.IdempotencyKey, kind)
	if err != nil {
		return memory.MutationResult{}, err
	}
	outcome, err := s.deps.Memory.ApplyMemoryMutation(ctx, intent)
	if err != nil {
		return memory.MutationResult{}, err
	}
	return memory.MutationResult{MemoryItem: outcome.Item, OperationID: outcome.OperationID}, nil
}

// SetMemoryItemPin sets one item's pin state (the pin/unpin member:
// pinned=true pins, pinned=false unpins — one member operation covering
// both directions). The pin register position is this Core's served
// definition of the recorded gap: a FLAG on the item, never a lifecycle
// state (the closed lifecycle register carries no pinned value); the
// Core-confirmed pin state rides the response's item.
func (s *Slice) SetMemoryItemPin(ctx context.Context, itemID string, req MemoryPinRequest) (memory.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.MutationResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataWrite); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return memory.MutationResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.MutationResult{}, err
	}
	kind := memory.KindPin
	if !req.Pinned {
		kind = memory.KindUnpin
	}
	intent, err := s.memoryMutationIntent(req.CorrelationID, itemID, req.IdempotencyKey, kind)
	if err != nil {
		return memory.MutationResult{}, err
	}
	intent.Pinned = req.Pinned
	outcome, err := s.deps.Memory.ApplyMemoryMutation(ctx, intent)
	if err != nil {
		return memory.MutationResult{}, err
	}
	return memory.MutationResult{MemoryItem: outcome.Item, OperationID: outcome.OperationID}, nil
}

// ForgetMemoryItem requests one item's forget (THE FORGET/DELETE
// DISTINCTNESS recorded at the 2026-11-09 resolution act, binding on
// this family: forget and delete are DISTINCT Core semantics at the
// register level — the forgotten lifecycle state and the
// memory_item.forgotten event vs the deleted state and the
// memory_item.deleted event — both riding the SAME M-16 removal
// discipline; the retention caveat identical, no immediate-total-erasure
// promise. The M-16 destructive discipline governs regardless: only a
// confirmed 200 carries the outcome truth, and a lost response is never
// assumed to have forgotten — reconcile by the operation ID or the item's
// authoritative state read BEFORE any retry). The removal drops the
// content and tombstones the identity row at the confirmed register state.
func (s *Slice) ForgetMemoryItem(ctx context.Context, itemID string, cmd MemoryReviewCommand) (memory.MutationResult, error) {
	outcome, err := s.memoryDestructive(ctx, itemID, cmd, memory.KindForget)
	if err != nil {
		return memory.MutationResult{}, err
	}
	return memory.MutationResult{MemoryItem: outcome.Item, OperationID: outcome.OperationID}, nil
}

// DeleteMemoryItem deletes one item (the M-16 destructive class;
// Core-confirmed only; the retention caveat rides the Core-confirmed
// outcome, never a client promise; Memory deletion never auto-deletes
// KnowledgePack claims — the promotion law's boundary).
func (s *Slice) DeleteMemoryItem(ctx context.Context, itemID string, cmd MemoryReviewCommand) (memory.DeleteResult, error) {
	outcome, err := s.memoryDestructive(ctx, itemID, cmd, memory.KindDelete)
	if err != nil {
		return memory.DeleteResult{}, err
	}
	op, err := s.deps.Memory.LoadMemoryOperation(ctx, outcome.OperationID)
	if err != nil {
		return memory.DeleteResult{}, err
	}
	return memory.DeleteResult{
		MemoryItemID:  itemID,
		OutcomeState:  op.OutcomeState,
		RetentionNote: MemoryDeleteRetentionNote,
		OperationID:   outcome.OperationID,
	}, nil
}

func (s *Slice) memoryDestructive(ctx context.Context, itemID string, cmd MemoryReviewCommand, kind string) (memory.MutationOutcome, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.MutationOutcome{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataWrite); err != nil {
		return memory.MutationOutcome{}, err
	}
	clientCorr := cmd.CorrelationID
	if len(clientCorr) > maxMemoryIDLen {
		return memory.MutationOutcome{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "correlation_id"})
	}
	if len(cmd.IdempotencyKey) > maxMemoryIDLen {
		return memory.MutationOutcome{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "idempotency_key"})
	}
	if clientCorr == "" {
		// No client carrier (the recorded gap): the Core mints the
		// correlation for the unkeyed destructive record.
		clientCorr = errs.NewCorrelationID()
	}
	intent, err := s.memoryMutationIntent(clientCorr, itemID, cmd.IdempotencyKey, kind)
	if err != nil {
		return memory.MutationOutcome{}, err
	}
	return s.deps.Memory.ApplyMemoryMutation(ctx, intent)
}

// SetMemoryItemInclusion sets one item's per-request inclusion (the M-18
// class: a per-request decision, NOT an item-state change, with no global
// effect; the decision applies ONLY to the stated request/scope carried by
// request_ref — it never widens the item's scope and never changes another
// scope).
func (s *Slice) SetMemoryItemInclusion(ctx context.Context, itemID string, req MemoryInclusionRequest) (memory.InclusionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.InclusionResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: itemID}, security.ActionDataWrite); err != nil {
		return memory.InclusionResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.InclusionResult{}, err
	}
	if err := validateMemoryIdentity(req.RequestRef, "request_ref", corr); err != nil {
		return memory.InclusionResult{}, err
	}
	intent := memory.InclusionIntent{
		Operation: memory.Operation{
			OperationID:   "memop-" + randomID(),
			Kind:          memory.KindInclusion,
			State:         memory.OpStateRecorded,
			CorrelationID: req.CorrelationID,
		},
		MemoryItemID: itemID,
		RequestRef:   req.RequestRef,
		Included:     req.Included,
		// The M-18 key carrier is the decision row's own (request, item)
		// scope — the operation row carries no durable idempotency key.
		Fingerprint: req.CorrelationID + memory.CorrelationFingerprint + itemID,
	}
	outcome, err := s.deps.Memory.SetMemoryInclusion(ctx, intent)
	if err != nil {
		return memory.InclusionResult{}, err
	}
	return outcome.Result, nil
}

// ExportMemoryItems exports the stated scope's items (the M-32
// read-with-output class: mutates nothing, no item-state change, safe to
// re-issue — no idempotency key, NO replay window). The exact export scope
// is stated, never assumed; a range exceeding the finite bound truncates
// honestly (truncated=true) — never silently. SECRETS ARE EXCLUDED BY
// DEFAULT — a prohibition, not a preference: no secret value ever enters
// an export artifact (the stored items structurally carry none — the
// binding law refused them at write). The export document rides the
// response; the export_ref is the produced document's content digest (a
// content identity — no artifact retrieval surface exists on this family,
// recorded honestly).
func (s *Slice) ExportMemoryItems(ctx context.Context, req MemoryExportRequest) (memory.ExportResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.ExportResult{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory}, security.ActionDataWrite); err != nil {
		return memory.ExportResult{}, err
	}
	if err := validateMemoryIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return memory.ExportResult{}, err
	}
	if !memory.ValidScope(req.Scope) {
		return memory.ExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "scope"})
	}
	if len(req.Destination) > maxMemoryLabelLen {
		return memory.ExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "destination"})
	}
	var items []memory.Item
	if len(req.ItemIDs) > 0 {
		if len(req.ItemIDs) > MaxMemoryExportItems {
			return memory.ExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "item_ids"})
		}
		named, err := s.deps.Memory.MemoryItemsByID(ctx, req.ItemIDs)
		if err != nil {
			return memory.ExportResult{}, err
		}
		// The scope-never-broadens law as the export's read authorization:
		// a named item outside the stated export scope is the typed
		// MEMORY_SCOPE_VIOLATION (the read crossing the forbidden
		// cross-scope default), never a silent narrowing.
		for _, item := range named {
			if item.Scope != req.Scope {
				return memory.ExportResult{}, envelopeErr(errs.CodeMemoryScopeViolation, corr, map[string]any{
					"reason": "the named item set crosses the stated export scope; cross-scope visibility is forbidden by default",
				})
			}
		}
		items = named
	} else {
		page, err := s.deps.Memory.ListMemoryItems(ctx, memory.ListQuery{Limit: MaxMemoryExportItems + 1, Scope: req.Scope})
		if err != nil {
			return memory.ExportResult{}, err
		}
		items = page.Items
	}
	truncated := false
	if len(items) > MaxMemoryExportItems {
		items = items[:MaxMemoryExportItems]
		truncated = true
	}
	doc, err := memoryExportDocument(items)
	if err != nil {
		return memory.ExportResult{}, envelopeErr(errs.CodeInternal, corr, nil)
	}
	op := memory.Operation{
		OperationID:   "memop-" + randomID(),
		Kind:          memory.KindExport,
		State:         memory.OpStateRecorded,
		CorrelationID: req.CorrelationID,
	}
	recorded, err := s.deps.Memory.SubmitMemoryExport(ctx, memory.ExportIntent{Operation: op, ExportRef: KnowledgeDigestOf(doc)})
	if err != nil {
		return memory.ExportResult{}, err
	}
	return memory.ExportResult{
		ExportOperationID: recorded.OperationID,
		Count:             len(items),
		GeneratedAt:       recorded.UpdatedAt,
		Truncated:         truncated,
		Items:             items,
	}, nil
}

// memoryExportDocument builds the export document (the canonical JSON of
// the served items — the exact scope the request named). The document
// structurally carries no secret value: the binding law refused secret
// material at every write.
func memoryExportDocument(items []memory.Item) ([]byte, error) {
	doc := struct {
		Items []memory.Item `json:"items"`
	}{Items: items}
	return events.CanonicalJSON(doc)
}

// MemoryOperationStatus is the family's authoritative state endpoint (a
// pure query — never a retry): resolved by the operation identity; a lost
// response is NEVER proof of success, cancellation, or safe retry —
// reconcile by the operation ID through this read (or by the item identity
// through the inspect read) BEFORE any retry.
func (s *Slice) MemoryOperationStatus(ctx context.Context, opID string) (memory.Operation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Memory == nil {
		return memory.Operation{}, memoryNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceMemory, ID: opID}, security.ActionDataRead); err != nil {
		return memory.Operation{}, err
	}
	if err := validateMemoryIdentity(opID, "operation_id", corr); err != nil {
		return memory.Operation{}, err
	}
	return s.deps.Memory.LoadMemoryOperation(ctx, opID)
}
