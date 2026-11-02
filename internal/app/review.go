package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/knowledgepack"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/workflow"
)

// The pre-submit review payload (the owner's card-13 working candidate,
// 2026-11-02): a Core-side projection that builds the eight review fields
// per the recorded semantics (P3.6 §4.1; the P3.2 §3.6 [P] field set) so a
// user can tell BEFORE submission whether durable work or side effects may
// occur. The tool-effects field is a Run-specific PROJECTION of the
// workflow effect vocabulary (workflow.EffectClass/RetryClass/
// UnknownOutcomePolicy) — never direct exposure of workflow internals: a
// v1 Run submission is a task submission, not a workflow graph (the
// workflow-run path stays behind D-40/UI-30), so the projection reports
// what the Core's own task execution path does, expressed in that
// vocabulary. Fail-closed everywhere: anything the projection cannot
// determine renders as its honest unknown/absent value — never a guessed
// value, never a reassuring zero. UI visibility is not enforcement: the
// display decisions here never replace the Submit path's own
// authorization, validation, or gates.

// ReviewRequest is the intended submission a review is built for: the
// SubmitRequest minus the idempotency key (the key is minted at submit
// time, never at review time — the review is pre-submit S0 and creates
// nothing).
type ReviewRequest struct {
	Task            string
	System          string
	Schema          json.RawMessage
	PackRef         string // "packID" or "packID@version"; empty = no retrieval
	Provider        string // adapter id from the wired provider set
	Model           string
	MaxOutputTokens int
	Classification  string
	BaseURL         string
}

// ReviewIntent is field 1: the user's own task intent as it will be
// submitted (the authoritative source is the user's local input, echoed
// verbatim so the review shows exactly what would be submitted).
type ReviewIntent struct {
	Task           string `json:"task"`
	System         string `json:"system,omitempty"`
	SchemaProvided bool   `json:"schema_provided"`
}

