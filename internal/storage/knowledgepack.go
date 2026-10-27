package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/knowledgepack"
)

// The Store satisfies the knowledge pack persistence port (C3-06): the
// domain package owns the contract; SQLite mechanics live here per the
// dependency map. Saves are atomic per version; (pack_id, version) is
// immutable — a re-save is a conflict, never an overwrite.
var _ knowledgepack.StorePort = (*Store)(nil)

// SavePackVersion implements knowledgepack.StorePort.
func (s *Store) SavePackVersion(ctx context.Context, pack knowledgepack.StoredPack) error {
	if err := pack.ValidateStored(); err != nil {
		return errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	manifestJSON, err := json.Marshal(pack.Manifest)
	if err != nil {
		return errStoragef(errs.CodeValidationFailed, "manifest does not marshal: %v", err)
	}
	if len(manifestJSON) > knowledgepack.MaxManifestBytes {
		return errStoragef(errs.CodeValidationFailed, "manifest of %d bytes exceeds the bound %d", len(manifestJSON), knowledgepack.MaxManifestBytes)
	}
	root, err := knowledgepack.PackRootDigest(pack.Manifest)
	if err != nil {
		return errStoragef(errs.CodeValidationFailed, "root digest: %v", err)
	}
	if pack.RootDigest != "" && pack.RootDigest != root {
		return errStoragef(errs.CodeConflictState, "stored root digest %q does not match the manifest root %q", pack.RootDigest, root)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriter(ctx, func(tx *sqlTx) error {
		// Immutability check first: an existing version is a conflict.
		var one int
		err := tx.QueryRow(`SELECT 1 FROM kp_versions WHERE pack_id = ? AND version = ?`,
			pack.Manifest.PackID, pack.Manifest.PackVersion).Scan(&one)
		if err == nil {
			return errStoragef(errs.CodeConflictState, "pack %q version %d already exists; versions are immutable", pack.Manifest.PackID, pack.Manifest.PackVersion)
		}
		if err != sql.ErrNoRows {
			return mapVendorError(err)
		}
		var exists int
		err = tx.QueryRow(`SELECT 1 FROM kp_packs WHERE pack_id = ?`, pack.Manifest.PackID).Scan(&exists)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(`INSERT INTO kp_packs (pack_id, current_version, created_at) VALUES (?, ?, ?)`,
				pack.Manifest.PackID, pack.Manifest.PackVersion, now); err != nil {
				return mapVendorError(err)
			}
		} else if err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`UPDATE kp_packs SET current_version = MAX(current_version, ?) WHERE pack_id = ?`,
			pack.Manifest.PackVersion, pack.Manifest.PackID); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`INSERT INTO kp_versions
			(pack_id, version, root_digest, manifest_json, trust_state, license, license_status, freshness, status, status_reason, stored_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			pack.Manifest.PackID, pack.Manifest.PackVersion, root, manifestJSON,
			pack.Manifest.Trust.State, pack.Manifest.License, pack.Manifest.LicenseStatus,
			knowledgepack.FreshnessUnknown, pack.Status, pack.StatusReason, now); err != nil {
			return mapVendorError(err)
		}
		for i := range pack.Manifest.Entries {
			e := &pack.Manifest.Entries[i]
			data, ok := pack.Entries[e.Path]
			if !ok {
				return errStoragef(errs.CodeValidationFailed, "entry %q bytes are missing at save", e.Path)
			}
			if _, err := tx.Exec(`INSERT INTO kp_entries
				(pack_id, version, entry_path, content, content_digest, content_type, size_bytes)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				pack.Manifest.PackID, pack.Manifest.PackVersion, e.Path, data, e.Digest, e.ContentType, e.SizeBytes); err != nil {
				return mapVendorError(err)
			}
		}
		return nil
	})
}

