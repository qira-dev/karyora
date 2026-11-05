package httplocal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/security"
)

// The C-4 logout/revocation wire-command tests (IMPL-23, the S9-adopted
// P-S9-02 class over the wire): the per-token logout and the
// credential-change revocation served through the real listener — the
// route register, the mutates boundary posture, the typed refusals on
// the D-35 envelope, and the S-5 expected behavior over the wire (a
// revoked session stops working immediately; the next decision denies
// with the coded session reason).

// secondSession exchanges one more live session in-process (the same C-1
// exchange the wire performs) — the second live session the revoke-all
// test needs.
func (e *testEnv) secondSession(t *testing.T) string {
	t.Helper()
	cred, err := e.sessions.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	token, err := e.sessions.Exchange(cred, "", []security.Scope{security.ScopeData})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return token.Secret
}

func TestSessionLogoutCommandEndpoint(t *testing.T) {
	env := newTestEnv(t)
	if !env.sessions.Validate(env.token, security.ScopeData) {
		t.Fatal("precondition: the harness session must validate")
	}

	status, body := env.do(t, http.MethodPost, "/v1/session/logout", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the logout command must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Revoked bool `json:"revoked"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if !res.Revoked {
		t.Fatalf("the confirmed logout must report the revocation, got %s", body)
	}

	// The S-5 expected behavior over the wire: the revoked session stops
	// working immediately — the next decision for the held token denies
	// with the coded session reason (SESSION_REVOKED, the coded register).
	status, body = env.do(t, http.MethodGet, "/v1/jobs?limit=1", "", env.token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("the logged-out session's next decision must refuse 403, got %d: %s", status, body)
	}
	if !strings.Contains(string(body), "AUTHZ_DENIED") || !strings.Contains(string(body), "SESSION_REVOKED") {
		t.Fatalf("the typed session reason must ride the D-35 envelope: %s", body)
	}
}

func TestSessionRevokeAllCommandEndpoint(t *testing.T) {
	env := newTestEnv(t)
	second := env.secondSession(t)
	if !env.sessions.Validate(env.token, security.ScopeData) || !env.sessions.Validate(second, security.ScopeData) {
		t.Fatal("precondition: both sessions must validate")
	}

	status, body := env.do(t, http.MethodPost, "/v1/session/revoke-all", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the revoke-all command must answer 200, got %d: %s", status, body)
	}
	var res struct {
		LiveSessions int `json:"live_sessions"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if res.LiveSessions != 0 {
		t.Fatalf("every live session must be invalidated, got post-count %d (%s)", res.LiveSessions, body)
	}

	// Every held token denies at its next decision with the typed reason.
	for name, token := range map[string]string{"presented": env.token, "other": second} {
		status, body = env.do(t, http.MethodGet, "/v1/jobs?limit=1", "", token, nil)
		if status != http.StatusForbidden || !strings.Contains(string(body), "SESSION_REVOKED") {
			t.Fatalf("the %s session must deny with the typed reason, got %d: %s", name, status, body)
		}
	}
}

func TestSessionCommandEndpointRefusals(t *testing.T) {
	env := newTestEnv(t)
	token := env.token
	cases := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "logout_unauthenticated", method: http.MethodPost, path: "/v1/session/logout", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "revoke_all_unauthenticated", method: http.MethodPost, path: "/v1/session/revoke-all", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "logout_garbage_credential", method: http.MethodPost, path: "/v1/session/logout", token: "kry1-not-a-real-token-000000000000", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "logout_get_is_no_route", method: http.MethodGet, path: "/v1/session/logout", token: token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "revoke_all_get_is_no_route", method: http.MethodGet, path: "/v1/session/revoke-all", token: token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "unknown_session_route", method: http.MethodPost, path: "/v1/session/nonexistent", token: token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, "", tc.token, nil)
			if status != tc.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.wantStatus, status, body)
			}
			if !strings.Contains(string(body), tc.wantCode) {
				t.Fatalf("%s: the refusal must carry the typed %s code: %s", tc.name, tc.wantCode, body)
			}
		})
	}

	// The failure law over the wire (P1.8 §6.2): every refusal above
	// revoked nothing — the harness session still authorizes.
	if !env.sessions.Validate(token, security.ScopeData) {
		t.Fatal("the refusals must not have revoked the live session")
	}
}
