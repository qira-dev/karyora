package httplocal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// memoryAuditStore is the test audit sink (the chain contract is exercised
// by the security and storage packages; this sink records decisions).
type memoryAuditStore struct {
	mu      sync.Mutex
	records []security.ChainedRecord
	anchors []security.Anchor
}

func (m *memoryAuditStore) Tail() (security.TailState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return security.TailState{}, nil
	}
	last := m.records[len(m.records)-1]
	return security.TailState{Sequence: last.Sequence, RecHash: last.RecHash}, nil
}

func (m *memoryAuditStore) Append(rec security.ChainedRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, rec)
	return nil
}

func (m *memoryAuditStore) Records() ([]security.ChainedRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]security.ChainedRecord(nil), m.records...), nil
}

func (m *memoryAuditStore) Anchor(a security.Anchor) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anchors = append(m.anchors, a)
	return nil
}

func (m *memoryAuditStore) Anchors() ([]security.Anchor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]security.Anchor(nil), m.anchors...), nil
}

func (m *memoryAuditStore) SetIntegrityBroken(bool) error { return nil }
func (m *memoryAuditStore) IntegrityBroken() (bool, error) {
	return false, nil
}

// slowExecutor delays the provider call so a cancel can race an in-flight
// attempt (the same pattern the app-level cancellation test uses).
type slowExecutor struct {
	inner app.ProviderExecutor
	delay time.Duration
}

func (s *slowExecutor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	time.Sleep(s.delay)
	return s.inner.Execute(ctx, plan)
}

func (s *slowExecutor) ProfileFor(model string) dprovider.ProfileID {
	return s.inner.ProfileFor(model)
}

func (s *slowExecutor) Connection() dprovider.Connection {
	return s.inner.Connection()
}

func (s *slowExecutor) Capabilities(profile dprovider.ProfileID) dprovider.CapabilitySet {
	return s.inner.Capabilities(profile)
}

func (s *slowExecutor) TestConnection(ctx context.Context, plan *dprovider.RequestPlan) error {
	return s.inner.TestConnection(ctx, plan)
}

func (s *slowExecutor) TestConnectionWith(ctx context.Context, secrets security.SecretStore, plan *dprovider.RequestPlan) error {
	return s.inner.TestConnectionWith(ctx, secrets, plan)
}

func (s *slowExecutor) DataHandling(profile dprovider.ProfileID, feature dprovider.Feature) (dprovider.DataHandlingProfile, bool) {
	return s.inner.DataHandling(profile, feature)
}

// auditTestReader adapts the test store's audit row query to the app
// slice's AuditReader port (IMPL-24): the test-side twin of the
// composition root's auditRowReader — the payload decode happens in the
// app layer, the adapter copies the raw chain frames.
type auditTestReader struct{ st *storage.Store }

func (a auditTestReader) ListAuditRows(ctx context.Context, q app.AuditRowsQuery) (app.AuditRowsPage, error) {
	page, err := a.st.AuditListRows(ctx, storage.AuditListQuery{
		Limit:          q.Limit,
		AfterSequence:  q.AfterSequence,
		PolicyDecision: q.PolicyDecision,
		FromTime:       q.FromTime,
		ToTime:         q.ToTime,
		OrderDesc:      q.OrderDesc,
	})
	if err != nil {
		return app.AuditRowsPage{}, err
	}
	out := app.AuditRowsPage{More: page.More, Rows: make([]app.AuditRow, 0, len(page.Rows))}
	for _, r := range page.Rows {
		out.Rows = append(out.Rows, app.AuditRow{Sequence: r.Sequence, Payload: r.Payload})
	}
	return out, nil
}

type testEnv struct {
	slice    *app.Slice
	deps     app.SliceDeps
	sessions *security.SessionService
	srv      *Server
	ts       *httptest.Server
	token    string
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvComposed(t, nil)
}

