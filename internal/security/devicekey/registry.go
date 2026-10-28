package devicekey

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// DefaultSupersedeGrace bounds how long a superseded key still verifies
// for in-flight receipt reconciliation after rotation (accepted model
// §2.3). The numeric stays PROPOSED — owner acceptance is explicitly
// pending in the accepted model — and is never presented as an authorized
// value; call sites that need a decided bound must receive one as an
// owner decision, not from this constant.
const DefaultSupersedeGrace = 72 * time.Hour

// PayloadV1 is the assertion payload profile version. The Assertion type
// is the v1 test vehicle for the key-state fixtures — NOT the COSE_Sign1
// receipt envelope (Proposed, unfrozen, model §2.6) and NOT the transfer
// receipt (C4-42, D-42-gated).
const PayloadV1 = 1

// MaxPayloadBytes bounds an assertion payload (the bounded-data rule).
const MaxPayloadBytes = 4096

// Purpose distinguishes the two authorization behaviors the accepted
// model §2.4 assigns differently to a superseded key: in-flight receipt
// reconciliation may verify inside the grace window, a NEW transfer may
// never start on a non-current key.
type Purpose string

const (
	PurposeReceiptReconciliation Purpose = "receipt.reconciliation"
	PurposeNewTransfer           Purpose = "transfer.start"
)

// Record is one server-side registry entry (accepted model §2.1): the
// key id, the public key, the binding tuple, the lifecycle state and the
// timestamps. It deliberately carries NO lease, fencing or authority
// epoch — the device-key identity is distinct from all of them, and the
// key-state fixtures hold that boundary by inspecting this shape.
type Record struct {
	KeyID        KeyID
	PublicKey    ed25519.PublicKey
	Binding      Binding
	State        State
	RegisteredAt time.Time
	ActivatedAt  time.Time // zero until Active
	SupersededAt time.Time // zero until Superseded
	GraceUntil   time.Time // zero unless Superseded
	NotAfter     time.Time // zero = no expiry set
	RevokedAt    time.Time // zero until Revoked
	CompromiseAt time.Time // zero unless a compromise was recorded
}

// effectiveState applies the clock to the stored state: an Active or
// Superseded key whose NotAfter has passed reads Expired at every CURRENT
// read (no caching, no sweep required for correctness).
func (rec Record) effectiveState(now time.Time) State {
	switch rec.State {
	case StateActive, StateSuperseded:
		if !rec.NotAfter.IsZero() && now.After(rec.NotAfter) {
			return StateExpired
		}
		return rec.State
	default:
		return rec.State
	}
}

// Ceremony carries the registration inputs the accepted model §2.2
// requires: an authenticated remote-session proof plus explicit user
// confirmation. v1 validates presence and bounds; the credential
// validation itself stays with the future remote session surface
// (TM-AUTH planned) — the same boundary as the teamauth adapter's
// presented principal.
type Ceremony struct {
	SessionProof  string
	UserConfirmed bool
}

func (c Ceremony) validate() error {
	if c.SessionProof == "" || len(c.SessionProof) > 4096 {
		return errors.New("devicekey: registration ceremony lacks a bounded session proof")
	}
	if !c.UserConfirmed {
		return errors.New("devicekey: registration ceremony lacks explicit user confirmation")
	}
	return nil
}

// Registry is the server-side registry semantics of the accepted model:
// it records bindings, states and timestamps and answers CURRENT state on
// every call — revocation and compromise are visible on the next
// verification, never cached. The durable PostgreSQL implementation lands
// with the remote surfaces (the same adapter→port direction as
// teamauth.Directory resolving into remotestore.StoreDirectory); this v1
// carries the complete state machine and verification rules behind a
// mutex-guarded store so the key-state fixtures execute.
type Registry struct {
	mu   sync.Mutex
	now  func() time.Time
	keys map[KeyID]Record

	// SupersedeGrace bounds the superseded-key reconciliation window. It
	// is a PROPOSED numeric (model §2.3) with the DefaultSupersedeGrace
	// default; a decided value must arrive as an owner decision.
	SupersedeGrace time.Duration
}

// NewRegistry returns the registry with the injected clock (nil →
// time.Now) and the proposed default grace window.
func NewRegistry(clock func() time.Time) *Registry {
	if clock == nil {
		clock = time.Now
	}
	return &Registry{now: clock, keys: make(map[KeyID]Record), SupersedeGrace: DefaultSupersedeGrace}
}

// Lookup resolves the CURRENT registry record, with the clock applied.
func (r *Registry) Lookup(id KeyID) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.keys[id]
	if !ok {
		return Record{}, false
	}
	rec.State = rec.effectiveState(r.now())
	return rec, true
}

