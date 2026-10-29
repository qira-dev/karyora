package knowledgepack

// C5-10 measured-evidence harness for the D-28 W4 shape (CR-5.1
// sub-milestone 3, under the owner's 2026-10-29 Go): knowledge-pack
// retrieval on a SIZED synthetic corpus built in the harness. The proposed
// v1 tier is ~1,000 documents / ~100 MB; the delivered pack-import bounds
// (64 MiB per pack import, 10 MiB per entry) are honored by splitting the
// corpus across packs — the retrieval index spans all of them, which is
// the delivered cross-pack search surface.
//
// Sub-benchmarks: import (ImportDocument + BuildEntry throughput, MB/s),
// index-build (ChunkManifest + BuildIndex over the whole corpus), search
// (per-query wall time, p50/p95 over within-run samples of a fixed query
// batch). The corpus is deterministic (seeded), so runs are comparable.
//
// Method binding (D-28 Option A): this machine pinned (Intel i5-6500 4
// cores, 15 GiB RAM, NVMe ext4, kernel 7.0.0-42-generic, go1.25.14);
// timing WITHOUT -race; every number is **Proposed — measured, pending
// review**. Benchmarks run only under -bench.

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func benchW4Docs() int {
	if v := os.Getenv("UAWP_BENCH_W4_DOCS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1000
}

func benchW4TotalBytes() int {
	if v := os.Getenv("UAWP_BENCH_W4_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 100 << 20
}

// corpusBuilder deterministically generates the synthetic corpus: docs
// of ~docBytes with distinctive marker terms so search queries have a
// known relevant document (the same fixture discipline as the retrieval
// quality fixture).
type corpusBuilder struct {
	docs     int
	docBytes int
}

func (cb corpusBuilder) document(i int) string {
	// Deterministic body: filler paragraphs + a distinctive marker term
	// every document carries (marker-i) plus shared technical vocabulary.
	marker := fmt.Sprintf("marker%05d", i)
	var b strings.Builder
	sentence := fmt.Sprintf("Karyora stores durable local state for document %s with verified digests and bounded retrieval budgets. ", marker)
	for b.Len() < cb.docBytes {
		b.WriteString(sentence)
		if b.Len()%7 == 0 {
			b.WriteString("The context engine compiles snapshots with provenance and bounded exposure. ")
		}
	}
	body := b.String()
	body = body[:cb.docBytes]
	return "# Document " + marker + "\n\n" + body
}

// buildCorpusManifests builds the manifests + entries for the corpus,
// split across packs that each stay inside the delivered import bounds.
func buildCorpusManifests(tb testing.TB, docs, totalBytes int) ([]*Manifest, []map[string][]byte, int64) {
	tb.Helper()
	docBytes := totalBytes / docs
	if int64(docBytes) > DefaultLimits().MaxFileBytes {
		docBytes = int(DefaultLimits().MaxFileBytes)
	}
	docsPerPack := int((DefaultLimits().MaxTotalBytes - 1<<20) / int64(docBytes))
	if docsPerPack < 1 {
		docsPerPack = 1
	}
	cb := corpusBuilder{docs: docs, docBytes: docBytes}
	var manifests []*Manifest
	var entriesList []map[string][]byte
	var total int64
	for start := 0; start < docs; start += docsPerPack {
		end := start + docsPerPack
		if end > docs {
			end = docs
		}
		m := &Manifest{
			SchemaVersion: SchemaVersion,
			PackID:        fmt.Sprintf("pk-w4-%03d", start/docsPerPack),
			PackVersion:   1,
			Title:         "C5-10 sized corpus",
			Source:        Source{Kind: "file", Identity: "c510-bench", Locator: "bench://corpus"},
			License:       "CC0",
			LicenseStatus: LicenseKnown,
			Trust:         Trust{State: TrustUnsigned},
		}
		entries := make(map[string][]byte, end-start)
		for i := start; i < end; i++ {
			body := cb.document(i)
			res, err := ImportDocument(fmt.Sprintf("doc-%05d.md", i), []byte(body), DefaultLimits())
			if err != nil {
				tb.Fatalf("import doc %d: %v", i, err)
			}
			entry, err := BuildEntry(res)
			if err != nil {
				tb.Fatalf("build entry %d: %v", i, err)
			}
			m.Entries = append(m.Entries, entry)
			entries[entry.Path] = []byte(body)
			total += int64(len(body))
		}
		if err := m.Validate(); err != nil {
			tb.Fatalf("validate manifest %d: %v", start, err)
		}
		manifests = append(manifests, m)
		entriesList = append(entriesList, entries)
	}
	return manifests, entriesList, total
}

// benchCorpus caches the generated corpus across benchmark runs (count=N
// repeats the whole benchmark function; the corpus itself is fixed data).
var (
	corpusOnce    sync.Once
	corpusMan     []*Manifest
	corpusEntries []map[string][]byte
	corpusBytes   int64
)

func benchCorpus(tb testing.TB) ([]*Manifest, []map[string][]byte, int64) {
	tb.Helper()
	corpusOnce.Do(func() {
		corpusMan, corpusEntries, corpusBytes = buildCorpusManifests(tb, benchW4Docs(), benchW4TotalBytes())
	})
	return corpusMan, corpusEntries, corpusBytes
}

// BenchmarkW4Corpus is the D-28 W4 shape at the proposed tier.
func BenchmarkW4Corpus(b *testing.B) {
	manifests, entriesList, total := benchCorpus(b)

	b.Run("import", func(b *testing.B) {
		b.ResetTimer()
		var bytes int64
		for i := 0; i < b.N; i++ {
			bytes = 0
			for pi := range manifests {
				for path, body := range entriesList[pi] {
					res, err := ImportDocument(path, body, DefaultLimits())
					if err != nil {
						b.Fatalf("import %s: %v", path, err)
					}
					if _, err := BuildEntry(res); err != nil {
						b.Fatalf("build entry %s: %v", path, err)
					}
					bytes += int64(len(body))
				}
			}
		}
		b.StopTimer()
		elapsed := b.Elapsed()
		if elapsed > 0 && bytes > 0 {
			b.ReportMetric(float64(bytes)*float64(b.N)/(1<<20)/elapsed.Seconds(), "import-mib-per-sec")
		}
	})

	b.Run("index-build", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var chunks []Chunk
			for pi := range manifests {
				chunks = append(chunks, ChunkManifest(manifests[pi], entriesList[pi], DefaultIndexConfig())...)
			}
			idx := BuildIndex(chunks, DefaultIndexConfig())
			if idx == nil {
				b.Fatal("index build returned nil")
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(total>>20), "corpus-mib")
	})

	b.Run("search", func(b *testing.B) {
		var chunks []Chunk
		for pi := range manifests {
			chunks = append(chunks, ChunkManifest(manifests[pi], entriesList[pi], DefaultIndexConfig())...)
		}
		idx := BuildIndex(chunks, DefaultIndexConfig())
		statuses := map[string]string{}
		for _, m := range manifests {
			statuses[m.PackID] = PackStatusActive
		}
		queries := make([]string, 0, 32)
		for q := 0; q < 16; q++ {
			queries = append(queries, fmt.Sprintf("marker%05d", q*37%benchW4Docs()))
		}
		for _, shared := range []string{"durable local state", "verified digests", "retrieval budgets", "snapshots with provenance"} {
			queries = append(queries, shared)
		}
		const batch = 4
		samples := make([]float64, 0, b.N*batch)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for k := 0; k < batch; k++ {
				query := queries[(i*batch+k)%len(queries)]
				t0 := time.Now()
				hits := idx.Search(query, Filters{MaxResults: 5}, statuses, nil, nil)
				if len(hits) == 0 {
					b.Fatalf("query %q returned no hits", query)
				}
				samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
			}
		}
		b.StopTimer()
		percentilesC510(b, "search", samples)
	})
}

// percentilesC510 reports median/p50/p95 (ms) over the harness's within-run
// samples as benchmark metrics. Every value is Proposed — measured,
// pending review.
func percentilesC510(b *testing.B, name string, samples []float64) {
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
