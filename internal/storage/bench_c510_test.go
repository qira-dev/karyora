package storage

// C5-10 measured-evidence harnesses and failure injections for the
// storage-side of the CR-5.1 sub-milestone 3 (under the owner's
// 2026-10-29 Go):
//
//   - BenchmarkW5BackupRestore — the D-28 W5 shape: backup creation +
//     restore duration at a sized data volume (proposed tier ~0.5 GiB;
//     overridable per run), with the delivered digest-verification gates
//     inside every restore.
//   - TestFIS4DiskFull — the FI-S4 disk-full residual, simulated in-process
//     with the SQLite max_page_count bound (real SQLITE_FULL → the typed
//     STORAGE_UNAVAILABLE family), covering the disk-full-during-WAL-growth
//     window and the recovery after the bound is lifted.
//   - TestFIR2ClockDiscontinuity — the FI-R2 residual: lease fencing under
//     injected clock discontinuity; the quarantine behavior IS the
//     assertion (backward jump → nothing released; forward jump → honest
//     requeue with a fencing bump and a journaled event; stale commits
//     refused; recovery settles exactly once).
//   - TestFIB3RestoreOntoPopulatedDataDir — the FI-B3 residual + the
//     C5-09 restore matrix row: restore onto a POPULATED data dir carries
//     exactly the captured generation.
//   - TestC509KeyLossRecoveryFacts — the C5-09 key-loss matrix rows: no
//     secret values in manifests, the typed keyring-unavailable gate, and
//     the honest "re-enter the credential, no product data loss" result.
//   - TestC508DeletionPropagation — the C5-08 deletion-propagation checks:
//     knowledge-pack status disposition, memory deletion status, and
//     reference-drop → CAS GC → journal-retention propagation with nothing
//     dangling and an idempotent second pass.
//
// Method binding (D-28 Option A, accepted 2026-10-29): this machine pinned
// (Intel i5-6500 4 cores, 15 GiB RAM, NVMe ext4, kernel 7.0.0-42-generic,
// go1.25.14); timing WITHOUT -race; every number is **Proposed — measured,
// pending review**. The benchmarks run only under -bench; the injection
// tests run in the normal suite (including CI, under -race).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/knowledgepack"
)

