package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The F12 audit/activity read surface (IMPL-24; the S4 §20 freeze act's
// frozen content + the adopted S11 supply's P-S11-01/P-S11-03 classes as
// implemented): the permission-filtered, server-side, paginated audit
// list and the M-32-class read-with-export operation. Both are READS over
// the coded append-only audit chain — neither mutates anything, neither
// offers any dismiss/delete/correction of source records (the [S]
// read-only law), and neither adds tamper-resistance (the export is a
// point-in-time read; the chain's tamper-evident honesty stays the coded
// machinery's, never claimed as proof by this surface).
//
// The correction contract class (P-S11-02) is NOT implemented here, and
// the honest typing is recorded: the coded audit-record schema is the
// adopted [S] schema "carried exactly" — digests and CAS references only,
// no free-text annotation carrier — so a correction as an additive,
// referenced annotation record has NO honest carrier in the coded
// machinery without either an audit-schema extension (the schema is
// frozen carried-exactly content; extending it is not this milestone's
// call) or a parallel store (forbidden by the class). The recorded
// verified absence stands: no correction operation exists, prior records
// are NEVER mutated (test-pinned), and the correction contract stays its
// recorded owners' item.

// Audit list bounds (the card-9 working candidate's class: a per-surface
// default and the same 1–1000 bound class; the canonical values finalize
// at the freeze/publication acts).
const (
	// DefaultAuditListLimit is the audit-list per-surface default page
	// size.
	DefaultAuditListLimit = 50
	// MaxAuditListLimit is the audit-list page bound.
	MaxAuditListLimit = 1000
	// MaxAuditExportRecords is the export's finite bound (the C2-05 §5
	// finite-bounds rule: the operation carries an explicit cap; a range
	// exceeding it truncates honestly, never silently).
	MaxAuditExportRecords = 1000
)

// auditPolicyDecisions is the closed filter vocabulary: the decision
// port's own coded policy_decision values (recordDecision writes exactly
// these two). Filter options come from the Core only — this register IS
// the Core's vocabulary; nothing client-invented rides the filter
// (SEL-9's rule via P3.2 §3.12; the IMPL-4 status-filter pattern).
var auditPolicyDecisions = map[string]bool{
	"allow": true,
	"deny":  true,
}

// AuditRowsQuery is the audit read port's query (the card-9 class). The
// storage adapter maps it onto its own query shape; the app layer never
// imports the storage package (the dependency map's own rule).
type AuditRowsQuery struct {
	Limit          int
	AfterSequence  uint64
	PolicyDecision string
	FromTime       string
	ToTime         string
	OrderDesc      bool
}

// AuditRow is one raw chain frame as the port supplies it: the chain
// sequence and the canonical payload bytes the security package hashed.
// The payload decodes to security.AuditRecord (the adopted [S] schema).
type AuditRow struct {
	Sequence uint64
	Payload  []byte
}

// AuditRowsPage is one bounded port page.
type AuditRowsPage struct {
	Rows []AuditRow
	More bool
}

// AuditReader is the audit read port (IMPL-24): the paginated,
// server-side read over the durable audit chain frames. The composition
// root implements it over the storage adapter (the lazyAuditStore
// precedent); a nil port keeps the recorded unwired posture (typed
// refusals, fail closed). A pure read face — the port carries no write,
// no delete, and no correction path of any kind.
type AuditReader interface {
	ListAuditRows(ctx context.Context, q AuditRowsQuery) (AuditRowsPage, error)
}

// AuditEntry is one audit list/export entry: the record's chain position
// and the record itself — the coded [S] schema carried exactly, never
// extended (no content field exists or is invented; digests and CAS
// references only).
type AuditEntry struct {
	Sequence uint64               `json:"sequence"`
	Record   security.AuditRecord `json:"record"`
}

// AuditListRequest is the audit-list query contract (the card-9 working
// candidate's class). Limit zero selects the per-surface default;
// PolicyDecision filters by the closed coded register (empty = all);
// Cursor is the opaque continuation cursor from a previous page (empty =
// the first page).
type AuditListRequest struct {
	Limit          int
	PolicyDecision string
	Cursor         string
}

// AuditListResult is one bounded page. NextCursor is the opaque cursor
// for the next page; empty marks the final page — no has_more/total
// vocabulary exists (the card-9 working candidate finalizes at the
// freeze).
type AuditListResult struct {
	Entries    []AuditEntry `json:"entries"`
	NextCursor string       `json:"cursor,omitempty"`
}

// ListAudit returns one bounded page of audit records in the Core-defined
// order — chain sequence descending (newest first; the audit chain is
// append-only, so the sequence IS the time order), through the same
// decision port every read authorizes (the coded audit.read action over
// the coded audit resource — the register's own read class for audit
// records). The read is server-side and permission-filtered exactly as
// its read port is; the stale-page race guard is structural (the keyset
// cursor is a position, not a page number); the no-leak rule binds every
// refusal and empty state.
func (s *Slice) ListAudit(ctx context.Context, req AuditListRequest) (AuditListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.AuditReader == nil {
		return AuditListResult{}, envelopeErr(errs.CodeStorageUnavailable, corr,
			map[string]any{"reason": "audit reader is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceAudit}, security.ActionAuditRead); err != nil {
		return AuditListResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultAuditListLimit
	}
	if limit < 1 || limit > MaxAuditListLimit {
		return AuditListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	if req.PolicyDecision != "" && !auditPolicyDecisions[req.PolicyDecision] {
		return AuditListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "policy_decision"})
	}
	var after uint64
	if req.Cursor != "" {
		pos, err := decodeAuditListCursor(req.Cursor)
		if err != nil {
			return AuditListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		after = pos.Sequence
	}
	page, err := s.deps.AuditReader.ListAuditRows(ctx, AuditRowsQuery{
		Limit:          limit,
		AfterSequence:  after,
		PolicyDecision: req.PolicyDecision,
		OrderDesc:      true,
	})
	if err != nil {
		return AuditListResult{}, err
	}
	entries, err := auditEntries(page.Rows, corr)
	if err != nil {
		return AuditListResult{}, err
	}
	res := AuditListResult{Entries: entries}
	if page.More {
		if len(entries) == 0 {
			return AuditListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeAuditListCursor(entries[len(entries)-1].Sequence)
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return AuditListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// AuditExportRequest is the M-32-class export request: the optional
// RFC3339 date-range bounds and the finite record cap (zero selects the
// bound). The scope/count/date-range confirmation is the CALLING
// surface's duty before this command runs (P1.6's precondition; the UI's
// confirmation flow states the scope, the range, and the bound).
type AuditExportRequest struct {
	FromTime string
	ToTime   string
	Limit    int
}

// AuditExportResult is the export's outcome: the records in the range,
// the honest count, the echoed range, the generation instant, and the
// honest truncation flag. The export is a point-in-time read — it adds
// no tamper-resistance and claims no integrity receipt (the
// integrity-honesty rule: no verifiable receipt mechanism exists, so
// none is claimed).
type AuditExportResult struct {
	Records     []AuditEntry `json:"records"`
	Count       int          `json:"count"`
	From        string       `json:"from,omitempty"`
	To          string       `json:"to,omitempty"`
	GeneratedAt string       `json:"generated_at"`
	Truncated   bool         `json:"truncated"`
}

// ExportAudit is the M-32-class read-with-export operation (the adopted
// P-S11-03 class as implemented): it reads records and mutates nothing;
// it never deletes, edits, or rewrites a source record; the redaction law
// rides the record schema itself (the records carry digests and CAS
// references only — never content, never secret values — so the export
// inherits the schema's redaction by construction, and the export adds
// nothing to it). Authorization-gated through the decision port: the
// implementation's typing rides the coded audit.read action class — the
// export IS a read of audit records (no new register member exists or is
// invented); the canonical export identity resolves at the publication
// act (O-1; the S11 §5 gap 4 note carried). The idempotency class is the
// M-32 row's own: "None required — safe re-issue" — the correlation ID is
// the operation's request identity; no idempotency key is invented. The
// chronological order (sequence ascending) is the export document's
// natural reading order; a range exceeding the finite bound truncates
// honestly (Truncated) — never silently.
func (s *Slice) ExportAudit(ctx context.Context, req AuditExportRequest) (AuditExportResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.AuditReader == nil {
		return AuditExportResult{}, envelopeErr(errs.CodeStorageUnavailable, corr,
			map[string]any{"reason": "audit reader is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceAudit}, security.ActionAuditRead); err != nil {
		return AuditExportResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = MaxAuditExportRecords
	}
	if limit < 1 || limit > MaxAuditExportRecords {
		return AuditExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	if req.FromTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, req.FromTime); err != nil {
			return AuditExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "from"})
		}
	}
	if req.ToTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, req.ToTime); err != nil {
			return AuditExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "to"})
		}
	}
	if req.FromTime != "" && req.ToTime != "" {
		from, _ := time.Parse(time.RFC3339Nano, req.FromTime)
		to, _ := time.Parse(time.RFC3339Nano, req.ToTime)
		if from.After(to) {
			return AuditExportResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "from"})
		}
	}
	page, err := s.deps.AuditReader.ListAuditRows(ctx, AuditRowsQuery{
		Limit:    limit,
		FromTime: req.FromTime,
		ToTime:   req.ToTime,
	})
	if err != nil {
		return AuditExportResult{}, err
	}
	entries, err := auditEntries(page.Rows, corr)
	if err != nil {
		return AuditExportResult{}, err
	}
	return AuditExportResult{
		Records:     entries,
		Count:       len(entries),
		From:        req.FromTime,
		To:          req.ToTime,
		GeneratedAt: s.deps.Now().UTC().Format(time.RFC3339Nano),
		Truncated:   page.More,
	}, nil
}

