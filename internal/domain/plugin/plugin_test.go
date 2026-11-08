package plugin

import (
	"encoding/json"
	"testing"
)

// The B5 Plugin lifecycle family's domain register (DE-0 queue row 8): the
// closed registers carry the published canonical values VERBATIM (the
// lifecycle-state values, the event aggregate and the seven event types,
// the seven error-code strings), the wire shapes' JSON tags are the
// contract member names, and THE GATED EXECUTION ROWS stand exactly as
// recorded (the invocation/execution operation does not exist; the
// family's own support constant carries the recorded state — never flipped
// by implementation).

// TestClosedRegisters pins every closed register to the published values.
func TestClosedRegisters(t *testing.T) {
	// The lifecycle-state values: the five DISTINCT states (the
	// distinctness laws bind structurally — DISABLED ≠ GRANTS-REVOKED ≠
	// TRUST-REVOKED; installed is the installed NOT-yet-enabled state).
	states := []string{StateInstalled, StateEnabled, StateDisabled, StateGrantsRevoked, StateTrustRevoked}
	wantStates := map[string]bool{
		"installed": true, "enabled": true, "disabled": true,
		"grants_revoked": true, "trust_revoked": true,
	}
	if len(states) != 5 {
		t.Fatalf("the lifecycle-state register carries exactly five distinct values: %v", states)
	}
	for _, s := range states {
		if !wantStates[s] || !ValidLifecycleState(s) {
			t.Fatalf("the lifecycle-state register carries the published values verbatim: %q", s)
		}
	}
	if ValidLifecycleState("") || ValidLifecycleState("running") || ValidLifecycleState("invoking") {
		t.Fatalf("the lifecycle-state register is closed (no invocation/in-flight state exists — the GATED row)")
	}

	// The F10 event aggregate: plugin.
	if AggregatePlugin != "plugin" {
		t.Fatalf("the event-aggregate register carries the published value verbatim")
	}
	if !ValidAggregateType("plugin") || ValidAggregateType("") || ValidAggregateType("pluginInvocation") {
		t.Fatalf("the event-aggregate register is closed")
	}

	// The F10 event types: the seven lifecycle types. NO
	// invocation/execution event exists — the vocabulary is closed exactly
	// at the published seven (the GATED row; nothing drafts one).
	eventTypes := map[string]bool{
		EventInstallCompleted: true, EventEnabled: true, EventDisabled: true,
		EventUpdated: true, EventRemoved: true, EventGrantsRevoked: true, EventTrustRevoked: true,
	}
	if len(eventTypes) != 7 || !ValidEventType("plugin.install_completed") ||
		!ValidEventType("plugin.enabled") || !ValidEventType("plugin.disabled") ||
		!ValidEventType("plugin.updated") || !ValidEventType("plugin.removed") ||
		!ValidEventType("plugin.grants_revoked") || !ValidEventType("plugin.trust_revoked") {
		t.Fatalf("the event-type register carries the published values verbatim")
	}
	if ValidEventType("") || ValidEventType("plugin.invoked") || ValidEventType("plugin.quota_exceeded") ||
		ValidEventType("plugin.invocation_started") {
		t.Fatalf("no invocation/execution event exists in the vocabulary (the GATED row)")
	}

	// The PluginErrorCode register strings (the published codes carried
	// verbatim; the idempotency/state conflicts ride the PUBLISHED codes —
	// no Plugin-specific conflict code exists). The three typed INVALID
	// outcomes stay DISTINCT (the shape question RECORDED OPEN).
	codes := map[string]string{
		FailTrustUnknown:     "PLUGIN_TRUST_UNKNOWN",
		FailSignatureInvalid: "PLUGIN_SIGNATURE_INVALID",
		FailDigestMismatch:   "PLUGIN_DIGEST_MISMATCH",
		FailManifestInvalid:  "PLUGIN_MANIFEST_INVALID",
		FailABIIncompatible:  "PLUGIN_ABI_INCOMPATIBLE",
		FailTrustRevoked:     "PLUGIN_TRUST_REVOKED",
		FailCapabilityDenied: "PLUGIN_CAPABILITY_DENIED",
	}
	if len(codes) != 7 {
		t.Fatalf("the error-code register carries exactly seven distinct codes: %v", codes)
	}
	for got, want := range codes {
		if got != want {
			t.Fatalf("the error-code register carries the published strings verbatim: %q", got)
		}
	}
}

