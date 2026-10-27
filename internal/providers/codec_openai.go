package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The OpenAI-compatible chat-completions family codec (C3-17/C3-21 and the
// compatible/custom/local families of C3-24/C3-26/C3-28/C3-29/C3-30). Wire
// shapes live HERE only; the neutral contract never leaks them outward.
// Differences between family members are expressed as codec options
// (token-limit field name), never by forking the neutral contract.

type openAIOptions struct {
	MaxTokensField string // "max_completion_tokens" (OpenAI direct) or "max_tokens" (compatible families)
}

func defaultOpenAIOptions() openAIOptions {
	return openAIOptions{MaxTokensField: "max_completion_tokens"}
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function openAIToolCallFn `json:"function"`
}

type openAIToolCallFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAITool struct {
	Type     string        `json:"type"`
	Function openAIToolDef `json:"function"`
}

type openAIToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIJSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict,omitempty"`
	Schema json.RawMessage `json:"schema"`
}

// buildOpenAIChatBody encodes the neutral plan into the family's wire body.
func buildOpenAIChatBody(plan *dprovider.RequestPlan, opts openAIOptions) ([]byte, error) {
	req := map[string]any{
		"model": plan.Profile.Model,
	}
	msgs := make([]openAIMessage, 0, len(plan.Messages)+1)
	if plan.System != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: plan.System})
	}
	for _, m := range plan.Messages {
		wire := openAIMessage{Role: string(m.Role)}
		hasImage := false
		for _, p := range m.Parts {
			if p.Type == "image" {
				hasImage = true
			}
		}
		switch {
		case hasImage:
			parts := make([]openAIContentPart, 0, len(m.Parts))
			for _, p := range m.Parts {
				if p.Type == "text" {
					parts = append(parts, openAIContentPart{Type: "text", Text: p.Text})
				} else {
					parts = append(parts, openAIContentPart{Type: "image_url",
						ImageURL: &openAIImageURL{URL: dataURL(p.ImageMIME, p.ImageData)}})
				}
			}
			wire.Content = parts
		case len(m.Parts) > 0:
			wire.Content = m.Text()
		default:
			wire.Content = ""
		}
		wire.ToolCallID = m.ToolCallID
		for _, tc := range m.ToolCalls {
			wire.ToolCalls = append(wire.ToolCalls, openAIToolCall{ID: tc.ID, Type: "function",
				Function: openAIToolCallFn{Name: tc.Name, Arguments: tc.ArgumentsJSON}})
		}
		msgs = append(msgs, wire)
	}
	req["messages"] = msgs
	if len(plan.Tools) > 0 {
		tools := make([]openAITool, 0, len(plan.Tools))
		for _, t := range plan.Tools {
			tools = append(tools, openAITool{Type: "function", Function: openAIToolDef{
				Name: t.Name, Description: t.Description, Parameters: t.ParametersSchema}})
		}
		req["tools"] = tools
		switch {
		case plan.ToolChoice == dprovider.ToolChoiceAuto:
			req["tool_choice"] = "auto"
		case plan.ToolChoice == dprovider.ToolChoiceNone:
			req["tool_choice"] = "none"
		case plan.ToolChoice == dprovider.ToolChoiceRequired:
			req["tool_choice"] = "required"
		case strings.HasPrefix(string(plan.ToolChoice), "tool:"):
			req["tool_choice"] = map[string]any{"type": "function",
				"function": map[string]any{"name": strings.TrimPrefix(string(plan.ToolChoice), "tool:")}}
		}
	}
	if plan.Structured != nil {
		req["response_format"] = openAIResponseFormat{Type: "json_schema",
			JSONSchema: &openAIJSONSchema{Name: plan.Structured.Name, Strict: plan.Structured.Strict, Schema: plan.Structured.Schema}}
	}
	if plan.Stream {
		req["stream"] = true
	}
	if plan.MaxOutputTokens > 0 {
		req[opts.MaxTokensField] = plan.MaxOutputTokens
	}
	if plan.Temperature != nil {
		req["temperature"] = *plan.Temperature
	}
	return json.Marshal(req)
}

// buildOpenAIEmbedBody encodes the embeddings request.
func buildOpenAIEmbedBody(plan *dprovider.RequestPlan) ([]byte, error) {
	return json.Marshal(map[string]any{"model": plan.Profile.Model, "input": plan.EmbedInputs})
}

// openAIRequest is the shared family request builder used by the adapters.
func openAIRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string, path string, opts openAIOptions) (*http.Request, error) {
	u, err := ValidateDestinationURL(e.Policy, joinURL(plan.Connection.BaseURL, path))
	if err != nil {
		return nil, err
	}
	var body []byte
	switch plan.Operation {
	case dprovider.OperationEmbed:
		body, err = buildOpenAIEmbedBody(plan)
	default:
		body, err = buildOpenAIChatBody(plan, opts)
	}
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "request encoding failed: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "request build failed")
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range plan.Connection.ExtraHeaders {
		if err := ValidateExtraHeader(name, value); err != nil {
			return nil, err
		}
		req.Header.Set(name, value)
	}
	SetCredentialHeader(req, "Authorization", bearerValue(secret))
	return req, nil
}

