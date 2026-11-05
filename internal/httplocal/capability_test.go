package httplocal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/provider"
)

// The F2 capability discovery/status endpoint tests (IMPL-22, the adopted
// S10 supply's P-S10-02 class over the wire): the descriptor shape inside
// the capability_descriptor wrapper, the typed refusals, the GET-only
// surface, and the no-invention pin — the descriptor carries exactly the
// coded three-state axes, never an invented capability (UI-13; C3-29).

func TestProviderCapabilitiesEndpoint(t *testing.T) {
	env := newTestEnv(t)

	status, body := env.do(t, http.MethodGet, "/v1/providers/"+provider.ProviderMock+"/capabilities", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the read must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Descriptor struct {
			Provider string `json:"provider"`
			Profile  struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			} `json:"profile"`
			Capabilities provider.CapabilitySet `json:"capabilities"`
		} `json:"capability_descriptor"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if res.Descriptor.Provider != provider.ProviderMock {
		t.Fatalf("the descriptor names the wired provider, got %q (%s)", res.Descriptor.Provider, body)
	}
	if res.Descriptor.Profile.Model != "default" {
		t.Fatalf("the registered default profile resolves without a model: %+v (%s)", res.Descriptor.Profile, body)
	}
	// The coded facts verbatim — supported axes ride, every unoverridden
	// axis stays unknown (fail closed), no limit value invented.
	if res.Descriptor.Capabilities.Chat != provider.CapSupported ||
		res.Descriptor.Capabilities.Streaming != provider.CapSupported ||
		res.Descriptor.Capabilities.StructuredOutput != provider.CapSupported ||
		res.Descriptor.Capabilities.Embeddings != provider.CapSupported {
		t.Fatalf("the mock's coded supported axes must ride verbatim: %+v (%s)", res.Descriptor.Capabilities, body)
	}
	if res.Descriptor.Capabilities.Tools != provider.CapUnknown ||
		res.Descriptor.Capabilities.Vision != provider.CapUnknown ||
		res.Descriptor.Capabilities.Batch != provider.CapUnknown {
		t.Fatalf("every unoverridden axis must stay unknown: %+v", res.Descriptor.Capabilities)
	}
	if res.Descriptor.Capabilities.MaxContextTokens != 0 || res.Descriptor.Capabilities.MaxOutputTokens != 0 {
		t.Fatalf("no limit value is invented (0 = unknown): %+v", res.Descriptor.Capabilities)
	}

	// The optional model query value rides the profile resolution.
	status, body = env.do(t, http.MethodGet, "/v1/providers/"+provider.ProviderMock+"/capabilities?model=fixture-model", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the model-resolved read must answer 200, got %d: %s", status, body)
	}
	var withModel struct {
		Descriptor struct {
			Profile struct {
				Model string `json:"model"`
			} `json:"profile"`
		} `json:"capability_descriptor"`
	}
	if err := json.Unmarshal(body, &withModel); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if withModel.Descriptor.Profile.Model != "fixture-model" {
		t.Fatalf("the model query value rides the profile: %+v (%s)", withModel.Descriptor.Profile, body)
	}
}

func TestProviderCapabilitiesEndpointRefusals(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "unauthenticated", method: http.MethodGet, path: "/v1/providers/" + provider.ProviderMock + "/capabilities", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "unwired_provider", method: http.MethodGet, path: "/v1/providers/provider-does-not-exist/capabilities", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "model_bound", method: http.MethodGet, path: "/v1/providers/" + provider.ProviderMock + "/capabilities?model=" + strings.Repeat("m", 257), token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "method_mismatch", method: http.MethodPost, path: "/v1/providers/" + provider.ProviderMock + "/capabilities", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, "", tc.token, nil)
			if status != tc.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.wantStatus, status, body)
			}
			if !strings.Contains(string(body), tc.wantCode) {
				t.Fatalf("%s: the refusal must carry the typed %s code: %s", tc.name, tc.wantCode, body)
			}
		})
	}
}
