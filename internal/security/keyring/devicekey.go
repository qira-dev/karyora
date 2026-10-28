package keyring

import (
	"crypto/ed25519"
	"errors"

	zkeyring "github.com/zalando/go-keyring"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security/devicekey"
)

// DeviceKeySource is the accepted D-38 private-key storage for device
// signing keys: the accepted D-37 posture (this package's concrete store)
// extended to the device-key class per the owner-accepted lifecycle model
// §2.1. The private key persists ONLY in the OS credential store —
// Windows Credential Manager/DPAPI, macOS Keychain, Linux Secret
// Service/keyring — through the same confined go-keyring backend. There
// is no file, plaintext, or obfuscated fallback: where no OS keyring is
// reachable every operation fails closed with the typed KEY_UNAVAILABLE
// family, which is the accepted correct state (for example on headless
// Linux), never a degraded mode. There is no product-side recovery copy
// and no export path: key loss means re-enrollment (model §2.5). Key
// material is returned only at the call boundary and never enters an
// error string.
//
// The type satisfies devicekey.PrivateKeySource structurally — the
// implementer imports the port package, the adapter→port direction of the
// dependency map.
type DeviceKeySource struct{}

// NewDeviceKeySource returns the accepted D-38 concrete source.
func NewDeviceKeySource() *DeviceKeySource { return &DeviceKeySource{} }

// deviceKeyUserPrefix namespaces the device-key item class under the same
// stable service name as the D-37 secret items. Only the key id — a
// bounded SHA-256 hex identifier — crosses into the item attributes,
// never key material.
const deviceKeyUserPrefix = "devicekey/"

func deviceKeyItem(id devicekey.KeyID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", &devicekey.DeviceKeyError{Code: errs.CodeValidationFailed, Reason: devicekey.ReasonKeyStateInvalid, KeyID: id}
	}
	return deviceKeyUserPrefix + string(id), nil
}

// StoreDeviceKey persists the private key under the registry id (the
// re-enrollment/registration ceremony's local half). The 64-byte
// Ed25519 form is required; anything else is refused before the store.
func (DeviceKeySource) StoreDeviceKey(id devicekey.KeyID, key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return &devicekey.DeviceKeyError{Code: errs.CodeValidationFailed, Reason: devicekey.ReasonKeyStateInvalid, KeyID: id}
	}
	user, err := deviceKeyItem(id)
	if err != nil {
		return err
	}
	if err := backend.Set(Service, user, string(key)); err != nil {
		return mapDeviceKeyStoreError(id, err)
	}
	return nil
}

// LoadDeviceKey returns the private key ONLY at the call boundary. A
// missing item, an empty stored value, and a wrong-length value all fail
// closed — the caller folds every failure onto the typed
// KEY_UNAVAILABLE denial, so no malformed state can degrade into a
// weaker identity.
func (DeviceKeySource) LoadDeviceKey(id devicekey.KeyID) (ed25519.PrivateKey, error) {
	user, err := deviceKeyItem(id)
	if err != nil {
		return nil, err
	}
	v, err := backend.Get(Service, user)
	if err != nil {
		return nil, mapDeviceKeyStoreError(id, err)
	}
	if len(v) != ed25519.PrivateKeySize {
		return nil, &devicekey.DeviceKeyError{Code: errs.CodeStateCorrupt, Reason: devicekey.ReasonKeyUnavailable, KeyID: id}
	}
	return ed25519.PrivateKey(v), nil
}

// DeleteDeviceKey removes the item (the loss/re-enrollment path).
// Deleting an already-absent item is idempotent — the removed state is
// the goal.
func (DeviceKeySource) DeleteDeviceKey(id devicekey.KeyID) error {
	user, err := deviceKeyItem(id)
	if err != nil {
		return err
	}
	if err := backend.Delete(Service, user); err != nil && !errors.Is(err, zkeyring.ErrNotFound) {
		return mapDeviceKeyStoreError(id, err)
	}
	return nil
}

// mapDeviceKeyStoreError folds every keyring failure onto the typed,
// fail-closed device-key family. The underlying error text is NOT
// embedded — error surfaces stay value-free, and the reason code carries
// the distinction. Unreachable session bus, denied access, and a locked
// collection all fail closed as unavailable (the accepted correct state
// where no keyring exists).
func mapDeviceKeyStoreError(id devicekey.KeyID, err error) *devicekey.DeviceKeyError {
	return &devicekey.DeviceKeyError{Code: errs.CodeStorageUnavailable, Reason: devicekey.ReasonKeyUnavailable, KeyID: id}
}
