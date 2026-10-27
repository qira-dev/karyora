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

// The Anthropic messages-family codec (C3-17/C3-22 and the compatible
// subset of C3-30). Native differences from the OpenAI family are preserved
// here rather than hidden: the system prompt is a top-level field, max
// output tokens are REQUIRED by the protocol, content is a typed block
// list, and the stream is event-framed SSE.

type anthropicBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// image block
	Source *anthropicSource `json:"source,omitempty"`
	// tool_use / tool_result
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"` // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func buildAnthropicBody(plan *dprovider.RequestPlan) ([]byte, error) {
	maxTokens := plan.MaxOutputTokens
	if maxTokens <= 0 {
		// The protocol requires the field; the neutral contract's zero means
		// unspecified, so the family default is recorded, not invented per call.
		maxTokens = 1024
	}
	req := map[string]any{
		"model":      plan.Profile.Model,
		"max_tokens": maxTokens,
	}
	if plan.System != "" {
		req["system"] = plan.System
	}
	msgs := make([]anthropicMessage, 0, len(plan.Messages))
	for _, m := range plan.Messages {
		switch m.Role {
		case dprovider.RoleTool:
			// tool results travel as user-role tool_result blocks
			raw := json.RawMessage(m.Text())
			if !json.Valid(raw) {
				raw, _ = json.Marshal(map[string]string{"result": m.Text()})
			}
			block := anthropicBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: raw}
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicBlock{block}})
		case dprovider.RoleAssistant:
			blocks := make([]anthropicBlock, 0, len(m.ToolCalls)+1)
			if t := m.Text(); t != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: t})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.ArgumentsJSON)
				if !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
			msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
		default:
			blocks := make([]anthropicBlock, 0, len(m.Parts)+1)
			if t := m.Text(); t != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: t})
			}
			for _, p := range m.Parts {
				if p.Type == "image" {
					blocks = append(blocks, anthropicBlock{Type: "image", Source: &anthropicSource{
						Type: "base64", MediaType: p.ImageMIME, Data: base64.StdEncoding.EncodeToString(p.ImageData)}})
				}
			}
			msgs = append(msgs, anthropicMessage{Role: "user", Content: blocks})
		}
	}
	req["messages"] = msgs
	if len(plan.Tools) > 0 {
		tools := make([]anthropicToolDef, 0, len(plan.Tools))
		for _, t := range plan.Tools {
			schema := t.ParametersSchema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			tools = append(tools, anthropicToolDef{Name: t.Name, Description: t.Description, InputSchema: schema})
		}
		req["tools"] = tools
	}
	switch plan.ToolChoice {
	case dprovider.ToolChoiceAuto, "":
		req["tool_choice"] = map[string]any{"type": "auto"}
	case dprovider.ToolChoiceNone:
		// no tool_choice field: the tools are simply not sent
		delete(req, "tools")
	case dprovider.ToolChoiceRequired:
		req["tool_choice"] = map[string]any{"type": "any"}
	default:
		if strings.HasPrefix(string(plan.ToolChoice), "tool:") {
			req["tool_choice"] = map[string]any{"type": "tool", "name": strings.TrimPrefix(string(plan.ToolChoice), "tool:")}
		}
	}
	if plan.Stream {
		req["stream"] = true
	}
	if plan.Temperature != nil {
		req["temperature"] = *plan.Temperature
	}
	return json.Marshal(req)
}

// anthropicRequest builds the wire request for the messages family.
func anthropicRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string, path string) (*http.Request, error) {
	u, err := ValidateDestinationURL(e.Policy, joinURL(plan.Connection.BaseURL, path))
	if err != nil {
		return nil, err
	}
	body, err := buildAnthropicBody(plan)
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "request encoding failed: "+err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "request build failed")
	}
	req.Header.Set("Content-Type", "application/json")
	if v := plan.Connection.ProtocolVersion; v != "" {
		req.Header.Set("anthropic-version", v)
	}
	for name, value := range plan.Connection.ExtraHeaders {
		if err := ValidateExtraHeader(name, value); err != nil {
			return nil, err
		}
		req.Header.Set(name, value)
	}
	SetCredentialHeader(req, "x-api-key", secret)
	return req, nil
}

