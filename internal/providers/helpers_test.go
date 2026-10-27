package providers

import (
	"crypto/tls"
	"testing"
	"time"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// verifiedFixtureRecord mirrors the conformance harness's fixture-profile
// record: a VERIFIED record for the FIXTURE endpoint only, allowing the
// test-fixture classification. It inherits nothing from the provider's
// dated-review record: policy metadata is cleared so the fixture record
// makes no provider policy claim.
func verifiedFixtureRecord(a Adapter, now time.Time) dprovider.DataHandlingProfile {
	rec := a.SeedDataHandling(fixtureProfile(a))
	rec.Deployment = dprovider.DeploymentFixture
	rec.TrainingUse = dprovider.FactNo
	rec.AbuseRetention = dprovider.FactNo
	rec.ApplicationStateTTL = dprovider.FactNo
	rec.Subprocessors = dprovider.FactNo
	rec.DeletionEligibility = "deletion_supported"
	rec.PolicyURL = ""
	rec.PolicyVersion = ""
	rec.ReviewDate = now
	rec.Reviewer = "package test fixture record (no provider policy claim)"
	rec.AllowedClassifications = []string{dprovider.ClassificationTestFixture}
	return rec
}

// conformancePlan builds the baseline chat plan used by package tests.
func conformancePlan(a Adapter, baseURL string) *dprovider.RequestPlan {
	return &dprovider.RequestPlan{
		Operation: dprovider.OperationChat,
		Profile:   fixtureProfile(a),
		Connection: dprovider.Connection{
			BaseURL:   baseURL,
			AuthKind:  dprovider.AuthBearer,
			SecretRef: "provider:conformance/fixture-credential",
		},
		System:   "test system text",
		Messages: []dprovider.Message{{Role: dprovider.RoleUser, Parts: []dprovider.Part{{Type: "text", Text: "test input"}}}},
		Metadata: map[string]string{dprovider.ClassificationKey: dprovider.ClassificationTestFixture},
	}
}

func tlsVersion12() uint16 { return tls.VersionTLS12 }

// allAdapters returns every integration under test: the eight direct
// adapters, the two custom families, the explicit local family, and the
// fixture adapter.
func allAdapters(baseURL string) []Adapter {
	ads := newDirectAdapters()
	ads = append(ads,
		NewCustomOpenAIAdapter(baseURL, "m", nil),
		NewCustomAnthropicAdapter(baseURL, "m", "2023-06-01", nil),
		NewLocalAdapter(baseURL, "m", nil),
		NewMockAdapter(MockBehavior{}),
	)
	return ads
}

func mustExecutor(t *testing.T, a Adapter, policy EgressPolicy, reg *dprovider.Registry) *Executor {
	t.Helper()
	e, err := NewExecutor(a, policy, fixtureSecretStore{}, reg)
	if err != nil {
		t.Fatalf("executor build failed: %v", err)
	}
	return e
}
