package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// convTestRecord builds one conversation record with a fresh Core-issued id
// (the app layer mints the identity; the store enforces the shape law).
func convTestRecord(t *testing.T, s *Store, title string) conversation.Conversation {
	t.Helper()
	rec, err := s.CreateConversation(context.Background(), conversation.Conversation{
		ConversationID: "conv-" + randomHexForTest(t),
		Title:          title,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return rec
}

// randomHexForTest mints fresh test-unique hex (the id-grammar's random
// half; the shape law and uniqueness come from the store either way).
func randomHexForTest(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("entropy: %v", err)
	}
	return hex.EncodeToString(b[:])
}

func sendTestExchange(t *testing.T, s *Store, conversationID, content, corr string) (conversation.Exchange, conversation.Message) {
	t.Helper()
	x, msg, err := s.AppendConversationExchange(context.Background(), conversation.Exchange{
		CorrelationID:  corr,
		ConversationID: conversationID,
		State:          conversation.ExchangeRecorded,
	}, content)
	if err != nil {
		t.Fatalf("append exchange: %v", err)
	}
	return x, msg
}

// TestConversationMigrationApplied pins migration 0009: a fresh apply
// reaches schema version 9 with the conversation tables present, and the
// prior migrations' recorded checksums still verify (0007/0008 byte-identical
// — the checksum authority refuses a modified SQL on every open).
func TestConversationMigrationApplied(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersionMax {
		t.Fatalf("schema version = %d, want %d (migration 0009 applied)", v, schemaVersionMax)
	}
	for _, version := range []int64{7, 8, 9} {
		state, err := s.migrationState(ctx, version)
		if err != nil {
			t.Fatal(err)
		}
		if state != migStateApplied {
			t.Fatalf("migration %d state = %s, want applied", version, state)
		}
	}
	for _, table := range []string{"conversations", "conversation_messages", "conversation_exchanges"} {
		var n int
		if err := s.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("migration 0009 table %q missing", table)
		}
	}
}

// TestConversationAppendExchangeOrdering pins the aggregate's authoritative
// messages (P-S5-02): each send persists the user's message with the next
// intra-conversation Seq and the correlation-ID linkage; the timeline read
// returns them in Seq order; the role register carries `user`.
func TestConversationAppendExchangeOrdering(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "ordering")
	sendTestExchange(t, s, conv.ConversationID, "first question", "corr-1")
	sendTestExchange(t, s, conv.ConversationID, "second question", "corr-2")
	page, err := s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: conv.ConversationID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(page.Messages))
	}
	if page.Messages[0].Seq != 1 || page.Messages[1].Seq != 2 {
		t.Fatalf("seq order = %d,%d, want 1,2", page.Messages[0].Seq, page.Messages[1].Seq)
	}
	if page.Messages[0].Role != conversation.MessageRoleUser || page.Messages[1].Role != conversation.MessageRoleUser {
		t.Fatalf("role register: only the user role exists (no answering producer — the named owner gap)")
	}
	if page.Messages[0].CorrelationID != "corr-1" || page.Messages[1].CorrelationID != "corr-2" {
		t.Fatalf("correlation linkage broken: %q %q", page.Messages[0].CorrelationID, page.Messages[1].CorrelationID)
	}
	// The conversation's updated_at advanced (the list's sort key follows
	// the aggregate's activity).
	after, err := s.LoadConversation(ctx, conv.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if after.UpdatedAt < conv.UpdatedAt {
		t.Fatalf("updated_at did not advance on append: %s < %s", after.UpdatedAt, conv.UpdatedAt)
	}
}

// TestConversationExchangeAtomicity pins the send's atomicity (P-S5-05):
// the user's message and the exchange row commit together — the exchange
// row exists only with its message (the S4 query resolves honestly), and a
// refused send leaves NEITHER row.
func TestConversationExchangeAtomicity(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "atomicity")
	x, msg, err := s.AppendConversationExchange(ctx, conversation.Exchange{
		CorrelationID:  "corr-a",
		ConversationID: conv.ConversationID,
		State:          conversation.ExchangeRecorded,
	}, "recorded question")
	if err != nil {
		t.Fatal(err)
	}
	if x.UserMessageID != msg.MessageID || x.State != conversation.ExchangeRecorded {
		t.Fatalf("exchange linkage/state wrong: %+v %+v", x, msg)
	}
	// The correlation-ID query resolves the same pair.
	x2, msg2, err := s.LoadConversationExchange(ctx, conv.ConversationID, "corr-a")
	if err != nil {
		t.Fatal(err)
	}
	if x2.UserMessageID != msg2.MessageID || msg2.Content != "recorded question" {
		t.Fatalf("the S4 query must resolve the exchange and its message")
	}
	// A refused send (an unregistered state) writes neither row.
	_, _, err = s.AppendConversationExchange(ctx, conversation.Exchange{
		CorrelationID:  "corr-b",
		ConversationID: conv.ConversationID,
		State:          "answered", // not a registered state — no producer exists
	}, "refused")
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeValidationFailed {
		t.Fatalf("unregistered exchange state err = %v, want VALIDATION_FAILED", err)
	}
	if _, _, err := s.LoadConversationExchange(ctx, conv.ConversationID, "corr-b"); !errors.As(err, &se) || se.Code != errs.CodeNotFound {
		t.Fatalf("a refused send must leave no exchange row: %v", err)
	}
	var n int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM conversation_messages WHERE correlation_id = 'corr-b'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused send must leave no message row (atomicity)")
	}
}

