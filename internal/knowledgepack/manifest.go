package knowledgepack

import (
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// SchemaVersion is the manifest schema this package implements (C3-03).
// Readers refuse newer or unknown versions fail-closed (C3-02 compatibility
// rules); additive optional fields only within a version.
const SchemaVersion = 1

// Claim status vocabulary (C3-03): conflicting, superseded, and revoked
// states remain visible; they are never silently rewritten or dropped.
const (
	ClaimActive     = "active"
	ClaimSuperseded = "superseded"
	ClaimConflicted = "conflicted"
	ClaimRevoked    = "revoked"
)

// Trust vocabulary (C3-03/C3-06): integrity (digests), authenticity
// (signature), and source trust are separate claims. Unsigned is a legal
// state for local packs and is never confused with a digest failure.
const (
	TrustUnsigned    = "unsigned"
	TrustVerified    = "verified"
	TrustRevoked     = "revoked"
	TrustQuarantined = "quarantined"
)

// License status (C3-03): unknown license is represented explicitly and
// stays visible; it never silently blocks or silently passes.
const (
	LicenseKnown   = "known"
	LicenseUnknown = "unknown"
)

// Freshness states (C3-03): staleness is visible state, not rejection; the
// retrieval layer decides exposure under the recorded policy.
const (
	FreshnessCurrent = "current"
	FreshnessStale   = "stale"
	FreshnessUnknown = "unknown"
)

// Source records where the pack's content was captured from (C3-03).
type Source struct {
	Kind       string    `json:"kind"` // file | url | archive | manual
	Identity   string    `json:"identity"`
	Locator    string    `json:"locator"`
	CapturedAt time.Time `json:"captured_at"`
}

// Freshness carries the recorded staleness policy reference and the
// computed state; the policy itself lives in configuration, not in the pack.
type Freshness struct {
	PolicyRef      string    `json:"policy_ref,omitempty"`
	AssessedAt     time.Time `json:"assessed_at,omitempty"`
	StaleAfterSecs int64     `json:"stale_after_secs,omitempty"`
}

// Trust records the authenticity/trust disposition (C3-04): an absent
// signature is TrustUnsigned, a verified signature is TrustVerified, and
// revocation/quarantine remain visible states.
type Trust struct {
	State    string `json:"state"`
	SignerID string `json:"signer_id,omitempty"`
}

// Entry is one immutable content item in the pack (C3-03). Path is a
// normalized, pack-relative path (digest.go path rules); Digest covers the
// exact entry bytes.
type Entry struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Digest      string `json:"digest"` // sha256:<hex> (events.DigestGrammar)
}

// Claim is one knowledge statement (C3-03). Claims are distinct from
// evidence and from derived search chunks; a generated summary is never
// treated as verified truth — it would be a claim carrying its own
// provenance and Generated flag.
type Claim struct {
	ClaimID      string   `json:"claim_id"`
	Statement    string   `json:"statement"`
	Status       string   `json:"status"`
	EvidenceIDs  []string `json:"evidence_ids"`
	Supersedes   []string `json:"supersedes,omitempty"`
	Generated    bool     `json:"generated,omitempty"`
	GeneratedRef string   `json:"generated_ref,omitempty"` // method/version when Generated
}

// Evidence is one provenance record backing claims (C3-03). Digest points
// at the captured source content; trust state is recorded per evidence.
type Evidence struct {
	EvidenceID string    `json:"evidence_id"`
	Kind       string    `json:"kind"` // document | url | archive | observation
	Locator    string    `json:"locator"`
	Digest     string    `json:"digest,omitempty"` // sha256:<hex> of captured content
	CapturedAt time.Time `json:"captured_at"`
	TrustState string    `json:"trust_state"`
}

// Relation is a bounded, typed link between record references (C3-03).
type Relation struct {
	FromRef string `json:"from_ref"` // claim:<id> | evidence:<id> | entry:<path>
	Type    string `json:"type"`     // supports | contradicts | supersedes | derives
	ToRef   string `json:"to_ref"`
}

// Manifest is the versioned pack manifest (C3-03). It is immutable once a
// version is stored: corrections create a new PackVersion.
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	PackID        string            `json:"pack_id"`
	PackVersion   int               `json:"pack_version"`
	Title         string            `json:"title"`
	Description   string            `json:"description,omitempty"`
	Source        Source            `json:"source"`
	License       string            `json:"license"`
	LicenseStatus string            `json:"license_status"`
	Freshness     Freshness         `json:"freshness"`
	Trust         Trust             `json:"trust"`
	Entries       []Entry           `json:"entries"`
	Claims        []Claim           `json:"claims"`
	Evidence      []Evidence        `json:"evidence"`
	Relations     []Relation        `json:"relations,omitempty"`
	Extensions    map[string]string `json:"extensions,omitempty"`
}

// MaxManifestBytes bounds the canonical manifest document. The manifest is
// metadata; bulky content lives in entries, not here.
const MaxManifestBytes = 4 * 1024 * 1024

