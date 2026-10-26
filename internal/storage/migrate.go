package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Migration states (C2-11 §3): a partially applied migration is never
// represented as current; only `applied` counts toward the schema version.
const (
	migStatePending    = "pending"
	migStateInProgress = "in_progress"
	migStateApplied    = "applied"
)

// Migration is one versioned schema change. Every migration declares its
// atomicity contract (C2-11 §3): Transactional applies inside one SQLite
// transaction; Resumable runs stepped batches with durable per-step
// progress and no unbounded batch.
type Migration struct {
	Version int64
	Name    string
	// SQL runs for a Transactional migration (SQLite DDL is transactional;
	// a crash rolls the whole migration back and the recorded state becomes
	// applied only after commit).
	SQL string
	// Destructive marks a migration that can make prior data unreadable
	// (C2-11 §4): it refuses to run without a verified recovery point.
	Destructive bool
}

// migrations is the ordered, built-in migration set — the single migration
// authority (C2-11 §1). The D-12/D-33 exclusion holds: no migration may
// introduce Work tables or fields (gate exclusion, C2-11 §7).
var migrations = []Migration{
	{
		Version: 1,
		Name:    "0001-initial-core-schema",
		SQL: `
CREATE TABLE IF NOT EXISTS core_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events_journal (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id   TEXT NOT NULL,
    sequence       INTEGER NOT NULL,
    event_type     TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    occurred_at    TEXT NOT NULL,
    causation_id   TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    producer       TEXT NOT NULL,
    payload        BLOB,
    payload_digest TEXT NOT NULL DEFAULT '',
    blob_ref       TEXT NOT NULL DEFAULT '',
    UNIQUE (aggregate_type, aggregate_id, sequence)
);

CREATE TABLE IF NOT EXISTS events_outbox (
    event_id        TEXT PRIMARY KEY REFERENCES events_journal(event_id) ON DELETE CASCADE,
    published       INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_journal_aggregate
    ON events_journal (aggregate_type, aggregate_id, sequence);

CREATE TABLE IF NOT EXISTS events_consumer_ledger (
    consumer    TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    delivered_at TEXT NOT NULL,
    PRIMARY KEY (consumer, event_id)
);

CREATE TABLE IF NOT EXISTS cas_references (
    blob_ref    TEXT NOT NULL,
    owner_type  TEXT NOT NULL,
    owner_id    TEXT NOT NULL,
    registered_at TEXT NOT NULL,
    PRIMARY KEY (blob_ref, owner_type, owner_id)
);

CREATE INDEX IF NOT EXISTS idx_cas_refs_owner ON cas_references (owner_type, owner_id);
`,
	},
}

// compatibility window (C2-11 §5): a binary of release N supports schema
// versions [N_min, N]. Phase 2 supports [1, 1]; the window is recorded per
// release and published floors remain a Phase 5/D-30 concern.
const (
	schemaVersionMin = 1
	schemaVersionMax = 1
)

// ensureMigrationAuthority bootstraps the schema_migrations table itself —
// the single migration authority; there is no second schema-version source
// (C2-11 §1). It runs under exclusive Core ownership (the runner's kernel
// lock) and the single-writer admission gate.
func (s *Store) ensureMigrationAuthority(ctx context.Context) error {
	_, err := s.writers.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version       INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    checksum      TEXT NOT NULL,
    state         TEXT NOT NULL,
    step          INTEGER NOT NULL DEFAULT 0,
    applied_at    TEXT NOT NULL,
    core_version  TEXT NOT NULL
)`)
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

func checksumOf(m Migration) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", m.Version, m.Name, m.SQL)))
	return hex.EncodeToString(sum[:])
}

// migrate runs the C2-11 sequence: tamper verification of applied records,
// newer-schema refusal, then ordered application with explicit atomicity.
// coreVersion attributes each applied migration.
func (s *Store) migrate(ctx context.Context, coreVersion string) error {
	if err := s.ensureMigrationAuthority(ctx); err != nil {
		return err
	}
	if err := s.verifyAppliedChecksums(ctx); err != nil {
		return err
	}
	if err := s.refuseNewerSchema(ctx); err != nil {
		return err
	}
	for _, m := range migrations {
		state, err := s.migrationState(ctx, m.Version)
		if err != nil {
			return err
		}
		switch state {
		case migStateApplied:
			continue
		case migStatePending, migStateInProgress:
			if err := s.applyTransactional(ctx, m, coreVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

// verifyAppliedChecksums detects a modified or missing record for an
// already-applied migration: STATE_CORRUPT — quarantine path, never
// silently treated as current (C2-11 §2).
func (s *Store) verifyAppliedChecksums(ctx context.Context) error {
	rows, err := s.readers.QueryContext(ctx, `SELECT version, name, checksum, state FROM schema_migrations WHERE state = ?`, migStateApplied)
	if err != nil {
		return mapVendorError(err)
	}
	defer rows.Close()
	known := map[int64]Migration{}
	for _, m := range migrations {
		known[m.Version] = m
	}
	for rows.Next() {
		var version int64
		var name, checksum, state string
		if err := rows.Scan(&version, &name, &checksum, &state); err != nil {
			return mapVendorError(err)
		}
		m, ok := known[version]
		if !ok {
			continue // applied by a newer binary; the window check refuses below
		}
		if m.Name != name || checksumOf(m) != checksum {
			return errStoragef(errs.CodeStateCorrupt, "schema_migrations record %d is modified or unrecognized", version)
		}
	}
	return rows.Err()
}

// refuseNewerSchema refuses to start when the database's schema version is
// newer than the running binary supports (downgrade unsupported; the remedy
// is the matching binary or a verified backup restore — never automatic).
func (s *Store) refuseNewerSchema(ctx context.Context) error {
	var maxVersion sql.NullInt64
	row := s.readers.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations WHERE state = ?`, migStateApplied)
	if err := row.Scan(&maxVersion); err != nil {
		return mapVendorError(err)
	}
	if maxVersion.Valid && maxVersion.Int64 > schemaVersionMax {
		return errStoragef(errs.CodeConflictState, "database schema version %d is newer than this binary supports (window [%d, %d])", maxVersion.Int64, schemaVersionMin, schemaVersionMax)
	}
	return nil
}

