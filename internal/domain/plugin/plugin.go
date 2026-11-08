// Package plugin defines the B5 Plugin lifecycle family's wire shapes and
// closed registers (the published canonical contract family of
// contracts/openapi.yaml — the Plugin* schemas, the PluginErrorCode
// register, the PluginEvent* vocabularies, and the PluginId parameter,
// carried verbatim: never a restatement, never an extension). The package
// is part of the domain leaf: it imports no other internal package and
// carries no transport, database, or OS types.
//
// THE GATED EXECUTION ROWS (carried exactly, binding): THE
// INVOCATION/EXECUTION OPERATION DOES NOT EXIST — no invocation operation,
// no invocation/in-flight state schema, no invocation-status read, no
// invocation events, no quota-value reporting, no steering/stop controls.
// GATED [D-13/D-14 + the Core MB §10.2 runtime proof]. The Core MB §17.5
// Plugin call reference sequence is CITED, never extended (every step is
// Core-owned; no surface observes anything the Core does not report).
//
// THE NEVER-GRANT LAW (binding): the plugin itself can NEVER grant
// permissions — not to itself, not to anything else; the user
// grants/denies; the Core mediates and enforces; every grant is explicit,
// scoped, revocable, auditable, and quota-bound. The concrete enumerated
// capability set, each request's scope, and the grant-scope class are
// UNSUPPLIED [B5] — the capability CLASS only rides this family and NO
// capability catalogue is published or invented here.
//
// THE DISTINCTNESS LAWS (carried, binding): DISABLED (the user's action) ≠
// GRANTS-REVOKED ≠ TRUST-REVOKED; INVOCATION TERMINATION ≠ PLUGIN
// DISABLE/REVOCATION (a WASM quota failure terminates THAT invocation,
// preserves Core stability, and emits safe audit evidence — it does not
// disable or revoke the plugin); USER-INSTALLED ≠ CATALOG-LISTED (catalog
// presence is metadata, never a trust signal — and a catalog exists ONLY
// when the Core supplies one: none is published).
//
// THE DUTY SPLIT (binding, carried): REDACTION IS THE CORE'S DUTY — the
// UI displays only Core-returned sanitized content and never exposes,
// requests, or fabricates an unredacted channel. The MAY-NEVER LIST: no
// secret values of any kind (a named secret-handle's NAME only, never its
// value); no raw logs, runtime internals, stack traces, or memory
// contents; no raw plugin output or raw provider payloads as diagnostics;
// no diagnostic the Core did not return.
//
// THE S0–S4 LAW (binding): a lost or unknown outcome reconciles by the
// authoritative plugin/package identity through the family's state read
// BEFORE any retry — never by timeout alone, never a blind retry, never an
// optimistic confirmation.
package plugin

import "sort"

// THE GATE CONSTANT (the family's own support state — the recorded gate,
// NOT resolved by any implementation act; flipping it is the un-gate act,
// its own LIVE-gate class, never a side effect). FALSE today: the
// invocation/execution operation does not exist, no invocation/in-flight
// state schema exists, no invocation event exists, no quota-value
// reporting exists, no steering/stop control exists — D-13/D-14 and the
// Core MB §10.2 runtime proof keep their recorded states.
const InvocationSupported = false

// The lifecycle-state values (THE CORE'S OWN SERVED VALUES — recorded
// honestly as this Core's state-value selection: the state-machine wire
// values are UNSUPPLIED [B5] — the Core's contract decides — and no
// register is invented in the canonical set; the Core implementation must
// still store and return ITS OWN state, and these are this Core's served
// values under that recorded gap, the served-definition class of the
// recorded-gap precedents). The five values are DISTINCT — the
// distinctness laws bind structurally: DISABLED (the user's action) ≠
// GRANTS-REVOKED ≠ TRUST-REVOKED, each a distinct value never merged into
// another; `installed` is the installed NOT-yet-enabled state every
// Core-confirmed install lands in (activation is separate).
const (
	StateInstalled     = "installed"
	StateEnabled       = "enabled"
	StateDisabled      = "disabled"
	StateGrantsRevoked = "grants_revoked"
	StateTrustRevoked  = "trust_revoked"
)

// ValidLifecycleState reports membership in the closed lifecycle-state
// value set (the Core's own served values — the five distinct states).
func ValidLifecycleState(s string) bool {
	switch s {
	case StateInstalled, StateEnabled, StateDisabled, StateGrantsRevoked, StateTrustRevoked:
		return true
	default:
		return false
	}
}

