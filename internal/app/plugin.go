package app

import (
	stdcontext "context"
	"encoding/json"
	"fmt"

	"github.com/qira-dev/karyora/internal/domain/errs"
	plugindomain "github.com/qira-dev/karyora/internal/domain/plugin"
	"github.com/qira-dev/karyora/internal/security"
)

// The B5 Plugin lifecycle family service (DE-0 queue row 8): the published
// /plugins route family's use cases, implemented against the canonical
// contract at the working HEAD — the C1-07 install class (the Core mints
// the identities; the body never carries one; the 72 h post-terminal keyed
// replay; the install lands the plugin in its installed NOT-yet-enabled
// state; the admission-pipeline STRUCTURE + verdict law + audit-receipt law
// carried by the published descriptions and honored here: every admission
// verdict is the CORE's output and the audit receipt rides the Core's audit
// layer — the F12 path with the plugin_id context field — never a UI-side
// journal), the M-29 state-toggle member (ONE member covering both
// directions via the body's target state; the Core-recorded state-checked
// mutation — the published body carries no keyed members), the M-30 update
// member (the fresh-review law; the atomic upgrade), the M-31 remove
// member (the remove-vs-revoke question RECORDED OPEN — no text asserts
// they are the same or distinct), the G7 revoke-grants member (revocation
// blocks FUTURE invocations; the current invocation's behavior
// Core-reported), the keyset lifecycle list, the inspect member, and the
// family's authoritative state endpoint (a pure query — never a retry; the
// S0–S4 reconcile anchor).
//
// THE GATED EXECUTION ROWS (carried exactly, binding): THE
// INVOCATION/EXECUTION OPERATION DOES NOT EXIST — no invocation operation,
// no invocation/in-flight state schema, no invocation-status read, no
// invocation events, no quota-value reporting, no steering/stop controls.
// GATED [D-13/D-14 + the Core MB §10.2 runtime proof]. THE NEVER-GRANT LAW
// (binding): the plugin itself can NEVER grant permissions — no operation
// on this surface grants a capability; the granted set of record only ever
// shrinks or stays; the user grants/denies; the Core mediates and
// enforces. THE DISTINCTNESS LAWS (binding): DISABLED ≠ GRANTS-REVOKED ≠
// TRUST-REVOKED — the toggle operates on its own state class only; a
// revoked state is never exitable through a user toggle. THE DUTY SPLIT
// (binding): REDACTION IS THE CORE'S DUTY — no secret value, raw log,
// runtime internal, stack trace, memory content, raw plugin output, or
// provider payload enters any record on this surface; references only.
// THE S0–S4 LAW (binding): a lost response reconciles by the authoritative
// plugin/package identity through the family's state read BEFORE any
// retry — never by timeout alone. Every protected effect rides the single
// decision port (security.ResourcePlugin — the additive register
// extension; Local mode stays role-free: no permission policy is
// invented).

// PluginStore is the durable Plugin lifecycle surface port (B5). The
// storage adapter implements it over the plugin tables (migration 0017);
// the slice consumes only this port. Nil disables the surface — typed
// refusals, fail closed (the KnowledgeStore/MemoryStore/ContextStore/
// WorkflowStore pattern). THE GATED EXECUTION ROWS: the port carries NO
// invocation member — nothing here enables any execution.
type PluginStore interface {
	// SubmitPluginInstall applies the C1-07 install class's ruling (72 h
	// post-terminal; a duplicate replay returns the SAME install-record
	// reference — never a second one; the install lands the plugin in its
	// installed NOT-yet-enabled state).
	SubmitPluginInstall(ctx stdcontext.Context, intent plugindomain.InstallIntent) (plugindomain.InstallOutcome, error)
	// SubmitPluginToggle applies the M-29 state-toggle class (the
	// Core-recorded state-checked mutation; the revoked states are not
	// toggle-targetable — the distinctness laws' consequence).
	SubmitPluginToggle(ctx stdcontext.Context, intent plugindomain.ToggleIntent) (plugindomain.MutationOutcome, error)
	// SubmitPluginUpdate applies the M-30 update class's ruling (1 h
	// post-terminal; the fresh-review law gates completion past an
	// un-reviewed change; the atomic upgrade).
	SubmitPluginUpdate(ctx stdcontext.Context, intent plugindomain.UpdateIntent) (plugindomain.MutationOutcome, error)
	// SubmitPluginRemove applies the M-31 remove class's ruling (1 h
	// post-terminal; Core-confirmed removal; the remove-vs-revoke question
	// stays OPEN).
	SubmitPluginRemove(ctx stdcontext.Context, intent plugindomain.MutationIntent) (plugindomain.MutationOutcome, error)
	// SubmitPluginRevokeGrants applies the G7 revoke-grants class's ruling
	// (1 h post-terminal; revocation blocks FUTURE invocations; the
	// current invocation's behavior Core-reported).
	SubmitPluginRevokeGrants(ctx stdcontext.Context, intent plugindomain.MutationIntent) (plugindomain.MutationOutcome, error)
	// LoadPlugin resolves one plugin's authoritative record; the unknown
	// identity and the out-of-scope identity are the SAME typed 404 (the
	// no-leak rule).
	LoadPlugin(ctx stdcontext.Context, pluginID string) (plugindomain.Plugin, error)
	// ListPlugins reads one bounded keyset page (the cursor's absence
	// marks the final page).
	ListPlugins(ctx stdcontext.Context, q plugindomain.ListQuery) (plugindomain.PluginPage, error)
	// LoadPluginState resolves the family's authoritative state read (a
	// pure query — never a retry).
	LoadPluginState(ctx stdcontext.Context, pluginID string) (plugindomain.State, error)
}

