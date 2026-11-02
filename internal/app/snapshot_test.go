package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The context-snapshot read tests (the owner's card-11 working
// candidate): the read's shape (included/omitted with their Core-supplied
// reasons; the budget pair; the exact-vs-approximate label), the
// authorization, the refusal ladder, and the no-bulk-content rule. The
// store's conflict-on-duplicate is pinned AS CODED at the storage layer
// (internal/storage contextengine_test.go).

// submitAndSettle submits the fixture task and runs it to settlement so
// the real execution path compiles and stores the attempt's snapshot.
func submitAndSettle(t *testing.T, env *testEnv, mutate func(*SubmitRequest)) string {
	t.Helper()
	req := baseRequest()
	req.IdempotencyKey = "idem-snap-" + time.Now().UTC().Format("150405.000000000")
	if mutate != nil {
		mutate(&req)
	}
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-snap", 1, sub.Job.JobID, 64)
	if err != nil {
		t.Fatalf("run to settlement: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded {
		t.Fatalf("job did not settle successfully: %+v", final)
	}
	return sub.Job.JobID
}

// TestJobContextSnapshotReadShape pins the read's shape over a REAL
// execution path snapshot: the identity (the coded snapshot-identity
// grammar), the scope, the created-at, the exact-vs-approximate label,
// the provenance versions verbatim, and the empty included/omitted lists
// as honest empties (a no-pack compile selects no candidates).
func TestJobContextSnapshotReadShape(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettle(t, env, func(req *SubmitRequest) { req.MaxOutputTokens = 64 })

	view, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 0)
	if err != nil {
		t.Fatalf("snapshot read: %v", err)
	}
	if view.SnapshotID != SnapshotIDFor(jobID, 1) {
		t.Fatalf("identity must follow the coded grammar: got %q want %q", view.SnapshotID, SnapshotIDFor(jobID, 1))
	}
	if view.TaskRef != jobID || view.Scope != "local" {
		t.Fatalf("task_ref/scope must be the compile's own values: %+v", view)
	}
	if view.SchemaVersion != contextengine.SchemaVersion {
		t.Fatalf("schema version must ride the record: %d", view.SchemaVersion)
	}
	if _, err := time.Parse(time.RFC3339Nano, view.CreatedAt); err != nil {
		t.Fatalf("created_at must be an RFC 3339 instant: %q", view.CreatedAt)
	}
	if view.ExactVsApproximate != snapshotExactValue {
		t.Fatalf("exact-vs-approximate label must be the pinned core_produced value: %q", view.ExactVsApproximate)
	}
	if view.EstimatorVersion == "" || view.RendererVersion == "" || view.CompressionMethod != contextengine.CompressionNone || view.RetrievalConfigVersion == "" {
		t.Fatalf("the provenance versions must ride the record verbatim: %+v", view)
	}
	// No pack: no candidates — the honest empty lists (never null, never
	// invented coverage).
	if view.Included == nil || view.Omitted == nil {
		t.Fatalf("included/omitted must be honest empty lists, not nil: %+v", view)
	}
	if len(view.Included) != 0 || len(view.Omitted) != 0 {
		t.Fatalf("a no-pack compile selects nothing: %+v", view)
	}
	// The budget pair: the reserved output bound the compile carried, the
	// Core's compiled estimate, the estimator version verbatim.
	if view.Budget.Requested.ReservedOutputTokens == nil || *view.Budget.Requested.ReservedOutputTokens != 64 {
		t.Fatalf("the reserved output bound must be present as requested: %+v", view.Budget)
	}
	if view.Budget.Requested.MaxInputTokens != nil || view.Budget.Requested.MaxItems != nil {
		t.Fatalf("a zero bound means no bound of this kind — it must render ABSENT: %+v", view.Budget)
	}
	if view.Budget.EstimatedInputTokens <= 0 {
		t.Fatalf("the compiled estimate must be reported: %d", view.Budget.EstimatedInputTokens)
	}
	if view.Budget.EstimatorVersion != view.EstimatorVersion {
		t.Fatalf("the estimate carries its estimator version: %+v", view.Budget)
	}
}

// TestJobContextSnapshotIncludedEntries pins the included list over a
// pack-driven compile: each entry carries the exact canonical reference,
// the source digest, the trust label, and the inclusion reason — the
// coded values, never invented.
func TestJobContextSnapshotIncludedEntries(t *testing.T) {
	env := newTestEnv(t)
	canaryText := "The fixture corpus states the pack fact verbatim for the snapshot read."
	packID := seedFixturePack(t, env, canaryText)
	jobID := submitAndSettle(t, env, func(req *SubmitRequest) {
		req.Task = canaryText
		req.PackRef = packID
	})

	view, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 0)
	if err != nil {
		t.Fatalf("snapshot read: %v", err)
	}
	if len(view.Included) != 1 {
		t.Fatalf("the pack chunk must appear exactly once in the included list: %+v", view.Included)
	}
	item := view.Included[0]
	if item.Ref == "" || item.Digest == "" || item.TrustLabel != string(contextengine.TrustUntrustedRetrived) {
		t.Fatalf("the included entry must carry ref/digest/trust label: %+v", item)
	}
	if item.Reason != "within_budget_and_policy" {
		t.Fatalf("the inclusion reason must be the coded value: %q", item.Reason)
	}
	if !strings.HasPrefix(item.Digest, "sha256:") {
		t.Fatalf("the digest must carry the C2-12 grammar: %q", item.Digest)
	}
}

