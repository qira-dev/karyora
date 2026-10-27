package provider

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validPlan() *RequestPlan {
	return &RequestPlan{
		Operation: OperationChat,
		Profile:   ProfileID{Provider: ProviderMock, Endpoint: "fixture", Model: "m"},
		Connection: Connection{
			BaseURL: "https://example.test/v1", AuthKind: AuthNone,
		},
		Messages: []Message{{Role: RoleUser, Parts: []Part{{Type: "text", Text: "hello"}}}},
	}
}

func TestRequestPlanValidation(t *testing.T) {
	p := validPlan()
	if err := p.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}

	// credentials never travel in URLs (C3-18)
	p2 := validPlan()
	p2.Connection.BaseURL = "https://user:secret@example.test/v1"
	if err := p2.Validate(); err == nil {
		t.Fatal("userinfo URL accepted")
	}

	// auth without a secret reference is invalid; an inline value has no home
	p3 := validPlan()
	p3.Connection.AuthKind = AuthBearer
	if err := p3.Validate(); err == nil {
		t.Fatal("bearer auth without a secret reference accepted")
	}

	// unknown operation refused
	p4 := validPlan()
	p4.Operation = "magic"
	if err := p4.Validate(); err == nil {
		t.Fatal("unknown operation accepted")
	}

	// image parts require inline bytes
	p5 := validPlan()
	p5.Messages[0].Parts = append(p5.Messages[0].Parts, Part{Type: "image", ImageMIME: "image/png"})
	if err := p5.Validate(); err == nil {
		t.Fatal("image part without bytes accepted")
	}

	// bounds
	p6 := validPlan()
	p6.Messages = make([]Message, MaxMessages+1)
	if err := p6.Validate(); err == nil {
		t.Fatal("message bound not enforced")
	}
}

func TestCapabilityUnknownMeansUnsupported(t *testing.T) {
	caps := CapabilitySet{Chat: CapSupported, Streaming: CapUnknown, Tools: CapUnsupported}
	if !caps.Supported(FeatureChat) {
		t.Fatal("supported capability not allowed")
	}
	if caps.Supported(FeatureStreaming) {
		t.Fatal("UNKNOWN capability must mean unsupported (fail closed)")
	}
	if caps.Supported(FeatureTools) {
		t.Fatal("unsupported capability allowed")
	}
	if caps.Supported(Feature("nonexistent")) {
		t.Fatal("unknown feature allowed")
	}
}

func TestErrorRetryClasses(t *testing.T) {
	retryable := []ErrorKind{ErrRateLimited, ErrTimeout, ErrUnreachable, ErrMidStreamFailure}
	for _, k := range retryable {
		if !k.Retryable() {
			t.Errorf("kind %s should be retryable", k)
		}
	}
	terminal := []ErrorKind{ErrAuth, ErrPermission, ErrInvalidRequest, ErrUnsupported, ErrEgressPolicy, ErrTLS, ErrResponseTooLarge, ErrProtocol, ErrCanceled}
	for _, k := range terminal {
		if k.Retryable() {
			t.Errorf("kind %s must not be retryable", k)
		}
	}
}

func fixedNow() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }

func verifiedProfile() DataHandlingProfile {
	return DataHandlingProfile{
		Profile:                ProfileID{Provider: ProviderMock, Endpoint: "fixture", Model: "m"},
		Deployment:             DeploymentLocal,
		TrainingUse:            FactNo,
		AbuseRetention:         FactNo,
		ApplicationStateTTL:    FactNo,
		Subprocessors:          FactNo,
		DeletionEligibility:    "deletion_supported",
		ReviewDate:             fixedNow().Add(-24 * time.Hour),
		Reviewer:               "test",
		AllowedClassifications: []string{ClassificationTestFixture},
	}
}

