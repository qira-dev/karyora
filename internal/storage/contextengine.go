package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The Store satisfies the context-engine persistence ports (C3-10/C3-11):
// snapshots are immutable and written once; memory queries are always
// scope-bound. The domain package owns the contracts; SQLite mechanics
// live here per the dependency map.
var (
	_ contextengine.SnapshotStore = (*Store)(nil)
	_ contextengine.MemoryStore   = (*Store)(nil)
)

// SaveSnapshot implements contextengine.SnapshotStore.
func (s *Store) SaveSnapshot(ctx context.Context, snap contextengine.ContextSnapshot) error {
	if err := snap.Validate(); err != nil {
		return errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		return errStoragef(errs.CodeValidationFailed, "snapshot does not marshal: %v", err)
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		var one int
		err := tx.QueryRow(`SELECT 1 FROM context_snapshots WHERE snapshot_id = ?`, snap.SnapshotID).Scan(&one)
		if err == nil {
			return errStoragef(errs.CodeConflictState, "snapshot %q already exists; snapshots are immutable", snap.SnapshotID)
		}
		if err != sql.ErrNoRows {
			return mapVendorError(err)
		}
		_, err = tx.Exec(`INSERT INTO context_snapshots
			(snapshot_id, scope, task_ref, schema_version, assembled_digest, snapshot_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snap.SnapshotID, snap.Scope, snap.TaskRef, snap.SchemaVersion, snap.AssembledDigest,
			snapJSON, snap.CreatedAt.UTC().Format(time.RFC3339Nano))
		return mapVendorError(err)
	})
}

// LoadSnapshot implements contextengine.SnapshotStore.
func (s *Store) LoadSnapshot(ctx context.Context, snapshotID string) (contextengine.ContextSnapshot, error) {
	var snapJSON []byte
	err := s.readers.QueryRowContext(ctx,
		`SELECT snapshot_json FROM context_snapshots WHERE snapshot_id = ?`, snapshotID).Scan(&snapJSON)
	if err == sql.ErrNoRows {
		return contextengine.ContextSnapshot{}, errStoragef(errs.CodeNotFound, "snapshot %q is not stored", snapshotID)
	}
	if err != nil {
		return contextengine.ContextSnapshot{}, mapVendorError(err)
	}
	var snap contextengine.ContextSnapshot
	if err := json.Unmarshal(snapJSON, &snap); err != nil {
		return contextengine.ContextSnapshot{}, errStoragef(errs.CodeStateCorrupt, "stored snapshot %q does not decode: %v", snapshotID, err)
	}
	if err := snap.Validate(); err != nil {
		return contextengine.ContextSnapshot{}, errStoragef(errs.CodeStateCorrupt, "stored snapshot %q fails validation: %v", snapshotID, err)
	}
	return snap, nil
}

// SaveMemory implements contextengine.MemoryStore.
func (s *Store) SaveMemory(ctx context.Context, record contextengine.MemoryRecord) error {
	if err := record.Validate(); err != nil {
		return errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	provJSON, err := json.Marshal(record.Provenance)
	if err != nil {
		return errStoragef(errs.CodeValidationFailed, "provenance does not marshal: %v", err)
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		var one int
		err := tx.QueryRow(`SELECT 1 FROM memory_records WHERE memory_id = ?`, record.MemoryID).Scan(&one)
		if err == nil {
			return errStoragef(errs.CodeConflictState, "memory %q already exists; corrections create new records", record.MemoryID)
		}
		if err != sql.ErrNoRows {
			return mapVendorError(err)
		}
		_, err = tx.Exec(`INSERT INTO memory_records
			(memory_id, scope, content, sensitivity, confidence, status, provenance_json,
			 correction_of, superseded_by, pinned, ttl_seconds, created_at, last_confirmed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.MemoryID, record.Scope, record.Content, record.Sensitivity, record.Confidence,
			record.Status, provJSON, record.CorrectionOf, record.SupersededBy, record.Pinned,
			record.TTLSeconds, record.CreatedAt.UTC().Format(time.RFC3339Nano),
			record.LastConfirmedAt.UTC().Format(time.RFC3339Nano))
		return mapVendorError(err)
	})
}

