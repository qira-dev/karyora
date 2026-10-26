package storage

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// writerGate is the C2-10 §2 writer admission: exactly one active writer
// (SQLite's own limit), a bounded admission queue, and a per-request
// deadline. busy_timeout remains a backstop, never the admission strategy.
// Rejections surface as bounded RATE_LIMITED errors, never silent drops.
type writerGate struct {
	queue chan struct{}
	slots chan struct{}

	deadline time.Duration

	active     atomic.Bool
	rejected   atomic.Int64
	lastWaitNs atomic.Int64
}

func newWriterGate(depth int, deadline time.Duration) *writerGate {
	return &writerGate{
		queue:    make(chan struct{}, depth),
		slots:    make(chan struct{}, 1),
		deadline: deadline,
	}
}

// Acquire admits one writer: the bounded queue takes the request first
// (full queue → immediate RATE_LIMITED rejection), then the single slot is
// taken under the per-request deadline. The returned release function must
// be called exactly once.
func (g *writerGate) Acquire(ctx context.Context) (func(), error) {
	select {
	case g.queue <- struct{}{}:
	default:
		g.rejected.Add(1)
		return nil, errStoragef(errs.CodeRateLimited, "writer queue full (depth %d)", cap(g.queue))
	}
	defer func() { <-g.queue }()

	deadlineCtx, cancel := context.WithTimeout(ctx, g.deadline)
	defer cancel()
	start := time.Now()
	select {
	case g.slots <- struct{}{}:
		g.lastWaitNs.Store(int64(time.Since(start)))
		g.active.Store(true)
		release := func() {
			g.active.Store(false)
			<-g.slots
		}
		return release, nil
	case <-deadlineCtx.Done():
		g.rejected.Add(1)
		if ctx.Err() != nil {
			return nil, errStoragef(errs.CodeRateLimited, "writer admission cancelled")
		}
		return nil, errStoragef(errs.CodeRateLimited, "writer admission deadline exceeded (%s)", g.deadline)
	}
}

// queueDepth reports the current admission queue depth (C2-05 signal).
func (g *writerGate) queueDepth() int64 { return int64(len(g.queue)) }

// rejections reports the cumulative bounded rejections.
func (g *writerGate) rejections() int64 { return g.rejected.Load() }

// lastWaitMS reports the most recent admission wait in milliseconds.
func (g *writerGate) lastWaitMS() int64 { return g.lastWaitNs.Load() / int64(time.Millisecond) }

// active reports whether one writer is currently admitted.
func (g *writerGate) isActive() bool { return g.active.Load() }

// withWriter runs fn under writer admission (C2-10 §4: transactions are
// short and database-only; callers never hold them across non-database
// waits — the gate exists so this rule has exactly one enforcement point).
func (s *Store) withWriter(ctx context.Context, fn func(tx *sqlTx) error) error {
	release, err := s.gate.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := s.writers.BeginTx(ctx, nil)
	if err != nil {
		return mapVendorError(err)
	}
	wrapped := &sqlTx{tx: tx}
	if err := fn(wrapped); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// readerCtx bounds a read operation with the C2-10 §4 read budget
// (Proposed max 5 s); a read transaction is never held across a user wait.
func (s *Store) readerCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := s.opts.ReadBudget
	if budget <= 0 {
		budget = 5 * time.Second
	}
	return context.WithTimeout(ctx, budget)
}
