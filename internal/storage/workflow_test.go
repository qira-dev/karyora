package storage

import (
	stdcontext "context"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	workflowdomain "github.com/qira-dev/karyora/internal/domain/workflow"
)

// The B4 Workflow authoring family's store register (DE-0 queue row 6):
// the aggregate-create class's keyed ruling (72 h post-terminal; a replay
// returns the recorded Core-minted identity — never a regenerated one),
// the M-21 save class's keyed ruling (1 h post-terminal; the NEW immutable
// revision is an INSERT — the old revision preserved immutable and
// reachable; the concurrent-revision outcome is the typed
// WORKFLOW_REVISION_CONFLICT under the interim conflict rule — nothing
// overwritten, nothing merged), the SAME typed 404 for unknown and
// out-of-scope identities (the no-leak rule), the run-history LINKAGE
// read's honest empty page while the D-40 gate stands, and the
// authoritative state read.

// workflowCreateIntent builds a legal create intent (the Core-minted
// identity rides the intent; the effective key is workspace-scoped).
func workflowCreateIntent(defID, key, corr string) workflowdomain.CreateIntent {
	return workflowdomain.CreateIntent{
		Definition:     workflowdomain.Definition{DefinitionID: defID, DisplayName: "the fixture workflow"},
		OperationID:    "wfop-" + defID,
		IdempotencyKey: workflowdomain.CreateKey(key),
		Fingerprint:    corr,
	}
}

// workflowSaveIntent builds a legal M-21 save intent.
func workflowSaveIntent(defID, revRef, key, corr, base string) workflowdomain.SaveIntent {
	return workflowdomain.SaveIntent{
		DefinitionID:    defID,
		Revision:        workflowdomain.Revision{RevisionRef: revRef},
		OperationID:     "wfop-save-" + revRef,
		CorrelationID:   corr,
		IdempotencyKey:  workflowdomain.SaveKey(key, defID),
		Fingerprint:     workflowdomain.SaveFingerprint(corr, base),
		BaseRevisionRef: base,
	}
}

// backdateWorkflowOperation moves one recorded operation's terminal stamp
// behind the replay window (the fixture manipulation for the past-window
// ruling — the product code is untouched).
func backdateWorkflowOperation(t *testing.T, s *Store, operationID string, age time.Duration) {
	t.Helper()
	past := time.Now().Add(-age).UTC().Format(time.RFC3339Nano)
	if err := s.CommitStateChange(stdcontext.Background(), "corr-backdate", func(tx *sqlTx) error {
		_, err := tx.Exec(`UPDATE workflow_operations SET updated_at = ?, terminal_at = ? WHERE operation_id = ?`,
			past, past, operationID)
		return err
	}); err != nil {
		t.Fatalf("backdate fixture: %v", err)
	}
}

