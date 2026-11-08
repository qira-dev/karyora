package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/knowledge"
)

// newKnowledgeEnv is newTestEnv with the knowledge port wired to the same
// store (the test-side twin of the composition root's wiring).
func newKnowledgeEnv(t testing.TB) (*testEnv, *Slice) {
	t.Helper()
	env := newTestEnv(t)
	kdeps := env.deps
	kdeps.Knowledge = env.store
	kslice, err := NewSlice(kdeps)
	if err != nil {
		t.Fatalf("knowledge slice: %v", err)
	}
	return env, kslice
}

// knowledgeDocBody builds one text-document import request body's parts.
func knowledgeDocB64(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}

// knowledgeZipB64 builds a zip archive's base64 over (name, content) pairs.
func knowledgeZipB64(t *testing.T, entries ...[2]string) string {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, e := range entries {
		f, err := w.Create(e[0])
		if err != nil {
			t.Fatalf("zip entry: %v", err)
		}
		if _, err := f.Write([]byte(e[1])); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// assertKnowledgeEnvelope converts one envelope-carrying error (the
// app-layer EnvelopeError or the storage boundary's carrier) and checks
// its code — the same dual mapping the transport's writeError applies.
func assertKnowledgeEnvelope(t *testing.T, err error, want errs.Code) errs.Envelope {
	t.Helper()
	if err == nil {
		t.Fatalf("expected the typed envelope error %s, got nil", want)
	}
	var env errs.Envelope
	if ee, ok := err.(*EnvelopeError); ok {
		env = ee.Envelope
	} else if carrier, ok := err.(interface {
		Envelope(correlationID string) (errs.Envelope, error)
	}); ok {
		built, cerr := carrier.Envelope(errs.NewCorrelationID())
		if cerr != nil {
			t.Fatalf("envelope construction: %v", cerr)
		}
		env = built
	} else {
		t.Fatalf("expected the typed envelope error %s, got %T: %v", want, err, err)
	}
	if env.Code != want {
		t.Fatalf("expected %s, got %s (%+v)", want, env.Code, env.Data)
	}
	return env
}

// TestKnowledgeAuthzBoundary pins the authorization boundary: every B1
// surface refuses the session-less caller with the typed
// AUTHZ_DENIED/SESSION_INVALID envelope through the one decision port —
// and never the data.
func TestKnowledgeAuthzBoundary(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	noSession := WithCaller(context.Background(), "local", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"registry", func() error { _, err := slice.KnowledgeImportRegistry(noSession); return err }},
		{"pack list", func() error { _, err := slice.KnowledgePacks(noSession, KnowledgeListRequest{}); return err }},
		{"pack read", func() error { _, err := slice.KnowledgePack(noSession, "kpack-x"); return err }},
		{"version read", func() error { _, err := slice.KnowledgePackVersion(noSession, "kpack-x", "kver-x"); return err }},
		{"import submit", func() error {
			_, err := slice.SubmitKnowledgeImport(noSession, KnowledgeImportRequest{
				IdempotencyKey: "k", SourceName: "n", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("x"),
			})
			return err
		}},
		{"export submit", func() error {
			_, err := slice.SubmitKnowledgeExport(noSession, KnowledgeExportRequest{CorrelationID: "c", PackID: "kpack-x", Destination: "d"})
			return err
		}},
		{"state read", func() error { _, err := slice.KnowledgeOperationStatus(noSession, "kop-x"); return err }},
		{"cancel", func() error { _, err := slice.CancelKnowledgeOperation(noSession, "kop-x"); return err }},
	}
	for _, tc := range cases {
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: the session-less caller must be refused", tc.name)
		}
		env := assertKnowledgeEnvelope(t, err, errs.CodeAuthzDenied)
		if env.Data["reason"] != "SESSION_INVALID" {
			t.Fatalf("%s: the denial must carry SESSION_INVALID, got %+v", tc.name, env.Data)
		}
	}
	// An authenticated, well-formed call passes the boundary (the
	// registry read is the cheapest positive).
	if _, err := slice.KnowledgeImportRegistry(env.ctx()); err != nil {
		t.Fatalf("authenticated registry read: %v", err)
	}
}

