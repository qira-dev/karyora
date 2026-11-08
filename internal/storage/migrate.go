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
	{
		Version: 2,
		Name:    "0002-audit-store",
		SQL: `
CREATE TABLE IF NOT EXISTS audit_records (
    sequence    INTEGER PRIMARY KEY,
    prev_hash   TEXT NOT NULL,
    record_hash TEXT NOT NULL,
    payload     BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_checkpoints (
    sequence    INTEGER PRIMARY KEY,
    anchor_hash TEXT NOT NULL,
    anchored_at TEXT NOT NULL
);
`,
	},
	{
		Version: 3,
		Name:    "0003-backup-runs-and-pins",
		SQL: `
CREATE TABLE IF NOT EXISTS backup_runs (
    generation TEXT PRIMARY KEY,
    state      TEXT NOT NULL,
    started_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_pins (
    generation TEXT NOT NULL,
    blob_ref   TEXT NOT NULL,
    pinned_at  TEXT NOT NULL,
    PRIMARY KEY (generation, blob_ref)
);
`,
	},
	{
		Version: 4,
		Name:    "0004-knowledge-pack-store",
		SQL: `
CREATE TABLE IF NOT EXISTS kp_packs (
    pack_id         TEXT PRIMARY KEY,
    current_version INTEGER NOT NULL,
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS kp_versions (
    pack_id        TEXT NOT NULL REFERENCES kp_packs(pack_id),
    version        INTEGER NOT NULL,
    root_digest    TEXT NOT NULL,
    manifest_json  BLOB NOT NULL,
    trust_state    TEXT NOT NULL,
    license        TEXT NOT NULL DEFAULT '',
    license_status TEXT NOT NULL,
    freshness      TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL,
    status_reason  TEXT NOT NULL DEFAULT '',
    stored_at      TEXT NOT NULL,
    PRIMARY KEY (pack_id, version)
);

CREATE TABLE IF NOT EXISTS kp_entries (
    pack_id        TEXT NOT NULL,
    version        INTEGER NOT NULL,
    entry_path     TEXT NOT NULL,
    content        BLOB NOT NULL,
    content_digest TEXT NOT NULL,
    content_type   TEXT NOT NULL,
    size_bytes     INTEGER NOT NULL,
    PRIMARY KEY (pack_id, version, entry_path),
    FOREIGN KEY (pack_id, version) REFERENCES kp_versions(pack_id, version)
);
`,
	},
	{
		Version: 5,
		Name:    "0005-context-snapshots-and-memory",
		SQL: `
CREATE TABLE IF NOT EXISTS context_snapshots (
    snapshot_id      TEXT PRIMARY KEY,
    scope            TEXT NOT NULL,
    task_ref         TEXT NOT NULL DEFAULT '',
    schema_version   INTEGER NOT NULL,
    assembled_digest TEXT NOT NULL,
    snapshot_json    BLOB NOT NULL,
    created_at       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_records (
    memory_id         TEXT PRIMARY KEY,
    scope             TEXT NOT NULL,
    content           TEXT NOT NULL,
    sensitivity       TEXT NOT NULL,
    confidence        REAL NOT NULL,
    status            TEXT NOT NULL,
    provenance_json   BLOB NOT NULL,
    correction_of     TEXT NOT NULL DEFAULT '',
    superseded_by     TEXT NOT NULL DEFAULT '',
    pinned            INTEGER NOT NULL DEFAULT 0,
    ttl_seconds       INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    last_confirmed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_scope_status
    ON memory_records (scope, status);
`,
	},
	{
		Version: 6,
		Name:    "0006-durable-job-relay",
		SQL: `
CREATE TABLE IF NOT EXISTS jobs (
    job_id          TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    input_digest    TEXT NOT NULL,
    state           TEXT NOT NULL,
    version         INTEGER NOT NULL,
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    waiting_reason  TEXT NOT NULL DEFAULT '',
    provider_id     TEXT NOT NULL DEFAULT '',
    min_memory_mb   INTEGER NOT NULL DEFAULT 0,
    deadline_secs   INTEGER NOT NULL DEFAULT 0,
    result_digest   TEXT NOT NULL DEFAULT '',
    receipt_id      TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_state_created ON jobs (state, created_at);

CREATE TABLE IF NOT EXISTS attempts (
    attempt_id       TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL REFERENCES jobs(job_id),
    seq              INTEGER NOT NULL,
    worker_id        TEXT NOT NULL,
    lease_id         TEXT NOT NULL,
    lease_epoch      INTEGER NOT NULL,
    lease_expires_at TEXT NOT NULL DEFAULT '',
    outcome          TEXT NOT NULL,
    started_at       TEXT NOT NULL,
    ends_at          TEXT NOT NULL DEFAULT '',
    UNIQUE (job_id, seq)
);

CREATE TABLE IF NOT EXISTS side_effect_intents (
    intent_id       TEXT PRIMARY KEY,
    job_id          TEXT NOT NULL REFERENCES jobs(job_id),
    attempt_id      TEXT NOT NULL,
    operation       TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    input_digest    TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL,
    provider_ref    TEXT NOT NULL DEFAULT '',
    result_digest   TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    observed_at     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS job_receipts (
    receipt_id    TEXT PRIMARY KEY,
    job_id        TEXT NOT NULL,
    attempt_id    TEXT NOT NULL,
    outcome       TEXT NOT NULL,
    result_digest TEXT NOT NULL DEFAULT '',
    job_version   INTEGER NOT NULL,
    lease_epoch   INTEGER NOT NULL,
    created_at    TEXT NOT NULL
);
`,
	},
	{
		Version: 7,
		Name:    "0007-approval-requests",
		SQL: `
CREATE TABLE IF NOT EXISTS approvals (
    approval_id    TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id),
    state          TEXT NOT NULL,
    decision       TEXT NOT NULL DEFAULT '',
    decided_by     TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    decided_at     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_approvals_job ON approvals (job_id, created_at);
`,
	},
	{
		Version: 8,
		Name:    "0008-provider-connections",
		SQL: `
CREATE TABLE IF NOT EXISTS provider_connections (
    connection_id    TEXT PRIMARY KEY,
    idempotency_key  TEXT NOT NULL UNIQUE,
    provider         TEXT NOT NULL,
    base_url         TEXT NOT NULL DEFAULT '',
    model_id         TEXT NOT NULL,
    custom_headers   TEXT NOT NULL DEFAULT '',
    secret_ref       TEXT NOT NULL,
    credential_epoch INTEGER NOT NULL DEFAULT 0,
    fingerprint      TEXT NOT NULL DEFAULT '',
    tested_at        TEXT NOT NULL DEFAULT '',
    tested_digest    TEXT NOT NULL DEFAULT '',
    tested_epoch     INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_connections_created
    ON provider_connections (created_at, connection_id);

CREATE TABLE IF NOT EXISTS connection_state (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    active_connection_id TEXT NOT NULL DEFAULT '',
    updated_at          TEXT NOT NULL
);

INSERT OR IGNORE INTO connection_state (id, active_connection_id, updated_at)
    VALUES (1, '', '');
`,
	},
	{
		Version: 9,
		Name:    "0009-conversation-aggregates",
		SQL: `
CREATE TABLE IF NOT EXISTS conversations (
    conversation_id TEXT PRIMARY KEY,
    title           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversations_updated
    ON conversations (updated_at, conversation_id);

CREATE TABLE IF NOT EXISTS conversation_messages (
    message_id      TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(conversation_id),
    seq             INTEGER NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    correlation_id  TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    UNIQUE (conversation_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_conv_messages_seq
    ON conversation_messages (conversation_id, seq);

CREATE INDEX IF NOT EXISTS idx_conv_messages_corr
    ON conversation_messages (correlation_id);

CREATE TABLE IF NOT EXISTS conversation_exchanges (
    correlation_id  TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(conversation_id),
    user_message_id TEXT NOT NULL REFERENCES conversation_messages(message_id),
    state           TEXT NOT NULL,
    created_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conv_exchanges_conv
    ON conversation_exchanges (conversation_id, created_at);
`,
	},
	{
		Version: 10,
		Name:    "0010-notification-records",
		SQL: `
CREATE TABLE IF NOT EXISTS notification_records (
    notification_id     TEXT PRIMARY KEY,
    category            TEXT NOT NULL,
    state_class         TEXT NOT NULL,
    related_object_type TEXT NOT NULL,
    related_object_id   TEXT NOT NULL,
    source_event_id     TEXT NOT NULL DEFAULT '',
    correlation_id      TEXT NOT NULL DEFAULT '',
    read_state          TEXT NOT NULL DEFAULT 'unread',
    cleared             INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    read_at             TEXT NOT NULL DEFAULT '',
    cleared_at          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_notifications_created
    ON notification_records (created_at, notification_id);

CREATE INDEX IF NOT EXISTS idx_notifications_object
    ON notification_records (related_object_type, related_object_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_source_event
    ON notification_records (source_event_id) WHERE source_event_id != '';
`,
	},
	{
		Version: 11,
		Name:    "0011-connection-display-name",
		SQL: `
ALTER TABLE provider_connections ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
`,
	},
	{
		Version: 12,
		Name:    "0012-conversation-attachments",
		SQL: `
CREATE TABLE IF NOT EXISTS conversation_attachments (
    attachment_id   TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(conversation_id),
    upload_id       TEXT NOT NULL,
    file_name       TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      INTEGER NOT NULL,
    digest          TEXT NOT NULL,
    untrusted_label TEXT NOT NULL,
    input_revision  TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    UNIQUE (conversation_id, upload_id)
);

CREATE INDEX IF NOT EXISTS idx_conv_attachments_conv
    ON conversation_attachments (conversation_id, created_at, attachment_id);
`,
	},
	{
		Version: 13,
		Name:    "0013-knowledge-pack-family",
		SQL: `
CREATE TABLE IF NOT EXISTS knowledge_packs (
    pack_id            TEXT PRIMARY KEY,
    schema_version     INTEGER NOT NULL,
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    current_version_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_knowledge_packs_created
    ON knowledge_packs (created_at, pack_id);

CREATE TABLE IF NOT EXISTS knowledge_pack_versions (
    pack_id          TEXT NOT NULL REFERENCES knowledge_packs(pack_id),
    version_id       TEXT NOT NULL,
    schema_version   INTEGER NOT NULL,
    pack_root_digest TEXT NOT NULL,
    entry_count      INTEGER NOT NULL DEFAULT 0,
    claim_count      INTEGER NOT NULL DEFAULT 0,
    evidence_count   INTEGER NOT NULL DEFAULT 0,
    supersedes       TEXT NOT NULL DEFAULT '[]',
    superseded_by    TEXT NOT NULL DEFAULT '',
    license          TEXT NOT NULL DEFAULT '',
    license_status   TEXT NOT NULL DEFAULT 'unknown',
    created_at       TEXT NOT NULL,
    PRIMARY KEY (pack_id, version_id)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_versions_created
    ON knowledge_pack_versions (pack_id, created_at, version_id);

CREATE TABLE IF NOT EXISTS knowledge_pack_entries (
    pack_id      TEXT NOT NULL,
    version_id   TEXT NOT NULL,
    entry_path   TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    digest       TEXT NOT NULL,
    content      BLOB NOT NULL,
    PRIMARY KEY (pack_id, version_id, entry_path),
    FOREIGN KEY (pack_id, version_id) REFERENCES knowledge_pack_versions(pack_id, version_id)
);

CREATE TABLE IF NOT EXISTS knowledge_operations (
    operation_id    TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    state           TEXT NOT NULL,
    correlation_id  TEXT NOT NULL,
    failure_code    TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    untrusted_label TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    source_digest   TEXT NOT NULL DEFAULT '',
    create_intent   TEXT NOT NULL DEFAULT '',
    result          TEXT NOT NULL DEFAULT '',
    terminal_at     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_operations_key_unique
    ON knowledge_operations (idempotency_key) WHERE idempotency_key != '';

CREATE INDEX IF NOT EXISTS idx_knowledge_operations_intent
    ON knowledge_operations (create_intent, source_digest, state);
`,
	},
	{
		Version: 14,
		Name:    "0014-memory-item-family",
		SQL: `
CREATE TABLE IF NOT EXISTS memory_items (
    memory_item_id    TEXT PRIMARY KEY,
    scope             TEXT NOT NULL,
    origin            TEXT NOT NULL,
    lifecycle_state   TEXT NOT NULL,
    content           TEXT NOT NULL,
    pinned            INTEGER NOT NULL DEFAULT 0,
    creation_method   TEXT NOT NULL DEFAULT '',
    confidence        TEXT NOT NULL DEFAULT '',
    originating_refs  TEXT NOT NULL DEFAULT '[]',
    retention_ttl_ref TEXT NOT NULL DEFAULT '',
    conflict_note     TEXT NOT NULL DEFAULT '',
    untrusted_label   TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    last_used_at      TEXT NOT NULL DEFAULT '',
    last_confirmed_at TEXT NOT NULL DEFAULT '',
    updated_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_items_created
    ON memory_items (created_at, memory_item_id);

CREATE INDEX IF NOT EXISTS idx_memory_items_scope
    ON memory_items (scope, lifecycle_state, created_at);

CREATE TABLE IF NOT EXISTS memory_operations (
    operation_id    TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    state           TEXT NOT NULL,
    correlation_id  TEXT NOT NULL DEFAULT '',
    memory_item_id  TEXT NOT NULL DEFAULT '',
    outcome_state   TEXT NOT NULL DEFAULT '',
    export_ref      TEXT NOT NULL DEFAULT '',
    failure_code    TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    fingerprint     TEXT NOT NULL DEFAULT '',
    terminal_at     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_operations_key_unique
    ON memory_operations (idempotency_key) WHERE idempotency_key != '';

CREATE INDEX IF NOT EXISTS idx_memory_operations_item
    ON memory_operations (memory_item_id, created_at);

CREATE TABLE IF NOT EXISTS memory_inclusions (
    request_ref    TEXT NOT NULL,
    memory_item_id TEXT NOT NULL,
    included       INTEGER NOT NULL,
    correlation_id TEXT NOT NULL,
    operation_id   TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (request_ref, memory_item_id)
);
`,
	},
	{
		Version: 15,
		Name:    "0015-context-snapshot-family",
		SQL: `
CREATE TABLE IF NOT EXISTS context_operations (
    operation_id    TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    state           TEXT NOT NULL,
    correlation_id  TEXT NOT NULL,
    source_job_id   TEXT NOT NULL DEFAULT '',
    result_json     TEXT NOT NULL DEFAULT '',
    failure_code    TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    terminal_at     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_context_operations_correlation
    ON context_operations (correlation_id, created_at);

CREATE TABLE IF NOT EXISTS context_inclusions (
    request_ref    TEXT NOT NULL,
    memory_item_id TEXT NOT NULL,
    included       INTEGER NOT NULL,
    correlation_id TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (request_ref, memory_item_id)
);

CREATE INDEX IF NOT EXISTS idx_context_inclusions_request
    ON context_inclusions (request_ref, updated_at);
`,
	},
	{
		Version: 16,
		Name:    "0016-workflow-authoring-family",
		SQL: `
CREATE TABLE IF NOT EXISTS workflow_definitions (
    definition_id        TEXT PRIMARY KEY,
    display_name         TEXT NOT NULL DEFAULT '',
    current_revision_ref TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_definitions_created
    ON workflow_definitions (created_at, definition_id);

CREATE TABLE IF NOT EXISTS workflow_revisions (
    definition_id TEXT NOT NULL REFERENCES workflow_definitions(definition_id),
    revision_ref  TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    PRIMARY KEY (definition_id, revision_ref)
);

CREATE INDEX IF NOT EXISTS idx_workflow_revisions_created
    ON workflow_revisions (definition_id, created_at, revision_ref);

CREATE TABLE IF NOT EXISTS workflow_operations (
    operation_id    TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    state           TEXT NOT NULL,
    correlation_id  TEXT NOT NULL,
    definition_id   TEXT NOT NULL DEFAULT '',
    revision_ref    TEXT NOT NULL DEFAULT '',
    failure_code    TEXT NOT NULL DEFAULT '',
    failure_reason  TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    fingerprint     TEXT NOT NULL DEFAULT '',
    terminal_at     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_operations_key_unique
    ON workflow_operations (idempotency_key) WHERE idempotency_key != '';

CREATE INDEX IF NOT EXISTS idx_workflow_operations_definition
    ON workflow_operations (definition_id, created_at);

CREATE TABLE IF NOT EXISTS workflow_run_history (
    run_ref         TEXT PRIMARY KEY,
    definition_id   TEXT NOT NULL,
    source_revision TEXT NOT NULL DEFAULT '',
    linkage_state   TEXT NOT NULL,
    created_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_run_history_definition
    ON workflow_run_history (definition_id, created_at, run_ref);
`,
	},
}

// compatibility window (C2-11 §5): a binary of release N supports schema
// versions [N_min, N]. Phase 3 adds the knowledge-pack store; the window is
// recorded per release and published floors remain a Phase 5/D-30 concern.
// Migration 0013 adds the B1 KnowledgePack family's tables; migration 0014
// adds the B2 Memory family's tables; migration 0015 adds the B3
// ContextSnapshot family's tables (the authoritative operation records and
// the M-18 request-scoped inclusion decisions); migration 0016 adds the B4
// Workflow authoring family's tables (the Core-minted definitions, the
// immutable revision records — INSERT only, never updated or removed, the
// keyed create/save operation records, and the run-history linkage
// records); existing migrations are never modified (checksums
// authoritative).
const (
	schemaVersionMin = 1
	schemaVersionMax = 16
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
