package providers

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The direct and custom adapters (C3-21–C3-30). One implementation serves
// each family; the per-provider facts are the capability set, the endpoint
// paths, and the credential header. Everything not proven by the recorded
// conformance fixtures stays CapUnknown — unknown means unsupported at the
// gate (C3-17/C3-20).

type familyKind int

const (
	familyOpenAI familyKind = iota
	familyAnthropic
	familyGemini
	familyMiniMax
)

type probeKind int

const (
	probeModelsGET      probeKind = iota // GET {testPath}: a bounded read
	probeValidationPOST                  // POST an empty-message chat request; 400 proves reachability + credential handling without generation
)

// directAdapter is one provider integration over a family codec.
type directAdapter struct {
	id             string
	base           string
	chatPath       string
	testPath       string
	probe          probeKind
	family         familyKind
	maxTokensField string
	protocolNote   string // recorded protocol metadata (refreshable)
	caps           dprovider.CapabilitySet
	overrides      map[dprovider.Feature]dprovider.CapabilityState
	explicitLocal  bool
}

// capabilities builds a CapabilitySet from explicit facts; every axis not
// named stays unknown (fail closed).
func capabilities(facts map[dprovider.Feature]dprovider.CapabilityState) dprovider.CapabilitySet {
	c := dprovider.CapabilitySet{
		Chat:                     dprovider.CapUnknown,
		Streaming:                dprovider.CapUnknown,
		Tools:                    dprovider.CapUnknown,
		ParallelToolCalls:        dprovider.CapUnknown,
		StructuredOutput:         dprovider.CapUnknown,
		Vision:                   dprovider.CapUnknown,
		Audio:                    dprovider.CapUnknown,
		Embeddings:               dprovider.CapUnknown,
		Batch:                    dprovider.CapUnknown,
		ContextCaching:           dprovider.CapUnknown,
		ConversationContinuation: dprovider.CapUnknown,
		TokenEstimatorRef:        "char-ratio-4:v1",
	}
	for f, s := range facts {
		applyCapability(&c, f, s)
	}
	return c
}

func applyCapability(c *dprovider.CapabilitySet, f dprovider.Feature, s dprovider.CapabilityState) {
	switch f {
	case dprovider.FeatureChat:
		c.Chat = s
	case dprovider.FeatureStreaming:
		c.Streaming = s
	case dprovider.FeatureTools:
		c.Tools = s
	case dprovider.FeatureParallelTools:
		c.ParallelToolCalls = s
	case dprovider.FeatureStructured:
		c.StructuredOutput = s
	case dprovider.FeatureVision:
		c.Vision = s
	case dprovider.FeatureAudio:
		c.Audio = s
	case dprovider.FeatureEmbeddings:
		c.Embeddings = s
	case dprovider.FeatureBatch:
		c.Batch = s
	case dprovider.FeatureContextCaching:
		c.ContextCaching = s
	case dprovider.FeatureContinuation:
		c.ConversationContinuation = s
	}
}

func sup(facts ...dprovider.Feature) map[dprovider.Feature]dprovider.CapabilityState {
	m := map[dprovider.Feature]dprovider.CapabilityState{}
	for _, f := range facts {
		m[f] = dprovider.CapSupported
	}
	return m
}

