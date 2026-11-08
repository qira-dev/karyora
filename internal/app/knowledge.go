package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/knowledge"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/knowledgepack"
	"github.com/qira-dev/karyora/internal/security"
)

// The B1 KnowledgePack family service (DE-0 queue row 1): the published
// /knowledgepacks route family's use cases, implemented against the
// canonical contract at the working HEAD — the M-9 digest-keyed idempotency
// class, the authoritative state endpoint, the S0–S4 reconcile-by-identity
// law, the untrusted-import Core-side validation, the typed-404 no-leak
// rule, and the F10-shaped event vocabulary through the existing journal
// machinery. Every protected effect rides the single decision port
// (security.ResourceKnowledge — the additive register extension; Local mode
// stays role-free: no permission policy is invented). The store owns the
// durable wall-clock values and the transactional M-9 ruling, exactly like
// the SubmitJob precedent.

// KnowledgeStore is the durable KnowledgePack surface port (B1). The
// storage adapter implements it over the knowledge tables (migration 0013);
// the slice consumes only this port. Nil disables the surface — typed
// refusals, fail closed (the Conversations/Notifications pattern).
type KnowledgeStore interface {
	// SubmitKnowledgeImport applies the M-9 digest-keyed ruling and the
	// identity-create duplicate-create layer transactionally, then stores
	// the fresh operation (state submitted + import.submitted event) when
	// no replay resolves. replayed reports which path served the call.
	SubmitKnowledgeImport(ctx context.Context, intent knowledge.ImportIntent) (knowledge.Operation, bool, error)
	// SubmitKnowledgeExport stores the fresh export operation (state
	// submitted + export.submitted event). No idempotency layer exists —
	// the read-with-output class (none required).
	SubmitKnowledgeExport(ctx context.Context, op knowledge.Operation) (knowledge.Operation, error)
	// KnowledgeOperationStage commits one import stage transition and its
	// import.stage_entered event (the stage vocabulary from actual Core
	// stages only). A stage on a terminal operation is a CONFLICT_STATE.
	KnowledgeOperationStage(ctx context.Context, opID, correlationID, state string) (knowledge.Operation, error)
	// CompleteKnowledgeImport commits the terminal completed state, the
	// result, and the pack/version/entry rows with the family's events in
	// ONE transaction.
	CompleteKnowledgeImport(ctx context.Context, commit knowledge.ImportCommit) (knowledge.Operation, error)
	// RejectKnowledgeImport commits the terminal rejected state with its
	// explicit failure code and reason + the import.rejected event.
	RejectKnowledgeImport(ctx context.Context, opID, correlationID, failureCode, reason string) (knowledge.Operation, error)
	// FailKnowledgeOperation commits the terminal failed state with the
	// explicit failure code and reason. No import.failed event type exists
	// in the closed register (export.failed does — the store emits it for
	// export operations); the authoritative state endpoint is the source
	// of truth either way.
	FailKnowledgeOperation(ctx context.Context, opID, correlationID, failureCode, reason string) (knowledge.Operation, error)
	// CompleteKnowledgeExport CAS-publishes the artifact, then commits the
	// terminal completed state, the result, and the export.completed event
	// in ONE transaction.
	CompleteKnowledgeExport(ctx context.Context, commit knowledge.ExportCommit) (knowledge.Operation, error)
	// CancelKnowledgeOperation applies the cancellation ruling: unknown →
	// typed NOT_FOUND (no-leak); terminal → typed CONFLICT_STATE (the
	// Core-confirmed state decides — post-boundary operations are
	// non-cancellable); live → the Core-confirmed cancelled state with the
	// import.cancelled event (imports only — the register carries no
	// export.cancelled type).
	CancelKnowledgeOperation(ctx context.Context, opID, correlationID string) (knowledge.Operation, error)
	// LoadKnowledgeOperation resolves the authoritative state read; an
	// unknown operation is the typed NOT_FOUND (the no-leak rule).
	LoadKnowledgeOperation(ctx context.Context, opID string) (knowledge.Operation, error)
	// LoadKnowledgePack resolves one pack; unknown is the typed NOT_FOUND.
	LoadKnowledgePack(ctx context.Context, packID string) (knowledge.Pack, error)
	// LoadKnowledgeVersion resolves one immutable version; unknown pack or
	// unknown version is the SAME typed NOT_FOUND (the no-leak rule).
	LoadKnowledgeVersion(ctx context.Context, packID, versionID string) (knowledge.PackVersion, error)
	// KnowledgeEntries loads one version's entries in path order (the
	// export's structural input).
	KnowledgeEntries(ctx context.Context, packID, versionID string) ([]knowledge.EntryRecord, error)
	// ListKnowledgePacks reads one bounded keyset page.
	ListKnowledgePacks(ctx context.Context, q knowledge.ListQuery) (knowledge.PackPage, error)
	// ListKnowledgeVersions reads one bounded keyset page within one pack.
	ListKnowledgeVersions(ctx context.Context, packID string, q knowledge.ListQuery) (knowledge.VersionPage, error)
}

