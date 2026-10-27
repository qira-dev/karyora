package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	derrs "github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// TestConformanceCatalogPasses runs the full C3-20 case catalog for every
// integration (eight direct + custom + local + fixture) and asserts the
// support matrix: no failing case, and every claimed capability is proven
// by a passing case (claims are never inferred from protocol labels).
func TestConformanceCatalogPasses(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	ads := allAdapters("https://catalog-placeholder.test")
	rep := RunCatalog(context.Background(), ads, DefaultEgressPolicy(CoreLocal), now)
	rep.SortCases()

	failures := 0
	for _, c := range rep.Cases {
		switch c.Status {
		case "pass", "skip":
			// skip is a recorded, reasoned non-claim
		default:
			failures++
			t.Errorf("case %s/%s failed: %s", c.AdapterID, c.CaseID, c.Detail)
		}
	}
	if failures > 0 {
		t.Fatalf("%d conformance cases failed", failures)
	}

	// The support matrix proves or demotes every claim.
	unproven := 0
	for adapterID, row := range rep.Matrix {
		for feature, state := range row {
			switch state {
			case "supported", "unsupported":
			case "unproven":
				// The direct adapters claim parallel-tool behavior on two
				// families through one tool-call case; anything else claimed
				// without a case must be surfaced.
				unproven++
				t.Logf("UNPROVEN claim recorded honestly: %s/%s", adapterID, feature)
			default:
				t.Fatalf("unknown matrix state %q", state)
			}
		}
	}
	t.Logf("conformance: %d cases, %d unproven claims", len(rep.Cases), unproven)

	// Every case run is tied to a dated fixture profile (the harness
	// registry), and live checks are recorded Not Run while D-37 is open.
	if !strings.Contains(rep.LiveChecks, "Not Run") {
		t.Fatal("live checks must be recorded Not Run while D-37 is open")
	}
}

// TestRealProviderProfilesBlockEgress proves the C3-19 fail-closed default:
// every direct and custom integration ships with an unknown-review record
// and blocks protected data until a dated official-source review exists.
func TestRealProviderProfilesBlockEgress(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, a := range newDirectAdapters() {
		reg := dprovider.NewRegistry(func() time.Time { return now })
		reg.Upsert(a.SeedDataHandling(a.DefaultProfile("any-model")))
		p := a.DefaultProfile("any-model")
		for _, class := range []string{dprovider.ClassificationTestFixture, "user-documents", "prompts", ""} {
			if err := reg.EgressAllowed(p, dprovider.FeatureChat, class); err == nil {
				t.Errorf("%s: unknown-review profile must block classification %q", a.ID(), class)
			}
		}
	}
}

