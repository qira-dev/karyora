// Package conversation holds the canonical conversation-aggregate contract
// (the adopted S5 mapping, P-S5-01…05 — the owner's LIVE S5 approval of
// 2026-11-02, implemented as working candidates, never finalized). The
// conversation is a first-class, Core-owned durable aggregate: Core-stored,
// Core-authorized, carrying a Core-issued stable conversation identity
// (P-S5-01); its messages are authoritative Core state inside the aggregate
// with a stable per-message identity, an intra-conversation ordering key,
// and the correlation-ID linkage to the send that produced them (P-S5-02).
// The provider-layer `Message` (internal/domain/provider) is request-plan
// vocabulary — never the durable shape; the conversation's message contract
// is the Core's own (this package).
//
// The Ask send (P-S5-05) is a conversation-plane operation: it persists the
// user's message plus the exchange state bound to (conversation identity,
// correlation ID) and NEVER creates a Job or any durable Run (the X-1
// read-only law). Conversation-send idempotency is Open (no C1-07 row):
// there is NO idempotency key here and NO auto-retry anywhere — a lost send
// response resolves ONLY by the query-by-correlation-ID read (the S4 state),
// never a blind duplicate send.
//
// The stream discipline (P-S5-03) is adopted as BINDING RULES ONLY: dedup
// by event_id, reconcile by per-aggregate sequence, the stream never the
// source of truth, its absence never reads as success. The stream's
// TRANSPORT MECHANISM is a named owner gap (S5 §4 P-S5-03 gap (a)) and is
// implemented by NOTHING in this milestone — no streaming endpoint, no
// event types, no stop, no steering exists anywhere in this package's
// vocabulary.
//
// Lifecycle is absent by the UI-28 gate (the owner's decision 8):
// edit/regenerate/archive/delete/export and every revision/edit-history
// shape stay out of this contract through the first implementation
// increment. Attachments (P-S5-06…11) are their owners' supply — no
// attachment field exists here.
package conversation

import (
	"errors"
	"fmt"
)