// ToggleTargetable reports whether the enabled/disabled toggle may apply
// to a plugin in the stated lifecycle state. THE DISTINCTNESS LAW'S
// CONSEQUENCE (binding): the toggle is its own state class — it operates
// on the installed/enabled/disabled class only; a revoked state is NEVER
// exitable through the toggle (a user toggle never merges a revoked state
// into a user-action state; re-grant is the user's explicit act through
// the Core's defined path, and re-admission after a trust revocation is
// the Core policy's decision alone). UNSUPPLIED transition detail
// (whether a disable→enable cycle re-opens the permission review) stays
// recorded Open — this predicate decides no review question.
func ToggleTargetable(state string) bool {
	switch state {
	case StateInstalled, StateEnabled, StateDisabled:
		return true
	default:
		return false
	}
}

// The F10-shaped event aggregate type (the published
// PluginEventAggregateType register carried verbatim; events ride the
// frozen F10 EventRecord field set and are never the source of truth —
// the authoritative state endpoint is).
const AggregatePlugin = "plugin"

// ValidAggregateType reports membership in the closed aggregate register.
func ValidAggregateType(s string) bool {
	return s == AggregatePlugin
}

// The F10-shaped event_type vocabulary (the published PluginEventType
// register carried verbatim — each naming a Core-confirmed lifecycle
// transition). NO invocation/execution event exists in this vocabulary —
// that vocabulary is a GATED row [D-13/D-14 + the Core MB §10.2 runtime
// proof] and nothing drafts one.
//
// `plugin.grants_revoked` records the grant-revocation transition
// (GRANTS-REVOKED ≠ TRUST-REVOKED ≠ DISABLED — each distinct);
// `plugin.trust_revoked` records the Core's trust-revocation transition.
const (
	EventInstallCompleted = "plugin.install_completed"
	EventEnabled          = "plugin.enabled"
	EventDisabled         = "plugin.disabled"
	EventUpdated          = "plugin.updated"
	EventRemoved          = "plugin.removed"
	EventGrantsRevoked    = "plugin.grants_revoked"
	EventTrustRevoked     = "plugin.trust_revoked"
)

// ValidEventType reports membership in the closed event-type register.
func ValidEventType(s string) bool {
	switch s {
	case EventInstallCompleted, EventEnabled, EventDisabled, EventUpdated,
		EventRemoved, EventGrantsRevoked, EventTrustRevoked:
		return true
	default:
		return false
	}
}

// The Plugin family's domain codes (the published PluginErrorCode register
// carried verbatim — each riding the published ErrorEnvelope with its
// PUBLISHED retryable assignment and the coded retryClass; the codes join
// the coded register additively at their implementation, the established
// register-extension class). THE SHAPE QUESTION RECORDED OPEN: whether the
// Core collapses the three typed INVALID outcomes
// (PLUGIN_SIGNATURE_INVALID, PLUGIN_DIGEST_MISMATCH,
// PLUGIN_MANIFEST_INVALID) into one typed `data` payload instead is still
// OPEN — the three codes stay DISTINCT here as the contract publishes
// them, and both readings satisfy the consuming table's distinctness law.
// The idempotency/state conflicts ride the PUBLISHED codes exactly
// (CONFLICT_IDEMPOTENCY_KEY / CONFLICT_STATE) — no Plugin-specific
// conflict code exists.
const (
	// FailTrustUnknown — the UNKNOWN admission state's outcome: publisher/
	// trust root unverified or not determinable by the Core; the state can
	// change only Core-side — reconcile through the authoritative state
	// read before any retry, never a blind resubmit. The published
	// retryable assignment: reconcile (the coded RetryReconcile class).
	FailTrustUnknown = "PLUGIN_TRUST_UNKNOWN"
	// FailSignatureInvalid — the signature-verification failure; a DISTINCT
	// typed INVALID outcome; a fact about the submitted bytes; a retry of
	// the same bytes re-fails deterministically. The published retryable
	// assignment: never.
	FailSignatureInvalid = "PLUGIN_SIGNATURE_INVALID"
	// FailDigestMismatch — the digest mismatch; a DISTINCT typed INVALID
	// outcome; a fact about the submitted bytes. The published retryable
	// assignment: never.
	FailDigestMismatch = "PLUGIN_DIGEST_MISMATCH"
	// FailManifestInvalid — the malformed-manifest outcome; a DISTINCT
	// typed INVALID outcome; a fact about the submitted manifest. The
	// published retryable assignment: never.
	FailManifestInvalid = "PLUGIN_MANIFEST_INVALID"
	// FailABIIncompatible — the INCOMPATIBLE state's outcome: the
	// ABI/runtime mismatch; update availability is rendered where the Core
	// reports it. The published retryable assignment: never.
	FailABIIncompatible = "PLUGIN_ABI_INCOMPATIBLE"
	// FailTrustRevoked — the REVOKED state's outcome: trust revoked; FUTURE
	// invocations blocked and the current invocation's behavior
	// Core-reported — never guessed. The published retryable assignment:
	// never.
	FailTrustRevoked = "PLUGIN_TRUST_REVOKED"
	// FailCapabilityDenied — the DENIED state's outcome: policy denies a
	// required capability, or the user denies it in the review; a retry
	// never grants — the user's explicit act governs. The published
	// retryable assignment: never.
	FailCapabilityDenied = "PLUGIN_CAPABILITY_DENIED"
)