// Plugin bounds (the coded bounds class: identifiers ride the 128-char
// bound — bounded, never unbounded).
const maxPluginIDLen = 128

// PluginInstallRequest is one install body (the published
// PluginInstallRequest members as the service contract).
type PluginInstallRequest struct {
	IdempotencyKey string
	CorrelationID  string
	PackageRef     string
}

// PluginUpdateRequest is one update body (the published
// PluginUpdateRequest members as the service contract).
type PluginUpdateRequest struct {
	IdempotencyKey   string
	CorrelationID    string
	TargetVersionRef string
}

// PluginMutationRequestBody is one keyed mutation body (the published
// PluginMutationRequest members as the service contract — the remove and
// revoke-grants members).
type PluginMutationRequestBody struct {
	IdempotencyKey string
	CorrelationID  string
}

// PluginEnableRequestBody is one state-toggle body (the published
// PluginEnableRequest member as the service contract — the target state
// only).
type PluginEnableRequestBody struct {
	Enabled bool
}

// PluginListRequest is one bounded list request.
type PluginListRequest struct {
	Limit  int
	Cursor string
}

// PluginListResult is one bounded page (the cursor's absence marks the
// final page).
type PluginListResult struct {
	Plugins []plugindomain.Plugin
	Cursor  string
}

// pluginNilPort is the typed refusal for a nil store (fail closed).
func pluginNilPort(corr string) error {
	return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "plugin store is not wired"})
}

// validatePluginIdentity validates one bounded identifier member.
func validatePluginIdentity(value, field string, corr string) error {
	if value == "" || len(value) > maxPluginIDLen {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": field})
	}
	return nil
}

