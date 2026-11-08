package app

import (
	stdcontext "context"
	"testing"

	contextdomain "github.com/qira-dev/karyora/internal/domain/context"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
)

// The B3 ContextSnapshot family's service register (DE-0 queue row 5):
// the M-18 request-scoped inclusion class implemented fully, the two
// CONDITIONAL commands serving their typed unmet-condition behaviors
// exactly as the contract's descriptions carry them (never a fake success
// path), the authoritative state endpoint with its SAME typed 404, the
// authorization boundary through the one decision port, and the
// Core-provided-only reason law.

// newContextEnv is newTestEnv with the context and memory ports wired to
// the same store (the test-side twin of the composition root's wiring —
// the inclusion decisions resolve their items through the wired Memory
// port's authoritative read).
func newContextEnv(t testing.TB) (*testEnv, *Slice) {
	t.Helper()
	env := newTestEnv(t)
	cdeps := env.deps
	cdeps.Memory = env.store
	cdeps.Context = env.store
	cslice, err := NewSlice(cdeps)
	if err != nil {
		t.Fatalf("context slice: %v", err)
	}
	return env, cslice
}

// contextFixtureItem adds one item through the Memory service (the
// decision's item fixture — the existence check resolves through the same
// port the product uses).
func contextFixtureItem(t *testing.T, slice *Slice, ctx stdcontext.Context, key string) memory.Item {
	t.Helper()
	res, err := slice.AddMemoryItem(ctx, memoryAddRequest(key, "corr-"+key, memory.ScopeUser))
	if err != nil {
		t.Fatalf("fixture add: %v", err)
	}
	return res.MemoryItem
}

// TestContextAuthzBoundary pins the authorization boundary: every B3
// surface refuses the session-less caller with the typed
// AUTHZ_DENIED/SESSION_INVALID envelope through the one decision port —
// and never the data. Local mode stays role-free.
func TestContextAuthzBoundary(t *testing.T) {
	env, slice := newContextEnv(t)
	noSession := WithCaller(stdcontext.Background(), "local", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"save snapshot", func() error {
			_, err := slice.SaveContextSnapshot(noSession, ContextSaveRequest{IdempotencyKey: "k", CorrelationID: "c", SourceJobID: "j"})
			return err
		}},
		{"compare", func() error {
			_, err := slice.CompareContextSnapshots(noSession, ContextCompareRequest{BaselineSnapshotID: "b", CandidateSnapshotID: "c"})
			return err
		}},
		{"inclusion", func() error {
			_, err := slice.SetContextInclusion(noSession, ContextInclusionRequest{
				CorrelationID: "c", RequestRef: "r",
				Decisions: []contextdomain.InclusionDecision{{MemoryItemID: "mem-x", Included: true}},
			})
			return err
		}},
		{"state read", func() error { _, err := slice.ContextOperationStatus(noSession, "ctxop-x"); return err }},
	}
	for _, tc := range cases {
		err := tc.call()
		envl := assertKnowledgeEnvelope(t, err, errs.CodeAuthzDenied)
		if envl.Data["reason"] != "SESSION_INVALID" {
			t.Fatalf("%s: the session boundary refusal: %+v", tc.name, envl.Data)
		}
	}
	// One authenticated positive (the state read's typed 404 — the
	// boundary, not the router, answers, and the code is the family's
	// own typed carrier).
	_, err := slice.ContextOperationStatus(env.ctx(), "ctxop-authz-ok")
	assertKnowledgeEnvelope(t, err, errs.CodeContextSnapshotNotFound)
}

