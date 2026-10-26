// Package health defines the liveness/readiness report model (C2-05): the
// five Phase 2 signal groups with stable machine-readable field names, and
// the readiness gate that never derives product readiness from liveness
// alone. JSON field names are fixed English; the human form is derived by
// the presentation layer from this same report, so the two cannot disagree.
package health

import "encoding/json"

// State mirrors the Core lifecycle machine (C2-07).
type State string

const (
	StateStarting         State = "starting"
	StateRecoveryRequired State = "recovery_required"
	StateReady            State = "ready"
	StateDraining         State = "draining"
	StateStopped          State = "stopped"
)

// ProcessSignals cover the process/startup group.
type ProcessSignals struct {
	State             string `json:"state"`
	LockHeld          bool   `json:"lockHeld"`
	MigrationsApplied bool   `json:"migrationsApplied"`
	RecoveryComplete  bool   `json:"recoveryComplete"`
}

// StorageSignals cover the storage/disk group.
type StorageSignals struct {
	OK            bool   `json:"ok"`
	DiskFreeBytes uint64 `json:"diskFreeBytes"`
}

// WALSignals cover the WAL/checkpoint group.
type WALSignals struct {
	SizeBytes uint64 `json:"sizeBytes"`
	Starving  bool   `json:"starving"`
}

// QueueSignal is one bounded local queue's backlog depth.
type QueueSignal struct {
	Name  string `json:"name"`
	Depth int    `json:"depth"`
}

// DiagnosticsSignals cover the diagnostics group.
type DiagnosticsSignals struct {
	Bounded      bool   `json:"bounded"`
	DroppedCount uint64 `json:"droppedCount"`
}

// Report is the single source for both machine and human forms (C2-05).
type Report struct {
	Process     ProcessSignals     `json:"process"`
	Storage     StorageSignals     `json:"storage"`
	WAL         WALSignals         `json:"wal"`
	Queues      []QueueSignal      `json:"queues"`
	Diagnostics DiagnosticsSignals `json:"diagnostics"`
}

// Ready applies the readiness gate: migrations applied, authority lock held,
// startup recovery complete, storage writable, and no sustained WAL/checkpoint
// starvation. Liveness alone is never sufficient.
func (r Report) Ready() bool {
	return r.Process.State == string(StateReady) &&
		r.Process.LockHeld &&
		r.Process.MigrationsApplied &&
		r.Process.RecoveryComplete &&
		r.Storage.OK &&
		!r.WAL.Starving
}

// JSON returns the machine form with the report's stable field names.
func (r Report) JSON() ([]byte, error) {
	return json.Marshal(r)
}
