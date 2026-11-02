package httplocal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
)

// The context-snapshot endpoint tests (the owner's card-11 working
// candidate): the wire surface pins — the coded snapshot view inside the
// `snapshot` wrapper, the attempt resolution (absent = the current
// attempt; explicit positive = that attempt), the typed refusals, and the
// no-bulk-content rule over the wire.

// submitAndSettleOverWire submits the fixture task through the slice and
// settles it through the bounded scheduler, so the execution path compiles
// and stores the attempt's snapshot exactly as production does.
func submitAndSettleOverWire(t *testing.T, env *testEnv, maxOutput int) string {
	t.Helper()
	res, err := env.slice.Submit(snapCallerCtx(env), app.SubmitRequest{
		IdempotencyKey:  "idem-wire-snap-" + time.Now().UTC().Format("150405.000000000"),
		Task:            "Summarize the recorded fixture corpus for the snapshot endpoint.",
		Provider:        "mock",
		Model:           "m",
		MaxOutputTokens: maxOutput,
		Classification:  "test-fixture",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	final, err := app.RunToSettlement(snapCallerCtx(env), env.deps, "worker-wire-snap", 1, res.Job.JobID, 64)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded {
		t.Fatalf("job did not settle successfully: %+v", final)
	}
	return res.Job.JobID
}

func TestJobSnapshotEndpoint(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettleOverWire(t, env, 64)

	// The default read (no attempt parameter) resolves the Job's current
	// attempt's snapshot.
	status, body := env.do(t, http.MethodGet, "/v1/jobs/"+jobID+"/snapshot", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot read must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Snapshot map[string]any `json:"snapshot"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	snap := res.Snapshot
	for _, field := range []string{"schema_version", "snapshot_id", "task_ref", "scope", "created_at",
		"included", "omitted", "budget", "exact_vs_approximate", "estimator_version",
		"renderer_version", "compression_method", "retrieval_config_version", "assembled_digest"} {
		if _, ok := snap[field]; !ok {
			t.Fatalf("the snapshot view must carry the %q field", field)
		}
	}
	if snap["task_ref"] != jobID || snap["scope"] != "local" {
		t.Fatalf("identity fields must ride the record: %v", snap)
	}
	if snap["exact_vs_approximate"] != "core_produced" {
		t.Fatalf("the exact-vs-approximate label must be the pinned value: %v", snap["exact_vs_approximate"])
	}
	if snap["snapshot_id"] != app.SnapshotIDFor(jobID, 1) {
		t.Fatalf("the identity must follow the coded grammar: %v", snap["snapshot_id"])
	}
	budget, _ := snap["budget"].(map[string]any)
	requested, _ := budget["requested"].(map[string]any)
	if requested["reserved_output_tokens"].(float64) != 64 {
		t.Fatalf("the requested reserved output bound must ride the budget pair: %v", budget)
	}
	if budget["estimated_input_tokens"].(float64) <= 0 || budget["estimator_version"] == "" {
		t.Fatalf("the estimate must carry its estimator version: %v", budget)
	}
	for _, absent := range []string{"max_input_tokens", "max_items"} {
		if _, present := requested[absent]; present {
			t.Fatalf("a zero bound must be absent, never zero: %v", budget)
		}
	}

	// The explicit attempt parameter reads the same stored snapshot.
	status, body = env.do(t, http.MethodGet, "/v1/jobs/"+jobID+"/snapshot?attempt=1", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("explicit-attempt read must answer 200, got %d: %s", status, body)
	}
	var explicit struct {
		Snapshot struct {
			SnapshotID string `json:"snapshot_id"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(body, &explicit); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if explicit.Snapshot.SnapshotID != app.SnapshotIDFor(jobID, 1) {
		t.Fatalf("the explicit attempt must resolve the coded grammar: %v", explicit.Snapshot)
	}
}

func TestJobSnapshotEndpointRefusals(t *testing.T) {
	env := newTestEnv(t)
	jobID := submitAndSettleOverWire(t, env, 0)

	cases := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "unauthenticated", path: "/v1/jobs/" + jobID + "/snapshot", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "unknown_job", path: "/v1/jobs/job-does-not-exist/snapshot", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "never_executed_job", path: "/v1/jobs/no-such-job/snapshot?attempt=3", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "malformed_attempt_zero", path: "/v1/jobs/" + jobID + "/snapshot?attempt=0", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "malformed_attempt_negative", path: "/v1/jobs/" + jobID + "/snapshot?attempt=-2", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "malformed_attempt_text", path: "/v1/jobs/" + jobID + "/snapshot?attempt=abc", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, http.MethodGet, tc.path, "", tc.token, nil)
			if status != tc.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.wantStatus, status, body)
			}
			if !strings.Contains(string(body), tc.wantCode) {
				t.Fatalf("%s: the refusal must carry the typed %s code: %s", tc.name, tc.wantCode, body)
			}
		})
	}
}

func TestJobSnapshotEndpointNoBulkContent(t *testing.T) {
	env := newTestEnv(t)
	canary := "WIRE-CANARY the compiled representation embeds this task text verbatim."
	res, err := env.slice.Submit(snapCallerCtx(env), app.SubmitRequest{
		IdempotencyKey: "idem-wire-no-bulk",
		Task:           canary,
		Provider:       "mock",
		Model:          "m",
		Classification: "test-fixture",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := app.RunToSettlement(snapCallerCtx(env), env.deps, "worker-wire-no-bulk", 1, res.Job.JobID, 64); err != nil {
		t.Fatalf("settle: %v", err)
	}

	status, body := env.do(t, http.MethodGet, "/v1/jobs/"+res.Job.JobID+"/snapshot", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot read must answer 200, got %d: %s", status, body)
	}
	// The compiled CONTENT (which embeds the task text) and the provider
	// handle never ride the wire — the view shape excludes them by
	// construction.
	for _, forbidden := range []string{canary, "assembled_representation", "provider_handle"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("the wire must never carry compiled content (%q leaked): %s", forbidden, body)
		}
	}
}

// snapCallerCtx builds the caller context the settlement path runs under
// (the same pattern the journey test uses for its scheduler session).
func snapCallerCtx(env *testEnv) context.Context {
	return app.WithCaller(context.Background(), "wire-snap", env.token)
}
