// Package provider holds the provider-NEUTRAL contracts (C3-17): request
// plans, model/profile identity, capability states, result metadata, the
// stable provider error taxonomy, and the per-profile data-handling records
// (C3-19). Provider SDK types and wire protocols stay behind the adapters in
// internal/providers; nothing here imports an SDK, a network library beyond
// encoding/json, or any internal package.
//
// Fail-closed rule carried by this package: a capability state of "unknown"
// means unsupported, and a data-handling profile without a dated review
// blocks protected-data egress.
package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Provider identifiers (C3-17): the eight direct integrations plus the
// custom and explicit-local connection families (C3-29/C3-30).
const (
	ProviderOpenAI          = "openai"
	ProviderAnthropic       = "anthropic"
	ProviderGemini          = "google-gemini"
	ProviderQwen            = "alibaba-qwen"
	ProviderGLM             = "glm-zai"
	ProviderMoonshot        = "kimi-moonshot"
	ProviderMiniMax         = "minimax"
	ProviderDeepSeek        = "deepseek"
	ProviderCustomOpenAI    = "custom-openai"
	ProviderCustomAnthropic = "custom-anthropic"
	ProviderLocal           = "local"
	ProviderMock            = "mock"
)

// ProfileID identifies one TESTED provider deployment: endpoint, model,
// region, and protocol version. Capability and data-handling claims attach
// to profiles, never to a provider in general (C3-17/C3-19/C3-20): a claim
// proven on one Gemini surface is never inferred for another.
type ProfileID struct {
	Provider   string `json:"provider"`
	Endpoint   string `json:"endpoint"`
	Model      string `json:"model"`
	Region     string `json:"region,omitempty"`
	APIVersion string `json:"api_version,omitempty"` // refreshable metadata
}

// Key is the registry key form: provider/endpoint/model[/region].
func (p ProfileID) Key() string {
	k := p.Provider + "/" + p.Endpoint + "/" + p.Model
	if p.Region != "" {
		k += "/" + p.Region
	}
	return k
}

func (p ProfileID) Validate() error {
	if p.Provider == "" || p.Endpoint == "" || p.Model == "" {
		return fmt.Errorf("provider: profile identity requires provider, endpoint, and model")
	}
	if strings.ContainsAny(p.Provider+p.Endpoint+p.Model+p.Region, " \t\n") {
		return fmt.Errorf("provider: profile identity fields must not carry whitespace")
	}
	return nil
}

// AuthKind fixes WHERE a credential travels: never in a URL (C3-18). The
// header NAME is fixed by the adapter family; the VALUE is resolved at call
// time through the secret boundary (D-37) and never stored here.
type AuthKind string

const (
	AuthNone   AuthKind = "none"
	AuthBearer AuthKind = "bearer"  // Authorization: Bearer <secret>
	AuthAPIKey AuthKind = "api-key" // family-specific key header
)

// Connection is the caller-supplied connection configuration for one
// profile (C3-17/C3-29): base URL, protocol version, secret REFERENCE, and
// allowlisted extra headers. It never carries a credential value.
type Connection struct {
	BaseURL         string            `json:"base_url"`
	SecretRef       string            `json:"secret_ref,omitempty"` // security.Handle string form
	AuthKind        AuthKind          `json:"auth_kind"`
	ProtocolVersion string            `json:"protocol_version,omitempty"`
	ExtraHeaders    map[string]string `json:"extra_headers,omitempty"`
	ExplicitLocal   bool              `json:"explicit_local,omitempty"` // C3-30 explicit local connection
}

// RequestPlan is the neutral execution request (C3-17). It is validated
// before use: bounded message/tool/metadata sizes, no credential material
// in the connection, and a known operation.
const (
	OperationChat  = "chat"
	OperationEmbed = "embed"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Part is one content part. Image parts carry INLINE bytes: the caller
// resolves CAS references before the call; adapters refuse image parts
// without bytes rather than inventing a fetch.
type Part struct {
	Type      string `json:"type"` // "text" | "image"
	Text      string `json:"text,omitempty"`
	ImageMIME string `json:"image_mime,omitempty"`
	ImageData []byte `json:"image_data,omitempty"`
}

// ToolCall is one provider-requested tool invocation.
type ToolCall struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	ArgumentsJSON string `json:"arguments_json"`
}

