package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// The conversation surface (the ADOPTED S5 mapping, P-S5-01…05): the pins
// the dispatch owes — (1) a send NEVER creates a Job (X-1); (2) no
// auto-retry / the lost-response query path resolves without a duplicate
// send; (3) the no-leak refusal class; (4) the authorization join;
// (5) the pagination bounds; (6) migration 0009 applied and verified (the
// storage register pins it; the app fixtures open over the migrated store).

// convEnv extends the shared test env with the conversation surface wired
// (the real storage adapter over migration 0009 — the real composition).
func convEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	env.deps.Conversations = env.store
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	return env
}

func createConv(t *testing.T, env *testEnv, title string) conversation.Conversation {
	t.Helper()
	conv, err := env.slice.CreateConversation(env.ctx(), ConversationRecordRequest{Title: title})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if !strings.HasPrefix(conv.ConversationID, "conv-") {
		t.Fatalf("the Core issues the stable conversation identity, got %q", conv.ConversationID)
	}
	return conv
}

func sendReq(conversationID, content, corr string) ConversationSendRequest {
	return ConversationSendRequest{ConversationID: conversationID, Content: content, CorrelationID: corr}
}

// codeOf resolves the machine code of a slice refusal: the app layer's own
// typed envelopes and the storage adapter's typed errors (which pass
// through unchanged — the transport maps them) both carry one.
func codeOf(t *testing.T, err error) errs.Code {
	t.Helper()
	var ee *EnvelopeError
	if errors.As(err, &ee) {
		return ee.Envelope.Code
	}
	var se *storage.Error
	if errors.As(err, &se) {
		return se.Code
	}
	t.Fatalf("the refusal must be typed, got %v", err)
	return ""
}

// TestConversationSendNeverCreatesJob pins X-1 (P-S5-05): the Ask send
// persists the user's message and the exchange state inside the
// conversation aggregate and NEVER creates a Job or any durable Run — the
// Job register is byte-identical around the send (a control Job proves the
// probe sees real rows), no Job event exists for anything the send touched,
// and the exchange state is the conversation-plane `recorded`.
func TestConversationSendNeverCreatesJob(t *testing.T) {
	env := convEnv(t)
	ctx := env.ctx()

	// The control Job: the probe must see real rows for the pin to mean
	// anything.
	if _, err := env.slice.Submit(ctx, SubmitRequest{
		IdempotencyKey: "ctrl-1", Task: "control task", Provider: "mock", Model: "m",
	}); err != nil {
		t.Fatalf("control submit: %v", err)
	}
	jobsBefore, err := env.deps.Relay.ListJobs(context.Background(), aijobrelay.ListJobsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsBefore.Jobs) != 1 {
		t.Fatalf("precondition: one control Job expected, got %d", len(jobsBefore.Jobs))
	}

	conv := createConv(t, env, "the X-1 pin")
	res, err := env.slice.Send(ctx, sendReq(conv.ConversationID, "an ask request — never a durable job", "corr-x1"))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.Exchange.State != conversation.ExchangeRecorded {
		t.Fatalf("exchange state = %q, want the conversation-plane %q", res.Exchange.State, conversation.ExchangeRecorded)
	}
	if res.Exchange.CorrelationID != "corr-x1" || res.Exchange.ConversationID != conv.ConversationID {
		t.Fatalf("the exchange binds (conversation identity, correlation ID): %+v", res.Exchange)
	}
	if res.Message.Role != conversation.MessageRoleUser {
		t.Fatalf("the send persists the user's message, got role %q", res.Message.Role)
	}

	jobsAfter, err := env.deps.Relay.ListJobs(context.Background(), aijobrelay.ListJobsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsAfter.Jobs) != 1 {
		t.Fatalf("X-1: the send created %d Job rows, want none (the control Job only)", len(jobsAfter.Jobs)-1)
	}
	if jobsAfter.Jobs[0].JobID != jobsBefore.Jobs[0].JobID {
		t.Fatalf("X-1: the Job register changed around the send")
	}
	// The send's durable state lives ONLY in the conversation aggregate:
	// the exchange and its message are readable back through the reads.
	x, msg, err := env.slice.ConversationExchangeStatus(ctx, conv.ConversationID, "corr-x1")
	if err != nil {
		t.Fatalf("the S4 query must resolve the send: %v", err)
	}
	if x.UserMessageID != msg.MessageID || msg.Content != "an ask request — never a durable job" {
		t.Fatalf("the exchange's linkage must resolve its message")
	}
}

