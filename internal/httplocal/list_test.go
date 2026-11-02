package httplocal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
)

// listJobAt submits one Job directly through the wired relay with an
// explicit creation instant so the Core-defined order is deterministic.
func listJobAt(t *testing.T, e *testEnv, id string, at time.Time) {
	t.Helper()
	if _, err := e.deps.Relay.SubmitJob(context.Background(), aijobrelay.Job{
		JobID: id, IdempotencyKey: "key-" + id, InputDigest: "sha256:" + hexDigest(id),
		State: aijobrelay.JobCreated, Version: 1,
		CorrelationID: "corr-" + id, CreatedAt: at,
	}); err != nil {
		t.Fatalf("relay submit %s: %v", id, err)
	}
}

func hexDigest(s string) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, 0, 64)
	for len(out) < 64 {
		out = append(out, hexChars[(len(out)+len(s))%16])
	}
	return string(out)
}

type wireListResult struct {
	Jobs   []map[string]any `json:"jobs"`
	Cursor string           `json:"cursor"`
}

func decodeList(t *testing.T, body []byte) wireListResult {
	t.Helper()
	var res wireListResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("list decode: %v (%s)", err, body)
	}
	return res
}

// TestListEndpoint covers the GET /v1/jobs surface (the card-9 working
// candidate over the wire): the typed shape (jobs + the opaque cursor),
// the created-at descending order, the cursor round-trip with the final
// page cursorless, and the untouched single-Job reads beside it.
func TestListEndpoint(t *testing.T) {
	env := newTestEnv(t)

	// An empty store lists as an empty jobs array, no cursor.
	status, body := env.do(t, http.MethodGet, "/v1/jobs", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("empty list status = %d (%s)", status, body)
	}
	res := decodeList(t, body)
	if res.Jobs == nil || len(res.Jobs) != 0 {
		t.Fatalf("empty list must be an empty jobs array, got %s", body)
	}
	if res.Cursor != "" {
		t.Fatalf("empty list must carry no cursor: %q", res.Cursor)
	}

	// Filled: three Jobs with distinct creation instants — newest first.
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"job-a", "job-b", "job-c"} {
		listJobAt(t, env, id, base.Add(time.Duration(i)*time.Second))
	}
	status, body = env.do(t, http.MethodGet, "/v1/jobs?limit=2", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("list status = %d (%s)", status, body)
	}
	res = decodeList(t, body)
	if len(res.Jobs) != 2 || res.Jobs[0]["job_id"] != "job-c" || res.Jobs[1]["job_id"] != "job-b" {
		t.Fatalf("page one must be newest first: %s", body)
	}
	if res.Jobs[0]["state"] != aijobrelay.JobQueued || res.Jobs[0]["correlation_id"] == "" {
		t.Fatalf("the list rows carry the coded Job field set: %s", body)
	}
	if res.Cursor == "" {
		t.Fatal("page one must carry the opaque next cursor")
	}

	// The cursor round-trips; the final page carries no cursor.
	page2 := decodeList(t, mustGet(t, env, "/v1/jobs?limit=2&cursor="+res.Cursor))
	if len(page2.Jobs) != 1 || page2.Jobs[0]["job_id"] != "job-a" {
		t.Fatalf("page two must carry the remainder: %s", page2.Jobs)
	}
	if page2.Cursor != "" {
		t.Fatalf("the final page must carry no cursor: %q", page2.Cursor)
	}

	// The single-Job reads beside the list are untouched.
	status, body = env.do(t, http.MethodGet, "/v1/jobs/job-a", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("status read = %d (%s)", status, body)
	}
}

func mustGet(t *testing.T, e *testEnv, path string) []byte {
	t.Helper()
	status, body := e.do(t, http.MethodGet, path, "", e.token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d (%s)", path, status, body)
	}
	return body
}

// TestListDefaultLimit proves the per-surface default: a limitless request
// over more Jobs than the default returns exactly app.DefaultListLimit rows
// plus the continuation cursor.
func TestListDefaultLimit(t *testing.T) {
	env := newTestEnv(t)
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for i := 0; i < app.DefaultListLimit+1; i++ {
		listJobAt(t, env, fmt.Sprintf("job-%03d", i), base.Add(time.Duration(i)*time.Second))
	}
	res := decodeList(t, mustGet(t, env, "/v1/jobs"))
	if len(res.Jobs) != app.DefaultListLimit {
		t.Fatalf("the default limit must bound the page at %d, got %d", app.DefaultListLimit, len(res.Jobs))
	}
	if res.Jobs[0]["job_id"] != fmt.Sprintf("job-%03d", app.DefaultListLimit) {
		t.Fatalf("the newest job must lead: %v", res.Jobs[0]["job_id"])
	}
	if res.Cursor == "" {
		t.Fatal("a default-bounded page must carry the continuation cursor")
	}
	rest := decodeList(t, mustGet(t, env, "/v1/jobs?cursor="+res.Cursor))
	if len(rest.Jobs) != 1 || rest.Cursor != "" {
		t.Fatalf("the remainder must be the final page: %d jobs, cursor %q", len(rest.Jobs), rest.Cursor)
	}
}

// TestListValidationAndAuthz maps the list's failure paths onto the typed
// envelopes: the limit bound class, the closed filter vocabulary, the
// fail-closed cursor decode, and the decision port's denial without a
// session credential.
func TestListValidationAndAuthz(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name  string
		query string
		field string
	}{
		{"limit over the bound class", "?limit=1001", "limit"},
		{"limit not an integer", "?limit=abc", "limit"},
		{"limit non-positive", "?limit=0", "limit"},
		{"limit negative", "?limit=-3", "limit"},
		{"unregistered status", "?status=leased", "status"},
		{"malformed cursor", "?cursor=not-a-cursor", "cursor"},
		{"oversized cursor", "?cursor=" + string(make([]byte, 0)) + oversized(), "cursor"},
	}
	for _, tc := range cases {
		status, body := env.do(t, http.MethodGet, "/v1/jobs"+tc.query, "", env.token, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: status = %d (%s)", tc.name, status, body)
		}
		envl := decodeEnvelope(t, body)
		if envl.Code != "VALIDATION_FAILED" || envl.Data["field"] != tc.field {
			t.Fatalf("%s: envelope = %s", tc.name, body)
		}
	}

	// The registered filter value passes and filters server-side.
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	listJobAt(t, env, "job-f1", base)
	listJobAt(t, env, "job-f2", base.Add(time.Second))
	if err := env.deps.Relay.PauseJob(context.Background(), "job-f2"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	status, body := env.do(t, http.MethodGet, "/v1/jobs?status=paused", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("filtered list = %d (%s)", status, body)
	}
	res := decodeList(t, body)
	if len(res.Jobs) != 1 || res.Jobs[0]["job_id"] != "job-f2" {
		t.Fatalf("the paused filter must match only job-f2: %s", body)
	}

	// Authorization through the decision port: no credential, no list.
	status, body = env.do(t, http.MethodGet, "/v1/jobs", "", "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("unauthenticated list = %d (%s)", status, body)
	}
	envl := decodeEnvelope(t, body)
	if envl.Code != "AUTHZ_DENIED" {
		t.Fatalf("the denial must be the typed AUTHZ_DENIED envelope: %s", body)
	}
}

func oversized() string {
	b := make([]byte, maxListCursorLen+1)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
