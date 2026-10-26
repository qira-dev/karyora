package runtime

import (
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

func TestMachineHappyPath(t *testing.T) {
	var events []Event
	m := NewMachine(func(e Event) { events = append(events, e) })
	path := []State{StateAcquiring, StateValidating, StateMigrating, StateRecovering, StateReady, StateDraining, StateStopped}
	for _, s := range path {
		if err := m.Transition(s, "test"); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	if m.Current() != StateStopped {
		t.Fatalf("state = %s, want stopped", m.Current())
	}
	if len(events) != len(path) {
		t.Fatalf("emitted %d events, want %d", len(events), len(path))
	}
}

func TestMachineIllegalTransition(t *testing.T) {
	m := NewMachine(nil)
	if err := m.Transition(StateReady, "skip"); err == nil {
		t.Fatal("idle → ready must be illegal")
	}
	if err := m.Transition(StateStopped, "skip"); err == nil {
		t.Fatal("idle → stopped must be illegal")
	}
	if m.Current() != StateIdle {
		t.Fatalf("failed transitions must not move the machine: %s", m.Current())
	}
}

func TestMachineFailCarriesEnvelope(t *testing.T) {
	m := NewMachine(nil)
	if err := m.Transition(StateAcquiring, "start"); err != nil {
		t.Fatal(err)
	}
	env, _ := errs.New(errs.CodeMigrationFailed, "c1", nil)
	m.Fail(env, "migrate")
	if m.Current() != StateFailed {
		t.Fatalf("state = %s, want failed", m.Current())
	}
	got := m.Failure()
	if got == nil || got.Code != errs.CodeMigrationFailed {
		t.Fatalf("carried failure = %+v", got)
	}
	// Failed is terminal.
	if err := m.Transition(StateValidating, "resume"); err == nil {
		t.Fatal("failed machine must be terminal")
	}
	// A second Fail does not overwrite the recorded envelope.
	env2, _ := errs.New(errs.CodeInternal, "c2", nil)
	m.Fail(env2, "again")
	if m.Failure().CorrelationID != "c1" {
		t.Fatalf("first failure must be preserved: %+v", m.Failure())
	}
}

func TestRecoveryGatedStates(t *testing.T) {
	if !RecoveryGated(StateMigrating) || !RecoveryGated(StateRecovering) {
		t.Fatal("migrating/recovering gate dispatch on recovery")
	}
	if RecoveryGated(StateReady) {
		t.Fatal("ready must not be recovery-gated")
	}
}

func TestDrainBudgetIsProposedDefault(t *testing.T) {
	if DrainBudget() != 20*time.Second {
		t.Fatalf("drain budget = %s, want the Proposed 20s default", DrainBudget())
	}
}
