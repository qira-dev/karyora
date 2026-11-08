package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
	"github.com/qira-dev/karyora/internal/events"
)

// memoryAddIntent builds a legal fresh M-17 add intent (the effective key
// scoped by the stated scope; the fingerprint is the request identity).
func memoryAddIntent(itemID, opID, clientKey, correlation, scope string) memory.AddIntent {
	return memory.AddIntent{
		Item: memory.Item{
			MemoryItemID:   itemID,
			Scope:          scope,
			Origin:         memory.OriginUserEntered,
			LifecycleState: memory.StateActive,
			Content:        "fixture content for " + itemID,
		},
		Operation: memory.Operation{
			OperationID:   opID,
			Kind:          memory.KindAdd,
			State:         memory.OpStateRecorded,
			CorrelationID: correlation,
			MemoryItemID:  itemID,
			OutcomeState:  memory.StateActive,
		},
		IdempotencyKey: memory.ScopeKey(clientKey, scope),
		Fingerprint:    correlation,
	}
}

// memoryMutationIntent builds a legal fresh M-16 mutation intent for one
// member over one item.
func memoryMutationIntent(kind, itemID, opID, clientKey, correlation, scope string) memory.MutationIntent {
	return memory.MutationIntent{
		Kind: kind,
		Operation: memory.Operation{
			OperationID:   opID,
			Kind:          kind,
			State:         memory.OpStateRecorded,
			CorrelationID: correlation,
		},
		MemoryItemID: itemID,
		ClientKey:    clientKey,
		Fingerprint:  correlation,
	}
}

// memoryPendingFixture inserts a pending_review item directly (the Core's
// extraction path has no producer on the served surface — the store-level
// fixture exercises the review rulings).
func memoryPendingFixture(t *testing.T, s *Store, itemID string) {
	t.Helper()
	err := s.CommitStateChange(context.Background(), "corr-fixture", func(tx *sqlTx) error {
		return insertMemoryItem(tx, memory.Item{
			MemoryItemID:   itemID,
			Scope:          memory.ScopeUser,
			Origin:         memory.OriginModelExtracted,
			LifecycleState: memory.StatePendingReview,
			Content:        "pending fixture content",
		}, time.Now())
	})
	if err != nil {
		t.Fatalf("pending fixture: %v", err)
	}
}

