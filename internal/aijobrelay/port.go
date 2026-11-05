package aijobrelay

import (
	"context"
	"time"
)

// WorkerCapability declares what one worker can run (C3-16): the scheduler
// never hands a Job to a worker outside its capability/resource profile.
type WorkerCapability struct {
	WorkerID    string
	ProviderIDs []string
	MaxMemoryMB int
}

// RelayStore is the durable execution contract (C3-13/C3-14/C3-15). The
// storage package implements it; every authoritative write is fenced by
// the Job version and lease epoch, and commits are atomic across state,
// receipt, event journal, and outbox.
type RelayStore interface {
	// SubmitJob stores a new Job in Queued state. Idempotency semantics
	// (C3-02): same key + same input digest returns the existing Job;
	// same key + different input digest is a CONFLICT, never a second Job.
	SubmitJob(ctx context.Context, job Job) (*Job, error)
	// LoadJob loads one Job by id.
	LoadJob(ctx context.Context, jobID string) (Job, error)
	// LoadCurrentAttempt loads the Job's current Attempt.
	LoadCurrentAttempt(ctx context.Context, jobID string) (Attempt, error)
	// ClaimNext atomically claims the next runnable Job for a worker with
	// the given capability (FIFO fairness by submission order): creates the
	// Attempt and Lease, bumps the Job version, and returns the Claim.
	// claimed=false means nothing runnable matched.
	ClaimNext(ctx context.Context, worker WorkerCapability, leaseTTL time.Duration) (*Claim, bool, error)
	// RenewLease extends the live lease; an expired or stale lease cannot
	// be renewed back to life (C3-13).
	RenewLease(ctx context.Context, jobID, attemptID, workerID string, leaseEpoch uint64, ttl time.Duration) error
	// ReleaseExpiredLeases resolves every expired lease: the Attempt fails
	// retryable and the Job returns to Queued — unless an Unknown side
	// effect exists, which forces NeedsReconciliation (C3-14). Persisted
	// wall-clock time is diagnostic; the expiry check fences by epoch.
	ReleaseExpiredLeases(ctx context.Context, now time.Time) ([]Job, error)
	// PrepareSideEffect durably records the intent BEFORE any external
	// effect (C3-14).
	PrepareSideEffect(ctx context.Context, intent SideEffectIntent) error
	// ObserveSideEffect records the outcome of a dispatched effect with
	// full fencing (C3-14): Unknown outcomes are preserved as Unknown and
	// move the Job to NeedsReconciliation — never a blind retry.
	ObserveSideEffect(ctx context.Context, jobID, intentID string, jobVersion, leaseEpoch uint64, attemptID, toState, resultDigest string) error
	// CommitAttempt applies one fenced Commit atomically (C3-15): Job
	// transition + Attempt outcome + Receipt + event journal + outbox in
	// one transaction; no event announces an uncommitted result.
	CommitAttempt(ctx context.Context, commit Commit) error
	// RequestCancel moves a Queued Job to Cancelled directly and a
	// Running/Waiting Job to CancelRequested (cooperative cancellation
	// reaches the running handler; the terminal commit resolves it).
	RequestCancel(ctx context.Context, jobID string) error
	// PauseJob moves a Queued/Running/Waiting Job to Paused and releases
	// the lease (pause releases the lease, C3-16).
	PauseJob(ctx context.Context, jobID string) error
	// ResumeJob moves a Paused Job back to Queued for scheduling.
	ResumeJob(ctx context.Context, jobID string) error
	// LoadIntent loads one side-effect intent by id.
	LoadIntent(ctx context.Context, intentID string) (SideEffectIntent, error)
	// HasUnknownSideEffectIntent reports whether the Job carries any
	// side-effect intent whose outcome is Unknown (C3-14): the read-only
	// fact the F9-frozen retry-eligibility read consumes for its
	// reconciliation member (a retry never stands where an Unknown side
	// effect is unresolved: needs reconciliation first).
	HasUnknownSideEffectIntent(ctx context.Context, jobID string) (bool, error)
	// ListJobs returns one bounded page of Jobs in the Core-defined order
	// (created_at descending, job_id ascending tiebreak) with an optional
	// registered-state filter — the card-9 working candidate's server-side
	// cursor pagination (2026-11-02): the keyset position rides
	// AfterCreatedAt/AfterJobID (zero/empty = the first page), the adapter
	// fetches one probe row beyond Limit to report More, and the opaque
	// cursor envelope that carries the position is the use-case layer's
	// (never structured data on the wire).
	ListJobs(ctx context.Context, q ListJobsQuery) (ListJobsPage, error)
	// LoadApproval loads one approval request by its Core-issued id
	// (NOT_FOUND when absent) — the card-14 identity's queryable half.
	LoadApproval(ctx context.Context, approvalID string) (Approval, error)
	// LoadLatestApprovalForJob loads the Job's latest raised approval
	// request (NOT_FOUND when the Job has none) — the query the Job detail
	// inspection consumes.
	LoadLatestApprovalForJob(ctx context.Context, jobID string) (Approval, error)
	// DecideApproval applies one approval decision command (the card-14
	// working candidate) under the fenced single-writer path: a pending
	// approval resolves exactly once to accepted (the Job re-queues:
	// Waiting → Queued) or rejected (the Job's terminal disposition:
	// Waiting → Failed, the register-consistent extension); the Job update
	// is version-fenced and the approval update is state-fenced in ONE
	// transaction. A replayed decision (same decision on an already
	// decided approval) returns the recorded outcome WITHOUT re-applying —
	// no state change, no second event, Replayed=true; a contradicting
	// decision is the typed CONFLICT_STATE. The decision's scope is exactly
	// the presented approval identity bound to the presented Job: an
	// approval bound to another Job is the same typed NOT_FOUND shape as an
	// unknown identity (the no-existence-leak rule).
	DecideApproval(ctx context.Context, dec ApprovalDecision) (ApprovalDecisionResult, error)
	// RetryJob applies one M-7 retry command under the fenced single-writer
	// path (IMPL-21, the F9 family's frozen semantics): a NEW Attempt of
	// the SAME Job — never a new Job — by re-queueing the terminal
	// Failed/TimedOut Job exactly as the reconciliation exit re-queues; the
	// scheduler's next claim creates the new Attempt (never a rewrite). The
	// command is Supplied-keyed: the request carries the operation's
	// idempotency reference, the applied retry records it durably on its
	// re-queue event, and the same reference re-presented returns the
	// recorded outcome WITHOUT re-applying (Replayed=true — no second
	// re-queue, no second event; never regenerated); a contradicting
	// re-issue (a different reference against a standing retry, or a
	// re-presented consumed reference) is the typed CONFLICT_STATE — the
	// coded approval-decision precedent's class. A retried Attempt does not
	// undo the prior Attempt's effects (retry ≠ reversal): no intent, no
	// receipt, no prior record is rewritten. The command's eligibility gate
	// is the SAME truth the eligibility read reports (RetryEligibilityFor —
	// allow only terminal Failed/TimedOut); the storage layer's own
	// transition-table fence is the same allow-set expressed as the table.
	RetryJob(ctx context.Context, req RetryRequest) (RetryOutcome, error)
	// StandingRetryReference reports the Job's most recent recorded retry
	// reference ('' when the Job carries no recorded retry) — the read the
	// command path uses to exempt the Supplied-keyed replay from the
	// fresh-retry eligibility gate (the same operation re-presented is not
	// a fresh retry; whether the recorded outcome still stands is the
	// RetryJob ruling's, which stays authoritative). Read-only; the
	// reference rides the journal's retry events — no schema change.
	StandingRetryReference(ctx context.Context, jobID string) (string, error)
}

// ListJobsQuery is one server-side list query (the card-9 working
// candidate). Status accepts only registry members (RegisteredJobState) —
// the caller validates; the adapter fails closed on an unknown value.
type ListJobsQuery struct {
	Limit          int
	Status         string
	AfterCreatedAt time.Time
	AfterJobID     string
}

// ListJobsPage is one bounded page. More reports that a further page exists
// after the returned Jobs (the adapter's internal limit+1 probe).
type ListJobsPage struct {
	Jobs []Job
	More bool
}
