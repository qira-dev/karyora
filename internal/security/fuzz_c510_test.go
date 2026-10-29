package security

// C5-10 adversarial checks for the TB-2/TB-3 residuals of the C5-06
// threat-model record (CR-5.1 sub-milestone 3): bounded fuzzing of the
// session-token authentication path and the local HTTP boundary
// predicates. The fuzz targets run their seed corpus in every normal
// `go test` pass (including CI under -race); longer -fuzztime campaigns
// are the C5-10 measurement runs and are recorded per run.
//
// The invariants are the fail-closed properties the boundary contract
// already demands: an adversarial token never validates, and the bind /
// Host-header predicates never admit a non-loopback host without the
// explicit opt-in.

import (
	"strings"
	"testing"
)

// FuzzSessionTokenValidation is the TB-2 adversarial check: arbitrary
// token material against the session validation path must never panic and
// must never validate for the data scope (fail closed), except the one
// legitimately issued token.
func FuzzSessionTokenValidation(f *testing.F) {
	svc := NewSessionService("inst-c510-fuzz", f.TempDir())
	cred, err := svc.IssueBootstrapCredential("cli")
	if err != nil {
		f.Fatalf("issue bootstrap credential: %v", err)
	}
	tok, err := svc.Exchange(cred, "cli", []Scope{ScopeData})
	if err != nil {
		f.Fatalf("exchange: %v", err)
	}
	valid := tok.Secret
	// Seed corpus: empty, junk, the valid token, near-misses, malformed
	// shapes (the exchange/replay paths the threat model names).
	for _, seed := range []string{
		"", "x", valid, valid + "x", valid[:len(valid)/2], strings.ToUpper(valid),
		"kry1-0000000000000000", "\x00\x01\x02", strings.Repeat("A", 4096),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, token string) {
		got := svc.Validate(token, ScopeData)
		if got && token != valid {
			t.Errorf("an adversarial token validated for the data scope (len %d)", len(token))
		}
		// Re-validating the same material never flips the answer (no
		// state-dependent flapping on read paths).
		if svc.Validate(token, ScopeData) != got {
			t.Errorf("validation flapped between calls for the same token")
		}
	})
}

// FuzzHTTPBoundaryHosts is the TB-3 adversarial check: arbitrary Host /
// Origin / bind material against the local HTTP boundary predicates must
// never panic, and the loopback-only invariants must hold for every input
// (a bind without opt-in passes iff the host is loopback; an empty
// allowlist admits exactly the loopback forms).
func FuzzHTTPBoundaryHosts(f *testing.F) {
	for _, seed := range []string{
		"", "localhost", "127.0.0.1:8080", "[::1]:9000", "example.com",
		"localhost.evil.com", "127.0.0.1.evil.com", "0x7f000001", "2130706433",
		"[::ffff:127.0.0.1]", "LOCALHOST", " localhost ", "a b c", "127.0.0.1:",
		"1.2.3.4:1", "::1", "%00localhost",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, host string) {
		loopback := IsLoopbackHost(host)
		// Bind policy invariant (loopback-only without opt-in).
		err := ValidateBindAddress(host, false)
		if (err == nil) != loopback {
			t.Errorf("bind %q: accepted=%v but loopback=%v", host, err == nil, loopback)
		}
		// Empty-allowlist Host header invariant: exactly the loopback forms.
		if got := HostHeaderAllowed(host, nil); got != loopback {
			t.Errorf("host header %q: allowed=%v but loopback=%v", host, got, loopback)
		}
		// Origin predicate: never panics; empty allowlist admits only
		// loopback http(s) origins.
		_ = OriginAllowed(host, nil)
		// An explicit allowlist never admits anything but its exact
		// normalized entries.
		if got := HostHeaderAllowed(host, []string{"exact.host"}); got && normalizeHost(host) != "exact.host" {
			t.Errorf("host header %q admitted outside the exact allowlist entry", host)
		}
	})
}
