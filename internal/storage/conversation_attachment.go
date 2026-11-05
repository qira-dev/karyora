package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The durable conversation-attachment store (the ADOPTED S12 supply,
// P-S12-01…06, implemented as the F4 attachment-contract working
// candidates — migration 0012). The attachment is conversation-scoped
// durable state: its BYTES live in the CAS behind the content digest (the
// C2-12 staged-write protocol: publish, then the record row + the
// cas_references row in ONE transaction — the reference commit only after
// publish), its RECORD is the queryable metadata (the S4-for-uploads query
// resolves through it). The attachment path NEVER writes a Job row or
// touches any Job machinery (X-1). No message binding, no snapshot
// reference, no tombstone: the record is created on a completed upload and
// removed on delete — nothing else exists.

// randomAttachmentID mints the Core-issued per-attachment identity (the
// `att-` id-grammar class; entropy failure aborts, exactly like the
// message-id mint).
func randomAttachmentID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("storage: entropy unavailable: " + err.Error())
	}
	return "att-" + hex.EncodeToString(b[:])
}

// conversationAttachmentOwnerType is the attachment's cas_references
// owner type (the GC barrier's owner record — the reference row holds the
// object reachable for this owner; deleting the reference row releases
// the object to the store's normal mark/sweep discipline).
const conversationAttachmentOwnerType = "conversation.attachment"

const conversationAttachmentColumns = `attachment_id, conversation_id, upload_id, file_name, content_type, size_bytes, digest, untrusted_label, input_revision, state, created_at`

func scanAttachment(row rowScanner) (conversation.Attachment, error) {
	var a conversation.Attachment
	var label, state string
	err := row.Scan(&a.AttachmentID, &a.ConversationID, &a.UploadID, &a.FileName, &a.ContentType, &a.SizeBytes,
		&a.Digest, &label, &a.InputRevision, &state, &a.CreatedAt)
	if err != nil {
		return a, err
	}
	if !conversation.ValidUntrustedLabel(conversation.UntrustedLabel(label)) {
		return conversation.Attachment{}, errStoragef(errs.CodeStateCorrupt, "attachment %q carries an unregistered untrusted label", a.AttachmentID)
	}
	a.UntrustedLabel = conversation.UntrustedLabel(label)
	if !conversation.ValidAttachmentState(conversation.AttachmentState(state)) {
		return conversation.Attachment{}, errStoragef(errs.CodeStateCorrupt, "attachment %q carries an unregistered state", a.AttachmentID)
	}
	a.State = conversation.AttachmentState(state)
	return a, nil
}