// newDirectAdapters records the eight direct integrations of C3-21–C3-28
// with their honest capability facts. Base URLs are refreshable metadata
// (Proposed): every connection can override them, and the defaults exist
// only so a misconfigured connection fails with a clear destination error
// rather than an empty-URL error.
func newDirectAdapters() []Adapter {
	ads := []*directAdapter{
		{
			id: dprovider.ProviderOpenAI, base: "https://api.openai.com/v1",
			chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
			family: familyOpenAI, maxTokensField: "max_completion_tokens",
			protocolNote: "openai chat-completions wire family; API version metadata is refreshable",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureParallelTools, dprovider.FeatureStructured, dprovider.FeatureVision,
				dprovider.FeatureEmbeddings, dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderAnthropic, base: "https://api.anthropic.com",
			chatPath: "/v1/messages", testPath: "/v1/models", probe: probeModelsGET,
			family:       familyAnthropic,
			protocolNote: "anthropic messages wire family; anthropic-version header comes from the connection record",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureParallelTools, dprovider.FeatureVision, dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderGemini, base: "https://generativelanguage.googleapis.com/v1beta",
			chatPath: "/models/{model}:generateContent", testPath: "/models", probe: probeModelsGET,
			family:       familyGemini,
			protocolNote: "gemini generateContent wire family; service tier/region belongs to the profile record",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureStructured, dprovider.FeatureVision, dprovider.FeatureEmbeddings,
				dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderQwen, base: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
			family: familyOpenAI, maxTokensField: "max_tokens",
			protocolNote: "dashscope compatible-mode wire family; region belongs to the profile record",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderGLM, base: "https://api.z.ai/api/paas/v4",
			chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
			family: familyOpenAI, maxTokensField: "max_tokens",
			protocolNote: "glm/z.ai paas v4 wire family; the credential is the bearer token from the secret boundary",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderMoonshot, base: "https://api.moonshot.ai/v1",
			chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
			family: familyOpenAI, maxTokensField: "max_tokens",
			protocolNote: "moonshot openai-compatible wire family; an ambiguous timeout never authorizes a blind retry",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureContinuation)),
		},
		{
			id: dprovider.ProviderMiniMax, base: "https://api.minimax.chat/v1",
			chatPath: "/text/chatcompletion_v2", testPath: "/text/chatcompletion_v2", probe: probeValidationPOST,
			family: familyMiniMax, maxTokensField: "max_tokens",
			protocolNote: "minimax chatcompletion_v2 wire family (recorded subset; base_resp carries provider status)",
			caps:         capabilities(sup(dprovider.FeatureChat)),
		},
		{
			id: dprovider.ProviderDeepSeek, base: "https://api.deepseek.com",
			chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
			family: familyOpenAI, maxTokensField: "max_tokens",
			protocolNote: "deepseek openai-compatible wire family; no silent provider fallback",
			caps: capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
				dprovider.FeatureContinuation)),
		},
	}
	for _, a := range ads {
		a.caps.Notes = a.protocolNote
	}
	out := make([]Adapter, 0, len(ads)+3)
	for _, a := range ads {
		out = append(out, a)
	}
	return out
}

// NewCustomOpenAIAdapter builds a custom OpenAI-compatible connection
// (C3-29): explicit base URL, model, and per-connection capability
// overrides. Compatibility is a tested profile, never a promise of full
// native equivalence — unoverridden axes stay unknown/unsupported.
func NewCustomOpenAIAdapter(baseURL, model string, overrides map[dprovider.Feature]dprovider.CapabilityState) Adapter {
	a := &directAdapter{
		id:       dprovider.ProviderCustomOpenAI,
		chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
		family: familyOpenAI, maxTokensField: "max_tokens",
		protocolNote: "custom openai-compatible connection; capability claims come from the per-connection overrides",
		caps:         capabilities(sup(dprovider.FeatureChat)),
		overrides:    overrides,
	}
	a.base = baseURL
	a.caps.Notes = a.protocolNote + "; model=" + model
	return a
}

// NewCustomAnthropicAdapter builds a custom Anthropic-compatible connection
// (C3-30) over the messages-family subset.
func NewCustomAnthropicAdapter(baseURL, model, protocolVersion string, overrides map[dprovider.Feature]dprovider.CapabilityState) Adapter {
	a := &directAdapter{
		id:       dprovider.ProviderCustomAnthropic,
		chatPath: "/v1/messages", testPath: "/v1/models", probe: probeModelsGET,
		family:       familyAnthropic,
		protocolNote: "custom anthropic-compatible connection; protocol version is recorded per connection",
		caps:         capabilities(sup(dprovider.FeatureChat)),
		overrides:    overrides,
	}
	a.base = baseURL
	a.caps.Notes = a.protocolNote + "; model=" + model + "; protocol=" + protocolVersion
	return a
}

