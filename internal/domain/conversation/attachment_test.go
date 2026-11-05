package conversation

import (
	"encoding/json"
	"strings"
	"testing"
)

// The attachment contract's domain register (the ADOPTED S12 supply,
// P-S12-01…06 as typed): the closed registers, the shape laws, and the
// coded JSON tags — the shapes every layer above binds to.

// TestAttachmentRegistersClosed pins the closed registers: `stored` is the
// only attachment state and `untrusted` the only untrusted label — no
// value is invented for a machinery that does not exist (the role/
// exchange-state register-extension class).
func TestAttachmentRegistersClosed(t *testing.T) {
	if !ValidAttachmentState(AttachmentStored) {
		t.Fatalf("%q must be a registered state", AttachmentStored)
	}
	if ValidAttachmentState(AttachmentState("pending")) || ValidAttachmentState(AttachmentState("failed")) || ValidAttachmentState(AttachmentState("deleted")) {
		t.Fatalf("no pending/failed/deleted state may exist: the refusal is the outcome and the delete REMOVES the record")
	}
	if !ValidUntrustedLabel(UntrustedImported) {
		t.Fatalf("%q must be a registered label", UntrustedImported)
	}
	if ValidUntrustedLabel("trusted") || ValidUntrustedLabel("") {
		t.Fatalf("no trusted/empty label may exist: every imported attachment is untrusted (the TB-5 law)")
	}
}

// TestValidateFileName pins the file-name shape law (the TB-5
// parser-attack class): a display label — non-empty, bounded, never a
// path shape.
func TestValidateFileName(t *testing.T) {
	for _, ok := range []string{"notes.txt", "данные-2026.json", "image.png", "a", strings.Repeat("n", MaxFileNameRunes)} {
		if err := ValidateFileName(ok); err != nil {
			t.Fatalf("ValidateFileName(%q) = %v, want accepted", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		"   ",
		strings.Repeat("n", MaxFileNameRunes+1),
		"a/b.txt",
		`a\b.txt`,
		"../escape.txt",
		"notes..txt",
		"nul\x00.txt",
		" leading.txt",
		"trailing.txt ",
	} {
		if err := ValidateFileName(bad); err == nil {
			t.Fatalf("ValidateFileName(%q) = nil, want refused (the shape law)", bad)
		}
	}
}

// TestValidateUploadAndRevision pins the upload-identity and
// input-revision shape laws: required, bounded (the correlation-ID bound
// class); the revision is opaque — the Core never interprets it.
func TestValidateUploadAndRevision(t *testing.T) {
	if err := ValidateUploadID("up-1"); err != nil {
		t.Fatalf("ValidateUploadID: %v", err)
	}
	if err := ValidateUploadID("  "); err == nil {
		t.Fatalf("a whitespace-only upload identity must be refused")
	}
	if err := ValidateUploadID(strings.Repeat("u", MaxUploadIDLen+1)); err == nil {
		t.Fatalf("an over-bound upload identity must be refused")
	}
	if err := ValidateInputRevision("rev-1"); err != nil {
		t.Fatalf("ValidateInputRevision: %v", err)
	}
	if err := ValidateInputRevision(""); err == nil {
		t.Fatalf("an absent input revision must be refused (the revision tie is structural)")
	}
	if err := ValidateInputRevision(strings.Repeat("r", MaxInputRevisionLen+1)); err == nil {
		t.Fatalf("an over-bound input revision must be refused")
	}
}

// TestAttachmentCodedJSONTags pins the coded JSON tags: the wire shape is
// the domain contract's own (the coded-name rule).
func TestAttachmentCodedJSONTags(t *testing.T) {
	b, err := json.Marshal(Attachment{
		AttachmentID:   "att-1",
		ConversationID: "conv-1",
		UploadID:       "up-1",
		FileName:       "notes.txt",
		ContentType:    "text/plain",
		SizeBytes:      3,
		Digest:         "sha256:abc",
		UntrustedLabel: UntrustedImported,
		InputRevision:  "rev-1",
		State:          AttachmentStored,
		CreatedAt:      "2026-11-06T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"attachment_id":"att-1","conversation_id":"conv-1","upload_id":"up-1","file_name":"notes.txt",` +
		`"content_type":"text/plain","size_bytes":3,"digest":"sha256:abc","untrusted_label":"untrusted",` +
		`"input_revision":"rev-1","state":"stored","created_at":"2026-11-06T00:00:00Z"}`
	if string(b) != want {
		t.Fatalf("the coded tags drifted:\n got %s\nwant %s", b, want)
	}
}
