package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/notification"
)

// The notification-record store (the ADOPTED S6 supply, P-S6-01…03 — the
// Core-side notification machinery implemented as working candidates over
// migration 0010). The list is a server-side, Core-defined query over the
// Core-owned notification state: the grouped collapse walk (the recorded
// grouping rule as the working candidate's mechanics — one entry per
// CONSECUTIVE run of a related object in the Core-defined newest-first
// order; repeated identical updates collapse into counted blocks; a
// meaningful state transition is its own block, never hidden; each entry
// keeps the newest timestamp, the unread count, and the current canonical
// state the owning domain supplies). Keyset pagination rides the LAST
// CONSUMED RAW ROW's position, so a page window can never duplicate or drop
// a row; page-scoped counts are honest windows, not totals (the Core
// unread-count read is the totals' authority).
//
// The four-way presentation state: stale is computed when the related
// object's row no longer resolves (the owning domain's join miss); the
// permission_revoked VALUE stays in the closed register and its computation
// joins additively with a mode that has per-object permissions (Team mode,
// D-36) — the single-user Local slice computes no row to it. The no-leak
// class binds: a stale entry carries no canonical state and no object
// detail beyond its own reference.
//
// Every mutation writes ONLY notification rows — it NEVER touches the
// underlying Job, Activity/audit, approval, or security record (the
// non-mutation law, P-S6-02 / P3.8 §9; the IMPL-7 requirement 6; pinned by
// test). Cleared rows are RETAINED (the soft-hide working candidate — the
// recorded gap: soft-hide vs durable deletion stays the owner's decision;
// no retention value is implemented — every retention value stays Proposed
// until its own acceptance).

// notificationMaxMutationIDs bounds one mutation's id set (the same
// 1–1000 bound class the other bounded sets use).
const notificationMaxMutationIDs = 1000

const notificationColumns = `notification_id, category, state_class, related_object_type, related_object_id, source_event_id, correlation_id, read_state, created_at, read_at`

func scanNotification(row rowScanner) (notification.Notification, error) {
	var n notification.Notification
	err := row.Scan(&n.NotificationID, &n.Category, &n.StateClass, &n.RelatedObjectType,
		&n.RelatedObjectID, &n.SourceEventID, &n.CorrelationID, &n.ReadState, &n.CreatedAt, &n.ReadAt)
	if err != nil {
		return n, err
	}
	if !notification.ValidCategory(n.Category) {
		return notification.Notification{}, errStoragef(errs.CodeStateCorrupt, "notification %q carries an unregistered category", n.NotificationID)
	}
	if !notification.ValidStateClass(n.StateClass) {
		return notification.Notification{}, errStoragef(errs.CodeStateCorrupt, "notification %q carries an unregistered state class", n.NotificationID)
	}
	if !notification.ValidReadState(n.ReadState) {
		return notification.Notification{}, errStoragef(errs.CodeStateCorrupt, "notification %q carries an unregistered read state", n.NotificationID)
	}
	return n, nil
}

// notificationScanCap is the raw-row scan window for one grouped page: the
// collapse consumes raw rows newest-first until `limit` entries are formed
// or the window ends. A window edge can split a long run — the next page
// continues it (page-scoped counts, the cursor riding the last consumed
// raw row, nothing duplicated or dropped).
func notificationScanCap(limit int) int {
	cap_ := limit * 8
	if cap_ < 200 {
		cap_ = 200
	}
	if cap_ > 5000 {
		cap_ = 5000
	}
	return cap_
}

// notificationRun is one consecutive run of one related object within the
// scan window, with the raw keyset position of its last consumed row.
type notificationRun struct {
	rows               []notification.Notification
	lastCreatedAt      string
	lastNotificationID string
}

