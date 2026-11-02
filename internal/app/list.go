package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The Jobs-list operation (the card-9 working candidate, the owner's
// 2026-11-02 P-S1-02 answer): server-side cursor pagination consistent with
// the coded event-cursor pattern, a per-surface default limit, and a
// Core-defined sort. It is the working candidate's implementation, recorded
// for the freeze — not a freeze and not a canonical-wire claim beyond the
// adopted operation-name vocabulary.

const (
	// DefaultListLimit is the Jobs-list per-surface default page size (the
	// card-9 working candidate's per-surface default; finalized at the
	// freeze). It bounds one page well under the transport bound so a UI
	// list surface reads bounded pages by default.
	DefaultListLimit = 50
	// MaxListLimit is the list page bound — the same 1–1000 bound class the
	// replay read's transport safeguard uses.
	MaxListLimit = 1000
)

// ListRequest is the Jobs-list query contract. Limit zero selects the
// per-surface default; Status filters by a registered Job state (empty =
// all); Cursor is the opaque continuation cursor from a previous page
// (empty = the first page).
type ListRequest struct {
	Limit  int
	Status string
	Cursor string
}

// ListResult is one bounded page. NextCursor is the opaque cursor for the
// next page; empty marks the final page — no has_more/total vocabulary
// exists (P-S1-02's working candidate finalizes at the freeze).
type ListResult struct {
	Jobs       []aijobrelay.Job
	NextCursor string
}

// List returns one bounded page of the requesting principal's Jobs in the
// Core-defined order — created_at descending (newest first, the natural
// order for a Jobs list; no client sort parameter exists to name), the
// job_id ascending tiebreak for determinism — through the same decision
// port every read authorizes (C2-14: Local mode stays role-free; the
// server-side list is exactly as permission-filtered as its read port).
func (s *Slice) List(ctx context.Context, req ListRequest) (ListResult, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob}, security.ActionDataRead); err != nil {
		return ListResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return ListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	if req.Status != "" && !aijobrelay.RegisteredJobState(req.Status) {
		return ListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "status"})
	}
	q := aijobrelay.ListJobsQuery{Limit: limit, Status: req.Status}
	if req.Cursor != "" {
		pos, err := decodeListCursor(req.Cursor)
		if err != nil {
			return ListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterJobID = pos.CreatedAt, pos.JobID
	}
	page, err := s.deps.Relay.ListJobs(ctx, q)
	if err != nil {
		return ListResult{}, err
	}
	res := ListResult{Jobs: page.Jobs}
	if page.More {
		if len(page.Jobs) == 0 {
			return ListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeListCursor(page.Jobs[len(page.Jobs)-1], s.deps.Now())
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return ListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// listCursorPosition is the decoded form behind the opaque Jobs-list
// cursor: the keyset position (the last page's sort-key values). It follows
// the coded event-cursor PATTERN — a salt-prefixed hex-JSON envelope, never
// structured data on the wire (events.EncodeCursor) — as a SEPARATE
// query-side mechanism (P1.7 §3.5): the event family's cursor is untouched
// (the F10 freeze stands). Carrying only the keyset, it is a pure position,
// not a retention-bound token.
type listCursorPosition struct {
	CreatedAt time.Time `json:"created_at"`
	JobID     string    `json:"job_id"`
}

func encodeListCursor(job aijobrelay.Job, at time.Time) (string, error) {
	if job.JobID == "" || job.CreatedAt.IsZero() {
		return "", fmt.Errorf("app: list cursor position lacks its keyset values")
	}
	b, err := json.Marshal(listCursorPosition{CreatedAt: job.CreatedAt, JobID: job.JobID})
	if err != nil {
		return "", err
	}
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt[:]) + "." + hex.EncodeToString(b), nil
}

func decodeListCursor(cursor string) (listCursorPosition, error) {
	var pos listCursorPosition
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return pos, fmt.Errorf("app: list cursor is malformed")
	}
	b, err := hex.DecodeString(parts[1])
	if err != nil {
		return pos, fmt.Errorf("app: list cursor is malformed")
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: list cursor is malformed")
	}
	if pos.JobID == "" || pos.CreatedAt.IsZero() {
		return pos, fmt.Errorf("app: list cursor is malformed")
	}
	return pos, nil
}
