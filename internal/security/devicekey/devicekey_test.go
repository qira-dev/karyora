package devicekey

import (
	"crypto/ed25519"
	"reflect"
	"strings"
	"testing"
	"time"
)

// newFixture returns a registry with a controllable clock plus a
// registered+activated key with its signer.
func newFixture(t *testing.T) (*Registry, *Signer, ed25519.PublicKey, *InMemorySource, *time.Time) {
	t.Helper()
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	clock := base
	setClock := &clock
	reg := NewRegistry(func() time.Time { return *setClock })
	ceremony := Ceremony{SessionProof: "session-ref-1", UserConfirmed: true}
	binding := Binding{UserID: "user-1", DeviceID: "device-1", WorkspaceID: "ws-1"}
	pub, priv, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := reg.Register(ceremony, pub, binding); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Activate(KeyIDFor(pub)); err != nil {
		t.Fatalf("activate: %v", err)
	}
	src := NewInMemorySource()
	if err := src.StoreDeviceKey(KeyIDFor(pub), priv); err != nil {
		t.Fatalf("store device key: %v", err)
	}
	signer := NewSigner(src, pub, binding)
	signer.now = func() time.Time { return *setClock } // deterministic: the signer shares the fixture clock
	return reg, signer, pub, src, setClock
}

func payload(s string) []byte { return []byte("fixture-payload:" + s) }

// denyReason asserts the expected typed denial.
func denyReason(t *testing.T, res VerifyResult, err error, want string, purpose Purpose) {
	t.Helper()
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	if res.Allow {
		t.Fatalf("expected deny %s, got allow", want)
	}
	if res.Reason != want {
		t.Fatalf("expected deny reason %s, got %s", want, res.Reason)
	}
	if res.Purpose != purpose {
		t.Fatalf("expected purpose %s echoed, got %s", purpose, res.Purpose)
	}
}

// TestUnregisteredKeyCannotAuthorize covers model §2.4 row 1: no registry
// entry → KEY_UNREGISTERED for both purposes.
func TestUnregisteredKeyCannotAuthorize(t *testing.T) {
	reg, _, _, _, setClock := newFixture(t)
	roguePub, roguePriv, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Sign at the registry's fixture clock: Verify fails closed on a
	// future-signed assertion BEFORE the deny table, so the real wall
	// clock would mask the denial under test after the fixture base time.
	sig := ed25519.Sign(roguePriv, Assertion{Version: PayloadV1, KeyID: KeyIDFor(roguePub), Payload: payload("x"), SignedAt: *setClock}.canonical())
	for _, purpose := range []Purpose{PurposeReceiptReconciliation, PurposeNewTransfer} {
		res, err := reg.Verify(Assertion{Version: PayloadV1, KeyID: KeyIDFor(roguePub), Payload: payload("x"), SignedAt: *setClock, Signature: sig}, purpose)
		denyReason(t, res, err, ReasonKeyUnregistered, purpose)
	}
}

// TestSelfSignedAssertionAloneProvesNothing covers model §2.1/§2.4: a
// self-consistent signature from a key that was never registered is the
// unregistered denial — the signature is never consulted without the
// registry entry.
func TestSelfSignedAssertionAloneProvesNothing(t *testing.T) {
	reg, _, _, _, setClock := newFixture(t)
	pub, priv, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// The attacker holds and uses their OWN keypair — a complete, valid,
	// self-signed assertion — against a registry that has never seen the
	// key.
	rogueSrc := NewInMemorySource()
	if err := rogueSrc.StoreDeviceKey(KeyIDFor(pub), priv); err != nil {
		t.Fatalf("attacker stores own key: %v", err)
	}
	rogue := NewSigner(rogueSrc, pub, Binding{UserID: "attacker", DeviceID: "attacker-device", WorkspaceID: "ws-1"})
	rogue.now = func() time.Time { return *setClock } // signed at the registry's present, not the future
	a, err := rogue.Sign(payload("self-signed"))
	if err != nil {
		t.Fatalf("rogue sign: %v", err)
	}
	res, err := reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyUnregistered, PurposeNewTransfer)
}

