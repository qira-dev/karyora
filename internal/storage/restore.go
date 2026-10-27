package storage

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

// Controlled restore and authority fencing (C2-17): restore is a
// controlled, fail-closed operation into a swap-protected target; the
// active data directory is never overwritten in place. All five validation
// gates pass before the durable restore marker commits; the new restore
// generation fences every prior lease/authority token before dispatch
// resumes. Only accepted D-34 machines apply; Phase 2 proves the local
// D-42 slice (local live leases are governed by the Core monotonic clock —
// a restored wall-clock expiry never proves lease validity). D-27 and
// D-38 boundaries are preserved; restore enables no transfer capability.

// Restore state machine states (C2-17 §5).
const (
	restoreStateRequested = "requested"
	restoreStateStaging   = "staging"
	restoreStateValidatin = "validating"
	restoreStateMarker    = "marker-commit"
	restoreStateSwapping  = "swapping"
	restoreStateRecoverin = "recovering"
	restoreStateReady     = "ready"
)

// restoreMarkerName identifies the restored store inside a directory; the
// marker presence decides interrupted-swap resolution — never acceptance
// without it (C2-17 §3).
const restoreMarkerName = "restore.marker.json"

// RestoreMarker is the durable in-directory marker of the restored store.
type RestoreMarker struct {
	RestoreGeneration  uint64 `json:"restore_generation"`
	ManifestGeneration string `json:"manifest_generation"`
	CommittedAt        string `json:"committed_at"`
}

// RestoreOptions carries one controlled restore request.
type RestoreOptions struct {
	// ActiveDataDir is the data directory the validated store will take
	// over. It is never overwritten in place; the original directory is
	// retained untouched until post-restore acceptance and the explicit,
	// separately confirmed deletion action (C2-06).
	ActiveDataDir string
	// ManifestPath is the published backup manifest (C2-16).
	ManifestPath string
	// CoreVersion attributes the recovery replay.
	CoreVersion string
	// KeyRefChecker verifies one non-secret key/secret handle reference is
	// present and accessible (gate 5). nil with references present is a
	// typed failure — data is never silently rewritten and never falsely
	// reported as decrypted.
	KeyRefChecker func(handle string) error
	// CorrelationID threads the audit/journal events.
	CorrelationID string
}

// RestoreResult reports the deterministic successful outcome.
type RestoreResult struct {
	RestoreGeneration  uint64 `json:"restore_generation"`
	RecoveryGeneration uint64 `json:"recovery_generation"`
	ManifestGeneration string `json:"manifest_generation"`
	OriginalPath       string `json:"original_path"`
	RestoredPath       string `json:"restored_path"`
	ExtraObjects       int    `json:"extra_objects"`
}

