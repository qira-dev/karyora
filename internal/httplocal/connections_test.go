package httplocal

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

// newWireFakeSecrets is the in-memory D-37 store double behind the wire
// tests' slice (no real keyring in tests).
func newWireFakeSecrets() *wireFakeSecrets { return &wireFakeSecrets{items: map[string][]byte{}} }

type wireFakeSecrets struct {
	mu    sync.Mutex
	items map[string][]byte
}

func (f *wireFakeSecrets) Store(_ context.Context, h security.Handle, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(value) == 0 {
		return &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	f.items[h.String()] = cp
	return nil
}

func (f *wireFakeSecrets) Revoke(_ context.Context, h security.Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, h.String())
	return nil
}

func (f *wireFakeSecrets) Resolve(_ context.Context, h security.Handle) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.items[h.String()]
	if !ok {
		return nil, &security.SecretError{Code: errs.CodeNotFound, Reason: security.ReasonSecretMissing, Handle: h.String()}
	}
	return v, nil
}

func (f *wireFakeSecrets) Status(_ context.Context, h security.Handle) (security.SecretStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[h.String()]; ok {
		return security.SecretPresent, nil
	}
	return security.SecretMissing, nil
}

// The obviously-fake wire-test credential (never a real key; Project
// Rule 14).
const wireTestCanary = "karyora-test-canary-77aa-not-a-real-credential"

// connectionSaveBody builds one save body; the credential-bearing posture
// (deepseek — AuthBearer) carries the masked-status and exactly-once pins,
// the mock fixture (AuthNone) needs no credential.
func connectionSaveBody(key, provider, model string, withSecret bool) string {
	secret := ""
	if withSecret {
		secret = `,"secret":"` + wireTestCanary + `"`
	}
	return `{"idempotency_key":"` + key + `","provider":"` + provider + `","model_id":"` + model + `"` + secret + `}`
}

