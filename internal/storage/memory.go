package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
	"github.com/qira-dev/karyora/internal/events"
)

// The Store implements the B2 Memory family's durable surface (DE-0 queue
// row 3): the M-17 creation ruling and the M-16 state-checked ruling run
// transactionally under the single-writer gate with the per-kind
// preconditions INSIDE the transaction (the review-state, eligibility, and
// removed-item laws are Core-enforced, never client-trusted); every
// authoritative state change commits with its journal/outbox rows in ONE
// transaction (CommitStateChange); the durable wall-clock values are
// stamped here. The F10-shaped event vocabulary rides the closed register
// verbatim and events are never the source of truth — the authoritative
// reads are.

// MemoryReplayWindow is the M-17 add class's post-terminal replay window:
// the 72 h post-terminal window (the F6-precedent window the published
// description cites — the shared recorded IdempotencyReplayWindow
// constant).
const MemoryReplayWindow = IdempotencyReplayWindow

// MemoryMutationReplayWindow is the M-16 state-checked class's post-terminal
// replay window: 1 h (the published short window selected at the CF-2
// publication act — the same window for edit/approve/reject/pin/unpin and
// the destructive forget/delete members).
const MemoryMutationReplayWindow = time.Hour

// MemoryIdempotencyConflictError carries the M-17 conflict's envelope: the
// published CONFLICT_IDEMPOTENCY_KEY with its fingerprintMatch:false datum
// (the same typed shape the B1 knowledge import conflict rides).
type MemoryIdempotencyConflictError struct{ Detail string }

func (e *MemoryIdempotencyConflictError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", errs.CodeConflictIdempotencyKey, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *MemoryIdempotencyConflictError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeConflictIdempotencyKey, correlationID, map[string]any{
		"keyScope":         "memory.add",
		"fingerprintMatch": false,
		"reason":           e.Detail,
	})
}

// errMemoryDuplicateKey marks the in-transaction same-key insert race that
// resolves to the replay ruling (the SubmitJob duplicate idiom).
var errMemoryDuplicateKey = errors.New("storage: duplicate memory idempotency key")

// memoryFingerprintSeparator joins the correlation half and the item's
// state-reference half of the M-16 fingerprint (the builders live in the
// domain package — the shared leaf).
const memoryFingerprintSeparator = "\x1f"

// SubmitMemoryAdd applies the M-17 ruling and stores the fresh item with
// its operation record and the memory_item.added event when no replay
// resolves. The entry state rides the intent's item (the Core's entry-state
// policy — the response's lifecycle_state carries the truth).
func (s *Store) SubmitMemoryAdd(ctx context.Context, intent memory.AddIntent) (memory.MutationOutcome, error) {
	item := intent.Item
	op := intent.Operation
	if item.MemoryItemID == "" || !memory.ValidScope(item.Scope) || !memory.ValidOrigin(item.Origin) ||
		!memory.ValidLifecycleState(item.LifecycleState) || op.OperationID == "" ||
		op.Kind != memory.KindAdd || intent.IdempotencyKey == "" || intent.Fingerprint == "" {
		return memory.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "memory add intent lacks its identity members")
	}
	// The M-17 ruling: one operation per effective key.
	if prior, err := s.memoryOpByKey(ctx, intent.IdempotencyKey); err != nil {
		return memory.MutationOutcome{}, err
	} else if prior != nil {
		return s.resolveMemoryAddReplay(ctx, prior, intent)
	}
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		if err := insertMemoryItem(tx, item, s.now()); err != nil {
			return err
		}
		if err := insertMemoryOperation(tx, op, intent.IdempotencyKey, intent.Fingerprint, s.now()); err != nil {
			return err
		}
		return appendEventTx(tx, memoryEvent(
			"evt-"+op.OperationID,
			memory.AggregateMemoryItem, item.MemoryItemID,
			memory.EventItemAdded, op.CorrelationID, s.instanceUUID,
			map[string]string{"scope": item.Scope, "origin": item.Origin, "lifecycle_state": item.LifecycleState}))
	})
	if err == errMemoryDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		prior, lookupErr := s.memoryOpByKey(ctx, intent.IdempotencyKey)
		if lookupErr != nil || prior == nil {
			return memory.MutationOutcome{}, err
		}
		return s.resolveMemoryAddReplay(ctx, prior, intent)
	}
	if err != nil {
		return memory.MutationOutcome{}, err
	}
	fresh, err := s.LoadMemoryItem(ctx, item.MemoryItemID)
	if err != nil {
		return memory.MutationOutcome{}, err
	}
	return memory.MutationOutcome{Item: fresh, OperationID: op.OperationID, Replayed: false}, nil
}

