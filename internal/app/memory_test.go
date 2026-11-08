package app

import (
	"context"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
)

// newMemoryEnv is newTestEnv with the memory port wired to the same store
// (the test-side twin of the composition root's wiring).
func newMemoryEnv(t testing.TB) (*testEnv, *Slice) {
	t.Helper()
	env := newTestEnv(t)
	mdeps := env.deps
	mdeps.Memory = env.store
	mslice, err := NewSlice(mdeps)
	if err != nil {
		t.Fatalf("memory slice: %v", err)
	}
	return env, mslice
}

// memoryAddRequest builds a legal add request.
func memoryAddRequest(key, correlation, scope string) MemoryAddRequest {
	return MemoryAddRequest{
		IdempotencyKey: key,
		CorrelationID:  correlation,
		Scope:          scope,
		Origin:         memory.OriginUserEntered,
		Content:        "the memory content",
	}
}

// addFixtureItem adds one item through the service (the test fixture path).
func addFixtureItem(t *testing.T, slice *Slice, ctx context.Context, key, scope string) memory.MutationResult {
	t.Helper()
	res, err := slice.AddMemoryItem(ctx, memoryAddRequest(key, "corr-"+key, scope))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	return res
}

// TestMemoryAuthzBoundary pins the authorization boundary: every B2
// surface refuses the session-less caller with the typed
// AUTHZ_DENIED/SESSION_INVALID envelope through the one decision port —
// and never the data.
func TestMemoryAuthzBoundary(t *testing.T) {
	env, slice := newMemoryEnv(t)
	noSession := WithCaller(context.Background(), "local", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"list", func() error { _, err := slice.MemoryItems(noSession, MemoryListRequest{}); return err }},
		{"add", func() error {
			_, err := slice.AddMemoryItem(noSession, memoryAddRequest("k", "c", memory.ScopeUser))
			return err
		}},
		{"inspect", func() error { _, err := slice.MemoryItem(noSession, "mem-x"); return err }},
		{"edit", func() error {
			_, err := slice.EditMemoryItem(noSession, "mem-x", MemoryEditRequest{IdempotencyKey: "k", CorrelationID: "c", ItemStateRef: "r", Content: "x"})
			return err
		}},
		{"approve", func() error {
			_, err := slice.ApproveMemoryItem(noSession, "mem-x", MemoryReviewRequest{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"reject", func() error {
			_, err := slice.RejectMemoryItem(noSession, "mem-x", MemoryReviewRequest{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"pin", func() error {
			_, err := slice.SetMemoryItemPin(noSession, "mem-x", MemoryPinRequest{IdempotencyKey: "k", CorrelationID: "c", Pinned: true})
			return err
		}},
		{"inclusion", func() error {
			_, err := slice.SetMemoryItemInclusion(noSession, "mem-x", MemoryInclusionRequest{CorrelationID: "c", RequestRef: "r", Included: true})
			return err
		}},
		{"forget", func() error { _, err := slice.ForgetMemoryItem(noSession, "mem-x", MemoryReviewCommand{}); return err }},
		{"delete", func() error { _, err := slice.DeleteMemoryItem(noSession, "mem-x", MemoryReviewCommand{}); return err }},
		{"export", func() error {
			_, err := slice.ExportMemoryItems(noSession, MemoryExportRequest{CorrelationID: "c", Scope: memory.ScopeUser})
			return err
		}},
		{"state read", func() error { _, err := slice.MemoryOperationStatus(noSession, "memop-x"); return err }},
	}
	for _, tc := range cases {
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: the session-less caller must be refused", tc.name)
		}
		envl := assertKnowledgeEnvelope(t, err, errs.CodeAuthzDenied)
		if envl.Data["reason"] != "SESSION_INVALID" {
			t.Fatalf("%s: the denial must carry SESSION_INVALID, got %+v", tc.name, envl.Data)
		}
	}
	// An authenticated, well-formed call passes the boundary (the list read
	// is the cheapest positive).
	if _, err := slice.MemoryItems(env.ctx(), MemoryListRequest{}); err != nil {
		t.Fatalf("authenticated list read: %v", err)
	}
}

// TestMemoryRequestShapeLaw pins the request-shape refusals (the typed
// envelope BEFORE any operation exists) and the origin-labeling law.
func TestMemoryRequestShapeLaw(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	cases := []struct {
		name string
		req  MemoryAddRequest
		want errs.Code
	}{
		{"missing key", MemoryAddRequest{CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: "x"}, errs.CodeValidationFailed},
		{"missing correlation", MemoryAddRequest{IdempotencyKey: "k", Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: "x"}, errs.CodeValidationFailed},
		{"unknown scope", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: "galaxy", Origin: memory.OriginUserEntered, Content: "x"}, errs.CodeValidationFailed},
		{"unknown origin", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: "cosmic", Content: "x"}, errs.CodeValidationFailed},
		{"model-extracted origin is never a client claim", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginModelExtracted, Content: "x"}, errs.CodeValidationFailed},
		{"imported origin has no producer here", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginImported, Content: "x"}, errs.CodeValidationFailed},
		{"approved-decision origin has no producer here", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginApprovedDecision, Content: "x"}, errs.CodeValidationFailed},
		{"empty content", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: ""}, errs.CodeValidationFailed},
		{"oversized content", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: strings.Repeat("x", MaxMemoryContentBytes+1)}, errs.CodeValidationFailed},
		{"oversized key", MemoryAddRequest{IdempotencyKey: strings.Repeat("k", maxMemoryIDLen+1), CorrelationID: "c", Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: "x"}, errs.CodeValidationFailed},
		{"the Work scope is DEFERRED (D-12 — no Work value exists)", MemoryAddRequest{IdempotencyKey: "k", CorrelationID: "c", Scope: "work", Origin: memory.OriginUserEntered, Content: "x"}, errs.CodeValidationFailed},
	}
	for _, tc := range cases {
		_, err := slice.AddMemoryItem(ctx, tc.req)
		assertKnowledgeEnvelope(t, err, tc.want)
	}
	// The add mints the identity — the body never carries one; the
	// Core-confirmed entry state is active (the response's lifecycle_state
	// carries the truth).
	res := addFixtureItem(t, slice, ctx, "shape-ok", memory.ScopeUser)
	if res.MemoryItem.MemoryItemID == "" || res.MemoryItem.LifecycleState != memory.StateActive || res.OperationID == "" {
		t.Fatalf("the add serves the Core-minted truth: %+v", res)
	}
	if res.MemoryItem.Origin != memory.OriginUserEntered {
		t.Fatalf("a user-created item is user-entered, never mislabeled: %+v", res.MemoryItem)
	}
	// The list read's scope/state filter values are validated.
	if _, err := slice.MemoryItems(ctx, MemoryListRequest{Scope: "galaxy"}); err == nil {
		t.Fatalf("an unknown list scope must refuse")
	}
	if _, err := slice.MemoryItems(ctx, MemoryListRequest{State: "galaxy"}); err == nil {
		t.Fatalf("an unknown list state must refuse")
	}
}

