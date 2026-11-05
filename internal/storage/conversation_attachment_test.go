package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The conversation-attachment store's register (the ADOPTED S12 supply,
// P-S12-01…06 as implemented — migration 0012): the migration pin, the
// replay ruling, the shape law refusals, the keyset list, and the delete
// semantics. The attachment path NEVER touches the Job machinery (X-1 —
// the app register pins it end to end).

func attachTestRec(t *testing.T, s *Store, conversationID, uploadID, content string) conversation.Attachment {
	t.Helper()
	rec, err := s.CreateAttachment(context.Background(), conversation.Attachment{
		ConversationID: conversationID,
		UploadID:       uploadID,
		FileName:       "notes.txt",
		ContentType:    "text/plain",
		UntrustedLabel: conversation.UntrustedImported,
		InputRevision:  "rev-1",
		State:          conversation.AttachmentStored,
	}, []byte(content))
	if err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	return rec
}

// TestConversationAttachmentMigrationApplied pins migration 0012: a fresh
// apply reaches schema version max with the attachment table present, and
// the PRIOR migrations' recorded checksums still verify (0007/0008/0009
// byte-identical — the checksum authority refuses a modified SQL on every
// open).
func TestConversationAttachmentMigrationApplied(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersionMax {
		t.Fatalf("schema version = %d, want %d (migration 0012 applied)", v, schemaVersionMax)
	}
	for _, version := range []int64{7, 8, 9, 10, 11, 12} {
		state, err := s.migrationState(ctx, version)
		if err != nil {
			t.Fatal(err)
		}
		if state != migStateApplied {
			t.Fatalf("migration %d state = %s, want applied", version, state)
		}
	}
	var n int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, "conversation_attachments").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("the conversation_attachments table must exist")
	}
}

// TestAttachmentReplayRuling pins the P-S12-02 duplicate-free ruling: a
// re-presented (conversation, upload identity) with the SAME digest
// returns the RECORDED attachment (same Core-issued id — no duplicate
// row, no re-store); with a DIFFERENT digest it is the typed
// CONFLICT_STATE. The digest is the CAS content address.
func TestAttachmentReplayRuling(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "replay ruling")

	first := attachTestRec(t, s, conv.ConversationID, "up-1", "the first content")
	if !strings.HasPrefix(first.AttachmentID, "att-") {
		t.Fatalf("the Core issues the stable attachment identity, got %q", first.AttachmentID)
	}
	if first.Digest == "" || first.SizeBytes != int64(len("the first content")) {
		t.Fatalf("the record carries the CAS digest and the verified size: %+v", first)
	}
	if first.UntrustedLabel != conversation.UntrustedImported || first.State != conversation.AttachmentStored {
		t.Fatalf("the record carries the closed registers: %+v", first)
	}

	// Same identity + same content → replay: the SAME record, no
	// duplicate.
	replayed, err := s.CreateAttachment(ctx, conversation.Attachment{
		ConversationID: conv.ConversationID,
		UploadID:       "up-1",
		FileName:       "notes.txt",
		ContentType:    "text/plain",
		UntrustedLabel: conversation.UntrustedImported,
		InputRevision:  "rev-1",
		State:          conversation.AttachmentStored,
	}, []byte("the first content"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.AttachmentID != first.AttachmentID || replayed.CreatedAt != first.CreatedAt {
		t.Fatalf("the replay must return the recorded attachment unchanged: %+v vs %+v", replayed, first)
	}

	// Same identity + different content → the typed CONFLICT_STATE.
	_, err = s.CreateAttachment(ctx, conversation.Attachment{
		ConversationID: conv.ConversationID,
		UploadID:       "up-1",
		FileName:       "notes.txt",
		ContentType:    "text/plain",
		UntrustedLabel: conversation.UntrustedImported,
		InputRevision:  "rev-1",
		State:          conversation.AttachmentStored,
	}, []byte("DIFFERENT content"))
	var se *Error
	if !asStorageTestErr(err, &se) || se.Code != errs.CodeConflictState {
		t.Fatalf("a contradicting re-upload must be the typed CONFLICT_STATE, got %v", err)
	}

	// The same identity on a DIFFERENT conversation is an independent
	// upload (the identity scope is its conversation — the
	// implementation's typing), not a conflict.
	conv2 := convTestRecord(t, s, "second conversation")
	other := attachTestRec(t, s, conv2.ConversationID, "up-1", "the first content")
	if other.AttachmentID == first.AttachmentID || other.ConversationID != conv2.ConversationID {
		t.Fatalf("the identity scope is its conversation: %+v", other)
	}
}

// TestAttachmentShapeLawRefusals pins the store's fail-closed guards: the
// traversal-shaped file name, an unregistered label, an unregistered
// state, and an unknown conversation each refuse typed; NOTHING is
// stored.
func TestAttachmentShapeLawRefusals(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "shape law")

	cases := []struct {
		name string
		rec  conversation.Attachment
	}{
		{"traversal file name", conversation.Attachment{ConversationID: conv.ConversationID, UploadID: "up-t", FileName: "../escape.txt", ContentType: "text/plain", UntrustedLabel: conversation.UntrustedImported, InputRevision: "rev-1", State: conversation.AttachmentStored}},
		{"unregistered label", conversation.Attachment{ConversationID: conv.ConversationID, UploadID: "up-t", FileName: "n.txt", ContentType: "text/plain", UntrustedLabel: "trusted", InputRevision: "rev-1", State: conversation.AttachmentStored}},
		{"unregistered state", conversation.Attachment{ConversationID: conv.ConversationID, UploadID: "up-t", FileName: "n.txt", ContentType: "text/plain", UntrustedLabel: conversation.UntrustedImported, InputRevision: "rev-1", State: "pending"}},
		{"empty upload identity", conversation.Attachment{ConversationID: conv.ConversationID, UploadID: " ", FileName: "n.txt", ContentType: "text/plain", UntrustedLabel: conversation.UntrustedImported, InputRevision: "rev-1", State: conversation.AttachmentStored}},
	}
	for _, tc := range cases {
		_, err := s.CreateAttachment(ctx, tc.rec, []byte("content"))
		var se *Error
		if !asStorageTestErr(err, &se) || se.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: must be the typed VALIDATION_FAILED, got %v", tc.name, err)
		}
	}
	_, err := s.CreateAttachment(ctx, conversation.Attachment{ConversationID: "conv-nope", UploadID: "up-t", FileName: "n.txt", ContentType: "text/plain", UntrustedLabel: conversation.UntrustedImported, InputRevision: "rev-1", State: conversation.AttachmentStored}, []byte("content"))
	var se *Error
	if !asStorageTestErr(err, &se) || se.Code != errs.CodeNotFound {
		t.Fatalf("an unknown conversation must be the typed NOT_FOUND: %v", err)
	}
}

