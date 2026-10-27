package security

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeNeutralizesLogInjection(t *testing.T) {
	// A forged entry attempt through untrusted data must stay escaped:
	// structure is injected by the logger, never by data (C2-15 §3).
	in := "info: FAKE ENTRY\nfrom attacker\r\x00\x07x"
	out := Sanitize(in, 4096)
	if strings.Contains(out, "\n") || strings.Contains(out, "\r") {
		t.Fatalf("raw control characters must be escaped: %q", out)
	}
	if !strings.Contains(out, `\n`) || !strings.Contains(out, `\r`) || !strings.Contains(out, `\x00`) || !strings.Contains(out, `\x07`) {
		t.Fatalf("escapes missing: %q", out)
	}
	if got := Sanitize(strings.Repeat("a", 100), 16); len(got) != 16 {
		t.Fatalf("length bound: got %d", len(got))
	}
}

func TestRedactExcludesSensitiveKeysAtSource(t *testing.T) {
	fields := map[string]string{
		"Authorization":  "Bearer tok",
		"Set-Cookie":     "sid=1",
		"api_key":        "sk-123",
		"provider_token": "tok",
		"user_prompt":    "attack",
		"model_output":   "raw",
		"file_content":   "docs",
		"plain":          "kept",
	}
	out := Redact(fields)
	for _, k := range []string{"Authorization", "Set-Cookie", "api_key", "provider_token", "user_prompt", "model_output", "file_content"} {
		if out[k] != RedactMarker {
			t.Fatalf("key %q must be redacted, got %q", k, out[k])
		}
	}
	if out["plain"] != "kept" {
		t.Fatal("non-excluded fields must pass through")
	}
}

func TestLoggerBoundsDropsAndDelivers(t *testing.T) {
	delivered := make(chan Event, 64)
	l := NewLogger(2, 4096, 1000, func(ev Event) { delivered <- ev })
	defer l.Close()

	l.Emit(Event{Level: LevelInfo, Category: "runtime", MessageKey: "A", CorrelationID: "c"})
	l.Emit(Event{Level: LevelDebug, Category: "runtime", MessageKey: "B", CorrelationID: "c"})
	l.Emit(Event{Level: LevelWarn, Category: "runtime", MessageKey: "C", CorrelationID: "c"})
	if !l.Flush(2 * time.Second) {
		t.Fatal("flush must drain a shallow queue")
	}
	if len(delivered) < 2 {
		t.Fatalf("delivered %d events", len(delivered))
	}

	// Full queue: the lowest-severity queued record is dropped with a
	// counter, never a deadlock (C2-15 §5).
	time.Sleep(50 * time.Millisecond) // let the drain goroutine empty the buffer
	l.Emit(Event{Level: LevelDebug, Category: "cat", MessageKey: "D1"})
	l.Emit(Event{Level: LevelDebug, Category: "cat", MessageKey: "D2"})
	l.Emit(Event{Level: LevelError, Category: "cat", MessageKey: "E"})
	l.Emit(Event{Level: LevelError, Category: "cat", MessageKey: "E2"})
	dropped := l.Dropped()
	if dropped[LevelDebug] == 0 {
		t.Fatalf("debug drops must be counted: %+v", dropped)
	}
	if !l.Flush(2 * time.Second) {
		t.Fatal("flush after drops")
	}
}

func TestLoggerRateBoundsPerCategory(t *testing.T) {
	l := NewLogger(64, 4096, 2, nil) // 2 per minute per category
	defer l.Close()
	for i := 0; i < 5; i++ {
		l.Emit(Event{Level: LevelInfo, Category: "hot", MessageKey: "M"})
	}
	dropped := l.Dropped()
	if dropped[LevelInfo] != 3 {
		t.Fatalf("rate bound must drop beyond 2/min: dropped=%+v", dropped)
	}
	// Another category is unaffected.
	l.Emit(Event{Level: LevelInfo, Category: "cold", MessageKey: "M"})
	if dropped := l.Dropped(); dropped[LevelInfo] != 3 {
		t.Fatalf("categories must be bounded independently: %+v", dropped)
	}
}

func TestExportBundleInheritsRedaction(t *testing.T) {
	evs := []Event{{
		Level:      LevelWarn,
		Category:   "export",
		MessageKey: "K",
		Fields:     map[string]string{"session_token": "tok", "note": "ok"},
	}}
	out := ExportBundle(evs, 4096)
	if out[0].Fields["session_token"] != RedactMarker || out[0].Fields["note"] != "ok" {
		t.Fatalf("export must inherit the emit-boundary rules: %+v", out[0].Fields)
	}
}

// --- local HTTP boundary predicates (C2-14 §5) --------------------------

