package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/storage"
)

// storageProbe is doctor's read-only storage check (C2-07 §2): open the
// database read-only, run the integrity check, and report the schema
// version and recovery state. It never repairs or deletes; a live owner
// making the store busy surfaces as an explicit unavailable check.
func storageProbe(ctx context.Context, opts storage.Options) runtime.DoctorCheck {
	if _, err := os.Stat(opts.Path); os.IsNotExist(err) {
		// A never-started install has no database yet: nothing is broken,
		// and doctor never creates or repairs anything.
		return runtime.DoctorCheck{Name: "storage", Status: runtime.DoctorUnavailable, Detail: "database not initialized (run start once)"}
	}
	version, integrity, err := storage.ProbeReadonly(ctx, opts.Path)
	if err != nil {
		return runtime.DoctorCheck{Name: "storage", Status: runtime.DoctorFail, Detail: bounded(err)}
	}
	detail := fmt.Sprintf("integrity=%s schema_version=%d", integrity, version)
	return runtime.DoctorCheck{Name: "storage", Status: runtime.DoctorPass, Detail: detail}
}

func bounded(err error) string {
	const max = 160
	s := err.Error()
	if len(s) > max {
		s = s[:max]
	}
	return s
}