// Message is one neutral conversation message. Vision (C3-17) is one image
// plus text in this contract version; audio/multimodal beyond images stays
// unsupported (unknown means unsupported).
type Message struct {
	Role       Role       `json:"role"`
	Parts      []Part     `json:"parts,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant history
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool replies
}

func (m Message) Text() string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func (m Message) HasImage() bool {
	for _, p := range m.Parts {
		if p.Type == "image" {
			return true
		}
	}
	return false
}

// ToolSpec declares one callable tool with a JSON Schema parameters object.
type ToolSpec struct {
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	ParametersSchema json.RawMessage `json:"parameters_schema,omitempty"`
}

// ToolChoice values (C3-17): auto | none | required | tool:<name>.
type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceNone     ToolChoice = "none"
	ToolChoiceRequired ToolChoice = "required"
)

// StructuredOutput selects the schema-constrained JSON output mode. The
// schema is validated against the generated output BEFORE any result commit
// (C3-20 acceptance).
type StructuredOutput struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict,omitempty"`
}

// RequestPlan is the neutral request for one provider call.
type RequestPlan struct {
	Operation       string            `json:"operation"`
	Profile         ProfileID         `json:"profile"`
	Connection      Connection        `json:"connection"`
	System          string            `json:"system,omitempty"`
	Messages        []Message         `json:"messages,omitempty"`
	Tools           []ToolSpec        `json:"tools,omitempty"`
	ToolChoice      ToolChoice        `json:"tool_choice,omitempty"`
	Structured      *StructuredOutput `json:"structured,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	EmbedInputs     []string          `json:"embed_inputs,omitempty"`
	Timeout         time.Duration     `json:"timeout,omitempty"` // bounded per call
	// Metadata carries bounded non-secret correlation refs (job id,
	// correlation id, data classification label). It never carries
	// credentials, prompts, or provider-side state.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ClassificationKey is the metadata key carrying the workspace data
// classification label consulted by the C3-19 egress gate. An empty label
// is treated as protected (fail closed).
const ClassificationKey = "classification"

// Plan bounds (Proposed): explicit finite limits at the neutral boundary.
const (
	MaxMessages        = 256
	MaxTextRunes       = 1 << 20
	MaxTools           = 64
	MaxMetadataEntries = 16
	MaxMetadataValue   = 256
	MaxEmbedInputs     = 128
)

// Validate enforces the plan contract (C3-17 acceptance: provider SDK types
// stay behind adapters — the neutral plan itself is checked, bounded, and
// credential-free before any adapter sees it).
func (p *RequestPlan) Validate() error {
	if p.Operation != OperationChat && p.Operation != OperationEmbed {
		return fmt.Errorf("provider: unknown operation %q", p.Operation)
	}
	if err := p.Profile.Validate(); err != nil {
		return err
	}
	if p.Connection.BaseURL == "" {
		return fmt.Errorf("provider: connection base URL is required")
	}
	if strings.Contains(p.Connection.BaseURL, "@") {
		return fmt.Errorf("provider: credentials in URLs are refused (C3-18)")
	}
	if p.Connection.AuthKind != AuthNone && p.Connection.SecretRef == "" {
		return fmt.Errorf("provider: auth kind %q requires a secret reference, never an inline value", p.Connection.AuthKind)
	}
	if len(p.Messages) > MaxMessages {
		return fmt.Errorf("provider: message count %d exceeds the bound", len(p.Messages))
	}
	if len(p.Tools) > MaxTools {
		return fmt.Errorf("provider: tool count %d exceeds the bound", len(p.Tools))
	}
	if len(p.EmbedInputs) > MaxEmbedInputs {
		return fmt.Errorf("provider: embed input count %d exceeds the bound", len(p.EmbedInputs))
	}
	for i, m := range p.Messages {
		for j, part := range m.Parts {
			if part.Type != "text" && part.Type != "image" {
				return fmt.Errorf("provider: message %d part %d has unknown type %q", i, j, part.Type)
			}
			if len([]rune(part.Text)) > MaxTextRunes {
				return fmt.Errorf("provider: message %d part %d exceeds the text bound", i, j)
			}
			if part.Type == "image" && len(part.ImageData) == 0 {
				return fmt.Errorf("provider: message %d part %d is an image without inline bytes", i, j)
			}
		}
	}
	if len(p.Metadata) > MaxMetadataEntries {
		return fmt.Errorf("provider: metadata entries exceed the bound")
	}
	for k, v := range p.Metadata {
		if k == "" || len(v) > MaxMetadataValue {
			return fmt.Errorf("provider: metadata entry %q exceeds the bounds", k)
		}
	}
	return nil
}

// FinishReason is the neutral finish taxonomy.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishToolCalls     FinishReason = "tool_calls"
	FinishLength        FinishReason = "length"
	FinishContentFilter FinishReason = "content_filter"
	FinishUnknown       FinishReason = "unknown"
)

// Usage is the provider-reported token usage (C3-17/C3-20): recorded as
// provider metadata, never invented by the Core.
type Usage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens,omitempty"`
}

