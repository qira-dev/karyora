package app

import (
	"context"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// RetryEligibility reports the F9-frozen retry-eligibility read for ONE
// Job (the P3.2 §3.9 [P] typed-member set as frozen at the S4 §15 freeze
// act — the adopted P-S7-01…06 supply): a Core query over Core-owned
// Job/eligibility state, server-side, permission-filtered through the one
// decision port (the coded ActionDataRead posture every read decides
// through). The read returns exactly the four recorded typed members —
// the eligibility verdict, the side-effect risk statement, the
// idempotency guarantee class, the reconciliation requirement — and
// nothing beyond what the Core returns. It mutates nothing, creates
// nothing, and executes nothing: a read's allow verdict authorizes no
// execution — the retry is the command below (its own write path, its own
// authorization). The no-leak rule binds the refusals: an unknown
// identity is the same typed NOT_FOUND shape as an inaccessible one (the
// decision-port denial), never a distinguishing answer.
func (s *Slice) RetryEligibility(ctx context.Context, jobID string) (aijobrelay.RetryEligibility, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataRead); err != nil {
		return aijobrelay.RetryEligibility{}, err
	}
	job, err := s.deps.Relay.LoadJob(ctx, jobID)
	if err != nil {
		return aijobrelay.RetryEligibility{}, err
	}
	unknown, err := s.deps.Relay.HasUnknownSideEffectIntent(ctx, jobID)
	if err != nil {
		return aijobrelay.RetryEligibility{}, err
	}
	return aijobrelay.RetryEligibilityFor(job.State, unknown), nil
}

// RetryJob is the M-7 retry command (IMPL-21, the F9 family's frozen
// semantics): a NEW Attempt of the SAME Job — never a new Job — applied
// only where the Core reports eligibility. A retry is a WRITE, so the
// command decides through the one decision port with the coded
// ActionDataWrite posture every command decides through (ResourceJob;
// audited through the Guard path like every command). The eligibility
// gate is the SAME truth the read reports — RetryEligibilityFor itself,
// never a second rule forked for the command path. The command carries
// its Supplied-keyed idempotency reference (the frozen law): the same
// reference re-presented returns the original result, replayed, never
// regenerated; a contradicting re-issue is the typed CONFLICT_STATE (the
// coded approval-decision precedent's class). UnknownOutcome is never
// blindly retried: NeedsReconciliation denies through the same verdict;
// no auto-retry exists anywhere on this path. The no-leak rule binds the
// refusals exactly as the read's: an unknown identity is the typed
// NOT_FOUND, never a distinguishing answer.
func (s *Slice) RetryJob(ctx context.Context, jobID, idempotencyReference string) (aijobrelay.RetryOutcome, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataWrite); err != nil {
		return aijobrelay.RetryOutcome{}, err
	}
	if !aijobrelay.ValidRetryReference(idempotencyReference) {
		return aijobrelay.RetryOutcome{}, envelopeErr(errs.CodeValidationFailed, corr,
			map[string]any{"field": "idempotency_reference"})
	}
	job, err := s.deps.Relay.LoadJob(ctx, jobID)
	if err != nil {
		return aijobrelay.RetryOutcome{}, err
	}
	unknown, err := s.deps.Relay.HasUnknownSideEffectIntent(ctx, jobID)
	if err != nil {
		return aijobrelay.RetryOutcome{}, err
	}
	// THE one eligibility truth: the command's gate IS the read's verdict
	// object (P-S7-02's consumption law — the command acts only where the
	// Core reports eligibility). A reconciliation requirement reported
	// alongside an allow verdict rides the outcome (retry ≠ reversal); it
	// does not widen or narrow the verdict (the IMPL-20 typing, carried).
	//
	// The Supplied-keyed replay is exempt from the FRESH-retry gate — the
	// coded approval-decision precedent's order: the replay ruling comes
	// first, because re-presenting the SAME reference is the SAME operation
	// returning its recorded outcome (no new execution is created, so the
	// fresh-retry eligibility gate is not what governs it — the write
	// authorization above and the store's own replay ruling are). Whether
	// the recorded outcome still stands is the store's RetryJob ruling,
	// which stays authoritative: a re-presented consumed reference reaches
	// the store and refuses there (the typed CONFLICT_STATE).
	if !aijobrelay.RetryEligibilityFor(job.State, unknown).Eligible {
		standing, serr := s.deps.Relay.StandingRetryReference(ctx, jobID)
		if serr != nil {
			return aijobrelay.RetryOutcome{}, serr
		}
		if standing != idempotencyReference {
			return aijobrelay.RetryOutcome{}, envelopeErr(errs.CodeConflictState, corr,
				map[string]any{"reason": "job is not retry-eligible in state " + job.State})
		}
	}
	return s.deps.Relay.RetryJob(ctx, aijobrelay.RetryRequest{
		JobID:                jobID,
		IdempotencyReference: idempotencyReference,
		CorrelationID:        corr,
	})
}
