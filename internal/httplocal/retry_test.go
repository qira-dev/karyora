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

// The F9 family's wire tests: the retry-eligibility read's four typed
// members inside the retry_eligibility wrapper (IMPL-20) and the M-7
// retry command's surface, typed refusals, and Supplied-keyed replay
// (IMPL-21). The boundary between them stands: the eligibility read is a
// GET that executes nothing — execution exists only on the command's own
// POST path, behind its own ActionDataWrite decision.

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
	if res.RetryEligibility.IdempotencyClass != aijobrelay.RetryIdempotencySuppliedKeyed {
		t.Fatalf("the class must report the implemented Supplied-keyed keying: %+v", res.RetryEligibility)
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

// TestJobRetryCommandEndpoint drives the M-7 retry command over the wire
// (IMPL-21, the F9 family's frozen semantics): the ineligible refusal is
// the typed CONFLICT_STATE; the terminal Failed Job applies through the
// command (a NEW Attempt of the SAME Job — never a new Job); the same
// Supplied-keyed reference re-presented replays (never regenerated); the
// command's surface is POST-only.
func TestJobRetryCommandEndpoint(t *testing.T) {
	env := newTestEnv(t)
	// A queued Job refuses through the same eligibility truth; the typed
	// envelope carries the CONFLICT_STATE.
	queuedID := submitForRetryRead(t, env, "idem-wire-retry-cmd-queued", nil)
	status, body := env.do(t, http.MethodPost, "/v1/jobs/"+queuedID+"/retry", `{"idempotency_reference":"ref-queued"}`, env.token, nil)
	if status != http.StatusConflict || !strings.Contains(string(body), "CONFLICT_STATE") {
		t.Fatalf("an ineligible retry must be the typed 409 CONFLICT_STATE, got %d %s", status, body)
	}

	// The canonical case over the wire: the terminal Failed Job applies.
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
	failedID := submitForRetryRead(t, env2, "idem-wire-retry-cmd-failed", json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`))
	if _, err := app.RunToSettlement(snapCallerCtx(env2), env2.deps, "worker-wire-retry-cmd", 1, failedID, 16); err != nil {
		t.Fatalf("settle: %v", err)
	}
	status, body = env2.do(t, http.MethodPost, "/v1/jobs/"+failedID+"/retry", `{"idempotency_reference":"ref-1"}`, env2.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the allow case must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Job      aijobrelay.Job `json:"job"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if res.Replayed || res.Job.JobID != failedID || res.Job.State != aijobrelay.JobQueued {
		t.Fatalf("the retry must re-queue the SAME Job fresh: %+v (%s)", res, body)
	}
	// The replay over the wire: the same reference returns the recorded
	// outcome without re-applying.
	status, body = env2.do(t, http.MethodPost, "/v1/jobs/"+failedID+"/retry", `{"idempotency_reference":"ref-1"}`, env2.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the replay must answer 200, got %d: %s", status, body)
	}
	var replayed struct {
		Job      aijobrelay.Job `json:"job"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &replayed); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if !replayed.Replayed {
		t.Fatalf("the same reference must replay: %+v (%s)", replayed, body)
	}
}

// TestJobRetryCommandRefusals pins the command's typed refusals over the
// wire: the reference is required (VALIDATION_FAILED); an unknown identity
// is the typed NOT_FOUND (the no-leak rule); the unauthenticated command is
// the 403 AUTHZ_DENIED; the command surface is POST-only (a GET on the
// command path is NOT_FOUND — the state-changing-GET prohibition's
// registration-time discipline).
func TestJobRetryCommandRefusals(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitForRetryRead(t, env, "idem-wire-retry-cmd-refusals", nil)
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "missing_reference", method: http.MethodPost, path: "/v1/jobs/" + jobID + "/retry", body: `{}`, token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "unauthenticated", method: http.MethodPost, path: "/v1/jobs/" + jobID + "/retry", body: `{"idempotency_reference":"ref-1"}`, token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "unknown_job", method: http.MethodPost, path: "/v1/jobs/job-does-not-exist/retry", body: `{"idempotency_reference":"ref-1"}`, token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "method_mismatch", method: http.MethodGet, path: "/v1/jobs/" + jobID + "/retry", body: "", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, tc.body, tc.token, nil)
			if status != tc.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.wantStatus, status, body)
			}
			if !strings.Contains(string(body), tc.wantCode) {
				t.Fatalf("%s: the refusal must carry the typed %s code: %s", tc.name, tc.wantCode, body)
			}
		})
	}
}
