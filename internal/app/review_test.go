package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/knowledgepack"
	"github.com/qira-dev/karyora/internal/security"
)

// The pre-submit review payload (the owner's card-13 working candidate):
// table-driven pins per review field. Every pin proves the field's
// source semantics: the projection reports what the Core's own gates and
// execution path determine, fail-closed — anything undetermined renders
// as its honest unknown/absent value, never a guessed value.

// staticFactsExecutor wraps an executor with fixed fact answers, so the
// cloud/unknown deployment projections are pinnable without a real
// adapter family.
type staticFactsExecutor struct {
	inner  ProviderExecutor
	caps   dprovider.CapabilitySet
	rec    dprovider.DataHandlingProfile
	hasRec bool
}

func (s *staticFactsExecutor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	return s.inner.Execute(ctx, plan)
}
func (s *staticFactsExecutor) ProfileFor(model string) dprovider.ProfileID {
	return s.inner.ProfileFor(model)
}
func (s *staticFactsExecutor) Connection() dprovider.Connection { return s.inner.Connection() }
func (s *staticFactsExecutor) Capabilities(profile dprovider.ProfileID) dprovider.CapabilitySet {
	return s.caps
}
func (s *staticFactsExecutor) DataHandling(profile dprovider.ProfileID, feature dprovider.Feature) (dprovider.DataHandlingProfile, bool) {
	if !s.hasRec {
		return dprovider.DataHandlingProfile{}, false
	}
	return s.rec, true
}

func baseReview() ReviewRequest {
	return ReviewRequest{
		Task:           "Summarize the recorded fixture corpus.",
		Provider:       dprovider.ProviderMock,
		Model:          "m",
		Classification: dprovider.ClassificationTestFixture,
	}
}

// TestReviewIntentField pins field 1: the intent is the user's own text
// echoed verbatim (as it will be submitted) plus the schema-presence flag.
func TestReviewIntentField(t *testing.T) {
	env := newTestEnv(t)
	req := baseReview()
	req.System = "Be exact."
	payload, err := env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if payload.Intent.Task != req.Task || payload.Intent.System != req.System {
		t.Fatalf("intent must echo the submission text verbatim: %+v", payload.Intent)
	}
	if payload.Intent.SchemaProvided {
		t.Fatal("no schema was provided; the flag must be false")
	}
	req.Schema = []byte(`{"type":"object"}`)
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review with schema: %v", err)
	}
	if !payload.Intent.SchemaProvided {
		t.Fatal("a provided schema must set the flag")
	}
}

// TestReviewSourcesField pins field 2: the pack reference resolves with
// the execution path's own failure reasons; empty = none; no pack store =
// unavailable (fail closed).
func TestReviewSourcesField(t *testing.T) {
	env := newTestEnv(t)

	// No reference: none (no retrieval).
	payload, err := env.slice.Review(env.ctx(), baseReview())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if payload.Sources.State != "none" {
		t.Fatalf("empty pack_ref must resolve to none, got %+v", payload.Sources)
	}

	// An active pack resolves with its effective version and status.
	_ = seedFixturePack(t, env, "recorded fixture text")
	req := baseReview()
	req.PackRef = "pack-fixture"
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review with pack: %v", err)
	}
	if payload.Sources.State != "resolved" || payload.Sources.PackID != "pack-fixture" ||
		payload.Sources.Version != 1 || payload.Sources.Status != string(knowledgepack.PackStatusActive) {
		t.Fatalf("an active pack must resolve: %+v", payload.Sources)
	}

	// A non-active pack carries the coded omission reason.
	if err := env.deps.Packs.SetPackStatus(env.ctx(), "pack-fixture", 1, knowledgepack.PackStatusQuarantined, "test"); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review quarantined: %v", err)
	}
	if payload.Sources.State != "unresolved" || payload.Sources.Reason != "pack_quarantined" {
		t.Fatalf("a quarantined pack must carry the omission reason: %+v", payload.Sources)
	}

	// A missing pack: the coded not-found reason.
	req.PackRef = "pack-missing"
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review missing pack: %v", err)
	}
	if payload.Sources.State != "unresolved" || payload.Sources.Reason != "pack_not_found" {
		t.Fatalf("a missing pack must carry pack_not_found: %+v", payload.Sources)
	}

	// No pack store wired: unavailable (fail closed). A new slice over
	// the mutated deps is required — the constructed slice holds its own
	// deps copy (only maps are shared).
	env.deps.Packs = nil
	noPackSlice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("reslice: %v", err)
	}
	req.PackRef = "pack-fixture"
	payload, err = noPackSlice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review without packs: %v", err)
	}
	if payload.Sources.State != "unavailable" {
		t.Fatalf("an absent pack store must render unavailable: %+v", payload.Sources)
	}
}

