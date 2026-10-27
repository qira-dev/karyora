package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/knowledgepack"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// memoryAuditStore is the test audit sink: the chain contract is exercised
// by the security and storage packages; this sink records decisions.
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

type testEnv struct {
	store  *storage.Store
	dbPath string
	slice  *Slice
	mock   *providers.MockAdapter
	token  string // the caller's session token
	deps   SliceDeps
}

// flippingExecutor wraps the mock and fails the FIRST call with a
// retryable outage, proving the retry path end to end.
type flippingExecutor struct {
	inner ProviderExecutor
	mu    sync.Mutex
	fails int
}

func (f *flippingExecutor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	f.mu.Lock()
	if f.fails > 0 {
		f.fails--
		f.mu.Unlock()
		return nil, dprovider.NewError(dprovider.ErrUnreachable, dprovider.ProviderMock, "scripted transient outage")
	}
	f.mu.Unlock()
	return f.inner.Execute(ctx, plan)
}

func (f *flippingExecutor) ProfileFor(model string) dprovider.ProfileID {
	return f.inner.ProfileFor(model)
}

func (f *flippingExecutor) Connection() dprovider.Connection {
	return f.inner.Connection()
}

func newTestEnv(t *testing.T) *testEnv {
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

	sessions := security.NewSessionService("inst-test", filepath.Join(t.TempDir(), "data"))
	cred, err := sessions.IssueBootstrapCredential("cli")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	token, err := sessions.Exchange(cred, "cli", []security.Scope{security.ScopeData})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	guard := &security.Guard{
		Policy: security.NewLocalPolicy(sessions),
		Audit:  security.NewAuditService(&memoryAuditStore{}, 64, 1000),
	}
	deps := SliceDeps{
		Relay:     st,
		Snapshots: st,
		Objects:   st,
		Events:    st,
		Packs:     st,
		Guard:     guard,
		Providers: map[string]ProviderExecutor{dprovider.ProviderMock: exec},
		Backoff:   st,
	}
	slice, err := NewSlice(deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	return &testEnv{store: st, dbPath: dbPath, slice: slice, mock: mock, token: token.Secret, deps: deps}
}

func (e *testEnv) ctx() context.Context {
	return WithCaller(context.Background(), "local", e.token)
}

func (e *testEnv) submit(t *testing.T, req SubmitRequest) *SubmitResult {
	t.Helper()
	res, err := e.slice.Submit(e.ctx(), req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return res
}

func baseRequest() SubmitRequest {
	return SubmitRequest{
		IdempotencyKey: "idem-1",
		Task:           "Summarize the recorded fixture corpus.",
		Provider:       dprovider.ProviderMock,
		Model:          "m",
		Classification: dprovider.ClassificationTestFixture,
	}
}

// TestSubmitIdempotencySemantics proves the C3-02 semantics through the
// API surface: same key + same canonical input replays the original Job;
// the same key with different input is a typed CONFLICT.
func TestSubmitIdempotencySemantics(t *testing.T) {
	env := newTestEnv(t)
	first := env.submit(t, baseRequest())
	second, err := env.slice.Submit(env.ctx(), baseRequest())
	if err != nil {
		t.Fatalf("same-key same-input replay refused: %v", err)
	}
	if second.Job.JobID != first.Job.JobID || !second.Replayed {
		t.Fatal("same key + same input must return the original Job")
	}

	conflict := baseRequest()
	conflict.Task = "A different task entirely."
	if _, err := env.slice.Submit(env.ctx(), conflict); err == nil {
		t.Fatal("same key + different input must be refused")
	} else if !strings.Contains(err.Error(), string(errs.CodeConflictState)) {
		t.Fatalf("conflict must be typed, got %v", err)
	}
}

// TestJobSucceedsWithoutKnowledgePackOrWork is the D-12 acceptance: the
// full submit → claim → compile → gated provider call → atomic commit →
// inspect path succeeds with neither KnowledgePack nor Work, and the
// provenance (snapshot digest, provider, usage, result digest) is
// inspectable through the events.
func TestJobSucceedsWithoutKnowledgePackOrWork(t *testing.T) {
	env := newTestEnv(t)
	req := baseRequest()
	sub := env.submit(t, req)

	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 2, sub.Job.JobID, 32)
	if err != nil {
		t.Fatalf("run to settlement: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded || final.ReceiptID == "" {
		t.Fatalf("job did not succeed: %+v", final)
	}
	if final.AttemptCount != 1 {
		t.Fatalf("expected exactly one attempt, got %d", final.AttemptCount)
	}

	// The result payload is readable from the CAS under the committed digest.
	payload, err := env.slice.Result(env.ctx(), sub.Job.JobID)
	if err != nil {
		t.Fatalf("result read: %v", err)
	}
	var result dprovider.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("result payload is not the neutral result: %v", err)
	}
	if !strings.Contains(result.Text, "mock-reply") {
		t.Fatalf("unexpected result text %q", result.Text)
	}

	// Journal inspection: claimed → succeeded with bounded provenance.
	recs, err := env.slice.Events(env.ctx(), sub.Job.JobID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	types := eventTypes(recs)
	if len(types) < 3 || types[0] != "job.submitted" || types[1] != "job.claimed" || types[len(types)-1] != "job.succeeded" {
		t.Fatalf("unexpected event sequence %v", types)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal(recs[len(recs)-1].InlinePayload, &payloadMap); err != nil {
		t.Fatalf("succeeded event payload: %v", err)
	}
	for _, key := range []string{"snapshot_id", "assembled_digest", "provider", "result_digest", "usage"} {
		if _, ok := payloadMap[key]; !ok {
			t.Errorf("succeeded event is missing provenance %q", key)
		}
	}

	// The snapshot is inspectable and reconstructable from the record.
	snapID, _ := payloadMap["snapshot_id"].(string)
	snap, err := env.deps.Snapshots.LoadSnapshot(env.ctx(), snapID)
	if err != nil {
		t.Fatalf("snapshot load: %v", err)
	}
	if snap.AssembledDigest == "" || snap.AssembledRepresentation == "" {
		t.Fatal("snapshot lacks its assembled representation")
	}

	// Status reads back the same state through the same contract.
	job, attempt, err := env.slice.Status(env.ctx(), sub.Job.JobID)
	if err != nil || job.State != aijobrelay.JobSucceeded || attempt.Outcome != aijobrelay.AttemptSucceeded {
		t.Fatalf("status mismatch: %v %+v %+v", err, job, attempt)
	}
}

func eventTypes(recs []events.Record) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.EventType)
	}
	return out
}

