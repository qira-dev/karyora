// Package errs defines the Core machine-error contract: the owner-accepted
// wire field set (code, retryable, correlationId, bounded data) and the
// Phase 2 code register from the shared-contracts specification (C2-05).
// The package is part of the domain leaf: it imports no other internal
// package and carries no OS, database, or transport types. HTTP maps this
// envelope onto RFC 9457 Problem Details without title/detail; localized
// presentation copy never travels through this package.
package errs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Code is a stable, language-neutral Core error identifier.
type Code string

// The Phase 2 code register (shared-contracts specification, C2-05).
const (
	CodeValidationFailed       Code = "VALIDATION_FAILED"
	CodeConfigConflict         Code = "CONFIG_CONFLICT"
	CodeAuthzDenied            Code = "AUTHZ_DENIED"
	CodeNotFound               Code = "NOT_FOUND"
	CodeConflictState          Code = "CONFLICT_STATE"
	CodeSingleInstanceConflict Code = "SINGLE_INSTANCE_CONFLICT"
	CodeStorageUnavailable     Code = "STORAGE_UNAVAILABLE"
	CodeMigrationFailed        Code = "MIGRATION_FAILED"
	CodeStateCorrupt           Code = "STATE_CORRUPT"
	CodeRecoveryRequired       Code = "RECOVERY_REQUIRED"
	CodeRateLimited            Code = "RATE_LIMITED"
	CodeInternal               Code = "INTERNAL"
)

// RetryClass is the operation-level retry classification. On the wire it
// travels inside Data under the reserved "retryClass" key so the accepted
// envelope field set stays exactly the owner-approved four fields (D-35).
type RetryClass string

const (
	RetryNever     RetryClass = "never"
	RetrySafe      RetryClass = "safe-retry"
	RetryBackoff   RetryClass = "backoff"
	RetryReconcile RetryClass = "reconcile"
)

type codeDefault struct {
	retryable bool
	class     RetryClass
}

// register is the single source of Phase 2 code semantics.
var register = map[Code]codeDefault{
	CodeValidationFailed:       {retryable: false, class: RetryNever},
	CodeConfigConflict:         {retryable: false, class: RetryNever},
	CodeAuthzDenied:            {retryable: false, class: RetryNever},
	CodeNotFound:               {retryable: false, class: RetryNever},
	CodeConflictState:          {retryable: false, class: RetryNever},
	CodeSingleInstanceConflict: {retryable: true, class: RetryBackoff},
	CodeStorageUnavailable:     {retryable: true, class: RetryBackoff},
	CodeMigrationFailed:        {retryable: true, class: RetryReconcile},
	CodeStateCorrupt:           {retryable: true, class: RetryReconcile},
	CodeRecoveryRequired:       {retryable: true, class: RetryReconcile},
	CodeRateLimited:            {retryable: true, class: RetryBackoff},
	CodeInternal:               {retryable: true, class: RetrySafe},
}

// RetryClassOf reports the registered default classification for a code and
// whether the code is known at all.
func RetryClassOf(code Code) (RetryClass, bool) {
	d, ok := register[code]
	return d.class, ok
}

// Bounds enforce the bounded-data rule of the contract: an envelope never
// carries unbounded content.
const (
	MaxDataKeys       = 16
	MaxDataValueBytes = 512
)

// reservedDataKeys are set by the register, never by callers.
var reservedDataKeys = map[string]bool{
	"retryClass": true,
}

// Envelope is the machine error contract. JSON field names are the
// owner-accepted set; Data carries bounded typed members only (field/reason
// codes — never prose, secrets, stack traces, or raw upstream bodies).
type Envelope struct {
	Code          Code           `json:"code"`
	Retryable     bool           `json:"retryable"`
	CorrelationID string         `json:"correlationId"`
	Data          map[string]any `json:"data"`
}

// New builds a validated envelope for a known code. It fails closed on
// unknown codes, empty correlation IDs, reserved data keys, or unbounded
// data rather than emitting a contract violation.
func New(code Code, correlationID string, data map[string]any) (Envelope, error) {
	d, ok := register[code]
	if !ok {
		return Envelope{}, fmt.Errorf("errs: unknown code %q", string(code))
	}
	if correlationID == "" {
		return Envelope{}, fmt.Errorf("errs: correlationId is required for code %q", string(code))
	}
	clean := make(map[string]any, len(data)+1)
	for k, v := range data {
		if reservedDataKeys[k] {
			return Envelope{}, fmt.Errorf("errs: data key %q is reserved", k)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return Envelope{}, fmt.Errorf("errs: data[%q] is not encodable", k)
		}
		if len(b) > MaxDataValueBytes {
			return Envelope{}, fmt.Errorf("errs: data[%q] exceeds %d bytes", k, MaxDataValueBytes)
		}
		clean[k] = v
	}
	clean["retryClass"] = string(d.class)
	if len(clean) > MaxDataKeys {
		return Envelope{}, fmt.Errorf("errs: data exceeds %d keys", MaxDataKeys)
	}
	return Envelope{Code: code, Retryable: d.retryable, CorrelationID: correlationID, Data: clean}, nil
}

// NewCorrelationID returns a fresh random correlation identifier, or an
// empty string when the platform entropy source is unavailable; callers
// fail closed through New on an empty ID.
func NewCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
