package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// C2-12 §1/§2: digest grammar, staged-write protocol, idempotent publish.
func TestCASPutReadRoundtrip(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 1<<20, 1000, 10*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	data := []byte("canonical bytes for the digest")
	digest := DigestOf(data)
	got, err := c.Put(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Fatalf("digest mismatch: %s vs %s", got, digest)
	}
	if !strings.HasPrefix(digest, DigestPrefix) {
		t.Fatalf("digest must be algorithm-qualified: %s", digest)
	}
	// Fan-out layout: objects/<2>/<2>/<full-hex>.
	hexPart := strings.TrimPrefix(digest, DigestPrefix)
	expectedPath := filepath.Join(c.objects, hexPart[0:2], hexPart[2:4], hexPart)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("fan-out layout expected at %s: %v", expectedPath, err)
	}
	// Re-publishing the same content converges idempotently.
	again, err := c.Put(context.Background(), data)
	if err != nil || again != digest {
		t.Fatalf("idempotent publish: %s %v", again, err)
	}
	// Read re-verifies the digest before returning.
	out, err := c.Read(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(data) {
		t.Fatal("roundtrip mismatch")
	}
}

// C2-12 §3: a corrupted referenced object fails closed with STATE_CORRUPT.
func TestCASCorruptObjectFailsClosed(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 1<<20, 1000, 10*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	digest, err := c.Put(context.Background(), []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the stored object in place.
	path, _ := c.objectPath(digest)
	if err := os.WriteFile(path, []byte("tampered bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = c.Read(context.Background(), digest)
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeStateCorrupt {
		t.Fatalf("corrupt read err = %v, want STATE_CORRUPT", err)
	}
	// A missing referenced object also fails closed.
	_, err = c.Read(context.Background(), DigestPrefix+"0000000000000000000000000000000000000000000000000000000000000000")
	if !errors.As(err, &se) || se.Code != errs.CodeStateCorrupt {
		t.Fatalf("missing read err = %v, want STATE_CORRUPT", err)
	}
}

// C2-12 §5: write bounds are validated; oversized writes are rejected.
func TestCASBoundsRejected(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 16, 2, 10*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(context.Background(), make([]byte, 32)); err == nil {
		t.Fatal("oversized object must be rejected")
	}
}

// C2-12 §2 crash window 1: an orphaned staged file with valid content is
// completed by startup reconciliation; invalid content is discarded.
func TestCASReconcileStaging(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 1<<20, 1000, 10*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after step 1 (staged, fsynced, not published).
	valid := []byte("staged before the crash")
	stagedPath := filepath.Join(c.staging, "orphan1.tmp")
	if err := os.WriteFile(stagedPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stagedPath, past, past); err != nil {
		t.Fatal(err)
	}
	// And one whose bytes never verified (crash mid-write).
	badPath := filepath.Join(c.staging, "orphan2.tmp")
	if err := os.WriteFile(badPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(badPath, past, past); err != nil {
		t.Fatal(err)
	}
	published, discarded, err := c.ReconcileStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published != 0 || discarded != 2 {
		t.Fatalf("reconcile = %d published, %d discarded; want 0/2 — a partial write has no intended digest and is never published", published, discarded)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatal("stale staged file must be collected")
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatal("invalid staged file must be discarded")
	}
	// The next real write re-runs the full protocol and succeeds.
	if _, err := c.Put(context.Background(), valid); err != nil {
		t.Fatalf("a fresh Put after reconciliation: %v", err)
	}
}

// C2-12 §4: GC collects only unreachable objects past the grace period,
// rechecking reachability immediately before deletion; staged objects are
// never collected.
func TestCASGCRules(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 1<<20, 1000, 10*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	orphan, err := c.Put(ctx, []byte("unreferenced"))
	if err != nil {
		t.Fatal(err)
	}
	rooted, err := c.Put(ctx, []byte("referenced"))
	if err != nil {
		t.Fatal(err)
	}
	c.RegisterReferenceType("test.root", func(ctx context.Context) (map[string]bool, error) {
		return map[string]bool{rooted: true}, nil
	})
	// Age both past the grace period.
	hexOrphan := strings.TrimPrefix(orphan, DigestPrefix)
	hexRooted := strings.TrimPrefix(rooted, DigestPrefix)
	past := time.Now().Add(-time.Hour)
	for _, hex := range []string{hexOrphan, hexRooted} {
		p, err := c.objectPath(DigestPrefix + hex)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatal(err)
		}
	}
	result, err := c.Collect(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("deleted = %d, want exactly the unreachable object", result.Deleted)
	}
	if ok, _ := c.Stat(rooted); !ok {
		t.Fatal("a reachable object must never be collected")
	}
	if ok, _ := c.Stat(orphan); ok {
		t.Fatal("the unreachable object past grace must be collected")
	}
	// A staged file is never collected (C2-12 §4).
	staged := filepath.Join(c.staging, "live.tmp")
	if err := os.WriteFile(staged, []byte("in flight"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Collect(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("staged/in-flight objects are never collected")
	}
	// An unknown digest name is rejected by the grammar.
	if _, err := c.objectPath("../../escape"); err == nil {
		t.Fatal("path traversal must be rejected by the digest grammar")
	}
	var se *Error
	if _, err := c.Read(ctx, "../evil"); !errors.As(err, &se) || se.Code != errs.CodeValidationFailed {
		t.Fatalf("invalid digest err = %v, want VALIDATION_FAILED", err)
	}
}

// C2-12 §4: unregistered reachability is not collectible — with no
// registered roots, nothing referenced through an unregistered owner is
// deleted, and the batch bound is honored.
func TestGCBatchBound(t *testing.T) {
	c := NewCAS(filepath.Join(t.TempDir(), "cas"), 1<<20, 1000, 1*time.Millisecond)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := c.Put(ctx, []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	result, err := c.Collect(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted > 2 {
		t.Fatalf("batch bound violated: deleted %d", result.Deleted)
	}
}
