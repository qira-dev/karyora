package remotestore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/security/teamauth"
)

// Conformance fixtures for spec §4 (C4-36), executed against a LIVE
// S3-compatible MinIO server plus a LIVE PostgreSQL: the upload-success +
// reference-commit-failure recovery, orphan GC after grace with the
// second-pass recheck, GC never removing a referenced/pinned object, the
// typed visible failures for corrupt/missing objects, and tenant
// isolation (a bare digest is never a global key; the ambiguous bare
// digest fails closed across the StoreDirectory object path). The MinIO
// endpoint/credentials come from the REMOTESTORE_TEST_S3_* variables;
// when unset the object fixtures SKIP (recorded as Not Run for that run,
// never a pass). Fixture ids are per-run unique (runStamp) so the suite
// re-runs against persistent servers.
//
// Honest scope note: the restore-reconciliation fixture row of the spec
// §4 plan ("restore reconciliation rebuilds object references per
// tenant") is NOT RUN — no restore surface exists in this phase; the row
// stays with the backup/restore milestones that own it.

func testStoreWithBlob(t testing.TB) *Store {
	t.Helper()
	s := testStore(t) // skips unless REMOTESTORE_TEST_DSN is set
	endpoint := os.Getenv("REMOTESTORE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("REMOTESTORE_TEST_S3_ENDPOINT not set — MinIO object fixtures Not Run in this run")
	}
	cfg := BlobConfig{
		Endpoint:  endpoint,
		AccessKey: os.Getenv("REMOTESTORE_TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("REMOTESTORE_TEST_S3_SECRET_KEY"),
		Bucket:    "karyora-objects",
		Secure:    os.Getenv("REMOTESTORE_TEST_S3_SECURE") == "1",
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "karyora-objects"
	}
	if v := os.Getenv("REMOTESTORE_TEST_S3_BUCKET"); v != "" {
		cfg.Bucket = v
	}
	blob, err := OpenBlobStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open blob store: %v", err)
	}
	if err := blob.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	s.Blob = blob
	return s
}

// blobHas asserts the presence/absence of the object bytes at the
// tenant-prefixed key (the mark pass's own listing is the probe).
func blobHas(t *testing.T, s *Store, workspaceID, digest string) bool {
	t.Helper()
	sizes, err := s.Blob.listSizes(context.Background(), objectKey(workspaceID, digest))
	if err != nil {
		t.Fatalf("blob probe: %v", err)
	}
	_, ok := sizes[objectKey(workspaceID, digest)]
	return ok
}

func referenceState(t *testing.T, s *Store, workspaceID, digest string) string {
	t.Helper()
	var state string
	if err := s.pool.QueryRow(context.Background(),
		`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2`,
		workspaceID, digest).Scan(&state); err != nil {
		t.Fatalf("read reference row: %v", err)
	}
	return state
}

func TestObjectRoundTripAndTenantIsolation(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	wsA, wsB := uid("ws-obj-a"), uid("ws-obj-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)

	content := []byte("karyora object-store fixture payload " + fmt.Sprint(runStamp))
	res, err := s.PutObject(ctx, wsA, content)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if res.Digest != DigestOf(content) || res.Size != int64(len(content)) {
		t.Fatalf("put result mismatch: %+v", res)
	}
	// Strong read-after-write: the served bytes verify against the digest.
	got, err := s.GetObject(ctx, wsA, res.Digest, 1<<20)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("round-trip bytes differ")
	}
	// Tenant isolation: workspace B cannot resolve workspace A's digest —
	// a bare content digest is never a global key (spec §4); the refusal
	// happens at the DB reference (fail closed, before the blob store).
	if _, err := s.GetObject(ctx, wsB, res.Digest, 1<<20); !errors.Is(err, ErrObjectUnknown) {
		t.Fatalf("cross-workspace read must refuse typed, got %v", err)
	}
	// The same content CAN land in B's own scope: same digest, different
	// tenant key, its own reference row.
	if _, err := s.PutObject(ctx, wsB, content); err != nil {
		t.Fatalf("put in B: %v", err)
	}
	if got, err := s.GetObject(ctx, wsB, res.Digest, 1<<20); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read in B scope: %v", err)
	}
	// The ambiguous bare digest (two workspaces hold it) fails closed on
	// the StoreDirectory object path (TI-08): absent, deny.
	dir := s.Directory()
	if _, ok := dir.WorkspaceOfResource(security.Resource{Type: teamauth.ResourceObject, ID: res.Digest}); ok {
		t.Fatal("an ambiguous bare digest must fail closed on the Directory object path")
	}
	// A digest unique to one workspace resolves its tenant scope.
	unique := []byte("unique-to-a " + fmt.Sprint(runStamp))
	ures, err := s.PutObject(ctx, wsA, unique)
	if err != nil {
		t.Fatalf("put unique: %v", err)
	}
	ws, ok := dir.WorkspaceOfResource(security.Resource{Type: teamauth.ResourceObject, ID: ures.Digest})
	if !ok || ws != wsA {
		t.Fatalf("unique digest must resolve %q, got %q ok=%v", wsA, ws, ok)
	}
	// A malformed digest fails closed before anything is touched.
	if _, err := s.GetObject(ctx, wsA, "not-a-digest", 1<<20); !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("malformed digest must refuse typed, got %v", err)
	}
}

