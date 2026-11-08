package storage

import (
	stdcontext "context"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	plugindomain "github.com/qira-dev/karyora/internal/domain/plugin"
)

// The B5 Plugin lifecycle family's store register (DE-0 queue row 8): the
// C1-07 install class's keyed ruling (72 h post-terminal; a duplicate
// replay returns the SAME install-record reference — never a second one;
// the install lands the plugin in its installed NOT-yet-enabled state with
// an EMPTY granted set of record), the M-29 state-toggle (both directions;
// the revoked states never exitable through it), the M-30 update class (1 h
// post-terminal; the fresh-review law; the atomic upgrade), the M-31
// remove and G7 revoke-grants keyed rulings, the SAME typed 404 for
// unknown identities (the no-leak rule), and the authoritative state read.

// pluginInstallIntent builds a legal C1-07 install intent (the Core-minted
// identities ride the intent; the effective key is workspace +
// package-scoped).
func pluginInstallIntent(pluginID, key, corr, packageRef string) plugindomain.InstallIntent {
	return plugindomain.InstallIntent{
		Plugin: plugindomain.Plugin{
			PluginID:         pluginID,
			Package:          &plugindomain.Package{PackageRef: "pkg-" + pluginID},
			LifecycleState:   plugindomain.StateInstalled,
			InstallRecordRef: "plugrec-" + pluginID,
		},
		VersionRef:     packageRef,
		OperationID:    "plugop-" + pluginID,
		IdempotencyKey: plugindomain.InstallKey(key, packageRef),
		Fingerprint:    plugindomain.InstallFingerprint(corr, packageRef),
	}
}

// pluginUpdateIntent builds a legal M-30 update intent (the operation
// identity is unique per effective key — the operation_id is the store's
// PK).
func pluginUpdateIntent(pluginID, key, corr, target string) plugindomain.UpdateIntent {
	return plugindomain.UpdateIntent{
		PluginID:                    pluginID,
		TargetVersionRef:            target,
		PackageRef:                  "pkg-target-" + pluginID,
		TargetRequestedCapabilities: []string{},
		OperationID:                 "plugop-upd-" + pluginID + "-" + key + "-" + target,
		IdempotencyKey:              plugindomain.UpdateKey(key, target),
		Fingerprint:                 plugindomain.UpdateFingerprint(corr, target),
		CorrelationID:               corr,
	}
}

// pluginMutationIntent builds a legal M-31/G7 mutation intent (the
// operation identity is unique per effective key).
func pluginMutationIntent(pluginID, key, corr string, kind string) plugindomain.MutationIntent {
	intent := plugindomain.MutationIntent{
		PluginID:      pluginID,
		OperationID:   "plugop-" + kind + "-" + pluginID + "-" + key,
		CorrelationID: corr,
		Fingerprint:   corr,
	}
	if kind == "remove" {
		intent.IdempotencyKey = plugindomain.RemoveKey(key, pluginID)
	} else {
		intent.IdempotencyKey = plugindomain.RevokeKey(key, pluginID)
	}
	return intent
}

// backdatePluginOperation moves one recorded operation's terminal stamp
// behind the replay window (the fixture manipulation for the past-window
// ruling — the product code is untouched).
func backdatePluginOperation(t *testing.T, s *Store, operationID string, age time.Duration) {
	t.Helper()
	past := time.Now().Add(-age).UTC().Format(time.RFC3339Nano)
	if err := s.CommitStateChange(stdcontext.Background(), "corr-backdate", func(tx *sqlTx) error {
		_, err := tx.Exec(`UPDATE plugin_operations SET updated_at = ?, terminal_at = ? WHERE operation_id = ?`,
			past, past, operationID)
		return err
	}); err != nil {
		t.Fatalf("backdate fixture: %v", err)
	}
}