// LoadMemory implements contextengine.MemoryStore: a record from a
// different scope is NOT_FOUND, never returned (C3-11 scope isolation).
func (s *Store) LoadMemory(ctx context.Context, scope, memoryID string) (contextengine.MemoryRecord, error) {
	row := s.readers.QueryRowContext(ctx, `
		SELECT memory_id, scope, content, sensitivity, confidence, status, provenance_json,
		       correction_of, superseded_by, pinned, ttl_seconds, created_at, last_confirmed_at
		FROM memory_records WHERE scope = ? AND memory_id = ?`, scope, memoryID)
	rec, err := scanMemory(row)
	if err == sql.ErrNoRows {
		return contextengine.MemoryRecord{}, errStoragef(errs.CodeNotFound, "memory %q in scope %q is not stored", memoryID, scope)
	}
	return rec, err
}

// ListMemory implements contextengine.MemoryStore.
func (s *Store) ListMemory(ctx context.Context, scope string) ([]contextengine.MemoryRecord, error) {
	rows, err := s.readers.QueryContext(ctx, `
		SELECT memory_id, scope, content, sensitivity, confidence, status, provenance_json,
		       correction_of, superseded_by, pinned, ttl_seconds, created_at, last_confirmed_at
		FROM memory_records WHERE scope = ? ORDER BY created_at ASC, memory_id ASC`, scope)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := []contextengine.MemoryRecord{}
	for rows.Next() {
		rec, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// UpdateMemoryStatus implements contextengine.MemoryStore: only the visible
// lifecycle status moves; content and provenance are untouched.
func (s *Store) UpdateMemoryStatus(ctx context.Context, scope, memoryID, status, supersededBy string) error {
	switch status {
	case contextengine.MemoryActive, contextengine.MemoryCorrected, contextengine.MemoryExpired, contextengine.MemoryDeleted:
	default:
		return errStoragef(errs.CodeValidationFailed, "illegal memory status %q", status)
	}
	res, err := s.writers.ExecContext(ctx,
		`UPDATE memory_records SET status = ?, superseded_by = ? WHERE scope = ? AND memory_id = ?`,
		status, supersededBy, scope, memoryID)
	if err != nil {
		return mapVendorError(err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errStoragef(errs.CodeNotFound, "memory %q in scope %q is not stored", memoryID, scope)
	}
	return nil
}

// rowScanner abstracts *sql.Row and *sql.Rows for the shared scan.
type rowScanner interface{ Scan(dest ...any) error }

func scanMemory(row rowScanner) (contextengine.MemoryRecord, error) {
	var rec contextengine.MemoryRecord
	var provJSON []byte
	var pinned bool
	var createdAt, lastConfirmed string
	err := row.Scan(&rec.MemoryID, &rec.Scope, &rec.Content, &rec.Sensitivity, &rec.Confidence,
		&rec.Status, &provJSON, &rec.CorrectionOf, &rec.SupersededBy, &pinned, &rec.TTLSeconds,
		&createdAt, &lastConfirmed)
	if err != nil {
		return contextengine.MemoryRecord{}, err
	}
	rec.Pinned = pinned
	if err := json.Unmarshal(provJSON, &rec.Provenance); err != nil {
		return contextengine.MemoryRecord{}, errStoragef(errs.CodeStateCorrupt, "stored memory provenance does not decode: %v", err)
	}
	if rec.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return contextengine.MemoryRecord{}, errStoragef(errs.CodeStateCorrupt, "stored memory created_at does not parse: %v", err)
	}
	if rec.LastConfirmedAt, err = time.Parse(time.RFC3339Nano, lastConfirmed); err != nil {
		return contextengine.MemoryRecord{}, errStoragef(errs.CodeStateCorrupt, "stored memory last_confirmed_at does not parse: %v", err)
	}
	return rec, nil
}
