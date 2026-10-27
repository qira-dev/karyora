package knowledgepack

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// IndexConfigVersion versions the tokenizer/chunker/ranking configuration
// (C3-07): a rebuilt index declares what produced it, and a config change
// is a new version — never a silent rewrite of ranking behavior.
const IndexConfigVersion = 1

// IndexConfig is the versioned retrieval configuration (C3-07).
type IndexConfig struct {
	Version              int
	MaxChunkBytes        int // chunk size bound (Proposed: Limits.MaxChunkBytes)
	MaxRelationExpansion int // one-hop evidence expansion bound
}

// DefaultIndexConfig returns the Proposed configuration.
func DefaultIndexConfig() IndexConfig {
	return IndexConfig{Version: IndexConfigVersion, MaxChunkBytes: DefaultLimits().MaxChunkBytes, MaxRelationExpansion: 8}
}

// Chunk is one derived, addressable retrieval unit (C3-07). Chunks are
// never authority: they are rebuilt from the stored manifest and entries.
type Chunk struct {
	ID        string // <pack>:<version>:c<ordinal>
	PackID    string
	Version   int
	EntryPath string
	ClaimID   string // set for claim chunks
	Text      string
	Ordinal   int
}

// posting is one term's occurrence record.
type posting struct {
	chunkID string
	count   int
}

// Index is the derived lexical index (C3-07). It is immutable after build;
// a content change produces a new index through RebuildIndex.
type Index struct {
	config   IndexConfig
	chunks   map[string]Chunk
	postings map[string][]posting
}

// ChunkManifest derives the retrieval chunks from one pack version: one
// chunk per bounded entry paragraph plus one chunk per claim statement
// (claims stay searchable as first-class statements). Derivation is pure —
// the same inputs always produce the same chunks.
func ChunkManifest(m *Manifest, entries map[string][]byte, cfg IndexConfig) []Chunk {
	if cfg.MaxChunkBytes <= 0 {
		cfg.MaxChunkBytes = DefaultIndexConfig().MaxChunkBytes
	}
	var out []Chunk
	ordinal := 0
	push := func(c Chunk) {
		c.Ordinal = ordinal
		c.ID = fmt.Sprintf("%s:%d:c%d", c.PackID, c.Version, ordinal)
		ordinal++
		out = append(out, c)
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		data, ok := entries[e.Path]
		if !ok {
			continue
		}
		for _, para := range chunkParagraphs(string(data), cfg.MaxChunkBytes) {
			push(Chunk{PackID: m.PackID, Version: m.PackVersion, EntryPath: e.Path, Text: para})
		}
	}
	for i := range m.Claims {
		c := &m.Claims[i]
		for _, para := range chunkParagraphs(c.Statement, cfg.MaxChunkBytes) {
			push(Chunk{PackID: m.PackID, Version: m.PackVersion, ClaimID: c.ClaimID, Text: para})
		}
	}
	return out
}

// chunkParagraphs splits text into bounded paragraphs on blank lines,
// hard-splitting any paragraph that exceeds max bytes.
func chunkParagraphs(text string, max int) []string {
	raw := strings.Split(text, "\n\n")
	var paras []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for len(p) > max {
			cut := p[:max]
			if sp := strings.LastIndexByte(cut, ' '); sp > max/2 {
				cut = cut[:sp]
			}
			paras = append(paras, strings.TrimSpace(cut))
			p = strings.TrimSpace(p[len(cut):])
		}
		if p != "" {
			paras = append(paras, p)
		}
	}
	return paras
}

// tokenize lowercases and splits on non-letter/non-digit runes; terms
// shorter than two runes are dropped.
func tokenize(s string) []string {
	var terms []string
	var cur strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else if cur.Len() > 0 {
			if cur.Len() >= 2 {
				terms = append(terms, cur.String())
			}
			cur.Reset()
		}
	}
	if cur.Len() >= 2 {
		terms = append(terms, cur.String())
	}
	return terms
}

// BuildIndex builds the inverted index over deterministic chunk order.
func BuildIndex(chunks []Chunk, cfg IndexConfig) *Index {
	if cfg.Version == 0 {
		cfg.Version = IndexConfigVersion
	}
	idx := &Index{
		config:   cfg,
		chunks:   make(map[string]Chunk, len(chunks)),
		postings: make(map[string][]posting),
	}
	// Deterministic input order: sort by chunk ID so equal inputs always
	// build byte-equal postings.
	ordered := make([]Chunk, len(chunks))
	copy(ordered, chunks)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	counts := make(map[string]map[string]int) // term -> chunkID -> count
	for _, c := range ordered {
		idx.chunks[c.ID] = c
		for _, term := range tokenize(c.Text) {
			if counts[term] == nil {
				counts[term] = make(map[string]int)
			}
			counts[term][c.ID]++
		}
	}
	for term, cc := range counts {
		pl := make([]posting, 0, len(cc))
		for chunkID, n := range cc {
			pl = append(pl, posting{chunkID: chunkID, count: n})
		}
		sort.Slice(pl, func(i, j int) bool { return pl[i].chunkID < pl[j].chunkID })
		idx.postings[term] = pl
	}
	return idx
}

