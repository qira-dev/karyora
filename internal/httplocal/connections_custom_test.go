package httplocal

// The Custom API connection save over the real composition (IMPL-18): the
// wire surface with the per-connection custom-executor factory wired (the
// composition-provided app.ConnectionExecutorFactory port) — a Custom API
// save COMPLETES end to end against the real machinery, the constructed
// executor's gated probe rides the honest egress posture, and the
// genuinely-unwired cases keep their recorded typed refusal. No response
// ever echoes a credential.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/app"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// wireCustomExecutorFactory is the real-construction factory shape the
// composition root carries, built here over the providers package's own
// custom adapter constructors, the env's fake secret store, and a real
// registry (the same construction the composition root performs — the
// test env cannot import cmd/uawp, and its hand-composed deps are the
// repo's own test convention).
type wireCustomExecutorFactory struct {
	secrets security.SecretStore
	reg     *dprovider.Registry
}

func (f wireCustomExecutorFactory) IsConnectionProtocol(provider string) bool {
	switch provider {
	case dprovider.ProviderCustomOpenAI, dprovider.ProviderCustomAnthropic:
		return true
	}
	return false
}

func (f wireCustomExecutorFactory) ConnectionExecutor(provider, baseURL, modelID string) (app.ProviderExecutor, bool) {
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

func customWireEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnvComposed(t, func(deps *app.SliceDeps) {
		deps.ConnectionExecutors = wireCustomExecutorFactory{secrets: deps.Secrets.(security.SecretStore), reg: dprovider.NewRegistry(nil)}
	})
	return env
}

func customWireSaveBody(key, provider, baseURL, model string) string {
	return `{"idempotency_key":"` + key + `","provider":"` + provider + `","base_url":"` + baseURL + `","model_id":"` + model + `","secret":"` + wireTestCanary + `"}`
}

