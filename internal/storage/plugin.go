package storage

import (
	stdcontext "context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
	plugindomain "github.com/qira-dev/karyora/internal/domain/plugin"
	"github.com/qira-dev/karyora/internal/events"
)

// The Store implements the B5 Plugin lifecycle family's durable surface
// (DE-0 queue row 8): the C1-07 install class's keyed ruling (72 h
// post-terminal; a duplicate replay returns the SAME install-record
// reference — never a second one; the install lands the plugin in its
// installed NOT-yet-enabled state), the M-30 update class's keyed ruling
// (1 h post-terminal; the fresh-review law gates completion past an
// un-reviewed change; the atomic upgrade is the single-writer transaction
// — the prior record intact until the Core-confirmed switch commits), the
// M-31 remove and G7 revoke-grants keyed rulings (1 h post-terminal), and
// the M-29 state-toggle (the Core-recorded state-checked mutation — the
// published body carries no keyed members; each toggle its own
// Core-confirmed record). THE GATED EXECUTION ROWS (carried exactly): the
// invocation/execution operation DOES NOT EXIST — no invocation member
// exists on this store, no invocation table exists in this family's
// schema, no invocation event is ever appended; D-13/D-14 and the Core MB
// §10.2 runtime proof keep their recorded states. THE NEVER-GRANT LAW
// (binding): no ruling here grants a capability — the granted set of
// record only ever shrinks (the revoke) or stays; install lands it empty.
// THE DISTINCTNESS LAWS (binding): the toggle operates on the
// installed/enabled/disabled class only — a revoked state is never
// exitable through the toggle; every state value is distinct. THE
// REDACTION LAW: event payloads carry references only — never capability
// grant values, never secret values, never plugin output. Events are
// never the source of truth — the authoritative reads are.

// PluginInstallReplayWindow is the C1-07 install class's post-terminal
// replay window: 72 h (the keyed creation-class precedent selected at the
// CF-5 publication act — the shared recorded IdempotencyReplayWindow
// constant).
const PluginInstallReplayWindow = IdempotencyReplayWindow

// PluginShortReplayWindow is the short post-terminal replay window: 1 h
// (the M-30 update, M-31 remove, and G7 revoke classes — the
// short-window M-16-class precedent selected at the CF-5 publication act).
const PluginShortReplayWindow = time.Hour

// PluginIdempotencyConflictError carries the keyed classes'
// same-key-different-identity conflict's envelope: the published
// CONFLICT_IDEMPOTENCY_KEY with its fingerprintMatch:false datum (the
// same typed shape the workflow/memory/knowledge conflicts ride).
type PluginIdempotencyConflictError struct{ Detail string }

func (e *PluginIdempotencyConflictError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", errs.CodeConflictIdempotencyKey, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *PluginIdempotencyConflictError) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeConflictIdempotencyKey, correlationID, map[string]any{
		"keyScope":         "plugin",
		"fingerprintMatch": false,
		"reason":           e.Detail,
	})
}

// PluginRevokedStateError carries the revoked-state refusals' envelopes:
// the toggle/withdrawal attempts against a revoked state ride the
// published register code the state carries (the distinctness laws'
// consequence — a revoked state is never exitable through a user toggle,
// and the trust-revoked state admits no lifecycle control; the re-grant
// is the user's explicit act through the Core's defined path and
// re-admission is the Core policy's decision alone).
type PluginRevokedStateError struct {
	Code   errs.Code
	Detail string
}

func (e *PluginRevokedStateError) Error() string {
	return fmt.Sprintf("storage: %s (%s)", e.Code, e.Detail)
}

// Envelope implements the wire-envelope carrier.
func (e *PluginRevokedStateError) Envelope(correlationID string) (errs.Envelope, error) {
	data := map[string]any{"reason": e.Detail}
	if e.Code == errs.CodePluginCapabilityDenied {
		data["precondition"] = "PERMISSION-REVIEW"
	}
	return errs.New(e.Code, correlationID, data)
}

// errPluginDuplicateKey marks the in-transaction same-key insert race
// that resolves to the replay ruling (the SubmitJob duplicate idiom).
var errPluginDuplicateKey = errors.New("storage: duplicate plugin idempotency key")

// The keyed classes' durable operation-record kinds — this store's own
// identity discriminator (no operation-kind schema is published by the
// family; the keyed rulings' records are the store's own identity
// members). The enable toggle is the UNKEYED record kind (the published
// body carries no keyed members — the carrier gap recorded honestly).
const (
	pluginKindInstall      = "install"
	pluginKindToggle       = "toggle"
	pluginKindUpdate       = "update"
	pluginKindRemove       = "remove"
	pluginKindRevokeGrants = "revoke_grants"
)

// pluginOperationRecord is the store's own durable operation-record shape
// for the keyed classes.
type pluginOperationRecord struct {
	OperationID   string
	Kind          string
	CorrelationID string
	PluginID      string
	ResultRef     string // the recorded outcome's reference (the install-record reference for install)
	CreatedAt     string
	UpdatedAt     string
}