// failingAppendStore wraps the real store: it fails the FIRST
// AppendConversationExchange call with a typed error and counts every call,
// so the no-auto-retry pin can prove the send operation itself never
// retries.
type failingAppendStore struct {
	inner ConversationStore
	calls int
}

func (f *failingAppendStore) CreateConversation(ctx context.Context, rec conversation.Conversation) (conversation.Conversation, error) {
	return f.inner.CreateConversation(ctx, rec)
}
func (f *failingAppendStore) LoadConversation(ctx context.Context, id string) (conversation.Conversation, error) {
	return f.inner.LoadConversation(ctx, id)
}
func (f *failingAppendStore) AppendConversationExchange(ctx context.Context, x conversation.Exchange, content string) (conversation.Exchange, conversation.Message, error) {
	f.calls++
	if f.calls == 1 {
		return conversation.Exchange{}, conversation.Message{}, &EnvelopeError{Envelope: errs.Envelope{
			Code: errs.CodeStorageUnavailable, Retryable: true, CorrelationID: "scripted",
		}}
	}
	return f.inner.AppendConversationExchange(ctx, x, content)
}
func (f *failingAppendStore) ListConversations(ctx context.Context, q conversation.ListConversationsQuery) (conversation.ListConversationsPage, error) {
	return f.inner.ListConversations(ctx, q)
}
func (f *failingAppendStore) LoadMessagePage(ctx context.Context, q conversation.MessagePageQuery) (conversation.MessagePage, error) {
	return f.inner.LoadMessagePage(ctx, q)
}
func (f *failingAppendStore) LoadConversationExchange(ctx context.Context, conversationID, correlationID string) (conversation.Exchange, conversation.Message, error) {
	return f.inner.LoadConversationExchange(ctx, conversationID, correlationID)
}