// memoryItemEvents reads one aggregate's journal records.
func memoryItemEvents(t *testing.T, s *Store, aggregateType, aggregateID string) []events.Record {
	t.Helper()
	rows, err := s.readers.QueryContext(context.Background(),
		`SELECT event_id, aggregate_type, aggregate_id, sequence, event_type, schema_version, occurred_at, correlation_id, producer
		 FROM events_journal WHERE aggregate_type = ? AND aggregate_id = ? ORDER BY sequence ASC`, aggregateType, aggregateID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := []events.Record{}
	for rows.Next() {
		var r events.Record
		var occurred string
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.Sequence, &r.EventType,
			&r.SchemaVersion, &occurred, &r.CorrelationID, &r.Producer); err != nil {
			t.Fatalf("event scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("events: %v", err)
	}
	return out
}

// TestMemoryAddM17Outcomes pins the M-17 creation class exactly: the same
// key + the same request identity replays while live or within the 72 h
// post-terminal window; the same key with a different request identity is
// the typed CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false,
// keyScope memory.add); a replay past the window is the typed
// CONFLICT_STATE.
func TestMemoryAddM17Outcomes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	correlation := "corr-m17"
	opOutcome, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-m17", "memop-m17", "key-m17", correlation, memory.ScopeUser))
	if err != nil || opOutcome.Replayed {
		t.Fatalf("fresh add: replayed=%v err=%v", opOutcome.Replayed, err)
	}
	op := opOutcome
	if op.Item.MemoryItemID != "mem-m17" || op.Item.LifecycleState != memory.StateActive {
		t.Fatalf("the fresh add records the entry-state truth: %+v", op.Item)
	}
	// Same key + same request identity: the recorded add replays.
	replayOutcome, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-other", "memop-other", "key-m17", correlation, memory.ScopeUser))
	replay, replayed := replayOutcome, replayOutcome.Replayed
	if err != nil || !replayed || replay.OperationID != op.OperationID {
		t.Fatalf("M-17 replay: replayed=%v err=%v op=%+v", replayed, err, replay)
	}
	if replay.Item.MemoryItemID != "mem-m17" {
		t.Fatalf("the replay carries the recorded item, never a second one: %+v", replay.Item)
	}
	// Same key + different request identity: the typed conflict.
	conflictOutcome, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-x", "memop-x", "key-m17", "corr-other", memory.ScopeUser))
	conflict, conflictReplayed := conflictOutcome, conflictOutcome.Replayed
	if conflict.OperationID != "" || conflictReplayed || err == nil {
		t.Fatalf("different request identity must refuse, got op=%v replayed=%v err=%v", conflict, conflictReplayed, err)
	}
	ce, ok := err.(*MemoryIdempotencyConflictError)
	if !ok {
		t.Fatalf("expected the M-17 conflict carrier, got %T", err)
	}
	env, cerr := ce.Envelope(errs.NewCorrelationID())
	if cerr != nil {
		t.Fatalf("conflict envelope: %v", cerr)
	}
	if env.Code != errs.CodeConflictIdempotencyKey || env.Data["fingerprintMatch"] != false || env.Data["keyScope"] != "memory.add" {
		t.Fatalf("the M-17 conflict must carry its typed datum: %+v", env)
	}
	// Past the 72 h window: the typed CONFLICT_STATE.
	s.now = func() time.Time { return time.Now().Add(MemoryReplayWindow).Add(time.Nanosecond) }
	pastOutcome, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-y", "memop-y", "key-m17", correlation, memory.ScopeUser))
	pastOp, pastReplayed := pastOutcome, pastOutcome.Replayed
	if pastOp.OperationID != "" || pastReplayed || err == nil {
		t.Fatalf("past-window replay must refuse, got op=%v replayed=%v err=%v", pastOp, pastReplayed, err)
	}
	se, ok := err.(*Error)
	if !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected the typed CONFLICT_STATE, got %v", err)
	}
}

