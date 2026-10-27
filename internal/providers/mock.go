package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// MockAdapter is the recorded fixture adapter (C3-20 "recorded/mock
// fixtures" and the CR-3.5 vertical slice's capability-checked provider
// call). It performs NO network I/O and makes NO provider claims: its
// data-handling profile verifies only the workspace's own test-fixture
// classification, so real protected data is still refused. It must never
// be enabled for real workspaces without an owner decision.

// MockBehavior scripts one mock adapter for tests and the harness.
type MockBehavior struct {
	// FailWith, when set, makes every call return this error.
	FailWith error
	// InvalidStructuredJSON makes structured calls emit schema-invalid JSON
	// (the C3-32 output-schema-failure case).
	InvalidStructuredJSON bool
	// NoFinishOnStream makes streams end without a finish reason (the
	// mid-stream-failure case).
	NoFinishOnStream bool
	// Prefix prepends a fixed marker to generated text.
	Prefix string
}

// MockAdapter implements Adapter against MockBehavior.
type MockAdapter struct {
	mu       sync.Mutex
	behavior MockBehavior
	calls    int
}

// NewMockAdapter builds a scripted fixture adapter.
func NewMockAdapter(b MockBehavior) *MockAdapter { return &MockAdapter{behavior: b} }

// Calls reports how many executions were attempted (harness assertion aid).
func (m *MockAdapter) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *MockAdapter) ID() string { return dprovider.ProviderMock }

func (m *MockAdapter) DefaultProfile(model string) dprovider.ProfileID {
	return dprovider.ProfileID{Provider: dprovider.ProviderMock, Endpoint: "fixture", Model: model}
}

func (m *MockAdapter) Capabilities(p dprovider.ProfileID) dprovider.CapabilitySet {
	c := capabilities(sup(dprovider.FeatureChat, dprovider.FeatureStructured, dprovider.FeatureStreaming,
		dprovider.FeatureEmbeddings))
	c.Notes = "recorded fixture adapter; no network; claims verified against this workspace's own fixtures only"
	return c
}

func (m *MockAdapter) AuthHeaderName(p dprovider.ProfileID) string { return "X-Mock-Credential" }

// SeedDataHandling verifies ONLY the test-fixture classification: the mock
// is this workspace's own code, so its facts are known and dated to the
// implementation record. Real classifications stay blocked.
func (m *MockAdapter) SeedDataHandling(p dprovider.ProfileID) dprovider.DataHandlingProfile {
	return dprovider.DataHandlingProfile{
		Profile:                p,
		Deployment:             dprovider.DeploymentLocal,
		TrainingUse:            dprovider.FactNo,
		AbuseRetention:         dprovider.FactNo,
		ApplicationStateTTL:    dprovider.FactNo,
		Subprocessors:          dprovider.FactNo,
		DeletionEligibility:    "deletion_supported",
		ReviewDate:             time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		Reviewer:               "workspace implementation record (C3-20 mock fixture)",
		AllowedClassifications: []string{dprovider.ClassificationTestFixture},
		Notes:                  "fixture-only adapter; no network; no provider policy claim",
	}
}

func (m *MockAdapter) NewRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	// The mock performs no I/O: NewRequest exists to satisfy the adapter
	// contract shape and to refuse invalid plans exactly like the real ones.
	// A nil request tells the executor to go straight to the decoder path
	// with every gate already applied.
	if err := plan.Validate(); err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, m.ID(), err.Error())
	}
	// The fixture adapter validates its destination exactly like the network
	// adapters, so URL-policy refusals surface identically (C3-18).
	if _, err := ValidateDestinationURL(e.Policy, plan.Connection.BaseURL); err != nil {
		return nil, err
	}
	return nil, nil
}

func (m *MockAdapter) DecodeResponse(status int, body []byte, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	return m.result(plan)
}

func (m *MockAdapter) NewStreamDecoder(p dprovider.ProfileID) StreamDecoder {
	return &mockStreamDecoder{adapter: m}
}

func (m *MockAdapter) TestConnectionRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error) {
	return nil, nil
}

