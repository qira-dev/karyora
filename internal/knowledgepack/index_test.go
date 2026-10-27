package knowledgepack

import (
	"strings"
	"testing"
)

// qualityCorpus is the fixed retrieval fixture corpus (C3-07): five small
// documents whose relevant sets for the evaluation queries are known in
// advance. The quality report asserts measured precision/recall against
// these expectations; it never invents a numeric SLO (D-28 stays gated).
var qualityCorpus = []struct {
	path string
	text string
}{
	{"sqlite.md", "Karyora stores durable state in SQLite with WAL mode.\n\nThe writer queue is bounded and migrations are checksummed."},
	{"backup.md", "The backup snapshot is WAL-consistent and pins referenced blobs.\n\nRestore applies generation fencing before authority returns."},
	{"jobs.md", "A durable Job runs through attempts with leases.\n\nA stale worker cannot commit because fencing rejects old epochs."},
	{"context.md", "The context engine compiles snapshots with provenance.\n\nEach snapshot records selected and omitted evidence with reasons."},
	{"cooking.md", "A recipe for bread needs flour, water, salt and yeast.\n\nKnead the dough and let it rise before baking."},
}

// expectations maps an evaluation query to the entry paths a correct
// retriever should return as its top hit.
var expectations = map[string]map[string]bool{
	"sqlite wal migration":   {"sqlite.md": true},
	"backup restore fencing": {"backup.md": true},
}

