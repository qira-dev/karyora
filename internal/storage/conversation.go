package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// randomMessageID mints the Core-issued per-message identity (the typed
// prefix plus random hex, the id-grammar class). A mint without a unique id
// must never be silently reused — entropy failure aborts, exactly like the
// approval-id mint.
func randomMessageID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("storage: entropy unavailable: " + err.Error())
	}
	return "msg-" + hex.EncodeToString(b[:])
}

// The durable conversation-aggregate store (the ADOPTED S5 mapping,
// P-S5-01…05, implemented as working candidates — migration 0009). The
// conversation is Core-owned durable state: the aggregate row, its
// authoritative messages (a stable per-message identity, the
// intra-conversation ordering key, the correlation-ID linkage), and the
// exchange rows bound to (conversation identity, correlation ID). The send
// path NEVER writes a Job row or touches any Job machinery (X-1); the
// answering/assistant production path does not exist (the named owner gap),
// so the role register carries `user` and the exchange-state register
// carries `recorded` only. No lifecycle, no revisions, no edit history
// (the UI-28 gate holds).

const conversationColumns = `conversation_id, title, created_at, updated_at`

func scanConversation(row rowScanner) (conversation.Conversation, error) {
	var c conversation.Conversation
	err := row.Scan(&c.ConversationID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func conversationRowByID(ctx context.Context, s *Store, id string) (conversation.Conversation, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+conversationColumns+` FROM conversations WHERE conversation_id = ?`, id)
	rec, err := scanConversation(row)
	if err == sql.ErrNoRows {
		return conversation.Conversation{}, errStoragef(errs.CodeNotFound, "conversation %q is not stored", id)
	}
	return rec, err
}

// CreateConversation stores one conversation aggregate row (the Core mints
// the identity; the store enforces the shape law fail closed).
func (s *Store) CreateConversation(ctx context.Context, rec conversation.Conversation) (conversation.Conversation, error) {
	if rec.ConversationID == "" {
		return conversation.Conversation{}, errStoragef(errs.CodeValidationFailed, "a conversation record requires its Core-issued id")
	}
	if err := conversation.ValidateTitle(rec.Title); err != nil {
		return conversation.Conversation{}, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if rec.CreatedAt == "" {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt == "" {
		rec.UpdatedAt = rec.CreatedAt
	}
	_, err := s.writers.ExecContext(ctx, `INSERT INTO conversations (conversation_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, rec.ConversationID, rec.Title, rec.CreatedAt, rec.UpdatedAt)
	if err != nil {
		return conversation.Conversation{}, mapVendorError(err)
	}
	return conversationRowByID(ctx, s, rec.ConversationID)
}

// LoadConversation reads one conversation aggregate's record; an unknown id
// is the typed NOT_FOUND.
func (s *Store) LoadConversation(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	return conversationRowByID(ctx, s, conversationID)
}

// AppendConversationExchange persists ONE send atomically (P-S5-05): the
// user's message (its intra-conversation Seq assigned inside the
// transaction — MAX(seq)+1 per conversation) plus the exchange row bound to
// (conversation identity, correlation ID), in ONE transaction. Either both
// rows exist durably or neither does, so the S4 query-by-correlation-ID
// read resolves a lost send response honestly. Nothing else is written —
// no Job, no Run, no event (X-1).
func (s *Store) AppendConversationExchange(ctx context.Context, x conversation.Exchange, content string) (conversation.Exchange, conversation.Message, error) {
	if x.ConversationID == "" || x.CorrelationID == "" {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeValidationFailed,
			"an exchange requires its conversation identity and correlation id")
	}
	if x.State != conversation.ExchangeRecorded {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeValidationFailed,
			"exchange state %q is not a registered exchange state", x.State)
	}
	if err := conversation.ValidateMessageContent(content); err != nil {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	if _, err := conversationRowByID(ctx, s, x.ConversationID); err != nil {
		return conversation.Exchange{}, conversation.Message{}, err
	}
	tx, err := s.writers.BeginTx(ctx, nil)
	if err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// The correlation identity's uniqueness ruling (P-S5-05; the
	// send-idempotency class stays Open — no replay semantics are claimed):
	// a re-presented correlation ID on the SAME conversation is the typed
	// CONFLICT_STATE — the exchange is already recorded, the S4
	// query-by-correlation-ID read resolves the outcome, and no duplicate
	// exchange is ever written; a correlation ID bound to a DIFFERENT
	// conversation is the SAME typed NOT_FOUND as unknown (the no-leak
	// rule — an error never confirms an inaccessible object).
	var bound string
	err = tx.QueryRowContext(ctx, `SELECT conversation_id FROM conversation_exchanges WHERE correlation_id = ?`, x.CorrelationID).Scan(&bound)
	switch {
	case err == nil:
		if bound == x.ConversationID {
			return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeConflictState,
				"correlation identity %q is already recorded on this conversation; the status query resolves its outcome", x.CorrelationID)
		}
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeNotFound, "exchange %q is not stored", x.CorrelationID)
	case err != sql.ErrNoRows:
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}

	// The ordering key: the next intra-conversation sequence, assigned
	// inside the transaction (the UNIQUE (conversation_id, seq) constraint
	// backstops the assignment).
	var seq sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM conversation_messages WHERE conversation_id = ?`, x.ConversationID).Scan(&seq); err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	nextSeq := seq.Int64 + 1
	if nextSeq < 1 {
		nextSeq = 1
	}
	messageID := randomMessageID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_messages
		(message_id, conversation_id, seq, role, content, correlation_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		messageID, x.ConversationID, nextSeq, string(conversation.MessageRoleUser), content, x.CorrelationID, now); err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_exchanges
		(correlation_id, conversation_id, user_message_id, state, created_at)
		VALUES (?, ?, ?, ?, ?)`, x.CorrelationID, x.ConversationID, messageID, string(x.State), now); err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE conversation_id = ?`, now, x.ConversationID); err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	if err := tx.Commit(); err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	exchange := conversation.Exchange{
		CorrelationID:  x.CorrelationID,
		ConversationID: x.ConversationID,
		UserMessageID:  messageID,
		State:          x.State,
		CreatedAt:      now,
	}
	msg := conversation.Message{
		MessageID:      messageID,
		ConversationID: x.ConversationID,
		Seq:            nextSeq,
		Role:           conversation.MessageRoleUser,
		Content:        content,
		CorrelationID:  x.CorrelationID,
		CreatedAt:      now,
	}
	return exchange, msg, nil
}

const conversationMessageColumns = `message_id, conversation_id, seq, role, content, correlation_id, created_at`

func scanMessage(row rowScanner) (conversation.Message, error) {
	var m conversation.Message
	var role string
	err := row.Scan(&m.MessageID, &m.ConversationID, &m.Seq, &role, &m.Content, &m.CorrelationID, &m.CreatedAt)
	if err != nil {
		return m, err
	}
	if !conversation.ValidMessageRole(conversation.MessageRole(role)) {
		return conversation.Message{}, errStoragef(errs.CodeStateCorrupt, "message %q carries an unregistered role", m.MessageID)
	}
	m.Role = conversation.MessageRole(role)
	return m, nil
}

// LoadMessagePage reads one bounded page of a conversation's authoritative
// messages in timeline order (Seq ascending — the per-aggregate sequence
// discipline), one probe row beyond the limit to detect a further page.
// Fail-closed guards: a non-positive limit and an empty conversation
// identity refuse.
func (s *Store) LoadMessagePage(ctx context.Context, q conversation.MessagePageQuery) (conversation.MessagePage, error) {
	if q.ConversationID == "" {
		return conversation.MessagePage{}, errStoragef(errs.CodeValidationFailed, "a message page requires its conversation identity")
	}
	if q.Limit < 1 {
		return conversation.MessagePage{}, errStoragef(errs.CodeValidationFailed, "message page limit must be at least 1")
	}
	where := ` WHERE conversation_id = ?`
	args := []any{q.ConversationID}
	if q.AfterSeq > 0 {
		where += ` AND seq > ?`
		args = append(args, q.AfterSeq)
	}
	args = append(args, q.Limit+1)
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT `+conversationMessageColumns+` FROM conversation_messages`+where+` ORDER BY seq ASC LIMIT ?`, args...)
	if err != nil {
		return conversation.MessagePage{}, mapVendorError(err)
	}
	defer rows.Close()
	page := conversation.MessagePage{Messages: []conversation.Message{}}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return conversation.MessagePage{}, err
		}
		page.Messages = append(page.Messages, m)
	}
	if err := rows.Err(); err != nil {
		return conversation.MessagePage{}, mapVendorError(err)
	}
	if len(page.Messages) > q.Limit {
		page.Messages = page.Messages[:q.Limit]
		page.More = true
	}
	return page, nil
}

// LoadConversationExchange resolves the S4 query-by-correlation-ID read:
// the exchange bound to (conversation identity, correlation ID) with its
// linked user message. An exchange bound to a DIFFERENT conversation is the
// SAME typed NOT_FOUND as an unknown correlation ID — an error never
// confirms an inaccessible object (the no-leak rule).
func (s *Store) LoadConversationExchange(ctx context.Context, conversationID, correlationID string) (conversation.Exchange, conversation.Message, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	var x conversation.Exchange
	row := s.readers.QueryRowContext(cctx, `SELECT correlation_id, conversation_id, user_message_id, state, created_at
		FROM conversation_exchanges WHERE correlation_id = ?`, correlationID)
	err := row.Scan(&x.CorrelationID, &x.ConversationID, &x.UserMessageID, &x.State, &x.CreatedAt)
	if err == sql.ErrNoRows {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeNotFound, "exchange %q is not stored", correlationID)
	}
	if err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	if x.ConversationID != conversationID {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeNotFound, "exchange %q is not stored", correlationID)
	}
	if !conversation.ValidExchangeState(x.State) {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeStateCorrupt, "exchange %q carries an unregistered state", x.CorrelationID)
	}
	mrow := s.readers.QueryRowContext(cctx, `SELECT `+conversationMessageColumns+` FROM conversation_messages WHERE message_id = ?`, x.UserMessageID)
	msg, err := scanMessage(mrow)
	if err == sql.ErrNoRows {
		return conversation.Exchange{}, conversation.Message{}, errStoragef(errs.CodeStateCorrupt, "exchange %q links a missing message", x.CorrelationID)
	}
	if err != nil {
		return conversation.Exchange{}, conversation.Message{}, mapVendorError(err)
	}
	return x, msg, nil
}

// ListConversations implements the server-side list/search over the
// Core-owned store (the adopted card-9 working candidate): the Core-defined
// sort (updated_at descending — the most recently active first;
// conversation_id ascending tiebreak), the keyset position riding
// AfterUpdatedAt/AfterConversationID, one probe row beyond the limit to
// report More. The optional Query narrows server-side — a title or message
// content match through the store's own machinery (EXISTS over the
// conversation's own messages; no dedicated index exists and none is
// invented). Fail-closed guards: a non-positive limit and a keyset position
// without its tiebreak refuse.
func (s *Store) ListConversations(ctx context.Context, q conversation.ListConversationsQuery) (conversation.ListConversationsPage, error) {
	if q.Limit < 1 {
		return conversation.ListConversationsPage{}, errStoragef(errs.CodeValidationFailed, "list limit must be at least 1")
	}
	where := ""
	args := []any{}
	if q.Query != "" {
		// The search term rides as a bounded LIKE pattern; the ESCAPE
		// clause keeps the term's own % and _ literal (a search term is
		// data, never a wildcard injection).
		pattern := "%" + escapeLike(q.Query) + "%"
		where = ` WHERE (title LIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM conversation_messages m
			WHERE m.conversation_id = conversations.conversation_id AND m.content LIKE ? ESCAPE '\'))`
		args = append(args, pattern, pattern)
	}
	if q.AfterUpdatedAt != "" || q.AfterConversationID != "" {
		if q.AfterUpdatedAt == "" || q.AfterConversationID == "" {
			return conversation.ListConversationsPage{}, errStoragef(errs.CodeValidationFailed, "a keyset position requires its conversation_id tiebreak")
		}
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += `(updated_at < ? OR (updated_at = ? AND conversation_id > ?))`
		args = append(args, q.AfterUpdatedAt, q.AfterUpdatedAt, q.AfterConversationID)
	}
	args = append(args, q.Limit+1)
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT `+conversationColumns+` FROM conversations`+where+` ORDER BY updated_at DESC, conversation_id ASC LIMIT ?`, args...)
	if err != nil {
		return conversation.ListConversationsPage{}, mapVendorError(err)
	}
	defer rows.Close()
	page := conversation.ListConversationsPage{Conversations: []conversation.Conversation{}}
	for rows.Next() {
		rec, err := scanConversation(rows)
		if err != nil {
			return conversation.ListConversationsPage{}, mapVendorError(err)
		}
		page.Conversations = append(page.Conversations, rec)
	}
	if err := rows.Err(); err != nil {
		return conversation.ListConversationsPage{}, mapVendorError(err)
	}
	if len(page.Conversations) > q.Limit {
		page.Conversations = page.Conversations[:q.Limit]
		page.More = true
	}
	return page, nil
}

// escapeLike escapes the LIKE wildcards inside a search term (the ESCAPE
// clause's partner; a search term is data).
func escapeLike(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(term)
}