// TestConversationExchangeNoLeak pins the no-leak refusal class at the
// store (P-S5-04/P-S5-05): a correlation ID bound to a DIFFERENT
// conversation is the SAME typed NOT_FOUND as an unknown correlation ID —
// indistinguishable, changing nothing.
func TestConversationExchangeNoLeak(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	convA := convTestRecord(t, s, "conversation A")
	convB := convTestRecord(t, s, "conversation B")
	sendTestExchange(t, s, convA.ConversationID, "A's question", "corr-a")
	_, _, errA := s.LoadConversationExchange(ctx, convB.ConversationID, "corr-a") // foreign binding
	_, _, errUnknown := s.LoadConversationExchange(ctx, convB.ConversationID, "no-such-correlation")
	var seA, seU *Error
	if !errors.As(errA, &seA) || !errors.As(errUnknown, &seU) {
		t.Fatalf("both refusals must be typed storage errors: %v / %v", errA, errUnknown)
	}
	if seA.Code != errs.CodeNotFound || seU.Code != errs.CodeNotFound {
		t.Fatalf("the no-leak class: both are NOT_FOUND, got %s / %s", seA.Code, seU.Code)
	}
	// The refusal carries only the caller's own presented identity — never
	// a fact about the foreign binding (the detail shapes match the coded
	// NOT_FOUND convention; neither reveals that the exchange exists
	// elsewhere).
}

// TestConversationListKeysetPagination pins the card-9 keyset walk over the
// conversation list: the Core-defined order (updated_at DESC,
// conversation_id ASC), no overlap or gap over a size-2 walk, the limit+1
// More probe, the search narrowing, and the fail-closed guards.
func TestConversationListKeysetPagination(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Five conversations with strictly descending updated_at (distinct
	// instants so the keyset walk is deterministic).
	var ids []string
	for i := 0; i < 5; i++ {
		rec, err := s.CreateConversation(ctx, conversation.Conversation{
			ConversationID: "conv-keyset-" + string(rune('a'+i)),
			Title:          "keyset conversation",
			CreatedAt:      "2026-01-0" + string(rune('1'+i)) + "T00:00:00Z",
			UpdatedAt:      "2026-01-0" + string(rune('5'-i)) + "T00:00:00Z",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ConversationID)
	}
	q := conversation.ListConversationsQuery{Limit: 2}
	var seen []string
	for {
		page, err := s.ListConversations(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Conversations) > 2 {
			t.Fatalf("page over the bound: %d rows", len(page.Conversations))
		}
		for _, c := range page.Conversations {
			seen = append(seen, c.ConversationID)
		}
		if !page.More {
			break
		}
		if len(page.Conversations) == 0 {
			t.Fatal("More with an empty page")
		}
		last := page.Conversations[len(page.Conversations)-1]
		q.AfterUpdatedAt, q.AfterConversationID = last.UpdatedAt, last.ConversationID
	}
	if len(seen) != 5 {
		t.Fatalf("walk saw %d conversations, want 5 (no gap, no repeat)", len(seen))
	}
	// The walk is strictly descending in updated_at (the Core-defined sort).
	var prev string
	for _, id := range seen {
		var updated string
		if err := s.readers.QueryRow(`SELECT updated_at FROM conversations WHERE conversation_id = ?`, id).Scan(&updated); err != nil {
			t.Fatal(err)
		}
		if prev != "" && updated > prev {
			t.Fatalf("order violation: %s after %s", updated, prev)
		}
		prev = updated
	}
	// The search narrows server-side (title and message content), and a
	// no-match search is an EMPTY PAGE — never an error.
	page, err := s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 10, Query: "keyset"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Conversations) != 5 || page.More {
		t.Fatalf("title search: %d rows, more=%v, want 5 false", len(page.Conversations), page.More)
	}
	sendTestExchange(t, s, ids[2], "the needle content lives here", "corr-needle")
	page, err = s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 10, Query: "needle content"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Conversations) != 1 || page.Conversations[0].ConversationID != ids[2] {
		t.Fatalf("content search: %+v, want exactly %s", page.Conversations, ids[2])
	}
	page, err = s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 10, Query: "no-such-anything"})
	if err != nil {
		t.Fatalf("a no-match search is an empty page, not an error: %v", err)
	}
	if len(page.Conversations) != 0 || page.More {
		t.Fatalf("no-match search must be empty: %+v", page.Conversations)
	}
	// Fail-closed guards.
	if _, err := s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 0}); err == nil {
		t.Fatal("a non-positive limit must refuse")
	}
	if _, err := s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 2, AfterUpdatedAt: "x"}); err == nil {
		t.Fatal("a tiebreak-less keyset position must refuse")
	}
}