// LoadPackVersion implements knowledgepack.StorePort.
func (s *Store) LoadPackVersion(ctx context.Context, packID string, version int) (knowledgepack.StoredPack, error) {
	var rootDigest string
	var manifestJSON []byte
	var status, statusReason, storedAt string
	err := s.readers.QueryRowContext(ctx, `
		SELECT root_digest, manifest_json, status, status_reason, stored_at
		FROM kp_versions WHERE pack_id = ? AND version = ?`, packID, version).
		Scan(&rootDigest, &manifestJSON, &status, &statusReason, &storedAt)
	if err == sql.ErrNoRows {
		return knowledgepack.StoredPack{}, errStoragef(errs.CodeNotFound, "pack %q version %d is not stored", packID, version)
	}
	if err != nil {
		return knowledgepack.StoredPack{}, mapVendorError(err)
	}
	var m knowledgepack.Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return knowledgepack.StoredPack{}, errStoragef(errs.CodeStateCorrupt, "stored manifest of pack %q v%d does not decode: %v", packID, version, err)
	}
	entries := make(map[string][]byte)
	eRows, err := s.readers.QueryContext(ctx, `
		SELECT entry_path, content FROM kp_entries WHERE pack_id = ? AND version = ? ORDER BY entry_path`, packID, version)
	if err != nil {
		return knowledgepack.StoredPack{}, mapVendorError(err)
	}
	defer func() { _ = eRows.Close() }()
	for eRows.Next() {
		var path string
		var content []byte
		if err := eRows.Scan(&path, &content); err != nil {
			return knowledgepack.StoredPack{}, mapVendorError(err)
		}
		entries[path] = content
	}
	if err := eRows.Err(); err != nil {
		return knowledgepack.StoredPack{}, mapVendorError(err)
	}
	parsedAt, err := time.Parse(time.RFC3339Nano, storedAt)
	if err != nil {
		return knowledgepack.StoredPack{}, errStoragef(errs.CodeStateCorrupt, "stored_at of pack %q v%d does not parse: %v", packID, version, err)
	}
	return knowledgepack.StoredPack{
		Manifest:     &m,
		RootDigest:   rootDigest,
		Status:       status,
		StatusReason: statusReason,
		StoredAt:     parsedAt,
		Entries:      entries,
	}, nil
}

// LatestPackVersion implements knowledgepack.StorePort.
func (s *Store) LatestPackVersion(ctx context.Context, packID string) (int, error) {
	var maxVer sql.NullInt64
	err := s.readers.QueryRowContext(ctx,
		`SELECT MAX(version) FROM kp_versions WHERE pack_id = ?`, packID).Scan(&maxVer)
	if err != nil {
		return 0, mapVendorError(err)
	}
	return int(maxVer.Int64), nil
}

// SetPackStatus implements knowledgepack.StorePort: only the disposition
// moves; manifest and entries are untouched.
func (s *Store) SetPackStatus(ctx context.Context, packID string, version int, status, reason string) error {
	switch status {
	case knowledgepack.PackStatusActive, knowledgepack.PackStatusQuarantined, knowledgepack.PackStatusRejected:
	default:
		return errStoragef(errs.CodeValidationFailed, "illegal pack status %q", status)
	}
	if status != knowledgepack.PackStatusActive && reason == "" {
		return errStoragef(errs.CodeValidationFailed, "a %s pack requires a visible reason", status)
	}
	res, err := s.writers.ExecContext(ctx,
		`UPDATE kp_versions SET status = ?, status_reason = ? WHERE pack_id = ? AND version = ?`,
		status, reason, packID, version)
	if err != nil {
		return mapVendorError(err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errStoragef(errs.CodeNotFound, "pack %q version %d is not stored", packID, version)
	}
	return nil
}

// ListPackVersions implements knowledgepack.StorePort.
func (s *Store) ListPackVersions(ctx context.Context, packID string) ([]int, error) {
	rows, err := s.readers.QueryContext(ctx,
		`SELECT version FROM kp_versions WHERE pack_id = ? ORDER BY version ASC`, packID)
	if err != nil {
		return nil, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, mapVendorError(err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
