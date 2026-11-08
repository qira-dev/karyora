package storage

import (
	stdcontext "context"
	"testing"
	"time"

	contextdomain "github.com/qira-dev/karyora/internal/domain/context"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/memory"
)

// The B3 ContextSnapshot family's store register (DE-0 queue row 5): the
// M-18 request-scoped inclusion ruling (the decision rows' own
// (request_ref, item) scope IS the key carrier — no operation row, no
// durable idempotency key), the authoritative state endpoint read (the
// SAME typed CONTEXT_SNAPSHOT_NOT_FOUND for unknown and out-of-scope —
// the no-leak rule), and the CONDITIONAL law's structural absence (no
// save producer and no durable snapshot storage exist while the owner's
// PROPOSED decisions stand — nothing to test beyond the register's
// absence, never fabricated).

// contextInclusionIntent builds a legal M-18 inclusion intent.
func contextInclusionIntent(correlation, requestRef string, decisions ...contextdomain.InclusionDecision) contextdomain.InclusionIntent {
	return contextdomain.InclusionIntent{
		CorrelationID: correlation,
		RequestRef:    requestRef,
		Decisions:     decisions,
	}
}

// contextFixtureItem inserts one active memory item directly (the
// decision's existence check lives in the service layer; the store ruling
// records the decision rows only — the fixture keeps the store test
// self-contained).
func contextFixtureItem(t *testing.T, s *Store, itemID string) {
	t.Helper()
	if err := s.CommitStateChange(stdcontext.Background(), "corr-context-fixture", func(tx *sqlTx) error {
		return insertMemoryItem(tx, memory.Item{
			MemoryItemID:   itemID,
			Scope:          memory.ScopeUser,
			Origin:         memory.OriginUserEntered,
			LifecycleState: memory.StateActive,
			Content:        "context inclusion fixture content",
		}, time.Now())
	}); err != nil {
		t.Fatalf("fixture item: %v", err)
	}
}