// CreateAttachment stores ONE upload (P-S12-02): the replay ruling FIRST
// (the duplicate-free guarantee is the Core's to make) — a re-presented
// (conversation, upload identity) pair with the SAME digest returns the
// recorded attachment without re-storing (no duplicate); with a DIFFERENT
// digest it is the typed CONFLICT_STATE (the SubmitJob same-key-same-digest
// precedent, cited never claimed) — then the CAS publish (idempotent by
// content address) and the record row + cas_references row in ONE
// transaction (the C2-12 reference-commit order).
func (s *Store) CreateAttachment(ctx context.Context, rec conversation.Attachment, content []byte) (conversation.Attachment, error) {
	if rec.ConversationID == "" {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "an attachment requires its conversation identity")
	}
	if err := conversation.ValidateUploadID(rec.UploadID); err != nil {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	if err := conversation.ValidateFileName(rec.FileName); err != nil {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "%v", err)
	}
	if rec.UntrustedLabel != conversation.UntrustedImported {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "untrusted label %q is not a registered label", rec.UntrustedLabel)
	}
	if rec.State != conversation.AttachmentStored {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "attachment state %q is not a registered state", rec.State)
	}
	if _, err := conversationRowByID(ctx, s, rec.ConversationID); err != nil {
		return conversation.Attachment{}, err
	}

	// The replay ruling over the durable record (the store's to make):
	// same (conversation, upload identity) + same digest → the recorded
	// attachment returns unchanged (replay, no duplicate); different
	// digest → the typed CONFLICT_STATE.
	digest := DigestOf(content)
	existing, lookupErr := s.attachmentByUpload(ctx, rec.ConversationID, rec.UploadID)
	if lookupErr == nil {
		if existing.Digest == digest {
			return existing, nil
		}
		return conversation.Attachment{}, errStoragef(errs.CodeConflictState,
			"upload identity %q is already recorded on this conversation with different content; the status query resolves its outcome", rec.UploadID)
	}
	var se *Error
	if !errors.As(lookupErr, &se) || se.Code != errs.CodeNotFound {
		return conversation.Attachment{}, lookupErr
	}

	// The C2-12 staged-write protocol: publish the bytes first (idempotent
	// by content address — a concurrent write of the same digest
	// converges), then the record row + the reference row in ONE
	// transaction (the reference commit only after publish).
	digest, err := s.cas.Put(ctx, content)
	if err != nil {
		return conversation.Attachment{}, err
	}
	attachmentID := rec.AttachmentID
	if attachmentID == "" {
		attachmentID = randomAttachmentID()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if rec.CreatedAt != "" {
		now = rec.CreatedAt
	}
	err = s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`INSERT INTO conversation_attachments
			(attachment_id, conversation_id, upload_id, file_name, content_type, size_bytes, digest, untrusted_label, input_revision, state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			attachmentID, rec.ConversationID, rec.UploadID, rec.FileName, rec.ContentType, int64(len(content)), digest,
			string(rec.UntrustedLabel), rec.InputRevision, string(rec.State), now); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO cas_references
			(blob_ref, owner_type, owner_id, registered_at) VALUES (?, ?, ?, ?)`,
			digest, conversationAttachmentOwnerType, attachmentID, now); err != nil {
			return mapVendorError(err)
		}
		return nil
	})
	if err != nil {
		return conversation.Attachment{}, err
	}
	return s.attachmentByID(ctx, rec.ConversationID, attachmentID)
}

func attachmentByUploadWhere(ctx context.Context, s *Store, where string, args []any) (conversation.Attachment, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+conversationAttachmentColumns+` FROM conversation_attachments `+where, args...)
	rec, err := scanAttachment(row)
	if err == sql.ErrNoRows {
		return conversation.Attachment{}, errStoragef(errs.CodeNotFound, "attachment %q is not stored", args[len(args)-1])
	}
	return rec, err
}

func (s *Store) attachmentByUpload(ctx context.Context, conversationID, uploadID string) (conversation.Attachment, error) {
	return attachmentByUploadWhere(ctx, s, `WHERE conversation_id = ? AND upload_id = ?`, []any{conversationID, uploadID})
}

func (s *Store) attachmentByID(ctx context.Context, conversationID, attachmentID string) (conversation.Attachment, error) {
	return attachmentByUploadWhere(ctx, s, `WHERE conversation_id = ? AND attachment_id = ?`, []any{conversationID, attachmentID})
}

// LoadAttachmentByUpload resolves the S4-for-uploads query: the
// attachment bound to (conversation identity, upload identity). The pair
// is the only lookup — an upload identity on a DIFFERENT conversation is
// the SAME typed NOT_FOUND as unknown (the no-leak rule).
func (s *Store) LoadAttachmentByUpload(ctx context.Context, conversationID, uploadID string) (conversation.Attachment, error) {
	if conversationID == "" || uploadID == "" {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "an attachment status read requires its conversation identity and upload identity")
	}
	return s.attachmentByUpload(ctx, conversationID, uploadID)
}