// TestReviewProviderModelField pins field 3: the wired truth, the
// effective profile (an empty model executes as "default"), and the coded
// capability facts; an unwired provider renders wired=false with no
// invented facts.
func TestReviewProviderModelField(t *testing.T) {
	env := newTestEnv(t)

	payload, err := env.slice.Review(env.ctx(), baseReview())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !payload.ProviderModel.Wired || payload.ProviderModel.Provider != dprovider.ProviderMock {
		t.Fatalf("the mock provider is wired: %+v", payload.ProviderModel)
	}
	if payload.ProviderModel.Profile == nil || payload.ProviderModel.Profile.Provider != dprovider.ProviderMock {
		t.Fatalf("the effective profile must be reported: %+v", payload.ProviderModel)
	}
	if payload.ProviderModel.Capabilities == nil || payload.ProviderModel.Capabilities.Chat != dprovider.CapSupported {
		t.Fatalf("the coded capability facts must be reported: %+v", payload.ProviderModel.Capabilities)
	}

	// The empty model executes as "default": the profile mirrors that.
	req := baseReview()
	req.Model = ""
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review default model: %v", err)
	}
	if payload.ProviderModel.Profile == nil || payload.ProviderModel.Profile.Model != "default" {
		t.Fatalf("an empty model must resolve to the default profile: %+v", payload.ProviderModel.Profile)
	}

	// An unwired provider: wired=false and NO invented facts.
	req.Provider = "no-such-provider"
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review unwired: %v", err)
	}
	if payload.ProviderModel.Wired || payload.ProviderModel.Profile != nil || payload.ProviderModel.Capabilities != nil {
		t.Fatalf("an unwired provider must carry no invented facts: %+v", payload.ProviderModel)
	}
}