// Knowledge list bounds (the same class as the other read surfaces: the
// contract's page bound is 1000; the per-surface default is the slice's).
const (
	DefaultKnowledgeListLimit = 50
	MaxKnowledgeListLimit     = 1000
)

// Bounded label/identity lengths for the B1 request members (the coded
// bounds class: idempotency keys and identifiers ride the 128-char bound;
// display labels ride 512 — bounded, never unbounded).
const (
	maxKnowledgeIDLen    = 128
	maxKnowledgeLabelLen = 512
)

// knowledgeSourceSpec is one served registry member plus its validation
// class. The registry CONTENTS are Core-owned (the Core-authoritative
// supported-source law, UI §14.6): exactly the kinds this Core implements,
// never a speculative format or connector. Neither served kind fetches
// URLs, so no url_policy statement is served (the field is carried only
// where the kind fetches URLs).
type knowledgeSourceSpec struct {
	Spec  knowledge.ImportSourceSpec
	Class string // "document" | "archive"
}

// knowledgeSourceRegistry is the closed served registry (the implementation's
// typed values — Proposed pending the recorded owner review, the
// attachment-registry precedent). The document kind stores its bytes as the
// pack's text/plain entry; the archive kind expands every entry through the
// full archive-path/symlink/size defense (the C3-05 machinery).
var knowledgeSourceRegistry = []knowledgeSourceSpec{
	{
		Spec: knowledge.ImportSourceSpec{
			SourceKind: "file_text_document",
			MaxBytes:   knowledgepack.DefaultLimits().MaxFileBytes,
			Notes:      "One UTF-8 text document; NUL or non-UTF-8 bytes are rejected; the document becomes the pack's text/plain entry.",
		},
		Class: "document",
	},
	{
		Spec: knowledge.ImportSourceSpec{
			SourceKind: "file_zip_archive",
			// The archive bytes themselves are bounded well under the
			// transport's 32 MiB JSON-envelope body bound (base64-inflated).
			MaxBytes: 16 << 20,
			Notes:    "One ZIP archive; every entry passes the path-traversal, symlink, count, and per-entry size defense before acceptance.",
		},
		Class: "archive",
	},
}

func knowledgeSourceFor(kind string) (knowledgeSourceSpec, bool) {
	for _, spec := range knowledgeSourceRegistry {
		if spec.Spec.SourceKind == kind {
			return spec, true
		}
	}
	return knowledgeSourceSpec{}, false
}

// KnowledgeImportRequest is one import submit (the published
// KnowledgeImportRequest members as the service contract).
type KnowledgeImportRequest struct {
	IdempotencyKey            string
	SourceName                string
	SourceKind                string
	ContentB64                string
	DestinationPackID         string
	DuplicateMode             string
	ExternalTransferConfirmed bool
}

// KnowledgeSubmitResult is one submit's accepted outcome.
type KnowledgeSubmitResult struct {
	Operation knowledge.Operation
	Replayed  bool
}

// KnowledgeImportRegistry serves the supported-source registry (the
// Core-authoritative read class): the read creates nothing and probes
// nothing — the served contents ARE the canonical values.
func (s *Slice) KnowledgeImportRegistry(ctx context.Context) ([]knowledge.ImportSourceSpec, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return nil, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge}, security.ActionDataRead); err != nil {
		return nil, err
	}
	sources := make([]knowledge.ImportSourceSpec, 0, len(knowledgeSourceRegistry))
	for _, spec := range knowledgeSourceRegistry {
		sources = append(sources, spec.Spec)
	}
	return sources, nil
}

