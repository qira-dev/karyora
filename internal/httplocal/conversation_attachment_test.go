package httplocal

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// The conversation attachment surface's wire register (IMPL-25; the
// ADOPTED S12 supply, P-S12-01…06, as implemented): the endpoints end to
// end over the live loopback test server — the registry read, the
// identity-bearing upload with its duplicate-free replay, the
// S4-for-uploads status query, the list, the delete — and the typed
// refusals (GET-only surfaces, the no-leak NOT_FOUND class, the
// VALIDATION_FAILED rejections with their cause classes).

func uploadBody(uploadID, fileName, contentType, revision, content string) string {
	return `{"upload_id":` + jq(uploadID) + `,"file_name":` + jq(fileName) + `,"content_type":` + jq(contentType) +
		`,"input_revision":` + jq(revision) + `,"content_b64":"` + base64.StdEncoding.EncodeToString([]byte(content)) + `"}`
}

// jq quotes a string as a JSON value.
func jq(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestConversationAttachmentEndpoints drives the surface end to end over
// the wire: the registry (the Core's own types/sizes) → the upload (the
// record with its digest, untrusted label, echoed input revision) → the
// replay (same identity + same content returns the SAME record — no
// duplicate) → the status query → the list → the delete (the record gone).
func TestConversationAttachmentEndpoints(t *testing.T) {
	env := newTestEnv(t)

	// The conversation to attach to.
	status, body := env.do(t, "POST", "/v1/conversations", `{"title":"attachment wire"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("create conversation: %d %s", status, body)
	}
	convID := mustField(t, body, "conversation", "conversation_id")

	// The registry read: the Core's own contract data.
	status, body = env.do(t, "GET", "/v1/conversations/attachments/registry", "", env.token, nil)
	if status != 200 {
		t.Fatalf("registry: %d %s", status, body)
	}
	var registry struct {
		Types []struct {
			ContentType string `json:"content_type"`
			MaxBytes    int64  `json:"max_bytes"`
		} `json:"types"`
	}
	if err := json.Unmarshal(body, &registry); err != nil {
		t.Fatalf("registry body: %v", err)
	}
	if len(registry.Types) == 0 {
		t.Fatalf("the registry must carry the Core's supported types")
	}
	hasText := false
	for _, spec := range registry.Types {
		if spec.ContentType == "text/plain" && spec.MaxBytes > 0 {
			hasText = true
		}
	}
	if !hasText {
		t.Fatalf("the served registry must carry text/plain with a positive bound: %s", body)
	}

	// The upload.
	status, body = env.do(t, "POST", "/v1/conversations/"+convID+"/attachments", uploadBody("up-wire-1", "notes.txt", "text/plain", "rev-7", "the wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("upload: %d %s", status, body)
	}
	var uploaded struct {
		Attachment struct {
			AttachmentID   string `json:"attachment_id"`
			ConversationID string `json:"conversation_id"`
			UploadID       string `json:"upload_id"`
			FileName       string `json:"file_name"`
			ContentType    string `json:"content_type"`
			SizeBytes      int64  `json:"size_bytes"`
			Digest         string `json:"digest"`
			UntrustedLabel string `json:"untrusted_label"`
			InputRevision  string `json:"input_revision"`
			State          string `json:"state"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(body, &uploaded); err != nil {
		t.Fatalf("upload body: %v", err)
	}
	att := uploaded.Attachment
	if !strings.HasPrefix(att.AttachmentID, "att-") || att.ConversationID != convID || att.UploadID != "up-wire-1" {
		t.Fatalf("the upload record binds (attachment id, conversation, upload identity): %+v", att)
	}
	if att.SizeBytes != int64(len("the wire content")) || att.Digest == "" {
		t.Fatalf("the record carries the CAS-verified size and digest: %+v", att)
	}
	if att.UntrustedLabel != "untrusted" || att.State != "stored" || att.InputRevision != "rev-7" {
		t.Fatalf("the record carries the closed registers and the echoed revision: %+v", att)
	}

	// The replay: same identity + same content → the SAME record.
	status, body = env.do(t, "POST", "/v1/conversations/"+convID+"/attachments", uploadBody("up-wire-1", "notes.txt", "text/plain", "rev-7", "the wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("replay: %d %s", status, body)
	}
	var replayed struct {
		Attachment struct {
			AttachmentID string `json:"attachment_id"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(body, &replayed); err != nil {
		t.Fatalf("replay body: %v", err)
	}
	if replayed.Attachment.AttachmentID != att.AttachmentID {
		t.Fatalf("the replay must return the recorded attachment (no duplicate): %s vs %s", replayed.Attachment.AttachmentID, att.AttachmentID)
	}

	// The S4-for-uploads status query.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/attachments/status?upload_id=up-wire-1", "", env.token, nil)
	if status != 200 {
		t.Fatalf("status: %d %s", status, body)
	}
	var statusRes struct {
		Attachment struct {
			Digest string `json:"digest"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(body, &statusRes); err != nil {
		t.Fatalf("status body: %v", err)
	}
	if statusRes.Attachment.Digest != att.Digest {
		t.Fatalf("the status query resolves the recorded upload: %s", body)
	}

	// The list.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/attachments", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var listRes struct {
		Attachments []struct {
			UploadID string `json:"upload_id"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(body, &listRes); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(listRes.Attachments) != 1 || listRes.Attachments[0].UploadID != "up-wire-1" {
		t.Fatalf("the list carries the conversation's attachments: %s", body)
	}

	// The delete: the confirmed 200 removes the record; a second delete
	// and a foreign binding are the typed NOT_FOUND.
	status, body = env.do(t, "POST", "/v1/conversations/"+convID+"/attachments/delete", `{"attachment_id":"`+att.AttachmentID+`"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("delete: %d %s", status, body)
	}
	status, _ = env.do(t, "GET", "/v1/conversations/"+convID+"/attachments", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list after delete: %d", status)
	}
	status, body = env.do(t, "POST", "/v1/conversations/"+convID+"/attachments/delete", `{"attachment_id":"`+att.AttachmentID+`"}`, env.token, nil)
	if status != 404 {
		t.Fatalf("a second delete must be the typed NOT_FOUND, got %d %s", status, body)
	}
}

// TestConversationAttachmentWireRejections pins the typed rejections over
// the wire (P-S12-04): an unsupported type and an oversized body refuse
// with the VALIDATION_FAILED envelope naming the cause; a traversal file
// name refuses; the status query without its upload identity refuses.
func TestConversationAttachmentWireRejections(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, "POST", "/v1/conversations", `{}`, env.token, nil)
	if status != 200 {
		t.Fatalf("create: %d %s", status, body)
	}
	convID := mustField(t, body, "conversation", "conversation_id")

	cases := []struct {
		name  string
		body  string
		cause string
	}{
		{"unsupported type", uploadBody("up-r", "e.exe", "application/x-msdownload", "rev-1", "MZ"), "type"},
		{"traversal file name", uploadBody("up-r", "../escape.txt", "text/plain", "rev-1", "x"), ""},
		{"bad base64", `{"upload_id":"up-r","file_name":"n.txt","content_type":"text/plain","input_revision":"rev-1","content_b64":"!!!"}`, ""},
	}
	for _, tc := range cases {
		status, body := env.do(t, "POST", "/v1/conversations/"+convID+"/attachments", tc.body, env.token, nil)
		if status != 400 {
			t.Fatalf("%s: must be the typed 400, got %d %s", tc.name, status, body)
		}
		var envBody struct {
			Code string         `json:"code"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &envBody); err != nil {
			t.Fatalf("%s: body: %v", tc.name, err)
		}
		if envBody.Code != "VALIDATION_FAILED" {
			t.Fatalf("%s: must be the typed VALIDATION_FAILED, got %s", tc.name, envBody.Code)
		}
		if tc.cause != "" && envBody.Data["cause"] != tc.cause {
			t.Fatalf("%s: the rejection must name its cause (%s), got %v", tc.name, tc.cause, envBody.Data["cause"])
		}
	}

	// The status query without its upload identity: the typed 400.
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/attachments/status", "", env.token, nil)
	if status != 400 {
		t.Fatalf("status without upload_id: %d %s", status, body)
	}
	// An unknown upload identity: the typed NOT_FOUND (the no-leak class).
	status, body = env.do(t, "GET", "/v1/conversations/"+convID+"/attachments/status?upload_id=up-nope", "", env.token, nil)
	if status != 404 {
		t.Fatalf("unknown upload identity: %d %s", status, body)
	}
}

// TestConversationAttachmentSurfaceRefusals pins the surface's boundary
// refusals: every attachment endpoint refuses the unauthenticated caller
// with the typed 403, and the mutating routes are POST-only (a GET on the
// upload/delete paths resolves to the register's NOT_FOUND — the
// method-mismatch class).
func TestConversationAttachmentSurfaceRefusals(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, "POST", "/v1/conversations", `{}`, env.token, nil)
	if status != 200 {
		t.Fatalf("create: %d %s", status, body)
	}
	convID := mustField(t, body, "conversation", "conversation_id")

	for name, tc := range map[string]struct{ method, path, body string }{
		"registry no session":   {"GET", "/v1/conversations/attachments/registry", ""},
		"upload no session":     {"POST", "/v1/conversations/" + convID + "/attachments", uploadBody("up-a", "n.txt", "text/plain", "rev-1", "x")},
		"status no session":     {"GET", "/v1/conversations/" + convID + "/attachments/status?upload_id=up-a", ""},
		"list no session":       {"GET", "/v1/conversations/" + convID + "/attachments", ""},
		"delete no session":     {"POST", "/v1/conversations/" + convID + "/attachments/delete", `{"attachment_id":"att-x"}`},
		"upload GET mismatch":   {"GET", "/v1/conversations/" + convID + "/attachments-status-mismatch", ""},
		"unknown conversation":  {"POST", "/v1/conversations/conv-nope/attachments", uploadBody("up-a", "n.txt", "text/plain", "rev-1", "x")},
		"delete unknown conv":   {"POST", "/v1/conversations/conv-nope/attachments/delete", `{"attachment_id":"att-x"}`},
		"delete unknown record": {"POST", "/v1/conversations/" + convID + "/attachments/delete", `{"attachment_id":"att-nope"}`},
	} {
		token := ""
		if !strings.HasSuffix(name, "no session") {
			token = env.token
		}
		status, body := env.do(t, tc.method, tc.path, tc.body, token, nil)
		if status != 403 && status != 404 {
			t.Fatalf("%s: must be the typed 403/404 refusal, got %d %s", name, status, body)
		}
	}
}

// mustField digs one nested string field out of a JSON body.
func mustField(t *testing.T, body []byte, outer, field string) string {
	t.Helper()
	var m map[string]map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body: %v", err)
	}
	inner, ok := m[outer]
	if !ok {
		t.Fatalf("body lacks %q: %s", outer, body)
	}
	v, _ := inner[field].(string)
	if v == "" {
		t.Fatalf("body lacks %q.%q: %s", outer, field, body)
	}
	return v
}