// ReviewSources is field 2: the context that will accompany the task. The
// only source mechanism the Core defines is the KnowledgePack reference on
// a Job (JobInput.PackRef; empty = no retrieval), resolved against the
// pack store with the same failure reasons the execution path records.
// State is one of: none (no retrieval), resolved, unresolved (the typed
// reason carries why), unavailable (no pack store wired — fail closed).
type ReviewSources struct {
	State   string `json:"state"`
	PackRef string `json:"pack_ref,omitempty"`
	PackID  string `json:"pack_id,omitempty"`
	Version int    `json:"version,omitempty"`
	Status  string `json:"status,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Scope is the coded retrieval scope the execution path applies
	// (contextengine.Policy Scope "local" on the slice path).
	Scope string `json:"scope,omitempty"`
}

// Review provider/model (field 3): the provider id as submitted, whether
// it is in the wired provider set (the unwired-provider fail-fast is the
// Core's own validation), and the capability facts the wired executor
// reports for the effective profile. Capabilities carry the coded
// CapabilitySet verbatim — supported/unsupported/unknown, "unknown means
// unsupported wherever it gates a call"; the UI renders unknown as its
// not-verified state, never as a granted badge.
type ReviewProviderModel struct {
	Provider     string                   `json:"provider"`
	Wired        bool                     `json:"wired"`
	Model        string                   `json:"model,omitempty"`
	Profile      *dprovider.ProfileID     `json:"profile,omitempty"`
	Capabilities *dprovider.CapabilitySet `json:"capabilities,omitempty"`
}

// ReviewEffect is one projected effect of the v1 task execution path,
// expressed in the workflow effect vocabulary (the coded wire values:
// class none|local|external, retry safe|scoped|never, unknown_outcome
// fail|needs_reconciliation). The class value "unknown" is the
// projection's own honest extension: when the Core cannot determine the
// class (no wired executor, no data-handling facts), the payload says
// unknown — never a guessed class.
type ReviewEffect struct {
	Effect           string `json:"effect"`
	Class            string `json:"class"`
	Retry            string `json:"retry"`
	IdempotencyScope string `json:"idempotency_scope,omitempty"`
	UnknownOutcome   string `json:"unknown_outcome"`
}

// ReviewPermission is field 5: the permission state the Core reports for
// the submitting principal over the submission's resource — the pure
// decision-port outcome (Policy.Decide) for the SAME resource/action class
// Submit authorizes (Resource job / ActionDataWrite). It is a display
// fact, never enforcement: the Submit path runs the authoritative Guard
// decision (and its audit) itself. A denial renders the stable bounded
// reason code with the required resource/action — no existence leak.
type ReviewPermission struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Action  string `json:"action"`
}

// ReviewPolicy is field 6: the policy snapshot the Core applies to the
// submission — the per-profile data-handling record as the Core reports
// it (the coded facts verbatim, unknown facts carried as the coded
// "unknown"), the record's review status (the coded verified/unknown
// rule), and the fail-closed egress decision for the submitted
// classification with the typed blocking reason. No record → the payload
// says so and reports the coded no-record reason.
type ReviewPolicy struct {
	ProfileKnown        bool   `json:"profile_known"`
	Review              string `json:"review,omitempty"`
	Deployment          string `json:"deployment,omitempty"`
	TrainingUse         string `json:"training_use,omitempty"`
	AbuseRetention      string `json:"abuse_retention,omitempty"`
	ApplicationStateTTL string `json:"application_state_ttl,omitempty"`
	Subprocessors       string `json:"subprocessors,omitempty"`
	DeletionEligibility string `json:"deletion_eligibility,omitempty"`
	EgressAllowed       bool   `json:"egress_allowed"`
	EgressReason        string `json:"egress_reason,omitempty"`
}

// ReviewApprovalNeed is field 7: whether the Core's policy requires
// approval for this submission. The coded posture IS determinable: the
// local task-submission path applies no approval gate (the approval
// declaration exists only at the workflow definition layer — the
// human_approval node — which a v1 task submission does not carry), so
// the projection reports required=false with that stable basis. This is
// the Core's report of its own coded behavior, never an assumption about
// a policy that does not exist.
type ReviewApprovalNeed struct {
	Required bool   `json:"required"`
	Basis    string `json:"basis"`
}

// ReviewBudget is field 8: the estimated budget. Only what the Core can
// honestly determine pre-submit is present: the reserved output tokens
// the execution path would reserve (the request's max_output_tokens when
// positive). The estimated input tokens, the provider-reported actual,
// and any cost are ABSENT (nil) until the Core or the provider reports
// them — absent renders Not-available, never zero, and no cost source
// exists in code so no cost value is ever invented here.
type ReviewBudget struct {
	ReservedOutputTokens *int `json:"reserved_output_tokens,omitempty"`
}

// ReviewPayload is the eight-field pre-submit review payload, exactly the
// recorded field set (P3.6 §4.1).
type ReviewPayload struct {
	Intent        ReviewIntent        `json:"intent"`
	Sources       ReviewSources       `json:"sources"`
	ProviderModel ReviewProviderModel `json:"provider_model"`
	ToolEffects   []ReviewEffect      `json:"tool_effects"`
	Permission    ReviewPermission    `json:"permission"`
	Policy        ReviewPolicy        `json:"policy"`
	ApprovalNeed  ReviewApprovalNeed  `json:"approval_need"`
	Budget        ReviewBudget        `json:"budget"`
}

// Review builds the pre-submit review payload for the intended
// submission. It is a READ (ActionDataRead through the same decision port
// every read authorizes): it creates no Job, no snapshot, no event, and
// no audit effect beyond its own read decision. Invalid request shapes
// refuse with the typed VALIDATION_FAILED envelope (the same shape rules
// Submit enforces); a well-formed draft always yields a payload whose
// per-field states carry the gaps.
func (s *Slice) Review(ctx context.Context, req ReviewRequest) (*ReviewPayload, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob}, security.ActionDataRead); err != nil {
		return nil, err
	}
	if err := validateReviewShape(req, corr); err != nil {
		return nil, err
	}

	payload := &ReviewPayload{
		Intent: ReviewIntent{
			Task:           req.Task,
			System:         req.System,
			SchemaProvided: len(req.Schema) > 0,
		},
	}
	payload.Sources = s.reviewSources(ctx, req)
	payload.ProviderModel = s.reviewProviderModel(req)
	payload.ToolEffects = s.reviewToolEffects(req)
	payload.Permission = s.reviewPermission(ctx, corr)
	payload.Policy = s.reviewPolicy(req)
	payload.ApprovalNeed = ReviewApprovalNeed{
		Required: false,
		Basis:    "no_approval_gate_on_task_submission",
	}
	payload.Budget = reviewBudget(req)
	return payload, nil
}

// reviewSources resolves the KnowledgePack reference with the same
// failure reasons the execution path records (retrieve's coded reasons),
// fail-closed: an absent pack store renders unavailable.
func (s *Slice) reviewSources(ctx context.Context, req ReviewRequest) ReviewSources {
	if req.PackRef == "" {
		return ReviewSources{State: "none"}
	}
	src := ReviewSources{State: "unresolved", PackRef: req.PackRef, Scope: "local"}
	if s.deps.Packs == nil {
		src.State = "unavailable"
		return src
	}
	packID, version, err := parsePackRef(req.PackRef)
	if err != nil {
		src.Reason = bounded(err.Error())
		return src
	}
	src.PackID = packID
	if version == 0 {
		v, err := s.deps.Packs.LatestPackVersion(ctx, packID)
		if err != nil || v == 0 {
			src.Reason = "pack_not_found"
			return src
		}
		version = v
	}
	src.Version = version
	pack, err := s.deps.Packs.LoadPackVersion(ctx, packID, version)
	if err != nil {
		src.Reason = "pack_not_found"
		return src
	}
	src.Status = string(pack.Status)
	if pack.Status != knowledgepack.PackStatusActive {
		src.Reason = "pack_" + string(pack.Status)
		return src
	}
	src.State = "resolved"
	return src
}

// reviewProviderModel reports the wired truth and the capability facts.
// The effective model mirrors the execution path's substitution (an empty
// model executes as "default" — every profile needs a model identity).
func (s *Slice) reviewProviderModel(req ReviewRequest) ReviewProviderModel {
	out := ReviewProviderModel{Provider: req.Provider, Model: req.Model}
	executor, wired := s.deps.Providers[req.Provider]
	if !wired {
		return out
	}
	out.Wired = true
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = "default"
	}
	profile := executor.ProfileFor(model)
	out.Profile = &profile
	caps := executor.Capabilities(profile)
	out.Capabilities = &caps
	return out
}

// reviewToolEffects is the Run-specific projection of the workflow effect
// vocabulary onto the v1 task execution path. The path's two effect
// classes are what the coded handler does: the provider call and the
// local durable writes. Every value is the coded vocabulary's wire value;
// the provider-call class derives from the wired executor's data-handling
// deployment facts and is unknown when those facts are absent.
func (s *Slice) reviewToolEffects(req ReviewRequest) []ReviewEffect {
	// durable_state: the Job's local durable writes (input/result CAS,
	// snapshot, journal) — content-addressed and attempt-keyed, so a
	// re-issued attempt's local writes are idempotent by construction
	// (retry safe); a purely local write has no unknown external outcome
	// (fail).
	effects := []ReviewEffect{
		{
			Effect:         "durable_state",
			Class:          string(workflow.EffectLocal),
			Retry:          string(workflow.RetrySafe),
			UnknownOutcome: string(workflow.UnknownFail),
		},
	}
	// provider_call: reaches outside the core exactly when the wired
	// executor's data-handling facts say the deployment is a cloud form;
	// local/fixture deployments do not dial out (the fixture adapter
	// performs no network I/O by construction). Retryable provider
	// failures requeue as a NEW attempt of the SAME Job under its durable
	// idempotency/fencing scope (retry scoped — never a blind retry); an
	// unknown provider outcome is a first-class state that forces
	// needs_reconciliation (the coded SideEffectIntent law).
	call := ReviewEffect{
		Effect:           "provider_call",
		Class:            "unknown",
		Retry:            string(workflow.RetryScoped),
		IdempotencyScope: "job",
		UnknownOutcome:   string(workflow.UnknownNeedsReconciliation),
	}
	if executor, wired := s.deps.Providers[req.Provider]; wired {
		model := req.Model
		if strings.TrimSpace(model) == "" {
			model = "default"
		}
		if rec, ok := executor.DataHandling(executor.ProfileFor(model), reviewFeature(req)); ok {
			switch rec.Deployment {
			case dprovider.DeploymentCloudPublic, dprovider.DeploymentCloudDedicated:
				call.Class = string(workflow.EffectExternal)
			case dprovider.DeploymentLocal, dprovider.DeploymentFixture:
				call.Class = string(workflow.EffectLocal)
			default:
				call.Class = "unknown" // an unknown deployment stays unknown
			}
		}
	}
	effects = append(effects, call)
	return effects
}

// reviewPermission reports the pure decision-port outcome for the
// submit-class check (Resource job / ActionDataWrite) — the SAME
// resource/action pair the Submit path authorizes through the Guard. The
// projection calls Policy.Decide directly (no audit record, no effect):
// this is a display fact; Submit's own Guard decision stays the
// authoritative, audited check. An unwired policy denies fail-closed
// with the coded reason, exactly as the Guard does.
func (s *Slice) reviewPermission(ctx context.Context, corr string) ReviewPermission {
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: principalID(ctx)}
	pctx := security.PolicyContext{CorrelationID: corr, Credential: credential(ctx)}
	resource := security.Resource{Type: security.ResourceJob}
	if s.deps.Guard == nil || s.deps.Guard.Policy == nil {
		return ReviewPermission{Allowed: false, Reason: security.ReasonPolicyUnavailable, Action: string(security.ActionDataWrite)}
	}
	decision := s.deps.Guard.Policy.Decide(p, resource, security.ActionDataWrite, pctx)
	return ReviewPermission{
		Allowed: decision.Allow,
		Reason:  decision.ReasonCode,
		Action:  string(security.ActionDataWrite),
	}
}

// reviewPolicy reports the data-handling snapshot the Core applies: the
// governing per-profile record verbatim (unknown facts stay the coded
// "unknown"), its coded review status, and the fail-closed egress
// decision for the submitted classification (empty classification =
// protected-unlabeled, exactly as the gate treats it). No governing
// record → the coded no-record reason.
func (s *Slice) reviewPolicy(req ReviewRequest) ReviewPolicy {
	out := ReviewPolicy{EgressAllowed: false}
	executor, wired := s.deps.Providers[req.Provider]
	if !wired {
		out.EgressReason = "provider_not_wired"
		return out
	}
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = "default"
	}
	rec, ok := executor.DataHandling(executor.ProfileFor(model), reviewFeature(req))
	if !ok {
		out.EgressReason = "no_data_handling_profile_recorded"
		return out
	}
	out.ProfileKnown = true
	out.Review = rec.Review(s.deps.Now())
	out.Deployment = rec.Deployment
	out.TrainingUse = rec.TrainingUse
	out.AbuseRetention = rec.AbuseRetention
	out.ApplicationStateTTL = rec.ApplicationStateTTL
	out.Subprocessors = rec.Subprocessors
	out.DeletionEligibility = rec.DeletionEligibility
	if err := rec.EgressAllowed(s.deps.Now(), req.Classification); err != nil {
		if pe, ok := err.(*dprovider.ProviderError); ok {
			out.EgressReason = pe.Detail
		} else {
			out.EgressReason = bounded(err.Error())
		}
		return out
	}
	out.EgressAllowed = true
	return out
}

// reviewBudget reports the one budget fact the Core determines pre-submit:
// the reserved output tokens (the execution path reserves the request's
// max_output_tokens when positive). Everything else stays absent.
func reviewBudget(req ReviewRequest) ReviewBudget {
	out := ReviewBudget{}
	if req.MaxOutputTokens > 0 {
		tokens := req.MaxOutputTokens
		out.ReservedOutputTokens = &tokens
	}
	return out
}

// reviewFeature mirrors the execution path's plan shape: the feature the
// capability and data-handling gates key on is structured_output when the
// submission carries a schema, chat otherwise (FeatureForPlan on the
// handler's plan).
func reviewFeature(req ReviewRequest) dprovider.Feature {
	if len(req.Schema) > 0 {
		return dprovider.FeatureStructured
	}
	return dprovider.FeatureChat
}

// validateReviewShape applies the same request-shape rules Submit
// validates (minus the idempotency key, which a review never carries) —
// the shared shape law, never a second one.
func validateReviewShape(req ReviewRequest, corr string) error {
	return validateSubmissionShape(req.Task, req.System, req.Provider, req.Schema, corr)
}

// bounded caps a reason string to a bounded, single-line value (the
// handler's boundedDetail discipline at the payload boundary).
func bounded(s string) string {
	if len(s) > 400 {
		s = s[:400]
	}
	return strings.ReplaceAll(s, "\n", " ")
}