// SubmitKnowledgeImport submits one KnowledgePack import (the M-9
// digest-keyed class): authorize → the request-shape law → the destination
// identity resolution → the M-9/identity-create ruling (the store's) → the
// staged processing pipeline → the terminal state. 202 means accepted for
// processing, never completion — the authoritative operation-state read
// confirms every outcome. The submitted bytes are UNTRUSTED: every check on
// this path is Core-side and authoritative; client checks are early
// feedback only.
func (s *Slice) SubmitKnowledgeImport(ctx context.Context, req KnowledgeImportRequest) (KnowledgeSubmitResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge}, security.ActionDataWrite); err != nil {
		return KnowledgeSubmitResult{}, err
	}
	// The request-shape law: every shape refusal is the typed envelope
	// BEFORE any operation exists (nothing was accepted).
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "idempotency_key"})
	}
	if req.SourceName == "" || len(req.SourceName) > maxKnowledgeLabelLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "source_name"})
	}
	if req.SourceKind == "" || len(req.SourceKind) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "source_kind"})
	}
	if req.ContentB64 == "" {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content_b64"})
	}
	if len(req.DestinationPackID) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "destination_pack_id"})
	}
	// The duplicate-mode law (the register's own): displayed and accepted
	// ONLY where the Core defines the corresponding operation. This
	// implementation defines no separate duplicate-choice operation — the
	// digest-keyed M-9 replay is automatic — so any non-empty value is the
	// typed refusal.
	if req.DuplicateMode != "" {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "duplicate_mode", "reason": "the Core defines no duplicate-choice operation on this surface",
		})
	}
	content, err := base64.StdEncoding.DecodeString(req.ContentB64)
	if err != nil {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "content_b64"})
	}
	// The destination identity resolution precedes the submit effect: a
	// version-create on an unresolvable pack is the SAME typed 404 the
	// pack read serves (the no-leak rule), and the idempotency key stays
	// unconsumed by an unresolvable intent.
	if req.DestinationPackID != "" {
		if _, err := s.deps.Knowledge.LoadKnowledgePack(ctx, req.DestinationPackID); err != nil {
			return KnowledgeSubmitResult{}, err
		}
	}
	// The SOURCE DIGEST of the submitted bytes: the M-9 fingerprint's
	// digest member, in the algorithm-qualified value FORM
	// sha256:<lowercase-hex> (a value format; no algorithm approval).
	digest := KnowledgeDigestOf(content)
	intent := knowledge.ImportIntent{
		Operation: knowledge.Operation{
			OperationID:    "kop-" + randomID(),
			Kind:           knowledge.KindImport,
			State:          knowledge.StateSubmitted,
			CorrelationID:  corr,
			UntrustedLabel: knowledge.UntrustedLabel,
		},
		IdempotencyKey: req.IdempotencyKey,
		SourceDigest:   digest,
		CreateIntent:   knowledgeCreateIntent(req.DestinationPackID),
	}
	op, replayed, err := s.deps.Knowledge.SubmitKnowledgeImport(ctx, intent)
	if err != nil {
		return KnowledgeSubmitResult{}, err
	}
	if replayed {
		// The M-9 replay: the existing pack-version reference (through the
		// recorded operation) is returned while live or within the replay
		// window — never a second operation.
		return KnowledgeSubmitResult{Operation: op, Replayed: true}, nil
	}
	final, err := s.processKnowledgeImport(ctx, op, req, content)
	if err != nil {
		return KnowledgeSubmitResult{}, err
	}
	return KnowledgeSubmitResult{Operation: final, Replayed: false}, nil
}

