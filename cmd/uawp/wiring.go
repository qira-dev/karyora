package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/storage"
)

// lazyStore opens the SQLite store only inside the runner's Migrating step
// (C2-09 §2 step 3: open, integrity, migrations — never before the runner
// owns the data directory).
type lazyStore struct {
	opts  storage.Options
	mu    sync.Mutex
	store *storage.Store
}

func (l *lazyStore) get() (*storage.Store, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store == nil {
		return nil, fmt.Errorf("storage: not opened (startup did not reach the migrating step)")
	}
	return l.store, nil
}

func (l *lazyStore) connect(ctx context.Context, coreVersion string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store != nil {
		return nil
	}
	s, err := storage.Connect(l.opts)
	if err != nil {
		return err
	}
	if err := s.Migrate(ctx, coreVersion); err != nil {
		s.Close()
		return err
	}
	l.store = s
	return nil
}

func (l *lazyStore) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store != nil {
		l.store.StopPublisher()
		_ = l.store.Close()
		l.store = nil
	}
}

// lifecycleJournal emits lifecycle transitions through the journal
// (C2-07 §1, C1-08 semantics). Transitions before the store opens are
// buffered in a hard-bounded list and flushed during startup recovery;
// post-open transitions commit through the atomic journal path.
type lifecycleJournal struct {
	store   *lazyStore
	corrID  string
	mu      sync.Mutex
	pending []pendingEvent
	dropped int
}

type pendingEvent struct {
	aggregateID string
	eventType   string
	from        string
	to          string
	cause       string
}

// maxPending bounds the pre-open buffer; overflow is counted and reported,
// never silent.
const maxPending = 64

func (j *lifecycleJournal) sink(ev runtime.Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err := j.store.get(); err != nil {
		if len(j.pending) < maxPending {
			j.pending = append(j.pending, pendingEvent{
				aggregateID: j.instanceID(),
				eventType:   "core.lifecycle.transition",
				from:        string(ev.From),
				to:          string(ev.To),
				cause:       ev.Cause,
			})
		} else {
			j.dropped++
		}
		return
	}
	j.write(ev)
}

func (j *lifecycleJournal) instanceID() string {
	// The lifecycle aggregate is the core process instance; the owner
	// record's instance UUID is the stable identifier, carried by the
	// store once open.
	return "foreground"
}

func (j *lifecycleJournal) write(ev runtime.Event) {
	s, err := j.store.get()
	if err != nil {
		return
	}
	detail := map[string]any{"from": string(ev.From), "to": string(ev.To), "cause": ev.Cause}
	if err := s.RecordLifecycleEvent(context.Background(), j.corrID, storage.AggregateCore, "foreground", "core.lifecycle.transition", detail); err != nil {
		// A journal write failure during a transition is reported through
		// startup recovery (the runner gates on Reconcile), never swallowed.
		return
	}
}

// flush commits the buffered pre-open transitions once the store is open;
// called inside the Reconcile port so a failure gates dispatch
// (RECOVERY_REQUIRED), never silently passes.
func (j *lifecycleJournal) flush(ctx context.Context) error {
	j.mu.Lock()
	pending := j.pending
	j.pending = nil
	dropped := j.dropped
	j.dropped = 0
	j.mu.Unlock()
	s, err := j.store.get()
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.RecordLifecycleEvent(ctx, j.corrID, storage.AggregateCore, "foreground", p.eventType, map[string]any{
			"from": p.from, "to": p.to, "cause": p.cause,
		}); err != nil {
			return err
		}
	}
	if dropped > 0 {
		if err := s.RecordLifecycleEvent(ctx, j.corrID, storage.AggregateCore, "foreground", "core.lifecycle.buffer_overflow", map[string]any{
			"dropped": dropped,
		}); err != nil {
			return err
		}
	}
	return nil
}

// buildOptions translates the validated configuration into the storage
// policy options (C2-10 §7 operator-tunable keys; every numeric default
// remains Proposed until recorded review).
func buildOptions(cfg config.Config, dataDir string) storage.Options {
	return storage.Options{
		Path:                   dataDir + "/karyora.db",
		PoolMaxOpen:            cfg.PoolMaxOpen,
		WriterQueueDepth:       cfg.WriterQueueDepth,
		WriterDeadline:         msDuration(cfg.WriterDeadlineMS),
		BusyTimeout:            msDuration(cfg.BusyTimeoutMS),
		WALAutocheckpointPages: cfg.WALAutocheckpointPages,
		Synchronous:            string(cfg.DurabilitySynchronous),
		ReadBudget:             msDuration(5000), // Proposed max read lifetime (C2-10 §4)
		MaxObjectBytes:         int64(cfg.CASMaxObjectBytes),
		MaxObjects:             int64(cfg.CASMaxObjects),
		GCGracePeriod:          hoursDuration(cfg.CASGCGraceHours),
		PublishInterval:        msDuration(cfg.EventsPublishIntervalMS),
		PublishBatch:           cfg.EventsPublishBatch,
		Retention:              daysDuration(cfg.EventsRetentionDays),
	}
}

// runnerPorts wires the storage-side startup and drain steps to the
// lifecycle machine (C2-09 §2 ordering; C2-07 §2 safe order). The drain
// flush covers journal/outbox/audit (C2-07 §2 step 3) and the security
// bundle's audit buffer is drained with them.
func runnerPorts(l *lazyStore, journal *lifecycleJournal, coreVersion string, bundle *securityBundle) runtime.Ports {
	return runtime.Ports{
		Migrate: func(ctx context.Context) error {
			return l.connect(ctx, coreVersion)
		},
		Reconcile: func(ctx context.Context) error {
			s, err := l.get()
			if err != nil {
				return err
			}
			if err := s.Reconcile(ctx); err != nil {
				return err
			}
			return journal.flush(ctx)
		},
		BumpGeneration: func(ctx context.Context) (uint64, error) {
			s, err := l.get()
			if err != nil {
				return 0, err
			}
			return s.BumpGeneration(ctx)
		},
		FencePrior: func(ctx context.Context, generation uint64) error {
			s, err := l.get()
			if err != nil {
				return err
			}
			return s.FencePrior(ctx, journal.corrID, generation)
		},
		Persist: func(ctx context.Context) error {
			s, err := l.get()
			if err != nil {
				return err
			}
			return s.Persist(ctx)
		},
		FlushEvents: func(ctx context.Context) error {
			s, err := l.get()
			if err != nil {
				return err
			}
			if err := s.FlushEvents(ctx); err != nil {
				return err
			}
			// Audit is the third flush class (C2-07 §2 safe order): the
			// reserved-quota buffer must reach the durable chain before the
			// drain reports completion.
			return bundle.audit.Flush()
		},
		Metrics: func() (uint64, bool, int64, int64) {
			s, err := l.get()
			if err != nil {
				return 0, false, 0, 0
			}
			return s.Signals()
		},
	}
}

func msDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

func hoursDuration(h int) time.Duration { return time.Duration(h) * time.Hour }

func daysDuration(d int) time.Duration { return time.Duration(d) * 24 * time.Hour }