// auditEntries decodes the port's raw rows into entries. The payload is
// the chain's canonical hashed form — a payload that does not decode, or
// that does not carry the adopted schema version, is the typed
// STATE_CORRUPT (never a silently skipped row, never an invented record).
func auditEntries(rows []AuditRow, corr string) ([]AuditEntry, error) {
	entries := make([]AuditEntry, 0, len(rows))
	for _, row := range rows {
		var rec security.AuditRecord
		if err := json.Unmarshal(row.Payload, &rec); err != nil {
			return nil, envelopeErr(errs.CodeStateCorrupt, corr,
				map[string]any{"reason": "audit record payload is not decodable"})
		}
		if rec.SchemaVersion != security.AuditSchemaVersion {
			return nil, envelopeErr(errs.CodeStateCorrupt, corr,
				map[string]any{"reason": "audit record schema version mismatch"})
		}
		entries = append(entries, AuditEntry{Sequence: row.Sequence, Record: rec})
	}
	return entries, nil
}

// auditListCursorPosition is the decoded form behind the opaque
// audit-list cursor: the keyset position (the last page's lowest chain
// sequence). It follows the coded event-cursor PATTERN — a salt-prefixed
// hex-JSON envelope, never structured data on the wire — as a SEPARATE
// query-side mechanism (P1.7 §3.5; the S11 §4 P-S11-01 item 3 separation
// carried): the event family's cursor is untouched.
type auditListCursorPosition struct {
	Sequence uint64 `json:"sequence"`
}

func encodeAuditListCursor(sequence uint64) (string, error) {
	if sequence == 0 {
		return "", fmt.Errorf("app: audit list cursor position lacks its keyset value")
	}
	b, err := json.Marshal(auditListCursorPosition{Sequence: sequence})
	if err != nil {
		return "", err
	}
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt[:]) + "." + hex.EncodeToString(b), nil
}

func decodeAuditListCursor(cursor string) (auditListCursorPosition, error) {
	var pos auditListCursorPosition
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return pos, fmt.Errorf("app: audit list cursor is malformed")
	}
	b, err := hex.DecodeString(parts[1])
	if err != nil {
		return pos, fmt.Errorf("app: audit list cursor is malformed")
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: audit list cursor is malformed")
	}
	if pos.Sequence == 0 {
		return pos, fmt.Errorf("app: audit list cursor is malformed")
	}
	return pos, nil
}
