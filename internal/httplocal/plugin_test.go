package httplocal

import (
	"encoding/json"
	"testing"
)

// The B5 Plugin lifecycle family's wire register (DE-0 queue row 8): the
// published /plugins route family end to end over the live loopback test
// server — the C1-07 install class with its keyed replay (the SAME
// install-record reference, never a second one), the M-29 state-toggle
// member's both directions, the M-30 update, the G7 revoke-grants, the
// M-31 remove, the keyset list, the inspect and state reads, the typed 404s
// and 403s, and THE SCRIPTED ABSENCE CHECKS (no invocation/execution
// operation is reachable — the GATED rows [D-13/D-14 + the Core MB §10.2
// runtime proof], carried exactly).

func pluginInstallBody(key, correlation, packageRef string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"package_ref":` + jq(packageRef) + `}`
}

func pluginEnableBody(enabled bool) string {
	return `{"enabled":` + boolJSON(enabled) + `}`
}

func pluginUpdateBody(key, correlation, target string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `,"target_version_ref":` + jq(target) + `}`
}

func pluginMutationBody(key, correlation string) string {
	return `{"idempotency_key":` + jq(key) + `,"correlation_id":` + jq(correlation) + `}`
}

func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// pluginField decodes one JSON object field as a string.
func pluginField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	v, _ := m[field].(string)
	return v
}

// pluginBool decodes one JSON object field as a boolean.
func pluginBool(t *testing.T, body []byte, field string) bool {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body: %v (%s)", err, body)
	}
	v, _ := m[field].(bool)
	return v
}

// TestPluginWireFamily drives the family end to end: install (the
// Core-minted identities; the installed NOT-yet-enabled state) → replay →
// enable (both directions) → state → inspect → update → revoke grants →
// the toggle's revoked-state refusal → remove → the SAME typed 404s → the
// list page.
func TestPluginWireFamily(t *testing.T) {
	env := newTestEnv(t)

	// The install member: the Core mints the plugin, package,
	// install-record, and grant identities — the body never carries one.
	status, body := env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-w1", "corr-w1", "pkgref-w1"), env.token, nil)
	if status != 200 {
		t.Fatalf("install: %d %s", status, body)
	}
	installRef := pluginField(t, body, "install_record_ref")
	if installRef == "" || pluginBool(t, body, "replayed") {
		t.Fatalf("the Core-confirmed install: %s", body)
	}

	// The keyed replay: the SAME install-record reference, never a second
	// one (the C1-07 class).
	status, body = env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-w1", "corr-w1", "pkgref-w1"), env.token, nil)
	if status != 200 {
		t.Fatalf("replay: %d %s", status, body)
	}
	if pluginField(t, body, "install_record_ref") != installRef || !pluginBool(t, body, "replayed") {
		t.Fatalf("the replay returns the recorded install-record reference: %s", body)
	}

	// The keyset list: exactly one plugin, in its installed NOT-yet-enabled
	// state (the cursor's absence marks the final page).
	status, body = env.do(t, "GET", "/v1/plugins?limit=50", "", env.token, nil)
	if status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	var listResult struct {
		Plugins []struct {
			PluginID       string `json:"plugin_id"`
			LifecycleState string `json:"lifecycle_state"`
		} `json:"plugins"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &listResult); err != nil {
		t.Fatalf("list body: %v (%s)", err, body)
	}
	if len(listResult.Plugins) != 1 || listResult.Plugins[0].LifecycleState != "installed" || listResult.NextCursor != "" {
		t.Fatalf("the installed NOT-yet-enabled state on the list: %s", body)
	}
	pluginID := listResult.Plugins[0].PluginID
	if pluginID == "" {
		t.Fatalf("the Core-minted plugin identity: %s", body)
	}

	// The inspect member: a pure read rendering ONLY Core-returned
	// verification states.
	status, body = env.do(t, "GET", "/v1/plugins/"+pluginID, "", env.token, nil)
	if status != 200 {
		t.Fatalf("inspect: %d %s", status, body)
	}
	var inspected struct {
		PluginID         string `json:"plugin_id"`
		LifecycleState   string `json:"lifecycle_state"`
		InstallRecordRef string `json:"install_record_ref"`
		Package          *struct {
			PackageRef                  string `json:"package_ref"`
			SignatureVerificationResult string `json:"signature_verification_result"`
		} `json:"package"`
	}
	if err := json.Unmarshal(body, &inspected); err != nil {
		t.Fatalf("inspect body: %v (%s)", err, body)
	}
	if inspected.PluginID != pluginID || inspected.InstallRecordRef != installRef {
		t.Fatalf("the inspect member: %s", body)
	}
	if inspected.Package == nil || inspected.Package.PackageRef == "" || inspected.Package.SignatureVerificationResult != "" {
		t.Fatalf("no verification state is ever fabricated: %s", body)
	}

	// The M-29 state-toggle member: enable (installed → enabled).
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/enable", pluginEnableBody(true), env.token, nil)
	if status != 200 {
		t.Fatalf("enable: %d %s", status, body)
	}
	if pluginField(t, body, "plugin_id") != pluginID || pluginField(t, body, "operation_ref") == "" {
		t.Fatalf("the enable outcome: %s", body)
	}

	// The authoritative state endpoint: the enabled state (a pure query —
	// never a retry).
	status, body = env.do(t, "GET", "/v1/plugins/"+pluginID+"/state", "", env.token, nil)
	if status != 200 {
		t.Fatalf("state: %d %s", status, body)
	}
	var stateResult struct {
		PluginID       string `json:"plugin_id"`
		LifecycleState string `json:"lifecycle_state"`
		PackageRef     string `json:"package_ref"`
	}
	if err := json.Unmarshal(body, &stateResult); err != nil {
		t.Fatalf("state body: %v (%s)", err, body)
	}
	if stateResult.PluginID != pluginID || stateResult.LifecycleState != "enabled" || stateResult.PackageRef == "" {
		t.Fatalf("the authoritative state: %s", body)
	}

	// The other direction through the ONE member: disable.
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/enable", pluginEnableBody(false), env.token, nil)
	if status != 200 {
		t.Fatalf("disable: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins/"+pluginID+"/state", "", env.token, nil)
	if pluginField(t, body, "lifecycle_state") != "disabled" {
		t.Fatalf("the disabled state (DISTINCT from the revoked states): %s", body)
	}

	// The M-30 update member: the fresh update succeeds (the target's
	// requested-of-record is EMPTY on this Core — no package-content
	// examination exists); the keyed replay returns the recorded outcome.
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/update", pluginUpdateBody("ukey-w1", "corr-u1", "target-1"), env.token, nil)
	if status != 200 {
		t.Fatalf("update: %d %s", status, body)
	}
	updateRef := pluginField(t, body, "operation_ref")
	if updateRef == "" {
		t.Fatalf("the update outcome: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/update", pluginUpdateBody("ukey-w1", "corr-u1", "target-1"), env.token, nil)
	if status != 200 || pluginField(t, body, "operation_ref") != updateRef || !pluginBool(t, body, "replayed") {
		t.Fatalf("the update replay returns the recorded outcome: %d %s", status, body)
	}

	// The G7 revoke-grants member: the state moves to grants-revoked
	// (DISTINCT from disabled and trust-revoked).
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/grants/revoke", pluginMutationBody("rkey-w1", "corr-r1"), env.token, nil)
	if status != 200 {
		t.Fatalf("revoke grants: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins/"+pluginID+"/state", "", env.token, nil)
	if pluginField(t, body, "lifecycle_state") != "grants_revoked" {
		t.Fatalf("the grants-revoked state: %s", body)
	}

	// The distinctness laws' consequence at the wire: the toggle refusal
	// from the revoked state rides the published register code the state
	// carries (a revoked state is never exitable through the toggle).
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/enable", pluginEnableBody(true), env.token, nil)
	if status != 403 {
		t.Fatalf("the toggle refusal from the revoked state: %d %s", status, body)
	}
	code, _ := workflowEnvelopeCode(t, body)
	if code != "PLUGIN_CAPABILITY_DENIED" {
		t.Fatalf("the distinctness law's typed refusal: %s", body)
	}

	// The M-31 remove member: the record leaves the installed set; the
	// keyed replay returns the recorded outcome.
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/remove", pluginMutationBody("mkey-w1", "corr-m1"), env.token, nil)
	if status != 200 {
		t.Fatalf("remove: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins/"+pluginID+"/state", "", env.token, nil)
	if status != 404 {
		t.Fatalf("the removed plugin: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/"+pluginID+"/remove", pluginMutationBody("mkey-w1", "corr-m1"), env.token, nil)
	if status != 200 || !pluginBool(t, body, "replayed") {
		t.Fatalf("the remove replay returns the recorded outcome: %d %s", status, body)
	}

	// The SAME typed 404 (the no-leak rule) on every {plugin_id} route.
	for _, p := range []string{
		"/v1/plugins/plug-nope",
		"/v1/plugins/plug-nope/state",
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
	for _, p := range []struct{ method, path, body string }{
		{"POST", "/v1/plugins/plug-nope/enable", pluginEnableBody(true)},
		{"POST", "/v1/plugins/plug-nope/update", pluginUpdateBody("k", "c", "r")},
		{"POST", "/v1/plugins/plug-nope/remove", pluginMutationBody("k", "c")},
		{"POST", "/v1/plugins/plug-nope/grants/revoke", pluginMutationBody("k", "c")},
	} {
		status, body = env.do(t, p.method, p.path, p.body, env.token, nil)
		if status != 404 {
			t.Fatalf("%s %s: the unknown destination: %d %s", p.method, p.path, status, body)
		}
		code, _ = workflowEnvelopeCode(t, body)
		if code != "NOT_FOUND" {
			t.Fatalf("%s %s: the SAME typed 404: %s", p.method, p.path, body)
		}
	}

	// Method mismatches on known paths resolve the register's NOT_FOUND.
	status, _ = env.do(t, "DELETE", "/v1/plugins", "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch: %d", status)
	}
	status, _ = env.do(t, "DELETE", "/v1/plugins/"+pluginID, "", env.token, nil)
	if status != 404 {
		t.Fatalf("method mismatch on the inspect route: %d", status)
	}
}

// TestPluginWireRefusals pins the typed 403s and the route-shape behavior:
// every B5 surface refuses the session-less caller with
// AUTHZ_DENIED/SESSION_INVALID; a mutating route with a foreign
// non-loopback Origin refuses with ORIGIN_NOT_ALLOWED.
func TestPluginWireRefusals(t *testing.T) {
	env := newTestEnv(t)

	// Probe 1: a read without session — the typed 403 with SESSION_INVALID.
	status, body := env.do(t, "GET", "/v1/plugins", "", "", nil)
	if status != 403 {
		t.Fatalf("no-session list: %d %s", status, body)
	}
	code, data := workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "SESSION_INVALID" {
		t.Fatalf("the session boundary refusal: %s", body)
	}

	// Every B5 route refuses the session-less caller with the typed 403.
	noSessionPaths := []struct{ method, path, body string }{
		{"GET", "/v1/plugins", ""},
		{"POST", "/v1/plugins", pluginInstallBody("k", "c", "r")},
		{"GET", "/v1/plugins/plug-x", ""},
		{"GET", "/v1/plugins/plug-x/state", ""},
		{"POST", "/v1/plugins/plug-x/enable", pluginEnableBody(true)},
		{"POST", "/v1/plugins/plug-x/update", pluginUpdateBody("k", "c", "r")},
		{"POST", "/v1/plugins/plug-x/remove", pluginMutationBody("k", "c")},
		{"POST", "/v1/plugins/plug-x/grants/revoke", pluginMutationBody("k", "c")},
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
	status, body = env.do(t, "POST", "/v1/plugins", pluginInstallBody("k", "c", "r"), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin install: %d %s", status, body)
	}
	code, data = workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" || data["reason"] != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("the Origin boundary refusal: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/plug-x/enable", pluginEnableBody(true), "",
		map[string]string{"Origin": "https://evil.example.com"})
	if status != 403 {
		t.Fatalf("foreign-Origin toggle: %d %s", status, body)
	}
	code, _ = workflowEnvelopeCode(t, body)
	if code != "AUTHZ_DENIED" {
		t.Fatalf("the toggle's Origin refusal: %s", body)
	}
}

// TestPluginInvocationAbsence is THE SCRIPTED ABSENCE CHECK (the GATED
// execution rows, carried exactly): NO invocation/execution operation is
// reachable — no invocation submission, no in-flight invocation state, no
// invocation-status read, no quota-value reporting, no steering/stop
// control. Every plausible invocation path resolves the router's NOT_FOUND
// (where a GET path resolves the generic inspect read's unknown-identity
// 404, the boundary still answers — never an invocation operation).
func TestPluginInvocationAbsence(t *testing.T) {
	env := newTestEnv(t)

	// A real plugin fixture (so the probes meet a live plugin, not just the
	// unknown-identity 404).
	status, body := env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-abs", "corr-abs", "pkgref-abs"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture install: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins?limit=1", "", env.token, nil)
	var listResult struct {
		Plugins []struct {
			PluginID string `json:"plugin_id"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &listResult); err != nil || len(listResult.Plugins) != 1 {
		t.Fatalf("fixture list: %v (%s)", err, body)
	}
	pluginID := listResult.Plugins[0].PluginID

	// The invocation/execution path matrix: every plausible invocation
	// submission, in-flight invocation state, invocation-status read,
	// quota-value report, and steering/stop surface — under BOTH methods —
	// is unreachable.
	invocationPaths := []string{
		"/v1/plugins/" + pluginID + "/invoke",
		"/v1/plugins/" + pluginID + "/invocations",
		"/v1/plugins/" + pluginID + "/invoke/start",
		"/v1/plugins/" + pluginID + "/invoke/cancel",
		"/v1/plugins/" + pluginID + "/invoke/stop",
		"/v1/plugins/" + pluginID + "/execute",
		"/v1/plugins/" + pluginID + "/execution",
		"/v1/plugins/" + pluginID + "/quota",
		"/v1/plugins/" + pluginID + "/invocations/inv-1",
		"/v1/plugins/" + pluginID + "/invocations/inv-1/status",
		"/v1/plugins/" + pluginID + "/invocations/inv-1/steer",
		"/v1/invocations",
		"/v1/invocations/inv-1",
		"/v1/plugin-invocations",
		"/v1/plugins/" + pluginID + "/steer",
		"/v1/plugins/" + pluginID + "/stop",
	}
	for _, p := range invocationPaths {
		for _, method := range []string{"POST", "GET"} {
			status, body := env.do(t, method, p, `{}`, env.token, nil)
			if status != 404 {
				t.Fatalf("%s %s: NO invocation/execution operation may be reachable (the GATED rows), got %d %s", method, p, status, body)
			}
			code, _ := workflowEnvelopeCode(t, body)
			if code != "NOT_FOUND" {
				t.Fatalf("%s %s: the absence resolves the register's NOT_FOUND: %s", method, p, body)
			}
		}
	}
}

// TestPluginRemoveRevokeDistinctnessWire pins THE REMOVE-VS-REVOKE
// DISTINCTNESS over the live loopback test server (the G-09a fixture set —
// the recorded 2026-11-09 resolution act, binding on this family): the two
// published members are DISTINCT Core semantics — removing a plugin does
// NOT revoke its grants; revoking grants does NOT remove the artifact;
// each acts only on its own named object. The wire's served outcomes pin
// the distinctness: the M-31 remove member's outcome is the artifact GONE
// (the state endpoint resolves the typed 404 and the list no longer
// carries it); the G7 revoke-grants member's outcome is the
// grants-revoked state on the STILL-INSTALLED artifact (the state endpoint
// still resolves it and the list still carries it); and the two intents
// are distinct shapes (the same client body key across the two routes
// carries two independent recorded outcomes; a post-removal revoke intent
// resolves the typed 404; a removal after a revocation still applies).
func TestPluginRemoveRevokeDistinctnessWire(t *testing.T) {
	env := newTestEnv(t)

	// The fixture install (the Core mints the identities; the body never
	// carries one).
	status, body := env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-g09aw-r", "corr-g09aw-r", "pkgref-g09aw-r"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture install (remove case): %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins?limit=1", "", env.token, nil)
	if status != 200 {
		t.Fatalf("fixture list: %d %s", status, body)
	}
	var listResult struct {
		Plugins []struct {
			PluginID string `json:"plugin_id"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &listResult); err != nil || len(listResult.Plugins) != 1 {
		t.Fatalf("fixture list body: %v (%s)", err, body)
	}
	removedID := listResult.Plugins[0].PluginID

	// PART 1 — REMOVE-NEVER-REVOKES on the wire: the removal's outcome is
	// the artifact GONE.
	status, body = env.do(t, "POST", "/v1/plugins/"+removedID+"/remove", pluginMutationBody("mkey-g09aw", "corr-g09aw-rm"), env.token, nil)
	if status != 200 {
		t.Fatalf("remove: %d %s", status, body)
	}
	if pluginField(t, body, "plugin_id") != removedID || pluginBool(t, body, "replayed") {
		t.Fatalf("the Core-confirmed removal: %s", body)
	}
	status, body = env.do(t, "GET", "/v1/plugins/"+removedID+"/state", "", env.token, nil)
	if status != 404 {
		t.Fatalf("the removed artifact's state read: %d %s", status, body)
	}
	if code, _ := workflowEnvelopeCode(t, body); code != "NOT_FOUND" {
		t.Fatalf("the removed artifact is the typed NOT_FOUND: %s", body)
	}
	// The removal's replay resolves the RECORDED removal outcome (never a
	// second removal).
	status, body = env.do(t, "POST", "/v1/plugins/"+removedID+"/remove", pluginMutationBody("mkey-g09aw", "corr-g09aw-rm"), env.token, nil)
	if status != 200 || !pluginBool(t, body, "replayed") {
		t.Fatalf("the removal's replay returns the recorded outcome: %d %s", status, body)
	}

	// PART 2 — REVOKE-NEVER-REMOVES on the wire: the revocation's outcome
	// is the grants-revoked state on the STILL-INSTALLED artifact.
	status, body = env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-g09aw-v", "corr-g09aw-v", "pkgref-g09aw-v"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture install (revoke case): %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins?limit=50", "", env.token, nil)
	if status != 200 {
		t.Fatalf("fixture list: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &listResult); err != nil {
		t.Fatalf("fixture list body: %v (%s)", err, body)
	}
	var revokedID string
	for _, p := range listResult.Plugins {
		if p.PluginID != removedID {
			revokedID = p.PluginID
		}
	}
	if revokedID == "" {
		t.Fatalf("the second fixture plugin is missing: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/"+revokedID+"/grants/revoke", pluginMutationBody("rkey-g09aw", "corr-g09aw-v"), env.token, nil)
	if status != 200 {
		t.Fatalf("revoke grants: %d %s", status, body)
	}
	// The state endpoint still resolves the artifact — serving the
	// grants-revoked state (DISTINCT from a removed artifact's typed 404).
	status, body = env.do(t, "GET", "/v1/plugins/"+revokedID+"/state", "", env.token, nil)
	if status != 200 || pluginField(t, body, "lifecycle_state") != "grants_revoked" {
		t.Fatalf("the artifact stays installed under the grants-revoked state: %d %s", status, body)
	}
	// The artifact is still inspectable and still listed (revoking grants
	// does NOT remove the artifact).
	status, body = env.do(t, "GET", "/v1/plugins/"+revokedID, "", env.token, nil)
	if status != 200 || pluginField(t, body, "lifecycle_state") != "grants_revoked" {
		t.Fatalf("the artifact stays inspectable after the revocation: %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins?limit=50", "", env.token, nil)
	if err := json.Unmarshal(body, &listResult); err != nil {
		t.Fatalf("list body: %v (%s)", err, body)
	}
	stillListed := false
	for _, p := range listResult.Plugins {
		if p.PluginID == revokedID {
			stillListed = true
		}
	}
	if !stillListed {
		t.Fatalf("revoking grants does NOT remove the artifact — it stays listed: %s", body)
	}
	// The revocation's replay resolves the RECORDED revocation outcome.
	status, body = env.do(t, "POST", "/v1/plugins/"+revokedID+"/grants/revoke", pluginMutationBody("rkey-g09aw", "corr-g09aw-v"), env.token, nil)
	if status != 200 || !pluginBool(t, body, "replayed") {
		t.Fatalf("the revocation's replay returns the recorded outcome: %d %s", status, body)
	}

	// PART 3 — THE DISTINCT OUTCOME IDENTITIES on the wire.
	// (a) The same client body key across the two routes carries two
	// independent recorded outcomes (the distinct keying scopes: the
	// remove scope vs the revoke scope).
	status, body = env.do(t, "POST", "/v1/plugins", pluginInstallBody("key-g09aw-d", "corr-g09aw-d", "pkgref-g09aw-d"), env.token, nil)
	if status != 200 {
		t.Fatalf("fixture install (identities case): %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins?limit=50", "", env.token, nil)
	if err := json.Unmarshal(body, &listResult); err != nil {
		t.Fatalf("list body: %v (%s)", err, body)
	}
	var distinctID string
	for _, p := range listResult.Plugins {
		if p.PluginID != removedID && p.PluginID != revokedID {
			distinctID = p.PluginID
		}
	}
	if distinctID == "" {
		t.Fatalf("the third fixture plugin is missing: %s", body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/"+distinctID+"/grants/revoke", pluginMutationBody("dkey-g09aw", "corr-g09aw-d1"), env.token, nil)
	if status != 200 {
		t.Fatalf("the revocation under the shared client key: %d %s", status, body)
	}
	status, body = env.do(t, "POST", "/v1/plugins/"+distinctID+"/remove", pluginMutationBody("dkey-g09aw", "corr-g09aw-d2"), env.token, nil)
	if status != 200 {
		t.Fatalf("the removal under the SAME client key does not collide with the revocation's record (distinct keying scopes): %d %s", status, body)
	}
	// (b) A post-removal revoke intent cannot resolve (the revoke's own
	// named object no longer exists once the artifact is removed).
	status, body = env.do(t, "POST", "/v1/plugins/"+distinctID+"/grants/revoke", pluginMutationBody("rkey-post", "corr-post"), env.token, nil)
	if status != 404 {
		t.Fatalf("a revoke intent on a removed artifact must refuse: %d %s", status, body)
	}
	if code, _ := workflowEnvelopeCode(t, body); code != "NOT_FOUND" {
		t.Fatalf("the post-removal revoke intent is the typed NOT_FOUND: %s", body)
	}
	// (c) A removal after a revocation still applies (the artifact is
	// still installed — grants-revoked is not trust-revoked).
	status, body = env.do(t, "POST", "/v1/plugins/"+revokedID+"/remove", pluginMutationBody("mkey-after-revoke", "corr-after-revoke"), env.token, nil)
	if status != 200 {
		t.Fatalf("the removal after the revocation (the artifact still installed): %d %s", status, body)
	}
	status, body = env.do(t, "GET", "/v1/plugins/"+revokedID+"/state", "", env.token, nil)
	if status != 404 {
		t.Fatalf("the removed artifact's state read: %d %s", status, body)
	}
}
