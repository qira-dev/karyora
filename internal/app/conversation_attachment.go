package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The conversation attachment surface (the ADOPTED S12 supply,
// P-S12-01…06 — the owner's LIVE S12 approval of 2026-11-05 — implemented
// as the F4 attachment-contract working candidates per the IMPL-4..11
// discipline: nothing finalized, every undetermined shape typed as THIS
// implementation's typing and carried to its recorded owner; the
// conversation-authority mapping [B] is NOT touched here — it stays its
// own recorded item). Per draft:
//
//   - P-S12-01: the registry below is CORE data — a closed code register
//     served through the decision port; the UI renders ONLY it, invents no
//     list, no guess, no default allowance. The registry CONTENTS (the
//     types, the max sizes, the absence of any per-scope difference) are
//     the implementation's typing — the Core/API + security/privacy
//     owners' deepest gap, never claimed final.
//   - P-S12-02: the upload is identity-bearing — the caller-supplied
//     bounded identity scoped to its conversation, the CAS digest as the
//     fingerprint; the store's replay ruling returns the existing upload's
//     state on (same identity, same digest) and refuses (same identity,
//     different digest) as the typed CONFLICT_STATE — the SubmitJob
//     same-key-same-digest replay precedent, cited never claimed. On a
//     lost upload response the UI holds the S4 state and resolves by
//     ConversationAttachmentStatus — NEVER a blind re-upload.
//   - P-S12-03: the upload result echoes the input revision it belongs
//     to; the stale-revision discard is the UI's race guard (the UI
//     accepts a result only for the revision that requested it).
//   - P-S12-04: rejection is the typed envelope (the coded
//     VALIDATION_FAILED family; data.cause ∈ {type, size, policy} — the
//     P3.5 §7.4 cause classes); never a silent client-side truncation,
//     never a raw upstream error; the register gains NOTHING.
//   - P-S12-05: the untrusted label rides the record AS DATA (`untrusted`
//     — the closed register); attachment content is data, never
//     instruction; no path executes, commands, or escalates it.
//   - P-S12-06: NO snapshot participation is exercised — no
//     conversation-plane snapshot producer exists (the answering path is
//     the named owner gap); no Included-list report, reference form, or
//     omission vocabulary is invented; the UI never assumes either way.
//
// The X-1 law binds exactly as it binds the send: an attachment operation
// NEVER creates a Job or any durable Run and NEVER touches the Job
// machinery (pinned by test). Secrets and credentials never ride
// attachment content — the no-leak rule binds every refusal (an error
// never confirms an inaccessible conversation or attachment).

// ConversationAttachmentStore is the durable attachment port (the
// storage adapter implements it structurally; the slice consumes only
// this port). Nil disables the attachment surface — typed refusals, fail
// closed (the Conversations pattern).
type ConversationAttachmentStore interface {
	// CreateAttachment stores ONE upload atomically at the record level:
	// the CAS staged-write protocol publishes the bytes (idempotent by
	// content address), then the record row and the cas_references row
	// commit in one transaction. The replay ruling is the store's (the
	// draft's duplicate-free guarantee is the Core's to make).
	CreateAttachment(ctx context.Context, rec conversation.Attachment, content []byte) (conversation.Attachment, error)
	// LoadAttachmentByUpload resolves the S4-for-uploads query: the
	// attachment bound to (conversation identity, upload identity), or
	// the typed NOT_FOUND. An upload identity bound to a DIFFERENT
	// conversation is the SAME typed NOT_FOUND as unknown (the no-leak
	// rule).
	LoadAttachmentByUpload(ctx context.Context, conversationID, uploadID string) (conversation.Attachment, error)
	// LoadAttachment reads one attachment by its Core-issued identity; a
	// record bound to a DIFFERENT conversation is the SAME typed
	// NOT_FOUND as unknown (the no-leak rule).
	LoadAttachment(ctx context.Context, conversationID, attachmentID string) (conversation.Attachment, error)
	ListAttachments(ctx context.Context, q conversation.AttachmentListQuery) (conversation.AttachmentListPage, error)
	// DeleteAttachment removes ONE attachment's record row and its
	// cas_references row in one transaction (the CAS object then becomes
	// GC-collectible through the store's normal barrier discipline — no
	// direct file deletion exists on this path). Unknown or
	// foreign-bound = the typed NOT_FOUND.
	DeleteAttachment(ctx context.Context, conversationID, attachmentID string) error
}

// Conversation attachment bounds (Proposed, pending the recorded owner
// review — the slice-bounds discipline; the implementation's typing,
// never final values).
const (
	// DefaultConversationAttachmentListLimit is the attachments-list
	// per-surface default page size (the card-9 per-surface default
	// class).
	DefaultConversationAttachmentListLimit = 50
	// MaxConversationAttachmentListLimit is the list page bound — the
	// same 1–1000 bound class the other read surfaces use.
	MaxConversationAttachmentListLimit = 1000
)

// attachmentTypeSpec is one closed registry member: the content type and
// its max size in bytes (P-S12-01: the values are the implementation's
// typing — the owners' supply act canonicalizes them; per-scope
// differences: NONE implemented — the single Local-mode scope carries one
// registry, and no per-policy difference exists to serve).
type attachmentTypeSpec struct {
	ContentType string
	MaxBytes    int64
}

// DefaultAttachmentMaxBytes is the implementation's typed per-type size
// limit (10 MiB — the same class as the slice's other content bounds,
// well under the transport's own 32 MiB body bound even base64-inflated;
// the deeper CAS object bound remains the machinery backstop). Proposed,
// pending the recorded owner review.
const DefaultAttachmentMaxBytes = int64(10) << 20

// attachmentRegistry is the closed registry the Core serves (P-S12-01 as
// typed above): text-family and PNG members only — the minimal honest
// placeholder set, chosen by the implementation, never claimed canonical.
var attachmentRegistry = []attachmentTypeSpec{
	{ContentType: "text/plain", MaxBytes: DefaultAttachmentMaxBytes},
	{ContentType: "text/markdown", MaxBytes: DefaultAttachmentMaxBytes},
	{ContentType: "application/json", MaxBytes: DefaultAttachmentMaxBytes},
	{ContentType: "image/png", MaxBytes: DefaultAttachmentMaxBytes},
}

// AttachmentTypeSpec is the served registry member (the wire-safe shape).
type AttachmentTypeSpec struct {
	ContentType string `json:"content_type"`
	MaxBytes    int64  `json:"max_bytes"`
}

// AttachmentRegistryResult is the registry read's result: the Core's own
// supported attachment types and size limits as queryable contract data
// (P-S12-01). No per-scope difference exists in this implementation — the
// absence is carried honestly rather than dressed as a member.
type AttachmentRegistryResult struct {
	Types []AttachmentTypeSpec `json:"types"`
}

// ConversationAttachmentRegistry serves the attachment registry (the
// P-S5-06 read class): Core data through the decision port — the read
// creates nothing and probes nothing.
func (s *Slice) ConversationAttachmentRegistry(ctx context.Context) (AttachmentRegistryResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.ConversationAttachments == nil {
		return AttachmentRegistryResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation attachment store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation}, security.ActionDataRead); err != nil {
		return AttachmentRegistryResult{}, err
	}
	types := make([]AttachmentTypeSpec, 0, len(attachmentRegistry))
	for _, spec := range attachmentRegistry {
		types = append(types, AttachmentTypeSpec{ContentType: spec.ContentType, MaxBytes: spec.MaxBytes})
	}
	return AttachmentRegistryResult{Types: types}, nil
}