// Result is the neutral result of one successful provider call.
type Result struct {
	Text           string       `json:"text,omitempty"`
	ToolCalls      []ToolCall   `json:"tool_calls,omitempty"`
	StructuredJSON []byte       `json:"structured_json,omitempty"`
	Embeddings     [][]float64  `json:"embeddings,omitempty"` // OperationEmbed results
	FinishReason   FinishReason `json:"finish_reason"`
	Usage          Usage        `json:"usage"`
	ProviderRef    string       `json:"provider_ref,omitempty"` // provider-side response id (reconciliation evidence)
	ResultDigest   string       `json:"result_digest"`          // sha256 over the canonical result payload
	RawDigest      string       `json:"raw_digest"`             // sha256 over the raw response body received
}

// ResultPayload returns the canonical bytes the ResultDigest is computed
// over: the neutral result fields only, in a deterministic form.
func (r *Result) ResultPayload() []byte {
	payload := struct {
		Text           string       `json:"text,omitempty"`
		ToolCalls      []ToolCall   `json:"tool_calls,omitempty"`
		StructuredJSON string       `json:"structured_json,omitempty"`
		Embeddings     [][]float64  `json:"embeddings,omitempty"`
		FinishReason   FinishReason `json:"finish_reason"`
		Usage          Usage        `json:"usage"`
		ProviderRef    string       `json:"provider_ref,omitempty"`
	}{}
	if r.ToolCalls != nil {
		payload.ToolCalls = r.ToolCalls
	}
	if r.Embeddings != nil {
		payload.Embeddings = r.Embeddings
	}
	payload.StructuredJSON = string(r.StructuredJSON)
	payload.FinishReason = r.FinishReason
	payload.Usage = r.Usage
	payload.ProviderRef = r.ProviderRef
	payload.Text = r.Text
	b, _ := json.Marshal(payload)
	return b
}

// ErrorKind is the stable provider error taxonomy (C3-17). Retryability is
// a property of the kind; the relay's D-34 retry classification consumes it.
type ErrorKind string

const (
	ErrAuth             ErrorKind = "auth"
	ErrPermission       ErrorKind = "permission"
	ErrRateLimited      ErrorKind = "rate_limited"
	ErrTimeout          ErrorKind = "timeout"
	ErrCanceled         ErrorKind = "canceled"
	ErrInvalidRequest   ErrorKind = "invalid_request"
	ErrUnsupported      ErrorKind = "unsupported_capability"
	ErrEgressPolicy     ErrorKind = "egress_policy"
	ErrTLS              ErrorKind = "tls"
	ErrResponseTooLarge ErrorKind = "response_too_large"
	ErrMidStreamFailure ErrorKind = "mid_stream_failure"
	ErrProtocol         ErrorKind = "protocol"
	ErrUnreachable      ErrorKind = "unreachable"
	ErrUnknown          ErrorKind = "unknown"
)

// Retryable reports the neutral retry class. Timeout, unreachable,
// rate-limited, and mid-stream failures are retryable; a lost response that
// may have produced a side effect is NOT blindly retried — that ambiguity
// belongs to the relay's side-effect intent protocol (C3-14).
func (k ErrorKind) Retryable() bool {
	switch k {
	case ErrRateLimited, ErrTimeout, ErrUnreachable, ErrMidStreamFailure:
		return true
	}
	return false
}

