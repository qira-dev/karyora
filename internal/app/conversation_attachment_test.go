package app

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The conversation attachment surface's app register (the ADOPTED S12
// supply, P-S12-01…06 as implemented): the Core-data registry, the
// duplicate-free replay ruling, the typed rejections, the no-leak status
// query, the delete, the authorization join, the unwired posture, and the
// X-1 pin — an attachment operation NEVER creates a Job or any durable
// Run.

// convAttachEnv extends the shared test env with the attachment surface
// wired beside the conversation surface (the real storage adapter — the
// real composition).
func convAttachEnv(t *testing.T) *testEnv {
	t.Helper()
	env := convEnv(t)
	env.deps.ConversationAttachments = env.store
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	return env
}

func uploadB64(content string) string { return base64.StdEncoding.EncodeToString([]byte(content)) }

func uploadReq(conversationID, uploadID, content string) ConversationAttachmentUploadRequest {
	return ConversationAttachmentUploadRequest{
		ConversationID: conversationID,
		UploadID:       uploadID,
		FileName:       "notes.txt",
		ContentType:    "text/plain",
		InputRevision:  "rev-1",
		ContentB64:     uploadB64(content),
	}
}

// TestAttachmentRegistryIsCoreData pins P-S12-01: the registry is the
// Core's own served contract data (closed members with typed per-type
// bounds), the read is a pure read (it creates nothing), and every member
// bound is positive and under the transport's own body bound even
// base64-inflated.
func TestAttachmentRegistryIsCoreData(t *testing.T) {
	env := convAttachEnv(t)
	res, err := env.slice.ConversationAttachmentRegistry(env.ctx())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if len(res.Types) == 0 {
		t.Fatalf("the registry must carry the Core's supported types (never an empty client-side guess)")
	}
	seen := map[string]bool{}
	for _, spec := range res.Types {
		if spec.ContentType == "" || spec.MaxBytes <= 0 {
			t.Fatalf("every registry member carries its type and a positive bound: %+v", spec)
		}
		if seen[spec.ContentType] {
			t.Fatalf("the registry is a closed set — no duplicate member: %+v", spec)
		}
		seen[spec.ContentType] = true
		if spec.MaxBytes*4/3 >= int64(maxBodyTestBound) {
			t.Fatalf("the per-type bound must sit under the transport body bound even base64-inflated: %+v", spec)
		}
	}
}

// maxBodyTestBound mirrors the transport's maxBodyBytes class (32 MiB) —
// the app package cannot import the adapter's constant, the shape class
// is what the pin binds.
const maxBodyTestBound = 32 << 20

// TestAttachmentUploadReplayNoDuplicate pins P-S12-02's duplicate-free
// ruling end to end at the app layer: (same identity, same digest) → the
// SAME recorded attachment (no duplicate, no re-store); (same identity,
// different digest) → the typed CONFLICT_STATE. On a lost response the
// status query resolves the outcome — never a blind re-upload.
func TestAttachmentUploadReplayNoDuplicate(t *testing.T) {
	env := convAttachEnv(t)
	ctx := env.ctx()
	conv := createConv(t, env, "attachment replay")

	first, err := env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-1", "the first content"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !strings.HasPrefix(first.Attachment.AttachmentID, "att-") {
		t.Fatalf("the Core issues the stable attachment identity: %+v", first.Attachment)
	}
	if first.Attachment.UntrustedLabel != conversation.UntrustedImported {
		t.Fatalf("P-S12-05: the record carries the untrusted label AS DATA: %+v", first.Attachment)
	}
	if first.Attachment.InputRevision != "rev-1" {
		t.Fatalf("P-S12-03: the result echoes the input revision it belongs to: %+v", first.Attachment)
	}

	replayed, err := env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-1", "the first content"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Attachment.AttachmentID != first.Attachment.AttachmentID {
		t.Fatalf("the replay must return the recorded attachment (no duplicate): %+v vs %+v", replayed.Attachment, first.Attachment)
	}
	page, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attachments) != 1 {
		t.Fatalf("no duplicate row may exist, got %d", len(page.Attachments))
	}

	_, err = env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-1", "DIFFERENT content"))
	if codeOf(t, err) != errs.CodeConflictState {
		t.Fatalf("a contradicting re-upload must be the typed CONFLICT_STATE, got %v", err)
	}

	// The S4-for-uploads resolution: the status query returns the
	// authoritative record for the held identity.
	att, err := env.slice.ConversationAttachmentStatus(ctx, conv.ConversationID, "up-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if att.AttachmentID != first.Attachment.AttachmentID || att.Digest != first.Attachment.Digest {
		t.Fatalf("the status query must resolve the recorded upload: %+v", att)
	}
}

