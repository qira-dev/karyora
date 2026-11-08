package app

import (
	stdcontext "context"

	contextdomain "github.com/qira-dev/karyora/internal/domain/context"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The B3 ContextSnapshot family service (DE-0 queue row 5): the published
// /context route family's use cases, implemented against the canonical
// contract at the working HEAD — the M-18 request-scoped inclusion class
// implemented FULLY (request-scoped only, no Memory action row, no scope
// broadening, the Core-provided-only reason law), and the two CONDITIONAL
// commands serving their typed unmet-condition behaviors exactly as the
// contract's descriptions carry them (never a fake success path): the M-19
// save-snapshot command is CONDITIONAL on durable snapshot storage
// existing (the GC-3 un-gate precondition — the owner's PROPOSED decision,
// unresolved; while unmet no operation row can exist and the typed refusal
// is served), and the M-20 compare is CONDITIONAL on stable Core
// comparison existing (the GC-4 un-gate precondition; while unmet the
// family answers the typed CONTEXT_COMPARE_UNSUPPORTED and no diff is ever
// fabricated). The authoritative state endpoint is the family's source of
// truth (events are never the source of truth); the unknown identity and
// the out-of-scope identity are the SAME typed CONTEXT_SNAPSHOT_NOT_FOUND
// 404 (the no-leak rule). THE F5 NON-DUPLICATION LAW (BINDING): this
// family never duplicates, redefines, or modifies the served Job-flow
// snapshot read (/jobs/{id}/snapshot → SnapshotView, served by
// JobContextSnapshot below on the ResourceJob surface) — the compiled
// content is never carried; the extension composes ADDITIVELY alongside
// the served read. Every protected effect rides the single decision port
// (security.ResourceContext — the additive register extension; Local mode
// stays role-free: no permission policy is invented).

// ContextStore is the durable Context surface port (B3). The storage
// adapter implements it over the context tables (migration 0015); the
// slice consumes only this port. Nil disables the surface — typed
// refusals, fail closed (the KnowledgeStore/MemoryStore pattern).
type ContextStore interface {
	// ApplyContextInclusion applies the M-18 request-scoped ruling (the
	// decision rows' own (request_ref, item) scope IS the key carrier; no
	// operation row, no durable idempotency key).
	ApplyContextInclusion(ctx stdcontext.Context, intent contextdomain.InclusionIntent) (contextdomain.InclusionOutcome, error)
	// LoadContextOperation resolves the authoritative state read; the
	// unknown identity and the out-of-scope identity are the SAME typed
	// CONTEXT_SNAPSHOT_NOT_FOUND 404 (the no-leak rule).
	LoadContextOperation(ctx stdcontext.Context, opID string) (contextdomain.Operation, error)
}

// Context bounds (the coded bounds class: identifiers ride the 128-char
// bound; reference labels ride 512; the decision set is bounded —
// bounded, never unbounded).
const (
	maxContextIDLen     = 128
	maxContextLabelLen  = 512
	MaxContextDecisions = 64
)

// ContextSaveRequest is one save-snapshot body (the published
// ContextSnapshotSaveRequest members as the service contract).
type ContextSaveRequest struct {
	IdempotencyKey string
	CorrelationID  string
	SourceJobID    string
}

// ContextCompareRequest is one compare body (the published
// ContextSnapshotCompareRequest members as the service contract).
type ContextCompareRequest struct {
	BaselineSnapshotID  string
	CandidateSnapshotID string
}

// ContextInclusionRequest is one per-request inclusion body (the
// published ContextInclusionRequest members as the service contract).
type ContextInclusionRequest struct {
	CorrelationID string
	RequestRef    string
	Decisions     []contextdomain.InclusionDecision
}

// contextNilPort is the typed refusal for a nil store (fail closed).
func contextNilPort(corr string) error {
	return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "context store is not wired"})
}

// validateContextIdentity validates one bounded identifier member.
func validateContextIdentity(value, field string, corr string) error {
	if value == "" || len(value) > maxContextIDLen {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": field})
	}
	return nil
}

