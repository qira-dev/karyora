package plugins

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// CR-4.3 acceptance fixtures (C4-19–C4-28, contract model): manifest
// validation, trust lifecycle, per-call authorization, and untrusted
// result receipts. Plugin EXECUTION is not exercised — it stays disabled
// until D-14 evidence.

var now = time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)

const testDigest = "sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validManifest() PluginManifest {
	return PluginManifest{
		Name: "exporter", Version: "1.0.0", ABIVersion: ABIVersion,
		Capabilities: []CapabilityScope{{Capability: "fs.read", Scope: "/workspace/exports/*"}},
		Quota:        Quota{MaxMemoryBytes: 64 << 20, MaxCPUMillis: 1000, Timeout: time.Minute, MaxHostCalls: 100},
		Digest:       testDigest,
	}
}

func enabledRecord(t *testing.T) *PluginRecord {
	t.Helper()
	rec, err := NewPluginRecord(validManifest(), now)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := MarkVerified(rec, testDigest, now); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := TransitionPlugin(rec, LifecycleEnabled, now); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return rec
}

func TestManifestValidationNegatives(t *testing.T) {
	cases := map[string]func(m *PluginManifest){
		"unknown abi":         func(m *PluginManifest) { m.ABIVersion = "plugin-abi/v0" },
		"missing name":        func(m *PluginManifest) { m.Name = "" },
		"no capabilities":     func(m *PluginManifest) { m.Capabilities = nil },
		"capability no scope": func(m *PluginManifest) { m.Capabilities[0].Scope = "" },
		"zero quota axis":     func(m *PluginManifest) { m.Quota.MaxHostCalls = 0 },
		"bad digest form":     func(m *PluginManifest) { m.Digest = "abc123" },
	}
	for name, mutate := range cases {
		m := validManifest()
		mutate(&m)
		if err := ValidateManifest(&m); err == nil {
			t.Errorf("%s: manifest must be refused", name)
		}
	}
	if err := ValidateManifest(&PluginManifest{}); err == nil {
		t.Fatal("empty manifest must be refused")
	}
}

func TestTrustLifecycle(t *testing.T) {
	rec, err := NewPluginRecord(validManifest(), now)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// Enable before verification is refused.
	if err := TransitionPlugin(rec, LifecycleEnabled, now); !errors.Is(err, ErrLifecycle) {
		t.Fatalf("enable before verify must be refused, got %v", err)
	}
	// Verification with the WRONG digest is refused.
	if err := MarkVerified(rec, strings.Replace(testDigest, "a", "b", 1), now); err == nil {
		t.Fatal("verifying a mismatched digest must be refused")
	}
	if err := MarkVerified(rec, testDigest, now); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := TransitionPlugin(rec, LifecycleEnabled, now); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Disable -> re-enable is legal.
	if err := TransitionPlugin(rec, LifecycleDisabled, now); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := TransitionPlugin(rec, LifecycleEnabled, now); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	// Revocation is terminal.
	if err := Revoke(rec, now); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	for _, to := range []string{LifecycleEnabled, LifecycleDisabled, LifecycleVerified} {
		if err := TransitionPlugin(rec, to, now); err == nil {
			t.Fatalf("revoked plugin must never transition to %s", to)
		}
	}
}

func TestEveryHostCallAuthorized(t *testing.T) {
	table := NewGrantTable()
	rec := enabledRecord(t)
	// Deny-by-default: no grants at all.
	if err := AuthorizeHostCall(table, rec, "fs.read", "/workspace/exports/a.txt"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatalf("ungranted call must be denied, got %v", err)
	}
	table.Grant(HostGrant{Plugin: "exporter", Capability: "fs.read", Scope: "/workspace/exports/*"})
	// Wildcard-scope grant matches a request inside the tree.
	if err := AuthorizeHostCall(table, rec, "fs.read", "/workspace/exports/a.txt"); err != nil {
		t.Fatalf("granted call must pass, got %v", err)
	}
	// Outside the granted scope: denied.
	if err := AuthorizeHostCall(table, rec, "fs.read", "/etc/passwd"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatal("out-of-scope call must be denied")
	}
	// Unknown capability: denied even with other grants present.
	table.Grant(HostGrant{Plugin: "exporter", Capability: "net.call", Scope: "*"})
	if err := AuthorizeHostCall(table, rec, "fs.write", "/workspace/exports/a.txt"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatal("undeclared capability must be denied")
	}
	// Disabled plugin: everything denied even with grants.
	if err := TransitionPlugin(rec, LifecycleDisabled, now); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := AuthorizeHostCall(table, rec, "fs.read", "/workspace/exports/a.txt"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatal("disabled plugin calls must be denied")
	}
	// Artifact digest mismatch (artifact swapped after verification):
	// denied even while enabled.
	if err := TransitionPlugin(rec, LifecycleEnabled, now); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	rec.Manifest.Digest = "sha256-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := AuthorizeHostCall(table, rec, "fs.read", "/workspace/exports/a.txt"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatal("digest-mismatched artifact calls must be denied")
	}
	// Revocation denies everything, permanently.
	rec.Manifest.Digest = testDigest
	if err := Revoke(rec, now); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := AuthorizeHostCall(table, rec, "fs.read", "/workspace/exports/a.txt"); !errors.Is(err, ErrHostCallDenied) {
		t.Fatal("revoked plugin calls must be denied")
	}
}

func TestUntrustedResultReceipts(t *testing.T) {
	good := PluginResult{Plugin: "exporter", Version: "1.0.0", ABIVersion: ABIVersion,
		OutputDigest: testDigest, OutputBytes: 1024}
	if err := ValidateResult(&good, MaxPluginResultBytes); err != nil {
		t.Fatalf("valid result refused: %v", err)
	}
	negatives := map[string]func(r *PluginResult){
		"unknown abi":   func(r *PluginResult) { r.ABIVersion = "plugin-abi/v0" },
		"bad digest":    func(r *PluginResult) { r.OutputDigest = "digest" },
		"zero size":     func(r *PluginResult) { r.OutputBytes = 0 },
		"oversize":      func(r *PluginResult) { r.OutputBytes = MaxPluginResultBytes + 1 },
		"missing input": func(r *PluginResult) { r.Plugin = "" },
	}
	for name, mutate := range negatives {
		r := good
		mutate(&r)
		if err := ValidateResult(&r, MaxPluginResultBytes); err == nil {
			t.Errorf("%s: result must be rejected", name)
		}
	}
	rec := enabledRecord(t)
	receipt := BuildReceipt(rec, "fs.read", "/workspace/exports/*", testDigest, "succeeded", 3, now)
	if receipt.Plugin != "exporter" || receipt.Artifact != testDigest || receipt.HostCalls != 3 {
		t.Fatalf("receipt mismatch: %+v", receipt)
	}
	if receipt.At.IsZero() {
		t.Fatal("receipt must carry its timestamp")
	}
}
