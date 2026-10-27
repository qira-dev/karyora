package aijobrelay

import (
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// Requirements is the capability requirement a Job places on its worker
// (C3-16): an empty ProviderID means any worker; unsupported capabilities
// are never scheduled.
type Requirements struct {
	ProviderID  string `json:"provider_id,omitempty"`
	MinMemoryMB int    `json:"min_memory_mb,omitempty"`
}

// Job is the durable execution record (C3-12). Version is the monotonic
// Job fencing token; every authoritative commit carries the version the
// writer observed, and a stale version is rejected (C3-13).
type Job struct {
	JobID          string       `json:"job_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	InputDigest    string       `json:"input_digest"`
	State          string       `json:"state"`
	Version        uint64       `json:"version"`
	AttemptCount   int          `json:"attempt_count"`
	WaitingReason  string       `json:"waiting_reason,omitempty"`
	Requirements   Requirements `json:"requirements"`
	DeadlineSecs   int64        `json:"deadline_secs,omitempty"`
	ResultDigest   string       `json:"result_digest,omitempty"`
	ReceiptID      string       `json:"receipt_id,omitempty"`
	CorrelationID  string       `json:"correlation_id,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// Attempt is one bounded execution of a Job (C3-12/C3-13). LeaseEpoch is
// the monotonic lease fencing token bound into every authoritative write.
type Attempt struct {
	AttemptID  string    `json:"attempt_id"`
	JobID      string    `json:"job_id"`
	Seq        int       `json:"seq"`
	WorkerID   string    `json:"worker_id"`
	LeaseID    string    `json:"lease_id"`
	LeaseEpoch uint64    `json:"lease_epoch"`
	Outcome    string    `json:"outcome"`
	StartedAt  time.Time `json:"started_at"`
	EndsAt     time.Time `json:"ends_at,omitempty"`
}

// SideEffectIntent is the durable intent recorded BEFORE any external
// effect (C3-14): input reference/digest, the operation idempotency key,
// and the provider identity. Observations land with their own fencing.
type SideEffectIntent struct {
	IntentID       string    `json:"intent_id"`
	JobID          string    `json:"job_id"`
	AttemptID      string    `json:"attempt_id"`
	Operation      string    `json:"operation"`
	IdempotencyKey string    `json:"idempotency_key"`
	InputDigest    string    `json:"input_digest"`
	State          string    `json:"state"`
	ProviderRef    string    `json:"provider_ref,omitempty"`
	ResultDigest   string    `json:"result_digest,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
}

// Receipt is the durable proof that an Attempt reached an outcome
// (C3-12/C3-15): it carries the fencing values that authorized the commit.
type Receipt struct {
	ReceiptID    string    `json:"receipt_id"`
	JobID        string    `json:"job_id"`
	AttemptID    string    `json:"attempt_id"`
	Outcome      string    `json:"outcome"`
	ResultDigest string    `json:"result_digest,omitempty"`
	JobVersion   uint64    `json:"job_version"`
	LeaseEpoch   uint64    `json:"lease_epoch"`
	CreatedAt    time.Time `json:"created_at"`
}

// Claim is the result of an atomic claim (C3-13): exactly one current
// owner per Job, with the fresh lease bound to the new Attempt.
type Claim struct {
	Job     Job     `json:"job"`
	Attempt Attempt `json:"attempt"`
}

// Commit is one fenced authoritative write (C3-13/C3-15): the storage
// adapter must apply the Job transition, the Attempt outcome, the receipt,
// and the event journal/outbox rows in ONE transaction, rejecting stale
// versions and stale lease epochs.
type Commit struct {
	JobID          string
	JobVersion     uint64
	AttemptID      string
	WorkerID       string
	LeaseID        string
	LeaseEpoch     uint64
	ToState        string
	WaitingReason  string
	AttemptOutcome string
	ResultDigest   string
	ReceiptID      string
	CorrelationID  string
	Event          events.Record
}

// ValidateSubmit enforces the submission contract (C3-12/C3-02): identity,
// idempotency key, canonical input digest, and the initial state.
func ValidateSubmit(job *Job) error {
	if job.JobID == "" {
		return fmt.Errorf("aijobrelay: job_id is required")
	}
	if job.IdempotencyKey == "" {
		return fmt.Errorf("aijobrelay: idempotency_key is required (same key + same input replays the original; same key + different input is a conflict)")
	}
	if !digestGrammar(job.InputDigest) {
		return fmt.Errorf("aijobrelay: input_digest must match the sha256 grammar")
	}
	if job.State != JobCreated {
		return fmt.Errorf("aijobrelay: a submission starts in state %s", JobCreated)
	}
	if job.DeadlineSecs < 0 {
		return fmt.Errorf("aijobrelay: deadline cannot be negative")
	}
	if job.Version != 1 || job.AttemptCount != 0 {
		return fmt.Errorf("aijobrelay: a fresh job carries version 1 and no attempts")
	}
	return nil
}

// CheckCommit validates a fenced commit against the current stored Job
// (C3-13 acceptance): current version, current attempt, legal transition,
// terminal guard, and outcome/state consistency. Persisted wall-clock time
// is never proof of liveness — the lease epoch is.
func CheckCommit(commit Commit, current Job, currentAttempt Attempt) error {
	if err := CheckTerminalJob(current.State); err != nil {
		return err
	}
	if commit.JobVersion != current.Version {
		return fmt.Errorf("aijobrelay: stale Job version %d (current %d); fenced write refused", commit.JobVersion, current.Version)
	}
	if commit.AttemptID != currentAttempt.AttemptID {
		return fmt.Errorf("aijobrelay: stale attempt %q (current %q); fenced write refused", commit.AttemptID, currentAttempt.AttemptID)
	}
	if commit.LeaseEpoch != currentAttempt.LeaseEpoch || commit.WorkerID != currentAttempt.WorkerID {
		return fmt.Errorf("aijobrelay: stale lease epoch/worker; fenced write refused")
	}
	if !CanTransition(current.State, commit.ToState) {
		return fmt.Errorf("aijobrelay: illegal transition %s → %s", current.State, commit.ToState)
	}
	switch commit.ToState {
	case JobSucceeded:
		if commit.AttemptOutcome != AttemptSucceeded || !digestGrammar(commit.ResultDigest) || commit.ReceiptID == "" {
			return fmt.Errorf("aijobrelay: a succeeded commit requires a successful attempt, a result digest, and a receipt")
		}
	case JobFailed, JobTimedOut:
		if commit.AttemptOutcome != AttemptFailedTerminal {
			return fmt.Errorf("aijobrelay: a terminal failure requires a terminal attempt outcome")
		}
	case JobWaiting:
		if WaitingReason(commit.WaitingReason) == "" {
			return fmt.Errorf("aijobrelay: a Waiting job must carry a typed reason")
		}
	}
	return nil
}

// CheckObservation validates a side-effect observation (C3-14): legal
// intent transition, fencing against the owning attempt, and the Unknown
// rule — an Unknown observation is recorded as Unknown, never as success.
func CheckObservation(intent SideEffectIntent, jobVersion uint64, attemptID string, leaseEpoch uint64, toState string, currentJob Job) error {
	if jobVersion != currentJob.Version {
		return fmt.Errorf("aijobrelay: stale Job version %d (current %d); observation refused", jobVersion, currentJob.Version)
	}
	if intent.AttemptID != attemptID || leaseEpoch == 0 {
		return fmt.Errorf("aijobrelay: observation is not fenced to the owning attempt")
	}
	if intent.State == SideEffectUnknown && toState == SideEffectSucceeded && intent.ResultDigest == "" {
		return fmt.Errorf("aijobrelay: an Unknown effect cannot become a success without provider-verified reconciliation")
	}
	if !CanTransitionSideEffect(intent.State, toState) {
		return fmt.Errorf("aijobrelay: illegal side-effect transition %s → %s", intent.State, toState)
	}
	return nil
}

func digestGrammar(s string) bool {
	const prefix = "sha256:"
	if len(s) != len(prefix)+64 {
		return false
	}
	for _, c := range s[len(prefix):] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