// pluginRow is the store's own row shape behind the plugins table (the
// Core-returned record members; the package_json column carries the
// Core-returned verification states — absent where the Core returns
// none, never fabricated).
type pluginRow struct {
	PluginID         string
	LifecycleState   string
	PackageRef       string
	VersionRef       string
	InstallRecordRef string
	PackageJSON      string
	GrantedJSON      string
	InstalledAt      string
}

func (r pluginRow) grantedCapabilities() []string {
	var granted []string
	if r.GrantedJSON != "" {
		if err := json.Unmarshal([]byte(r.GrantedJSON), &granted); err != nil || granted == nil {
			granted = []string{}
		}
	} else {
		granted = []string{}
	}
	return granted
}

func (r pluginRow) domain() plugindomain.Plugin {
	rec := plugindomain.Plugin{
		PluginID:         r.PluginID,
		LifecycleState:   r.LifecycleState,
		InstallRecordRef: r.InstallRecordRef,
		InstalledAt:      r.InstalledAt,
	}
	if r.PackageJSON != "" {
		var pkg plugindomain.Package
		if err := json.Unmarshal([]byte(r.PackageJSON), &pkg); err == nil {
			rec.Package = &pkg
		}
	}
	if rec.Package == nil {
		// The Core-returned package record: the Core-minted package
		// identity is the required member; the verification states are
		// ABSENT where the Core returns none (never fabricated).
		rec.Package = &plugindomain.Package{PackageRef: r.PackageRef}
	}
	return rec
}

// SubmitPluginInstall applies the C1-07 install class's ruling and stores
// the fresh Core-minted plugin with its operation record and the
// plugin.install_completed event when no replay resolves. THE IDENTITY
// LAW: the plugin, package, and install-record identities ride the intent
// (minted by the app layer) — the body never carries one. THE
// INSTALLED-NOT-ENABLED LAW: the install lands the plugin in its
// installed, NOT-yet-enabled state (activation is separate). THE
// NEVER-GRANT LAW: the granted set of record starts EMPTY (no operation
// on this surface grants a capability). The effective key is
// workspace + package-scoped (InstallKey); the fingerprint is the
// correlation ID + the package artifact/version reference; the same key +
// the same fingerprint replays the recorded install while the operation
// is live or within the 72 h post-terminal window (the SAME
// install-record reference, never a second one); the same key with a
// different fingerprint is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false — the C1-07 class: same key + different body);
// a replay past the window is the typed CONFLICT_STATE.
func (s *Store) SubmitPluginInstall(ctx stdcontext.Context, intent plugindomain.InstallIntent) (plugindomain.InstallOutcome, error) {
	rec := intent.Plugin
	if rec.PluginID == "" || rec.Package == nil || rec.Package.PackageRef == "" ||
		rec.InstallRecordRef == "" || intent.VersionRef == "" ||
		intent.OperationID == "" || intent.IdempotencyKey == "" || intent.Fingerprint == "" {
		return plugindomain.InstallOutcome{}, errStoragef(errs.CodeValidationFailed, "plugin install intent lacks its identity members")
	}
	// The C1-07 ruling: one operation per effective key.
	if prior, err := s.pluginOpByKey(ctx, intent.IdempotencyKey); err != nil {
		return plugindomain.InstallOutcome{}, err
	} else if prior != nil {
		return s.resolvePluginInstallReplay(prior, intent)
	}
	err := s.CommitStateChange(ctx, intent.Fingerprint, func(tx *sqlTx) error {
		if err := insertPlugin(tx, rec, intent.VersionRef, s.now()); err != nil {
			return err
		}
		if err := insertPluginOperation(tx, pluginOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          pluginKindInstall,
			CorrelationID: intent.Fingerprint,
			PluginID:      rec.PluginID,
			ResultRef:     rec.InstallRecordRef,
		}, intent.IdempotencyKey, intent.Fingerprint, s.now()); err != nil {
			return err
		}
		// The register's plugin.install_completed event: the Core-confirmed
		// install (the payload carries references only — the redaction law).
		return appendEventTx(tx, pluginEvent(
			"evt-"+intent.OperationID,
			plugindomain.AggregatePlugin, rec.PluginID,
			plugindomain.EventInstallCompleted, intent.Fingerprint, s.instanceUUID,
			map[string]string{"plugin_id": rec.PluginID, "install_record_ref": rec.InstallRecordRef, "version_ref": intent.VersionRef}))
	})
	if err == errPluginDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		prior, lookupErr := s.pluginOpByKey(ctx, intent.IdempotencyKey)
		if lookupErr != nil || prior == nil {
			return plugindomain.InstallOutcome{}, err
		}
		return s.resolvePluginInstallReplay(prior, intent)
	}
	if err != nil {
		return plugindomain.InstallOutcome{}, err
	}
	return plugindomain.InstallOutcome{InstallRecordRef: rec.InstallRecordRef, OperationID: intent.OperationID, Replayed: false}, nil
}