func (m *MockAdapter) InterpretProbe(status int) error { return nil }

// result produces the scripted neutral result for one plan.
func (m *MockAdapter) result(plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	m.mu.Lock()
	m.calls++
	behavior := m.behavior
	m.mu.Unlock()
	if behavior.FailWith != nil {
		return nil, behavior.FailWith
	}
	if plan.Operation == dprovider.OperationEmbed {
		return &dprovider.Result{
			FinishReason: dprovider.FinishStop,
			Embeddings:   [][]float64{{1, 0, 0}, {0, 1, 0}},
			Usage:        dprovider.Usage{InputTokens: int64(len(plan.EmbedInputs)), OutputTokens: 0},
			ProviderRef:  "mock-embed",
		}, nil
	}
	last := ""
	for i := len(plan.Messages) - 1; i >= 0; i-- {
		if plan.Messages[i].Role == dprovider.RoleUser {
			last = plan.Messages[i].Text()
			break
		}
	}
	result := &dprovider.Result{
		Text:         behavior.Prefix + "mock-reply:" + last,
		FinishReason: dprovider.FinishStop,
		Usage: dprovider.Usage{
			InputTokens:  int64(len([]rune(last)) / 4),
			OutputTokens: 8,
		},
		ProviderRef: "mock-" + fmt.Sprint(m.Calls()),
	}
	if plan.Structured != nil {
		doc, err := fixtureForSchema(plan.Structured.Schema, behavior.InvalidStructuredJSON)
		if err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, m.ID(), "fixture generation failed: "+err.Error())
		}
		result.StructuredJSON = doc
		result.Text = ""
	}
	return result, nil
}

// fixtureForSchema builds the smallest fixture document that satisfies the
// enforced schema subset: required string properties become "ok", required
// numbers become 1, required booleans true, required arrays empty.
func fixtureForSchema(schema json.RawMessage, invalid bool) ([]byte, error) {
	var s struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil, fmt.Errorf("schema does not parse")
	}
	if s.Type != "object" {
		return nil, fmt.Errorf("only object schemas are supported by the fixture generator")
	}
	doc := map[string]any{}
	for _, name := range s.Required {
		prop, ok := s.Properties[name]
		if !ok {
			return nil, fmt.Errorf("required property %q has no schema", name)
		}
		switch prop.Type {
		case "string":
			doc[name] = "ok"
		case "integer", "number":
			doc[name] = 1
		case "boolean":
			doc[name] = true
		case "array":
			doc[name] = []any{}
		default:
			return nil, fmt.Errorf("required property %q has unsupported type %q", name, prop.Type)
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if invalid {
		// Corrupt exactly one required field so the executor's pre-commit
		// schema validation fails (the C3-32 evidence case).
		var flipped map[string]any
		_ = json.Unmarshal(b, &flipped)
		for _, name := range s.Required {
			if s.Properties[name].Type == "string" {
				flipped[name] = 42
				break
			}
		}
		b, _ = json.Marshal(flipped)
	}
	return b, nil
}

// mockStreamDecoder feeds one synthetic SSE frame carrying the full reply,
// then a finish frame (or, when scripted, stops without finishing).
type mockStreamDecoder struct {
	adapter *MockAdapter
	done    bool
}

func (d *mockStreamDecoder) Feed(line string) ([]StreamDelta, error) {
	if d.done {
		return nil, nil
	}
	d.done = true
	m := d.adapter
	m.mu.Lock()
	behavior := m.behavior
	m.mu.Unlock()
	if behavior.FailWith != nil {
		return nil, behavior.FailWith
	}
	payload := `{"choices":[{"delta":{"content":"mock-stream"}}]}`
	_ = payload
	var out []StreamDelta
	out = append(out, StreamDelta{Text: "streamed"})
	if behavior.NoFinishOnStream {
		return out, nil
	}
	out = append(out, StreamDelta{Finish: dprovider.FinishStop, Usage: dprovider.Usage{InputTokens: 7, OutputTokens: 5}})
	return out, nil
}
