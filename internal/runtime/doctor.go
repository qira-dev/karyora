package runtime

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

// Doctor runs the read-only Phase 2 diagnostic scope (C2-07 §2): configuration
// validation, paths/permissions posture, lock owner, and — through the
// wired storage port once it lands — storage open/integrity, migration
// state, and recovery readiness. doctor never repairs or deletes, and Phase 2
// claims no provider, plugin, update-trust, or UI checks.
type Doctor struct {
	Paths   platform.DirSet
	Locale  Locale
	Storage func(ctx context.Context) DoctorCheck // wired with the storage stack
	// Audit verifies the tamper-evident audit chain (C2-15 §2): wired with
	// the audit stack; an integrity-broken or unverifiable chain is a
	// visible FAIL, never a silent pass.
	Audit func(ctx context.Context) DoctorCheck
	// ConfigFailure, when set, makes the configuration check fail with the
	// recorded envelope instead of re-validating internally.
	ConfigFailure *errs.Envelope
}

// DoctorCheck is one bounded diagnostic result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass | fail | unavailable
	Detail string `json:"detail,omitempty"`
}

// Doctor statuses (stable English identifiers).
const (
	DoctorPass        = "pass"
	DoctorFail        = "fail"
	DoctorUnavailable = "unavailable"
)

// DoctorReport is the single object behind both output forms.
type DoctorReport struct {
	Overall string        `json:"overall"`
	Checks  []DoctorCheck `json:"checks"`
}

// Human renders the human form from the same report object.
func (r DoctorReport) Human(loc Locale) string {
	var b strings.Builder
	b.WriteString(loc.Text(KeyDoctorOverall) + ": " + r.Overall + "\n")
	for _, c := range r.Checks {
		line := c.Name + ": " + c.Status
		if c.Detail != "" {
			line += " (" + c.Detail + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// Run executes the checks read-only and returns the report.
func (d Doctor) Run(ctx context.Context) DoctorReport {
	checks := []DoctorCheck{}
	checks = append(checks, d.checkConfig())
	checks = append(checks, d.checkPaths())
	checks = append(checks, d.checkLockOwner())
	if d.Storage != nil {
		checks = append(checks, d.Storage(ctx))
	} else {
		checks = append(checks, DoctorCheck{Name: "storage", Status: DoctorUnavailable, Detail: "storage stack not linked in this build slice"})
		checks = append(checks, DoctorCheck{Name: "migrations", Status: DoctorUnavailable, Detail: "storage stack not linked in this build slice"})
		checks = append(checks, DoctorCheck{Name: "recovery", Status: DoctorUnavailable, Detail: "storage stack not linked in this build slice"})
	}
	if d.Audit != nil {
		checks = append(checks, d.Audit(ctx))
	} else {
		checks = append(checks, DoctorCheck{Name: "audit", Status: DoctorUnavailable, Detail: "audit stack not linked in this build slice"})
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	overall := DoctorPass
	for _, c := range checks {
		if c.Status == DoctorFail {
			overall = DoctorFail
			break
		}
		if c.Status == DoctorUnavailable && overall == DoctorPass {
			overall = DoctorUnavailable
		}
	}
	return DoctorReport{Overall: overall, Checks: checks}
}

// checkConfig reports the configuration validation result the caller
// resolved (doctor itself stays read-only; it does not reload files).
func (d Doctor) checkConfig() DoctorCheck {
	if d.ConfigFailure != nil {
		return DoctorCheck{Name: "config", Status: DoctorFail, Detail: string(d.ConfigFailure.Code)}
	}
	return DoctorCheck{Name: "config", Status: DoctorPass, Detail: "configuration valid"}
}

// checkPaths verifies the class directories exist with the least-privilege
// posture (0700 directories per C2-06 §3).
func (d Doctor) checkPaths() DoctorCheck {
	for _, p := range []string{d.Paths.Data, d.Paths.Config, d.Paths.Runtime} {
		st, err := os.Stat(p)
		if err != nil {
			return DoctorCheck{Name: "paths", Status: DoctorFail, Detail: "missing class directory (run start once to create the layout)"}
		}
		if !st.IsDir() {
			return DoctorCheck{Name: "paths", Status: DoctorFail, Detail: "class path is not a directory"}
		}
		if st.Mode().Perm()&0o077 != 0 {
			return DoctorCheck{Name: "paths", Status: DoctorFail, Detail: "class directory broader than 0700"}
		}
	}
	return DoctorCheck{Name: "paths", Status: DoctorPass, Detail: "data/config/runtime directories present with 0700 posture"}
}

// checkLockOwner reports the recorded owner and whether the kernel lock is
// currently held (proven only by attempting nothing — doctor stays
// read-only; a live endpoint answering status proves liveness).
func (d Doctor) checkLockOwner() DoctorCheck {
	rec, token, err := platform.OwnerCredentials(d.Paths.Runtime)
	if err != nil {
		return DoctorCheck{Name: "lock_owner", Status: DoctorPass, Detail: "no owner recorded (core not started)"}
	}
	resp, callErr := platform.CallIPC(d.Paths.Runtime, platform.IPCOpStatus, rec.InstanceUUID, token)
	if callErr != nil {
		return DoctorCheck{Name: "lock_owner", Status: DoctorPass, Detail: "recorded owner present; no live endpoint answered (prior owner exited)"}
	}
	if !resp.OK {
		return DoctorCheck{Name: "lock_owner", Status: DoctorFail, Detail: "live endpoint denied the status request (" + resp.Code + ")"}
	}
	return DoctorCheck{Name: "lock_owner", Status: DoctorPass, Detail: "live owner: pid=" + itoa(rec.PID) + " instance=" + rec.InstanceUUID}
}