// TestConnectionEndpoints walks the connection wire surface end to end:
// the save (with its Supplied idempotency), the list, the get, the saved
// and draft tests, the rotate/re-authenticate, the switch, and the
// delete — every response in the coded shape, every secret status masked,
// and the authoritative read confirming each transition independently.
func TestConnectionEndpoints(t *testing.T) {
	env := newTestEnv(t)
	h := map[string]string{"Origin": "http://127.0.0.1:1"}

	// Save: 200 with the coded record shape and replayed=false.
	status, body := env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-1", "deepseek", "m", true), env.token, h)
	if status != 200 {
		t.Fatalf("save: %d %s", status, body)
	}
	var saved struct {
		Connection struct {
			Record       dprovider.ConnectionRecord `json:"record"`
			SecretStatus string                     `json:"secret_status"`
			TestCurrent  bool                       `json:"test_current"`
			Active       bool                       `json:"active"`
		} `json:"connection"`
		Replayed bool `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		t.Fatalf("save shape: %v (%s)", err, body)
	}
	if saved.Replayed || saved.Connection.Record.ConnectionID == "" || saved.Connection.Record.SecretRef == "" {
		t.Fatalf("save result: %+v", saved)
	}
	if saved.Connection.SecretStatus != string(security.SecretPresent) {
		t.Fatalf("the masked status reports the stored credential: %+v", saved.Connection)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the save response never echoes the credential")
	}
	connID := saved.Connection.Record.ConnectionID

	// The Supplied idempotency over the wire: duplicate → the existing
	// reference, replayed=true; same key + different body → 409
	// CONFLICT_IDEMPOTENCY_KEY.
	status, body = env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-1", "deepseek", "m", true), env.token, h)
	if status != 200 {
		t.Fatalf("duplicate save: %d %s", status, body)
	}
	var replayed struct {
		Connection struct {
			Record dprovider.ConnectionRecord `json:"record"`
		} `json:"connection"`
		Replayed bool `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &replayed); err != nil {
		t.Fatalf("replay shape: %v", err)
	}
	if !replayed.Replayed || replayed.Connection.Record.ConnectionID != connID {
		t.Fatalf("duplicate save returns the existing reference: %+v", replayed)
	}
	status, body = env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-1", "deepseek", "other-model", true), env.token, h)
	if status != 409 {
		t.Fatalf("same key + different body must be 409: %d %s", status, body)
	}
	if !strings.Contains(string(body), string(errs.CodeConflictIdempotencyKey)) {
		t.Fatalf("the conflict is the coded CONFLICT_IDEMPOTENCY_KEY: %s", body)
	}

	// List: the coded page shape.
	status, body = env.do(t, "GET", "/v1/connections", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var list struct {
		Connections []struct {
			Record       dprovider.ConnectionRecord `json:"record"`
			SecretStatus string                     `json:"secret_status"`
			Active       bool                       `json:"active"`
		} `json:"connections"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("list shape: %v", err)
	}
	if len(list.Connections) != 1 || list.Connections[0].Record.ConnectionID != connID {
		t.Fatalf("list result: %s", body)
	}

	// Get: the typed view.
	status, body = env.do(t, "GET", "/v1/connections/"+connID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("get: %d %s", status, body)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the get response never echoes the credential")
	}

	// Test (saved, the fixture provider): the sanitized report + the TC-1
	// stamp. The fixture connection rides the mock (its probe succeeds
	// without a network; the deepseek connection's probe is the typed
	// fail-closed egress refusal, pinned below).
	status, body = env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-mock", "mock", "m", false), env.token, h)
	if status != 200 {
		t.Fatalf("fixture save: %d %s", status, body)
	}
	var fixtureSaved struct {
		Connection struct {
			Record dprovider.ConnectionRecord `json:"record"`
		} `json:"connection"`
	}
	if err := json.Unmarshal([]byte(body), &fixtureSaved); err != nil {
		t.Fatalf("fixture save shape: %v", err)
	}
	fixtureID := fixtureSaved.Connection.Record.ConnectionID
	status, body = env.do(t, "POST", "/v1/connections/test", `{"connection_id":"`+fixtureID+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("saved test: %d %s", status, body)
	}
	var tested struct {
		Report struct {
			OK            bool           `json:"ok"`
			Provider      string         `json:"provider"`
			ModelID       string         `json:"model_id"`
			CorrelationID string         `json:"correlation_id"`
			Capabilities  map[string]any `json:"capabilities"`
		} `json:"report"`
		Record *dprovider.ConnectionRecord `json:"record"`
	}
	if err := json.Unmarshal([]byte(body), &tested); err != nil {
		t.Fatalf("test shape: %v", err)
	}
	if !tested.Report.OK || tested.Report.CorrelationID == "" || tested.Report.Capabilities == nil {
		t.Fatalf("the fixture probe succeeds with its report: %s", body)
	}
	if tested.Record == nil || tested.Record.TestedAt == "" {
		t.Fatalf("the saved test stamps the tested state: %s", body)
	}
	// The authoritative read confirms the stamp independently (the stamp
	// rode the fixture connection).
	status, body = env.do(t, "GET", "/v1/connections/"+fixtureID, "", env.token, nil)
	if !strings.Contains(string(body), `"test_current":true`) {
		t.Fatalf("the get view reports the tested truth: %s", body)
	}

	// Test (draft): the transient secret rides once; nothing is recorded.
	status, body = env.do(t, "POST", "/v1/connections/test", `{"provider":"mock","model_id":"m","secret":"`+wireTestCanary+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("draft test: %d %s", status, body)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the draft-test response never echoes the credential")
	}
	if strings.Contains(string(body), `"record"`) {
		t.Fatal("a draft test records nothing")
	}

	// Test (saved, the gated provider): the fail-closed egress refusal is
	// the sanitized typed report — no network, no secret echo, no tested
	// stamp.
	status, body = env.do(t, "POST", "/v1/connections/test", `{"connection_id":"`+connID+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("gated test: %d %s", status, body)
	}
	var gated struct {
		Report struct {
			OK        bool   `json:"ok"`
			ErrorKind string `json:"error_kind"`
		} `json:"report"`
		Record *dprovider.ConnectionRecord `json:"record"`
	}
	if err := json.Unmarshal([]byte(body), &gated); err != nil {
		t.Fatalf("gated test shape: %v", err)
	}
	if gated.Report.OK || gated.Report.ErrorKind != "egress_policy" || gated.Record != nil {
		t.Fatalf("the gated probe refuses typed and records nothing: %s", body)
	}

	// Rotate: the working candidate's data rides the response.
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/rotate", `{"secret":"`+wireTestCanary+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("rotate: %d %s", status, body)
	}
	var rotated struct {
		Record           dprovider.ConnectionRecord `json:"record"`
		AffectedModels   []string                   `json:"affected_models"`
		RoutingNote      string                     `json:"routing_note"`
		InterruptionNote string                     `json:"interruption_note"`
	}
	if err := json.Unmarshal([]byte(body), &rotated); err != nil {
		t.Fatalf("rotate shape: %v", err)
	}
	if len(rotated.AffectedModels) != 1 || rotated.RoutingNote == "" || rotated.InterruptionNote == "" {
		t.Fatalf("the rotation response data: %s", body)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("the rotate response never echoes the credential")
	}

	// Re-authenticate: the same path, its own action.
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/reauthenticate", `{"secret":"`+wireTestCanary+`"}`, env.token, h)
	if status != 200 {
		t.Fatalf("re-auth: %d %s", status, body)
	}

	// Switch: the minimal M-34 working candidate, idempotent.
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/switch", "", env.token, h)
	if status != 200 {
		t.Fatalf("switch: %d %s", status, body)
	}
	var sw struct {
		ActiveConnectionID string `json:"active_connection_id"`
		Replayed           bool   `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &sw); err != nil {
		t.Fatalf("switch shape: %v", err)
	}
	if sw.ActiveConnectionID != connID || sw.Replayed {
		t.Fatalf("switch result: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/switch", "", env.token, h)
	if !strings.Contains(string(body), `"replayed":true`) {
		t.Fatalf("a repeat switch is the idempotent no-op: %s", body)
	}

	// Delete: the record is gone and the read is the typed NOT_FOUND.
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/delete", "", env.token, h)
	if status != 200 {
		t.Fatalf("delete: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/connections/"+connID, "", env.token, nil)
	if status != 404 || !strings.Contains(string(body), string(errs.CodeNotFound)) {
		t.Fatalf("the deleted connection reads as the typed NOT_FOUND: %d %s", status, body)
	}
}

// TestConnectionWireRefusals pins the wire failure paths: the
// unauthenticated operations are the boundary's 403; a malformed body is
// 400; an unknown connection read is 404; the browser-shaped POST without
// a session is the boundary's fail-closed refusal — and no refusal ever
// echoes a credential.
func TestConnectionWireRefusals(t *testing.T) {
	env := newTestEnv(t)
	h := map[string]string{"Origin": "http://127.0.0.1:1"}

	// The unauthenticated save: 403 through the boundary (no session, the
	// CSRF predicate refuses the browser-shaped POST).
	status, body := env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-x", "deepseek", "m", true), "", h)
	if status != 403 {
		t.Fatalf("the unauthenticated save is 403: %d %s", status, body)
	}
	if strings.Contains(string(body), wireTestCanary) {
		t.Fatal("no refusal echoes a credential")
	}

	// Save with a session, then the unauthenticated reads/writes beside it.
	status, body = env.do(t, "POST", "/v1/connections", connectionSaveBody("idem-y", "deepseek", "m", true), env.token, h)
	if status != 200 {
		t.Fatalf("save: %d %s", status, body)
	}
	var saved struct {
		Connection struct {
			Record dprovider.ConnectionRecord `json:"record"`
		} `json:"connection"`
	}
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		t.Fatalf("save shape: %v", err)
	}
	connID := saved.Connection.Record.ConnectionID

	status, body = env.do(t, "GET", "/v1/connections", "", "", nil)
	if status != 403 {
		t.Fatalf("the unauthenticated list is 403: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/connections/"+connID, "", "", nil)
	if status != 403 {
		t.Fatalf("the unauthenticated get is 403: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/rotate", `{"secret":"x"}`, "", h)
	if status != 403 {
		t.Fatalf("the unauthenticated rotate is 403: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/connections/"+connID+"/delete", "", "", h)
	if status != 403 {
		t.Fatalf("the unauthenticated delete is 403: %d %s", status, body)
	}

	// A malformed body is 400 VALIDATION_FAILED.
	status, body = env.do(t, "POST", "/v1/connections", `{not-json`, env.token, h)
	if status != 400 || !strings.Contains(string(body), string(errs.CodeValidationFailed)) {
		t.Fatalf("a malformed body is 400: %d %s", status, body)
	}

	// An unknown connection read is the typed NOT_FOUND; an unknown
	// rotate is the same shape.
	status, body = env.do(t, "GET", "/v1/connections/conn-unknown", "", env.token, nil)
	if status != 404 || !strings.Contains(string(body), string(errs.CodeNotFound)) {
		t.Fatalf("the unknown read is 404: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/connections/conn-unknown/rotate", `{"secret":"x"}`, env.token, h)
	if status != 404 || !strings.Contains(string(body), string(errs.CodeNotFound)) {
		t.Fatalf("the unknown rotate is 404: %d %s", status, body)
	}
}