// resolveMemoryAddReplay applies the M-17 class exactly: same key + same
// request identity (the correlation ID) replays the recorded item while
// the operation is live or within the 72 h post-terminal window; the same
// key with a different request identity is the typed
// CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false); a replay past the
// window is the typed CONFLICT_STATE.
func (s *Store) resolveMemoryAddReplay(ctx context.Context, prior *memory.Operation, intent memory.AddIntent) (memory.MutationOutcome, error) {
	if prior.CorrelationID != intent.Fingerprint {
		return memory.MutationOutcome{}, &MemoryIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity; the M-17 fingerprint does not match (recorded correlation %q)", prior.CorrelationID),
		}
	}
	if prior.State != memory.OpStateRecorded {
		return memory.MutationOutcome{}, errStoragef(errs.CodeConflictState, "memory add operation %q is not recorded", prior.OperationID)
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return memory.MutationOutcome{}, errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > MemoryReplayWindow {
		return memory.MutationOutcome{}, errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", MemoryReplayWindow)
	}
	item, err := s.LoadMemoryItem(ctx, prior.MemoryItemID)
	if err != nil {
		// The recorded item is no longer served (removed after the add):
		// the same typed 404; the operation record still resolves through
		// the state endpoint (recorded honestly).
		return memory.MutationOutcome{}, err
	}
	return memory.MutationOutcome{Item: item, OperationID: prior.OperationID, Replayed: true}, nil
}

