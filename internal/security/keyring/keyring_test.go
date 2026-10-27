package keyring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	zkeyring "github.com/zalando/go-keyring"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

func testHandle() security.Handle {
	return security.Handle{Scope: "provider:test", Name: "key"}
}

func asSecretError(t *testing.T, err error) *security.SecretError {
	t.Helper()
	se, ok := err.(*security.SecretError)
	if !ok {
		t.Fatalf("expected *security.SecretError, got %T: %v", err, err)
	}
	return se
}

func wantCode(t *testing.T, err error, code errs.Code) *security.SecretError {
	t.Helper()
	se := asSecretError(t, err)
	if se.Code != code {
		t.Fatalf("code = %v, want %v (reason %s)", se.Code, code, se.Reason)
	}
	return se
}

// memBackend is the in-memory double of the keyring seam, mirroring the
// real semantics: Get of an absent item yields zkeyring.ErrNotFound.
type memBackend struct{ m map[string]map[string]string }

func (b *memBackend) Set(service, user, password string) error {
	if b.m == nil {
		b.m = map[string]map[string]string{}
	}
	if b.m[service] == nil {
		b.m[service] = map[string]string{}
	}
	b.m[service][user] = password
	return nil
}

func (b *memBackend) Get(service, user string) (string, error) {
	if v, ok := b.m[service][user]; ok {
		return v, nil
	}
	return "", zkeyring.ErrNotFound
}

func (b *memBackend) Delete(service, user string) error {
	if _, ok := b.m[service][user]; ok {
		delete(b.m[service], user)
		return nil
	}
	return zkeyring.ErrNotFound
}

// failBackend fails every operation with its error, mirroring an
// unreachable or refusing OS keyring.
type failBackend struct{ err error }

func (b failBackend) Set(string, string, string) error   { return b.err }
func (b failBackend) Get(string, string) (string, error) { return "", b.err }
func (b failBackend) Delete(string, string) error        { return b.err }

// swapBackend replaces the keyring seam for the test and restores the real
// (OS) backend when the test finishes.
func swapBackend(t *testing.T, b keyringBackend) {
	t.Helper()
	orig := backend
	backend = b
	t.Cleanup(func() { backend = orig })
}

// keyringReachable reports whether a usable OS keyring answers on this
// machine: a probe lookup answered with ErrNotFound means the store itself
// responded (the probe item never exists). Any other error means there is
// no reachable keyring — the accepted fail-closed environment.
func keyringReachable() bool {
	_, err := backend.Get(Service, "probe/session-check")
	return errors.Is(err, zkeyring.ErrNotFound)
}

func TestHandleValidationRefusedBeforeAnyStoreCall(t *testing.T) {
	s := New()
	ctx := context.Background()
	invalid := []security.Handle{
		{},
		{Scope: "provider", Name: ""},
		{Scope: strings.Repeat("x", 200), Name: "k"},
		{Scope: "pro vider", Name: "k"},
		{Scope: "provider", Name: "ke\ny"},
	}
	for _, h := range invalid {
		if _, err := s.Resolve(ctx, h); err == nil {
			t.Fatalf("Resolve accepted invalid handle %q", h.String())
		} else {
			se := wantCode(t, err, errs.CodeValidationFailed)
			if se.Reason != security.ReasonSecretMissing {
				t.Fatalf("reason = %s, want %s", se.Reason, security.ReasonSecretMissing)
			}
		}
		if _, err := s.Status(ctx, h); err == nil {
			t.Fatalf("Status accepted invalid handle %q", h.String())
		} else {
			wantCode(t, err, errs.CodeValidationFailed)
		}
		if err := s.Store(ctx, h, []byte("v")); err == nil {
			t.Fatalf("Store accepted invalid handle %q", h.String())
		} else {
			wantCode(t, err, errs.CodeValidationFailed)
		}
		if err := s.Revoke(ctx, h); err == nil {
			t.Fatalf("Revoke accepted invalid handle %q", h.String())
		} else {
			wantCode(t, err, errs.CodeValidationFailed)
		}
	}
}

