package storage

import (
	stdcontext "context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/workflow"
	"github.com/qira-dev/karyora/internal/events"
)

// The Store implements the B4 Workflow authoring family's durable surface
// (DE-0 queue row 6): the aggregate-create class's keyed ruling (72 h
// post-terminal) and the M-21 save class's keyed ruling (1 h post-terminal)
// run transactionally under the single-writer gate; the NEW immutable
// revision is an INSERT — the revision rows are NEVER updated or removed
// (the no-update-in-place law; the old revision stays immutable and
// reachable); the definition record's current_revision_ref is the Core's
// own mutable pointer (the authoritative state), never a revision
// mutation. THE D-40 GATE (carried exactly): the run-execution operation
// DOES NOT EXIST — the run-history linkage read's only producer (the Run
// execution) is gated, so the workflow_run_history table backs the pure
// read with zero rows today (the machinery for the day the mapping is
// contracted; never a fabricated record — the B3 context_operations
// precedent). THE IF-MATCH CLAUSE: prose-only — no precondition member
// exists anywhere; base_revision_ref carries the client's known base and
// asserts no protocol; the interim conflict rule governs (no overwrite; no
// merge without an approved policy). THE REDACTION LAW: event payloads
// carry references only — never draft content, never secret values.
// Events are never the source of truth — the authoritative reads are.

// WorkflowCreateReplayWindow is the aggregate-create class's post-terminal
// replay window: 72 h (the F6-precedent window selected at the CF-4
// publication act — the shared recorded IdempotencyReplayWindow constant).
const WorkflowCreateReplayWindow = IdempotencyReplayWindow

// WorkflowSaveReplayWindow is the M-21 save class's post-terminal replay
// window: 1 h (the short-window M-16-class precedent selected at the CF-4
// publication act).
const WorkflowSaveReplayWindow = time.Hour

// WorkflowIdempotencyConflictError carries the keyed classes'
// same-key-different-identity conflict's envelope: the published
// CONFLICT_IDEMPOTENCY_KEY with its fingerprintMatch:false datum (the
// same typed shape the memory add and knowledge import conflicts ride).
type WorkflowIdempotencyConflictError struct{ Detail string }

func (e *WorkflowIdempotencyConflictError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", errs.CodeConflictIdempotencyKey, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *WorkflowIdempotencyConflictError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeConflictIdempotencyKey, correlationID, map[string]any{
		"keyScope":         "workflow",
		"fingerprintMatch": false,
		"reason":           e.Detail,
	})
}

// WorkflowRevisionConflictError carries the concurrent-revision outcome's
// envelope: the published WORKFLOW_REVISION_CONFLICT with the stored
// side's reference only (both sides shown separately and labeled — the
// stored side by its reference, the local draft surviving client-side;
// the redaction law: references, never content).
type WorkflowRevisionConflictError struct {
	CurrentRevisionRef string
	Detail             string
}

func (e *WorkflowRevisionConflictError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", errs.CodeWorkflowRevisionConflict, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *WorkflowRevisionConflictError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeWorkflowRevisionConflict, correlationID, map[string]any{
		"current_revision_ref": e.CurrentRevisionRef,
		"reason":               e.Detail,
	})
}

// errWorkflowDuplicateKey marks the in-transaction same-key insert race
// that resolves to the replay ruling (the SubmitJob duplicate idiom).
var errWorkflowDuplicateKey = errors.New("storage: duplicate workflow idempotency key")

// The keyed classes' durable operation-record kinds — this store's own
// identity discriminator (no operation-kind schema is published by the
// family; the keyed rulings' records are the store's own identity
// members).
const (
	workflowKindCreate       = "create"
	workflowKindSaveRevision = "save_revision"
)

// workflowOperationRecord is the store's own durable operation-record
// shape for the keyed classes.
type workflowOperationRecord struct {
	OperationID   string
	Kind          string
	CorrelationID string
	DefinitionID  string
	RevisionRef   string
	CreatedAt     string
	UpdatedAt     string
}

