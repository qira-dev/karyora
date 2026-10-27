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

// The Google Gemini generateContent codec (C3-17/C3-23). Native shape:
// contents/parts, systemInstruction, functionDeclarations, and a
// generationConfig that carries the structured-output mime type and schema.
// The credential travels ONLY in the x-goog-api-key header — the query
// parameter form exists but is refused here because credentials never
// travel in URLs (C3-18).

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

func buildGeminiBody(plan *dprovider.RequestPlan) ([]byte, error) {
	req := map[string]any{}
	contents := make([]geminiContent, 0, len(plan.Messages))
	for _, m := range plan.Messages {
		var role string
		switch m.Role {
		case dprovider.RoleAssistant, dprovider.RoleSystem:
			role = "model"
		default:
			role = "user"
		}
		parts := make([]geminiPart, 0, len(m.Parts)+1)
		if t := m.Text(); t != "" {
			parts = append(parts, geminiPart{Text: t})
		}
		for _, p := range m.Parts {
			if p.Type == "image" {
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{
					MimeType: p.ImageMIME, Data: base64.StdEncoding.EncodeToString(p.ImageData)}})
			}
		}
		contents = append(contents, geminiContent{Role: role, Parts: parts})
	}
	req["contents"] = contents
	if plan.System != "" {
		req["systemInstruction"] = geminiContent{Parts: []geminiPart{{Text: plan.System}}}
	}
	if len(plan.Tools) > 0 {
		decls := make([]geminiFunctionDef, 0, len(plan.Tools))
		for _, t := range plan.Tools {
			decls = append(decls, geminiFunctionDef{Name: t.Name, Description: t.Description, Parameters: t.ParametersSchema})
		}
		req["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}
	gen := map[string]any{}
	if plan.Structured != nil {
		gen["responseMimeType"] = "application/json"
		gen["responseSchema"] = plan.Structured.Schema
	}
	if plan.MaxOutputTokens > 0 {
		gen["maxOutputTokens"] = plan.MaxOutputTokens
	}
	if plan.Temperature != nil {
		gen["temperature"] = *plan.Temperature
	}
	if len(gen) > 0 {
		req["generationConfig"] = gen
	}
	return json.Marshal(req)
}

// geminiMethod selects the action endpoint for the operation.
func geminiMethod(plan *dprovider.RequestPlan) string {
	if plan.Operation == dprovider.OperationEmbed {
		return "models/" + plan.Profile.Model + ":embedContent"
	}
	return "models/" + plan.Profile.Model + ":generateContent"
}

func geminiRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	u, err := ValidateDestinationURL(e.Policy, joinURL(plan.Connection.BaseURL, geminiMethod(plan)))
	if err != nil {
		return nil, err
	}
	body, err := buildGeminiBody(plan)
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "request encoding failed: "+err.Error())
	}
	method := http.MethodPost
	if plan.Operation == dprovider.OperationEmbed {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
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
	SetCredentialHeader(req, "x-goog-api-key", secret)
	return req, nil
}

// decodeGemini parses a complete generateContent response.
func decodeGemini(providerID string, status int, body []byte, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string              `json:"text"`
					FunctionCall *geminiFunctionCall `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount        int64 `json:"promptTokenCount"`
			CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
			CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "response is not the expected JSON shape")
	}
	result := &dprovider.Result{
		Usage: dprovider.Usage{
			InputTokens:       resp.UsageMetadata.PromptTokenCount,
			OutputTokens:      resp.UsageMetadata.CandidatesTokenCount,
			CachedInputTokens: resp.UsageMetadata.CachedContentTokenCount,
		},
	}
	if len(resp.Candidates) > 0 {
		c := resp.Candidates[0]
		result.FinishReason = mapGeminiFinish(c.FinishReason)
		for _, p := range c.Content.Parts {
			if p.FunctionCall != nil {
				result.ToolCalls = append(result.ToolCalls, dprovider.ToolCall{
					Name: p.FunctionCall.Name, ArgumentsJSON: string(p.FunctionCall.Args)})
			} else {
				result.Text += p.Text
			}
		}
	}
	// Structured output arrives as the candidate text: lifted into
	// StructuredJSON for the executor's pre-commit schema validation.
	if plan != nil && plan.Structured != nil && result.Text != "" {
		result.StructuredJSON = []byte(result.Text)
		result.Text = ""
	}
	return result, nil
}

// decodeGeminiEmbed parses a complete embedContent response.
func decodeGeminiEmbed(providerID string, status int, body []byte) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "embedding response is not the expected JSON shape")
	}
	return &dprovider.Result{
		FinishReason: dprovider.FinishStop,
		Embeddings:   [][]float64{resp.Embedding.Values},
	}, nil
}

func mapGeminiFinish(s string) dprovider.FinishReason {
	switch s {
	case "STOP":
		return dprovider.FinishStop
	case "MAX_TOKENS":
		return dprovider.FinishLength
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "RECITATION":
		return dprovider.FinishContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return dprovider.FinishUnknown
	default:
		return dprovider.FinishUnknown
	}
}

// geminiStreamDecoder parses the alt=sse frames of streamGenerateContent:
// each `data:` line is a GenerateContentResponse with partial content and,
// at the end, usageMetadata and a finishReason.
type geminiStreamDecoder struct {
	text       strings.Builder
	finish     dprovider.FinishReason
	usage      dprovider.Usage
	finishSeen bool
}

func newGeminiStreamDecoder() *geminiStreamDecoder { return &geminiStreamDecoder{} }

func (d *geminiStreamDecoder) Feed(line string) ([]StreamDelta, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, ":") || !strings.HasPrefix(trimmed, "data:") {
		return nil, nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" {
		return nil, nil
	}
	var frame struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string              `json:"text"`
					FunctionCall *geminiFunctionCall `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, "gemini", "stream frame is not the expected JSON shape")
	}
	var out []StreamDelta
	for _, c := range frame.Candidates {
		for _, p := range c.Content.Parts {
			if p.FunctionCall != nil {
				// function calls arrive whole; structured streaming is refused
				// by the executor, tool-call streaming emits the call at finish
				d.finish = mapGeminiFinish(c.FinishReason)
			} else if p.Text != "" {
				d.text.WriteString(p.Text)
				out = append(out, StreamDelta{Text: p.Text})
			}
		}
		if fr := mapGeminiFinish(c.FinishReason); fr != dprovider.FinishUnknown {
			d.finish = fr
			d.finishSeen = true
		}
	}
	if frame.UsageMetadata != nil {
		d.usage.InputTokens = frame.UsageMetadata.PromptTokenCount
		d.usage.OutputTokens = frame.UsageMetadata.CandidatesTokenCount
	}
	if d.finishSeen {
		out = append(out, StreamDelta{Finish: d.finish, Usage: d.usage})
	}
	return out, nil
}
