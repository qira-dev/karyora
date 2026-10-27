package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

// The provider conformance harness (C3-20): repeatable per-profile checks
// over recorded fixture servers. Every capability claim must link to a
// passing case; a claim without a case is reported UNPROVEN and never
// treated as supported (claims are not inferred from protocol labels).
// Live checks are recorded Not Run while D-37 is open: no live provider
// target, no credentials, no real API keys anywhere.

// Fixture credential material is obviously synthetic; it exists to prove
// header mapping and redaction, never to authenticate anything.
const fixtureCredential = "conformance-fixture-credential-value"

type fixtureSecretStore struct{}

func (fixtureSecretStore) Resolve(context.Context, security.Handle) ([]byte, error) {
	return []byte(fixtureCredential), nil
}
func (fixtureSecretStore) Status(context.Context, security.Handle) (security.SecretStatus, error) {
	return security.SecretPresent, nil
}

// fixtureEndpointLabel marks the harness profile identity: conformance
// evidence attaches to THIS endpoint profile only. Real provider profiles
// carry their dated-review records but stay blocked on classifications —
// the classification allowlist is empty until the owner sets that policy
// (C3-19 fail-closed).
const (
	fixtureEndpointLabel = "conformance-fixture"
	fixtureRegion        = "fixture"
)

func fixtureProfile(a Adapter) dprovider.ProfileID {
	p := a.DefaultProfile("fixture-model")
	p.Endpoint = fixtureEndpointLabel
	p.Region = fixtureRegion
	return p
}

// seedFixtureRegistry seeds the adapter's real-profile record (dated
// review; empty classification allowlist → still blocks) plus the verified
// FIXTURE-profile record for the harness run.
func seedFixtureRegistry(a Adapter, reg *dprovider.Registry, now time.Time) {
	reg.Upsert(a.SeedDataHandling(a.DefaultProfile("fixture-model")))
	fixture := a.SeedDataHandling(fixtureProfile(a))
	fixture.Deployment = dprovider.DeploymentFixture
	fixture.TrainingUse = dprovider.FactNo
	fixture.AbuseRetention = dprovider.FactNo
	fixture.ApplicationStateTTL = dprovider.FactNo
	fixture.Subprocessors = dprovider.FactNo
	fixture.DeletionEligibility = "deletion_supported"
	fixture.PolicyURL = ""
	fixture.PolicyVersion = ""
	fixture.ReviewDate = now
	fixture.Reviewer = "conformance harness fixture record (no provider policy claim)"
	fixture.AllowedClassifications = []string{dprovider.ClassificationTestFixture}
	reg.Upsert(fixture)
}

// networkCase reports whether a case exercises real HTTP machinery: the
// fixture (mock) adapter has no network path, so transport cases skip it
// and its protocol behavior is covered by the package tests directly.
func networkCase(a Adapter) bool {
	_, isMock := a.(*MockAdapter)
	return !isMock
}

// CaseContext carries one case's wiring.
type CaseContext struct {
	Adapter  Adapter
	BaseURL  string
	Secrets  security.SecretStore
	Registry *dprovider.Registry
	Policy   EgressPolicy
}

// Case is one conformance check.
type Case struct {
	ID         string
	Capability dprovider.Feature
	// Applies selects the adapters the case runs for; non-matching adapters
	// record a SKIP with the reason.
	Applies func(a Adapter) bool
	Run     func(ctx context.Context, c CaseContext) error
}

