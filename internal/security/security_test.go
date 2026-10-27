package security

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- policy / sessions -------------------------------------------------

func testSessions(t *testing.T) *SessionService {
	t.Helper()
	svc := NewSessionService("inst-test", "/data/dir")
	base := time.Now()
	svc.Clock = func() time.Time { return base }
	return svc
}

func localPrincipal() Principal {
	return Principal{Kind: PrincipalLocalUser, ID: "local"}
}

func TestLocalPolicyFailsClosed(t *testing.T) {
	svc := testSessions(t)
	lp := NewLocalPolicy(svc)
	ctx := PolicyContext{CorrelationID: "c1", Credential: "no-such-token"}

	cases := []struct {
		name      string
		p         Principal
		r         Resource
		a         Action
		ctx       PolicyContext
		wantDeny  string
		wantAllow bool
	}{
		{"unknown principal kind", Principal{Kind: PrincipalPlugin, ID: "p1"}, Resource{Type: ResourceCore}, ActionStatusRead, ctx, ReasonUnknownPrincipal, false},
		{"empty principal id", Principal{Kind: PrincipalLocalUser}, Resource{Type: ResourceCore}, ActionStatusRead, ctx, ReasonUnknownPrincipal, false},
		{"malformed context", localPrincipal(), Resource{Type: ResourceCore}, ActionStatusRead, PolicyContext{Credential: "x"}, ReasonMalformedContext, false},
		{"out-of-scope resource", localPrincipal(), Resource{Type: "remote-cluster"}, ActionStatusRead, ctx, ReasonResourceOutOfScope, false},
		{"unknown action", localPrincipal(), Resource{Type: ResourceCore}, Action("core.nuke"), ctx, ReasonDenyDefault, false},
		{"invalid token", localPrincipal(), Resource{Type: ResourceCore}, ActionStatusRead, ctx, ReasonSessionInvalid, false},
		{"nil sessions denies", localPrincipal(), Resource{Type: ResourceCore}, ActionDataRead, ctx, ReasonPolicyUnavailable, false},
	}
	for _, tc := range cases {
		policy := lp
		if tc.wantDeny == ReasonPolicyUnavailable {
			policy = &LocalPolicy{} // no sessions wired
		}
		d := policy.Decide(tc.p, tc.r, tc.a, tc.ctx)
		if d.Allow || d.ReasonCode != tc.wantDeny {
			t.Fatalf("%s: decision = %+v, want deny %s", tc.name, d, tc.wantDeny)
		}
	}
}

func TestSessionBootstrapExchangeAndReplayInvalidates(t *testing.T) {
	svc := testSessions(t)
	cred, err := svc.IssueBootstrapCredential("")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := svc.Exchange(cred, "", []Scope{ScopeLifecycle, ScopeData})
	if err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	if !svc.Validate(tok.Secret, ScopeLifecycle) || !svc.Validate(tok.Secret, ScopeData) {
		t.Fatal("minted session must cover both scopes")
	}
	// Replay: rejected AND the minted session is invalidated (C2-14 §3).
	if _, err := svc.Exchange(cred, "", []Scope{ScopeLifecycle}); err == nil {
		t.Fatal("replayed bootstrap credential must be rejected")
	}
	if svc.Validate(tok.Secret, ScopeLifecycle) {
		t.Fatal("replay must invalidate the original exchange")
	}
}