// The published idempotency/retention selections carried as the recorded
// window statements (stated in the published descriptions; carried here as
// the recorded values only — the windows govern the keyed classes at their
// implementation):
//   - install (the recorded C1-07 class): key scoped workspace + package
//     identity Core-side; fingerprint the request identity (the
//     correlation ID) + the package artifact digest/version reference;
//     replay window 72 h post-terminal (the keyed creation-class
//     precedent — the same class B1's import submit, B2's add, and B4's
//     create received).
//   - update (the recorded M-30 class): key scoped workspace + package
//     identity; fingerprint the correlation ID + the target version
//     reference; replay window 1 h post-terminal.
//   - remove (the recorded M-31 class) and revoke grants (the recorded G7
//     class): key scoped workspace + plugin identity (+ the grant-scope
//     component Core-side for revoke); fingerprint the correlation ID;
//     replay window 1 h post-terminal.
//   - the lifecycle list/reads: the M-1 correlation-only read class (no
//     idempotency row; none required).
//
// THE ENABLE MEMBER'S CARRIER GAP (recorded honestly): the published
// PluginEnableRequest body carries exactly ONE member (`enabled`) — the
// keyed-class members its description references (the idempotency key, the
// correlation ID) are UNSUPPLIED on the published body, so the served
// toggle is the Core-recorded state-checked mutation (each toggle its own
// Core-confirmed record; the reconcile law is the state query the
// description itself carries); the keyed replay/conflict behaviors have no
// client carrier on this member. The wire schema governs.
const (
	InstallReplayWindowNote = "72 h post-terminal (the keyed creation-class precedent selected at the publication act)"
	ShortReplayWindowNote   = "1 h post-terminal (the short-window M-16-class precedent selected at the publication act)"
)

// Manifest is the plugin manifest structure (the published PluginManifest
// schema carried verbatim — JSON tags are the contract member names; the
// Core MB §10.1 field list carried exactly). THE NEVER-GRANT LAW,
// THE ABI LAW (the ABI version SEPARATE from the host capability version;
// an unknown required capability rejected Core-side; host calls re-check
// user/workspace scope and permissions per call), and THE PROVEN-ONLY
// QUOTA LAW (the manifest declares resource needs; the Core enforces the
// limits; no computed, assumed, or documented-in-theory budget;
// per-instruction CPU/fuel quota must NOT be assumed; wall-clock timeout
// is NOT fuel — the wazero caveat) ride this type's doc comment. THE D-41
// NOTE (the gate stays GATED): digest and signature members carry values
// only in the algorithm-qualified string form (`sha256:<lowercase-hex>`)
// as a value FORMAT — no algorithm is approved as the permanent digest
// profile. The compatibility-range representation, the concrete ABI
// version values, and the declared-resource wire representation are
// UNSUPPLIED [B5] — the Core's contract decides; this record carries the
// structure, not the values. The manifest member is OPTIONAL on the
// published Plugin record — where the Core returns none, none is served
// (never a fabricated manifest).
type Manifest struct {
	ManifestVersion       string   `json:"manifest_version"`
	PluginID              string   `json:"plugin_id"`
	PluginVersion         string   `json:"plugin_version"`
	PublisherRef          string   `json:"publisher_ref,omitempty"`
	ArtifactDigest        string   `json:"artifact_digest,omitempty"`
	ArtifactSignature     string   `json:"artifact_signature,omitempty"`
	ABIVersion            string   `json:"abi_version,omitempty"`
	RequestedCapabilities []string `json:"requested_capabilities,omitempty"`
	CompatibilityRange    string   `json:"compatibility_range,omitempty"`
	DeclaredResourceNeeds string   `json:"declared_resource_needs,omitempty"`
	LicenseRef            string   `json:"license_ref,omitempty"`
	LifecycleState        string   `json:"lifecycle_state,omitempty"`
}

