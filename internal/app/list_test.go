package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// listJobAt submits one Job directly through the relay with an explicit
// creation instant so the Core-defined order is deterministic (the app
// submit path stamps the wall clock; the list order tests control it).
func listJobAt(t *testing.T, e *testEnv, id, key string, at time.Time) aijobrelay.Job {
	t.Helper()
	j, err := e.store.SubmitJob(e.ctx(), aijobrelay.Job{
		JobID: id, IdempotencyKey: key, InputDigest: DigestOf([]byte("input-" + id)),
		State: aijobrelay.JobCreated, Version: 1,
		CorrelationID: "corr-" + id, CreatedAt: at,
	})
	if err != nil {
		t.Fatalf("relay submit %s: %v", id, err)
	}
	return *j
}

// TestListOrderAndPagination covers the card-9 working candidate at the app
// surface: the Core-defined created-at descending order, the opaque cursor
// round-trip with no overlap or gap, and the final page's empty cursor.
func TestListOrderAndPagination(t *testing.T) {
	env := newTestEnv(t)
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"job-a", "job-b", "job-c"} {
		listJobAt(t, env, id, "key-"+id, base.Add(time.Duration(i)*time.Second))
	}

	// Empty-limit request: the per-surface default applies and the order is
	// created_at descending.
	res, err := env.slice.List(env.ctx(), ListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Jobs) != 3 || res.Jobs[0].JobID != "job-c" || res.Jobs[2].JobID != "job-a" {
		t.Fatalf("order must be created_at descending: %v", res.Jobs)
	}
	if res.NextCursor != "" {
		t.Fatalf("a complete page must carry no next cursor: %q", res.NextCursor)
	}

	// The cursor round-trip: page one of two, then page two — no overlap,
	// no gap, and the union is the whole set in order.
	page1, err := env.slice.List(env.ctx(), ListRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Jobs) != 2 || page1.NextCursor == "" {
		t.Fatalf("page one: %v cursor %q", page1.Jobs, page1.NextCursor)
	}
	if strings.ContainsAny(page1.NextCursor, "{}") {
		t.Fatal("the cursor must be opaque — never structured data")
	}
	page2, err := env.slice.List(env.ctx(), ListRequest{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Jobs) != 1 || page2.Jobs[0].JobID != "job-a" {
		t.Fatalf("page two must carry the remainder: %v", page2.Jobs)
	}
	if page2.NextCursor != "" {
		t.Fatalf("the final page must carry no cursor: %q", page2.NextCursor)
	}
	if page1.Jobs[0].JobID != "job-c" || page1.Jobs[1].JobID != "job-b" {
		t.Fatalf("page one order: %v", page1.Jobs)
	}
}

// TestListValidation maps every request-shape refusal to the typed D-35
// envelope: the limit bound class, the closed filter vocabulary, and the
// fail-closed cursor decode.
func TestListValidation(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name  string
		req   ListRequest
		field string
	}{
		{"negative limit", ListRequest{Limit: -1}, "limit"},
		{"over the bound class", ListRequest{Limit: MaxListLimit + 1}, "limit"},
		{"unregistered status", ListRequest{Status: "leased"}, "status"},
		{"malformed cursor", ListRequest{Cursor: "not-a-cursor"}, "cursor"},
		{"empty cursor payload", ListRequest{Cursor: "ab.7b7d"}, "cursor"},
	}
	for _, tc := range cases {
		_, err := env.slice.List(env.ctx(), tc.req)
		var ee *EnvelopeError
		if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: expected a typed VALIDATION_FAILED envelope, got %v", tc.name, err)
		}
		if ee.Envelope.Data["field"] != tc.field {
			t.Fatalf("%s: expected field %q in data, got %v", tc.name, tc.field, ee.Envelope.Data["field"])
		}
	}

	// The bound class edge: the replay read's 1–1000 class applies.
	if _, err := env.slice.List(env.ctx(), ListRequest{Limit: MaxListLimit}); err != nil {
		t.Fatalf("limit %d must be accepted: %v", MaxListLimit, err)
	}
	// A registered filter value passes.
	if _, err := env.slice.List(env.ctx(), ListRequest{Status: aijobrelay.JobQueued}); err != nil {
		t.Fatalf("a registered status must be accepted: %v", err)
	}
}

// TestListFilter selects only the filtered state's Jobs — filter options
// come from the Core's own registry, never invented client-side.
func TestListFilter(t *testing.T) {
	env := newTestEnv(t)
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	listJobAt(t, env, "job-q1", "key-q1", base)
	listJobAt(t, env, "job-q2", "key-q2", base.Add(time.Second))
	listJobAt(t, env, "job-c1", "key-c1", base.Add(2*time.Second))
	// queued → paused is an accepted transition, giving the filter a second
	// state to distinguish.
	if err := env.deps.Relay.PauseJob(env.ctx(), "job-q2"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	queued, err := env.slice.List(env.ctx(), ListRequest{Status: aijobrelay.JobQueued})
	if err != nil {
		t.Fatal(err)
	}
	if len(queued.Jobs) != 2 {
		t.Fatalf("queued filter should match job-q1 and job-c1, got %v", queued.Jobs)
	}
	for _, j := range queued.Jobs {
		if j.State != aijobrelay.JobQueued {
			t.Fatalf("filter returned a non-queued job: %s in %s", j.JobID, j.State)
		}
	}
	paused, err := env.slice.List(env.ctx(), ListRequest{Status: aijobrelay.JobPaused})
	if err != nil {
		t.Fatal(err)
	}
	if len(paused.Jobs) != 1 || paused.Jobs[0].JobID != "job-q2" {
		t.Fatalf("paused filter should match only job-q2, got %v", paused.Jobs)
	}
}

// TestListAuthorization applies the decision port: a request without a
// valid session credential is the typed AUTHZ_DENIED envelope — the list is
// authorized exactly like every other read.
func TestListAuthorization(t *testing.T) {
	env := newTestEnv(t)
	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	_, err := env.slice.List(denied, ListRequest{})
	var ee *EnvelopeError
	if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeAuthzDenied {
		t.Fatalf("expected the typed AUTHZ_DENIED envelope, got %v", err)
	}
	if _, err := env.slice.List(env.ctx(), ListRequest{}); err != nil {
		t.Fatalf("a data-scoped session must read the list: %v", err)
	}
}