// fixturePlan builds a baseline chat plan against the fixture server.
func fixturePlan(c CaseContext, mutate func(*dprovider.RequestPlan)) *dprovider.RequestPlan {
	p := &dprovider.RequestPlan{
		Operation: dprovider.OperationChat,
		Profile:   fixtureProfile(c.Adapter),
		Connection: dprovider.Connection{
			BaseURL:   c.BaseURL,
			AuthKind:  dprovider.AuthBearer,
			SecretRef: "provider:conformance/fixture-credential",
			// Explicit local connection records are legal for every fixture
			// call; only the local adapter consults the flag (C3-30).
			ExplicitLocal: true,
		},
		System:   "conformance system text",
		Messages: []dprovider.Message{{Role: dprovider.RoleUser, Parts: []dprovider.Part{{Type: "text", Text: "conformance input"}}}},
		Metadata: map[string]string{dprovider.ClassificationKey: dprovider.ClassificationTestFixture},
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func familyOf(a Adapter) familyKind {
	if da, ok := a.(*directAdapter); ok {
		return da.family
	}
	if _, ok := a.(*MockAdapter); ok {
		return familyOpenAI // the mock speaks the openai-family frame shape
	}
	return familyOpenAI
}

// fixtureChatBody returns a recorded success body for the family.
func fixtureChatBody(a Adapter, text string) []byte {
	switch familyOf(a) {
	case familyAnthropic:
		return []byte(`{"id":"msg_fixture","content":[{"type":"text","text":"` + text + `"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":5}}`)
	case familyGemini:
		return []byte(`{"candidates":[{"content":{"parts":[{"text":"` + text + `"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":5}}`)
	case familyMiniMax:
		return []byte(`{"id":"mm_fixture","choices":[{"message":{"content":"` + text + `"},"finish_reason":"stop"}],"usage":{"total_tokens":12},"base_resp":{"status_code":0}}`)
	default:
		return []byte(`{"id":"cmpl_fixture","choices":[{"message":{"content":"` + text + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5}}`)
	}
}

// streamFrames returns the family's recorded SSE lines for one reply.
func streamFrames(a Adapter, text string) []string {
	switch familyOf(a) {
	case familyAnthropic:
		return []string{
			"event: message_start",
			`data: {"message":{"id":"msg_fixture","usage":{"input_tokens":7}}}`,
			"event: content_block_delta",
			`data: {"delta":{"type":"text_delta","text":"` + text + `"}}`,
			"event: message_delta",
			`data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			"event: message_stop",
			`data: {"type":"message_stop"}`,
		}
	case familyGemini:
		return []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"` + text + `"}]}}]}`,
			`data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":5}}`,
		}
	default:
		return []string{
			`data: {"choices":[{"delta":{"content":"` + text + `"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5}}`,
			"data: [DONE]",
		}
	}
}

// writeFixtureBody writes a recorded response body from a process-local
// conformance fixture server (httptest). nosemgrep: the direct-write XSS
// audit targets user-facing HTTP surfaces; these handlers serve recorded
// fixture bytes only to this process's own conformance calls.
func writeFixtureBody(w http.ResponseWriter, b []byte) {
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	_, _ = w.Write(b)
}