// SubmitWorkflowCreate applies the aggregate-create class's ruling and
// stores the fresh Core-minted definition with its operation record when
// no replay resolves. THE CORE-MINTS-THE-IDENTITY LAW: the definition
// identity rides the intent (minted by the app layer) — the body never
// carries one. The effective key is workspace-scoped (CreateKey); the
// fingerprint is the request identity (the correlation ID); the same key +
// the same request identity replays the recorded create while the
// operation is live or within the 72 h post-terminal window; the same key
// with a different request identity is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); a replay past the window is the typed
// CONFLICT_STATE. A replay returns the recorded Core-minted identity —
// never a second one (the Supplied-keyed replay pattern).
func (s *Store) SubmitWorkflowCreate(ctx stdcontext.Context, intent workflow.CreateIntent) (workflow.CreateOutcome, error) {
	def := intent.Definition
	if def.DefinitionID == "" || intent.OperationID == "" ||
		intent.IdempotencyKey == "" || intent.Fingerprint == "" {
		return workflow.CreateOutcome{}, errStoragef(errs.CodeValidationFailed, "workflow create intent lacks its identity members")
	}
	// The aggregate-create ruling: one operation per effective key.
	if prior, err := s.workflowOpByKey(ctx, intent.IdempotencyKey); err != nil {
		return workflow.CreateOutcome{}, err
	} else if prior != nil {
		return s.resolveWorkflowCreateReplay(ctx, prior, intent)
	}
	err := s.CommitStateChange(ctx, intent.Fingerprint, func(tx *sqlTx) error {
		if err := insertWorkflowDefinition(tx, def, s.now()); err != nil {
			return err
		}
		return insertWorkflowOperation(tx, workflowOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          workflowKindCreate,
			CorrelationID: intent.Fingerprint,
			DefinitionID:  def.DefinitionID,
		}, intent.IdempotencyKey, intent.Fingerprint, s.now())
	})
	if err == errWorkflowDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		prior, lookupErr := s.workflowOpByKey(ctx, intent.IdempotencyKey)
		if lookupErr != nil || prior == nil {
			return workflow.CreateOutcome{}, err
		}
		return s.resolveWorkflowCreateReplay(ctx, prior, intent)
	}
	if err != nil {
		return workflow.CreateOutcome{}, err
	}
	fresh, err := s.LoadWorkflowDefinition(ctx, def.DefinitionID)
	if err != nil {
		return workflow.CreateOutcome{}, err
	}
	return workflow.CreateOutcome{Definition: fresh, OperationID: intent.OperationID, Replayed: false}, nil
}

// resolveWorkflowCreateReplay applies the aggregate-create class exactly:
// the same key + the same request identity (the correlation ID) replays
// the recorded create within the 72 h post-terminal window; the same key
// with a different request identity is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); a replay past the window is the typed
// CONFLICT_STATE.
func (s *Store) resolveWorkflowCreateReplay(ctx stdcontext.Context, prior *workflowOperationRecord, intent workflow.CreateIntent) (workflow.CreateOutcome, error) {
	if prior.CorrelationID != intent.Fingerprint {
		return workflow.CreateOutcome{}, &WorkflowIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity; the create fingerprint does not match (recorded correlation %q)", prior.CorrelationID),
		}
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return workflow.CreateOutcome{}, errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > WorkflowCreateReplayWindow {
		return workflow.CreateOutcome{}, errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", WorkflowCreateReplayWindow)
	}
	def, err := s.LoadWorkflowDefinition(ctx, prior.DefinitionID)
	if err != nil {
		// The recorded identity is the only truth a replay may return; the
		// definitions are never removed, so an unresolvable recorded
		// identity is a corrupt-state refusal — never a regenerated one.
		return workflow.CreateOutcome{}, err
	}
	return workflow.CreateOutcome{Definition: def, OperationID: prior.OperationID, Replayed: true}, nil
}