// ApplyMemoryMutation applies the M-16 state-checked ruling for one
// mutation member (edit | approve | reject | pin | unpin | forget |
// delete) with the per-kind preconditions INSIDE the transaction:
// unknown and removed items are the SAME typed
// MEMORY_ITEM_NOT_FOUND (the no-leak rule); non-eligible edits are
// MEMORY_NOT_EDITABLE (the eligibility law); a stale edit reference is
// MEMORY_CONFLICT (reconcile by re-reading, never overwrite); a review
// action against a non-pending item is MEMORY_REVIEW_STATE_CONFLICT; the
// same key + the same correlation + the same Core-resolved item state
// reference replays the recorded outcome within the 1 h post-terminal
// window; anything else under the same key is the typed CONFLICT_STATE.
// Only a confirmed outcome carries the truth (the F11 law).
func (s *Store) ApplyMemoryMutation(ctx context.Context, intent memory.MutationIntent) (memory.MutationOutcome, error) {
	op := intent.Operation
	if op.OperationID == "" || !memory.ValidOperationKind(op.Kind) || intent.MemoryItemID == "" ||
		intent.Fingerprint == "" || op.CorrelationID == "" {
		return memory.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "memory mutation intent lacks its identity members")
	}
	// The state-checked members require their published key; the
	// destructive members may ride the published OPTIONAL keying carrier
	// (the B2-CA amendment): an unkeyed call records a non-replayable
	// operation.
	keyed := intent.ClientKey != ""
	if !keyed && intent.Kind != memory.KindForget && intent.Kind != memory.KindDelete {
		return memory.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "memory mutation kind %q requires its M-16 idempotency key", intent.Kind)
	}
	if intent.Kind == memory.KindEdit && intent.EditContent == "" {
		return memory.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "memory edit intent lacks its corrected content")
	}
	var outcome memory.MutationOutcome
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		item, err := loadMemoryItemIncludeRemovedTx(tx, intent.MemoryItemID)
		if err != nil {
			return err
		}
		currentRef := memory.StateRef(item.LifecycleState, item.UpdatedAt)
		// The effective key is built HERE (the item's scope rides the
		// Core-side key scope law; the tombstone retains the scope).
		effectiveKey := ""
		if keyed {
			effectiveKey = memory.ItemKey(intent.ClientKey, intent.MemoryItemID, item.Scope)
		}
		// The M-16 state-checked ruling: resolve the prior operation under
		// the effective key first (skipped for the unkeyed destructive
		// records — no key row exists to match; the empty body is the
		// unkeyed record).
		// The replay precedes the removed-item refusal so a lost
		// destructive response's retry reconciles (the S0–S4 law) instead
		// of surfacing the removed-item 404.
		if keyed {
			prior, priorFingerprint, err := memoryOpByKeyTx(tx, effectiveKey)
			if err != nil {
				return err
			}
			if prior != nil {
				return s.resolveMemoryMutationReplay(tx, prior, priorFingerprint, intent, &outcome)
			}
		}
		if memory.Removed(item.LifecycleState) {
			// A removed item is not served: the SAME typed 404 for any
			// further mutation (the no-leak rule; the identity tombstone
			// stays for the reconcile law through the operation records).
			return errStoragef(errs.CodeMemoryItemNotFound, "no such memory item")
		}
		// The per-kind preconditions (Core-enforced inside the transaction).
		effectState, confirmed := "", ""
		eventType := ""
		switch intent.Kind {
		case memory.KindEdit:
			if !memory.Editable(item.LifecycleState) {
				return errStoragef(errs.CodeMemoryNotEditable, "memory item %q is not editable in state %q (the Core owns eligibility)", intent.MemoryItemID, item.LifecycleState)
			}
			if intent.StateRef != currentRef {
				return errStoragef(errs.CodeMemoryConflict, "memory item %q moved since the edit was prepared (state reference mismatch); reconcile by re-reading the item's authoritative state, never overwrite", intent.MemoryItemID)
			}
			effectState = memory.StateUpdated
			eventType = memory.EventItemUpdated
		case memory.KindApprove, memory.KindReject:
			if !memory.Reviewable(item.LifecycleState) {
				return errStoragef(errs.CodeMemoryReviewStateConflict, "memory item %q is not in the required review state (state %q)", intent.MemoryItemID, item.LifecycleState)
			}
			if intent.Kind == memory.KindApprove {
				effectState = memory.StateApproved
				confirmed = s.now().UTC().Format(time.RFC3339Nano)
				eventType = memory.EventReviewApproved
			} else {
				effectState = memory.StateRejected
				eventType = memory.EventReviewRejected
			}
		case memory.KindPin, memory.KindUnpin:
			if !memory.PinTargetable(item.LifecycleState) {
				return errStoragef(errs.CodeValidationFailed, "memory item %q does not accept a pin change in state %q", intent.MemoryItemID, item.LifecycleState)
			}
			eventType = memory.EventItemPinned
			if intent.Kind == memory.KindUnpin {
				eventType = memory.EventItemUnpinned
			}
		case memory.KindForget, memory.KindDelete:
			effectState = memory.StateForgotten
			eventType = memory.EventItemForgotten
			if intent.Kind == memory.KindDelete {
				effectState = memory.StateDeleted
				eventType = memory.EventItemDeleted
			}
		default:
			return errStoragef(errs.CodeValidationFailed, "memory mutation kind %q is not a state-checked member", intent.Kind)
		}
		// The effect: the item's post-state, the operation record, and the
		// register event — ONE transaction (the F11 law: only a confirmed
		// outcome carries the truth). The fingerprint's state half is the
		// item's PRE-execution state reference (what the client prepared
		// against — the M-16 fingerprint's state half, Core-resolved).
		now := s.now().UTC().Format(time.RFC3339Nano)
		fingerprint := intent.Fingerprint + memoryFingerprintSeparator + currentRef
		if intent.Kind == memory.KindEdit {
			if _, err := tx.Exec(`UPDATE memory_items SET lifecycle_state = ?, content = ?, updated_at = ?
				WHERE memory_item_id = ?`, effectState, intent.EditContent, now, intent.MemoryItemID); err != nil {
				return mapVendorError(err)
			}
		} else if intent.Kind == memory.KindPin || intent.Kind == memory.KindUnpin {
			pinned := 0
			if intent.Kind == memory.KindPin {
				pinned = 1
			}
			if _, err := tx.Exec(`UPDATE memory_items SET pinned = ?, updated_at = ?
				WHERE memory_item_id = ?`, pinned, now, intent.MemoryItemID); err != nil {
				return mapVendorError(err)
			}
		} else if intent.Kind == memory.KindApprove {
			if _, err := tx.Exec(`UPDATE memory_items SET lifecycle_state = ?, last_confirmed_at = ?, updated_at = ?
				WHERE memory_item_id = ?`, effectState, confirmed, now, intent.MemoryItemID); err != nil {
				return mapVendorError(err)
			}
		} else if intent.Kind == memory.KindForget || intent.Kind == memory.KindDelete {
			// The removal discipline: the content is dropped; the identity
			// tombstone retains the identity/scope/state members for the
			// reconcile law (the authoritative operation record carries the
			// confirmed outcome). No immediate-total-erasure is promised —
			// the delete member's result carries the Core-confirmed
			// retention caveat.
			if _, err := tx.Exec(`UPDATE memory_items SET lifecycle_state = ?, content = '', pinned = 0, updated_at = ?
				WHERE memory_item_id = ?`, effectState, now, intent.MemoryItemID); err != nil {
				return mapVendorError(err)
			}
		}
		recorded := op
		recorded.State = memory.OpStateRecorded
		recorded.MemoryItemID = intent.MemoryItemID
		recorded.OutcomeState = effectState
		if err := insertMemoryOperation(tx, recorded, effectiveKey, fingerprint, s.now()); err != nil {
			return err
		}
		if err := appendEventTx(tx, memoryEvent(
			"evt-"+op.OperationID,
			memory.AggregateMemoryItem, intent.MemoryItemID,
			eventType, op.CorrelationID, s.instanceUUID,
			map[string]string{"kind": intent.Kind, "lifecycle_state": orDefault(effectState, item.LifecycleState)})); err != nil {
			return err
		}
		outcome = memory.MutationOutcome{OperationID: op.OperationID, Replayed: false}
		return nil
	})
	if err == errMemoryDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom). The
		// effective key rebuilds from the item's scope (the tombstone
		// retains it).
		item, loadErr := s.loadMemoryItemIncludeRemoved(ctx, intent.MemoryItemID)
		if loadErr != nil {
			return memory.MutationOutcome{}, err
		}
		prior, priorFingerprint, lookupErr := s.memoryOpByKeyWithFingerprint(ctx, memory.ItemKey(intent.ClientKey, intent.MemoryItemID, item.Scope))
		if lookupErr != nil || prior == nil {
			return memory.MutationOutcome{}, err
		}
		var replay memory.MutationOutcome
		rerr := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
			return s.resolveMemoryMutationReplay(tx, prior, priorFingerprint, intent, &replay)
		})
		if rerr != nil {
			return memory.MutationOutcome{}, rerr
		}
		return replay, nil
	}
	if err != nil {
		return memory.MutationOutcome{}, err
	}
	// The outcome's item is the Core-confirmed post-state (the tombstone
	// for the removal members — loaded WITHOUT the removed filter; the
	// public inspect read stays the readable-item read).
	fresh, lerr := s.loadMemoryItemIncludeRemoved(ctx, intent.MemoryItemID)
	if lerr != nil {
		return memory.MutationOutcome{}, lerr
	}
	outcome.Item = fresh
	return outcome, nil
}