// attachmentSpecFor resolves the registry member for a content type; a
// non-member is nil (the typed type rejection's source — never a
// client-side guess).
func attachmentSpecFor(contentType string) (attachmentTypeSpec, bool) {
	for _, spec := range attachmentRegistry {
		if spec.ContentType == contentType {
			return spec, true
		}
	}
	return attachmentTypeSpec{}, false
}

// ConversationAttachmentUploadRequest is one identity-bearing upload
// (P-S12-02 as typed): the content rides base64 over the local JSON
// transport (the envelope-only boundary's typing — no multipart parser
// exists on this surface and none is introduced), bounded by the
// registry's per-type max; the input revision is the opaque token the
// result echoes (P-S12-03).
type ConversationAttachmentUploadRequest struct {
	ConversationID string
	UploadID       string
	FileName       string
	ContentType    string
	InputRevision  string
	// ContentB64 is the base64 (standard, padded) encoding of the
	// attachment bytes.
	ContentB64 string
}

// ConversationAttachmentUploadResult is the recorded upload: the
// attachment record (its digest, its untrusted label, the echoed input
// revision) — and NO duplicate guarantee claimed beyond the store's
// replay ruling.
type ConversationAttachmentUploadResult struct {
	Attachment conversation.Attachment
}

// UploadConversationAttachment stores one conversation attachment
// (P-S12-02): authorize → the request-shape law → the registry check
// (type membership, then the per-type size bound — the typed rejections
// BEFORE any byte is stored) → the store's atomic record (the CAS
// publish + the record row). The upload NEVER creates a Job (X-1) and
// NEVER auto-retries: a lost response resolves ONLY by
// ConversationAttachmentStatus (the S4-for-uploads query), never a blind
// re-upload.
func (s *Slice) UploadConversationAttachment(ctx context.Context, req ConversationAttachmentUploadRequest) (ConversationAttachmentUploadResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.ConversationAttachments == nil {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation attachment store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: req.ConversationID}, security.ActionDataWrite); err != nil {
		return ConversationAttachmentUploadResult{}, err
	}
	if req.ConversationID == "" {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	if err := conversation.ValidateUploadID(req.UploadID); err != nil {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "upload_id"})
	}
	if err := conversation.ValidateFileName(req.FileName); err != nil {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "file_name"})
	}
	if err := conversation.ValidateInputRevision(req.InputRevision); err != nil {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "input_revision"})
	}
	// P-S12-04's typed rejections: type membership first, then the
	// per-type size bound — each names the cause and the safe next
	// action through the envelope data; no silent truncation exists
	// anywhere on this path.
	spec, ok := attachmentSpecFor(req.ContentType)
	if !ok {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "content_type",
			"cause": "type",
		})
	}
	content, err := base64.StdEncoding.DecodeString(req.ContentB64)
	if err != nil {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content"})
	}
	if int64(len(content)) > spec.MaxBytes {
		return ConversationAttachmentUploadResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field":      "content",
			"cause":      "size",
			"limit_type": req.ContentType,
			"max_bytes":  spec.MaxBytes,
		})
	}
	rec := conversation.Attachment{
		ConversationID: req.ConversationID,
		UploadID:       req.UploadID,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		SizeBytes:      int64(len(content)),
		UntrustedLabel: conversation.UntrustedImported,
		InputRevision:  req.InputRevision,
		State:          conversation.AttachmentStored,
	}
	saved, err := s.deps.ConversationAttachments.CreateAttachment(ctx, rec, content)
	if err != nil {
		return ConversationAttachmentUploadResult{}, err
	}
	return ConversationAttachmentUploadResult{Attachment: saved}, nil
}

