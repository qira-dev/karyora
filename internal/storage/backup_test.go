package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// recordCASBackedEvent writes one CAS object and references it durably from
// both a cas_references row and a journal event with a digest+blob
// reference (the committed-reference shapes the snapshot inventory
// extracts).
func recordCASBackedEvent(t *testing.T, s *Store, correlationID string, data []byte) string {
	t.Helper()
	digest, err := s.cas.Put(context.Background(), data)
	if err != nil {
		t.Fatalf("cas put: %v", err)
	}
	owner := "obj-" + strings.TrimPrefix(digest, DigestPrefix)[:12]
	err = s.CommitStateChange(context.Background(), correlationID, func(tx *sqlTx) error {
		if _, err := tx.Exec(
			`INSERT INTO cas_references (blob_ref, owner_type, owner_id, registered_at) VALUES (?, 'test', ?, ?)`,
			digest, owner, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return mapVendorError(err)
		}
		return nil
	}, events.Record{
		EventID:       owner,
		AggregateType: "test",
		AggregateID:   owner,
		EventType:     "test.blob.written",
		CorrelationID: correlationID,
		PayloadDigest: digest,
		BlobRef:       digest,
	})
	if err != nil {
		t.Fatalf("reference commit: %v", err)
	}
	return digest
}

func TestBackupSnapshotPublishedWithPins(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	d1 := recordCASBackedEvent(t, s, "c1", []byte("knowledge object one"))
	d2 := recordCASBackedEvent(t, s, "c1", []byte("knowledge object two"))

	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if manifest.ManifestVersion != ManifestVersion || manifest.BackupGeneration == "" {
		t.Fatalf("manifest identity: %+v", manifest)
	}
	// The inventory carries exactly the committed references, extracted
	// from the captured image.
	got := map[string]bool{}
	for _, o := range manifest.Objects {
		got[o.Digest] = true
	}
	if !got[d1] || !got[d2] || len(manifest.Objects) != 2 {
		t.Fatalf("inventory = %+v, want the two committed references", manifest.Objects)
	}
	// The database image digest matches the captured file.
	dir := filepath.Join(s.backupsRoot(), manifest.BackupGeneration)
	digest, err := fileDigest(filepath.Join(dir, "database.db"))
	if err != nil || digest != manifest.Database.Digest {
		t.Fatalf("captured image digest: %q vs manifest %q (%v)", digest, manifest.Database.Digest, err)
	}
	// Durable pins exist for every inventory digest, and the run is
	// published with the manifest on disk.
	for _, o := range manifest.Objects {
		var one int
		if err := s.readers.QueryRowContext(ctx,
			`SELECT 1 FROM backup_pins WHERE generation = ? AND blob_ref = ?`,
			manifest.BackupGeneration, o.Digest).Scan(&one); err != nil {
			t.Fatalf("pin for %s: %v", o.Digest, err)
		}
	}
	var state string
	if err := s.readers.QueryRowContext(ctx, `SELECT state FROM backup_runs WHERE generation = ?`, manifest.BackupGeneration).Scan(&state); err != nil || state != backupStatePublished {
		t.Fatalf("run state = %q %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("published manifest missing: %v", err)
	}
	// The journal marker reflects the captured generation.
	if manifest.JournalMarker.EventCount < 2 {
		t.Fatalf("journal marker = %+v", manifest.JournalMarker)
	}
	// Non-secret recovery references only (D-37 open): no values, no keys.
	if len(manifest.RecoveryReferences.SecretHandles) != 0 {
		t.Fatalf("Phase 2 records no secret values in the manifest: %+v", manifest.RecoveryReferences)
	}
}

func TestBackupPinsAreTheGCBarrier(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	digest := recordCASBackedEvent(t, s, "c1", []byte("pinned knowledge"))
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	// The live generation moves on: both durable references to the object
	// are aged out — the cas_references row is deleted and the journal's
	// payload reference is pruned (retention of a delivered event) — so the
	// backup pin is the ONLY remaining reachability root. Without the pin
	// the object would be collectible.
	if _, err := s.writers.ExecContext(ctx, `DELETE FROM cas_references WHERE blob_ref = ?`, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writers.ExecContext(ctx, `UPDATE events_journal SET blob_ref = '', payload_digest = '' WHERE blob_ref = ?`, digest); err != nil {
		t.Fatal(err)
	}
	// Writers keep working while the snapshot exists (no global pause).
	if err := s.RecordLifecycleEvent(ctx, "c2", AggregateCore, "live", "core.write.progressed", nil); err != nil {
		t.Fatalf("writers must continue during a snapshot: %v", err)
	}
	if _, err := s.cas.Collect(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.cas.Stat(digest); !ok {
		t.Fatal("the backup pin must bar collection (the barrier is the pin set)")
	}
	// Abort releases the pins; after the grace window the object becomes
	// collectible again (fail-safe for GC, C2-16 §3).
	s.abortBackup(ctx, "c-abort", manifest.BackupGeneration, "test abort")
	time.Sleep(10 * time.Millisecond) // GCGracePeriod is 5 ms in test options
	if _, err := s.cas.Collect(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.cas.Stat(digest); ok {
		t.Fatal("released pins must not bar collection")
	}
}

func TestBackupMissingObjectAborts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	digest := recordCASBackedEvent(t, s, "c1", []byte("will go missing"))
	// The referenced object disappears before capture verification.
	path, err := s.cas.objectPath(digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err = s.Backup(ctx, "c1")
	if err == nil {
		t.Fatal("verification must fail closed on a missing object")
	}
	// The run is aborted with pins released and staging collected.
	var state string
	if err := s.readers.QueryRowContext(ctx,
		`SELECT state FROM backup_runs WHERE state = ?`, backupStateAborted).Scan(&state); err != nil {
		t.Fatalf("aborted run must be recorded: %v", err)
	}
	var pins int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_pins`).Scan(&pins); err != nil || pins != 0 {
		t.Fatalf("abort must release all pins: %d %v", pins, err)
	}
	entries, _ := os.ReadDir(s.backupsRoot())
	if len(entries) != 0 {
		t.Fatalf("abort must collect staged artifacts, found %d", len(entries))
	}
}

func TestBackupInterruptionResolvesDeterministically(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	digest := recordCASBackedEvent(t, s, "c1", []byte("crash-window object"))
	// Simulate a crash mid-Pinning: a stale run row, partial pins, and a
	// staged directory that was never published.
	gen := "bk-crashed"
	if err := s.setBackupRunState(ctx, gen, backupStatePinning); err != nil {
		t.Fatal(err)
	}
	if err := s.pinInventory(ctx, gen, map[string]bool{digest: true}); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(s.backupsRoot(), gen)
	if err := os.MkdirAll(staged, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "database.db"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Startup reconciliation resolves: pins released, artifacts collected,
	// health event recorded (C2-16 §4).
	if err := s.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var state string
	if err := s.readers.QueryRowContext(ctx, `SELECT state FROM backup_runs WHERE generation = ?`, gen).Scan(&state); err != nil || state != backupStateAborted {
		t.Fatalf("interrupted run must resolve to aborted: %q %v", state, err)
	}
	var pins int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_pins WHERE generation = ?`, gen).Scan(&pins); err != nil || pins != 0 {
		t.Fatalf("interrupted pin set must be released: %d %v", pins, err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged artifacts must be collected: %v", err)
	}
	// A second reconcile is a no-op (idempotent).
	if err := s.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile must be idempotent: %v", err)
	}
}

func TestBackupManifestIsAtomicAndVerifiable(t *testing.T) {
	s := openTestStore(t)
	recordCASBackedEvent(t, s, "c1", []byte("manifest object"))
	manifest, err := s.Backup(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.backupsRoot(), manifest.BackupGeneration)
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed BackupManifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("published manifest must parse: %v", err)
	}
	if err := parsed.validate(); err != nil {
		t.Fatalf("published manifest must validate: %v", err)
	}
	// No staged temp artifact survives publication.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("staged manifest artifact survived: %s", e.Name())
		}
	}
}