func TestEgressGateFailClosed(t *testing.T) {
	// A current, complete profile allows its recorded classification.
	p := verifiedProfile()
	if err := p.EgressAllowed(fixedNow(), ClassificationTestFixture); err != nil {
		t.Fatalf("fixture classification refused: %v", err)
	}

	// Protected classifications block.
	if err := p.EgressAllowed(fixedNow(), "user-documents"); err == nil {
		t.Fatal("protected classification allowed by a profile that does not record it")
	}

	// An empty classification is treated as protected (fail closed).
	if err := p.EgressAllowed(fixedNow(), ""); err == nil {
		t.Fatal("empty classification allowed")
	}

	// UNKNOWN facts block, even with a dated review.
	unknown := verifiedProfile()
	unknown.TrainingUse = FactUnknown
	if err := unknown.EgressAllowed(fixedNow(), ClassificationTestFixture); err == nil {
		t.Fatal("unknown training-use fact allowed egress")
	}

	// No dated review blocks everything.
	never := verifiedProfile()
	never.ReviewDate = time.Time{}
	never.Reviewer = ""
	if err := never.EgressAllowed(fixedNow(), ClassificationTestFixture); err == nil {
		t.Fatal("never-reviewed profile allowed egress")
	}

	// STALE reviews block exactly like unknown ones (vendor changes
	// invalidate the affected profile claims).
	stale := verifiedProfile()
	stale.ReviewDate = fixedNow().Add(-ProfileRecheckInterval - time.Hour)
	if err := stale.EgressAllowed(fixedNow(), ClassificationTestFixture); err == nil {
		t.Fatal("stale profile allowed egress")
	}

	// Deployment conflicts block: a LOCAL deployment recorded for a
	// non-local provider family is a contradiction.
	conflict := verifiedProfile()
	conflict.Profile = ProfileID{Provider: ProviderOpenAI, Endpoint: "chat-completions", Model: "m"}
	conflict.Deployment = DeploymentLocal
	if err := conflict.EgressAllowed(fixedNow(), ClassificationTestFixture); err == nil {
		t.Fatal("a local deployment recorded for a remote provider family must conflict-block")
	}
}

func TestRegistryPrecedenceAndFailClosed(t *testing.T) {
	reg := NewRegistry(fixedNow)
	p := ProfileID{Provider: ProviderMock, Endpoint: "fixture", Model: "m"}

	// No record at all: blocks.
	if err := reg.EgressAllowed(p, FeatureChat, ClassificationTestFixture); err == nil {
		t.Fatal("unknown profile allowed egress")
	} else {
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Kind != ErrEgressPolicy {
			t.Fatalf("gate must surface the typed egress-policy error, got %v", err)
		}
	}

	// Whole-profile record (unknown review): still blocks.
	reg.Upsert(DataHandlingProfile{Profile: p, TrainingUse: FactUnknown})
	if err := reg.EgressAllowed(p, FeatureChat, ClassificationTestFixture); err == nil {
		t.Fatal("unknown-review record allowed egress")
	}

	// Feature-specific VERIFIED record takes precedence and allows.
	featureRecord := verifiedProfile()
	featureRecord.FeatureScope = FeatureStructured
	featureRecord.Profile = p
	reg.Upsert(featureRecord)
	if err := reg.EgressAllowed(p, FeatureStructured, ClassificationTestFixture); err != nil {
		t.Fatalf("feature-scoped verified record refused: %v", err)
	}
	// Other features still fall back to the unknown whole-profile record.
	if err := reg.EgressAllowed(p, FeatureChat, ClassificationTestFixture); err == nil {
		t.Fatal("chat must still be governed by the unknown record")
	}

	// Upsert keeps the NEWEST review (vendor-change recheck).
	newer := featureRecord
	newer.ReviewDate = fixedNow().Add(2 * time.Hour)
	newer.AllowedClassifications = []string{"other"}
	reg.Upsert(newer)
	if err := reg.EgressAllowed(p, FeatureStructured, ClassificationTestFixture); err == nil {
		t.Fatal("newer review must govern")
	}
}

func TestProfileKeyAndValidation(t *testing.T) {
	p := ProfileID{Provider: ProviderOpenAI, Endpoint: "chat-completions", Model: "g", Region: "r"}
	if p.Key() != "openai/chat-completions/g/r" {
		t.Fatalf("unexpected key %q", p.Key())
	}
	if err := (ProfileID{Provider: "x"}).Validate(); err == nil {
		t.Fatal("incomplete profile identity accepted")
	}
}

func TestResultPayloadDigestStability(t *testing.T) {
	r1 := &Result{Text: "t", FinishReason: FinishStop, Usage: Usage{InputTokens: 1, OutputTokens: 2}}
	r2 := &Result{Text: "t", FinishReason: FinishStop, Usage: Usage{InputTokens: 1, OutputTokens: 2}}
	if string(r1.ResultPayload()) != string(r2.ResultPayload()) {
		t.Fatal("result payload is not deterministic")
	}
	var probe map[string]any
	if err := json.Unmarshal(r1.ResultPayload(), &probe); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
}
