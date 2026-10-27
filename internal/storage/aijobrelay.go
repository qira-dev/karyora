package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/events"
)

// The Store satisfies the durable relay contract (C3-13/C3-14/C3-15):
// claims are atomic under the single-writer gate; every authoritative
// write is fenced by the Job version and lease epoch; commits apply the
// Job transition, Attempt outcome, receipt, journal, and outbox in ONE
// transaction.
var _ aijobrelay.RelayStore = (*Store)(nil)

// errIdempotentDuplicate marks the in-transaction duplicate-submit case
// that resolves to returning the existing Job.
var errIdempotentDuplicate = fmt.Errorf("aijobrelay: duplicate submission of the same key and input")

// appendEventTx allocates the per-aggregate monotonic sequence and writes
// the journal + outbox rows inside the caller's transaction (C2-13 §2).
func appendEventTx(tx *sqlTx, rec events.Record) error {
	if rec.EventID == "" || rec.AggregateType == "" || rec.AggregateID == "" || rec.EventType == "" {
		return errStoragef(errs.CodeValidationFailed, "event lacks identity fields")
	}
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now().UTC()
	}
	rec.SchemaVersion = 1
	var maxSeq sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(sequence) FROM events_journal WHERE aggregate_type = ? AND aggregate_id = ?`,
		rec.AggregateType, rec.AggregateID).Scan(&maxSeq); err != nil {
		return mapVendorError(err)
	}
	rec.Sequence = uint64(maxSeq.Int64) + 1
	payload := any(nil)
	if len(rec.InlinePayload) > 0 {
		payload = []byte(rec.InlinePayload)
	}
	if _, err := tx.Exec(`INSERT INTO events_journal
		(event_id, aggregate_type, aggregate_id, sequence, event_type, schema_version, occurred_at,
		 causation_id, correlation_id, producer, payload, payload_digest, blob_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.EventID, rec.AggregateType, rec.AggregateID, rec.Sequence, rec.EventType, rec.SchemaVersion,
		rec.OccurredAt.Format(time.RFC3339Nano), rec.CausationID, rec.CorrelationID, rec.Producer,
		payload, rec.PayloadDigest, rec.BlobRef); err != nil {
		return mapVendorError(err)
	}
	if _, err := tx.Exec(`INSERT INTO events_outbox (event_id) VALUES (?)`, rec.EventID); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// SubmitJob implements aijobrelay.RelayStore with the C3-02 idempotency
