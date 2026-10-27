package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/events"
)

// readonlyConnector opens one read-only connection for the doctor probe.
type readonlyConnector struct {
	dsn string
}

func (c readonlyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	d := &sqlite3.SQLiteDriver{}
	return d.Open(c.dsn)
}

func (c readonlyConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

// ProbeReadonly runs doctor's read-only storage check: open the database
// read-only, run PRAGMA integrity_check, and report the applied schema
// version. doctor never repairs; corruption surfaces as an error for the
// quarantine path (C2-07 §2, C2-11 §2).
func ProbeReadonly(ctx context.Context, path string) (schemaVersion int64, integrity string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, "", fmt.Errorf("storage: resolve %s: %w", path, err)
	}
	db := sql.OpenDB(readonlyConnector{dsn: "file:" + abs + "?mode=ro"})
	defer db.Close()
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return 0, "", fmt.Errorf("storage: integrity check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return 0, "", fmt.Errorf("storage: integrity check: %w", err)
		}
		if result != "ok" {
			return 0, result, fmt.Errorf("storage: integrity_check reported %q", result)
		}
		integrity = result
	}
	if err := rows.Err(); err != nil {
		return 0, "", fmt.Errorf("storage: integrity check: %w", err)
	}
	row := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`)
	var hasTable int
	if err := row.Scan(&hasTable); err != nil {
		return 0, integrity, fmt.Errorf("storage: schema probe: %w", err)
	}
	if hasTable == 0 {
		return 0, integrity, nil
	}
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE state = 'applied'`).Scan(&schemaVersion)
	if err != nil {
		return 0, integrity, fmt.Errorf("storage: schema version: %w", err)
	}
	return schemaVersion, integrity, nil
}

// JobRecordReadonly reads one Job's record and journal events without
// taking data-directory ownership (the doctor-style read-only probe path,
// C2-07): the CLI status/events surface never writes. A missing table or
// row is a clean not-found, never an error state.
func JobRecordReadonly(ctx context.Context, path, jobID string) (*JobRecord, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve %s: %w", path, err)
	}
	db := sql.OpenDB(readonlyConnector{dsn: "file:" + abs + "?mode=ro"})
	defer db.Close()
	rec := &JobRecord{}
	row := db.QueryRowContext(ctx, `SELECT job_id, idempotency_key, input_digest, state, version,
		attempt_count, waiting_reason, provider_id, deadline_secs, result_digest, receipt_id, correlation_id,
		created_at, updated_at FROM jobs WHERE job_id = ?`, jobID)
	var waitingReason, providerID string
	var createdAt, updatedAt string
	if err := row.Scan(&rec.Job.JobID, &rec.Job.IdempotencyKey, &rec.Job.InputDigest, &rec.Job.State,
		&rec.Job.Version, &rec.Job.AttemptCount, &waitingReason, &providerID, &rec.Job.DeadlineSecs,
		&rec.Job.ResultDigest, &rec.Job.ReceiptID, &rec.Job.CorrelationID, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: job probe: %w", err)
	}
	rec.Job.WaitingReason = waitingReason
	rec.Job.Requirements.ProviderID = providerID
	rec.Job.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	rec.Job.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	events, err := jobEventsReadonly(ctx, db, jobID)
	if err != nil {
		return nil, err
	}
	rec.Events = events
	return rec, nil
}

// JobRecord is the read-only probe result for one job.
type JobRecord struct {
	Job    aijobrelay.Job  `json:"job"`
	Events []events.Record `json:"events,omitempty"`
}

func jobEventsReadonly(ctx context.Context, db *sql.DB, jobID string) ([]events.Record, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_id, aggregate_type, aggregate_id, sequence, event_type,
		schema_version, occurred_at, causation_id, correlation_id, producer, payload, payload_digest, blob_ref
		FROM events_journal WHERE aggregate_type = 'job' AND aggregate_id = ? ORDER BY sequence ASC`, jobID)
	if err != nil {
		return nil, fmt.Errorf("storage: job events probe: %w", err)
	}
	defer rows.Close()
	var out []events.Record
	for rows.Next() {
		var r events.Record
		var occurred string
		var payload []byte
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.Sequence, &r.EventType,
			&r.SchemaVersion, &occurred, &r.CausationID, &r.CorrelationID, &r.Producer, &payload,
			&r.PayloadDigest, &r.BlobRef); err != nil {
			return nil, fmt.Errorf("storage: job events probe: %w", err)
		}
		r.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		if len(payload) > 0 {
			r.InlinePayload = json.RawMessage(payload)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
