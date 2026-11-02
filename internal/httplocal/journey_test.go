package httplocal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/security"
)

// The IMPL-3 live verification: the delivered browser-transport surface
// driven as ONE full journey over the real loopback HTTP listener, over the
// real composition (the app test harness's real SQLite store, the real
// session service, LocalPolicy, the Guard, and the audit sink — the same
// pattern server_test.go uses). Issuance stays the trusted-local Go method
// exactly as delivered; the exchange, the probe, the operations, the
// replay, and every refusal below run over real wire requests.

// doWire performs one wire request against the live loopback listener with
// full control over the request surface (the journey-level form of do: the
// Host header, the Origin, and the URL query are the caller's), returning
// the status, the response headers, and the body.
func (e *testEnv) doWire(t *testing.T, method, path, body, token string, set func(*http.Request)) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, e.ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("wire request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if set != nil {
		set(req)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("wire do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("wire read: %v", err)
	}
	return resp.StatusCode, resp.Header, payload
}

// pollStatusTerminal polls Status over the wire until the Job reaches a
// terminal state (the transport consumer's own loop shape), bounded.
func (e *testEnv) pollStatusTerminal(t *testing.T, token, jobID string) aijobrelay.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last aijobrelay.Job
	for time.Now().Before(deadline) {
		status, body := e.do(t, http.MethodGet, "/v1/jobs/"+jobID, "", token, nil)
		if status != http.StatusOK {
			t.Fatalf("status poll = %d (%s)", status, body)
		}
		var st struct {
			Job     aijobrelay.Job     `json:"job"`
			Attempt aijobrelay.Attempt `json:"attempt"`
		}
		if err := json.Unmarshal(body, &st); err != nil {
			t.Fatalf("status poll decode: %v (%s)", err, body)
		}
		last = st.Job
		if aijobrelay.TerminalJob(st.Job.State) {
			return st.Job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status poll never reached a terminal state, last %+v", last)
	return last
}

// TestLiveEndToEndJourney is the live end-to-end verification (IMPL-3): an
// in-process-issued bootstrap credential exchanges over the real listener
// for a session; the probe read authorizes; a real Job is submitted through
// the wire, settled through the bounded scheduler, and observed back
// through Status, Events, and Result; a fresh Job exercises cooperative
// cancellation; the replay read round-trips its opaque cursor; a replayed
// bootstrap credential is refused AND the session it first minted is
// revoked (proven on the wire through the decision port).
func TestLiveEndToEndJourney(t *testing.T) {
	env := newTestEnv(t)

	// --- Phase 1: the C-1 exchange over the real listener ------------------
	// The credential is issued in-process through the trusted-local channel
	// (the Go method — no issuance path exists over HTTP and none is built
	// here); what the wire consumes is the one-time secret string.
	cred, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	exchangeBody := fmt.Sprintf(`{"credential":%q,"scopes":["data","lifecycle"]}`, cred.Secret)
	status, _, body := env.doWire(t, http.MethodPost, "/v1/session/exchange", exchangeBody, "", nil)
	if status != http.StatusOK {
		t.Fatalf("exchange = %d (%s)", status, body)
	}
	var sess sessionResult
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("exchange decode: %v (%s)", err, body)
	}
	if !strings.HasPrefix(sess.Token, "kry1-") || len(sess.ID) != 16 {
		t.Fatalf("session shape = %+v", sess)
	}
	if len(sess.Scopes) != 2 || sess.Scopes[0] != string(security.ScopeData) || sess.Scopes[1] != string(security.ScopeLifecycle) {
		t.Fatalf("session scopes = %v", sess.Scopes)
	}
	if sess.Expires.IsZero() {
		t.Fatal("session must carry its absolute expiry")
	}
	token := sess.Token

	// --- Phase 2: the probe read authorizes the minted session -------------
	status, _, body = env.doWire(t, http.MethodGet, "/v1/events/replay?stream=job&limit=1", "", token, nil)
	if status != http.StatusOK {
		t.Fatalf("probe = %d (%s)", status, body)
	}
	var probe replayResult
	if err := json.Unmarshal(body, &probe); err != nil {
		t.Fatalf("probe decode: %v (%s)", err, body)
	}
	if probe.Outcome != events.ReplayOK {
		t.Fatalf("probe outcome = %s (%s)", probe.Outcome, body)
	}
	// The same probe with no credential resolves to the coded denial — the
	// transport answered, the decision port refused (never stranded).
	status, _, body = env.doWire(t, http.MethodGet, "/v1/events/replay?stream=job&limit=1", "", "", nil)
	if status != http.StatusOK {
		t.Fatalf("unauthenticated probe = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Outcome != events.ReplayForbidden {
		t.Fatalf("unauthenticated probe outcome = %+v (%s)", probe, body)
	}

	// --- Phase 3: Submit through the wire; settle through the bounded
	// scheduler the CLI foreground session runs; poll Status to terminal ---
	status, _, body = env.doWire(t, http.MethodPost, "/v1/jobs", submitBody("journey-live-1", "Summarize the live journey corpus."), token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d (%s)", status, body)
	}
	var sub struct {
		Job      aijobrelay.Job `json:"job"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &sub); err != nil {
		t.Fatalf("submit decode: %v (%s)", err, body)
	}
	if sub.Job.JobID == "" || sub.Replayed || sub.Job.State != aijobrelay.JobQueued {
		t.Fatalf("submit must report a fresh queued Job, got %+v", sub)
	}
	done := make(chan aijobrelay.Job, 1)
	go func() {
		job, _ := app.RunToSettlement(app.WithCaller(context.Background(), "journey", token), env.deps, "worker-journey", 1, sub.Job.JobID, 64)
		done <- job
	}()
	polled := env.pollStatusTerminal(t, token, sub.Job.JobID)
	final := <-done
	if polled.JobID != sub.Job.JobID || polled.State != aijobrelay.JobSucceeded || final.State != aijobrelay.JobSucceeded {
		t.Fatalf("the polled Job must settle Succeeded: polled %+v, settled %+v", polled, final)
	}
	if polled.ResultDigest == "" || polled.ReceiptID == "" {
		t.Fatalf("the terminal Job must carry its result digest and receipt, got %+v", polled)
	}
	if final.Version <= sub.Job.Version {
		t.Fatalf("the durable version must have advanced: %d <= %d", final.Version, sub.Job.Version)
	}

	// --- Phase 4: Events — the coded record set incl. occurred_at -----------
	status, _, body = env.doWire(t, http.MethodGet, "/v1/jobs/"+sub.Job.JobID+"/events", "", token, nil)
	if status != http.StatusOK {
		t.Fatalf("events = %d (%s)", status, body)
	}
	var ev struct {
		Events []events.Record `json:"events"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatalf("events decode: %v (%s)", err, body)
	}
	if len(ev.Events) < 2 {
		t.Fatalf("the settled Job must carry its lifecycle events, got %s", body)
	}
	if ev.Events[0].EventType != "job.submitted" {
		t.Fatalf("first event = %s", ev.Events[0].EventType)
	}
	var sawSucceeded bool
	for _, rec := range ev.Events {
		if rec.EventType == "job.succeeded" {
			sawSucceeded = true
		}
		if rec.EventID == "" || rec.AggregateType == "" || rec.AggregateID != sub.Job.JobID || rec.Sequence == 0 {
			t.Fatalf("event record identity incomplete: %+v", rec)
		}
		if rec.SchemaVersion == 0 || rec.OccurredAt.IsZero() || rec.Producer == "" {
			t.Fatalf("event record must carry schema_version/occurred_at/producer: %+v", rec)
		}
	}
	if !sawSucceeded {
		t.Fatalf("the lifecycle must include the terminal event, got %+v", ev.Events)
	}

	// --- Phase 5: Result — the stored CAS bytes over the wire ----------------
	status, hdr, body := env.doWire(t, http.MethodGet, "/v1/jobs/"+sub.Job.JobID+"/result", "", token, nil)
	if status != http.StatusOK {
		t.Fatalf("result = %d (%s)", status, body)
	}
	if ct := hdr.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("result content type = %q", ct)
	}
	if !json.Valid(body) || len(body) == 0 {
		t.Fatalf("result payload must be the stored JSON result, got %s", body)
	}

	// --- Phase 6: Cancel behavior on a fresh Job ------------------------------
	slow := &slowExecutor{inner: env.deps.Providers[dprovider.ProviderMock], delay: 300 * time.Millisecond}
	env.deps.Providers[dprovider.ProviderMock] = slow
	status, _, body = env.doWire(t, http.MethodPost, "/v1/jobs", submitBody("journey-live-cancel", "Long journey task."), token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("cancel-case submit = %d (%s)", status, body)
	}
	var csub struct {
		Job aijobrelay.Job `json:"job"`
	}
	if err := json.Unmarshal(body, &csub); err != nil {
		t.Fatalf("cancel-case decode: %v (%s)", err, body)
	}
	cancelDone := make(chan aijobrelay.Job, 1)
	go func() {
		job, _ := app.RunToSettlement(app.WithCaller(context.Background(), "journey", token), env.deps, "worker-journey", 1, csub.Job.JobID, 128)
		cancelDone <- job
	}()
	time.Sleep(60 * time.Millisecond) // the attempt is in flight
	status, _, body = env.doWire(t, http.MethodPost, "/v1/jobs/"+csub.Job.JobID+"/cancel", "", token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("cancel in flight = %d (%s)", status, body)
	}
	cancelled := awaitSettlement(t, env, csub.Job.JobID, cancelDone)
	if cancelled.State != aijobrelay.JobCancelled {
		t.Fatalf("the cancelled Job must settle Cancelled, got %s", cancelled.State)
	}
	// A terminal Job accepts no further transitions: the coded conflict.
	status, _, body = env.doWire(t, http.MethodPost, "/v1/jobs/"+csub.Job.JobID+"/cancel", "", token, nil)
	if status != http.StatusConflict {
		t.Fatalf("cancel of terminal job = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Code; got != string(errs.CodeConflictState) {
		t.Fatalf("cancel of terminal job code = %s", got)
	}

	// --- Phase 7: the replay read with the cursor round-trip -----------------
	status, _, body = env.doWire(t, http.MethodGet, "/v1/events/replay?stream=job&limit=2", "", token, nil)
	if status != http.StatusOK {
		t.Fatalf("replay = %d (%s)", status, body)
	}
	var rep replayResult
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("replay decode: %v (%s)", err, body)
	}
	if rep.Outcome != events.ReplayOK || len(rep.Events) == 0 || rep.Cursor == "" {
		t.Fatalf("replay result = outcome %s, %d events, cursor %q", rep.Outcome, len(rep.Events), rep.Cursor)
	}
	if strings.ContainsAny(rep.Cursor, "{}") {
		t.Fatalf("the cursor must stay opaque on the wire: %q", rep.Cursor)
	}
	// The cursor round-trips: the next read resolves after the position.
	status, _, body = env.doWire(t, http.MethodGet, "/v1/events/replay?stream=job&limit=2&cursor="+url.QueryEscape(rep.Cursor), "", token, nil)
	if status != http.StatusOK {
		t.Fatalf("cursor replay = %d (%s)", status, body)
	}
	var rep2 replayResult
	if err := json.Unmarshal(body, &rep2); err != nil {
		t.Fatalf("cursor replay decode: %v", err)
	}
	if rep2.Outcome != events.ReplayOK {
		t.Fatalf("cursor replay outcome = %s (%s)", rep2.Outcome, body)
	}

	// --- Phase 8: the replay-reject-and-invalidate rule, live ----------------
	// A second credential exchanges exactly once; the REPLAYED credential is
	// refused, AND the session the first exchange minted is revoked — proven
	// on the wire: the revoked token's probe read resolves to the coded
	// forbidden denial through the decision port.
	cred2, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue second: %v", err)
	}
	exchange2 := fmt.Sprintf(`{"credential":%q,"scopes":["data"]}`, cred2.Secret)
	status, _, body = env.doWire(t, http.MethodPost, "/v1/session/exchange", exchange2, "", nil)
	if status != http.StatusOK {
		t.Fatalf("second exchange = %d (%s)", status, body)
	}
	var sess2 sessionResult
	if err := json.Unmarshal(body, &sess2); err != nil {
		t.Fatalf("second exchange decode: %v (%s)", err, body)
	}
	status, _, body = env.doWire(t, http.MethodPost, "/v1/session/exchange", exchange2, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("replayed credential = %d (%s)", status, body)
	}
	refused := decodeEnvelope(t, body)
	if refused.Code != string(errs.CodeAuthzDenied) || refused.Data["reason"] != security.ReasonSessionInvalid {
		t.Fatalf("replayed credential envelope = %+v", refused)
	}
	// The first session no longer validates anywhere — the wire proof.
	status, _, body = env.doWire(t, http.MethodGet, "/v1/events/replay?stream=job&limit=1", "", sess2.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("revoked probe = %d (%s)", status, body)
	}
	var revoked replayResult
	if err := json.Unmarshal(body, &revoked); err != nil {
		t.Fatalf("revoked probe decode: %v (%s)", err, body)
	}
	if revoked.Outcome != events.ReplayForbidden {
		t.Fatalf("the revoked session's probe must resolve to the coded denial, got %s (%s)", revoked.Outcome, body)
	}
	// And the in-process service state agrees (the same revocation, the
	// session service's own view).
	if env.sessions.Validate(sess2.Token, security.ScopeData) {
		t.Fatal("the session minted by the replayed exchange must be revoked in the service")
	}
}