// decodeAnthropic parses a complete messages response.
func decodeAnthropic(providerID string, status int, body []byte, _ *dprovider.RequestPlan) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		ID      string `json:"id"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "response is not the expected JSON shape")
	}
	result := &dprovider.Result{
		FinishReason: mapAnthropicStop(resp.StopReason),
		Usage:        dprovider.Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens},
		ProviderRef:  resp.ID,
	}
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			result.Text += b.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, dprovider.ToolCall{
				ID: b.ID, Name: b.Name, ArgumentsJSON: string(b.Input)})
		}
	}
	return result, nil
}

func mapAnthropicStop(s string) dprovider.FinishReason {
	switch s {
	case "end_turn", "stop_sequence":
		return dprovider.FinishStop
	case "tool_use":
		return dprovider.FinishToolCalls
	case "max_tokens":
		return dprovider.FinishLength
	case "refusal":
		return dprovider.FinishContentFilter
	default:
		return dprovider.FinishUnknown
	}
}

// anthropicStreamDecoder is the event-framed SSE decoder: `event: X` lines
// select the meaning of the following `data:` line. Tool calls accumulate
// from input_json_delta fragments and are emitted complete at message_stop.
type anthropicStreamDecoder struct {
	pendingEvent string
	text         strings.Builder
	toolIdx      int
	toolIDs      map[int]string
	toolNames    map[int]string
	toolArgs     map[int]*strings.Builder
	finish       dprovider.FinishReason
	usage        dprovider.Usage
	usageSeen    bool
	finishSeen   bool
}

func newAnthropicStreamDecoder() *anthropicStreamDecoder {
	return &anthropicStreamDecoder{
		toolIDs: map[int]string{}, toolNames: map[int]string{}, toolArgs: map[int]*strings.Builder{},
	}
}

func (d *anthropicStreamDecoder) Feed(line string) ([]StreamDelta, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, ":") {
		return nil, nil
	}
	if ev, ok := strings.CutPrefix(trimmed, "event:"); ok {
		d.pendingEvent = strings.TrimSpace(ev)
		return nil, nil
	}
	payload, ok := strings.CutPrefix(trimmed, "data:")
	if !ok {
		return nil, nil
	}
	payload = strings.TrimSpace(payload)

	switch d.pendingEvent {
	case "message_start":
		var frame struct {
			Message struct {
				ID    string `json:"id"`
				Usage struct {
					InputTokens int64 `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, "anthropic", "message_start frame is not the expected shape")
		}
		d.usage.InputTokens = frame.Message.Usage.InputTokens
		d.usageSeen = true
		return nil, nil
	case "content_block_start":
		var frame struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, "anthropic", "content_block_start frame is not the expected shape")
		}
		if frame.ContentBlock.Type == "tool_use" {
			d.toolIDs[frame.Index] = frame.ContentBlock.ID
			d.toolNames[frame.Index] = frame.ContentBlock.Name
			d.toolArgs[frame.Index] = &strings.Builder{}
		}
		return nil, nil
	case "content_block_delta":
		var frame struct {
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, "anthropic", "content_block_delta frame is not the expected shape")
		}
		switch frame.Delta.Type {
		case "text_delta":
			d.text.WriteString(frame.Delta.Text)
			return []StreamDelta{{Text: frame.Delta.Text}}, nil
		case "input_json_delta":
			if b, ok := d.toolArgs[d.toolIdxFromPayload(payload)]; ok {
				// the frame carries no index in this shape; the decoder keeps a
				// single open tool block index tracked at content_block_start
				b.WriteString(frame.Delta.PartialJSON)
			}
			return nil, nil
		}
		return nil, nil
	case "message_delta":
		var frame struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, "anthropic", "message_delta frame is not the expected shape")
		}
		d.finish = mapAnthropicStop(frame.Delta.StopReason)
		d.finishSeen = true
		d.usage.OutputTokens = frame.Usage.OutputTokens
		var out []StreamDelta
		if fr := d.finish; fr == dprovider.FinishToolCalls {
			out = append(out, StreamDelta{Finish: fr, ToolCalls: d.completedTools(), Usage: d.usage})
		} else {
			out = append(out, StreamDelta{Finish: fr, Usage: d.usage})
		}
		return out, nil
	case "error":
		var frame struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal([]byte(payload), &frame)
		return nil, dprovider.NewError(dprovider.ErrMidStreamFailure, "anthropic",
			"provider streamed an error event: "+bounded(frame.Error.Message))
	default:
		return nil, nil
	}
}

// completedTools emits the accumulated tool calls in block order.
func (d *anthropicStreamDecoder) completedTools() []dprovider.ToolCall {
	var out []dprovider.ToolCall
	for i := 0; i <= d.toolIdx; i++ {
		id, okID := d.toolIDs[i]
		name, okName := d.toolNames[i]
		if !okID && !okName {
			continue
		}
		args := "{}"
		if b, ok := d.toolArgs[i]; ok && b.Len() > 0 {
			args = b.String()
		}
		out = append(out, dprovider.ToolCall{ID: id, Name: name, ArgumentsJSON: args})
	}
	return out
}

// toolIdxFromPayload extracts the index field for input_json_delta frames.
func (d *anthropicStreamDecoder) toolIdxFromPayload(payload string) int {
	var frame struct {
		Index int `json:"index"`
	}
	_ = json.Unmarshal([]byte(payload), &frame)
	if frame.Index > d.toolIdx {
		d.toolIdx = frame.Index
	}
	return frame.Index
}
