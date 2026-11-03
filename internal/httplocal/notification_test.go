package httplocal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/notification"
)

// The notification surface's wire register (the adopted S6 supply,
// P-S6-01/02): the endpoints end to end over the live loopback test server,
// the typed refusals, and the route register pin that proves NO
// OS-delivery, OS-permission, quiet-hours, mark-all-read, or stream
// endpoint exists (decision 6: in-app ON + OS OFF by default — no OS
// delivery code anywhere in this milestone; the UI-34 FINAL freeze stays
// its own Checkpoint-C act; the stream transport stays the UI-26/C-6
// gate's, P-S6-04; the P-S6-02 named mutation set is exactly mark
// read/unread/filter/clear — "filter" is the list read's query parameters
// over the closed registers, never an endpoint).

// notifWireJob drives one Job to a notification-relevant outcome through
// the real composition (the relay port's submit/claim/commit — the same
// path the durable worker takes), so the backend-originated producer
// notification exists for the wire tests to read. The fixture Job carries
// its own provider id so the FIFO claim order never drifts across the
// test's job set.
func notifWireJob(t *testing.T, e *testEnv, id, toState string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.deps.Relay.SubmitJob(ctx, aijobrelay.Job{
		JobID: id, IdempotencyKey: "key-" + id, InputDigest: "sha256:" + hexDigest(id),
		State: aijobrelay.JobCreated, Version: 1,
		Requirements:  aijobrelay.Requirements{ProviderID: "prov-" + id},
		CorrelationID: "corr-" + id, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("relay submit %s: %v", id, err)
	}
	claim, ok, err := e.deps.Relay.ClaimNext(ctx,
		aijobrelay.WorkerCapability{WorkerID: "w-notif-wire", ProviderIDs: []string{"prov-" + id}, MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim %s: %v %v", id, ok, err)
	}
	commit := aijobrelay.Commit{
		JobID: id, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w-notif-wire", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: toState, CorrelationID: "corr-" + id,
	}
	switch toState {
	case aijobrelay.JobSucceeded:
		commit.AttemptOutcome = aijobrelay.AttemptSucceeded
		commit.ResultDigest = "sha256:" + hexDigest("result "+id)
		commit.ReceiptID = "rc-" + id
	case aijobrelay.JobWaiting:
		commit.WaitingReason = string(aijobrelay.WaitApproval)
		commit.AttemptOutcome = aijobrelay.AttemptRunning
	default:
		t.Fatalf("unsupported wire outcome %q", toState)
	}
	if err := e.deps.Relay.CommitAttempt(ctx, commit); err != nil {
		t.Fatalf("commit %s → %s: %v", id, toState, err)
	}
}

// wireNotificationID reads the newest group entry's representative
// notification id through the wire list (no raw SQL — the surface's own
// read).
func wireNotificationID(t *testing.T, e *testEnv, objectID string) string {
	t.Helper()
	body := mustGet(t, e, "/v1/notifications?limit=1000")
	var res struct {
		Entries []struct {
			NotificationID  string `json:"notification_id"`
			RelatedObjectID string `json:"related_object_id"`
		} `json:"entries"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("list decode: %v (%s)", err, body)
	}
	for _, e := range res.Entries {
		if e.RelatedObjectID == objectID {
			return e.NotificationID
		}
	}
	t.Fatalf("no notification group for object %q in %s", objectID, body)
	return ""
}

// TestNotificationRouteRegister pins the exact notification route set: five
// endpoints, and no OS-delivery, OS-permission, quiet-hours, mark-all-read,
// or stream route — nothing of the OS channel or the stream plane was
// invented (the route table is the pin's authority).
func TestNotificationRouteRegister(t *testing.T) {
	var got []string
	for _, rt := range routes() {
		if strings.HasPrefix(rt.prefix, "/v1/notifications") {
			got = append(got, rt.method+" "+rt.prefix+rt.suffix)
		}
	}
	want := []string{
		"GET /v1/notifications/counts",
		"GET /v1/notifications",
		"POST /v1/notifications/read",
		"POST /v1/notifications/unread",
		"POST /v1/notifications/clear",
	}
	if len(got) != len(want) {
		t.Fatalf("the notification route register carries %d routes, want %d: %v", len(got), len(want), got)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("route %q missing from the register: %v", w, got)
		}
	}
	for _, g := range got {
		if strings.Contains(g, "stream") || strings.Contains(g, "quiet") ||
			strings.Contains(g, "desktop") || strings.Contains(g, "permission") ||
			strings.Contains(g, "read-all") || strings.Contains(g, "os-") {
			t.Fatalf("no OS-delivery/OS-permission/quiet-hours/mark-all/stream endpoint may exist: %q", g)
		}
	}
}

// TestNotificationEndpoints drives the surface end to end over the wire:
// the honest empty page, the backend-originated producer notifications
// (driven through the real composition), the grouped list read with its
// four-way payload fields and the current canonical state, the
// Core-supplied unread counts, and the O-6 mutation set's typed outcomes
// (mark read → mark unread → clear) with their list/counts effects.
func TestNotificationEndpoints(t *testing.T) {
	env := newTestEnv(t)

	// An empty store lists as an empty entries array with no cursor — the
	// honest no-notification answer, never an error shape that invents one
	// — and the counts read zero from Core-supplied state only.
	status, body := env.do(t, http.MethodGet, "/v1/notifications", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("empty list: %d %s", status, body)
	}
	if !strings.Contains(string(body), `"entries":[]`) || strings.Contains(string(body), `"cursor"`) {
		t.Fatalf("the empty page must be an entries array with no cursor: %s", body)
	}
	status, body = env.do(t, http.MethodGet, "/v1/notifications/counts", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("empty counts: %d %s", status, body)
	}
	var counts struct {
		Total      int            `json:"total"`
		ByCategory map[string]int `json:"by_category"`
	}
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatalf("counts decode: %v (%s)", err, body)
	}
	if counts.Total != 0 || len(counts.ByCategory) != 0 {
		t.Fatalf("the empty counts = %+v, want zero", counts)
	}

	// Two backend-originated producer outcomes through the real
	// composition (a succeeded result and an approval-needed wait).
	notifWireJob(t, env, "job-wire-1", aijobrelay.JobSucceeded)
	time.Sleep(2 * time.Millisecond)
	notifWireJob(t, env, "job-wire-2", aijobrelay.JobWaiting)
	time.Sleep(2 * time.Millisecond)

	// The grouped list read: newest first, the four-way payload fields, the
	// current canonical state, the cursorless final page.
	body = mustGet(t, env, "/v1/notifications?limit=1000")
	var list struct {
		Entries []notification.GroupEntry `json:"entries"`
		Cursor  string                    `json:"cursor"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list decode: %v (%s)", err, body)
	}
	if len(list.Entries) != 2 || list.Cursor != "" {
		t.Fatalf("entries = %d cursor = %q, want 2 entries and a cursorless final page: %s", len(list.Entries), list.Cursor, body)
	}
	if list.Entries[0].RelatedObjectID != "job-wire-2" || list.Entries[1].RelatedObjectID != "job-wire-1" {
		t.Fatalf("the list must be newest first: %+v", list.Entries)
	}
	for _, entry := range list.Entries {
		if !notification.ValidPresentationState(entry.Presentation) {
			t.Fatalf("entry %q carries an unregistered presentation state %q", entry.NotificationID, entry.Presentation)
		}
		if entry.Presentation == notification.PresentPermissionRevoked {
			t.Fatal("permission_revoked must never be emitted in the single-user Local mode")
		}
		if entry.Category != notification.CategoryJob {
			t.Fatalf("the entry's category = %q, want the one coded producer category", entry.Category)
		}
	}
	if list.Entries[0].ObjectState != aijobrelay.JobWaiting || list.Entries[0].StateClass != notification.ClassApprovalNeeded {
		t.Fatalf("the approval-needed entry = %+v", list.Entries[0])
	}
	if list.Entries[1].ObjectState != aijobrelay.JobSucceeded || list.Entries[1].StateClass != notification.ClassResult {
		t.Fatalf("the result entry = %+v", list.Entries[1])
	}

	// The Core-supplied unread counts: total and per category, only from
	// the stored read state.
	body = mustGet(t, env, "/v1/notifications/counts")
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatalf("counts decode: %v (%s)", err, body)
	}
	if counts.Total != 2 || counts.ByCategory[string(notification.CategoryJob)] != 2 {
		t.Fatalf("the unread counts = %+v, want total 2 / job 2", counts)
	}

	// The O-6 mutations over the newest group's representative: mark read
	// (the counts drop) → the read_state filter narrows server-side → mark
	// unread (the stamp clears, the counts restore) → clear (the row is
	// excluded from the list and the counts, retained in the store).
	n2 := wireNotificationID(t, env, "job-wire-2")
	status, body = env.do(t, http.MethodPost, "/v1/notifications/read", `{"notification_ids":["`+n2+`"]}`, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("mark read: %d %s", status, body)
	}
	var mutated struct {
		Updated   int `json:"updated"`
		Unchanged int `json:"unchanged"`
	}
	if err := json.Unmarshal(body, &mutated); err != nil {
		t.Fatalf("mark-read decode: %v (%s)", err, body)
	}
	if mutated.Updated != 1 || mutated.Unchanged != 0 {
		t.Fatalf("the mark-read outcome = %+v, want updated 1", mutated)
	}
	body = mustGet(t, env, "/v1/notifications/counts")
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("the unread total after the mark-read = %d, want 1 (Core-supplied state only)", counts.Total)
	}
	body = mustGet(t, env, "/v1/notifications?read_state=read")
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 1 || list.Entries[0].RelatedObjectID != "job-wire-2" {
		t.Fatalf("the read_state filter must narrow server-side: %s", body)
	}
	status, body = env.do(t, http.MethodPost, "/v1/notifications/unread", `{"notification_ids":["`+n2+`"]}`, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("mark unread: %d %s", status, body)
	}
	body = mustGet(t, env, "/v1/notifications/counts")
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 {
		t.Fatalf("the unread total after the mark-unread = %d, want 2", counts.Total)
	}
	status, body = env.do(t, http.MethodPost, "/v1/notifications/clear", `{"notification_ids":["`+n2+`"]}`, env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("clear: %d %s", status, body)
	}
	body = mustGet(t, env, "/v1/notifications?limit=1000")
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 1 || list.Entries[0].RelatedObjectID != "job-wire-1" {
		t.Fatalf("the cleared row must be excluded from the list: %s", body)
	}
	body = mustGet(t, env, "/v1/notifications/counts")
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("the cleared row must be excluded from the counts: %+v", counts)
	}
}

