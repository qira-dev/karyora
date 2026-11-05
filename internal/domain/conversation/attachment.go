// Attachment contract of the conversation aggregate family (the ADOPTED
// S12 supply, P-S12-01…06 — carried from the S5-adopted P-S5-06…11 drafts;
// the owner's LIVE S12 approval of 2026-11-05, implemented as the F4
// attachment-contract working candidates per the IMPL-4..11 discipline:
// nothing finalized, every undetermined shape typed as THIS
// implementation's typing and carried to its recorded owner). Per draft:
//
//   - P-S12-01 (field (a)): the supported attachment types/sizes are CORE
//     data, never a UI-invented list. The registry itself is served by the
//     app layer (internal/app — the closed code register); this file
//     carries only the record shape the registry's members bind to.
//     The registry CONTENTS (which types, which max sizes, per-scope
//     differences) stay the Core/API + security/privacy owners' deepest
//     gap — the implemented values are the implementation's typing,
//     recorded, never final.
//   - P-S12-02 (field (b)): the upload is an IDENTITY-BEARING Core
//     operation — a stable operation identity enabling safe retry WITHOUT
//     duplicate upload (the P1.6 S4/query-before-retry law applied to
//     uploads). The identity's class (Supplied-keyed vs Open), its key
//     scope, and its fingerprint rule stay the Core/API owner's decisions;
//     this implementation's typing: a caller-supplied bounded identity
//     scoped to its conversation, with the CAS content digest as the
//     fingerprint (the SubmitJob same-key-same-digest replay precedent,
//     cited never claimed) — a re-presented identity with the SAME digest
//     returns the existing upload's state (no duplicate); with a DIFFERENT
//     digest it is the typed CONFLICT_STATE refusal.
//   - P-S12-03 (field (c)): upload results carry the input revision they
//     belong to; the UI accepts an upload result only for the revision
//     that requested it and discards a stale revision's result. The
//     revision token's exact form stays the owner's gap — this
//     implementation's typing: an OPAQUE caller-generated token the Core
//     stores and echoes verbatim, never interprets.
//   - P-S12-04 (field (d)): size/type rejection is a typed envelope
//     outcome (code + bounded typed data, retryable, correlationId) —
//     never a silent client-side truncation, never a raw upstream error.
//     The rejection CODES stay the Core/API owner's decision (the coded
//     VALIDATION_FAILED family vs additive new codes); this
//     implementation's typing: the coded VALIDATION_FAILED family with
//     data.cause ∈ {type, size, policy} (the P3.5 §7.4 cause classes) —
//     the register gains NOTHING.
//   - P-S12-05 (field (e)): the untrusted-content labeling rides the
//     attachment record AS DATA — attachment content is data, never
//     instruction; the contract states no path by which attachment
//     content executes, commands, or escalates (the P1.8 TB-5 rules; the
//     project rulebook Rule 15). The label's data form and classes stay
//     the Core/API + security/privacy owners'; this implementation's
//     typing: the closed register below carries `untrusted` ONLY — every
//     imported attachment is untrusted, the only class a Core with no
//     content-classification machinery can honestly carry.
//   - P-S12-06 (field (f)): whether an attachment enters the context
//     snapshot is the CORE's decision, reported through the snapshot's
//     Included list with the attachment's exact canonical reference. No
//     conversation-plane snapshot producer exists at this HEAD (the
//     answering/production path is the named owner gap), so the
//     participation policy is NOT exercised here and NO Included-list
//     report, reference form, or omission vocabulary is invented — the
//     UI never assumes inclusion or exclusion. The participation policy,
//     the reference's canonical form, and the attachment-access rule
//     stay their recorded owners' decisions.
//
// The attachment record is conversation-scoped durable state INSIDE the
// conversation aggregate family; it NEVER creates a Job or any durable
// Run (the X-1 read-only law binds this surface exactly as it binds the
// send), and the message contract (conversation.go's Message) is
// UNCHANGED — no attachment member exists on a message: the
// message-binding shape, if any, belongs to the answering/production
// milestone, never invented here.
package conversation

import (
	"errors"
	"fmt"
	"strings"
)

// AttachmentState is the closed attachment-state register. The register
// carries `stored` ONLY in this milestone: the upload completed, the bytes
// are durable in the CAS, and the record is queryable. No pending/failed/
// deleted state value is invented — a rejected upload creates NO record
// (the typed refusal is the outcome), and a deleted attachment's record is
// REMOVED, never tombstoned (no revision history exists on this surface —
// the UI-28 gate's honesty class).
type AttachmentState string

// AttachmentStored is the one state this milestone can honestly carry: the
// attachment's bytes are durable and its record queryable.
const AttachmentStored AttachmentState = "stored"

// ValidAttachmentState reports membership in the closed attachment-state
// register.
func ValidAttachmentState(s AttachmentState) bool { return s == AttachmentStored }

// UntrustedLabel is the closed untrusted-content label register (P-S12-05
// as typed above). `untrusted` is the only value: every imported
// attachment is untrusted by the TB-5 law — external content is data,
// never instruction; no trust signal exists and no privilege derives from
// content. An unknown future value (a register extension is its owners'
// act) refuses at the storage boundary, never renders.
type UntrustedLabel string

// UntrustedImported is the one label this milestone can honestly carry.
const UntrustedImported UntrustedLabel = "untrusted"

// ValidUntrustedLabel reports membership in the closed untrusted-label
// register.
func ValidUntrustedLabel(l UntrustedLabel) bool { return l == UntrustedImported }