// resolvePluginInstallReplay applies the C1-07 class exactly: the same
// key + the same fingerprint (the correlation ID + the package
// artifact/version reference) replays the recorded install within the 72 h
// post-terminal window — the SAME install-record reference, never a
// second one; the same key with a different fingerprint (either half) is
// the typed CONFLICT_IDEMPOTENCY_KEY (the C1-07 class: same key +
// different body); a replay past the window is the typed CONFLICT_STATE.
func (s *Store) resolvePluginInstallReplay(prior *pluginOperationRecord, intent plugindomain.InstallIntent) (plugindomain.InstallOutcome, error) {
	if prior.CorrelationID != intent.Fingerprint {
		return plugindomain.InstallOutcome{}, &PluginIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity; the install fingerprint does not match (recorded package reference %q)", plugindomain.FingerprintRef(prior.CorrelationID)),
		}
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return plugindomain.InstallOutcome{}, errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > PluginInstallReplayWindow {
		return plugindomain.InstallOutcome{}, errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", PluginInstallReplayWindow)
	}
	if prior.ResultRef == "" {
		return plugindomain.InstallOutcome{}, errStoragef(errs.CodeStateCorrupt, "recorded install operation %q carries no install-record reference", prior.OperationID)
	}
	return plugindomain.InstallOutcome{InstallRecordRef: prior.ResultRef, OperationID: prior.OperationID, Replayed: true}, nil
}

// SubmitPluginToggle applies the M-29 state-toggle class (the
// Core-recorded state-checked mutation — the published body carries no
// keyed members, so each toggle is its own Core-confirmed record with a
// Core-minted correlation; the reconcile law is the state query the
// member's description carries). ONE member operation covers both
// directions via the body's target state. THE DISTINCTNESS LAWS'
// CONSEQUENCE (binding): the toggle operates on the installed/enabled/
// disabled class only — a revoked state is NEVER exitable through the
// toggle (a user toggle never merges a revoked state into a user-action
// state): from the grants-revoked state the refusal rides the published
// PLUGIN_CAPABILITY_DENIED (the re-grant is the user's explicit act
// through the Core's defined path; a toggle never re-grants); from the
// trust-revoked state the refusal rides the published
// PLUGIN_TRUST_REVOKED (no lifecycle control; re-admission is the Core
// policy's decision alone). The unknown identity is the SAME typed 404
// (the no-leak rule).
func (s *Store) SubmitPluginToggle(ctx stdcontext.Context, intent plugindomain.ToggleIntent) (plugindomain.MutationOutcome, error) {
	if intent.PluginID == "" || intent.OperationID == "" || intent.CorrelationID == "" {
		return plugindomain.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "plugin toggle intent lacks its identity members")
	}
	var outcome plugindomain.MutationOutcome
	err := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
		rec, err := loadPluginRowTx(tx, intent.PluginID)
		if err != nil {
			// The destination identity resolves INSIDE the transaction:
			// the unknown plugin is the SAME typed 404 the inspect read
			// serves (the no-leak rule).
			return err
		}
		if !plugindomain.ToggleTargetable(rec.LifecycleState) {
			// The revoked states are not toggle-targetable (the
			// distinctness laws' consequence — nothing is written).
			return revokedStateRefusal(rec.LifecycleState,
				"the enabled/disabled toggle does not apply to a plugin in the %q state; the revoked state is distinct and is never exitable through a user toggle", rec.LifecycleState)
		}
		now := s.now()
		next := plugindomain.StateEnabled
		eventType := plugindomain.EventEnabled
		if !intent.Enabled {
			next = plugindomain.StateDisabled
			eventType = plugindomain.EventDisabled
		}
		if _, err := tx.Exec(`UPDATE plugins SET lifecycle_state = ?, updated_at = ? WHERE plugin_id = ?`,
			next, now.UTC().Format(time.RFC3339Nano), intent.PluginID); err != nil {
			return mapVendorError(err)
		}
		// The UNKEYED record kind: the published body carries no keyed
		// members (the carrier gap recorded honestly) — the operation
		// record persists with an empty key (never colliding) as the
		// Core-confirmed record of this toggle.
		if err := insertPluginOperation(tx, pluginOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          pluginKindToggle,
			CorrelationID: intent.CorrelationID,
			PluginID:      intent.PluginID,
		}, "", "", now); err != nil {
			return err
		}
		if err := appendEventTx(tx, pluginEvent(
			"evt-"+intent.OperationID,
			plugindomain.AggregatePlugin, intent.PluginID,
			eventType, intent.CorrelationID, s.instanceUUID,
			map[string]string{"plugin_id": intent.PluginID, "lifecycle_state": next})); err != nil {
			return err
		}
		outcome = plugindomain.MutationOutcome{PluginID: intent.PluginID, OperationRef: intent.OperationID, Replayed: false}
		return nil
	})
	if err != nil {
		return plugindomain.MutationOutcome{}, err
	}
	return outcome, nil
}

