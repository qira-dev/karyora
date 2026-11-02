package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The conversation surface (the ADOPTED S5 mapping — the owner's LIVE S5
// approval of 2026-11-02 implemented as working candidates per the
// IMPL-4..9 discipline: nothing finalized, every gap carried). Per draft:
//
//   - P-S5-01: the conversation is a first-class, Core-owned durable
//     aggregate — Core-stored (migration 0009, the storage adapter),
//     Core-authorized (the conversation resource row's additive register
//     join, every operation through the ONE decision port), carrying a
//     Core-issued stable conversation identity. A provider conversation
//     handle is optimization metadata only — never canonical identity (the
//     coded C3-10 law, internal/contextengine/pipeline.go:152); nothing
//     here carries one.
//   - P-S5-02: the messages are authoritative Core state inside the
//     aggregate — a stable per-message identity, an intra-conversation
//     ordering key (Seq), the correlation-ID linkage to the send that
//     produced each message, safe metadata only (X-6). No revisions, no
//     edit history (UI-28 gated, the owner's decision 8). The provider
//     layer's `Message` is request-plan vocabulary — never this shape.
//   - P-S5-03: the stream discipline is adopted as BINDING RULES ONLY —
//     dedup by event_id, reconcile by per-aggregate sequence, the stream
//     never the source of truth, its absence never reads as success. The
//     stream's TRANSPORT MECHANISM is a named owner gap (S5 §4 P-S5-03 gap
//     (a)) and is implemented by NOTHING here: no streaming endpoint, no
//     stream event type, no stop, no steering exists in this surface (the
//     route register is pinned so none can silently appear).
//   - P-S5-04: the list + search are Core queries over the Core-owned
//     store — server-side, permission-filtered through the decision port,
//     cursor-paginated per the adopted card-9 working candidate (the
//     IMPL-4 List pattern: bounded limits, a Core-defined order, the
//     empty-cursor-marks-final protocol, no has_more/total vocabulary).
//     The no-leak rule binds: an error never confirms that an inaccessible
//     conversation exists.
//   - P-S5-05: the Ask send is a conversation-plane operation inside the
//     aggregate — it persists the user's message plus the exchange state
//     bound to (conversation identity, correlation ID) in ONE transaction
//     and NEVER creates a Job or any durable Run (X-1, pinned by test).
//     There is NO auto-retry anywhere and NO send idempotency key (the
//     class stays Open): a lost send response resolves ONLY by the
//     query-by-correlation-ID read (the S4 state), never a blind duplicate
//     send. NO answer/assistant-message production path exists in this
//     milestone — the answering/stream production path's transport
//     mechanism is the named owner gap; the role and exchange-state
//     registers carry only what a producer-less milestone can honestly
//     carry (`user`; `recorded`).

// ConversationStore is the durable conversation-aggregate port (P-S5-01).
// The storage adapter implements it structurally; the slice consumes only
// this port. Nil disables the conversation surface — typed refusals, fail
// closed (the Connections pattern).
type ConversationStore interface {
	CreateConversation(ctx context.Context, rec conversation.Conversation) (conversation.Conversation, error)
	LoadConversation(ctx context.Context, conversationID string) (conversation.Conversation, error)
	// AppendConversationExchange persists ONE send atomically: the user's
	// message (its intra-conversation Seq assigned inside the transaction)
	// plus the exchange row bound to (conversation identity, correlation
	// ID). Nothing else is written; no Job is touched (X-1).
	AppendConversationExchange(ctx context.Context, x conversation.Exchange, content string) (conversation.Exchange, conversation.Message, error)
	ListConversations(ctx context.Context, q conversation.ListConversationsQuery) (conversation.ListConversationsPage, error)
	LoadMessagePage(ctx context.Context, q conversation.MessagePageQuery) (conversation.MessagePage, error)
	// LoadConversationExchange resolves the S4 query-by-correlation-ID read:
	// the exchange bound to (conversation identity, correlation ID) with
	// its linked user message, or the typed NOT_FOUND. An exchange bound to
	// a DIFFERENT conversation is the SAME typed NOT_FOUND as unknown (the
	// no-leak rule).
	LoadConversationExchange(ctx context.Context, conversationID, correlationID string) (conversation.Exchange, conversation.Message, error)
}

// Conversation bounds (Proposed, pending the recorded owner review — the
// slice-bounds discipline; the domain package carries the shape rules).
const (
	// DefaultConversationListLimit is the conversation-list per-surface
	// default page size (the card-9 per-surface default class).
	DefaultConversationListLimit = 50
	// MaxConversationListLimit is the list page bound — the same 1–1000
	// bound class the other read surfaces use.
	MaxConversationListLimit = 1000
)

// ConversationRecordRequest is the aggregate-creation request.
type ConversationRecordRequest struct {
	Title string
}

// CreateConversation creates one Core-owned conversation aggregate (the
// P3.5 §3 row 3 create — "only when supported by the Core conversation
// contract"; the adopted S5 mapping supplies it). The Core mints the stable
// conversation identity; the caller never presents one.
func (s *Slice) CreateConversation(ctx context.Context, req ConversationRecordRequest) (conversation.Conversation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return conversation.Conversation{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation}, security.ActionDataWrite); err != nil {
		return conversation.Conversation{}, err
	}
	if err := conversation.ValidateTitle(req.Title); err != nil {
		return conversation.Conversation{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "title"})
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = ""
	}
	now := s.deps.Now().UTC().Format(time.RFC3339Nano)
	rec := conversation.Conversation{
		ConversationID: "conv-" + randomID(),
		Title:          req.Title,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	saved, err := s.deps.Conversations.CreateConversation(ctx, rec)
	if err != nil {
		return conversation.Conversation{}, err
	}
	return saved, nil
}

