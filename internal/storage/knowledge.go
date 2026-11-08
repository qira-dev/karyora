package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/knowledge"
	"github.com/qira-dev/karyora/internal/events"
)

// The Store implements the B1 KnowledgePack family's durable surface (DE-0
// queue row 1): the M-9 digest-keyed idempotency ruling and the
// identity-create duplicate-create layer run transactionally under the
// single-writer gate; every authoritative state change commits with its
// journal/outbox rows in ONE transaction (CommitStateChange); the durable
// wall-clock values are stamped here. (pack_id, version_id) is immutable;
// an existing version is a conflict, never an overwrite.

// KnowledgeReplayWindow is the M-9 class's post-terminal replay window: the
// same 72 h post-terminal window the Job idempotency class carries (the
// F6-precedent window — the recorded constant is the one window the class
// cites; within it the same key + same SOURCE DIGEST replays the existing
// operation, past it the replay is the typed CONFLICT_STATE).
const KnowledgeReplayWindow = IdempotencyReplayWindow

// KnowledgeCreateReplayWindow is the identity-create duplicate-create
// window: within 1 h after a completed create's terminal transition, a
// re-issued same-intent create (the identity-create fingerprint: the
// create-intent class + target pack identity + the SOURCE DIGEST) returns
// the existing pack-version reference; past it a repeated create intent is
// the caller's explicit new operation.
const KnowledgeCreateReplayWindow = time.Hour

// KnowledgeIdempotencyConflictError carries the M-9 conflict's envelope: the
// published CONFLICT_IDEMPOTENCY_KEY with its fingerprintMatch:false datum
// (the same typed shape the connection-record create conflict rides).
type KnowledgeIdempotencyConflictError struct{ Detail string }

func (e *KnowledgeIdempotencyConflictError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", errs.CodeConflictIdempotencyKey, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *KnowledgeIdempotencyConflictError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeConflictIdempotencyKey, correlationID, map[string]any{
		"keyScope":         "knowledge.import",
		"fingerprintMatch": false,
		"reason":           e.Detail,
	})
}

// errKnowledgeDuplicateKey marks the in-transaction same-key insert race
// that resolves to the replay ruling (the SubmitJob duplicate idiom).
var errKnowledgeDuplicateKey = errors.New("storage: duplicate knowledge idempotency key")

// SubmitKnowledgeImport applies the M-9 ruling and stores the fresh
// operation with the import.submitted event when no replay resolves.
func (s *Store) SubmitKnowledgeImport(ctx context.Context, intent knowledge.ImportIntent) (knowledge.Operation, bool, error) {
	op := intent.Operation
	if op.OperationID == "" || op.Kind != knowledge.KindImport || op.State != knowledge.StateSubmitted || op.CorrelationID == "" {
		return knowledge.Operation{}, false, errStoragef(errs.CodeValidationFailed, "knowledge import intent lacks its identity members")
	}
	if intent.IdempotencyKey == "" || intent.SourceDigest == "" {
		return knowledge.Operation{}, false, errStoragef(errs.CodeValidationFailed, "knowledge import intent lacks its M-9 keying members")
	}
	// The M-9 ruling: one operation per idempotency key, forever.
	if prior, digest, err := s.knowledgeOpByKey(ctx, intent.IdempotencyKey); err != nil {
		return knowledge.Operation{}, false, err
	} else if prior != nil {
		return s.resolveKnowledgeReplay(prior, digest, intent)
	}
	// The identity-create duplicate-create layer: a fresh key does not
	// escape the create fingerprint — a completed create with the same
	// intent fingerprint inside its window replays the existing
	// pack-version reference instead of duplicating the create.
	if matched, err := s.knowledgeCreateIntentMatch(ctx, intent); err != nil {
		return knowledge.Operation{}, false, err
	} else if matched != nil {
		return *matched, true, nil
	}
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		if err := insertKnowledgeOperation(tx, op, intent.IdempotencyKey, intent.SourceDigest, intent.CreateIntent, s.now()); err != nil {
			return err
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+op.OperationID+"-submitted",
			knowledge.AggregateKnowledgeImportOperation, op.OperationID,
			knowledge.EventImportSubmitted, op.CorrelationID, s.instanceUUID,
			map[string]string{"idempotency_key_present": "true", "source_digest": intent.SourceDigest}))
	})
	if err == errKnowledgeDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		prior, digest, lookupErr := s.knowledgeOpByKey(ctx, intent.IdempotencyKey)
		if lookupErr != nil || prior == nil {
			return knowledge.Operation{}, false, err
		}
		return s.resolveKnowledgeReplay(prior, digest, intent)
	}
	if err != nil {
		return knowledge.Operation{}, false, err
	}
	fresh, err := s.LoadKnowledgeOperation(ctx, op.OperationID)
	if err != nil {
		return knowledge.Operation{}, false, err
	}
	return fresh, false, nil
}