// NewLocalAdapter builds an explicit local inference connection (C3-30):
// it is legal only on a LOCAL core with an explicit connection record, and
// the egress guard confines it to loopback.
func NewLocalAdapter(baseURL, model string, overrides map[dprovider.Feature]dprovider.CapabilityState) Adapter {
	a := &directAdapter{
		id:       dprovider.ProviderLocal,
		chatPath: "/chat/completions", testPath: "/models", probe: probeModelsGET,
		family: familyOpenAI, maxTokensField: "max_tokens",
		protocolNote:  "explicit local inference endpoint (openai-compatible local servers); loopback only",
		caps:          capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStreaming)),
		overrides:     overrides,
		explicitLocal: true,
	}
	a.base = baseURL
	a.caps.Notes = a.protocolNote + "; model=" + model
	return a
}

func (a *directAdapter) ID() string { return a.id }

func endpointLabel(f familyKind) string {
	switch f {
	case familyAnthropic:
		return "messages"
	case familyGemini:
		return "generateContent"
	case familyMiniMax:
		return "chatcompletion_v2"
	default:
		return "chat-completions"
	}
}

func (a *directAdapter) DefaultProfile(model string) dprovider.ProfileID {
	return dprovider.ProfileID{
		Provider: a.id,
		Endpoint: endpointLabel(a.family),
		Model:    model,
	}
}

func (a *directAdapter) Capabilities(p dprovider.ProfileID) dprovider.CapabilitySet {
	c := a.caps
	for f, s := range a.overrides {
		applyCapability(&c, f, s)
	}
	return c
}

func (a *directAdapter) AuthHeaderName(p dprovider.ProfileID) string {
	switch a.family {
	case familyAnthropic:
		return "x-api-key"
	case familyGemini:
		return "x-goog-api-key"
	default:
		return "Authorization"
	}
}

// SeedDataHandling is the honest seeded record (C3-19): every direct and
// custom integration ships with an UNKNOWN review — this workspace has
// recorded no dated official-source policy check, so protected data egress
// is blocked until such a review exists. No policy URL is claimed here.
func (a *directAdapter) SeedDataHandling(p dprovider.ProfileID) dprovider.DataHandlingProfile {
	return dprovider.DataHandlingProfile{
		Profile:                p,
		Deployment:             dprovider.FactUnknown,
		TrainingUse:            dprovider.FactUnknown,
		AbuseRetention:         dprovider.FactUnknown,
		ApplicationStateTTL:    dprovider.FactUnknown,
		Subprocessors:          dprovider.FactUnknown,
		DeletionEligibility:    dprovider.FactUnknown,
		AllowedClassifications: nil, // nothing is allowed without a dated review
		Notes:                  "no dated official-source review exists in this workspace; protected data egress is blocked (C3-19 fail-closed)",
	}
}

// resolvePath substitutes the {model} placeholder used by path-templated
// families (Gemini).
func (a *directAdapter) resolvePath(model, path string) string {
	return strings.ReplaceAll(path, "{model}", model)
}

func (a *directAdapter) NewRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	if a.explicitLocal {
		if e.Policy.Mode != CoreLocal {
			return nil, dprovider.NewError(dprovider.ErrEgressPolicy, a.id,
				"explicit local connections are legal only on a local core (C3-30)")
		}
		if !plan.Connection.ExplicitLocal {
			return nil, dprovider.NewError(dprovider.ErrEgressPolicy, a.id,
				"local inference requires an explicit local connection record (C3-30)")
		}
	}
	base := plan.Connection.BaseURL
	if base == "" {
		base = a.base
	}
	_ = base // the codecs join the connection base with the relative path
	switch a.family {
	case familyOpenAI:
		return openAIRequest(ctx, e, plan, secret, a.resolvePath(plan.Profile.Model, a.chatPath),
			openAIOptions{MaxTokensField: a.maxTokensField})
	case familyAnthropic:
		return anthropicRequest(ctx, e, plan, secret, a.chatPath)
	case familyGemini:
		return geminiRequest(ctx, e, plan, secret)
	case familyMiniMax:
		return miniMaxRequest(ctx, e, plan, secret)
	default:
		return nil, dprovider.NewError(dprovider.ErrUnsupported, a.id, "unknown family")
	}
}

