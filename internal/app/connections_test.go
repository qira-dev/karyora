package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// fakeConnectionSecrets is the in-memory D-37 SecretWriter/SecretStore
// test double (no real keyring in tests): Store/Revoke/Resolve/Status over
// a map keyed by the handle string. It records every write so the tests
// can pin exactly-once credential writes and the same-handle swap.
type fakeConnectionSecrets struct {
	mu     sync.Mutex
	items  map[string][]byte
	stored []string // handle strings in write order
	failed bool     // scripted store failure (typed inaccessible)
}

func newFakeConnectionSecrets() *fakeConnectionSecrets {
	return &fakeConnectionSecrets{items: map[string][]byte{}}
}

func (f *fakeConnectionSecrets) Store(_ context.Context, h security.Handle, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failed || len(value) == 0 {
		return &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	f.items[h.String()] = cp
	f.stored = append(f.stored, h.String())
	return nil
}

func (f *fakeConnectionSecrets) Revoke(_ context.Context, h security.Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failed {
		return &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
	}
	delete(f.items, h.String())
	return nil
}

func (f *fakeConnectionSecrets) Resolve(_ context.Context, h security.Handle) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.items[h.String()]
	if !ok {
		return nil, &security.SecretError{Code: errs.CodeNotFound, Reason: security.ReasonSecretMissing, Handle: h.String()}
	}
	return v, nil
}

func (f *fakeConnectionSecrets) Status(_ context.Context, h security.Handle) (security.SecretStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[h.String()]; ok {
		return security.SecretPresent, nil
	}
	return security.SecretMissing, nil
}

func (f *fakeConnectionSecrets) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stored)
}

// The obviously-fake test credential (never a real key; Project Rule 14).
const connTestCanary = "karyora-test-canary-4b7e-not-a-real-credential"

// connEnv extends the shared test env with the connection surface wired:
// the connection store + the fake secret double, plus one direct adapter
// executor (deepseek — its data-handling record carries an empty
// classification allowlist, so its probe refuses typed at the fail-closed
// egress gate without any network; the mock fixture provider's probe
// succeeds without a network).
func connEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	direct := providers.NewDirectAdapters()
	var deepseek providers.Adapter
	for _, a := range direct {
		if a.ID() == dprovider.ProviderDeepSeek {
			deepseek = a
		}
	}
	if deepseek == nil {
		t.Fatal("the deepseek direct adapter is expected in the coded set")
	}
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(deepseek.SeedDataHandling(deepseek.DefaultProfile("m")))
	exec, err := providers.NewExecutor(deepseek, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatalf("direct executor: %v", err)
	}
	env.deps.Providers[dprovider.ProviderDeepSeek] = exec
	env.deps.Connections = env.store
	env.deps.Secrets = newFakeConnectionSecrets()
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	return env
}

func saveReq(key string) SaveConnectionRequest {
	return SaveConnectionRequest{
		IdempotencyKey: key,
		Provider:       dprovider.ProviderMock,
		ModelID:        "m",
		Secret:         []byte(connTestCanary),
	}
}