// ListNotificationGroups implements the server-side grouped list read
// (the adopted card-9 working candidate): the Core-defined order
// (created_at descending — newest first, the notification_id ascending
// tiebreak for determinism), the grouped collapse (one entry per
// consecutive run of one related object; repeated identical updates
// collapse into counted blocks; a meaningful state transition is its own
// block), the current canonical state joined from the owning domain (the
// Job's current state; a missing object row computes the stale
// presentation — never invented), and the keyset pagination over the last
// consumed raw row's position. Fail-closed guards: a non-positive limit,
// an unregistered filter value, and a keyset position without its
// tiebreak refuse.
func (s *Store) ListNotificationGroups(ctx context.Context, q notification.ListNotificationsQuery) (notification.GroupedListPage, error) {
	if q.Limit < 1 {
		return notification.GroupedListPage{}, errStoragef(errs.CodeValidationFailed, "list limit must be at least 1")
	}
	if q.Category != "" && !notification.ValidCategory(notification.Category(q.Category)) {
		return notification.GroupedListPage{}, errStoragef(errs.CodeValidationFailed, "category %q is not a registered notification category", q.Category)
	}
	if q.ReadState != "" && !notification.ValidReadState(notification.ReadState(q.ReadState)) {
		return notification.GroupedListPage{}, errStoragef(errs.CodeValidationFailed, "read state %q is not a registered read state", q.ReadState)
	}
	where := ` WHERE cleared = 0`
	args := []any{}
	if q.Category != "" {
		where += ` AND category = ?`
		args = append(args, q.Category)
	}
	if q.ReadState != "" {
		where += ` AND read_state = ?`
		args = append(args, q.ReadState)
	}
	if q.AfterCreatedAt != "" || q.AfterNotificationID != "" {
		if q.AfterCreatedAt == "" || q.AfterNotificationID == "" {
			return notification.GroupedListPage{}, errStoragef(errs.CodeValidationFailed, "a keyset position requires its notification_id tiebreak")
		}
		where += ` AND (n.created_at < ? OR (n.created_at = ? AND n.notification_id > ?))`
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterNotificationID)
	}
	scanCap := notificationScanCap(q.Limit)
	args = append(args, scanCap+1)
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	// The notification columns are qualified with the alias: the jobs join
	// carries its own correlation_id/created_at columns, and an unqualified
	// reference would be ambiguous.
	rows, err := s.readers.QueryContext(cctx, `
SELECT n.notification_id, n.category, n.state_class, n.related_object_type, n.related_object_id,
       n.source_event_id, n.correlation_id, n.read_state, n.created_at, n.read_at,
       CASE WHEN j.job_id IS NULL THEN '' ELSE j.state END AS object_state,
       CASE WHEN j.job_id IS NULL THEN 0 ELSE 1 END AS object_resolved
FROM notification_records n
LEFT JOIN jobs j ON n.related_object_type = 'job' AND j.job_id = n.related_object_id
`+where+` ORDER BY n.created_at DESC, n.notification_id ASC LIMIT ?`, args...)
	if err != nil {
		return notification.GroupedListPage{}, mapVendorError(err)
	}
	defer rows.Close()
	type scannedRow struct {
		n        notification.Notification
		state    string
		resolved bool
	}
	var scanned []scannedRow
	for rows.Next() {
		var r scannedRow
		if err := rows.Scan(&r.n.NotificationID, &r.n.Category, &r.n.StateClass, &r.n.RelatedObjectType,
			&r.n.RelatedObjectID, &r.n.SourceEventID, &r.n.CorrelationID, &r.n.ReadState, &r.n.CreatedAt, &r.n.ReadAt,
			&r.state, &r.resolved); err != nil {
			return notification.GroupedListPage{}, mapVendorError(err)
		}
		if !notification.ValidCategory(r.n.Category) || !notification.ValidStateClass(r.n.StateClass) || !notification.ValidReadState(r.n.ReadState) {
			return notification.GroupedListPage{}, errStoragef(errs.CodeStateCorrupt, "notification %q carries an unregistered register value", r.n.NotificationID)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		return notification.GroupedListPage{}, mapVendorError(err)
	}
	probeExists := len(scanned) > scanCap
	if probeExists {
		scanned = scanned[:scanCap]
	}
	// The consecutive-run collapse: a maximal run of one related object in
	// the scanned order forms one entry; within the run, a maximal run of
	// one (category, state-class) forms one collapsed block.
	var runs []notificationRun
	for _, r := range scanned {
		if len(runs) > 0 {
			last := &runs[len(runs)-1]
			tail := last.rows[len(last.rows)-1]
			if tail.RelatedObjectType == r.n.RelatedObjectType && tail.RelatedObjectID == r.n.RelatedObjectID {
				last.rows = append(last.rows, r.n)
				last.lastCreatedAt = r.n.CreatedAt
				last.lastNotificationID = r.n.NotificationID
				continue
			}
		}
		runs = append(runs, notificationRun{
			rows:               []notification.Notification{r.n},
			lastCreatedAt:      r.n.CreatedAt,
			lastNotificationID: r.n.NotificationID,
		})
	}
	// Track each run's owning-object state/resolution for the presentation.
	states := map[string]scannedObject{}
	for _, r := range scanned {
		key := r.n.RelatedObjectType + "\x00" + r.n.RelatedObjectID
		if _, ok := states[key]; !ok {
			states[key] = scannedObject{state: r.state, resolved: r.resolved}
		}
	}
	more := false
	kept := runs
	if len(runs) > q.Limit {
		kept = runs[:q.Limit]
		more = true
	} else if probeExists && len(runs) > 0 {
		more = true
	}
	page := notification.GroupedListPage{Entries: []notification.GroupEntry{}}
	if more && len(kept) > 0 {
		tail := kept[len(kept)-1]
		page.More = true
		page.AfterCreatedAt = tail.lastCreatedAt
		page.AfterNotificationID = tail.lastNotificationID
	}
	for _, run := range kept {
		entry := buildGroupEntry(run.rows, states)
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}

// scannedObject is one scanned raw row's owning-object resolution record:
// the current canonical state the owning domain supplies (the Job's
// current state) and whether the object's row still resolves (a miss
// computes the stale presentation).
type scannedObject struct {
	state    string
	resolved bool
}

// buildGroupEntry collapses one run into its list entry: the representative
// (the newest row), the run's unread count and total, the collapsed blocks
// (every meaningful transition visible), the current canonical state where
// the owning domain supplies it, and the four-way presentation state.
func buildGroupEntry(rows []notification.Notification, states map[string]scannedObject) notification.GroupEntry {
	head := rows[0]
	key := head.RelatedObjectType + "\x00" + head.RelatedObjectID
	owning := states[key]
	entry := notification.GroupEntry{
		NotificationID:    head.NotificationID,
		Category:          head.Category,
		StateClass:        head.StateClass,
		RelatedObjectType: head.RelatedObjectType,
		RelatedObjectID:   head.RelatedObjectID,
		CreatedAt:         head.CreatedAt,
		RepeatCount:       len(rows),
		Collapsed:         []notification.CollapsedBlock{},
	}
	for _, r := range rows {
		if r.ReadState == notification.ReadUnread {
			entry.UnreadCount++
		}
		if len(entry.Collapsed) > 0 {
			block := &entry.Collapsed[len(entry.Collapsed)-1]
			if block.StateClass == r.StateClass {
				block.Count++
				if r.ReadState == notification.ReadUnread {
					block.UnreadCount++
				}
				// The rows are newest-first; the block's newest timestamp is
				// the run's first row of this class (already set).
				continue
			}
		}
		entry.Collapsed = append(entry.Collapsed, notification.CollapsedBlock{
			StateClass:  r.StateClass,
			Count:       1,
			UnreadCount: boolInt(r.ReadState == notification.ReadUnread),
			NewestAt:    r.CreatedAt,
		})
	}
	// The four-way presentation state: an unresolvable related object is
	// stale (no canonical state supplied — never invented); otherwise the
	// stored Core read state. permission_revoked stays the register's
	// fourth value: its computation joins additively with a mode that has
	// per-object permissions (Team mode, D-36); the single-user Local slice
	// computes no row to it.
	if !owning.resolved {
		entry.Presentation = notification.PresentStale
		entry.ObjectState = ""
	} else {
		if head.ReadState == notification.ReadUnread {
			entry.Presentation = notification.PresentUnread
		} else {
			entry.Presentation = notification.PresentRead
		}
		entry.ObjectState = owning.state
	}
	return entry
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UnreadNotificationCounts is the Core-supplied unread-count read: per
// category and total, from the stored Core read state only. Cleared rows
// are excluded (dismissed from the list). Whether one list query serves
// both the panel and the counts read, or two, is the recorded O-2 gap —
// the separate counts read is the working candidate's resolution.
func (s *Store) UnreadNotificationCounts(ctx context.Context) (notification.UnreadCounts, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx, `
SELECT category, COUNT(*) FROM notification_records
WHERE read_state = 'unread' AND cleared = 0 GROUP BY category`)
	if err != nil {
		return notification.UnreadCounts{}, mapVendorError(err)
	}
	defer rows.Close()
	counts := notification.UnreadCounts{ByCategory: map[string]int{}}
	for rows.Next() {
		var category string
		var n int
		if err := rows.Scan(&category, &n); err != nil {
			return notification.UnreadCounts{}, mapVendorError(err)
		}
		if !notification.ValidCategory(notification.Category(category)) {
			return notification.UnreadCounts{}, errStoragef(errs.CodeStateCorrupt, "notification row carries an unregistered category")
		}
		counts.ByCategory[category] = n
		counts.Total += n
	}
	if err := rows.Err(); err != nil {
		return notification.UnreadCounts{}, mapVendorError(err)
	}
	return counts, nil
}

// MarkNotificationsRead applies the mark-read mutation to exactly the
// presented identities (all-or-nothing; see mutateNotificationReadState).
func (s *Store) MarkNotificationsRead(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	return s.mutateNotificationReadState(ctx, ids, notification.ReadRead)
}

// MarkNotificationsUnread applies the mark-unread mutation (the read_at
// stamp clears — the unread state has no read instant).
func (s *Store) MarkNotificationsUnread(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	return s.mutateNotificationReadState(ctx, ids, notification.ReadUnread)
}

// mutateNotificationReadState applies a read-state transition to exactly
// the presented notification identities — ALL-OR-NOTHING in ONE
// transaction: every id must resolve to a stored, uncleared notification
// row (an unknown id is the typed NOT_FOUND and NOTHING is written; a
// cleared id is the typed CONFLICT_STATE — a dismissed row's read state is
// never resurrected). Rows already in the requested state are convergent
// no-ops (counted Unchanged) — the mutation is state-convergent by nature;
// NO idempotency-key or replay semantics are claimed (the class stays
// Open, no C1-07 row) and NO auto-retry exists anywhere (the caller
// re-resolves; pinned by test). The mutation writes ONLY notification
// rows — it NEVER touches the underlying Job, approval, audit, or security
// record (the non-mutation law, P-S6-02 / P3.8 §9; pinned by test).
func (s *Store) mutateNotificationReadState(ctx context.Context, ids []string, to notification.ReadState) (notification.MutationOutcome, error) {
	if len(ids) == 0 {
		return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a read-state mutation requires at least one notification id")
	}
	if len(ids) > notificationMaxMutationIDs {
		return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a read-state mutation accepts at most %d ids", notificationMaxMutationIDs)
	}
	for _, id := range ids {
		if id == "" {
			return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a read-state mutation requires non-empty notification ids")
		}
	}
	tx, err := s.writers.BeginTx(ctx, nil)
	if err != nil {
		return notification.MutationOutcome{}, mapVendorError(err)
	}
	defer func() { _ = tx.Rollback() }()
	outcome := notification.MutationOutcome{}
	for _, id := range ids {
		var currentState string
		var cleared int
		err := tx.QueryRowContext(ctx, `SELECT read_state, cleared FROM notification_records WHERE notification_id = ?`, id).Scan(&currentState, &cleared)
		if err == sql.ErrNoRows {
			return notification.MutationOutcome{}, errStoragef(errs.CodeNotFound, "notification %q is not stored", id)
		}
		if err != nil {
			return notification.MutationOutcome{}, mapVendorError(err)
		}
		if !notification.ValidReadState(notification.ReadState(currentState)) {
			return notification.MutationOutcome{}, errStoragef(errs.CodeStateCorrupt, "notification %q carries an unregistered read state", id)
		}
		if cleared == 1 {
			return notification.MutationOutcome{}, errStoragef(errs.CodeConflictState, "notification %q is cleared; its read state is never resurrected", id)
		}
		if notification.ReadState(currentState) == to {
			outcome.Unchanged++
			continue
		}
		readAt := ""
		if to == notification.ReadRead {
			readAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE notification_records SET read_state = ?, read_at = ? WHERE notification_id = ?`,
			string(to), readAt, id); err != nil {
			return notification.MutationOutcome{}, mapVendorError(err)
		}
		outcome.Updated++
	}
	if err := tx.Commit(); err != nil {
		return notification.MutationOutcome{}, mapVendorError(err)
	}
	return outcome, nil
}

// ClearNotifications dismisses the presented notifications from the list —
// the soft-hide working candidate (the recorded gap: soft-hide vs durable
// deletion stays the owner's decision; the non-mutation law bounds either
// reading). Cleared rows are RETAINED (no retention value is implemented —
// every retention value stays Proposed until its own acceptance), excluded
// from the list and the unread counts, and never resurrected by a later
// read-state mutation. An unknown id is the typed NOT_FOUND and NOTHING is
// written; an already-cleared row is a convergent no-op. The underlying
// Job, approval, audit, or security record is NEVER touched (the
// non-mutation law; pinned by test).
func (s *Store) ClearNotifications(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	if len(ids) == 0 {
		return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a clear mutation requires at least one notification id")
	}
	if len(ids) > notificationMaxMutationIDs {
		return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a clear mutation accepts at most %d ids", notificationMaxMutationIDs)
	}
	for _, id := range ids {
		if id == "" {
			return notification.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "a clear mutation requires non-empty notification ids")
		}
	}
	tx, err := s.writers.BeginTx(ctx, nil)
	if err != nil {
		return notification.MutationOutcome{}, mapVendorError(err)
	}
	defer func() { _ = tx.Rollback() }()
	outcome := notification.MutationOutcome{}
	for _, id := range ids {
		var cleared int
		err := tx.QueryRowContext(ctx, `SELECT cleared FROM notification_records WHERE notification_id = ?`, id).Scan(&cleared)
		if err == sql.ErrNoRows {
			return notification.MutationOutcome{}, errStoragef(errs.CodeNotFound, "notification %q is not stored", id)
		}
		if err != nil {
			return notification.MutationOutcome{}, mapVendorError(err)
		}
		if cleared == 1 {
			outcome.Unchanged++
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE notification_records SET cleared = 1, cleared_at = ? WHERE notification_id = ?`,
			time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			return notification.MutationOutcome{}, mapVendorError(err)
		}
		outcome.Updated++
	}
	if err := tx.Commit(); err != nil {
		return notification.MutationOutcome{}, mapVendorError(err)
	}
	return outcome, nil
}
