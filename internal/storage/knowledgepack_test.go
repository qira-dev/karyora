package storage

import (
	"context"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/knowledgepack"
)

// storedFixture builds a legal unsigned pack version over one document.
func storedFixture(t *testing.T, packID string, version int, text string) knowledgepack.StoredPack {
	t.Helper()
	res, err := knowledgepack.ImportDocument("doc.md", []byte(text), knowledgepack.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := knowledgepack.BuildEntry(res)
	if err != nil {
		t.Fatal(err)
	}
	m := &knowledgepack.Manifest{
		SchemaVersion: knowledgepack.SchemaVersion,
		PackID:        packID,
		PackVersion:   version,
		Title:         "fixture",
		Source: knowledgepack.Source{
			Kind: "file", Identity: "test", Locator: "test://fixture",
		},
		License:       "CC0",
		LicenseStatus: knowledgepack.LicenseKnown,
		Trust:         knowledgepack.Trust{State: knowledgepack.TrustUnsigned},
	}
	m.Entries = append(m.Entries, entry)
	m.Evidence = append(m.Evidence, knowledgepack.Evidence{
		EvidenceID: "ev-1", Kind: "document", Locator: "test://fixture/doc",
		Digest: entry.Digest, TrustState: knowledgepack.TrustUnsigned,
	})
	m.Claims = append(m.Claims, knowledgepack.Claim{
		ClaimID: "cl-1", Statement: text, Status: knowledgepack.ClaimActive,
		EvidenceIDs: []string{"ev-1"},
	})
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	root, err := knowledgepack.PackRootDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	return knowledgepack.StoredPack{
		Manifest:   m,
		RootDigest: root,
		Status:     knowledgepack.PackStatusActive,
		Entries:    map[string][]byte{"doc.md": []byte(text)},
	}
}

func TestPackStoreRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	pack := storedFixture(t, "pk-1", 1, "alpha body")
	if err := s.SavePackVersion(ctx, pack); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.LoadPackVersion(ctx, "pk-1", 1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.RootDigest != pack.RootDigest {
		t.Fatal("root digest changed across the round trip")
	}
	if string(got.Entries["doc.md"]) != "alpha body" {
		t.Fatal("entry content changed across the round trip")
	}
	if got.Manifest.Claims[0].Statement != "alpha body" {
		t.Fatal("claim did not survive the round trip")
	}
	latest, err := s.LatestPackVersion(ctx, "pk-1")
	if err != nil || latest != 1 {
		t.Fatalf("latest = %d, err = %v", latest, err)
	}
	// A second version is additive; history is preserved.
	pack2 := storedFixture(t, "pk-1", 2, "beta body")
	if err := s.SavePackVersion(ctx, pack2); err != nil {
		t.Fatalf("save v2: %v", err)
	}
	versions, err := s.ListPackVersions(ctx, "pk-1")
	if err != nil || len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
		t.Fatalf("versions = %v, err = %v", versions, err)
	}
	first, err := s.LoadPackVersion(ctx, "pk-1", 1)
	if err != nil || first.Manifest.Claims[0].Statement != "alpha body" {
		t.Fatal("historical version was rewritten")
	}
}

func TestPackStoreImmutability(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	pack := storedFixture(t, "pk-imm", 1, "only version")
	if err := s.SavePackVersion(ctx, pack); err != nil {
		t.Fatal(err)
	}
	err := s.SavePackVersion(ctx, pack)
	if err == nil {
		t.Fatal("re-saving the same version must conflict")
	}
	env := err.(*Error)
	if env.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %s", env.Code)
	}
}

func TestPackStoreStatusDisposition(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	pack := storedFixture(t, "pk-st", 1, "statusful")
	if err := s.SavePackVersion(ctx, pack); err != nil {
		t.Fatal(err)
	}
	// Quarantine requires a visible reason.
	if err := s.SetPackStatus(ctx, "pk-st", 1, knowledgepack.PackStatusQuarantined, ""); err == nil {
		t.Fatal("quarantine without a reason was accepted")
	}
	if err := s.SetPackStatus(ctx, "pk-st", 1, knowledgepack.PackStatusQuarantined, "digest mismatch detected"); err != nil {
		t.Fatalf("quarantine with reason: %v", err)
	}
	got, err := s.LoadPackVersion(ctx, "pk-st", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != knowledgepack.PackStatusQuarantined || got.StatusReason != "digest mismatch detected" {
		t.Fatalf("disposition did not persist: %+v", got)
	}
	// The manifest and entries are untouched by a status move.
	if got.RootDigest != pack.RootDigest || string(got.Entries["doc.md"]) != "statusful" {
		t.Fatal("status move changed immutable content")
	}
	// Unknown pack is NOT_FOUND, never a silent empty.
	_, err = s.LoadPackVersion(ctx, "missing", 1)
	env, ok := err.(*Error)
	if !ok || env.Code != errs.CodeNotFound {
		t.Fatalf("expected NOT_FOUND envelope, got %v", err)
	}
}