// SubmitWorkflowRevisionSave applies the M-21 class's ruling and stores
// the NEW immutable revision with its operation record and the
// revision.created event when no replay resolves. THE NO-UPDATE-IN-PLACE
// LAW (binding): the revision row is an INSERT — the old revision is
// never overwritten, never silently replaced; the definition record's
// current_revision_ref is the Core's own mutable pointer. The effective
// key is workspace + definition-scoped (SaveKey); the fingerprint is the
// correlation ID + the revision's current state reference (the client's
// known base — base_revision_ref, which asserts no protocol). The
// concurrent-revision outcome (the client's base no longer the current
// stored revision) is the typed WORKFLOW_REVISION_CONFLICT — the interim
// conflict rule governs: nothing is overwritten, nothing is merged, and
// the conflict data carries the stored side's REFERENCE only (the
// redaction law; the local draft survives client-side).
func (s *Store) SubmitWorkflowRevisionSave(ctx stdcontext.Context, intent workflow.SaveIntent) (workflow.SaveOutcome, error) {
	if intent.DefinitionID == "" || intent.Revision.RevisionRef == "" ||
		intent.OperationID == "" || intent.IdempotencyKey == "" ||
		intent.Fingerprint == "" || intent.CorrelationID == "" {
		return workflow.SaveOutcome{}, errStoragef(errs.CodeValidationFailed, "workflow save intent lacks its identity members")
	}
	var outcome workflow.SaveOutcome
	err := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
		def, err := loadWorkflowDefinitionTx(tx, intent.DefinitionID)
		if err != nil {
			// The destination identity resolves INSIDE the transaction:
			// the unknown definition is the SAME typed 404 the inspect read
			// serves (the no-leak rule), and the key stays unconsumed by an
			// unresolvable intent (the memory-mutation precedent).
			return err
		}
		// The M-21 replay ruling precedes the concurrent-revision check so
		// a lost save response's keyed retry reconciles (the S0–S4 law)
		// instead of surfacing the conflict.
		prior, priorFingerprint, err := workflowOpByKeyTx(tx, intent.IdempotencyKey)
		if err != nil {
			return err
		}
		if prior != nil {
			return s.resolveWorkflowSaveReplay(tx, prior, priorFingerprint, intent, &outcome)
		}
		// The concurrent-revision outcome: the client's known base must be
		// the current stored revision reference (an empty base is the
		// client's known base for a definition with no revisions).
		if intent.BaseRevisionRef != def.CurrentRevisionRef {
			return &WorkflowRevisionConflictError{
				CurrentRevisionRef: def.CurrentRevisionRef,
				Detail: fmt.Sprintf("the draft's base revision %q is not the current stored revision %q; the interim conflict rule governs — no overwrite, no merge, both sides preserved",
					intent.BaseRevisionRef, def.CurrentRevisionRef),
			}
		}
		now := s.now()
		rev := workflow.Revision{RevisionRef: intent.Revision.RevisionRef, CreatedAt: now.UTC().Format(time.RFC3339Nano)}
		if err := insertWorkflowRevision(tx, intent.DefinitionID, rev); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE workflow_definitions
			SET current_revision_ref = ?, updated_at = ? WHERE definition_id = ?`,
			rev.RevisionRef, now.UTC().Format(time.RFC3339Nano), intent.DefinitionID); err != nil {
			return mapVendorError(err)
		}
		if err := insertWorkflowOperation(tx, workflowOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          workflowKindSaveRevision,
			CorrelationID: intent.CorrelationID,
			DefinitionID:  intent.DefinitionID,
			RevisionRef:   rev.RevisionRef,
		}, intent.IdempotencyKey, intent.Fingerprint, now); err != nil {
			return err
		}
		// The register's revision.created event: each NEW immutable
		// revision's creation (the aggregate is the revision itself; the
		// payload carries references only — the redaction law).
		if err := appendEventTx(tx, workflowEvent(
			"evt-"+intent.OperationID,
			workflow.AggregateWorkflowRevision, rev.RevisionRef,
			workflow.EventRevisionCreated, intent.CorrelationID, s.instanceUUID,
			map[string]string{"definition_id": intent.DefinitionID, "revision_ref": rev.RevisionRef})); err != nil {
			return err
		}
		outcome = workflow.SaveOutcome{Revision: rev, OperationID: intent.OperationID, Replayed: false}
		return nil
	})
	if err == errWorkflowDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		var replay workflow.SaveOutcome
		rerr := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
			prior, priorFingerprint, lookupErr := workflowOpByKeyTx(tx, intent.IdempotencyKey)
			if lookupErr != nil || prior == nil {
				return err
			}
			return s.resolveWorkflowSaveReplay(tx, prior, priorFingerprint, intent, &replay)
		})
		if rerr != nil {
			return workflow.SaveOutcome{}, rerr
		}
		return replay, nil
	}
	if err != nil {
		return workflow.SaveOutcome{}, err
	}
	return outcome, nil
}

// resolveWorkflowSaveReplay applies the M-21 replay law: the same key +
// the same correlation + the same prepared-against base revision replays
// the recorded save within the 1 h post-terminal window; the same key with
// a different correlation is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); the same key prepared against a different base
// — or a replay past the window — is the typed CONFLICT_STATE.
func (s *Store) resolveWorkflowSaveReplay(tx *sqlTx, prior *workflowOperationRecord, priorFingerprint string, intent workflow.SaveIntent, outcome *workflow.SaveOutcome) error {
	if prior.CorrelationID != intent.CorrelationID {
		return &WorkflowIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity (recorded correlation %q); replay rejected", prior.CorrelationID),
		}
	}
	recordedBase := workflow.FingerprintBase(priorFingerprint)
	if recordedBase != intent.BaseRevisionRef {
		return errStoragef(errs.CodeConflictState,
			"workflow save was recorded against a different base revision (%q); replay rejected", recordedBase)
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > WorkflowSaveReplayWindow {
		return errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", WorkflowSaveReplayWindow)
	}
	rev, err := loadWorkflowRevisionTx(tx, intent.DefinitionID, prior.RevisionRef)
	if err != nil {
		// The recorded revision is immutable and reachable; an
		// unresolvable recorded revision is a corrupt-state refusal —
		// never a fabricated second revision.
		return err
	}
	*outcome = workflow.SaveOutcome{Revision: rev, OperationID: prior.OperationID, Replayed: true}
	return nil
}

// LoadWorkflowDefinition resolves one definition's authoritative identity
// record (a pure read): the unknown identity and the out-of-scope identity
// are the SAME typed 404 (the no-leak rule; no Workflow not-found domain
// code exists in the published register — the envelope register's
// NOT_FOUND rides, the B2 operation-record precedent).
func (s *Store) LoadWorkflowDefinition(ctx stdcontext.Context, definitionID string) (workflow.Definition, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT definition_id, display_name, current_revision_ref, created_at, updated_at
FROM workflow_definitions WHERE definition_id = ?`, definitionID)
	return scanWorkflowDefinition(row)
}