// resolveKnowledgeReplay applies the M-9 class exactly: same key + same
// SOURCE DIGEST replays while live or within the 72 h post-terminal window;
// same key + different bytes is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); a replay past the window is the typed
// CONFLICT_STATE.
func (s *Store) resolveKnowledgeReplay(prior *knowledge.Operation, priorDigest string, intent knowledge.ImportIntent) (knowledge.Operation, bool, error) {
	if priorDigest != intent.SourceDigest {
		return knowledge.Operation{}, false, &KnowledgeIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key %q was already used with different submitted bytes; the M-9 fingerprint does not match", intent.IdempotencyKey),
		}
	}
	if !knowledge.Terminal(prior.State) {
		return *prior, true, nil
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return knowledge.Operation{}, false, errStoragef(errs.CodeStateCorrupt, "terminal transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > KnowledgeReplayWindow {
		return knowledge.Operation{}, false, errStoragef(errs.CodeConflictState,
			"idempotency key %q replay window has elapsed (%s after the operation reached a terminal state); replay rejected",
			intent.IdempotencyKey, KnowledgeReplayWindow)
	}
	return *prior, true, nil
}

// knowledgeCreateIntentMatch resolves the identity-create layer: the most
// recent COMPLETED create with the same fingerprint, inside its 1 h window.
func (s *Store) knowledgeCreateIntentMatch(ctx context.Context, intent knowledge.ImportIntent) (*knowledge.Operation, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT operation_id FROM knowledge_operations
WHERE create_intent = ? AND source_digest = ? AND state = ? AND terminal_at != ''
ORDER BY terminal_at DESC LIMIT 1`,
		intent.CreateIntent, intent.SourceDigest, knowledge.StateCompleted)
	var opID string
	if err := row.Scan(&opID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, mapVendorError(err)
	}
	matched, err := s.LoadKnowledgeOperation(ctx, opID)
	if err != nil {
		return nil, err
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, matched.UpdatedAt)
	if perr != nil {
		return nil, errStoragef(errs.CodeStateCorrupt, "terminal transition of operation %q does not parse", matched.OperationID)
	}
	if s.now().Sub(terminalAt) > KnowledgeCreateReplayWindow {
		return nil, nil // past the window: the caller's explicit new operation
	}
	return &matched, nil
}

// SubmitKnowledgeExport stores the fresh export operation with the
// export.submitted event. No idempotency layer exists (none required).
func (s *Store) SubmitKnowledgeExport(ctx context.Context, op knowledge.Operation) (knowledge.Operation, error) {
	if op.OperationID == "" || op.Kind != knowledge.KindExport || op.State != knowledge.StateSubmitted || op.CorrelationID == "" {
		return knowledge.Operation{}, errStoragef(errs.CodeValidationFailed, "knowledge export operation lacks its identity members")
	}
	err := s.CommitStateChange(ctx, op.CorrelationID, func(tx *sqlTx) error {
		if err := insertKnowledgeOperation(tx, op, "", "", "", s.now()); err != nil {
			return err
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+op.OperationID+"-submitted",
			knowledge.AggregateKnowledgeExportOperation, op.OperationID,
			knowledge.EventExportSubmitted, op.CorrelationID, s.instanceUUID,
			map[string]string{"correlation_id": op.CorrelationID}))
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, op.OperationID)
}

// KnowledgeOperationStage commits one import stage transition and its
// import.stage_entered event (the stage vocabulary from actual Core stages
// only). A stage on a terminal or unknown operation is refused.
func (s *Store) KnowledgeOperationStage(ctx context.Context, opID, correlationID, state string) (knowledge.Operation, error) {
	if !knowledge.ValidOperationState(state) || knowledge.Terminal(state) {
		return knowledge.Operation{}, errStoragef(errs.CodeValidationFailed, "stage state %q is not a live knowledge operation state", state)
	}
	err := s.CommitStateChange(ctx, correlationID, func(tx *sqlTx) error {
		res, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, updated_at = ?
			WHERE operation_id = ? AND kind = ? AND terminal_at = ''`,
			state, s.now().UTC().Format(time.RFC3339Nano), opID, knowledge.KindImport)
		if err != nil {
			return mapVendorError(err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return errStoragef(errs.CodeConflictState, "operation %q cannot stage to %q (unknown or already terminal)", opID, state)
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+opID+"-stage-"+state,
			knowledge.AggregateKnowledgeImportOperation, opID,
			knowledge.EventImportStageEntered, correlationID, s.instanceUUID,
			map[string]string{"stage": state}))
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, opID)
}

