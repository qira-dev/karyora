package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// fakeSaveBody is one connection-save form (the fingerprint rides the
// create form; the credential bytes themselves never reach this store).
type fakeSaveBody struct {
	Provider string
	BaseURL  string
	ModelID  string
	Secret   []byte
}

func fakeRecord(t *testing.T, s *Store, ctx context.Context, key string, body fakeSaveBody) dprovider.ConnectionRecord {
	t.Helper()
	rec := dprovider.ConnectionRecord{
		ConnectionID:   "conn-" + key,
		IdempotencyKey: key,
		Provider:       body.Provider,
		BaseURL:        body.BaseURL,
		ModelID:        body.ModelID,
		SecretRef:      "connection:conn-" + key + "/api-key",
		Fingerprint:    dprovider.FingerprintOf(body.Provider, body.BaseURL, body.ModelID, nil, body.Secret),
	}
	saved, _, err := s.SaveConnection(ctx, rec)
	if err != nil {
		t.Fatalf("save connection: %v", err)
	}
	return saved
}

func newConnTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, Options{
		Path:                   t.TempDir() + "/karyora.db",
		PoolMaxOpen:            2,
		WriterQueueDepth:       4,
		WriterDeadline:         2 * time.Second,
		BusyTimeout:            200 * time.Millisecond,
		WALAutocheckpointPages: 1000,
		Synchronous:            "NORMAL",
		ReadBudget:             2 * time.Second,
		MaxObjectBytes:         1 << 20,
		MaxObjects:             1000,
		GCGracePeriod:          time.Second,
		PublishInterval:        10 * time.Millisecond,
		PublishBatch:           64,
		Retention:              24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, ctx
}

// TestConnectionSaveIdempotency pins the Supplied create idempotency
// (P3.2 §3.3; C1-07): duplicate → the existing connection reference (no
// second record); same key + different body → the typed
// CONFLICT_IDEMPOTENCY_KEY (fingerprintMatch:false rides the app layer's
// data; the store's refusal is the coded conflict).
func TestConnectionSaveIdempotency(t *testing.T) {
	st, ctx := newConnTestStore(t)
	body := fakeSaveBody{Provider: "mock", ModelID: "m", Secret: []byte("karyora-test-canary-fake-key")}
	first := fakeRecord(t, st, ctx, "idem-1", body)
	replay := dprovider.ConnectionRecord{
		ConnectionID:   "conn-other-mint",
		IdempotencyKey: "idem-1",
		Provider:       body.Provider,
		ModelID:        body.ModelID,
		SecretRef:      "connection:conn-other-mint/api-key",
		Fingerprint:    dprovider.FingerprintOf(body.Provider, body.BaseURL, body.ModelID, nil, body.Secret),
	}
	saved, replayed, err := st.SaveConnection(ctx, replay)
	if err != nil {
		t.Fatalf("duplicate save must replay: %v", err)
	}
	if !replayed || saved.ConnectionID != first.ConnectionID {
		t.Fatalf("duplicate save must return the existing connection reference (replayed=%v id=%s)", replayed, saved.ConnectionID)
	}
	different := replay
	different.Fingerprint = dprovider.FingerprintOf(body.Provider, body.BaseURL, "other-model", nil, body.Secret)
	if _, _, err := st.SaveConnection(ctx, different); err == nil {
		t.Fatal("same key + different body must refuse")
	} else if !strings.Contains(err.Error(), string(errs.CodeConflictIdempotencyKey)) {
		t.Fatalf("the conflict must be the coded CONFLICT_IDEMPOTENCY_KEY, got %v", err)
	}
	// Exactly one record exists under the key.
	found, exists, err := st.LoadConnectionByIdempotencyKey(ctx, "idem-1")
	if err != nil || !exists || found.ConnectionID != first.ConnectionID {
		t.Fatalf("the key resolves to the original record: %v %v %+v", err, exists, found)
	}
}