// TestConversationSendNoAutoRetry pins the no-auto-retry law and the
// duplicate-identity ruling (P-S5-05; P1.6 §4 rule 2 — conversation-send
// idempotency is Open, so NO auto-retry): a failed send makes EXACTLY ONE
// store call (the operation never retries on its own); the caller's
// EXPLICIT re-send with the same correlation ID after a never-committed
// attempt succeeds (the first attempt left nothing behind); and a re-send
// with an ALREADY-RECORDED correlation ID is refused (CONFLICT_STATE) —
// never a duplicate exchange — while the S4 query resolves the recorded
// outcome.
func TestConversationSendNoAutoRetry(t *testing.T) {
	env := convEnv(t)
	failing := &failingAppendStore{inner: env.store}
	env.deps.Conversations = failing
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice

	conv := createConv(t, env, "no auto-retry")
	_, err = env.slice.Send(env.ctx(), sendReq(conv.ConversationID, "the lost attempt", "corr-retry"))
	var ee *EnvelopeError
	if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeStorageUnavailable {
		t.Fatalf("the scripted outage must surface typed: %v", err)
	}
	if failing.calls != 1 {
		t.Fatalf("NO auto-retry: the send made %d store calls, want exactly 1", failing.calls)
	}
	// The failed attempt committed NOTHING (the S4 query resolves unknown):
	if _, _, err := env.slice.ConversationExchangeStatus(env.ctx(), conv.ConversationID, "corr-retry"); err == nil {
		t.Fatal("a failed (never-committed) send must leave no exchange to query")
	}
	// The caller's EXPLICIT re-send with the same identity succeeds — the
	// first attempt left nothing behind (never a partial write).
	res, err := env.slice.Send(env.ctx(), sendReq(conv.ConversationID, "the explicit re-send", "corr-retry"))
	if err != nil {
		t.Fatalf("the explicit re-send after a never-committed attempt: %v", err)
	}
	if res.Message.Content != "the explicit re-send" {
		t.Fatalf("the re-send recorded the caller's content: %+v", res.Message)
	}
	// A re-presented correlation ID on an ALREADY-RECORDED exchange is
	// refused — never a duplicate exchange, never a silent replay claim.
	env.deps.Conversations = env.store
	slice, err = NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	before, err := env.slice.ConversationMessages(env.ctx(), conv.ConversationID, ConversationMessagePageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.slice.Send(env.ctx(), sendReq(conv.ConversationID, "a blind duplicate", "corr-retry"))
	if codeOf(t, err) != errs.CodeConflictState {
		t.Fatalf("a duplicate correlation identity must refuse CONFLICT_STATE, got %v", err)
	}
	after, err := env.slice.ConversationMessages(env.ctx(), conv.ConversationID, ConversationMessagePageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Messages) != len(before.Messages) {
		t.Fatalf("the refused duplicate must write nothing: %d → %d messages", len(before.Messages), len(after.Messages))
	}
	// The recovery is the QUERY: the S4 read resolves the recorded outcome
	// truthfully — the first exchange, not the refused duplicate.
	x, msg, err := env.slice.ConversationExchangeStatus(env.ctx(), conv.ConversationID, "corr-retry")
	if err != nil {
		t.Fatalf("the S4 query must resolve the outcome: %v", err)
	}
	if msg.Content != "the explicit re-send" {
		t.Fatalf("the query resolves the recorded exchange's own message: %+v", msg)
	}
	if x.State != conversation.ExchangeRecorded {
		t.Fatalf("the recorded state is %q", x.State)
	}
}

// TestConversationLostResponseResolvesByQuery pins the S4 state's
// resolution path (P-S5-05): a send whose response is lost (the caller
// keeps only the request identity it presented) resolves by the
// query-by-correlation-ID read — a pure QUERY that creates nothing and
// mutates nothing, and the only recovery the law allows.
func TestConversationLostResponseResolvesByQuery(t *testing.T) {
	env := convEnv(t)
	conv := createConv(t, env, "lost response")
	corr := "corr-lost-1"
	if _, err := env.slice.Send(env.ctx(), sendReq(conv.ConversationID, "the question whose response was lost", corr)); err != nil {
		t.Fatalf("send: %v", err)
	}
	timeline, err := env.slice.ConversationMessages(env.ctx(), conv.ConversationID, ConversationMessagePageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	// The lost-response recovery: query by the correlation ID — never a
	// blind duplicate send (a second send would be a NEW exchange with a
	// NEW identity; the query is what resolves the original).
	x, msg, err := env.slice.ConversationExchangeStatus(env.ctx(), conv.ConversationID, corr)
	if err != nil {
		t.Fatalf("the S4 query must resolve the lost response: %v", err)
	}
	if x.CorrelationID != corr || x.State != conversation.ExchangeRecorded {
		t.Fatalf("the query resolves the authoritative exchange state: %+v", x)
	}
	if msg.CorrelationID != corr {
		t.Fatalf("the produced message carries the send's correlation ID: %+v", msg)
	}
	// The query is a pure read: the timeline is unchanged, nothing minted.
	timeline2, err := env.slice.ConversationMessages(env.ctx(), conv.ConversationID, ConversationMessagePageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline2.Messages) != len(timeline.Messages) {
		t.Fatalf("the S4 query must not mutate the aggregate")
	}
}

// TestConversationExchangeStatusNoLeak pins the no-leak refusal class
// (P-S5-04/P-S5-05): a correlation ID bound to a DIFFERENT conversation and
// an unknown correlation ID are the SAME typed NOT_FOUND — an error never
// confirms an inaccessible object — and an unknown conversation read is the
// typed NOT_FOUND, never a shape that distinguishes existence.
func TestConversationExchangeStatusNoLeak(t *testing.T) {
	env := convEnv(t)
	convA := createConv(t, env, "conversation A")
	convB := createConv(t, env, "conversation B")
	if _, err := env.slice.Send(env.ctx(), sendReq(convA.ConversationID, "A's question", "corr-a")); err != nil {
		t.Fatalf("send: %v", err)
	}
	_, _, errForeign := env.slice.ConversationExchangeStatus(env.ctx(), convB.ConversationID, "corr-a")
	_, _, errUnknown := env.slice.ConversationExchangeStatus(env.ctx(), convB.ConversationID, "no-such-corr")
	if codeOf(t, errForeign) != errs.CodeNotFound || codeOf(t, errUnknown) != errs.CodeNotFound {
		t.Fatalf("the no-leak class: both are NOT_FOUND, got %v / %v", errForeign, errUnknown)
	}
	if _, err := env.slice.Conversation(env.ctx(), "conv-nope"); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("an unknown conversation is the typed NOT_FOUND: %v", err)
	}
}

// TestConversationAuthorizationJoin pins the authorization join (the
// additive register extension): every conversation operation denies the
// unauthenticated caller through the ONE decision port (typed
// AUTHZ_DENIED, nothing changed), a data-scoped session proceeds, the
// conversation resource row's decisions carry the audit scope
// `conversation:<id>`, and the fail-closed default-deny machinery is
// UNCHANGED — an unknown resource type still denies with
// RESOURCE_OUT_OF_SCOPE exactly as before the join.
func TestConversationAuthorizationJoin(t *testing.T) {
	env := convEnv(t)
	denied := WithCaller(context.Background(), "local", "kry1-not-a-real-token")

	conv := createConv(t, env, "authorization join")
	for name, op := range map[string]func() error{
		"create": func() error {
			_, err := env.slice.CreateConversation(denied, ConversationRecordRequest{Title: "x"})
			return err
		},
		"read": func() error { _, err := env.slice.Conversation(denied, conv.ConversationID); return err },
		"send": func() error { _, err := env.slice.Send(denied, sendReq(conv.ConversationID, "x", "c")); return err },
		"messages": func() error {
			_, err := env.slice.ConversationMessages(denied, conv.ConversationID, ConversationMessagePageRequest{})
			return err
		},
		"list": func() error { _, err := env.slice.ListConversations(denied, ListConversationsRequest{}); return err },
		"extraction": func() error {
			_, _, err := env.slice.ConversationExchangeStatus(denied, conv.ConversationID, "c")
			return err
		},
	} {
		err := op()
		var ee *EnvelopeError
		if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeAuthzDenied {
			t.Fatalf("%s: the unauthenticated caller must be the typed AUTHZ_DENIED envelope, got %v", name, err)
		}
	}

	// The audit scope rides the conversation resource row (the join's
	// observable shape).
	sink := &memoryAuditStore{}
	env.deps.Guard.Audit = security.NewAuditService(sink, 64, 1000)
	if _, err := env.slice.Send(env.ctx(), sendReq(conv.ConversationID, "audited question", "corr-audit")); err != nil {
		t.Fatalf("send: %v", err)
	}
	records, err := sink.Records()
	if err != nil {
		t.Fatalf("audit records: %v", err)
	}
	found := false
	for _, rec := range records {
		var ar security.AuditRecord
		if err := json.Unmarshal(rec.Payload, &ar); err != nil {
			t.Fatalf("audit payload: %v", err)
		}
		if ar.Action != string(security.ActionDataWrite) {
			continue
		}
		found = true
		if ar.Scope != "conversation:"+conv.ConversationID {
			t.Fatalf("the audit scope must name the conversation resource row, got %q", ar.Scope)
		}
		if ar.PolicyDecision != "allow" || ar.PolicyReference != security.ReasonOK {
			t.Fatalf("the audit record must carry the authorization reference: %+v", ar)
		}
	}
	if !found {
		t.Fatalf("the send must be an audited decision among %d records", len(records))
	}

	// The default-deny machinery is unchanged: an unknown resource type
	// denies exactly as before the join (the predicates and every existing
	// row untouched).
	decision, auditErr := env.deps.Guard.Authorize(
		security.Principal{Kind: security.PrincipalLocalUser, ID: "local"},
		security.Resource{Type: "remote-cluster"}, security.ActionStatusRead,
		security.PolicyContext{CorrelationID: "corr-default-deny", Credential: security.CredentialIPCCapability},
	)
	if auditErr != nil {
		t.Fatalf("audit: %v", auditErr)
	}
	if decision.Allow || decision.ReasonCode != security.ReasonResourceOutOfScope {
		t.Fatalf("the fail-closed default is unchanged, got %+v", decision)
	}
}

// TestConversationListPaginationAndOrder pins the card-9 pagination at the
// API surface (P-S5-04): the per-surface default applies; the 1–1000 bound
// refuses out-of-range limits typed; the opaque cursor round-trips over a
// size-2 walk with no overlap or gap; the cursorless page IS the final
// page (no has_more/total vocabulary exists); the cursor never carries
// structured JSON on the wire.
func TestConversationListPaginationAndOrder(t *testing.T) {
	env := convEnv(t)
	for i := 0; i < 5; i++ {
		if _, err := env.slice.CreateConversation(env.ctx(), ConversationRecordRequest{
			Title: "paginated conversation",
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	// The default limit applies (0 → the per-surface default): a page
	// carrying the continuation cursor must be exactly one default page,
	// and a cursorless page never exceeds it.
	first, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor != "" && len(first.Conversations) != DefaultConversationListLimit {
		t.Fatalf("a continued page must carry exactly the per-surface default: %d rows", len(first.Conversations))
	}
	if first.NextCursor == "" && len(first.Conversations) > DefaultConversationListLimit {
		t.Fatalf("a final page never exceeds the per-surface default: %d rows", len(first.Conversations))
	}
	// A size-2 keyset walk over five conversations: no overlap, no gap,
	// the cursorless final page.
	seen := map[string]bool{}
	var walked int
	cursor := ""
	for page := 0; ; page++ {
		res, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Conversations) > 2 {
			t.Fatalf("page over the bound")
		}
		for _, c := range res.Conversations {
			if seen[c.ConversationID] {
				t.Fatalf("overlap in the keyset walk: %s seen twice", c.ConversationID)
			}
			seen[c.ConversationID] = true
			walked++
		}
		if res.NextCursor == "" {
			break
		}
		if strings.ContainsAny(res.NextCursor, "{}") {
			t.Fatalf("the cursor never carries structured JSON: %q", res.NextCursor)
		}
		cursor = res.NextCursor
		if page > 10 {
			t.Fatal("the walk did not terminate")
		}
	}
	if walked != 5 {
		t.Fatalf("the walk saw %d conversations, want 5", walked)
	}
	// The bound class: 1000 accepted; -1 and 1001 refused typed (0 is the
	// per-surface default selector, not a bound case).
	if _, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Limit: MaxConversationListLimit}); err != nil {
		t.Fatalf("the 1000 bound is accepted: %v", err)
	}
	for _, bad := range []int{-1, 1001} {
		_, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Limit: bad})
		var ee *EnvelopeError
		if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("limit %d must refuse typed VALIDATION_FAILED, got %v", bad, err)
		}
	}
	// A malformed cursor refuses typed.
	if _, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Limit: 2, Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("a malformed cursor must refuse")
	}
}

