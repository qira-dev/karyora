package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

// Ports are the storage-side startup and shutdown steps (C2-09 §2). The
// machine gates each step on the previous one; until the storage stack is
// wired by the composition root, Migrate fails closed and the Core never
// reaches Ready (RECOVERY_REQUIRED, exit EXIT_RECOVERY_REQUIRED).
type Ports struct {
	// Migrate opens storage, runs the integrity check and applies/resumes
	// migrations (C2-11). MIGRATION_FAILED gates everything.
	Migrate func(ctx context.Context) error
	// Reconcile re-publishes committed-but-unpublished outbox rows and
	// completes staged CAS writes; idempotent by construction (C2-12/13).
	Reconcile func(ctx context.Context) error
	// BumpGeneration increments the persisted recovery generation and
	// returns it — the local fencing token (C2-09 §2 step 5).
	BumpGeneration func(ctx context.Context) (uint64, error)
	// FencePrior fences every generation older than the current one and
	// quarantines work with unknown outcomes; the Phase 2 local slice has no
	// attempts yet, so the wired implementation records the fence outcome.
	FencePrior func(ctx context.Context, generation uint64) error
	// Persist saves required durable state during draining (safe-order step
	// 2, C2-07 §2).
	Persist func(ctx context.Context) error
	// FlushEvents flushes journal/outbox/audit during draining (safe-order
	// step 3, C2-07 §2).
	FlushEvents func(ctx context.Context) error
}

// Failure couples a stable machine error envelope with the CLI exit code
// (C2-07 §4 register). Runtime maps faults at the boundary; the envelope
// never carries OS or DB vendor types (C2-05).
type Failure struct {
	Envelope errs.Envelope
	Exit     ExitCode
}

func (f *Failure) Error() string {
	return fmt.Sprintf("runtime: %s (exit %d, correlationId=%s)", f.Envelope.Code, f.Exit, f.Envelope.CorrelationID)
}

func failf(corr string, code errs.Code, exit ExitCode, data map[string]any) *Failure {
	env, err := errs.New(code, corr, data)
	if err != nil {
		// A contract-violating envelope fails closed to a redacted internal
		// envelope; the register mapping below stays exact.
		env = errs.Envelope{Code: errs.CodeInternal, Retryable: true, CorrelationID: corr, Data: map[string]any{"retryClass": "safe-retry"}}
	}
	return &Failure{Envelope: env, Exit: exit}
}

// Runner executes the single lifecycle for one data directory. It is created
// per process; foreground and service adapters share this type.
type Runner struct {
	machine  *Machine
	ports    Ports
	paths    platform.DirSet
	own      *platform.Ownership
	listener interface{ Close() error } // IPC listener while Ready

	mu         sync.Mutex
	generation uint64
	tookOver   bool
	stopOnce   sync.Once
	stopDone   chan struct{}
	drainState DrainOutcome
	readyGate  bool
}

// NewRunner wires one runner. sink receives lifecycle transitions (C1-08
// semantics); nil events are simply not emitted.
func NewRunner(paths platform.DirSet, ports Ports, sink func(Event)) *Runner {
	if ports.Migrate == nil {
		ports.Migrate = func(ctx context.Context) error {
			return fmt.Errorf("storage stack is not linked in this build slice")
		}
	}
	return &Runner{
		machine:  NewMachine(sink),
		ports:    ports,
		paths:    paths,
		stopDone: make(chan struct{}),
	}
}

// Machine exposes the lifecycle machine for read-only diagnostics.
func (r *Runner) Machine() *Machine { return r.machine }

// Owner reports the recorded owner snapshot, when the lock is held.
func (r *Runner) Owner() (platform.OwnerRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.own == nil {
		return platform.OwnerRecord{}, false
	}
	return r.own.Record, true
}

// Generation reports the current recovery generation (0 before startup).
func (r *Runner) Generation() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation
}

// TookOver reports that this start was a stale-owner takeover.
func (r *Runner) TookOver() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tookOver
}

// drainStateResult is the bounded drain outcome.
type DrainOutcome struct {
	Completed        bool   `json:"completed"`
	DeadlineExceeded bool   `json:"deadlineExceeded"`
	Reason           string `json:"reason,omitempty"`
}

// DrainResult reports how the last drain ended.
func (r *Runner) DrainResult() DrainOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.drainState
}