// Register performs the registration ceremony (model §2.2): an
// authenticated session plus explicit user confirmation bind the
// presented public key to the (user, device, workspace) tuple. The key
// lands in Registered and is usable ONLY after Activate. Re-registering
// an existing key id fails closed — registry entries are never silently
// replaced.
func (r *Registry) Register(c Ceremony, pub ed25519.PublicKey, b Binding) (Record, error) {
	if err := c.validate(); err != nil {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	if err := b.validate(); err != nil {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	if len(pub) != ed25519.PublicKeySize {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	id := KeyIDFor(pub)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.keys[id]; exists {
		return Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: id}
	}
	rec := Record{
		KeyID:        id,
		PublicKey:    append(ed25519.PublicKey(nil), pub...),
		Binding:      b,
		State:        StateRegistered,
		RegisteredAt: r.now(),
	}
	r.keys[id] = rec
	return rec, nil
}

// Activate moves a Registered key to Active (the server-side activation
// of the ceremony). Every other source state fails closed.
func (r *Registry) Activate(id KeyID) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.keys[id]
	if !ok || rec.State != StateRegistered {
		return Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: id}
	}
	rec.State = StateActive
	rec.ActivatedAt = r.now()
	r.keys[id] = rec
	return rec, nil
}

// SetExpiry stamps a bounded validity window onto a non-revoked key (the
// Expired arm of the accepted state machine). A zero NotAfter clears the
// window.
func (r *Registry) SetExpiry(id KeyID, notAfter time.Time) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.keys[id]
	if !ok || rec.State == StateRevoked {
		return Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: id}
	}
	rec.NotAfter = notAfter
	r.keys[id] = rec
	return rec, nil
}

// Rotate performs the accepted rotation (model §2.3): the new key is
// registered AND activated against the same binding while the old key is
// healthy (Active), then the old key is marked Superseded with the grace
// window — receipts it signed still verify for in-flight reconciliation
// until GraceUntil, and no NEW transfer may start with it at any time.
// The ordering (new active before old superseded) leaves no window in
// which neither key is current.
func (r *Registry) Rotate(oldID KeyID, c Ceremony, newPub ed25519.PublicKey) (old, new Record, err error) {
	if err := c.validate(); err != nil {
		return Record{}, Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	if len(newPub) != ed25519.PublicKeySize {
		return Record{}, Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	newID := KeyIDFor(newPub)
	if newID == oldID {
		return Record{}, Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid, KeyID: oldID}
	}
	now := r.now()
	grace := r.SupersedeGrace
	r.mu.Lock()
	defer r.mu.Unlock()
	oldRec, ok := r.keys[oldID]
	if !ok || oldRec.effectiveState(now) != StateActive {
		return Record{}, Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: oldID}
	}
	if _, exists := r.keys[newID]; exists {
		return Record{}, Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: newID}
	}
	newRec := Record{
		KeyID:        newID,
		PublicKey:    append(ed25519.PublicKey(nil), newPub...),
		Binding:      oldRec.Binding,
		State:        StateActive,
		RegisteredAt: now,
		ActivatedAt:  now,
	}
	oldRec.State = StateSuperseded
	oldRec.SupersededAt = now
	oldRec.GraceUntil = now.Add(grace)
	r.keys[newID] = newRec
	r.keys[oldID] = oldRec
	return oldRec, newRec, nil
}

// Revoke marks a key REVOKED — terminal, from any state, effective on the
// next CURRENT read (model §2.4). A non-nil compromise timestamp records
// the compromise time: receipts signed at or after it are untrusted even
// if signatures verify. Revoking an already-revoked key is idempotent.
func (r *Registry) Revoke(id KeyID, compromise *time.Time) (Record, error) {
	now := r.now()
	if compromise != nil && compromise.After(now) {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid, KeyID: id}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.keys[id]
	if !ok {
		return Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: id}
	}
	if rec.State == StateRevoked {
		return rec, nil
	}
	rec.State = StateRevoked
	rec.RevokedAt = now
	if compromise != nil {
		rec.CompromiseAt = *compromise
	}
	r.keys[id] = rec
	return rec, nil
}

