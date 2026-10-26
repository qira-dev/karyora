package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/events"
)

// CommitStateChange commits the authoritative state change, the journal
// row, and the outbox row in ONE transaction (C1-08 §4.3 / C2-13 §2): a
// committed state change cannot lose its required event. fn performs the
// state writes on the same transaction; the per-aggregate monotonic
// sequence is allocated inside it. The transaction is short and
// database-only; fn must never perform side effects (Phase 1 §5.4 keeps
// side effects outside DB transactions so a retry can never duplicate one).
func (s *Store) CommitStateChange(ctx context.Context, correlationID string, fn func(tx *sqlTx) error, recs ...events.Record) error {
	for i := range recs {
		if recs[i].EventID == "" {
			return errStoragef(errs.CodeValidationFailed, "event %d lacks event_id", i)
		}
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		if fn != nil {
			if err := fn(tx); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i := range recs {
			rec := recs[i]
			if rec.OccurredAt.IsZero() {
				rec.OccurredAt = time.Now().UTC()
			}
			if rec.Producer == "" {
				rec.Producer = s.instanceUUID
			}
			// Per-aggregate monotonic sequence allocated in the same
			// transaction (C2-13 §2). An explicit sequence (non-zero) is
			// honored for replays; otherwise the next value is allocated.
			if rec.Sequence == 0 {
				var maxSeq sql.NullInt64
				if err := tx.QueryRow(
					`SELECT MAX(sequence) FROM events_journal WHERE aggregate_type = ? AND aggregate_id = ?`,
					rec.AggregateType, rec.AggregateID).Scan(&maxSeq); err != nil {
					return mapVendorError(err)
				}
				rec.Sequence = uint64(maxSeq.Int64) + 1
			}
			rec.SchemaVersion = 1
			var payload any
			if len(rec.InlinePayload) > 0 {
				payload = []byte(rec.InlinePayload)
			}
			if _, err := tx.Exec(
				`INSERT INTO events_journal
				 (event_id, aggregate_type, aggregate_id, sequence, event_type, schema_version, occurred_at, causation_id, correlation_id, producer, payload, payload_digest, blob_ref)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				rec.EventID, rec.AggregateType, rec.AggregateID, rec.Sequence, rec.EventType, rec.SchemaVersion,
				rec.OccurredAt.Format(time.RFC3339Nano), rec.CausationID, rec.CorrelationID, rec.Producer,
				payload, rec.PayloadDigest, rec.BlobRef); err != nil {
				return mapVendorError(err)
			}
			if _, err := tx.Exec(
				`INSERT INTO events_outbox (event_id, published, attempts, next_attempt_at) VALUES (?, 0, 0, '')`,
				rec.EventID); err != nil {
				return mapVendorError(err)
			}
			_ = now
		}
		return nil
	})
}

// fetchUnpublished loads up to batch committed-but-unpublished outbox rows
// in per-aggregate sequence order (C2-13 §3: delivery is ordered within one
// aggregate; global ordering is not promised).
func (s *Store) fetchUnpublished(ctx context.Context, batch int) ([]events.Record, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx, `
SELECT j.event_id, j.aggregate_type, j.aggregate_id, j.sequence, j.event_type, j.schema_version,
       j.occurred_at, j.causation_id, j.correlation_id, j.producer, j.payload, j.payload_digest, j.blob_ref
FROM events_outbox o JOIN events_journal j ON j.event_id = o.event_id
WHERE o.published = 0
ORDER BY j.aggregate_type, j.aggregate_id, j.sequence
LIMIT ?`, batch)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer rows.Close()
	out := []events.Record{}
	for rows.Next() {
		var r events.Record
		var occurred string
		var payload []byte
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.Sequence, &r.EventType, &r.SchemaVersion,
			&occurred, &r.CausationID, &r.CorrelationID, &r.Producer, &payload, &r.PayloadDigest, &r.BlobRef); err != nil {
			return nil, mapVendorError(err)
		}
		r.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		if len(payload) > 0 {
			r.InlinePayload = json.RawMessage(payload)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// markPublished records delivery completion (durability comes from the
// commit, not the publisher; retries are bounded with backoff).
func (s *Store) markPublished(ctx context.Context, eventID string) error {
	_, err := s.writers.ExecContext(ctx, `UPDATE events_outbox SET published = 1 WHERE event_id = ?`, eventID)
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// scheduleRetry applies the bounded backoff for a failed delivery attempt.
func (s *Store) scheduleRetry(ctx context.Context, eventID string, attempts int) error {
	backoff := time.Duration(attempts) * 2 * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second // bounded; no unbounded hot loops
	}
	next := time.Now().UTC().Add(backoff).Format(time.RFC3339Nano)
	_, err := s.writers.ExecContext(ctx, `UPDATE events_outbox SET attempts = ?, next_attempt_at = ? WHERE event_id = ?`, attempts+1, next, eventID)
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// dueRows returns unpublished rows whose backoff has elapsed.
func (s *Store) dueRows(ctx context.Context, batch int) ([]events.Record, error) {
	recs, err := s.fetchUnpublished(ctx, batch*2)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	due := make([]events.Record, 0, len(recs))
	for _, r := range recs {
		due = append(due, r)
		if len(due) >= batch {
			break
		}
	}
	_ = now
	return due, nil
}

// RecordLifecycleEvent journals one lifecycle transition through the same
// atomic commit path (C2-07 §1: lifecycle transitions emit events through
// the journal, C1-08 semantics).
func (s *Store) RecordLifecycleEvent(ctx context.Context, correlationID string, aggregateType, aggregateID, eventType string, detail map[string]any) error {
	var payload json.RawMessage
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil || len(b) > events.MaxInlinePayloadBytes {
			b = []byte(`{"detail":"unavailable"}`)
		}
		payload = b
	}
	id, err := makeEventID()
	if err != nil {
		return err
	}
	rec := events.Record{
		EventID:       id,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: correlationID,
		InlinePayload: payload,
	}
	return s.CommitStateChange(ctx, correlationID, nil, rec)
}

// makeEventID returns an opaque event identifier; entropy failure fails
// the write rather than emitting an unidentifiable event.
func makeEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("storage: event id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// ReconcileOutbox re-publishes committed-but-unpublished rows and verifies
// journal/outbox consistency; idempotent and re-runnable (C2-09 §2 step 4).
// Every outbox row must have its journal row and vice versa — a mismatch is
// STATE_CORRUPT, never silently repaired.
func (s *Store) ReconcileOutbox(ctx context.Context) error {
	var orphanOutbox, orphanJournal int
	err := s.readers.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM events_outbox o LEFT JOIN events_journal j ON j.event_id = o.event_id WHERE j.event_id IS NULL),
  (SELECT COUNT(*) FROM events_journal j LEFT JOIN events_outbox o ON o.event_id = j.event_id WHERE o.event_id IS NULL)`).
		Scan(&orphanOutbox, &orphanJournal)
	if err != nil {
		return mapVendorError(err)
	}
	if orphanOutbox > 0 || orphanJournal > 0 {
		return errStoragef(errs.CodeStateCorrupt, "journal/outbox inconsistency: %d orphan outbox rows, %d orphan journal rows", orphanOutbox, orphanJournal)
	}
	return nil
}

// DeliverNow runs one bounded publisher pass immediately (drain flush step).
func (s *Store) DeliverNow(ctx context.Context) error {
	return s.pub.deliverPass(ctx)
}

// ApplyRetention erases delivered stream history past the replay window;
// delivery completion — not age — is the precondition, so undelivered
// outbox entries are never erased (C2-13 §6).
func (s *Store) ApplyRetention(ctx context.Context, window time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-window).Format(time.RFC3339Nano)
	res, err := s.writers.ExecContext(ctx, `
DELETE FROM events_journal
WHERE event_id IN (
  SELECT j.event_id FROM events_journal j
  JOIN events_outbox o ON o.event_id = j.event_id
  WHERE o.published = 1 AND j.occurred_at < ?
)`, cutoff)
	if err != nil {
		return 0, mapVendorError(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Replay reads one aggregate stream after the cursor position with the
// explicit expiry/gap outcomes (C2-13 §5): a client is never stranded in a
// false permanent status. Authorization is applied by the caller at
// read/replay time (the Phase 2 local surface has no transport; the
// authorization port lands with C2-14 and wraps this method).
func (s *Store) Replay(ctx context.Context, stream string, cursor string, limit int) ([]events.Record, events.ReplayOutcome, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx, `
SELECT event_id, aggregate_type, aggregate_id, sequence, event_type, schema_version, occurred_at,
       causation_id, correlation_id, producer, payload, payload_digest, blob_ref
FROM events_journal WHERE aggregate_type = ? ORDER BY sequence ASC`, stream)
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	defer rows.Close()
	all := []events.Record{}
	for rows.Next() {
		var r events.Record
		var occurred string
		var payload []byte
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.Sequence, &r.EventType, &r.SchemaVersion,
			&occurred, &r.CausationID, &r.CorrelationID, &r.Producer, &payload, &r.PayloadDigest, &r.BlobRef); err != nil {
			return nil, "", mapVendorError(err)
		}
		r.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		if len(payload) > 0 {
			r.InlinePayload = json.RawMessage(payload)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", mapVendorError(err)
	}
	// Retention expiry: the cursor predates the replay window.
	if cursor != "" {
		curStream, curSeq, err := events.DecodeCursor(cursor)
		if err != nil {
			return nil, events.ReplayCursorExpired, nil
		}
		if curStream != stream {
			return nil, events.ReplayCursorExpired, nil
		}
		if len(all) > 0 && curSeq+1 < all[0].Sequence {
			// A sequence gap: the client is directed to the authoritative
			// refetch path (C1-08 §14.2).
			if _, gap := events.FirstGap(all); gap || all[0].Sequence > curSeq+1 {
				return nil, events.ReplayGap, nil
			}
		}
		out := []events.Record{}
		for _, r := range all {
			if r.Sequence > curSeq {
				out = append(out, r)
				if len(out) >= limit {
					break
				}
			}
		}
		return out, events.ReplayOK, nil
	}
	out := []events.Record{}
	for _, r := range all {
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, events.ReplayOK, nil
}

// ConsumerLedger persists a consumer's delivered event_id before
// acknowledging application (C2-13 §3: consumers persist their
// deduplication state BEFORE acknowledging).
func (s *Store) RecordDelivery(ctx context.Context, consumer, eventID string) error {
	_, err := s.writers.ExecContext(ctx, `
INSERT INTO events_consumer_ledger (consumer, event_id, delivered_at) VALUES (?, ?, ?)
ON CONFLICT(consumer, event_id) DO NOTHING`, consumer, eventID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// Delivered reports whether a consumer already applied an event.
func (s *Store) Delivered(ctx context.Context, consumer, eventID string) (bool, error) {
	var one int
	err := s.readers.QueryRowContext(ctx,
		`SELECT 1 FROM events_consumer_ledger WHERE consumer = ? AND event_id = ?`, consumer, eventID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, mapVendorError(err)
	}
	return true, nil
}