// Start executes the strict startup ordering (C2-09 §2): acquire authority →
// validate → migrate → reconcile → bump generation → fence → ready. Each
// step gates the next; any failure moves the machine to Failed with a stable
// envelope and mapped exit code. Acquiring is the only step with an
// externally visible authority effect (the kernel lock).
func (r *Runner) Start(ctx context.Context, correlationID, coreVersion string) *Failure {
	if err := r.machine.Transition(StateAcquiring, "start"); err != nil {
		return failf(correlationID, errs.CodeConflictState, ExitFailure, map[string]any{"detail": err.Error()})
	}
	own, tookOver, err := platform.AcquireOwnership(r.paths, r.paths.Data, coreVersion)
	if err != nil {
		if err == platform.ErrLockHeld {
			env, _ := errs.New(errs.CodeSingleInstanceConflict, correlationID, map[string]any{"resource": "data-directory"})
			r.machine.Fail(env, "lock")
			return &Failure{Envelope: env, Exit: ExitAlreadyRunning}
		}
		env, _ := errs.New(errs.CodeInternal, correlationID, map[string]any{"reason": "LOCK_INFRASTRUCTURE"})
		r.machine.Fail(env, "lock")
		return &Failure{Envelope: env, Exit: ExitAuthority}
	}
	r.mu.Lock()
	r.own = own
	r.tookOver = tookOver
	r.mu.Unlock()
	if tookOver {
		// A stale takeover is a recovery event, not a silent restart
		// (C2-08 §4): the kernel-lock handoff plus the generation bump in
		// step 5 gate the start behind recovery, and status reports it.
	}

	if err := r.machine.Transition(StateValidating, "authority acquired"); err != nil {
		return failf(correlationID, errs.CodeConflictState, ExitFailure, map[string]any{"detail": err.Error()})
	}
	if err := platform.EnsureLayout(r.paths); err != nil {
		env, _ := errs.New(errs.CodeValidationFailed, correlationID, map[string]any{"resource": "paths"})
		r.machine.Fail(env, "layout")
		_ = own.Release()
		return &Failure{Envelope: env, Exit: ExitPermission}
	}

	if err := r.machine.Transition(StateMigrating, "validated"); err != nil {
		return failf(correlationID, errs.CodeConflictState, ExitFailure, map[string]any{"detail": err.Error()})
	}
	if err := r.ports.Migrate(ctx); err != nil {
		env, _ := errs.New(errs.CodeMigrationFailed, correlationID, map[string]any{"reason": boundedReason(err)})
		r.machine.Fail(env, "migrate")
		_ = own.Release()
		return &Failure{Envelope: env, Exit: ExitRecoveryRequired}
	}

	if err := r.machine.Transition(StateRecovering, "migrated"); err != nil {
		return failf(correlationID, errs.CodeConflictState, ExitFailure, map[string]any{"detail": err.Error()})
	}
	if r.ports.Reconcile != nil {
		if err := r.ports.Reconcile(ctx); err != nil {
			env, _ := errs.New(errs.CodeRecoveryRequired, correlationID, map[string]any{"reason": boundedReason(err)})
			r.machine.Fail(env, "reconcile")
			_ = own.Release()
			return &Failure{Envelope: env, Exit: ExitRecoveryRequired}
		}
	}
	if r.ports.BumpGeneration != nil {
		gen, err := r.ports.BumpGeneration(ctx)
		if err != nil {
			env, _ := errs.New(errs.CodeRecoveryRequired, correlationID, map[string]any{"reason": boundedReason(err)})
			r.machine.Fail(env, "generation")
			_ = own.Release()
			return &Failure{Envelope: env, Exit: ExitRecoveryRequired}
		}
		r.mu.Lock()
		r.generation = gen
		r.mu.Unlock()
	}
	if r.ports.FencePrior != nil && r.Generation() > 0 {
		if err := r.ports.FencePrior(ctx, r.Generation()); err != nil {
			env, _ := errs.New(errs.CodeRecoveryRequired, correlationID, map[string]any{"reason": boundedReason(err)})
			r.machine.Fail(env, "fence")
			_ = own.Release()
			return &Failure{Envelope: env, Exit: ExitRecoveryRequired}
		}
	}

	if err := r.machine.Transition(StateReady, "recovery complete"); err != nil {
		return failf(correlationID, errs.CodeConflictState, ExitFailure, map[string]any{"detail": err.Error()})
	}
	r.mu.Lock()
	r.readyGate = true
	r.mu.Unlock()
	return nil
}