// processKnowledgeImport runs the staged pipeline from submitted to a
// terminal state. Stage transitions commit through the journal machinery so
// the authoritative state endpoint shows the actual Core stages only; a
// stage without its commit renders as pending, never as assumed-complete.
// Internal storage failures fail the operation with the explicit INTERNAL
// code (a published register code) and a bounded safe reason — never a
// suppressed warning, never a stuck live operation.
func (s *Slice) processKnowledgeImport(ctx context.Context, op knowledge.Operation, req KnowledgeImportRequest, content []byte) (knowledge.Operation, error) {
	corr := op.CorrelationID
	// The declared kind resolves against the served registry: an unknown
	// kind is the typed IMPORT_TYPE_UNSUPPORTED rejection (the operation
	// lifecycle's rejection — the registry is the Core-authoritative list).
	sp, known := knowledgeSourceFor(req.SourceKind)
	if !known {
		return s.rejectKnowledgeImport(ctx, op, knowledge.FailImportTypeUnsupported)
	}
	// Uploading: the bytes are received (the envelope decode is complete).
	if _, err := s.deps.Knowledge.KnowledgeOperationStage(ctx, op.OperationID, corr, knowledge.StateUploading); err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	// Extracting: the untrusted bytes are expanded per the declared kind's
	// machinery — the full C3-05 defense set (allowlist, entry paths, dot-dot
	// and backslash traversal, symlink/device entries, per-entry and total
	// size bounds, entry-count bound, NUL/UTF-8 content checks) runs here,
	// BEFORE any parse or storage.
	lim := knowledgepack.DefaultLimits()
	var items []knowledgepack.Content
	switch sp.Class {
	case "document":
		item, err := knowledgepack.ImportDocument("document.txt", content, lim)
		if err != nil {
			return s.rejectKnowledgeImport(ctx, op, knowledgeRejectionCode(err))
		}
		items = []knowledgepack.Content{item}
	case "archive":
		lim.MaxTotalBytes = sp.Spec.MaxBytes
		expanded, err := knowledgepack.ImportArchive(content, lim)
		if err != nil {
			return s.rejectKnowledgeImport(ctx, op, knowledgeRejectionCode(err))
		}
		items = expanded
	default:
		return s.rejectKnowledgeImport(ctx, op, knowledge.FailImportTypeUnsupported)
	}
	// Validating: the accepted items become immutable entries with their
	// content digests; every entry path re-validates and every digest rides
	// the digest grammar (content identity relative to the compared bytes
	// only — never safety, authenticity, or truth).
	entries := make([]knowledge.EntryRecord, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		entry, err := knowledgepack.BuildEntry(item)
		if err != nil {
			return s.rejectKnowledgeImport(ctx, op, knowledge.FailImportMalformed)
		}
		if seen[item.Path] {
			return s.rejectKnowledgeImport(ctx, op, knowledge.FailImportMalformed)
		}
		seen[item.Path] = true
		entries = append(entries, knowledge.EntryRecord{
			Path:        entry.Path,
			ContentType: entry.ContentType,
			SizeBytes:   entry.SizeBytes,
			Digest:      entry.Digest,
			Content:     item.Data,
		})
	}
	if _, err := s.deps.Knowledge.KnowledgeOperationStage(ctx, op.OperationID, corr, knowledge.StateValidating); err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	// Indexing: the pack/version identities and the pack-root digest over
	// the normalized, deterministically ordered entry paths, media types,
	// and blob digests (the algorithm-qualified value FORM).
	root, err := knowledgeRootDigest(entries)
	if err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	if _, err := s.deps.Knowledge.KnowledgeOperationStage(ctx, op.OperationID, corr, knowledge.StateIndexing); err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	packID := req.DestinationPackID
	packIsNew := packID == ""
	if packIsNew {
		packID = "kpack-" + randomID()
	} else {
		existing, err := s.deps.Knowledge.LoadKnowledgePack(ctx, packID)
		if err != nil {
			return s.failKnowledgeOperation(ctx, op, err)
		}
		_ = existing
	}
	versionID := "kver-" + randomID()
	title := req.SourceName
	if !packIsNew {
		// The pack's display title is the pack's own; the submitted source
		// name rides the operation's provenance, never an edit of the pack.
		pack, err := s.deps.Knowledge.LoadKnowledgePack(ctx, packID)
		if err != nil {
			return s.failKnowledgeOperation(ctx, op, err)
		}
		title = pack.Title
	}
	now := s.deps.Now().UTC()
	commit := knowledge.ImportCommit{
		OperationID:   op.OperationID,
		CorrelationID: corr,
		Pack: knowledge.Pack{
			PackID:           packID,
			SchemaVersion:    knowledge.PackSchemaVersion,
			Title:            title,
			CreatedAt:        now.Format(timeRFC3339),
			UpdatedAt:        now.Format(timeRFC3339),
			CurrentVersionID: versionID,
		},
		PackIsNew: packIsNew,
		Version: knowledge.PackVersion{
			PackID:         packID,
			VersionID:      versionID,
			SchemaVersion:  knowledge.PackSchemaVersion,
			PackRootDigest: root,
			EntryCount:     len(entries),
			LicenseStatus:  "unknown",
			CreatedAt:      now.Format(timeRFC3339),
		},
		Entries: entries,
		Result: knowledge.OperationResult{
			Kind:      knowledge.KindImport,
			PackID:    packID,
			VersionID: versionID,
		},
	}
	final, err := s.deps.Knowledge.CompleteKnowledgeImport(ctx, commit)
	if err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	return final, nil
}

