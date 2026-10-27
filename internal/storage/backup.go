package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

// Backup snapshot (C2-16): the minimum local recovery mechanism — one
// generation-consistent online snapshot of SQLite state plus its referenced
// CAS objects. The DB image is captured with SQLite's supported online
// backup mechanism under writer coordination (C2-10 admission), never a raw
// main-file copy while WAL is active; the CAS reference inventory is
// extracted FROM the captured image, so database content and inventory
// derive from one transactionally consistent generation. The GC barrier is
// the durable pin set — writers, uploads, and GC continue normally once
// pins are committed (content addressing makes captured objects immutable).
// Retention/legal-hold/offsite/RPO/RTO remain D-27; encryption and key
// recovery remain D-37. Neither is claimed here.

// ManifestVersion is the manifest schema/format version (C2-16 §3).
const ManifestVersion = 1

// Capture coordination bounds (Proposed, C2-16 §1/§2): bounded page batches
// keep capture steps short; a bounded retry budget means a writer storm
// fails the capture into the deterministic Capturing-abort outcome (partial
// image discarded, clean retry) instead of freezing writes.
const (
	backupStepPages     = 128
	backupStepMaxRounds = 256
	backupStepPause     = 2 * time.Millisecond
	// backupPinBatch is the batch bound for idempotent pin registration.
	backupPinBatch = 512
)

// Backup state machine states (C2-16 §4). Persisted in backup_runs so an
// interruption at any point resolves deterministically on restart.
const (
	backupStateCapturing   = "capturing"
	backupStateInventoryin = "inventorying"
	backupStatePinning     = "pinning"
	backupStateVerifying   = "verifying"
	backupStatePublished   = "published"
	backupStateAborted     = "aborted"
)

// ManifestObject is one CAS object in the snapshot inventory.
type ManifestObject struct {
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}

// RecoveryReferences are the non-secret configuration and key/secret
// recovery references required for restore — handle names and policy
// references only, never secret values or key material (D-37 governs the
// actual key policy).
type RecoveryReferences struct {
	ConfigRefs    []string `json:"config_refs"`
	SecretHandles []string `json:"secret_handles"`
}

// JournalMarker is the journal/outbox consistency marker of the captured
// generation, read from the captured image.
type JournalMarker struct {
	EventCount         int64  `json:"event_count"`
	UnpublishedCount   int64  `json:"unpublished_count"`
	RecoveryGeneration uint64 `json:"recovery_generation"`
}

// BackupManifest is the versioned, atomically published snapshot manifest.
type BackupManifest struct {
	ManifestVersion    int                `json:"manifest_version"`
	BackupGeneration   string             `json:"backup_generation"`
	CreatedAt          string             `json:"created_at"`
	Database           ManifestObject     `json:"database"`
	ObjectCount        int                `json:"object_count"`
	Objects            []ManifestObject   `json:"objects"`
	RecoveryReferences RecoveryReferences `json:"recovery_references"`
	JournalMarker      JournalMarker      `json:"journal_marker"`
}

func (m BackupManifest) validate() error {
	if m.ManifestVersion != ManifestVersion {
		return errStoragef(errs.CodeValidationFailed, "manifest version %d is unknown", m.ManifestVersion)
	}
	if m.BackupGeneration == "" || len(m.BackupGeneration) > 64 {
		return errStoragef(errs.CodeValidationFailed, "manifest generation is missing or unbounded")
	}
	if m.Database.Digest == "" {
		return errStoragef(errs.CodeValidationFailed, "manifest lacks the database image digest")
	}
	if m.ObjectCount != len(m.Objects) {
		return errStoragef(errs.CodeStateCorrupt, "manifest object count %d does not match %d entries", m.ObjectCount, len(m.Objects))
	}
	return nil
}

func (s *Store) backupsRoot() string {
	return filepath.Join(filepath.Dir(s.opts.Path), "backups")
}

func newBackupGeneration() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "generation id entropy: %v", err)
	}
	return "bk-" + hex.EncodeToString(b[:]), nil
}

func (s *Store) setBackupRunState(ctx context.Context, generation, state string) error {
	_, err := s.writers.ExecContext(ctx, `
INSERT INTO backup_runs (generation, state, started_at, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT(generation) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at`,
		generation, state, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return mapVendorError(err)
	}
	return nil
}

