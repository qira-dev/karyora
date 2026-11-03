package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/notification"
	"github.com/qira-dev/karyora/internal/security"
)

// The notification surface (the ADOPTED S6 supply — the owner's LIVE S6
// approval gate of 2026-11-03 implemented as working candidates per the
// IMPL-4..10 discipline: nothing finalized, every gap carried). Per draft:
//
//   - P-S6-01 (the list contract): a Core query over Core-owned
//     notification state — server-side, permission-filtered through the
//     ONE decision port, cursor-paginated per the adopted card-9 working
//     candidate (the coded Jobs-list/conversation-list pattern: bounded
//     limits, a Core-defined order, the empty-cursor-marks-final protocol,
//     no has_more/total vocabulary). The grouped collapse follows the
//     recorded rule (group by related object; collapse repeated identical
//     updates without hiding a meaningful state transition; keep the newest
//     timestamp, the unread count, the current canonical state); its
//     concrete mechanics are the recorded owners' Proposed item (P3.8 §11
//     row 4) — this implementation is the working candidate recorded for
//     the freeze. The four-way state distinction (unread/read/stale/
//     permission-revoked) is carried non-color in the payload; unread
//     counts render only from Core-supplied state.
//
//   - P-S6-02 (the retention/mutation contract): the O-6 mutation set is
//     exactly the controls the withheld statement names — mark read, mark
//     unread, filter, clear — each a typed Core outcome riding the D-35
//     envelope on refusal. "Filter" is implemented as list-query parameters
//     over the closed registers (the recorded gap (f): query parameter vs
//     retained preference stays the owner's decision). NO mark-all-read
//     operation exists (not in the named set).
//
//   - P-S6-03 (identity/dedup): the notification identity is Core-issued
//     and stable; dedup is claimed ONLY by the stable Core event identity
//     where the producer provides one (the checked same-transaction ruling
//     in the storage producer) — a re-delivered event never duplicates its
//     notification.
//
//   - P-S6-04 (the stream transport): NOT part of this family's content —
//     the transport stays the UI-26/C-6 gate's decision. Nothing here
//     streams, publishes, or rides the outbox for client delivery; the
//     in-app list read is the panel's authority.
//
//   - P-S6-05 (delivery disposition): in-app ON + OS OFF by default — the
//     OS channel is implemented by NOTHING in this milestone: no
//     OS-permission request, no OS delivery code, no quiet hours in v1
//     (decision 6). The UI-34 FINAL freeze stays its own Checkpoint-C act.
//
//   - P-S6-06 (privacy): every notification record carries safe metadata
//     only — no prompt, file path, token, generated output, or private
//     model data ever rides the notification path.
//
//   - P-S6-07 (the delivered entry surface): this milestone changes
//     NOTHING in the delivered C-04 UI surface — zero ui/ files are
//     touched; the UI notification surface is the NEXT milestone's work.
//
// (6) Notification-read ≠ approval — nothing here lets a read or dismiss
// imply a decision or a work completion: the mutations below write ONLY
// notification rows — they NEVER touch the underlying Job, Activity/audit,
// approval, or security record (the non-mutation law, P-S6-02 / P3.8 §9;
// the non-mutation law is enforced structurally: no code path in this
// surface reaches any other store, and it is pinned by test), and the only
// path that changes an approval remains the explicit decision command.
// Marking read is not acknowledgement; a read approval notification is NOT
// an approved approval.

// NotificationStore is the durable notification-record port (P-S6-03 — the
// identity store is new Core state, the P-S2-04 missing store). The storage
// adapter implements it structurally; the slice consumes only this port.
// Nil disables the notification surface — typed refusals, fail closed (the
// Connections pattern).
type NotificationStore interface {
	// ListNotificationGroups is the server-side grouped list read (the
	// card-9 working candidate; the collapse mechanics are the store's).
	ListNotificationGroups(ctx context.Context, q notification.ListNotificationsQuery) (notification.GroupedListPage, error)
	// UnreadNotificationCounts is the Core-supplied unread-count read.
	UnreadNotificationCounts(ctx context.Context) (notification.UnreadCounts, error)
	// MarkNotificationsRead / MarkNotificationsUnread / ClearNotifications
	// are the O-6 mutation set — ALL-OR-NOTHING typed outcomes writing ONLY
	// notification rows (the non-mutation law above).
	MarkNotificationsRead(ctx context.Context, ids []string) (notification.MutationOutcome, error)
	MarkNotificationsUnread(ctx context.Context, ids []string) (notification.MutationOutcome, error)
	ClearNotifications(ctx context.Context, ids []string) (notification.MutationOutcome, error)
}