// rejectKnowledgeImport moves the operation to the terminal rejected state
// with the mapped register code. A rejection failure to commit is an
// internal failure (the operation must reach a terminal state — never a
// stuck live row).
func (s *Slice) rejectKnowledgeImport(ctx context.Context, op knowledge.Operation, failureCode string) (knowledge.Operation, error) {
	final, err := s.deps.Knowledge.RejectKnowledgeImport(ctx, op.OperationID, op.CorrelationID, failureCode, knowledgeRejectionReason(failureCode))
	if err != nil {
		return s.failKnowledgeOperation(ctx, op, err)
	}
	return final, nil
}

// failKnowledgeOperation is the fail-safe: an operation whose processing
// could not continue reaches the terminal failed state with the explicit
// INTERNAL code and a bounded safe reason (the MB §6.3 explicit-failure
// law — never a suppressed warning, never a stuck live operation).
func (s *Slice) failKnowledgeOperation(ctx context.Context, op knowledge.Operation, cause error) (knowledge.Operation, error) {
	final, ferr := s.deps.Knowledge.FailKnowledgeOperation(ctx, op.OperationID, op.CorrelationID, "INTERNAL", boundedReason(cause))
	if ferr != nil {
		return knowledge.Operation{}, ferr
	}
	return final, nil
}

// KnowledgeExportRequest is one export submit (the published
// KnowledgeExportRequest members as the service contract).
type KnowledgeExportRequest struct {
	CorrelationID             string
	PackID                    string
	VersionID                 string
	Destination               string
	ExternalTransferConfirmed bool
}

// SubmitKnowledgeExport submits one export generation (the read-with-output
// class): safe to re-issue, no durable effect to duplicate — NO idempotency
// layer and NO replay window. Secrets are excluded by default — the export
// artifact carries the pack/version/entries only; no secret value exists in
// pack storage to carry. Integrity is stated only as the artifact's own
// digest over the produced bytes — never a verification claim.
func (s *Slice) SubmitKnowledgeExport(ctx context.Context, req KnowledgeExportRequest) (KnowledgeSubmitResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge}, security.ActionDataWrite); err != nil {
		return KnowledgeSubmitResult{}, err
	}
	if req.CorrelationID == "" || len(req.CorrelationID) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "correlation_id"})
	}
	if req.PackID == "" || len(req.PackID) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "pack_id"})
	}
	if len(req.VersionID) > maxKnowledgeIDLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "version_id"})
	}
	if req.Destination == "" || len(req.Destination) > maxKnowledgeLabelLen {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "destination"})
	}
	// The exact identity resolution: the version resolves to the request's
	// version or the pack's current version; an unresolvable pack or an
	// unresolvable version is the SAME typed 404 (the no-leak rule).
	pack, err := s.deps.Knowledge.LoadKnowledgePack(ctx, req.PackID)
	if err != nil {
		return KnowledgeSubmitResult{}, err
	}
	versionID := req.VersionID
	if versionID == "" {
		versionID = pack.CurrentVersionID
	}
	if versionID == "" {
		return KnowledgeSubmitResult{}, envelopeErr(errs.CodeNotFound, corr, map[string]any{"reason": "no such knowledge pack version"})
	}
	version, err := s.deps.Knowledge.LoadKnowledgeVersion(ctx, req.PackID, versionID)
	if err != nil {
		return KnowledgeSubmitResult{}, err
	}
	op, err := s.deps.Knowledge.SubmitKnowledgeExport(ctx, knowledge.Operation{
		OperationID:   "kop-" + randomID(),
		Kind:          knowledge.KindExport,
		State:         knowledge.StateSubmitted,
		CorrelationID: req.CorrelationID,
	})
	if err != nil {
		return KnowledgeSubmitResult{}, err
	}
	// Generating: the export has no stage events (the closed register
	// carries import.stage_entered only); the state transition is the
	// actual Core stage, visible through the state endpoint.
	entries, err := s.deps.Knowledge.KnowledgeEntries(ctx, req.PackID, versionID)
	if err != nil {
		return s.exportFailed(ctx, op, err)
	}
	artifact, err := knowledgeExportArtifact(pack, version, entries)
	if err != nil {
		return s.exportFailed(ctx, op, err)
	}
	digest := KnowledgeDigestOf(artifact)
	final, err := s.deps.Knowledge.CompleteKnowledgeExport(ctx, knowledge.ExportCommit{
		OperationID:   op.OperationID,
		CorrelationID: op.CorrelationID,
		Result: knowledge.OperationResult{
			Kind:      knowledge.KindExport,
			ExportRef: digest,
			Digest:    digest,
		},
		Artifact: artifact,
	})
	if err != nil {
		return s.exportFailed(ctx, op, err)
	}
	return KnowledgeSubmitResult{Operation: final, Replayed: false}, nil
}