func TestIsLoopbackHost(t *testing.T) {
	yes := []string{"127.0.0.1", "127.8.8.8", "localhost", "::1", "[::1]", "127.0.0.1:8080", "[::1]:443", "foo.localhost"}
	no := []string{"192.168.1.10", "0.0.0.0", "example.com", "localhost.evil.com", "", "10.0.0.1:80"}
	for _, h := range yes {
		if !IsLoopbackHost(h) {
			t.Fatalf("%q must be loopback", h)
		}
	}
	for _, h := range no {
		if IsLoopbackHost(h) {
			t.Fatalf("%q must not be loopback", h)
		}
	}
}

func TestValidateBindAddress(t *testing.T) {
	if err := ValidateBindAddress("127.0.0.1", false); err != nil {
		t.Fatalf("loopback bind is the default: %v", err)
	}
	err := ValidateBindAddress("192.168.1.10", false)
	be, ok := err.(*ErrBoundary)
	if !ok || be.Reason != ReasonNonLoopbackBind {
		t.Fatalf("non-loopback bind must be refused: %v", err)
	}
	if err := ValidateBindAddress("192.168.1.10", true); err != nil {
		t.Fatalf("explicit operator opt-in is the only override: %v", err)
	}
}

func TestHostHeaderAllowlistBlocksRebinding(t *testing.T) {
	// DNS-rebinding defense [R34]: exact allowlist or loopback default.
	if !HostHeaderAllowed("127.0.0.1:8080", nil) {
		t.Fatal("loopback host is the default acceptance")
	}
	if HostHeaderAllowed("attacker.example", []string{"core.local"}) {
		t.Fatal("foreign hosts must be refused")
	}
	if !HostHeaderAllowed("CORE.LOCAL:8080", []string{"core.local"}) {
		t.Fatal("allowlist match is case-insensitive with the port stripped")
	}
}

func TestOriginAllowlist(t *testing.T) {
	if !OriginAllowed("http://localhost:7780", nil) {
		t.Fatal("loopback origin is the default acceptance")
	}
	if OriginAllowed("http://evil.example", nil) {
		t.Fatal("foreign origins must be refused")
	}
	if !OriginAllowed("https://ui.local", []string{"https://ui.local"}) {
		t.Fatal("allowlist match")
	}
	if OriginAllowed("", nil) {
		t.Fatal("empty origin is denied")
	}
}

func TestStateChangingGETProhibited(t *testing.T) {
	if err := CheckStateChangingGET("GET", true); err == nil {
		t.Fatal("a mutating GET is prohibited outright (C2-14 §5)")
	}
	if err := CheckStateChangingGET("GET", false); err != nil {
		t.Fatalf("read-only GET is fine: %v", err)
	}
	if err := CheckStateChangingGET("POST", true); err != nil {
		t.Fatalf("POST may mutate: %v", err)
	}
}

func TestCSRFCheck(t *testing.T) {
	// Reads are not CSRF-protected mutations.
	if err := CSRFCheck("GET", "", "", "", false); err != nil {
		t.Fatalf("GET must not require CSRF: %v", err)
	}
	// Mutations require origin enforcement + anti-CSRF token.
	if err := CSRFCheck("POST", "http://evil.example", "t", "t", false); err == nil {
		t.Fatal("foreign origin must be refused")
	}
	if err := CSRFCheck("POST", "http://localhost:7780", "", "t", false); err == nil {
		t.Fatal("missing CSRF token must be refused")
	}
	if err := CSRFCheck("POST", "http://localhost:7780", "wrong", "right", false); err == nil {
		t.Fatal("token mismatch must be refused")
	}
	if err := CSRFCheck("POST", "http://localhost:7780", "t", "t", false); err != nil {
		t.Fatalf("valid origin + token must pass: %v", err)
	}
	// Authenticated non-browser local clients (capability/session proven).
	if err := CSRFCheck("POST", "", "", "", true); err != nil {
		t.Fatalf("same-origin local client: %v", err)
	}
}

func TestCredentialNeverInURLsOrReferers(t *testing.T) {
	cred := "bootstrap-abc123"
	if err := CredentialSurfaceRule(cred, []string{"http://127.0.0.1/bootstrap?cred=" + cred}, nil); err == nil {
		t.Fatal("a credential in a URL must be refused")
	}
	if err := CredentialSurfaceRule(cred, nil, []string{"http://127.0.0.1/?cred=" + cred}); err == nil {
		t.Fatal("a credential in a Referer must be refused")
	}
	if err := CredentialSurfaceRule(cred, []string{"http://127.0.0.1/bootstrap"}, nil); err != nil {
		t.Fatalf("clean surfaces pass: %v", err)
	}
	if got := ScrubCredential("token bootstrap-abc123 leaked", cred); strings.Contains(got, cred) {
		t.Fatal("scrub must remove the credential")
	}
}