// TestConversationListSearch pins the server-side search at the API surface
// (P-S5-04): the query narrows server-side over the Core-owned store; a
// successful no-match search is an EMPTY page — never a fake error shape;
// the results ride the same bounded page protocol.
func TestConversationListSearch(t *testing.T) {
	env := convEnv(t)
	needle := createConv(t, env, "the needle conversation title")
	other := createConv(t, env, "an unrelated conversation")
	if _, err := env.slice.Send(env.ctx(), sendReq(other.ConversationID, "plain message body", "c1")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := env.slice.Send(env.ctx(), sendReq(needle.ConversationID, "the search needle lives in this message", "c2")); err != nil {
		t.Fatalf("send: %v", err)
	}
	byTitle, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Query: "needle conversation title"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byTitle.Conversations) != 1 || byTitle.Conversations[0].ConversationID != needle.ConversationID {
		t.Fatalf("the title search must narrow server-side: %+v", byTitle.Conversations)
	}
	byContent, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Query: "search needle lives"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byContent.Conversations) != 1 || byContent.Conversations[0].ConversationID != needle.ConversationID {
		t.Fatalf("the content search must narrow server-side: %+v", byContent.Conversations)
	}
	noMatch, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{Query: "nothing-matches-this"})
	if err != nil {
		t.Fatalf("a no-match search is an empty page, never an error: %v", err)
	}
	if len(noMatch.Conversations) != 0 || noMatch.NextCursor != "" {
		t.Fatalf("a no-match search must be the empty final page: %+v", noMatch)
	}
}