// Notification bounds (Proposed, pending the recorded owner review — the
// slice-bounds discipline). The VALUES are working-candidate disclosures
// for the recorded gap (the card-9 mechanism is adopted; the per-surface
// values stay the owner's — the coded Jobs-list values are the coded
// pattern cited, never assumed for notifications): the same 1–1000 bound
// class and the same per-surface default shape as the other read surfaces.
const (
	// DefaultNotificationListLimit is the notification-list per-surface
	// default page size.
	DefaultNotificationListLimit = 50
	// MaxNotificationListLimit is the list page bound — the same 1–1000
	// bound class the other read surfaces use.
	MaxNotificationListLimit = 1000
	// MaxNotificationMutationIDs bounds one mutation's id set.
	MaxNotificationMutationIDs = 1000
)

// ListNotificationsRequest is the grouped list query contract (P-S6-01):
// Limit zero selects the per-surface default; Category and ReadState are
// the "filter" control's working-candidate resolution — list-query
// parameters over the closed registers (the recorded gap (f) stays the
// owner's decision); Cursor is the opaque continuation cursor (empty = the
// first page).
type ListNotificationsRequest struct {
	Limit     int
	Category  string
	ReadState string
	Cursor    string
}

// ListNotificationsResult is one bounded page of group entries. NextCursor
// is the opaque cursor for the next page; empty marks the final page — no
// has_more/total vocabulary exists (the card-9 working candidate finalizes
// at the freeze).
type ListNotificationsResult struct {
	Entries    []notification.GroupEntry
	NextCursor string
}

