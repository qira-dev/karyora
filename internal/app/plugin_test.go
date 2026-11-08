package app

import (
	stdcontext "context"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	plugindomain "github.com/qira-dev/karyora/internal/domain/plugin"
	"github.com/qira-dev/karyora/internal/storage"
)

// The B5 Plugin lifecycle family's service register (DE-0 queue row 8):
// the C1-07 install class through the service (the Core mints the plugin,
// package, install-record, and grant identities — the body never carries
// one; the replay returns the recorded install-record reference), the
// M-29 state-toggle member (ONE member, both directions), the M-30 update
// member (the fresh-review law; the typed refusal stores nothing), the
// M-31 remove and G7 revoke-grants members, the keyset list, the inspect
// and state reads, the authorization boundary through the one decision
// port, and the nil-port refusals (fail closed).

// newPluginEnv is newTestEnv with the plugin port wired to the same store
// (the test-side twin of the composition root's wiring).
func newPluginEnv(t testing.TB) (*testEnv, *Slice) {
	t.Helper()
	env := newTestEnv(t)
	cdeps := env.deps
	cdeps.Plugin = env.store
	cslice, err := NewSlice(cdeps)
	if err != nil {
		t.Fatalf("plugin slice: %v", err)
	}
	return env, cslice
}

// pluginFixture installs one plugin through the service (the Core-minted
// identities are the fixture's anchor).
func pluginFixture(t *testing.T, slice *Slice, ctx stdcontext.Context, key string) plugindomain.Plugin {
	t.Helper()
	res, err := slice.InstallPlugin(ctx, PluginInstallRequest{
		IdempotencyKey: "key-" + key, CorrelationID: "corr-" + key, PackageRef: "pkgref-" + key,
	})
	if err != nil {
		t.Fatalf("fixture install: %v", err)
	}
	if res.Replayed || res.InstallRecordRef == "" {
		t.Fatalf("fixture install result: %+v", res)
	}
	page, err := slice.Plugins(ctx, PluginListRequest{Limit: MaxListLimit})
	if err != nil {
		t.Fatalf("fixture list: %v", err)
	}
	for _, rec := range page.Plugins {
		if rec.InstallRecordRef == res.InstallRecordRef {
			return rec
		}
	}
	t.Fatalf("the fixture plugin is missing from the installed list")
	return plugindomain.Plugin{}
}