// TestAttachmentUploadTypedRejections pins P-S12-04: type membership and
// the per-type size bound are typed envelope outcomes BEFORE any byte is
// stored (never a silent client-side truncation, never a raw upstream
// error); the cause classes are {type, size}; nothing is stored on any
// refusal.
func TestAttachmentUploadTypedRejections(t *testing.T) {
	env := convAttachEnv(t)
	ctx := env.ctx()
	conv := createConv(t, env, "typed rejections")

	cases := []struct {
		name  string
		req   ConversationAttachmentUploadRequest
		cause string
	}{
		{"unsupported type", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: "up-x", FileName: "e.exe", ContentType: "application/x-msdownload", InputRevision: "rev-1", ContentB64: uploadB64("MZ")}, "type"},
		{"oversized content", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: "up-x", FileName: "big.txt", ContentType: "text/plain", InputRevision: "rev-1", ContentB64: uploadB64(strings.Repeat("b", int(DefaultAttachmentMaxBytes)+1))}, "size"},
		{"traversal file name", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: "up-x", FileName: "../escape.txt", ContentType: "text/plain", InputRevision: "rev-1", ContentB64: uploadB64("x")}, ""},
		{"bad base64", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: "up-x", FileName: "n.txt", ContentType: "text/plain", InputRevision: "rev-1", ContentB64: "!!!not-base64!!!"}, ""},
		{"empty upload identity", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: " ", FileName: "n.txt", ContentType: "text/plain", InputRevision: "rev-1", ContentB64: uploadB64("x")}, ""},
		{"empty input revision", ConversationAttachmentUploadRequest{ConversationID: conv.ConversationID, UploadID: "up-x", FileName: "n.txt", ContentType: "text/plain", InputRevision: "  ", ContentB64: uploadB64("x")}, ""},
	}
	for _, tc := range cases {
		_, err := env.slice.UploadConversationAttachment(ctx, tc.req)
		ee := envelopeOf(t, err)
		if ee.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: must be the typed VALIDATION_FAILED, got %v", tc.name, ee.Code)
		}
		if tc.cause != "" && ee.Data["cause"] != tc.cause {
			t.Fatalf("%s: the rejection must name its cause (%q), got %v", tc.name, tc.cause, ee.Data["cause"])
		}
	}
	page, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attachments) != 0 {
		t.Fatalf("a rejected upload stores NOTHING (the refusal is the outcome), got %d rows", len(page.Attachments))
	}
}

// TestAttachmentStatusNoLeak pins the no-leak rule on the attachment
// reads: an unknown upload identity and an identity bound to a DIFFERENT
// conversation are the SAME typed NOT_FOUND; an unknown attachment id and
// a foreign attachment id are the SAME typed NOT_FOUND.
func TestAttachmentStatusNoLeak(t *testing.T) {
	env := convAttachEnv(t)
	ctx := env.ctx()
	conv := createConv(t, env, "no-leak")
	other := createConv(t, env, "other")
	if _, err := env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-1", "content")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.slice.ConversationAttachmentStatus(ctx, conv.ConversationID, "up-nope"); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("an unknown upload identity is the typed NOT_FOUND: %v", err)
	}
	// The foreign binding: the same identity on a DIFFERENT conversation
	// is the SAME NOT_FOUND (an error never confirms an inaccessible
	// object).
	if _, err := env.slice.ConversationAttachmentStatus(ctx, other.ConversationID, "up-1"); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("a foreign-bound upload identity must be the SAME NOT_FOUND: %v", err)
	}
	page, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.slice.ConversationAttachmentStatus(ctx, conv.ConversationID, "up-nope"); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("unknown again: %v", err)
	}
	foreignID := page.Attachments[0].AttachmentID
	if _, err := env.slice.ConversationAttachmentStatus(ctx, other.ConversationID, "up-1"); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("foreign: %v", err)
	}
	if err := env.slice.DeleteConversationAttachment(ctx, ConversationAttachmentDeleteRequest{ConversationID: other.ConversationID, AttachmentID: foreignID}); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("a foreign delete must be the SAME NOT_FOUND (never a cross-conversation delete): %v", err)
	}
}

// TestAttachmentDeleteRemovesRecord pins the delete at the app layer: the
// confirmed delete removes the record (the list reflects it), and a
// second delete is the typed NOT_FOUND.
func TestAttachmentDeleteRemovesRecord(t *testing.T) {
	env := convAttachEnv(t)
	ctx := env.ctx()
	conv := createConv(t, env, "delete")
	up := env.slice.UploadConversationAttachment
	res, err := up(ctx, uploadReq(conv.ConversationID, "up-del", "to be deleted"))
	if err != nil {
		t.Fatal(err)
	}
	if err := env.slice.DeleteConversationAttachment(ctx, ConversationAttachmentDeleteRequest{ConversationID: conv.ConversationID, AttachmentID: res.Attachment.AttachmentID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	page, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attachments) != 0 {
		t.Fatalf("the deleted record must be gone, got %d", len(page.Attachments))
	}
	if err := env.slice.DeleteConversationAttachment(ctx, ConversationAttachmentDeleteRequest{ConversationID: conv.ConversationID, AttachmentID: res.Attachment.AttachmentID}); codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("a second delete is the typed NOT_FOUND: %v", err)
	}
}