// ListNotifications returns one bounded page of notification GROUPS in the
// Core-defined order — created_at descending (newest first), the
// notification_id ascending tiebreak for determinism — server-side and
// permission-filtered through the same decision port every read authorizes
// (C2-14: Local mode stays role-free; the server-side list is exactly as
// permission-filtered as its read port). The read creates nothing and
// mutates nothing.
func (s *Slice) ListNotifications(ctx context.Context, req ListNotificationsRequest) (ListNotificationsResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Notifications == nil {
		return ListNotificationsResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "notification store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceNotification}, security.ActionDataRead); err != nil {
		return ListNotificationsResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultNotificationListLimit
	}
	if limit < 1 || limit > MaxNotificationListLimit {
		return ListNotificationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	if req.Category != "" && !notification.ValidCategory(notification.Category(req.Category)) {
		return ListNotificationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "category"})
	}
	if req.ReadState != "" && !notification.ValidReadState(notification.ReadState(req.ReadState)) {
		return ListNotificationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "read_state"})
	}
	q := notification.ListNotificationsQuery{Limit: limit, Category: req.Category, ReadState: req.ReadState}
	if req.Cursor != "" {
		pos, err := decodeNotificationCursor(req.Cursor)
		if err != nil {
			return ListNotificationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterNotificationID = pos.CreatedAt, pos.NotificationID
	}
	page, err := s.deps.Notifications.ListNotificationGroups(ctx, q)
	if err != nil {
		return ListNotificationsResult{}, err
	}
	res := ListNotificationsResult{Entries: page.Entries}
	if page.More {
		if len(page.Entries) == 0 || page.AfterCreatedAt == "" || page.AfterNotificationID == "" {
			return ListNotificationsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeNotificationCursor(page.AfterCreatedAt, page.AfterNotificationID)
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return ListNotificationsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// NotificationUnreadCounts reads the Core-supplied unread counts (P-S6-01:
// unread counts render only from Core-supplied state — never locally
// counted by the UI). A pure read — it creates nothing and mutates nothing.
// Whether one list query serves both the panel and the counts read, or two,
// is the recorded O-2 gap — the separate counts read is the working
// candidate's resolution, finalized at the freeze.
func (s *Slice) NotificationUnreadCounts(ctx context.Context) (notification.UnreadCounts, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Notifications == nil {
		return notification.UnreadCounts{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "notification store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceNotification}, security.ActionDataRead); err != nil {
		return notification.UnreadCounts{}, err
	}
	return s.deps.Notifications.UnreadNotificationCounts(ctx)
}

// NotificationMutationRequest is one O-6 mutation's request: the explicit
// notification identities to apply the mutation to (1..1000 ids — the
// coded bound class). There is NO mark-all-read operation (not in the
// P-S6-02 named set) and no implicit target: the caller names exactly what
// it mutates.
type NotificationMutationRequest struct {
	NotificationIDs []string
}

// NotificationMutationResult is one mutation's typed outcome (P-S6-02):
// the rows that changed and the rows already in the requested state (the
// convergent no-op). Refusals ride the D-35 envelope.
type NotificationMutationResult struct {
	notification.MutationOutcome
}

// MarkNotificationsRead applies the mark-read mutation (P-S6-02): a typed
// Core outcome over exactly the presented identities — ALL-OR-NOTHING (an
// unknown id is the typed NOT_FOUND and nothing is written; a cleared id
// refuses CONFLICT_STATE — a dismissed row's read state is never
// resurrected). The mutation NEVER touches the underlying Job, approval,
// audit, or security record (the non-mutation law above; pinned by test).
// NO auto-retry exists: the idempotency class stays Open (no C1-07 row);
// the mutation is state-convergent by nature, and a repeat yields the same
// state without creating or destroying anything.
func (s *Slice) MarkNotificationsRead(ctx context.Context, req NotificationMutationRequest) (NotificationMutationResult, error) {
	outcome, err := s.notificationMutation(ctx, "mark read", func() (notification.MutationOutcome, error) {
		return s.deps.Notifications.MarkNotificationsRead(ctx, req.NotificationIDs)
	}, req.NotificationIDs)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	return NotificationMutationResult{MutationOutcome: outcome}, nil
}

// MarkNotificationsUnread applies the mark-unread mutation (the read_at
// stamp clears — the unread state has no read instant). The same
// all-or-nothing, typed-outcome, non-mutation law as mark read.
func (s *Slice) MarkNotificationsUnread(ctx context.Context, req NotificationMutationRequest) (NotificationMutationResult, error) {
	outcome, err := s.notificationMutation(ctx, "mark unread", func() (notification.MutationOutcome, error) {
		return s.deps.Notifications.MarkNotificationsUnread(ctx, req.NotificationIDs)
	}, req.NotificationIDs)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	return NotificationMutationResult{MutationOutcome: outcome}, nil
}

// ClearNotifications applies the clear (dismiss) mutation — the soft-hide
// working candidate (the recorded gap: soft-hide vs durable deletion stays
// the owner's decision; the non-mutation law bounds either reading). A
// dismissed row is retained, excluded from the list and the unread counts,
// and never resurrected. The underlying Job, Activity/audit, approval, or
// security record is NEVER deleted and NEVER changed by a dismiss (the
// non-mutation law above; pinned by test).
func (s *Slice) ClearNotifications(ctx context.Context, req NotificationMutationRequest) (NotificationMutationResult, error) {
	outcome, err := s.notificationMutation(ctx, "clear", func() (notification.MutationOutcome, error) {
		return s.deps.Notifications.ClearNotifications(ctx, req.NotificationIDs)
	}, req.NotificationIDs)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	return NotificationMutationResult{MutationOutcome: outcome}, nil
}

// notificationMutation is the shared mutation frame: authorize (the ONE
// decision port, ActionDataWrite — every Core mutation's own law) → the
// id-set shape law → exactly ONE store call. NO auto-retry anywhere: a
// store failure returns the typed error and the caller re-resolves (the
// idempotency class stays Open; pinned by test).
func (s *Slice) notificationMutation(ctx context.Context, what string, apply func() (notification.MutationOutcome, error), ids []string) (notification.MutationOutcome, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Notifications == nil {
		return notification.MutationOutcome{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "notification store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceNotification}, security.ActionDataWrite); err != nil {
		return notification.MutationOutcome{}, err
	}
	if len(ids) == 0 {
		return notification.MutationOutcome{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "notification_ids"})
	}
	if len(ids) > MaxNotificationMutationIDs {
		return notification.MutationOutcome{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "notification_ids"})
	}
	for _, id := range ids {
		if id == "" || len(id) > 128 {
			return notification.MutationOutcome{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "notification_ids"})
		}
	}
	return apply()
}

// notificationCursorPosition is the decoded form behind the opaque
// notification-list cursor: the LAST CONSUMED RAW ROW's keyset position
// (the grouped collapse consumes raw rows newest-first, so the raw
// position is what the next page continues from — a page window can never
// duplicate or drop a row). It follows the coded event-cursor PATTERN — a
// salt-prefixed hex-JSON envelope, never structured data on the wire
// (events.EncodeCursor) — as a SEPARATE query-side mechanism (P1.7 §3.5):
// the event family's cursor is untouched (the F10 freeze stands). Carrying
// only the keyset, it is a pure position, not a retention-bound token.
type notificationCursorPosition struct {
	CreatedAt      string `json:"created_at"`
	NotificationID string `json:"notification_id"`
}

func encodeNotificationCursor(createdAt, notificationID string) (string, error) {
	if createdAt == "" || notificationID == "" {
		return "", fmt.Errorf("app: notification cursor position lacks its keyset values")
	}
	b, err := json.Marshal(notificationCursorPosition{CreatedAt: createdAt, NotificationID: notificationID})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

func decodeNotificationCursor(cursor string) (notificationCursorPosition, error) {
	var pos notificationCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: notification cursor is malformed")
	}
	if pos.CreatedAt == "" || pos.NotificationID == "" {
		return pos, fmt.Errorf("app: notification cursor is malformed")
	}
	return pos, nil
}