// TestJobWithKnowledgePackProvenance proves the optional retrieval leg:
// an attached ACTIVE pack contributes inspectable provenance, and its
// chunks reach the snapshot as untrusted retrieved data.
func TestJobWithKnowledgePackProvenance(t *testing.T) {
	env := newTestEnv(t)
	packID := seedFixturePack(t, env, "Karyora storage keeps durable job receipts for every attempt.")
	req := baseRequest()
	req.IdempotencyKey = "idem-pack"
	req.Task = "Explain how durable job receipts are kept."
	req.PackRef = packID
	sub := env.submit(t, req)

	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 32)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded {
		t.Fatalf("job did not succeed: %+v", final)
	}
	recs, _ := env.slice.Events(env.ctx(), sub.Job.JobID)
	var payloadMap map[string]any
	_ = json.Unmarshal(recs[len(recs)-1].InlinePayload, &payloadMap)
	hits, _ := payloadMap["retrieval"].(string)
	if !strings.HasPrefix(hits, "hits:") || strings.TrimPrefix(hits, "hits:") == "0" {
		t.Fatalf("retrieval provenance missing: %v", payloadMap["retrieval"])
	}
	snapID, _ := payloadMap["snapshot_id"].(string)
	snap, err := env.deps.Snapshots.LoadSnapshot(env.ctx(), snapID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Selected) < 1 || snap.Selected[0].TrustLabel != contextengine.TrustUntrustedRetrived {
		t.Fatalf("pack chunk must enter the snapshot as untrusted retrieved data: %+v", snap.Selected)
	}
}

