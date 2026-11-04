package app

// The per-connection custom-executor construction tests (IMPL-18): the
// coded custom connection-protocol adapters (C3-29/C3-30) reach the
// connection surface through the composition-provided
// ConnectionExecutorFactory port — the app layer cannot construct
// executors (the dependency map's own rule), so the tests exercise the
// PORT: a real-construction factory over the providers package's own
// custom adapter constructors (the composition root's production shape)
// for the save/gate/canary pins, and a scripted factory with a recording
// executor for the port-consumption proof (the surface drives the
// CONSTRUCTED executor, not merely the wired map).

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// testCustomExecutorFactory is the real-construction factory shape the
// composition root carries (cmd/uawp's customExecutorFactory), built here
// over the same providers constructors, the test fake secret store, and a
// real registry — the honest production mechanics, no scripted shortcut.
type testCustomExecutorFactory struct {
	secrets security.SecretStore
	reg     *dprovider.Registry
}

func newTestCustomExecutorFactory(secrets security.SecretStore) testCustomExecutorFactory {
	return testCustomExecutorFactory{secrets: secrets, reg: dprovider.NewRegistry(nil)}
}

func (f testCustomExecutorFactory) IsConnectionProtocol(provider string) bool {
	switch provider {
	case dprovider.ProviderCustomOpenAI, dprovider.ProviderCustomAnthropic:
		return true
	}
	return false
}

func (f testCustomExecutorFactory) ConnectionExecutor(provider, baseURL, modelID string) (ProviderExecutor, bool) {
	if !f.IsConnectionProtocol(provider) {
		return nil, false
	}
	var a providers.Adapter
	switch provider {
	case dprovider.ProviderCustomOpenAI:
		a = providers.NewCustomOpenAIAdapter(baseURL, modelID, nil)
	case dprovider.ProviderCustomAnthropic:
		a = providers.NewCustomAnthropicAdapter(baseURL, modelID, "", nil)
	default:
		return nil, false
	}
	f.reg.Upsert(a.SeedDataHandling(a.DefaultProfile(modelID)))
	exec, err := providers.NewExecutor(a, providers.DefaultEgressPolicy(providers.CoreLocal), f.secrets, f.reg)
	if err != nil {
		return nil, false
	}
	return exec, true
}

// customEnv extends the connection env with the real-construction factory
// wired (the recorded unwired posture stays whenever the factory is nil).
func customEnv(t *testing.T) *testEnv {
	t.Helper()
	env := connEnv(t)
	env.deps.ConnectionExecutors = newTestCustomExecutorFactory(env.deps.Secrets.(security.SecretStore))
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	return env
}

func customSaveReq(key, provider string) SaveConnectionRequest {
	return SaveConnectionRequest{
		IdempotencyKey: key,
		Provider:       provider,
		BaseURL:        "https://custom.example.invalid/v1",
		ModelID:        "my-model",
		Secret:         []byte(connTestCanary),
	}
}