// SaveContextSnapshot is the M-19 save-snapshot command (CONDITIONAL).
// THE PUBLISHED UNMET-CONDITION BEHAVIOR (carried exactly, never a fake
// success): the command's condition — durable snapshot storage existing —
// is the GC-3 un-gate precondition, the owner's PROPOSED decision, NOT
// resolved; while unmet the command serves the typed unmet-condition
// refusal (the family's honest-unavailable code riding the refusal's
// reason naming the precondition), NO operation row is created (the
// ContextOperationKind register's own law: "no operation row can exist"
// while unsupported), and no snapshot.saved event exists. The refusal
// rides the register's published retryable assignment. There is no
// replay path, no 202, and no fabricated result anywhere: the typed
// refusal IS the served behavior until the owner's decision resolves
// (flipping the gate is the un-gate act, its own LIVE-gate class).
func (s *Slice) SaveContextSnapshot(ctx stdcontext.Context, req ContextSaveRequest) (contextdomain.SnapshotSaveResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Context == nil {
		return contextdomain.SnapshotSaveResult{}, contextNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceContext}, security.ActionDataWrite); err != nil {
		return contextdomain.SnapshotSaveResult{}, err
	}
	// The bounded-identity law rides every surface (ahead of the gate —
	// the same order the compare member carries).
	if err := validateContextIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return contextdomain.SnapshotSaveResult{}, err
	}
	if err := validateContextIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return contextdomain.SnapshotSaveResult{}, err
	}
	if err := validateContextIdentity(req.SourceJobID, "source_job_id", corr); err != nil {
		return contextdomain.SnapshotSaveResult{}, err
	}
	if contextdomain.DurableSnapshotStorageSupported {
		// UNREACHABLE while the gate stands: the gate constant is the
		// family's own support state (the owner's PROPOSED decision,
		// unresolved) and no durable snapshot storage exists behind it.
		// Reaching this branch would be a coding error against the
		// recorded gate — fail closed, never a fabricated success.
		return contextdomain.SnapshotSaveResult{}, envelopeErr(errs.CodeInternal, corr, map[string]any{
			"reason": "the durable-storage gate is recorded unsupported; no save producer exists behind it",
		})
	}
	return contextdomain.SnapshotSaveResult{}, envelopeErr(errs.CodeContextCompilationUnavailable, corr, map[string]any{
		"reason":       "durable snapshot storage does not exist (the GC-3 un-gate precondition — the owner's PROPOSED decision, unresolved); the save-snapshot command cannot be served and no operation row can exist while the condition stands",
		"precondition": "GC-3",
	})
}

// CompareContextSnapshots is the M-20 snapshot compare (CONDITIONAL,
// read-only). THE PUBLISHED UNMET-CONDITION BEHAVIOR (the contract's own
// law, carried exactly): where stable Core comparison does not exist the
// family answers the typed CONTEXT_COMPARE_UNSUPPORTED — the
// comparison-unavailable state renders honestly and no diff is ever
// fabricated. The refusal mutates nothing (the read-only class; safe to
// re-issue as a read-only effect-free read while the outcome persists
// until support exists — the GC-4 law) and rides the register's published
// safe-retry assignment.
func (s *Slice) CompareContextSnapshots(ctx stdcontext.Context, req ContextCompareRequest) (contextdomain.SnapshotCompareResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Context == nil {
		return contextdomain.SnapshotCompareResult{}, contextNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceContext}, security.ActionDataRead); err != nil {
		return contextdomain.SnapshotCompareResult{}, err
	}
	if err := validateContextIdentity(req.BaselineSnapshotID, "baseline_snapshot_id", corr); err != nil {
		return contextdomain.SnapshotCompareResult{}, err
	}
	if err := validateContextIdentity(req.CandidateSnapshotID, "candidate_snapshot_id", corr); err != nil {
		return contextdomain.SnapshotCompareResult{}, err
	}
	if contextdomain.StableComparisonSupported {
		// UNREACHABLE while the gate stands: no stable Core comparison
		// exists behind the recorded support state — fail closed, never a
		// fabricated diff.
		return contextdomain.SnapshotCompareResult{}, envelopeErr(errs.CodeInternal, corr, map[string]any{
			"reason": "the comparison gate is recorded unsupported; no comparison producer exists behind it",
		})
	}
	return contextdomain.SnapshotCompareResult{}, envelopeErr(errs.CodeContextCompareUnsupported, corr, map[string]any{
		"reason":       "no stable Core comparison exists (the GC-4 un-gate precondition); the comparison-unavailable state is served honestly and no diff is fabricated",
		"precondition": "GC-4",
	})
}

