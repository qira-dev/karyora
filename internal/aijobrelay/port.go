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
}
