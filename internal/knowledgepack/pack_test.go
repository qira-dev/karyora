package knowledgepack

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"
)

// testManifest builds a legal v1 manifest over the given entries.
func testManifest(t *testing.T, packID string, version int, contents map[string]string) (*Manifest, map[string][]byte) {
	t.Helper()
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		PackID:        packID,
		PackVersion:   version,
		Title:         "Test pack",
		Source:        Source{Kind: "file", Identity: "unit-test", Locator: "test://unit", CapturedAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)},
		License:       "CC-BY-4.0",
		LicenseStatus: LicenseKnown,
		Trust:         Trust{State: TrustUnsigned},
	}
	entries := make(map[string][]byte, len(contents))
	for path, text := range contents {
		data := []byte(text)
		res, err := ImportDocument(path, data, DefaultLimits())
		if err != nil {
			t.Fatalf("import %q: %v", path, err)
		}
		entry, err := BuildEntry(res)
		if err != nil {
			t.Fatalf("entry %q: %v", path, err)
		}
		m.Entries = append(m.Entries, entry)
		entries[path] = data
	}
	m.Evidence = append(m.Evidence, Evidence{
		EvidenceID: "ev-1", Kind: "document", Locator: "test://unit/doc",
		Digest: m.Entries[0].Digest, CapturedAt: m.Source.CapturedAt, TrustState: TrustUnsigned,
	})
	m.Claims = append(m.Claims, Claim{
		ClaimID: "cl-1", Statement: "Karyora stores knowledge in immutable pack versions.",
		Status: ClaimActive, EvidenceIDs: []string{"ev-1"},
	})
	if err := m.Validate(); err != nil {
		t.Fatalf("test manifest invalid: %v", err)
	}
	return m, entries
}

func TestManifestValidate(t *testing.T) {
	m, _ := testManifest(t, "pk-1", 1, map[string]string{"doc.md": "# hello\n\nworld body"})
	if err := m.Validate(); err != nil {
		t.Fatalf("legal manifest rejected: %v", err)
	}
	// Broken evidence reference fails closed.
	bad := *m
	bad.Claims[0].EvidenceIDs = []string{"missing"}
	if err := bad.Validate(); err == nil {
		t.Fatal("claim with missing evidence was accepted")
	}
	// Unknown schema version fails closed.
	old := *m
	old.SchemaVersion = 99
	if err := old.Validate(); err == nil {
		t.Fatal("unknown schema version was accepted")
	}
}