// TestKnowledgeImportRegistry pins the served registry: the Core's own
// supported kinds as queryable data — the file kinds this Core implements,
// each with a positive bound, and NO url-fetching kind (no url_policy is
// served where no kind fetches URLs).
func TestKnowledgeImportRegistry(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	sources, err := slice.KnowledgeImportRegistry(env.ctx())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("the served registry must carry the implemented kinds")
	}
	seen := map[string]int64{}
	for _, s := range sources {
		if s.SourceKind == "" || s.MaxBytes <= 0 {
			t.Fatalf("every served kind carries a positive bound: %+v", s)
		}
		if s.URLPolicy != "" {
			t.Fatalf("no served kind fetches URLs; no url_policy may be served: %+v", s)
		}
		seen[s.SourceKind] = s.MaxBytes
	}
	if _, ok := seen["file_text_document"]; !ok {
		t.Fatalf("the text-document kind must be served: %v", seen)
	}
	if _, ok := seen["file_zip_archive"]; !ok {
		t.Fatalf("the zip-archive kind must be served: %v", seen)
	}
}

// TestKnowledgeImportValidation pins the request-shape law: every shape
// refusal is the typed VALIDATION_FAILED envelope BEFORE any operation
// exists; the duplicate_mode law refuses any non-empty value (the Core
// defines no duplicate-choice operation on this surface).
func TestKnowledgeImportValidation(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	base := KnowledgeImportRequest{
		IdempotencyKey: "k", SourceName: "n", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("ok"),
	}
	cases := []struct {
		name  string
		mutat func(*KnowledgeImportRequest)
		field string
	}{
		{"missing key", func(r *KnowledgeImportRequest) { r.IdempotencyKey = "" }, "idempotency_key"},
		{"long key", func(r *KnowledgeImportRequest) { r.IdempotencyKey = strings.Repeat("k", 129) }, "idempotency_key"},
		{"missing name", func(r *KnowledgeImportRequest) { r.SourceName = "" }, "source_name"},
		{"missing kind", func(r *KnowledgeImportRequest) { r.SourceKind = "" }, "source_kind"},
		{"missing content", func(r *KnowledgeImportRequest) { r.ContentB64 = "" }, "content_b64"},
		{"bad base64", func(r *KnowledgeImportRequest) { r.ContentB64 = "!!!" }, "content_b64"},
		{"long destination", func(r *KnowledgeImportRequest) { r.DestinationPackID = strings.Repeat("p", 129) }, "destination_pack_id"},
	}
	for _, tc := range cases {
		req := base
		tc.mutat(&req)
		_, err := slice.SubmitKnowledgeImport(env.ctx(), req)
		env := assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
		if env.Data["field"] != tc.field {
			t.Fatalf("%s: the refusal must name its field (%s), got %+v", tc.name, tc.field, env.Data)
		}
	}
	// The duplicate-mode law.
	req := base
	req.DuplicateMode = knowledge.DuplicateDeduplication
	_, err := slice.SubmitKnowledgeImport(env.ctx(), req)
	envl := assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	if envl.Data["field"] != "duplicate_mode" {
		t.Fatalf("the duplicate-mode refusal must name its field: %+v", envl.Data)
	}
}

// TestKnowledgeImportServicePipeline drives the full import pipeline: the
// staged operation lands completed with the pack-version reference; the
// pack/version/entries are readable; the M-9 replay returns the SAME
// operation with replayed=true; the same key with different bytes refuses.
func TestKnowledgeImportServicePipeline(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	res, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "pipe-1", SourceName: "fixture doc", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("the pipeline content"),
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if res.Replayed {
		t.Fatal("a fresh submit is never a replay")
	}
	op := res.Operation
	if op.State != knowledge.StateCompleted || op.Kind != knowledge.KindImport || op.UntrustedLabel != knowledge.UntrustedLabel {
		t.Fatalf("the pipeline must complete: %+v", op)
	}
	if op.Result == nil || op.Result.PackID == "" || op.Result.VersionID == "" {
		t.Fatalf("the completed operation carries the pack-version reference: %+v", op)
	}
	pack, err := slice.KnowledgePack(ctx, op.Result.PackID)
	if err != nil || pack.Title != "fixture doc" || pack.CurrentVersionID != op.Result.VersionID {
		t.Fatalf("pack read: %+v err=%v", pack, err)
	}
	version, err := slice.KnowledgePackVersion(ctx, op.Result.PackID, op.Result.VersionID)
	if err != nil || version.EntryCount != 1 || !strings.HasPrefix(version.PackRootDigest, "sha256:") {
		t.Fatalf("version read: %+v err=%v", version, err)
	}
	// The M-9 replay: same key + same bytes → the SAME operation,
	// replayed=true.
	replay, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "pipe-1", SourceName: "fixture doc", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("the pipeline content"),
	})
	if err != nil || !replay.Replayed || replay.Operation.OperationID != op.OperationID {
		t.Fatalf("M-9 replay: %+v err=%v", replay, err)
	}
	// Same key + different bytes → the M-9 conflict with
	// fingerprintMatch:false.
	_, err = slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "pipe-1", SourceName: "fixture doc", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("DIFFERENT bytes"),
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeConflictIdempotencyKey)
	if envl.Data["fingerprintMatch"] != false {
		t.Fatalf("the M-9 conflict carries fingerprintMatch:false: %+v", envl.Data)
	}
}

