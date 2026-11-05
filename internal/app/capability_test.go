package app

import (
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The F2 capability discovery/status read's app-level pins (IMPL-22, the
// adopted S10 supply's P-S10-01/02 classes as implemented): the coded
// CapabilitySet verbatim (nothing beyond what the Core's coded
// Capabilities() supplies), the effective-profile resolution, the
// decision-port authorization, and the no-leak NOT_FOUND for an identity
// outside the wired set.

func TestProviderCapabilitiesDescriptor(t *testing.T) {
	env := newTestEnv(t)

	res, err := env.slice.ProviderCapabilities(env.ctx(), dprovider.ProviderMock, "")
	if err != nil {
		t.Fatalf("descriptor read: %v", err)
	}
	if res.Provider != dprovider.ProviderMock {
		t.Fatalf("the descriptor names the wired provider, got %q", res.Provider)
	}
	if res.Profile.Provider != dprovider.ProviderMock || res.Profile.Model != "default" {
		t.Fatalf("the registered default profile resolves without a model: %+v", res.Profile)
	}
	// The coded CapabilitySet verbatim — the mock's own coded facts, every
	// unoverridden axis unknown (fail closed; C3-29's boundary: facts,
	// never promises).
	if !res.Capabilities.Chat.Allowed() || !res.Capabilities.StructuredOutput.Allowed() ||
		!res.Capabilities.Streaming.Allowed() || !res.Capabilities.Embeddings.Allowed() {
		t.Fatalf("the mock's coded supported axes must ride the descriptor verbatim: %+v", res.Capabilities)
	}
	for _, f := range []struct {
		name  string
		state dprovider.CapabilityState
	}{
		{"tools", res.Capabilities.Tools},
		{"parallel_tool_calls", res.Capabilities.ParallelToolCalls},
		{"vision", res.Capabilities.Vision},
		{"audio", res.Capabilities.Audio},
		{"batch", res.Capabilities.Batch},
		{"context_caching", res.Capabilities.ContextCaching},
		{"conversation_continuation", res.Capabilities.ConversationContinuation},
	} {
		if f.state != dprovider.CapUnknown {
			t.Fatalf("%s must stay unknown (every unoverridden axis stays unknown — the fail-closed law), got %q", f.name, f.state)
		}
	}
	if res.Capabilities.MaxContextTokens != 0 || res.Capabilities.MaxOutputTokens != 0 {
		t.Fatalf("no limit value is invented (0 = unknown): %+v", res.Capabilities)
	}
	if res.Capabilities.TokenEstimatorRef == "" {
		t.Fatalf("the coded estimator ref rides the descriptor: %+v", res.Capabilities)
	}

	// The optional model resolves the effective profile (the coded
	// ProfileFor resolution — the review field 3's own pattern).
	withModel, err := env.slice.ProviderCapabilities(env.ctx(), dprovider.ProviderMock, "fixture-model")
	if err != nil {
		t.Fatalf("descriptor read with model: %v", err)
	}
	if withModel.Profile.Model != "fixture-model" {
		t.Fatalf("the model value rides the profile resolution: %+v", withModel.Profile)
	}

	// The same eligibility truth as the review's field 3: the descriptor's
	// capabilities match what the pre-submit review reports for the same
	// identity (one capability truth in the Core, never two).
	review, err := env.slice.Review(env.ctx(), ReviewRequest{Task: "capability descriptor parity probe", Provider: dprovider.ProviderMock, Model: "fixture-model"})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if review.ProviderModel.Capabilities == nil {
		t.Fatalf("the review field 3 must carry the capabilities for a wired provider: %+v", review.ProviderModel)
	}
	if *review.ProviderModel.Capabilities != withModel.Capabilities {
		t.Fatalf("the descriptor and the review must carry the SAME coded capability set: %+v vs %+v", withModel.Capabilities, *review.ProviderModel.Capabilities)
	}
}

func TestProviderCapabilitiesUnwired(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.slice.ProviderCapabilities(env.ctx(), "provider-does-not-exist", "")
	if codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("an identity outside the wired set must be the typed NOT_FOUND, got %v", err)
	}
	_, err = env.slice.ProviderCapabilities(env.ctx(), dprovider.ProviderMock, strings.Repeat("m", 257))
	if codeOf(t, err) != errs.CodeValidationFailed {
		t.Fatalf("the model bound must refuse with VALIDATION_FAILED, got %v", err)
	}
}

func TestProviderCapabilitiesAuthorization(t *testing.T) {
	env := newTestEnv(t)
	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	if _, err := env.slice.ProviderCapabilities(denied, dprovider.ProviderMock, ""); err == nil {
		t.Fatal("an unauthorized descriptor read must refuse")
	} else if codeOf(t, err) != errs.CodeAuthzDenied {
		t.Fatalf("expected the typed AUTHZ_DENIED envelope, got %v", err)
	}
	if _, err := env.slice.ProviderCapabilities(env.ctx(), dprovider.ProviderMock, ""); err != nil {
		t.Fatalf("a data-scoped session must read the descriptor: %v", err)
	}
}

// TestProviderCapabilitiesReadsNothingDurable pins the read's boundary:
// the discovery read runs no probe, mutates nothing, and creates no Job —
// the Jobs surface is untouched by a descriptor read (the S10 P-S10-02
// class: ADDITIVE to the M-25 test, never a replacement).
func TestProviderCapabilitiesReadsNothingDurable(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.slice.ProviderCapabilities(env.ctx(), dprovider.ProviderMock, ""); err != nil {
		t.Fatalf("descriptor read: %v", err)
	}
	page, err := env.slice.List(env.ctx(), ListRequest{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Jobs) != 0 {
		t.Fatalf("a capability read must not create Jobs: %+v", page.Jobs)
	}
}
