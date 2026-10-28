package remotestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The C2-11 migration discipline carried to PostgreSQL (spec §1): one
// versioned, checksummed migration authority under exclusive ownership;
// each migration declares atomicity (v1 migrations are single-transaction
// and land directly as `applied`; the `pending`/`in_progress` states exist
// for future resumable migrations); a database whose applied version is
// NEWER than the known set refuses to start (ErrSchemaAhead); a mutated
// applied checksum refuses (ErrChecksumMismatch) — forward-fix only.

type migration struct {
	version int64
	name    string
	sql     string
}

// checksum returns the sha256 integrity anchor recorded in
// schema_migrations.
func (m migration) checksum() string {
	sum := sha256.Sum256([]byte(m.sql))
	return "sha256-" + hex.EncodeToString(sum[:])
}

// migrations is the versioned migration authority (C2-11 discipline):
// the spec §1 schema-area map (v1, Work excluded per D-12/D-33) plus
// forward migrations. Applied migrations are immutable (a change is a new
// version, never an edit); additions append a new entry.
var migrations = []migration{
	{
		version: 1,
		name:    "remote-authority-v1-schema",
		sql: `
CREATE TABLE workspaces (
    id         text PRIMARY KEY,
    org_id     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    workspace_id text NOT NULL REFERENCES workspaces(id),
    principal_id text NOT NULL,
    role         text NOT NULL CHECK (role IN ('owner','admin','member','viewer')),
    state        text NOT NULL CHECK (state IN ('active','revoked')),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, principal_id)
);

CREATE TABLE service_identities (
    id           text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id),
    state        text NOT NULL CHECK (state IN ('active','revoked')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE jobs (
    id                  text PRIMARY KEY,
    workspace_id        text NOT NULL REFERENCES workspaces(id),
    state               text NOT NULL CHECK (state IN ('queued','running','succeeded','failed','cancelled','needs_reconciliation')),
    submitter_id        text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    job_version         bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX jobs_ws_idem ON jobs (workspace_id, idempotency_key);
CREATE INDEX jobs_claim_idx ON jobs (workspace_id, created_at) WHERE state = 'queued';

CREATE TABLE job_fencing (
    job_id        text PRIMARY KEY REFERENCES jobs(id),
    fencing_epoch bigint NOT NULL
);

CREATE TABLE attempts (
    id           text PRIMARY KEY,
    job_id       text NOT NULL REFERENCES jobs(id),
    workspace_id text NOT NULL,
    worker_id    text NOT NULL,
    attempt_no   integer NOT NULL,
    state        text NOT NULL CHECK (state IN ('claimed','running','succeeded','failed','needs_reconciliation')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (job_id, attempt_no)
);

CREATE TABLE leases (
    id            text PRIMARY KEY,
    job_id        text NOT NULL,
    attempt_id    text NOT NULL UNIQUE,
    worker_id     text NOT NULL,
    fencing_epoch bigint NOT NULL,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE side_effect_intents (
    id           text PRIMARY KEY,
    workspace_id text NOT NULL,
    job_id       text NOT NULL,
    attempt_id   text NOT NULL,
    state        text NOT NULL CHECK (state IN ('planned','in_flight','succeeded','failed','unknown','needs_reconciliation')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE job_receipts (
    id            text PRIMARY KEY,
    workspace_id  text NOT NULL,
    job_id        text NOT NULL,
    attempt_id    text NOT NULL,
    outcome       text NOT NULL,
    result_digest text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspace_seq (
    workspace_id text PRIMARY KEY,
    last_seq     bigint NOT NULL
);

CREATE TABLE event_journal (
    workspace_id text NOT NULL,
    seq          bigint NOT NULL,
    event_id     text NOT NULL,
    aggregate    text NOT NULL,
    aggregate_id text NOT NULL,
    type         text NOT NULL,
    payload      jsonb NOT NULL,
    occurred_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, seq)
);
CREATE UNIQUE INDEX event_journal_event_id ON event_journal (event_id);

CREATE TABLE outbox (
    event_id     text PRIMARY KEY,
    workspace_id text NOT NULL,
    published    boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE object_refs (
    workspace_id text NOT NULL,
    digest       text NOT NULL,
    size_bytes   bigint NOT NULL,
    state        text NOT NULL CHECK (state IN ('referenced','orphan_candidate')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, digest)
);`,
	},
	{
		version: 2,
		name:    "service-identity-lifecycle",
		sql: `
-- C4-38 implementation remainder (Remote Operations Specification §1):
-- the workload-scoped service-identity lifecycle. A service principal
-- binds to NAMED WORKSPACES (plural), which the v1 single-column shape
-- cannot express: the binding moves into its own table, where every row
-- carries its workspace scope column (tenant-scoping stays schema-level).
-- The v1 inline column is dropped to nullable for continuity with any
-- rows a pre-v2 database may hold; v2 code resolves bindings ONLY through
-- service_identity_bindings. Rotation uses versioned credentials: the
-- one-time credential plaintext is NEVER stored — the server keeps the
-- SHA-256 digest only. Revocation timestamps make "immediate at the
-- claim boundary" observable and auditable.
ALTER TABLE service_identities ALTER COLUMN workspace_id DROP NOT NULL;
ALTER TABLE service_identities ADD COLUMN revoked_at timestamptz;

CREATE TABLE service_identity_bindings (
    identity_id  text NOT NULL REFERENCES service_identities(id),
    workspace_id text NOT NULL REFERENCES workspaces(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (identity_id, workspace_id)
);

CREATE TABLE service_identity_credentials (
    identity_id     text NOT NULL REFERENCES service_identities(id),
    version         integer NOT NULL,
    credential_hash text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    revoked_at      timestamptz,
    PRIMARY KEY (identity_id, version)
);`,
	},
	{
		version: 3,
		name:    "object-store-lifecycle",
		sql: `
-- C4-36 implementation (spec §4): the shared object/blob store GC
-- lifecycle. The v1 object_refs table already carries the tenant-scoped
-- identity (workspace_id, digest) — the composite IS the object key; a
-- bare content digest is never a global key. v3 adds the GC machinery
-- (C2-12 discipline extended cross-store):
--   * orphan_since stamps the first mark pass durably, so the grace
--     period is measured by the SERVER clock inside the sweep
--     transaction and a sweep is restartable across processes;
--   * the third state 'sweeping' is the sweep LEASE: a row flipped to
--     sweeping excludes it from every concurrent reference commit (the
--     commit refuses and re-uploads afterwards), which is what makes
--     "orphan cleanup never deletes a referenced object" structural —
--     the lease state replaces the CHECK constraint (forward-fix);
--   * object_pins are the backup-pin barriers: while a pin row exists
--     the GC never removes the object and the reference cannot be
--     dropped (the pin id is the backup manifest's reference; no TTL
--     numeric is encoded — the D-27 retention numerics stay Proposed).
ALTER TABLE object_refs ADD COLUMN orphan_since timestamptz;
ALTER TABLE object_refs DROP CONSTRAINT object_refs_state_check;
ALTER TABLE object_refs ADD CONSTRAINT object_refs_state_check
    CHECK (state IN ('referenced','orphan_candidate','sweeping'));

CREATE TABLE object_pins (
    workspace_id text NOT NULL,
    digest       text NOT NULL,
    pin_id       text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, digest, pin_id)
);`,
	},
}

