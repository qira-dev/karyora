// Package runtime implements the single Core lifecycle state machine and its
// startup/shutdown/recovery wiring (C2-07/C2-09). One machine serves both the
// foreground CLI path and the OS service adapters; adapters call the same
// lifecycle API and cannot create a second scheduler or database owner.
// Storage-side steps arrive through the Ports surface: the composition root
// wires the durable implementations, and the machine fails closed until it
// does. Lifecycle transitions are emitted to the event sink (C1-08
// semantics); the journal-backed sink is wired when the storage stack lands.
package runtime

import (
	"fmt"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// State is one lifecycle machine state (C2-09 §1). The machine is single and
// explicit: Idle → Acquiring → Validating → Migrating → Recovering → Ready →
// Draining → Stopped, with Failed reachable from any state.
type State string

const (
	StateIdle       State = "idle"
	StateAcquiring  State = "acquiring"
	StateValidating State = "validating"
	StateMigrating  State = "migrating"
	StateRecovering State = "recovering"
	StateReady      State = "ready"
	StateDraining   State = "draining"
	StateStopped    State = "stopped"
	StateFailed     State = "failed"
)

// transitions is the legal transition table. A durable state never ends
// outside these edges (Phase 1 transition contract).
var transitions = map[State][]State{
	StateIdle:       {StateAcquiring},
	StateAcquiring:  {StateValidating, StateFailed},
	StateValidating: {StateMigrating, StateFailed},
	StateMigrating:  {StateRecovering, StateFailed},
	StateRecovering: {StateReady, StateFailed},
	StateReady:      {StateDraining, StateFailed},
	StateDraining:   {StateStopped, StateFailed},
	StateStopped:    {},
	StateFailed:     {},
}

// Recovery-gated states: while the machine sits here, dispatch is gated and
// status reports RECOVERY_REQUIRED (C2-09 §1).
func RecoveryGated(s State) bool {
	return s == StateMigrating || s == StateRecovering
}

// Event is one lifecycle transition record emitted to the sink (C1-08
// semantics; the journal-backed sink persists it with the storage stack).
type Event struct {
	From     State          `json:"from"`
	To       State          `json:"to"`
	At       string         `json:"at"`
	Cause    string         `json:"cause,omitempty"`
	Envelope *errs.Envelope `json:"error,omitempty"`
}

// Machine is the single lifecycle state machine. It is safe for concurrent
// use; every transition is validated against the table and emitted.
type Machine struct {
	mu      sync.Mutex
	current State
	sink    func(Event)
	failure *errs.Envelope
}

// NewMachine starts an idle machine wired to the given transition sink (nil
// sinks events).
func NewMachine(sink func(Event)) *Machine {
	return &Machine{current: StateIdle, sink: sink}
}

// Current reports the active state.
func (m *Machine) Current() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Failure reports the machine error carried by the Failed state, if any.
func (m *Machine) Failure() *errs.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failure
}

// Transition moves the machine along a legal edge and emits the event.
func (m *Machine) Transition(to State, cause string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, legal := range transitions[m.current] {
		if legal == to {
			ev := Event{From: m.current, To: to, At: time.Now().UTC().Format(time.RFC3339Nano), Cause: cause}
			m.current = to
			if m.sink != nil {
				m.sink(ev)
			}
			return nil
		}
	}
	return fmt.Errorf("runtime: illegal transition %s → %s", m.current, to)
}

// Fail moves the machine to Failed carrying the stable machine error.
func (m *Machine) Fail(env errs.Envelope, cause string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == StateFailed || m.current == StateStopped {
		return
	}
	ev := Event{From: m.current, To: StateFailed, At: time.Now().UTC().Format(time.RFC3339Nano), Cause: cause, Envelope: &env}
	m.current = StateFailed
	m.failure = &env
	if m.sink != nil {
		m.sink(ev)
	}
}

// drainBudget is the bounded graceful-shutdown time budget (Proposed default
// 20 seconds, C2-07 §2; fixed once published after recorded review). It is a
// variable only so tests can shrink it; production code never mutates it.
var drainBudget = 20 * time.Second

// DrainBudget reports the Proposed graceful-stop budget.
func DrainBudget() time.Duration { return drainBudget }