func TestEmptyValueNeverStored(t *testing.T) {
	swapBackend(t, &memBackend{})
	s := New()
	ctx := context.Background()
	for _, v := range [][]byte{nil, {}} {
		err := s.Store(ctx, testHandle(), v)
		if err == nil {
			t.Fatal("Store accepted an empty value")
		}
		se := wantCode(t, err, errs.CodeValidationFailed)
		if se.Reason != security.ReasonSecretMissing {
			t.Fatalf("reason = %s, want %s", se.Reason, security.ReasonSecretMissing)
		}
	}
}

func TestLifecycleRotationAndRevocation(t *testing.T) {
	swapBackend(t, &memBackend{})
	s := New()
	ctx := context.Background()
	h := testHandle()

	if st, err := s.Status(ctx, h); err != nil || st != security.SecretMissing {
		t.Fatalf("fresh handle status = (%v, %v), want (missing, nil)", st, err)
	}
	if _, err := s.Resolve(ctx, h); err == nil {
		t.Fatal("Resolve of a missing handle succeeded")
	} else {
		se := wantCode(t, err, errs.CodeNotFound)
		if se.Reason != security.ReasonSecretMissing {
			t.Fatalf("reason = %s, want %s", se.Reason, security.ReasonSecretMissing)
		}
	}
	if err := s.Store(ctx, h, []byte("first")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if st, err := s.Status(ctx, h); err != nil || st != security.SecretPresent {
		t.Fatalf("status after Store = (%v, %v), want (present, nil)", st, err)
	}
	if v, err := s.Resolve(ctx, h); err != nil || string(v) != "first" {
		t.Fatalf("Resolve = (%q, %v), want (first, nil)", v, err)
	}
	// The sanctioned resolution path returns the value at the call boundary.
	if v, err := security.ResolveForUse(ctx, s, h); err != nil || string(v) != "first" {
		t.Fatalf("ResolveForUse = (%q, %v), want (first, nil)", v, err)
	}
	// Rotation: the same-handle overwrite resolves at the next call.
	if err := s.Store(ctx, h, []byte("second")); err != nil {
		t.Fatalf("rotating Store: %v", err)
	}
	if v, err := s.Resolve(ctx, h); err != nil || string(v) != "second" {
		t.Fatalf("Resolve after rotation = (%q, %v), want (second, nil)", v, err)
	}
	// Revocation: the handle then fails closed as missing; revoking twice is
	// idempotent — the revoked state is the goal.
	if err := s.Revoke(ctx, h); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := s.Revoke(ctx, h); err != nil {
		t.Fatalf("idempotent Revoke: %v", err)
	}
	if st, err := s.Status(ctx, h); err != nil || st != security.SecretMissing {
		t.Fatalf("status after Revoke = (%v, %v), want (missing, nil)", st, err)
	}
	if _, err := s.Resolve(ctx, h); err == nil {
		t.Fatal("Resolve of a revoked handle succeeded")
	} else {
		wantCode(t, err, errs.CodeNotFound)
	}
	if _, err := security.ResolveForUse(ctx, s, h); err == nil {
		t.Fatal("ResolveForUse of a revoked handle succeeded")
	}
}

func TestFailuresMapToTypedFailClosedFamily(t *testing.T) {
	ctx := context.Background()
	s := New()
	cases := []struct {
		name     string
		storeErr error
		code     errs.Code
	}{
		{"unreachable bus", errors.New("dbus: connection closed by user"), errs.CodeStorageUnavailable},
		{"unsupported platform", zkeyring.ErrUnsupportedPlatform, errs.CodeStorageUnavailable},
		{"value too big", zkeyring.ErrSetDataTooBig, errs.CodeValidationFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapBackend(t, failBackend{err: tc.storeErr})

			if _, err := s.Resolve(ctx, testHandle()); err == nil {
				t.Fatal("Resolve succeeded while the keyring fails")
			} else {
				wantCode(t, err, tc.code)
			}
			if st, err := s.Status(ctx, testHandle()); st != security.SecretInaccessib || err == nil {
				t.Fatalf("Status = (%v, %v), want (inaccessible, error)", st, err)
			} else {
				wantCode(t, err, tc.code)
			}
			if err := s.Store(ctx, testHandle(), []byte("v")); err == nil {
				t.Fatal("Store succeeded while the keyring fails")
			} else {
				wantCode(t, err, tc.code)
			}
			if err := s.Revoke(ctx, testHandle()); err == nil {
				t.Fatal("Revoke succeeded while the keyring fails")
			} else {
				wantCode(t, err, tc.code)
			}
		})
	}
}

