// Package notification holds the canonical notification-record contract
// (the ADOPTED S6 supply, P-S6-01…07 — the owner's LIVE S6 approval gate of
// 2026-11-03, implemented as working candidates per the IMPL-4..10
// discipline: nothing finalized, every gap carried). The notification list
// is a Core query over Core-owned notification state — server-side,
// permission-filtered through the one decision port, cursor-paginated per
// the adopted card-9 working candidate (the coded Jobs-list/conversation
// list pattern: bounded limits, a Core-defined order, the
// empty-cursor-marks-final protocol, no has_more/total vocabulary).
//
// Identity and dedup (P-S6-03): a notification's identity is Core-issued
// and stable; deduplication is claimed ONLY by the stable Core event
// identity (the source event's journal event_id) where the producer
// provides one — a re-delivered event never duplicates its notification.
// Where no stable identity is provided, NO replay-safe deduplication is
// claimed for that notification.
//
// Grouping (the recorded rule, UI §23.3): the list groups by related
// object; repeated identical updates collapse WITHOUT hiding a meaningful
// state transition; each entry keeps the newest timestamp, the unread
// count, and the current canonical state the owning domain supplies. The
// concrete mechanics (the collapse key, the page-scoped window) are the
// recorded owners' Proposed item (P3.8 §11 row 4) — this implementation is
// the working candidate recorded for the freeze.
//
// The four-way state distinction (P3.8 §5.4, non-color): unread | read |
// stale | permission_revoked. unread/read are the Core-supplied stored read
// state; stale is computed when the related object no longer resolves; the
// permission_revoked VALUE is carried in the register, and its computation
// joins additively when a mode with per-object permissions exists (Team
// mode, D-36) — in the single-user Local slice no per-object denial
// dimension exists, so no row computes to it here (recorded honestly, never
// faked).
//
// The non-mutation law (P-S6-02; P3.8 §9; the IMPL-7 requirement 6): a
// notification read/dismiss is NOT approval, acknowledgement, or work
// completion, and NEVER changes the underlying Job, Activity, approval, or
// security record. Nothing in this package or its consumers writes any
// underlying-domain record from a notification mutation.
//
// Privacy (P-S6-06): every notification record carries safe metadata only —
// NO prompt text, NO file path, NO token, NO generated output, NO private
// model data, NO secret value. No every-token/heartbeat notification: only
// the Phase 3 producer scope's meaningful, backend-originated transitions
// produce records.
//
// ABSENT BY RECORDED GATE: the OS delivery channel (decision 6: in-app ON +
// OS OFF by default; no OS-permission request, no OS delivery code, no
// quiet hours in v1 — the UI-34 FINAL freeze stays its own Checkpoint-C
// act), and the stream transport (P-S6-04: the UI-26/C-6 gate's decision —
// no streaming notification endpoint, no notification stream event type
// exists anywhere; the in-app list read is the panel's authority).
package notification

import "fmt"

// Category is the closed notification-category register. The register
// carries ONLY the categories with a coded Phase 3 producer (the
// register-extension class: a value joins additively when its producer
// milestone implements it, never before). In this milestone that is exactly
// one: the "Job result/failure/approval" category (P3.8 §5.1 row 1 — the
// AIJobRelay/Job event contract supplies the producer). The
// provider-connection/credential-health and Core-connection/health
// categories carry NO coded producer event today (verified by search — the
// connections surface writes no journal event; the health surface reports
// status reads), so no producer is implemented for them; the Knowledge and
// Plugin categories are RESERVED by P3.8 §5.1 (no Phase 3 producer — their
// values join at their later Roadmap packages).
type Category string

// CategoryJob is the "Job result/failure/approval" category — the one
// category whose producer the coded event register supplies in Phase 3.
const CategoryJob Category = "job"

// ValidCategory reports membership in the closed category register.
func ValidCategory(c Category) bool { return c == CategoryJob }

// StateClass is the closed per-notification state-class register: the
// meaningful transition class the producer recorded — never prompt/output
// content (the privacy law bounds the record to safe metadata).
type StateClass string