// ListWorkflowDefinitions reads one bounded keyset page (created_at DESC,
// definition_id DESC — the newest first; the keyset list semantics; the
// cursor's absence marks the final page).
func (s *Store) ListWorkflowDefinitions(ctx stdcontext.Context, q workflow.ListQuery) (workflow.DefinitionPage, error) {
	conds := []string{"1=1"}
	args := []any{}
	if q.AfterCreatedAt != "" {
		conds = append(conds, "(created_at < ? OR (created_at = ? AND definition_id < ?))")
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.readers.QueryContext(ctx, `
SELECT definition_id, display_name, current_revision_ref, created_at, updated_at
FROM workflow_definitions WHERE `+strings.Join(conds, " AND ")+`
ORDER BY created_at DESC, definition_id DESC LIMIT ?`, args...)
	if err != nil {
		return workflow.DefinitionPage{}, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := workflow.DefinitionPage{Definitions: []workflow.Definition{}}
	for rows.Next() {
		var def workflow.Definition
		if err := rows.Scan(&def.DefinitionID, &def.DisplayName, &def.CurrentRevisionRef, &def.CreatedAt, &def.UpdatedAt); err != nil {
			return workflow.DefinitionPage{}, mapVendorError(err)
		}
		out.Definitions = append(out.Definitions, def)
	}
	if err := rows.Err(); err != nil {
		return workflow.DefinitionPage{}, mapVendorError(err)
	}
	if len(out.Definitions) > q.Limit {
		out.More = true
		out.Definitions = out.Definitions[:q.Limit]
	}
	return out, nil
}

// ListWorkflowRevisions reads one definition's revision records (the
// inspect member's revisions — each immutable and reachable; the history
// order: created_at ASC, revision_ref ASC). The unknown definition is the
// SAME typed 404 as the definition read (the no-leak rule). The revision
// rows are never updated or removed (the no-update-in-place law).
func (s *Store) ListWorkflowRevisions(ctx stdcontext.Context, definitionID string) ([]workflow.Revision, error) {
	if _, err := s.LoadWorkflowDefinition(ctx, definitionID); err != nil {
		return nil, err
	}
	rows, err := s.readers.QueryContext(ctx, `
SELECT revision_ref, created_at FROM workflow_revisions
WHERE definition_id = ? ORDER BY created_at ASC, revision_ref ASC`, definitionID)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []workflow.Revision{}
	for rows.Next() {
		var rev workflow.Revision
		if err := rows.Scan(&rev.RevisionRef, &rev.CreatedAt); err != nil {
			return nil, mapVendorError(err)
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, mapVendorError(err)
	}
	return out, nil
}

// ListWorkflowRunHistory reads one definition's run-history LINKAGE
// records (a pure read; the LINKAGE record, not a Run schema). THE D-40
// GATE (carried exactly): the read's only producer (the Run execution) is
// GATED — no run-execution operation exists anywhere, so the table backs
// the read with zero rows today and every existing definition answers the
// honest empty page (the machinery for the day the mapping is contracted;
// never a fabricated record). The unknown definition is the SAME typed 404
// as the definition read (the no-leak rule).
func (s *Store) ListWorkflowRunHistory(ctx stdcontext.Context, definitionID string) ([]workflow.RunHistoryEntry, error) {
	if _, err := s.LoadWorkflowDefinition(ctx, definitionID); err != nil {
		return nil, err
	}
	rows, err := s.readers.QueryContext(ctx, `
SELECT run_ref, source_revision, linkage_state, created_at FROM workflow_run_history
WHERE definition_id = ? ORDER BY created_at ASC, run_ref ASC`, definitionID)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []workflow.RunHistoryEntry{}
	for rows.Next() {
		var e workflow.RunHistoryEntry
		if err := rows.Scan(&e.RunRef, &e.SourceRevision, &e.LinkageState, &e.CreatedAt); err != nil {
			return nil, mapVendorError(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, mapVendorError(err)
	}
	return out, nil
}

// LoadWorkflowDefinitionState resolves the family's authoritative state
// read (a pure query — never a retry; the reconcile anchor after a lost
// write response). Without the optional revision query the definition's
// current state is served; where the revision query rides, the state of
// that specific revision is served (the revision must exist AND belong to
// the stated definition — otherwise the SAME typed 404, the no-leak rule).
func (s *Store) LoadWorkflowDefinitionState(ctx stdcontext.Context, definitionID, revisionID string) (workflow.DefinitionState, error) {
	def, err := s.LoadWorkflowDefinition(ctx, definitionID)
	if err != nil {
		return workflow.DefinitionState{}, err
	}
	if revisionID == "" {
		return workflow.DefinitionState{
			DefinitionID:       def.DefinitionID,
			CurrentRevisionRef: def.CurrentRevisionRef,
			UpdatedAt:          def.UpdatedAt,
		}, nil
	}
	rev, err := s.loadWorkflowRevision(ctx, definitionID, revisionID)
	if err != nil {
		return workflow.DefinitionState{}, err
	}
	return workflow.DefinitionState{
		DefinitionID:       def.DefinitionID,
		CurrentRevisionRef: rev.RevisionRef,
		UpdatedAt:          rev.CreatedAt,
	}, nil
}

func (s *Store) loadWorkflowRevision(ctx stdcontext.Context, definitionID, revisionID string) (workflow.Revision, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT revision_ref, created_at FROM workflow_revisions
WHERE definition_id = ? AND revision_ref = ?`, definitionID, revisionID)
	return scanWorkflowRevision(row)
}

// workflowEvent builds one F10-shaped journal record: the frozen field set
// with a bounded inline payload of REFERENCES only (the redaction law —
// never draft content, never secret values).
func workflowEvent(eventID, aggregateType, aggregateID, eventType, correlationID, producer string, payload map[string]string) events.Record {
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

func scanWorkflowDefinition(row *sql.Row) (workflow.Definition, error) {
	var def workflow.Definition
	err := row.Scan(&def.DefinitionID, &def.DisplayName, &def.CurrentRevisionRef, &def.CreatedAt, &def.UpdatedAt)
	if err == sql.ErrNoRows {
		return workflow.Definition{}, errStoragef(errs.CodeNotFound, "no such workflow definition")
	}
	if err != nil {
		return workflow.Definition{}, mapVendorError(err)
	}
	return def, nil
}

func scanWorkflowRevision(row *sql.Row) (workflow.Revision, error) {
	var rev workflow.Revision
	err := row.Scan(&rev.RevisionRef, &rev.CreatedAt)
	if err == sql.ErrNoRows {
		return workflow.Revision{}, errStoragef(errs.CodeNotFound, "no such workflow revision")
	}
	if err != nil {
		return workflow.Revision{}, mapVendorError(err)
	}
	return rev, nil
}

func loadWorkflowDefinitionTx(tx *sqlTx, definitionID string) (workflow.Definition, error) {
	row := tx.QueryRow(`SELECT definition_id, display_name, current_revision_ref, created_at, updated_at
FROM workflow_definitions WHERE definition_id = ?`, definitionID)
	return scanWorkflowDefinition(row)
}

func loadWorkflowRevisionTx(tx *sqlTx, definitionID, revisionRef string) (workflow.Revision, error) {
	row := tx.QueryRow(`SELECT revision_ref, created_at FROM workflow_revisions
WHERE definition_id = ? AND revision_ref = ?`, definitionID, revisionRef)
	return scanWorkflowRevision(row)
}

func insertWorkflowDefinition(tx *sqlTx, def workflow.Definition, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO workflow_definitions
		(definition_id, display_name, current_revision_ref, created_at, updated_at)
		VALUES (?, ?, '', ?, ?)`,
		def.DefinitionID, def.DisplayName, nowNano, nowNano); err != nil {
		return mapVendorError(err)
	}
	return nil
}

func insertWorkflowRevision(tx *sqlTx, definitionID string, rev workflow.Revision) error {
	if _, err := tx.Exec(`INSERT INTO workflow_revisions
		(definition_id, revision_ref, created_at) VALUES (?, ?, ?)`,
		definitionID, rev.RevisionRef, rev.CreatedAt); err != nil {
		return mapVendorError(err)
	}
	return nil
}

func insertWorkflowOperation(tx *sqlTx, rec workflowOperationRecord, idempotencyKey, fingerprint string, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	terminal := nowNano
	_, err := tx.Exec(`INSERT INTO workflow_operations
		(operation_id, kind, state, correlation_id, definition_id, revision_ref,
		 failure_code, failure_reason, idempotency_key, fingerprint, terminal_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.OperationID, rec.Kind, "recorded", rec.CorrelationID, rec.DefinitionID, rec.RevisionRef,
		"", "", idempotencyKey, fingerprint, terminal, nowNano, nowNano)
	if err != nil {
		var serr sqlite3.Error
		if errors.As(err, &serr) && serr.Code == sqlite3.ErrConstraint {
			return errWorkflowDuplicateKey
		}
		return mapVendorError(err)
	}
	return nil
}

// workflowOpByKey resolves the prior operation under an effective key (nil
// when none).
func (s *Store) workflowOpByKey(ctx stdcontext.Context, key string) (*workflowOperationRecord, error) {
	var rec workflowOperationRecord
	err := s.readers.QueryRowContext(ctx, `
SELECT operation_id, kind, correlation_id, definition_id, revision_ref, created_at, updated_at
FROM workflow_operations WHERE idempotency_key = ?`, key).
		Scan(&rec.OperationID, &rec.Kind, &rec.CorrelationID, &rec.DefinitionID, &rec.RevisionRef, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, mapVendorError(err)
	}
	return &rec, nil
}

func workflowOpByKeyTx(tx *sqlTx, key string) (*workflowOperationRecord, string, error) {
	var rec workflowOperationRecord
	var fingerprint string
	err := tx.QueryRow(`SELECT operation_id, kind, correlation_id, definition_id, revision_ref, fingerprint, created_at, updated_at
FROM workflow_operations WHERE idempotency_key = ?`, key).
		Scan(&rec.OperationID, &rec.Kind, &rec.CorrelationID, &rec.DefinitionID, &rec.RevisionRef, &fingerprint, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	return &rec, fingerprint, nil
}