// newTestEnvComposed is newTestEnv with an optional deps hook: a surface
// that needs an extra port wired into the REAL composition (e.g. the
// per-connection custom-executor factory) configures the deps before the
// slice and the server are built (the IMPL-13 env-wiring precedent).
func newTestEnvComposed(t *testing.T, configure func(*app.SliceDeps)) *testEnv {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "karyora.db")
	st, err := storage.Open(ctx, storage.Options{
		Path:                   dbPath,
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

	// One direct adapter beside the fixture: the deepseek executor (its
	// data-handling record refuses every probe label typed, so no wire
	// test can dial out) gives the connection surface a credential-bearing
	// posture for its masked-status and idempotency pins.
	ds := providers.NewDirectAdapters()
	var deepseek providers.Adapter
	for _, a := range ds {
		if a.ID() == dprovider.ProviderDeepSeek {
			deepseek = a
		}
	}
	dreg := dprovider.NewRegistry(nil)
	dreg.Upsert(deepseek.SeedDataHandling(deepseek.DefaultProfile("m")))
	dexec, err := providers.NewExecutor(deepseek, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, dreg)
	if err != nil {
		t.Fatalf("direct executor: %v", err)
	}

	sessions := security.NewSessionService("inst-httplocal-test", filepath.Join(t.TempDir(), "data"))
	guard := &security.Guard{
		Policy: security.NewLocalPolicy(sessions),
		Audit:  security.NewAuditService(&memoryAuditStore{}, 64, 1000),
	}
	deps := app.SliceDeps{
		Relay:         st,
		Snapshots:     st,
		Objects:       st,
		Events:        st,
		Packs:         st,
		Guard:         guard,
		Providers:     map[string]app.ProviderExecutor{dprovider.ProviderMock: exec, dprovider.ProviderDeepSeek: dexec},
		Backoff:       st,
		Connections:   st,
		Secrets:       newWireFakeSecrets(),
		Conversations: st,
		// The F4 attachment surface (IMPL-25): the test-side twin of the
		// composition root's wiring — the same store implements the
		// attachment port.
		ConversationAttachments: st,
		Notifications:           st,
		// The B1 KnowledgePack family surface (DE-0 queue row 1): the
		// test-side twin of the composition root's wiring — the same store
		// implements the knowledge port.
		Knowledge: st,
		// The C-4 session-command surface (IMPL-23): the real composition
		// wires the same session service the Guard and this transport hold.
		Sessions: sessions,
		// The F12 audit read surface (IMPL-24): the test-side twin of the
		// composition root's auditRowReader adapter over the same store.
		AuditReader: auditTestReader{st: st},
	}
	if configure != nil {
		configure(&deps)
	}
	slice, err := app.NewSlice(deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	srv, err := New(Deps{Slice: slice, Sessions: sessions, Guard: guard, Replay: st})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// The harness caller session: the same C-1 exchange the wire performs,
	// run in-process so each test stays focused on its own surface.
	cred, err := sessions.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	token, err := sessions.Exchange(cred, "", []security.Scope{security.ScopeData})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	return &testEnv{slice: slice, deps: deps, sessions: sessions, srv: srv, ts: ts, token: token.Secret}
}

// do performs one request against the live loopback test server.
func (e *testEnv) do(t *testing.T, method, path, body, token string, headers map[string]string) (int, []byte) {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

// raw performs one boundary-level request against the handler directly so a
// test controls the Host/Origin/Referer surfaces exactly. The default Host
// is the loopback form (what the real server's clients present); tests
// override it to prove the Host refusal.
func (e *testEnv) raw(method, target, body string, set func(*http.Request)) *httptest.ResponseRecorder {
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, rd)
	r.Host = "127.0.0.1:8080"
	if set != nil {
		set(r)
	}
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	return w
}

// wireEnvelope mirrors the D-35 four-field envelope on the wire.
type wireEnvelope struct {
	Code          string         `json:"code"`
	Retryable     bool           `json:"retryable"`
	CorrelationID string         `json:"correlationId"`
	Data          map[string]any `json:"data"`
}

func decodeEnvelope(t *testing.T, body []byte) wireEnvelope {
	t.Helper()
	var env wireEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("envelope decode: %v (%s)", err, body)
	}
	return env
}

// awaitSettlement closes the gap between the bounded driver loop's return
// and the durable settlement (the routine repair the IMPL-7 verification
// surfaced: under the race detector's CI load, RunToSettlement's tick
// budget can expire while the cooperative-cancellation settlement commit
// is still in flight on the handler goroutine — the loop is a driver, not
// the settlement's clock). The poll is the system's own
// reconciliation-by-read — the same authoritative store the cancel
// semantics prescribe — bounded, and it WEAKENS nothing: the assertion
// still requires the job to reach its terminal state.
func awaitSettlement(t *testing.T, env *testEnv, jobID string, driven <-chan aijobrelay.Job) aijobrelay.Job {
	t.Helper()
	final := <-driven
	if aijobrelay.TerminalJob(final.State) {
		return final
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err := env.deps.Relay.LoadJob(context.Background(), jobID)
		if err != nil {
			t.Fatalf("settlement reload: %v", err)
		}
		if aijobrelay.TerminalJob(job.State) {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	return final
}

func submitBody(key, task string) string {
	return fmt.Sprintf(`{"idempotency_key":%q,"task":%q,"provider":"mock","model":"m","classification":"test-fixture"}`, key, task)
}

// TestPositiveOperations covers one positive case per exposed operation:
// the wire payloads carry the adopted extraction's shapes (the JobInput/Job
// field names, the event Record field set with occurred_at/schema_version,
// and the read-only probe's job/events keys).
func TestPositiveOperations(t *testing.T) {
	env := newTestEnv(t)

	// Submit: 202 Accepted (processing, never completion).
	status, body := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-1", "Summarize the fixture corpus."), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("submit status = %d (%s)", status, body)
	}
	var sub struct {
		Job      aijobrelay.Job `json:"job"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &sub); err != nil {
		t.Fatalf("submit decode: %v (%s)", err, body)
	}
	if sub.Job.JobID == "" || sub.Replayed {
		t.Fatalf("submit must report a fresh Job, got %+v", sub)
	}
	if sub.Job.State != aijobrelay.JobQueued || sub.Job.IdempotencyKey != "idem-1" {
		t.Fatalf("submit job shape = %+v", sub.Job)
	}

	// Status: the Job plus its (empty) current Attempt.
	status, body = env.do(t, http.MethodGet, "/v1/jobs/"+sub.Job.JobID, "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("status status = %d (%s)", status, body)
	}
	var st struct {
		Job     aijobrelay.Job     `json:"job"`
		Attempt aijobrelay.Attempt `json:"attempt"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("status decode: %v (%s)", err, body)
	}
	if st.Job.JobID != sub.Job.JobID {
		t.Fatalf("status job = %+v", st.Job)
	}

	// Events: the coded Record field set, in sequence order.
	status, body = env.do(t, http.MethodGet, "/v1/jobs/"+sub.Job.JobID+"/events", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("events status = %d (%s)", status, body)
	}
	var ev struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatalf("events decode: %v (%s)", err, body)
	}
	if len(ev.Events) == 0 {
		t.Fatalf("events must carry the submitted event, got %s", body)
	}
	first := ev.Events[0]
	for _, key := range []string{"event_id", "aggregate_type", "aggregate_id", "sequence", "event_type", "schema_version", "occurred_at", "producer"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("event record missing the coded field %q: %v", key, first)
		}
	}
	if first["event_type"] != "job.submitted" {
		t.Fatalf("first event = %v", first["event_type"])
	}

	// Result: the settled Job's CAS payload, after the same bounded
	// scheduler the CLI foreground session runs.
	final, err := app.RunToSettlement(app.WithCaller(context.Background(), "local", env.token), env.deps, "worker-test", 1, sub.Job.JobID, 64)
	if err != nil {
		t.Fatalf("settlement: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded {
		t.Fatalf("fixture job must succeed, got %s", final.State)
	}
	status, body = env.do(t, http.MethodGet, "/v1/jobs/"+sub.Job.JobID+"/result", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("result status = %d (%s)", status, body)
	}
	if !json.Valid(body) || len(body) == 0 {
		t.Fatalf("result payload must be the stored JSON result, got %s", body)
	}
}

// TestEnvelopeFailurePaths covers the failure/envelope cases: the four-field
// D-35 envelope as coded, with the register's default retryable/retryClass
// semantics and the typed CONFLICT_STATE idempotency-conflict path.
func TestEnvelopeFailurePaths(t *testing.T) {
	env := newTestEnv(t)

	status, body := env.do(t, http.MethodPost, "/v1/jobs", submitBody("", "no key"), env.token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid submit status = %d (%s)", status, body)
	}
	env1 := decodeEnvelope(t, body)
	if env1.Code != string(errs.CodeValidationFailed) || env1.Retryable {
		t.Fatalf("envelope = %+v", env1)
	}
	if env1.Data["field"] != "idempotency_key" || env1.Data["retryClass"] != string(errs.RetryNever) {
		t.Fatalf("validation data = %v", env1.Data)
	}
	if env1.CorrelationID == "" {
		t.Fatal("envelope must carry a correlation id")
	}

	status, body = env.do(t, http.MethodGet, "/v1/jobs/job-unknown", "", env.token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown job status = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Code; got != string(errs.CodeNotFound) {
		t.Fatalf("code = %s", got)
	}

	// Cancelling a submitted (queued) Job is accepted; cancelling the now
	// terminal Job again is a typed conflict (terminal states accept no
	// further transitions).
	status, body = env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-fail", "task"), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("submit status = %d (%s)", status, body)
	}
	var sub struct {
		Job aijobrelay.Job `json:"job"`
	}
	if err := json.Unmarshal(body, &sub); err != nil {
		t.Fatalf("decode: %v", err)
	}
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+sub.Job.JobID+"/cancel", "", env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("cancel of queued job status = %d (%s)", status, body)
	}
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+sub.Job.JobID+"/cancel", "", env.token, nil)
	if status != http.StatusConflict {
		t.Fatalf("cancel of cancelled job status = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Code; got != string(errs.CodeConflictState) {
		t.Fatalf("code = %s", got)
	}
}