// TestMemoryMutationM16Outcomes pins the M-16 state-checked class: the
// same key + the same correlation + the same prepared-against reference
// replays within the 1 h post-terminal window; a different correlation or
// a moved state is the typed CONFLICT_STATE; the eligibility, review-state,
// and conflict preconditions are Core-enforced inside the transaction.
func TestMemoryMutationM16Outcomes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-m16", "memop-m16a", "key-m16", "corr-m16", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	item, err := s.LoadMemoryItem(ctx, "mem-m16")
	if err != nil {
		t.Fatalf("fixture load: %v", err)
	}
	stateRef := memory.StateRef(item.LifecycleState, item.UpdatedAt)

	// The edit: recorded; provenance members untouched; the state moves to
	// updated.
	intent := memoryMutationIntent(memory.KindEdit, "mem-m16", "memop-m16b", "key-m16", "corr-m16-edit", memory.ScopeUser)
	intent.EditContent = "the corrected content"
	intent.StateRef = stateRef
	outcome, err := s.ApplyMemoryMutation(ctx, intent)
	if err != nil || outcome.Replayed {
		t.Fatalf("fresh edit: replayed=%v err=%v", outcome.Replayed, err)
	}
	if outcome.Item.LifecycleState != memory.StateUpdated || outcome.Item.Content != "the corrected content" {
		t.Fatalf("the edit records the corrected post-state: %+v", outcome.Item)
	}
	if outcome.Item.Origin != memory.OriginUserEntered {
		t.Fatalf("the edit preserves the origin label (provenance law): %+v", outcome.Item)
	}
	edited, err := s.LoadMemoryItem(ctx, "mem-m16")
	if err != nil {
		t.Fatalf("load after edit: %v", err)
	}
	updatedRef := memory.StateRef(edited.LifecycleState, edited.UpdatedAt)

	// The replay: same key + same correlation + the same prepared-against
	// reference (the recorded one) replays within the 1 h window.
	replayIntent := intent
	replayIntent.Operation.OperationID = "memop-m16c"
	replayedOutcome, err := s.ApplyMemoryMutation(ctx, replayIntent)
	if err != nil || !replayedOutcome.Replayed || replayedOutcome.OperationID != outcome.OperationID {
		t.Fatalf("M-16 replay: outcome=%+v err=%v", replayedOutcome, err)
	}
	// Same key + a different correlation: the typed CONFLICT_STATE.
	otherIntent := intent
	otherIntent.Operation.CorrelationID = "corr-different"
	otherIntent.Fingerprint = "corr-different"
	otherIntent.Operation.OperationID = "memop-m16d"
	if o, mErr := s.ApplyMemoryMutation(ctx, otherIntent); mErr == nil || o.OperationID != "" {
		t.Fatalf("different correlation must CONFLICT_STATE, got %v", mErr)
	}
	// Same key + a different prepared-against reference: the typed
	// CONFLICT_STATE.
	staleIntent := intent
	staleIntent.StateRef = memory.StateRef(memory.StateActive, "stale-instant")
	staleIntent.Operation.OperationID = "memop-m16e"
	if _, mErr := s.ApplyMemoryMutation(ctx, staleIntent); mErr == nil {
		t.Fatalf("different state reference must CONFLICT_STATE")
	}
	// Past the 1 h window: the typed CONFLICT_STATE.
	s.now = func() time.Time { return time.Now().Add(MemoryMutationReplayWindow).Add(time.Nanosecond) }
	pastIntent := intent
	pastIntent.Operation.OperationID = "memop-m16f"
	if o, mErr := s.ApplyMemoryMutation(ctx, pastIntent); mErr == nil || o.OperationID != "" {
		t.Fatalf("past-window replay must CONFLICT_STATE, got %v", mErr)
	}
	s.now = time.Now

	// Eligibility: the pending item is not editable (MEMORY_NOT_EDITABLE).
	memoryPendingFixture(t, s, "mem-pending")
	pendIntent := memoryMutationIntent(memory.KindEdit, "mem-pending", "memop-m16g", "key-pending", "corr-pending", memory.ScopeUser)
	pendIntent.EditContent = "nope"
	pendIntent.StateRef = updatedRef
	if _, mErr := s.ApplyMemoryMutation(ctx, pendIntent); mErr == nil {
		t.Fatalf("editing a non-eligible item must refuse")
	} else if se, ok := mErr.(*Error); !ok || se.Code != errs.CodeMemoryNotEditable {
		t.Fatalf("expected MEMORY_NOT_EDITABLE, got %v", mErr)
	}
	// The state-reference conflict: a stale prepared-against reference on
	// an eligible item (MEMORY_CONFLICT — reconcile by re-reading, never
	// overwrite).
	conflictIntent := memoryMutationIntent(memory.KindEdit, "mem-m16", "memop-m16h", "key-conflict", "corr-conflict", memory.ScopeUser)
	conflictIntent.EditContent = "overwrite attempt"
	conflictIntent.StateRef = stateRef // the pre-edit reference — stale
	if _, mErr := s.ApplyMemoryMutation(ctx, conflictIntent); mErr == nil {
		t.Fatalf("a stale edit reference must refuse")
	} else if se, ok := mErr.(*Error); !ok || se.Code != errs.CodeMemoryConflict {
		t.Fatalf("expected MEMORY_CONFLICT, got %v", mErr)
	}
	// The review rulings: approve on a non-pending item is the typed
	// MEMORY_REVIEW_STATE_CONFLICT; on the pending fixture it Core-confirms.
	approveIntent := memoryMutationIntent(memory.KindApprove, "mem-m16", "memop-m16i", "key-approve", "corr-approve", memory.ScopeUser)
	if _, mErr := s.ApplyMemoryMutation(ctx, approveIntent); mErr == nil {
		t.Fatalf("approving an active item must refuse")
	} else if se, ok := mErr.(*Error); !ok || se.Code != errs.CodeMemoryReviewStateConflict {
		t.Fatalf("expected MEMORY_REVIEW_STATE_CONFLICT, got %v", mErr)
	}
	pendApprove := memoryMutationIntent(memory.KindApprove, "mem-pending", "memop-m16j", "key-approve-pending", "corr-approve-pending", memory.ScopeUser)
	pendOutcome, err := s.ApplyMemoryMutation(ctx, pendApprove)
	if err != nil || pendOutcome.Item.LifecycleState != memory.StateApproved {
		t.Fatalf("the pending approve: %+v err=%v", pendOutcome, err)
	}
	if pendOutcome.Item.LastConfirmedAt == "" {
		t.Fatalf("the approval stamps last_confirmed_at (the explicit user action): %+v", pendOutcome.Item)
	}
	eventsSeen := memoryItemEvents(t, s, memory.AggregateMemoryItem, "mem-pending")
	found := false
	for _, e := range eventsSeen {
		if e.EventType == memory.EventReviewApproved {
			found = true
		}
	}
	if !found {
		t.Fatalf("the approve emits memory_review.approved: %+v", eventsSeen)
	}
}