// InstallPlugin is the C1-07 install member (the Core mints the plugin,
// package, install-record, and grant identities — the body never carries
// one). The effective key is workspace + package-scoped; the fingerprint
// is the request identity (the correlation ID) + the package
// artifact/version reference; the replay window is 72 h post-terminal. A
// Core-confirmed install lands the plugin in its installed NOT-yet-enabled
// state with an EMPTY granted set of record (the never-grant law). THE
// ADMISSION PIPELINE (the STRUCTURE carried by the published description):
// the admission verdict is the CORE's output — the Core-confirmed install
// is the verdict's record; the Core-returned verification states ride the
// package record exactly as the Core returns them (each absent where the
// Core returns none — never fabricated; on this Core no verification
// machinery exists while the MB §10.2 runtime proof gate stands, so the
// verification members render absent and no trust assertion is ever
// minted). THE AUDIT-RECEIPT LAW: the audit receipt rides the Core's
// audit layer — never a parallel journal (this family mints none). A lost
// or unknown outcome reconciles by the authoritative package ID/version
// through the family's state read BEFORE any retry (the S0–S4 law) —
// never by timeout alone, never an optimistic "Installed".
func (s *Slice) InstallPlugin(ctx stdcontext.Context, req PluginInstallRequest) (plugindomain.InstallResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.InstallResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin}, security.ActionDataWrite); err != nil {
		return plugindomain.InstallResult{}, err
	}
	if err := validatePluginIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return plugindomain.InstallResult{}, err
	}
	if err := validatePluginIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return plugindomain.InstallResult{}, err
	}
	if err := validatePluginIdentity(req.PackageRef, "package_ref", corr); err != nil {
		return plugindomain.InstallResult{}, err
	}
	// THE CORE-MINTS-THE-IDENTITY LAW: the identities are minted HERE (the
	// Core) — the body never carries one. THE NEVER-GRANT LAW: the granted
	// set of record starts empty and no operation on this surface grants.
	intent := plugindomain.InstallIntent{
		Plugin: plugindomain.Plugin{
			PluginID: "plug-" + randomID(),
			Package:  &plugindomain.Package{PackageRef: "pkg-" + randomID()},
			// The installed NOT-yet-enabled state — the lifecycle state the
			// Core-confirmed install lands in (activation is separate).
			LifecycleState:   plugindomain.StateInstalled,
			InstallRecordRef: "plugrec-" + randomID(),
		},
		VersionRef:     req.PackageRef,
		OperationID:    "plugop-" + randomID(),
		IdempotencyKey: plugindomain.InstallKey(req.IdempotencyKey, req.PackageRef),
		Fingerprint:    plugindomain.InstallFingerprint(req.CorrelationID, req.PackageRef),
	}
	outcome, err := s.deps.Plugin.SubmitPluginInstall(ctx, intent)
	if err != nil {
		return plugindomain.InstallResult{}, err
	}
	return plugindomain.InstallResult{InstallRecordRef: outcome.InstallRecordRef, Replayed: outcome.Replayed}, nil
}

// SetPluginEnabled is the M-29 state-toggle member — ONE member operation
// covering both directions via the body's target state (the published pin
// precedent's shape). THE DISTINCTNESS LAW (binding): DISABLED (the user's
// action) ≠ GRANTS-REVOKED ≠ TRUST-REVOKED — the toggle is its own state
// class; a revoked state is never exitable through the toggle (the typed
// refusals ride the published register codes the states carry). Whether a
// disable→enable cycle re-opens the permission review is UNSUPPLIED [B5] —
// recorded Open; this member decides no review question. THE CARRIER GAP
// (recorded honestly): the published body carries exactly the target
// state — the keyed-class members its description references are
// UNSUPPLIED, so the served toggle is the Core-recorded state-checked
// mutation (each toggle its own Core-confirmed record; the reconcile law
// is the state query the description itself carries). A lost or unknown
// outcome reconciles by querying the plugin's authoritative state BEFORE
// any toggle retry (the S0–S4 law) — no local flip, never an optimistic
// state change.
func (s *Slice) SetPluginEnabled(ctx stdcontext.Context, pluginID string, req PluginEnableRequestBody) (plugindomain.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.MutationResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataWrite); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	outcome, err := s.deps.Plugin.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{
		PluginID:      pluginID,
		Enabled:       req.Enabled,
		OperationID:   "plugop-" + randomID(),
		CorrelationID: corr,
	})
	if err != nil {
		return plugindomain.MutationResult{}, err
	}
	return plugindomain.MutationResult{PluginID: outcome.PluginID, OperationRef: outcome.OperationRef, Replayed: outcome.Replayed}, nil
}