// TestConnectionListKeysetPagination pins the card-9 list discipline over
// the connection records: created_at descending, connection_id tiebreak,
// the limit+1 More probe, no overlap or gap across pages, and the
// fail-closed guards.
func TestConnectionListKeysetPagination(t *testing.T) {
	st, ctx := newConnTestStore(t)
	// Save five connections with distinct instants (stamp each one's
	// created_at by saving sequentially and nudging updated ordering via
	// distinct model ids — the storage stamps the wall clock; within the
	// same nanosecond resolution the connection_id tiebreak keeps the walk
	// deterministic).
	for i := 0; i < 5; i++ {
		fakeRecord(t, st, ctx, "idem-"+string(rune('a'+i)), fakeSaveBody{Provider: "mock", ModelID: "m"})
	}
	seen := map[string]bool{}
	total := 0
	cursor := ""
	for page := 0; page < 10; page++ {
		q := ListConnectionsQuery{Limit: 2}
		if cursor != "" {
			parts := splitCursor(t, cursor)
			q.AfterCreatedAt, q.AfterConnectionID = parts[0], parts[1]
		}
		res, err := st.ListConnections(ctx, q)
		if err != nil {
			t.Fatalf("list page %d: %v", page, err)
		}
		total += len(res.Connections)
		for _, rec := range res.Connections {
			if seen[rec.ConnectionID] {
				t.Fatalf("page %d repeats %s", page, rec.ConnectionID)
			}
			seen[rec.ConnectionID] = true
		}
		if !res.More {
			break
		}
		last := res.Connections[len(res.Connections)-1]
		cursor = encodeConnTestCursor(last)
	}
	if total != 5 || len(seen) != 5 {
		t.Fatalf("the walk must cover exactly the five records: total=%d seen=%d", total, len(seen))
	}
	if _, err := st.ListConnections(ctx, ListConnectionsQuery{Limit: 0}); err == nil {
		t.Error("a non-positive limit must refuse")
	}
	if _, err := st.ListConnections(ctx, ListConnectionsQuery{Limit: 2, AfterCreatedAt: "2026-11-02T00:00:00Z"}); err == nil {
		t.Error("a keyset position without its tiebreak must refuse")
	}
}

// splitCursor/encodeConnTestCursor mirror the app-layer cursor envelope
// (a salt-prefixed hex-JSON envelope of the keyset position — RFC 3339
// instants carry dots, so the wire form is the JSON envelope, never a raw
// join) for the store-level walk.
func splitCursor(t *testing.T, cursor string) [2]string {
	t.Helper()
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("cursor is malformed: %q", cursor)
	}
	raw, err := hex.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("cursor payload: %v", err)
	}
	var pos struct {
		CreatedAt    string `json:"created_at"`
		ConnectionID string `json:"connection_id"`
	}
	if err := json.Unmarshal(raw, &pos); err != nil {
		t.Fatalf("cursor JSON: %v", err)
	}
	return [2]string{pos.CreatedAt, pos.ConnectionID}
}

func encodeConnTestCursor(rec dprovider.ConnectionRecord) string {
	b, _ := json.Marshal(struct {
		CreatedAt    string `json:"created_at"`
		ConnectionID string `json:"connection_id"`
	}{rec.CreatedAt, rec.ConnectionID})
	salt := []byte{0x01, 0x02, 0x03, 0x04}
	return hex.EncodeToString(salt) + "." + hex.EncodeToString(b)
}