// snapshotOmissionCase drives one Compile over candidates crafted to
// produce exactly one class of omission (the code's own pipeline — no new
// selection logic; the read then returns what the store carries).
func snapshotOmissionCase(t *testing.T, env *testEnv, name string, build func(req *SubmitRequest, candidates *[]contextengine.RetrievedItem, policy *contextengine.Policy, budget *contextengine.Budget)) (string, *SnapshotView) {
	t.Helper()
	jobID := submitAndSettle(t, env, func(req *SubmitRequest) {})

	candidates := []contextengine.RetrievedItem{}
	policy := contextengine.Policy{Scope: "local", AuthorizedScopes: []string{"local"}, SystemPolicyRef: "uawp.slice.v1"}
	budget := contextengine.Budget{}
	build(nil, &candidates, &policy, &budget)
	snap, err := contextengine.Compile(SnapshotIDFor(jobID, 2), jobID, "read the fixture task", candidates, policy, budget, contextengine.CharRatioEstimator{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: compile: %v", name, err)
	}
	if err := env.deps.Snapshots.SaveSnapshot(env.ctx(), *snap); err != nil {
		t.Fatalf("%s: save crafted snapshot: %v", name, err)
	}
	view, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 2)
	if err != nil {
		t.Fatalf("%s: snapshot read: %v", name, err)
	}
	return jobID, view
}