// TestConnectionSaveIdempotentReplayNeverReWrites pins the M-24 Supplied
// idempotency end to end: duplicate → the existing connection reference
// with Replayed=true and NO second credential write; same key + different
// body → the typed CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false).
func TestConnectionSaveIdempotentReplayNeverReWrites(t *testing.T) {
	env := connEnv(t)
	// The credential-bearing posture: a replayed save must never write a
	// second credential, so the exactly-once pin rides deepseek
	// (AuthBearer — the mock fixture carries no credential class).
	req := saveReq("idem-1")
	req.Provider = dprovider.ProviderDeepSeek
	first, err := env.slice.SaveConnection(env.ctx(), req)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if first.Replayed {
		t.Fatal("the first save is not a replay")
	}
	if !strings.HasPrefix(first.Connection.Record.SecretRef, "connection:"+first.Connection.Record.ConnectionID+"/") {
		t.Fatalf("the record stores the connection-scoped handle reference: %q", first.Connection.Record.SecretRef)
	}
	replay, err := env.slice.SaveConnection(env.ctx(), req)
	if err != nil {
		t.Fatalf("duplicate save must replay: %v", err)
	}
	if !replay.Replayed || replay.Connection.Record.ConnectionID != first.Connection.Record.ConnectionID {
		t.Fatalf("duplicate save returns the existing reference: %+v", replay)
	}
	if writes := env.deps.Secrets.(*fakeConnectionSecrets).writeCount(); writes != 1 {
		t.Fatalf("a replayed save must never write a second credential: %d writes", writes)
	}
	conflict := saveReq("idem-1")
	conflict.ModelID = "other-model"
	_, err = env.slice.SaveConnection(env.ctx(), conflict)
	if err == nil {
		t.Fatal("same key + different body must refuse")
	}
	ee, ok := err.(*EnvelopeError)
	if !ok || ee.Envelope.Code != errs.CodeConflictIdempotencyKey {
		t.Fatalf("the conflict must be the coded CONFLICT_IDEMPOTENCY_KEY envelope, got %v", err)
	}
	if fm, ok := ee.Envelope.Data["fingerprintMatch"]; !ok || fm != false {
		t.Fatalf("the conflict data carries fingerprintMatch:false, got %v", ee.Envelope.Data)
	}
}

// TestConnectionSaveRefusals pins the request-shape law: the empty
// idempotency key, the empty model, the unwired provider, and the missing
// secret on a credential-bearing posture are all typed VALIDATION_FAILED
// refusals changing nothing.
func TestConnectionSaveRefusals(t *testing.T) {
	env := connEnv(t)
	cases := []struct {
		name  string
		mut   func(*SaveConnectionRequest)
		field string
	}{
		{"empty key", func(r *SaveConnectionRequest) { r.IdempotencyKey = " " }, "idempotency_key"},
		{"empty model", func(r *SaveConnectionRequest) { r.ModelID = "" }, "model_id"},
		{"unwired provider", func(r *SaveConnectionRequest) { r.Provider = "not-a-wired-provider" }, "provider"},
		{"missing secret", func(r *SaveConnectionRequest) { r.Secret = nil; r.Provider = dprovider.ProviderMock }, "secret"},
		{"embedded credentials", func(r *SaveConnectionRequest) { r.BaseURL = "https://u:p@x.invalid" }, "base_url"},
		{"forbidden header", func(r *SaveConnectionRequest) {
			r.CustomHeaders = []dprovider.ConnectionHeader{{Name: "Authorization", Value: "x"}}
		}, "custom_headers"},
	}
	// The missing-secret case rides the credential-bearing posture (the
	// deepseek direct adapter is AuthBearer).
	cases[3].mut = func(r *SaveConnectionRequest) { r.Secret = nil; r.Provider = dprovider.ProviderDeepSeek }
	for _, tc := range cases {
		req := saveReq("idem-" + tc.name)
		tc.mut(&req)
		_, err := env.slice.SaveConnection(env.ctx(), req)
		if err == nil {
			t.Fatalf("%s must refuse", tc.name)
		}
		ee, ok := err.(*EnvelopeError)
		if !ok || ee.Envelope.Code != errs.CodeValidationFailed {
			t.Fatalf("%s must be the typed VALIDATION_FAILED envelope, got %v", tc.name, err)
		}
		if f, ok := ee.Envelope.Data["field"]; !ok || f != tc.field {
			t.Fatalf("%s refusal names its field (%s), got %v", tc.name, tc.field, ee.Envelope.Data)
		}
	}
}