// SetContextInclusion sets the stated request's per-item memory-inclusion
// decisions (the M-18 class, UNCONDITIONAL — implemented fully). The
// decision applies ONLY to the stated request carried by request_ref — it
// changes no Memory item's lifecycle state, creates no Memory action row
// (the P4.6 §4 boundary — the Inspector renders only what the Core's
// context policy exposes about inclusion), and never widens any scope.
// The retention is request-lifetime at most (the decision row's own
// lifetime — recorded honestly). Each decided item's existence is
// resolved through the wired Memory port's authoritative read (the same
// typed 404 the Memory family's inspect read serves — the no-leak rule);
// the decision is a read of the item, never a write. The result's reason
// members are never set: the Core-provided-only reason law — no reason
// producer exists on this surface, and absence is the absence, never a
// synthesized reason.
func (s *Slice) SetContextInclusion(ctx stdcontext.Context, req ContextInclusionRequest) (contextdomain.InclusionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Context == nil {
		return contextdomain.InclusionResult{}, contextNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceContext}, security.ActionDataWrite); err != nil {
		return contextdomain.InclusionResult{}, err
	}
	if s.deps.Memory == nil {
		// The decisions concern Memory items: the wired Memory port's
		// authoritative read resolves each item's existence (a read, never
		// a write). A nil Memory port is the fail-closed refusal.
		return contextdomain.InclusionResult{}, memoryNilPort(corr)
	}
	if err := validateContextIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return contextdomain.InclusionResult{}, err
	}
	if err := validateContextIdentity(req.RequestRef, "request_ref", corr); err != nil {
		return contextdomain.InclusionResult{}, err
	}
	if len(req.Decisions) == 0 || len(req.Decisions) > MaxContextDecisions {
		return contextdomain.InclusionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "decisions"})
	}
	seen := make(map[string]bool, len(req.Decisions))
	for _, d := range req.Decisions {
		if err := validateContextIdentity(d.MemoryItemID, "memory_item_id", corr); err != nil {
			return contextdomain.InclusionResult{}, err
		}
		if seen[d.MemoryItemID] {
			return contextdomain.InclusionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
				"field": "decisions", "reason": "the same memory item is decided twice in one request",
			})
		}
		seen[d.MemoryItemID] = true
		// The decided item's existence resolves through the Memory port's
		// authoritative read (the SAME typed 404 the Memory family's
		// inspect read serves — the no-leak rule). The decision touches no
		// item state (the M-18 boundary).
		if _, err := s.deps.Memory.LoadMemoryItem(ctx, d.MemoryItemID); err != nil {
			return contextdomain.InclusionResult{}, err
		}
	}
	outcome, err := s.deps.Context.ApplyContextInclusion(ctx, contextdomain.InclusionIntent{
		CorrelationID: req.CorrelationID,
		RequestRef:    req.RequestRef,
		Decisions:     req.Decisions,
	})
	if err != nil {
		return contextdomain.InclusionResult{}, err
	}
	return outcome.Result, nil
}

// ContextOperationStatus is the family's authoritative state endpoint (a
// pure query — never a retry): resolved by the operation identity; a lost
// response is NEVER proof of success or safe retry — reconcile by the
// operation ID through this read BEFORE any retry (the
// reconcile-by-identity law). The unknown identity and the out-of-scope
// identity are the SAME typed CONTEXT_SNAPSHOT_NOT_FOUND 404 (the
// no-leak rule).
func (s *Slice) ContextOperationStatus(ctx stdcontext.Context, opID string) (contextdomain.Operation, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Context == nil {
		return contextdomain.Operation{}, contextNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceContext, ID: opID}, security.ActionDataRead); err != nil {
		return contextdomain.Operation{}, err
	}
	if err := validateContextIdentity(opID, "operation_id", corr); err != nil {
		return contextdomain.Operation{}, err
	}
	return s.deps.Context.LoadContextOperation(ctx, opID)
}