// TestConnectionCustomSaveCompletes pins the honest unblocking: a Custom
// API connection save COMPLETES when the Core can construct its executor
// per the coded adapter contracts — the record stored, the credential
// written exactly once, the view masked, the replay semantics riding
// unchanged — for BOTH coded custom adapters. The canary never appears on
// any read surface.
func TestConnectionCustomSaveCompletes(t *testing.T) {
	env := customEnv(t)
	secrets := env.deps.Secrets.(*fakeConnectionSecrets)

	var lastResult SaveConnectionResult
	for _, provider := range []string{dprovider.ProviderCustomOpenAI, dprovider.ProviderCustomAnthropic} {
		key := "idem-custom-" + provider
		req := customSaveReq(key, provider)
		res, err := env.slice.SaveConnection(env.ctx(), req)
		if err != nil {
			t.Fatalf("custom save %s: %v", provider, err)
		}
		if res.Replayed {
			t.Fatalf("first save of %s reported Replayed", provider)
		}
		rec := res.Connection.Record
		if rec.Provider != provider {
			t.Fatalf("provider recorded as %q, want %q", rec.Provider, provider)
		}
		if rec.BaseURL != "https://custom.example.invalid/v1" {
			t.Fatalf("base URL recorded as %q", rec.BaseURL)
		}
		if rec.ModelID != "my-model" {
			t.Fatalf("model recorded as %q", rec.ModelID)
		}
		if res.Connection.SecretStatus != string(security.SecretPresent) {
			t.Fatalf("secret status %q, want the masked present status", res.Connection.SecretStatus)
		}
		if strings.Contains(rec.SecretRef, connTestCanary) || rec.SecretRef == "" {
			t.Fatalf("secret reference must be the handle only, got %q", rec.SecretRef)
		}
		lastResult = res

		// The Supplied idempotency rides unchanged: a replayed custom save
		// returns the original connection and never writes a second
		// credential.
		replay, err := env.slice.SaveConnection(env.ctx(), req)
		if err != nil {
			t.Fatalf("custom save replay %s: %v", provider, err)
		}
		if !replay.Replayed {
			t.Fatalf("replayed save of %s did not report Replayed", provider)
		}
		if replay.Connection.Record.ConnectionID != res.Connection.Record.ConnectionID {
			t.Fatalf("replay returned a different connection for %s", provider)
		}
	}
	if got := secrets.writeCount(); got != 2 {
		t.Fatalf("credential write count %d, want exactly one per custom save (2)", got)
	}

	// The canary pin: the credential value never appears in the save
	// result, the list, or the get view.
	blob := marshalForCanary(t, lastResult)
	list, err := env.slice.ListConnections(env.ctx(), ListConnectionsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	blob += marshalForCanary(t, list)
	view, err := env.slice.GetConnection(env.ctx(), lastResult.Connection.Record.ConnectionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	blob += marshalForCanary(t, view)
	if strings.Contains(blob, connTestCanary) {
		t.Fatal("the credential canary leaked onto a read surface")
	}
}

// TestConnectionCustomSaveRefusals pins the retained law set: a custom
// connection without its destination refuses the typed base_url field law
// (the constructed posture carries no recorded default); a genuinely-
// unwired provider keeps the recorded "provider is not wired for this
// core" refusal — including the retired "custom" sentinel id and the
// factory-absent posture (fail closed; nothing invented to bypass it).
// Every refusal changes nothing.
func TestConnectionCustomSaveRefusals(t *testing.T) {
	env := customEnv(t)
	secrets := env.deps.Secrets.(*fakeConnectionSecrets)
	writesBefore := secrets.writeCount()

	rows := []struct {
		name       string
		provider   string
		baseURL    string
		wantField  string
		wantReason string
	}{
		{"custom without base url", dprovider.ProviderCustomOpenAI, "", "base_url", ""},
		{"retired custom sentinel id", "custom", "https://custom.example.invalid/v1", "provider", "provider is not wired for this core"},
		{"unknown protocol id", "made-up-protocol", "https://custom.example.invalid/v1", "provider", "provider is not wired for this core"},
	}
	for _, row := range rows {
		req := customSaveReq("idem-refusal-"+row.name, row.provider)
		req.BaseURL = row.baseURL
		_, err := env.slice.SaveConnection(env.ctx(), req)
		if err == nil {
			t.Fatalf("%s: save must refuse", row.name)
		}
		envErr, ok := err.(*EnvelopeError)
		if !ok {
			t.Fatalf("%s: refusal is not the typed envelope: %v", row.name, err)
		}
		if envErr.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: code %q, want VALIDATION_FAILED", row.name, envErr.Envelope.Code)
		}
		field, _ := envErr.Envelope.Data["field"].(string)
		if field != row.wantField {
			t.Fatalf("%s: refusal field %q, want %q", row.name, field, row.wantField)
		}
		if row.wantReason != "" {
			reason, _ := envErr.Envelope.Data["reason"].(string)
			if reason != row.wantReason {
				t.Fatalf("%s: refusal reason %q, want %q", row.name, reason, row.wantReason)
			}
		}
	}
	if got := secrets.writeCount(); got != writesBefore {
		t.Fatalf("a refusal wrote a credential (%d -> %d)", writesBefore, got)
	}

	// The factory-absent posture: the recorded unwired state for the
	// custom families (nil ConnectionExecutors — fail closed).
	env.deps.ConnectionExecutors = nil
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	_, err = env.slice.SaveConnection(env.ctx(), customSaveReq("idem-refusal-nil", dprovider.ProviderCustomOpenAI))
	if err == nil {
		t.Fatal("a nil factory must keep the recorded unwired refusal")
	}
	envErr, ok := err.(*EnvelopeError)
	if !ok || envErr.Envelope.Code != errs.CodeValidationFailed {
		t.Fatalf("nil-factory refusal not the typed envelope: %v", err)
	}
	reason, _ := envErr.Envelope.Data["reason"].(string)
	if reason != "provider is not wired for this core" {
		t.Fatalf("nil-factory refusal reason %q", reason)
	}
}

// TestConnectionCustomTestGatedAndSanitized pins the probe path over the
// CONSTRUCTED executor: the full gate set applies unchanged, so the custom
// profile's honest unknown data-handling record blocks the probe typed
// (egress_policy) with no network dialed — the recorded C3-19 posture, the
// classification allowlists untouched. The sanitized report carries the
// typed class and the correlation id, never a secret; a refusal stamps no
// tested state; a draft without its destination refuses the base_url field
// law.
func TestConnectionCustomTestGatedAndSanitized(t *testing.T) {
	env := customEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), customSaveReq("idem-custom-test", dprovider.ProviderCustomOpenAI))
	if err != nil {
		t.Fatalf("custom save: %v", err)
	}
	connID := saved.Connection.Record.ConnectionID

	// The saved connection's test: the constructed executor's seeded
	// record refuses every classification (the honest unknown profile) —
	// the typed egress refusal, sanitized, no tested stamp.
	res, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: connID})
	if err != nil {
		t.Fatalf("saved custom test: %v", err)
	}
	if res.Report.OK {
		t.Fatal("the gated custom probe must not report success")
	}
	if res.Report.ErrorKind != string(dprovider.ErrEgressPolicy) {
		t.Fatalf("error kind %q, want egress_policy", res.Report.ErrorKind)
	}
	if res.Report.Provider != dprovider.ProviderCustomOpenAI || res.Report.ModelID != "my-model" {
		t.Fatalf("report identity mismatch: %+v", res.Report)
	}
	if res.Report.CorrelationID == "" {
		t.Fatal("the report carries no correlation id")
	}
	if res.Record != nil && res.Record.Tested() {
		t.Fatal("a refused probe stamped no tested state")
	}
	if strings.Contains(marshalForCanary(t, res), connTestCanary) {
		t.Fatal("the credential canary leaked into the test report")
	}

	// The unsaved draft's test: the same gate, the transient secret
	// dropped after the single probe.
	draft, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{
		Provider: dprovider.ProviderCustomAnthropic,
		BaseURL:  "https://custom.example.invalid/v1",
		ModelID:  "my-model",
		Secret:   []byte(connTestCanary),
	})
	if err != nil {
		t.Fatalf("draft custom test: %v", err)
	}
	if draft.Report.OK || draft.Report.ErrorKind != string(dprovider.ErrEgressPolicy) {
		t.Fatalf("draft test outcome mismatch: %+v", draft.Report)
	}
	if draft.Record != nil {
		t.Fatal("an unsaved draft's test recorded nothing")
	}

	// A draft without its destination: the typed base_url field law (the
	// constructed posture has no recorded default).
	_, err = env.slice.TestConnection(env.ctx(), TestConnectionRequest{
		Provider: dprovider.ProviderCustomOpenAI,
		ModelID:  "my-model",
		Secret:   []byte(connTestCanary),
	})
	if err == nil {
		t.Fatal("a baseless custom draft test must refuse")
	}
	envErr, ok := err.(*EnvelopeError)
	if !ok || envErr.Envelope.Code != errs.CodeValidationFailed {
		t.Fatalf("baseless draft refusal not the typed envelope: %v", err)
	}
	if field, _ := envErr.Envelope.Data["field"].(string); field != "base_url" {
		t.Fatalf("refusal field %q, want base_url", envErr.Envelope.Data["field"])
	}
}