// Restore executes the C2-17 state machine:
// Requested → Staging → Validating → MarkerCommit → Swapping → Recovering
// → Ready | Failed (original preserved). Any gate failure fails closed
// with the original data preserved; an incomplete/corrupt/incompatible
// backup never becomes "usable".
func Restore(ctx context.Context, opts RestoreOptions) (RestoreResult, error) {
	if opts.ActiveDataDir == "" || opts.ManifestPath == "" {
		return RestoreResult{}, errStoragef(errs.CodeValidationFailed, "restore requires the active data dir and manifest path")
	}
	if opts.CorrelationID == "" {
		opts.CorrelationID = "restore-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	manifestBytes, err := os.ReadFile(opts.ManifestPath)
	if err != nil {
		return RestoreResult{}, errStoragef(errs.CodeNotFound, "read manifest: %v", err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return RestoreResult{}, errStoragef(errs.CodeStateCorrupt, "manifest is not parsable")
	}
	// Gate 1: manifest accepted — format/version known, complete.
	if err := manifest.validate(); err != nil {
		return RestoreResult{}, err
	}
	manifestDir := filepath.Dir(opts.ManifestPath)
	sourceDataDir := filepath.Dir(filepath.Dir(manifestDir))
	sourceCAS := NewCAS(filepath.Join(sourceDataDir, "karyora.db.cas"), defaultRestoreObjectBytes, 0, 0)

	// Staging: the dedicated recovery directory (sibling of the active
	// data directory, 0700) — the active directory is never touched.
	stagingDir := filepath.Join(filepath.Dir(opts.ActiveDataDir),
		filepath.Base(opts.ActiveDataDir)+".restore-"+manifest.BackupGeneration)
	if err := os.RemoveAll(stagingDir); err != nil {
		return RestoreResult{}, errStoragef(errs.CodeStorageUnavailable, "clear stale staging: %v", err)
	}
	if err := platform.EnsureDir(stagingDir, 0o700); err != nil {
		return RestoreResult{}, errStoragef(errs.CodeStorageUnavailable, "staging dir: %v", err)
	}
	failed := func(err error) (RestoreResult, error) {
		// Staging/Validating failures: staged artifacts discarded, original
		// untouched, typed diagnostics (C2-17 §5).
		_ = os.RemoveAll(stagingDir)
		return RestoreResult{}, err
	}

	// Stage the database image and verify it against the manifest digest.
	imageSource := filepath.Join(manifestDir, "database.db")
	stagedDB := filepath.Join(stagingDir, "karyora.db")
	if err := copyFileVerified(imageSource, stagedDB, manifest.Database.Digest); err != nil {
		return failed(err)
	}
	// Stage every manifest object into the staged CAS: the source read
	// re-verifies the digest (a tampered source object fails here, before
	// anything is trusted) and the staged write protocol re-verifies again.
	stagedCAS := NewCAS(filepath.Join(stagingDir, "karyora.db.cas"), defaultRestoreObjectBytes, 0, 0)
	if err := stagedCAS.Init(); err != nil {
		return failed(errStoragef(errs.CodeStorageUnavailable, "staged cas init: %v", err))
	}
	for _, obj := range manifest.Objects {
		data, err := sourceCAS.Read(ctx, obj.Digest)
		if err != nil {
			return failed(errStoragef(errs.CodeStateCorrupt, "stage: manifest object %s does not verify in the source store: %v", obj.Digest, err))
		}
		if _, err := stagedCAS.Put(ctx, data); err != nil {
			return failed(errStoragef(errs.CodeStorageUnavailable, "stage: object %s: %v", obj.Digest, err))
		}
	}

	// Validating — gates 2–5 (gate 1 passed above); extras are the
	// manifest objects the restored database does not reference (reported
	// and kept per the retention policy — never silently trusted).
	extras, err := validateRestoreStaging(ctx, sourceCAS, stagedCAS, stagedDB, manifest, opts)
	if err != nil {
		return failed(err)
	}

	// MarkerCommit: the restored store durably writes the new
	// restore_generation (bumping the C2-09 recovery generation) and the
	// in-directory marker; the readback must agree — ambiguous never
	// accepts (C2-17 §3).
	marker, err := commitRestoreMarker(ctx, stagedDB, opts, manifest)
	if err != nil {
		return failed(err)
	}

	// Swapping: the atomic directory-role swap. An interrupted swap
	// resolves deterministically via the marker (ResolveRestoreSwap).
	formerPath := fmt.Sprintf("%s.former-%d", opts.ActiveDataDir, time.Now().Unix())
	if err := os.Rename(opts.ActiveDataDir, formerPath); err != nil {
		_ = os.RemoveAll(stagingDir)
		return RestoreResult{}, errStoragef(errs.CodeStorageUnavailable, "swap stage 1: %v", err)
	}
	if err := os.Rename(stagingDir, opts.ActiveDataDir); err != nil {
		// Deterministic state: former exists, staging (with marker) exists.
		return RestoreResult{}, errStoragef(errs.CodeStorageUnavailable, "swap stage 2: %v", err)
	}

	// Recovering: the C2-09 startup replay against the swapped store —
	// migrations, reconciliation, a fresh generation bump, and the fence
	// of every prior authority token before dispatch resumes.
	recoveryGen, err := replayRecovery(ctx, opts.ActiveDataDir, opts.CoreVersion, opts.CorrelationID, marker.RestoreGeneration)
	if err != nil {
		// The original is retained; the marker identifies the restored
		// store; the next start resolves via the C2-09 path.
		return RestoreResult{}, err
	}
	return RestoreResult{
		RestoreGeneration:  marker.RestoreGeneration,
		RecoveryGeneration: recoveryGen,
		ManifestGeneration: manifest.BackupGeneration,
		OriginalPath:       formerPath,
		RestoredPath:       opts.ActiveDataDir,
		ExtraObjects:       extras,
	}, nil
}

// defaultRestoreObjectBytes bounds object handling in the restore path
// (Proposed 256 MiB, matching the CAS default bound).
const defaultRestoreObjectBytes = 268435456

// s_restoreValidate runs gates 2–5 on the staged store; every failure is a
// typed, actionable diagnostic naming the gate and the object/key.
func validateRestoreStaging(ctx context.Context, sourceCAS, stagedCAS *CAS, stagedDB string, manifest BackupManifest, opts RestoreOptions) (int, error) {
	// Gate 3: every manifest object's digest and count re-verified in the
	// staged store.
	for _, obj := range manifest.Objects {
		data, err := stagedCAS.Read(ctx, obj.Digest)
		if err != nil {
			return 0, errStoragef(errs.CodeStateCorrupt, "gate CAS: manifest object %s does not verify in the staged store", obj.Digest)
		}
		if int64(len(data)) != obj.Bytes {
			return 0, errStoragef(errs.CodeStateCorrupt, "gate CAS: object %s size %d does not match the manifest (%d)", obj.Digest, len(data), obj.Bytes)
		}
	}
	// Also confirm the SOURCE still holds every object (the copy path).
	for _, obj := range manifest.Objects {
		if ok, err := sourceCAS.Stat(obj.Digest); err != nil || !ok {
			return 0, errStoragef(errs.CodeStateCorrupt, "gate CAS: manifest object %s is missing from the source store", obj.Digest)
		}
	}

	// Gate 2: SQLite integrity_check AND foreign_key_check on the staged
	// image.
	conn, err := openValidatedConnection(ctx, stagedDB)
	if err != nil {
		return 0, errStoragef(errs.CodeStateCorrupt, "gate DB: %v", err)
	}
	defer conn.Close()
	sc, ok := conn.(*sqlite3.SQLiteConn)
	if !ok {
		return 0, errStoragef(errs.CodeInternal, "gate DB: unexpected connection type")
	}
	if err := pragmaCheck(ctx, sc, "PRAGMA integrity_check", "ok"); err != nil {
		return 0, errStoragef(errs.CodeStateCorrupt, "gate DB: %v", err)
	}
	if err := pragmaNoRows(ctx, sc, "PRAGMA foreign_key_check"); err != nil {
		return 0, errStoragef(errs.CodeStateCorrupt, "gate DB: foreign_key_check reported violations")
	}

	// Gate 4: two-way cross-check — every digest referenced by the
	// restored database exists in the manifest AND verifies in the staged
	// CAS; every manifest object is accounted for. Extra unreferenced
	// manifest objects are reported and kept, never silently trusted or
	// imported as references.
	referenced := map[string]bool{}
	collect := func(query string) error {
		rows, err := sc.QueryContext(ctx, query, nil)
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
				referenced[ref] = true
			}
		}
	}
	if err := collect(`SELECT DISTINCT blob_ref FROM cas_references`); err != nil {
		return 0, errStoragef(errs.CodeStateCorrupt, "gate cross-check: cas_references: %v", err)
	}
	if err := collect(`SELECT DISTINCT blob_ref FROM events_journal WHERE blob_ref != ''`); err != nil {
		return 0, errStoragef(errs.CodeStateCorrupt, "gate cross-check: journal: %v", err)
	}
	inManifest := map[string]bool{}
	for _, obj := range manifest.Objects {
		inManifest[obj.Digest] = true
	}
	for ref := range referenced {
		if !inManifest[ref] {
			return 0, errStoragef(errs.CodeStateCorrupt, "gate cross-check: database references %s which the manifest does not carry", ref)
		}
		if _, err := stagedCAS.Read(ctx, ref); err != nil {
			return 0, errStoragef(errs.CodeStateCorrupt, "gate cross-check: referenced object %s fails verification", ref)
		}
	}
	extras := 0
	for _, obj := range manifest.Objects {
		if !referenced[obj.Digest] {
			extras++
		}
	}

	// Gate 5: configuration/key references present AND accessible. A
	// missing/inaccessible key reference is an explicit typed failure —
	// the honest encrypted-but-unreadable state is surfaced, never a
	// silent rewrite or a false decryption claim.
	refs := manifest.RecoveryReferences
	if len(refs.SecretHandles) > 0 && opts.KeyRefChecker == nil {
		return 0, errStoragef(errs.CodeRecoveryRequired, "gate keys: %d key/secret handle reference(s) are recorded but no resolution path is wired; the restored data stays encrypted-but-unreadable until the documented recovery path runs", len(refs.SecretHandles))
	}
	for _, handle := range refs.SecretHandles {
		if err := opts.KeyRefChecker(handle); err != nil {
			return 0, errStoragef(errs.CodeRecoveryRequired, "gate keys: handle reference %q is not accessible: %v", handle, err)
		}
	}
	return extras, nil
}