// exportFailed is the export pipeline's fail-safe: the operation reaches
// the terminal failed state and the 202 body carries it (the authoritative
// state read confirms every outcome — never a stuck live operation).
func (s *Slice) exportFailed(ctx context.Context, op knowledge.Operation, cause error) (KnowledgeSubmitResult, error) {
	final, ferr := s.failKnowledgeOperation(ctx, op, cause)
	if ferr != nil {
		return KnowledgeSubmitResult{}, ferr
	}
	return KnowledgeSubmitResult{Operation: final, Replayed: false}, nil
}

// KnowledgeOperationStatus is the family's authoritative state endpoint (a
// pure query — never a retry): resolved by the operation identity; a lost
// response is NEVER proof of success, cancellation, or safe retry —
// reconcile by the operation ID through this read BEFORE any retry.
func (s *Slice) KnowledgeOperationStatus(ctx context.Context, opID string) (knowledge.Operation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return knowledge.Operation{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge, ID: opID}, security.ActionDataRead); err != nil {
		return knowledge.Operation{}, err
	}
	if opID == "" || len(opID) > maxKnowledgeIDLen {
		return knowledge.Operation{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "operation_id"})
	}
	return s.deps.Knowledge.LoadKnowledgeOperation(ctx, opID)
}

// CancelKnowledgeOperation requests one knowledge operation's cancellation:
// a Core-confirmed operation with its own state — never the pre-submit
// queue removal (a local act with NO Core meaning) and never displayed as
// the confirmation (the J-5 rule): the authoritative operation-state read
// confirms the terminal cancelled state.
func (s *Slice) CancelKnowledgeOperation(ctx context.Context, opID string) (knowledge.Operation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return knowledge.Operation{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge, ID: opID}, security.ActionDataWrite); err != nil {
		return knowledge.Operation{}, err
	}
	if opID == "" || len(opID) > maxKnowledgeIDLen {
		return knowledge.Operation{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "operation_id"})
	}
	return s.deps.Knowledge.CancelKnowledgeOperation(ctx, opID, corr)
}

// KnowledgePack reads one pack's authoritative state.
func (s *Slice) KnowledgePack(ctx context.Context, packID string) (knowledge.Pack, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return knowledge.Pack{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge, ID: packID}, security.ActionDataRead); err != nil {
		return knowledge.Pack{}, err
	}
	if packID == "" || len(packID) > maxKnowledgeIDLen {
		return knowledge.Pack{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "pack_id"})
	}
	return s.deps.Knowledge.LoadKnowledgePack(ctx, packID)
}

// KnowledgePackVersion reads one immutable pack version. An unknown pack
// and an unknown version are the SAME typed 404 (the no-leak rule).
func (s *Slice) KnowledgePackVersion(ctx context.Context, packID, versionID string) (knowledge.PackVersion, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return knowledge.PackVersion{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge, ID: packID}, security.ActionDataRead); err != nil {
		return knowledge.PackVersion{}, err
	}
	if packID == "" || len(packID) > maxKnowledgeIDLen {
		return knowledge.PackVersion{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "pack_id"})
	}
	if versionID == "" || len(versionID) > maxKnowledgeIDLen {
		return knowledge.PackVersion{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "version_id"})
	}
	return s.deps.Knowledge.LoadKnowledgeVersion(ctx, packID, versionID)
}