// ProviderError is the stable neutral error. Detail is bounded and REDACTED
// before construction: resolved credential bytes must never appear here
// (C3-18).
type ProviderError struct {
	Kind       ErrorKind     `json:"kind"`
	Provider   string        `json:"provider"`
	Status     int           `json:"status,omitempty"`
	Detail     string        `json:"detail,omitempty"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
	Err        error         `json:"-"`
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("provider %s: %s: %s: %v", e.Provider, e.Kind, e.Detail, e.Err)
	}
	return fmt.Sprintf("provider %s: %s: %s", e.Provider, e.Kind, e.Detail)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// Retryable reports the neutral retry class of the error's kind.
func (e *ProviderError) Retryable() bool { return e.Kind.Retryable() }

// NewError builds a bounded provider error.
func NewError(kind ErrorKind, provider, detail string) *ProviderError {
	if len(detail) > 512 {
		detail = detail[:512]
	}
	return &ProviderError{Kind: kind, Provider: provider, Detail: detail}
}

// CapabilityState is the three-state capability fact (C3-17): supported,
// unsupported, or unknown — and unknown means unsupported wherever it gates
// a call.
type CapabilityState string

const (
	CapSupported   CapabilityState = "supported"
	CapUnsupported CapabilityState = "unsupported"
	CapUnknown     CapabilityState = "unknown"
)

// Allowed reports whether the capability may be used. Unknown is never
// allowed (fail closed).
func (c CapabilityState) Allowed() bool { return c == CapSupported }

// Feature enumerates the capability axes the plan and harness consume
// (C3-17): chat/streaming, tool use and parallel calls, structured output,
// vision, audio/multimodal, embeddings, batch/background, context caching,
// and conversation continuation.
type Feature string

const (
	FeatureChat           Feature = "chat"
	FeatureStreaming      Feature = "streaming"
	FeatureTools          Feature = "tools"
	FeatureParallelTools  Feature = "parallel_tool_calls"
	FeatureStructured     Feature = "structured_output"
	FeatureVision         Feature = "vision"
	FeatureAudio          Feature = "audio"
	FeatureEmbeddings     Feature = "embeddings"
	FeatureBatch          Feature = "batch"
	FeatureContextCaching Feature = "context_caching"
	FeatureContinuation   Feature = "conversation_continuation"
)

// CapabilitySet records one profile's capability facts. Limits of 0 mean
// unknown; model names, prices, and API versions are refreshable metadata
// and are recorded as profile metadata, never as code constants of behavior.
type CapabilitySet struct {
	Chat                     CapabilityState `json:"chat"`
	Streaming                CapabilityState `json:"streaming"`
	Tools                    CapabilityState `json:"tools"`
	ParallelToolCalls        CapabilityState `json:"parallel_tool_calls"`
	StructuredOutput         CapabilityState `json:"structured_output"`
	Vision                   CapabilityState `json:"vision"`
	Audio                    CapabilityState `json:"audio"`
	Embeddings               CapabilityState `json:"embeddings"`
	Batch                    CapabilityState `json:"batch"`
	ContextCaching           CapabilityState `json:"context_caching"`
	ConversationContinuation CapabilityState `json:"conversation_continuation"`
	MaxContextTokens         int             `json:"max_context_tokens,omitempty"` // 0 = unknown
	MaxOutputTokens          int             `json:"max_output_tokens,omitempty"`  // 0 = unknown
	TokenEstimatorRef        string          `json:"token_estimator_ref,omitempty"`
	Notes                    string          `json:"notes,omitempty"`
}

// Supported reports the fail-closed view of one feature.
func (c CapabilitySet) Supported(f Feature) bool {
	var s CapabilityState
	switch f {
	case FeatureChat:
		s = c.Chat
	case FeatureStreaming:
		s = c.Streaming
	case FeatureTools:
		s = c.Tools
	case FeatureParallelTools:
		s = c.ParallelToolCalls
	case FeatureStructured:
		s = c.StructuredOutput
	case FeatureVision:
		s = c.Vision
	case FeatureAudio:
		s = c.Audio
	case FeatureEmbeddings:
		s = c.Embeddings
	case FeatureBatch:
		s = c.Batch
	case FeatureContextCaching:
		s = c.ContextCaching
	case FeatureContinuation:
		s = c.ConversationContinuation
	default:
		return false
	}
	return s.Allowed()
}

// FeatureForPlan maps a request plan to the feature that must be supported
// before the call (C3-20: capability-checked provider calls).
func FeatureForPlan(p *RequestPlan) Feature {
	if p.Operation == OperationEmbed {
		return FeatureEmbeddings
	}
	switch {
	case p.Structured != nil:
		return FeatureStructured
	case p.Stream:
		return FeatureStreaming
	default:
		return FeatureChat
	}
}