// TestUploadSuccessReferenceCommitFailureRecovery exercises the
// designed-for case of the hard upload-before-reference rule: the object
// is durably written and the DB reference commit did NOT land (the
// fixture writes the bytes through the blob store directly, which is
// exactly the post-upload pre-reference state). The orphan is unreadable
// (no reference = no authority), recoverable by an idempotent PutObject,
// and GC-eligible via the mark pass until it is recovered.
func TestUploadSuccessReferenceCommitFailureRecovery(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	ws := uid("ws-orphan")
	seedWorkspace(t, s, ws)

	content := []byte("orphan recovery fixture " + fmt.Sprint(runStamp))
	digest := DigestOf(content)
	key := objectKey(ws, digest)
	// The upload succeeded; the reference commit "failed" (never ran).
	if err := s.Blob.put(ctx, key, content); err != nil {
		t.Fatalf("direct blob put: %v", err)
	}
	// No reference = no authority: the read refuses before the blob store.
	if _, err := s.GetObject(ctx, ws, digest, 1<<20); !errors.Is(err, ErrObjectUnknown) {
		t.Fatalf("an unreferenced upload must not be readable, got %v", err)
	}
	// The mark pass records the store-side orphan (durable grace start).
	marked, err := s.MarkStoreOrphans(ctx)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if marked != 1 {
		t.Fatalf("marked %d orphans, want 1", marked)
	}
	if state := referenceState(t, s, ws, digest); state != "orphan_candidate" {
		t.Fatalf("marked state = %q, want orphan_candidate", state)
	}
	// Recovery: the idempotent PutObject re-uploads (same content, same
	// digest, same key) and lands the reference, flipping the marked row
	// back — one row, referenced, nothing dangling.
	if _, err := s.PutObject(ctx, ws, content); err != nil {
		t.Fatalf("recovery put: %v", err)
	}
	if state := referenceState(t, s, ws, digest); state != "referenced" {
		t.Fatalf("recovered state = %q, want referenced", state)
	}
	var rows int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM object_refs WHERE workspace_id = $1 AND digest = $2`,
		ws, digest).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("recovery duplicated the reference row (%d rows)", rows)
	}
	if got, err := s.GetObject(ctx, ws, digest, 1<<20); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read after recovery: %v", err)
	}
}

// TestOrphanGCGraceSecondPassRecheck proves the GC's grace period, the
// second-pass recheck and the nothing-dangling invariant for BOTH orphan
// origins: a dropped reference (the deletion path) and a store-side
// orphan (upload-success + reference-commit-failure).
func TestOrphanGCGraceSecondPassRecheck(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	ws := uid("ws-gc")
	seedWorkspace(t, s, ws)
	s.GCGrace = 50 * time.Millisecond // Proposed numeric; the SERVER clock decides

	// Origin 1: the designed deletion path.
	dropped := []byte("gc dropped reference " + fmt.Sprint(runStamp))
	dropDigest := DigestOf(dropped)
	if _, err := s.PutObject(ctx, ws, dropped); err != nil {
		t.Fatalf("put dropped: %v", err)
	}
	if err := s.DropObjectReference(ctx, ws, dropDigest); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if state := referenceState(t, s, ws, dropDigest); state != "orphan_candidate" {
		t.Fatalf("dropped state = %q, want orphan_candidate", state)
	}
	// Origin 2: the store-side orphan (upload without a commit).
	stray := []byte("gc store-side orphan " + fmt.Sprint(runStamp))
	strayDigest := DigestOf(stray)
	if err := s.Blob.put(ctx, objectKey(ws, strayDigest), stray); err != nil {
		t.Fatalf("direct blob put: %v", err)
	}
	if marked, err := s.MarkStoreOrphans(ctx); err != nil || marked != 1 {
		t.Fatalf("mark stray: marked=%d err=%v", marked, err)
	}

	// Inside the grace period nothing is swept.
	if res, err := s.SweepOrphanedObjects(ctx, s.GCGrace); err != nil || res.Swept != 0 || res.Removed != 0 {
		t.Fatalf("sweep before grace must remove nothing: %+v err=%v", res, err)
	}
	if !blobHas(t, s, ws, dropDigest) || !blobHas(t, s, ws, strayDigest) {
		t.Fatal("objects must survive inside the grace period")
	}
	time.Sleep(120 * time.Millisecond) // the SERVER clock passes the grace

	// The sweep removes both after grace; no rows dangle, no bytes dangle.
	res, err := s.SweepOrphanedObjects(ctx, s.GCGrace)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if res.Swept != 2 || res.Removed != 2 {
		t.Fatalf("sweep result %+v, want swept=2 removed=2", res)
	}
	if blobHas(t, s, ws, dropDigest) || blobHas(t, s, ws, strayDigest) {
		t.Fatal("swept objects must be gone from the blob store")
	}
	var rows int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM object_refs WHERE workspace_id = $1 AND digest = ANY($2)`,
		ws, []string{dropDigest, strayDigest}).Scan(&rows); err != nil {
		t.Fatalf("count swept rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("swept references dangle (%d rows)", rows)
	}
	// The store is clean: the mark pass finds nothing new.
	if marked, err := s.MarkStoreOrphans(ctx); err != nil || marked != 0 {
		t.Fatalf("mark after sweep: marked=%d err=%v", marked, err)
	}
}

