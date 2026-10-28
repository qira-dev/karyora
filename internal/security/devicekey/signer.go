package devicekey

import (
	"crypto/ed25519"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// PrivateKeySource is the fail-closed storage port for the device's own
// private key material. The accepted D-38 storage posture is the accepted
// D-37 posture extended to this key class: the OS credential store ONLY
// (Windows Credential Manager/DPAPI, macOS Keychain, Linux keyring), no
// fallback of any kind, no product-side recovery copy, no export through
// product flows; where no keyring exists every operation fails closed.
// The concrete implementation is internal/security/keyring's
// DeviceKeySource (the go-keyring import stays confined there); fixtures
// provide in-memory doubles.
type PrivateKeySource interface {
	// StoreDeviceKey persists the private key under the registry id.
	StoreDeviceKey(id KeyID, key ed25519.PrivateKey) error
	// LoadDeviceKey returns the private key; ANY failure — including a
	// missing item or an unreachable keyring — is an error, never a
	// degraded substitute.
	LoadDeviceKey(id KeyID) (ed25519.PrivateKey, error)
	// DeleteDeviceKey removes the item (the loss/re-enrollment path; an
	// absent item is the goal state and stays idempotent).
	DeleteDeviceKey(id KeyID) error
}

// Signer is the local half of the device-key identity: it holds the
// binding the key was registered against and produces signed assertions
// from the keyring-held private key. It carries no authority epoch and
// grants nothing — signing is the identity's ONLY act (accepted model
// §2.1: the device-key identity signs handoff receipts only).
type Signer struct {
	source     PrivateKeySource
	keyID      KeyID
	Binding    Binding
	now        func() time.Time
	maxPayload int
}

// NewSigner returns a signer over the source-held key. The key id is
// derived from the presented public key (the registry derives the same
// id), and the source must already hold the matching private key.
func NewSigner(source PrivateKeySource, pub ed25519.PublicKey, b Binding) *Signer {
	return &Signer{
		source:     source,
		keyID:      KeyIDFor(pub),
		Binding:    b,
		now:        time.Now,
		maxPayload: MaxPayloadBytes,
	}
}

// ID returns the signer's registry key id.
func (s *Signer) ID() KeyID { return s.keyID }

// Sign produces a signed assertion over the bounded caller payload. A
// source failure — missing item, unreachable keyring, locked collection —
// fails closed with the typed KEY_UNAVAILABLE denial: an unrecoverable
// key cannot authorize, and no weaker identity is ever substituted
// (accepted model §2.4). The signing time is the signer's clock.
func (s *Signer) Sign(payload []byte) (Assertion, error) {
	if len(payload) == 0 || len(payload) > s.maxPayload {
		return Assertion{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid, KeyID: s.keyID}
	}
	priv, err := s.source.LoadDeviceKey(s.keyID)
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		// Every source failure is unavailability — the underlying text
		// never crosses this boundary.
		return Assertion{}, &DeviceKeyError{Code: errs.CodeStorageUnavailable, Reason: ReasonKeyUnavailable, KeyID: s.keyID}
	}
	a := Assertion{
		Version:  PayloadV1,
		KeyID:    s.keyID,
		Payload:  append([]byte(nil), payload...),
		SignedAt: s.now().Truncate(time.Second),
	}
	a.Signature = ed25519.Sign(priv, a.canonical())
	return a, nil
}

// InMemorySource is the test double for the key-state fixtures; it
// reproduces the port's fail-closed contract, including a forced
// unavailability mode for the unrecoverable-key row.
type InMemorySource struct {
	items    map[KeyID]ed25519.PrivateKey
	FailNext bool // when set, the next load fails like an unreachable keyring
}

func NewInMemorySource() *InMemorySource {
	return &InMemorySource{items: make(map[KeyID]ed25519.PrivateKey)}
}

func (m *InMemorySource) StoreDeviceKey(id KeyID, key ed25519.PrivateKey) error {
	if err := id.Validate(); err != nil {
		return err
	}
	m.items[id] = append(ed25519.PrivateKey(nil), key...)
	return nil
}

func (m *InMemorySource) LoadDeviceKey(id KeyID) (ed25519.PrivateKey, error) {
	if m.FailNext {
		m.FailNext = false
		return nil, errUnavailable
	}
	key, ok := m.items[id]
	if !ok {
		return nil, errUnavailable
	}
	return key, nil
}

func (m *InMemorySource) DeleteDeviceKey(id KeyID) error {
	delete(m.items, id)
	return nil
}

type fixedError string

func (e fixedError) Error() string { return string(e) }

const errUnavailable = fixedError("in-memory source: unavailable")