// TestConnectionCustomSaveWire walks the Custom API save end to end over
// the real composition: the save completes (the coded record shape, the
// masked status, no echo), the list and get confirm it, the replay rides
// the Supplied idempotency, the constructed executor's probe refuses typed
// at the fail-closed egress gate (sanitized, no network, no stamp), and
// the genuinely-unwired ids keep the recorded "provider is not wired for
// this core" refusal (the retired "custom" sentinel included). Every
// refusal and every report is credential-free.
func TestConnectionCustomSaveWire(t *testing.T) {
	env := customWireEnv(t)
	h := map[string]string{"Origin": "http://127.0.0.1:1"}

	// The save completes: 200 with the coded view (provider recorded, the
	// masked present status, no echo).
	status, body := env.do(t, "POST", "/v1/connections",
		customWireSaveBody("idem-custom-1", dprovider.ProviderCustomOpenAI, "https://custom.example.invalid/v1", "my-model"),
		env.token, h)
	if status != 200 {
		t.Fatalf("custom save: %d %s", status, body)
	}
	var saved struct {
		Connection struct {
			Record struct {
				ConnectionID string `json:"connection_id"`
				Provider     string `json:"provider"`
				BaseURL      string `json:"base_url"`
				ModelID      string `json:"model_id"`
				SecretRef    string `json:"secret_ref"`
			} `json:"record"`
			SecretStatus string `json:"secret_status"`
			Replayed     bool   `json:"replayed"`
		} `json:"connection"`
		Replayed bool `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		t.Fatalf("save body is not the coded shape: %v\n%s", err, body)
	}
	if saved.Connection.Record.Provider != dprovider.ProviderCustomOpenAI ||
		saved.Connection.Record.BaseURL != "https://custom.example.invalid/v1" ||
		saved.Connection.Record.ModelID != "my-model" {
		t.Fatalf("record mismatch: %s", body)
	}
	if saved.Replayed || saved.Connection.Replayed {
		t.Fatal("the first save must not report a replay")
	}
	if saved.Connection.SecretStatus != "present" {
		t.Fatalf("masked status %q, want present", saved.Connection.SecretStatus)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the save response carries the credential value")
	}
	connID := saved.Connection.Record.ConnectionID
	if connID == "" {
		t.Fatal("the save returned no Core-issued connection id")
	}

	// The replay rides the Supplied idempotency over the wire.
	status, body = env.do(t, "POST", "/v1/connections",
		customWireSaveBody("idem-custom-1", dprovider.ProviderCustomOpenAI, "https://custom.example.invalid/v1", "my-model"),
		env.token, h)
	if status != 200 {
		t.Fatalf("custom save replay: %d %s", status, body)
	}
	if !strings.Contains(string(body), `"replayed":true`) {
		t.Fatalf("the replayed save must report replayed:true: %s", body)
	}

	// The list and get confirm the record independently (the authoritative
	// read), credential-free.
	status, body = env.do(t, "GET", "/v1/connections", "", env.token, h)
	if status != 200 || !strings.Contains(string(body), connID) || !strings.Contains(string(body), dprovider.ProviderCustomOpenAI) {
		t.Fatalf("list must carry the saved custom connection: %d %s", status, body)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the list carries the credential value")
	}
	status, body = env.do(t, "GET", "/v1/connections/"+connID, "", env.token, h)
	if status != 200 || !strings.Contains(string(body), `"secret_status":"present"`) {
		t.Fatalf("get must carry the masked status: %d %s", status, body)
	}

	// The saved connection's test: the CONSTRUCTED executor probes — the
	// honest unknown data-handling record refuses every classification
	// typed (egress_policy), sanitized, no network, no tested stamp.
	status, body = env.do(t, "POST", "/v1/connections/test",
		`{"connection_id":"`+connID+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("saved custom test: %d %s", status, body)
	}
	var report struct {
		Report struct {
			OK            bool   `json:"ok"`
			ErrorKind     string `json:"error_kind"`
			Provider      string `json:"provider"`
			CorrelationID string `json:"correlation_id"`
		} `json:"report"`
		Record *struct {
			TestedAt string `json:"tested_at"`
		} `json:"record"`
	}
	if err := json.Unmarshal([]byte(body), &report); err != nil {
		t.Fatalf("test body is not the coded shape: %v\n%s", err, body)
	}
	if report.Report.OK || report.Report.ErrorKind != "egress_policy" {
		t.Fatalf("the gated custom probe must refuse typed: %s", body)
	}
	if report.Report.Provider != dprovider.ProviderCustomOpenAI || report.Report.CorrelationID == "" {
		t.Fatalf("the report must carry the typed identity and correlation id: %s", body)
	}
	if report.Record != nil && report.Record.TestedAt != "" {
		t.Fatal("a refused probe stamped no tested state")
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the test response carries the credential value")
	}

	// The genuinely-unwired cases keep the recorded typed refusal: the
	// retired "custom" sentinel id and an unknown protocol id.
	for _, provider := range []string{"custom", "made-up-protocol"} {
		status, body = env.do(t, "POST", "/v1/connections",
			customWireSaveBody("idem-unwired-"+provider, provider, "https://custom.example.invalid/v1", "my-model"),
			env.token, h)
		if status != 400 || !strings.Contains(string(body), "VALIDATION_FAILED") || !strings.Contains(string(body), "provider is not wired for this core") {
			t.Fatalf("unwired %q save must refuse typed: %d %s", provider, status, body)
		}
		if strings.Contains(string(body), wireTestCanary) {
			t.Fatal("a refusal carries the credential value")
		}
	}

	// A custom save without its destination refuses the base_url field law
	// (the constructed posture carries no recorded default).
	status, body = env.do(t, "POST", "/v1/connections",
		`{"idempotency_key":"idem-custom-baseless","provider":"`+dprovider.ProviderCustomOpenAI+`","model_id":"my-model","secret":"`+wireTestCanary+`"}`,
		env.token, h)
	if status != 400 || !strings.Contains(string(body), `"field":"base_url"`) {
		t.Fatalf("baseless custom save must refuse the base_url field: %d %s", status, body)
	}
}