// TestJobContextSnapshotOmissionReasons pins the omitted list WITH its
// Core-supplied reasons: each excluded candidate carries exactly one of
// the coded C3-09 vocabulary values, verbatim — no silent truncation, no
// invented reason.
func TestJobContextSnapshotOmissionReasons(t *testing.T) {
	cases := []struct {
		name       string
		build      func(req *SubmitRequest, candidates *[]contextengine.RetrievedItem, policy *contextengine.Policy, budget *contextengine.Budget)
		wantRef    string
		wantReason string
	}{
		{
			name: "scope_denied",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "out-of-scope", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "memory", Scope: "other-scope"})
			},
			wantRef: "out-of-scope", wantReason: contextengine.OmitScopeDenied,
		},
		{
			name: "pack_quarantined",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "quarantined-chunk", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "knowledgepack", Scope: "local", PackStatus: "quarantined"})
			},
			wantRef: "quarantined-chunk", wantReason: contextengine.OmitPackQuarantined,
		},
		{
			name: "claim_revoked",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "revoked-chunk", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "knowledgepack", Scope: "local", ClaimStatus: "revoked"})
			},
			wantRef: "revoked-chunk", wantReason: contextengine.OmitClaimRevoked,
		},
		{
			name: "claim_superseded",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "superseded-chunk", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "knowledgepack", Scope: "local", ClaimStatus: "superseded"})
			},
			wantRef: "superseded-chunk", wantReason: contextengine.OmitClaimSuperseded,
		},
		{
			name: "stale_excluded",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "stale-chunk", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "memory", Scope: "local", Freshness: "stale"})
			},
			wantRef: "stale-chunk", wantReason: contextengine.OmitStale,
		},
		{
			name: "license_unknown",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, policy *contextengine.Policy, _ *contextengine.Budget) {
				policy.RequireLicenseKnown = true
				*candidates = append(*candidates, contextengine.RetrievedItem{Ref: "unknown-license", Text: "x", Digest: DigestOf([]byte("x")), SourceKind: "knowledgepack", Scope: "local", LicenseStatus: "unknown"})
			},
			wantRef: "unknown-license", wantReason: contextengine.OmitLicenseUnknown,
		},
		{
			name: "duplicate",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, _ *contextengine.Budget) {
				*candidates = append(*candidates,
					contextengine.RetrievedItem{Ref: "first-copy", Text: "same text", Digest: DigestOf([]byte("same text")), SourceKind: "memory", Scope: "local"},
					contextengine.RetrievedItem{Ref: "second-copy", Text: "same text", Digest: DigestOf([]byte("same text")), SourceKind: "memory", Scope: "local"})
			},
			wantRef: "second-copy", wantReason: contextengine.OmitDuplicate,
		},
		{
			name: "item_cap",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, budget *contextengine.Budget) {
				budget.MaxItems = 1
				*candidates = append(*candidates,
					contextengine.RetrievedItem{Ref: "kept-item", Text: "kept", Digest: DigestOf([]byte("kept")), SourceKind: "memory", Scope: "local"},
					contextengine.RetrievedItem{Ref: "capped-item", Text: "capped", Digest: DigestOf([]byte("capped")), SourceKind: "memory", Scope: "local"})
			},
			wantRef: "capped-item", wantReason: contextengine.OmitItemCap,
		},
		{
			name: "budget_exhausted",
			build: func(_ *SubmitRequest, candidates *[]contextengine.RetrievedItem, _ *contextengine.Policy, budget *contextengine.Budget) {
				budget.MaxInputTokens = 40
				*candidates = append(*candidates,
					contextengine.RetrievedItem{Ref: "kept-within-budget", Text: "ok", Digest: DigestOf([]byte("ok")), SourceKind: "memory", Scope: "local"},
					contextengine.RetrievedItem{Ref: "over-budget", Text: "this retrieval candidate is deliberately long so that its estimated token cost exceeds the remaining input budget entirely", Digest: DigestOf([]byte("big")), SourceKind: "memory", Scope: "local"})
			},
			wantRef: "over-budget", wantReason: contextengine.OmitBudgetExhausted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			_, view := snapshotOmissionCase(t, env, tc.name, tc.build)
			found := false
			for _, omission := range view.Omitted {
				if omission.Ref == tc.wantRef {
					found = true
					if omission.Reason != tc.wantReason {
						t.Fatalf("omission reason must be the coded value %q, got %q", tc.wantReason, omission.Reason)
					}
				}
			}
			if !found {
				t.Fatalf("the omitted list must carry ref %q: %+v", tc.wantRef, view.Omitted)
			}
		})
	}
}

// TestSnapshotBudgetPairAbsentNeverZero pins the budget pair's absent
// discipline through the read: a zero bound means "no bound of this kind"
// and renders ABSENT on the wire — never a fabricated zero; the estimate
// is reported with its estimator version; no provider-reported usage and
// no cost member exist at all (no source in the record).
func TestSnapshotBudgetPairAbsentNeverZero(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettle(t, env, nil)

	candidates := []contextengine.RetrievedItem{
		{Ref: "only-item", Text: "small", Digest: DigestOf([]byte("small")), SourceKind: "memory", Scope: "local"},
	}
	snap, err := contextengine.Compile(SnapshotIDFor(jobID, 2), jobID, "the task", candidates,
		contextengine.Policy{Scope: "local", AuthorizedScopes: []string{"local"}}, contextengine.Budget{}, contextengine.CharRatioEstimator{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := env.deps.Snapshots.SaveSnapshot(env.ctx(), *snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	view, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "max_input_tokens") || strings.Contains(body, "max_items") {
		t.Fatalf("a zero bound must be ABSENT from the wire, never zero: %s", body)
	}
	if view.Budget.Requested.ReservedOutputTokens != nil {
		t.Fatalf("an unset bound must be absent: %+v", view.Budget)
	}
	// No provider-reported usage and no cost member exist anywhere in the
	// shape: missing renders Not-reported upstream, never zero.
	for _, forbidden := range []string{"provider_reported", "actual", "cost", "price", "currency"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the budget shape must not fabricate a %q member: %s", forbidden, body)
		}
	}
	if view.Budget.EstimatedInputTokens <= 0 || view.Budget.EstimatorVersion == "" {
		t.Fatalf("the estimate must ride its estimator version: %+v", view.Budget)
	}
}

// TestJobContextSnapshotRefusalLadder pins the read's typed refusals:
// the empty id (VALIDATION_FAILED), the unknown Job and the
// never-claimed Job (NOT_FOUND — no fabricated structure), the explicit
// attempt with no stored snapshot (NOT_FOUND), and the unauthenticated
// caller (AUTHZ_DENIED through the decision port, changing nothing).
func TestJobContextSnapshotRefusalLadder(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettle(t, env, nil)

	t.Run("empty_job_id", func(t *testing.T) {
		_, err := env.slice.JobContextSnapshot(env.ctx(), "", 0)
		if err == nil {
			t.Fatal("an empty job id must refuse")
		}
		if !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
			t.Fatalf("the refusal must be the typed envelope: %v", err)
		}
	})
	t.Run("unknown_job", func(t *testing.T) {
		_, err := env.slice.JobContextSnapshot(env.ctx(), "job-does-not-exist", 0)
		if err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
			t.Fatalf("an unknown job must be the typed NOT_FOUND: %v", err)
		}
	})
	t.Run("never_claimed_job", func(t *testing.T) {
		req := baseRequest()
		req.IdempotencyKey = "idem-never-claimed"
		sub := env.submit(t, req)
		_, err := env.slice.JobContextSnapshot(env.ctx(), sub.Job.JobID, 0)
		if err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
			t.Fatalf("a never-claimed job has no snapshot: %v", err)
		}
	})
	t.Run("explicit_attempt_without_snapshot", func(t *testing.T) {
		_, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 9)
		if err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
			t.Fatalf("an attempt with no stored snapshot must be the typed NOT_FOUND: %v", err)
		}
	})
	t.Run("unauthenticated_caller", func(t *testing.T) {
		bare := context.Background()
		_, err := env.slice.JobContextSnapshot(WithCaller(bare, "local", ""), jobID, 0)
		if err == nil || !strings.Contains(err.Error(), string(errs.CodeAuthzDenied)) {
			t.Fatalf("the decision port must deny the credential-less caller: %v", err)
		}
	})
}

