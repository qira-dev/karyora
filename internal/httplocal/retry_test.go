package httplocal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// The retry-eligibility endpoint tests (the F9 family's frozen contract
// over the wire): the four typed members inside the retry_eligibility
// wrapper, the typed refusals, and the read/command boundary pin — the
// read is NOT the retry command; no retry command route exists at this
// HEAD (the M-7 command's identity stays its recorded Open item).

// submitForRetryRead submits the fixture task through the slice for the
// retry-eligibility endpoint tests.
func submitForRetryRead(t *testing.T, env *testEnv, idem string, schema json.RawMessage) string {
	t.Helper()
	req := app.SubmitRequest{
		IdempotencyKey:  idem,
		Task:            "Summarize the recorded fixture corpus for the retry-eligibility read.",
		Provider:        "mock",
		Model:           "m",
		MaxOutputTokens: 16,
		Classification:  "test-fixture",
		Schema:          schema,
	}
	res, err := env.slice.Submit(snapCallerCtx(env), req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return res.Job.JobID
}

func TestJobRetryEligibilityEndpoint(t *testing.T) {
	env := newTestEnv(t)

	// A queued Job denies over the wire, with the exact four members.
	jobID := submitForRetryRead(t, env, "idem-wire-retry-queued", nil)
	status, body := env.do(t, http.MethodGet, "/v1/jobs/"+jobID+"/retry-eligibility", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the read must answer 200, got %d: %s", status, body)
	}
	var res struct {
		RetryEligibility aijobrelay.RetryEligibility `json:"retry_eligibility"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if res.RetryEligibility.Eligible {
		t.Fatalf("a queued job must deny: %+v", res.RetryEligibility)
	}
	if res.RetryEligibility.ReconciliationRequired {
		t.Fatalf("a fresh queued job reports no reconciliation requirement: %+v", res.RetryEligibility)
	}
	if res.RetryEligibility.IdempotencyClass != aijobrelay.RetryIdempotencyOpen {
		t.Fatalf("the class must be the coded `open`: %+v", res.RetryEligibility)
	}
	for _, term := range []string{"new attempt of the same job", "at-least-once attempts", "needs reconciliation"} {
		if !strings.Contains(res.RetryEligibility.RiskStatement, term) {
			t.Fatalf("the exact term %q must ride the statement: %q", term, res.RetryEligibility.RiskStatement)
		}
	}
	if strings.Contains(strings.ToLower(res.RetryEligibility.RiskStatement), "exactly-once") {
		t.Fatalf("no exactly-once wording anywhere: %q", res.RetryEligibility.RiskStatement)
	}

	// A terminal failure allows over the wire (the real-listener drive:
	// the schema-invalid executor fails the job terminally exactly as
	// production does).
	env2 := newTestEnvComposed(t, func(deps *app.SliceDeps) {
		invalid := providers.NewMockAdapter(providers.MockBehavior{InvalidStructuredJSON: true})
		reg := dprovider.NewRegistry(nil)
		reg.Upsert(invalid.SeedDataHandling(invalid.DefaultProfile("m")))
		exec, err := providers.NewExecutor(invalid, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
		if err != nil {
			t.Fatalf("provider executor: %v", err)
		}
		deps.Providers[dprovider.ProviderMock] = exec
	})
	failedID := submitForRetryRead(t, env2, "idem-wire-retry-failed", json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`))
	final, err := app.RunToSettlement(snapCallerCtx(env2), env2.deps, "worker-wire-retry", 1, failedID, 16)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if final.State != aijobrelay.JobFailed {
		t.Fatalf("the fixture job must settle Failed, got %s", final.State)
	}
	status, body = env2.do(t, http.MethodGet, "/v1/jobs/"+failedID+"/retry-eligibility", "", env2.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the allow case must answer 200, got %d: %s", status, body)
	}
	var allow struct {
		RetryEligibility aijobrelay.RetryEligibility `json:"retry_eligibility"`
	}
	if err := json.Unmarshal(body, &allow); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if !allow.RetryEligibility.Eligible {
		t.Fatalf("a terminal Failed job is the canonical retry case: %+v", allow.RetryEligibility)
	}
	if allow.RetryEligibility.ReconciliationRequired {
		t.Fatalf("no Unknown side effect stands on this failed job: %+v", allow.RetryEligibility)
	}
}

func TestJobRetryEligibilityEndpointRefusals(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitForRetryRead(t, env, "idem-wire-retry-refusals", nil)
	cases := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "unauthenticated", method: http.MethodGet, path: "/v1/jobs/" + jobID + "/retry-eligibility", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "unknown_job", method: http.MethodGet, path: "/v1/jobs/job-does-not-exist/retry-eligibility", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "method_mismatch", method: http.MethodPost, path: "/v1/jobs/" + jobID + "/retry-eligibility", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
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
}

// TestJobRetryEligibilityNoCommandSurface pins the read/command boundary
// over the wire: the eligibility read is NOT the retry command — no
// retry command route exists at this HEAD, and the read's allow verdict
// authorizes nothing (the M-7 command's identity stays its recorded Open
// item; the S7 §5 gap 2).
func TestJobRetryEligibilityNoCommandSurface(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitForRetryRead(t, env, "idem-wire-retry-nocommand", nil)
	for _, shape := range []string{"/retry", "/retry/attempt"} {
		status, body := env.do(t, http.MethodPost, "/v1/jobs/"+jobID+shape, "{}", env.token, nil)
		if status != http.StatusNotFound || !strings.Contains(string(body), "NOT_FOUND") {
			t.Fatalf("no retry command route may exist (POST %s): got %d %s", shape, status, body)
		}
	}
}
