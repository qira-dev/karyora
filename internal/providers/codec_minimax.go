package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// The MiniMax chatcompletion_v2 codec (C3-17/C3-27). This adapter records
// the tested subset exactly: the request shape below and the response
// fields below are the fixture-defined protocol cases (C3-20: provider
// claims are not inferred from protocol labels — everything not in the
// recorded fixtures stays unknown/unsupported). The family's base_resp
// status object is the documented error carrier and is mapped explicitly.

type miniMaxPart struct {
	Type string `json:"type"` // "text"
	Text string `json:"text,omitempty"`
}

type miniMaxMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string, or []miniMaxPart with image parts
	Name    string `json:"name,omitempty"`
}

type miniMaxImagePart struct {
	Type     string `json:"type"` // "image_url"
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

func buildMiniMaxBody(plan *dprovider.RequestPlan) ([]byte, error) {
	if plan.Operation == dprovider.OperationEmbed {
		return nil, dprovider.NewError(dprovider.ErrUnsupported, dprovider.ProviderMiniMax, "embeddings are not part of the recorded MiniMax subset")
	}
	req := map[string]any{"model": plan.Profile.Model}
	msgs := make([]miniMaxMessage, 0, len(plan.Messages)+1)
	if plan.System != "" {
		msgs = append(msgs, miniMaxMessage{Role: "system", Content: plan.System})
	}
	for _, m := range plan.Messages {
		wire := miniMaxMessage{Role: string(m.Role)}
		hasImage := false
		for _, p := range m.Parts {
			if p.Type == "image" {
				hasImage = true
			}
		}
		if hasImage {
			parts := []any{}
			for _, p := range m.Parts {
				if p.Type == "text" {
					parts = append(parts, miniMaxPart{Type: "text", Text: p.Text})
				} else {
					var ip miniMaxImagePart
					ip.Type = "image_url"
					ip.ImageURL.URL = dataURL(p.ImageMIME, p.ImageData)
					parts = append(parts, ip)
				}
			}
			wire.Content = parts
		} else {
			wire.Content = m.Text()
		}
		msgs = append(msgs, wire)
	}
	req["messages"] = msgs
	if plan.MaxOutputTokens > 0 {
		req["max_tokens"] = plan.MaxOutputTokens
	}
	if plan.Temperature != nil {
		req["temperature"] = *plan.Temperature
	}
	return json.Marshal(req)
}

func miniMaxRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	u, err := ValidateDestinationURL(e.Policy, joinURL(plan.Connection.BaseURL, "/text/chatcompletion_v2"))
	if err != nil {
		return nil, err
	}
	body, err := buildMiniMaxBody(plan)
	if err != nil {
		return nil, err
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

// decodeMiniMax parses the recorded response subset: id, choices, usage
// token counts, and the base_resp status carrier (a non-zero base_resp
// status_code is a provider failure even on HTTP 200).
func decodeMiniMax(providerID string, status int, body []byte, _ *dprovider.RequestPlan) (*dprovider.Result, error) {
	if status < 200 || status > 299 {
		return nil, mapStatusError(providerID, status, body, 0)
	}
	var resp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, dprovider.NewError(dprovider.ErrProtocol, providerID, "response is not the expected JSON shape")
	}
	if resp.BaseResp.StatusCode != 0 {
		pe := dprovider.NewError(dprovider.ErrUnreachable, providerID,
			"provider base_resp status "+bounded(strings.TrimSpace(resp.BaseResp.StatusMsg)))
		pe.Status = resp.BaseResp.StatusCode
		return nil, pe
	}
	result := &dprovider.Result{
		FinishReason: mapFinishReason(resp.Choices[0].FinishReason),
		ProviderRef:  resp.ID,
	}
	if len(resp.Choices) > 0 {
		result.Text = resp.Choices[0].Message.Content
	}
	result.Usage = dprovider.Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	if result.Usage.InputTokens == 0 && result.Usage.OutputTokens == 0 {
		// the family sometimes reports only the aggregate total: recorded
		// as the aggregate, never split by guessing
		result.Usage.InputTokens = resp.Usage.TotalTokens
	}
	return result, nil
}
