package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The durable provider-connection store (the owner's card-10 working
// candidate of 2026-11-02 — the P3.2 §3.3 basis, migration 0008). The
// record carries the secret REFERENCE only (the keyring Handle string —
// handle-only on disk per the accepted D-37 policy); a secret VALUE never
// enters this store, any query result, or any error. The connection-create
// idempotency is the Supplied C1-07 class (P3.2 §3.3): duplicate returns
// the existing connection reference; the same key with a different body is
// the typed CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false). The
// single-row connection_state table carries the active-connection
// reference (the minimal M-34 switch working candidate).

const connectionColumns = `connection_id, idempotency_key, provider, base_url, model_id, custom_headers, secret_ref, credential_epoch, fingerprint, tested_at, tested_digest, tested_epoch, created_at, updated_at`

func scanConnection(row rowScanner) (dprovider.ConnectionRecord, error) {
	var r dprovider.ConnectionRecord
	var headersJSON string
	err := row.Scan(&r.ConnectionID, &r.IdempotencyKey, &r.Provider, &r.BaseURL, &r.ModelID,
		&headersJSON, &r.SecretRef, &r.CredentialEpoch, &r.Fingerprint,
		&r.TestedAt, &r.TestedDigest, &r.TestedEpoch, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	if headersJSON != "" {
		if err := json.Unmarshal([]byte(headersJSON), &r.CustomHeaders); err != nil {
			return dprovider.ConnectionRecord{}, errStoragef(errs.CodeStateCorrupt, "connection %q custom headers do not parse", r.ConnectionID)
		}
	}
	return r, nil
}

func connectionByID(ctx context.Context, s *Store, id string) (dprovider.ConnectionRecord, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+connectionColumns+` FROM provider_connections WHERE connection_id = ?`, id)
	rec, err := scanConnection(row)
	if err == sql.ErrNoRows {
		return dprovider.ConnectionRecord{}, errStoragef(errs.CodeNotFound, "connection %q is not stored", id)
	}
	return rec, err
}

func connectionByIdempotencyKey(ctx context.Context, s *Store, key string) (dprovider.ConnectionRecord, bool, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+connectionColumns+` FROM provider_connections WHERE idempotency_key = ?`, key)
	rec, err := scanConnection(row)
	if err == sql.ErrNoRows {
		return dprovider.ConnectionRecord{}, false, nil
	}
	if err != nil {
		return dprovider.ConnectionRecord{}, false, err
	}
	return rec, true, nil
}