func TestSessionExpiryIdleRotationRevocation(t *testing.T) {
	now := time.Now()
	svc := NewSessionService("inst", "/data")
	svc.Clock = func() time.Time { return now }
	cred, _ := svc.IssueBootstrapCredential("")
	tok, err := svc.Exchange(cred, "", []Scope{ScopeLifecycle})
	if err != nil {
		t.Fatal(err)
	}

	// Idle timeout (Proposed 8 h): inactivity expires the session.
	now = now.Add(SessionIdleTimeout + time.Minute)
	if svc.Validate(tok.Secret, ScopeLifecycle) {
		t.Fatal("idle timeout must expire the session")
	}

	// Fresh session; absolute TTL (Proposed 24 h) wins over activity.
	now = time.Now()
	cred2, _ := svc.IssueBootstrapCredential("")
	tok2, _ := svc.Exchange(cred2, "", []Scope{ScopeLifecycle})
	now = now.Add(time.Hour)
	if !svc.Validate(tok2.Secret, ScopeLifecycle) {
		t.Fatal("fresh session must validate within the idle window")
	}
	now = now.Add(SessionAbsoluteTTL)
	if svc.Validate(tok2.Secret, ScopeLifecycle) {
		t.Fatal("absolute TTL must expire the session despite activity")
	}

	// Rotation on scope elevation mints a new token and revokes the old.
	now = time.Now()
	cred3, _ := svc.IssueBootstrapCredential("")
	tok3, _ := svc.Exchange(cred3, "", []Scope{ScopeLifecycle})
	rotated, err := svc.Rotate(tok3.Secret, []Scope{ScopeData})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if svc.Validate(tok3.Secret, ScopeLifecycle) {
		t.Fatal("rotation must revoke the old token")
	}
	if !svc.Validate(rotated.Secret, ScopeData) {
		t.Fatal("rotated token must carry the elevated scope")
	}

	// Explicit revocation (logout) and RevokeAll (lock takeover).
	now = time.Now()
	cred4, _ := svc.IssueBootstrapCredential("")
	tok4, _ := svc.Exchange(cred4, "", []Scope{ScopeData})
	svc.Revoke(tok4.Secret)
	if svc.Validate(tok4.Secret, ScopeData) {
		t.Fatal("revoked token must fail validation")
	}
	cred5, _ := svc.IssueBootstrapCredential("")
	tok5, _ := svc.Exchange(cred5, "", []Scope{ScopeData})
	svc.RevokeAll()
	if svc.Validate(tok5.Secret, ScopeData) {
		t.Fatal("RevokeAll must invalidate every live session")
	}
}

func TestSessionScopeAndBinding(t *testing.T) {
	svc := testSessions(t)
	cred, _ := svc.IssueBootstrapCredential("")
	tok, _ := svc.Exchange(cred, "", []Scope{ScopeLifecycle})
	// A lifecycle-only token must not authorize a data-scope action.
	if svc.Validate(tok.Secret, ScopeData) {
		t.Fatal("lifecycle scope must not cover data operations")
	}
	// Wrong origin binding is refused at exchange time.
	cred2, _ := svc.IssueBootstrapCredential("http://127.0.0.1:7780")
	if _, err := svc.Exchange(cred2, "http://evil.example", []Scope{ScopeData}); err == nil {
		t.Fatal("origin-bound bootstrap must refuse a foreign origin")
	}
}

func TestIPCCapabilityCredentialScope(t *testing.T) {
	svc := testSessions(t)
	lp := NewLocalPolicy(svc)
	ctx := PolicyContext{CorrelationID: "c1", Credential: CredentialIPCCapability}
	// The sentinel covers only the local lifecycle surface (C2-14 §2).
	if d := lp.Decide(localPrincipal(), Resource{Type: ResourceCore}, ActionLifecycleStop, ctx); !d.Allow {
		t.Fatalf("IPC stop must be authorized: %+v", d)
	}
	if d := lp.Decide(localPrincipal(), Resource{Type: ResourceCore}, ActionStatusRead, ctx); !d.Allow {
		t.Fatalf("IPC status must be authorized: %+v", d)
	}
	// Data operations never accept the capability sentinel.
	if d := lp.Decide(localPrincipal(), Resource{Type: ResourceStorage}, ActionDataWrite, ctx); d.Allow || d.ReasonCode != ReasonScopeMismatch {
		t.Fatalf("data write must deny the capability sentinel: %+v", d)
	}
	// Disabled acceptance denies (tests).
	strict := &LocalPolicy{Sessions: svc, AcceptIPCCredential: false}
	if d := strict.Decide(localPrincipal(), Resource{Type: ResourceCore}, ActionLifecycleStop, ctx); d.Allow {
		t.Fatal("disabled IPC acceptance must deny")
	}
}