// TestPluginInstallClass pins the C1-07 install class's replay outcomes
// table at the store: the fresh install is Core-confirmed (the Core-minted
// identities; the installed NOT-yet-enabled state; the EMPTY granted set of
// record — the never-grant law); the same key + the same fingerprint
// replays the recorded install (the SAME install-record reference, never a
// second one); the same key with a different fingerprint is the typed
// CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false — same key + different
// body); a replay past the 72 h post-terminal window is the typed
// CONFLICT_STATE.
func TestPluginInstallClass(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	// The fresh install: Core-confirmed, the installed NOT-yet-enabled
	// state, the EMPTY granted set of record.
	first, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-1", "key-1", "corr-1", "pkgref-1"))
	if err != nil {
		t.Fatalf("fresh install: %v", err)
	}
	if first.Replayed || first.InstallRecordRef != "plugrec-plug-1" {
		t.Fatalf("the fresh install: %+v", first)
	}
	rec, err := s.LoadPlugin(ctx, "plug-1")
	if err != nil {
		t.Fatalf("installed record: %v", err)
	}
	if rec.LifecycleState != plugindomain.StateInstalled {
		t.Fatalf("the install lands the plugin in its installed NOT-yet-enabled state: %+v", rec)
	}
	if rec.Package == nil || rec.Package.PackageRef != "pkg-plug-1" {
		t.Fatalf("the Core-minted package identity: %+v", rec.Package)
	}

	// The same key + the same fingerprint: the keyed replay returns the
	// SAME install-record reference — never a second one.
	replay, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-OTHER", "key-1", "corr-1", "pkgref-1"))
	if err != nil {
		t.Fatalf("replay install: %v", err)
	}
	if !replay.Replayed || replay.InstallRecordRef != "plugrec-plug-1" {
		t.Fatalf("the replay returns the recorded install record, never a second one: %+v", replay)
	}
	var installed int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugins`).Scan(&installed); err != nil {
		t.Fatalf("plugin count: %v", err)
	}
	if installed != 1 {
		t.Fatalf("a replay never installs a second plugin: %d", installed)
	}

	// The same key + the same package identity with a different request
	// identity (the C1-07 class: same key + different body — the
	// correlation ID is the request-identity half of the fingerprint): the
	// typed CONFLICT_IDEMPOTENCY_KEY with fingerprintMatch:false.
	_, err = s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-2", "key-1", "corr-DIFFERENT", "pkgref-1"))
	ice, ok := err.(*PluginIdempotencyConflictError)
	if !ok {
		t.Fatalf("expected the CONFLICT_IDEMPOTENCY_KEY carrier, got %v", err)
	}
	envl, eerr := ice.Envelope("corr-x")
	if eerr != nil || envl.Code != errs.CodeConflictIdempotencyKey || envl.Data["fingerprintMatch"] != false {
		t.Fatalf("the conflict envelope: %v %+v", eerr, envl.Data)
	}

	// The KEY-SCOPE LAW: the effective key is scoped workspace + package
	// identity, so the same client key under a different package reference
	// is a DIFFERENT effective key — a fresh install succeeds (the keys do
	// not collide across package identities).
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-2", "key-1", "corr-2", "pkgref-2")); err != nil {
		t.Fatalf("the same client key under a different package identity is a different effective key: %v", err)
	}
	// A different key installs another plugin.
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-3", "key-3", "corr-3", "pkgref-3")); err != nil {
		t.Fatalf("third install: %v", err)
	}

	// The replay past the 72 h post-terminal window: the typed
	// CONFLICT_STATE.
	backdatePluginOperation(t, s, first.OperationID, PluginInstallReplayWindow+time.Minute)
	_, err = s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-9", "key-1", "corr-1", "pkgref-1"))
	if err == nil {
		t.Fatalf("the past-window replay must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitPluginInstall(ctx, plugindomain.InstallIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}

	// The register's plugin.install_completed event rode each fresh install
	// (the F10-shaped journal; the payload carries references only — the
	// redaction law).
	var events int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal
WHERE aggregate_type = ? AND event_type = ?`,
		plugindomain.AggregatePlugin, plugindomain.EventInstallCompleted).Scan(&events); err != nil {
		t.Fatalf("event count: %v", err)
	}
	if events != 3 {
		t.Fatalf("each fresh install recorded plugin.install_completed (plug-1, plug-2, plug-3): %d", events)
	}
}

