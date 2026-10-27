package aijobrelay

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Scheduler is the bounded local scheduler (C3-16): bounded goroutines,
// per-Job deadlines, cooperative pause/cancel propagation, FIFO fairness
// (the store claims in submission order), and safe shutdown. It owns no
// execution truth — every outcome goes back through the fenced store.
type Scheduler struct {
	store         RelayStore
	handler       Handler
	maxConcurrent int
	leaseTTL      time.Duration
	worker        WorkerCapability

	mu      sync.Mutex
	running map[string]context.CancelFunc
	wg      sync.WaitGroup
	stopped bool
}

// Handler executes one claimed Job and returns the fenced Commit that
// records its outcome. The handler must be cooperative: it watches the
// context for pause/cancel/deadline and returns the matching Commit.
type Handler func(ctx context.Context, claim Claim) (Commit, error)

// NewScheduler builds a bounded scheduler for one worker capability.
func NewScheduler(store RelayStore, worker WorkerCapability, handler Handler, maxConcurrent int, leaseTTL time.Duration) *Scheduler {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	return &Scheduler{
		store:         store,
		handler:       handler,
		maxConcurrent: maxConcurrent,
		leaseTTL:      leaseTTL,
		worker:        worker,
		running:       make(map[string]context.CancelFunc),
	}
}

// Tick performs one bounded scheduling pass: it claims while slots are
// free and the store offers runnable Jobs, and dispatches each claim to a
// bounded goroutine. It returns how many Jobs were dispatched.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return 0, fmt.Errorf("aijobrelay: scheduler is stopped")
	}
	s.mu.Unlock()
	dispatched := 0
	for {
		s.mu.Lock()
		free := s.maxConcurrent - len(s.running)
		s.mu.Unlock()
		if free <= 0 {
			return dispatched, nil
		}
		claim, claimed, err := s.store.ClaimNext(ctx, s.worker, s.leaseTTL)
		if err != nil {
			return dispatched, err
		}
		if !claimed {
			return dispatched, nil
		}
		if err := s.launch(claim); err != nil {
			return dispatched, err
		}
		dispatched++
	}
}

// launch runs one claim on its own bounded goroutine. The context deadline
// is the tighter of the Job deadline and the lease TTL (Attempt timeout,
// Job deadline, and lease liveness stay distinct values in the store).
func (s *Scheduler) launch(claim *Claim) error {
	timeout := s.leaseTTL
	if dl := time.Duration(claim.Job.DeadlineSecs) * time.Second; dl > 0 && dl < timeout {
		timeout = dl
	}
	base, cancel := context.WithCancel(context.Background())
	var deadlineCancel context.CancelFunc
	if timeout > 0 {
		base, deadlineCancel = context.WithTimeout(base, timeout)
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		cancel()
		if deadlineCancel != nil {
			deadlineCancel()
		}
		return fmt.Errorf("aijobrelay: scheduler is stopped")
	}
	s.running[claim.Job.JobID] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.running, claim.Job.JobID)
			s.mu.Unlock()
			cancel()
			if deadlineCancel != nil {
				deadlineCancel()
			}
		}()
		commit, err := s.handler(base, *claim)
		if err != nil {
			// A handler error is a retryable Attempt failure with a typed
			// wait: never a silent drop, never a terminal state by guess.
			commit = Commit{
				JobID: claim.Job.JobID, JobVersion: claim.Job.Version,
				AttemptID: claim.Attempt.AttemptID, WorkerID: claim.Attempt.WorkerID,
				LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
				ToState: JobWaiting, WaitingReason: string(WaitBackoffScheduled),
				AttemptOutcome: AttemptFailedRetryable,
				CorrelationID:  claim.Job.CorrelationID,
			}
		}
		// The fenced store rejects this write when the lease expired or a
		// newer authority exists; the outcome then belongs to recovery.
		_ = s.store.CommitAttempt(base, commit)
	}()
	return nil
}

// CancelRunning cooperatively cancels the in-flight handler of one Job
// (C3-16): cancellation reaches provider/DB calls through the context;
// the terminal commit resolves the state legally.
func (s *Scheduler) CancelRunning(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, ok := s.running[jobID]; ok {
		cancel()
		return true
	}
	return false
}

// Running reports how many Jobs are currently dispatched.
func (s *Scheduler) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// Stop performs the safe shutdown (C3-16): no new dispatches, cooperative
// cancellation of everything in flight, and a wait for all goroutines.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	for _, cancel := range s.running {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
