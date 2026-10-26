package runtime

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/qira-dev/karyora/internal/health"
	"github.com/qira-dev/karyora/internal/platform"
)

// StatusReport is the single status object both the machine form (stable
// English JSON keys) and the human form derive from, so the two cannot
// disagree (C2-05 §4, C2-07 §5). Liveness says only that the process exists
// and responds — never that the product is ready.
type StatusReport struct {
	Live   bool           `json:"live"`
	Owner  *OwnerSnapshot `json:"owner,omitempty"`
	Health health.Report  `json:"health"`
}

// OwnerSnapshot carries the recorded/live owner identity diagnostics
// (C2-08 §3). It never contains secrets.
type OwnerSnapshot struct {
	InstanceUUID       string `json:"instanceUUID"`
	PID                int    `json:"pid"`
	ProcessStartTime   string `json:"processStartTime"`
	BootID             string `json:"bootID"`
	CoreVersion        string `json:"coreVersion"`
	DataDirIdentity    string `json:"dataDirIdentity"`
	RecoveryGeneration uint64 `json:"recoveryGeneration"`
	TookOver           bool   `json:"tookOver"`
}

// BuildStatusReport assembles the live report from the machine, lock state,
// generation, and owner record.
func BuildStatusReport(m *Machine, lockHeld bool, generation uint64, tookOver bool, owner *platform.OwnerRecord) StatusReport {
	state := StateStopped
	failed := false
	if m != nil {
		state = m.Current()
		failed = state == StateFailed
	}
	rep := health.Report{
		Process: health.ProcessSignals{
			State:             string(state),
			LockHeld:          lockHeld,
			MigrationsApplied: state == StateReady || state == StateDraining,
			RecoveryComplete:  state == StateReady || state == StateDraining,
		},
		Storage:     health.StorageSignals{OK: state == StateReady || state == StateDraining},
		WAL:         health.WALSignals{},
		Queues:      []health.QueueSignal{},
		Diagnostics: health.DiagnosticsSignals{Bounded: true},
	}
	if failed {
		// A failed machine stays observable: the state carries the failure;
		// storage is not writable beyond the failure point.
		rep.Storage.OK = false
	}
	report := StatusReport{Live: lockHeld && !failed, Health: rep}
	if owner != nil {
		report.Owner = &OwnerSnapshot{
			InstanceUUID:       owner.InstanceUUID,
			PID:                owner.PID,
			ProcessStartTime:   owner.ProcessStartTime,
			BootID:             owner.BootID,
			CoreVersion:        owner.CoreVersion,
			DataDirIdentity:    owner.DataDirIdentity,
			RecoveryGeneration: generation,
			TookOver:           tookOver,
		}
	}
	return report
}

// NotRunningReport is the stable report when no live owner exists: liveness
// false, stopped state, no readiness derived from anything.
func NotRunningReport(recorded *platform.OwnerRecord) StatusReport {
	rep := health.Report{
		Process:     health.ProcessSignals{State: string(StateStopped)},
		Storage:     health.StorageSignals{OK: false},
		WAL:         health.WALSignals{},
		Queues:      []health.QueueSignal{},
		Diagnostics: health.DiagnosticsSignals{Bounded: true},
	}
	report := StatusReport{Live: false, Health: rep}
	if recorded != nil {
		report.Owner = &OwnerSnapshot{
			InstanceUUID:     recorded.InstanceUUID,
			PID:              recorded.PID,
			ProcessStartTime: recorded.ProcessStartTime,
			BootID:           recorded.BootID,
			CoreVersion:      recorded.CoreVersion,
			DataDirIdentity:  recorded.DataDirIdentity,
		}
	}
	return report
}

// JSON renders the machine form.
func (r StatusReport) JSON() ([]byte, error) {
	return json.Marshal(r)
}

// Human renders the human form from the same report object. Locale copy
// resolves through the catalog with English fallback (C2-07 §5).
func (r StatusReport) Human(loc Locale) string {
	var b strings.Builder
	if r.Live {
		b.WriteString(loc.Text(KeyStatusLive))
	} else {
		b.WriteString(loc.Text(KeyStatusNotRunning))
	}
	b.WriteString("\n")
	if r.Owner != nil {
		b.WriteString(loc.Text(KeyStatusOwner) + ": pid=" + itoa(r.Owner.PID) +
			" instance=" + r.Owner.InstanceUUID +
			" version=" + r.Owner.CoreVersion)
		if r.Owner.TookOver {
			b.WriteString(" " + loc.Text(KeyStatusTakeover))
		}
		b.WriteString("\n")
	}
	h := r.Health
	b.WriteString(loc.Text(KeyStatusState) + ": " + h.Process.State + "\n")
	b.WriteString(loc.Text(KeyStatusLock) + ": " + boolWord(h.Process.LockHeld, loc) + "\n")
	b.WriteString(loc.Text(KeyStatusMigrations) + ": " + boolWord(h.Process.MigrationsApplied, loc) + "\n")
	b.WriteString(loc.Text(KeyStatusRecovery) + ": " + boolWord(h.Process.RecoveryComplete, loc) + "\n")
	b.WriteString(loc.Text(KeyStatusStorage) + ": " + boolWord(h.Storage.OK, loc) + "\n")
	if h.WAL.Starving {
		b.WriteString(loc.Text(KeyStatusWALStarving) + "\n")
	}
	return b.String()
}

func boolWord(v bool, loc Locale) string {
	if v {
		return loc.Text(KeyWordYes)
	}
	return loc.Text(KeyWordNo)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
