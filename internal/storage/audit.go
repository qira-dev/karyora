package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Audit persistence (C2-15): the durable append-only store behind the
// security package's AuditStore port. This file owns only ordered durable
// rows and the integrity-broken flag; all chain semantics (canonical form,
// hashing, verification, reserved quota) live in internal/security. The
// composition root adapts these accessors to the port.

// coreMetaAuditBroken is the durable integrity-broken health signal: while
// set, audit writes stop (C2-15 §2) until the incident process (C1-12)
// explicitly clears it.
const coreMetaAuditBroken = "audit_integrity_broken"

// AuditTail returns the current chain tail (zero when empty).
func (s *Store) AuditTail() (sequence uint64, recHash string, err error) {
	cctx, cancel := s.readerCtx(context.Background())
	defer cancel()
	var seq sql.NullInt64
	var hash string
	err = s.readers.QueryRowContext(cctx,
		`SELECT sequence, record_hash FROM audit_records ORDER BY sequence DESC LIMIT 1`).Scan(&seq, &hash)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", mapVendorError(err)
	}
	return uint64(seq.Int64), hash, nil
}

// AuditAppend durably appends one chained record. The tail conditions are
// re-checked inside the write transaction: a record that does not continue
// the current chain is refused (STATE_CORRUPT) — the chain can never fork,
// whatever the caller computed.
func (s *Store) AuditAppend(sequence uint64, prevHash, recHash string, payload []byte) error {
	if sequence == 0 {
		return errStoragef(errs.CodeValidationFailed, "audit sequence must start at 1")
	}
	if len(payload) == 0 || len(recHash) != 64 || len(prevHash) > 64 {
		return errStoragef(errs.CodeValidationFailed, "audit record shape is invalid")
	}
	return s.withWriter(context.Background(), func(tx *sqlTx) error {
		broken, err := auditBrokenLocked(tx)
		if err != nil {
			return err
		}
		if broken {
			return errStoragef(errs.CodeStateCorrupt, "audit chain integrity broken; writes stopped")
		}
		var seq sql.NullInt64
		var hash string
		err = tx.QueryRow(`SELECT sequence, record_hash FROM audit_records ORDER BY sequence DESC LIMIT 1`).Scan(&seq, &hash)
		switch {
		case err == sql.ErrNoRows:
			if sequence != 1 || prevHash != "" {
				return errStoragef(errs.CodeStateCorrupt, "audit record %d does not continue the empty chain", sequence)
			}
		case err != nil:
			return mapVendorError(err)
		default:
			if sequence != uint64(seq.Int64)+1 || prevHash != hash {
				return errStoragef(errs.CodeStateCorrupt, "audit record %d does not continue the chain at %d", sequence, seq.Int64)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO audit_records (sequence, prev_hash, record_hash, payload) VALUES (?, ?, ?, ?)`,
			sequence, prevHash, recHash, payload); err != nil {
			return mapVendorError(err)
		}
		return nil
	})
}

func auditBrokenLocked(tx *sqlTx) (bool, error) {
	var v string
	err := tx.QueryRow(`SELECT value FROM core_meta WHERE key = ?`, coreMetaAuditBroken).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, mapVendorError(err)
	}
	return v == "1", nil
}

// AuditRecordRow is one stored record (payload is the canonical form the
// security package hashed).
type AuditRecordRow struct {
	Sequence uint64
	PrevHash string
	RecHash  string
	Payload  []byte
}

// AuditRecords returns every stored record in sequence order.
func (s *Store) AuditRecords() ([]AuditRecordRow, error) {
	cctx, cancel := s.readerCtx(context.Background())
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT sequence, prev_hash, record_hash, payload FROM audit_records ORDER BY sequence ASC`)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer rows.Close()
	out := []AuditRecordRow{}
	for rows.Next() {
		var r AuditRecordRow
		var seq int64
		if err := rows.Scan(&seq, &r.PrevHash, &r.RecHash, &r.Payload); err != nil {
			return nil, mapVendorError(err)
		}
		r.Sequence = uint64(seq)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AuditAnchor stores one checkpoint anchor (idempotent per sequence).
func (s *Store) AuditAnchor(sequence uint64, hash string) error {
	if sequence == 0 || len(hash) != 64 {
		return errStoragef(errs.CodeValidationFailed, "audit anchor shape is invalid")
	}
	_, err := s.writers.ExecContext(context.Background(),
		`INSERT INTO audit_checkpoints (sequence, anchor_hash, anchored_at) VALUES (?, ?, ?)
		 ON CONFLICT(sequence) DO NOTHING`,
		sequence, hash, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// AuditAnchors returns every checkpoint anchor in sequence order.
func (s *Store) AuditAnchors() ([]AuditAnchorRow, error) {
	cctx, cancel := s.readerCtx(context.Background())
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT sequence, anchor_hash FROM audit_checkpoints ORDER BY sequence ASC`)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer rows.Close()
	out := []AuditAnchorRow{}
	for rows.Next() {
		var r AuditAnchorRow
		var seq int64
		if err := rows.Scan(&seq, &r.Hash); err != nil {
			return nil, mapVendorError(err)
		}
		r.Sequence = uint64(seq)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AuditAnchorRow is one stored checkpoint anchor.
type AuditAnchorRow struct {
	Sequence uint64
	Hash     string
}

// SetAuditIntegrityBroken marks/clears the durable integrity-broken
// signal. Clearing is an explicit incident-process action (C1-12) — this
// method exists for that path and for tests, never for automatic repair.
func (s *Store) SetAuditIntegrityBroken(broken bool) error {
	v := "0"
	if broken {
		v = "1"
	}
	_, err := s.writers.ExecContext(context.Background(),
		`INSERT INTO core_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, coreMetaAuditBroken, v)
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// AuditIntegrityBroken reports the durable signal.
func (s *Store) AuditIntegrityBroken() (bool, error) {
	cctx, cancel := s.readerCtx(context.Background())
	defer cancel()
	var v string
	err := s.readers.QueryRowContext(cctx, `SELECT value FROM core_meta WHERE key = ?`, coreMetaAuditBroken).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, mapVendorError(err)
	}
	return v == "1", nil
}