// LoadAttachment reads one attachment by its Core-issued identity within
// its conversation; a foreign binding is the SAME typed NOT_FOUND as
// unknown (the no-leak rule).
func (s *Store) LoadAttachment(ctx context.Context, conversationID, attachmentID string) (conversation.Attachment, error) {
	if conversationID == "" || attachmentID == "" {
		return conversation.Attachment{}, errStoragef(errs.CodeValidationFailed, "an attachment read requires its conversation identity and attachment identity")
	}
	return s.attachmentByID(ctx, conversationID, attachmentID)
}

// ListAttachments reads one bounded page of a conversation's attachments
// in chronological order (created_at then attachment_id ascending — the
// composer's natural reading order), one probe row beyond the limit to
// detect a further page. Fail-closed guards: a non-positive limit and an
// empty conversation identity refuse.
func (s *Store) ListAttachments(ctx context.Context, q conversation.AttachmentListQuery) (conversation.AttachmentListPage, error) {
	if q.ConversationID == "" {
		return conversation.AttachmentListPage{}, errStoragef(errs.CodeValidationFailed, "an attachment list requires its conversation identity")
	}
	if q.Limit < 1 {
		return conversation.AttachmentListPage{}, errStoragef(errs.CodeValidationFailed, "attachment list limit must be at least 1")
	}
	where := ` WHERE conversation_id = ?`
	args := []any{q.ConversationID}
	if q.AfterCreatedAt != "" || q.AfterAttachmentID != "" {
		if q.AfterCreatedAt == "" || q.AfterAttachmentID == "" {
			return conversation.AttachmentListPage{}, errStoragef(errs.CodeValidationFailed, "a keyset position requires its attachment_id tiebreak")
		}
		where += ` AND (created_at > ? OR (created_at = ? AND attachment_id > ?))`
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterAttachmentID)
	}
	args = append(args, q.Limit+1)
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	rows, err := s.readers.QueryContext(cctx,
		`SELECT `+conversationAttachmentColumns+` FROM conversation_attachments`+where+` ORDER BY created_at ASC, attachment_id ASC LIMIT ?`, args...)
	if err != nil {
		return conversation.AttachmentListPage{}, mapVendorError(err)
	}
	defer rows.Close()
	page := conversation.AttachmentListPage{Attachments: []conversation.Attachment{}}
	for rows.Next() {
		rec, err := scanAttachment(rows)
		if err != nil {
			return conversation.AttachmentListPage{}, err
		}
		page.Attachments = append(page.Attachments, rec)
	}
	if err := rows.Err(); err != nil {
		return conversation.AttachmentListPage{}, mapVendorError(err)
	}
	if len(page.Attachments) > q.Limit {
		page.Attachments = page.Attachments[:q.Limit]
		page.More = true
	}
	return page, nil
}

// DeleteAttachment removes ONE attachment: its record row and its
// cas_references row in ONE transaction (the CAS object then becomes
// GC-collectible through the store's normal barrier discipline — no
// direct file deletion exists on this path). Unknown or foreign-bound =
// the typed NOT_FOUND; nothing else is written (no tombstone, no event,
// no Job row — X-1).
func (s *Store) DeleteAttachment(ctx context.Context, conversationID, attachmentID string) error {
	if conversationID == "" || attachmentID == "" {
		return errStoragef(errs.CodeValidationFailed, "an attachment delete requires its conversation identity and attachment identity")
	}
	rec, err := s.attachmentByID(ctx, conversationID, attachmentID)
	if err != nil {
		return err
	}
	return s.withWriter(ctx, func(tx *sqlTx) error {
		res, err := tx.Exec(`DELETE FROM conversation_attachments WHERE attachment_id = ? AND conversation_id = ?`, attachmentID, conversationID)
		if err != nil {
			return mapVendorError(err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errStoragef(errs.CodeNotFound, "attachment %q is not stored", attachmentID)
		}
		if _, err := tx.Exec(`DELETE FROM cas_references WHERE blob_ref = ? AND owner_type = ? AND owner_id = ?`,
			rec.Digest, conversationAttachmentOwnerType, attachmentID); err != nil {
			return mapVendorError(err)
		}
		return nil
	})
}