// recordingCustomExecutor is a fully scripted executor the scripted
// factory hands back: it records the probe calls (the port-consumption
// proof's witness that the connection surface drives the CONSTRUCTED
// executor) and answers a scripted SUCCESS, so the full post-probe path
// (the TC-1 stamp over the constructed executor) is proven
// construction-agnostic. The REAL construction mechanics — the adapters,
// the seeded records, the egress gate — are pinned by the real-factory
// tests above.
type recordingCustomExecutor struct {
	mu         sync.Mutex
	probeCalls int
	lastPlan   *dprovider.RequestPlan
}

func (r *recordingCustomExecutor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	return nil, dprovider.NewError(dprovider.ErrUnsupported, "scripted", "not exercised by this test")
}

func (r *recordingCustomExecutor) ProfileFor(model string) dprovider.ProfileID {
	return dprovider.ProfileID{Provider: dprovider.ProviderCustomOpenAI, Endpoint: "chat-completions", Model: model}
}

func (r *recordingCustomExecutor) Connection() dprovider.Connection {
	return dprovider.Connection{BaseURL: "https://scripted.example.invalid", AuthKind: dprovider.AuthBearer}
}

func (r *recordingCustomExecutor) Capabilities(profile dprovider.ProfileID) dprovider.CapabilitySet {
	return dprovider.CapabilitySet{Chat: dprovider.CapSupported}
}