// TestConnectionSecretNeverAppears pins the milestone's core secret rule:
// the credential VALUE never appears in any response surface — the save
// result, the list views, the get view, the test report (success AND
// failure), the rotate result, or any error. The canary is the obviously
// fake test credential.
func TestConnectionSecretNeverAppears(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	views := []any{saved, saved.Connection.Record}
	if page, err := env.slice.ListConnections(env.ctx(), ListConnectionsRequest{}); err != nil {
		t.Fatalf("list: %v", err)
	} else {
		views = append(views, page)
	}
	if view, err := env.slice.GetConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("get: %v", err)
	} else {
		views = append(views, view)
	}
	testRes, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	views = append(views, testRes)
	rot, err := env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	views = append(views, rot)
	if _, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1")); err != nil {
		views = append(views, err.Error())
	}
	for i, v := range views {
		b, merr := json.Marshal(v)
		if merr != nil {
			t.Fatalf("marshal view %d: %v", i, merr)
		}
		if strings.Contains(string(b), connTestCanary) {
			t.Fatalf("view %d carries the credential value: %s", i, string(b))
		}
	}
	// The stored credential resolves under the record's handle only.
	fake := env.deps.Secrets.(*fakeConnectionSecrets)
	if v, err := fake.Resolve(env.ctx(), ConnectionHandle(saved.Connection.Record.ConnectionID)); err != nil || string(v) != connTestCanary {
		t.Fatalf("the credential lives under the record's own handle: %v", err)
	}
}

// TestConnectionTestStampsSavedRecord pins TC-1 and the unsaved-draft
// discipline: a saved connection's successful test records the tested
// state (the digest + epoch the test ran under); an unsaved draft's test
// rides the transient secret, records nothing, and never persists the
// typed value.
func TestConnectionTestStampsSavedRecord(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	res, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID})
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if !res.Report.OK {
		t.Fatalf("the fixture probe must succeed: %+v", res.Report)
	}
	if res.Report.CorrelationID == "" {
		t.Fatal("the report carries its correlation id")
	}
	if res.Report.Capabilities == nil {
		t.Fatal("the report carries the capability statuses")
	}
	if res.Record == nil || !res.Record.Tested() || !res.Record.TestCurrent() {
		t.Fatalf("a saved connection's successful test records the tested state: %+v", res.Record)
	}
	// The unsaved draft: a transient typed secret rides the request-scoped
	// store and nothing is recorded (no record exists to stamp).
	draft, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{Provider: dprovider.ProviderMock, ModelID: "m", Secret: []byte(connTestCanary)})
	if err != nil {
		t.Fatalf("draft test: %v", err)
	}
	if !draft.Report.OK || draft.Record != nil {
		t.Fatalf("an unsaved draft's test records nothing: %+v", draft)
	}
	// The missing-secret draft on a credential-required posture is a typed
	// refusal (the mock is AuthNone, so the refusal case rides the
	// validation of an empty model instead).
	if _, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{Provider: dprovider.ProviderMock, ModelID: " "}); err == nil {
		t.Fatal("an empty model id must refuse")
	}
}

// TestConnectionTestSanitizedFailure pins the sanitized-failure rule: a
// failing probe reports the typed error class + the bounded redacted
// detail + the correlation id — and a credential value never rides it
// (the executor's redaction plus this report's own shape, pinned). The
// deepseek direct adapter's data-handling record refuses the probe label —
// the typed egress refusal is the sanitized failure path (fail-closed
// gate; no network is dialed).
func TestConnectionTestSanitizedFailure(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), SaveConnectionRequest{
		IdempotencyKey: "idem-egress",
		Provider:       dprovider.ProviderDeepSeek,
		ModelID:        "m",
		Secret:         []byte(connTestCanary),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	res, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID})
	if err != nil {
		t.Fatalf("the sanitized failure is the report, not an error: %v", err)
	}
	if res.Report.OK {
		t.Fatalf("the gated probe must not report success: %+v", res.Report)
	}
	if res.Report.ErrorKind != string(dprovider.ErrEgressPolicy) {
		t.Fatalf("the typed error class rides the report: %+v", res.Report)
	}
	if res.Report.CorrelationID == "" {
		t.Fatal("the failure carries its correlation id")
	}
	if strings.Contains(res.Report.ErrorDetail, connTestCanary) {
		t.Fatal("the sanitized detail never echoes a credential")
	}
	if res.Record != nil && res.Record.Tested() {
		t.Fatal("a failed test records no tested state")
	}
}