// UpdatePlugin is the M-30 update member. THE FRESH-REVIEW LAW (binding):
// ANY permission change triggers a FRESH review of the changed set — the
// update does not complete past an un-reviewed change; the changed set is
// the target's requested capabilities not in the granted set of record
// (the Core's own comparison — the typed PLUGIN_CAPABILITY_DENIED refusal
// stores nothing and consumes no key; the retry never grants; the user's
// explicit act governs). On this Core the target's requested-of-record is
// the Core's EMPTY record (no package-content examination exists — the
// package format is UNSUPPLIED [B5] and no manifest bytes ride any
// published member), so the changed-set trigger has no producer on this
// surface; the check is implemented structurally and its typed refusal is
// pinned at the ruling (recorded honestly as the trigger's no-producer
// gap — never a fabricated finding). THE ATOMIC UPGRADE (binding; Core
// MB §10.3): the switch is the single-writer transaction — the prior
// record intact until the Core-confirmed switch commits (no artifact
// overwrite while an old active plugin runs). Update preserves the prior
// version/permissions summary: the lifecycle state, the granted set, and
// the install-record reference are preserved. A lost or unknown outcome
// reconciles by the authoritative package ID/version through the family's
// state read BEFORE any retry (the S0–S4 law) — never a blind retry,
// never an optimistic "Updated".
func (s *Slice) UpdatePlugin(ctx stdcontext.Context, pluginID string, req PluginUpdateRequest) (plugindomain.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.MutationResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataWrite); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.TargetVersionRef, "target_version_ref", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	intent := plugindomain.UpdateIntent{
		PluginID:         pluginID,
		TargetVersionRef: req.TargetVersionRef,
		PackageRef:       "pkg-" + randomID(),
		// THE CORE'S REQUESTED-OF-RECORD for the target version: the Core
		// carries no package-content examination on this surface (the
		// package format is UNSUPPLIED [B5]; no manifest bytes ride any
		// published member) — the record is EMPTY, never a fabricated set
		// (the no-fabricated-finding discipline).
		TargetRequestedCapabilities: []string{},
		OperationID:                 "plugop-" + randomID(),
		IdempotencyKey:              plugindomain.UpdateKey(req.IdempotencyKey, req.TargetVersionRef),
		Fingerprint:                 plugindomain.UpdateFingerprint(req.CorrelationID, req.TargetVersionRef),
		CorrelationID:               req.CorrelationID,
	}
	outcome, err := s.deps.Plugin.SubmitPluginUpdate(ctx, intent)
	if err != nil {
		return plugindomain.MutationResult{}, err
	}
	return plugindomain.MutationResult{PluginID: outcome.PluginID, OperationRef: outcome.OperationRef, Replayed: outcome.Replayed}, nil
}

// RemovePlugin is the M-31 remove member: Core-confirmed removal; disposal
// follows the Core's retention — no erasure claim beyond the
// Core-confirmed scope (the deletion-honesty class). THE REMOVE-VS-REVOKE
// QUESTION RECORDED OPEN: no text asserts removal and grant revocation are
// the same or distinct — this member implements its own published behavior
// and asserts neither. A lost or unknown outcome reconciles by the
// authoritative package/plugin identity through the family's state read
// BEFORE any retry (the S0–S4 law) — removal is never assumed complete.
func (s *Slice) RemovePlugin(ctx stdcontext.Context, pluginID string, req PluginMutationRequestBody) (plugindomain.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.MutationResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataWrite); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	outcome, err := s.deps.Plugin.SubmitPluginRemove(ctx, plugindomain.MutationIntent{
		PluginID:       pluginID,
		OperationID:    "plugop-" + randomID(),
		IdempotencyKey: plugindomain.RemoveKey(req.IdempotencyKey, pluginID),
		Fingerprint:    req.CorrelationID,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		return plugindomain.MutationResult{}, err
	}
	return plugindomain.MutationResult{PluginID: outcome.PluginID, OperationRef: outcome.OperationRef, Replayed: outcome.Replayed}, nil
}

// RevokePluginGrants is the G7 revoke-grants member: revocation blocks
// FUTURE invocations; the current invocation's behavior is
// Core-reported — never guessed (on this Core no invocation path exists
// while the execution rows are gated, so there is no current invocation
// to report — the honest absence, never a fabricated one). THE
// DISTINCTNESS LAW (binding): GRANTS-REVOKED is DISTINCT from DISABLED and
// from TRUST-REVOKED. THE NEVER-GRANT LAW (binding): a retry never
// re-grants and a denial stands until the user's own explicit act. A lost
// or unknown outcome reconciles by the authoritative grant/operation
// reference through the family's state read BEFORE any retry (the S0–S4
// law) — no assumption about the grant state.
func (s *Slice) RevokePluginGrants(ctx stdcontext.Context, pluginID string, req PluginMutationRequestBody) (plugindomain.MutationResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.MutationResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataWrite); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.IdempotencyKey, "idempotency_key", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	if err := validatePluginIdentity(req.CorrelationID, "correlation_id", corr); err != nil {
		return plugindomain.MutationResult{}, err
	}
	outcome, err := s.deps.Plugin.SubmitPluginRevokeGrants(ctx, plugindomain.MutationIntent{
		PluginID:       pluginID,
		OperationID:    "plugop-" + randomID(),
		IdempotencyKey: plugindomain.RevokeKey(req.IdempotencyKey, pluginID),
		Fingerprint:    req.CorrelationID,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		return plugindomain.MutationResult{}, err
	}
	return plugindomain.MutationResult{PluginID: outcome.PluginID, OperationRef: outcome.OperationRef, Replayed: outcome.Replayed}, nil
}

