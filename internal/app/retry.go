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
// nothing, and is NOT the retry command: the M-7 command's identity
// stays its recorded Open item (the S7 §5 gap 2; O-1), so no eligibility
// response is an authorization to execute anything. The no-leak rule
// binds the refusals: an unknown identity is the same typed NOT_FOUND
// shape as an inaccessible one (the decision-port denial), never a
// distinguishing answer.
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