// SubmitPluginUpdate applies the M-30 update class's ruling: the atomic
// upgrade is the single-writer transaction — the prior record intact
// until the Core-confirmed switch commits (no artifact overwrite while an
// old active plugin runs; the prior version keeps running until the Core
// confirms the switch). THE FRESH-REVIEW LAW (binding): ANY permission
// change triggers a FRESH review of the changed set — the update does not
// complete past an un-reviewed change; the changed set is the target's
// requested capabilities not in the granted set of record, and the typed
// PLUGIN_CAPABILITY_DENIED refusal stores nothing and consumes no key.
// The effective key is workspace + package-scoped (UpdateKey); the
// fingerprint is the correlation ID + the target version reference; the
// same key + the same fingerprint replays the recorded update within the
// 1 h post-terminal window; the same key with a different correlation is
// the typed CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false); the same
// key recorded against a different target — or a replay past the window —
// is the typed CONFLICT_STATE. Update preserves the prior
// version/permissions summary: the lifecycle state, the granted set, and
// the install-record reference are preserved (only the package identity
// and the version-of-record switch).
func (s *Store) SubmitPluginUpdate(ctx stdcontext.Context, intent plugindomain.UpdateIntent) (plugindomain.MutationOutcome, error) {
	if intent.PluginID == "" || intent.TargetVersionRef == "" ||
		intent.PackageRef == "" || intent.OperationID == "" ||
		intent.IdempotencyKey == "" || intent.Fingerprint == "" || intent.CorrelationID == "" {
		return plugindomain.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "plugin update intent lacks its identity members")
	}
	var outcome plugindomain.MutationOutcome
	err := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
		rec, err := loadPluginRowTx(tx, intent.PluginID)
		if err != nil {
			// The destination identity resolves INSIDE the transaction:
			// the unknown plugin is the SAME typed 404 the inspect read
			// serves (the no-leak rule), and the key stays unconsumed by
			// an unresolvable intent (the memory-mutation precedent).
			return err
		}
		// The M-30 replay ruling precedes the fresh-review check so a lost
		// update response's keyed retry reconciles (the S0–S4 law) instead
		// of surfacing the refusal.
		prior, priorFingerprint, err := pluginOpByKeyTx(tx, intent.IdempotencyKey)
		if err != nil {
			return err
		}
		if prior != nil {
			return s.resolvePluginUpdateReplay(prior, priorFingerprint, intent, &outcome)
		}
		if rec.LifecycleState == plugindomain.StateTrustRevoked {
			return revokedStateRefusal(rec.LifecycleState,
				"the update does not apply to a plugin in the %q state; no lifecycle control applies and re-admission is the Core policy's decision alone", rec.LifecycleState)
		}
		// THE FRESH-REVIEW LAW: ANY permission change triggers a FRESH
		// review of the changed set — the update does not complete past an
		// un-reviewed change. The changed set is the target's requested
		// capabilities not in the granted set of record (the Core's own
		// comparison; the refusal is typed and stores nothing — the retry
		// never grants; the user's explicit act governs).
		if delta := plugindomain.FreshReviewDelta(rec.grantedCapabilities(), intent.TargetRequestedCapabilities); len(delta) > 0 {
			return &PluginRevokedStateError{
				Code: errs.CodePluginCapabilityDenied,
				Detail: fmt.Sprintf("the update carries un-reviewed permission changes (%s); the update does not complete past an un-reviewed change — the fresh review of the changed set governs and a retry never grants",
					strings.Join(delta, ", ")),
			}
		}
		now := s.now()
		if _, err := tx.Exec(`UPDATE plugins SET package_ref = ?, version_ref = ?, updated_at = ?
			WHERE plugin_id = ?`,
			intent.PackageRef, intent.TargetVersionRef, now.UTC().Format(time.RFC3339Nano), intent.PluginID); err != nil {
			return mapVendorError(err)
		}
		if err := insertPluginOperation(tx, pluginOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          pluginKindUpdate,
			CorrelationID: intent.CorrelationID,
			PluginID:      intent.PluginID,
		}, intent.IdempotencyKey, intent.Fingerprint, now); err != nil {
			return err
		}
		if err := appendEventTx(tx, pluginEvent(
			"evt-"+intent.OperationID,
			plugindomain.AggregatePlugin, intent.PluginID,
			plugindomain.EventUpdated, intent.CorrelationID, s.instanceUUID,
			map[string]string{"plugin_id": intent.PluginID, "version_ref": intent.TargetVersionRef})); err != nil {
			return err
		}
		outcome = plugindomain.MutationOutcome{PluginID: intent.PluginID, OperationRef: intent.OperationID, Replayed: false}
		return nil
	})
	if err == errPluginDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		var replay plugindomain.MutationOutcome
		rerr := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
			prior, priorFingerprint, lookupErr := pluginOpByKeyTx(tx, intent.IdempotencyKey)
			if lookupErr != nil || prior == nil {
				return err
			}
			return s.resolvePluginUpdateReplay(prior, priorFingerprint, intent, &replay)
		})
		if rerr != nil {
			return plugindomain.MutationOutcome{}, rerr
		}
		return replay, nil
	}
	if err != nil {
		return plugindomain.MutationOutcome{}, err
	}
	return outcome, nil
}

