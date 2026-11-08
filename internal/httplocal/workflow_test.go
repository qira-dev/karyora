package httplocal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	workflowdomain "github.com/qira-dev/karyora/internal/domain/workflow"
)

// doWithHeaders is testEnv.do with the response headers returned (the
// If-Match/ETag absence check inspects the served header set).
func (e *testEnv) doWithHeaders(t *testing.T, method, path, body, token string, headers map[string]string) (int, []byte, http.Header) {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes(), resp.Header
}

// The B4 Workflow authoring family's wire register (DE-0 queue row 6): the
// published /workflows route family end to end over the live loopback test
// server — the aggregate-create class with its keyed replay, the
// Core-authoritative typed validation, the M-21 save with the NEW revision
// reference STATED, the interim conflict rule's concurrent-revision
// outcome, the AI-diff propose member's honest unmet-producer refusal, the
// run-history LINKAGE read's honest empty page, the authoritative state
// endpoint, the typed 404s and 403s, and THE SCRIPTED ABSENCE CHECKS (no
// run-execution operation is reachable; no If-Match/ETag member exists on
// any served response and no precondition is ever honored).

func workflowCreateBody(key, correlation, name string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"display_name":` + jq(name) + `}`
}

func workflowValidateBody(correlation, definitionID string, draft string) string {
	if definitionID != "" {
		return `{"correlation_id":` + jq(correlation) + `,"definition_id":` + jq(definitionID) + `,"draft":` + draft + `}`
	}
	return `{"correlation_id":` + jq(correlation) + `,"draft":` + draft + `}`
}

func workflowSaveBody(key, correlation, base, draft string) string {
	if base != "" {
		return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"base_revision_ref":` + jq(base) + `,"draft":` + draft + `}`
	}
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"base_revision_ref":"","draft":` + draft + `}`
}

func workflowAIBody(correlation, definitionID, instruction string) string {
	if definitionID != "" {
		return `{"correlation_id":` + jq(correlation) + `,"definition_id":` + jq(definitionID) + `,"draft":{"nodes":[],"edges":[]},"instruction":` + jq(instruction) + `}`
	}
	return `{"correlation_id":` + jq(correlation) + `,"draft":{"nodes":[],"edges":[]},"instruction":` + jq(instruction) + `}`
}

// workflowValidDraftJSON is a structurally valid draft over the Core's own
// catalogue types.
var workflowValidDraftJSON = `{"nodes":[` +
	`{"node_ref":"n1","node_type":"trigger","config":{}},` +
	`{"node_ref":"n2","node_type":"result","config":{"k":"v"}}],` +
	`"edges":[{"edge_ref":"e1","from_node":"n1","to_node":"n2"}]}`

// workflowUnknownNodeDraftJSON declares a node type the Core's own
// catalogue does not contract.
var workflowUnknownNodeDraftJSON = `{"nodes":[{"node_ref":"n1","node_type":"totally-unknown","config":{}}],"edges":[]}`

// workflowEnvelopeCode decodes one envelope body's code.
func workflowEnvelopeCode(t *testing.T, body []byte) (string, map[string]any) {
	t.Helper()
	var envBody struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envBody); err != nil {
		t.Fatalf("envelope body: %v (%s)", err, body)
	}
	return envBody.Code, envBody.Data
}

// workflowField decodes one JSON object field as a string.
func workflowField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	v, _ := m[field].(string)
	return v
}

// TestWorkflowWireFamily drives the family end to end: create (the
// Core-minted identity) → replay → validate (the typed result) → save (the
// NEW revision STATED) → inspect (each revision immutable and reachable) →
// state → run-history (the honest empty page) → the interim conflict
// rule's concurrent-revision outcome → the save's authoritative re-check →
// the AI propose refusal → the SAME typed 404s.
func TestWorkflowWireFamily(t *testing.T) {
	env := newTestEnv(t)

	// The aggregate-create member: the Core mints the definition identity —
	// the body never carries one.
	status, body := env.do(t, "POST", "/v1/workflows", workflowCreateBody("key-w1", "corr-w1", "the wire workflow"), env.token, nil)
	if status != 200 {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Definition struct {
			DefinitionID       string `json:"definition_id"`
			DisplayName        string `json:"display_name"`
			CurrentRevisionRef string `json:"current_revision_ref"`
		} `json:"definition"`
		Replayed bool `json:"replayed"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("create body: %v (%s)", err, body)
	}
	if created.Replayed || created.Definition.DefinitionID == "" || created.Definition.DisplayName != "the wire workflow" {
		t.Fatalf("the Core-confirmed create: %s", body)
	}
	if created.Definition.CurrentRevisionRef != "" {
		t.Fatalf("a definition with no saved revision carries none: %s", body)
	}
	defID := created.Definition.DefinitionID

	// The keyed replay: the recorded identity, never a second one.
	status, body = env.do(t, "POST", "/v1/workflows", workflowCreateBody("key-w1", "corr-w1", "different body"), env.token, nil)
	if status != 200 {
		t.Fatalf("replay: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("replay body: %v (%s)", err, body)
	}
	if !created.Replayed || created.Definition.DefinitionID != defID {
		t.Fatalf("the replay returns the recorded identity: %s", body)
	}

	// The Core-authoritative validation: the typed result labels exactly
	// which draft it evaluated (the exact-object labeling law).
	status, body = env.do(t, "POST", "/v1/workflows/validate", workflowValidateBody("corr-v1", defID, workflowValidDraftJSON), env.token, nil)
	if status != 200 {
		t.Fatalf("validate: %d %s", status, body)
	}
	var validated workflowdomain.ValidationResult
	if err := json.Unmarshal(body, &validated); err != nil {
		t.Fatalf("validate body: %v (%s)", err, body)
	}
	if validated.EvaluatedObjectRef != "draft:"+defID || len(validated.BlockingErrors) != 0 {
		t.Fatalf("the typed result: %s", body)
	}

	// The unknown node type: the blocking issue rides the family register
	// code and links to the offending node (the Core's own catalogue — no
	// node catalogue is published and none is invented).
	status, body = env.do(t, "POST", "/v1/workflows/validate", workflowValidateBody("corr-v2", defID, workflowUnknownNodeDraftJSON), env.token, nil)
	if status != 200 {
		t.Fatalf("validate unknown node: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &validated); err != nil {
		t.Fatalf("validate unknown body: %v (%s)", err, body)
	}
	if len(validated.BlockingErrors) != 1 ||
		validated.BlockingErrors[0].IssueClass != "blocking_error" ||
		validated.BlockingErrors[0].Code != "WORKFLOW_UNKNOWN_NODE_TYPE" ||
		validated.BlockingErrors[0].NodeRef != "n1" {
		t.Fatalf("the unknown-node issue: %s", body)
	}

	// Save 1: the NEW revision reference STATED on save (the empty base is
	// the client's known base for a definition with no revisions).
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", workflowSaveBody("skey-1", "corr-s1", "", workflowValidDraftJSON), env.token, nil)
	if status != 200 {
		t.Fatalf("save 1: %d %s", status, body)
	}
	save1Ref := workflowField(t, body, "revision_ref")
	if save1Ref == "" || workflowField(t, body, "replayed") == "true" {
		t.Fatalf("the save states the new revision reference: %s", body)
	}

	// The inspect member: the definition and its revision records — each
	// immutable and reachable.
	status, body = env.do(t, "GET", "/v1/workflows/"+defID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("inspect: %d %s", status, body)
	}
	var inspected struct {
		Definition struct {
			CurrentRevisionRef string `json:"current_revision_ref"`
		} `json:"definition"`
		Revisions []workflowdomain.Revision `json:"revisions"`
	}
	if err := json.Unmarshal(body, &inspected); err != nil {
		t.Fatalf("inspect body: %v (%s)", err, body)
	}
	if len(inspected.Revisions) != 1 || inspected.Revisions[0].RevisionRef != save1Ref ||
		inspected.Definition.CurrentRevisionRef != save1Ref {
		t.Fatalf("the inspect member: %s", body)
	}

	// Save 2 against the recorded base: a second NEW immutable revision.
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", workflowSaveBody("skey-2", "corr-s2", save1Ref, workflowValidDraftJSON), env.token, nil)
	if status != 200 {
		t.Fatalf("save 2: %d %s", status, body)
	}
	save2Ref := workflowField(t, body, "revision_ref")
	if save2Ref == "" || save2Ref == save1Ref {
		t.Fatalf("a save creates a NEW revision, never an update-in-place: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/workflows/"+defID, "", env.token, nil)
	if err := json.Unmarshal(body, &inspected); err != nil {
		t.Fatalf("inspect 2 body: %v (%s)", err, body)
	}
	if len(inspected.Revisions) != 2 {
		t.Fatalf("the revision history: %s", body)
	}

	// The authoritative state endpoint (a pure query — never a retry): the
	// current state and the revision-specific state.
	status, body = env.do(t, "GET", "/v1/workflows/"+defID+"/state", "", env.token, nil)
	if status != 200 {
		t.Fatalf("state: %d %s", status, body)
	}
	var stateResult struct {
		DefinitionState struct {
			DefinitionID       string `json:"definition_id"`
			CurrentRevisionRef string `json:"current_revision_ref"`
		} `json:"definition_state"`
	}
	if err := json.Unmarshal(body, &stateResult); err != nil {
		t.Fatalf("state body: %v (%s)", err, body)
	}
	if stateResult.DefinitionState.DefinitionID != defID || stateResult.DefinitionState.CurrentRevisionRef != save2Ref {
		t.Fatalf("the authoritative state: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/workflows/"+defID+"/state?revision_id="+save1Ref, "", env.token, nil)
	if status != 200 {
		t.Fatalf("revision state: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &stateResult); err != nil {
		t.Fatalf("revision state body: %v (%s)", err, body)
	}
	if stateResult.DefinitionState.CurrentRevisionRef != save1Ref {
		t.Fatalf("the revision-specific state: %s", body)
	}

	// THE D-40 GATE: the run-history LINKAGE read's honest empty page — no
	// Run producer exists while the gate stands (the LINKAGE record, not a
	// Run schema).
	status, body = env.do(t, "GET", "/v1/workflows/"+defID+"/run-history", "", env.token, nil)
	if status != 200 {
		t.Fatalf("run history: %d %s", status, body)
	}
	var history struct {
		Entries []workflowdomain.RunHistoryEntry `json:"entries"`
	}
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("run history body: %v (%s)", err, body)
	}
	if len(history.Entries) != 0 {
		t.Fatalf("no Run producer exists while the gate stands: %s", body)
	}

	// The concurrent-revision outcome: the stale base — the typed
	// WORKFLOW_REVISION_CONFLICT carrying the stored side's reference (the
	// interim conflict rule: no overwrite, no merge, both sides preserved).
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", workflowSaveBody("skey-3", "corr-s3", "wfrev-stale", workflowValidDraftJSON), env.token, nil)
	if status != 409 {
		t.Fatalf("the stale-base conflict: %d %s", status, body)
	}
	code, data := workflowEnvelopeCode(t, body)
	if code != "WORKFLOW_REVISION_CONFLICT" || data["current_revision_ref"] != save2Ref {
		t.Fatalf("the interim conflict rule's typed outcome: %s", body)
	}

	// The save's authoritative re-check: a draft carrying blocking findings
	// is refused (WORKFLOW_VALIDATION_FAILED) — nothing stored, the draft
	// surviving byte-for-byte client-side.
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", workflowSaveBody("skey-4", "corr-s4", save2Ref, workflowUnknownNodeDraftJSON), env.token, nil)
	if status != 400 {
		t.Fatalf("the blocking-draft refusal: %d %s", status, body)
	}
	code, _ = workflowEnvelopeCode(t, body)
	if code != "WORKFLOW_VALIDATION_FAILED" {
		t.Fatalf("the typed rejection: %s", body)
	}

	// THE AI-PROPOSAL LAW: the honest unmet-producer refusal — no proposal
	// is ever fabricated, nothing is stored (a draft-local generation with
	// no stored artifact; the approval carries no Core write at all).
	status, body = env.do(t, "POST", "/v1/workflows/ai-proposals", workflowAIBody("corr-ai", defID, "add a retry"), env.token, nil)
	if status != 503 {
		t.Fatalf("the AI propose refusal: %d %s", status, body)
	}
	code, data = workflowEnvelopeCode(t, body)
	if code != "STORAGE_UNAVAILABLE" || data["precondition"] != "AI-PROPOSAL-PRODUCER" {
		t.Fatalf("the unmet-producer refusal names its condition: %s", body)
	}
	// The same request re-issued: the SAME stable refusal.
	status, body = env.do(t, "POST", "/v1/workflows/ai-proposals", workflowAIBody("corr-ai", defID, "add a retry"), env.token, nil)
	if status != 503 {
		t.Fatalf("the stable refusal: %d %s", status, body)
	}

	// The keyset list read (the cursor's absence marks the final page).
	status, body = env.do(t, "GET", "/v1/workflows?limit=50", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var listResult struct {
		Definitions []workflowdomain.Definition `json:"definitions"`
		NextCursor  string                      `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &listResult); err != nil {
		t.Fatalf("list body: %v (%s)", err, body)
	}
	if len(listResult.Definitions) != 1 || listResult.NextCursor != "" {
		t.Fatalf("the list page: %s", body)
	}

	// The SAME typed 404 (the no-leak rule) on every {definition_id} route:
	// the unknown identity and the out-of-scope identity are the SAME
	// typed refusal — no existence leaks from any of them.
	for _, p := range []string{
		"/v1/workflows/wfdef-nope",
		"/v1/workflows/wfdef-nope/run-history",
		"/v1/workflows/wfdef-nope/state",
	} {
		status, body = env.do(t, "GET", p, "", env.token, nil)
		if status != 404 {
			t.Fatalf("%s: the unknown identity: %d %s", p, status, body)
		}
		code, _ = workflowEnvelopeCode(t, body)
		if code != "NOT_FOUND" {
			t.Fatalf("%s: the SAME typed 404: %s", p, body)
		}
	}
	status, body = env.do(t, "POST", "/v1/workflows/wfdef-nope/revisions", workflowSaveBody("skey-5", "corr-s5", "", workflowValidDraftJSON), env.token, nil)
	if status != 404 {
		t.Fatalf("save into unknown: %d %s", status, body)
	}
	code, _ = workflowEnvelopeCode(t, body)
	if code != "NOT_FOUND" {
		t.Fatalf("the save's unknown destination: %s", body)
	}

	// Method mismatches on known paths resolve the register's NOT_FOUND.
	status, _ = env.do(t, "DELETE", "/v1/workflows", "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch: %d", status)
	}
	status, _ = env.do(t, "DELETE", "/v1/workflows/"+defID, "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch on the inspect route: %d", status)
	}
}

// TestWorkflowWireRefusals pins the typed 403s and the route-shape
// behavior: every B4 surface refuses the session-less caller with
// AUTHZ_DENIED/SESSION_INVALID; a mutating route with a foreign
// non-loopback Origin refuses with ORIGIN_NOT_ALLOWED.
func TestWorkflowWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Probe 1: a read without session — the typed 403 with SESSION_INVALID.
	status, body := env.do(t, "GET", "/v1/workflows", "", "", nil)
	if status != 403 {
		t.Fatalf("no-session list: %d %s", status, body)
	}
	code, data := workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "SESSION_INVALID" {
		t.Fatalf("the session boundary refusal: %s", body)
	}

	// Every B4 route refuses the session-less caller with the typed 403.
	noSessionPaths := []struct{ method, path, body string }{
		{"GET", "/v1/workflows", ""},
		{"POST", "/v1/workflows", workflowCreateBody("k", "c", "d")},
		{"POST", "/v1/workflows/validate", workflowValidateBody("c", "", workflowValidDraftJSON)},
		{"POST", "/v1/workflows/ai-proposals", workflowAIBody("c", "", "i")},
		{"GET", "/v1/workflows/wfdef-x", ""},
		{"POST", "/v1/workflows/wfdef-x/revisions", workflowSaveBody("k", "c", "", workflowValidDraftJSON)},
		{"GET", "/v1/workflows/wfdef-x/run-history", ""},
		{"GET", "/v1/workflows/wfdef-x/state", ""},
	}
	for _, p := range noSessionPaths {
		status, body := env.do(t, p.method, p.path, p.body, "", nil)
		if status != 403 {
			t.Fatalf("%s %s: the session-less caller must see the typed 403, got %d %s", p.method, p.path, status, body)
		}
		code, _ = workflowEnvelopeCode(t, body)
		if code != "AUTHZ_DENIED" {
			t.Fatalf("%s %s: the boundary refusal rides the register code: %s", p.method, p.path, body)
		}
	}

	// Probe 2: a mutation with a foreign non-loopback Origin — the typed
	// 403 with ORIGIN_NOT_ALLOWED.
	status, body = env.do(t, "POST", "/v1/workflows", workflowCreateBody("k", "c", "d"), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin create: %d %s", status, body)
	}
	code, data = workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("the Origin boundary refusal: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/workflows/wfdef-x/revisions", workflowSaveBody("k", "c", "", workflowValidDraftJSON), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin save: %d %s", status, body)
	}
	code, _ = workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" {
		t.Fatalf("the save's Origin refusal: %s", body)
	}
}

// TestWorkflowRunExecutionAbsence is THE SCRIPTED ABSENCE CHECK (the D-40
// gate, carried exactly): NO run-execution operation is reachable — no Run
// submission, no Run start/resume/cancel/pause, no Run schema, no
// Run/step lifecycle surface. Every plausible run-execution path resolves
// the router's NOT_FOUND (where a GET path resolves the generic inspect
// read's unknown-identity 404, the boundary still answers — never a Run
// operation). THE IF-MATCH ABSENCE rides beside it: no served response
// carries an If-Match/ETag member, and a save carrying an If-Match header
// is decided by base_revision_ref ALONE (no precondition is ever honored —
// the client asserts no protocol; the interim conflict rule governs).
func TestWorkflowRunExecutionAbsence(t *testing.T) {
	env := newTestEnv(t)

	// A real definition fixture (so the probes meet a live definition, not
	// just the unknown-identity 404).
	status, body := env.do(t, "POST", "/v1/workflows", workflowCreateBody("key-abs", "corr-abs", "the absence fixture"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture create: %d %s", status, body)
	}
	defID := ""
	var created struct {
		Definition struct {
			DefinitionID string `json:"definition_id"`
		} `json:"definition"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("fixture body: %v (%s)", err, body)
	}
	defID = created.Definition.DefinitionID

	// The run-execution path matrix: every plausible Run submission,
	// start/resume/cancel/pause/stop, execution, invocation, and step
	// surface — under BOTH methods — is unreachable.
	runPaths := []string{
		"/v1/workflows/" + defID + "/run",
		"/v1/workflows/" + defID + "/run/start",
		"/v1/workflows/" + defID + "/run/cancel",
		"/v1/workflows/" + defID + "/run/resume",
		"/v1/workflows/" + defID + "/run/pause",
		"/v1/workflows/" + defID + "/run/stop",
		"/v1/workflows/" + defID + "/runs",
		"/v1/workflows/" + defID + "/execute",
		"/v1/workflows/" + defID + "/start",
		"/v1/workflows/" + defID + "/cancel",
		"/v1/workflows/" + defID + "/invocations",
		"/v1/workflows/" + defID + "/steps",
		"/v1/runs",
		"/v1/runs/run-1",
		"/v1/workflow-runs",
		"/v1/workflows/" + defID + "/run-history/submit",
	}
	for _, p := range runPaths {
		for _, method := range []string{"POST", "GET"} {
			status, body := env.do(t, method, p, `{}`, env.token, nil)
			if status != 404 {
				t.Fatalf("%s %s: NO run-execution operation may be reachable (the D-40 gate), got %d %s", method, p, status, body)
			}
			code, _ := workflowEnvelopeCode(t, body)
			if code != "NOT_FOUND" {
				t.Fatalf("%s %s: the absence resolves the register's NOT_FOUND: %s", method, p, body)
			}
		}
	}

	// THE IF-MATCH ABSENCE (scripted): no served response of the family
	// carries an If-Match or ETag header.
	respPaths := []struct{ method, path, body string }{
		{"GET", "/v1/workflows", ""},
		{"GET", "/v1/workflows/" + defID, ""},
		{"GET", "/v1/workflows/" + defID + "/state", ""},
		{"GET", "/v1/workflows/" + defID + "/run-history", ""},
		{"POST", "/v1/workflows/validate", workflowValidateBody("corr-im", defID, workflowValidDraftJSON)},
	}
	for _, p := range respPaths {
		status, body, headers := env.doWithHeaders(t, p.method, p.path, p.body, env.token, nil)
		if status != 200 {
			t.Fatalf("%s %s: %d %s", p.method, p.path, status, body)
		}
		for _, h := range []string{"If-Match", "ETag", "If-None-Match"} {
			if _, ok := headers[h]; ok {
				t.Fatalf("%s %s: no If-Match/ETag member exists anywhere (the clause is prose-only): %s", p.method, p.path, h)
			}
		}
	}

	// A save carrying an If-Match header is decided by base_revision_ref
	// ALONE — no precondition is honored (the client asserts no protocol):
	// the same stale-base save with and without the header meets the SAME
	// typed conflict.
	withHeader := workflowSaveBody("skey-im-1", "corr-im-1", "wfrev-stale", workflowValidDraftJSON)
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", withHeader, env.token,
		map[string]string{"If-Match": `"stale-etag-value"`})
	if status != 409 {
		t.Fatalf("the header-carrying save: %d %s", status, body)
	}
	code, _ := workflowEnvelopeCode(t, body)
	if code != "WORKFLOW_REVISION_CONFLICT" {
		t.Fatalf("no precondition is ever honored (base_revision_ref decides alone): %s", body)
	}
	status, body = env.do(t, "POST", "/v1/workflows/"+defID+"/revisions", withHeader, env.token, nil)
	if status != 409 {
		t.Fatalf("the header-less save: %d %s", status, body)
	}
	code, _ = workflowEnvelopeCode(t, body)
	if code != "WORKFLOW_REVISION_CONFLICT" {
		t.Fatalf("the identical outcome without the header: %s", body)
	}
}