// TestMemoryDestructiveMembers pins the destructive discipline: forget and
// delete each drop the content and tombstone the identity row at their own
// confirmed register state; the removed item is the SAME typed
// MEMORY_ITEM_NOT_FOUND afterwards; the keyed destructive replay replays
// within the 1 h window; the unkeyed record (the recorded carrier gap) is
// non-replayable; the register events ride the member's own type.
func TestMemoryDestructiveMembers(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-del", "memop-del-a", "key-del", "corr-del", memory.ScopeSession))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	// The keyed delete: the tombstone records the confirmed outcome.
	delOutcome, err := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindDelete, "mem-del", "memop-del-b", "key-del2", "corr-del2", memory.ScopeSession))
	if err != nil || delOutcome.Item.LifecycleState != memory.StateDeleted {
		t.Fatalf("the delete: %+v err=%v", delOutcome, err)
	}
	if delOutcome.Item.Content != "" {
		t.Fatalf("the removal drops the content: %+v", delOutcome.Item)
	}
	// The item is no longer served: the SAME typed MEMORY_ITEM_NOT_FOUND.
	if _, err := s.LoadMemoryItem(ctx, "mem-del"); err == nil {
		t.Fatalf("a removed item must not be served")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeMemoryItemNotFound {
		t.Fatalf("expected MEMORY_ITEM_NOT_FOUND, got %v", err)
	}
	// Any further mutation is the same typed 404.
	if _, mErr := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindEdit, "mem-del", "memop-del-c", "key-del3", "corr-del3", memory.ScopeSession)); mErr == nil {
		t.Fatalf("mutating a removed item must refuse")
	}
	// The keyed forget: replay within the window; the register events ride
	// the member's own type.
	_, err = s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-forget", "memop-fg-a", "key-fg", "corr-fg", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add 2: %v", err)
	}
	fgOutcome, err := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindForget, "mem-forget", "memop-fg-b", "key-fg2", "corr-fg2", memory.ScopeUser))
	if err != nil || fgOutcome.Item.LifecycleState != memory.StateForgotten {
		t.Fatalf("the forget: %+v err=%v", fgOutcome, err)
	}
	replayIntent := memoryMutationIntent(memory.KindForget, "mem-forget", "memop-fg-c", "key-fg2", "corr-fg2", memory.ScopeUser)
	replay, err := s.ApplyMemoryMutation(ctx, replayIntent)
	if err != nil || !replay.Replayed || replay.OperationID != fgOutcome.OperationID {
		t.Fatalf("the forget replay: %+v err=%v", replay, err)
	}
	delEvents := memoryItemEvents(t, s, memory.AggregateMemoryItem, "mem-del")
	if len(delEvents) == 0 || delEvents[len(delEvents)-1].EventType != memory.EventItemDeleted {
		t.Fatalf("the delete emits memory_item.deleted: %+v", delEvents)
	}
	fgEvents := memoryItemEvents(t, s, memory.AggregateMemoryItem, "mem-forget")
	if len(fgEvents) == 0 || fgEvents[len(fgEvents)-1].EventType != memory.EventItemForgotten {
		t.Fatalf("the forget emits its member's register event: %+v", fgEvents)
	}
	// The unkeyed destructive record (the recorded carrier gap): recorded
	// with no key row; non-replayable by construction.
	_, err = s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-unkeyed", "memop-uk-a", "key-uk", "corr-uk", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add 3: %v", err)
	}
	unkeyed := memoryMutationIntent(memory.KindDelete, "mem-unkeyed", "memop-uk-b", "", "corr-uk2", memory.ScopeUser)
	ukOutcome, ukErr := s.ApplyMemoryMutation(ctx, unkeyed)
	if ukErr != nil || ukOutcome.Item.LifecycleState != memory.StateDeleted {
		t.Fatalf("the unkeyed delete: %+v err=%v", ukOutcome, ukErr)
	}
	// The state-checked members REQUIRE their key.
	keylessEdit := memoryMutationIntent(memory.KindEdit, "mem-unkeyed", "memop-uk-c", "", "corr-uk3", memory.ScopeUser)
	keylessEdit.EditContent = "x"
	if _, kErr := s.ApplyMemoryMutation(ctx, keylessEdit); kErr == nil {
		t.Fatalf("a keyless edit must refuse")
	}
}

