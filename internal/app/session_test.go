package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// sessionTestEnv is the C-4 session-command fixture (IMPL-23): the same
// wiring class newTestEnv uses, plus the session service the commands
// serve and a dedicated audit sink the tests read back.
type sessionTestEnv struct {
	slice    *Slice
	sessions *security.SessionService
	guard    *security.Guard
	sink     *memoryAuditStore
}

func newSessionTestEnv(t *testing.T) *sessionTestEnv {
	t.Helper()
	ctx := context.Background()
	st, err := storage.Open(ctx, storage.Options{
		Path:                   filepath.Join(t.TempDir(), "karyora.db"),
		PoolMaxOpen:            2,
		WriterQueueDepth:       4,
		WriterDeadline:         2 * time.Second,
		BusyTimeout:            200 * time.Millisecond,
		WALAutocheckpointPages: 1000,
		Synchronous:            "NORMAL",
		ReadBudget:             2 * time.Second,
		MaxObjectBytes:         1 << 20,
		MaxObjects:             1000,
		GCGracePeriod:          time.Second,
		PublishInterval:        10 * time.Millisecond,
		PublishBatch:           64,
		Retention:              24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("storage open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mock := providers.NewMockAdapter(providers.MockBehavior{})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(mock.SeedDataHandling(mock.DefaultProfile("m")))
	exec, err := providers.NewExecutor(mock, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatalf("provider executor: %v", err)
	}

	sessions := security.NewSessionService("inst-test", filepath.Join(t.TempDir(), "data"))
	sink := &memoryAuditStore{}
	guard := &security.Guard{
		Policy: security.NewLocalPolicy(sessions),
		Audit:  security.NewAuditService(sink, 64, 1000),
	}
	slice, err := NewSlice(SliceDeps{
		Relay:     st,
		Snapshots: st,
		Objects:   st,
		Events:    st,
		Guard:     guard,
		Providers: map[string]ProviderExecutor{dprovider.ProviderMock: exec},
		Backoff:   st,
		Sessions:  sessions,
	})
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	return &sessionTestEnv{slice: slice, sessions: sessions, guard: guard, sink: sink}
}

// sessionToken exchanges one bootstrap credential for a live data-scoped
// session token (the browser session's own posture: the full coded
// register request class collapses to the data scope for these commands).
func (e *sessionTestEnv) sessionToken(t *testing.T) string {
	t.Helper()
	cred, err := e.sessions.IssueBootstrapCredential("cli")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	token, err := e.sessions.Exchange(cred, "cli", []security.Scope{security.ScopeData})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return token.Secret
}

func (e *sessionTestEnv) ctxWith(token string) context.Context {
	return WithCaller(context.Background(), "local", token)
}

// auditRecord finds the one operation-specific record of the given action.
func (e *sessionTestEnv) auditRecord(t *testing.T, action string) security.AuditRecord {
	t.Helper()
	records, err := e.sink.Records()
	if err != nil {
		t.Fatalf("audit records: %v", err)
	}
	var found *security.AuditRecord
	for _, rec := range records {
		var ar security.AuditRecord
		if err := json.Unmarshal(rec.Payload, &ar); err != nil {
			t.Fatalf("audit payload: %v", err)
		}
		if ar.Action == action {
			if found != nil {
				t.Fatalf("expected ONE %s audit record, found multiple", action)
			}
			arCopy := ar
			found = &arCopy
		}
	}
	if found == nil {
		t.Fatalf("no %s audit record was written", action)
	}
	return *found
}

func envelopeOf(t *testing.T, err error) errs.Envelope {
	t.Helper()
	var ee *EnvelopeError
	if !errors.As(err, &ee) {
		t.Fatalf("expected a typed D-35 envelope error, got %v", err)
	}
	return ee.Envelope
}

// TestSessionLogoutRevokesPresentedToken pins the per-token logout class:
// a live session's own credential logs that session out — the coded
// Revoke served through the one decision port, the operation-specific
// audit record written fail-closed BEFORE the effect.
func TestSessionLogoutRevokesPresentedToken(t *testing.T) {
	env := newSessionTestEnv(t)
	token := env.sessionToken(t)
	if !env.sessions.Validate(token, security.ScopeData) {
		t.Fatal("precondition: the exchanged session must validate before the command")
	}

	res, err := env.slice.SessionLogout(env.ctxWith(token))
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !res.Revoked {
		t.Fatal("the confirmed logout must report the revocation truth")
	}
	if env.sessions.Validate(token, security.ScopeData) {
		t.Fatal("the logged-out session must stop authorizing immediately Core-side (S-5)")
	}

	rec := env.auditRecord(t, "session.logout")
	if rec.Scope != "core" || rec.PolicyDecision != "allow" || rec.Outcome != "logout" {
		t.Fatalf("the audit record must carry the honest instance-scoped posture, got scope=%q decision=%q outcome=%q", rec.Scope, rec.PolicyDecision, rec.Outcome)
	}
	if !strings.Contains(rec.Actor, "local-user:") {
		t.Fatalf("the audit record must carry the actor, got %q", rec.Actor)
	}
	if rec.CorrelationID == "" {
		t.Fatal("the audit record must carry the command's correlation id")
	}
}

// TestSessionLogoutRefusals pins the failure law (P1.8 §6.2): every
// refusal is typed on the D-35 envelope and NEVER revokes anything — the
// client never assumes the session died because the command failed.
func TestSessionLogoutRefusals(t *testing.T) {
	env := newSessionTestEnv(t)

	// No presented credential (the IPC capability class): the decision
	// port denies (data operations never accept it) — typed AUTHZ_DENIED,
	// no effect.
	if _, err := env.slice.SessionLogout(WithCaller(context.Background(), "local", "")); err == nil {
		t.Fatal("a credential-less logout must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeAuthzDenied {
		t.Fatalf("the credential-less refusal must be typed AUTHZ_DENIED, got %q", env.Code)
	}

	// A garbage credential: the same typed refusal, no effect.
	if _, err := env.slice.SessionLogout(env.ctxWith("kry1-not-a-real-token-000000000000")); err == nil {
		t.Fatal("a garbage-credential logout must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeAuthzDenied {
		t.Fatalf("the garbage-credential refusal must be typed AUTHZ_DENIED, got %q", env.Code)
	}

	// A live session stays live through every refusal above.
	token := env.sessionToken(t)
	other := env.sessionToken(t)
	if !env.sessions.Validate(token, security.ScopeData) || !env.sessions.Validate(other, security.ScopeData) {
		t.Fatal("the refusals above must not have revoked any live session")
	}

	// An already-revoked credential: the typed session reason rides the
	// envelope data (the coded register), and the refusal is a refusal —
	// never a fresh success report.
	if _, err := env.slice.SessionLogout(env.ctxWith(token)); err != nil {
		t.Fatalf("first logout: %v", err)
	}
	if env.sessions.Validate(token, security.ScopeData) {
		t.Fatal("the first logout must have revoked the session")
	}
	_, err := env.slice.SessionLogout(env.ctxWith(token))
	if err == nil {
		t.Fatal("a second logout with the revoked credential must refuse (no fabricated success)")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeAuthzDenied {
		t.Fatalf("the revoked-credential refusal must be typed AUTHZ_DENIED, got %q", env.Code)
	} else if reason, _ := env.Data["reason"].(string); reason != "SESSION_REVOKED" {
		t.Fatalf("the coded session reason must ride the envelope data, got %v", env.Data["reason"])
	}

	// The other live session was untouched by all of it.
	if !env.sessions.Validate(other, security.ScopeData) {
		t.Fatal("the per-token logout must never touch any other session")
	}
}

// TestSessionLogoutUnwiredPosture pins the optional-port discipline: a
// slice wired without the session service refuses typed and fail-closed,
// revoking nothing.
func TestSessionLogoutUnwiredPosture(t *testing.T) {
	env := newSessionTestEnv(t)
	env.slice.deps.Sessions = nil
	token := env.sessionToken(t)

	_, err := env.slice.SessionLogout(env.ctxWith(token))
	if err == nil {
		t.Fatal("the unwired session-command surface must refuse")
	}
	env2 := envelopeOf(t, err)
	if env2.Code != errs.CodeStorageUnavailable {
		t.Fatalf("the unwired refusal must be typed STORAGE_UNAVAILABLE, got %q", env2.Code)
	}
	if reason, _ := env2.Data["reason"].(string); reason != "session service is not wired" {
		t.Fatalf("the unwired reason must ride the envelope data, got %v", env2.Data["reason"])
	}
	if !env.sessions.Validate(token, security.ScopeData) {
		t.Fatal("the unwired refusal must not revoke the live session")
	}
}

// TestSessionRevokeAllInvalidatesEverySession pins the credential-change
// class: every live session of the instance is invalidated — including
// the presented one — and the result reports the honest post-effect live
// count (the coded diagnostics class: counts only, never token material).
func TestSessionRevokeAllInvalidatesEverySession(t *testing.T) {
	env := newSessionTestEnv(t)
	first := env.sessionToken(t)
	second := env.sessionToken(t)
	third := env.sessionToken(t)
	if !env.sessions.Validate(first, security.ScopeData) || !env.sessions.Validate(second, security.ScopeData) || !env.sessions.Validate(third, security.ScopeData) {
		t.Fatal("precondition: all three sessions must validate before the command")
	}

	res, err := env.slice.SessionRevokeAll(env.ctxWith(second))
	if err != nil {
		t.Fatalf("revoke-all: %v", err)
	}
	if res.LiveSessions != 0 {
		t.Fatalf("every live session must be invalidated, got post-count %d", res.LiveSessions)
	}
	for name, token := range map[string]string{"first": first, "second": second, "third": third} {
		if env.sessions.Validate(token, security.ScopeData) {
			t.Fatalf("the %s session must be dead after the credential-change revocation", name)
		}
	}

	rec := env.auditRecord(t, "session.revoke_all")
	if rec.Scope != "core" || rec.PolicyDecision != "allow" || rec.Outcome != "revoke_all" {
		t.Fatalf("the audit record must carry the honest instance-scoped posture, got scope=%q decision=%q outcome=%q", rec.Scope, rec.PolicyDecision, rec.Outcome)
	}
}

// TestSessionRevokeAllRefusals pins the failure law on the
// credential-change class: a refused command invalidates nothing.
func TestSessionRevokeAllRefusals(t *testing.T) {
	env := newSessionTestEnv(t)
	token := env.sessionToken(t)
	other := env.sessionToken(t)

	if _, err := env.slice.SessionRevokeAll(env.ctxWith("kry1-not-a-real-token-000000000000")); err == nil {
		t.Fatal("a garbage-credential revoke-all must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeAuthzDenied {
		t.Fatalf("the refusal must be typed AUTHZ_DENIED, got %q", env.Code)
	}
	if !env.sessions.Validate(token, security.ScopeData) || !env.sessions.Validate(other, security.ScopeData) {
		t.Fatal("the refused revoke-all must not have invalidated any live session")
	}
}