// KnowledgePacks reads the workspace-scoped pack list (the keyset list
// semantics; the cursor's absence marks the final page).
func (s *Slice) KnowledgePacks(ctx context.Context, req KnowledgeListRequest) (KnowledgePackListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return KnowledgePackListResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge}, security.ActionDataRead); err != nil {
		return KnowledgePackListResult{}, err
	}
	limit, err := knowledgeListLimit(req.Limit, corr)
	if err != nil {
		return KnowledgePackListResult{}, err
	}
	q := knowledge.ListQuery{Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeSaltedCursorEnvelope(req.Cursor)
		if err != nil {
			return KnowledgePackListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		var p knowledgeCursorPayload
		if err := json.Unmarshal(pos, &p); err != nil || p.CreatedAt == "" || p.ID == "" {
			return KnowledgePackListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterID = p.CreatedAt, p.ID
	}
	page, err := s.deps.Knowledge.ListKnowledgePacks(ctx, q)
	if err != nil {
		return KnowledgePackListResult{}, err
	}
	res := KnowledgePackListResult{Packs: page.Packs}
	if page.More {
		next, cerr := encodeKnowledgeCursor(page.Packs[len(page.Packs)-1].CreatedAt, page.Packs[len(page.Packs)-1].PackID)
		if cerr != nil {
			return KnowledgePackListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.Cursor = next
	}
	return res, nil
}

// KnowledgeVersions reads one pack's version list; an unknown pack is the
// SAME typed 404 as the pack read (the no-leak rule).
func (s *Slice) KnowledgeVersions(ctx context.Context, packID string, req KnowledgeListRequest) (KnowledgeVersionListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Knowledge == nil {
		return KnowledgeVersionListResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "knowledge store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceKnowledge, ID: packID}, security.ActionDataRead); err != nil {
		return KnowledgeVersionListResult{}, err
	}
	if packID == "" || len(packID) > maxKnowledgeIDLen {
		return KnowledgeVersionListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "pack_id"})
	}
	// The pack identity resolves first: the versions list of an unknown
	// pack is the SAME typed 404 as the pack read (the no-leak rule).
	if _, err := s.deps.Knowledge.LoadKnowledgePack(ctx, packID); err != nil {
		return KnowledgeVersionListResult{}, err
	}
	limit, err := knowledgeListLimit(req.Limit, corr)
	if err != nil {
		return KnowledgeVersionListResult{}, err
	}
	q := knowledge.ListQuery{Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeSaltedCursorEnvelope(req.Cursor)
		if err != nil {
			return KnowledgeVersionListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		var p knowledgeCursorPayload
		if err := json.Unmarshal(pos, &p); err != nil || p.CreatedAt == "" || p.ID == "" {
			return KnowledgeVersionListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterID = p.CreatedAt, p.ID
	}
	page, err := s.deps.Knowledge.ListKnowledgeVersions(ctx, packID, q)
	if err != nil {
		return KnowledgeVersionListResult{}, err
	}
	res := KnowledgeVersionListResult{Versions: page.Versions}
	if page.More {
		last := page.Versions[len(page.Versions)-1]
		next, cerr := encodeKnowledgeCursor(last.CreatedAt, last.VersionID)
		if cerr != nil {
			return KnowledgeVersionListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.Cursor = next
	}
	return res, nil
}

// KnowledgeListRequest is one bounded list request.
type KnowledgeListRequest struct {
	Limit  int
	Cursor string
}

// KnowledgePackListResult is one bounded pack page.
type KnowledgePackListResult struct {
	Packs  []knowledge.Pack
	Cursor string
}

// KnowledgeVersionListResult is one bounded version page.
type KnowledgeVersionListResult struct {
	Versions []knowledge.PackVersion
	Cursor   string
}

func knowledgeListLimit(limit int, corr string) (int, error) {
	if limit == 0 {
		limit = DefaultKnowledgeListLimit
	}
	if limit < 1 || limit > MaxKnowledgeListLimit {
		return 0, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	return limit, nil
}

// knowledgeCursorPayload is the decoded keyset position behind the opaque
// list cursors (the salted-cursor PATTERN as a separate query-side
// mechanism — the event family's cursor is untouched).
type knowledgeCursorPayload struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeKnowledgeCursor(createdAt, id string) (string, error) {
	if createdAt == "" || id == "" {
		return "", fmt.Errorf("app: knowledge cursor position lacks its keyset values")
	}
	b, err := json.Marshal(knowledgeCursorPayload{CreatedAt: createdAt, ID: id})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

// knowledgeCreateIntent encodes the identity-create intent (the
// duplicate-create layer's fingerprint class member).
func knowledgeCreateIntent(destinationPackID string) string {
	if destinationPackID == "" {
		return knowledge.CreateIntentPack
	}
	return knowledge.CreateIntentVersionPrefix + destinationPackID
}

// KnowledgeDigestOf is the B1 content digest: the algorithm-qualified value
// FORM sha256:<lowercase-hex> over the exact bytes (a value format; no
// algorithm approval is implied — the D-41 note rides the contract
// descriptions, not this helper's carriage).
func KnowledgeDigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// knowledgeRootDigest computes the pack-root digest over the normalized,
// deterministically ordered entry paths, media types, and blob digests:
// canonical JSON over the path-sorted {content_type, digest, path} list,
// in the algorithm-qualified value FORM.
func knowledgeRootDigest(entries []knowledge.EntryRecord) (string, error) {
	type rootEntry struct {
		ContentType string `json:"content_type"`
		Digest      string `json:"digest"`
		Path        string `json:"path"`
	}
	list := make([]rootEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, rootEntry{ContentType: e.ContentType, Digest: e.Digest, Path: e.Path})
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Path > list[i].Path {
			return "", fmt.Errorf("app: knowledge root digest requires path-sorted entries")
		}
	}
	canonical, err := events.CanonicalJSON(list)
	if err != nil {
		return "", err
	}
	return KnowledgeDigestOf(canonical), nil
}

// knowledgeExportArtifact builds the export artifact: the canonical JSON
// document of the pack, the version, and the ordered entries WITH their
// content — the exact scope the submit named. SECRETS ARE EXCLUDED BY
// DEFAULT — pack storage carries no secret values, so the artifact
// structurally cannot carry one. The digest over the produced bytes is
// content identity only — never a verification claim.
func knowledgeExportArtifact(pack knowledge.Pack, version knowledge.PackVersion, entries []knowledge.EntryRecord) ([]byte, error) {
	type artifactEntry struct {
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		Digest      string `json:"digest"`
		Path        string `json:"path"`
		SizeBytes   int64  `json:"size_bytes"`
	}
	doc := struct {
		Pack    knowledge.Pack        `json:"pack"`
		Version knowledge.PackVersion `json:"version"`
		Entries []artifactEntry       `json:"entries"`
	}{Pack: pack, Version: version, Entries: make([]artifactEntry, 0, len(entries))}
	for _, e := range entries {
		doc.Entries = append(doc.Entries, artifactEntry{
			Content:     string(e.Content),
			ContentType: e.ContentType,
			Digest:      e.Digest,
			Path:        e.Path,
			SizeBytes:   e.SizeBytes,
		})
	}
	return events.CanonicalJSON(doc)
}

// knowledgeRejectionCode maps the import machinery's quarantine reasons onto
// the published KnowledgeErrorCode register (each a closed-register code —
// never invented). The consistent rule: bytes that do not form a valid
// instance of the declared kind are IMPORT_MALFORMED; a declared kind
// outside the served registry is IMPORT_TYPE_UNSUPPORTED; declared-kind
// instances that violate a policy bound are the policy codes.
func knowledgeRejectionCode(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not a readable zip archive"):
		return knowledge.FailImportMalformed
	case strings.Contains(msg, "unsupported_format"):
		return knowledge.FailImportTypeUnsupported
	case strings.Contains(msg, "oversize"):
		return knowledge.FailImportSizeOverLimit
	case strings.Contains(msg, "too_many_entries"):
		return knowledge.FailImportPolicyBlocked
	case strings.Contains(msg, "path_traversal"), strings.Contains(msg, "symlink_entry"):
		return knowledge.FailImportPolicyBlocked
	case strings.Contains(msg, "bad_entry_path"):
		return knowledge.FailImportMalformed
	case strings.Contains(msg, "binary_content"):
		return knowledge.FailImportMalformed
	default:
		return knowledge.FailImportMalformed
	}
}

// knowledgeRejectionReason is the bounded safe reason per register code
// (the honest statement — never a suppressed warning, never payload echo).
func knowledgeRejectionReason(code string) string {
	switch code {
	case knowledge.FailImportTypeUnsupported:
		return "the submitted content is not a supported type for the declared source kind"
	case knowledge.FailImportSizeOverLimit:
		return "the submitted content exceeds the served source kind's size bound"
	case knowledge.FailImportPolicyBlocked:
		return "the submitted content violated an import policy bound (archive paths, entry count, or traversal defense)"
	case knowledge.FailImportMalformed:
		return "the submitted content is malformed relative to the declared source kind"
	default:
		return "the import was rejected"
	}
}

// boundedReason reduces an error to one bounded machine-safe line for the
// failed state's reason (never raw internals, never payload echo).
func boundedReason(err error) string {
	if err == nil {
		return "the operation failed"
	}
	s := err.Error()
	if len(s) > 256 {
		s = s[:256]
	}
	return s
}

// timeRFC3339 is the durable record's timestamp format.
const timeRFC3339 = "2006-01-02T15:04:05.000000000Z07:00"