// RebuildIndex rebuilds the index from the authoritative source through
// load (C3-07: indexes are derived and rebuildable — never authority).
func RebuildIndex(load func() ([]Chunk, error), cfg IndexConfig) (*Index, error) {
	chunks, err := load()
	if err != nil {
		return nil, err
	}
	return BuildIndex(chunks, cfg), nil
}

// Filters are the exposure controls applied BEFORE results are returned
// (C3-07 acceptance: authorization, freshness, license, conflict, and
// trust filters precede model exposure).
type Filters struct {
	IncludeStale         bool     // false excludes FreshnessStale packs
	RequireLicenseKnown  bool     // true excludes LicenseUnknown packs
	ExcludeClaimStatuses []string // e.g. revoked, superseded, conflicted
	ExcludePackStatuses  []string // e.g. quarantined
	MaxResults           int
}

// Hit is one explainable search result (C3-07).
type Hit struct {
	Chunk        Chunk
	Score        float64
	MatchedTerms []string
	PackStatus   string   // the pack's stored status at query time
	RelatedRefs  []string // bounded one-hop relation expansion
}

// Search runs a lexical query with filters and bounded relation expansion.
// Determinism: equal inputs produce equal results (score desc, chunk ID
// asc). Explainability: every hit names its matched terms and carries its
// source refs; the caller can always answer "why was this included?".
func (idx *Index) Search(query string, filters Filters, packStatuses map[string]string, claimStatuses map[string]string, related func(chunk Chunk) []string) []Hit {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	limit := filters.MaxResults
	if limit <= 0 {
		limit = 50
	}
	exclClaim := make(map[string]bool, len(filters.ExcludeClaimStatuses))
	for _, s := range filters.ExcludeClaimStatuses {
		exclClaim[s] = true
	}
	exclPack := make(map[string]bool, len(filters.ExcludePackStatuses))
	for _, s := range filters.ExcludePackStatuses {
		exclPack[s] = true
	}
	score := make(map[string]float64)
	matched := make(map[string]map[string]bool)
	seenTerm := make(map[string]bool, len(terms))
	for _, t := range terms {
		if seenTerm[t] {
			continue // duplicate query terms count once
		}
		seenTerm[t] = true
		for _, p := range idx.postings[t] {
			score[p.chunkID] += float64(p.count)
			if matched[p.chunkID] == nil {
				matched[p.chunkID] = make(map[string]bool)
			}
			matched[p.chunkID][t] = true
		}
	}
	hits := make([]Hit, 0, len(score))
	for chunkID, s := range score {
		c := idx.chunks[chunkID]
		// Hard exposure rule (C3-06/C3-07): a quarantined or rejected pack
		// is never served to retrieval, regardless of the soft filters —
		// the disposition is preserved and inspectable, not silent.
		switch packStatuses[c.PackID] {
		case PackStatusQuarantined, PackStatusRejected:
			continue
		}
		// Pack-level soft filters precede exposure.
		if exclPack[packStatuses[c.PackID]] {
			continue
		}
		if !filters.IncludeStale && packStatuses[c.PackID+"/freshness"] == FreshnessStale {
			continue
		}
		if filters.RequireLicenseKnown && packStatuses[c.PackID+"/license"] != LicenseKnown {
			continue
		}
		// Claim-level filters: a revoked claim is a trust disposition and
		// is never served; superseded/conflicted states follow the soft
		// filter list.
		if c.ClaimID != "" {
			cs := claimStatuses[c.PackID+":"+c.ClaimID]
			if cs == ClaimRevoked || exclClaim[cs] {
				continue
			}
		}
		termsMatched := make([]string, 0, len(matched[chunkID]))
		for t := range matched[chunkID] {
			termsMatched = append(termsMatched, t)
		}
		sort.Strings(termsMatched)
		hit := Hit{Chunk: c, Score: s, MatchedTerms: termsMatched, PackStatus: packStatuses[c.PackID]}
		if related != nil {
			expansion := related(c)
			if len(expansion) > idx.config.MaxRelationExpansion {
				expansion = expansion[:idx.config.MaxRelationExpansion]
			}
			hit.RelatedRefs = expansion
		}
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Chunk.ID < hits[j].Chunk.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