// TestReviewToolEffectsField pins field 4: the Run-specific projection of
// the workflow effect vocabulary — durable_state (local/safe/fail) and
// provider_call (deployment-derived class; scoped retry under the Job
// scope; needs_reconciliation) — with the fail-closed unknown class.
func TestReviewToolEffectsField(t *testing.T) {
	env := newTestEnv(t)
	cloudRecord := dprovider.DataHandlingProfile{
		Profile:     dprovider.ProfileID{Provider: "fake-cloud", Endpoint: "api", Model: "m"},
		Deployment:  dprovider.DeploymentCloudPublic,
		TrainingUse: dprovider.FactNo,
	}
	caps := dprovider.CapabilitySet{Chat: dprovider.CapSupported}
	env.deps.Providers["fake-cloud"] = &staticFactsExecutor{
		inner: env.deps.Providers[dprovider.ProviderMock], caps: caps, rec: cloudRecord, hasRec: true,
	}
	unknownRecord := cloudRecord
	unknownRecord.Profile = dprovider.ProfileID{Provider: "fake-unknown", Endpoint: "api", Model: "m"}
	unknownRecord.Deployment = "unknown"
	env.deps.Providers["fake-unknown"] = &staticFactsExecutor{
		inner: env.deps.Providers[dprovider.ProviderMock], caps: caps, rec: unknownRecord, hasRec: true,
	}

	cases := []struct {
		name     string
		provider string
		class    string
	}{
		{"cloud deployment projects external", "fake-cloud", "external"},
		{"local fixture deployment projects local", dprovider.ProviderMock, "local"},
		{"unknown deployment stays unknown", "fake-unknown", "unknown"},
		{"unwired provider stays unknown", "no-such-provider", "unknown"},
	}
	for _, tc := range cases {
		req := baseReview()
		req.Provider = tc.provider
		payload, err := env.slice.Review(env.ctx(), req)
		if err != nil {
			t.Fatalf("%s: review: %v", tc.name, err)
		}
		if len(payload.ToolEffects) != 2 {
			t.Fatalf("%s: the projection carries exactly two effects, got %d", tc.name, len(payload.ToolEffects))
		}
		byName := map[string]ReviewEffect{}
		for _, e := range payload.ToolEffects {
			byName[e.Effect] = e
		}
		durable, ok := byName["durable_state"]
		if !ok {
			t.Fatalf("%s: the durable_state effect must be projected", tc.name)
		}
		if durable.Class != "local" || durable.Retry != "safe" || durable.UnknownOutcome != "fail" {
			t.Fatalf("%s: durable_state pin mismatch: %+v", tc.name, durable)
		}
		call, ok := byName["provider_call"]
		if !ok {
			t.Fatalf("%s: the provider_call effect must be projected", tc.name)
		}
		if call.Class != tc.class {
			t.Fatalf("%s: provider_call class must be %q, got %q", tc.name, tc.class, call.Class)
		}
		if call.Retry != "scoped" || call.IdempotencyScope != "job" || call.UnknownOutcome != "needs_reconciliation" {
			t.Fatalf("%s: provider_call retry/unknown-outcome pin mismatch: %+v", tc.name, call)
		}
	}
}

// TestReviewPermissionField pins field 5: the pure decision-port outcome
// for the submit-class check — allowed for a valid data session, the
// stable denial reason with no existence leak otherwise; never an audit
// side effect.
func TestReviewPermissionField(t *testing.T) {
	env := newTestEnv(t)

	payload, err := env.slice.Review(env.ctx(), baseReview())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !payload.Permission.Allowed || payload.Permission.Reason != "OK" {
		t.Fatalf("a valid data session must be allowed: %+v", payload.Permission)
	}
	if payload.Permission.Action != string(security.ActionDataWrite) {
		t.Fatalf("the permission field names the submit-class action: %+v", payload.Permission)
	}

	// An authenticated caller whose session the POLICY refuses for the
	// submit class: the same session authorizes both read and write in
	// Local mode, so the denial surface is pinned at the read boundary —
	// the empty credential is a well-formed principal the port denies
	// with its stable session reason.
	env2 := newTestEnv(t)
	limited := WithCaller(context.Background(), "local", "")
	if _, err := env2.slice.Review(limited, baseReview()); err == nil {
		t.Fatal("the empty credential must be denied at the review read")
	}
}