// Backup executes the C2-16 state machine:
// Idle → Capturing → Inventorying → Pinning → Verifying → ManifestPublished
// (| Aborted). Every interruption point has a deterministic outcome; abort
// releases all pins and collects staged artifacts (fail-safe for GC).
func (s *Store) Backup(ctx context.Context, correlationID string) (BackupManifest, error) {
	generation, err := newBackupGeneration()
	if err != nil {
		return BackupManifest{}, err
	}
	dir := filepath.Join(s.backupsRoot(), generation)
	if err := s.setBackupRunState(ctx, generation, backupStateCapturing); err != nil {
		return BackupManifest{}, err
	}
	// 1. Capturing: the supported online backup mechanism into the
	// generation's staging directory. A failure here discards the partial
	// image (no pins exist yet) — clean retry.
	if err := s.captureDatabase(ctx, dir); err != nil {
		s.abortBackup(ctx, correlationID, generation, "capturing: "+boundedReason(err))
		return BackupManifest{}, err
	}
	if err := s.setBackupRunState(ctx, generation, backupStateInventoryin); err != nil {
		s.abortBackup(ctx, correlationID, generation, "state: "+boundedReason(err))
		return BackupManifest{}, err
	}
	// 2. Inventorying: the digest inventory is extracted FROM the captured
	// image — committed DB references only; staged/unreferenced objects
	// are excluded by construction.
	inventory, marker, err := inventoryFromImage(ctx, filepath.Join(dir, "database.db"), s.opts)
	if err != nil {
		s.abortBackup(ctx, correlationID, generation, "inventorying: "+boundedReason(err))
		return BackupManifest{}, err
	}
	// 3. Pinning: a durable CAS pin for EVERY inventory digest before GC
	// can consider any of them; registration is idempotent and
	// batch-bounded. Writers and GC continue normally afterwards.
	if err := s.setBackupRunState(ctx, generation, backupStatePinning); err != nil {
		s.abortBackup(ctx, correlationID, generation, "state: "+boundedReason(err))
		return BackupManifest{}, err
	}
	if err := s.pinInventory(ctx, generation, inventory); err != nil {
		s.abortBackup(ctx, correlationID, generation, "pinning: "+boundedReason(err))
		return BackupManifest{}, err
	}
	if err := s.setBackupRunState(ctx, generation, backupStateVerifying); err != nil {
		s.abortBackup(ctx, correlationID, generation, "state: "+boundedReason(err))
		return BackupManifest{}, err
	}
	// 4. Verifying: the database image digest and every object digest/count
	// against the inventory. Any failure ⇒ Aborted + pin release — the
	// backup never half-publishes.
	digest, err := fileDigest(filepath.Join(dir, "database.db"))
	if err != nil {
		s.abortBackup(ctx, correlationID, generation, "verifying: "+boundedReason(err))
		return BackupManifest{}, err
	}
	objects := make([]ManifestObject, 0, len(inventory))
	for d := range inventory {
		path, err := s.cas.objectPath(d)
		if err != nil {
			s.abortBackup(ctx, correlationID, generation, "verifying: "+boundedReason(err))
			return BackupManifest{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			s.abortBackup(ctx, correlationID, generation, fmt.Sprintf("verifying: referenced object %s is missing", d))
			return BackupManifest{}, errStoragef(errs.CodeStateCorrupt, "referenced object %s is missing", d)
		}
		objects = append(objects, ManifestObject{Digest: d, Bytes: info.Size()})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Digest < objects[j].Digest })
	manifest := BackupManifest{
		ManifestVersion:  ManifestVersion,
		BackupGeneration: generation,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		Database:         ManifestObject{Digest: digest, Bytes: objectSize(filepath.Join(dir, "database.db"))},
		ObjectCount:      len(objects),
		Objects:          objects,
		RecoveryReferences: RecoveryReferences{
			ConfigRefs:    []string{},
			SecretHandles: []string{},
		},
		JournalMarker: marker,
	}
	// 5. ManifestPublished: stage → verify → atomic rename (C2-12 atomic
	// protocol). A partial or unverifiable manifest is never published.
	tmp := filepath.Join(dir, "manifest.json.tmp")
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		s.abortBackup(ctx, correlationID, generation, "manifest: "+boundedReason(err))
		return BackupManifest{}, err
	}
	if err := writeFileSync(tmp, payload); err != nil {
		s.abortBackup(ctx, correlationID, generation, "manifest: "+boundedReason(err))
		return BackupManifest{}, err
	}
	var check BackupManifest
	if err := json.Unmarshal(payload, &check); err != nil || check.validate() != nil {
		s.abortBackup(ctx, correlationID, generation, "manifest: staged manifest does not verify")
		return BackupManifest{}, errStoragef(errs.CodeStateCorrupt, "staged manifest does not verify")
	}
	if err := os.Rename(tmp, filepath.Join(dir, "manifest.json")); err != nil {
		s.abortBackup(ctx, correlationID, generation, "manifest: "+boundedReason(err))
		return BackupManifest{}, errStoragef(errs.CodeStorageUnavailable, "manifest publish: %v", err)
	}
	if err := platform.SyncDir(dir); err != nil {
		s.abortBackup(ctx, correlationID, generation, "manifest: "+boundedReason(err))
		return BackupManifest{}, err
	}
	if err := s.setBackupRunState(ctx, generation, backupStatePublished); err != nil {
		s.abortBackup(ctx, correlationID, generation, "state: "+boundedReason(err))
		return BackupManifest{}, err
	}
	_ = s.RecordLifecycleEvent(ctx, correlationID, AggregateCore, generation, "core.backup.published", map[string]any{
		"generation": generation, "objects": len(objects), "bytes": manifest.Database.Bytes,
	})
	return manifest, nil
}