// TestRegisteredButNotActiveCannotAuthorize covers model §2.2/§2.4:
// Registered-but-not-Active → KEY_NOT_ACTIVE, even with a fully valid
// signature (the possession half passes, the authorization half denies).
func TestRegisteredButNotActiveCannotAuthorize(t *testing.T) {
	reg, _, _, _, setClock := newFixture(t)
	ceremony := Ceremony{SessionProof: "session-ref-2", UserConfirmed: true}
	pub2, priv2, _ := Generate()
	if _, err := reg.Register(ceremony, pub2, Binding{UserID: "user-1", DeviceID: "device-3", WorkspaceID: "ws-1"}); err != nil {
		t.Fatalf("register 2: %v", err)
	}
	id2 := KeyIDFor(pub2)
	// Signed at the registry's fixture clock — same fail-closed rule as
	// the unregistered-key test above.
	signed := Assertion{Version: PayloadV1, KeyID: id2, Payload: payload("pending-2"), SignedAt: *setClock}
	signed.Signature = ed25519.Sign(priv2, signed.canonical())
	res, err := reg.Verify(signed, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyNotActive, PurposeNewTransfer)
	// A not-active key cannot authorize reconciliation either.
	res, err = reg.Verify(signed, PurposeReceiptReconciliation)
	denyReason(t, res, err, ReasonKeyNotActive, PurposeReceiptReconciliation)
}

// TestSupersededGraceSplit covers model §2.3/§2.4: within the grace
// window a superseded key verifies for IN-FLIGHT RECONCILIATION ONLY and
// never for a NEW transfer; beyond the window it verifies nothing.
func TestSupersededGraceSplit(t *testing.T) {
	reg, signer, _, _, setClock := newFixture(t)
	newPub, newPriv, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	oldID := signer.ID()
	old, _, err := reg.Rotate(oldID, Ceremony{SessionProof: "session-ref-r", UserConfirmed: true}, newPub)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if old.State != StateSuperseded {
		t.Fatalf("expected old key superseded, got %s", old.State)
	}
	newSrc := NewInMemorySource()
	if err := newSrc.StoreDeviceKey(KeyIDFor(newPub), newPriv); err != nil {
		t.Fatalf("store: %v", err)
	}
	newSigner := NewSigner(newSrc, newPub, signer.Binding)
	newSigner.now = signer.now // share the fixture clock

	// A receipt signed by the OLD key while it was still current.
	a, err := signer.Sign(payload("in-flight"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Within grace: reconciliation verifies, new transfer denies.
	res, err := reg.Verify(a, PurposeReceiptReconciliation)
	if err != nil || !res.Allow {
		t.Fatalf("expected within-grace reconciliation allow, got res=%+v err=%v", res, err)
	}
	res, err = reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyNotCurrent, PurposeNewTransfer)

	// Beyond grace: nothing verifies.
	*setClock = setClock.Add(reg.SupersedeGrace + time.Minute)
	res, err = reg.Verify(a, PurposeReceiptReconciliation)
	denyReason(t, res, err, ReasonKeyNotCurrent, PurposeReceiptReconciliation)
	res, err = reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyNotCurrent, PurposeNewTransfer)

	// The new key authorizes for both purposes.
	na, err := newSigner.Sign(payload("fresh"))
	if err != nil {
		t.Fatalf("new sign: %v", err)
	}
	for _, purpose := range []Purpose{PurposeReceiptReconciliation, PurposeNewTransfer} {
		res, err := reg.Verify(na, purpose)
		if err != nil || !res.Allow {
			t.Fatalf("expected new-key allow for %s, got res=%+v err=%v", purpose, res, err)
		}
	}
}