// semantics: same key + same input digest returns the existing Job; same
// key + different input digest is a CONFLICT.
func (s *Store) SubmitJob(ctx context.Context, job aijobrelay.Job) (*aijobrelay.Job, error) {
	if err := aijobrelay.ValidateSubmit(&job); err != nil {
		return nil, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	if existing, err := s.jobByIdempotencyKey(ctx, job.IdempotencyKey); err == nil && existing != nil {
		if existing.InputDigest == job.InputDigest {
			return existing, nil
		}
		return nil, errStoragef(errs.CodeConflictState, "idempotency key %q was already used with a different input; replay rejected", job.IdempotencyKey)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job.State = aijobrelay.JobQueued
	job.UpdatedAt = job.CreatedAt
	err := s.CommitStateChange(ctx, job.CorrelationID, func(tx *sqlTx) error {
		_, err := tx.Exec(`INSERT INTO jobs
			(job_id, idempotency_key, input_digest, state, version, attempt_count, waiting_reason,
			 provider_id, min_memory_mb, deadline_secs, result_digest, receipt_id, correlation_id,
			 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.JobID, job.IdempotencyKey, job.InputDigest, job.State, job.Version, job.AttemptCount,
			job.WaitingReason, job.Requirements.ProviderID, job.Requirements.MinMemoryMB,
			job.DeadlineSecs, job.ResultDigest, job.ReceiptID, job.CorrelationID,
			job.CreatedAt.UTC().Format(time.RFC3339Nano), now)
		if err != nil {
			if existing, lookupErr := s.jobByIdempotencyKey(ctx, job.IdempotencyKey); lookupErr == nil && existing != nil {
				if existing.InputDigest == job.InputDigest {
					return errIdempotentDuplicate
				}
				return errStoragef(errs.CodeConflictState, "idempotency key %q was already used with a different input; replay rejected", job.IdempotencyKey)
			}
			return mapVendorError(err)
		}
		return appendEventTx(tx, events.Record{
			EventID:       "evt-" + job.JobID + "-submitted",
			AggregateType: "job",
			AggregateID:   job.JobID,
			EventType:     "job.submitted",
			CorrelationID: job.CorrelationID,
			Producer:      s.instanceUUID,
			InlinePayload: mustJSON(map[string]string{"input_digest": job.InputDigest, "idempotency_key": job.IdempotencyKey}),
		})
	})
	if err == errIdempotentDuplicate {
		return s.jobByIdempotencyKey(ctx, job.IdempotencyKey)
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) jobByIdempotencyKey(ctx context.Context, key string) (*aijobrelay.Job, error) {
	row := s.readers.QueryRowContext(ctx, `SELECT job_id FROM jobs WHERE idempotency_key = ?`, key)
	var jobID string
	if err := row.Scan(&jobID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, mapVendorError(err)
	}
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

const jobColumns = `job_id, idempotency_key, input_digest, state, version, attempt_count, waiting_reason,
	provider_id, min_memory_mb, deadline_secs, result_digest, receipt_id, correlation_id, created_at, updated_at`

func scanJob(row rowScanner) (aijobrelay.Job, error) {
	var job aijobrelay.Job
	var createdAt, updatedAt string
	err := row.Scan(&job.JobID, &job.IdempotencyKey, &job.InputDigest, &job.State, &job.Version,
		&job.AttemptCount, &job.WaitingReason, &job.Requirements.ProviderID, &job.Requirements.MinMemoryMB,
		&job.DeadlineSecs, &job.ResultDigest, &job.ReceiptID, &job.CorrelationID, &createdAt, &updatedAt)
	if err != nil {
		return job, err
	}
	if job.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return job, errStoragef(errs.CodeStateCorrupt, "job created_at does not parse: %v", err)
	}
	if job.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return job, errStoragef(errs.CodeStateCorrupt, "job updated_at does not parse: %v", err)
	}
	return job, nil
}

// LoadJob implements aijobrelay.RelayStore.
func (s *Store) LoadJob(ctx context.Context, jobID string) (aijobrelay.Job, error) {
	row := s.readers.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE job_id = ?`, jobID)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return aijobrelay.Job{}, errStoragef(errs.CodeNotFound, "job %q is not stored", jobID)
	}
	return job, err
}

// LoadCurrentAttempt implements aijobrelay.RelayStore.
func (s *Store) LoadCurrentAttempt(ctx context.Context, jobID string) (aijobrelay.Attempt, error) {
	row := s.readers.QueryRowContext(ctx, `SELECT attempt_id, job_id, seq, worker_id, lease_id, lease_epoch,
		lease_expires_at, outcome, started_at, ends_at FROM attempts WHERE job_id = ? ORDER BY seq DESC LIMIT 1`, jobID)
	return scanAttempt(row)
}

func scanAttempt(row rowScanner) (aijobrelay.Attempt, error) {
	var a aijobrelay.Attempt
	var leaseExpires, startedAt, endsAt string
	err := row.Scan(&a.AttemptID, &a.JobID, &a.Seq, &a.WorkerID, &a.LeaseID, &a.LeaseEpoch,
		&leaseExpires, &a.Outcome, &startedAt, &endsAt)
	if err != nil {
		return a, err
	}
	if a.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
		return a, errStoragef(errs.CodeStateCorrupt, "attempt started_at does not parse: %v", err)
	}
	return a, nil
}

// ClaimNext implements aijobrelay.RelayStore: one writer transaction picks
// the oldest runnable Job matching the worker capability, creates the
// Attempt + Lease, bumps the version, and emits job.claimed atomically.
func (s *Store) ClaimNext(ctx context.Context, worker aijobrelay.WorkerCapability, leaseTTL time.Duration) (*aijobrelay.Claim, bool, error) {
	if worker.WorkerID == "" {
		return nil, false, errStoragef(errs.CodeValidationFailed, "worker identity is required")
	}
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	providers := worker.ProviderIDs
	var claimed *aijobrelay.Claim
	err := s.withWriter(ctx, func(tx *sqlTx) error {
		query := `SELECT ` + jobColumns + ` FROM jobs
			WHERE state = ? AND min_memory_mb <= ?
			ORDER BY created_at ASC, job_id ASC LIMIT 1`
		args := []any{aijobrelay.JobQueued, worker.MaxMemoryMB}
		if len(providers) > 0 {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(providers)), ",")
			query = `SELECT ` + jobColumns + ` FROM jobs
				WHERE state = ? AND min_memory_mb <= ? AND (provider_id = '' OR provider_id IN (` + placeholders + `))
				ORDER BY created_at ASC, job_id ASC LIMIT 1`
			args = append(args, toAny(providers)...)
		}
		row := tx.QueryRow(query, args...)
		job, err := scanJob(row)
		if err == sql.ErrNoRows {
			return nil // nothing runnable; claimed=false
		}
		if err != nil {
			return mapVendorError(err)
		}
		attempt := aijobrelay.Attempt{
			AttemptID:  fmt.Sprintf("att-%s-%d", job.JobID, job.AttemptCount+1),
			JobID:      job.JobID,
			Seq:        job.AttemptCount + 1,
			WorkerID:   worker.WorkerID,
			LeaseID:    fmt.Sprintf("lease-%s-%d", job.JobID, job.AttemptCount+1),
			LeaseEpoch: uint64(job.AttemptCount + 1),
			Outcome:    aijobrelay.AttemptRunning,
			StartedAt:  time.Now().UTC(),
		}
		expires := attempt.StartedAt.Add(leaseTTL).Format(time.RFC3339Nano)
		if _, err := tx.Exec(`INSERT INTO attempts
			(attempt_id, job_id, seq, worker_id, lease_id, lease_epoch, lease_expires_at, outcome, started_at, ends_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
			attempt.AttemptID, attempt.JobID, attempt.Seq, attempt.WorkerID, attempt.LeaseID,
			attempt.LeaseEpoch, expires, attempt.Outcome, attempt.StartedAt.Format(time.RFC3339Nano)); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, attempt_count = ?,
			waiting_reason = '', updated_at = ? WHERE job_id = ? AND version = ? AND state = ?`,
			aijobrelay.JobRunning, attempt.Seq, time.Now().UTC().Format(time.RFC3339Nano),
			job.JobID, job.Version, aijobrelay.JobQueued); err != nil {
			return mapVendorError(err)
		}
		job.State = aijobrelay.JobRunning
		job.Version++
		job.AttemptCount = attempt.Seq
		if err := appendEventTx(tx, events.Record{
			EventID:       "evt-" + job.JobID + "-claimed-" + fmt.Sprint(attempt.Seq),
			AggregateType: "job",
			AggregateID:   job.JobID,
			EventType:     "job.claimed",
			CorrelationID: job.CorrelationID,
			Producer:      s.instanceUUID,
			InlinePayload: mustJSON(map[string]any{"attempt_id": attempt.AttemptID, "worker_id": attempt.WorkerID, "lease_epoch": attempt.LeaseEpoch}),
		}); err != nil {
			return err
		}
		claimed = &aijobrelay.Claim{Job: job, Attempt: attempt}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return claimed, claimed != nil, nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// RenewLease implements aijobrelay.RelayStore: only a live (unexpired)
// lease with the exact epoch/worker can be renewed back to life (C3-13).
func (s *Store) RenewLease(ctx context.Context, jobID, attemptID, workerID string, leaseEpoch uint64, ttl time.Duration) error {
	res, err := s.writers.ExecContext(ctx,
		`UPDATE attempts SET lease_expires_at = ?
		 WHERE job_id = ? AND attempt_id = ? AND worker_id = ? AND lease_epoch = ?
		   AND outcome = ? AND lease_expires_at != '' AND lease_expires_at > ?`,
		time.Now().UTC().Add(ttl).Format(time.RFC3339Nano), jobID, attemptID, workerID, leaseEpoch,
		aijobrelay.AttemptRunning, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return mapVendorError(err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errStoragef(errs.CodeConflictState, "lease for job %q attempt %q is expired or stale; renewal refused", jobID, attemptID)
	}
	return nil
}

// ReleaseExpiredLeases implements aijobrelay.RelayStore (C3-13/C3-14):
// expired leases fail their Attempt retryably and re-queue the Job —
// unless an Unknown side effect exists, which forces NeedsReconciliation.
func (s *Store) ReleaseExpiredLeases(ctx context.Context, now time.Time) ([]aijobrelay.Job, error) {
	nowStr := now.UTC().Format(time.RFC3339Nano)
	rows, err := s.readers.QueryContext(ctx, `
		SELECT a.job_id, a.attempt_id, a.seq, a.worker_id, a.lease_id, a.lease_epoch,
		       a.lease_expires_at, a.outcome, a.started_at, a.ends_at
		FROM attempts a JOIN jobs j ON j.job_id = a.job_id
		WHERE a.outcome = ? AND a.lease_expires_at != '' AND a.lease_expires_at <= ?
		  AND j.state = ?`, aijobrelay.AttemptRunning, nowStr, aijobrelay.JobRunning)
	if err != nil {
		return nil, mapVendorError(err)
	}
	type expiredLease struct {
		jobID string
		att   aijobrelay.Attempt
	}
	var expiredLeases []expiredLease
	for rows.Next() {
		var e expiredLease
		var leaseExpires, startedAt, endsAt string
		if err := rows.Scan(&e.jobID, &e.att.AttemptID, &e.att.Seq, &e.att.WorkerID, &e.att.LeaseID,
			&e.att.LeaseEpoch, &leaseExpires, &e.att.Outcome, &startedAt, &endsAt); err != nil {
			_ = rows.Close()
			return nil, mapVendorError(err)
		}
		if e.att.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
			_ = rows.Close()
			return nil, errStoragef(errs.CodeStateCorrupt, "attempt started_at does not parse: %v", err)
		}
		expiredLeases = append(expiredLeases, e)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, mapVendorError(err)
	}
	var resolved []aijobrelay.Job
	for _, e := range expiredLeases {
		var unknownCount int
		if err := s.readers.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM side_effect_intents WHERE job_id = ? AND state = ?`,
			e.jobID, aijobrelay.SideEffectUnknown).Scan(&unknownCount); err != nil {
			return resolved, mapVendorError(err)
		}
		toState := aijobrelay.JobQueued
		waitingReason := ""
		if unknownCount > 0 {
			toState = aijobrelay.JobNeedsReconciliation
			waitingReason = string(aijobrelay.WaitUnknownSideEffect)
		}
		job, err := s.LoadJob(ctx, e.jobID)
		if err != nil {
			return resolved, err
		}
		err = s.CommitStateChange(ctx, job.CorrelationID, func(tx *sqlTx) error {
			if _, err := tx.Exec(`UPDATE attempts SET outcome = ?, ends_at = ? WHERE attempt_id = ? AND outcome = ?`,
				aijobrelay.AttemptFailedRetryable, nowStr, e.att.AttemptID, aijobrelay.AttemptRunning); err != nil {
				return mapVendorError(err)
			}
			if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = ?, updated_at = ?
				WHERE job_id = ? AND version = ? AND state = ?`,
				toState, waitingReason, nowStr, e.jobID, job.Version, aijobrelay.JobRunning); err != nil {
				return mapVendorError(err)
			}
			eventType := "job.requeued"
			if unknownCount > 0 {
				eventType = "job.needs_reconciliation"
			}
			return appendEventTx(tx, events.Record{
				EventID:       "evt-" + e.jobID + "-lease-expired-" + fmt.Sprint(e.att.Seq),
				AggregateType: "job",
				AggregateID:   e.jobID,
				EventType:     eventType,
				CorrelationID: job.CorrelationID,
				Producer:      s.instanceUUID,
				InlinePayload: mustJSON(map[string]any{"attempt_id": e.att.AttemptID, "reason": "lease_expired"}),
			})
		})
		if err != nil {
			return resolved, err
		}
		updated, err := s.LoadJob(ctx, e.jobID)
		if err != nil {
			return resolved, err
		}
		resolved = append(resolved, updated)
	}
	return resolved, nil
}