// TestJobContextSnapshotNoBulkContent is the no-bulk-content pin: the
// read returns the STRUCTURE only. The compiled representation (which
// embeds the task text and every retrieved item's text), the provider
// handle, and any item text NEVER appear in the read's marshaled shape.
func TestJobContextSnapshotNoBulkContent(t *testing.T) {
	env := newTestEnv(t)
	canaryText := "SECRET-ADJACENT-CANARY the assembled representation embeds this task text verbatim."
	packID := seedFixturePack(t, env, "Pack chunk body with its own canary-marker content.")
	jobID := submitAndSettle(t, env, func(req *SubmitRequest) {
		req.Task = canaryText
		req.PackRef = packID
	})

	// The stored record DOES carry the compiled content (the store is the
	// Core's own record); the READ must not expose it.
	stored, err := env.deps.Snapshots.LoadSnapshot(env.ctx(), SnapshotIDFor(jobID, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.AssembledRepresentation, canaryText) {
		t.Fatalf("fixture precondition: the record carries the compiled content")
	}
	stored.ProviderHandle = "provider-handle-canary"
	if err := env.deps.Snapshots.SaveSnapshot(env.ctx(), stored); err == nil {
		t.Fatal("fixture precondition: snapshots are immutable (the duplicate save must conflict)")
	}

	view, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, forbidden := range []string{canaryText, "Pack chunk body", "assembled_representation", "provider_handle", "provider-handle-canary"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the read must never expose compiled content (%q leaked): %s", forbidden, body)
		}
	}
	if view.AssembledDigest == "" {
		t.Fatal("the one-way assembled digest is a structural fact the record carries")
	}
}

// TestJobContextSnapshotReadCreatesNothing pins the read's QUERY nature:
// reading twice returns identical structures and the read leaves the
// Job's journal and the store's records untouched.
func TestJobContextSnapshotReadCreatesNothing(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettle(t, env, nil)

	eventsBefore, err := env.slice.Events(env.ctx(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	second, err := env.slice.JobContextSnapshot(env.ctx(), jobID, 0)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("the read must be stable: %s vs %s", firstJSON, secondJSON)
	}
	eventsAfter, err := env.slice.Events(env.ctx(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsBefore) != len(eventsAfter) {
		t.Fatalf("the read must record no event: %d vs %d", len(eventsBefore), len(eventsAfter))
	}
	// The store's records are untouched: the crafted-id probe still does
	// not exist (the read minted nothing).
	if _, err := env.deps.Snapshots.LoadSnapshot(env.ctx(), SnapshotIDFor(jobID, 42)); err == nil {
		t.Fatal("the read must not create any snapshot record")
	}
}