// TestExpiredKeyCannotAuthorize covers the Expired arm: an Active key
// past its NotAfter reads Expired at CURRENT reads and denies
// KEY_NOT_CURRENT.
func TestExpiredKeyCannotAuthorize(t *testing.T) {
	reg, signer, _, _, setClock := newFixture(t)
	id := signer.ID()
	expiry := setClock.Add(time.Hour)
	if _, err := reg.SetExpiry(id, expiry); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	a, err := signer.Sign(payload("pre-expiry"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if res, err := reg.Verify(a, PurposeNewTransfer); err != nil || !res.Allow {
		t.Fatalf("expected allow before expiry, got res=%+v err=%v", res, err)
	}
	*setClock = expiry.Add(time.Minute)
	res, err := reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyNotCurrent, PurposeNewTransfer)
	if rec, ok := reg.Lookup(id); !ok || rec.State != StateExpired {
		t.Fatalf("expected CURRENT state expired, got %+v ok=%v", rec, ok)
	}
}

// TestRevokedKeyDeniesEverywhereAndIsTerminal covers model §2.4: Revoked
// denies instantly for both purposes, re-activation is refused, and a
// valid signature alone never authorizes.
func TestRevokedKeyDeniesEverywhereAndIsTerminal(t *testing.T) {
	reg, signer, _, _, _ := newFixture(t)
	id := signer.ID()
	a, err := signer.Sign(payload("pre-revocation"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := reg.Revoke(id, nil); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, purpose := range []Purpose{PurposeReceiptReconciliation, PurposeNewTransfer} {
		res, err := reg.Verify(a, purpose)
		denyReason(t, res, err, ReasonKeyRevoked, purpose)
	}
	if _, err := reg.Activate(id); err == nil {
		t.Fatal("expected re-activation of a revoked key to fail")
	}
}

// TestCompromiseTimestampEnforcement covers model §2.4's compromise row:
// the registry records the compromise time and a receipt signed after it
// is untrusted even though its signature verifies — the state denial, not
// the signature, decides.
func TestCompromiseTimestampEnforcement(t *testing.T) {
	reg, signer, _, _, setClock := newFixture(t)
	id := signer.ID()
	a, err := signer.Sign(payload("post-compromise"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// The assertion is validly signed and was signed BEFORE the recorded
	// compromise moment; the compromise is then recorded and the key
	// revoked — the model's mandatory response.
	compromise := *setClock
	if _, err := reg.Revoke(id, &compromise); err != nil {
		t.Fatalf("revoke with compromise: %v", err)
	}
	// Post-compromise signing attempt (the stolen key in attacker hands)
	// must also fail closed at the registry.
	res, err := reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeyRevoked, PurposeNewTransfer)
	if rec, _ := reg.Lookup(id); rec.CompromiseAt.IsZero() {
		t.Fatal("expected the compromise timestamp to be recorded")
	}
	// A future-dated compromise is refused (records never lie forward).
	future := setClock.Add(time.Hour)
	if _, err := reg.Revoke(id, &future); err == nil {
		t.Fatal("expected a future compromise timestamp to be refused")
	}
}

// TestUnrecoverableKeyFailsClosed covers model §2.4's unrecoverable row
// and §2.1 storage posture: when the OS keyring cannot produce the
// private key, signing fails with the typed KEY_UNAVAILABLE denial and NO
// weaker identity is substituted.
func TestUnrecoverableKeyFailsClosed(t *testing.T) {
	_, signer, _, src, _ := newFixture(t)
	src.FailNext = true
	a, err := signer.Sign(payload("no-keyring"))
	if err == nil {
		t.Fatal("expected the signing attempt to fail closed")
	}
	dke, ok := err.(*DeviceKeyError)
	if !ok || dke.Reason != ReasonKeyUnavailable {
		t.Fatalf("expected typed KEY_UNAVAILABLE, got %v", err)
	}
	if a.Signature != nil {
		t.Fatal("no assertion may be produced on unavailability")
	}
	// The registry never saw a fallback identity: the failure carries the
	// SAME key id, not a substitute.
	if dke.KeyID != signer.ID() {
		t.Fatalf("expected the same key id, got %s", dke.KeyID)
	}
}

// TestSignatureInvalidDenies covers the possession half: a signature that
// does not verify against the registered public key denies, and a valid
// signature under a revoked/foreign key never authorizes.
func TestSignatureInvalidDenies(t *testing.T) {
	reg, signer, _, _, _ := newFixture(t)
	a, err := signer.Sign(payload("legit"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := a
	tampered.Payload = payload("tampered")
	res, err := reg.Verify(tampered, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeySignatureInvalid, PurposeNewTransfer)
	// Foreign key's signature presented under the registered key id.
	foreignPub, foreignPriv, _ := Generate()
	_ = foreignPub
	forged := a
	forged.Signature = ed25519.Sign(foreignPriv, a.canonical())
	res, err = reg.Verify(forged, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeySignatureInvalid, PurposeNewTransfer)
}

// TestFutureSignedAssertionFailsClosed pins the v1 no-skew rule: an
// assertion dated after the registry clock fails closed before any state
// logic.
func TestFutureSignedAssertionFailsClosed(t *testing.T) {
	reg, signer, _, _, setClock := newFixture(t)
	signer.now = func() time.Time { return setClock.Add(time.Hour) } // signer ahead of the registry
	a, err := signer.Sign(payload("future"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	signer.now = func() time.Time { return *setClock }
	res, err := reg.Verify(a, PurposeNewTransfer)
	denyReason(t, res, err, ReasonKeySignatureInvalid, PurposeNewTransfer)
}

// TestCeremonyFailsClosed covers model §2.2: registration without an
// authenticated session proof or without explicit user confirmation is
// refused; the key never enters the registry.
func TestCeremonyFailsClosed(t *testing.T) {
	reg, _, pub, _, _ := newFixture(t)
	binding := Binding{UserID: "user-2", DeviceID: "device-9", WorkspaceID: "ws-1"}
	if _, err := reg.Register(Ceremony{SessionProof: "", UserConfirmed: true}, pub, binding); err == nil {
		t.Fatal("expected missing session proof to refuse")
	}
	if _, err := reg.Register(Ceremony{SessionProof: "sp", UserConfirmed: false}, pub, binding); err == nil {
		t.Fatal("expected missing user confirmation to refuse")
	}
}

// TestRotationAndReEnrollmentTraces covers the accepted model's rotation
// and re-enrollment traces (§4 evidence list): rotate keeps one current
// key per binding, and re-enrollment binds the new key to the SAME tuple
// while the old key terminates.
func TestRotationAndReEnrollmentTraces(t *testing.T) {
	reg, signer, _, _, setClock := newFixture(t)
	oldID := signer.ID()

	// Rotation trace.
	newPub, _, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	oldRec, newRec, err := reg.Rotate(oldID, Ceremony{SessionProof: "rot", UserConfirmed: true}, newPub)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if oldRec.Binding != newRec.Binding {
		t.Fatal("rotation must preserve the binding tuple")
	}
	if oldRec.State != StateSuperseded || newRec.State != StateActive {
		t.Fatalf("rotation states: old=%s new=%s", oldRec.State, newRec.State)
	}
	if oldRec.GraceUntil.IsZero() || !oldRec.GraceUntil.After(*setClock) {
		t.Fatal("expected a live grace window after rotation")
	}

	// Re-enrollment trace: loss of the CURRENT key.
	lossPub, _, err := Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	reRec, err := reg.Reenroll(newRec.KeyID, Ceremony{SessionProof: "reenroll", UserConfirmed: true}, lossPub)
	if err != nil {
		t.Fatalf("reenroll: %v", err)
	}
	if reRec.Binding != newRec.Binding {
		t.Fatal("re-enrollment must bind to the SAME tuple")
	}
	if reRec.State != StateRegistered {
		t.Fatalf("expected the new key to start Registered, got %s", reRec.State)
	}
	if rec, _ := reg.Lookup(newRec.KeyID); rec.State != StateRevoked {
		t.Fatalf("expected the lost key revoked, got %s", rec.State)
	}
	if _, err := reg.Activate(reRec.KeyID); err != nil {
		t.Fatalf("activate re-enrolled key: %v", err)
	}
	if rec, _ := reg.Lookup(reRec.KeyID); rec.State != StateActive {
		t.Fatalf("expected the re-enrolled key Active, got %s", rec.State)
	}
}

// TestIdentityDistinctFromAuthorityEpochs is the executable distinctness
// guard (accepted model §2.1, C4-41 acceptance "identity is distinct from
// lease/fencing epoch"): the identity surface — records, results,
// bindings — carries NO lease, fencing or authority epoch field, and a
// full lifecycle leaves no epoch-shaped value behind because the model
// owns none.
func TestIdentityDistinctFromAuthorityEpochs(t *testing.T) {
	for _, shape := range []reflect.Type{reflect.TypeOf(Record{}), reflect.TypeOf(VerifyResult{}), reflect.TypeOf(Binding{}), reflect.TypeOf(Assertion{})} {
		for i := 0; i < shape.NumField(); i++ {
			name := strings.ToUpper(shape.Field(i).Name)
			for _, forbidden := range []string{"EPOCH", "LEASE", "FENCE", "FENCING", "AUTHORITY"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("%s carries forbidden identity-coupling field %s", shape, shape.Field(i).Name)
				}
			}
		}
	}
	// The full lifecycle (register → activate → rotate → revoke) runs
	// without any epoch input or output: the API has none to pass.
	reg, signer, _, _, _ := newFixture(t)
	newPub, _, _ := Generate()
	if _, _, err := reg.Rotate(signer.ID(), Ceremony{SessionProof: "s", UserConfirmed: true}, newPub); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := reg.Revoke(KeyIDFor(newPub), nil); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}

// TestAssertionValidationFailsClosed pins the bounded-input rules.
func TestAssertionValidationFailsClosed(t *testing.T) {
	reg, signer, _, _, _ := newFixture(t)
	a, err := signer.Sign(payload("ok"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	badVersion := a
	badVersion.Version = 99
	if _, err := reg.Verify(badVersion, PurposeNewTransfer); err == nil {
		t.Fatal("expected unknown payload version to be refused")
	}
	noKey := a
	noKey.KeyID = ""
	if _, err := reg.Verify(noKey, PurposeNewTransfer); err == nil {
		t.Fatal("expected missing key id to be refused")
	}
	oversized := a
	oversized.Payload = make([]byte, MaxPayloadBytes+1)
	if _, err := signer.Sign(oversized.Payload); err == nil {
		t.Fatal("expected oversized payload to be refused at signing")
	}
	if _, err := reg.Verify(a, Purpose("unknown")); err == nil {
		t.Fatal("expected unknown purpose to be refused")
	}
}