// writeSSE responds with the frames as an SSE body.
func writeSSE(w http.ResponseWriter, frames []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for _, f := range frames {
		writeFixtureBody(w, []byte(f+"\n\n"))
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// Catalog returns the repeatable protocol case set (C3-20). Cases are
// non-destructive by construction: every target is this process's own
// fixture server.
func Catalog() []Case {
	return []Case{
		{
			ID: "auth-header-mapping", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				var sawHeader string
				var sawURL string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					sawHeader = r.Header.Get(c.Adapter.AuthHeaderName(fixtureProfile(c.Adapter)))
					sawURL = r.URL.String()
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "ok"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				if _, err := e.Execute(ctx, fixturePlan(c, nil)); err != nil {
					return err
				}
				if sawHeader != fixtureCredential && sawHeader != "Bearer "+fixtureCredential {
					return fmt.Errorf("credential header missing or wrong")
				}
				if strings.Contains(sawURL, fixtureCredential) || strings.Contains(sawURL, "@") {
					return fmt.Errorf("credential appeared in the URL")
				}
				return nil
			},
		},
		{
			ID: "chat-request-mapping", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				var body map[string]any
				var sawPath string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					sawPath = r.URL.Path
					_ = json.NewDecoder(r.Body).Decode(&body)
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "mapped"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				res, err := e.Execute(ctx, fixturePlan(c, nil))
				if err != nil {
					return err
				}
				// The model identity maps where the family puts it: the body
				// field, or the URL path for the path-templated family.
				if familyOf(c.Adapter) == familyGemini {
					if !strings.Contains(sawPath, "models/fixture-model:generateContent") {
						return fmt.Errorf("model did not map to the request path: %s", sawPath)
					}
				} else if body["model"] != "fixture-model" {
					return fmt.Errorf("model did not map to the wire body")
				}
				if res.Text != "mapped" || res.FinishReason != dprovider.FinishStop {
					return fmt.Errorf("result did not decode as recorded")
				}
				if res.RawDigest == "" || res.ResultDigest == "" {
					return fmt.Errorf("result digests missing")
				}
				return nil
			},
		},
		{
			ID: "streaming-full", Capability: dprovider.FeatureStreaming,
			Applies: func(a Adapter) bool {
				if _, isMock := a.(*MockAdapter); isMock {
					return false // the fixture adapter's streaming is covered by package tests
				}
				return a.Capabilities(fixtureProfile(a)).Streaming.Allowed()
			},
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeSSE(w, streamFrames(c.Adapter, "streamed"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				var got strings.Builder
				var finish dprovider.FinishReason
				res, err := e.ExecuteStream(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) { p.Stream = true }),
					func(d StreamDelta) error {
						got.WriteString(d.Text)
						if d.Finish != "" {
							finish = d.Finish
						}
						return nil
					})
				if err != nil {
					return err
				}
				if got.String() != "streamed" || finish != dprovider.FinishStop || res.FinishReason != dprovider.FinishStop {
					return fmt.Errorf("stream did not produce the recorded reply")
				}
				return nil
			},
		},
		{
			ID: "streaming-mid-stream-failure", Capability: dprovider.FeatureStreaming,
			Applies: func(a Adapter) bool {
				if _, isMock := a.(*MockAdapter); isMock {
					return false // the fixture adapter's stream cut is covered by package tests
				}
				return a.Capabilities(fixtureProfile(a)).Streaming.Allowed()
			},
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeSSE(w, streamFrames(c.Adapter, "partial-")[:2]) // cut after the first frame
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					panic(http.ErrAbortHandler) // sever the connection mid-stream
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				res, err := e.ExecuteStream(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) { p.Stream = true }), nil)
				if err == nil {
					return fmt.Errorf("a mid-stream cut must not produce a success")
				}
				pe, ok := err.(*dprovider.ProviderError)
				if !ok || (pe.Kind != dprovider.ErrMidStreamFailure && pe.Kind != dprovider.ErrUnreachable && pe.Kind != dprovider.ErrResponseTooLarge) {
					return fmt.Errorf("mid-stream failure mapped to %v", err)
				}
				if res != nil {
					return fmt.Errorf("no result may accompany a mid-stream failure")
				}
				return nil
			},
		},
		{
			ID: "tool-call-mapping", Capability: dprovider.FeatureTools,
			Applies: func(a Adapter) bool { return a.Capabilities(fixtureProfile(a)).Tools.Allowed() },
			Run: func(ctx context.Context, c CaseContext) error {
				var body map[string]any
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewDecoder(r.Body).Decode(&body)
					switch familyOf(c.Adapter) {
					case familyAnthropic:
						writeFixtureBody(w, []byte(`{"id":"msg_t","content":[{"type":"tool_use","id":"tu1","name":"lookup","input":{"q":"a"}},{"type":"tool_use","id":"tu2","name":"fetch","input":{"u":"b"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":4}}`))
					case familyGemini:
						writeFixtureBody(w, []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"a"}}},{"functionCall":{"name":"fetch","args":{"u":"b"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4}}`))
					default:
						writeFixtureBody(w, []byte(`{"id":"cmpl_t","choices":[{"message":{"tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"a\"}"}},{"id":"c2","type":"function","function":{"name":"fetch","arguments":"{\"u\":\"b\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`))
					}
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`)
				res, err := e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Tools = []dprovider.ToolSpec{{Name: "lookup", Description: "d", ParametersSchema: schema}}
					p.ToolChoice = dprovider.ToolChoiceAuto
				}))
				if err != nil {
					return err
				}
				if len(res.ToolCalls) != 2 {
					return fmt.Errorf("expected two parallel tool calls, got %d", len(res.ToolCalls))
				}
				if body["tools"] == nil {
					return fmt.Errorf("tools did not map to the wire body")
				}
				return nil
			},
		},
		{
			ID: "structured-output-validation", Capability: dprovider.FeatureStructured,
			Applies: func(a Adapter) bool { return a.Capabilities(fixtureProfile(a)).StructuredOutput.Allowed() },
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					switch familyOf(c.Adapter) {
					case familyGemini:
						if body["generationConfig"] == nil {
							panic("missing generationConfig")
						}
						writeFixtureBody(w, []byte(`{"candidates":[{"content":{"parts":[{"text":"{\"answer\":\"ok\",\"score\":3}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":3}}`))
					default:
						writeFixtureBody(w, []byte(`{"id":"cmpl_s","choices":[{"message":{"content":"{\"answer\":\"ok\",\"score\":3}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":3}}`))
					}
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				schema := json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"},"score":{"type":"integer"}},"required":["answer","score"],"additionalProperties":false}`)
				res, err := e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Structured = &dprovider.StructuredOutput{Name: "answer", Schema: schema, Strict: true}
				}))
				if err != nil {
					return err
				}
				if len(res.StructuredJSON) == 0 {
					return fmt.Errorf("structured output missing")
				}
				return nil
			},
		},
		{
			ID: "structured-output-schema-failure", Capability: dprovider.FeatureStructured,
			Applies: func(a Adapter) bool {
				if _, isMock := a.(*MockAdapter); isMock {
					return false // covered by the scripted fixture tests
				}
				return a.Capabilities(fixtureProfile(a)).StructuredOutput.Allowed()
			},
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch familyOf(c.Adapter) {
					case familyGemini:
						writeFixtureBody(w, []byte(`{"candidates":[{"content":{"parts":[{"text":"{\"answer\":42,\"score\":3}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":3}}`))
					default:
						writeFixtureBody(w, []byte(`{"id":"cmpl_s","choices":[{"message":{"content":"{\"answer\":42,\"score\":3}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":3}}`))
					}
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				schema := json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"},"score":{"type":"integer"}},"required":["answer","score"]}`)
				_, err = e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Structured = &dprovider.StructuredOutput{Name: "answer", Schema: schema}
				}))
				if err == nil {
					return fmt.Errorf("schema-invalid output must not produce a result")
				}
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrProtocol {
					return fmt.Errorf("schema failure mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "error-mapping-auth", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, `{"error":"denied"}`, http.StatusUnauthorized)
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrAuth || pe.Retryable() {
					return fmt.Errorf("401 mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "error-mapping-rate-limit", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "3")
					http.Error(w, `{"error":"slow down"}`, http.StatusTooManyRequests)
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrRateLimited || !pe.Retryable() || pe.RetryAfter != 3*time.Second {
					return fmt.Errorf("429 mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "error-mapping-server", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrUnreachable || !pe.Retryable() {
					return fmt.Errorf("500 mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "timeout-bounded", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(300 * time.Millisecond)
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "late"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) { p.Timeout = 50 * time.Millisecond }))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrTimeout {
					return fmt.Errorf("deadline mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "cancellation", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(500 * time.Millisecond)
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "late"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				ctx2, cancel := context.WithCancel(ctx)
				cancel()
				_, err = e.Execute(ctx2, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrCanceled {
					return fmt.Errorf("cancellation mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "response-size-cap", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					writeFixtureBody(w, make([]byte, 4096))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				c.Policy.MaxResponseBytes = 1024
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrResponseTooLarge {
					return fmt.Errorf("oversized response mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "redirect-refusal", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "/elsewhere", http.StatusFound)
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, nil))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrEgressPolicy {
					return fmt.Errorf("redirect mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "credentials-in-url-refusal", Capability: dprovider.FeatureChat, Applies: func(Adapter) bool { return true },
			Run: func(ctx context.Context, c CaseContext) error {
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Connection.BaseURL = c.BaseURL + "?key=leaked"
				}))
				if pe, ok := err.(*dprovider.ProviderError); !ok || pe.Kind != dprovider.ErrEgressPolicy {
					return fmt.Errorf("credential-in-URL mapped to %v", err)
				}
				return nil
			},
		},
		{
			ID: "data-handling-gate", Capability: dprovider.FeatureChat, Applies: func(Adapter) bool { return true },
			Run: func(ctx context.Context, c CaseContext) error {
				p := fixtureProfile(c.Adapter)
				// the fixture profile allows only the fixture classification
				if err := c.Registry.EgressAllowed(p, dprovider.FeatureChat, dprovider.ClassificationTestFixture); err != nil {
					return fmt.Errorf("fixture classification must pass: %v", err)
				}
				// protected classifications block on the fixture profile
				if err := c.Registry.EgressAllowed(p, dprovider.FeatureChat, "user-documents"); err == nil {
					return fmt.Errorf("protected classification must block on the fixture profile")
				}
				if _, isMock := c.Adapter.(*MockAdapter); isMock {
					// The fixture adapter IS this workspace's own code: its
					// record is verified for the fixture classification, and
					// every other classification still blocks.
					if err := c.Registry.EgressAllowed(p, dprovider.FeatureChat, "user-documents"); err == nil {
						return fmt.Errorf("the fixture profile must block non-fixture classifications")
					}
					return nil
				}
				// the REAL provider profile still blocks everything: its dated
				// review records facts but the classification allowlist is
				// empty (the classification policy is an owner decision)
				real := c.Adapter.DefaultProfile("fixture-model")
				if err := c.Registry.EgressAllowed(real, dprovider.FeatureChat, dprovider.ClassificationTestFixture); err == nil {
					return fmt.Errorf("real provider profile must block egress (empty classification allowlist)")
				}
				return nil
			},
		},
		{
			ID: "vision-mapping", Capability: dprovider.FeatureVision,
			Applies: func(a Adapter) bool {
				if _, isMock := a.(*MockAdapter); isMock {
					return false // no wire mapping to prove
				}
				return a.Capabilities(fixtureProfile(a)).Vision.Allowed()
			},
			Run: func(ctx context.Context, c CaseContext) error {
				var body map[string]any
				var raw []byte
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					raw, _ = io.ReadAll(r.Body)
					_ = json.Unmarshal(raw, &body)
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "seen"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				res, err := e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Messages[0].Parts = append(p.Messages[0].Parts, dprovider.Part{
						Type: "image", ImageMIME: "image/png", ImageData: []byte("fixture-image-bytes")})
				}))
				if err != nil {
					return err
				}
				if res.Text != "seen" {
					return fmt.Errorf("vision call did not decode as recorded")
				}
				// the image bytes must actually travel, family-appropriately
				b := string(raw)
				switch familyOf(c.Adapter) {
				case familyAnthropic:
					if !strings.Contains(b, "base64") || !strings.Contains(b, "image/png") {
						return fmt.Errorf("image block missing from the wire body")
					}
				case familyGemini:
					if !strings.Contains(b, "inline_data") {
						return fmt.Errorf("inline image missing from the wire body")
					}
				default:
					if !strings.Contains(b, "image_url") || !strings.Contains(b, "data:image/png") {
						return fmt.Errorf("image_url data part missing from the wire body")
					}
				}
				return nil
			},
		},
		{
			ID: "conversation-continuation", Capability: dprovider.FeatureContinuation,
			Applies: func(a Adapter) bool {
				if _, isMock := a.(*MockAdapter); isMock {
					return false // no wire mapping to prove
				}
				return a.Capabilities(fixtureProfile(a)).ConversationContinuation.Allowed()
			},
			Run: func(ctx context.Context, c CaseContext) error {
				var body map[string]any
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewDecoder(r.Body).Decode(&body)
					writeFixtureBody(w, fixtureChatBody(c.Adapter, "continued"))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				_, err = e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					// Stateless continuation: the caller supplies the history.
					// The system text is cleared so the turn count is exact
					// across families (the openai family inlines system as a
					// message; anthropic/gemini carry it separately).
					p.System = ""
					p.Messages = []dprovider.Message{
						{Role: dprovider.RoleUser, Parts: []dprovider.Part{{Type: "text", Text: "first"}}},
						{Role: dprovider.RoleAssistant, Parts: []dprovider.Part{{Type: "text", Text: "second"}}},
						{Role: dprovider.RoleUser, Parts: []dprovider.Part{{Type: "text", Text: "third"}}},
					}
				}))
				if err != nil {
					return err
				}
				key := "messages"
				if familyOf(c.Adapter) == familyGemini {
					key = "contents"
				}
				history, _ := body[key].([]any)
				if len(history) != 3 {
					return fmt.Errorf("conversation history did not map (%d turns in %q)", len(history), key)
				}
				return nil
			},
		},
		{
			ID: "embeddings", Capability: dprovider.FeatureEmbeddings,
			Applies: func(a Adapter) bool { return a.Capabilities(fixtureProfile(a)).Embeddings.Allowed() },
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch familyOf(c.Adapter) {
					case familyGemini:
						writeFixtureBody(w, []byte(`{"embedding":{"values":[0.1,0.2]}}`))
					default:
						writeFixtureBody(w, []byte(`{"data":[{"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":2,"completion_tokens":0}}`))
					}
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				res, err := e.Execute(ctx, fixturePlan(c, func(p *dprovider.RequestPlan) {
					p.Operation = dprovider.OperationEmbed
					p.EmbedInputs = []string{"text one", "text two"}
				}))
				if err != nil {
					return err
				}
				if len(res.Embeddings) == 0 {
					return fmt.Errorf("embedding result missing")
				}
				return nil
			},
		},
		{
			ID: "test-connection-bounded", Capability: dprovider.FeatureChat, Applies: networkCase,
			Run: func(ctx context.Context, c CaseContext) error {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "chatcompletion") {
						http.Error(w, `{}`, http.StatusBadRequest) // validation probe path
						return
					}
					writeFixtureBody(w, []byte(`{"data":[]}`))
				}))
				defer srv.Close()
				c.BaseURL = srv.URL
				e, err := NewExecutor(c.Adapter, c.Policy, c.Secrets, c.Registry)
				if err != nil {
					return err
				}
				return e.TestConnection(ctx, fixturePlan(c, nil))
			},
		},
	}
}