// ConversationAttachmentStatus is the S4-for-uploads read (P-S12-02): the
// authoritative attachment state for one upload identity, resolved by
// (conversation identity, upload identity). It is a pure QUERY — it
// creates nothing, stores nothing, never re-uploads. An upload identity
// bound to a DIFFERENT conversation is the SAME typed NOT_FOUND as an
// unknown id (the no-leak rule).
func (s *Slice) ConversationAttachmentStatus(ctx context.Context, conversationID, uploadID string) (conversation.Attachment, error) {
	corr := errs.NewCorrelationID()
	if s.deps.ConversationAttachments == nil {
		return conversation.Attachment{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation attachment store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: conversationID}, security.ActionDataRead); err != nil {
		return conversation.Attachment{}, err
	}
	if conversationID == "" {
		return conversation.Attachment{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	if err := conversation.ValidateUploadID(uploadID); err != nil {
		return conversation.Attachment{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "upload_id"})
	}
	return s.deps.ConversationAttachments.LoadAttachmentByUpload(ctx, conversationID, uploadID)
}

// ConversationAttachmentListRequest is the conversation's attachments
// read: server-side, bounded, keyset-paginated, chronological.
type ConversationAttachmentListRequest struct {
	ConversationID string
	Limit          int
	Cursor         string
}

// ConversationAttachmentListResult is one bounded page; NextCursor empty
// marks the final page.
type ConversationAttachmentListResult struct {
	Attachments []conversation.Attachment
	NextCursor  string
}

// ConversationAttachments reads one conversation's attachments (the
// card-9 list discipline): the Core's stored records, never a client-side
// projection. The read creates nothing and mutates nothing.
func (s *Slice) ConversationAttachments(ctx context.Context, conversationID string, req ConversationAttachmentListRequest) (ConversationAttachmentListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.ConversationAttachments == nil {
		return ConversationAttachmentListResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation attachment store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: conversationID}, security.ActionDataRead); err != nil {
		return ConversationAttachmentListResult{}, err
	}
	if conversationID == "" {
		return ConversationAttachmentListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultConversationAttachmentListLimit
	}
	if limit < 1 || limit > MaxConversationAttachmentListLimit {
		return ConversationAttachmentListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	q := conversation.AttachmentListQuery{ConversationID: conversationID, Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeConversationAttachmentCursor(req.Cursor)
		if err != nil {
			return ConversationAttachmentListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterAttachmentID = pos.CreatedAt, pos.AttachmentID
	}
	page, err := s.deps.ConversationAttachments.ListAttachments(ctx, q)
	if err != nil {
		return ConversationAttachmentListResult{}, err
	}
	res := ConversationAttachmentListResult{Attachments: page.Attachments}
	if page.More {
		if len(page.Attachments) == 0 {
			return ConversationAttachmentListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeConversationAttachmentCursor(page.Attachments[len(page.Attachments)-1])
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return ConversationAttachmentListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// ConversationAttachmentDeleteRequest names exactly the attachment to
// delete — no implicit target (the notification-mutation pattern).
type ConversationAttachmentDeleteRequest struct {
	ConversationID string
	AttachmentID   string
}

// DeleteConversationAttachment removes one attachment (the draft-stage
// delete affordance's Core operation): authorize → the typed
// NOT_FOUND on unknown/foreign → the record row and its cas_references
// row removed in one transaction; the CAS object becomes GC-collectible
// through the store's normal barrier discipline. The delete NEVER
// touches the Job machinery (X-1) and never tombstones (no revision
// history exists on this surface).
func (s *Slice) DeleteConversationAttachment(ctx context.Context, req ConversationAttachmentDeleteRequest) error {
	corr := errs.NewCorrelationID()
	if s.deps.ConversationAttachments == nil {
		return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "conversation attachment store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConversation, ID: req.ConversationID}, security.ActionDataWrite); err != nil {
		return err
	}
	if req.ConversationID == "" {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "conversation_id"})
	}
	if req.AttachmentID == "" {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "attachment_id"})
	}
	return s.deps.ConversationAttachments.DeleteAttachment(ctx, req.ConversationID, req.AttachmentID)
}

// conversationAttachmentCursorPosition is the decoded form behind the
// opaque attachments-list cursor: the keyset position (the last page's
// sort-key values). It follows the coded event-cursor PATTERN as a
// SEPARATE query-side mechanism (P1.7 §3.5) — the event family's cursor
// is untouched (the F10 freeze stands).
type conversationAttachmentCursorPosition struct {
	CreatedAt    string `json:"created_at"`
	AttachmentID string `json:"attachment_id"`
}

func encodeConversationAttachmentCursor(rec conversation.Attachment) (string, error) {
	if rec.AttachmentID == "" || rec.CreatedAt == "" {
		return "", fmt.Errorf("app: attachment cursor position lacks its keyset values")
	}
	b, err := json.Marshal(conversationAttachmentCursorPosition{CreatedAt: rec.CreatedAt, AttachmentID: rec.AttachmentID})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

func decodeConversationAttachmentCursor(cursor string) (conversationAttachmentCursorPosition, error) {
	var pos conversationAttachmentCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: attachment cursor is malformed")
	}
	if pos.AttachmentID == "" || pos.CreatedAt == "" {
		return pos, fmt.Errorf("app: attachment cursor is malformed")
	}
	return pos, nil
}