// TestSweepLeaseBlocksReferenceCommit proves the sweep-lease semantics:
// while a (workspace, digest) holds the 'sweeping' lease, a reference
// commit refuses (and never lands a partial row), and the object becomes
// puttable again after the sweep completes — the ordering that makes
// "orphan cleanup never deletes a referenced object" structural.
func TestSweepLeaseBlocksReferenceCommit(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	ws := uid("ws-lease")
	seedWorkspace(t, s, ws)

	content := []byte("sweep lease fixture " + fmt.Sprint(runStamp))
	digest := DigestOf(content)
	// Upload without a reference, then take the sweep lease "by hand"
	// (the lease state a real sweep holds between its phases).
	if err := s.Blob.put(ctx, objectKey(ws, digest), content); err != nil {
		t.Fatalf("direct blob put: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO object_refs (workspace_id, digest, size_bytes, state)
		VALUES ($1, $2, $3, 'sweeping')`, ws, digest, len(content)); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	// The reference commit refuses while the lease is held; after the
	// bounded attempts nothing partial has landed.
	_, err := s.PutObject(ctx, ws, content)
	if !errors.Is(err, ErrReferenceCommitFailed) || !errors.Is(err, ErrObjectSweepInFlight) {
		t.Fatalf("commit under a sweep lease must refuse typed, got %v", err)
	}
	if _, ok := func() (state string, ok bool) {
		err := s.pool.QueryRow(ctx,
			`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2`,
			ws, digest).Scan(&state)
		return state, err == nil
	}(); !ok {
		t.Fatal("the seeded lease row must still exist")
	}
	// The sweep (grace 0 — the lease is already held) removes the object
	// and the row on its second-pass recheck.
	if res, err := s.SweepOrphanedObjects(ctx, 0); err != nil || res.Removed != 1 {
		t.Fatalf("sweep under lease: %+v err=%v", res, err)
	}
	// A fresh PutObject now succeeds end to end (upload → reference).
	if _, err := s.PutObject(ctx, ws, content); err != nil {
		t.Fatalf("put after sweep: %v", err)
	}
	if got, err := s.GetObject(ctx, ws, digest, 1<<20); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read after lease cleared: %v", err)
	}
}

// TestGCNeverRemovesReferencedOrPinned proves the GC's hard invariants:
// referenced objects survive every sweep, and a backup pin blocks both
// the reference drop and the collection (C2-12 discipline extended
// cross-store; the D-27 legal-hold posture).
func TestGCNeverRemovesReferencedOrPinned(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	ws := uid("ws-pins")
	seedWorkspace(t, s, ws)

	plain := []byte("gc-protected referenced object " + fmt.Sprint(runStamp))
	plainDigest := DigestOf(plain)
	if _, err := s.PutObject(ctx, ws, plain); err != nil {
		t.Fatalf("put plain: %v", err)
	}
	pinned := []byte("gc-protected pinned object " + fmt.Sprint(runStamp))
	pinDigest := DigestOf(pinned)
	if _, err := s.PutObject(ctx, ws, pinned); err != nil {
		t.Fatalf("put pinned: %v", err)
	}
	pinID := uid("backup-pin")
	if err := s.PinObject(ctx, ws, pinDigest, pinID); err != nil {
		t.Fatalf("pin: %v", err)
	}
	// A pinned object refuses the reference drop (the retention barrier).
	if err := s.DropObjectReference(ctx, ws, pinDigest); !errors.Is(err, ErrObjectPinned) {
		t.Fatalf("drop of a pinned object must refuse typed, got %v", err)
	}
	// Force the pinned row into the past-grace orphan state by hand: even
	// from a state the API alone cannot produce, the pin barrier holds.
	if _, err := s.pool.Exec(ctx, `
		UPDATE object_refs SET state = 'orphan_candidate', orphan_since = clock_timestamp() - interval '1 hour'
		WHERE workspace_id = $1 AND digest = $2`, ws, pinDigest); err != nil {
		t.Fatalf("force orphan: %v", err)
	}
	res, err := s.SweepOrphanedObjects(ctx, 0)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if res.Removed != 0 {
		t.Fatalf("the sweep removed %d objects; referenced and pinned objects must survive", res.Removed)
	}
	if !blobHas(t, s, ws, plainDigest) || !blobHas(t, s, ws, pinDigest) {
		t.Fatal("referenced/pinned bytes must survive the sweep")
	}
	if state := referenceState(t, s, ws, plainDigest); state != "referenced" {
		t.Fatalf("referenced state disturbed: %q", state)
	}
	// Releasing the pin restores the normal rules: the sweep collects.
	if err := s.UnpinObject(ctx, ws, pinDigest, pinID); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if res, err := s.SweepOrphanedObjects(ctx, 0); err != nil || res.Removed != 1 {
		t.Fatalf("sweep after unpin: %+v err=%v", res, err)
	}
	if blobHas(t, s, ws, pinDigest) {
		t.Fatal("the unpinned orphan must be collected")
	}
	// The plain referenced object survives everything, bytes verified.
	if got, err := s.GetObject(ctx, ws, plainDigest, 1<<20); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("referenced object after all sweeps: %v", err)
	}
}

// TestCorruptAndMissingObjectBlocksVisibly proves the typed visible
// failures: a live reference whose bytes are tampered with or absent
// refuses with distinct typed errors, and the errors stay value-free
// (no content, no credentials — identifiers only).
func TestCorruptAndMissingObjectBlocksVisibly(t *testing.T) {
	s := testStoreWithBlob(t)
	ctx := context.Background()
	ws := uid("ws-corrupt")
	seedWorkspace(t, s, ws)

	content := []byte("corruption fixture payload-marker " + fmt.Sprint(runStamp))
	digest := DigestOf(content)
	if _, err := s.PutObject(ctx, ws, content); err != nil {
		t.Fatalf("put: %v", err)
	}
	key := objectKey(ws, digest)

	// Corrupt the stored bytes under the digest key (simulated bit-rot or
	// tampering — the reference still lives).
	tampered := append([]byte{}, content...)
	tampered[0] ^= 0xFF
	if err := s.Blob.put(ctx, key, tampered); err != nil {
		t.Fatalf("tamper put: %v", err)
	}
	_, err := s.GetObject(ctx, ws, digest, 1<<20)
	if !errors.Is(err, ErrObjectCorrupt) {
		t.Fatalf("corrupt object must fail typed, got %v", err)
	}
	// Value-free errors: the failure text carries no content bytes.
	if strings.Contains(err.Error(), string(content)) {
		t.Fatal("the corrupt-object error must not carry content")
	}
	// Missing bytes with a live reference: a distinct typed failure.
	if err := s.Blob.remove(ctx, key); err != nil {
		t.Fatalf("remove bytes: %v", err)
	}
	_, err = s.GetObject(ctx, ws, digest, 1<<20)
	if !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("missing object must fail typed, got %v", err)
	}
	if strings.Contains(err.Error(), string(content)) {
		t.Fatal("the missing-object error must not carry content")
	}
	// The typed failures are distinct and inspectable.
	if errors.Is(ErrObjectMissing, ErrObjectCorrupt) || errors.Is(ErrObjectCorrupt, ErrObjectMissing) {
		t.Fatal("missing and corrupt must be distinct typed failures")
	}
}