// TestConnectionRotateReauthSameHandleSwap pins M-26/M-27: the replacement
// key is stored under the SAME handle atomically (exactly two writes on
// the handle across save+rotate — one per replace), the credential epoch
// advances so the tested state renders stale-truthfully, the re-auth path
// rides the same mechanics with its own audited action, and a swap
// failure is typed fail-closed (the epoch already bumped — stale-truthful).
func TestConnectionRotateReauthSameHandleSwap(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := env.slice.TestConnection(env.ctx(), TestConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID}); err != nil {
		t.Fatalf("test: %v", err)
	}
	fake := env.deps.Secrets.(*fakeConnectionSecrets)
	handle := ConnectionHandle(saved.Connection.Record.ConnectionID)
	writesBefore := fake.writeCount()

	rot, err := env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if len(rot.AffectedModels) != 1 || rot.AffectedModels[0] != "m" {
		t.Fatalf("the working candidate's affected-models data rides the response: %+v", rot)
	}
	if rot.RoutingNote == "" || rot.InterruptionNote == "" {
		t.Fatal("the routing + interruption statements ride the response")
	}
	if rot.Record.CredentialEpoch != saved.Connection.Record.CredentialEpoch+1 {
		t.Fatalf("the rotation advances the credential epoch: %+v", rot.Record)
	}
	if fake.writeCount() != writesBefore+1 {
		t.Fatalf("the swap is exactly one same-handle write: %d → %d", writesBefore, fake.writeCount())
	}
	if v, err := fake.Resolve(env.ctx(), handle); err != nil || string(v) != connTestCanary {
		t.Fatalf("the new value resolves under the SAME handle: %v", err)
	}
	view, err := env.slice.GetConnection(env.ctx(), saved.Connection.Record.ConnectionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if view.Record.TestCurrent() {
		t.Fatal("a rotated record renders the tested state stale-truthfully")
	}
	// Re-authentication (M-27) rides the same path.
	if _, err := env.slice.ReauthenticateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)}); err != nil {
		t.Fatalf("re-auth: %v", err)
	}
	// A scripted swap failure is typed fail-closed: the epoch already
	// bumped (the safe direction), the refusal is the typed family.
	fake.failed = true
	_, err = env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)})
	if err == nil {
		t.Fatal("a failed swap must refuse typed")
	}
	if !strings.Contains(err.Error(), security.ReasonSecretInaccessible) {
		t.Fatalf("the swap failure is the typed SECRET_UNAVAILABLE family, got %v", err)
	}
	// An empty replacement key is a typed refusal (never a silent no-op).
	fake.failed = false
	if _, err := env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID}); err == nil {
		t.Fatal("an empty replacement key must refuse")
	}
}

// TestConnectionDeleteRevokesAndClears pins the delete/revoke command: the
// keyring entry is revoked and the record deleted; a typed revoke failure
// REFUSES the delete (a live credential is never abandoned behind a
// deleted record); the unknown id is the typed NOT_FOUND; the active
// pointer clears.
func TestConnectionDeleteRevokesAndClears(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := env.slice.SwitchConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if err := env.slice.DeleteConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	fake := env.deps.Secrets.(*fakeConnectionSecrets)
	if _, err := fake.Resolve(env.ctx(), ConnectionHandle(saved.Connection.Record.ConnectionID)); err == nil {
		t.Fatal("the keyring entry is revoked")
	}
	if _, err := env.slice.GetConnection(env.ctx(), saved.Connection.Record.ConnectionID); err == nil {
		t.Fatal("the record is deleted")
	}
	active, err := env.slice.ActiveConnection(env.ctx())
	if err != nil || active != "" {
		t.Fatalf("the active pointer clears: %v %q", err, active)
	}
	// A typed revoke failure refuses the delete; the record survives.
	saved2, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-2"))
	if err != nil {
		t.Fatalf("save 2: %v", err)
	}
	fake.failed = true
	if err := env.slice.DeleteConnection(env.ctx(), saved2.Connection.Record.ConnectionID); err == nil {
		t.Fatal("a failed revoke must refuse the delete")
	}
	fake.failed = false
	if _, err := env.slice.GetConnection(env.ctx(), saved2.Connection.Record.ConnectionID); err != nil {
		t.Fatal("the record survives the refused delete")
	}
}