// TestContextSaveConditionalUnmet pins the M-19 save-snapshot command's
// PUBLISHED unmet-condition behavior exactly: the command's condition —
// durable snapshot storage existing — is the GC-3 un-gate precondition,
// the owner's PROPOSED decision, unresolved; while unmet the command
// serves the typed refusal, NO operation row can exist, and no fake
// success path (no 202, no fabricated operation, no replay) exists
// anywhere. The refusal rides the family's honest-unavailable register
// code with its published safe-retry assignment.
func TestContextSaveConditionalUnmet(t *testing.T) {
	env, slice := newContextEnv(t)
	ctx := env.ctx()

	// The gate constants are the family's own support state (the owner's
	// PROPOSED decisions, unresolved — the implementation asserts the
	// recorded state, never flips it).
	if contextdomain.DurableSnapshotStorageSupported {
		t.Fatalf("the durable-storage gate is the owner's PROPOSED decision — it must stand unresolved")
	}
	if contextdomain.StableComparisonSupported {
		t.Fatalf("the comparison gate is the owner's PROPOSED decision — it must stand unresolved")
	}

	// The save attempt: the typed unmet-condition refusal — never a 202,
	// never an operation row, never a replay flag.
	res, err := slice.SaveContextSnapshot(ctx, ContextSaveRequest{
		IdempotencyKey: "key-save-1", CorrelationID: "corr-save-1", SourceJobID: "job-save-1",
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeContextCompilationUnavailable)
	if res.Operation.OperationID != "" || res.Replayed {
		t.Fatalf("no fake success path exists: %+v", res)
	}
	if envl.Retryable != true {
		t.Fatalf("the register's published safe-retry assignment: %+v", envl)
	}
	if envl.Data["precondition"] != "GC-3" {
		t.Fatalf("the refusal names its un-gate precondition honestly: %+v", envl.Data)
	}

	// The same key re-issued: the SAME refusal — no idempotency row was
	// consumed (no operation row can exist), so the refusal is stable and
	// nothing was recorded.
	_, err = slice.SaveContextSnapshot(ctx, ContextSaveRequest{
		IdempotencyKey: "key-save-1", CorrelationID: "corr-save-1", SourceJobID: "job-save-1",
	})
	assertKnowledgeEnvelope(t, err, errs.CodeContextCompilationUnavailable)

	// The validation bound still applies ahead of the refusal (the
	// bounded-identity law rides every surface).
	_, err = slice.SaveContextSnapshot(ctx, ContextSaveRequest{
		IdempotencyKey: "", CorrelationID: "corr-x", SourceJobID: "j",
	})
	assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
}

// TestContextCompareConditionalUnmet pins the M-20 snapshot compare's
// PUBLISHED unmet-condition behavior exactly: where stable Core comparison
// does not exist the family answers the typed CONTEXT_COMPARE_UNSUPPORTED
// — the comparison-unavailable state renders honestly and no diff is ever
// fabricated. The refusal is read-only (mutates nothing — the read-with-
// output class's safe-retry re-issue) and rides the register's published
// safe-retry assignment.
func TestContextCompareConditionalUnmet(t *testing.T) {
	env, slice := newContextEnv(t)
	ctx := env.ctx()
	res, err := slice.CompareContextSnapshots(ctx, ContextCompareRequest{
		BaselineSnapshotID: "snap-baseline-1", CandidateSnapshotID: "snap-candidate-1",
	})
	envl := assertKnowledgeEnvelope(t, err, errs.CodeContextCompareUnsupported)
	if res.Comparison != nil || res.BaselineSnapshotID != "" || res.CandidateSnapshotID != "" {
		t.Fatalf("no diff is ever fabricated: %+v", res)
	}
	if envl.Retryable != true {
		t.Fatalf("the register's published safe-retry assignment: %+v", envl)
	}
	if envl.Data["precondition"] != "GC-4" {
		t.Fatalf("the refusal names its un-gate precondition honestly: %+v", envl.Data)
	}

	// The validation bounds ride the read-only class ahead of the
	// refusal.
	cases := []struct {
		name string
		req  ContextCompareRequest
	}{
		{"missing baseline", ContextCompareRequest{CandidateSnapshotID: "c"}},
		{"missing candidate", ContextCompareRequest{BaselineSnapshotID: "b"}},
		{"oversized baseline", ContextCompareRequest{BaselineSnapshotID: string(make([]byte, maxContextIDLen+1)), CandidateSnapshotID: "c"}},
	}
	for _, tc := range cases {
		_, err := slice.CompareContextSnapshots(ctx, tc.req)
		assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	}
}

// TestContextInclusionM18 pins the M-18 request-scoped inclusion class
// through the service: the validation bounds, the decided item's
// existence resolved through the Memory port's authoritative read (the
// SAME typed 404 the Memory family's inspect read serves — the no-leak
// rule), the Core-confirmed decision record, the replay of the same
// request identity, and the M-18 boundary (no item-state change).
func TestContextInclusionM18(t *testing.T) {
	env, slice := newContextEnv(t)
	ctx := env.ctx()
	item := contextFixtureItem(t, slice, ctx, "ctx-incl-1")

	// The fresh decision: the Core-confirmed record for the stated
	// request; the reason members stay unset (the Core-provided-only
	// reason law — no reason producer exists, absence is the absence).
	res, err := slice.SetContextInclusion(ctx, ContextInclusionRequest{
		CorrelationID: "corr-incl-1", RequestRef: "req-incl-1",
		Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: true}},
	})
	if err != nil {
		t.Fatalf("inclusion: %v", err)
	}
	if res.RequestRef != "req-incl-1" || len(res.Decisions) != 1 || !res.Decisions[0].Included {
		t.Fatalf("the decision record: %+v", res)
	}
	if res.Decisions[0].Reason != nil {
		t.Fatalf("no reason is ever synthesized: %+v", *res.Decisions[0].Reason)
	}

	// The same request identity replays the recorded decision.
	replay, err := slice.SetContextInclusion(ctx, ContextInclusionRequest{
		CorrelationID: "corr-incl-1", RequestRef: "req-incl-1",
		Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: false}},
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(replay.Decisions) != 1 || !replay.Decisions[0].Included {
		t.Fatalf("the replay carries the recorded decision, never the re-issued body: %+v", replay.Decisions)
	}

	// The item's state is untouched (the M-18 boundary).
	fresh, err := slice.MemoryItem(ctx, item.MemoryItemID)
	if err != nil {
		t.Fatalf("item read: %v", err)
	}
	if fresh.LifecycleState != memory.StateActive {
		t.Fatalf("the decision changes no item state: %+v", fresh)
	}

	// The validation bounds (table-driven).
	cases := []struct {
		name string
		req  ContextInclusionRequest
		want errs.Code
	}{
		{"missing correlation", ContextInclusionRequest{RequestRef: "r",
			Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: true}}}, errs.CodeValidationFailed},
		{"missing request_ref", ContextInclusionRequest{CorrelationID: "c",
			Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: true}}}, errs.CodeValidationFailed},
		{"no decisions", ContextInclusionRequest{CorrelationID: "c", RequestRef: "r"}, errs.CodeValidationFailed},
		{"duplicate item in one call", ContextInclusionRequest{CorrelationID: "c", RequestRef: "r",
			Decisions: []contextdomain.InclusionDecision{
				{MemoryItemID: item.MemoryItemID, Included: true},
				{MemoryItemID: item.MemoryItemID, Included: false},
			}}, errs.CodeValidationFailed},
		{"unknown item is the SAME typed 404 (the no-leak rule)", ContextInclusionRequest{CorrelationID: "c", RequestRef: "r",
			Decisions: []contextdomain.InclusionDecision{{MemoryItemID: "mem-nope", Included: true}}}, errs.CodeMemoryItemNotFound},
		{"oversized correlation", ContextInclusionRequest{CorrelationID: string(make([]byte, maxContextIDLen+1)), RequestRef: "r",
			Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: true}}}, errs.CodeValidationFailed},
		{"oversized request_ref", ContextInclusionRequest{CorrelationID: "c", RequestRef: string(make([]byte, maxContextIDLen+1)),
			Decisions: []contextdomain.InclusionDecision{{MemoryItemID: item.MemoryItemID, Included: true}}}, errs.CodeValidationFailed},
	}
	for _, tc := range cases {
		_, err := slice.SetContextInclusion(ctx, tc.req)
		assertKnowledgeEnvelope(t, err, tc.want)
	}

	// The state endpoint: the unknown identity is the family's SAME typed
	// 404 (no operation row exists for the M-18 decision — the published
	// law).
	_, err = slice.ContextOperationStatus(ctx, "ctxop-incl-nope")
	assertKnowledgeEnvelope(t, err, errs.CodeContextSnapshotNotFound)
}

// TestContextNilPorts pins the fail-closed nil-port refusals (the
// KnowledgeStore/MemoryStore pattern).
func TestContextNilPorts(t *testing.T) {
	env := newTestEnv(t)
	cdeps := env.deps
	cdeps.Memory = env.store
	cdeps.Context = nil // the context surface is unwired
	cslice, err := NewSlice(cdeps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	ctx := env.ctx()
	_, err = cslice.SaveContextSnapshot(ctx, ContextSaveRequest{IdempotencyKey: "k", CorrelationID: "c", SourceJobID: "j"})
	assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
	_, err = cslice.CompareContextSnapshots(ctx, ContextCompareRequest{BaselineSnapshotID: "b", CandidateSnapshotID: "c"})
	assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
	_, err = cslice.SetContextInclusion(ctx, ContextInclusionRequest{CorrelationID: "c", RequestRef: "r",
		Decisions: []contextdomain.InclusionDecision{{MemoryItemID: "mem-x", Included: true}}})
	assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
	_, err = cslice.ContextOperationStatus(ctx, "ctxop-x")
	assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
}