// TestCanaryValueNeverInErrors drives failing stores with a canary value
// and asserts the value never reaches any error surface.
func TestCanaryValueNeverInErrors(t *testing.T) {
	ctx := context.Background()
	const canary = "karyora-canary-9f3a-do-not-leak"
	s := New()
	swapBackend(t, failBackend{err: errors.New("dbus: connection closed by user")})

	if err := s.Store(ctx, testHandle(), []byte(canary)); err == nil {
		t.Fatal("expected the failing store to refuse the canary")
	} else if strings.Contains(err.Error(), canary) {
		t.Fatal("error surface contains the secret value")
	}
	if _, err := s.Resolve(ctx, testHandle()); err == nil {
		t.Fatal("expected the failing store to refuse Resolve")
	} else if strings.Contains(err.Error(), canary) {
		t.Fatal("error surface contains the secret value")
	}
}

// TestFailClosedWithoutKeyring runs only where no OS keyring answers (for
// example CI or headless Linux) and proves the accepted fail-closed
// behavior on the real backend.
func TestFailClosedWithoutKeyring(t *testing.T) {
	if keyringReachable() {
		t.Skip("OS keyring reachable; the live round-trip test covers the real path")
	}
	s := New()
	ctx := context.Background()
	if _, err := s.Resolve(ctx, testHandle()); err == nil {
		t.Fatal("Resolve succeeded without a keyring")
	} else {
		wantCode(t, err, errs.CodeStorageUnavailable)
	}
	if st, err := s.Status(ctx, testHandle()); st != security.SecretInaccessib || err == nil {
		t.Fatalf("Status = (%v, %v), want (inaccessible, error)", st, err)
	}
	if err := s.Store(ctx, testHandle(), []byte("v")); err == nil {
		t.Fatal("Store succeeded without a keyring")
	} else {
		wantCode(t, err, errs.CodeStorageUnavailable)
	}
}

// TestLiveRoundTripOnRealKeyring proves the real OS-keyring path where a
// keyring answers: store → status → resolve → rotate → revoke, then cleans
// up its own item. Where no keyring exists it skips with the recorded
// reason (the fail-closed test covers that environment).
func TestLiveRoundTripOnRealKeyring(t *testing.T) {
	if !keyringReachable() {
		t.Skip("no reachable OS keyring on this machine; recorded as the accepted fail-closed environment")
	}
	s := New()
	ctx := context.Background()
	h := security.Handle{
		Scope: "provider:roundtrip-test",
		Name:  fmt.Sprintf("t%d", time.Now().UnixNano()),
	}
	t.Cleanup(func() { _ = s.Revoke(ctx, h) })

	if err := s.Store(ctx, h, []byte("live-one")); err != nil {
		t.Fatalf("live Store: %v", err)
	}
	if st, err := s.Status(ctx, h); err != nil || st != security.SecretPresent {
		t.Fatalf("live status = (%v, %v), want (present, nil)", st, err)
	}
	if v, err := s.Resolve(ctx, h); err != nil || string(v) != "live-one" {
		t.Fatalf("live Resolve = (%q, %v), want (live-one, nil)", v, err)
	}
	if err := s.Store(ctx, h, []byte("live-two")); err != nil {
		t.Fatalf("live rotating Store: %v", err)
	}
	if v, err := s.Resolve(ctx, h); err != nil || string(v) != "live-two" {
		t.Fatalf("live Resolve after rotation = (%q, %v), want (live-two, nil)", v, err)
	}
	if err := s.Revoke(ctx, h); err != nil {
		t.Fatalf("live Revoke: %v", err)
	}
	if st, err := s.Status(ctx, h); err != nil || st != security.SecretMissing {
		t.Fatalf("live status after Revoke = (%v, %v), want (missing, nil)", st, err)
	}
	if _, err := s.Resolve(ctx, h); err == nil {
		t.Fatal("live Resolve of a revoked handle succeeded")
	} else {
		wantCode(t, err, errs.CodeNotFound)
	}
}
