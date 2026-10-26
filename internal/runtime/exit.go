package runtime

import (
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// ExitCode is the stable CLI exit-code register (C2-07 §4). Identifiers are
// fixed English; the numeric values are Proposed and fixed once published.
type ExitCode int

const (
	ExitOK               ExitCode = 0   // EXIT_OK: success / idempotent no-op
	ExitFailure          ExitCode = 1   // EXIT_FAILURE: uncategorized failure
	ExitConfigInvalid    ExitCode = 2   // EXIT_CONFIG_INVALID
	ExitAlreadyRunning   ExitCode = 3   // EXIT_ALREADY_RUNNING
	ExitAuthority        ExitCode = 4   // EXIT_AUTHORITY: lock/ownership/IPC failure
	ExitStorage          ExitCode = 5   // EXIT_STORAGE: storage open/IO/integrity failure
	ExitRecoveryRequired ExitCode = 6   // EXIT_RECOVERY_REQUIRED
	ExitPermission       ExitCode = 7   // EXIT_PERMISSION
	ExitInternal         ExitCode = 120 // EXIT_INTERNAL (redacted)
)

// ExitCodeForFailure maps a machine error envelope to the exit register.
// The mapping is part of the runtime's boundary contract; unknown codes fail
// closed to EXIT_FAILURE rather than guessing.
func ExitCodeForFailure(env errs.Envelope) ExitCode {
	switch env.Code {
	case errs.CodeValidationFailed, errs.CodeConfigConflict:
		return ExitConfigInvalid
	case errs.CodeSingleInstanceConflict:
		return ExitAlreadyRunning
	case errs.CodeAuthzDenied:
		return ExitAuthority
	case errs.CodeStorageUnavailable, errs.CodeStateCorrupt:
		return ExitStorage
	case errs.CodeMigrationFailed, errs.CodeRecoveryRequired:
		return ExitRecoveryRequired
	case errs.CodeInternal:
		return ExitInternal
	default:
		return ExitFailure
	}
}

// ExitName returns the stable English identifier of an exit code.
func ExitName(c ExitCode) string {
	switch c {
	case ExitOK:
		return "EXIT_OK"
	case ExitFailure:
		return "EXIT_FAILURE"
	case ExitConfigInvalid:
		return "EXIT_CONFIG_INVALID"
	case ExitAlreadyRunning:
		return "EXIT_ALREADY_RUNNING"
	case ExitAuthority:
		return "EXIT_AUTHORITY"
	case ExitStorage:
		return "EXIT_STORAGE"
	case ExitRecoveryRequired:
		return "EXIT_RECOVERY_REQUIRED"
	case ExitPermission:
		return "EXIT_PERMISSION"
	case ExitInternal:
		return "EXIT_INTERNAL"
	default:
		return "EXIT_FAILURE"
	}
}