// TestGatedExecutionConstant pins THE GATED EXECUTION ROWS exactly as
// recorded: the invocation/execution operation does not exist; the
// family's own support constant carries the recorded gate state (flipping
// it is the un-gate act, its own LIVE-gate class — never a side effect of
// implementation).
func TestGatedExecutionConstant(t *testing.T) {
	if InvocationSupported {
		t.Fatalf("the gated execution rows stand: the invocation/execution operation does not exist; the support constant must carry the recorded state")
	}
}

// TestToggleTargetable pins the distinctness laws' structural consequence:
// the toggle operates on the installed/enabled/disabled class only; a
// revoked state is NEVER exitable through the toggle (a user toggle never
// merges a revoked state into a user-action state).
func TestToggleTargetable(t *testing.T) {
	cases := map[string]bool{
		StateInstalled:     true,
		StateEnabled:       true,
		StateDisabled:      true,
		StateGrantsRevoked: false,
		StateTrustRevoked:  false,
		"":                 false,
	}
	for state, want := range cases {
		if got := ToggleTargetable(state); got != want {
			t.Fatalf("ToggleTargetable(%q) = %v, want %v", state, got, want)
		}
	}
}

// TestWireShapeJSONTags pins the wire shapes' JSON tags to the contract
// member names verbatim (the coded-name rule; never a restatement).
func TestWireShapeJSONTags(t *testing.T) {
	// PluginManifest (the required three; the optional members omitted).
	b, err := json.Marshal(Manifest{ManifestVersion: "1", PluginID: "p1", PluginVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"manifest_version":"1","plugin_id":"p1","plugin_version":"v1"}` {
		t.Fatalf("the PluginManifest wire shape: %s", b)
	}

	// PluginPackage (the required package_ref; the verification states
	// absent where the Core returns none — never fabricated).
	b, err = json.Marshal(Package{PackageRef: "pkg-1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"package_ref":"pkg-1"}` {
		t.Fatalf("the PluginPackage wire shape: %s", b)
	}

	// Plugin (the record renders ONLY Core-returned verification states and
	// lifecycle state; the manifest member is OPTIONAL).
	b, err = json.Marshal(Plugin{PluginID: "p1", LifecycleState: StateInstalled, InstallRecordRef: "plugrec-1", InstalledAt: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"plugin_id":"p1","lifecycle_state":"installed","install_record_ref":"plugrec-1","installed_at":"t"}` {
		t.Fatalf("the Plugin wire shape: %s", b)
	}

	// PluginList (the keyset page; the cursor's absence marks the final
	// page).
	b, err = json.Marshal(PluginList{Plugins: []Plugin{{PluginID: "p1", LifecycleState: StateInstalled}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"plugins":[{"plugin_id":"p1","lifecycle_state":"installed"}]}` {
		t.Fatalf("the PluginList wire shape: %s", b)
	}

	// PluginInstallRequest / PluginInstallResult (the C1-07 class — the
	// body never carries an identity).
	b, err = json.Marshal(InstallRequest{IdempotencyKey: "k", CorrelationID: "c", PackageRef: "ref"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"idempotency_key":"k","correlation_id":"c","package_ref":"ref"}` {
		t.Fatalf("the PluginInstallRequest wire shape: %s", b)
	}
	b, err = json.Marshal(InstallResult{InstallRecordRef: "plugrec-1", Replayed: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"install_record_ref":"plugrec-1","replayed":true}` {
		t.Fatalf("the PluginInstallResult wire shape: %s", b)
	}

	// PluginEnableRequest (the ONE member — the carrier gap recorded
	// honestly; the wire schema governs).
	b, err = json.Marshal(EnableRequest{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"enabled":true}` {
		t.Fatalf("the PluginEnableRequest wire shape: %s", b)
	}

	// PluginUpdateRequest.
	b, err = json.Marshal(UpdateRequest{IdempotencyKey: "k", CorrelationID: "c", TargetVersionRef: "ref-2"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"idempotency_key":"k","correlation_id":"c","target_version_ref":"ref-2"}` {
		t.Fatalf("the PluginUpdateRequest wire shape: %s", b)
	}

	// PluginMutationRequest / PluginMutationResult (the M-31/G7 classes).
	b, err = json.Marshal(MutationRequest{IdempotencyKey: "k", CorrelationID: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"idempotency_key":"k","correlation_id":"c"}` {
		t.Fatalf("the PluginMutationRequest wire shape: %s", b)
	}
	b, err = json.Marshal(MutationResult{PluginID: "p1", OperationRef: "plugop-1", Replayed: false})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"plugin_id":"p1","operation_ref":"plugop-1","replayed":false}` {
		t.Fatalf("the PluginMutationResult wire shape: %s", b)
	}

	// PluginState (the authoritative state record — the optional members
	// absent where the Core reports none).
	b, err = json.Marshal(State{PluginID: "p1", LifecycleState: StateEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"plugin_id":"p1","lifecycle_state":"enabled"}` {
		t.Fatalf("the PluginState wire shape: %s", b)
	}
}

// TestKeyFingerprintBuildersAndFreshReview pins the keyed classes'
// identity builders and the fresh-review law's changed-set computation.
func TestKeyFingerprintBuildersAndFreshReview(t *testing.T) {
	// The install key is package-identity-scoped; distinct refs never
	// collide under the same client key.
	if InstallKey("k1", "ref-1") == InstallKey("k1", "ref-2") {
		t.Fatalf("the install key is scoped by the package identity")
	}
	// The install fingerprint joins the correlation ID and the package
	// artifact/version reference.
	if InstallFingerprint("corr-1", "ref-1") == InstallFingerprint("corr-2", "ref-1") ||
		InstallFingerprint("corr-1", "ref-1") == InstallFingerprint("corr-1", "ref-2") {
		t.Fatalf("the install fingerprint distinguishes both halves")
	}
	// The update key is target-version-scoped; the update fingerprint joins
	// the correlation ID and the target version reference.
	if UpdateKey("k1", "ref-1") == UpdateKey("k1", "ref-2") {
		t.Fatalf("the update key is scoped by the target version reference")
	}
	if FingerprintRef(UpdateFingerprint("corr-1", "ref-9")) != "ref-9" {
		t.Fatalf("the fingerprint's reference half is the reconciliation key")
	}
	// The remove and revoke keys are plugin-identity-scoped (the revoke's
	// grant-scope component is Core-side); the two classes never collide.
	if RemoveKey("k1", "p1") == RevokeKey("k1", "p1") {
		t.Fatalf("the remove and revoke classes never collide under the same key")
	}
	if RemoveKey("k1", "p1") == RemoveKey("k1", "p2") {
		t.Fatalf("the remove key is scoped by the plugin identity")
	}

	// THE FRESH-REVIEW LAW's changed set: the target's requested
	// capabilities not in the granted set of record — sorted and
	// deduplicated (references only).
	delta := FreshReviewDelta([]string{"knowledge.read", "jobs.submit"}, []string{"jobs.submit", "network.connect", "knowledge.read", "network.connect"})
	if len(delta) != 1 || delta[0] != "network.connect" {
		t.Fatalf("the changed set: %v", delta)
	}
	if got := FreshReviewDelta([]string{"knowledge.read"}, []string{"knowledge.read"}); len(got) != 0 {
		t.Fatalf("no change is no delta: %v", got)
	}
	if got := FreshReviewDelta(nil, []string{"jobs.submit"}); len(got) != 1 || got[0] != "jobs.submit" {
		t.Fatalf("an empty granted set of record flags every request: %v", got)
	}
}