// Reenroll performs the accepted re-enrollment (model §2.5): there is no
// private-key recovery path — key loss means a NEW keypair registered
// against the SAME binding tuple while the old key is revoked server-side
// (terminal). The new key starts Registered and activates through the
// normal ceremony. Rate limiting and audit trails belong to the remote
// session/administration surface (D-36 administration; TM-AUTH planned) —
// recorded as a boundary, not implemented here.
func (r *Registry) Reenroll(oldID KeyID, c Ceremony, newPub ed25519.PublicKey) (Record, error) {
	if err := c.validate(); err != nil {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	if len(newPub) != ed25519.PublicKeySize {
		return Record{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	r.mu.Lock()
	oldRec, ok := r.keys[oldID]
	r.mu.Unlock()
	if !ok {
		return Record{}, &DeviceKeyError{Code: errs.CodeConflictState, Reason: ReasonKeyStateInvalid, KeyID: oldID}
	}
	if _, err := r.Revoke(oldID, nil); err != nil {
		return Record{}, err
	}
	return r.Register(c, newPub, oldRec.Binding)
}

// Assertion is the v1 signed assertion: a versioned deterministic byte
// payload, the signing key id, the signing time, and the Ed25519
// signature over the canonical encoding of all four. It is the test
// vehicle for the key-state fixtures — the COSE_Sign1 receipt envelope
// and the transfer receipt profile stay Proposed/unfrozen and
// D-42-gated respectively.
type Assertion struct {
	Version   int
	KeyID     KeyID
	Payload   []byte
	SignedAt  time.Time
	Signature []byte
}

func (a Assertion) validate() error {
	if a.Version != PayloadV1 {
		return errors.New("devicekey: assertion payload version is unknown")
	}
	if err := a.KeyID.Validate(); err != nil {
		return errors.New("devicekey: assertion lacks a bounded key id")
	}
	if len(a.Payload) == 0 || len(a.Payload) > MaxPayloadBytes {
		return errors.New("devicekey: assertion payload is missing or unbounded")
	}
	if a.SignedAt.IsZero() {
		return errors.New("devicekey: assertion lacks a signing time")
	}
	return nil
}

// canonical encodes the signed bytes deterministically: version (u32
// big-endian) ‖ key id (fixed-length bytes) ‖ unix seconds (u64
// big-endian) ‖ payload. Equivalent re-encodings cannot exist — the
// profile is fixed bytes, which is what keeps the v1 vehicle
// deterministic (the CBOR determinism concern of model §2.6 belongs to
// the unfrozen envelope).
func (a Assertion) canonical() []byte {
	out := make([]byte, 0, 4+len(a.KeyID)+8+len(a.Payload))
	out = binary.BigEndian.AppendUint32(out, uint32(a.Version))
	out = append(out, []byte(a.KeyID)...)
	out = binary.BigEndian.AppendUint64(out, uint64(a.SignedAt.Unix()))
	return append(out, a.Payload...)
}

// VerifyResult is the outcome of verification. It carries the allow/deny
// decision, the typed reason, and the key id — and NO epoch of any kind.
type VerifyResult struct {
	Allow   bool
	Reason  string
	KeyID   KeyID
	Purpose Purpose
}

// Verify applies the accepted model's dual rule in order: possession
// (the signature verifies against the REGISTERED public key) then
// authorization (the CURRENT registry state permits the purpose). Every
// state of the model's deny table denies with its typed reason; a valid
// signature alone never authorizes. An unknown key id denies before any
// signature check — a self-signed assertion over an unregistered key
// proves nothing.
func (r *Registry) Verify(a Assertion, purpose Purpose) (VerifyResult, error) {
	if err := a.validate(); err != nil {
		return VerifyResult{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	if purpose != PurposeReceiptReconciliation && purpose != PurposeNewTransfer {
		return VerifyResult{}, &DeviceKeyError{Code: errs.CodeValidationFailed, Reason: ReasonKeyStateInvalid}
	}
	now := r.now()
	if a.SignedAt.After(now) {
		// Future-signed assertion: fail closed (no skew tolerance in v1;
		// skew handling belongs to the unfrozen protocol profile).
		return VerifyResult{Allow: false, Reason: ReasonKeySignatureInvalid, KeyID: a.KeyID, Purpose: purpose}, nil
	}
	rec, ok := r.Lookup(a.KeyID)
	if !ok {
		return VerifyResult{Allow: false, Reason: ReasonKeyUnregistered, KeyID: a.KeyID, Purpose: purpose}, nil
	}
	if !ed25519.Verify(rec.PublicKey, a.canonical(), a.Signature) {
		return VerifyResult{Allow: false, Reason: ReasonKeySignatureInvalid, KeyID: rec.KeyID, Purpose: purpose}, nil
	}
	switch rec.State {
	case StateRevoked:
		return VerifyResult{Allow: false, Reason: ReasonKeyRevoked, KeyID: rec.KeyID, Purpose: purpose}, nil
	case StateRegistered:
		return VerifyResult{Allow: false, Reason: ReasonKeyNotActive, KeyID: rec.KeyID, Purpose: purpose}, nil
	case StateExpired:
		return VerifyResult{Allow: false, Reason: ReasonKeyNotCurrent, KeyID: rec.KeyID, Purpose: purpose}, nil
	case StateSuperseded:
		withinGrace := !rec.GraceUntil.IsZero() && !now.After(rec.GraceUntil)
		if withinGrace && purpose == PurposeReceiptReconciliation {
			return VerifyResult{Allow: true, Reason: security.ReasonOK, KeyID: rec.KeyID, Purpose: purpose}, nil
		}
		return VerifyResult{Allow: false, Reason: ReasonKeyNotCurrent, KeyID: rec.KeyID, Purpose: purpose}, nil
	case StateActive:
		return VerifyResult{Allow: true, Reason: security.ReasonOK, KeyID: rec.KeyID, Purpose: purpose}, nil
	default:
		return VerifyResult{Allow: false, Reason: ReasonKeyUnregistered, KeyID: rec.KeyID, Purpose: purpose}, nil
	}
}