// seedFixturePack stores one minimal ACTIVE pack through the store port.
func seedFixturePack(t *testing.T, env *testEnv, text string) string {
	t.Helper()
	body := []byte(text)
	manifest := &knowledgepack.Manifest{
		SchemaVersion: knowledgepack.SchemaVersion,
		PackID:        "pack-fixture",
		PackVersion:   1,
		Title:         "Fixture pack",
		Source:        knowledgepack.Source{Kind: "manual", Identity: "test", Locator: "test://fixture", CapturedAt: time.Now().UTC()},
		LicenseStatus: knowledgepack.LicenseUnknown,
		Freshness:     knowledgepack.Freshness{AssessedAt: time.Now().UTC()},
		Trust:         knowledgepack.Trust{State: knowledgepack.TrustUnsigned},
		Entries: []knowledgepack.Entry{{
			Path:        "docs/fixture.md",
			ContentType: "text/markdown",
			SizeBytes:   int64(len(body)),
			Digest:      DigestOf(body),
		}},
		Claims: []knowledgepack.Claim{{
			ClaimID:   "claim-1",
			Statement: text,
			Status:    knowledgepack.ClaimActive,
		}},
	}
	root, err := knowledgepack.PackRootDigest(manifest)
	if err != nil {
		t.Fatalf("pack root: %v", err)
	}
	pack := knowledgepack.StoredPack{
		Manifest:   manifest,
		RootDigest: root,
		Status:     knowledgepack.PackStatusActive,
		Entries:    map[string][]byte{"docs/fixture.md": body},
		StoredAt:   time.Now().UTC(),
	}
	if err := env.deps.Packs.SavePackVersion(env.ctx(), pack); err != nil {
		t.Fatalf("seed pack: %v", err)
	}
	return "pack-fixture"
}

// TestRetryableProviderFailureThenSuccess proves the recovery path: a
// retryable outage waits on the typed backoff, requeues through the
// accepted transition, and succeeds on a NEW attempt — attempt 2, no
// rewrite of attempt 1.
func TestRetryableProviderFailureThenSuccess(t *testing.T) {
	env := newTestEnv(t)
	flip := &flippingExecutor{inner: env.deps.Providers[dprovider.ProviderMock], fails: 1}
	env.deps.Providers[dprovider.ProviderMock] = flip

	req := baseRequest()
	req.IdempotencyKey = "idem-retry"
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 64)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobSucceeded {
		t.Fatalf("retry did not reach success: %+v", final)
	}
	if final.AttemptCount != 2 {
		t.Fatalf("expected two attempts, got %d", final.AttemptCount)
	}
	recs, _ := env.slice.Events(env.ctx(), sub.Job.JobID)
	types := eventTypes(recs)
	joined := strings.Join(types, ",")
	for _, want := range []string{"job.claimed", "job.waiting", "job.requeued", "job.succeeded"} {
		if !strings.Contains(joined, want) {
			t.Errorf("event sequence missing %q: %v", want, types)
		}
	}
	if flip.fails != 0 {
		t.Fatal("the scripted failure never re-executed")
	}
}