// TestMemoryPinRegister pins the pin/unpin member: the pin state is a FLAG
// on the item (this Core's served definition of the recorded gap), both
// directions Core-confirmed, and the register events ride the direction.
func TestMemoryPinRegister(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-pin", "memop-pin-a", "key-pin", "corr-pin", memory.ScopeWorkspace))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	pinOutcome, err := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindPin, "mem-pin", "memop-pin-b", "key-pin2", "corr-pin2", memory.ScopeWorkspace))
	if err != nil || !pinOutcome.Item.Pinned {
		t.Fatalf("the pin: %+v err=%v", pinOutcome, err)
	}
	unpinOutcome, err := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindUnpin, "mem-pin", "memop-pin-c", "key-pin3", "corr-pin3", memory.ScopeWorkspace))
	if err != nil || unpinOutcome.Item.Pinned {
		t.Fatalf("the unpin: %+v err=%v", unpinOutcome, err)
	}
	foundPinned, foundUnpinned := false, false
	for _, e := range memoryItemEvents(t, s, memory.AggregateMemoryItem, "mem-pin") {
		if e.EventType == memory.EventItemPinned {
			foundPinned = true
		}
		if e.EventType == memory.EventItemUnpinned {
			foundUnpinned = true
		}
	}
	if !foundPinned || !foundUnpinned {
		t.Fatalf("the pin register events: pinned=%v unpinned=%v", foundPinned, foundUnpinned)
	}
	// The pin replay: same key + same correlation replays the recorded
	// outcome.
	replay, err := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindPin, "mem-pin", "memop-pin-d", "key-pin2", "corr-pin2", memory.ScopeWorkspace))
	if err != nil || !replay.Replayed {
		t.Fatalf("the pin replay: %+v err=%v", replay, err)
	}
}

// TestMemoryInclusionM18 pins the M-18 class: a per-request decision that
// changes no item state; the same request identity replays the recorded
// decision; a different correlation under the same request is the caller's
// new decision record.
func TestMemoryInclusionM18(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-inc", "memop-inc-a", "key-inc", "corr-inc", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	buildIntent := func(opID, correlation string, included bool) memory.InclusionIntent {
		return memory.InclusionIntent{
			Operation: memory.Operation{
				OperationID:   opID,
				Kind:          memory.KindInclusion,
				State:         memory.OpStateRecorded,
				CorrelationID: correlation,
			},
			MemoryItemID:   "mem-inc",
			RequestRef:     "req-alpha",
			Included:       included,
			IdempotencyKey: memory.InclusionKey("req-alpha", "mem-inc"),
			Fingerprint:    correlation + "\x1f" + "mem-inc",
		}
	}
	outcome, err := s.SetMemoryInclusion(ctx, buildIntent("memop-inc-b", "corr-inc2", true))
	if err != nil || outcome.Replayed || !outcome.Result.Included {
		t.Fatalf("the inclusion decision: %+v err=%v", outcome, err)
	}
	replay, err := s.SetMemoryInclusion(ctx, buildIntent("memop-inc-c", "corr-inc2", true))
	if err != nil || !replay.Replayed || replay.Result.OperationID != outcome.Result.OperationID {
		t.Fatalf("the M-18 replay: %+v err=%v", replay, err)
	}
	fresh, err := s.SetMemoryInclusion(ctx, buildIntent("memop-inc-d", "corr-inc3", false))
	if err != nil || fresh.Replayed || fresh.Result.Included {
		t.Fatalf("the new decision under the same request: %+v err=%v", fresh, err)
	}
	// The decision changed no item state.
	item, err := s.LoadMemoryItem(ctx, "mem-inc")
	if err != nil || item.LifecycleState != memory.StateActive {
		t.Fatalf("the M-18 decision never changes the item state: %+v err=%v", item, err)
	}
	// The operation record resolves through the state endpoint.
	if _, err := s.LoadMemoryOperation(ctx, outcome.Result.OperationID); err != nil {
		t.Fatalf("the inclusion operation record: %v", err)
	}
}

