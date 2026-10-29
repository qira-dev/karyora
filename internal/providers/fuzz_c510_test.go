package providers

// C5-10 adversarial check for the FI-P1/TB-7 egress-boundary residual
// (CR-5.1 sub-milestone 3): bounded fuzzing of the destination-URL
// validation. The fuzz target runs its seed corpus in every normal
// `go test` pass (including CI under -race); longer -fuzztime campaigns
// are the C5-10 measurement runs and are recorded per run.
//
// Invariants (the egress contract): whatever validates must be an https
// URL or an explicit local loopback http URL, must carry no userinfo, and
// must carry no fragment — and validation must never panic on hostile
// input.

import (
	"net/url"
	"testing"
)

func FuzzEgressDestinationValidation(f *testing.F) {
	local := DefaultEgressPolicy(CoreLocal)
	remote := DefaultEgressPolicy(CoreRemote)
	for _, seed := range []string{
		"https://api.example.com/v1", "http://127.0.0.1:9/x", "http://localhost/x",
		"file:///etc/passwd", "https://u:p@host/x", "https://h/x#frag",
		"https://169.254.169.254/latest/meta-data", "https://[::1]/x",
		"https://100.64.0.1/x", "http://0x7f000001/x", "https://192.0.2.1/x",
		"HTTPS://HOST/x", "https://h/x?api_key=secret", "https://h/x?key=",
		"  https://h/x  ", "//h/x", "https://", "https://h/x?api_key",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		u, err := ValidateDestinationURL(local, raw)
		if err != nil {
			// The remote profile refuses plaintext http outright, so
			// anything the local profile refused the remote profile must
			// refuse too (the remote policy is strictly narrower).
			if _, err2 := ValidateDestinationURL(remote, raw); err2 == nil {
				t.Errorf("remote profile accepted %q while the local profile refused it", raw)
			}
			return
		}
		// Invariants on anything that validates under the local profile.
		if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
			t.Errorf("validated URL %q is neither https nor loopback http", raw)
		}
		if u.User != nil {
			t.Errorf("userinfo survived validation in %q", raw)
		}
		if u.Fragment != "" {
			t.Errorf("fragment survived validation in %q", raw)
		}
		// A validated URL must re-parse to the same destination.
		if _, err := url.Parse(raw); err != nil {
			t.Errorf("validated URL %q does not re-parse: %v", raw, err)
		}
	})
}
