package main

// The composition root's per-connection custom-executor construction tests
// (IMPL-18): the production customExecutorFactory builds the coded
// C3-29/C3-30 custom connection-protocol executors from a connection
// record's own non-secret posture — both adapters, the honest per-profile
// data-handling record seeded, the shared posture carried — and refuses
// every id outside the coded custom families (the typed wired-check
// refusal stays for the genuinely-unwired case).

import (
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/app"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

func TestCustomExecutorFactoryProtocolRegister(t *testing.T) {
	f := newCustomExecutorFactory(security.UnavailableStore{}, dprovider.NewRegistry(nil))
	for _, provider := range []string{dprovider.ProviderCustomOpenAI, dprovider.ProviderCustomAnthropic} {
		if !f.IsConnectionProtocol(provider) {
			t.Fatalf("%s must name a constructible connection protocol", provider)
		}
	}
	for _, provider := range []string{"custom", "", "openai", "mock", "made-up-protocol"} {
		if f.IsConnectionProtocol(provider) {
			t.Fatalf("%q must not name a constructible connection protocol", provider)
		}
	}
}

func TestCustomExecutorFactoryConstructsBothAdapters(t *testing.T) {
	f := newCustomExecutorFactory(security.UnavailableStore{}, dprovider.NewRegistry(nil))

	// custom-openai: the executor rides the record's own posture.
	exec, ok := f.ConnectionExecutor(dprovider.ProviderCustomOpenAI, "https://custom.example.invalid/v1", "my-model")
	if !ok {
		t.Fatal("the custom-openai construction must succeed")
	}
	var appExec app.ProviderExecutor = exec
	if got := appExec.Connection(); got.BaseURL != "https://custom.example.invalid/v1" || got.AuthKind != dprovider.AuthBearer {
		t.Fatalf("custom-openai posture mismatch: %+v", got)
	}
	profile := appExec.ProfileFor("my-model")
	if profile.Provider != dprovider.ProviderCustomOpenAI || profile.Model != "my-model" || profile.Endpoint == "" {
		t.Fatalf("custom-openai profile mismatch: %+v", profile)
	}
	caps := appExec.Capabilities(profile)
	if !caps.Supported(dprovider.FeatureChat) {
		t.Fatal("the custom adapter's coded contract states chat supported")
	}
	for _, feature := range []dprovider.Feature{
		dprovider.FeatureStreaming, dprovider.FeatureTools, dprovider.FeatureStructured,
		dprovider.FeatureVision, dprovider.FeatureEmbeddings,
	} {
		if caps.Supported(feature) {
			t.Fatalf("unoverridden axis %s must stay unknown (fail closed), reported supported", feature)
		}
	}

	// custom-anthropic: the messages-family posture over the record's own
	// base URL and model.
	execA, ok := f.ConnectionExecutor(dprovider.ProviderCustomAnthropic, "https://relay.example.invalid/api", "claude-model")
	if !ok {
		t.Fatal("the custom-anthropic construction must succeed")
	}
	var appExecA app.ProviderExecutor = execA
	if got := appExecA.Connection(); got.BaseURL != "https://relay.example.invalid/api" || got.AuthKind != dprovider.AuthBearer {
		t.Fatalf("custom-anthropic posture mismatch: %+v", got)
	}
	profileA := appExecA.ProfileFor("claude-model")
	if profileA.Provider != dprovider.ProviderCustomAnthropic || profileA.Model != "claude-model" {
		t.Fatalf("custom-anthropic profile mismatch: %+v", profileA)
	}

	// The honest per-profile record is seeded on construction: the egress
	// gate keys on it (the unknown-base record blocks every classification
	// — the fail-closed posture, never weakened here).
	rec, found := appExec.DataHandling(profile, dprovider.FeatureChat)
	if !found {
		t.Fatal("the constructed profile carries no data-handling record")
	}
	if rec.Deployment != dprovider.FactUnknown || len(rec.AllowedClassifications) != 0 {
		t.Fatalf("the custom record must be the honest unknown-base record: %+v", rec)
	}
	if err := rec.EgressAllowed(time.Now(), dprovider.ClassificationTestFixture); err == nil {
		t.Fatal("the custom record's gate must block every classification, test-fixture included")
	}
}

func TestCustomExecutorFactoryRefusesOutsideIds(t *testing.T) {
	f := newCustomExecutorFactory(security.UnavailableStore{}, dprovider.NewRegistry(nil))
	if _, ok := f.ConnectionExecutor("custom", "https://x.invalid", "m"); ok {
		t.Fatal("the retired custom sentinel id must not construct")
	}
	if _, ok := f.ConnectionExecutor("made-up-protocol", "https://x.invalid", "m"); ok {
		t.Fatal("an unknown protocol id must not construct")
	}
	if _, ok := f.ConnectionExecutor(dprovider.ProviderOpenAI, "https://x.invalid", "m"); ok {
		t.Fatal("a statically wired id must not construct through the factory")
	}
}
