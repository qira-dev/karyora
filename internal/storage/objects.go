package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/events"
)

// Object and journal-read ports for the vertical slice (C3-31): job inputs
// and results live in the CAS (C2-12) behind durable reference rows, journal
// events are inspectable per aggregate (C2-13), and Waiting backoff jobs
// requeue through the accepted D-34 transition Waiting → Queued.

// PutObject stores one durable object through the C2-12 staged-write
// protocol and registers the reference row so the GC barrier holds it for
// the owner. The returned digest is the caller's content address.
func (s *Store) PutObject(ctx context.Context, data []byte, ownerType, ownerID string) (string, error) {
	if ownerType == "" || ownerID == "" {
		return "", errStoragef(errs.CodeValidationFailed, "object owner type and id are required")
	}
	digest, err := s.cas.Put(ctx, data)
	if err != nil {
		return "", err
	}
	err = s.withWriter(ctx, func(tx *sqlTx) error {
		_, err := tx.Exec(`INSERT OR IGNORE INTO cas_references
			(blob_ref, owner_type, owner_id, registered_at) VALUES (?, ?, ?, ?)`,
			digest, ownerType, ownerID, time.Now().UTC().Format(time.RFC3339Nano))
		return mapVendorError(err)
	})
	if err != nil {
		return "", err
	}
	return digest, nil
}

// ReadObject reads one stored object back through the fail-closed C2-12
// read path (digest re-verified; missing/corrupt surfaces STATE_CORRUPT).
func (s *Store) ReadObject(ctx context.Context, digest string) ([]byte, error) {
	return s.cas.Read(ctx, digest)
}

// JobEvents reads one Job's journal events in per-aggregate sequence order
// (C2-13: per-aggregate ordering is the only ordering guarantee).
func (s *Store) JobEvents(ctx context.Context, jobID string) ([]events.Record, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT event_id, aggregate_type, aggregate_id, sequence,
		event_type, schema_version, occurred_at, causation_id, correlation_id, producer, payload, payload_digest, blob_ref
		FROM events_journal WHERE aggregate_type = 'job' AND aggregate_id = ? ORDER BY sequence ASC`, jobID)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer rows.Close()
	var out []events.Record
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
	if err := rows.Err(); err != nil {
		return nil, mapVendorError(err)
	}
	return out, nil
}

// RequeueBackoff moves every Waiting job whose typed reason is
// backoff_scheduled back to Queued so the scheduler can claim it again
// (the accepted D-34 transition Waiting → Queued). The bumped Job version
// fences any stale writer. Other Waiting reasons stay put — they belong to
// recovery or reconciliation, not to the backoff clock.
func (s *Store) RequeueBackoff(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT job_id, version, correlation_id FROM jobs
		WHERE state = ? AND waiting_reason = ? ORDER BY created_at ASC`, aijobrelay.JobWaiting, string(aijobrelay.WaitBackoffScheduled))
	if err != nil {
		return nil, mapVendorError(err)
	}
	type candidate struct {
		id      string
		version uint64
		corrID  string
	}
	var batch []candidate
	for rows.Next() {
		var c candidate
		var versionStr string
		if err := rows.Scan(&c.id, &versionStr, &c.corrID); err != nil {
			rows.Close()
			return nil, mapVendorError(err)
		}
		c.version = parseUint(versionStr)
		batch = append(batch, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, mapVendorError(err)
	}
	rows.Close()

	var requeued []string
	for _, c := range batch {
		err := s.withWriter(ctx, func(tx *sqlTx) error {
			res, err := tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = '', updated_at = ?
				WHERE job_id = ? AND version = ? AND state = ?`,
				aijobrelay.JobQueued, now.UTC().Format(time.RFC3339Nano), c.id, c.version, aijobrelay.JobWaiting)
			if err != nil {
				return mapVendorError(err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return nil // raced with another authority; not an error
			}
			return appendEventTx(tx, events.Record{
				EventID:       "evt-" + c.id + "-requeued-backoff-" + parseUintToString(c.version+1),
				AggregateType: "job",
				AggregateID:   c.id,
				EventType:     "job.requeued",
				CorrelationID: c.corrID,
				Producer:      s.instanceUUID,
				InlinePayload: mustJSON(map[string]any{"reason": string(aijobrelay.WaitBackoffScheduled)}),
			})
		})
		if err != nil {
			return requeued, err
		}
		requeued = append(requeued, c.id)
	}
	return requeued, nil
}

func parseUintToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