// resolvePluginUpdateReplay applies the M-30 replay law: the same key +
// the same correlation + the same target version reference replays the
// recorded update within the 1 h post-terminal window; the same key with
// a different correlation is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); the same key recorded against a different
// target — or a replay past the window — is the typed CONFLICT_STATE.
func (s *Store) resolvePluginUpdateReplay(prior *pluginOperationRecord, priorFingerprint string, intent plugindomain.UpdateIntent, outcome *plugindomain.MutationOutcome) error {
	if prior.CorrelationID != intent.CorrelationID {
		return &PluginIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity (recorded correlation %q); replay rejected", prior.CorrelationID),
		}
	}
	recordedTarget := plugindomain.FingerprintRef(priorFingerprint)
	if recordedTarget != intent.TargetVersionRef {
		return errStoragef(errs.CodeConflictState,
			"plugin update was recorded against a different target version reference (%q); replay rejected", recordedTarget)
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > PluginShortReplayWindow {
		return errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", PluginShortReplayWindow)
	}
	*outcome = plugindomain.MutationOutcome{PluginID: prior.PluginID, OperationRef: prior.OperationID, Replayed: true}
	return nil
}

// SubmitPluginRemove applies the M-31 remove class's ruling: Core-confirmed
// removal; disposal follows the Core's retention — no erasure claim beyond
// the Core-confirmed scope (the deletion-honesty class). The effective key
// is workspace + plugin-scoped (RemoveKey); the fingerprint is the
// correlation ID; the same key + the same correlation replays the recorded
// removal within the 1 h post-terminal window; the same key with a
// different correlation is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); a replay past the window is the typed
// CONFLICT_STATE. THE REMOVE-VS-REVOKE QUESTION STAYS OPEN (recorded, not
// decided here): this ruling implements the remove member's own published
// behavior — no text asserts removal and grant revocation are the same or
// distinct.
func (s *Store) SubmitPluginRemove(ctx stdcontext.Context, intent plugindomain.MutationIntent) (plugindomain.MutationOutcome, error) {
	return s.submitPluginKeyedMutation(ctx, intent, pluginKindRemove, plugindomain.EventRemoved, removePluginEffect)
}

// SubmitPluginRevokeGrants applies the G7 revoke-grants class's ruling:
// revocation blocks FUTURE invocations and the current invocation's
// behavior is Core-reported — never guessed (on this Core no invocation
// path exists while the execution rows are gated, so there is no current
// invocation to report — the honest absence, never a fabricated one). The
// effective key is workspace + plugin-scoped with the grant-scope
// component Core-side (RevokeKey); the fingerprint is the correlation ID;
// the replay law is the short-window keyed class. THE NEVER-GRANT LAW
// (binding): a retry never re-grants and a denial stands until the user's
// own explicit act.
func (s *Store) SubmitPluginRevokeGrants(ctx stdcontext.Context, intent plugindomain.MutationIntent) (plugindomain.MutationOutcome, error) {
	return s.submitPluginKeyedMutation(ctx, intent, pluginKindRevokeGrants, plugindomain.EventGrantsRevoked, revokePluginGrantsEffect)
}