// Drain executes the bounded safe-order shutdown (C2-07 §2): stop
// admission/dispatch → persist durable state → flush journal/outbox/audit →
// release ownership. Deadline expiry never forces a half-state: the outcome
// is reported and the next start's crash-recovery path reconciles.
func (r *Runner) Drain(ctx context.Context, cause string) DrainOutcome {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		current := r.machine.Current()
		r.mu.Unlock()
		if current != StateReady {
			// Nothing to drain for a machine that never became ready.
			r.drainState = DrainOutcome{Completed: false, Reason: "NOT_READY"}
			close(r.stopDone)
			return
		}
		_ = r.machine.Transition(StateDraining, cause)
		r.mu.Lock()
		r.readyGate = false
		r.mu.Unlock()

		budget, cancel := context.WithTimeout(context.Background(), drainBudget)
		defer cancel()

		outcome := DrainOutcome{Completed: true}
		if err := step(budget, r.ports.Persist); err != nil {
			outcome = drainDeadlineOr(err)
		}
		if outcome.Completed {
			if err := step(budget, r.ports.FlushEvents); err != nil {
				outcome = drainDeadlineOr(err)
			}
		}
		if r.listener != nil {
			_ = r.listener.Close()
		}
		if r.own != nil {
			_ = r.own.Release()
			r.mu.Lock()
			r.own = nil
			r.mu.Unlock()
		}
		if outcome.Completed {
			_ = r.machine.Transition(StateStopped, cause)
		} else {
			// Deadline expiry: the process exits and crash recovery
			// reconciles; the machine records the drained-to-stopped end of
			// the process with the outcome reported alongside.
			_ = r.machine.Transition(StateStopped, cause+":deadline-expired")
		}
		r.mu.Lock()
		r.drainState = outcome
		r.mu.Unlock()
		close(r.stopDone)
	})
	<-r.stopDone
	return r.DrainResult()
}

// StopRequested triggers one graceful drain (signals and IPC converge here;
// a repeated request does not bypass the safe order, C2-07 §3).
func (r *Runner) StopRequested(cause string) {
	go r.Drain(context.Background(), cause)
}

// Done is closed once the drain (or a refused drain for a non-ready machine)
// has finished.
func (r *Runner) Done() <-chan struct{} { return r.stopDone }

// ServeIPC binds the local lifecycle endpoint; it exists only while the lock
// is held (C2-08 §5) and exposes only status/stop operations.
func (r *Runner) ServeIPC() (func() error, error) {
	ln, err := platform.ListenIPC(r.paths)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.listener = ln
	r.mu.Unlock()
	rec := r.own.Record
	token := r.own.Capability()
	verify := func(req platform.IPCRequest) (bool, string) {
		switch req.Op {
		case platform.IPCOpStatus, platform.IPCOpStop:
			return true, ""
		default:
			return false, "NOT_FOUND"
		}
	}
	handle := func(req platform.IPCRequest) platform.IPCResponse {
		switch req.Op {
		case platform.IPCOpStatus:
			body, err := json.Marshal(r.StatusReport())
			if err != nil {
				return platform.IPCResponse{OK: false, Code: "INTERNAL"}
			}
			return platform.IPCResponse{OK: true, Body: body}
		case platform.IPCOpStop:
			r.StopRequested("ipc-stop")
			select {
			case <-r.stopDone:
				body, _ := json.Marshal(r.DrainResult())
				return platform.IPCResponse{OK: true, Body: body}
			case <-time.After(drainBudget + 5*time.Second):
				return platform.IPCResponse{OK: false, Code: "INTERNAL"}
			}
		default:
			return platform.IPCResponse{OK: false, Code: "NOT_FOUND"}
		}
	}
	go func() {
		_ = platform.ServeIPC(ln, rec, token, verify, handle)
	}()
	return func() error { return ln.Close() }, nil
}

// StatusReport assembles the single status report both output forms derive
// from (C2-05 §4, C2-07 §2 status semantics).
func (r *Runner) StatusReport() StatusReport {
	r.mu.Lock()
	own := r.own
	generation := r.generation
	tookOver := r.tookOver
	r.mu.Unlock()
	var rec *platform.OwnerRecord
	if own != nil {
		copied := own.Record
		rec = &copied
	}
	return BuildStatusReport(r.machine, own != nil, generation, tookOver, rec)
}

func step(ctx context.Context, f func(context.Context) error) error {
	if f == nil {
		return nil
	}
	return f(ctx)
}

func drainDeadlineOr(err error) DrainOutcome {
	if err == nil {
		return DrainOutcome{Completed: true}
	}
	if err == context.DeadlineExceeded {
		return DrainOutcome{Completed: false, DeadlineExceeded: true, Reason: "STOP_DEADLINE_EXPIRED"}
	}
	return DrainOutcome{Completed: false, Reason: boundedReason(err)}
}

// boundedReason reduces an arbitrary error to one bounded machine-safe
// detail member; the envelope never carries raw upstream content.
func boundedReason(err error) string {
	const max = 200
	s := err.Error()
	if len(s) > max {
		s = s[:max]
	}
	return s
}