func TestCanonicalStableAcrossFieldOrder(t *testing.T) {
	// The same logical document presented with different key order
	// canonicalizes to identical bytes.
	a := map[string]any{"b": 2, "a": map[string]any{"y": []any{1, 2}, "x": "s"}}
	b := map[string]any{"a": map[string]any{"x": "s", "y": []any{1, 2}}, "b": 2}
	ca, err := Canonicalize(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := Canonicalize(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ca, cb) {
		t.Fatalf("canonical forms differ:\n%s\n%s", ca, cb)
	}
}

func TestEntryPathRules(t *testing.T) {
	legal := []string{"doc.md", "a/b/c.txt", "guide-1.markdown"}
	for _, p := range legal {
		if err := ValidateEntryPath(p); err != nil {
			t.Fatalf("path %q should be legal: %v", p, err)
		}
	}
	hostile := map[string]string{
		"/abs.md":        "absolute",
		"../escape.md":   "dot-dot",
		"a/../../up.md":  "nested dot-dot",
		"back\\slash.md": "backslash",
		"nul\x00byte.md": "NUL",
		"trailing/":      "trailing separator",
		"":               "empty",
	}
	for p, why := range hostile {
		if err := ValidateEntryPath(p); err == nil {
			t.Fatalf("path %q (%s) should be rejected", p, why)
		}
	}
}

func TestSignAndVerify(t *testing.T) {
	m, _ := testManifest(t, "pk-sig", 1, map[string]string{"doc.md": "signed content"})
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	env, err := Sign(m, "signer-1", "2026-10-27T00:00:00Z", priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(m, env, pub); err != nil {
		t.Fatalf("signature should verify: %v", err)
	}
	// Tamper with the manifest: the same envelope must fail.
	tampered := *m
	tampered.Title = "Changed after signing"
	if err := VerifySignature(&tampered, env, pub); err == nil {
		t.Fatal("signature verified against a changed manifest")
	}
	// Unknown algorithm fails closed.
	badAlg := *env
	badAlg.Algorithm = "rsa-v1"
	if err := VerifySignature(m, &badAlg, pub); err == nil {
		t.Fatal("unknown algorithm was accepted")
	}
}

func TestImportBounds(t *testing.T) {
	lim := Limits{MaxFileBytes: 100, MaxTotalBytes: 150, MaxEntries: 4, ParseTimeoutSecs: 1}
	if _, err := ImportDocument("doc.md", []byte("ok"), lim); err != nil {
		t.Fatalf("small doc should import: %v", err)
	}
	if _, err := ImportDocument("doc.exe", []byte("ok"), lim); err == nil {
		t.Fatal("disallowed extension was accepted")
	}
	if _, err := ImportDocument("big.md", make([]byte, 101), lim); err == nil {
		t.Fatal("oversize file was accepted")
	}
	if _, err := ImportDocument("bin.md", []byte{0x00, 0x01}, lim); err == nil {
		t.Fatal("binary content was accepted")
	}
	if _, err := ImportDocument("bad.md", []byte("caf\xc3\xa9\xff\xfe"), lim); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportArchiveHostileInputs(t *testing.T) {
	lim := Limits{MaxFileBytes: 1000, MaxTotalBytes: 4000, MaxEntries: 10, ParseTimeoutSecs: 1}
	good := makeZip(t, map[string]string{"a.md": "alpha", "b/c.txt": "beta"})
	got, err := ImportArchive(good, lim)
	if err != nil || len(got) != 2 {
		t.Fatalf("clean archive should import: %v (%d entries)", err, len(got))
	}
	for name, wantReason := range map[string]QuarantineReason{
		"../escape.md":   QuarantineTraversal,
		"/absolute.md":   QuarantineBadEntryPath,
		"back\\slash.md": QuarantineBadEntryPath,
		"virus.exe":      QuarantineUnsupportedFormat,
	} {
		_, err := ImportArchive(makeZip(t, map[string]string{name: "x"}), lim)
		if err == nil {
			t.Fatalf("archive with %q was accepted", name)
		}
		if !bytes.Contains([]byte(err.Error()), []byte(wantReason)) {
			t.Fatalf("archive with %q failed with the wrong reason: %v", name, err)
		}
	}
	tooMany := map[string]string{}
	for i := 0; i < 11; i++ {
		tooMany[fmt.Sprintf("f%d.md", i)] = "x"
	}
	if _, err := ImportArchive(makeZip(t, tooMany), lim); err == nil {
		t.Fatal("over-limit entry count was accepted")
	}
}

func TestVerifyPackFlow(t *testing.T) {
	m, entries := testManifest(t, "pk-v", 1, map[string]string{"doc.md": "# Karyora\n\nImmutable versions."})
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	// Unsigned local pack: legal, trust = unsigned.
	v, err := Verify(m, entries, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != VerdictAccepted || v.TrustState != TrustUnsigned {
		t.Fatalf("unsigned pack should be accepted as unsigned: %+v", v)
	}
	if v.RootDigest == "" {
		t.Fatal("accepted pack must record a root digest")
	}
	// Tampering is a rejection, never a quarantine.
	entries2 := map[string][]byte{"doc.md": []byte("# Karyora\n\nRewritten body.")}
	v2, err := Verify(m, entries2, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Status != VerdictRejected {
		t.Fatalf("tampered pack must be rejected: %+v", v2)
	}
	// Revoked trust quarantines (visible, preserved, not served).
	revoked := *m
	revoked.Trust.State = TrustRevoked
	v3, err := Verify(&revoked, entries, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if v3.Status != VerdictQuarantined {
		t.Fatalf("revoked pack must quarantine: %+v", v3)
	}
	// Unsupported schema version quarantines with a visible reason.
	future := *m
	future.SchemaVersion = 2
	v4, err := Verify(&future, entries, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if v4.Status != VerdictQuarantined {
		t.Fatalf("unsupported version must quarantine: %+v", v4)
	}
	// Freshness is visible state, not rejection.
	stale := *m
	stale.Freshness = Freshness{AssessedAt: now.Add(-48 * time.Hour), StaleAfterSecs: 3600}
	v5, err := Verify(&stale, entries, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if v5.Status != VerdictAccepted || v5.Freshness != FreshnessStale {
		t.Fatalf("stale pack should be accepted-but-stale: %+v", v5)
	}
}

func TestExportRoundTrip(t *testing.T) {
	m, entries := testManifest(t, "pk-x", 1, map[string]string{"doc.md": "# Export\n\nRound trip."})
	var buf bytes.Buffer
	if err := Export(&buf, m, entries, nil, nil); err != nil {
		t.Fatalf("export: %v", err)
	}
	gotManifest, gotEntries, _, err := ImportExported(buf.Bytes())
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	root1, err := PackRootDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	root2, err := PackRootDigest(gotManifest)
	if err != nil {
		t.Fatal(err)
	}
	if root1 != root2 {
		t.Fatalf("round trip changed the root digest: %s vs %s", root1, root2)
	}
	if string(gotEntries["doc.md"]) != string(entries["doc.md"]) {
		t.Fatal("round trip changed entry content")
	}
	// A pack that fails verification is never exported.
	tampered := map[string][]byte{"doc.md": []byte("changed")}
	var sink bytes.Buffer
	if err := Export(&sink, m, tampered, nil, nil); err == nil {
		t.Fatal("tampered pack was exported")
	}
}
