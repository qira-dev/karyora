package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

func TestValidateDestinationURL(t *testing.T) {
	local := DefaultEgressPolicy(CoreLocal)
	remote := DefaultEgressPolicy(CoreRemote)

	// https is always acceptable syntactically
	if _, err := ValidateDestinationURL(remote, "https://api.example.test/v1"); err != nil {
		t.Fatalf("https refused: %v", err)
	}

	// plaintext http only for explicit local loopback
	if _, err := ValidateDestinationURL(remote, "http://127.0.0.1:1/v1"); err == nil {
		t.Fatal("remote core accepted plaintext http")
	}
	if _, err := ValidateDestinationURL(local, "http://10.0.0.1:1/v1"); err == nil {
		t.Fatal("plaintext http to a private LAN address accepted on a local core")
	}
	if _, err := ValidateDestinationURL(local, "http://localhost:11434/v1"); err != nil {
		t.Fatalf("explicit local loopback http refused: %v", err)
	}

	// credentials never in URLs
	for _, u := range []string{
		"https://user:pass@api.example.test/v1",
		"https://api.example.test/v1?key=secret-value",
		"https://api.example.test/v1?api_key=secret-value",
		"https://api.example.test/v1#fragment",
	} {
		if _, err := ValidateDestinationURL(remote, u); err == nil {
			t.Errorf("credential-bearing URL accepted: %s", u)
		}
	}

	// empty host
	if _, err := ValidateDestinationURL(remote, "https:///v1"); err == nil {
		t.Fatal("empty host accepted")
	}
}

func TestRemoteCoreRefusesProhibitedRanges(t *testing.T) {
	// The local httptest listener IS loopback: a remote core must refuse it
	// at dial time even though the URL validated syntactically.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixtureChatBody(&directAdapter{family: familyOpenAI}, "no"))
	}))
	defer srv.Close()

	reg := dprovider.NewRegistry(time.Now)
	a := NewCustomOpenAIAdapter(srv.URL, "m", nil)
	reg.Upsert(verifiedFixtureRecord(a, time.Now()))
	exec, err := NewExecutor(a, DefaultEgressPolicy(CoreRemote), fixtureSecretStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	plan := conformancePlan(a, srv.URL)
	_, err = exec.Execute(context.Background(), plan)
	pe, ok := err.(*dprovider.ProviderError)
	if !ok || pe.Kind != dprovider.ErrEgressPolicy {
		t.Fatalf("remote core must refuse loopback destinations, got %v", err)
	}
	// The plaintext fixture URL is refused at URL validation; the dial-time
	// loopback refusal for https destinations is proven by
	// TestDialGuardChecksResolvedAddresses.
	if !strings.Contains(pe.Detail, "prohibited destination range") &&
		!strings.Contains(pe.Detail, "plaintext http") {
		t.Fatalf("unexpected refusal detail: %s", pe.Detail)
	}
}

func TestLocalCoreAllowsExplicitLocalConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixtureChatBody(&directAdapter{family: familyOpenAI}, "local"))
	}))
	defer srv.Close()

	reg := dprovider.NewRegistry(time.Now)
	a := NewLocalAdapter(srv.URL, "m", nil)
	reg.Upsert(verifiedFixtureRecord(a, time.Now()))
	exec, err := NewExecutor(a, DefaultEgressPolicy(CoreLocal), fixtureSecretStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	plan := conformancePlan(a, srv.URL)
	plan.Connection.ExplicitLocal = true
	res, err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("explicit local connection refused: %v", err)
	}
	if res.Text != "local" {
		t.Fatalf("unexpected result %q", res.Text)
	}

	// The same local destination is refused on a REMOTE core (C3-30: remote
	// cores cannot forward local endpoints on behalf of users).
	execRemote, err := NewExecutor(a, DefaultEgressPolicy(CoreRemote), fixtureSecretStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = execRemote.Execute(context.Background(), plan)
	if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrEgressPolicy {
		t.Fatalf("remote core must refuse an explicit local connection, got %v", err)
	}

	// A local connection without the explicit record is refused even locally.
	plan.Connection.ExplicitLocal = false
	_, err = exec.Execute(context.Background(), plan)
	if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrEgressPolicy {
		t.Fatalf("implicit local connection must be refused, got %v", err)
	}
}

func TestRedirectsAreRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/moved", http.StatusFound)
	}))
	defer srv.Close()

	reg := dprovider.NewRegistry(time.Now)
	a := NewCustomOpenAIAdapter(srv.URL, "m", nil)
	reg.Upsert(verifiedFixtureRecord(a, time.Now()))
	exec, _ := NewExecutor(a, DefaultEgressPolicy(CoreLocal), fixtureSecretStore{}, reg)
	_, err := exec.Execute(context.Background(), conformancePlan(a, srv.URL))
	pe, ok := err.(*dprovider.ProviderError)
	if !ok || pe.Kind != dprovider.ErrEgressPolicy {
		t.Fatalf("redirects must be refused, got %v", err)
	}
}

func TestHeaderPolicy(t *testing.T) {
	// forbidden families are refused on connection headers
	for _, name := range []string{"Host", "Authorization", "X-Api-Key", "X-Goog-Api-Key",
		"X-Forwarded-For", "Via", "Proxy-Authorization", "Connection", "Cookie"} {
		if err := ValidateExtraHeader(name, "x"); err == nil {
			t.Errorf("header %s must be refused on a connection record", name)
		}
	}
	// header injection is refused
	if err := ValidateExtraHeader("X-Good", "value\r\nX-Evil: 1"); err == nil {
		t.Fatal("header injection accepted")
	}
	// benign headers pass
	if err := ValidateExtraHeader("X-Custom-Trace", "abc"); err != nil {
		t.Fatalf("benign header refused: %v", err)
	}
}

func TestDialGuardChecksResolvedAddresses(t *testing.T) {
	// "localhost" resolves to the loopback range: a remote core must refuse
	// at dial time (the DNS-rebinding defense path — every resolved address
	// is checked before the socket opens).
	reg := dprovider.NewRegistry(time.Now)
	a := NewCustomOpenAIAdapter("https://localhost:1/v1", "m", nil)
	reg.Upsert(verifiedFixtureRecord(a, time.Now()))
	exec, _ := NewExecutor(a, DefaultEgressPolicy(CoreRemote), fixtureSecretStore{}, reg)
	_, err := exec.Execute(context.Background(), conformancePlan(a, "https://localhost:1/v1"))
	pe, ok := err.(*dprovider.ProviderError)
	if !ok || pe.Kind != dprovider.ErrEgressPolicy {
		t.Fatalf("resolved loopback must be refused for a remote core, got %v", err)
	}
}

func TestCappedBody(t *testing.T) {
	policy := DefaultEgressPolicy(CoreLocal)
	policy.MaxResponseBytes = 8
	if _, err := cappedBody(strings.NewReader("12345678"), policy); err != nil {
		t.Fatalf("exact-size body refused: %v", err)
	}
	_, err := cappedBody(strings.NewReader("123456789"), policy)
	pe, ok := err.(*dprovider.ProviderError)
	if !ok || pe.Kind != dprovider.ErrResponseTooLarge {
		t.Fatalf("oversized body must be refused, got %v", err)
	}
	_, err = cappedBody(io.NopCloser(strings.NewReader("ok")), policy)
	if err != nil {
		t.Fatalf("small body refused: %v", err)
	}
}

func TestTLSFloor(t *testing.T) {
	tr := NewTransport(DefaultEgressPolicy(CoreLocal))
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion < tlsVersion12() {
		t.Fatal("TLS 1.2 floor not enforced")
	}
	if tr.Proxy != nil {
		t.Fatal("proxies must stay unsupported in this slice (credentials could reach a proxy)")
	}
}
