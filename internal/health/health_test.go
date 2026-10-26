package health

import (
	"encoding/json"
	"strings"
	"testing"
)

func readyFixture() Report {
	return Report{
		Process:     ProcessSignals{State: string(StateReady), LockHeld: true, MigrationsApplied: true, RecoveryComplete: true},
		Storage:     StorageSignals{OK: true, DiskFreeBytes: 1024},
		WAL:         WALSignals{SizeBytes: 2048},
		Queues:      []QueueSignal{{Name: "writer", Depth: 3}},
		Diagnostics: DiagnosticsSignals{Bounded: true},
	}
}

type edit func(*Report)

func processLock(r *Report)       { r.Process.LockHeld = false }
func processMigrations(r *Report) { r.Process.MigrationsApplied = false }
func processRecovery(r *Report)   { r.Process.RecoveryComplete = false }
func storageOK(r *Report)         { r.Storage.OK = false }
func walNotStarving(r *Report)    { r.WAL.Starving = true }

func drop(base Report, e edit) Report {
	e(&base)
	return base
}

// TestReadyGate pins the readiness rule: liveness alone is never readiness;
// every gate condition is required.
func TestReadyGate(t *testing.T) {
	if !(readyFixture()).Ready() {
		t.Fatal("fixture must be ready")
	}
	cases := map[string]Report{
		"liveness alone is not readiness": {Process: ProcessSignals{State: string(StateReady)}},
		"lock missing":                    drop(readyFixture(), processLock),
		"migrations pending":              drop(readyFixture(), processMigrations),
		"recovery incomplete":             drop(readyFixture(), processRecovery),
		"storage down":                    drop(readyFixture(), storageOK),
		"wal starving":                    drop(readyFixture(), walNotStarving),
	}
	for name, r := range cases {
		if r.Ready() {
			t.Errorf("%s: must not be ready", name)
		}
	}
}

// TestStableMachineFields pins the machine form's field names: fixed English
// keys, no prose fields, so scripts and the CLI JSON mode stay stable.
func TestStableMachineFields(t *testing.T) {
	b, err := readyFixture().JSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"process"`, `"state"`, `"lockHeld"`, `"migrationsApplied"`, `"recoveryComplete"`,
		`"storage"`, `"ok"`, `"diskFreeBytes"`,
		`"wal"`, `"sizeBytes"`, `"starving"`,
		`"queues"`, `"name"`, `"depth"`,
		`"diagnostics"`, `"bounded"`, `"droppedCount"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing stable field %s in %s", want, s)
		}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["message"]; ok {
		t.Fatal("machine form must not carry prose fields")
	}
}