// TestMemoryExportRecordAndItemsByID pins the M-32 record (no idempotency
// row, no replay window — every submit a new operation) and the export's
// explicit-subset read (unknown and removed items are the SAME typed 404).
func TestMemoryExportRecordAndItemsByID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-exp", "memop-exp-a", "key-exp", "corr-exp", memory.ScopePolicyDefined))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	op := memory.Operation{OperationID: "memop-exp-b", Kind: memory.KindExport, State: memory.OpStateRecorded, CorrelationID: "corr-exp2"}
	recorded, err := s.SubmitMemoryExport(ctx, memory.ExportIntent{Operation: op, ExportRef: "sha256:" + strings.Repeat("0", 64)})
	if err != nil || recorded.OperationID != "memop-exp-b" || recorded.ExportRef == "" {
		t.Fatalf("the export record: %+v err=%v", recorded, err)
	}
	items, err := s.MemoryItemsByID(ctx, []string{"mem-exp"})
	if err != nil || len(items) != 1 {
		t.Fatalf("items by id: %v", err)
	}
	if _, err := s.MemoryItemsByID(ctx, []string{"mem-nope"}); err == nil {
		t.Fatalf("an unknown named item must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeMemoryItemNotFound {
		t.Fatalf("expected MEMORY_ITEM_NOT_FOUND, got %v", err)
	}
	// The unknown operation: the typed NOT_FOUND (the no-leak rule).
	if _, err := s.LoadMemoryOperation(ctx, "memop-nope"); err == nil {
		t.Fatalf("an unknown operation must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

// TestMemoryListFilters pins the keyset list: the scope and state filters
// are Core-side (scope never silently broadens); removed items are not
// served; the page bound drives the More flag.
func TestMemoryListFilters(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, err := s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-l1", "memop-l1", "key-l1", "corr-l1", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add 1: %v", err)
	}
	_, err = s.SubmitMemoryAdd(ctx, memoryAddIntent("mem-l2", "memop-l2", "key-l2", "corr-l2", memory.ScopeSession))
	if err != nil {
		t.Fatalf("fixture add 2: %v", err)
	}
	page, err := s.ListMemoryItems(ctx, memory.ListQuery{Limit: 10, Scope: memory.ScopeSession})
	if err != nil || len(page.Items) != 1 || page.Items[0].MemoryItemID != "mem-l2" {
		t.Fatalf("the scope filter: %+v err=%v", page, err)
	}
	page, err = s.ListMemoryItems(ctx, memory.ListQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 || !page.More {
		t.Fatalf("the page bound: %+v err=%v", page, err)
	}
	// The removal hides the item from the list too.
	if _, dErr := s.ApplyMemoryMutation(ctx, memoryMutationIntent(memory.KindDelete, "mem-l2", "memop-l3", "key-l2b", "corr-l2b", memory.ScopeSession)); dErr != nil {
		t.Fatalf("delete fixture: %v", err)
	}
	page, err = s.ListMemoryItems(ctx, memory.ListQuery{Limit: 10, Scope: memory.ScopeSession})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("removed items are not served: %+v err=%v", page, err)
	}
}