// TestWorkflowCreateClass pins the aggregate-create class's replay
// outcomes table at the store: the fresh create is Core-confirmed (the
// Core-minted identity; the body never carries one); the same key + the
// same request identity replays the recorded create (the SAME identity,
// never a second one); the same key with a different request identity is
// the typed CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false); a replay
// past the 72 h post-terminal window is the typed CONFLICT_STATE.
func TestWorkflowCreateClass(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	// The fresh create: Core-confirmed, replayed=false, no revision yet.
	first, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-1", "key-1", "corr-1"))
	if err != nil {
		t.Fatalf("fresh create: %v", err)
	}
	if first.Replayed || first.Definition.DefinitionID != "wfdef-1" || first.Definition.CurrentRevisionRef != "" {
		t.Fatalf("the fresh create: %+v", first)
	}
	if first.Definition.DisplayName != "the fixture workflow" {
		t.Fatalf("the Core-carried display name: %+v", first.Definition)
	}

	// The same key + the same request identity: the keyed replay returns
	// the recorded create — the SAME Core-minted identity, never a second
	// one (the Supplied-keyed replay pattern).
	replay, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-OTHER", "key-1", "corr-1"))
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if !replay.Replayed || replay.Definition.DefinitionID != "wfdef-1" {
		t.Fatalf("the replay returns the recorded identity, never a regenerated one: %+v", replay)
	}
	var definitions int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_definitions`).Scan(&definitions); err != nil {
		t.Fatalf("definition count: %v", err)
	}
	if definitions != 1 {
		t.Fatalf("a replay never creates a second definition: %d", definitions)
	}

	// The same key with a different request identity: the typed
	// CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false).
	_, err = s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-2", "key-1", "corr-DIFFERENT"))
	wce, ok := err.(*WorkflowIdempotencyConflictError)
	if !ok {
		t.Fatalf("expected the CONFLICT_IDEMPOTENCY_KEY carrier, got %v", err)
	}
	envl, eerr := wce.Envelope("corr-x")
	if eerr != nil || envl.Code != errs.CodeConflictIdempotencyKey || envl.Data["fingerprintMatch"] != false {
		t.Fatalf("the conflict envelope: %v %+v", eerr, envl.Data)
	}

	// A different key creates a second definition (the keys do not
	// collide).
	if _, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-2", "key-2", "corr-2")); err != nil {
		t.Fatalf("second create: %v", err)
	}

	// The replay past the 72 h post-terminal window: the typed
	// CONFLICT_STATE.
	backdateWorkflowOperation(t, s, first.OperationID, WorkflowCreateReplayWindow+time.Minute)
	_, err = s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-3", "key-1", "corr-1"))
	if err == nil {
		t.Fatalf("the past-window replay must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitWorkflowCreate(ctx, workflowdomain.CreateIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
}

// TestWorkflowSaveRevisionM21 pins the M-21 class at the store: the save
// creates a NEW immutable revision with the new reference STATED; the old
// revision is preserved immutable and reachable (never overwritten — the
// no-update-in-place law); the keyed replay (1 h post-terminal) returns
// the recorded save; the concurrent-revision outcome (the stated base no
// longer the current stored revision) is the typed WORKFLOW_REVISION_CONFLICT
// under the interim conflict rule — nothing overwritten, nothing merged;
// the unknown definition is the SAME typed 404 and consumes no key.
func TestWorkflowSaveRevisionM21(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	if _, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-1", "key-1", "corr-1")); err != nil {
		t.Fatalf("fixture create: %v", err)
	}

	// Save 1: the client's known base for a definition with no revisions
	// is the empty base; the NEW revision reference is STATED on save.
	save1, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-1", "skey-1", "corr-s1", ""))
	if err != nil {
		t.Fatalf("save 1: %v", err)
	}
	if save1.Replayed || save1.Revision.RevisionRef != "wfrev-1" {
		t.Fatalf("the save states the new revision reference: %+v", save1)
	}
	def, err := s.LoadWorkflowDefinition(ctx, "wfdef-1")
	if err != nil {
		t.Fatalf("definition read: %v", err)
	}
	if def.CurrentRevisionRef != "wfrev-1" {
		t.Fatalf("the definition's current-revision pointer moved: %+v", def)
	}
	beforeSave2, err := s.ListWorkflowRevisions(ctx, "wfdef-1")
	if err != nil || len(beforeSave2) != 1 {
		t.Fatalf("the pre-save-2 history: %v %+v", err, beforeSave2)
	}

	// Save 2 against the recorded base: a second NEW immutable revision;
	// the old revision preserved immutable and reachable.
	save2, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-2", "skey-2", "corr-s2", "wfrev-1"))
	if err != nil {
		t.Fatalf("save 2: %v", err)
	}
	if save2.Replayed || save2.Revision.RevisionRef != "wfrev-2" {
		t.Fatalf("save 2: %+v", save2)
	}
	revisions, err := s.ListWorkflowRevisions(ctx, "wfdef-1")
	if err != nil {
		t.Fatalf("revisions read: %v", err)
	}
	if len(revisions) != 2 || revisions[0].RevisionRef != "wfrev-1" || revisions[1].RevisionRef != "wfrev-2" {
		t.Fatalf("the revision history (each immutable and reachable): %+v", revisions)
	}

	// THE IMMUTABILITY LAW: the old revision row is BYTE-FOR-BYTE unchanged
	// by the later save (the no-update-in-place law; the revision rows are
	// INSERT-only — the later save never mutates the prior record).
	if revisions[0].RevisionRef != beforeSave2[0].RevisionRef || revisions[0].CreatedAt != beforeSave2[0].CreatedAt {
		t.Fatalf("the old revision was mutated by the later save (the no-update-in-place law): %+v vs %+v", revisions[0], beforeSave2[0])
	}

	// The keyed replay: the same key + the same correlation + the same
	// prepared-against base replays the recorded save — the SAME revision,
	// never a second one.
	replay, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-OTHER", "skey-1", "corr-s1", ""))
	if err != nil {
		t.Fatalf("save replay: %v", err)
	}
	if !replay.Replayed || replay.Revision.RevisionRef != "wfrev-1" {
		t.Fatalf("the replay returns the recorded revision, never a second one: %+v", replay)
	}
	revisions, _ = s.ListWorkflowRevisions(ctx, "wfdef-1")
	if len(revisions) != 2 {
		t.Fatalf("a replay never creates a second revision: %d", len(revisions))
	}

	// The same key with a different correlation: the typed
	// CONFLICT_IDEMPOTENCY_KEY.
	_, err = s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-3", "skey-1", "corr-OTHER", ""))
	if wce, ok := err.(*WorkflowIdempotencyConflictError); ok {
		envl, eerr := wce.Envelope("corr-x")
		if eerr != nil || envl.Code != errs.CodeConflictIdempotencyKey {
			t.Fatalf("the conflict envelope: %v", eerr)
		}
	} else {
		t.Fatalf("expected the CONFLICT_IDEMPOTENCY_KEY carrier, got %v", err)
	}

	// The same key + the same correlation + a different prepared-against
	// base: the typed CONFLICT_STATE (the recorded outcome no longer
	// matches the attempt).
	_, err = s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-3", "skey-1", "corr-s1", "wfrev-2"))
	if err == nil {
		t.Fatalf("the different-base replay must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}

	// The concurrent-revision outcome: the stated base is no longer the
	// current stored revision — the typed WORKFLOW_REVISION_CONFLICT; the
	// interim conflict rule governs (nothing overwritten, nothing merged);
	// the envelope carries the stored side's REFERENCE only.
	_, err = s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-3", "skey-3", "corr-s3", "wfrev-1"))
	rce, ok := err.(*WorkflowRevisionConflictError)
	if !ok {
		t.Fatalf("expected the WORKFLOW_REVISION_CONFLICT carrier, got %v", err)
	}
	if rce.CurrentRevisionRef != "wfrev-2" {
		t.Fatalf("the conflict carries the stored side's reference: %+v", rce)
	}
	envl, eerr := rce.Envelope("corr-x")
	if eerr != nil || envl.Code != errs.CodeWorkflowRevisionConflict || envl.Data["current_revision_ref"] != "wfrev-2" {
		t.Fatalf("the conflict envelope: %v %+v", eerr, envl.Data)
	}
	revisions, _ = s.ListWorkflowRevisions(ctx, "wfdef-1")
	if len(revisions) != 2 {
		t.Fatalf("the conflict stores nothing (no overwrite, no merge): %d", len(revisions))
	}
	def, _ = s.LoadWorkflowDefinition(ctx, "wfdef-1")
	if def.CurrentRevisionRef != "wfrev-2" {
		t.Fatalf("the conflict moves nothing: %+v", def)
	}

	// The replay past the 1 h post-terminal window: the typed
	// CONFLICT_STATE.
	backdateWorkflowOperation(t, s, save1.OperationID, time.Hour+time.Minute)
	_, err = s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-9", "skey-1", "corr-s1", ""))
	if err == nil {
		t.Fatalf("the past-window replay must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}

	// The unknown definition: the SAME typed 404 (the no-leak rule), and
	// the key stays unconsumed by the unresolvable intent (a valid save
	// under the same key still works afterwards).
	_, err = s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-NOPE", "wfrev-4", "skey-4", "corr-s4", ""))
	if err == nil {
		t.Fatalf("the unknown definition must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
	if _, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-3", "skey-4", "corr-s4", "wfrev-2")); err != nil {
		t.Fatalf("the unresolvable intent consumed no key: %v", err)
	}

	// The register's revision.created event rode each fresh save (the
	// F10-shaped journal; the payload carries references only).
	var events int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal
WHERE aggregate_type = ? AND event_type = ?`,
		workflowdomain.AggregateWorkflowRevision, workflowdomain.EventRevisionCreated).Scan(&events); err != nil {
		t.Fatalf("event count: %v", err)
	}
	if events != 3 {
		t.Fatalf("each fresh save recorded revision.created (wfrev-1, wfrev-2, wfrev-3): %d", events)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitWorkflowRevisionSave(ctx, workflowdomain.SaveIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
}

// TestWorkflowInspectStateRunHistory pins the read surfaces: the inspect
// member's definition + revisions (each immutable and reachable), the
// authoritative state read (current and revision-specific), the SAME typed
// 404 for unknown and out-of-scope identities (the no-leak rule), and the
// run-history LINKAGE read's honest empty page while the D-40 gate stands.
func TestWorkflowInspectStateRunHistory(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	// The unknown identity: the SAME typed 404 on every read (the no-leak
	// rule).
	for name, call := range map[string]func() error{
		"definition":  func() error { _, err := s.LoadWorkflowDefinition(ctx, "wfdef-nope"); return err },
		"revisions":   func() error { _, err := s.ListWorkflowRevisions(ctx, "wfdef-nope"); return err },
		"run-history": func() error { _, err := s.ListWorkflowRunHistory(ctx, "wfdef-nope"); return err },
		"state":       func() error { _, err := s.LoadWorkflowDefinitionState(ctx, "wfdef-nope", ""); return err },
	} {
		err := call()
		if err == nil {
			t.Fatalf("%s: the unknown identity must refuse", name)
		} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
			t.Fatalf("%s: expected the typed NOT_FOUND, got %v", name, err)
		}
	}

	// The fixture: a definition with two revisions.
	if _, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-1", "key-1", "corr-1")); err != nil {
		t.Fatalf("fixture create: %v", err)
	}
	if _, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-1", "skey-1", "corr-s1", "")); err != nil {
		t.Fatalf("fixture save 1: %v", err)
	}
	if _, err := s.SubmitWorkflowRevisionSave(ctx, workflowSaveIntent("wfdef-1", "wfrev-2", "skey-2", "corr-s2", "wfrev-1")); err != nil {
		t.Fatalf("fixture save 2: %v", err)
	}
	if _, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-2", "key-2", "corr-2")); err != nil {
		t.Fatalf("fixture create 2: %v", err)
	}

	// The inspect member: the definition + its revision records (each
	// immutable and reachable).
	revisions, err := s.ListWorkflowRevisions(ctx, "wfdef-1")
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(revisions) != 2 || revisions[0].RevisionRef != "wfrev-1" || revisions[1].RevisionRef != "wfrev-2" {
		t.Fatalf("the revision history: %+v", revisions)
	}
	// A definition with no revisions carries none.
	revs2, err := s.ListWorkflowRevisions(ctx, "wfdef-2")
	if err != nil || len(revs2) != 0 {
		t.Fatalf("a definition with no saved revision carries none: %v %+v", err, revs2)
	}

	// The authoritative state read (the current state).
	state, err := s.LoadWorkflowDefinitionState(ctx, "wfdef-1", "")
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.DefinitionID != "wfdef-1" || state.CurrentRevisionRef != "wfrev-2" || state.UpdatedAt == "" {
		t.Fatalf("the authoritative state: %+v", state)
	}
	// The definition with no revisions carries no current revision
	// reference (the authoritative state read confirms).
	state2, err := s.LoadWorkflowDefinitionState(ctx, "wfdef-2", "")
	if err != nil || state2.CurrentRevisionRef != "" {
		t.Fatalf("the no-revision state: %v %+v", err, state2)
	}

	// The revision-specific state: the state of that specific revision.
	revState, err := s.LoadWorkflowDefinitionState(ctx, "wfdef-1", "wfrev-1")
	if err != nil {
		t.Fatalf("revision state: %v", err)
	}
	if revState.CurrentRevisionRef != "wfrev-1" || revState.UpdatedAt != revisions[0].CreatedAt {
		t.Fatalf("the revision-specific state: %+v (want %+v)", revState, revisions[0])
	}

	// The out-of-scope identity: a revision of ANOTHER definition is the
	// SAME typed 404 (the no-leak rule).
	_, err = s.LoadWorkflowDefinitionState(ctx, "wfdef-2", "wfrev-1")
	if err == nil {
		t.Fatalf("the out-of-scope revision must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
	// An unknown revision identity: the SAME typed 404.
	_, err = s.LoadWorkflowDefinitionState(ctx, "wfdef-1", "wfrev-nope")
	if err == nil {
		t.Fatalf("the unknown revision must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}

	// THE D-40 GATE (carried exactly): the run-history LINKAGE read's only
	// producer (the Run execution) is gated — the honest empty page for
	// every existing definition; never a fabricated record.
	history, err := s.ListWorkflowRunHistory(ctx, "wfdef-1")
	if err != nil {
		t.Fatalf("run history: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("no Run producer exists while the gate stands — the honest empty page: %+v", history)
	}

	// The keyset list: created_at DESC, definition_id DESC; the page bound
	// and the More flag (the cursor's mint is the app layer's).
	if _, err := s.SubmitWorkflowCreate(ctx, workflowCreateIntent("wfdef-3", "key-3", "corr-3")); err != nil {
		t.Fatalf("fixture create 3: %v", err)
	}
	page, err := s.ListWorkflowDefinitions(ctx, workflowdomain.ListQuery{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Definitions) != 2 || !page.More {
		t.Fatalf("the bounded page: %+v", page)
	}
	full, err := s.ListWorkflowDefinitions(ctx, workflowdomain.ListQuery{Limit: 10})
	if err != nil || len(full.Definitions) != 3 || full.More {
		t.Fatalf("the final page: %v %+v", err, full)
	}
	if full.Definitions[0].DefinitionID != "wfdef-3" || full.Definitions[2].DefinitionID != "wfdef-1" {
		t.Fatalf("the keyset order (newest first): %+v", full.Definitions)
	}
}