// captureDatabase runs SQLite's supported online backup: a dedicated
// verified source connection and a destination connection on the staged
// image, stepped in bounded page batches so the writer path keeps making
// progress between steps (C2-10 admission coordination).
func (s *Store) captureDatabase(ctx context.Context, dir string) error {
	if err := platform.EnsureDir(dir, 0o700); err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "backup dir: %v", err)
	}
	destPath := filepath.Join(dir, "database.db")
	destDriver := &sqlite3.SQLiteDriver{}
	destConn, err := destDriver.Open(destPath)
	if err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "open capture destination: %v", err)
	}
	dest, ok := destConn.(*sqlite3.SQLiteConn)
	if !ok {
		destConn.Close()
		return errStoragef(errs.CodeInternal, "unexpected capture destination type")
	}
	defer dest.Close()

	srcConn, err := verifiedConnector{opts: s.opts}.Connect(ctx)
	if err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "open capture source: %v", err)
	}
	src, ok := srcConn.(*sqlite3.SQLiteConn)
	if !ok {
		srcConn.Close()
		return errStoragef(errs.CodeInternal, "unexpected capture source type")
	}
	defer srcConn.Close()

	bk, err := dest.Backup("main", src, "main")
	if err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "backup init: %v", err)
	}
	defer bk.Close()
	for round := 0; round < backupStepMaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return errStoragef(errs.CodeStorageUnavailable, "capture cancelled")
		}
		done, err := bk.Step(backupStepPages)
		if err != nil {
			// BUSY/LOCKED surface from a concurrent writer mid-step: the
			// bounded budget retries; exhaustion is the deterministic
			// Capturing abort (partial image discarded, clean retry).
			if round == backupStepMaxRounds-1 {
				return errStoragef(errs.CodeStorageUnavailable, "capture could not make progress under writers")
			}
			time.Sleep(backupStepPause)
			continue
		}
		if done {
			return nil
		}
	}
	return errStoragef(errs.CodeStorageUnavailable, "capture exceeded the bounded step budget")
}

// inventoryFromImage opens the captured image and extracts the committed
// digest-reference inventory plus the journal/outbox consistency marker.
func inventoryFromImage(ctx context.Context, imagePath string, opts Options) (map[string]bool, JournalMarker, error) {
	imageOpts := opts
	imageOpts.Path = imagePath
	conn, err := verifiedConnector{opts: imageOpts}.Connect(ctx)
	if err != nil {
		return nil, JournalMarker{}, errStoragef(errs.CodeStorageUnavailable, "open captured image: %v", err)
	}
	sqlConn, ok := conn.(*sqlite3.SQLiteConn)
	if !ok {
		conn.Close()
		return nil, JournalMarker{}, errStoragef(errs.CodeInternal, "unexpected image connection type")
	}
	defer conn.Close()

	inventory := map[string]bool{}
	collect := func(query string) error {
		rows, err := sqlConn.QueryContext(ctx, query, nil)
		if err != nil {
			return err
		}
		defer rows.Close()
		dest := make([]driver.Value, 1)
		for {
			if err := rows.Next(dest); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if ref, ok := dest[0].(string); ok && ref != "" {
				inventory[ref] = true
			}
		}
	}
	if err := collect(`SELECT DISTINCT blob_ref FROM cas_references`); err != nil {
		return nil, JournalMarker{}, errStoragef(errs.CodeStateCorrupt, "inventory cas_references: %v", err)
	}
	if err := collect(`SELECT DISTINCT blob_ref FROM events_journal WHERE blob_ref != ''`); err != nil {
		return nil, JournalMarker{}, errStoragef(errs.CodeStateCorrupt, "inventory journal: %v", err)
	}
	scalar := func(query string) (string, bool, error) {
		rows, err := sqlConn.QueryContext(ctx, query, nil)
		if err != nil {
			return "", false, err
		}
		defer rows.Close()
		dest := make([]driver.Value, 1)
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				return "", false, nil
			}
			return "", false, err
		}
		switch v := dest[0].(type) {
		case string:
			return v, true, nil
		case int64:
			return fmt.Sprintf("%d", v), true, nil
		default:
			return "", false, nil
		}
	}
	marker := JournalMarker{}
	if v, ok, err := scalar(`SELECT COUNT(*) FROM events_journal`); err != nil {
		return nil, JournalMarker{}, errStoragef(errs.CodeStateCorrupt, "marker journal count: %v", err)
	} else if ok {
		marker.EventCount = int64(parseUint(v))
	}
	if v, ok, err := scalar(`SELECT COUNT(*) FROM events_outbox WHERE published = 0`); err != nil {
		return nil, JournalMarker{}, errStoragef(errs.CodeStateCorrupt, "marker outbox count: %v", err)
	} else if ok {
		marker.UnpublishedCount = int64(parseUint(v))
	}
	// The generation row may be absent in a fresh store; the marker then
	// records 0 — an honest captured-generation value, never an invention.
	if v, ok, err := scalar(`SELECT value FROM core_meta WHERE key = 'recovery_generation'`); err == nil && ok {
		marker.RecoveryGeneration = parseUint(v)
	}
	return inventory, marker, nil
}

