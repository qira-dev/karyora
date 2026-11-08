package httplocal

import (
	"encoding/json"
	"testing"

	contextdomain "github.com/qira-dev/karyora/internal/domain/context"
)

// The B3 ContextSnapshot family's wire register (DE-0 queue row 5): the
// published /context route family end to end over the live loopback test
// server — the M-18 inclusion decision with its replay and supersede, the
// two CONDITIONAL commands' typed unmet-condition refusals (never a fake
// success path), the authoritative state endpoint's SAME typed 404, the
// typed 403s (the session boundary and the disallowed-Origin mutation),
// and the Core-provided-only reason law (no reason member is ever set).

func contextInclusionBody(correlation, requestRef string, decisions ...string) string {
	body := `{"correlation_id":` + jq(correlation) + `,"request_ref":` + jq(requestRef) + `,"decisions":[`
	for i, itemID := range decisions {
		if i > 0 {
			body += ","
		}
		body += `{"memory_item_id":` + jq(itemID) + `,"included":true}`
	}
	body += `]}`
	return body
}

func contextSaveBody(key, correlation, jobID string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"source_job_id":` + jq(jobID) + `}`
}

func contextCompareBody(baseline, candidate string) string {
	return `{"baseline_snapshot_id":` + jq(baseline) + `,"candidate_snapshot_id":` + jq(candidate) + `}`
}

// contextEnvelopeCode decodes one envelope body's code.
func contextEnvelopeCode(t *testing.T, body []byte) (string, map[string]any) {
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

// TestContextWireFamily drives the family end to end: the M-18 inclusion
// (decision → replay → supersede → the M-18 boundary) over a real memory
// fixture item; the save/compare typed unmet-condition refusals; the state
// endpoint's SAME typed 404; the no-leak rule.
func TestContextWireFamily(t *testing.T) {
	env := newTestEnv(t)

	// The fixture: the decision's item exists through the wired Memory
	// surface (the same store; the existence check resolves through the
	// authoritative read).
	status, body := env.do(t, "POST", "/v1/memories", memoryAddBody("ctx-wire-mem-1", "corr-ctx-wire-mem-1", "user", "the context fixture content"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture add: %d %s", status, body)
	}
	item, _ := memoryItemOf(t, body)
	itemID := memoryField(item, "memory_item_id")

	// The M-18 inclusion: 200 with the Core-confirmed decision record;
	// NO reason member is ever set (the Core-provided-only reason law —
	// no reason producer exists, absence is the absence).
	status, body = env.do(t, "POST", "/v1/context/inclusion", contextInclusionBody("corr-ctx-1", "req-ctx-1", itemID), env.token, nil)
	if status != 200 {
		t.Fatalf("inclusion: %d %s", status, body)
	}
	var inclResult struct {
		RequestRef string `json:"request_ref"`
		Decisions  []struct {
			MemoryItemID string  `json:"memory_item_id"`
			Included     bool    `json:"included"`
			Reason       *string `json:"reason"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal(body, &inclResult); err != nil {
		t.Fatalf("inclusion body: %v (%s)", err, body)
	}
	if inclResult.RequestRef != "req-ctx-1" || len(inclResult.Decisions) != 1 ||
		inclResult.Decisions[0].MemoryItemID != itemID || !inclResult.Decisions[0].Included {
		t.Fatalf("the decision record: %s", body)
	}
	if inclResult.Decisions[0].Reason != nil {
		t.Fatalf("no reason is ever synthesized (the Core-provided-only law): %s", body)
	}

	// The same request identity replays the recorded decision.
	status, body = env.do(t, "POST", "/v1/context/inclusion", contextInclusionBody("corr-ctx-1", "req-ctx-1", itemID), env.token, nil)
	if status != 200 {
		t.Fatalf("inclusion replay: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &inclResult); err != nil {
		t.Fatalf("replay body: %v (%s)", err, body)
	}
	if len(inclResult.Decisions) != 1 || !inclResult.Decisions[0].Included {
		t.Fatalf("the replay carries the recorded decision: %s", body)
	}

	// A different correlation under the same request: the caller's NEW
	// decision record (supersedes within the request). The body here
	// carries included=false through a distinct correlation.
	supersedeBody := `{"correlation_id":"corr-ctx-2","request_ref":"req-ctx-1","decisions":[{"memory_item_id":` + jq(itemID) + `,"included":false}]}`
	status, body = env.do(t, "POST", "/v1/context/inclusion", supersedeBody, env.token, nil)
	if status != 200 {
		t.Fatalf("inclusion supersede: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &inclResult); err != nil {
		t.Fatalf("supersede body: %v (%s)", err, body)
	}
	if len(inclResult.Decisions) != 1 || inclResult.Decisions[0].Included {
		t.Fatalf("the new decision record carries the caller's decision: %s", body)
	}

	// The M-18 boundary: the item's state is untouched (the decision is
	// never an item-state change and never a Memory action row). The
	// inspect read carries the item — decode it directly (the inspect
	// read is a pure query, not a mutation result).
	status, body = env.do(t, "GET", "/v1/memories/"+itemID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("item read: %d %s", status, body)
	}
	var inspected struct {
		MemoryItem struct {
			LifecycleState string `json:"lifecycle_state"`
		} `json:"memory_item"`
	}
	if err := json.Unmarshal(body, &inspected); err != nil {
		t.Fatalf("item body: %v (%s)", err, body)
	}
	if inspected.MemoryItem.LifecycleState != "active" {
		t.Fatalf("the decision changes no item state: %s", body)
	}

	// An unknown decided item is the SAME typed 404 the Memory family's
	// inspect read serves (the no-leak rule).
	status, body = env.do(t, "POST", "/v1/context/inclusion", contextInclusionBody("corr-ctx-3", "req-ctx-3", "mem-nope"), env.token, nil)
	if status != 404 {
		t.Fatalf("unknown item: %d %s", status, body)
	}
	code, _ := contextEnvelopeCode(t, body)
	if code != "MEMORY_ITEM_NOT_FOUND" {
		t.Fatalf("the no-leak rule's typed 404: %s", body)
	}

	// THE CONDITIONAL COMMANDS — their typed unmet-condition refusals are
	// the served behavior (never a fake success path).
	status, body = env.do(t, "POST", "/v1/context/snapshots", contextSaveBody("key-ctx-1", "corr-ctx-save-1", "job-ctx-1"), env.token, nil)
	if status != 503 {
		t.Fatalf("the save's unmet-condition refusal: %d %s", status, body)
	}
	code, data := contextEnvelopeCode(t, body)
	if code != "CONTEXT_COMPILATION_UNAVAILABLE" || data["precondition"] != "GC-3" {
		t.Fatalf("the save's typed refusal names its un-gate precondition: %s", body)
	}
	// The same key re-issued: the SAME refusal — no operation row was
	// consumed (no operation row can exist while the gate stands).
	status, body = env.do(t, "POST", "/v1/context/snapshots", contextSaveBody("key-ctx-1", "corr-ctx-save-1", "job-ctx-1"), env.token, nil)
	if status != 503 {
		t.Fatalf("the save's stable refusal: %d %s", status, body)
	}

	// The compare: the typed CONTEXT_COMPARE_UNSUPPORTED (the
	// comparison-unavailable state; no diff is ever fabricated).
	status, body = env.do(t, "POST", "/v1/context/snapshots/compare", contextCompareBody("snap-b", "snap-c"), env.token, nil)
	if status != 503 {
		t.Fatalf("the compare's unmet-condition refusal: %d %s", status, body)
	}
	code, data = contextEnvelopeCode(t, body)
	if code != "CONTEXT_COMPARE_UNSUPPORTED" || data["precondition"] != "GC-4" {
		t.Fatalf("the compare's typed refusal names its un-gate precondition: %s", body)
	}

	// The authoritative state endpoint: the unknown identity is the SAME
	// typed 404 (the no-leak rule) — no operation row can exist while the
	// durable-storage gate stands.
	for _, opID := range []string{"ctxop-nope", "ctxop-also-nope"} {
		status, body = env.do(t, "GET", "/v1/context/operations/"+opID, "", env.token, nil)
		if status != 404 {
			t.Fatalf("the state endpoint: %d %s", status, body)
		}
		code, _ = contextEnvelopeCode(t, body)
		if code != "CONTEXT_SNAPSHOT_NOT_FOUND" {
			t.Fatalf("the SAME typed 404 (the no-leak rule): %s", body)
		}
	}

	// The register values ride the closed vocabulary verbatim (the
	// domain package's registers are the wire values — the kind/state
	// registers carry exactly the published values).
	if contextdomain.KindSaveSnapshot != "save_snapshot" {
		t.Fatalf("the kind register carries the published value verbatim")
	}
	for _, state := range []string{"accepted", "saved", "failed", "outcome_unknown"} {
		if !contextdomain.ValidOperationState(state) {
			t.Fatalf("the state register carries the published values verbatim: %s", state)
		}
	}
}

// TestContextWireRefusals pins the typed 403s and the route-shape
// behavior: every B3 surface refuses the session-less caller with
// AUTHZ_DENIED/SESSION_INVALID; a mutating route with a foreign
// non-loopback Origin refuses with ORIGIN_NOT_ALLOWED; method mismatches
// resolve NOT_FOUND; the state-endpoint route EXISTS (the boundary, not
// the router, answers).
func TestContextWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Probe 1: a read without session — the typed 403 with SESSION_INVALID.
	status, body := env.do(t, "GET", "/v1/context/operations/test-op-id", "", "", nil)
	if status != 403 {
		t.Fatalf("no-session state read: %d %s", status, body)
	}
	code, data := contextEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "SESSION_INVALID" {
		t.Fatalf("the session boundary refusal: %s", body)
	}

	// Every B3 route refuses the session-less caller with the typed 403
	// (the mutating routes' CSRF gate answers the boundary first — the
	// same refusal class the B2 family's register records).
	noSessionPaths := []struct{ method, path, body string }{
		{"GET", "/v1/context/operations/ctxop-x", ""},
		{"POST", "/v1/context/snapshots", contextSaveBody("k", "c", "j")},
		{"POST", "/v1/context/snapshots/compare", contextCompareBody("b", "c")},
		{"POST", "/v1/context/inclusion", contextInclusionBody("c", "r", "mem-x")},
	}
	for _, p := range noSessionPaths {
		status, body := env.do(t, p.method, p.path, p.body, "", nil)
		if status != 403 {
			t.Fatalf("%s %s: the session-less caller must see the typed 403, got %d %s", p.method, p.path, status, body)
		}
		code, _ = contextEnvelopeCode(t, body)
		if code != "AUTHZ_DENIED" {
			t.Fatalf("%s %s: the boundary refusal rides the register code: %s", p.method, p.path, body)
		}
	}

	// Probe 2: a mutation with a foreign non-loopback Origin — the typed
	// 403 with ORIGIN_NOT_ALLOWED.
	status, body = env.do(t, "POST", "/v1/context/inclusion", contextInclusionBody("corr-origin", "req-origin", "mem-x"), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin mutation: %d %s", status, body)
	}
	code, data = contextEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("the Origin boundary refusal: %s", body)
	}

	// Method mismatches on known paths resolve the register's NOT_FOUND.
	status, _ = env.do(t, "DELETE", "/v1/context/inclusion", "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch: %d", status)
	}
}
