// Package keyring is the concrete D-37 store (owner decision, 2026-10-28):
// provider BYOK credentials persist ONLY in the OS credential store —
// Windows Credential Manager, macOS Keychain, Linux Secret Service/keyring —
// through github.com/zalando/go-keyring. There is no file, plaintext, or
// obfuscated fallback: where no OS keyring is reachable every operation
// fails closed with the typed SECRET_UNAVAILABLE family, which is the
// accepted correct state (for example on headless Linux), never a degraded
// mode. Values live only in the OS store; they are returned only at the
// call boundary and never enter an error string.
package keyring

import (
	"context"
	"errors"

	zkeyring "github.com/zalando/go-keyring"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// Service is the stable, language-neutral keyring service name under which
// the product stores its secret items — a machine identifier, never
// localized (the same rule as the CLI and exit-code registers).
const Service = "karyora"

// backend is the swappable keyring seam: the go-keyring package functions
// in production, controlled test doubles in the test file. Production code
// never swaps it.
var backend keyringBackend = zkeyringBackend{}

type keyringBackend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type zkeyringBackend struct{}

func (zkeyringBackend) Set(service, user, password string) error {
	return zkeyring.Set(service, user, password)
}

func (zkeyringBackend) Get(service, user string) (string, error) {
	return zkeyring.Get(service, user)
}

func (zkeyringBackend) Delete(service, user string) error {
	return zkeyring.Delete(service, user)
}

// Store is the OS keyring behind the security secret ports: Resolve/Status
// satisfy security.SecretStore, Store/Revoke satisfy security.SecretWriter
// (lifecycle create → use → replace → revoke; deleting the owning
// connection deletes its handles).
type Store struct{}

// New returns the accepted D-37 concrete store.
func New() *Store { return &Store{} }

// itemUser validates the handle reference and maps it onto keyring item
// attributes: the fixed service name plus the scope/name user key. Only the
// reference crosses into the attributes — never a value.
func itemUser(h security.Handle) (string, error) {
	if err := h.Validate(); err != nil {
		return "", &security.SecretError{Code: errs.CodeValidationFailed, Reason: security.ReasonSecretMissing, Handle: h.String()}
	}
	return h.Scope + "/" + h.Name, nil
}

// Store writes the value under the handle reference (create, or replace —
// the atomic same-handle overwrite is the accepted rotation semantics).
func (Store) Store(_ context.Context, h security.Handle, value []byte) error {
	if len(value) == 0 {
		return &security.SecretError{Code: errs.CodeValidationFailed, Reason: security.ReasonSecretMissing, Handle: h.String()}
	}
	user, err := itemUser(h)
	if err != nil {
		return err
	}
	if err := backend.Set(Service, user, string(value)); err != nil {
		return mapStoreError(h, err)
	}
	return nil
}

// Revoke deletes the item; dependent resolution then fails closed. Deleting
// an already-absent handle is idempotent — the revoked state is the goal.
func (Store) Revoke(_ context.Context, h security.Handle) error {
	user, err := itemUser(h)
	if err != nil {
		return err
	}
	if err := backend.Delete(Service, user); err != nil && !errors.Is(err, zkeyring.ErrNotFound) {
		return mapStoreError(h, err)
	}
	return nil
}

// Resolve returns the value ONLY at the call boundary. A missing item and
// an empty stored value both produce the typed SECRET_MISSING failure — an
// empty credential is never silently accepted.
func (Store) Resolve(_ context.Context, h security.Handle) ([]byte, error) {
	user, err := itemUser(h)
	if err != nil {
		return nil, err
	}
	v, err := backend.Get(Service, user)
	if err != nil {
		if errors.Is(err, zkeyring.ErrNotFound) {
			return nil, &security.SecretError{Code: errs.CodeNotFound, Reason: security.ReasonSecretMissing, Handle: h.String()}
		}
		return nil, mapStoreError(h, err)
	}
	if v == "" {
		return nil, &security.SecretError{Code: errs.CodeNotFound, Reason: security.ReasonSecretMissing, Handle: h.String()}
	}
	return []byte(v), nil
}

// Status discloses safe metadata only: present/missing from the keyring
// answer; inaccessible with the typed error when the keyring itself is
// unreachable (for example headless Linux without a Secret Service).
func (Store) Status(_ context.Context, h security.Handle) (security.SecretStatus, error) {
	user, err := itemUser(h)
	if err != nil {
		return security.SecretInaccessib, err
	}
	if _, err := backend.Get(Service, user); err != nil {
		if errors.Is(err, zkeyring.ErrNotFound) {
			return security.SecretMissing, nil
		}
		return security.SecretInaccessib, mapStoreError(h, err)
	}
	return security.SecretPresent, nil
}

// mapStoreError folds every keyring failure onto the typed, user-actionable
// family over the closed register. The underlying error text is NOT
// embedded — error surfaces stay value-free, and the reason code carries
// the distinction. ErrUnsupportedPlatform, an unreachable session bus,
// denied access, and a locked collection all fail closed as inaccessible.
func mapStoreError(h security.Handle, err error) *security.SecretError {
	switch {
	case errors.Is(err, zkeyring.ErrSetDataTooBig):
		return &security.SecretError{Code: errs.CodeValidationFailed, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
	default:
		return &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
	}
}