// Package is the package structure (the published PluginPackage schema
// carried verbatim). The package is admitted only through the Core's
// verification: the artifact digest, the signature verification result,
// the trust root, and the rotation/revocation status are each
// CORE-RETURNED verification states rendered as returned, NEVER computed.
// THE TRUST-WORD DISCIPLINE (binding): "Verified," "Trusted," and
// "Installed" appear ONLY after the corresponding Core evidence and policy
// conditions are met. THE D-41 NOTE rides the digest member (the
// algorithm-qualified string form as a value FORMAT only). Each optional
// verification member is ABSENT where the Core returns none — an absent
// verification state is never rendered as a verified one.
type Package struct {
	PackageRef                  string `json:"package_ref"`
	ArtifactDigest              string `json:"artifact_digest,omitempty"`
	SignatureVerificationResult string `json:"signature_verification_result,omitempty"`
	TrustRootRef                string `json:"trust_root_ref,omitempty"`
	RotationStatus              string `json:"rotation_status,omitempty"`
	RevocationStatus            string `json:"revocation_status,omitempty"`
}

// Plugin is one installed plugin record (the published Plugin schema
// carried verbatim — the inspect member's payload). THE IDENTITY LAW: the
// Core mints the plugin, package, install-record, and grant identities —
// never client-minted (the published Core-mints-the-identity precedent).
// The record renders ONLY Core-returned verification states and lifecycle
// state; the trust-word discipline binds every rendering.
type Plugin struct {
	PluginID         string    `json:"plugin_id"`
	Manifest         *Manifest `json:"manifest,omitempty"`
	Package          *Package  `json:"package,omitempty"`
	LifecycleState   string    `json:"lifecycle_state"`
	InstallRecordRef string    `json:"install_record_ref,omitempty"`
	InstalledAt      string    `json:"installed_at,omitempty"`
}