// TestKnowledgeImportRejections pins the untrusted-import lifecycle
// rejections: the typed register codes surface through the operation's
// rejected state (each its own explicit failure — never a suppressed
// warning).
func TestKnowledgeImportRejections(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	cases := []struct {
		name     string
		kind     string
		b64      func(t *testing.T) string
		wantCode string
	}{
		{"unknown kind", "file_speculative", func(t *testing.T) string { return knowledgeDocB64("x") }, knowledge.FailImportTypeUnsupported},
		{"binary content", "file_text_document", func(t *testing.T) string {
			return base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02})
		}, knowledge.FailImportMalformed},
		{"not a zip", "file_zip_archive", func(t *testing.T) string { return knowledgeDocB64("plain text, not an archive") }, knowledge.FailImportMalformed},
		{"traversal entry", "file_zip_archive", func(t *testing.T) string {
			return knowledgeZipB64(t, [2]string{"doc.txt", "ok"}, [2]string{"../evil.txt", "escape"})
		}, knowledge.FailImportPolicyBlocked},
	}
	for i, tc := range cases {
		res, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
			IdempotencyKey: "rej-" + string(rune('a'+i)), SourceName: "rejected doc", SourceKind: tc.kind, ContentB64: tc.b64(t),
		})
		if err != nil {
			t.Fatalf("%s: submit: %v", tc.name, err)
		}
		// 202 accepted; the authoritative state read confirms the rejection.
		op := res.Operation
		if op.State != knowledge.StateRejected {
			t.Fatalf("%s: the operation must reach rejected, got %s (%+v)", tc.name, op.State, op)
		}
		if op.FailureCode != tc.wantCode || op.FailureReason == "" {
			t.Fatalf("%s: the rejection must carry its register code (%s) and reason, got %+v", tc.name, tc.wantCode, op)
		}
		// The state endpoint is the authoritative confirmation.
		confirmed, err := slice.KnowledgeOperationStatus(ctx, op.OperationID)
		if err != nil || confirmed.State != knowledge.StateRejected || confirmed.FailureCode != tc.wantCode {
			t.Fatalf("%s: the state read must confirm the rejection: %+v err=%v", tc.name, confirmed, err)
		}
	}
}

// TestKnowledgeImportDestination404 pins the destination identity law: a
// version-create on an unresolvable pack is the SAME typed 404 the pack
// read serves, before any operation exists.
func TestKnowledgeImportDestination404(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	_, packErr := slice.KnowledgePack(ctx, "kpack-nope")
	packEnv := assertKnowledgeEnvelope(t, packErr, errs.CodeNotFound)
	_, impErr := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "dest-1", SourceName: "n", SourceKind: "file_text_document",
		ContentB64: knowledgeDocB64("x"), DestinationPackID: "kpack-nope",
	})
	impEnv := assertKnowledgeEnvelope(t, impErr, errs.CodeNotFound)
	if packEnv.Code != impEnv.Code {
		t.Fatal("the unknown destination and the unknown pack read are the SAME typed 404")
	}
}