func (r *recordingCustomExecutor) DataHandling(profile dprovider.ProfileID, feature dprovider.Feature) (dprovider.DataHandlingProfile, bool) {
	return dprovider.DataHandlingProfile{}, false
}

func (r *recordingCustomExecutor) TestConnection(ctx context.Context, plan *dprovider.RequestPlan) error {
	return nil
}

func (r *recordingCustomExecutor) TestConnectionWith(ctx context.Context, secrets security.SecretStore, plan *dprovider.RequestPlan) error {
	r.mu.Lock()
	r.probeCalls++
	r.lastPlan = plan
	r.mu.Unlock()
	return nil
}

// scriptedCustomFactory answers exactly one provider id with a scripted
// executor: the port-consumption proof's factory.
type scriptedCustomFactory struct {
	provider string
	executor ProviderExecutor
}

func (f scriptedCustomFactory) IsConnectionProtocol(provider string) bool {
	return provider == f.provider
}

func (f scriptedCustomFactory) ConnectionExecutor(provider, baseURL, modelID string) (ProviderExecutor, bool) {
	if provider != f.provider {
		return nil, false
	}
	return f.executor, true
}

// TestConnectionCustomTestDrivesConstructedExecutor proves the surface
// consumes the PORT: a scripted factory's recording executor receives the
// probe (the constructed executor, not a wired-map lookup), carrying the
// record's own base URL and exact model id; its success stamps the tested
// state exactly like every delivered provider (the TC-1 discipline is
// construction-agnostic).
func TestConnectionCustomTestDrivesConstructedExecutor(t *testing.T) {
	env := customEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), customSaveReq("idem-custom-port", dprovider.ProviderCustomOpenAI))
	if err != nil {
		t.Fatalf("custom save: %v", err)
	}
	connID := saved.Connection.Record.ConnectionID

	witness := &recordingCustomExecutor{}
	env.deps.ConnectionExecutors = scriptedCustomFactory{provider: dprovider.ProviderCustomOpenAI, executor: witness}
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice

	res, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: connID})
	if err != nil {
		t.Fatalf("test over the scripted factory: %v", err)
	}
	if witness.probeCalls != 1 {
		t.Fatalf("the constructed executor received %d probes, want exactly 1", witness.probeCalls)
	}
	plan := witness.lastPlan
	if plan == nil {
		t.Fatal("the probe carried no plan")
	}
	if plan.Connection.BaseURL != "https://custom.example.invalid/v1" {
		t.Fatalf("the probe plan's base URL %q, want the record's own", plan.Connection.BaseURL)
	}
	if plan.Profile.Model != "my-model" {
		t.Fatalf("the probe plan's model %q, want the record's exact model id", plan.Profile.Model)
	}
	if plan.Profile.Provider != dprovider.ProviderCustomOpenAI {
		t.Fatalf("the probe plan's provider %q", plan.Profile.Provider)
	}
	if !res.Report.OK {
		t.Fatalf("the scripted executor's probe must drive the full path to the report: %+v", res.Report)
	}
	if res.Record == nil || !res.Record.Tested() {
		t.Fatal("a successful constructed-executor test stamps no tested state")
	}
}

// marshalForCanary serializes a value for the canary scan (the existing
// canary convention: the JSON of every read surface, scanned as a blob).
func marshalForCanary(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canary marshal: %v", err)
	}
	return string(b)
}