// TestPluginToggleM29 pins the M-29 state-toggle class at the store: ONE
// member covering both directions via the body's target state; the toggle
// is the Core-recorded state-checked mutation (each toggle its own
// Core-confirmed record); the revoked states are NEVER exitable through it
// (the distinctness laws — the typed refusals ride the published register
// codes the states carry); the unknown identity is the SAME typed 404.
func TestPluginToggleM29(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-1", "key-1", "corr-1", "pkgref-1")); err != nil {
		t.Fatalf("fixture install: %v", err)
	}

	// Enable: installed → enabled (the Core-confirmed toggle record).
	out, err := s.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{PluginID: "plug-1", Enabled: true, OperationID: "plugop-t1", CorrelationID: "corr-t1"})
	if err != nil || out.Replayed || out.PluginID != "plug-1" {
		t.Fatalf("enable: %v %+v", err, out)
	}
	st, err := s.LoadPluginState(ctx, "plug-1")
	if err != nil || st.LifecycleState != plugindomain.StateEnabled {
		t.Fatalf("the enabled state: %v %+v", err, st)
	}

	// Disable: enabled → disabled (the OTHER direction through the ONE
	// member).
	out, err = s.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{PluginID: "plug-1", Enabled: false, OperationID: "plugop-t2", CorrelationID: "corr-t2"})
	if err != nil || out.Replayed {
		t.Fatalf("disable: %v %+v", err, out)
	}
	st, _ = s.LoadPluginState(ctx, "plug-1")
	if st.LifecycleState != plugindomain.StateDisabled {
		t.Fatalf("the disabled state (DISTINCT from the revoked states): %+v", st)
	}

	// The register's events: plugin.enabled and plugin.disabled each rode
	// their Core-confirmed toggle; no invocation event exists.
	var enabledEvents, disabledEvents int
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_type = ?`, plugindomain.EventEnabled).Scan(&enabledEvents)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_type = ?`, plugindomain.EventDisabled).Scan(&disabledEvents)
	if enabledEvents != 1 || disabledEvents != 1 {
		t.Fatalf("each toggle recorded its event: enabled=%d disabled=%d", enabledEvents, disabledEvents)
	}

	// THE DISTINCTNESS LAWS' CONSEQUENCE: from the grants-revoked state the
	// toggle refusal rides PLUGIN_CAPABILITY_DENIED (a toggle never
	// re-grants); from the trust-revoked state it rides
	// PLUGIN_TRUST_REVOKED (no lifecycle control).
	for _, tc := range []struct {
		state string
		want  errs.Code
	}{
		{plugindomain.StateGrantsRevoked, errs.CodePluginCapabilityDenied},
		{plugindomain.StateTrustRevoked, errs.CodePluginTrustRevoked},
	} {
		if err := s.CommitStateChange(ctx, "corr-fixture", func(tx *sqlTx) error {
			_, err := tx.Exec(`UPDATE plugins SET lifecycle_state = ? WHERE plugin_id = ?`, tc.state, "plug-1")
			return err
		}); err != nil {
			t.Fatalf("state fixture: %v", err)
		}
		_, err = s.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{PluginID: "plug-1", Enabled: true, OperationID: "plugop-tx", CorrelationID: "corr-tx"})
		rse, ok := err.(*PluginRevokedStateError)
		if !ok {
			t.Fatalf("%s: expected the revoked-state carrier, got %v", tc.state, err)
		}
		if rse.Code != tc.want {
			t.Fatalf("%s: expected %s, got %s (the distinctness law)", tc.state, tc.want, rse.Code)
		}
		envl, eerr := rse.Envelope("corr-x")
		if eerr != nil || envl.Code != tc.want {
			t.Fatalf("%s: the refusal envelope: %v %+v", tc.state, eerr, envl)
		}
		st, _ = s.LoadPluginState(ctx, "plug-1")
		if st.LifecycleState != tc.state {
			t.Fatalf("the revoked state is never exitable through the toggle: %+v", st)
		}
	}

	// The unknown identity: the SAME typed 404 (the no-leak rule).
	_, err = s.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{PluginID: "plug-nope", Enabled: true, OperationID: "plugop-t3", CorrelationID: "corr-t3"})
	if err == nil {
		t.Fatalf("the unknown plugin must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitPluginToggle(ctx, plugindomain.ToggleIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
}

// TestPluginUpdateM30 pins the M-30 update class at the store: the atomic
// upgrade (the prior record intact until the Core-confirmed switch commits
// — the lifecycle state, the granted set, and the install-record reference
// preserved); the fresh-review law (ANY permission change refuses the
// update — the typed PLUGIN_CAPABILITY_DENIED stores nothing and consumes
// no key); the keyed replay ruling (1 h post-terminal; the same key with a
// different correlation is CONFLICT_IDEMPOTENCY_KEY; a different target or
// a past-window replay is CONFLICT_STATE); the unknown identity is the
// SAME typed 404 and consumes no key.
func TestPluginUpdateM30(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-1", "key-1", "corr-1", "pkgref-1")); err != nil {
		t.Fatalf("fixture install: %v", err)
	}

	// The fresh update: Core-confirmed; the atomic upgrade preserves the
	// lifecycle state, the granted set, and the install-record reference
	// (only the package identity and the version-of-record switch).
	out, err := s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-1", "corr-u1", "pkgref-2"))
	if err != nil || out.Replayed {
		t.Fatalf("fresh update: %v %+v", err, out)
	}
	rec, err := s.LoadPlugin(ctx, "plug-1")
	if err != nil || rec.LifecycleState != plugindomain.StateInstalled || rec.InstallRecordRef != "plugrec-plug-1" {
		t.Fatalf("the atomic upgrade preserves the prior summary: %v %+v", err, rec)
	}

	// The keyed replay: the same key + the same correlation + the same
	// target replays the recorded update — never a second one.
	replay, err := s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-1", "corr-u1", "pkgref-2"))
	if err != nil || !replay.Replayed || replay.OperationRef != "plugop-upd-plug-1-ukey-1-pkgref-2" {
		t.Fatalf("the replay returns the recorded update: %v %+v", err, replay)
	}

	// The same key with a different correlation: the typed
	// CONFLICT_IDEMPOTENCY_KEY.
	_, err = s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-1", "corr-OTHER", "pkgref-2"))
	ice, ok := err.(*PluginIdempotencyConflictError)
	if !ok {
		t.Fatalf("expected the CONFLICT_IDEMPOTENCY_KEY carrier, got %v", err)
	}
	if envl, eerr := ice.Envelope("corr-x"); eerr != nil || envl.Code != errs.CodeConflictIdempotencyKey {
		t.Fatalf("the conflict envelope: %v", eerr)
	}

	// The KEY-SCOPE LAW: the effective key is scoped workspace + package
	// identity (the target version reference is the scope component), so
	// the same client key under a different target is a DIFFERENT effective
	// key — a fresh update succeeds (the keys do not collide across
	// targets).
	if _, err := s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-1", "corr-u1", "pkgref-OTHER")); err != nil {
		t.Fatalf("the same client key under a different target is a different effective key: %v", err)
	}

	// THE FRESH-REVIEW LAW: ANY permission change refuses the update — the
	// typed PLUGIN_CAPABILITY_DENIED stores nothing and consumes no key (a
	// retry never grants; the user's explicit act governs).
	denied := pluginUpdateIntent("plug-1", "ukey-2", "corr-u2", "pkgref-3")
	denied.TargetRequestedCapabilities = []string{"network.connect"}
	out2, err := s.SubmitPluginUpdate(ctx, denied)
	if err == nil {
		t.Fatalf("the un-reviewed permission change must refuse the update")
	}
	rse, ok := err.(*PluginRevokedStateError)
	if !ok || rse.Code != errs.CodePluginCapabilityDenied {
		t.Fatalf("expected the PLUGIN_CAPABILITY_DENIED carrier, got %v", err)
	}
	if envl, eerr := rse.Envelope("corr-x"); eerr != nil || envl.Code != errs.CodePluginCapabilityDenied ||
		envl.Data["precondition"] != "PERMISSION-REVIEW" {
		t.Fatalf("the fresh-review refusal envelope: %v %+v", eerr, envl.Data)
	}
	_ = out2
	// The refusal stores nothing: the version-of-record stays the last
	// Core-confirmed one (the refused update's target never landed).
	var versionRef string
	if err := s.readers.QueryRowContext(ctx, `SELECT version_ref FROM plugins WHERE plugin_id = ?`, "plug-1").Scan(&versionRef); err != nil {
		t.Fatalf("version read: %v", err)
	}
	if versionRef != "pkgref-OTHER" {
		t.Fatalf("the refusal stores nothing (the update did not complete): version_ref %q", versionRef)
	}
	// The refusal consumed no key: the same key now serves a fresh update.
	if _, err := s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-2", "corr-u2", "pkgref-3")); err != nil {
		t.Fatalf("the refusal consumed no key: %v", err)
	}

	// The replay past the 1 h post-terminal window: the typed
	// CONFLICT_STATE.
	backdatePluginOperation(t, s, "plugop-upd-plug-1-ukey-1-pkgref-2", PluginShortReplayWindow+time.Minute)
	_, err = s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-1", "corr-u1", "pkgref-2"))
	if err == nil {
		t.Fatalf("the past-window replay must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}

	// The unknown identity: the SAME typed 404, and the key stays
	// unconsumed by the unresolvable intent.
	_, err = s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-nope", "ukey-3", "corr-u3", "pkgref-9"))
	if err == nil {
		t.Fatalf("the unknown plugin must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
	if _, err := s.SubmitPluginUpdate(ctx, pluginUpdateIntent("plug-1", "ukey-3", "corr-u3", "pkgref-4")); err != nil {
		t.Fatalf("the unresolvable intent consumed no key: %v", err)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitPluginUpdate(ctx, plugindomain.UpdateIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
}

// TestPluginRemoveRevoke pins the M-31 remove and G7 revoke-grants classes
// at the store: remove deletes the record from the installed set (the
// operation record persists for the replay window); revoke empties the
// granted set of record and moves the state to grants-revoked (DISTINCT
// from disabled and from trust-revoked — the distinctness law); both ride
// the 1 h keyed replay law; the trust-revoked state admits neither (no
// lifecycle control applies); the unknown identity is the SAME typed 404.
func TestPluginRemoveRevoke(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-1", "key-1", "corr-1", "pkgref-1")); err != nil {
		t.Fatalf("fixture install: %v", err)
	}

	// The G7 revoke-grants member: the granted set empties; the state moves
	// to grants-revoked (revocation blocks FUTURE invocations).
	out, err := s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-1", "rkey-1", "corr-r1", "revoke"))
	if err != nil || out.Replayed {
		t.Fatalf("revoke grants: %v %+v", err, out)
	}
	st, err := s.LoadPluginState(ctx, "plug-1")
	if err != nil || st.LifecycleState != plugindomain.StateGrantsRevoked {
		t.Fatalf("the grants-revoked state (DISTINCT from disabled and trust-revoked): %v %+v", err, st)
	}
	if _, err := s.LoadPlugin(ctx, "plug-1"); err != nil {
		t.Fatalf("the revoked record stays loadable: %v", err)
	}
	var granted string
	if err := s.readers.QueryRowContext(ctx, `SELECT granted_capabilities_json FROM plugins WHERE plugin_id = ?`, "plug-1").Scan(&granted); err != nil {
		t.Fatalf("granted set read: %v", err)
	}
	if granted != "[]" {
		t.Fatalf("the granted set of record is emptied (revocation blocks FUTURE invocations): %s", granted)
	}
	var revokeEvents int
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_type = ?`, plugindomain.EventGrantsRevoked).Scan(&revokeEvents)
	if revokeEvents != 1 {
		t.Fatalf("the revoke recorded plugin.grants_revoked: %d", revokeEvents)
	}

	// The keyed replay of the revoke: the recorded outcome, never a second
	// mutation.
	replay, err := s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-1", "rkey-1", "corr-r1", "revoke"))
	if err != nil || !replay.Replayed {
		t.Fatalf("the revoke replay: %v %+v", err, replay)
	}
	// The same key with a different correlation: the typed
	// CONFLICT_IDEMPOTENCY_KEY.
	_, err = s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-1", "rkey-1", "corr-OTHER", "revoke"))
	if ice, ok := err.(*PluginIdempotencyConflictError); ok {
		if envl, eerr := ice.Envelope("corr-x"); eerr != nil || envl.Code != errs.CodeConflictIdempotencyKey {
			t.Fatalf("the conflict envelope: %v", eerr)
		}
	} else {
		t.Fatalf("expected the CONFLICT_IDEMPOTENCY_KEY carrier, got %v", err)
	}

	// The M-31 remove member: the record leaves the installed set; the
	// operation record persists for the replay window (Core-confirmed
	// removal; no erasure claim beyond it).
	out, err = s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-1", "mkey-1", "corr-m1", "remove"))
	if err != nil || out.Replayed {
		t.Fatalf("remove: %v %+v", err, out)
	}
	if _, err := s.LoadPlugin(ctx, "plug-1"); err == nil {
		t.Fatalf("the removed plugin leaves the installed set")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("the removed plugin is the typed NOT_FOUND: %v", err)
	}
	var removeOps int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE kind = ?`, pluginKindRemove).Scan(&removeOps); err != nil {
		t.Fatalf("operation record: %v", err)
	}
	if removeOps != 1 {
		t.Fatalf("the operation record persists for the replay window: %d", removeOps)
	}
	// The keyed replay of the remove: the recorded outcome (the plugin is
	// gone but the ruling replays).
	replay, err = s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-1", "mkey-1", "corr-m1", "remove"))
	if err != nil || !replay.Replayed {
		t.Fatalf("the remove replay: %v %+v", err, replay)
	}

	// The trust-revoked state admits neither member (no lifecycle control
	// applies; re-admission is the Core policy's decision alone).
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-2", "key-2", "corr-2", "pkgref-2")); err != nil {
		t.Fatalf("fixture install 2: %v", err)
	}
	if err := s.CommitStateChange(ctx, "corr-fixture2", func(tx *sqlTx) error {
		_, err := tx.Exec(`UPDATE plugins SET lifecycle_state = ? WHERE plugin_id = ?`, plugindomain.StateTrustRevoked, "plug-2")
		return err
	}); err != nil {
		t.Fatalf("state fixture: %v", err)
	}
	_, err = s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-2", "rkey-2", "corr-r2", "revoke"))
	rse, ok := err.(*PluginRevokedStateError)
	if !ok || rse.Code != errs.CodePluginTrustRevoked {
		t.Fatalf("the trust-revoked refusal: %v", err)
	}
	_, err = s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-2", "mkey-2", "corr-m2", "remove"))
	if rse, ok := err.(*PluginRevokedStateError); !ok || rse.Code != errs.CodePluginTrustRevoked {
		t.Fatalf("the trust-revoked refusal (remove): %v", err)
	}

	// The unknown identity: the SAME typed 404, and the key stays
	// unconsumed (a fresh plugin under the same key removes successfully
	// afterwards — the unresolvable intent never consumed the key).
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-3", "key-3", "corr-3", "pkgref-3")); err != nil {
		t.Fatalf("fixture install 3: %v", err)
	}
	_, err = s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-nope", "mkey-3", "corr-m3", "remove"))
	if err == nil {
		t.Fatalf("the unknown plugin must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
	if _, err := s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-3", "mkey-3", "corr-m3", "remove")); err != nil {
		t.Fatalf("the unresolvable intent consumed no key: %v", err)
	}

	// The intent validation: a missing identity member refuses.
	if _, err := s.SubmitPluginRemove(ctx, plugindomain.MutationIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
	if _, err := s.SubmitPluginRevokeGrants(ctx, plugindomain.MutationIntent{}); err == nil {
		t.Fatalf("an empty intent must refuse")
	}
}

// TestPluginReads pins the read surfaces: the inspect member (ONLY
// Core-returned verification states rendered — an absent verification state
// is never fabricated), the keyset list (created_at DESC, plugin_id DESC;
// the page bound and the More flag), the authoritative state read, and the
// SAME typed 404 for unknown identities on every read (the no-leak rule).
func TestPluginReads(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	// The unknown identity: the SAME typed 404 on every read.
	for name, call := range map[string]func() error{
		"plugin": func() error { _, err := s.LoadPlugin(ctx, "plug-nope"); return err },
		"state":  func() error { _, err := s.LoadPluginState(ctx, "plug-nope"); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s: the unknown identity must refuse", name)
		} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
			t.Fatalf("%s: expected the typed NOT_FOUND, got %v", name, err)
		}
	}

	// The fixture: three installed plugins.
	for i, id := range []string{"plug-a", "plug-b", "plug-c"} {
		if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent(id, "key-"+id, "corr-"+id, "pkgref-"+id)); err != nil {
			t.Fatalf("fixture install %d: %v", i, err)
		}
	}

	// The inspect member: the record renders the Core-returned members
	// (the Core-minted package identity; the verification states ABSENT
	// where the Core returns none — never fabricated).
	rec, err := s.LoadPlugin(ctx, "plug-a")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if rec.PluginID != "plug-a" || rec.LifecycleState != plugindomain.StateInstalled ||
		rec.InstallRecordRef != "plugrec-plug-a" || rec.Package == nil || rec.Package.PackageRef != "pkg-plug-a" {
		t.Fatalf("the inspect member: %+v", rec)
	}
	if rec.Package.ArtifactDigest != "" || rec.Package.SignatureVerificationResult != "" ||
		rec.Package.TrustRootRef != "" || rec.Package.RotationStatus != "" || rec.Package.RevocationStatus != "" {
		t.Fatalf("no verification state is ever fabricated: %+v", rec.Package)
	}

	// The authoritative state read: the reconcile anchor.
	st, err := s.LoadPluginState(ctx, "plug-a")
	if err != nil || st.PluginID != "plug-a" || st.LifecycleState != plugindomain.StateInstalled ||
		st.PackageRef != "pkg-plug-a" || st.InstallRecordRef != "plugrec-plug-a" || st.UpdatedAt == "" {
		t.Fatalf("the authoritative state: %v %+v", err, st)
	}

	// The keyset list: created_at DESC, plugin_id DESC; the page bound and
	// the More flag.
	page, err := s.ListPlugins(ctx, plugindomain.ListQuery{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Plugins) != 2 || !page.More {
		t.Fatalf("the bounded page: %+v", page)
	}
	if page.Plugins[0].PluginID != "plug-c" || page.Plugins[1].PluginID != "plug-b" {
		t.Fatalf("the keyset order (newest first): %+v", page.Plugins)
	}
	full, err := s.ListPlugins(ctx, plugindomain.ListQuery{Limit: 10})
	if err != nil || len(full.Plugins) != 3 || full.More {
		t.Fatalf("the final page: %v %+v", err, full)
	}
	// The keyset continuation: the position rides the last page's sort-key
	// values.
	cont, err := s.ListPlugins(ctx, plugindomain.ListQuery{Limit: 10, AfterCreatedAt: full.Plugins[0].InstalledAt, AfterID: full.Plugins[0].PluginID})
	if err != nil || len(cont.Plugins) != 2 || cont.More {
		t.Fatalf("the keyset continuation: %v %+v", err, cont)
	}
	if cont.Plugins[0].PluginID != "plug-b" {
		t.Fatalf("the continuation order: %+v", cont.Plugins)
	}
}

// TestPluginRemoveRevokeDistinctness pins THE REMOVE-VS-REVOKE
// DISTINCTNESS (recorded at the 2026-11-09 resolution act, binding on this
// family — the G-09a fixture set, the XC-1 §5 G-09 row's decided half):
// removal (M-31) and grant revocation (G7) are DISTINCT Core semantics —
// removing a plugin does NOT revoke its grants; revoking grants does NOT
// remove the artifact; each acts only on its own named object. Pinned per
// what the Core actually serves: the removal's effect is the artifact
// record's deletion (its own named object) with NO revocation semantics
// applied anywhere (no grants-revoked state on any record, no revocation
// event in the journal, no revoke_grants operation record); the
// revocation's effect is the granted set of record emptied + the
// grants-revoked state on the STILL-INSTALLED artifact record; and the two
// intents carry distinct operation identities (the distinct keying scopes
// RemoveKey vs RevokeKey; the distinct operation-record kinds; the
// distinct event types), so the same client key carries two independent
// recorded outcomes, a post-removal revoke intent cannot resolve, and a
// removal after a revocation still applies.
func TestPluginRemoveRevokeDistinctness(t *testing.T) {
	s := openTestStore(t)
	ctx := stdcontext.Background()

	// seedGrants records a granted set of record on the fixture plugin
	// (the never-grant law: no served operation grants, so the fixture
	// records the set the Core holds — the established store-fixture
	// idiom) and pins that the set exists at mutation time.
	seedGrants := func(pluginID string) {
		t.Helper()
		if err := s.CommitStateChange(ctx, "corr-g09a-seed", func(tx *sqlTx) error {
			_, err := tx.Exec(`UPDATE plugins SET granted_capabilities_json = ? WHERE plugin_id = ?`,
				`["network.connect","storage.read"]`, pluginID)
			return err
		}); err != nil {
			t.Fatalf("granted-set fixture: %v", err)
		}
		var granted string
		if err := s.readers.QueryRowContext(ctx, `SELECT granted_capabilities_json FROM plugins WHERE plugin_id = ?`, pluginID).Scan(&granted); err != nil {
			t.Fatalf("granted-set read: %v", err)
		}
		if granted != `["network.connect","storage.read"]` {
			t.Fatalf("the granted set of record at mutation time: %s", granted)
		}
	}

	// PART 1 — REMOVE-NEVER-REVOKES: the removal acts on its own named
	// object (the artifact record) and applies NO revocation semantics.
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-g09a-r", "key-g09a-r", "corr-g09a-r", "pkgref-g09a-r")); err != nil {
		t.Fatalf("fixture install (remove case): %v", err)
	}
	seedGrants("plug-g09a-r")
	out, err := s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-g09a-r", "mkey-g09a-r", "corr-g09a-rm", "remove"))
	if err != nil || out.Replayed {
		t.Fatalf("the M-31 removal: %v %+v", err, out)
	}
	// The authoritative state read: the artifact GONE (the SAME typed 404
	// the inspect read serves; absent from the user-installed list).
	if _, err := s.LoadPlugin(ctx, "plug-g09a-r"); err == nil {
		t.Fatalf("the removed artifact leaves the installed set")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("the removed artifact is the typed NOT_FOUND: %v", err)
	}
	if _, err := s.LoadPluginState(ctx, "plug-g09a-r"); err == nil {
		t.Fatalf("the authoritative state read: the artifact is gone")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("the state read's typed NOT_FOUND: %v", err)
	}
	page, err := s.ListPlugins(ctx, plugindomain.ListQuery{Limit: 100})
	if err != nil {
		t.Fatalf("the user-installed list: %v", err)
	}
	for _, listed := range page.Plugins {
		if listed.PluginID == "plug-g09a-r" {
			t.Fatalf("the removed artifact is absent from the user-installed list: %+v", page.Plugins)
		}
	}
	// THE GRANT RECORDS ARE NOT ASSERTED-REVOKED (per what the Core
	// actually serves): the removal applied NO revocation semantics — no
	// grants-revoked state on any record, no revocation event in the
	// journal, no revoke_grants operation record. The grant records ride
	// the artifact record per the Core's own storage; the removal deletes
	// the artifact record whole (the deletion-honesty class — no erasure
	// claim beyond the Core-confirmed scope) and never rides the
	// revocation vocabulary.
	var grantsRevokedStates, revokeEvents, revokeOps, removedEvents, removeOps int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugins WHERE lifecycle_state = ?`, plugindomain.StateGrantsRevoked).Scan(&grantsRevokedStates); err != nil {
		t.Fatalf("grants-revoked state count: %v", err)
	}
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_type = ?`, plugindomain.EventGrantsRevoked).Scan(&revokeEvents)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE kind = ?`, pluginKindRevokeGrants).Scan(&revokeOps)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE event_type = ?`, plugindomain.EventRemoved).Scan(&removedEvents)
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE kind = ?`, pluginKindRemove).Scan(&removeOps); err != nil {
		t.Fatalf("operation record count: %v", err)
	}
	if grantsRevokedStates != 0 || revokeEvents != 0 || revokeOps != 0 {
		t.Fatalf("the removal never rode the revocation semantics: grants_revoked states=%d revoke events=%d revoke op records=%d", grantsRevokedStates, revokeEvents, revokeOps)
	}
	if removedEvents != 1 || removeOps != 1 {
		t.Fatalf("the removal's own recorded trace (its own identity only): removed events=%d remove op records=%d", removedEvents, removeOps)
	}
	// The removal's replay resolves the RECORDED removal outcome (never a
	// second removal, never a revocation).
	replay, err := s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-g09a-r", "mkey-g09a-r", "corr-g09a-rm", "remove"))
	if err != nil || !replay.Replayed || replay.OperationRef != out.OperationRef {
		t.Fatalf("the removal's replay resolves the recorded removal outcome: %v %+v", err, replay)
	}

	// PART 2 — REVOKE-NEVER-REMOVES: the revocation acts on its own named
	// object (the granted set of record + the lifecycle state) and the
	// artifact STAYS INSTALLED.
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-g09a-v", "key-g09a-v", "corr-g09a-v", "pkgref-g09a-v")); err != nil {
		t.Fatalf("fixture install (revoke case): %v", err)
	}
	seedGrants("plug-g09a-v")
	out, err = s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-g09a-v", "rkey-g09a-v", "corr-g09a-v", "revoke"))
	if err != nil || out.Replayed {
		t.Fatalf("the G7 revocation: %v %+v", err, out)
	}
	// The authoritative state read: the artifact STILL INSTALLED — the
	// grants-revoked state on the still-loadable record; still in the
	// user-installed list.
	rec, err := s.LoadPlugin(ctx, "plug-g09a-v")
	if err != nil || rec.LifecycleState != plugindomain.StateGrantsRevoked {
		t.Fatalf("the artifact stays installed under the grants-revoked state: %v %+v", err, rec)
	}
	st, err := s.LoadPluginState(ctx, "plug-g09a-v")
	if err != nil || st.LifecycleState != plugindomain.StateGrantsRevoked {
		t.Fatalf("the authoritative state read: %v %+v", err, st)
	}
	page, err = s.ListPlugins(ctx, plugindomain.ListQuery{Limit: 100})
	if err != nil {
		t.Fatalf("the user-installed list: %v", err)
	}
	stillListed := false
	for _, listed := range page.Plugins {
		if listed.PluginID == "plug-g09a-v" {
			stillListed = true
		}
	}
	if !stillListed {
		t.Fatalf("revoking grants does NOT remove the artifact — it stays in the user-installed list: %+v", page.Plugins)
	}
	// The revocation acted on ITS own named object: the granted set of
	// record is emptied (revocation blocks FUTURE invocations) while the
	// artifact record persists.
	var granted string
	if err := s.readers.QueryRowContext(ctx, `SELECT granted_capabilities_json FROM plugins WHERE plugin_id = ?`, "plug-g09a-v").Scan(&granted); err != nil {
		t.Fatalf("granted-set read after the revocation: %v", err)
	}
	if granted != "[]" {
		t.Fatalf("the granted set of record is emptied by the revocation: %s", granted)
	}
	// THE REMOVAL SEMANTICS WERE NEVER APPLIED (per what the Core actually
	// serves, scoped to this plugin): the artifact record still exists (not
	// deleted), no removal event rode the revocation, no remove-kind
	// operation record exists; the revocation's own trace is exactly one
	// grants_revoked event + one revoke_grants record.
	var artifactRows, vRemovedEvents, vRemoveOps, vRevokeEvents, vRevokeOps int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugins WHERE plugin_id = ?`, "plug-g09a-v").Scan(&artifactRows); err != nil {
		t.Fatalf("artifact record count: %v", err)
	}
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = ?`, "plug-g09a-v", plugindomain.EventRemoved).Scan(&vRemovedEvents)
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE plugin_id = ? AND kind = ?`, "plug-g09a-v", pluginKindRemove).Scan(&vRemoveOps); err != nil {
		t.Fatalf("remove op count: %v", err)
	}
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = ?`, "plug-g09a-v", plugindomain.EventGrantsRevoked).Scan(&vRevokeEvents)
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE plugin_id = ? AND kind = ?`, "plug-g09a-v", pluginKindRevokeGrants).Scan(&vRevokeOps); err != nil {
		t.Fatalf("revoke op count: %v", err)
	}
	if artifactRows != 1 || vRemovedEvents != 0 || vRemoveOps != 0 {
		t.Fatalf("the revocation never rode the removal semantics: artifact rows=%d removed events=%d remove op records=%d", artifactRows, vRemovedEvents, vRemoveOps)
	}
	if vRevokeEvents != 1 || vRevokeOps != 1 {
		t.Fatalf("the revocation's own recorded trace: revoke events=%d revoke op records=%d", vRevokeEvents, vRevokeOps)
	}
	replay, err = s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-g09a-v", "rkey-g09a-v", "corr-g09a-v", "revoke"))
	if err != nil || !replay.Replayed || replay.OperationRef != out.OperationRef {
		t.Fatalf("the revocation's replay resolves the recorded revocation outcome: %v %+v", err, replay)
	}

	// PART 3 — THE DISTINCT OUTCOME IDENTITIES: the two intents' distinct
	// shapes (the published classes' distinct keying scopes and journals).
	// (a) The distinct keying scopes: the same client key under the two
	// classes builds two DIFFERENT effective keys (remove: workspace +
	// plugin; revoke: workspace + plugin + the grant-scope component
	// Core-side).
	if plugindomain.RemoveKey("key-x", "plug-1") == plugindomain.RevokeKey("key-x", "plug-1") {
		t.Fatalf("the remove and revoke keying scopes are distinct")
	}
	// (b) The same client key carries two independent recorded outcomes —
	// no key collision across the classes (each class's effective key is
	// its own scope).
	if _, err := s.SubmitPluginInstall(ctx, pluginInstallIntent("plug-g09a-d", "key-g09a-d", "corr-g09a-d", "pkgref-g09a-d")); err != nil {
		t.Fatalf("fixture install (identities case): %v", err)
	}
	if _, err := s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-g09a-d", "key-d", "corr-d1", "revoke")); err != nil {
		t.Fatalf("the revocation under the shared client key: %v", err)
	}
	if _, err := s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-g09a-d", "key-d", "corr-d2", "remove")); err != nil {
		t.Fatalf("the removal under the SAME client key does not collide with the revocation's record (distinct keying scopes): %v", err)
	}
	// (c) The two intents each left their own DISTINCT journal trace on
	// the one plugin: one remove record + one revoke_grants record; one
	// plugin.removed event + one plugin.grants_revoked event.
	var dRemoveOps, dRevokeOps, dRemovedEvents, dRevokeEvents int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE plugin_id = ? AND kind = ?`, "plug-g09a-d", pluginKindRemove).Scan(&dRemoveOps); err != nil {
		t.Fatalf("distinct remove records: %v", err)
	}
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugin_operations WHERE plugin_id = ? AND kind = ?`, "plug-g09a-d", pluginKindRevokeGrants).Scan(&dRevokeOps)
	_ = s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = ?`, "plug-g09a-d", plugindomain.EventRemoved).Scan(&dRemovedEvents)
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM events_journal WHERE aggregate_id = ? AND event_type = ?`, "plug-g09a-d", plugindomain.EventGrantsRevoked).Scan(&dRevokeEvents); err != nil {
		t.Fatalf("distinct revoke events: %v", err)
	}
	if dRemoveOps != 1 || dRevokeOps != 1 || dRemovedEvents != 1 || dRevokeEvents != 1 {
		t.Fatalf("the two intents carry distinct operation identities: removeOps=%d revokeOps=%d removedEvents=%d revokeEvents=%d", dRemoveOps, dRevokeOps, dRemovedEvents, dRevokeEvents)
	}
	// (d) A post-removal revoke intent cannot resolve (the revoke's own
	// named object — the granted set of record on an installed artifact —
	// no longer exists once the artifact is removed); the typed 404.
	_, err = s.SubmitPluginRevokeGrants(ctx, pluginMutationIntent("plug-g09a-d", "rkey-post", "corr-post", "revoke"))
	if err == nil {
		t.Fatalf("a revoke intent on a removed artifact must refuse")
	} else if se, ok := err.(*Error); !ok || se.Code != errs.CodeNotFound {
		t.Fatalf("the post-removal revoke intent is the typed NOT_FOUND: %v", err)
	}
	// (e) A removal after a revocation still applies (the artifact is
	// still installed — grants-revoked is not trust-revoked): the removal
	// acts on its own named object.
	if _, err := s.SubmitPluginRemove(ctx, pluginMutationIntent("plug-g09a-v", "mkey-after-revoke", "corr-after-revoke", "remove")); err != nil {
		t.Fatalf("the removal after the revocation (the artifact still installed): %v", err)
	}
}