// CompleteKnowledgeImport commits the terminal completed state, the result,
// the pack/version/entry rows, and the family's events in ONE transaction.
func (s *Store) CompleteKnowledgeImport(ctx context.Context, commit knowledge.ImportCommit) (knowledge.Operation, error) {
	if commit.OperationID == "" || commit.CorrelationID == "" || commit.Pack.PackID == "" || commit.Version.VersionID == "" || len(commit.Entries) == 0 {
		return knowledge.Operation{}, errStoragef(errs.CodeValidationFailed, "knowledge import commit lacks its identity members")
	}
	now := s.now().UTC()
	err := s.CommitStateChange(ctx, commit.CorrelationID, func(tx *sqlTx) error {
		if commit.PackIsNew {
			if _, err := tx.Exec(`INSERT INTO knowledge_packs
				(pack_id, schema_version, title, description, created_at, updated_at, current_version_id)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				commit.Pack.PackID, commit.Pack.SchemaVersion, commit.Pack.Title, commit.Pack.Description,
				commit.Pack.CreatedAt, commit.Pack.UpdatedAt, commit.Version.VersionID); err != nil {
				return mapVendorError(err)
			}
		} else {
			if _, err := tx.Exec(`UPDATE knowledge_packs SET current_version_id = ?, updated_at = ? WHERE pack_id = ?`,
				commit.Version.VersionID, now.Format(time.RFC3339Nano), commit.Pack.PackID); err != nil {
				return mapVendorError(err)
			}
		}
		supersedes := "[]"
		if b, err := json.Marshal(commit.Version.Supersedes); err == nil && commit.Version.Supersedes != nil {
			supersedes = string(b)
		}
		if _, err := tx.Exec(`INSERT INTO knowledge_pack_versions
			(pack_id, version_id, schema_version, pack_root_digest, entry_count, claim_count, evidence_count,
			 supersedes, superseded_by, license, license_status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			commit.Version.PackID, commit.Version.VersionID, commit.Version.SchemaVersion, commit.Version.PackRootDigest,
			commit.Version.EntryCount, commit.Version.ClaimCount, commit.Version.EvidenceCount,
			supersedes, commit.Version.SupersededBy, commit.Version.License, commit.Version.LicenseStatus,
			commit.Version.CreatedAt); err != nil {
			return mapVendorError(err)
		}
		for i := range commit.Entries {
			e := &commit.Entries[i]
			if _, err := tx.Exec(`INSERT INTO knowledge_pack_entries
				(pack_id, version_id, entry_path, content_type, size_bytes, digest, content)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				commit.Version.PackID, commit.Version.VersionID, e.Path, e.ContentType, e.SizeBytes, e.Digest, e.Content); err != nil {
				return mapVendorError(err)
			}
		}
		resultJSON, err := json.Marshal(commit.Result)
		if err != nil {
			return errStoragef(errs.CodeValidationFailed, "result does not marshal: %v", err)
		}
		if _, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, result = ?, terminal_at = ?, updated_at = ?
			WHERE operation_id = ? AND terminal_at = ''`,
			knowledge.StateCompleted, resultJSON, s.now().UTC().Format(time.RFC3339Nano), s.now().UTC().Format(time.RFC3339Nano), commit.OperationID); err != nil {
			return mapVendorError(err)
		}
		if commit.PackIsNew {
			if err := appendEventTx(tx, knowledgeEvent(
				"evt-"+commit.Pack.PackID+"-created",
				knowledge.AggregateKnowledgePack, commit.Pack.PackID,
				knowledge.EventPackCreated, commit.CorrelationID, s.instanceUUID,
				map[string]string{"pack_id": commit.Pack.PackID})); err != nil {
				return err
			}
		}
		if err := appendEventTx(tx, knowledgeEvent(
			"evt-"+commit.Version.VersionID+"-created",
			knowledge.AggregateKnowledgePackVersion, commit.Version.VersionID,
			knowledge.EventPackVersionCreated, commit.CorrelationID, s.instanceUUID,
			map[string]string{"pack_id": commit.Version.PackID, "version_id": commit.Version.VersionID})); err != nil {
			return err
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+commit.OperationID+"-completed",
			knowledge.AggregateKnowledgeImportOperation, commit.OperationID,
			knowledge.EventImportCompleted, commit.CorrelationID, s.instanceUUID,
			map[string]string{"pack_id": commit.Version.PackID, "version_id": commit.Version.VersionID}))
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, commit.OperationID)
}

// RejectKnowledgeImport commits the terminal rejected state with the
// explicit failure code and reason + the import.rejected event.
func (s *Store) RejectKnowledgeImport(ctx context.Context, opID, correlationID, failureCode, reason string) (knowledge.Operation, error) {
	now := s.now().UTC().Format(time.RFC3339Nano)
	err := s.CommitStateChange(ctx, correlationID, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, failure_code = ?, failure_reason = ?, terminal_at = ?, updated_at = ?
			WHERE operation_id = ? AND terminal_at = ''`,
			knowledge.StateRejected, failureCode, reason, now, now, opID); err != nil {
			return mapVendorError(err)
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+opID+"-rejected",
			knowledge.AggregateKnowledgeImportOperation, opID,
			knowledge.EventImportRejected, correlationID, s.instanceUUID,
			map[string]string{"failure_code": failureCode}))
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, opID)
}

// FailKnowledgeOperation commits the terminal failed state with the explicit
// failure code and reason. The closed event register carries export.failed
// but no import.failed type — a failed import emits no event (events are
// never the source of truth; the state endpoint is).
func (s *Store) FailKnowledgeOperation(ctx context.Context, opID, correlationID, failureCode, reason string) (knowledge.Operation, error) {
	now := s.now().UTC().Format(time.RFC3339Nano)
	var kind string
	if err := s.readers.QueryRowContext(ctx, `SELECT kind FROM knowledge_operations WHERE operation_id = ?`, opID).Scan(&kind); err != nil {
		if err == sql.ErrNoRows {
			return knowledge.Operation{}, errStoragef(errs.CodeNotFound, "operation %q is not stored", opID)
		}
		return knowledge.Operation{}, mapVendorError(err)
	}
	err := s.CommitStateChange(ctx, correlationID, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, failure_code = ?, failure_reason = ?, terminal_at = ?, updated_at = ?
			WHERE operation_id = ? AND terminal_at = ''`,
			knowledge.StateFailed, failureCode, reason, now, now, opID); err != nil {
			return mapVendorError(err)
		}
		if kind == knowledge.KindExport {
			return appendEventTx(tx, knowledgeEvent(
				"evt-"+opID+"-failed",
				knowledge.AggregateKnowledgeExportOperation, opID,
				knowledge.EventExportFailed, correlationID, s.instanceUUID,
				map[string]string{"failure_code": failureCode}))
		}
		return nil
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, opID)
}

// CompleteKnowledgeExport CAS-publishes the artifact, then commits the
// terminal completed state, the result, and the export.completed event in
// ONE transaction. The CAS registration carries the owner reference so the
// artifact is GC-barred while the operation record references it.
func (s *Store) CompleteKnowledgeExport(ctx context.Context, commit knowledge.ExportCommit) (knowledge.Operation, error) {
	if commit.OperationID == "" || commit.CorrelationID == "" || len(commit.Artifact) == 0 {
		return knowledge.Operation{}, errStoragef(errs.CodeValidationFailed, "knowledge export commit lacks its identity members")
	}
	digest, err := s.PutObject(ctx, commit.Artifact, "knowledge.export", commit.OperationID)
	if err != nil {
		return knowledge.Operation{}, err
	}
	err = s.CommitStateChange(ctx, commit.CorrelationID, func(tx *sqlTx) error {
		result := commit.Result
		if result.ExportRef == "" {
			result.ExportRef = digest
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return errStoragef(errs.CodeValidationFailed, "result does not marshal: %v", err)
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, result = ?, terminal_at = ?, updated_at = ?
			WHERE operation_id = ? AND terminal_at = ''`,
			knowledge.StateCompleted, resultJSON, now, now, commit.OperationID); err != nil {
			return mapVendorError(err)
		}
		return appendEventTx(tx, knowledgeEvent(
			"evt-"+commit.OperationID+"-completed",
			knowledge.AggregateKnowledgeExportOperation, commit.OperationID,
			knowledge.EventExportCompleted, commit.CorrelationID, s.instanceUUID,
			map[string]string{"export_ref": result.ExportRef}))
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, commit.OperationID)
}

// CancelKnowledgeOperation applies the cancellation ruling: unknown → typed
// NOT_FOUND; terminal → typed CONFLICT_STATE (the Core-confirmed state
// decides — post-boundary operations are non-cancellable); live → the
// Core-confirmed cancelled state + the import.cancelled event (imports
// only — the register carries no export.cancelled type).
func (s *Store) CancelKnowledgeOperation(ctx context.Context, opID, correlationID string) (knowledge.Operation, error) {
	op, err := s.LoadKnowledgeOperation(ctx, opID)
	if err != nil {
		return knowledge.Operation{}, err
	}
	if knowledge.Terminal(op.State) || op.State == knowledge.StateOutcomeUnknown {
		return knowledge.Operation{}, errStoragef(errs.CodeConflictState,
			"operation %q is %s; cancellation is refused past the boundary", opID, op.State)
	}
	err = s.CommitStateChange(ctx, correlationID, func(tx *sqlTx) error {
		now := s.now().UTC().Format(time.RFC3339Nano)
		res, err := tx.Exec(`UPDATE knowledge_operations SET state = ?, terminal_at = ?, updated_at = ?
			WHERE operation_id = ? AND terminal_at = ''`,
			knowledge.StateCancelled, now, now, opID)
		if err != nil {
			return mapVendorError(err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return errStoragef(errs.CodeConflictState, "operation %q reached a terminal state; cancellation refused", opID)
		}
		if op.Kind == knowledge.KindImport {
			return appendEventTx(tx, knowledgeEvent(
				"evt-"+opID+"-cancelled",
				knowledge.AggregateKnowledgeImportOperation, opID,
				knowledge.EventImportCancelled, correlationID, s.instanceUUID,
				map[string]string{"cancelled_from": op.State}))
		}
		return nil
	})
	if err != nil {
		return knowledge.Operation{}, err
	}
	return s.LoadKnowledgeOperation(ctx, opID)
}

// LoadKnowledgeOperation resolves the authoritative state read; an unknown
// operation is the typed NOT_FOUND (the no-leak rule).
func (s *Store) LoadKnowledgeOperation(ctx context.Context, opID string) (knowledge.Operation, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT kind, state, correlation_id, failure_code, failure_reason, untrusted_label, result, created_at, updated_at
FROM knowledge_operations WHERE operation_id = ?`, opID)
	var op knowledge.Operation
	var result []byte
	var createdAt, updatedAt string
	err := row.Scan(&op.Kind, &op.State, &op.CorrelationID, &op.FailureCode, &op.FailureReason,
		&op.UntrustedLabel, &result, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return knowledge.Operation{}, errStoragef(errs.CodeNotFound, "no such knowledge operation")
	}
	if err != nil {
		return knowledge.Operation{}, mapVendorError(err)
	}
	op.OperationID = opID
	if len(result) > 0 {
		var res knowledge.OperationResult
		if err := json.Unmarshal(result, &res); err != nil {
			return knowledge.Operation{}, errStoragef(errs.CodeStateCorrupt, "stored result of operation %q does not decode", opID)
		}
		op.Result = &res
	}
	op.CreatedAt, op.UpdatedAt = createdAt, updatedAt
	return op, nil
}

// knowledgeOpByKey resolves the prior operation under one idempotency key
// with its recorded SOURCE DIGEST (nil op when none).
func (s *Store) knowledgeOpByKey(ctx context.Context, key string) (*knowledge.Operation, string, error) {
	var opID, digest string
	err := s.readers.QueryRowContext(ctx,
		`SELECT operation_id, source_digest FROM knowledge_operations WHERE idempotency_key = ?`, key).Scan(&opID, &digest)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	op, err := s.LoadKnowledgeOperation(ctx, opID)
	if err != nil {
		return nil, "", err
	}
	return &op, digest, nil
}

// LoadKnowledgePack resolves one pack; unknown is the typed NOT_FOUND (the
// no-leak rule: unknown and out-of-scope the SAME typed 404).
func (s *Store) LoadKnowledgePack(ctx context.Context, packID string) (knowledge.Pack, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT schema_version, title, description, created_at, updated_at, current_version_id
FROM knowledge_packs WHERE pack_id = ?`, packID)
	var p knowledge.Pack
	var schemaVersion int64
	var description string
	err := row.Scan(&schemaVersion, &p.Title, &description, &p.CreatedAt, &p.UpdatedAt, &p.CurrentVersionID)
	if err == sql.ErrNoRows {
		return knowledge.Pack{}, errStoragef(errs.CodeNotFound, "no such knowledge pack")
	}
	if err != nil {
		return knowledge.Pack{}, mapVendorError(err)
	}
	p.PackID = packID
	p.SchemaVersion = int(schemaVersion)
	p.Description = description
	return p, nil
}

// LoadKnowledgeVersion resolves one immutable version; unknown pack and
// unknown version are the SAME typed NOT_FOUND (the no-leak rule).
func (s *Store) LoadKnowledgeVersion(ctx context.Context, packID, versionID string) (knowledge.PackVersion, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT schema_version, pack_root_digest, entry_count, claim_count, evidence_count, supersedes, superseded_by, license, license_status, created_at
FROM knowledge_pack_versions WHERE pack_id = ? AND version_id = ?`, packID, versionID)
	var v knowledge.PackVersion
	var schemaVersion int64
	var entryCount, claimCount, evidenceCount int64
	var supersedes, license, licenseStatus string
	err := row.Scan(&schemaVersion, &v.PackRootDigest, &entryCount, &claimCount, &evidenceCount,
		&supersedes, &v.SupersededBy, &license, &licenseStatus, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return knowledge.PackVersion{}, errStoragef(errs.CodeNotFound, "no such knowledge pack version")
	}
	if err != nil {
		return knowledge.PackVersion{}, mapVendorError(err)
	}
	v.PackID, v.VersionID = packID, versionID
	v.SchemaVersion = int(schemaVersion)
	v.EntryCount, v.ClaimCount, v.EvidenceCount = int(entryCount), int(claimCount), int(evidenceCount)
	if supersedes != "" && supersedes != "[]" {
		_ = json.Unmarshal([]byte(supersedes), &v.Supersedes)
	}
	v.License, v.LicenseStatus = license, licenseStatus
	return v, nil
}

// KnowledgeEntries loads one version's entries in path order.
func (s *Store) KnowledgeEntries(ctx context.Context, packID, versionID string) ([]knowledge.EntryRecord, error) {
	rows, err := s.readers.QueryContext(ctx, `
SELECT entry_path, content_type, size_bytes, digest, content
FROM knowledge_pack_entries WHERE pack_id = ? AND version_id = ? ORDER BY entry_path ASC`, packID, versionID)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []knowledge.EntryRecord{}
	for rows.Next() {
		var e knowledge.EntryRecord
		if err := rows.Scan(&e.Path, &e.ContentType, &e.SizeBytes, &e.Digest, &e.Content); err != nil {
			return nil, mapVendorError(err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListKnowledgePacks reads one bounded keyset page (created_at DESC,
// pack_id DESC — the newest first; the More flag drives the cursor).
func (s *Store) ListKnowledgePacks(ctx context.Context, q knowledge.ListQuery) (knowledge.PackPage, error) {
	rows, err := s.readers.QueryContext(ctx, `
SELECT pack_id, schema_version, title, description, created_at, updated_at, current_version_id
FROM knowledge_packs
WHERE (? = '' OR (created_at < ? OR (created_at = ? AND pack_id < ?)))
ORDER BY created_at DESC, pack_id DESC LIMIT ?`,
		q.AfterCreatedAt, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterID, q.Limit+1)
	if err != nil {
		return knowledge.PackPage{}, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	return scanKnowledgePackPage(rows, q.Limit)
}

// ListKnowledgeVersions reads one bounded keyset page within one pack.
func (s *Store) ListKnowledgeVersions(ctx context.Context, packID string, q knowledge.ListQuery) (knowledge.VersionPage, error) {
	rows, err := s.readers.QueryContext(ctx, `
SELECT version_id, schema_version, pack_root_digest, entry_count, claim_count, evidence_count, supersedes, superseded_by, license, license_status, created_at
FROM knowledge_pack_versions
WHERE pack_id = ? AND (? = '' OR (created_at < ? OR (created_at = ? AND version_id < ?)))
ORDER BY created_at DESC, version_id DESC LIMIT ?`,
		packID, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterID, q.Limit+1)
	if err != nil {
		return knowledge.VersionPage{}, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := knowledge.VersionPage{Versions: []knowledge.PackVersion{}}
	for rows.Next() {
		var v knowledge.PackVersion
		var schemaVersion int64
		var entryCount, claimCount, evidenceCount int64
		var supersedes, license, licenseStatus, createdAt string
		if err := rows.Scan(&v.VersionID, &schemaVersion, &v.PackRootDigest, &entryCount, &claimCount, &evidenceCount,
			&supersedes, &v.SupersededBy, &license, &licenseStatus, &createdAt); err != nil {
			return knowledge.VersionPage{}, mapVendorError(err)
		}
		v.PackID = packID
		v.SchemaVersion = int(schemaVersion)
		v.EntryCount, v.ClaimCount, v.EvidenceCount = int(entryCount), int(claimCount), int(evidenceCount)
		if supersedes != "" && supersedes != "[]" {
			_ = json.Unmarshal([]byte(supersedes), &v.Supersedes)
		}
		v.License, v.LicenseStatus, v.CreatedAt = license, licenseStatus, createdAt
		out.Versions = append(out.Versions, v)
	}
	if err := rows.Err(); err != nil {
		return knowledge.VersionPage{}, mapVendorError(err)
	}
	if len(out.Versions) > q.Limit {
		out.More = true
		out.Versions = out.Versions[:q.Limit]
	}
	return out, nil
}

func scanKnowledgePackPage(rows *sql.Rows, limit int) (knowledge.PackPage, error) {
	out := knowledge.PackPage{Packs: []knowledge.Pack{}}
	for rows.Next() {
		var p knowledge.Pack
		var schemaVersion int64
		var description string
		if err := rows.Scan(&p.PackID, &schemaVersion, &p.Title, &description, &p.CreatedAt, &p.UpdatedAt, &p.CurrentVersionID); err != nil {
			return knowledge.PackPage{}, mapVendorError(err)
		}
		p.SchemaVersion = int(schemaVersion)
		p.Description = description
		out.Packs = append(out.Packs, p)
	}
	if err := rows.Err(); err != nil {
		return knowledge.PackPage{}, mapVendorError(err)
	}
	if len(out.Packs) > limit {
		out.More = true
		out.Packs = out.Packs[:limit]
	}
	return out, nil
}

// insertKnowledgeOperation stamps and writes the fresh operation row.
func insertKnowledgeOperation(tx *sqlTx, op knowledge.Operation, idempotencyKey, sourceDigest, createIntent string, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	_, err := tx.Exec(`INSERT INTO knowledge_operations
		(operation_id, kind, state, correlation_id, failure_code, failure_reason, untrusted_label,
		 idempotency_key, source_digest, create_intent, result, terminal_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)`,
		op.OperationID, op.Kind, op.State, op.CorrelationID, op.FailureCode, op.FailureReason, op.UntrustedLabel,
		idempotencyKey, sourceDigest, createIntent, nowNano, nowNano)
	if err != nil {
		var serr sqlite3.Error
		if errors.As(err, &serr) && serr.Code == sqlite3.ErrConstraint {
			return errKnowledgeDuplicateKey
		}
		return mapVendorError(err)
	}
	return nil
}

// knowledgeEvent builds one F10-shaped journal record: the frozen field set
// with a bounded inline payload (the register's aggregate/type values ride
// verbatim).
func knowledgeEvent(eventID, aggregateType, aggregateID, eventType, correlationID, producer string, payload map[string]string) events.Record {
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