// CaseResult is one harness outcome.
type CaseResult struct {
	AdapterID  string `json:"adapter_id"`
	CaseID     string `json:"case_id"`
	Capability string `json:"capability"`
	Status     string `json:"status"` // pass | fail | skip
	Detail     string `json:"detail,omitempty"`
}

// Report is the conformance report (C3-20 evidence).
type Report struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	Cases       []CaseResult                 `json:"cases"`
	LiveChecks  string                       `json:"live_checks"`
	Matrix      map[string]map[string]string `json:"support_matrix"` // adapter → feature → supported | unproven | unsupported
}

// RunCatalog runs the full case set for every adapter and builds the
// support matrix. Fixture servers are process-local; nothing external is
// contacted.
func RunCatalog(ctx context.Context, adapters []Adapter, policy EgressPolicy, now time.Time) *Report {
	cases := Catalog()
	rep := &Report{GeneratedAt: now, LiveChecks: "Not Run: D-37 is open — no live provider target, no credentials, no real API keys; recorded/mock fixtures only"}
	for _, a := range adapters {
		reg := dprovider.NewRegistry(func() time.Time { return now })
		seedFixtureRegistry(a, reg, now)
		cc := CaseContext{Adapter: a, Secrets: fixtureSecretStore{}, Registry: reg, Policy: policy}
		for _, tc := range cases {
			r := CaseResult{AdapterID: a.ID(), CaseID: tc.ID, Capability: string(tc.Capability)}
			if tc.Applies != nil && !tc.Applies(a) {
				r.Status = "skip"
				r.Detail = "capability not claimed for this profile"
				rep.Cases = append(rep.Cases, r)
				continue
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"no handler for this request"}`, http.StatusTeapot)
			}))
			cc.BaseURL = srv.URL
			err := tc.Run(ctx, cc)
			srv.Close()
			if err != nil {
				r.Status = "fail"
				r.Detail = bounded(err.Error())
			} else {
				r.Status = "pass"
			}
			rep.Cases = append(rep.Cases, r)
		}
	}
	rep.Matrix = BuildSupportMatrix(adapters, rep)
	return rep
}

// BuildSupportMatrix crosses capability claims with case results: a claim
// is "supported" only with a passing case, "unproven" when claimed without
// one, and "unsupported" when the adapter does not claim it. Claims are
// never inferred from protocol labels (C3-20).
func BuildSupportMatrix(adapters []Adapter, rep *Report) map[string]map[string]string {
	passed := map[string]bool{}
	for _, c := range rep.Cases {
		if c.Status == "pass" {
			passed[c.AdapterID+"/"+c.CaseID] = true
		}
	}
	out := map[string]map[string]string{}
	features := []dprovider.Feature{
		dprovider.FeatureChat, dprovider.FeatureStreaming, dprovider.FeatureTools,
		dprovider.FeatureParallelTools, dprovider.FeatureStructured, dprovider.FeatureVision,
		dprovider.FeatureAudio, dprovider.FeatureEmbeddings, dprovider.FeatureBatch,
		dprovider.FeatureContextCaching, dprovider.FeatureContinuation,
	}
	// the case that proves each feature
	provingCase := map[dprovider.Feature]string{
		dprovider.FeatureChat:          "chat-request-mapping",
		dprovider.FeatureStreaming:     "streaming-full",
		dprovider.FeatureTools:         "tool-call-mapping",
		dprovider.FeatureParallelTools: "tool-call-mapping", // the case asserts TWO calls
		dprovider.FeatureStructured:    "structured-output-validation",
		dprovider.FeatureEmbeddings:    "embeddings",
		dprovider.FeatureVision:        "vision-mapping",
		dprovider.FeatureContinuation:  "conversation-continuation",
	}
	for _, a := range adapters {
		caps := a.Capabilities(fixtureProfile(a))
		row := map[string]string{}
		for _, f := range features {
			if !caps.Supported(f) {
				row[string(f)] = "unsupported"
				continue
			}
			caseID, proven := provingCase[f]
			if !proven || !passed[a.ID()+"/"+caseID] {
				row[string(f)] = "unproven"
				continue
			}
			row[string(f)] = "supported"
		}
		out[a.ID()] = row
	}
	return out
}

// SortCases orders the report deterministically (adapter, case).
func (r *Report) SortCases() {
	sort.Slice(r.Cases, func(i, j int) bool {
		if r.Cases[i].AdapterID != r.Cases[j].AdapterID {
			return r.Cases[i].AdapterID < r.Cases[j].AdapterID
		}
		return r.Cases[i].CaseID < r.Cases[j].CaseID
	})
}
