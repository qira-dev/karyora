package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

func sha256ForTest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func snapshotFixture(id string) contextengine.ContextSnapshot {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	return contextengine.ContextSnapshot{
		SchemaVersion:           contextengine.SchemaVersion,
		SnapshotID:              id,
		TaskRef:                 "task-1",
		Scope:                   "local",
		UserInputDigest:         sha256ForTest("hello"),
		AssembledDigest:         sha256ForTest("assembled"),
		AssembledRepresentation: "### system policy (policy-v1) ###\nhello",
		EstimatorVersion:        "char-ratio-4:v1",
		RendererVersion:         contextengine.RendererVersion,
		CompressionMethod:       contextengine.CompressionNone,
		CreatedAt:               now,
	}
}

func memoryFixture(id string) contextengine.MemoryRecord {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	return contextengine.MemoryRecord{
		MemoryID: id, Scope: "local", Content: "Knowledge fact.",
		Sensitivity: string(contextengine.MemorySensitivityLow), Confidence: 0.9,
		Provenance: contextengine.MemoryProvenance{ExtractionMethod: "manual:v1", ExtractedAt: now},
		Status:     contextengine.MemoryActive, CreatedAt: now, LastConfirmedAt: now,
	}
}

func TestSnapshotStoreImmutable(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	snap := snapshotFixture("snap-1")
	if err := s.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.SaveSnapshot(ctx, snap); err == nil {
		t.Fatal("re-saving a snapshot must conflict")
	}
	got, err := s.LoadSnapshot(ctx, "snap-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AssembledDigest != snap.AssembledDigest || got.Scope != "local" {
		t.Fatalf("snapshot did not round-trip: %+v", got)
	}
	if _, err := s.LoadSnapshot(ctx, "missing"); err == nil {
		t.Fatal("missing snapshot must be NOT_FOUND")
	} else if env, ok := err.(*Error); !ok || env.Code != errs.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestMemoryStoreScopeIsolationAndLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	rec := memoryFixture("mem-1")
	if err := s.SaveMemory(ctx, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.SaveMemory(ctx, rec); err == nil {
		t.Fatal("re-saving the same memory must conflict")
	}
	// Cross-scope read is denied: the record is invisible outside its scope.
	if _, err := s.LoadMemory(ctx, "other", "mem-1"); err == nil {
		t.Fatal("cross-scope read leaked a memory record")
	}
	got, err := s.LoadMemory(ctx, "local", "mem-1")
	if err != nil || got.Content != "Knowledge fact." {
		t.Fatalf("in-scope read failed: %+v %v", got, err)
	}
	// A record in another scope does not leak into this scope's list.
	other := memoryFixture("mem-2")
	other.Scope = "other"
	if err := s.SaveMemory(ctx, other); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListMemory(ctx, "local")
	if err != nil || len(list) != 1 {
		t.Fatalf("scope list leaked: %+v %v", list, err)
	}
	// Status move changes only the disposition.
	if err := s.UpdateMemoryStatus(ctx, "local", "mem-1", contextengine.MemoryDeleted, "user request"); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadMemory(ctx, "local", "mem-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != contextengine.MemoryDeleted || got.SupersededBy != "user request" {
		t.Fatalf("status move did not persist: %+v", got)
	}
	if got.Content != "Knowledge fact." {
		t.Fatal("status move changed content")
	}
}