// Plugins is the keyset list read (the M-1 correlation-only read class;
// safe to re-issue; the cursor's absence marks the final page). The list
// IS the user-installed list — USER-INSTALLED ≠ CATALOG-LISTED, always;
// a catalog exists ONLY when the Core supplies one (none is published).
func (s *Slice) Plugins(ctx stdcontext.Context, req PluginListRequest) (PluginListResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return PluginListResult{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin}, security.ActionDataRead); err != nil {
		return PluginListResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return PluginListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	q := plugindomain.ListQuery{Limit: limit}
	if req.Cursor != "" {
		pos, err := decodePluginCursor(req.Cursor)
		if err != nil {
			return PluginListResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterID = pos.CreatedAt, pos.ID
	}
	page, err := s.deps.Plugin.ListPlugins(ctx, q)
	if err != nil {
		return PluginListResult{}, err
	}
	res := PluginListResult{Plugins: page.Plugins}
	if page.More {
		if len(page.Plugins) == 0 {
			return PluginListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodePluginCursor(page.Plugins[len(page.Plugins)-1])
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return PluginListResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.Cursor = next
	}
	return res, nil
}

// PluginInspect is the inspect member: a pure read; no state change.
// Renders ONLY Core-returned verification states (the artifact digest, the
// signature verification result, the trust root, and the rotation/
// revocation status are each Core-returned — never computed) and ONLY
// Core-returned, PROVEN limits where any limit is shown (the PROVEN-only
// law: no computed, assumed, or documented-in-theory budget;
// per-instruction CPU/fuel quota must NOT be assumed; wall-clock timeout
// is NOT fuel — the wazero caveat carried). The unknown identity and the
// out-of-scope identity are the SAME typed 404 (the no-leak rule).
func (s *Slice) PluginInspect(ctx stdcontext.Context, pluginID string) (plugindomain.Plugin, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.Plugin{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataRead); err != nil {
		return plugindomain.Plugin{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.Plugin{}, err
	}
	return s.deps.Plugin.LoadPlugin(ctx, pluginID)
}

// PluginState is the family's authoritative state endpoint (a pure query
// — never a retry): resolved by (workspace, plugin/package identity). A
// timeout or lost response is NEVER proof of success, cancellation, or
// safe retry (the S0–S4 model) — reconcile by the authoritative
// plugin/package identity through this pure query BEFORE any retry; the
// family's reconciliation keys resolve here (install and update by
// package ID/version; remove by the package/plugin identity; enable by
// the plugin state; revoke by the grant/operation reference). Events are
// never the source of truth; this endpoint is. The unknown identity and
// the out-of-scope identity are the SAME typed 404 (the no-leak rule).
func (s *Slice) PluginState(ctx stdcontext.Context, pluginID string) (plugindomain.State, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Plugin == nil {
		return plugindomain.State{}, pluginNilPort(corr)
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourcePlugin, ID: pluginID}, security.ActionDataRead); err != nil {
		return plugindomain.State{}, err
	}
	if err := validatePluginIdentity(pluginID, "plugin_id", corr); err != nil {
		return plugindomain.State{}, err
	}
	return s.deps.Plugin.LoadPluginState(ctx, pluginID)
}

// pluginCursorPosition is the decoded form behind the opaque list
// cursor: the keyset position (the last page's sort-key values). It
// follows the shared salted-cursor PATTERN — a salt-prefixed hex-JSON
// envelope, never structured data on the wire.
type pluginCursorPosition struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodePluginCursor(rec plugindomain.Plugin) (string, error) {
	if rec.PluginID == "" || rec.InstalledAt == "" {
		return "", fmt.Errorf("app: plugin list cursor position lacks its keyset values")
	}
	b, err := json.Marshal(pluginCursorPosition{CreatedAt: rec.InstalledAt, ID: rec.PluginID})
	if err != nil {
		return "", err
	}
	return saltedCursorEnvelope(b)
}

func decodePluginCursor(cursor string) (pluginCursorPosition, error) {
	var pos pluginCursorPosition
	b, err := decodeSaltedCursorEnvelope(cursor)
	if err != nil {
		return pos, err
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: plugin cursor is malformed")
	}
	if pos.ID == "" || pos.CreatedAt == "" {
		return pos, fmt.Errorf("app: plugin cursor is malformed")
	}
	return pos, nil
}
