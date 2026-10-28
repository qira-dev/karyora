// Package devicekey implements the owner-accepted D-38 device-key identity
// lifecycle model v1 (Phase 4 C4-41, PROVE half; Option A accepted
// 2026-10-28 via the structured decision prompt): register/bind/protect/
// rotate/revoke/recover for the transfer-signing device identity.
//
// The model's core rule is that a signature proves possession and the
// server-side registry state proves authorization — BOTH must pass and
// neither substitutes for the other. Every state the accepted model
// denies is denied here with the model's typed reasons: an unregistered,
// self-signed-only, registered-but-not-active, expired, superseded (for
// new transfers) or revoked key cannot authorize, a compromised key's
// post-compromise receipts are untrusted even when the signature
// verifies, and an unrecoverable key (no OS keyring) fails closed with
// no fallback of any kind.
//
// Status boundary (the D-38 evidence milestone): the lifecycle and the
// key-state verification rules are real code with the key-state fixtures
// executed. The package is NOT wired into any product surface, no remote
// session surface exists (the registration ceremony's credential
// validation stays with it — TM-AUTH planned), and the two-party
// authority-transfer protocol is NOT implemented: active local↔remote
// transfer stays UNSUPPORTED until D-42 fencing/recovery proof and the
// CR-4.5 conformance evidence also pass. The COSE_Sign1 receipt envelope
// stays Proposed and unfrozen per the accepted model §2.6 — the Assertion
// type here is a minimal versioned deterministic test vehicle, never an
// interoperability claim. Local mode is byte-for-byte unchanged.
package devicekey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// maxIDLen bounds every identifier crossing this package (the same
// discipline as the security package).
const maxIDLen = 128

// KeyID is the stable registry identifier of one device key: the hex
// SHA-256 of the public key bytes. It is a distinct named type — the
// device-key identity is NOT a lease id, NOT a worker id, NOT a fencing
// epoch and NOT an authority epoch (accepted model §2.1); the type system
// keeps those worlds apart and the registry carries no epoch of any kind.
type KeyID string

// Validate bounds the identifier.
func (k KeyID) Validate() error {
	if k == "" || len(k) > maxIDLen {
		return errors.New("devicekey: key id is missing or unbounded")
	}
	return nil
}

// Binding is the ONLY tuple a key may be registered against (accepted
// model §2.1): user identity, device record, workspace membership. A key
// registered outside this tuple does not exist; a key alone, or a
// self-signed assertion alone, proves nothing.
type Binding struct {
	UserID      string
	DeviceID    string
	WorkspaceID string
}

func (b Binding) validate() error {
	for _, id := range []string{b.UserID, b.DeviceID, b.WorkspaceID} {
		if id == "" || len(id) > maxIDLen {
			return errors.New("devicekey: binding tuple member is missing or unbounded")
		}
	}
	return nil
}

// State is the accepted lifecycle state machine v1 (model §2.2):
// Registered → Active → (Superseded | Revoked | Expired); Revoked is
// terminal.
type State string

const (
	StateRegistered State = "registered"
	StateActive     State = "active"
	StateSuperseded State = "superseded"
	StateRevoked    State = "revoked"
	StateExpired    State = "expired"
)

// Verification denial reasons of the accepted model §2.4, mapped onto the
// Core reason-code convention (stable, bounded, language-neutral
// uppercase; the model's informal lowercase spellings are noted).
const (
	// ReasonKeyUnregistered (model: key_unregistered) — no registry entry:
	// an unregistered or self-signed-only key.
	ReasonKeyUnregistered = "KEY_UNREGISTERED"
	// ReasonKeyNotActive (model: key_not_active) — Registered but not yet
	// activated by the server-side ceremony.
	ReasonKeyNotActive = "KEY_NOT_ACTIVE"
	// ReasonKeyNotCurrent (model: key_not_current) — Superseded beyond its
	// grace window, Superseded for a NEW transfer, or Expired.
	ReasonKeyNotCurrent = "KEY_NOT_CURRENT"
	// ReasonKeyRevoked (model: key_revoked) — terminal revocation,
	// including the compromise rule: receipts signed after the recorded
	// compromise timestamp are untrusted even if signatures verify.
	ReasonKeyRevoked = "KEY_REVOKED"
	// ReasonKeyUnavailable (model: key_unavailable) — the local private
	// key could not be read from the OS credential store: fail closed, no
	// silent fallback to any weaker identity.
	ReasonKeyUnavailable = "KEY_UNAVAILABLE"
	// ReasonKeySignatureInvalid — the possession half of verification
	// failed: the presented signature does not verify against the
	// registered public key. Possession failure never authorizes.
	ReasonKeySignatureInvalid = "KEY_SIGNATURE_INVALID"
	// ReasonKeyStateInvalid is the lifecycle-transition guard for registry
	// operations (not a verification denial): an operation applied to a
	// record whose current state forbids it, or a duplicate registration.
	ReasonKeyStateInvalid = "KEY_STATE_INVALID"
)

// DeviceKeyError is the typed, user-actionable failure. It carries the
// stable register code and the bounded reason only — never key material,
// registry contents, or error-chain text.
type DeviceKeyError struct {
	Code   errs.Code
	Reason string
	KeyID  KeyID
}

func (e *DeviceKeyError) Error() string {
	id := string(e.KeyID)
	if len(id) > 12 {
		id = id[:12]
	}
	return "devicekey: " + e.Reason + " (" + id + ")"
}

// Envelope builds the machine error for the failure envelope contract.
func (e *DeviceKeyError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(e.Code, correlationID, map[string]any{
		"reason": e.Reason,
		"keyId":  bounded(string(e.KeyID), maxIDLen),
	})
}

func bounded(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

// Generate creates a new device signing keypair: Ed25519 (the
// implementation-level algorithm selection under the accepted model,
// rechecked 2026-10-29 — EdDSA carries COSE algorithm identifier −8 per
// RFC 9053 and the IANA COSE Algorithms registry; Go stdlib covers it
// with zero new dependencies). The private half goes ONLY into the
// accepted OS-keyring source (PrivateKeySource) — never a file, never a
// fallback, no product-side recovery copy.
func Generate() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, &DeviceKeyError{Code: errs.CodeInternal, Reason: ReasonKeyUnavailable}
	}
	return pub, priv, nil
}

// KeyIDFor derives the registry identifier from a public key.
func KeyIDFor(pub ed25519.PublicKey) KeyID {
	sum := sha256.Sum256(pub)
	return KeyID(hex.EncodeToString(sum[:]))
}