// Validate enforces the manifest contract (C3-03 acceptance): unique IDs,
// normalized entry paths, resolvable references, legal states, and the
// entry digest grammar.
func (m *Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("knowledgepack: schema_version %d unsupported (this build reads v%d)", m.SchemaVersion, SchemaVersion)
	}
	if m.PackID == "" {
		return fmt.Errorf("knowledgepack: pack_id is required")
	}
	if m.PackVersion < 1 {
		return fmt.Errorf("knowledgepack: pack_version must be >= 1")
	}
	if m.Source.Kind == "" || m.Source.Identity == "" {
		return fmt.Errorf("knowledgepack: source kind and identity are required")
	}
	switch m.LicenseStatus {
	case LicenseKnown, LicenseUnknown:
	default:
		return fmt.Errorf("knowledgepack: license_status must be known or unknown")
	}
	if m.LicenseStatus == LicenseKnown && m.License == "" {
		return fmt.Errorf("knowledgepack: known license requires a license identifier")
	}
	switch m.Trust.State {
	case TrustUnsigned, TrustVerified, TrustRevoked, TrustQuarantined:
	default:
		return fmt.Errorf("knowledgepack: illegal trust state")
	}
	if m.Trust.State == TrustVerified && m.Trust.SignerID == "" {
		return fmt.Errorf("knowledgepack: verified trust requires a signer identity")
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("knowledgepack: a pack requires at least one entry")
	}
	seenPath := make(map[string]bool, len(m.Entries))
	for i := range m.Entries {
		e := &m.Entries[i]
		if err := ValidateEntryPath(e.Path); err != nil {
			return err
		}
		if seenPath[e.Path] {
			return fmt.Errorf("knowledgepack: duplicate entry path %q", e.Path)
		}
		seenPath[e.Path] = true
		if !events.DigestGrammar(e.Digest) {
			return fmt.Errorf("knowledgepack: entry %q digest does not match the grammar", e.Path)
		}
		if e.SizeBytes < 0 {
			return fmt.Errorf("knowledgepack: entry %q has a negative size", e.Path)
		}
		if e.ContentType == "" {
			return fmt.Errorf("knowledgepack: entry %q lacks content_type", e.Path)
		}
	}
	seenEvidence := make(map[string]bool, len(m.Evidence))
	for i := range m.Evidence {
		ev := &m.Evidence[i]
		if ev.EvidenceID == "" {
			return fmt.Errorf("knowledgepack: evidence_id is required")
		}
		if seenEvidence[ev.EvidenceID] {
			return fmt.Errorf("knowledgepack: duplicate evidence_id %q", ev.EvidenceID)
		}
		seenEvidence[ev.EvidenceID] = true
		if ev.Kind == "" || ev.Locator == "" {
			return fmt.Errorf("knowledgepack: evidence %q requires kind and locator", ev.EvidenceID)
		}
		if ev.Digest != "" && !events.DigestGrammar(ev.Digest) {
			return fmt.Errorf("knowledgepack: evidence %q digest does not match the grammar", ev.EvidenceID)
		}
		switch ev.TrustState {
		case TrustUnsigned, TrustVerified, TrustRevoked, TrustQuarantined:
		default:
			return fmt.Errorf("knowledgepack: evidence %q has an illegal trust state", ev.EvidenceID)
		}
	}
	seenClaim := make(map[string]bool, len(m.Claims))
	for i := range m.Claims {
		c := &m.Claims[i]
		if c.ClaimID == "" {
			return fmt.Errorf("knowledgepack: claim_id is required")
		}
		if seenClaim[c.ClaimID] {
			return fmt.Errorf("knowledgepack: duplicate claim_id %q", c.ClaimID)
		}
		seenClaim[c.ClaimID] = true
		if c.Statement == "" {
			return fmt.Errorf("knowledgepack: claim %q lacks a statement", c.ClaimID)
		}
		switch c.Status {
		case ClaimActive, ClaimSuperseded, ClaimConflicted, ClaimRevoked:
		default:
			return fmt.Errorf("knowledgepack: claim %q has an illegal status", c.ClaimID)
		}
		for _, evID := range c.EvidenceIDs {
			if !seenEvidence[evID] {
				return fmt.Errorf("knowledgepack: claim %q references missing evidence %q", c.ClaimID, evID)
			}
		}
		for _, sup := range c.Supersedes {
			if !seenClaim[sup] {
				return fmt.Errorf("knowledgepack: claim %q supersedes missing claim %q", c.ClaimID, sup)
			}
		}
	}
	for i := range m.Relations {
		r := &m.Relations[i]
		if !validRelationRef(r.FromRef) || !validRelationRef(r.ToRef) {
			return fmt.Errorf("knowledgepack: relation %d has a malformed reference", i)
		}
		switch r.Type {
		case "supports", "contradicts", "supersedes", "derives":
		default:
			return fmt.Errorf("knowledgepack: relation %d has an illegal type", i)
		}
	}
	return nil
}

func validRelationRef(ref string) bool {
	kind, rest, ok := cutPrefix(ref, ":")
	if !ok || rest == "" {
		return false
	}
	switch kind {
	case "claim", "evidence", "entry":
		return true
	default:
		return false
	}
}

// cutPrefix splits s at the first occurrence of sep.
func cutPrefix(s, sep string) (before, after string, found bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}