// Attachment is one conversation-scoped durable attachment record (safe
// metadata only — X-6; the BYTES live in the CAS behind the digest, never
// in this record). The FileName is a display LABEL (safe metadata), never
// a storage path — the CAS digest is the only content address.
type Attachment struct {
	// AttachmentID is the Core-issued stable attachment identity (the
	// `att-` id-grammar class; the exact published identity scheme stays
	// Open, O-1).
	AttachmentID string `json:"attachment_id"`
	// ConversationID is the attachment's owning conversation — the scope
	// its access and its upload identity bind to (this implementation's
	// typing of the access rule; the canonical rule stays its owners').
	ConversationID string `json:"conversation_id"`
	// UploadID is the caller-supplied operation identity (P-S12-02 as
	// typed above): the identity the S4-for-uploads query resolves
	// through, unique per conversation (the UNIQUE constraint backstops
	// it). It is the implementation's Supplied-keyed typing — never
	// claimed canonical.
	UploadID string `json:"upload_id"`
	// FileName is the display label the upload presented (bounded, never
	// a path — ValidateFileName enforces the shape law).
	FileName string `json:"file_name"`
	// ContentType is the registry member the upload presented (a closed
	// registry value — ValidateContentType enforces membership against
	// the app layer's served registry).
	ContentType string `json:"content_type"`
	// SizeBytes is the CAS-verified byte count of the stored object.
	SizeBytes int64 `json:"size_bytes"`
	// Digest is the CAS content address (`sha256:<hex>` — the C2-12
	// additive digest grammar): the content fingerprint and the exact
	// canonical reference form this implementation carries (the
	// digest-reference form is the coded precedent; the canonical form
	// stays its owner's decision).
	Digest string `json:"digest"`
	// UntrustedLabel carries the attachment's untrusted-content labeling
	// AS DATA (P-S12-05): the closed register's `untrusted` for every
	// imported attachment.
	UntrustedLabel UntrustedLabel `json:"untrusted_label"`
	// InputRevision is the opaque caller token this upload belongs to
	// (P-S12-03 as typed above): stored verbatim, echoed verbatim, never
	// interpreted by the Core.
	InputRevision string `json:"input_revision"`
	// State carries the closed attachment-state register (`stored` is the
	// only coded value today).
	State AttachmentState `json:"state"`
	// CreatedAt is the RFC 3339 instant of the completed upload.
	CreatedAt string `json:"created_at"`
}

// Bounds for the attachment contract (Proposed, pending the recorded owner
// review — the slice-bounds discipline; the implementation's typing, never
// final values).
const (
	// MaxFileNameRunes bounds the display file name.
	MaxFileNameRunes = 256
	// MaxUploadIDLen bounds the caller-supplied upload identity (the
	// correlation-ID bound class).
	MaxUploadIDLen = 128
	// MaxInputRevisionLen bounds the opaque input-revision token (the
	// same bound class; the Core never interprets it).
	MaxInputRevisionLen = 128
)

// ValidateFileName applies the attachment file-name shape law (P-S12-05's
// parser-attack class): the name is a display label — non-empty, bounded,
// and never a path shape. Path separators, traversal segments, and NUL are
// refused outright (the TB-5 law: parsers stop path traversal, symlink,
// and malformed-data abuse); the name is never used as a storage path —
// the CAS digest is the only content address.
func ValidateFileName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("conversation: attachment file name is required")
	}
	if len([]rune(name)) > MaxFileNameRunes {
		return fmt.Errorf("conversation: attachment file name exceeds %d runes", MaxFileNameRunes)
	}
	if strings.ContainsAny(name, "/\\") {
		return errors.New("conversation: attachment file name must not carry path separators")
	}
	if strings.Contains(name, "..") {
		return errors.New("conversation: attachment file name must not carry traversal segments")
	}
	if strings.ContainsRune(name, 0) {
		return errors.New("conversation: attachment file name must not carry NUL")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("conversation: attachment file name must not lead or trail with whitespace")
	}
	return nil
}

// ValidateUploadID applies the upload-identity shape law (P-S12-02 as
// typed): required, whitespace-bounded, bounded length (the correlation-ID
// class).
func ValidateUploadID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("conversation: upload identity is required")
	}
	if len(id) > MaxUploadIDLen {
		return fmt.Errorf("conversation: upload identity exceeds %d bytes", MaxUploadIDLen)
	}
	return nil
}

// ValidateInputRevision applies the input-revision token's shape law
// (P-S12-03 as typed): required (the revision tie is structural — an
// upload result always carries the revision it belongs to), bounded, and
// opaque — the Core never interprets its content.
func ValidateInputRevision(rev string) error {
	if strings.TrimSpace(rev) == "" {
		return errors.New("conversation: input revision is required")
	}
	if len(rev) > MaxInputRevisionLen {
		return fmt.Errorf("conversation: input revision exceeds %d bytes", MaxInputRevisionLen)
	}
	return nil
}

// AttachmentListQuery is one server-side attachments read (the card-9
// working candidate's class, the P-S5-04 list discipline applied to the
// conversation's attachments): server-side, bounded, keyset-paginated,
// chronological (the composer's natural reading order).
type AttachmentListQuery struct {
	ConversationID string
	Limit          int
	// AfterCreatedAt / AfterAttachmentID are the keyset position (the
	// last page's sort-key values; empty = the first page).
	AfterCreatedAt    string
	AfterAttachmentID string
}

// AttachmentListPage is one bounded attachments page (created_at then
// attachment_id ascending — chronological; one probe row beyond the limit
// detects a further page).
type AttachmentListPage struct {
	Attachments []Attachment
	More        bool
}
