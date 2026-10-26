// Package events defines the event record and the journal/outbox/cursor
// semantics shared by the durable Core (C1-08 Part B, C2-13). It is a pure
// contract package: it imports only the domain leaf and carries no
// database, transport, or OS types. Delivery is at-least-once with
// idempotent consumers; no exactly-once claim exists anywhere, and no
// external broker is required for correctness (locked D-11).
package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MaxInlinePayloadBytes bounds the safe inline payload. Anything larger (or
// secret-bearing/raw) must travel as a digest + CAS blob reference (C2-13
// §2; raw content never goes inline by default).
const MaxInlinePayloadBytes = 16 * 1024

// Record is one event (C1-08 §14.1 field set). Sequence is monotonic per
// aggregate and allocated in the same transaction as the authoritative
// state change and the journal/outbox rows (§4.3 atomicity).
type Record struct {
	EventID       string          `json:"event_id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Sequence      uint64          `json:"sequence"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CausationID   string          `json:"causation_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Producer      string          `json:"producer"`
	InlinePayload json.RawMessage `json:"inline_payload,omitempty"`
	PayloadDigest string          `json:"payload_digest,omitempty"`
	BlobRef       string          `json:"blob_ref,omitempty"`
}

// Validate enforces the record contract: identity fields present, bounded
// inline payload, and the payload rule — either a bounded inline payload or
// a digest + blob reference, never both and never neither.
func (r Record) Validate() error {
	if r.EventID == "" {
		return fmt.Errorf("events: event_id is required")
	}
	if r.AggregateType == "" || r.AggregateID == "" {
		return fmt.Errorf("events: aggregate identity is required")
	}
	if r.EventType == "" {
		return fmt.Errorf("events: event_type is required")
	}
	if r.Producer == "" {
		return fmt.Errorf("events: producer identity is required")
	}
	hasInline := len(r.InlinePayload) > 0
	hasRef := r.PayloadDigest != "" && r.BlobRef != ""
	if hasInline && hasRef {
		return fmt.Errorf("events: payload must be inline or referenced, not both")
	}
	if !hasInline && !hasRef {
		return fmt.Errorf("events: payload or digest+blob reference is required")
	}
	if hasInline && len(r.InlinePayload) > MaxInlinePayloadBytes {
		return fmt.Errorf("events: inline payload exceeds %d bytes; reference the blob instead", MaxInlinePayloadBytes)
	}
	if hasRef && !DigestGrammar(r.BlobRef) {
		return fmt.Errorf("events: blob_ref %q does not match the digest grammar", r.BlobRef)
	}
	return nil
}

// DigestGrammar reports whether s matches the additive, version-qualified
// CAS naming scheme (C2-12 §1): `sha256:<64 lowercase hex>`.
func DigestGrammar(s string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	hexPart := strings.TrimPrefix(s, prefix)
	if len(hexPart) != 64 {
		return false
	}
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// SortBySequence orders records within one aggregate stream (per-aggregate
// monotonic sequence; global ordering is never implied, C2-13 §4).
func SortBySequence(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].AggregateType != records[j].AggregateType {
			return records[i].AggregateType < records[j].AggregateType
		}
		if records[i].AggregateID != records[j].AggregateID {
			return records[i].AggregateID < records[j].AggregateID
		}
		return records[i].Sequence < records[j].Sequence
	})
}

// FirstGap returns the first missing sequence in a single aggregate's
// ordered records, assuming the stream starts at 1. found=false means the
// run is gapless up to len(records).
func FirstGap(ordered []Record) (gapAt uint64, found bool) {
	for i, r := range ordered {
		if r.Sequence != uint64(i)+1 {
			return uint64(i) + 1, true
		}
	}
	return 0, false
}

// cursorPayload is the decoded form behind an opaque cursor. Cursors are
// never exposed as structured data to clients: they are bound to one
// stream, authorization-checked at read/replay time, and expire with
// retention (C2-13 §5).
type cursorPayload struct {
	Stream   string `json:"stream"`
	Sequence uint64 `json:"sequence"`
	IssuedAt int64  `json:"issued_at"`
}

// EncodeCursor returns the opaque cursor string for a stream position.
func EncodeCursor(stream string, sequence uint64, at time.Time) (string, error) {
	b, err := json.Marshal(cursorPayload{Stream: stream, Sequence: sequence, IssuedAt: at.Unix()})
	if err != nil {
		return "", err
	}
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt[:]) + "." + hex.EncodeToString(b), nil
}

// DecodeCursor parses an opaque cursor and returns its stream and sequence.
func DecodeCursor(cursor string) (stream string, sequence uint64, err error) {
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("events: cursor is malformed")
	}
	b, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("events: cursor is malformed")
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return "", 0, fmt.Errorf("events: cursor is malformed")
	}
	if p.Stream == "" {
		return "", 0, fmt.Errorf("events: cursor stream is empty")
	}
	return p.Stream, p.Sequence, nil
}

// ReplayOutcome is the explicit stream-read result (C2-13 §5): a client can
// never be stranded in a false permanent status — expiry and gaps resolve
// through the authoritative refetch path.
type ReplayOutcome string

const (
	ReplayOK            ReplayOutcome = "ok"
	ReplayCursorExpired ReplayOutcome = "cursor_expired"
	ReplayGap           ReplayOutcome = "replay_gap"
	ReplayForbidden     ReplayOutcome = "forbidden"
)

// ReplayWindow is the replay retention window (Proposed 30 days from
// C1-08, pending owner acceptance). The production value is an owner
// policy decision, not a hidden default.
const ReplayWindow = 30 * 24 * time.Hour