// PluginList is the lifecycle list read's payload (the published
// PluginList schema carried verbatim — the keyset page; the cursor's
// absence marks the final page). USER-INSTALLED ≠ CATALOG-LISTED, always;
// catalog presence is metadata, never a trust signal.
type PluginList struct {
	Plugins    []Plugin `json:"plugins"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// InstallRequest is the install body (the published PluginInstallRequest
// schema carried verbatim) — the recorded C1-07 install class. The Core
// mints the plugin, package, install-record, and grant identities — the
// body never carries one. The key is scoped workspace + package identity
// Core-side; the fingerprint is the request identity (the correlation ID)
// + the package artifact digest/version reference; the replay window is
// 72 h post-terminal.
type InstallRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	PackageRef     string `json:"package_ref"`
}

// InstallResult is the Core-confirmed install outcome (the published
// PluginInstallResult schema carried verbatim) — the recorded C1-07
// class: a duplicate with the same key returns the SAME install-record
// reference, never a second one. The install lands the plugin in its
// installed, NOT-yet-enabled state.
type InstallResult struct {
	InstallRecordRef string `json:"install_record_ref"`
	Replayed         bool   `json:"replayed"`
}

// EnableRequest is the state-toggle body (the published PluginEnableRequest
// schema carried verbatim) — the recorded M-29 class as the state-toggle
// class — ONE member operation covers both directions via the target
// state (the published pin precedent's shape). THE CARRIER GAP (recorded
// honestly): the published body carries exactly this one member — the
// keyed-class members its description references are UNSUPPLIED here; the
// served toggle is the Core-recorded state-checked mutation.
type EnableRequest struct {
	Enabled bool `json:"enabled"`
}

// UpdateRequest is the update body (the published PluginUpdateRequest
// schema carried verbatim) — the recorded M-30 class. THE FRESH-REVIEW
// LAW (binding): ANY permission change triggers a FRESH review of the
// changed set — the update does not complete past an un-reviewed change.
// THE ATOMIC UPGRADE (binding; Core MB §10.3): no artifact overwrite while
// an old active plugin runs — the prior version keeps running until the
// Core confirms the switch.
type UpdateRequest struct {
	IdempotencyKey   string `json:"idempotency_key"`
	CorrelationID    string `json:"correlation_id"`
	TargetVersionRef string `json:"target_version_ref"`
}

// MutationRequest is the keyed mutation body shared by the remove and
// revoke-grants members (the published PluginMutationRequest schema
// carried verbatim — the recorded M-31 and G7 classes) — the plugin
// identity rides the path; the key scope and any grant-scope component
// are Core-side (the grant identity is Core-owned; the grant-scope class
// is UNSUPPLIED [B5]).
type MutationRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
}

// MutationResult is the Core-confirmed mutation outcome (the published
// PluginMutationResult schema carried verbatim) — the enable/disable,
// update, remove, and revoke-grants members' shared result shape (the
// published mutation-result precedent, MemoryMutationResult). The
// remove-vs-revoke question stays OPEN (no text asserts removal and grant
// revocation are the same or distinct — the Core's contract decides);
// revocation blocks FUTURE invocations and the current invocation's
// behavior is Core-reported — never guessed.
type MutationResult struct {
	PluginID     string `json:"plugin_id"`
	OperationRef string `json:"operation_ref"`
	Replayed     bool   `json:"replayed"`
}

// State is the authoritative plugin-state record (the published
// PluginState schema carried verbatim) — the family's authoritative state
// endpoint's payload (a pure query). Events are never the source of truth;
// this endpoint is. The distinctness laws bind the rendered state. The
// unknown identity and the out-of-scope identity are the SAME typed 404
// (the no-leak rule).
type State struct {
	PluginID         string `json:"plugin_id"`
	LifecycleState   string `json:"lifecycle_state"`
	PackageRef       string `json:"package_ref,omitempty"`
	InstallRecordRef string `json:"install_record_ref,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// The durable intents and outcomes (the store port's contracts — not wire
// shapes; the keyed classes' identity members).

// keySeparator joins the client key and the Core-side scope halves of the
// effective idempotency keys, and the fingerprint's halves (the shared
// separator class — references never collide across classes).
const keySeparator = "\x1f"

// InstallKey builds the C1-07 install class's effective key: the client
// key scoped workspace + package identity Core-side (the local Core IS
// the workspace scope; the package identity rides the stated package
// artifact/version reference).
func InstallKey(clientKey, packageRef string) string {
	return clientKey + keySeparator + "plugin.install:" + packageRef
}

// InstallFingerprint builds the C1-07 fingerprint: the request identity
// (the correlation ID) + the package artifact digest/version reference.
func InstallFingerprint(correlationID, packageRef string) string {
	return correlationID + keySeparator + packageRef
}

// UpdateKey builds the M-30 update class's effective key: the client key
// scoped workspace + package identity Core-side (the target version
// reference is the package-identity scope component).
func UpdateKey(clientKey, targetVersionRef string) string {
	return clientKey + keySeparator + "plugin.update:" + targetVersionRef
}

// UpdateFingerprint builds the M-30 fingerprint: the request identity (the
// correlation ID) + the target version reference.
func UpdateFingerprint(correlationID, targetVersionRef string) string {
	return correlationID + keySeparator + targetVersionRef
}

// RemoveKey builds the M-31 remove class's effective key: the client key
// scoped workspace + plugin identity Core-side.
func RemoveKey(clientKey, pluginID string) string {
	return clientKey + keySeparator + "plugin.remove:" + pluginID
}

// RevokeKey builds the G7 revoke-grants class's effective key: the client
// key scoped workspace + plugin identity + the grant-scope component
// Core-side (the grant identity is Core-owned; the grant-scope class is
// UNSUPPLIED [B5] — this Core's grant scope is the plugin's granted
// capability set of record).
func RevokeKey(clientKey, pluginID string) string {
	return clientKey + keySeparator + "plugin.revoke:" + pluginID
}

// FingerprintRef extracts the reference half of a recorded install/update
// fingerprint (the replay ruling's comparison half).
func FingerprintRef(fingerprint string) string {
	for i := len(fingerprint) - 1; i >= 0; i-- {
		if fingerprint[i] == keySeparator[0] {
			return fingerprint[i+len(keySeparator):]
		}
	}
	return ""
}

// InstallIntent is the C1-07 install class's durable intent: the
// Core-minted identities (the plugin, the package, the install-record —
// the identity law), the operation record's identity members, the
// effective (workspace + package-scoped) key, and the fingerprint (the
// correlation ID + the package artifact/version reference).
type InstallIntent struct {
	Plugin         Plugin
	VersionRef     string // the package artifact/version reference of record (the request's package_ref; the reconciliation key)
	OperationID    string
	IdempotencyKey string // the effective key (InstallKey)
	Fingerprint    string // InstallFingerprint(correlation, packageRef)
}

// InstallOutcome is one install call's store-confirmed outcome: the
// Core-confirmed install-record reference (the recorded one on a replay —
// never a second one) and the replay datum.
type InstallOutcome struct {
	InstallRecordRef string
	OperationID      string
	Replayed         bool
}

// ToggleIntent is the M-29 toggle class's durable intent (the
// Core-recorded state-checked mutation — the published body carries no
// keyed members; the Core mints the correlation per toggle).
type ToggleIntent struct {
	PluginID      string
	Enabled       bool
	OperationID   string
	CorrelationID string
}

// UpdateIntent is the M-30 update class's durable intent: the destination
// plugin, the target version reference (the fingerprint's second half and
// the reconciliation key), the Core-minted package identity for the
// target, the operation record's identity members, the effective key, and
// the fingerprint. TargetRequestedCapabilities carries the Core's
// requested-capability record for the target version — the fresh-review
// law's comparison input (the Core's requested-of-record; empty where the
// Core carries none — never a fabricated set).
type UpdateIntent struct {
	PluginID                    string
	TargetVersionRef            string
	PackageRef                  string // the Core-minted package identity for the target
	TargetRequestedCapabilities []string
	OperationID                 string
	IdempotencyKey              string // the effective key (UpdateKey)
	Fingerprint                 string // UpdateFingerprint(correlation, targetRef)
	CorrelationID               string
}

// MutationIntent is the keyed mutation class's durable intent (the remove
// M-31 and revoke-grants G7 classes): the destination plugin, the
// operation record's identity members, the effective key (the class's
// own scope), and the fingerprint (the correlation ID).
type MutationIntent struct {
	PluginID       string
	Kind           string // the store's own identity discriminator (remove | revoke_grants)
	OperationID    string
	IdempotencyKey string // the effective key (RemoveKey | RevokeKey)
	Fingerprint    string // the request identity (the correlation ID)
	CorrelationID  string
}

// MutationOutcome is one mutation call's store-confirmed outcome: the
// Core-confirmed result (the recorded one on a replay — never a second
// mutation).
type MutationOutcome struct {
	PluginID     string
	OperationRef string
	Replayed     bool
}

// ListQuery is one bounded keyset page request (created_at DESC,
// plugin_id DESC — the newest first; the keyset position rides the opaque
// cursor).
type ListQuery struct {
	Limit          int
	AfterCreatedAt string
	AfterID        string
}

// PluginPage is one bounded page. More marks a following page (the
// cursor's mint is the app layer's; the cursor's absence on the wire
// marks the final page).
type PluginPage struct {
	Plugins []Plugin
	More    bool
}

// FreshReviewDelta computes the fresh-review law's changed set (binding,
// carried): the target version's requested capabilities that are not in
// the plugin's granted set of record — ANY permission change triggers a
// FRESH review of the changed set, and the update does not complete past
// an un-reviewed change. The granted set of record is the Core's own
// (the never-grant law: only the user's explicit act moves it; on this
// Core no grant producer exists, so the granted set of record is empty
// and no change is detectable — the Core carries no package-content
// examination; recorded honestly as the trigger's no-producer gap). The
// result is sorted and deduplicated (references only — capability NAMES,
// never secret values).
func FreshReviewDelta(granted, requested []string) []string {
	grantedSet := make(map[string]bool, len(granted))
	for _, g := range granted {
		grantedSet[g] = true
	}
	seen := make(map[string]bool, len(requested))
	delta := []string{}
	for _, r := range requested {
		if grantedSet[r] || seen[r] {
			continue
		}
		seen[r] = true
		delta = append(delta, r)
	}
	sort.Strings(delta)
	return delta
}