// TestConversationMessagePageGuards pins the messages read's fail-closed
// guards and its keyset walk.
func TestConversationMessagePageGuards(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "pages")
	for i := 0; i < 3; i++ {
		sendTestExchange(t, s, conv.ConversationID, "message body", "corr-p"+string(rune('a'+i)))
	}
	page, err := s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: conv.ConversationID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || !page.More || page.Messages[0].Seq != 1 {
		t.Fatalf("first page wrong: %+v more=%v", page.Messages, page.More)
	}
	page, err = s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: conv.ConversationID, Limit: 2, AfterSeq: page.Messages[1].Seq})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.More || page.Messages[0].Seq != 3 {
		t.Fatalf("second page wrong: %+v more=%v", page.Messages, page.More)
	}
	if _, err := s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: conv.ConversationID, Limit: 0}); err == nil {
		t.Fatal("a non-positive limit must refuse")
	}
	if _, err := s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: "", Limit: 2}); err == nil {
		t.Fatal("an empty conversation identity must refuse")
	}
	// An unknown conversation's page is empty — never an invented row.
	page, err = s.LoadMessagePage(ctx, conversation.MessagePageQuery{ConversationID: "conv-nope", Limit: 5})
	if err != nil {
		t.Fatalf("an unknown conversation is an empty page, not an error: %v", err)
	}
	if len(page.Messages) != 0 {
		t.Fatalf("an unknown conversation must return no messages: %+v", page.Messages)
	}
}

// TestConversationStoreShapeLaw pins the store's fail-closed shape rules:
// an empty identity refuses; an over-long title refuses; an over-long or
// empty message content refuses; the SQL LIKE wildcards inside a search
// term stay literal data.
func TestConversationStoreShapeLaw(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateConversation(ctx, conversation.Conversation{ConversationID: ""}); err == nil {
		t.Fatal("an empty identity must refuse")
	}
	if _, err := s.CreateConversation(ctx, conversation.Conversation{
		ConversationID: "conv-long",
		Title:          strings.Repeat("t", conversation.MaxTitleRunes+1),
	}); err == nil {
		t.Fatal("an over-long title must refuse")
	}
	conv := convTestRecord(t, s, "shapes")
	if _, _, err := s.AppendConversationExchange(ctx, conversation.Exchange{
		CorrelationID:  "corr-empty",
		ConversationID: conv.ConversationID,
		State:          conversation.ExchangeRecorded,
	}, ""); err == nil {
		t.Fatal("an empty message content must refuse")
	}
	if _, _, err := s.AppendConversationExchange(ctx, conversation.Exchange{
		CorrelationID:  "corr-long",
		ConversationID: conv.ConversationID,
		State:          conversation.ExchangeRecorded,
	}, strings.Repeat("x", conversation.MaxMessageRunes+1)); err == nil {
		t.Fatal("an over-long message content must refuse")
	}
	// A send on an unknown conversation refuses (NOT_FOUND) and records
	// nothing.
	_, _, err := s.AppendConversationExchange(ctx, conversation.Exchange{
		CorrelationID:  "corr-unknown",
		ConversationID: "conv-nope",
		State:          conversation.ExchangeRecorded,
	}, "content")
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeNotFound {
		t.Fatalf("a send on an unknown conversation must refuse NOT_FOUND: %v", err)
	}
	// The LIKE wildcards stay literal: a search for a literal percent sign
	// matches nothing (never everything).
	sendTestExchange(t, s, conv.ConversationID, "plain content", "corr-like")
	page, err := s.ListConversations(ctx, conversation.ListConversationsQuery{Limit: 10, Query: "%"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Conversations) != 0 {
		t.Fatalf("a bare wildcard term must match nothing, got %d rows", len(page.Conversations))
	}
}
