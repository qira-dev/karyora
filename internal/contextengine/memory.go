package contextengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// Memory sensitivity classes (C3-11): only low-sensitivity, sufficiently
// confident candidates may auto-save; sensitive or important facts require
// explicit approval. The Work scope stays disabled (D-12/D-33 exclusion):
// a Work-scoped record is rejected, never silently created.
type MemorySensitivity string

const (
	MemorySensitivityLow    MemorySensitivity = "low"
	MemorySensitivityMedium MemorySensitivity = "medium"
	MemorySensitivityHigh   MemorySensitivity = "high"
)

// Memory lifecycle statuses (C3-11): correction and deletion are visible
// state transitions, never history rewrites; expired records stop being
// served but remain inspectable.
const (
	MemoryActive    = "active"
	MemoryCorrected = "corrected"
	MemoryExpired   = "expired"
	MemoryDeleted   = "deleted"
)

// Auto-save policy (C3-11): the Proposed defaults — low-sensitivity
// candidates with confidence >= 0.6 may auto-save; everything else needs
// explicit approval.
const AutoSaveMinConfidence = 0.6

// MemoryProvenance records where a memory came from and how it was
// extracted (C3-11). Memory is never automatically verified knowledge.
type MemoryProvenance struct {
	SourceEventID    string    `json:"source_event_id,omitempty"`
	ExtractionMethod string    `json:"extraction_method"`
	ModelRef         string    `json:"model_ref,omitempty"`
	ExtractedAt      time.Time `json:"extracted_at"`
}

// MemoryRecord is one scoped memory (C3-11). It is separate from the
// KnowledgePack: promotion to pack content is an explicit, reviewed,
// provenance-preserving, reversible act — never automatic.
type MemoryRecord struct {
	MemoryID        string            `json:"memory_id"`
	Scope           string            `json:"scope"`
	Content         string            `json:"content"`
	Sensitivity     string            `json:"sensitivity"`
	Confidence      float64           `json:"confidence"`
	Provenance      MemoryProvenance  `json:"provenance"`
	Status          string            `json:"status"`
	CorrectionOf    string            `json:"correction_of,omitempty"`
	SupersededBy    string            `json:"superseded_by,omitempty"`
	Pinned          bool              `json:"pinned,omitempty"`
	TTLSeconds      int64             `json:"ttl_seconds,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	LastConfirmedAt time.Time         `json:"last_confirmed_at"`
	Extensions      map[string]string `json:"extensions,omitempty"`
}

// Validate enforces the record contract (C3-11).
func (m *MemoryRecord) Validate() error {
	if m.MemoryID == "" || m.Scope == "" {
		return fmt.Errorf("contextengine: memory_id and scope are required")
	}
	if strings.EqualFold(m.Scope, "work") || strings.HasPrefix(strings.ToLower(m.Scope), "work:") {
		return fmt.Errorf("contextengine: Work-scoped memory is excluded (D-12/D-33)")
	}
	if m.Content == "" {
		return fmt.Errorf("contextengine: memory content is empty")
	}
	if err := RejectSecretContent(m.Content); err != nil {
		return err
	}
	switch m.Sensitivity {
	case string(MemorySensitivityLow), string(MemorySensitivityMedium), string(MemorySensitivityHigh):
	default:
		return fmt.Errorf("contextengine: illegal memory sensitivity %q", m.Sensitivity)
	}
	switch m.Status {
	case MemoryActive, MemoryCorrected, MemoryExpired, MemoryDeleted:
	default:
		return fmt.Errorf("contextengine: illegal memory status %q", m.Status)
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		return fmt.Errorf("contextengine: confidence must be within [0,1]")
	}
	if m.Provenance.ExtractionMethod == "" {
		return fmt.Errorf("contextengine: memory provenance requires an extraction method")
	}
	return nil
}

// secretMarkers are the content patterns that fail closed (C3-11): memory
// is a no-secret store. The check is a marker scan, not a security proof —
// the guarantee is that memory never stores credential classes, and any
// marker-like content is rejected rather than stored.
var secretMarkers = []string{
	"-----begin", "private key-----",
	"api_key=", "apikey=", "api-key:",
	"authorization: bearer",
	"sk-", "ghp_", "gho_", "xoxb-",
	"aws_secret_access_key", "private_key=",
}

// RejectSecretContent refuses marker-shaped credential content (C3-11
// acceptance: credentials/secrets cannot be written).
func RejectSecretContent(content string) error {
	lower := strings.ToLower(content)
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("contextengine: content matches a secret marker; memory never stores credentials")
		}
	}
	return nil
}

// DecideAutoSave applies the sensitivity/approval policy (C3-11): low
// sensitivity with sufficient confidence auto-saves; medium or high
// sensitivity requires explicit approval; the approval is recorded by the
// caller in the provenance method.
func DecideAutoSave(m *MemoryRecord, approved bool) (bool, error) {
	if err := m.Validate(); err != nil {
		return false, err
	}
	if m.Sensitivity == string(MemorySensitivityLow) && m.Confidence >= AutoSaveMinConfidence {
		return true, nil
	}
	if !approved {
		return false, nil
	}
	return true, nil
}

// Expired reports whether the record's TTL has lapsed at now (C3-11).
// A TTL of zero means no expiry; pinned records do not expire.
func (m *MemoryRecord) Expired(now time.Time) bool {
	if m.Pinned || m.TTLSeconds <= 0 {
		return false
	}
	return now.Sub(m.LastConfirmedAt) > time.Duration(m.TTLSeconds)*time.Second
}

// Correct creates the corrected successor of a record (C3-11): the
// successor links back to the original via CorrectionOf; history is never
// rewritten.
func (m *MemoryRecord) Correct(successorID string, content string, now time.Time) (*MemoryRecord, error) {
	if successorID == "" {
		return nil, fmt.Errorf("contextengine: successor id is required")
	}
	if err := RejectSecretContent(content); err != nil {
		return nil, err
	}
	successor := *m
	successor.MemoryID = successorID
	successor.Content = content
	successor.Status = MemoryActive
	successor.CorrectionOf = m.MemoryID
	successor.SupersededBy = ""
	successor.Pinned = false
	successor.CreatedAt = now
	successor.LastConfirmedAt = now
	return &successor, nil
}

// ExportForPromotion renders the provenance-preserving bundle used by the
// explicit, auditable KnowledgePack promotion path (C3-11): memory never
// becomes pack content automatically — the application layer creates the
// pack and records the reviewed, reversible promotion link.
func (m *MemoryRecord) ExportForPromotion() (map[string]any, error) {
	if m.Status != MemoryActive {
		return nil, fmt.Errorf("contextengine: only active memory can be exported for promotion")
	}
	bundle := map[string]any{
		"memory_id":         m.MemoryID,
		"scope":             m.Scope,
		"content":           m.Content,
		"provenance":        m.Provenance,
		"created_at":        m.CreatedAt,
		"last_confirmed_at": m.LastConfirmedAt,
		"promotion":         "explicit-reviewed-reversible",
	}
	canonical, err := events.CanonicalJSON(bundle)
	if err != nil {
		return nil, err
	}
	return map[string]any{"canonical": string(canonical), "memory_id": m.MemoryID}, nil
}