// TestNotificationWireRefusals pins the typed wire refusals: the
// unauthenticated calls → 403 AUTHZ_DENIED through the decision port; the
// malformed query values and bodies → 400 VALIDATION_FAILED (the closed
// registers refuse unknown filter values); the unknown notification id →
// 404 NOT_FOUND writing nothing; and the OS-delivery, OS-permission,
// quiet-hours, mark-all-read, and stream-shaped paths are NO endpoint at
// all (404 — none of them exists anywhere).
func TestNotificationWireRefusals(t *testing.T) {
	env := newTestEnv(t)
	notifWireJob(t, env, "job-ref-1", aijobrelay.JobSucceeded)
	n1 := wireNotificationID(t, env, "job-ref-1")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
		want   int
		code   string
	}{
		{"unauthenticated list", http.MethodGet, "/v1/notifications", "", "", 403, "AUTHZ_DENIED"},
		{"unauthenticated counts", http.MethodGet, "/v1/notifications/counts", "", "", 403, "AUTHZ_DENIED"},
		{"unauthenticated mark read", http.MethodPost, "/v1/notifications/read", `{"notification_ids":["` + n1 + `"]}`, "", 403, "AUTHZ_DENIED"},
		{"unauthenticated mark unread", http.MethodPost, "/v1/notifications/unread", `{"notification_ids":["` + n1 + `"]}`, "", 403, "AUTHZ_DENIED"},
		{"unauthenticated clear", http.MethodPost, "/v1/notifications/clear", `{"notification_ids":["` + n1 + `"]}`, "", 403, "AUTHZ_DENIED"},
		{"malformed limit", http.MethodGet, "/v1/notifications?limit=zero", "", env.token, 400, "VALIDATION_FAILED"},
		{"malformed cursor", http.MethodGet, "/v1/notifications?cursor=garbage-cursor", "", env.token, 400, "VALIDATION_FAILED"},
		{"unregistered category", http.MethodGet, "/v1/notifications?category=knowledge", "", env.token, 400, "VALIDATION_FAILED"},
		{"unregistered read state", http.MethodGet, "/v1/notifications?read_state=seen", "", env.token, 400, "VALIDATION_FAILED"},
		{"malformed body", http.MethodPost, "/v1/notifications/read", `{not json`, env.token, 400, "VALIDATION_FAILED"},
		{"empty id set", http.MethodPost, "/v1/notifications/read", `{"notification_ids":[]}`, env.token, 400, "VALIDATION_FAILED"},
		{"unknown id mark read", http.MethodPost, "/v1/notifications/read", `{"notification_ids":["ntf-does-not-exist"]}`, env.token, 404, "NOT_FOUND"},
		{"unknown id clear", http.MethodPost, "/v1/notifications/clear", `{"notification_ids":["ntf-does-not-exist"]}`, env.token, 404, "NOT_FOUND"},
		{"no OS delivery endpoint", http.MethodGet, "/v1/notifications/os-delivery", "", env.token, 404, "NOT_FOUND"},
		{"no OS permission endpoint", http.MethodPost, "/v1/notifications/os-permission", `{}`, env.token, 404, "NOT_FOUND"},
		{"no quiet-hours endpoint", http.MethodGet, "/v1/notifications/quiet-hours", "", env.token, 404, "NOT_FOUND"},
		{"no mark-all-read endpoint", http.MethodPost, "/v1/notifications/read-all", `{}`, env.token, 404, "NOT_FOUND"},
		{"no stream endpoint", http.MethodGet, "/v1/notifications/stream", "", env.token, 404, "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, tc.body, tc.token, nil)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %s", status, tc.want, body)
			}
			if !strings.Contains(string(body), tc.code) {
				t.Fatalf("body must carry %s: %s", tc.code, body)
			}
		})
	}

	// The refused unknown-id mutation wrote nothing: the stored unread
	// state is untouched (the all-or-nothing law at the wire).
	body := mustGet(t, env, "/v1/notifications/counts")
	var counts struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("the refused mutations changed the unread state: %s", body)
	}
}