// TestKnowledgeExportService drives the export class: none-required
// idempotency (safe re-issue mints a new operation), the completed state
// with the artifact reference, and the typed refusals.
func TestKnowledgeExportService(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	imp, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "exp-src", SourceName: "export fixture", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("exportable content"),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	packID := imp.Operation.Result.PackID
	res, err := slice.SubmitKnowledgeExport(ctx, KnowledgeExportRequest{
		CorrelationID: "export-corr-1", PackID: packID, Destination: "local workspace export",
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.Replayed {
		t.Fatal("the export class carries no replay — safe re-issue")
	}
	op := res.Operation
	if op.State != knowledge.StateCompleted || op.Result == nil || op.Result.ExportRef == "" || !strings.HasPrefix(op.Result.Digest, "sha256:") {
		t.Fatalf("the export must complete with its artifact reference: %+v", op)
	}
	// Safe re-issue: a second submit mints a NEW operation (none-required).
	again, err := slice.SubmitKnowledgeExport(ctx, KnowledgeExportRequest{
		CorrelationID: "export-corr-2", PackID: packID, Destination: "local workspace export",
	})
	if err != nil || again.Operation.OperationID == op.OperationID {
		t.Fatalf("the export re-issue must mint a new operation: %+v err=%v", again, err)
	}
	// Unknown pack: the typed 404 (the no-leak rule).
	_, err = slice.SubmitKnowledgeExport(ctx, KnowledgeExportRequest{CorrelationID: "c2", PackID: "kpack-nope", Destination: "d"})
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	// Missing destination: the typed field refusal.
	_, err = slice.SubmitKnowledgeExport(ctx, KnowledgeExportRequest{CorrelationID: "c3", PackID: packID})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	if envl.Data["field"] != "destination" {
		t.Fatalf("the destination refusal must name its field: %+v", envl.Data)
	}
}

// TestKnowledgeCancelService pins the service-level cancellation rulings:
// unknown → the typed 404; terminal → the typed CONFLICT_STATE (the
// Core-confirmed state decides — post-boundary operations are
// non-cancellable; the request is never the confirmation).
func TestKnowledgeCancelService(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	_, err := slice.CancelKnowledgeOperation(ctx, "kop-nope")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	imp, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
		IdempotencyKey: "cancel-src", SourceName: "n", SourceKind: "file_text_document", ContentB64: knowledgeDocB64("done"),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	_, err = slice.CancelKnowledgeOperation(ctx, imp.Operation.OperationID)
	envl := assertKnowledgeEnvelope(t, err, errs.CodeConflictState)
	if envl.Data["reason"] == "" {
		t.Fatalf("the refusal carries its bounded reason: %+v", envl.Data)
	}
}

// TestKnowledgeListsService pins the list reads: the keyset pages, the
// cursor round trip, and the versions list of an unknown pack being the
// SAME typed 404.
func TestKnowledgeListsService(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	for i := 0; i < 3; i++ {
		// Distinct bytes per import: the identity-create layer would
		// replay a same-digest create intent inside its window.
		if _, err := slice.SubmitKnowledgeImport(ctx, KnowledgeImportRequest{
			IdempotencyKey: "list-" + string(rune('a'+i)), SourceName: "list doc " + string(rune('a'+i)),
			SourceKind: "file_text_document", ContentB64: knowledgeDocB64("content " + string(rune('a'+i))),
		}); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}
	page, err := slice.KnowledgePacks(ctx, KnowledgeListRequest{Limit: 2})
	if err != nil || len(page.Packs) != 2 || page.Cursor == "" {
		t.Fatalf("page 1: %+v err=%v", page, err)
	}
	page2, err := slice.KnowledgePacks(ctx, KnowledgeListRequest{Limit: 2, Cursor: page.Cursor})
	if err != nil || len(page2.Packs) != 1 || page2.Cursor != "" {
		t.Fatalf("page 2 (final): %+v err=%v", page2, err)
	}
	// The versions list of a known pack.
	first := page2.Packs[0]
	versions, err := slice.KnowledgeVersions(ctx, first.PackID, KnowledgeListRequest{})
	if err != nil || len(versions.Versions) != 1 {
		t.Fatalf("versions: %+v err=%v", versions, err)
	}
	// The versions list of an unknown pack: the SAME typed 404.
	_, err = slice.KnowledgeVersions(ctx, "kpack-nope", KnowledgeListRequest{})
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	// A malformed cursor: the typed field refusal.
	_, err = slice.KnowledgePacks(ctx, KnowledgeListRequest{Cursor: "not-a-cursor"})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	if envl.Data["field"] != "cursor" {
		t.Fatalf("the cursor refusal names its field: %+v", envl.Data)
	}
}

// TestKnowledgeOperationStatus404 pins the state endpoint's no-leak rule:
// an unknown operation is the typed NOT_FOUND, the same envelope whatever
// the caller names.
func TestKnowledgeOperationStatus404(t *testing.T) {
	env, slice := newKnowledgeEnv(t)
	ctx := env.ctx()
	_, err := slice.KnowledgeOperationStatus(ctx, "kop-nope")
	envl := assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	b, _ := json.Marshal(envl.Data)
	if !strings.Contains(string(b), "no such knowledge operation") {
		t.Fatalf("the no-leak reason rides the envelope: %s", b)
	}
}
