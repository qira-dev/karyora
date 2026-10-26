package storage

import (
	"fmt"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Error is the adapter-boundary machine error: vendor types never cross
// into the runtime; this type carries only the stable C2-05 code plus one
// bounded machine-safe detail (no OS or DB vendor strings, no paths above
// the data directory, no raw upstream bodies).
type Error struct {
	Code   errs.Code
	Detail string
}

func (e *Error) Error() string {
	return fmt.Sprintf("storage: %s (%s)", e.Code, e.Detail)
}

// Envelope converts the boundary error into the wire envelope.
func (e *Error) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(e.Code, correlationID, map[string]any{"reason": e.Detail})
}

func errStoragef(code errs.Code, format string, args ...any) *Error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}