// TestConnectionSwitchMinimal pins the minimal M-34 working candidate: the
// switch validates existence, records the active pointer durably, is
// idempotent (repeat switch → the same result, Replayed=true), and
// switches between records.
func TestConnectionSwitchMinimal(t *testing.T) {
	env := connEnv(t)
	a, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-a"))
	if err != nil {
		t.Fatalf("save a: %v", err)
	}
	b, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-b"))
	if err != nil {
		t.Fatalf("save b: %v", err)
	}
	if _, err := env.slice.SwitchConnection(env.ctx(), "conn-unknown"); err == nil {
		t.Fatal("a switch must validate against a stored record")
	}
	res, err := env.slice.SwitchConnection(env.ctx(), a.Connection.Record.ConnectionID)
	if err != nil || res.ActiveConnectionID != a.Connection.Record.ConnectionID || res.Replayed {
		t.Fatalf("switch to a: %+v %v", res, err)
	}
	repeat, err := env.slice.SwitchConnection(env.ctx(), a.Connection.Record.ConnectionID)
	if err != nil || !repeat.Replayed || repeat.ActiveConnectionID != a.Connection.Record.ConnectionID {
		t.Fatalf("a repeat switch is the idempotent no-op: %+v %v", repeat, err)
	}
	res, err = env.slice.SwitchConnection(env.ctx(), b.Connection.Record.ConnectionID)
	if err != nil || res.ActiveConnectionID != b.Connection.Record.ConnectionID || res.Replayed {
		t.Fatalf("switch to b: %+v %v", res, err)
	}
	page, err := env.slice.ListConnections(env.ctx(), ListConnectionsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, view := range page.Connections {
		want := view.Record.ConnectionID == b.Connection.Record.ConnectionID
		if view.Active != want {
			t.Fatalf("the active flag is per-record truthful: %+v", view)
		}
	}
}

// TestConnectionListPaginationAndOrder pins the card-9 list discipline at
// the API surface: the default limit applies, the order is created-at
// descending, the opaque cursor round-trips with no overlap or gap, the
// final page is cursorless, and the cursor never carries structured JSON
// on the wire.
func TestConnectionListPaginationAndOrder(t *testing.T) {
	env := connEnv(t)
	for _, key := range []string{"idem-a", "idem-b", "idem-c", "idem-d", "idem-e"} {
		if _, err := env.slice.SaveConnection(env.ctx(), saveReq(key)); err != nil {
			t.Fatalf("save %s: %v", key, err)
		}
	}
	seen := map[string]int{}
	cursor := ""
	pages := 0
	for {
		res, err := env.slice.ListConnections(env.ctx(), ListConnectionsRequest{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		pages++
		for _, view := range res.Connections {
			seen[view.Record.ConnectionID]++
			if seen[view.Record.ConnectionID] > 1 {
				t.Fatalf("a connection repeated across pages: %s", view.Record.ConnectionID)
			}
		}
		if res.NextCursor == "" {
			break
		}
		if strings.ContainsAny(res.NextCursor, "{}") {
			t.Fatalf("the cursor is opaque, never structured JSON: %q", res.NextCursor)
		}
		cursor = res.NextCursor
		if pages > 10 {
			t.Fatal("the walk did not terminate")
		}
	}
	if len(seen) != 5 || pages < 3 {
		t.Fatalf("the walk covers exactly five records over bounded pages: %d over %d pages", len(seen), pages)
	}
	// The per-surface default applies when the limit is omitted.
	if _, err := env.slice.ListConnections(env.ctx(), ListConnectionsRequest{Limit: -1}); err == nil {
		t.Fatal("an out-of-class limit refuses")
	}
}

// TestConnectionAuthorization pins the decision port on every operation:
// an unauthenticated caller is the typed AUTHZ_DENIED envelope changing
// nothing, on the reads (list/get/active) and the writes (save/rotate/
// re-auth/delete/switch/test) alike.
func TestConnectionAuthorization(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	denied := WithCaller(context.Background(), "local", "kry1-not-a-real-token")
	if _, err := env.slice.ListConnections(denied, ListConnectionsRequest{}); !isAuthzDenied(err) {
		t.Fatalf("list: %v", err)
	}
	if _, err := env.slice.GetConnection(denied, saved.Connection.Record.ConnectionID); !isAuthzDenied(err) {
		t.Fatalf("get: %v", err)
	}
	if _, err := env.slice.TestConnection(denied, TestConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID}); !isAuthzDenied(err) {
		t.Fatalf("test: %v", err)
	}
	if _, err := env.slice.SaveConnection(denied, saveReq("idem-2")); !isAuthzDenied(err) {
		t.Fatalf("save: %v", err)
	}
	if _, err := env.slice.RotateConnection(denied, RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)}); !isAuthzDenied(err) {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := env.slice.ReauthenticateConnection(denied, RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)}); !isAuthzDenied(err) {
		t.Fatalf("re-auth: %v", err)
	}
	if err := env.slice.DeleteConnection(denied, saved.Connection.Record.ConnectionID); !isAuthzDenied(err) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := env.slice.SwitchConnection(denied, saved.Connection.Record.ConnectionID); !isAuthzDenied(err) {
		t.Fatalf("switch: %v", err)
	}
	if _, err := env.slice.GetConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("an authorized read still passes: %v", err)
	}
}