// TestPluginAuthzBoundary pins the authorization boundary: every B5
// surface refuses the session-less caller with the typed
// AUTHZ_DENIED/SESSION_INVALID envelope through the one decision port —
// and never the data. Local mode stays role-free (no permission policy is
// invented).
func TestPluginAuthzBoundary(t *testing.T) {
	env, slice := newPluginEnv(t)
	noSession := WithCaller(stdcontext.Background(), "local", "")
	cases := []struct {
		name string
		call func() error
	}{
		{"install", func() error {
			_, err := slice.InstallPlugin(noSession, PluginInstallRequest{IdempotencyKey: "k", CorrelationID: "c", PackageRef: "r"})
			return err
		}},
		{"toggle", func() error {
			_, err := slice.SetPluginEnabled(noSession, "plug-x", PluginEnableRequestBody{Enabled: true})
			return err
		}},
		{"update", func() error {
			_, err := slice.UpdatePlugin(noSession, "plug-x", PluginUpdateRequest{IdempotencyKey: "k", CorrelationID: "c", TargetVersionRef: "r"})
			return err
		}},
		{"remove", func() error {
			_, err := slice.RemovePlugin(noSession, "plug-x", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"revoke grants", func() error {
			_, err := slice.RevokePluginGrants(noSession, "plug-x", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"list", func() error { _, err := slice.Plugins(noSession, PluginListRequest{}); return err }},
		{"inspect", func() error { _, err := slice.PluginInspect(noSession, "plug-x"); return err }},
		{"state", func() error { _, err := slice.PluginState(noSession, "plug-x"); return err }},
	}
	for _, tc := range cases {
		err := tc.call()
		envl := assertKnowledgeEnvelope(t, err, errs.CodeAuthzDenied)
		if envl.Data["reason"] != "SESSION_INVALID" {
			t.Fatalf("%s: the session boundary refusal: %+v", tc.name, envl.Data)
		}
	}
	// One authenticated positive (the state read's typed 404 — the
	// boundary, not the router, answers, and the code is the envelope
	// register's typed carrier).
	_, err := slice.PluginState(env.ctx(), "plug-authz-ok")
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
}

// TestPluginInstallThroughService pins the C1-07 install class through the
// service: the Core mints the plugin, package, install-record, and grant
// identities — the body never carries one; the replay returns the recorded
// install-record reference, never a second one; the install lands the
// plugin in its installed NOT-yet-enabled state; the bounded-identity
// validation refuses unbounded members.
func TestPluginInstallThroughService(t *testing.T) {
	env, slice := newPluginEnv(t)
	ctx := env.ctx()

	// The fresh install: the Core-minted identities; the installed
	// NOT-yet-enabled state.
	res, err := slice.InstallPlugin(ctx, PluginInstallRequest{IdempotencyKey: "key-i1", CorrelationID: "corr-i1", PackageRef: "pkgref-i1"})
	if err != nil || res.Replayed || res.InstallRecordRef == "" {
		t.Fatalf("the fresh install: %v %+v", err, res)
	}
	page, err := slice.Plugins(ctx, PluginListRequest{})
	if err != nil || len(page.Plugins) != 1 {
		t.Fatalf("the installed list: %v %+v", err, page)
	}
	rec := page.Plugins[0]
	if rec.PluginID == "" || rec.LifecycleState != plugindomain.StateInstalled || rec.InstallRecordRef != res.InstallRecordRef {
		t.Fatalf("the installed record: %+v", rec)
	}
	if rec.Package == nil || rec.Package.PackageRef == "" {
		t.Fatalf("the Core-minted package identity: %+v", rec.Package)
	}

	// The keyed replay: the recorded install-record reference, never a
	// second one.
	replay, err := slice.InstallPlugin(ctx, PluginInstallRequest{IdempotencyKey: "key-i1", CorrelationID: "corr-i1", PackageRef: "pkgref-i1"})
	if err != nil || !replay.Replayed || replay.InstallRecordRef != res.InstallRecordRef {
		t.Fatalf("the replay: %v %+v", err, replay)
	}
	page, _ = slice.Plugins(ctx, PluginListRequest{})
	if len(page.Plugins) != 1 {
		t.Fatalf("a replay never installs a second plugin: %d", len(page.Plugins))
	}

	// The bounded-identity validation: an empty or over-bound member
	// refuses (the coded bounds class).
	long := make([]byte, maxPluginIDLen+1)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct{ name, key, corr, ref string }{
		{"empty key", "", "corr", "ref"},
		{"empty correlation", "key", "", "ref"},
		{"empty package ref", "key", "corr", ""},
		{"over-bound key", string(long), "corr", "ref"},
	}
	for _, tc := range cases {
		if _, err := slice.InstallPlugin(ctx, PluginInstallRequest{IdempotencyKey: tc.key, CorrelationID: tc.corr, PackageRef: tc.ref}); err == nil {
			t.Fatalf("%s: the bounded-identity validation must refuse", tc.name)
		} else if envl := assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed); envl.Data["field"] == "" {
			t.Fatalf("%s: the refusal names its field: %+v", tc.name, envl.Data)
		}
	}

	// The conflict table at the service boundary: the same key + a
	// different request identity rides the typed CONFLICT_IDEMPOTENCY_KEY.
	_, err = slice.InstallPlugin(ctx, PluginInstallRequest{IdempotencyKey: "key-i1", CorrelationID: "corr-OTHER", PackageRef: "pkgref-i1"})
	assertKnowledgeEnvelope(t, err, errs.CodeConflictIdempotencyKey)
}

// TestPluginToggleUpdateRemoveRevokeThroughService pins the mutation
// members through the service: the M-29 toggle's both directions and the
// revoked-state refusals (the distinctness laws); the M-30 update's
// fresh-review law (the typed refusal stores nothing and consumes no key);
// the M-31 remove; the G7 revoke-grants; the SAME typed 404 for unknown
// identities.
func TestPluginToggleUpdateRemoveRevokeThroughService(t *testing.T) {
	env, slice := newPluginEnv(t)
	ctx := env.ctx()

	plug := pluginFixture(t, slice, ctx, "mut")
	plugID := plug.PluginID

	// The M-29 toggle: both directions through the ONE member.
	out, err := slice.SetPluginEnabled(ctx, plugID, PluginEnableRequestBody{Enabled: true})
	if err != nil || out.Replayed || out.PluginID != plugID || out.OperationRef == "" {
		t.Fatalf("enable: %v %+v", err, out)
	}
	st, err := slice.PluginState(ctx, plugID)
	if err != nil || st.LifecycleState != plugindomain.StateEnabled {
		t.Fatalf("the enabled state: %v %+v", err, st)
	}
	out, err = slice.SetPluginEnabled(ctx, plugID, PluginEnableRequestBody{Enabled: false})
	if err != nil || out.Replayed {
		t.Fatalf("disable: %v %+v", err, out)
	}
	st, _ = slice.PluginState(ctx, plugID)
	if st.LifecycleState != plugindomain.StateDisabled {
		t.Fatalf("the disabled state (DISTINCT from the revoked states): %+v", st)
	}

	// The M-30 update through the service: fresh-review law (the typed
	// refusal stores nothing and consumes no key) — pinned at the service
	// boundary by the store-level behavior the port carries; the fresh
	// update succeeds here (the target's requested-of-record is EMPTY on
	// this Core — the no-fabricated-set law).
	out, err = slice.UpdatePlugin(ctx, plugID, PluginUpdateRequest{IdempotencyKey: "ukey-1", CorrelationID: "corr-u1", TargetVersionRef: "target-1"})
	if err != nil || out.Replayed {
		t.Fatalf("update: %v %+v", err, out)
	}
	replay, err := slice.UpdatePlugin(ctx, plugID, PluginUpdateRequest{IdempotencyKey: "ukey-1", CorrelationID: "corr-u1", TargetVersionRef: "target-1"})
	if err != nil || !replay.Replayed || replay.OperationRef != out.OperationRef {
		t.Fatalf("the update replay: %v %+v", err, replay)
	}

	// The G7 revoke-grants member: the granted set empties; the state moves
	// to grants-revoked (DISTINCT from disabled and trust-revoked).
	out, err = slice.RevokePluginGrants(ctx, plugID, PluginMutationRequestBody{IdempotencyKey: "rkey-1", CorrelationID: "corr-r1"})
	if err != nil || out.Replayed {
		t.Fatalf("revoke grants: %v %+v", err, out)
	}
	st, _ = slice.PluginState(ctx, plugID)
	if st.LifecycleState != plugindomain.StateGrantsRevoked {
		t.Fatalf("the grants-revoked state: %+v", st)
	}
	// The distinctness laws' consequence at the service boundary: the
	// toggle refusal from the revoked state rides the published register
	// code the state carries.
	_, err = slice.SetPluginEnabled(ctx, plugID, PluginEnableRequestBody{Enabled: true})
	assertKnowledgeEnvelope(t, err, errs.CodePluginCapabilityDenied)

	// The M-31 remove member: the record leaves the installed set; the
	// keyed replay returns the recorded outcome.
	out, err = slice.RemovePlugin(ctx, plugID, PluginMutationRequestBody{IdempotencyKey: "mkey-1", CorrelationID: "corr-m1"})
	if err != nil || out.Replayed {
		t.Fatalf("remove: %v %+v", err, out)
	}
	_, err = slice.PluginState(ctx, plugID)
	assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
	replay, err = slice.RemovePlugin(ctx, plugID, PluginMutationRequestBody{IdempotencyKey: "mkey-1", CorrelationID: "corr-m1"})
	if err != nil || !replay.Replayed {
		t.Fatalf("the remove replay: %v %+v", err, replay)
	}

	// The unknown identity: the SAME typed 404 on every {plugin_id}
	// surface.
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"toggle", func() error {
			_, err := slice.SetPluginEnabled(ctx, "plug-nope", PluginEnableRequestBody{Enabled: true})
			return err
		}},
		{"update", func() error {
			_, err := slice.UpdatePlugin(ctx, "plug-nope", PluginUpdateRequest{IdempotencyKey: "k", CorrelationID: "c", TargetVersionRef: "r"})
			return err
		}},
		{"remove", func() error {
			_, err := slice.RemovePlugin(ctx, "plug-nope", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"revoke", func() error {
			_, err := slice.RevokePluginGrants(ctx, "plug-nope", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"inspect", func() error { _, err := slice.PluginInspect(ctx, "plug-nope"); return err }},
		{"state", func() error { _, err := slice.PluginState(ctx, "plug-nope"); return err }},
	} {
		if err := tc.call(); err == nil {
			t.Fatalf("%s: the unknown identity must refuse", tc.name)
		} else {
			assertKnowledgeEnvelope(t, err, errs.CodeNotFound)
		}
	}
}

// TestPluginListReadsNilPort pins the list/reads class and the nil-port
// refusals: the keyset list with the cursor continuation; the inspect
// member's record; the nil port refuses EVERY surface with the typed
// fail-closed refusal (the KnowledgeStore/MemoryStore/ContextStore/
// WorkflowStore pattern).
func TestPluginListReadsNilPort(t *testing.T) {
	env, slice := newPluginEnv(t)
	ctx := env.ctx()

	// The fixture: three installed plugins.
	var lastCursor string
	for _, k := range []string{"l1", "l2", "l3"} {
		pluginFixture(t, slice, ctx, k)
	}
	// The bounded page and the cursor continuation.
	page, err := slice.Plugins(ctx, PluginListRequest{Limit: 2})
	if err != nil || len(page.Plugins) != 2 || page.Cursor == "" {
		t.Fatalf("the bounded page: %v %+v", err, page)
	}
	lastCursor = page.Cursor
	cont, err := slice.Plugins(ctx, PluginListRequest{Limit: 50, Cursor: lastCursor})
	if err != nil || len(cont.Plugins) != 1 || cont.Cursor != "" {
		t.Fatalf("the continuation page (the cursor's absence marks the final page): %v %+v", err, cont)
	}
	// A malformed cursor refuses (typed validation).
	if _, err := slice.Plugins(ctx, PluginListRequest{Limit: 50, Cursor: "not-a-cursor"}); err == nil {
		t.Fatalf("a malformed cursor must refuse")
	} else {
		assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	}
	// The out-of-bound limit refuses.
	if _, err := slice.Plugins(ctx, PluginListRequest{Limit: MaxListLimit + 1}); err == nil {
		t.Fatalf("the out-of-bound limit must refuse")
	} else {
		assertKnowledgeEnvelope(t, err, errs.CodeValidationFailed)
	}

	// The inspect member's record renders the Core-returned members.
	page, _ = slice.Plugins(ctx, PluginListRequest{Limit: 1})
	inspected, err := slice.PluginInspect(ctx, page.Plugins[0].PluginID)
	if err != nil || inspected.PluginID != page.Plugins[0].PluginID || inspected.LifecycleState != plugindomain.StateInstalled {
		t.Fatalf("the inspect member: %v %+v", err, inspected)
	}

	// The nil-port refusals: every surface refuses with the typed
	// fail-closed refusal when the port is not wired.
	nilDeps := env.deps
	nilDeps.Plugin = nil
	nilSlice, err := NewSlice(nilDeps)
	if err != nil {
		t.Fatalf("nil-port slice: %v", err)
	}
	authenticated := env.ctx()
	nilCases := []struct {
		name string
		call func() error
	}{
		{"install", func() error {
			_, err := nilSlice.InstallPlugin(authenticated, PluginInstallRequest{IdempotencyKey: "k", CorrelationID: "c", PackageRef: "r"})
			return err
		}},
		{"toggle", func() error {
			_, err := nilSlice.SetPluginEnabled(authenticated, "plug-x", PluginEnableRequestBody{Enabled: true})
			return err
		}},
		{"update", func() error {
			_, err := nilSlice.UpdatePlugin(authenticated, "plug-x", PluginUpdateRequest{IdempotencyKey: "k", CorrelationID: "c", TargetVersionRef: "r"})
			return err
		}},
		{"remove", func() error {
			_, err := nilSlice.RemovePlugin(authenticated, "plug-x", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"revoke", func() error {
			_, err := nilSlice.RevokePluginGrants(authenticated, "plug-x", PluginMutationRequestBody{IdempotencyKey: "k", CorrelationID: "c"})
			return err
		}},
		{"list", func() error { _, err := nilSlice.Plugins(authenticated, PluginListRequest{}); return err }},
		{"inspect", func() error { _, err := nilSlice.PluginInspect(authenticated, "plug-x"); return err }},
		{"state", func() error { _, err := nilSlice.PluginState(authenticated, "plug-x"); return err }},
	}
	for _, tc := range nilCases {
		if err := tc.call(); err == nil {
			t.Fatalf("%s: the nil port must refuse (fail closed)", tc.name)
		} else {
			assertKnowledgeEnvelope(t, err, errs.CodeStorageUnavailable)
		}
	}
}

// TestPluginStoreImplementsPort pins the port implementation at compile
// time (the storage adapter implements the plugin port; the slice consumes
// only the port).
func TestPluginStoreImplementsPort(t *testing.T) {
	var _ PluginStore = (*storage.Store)(nil)
	_ = time.Now // keep the import honest if assertions change
}