// submitPluginKeyedMutation is the shared keyed-mutation ruling (the
// remove M-31 and revoke-grants G7 classes): the destination identity
// resolves INSIDE the transaction (the unknown plugin is the SAME typed
// 404 the inspect read serves — the no-leak rule; the key stays
// unconsumed by an unresolvable intent), the replay ruling precedes the
// state ruling (the S0–S4 law), the state refusal rides the published
// register codes, and the effect + the operation record + the register
// event commit in ONE transaction (the F11 law: only a confirmed outcome
// carries the truth).
func (s *Store) submitPluginKeyedMutation(ctx stdcontext.Context, intent plugindomain.MutationIntent, kind, eventType string, apply func(tx *sqlTx, pluginID string, now time.Time) error) (plugindomain.MutationOutcome, error) {
	if intent.PluginID == "" || intent.OperationID == "" ||
		intent.IdempotencyKey == "" || intent.Fingerprint == "" || intent.CorrelationID == "" {
		return plugindomain.MutationOutcome{}, errStoragef(errs.CodeValidationFailed, "plugin mutation intent lacks its identity members")
	}
	var outcome plugindomain.MutationOutcome
	err := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
		// THE KEYED REPLAY RULING PRECEDES THE STATE RULING (the S0–S4
		// law): a lost mutation response's keyed retry reconciles — for the
		// remove member this is load-bearing, because a confirmed removal
		// has already taken the destination out of the installed set, so
		// the replay resolves the RECORDED outcome (never a second removal,
		// never a destination 404). The effective key is
		// workspace + plugin-scoped, so a prior under this key is
		// necessarily this plugin's scope.
		prior, _, err := pluginOpByKeyTx(tx, intent.IdempotencyKey)
		if err != nil {
			return err
		}
		if prior != nil {
			return s.resolvePluginMutationReplay(prior, intent, &outcome)
		}
		rec, err := loadPluginRowTx(tx, intent.PluginID)
		if err != nil {
			// No recorded ruling and no destination: the unknown plugin is
			// the SAME typed 404 the inspect read serves (the no-leak rule),
			// and the key stays unconsumed by an unresolvable intent (the
			// memory-mutation precedent).
			return err
		}
		if rec.LifecycleState == plugindomain.StateTrustRevoked {
			return revokedStateRefusal(rec.LifecycleState,
				"the %s does not apply to a plugin in the %q state; no lifecycle control applies and re-admission is the Core policy's decision alone", kind, rec.LifecycleState)
		}
		now := s.now()
		if err := apply(tx, intent.PluginID, now); err != nil {
			return err
		}
		if err := insertPluginOperation(tx, pluginOperationRecord{
			OperationID:   intent.OperationID,
			Kind:          kind,
			CorrelationID: intent.CorrelationID,
			PluginID:      intent.PluginID,
		}, intent.IdempotencyKey, intent.Fingerprint, now); err != nil {
			return err
		}
		if err := appendEventTx(tx, pluginEvent(
			"evt-"+intent.OperationID,
			plugindomain.AggregatePlugin, intent.PluginID,
			eventType, intent.CorrelationID, s.instanceUUID,
			map[string]string{"plugin_id": intent.PluginID})); err != nil {
			return err
		}
		outcome = plugindomain.MutationOutcome{PluginID: intent.PluginID, OperationRef: intent.OperationID, Replayed: false}
		return nil
	})
	if err == errPluginDuplicateKey {
		// The concurrent writer stored the same key first; reload and let
		// the same ruling decide (the SubmitJob duplicate race idiom).
		var replay plugindomain.MutationOutcome
		rerr := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
			prior, _, lookupErr := pluginOpByKeyTx(tx, intent.IdempotencyKey)
			if lookupErr != nil || prior == nil {
				return err
			}
			return s.resolvePluginMutationReplay(prior, intent, &replay)
		})
		if rerr != nil {
			return plugindomain.MutationOutcome{}, rerr
		}
		return replay, nil
	}
	if err != nil {
		return plugindomain.MutationOutcome{}, err
	}
	return outcome, nil
}

// resolvePluginMutationReplay applies the keyed mutation replay law (the
// M-31/G7 classes): the same key + the same correlation replays the
// recorded mutation within the 1 h post-terminal window; the same key
// with a different correlation is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false); a replay past the window is the typed
// CONFLICT_STATE.
func (s *Store) resolvePluginMutationReplay(prior *pluginOperationRecord, intent plugindomain.MutationIntent, outcome *plugindomain.MutationOutcome) error {
	if prior.CorrelationID != intent.CorrelationID {
		return &PluginIdempotencyConflictError{
			Detail: fmt.Sprintf("idempotency key was already used with a different request identity (recorded correlation %q); replay rejected", prior.CorrelationID),
		}
	}
	terminalAt, perr := time.Parse(time.RFC3339Nano, prior.UpdatedAt)
	if perr != nil {
		return errStoragef(errs.CodeStateCorrupt, "recorded transition of operation %q does not parse", prior.OperationID)
	}
	if s.now().Sub(terminalAt) > PluginShortReplayWindow {
		return errStoragef(errs.CodeConflictState,
			"idempotency key replay window has elapsed (%s after the operation was recorded); replay rejected", PluginShortReplayWindow)
	}
	*outcome = plugindomain.MutationOutcome{PluginID: prior.PluginID, OperationRef: prior.OperationID, Replayed: true}
	return nil
}

