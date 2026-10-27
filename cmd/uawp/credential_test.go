package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The credential surface's tests never touch a real provider handle: the
// round-trip probe uses a dedicated --name under the fixture family, and a
// canary value asserts the secret NEVER reaches any output surface.

const credentialCanary = "uawp-canary-7b21-do-not-echo"

func withCredentialStdin(t *testing.T, value string) {
	t.Helper()
	old := credentialStdin
	credentialStdin = strings.NewReader(value)
	t.Cleanup(func() { credentialStdin = old })
}

// TestCredentialUnknownProviderRefused proves the typo guard: a provider id
// outside the recorded set is refused, and the value is never echoed.
func TestCredentialUnknownProviderRefused(t *testing.T) {
	h := newHarness(t)
	withCredentialStdin(t, credentialCanary)
	code, out := h.run("credential", "store", "--provider", "no-such-provider", "--json")
	if code == 0 {
		t.Fatal("an unknown provider must be refused")
	}
	if strings.Contains(out, credentialCanary) {
		t.Fatal("the credential value must never be echoed")
	}
}

// TestCredentialEmptyValueRefused proves an empty stdin is a typed
// validation refusal, never a stored empty secret.
func TestCredentialEmptyValueRefused(t *testing.T) {
	h := newHarness(t)
	withCredentialStdin(t, "   \n")
	code, out := h.run("credential", "store", "--provider", "glm-zai", "--json")
	if code == 0 {
		t.Fatal("an empty credential must be refused")
	}
	if !strings.Contains(out, "VALIDATION_FAILED") {
		t.Fatalf("refusal must be typed, got %s", out)
	}
}

// TestCredentialStatusIsTypedAndValueless proves status is a safe probe: on
// a fresh keyring it reports present:false with the SECRET_MISSING reason;
// where no keyring exists it fails typed — and neither path carries a value.
func TestCredentialStatusIsTypedAndValueless(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("credential", "status", "--provider", "glm-zai", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("status must print machine JSON: %q", out)
	}
	if code == 0 {
		if report["present"] != false {
			t.Fatalf("a fresh keyring cannot hold this credential: %v", report)
		}
		if report["reason"] != "SECRET_MISSING" {
			t.Fatalf("absence must be typed SECRET_MISSING, got %v", report["reason"])
		}
		return
	}
	// No usable keyring on this machine: the failure must still be typed.
	if !strings.Contains(out, "SECRET_") && !strings.Contains(out, "error") {
		t.Fatalf("storage failure must be typed, got %s", out)
	}
}

// TestCredentialRoundTripIsSecretless is the live-store proof (skipped
// where no OS keyring exists): store → present → remove → absent, and the
// value NEVER appears in any output surface.
func TestCredentialRoundTripIsSecretless(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("credential", "status", "--provider", "mock", "--name", "roundtrip-probe", "--json")
	if code != 0 {
		t.Skipf("no usable OS keyring here (exit %d): %s", code, out)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(out), &probe); err != nil {
		t.Fatalf("status must print machine JSON: %q", out)
	}
	if probe["present"] != false {
		t.Fatalf("the probe handle must start absent: %v", probe)
	}
	withCredentialStdin(t, credentialCanary)
	code, out = h.run("credential", "store", "--provider", "mock", "--name", "roundtrip-probe", "--json")
	if code != 0 {
		t.Fatalf("store failed: %d %s", code, out)
	}
	if strings.Contains(out, credentialCanary) {
		t.Fatal("the credential value must never be echoed")
	}
	code, out = h.run("credential", "status", "--provider", "mock", "--name", "roundtrip-probe", "--json")
	if code != 0 || !strings.Contains(out, `"present": true`) {
		t.Fatalf("credential must be present after store: %d %s", code, out)
	}
	code, out = h.run("credential", "remove", "--provider", "mock", "--name", "roundtrip-probe", "--json")
	if code != 0 {
		t.Fatalf("remove failed: %d %s", code, out)
	}
	if strings.Contains(out, credentialCanary) {
		t.Fatal("the credential value must never be echoed")
	}
	code, out = h.run("credential", "status", "--provider", "mock", "--name", "roundtrip-probe", "--json")
	if code != 0 || !strings.Contains(out, `"present": false`) {
		t.Fatalf("credential must be absent after remove: %d %s", code, out)
	}
}

// TestJobSubmitBaseUrlOverrideAccepted proves the endpoint-override flag:
// the fixture path accepts it, and a real provider with an override STILL
// fails at the credential boundary — the override bypasses no gate.
func TestJobSubmitBaseUrlOverrideAccepted(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-bu",
		"--base-url", "http://127.0.0.1:1", "--json")
	if code != 0 {
		t.Fatalf("the fixture path must accept an endpoint override, got %d %s", code, out)
	}
	code, out = h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-bu2",
		"--provider", "glm-zai", "--classification", "non-personal",
		"--base-url", "http://127.0.0.1:1", "--json")
	if code == 0 {
		t.Fatal("an override must not bypass the credential boundary")
	}
	if !strings.Contains(out, "credential unavailable") {
		t.Fatalf("the failure must come from the credential boundary, got %s", out)
	}
}
