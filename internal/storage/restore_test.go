package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildSnapshot makes a store with two CAS-backed objects and returns the
// store and its data directory.
func buildSnapshot(t *testing.T) (*Store, string) {
	t.Helper()
	s := openTestStore(t)
	recordCASBackedEvent(t, s, "c1", []byte("restorable object A"))
	recordCASBackedEvent(t, s, "c1", []byte("restorable object B"))
	return s, filepath.Dir(s.opts.Path)
}

func TestRestoreRoundTripFencesAndPreservesOriginal(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	// The live generation moves on after capture: the restored store must
	// NOT contain this event.
	if err := s.RecordLifecycleEvent(ctx, "c2", AggregateCore, "live", "core.after.snapshot", nil); err != nil {
		t.Fatal(err)
	}
	s.Close()

	result, err := Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir,
		ManifestPath:  filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json"),
		CoreVersion:   "test",
		CorrelationID: "c-restore",
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	// The restored store carries exactly the captured generation.
	reopened, err := Open(ctx, testOptionsAt(dataDir))
	if err != nil {
		t.Fatalf("reopen restored store: %v", err)
	}
	defer reopened.Close()
	var captured int64
	if err := reopened.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'test.blob.written'`).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if captured != 2 {
		t.Fatalf("restored journal carries %d captured events, want 2", captured)
	}
	var postSnapshot int64
	if err := reopened.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'core.after.snapshot'`).Scan(&postSnapshot); err != nil || postSnapshot != 0 {
		t.Fatalf("the restored store must reflect the captured generation, not the live one: %d %v", postSnapshot, err)
	}
	// Authority fencing: the marker and the recovery generation were
	// durably committed, and the replay bumped beyond the restore
	// generation (C2-17 §3).
	var restoreGen, recoveryGen string
	if err := reopened.readers.QueryRowContext(ctx, `SELECT value FROM core_meta WHERE key = 'restore_generation'`).Scan(&restoreGen); err != nil {
		t.Fatalf("restore_generation missing: %v", err)
	}
	if err := reopened.readers.QueryRowContext(ctx, `SELECT value FROM core_meta WHERE key = 'recovery_generation'`).Scan(&recoveryGen); err != nil {
		t.Fatal(err)
	}
	if parseUint(restoreGen) != result.RestoreGeneration || parseUint(recoveryGen) <= result.RestoreGeneration {
		t.Fatalf("fencing: restore=%s recovery=%s result=%+v", restoreGen, recoveryGen, result)
	}
	// The marker file identifies the restored store.
	if _, err := os.Stat(filepath.Join(dataDir, restoreMarkerName)); err != nil {
		t.Fatalf("restore marker missing: %v", err)
	}
	// The original directory is retained untouched (deletion is the
	// separate, explicitly confirmed action).
	if _, err := os.Stat(result.OriginalPath); err != nil {
		t.Fatalf("original must be retained: %v", err)
	}
	kept, err := os.ReadFile(filepath.Join(result.OriginalPath, "karyora.db"))
	if err != nil || len(kept) == 0 {
		t.Fatalf("original database must survive: %v", err)
	}
	// The fence event and the restore completion event are journaled.
	var fence int64
	if err := reopened.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'core.restore.completed'`).Scan(&fence); err != nil || fence != 1 {
		t.Fatalf("restore completion event: %d %v", fence, err)
	}
}

func testOptionsAt(dataDir string) Options {
	return Options{
		Path:                   filepath.Join(dataDir, "karyora.db"),
		PoolMaxOpen:            2,
		WriterQueueDepth:       2,
		WriterDeadline:         2 * time.Second,
		BusyTimeout:            200 * time.Millisecond,
		WALAutocheckpointPages: 1000,
		Synchronous:            "NORMAL",
		ReadBudget:             2 * time.Second,
		MaxObjectBytes:         1 << 20,
		MaxObjects:             1000,
		GCGracePeriod:          5 * time.Millisecond,
		PublishInterval:        10 * time.Millisecond,
		PublishBatch:           64,
		Retention:              24 * time.Hour,
	}
}

func TestRestoreGateRejectsTamperedSourceObject(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Tamper with one source object: restore must fail closed with the
	// original untouched and no usable staged store.
	path, _ := s.cas.objectPath(manifest.Objects[0].Digest)
	if err := os.WriteFile(path, []byte("tampered bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir,
		ManifestPath:  filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json"),
		CoreVersion:   "test",
	})
	if err == nil {
		t.Fatal("a tampered object must fail the restore")
	}
	if !strings.Contains(err.Error(), manifest.Objects[0].Digest) {
		t.Fatalf("the diagnostic must name the object: %v", err)
	}
	// The original data directory is untouched.
	if _, err := os.Stat(filepath.Join(dataDir, "karyora.db")); err != nil {
		t.Fatalf("original must be preserved: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dataDir))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), filepath.Base(dataDir)+".restore-") {
			t.Fatalf("staging must be discarded on failure: %s", e.Name())
		}
	}
}

func TestRestoreGateRejectsUnknownManifestVersion(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Gate 1: an unknown manifest format/version never becomes usable.
	path := filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json")
	raw, _ := os.ReadFile(path)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["manifest_version"] = 99
	broken, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Restore(ctx, RestoreOptions{ActiveDataDir: dataDir, ManifestPath: path, CoreVersion: "test"})
	if err == nil || !strings.Contains(err.Error(), "manifest version") {
		t.Fatalf("unknown manifest version must fail gate 1: %v", err)
	}
}

func TestRestoreGateRejectsInaccessibleKeyReferences(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Gate 5: recorded key references with no accessible resolution path
	// are an explicit typed failure — the honest encrypted-but-unreadable
	// state, never a false decryption claim.
	path := filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json")
	raw, _ := os.ReadFile(path)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["recovery_references"] = map[string]any{
		"config_refs":    []string{"policy:default"},
		"secret_handles": []string{"provider:openai/api-key"},
	}
	broken, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(path, broken, 0o600)
	_, err = Restore(ctx, RestoreOptions{ActiveDataDir: dataDir, ManifestPath: path, CoreVersion: "test"})
	if err == nil || !strings.Contains(err.Error(), "gate keys") {
		t.Fatalf("missing key resolution must fail gate 5: %v", err)
	}
	// With a checker that denies the handle, the restore still fails and
	// names the handle.
	_, err = Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir, ManifestPath: path, CoreVersion: "test",
		KeyRefChecker: func(handle string) error { return os.ErrNotExist },
	})
	if err == nil || !strings.Contains(err.Error(), "provider:openai/api-key") {
		t.Fatalf("inaccessible handle must be named: %v", err)
	}
}

func TestRestoreInterruptedSwapResolvesDeterministically(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	parent := filepath.Dir(dataDir)
	base := filepath.Base(dataDir)
	// The captured image travels with the (soon renamed) original data
	// directory — read it before the simulated crash moves it aside.
	raw, err := os.ReadFile(filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "database.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between the two renames: original set aside, staged
	// store (with marker) present, active directory gone.
	former := filepath.Join(parent, base+".former-123")
	staged := filepath.Join(parent, base+".restore-"+manifest.BackupGeneration)
	if err := os.Rename(dataDir, former); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staged, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := RestoreMarker{RestoreGeneration: 7, ManifestGeneration: manifest.BackupGeneration, CommittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	mb, _ := json.Marshal(marker)
	if err := os.WriteFile(filepath.Join(staged, restoreMarkerName), mb, 0o600); err != nil {
		t.Fatal(err)
	}
	// The staged store content is the captured image (copy it in so the
	// roll-forward result is a real store).
	_ = os.WriteFile(filepath.Join(staged, "karyora.db"), raw, 0o600)

	outcome, err := ResolveRestoreSwap(dataDir)
	if err != nil {
		t.Fatalf("roll-forward: %v", err)
	}
	if outcome != SwapRolledForward {
		t.Fatalf("outcome = %s, want rolled_forward", outcome)
	}
	if _, err := os.Stat(filepath.Join(dataDir, restoreMarkerName)); err != nil {
		t.Fatalf("the restored store now holds the active role with its marker: %v", err)
	}
	if _, err := os.Stat(former); err != nil {
		t.Fatalf("the original stays retained: %v", err)
	}

	// An active directory with the marker resolves as completed.
	outcome, err = ResolveRestoreSwap(dataDir)
	if err != nil || outcome != SwapCompleted {
		t.Fatalf("completed swap: %s %v", outcome, err)
	}
	// An untouched original resolves as not started.
	other := filepath.Join(parent, base+"-plain")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(other)
	outcome, err = ResolveRestoreSwap(other)
	if err != nil || outcome != SwapNotStarted {
		t.Fatalf("not-started swap: %s %v", outcome, err)
	}
	// An ambiguous state never accepts.
	ghost := filepath.Join(parent, base+".restore-ghost")
	if err := os.MkdirAll(ghost, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(ghost)
	os.Remove(filepath.Join(dataDir, restoreMarkerName))
	os.Rename(dataDir, former+"-moved") // active gone, no staged store with marker for it
	outcome, err = ResolveRestoreSwap(dataDir)
	if err == nil || outcome != SwapAmbiguous {
		t.Fatalf("ambiguous state must be refused: %s %v", outcome, err)
	}
}