// resolveMemoryMutationReplay applies the M-16 replay law: the same key +
// the same correlation (+, for the edit member, the same prepared-against
// state reference as the recorded one) replays the recorded outcome within
// the 1 h post-terminal window; the same key against a different
// correlation — or, for the edit member, a different prepared-against
// state — is the typed CONFLICT_STATE (the key's recorded outcome no
// longer matches the attempt — recorded as this implementation's uniform
// M-16 conflict ruling; the reconcile law rides the correlation for the
// members whose body carries no state reference).
func (s *Store) resolveMemoryMutationReplay(tx *sqlTx, prior *memory.Operation, priorFingerprint string, intent memory.MutationIntent, outcome *memory.MutationOutcome) error {
	if prior.CorrelationID != intent.Fingerprint {
		return errStoragef(errs.CodeConflictState,
			"memory idempotency key was already recorded under a different correlation (recorded outcome %q); replay rejected", prior.OutcomeState)
	}
	if intent.Kind == memory.KindEdit {
		recordedRef := priorFingerprint
		if i := strings.LastIndex(recordedRef, memoryFingerprintSeparator); i >= 0 {
			recordedRef = recordedRef[i+len(memoryFingerprintSeparator):]
		}
		if intent.StateRef != recordedRef {
			return errStoragef(errs.CodeConflictState,
				"memory edit was recorded against a different item state reference (%q); replay rejected", recordedRef)
		}
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > MemoryMutationReplayWindow {
		return errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", MemoryMutationReplayWindow)
	}
	if prior.State != memory.OpStateRecorded {
		return errStoragef(errs.CodeConflictState, "memory operation %q is not recorded", prior.OperationID)
	}
	item, err := loadMemoryItemIncludeRemovedTx(tx, intent.MemoryItemID)
	if err != nil {
		return err
	}
	*outcome = memory.MutationOutcome{Item: item, OperationID: prior.OperationID, Replayed: true}
	return nil
}

// SetMemoryInclusion applies the M-18 request-scoped decision: the
// decision applies ONLY to the stated request/scope, changes no item
// state, and has no global effect. The same effective key + the same
// fingerprint (the request's correlation ID + the item identity) replays
// the recorded decision; a different correlation under the same request is
// the caller's NEW decision record (supersedes within the request — the
// request-scoped semantics; the retention is the decision row's own
// lifetime, recorded honestly as the request-lifetime-at-most typing).
func (s *Store) SetMemoryInclusion(ctx context.Context, intent memory.InclusionIntent) (memory.InclusionOutcome, error) {
	op := intent.Operation
	if op.OperationID == "" || op.Kind != memory.KindInclusion || intent.MemoryItemID == "" ||
		intent.RequestRef == "" || intent.Fingerprint == "" || op.CorrelationID == "" {
		return memory.InclusionOutcome{}, errStoragef(errs.CodeValidationFailed, "memory inclusion intent lacks its identity members")
	}
	var outcome memory.InclusionOutcome
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		if _, err := loadMemoryItemTx(tx, intent.MemoryItemID); err != nil {
			return err
		}
		// The prior decision under the request-scoped key: the decision row
		// IS the M-18 key carrier (the request-scoped record — the
		// operation row carries no durable idempotency key, so a superseded
		// decision never collides on the key index).
		var corr, opID string
		var included int64
		row := tx.QueryRow(`SELECT correlation_id, operation_id, included FROM memory_inclusions
			WHERE request_ref = ? AND memory_item_id = ?`, intent.RequestRef, intent.MemoryItemID)
		err := row.Scan(&corr, &opID, &included)
		if err == nil {
			if corr == intent.Fingerprint {
				// The same request identity: replay the recorded decision.
				outcome = memory.InclusionOutcome{
					Result: memory.InclusionResult{
						MemoryItemID: intent.MemoryItemID,
						RequestRef:   intent.RequestRef,
						Included:     included == 1,
						OperationID:  opID,
					},
					Replayed: true,
				}
				return nil
			}
			// A different correlation under the same request: the caller's
			// new decision record — supersede below.
		} else if err != sql.ErrNoRows {
			return mapVendorError(err)
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`INSERT INTO memory_inclusions
			(request_ref, memory_item_id, included, correlation_id, operation_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(request_ref, memory_item_id) DO UPDATE SET
				included = excluded.included, correlation_id = excluded.correlation_id,
				operation_id = excluded.operation_id, updated_at = excluded.updated_at`,
			intent.RequestRef, intent.MemoryItemID, boolToInt(intent.Included), intent.Fingerprint, op.OperationID, now); err != nil {
			return mapVendorError(err)
		}
		recorded := op
		recorded.State = memory.OpStateRecorded
		recorded.MemoryItemID = intent.MemoryItemID
		if err := insertMemoryOperation(tx, recorded, "", intent.Fingerprint, s.now()); err != nil {
			return err
		}
		outcome = memory.InclusionOutcome{
			Result: memory.InclusionResult{
				MemoryItemID: intent.MemoryItemID,
				RequestRef:   intent.RequestRef,
				Included:     intent.Included,
				OperationID:  op.OperationID,
			},
			Replayed: false,
		}
		return nil
	})
	if err != nil {
		return memory.InclusionOutcome{}, err
	}
	return outcome, nil
}

