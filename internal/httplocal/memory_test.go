package httplocal

import (
	"encoding/json"
	"strings"
	"testing"
)

// The B2 Memory family's wire register (DE-0 queue row 3): the published
// /memories route family end to end over the live loopback test server —
// the M-17 add with its replay, the inspect read, the keyset list, the
// M-16 edit/pin/destructive members, the M-18 inclusion decision, the M-32
// export, the authoritative state endpoint, the BINDING secrets law's
// refusal, the typed 404s (the no-leak rule), and the typed 403s (the
// session boundary and the disallowed-Origin mutation).

func memoryAddBody(key, correlation, scope, content string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) +
		`,"scope":` + jq(scope) + `,"origin":"user-entered","content":` + jq(content) + `}`
}

func memoryEditBody(key, correlation, stateRef, content string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) +
		`,"item_state_ref":` + jq(stateRef) + `,"content":` + jq(content) + `}`
}

// memoryItemOf decodes one mutation response's memory_item member and the
// operation reference.
func memoryItemOf(t *testing.T, body []byte) (map[string]any, string) {
	t.Helper()
	var m struct {
		MemoryItem  map[string]any `json:"memory_item"`
		OperationID string         `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("mutation body: %v (%s)", err, body)
	}
	if m.MemoryItem == nil || m.OperationID == "" {
		t.Fatalf("the mutation result carries the confirmed post-state and the operation reference: %s", body)
	}
	return m.MemoryItem, m.OperationID
}

func memoryField(item map[string]any, field string) string {
	v, _ := item[field].(string)
	return v
}

// TestMemoryWireFamily drives the family end to end: add → inspect →
// M-17 replay → list (+cursor) → edit → M-16 replay → state endpoint →
// pin/unpin → inclusion (+replay +new decision) → export (+re-issue) →
// keyed delete (+replay) → the same typed 404 after removal → unkeyed
// forget (the recorded carrier gap).
func TestMemoryWireFamily(t *testing.T) {
	env := newTestEnv(t)

	// The add: 200 with the Core-confirmed recorded item and the operation
	// reference; the Core mints the identity.
	status, body := env.do(t, "POST", "/v1/memories", memoryAddBody("wire-mem-1", "corr-wire-1", "user", "the wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("add: %d %s", status, body)
	}
	item, addOpID := memoryItemOf(t, body)
	itemID := memoryField(item, "memory_item_id")
	if itemID == "" || memoryField(item, "lifecycle_state") != "active" || memoryField(item, "origin") != "user-entered" {
		t.Fatalf("the add serves the Core-minted truth: %s", body)
	}

	// The M-17 replay over the wire: same key + same request identity.
	status, body = env.do(t, "POST", "/v1/memories", memoryAddBody("wire-mem-1", "corr-wire-1", "user", "the wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("replay: %d %s", status, body)
	}
	replayItem, replayOpID := memoryItemOf(t, body)
	if replayOpID != addOpID || memoryField(replayItem, "memory_item_id") != itemID {
		t.Fatalf("the replay returns the SAME recorded outcome: %s", body)
	}

	// The inspect read: a pure query — the authoritative item state.
	status, body = env.do(t, "GET", "/v1/memories/"+itemID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("inspect: %d %s", status, body)
	}
	var inspect struct {
		MemoryItem map[string]any `json:"memory_item"`
	}
	if err := json.Unmarshal(body, &inspect); err != nil || memoryField(inspect.MemoryItem, "content") != "the wire content" {
		t.Fatalf("the inspect read: %s (%v)", body, err)
	}

	// The edit: the M-16 revision class (provenance-preserving).
	stateRef := memoryField(item, "lifecycle_state") + "@" + memoryField(item, "updated_at")
	status, body = env.do(t, "POST", "/v1/memories/"+itemID, memoryEditBody("wire-edit-1", "corr-edit-1", stateRef, "the corrected wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("edit: %d %s", status, body)
	}
	edited, editOpID := memoryItemOf(t, body)
	if memoryField(edited, "lifecycle_state") != "updated" || memoryField(edited, "content") != "the corrected wire content" {
		t.Fatalf("the edit records the corrected post-state: %s", body)
	}
	if memoryField(edited, "origin") != "user-entered" {
		t.Fatalf("the edit preserves the origin label: %s", body)
	}

	// The M-16 replay: the same key + the same correlation + the same
	// prepared-against reference.
	status, body = env.do(t, "POST", "/v1/memories/"+itemID, memoryEditBody("wire-edit-1", "corr-edit-1", stateRef, "the corrected wire content"), env.token, nil)
	if status != 200 {
		t.Fatalf("edit replay: %d %s", status, body)
	}
	if _, replayEditOp := memoryItemOf(t, body); replayEditOp != editOpID {
		t.Fatalf("the M-16 replay returns the recorded outcome: %s", body)
	}

	// The authoritative state endpoint: the pure query (never a retry).
	status, body = env.do(t, "GET", "/v1/memories/operations/"+editOpID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("state read: %d %s", status, body)
	}
	var opRes struct {
		Operation map[string]any `json:"operation"`
	}
	if err := json.Unmarshal(body, &opRes); err != nil || memoryField(opRes.Operation, "state") != "recorded" {
		t.Fatalf("the state endpoint is the authoritative truth: %s (%v)", body, err)
	}

	// The pin/unpin member: one member operation covering both directions;
	// the confirmed flag rides the response's item.
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/pin", `{"idempotency_key":"wire-pin-1","correlation_id":"corr-pin-1","pinned":true}`, env.token, nil)
	if status != 200 {
		t.Fatalf("pin: %d %s", status, body)
	}
	pinned, _ := memoryItemOf(t, body)
	if v, _ := pinned["pinned"].(bool); !v {
		t.Fatalf("the confirmed pin state rides the item: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/pin", `{"idempotency_key":"wire-pin-2","correlation_id":"corr-pin-2","pinned":false}`, env.token, nil)
	if status != 200 {
		t.Fatalf("unpin: %d %s", status, body)
	}
	unpinned, _ := memoryItemOf(t, body)
	if v, _ := unpinned["pinned"].(bool); v {
		t.Fatalf("the unpin: %s", body)
	}

	// The M-18 inclusion: a decision record, never an item-state change.
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/inclusion", `{"correlation_id":"corr-inc-1","request_ref":"req-alpha","included":true}`, env.token, nil)
	if status != 200 {
		t.Fatalf("inclusion: %d %s", status, body)
	}
	var incl struct {
		MemoryItemID string `json:"memory_item_id"`
		RequestRef   string `json:"request_ref"`
		Included     bool   `json:"included"`
		OperationID  string `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &incl); err != nil || !incl.Included || incl.OperationID == "" || incl.RequestRef != "req-alpha" {
		t.Fatalf("the inclusion decision: %s (%v)", body, err)
	}
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/inclusion", `{"correlation_id":"corr-inc-1","request_ref":"req-alpha","included":true}`, env.token, nil)
	if status != 200 {
		t.Fatalf("inclusion replay: %d %s", status, body)
	}
	var inclReplay struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &inclReplay); err != nil || inclReplay.OperationID != incl.OperationID {
		t.Fatalf("the M-18 replay: %s", body)
	}

	// The M-32 export: none-required idempotency; two submits mint two
	// operations; the document rides the response.
	status, body = env.do(t, "POST", "/v1/memories/export", `{"correlation_id":"corr-exp-1","scope":"user"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("export: %d %s", status, body)
	}
	var exp struct {
		ExportOperationID string           `json:"export_operation_id"`
		Count             int              `json:"count"`
		GeneratedAt       string           `json:"generated_at"`
		Truncated         bool             `json:"truncated"`
		Items             []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &exp); err != nil || exp.Count != 1 || len(exp.Items) != 1 || exp.GeneratedAt == "" {
		t.Fatalf("the export document: %s (%v)", body, err)
	}
	status, body = env.do(t, "POST", "/v1/memories/export", `{"correlation_id":"corr-exp-2","scope":"user"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("export re-issue: %d %s", status, body)
	}
	var exp2 struct {
		ExportOperationID string `json:"export_operation_id"`
	}
	if err := json.Unmarshal(body, &exp2); err != nil || exp2.ExportOperationID == exp.ExportOperationID {
		t.Fatalf("a re-issued export mints a new operation: %s", body)
	}

	// The keyset list (+cursor round trip): a second item gives the list a
	// second page.
	status, body = env.do(t, "POST", "/v1/memories", memoryAddBody("wire-mem-2", "corr-wire-2", "workspace", "second distinct content"), env.token, nil)
	if status != 200 {
		t.Fatalf("second add: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/memories?limit=1&scope=user", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var list struct {
		Items  []map[string]any `json:"items"`
		Cursor string           `json:"cursor"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list body: %v", err)
	}
	// The scope filter is Core-side: the stated scope's readable items
	// only (the forget tombstone from the session fixture never rides a
	// user list).
	if len(list.Items) != 1 || memoryField(list.Items[0], "scope") != "user" || memoryField(list.Items[0], "memory_item_id") != itemID {
		t.Fatalf("the list serves the stated scope's readable items: %s", body)
	}
	if list.Cursor != "" {
		t.Fatalf("the final page's cursor absence: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/memories?limit=1", "", env.token, nil)
	if status != 200 {
		t.Fatalf("unfiltered list: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("unfiltered list body: %v", err)
	}
	if len(list.Items) != 1 || list.Cursor == "" {
		t.Fatalf("the page and its cursor over the readable set: %s", body)
	}
	status, _ = env.do(t, "GET", "/v1/memories?limit=1&cursor="+list.Cursor, "", env.token, nil)
	if status != 200 {
		t.Fatalf("list page 2: %d", status)
	}
	// The keyed delete: 200 with the confirmed outcome and the retention
	// caveat; the replay reconciles the lost response; the inspect read
	// afterwards is the SAME typed 404.
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/delete", `{"idempotency_key":"wire-del-1","correlation_id":"corr-del-1"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("delete: %d %s", status, body)
	}
	var del struct {
		MemoryItemID  string `json:"memory_item_id"`
		OutcomeState  string `json:"outcome_state"`
		RetentionNote string `json:"retention_note"`
		OperationID   string `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &del); err != nil || del.OutcomeState != "deleted" || del.OperationID == "" || del.RetentionNote == "" {
		t.Fatalf("the delete outcome: %s (%v)", body, err)
	}
	status, body = env.do(t, "GET", "/v1/memories/"+itemID, "", env.token, nil)
	if status != 404 {
		t.Fatalf("the removed item is not served: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/delete", `{"idempotency_key":"wire-del-1","correlation_id":"corr-del-1"}`, env.token, nil)
	if status != 200 {
		t.Fatalf("the keyed destructive replay: %d %s", status, body)
	}
	var delReplay struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &delReplay); err != nil || delReplay.OperationID != del.OperationID {
		t.Fatalf("the replay returns the recorded outcome: %s", body)
	}

	// The unkeyed forget (the recorded carrier gap — an empty body): the
	// confirmed tombstone post-state; the state endpoint confirms.
	status, body = env.do(t, "POST", "/v1/memories", memoryAddBody("wire-fg-1", "corr-fg-1", "session", "the forgotten candidate"), env.token, nil)
	if status != 200 {
		t.Fatalf("forget fixture add: %d %s", status, body)
	}
	fgItem, _ := memoryItemOf(t, body)
	status, body = env.do(t, "POST", "/v1/memories/"+memoryField(fgItem, "memory_item_id")+"/forget", "", env.token, nil)
	if status != 200 {
		t.Fatalf("unkeyed forget: %d %s", status, body)
	}
	fgTombstone, fgOp := memoryItemOf(t, body)
	if memoryField(fgTombstone, "lifecycle_state") != "forgotten" || memoryField(fgTombstone, "content") != "" {
		t.Fatalf("the forget tombstone: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/memories/operations/"+fgOp, "", env.token, nil)
	if status != 200 {
		t.Fatalf("the forget record: %d %s", status, body)
	}

}

// TestMemoryWireSecrets pins the BINDING secrets law over the wire: the
// refusal rides MEMORY_SECRETS_REJECTED (400), the matched material is
// never echoed, and nothing is stored.
func TestMemoryWireSecrets(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, "POST", "/v1/memories",
		memoryAddBody("wire-sec-1", "corr-sec-1", "user", "my api_key = a1b2c3d4"), env.token, nil)
	if status != 400 {
		t.Fatalf("the secrets refusal: %d %s", status, body)
	}
	var envBody struct {
		Code      string         `json:"code"`
		Retryable bool           `json:"retryable"`
		Data      map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envBody); err != nil {
		t.Fatalf("refusal body: %v", err)
	}
	if envBody.Code != "MEMORY_SECRETS_REJECTED" || envBody.Retryable {
		t.Fatalf("the refusal must ride the register code: %s", body)
	}
	if strings.Contains(string(body), "a1b2c3d4") {
		t.Fatalf("the matched material is never echoed (the law extends to error messages): %s", body)
	}
	// Nothing was stored.
	status, body = env.do(t, "GET", "/v1/memories?scope=user", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list after refusal: %d %s", status, body)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil || len(list.Items) != 0 {
		t.Fatalf("the refusal precedes any accepted operation: %s (%v)", body, err)
	}
	// The PEM private-key shape refuses too.
	status, _ = env.do(t, "POST", "/v1/memories",
		memoryAddBody("wire-sec-2", "corr-sec-2", "user", "-----BEGIN RSA PRIVATE KEY-----\\nMIIEow\\n-----END RSA PRIVATE KEY-----"), env.token, nil)
	if status != 400 {
		t.Fatalf("the PEM shape refuses: %d", status)
	}
}

// TestMemoryWireTyped404s pins the no-leak rule over the wire: the unknown
// item and the removed item are the SAME typed MEMORY_ITEM_NOT_FOUND 404;
// the unknown operation rides the register's NOT_FOUND; GET
// /v1/memories/export resolves the generic item read (an item named
// "export" — the same typed 404).
func TestMemoryWireTyped404s(t *testing.T) {
	env := newTestEnv(t)
	status, body := env.do(t, "POST", "/v1/memories", memoryAddBody("wire-404", "corr-404", "user", "the doomed content"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture add: %d %s", status, body)
	}
	item, _ := memoryItemOf(t, body)
	itemID := memoryField(item, "memory_item_id")
	status, body = env.do(t, "POST", "/v1/memories/"+itemID+"/delete", "", env.token, nil)
	if status != 200 {
		t.Fatalf("fixture delete: %d %s", status, body)
	}
	paths := []string{
		"/v1/memories/mem-nope",
		"/v1/memories/export",
		"/v1/memories/" + itemID, // the removed item — the SAME typed 404
	}
	bodies := make([][]byte, 0, len(paths))
	for _, p := range paths {
		status, body := env.do(t, "GET", p, "", env.token, nil)
		if status != 404 {
			t.Fatalf("%s: must be the typed 404, got %d %s", p, status, body)
		}
		var envBody struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(body, &envBody); err != nil || envBody.Code != "MEMORY_ITEM_NOT_FOUND" {
			t.Fatalf("%s: must ride the MEMORY_ITEM_NOT_FOUND register code: %s", p, body)
		}
		bodies = append(bodies, body)
	}
	// The SAME typed 404: every unknown/removed identity resolves the
	// identical envelope code (the no-leak rule).
	for _, b := range bodies[1:] {
		var a, c map[string]any
		if err := json.Unmarshal(bodies[0], &a); err != nil {
			t.Fatalf("body: %v", err)
		}
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatalf("body: %v", err)
		}
		if a["code"] != c["code"] {
			t.Fatalf("the unknown identities share the one typed 404: %s vs %s", bodies[0], b)
		}
	}
	// The unknown operation: the register's NOT_FOUND (no Memory
	// operation-not-found domain code exists in the published register).
	status, body = env.do(t, "GET", "/v1/memories/operations/memop-nope", "", env.token, nil)
	if status != 404 {
		t.Fatalf("the unknown operation: %d %s", status, body)
	}
	var opBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &opBody); err != nil || opBody.Code != "NOT_FOUND" {
		t.Fatalf("the operation 404 rides the register code: %s", body)
	}
}

// TestMemoryWireRefusals pins the typed 403s and the route-shape behavior:
// every B2 surface refuses the session-less caller with
// AUTHZ_DENIED/SESSION_INVALID; a mutating route with a foreign
// non-loopback Origin refuses with ORIGIN_NOT_ALLOWED; method mismatches
// resolve NOT_FOUND; the state-endpoint route EXISTS (the boundary, not
// the router, answers).
func TestMemoryWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Probe 1: a read without session — the typed 403 with SESSION_INVALID.
	status, body := env.do(t, "GET", "/v1/memories", "", "", nil)
	if status != 403 {
		t.Fatalf("no-session list: %d %s", status, body)
	}
	var envBody struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envBody); err != nil {
		t.Fatalf("refusal body: %v", err)
	}
	if envBody.Code != "AUTHZ_DENIED" || envBody.Data["reason"] != "SESSION_INVALID" {
		t.Fatalf("the session boundary refusal: %s", body)
	}

	// Every B2 route refuses the session-less caller the same way.
	noSessionPaths := []struct{ method, path string }{
		{"GET", "/v1/memories"},
		{"GET", "/v1/memories/mem-x"},
		{"GET", "/v1/memories/operations/memop-x"},
		{"POST", "/v1/memories"},
		{"POST", "/v1/memories/mem-x"},
		{"POST", "/v1/memories/mem-x/delete"},
		{"POST", "/v1/memories/mem-x/forget"},
		{"POST", "/v1/memories/mem-x/approve"},
		{"POST", "/v1/memories/mem-x/reject"},
		{"POST", "/v1/memories/mem-x/pin"},
		{"POST", "/v1/memories/mem-x/inclusion"},
		{"POST", "/v1/memories/export"},
	}
	for _, p := range noSessionPaths {
		status, body := env.do(t, p.method, p.path, "{}", "", nil)
		if status != 403 {
			t.Fatalf("%s %s: the session-less caller must see the typed 403, got %d %s", p.method, p.path, status, body)
		}
	}

	// Probe 2: a mutation with a foreign non-loopback Origin — the typed
	// 403 with ORIGIN_NOT_ALLOWED.
	status, body = env.do(t, "POST", "/v1/memories", memoryAddBody("origin-mem", "corr-origin", "user", "x"), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin mutation: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &envBody); err != nil {
		t.Fatalf("origin refusal body: %v", err)
	}
	if envBody.Code != "AUTHZ_DENIED" || envBody.Data["reason"] != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("the Origin boundary refusal: %s", body)
	}

	// The state-endpoint route exists: the boundary answers (the
	// contract-only acts' 404 aborts are gone).
	status, _ = env.do(t, "GET", "/v1/memories/operations/test-op-id", "", "", nil)
	if status != 403 {
		t.Fatalf("the state route must exist live (the boundary answers): %d", status)
	}

	// Method mismatches on known paths resolve the register's NOT_FOUND.
	status, _ = env.do(t, "DELETE", "/v1/memories", "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch: %d", status)
	}
}