// removePluginEffect applies the remove effect: the plugin record is
// removed from the installed set (the artifact is absent from the
// user-installed list per the Core; the deletion-honesty class — no
// erasure claim beyond the Core-confirmed scope). The operation record
// persists for the replay window.
func removePluginEffect(tx *sqlTx, pluginID string, _ time.Time) error {
	if _, err := tx.Exec(`DELETE FROM plugins WHERE plugin_id = ?`, pluginID); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// revokePluginGrantsEffect applies the revoke-grants effect: the granted
// set of record is emptied (revocation blocks FUTURE invocations) and the
// lifecycle state moves to the grants-revoked state — DISTINCT from
// disabled and from trust-revoked (the distinctness law; re-grant is the
// user's explicit act through the Core's defined path).
func revokePluginGrantsEffect(tx *sqlTx, pluginID string, now time.Time) error {
	if _, err := tx.Exec(`UPDATE plugins SET lifecycle_state = ?, granted_capabilities_json = '[]', updated_at = ?
		WHERE plugin_id = ?`,
		plugindomain.StateGrantsRevoked, now.UTC().Format(time.RFC3339Nano), pluginID); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// LoadPlugin resolves one plugin's authoritative record (a pure read): the
// unknown identity and the out-of-scope identity are the SAME typed 404
// (the no-leak rule; no Plugin not-found domain code exists in the
// published register — the envelope register's NOT_FOUND rides, the B2/B4
// operation-record precedent).
func (s *Store) LoadPlugin(ctx stdcontext.Context, pluginID string) (plugindomain.Plugin, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT plugin_id, lifecycle_state, package_ref, version_ref, install_record_ref, package_json, granted_capabilities_json, installed_at
FROM plugins WHERE plugin_id = ?`, pluginID)
	return scanPlugin(row)
}

// ListPlugins reads one bounded keyset page (created_at DESC, plugin_id
// DESC — the newest first; the keyset list semantics; the cursor's absence
// marks the final page). The list IS the user-installed list
// (USER-INSTALLED ≠ CATALOG-LISTED, always; no catalog exists — none is
// published).
func (s *Store) ListPlugins(ctx stdcontext.Context, q plugindomain.ListQuery) (plugindomain.PluginPage, error) {
	conds := []string{"1=1"}
	args := []any{}
	if q.AfterCreatedAt != "" {
		conds = append(conds, "(created_at < ? OR (created_at = ? AND plugin_id < ?))")
		args = append(args, q.AfterCreatedAt, q.AfterCreatedAt, q.AfterID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.readers.QueryContext(ctx, `
SELECT plugin_id, lifecycle_state, package_ref, version_ref, install_record_ref, package_json, granted_capabilities_json, installed_at
FROM plugins WHERE `+strings.Join(conds, " AND ")+`
ORDER BY created_at DESC, plugin_id DESC LIMIT ?`, args...)
	if err != nil {
		return plugindomain.PluginPage{}, mapVendorError(err)
	}
	defer func() { _ = rows.Close() }()
	out := plugindomain.PluginPage{Plugins: []plugindomain.Plugin{}}
	for rows.Next() {
		rec, err := scanPluginRows(rows)
		if err != nil {
			return plugindomain.PluginPage{}, err
		}
		out.Plugins = append(out.Plugins, rec.domain())
	}
	if err := rows.Err(); err != nil {
		return plugindomain.PluginPage{}, mapVendorError(err)
	}
	if len(out.Plugins) > q.Limit {
		out.More = true
		out.Plugins = out.Plugins[:q.Limit]
	}
	return out, nil
}

// LoadPluginState resolves the family's authoritative state read (a pure
// query — never a retry; the reconcile anchor after a lost write
// response; the family's reconciliation keys resolve here). Events are
// never the source of truth; this endpoint is. The unknown identity is the
// SAME typed 404 (the no-leak rule).
func (s *Store) LoadPluginState(ctx stdcontext.Context, pluginID string) (plugindomain.State, error) {
	row := s.readers.QueryRowContext(ctx, `
SELECT plugin_id, lifecycle_state, package_ref, install_record_ref, updated_at
FROM plugins WHERE plugin_id = ?`, pluginID)
	var st plugindomain.State
	err := row.Scan(&st.PluginID, &st.LifecycleState, &st.PackageRef, &st.InstallRecordRef, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return plugindomain.State{}, errStoragef(errs.CodeNotFound, "no such plugin")
	}
	if err != nil {
		return plugindomain.State{}, mapVendorError(err)
	}
	return st, nil
}

// pluginEvent builds one F10-shaped journal record: the frozen field set
// with a bounded inline payload of REFERENCES only (the redaction law —
// never capability grant values, never secret values, never plugin
// output).
func pluginEvent(eventID, aggregateType, aggregateID, eventType, correlationID, producer string, payload map[string]string) events.Record {
	inline, _ := json.Marshal(payload)
	if len(inline) > events.MaxInlinePayloadBytes {
		inline = nil
	}
	return events.Record{
		EventID:       eventID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		CorrelationID: correlationID,
		Producer:      producer,
		InlinePayload: inline,
	}
}

// revokedStateRefusal builds the revoked-state refusal (the distinctness
// laws' consequence): the grants-revoked state rides the published
// PLUGIN_CAPABILITY_DENIED and the trust-revoked state rides the
// published PLUGIN_TRUST_REVOKED — each DISTINCT, never merged, never a
// milder state.
func revokedStateRefusal(state, format string, args ...any) error {
	code := errs.CodePluginCapabilityDenied
	if state == plugindomain.StateTrustRevoked {
		code = errs.CodePluginTrustRevoked
	}
	return &PluginRevokedStateError{Code: code, Detail: fmt.Sprintf(format, args...)}
}

func scanPlugin(row *sql.Row) (plugindomain.Plugin, error) {
	rec, err := scanPluginRows(row)
	if err != nil {
		return plugindomain.Plugin{}, err
	}
	return rec.domain(), nil
}

func scanPluginRows(sc interface{ Scan(...any) error }) (pluginRow, error) {
	var r pluginRow
	err := sc.Scan(&r.PluginID, &r.LifecycleState, &r.PackageRef, &r.VersionRef, &r.InstallRecordRef, &r.PackageJSON, &r.GrantedJSON, &r.InstalledAt)
	if err == sql.ErrNoRows {
		return pluginRow{}, errStoragef(errs.CodeNotFound, "no such plugin")
	}
	if err != nil {
		return pluginRow{}, mapVendorError(err)
	}
	return r, nil
}

func loadPluginRowTx(tx *sqlTx, pluginID string) (pluginRow, error) {
	row := tx.QueryRow(`SELECT plugin_id, lifecycle_state, package_ref, version_ref, install_record_ref, package_json, granted_capabilities_json, installed_at
FROM plugins WHERE plugin_id = ?`, pluginID)
	return scanPluginRows(row)
}

func insertPlugin(tx *sqlTx, rec plugindomain.Plugin, versionRef string, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	granted, err := json.Marshal([]string{})
	if err != nil {
		return errStoragef(errs.CodeInternal, "the empty granted set does not marshal")
	}
	packageJSON, err := marshalPluginPackage(rec.Package)
	if err != nil {
		return errStoragef(errs.CodeValidationFailed, "the package record does not marshal")
	}
	state := rec.LifecycleState
	if state == "" {
		state = plugindomain.StateInstalled
	}
	if _, err := tx.Exec(`INSERT INTO plugins
		(plugin_id, lifecycle_state, package_ref, version_ref, install_record_ref, package_json, granted_capabilities_json, installed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.PluginID, state, rec.Package.PackageRef, versionRef, rec.InstallRecordRef,
		packageJSON, string(granted), nowNano, nowNano, nowNano); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// marshalPluginPackage serializes the Core-returned verification states
// (each optional member rendered only where the Core returns it — an
// absent verification state is never fabricated).
func marshalPluginPackage(pkg *plugindomain.Package) (string, error) {
	if pkg == nil {
		return "", nil
	}
	b, err := json.Marshal(pkg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func insertPluginOperation(tx *sqlTx, rec pluginOperationRecord, idempotencyKey, fingerprint string, now time.Time) error {
	nowNano := now.UTC().Format(time.RFC3339Nano)
	terminal := nowNano
	_, err := tx.Exec(`INSERT INTO plugin_operations
		(operation_id, kind, state, correlation_id, plugin_id, result_ref,
		 failure_code, failure_reason, idempotency_key, fingerprint, terminal_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.OperationID, rec.Kind, "recorded", rec.CorrelationID, rec.PluginID, rec.ResultRef,
		"", "", idempotencyKey, fingerprint, terminal, nowNano, nowNano)
	if err != nil {
		var serr sqlite3.Error
		if errors.As(err, &serr) && serr.Code == sqlite3.ErrConstraint {
			return errPluginDuplicateKey
		}
		return mapVendorError(err)
	}
	return nil
}

// pluginOpByKey resolves the prior operation under an effective key (nil
// when none).
func (s *Store) pluginOpByKey(ctx stdcontext.Context, key string) (*pluginOperationRecord, error) {
	var rec pluginOperationRecord
	err := s.readers.QueryRowContext(ctx, `
SELECT operation_id, kind, correlation_id, plugin_id, result_ref, created_at, updated_at
FROM plugin_operations WHERE idempotency_key = ?`, key).
		Scan(&rec.OperationID, &rec.Kind, &rec.CorrelationID, &rec.PluginID, &rec.ResultRef, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, mapVendorError(err)
	}
	return &rec, nil
}

func pluginOpByKeyTx(tx *sqlTx, key string) (*pluginOperationRecord, string, error) {
	var rec pluginOperationRecord
	var fingerprint string
	err := tx.QueryRow(`SELECT operation_id, kind, correlation_id, plugin_id, result_ref, fingerprint, created_at, updated_at
FROM plugin_operations WHERE idempotency_key = ?`, key).
		Scan(&rec.OperationID, &rec.Kind, &rec.CorrelationID, &rec.PluginID, &rec.ResultRef, &fingerprint, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", mapVendorError(err)
	}
	return &rec, fingerprint, nil
}
