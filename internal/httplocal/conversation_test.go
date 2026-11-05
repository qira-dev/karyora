package httplocal

import (
	"encoding/json"
	"strings"
	"testing"
)

// The conversation surface's wire register (the adopted S5 mapping,
// P-S5-01…05): the endpoints end to end over the live loopback test server,
// the typed refusals, and the route register pin that proves NO streaming,
// stop, or steering endpoint exists (the stream-transport mechanism is the
// named owner gap, P-S5-03).

// TestConversationRouteRegister pins the exact conversation route set:
// the six conversation endpoints (IMPL-10) plus the five attachment
// endpoints (IMPL-25; the ADOPTED S12 supply as implemented), and NO
// streaming endpoint, no stop route, no steering route — nothing of the
// stream plane was invented (the route table is the pin's authority).
func TestConversationRouteRegister(t *testing.T) {
	var got []string
	for _, rt := range routes() {
		if strings.HasPrefix(rt.prefix, "/v1/conversations") {
			got = append(got, rt.method+" "+rt.prefix+rt.suffix)
		}
	}
	want := []string{
		"POST /v1/conversations",
		"GET /v1/conversations",
		"GET /v1/conversations//messages",
		"GET /v1/conversations//exchange",
		"POST /v1/conversations//send",
		"GET /v1/conversations/",
		"GET /v1/conversations/attachments/registry",
		"POST /v1/conversations//attachments",
		"GET /v1/conversations//attachments/status",
		"GET /v1/conversations//attachments",
		"POST /v1/conversations//attachments/delete",
	}
	if len(got) != len(want) {
		t.Fatalf("the conversation route register carries %d routes, want %d: %v", len(got), len(want), got)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("route %q missing from the register: %v", w, got)
		}
	}
	for _, g := range got {
		if strings.Contains(g, "stream") || strings.Contains(g, "stop") || strings.Contains(g, "steer") {
			t.Fatalf("no stream/stop/steering endpoint may exist (the owner gap): %q", g)
		}
	}
}