// SubmitMemoryExport records the export member's operation (the M-32
// read-with-output class): mutates nothing, no item-state change, NO
// idempotency key, NO replay window — every submit a new operation (safe
// re-issue). ExportRef carries the produced document's content digest.
func (s *Store) SubmitMemoryExport(ctx context.Context, intent memory.ExportIntent) (memory.Operation, error) {
	op := intent.Operation
	if op.OperationID == "" || op.Kind != memory.KindExport || intent.ExportRef == "" || op.CorrelationID == "" {
		return memory.Operation{}, errStoragef(errs.CodeValidationFailed, "memory export intent lacks its identity members")
	}
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		recorded := op
		recorded.State = memory.OpStateRecorded
		recorded.ExportRef = intent.ExportRef
		return insertMemoryOperation(tx, recorded, "", "", s.now())
	})
	if err != nil {
		return memory.Operation{}, err
	}
	return s.LoadMemoryOperation(ctx, op.OperationID)
}

// LoadMemoryItem resolves the authoritative item read; a removed item is
// not served — the SAME typed MEMORY_ITEM_NOT_FOUND for unknown and
// removed (the no-leak rule; the reconcile law rides the operation
// records).
func (s *Store) LoadMemoryItem(ctx context.Context, itemID string) (memory.Item, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT scope, origin, lifecycle_state, content, pinned, creation_method, confidence, originating_refs,
       retention_ttl_ref, conflict_note, untrusted_label, created_at, last_used_at, last_confirmed_at, updated_at, memory_item_id
FROM memory_items WHERE memory_item_id = ? AND lifecycle_state NOT IN ('forgotten', 'deleted')`, itemID)
	return scanMemoryItem(row, itemID)
}

// loadMemoryItemIncludeRemoved resolves the item row WITHOUT the removed
// filter — the mutation outcome's Core-confirmed post-state read (the
// tombstone for the removal members; the public inspect read stays the
// readable-item read).
func (s *Store) loadMemoryItemIncludeRemoved(ctx context.Context, itemID string) (memory.Item, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT scope, origin, lifecycle_state, content, pinned, creation_method, confidence, originating_refs,
       retention_ttl_ref, conflict_note, untrusted_label, created_at, last_used_at, last_confirmed_at, updated_at, memory_item_id
FROM memory_items WHERE memory_item_id = ?`, itemID)
	return scanMemoryItem(row, itemID)
}