func (s *Store) migrationState(ctx context.Context, version int64) (string, error) {
	var state string
	row := s.readers.QueryRowContext(ctx, `SELECT state FROM schema_migrations WHERE version = ?`, version)
	if err := row.Scan(&state); err != nil {
		if err == sql.ErrNoRows {
			return migStatePending, nil
		}
		return "", mapVendorError(err)
	}
	return state, nil
}

// applyTransactional applies one transactional migration (C2-11 §3): a
// separate transaction records `in_progress` so an interrupted run is
// observable; the migration + `applied` commit together. A crash rolls the
// DDL back; the next start resumes from the recorded state.
func (s *Store) applyTransactional(ctx context.Context, m Migration, coreVersion string) error {
	if m.Destructive {
		// Fail-closed pre-migration recovery point (C2-11 §4): without a
		// verified backup/recovery point the upgrade does not proceed.
		return errStoragef(errs.CodeRecoveryRequired, "migration %d is destructive and no verified recovery point is recorded", m.Version)
	}
	if _, err := s.writers.ExecContext(ctx, `
INSERT INTO schema_migrations (version, name, checksum, state, step, applied_at, core_version)
VALUES (?, ?, ?, ?, 0, ?, ?)
ON CONFLICT(version) DO UPDATE SET state = excluded.state, applied_at = excluded.applied_at, core_version = excluded.core_version`,
		m.Version, m.Name, checksumOf(m), migStateInProgress, time.Now().UTC().Format(time.RFC3339Nano), coreVersion); err != nil {
		return mapVendorError(err)
	}
	tx, err := s.writers.BeginTx(ctx, nil)
	if err != nil {
		return mapVendorError(err)
	}
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		_ = tx.Rollback()
		return mapVendorError(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET state = ?, applied_at = ?, core_version = ? WHERE version = ?`,
		migStateApplied, time.Now().UTC().Format(time.RFC3339Nano), coreVersion, m.Version); err != nil {
		_ = tx.Rollback()
		return mapVendorError(err)
	}
	if err := tx.Commit(); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// SchemaVersion reports the current applied schema version (status/health).
func (s *Store) SchemaVersion(ctx context.Context) (int64, error) {
	var maxVersion sql.NullInt64
	err := s.readers.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations WHERE state = ?`, migStateApplied).Scan(&maxVersion)
	if err != nil {
		return 0, mapVendorError(err)
	}
	if !maxVersion.Valid {
		return 0, nil
	}
	return maxVersion.Int64, nil
}

// IntegrityCheck runs PRAGMA integrity_check; anything but "ok" is
// STATE_CORRUPT — the quarantine path, never silent repair (C2-09 §4).
func (s *Store) IntegrityCheck(ctx context.Context) error {
	rows, err := s.readers.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return mapVendorError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return mapVendorError(err)
		}
		if result != "ok" {
			return errStoragef(errs.CodeStateCorrupt, "integrity_check reported %q", result)
		}
	}
	return rows.Err()
}