// commitRestoreMarker bumps the recovery generation inside the staged
// store, writes restore_generation + the in-directory marker, and verifies
// the readback agreement.
func commitRestoreMarker(ctx context.Context, stagedDB string, opts RestoreOptions, manifest BackupManifest) (RestoreMarker, error) {
	conn, err := openValidatedConnection(ctx, stagedDB)
	if err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeStateCorrupt, "marker: %v", err)
	}
	sc, ok := conn.(*sqlite3.SQLiteConn)
	if !ok {
		conn.Close()
		return RestoreMarker{}, errStoragef(errs.CodeInternal, "marker: unexpected connection type")
	}
	defer conn.Close()
	if err := execStmt(ctx, sc, "BEGIN IMMEDIATE"); err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker: %v", err)
	}
	rollback := func() { _ = execStmt(ctx, sc, "ROLLBACK") }
	// Ensure the generation row exists (a store that never completed a
	// startup bump starts from 0), then increment inside the transaction.
	if err := execStmt(ctx, sc, `INSERT INTO core_meta (key, value) VALUES ('recovery_generation', '0')
		ON CONFLICT(key) DO NOTHING`); err != nil {
		rollback()
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker: %v", err)
	}
	var gen int64
	if err := queryScalar(ctx, sc, `SELECT value FROM core_meta WHERE key = 'recovery_generation'`, &gen); err != nil {
		rollback()
		return RestoreMarker{}, errStoragef(errs.CodeStateCorrupt, "marker: recovery generation unreadable: %v", err)
	}
	newGen := uint64(gen) + 1
	if err := execStmt(ctx, sc, `INSERT INTO core_meta (key, value) VALUES ('restore_generation', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprintf("%d", newGen)); err != nil {
		rollback()
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker: %v", err)
	}
	if err := execStmt(ctx, sc, `UPDATE core_meta SET value = ? WHERE key = 'recovery_generation'`, fmt.Sprintf("%d", newGen)); err != nil {
		rollback()
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker: %v", err)
	}
	if err := execStmt(ctx, sc, "COMMIT"); err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker: %v", err)
	}
	// Fold the WAL into the image and make the marker durable.
	if err := execStmt(ctx, sc, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker checkpoint: %v", err)
	}
	marker := RestoreMarker{
		RestoreGeneration:  newGen,
		ManifestGeneration: manifest.BackupGeneration,
		CommittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	stagingDir := filepath.Dir(stagedDB)
	payload, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeInternal, "marker: %v", err)
	}
	if err := writeFileSync(filepath.Join(stagingDir, restoreMarkerName), payload); err != nil {
		return RestoreMarker{}, err
	}
	if err := platform.SyncDir(stagingDir); err != nil {
		return RestoreMarker{}, errStoragef(errs.CodeStorageUnavailable, "marker dir fsync: %v", err)
	}
	return marker, nil
}

// replayRecovery runs the C2-09 startup replay against the swapped
// directory and fences every prior authority token with the fresh
// generation (local D-42 slice: no lease survives the process boundary, so
// the quarantine list is empty by construction — recorded, never silent).
func replayRecovery(ctx context.Context, dataDir, coreVersion, correlationID string, restoreGen uint64) (uint64, error) {
	opts := Options{
		Path:                   filepath.Join(dataDir, "karyora.db"),
		PoolMaxOpen:            2,
		WriterQueueDepth:       4,
		WriterDeadline:         10 * time.Second,
		BusyTimeout:            2 * time.Second,
		WALAutocheckpointPages: 1000,
		Synchronous:            "FULL",
		ReadBudget:             5 * time.Second,
		MaxObjectBytes:         defaultRestoreObjectBytes,
		GCGracePeriod:          24 * time.Hour,
		PublishInterval:        time.Second,
		PublishBatch:           256,
		Retention:              30 * 24 * time.Hour,
	}
	s, err := Connect(opts)
	if err != nil {
		return 0, errStoragef(errs.CodeStorageUnavailable, "recovering: open restored store: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(ctx, coreVersion); err != nil {
		return 0, errStoragef(errs.CodeMigrationFailed, "recovering: %v", err)
	}
	if err := s.Reconcile(ctx); err != nil {
		return 0, errStoragef(errs.CodeRecoveryRequired, "recovering: %v", err)
	}
	gen, err := s.BumpGeneration(ctx)
	if err != nil {
		return 0, errStoragef(errs.CodeRecoveryRequired, "recovering: %v", err)
	}
	if gen <= restoreGen {
		return 0, errStoragef(errs.CodeStateCorrupt, "recovering: generation %d does not fence the restore generation %d", gen, restoreGen)
	}
	// Authority fencing: the new generation fences every prior
	// lease/authority token before dispatch resumes (C2-17 §3); Phase 2
	// has no Job/Attempt tables (D-12/D-33), so the quarantine list is
	// empty by construction and recorded, never silently skipped.
	if err := s.FencePrior(ctx, correlationID, gen); err != nil {
		return 0, errStoragef(errs.CodeRecoveryRequired, "recovering: %v", err)
	}
	_ = s.RecordLifecycleEvent(ctx, correlationID, AggregateCore, s.instanceUUID, "core.restore.completed", map[string]any{
		"restore_generation":  restoreGen,
		"recovery_generation": gen,
		"quarantined":         0,
	})
	return gen, nil
}

// SwapOutcome is the deterministic result of resolving an interrupted swap.
type SwapOutcome string

const (
	SwapCompleted     SwapOutcome = "completed"      // marker present in the active dir
	SwapRolledForward SwapOutcome = "rolled_forward" // interrupted between renames; completed now
	SwapNotStarted    SwapOutcome = "not_started"    // original in place, no marker
	SwapAmbiguous     SwapOutcome = "ambiguous"      // recovery/quarantine path, never acceptance
)

// ResolveRestoreSwap resolves an interrupted swap deterministically to one
// directory with the marker identifying which (C2-17 §1/§5). It never
// accepts an ambiguous state.
func ResolveRestoreSwap(activeDataDir string) (SwapOutcome, error) {
	markerPath := filepath.Join(activeDataDir, restoreMarkerName)
	if _, err := os.Stat(markerPath); err == nil {
		return SwapCompleted, nil
	} else if !os.IsNotExist(err) {
		return SwapAmbiguous, errStoragef(errs.CodeStorageUnavailable, "swap resolve: %v", err)
	}
	// Active dir exists without a marker: either the swap never started
	// (original in place) or a foreign directory holds the role.
	if _, err := os.Stat(activeDataDir); err == nil {
		return SwapNotStarted, nil
	}
	// Active missing: an interrupted swap between the two renames.
	base := filepath.Base(activeDataDir)
	parent := filepath.Dir(activeDataDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return SwapAmbiguous, errStoragef(errs.CodeStorageUnavailable, "swap resolve: %v", err)
	}
	var staged, former string
	for _, e := range entries {
		name := e.Name()
		candidate := filepath.Join(parent, name)
		switch {
		case strings.HasPrefix(name, base+".restore-"):
			if _, err := os.Stat(filepath.Join(candidate, restoreMarkerName)); err == nil {
				staged = candidate
			}
		case strings.HasPrefix(name, base+".former-"):
			former = candidate
		}
	}
	if staged != "" && former != "" {
		// Deterministic roll-forward: the validated store (marker present)
		// takes the active role; the original stays retained as former.
		if err := os.Rename(staged, activeDataDir); err != nil {
			return SwapAmbiguous, errStoragef(errs.CodeStorageUnavailable, "swap roll-forward: %v", err)
		}
		return SwapRolledForward, nil
	}
	if staged == "" && former != "" {
		// The staged store vanished between renames: ambiguous — the
		// recovery/quarantine path decides, never acceptance.
		return SwapAmbiguous, errStoragef(errs.CodeRecoveryRequired, "swap resolve: the staged restored store is missing while the original sits aside; recovery path required")
	}
	return SwapAmbiguous, errStoragef(errs.CodeRecoveryRequired, "swap resolve: no marker and no original present")
}

// openValidatedConnection opens one connection with the required option
// matrix (WAL, foreign_keys, trusted_schema off) to an arbitrary path.
func openValidatedConnection(ctx context.Context, path string) (driver.Conn, error) {
	return verifiedConnector{opts: Options{
		Path:                   path,
		BusyTimeout:            2 * time.Second,
		WALAutocheckpointPages: 1000,
		Synchronous:            "FULL",
	}}.Connect(ctx)
}

// pragmaCheck runs a single-value pragma and compares it with want.
func pragmaCheck(ctx context.Context, conn *sqlite3.SQLiteConn, query, want string) error {
	rows, err := conn.QueryContext(ctx, query, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", query, err)
	}
	defer rows.Close()
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		return fmt.Errorf("%s: %w", query, err)
	}
	got, _ := dest[0].(string)
	if got != want {
		return fmt.Errorf("%s reported %q", query, got)
	}
	return nil
}

// pragmaNoRows runs a pragma whose result must be empty (no violations).
func pragmaNoRows(ctx context.Context, conn *sqlite3.SQLiteConn, query string) error {
	rows, err := conn.QueryContext(ctx, query, nil)
	if err != nil {
		return err
	}
	defer rows.Close()
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s returned rows", query)
		}
		return err
	}
	return nil
}

func execStmt(ctx context.Context, conn *sqlite3.SQLiteConn, query string, args ...any) error {
	na := make([]driver.NamedValue, len(args))
	for i, a := range args {
		na[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	_, err := conn.ExecContext(ctx, query, na)
	return err
}

func queryScalar(ctx context.Context, conn *sqlite3.SQLiteConn, query string, dest *int64) error {
	rows, err := conn.QueryContext(ctx, query, nil)
	if err != nil {
		return err
	}
	defer rows.Close()
	d := make([]driver.Value, 1)
	if err := rows.Next(d); err != nil {
		return err
	}
	switch v := d[0].(type) {
	case int64:
		*dest = v
	case string:
		*dest = int64(parseUint(v))
	default:
		return fmt.Errorf("unexpected scalar type %T", d[0])
	}
	return nil
}

// copyFileVerified copies src to dst and verifies the SHA-256 digest.
func copyFileVerified(src, dst, wantDigest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errStoragef(errs.CodeStorageUnavailable, "stage image: %v", err)
	}
	sum := sha256.Sum256(data)
	got := DigestPrefix + hex.EncodeToString(sum[:])
	if got != wantDigest {
		return errStoragef(errs.CodeStateCorrupt, "staged image digest %s does not match the manifest (%s)", got, wantDigest)
	}
	if err := writeFileSync(dst, data); err != nil {
		return err
	}
	return platform.SyncDir(filepath.Dir(dst))
}