// TestConnectionTestStampAndCredentialBump pins the tested-state stamping
// (TC-1) and the rotation marker: a successful test stamps the tested
// digest + epoch; a credential bump advances the epoch so the tested
// result renders not-current truthfully; a failed test records nothing
// (the caller simply never stamps).
func TestConnectionTestStampAndCredentialBump(t *testing.T) {
	st, ctx := newConnTestStore(t)
	rec := fakeRecord(t, st, ctx, "idem-1", fakeSaveBody{Provider: "mock", ModelID: "m"})
	if rec.Tested() {
		t.Fatal("a fresh record is untested")
	}
	stamped, err := st.RecordConnectionTest(ctx, rec.ConnectionID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if !stamped.Tested() || !stamped.TestCurrent() {
		t.Fatalf("a stamped record is tested and current: %+v", stamped)
	}
	if stamped.TestedDigest != stamped.InputsDigest() || stamped.TestedEpoch != stamped.CredentialEpoch {
		t.Fatalf("the stamp must carry the current inputs digest and epoch: %+v", stamped)
	}
	bumped, err := st.BumpConnectionCredential(ctx, rec.ConnectionID)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	if bumped.CredentialEpoch != stamped.CredentialEpoch+1 {
		t.Fatalf("the bump advances the epoch: %+v", bumped)
	}
	if bumped.TestCurrent() {
		t.Fatal("a bumped record renders the tested state not-current")
	}
	if _, err := st.RecordConnectionTest(ctx, "conn-unknown", time.Now().UTC().Format(time.RFC3339Nano)); err == nil {
		t.Fatal("an unknown connection cannot be stamped")
	}
}

// TestConnectionDeleteAndActivePointer pins deletion and the minimal
// M-34 switch state: the active pointer validates against a stored
// record, the switch is idempotent, deleting the active record clears the
// pointer, and an unknown delete is the typed NOT_FOUND.
func TestConnectionDeleteAndActivePointer(t *testing.T) {
	st, ctx := newConnTestStore(t)
	rec := fakeRecord(t, st, ctx, "idem-1", fakeSaveBody{Provider: "mock", ModelID: "m"})
	if _, err := st.SetActiveConnection(ctx, "conn-unknown"); err == nil {
		t.Fatal("a switch must validate against a stored record")
	}
	active, err := st.SetActiveConnection(ctx, rec.ConnectionID)
	if err != nil || active != rec.ConnectionID {
		t.Fatalf("switch: %v %s", err, active)
	}
	current, err := st.ActiveConnectionID(ctx)
	if err != nil || current != rec.ConnectionID {
		t.Fatalf("active read: %v %s", err, current)
	}
	if _, err := st.DeleteConnection(ctx, "conn-unknown"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("an unknown delete is the typed NOT_FOUND, got %v", err)
	}
	deleted, err := st.DeleteConnection(ctx, rec.ConnectionID)
	if err != nil || deleted.ConnectionID != rec.ConnectionID {
		t.Fatalf("delete: %v %+v", err, deleted)
	}
	current, err = st.ActiveConnectionID(ctx)
	if err != nil || current != "" {
		t.Fatalf("deleting the active record clears the pointer: %v %q", err, current)
	}
}

// TestConnectionRecordHasNoSecretValue pins the store's secret rule with
// a canary: the credential VALUE never enters the stored row, the loaded
// record, the list results, or any store error (the store handles only
// the fingerprint digest and the reference).
func mustJSONConn(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestConnectionRecordHasNoSecretValue(t *testing.T) {
	st, ctx := newConnTestStore(t)
	const canary = "karyora-test-canary-9f2c-not-a-real-key"
	rec := fakeRecord(t, st, ctx, "idem-1", fakeSaveBody{Provider: "mock", ModelID: "m", Secret: []byte(canary)})
	loaded, err := st.LoadConnection(ctx, rec.ConnectionID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	page, err := st.ListConnections(ctx, ListConnectionsQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, view := range append([]dprovider.ConnectionRecord{loaded, rec}, page.Connections...) {
		b := mustJSONConn(view)
		if strings.Contains(b, canary) {
			t.Fatal("the credential value must never appear in any stored or listed record")
		}
	}
}

// TestConnectionDisplayNameMigration pins migration 0011: a fresh apply
// reaches the current schema version, the recorded migrations 7–11 are all
// `applied` (the checksum authority verifying 0007–0010 byte-identical on
// every open), and the display_name column exists on the connection
// records (the M-27 rename row's storage: the coded record carried no
// display-name field, so a NEW migration is the genuinely required form —
// never an edit to any recorded migration's SQL).
func TestConnectionDisplayNameMigration(t *testing.T) {
	st, ctx := newConnTestStore(t)
	v, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersionMax {
		t.Fatalf("schema version = %d, want %d (migration 0011 applied)", v, schemaVersionMax)
	}
	for _, version := range []int64{7, 8, 9, 10, 11} {
		state, err := st.migrationState(ctx, version)
		if err != nil {
			t.Fatal(err)
		}
		if state != migStateApplied {
			t.Fatalf("migration %d state = %s, want applied", version, state)
		}
	}
	var n int
	if err := st.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('provider_connections') WHERE name = 'display_name'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("the provider_connections table does not carry the display_name column")
	}
}

// TestConnectionRenameUpdatesDisplayNameOnly pins the M-27 rename row's
// binding rule at the store: a rename changes the record's display name
// (and its mutation marker) ONLY — the connection identity, the provider
// identity, the secret reference, the credential epoch, the fingerprint,
// the tested state, and the created instant are all byte-identical. The
// fence: a stale expected marker is the domain's stale-marker signal; an
// unknown id is the typed NOT_FOUND.
func TestConnectionRenameUpdatesDisplayNameOnly(t *testing.T) {
	st, ctx := newConnTestStore(t)
	rec := fakeRecord(t, st, ctx, "idem-1", fakeSaveBody{Provider: "mock", ModelID: "m", Secret: []byte("karyora-test-canary-rename-not-a-real-key")})
	renamed, err := st.RenameConnection(ctx, rec.ConnectionID, "  My Work Connection  ", rec.UpdatedAt)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.DisplayName != "My Work Connection" {
		t.Fatalf("display name = %q, want the trimmed value", renamed.DisplayName)
	}
	if renamed.ConnectionID != rec.ConnectionID || renamed.Provider != rec.Provider || renamed.ModelID != rec.ModelID ||
		renamed.BaseURL != rec.BaseURL || renamed.SecretRef != rec.SecretRef || renamed.CredentialEpoch != rec.CredentialEpoch ||
		renamed.Fingerprint != rec.Fingerprint || renamed.TestedAt != rec.TestedAt || renamed.CreatedAt != rec.CreatedAt ||
		renamed.TestedDigest != rec.TestedDigest || renamed.TestedEpoch != rec.TestedEpoch || renamed.IdempotencyKey != rec.IdempotencyKey || renamed.DisplayName != "My Work Connection" {
		t.Fatalf("a rename must change ONLY the display name: before %+v after %+v", rec, renamed)
	}
	if renamed.UpdatedAt == rec.UpdatedAt {
		t.Fatal("a rename bumps the record's mutation marker")
	}
	// The fence: a stale marker refuses and changes nothing.
	if _, err := st.RenameConnection(ctx, rec.ConnectionID, "Second Name", rec.UpdatedAt); !errors.Is(err, dprovider.ErrConnectionStaleMarker) {
		t.Fatalf("a stale marker must be the stale-marker fence signal, got %v", err)
	}
	after, err := st.LoadConnection(ctx, rec.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if after.DisplayName != "My Work Connection" || after.UpdatedAt != renamed.UpdatedAt {
		t.Fatalf("a fenced refusal changes nothing: %+v", after)
	}
	// The fresh marker succeeds (the guard's recover path).
	if _, err := st.RenameConnection(ctx, rec.ConnectionID, "Second Name", renamed.UpdatedAt); err != nil {
		t.Fatalf("rename with the fresh marker: %v", err)
	}
	// An unknown id is the typed NOT_FOUND.
	if _, err := st.RenameConnection(ctx, "conn-absent", "Name", "2026-11-04T00:00:00Z"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("an unknown id must be the typed NOT_FOUND, got %v", err)
	}
	// The empty display name refuses (a clearing is not a rename).
	if _, err := st.RenameConnection(ctx, rec.ConnectionID, "   ", "2026-11-04T00:00:00Z"); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("an empty display name must refuse, got %v", err)
	}
	// The empty marker refuses (the guard is mandatory on the coded path).
	if _, err := st.RenameConnection(ctx, rec.ConnectionID, "Name", ""); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("an empty marker must refuse, got %v", err)
	}
}

// TestConnectionUpdateChangesNonSecretFieldsAndFences pins the P-S8-02
// working candidate at the store: the update replaces EXACTLY the coded
// create shape's non-secret fields; the secret reference, the credential
// epoch, the create fingerprint, the idempotency key, and the created
// instant are carried over untouched (zero credential writes happen on
// this path); the tested state renders stale-truthfully afterward (the
// coded staleness rule's own outcome — the epoch-class precedent's
// behavior, unchanged); the fence refuses a stale marker; the merged
// record's shape law holds.
func TestConnectionUpdateChangesNonSecretFieldsAndFences(t *testing.T) {
	st, ctx := newConnTestStore(t)
	rec := fakeRecord(t, st, ctx, "idem-1", fakeSaveBody{Provider: "mock", ModelID: "m", Secret: []byte("karyora-test-canary-update-not-a-real-key")})
	// Stamp a successful test first so the stale-truthful outcome is provable.
	stamped, err := st.RecordConnectionTest(ctx, rec.ConnectionID, "2026-11-04T00:00:00Z")
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if !stamped.TestCurrent() {
		t.Fatal("the stamped record must test current before the update")
	}
	updated, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{
		Provider:      "mock",
		BaseURL:       "https://api.example.internal/v1",
		ModelID:       "m2",
		CustomHeaders: []dprovider.ConnectionHeader{{Name: "X-Karyora-Test", Value: "1"}},
	}, stamped.UpdatedAt)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.BaseURL != "https://api.example.internal/v1" || updated.ModelID != "m2" || len(updated.CustomHeaders) != 1 {
		t.Fatalf("the update must carry the new non-secret fields: %+v", updated)
	}
	if updated.SecretRef != rec.SecretRef || updated.CredentialEpoch != rec.CredentialEpoch ||
		updated.Fingerprint != rec.Fingerprint || updated.IdempotencyKey != rec.IdempotencyKey ||
		updated.CreatedAt != rec.CreatedAt || updated.ConnectionID != rec.ConnectionID {
		t.Fatalf("an update must never touch the secret/epoch/fingerprint/identity set: before %+v after %+v", rec, updated)
	}
	if updated.TestCurrent() {
		t.Fatal("an accepted update must render the tested state stale-truthfully (the inputs digest changed)")
	}
	// The fence: a stale marker refuses and changes nothing.
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "mock", ModelID: "m3"}, stamped.UpdatedAt); !errors.Is(err, dprovider.ErrConnectionStaleMarker) {
		t.Fatalf("a stale marker must be the stale-marker fence signal, got %v", err)
	}
	after, err := st.LoadConnection(ctx, rec.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModelID != "m2" || after.UpdatedAt != updated.UpdatedAt {
		t.Fatalf("a fenced refusal changes nothing: %+v", after)
	}
	// An unknown id is the typed NOT_FOUND; the merged record's shape law
	// refuses an empty provider/model id; the empty marker refuses.
	if _, err := st.UpdateConnection(ctx, "conn-absent", UpdateConnectionFields{Provider: "mock", ModelID: "m"}, "2026-11-04T00:00:00Z"); err == nil || !strings.Contains(err.Error(), string(errs.CodeNotFound)) {
		t.Fatalf("an unknown id must be the typed NOT_FOUND, got %v", err)
	}
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "", ModelID: "m"}, updated.UpdatedAt); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("an empty provider must refuse, got %v", err)
	}
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "mock", ModelID: ""}, updated.UpdatedAt); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("an empty model id must refuse, got %v", err)
	}
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "mock", ModelID: "m", CustomHeaders: []dprovider.ConnectionHeader{{Name: "Authorization", Value: "x"}}}, updated.UpdatedAt); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("a forbidden header must refuse, got %v", err)
	}
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "mock", ModelID: "m", BaseURL: "https://user:pass@example.internal/v1"}, updated.UpdatedAt); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("a URL-embedded credential must refuse, got %v", err)
	}
	if _, err := st.UpdateConnection(ctx, rec.ConnectionID, UpdateConnectionFields{Provider: "mock", ModelID: "m"}, ""); err == nil || !strings.Contains(err.Error(), string(errs.CodeValidationFailed)) {
		t.Fatalf("an empty marker must refuse, got %v", err)
	}
	// The create-idempotency replay ruling stays keyed to the CREATE body:
	// a replayed save under the original key still resolves (the update
	// never re-computed the fingerprint).
	replay := dprovider.ConnectionRecord{
		ConnectionID:   "conn-other-mint",
		IdempotencyKey: rec.IdempotencyKey,
		Provider:       rec.Provider,
		BaseURL:        rec.BaseURL,
		ModelID:        rec.ModelID,
		SecretRef:      "connection:conn-other-mint/api-key",
		Fingerprint:    rec.Fingerprint,
	}
	saved, replayed, err := st.SaveConnection(ctx, replay)
	if err != nil || !replayed || saved.ConnectionID != rec.ConnectionID {
		t.Fatalf("the create replay ruling must survive an update: %+v replayed=%v err=%v", saved, replayed, err)
	}
}