// PrepareSideEffect implements aijobrelay.RelayStore: the intent is durable
// BEFORE any external effect, fenced to the current attempt (C3-14).
func (s *Store) PrepareSideEffect(ctx context.Context, intent aijobrelay.SideEffectIntent) error {
	if intent.IntentID == "" || intent.JobID == "" || intent.AttemptID == "" || intent.Operation == "" || intent.IdempotencyKey == "" {
		return errStoragef(errs.CodeValidationFailed, "side-effect intent lacks identity fields")
	}
	intent.State = aijobrelay.SideEffectPrepared
	intent.CreatedAt = time.Now().UTC()
	return s.withWriter(ctx, func(tx *sqlTx) error {
		var one int
		err := tx.QueryRow(`SELECT 1 FROM side_effect_intents WHERE intent_id = ?`, intent.IntentID).Scan(&one)
		if err == nil {
			return errStoragef(errs.CodeConflictState, "side-effect intent %q already exists", intent.IntentID)
		}
		if err != sql.ErrNoRows {
			return mapVendorError(err)
		}
		_, err = tx.Exec(`INSERT INTO side_effect_intents
			(intent_id, job_id, attempt_id, operation, idempotency_key, input_digest, state,
			 provider_ref, result_digest, created_at, observed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
			intent.IntentID, intent.JobID, intent.AttemptID, intent.Operation, intent.IdempotencyKey,
			intent.InputDigest, intent.State, intent.ProviderRef, intent.ResultDigest,
			intent.CreatedAt.Format(time.RFC3339Nano))
		return mapVendorError(err)
	})
}

// ObserveSideEffect implements aijobrelay.RelayStore with full fencing
// (C3-14): the observation is validated against the domain rules, the
// intent transition is applied, and an Unknown outcome moves the Job to
// NeedsReconciliation — never a blind retry.
func (s *Store) ObserveSideEffect(ctx context.Context, jobID, intentID string, jobVersion, leaseEpoch uint64, attemptID, toState, resultDigest string) error {
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return err
	}
	intent, err := s.LoadIntent(ctx, intentID)
	if err != nil {
		return err
	}
	if err := aijobrelay.CheckObservation(intent, jobVersion, attemptID, leaseEpoch, toState, job); err != nil {
		return errStoragef(errs.CodeConflictState, "%v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE side_effect_intents SET state = ?, result_digest = ?, observed_at = ?
			WHERE intent_id = ? AND state = ?`, toState, resultDigest, now, intentID, intent.State); err != nil {
			return mapVendorError(err)
		}
		if toState == aijobrelay.SideEffectUnknown {
			if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = ?, updated_at = ?
				WHERE job_id = ? AND version = ? AND state = ?`,
				aijobrelay.JobNeedsReconciliation, string(aijobrelay.WaitUnknownSideEffect), now, jobID, jobVersion, job.State); err != nil {
				return mapVendorError(err)
			}
			return appendEventTx(tx, events.Record{
				EventID:       "evt-" + jobID + "-unknown-" + intentID,
				AggregateType: "job",
				AggregateID:   jobID,
				EventType:     "job.needs_reconciliation",
				CorrelationID: job.CorrelationID,
				Producer:      s.instanceUUID,
				InlinePayload: mustJSON(map[string]any{"intent_id": intentID, "reason": "unknown_side_effect"}),
			})
		}
		return nil
	})
}

// CommitAttempt implements aijobrelay.RelayStore: the fenced atomic commit
// (C3-13/C3-15) — Job transition + Attempt outcome + Receipt + journal +
// outbox in ONE transaction, validated by the domain rules first.
func (s *Store) CommitAttempt(ctx context.Context, commit aijobrelay.Commit) error {
	job, err := s.LoadJob(ctx, commit.JobID)
	if err != nil {
		return err
	}
	attempt, err := s.LoadCurrentAttempt(ctx, commit.JobID)
	if err != nil {
		return err
	}
	if err := aijobrelay.CheckCommit(commit, job, attempt); err != nil {
		return errStoragef(errs.CodeConflictState, "%v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	receipt := aijobrelay.Receipt{
		ReceiptID: commit.ReceiptID, JobID: commit.JobID, AttemptID: commit.AttemptID,
		Outcome: commit.AttemptOutcome, ResultDigest: commit.ResultDigest,
		JobVersion: commit.JobVersion, LeaseEpoch: commit.LeaseEpoch, CreatedAt: time.Now().UTC(),
	}
	if commit.ToState == aijobrelay.JobSucceeded || commit.ToState == aijobrelay.JobFailed ||
		commit.ToState == aijobrelay.JobTimedOut || commit.ToState == aijobrelay.JobCancelled {
		if receipt.ReceiptID == "" {
			return errStoragef(errs.CodeValidationFailed, "a terminal commit requires a receipt id")
		}
	}
	event := commit.Event
	if event.EventID == "" {
		event = events.Record{
			EventID:       "evt-" + commit.JobID + "-" + commit.ToState + "-" + fmt.Sprint(job.Version),
			AggregateType: "job",
			AggregateID:   commit.JobID,
			EventType:     "job." + commit.ToState,
			CorrelationID: commit.CorrelationID,
			Producer:      s.instanceUUID,
		}
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = ?,
			result_digest = CASE WHEN ? != '' THEN ? ELSE result_digest END,
			receipt_id = CASE WHEN ? != '' THEN ? ELSE receipt_id END, updated_at = ?
			WHERE job_id = ? AND version = ? AND state = ?`,
			commit.ToState, commit.WaitingReason, commit.ResultDigest, commit.ResultDigest,
			commit.ReceiptID, commit.ReceiptID, now, commit.JobID, commit.JobVersion, job.State); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`UPDATE attempts SET outcome = ?, ends_at = ? WHERE attempt_id = ? AND lease_epoch = ? AND outcome = ?`,
			commit.AttemptOutcome, now, commit.AttemptID, commit.LeaseEpoch, aijobrelay.AttemptRunning); err != nil {
			return mapVendorError(err)
		}
		if receipt.ReceiptID != "" {
			if _, err := tx.Exec(`INSERT INTO job_receipts
				(receipt_id, job_id, attempt_id, outcome, result_digest, job_version, lease_epoch, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				receipt.ReceiptID, receipt.JobID, receipt.AttemptID, receipt.Outcome, receipt.ResultDigest,
				receipt.JobVersion, receipt.LeaseEpoch, receipt.CreatedAt.Format(time.RFC3339Nano)); err != nil {
				return mapVendorError(err)
			}
		}
		return appendEventTx(tx, event)
	})
}

// RequestCancel implements aijobrelay.RelayStore: Queued → Cancelled
// directly; Running/Waiting → CancelRequested (cooperative).
func (s *Store) RequestCancel(ctx context.Context, jobID string) error {
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return err
	}
	if err := aijobrelay.CheckTerminalJob(job.State); err != nil {
		return errStoragef(errs.CodeConflictState, "%v", err)
	}
	toState := aijobrelay.JobCancelRequested
	if job.State == aijobrelay.JobQueued {
		toState = aijobrelay.JobCancelled
	}
	if !aijobrelay.CanTransition(job.State, toState) {
		return errStoragef(errs.CodeConflictState, "cannot cancel a job in state %s", job.State)
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, updated_at = ?
			WHERE job_id = ? AND version = ? AND state = ?`,
			toState, time.Now().UTC().Format(time.RFC3339Nano), jobID, job.Version, job.State); err != nil {
			return mapVendorError(err)
		}
		return appendEventTx(tx, events.Record{
			EventID:       "evt-" + jobID + "-" + toState + "-" + fmt.Sprint(job.Version),
			AggregateType: "job",
			AggregateID:   jobID,
			EventType:     "job." + toState,
			CorrelationID: job.CorrelationID,
			Producer:      s.instanceUUID,
		})
	})
}