func (a *directAdapter) DecodeResponse(status int, body []byte, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	switch a.family {
	case familyOpenAI:
		if plan.Operation == dprovider.OperationEmbed {
			return decodeOpenAIEmbed(a.id, status, body)
		}
		return decodeOpenAIChat(a.id, status, body, plan)
	case familyAnthropic:
		return decodeAnthropic(a.id, status, body, plan)
	case familyGemini:
		if plan.Operation == dprovider.OperationEmbed {
			return decodeGeminiEmbed(a.id, status, body)
		}
		return decodeGemini(a.id, status, body, plan)
	case familyMiniMax:
		return decodeMiniMax(a.id, status, body, plan)
	default:
		return nil, dprovider.NewError(dprovider.ErrUnsupported, a.id, "unknown family")
	}
}

func (a *directAdapter) NewStreamDecoder(p dprovider.ProfileID) StreamDecoder {
	if !a.caps.Streaming.Allowed() {
		return nil
	}
	switch a.family {
	case familyOpenAI:
		return newOpenAIStreamDecoder()
	case familyAnthropic:
		return newAnthropicStreamDecoder()
	case familyGemini:
		return newGeminiStreamDecoder()
	default:
		return nil
	}
}

// TestConnectionRequest builds the bounded, non-destructive probe against
// the connection's effective base URL (C3-18).
func (a *directAdapter) TestConnectionRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	base := plan.Connection.BaseURL
	if base == "" {
		base = a.base
	}
	switch a.probe {
	case probeModelsGET:
		u, err := ValidateDestinationURL(e.Policy, joinURL(base, a.testPath))
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, dprovider.NewError(dprovider.ErrInvalidRequest, a.id, "probe build failed")
		}
		for name, value := range plan.Connection.ExtraHeaders {
			if err := ValidateExtraHeader(name, value); err != nil {
				return nil, err
			}
			req.Header.Set(name, value)
		}
		switch a.family {
		case familyAnthropic:
			if v := plan.Connection.ProtocolVersion; v != "" {
				req.Header.Set("anthropic-version", v)
			}
			SetCredentialHeader(req, "x-api-key", secret)
		case familyGemini:
			SetCredentialHeader(req, "x-goog-api-key", secret)
		default:
			SetCredentialHeader(req, "Authorization", bearerValue(secret))
		}
		return req, nil
	default:
		// probeValidationPOST: an empty-message chat request. A 400 response
		// proves reachability and credential handling without generating.
		u, err := ValidateDestinationURL(e.Policy, joinURL(base, a.chatPath))
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader([]byte(`{"messages":[]}`)))
		if err != nil {
			return nil, dprovider.NewError(dprovider.ErrInvalidRequest, a.id, "probe build failed")
		}
		req.Header.Set("Content-Type", "application/json")
		SetCredentialHeader(req, "Authorization", bearerValue(secret))
		return req, nil
	}
}

// DefaultConnection returns the adapter's own connection posture: the
// recorded default base URL (refreshable metadata; every connection record
// may override it) and the family's credential class.
func (a *directAdapter) DefaultConnection() dprovider.Connection {
	return dprovider.Connection{
		BaseURL:       a.base,
		AuthKind:      dprovider.AuthBearer,
		SecretRef:     "provider:" + a.id + "/api-key",
		ExplicitLocal: a.explicitLocal,
	}
}

// InterpretProbe decides what a probe status means (C3-18: bounded and
// non-destructive).
func (a *directAdapter) InterpretProbe(status int) error {
	switch a.probe {
	case probeModelsGET:
		if status >= 200 && status <= 299 {
			return nil
		}
		return dprovider.NewError(dprovider.ErrUnreachable, a.id, "probe returned a non-success status")
	default:
		if (status >= 200 && status <= 299) || status == http.StatusBadRequest {
			return nil // 400: reachable, credential evaluated, nothing generated
		}
		return dprovider.NewError(dprovider.ErrAuth, a.id, "probe was rejected before evaluation")
	}
}