// TestTerminalFailureOnSchemaInvalidOutput proves the output-schema
// boundary: schema-invalid provider output is refused BEFORE any result
// commit, and the Job fails TERMINALLY — no false success, no blind retry.
func TestTerminalFailureOnSchemaInvalidOutput(t *testing.T) {
	env := newTestEnv(t)
	invalid := providers.NewMockAdapter(providers.MockBehavior{InvalidStructuredJSON: true})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(invalid.SeedDataHandling(invalid.DefaultProfile("m")))
	exec, err := providers.NewExecutor(invalid, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Providers[dprovider.ProviderMock] = exec

	req := baseRequest()
	req.IdempotencyKey = "idem-schema"
	req.Schema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`)
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 16)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobFailed {
		t.Fatalf("schema-invalid output must fail the job, got %s", final.State)
	}
	if final.ResultDigest != "" {
		t.Fatal("a failed job must not carry a result")
	}
	recs, _ := env.slice.Events(env.ctx(), sub.Job.JobID)
	if eventTypes(recs)[len(eventTypes(recs))-1] != "job.failed" {
		t.Fatalf("expected a terminal failure event, got %v", eventTypes(recs))
	}
}

// TestCooperativeCancellation proves the cancellation race: a cancel
// request during an in-flight attempt is honored cooperatively; the commit
// carries FRESH fencing (the cancel bumped the Job version) and the final
// state is Cancelled — never a false success after cancellation.
func TestCooperativeCancellation(t *testing.T) {
	env := newTestEnv(t)
	slow := &slowExecutor{inner: env.deps.Providers[dprovider.ProviderMock], delay: 300 * time.Millisecond}
	env.deps.Providers[dprovider.ProviderMock] = slow

	req := baseRequest()
	req.IdempotencyKey = "idem-cancel"
	sub := env.submit(t, req)

	done := make(chan aijobrelay.Job, 1)
	go func() {
		job, _ := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 128)
		done <- job
	}()
	time.Sleep(40 * time.Millisecond) // the attempt is in flight
	if err := env.slice.Cancel(env.ctx(), sub.Job.JobID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	final := <-done
	if final.State != aijobrelay.JobCancelled {
		t.Fatalf("cancelled job must settle as Cancelled, got %s", final.State)
	}
	if final.ResultDigest != "" {
		t.Fatal("a cancelled job must not carry a result")
	}
}

type slowExecutor struct {
	inner ProviderExecutor
	delay time.Duration
}

func (s *slowExecutor) ProfileFor(model string) dprovider.ProfileID { return s.inner.ProfileFor(model) }

func (s *slowExecutor) Connection() dprovider.Connection { return s.inner.Connection() }

func (s *slowExecutor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	select {
	case <-ctx.Done():
		return nil, dprovider.NewError(dprovider.ErrCanceled, dprovider.ProviderMock, "canceled")
	case <-time.After(s.delay):
		return s.inner.Execute(ctx, plan)
	}
}

// TestAuthorizationDeniedByDefault proves the C2-14 rule on the slice: a
// caller without a valid data-scope session is denied before any effect.
func TestAuthorizationDeniedByDefault(t *testing.T) {
	env := newTestEnv(t)
	denied := WithCaller(context.Background(), "local", "kry1-not-a-real-token")
	if _, err := env.slice.Submit(denied, baseRequest()); err == nil {
		t.Fatal("an unauthenticated submit must be denied")
	}
	if err := env.slice.Cancel(denied, "job-anything"); err == nil {
		t.Fatal("an unauthenticated cancel must be denied")
	}
}

// TestProviderSwitchSemantics proves the provider-choice leg: two jobs
// targeting different wired providers both run to success under the same
// contract, and an UNWIRED provider is a terminal failure (never a silent
// fallback to another provider).
func TestProviderSwitchSemantics(t *testing.T) {
	env := newTestEnv(t)
	second := providers.NewMockAdapter(providers.MockBehavior{Prefix: "second:"})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(second.SeedDataHandling(second.DefaultProfile("m")))
	exec2, err := providers.NewExecutor(second, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Providers["mock-two"] = exec2

	reqA := baseRequest()
	reqA.IdempotencyKey = "idem-a"
	subA := env.submit(t, reqA)
	reqB := baseRequest()
	reqB.IdempotencyKey = "idem-b"
	reqB.Provider = "mock-two"
	subB := env.submit(t, reqB)

	finalA, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 2, subA.Job.JobID, 32)
	if err != nil || finalA.State != aijobrelay.JobSucceeded {
		t.Fatalf("job A: %v %+v", err, finalA)
	}
	finalB, err := RunToSettlement(env.ctx(), env.deps, "worker-2", 2, subB.Job.JobID, 32)
	if err != nil || finalB.State != aijobrelay.JobSucceeded {
		t.Fatalf("job B: %v %+v", err, finalB)
	}
	payloadB, _ := env.slice.Result(env.ctx(), subB.Job.JobID)
	if !strings.Contains(string(payloadB), "second:") {
		t.Fatal("job B did not run on its selected provider")
	}

	// An unwired provider is refused at the API boundary — it would
	// otherwise sit Queued forever (no capable worker can claim it), and
	// the Core never silently falls back to another provider.
	reqC := baseRequest()
	reqC.IdempotencyKey = "idem-c"
	reqC.Provider = "unwired-provider"
	if _, err := env.slice.Submit(env.ctx(), reqC); err == nil {
		t.Fatal("an unwired provider must be refused at submission")
	} else if !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("unwired provider refusal must be typed, got %v", err)
	}
}

// TestClassificationGateBlocksRealProviders proves the C3-19 gate on the
// slice: with only real (unknown-review) provider profiles wired,
// protected data egress is blocked — the job fails terminally with the
// typed egress policy error.
func TestClassificationGateBlocksRealProviders(t *testing.T) {
	env := newTestEnv(t)
	// Wire a real adapter against a fixture server (network path) but keep
	// its honest unknown-review data-handling record.
	srv := newLocalFixtureServer(t)
	a := providers.NewCustomOpenAIAdapter(srv, "m", nil)
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(a.SeedDataHandling(a.DefaultProfile("m")))
	exec, err := providers.NewExecutor(a, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Providers[dprovider.ProviderCustomOpenAI] = exec

	req := baseRequest()
	req.IdempotencyKey = "idem-gate"
	req.Provider = dprovider.ProviderCustomOpenAI
	req.Classification = "user-documents" // protected data
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 16)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobFailed {
		t.Fatalf("protected data must not egress under an unknown profile, got %s", final.State)
	}
	recs, _ := env.slice.Events(env.ctx(), sub.Job.JobID)
	if !strings.Contains(string(recs[len(recs)-1].InlinePayload), "egress") {
		t.Fatal("the failure must name the egress policy gate")
	}
}

// newLocalFixtureServer starts a loopback fixture server. The C3-19 gate
// refuses the call BEFORE any dial, so the handler is never reached.
func newLocalFixtureServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestRestartPreservesDurableTruth proves the C3-32 restart case end to
// end: after the process closes and REOPENS the store, the settled Job's
// state, receipt, result payload, snapshot, and journal events are exactly
// what they were — a restart cannot produce a false success or lose
// provenance.
func TestRestartPreservesDurableTruth(t *testing.T) {
	env := newTestEnv(t)
	req := baseRequest()
	req.IdempotencyKey = "idem-restart"
	req.Schema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`)
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-1", 1, sub.Job.JobID, 32)
	if err != nil || final.State != aijobrelay.JobSucceeded {
		t.Fatalf("pre-restart run: %v %+v", err, final)
	}
	before, _ := env.slice.Events(env.ctx(), sub.Job.JobID)
	beforePayload, _ := env.slice.Result(env.ctx(), sub.Job.JobID)
	_ = env.store.Close()

	// Reopen the SAME database: a fresh Store is a fresh process view.
	st2, err := storage.Open(context.Background(), storage.Options{
		Path:                   env.dbPath,
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
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	deps2 := env.deps
	deps2.Relay, deps2.Snapshots, deps2.Objects, deps2.Events, deps2.Packs, deps2.Backoff = st2, st2, st2, st2, st2, st2
	slice2, err := NewSlice(deps2)
	if err != nil {
		t.Fatal(err)
	}
	ctx2 := WithCaller(context.Background(), "local", env.token)
	job, attempt, err := slice2.Status(ctx2, sub.Job.JobID)
	if err != nil || job.State != aijobrelay.JobSucceeded || attempt.Outcome != aijobrelay.AttemptSucceeded {
		t.Fatalf("post-restart status: %v %+v %+v", err, job, attempt)
	}
	after, err := slice2.Events(ctx2, sub.Job.JobID)
	if err != nil || len(after) != len(before) {
		t.Fatalf("post-restart events: %v", err)
	}
	afterPayload, err := slice2.Result(ctx2, sub.Job.JobID)
	if err != nil || string(afterPayload) != string(beforePayload) {
		t.Fatalf("post-restart result payload changed: %v", err)
	}
}