const (
	// ClassResult is a meaningful successful result (job.succeeded).
	ClassResult StateClass = "result"
	// ClassFailure is a meaningful failure-class transition
	// (job.failed, job.timed_out, job.needs_reconciliation — a
	// reconciliation-required Job is a failure-class condition the user
	// must act on; the mapping is the working candidate recorded for the
	// freeze).
	ClassFailure StateClass = "failure"
	// ClassApprovalNeeded is an approval-required wait
	// (job.waiting{reason: approval} — the ONLY waiting reason that
	// notifies; backoff/rate-limit waits are transient operational states,
	// never notifications).
	ClassApprovalNeeded StateClass = "approval_needed"
)

// ValidStateClass reports membership in the closed state-class register.
func ValidStateClass(s StateClass) bool {
	switch s {
	case ClassResult, ClassFailure, ClassApprovalNeeded:
		return true
	}
	return false
}

// ReadState is the Core-supplied read state (P-S6-02): stored Core state,
// never locally fabricated; no local read-state authority exists.
type ReadState string

const (
	ReadUnread ReadState = "unread"
	ReadRead   ReadState = "read"
)

// ValidReadState reports membership in the closed read-state register.
func ValidReadState(s ReadState) bool { return s == ReadUnread || s == ReadRead }

// PresentationState is the four-way distinction the list payload carries
// (P3.8 §5.4 — rendered non-color by the UI: icon + text, never color
// alone). unread/read come from the stored Core read state; stale is
// computed when the related object no longer resolves; permission_revoked
// is the register's fourth value — its computation joins additively with a
// mode that has per-object permissions (Team mode, D-36); the single-user
// Local slice computes no row to it (recorded honestly, never faked).
type PresentationState string

const (
	PresentUnread            PresentationState = "unread"
	PresentRead              PresentationState = "read"
	PresentStale             PresentationState = "stale"
	PresentPermissionRevoked PresentationState = "permission_revoked"
)

// ValidPresentationState reports membership in the closed
// presentation-state register.
func ValidPresentationState(s PresentationState) bool {
	switch s {
	case PresentUnread, PresentRead, PresentStale, PresentPermissionRevoked:
		return true
	}
	return false
}

// Related-object types the Phase 3 producers emit (the canonical-destination
// reference — the owning domain's detail route is the UI's duty; access is
// rechecked on open there).
const (
	// ObjectJob is the AIJobRelay Job aggregate (the R-1 family's
	// `/jobs/:jobId` canonical destination).
	ObjectJob = "job"
)

// Notification is one Core-owned notification record (P-S6-03): the
// Core-issued stable identity, the category, the state-class, the
// related-object reference (the grouping key and canonical destination),
// the source event identity (the dedup key where provided), the read state,
// and the timestamps. Safe metadata only (the package privacy law).
type Notification struct {
	// NotificationID is the Core-issued stable notification identity (the
	// exact published identity scheme stays Open, O-1 — the `ntf-` prefix is
	// this working candidate's mint, recorded for the freeze).
	NotificationID string     `json:"notification_id"`
	Category       Category   `json:"category"`
	StateClass     StateClass `json:"state_class"`
	// RelatedObjectType / RelatedObjectID are the owning domain's object
	// reference: the grouping key (the recorded rule groups by related
	// object) and the canonical-destination target.
	RelatedObjectType string `json:"related_object_type"`
	RelatedObjectID   string `json:"related_object_id"`
	// SourceEventID is the stable Core event identity that raised this
	// notification (the event-journal event_id when the producer rides the
	// event plane). Dedup is claimed ONLY through this field; empty means
	// no stable identity was provided and NO dedup is claimed.
	SourceEventID string `json:"source_event_id,omitempty"`
	// CorrelationID is the correlation id of the event that raised the
	// notification (the correlation law: diagnostics trail and notification
	// resolve through one identity).
	CorrelationID string    `json:"correlation_id,omitempty"`
	ReadState     ReadState `json:"read_state"`
	CreatedAt     string    `json:"created_at"`
	ReadAt        string    `json:"read_at,omitempty"`
}