// TestReviewPolicyField pins field 6: the data-handling snapshot verbatim
// (facts, review status) and the fail-closed egress decision with its
// typed reason; no record → the coded no-record reason.
func TestReviewPolicyField(t *testing.T) {
	env := newTestEnv(t)

	// The mock fixture record: verified review, local deployment, facts
	// verbatim, egress allowed for the fixture classification.
	payload, err := env.slice.Review(env.ctx(), baseReview())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	pol := payload.Policy
	if !pol.ProfileKnown || pol.Review != "verified" || pol.Deployment != dprovider.DeploymentLocal {
		t.Fatalf("the fixture record must be reported verbatim: %+v", pol)
	}
	if pol.TrainingUse != dprovider.FactNo || pol.Subprocessors != dprovider.FactNo {
		t.Fatalf("the record's facts must ride verbatim: %+v", pol)
	}
	if !pol.EgressAllowed || pol.EgressReason != "" {
		t.Fatalf("the fixture classification must pass the egress gate: %+v", pol)
	}

	// An unallowed classification: the typed blocking reason.
	req := baseReview()
	req.Classification = dprovider.ClassificationNonPersonal
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review non-personal: %v", err)
	}
	if payload.Policy.EgressAllowed || payload.Policy.EgressReason != "classification_not_allowed_by_profile" {
		t.Fatalf("a blocked classification must carry the typed reason: %+v", payload.Policy)
	}

	// An empty classification is protected-unlabeled: blocked, fail closed.
	req.Classification = ""
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review unlabeled: %v", err)
	}
	if payload.Policy.EgressAllowed {
		t.Fatal("an unlabeled draft must report a blocked egress (protected-unlabeled)")
	}

	// An unwired provider: the payload names it, no record invented.
	req.Provider = "no-such-provider"
	req.Classification = dprovider.ClassificationTestFixture
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review unwired policy: %v", err)
	}
	if payload.Policy.ProfileKnown || payload.Policy.EgressReason != "provider_not_wired" {
		t.Fatalf("an unwired provider must name itself in the policy field: %+v", payload.Policy)
	}

	// A wired executor without facts: the coded no-record reason.
	env.deps.Providers["no-facts"] = &staticFactsExecutor{inner: env.deps.Providers[dprovider.ProviderMock], hasRec: false}
	req.Provider = "no-facts"
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review no-facts: %v", err)
	}
	if payload.Policy.ProfileKnown || payload.Policy.EgressReason != "no_data_handling_profile_recorded" {
		t.Fatalf("a missing record must carry the coded reason: %+v", payload.Policy)
	}
}

// TestReviewApprovalNeedAndBudgetFields pin fields 7 and 8: the coded
// posture (no approval gate on the task path) and the one determinable
// budget fact (the reserved output tokens), everything else absent.
func TestReviewApprovalNeedAndBudgetFields(t *testing.T) {
	env := newTestEnv(t)

	payload, err := env.slice.Review(env.ctx(), baseReview())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if payload.ApprovalNeed.Required || payload.ApprovalNeed.Basis != "no_approval_gate_on_task_submission" {
		t.Fatalf("the coded approval posture must be reported: %+v", payload.ApprovalNeed)
	}
	if payload.Budget.ReservedOutputTokens != nil {
		t.Fatalf("no reserved output without max_output_tokens: %+v", payload.Budget)
	}

	req := baseReview()
	req.MaxOutputTokens = 512
	payload, err = env.slice.Review(env.ctx(), req)
	if err != nil {
		t.Fatalf("review budget: %v", err)
	}
	if payload.Budget.ReservedOutputTokens == nil || *payload.Budget.ReservedOutputTokens != 512 {
		t.Fatalf("the reserved output tokens must ride the request value: %+v", payload.Budget)
	}
}

// TestReviewCreatesNothing proves the review is a read: no Job, no
// attempt, no event — the Jobs list stays empty after a review.
func TestReviewCreatesNothing(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.slice.Review(env.ctx(), baseReview()); err != nil {
		t.Fatalf("review: %v", err)
	}
	page, err := env.slice.List(env.ctx(), ListRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Jobs) != 0 {
		t.Fatalf("a review must create no durable state, got %d jobs", len(page.Jobs))
	}
}

// TestReviewValidation pins the typed request-shape refusals (the shared
// shape law with Submit, minus the idempotency key a review never
// carries).
func TestReviewValidation(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name  string
		mut   func(*ReviewRequest)
		field string
	}{
		{"empty task", func(r *ReviewRequest) { r.Task = "" }, "task"},
		{"empty provider", func(r *ReviewRequest) { r.Provider = "" }, "provider"},
		{"invalid schema", func(r *ReviewRequest) { r.Schema = []byte("{nope") }, "schema"},
	}
	for _, tc := range cases {
		req := baseReview()
		tc.mut(&req)
		_, err := env.slice.Review(env.ctx(), req)
		var ee *EnvelopeError
		if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("%s: expected a typed VALIDATION_FAILED envelope, got %v", tc.name, err)
		}
		if ee.Envelope.Data["field"] != tc.field {
			t.Fatalf("%s: expected field %q, got %v", tc.name, tc.field, ee.Envelope.Data["field"])
		}
	}
}