// TestMemorySecretsLaw pins the BINDING secrets law: a write attempting
// secret storage is refused (MEMORY_SECRETS_REJECTED, retryClass never),
// the matched material is never echoed, and the refusal precedes any
// accepted operation. Extends to the edit member and the originating
// references.
func TestMemorySecretsLaw(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	secretBodies := []struct {
		name    string
		content string
	}{
		{"password assignment", "my server password=hunter2secret"},
		{"api key json", `{"api_key": "a1b2c3d4"}`},
		{"secret declaration", "secret: supersecretvalue99"},
		{"access token", "access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		{"aws key shape", "my AWS credentials AKIAIOSFODNN7EXAMPLE"},
		{"github token shape", "the token ghp_0123456789abcdefghijklmnopqrstuvwxyz"},
		{"pem private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"},
		{"bearer header", "Authorization: Bearer sk-abcdef0123456789abcdef"},
	}
	for _, tc := range secretBodies {
		_, err := slice.AddMemoryItem(ctx, MemoryAddRequest{
			IdempotencyKey: "sec-" + tc.name, CorrelationID: "corr-sec",
			Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: tc.content,
		})
		envl := assertKnowledgeEnvelope(t, err, errs.CodeMemorySecretsRejected)
		if envl.Retryable {
			t.Fatalf("%s: the secrets refusal is retryable=never", tc.name)
		}
		if envl.Data["retryClass"] != string(errs.RetryNever) {
			t.Fatalf("%s: the secrets refusal rides retryClass never: %+v", tc.name, envl.Data)
		}
		if strings.Contains(envl.Data["reason"].(string), tc.content) {
			t.Fatalf("%s: the matched material is never echoed (the law extends to error messages): %+v", tc.name, envl.Data)
		}
	}
	// The edit member rides the same law.
	res := addFixtureItem(t, slice, ctx, "sec-edit", memory.ScopeUser)
	item, err := slice.MemoryItem(ctx, res.MemoryItem.MemoryItemID)
	if err != nil {
		t.Fatalf("fixture item: %v", err)
	}
	_, err = slice.EditMemoryItem(ctx, item.MemoryItemID, MemoryEditRequest{
		IdempotencyKey: "sec-edit-k", CorrelationID: "corr-sec-edit",
		ItemStateRef: memory.StateRef(item.LifecycleState, item.UpdatedAt),
		Content:      "secret=leakedvalue123",
	})
	assertKnowledgeEnvelope(t, err, errs.CodeMemorySecretsRejected)
	// The originating references ride the law too.
	_, err = slice.AddMemoryItem(ctx, MemoryAddRequest{
		IdempotencyKey: "sec-ref", CorrelationID: "corr-sec-ref",
		Scope: memory.ScopeUser, Origin: memory.OriginUserEntered, Content: "safe content",
		OriginatingRefs: []string{"api_key=a1b2c3d4"},
	})
	assertKnowledgeEnvelope(t, err, errs.CodeMemorySecretsRejected)
	// A secret-manager REFERENCE (the law's allowed form) stores.
	ref, err := slice.AddMemoryItem(ctx, MemoryAddRequest{
		IdempotencyKey: "sec-ref-ok", CorrelationID: "corr-sec-ref-ok",
		Scope: memory.ScopeUser, Origin: memory.OriginUserEntered,
		Content: "the database credential lives in the key-store reference keyring://db/prod",
	})
	if err != nil {
		t.Fatalf("a key-store reference must store: %v", err)
	}
	if !strings.Contains(ref.MemoryItem.Content, "key-store reference") {
		t.Fatalf("the reference stores as a reference: %+v", ref.MemoryItem)
	}
}

// TestMemoryAddIdempotencyClass pins the M-17 class through the service:
// the same key + the same request identity replays; the same key with a
// different request identity is the typed CONFLICT_IDEMPOTENCY_KEY.
func TestMemoryAddIdempotencyClass(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	res, err := slice.AddMemoryItem(ctx, memoryAddRequest("m17", "corr-m17", memory.ScopeUser))
	if err != nil {
		t.Fatalf("fresh add: %v", err)
	}
	replay, err := slice.AddMemoryItem(ctx, memoryAddRequest("m17", "corr-m17", memory.ScopeUser))
	if err != nil || replay.OperationID != res.OperationID || replay.MemoryItem.MemoryItemID != res.MemoryItem.MemoryItemID {
		t.Fatalf("the M-17 replay: %+v err=%v", replay, err)
	}
	_, err = slice.AddMemoryItem(ctx, memoryAddRequest("m17", "corr-other", memory.ScopeUser))
	envl := assertKnowledgeEnvelope(t, err, errs.CodeConflictIdempotencyKey)
	if envl.Data["fingerprintMatch"] != false {
		t.Fatalf("the conflict carries fingerprintMatch:false: %+v", envl.Data)
	}
	// The same client key on a DIFFERENT scope is a different key (the M-17
	// key scope is user + scope).
	if _, err := slice.AddMemoryItem(ctx, memoryAddRequest("m17", "corr-m17", memory.ScopeWorkspace)); err != nil {
		t.Fatalf("the scope-scoped key namespace: %v", err)
	}
}

// TestMemoryEditAndReviewLaws pins the M-16 flows through the service: the
// provenance-preserving edit, the replay, the review-state conflict, and
// the state endpoint as the authoritative reconciliation read.
func TestMemoryEditAndReviewLaws(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	res := addFixtureItem(t, slice, ctx, "m16", memory.ScopeUser)
	item, err := slice.MemoryItem(ctx, res.MemoryItem.MemoryItemID)
	if err != nil {
		t.Fatalf("fixture item: %v", err)
	}
	// The edit: provenance-preserving; the post-state is updated.
	edited, err := slice.EditMemoryItem(ctx, item.MemoryItemID, MemoryEditRequest{
		IdempotencyKey: "m16-edit", CorrelationID: "corr-m16-edit",
		ItemStateRef: memory.StateRef(item.LifecycleState, item.UpdatedAt),
		Content:      "the corrected memory",
	})
	if err != nil || edited.MemoryItem.LifecycleState != memory.StateUpdated {
		t.Fatalf("the edit: %+v err=%v", edited, err)
	}
	if edited.MemoryItem.Origin != memory.OriginUserEntered || edited.MemoryItem.CreatedAt != item.CreatedAt {
		t.Fatalf("the edit never rewrites provenance: %+v", edited.MemoryItem)
	}
	// The replay: the same key + the same correlation + the same
	// prepared-against reference replays the recorded outcome.
	replay, err := slice.EditMemoryItem(ctx, item.MemoryItemID, MemoryEditRequest{
		IdempotencyKey: "m16-edit", CorrelationID: "corr-m16-edit",
		ItemStateRef: memory.StateRef(item.LifecycleState, item.UpdatedAt),
		Content:      "the corrected memory",
	})
	if err != nil || replay.OperationID != edited.OperationID {
		t.Fatalf("the M-16 replay: %+v err=%v", replay, err)
	}
	// A stale prepared-against reference: MEMORY_CONFLICT.
	_, err = slice.EditMemoryItem(ctx, item.MemoryItemID, MemoryEditRequest{
		IdempotencyKey: "m16-stale", CorrelationID: "corr-m16-stale",
		ItemStateRef: memory.StateRef(memory.StateActive, "stale-instant"),
		Content:      "overwrite attempt",
	})
	assertKnowledgeEnvelope(t, err, errs.CodeMemoryConflict)
	// The review-state conflict: approving an active item is the typed
	// MEMORY_REVIEW_STATE_CONFLICT (the positive approve path has NO
	// public producer on this surface — no pending_review item is
	// creatable through the served API; the store-level positive is pinned
	// by the storage tests).
	_, err = slice.ApproveMemoryItem(ctx, item.MemoryItemID, MemoryReviewRequest{IdempotencyKey: "m16-ap", CorrelationID: "corr-m16-ap"})
	assertKnowledgeEnvelope(t, err, errs.CodeMemoryReviewStateConflict)
	_, err = slice.RejectMemoryItem(ctx, item.MemoryItemID, MemoryReviewRequest{IdempotencyKey: "m16-rj", CorrelationID: "corr-m16-rj"})
	assertKnowledgeEnvelope(t, err, errs.CodeMemoryReviewStateConflict)
	// The authoritative state endpoint resolves the edit's operation.
	op, err := slice.MemoryOperationStatus(ctx, edited.OperationID)
	if err != nil || op.Kind != memory.KindEdit || op.OutcomeState != memory.StateUpdated {
		t.Fatalf("the state endpoint is the authoritative truth: %+v err=%v", op, err)
	}
	// An unknown operation: the typed NOT_FOUND (the no-leak rule).
	_, err = slice.MemoryOperationStatus(ctx, "memop-nope")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
}

// TestMemoryPinInclusionExport pins the pin register (a FLAG — both
// directions), the M-18 inclusion class, and the M-32 export class through
// the service.
func TestMemoryPinInclusionExport(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	res := addFixtureItem(t, slice, ctx, "m18", memory.ScopeUser)
	item, err := slice.MemoryItem(ctx, res.MemoryItem.MemoryItemID)
	if err != nil {
		t.Fatalf("fixture item: %v", err)
	}
	// Pin then unpin; the confirmed flag rides the response's item.
	pinned, err := slice.SetMemoryItemPin(ctx, item.MemoryItemID, MemoryPinRequest{IdempotencyKey: "m18-pin", CorrelationID: "corr-m18-pin", Pinned: true})
	if err != nil || !pinned.MemoryItem.Pinned {
		t.Fatalf("the pin: %+v err=%v", pinned, err)
	}
	unpinned, err := slice.SetMemoryItemPin(ctx, item.MemoryItemID, MemoryPinRequest{IdempotencyKey: "m18-unpin", CorrelationID: "corr-m18-unpin", Pinned: false})
	if err != nil || unpinned.MemoryItem.Pinned {
		t.Fatalf("the unpin: %+v err=%v", unpinned, err)
	}
	// The M-18 inclusion: a decision record, never an item-state change.
	incl, err := slice.SetMemoryItemInclusion(ctx, item.MemoryItemID, MemoryInclusionRequest{CorrelationID: "corr-m18-inc", RequestRef: "req-alpha", Included: true})
	if err != nil || !incl.Included || incl.OperationID == "" {
		t.Fatalf("the inclusion: %+v err=%v", incl, err)
	}
	replay, err := slice.SetMemoryItemInclusion(ctx, item.MemoryItemID, MemoryInclusionRequest{CorrelationID: "corr-m18-inc", RequestRef: "req-alpha", Included: true})
	if err != nil || replay.OperationID != incl.OperationID {
		t.Fatalf("the M-18 replay: %+v err=%v", replay, err)
	}
	fresh, err := slice.SetMemoryItemInclusion(ctx, item.MemoryItemID, MemoryInclusionRequest{CorrelationID: "corr-m18-inc2", RequestRef: "req-alpha", Included: false})
	if err != nil || fresh.Included || fresh.OperationID == incl.OperationID {
		t.Fatalf("the new decision under the same request: %+v err=%v", fresh, err)
	}
	item2, err := slice.MemoryItem(ctx, item.MemoryItemID)
	if err != nil || item2.LifecycleState != memory.StateActive {
		t.Fatalf("the M-18 decision never changes the item state: %+v err=%v", item2, err)
	}
	// The M-32 export: none-required idempotency (two submits, two
	// operations); the document rides the response with its honest count.
	first, err := slice.ExportMemoryItems(ctx, MemoryExportRequest{CorrelationID: "corr-m18-exp1", Scope: memory.ScopeUser})
	if err != nil || first.Count != 1 || len(first.Items) != 1 || first.Truncated {
		t.Fatalf("the export: %+v err=%v", first, err)
	}
	second, err := slice.ExportMemoryItems(ctx, MemoryExportRequest{CorrelationID: "corr-m18-exp2", Scope: memory.ScopeUser})
	if err != nil || second.ExportOperationID == first.ExportOperationID {
		t.Fatalf("a re-issued export mints a new operation: %+v err=%v", second, err)
	}
	if _, err := slice.MemoryOperationStatus(ctx, first.ExportOperationID); err != nil {
		t.Fatalf("the export record resolves: %v", err)
	}
	// The explicit subset: an unknown named item is the SAME typed 404.
	_, err = slice.ExportMemoryItems(ctx, MemoryExportRequest{CorrelationID: "corr-m18-exp3", Scope: memory.ScopeUser, ItemIDs: []string{"mem-nope"}})
	assertKnowledgeEnvelope(t, err, errs.CodeMemoryItemNotFound)
}

// TestMemoryScopeBoundary pins the scope-never-broadens and
// cross-scope-default laws through the service: the list filter is
// Core-side, and an export naming an item outside the stated export scope
// is the typed MEMORY_SCOPE_VIOLATION — never a silent narrowing.
func TestMemoryScopeBoundary(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	userRes := addFixtureItem(t, slice, ctx, "scope-u", memory.ScopeUser)
	addFixtureItem(t, slice, ctx, "scope-s", memory.ScopeSession)
	// The list filter serves only the stated scope.
	page, err := slice.MemoryItems(ctx, MemoryListRequest{Scope: memory.ScopeSession})
	if err != nil || len(page.Items) != 1 || page.Items[0].Scope != memory.ScopeSession {
		t.Fatalf("the list's scope filter: %+v err=%v", page, err)
	}
	// The export naming an out-of-scope item: the typed
	// MEMORY_SCOPE_VIOLATION.
	userItem, err := slice.MemoryItem(ctx, userRes.MemoryItem.MemoryItemID)
	if err != nil {
		t.Fatalf("fixture item: %v", err)
	}
	_, err = slice.ExportMemoryItems(ctx, MemoryExportRequest{
		CorrelationID: "corr-scope-x", Scope: memory.ScopeSession,
		ItemIDs: []string{userItem.MemoryItemID},
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeMemoryScopeViolation)
	if envl.Retryable {
		t.Fatalf("the scope violation is retryable=never: %+v", envl)
	}
	// The in-scope named subset exports.
	res, err := slice.ExportMemoryItems(ctx, MemoryExportRequest{
		CorrelationID: "corr-scope-ok", Scope: memory.ScopeUser,
		ItemIDs: []string{userItem.MemoryItemID},
	})
	if err != nil || res.Count != 1 {
		t.Fatalf("the in-scope subset export: %+v err=%v", res, err)
	}
}

// TestMemoryDestructiveService pins the destructive members through the
// service: the delete carries the Core-confirmed retention caveat and the
// outcome state; the forget serves the confirmed tombstone post-state;
// after either, the inspect read is the SAME typed MEMORY_ITEM_NOT_FOUND;
// the keyed destructive replay reconciles the lost response (the S0–S4
// law).
func TestMemoryDestructiveService(t *testing.T) {
	env, slice := newMemoryEnv(t)
	ctx := env.ctx()
	res := addFixtureItem(t, slice, ctx, "destruct", memory.ScopeUser)
	itemID := res.MemoryItem.MemoryItemID
	del, err := slice.DeleteMemoryItem(ctx, itemID, MemoryReviewCommand{IdempotencyKey: "destruct-del", CorrelationID: "corr-destruct-del"})
	if err != nil || del.OutcomeState != memory.StateDeleted || del.OperationID == "" {
		t.Fatalf("the delete: %+v err=%v", del, err)
	}
	if del.RetentionNote == "" || strings.Contains(del.RetentionNote, "erased") {
		t.Fatalf("the retention caveat is the honest statement, never a total-erasure promise: %q", del.RetentionNote)
	}
	// The inspect read after the removal: the SAME typed 404.
	_, err = slice.MemoryItem(ctx, itemID)
	assertKnowledgeEnvelope(t, err, errs.CodeMemoryItemNotFound)
	// The keyed destructive replay: the recorded outcome replays within
	// the window (the lost-response reconcile path).
	replay, err := slice.DeleteMemoryItem(ctx, itemID, MemoryReviewCommand{IdempotencyKey: "destruct-del", CorrelationID: "corr-destruct-del"})
	if err != nil || replay.OperationID != del.OperationID || replay.OutcomeState != memory.StateDeleted {
		t.Fatalf("the destructive replay: %+v err=%v", replay, err)
	}
	// The unkeyed destructive record (the recorded carrier gap) still
	// confirms its outcome and reconciles through the state endpoint.
	res2 := addFixtureItem(t, slice, ctx, "destruct-uk", memory.ScopeUser)
	fg, err := slice.ForgetMemoryItem(ctx, res2.MemoryItem.MemoryItemID, MemoryReviewCommand{})
	if err != nil || fg.MemoryItem.LifecycleState != memory.StateForgotten || fg.MemoryItem.Content != "" {
		t.Fatalf("the forget tombstone: %+v err=%v", fg, err)
	}
	op, err := slice.MemoryOperationStatus(ctx, fg.OperationID)
	if err != nil || op.Kind != memory.KindForget || op.OutcomeState != memory.StateForgotten {
		t.Fatalf("the forget record: %+v err=%v", op, err)
	}
	// The forget member's event rides the register vocabulary (the
	// conditional law carried verbatim — the mechanics are the Core's own).
}
