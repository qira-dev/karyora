package httplocal

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// The B1 KnowledgePack family's wire register (DE-0 queue row 1): the
// published /knowledgepacks route family end to end over the live loopback
// test server — the registry read, the M-9 digest-keyed import with its
// three outcomes, the authoritative state endpoint, the pack/version
// reads (including the two-ID deep link), the export class, the cancel
// rulings, the typed 404s, and the typed 403s (the session boundary and
// the disallowed-Origin mutation).

func knowledgeImportBody(key, kind, content string, destination string) string {
	dest := ""
	if destination != "" {
		dest = `,"destination_pack_id":` + jq(destination)
	}
	return `{"idempotency_key":` + jq(key) + `,"source_name":"wire doc","source_kind":` + jq(kind) +
		`,"content_b64":"` + base64.StdEncoding.EncodeToString([]byte(content)) + `"` + dest + `}`
}

func knowledgeZipB64(t *testing.T, entries ...[2]string) string {
	t.Helper()
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, e := range entries {
		f, err := w.Create(e[0])
		if err != nil {
			t.Fatalf("zip entry: %v", err)
		}
		if _, err := f.Write([]byte(e[1])); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// knowledgeOperation decodes one submit/state/cancel body's operation
// member.
func knowledgeOperation(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m struct {
		Operation map[string]any `json:"operation"`
		Replayed  bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("operation body: %v (%s)", err, body)
	}
	if m.Operation == nil {
		t.Fatalf("body lacks the operation member: %s", body)
	}
	return m.Operation
}

func operationString(op map[string]any, field string) string {
	v, _ := op[field].(string)
	return v
}

// TestKnowledgeWireFamily drives the family end to end: registry → import
// (completed) → M-9 replay → same-key-different-bytes conflict → state
// read → pack/version reads (the two-ID deep link included) → version
// list → pack list → export → cancel rulings.
func TestKnowledgeWireFamily(t *testing.T) {
	env := newTestEnv(t)

	// The registry read: the Core's own contract data.
	status, body := env.do(t, "GET", "/v1/knowledgepacks/import/registry", "", env.token, nil)
	if status != 200 {
		t.Fatalf("registry: %d %s", status, body)
	}
	var registry struct {
		Sources []struct {
			SourceKind string `json:"source_kind"`
			MaxBytes   int64  `json:"max_bytes"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(body, &registry); err != nil {
		t.Fatalf("registry body: %v", err)
	}
	if len(registry.Sources) == 0 {
		t.Fatalf("the registry must carry the served kinds: %s", body)
	}

	// The import: 202 accepted; the operation reaches completed inside the
	// pipeline; the response carries the pack-version reference.
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import", knowledgeImportBody("wire-1", "file_text_document", "the wire content", ""), env.token, nil)
	if status != 202 {
		t.Fatalf("import: %d %s", status, body)
	}
	var submit struct {
		Operation map[string]any `json:"operation"`
		Replayed  bool           `json:"replayed"`
	}
	if err := json.Unmarshal(body, &submit); err != nil {
		t.Fatalf("import body: %v", err)
	}
	if submit.Replayed {
		t.Fatalf("a fresh submit is never a replay: %s", body)
	}
	op := submit.Operation
	if operationString(op, "state") != "completed" || operationString(op, "kind") != "import" {
		t.Fatalf("the pipeline must complete: %s", body)
	}
	result, ok := op["result"].(map[string]any)
	if !ok || operationString(result, "pack_id") == "" || operationString(result, "version_id") == "" {
		t.Fatalf("the completed operation carries the pack-version reference: %s", body)
	}
	packID := operationString(result, "pack_id")
	versionID := operationString(result, "version_id")

	// The M-9 live/window replay over the wire: same key + same bytes.
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import", knowledgeImportBody("wire-1", "file_text_document", "the wire content", ""), env.token, nil)
	if status != 202 {
		t.Fatalf("replay: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &submit); err != nil {
		t.Fatalf("replay body: %v", err)
	}
	if !submit.Replayed || operationString(submit.Operation, "operation_id") != operationString(op, "operation_id") {
		t.Fatalf("the replay must return the SAME operation, replayed=true: %s", body)
	}

	// Same key + different bytes: 409 CONFLICT_IDEMPOTENCY_KEY with the
	// fingerprintMatch:false datum.
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import", knowledgeImportBody("wire-1", "file_text_document", "DIFFERENT bytes", ""), env.token, nil)
	if status != 409 {
		t.Fatalf("fingerprint conflict: %d %s", status, body)
	}
	var conflict struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &conflict); err != nil {
		t.Fatalf("conflict body: %v", err)
	}
	if conflict.Code != "CONFLICT_IDEMPOTENCY_KEY" || conflict.Data["fingerprintMatch"] != false {
		t.Fatalf("the M-9 conflict must carry fingerprintMatch:false: %s", body)
	}

	// The authoritative state endpoint.
	opID := operationString(op, "operation_id")
	status, body = env.do(t, "GET", "/v1/knowledgepacks/operations/"+opID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("state read: %d %s", status, body)
	}
	if state := operationString(knowledgeOperation(t, body), "state"); state != "completed" {
		t.Fatalf("the state endpoint is the authoritative confirmation: %s", body)
	}

	// The pack read.
	status, body = env.do(t, "GET", "/v1/knowledgepacks/"+packID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("pack read: %d %s", status, body)
	}
	var packRes struct {
		Pack struct {
			PackID           string `json:"pack_id"`
			Title            string `json:"title"`
			CurrentVersionID string `json:"current_version_id"`
		} `json:"pack"`
	}
	if err := json.Unmarshal(body, &packRes); err != nil {
		t.Fatalf("pack body: %v", err)
	}
	if packRes.Pack.PackID != packID || packRes.Pack.CurrentVersionID != versionID || packRes.Pack.Title != "wire doc" {
		t.Fatalf("the pack read serves the Core-owned truth: %s", body)
	}

	// The two-ID immutable-version deep link.
	status, body = env.do(t, "GET", "/v1/knowledgepacks/"+packID+"/versions/"+versionID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("version read: %d %s", status, body)
	}
	var versionRes struct {
		Version struct {
			PackID         string `json:"pack_id"`
			VersionID      string `json:"version_id"`
			PackRootDigest string `json:"pack_root_digest"`
			EntryCount     int    `json:"entry_count"`
		} `json:"version"`
	}
	if err := json.Unmarshal(body, &versionRes); err != nil {
		t.Fatalf("version body: %v", err)
	}
	if versionRes.Version.PackID != packID || versionRes.Version.VersionID != versionID ||
		!strings.HasPrefix(versionRes.Version.PackRootDigest, "sha256:") || versionRes.Version.EntryCount != 1 {
		t.Fatalf("the version read serves the immutable members: %s", body)
	}

	// The versions list and the packs list (keyset pages). A second pack
	// gives the pack list a second page.
	status, body = env.do(t, "GET", "/v1/knowledgepacks/"+packID+"/versions", "", env.token, nil)
	if status != 200 {
		t.Fatalf("version list: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import", knowledgeImportBody("wire-2", "file_text_document", "second distinct content", ""), env.token, nil)
	if status != 202 {
		t.Fatalf("second import: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/knowledgepacks?limit=1", "", env.token, nil)
	if status != 200 {
		t.Fatalf("pack list: %d %s", status, body)
	}
	var listRes struct {
		Packs []struct {
			PackID string `json:"pack_id"`
		} `json:"packs"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &listRes); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(listRes.Packs) != 1 || listRes.Cursor == "" {
		t.Fatalf("the page and its cursor: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/knowledgepacks?limit=1&cursor="+listRes.Cursor, "", env.token, nil)
	if status != 200 {
		t.Fatalf("pack list page 2: %d %s", status, body)
	}

	// The export class: none-required idempotency; completed with the
	// artifact reference.
	exportBody := `{"correlation_id":"wire-export-1","pack_id":"` + packID + `","destination":"local export"}`
	status, body = env.do(t, "POST", "/v1/knowledgepacks/export", exportBody, env.token, nil)
	if status != 202 {
		t.Fatalf("export: %d %s", status, body)
	}
	expOp := knowledgeOperation(t, body)
	if operationString(expOp, "state") != "completed" || operationString(expOp, "kind") != "export" {
		t.Fatalf("the export must complete: %s", body)
	}
	expResult, ok := expOp["result"].(map[string]any)
	if !ok || !strings.HasPrefix(operationString(expResult, "export_ref"), "sha256:") {
		t.Fatalf("the export carries its artifact reference: %s", body)
	}

	// The cancel rulings: terminal → 409 CONFLICT_STATE.
	status, body = env.do(t, "POST", "/v1/knowledgepacks/operations/"+opID+"/cancel", "", env.token, nil)
	if status != 409 {
		t.Fatalf("cancel of a terminal operation: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/knowledgepacks/operations/kop-nope/cancel", "", env.token, nil)
	if status != 404 {
		t.Fatalf("cancel of an unknown operation: %d %s", status, body)
	}
}

// TestKnowledgeWireArchiveImport drives the archive kind end to end and
// the typed rejection of a traversal entry (the untrusted-import Core-side
// defense — never a client-side check).
func TestKnowledgeWireArchiveImport(t *testing.T) {
	env := newTestEnv(t)
	// A clean archive imports.
	status, body := env.do(t, "POST", "/v1/knowledgepacks/import",
		`{"idempotency_key":"wire-zip-1","source_name":"zip doc","source_kind":"file_zip_archive","content_b64":"`+
			knowledgeZipB64(t, [2]string{"doc.txt", "archived"}, [2]string{"notes/readme.txt", "nested"})+`"}`,
		env.token, nil)
	if status != 202 {
		t.Fatalf("archive import: %d %s", status, body)
	}
	op := knowledgeOperation(t, body)
	if operationString(op, "state") != "completed" {
		t.Fatalf("the archive import must complete: %s", body)
	}
	result := op["result"].(map[string]any)
	packID := operationString(result, "pack_id")
	status, body = env.do(t, "GET", "/v1/knowledgepacks/"+packID+"/versions/"+operationString(result, "version_id"), "", env.token, nil)
	if status != 200 {
		t.Fatalf("archive version read: %d %s", status, body)
	}
	var versionRes struct {
		Version struct {
			EntryCount int `json:"entry_count"`
		} `json:"version"`
	}
	if err := json.Unmarshal(body, &versionRes); err != nil || versionRes.Version.EntryCount != 2 {
		t.Fatalf("the archive's two entries persist: %s (%v)", body, err)
	}
	// The traversal archive: 202 accepted; the state read confirms the
	// typed policy rejection.
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import",
		`{"idempotency_key":"wire-zip-2","source_name":"evil zip","source_kind":"file_zip_archive","content_b64":"`+
			knowledgeZipB64(t, [2]string{"doc.txt", "ok"}, [2]string{"../evil.txt", "escape"})+`"}`,
		env.token, nil)
	if status != 202 {
		t.Fatalf("traversal submit: %d %s", status, body)
	}
	trOp := knowledgeOperation(t, body)
	if operationString(trOp, "state") != "rejected" || operationString(trOp, "failure_code") != "IMPORT_POLICY_BLOCKED" {
		t.Fatalf("the traversal archive rejects with its register code: %s", body)
	}
}

// TestKnowledgeWireTyped404s pins the no-leak rule over the wire: unknown
// pack, unknown version, unknown operation — the SAME typed 404 envelope;
// GET /v1/knowledgepacks/import resolves the generic {id} read (a pack
// named "import" — the same 404).
func TestKnowledgeWireTyped404s(t *testing.T) {
	env := newTestEnv(t)
	paths := []string{
		"/v1/knowledgepacks/kpack-nope",
		"/v1/knowledgepacks/kpack-nope/versions/kver-nope",
		"/v1/knowledgepacks/kpack-real-nope/versions",
		"/v1/knowledgepacks/operations/kop-nope",
		"/v1/knowledgepacks/import",
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
		if err := json.Unmarshal(body, &envBody); err != nil || envBody.Code != "NOT_FOUND" {
			t.Fatalf("%s: must ride the NOT_FOUND register code: %s", p, body)
		}
		bodies = append(bodies, body)
	}
	// The SAME typed 404: every unknown identity resolves the identical
	// envelope code and shape (the no-leak rule).
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
}

// TestKnowledgeWireRefusals pins the typed 403s and the route-shape
// behavior: every B1 surface refuses the session-less caller with
// AUTHZ_DENIED/SESSION_INVALID; a mutating route with a foreign
// non-loopback Origin refuses with ORIGIN_NOT_ALLOWED; method mismatches
// resolve NOT_FOUND.
func TestKnowledgeWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Probe 1: a read without session — the typed 403 with SESSION_INVALID.
	status, body := env.do(t, "GET", "/v1/knowledgepacks", "", "", nil)
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

	// Every B1 route refuses the session-less caller the same way.
	noSessionPaths := []struct{ method, path string }{
		{"GET", "/v1/knowledgepacks/import/registry"},
		{"GET", "/v1/knowledgepacks/kpack-x"},
		{"GET", "/v1/knowledgepacks/kpack-x/versions"},
		{"GET", "/v1/knowledgepacks/kpack-x/versions/kver-x"},
		{"GET", "/v1/knowledgepacks/operations/kop-x"},
		{"POST", "/v1/knowledgepacks/import"},
		{"POST", "/v1/knowledgepacks/export"},
		{"POST", "/v1/knowledgepacks/operations/kop-x/cancel"},
	}
	for _, p := range noSessionPaths {
		status, body := env.do(t, p.method, p.path, "{}", "", nil)
		if status != 403 {
			t.Fatalf("%s %s: the session-less caller must see the typed 403, got %d %s", p.method, p.path, status, body)
		}
	}

	// Probe 2: a mutation with a foreign non-loopback Origin — the typed
	// 403 with ORIGIN_NOT_ALLOWED (the session-less presentation rides the
	// boundary predicate; a validated session credential takes the
	// recorded same-origin carve-out).
	status, body = env.do(t, "POST", "/v1/knowledgepacks/import",
		knowledgeImportBody("origin-1", "file_text_document", "x", ""), "",
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

	// Method mismatches on known paths resolve the register's NOT_FOUND.
	status, _ = env.do(t, "DELETE", "/v1/knowledgepacks/import", "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch: %d", status)
	}
}