// loadMemoryItemIncludeRemovedTx is the in-transaction variant.
func loadMemoryItemIncludeRemovedTx(tx *sqlTx, itemID string) (memory.Item, error) {
	row := tx.QueryRow(`
SELECT scope, origin, lifecycle_state, content, pinned, creation_method, confidence, originating_refs,
       retention_ttl_ref, conflict_note, untrusted_label, created_at, last_used_at, last_confirmed_at, updated_at, memory_item_id
FROM memory_items WHERE memory_item_id = ?`, itemID)
	return scanMemoryItem(row, itemID)
}

// ListMemoryItems reads one bounded keyset page (created_at DESC,
// memory_item_id DESC — the newest first) with the optional scope and
// state filters (the scope filter is part of the read authorization —
// scope never silently broadens). Removed items are not served.
func (s *Store) ListMemoryItems(ctx context.Context, q memory.ListQuery) (memory.ItemPage, error) {
	conds := []string{"lifecycle_state NOT IN ('forgotten', 'deleted')"}
	args := []any{}
	if q.Scope != "" {
		conds = append(conds, "scope = ?")
		args = append(args, q.Scope)
	}
	if q.State != "" {
		conds = append(conds, "lifecycle_state = ?")
		args = append(args, q.State)
	}
	if q.AfterCreatedAt != "" {
		conds = append(conds, "(created_at < ? OR (created_at = ? AND memory_item_id < ?))")
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.readers.QueryContext(ctx, `
SELECT scope, origin, lifecycle_state, content, pinned, creation_method, confidence, originating_refs,
       retention_ttl_ref, conflict_note, untrusted_label, created_at, last_used_at, last_confirmed_at, updated_at, memory_item_id
FROM memory_items WHERE `+strings.Join(conds, " AND ")+`
ORDER BY created_at DESC, memory_item_id DESC LIMIT ?`, args...)
	if err != nil {
		return memory.ItemPage{}, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := memory.ItemPage{Items: []memory.Item{}}
	for rows.Next() {
		item, err := scanMemoryItemRow(rows)
		if err != nil {
			return memory.ItemPage{}, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return memory.ItemPage{}, mapVendorError(err)
	}
	if len(out.Items) > q.Limit {
		out.More = true
		out.Items = out.Items[:q.Limit]
	}
	return out, nil
}

// MemoryItemsByID loads the named items in the caller's order (the export
// member's explicit-subset read; a named item that is unknown or removed
// is the SAME typed MEMORY_ITEM_NOT_FOUND — the no-leak rule).
func (s *Store) MemoryItemsByID(ctx context.Context, itemIDs []string) ([]memory.Item, error) {
	out := make([]memory.Item, 0, len(itemIDs))
	for _, id := range itemIDs {
		item, err := s.LoadMemoryItem(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// LoadMemoryOperation resolves the authoritative state read; an unknown
// operation is the typed NOT_FOUND (the no-leak rule; the envelope
// register code — no Memory operation-not-found domain code exists in the
// published register).
func (s *Store) LoadMemoryOperation(ctx context.Context, opID string) (memory.Operation, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT kind, state, correlation_id, memory_item_id, outcome_state, export_ref, failure_code, failure_reason, created_at, updated_at
FROM memory_operations WHERE operation_id = ?`, opID)
	var op memory.Operation
	var kind, state, corr, itemID, outcomeState, exportRef, failureCode, failureReason, createdAt, updatedAt string
	err := row.Scan(&kind, &state, &corr, &itemID, &outcomeState, &exportRef, &failureCode, &failureReason, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return memory.Operation{}, errStoragef(errs.CodeNotFound, "no such memory operation")
	}
	if err != nil {
		return memory.Operation{}, mapVendorError(err)
	}
	op.OperationID = opID
	op.Kind, op.State, op.CorrelationID = kind, state, corr
	op.MemoryItemID, op.OutcomeState, op.ExportRef = itemID, outcomeState, exportRef
	op.FailureCode, op.FailureReason = failureCode, failureReason
	op.CreatedAt, op.UpdatedAt = createdAt, updatedAt
	return op, nil
}

// memoryOpByKey resolves the prior operation under one effective key (nil
// op when none).
func (s *Store) memoryOpByKey(ctx context.Context, key string) (*memory.Operation, error) {
	op, _, err := s.memoryOpByKeyWithFingerprint(ctx, key)
	return op, err
}

// memoryOpByKeyWithFingerprint resolves the prior operation under one
// effective key with its recorded fingerprint (nil op when none).
func (s *Store) memoryOpByKeyWithFingerprint(ctx context.Context, key string) (*memory.Operation, string, error) {
	var opID, fingerprint string
	err := s.readers.QueryRowContext(ctx,
		`SELECT operation_id, fingerprint FROM memory_operations WHERE idempotency_key = ?`, key).Scan(&opID, &fingerprint)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	op, err := s.LoadMemoryOperation(ctx, opID)
	if err != nil {
		return nil, "", err
	}
	return &op, fingerprint, nil
}

// memoryOpByKeyTx is the in-transaction variant (the M-16 ruling runs
// inside the same transaction as the effect).
func memoryOpByKeyTx(tx *sqlTx, key string) (*memory.Operation, string, error) {
	var opID, fingerprint string
	err := tx.QueryRow(`SELECT operation_id, fingerprint FROM memory_operations WHERE idempotency_key = ?`, key).Scan(&opID, &fingerprint)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	var op memory.Operation
	var kind, state, corr, itemID, outcomeState, exportRef, failureCode, failureReason, createdAt, updatedAt string
	err = tx.QueryRow(`SELECT kind, state, correlation_id, memory_item_id, outcome_state, export_ref, failure_code, failure_reason, created_at, updated_at
FROM memory_operations WHERE operation_id = ?`, opID).Scan(&kind, &state, &corr, &itemID, &outcomeState, &exportRef, &failureCode, &failureReason, &createdAt, &updatedAt)
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	op.OperationID = opID
	op.Kind, op.State, op.CorrelationID = kind, state, corr
	op.MemoryItemID, op.OutcomeState, op.ExportRef = itemID, outcomeState, exportRef
	op.FailureCode, op.FailureReason = failureCode, failureReason
	op.CreatedAt, op.UpdatedAt = createdAt, updatedAt
	return &op, fingerprint, nil
}

// loadMemoryItemTx is the in-transaction item read (the preconditions run
// against the transactional read, never a stale reader row).
func loadMemoryItemTx(tx *sqlTx, itemID string) (memory.Item, error) {
	row := tx.QueryRow(`
SELECT scope, origin, lifecycle_state, content, pinned, creation_method, confidence, originating_refs,
       retention_ttl_ref, conflict_note, untrusted_label, created_at, last_used_at, last_confirmed_at, updated_at, memory_item_id
FROM memory_items WHERE memory_item_id = ?`, itemID)
	item, err := scanMemoryItem(row, itemID)
	if err == nil && memory.Removed(item.LifecycleState) {
		// The raw row exists but the item is removed: the same typed 404
		// (the no-leak rule).
		return memory.Item{}, errStoragef(errs.CodeMemoryItemNotFound, "no such memory item")
	}
	return item, err
}

type memoryRowScanner interface{ Scan(dest ...any) error }

func scanMemoryItem(row memoryRowScanner, itemID string) (memory.Item, error) {
	item, err := scanMemoryItemRow(row)
	if err == sql.ErrNoRows {
		return memory.Item{}, errStoragef(errs.CodeMemoryItemNotFound, "no such memory item")
	}
	if err != nil {
		return memory.Item{}, mapVendorError(err)
	}
	item.MemoryItemID = itemID
	return item, nil
}

func scanMemoryItemRow(row memoryRowScanner) (memory.Item, error) {
	var item memory.Item
	var scope, origin, state, content, creationMethod, confidence, originatingRefs,
		retentionTTLRef, conflictNote, untrustedLabel, createdAt, lastUsedAt, lastConfirmedAt, updatedAt, itemID string
	var pinned int64
	err := row.Scan(&scope, &origin, &state, &content, &pinned, &creationMethod, &confidence, &originatingRefs,
		&retentionTTLRef, &conflictNote, &untrustedLabel, &createdAt, &lastUsedAt, &lastConfirmedAt, &updatedAt, &itemID)
	if err != nil {
		return memory.Item{}, err
	}
	item.MemoryItemID = itemID
	item.Scope, item.Origin, item.LifecycleState, item.Content = scope, origin, state, content
	item.CreationMethod, item.Confidence = creationMethod, confidence
	if originatingRefs != "" && originatingRefs != "[]" {
		_ = json.Unmarshal([]byte(originatingRefs), &item.OriginatingRefs)
	}
	item.RetentionTTLRef, item.ConflictNote, item.UntrustedLabel = retentionTTLRef, conflictNote, untrustedLabel
	item.CreatedAt, item.LastUsedAt, item.LastConfirmedAt, item.UpdatedAt = createdAt, lastUsedAt, lastConfirmedAt, updatedAt
	if pinned == 1 {
		item.Pinned = true
	}
	return item, nil
}

// insertMemoryItem stamps and writes the fresh item row.
func insertMemoryItem(tx *sqlTx, item memory.Item, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	refs := "[]"
	if b, err := json.Marshal(item.OriginatingRefs); err == nil && item.OriginatingRefs != nil {
		refs = string(b)
	}
	pinned := 0
	if item.Pinned {
		pinned = 1
	}
	_, err := tx.Exec(`INSERT INTO memory_items
		(memory_item_id, scope, origin, lifecycle_state, content, pinned, creation_method, confidence,
		 originating_refs, retention_ttl_ref, conflict_note, untrusted_label,
		 created_at, last_used_at, last_confirmed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.MemoryItemID, item.Scope, item.Origin, item.LifecycleState, item.Content, pinned,
		item.CreationMethod, item.Confidence, refs, item.RetentionTTLRef, item.ConflictNote, item.UntrustedLabel,
		nowNano, item.LastUsedAt, item.LastConfirmedAt, nowNano)
	if err != nil {
		var serr sqlite3.Error
		if errors.As(err, &serr) && serr.Code == sqlite3.ErrConstraint {
			return errMemoryDuplicateKey
		}
		return mapVendorError(err)
	}
	return nil
}

// insertMemoryOperation stamps and writes the fresh operation row.
func insertMemoryOperation(tx *sqlTx, op memory.Operation, idempotencyKey, fingerprint string, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	terminal := ""
	if op.State == memory.OpStateRecorded {
		terminal = nowNano
	}
	_, err := tx.Exec(`INSERT INTO memory_operations
		(operation_id, kind, state, correlation_id, memory_item_id, outcome_state, export_ref,
		 failure_code, failure_reason, idempotency_key, fingerprint, terminal_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.OperationID, op.Kind, op.State, op.CorrelationID, op.MemoryItemID, op.OutcomeState, op.ExportRef,
		op.FailureCode, op.FailureReason, idempotencyKey, fingerprint, terminal, nowNano, nowNano)
	if err != nil {
		var serr sqlite3.Error
		if errors.As(err, &serr) && serr.Code == sqlite3.ErrConstraint {
			return errMemoryDuplicateKey
		}
		return mapVendorError(err)
	}
	return nil
}

// memoryEvent builds one F10-shaped journal record: the frozen field set
// with a bounded inline payload (the register's aggregate/type values ride
// verbatim).
func memoryEvent(eventID, aggregateType, aggregateID, eventType, correlationID, producer string, payload map[string]string) events.Record {
	inline, _ := json.Marshal(payload)
	if len(inline) > events.MaxInlinePayloadBytes {
		inline = nil
	}
	return events.Record{
		EventID:       eventID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		CorrelationID: correlationID,
		Producer:      producer,
		InlinePayload: inline,
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