// Conversation is one Core-owned conversation aggregate's durable record.
// Storage layout is the Core's implementation freedom — the mapping binds
// authority, not storage (P-S5-01). Safe metadata only (X-6 bounds it).
type Conversation struct {
	// ConversationID is the Core-issued stable conversation identity
	// (P-S5-01; the exact published identity scheme stays Open, O-1).
	ConversationID string `json:"conversation_id"`
	// Title is the conversation's display title. Title/rename behavior is
	// Core-support-conditional per the P3.5 §3 row 4 discipline; this
	// milestone implements creation-time titles only — no rename operation
	// exists.
	Title string `json:"title"`
	// CreatedAt / UpdatedAt are RFC 3339 instants. UpdatedAt advances when
	// the aggregate's messages advance (the list's Core-defined sort key).
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MessageRole is the closed per-message role register (P-S5-02). The
// register carries `user` ONLY: the answering/assistant production path is
// a named owner gap (S5 §4 P-S5-03 gap (a)) — its role value joins this
// register additively when its producer milestone implements it, never
// before (the recorded register-extension class). No revisions, no edit
// history: the UI-28 gate holds.
type MessageRole string

// MessageRoleUser is the user's own message (the send's persisted half).
const MessageRoleUser MessageRole = "user"

// ValidMessageRole reports membership in the closed role register.
func ValidMessageRole(r MessageRole) bool { return r == MessageRoleUser }

// Message is one authoritative conversation message (P-S5-02): a stable
// per-message identity; the intra-conversation ordering key (Seq — the
// per-aggregate sequence discipline of the P1.7 §2.1 class); the
// correlation-ID linkage to the send that produced it; safe metadata only.
type Message struct {
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	// Seq is the 1-based intra-conversation ordering key — monotonic within
	// one conversation, the timeline's authoritative order.
	Seq           int64       `json:"seq"`
	Role          MessageRole `json:"role"`
	Content       string      `json:"content"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	CreatedAt     string      `json:"created_at"`
}

// ExchangeState is the closed exchange-state register. The register carries
// `recorded` ONLY in this milestone: the send was durably recorded inside
// the aggregate. No answered/failed/streamed state exists — the
// answering/stream production path is the named owner gap, and no state
// value is invented for a producer that does not exist (the IMPL-7
// "no producer exists" pattern).
type ExchangeState string

// ExchangeRecorded is the one state this milestone can honestly carry: the
// user's message and the exchange linkage are durable inside the
// conversation aggregate. The S4 query returns exactly this state — the
// authoritative exchange state for a lost send response.
const ExchangeRecorded ExchangeState = "recorded"

// ValidExchangeState reports membership in the closed exchange-state
// register.
func ValidExchangeState(s ExchangeState) bool { return s == ExchangeRecorded }

// Exchange is the send's durable linkage row (P-S5-05): the exchange state
// bound to (conversation identity, correlation ID). The correlation ID is
// PRIMARY — it is the identity the S4 query-before-retry read resolves
// through; its uniqueness is what makes the query authoritative. It is NOT
// an idempotency key: the send-idempotency class stays Open, no C1-07 row.
type Exchange struct {
	CorrelationID  string        `json:"correlation_id"`
	ConversationID string        `json:"conversation_id"`
	UserMessageID  string        `json:"user_message_id"`
	State          ExchangeState `json:"state"`
	CreatedAt      string        `json:"created_at"`
}

// Bounds for the conversation contract (Proposed, pending the recorded
// owner review — the slice-bounds discipline).
const (
	// MaxTitleRunes bounds a conversation's title.
	MaxTitleRunes = 256
	// MaxMessageRunes bounds one message's content (the MaxSystemRunes
	// class of the slice bounds).
	MaxMessageRunes = 1 << 16
)

// ValidateTitle applies the title shape law (an optional title is allowed —
// the untitled conversation renders its own identity; a present title is
// bounded and never only whitespace).
func ValidateTitle(title string) error {
	if len([]rune(title)) > MaxTitleRunes {
		return fmt.Errorf("conversation: title exceeds %d runes", MaxTitleRunes)
	}
	return nil
}

// ValidateMessageContent applies the message content shape law.
func ValidateMessageContent(content string) error {
	runes := []rune(content)
	if len(runes) == 0 {
		return errors.New("conversation: message content is required")
	}
	if len(runes) > MaxMessageRunes {
		return fmt.Errorf("conversation: message content exceeds %d runes", MaxMessageRunes)
	}
	return nil
}

// ListConversationsQuery is one server-side list/search query (the adopted
// card-9 working candidate: server-side cursor pagination, per-surface
// default limits, Core-defined sort keys). Query empty = the plain list;
// non-empty = the server-side search over the Core-owned store. The
// list-vs-search split (one query or two) is the recorded O-1/O-2 gap —
// this single-query form is the working candidate's resolution, finalized
// at the freeze.
type ListConversationsQuery struct {
	Limit int
	// Query is the search term (empty = the unfiltered list). Matching is
	// server-side over the store's own machinery — no dedicated index (the
	// index-vs-store choice is the Core/API owner's implementation freedom;
	// the code supplies no conversation index).
	Query string
	// AfterUpdatedAt / AfterConversationID are the keyset position (the
	// last page's sort-key values; empty = the first page).
	AfterUpdatedAt      string
	AfterConversationID string
}

// ListConversationsPage is one bounded page. More reports that a further
// page exists (the adapter's internal limit+1 probe). No has_more/total
// vocabulary exists — the cursorless page IS the end marker.
type ListConversationsPage struct {
	Conversations []Conversation
	More          bool
}

// MessagePageQuery is one bounded messages read (chronological — the
// timeline's order; keyset pagination on the ordering key).
type MessagePageQuery struct {
	ConversationID string
	Limit          int
	// AfterSeq is the keyset position (0 = the first page).
	AfterSeq int64
}

// MessagePage is one bounded messages page (Seq ascending — chronological).
type MessagePage struct {
	Messages []Message
	More     bool
}