// SaveConnection implements the connection-create idempotency (the Supplied
// C1-07 class): the idempotency key is UNIQUE on the table; a stored row
// under the same key resolves by its create fingerprint — equal → the
// existing connection reference (Replayed=true, no second record, no
// second secret write); different → the typed CONFLICT_IDEMPOTENCY_KEY
// (the caller's fingerprintMatch data distinguishes the conflict). The
// caller stamps the fingerprint BEFORE calling: the credential bytes exist
// only for that computation and the SecretWriter store that follows.
func (s *Store) SaveConnection(ctx context.Context, rec dprovider.ConnectionRecord) (dprovider.ConnectionRecord, bool, error) {
	if rec.ConnectionID == "" || rec.IdempotencyKey == "" || rec.SecretRef == "" {
		return dprovider.ConnectionRecord{}, false, errStoragef(errs.CodeValidationFailed,
			"a connection record requires its Core-issued id, idempotency key, and secret reference")
	}
	if rec.Provider == "" || rec.ModelID == "" {
		return dprovider.ConnectionRecord{}, false, errStoragef(errs.CodeValidationFailed,
			"a connection record requires its provider id and exact model id")
	}
	if err := dprovider.ValidateRecord(rec); err != nil {
		return dprovider.ConnectionRecord{}, false, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	if existing, found, err := connectionByIdempotencyKey(ctx, s, rec.IdempotencyKey); err != nil {
		return dprovider.ConnectionRecord{}, false, err
	} else if found {
		return s.resolveConnectionReplay(existing, rec)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	headersJSON := ""
	if len(rec.CustomHeaders) > 0 {
		b, err := json.Marshal(rec.CustomHeaders)
		if err != nil {
			return dprovider.ConnectionRecord{}, false, errStoragef(errs.CodeValidationFailed, "connection headers are not encodable")
		}
		headersJSON = string(b)
	}
	_, err := s.writers.ExecContext(ctx, `INSERT INTO provider_connections
		(connection_id, idempotency_key, provider, base_url, model_id, custom_headers, secret_ref,
		 credential_epoch, fingerprint, tested_at, tested_digest, tested_epoch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', 0, ?, ?)`,
		rec.ConnectionID, rec.IdempotencyKey, rec.Provider, rec.BaseURL, rec.ModelID, headersJSON,
		rec.SecretRef, rec.CredentialEpoch, rec.Fingerprint, now, now)
	if err != nil {
		// The insert race: a concurrent writer stored the same key first —
		// the same replay ruling applies against the stored row.
		if existing, found, lookupErr := connectionByIdempotencyKey(ctx, s, rec.IdempotencyKey); lookupErr == nil && found {
			return s.resolveConnectionReplay(existing, rec)
		}
		return dprovider.ConnectionRecord{}, false, mapVendorError(err)
	}
	saved, err := connectionByID(ctx, s, rec.ConnectionID)
	if err != nil {
		return dprovider.ConnectionRecord{}, false, err
	}
	return saved, false, nil
}

// resolveConnectionReplay applies the Supplied provider-connection create
// idempotency against the stored row: equal fingerprint → the existing
// connection reference; different → the typed CONFLICT_IDEMPOTENCY_KEY
// with fingerprintMatch:false.
func (s *Store) resolveConnectionReplay(existing, incoming dprovider.ConnectionRecord) (dprovider.ConnectionRecord, bool, error) {
	if existing.Fingerprint == incoming.Fingerprint {
		return existing, true, nil
	}
	return dprovider.ConnectionRecord{}, false, errStoragef(errs.CodeConflictIdempotencyKey,
		"idempotency key %q was already used with a different connection body", incoming.IdempotencyKey)
}

// LoadConnection returns one connection record by its Core-issued id; an
// unknown id is the typed NOT_FOUND.
func (s *Store) LoadConnection(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error) {
	return connectionByID(ctx, s, connectionID)
}

// LoadConnectionByIdempotencyKey resolves the create-idempotency replay
// ruling's lookup: the stored row under one key, or not-found (false, nil).
func (s *Store) LoadConnectionByIdempotencyKey(ctx context.Context, key string) (dprovider.ConnectionRecord, bool, error) {
	return connectionByIdempotencyKey(ctx, s, key)
}

// Port type aliases: the list query/page shapes live in the domain
// package so the storage adapter and the use-case layer share one shape.
type (
	ListConnectionsQuery = dprovider.ListConnectionsQuery
	ListConnectionsPage  = dprovider.ListConnectionsPage
)

// ListConnections implements the server-side keyset pagination over the
// Core-defined sort (created_at descending — newest first, the Jobs-list
// order class; connection_id ascending tiebreak), one probe row beyond the
// limit to detect a further page. Fail-closed guards: a non-positive limit
// and a keyset position without its tiebreak refuse.
func (s *Store) ListConnections(ctx context.Context, q ListConnectionsQuery) (ListConnectionsPage, error) {
	if q.Limit < 1 {
		return ListConnectionsPage{}, errStoragef(errs.CodeValidationFailed, "list limit must be at least 1")
	}
	where := ""
	args := []any{}
	if q.AfterCreatedAt != "" || q.AfterConnectionID != "" {
		if q.AfterCreatedAt == "" || q.AfterConnectionID == "" {
			return ListConnectionsPage{}, errStoragef(errs.CodeValidationFailed, "a keyset position requires its connection_id tiebreak")
		}
		where = ` WHERE (created_at < ? OR (created_at = ? AND connection_id > ?))`
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterConnectionID)
	}
	args = append(args, q.Limit+1)
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT `+connectionColumns+` FROM provider_connections`+where+` ORDER BY created_at DESC, connection_id ASC LIMIT ?`, args...)
	if err != nil {
		return ListConnectionsPage{}, mapVendorError(err)
	}
	defer rows.Close()
	page := ListConnectionsPage{Connections: []dprovider.ConnectionRecord{}}
	for rows.Next() {
		rec, err := scanConnection(rows)
		if err != nil {
			return ListConnectionsPage{}, mapVendorError(err)
		}
		page.Connections = append(page.Connections, rec)
	}
	if err := rows.Err(); err != nil {
		return ListConnectionsPage{}, mapVendorError(err)
	}
	if len(page.Connections) > q.Limit {
		page.Connections = page.Connections[:q.Limit]
		page.More = true
	}
	return page, nil
}

// RecordConnectionTest stamps a SUCCESSFUL bounded test's result on the
// saved record (TC-1: the connection's tested state records the verified
// result and its time): the tested instant, the tested-at inputs digest,
// and the credential epoch the test ran under. A failed test records
// nothing (the tested state only ever records a verified result). The
// update is state-fenced on the untested-or-stale form so a concurrent
// rotation wins and the stale stamp refuses.
func (s *Store) RecordConnectionTest(ctx context.Context, connectionID, testedAt string) (dprovider.ConnectionRecord, error) {
	if connectionID == "" || testedAt == "" {
		return dprovider.ConnectionRecord{}, errStoragef(errs.CodeValidationFailed, "a tested stamp requires its connection id and instant")
	}
	// The tested digest rides the CURRENT inputs; the epoch the CURRENT
	// credential carries. Both are read here and written in one statement,
	// so the stamp can never describe a body the record no longer carries.
	current, err := connectionByID(ctx, s, connectionID)
	if err != nil {
		return dprovider.ConnectionRecord{}, err
	}
	_, err = s.writers.ExecContext(ctx, `UPDATE provider_connections
		SET tested_at = ?, tested_digest = ?, tested_epoch = ?, updated_at = ?
		WHERE connection_id = ? AND credential_epoch = ?`,
		testedAt, current.InputsDigest(), current.CredentialEpoch, time.Now().UTC().Format(time.RFC3339Nano),
		connectionID, current.CredentialEpoch)
	if err != nil {
		return dprovider.ConnectionRecord{}, mapVendorError(err)
	}
	return connectionByID(ctx, s, connectionID)
}

// BumpConnectionCredential advances the record's credential epoch (the
// rotation/re-authentication marker): the epoch bump is DURABLE BEFORE the
// caller swaps the secret value under the same handle, so a failed swap
// renders the tested state stale-truthfully (over-cautious staleness is
// the safe direction; a stale test stamp never claims currency it cannot
// prove). The caller performs the D-37 same-handle swap.
func (s *Store) BumpConnectionCredential(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error) {
	if connectionID == "" {
		return dprovider.ConnectionRecord{}, errStoragef(errs.CodeValidationFailed, "a credential bump requires its connection id")
	}
	if _, err := connectionByID(ctx, s, connectionID); err != nil {
		return dprovider.ConnectionRecord{}, err
	}
	_, err := s.writers.ExecContext(ctx, `UPDATE provider_connections
		SET credential_epoch = credential_epoch + 1, updated_at = ?
		WHERE connection_id = ?`, time.Now().UTC().Format(time.RFC3339Nano), connectionID)
	if err != nil {
		return dprovider.ConnectionRecord{}, mapVendorError(err)
	}
	return connectionByID(ctx, s, connectionID)
}

// DeleteConnection removes the record; the caller revokes the keyring
// entry first (the fail-closed order: a credential whose revoke failed
// typed is never abandoned behind a deleted record). Returns the deleted
// record so the caller knows the handle it revoked.
func (s *Store) DeleteConnection(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error) {
	rec, err := connectionByID(ctx, s, connectionID)
	if err != nil {
		return dprovider.ConnectionRecord{}, err
	}
	res, err := s.writers.ExecContext(ctx, `DELETE FROM provider_connections WHERE connection_id = ?`, connectionID)
	if err != nil {
		return dprovider.ConnectionRecord{}, mapVendorError(err)
	}
	if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
		return dprovider.ConnectionRecord{}, errStoragef(errs.CodeNotFound, "connection %q is not stored", connectionID)
	}
	// A deleted record can no longer be the active connection; the active
	// pointer clears in the same write path (the switch state is honest
	// about the record's removal).
	_, _ = s.writers.ExecContext(ctx, `UPDATE connection_state SET active_connection_id = '', updated_at = ?
		WHERE id = 1 AND active_connection_id = ?`, time.Now().UTC().Format(time.RFC3339Nano), connectionID)
	return rec, nil
}

// SetActiveConnection switches the active connection reference (the
// minimal M-34 working candidate): one durable pointer, validated against
// a stored record, idempotent on the already-active connection. No Job
// migrates and no cache mixes — the pointer is queryable state, nothing
// more (the execution path consumes it later, if ever recorded).
func (s *Store) SetActiveConnection(ctx context.Context, connectionID string) (string, error) {
	if connectionID == "" {
		return "", errStoragef(errs.CodeValidationFailed, "a switch requires its connection id")
	}
	if _, err := connectionByID(ctx, s, connectionID); err != nil {
		return "", err
	}
	_, err := s.writers.ExecContext(ctx, `UPDATE connection_state SET active_connection_id = ?, updated_at = ?
		WHERE id = 1`, connectionID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", mapVendorError(err)
	}
	return connectionID, nil
}

// ActiveConnectionID reports the stored active connection reference
// (empty when none was ever set). Unknown-id drift cannot arise: every
// setter validates the record first, and a record deletion clears the
// pointer.
func (s *Store) ActiveConnectionID(ctx context.Context) (string, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	var active string
	var updated string
	err := s.readers.QueryRowContext(cctx, `SELECT active_connection_id, updated_at FROM connection_state WHERE id = 1`).Scan(&active, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", mapVendorError(err)
	}
	return strings.TrimSpace(active), nil
}