// TestLiveBoundaryRefusals drives the boundary refusals over the REAL
// listener (wire-level forms of the handler-direct cases server_test.go
// pins): a forged Host header, a foreign-Origin mutation, and a
// URL-carried credential each resolve to the typed refusal envelope with
// the stable reason code — the DNS-rebinding defense, the CSRF posture,
// and the credential-surface rule, live.
func TestLiveBoundaryRefusals(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
		set    func(*http.Request)
		reason string
	}{
		{
			name:   "forged_host_refused",
			method: http.MethodGet,
			path:   "/v1/jobs/journey-unknown",
			token:  "kry1-not-a-live-token-0000",
			set:    func(r *http.Request) { r.Host = "attacker.example.com" },
			reason: security.ReasonHostNotAllowed,
		},
		{
			name:   "foreign_origin_mutation_refused",
			method: http.MethodPost,
			path:   "/v1/jobs",
			body:   submitBody("journey-boundary", "task"),
			set:    func(r *http.Request) { r.Header.Set("Origin", "http://attacker.example.com") },
			reason: security.ReasonOriginNotAllowed,
		},
		{
			name:   "url_carried_credential_refused",
			method: http.MethodGet,
			path:   "/v1/jobs/journey-unknown?leak=" + url.QueryEscape("kry1-abcdefabcdefabcdefabcdefabcdef"),
			token:  "kry1-abcdefabcdefabcdefabcdefabcdef",
			reason: security.ReasonCredentialInURL,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := env.doWire(t, tc.method, tc.path, tc.body, tc.token, tc.set)
			if status != http.StatusForbidden {
				t.Fatalf("status = %d (%s)", status, body)
			}
			got := decodeEnvelope(t, body)
			if got.Code != string(errs.CodeAuthzDenied) || got.Data["reason"] != tc.reason {
				t.Fatalf("envelope = %+v, want reason %s", got, tc.reason)
			}
		})
	}
}