// TestIdempotencyConflictOverTransport proves the coded idempotency
// semantics end to end: same key + same input replays the original Job;
// the same key with a different input is a typed CONFLICT_STATE — never a
// second Job.
func TestIdempotencyConflictOverTransport(t *testing.T) {
	env := newTestEnv(t)

	status, first := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-replay", "Same task."), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("first submit = %d (%s)", status, first)
	}
	status, second := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-replay", "Same task."), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("replay submit = %d (%s)", status, second)
	}
	var s1, s2 struct {
		Job      aijobrelay.Job `json:"job"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(first, &s1); err != nil {
		t.Fatalf("decode first: %v (%s)", err, first)
	}
	if err := json.Unmarshal(second, &s2); err != nil {
		t.Fatalf("decode second: %v (%s)", err, second)
	}
	if s2.Job.JobID != s1.Job.JobID || !s2.Replayed || s1.Replayed {
		t.Fatalf("same key + same input must replay the original: %v vs %v", s1, s2)
	}

	status, body := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-replay", "A different task."), env.token, nil)
	if status != http.StatusConflict {
		t.Fatalf("conflict submit = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Code; got != string(errs.CodeConflictState) {
		t.Fatalf("code = %s", got)
	}
}

// TestCancelOverTransport proves the cooperative-cancellation path through
// the wire: the cancel is accepted (202) while an attempt is in flight and
// the Job settles Cancelled.
func TestCancelOverTransport(t *testing.T) {
	env := newTestEnv(t)
	slow := &slowExecutor{inner: env.deps.Providers[dprovider.ProviderMock], delay: 300 * time.Millisecond}
	env.deps.Providers[dprovider.ProviderMock] = slow

	status, body := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-cancel", "Long task."), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d (%s)", status, body)
	}
	var sub struct {
		Job aijobrelay.Job `json:"job"`
	}
	if err := json.Unmarshal(body, &sub); err != nil {
		t.Fatalf("decode: %v", err)
	}
	done := make(chan aijobrelay.Job, 1)
	go func() {
		job, _ := app.RunToSettlement(app.WithCaller(context.Background(), "local", env.token), env.deps, "worker-test", 1, sub.Job.JobID, 128)
		done <- job
	}()
	time.Sleep(60 * time.Millisecond) // the attempt is in flight
	status, body = env.do(t, http.MethodPost, "/v1/jobs/"+sub.Job.JobID+"/cancel", "", env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("cancel = %d (%s)", status, body)
	}
	final := awaitSettlement(t, env, sub.Job.JobID, done)
	if final.State != aijobrelay.JobCancelled {
		t.Fatalf("cancelled job must settle Cancelled, got %s", final.State)
	}
}

// TestBoundaryRefusals exercises the boundary reason codes the adapter
// enforces, one refusal case per exercised code. CSRF_CHECK_FAILED is not
// reachable in this milestone's fail-closed posture (no token issuance —
// the recorded open item, C-2): the coded predicate refuses with
// MISSING_CSRF_TOKEN before any mismatch can arise.
func TestBoundaryRefusals(t *testing.T) {
	env := newTestEnv(t)

	t.Run("state_changing_get_prohibited_at_registration", func(t *testing.T) {
		if err := validateRoutes(routes()); err != nil {
			t.Fatalf("the fixed route table must register cleanly: %v", err)
		}
		bad := route{method: http.MethodGet, prefix: "/v1/jobs", mutates: true}
		err := validateRoutes([]route{bad})
		var b *security.ErrBoundary
		if err == nil || !errors.As(err, &b) || b.Reason != security.ReasonStateChangingGET {
			t.Fatalf("a mutating GET must be refused at registration, got %v", err)
		}
	})

	t.Run("non_loopback_bind_refused", func(t *testing.T) {
		if err := ValidateBind("127.0.0.1"); err != nil {
			t.Fatalf("loopback bind must pass: %v", err)
		}
		if err := ValidateBind("localhost"); err != nil {
			t.Fatalf("localhost bind must pass: %v", err)
		}
		err := ValidateBind("192.168.1.10")
		var b *security.ErrBoundary
		if err == nil || !errors.As(err, &b) || b.Reason != security.ReasonNonLoopbackBind {
			t.Fatalf("non-loopback bind must be refused, got %v", err)
		}
	})

	cases := []struct {
		name   string
		method string
		target string
		body   string
		token  string
		set    func(*http.Request)
		reason string
	}{
		{
			name:   "host_not_allowed",
			method: http.MethodGet,
			target: "/v1/jobs/job-1",
			token:  "unused",
			set:    func(r *http.Request) { r.Host = "attacker.example.com" },
			reason: security.ReasonHostNotAllowed,
		},
		{
			name:   "origin_not_allowed_foreign",
			method: http.MethodPost,
			target: "/v1/jobs",
			body:   submitBody("idem-boundary", "task"),
			set:    func(r *http.Request) { r.Header.Set("Origin", "http://attacker.example.com") },
			reason: security.ReasonOriginNotAllowed,
		},
		{
			name:   "origin_not_allowed_empty",
			method: http.MethodPost,
			target: "/v1/jobs",
			body:   submitBody("idem-boundary", "task"),
			reason: security.ReasonOriginNotAllowed,
		},
		{
			name:   "missing_csrf_token",
			method: http.MethodPost,
			target: "/v1/jobs",
			body:   submitBody("idem-boundary", "task"),
			set:    func(r *http.Request) { r.Header.Set("Origin", "http://localhost:8080") },
			reason: security.ReasonMissingCSRFToken,
		},
		{
			name:   "credential_in_url",
			method: http.MethodGet,
			target: "/v1/jobs/job-1?leak=" + url.QueryEscape("kry1-abcdefabcdefabcdefabcdefabcdef"),
			token:  "kry1-abcdefabcdefabcdefabcdefabcdef",
			reason: security.ReasonCredentialInURL,
		},
		{
			name:   "credential_in_referer",
			method: http.MethodGet,
			target: "/v1/jobs/job-1",
			token:  "kry1-abcdefabcdefabcdefabcdefabcdef",
			set: func(r *http.Request) {
				r.Header.Set("Referer", "https://attacker.example.com/?t=kry1-abcdefabcdefabcdefabcdefabcdef")
			},
			reason: security.ReasonCredentialInReferer,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := env.raw(tc.method, tc.target, tc.body, func(r *http.Request) {
				if tc.token != "" {
					r.Header.Set("Authorization", "Bearer "+tc.token)
				}
				if tc.set != nil {
					tc.set(r)
				}
			})
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
			}
			env := decodeEnvelope(t, w.Body.Bytes())
			if env.Code != string(errs.CodeAuthzDenied) || env.Data["reason"] != tc.reason {
				t.Fatalf("envelope = %+v, want reason %s", env, tc.reason)
			}
		})
	}
}

// TestReplayEndpoint covers the event-streaming surface: the coded replay
// read with the opaque cursor and the explicit outcomes, authorization
// applied at read/replay time through the decision port.
func TestReplayEndpoint(t *testing.T) {
	env := newTestEnv(t)

	status, body := env.do(t, http.MethodPost, "/v1/jobs", submitBody("idem-stream", "Stream me."), env.token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("submit = %d (%s)", status, body)
	}

	// A valid read: outcome ok, the records, and an opaque next cursor.
	status, body = env.do(t, http.MethodGet, "/v1/events/replay?stream=job&limit=10", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("replay = %d (%s)", status, body)
	}
	var rep replayResult
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("replay decode: %v (%s)", err, body)
	}
	if rep.Outcome != events.ReplayOK || len(rep.Events) == 0 || rep.Cursor == "" {
		t.Fatalf("replay result = %+v", rep)
	}
	if strings.ContainsAny(rep.Cursor, "{}") {
		t.Fatalf("the cursor must stay opaque on the wire: %q", rep.Cursor)
	}

	// The cursor round-trips: the next read resolves after the position.
	status, body = env.do(t, http.MethodGet, "/v1/events/replay?stream=job&limit=10&cursor="+url.QueryEscape(rep.Cursor), "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("cursor replay = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("cursor replay decode: %v", err)
	}
	if rep.Outcome != events.ReplayOK {
		t.Fatalf("cursor replay outcome = %s", rep.Outcome)
	}

	// A malformed cursor resolves to the explicit expiry outcome — a
	// client is never stranded in a false permanent status (C2-13 §5).
	status, body = env.do(t, http.MethodGet, "/v1/events/replay?stream=job&limit=10&cursor=garbage", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("malformed cursor = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Outcome != events.ReplayCursorExpired || len(rep.Events) != 0 {
		t.Fatalf("malformed cursor result = %+v", rep)
	}

	// An unauthenticated read resolves to the forbidden outcome through
	// the decision port (never stranded, never served).
	status, body = env.do(t, http.MethodGet, "/v1/events/replay?stream=job&limit=10", "", "kry1-not-a-live-token-0000", nil)
	if status != http.StatusOK {
		t.Fatalf("forbidden replay = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Outcome != events.ReplayForbidden {
		t.Fatalf("forbidden outcome = %s", rep.Outcome)
	}

	// The bounds: a missing/out-of-range limit and a missing stream are
	// typed validation failures.
	for _, tc := range []struct{ path, field string }{
		{"/v1/events/replay?stream=job", "limit"},
		{"/v1/events/replay?stream=job&limit=0", "limit"},
		{"/v1/events/replay?stream=job&limit=1001", "limit"},
		{"/v1/events/replay?limit=10", "stream"},
	} {
		status, body = env.do(t, http.MethodGet, tc.path, "", env.token, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("%s status = %d (%s)", tc.path, status, body)
		}
		if got := decodeEnvelope(t, body).Data["field"]; got != tc.field {
			t.Fatalf("%s field = %v", tc.path, got)
		}
	}
}

// TestC1BootstrapExchangeHappyPath covers the adopted P-S3-01 candidate's
// exchange over the wire: a one-time, short-lived, origin-bound bootstrap
// credential issued on the trusted local channel exchanges exactly once for
// a kry1- session token carrying the requested scopes.
func TestC1BootstrapExchangeHappyPath(t *testing.T) {
	env := newTestEnv(t)

	// Local-process binding (empty origin): the exchange carries no Origin.
	cred, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	reqBody := fmt.Sprintf(`{"credential":%q,"scopes":["data"]}`, cred.Secret)
	status, body := env.do(t, http.MethodPost, "/v1/session/exchange", reqBody, "", nil)
	if status != http.StatusOK {
		t.Fatalf("exchange = %d (%s)", status, body)
	}
	var sess sessionResult
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("exchange decode: %v (%s)", err, body)
	}
	if !strings.HasPrefix(sess.Token, "kry1-") || sess.ID == "" || len(sess.ID) != 16 {
		t.Fatalf("session token shape = %+v", sess)
	}
	if len(sess.Scopes) != 1 || sess.Scopes[0] != string(security.ScopeData) {
		t.Fatalf("session scopes = %v", sess.Scopes)
	}
	if sess.Expires.IsZero() {
		t.Fatal("session must carry its absolute expiry")
	}
	// The minted session authorizes the data surface.
	status, body = env.do(t, http.MethodGet, "/v1/events/replay?stream=job&limit=1", "", sess.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("token call = %d (%s)", status, body)
	}
	var rep replayResult
	if err := json.Unmarshal(body, &rep); err != nil || rep.Outcome != events.ReplayOK {
		t.Fatalf("token call result = %s (%s)", rep.Outcome, body)
	}

	// Origin-bound issuance: the exchange must present the matching Origin.
	bound, err := env.srv.IssueBootstrapCredential("http://localhost:5173")
	if err != nil {
		t.Fatalf("issue bound: %v", err)
	}
	boundBody := fmt.Sprintf(`{"credential":%q,"scopes":["data"]}`, bound.Secret)
	status, body = env.do(t, http.MethodPost, "/v1/session/exchange", boundBody, "", map[string]string{"Origin": "http://localhost:5173"})
	if status != http.StatusOK {
		t.Fatalf("bound exchange = %d (%s)", status, body)
	}
	// A mismatched Origin is refused by the service's binding check.
	mismatch := mustIssue(t, env, "http://localhost:5173")
	status, body = env.do(t, http.MethodPost, "/v1/session/exchange", fmt.Sprintf(`{"credential":%q,"scopes":["data"]}`, mismatch.Secret), "", map[string]string{"Origin": "http://localhost:9999"})
	if status != http.StatusForbidden {
		t.Fatalf("mismatched exchange = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body); got.Code != string(errs.CodeAuthzDenied) || got.Data["reason"] != security.ReasonSessionInvalid {
		t.Fatalf("mismatched envelope = %+v", got)
	}
}

func mustIssue(t *testing.T, env *testEnv, origin string) security.BootstrapCredential {
	t.Helper()
	cred, err := env.srv.IssueBootstrapCredential(origin)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return cred
}

// TestC1BootstrapReplayRejectedAndInvalidates proves the coded replay rule
// end to end: the replayed credential is rejected AND the session the first
// exchange minted is revoked (S-3 — no stale session continuity).
func TestC1BootstrapReplayRejectedAndInvalidates(t *testing.T) {
	env := newTestEnv(t)

	cred, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	reqBody := fmt.Sprintf(`{"credential":%q,"scopes":["data"]}`, cred.Secret)
	status, first := env.do(t, http.MethodPost, "/v1/session/exchange", reqBody, "", nil)
	if status != http.StatusOK {
		t.Fatalf("first exchange = %d (%s)", status, first)
	}
	var sess sessionResult
	if err := json.Unmarshal(first, &sess); err != nil {
		t.Fatalf("decode: %v", err)
	}

	status, body := env.do(t, http.MethodPost, "/v1/session/exchange", reqBody, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("replayed exchange = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body); got.Code != string(errs.CodeAuthzDenied) || got.Data["reason"] != security.ReasonSessionInvalid {
		t.Fatalf("replayed envelope = %+v", got)
	}
	// The minted session is invalidated: it no longer validates anywhere.
	if env.sessions.Validate(sess.Token, security.ScopeData) {
		t.Fatal("the session minted by the replayed exchange must be revoked")
	}
	// A third presentation is refused the same way.
	status, _ = env.do(t, http.MethodPost, "/v1/session/exchange", reqBody, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("third exchange = %d", status)
	}
}

// TestExchangeForgedCredentialRefused covers the invalid-credential
// refusal honestly: a credential this process never issued cannot pass the
// predicate's credential carve-out, so the request is refused at the CSRF
// boundary (the forged secret never reaches the exchange at all). The
// scope-validation refusals run after a credential HAS passed the
// carve-out.
func TestExchangeForgedCredentialRefused(t *testing.T) {
	env := newTestEnv(t)

	// No Origin presented: the empty-Origin denial.
	status, body := env.do(t, http.MethodPost, "/v1/session/exchange", `{"credential":"bootstrap-forged-value","scopes":["data"]}`, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("forged credential, no origin = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Data["reason"]; got != security.ReasonOriginNotAllowed {
		t.Fatalf("reason = %v", got)
	}
	// A loopback Origin without a token: the missing-token refusal.
	status, body = env.do(t, http.MethodPost, "/v1/session/exchange", `{"credential":"bootstrap-forged-value","scopes":["data"]}`, "", map[string]string{"Origin": "http://localhost:8080"})
	if status != http.StatusForbidden {
		t.Fatalf("forged credential, loopback origin = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Data["reason"]; got != security.ReasonMissingCSRFToken {
		t.Fatalf("reason = %v", got)
	}

	// A genuine credential with an unknown scope fails typed.
	cred, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	status, body = env.do(t, http.MethodPost, "/v1/session/exchange", fmt.Sprintf(`{"credential":%q,"scopes":["root"]}`, cred.Secret), "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown scope = %d (%s)", status, body)
	}
	if got := decodeEnvelope(t, body).Data["field"]; got != "scopes" {
		t.Fatalf("field = %v", got)
	}
	// An empty scope request fails closed (the service requires at least
	// one scope).
	cred2, err := env.srv.IssueBootstrapCredential("")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	status, _ = env.do(t, http.MethodPost, "/v1/session/exchange", fmt.Sprintf(`{"credential":%q}`, cred2.Secret), "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("empty scopes = %d", status)
	}
}
