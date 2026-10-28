package keyring

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security/devicekey"
)

// The concrete source satisfies the devicekey storage port structurally —
// the adapter→port direction recorded in dependency-map v1.16.
var _ devicekey.PrivateKeySource = DeviceKeySource{}

// deviceKeyFixture returns a source over the swapped backend plus a
// generated keypair and its registry id.
func deviceKeyFixture(t *testing.T, b keyringBackend) (*DeviceKeySource, ed25519.PublicKey, ed25519.PrivateKey, devicekey.KeyID) {
	t.Helper()
	swapBackend(t, b)
	src := NewDeviceKeySource()
	pub, priv, err := devicekey.Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return src, pub, priv, devicekey.KeyIDFor(pub)
}

// wantUnavailable asserts the typed fail-closed family.
func wantUnavailable(t *testing.T, err error) *devicekey.DeviceKeyError {
	t.Helper()
	if err == nil {
		t.Fatal("expected a fail-closed error, got nil")
	}
	dke, ok := err.(*devicekey.DeviceKeyError)
	if !ok {
		t.Fatalf("expected *devicekey.DeviceKeyError, got %T: %v", err, err)
	}
	if dke.Reason != devicekey.ReasonKeyUnavailable {
		t.Fatalf("reason = %s, want %s", dke.Reason, devicekey.ReasonKeyUnavailable)
	}
	return dke
}

// TestDeviceKeyRoundTrip covers the store → load → delete lifecycle: the
// exact private-key bytes return at the call boundary only, and after
// deletion the item is gone (load fails closed, delete stays idempotent).
func TestDeviceKeyRoundTrip(t *testing.T) {
	src, _, priv, id := deviceKeyFixture(t, &memBackend{})

	if err := src.StoreDeviceKey(id, priv); err != nil {
		t.Fatalf("store: %v", err)
	}
	got, err := src.LoadDeviceKey(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(got) != string(priv) {
		t.Fatal("loaded key bytes differ from the stored key")
	}
	if err := src.DeleteDeviceKey(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := src.LoadDeviceKey(id); err == nil {
		t.Fatal("expected load after delete to fail closed")
	}
	if err := src.DeleteDeviceKey(id); err != nil {
		t.Fatalf("delete of an absent item must stay idempotent: %v", err)
	}
}

// TestDeviceKeyFailClosedWithoutKeyring covers the accepted posture: no
// reachable OS keyring means every operation fails closed with the typed
// KEY_UNAVAILABLE family — the accepted correct state (headless Linux),
// never a degraded mode.
func TestDeviceKeyFailClosedWithoutKeyring(t *testing.T) {
	src, _, priv, id := deviceKeyFixture(t, failBackend{err: errors.New("dbus: connection refused")})

	if err := src.StoreDeviceKey(id, priv); err == nil {
		t.Fatal("expected store to fail closed")
	} else if dke := wantUnavailable(t, err); dke.Code != errs.CodeStorageUnavailable {
		t.Fatalf("code = %v, want storage-unavailable", dke.Code)
	}
	if _, err := src.LoadDeviceKey(id); err == nil {
		t.Fatal("expected load to fail closed")
	} else {
		wantUnavailable(t, err)
	}
	if err := src.DeleteDeviceKey(id); err == nil {
		t.Fatal("expected delete to fail closed")
	} else {
		wantUnavailable(t, err)
	}
}

// TestDeviceKeyValidationRefusedBeforeAnyStoreCall pins the input bounds:
// unbounded/empty ids and wrong-length key material are refused before
// the keyring is touched.
func TestDeviceKeyValidationRefusedBeforeAnyStoreCall(t *testing.T) {
	swapBackend(t, &memBackend{})
	src := NewDeviceKeySource()
	longID := devicekey.KeyID(strings.Repeat("a", 129)) // beyond the bounded id length
	if err := longID.Validate(); err == nil {
		t.Fatal("expected the overlong id to be invalid")
	}
	if err := src.StoreDeviceKey(devicekey.KeyID(""), ed25519.PrivateKey(make([]byte, 32))); err == nil {
		t.Fatal("expected the empty id to be refused")
	}
	if err := src.StoreDeviceKey(devicekey.KeyID(strings.Repeat("a", 64)), ed25519.PrivateKey(make([]byte, 32))); err == nil {
		t.Fatal("expected wrong-length key material to be refused")
	}
}

// TestDeviceKeyCorruptItemFailsClosed pins the wrong-length stored value
// rule: a corrupted item is a typed fail-closed unavailability, never a
// partial key.
func TestDeviceKeyCorruptItemFailsClosed(t *testing.T) {
	src, _, priv, id := deviceKeyFixture(t, &memBackend{})
	if err := src.StoreDeviceKey(id, priv); err != nil {
		t.Fatalf("store: %v", err)
	}
	// Corrupt the stored item to a wrong length through the seam.
	if err := backend.Set(Service, deviceKeyUserPrefix+string(id), "short"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	_, err := src.LoadDeviceKey(id)
	if err == nil {
		t.Fatal("expected the corrupted item to fail closed")
	}
	dke, ok := err.(*devicekey.DeviceKeyError)
	if !ok || dke.Code != errs.CodeStateCorrupt {
		t.Fatalf("expected state-corrupt typed failure, got %v", err)
	}
}

// TestDeviceKeyErrorNeverCarriesKeyMaterial keeps the value-free error
// discipline: no byte of the private key ever appears in an error string.
func TestDeviceKeyErrorNeverCarriesKeyMaterial(t *testing.T) {
	src, _, priv, id := deviceKeyFixture(t, failBackend{err: errors.New("refused")})
	err := src.StoreDeviceKey(id, priv)
	if err == nil {
		t.Fatal("expected failure")
	}
	if strings.Contains(err.Error(), string(priv[:8])) {
		t.Fatal("error text leaked key material")
	}
}