// TestContextInclusionM18 pins the M-18 request-scoped class at the store:
// the same (request_ref, item) row under the SAME correlation replays the
// recorded decision; a different correlation under the same request is the
// caller's NEW decision record (supersedes within the request); the
// decision changes no Memory item's lifecycle state and creates no Memory
// action row (the P4.6 §4 boundary); no operation row exists for the
// decision (the published law: the inclusion decision carries NO durable
// operation row); a multi-decision call is one atomic confirmed set.
func TestContextInclusionM18(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()
	contextFixtureItem(t, s, "mem-ctx-1")
	contextFixtureItem(t, s, "mem-ctx-2")

	// The fresh decision: the Core-confirmed decision record for the
	// stated request.
	first, err := s.ApplyContextInclusion(ctx, contextInclusionIntent("corr-ctx-1", "req-ctx-1",
		contextdomain.InclusionDecision{MemoryItemID: "mem-ctx-1", Included: true}))
	if err != nil {
		t.Fatalf("fresh inclusion: %v", err)
	}
	if first.Result.RequestRef != "req-ctx-1" || len(first.Result.Decisions) != 1 {
		t.Fatalf("the decision record: %+v", first.Result)
	}
	if !first.Result.Decisions[0].Included || first.Result.Decisions[0].MemoryItemID != "mem-ctx-1" {
		t.Fatalf("the confirmed decision: %+v", first.Result.Decisions[0])
	}
	if first.Result.Decisions[0].Reason != nil {
		t.Fatalf("the reason is the Core's alone — no reason producer exists, so it is never set (the Core-provided-only law): %+v", *first.Result.Decisions[0].Reason)
	}

	// The same request identity: replay the recorded decision.
	replay, err := s.ApplyContextInclusion(ctx, contextInclusionIntent("corr-ctx-1", "req-ctx-1",
		contextdomain.InclusionDecision{MemoryItemID: "mem-ctx-1", Included: false}))
	if err != nil {
		t.Fatalf("replay inclusion: %v", err)
	}
	if len(replay.Result.Decisions) != 1 || !replay.Result.Decisions[0].Included {
		t.Fatalf("the replay carries the RECORDED decision, never the re-issued body's value: %+v", replay.Result.Decisions)
	}

	// A different correlation under the same request: the caller's NEW
	// decision record — supersedes within the request.
	supersede, err := s.ApplyContextInclusion(ctx, contextInclusionIntent("corr-ctx-2", "req-ctx-1",
		contextdomain.InclusionDecision{MemoryItemID: "mem-ctx-1", Included: false}))
	if err != nil {
		t.Fatalf("supersede inclusion: %v", err)
	}
	if len(supersede.Result.Decisions) != 1 || supersede.Result.Decisions[0].Included {
		t.Fatalf("the new decision record carries the caller's decision: %+v", supersede.Result.Decisions)
	}

	// The item's state is untouched (the M-18 boundary: no item-state
	// change, no Memory action row).
	item, err := s.LoadMemoryItem(ctx, "mem-ctx-1")
	if err != nil {
		t.Fatalf("item read: %v", err)
	}
	if item.LifecycleState != memory.StateActive {
		t.Fatalf("the decision changes no Memory item state: %+v", item)
	}

	// The multi-decision call is one atomic record set (one call, one
	// confirmed set, one row per (request, item) pair).
	multi, err := s.ApplyContextInclusion(ctx, contextInclusionIntent("corr-ctx-3", "req-ctx-2",
		contextdomain.InclusionDecision{MemoryItemID: "mem-ctx-1", Included: true},
		contextdomain.InclusionDecision{MemoryItemID: "mem-ctx-2", Included: false}))
	if err != nil {
		t.Fatalf("multi inclusion: %v", err)
	}
	if len(multi.Result.Decisions) != 2 {
		t.Fatalf("the multi-decision record: %+v", multi.Result)
	}

	// The decision row count: exactly one row per (request, item) pair —
	// the request-scoped key carrier, never an operation row.
	var inclusions int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM context_inclusions`).Scan(&inclusions); err != nil {
		t.Fatalf("inclusion count: %v", err)
	}
	if inclusions != 3 {
		t.Fatalf("the decision rows: %d (req-ctx-1's item row + req-ctx-2's two item rows)", inclusions)
	}

	// The M-18 decision creates NO durable operation row (the published
	// law) — the operations table holds nothing.
	var operations int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM context_operations`).Scan(&operations); err != nil {
		t.Fatalf("operation count: %v", err)
	}
	if operations != 0 {
		t.Fatalf("the M-18 decision creates no operation row (the published law): %d", operations)
	}

	// Validation: the intent lacks its identity members → the typed
	// refusal.
	if _, err := s.ApplyContextInclusion(ctx, contextdomain.InclusionIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
	if _, err := s.ApplyContextInclusion(ctx, contextInclusionIntent("corr-x", "req-x")); err == nil {
		t.Fatalf("a decision-less intent must refuse")
	}
}

// TestContextOperationStatus404 pins the family's authoritative state
// endpoint at the store: the unknown identity and the out-of-scope
// identity are the SAME typed CONTEXT_SNAPSHOT_NOT_FOUND (the no-leak
// rule; the typed-404 pattern is the published precedent). The M-19
// save command is CONDITIONAL on the unmet durable-storage decision — no
// operation row can exist while the gate stands, so every identity
// answers the typed 404 today; the machinery is the authoritative read
// for the day the condition resolves, never a fabricated record.
func TestContextOperationStatus404(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()
	_, err := s.LoadContextOperation(ctx, "ctxop-nope")
	if err == nil {
		t.Fatalf("the unknown operation must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeContextSnapshotNotFound {
		t.Fatalf("expected CONTEXT_SNAPSHOT_NOT_FOUND, got %v", err)
	}
	// The same typed 404 for any other unknown identity (the no-leak
	// rule).
	_, err = s.LoadContextOperation(ctx, "ctxop-also-nope")
	if err == nil {
		t.Fatalf("the unknown operation must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeContextSnapshotNotFound {
		t.Fatalf("expected CONTEXT_SNAPSHOT_NOT_FOUND, got %v", err)
	}
}