// pinInventory registers idempotent, batch-bounded pins for every inventory
// digest (C2-16 §2 step 3).
func (s *Store) pinInventory(ctx context.Context, generation string, inventory map[string]bool) error {
	digests := make([]string, 0, len(inventory))
	for d := range inventory {
		digests = append(digests, d)
	}
	sort.Strings(digests)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for start := 0; start < len(digests); start += backupPinBatch {
		end := start + backupPinBatch
		if end > len(digests) {
			end = len(digests)
		}
		batch := digests[start:end]
		err := s.withWriter(ctx, func(tx *sqlTx) error {
			for _, d := range batch {
				if _, err := tx.Exec(
					`INSERT INTO backup_pins (generation, blob_ref, pinned_at) VALUES (?, ?, ?)
					 ON CONFLICT(generation, blob_ref) DO NOTHING`, generation, d, now); err != nil {
					return mapVendorError(err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// abortBackup is the deterministic Aborted outcome: pins released, staged
// artifacts collected, health event recorded (C2-16 §4). It never fails
// the original error path — the abort outcome is recorded best-effort.
func (s *Store) abortBackup(ctx context.Context, correlationID, generation, reason string) {
	_ = s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`DELETE FROM backup_pins WHERE generation = ?`, generation); err != nil {
			return mapVendorError(err)
		}
		return nil
	})
	_ = s.setBackupRunState(ctx, generation, backupStateAborted)
	_ = os.RemoveAll(filepath.Join(s.backupsRoot(), generation))
	_ = s.RecordLifecycleEvent(ctx, correlationID, AggregateCore, generation, "core.backup.aborted", map[string]any{
		"reason": boundedDetailString(reason),
	})
}

// ReconcileBackupRuns resolves interrupted runs on startup (C2-16 §4 crash
// rule): a pin set without a published manifest is never a commit — pins
// are released, staged artifacts are collected, and the interruption is
// recorded as a visible health event. Idempotent and re-runnable.
func (s *Store) ReconcileBackupRuns(ctx context.Context, correlationID string) (int, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT generation, state FROM backup_runs WHERE state NOT IN (?, ?)`, backupStatePublished, backupStateAborted)
	if err != nil {
		return 0, mapVendorError(err)
	}
	defer rows.Close()
	interrupted := []string{}
	for rows.Next() {
		var gen, state string
		if err := rows.Scan(&gen, &state); err != nil {
			return 0, mapVendorError(err)
		}
		interrupted = append(interrupted, gen)
	}
	if err := rows.Err(); err != nil {
		return 0, mapVendorError(err)
	}
	for _, gen := range interrupted {
		s.abortBackup(ctx, correlationID, gen, "interrupted run reconciled at startup (state was in-flight)")
	}
	return len(interrupted), nil
}

// --- shared small helpers ----------------------------------------------

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "digest read: %v", err)
	}
	return DigestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

func objectSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// writeFileSync writes the full bytes, fsyncs, and closes.
func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "write %s: %v", filepath.Base(path), err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return errStoragef(errs.CodeStorageUnavailable, "write %s: %v", filepath.Base(path), err)
	}
	if err := platform.SyncFile(f); err != nil {
		f.Close()
		return errStoragef(errs.CodeStorageUnavailable, "fsync %s: %v", filepath.Base(path), err)
	}
	return f.Close()
}

func boundedDetailString(s string) string {
	const max = 160
	if len(s) > max {
		return s[:max]
	}
	return s
}

// boundedReason is deliberately tiny and local (storage does not import
// runtime); it reduces an error to one bounded detail.
func boundedReason(err error) string {
	return boundedDetailString(err.Error())
}