func bearerValue(secret string) string {
	if secret == "" {
		return ""
	}
	return "Bearer " + secret
}

func dataURL(mime string, data []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// joinURL appends path to the base without allowing the path to escape the
// base (no "../", no scheme changes).
func joinURL(base, path string) string {
	b := strings.TrimSuffix(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return b + path
}

// decodeOpenAIChat parses a complete chat-completions response.
func decodeOpenAIChat(providerID string, status int, body []byte, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []openAIToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage openAIUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "response is not the expected JSON shape")
	}
	if len(resp.Choices) == 0 {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "response carries no choices")
	}
	c := resp.Choices[0]
	result := &dprovider.Result{
		Text:         c.Message.Content,
		FinishReason: mapFinishReason(c.FinishReason),
		Usage:        resp.Usage.neutral(),
		ProviderRef:  resp.ID,
	}
	for _, tc := range c.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, dprovider.ToolCall{ID: tc.ID, Name: tc.Function.Name, ArgumentsJSON: tc.Function.Arguments})
	}
	// Structured output arrives as the message content: the JSON document is
	// lifted into StructuredJSON so the executor can validate it against the
	// selected schema before any result commit (C3-20).
	if plan != nil && plan.Structured != nil && result.Text != "" {
		result.StructuredJSON = []byte(result.Text)
		result.Text = ""
	}
	return result, nil
}

// decodeOpenAIEmbed parses a complete embeddings response.
func decodeOpenAIEmbed(providerID string, status int, body []byte) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage openAIUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "embedding response is not the expected JSON shape")
	}
	result := &dprovider.Result{FinishReason: dprovider.FinishStop, Usage: resp.Usage.neutral()}
	for _, d := range resp.Data {
		result.Embeddings = append(result.Embeddings, d.Embedding)
	}
	return result, nil
}

type openAIUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) neutral() dprovider.Usage {
	out := dprovider.Usage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens}
	if u.PromptTokensDetails != nil {
		out.CachedInputTokens = u.PromptTokensDetails.CachedTokens
	}
	return out
}

func mapFinishReason(s string) dprovider.FinishReason {
	switch s {
	case "stop":
		return dprovider.FinishStop
	case "tool_calls", "function_call":
		return dprovider.FinishToolCalls
	case "length", "max_tokens":
		return dprovider.FinishLength
	case "content_filter":
		return dprovider.FinishContentFilter
	default:
		return dprovider.FinishUnknown
	}
}

// openAIStreamDecoder is the family SSE decoder: `data: {json}` frames, a
// `[DONE]` sentinel, and index-keyed tool-call delta accumulation. Tool
// calls are emitted COMPLETE at finish; partial argument fragments are
// never presented as callable output.
type openAIStreamDecoder struct {
	tools map[int]*openAIToolCall
	order []int
}

func newOpenAIStreamDecoder() *openAIStreamDecoder {
	return &openAIStreamDecoder{tools: map[int]*openAIToolCall{}}
}

func (d *openAIStreamDecoder) Feed(line string) ([]StreamDelta, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
		return nil, nil
	}
	payload, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return nil, nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "[DONE]" {
		return nil, nil
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string           `json:"content"`
				ToolCalls []openAIToolCall `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *openAIUsage     `json:"usage"`
		Error *openAIErrorBody `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, "openai-family", "stream frame is not the expected JSON shape")
	}
	if chunk.Error != nil {
		return nil, dprovider.NewError(dprovider.ErrMidStreamFailure, "openai-family",
			"provider streamed an error frame: "+bounded(chunk.Error.Message))
	}
	var out []StreamDelta
	for _, c := range chunk.Choices {
		if c.Delta.Content != "" {
			out = append(out, StreamDelta{Text: c.Delta.Content})
		}
		for _, tc := range c.Delta.ToolCalls {
			idx := tc.Index
			existing, ok := d.tools[idx]
			if !ok {
				clone := tc
				d.tools[idx] = &clone
				d.order = append(d.order, idx)
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Function.Name += tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
		if fr := mapFinishReason(c.FinishReason); fr != dprovider.FinishUnknown {
			delta := StreamDelta{Finish: fr}
			if fr == dprovider.FinishToolCalls && len(d.order) > 0 {
				for _, idx := range d.order {
					tc := d.tools[idx]
					delta.ToolCalls = append(delta.ToolCalls, dprovider.ToolCall{
						ID: tc.ID, Name: tc.Function.Name, ArgumentsJSON: tc.Function.Arguments})
				}
			}
			out = append(out, delta)
		}
	}
	if chunk.Usage != nil {
		out = append(out, StreamDelta{Usage: chunk.Usage.neutral()})
	}
	return out, nil
}

type openAIErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}
