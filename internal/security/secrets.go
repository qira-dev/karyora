package security

import (
	"context"
	"errors"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Credential/secret references (C2-14 §4): the Core holds opaque handle
// references (names scoped per provider/connection) and resolves values
// only through the OS credential store at use time. The concrete store and
// any at-rest encryption are D-37 decisions and stay open — nothing here
// implies an encryption or key-loss policy. Values never enter prompts,
// Context, KnowledgePack, Memory, manifests, traces, crash reports, or
// logs; only handles cross internal boundaries.
type Handle struct {
	// Scope names the owning provider/connection (e.g. "provider:openai",
	// "connection:main-db"). It is an opaque reference, never a value.
	Scope string
	// Name is the handle name within the scope.
	Name string
}

func (h Handle) String() string { return h.Scope + "/" + h.Name }

func (h Handle) validate() error {
	if h.Scope == "" || h.Name == "" {
		return errors.New("security: handle scope and name are required")
	}
	if len(h.Scope) > maxIDLen || len(h.Name) > maxIDLen {
		return errors.New("security: handle reference is unbounded")
	}
	for _, part := range []string{h.Scope, h.Name} {
		if strings.ContainsAny(part, " \t\n\r\x00") {
			return errors.New("security: handle reference carries forbidden characters")
		}
	}
	return nil
}

// SecretStatus is the safe metadata a surface may disclose (masked status).
type SecretStatus string

const (
	SecretPresent    SecretStatus = "present"
	SecretMissing    SecretStatus = "missing"
	SecretRevoked    SecretStatus = "revoked"
	SecretCorrupt    SecretStatus = "corrupt"
	SecretInaccessib SecretStatus = "inaccessible"
)

// SecretStore is the resolution boundary. Resolve returns the value ONLY at
// the call boundary — callers must consume it for the immediate operation
// and never persist, log, or copy it into content. Status discloses safe
// metadata only. The concrete store (OS keychain, file, encryption) is the
// D-37 decision and stays open.
type SecretStore interface {
	Resolve(ctx context.Context, h Handle) ([]byte, error)
	Status(ctx context.Context, h Handle) (SecretStatus, error)
}

// UnavailableStore is the Phase 2 wired default: no concrete OS store is
// selected until D-37 is decided, so every resolution fails closed with
// the typed, user-actionable error — never a plaintext fallback, never a
// silent skip.
type UnavailableStore struct{}

// Resolve always fails closed with SECRET_INACCESSIBLE.
func (UnavailableStore) Resolve(ctx context.Context, h Handle) ([]byte, error) {
	return nil, &SecretError{Code: errs.CodeStorageUnavailable, Reason: ReasonSecretInaccessible, Handle: h.String()}
}

// Status always reports inaccessible (safe metadata, no value).
func (UnavailableStore) Status(ctx context.Context, h Handle) (SecretStatus, error) {
	return SecretInaccessib, &SecretError{Code: errs.CodeStorageUnavailable, Reason: ReasonSecretInaccessible, Handle: h.String()}
}

// Stable reason codes of the SECRET_UNAVAILABLE family (C2-14 §4). The
// family maps onto the closed C2-05 register — the register stays exactly
// the owner-accepted twelve codes; the reason code in data distinguishes
// the typed failure.
const (
	ReasonSecretMissing      = "SECRET_MISSING"
	ReasonSecretRevoked      = "SECRET_REVOKED"
	ReasonSecretInaccessible = "SECRET_INACCESSIBLE"
	ReasonSecretCorrupt      = "SECRET_CORRUPT"
	ReasonReenterCredential  = "REENTER_CREDENTIAL"
)

// SecretError is the typed, user-actionable secret failure. It carries the
// handle REFERENCE (a name — never a value) and the recovery path.
type SecretError struct {
	Code   errs.Code
	Reason string
	Handle string
}

func (e *SecretError) Error() string {
	return "security: secret " + e.Reason + " (" + e.Handle + ")"
}

// Envelope builds the wire envelope: register code + bounded typed data
// (reason code, handle reference, recovery path).
func (e *SecretError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(e.Code, correlationID, map[string]any{
		"reason":   e.Reason,
		"handle":   boundedString(e.Handle, maxIDLen*2+1),
		"recovery": ReasonReenterCredential,
	})
}

// errSecret builds the typed failure for a store-reported status.
func errSecret(h Handle, status SecretStatus) *SecretError {
	switch status {
	case SecretMissing:
		return &SecretError{Code: errs.CodeNotFound, Reason: ReasonSecretMissing, Handle: h.String()}
	case SecretRevoked:
		return &SecretError{Code: errs.CodeNotFound, Reason: ReasonSecretRevoked, Handle: h.String()}
	case SecretCorrupt:
		return &SecretError{Code: errs.CodeStateCorrupt, Reason: ReasonSecretCorrupt, Handle: h.String()}
	default:
		return &SecretError{Code: errs.CodeStorageUnavailable, Reason: ReasonSecretInaccessible, Handle: h.String()}
	}
}

// ResolveForUse is the one sanctioned resolution path: it asks the store
// for the value at the call boundary and maps every failure onto the typed
// family. A zero-length resolved value is treated as missing — an empty
// credential is never silently accepted.
func ResolveForUse(ctx context.Context, store SecretStore, h Handle) ([]byte, error) {
	if err := h.validate(); err != nil {
		return nil, &SecretError{Code: errs.CodeValidationFailed, Reason: ReasonSecretMissing, Handle: h.String()}
	}
	if store == nil {
		return nil, &SecretError{Code: errs.CodeStorageUnavailable, Reason: ReasonSecretInaccessible, Handle: h.String()}
	}
	v, err := store.Resolve(ctx, h)
	if err != nil {
		var se *SecretError
		if asSecretError(err, &se) {
			return nil, se
		}
		return nil, errSecret(h, SecretInaccessib)
	}
	if len(v) == 0 {
		return nil, errSecret(h, SecretMissing)
	}
	return v, nil
}

// asSecretError is a local type-assertion helper so this file does not
// depend on errors.As callers elsewhere.
func asSecretError(err error, target **SecretError) bool {
	for err != nil {
		if se, ok := err.(*SecretError); ok {
			*target = se
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