// Migrate applies the known migrations idempotently. Each migration runs
// in exactly one transaction together with its schema_migrations row
// (atomicity). The runner refuses when the database holds a version newer
// than this build knows (forward-fix only) or when an applied checksum no
// longer matches the embedded SQL.
func (s *Store) Migrate(ctx context.Context) error {
	for _, m := range migrations {
		if err := s.migrateOne(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateOne(ctx context.Context, m migration) error {
	// The authority table exists BEFORE any migration transaction touches
	// it: on a fresh database the first probe would fail with a
	// missing-relation error that aborts the transaction, so the idempotent
	// DDL runs outside it.
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
		    version    bigint PRIMARY KEY,
		    name       text NOT NULL,
		    checksum   text NOT NULL,
		    state      text NOT NULL CHECK (state IN ('pending','in_progress','applied')),
		    applied_at timestamptz
		)`); err != nil {
		return fmt.Errorf("remotestore: ensure schema_migrations: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("remotestore: begin migration %d: %w", m.version, err)
	}
	defer tx.Rollback(ctx)

	var state string
	var appliedChecksum string
	err = tx.QueryRow(ctx,
		`SELECT state, checksum FROM schema_migrations WHERE version = $1`, m.version,
	).Scan(&state, &appliedChecksum)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Fresh version: create the objects and land the row atomically.
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("remotestore: apply migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name, checksum, state, applied_at)
			 VALUES ($1, $2, $3, 'applied', now())`, m.version, m.name, m.checksum(),
		); err != nil {
			return fmt.Errorf("remotestore: record migration %d: %w", m.version, err)
		}
	case err != nil:
		return fmt.Errorf("remotestore: read migration %d state: %w", m.version, err)
	case appliedChecksum != m.checksum():
		return fmt.Errorf("remotestore: applied migration %d checksum drifted: %w", m.version, ErrChecksumMismatch)
	case state != "applied":
		return fmt.Errorf("remotestore: migration %d is %q, not applied: %w", m.version, state, ErrSchemaAhead)
	}
	return tx.Commit(ctx)
}

// VerifyAhead refuses a database whose applied version is newer than the
// known set (newer-schema refusal, C2-11). Callers run it after Migrate.
func (s *Store) VerifyAhead(ctx context.Context) error {
	var maxApplied *int64
	if err := s.pool.QueryRow(ctx,
		`SELECT max(version) FROM schema_migrations`,
	).Scan(&maxApplied); err != nil {
		return fmt.Errorf("remotestore: read applied versions: %w", err)
	}
	if maxApplied == nil {
		return nil
	}
	var maxKnown int64
	for _, m := range migrations {
		if m.version > maxKnown {
			maxKnown = m.version
		}
	}
	if *maxApplied > maxKnown {
		return fmt.Errorf("remotestore: database schema v%d is newer than this build knows (v%d): %w",
			*maxApplied, maxKnown, ErrSchemaAhead)
	}
	return nil
}