// TestConversationEndpoints drives the surface end to end over the wire:
// create (the Core-issued identity) → send (the recorded exchange with the
// caller's correlation ID, NO answer member anywhere) → the messages read
// (the authoritative timeline) → the S4 exchange-status query → the list
// and its server-side search.
func TestConversationEndpoints(t *testing.T) {
	env := newTestEnv(t)

	// Create.
	status, body := env.do(t, "POST", "/v1/conversations", `{"title":"the wire conversation"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Conversation struct {
			ConversationID string `json:"conversation_id"`
			Title          string `json:"title"`
			CreatedAt      string `json:"created_at"`
			UpdatedAt      string `json:"updated_at"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("create body: %v", err)
	}
	if !strings.HasPrefix(created.Conversation.ConversationID, "conv-") || created.Conversation.Title != "the wire conversation" {
		t.Fatalf("the created aggregate: %+v", created.Conversation)
	}
	convID := created.Conversation.ConversationID

	// Get.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("get: %d %s", status, body)
	}

	// Send: the recorded exchange, the caller's correlation ID, the user
	// message — and NO answer/assistant member anywhere in the body.
	sendBody := `{"content":"the wire question","correlation_id":"corr-wire-1"}`
	status, body = env.do(t, "POST", "/v1/conversations/"+convID+"/send", sendBody, env.token, nil)
	if status != 200 {
		t.Fatalf("send: %d %s", status, body)
	}
	var sent struct {
		Exchange struct {
			CorrelationID  string `json:"correlation_id"`
			ConversationID string `json:"conversation_id"`
			UserMessageID  string `json:"user_message_id"`
			State          string `json:"state"`
		} `json:"exchange"`
		Message struct {
			MessageID string `json:"message_id"`
			Seq       int64  `json:"seq"`
			Role      string `json:"role"`
			Content   string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("send body: %v", err)
	}
	if sent.Exchange.CorrelationID != "corr-wire-1" || sent.Exchange.State != "recorded" {
		t.Fatalf("the recorded exchange: %+v", sent.Exchange)
	}
	if sent.Message.Role != "user" || sent.Message.Content != "the wire question" {
		t.Fatalf("the persisted user message: %+v", sent.Message)
	}
	if strings.Contains(string(body), "assistant") || strings.Contains(string(body), "answer") ||
		strings.Contains(string(body), "stream") {
		t.Fatalf("the send response carries no answer/stream member (no producer exists): %s", body)
	}

	// Messages read: the authoritative timeline in Seq order.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/messages", "", env.token, nil)
	if status != 200 {
		t.Fatalf("messages: %d %s", status, body)
	}
	var msgs struct {
		Messages []struct {
			Seq           int64  `json:"seq"`
			Role          string `json:"role"`
			Content       string `json:"content"`
			CorrelationID string `json:"correlation_id"`
		} `json:"messages"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &msgs); err != nil {
		t.Fatalf("messages body: %v", err)
	}
	if len(msgs.Messages) != 1 || msgs.Messages[0].Seq != 1 || msgs.Messages[0].CorrelationID != "corr-wire-1" {
		t.Fatalf("the timeline: %+v", msgs.Messages)
	}
	if msgs.Cursor != "" {
		t.Fatalf("a single message page is the final page (cursorless): %q", msgs.Cursor)
	}

	// The S4 exchange-status query: the authoritative exchange state by the
	// correlation ID.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/exchange?correlation_id=corr-wire-1", "", env.token, nil)
	if status != 200 {
		t.Fatalf("exchange query: %d %s", status, body)
	}
	var resolved struct {
		Exchange struct {
			CorrelationID string `json:"correlation_id"`
			State         string `json:"state"`
		} `json:"exchange"`
	}
	if err := json.Unmarshal(body, &resolved); err != nil {
		t.Fatalf("exchange body: %v", err)
	}
	if resolved.Exchange.CorrelationID != "corr-wire-1" || resolved.Exchange.State != "recorded" {
		t.Fatalf("the S4 query must resolve the recorded exchange: %+v", resolved)
	}

	// The list and the server-side search.
	status, body = env.do(t, "GET", "/v1/conversations", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var list struct {
		Conversations []struct {
			ConversationID string `json:"conversation_id"`
		} `json:"conversations"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(list.Conversations) != 1 || list.Conversations[0].ConversationID != convID {
		t.Fatalf("the list: %+v", list.Conversations)
	}
	status, body = env.do(t, "GET", "/v1/conversations?q=wire+conversation", "", env.token, nil)
	if status != 200 {
		t.Fatalf("search: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("search body: %v", err)
	}
	if len(list.Conversations) != 1 {
		t.Fatalf("the search must narrow server-side: %+v", list.Conversations)
	}
	status, body = env.do(t, "GET", "/v1/conversations?q=nothing-matches", "", env.token, nil)
	if status != 200 {
		t.Fatalf("no-match search: %d %s", status, body)
	}
	if !strings.Contains(string(body), `"conversations":[]`) {
		t.Fatalf("a no-match search is the honest empty page: %s", body)
	}
}

// TestConversationWireRefusals pins the typed wire refusals: the
// unauthenticated calls → 403 AUTHZ_DENIED through the decision port; the
// malformed bodies and query values → 400 VALIDATION_FAILED; the unknown
// conversation → 404 NOT_FOUND; the duplicate correlation identity → 409
// CONFLICT_STATE (never a duplicate exchange); a stream-shaped path is no
// endpoint at all; and the browser-shaped POST without a session keeps the
// boundary's fail-closed refusal.
func TestConversationWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Create + send a first exchange to have a recorded identity.
	status, body := env.do(t, "POST", "/v1/conversations", `{"title":"refusals"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Conversation struct {
			ConversationID string `json:"conversation_id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	convID := created.Conversation.ConversationID
	status, _ = env.do(t, "POST", "/v1/conversations/"+convID+"/send",
		`{"content":"first","correlation_id":"corr-dup"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("first send: %d", status)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
		want   int
		code   string
	}{
		{"unauthenticated create", "POST", "/v1/conversations", `{"title":"x"}`, "", 403, "AUTHZ_DENIED"},
		{"unauthenticated list", "GET", "/v1/conversations", "", "", 403, "AUTHZ_DENIED"},
		{"unauthenticated send", "POST", "/v1/conversations/" + convID + "/send", `{"content":"x","correlation_id":"c"}`, "", 403, "AUTHZ_DENIED"},
		{"unauthenticated messages", "GET", "/v1/conversations/" + convID + "/messages", "", "", 403, "AUTHZ_DENIED"},
		{"unauthenticated exchange query", "GET", "/v1/conversations/" + convID + "/exchange?correlation_id=corr-dup", "", "", 403, "AUTHZ_DENIED"},
		{"malformed send body", "POST", "/v1/conversations/" + convID + "/send", `{not json`, env.token, 400, "VALIDATION_FAILED"},
		{"missing correlation id", "POST", "/v1/conversations/" + convID + "/send", `{"content":"x"}`, env.token, 400, "VALIDATION_FAILED"},
		{"missing content", "POST", "/v1/conversations/" + convID + "/send", `{"correlation_id":"c2"}`, env.token, 400, "VALIDATION_FAILED"},
		{"malformed limit", "GET", "/v1/conversations?limit=zero", "", env.token, 400, "VALIDATION_FAILED"},
		{"missing correlation query", "GET", "/v1/conversations/" + convID + "/exchange", "", env.token, 400, "VALIDATION_FAILED"},
		{"unknown conversation get", "GET", "/v1/conversations/conv-nope", "", env.token, 404, "NOT_FOUND"},
		{"unknown conversation send", "POST", "/v1/conversations/conv-nope/send", `{"content":"x","correlation_id":"c3"}`, env.token, 404, "NOT_FOUND"},
		{"duplicate correlation identity", "POST", "/v1/conversations/" + convID + "/send", `{"content":"duplicate","correlation_id":"corr-dup"}`, env.token, 409, "CONFLICT_STATE"},
		{"no stream endpoint", "GET", "/v1/conversations/" + convID + "/stream", "", env.token, 404, "NOT_FOUND"},
		{"no stop endpoint", "POST", "/v1/conversations/" + convID + "/stop", `{}`, env.token, 404, "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, tc.body, tc.token, nil)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, body)
			}
			if !strings.Contains(string(body), tc.code) {
				t.Fatalf("body must carry %s: %s", tc.code, body)
			}
		})
	}

	// The refused duplicate wrote nothing: the timeline still carries
	// exactly the first exchange's message.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/messages", "", env.token, nil)
	if status != 200 {
		t.Fatalf("messages: %d %s", status, body)
	}
	if strings.Count(string(body), `"message_id"`) != 1 {
		t.Fatalf("the refused duplicate must leave the timeline untouched: %s", body)
	}
}