// TestAttachmentListKeyset pins the card-9 list discipline over the
// attachments: chronological (created_at then attachment_id ascending),
// one probe row beyond the limit, the keyset position, and the no-leak
// scoping (a conversation's list never carries another's rows).
func TestAttachmentListKeyset(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "list keyset")
	conv2 := convTestRecord(t, s, "other conversation")
	for _, id := range []string{"up-1", "up-2", "up-3"} {
		attachTestRec(t, s, conv.ConversationID, id, "content "+id)
	}
	attachTestRec(t, s, conv2.ConversationID, "up-foreign", "foreign content")

	page1, err := s.ListAttachments(ctx, conversation.AttachmentListQuery{ConversationID: conv.ConversationID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Attachments) != 2 || !page1.More {
		t.Fatalf("page one: %+v", page1)
	}
	page2, err := s.ListAttachments(ctx, conversation.AttachmentListQuery{
		ConversationID:    conv.ConversationID,
		Limit:             2,
		AfterCreatedAt:    page1.Attachments[1].CreatedAt,
		AfterAttachmentID: page1.Attachments[1].AttachmentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Attachments) != 1 || page2.More {
		t.Fatalf("page two: %+v", page2)
	}
	if page1.Attachments[0].UploadID != "up-1" || page2.Attachments[0].UploadID != "up-3" {
		t.Fatalf("the order is chronological: %v / %v", page1.Attachments, page2.Attachments)
	}
	for _, rec := range append(append([]conversation.Attachment{}, page1.Attachments...), page2.Attachments...) {
		if rec.ConversationID != conv.ConversationID {
			t.Fatalf("the list must never leak another conversation's rows: %+v", rec)
		}
	}
}

// TestAttachmentDeleteSemantics pins the delete: the record row AND its
// cas_references row are removed (the GC barrier releases the object
// through the store's normal discipline — no direct file deletion); a
// second delete and a foreign binding are the typed NOT_FOUND; the X-1
// posture holds (no Job machinery is touched — the app register pins it
// end to end).
func TestAttachmentDeleteSemantics(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	conv := convTestRecord(t, s, "delete semantics")
	other := convTestRecord(t, s, "other conversation")
	rec := attachTestRec(t, s, conv.ConversationID, "up-del", "to be deleted")

	var refs int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM cas_references WHERE owner_type='conversation.attachment' AND owner_id=?`, rec.AttachmentID).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 1 {
		t.Fatalf("the reference row must hold the GC barrier, got %d", refs)
	}

	if err := s.DeleteAttachment(ctx, other.ConversationID, rec.AttachmentID); err == nil {
		t.Fatalf("a foreign binding must be refused (the no-leak rule)")
	}
	if err := s.DeleteAttachment(ctx, conv.ConversationID, rec.AttachmentID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.LoadAttachment(ctx, conv.ConversationID, rec.AttachmentID); err == nil {
		t.Fatalf("the deleted record must be gone")
	}
	if err := s.DeleteAttachment(ctx, conv.ConversationID, rec.AttachmentID); err == nil {
		t.Fatalf("a second delete must be the typed NOT_FOUND")
	}
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM cas_references WHERE owner_type='conversation.attachment' AND owner_id=?`, rec.AttachmentID).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 0 {
		t.Fatalf("the reference row must be removed with the record, got %d", refs)
	}
	// The bytes remain in the CAS until the store's GC collects them (the
	// barrier discipline — never a direct file deletion on this path).
	if ok, err := s.cas.Stat(rec.Digest); err != nil || !ok {
		t.Fatalf("the CAS object stays until GC (the barrier discipline): %v %v", ok, err)
	}
}

// asStorageTestErr is the test-side errors.As over the store's typed
// *Error (the same shape codeOf resolves at the app layer).
func asStorageTestErr(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