func buildTestIndex(t *testing.T) *Index {
	t.Helper()
	m := &Manifest{
		SchemaVersion: SchemaVersion, PackID: "pk-q", PackVersion: 1,
		Title: "Quality corpus", Source: Source{Kind: "file", Identity: "fixture", Locator: "fixture://"},
		License: "CC0", LicenseStatus: LicenseKnown, Trust: Trust{State: TrustUnsigned},
	}
	entries := make(map[string][]byte, len(qualityCorpus))
	for _, doc := range qualityCorpus {
		res, err := ImportDocument(doc.path, []byte(doc.text), DefaultLimits())
		if err != nil {
			t.Fatalf("import %q: %v", doc.path, err)
		}
		entry, err := BuildEntry(res)
		if err != nil {
			t.Fatal(err)
		}
		m.Entries = append(m.Entries, entry)
		entries[doc.path] = []byte(doc.text)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	chunks := ChunkManifest(m, entries, DefaultIndexConfig())
	return BuildIndex(chunks, DefaultIndexConfig())
}

// TestRetrievalQualityFixture evaluates the fixed corpus: every expected
// document must be the top hit for its query (recall 1.0 at top-1), the
// result set must contain a relevant hit, and every hit must be
// explainable through its matched terms.
func TestRetrievalQualityFixture(t *testing.T) {
	idx := buildTestIndex(t)
	packStatuses := map[string]string{"pk-q": PackStatusActive}
	filters := Filters{MaxResults: 5}
	for query, want := range expectations {
		hits := idx.Search(query, filters, packStatuses, nil, nil)
		if len(hits) == 0 {
			t.Fatalf("query %q returned no hits", query)
		}
		top := hits[0].Chunk.EntryPath
		if !want[top] {
			t.Fatalf("query %q: top hit is %q, want one of the expected set", query, top)
		}
		relevant := 0
		for _, h := range hits {
			if want[h.Chunk.EntryPath] {
				relevant++
			}
		}
		if relevant == 0 {
			t.Fatalf("query %q: no relevant hits at all", query)
		}
		for _, h := range hits {
			if len(h.MatchedTerms) == 0 {
				t.Fatalf("query %q: hit %s carries no matched terms", query, h.Chunk.ID)
			}
		}
	}
}

func TestIndexRebuildDeterminism(t *testing.T) {
	m := &Manifest{SchemaVersion: SchemaVersion, PackID: "pk-r", PackVersion: 1,
		Title: "r", Source: Source{Kind: "file", Identity: "x", Locator: "x://"},
		LicenseStatus: LicenseUnknown, Trust: Trust{State: TrustUnsigned}}
	text := []byte("alpha beta\n\nalpha gamma")
	res, err := ImportDocument("doc.md", text, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	e, err := BuildEntry(res)
	if err != nil {
		t.Fatal(err)
	}
	m.Entries = append(m.Entries, e)
	entries := map[string][]byte{"doc.md": text}
	chunks := ChunkManifest(m, entries, DefaultIndexConfig())
	idx1 := BuildIndex(chunks, DefaultIndexConfig())
	// Rebuild from the same authoritative source produces the same hits.
	idx2, err := RebuildIndex(func() ([]Chunk, error) { return chunks, nil }, DefaultIndexConfig())
	if err != nil {
		t.Fatal(err)
	}
	h1 := idx1.Search("alpha", Filters{}, nil, nil, nil)
	h2 := idx2.Search("alpha", Filters{}, nil, nil, nil)
	if len(h1) != len(h2) {
		t.Fatalf("rebuild changed the hit count: %d vs %d", len(h1), len(h2))
	}
	for i := range h1 {
		if h1[i].Chunk.ID != h2[i].Chunk.ID || h1[i].Score != h2[i].Score {
			t.Fatalf("rebuild changed hit %d: %+v vs %+v", i, h1[i], h2[i])
		}
	}
}

func TestSearchFiltersPrecedeExposure(t *testing.T) {
	idx := buildTestIndex(t)
	// Quarantined pack: nothing is exposed.
	quarantined := map[string]string{"pk-q": PackStatusQuarantined}
	if hits := idx.Search("sqlite", Filters{}, quarantined, nil, nil); len(hits) != 0 {
		t.Fatalf("quarantined pack leaked %d hits", len(hits))
	}
	// Stale pack excluded unless explicitly included.
	stale := map[string]string{"pk-q": PackStatusActive, "pk-q/freshness": FreshnessStale}
	if hits := idx.Search("sqlite", Filters{}, stale, nil, nil); len(hits) != 0 {
		t.Fatal("stale pack leaked hits while IncludeStale is false")
	}
	if hits := idx.Search("sqlite", Filters{IncludeStale: true}, stale, nil, nil); len(hits) == 0 {
		t.Fatal("IncludeStale=true should expose the stale pack")
	}
	// Unknown license excluded under the policy.
	unknownLicense := map[string]string{"pk-q": PackStatusActive, "pk-q/license": LicenseUnknown}
	if hits := idx.Search("sqlite", Filters{RequireLicenseKnown: true}, unknownLicense, nil, nil); len(hits) != 0 {
		t.Fatal("unknown-license pack leaked hits under the policy")
	}
	// Revoked claim statements are withheld.
	revokedClaims := map[string]string{"pk-c:cl-1": ClaimRevoked}
	m, entries := testManifest(t, "pk-c", 1, map[string]string{"doc.md": "body text"})
	chunks := ChunkManifest(m, entries, DefaultIndexConfig())
	idx2 := BuildIndex(chunks, DefaultIndexConfig())
	packActive := map[string]string{"pk-c": PackStatusActive}
	query := "immutable pack versions"
	if hits := idx2.Search(query, Filters{}, packActive, revokedClaims, nil); len(hits) != 0 {
		t.Fatal("revoked claim chunk leaked into results")
	}
	if hits := idx2.Search(query, Filters{}, packActive, nil, nil); len(hits) == 0 {
		t.Fatal("active claim chunk should be searchable")
	}
}

func TestRelationExpansionBounded(t *testing.T) {
	m, entries := testManifest(t, "pk-e", 1, map[string]string{"doc.md": "alpha beta"})
	// Attach many evidence relations to the claim; expansion must cap.
	for i := 0; i < 20; i++ {
		id := "ev-" + string(rune('a'+i))
		m.Evidence = append(m.Evidence, Evidence{EvidenceID: id, Kind: "observation", Locator: "x", TrustState: TrustUnsigned})
		m.Claims[0].EvidenceIDs = append(m.Claims[0].EvidenceIDs, id)
		m.Relations = append(m.Relations, Relation{FromRef: "claim:cl-1", Type: "supports", ToRef: "evidence:" + id})
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	chunks := ChunkManifest(m, entries, DefaultIndexConfig())
	idx := BuildIndex(chunks, DefaultIndexConfig())
	related := func(c Chunk) []string {
		if c.ClaimID == "" {
			return nil
		}
		var out []string
		for i := range m.Evidence {
			out = append(out, "evidence:"+m.Evidence[i].EvidenceID)
		}
		return out
	}
	hits := idx.Search("alpha beta", Filters{}, map[string]string{"pk-e": PackStatusActive}, nil, related)
	for _, h := range hits {
		if len(h.RelatedRefs) > DefaultIndexConfig().MaxRelationExpansion {
			t.Fatalf("relation expansion exceeded the bound: %d", len(h.RelatedRefs))
		}
	}
	if len(hits) == 0 || !strings.Contains(hits[0].Chunk.Text, "alpha") {
		t.Fatal("expected the entry chunk as a hit")
	}
}