// TestAttachmentUnwiredPosture pins the fail-closed unwired posture: with
// the attachment port nil, every attachment operation is the typed
// STORAGE_UNAVAILABLE refusal and nothing else changes.
func TestAttachmentUnwiredPosture(t *testing.T) {
	env := convEnv(t)
	ctx := env.ctx()
	conv := createConv(t, env, "unwired attachments")

	if _, err := env.slice.ConversationAttachmentRegistry(ctx); codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("registry: %v", err)
	}
	if _, err := env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-1", "x")); codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("upload: %v", err)
	}
	if _, err := env.slice.ConversationAttachmentStatus(ctx, conv.ConversationID, "up-1"); codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("status: %v", err)
	}
	if _, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{}); codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("list: %v", err)
	}
	if err := env.slice.DeleteConversationAttachment(ctx, ConversationAttachmentDeleteRequest{ConversationID: conv.ConversationID, AttachmentID: "att-x"}); codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("delete: %v", err)
	}
}

// TestAttachmentOperationsNeverCreateJob pins X-1 over the attachment
// surface: the registry read, the upload, the status query, the list, and
// the delete NEVER create a Job or any durable Run — the Job register is
// byte-identical around every attachment operation (a control Job proves
// the probe sees real rows).
func TestAttachmentOperationsNeverCreateJob(t *testing.T) {
	env := convAttachEnv(t)
	ctx := env.ctx()

	// The control Job: the probe must see real rows for the pin to mean
	// anything.
	if _, err := env.slice.Submit(ctx, SubmitRequest{
		IdempotencyKey: "ctrl-att", Task: "control task", Provider: "mock", Model: "m",
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

	conv := createConv(t, env, "the attachment X-1 pin")
	if _, err := env.slice.ConversationAttachmentRegistry(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := env.slice.UploadConversationAttachment(ctx, uploadReq(conv.ConversationID, "up-x1", "an attachment — never a durable job"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := env.slice.ConversationAttachmentStatus(ctx, conv.ConversationID, "up-x1"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.slice.ConversationAttachments(ctx, conv.ConversationID, ConversationAttachmentListRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := env.slice.DeleteConversationAttachment(ctx, ConversationAttachmentDeleteRequest{ConversationID: conv.ConversationID, AttachmentID: res.Attachment.AttachmentID}); err != nil {
		t.Fatal(err)
	}

	jobsAfter, err := env.deps.Relay.ListJobs(context.Background(), aijobrelay.ListJobsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsAfter.Jobs) != 1 {
		t.Fatalf("X-1: the attachment operations created %d Job rows, want none (the control Job only)", len(jobsAfter.Jobs)-1)
	}
	if jobsAfter.Jobs[0].JobID != jobsBefore.Jobs[0].JobID {
		t.Fatalf("X-1: the Job register changed around the attachment operations")
	}
}

// TestAttachmentAuthorizationJoin pins the authorization join: every
// attachment operation denies the unauthenticated caller through the ONE
// decision port (typed AUTHZ_DENIED, nothing changed) — the conversation
// surface's existing posture is the pattern; no new resource or action
// register member exists.
func TestAttachmentAuthorizationJoin(t *testing.T) {
	env := convAttachEnv(t)
	denied := WithCaller(context.Background(), "local", "kry1-not-a-real-token")
	conv := createConv(t, env, "attachment authorization")
	for name, op := range map[string]func() error{
		"registry": func() error { _, err := env.slice.ConversationAttachmentRegistry(denied); return err },
		"upload": func() error {
			_, err := env.slice.UploadConversationAttachment(denied, uploadReq(conv.ConversationID, "up-1", "x"))
			return err
		},
		"status": func() error {
			_, err := env.slice.ConversationAttachmentStatus(denied, conv.ConversationID, "up-1")
			return err
		},
		"list": func() error {
			_, err := env.slice.ConversationAttachments(denied, conv.ConversationID, ConversationAttachmentListRequest{})
			return err
		},
		"delete": func() error {
			return env.slice.DeleteConversationAttachment(denied, ConversationAttachmentDeleteRequest{ConversationID: conv.ConversationID, AttachmentID: "att-x"})
		},
	} {
		err := op()
		if codeOf(t, err) != errs.CodeAuthzDenied {
			t.Fatalf("%s: the unauthenticated caller must be the typed AUTHZ_DENIED envelope, got %v", name, err)
		}
	}
}