// PauseJob implements aijobrelay.RelayStore: the lease is released (set to
// no-active-lease) so expiry recovery cannot resurrect the paused Job.
func (s *Store) PauseJob(ctx context.Context, jobID string) error {
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return err
	}
	if !aijobrelay.CanTransition(job.State, aijobrelay.JobPaused) {
		return errStoragef(errs.CodeConflictState, "cannot pause a job in state %s", job.State)
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, updated_at = ?
			WHERE job_id = ? AND version = ? AND state = ?`,
			aijobrelay.JobPaused, time.Now().UTC().Format(time.RFC3339Nano), jobID, job.Version, job.State); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`UPDATE attempts SET lease_expires_at = '' WHERE job_id = ? AND outcome = ?`,
			jobID, aijobrelay.AttemptRunning); err != nil {
			return mapVendorError(err)
		}
		return appendEventTx(tx, events.Record{
			EventID:       "evt-" + jobID + "-paused-" + fmt.Sprint(job.Version),
			AggregateType: "job",
			AggregateID:   jobID,
			EventType:     "job.paused",
			CorrelationID: job.CorrelationID,
			Producer:      s.instanceUUID,
		})
	})
}

// ResumeJob implements aijobrelay.RelayStore.
func (s *Store) ResumeJob(ctx context.Context, jobID string) error {
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return err
	}
	if !aijobrelay.CanTransition(job.State, aijobrelay.JobQueued) {
		return errStoragef(errs.CodeConflictState, "cannot resume a job in state %s", job.State)
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = '', updated_at = ?
			WHERE job_id = ? AND version = ? AND state = ?`,
			aijobrelay.JobQueued, time.Now().UTC().Format(time.RFC3339Nano), jobID, job.Version, job.State); err != nil {
			return mapVendorError(err)
		}
		return appendEventTx(tx, events.Record{
			EventID:       "evt-" + jobID + "-resumed-" + fmt.Sprint(job.Version),
			AggregateType: "job",
			AggregateID:   jobID,
			EventType:     "job.queued",
			CorrelationID: job.CorrelationID,
			Producer:      s.instanceUUID,
		})
	})
}

// LoadIntent implements aijobrelay.RelayStore.
func (s *Store) LoadIntent(ctx context.Context, intentID string) (aijobrelay.SideEffectIntent, error) {
	row := s.readers.QueryRowContext(ctx, `SELECT intent_id, job_id, attempt_id, operation, idempotency_key,
		input_digest, state, provider_ref, result_digest, created_at, observed_at
		FROM side_effect_intents WHERE intent_id = ?`, intentID)
	var in aijobrelay.SideEffectIntent
	var createdAt, observedAt string
	err := row.Scan(&in.IntentID, &in.JobID, &in.AttemptID, &in.Operation, &in.IdempotencyKey,
		&in.InputDigest, &in.State, &in.ProviderRef, &in.ResultDigest, &createdAt, &observedAt)
	if err == sql.ErrNoRows {
		return aijobrelay.SideEffectIntent{}, errStoragef(errs.CodeNotFound, "side-effect intent %q is not stored", intentID)
	}
	if err != nil {
		return in, mapVendorError(err)
	}
	if in.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return in, errStoragef(errs.CodeStateCorrupt, "intent created_at does not parse: %v", err)
	}
	return in, nil
}