// Conversation reads one conversation aggregate's record (safe metadata
// only). An unknown id is the typed NOT_FOUND — never a shape that could
// distinguish an inaccessible object (the no-leak rule).
func (s *Slice) Conversation(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return conversation.Conversation{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: conversationID}, security.ActionDataRead); err != nil {
		return conversation.Conversation{}, err
	}
	if conversationID == "" {
		return conversation.Conversation{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	return s.deps.Conversations.LoadConversation(ctx, conversationID)
}

// ListConversationsRequest is the list/search query contract (P-S5-04):
// Limit zero selects the per-surface default; Query empty = the plain
// list, non-empty = the server-side search; Cursor is the opaque
// continuation cursor (empty = the first page).
type ListConversationsRequest struct {
	Limit  int
	Query  string
	Cursor string
}

// ListConversationsResult is one bounded page. NextCursor is the opaque
// cursor for the next page; empty marks the final page — no has_more/total
// vocabulary exists (the card-9 working candidate finalizes at the freeze).
type ListConversationsResult struct {
	Conversations []conversation.Conversation
	NextCursor    string
}

// ListConversations returns one bounded page of conversations in the
// Core-defined order — updated_at descending (the most recently active
// first), the conversation_id ascending tiebreak for determinism —
// server-side and permission-filtered through the same decision port every
// read authorizes (C2-14: Local mode stays role-free; the server-side list
// is exactly as permission-filtered as its read port). The optional Query
// narrows server-side (title + message content, over the store's own
// machinery — no dedicated index exists and none is invented).
func (s *Slice) ListConversations(ctx context.Context, req ListConversationsRequest) (ListConversationsResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return ListConversationsResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation}, security.ActionDataRead); err != nil {
		return ListConversationsResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultConversationListLimit
	}
	if limit < 1 || limit > MaxConversationListLimit {
		return ListConversationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	if len([]rune(req.Query)) > conversation.MaxMessageRunes {
		return ListConversationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "query"})
	}
	q := conversation.ListConversationsQuery{Limit: limit, Query: req.Query}
	if req.Cursor != "" {
		pos, err := decodeConversationCursor(req.Cursor)
		if err != nil {
			return ListConversationsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterUpdatedAt, q.AfterConversationID = pos.UpdatedAt, pos.ConversationID
	}
	page, err := s.deps.Conversations.ListConversations(ctx, q)
	if err != nil {
		return ListConversationsResult{}, err
	}
	res := ListConversationsResult{Conversations: page.Conversations}
	if page.More {
		if len(page.Conversations) == 0 {
			return ListConversationsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeConversationCursor(page.Conversations[len(page.Conversations)-1])
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return ListConversationsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// ConversationMessagePageRequest is the messages read contract: the
// conversation's authoritative messages in timeline order (Seq ascending),
// one bounded page at a time.
type ConversationMessagePageRequest struct {
	Limit  int
	Cursor string
}

// ConversationMessagePageResult is one bounded messages page; NextCursor
// empty marks the final page.
type ConversationMessagePageResult struct {
	Messages   []conversation.Message
	NextCursor string
}

// ConversationMessages reads one conversation's authoritative messages
// (P-S5-02): the Core's stored state, never a client-side projection. The
// read creates nothing and mutates nothing.
func (s *Slice) ConversationMessages(ctx context.Context, conversationID string, req ConversationMessagePageRequest) (ConversationMessagePageResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return ConversationMessagePageResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: conversationID}, security.ActionDataRead); err != nil {
		return ConversationMessagePageResult{}, err
	}
	if conversationID == "" {
		return ConversationMessagePageResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultConversationListLimit
	}
	if limit < 1 || limit > MaxConversationListLimit {
		return ConversationMessagePageResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	q := conversation.MessagePageQuery{ConversationID: conversationID, Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeConversationMessageCursor(req.Cursor)
		if err != nil {
			return ConversationMessagePageResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterSeq = pos.Seq
	}
	page, err := s.deps.Conversations.LoadMessagePage(ctx, q)
	if err != nil {
		return ConversationMessagePageResult{}, err
	}
	res := ConversationMessagePageResult{Messages: page.Messages}
	if page.More {
		if len(page.Messages) == 0 {
			return ConversationMessagePageResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = encodeConversationMessageCursor(page.Messages[len(page.Messages)-1].Seq)
	}
	return res, nil
}

// ConversationSendRequest is one Ask send (P-S5-05): the conversational
// request content plus the send's correlation ID — the request identity the
// P1.6 §3.9 M-1 register fixes ("Request identity: Correlation ID"): the
// caller carries it, the Core records it on the exchange, and the S4
// query-before-retry read resolves a lost response through it. It is NOT an
// idempotency key: the send-idempotency class stays Open (no C1-07 row), no
// replay semantics are claimed, and a re-presented correlation ID on an
// already-recorded exchange is refused — never a duplicate exchange, never
// a replay claim.
type ConversationSendRequest struct {
	ConversationID string
	Content        string
	CorrelationID  string
}

// ConversationSendResult reports the recorded exchange: the Core-issued
// correlation ID (the identity the S4 query resolves through), the
// persisted user message, and the exchange's honest state. The response
// carries NO answer — no answering/assistant production path exists in this
// milestone (the named owner gap).
type ConversationSendResult struct {
	Exchange conversation.Exchange
	Message  conversation.Message
}

// Send persists one Ask send inside the conversation aggregate (P-S5-05):
// authorize → the request-shape law → ONE atomic transaction writing the
// user's message (its ordering key assigned inside the transaction) plus
// the exchange state bound to (conversation identity, correlation ID). The
// send NEVER creates a Job or any durable Run (X-1) — nothing in this path
// touches the Job machinery — and it NEVER auto-retries: a lost response
// resolves by ConversationExchangeStatus (the S4 query-by-correlation-ID
// read), never by a blind duplicate send.
func (s *Slice) Send(ctx context.Context, req ConversationSendRequest) (ConversationSendResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return ConversationSendResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: req.ConversationID}, security.ActionDataWrite); err != nil {
		return ConversationSendResult{}, err
	}
	if req.ConversationID == "" {
		return ConversationSendResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	if err := conversation.ValidateMessageContent(req.Content); err != nil {
		return ConversationSendResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content"})
	}
	// The correlation ID is the request identity (P1.6 §3.9 M-1): required,
	// bounded. It is not an idempotency key — no replay semantics ride it;
	// a duplicate identity on the same conversation is refused (the store's
	// ruling), and the recovery is ALWAYS the S4 query.
	if strings.TrimSpace(req.CorrelationID) == "" || len(req.CorrelationID) > 128 {
		return ConversationSendResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "correlation_id"})
	}
	// The exchange row is written FIRST-CLASS with its correlation ID in
	// the same transaction as the user's message: either both exist or
	// neither does, so the S4 query resolves the outcome honestly.
	x := conversation.Exchange{
		CorrelationID:  req.CorrelationID,
		ConversationID: req.ConversationID,
		State:          conversation.ExchangeRecorded,
	}
	recorded, msg, err := s.deps.Conversations.AppendConversationExchange(ctx, x, req.Content)
	if err != nil {
		return ConversationSendResult{}, err
	}
	return ConversationSendResult{Exchange: recorded, Message: msg}, nil
}

// ConversationExchangeStatus is the S4 query-before-retry read (P-S5-05):
// the authoritative exchange state for one send, resolved by the
// correlation ID the send returned. It is a pure QUERY — it creates
// nothing, mutates nothing, and never re-sends anything. A correlation ID
// bound to a DIFFERENT conversation is the SAME typed NOT_FOUND as an
// unknown id (the no-leak rule: an error never confirms an inaccessible
// object).
func (s *Slice) ConversationExchangeStatus(ctx context.Context, conversationID, correlationID string) (conversation.Exchange, conversation.Message, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Conversations == nil {
		return conversation.Exchange{}, conversation.Message{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: conversationID}, security.ActionDataRead); err != nil {
		return conversation.Exchange{}, conversation.Message{}, err
	}
	if conversationID == "" {
		return conversation.Exchange{}, conversation.Message{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	if correlationID == "" {
		return conversation.Exchange{}, conversation.Message{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "correlation_id"})
	}
	return s.deps.Conversations.LoadConversationExchange(ctx, conversationID, correlationID)
}

// conversationCursorPosition is the decoded form behind the opaque
// conversation-list cursor: the keyset position (the last page's sort-key
// values). It follows the coded event-cursor PATTERN — a salt-prefixed
// hex-JSON envelope, never structured data on the wire (events.EncodeCursor)
// — as a SEPARATE query-side mechanism (P1.7 §3.5): the event family's
// cursor is untouched (the F10 freeze stands). Carrying only the keyset, it
// is a pure position, not a retention-bound token.
type conversationCursorPosition struct {
	UpdatedAt      string `json:"updated_at"`
	ConversationID string `json:"conversation_id"`
}

func encodeConversationCursor(conv conversation.Conversation) (string, error) {
	if conv.ConversationID == "" || conv.UpdatedAt == "" {
		return "", fmt.Errorf("app: conversation cursor position lacks its keyset values")
	}
	b, err := json.Marshal(conversationCursorPosition{UpdatedAt: conv.UpdatedAt, ConversationID: conv.ConversationID})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

func decodeConversationCursor(cursor string) (conversationCursorPosition, error) {
	var pos conversationCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: conversation cursor is malformed")
	}
	if pos.ConversationID == "" || pos.UpdatedAt == "" {
		return pos, fmt.Errorf("app: conversation cursor is malformed")
	}
	return pos, nil
}

// conversationMessageCursorPosition is the messages-read keyset position.
type conversationMessageCursorPosition struct {
	Seq int64 `json:"seq"`
}

func encodeConversationMessageCursor(seq int64) string {
	b, err := json.Marshal(conversationMessageCursorPosition{Seq: seq})
	if err != nil {
		// A plain integer cannot fail to encode; an unencodable position
		// must never silently truncate the page, so the failure is loud.
		panic(fmt.Sprintf("app: message cursor position is unencodable: %v", err))
	}
	out, err := saltedCursorEnvelope(b)
	if err != nil {
		panic(fmt.Sprintf("app: message cursor envelope failed: %v", err))
	}
	return out
}

func decodeConversationMessageCursor(cursor string) (conversationMessageCursorPosition, error) {
	var pos conversationMessageCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: conversation message cursor is malformed")
	}
	if pos.Seq < 0 {
		return pos, fmt.Errorf("app: conversation message cursor is malformed")
	}
	return pos, nil
}

func saltedCursorEnvelope(payload []byte) (string, error) {
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt[:]) + "." + hex.EncodeToString(payload), nil
}

func decodeSaltedCursorEnvelope(cursor string) ([]byte, error) {
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("app: cursor is malformed")
	}
	b, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("app: cursor is malformed")
	}
	return b, nil
}
