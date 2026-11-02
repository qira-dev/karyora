package httplocal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The pre-submit review endpoint (the owner's card-13 working candidate):
// the wire surface pins — the coded eight-field payload inside the
// `review` wrapper, the typed refusals, the read authorization through
// the decision port, and the proof that the review creates nothing.

func reviewBody() string {
	return `{"task":"Summarize the recorded fixture corpus.","provider":"mock","model":"m","classification":"test-fixture"}`
}

func TestReviewEndpoint(t *testing.T) {
	env := newTestEnv(t)

	status, body := env.do(t, http.MethodPost, "/v1/jobs/review", reviewBody(), env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("review must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Review map[string]any `json:"review"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	review := res.Review
	// The eight recorded review fields, exactly.
	for _, field := range []string{"intent", "sources", "provider_model", "tool_effects",
		"permission", "policy", "approval_need", "budget"} {
		if _, ok := review[field]; !ok {
			t.Fatalf("the review payload must carry the %q field", field)
		}
	}
	// Field truths as the fixture environment determines them.
	if review["intent"].(map[string]any)["task"] != "Summarize the recorded fixture corpus." {
		t.Fatalf("intent must echo the task verbatim: %v", review["intent"])
	}
	pm := review["provider_model"].(map[string]any)
	if pm["wired"] != true || pm["provider"] != dprovider.ProviderMock {
		t.Fatalf("provider_model must report the wired truth: %v", pm)
	}
	effects := review["tool_effects"].([]any)
	if len(effects) != 2 {
		t.Fatalf("the projection carries two effects, got %d", len(effects))
	}
	perm := review["permission"].(map[string]any)
	if perm["allowed"] != true {
		t.Fatalf("the harness session must be allowed: %v", perm)
	}
	pol := review["policy"].(map[string]any)
	if pol["egress_allowed"] != true {
		t.Fatalf("the fixture classification must pass the egress gate: %v", pol)
	}
	if review["approval_need"].(map[string]any)["required"] != false {
		t.Fatalf("the coded approval posture must be reported: %v", review["approval_need"])
	}

	// The review created nothing: the Jobs list stays empty.
	status, body = env.do(t, http.MethodGet, "/v1/jobs", "", env.token, nil)
	if status != http.StatusOK || !strings.Contains(string(body), `"jobs":[]`) {
		t.Fatalf("a review must create no durable state, got %d: %s", status, body)
	}
}

func TestReviewEndpointRefusals(t *testing.T) {
	env := newTestEnv(t)

	// An invalid draft shape: the typed VALIDATION_FAILED envelope with
	// its field.
	status, body := env.do(t, http.MethodPost, "/v1/jobs/review",
		`{"task":"","provider":"mock"}`, env.token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("an invalid draft must refuse 400, got %d: %s", status, body)
	}
	if !strings.Contains(string(body), "VALIDATION_FAILED") || !strings.Contains(string(body), `"field":"task"`) {
		t.Fatalf("the refusal must be the typed envelope: %s", body)
	}

	// No session credential: the decision port's denial.
	status, body = env.do(t, http.MethodPost, "/v1/jobs/review", reviewBody(), "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("an unauthenticated review must refuse 403, got %d: %s", status, body)
	}
	if !strings.Contains(string(body), "AUTHZ_DENIED") {
		t.Fatalf("the denial must be the coded envelope: %s", body)
	}

	// The POST class boundary: a browser-shaped mutation without a
	// validated session refuses fail-closed at the CSRF predicate (the
	// same posture every POST route carries).
	w := env.raw(http.MethodPost, "/v1/jobs/review", reviewBody(), func(r *http.Request) {
		r.Header.Set("Origin", "http://127.0.0.1:8080")
	})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "MISSING_CSRF_TOKEN") {
		t.Fatalf("a browser-shaped unauthenticated review must refuse at the boundary, got %d: %s",
			w.Code, w.Body.String())
	}
}
