package app

import (
	"context"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
)

// RunToSettlement drives one Job to a terminal state with a bounded
// scheduler (C3-16): lease-expiry sweeps and backoff requeues use the
// accepted transitions, ticks are bounded, and the scheduler stops
// deterministically. It is the in-process execution side of the vertical
// slice — the durable truth stays in the store, never in this loop.
func RunToSettlement(ctx context.Context, deps SliceDeps, workerID string, maxConcurrent int, jobID string, maxTicks int) (aijobrelay.Job, error) {
	if maxTicks <= 0 {
		maxTicks = 64
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	handler := NewRelayHandler(deps)
	worker := aijobrelay.WorkerCapability{
		WorkerID:    workerID,
		ProviderIDs: wiredProviders(deps),
		MaxMemoryMB: 1024,
	}
	sched := aijobrelay.NewScheduler(deps.Relay, worker, handler.Handle, maxConcurrent, 30*time.Second)
	defer sched.Stop()

	var last aijobrelay.Job
	for i := 0; i < maxTicks; i++ {
		// Recovery sweeps first: expired leases requeue; backoff waits
		// return to Queued through the accepted transition.
		_, _ = deps.Relay.ReleaseExpiredLeases(ctx, time.Now())
		if deps.Backoff != nil {
			_, _ = deps.Backoff.RequeueBackoff(ctx, time.Now())
		}
		if _, err := sched.Tick(ctx); err != nil {
			return last, err
		}
		job, err := deps.Relay.LoadJob(ctx, jobID)
		if err != nil {
			return last, err
		}
		last = job
		if aijobrelay.TerminalJob(job.State) {
			return job, nil
		}
		// Cooperative cancellation: a CancelRequested job's in-flight
		// handler is canceled through the scheduler; the handler resolves
		// the interrupted outcome with fresh fencing.
		if job.State == aijobrelay.JobCancelRequested && sched.Running() > 0 {
			sched.CancelRunning(jobID)
		}
		// Yield briefly every pass so wall-clock-driven transitions
		// (lease expiry, cancellation) can surface.
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
	return last, nil
}

func wiredProviders(deps SliceDeps) []string {
	ids := make([]string, 0, len(deps.Providers))
	for id := range deps.Providers {
		ids = append(ids, id)
	}
	return ids
}