// CollapsedBlock is one collapsed block of repeated identical updates
// within a group: the same (related object, category, state-class) run
// collapsed to a count with its newest timestamp and its unread count. A
// meaningful state transition (a different state-class on the same object)
// is a DIFFERENT block — never hidden by the collapse (the recorded rule).
type CollapsedBlock struct {
	StateClass  StateClass `json:"state_class"`
	Count       int        `json:"count"`
	UnreadCount int        `json:"unread_count"`
	NewestAt    string     `json:"newest_at"`
}

// GroupEntry is one list entry: one related object's group (the recorded
// grouping rule as the working candidate's mechanics). The entry keeps the
// newest timestamp (the representative notification's), the group's unread
// count, the group's total, the collapsed blocks (every meaningful
// transition visible), the current canonical state where the owning domain
// supplies it, and the four-way presentation state.
type GroupEntry struct {
	// NotificationID is the group's representative — the group's newest
	// notification's Core-issued identity.
	NotificationID    string     `json:"notification_id"`
	Category          Category   `json:"category"`
	StateClass        StateClass `json:"state_class"`
	RelatedObjectType string     `json:"related_object_type"`
	RelatedObjectID   string     `json:"related_object_id"`
	// CreatedAt is the group's newest timestamp (the representative's).
	CreatedAt   string           `json:"created_at"`
	UnreadCount int              `json:"unread_count"`
	RepeatCount int              `json:"repeat_count"`
	Collapsed   []CollapsedBlock `json:"collapsed"`
	// ObjectState is the related object's current canonical state where the
	// owning domain supplies it (the Job's current state for job objects);
	// empty means the Core supplies none — rendered as Unavailable, never
	// invented (the P1.7 §5.3 truth law).
	ObjectState  string            `json:"object_state,omitempty"`
	Presentation PresentationState `json:"presentation"`
}

// ListNotificationsQuery is one server-side grouped list query (the
// adopted card-9 working candidate: server-side cursor pagination,
// per-surface default limits, a Core-defined sort). Limit zero is invalid
// at the port (the slice applies its per-surface default); Category and
// ReadState are the "filter" control's working-candidate resolution —
// list-query parameters over the closed registers (the recorded gap (f):
// query-parameter vs retained preference stays the owner's decision).
type ListNotificationsQuery struct {
	Limit     int
	Category  string // empty = all; else a registered category
	ReadState string // empty = all; else a registered read state
	// AfterCreatedAt / AfterNotificationID are the keyset position — the
	// LAST CONSUMED RAW ROW's sort-key values (empty = the first page).
	// The collapse consumes raw rows newest-first; the position rides the
	// raw row so a group split across a scan window never duplicates or
	// drops a row.
	AfterCreatedAt      string
	AfterNotificationID string
}

// GroupedListPage is one bounded page of group entries. More reports that
// a further page exists (the store's probe). AfterCreatedAt /
// AfterNotificationID carry the last consumed raw row's keyset position
// for the next page's cursor (empty when the page is final). No
// has_more/total vocabulary exists — the cursorless page IS the end marker.
type GroupedListPage struct {
	Entries             []GroupEntry
	More                bool
	AfterCreatedAt      string
	AfterNotificationID string
}

// UnreadCounts is the Core-supplied unread-count read (P-S6-01: unread
// counts render only from Core-supplied state — never locally counted by
// the UI). Cleared rows are excluded (they are dismissed from the list).
// Whether one list query serves both the panel and the counts read, or
// two, is the recorded O-2 gap — the separate counts read is this working
// candidate's resolution, finalized at the freeze.
type UnreadCounts struct {
	Total      int
	ByCategory map[string]int
}

// MutationOutcome is one notification mutation's typed outcome (P-S6-02:
// mark read/unread and clear are typed Core outcomes riding the D-35
// envelope on refusal). Updated counts rows that changed; Unchanged counts
// rows already in the requested state (the convergent no-op — recorded
// honestly: the mutation is state-convergent by nature, NOT a documented
// idempotency class; no C1-07 row exists and NO auto-retry is performed
// anywhere).
type MutationOutcome struct {
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// ValidateRelatedObject applies the related-object reference shape law.
func ValidateRelatedObject(objectType, objectID string) error {
	if objectType == "" {
		return fmt.Errorf("notification: related object type is required")
	}
	if objectID == "" {
		return fmt.Errorf("notification: related object id is required")
	}
	return nil
}