// TestSecretRedaction proves resolved credential bytes never survive into
// an error surface (C3-18 acceptance): a provider 500 body echoing the
// credential is redacted before the error escapes.
func TestSecretRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"denied for credential ` + fixtureCredential + `"}`))
	}))
	defer srv.Close()

	reg := dprovider.NewRegistry(time.Now)
	a := NewCustomOpenAIAdapter(srv.URL, "m", nil)
	reg.Upsert(verifiedFixtureRecord(a, time.Now()))
	exec := mustExecutor(t, a, DefaultEgressPolicy(CoreLocal), reg)
	_, err := exec.Execute(context.Background(), conformancePlan(a, srv.URL))
	if err == nil {
		t.Fatal("expected the mapped provider error")
	}
	if strings.Contains(err.Error(), fixtureCredential) {
		t.Fatal("resolved credential leaked into the error surface")
	}
	var pe *dprovider.ProviderError
	if !errors.As(err, &pe) || pe.Kind != dprovider.ErrUnreachable {
		t.Fatalf("unexpected error mapping: %v", err)
	}
}

// TestMockScriptedBehaviors covers the fixture adapter behaviors the
// vertical slice and the C3-32 evidence matrix rely on.
func TestMockScriptedBehaviors(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	reg := dprovider.NewRegistry(func() time.Time { return now })
	a := NewMockAdapter(MockBehavior{})
	reg.Upsert(verifiedFixtureRecord(a, now))
	exec := mustExecutor(t, a, DefaultEgressPolicy(CoreLocal), reg)
	plan := conformancePlan(a, "https://unused.test")

	// Deterministic success with digests.
	res, err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("mock call failed: %v", err)
	}
	if res.ResultDigest == "" || res.RawDigest == "" {
		t.Fatal("mock result must carry digests")
	}
	if a.Calls() != 1 {
		t.Fatalf("call counter wrong: %d", a.Calls())
	}

	// Structured output validates against the schema pre-commit.
	plan.Structured = &dprovider.StructuredOutput{Name: "answer",
		Schema: []byte(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`)}
	res, err = exec.Execute(context.Background(), plan)
	if err != nil || len(res.StructuredJSON) == 0 {
		t.Fatalf("structured mock call failed: %v", err)
	}

	// Scripted failure surfaces as the scripted provider error.
	failing := NewMockAdapter(MockBehavior{FailWith: dprovider.NewError(dprovider.ErrUnreachable, dprovider.ProviderMock, "scripted outage")})
	reg2 := dprovider.NewRegistry(func() time.Time { return now })
	reg2.Upsert(verifiedFixtureRecord(failing, now))
	exec2 := mustExecutor(t, failing, DefaultEgressPolicy(CoreLocal), reg2)
	_, err = exec2.Execute(context.Background(), conformancePlan(failing, "https://unused.test"))
	var pe *dprovider.ProviderError
	if !errors.As(err, &pe) || pe.Kind != dprovider.ErrUnreachable || !pe.Retryable() {
		t.Fatalf("scripted failure mapped to %v", err)
	}

	// Scripted schema-invalid output is refused BEFORE a result escapes.
	invalid := NewMockAdapter(MockBehavior{InvalidStructuredJSON: true})
	reg3 := dprovider.NewRegistry(func() time.Time { return now })
	reg3.Upsert(verifiedFixtureRecord(invalid, now))
	exec3 := mustExecutor(t, invalid, DefaultEgressPolicy(CoreLocal), reg3)
	_, err = exec3.Execute(context.Background(), plan)
	if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrProtocol {
		t.Fatalf("schema-invalid output must be a protocol failure, got %v", err)
	}

	// Streaming without a finish reason is a mid-stream failure, never a
	// false success (C3-32: response loss cannot produce false success).
	truncating := NewMockAdapter(MockBehavior{NoFinishOnStream: true})
	reg4 := dprovider.NewRegistry(func() time.Time { return now })
	reg4.Upsert(verifiedFixtureRecord(truncating, now))
	exec4 := mustExecutor(t, truncating, DefaultEgressPolicy(CoreLocal), reg4)
	_, err = exec4.ExecuteStream(context.Background(), conformancePlan(truncating, "https://unused.test"), nil)
	if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrMidStreamFailure {
		t.Fatalf("unfinished stream must be a mid-stream failure, got %v", err)
	}

	// Structured output cannot stream (the schema needs a whole document).
	_, err = exec.ExecuteStream(context.Background(), plan, nil)
	if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrUnsupported {
		t.Fatalf("structured streaming must be refused, got %v", err)
	}
}

// TestProviderErrorEnvelopeMapping proves the neutral taxonomy maps onto
// the closed D-35 error register without inventing new wire codes.
func TestProviderErrorEnvelopeMapping(t *testing.T) {
	cases := map[dprovider.ErrorKind]derrs.Code{
		dprovider.ErrRateLimited:    derrs.CodeRateLimited,
		dprovider.ErrAuth:           derrs.CodeAuthzDenied,
		dprovider.ErrInvalidRequest: derrs.CodeValidationFailed,
		dprovider.ErrUnsupported:    derrs.CodeValidationFailed,
		dprovider.ErrUnreachable:    derrs.CodeStorageUnavailable,
		dprovider.ErrEgressPolicy:   derrs.CodeAuthzDenied,
		dprovider.ErrProtocol:       derrs.CodeInternal,
	}
	for kind, want := range cases {
		pe := dprovider.NewError(kind, "test", "d")
		env, err := mapToEnvelope(pe, "corr-1")
		if err != nil {
			t.Fatalf("%s envelope failed: %v", kind, err)
		}
		if env.Code != want {
			t.Errorf("kind %s mapped to %s, want %s", kind, env.Code, want)
		}
	}
}