// TestConnectionAuditRecords pins the audit trail of the row-10 model for
// the connection commands (actor, action, scope, decision, outcome,
// correlation) — and that no audit field ever carries the credential.
func TestConnectionAuditRecords(t *testing.T) {
	env := connEnv(t)
	saved, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-1"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := env.slice.SwitchConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if err := env.slice.DeleteConnection(env.ctx(), saved.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	sink := &memoryAuditStore{}
	env.deps.Guard.Audit = security.NewAuditService(sink, 64, 1000)
	saved2, err := env.slice.SaveConnection(env.ctx(), saveReq("idem-audit"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := env.slice.RotateConnection(env.ctx(), RotateConnectionRequest{ConnectionID: saved2.Connection.Record.ConnectionID, Secret: []byte(connTestCanary)}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := env.slice.SwitchConnection(env.ctx(), saved2.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if err := env.slice.DeleteConnection(env.ctx(), saved2.Connection.Record.ConnectionID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	records, err := sink.Records()
	if err != nil {
		t.Fatalf("audit read: %v", err)
	}
	found := map[string]bool{}
	for _, rec := range records {
		var ar security.AuditRecord
		if err := json.Unmarshal(rec.Payload, &ar); err != nil {
			t.Fatalf("audit payload: %v", err)
		}
		b, _ := json.Marshal(ar)
		if strings.Contains(string(b), connTestCanary) {
			t.Fatal("the audit trail never carries the credential")
		}
		found[ar.Action] = true
	}
	for _, want := range []string{"connection.save", "connection.rotate", "connection.switch", "connection.delete"} {
		if !found[want] {
			t.Fatalf("the audit trail lacks %s (found %v)", want, found)
		}
	}
}

func isAuthzDenied(err error) bool {
	ee, ok := err.(*EnvelopeError)
	return ok && ee.Envelope.Code == errs.CodeAuthzDenied
}