func benchW5Volume() int64 {
	if v := os.Getenv("UAWP_BENCH_W5_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 512 << 20 // the proposed ~0.5 GiB tier (D-28 v1)
}

func benchW5ObjectSize() int {
	if v := os.Getenv("UAWP_BENCH_W5_OBJECT_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 4 << 20
}

// percentiles reports median/p50/p95 (ms) over the harness's within-run
// samples as benchmark metrics. Every value is Proposed — measured,
// pending review.
func percentiles(b *testing.B, name string, samples []float64) {
	if len(samples) == 0 {
		return
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	pick := func(p float64) float64 {
		return sorted[int(p*float64(len(sorted)-1))]
	}
	var sum float64
	for _, s := range samples {
		sum += s
	}
	b.ReportMetric(sum/float64(len(samples)), name+"-mean-ms")
	b.ReportMetric(pick(0.50), name+"-p50-ms")
	b.ReportMetric(pick(0.95), name+"-p95-ms")
}

// seedBenchStore opens a store sized for the W5 tier and fills it with
// durable CAS-backed objects (referenced rows) plus a small job history.
func seedBenchStore(tb testing.TB, volume int64, objectSize int) *Store {
	tb.Helper()
	dir := tb.TempDir()
	opts := testOptionsAt(dir)
	opts.MaxObjectBytes = int64(objectSize) * 4
	opts.MaxObjects = 100000
	s, err := Open(context.Background(), opts)
	if err != nil {
		tb.Fatalf("open bench store: %v", err)
	}
	tb.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	count := int(volume / int64(objectSize))
	payload := make([]byte, objectSize)
	for i := 0; i < count; i++ {
		for j := range payload {
			payload[j] = byte(uint32(i)*131 + uint32(j)*7)
		}
		recordCASBackedEvent(tb, s, fmt.Sprintf("c510-w5-%d", i%16), payload)
	}
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("job-w5-%d", i)
		if _, err := s.SubmitJob(ctx, aijobrelay.Job{
			JobID: id, IdempotencyKey: "key-" + id, InputDigest: inputDigest(id),
			State: aijobrelay.JobCreated, Version: 1,
			CorrelationID: "corr-" + id, CreatedAt: time.Now().UTC(),
		}); err != nil {
			tb.Fatalf("seed job %d: %v", i, err)
		}
	}
	return s
}

// BenchmarkW5BackupRestore is the D-28 W5 shape at the proposed tier:
// backup creation and restore duration, with the delivered gates (digest
// verification over the database image and every object) inside each
// measured restore.
func BenchmarkW5BackupRestore(b *testing.B) {
	volume := benchW5Volume()
	s := seedBenchStore(b, volume, benchW5ObjectSize())
	ctx := context.Background()
	nonce := time.Now().UnixNano()
	dataDir := filepath.Dir(s.opts.Path)

	b.Run("backup", func(b *testing.B) {
		samples := make([]float64, 0, b.N)
		var lastManifest BackupManifest
		for i := 0; i < b.N; i++ {
			t0 := time.Now()
			manifest, err := s.Backup(ctx, fmt.Sprintf("c510-w5-%d-%d", nonce, i))
			if err != nil {
				b.Fatalf("backup: %v", err)
			}
			samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
			lastManifest = manifest
		}
		b.StopTimer()
		percentiles(b, "backup", samples)
		// Correctness fixture (outside the timer): the last snapshot is
		// published with one pin per inventoried object.
		var state string
		if err := s.readers.QueryRowContext(ctx,
			`SELECT state FROM backup_runs WHERE generation = ?`, lastManifest.BackupGeneration).Scan(&state); err != nil || state != backupStatePublished {
			b.Fatalf("published run state = %q %v", state, err)
		}
		var pins int
		if err := s.readers.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM backup_pins WHERE generation = ?`, lastManifest.BackupGeneration).Scan(&pins); err != nil || pins != len(lastManifest.Objects) {
			b.Fatalf("pins = %d, want %d (%v)", pins, len(lastManifest.Objects), err)
		}
		b.ReportMetric(float64(lastManifest.ObjectCount), "objects-in-snapshot")
	})

	b.Run("restore", func(b *testing.B) {
		manifest, err := s.Backup(ctx, fmt.Sprintf("c510-w5-restore-%d", nonce))
		if err != nil {
			b.Fatalf("pre-restore backup: %v", err)
		}
		manifestPath := filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json")
		samples := make([]float64, 0, b.N)
		for i := 0; i < b.N; i++ {
			t0 := time.Now()
			result, err := Restore(ctx, RestoreOptions{
				ActiveDataDir: dataDir,
				ManifestPath:  manifestPath,
				CoreVersion:   "c510-bench",
				CorrelationID: fmt.Sprintf("c510-w5-restore-%d-%d", nonce, i),
			})
			if err != nil {
				b.Fatalf("restore: %v", err)
			}
			samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
			// The retired former dir is cleaned up outside the timer so
			// repeated restores never fill the disk.
			if result.OriginalPath != "" {
				_ = os.RemoveAll(result.OriginalPath)
			}
		}
		b.StopTimer()
		percentiles(b, "restore", samples)
	})
}

// TestFIS4DiskFull simulates disk-full in-process (the SQLite
// max_page_count bound produces real SQLITE_FULL errors on WAL-mode
// writes) and asserts the FI-S4 expected behavior: the typed
// STORAGE_UNAVAILABLE family, no partial commit, the writer gate released,
// and a clean recovery after the bound is lifted.
func TestFIS4DiskFull(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Populate first so page allocations are real.
	if err := s.RecordLifecycleEvent(ctx, "c-df", AggregateCore, "df", "core.df.seed", map[string]any{
		"blob": strings.Repeat("x", 1<<20),
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	var pageCount int
	if err := s.readers.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		t.Fatalf("page_count: %v", err)
	}
	// The injection: cap the database at its current size. Any write that
	// needs a new page now fails with SQLITE_FULL — the in-process
	// disk-full simulation (no mounts, no privileges required).
	if _, err := s.writers.ExecContext(ctx, fmt.Sprintf(`PRAGMA max_page_count = %d`, pageCount)); err != nil {
		t.Fatalf("set max_page_count: %v", err)
	}
	var lastErr error
	for i := 0; i < 64; i++ {
		lastErr = s.RecordLifecycleEvent(ctx, "c-df", AggregateCore, "df", "core.df.full", map[string]any{
			"blob": strings.Repeat("y", 1<<20),
		})
		if lastErr != nil {
			break
		}
	}
	if lastErr == nil {
		t.Fatal("the disk-full bound never refused a write (max_page_count did not bite)")
	}
	var se *Error
	if !errors.As(lastErr, &se) || se.Code != errs.CodeStorageUnavailable {
		t.Fatalf("disk-full write err = %v, want the typed STORAGE_UNAVAILABLE family", lastErr)
	}
	// No partial commit: the per-aggregate sequence stays gapless across
	// the refused writes (seed + the writes that landed before the bound
	// bit, never a torn row).
	var count, maxSeq int
	if err := s.readers.QueryRowContext(ctx,
		`SELECT COUNT(*), MAX(sequence) FROM events_journal WHERE aggregate_id = 'df'`).Scan(&count, &maxSeq); err != nil {
		t.Fatalf("probe sequences: %v", err)
	}
	if count != maxSeq {
		t.Fatalf("torn aggregate state after disk-full: count=%d maxSeq=%d", count, maxSeq)
	}
	// The writer gate is released after the typed refusal.
	if s.gate.isActive() {
		t.Fatal("the writer admission gate leaked after the disk-full refusal")
	}
	// Recovery: lift the bound and the store takes new writes cleanly.
	if _, err := s.writers.ExecContext(ctx, `PRAGMA max_page_count = 2147483646`); err != nil {
		t.Fatalf("lift max_page_count: %v", err)
	}
	if err := s.RecordLifecycleEvent(ctx, "c-df", AggregateCore, "df", "core.df.recovered", map[string]any{
		"blob": strings.Repeat("z", 1<<20),
	}); err != nil {
		t.Fatalf("post-recovery write: %v", err)
	}
	if err := s.IntegrityCheck(ctx); err != nil {
		t.Fatalf("integrity after disk-full recovery: %v", err)
	}
}

// TestFIR2ClockDiscontinuity is the FI-R2 residual: lease fencing under
// injected clock discontinuity. The quarantine behavior IS the assertion:
// a backward jump releases nothing; a forward jump quarantines the
// interrupted attempt honestly (requeue + fencing bump, journaled); the
// quarantined attempt's commit is refused; recovery settles exactly once.
func TestFIR2ClockDiscontinuity(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Now()
	jobID := "j-clock"
	if _, err := s.SubmitJob(ctx, submitFixture(jobID, "key-clock", "clock payload")); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := s.ClaimNext(ctx,
		aijobrelay.WorkerCapability{WorkerID: "w-clock", MaxMemoryMB: 1024}, time.Hour)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	staleVersion := claim.Job.Version

	// Window A — the clock jumps BACKWARD: under the wrong clock the live
	// lease is not expired, so nothing is released and the attempt keeps
	// its authority (no false requeue, no duplicate attempt).
	requeued, err := s.ReleaseExpiredLeases(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("backward jump sweep: %v", err)
	}
	if len(requeued) != 0 {
		t.Fatalf("a backward clock jump released leases: %+v", requeued)
	}
	job, err := s.LoadJob(ctx, jobID)
	if err != nil || job.State != aijobrelay.JobRunning || job.Version != staleVersion {
		t.Fatalf("job after backward jump: %+v %v", job, err)
	}

	// Window B — the clock jumps FORWARD past the lease: the interrupted
	// attempt is quarantined (requeued with a bumped fencing version).
	requeued, err = s.ReleaseExpiredLeases(ctx, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("forward jump sweep: %v", err)
	}
	if len(requeued) != 1 || requeued[0].JobID != jobID || requeued[0].State != aijobrelay.JobQueued {
		t.Fatalf("forward jump did not quarantine the attempt: %+v", requeued)
	}
	if requeued[0].Version != staleVersion+1 {
		t.Fatalf("quarantine did not bump the fencing version: %+v", requeued[0])
	}
	// The quarantine is visible, not silent: the requeue is journaled.
	var requeueEvents int
	if err := s.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = 'job.requeued'`, jobID).Scan(&requeueEvents); err != nil || requeueEvents < 1 {
		t.Fatalf("quarantine requeue not journaled: %d %v", requeueEvents, err)
	}

	// Window C — the quarantined attempt cannot commit (stale version and
	// stale lease epoch): no false success survives the discontinuity.
	stale := aijobrelay.Commit{
		JobID: jobID, JobVersion: staleVersion, AttemptID: claim.Attempt.AttemptID,
		WorkerID: claim.Attempt.WorkerID, LeaseID: claim.Attempt.LeaseID,
		LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:    aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("ghost"), ReceiptID: "rc-clock-ghost",
	}
	if err := s.CommitAttempt(ctx, stale); err == nil {
		t.Fatal("the quarantined attempt's commit was accepted after the clock jump")
	}

	// Window D — recovery: a fresh claim settles the job exactly once.
	claim2, ok, err := s.ClaimNext(ctx,
		aijobrelay.WorkerCapability{WorkerID: "w-clock-2", MaxMemoryMB: 1024}, time.Hour)
	if err != nil || !ok || claim2.Job.JobID != jobID {
		t.Fatalf("fresh claim after quarantine: %v %v", ok, err)
	}
	good := aijobrelay.Commit{
		JobID: jobID, JobVersion: claim2.Job.Version, AttemptID: claim2.Attempt.AttemptID,
		WorkerID: claim2.Attempt.WorkerID, LeaseID: claim2.Attempt.LeaseID,
		LeaseEpoch: claim2.Attempt.LeaseEpoch,
		ToState:    aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("recovered"), ReceiptID: "rc-clock-ok",
		CorrelationID: claim.Job.CorrelationID,
	}
	if err := s.CommitAttempt(ctx, good); err != nil {
		t.Fatalf("commit after recovery: %v", err)
	}
	final, err := s.LoadJob(ctx, jobID)
	if err != nil || final.State != aijobrelay.JobSucceeded || final.ReceiptID != "rc-clock-ok" {
		t.Fatalf("recovered job after the clock discontinuity: %+v %v", final, err)
	}
	var succeeded int
	if err := s.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = 'job.succeeded'`, jobID).Scan(&succeeded); err != nil || succeeded != 1 {
		t.Fatalf("succeeded events = %d, want exactly 1 (%v)", succeeded, err)
	}
}

// TestFIB3RestoreOntoPopulatedDataDir is the FI-B3 residual and the
// C5-09 restore-matrix row: restore onto a POPULATED data dir carries
// exactly the captured generation (references and payloads), the original
// is retained, and the fencing generations persist.
func TestFIB3RestoreOntoPopulatedDataDir(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	captured := make([]string, 0, len(manifest.Objects))
	for _, o := range manifest.Objects {
		captured = append(captured, o.Digest)
	}
	// The live generation moves on: MORE data (rows + CAS payloads +
	// journal events) lands after the capture, populating the data dir the
	// restore will target.
	recordCASBackedEvent(t, s, "c1", []byte("post-snapshot populated object"))
	if err := s.RecordLifecycleEvent(ctx, "c2", AggregateCore, "live", "core.after.snapshot", nil); err != nil {
		t.Fatal(err)
	}
	s.Close()

	result, err := Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir,
		ManifestPath:  filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json"),
		CoreVersion:   "c510-inject",
		CorrelationID: "c-restore-populated",
	})
	if err != nil {
		t.Fatalf("restore onto populated dir: %v", err)
	}
	reopened, err := Open(ctx, testOptionsAt(dataDir))
	if err != nil {
		t.Fatalf("reopen restored store: %v", err)
	}
	defer reopened.Close()
	// Exactly the captured references and payloads: the two captured
	// objects read back digest-verified; the post-snapshot object is gone.
	for _, digest := range captured {
		if _, err := reopened.ReadObject(ctx, digest); err != nil {
			t.Fatalf("captured payload %s unreadable after restore: %v", digest, err)
		}
	}
	var postSnapshot int64
	if err := reopened.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'core.after.snapshot'`).Scan(&postSnapshot); err != nil || postSnapshot != 0 {
		t.Fatalf("post-snapshot data leaked into the restored store: %d %v", postSnapshot, err)
	}
	var capturedEvents int64
	if err := reopened.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'test.blob.written'`).Scan(&capturedEvents); err != nil || capturedEvents != 2 {
		t.Fatalf("captured journal events = %d, want 2 (%v)", capturedEvents, err)
	}
	// Fencing: the recovery generation persists and is beyond the restore
	// generation (the C5-09 restore-covers-references row).
	var restoreGen, recoveryGen string
	if err := reopened.readers.QueryRowContext(ctx, `SELECT value FROM core_meta WHERE key = 'restore_generation'`).Scan(&restoreGen); err != nil {
		t.Fatalf("restore_generation missing: %v", err)
	}
	if err := reopened.readers.QueryRowContext(ctx, `SELECT value FROM core_meta WHERE key = 'recovery_generation'`).Scan(&recoveryGen); err != nil {
		t.Fatal(err)
	}
	if parseUint(restoreGen) != result.RestoreGeneration || parseUint(recoveryGen) <= result.RestoreGeneration {
		t.Fatalf("fencing after populated restore: restore=%s recovery=%s result=%+v", restoreGen, recoveryGen, result)
	}
	// The original (populated) directory is retained untouched.
	if _, err := os.Stat(result.OriginalPath); err != nil {
		t.Fatalf("original must be retained: %v", err)
	}
	// Writers work again on the restored store (authority returned).
	if err := reopened.RecordLifecycleEvent(ctx, "c3", AggregateCore, "restored", "core.post.restore.write", nil); err != nil {
		t.Fatalf("post-restore write: %v", err)
	}
}

// TestC509KeyLossRecoveryFacts fills the C5-09 backup/restore/key-loss
// acceptance matrix for the launch scope:
//
//   - the minimum-path manifest records NO secret handles and no key
//     values, so local content classes never depend on a key the product
//     could lose;
//   - a key reference that cannot be resolved (the keyring-unavailable
//     path) is the typed gate-5 refusal naming the handle — never a false
//     decryption, never silent data loss;
//   - the honest consequence of credential/key loss is re-entering the
//     credential: the content classes restore without any key.
func TestC509KeyLossRecoveryFacts(t *testing.T) {
	s, dataDir := buildSnapshot(t)
	ctx := context.Background()
	manifest, err := s.Backup(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(s.backupsRoot(), manifest.BackupGeneration, "manifest.json")

	// Fact 1: no secret handles, no key values in the real manifest.
	if len(manifest.RecoveryReferences.SecretHandles) != 0 {
		t.Fatalf("the minimum-path manifest records secret handles: %+v", manifest.RecoveryReferences.SecretHandles)
	}
	if len(manifest.RecoveryReferences.ConfigRefs) != 0 {
		t.Fatalf("the minimum-path manifest records config refs: %+v", manifest.RecoveryReferences.ConfigRefs)
	}

	// Fact 2: the keyring-unavailable path is the typed gate-5 refusal.
	// Inject a key reference into a COPY of the manifest (the delivered
	// minimum path cannot produce one) and resolve nothing: restore must
	// refuse, naming the handle, with the original data dir untouched.
	raw, _ := os.ReadFile(manifestPath)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["recovery_references"] = map[string]any{
		"config_refs":    []string{},
		"secret_handles": []string{"provider:openai/api-key"},
	}
	broken, _ := json.MarshalIndent(m, "", "  ")
	brokenPath := manifestPath + ".keyloss.json"
	if err := os.WriteFile(brokenPath, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(brokenPath)
	s.Close()
	_, err = Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir, ManifestPath: brokenPath, CoreVersion: "c510-inject",
	})
	if err == nil || !strings.Contains(err.Error(), "gate keys") {
		t.Fatalf("unresolvable key reference must fail gate 5: %v", err)
	}
	// Fact 3: the content classes restore WITHOUT any key — the real
	// manifest (no key references) restores cleanly; credential loss costs
	// a re-entry, never product data.
	result, err := Restore(ctx, RestoreOptions{
		ActiveDataDir: dataDir, ManifestPath: manifestPath, CoreVersion: "c510-inject",
		CorrelationID: "c-keyloss",
	})
	if err != nil {
		t.Fatalf("keyless content restore: %v", err)
	}
	reopened, err := Open(ctx, testOptionsAt(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, o := range manifest.Objects {
		if _, err := reopened.ReadObject(ctx, o.Digest); err != nil {
			t.Fatalf("restored payload %s: %v", o.Digest, err)
		}
	}
	if _, err := os.Stat(result.OriginalPath); err != nil {
		t.Fatalf("original retained: %v", err)
	}
}

// TestC508DeletionPropagation exercises the C5-08 deletion-propagation map
// across the delivered stores: knowledge entities delete by STATUS
// DISPOSITION (immutable versions never rewritten; retrieval exposure
// refuses afterwards), memory entities move to the deleted status, and
// reference deletions propagate to CAS content (GC after grace) and to the
// journal (retention) with nothing dangling and an idempotent second pass.
func TestC508DeletionPropagation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// 1. Knowledge entity: user-directed delete = the visible status
	// disposition; content and root digest are never rewritten.
	pack := storedFixture(t, "pk-dp", 1, "dispositioned by the user")
	if err := s.SavePackVersion(ctx, pack); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPackStatus(ctx, "pk-dp", 1, knowledgepack.PackStatusQuarantined, "user-directed delete"); err != nil {
		t.Fatalf("disposition: %v", err)
	}
	got, err := s.LoadPackVersion(ctx, "pk-dp", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != knowledgepack.PackStatusQuarantined || got.StatusReason != "user-directed delete" {
		t.Fatalf("disposition did not persist: %+v", got)
	}
	if got.RootDigest != pack.RootDigest || string(got.Entries["doc.md"]) != "dispositioned by the user" {
		t.Fatal("the delete disposition rewrote immutable content")
	}

	// 2. Memory entity: the deleted status is the delivered memory
	// lifecycle deletion; content and provenance stay untouched.
	mem := contextengine.MemoryRecord{
		MemoryID: "mem-dp", Scope: "scope-dp", Content: "to be deleted",
		Sensitivity: string(contextengine.MemorySensitivityLow), Confidence: 0.9,
		Provenance: contextengine.MemoryProvenance{
			ExtractionMethod: "manual:v1", ExtractedAt: time.Now().UTC(),
		},
		Status:          contextengine.MemoryActive,
		CreatedAt:       time.Now().UTC(),
		LastConfirmedAt: time.Now().UTC(),
	}
	if err := s.SaveMemory(ctx, mem); err != nil {
		t.Fatalf("save memory: %v", err)
	}
	if err := s.UpdateMemoryStatus(ctx, "scope-dp", "mem-dp", contextengine.MemoryDeleted, ""); err != nil {
		t.Fatalf("memory delete: %v", err)
	}
	records, err := s.ListMemory(ctx, "scope-dp")
	if err != nil || len(records) != 1 || records[0].Status != contextengine.MemoryDeleted {
		t.Fatalf("deleted memory not visible as deleted: %+v %v", records, err)
	}

	// 3. Job/CAS entity: deleting the reference rows (the delivered
	// deletion primitives) propagates to CAS content via GC and to the
	// journal via retention.
	digest := recordCASBackedEvent(t, s, "c1", []byte("propagation-deletable content"))
	digest2 := recordCASBackedEvent(t, s, "c1", []byte("propagation-deletable content two"))
	if _, err := s.writers.ExecContext(ctx, `DELETE FROM cas_references WHERE blob_ref IN (?, ?)`, digest, digest2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writers.ExecContext(ctx,
		`UPDATE events_journal SET blob_ref = '', payload_digest = '' WHERE blob_ref IN (?, ?)`, digest, digest2); err != nil {
		t.Fatal(err)
	}
	// Age the objects past the grace period, then collect.
	past := time.Now().Add(-time.Hour)
	for _, d := range []string{digest, digest2} {
		p, err := s.cas.objectPath(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.cas.Collect(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 {
		t.Fatalf("GC collected %d objects, want the 2 unreferenced ones", result.Deleted)
	}
	if ok, _ := s.cas.Stat(digest); ok {
		t.Fatal("deleted content survived in the CAS")
	}
	// Idempotent second pass: nothing dangles, nothing new.
	again, err := s.cas.Collect(ctx, 100)
	if err != nil || again.Deleted != 0 {
		t.Fatalf("second GC pass: %+v %v", again, err)
	}
	// Journal retention: after delivery, retention prunes the delivered
	// events of this fresh store.
	rows, err := s.readers.QueryContext(ctx,
		`SELECT event_id FROM events_journal WHERE event_type = 'test.blob.written'`)
	if err != nil {
		t.Fatal(err)
	}
	var eventIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		eventIDs = append(eventIDs, id)
	}
	rows.Close()
	for _, id := range eventIDs {
		if err := s.markPublished(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	pruned, err := s.ApplyRetention(ctx, 0)
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if pruned != int64(len(eventIDs)) {
		t.Fatalf("retention pruned %d rows, want %d", pruned, len(eventIDs))
	}
	var left int64
	if err := s.readers.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events_journal WHERE event_type = 'test.blob.written'`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("retention left %d deletion-target events: %v", left, err)
	}
	// The store stays consistent through the whole propagation: integrity
	// holds and a fresh write lands.
	if err := s.IntegrityCheck(ctx); err != nil {
		t.Fatalf("integrity after deletion propagation: %v", err)
	}
	if err := s.RecordLifecycleEvent(ctx, "c-dp", AggregateCore, "dp", "core.dp.completed", nil); err != nil {
		t.Fatalf("post-propagation write: %v", err)
	}
}