// TestConversationValidation pins the request-shape law (typed refusals,
// never silent coercion): the empty/missing correlation id, the empty
// content, the over-bound content, the over-bound title, and a send on an
// unknown conversation (the typed NOT_FOUND changing nothing).
func TestConversationValidation(t *testing.T) {
	env := convEnv(t)
	conv := createConv(t, env, "validation")
	cases := []struct {
		name string
		req  ConversationSendRequest
		code errs.Code
	}{
		{"missing correlation id", sendReq(conv.ConversationID, "content", ""), errs.CodeValidationFailed},
		{"over-bound correlation id", sendReq(conv.ConversationID, "content", strings.Repeat("c", 129)), errs.CodeValidationFailed},
		{"empty content", sendReq(conv.ConversationID, "", "corr-v"), errs.CodeValidationFailed},
		{"over-bound content", sendReq(conv.ConversationID, strings.Repeat("x", conversation.MaxMessageRunes+1), "corr-v"), errs.CodeValidationFailed},
		{"unknown conversation", sendReq("conv-nope", "content", "corr-v"), errs.CodeNotFound},
	}
	for _, tc := range cases {
		_, err := env.slice.Send(env.ctx(), tc.req)
		if codeOf(t, err) != tc.code {
			t.Fatalf("%s: want %s, got %v", tc.name, tc.code, err)
		}
	}
	if _, err := env.slice.CreateConversation(env.ctx(), ConversationRecordRequest{
		Title: strings.Repeat("t", conversation.MaxTitleRunes+1),
	}); err == nil {
		t.Fatal("an over-bound title must refuse")
	}
	// A send on an unknown conversation recorded nothing anywhere.
	page, err := env.slice.ListConversations(env.ctx(), ListConversationsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range page.Conversations {
		if c.ConversationID == "conv-nope" {
			t.Fatal("an unknown conversation must not appear")
		}
	}
}