// --- secrets ------------------------------------------------------------

func TestSecretHandleValidation(t *testing.T) {
	valid := []Handle{{Scope: "provider:openai", Name: "api-key"}, {Scope: "connection:main", Name: "password"}}
	for _, h := range valid {
		if err := h.validate(); err != nil {
			t.Fatalf("handle %+v must validate: %v", h, err)
		}
	}
	invalid := []Handle{
		{Scope: "", Name: "x"},
		{Scope: "s", Name: ""},
		{Scope: strings.Repeat("s", 200), Name: "n"},
		{Scope: "has space", Name: "n"},
	}
	for _, h := range invalid {
		if err := h.validate(); err == nil {
			t.Fatalf("handle %+v must be refused", h)
		}
	}
}

type fakeSecretStore struct {
	value   []byte
	status  SecretStatus
	resolve error
}

func (f fakeSecretStore) Resolve(ctx context.Context, h Handle) ([]byte, error) {
	if f.resolve != nil {
		return nil, f.resolve
	}
	if f.status != SecretPresent {
		return nil, errSecret(h, f.status)
	}
	return f.value, nil
}

func (f fakeSecretStore) Status(ctx context.Context, h Handle) (SecretStatus, error) {
	return f.status, nil
}

func TestSecretResolutionFailsClosed(t *testing.T) {
	h := Handle{Scope: "provider:p", Name: "api-key"}
	// Missing/revoked/corrupt/inaccessible map onto typed family errors.
	cases := []struct {
		status   SecretStatus
		wantCode string
		wantReas string
	}{
		{SecretMissing, "NOT_FOUND", ReasonSecretMissing},
		{SecretRevoked, "NOT_FOUND", ReasonSecretRevoked},
		{SecretCorrupt, "STATE_CORRUPT", ReasonSecretCorrupt},
		{SecretInaccessib, "STORAGE_UNAVAILABLE", ReasonSecretInaccessible},
	}
	for _, tc := range cases {
		_, err := ResolveForUse(context.Background(), fakeSecretStore{status: tc.status}, h)
		se, ok := err.(*SecretError)
		if !ok {
			t.Fatalf("status %s: want SecretError, got %v", tc.status, err)
		}
		if string(se.Code) != tc.wantCode || se.Reason != tc.wantReas {
			t.Fatalf("status %s: got %s/%s, want %s/%s", tc.status, se.Code, se.Reason, tc.wantCode, tc.wantReas)
		}
		env, envErr := se.Envelope("corr")
		if envErr != nil {
			t.Fatalf("envelope: %v", envErr)
		}
		if env.Data["recovery"] != ReasonReenterCredential {
			t.Fatal("the recovery path (re-enter the credential) must be present")
		}
		if env.Data["handle"] != h.String() {
			t.Fatal("the handle reference (name only) must be present")
		}
	}
	// The D-37-open default store denies everything.
	_, err := ResolveForUse(context.Background(), UnavailableStore{}, h)
	if err == nil {
		t.Fatal("the unavailable store must fail closed")
	}
	// An empty resolved value is missing, never silently accepted.
	_, err = ResolveForUse(context.Background(), fakeSecretStore{status: SecretPresent, value: nil}, h)
	if err == nil {
		t.Fatal("empty secret value must be treated as missing")
	}
	// A real value resolves at the call boundary only.
	v, err := ResolveForUse(context.Background(), fakeSecretStore{status: SecretPresent, value: []byte("s3cret")}, h)
	if err != nil || string(v) != "s3cret" {
		t.Fatalf("present secret must resolve: %v", err)
	}
}
