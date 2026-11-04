package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

// The provider-connection record surface (the owner's card-10 working
// candidate of 2026-11-02 — the P3.2 §3.3 basis implemented by the
// connection-record milestone). Every operation rides the one decision
// port (C2-14): the code's own register determines a connection read as
// ActionDataRead and a connection write as ActionDataWrite over the
// ResourceConfig resource (the connection record is validated
// configuration; the Team adapter's connection.* actions are the recorded
// not-wired role vocabulary, cited for the freeze). Secrets ride the
// accepted D-37 boundary only: the record stores the secret REFERENCE
// (handle-only on disk); values pass through the SecretWriter once and
// never enter a response, a log, an error, or a test. No operation here
// consumes or reports a secret value — masked status only.
//
// Per-operation recorded bases:
//   - save (M-24): the Supplied create idempotency (duplicate → the
//     existing connection reference; same key + different body → 409
//     CONFLICT_IDEMPOTENCY_KEY, fingerprintMatch:false) — the C1-07 class
//     per P3.2 §3.3; retention: until user deletion (the record's own
//     life). The replay ruling runs BEFORE any secret write, so a replayed
//     save never writes a second credential.
//   - test (M-25): read-only, safe re-issue — the executor's bounded probe
//     machinery with its full gate set; the sanitized report carries the
//     verified endpoint/model + capability statuses + correlation id; a
//     lost test response proves nothing. A SAVED connection's successful
//     test stamps the tested state (TC-1); an unsaved draft's test records
//     nothing.
//   - rotate (M-26) / re-authenticate (M-27): the replacement key is a NEW
//     write-only value stored under the SAME handle (the D-37 atomic
//     same-handle swap); the credential epoch bumps BEFORE the swap so a
//     failed swap renders the tested state stale-truthfully. Both are
//     Open-class operations per P1.6 §4: a lost response is reconciled by
//     re-reading the record — never a blind re-send. The response data
//     carries the affected-models/routing + interruption statement as the
//     working candidate (the P3.2 §3.3 [P] row).
//   - delete/revoke: the Core command deletes the record AND revokes the
//     keyring entry; the D-37 fail-closed effects follow (dependent
//     surfaces then resolve the typed SECRET_UNAVAILABLE family and
//     refuse protected calls — the existing secret-boundary machinery's
//     own behavior). The high-risk confirmation is the UI's.
//   - switch (M-34): the minimal switch working candidate — the Core
//     records the active connection reference durably (single-row state);
//     the operation carries its correlation id and is idempotent on the
//     already-active connection; no Job migrates and no cache mixes (the
//     M-34 owner item keeps its recorded owner for F11's freeze).
//   - list/get (the O-2 family read): the server-side paginated list per
//     the adopted card-9 list-semantics working candidate (the IMPL-4
//     List pattern) — created_at descending, connection_id tiebreak, a
//     bounded page, an opaque keyset cursor.
//   - rename (the M-27 register row's rename portion, the adopted S8
//     supply's P-S8-01 working candidate): a Core connection-record change
//     that changes the record's display name ONLY — never the connection
//     identity, never the Job identity, never the execution history (the
//     P3.1 M-27 row's binding rule). Open-class per P1.6 §4 (no C1-07 key
//     row exists for this record type): no idempotency key, no auto-retry;
//     a lost response is reconciled by re-reading the record.
//   - update (the adopted S8 supply's P-S8-02 working candidate): the
//     non-secret field update over an EXISTING connection — EXACTLY the
//     coded create shape's non-secret fields (provider, base URL, model
//     id, custom headers). The secret path is NEVER folded in: no secret
//     field exists on the request, the record's secret reference and
//     credential epoch are carried over untouched, and credential changes
//     stay the coded rotate/re-authenticate contract (the replacement-key
//     discipline; the epoch bump-first ordering untouched). Any accepted
//     update changes the inputs digest, so the tested state renders
//     stale-truthfully (the coded staleness rule's own behavior). Also
//     Open-class: no idempotency key, no auto-retry, re-read reconciliation.
//
// The concurrent-edit guard (the adopted S8 supply's P-S8-03 working
// candidate — the CHOSEN class disclosed at implementation): the
// P3.3-recorded typed CONFLICT_STATE guard with currentState (the P1.6 §5
// recorded typed-outcome class). Both record mutations (rename AND
// update) carry it alike: the request carries the updated_at marker the
// caller read (expected_updated_at); the store enforces it through its
// conditional one-statement write (the coded RecordConnectionTest fencing
// shape) against that existing marker; a stale marker is the typed
// CONFLICT_STATE 409 whose data.currentState carries the record's current
// safe metadata (updated_at, credential_epoch, test_current) — never
// secret content (P1.6 §5's rule 9). The UI renders the refusal verbatim
// and performs no overwrite and no merge (the P1.6 §5 interim rule).
//
// The per-connection custom-executor construction (IMPL-18): the coded
// custom connection-protocol adapters (C3-29/C3-30) are parameterized by
// the connection record's own non-secret posture, so the wired-provider
// check resolves a custom family through the composition-provided
// ConnectionExecutorFactory port (the app layer cannot construct
// executors — the dependency map's own rule). A custom save/test/update
// rides the CONSTRUCTED executor's posture (its credential class, its
// base-URL requirement — a custom family has no recorded default); the
// probe's full gate set applies unchanged, so a custom profile's honest
// unknown data-handling record blocks its probe typed (egress_policy) —
// the recorded C3-19 fail-closed posture, never weakened, the
// classification-policy allowlists staying their recorded owner's. A
// genuinely-unwired provider (no static executor, no constructible
// family) keeps the recorded typed refusal ("provider is not wired for
// this core").

// Port type aliases: the list query/page shapes live in the domain
// package so the storage adapter and the use-case layer share one shape.
type (
	ListConnectionsQuery = dprovider.ListConnectionsQuery
	ListConnectionsPage  = dprovider.ListConnectionsPage
)

// ConnectionStore is the durable connection-record port. The storage
// adapter implements it; the slice consumes only this port.
type ConnectionStore interface {
	SaveConnection(ctx context.Context, rec dprovider.ConnectionRecord) (dprovider.ConnectionRecord, bool, error)
	LoadConnection(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error)
	LoadConnectionByIdempotencyKey(ctx context.Context, key string) (dprovider.ConnectionRecord, bool, error)
	ListConnections(ctx context.Context, q ListConnectionsQuery) (ListConnectionsPage, error)
	RecordConnectionTest(ctx context.Context, connectionID, testedAt string) (dprovider.ConnectionRecord, error)
	BumpConnectionCredential(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error)
	RenameConnection(ctx context.Context, connectionID, displayName, expectedUpdatedAt string) (dprovider.ConnectionRecord, error)
	UpdateConnection(ctx context.Context, connectionID string, fields dprovider.UpdateConnectionFields, expectedUpdatedAt string) (dprovider.ConnectionRecord, error)
	DeleteConnection(ctx context.Context, connectionID string) (dprovider.ConnectionRecord, error)
	SetActiveConnection(ctx context.Context, connectionID string) (string, error)
	ActiveConnectionID(ctx context.Context) (string, error)
}

// ConnectionExecutorFactory constructs the per-connection executors of the
// custom connection-protocol families (C3-29/C3-30): a custom adapter is
// parameterized by the connection record's own non-secret posture (the
// explicit base URL, the exact model id), so its executor cannot pre-exist
// in the wired provider map — it is built per connection, from the record.
// The app layer cannot construct executors (the dependency map's own rule:
// internal/app never imports internal/providers), so the composition root
// implements this port over the providers package and the slice consumes
// only the port — the construction crosses the layer boundary through the
// composition-provided factory. Optional: nil keeps the recorded unwired
// posture for those providers (the typed wired-check refusal stays, fail
// closed — nothing is invented to bypass it).
type ConnectionExecutorFactory interface {
	// IsConnectionProtocol reports whether the provider id names a custom
	// connection-protocol family the factory constructs per connection
	// (as opposed to a statically wired executor in SliceDeps.Providers).
	IsConnectionProtocol(provider string) bool
	// ConnectionExecutor builds the per-connection executor for one
	// connection posture (the record's non-secret identity: the provider
	// id, the base URL, the exact model id). ok=false when the posture
	// does not yield a constructible executor — the caller then applies
	// its own wired-provider law (the typed refusal stays).
	ConnectionExecutor(provider, baseURL, modelID string) (ProviderExecutor, bool)
}

// Connection bounds (Proposed, pending the recorded owner review — the
// slice-bounds discipline).
const (
	// DefaultConnectionListLimit is the connection-list per-surface default
	// page size (the card-9 per-surface default class).
	DefaultConnectionListLimit = 50
	// MaxConnectionListLimit is the list page bound — the same 1–1000 bound
	// class the other read surfaces use.
	MaxConnectionListLimit = 1000
)

// connectionSecretName is the fixed handle name a connection's credential
// resolves under (the CLI credential surface's `api-key` name class; the
// scope is the connection's own).
const connectionSecretName = "api-key"

// ConnectionHandle builds the secret handle REFERENCE for one connection
// (handle-only on disk per D-37; the value lives only in the OS keyring).
func ConnectionHandle(connectionID string) security.Handle {
	return security.Handle{Scope: "connection:" + connectionID, Name: connectionSecretName}
}

// connectionExecutorFor resolves the executor a connection posture rides:
// the statically wired provider map first (the delivered posture — its
// behavior is unchanged for every statically wired provider), then the
// composition-provided factory for a custom connection-protocol family
// (C3-29/C3-30 — the per-connection construction). wired=false when
// neither source supplies the provider: the caller's typed wired-check
// refusal stays (the genuinely-unwired case — the recorded pin).
func (s *Slice) connectionExecutorFor(provider, baseURL, modelID string) (ProviderExecutor, bool) {
	if executor, wired := s.deps.Providers[provider]; wired {
		return executor, true
	}
	if s.deps.ConnectionExecutors == nil {
		return nil, false
	}
	return s.deps.ConnectionExecutors.ConnectionExecutor(provider, baseURL, modelID)
}

// SaveConnectionRequest is one connection-create command (M-24). Secret is
// the credential VALUE: it exists for the single SecretWriter store and
// the idempotency-fingerprint computation, and is never stored in the
// record, echoed in any response, or logged.
type SaveConnectionRequest struct {
	IdempotencyKey string
	Provider       string
	BaseURL        string
	ModelID        string
	CustomHeaders  []dprovider.ConnectionHeader
	Secret         []byte
}

// SaveConnectionResult reports the saved (or replayed) connection through
// its read view (the masked secret status + tested-state truth included).
type SaveConnectionResult struct {
	Connection ConnectionView
	Replayed   bool
}

// SaveConnection creates one provider connection (M-24): authorize →
// request-shape law → the wired-provider check (a typo must never plant a
// connection whose executor does not exist) → the record-shape law →
// audited (fail closed) → the secret through the SecretWriter → the
// durable record. The idempotency fingerprint is computed over the request
// form INCLUDING the credential bytes (one-way; the digest is stored, the
// bytes are not) and the replay ruling runs BEFORE any secret write, so a
// replayed save never writes a second credential.
func (s *Slice) SaveConnection(ctx context.Context, req SaveConnectionRequest) (SaveConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return SaveConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig}, security.ActionDataWrite)
	if err != nil {
		return SaveConnectionResult{}, err
	}
	// The request-shape law: typed refusals, never silent coercion.
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "idempotency_key"})
	}
	if len(req.IdempotencyKey) > 128 {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "idempotency_key"})
	}
	if strings.TrimSpace(req.Provider) == "" {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "provider"})
	}
	if strings.TrimSpace(req.ModelID) == "" {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model_id"})
	}
	if len(req.ModelID) > 256 || len(req.BaseURL) > 2048 {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model_id"})
	}
	if strings.Contains(req.BaseURL, "@") {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_url"})
	}
	executor, wired := s.connectionExecutorFor(req.Provider, req.BaseURL, req.ModelID)
	if !wired {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "provider", "reason": "provider is not wired for this core",
		})
	}
	// The executor's own posture decides the credential class and whether
	// the record must carry the destination: a statically wired adapter
	// records a default base (the record may omit one), while a custom
	// connection-protocol family (C3-29/C3-30) has NO recorded default —
	// its destination IS the record's base URL, so a connection posture
	// without a default requires the request's base URL (the typed field
	// law; the Core enforces, the UI validation is feedback — P3.3 §8).
	posture := executor.Connection()
	requiresSecret := posture.AuthKind != dprovider.AuthNone
	if requiresSecret && len(req.Secret) == 0 {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "secret"})
	}
	if !requiresSecret {
		req.Secret = nil // an AuthNone adapter never stores a credential
	}
	if posture.BaseURL == "" && req.BaseURL == "" {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_url"})
	}
	if err := dprovider.ValidateHeaders(req.CustomHeaders); err != nil {
		return SaveConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "custom_headers"})
	}
	fingerprint := dprovider.FingerprintOf(req.Provider, req.BaseURL, req.ModelID, req.CustomHeaders, req.Secret)
	// The replay ruling FIRST (the Supplied idempotency): a stored row under
	// this key resolves by its create fingerprint before anything is
	// written — a replayed save never mints a connection, never writes a
	// second credential, and a same-key-different-body save is the typed
	// CONFLICT_IDEMPOTENCY_KEY with fingerprintMatch:false.
	if existing, found, err := s.deps.Connections.LoadConnectionByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		return SaveConnectionResult{}, err
	} else if found {
		if existing.Fingerprint == fingerprint {
			return SaveConnectionResult{Connection: s.connectionView(ctx, existing, ""), Replayed: true}, nil
		}
		return SaveConnectionResult{}, envelopeErr(errs.CodeConflictIdempotencyKey, corr, map[string]any{
			"keyScope": "connection.create", "fingerprintMatch": false,
		})
	}
	// The secret-boundary precondition (fail closed before anything else
	// moves): the SecretWriter must be wired — where no OS keyring exists,
	// BYOK persistence is explicitly unavailable (the accepted D-37
	// residual limitation), reported typed.
	if requiresSecret && s.deps.Secrets == nil {
		return SaveConnectionResult{}, &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: "connection:new/" + connectionSecretName}
	}
	connID := "conn-" + randomID()
	handle := ConnectionHandle(connID)
	// The audited record rides the fail-closed path BEFORE the effect
	// (C2-15 §5); the audit never carries the credential.
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "connection.save",
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: connID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      req.Provider,
		Outcome:         "save",
		CorrelationID:   corr,
	}); err != nil {
		return SaveConnectionResult{}, err
	}
	if requiresSecret {
		// The credential is written ONCE under the record's own handle; the
		// record stores the REFERENCE. An empty credential is never stored
		// (the SecretWriter's own rule); a store failure aborts the save —
		// the record is never written without its credential when one is
		// required.
		if err := s.deps.Secrets.Store(ctx, handle, req.Secret); err != nil {
			return SaveConnectionResult{}, err
		}
	}
	rec := dprovider.ConnectionRecord{
		ConnectionID:   connID,
		IdempotencyKey: req.IdempotencyKey,
		Provider:       req.Provider,
		BaseURL:        req.BaseURL,
		ModelID:        req.ModelID,
		CustomHeaders:  req.CustomHeaders,
		SecretRef:      handle.String(),
		Fingerprint:    fingerprint,
	}
	saved, replayed, err := s.deps.Connections.SaveConnection(ctx, rec)
	if err != nil {
		return SaveConnectionResult{}, err
	}
	return SaveConnectionResult{Connection: s.connectionView(ctx, saved, ""), Replayed: replayed}, nil
}

// ConnectionView is one connection record as the read surfaces report it:
// the durable record's fields PLUS the safe masked secret status (the
// D-37 "masked status only" discipline) and the tested-state truth
// (tested / untested / tested-but-changed-since per the tested-at inputs
// digest and the credential epoch). No field ever carries a secret value.
type ConnectionView struct {
	Record       dprovider.ConnectionRecord `json:"record"`
	SecretStatus string                     `json:"secret_status"`
	TestCurrent  bool                       `json:"test_current"`
	Active       bool                       `json:"active"`
}

// connectionView projects a record through the secret boundary's masked
// status and the tested-state truth. A secret-status probe failure is a
// TYPED status (inaccessible), never a value and never a fabricated
// "present".
func (s *Slice) connectionView(ctx context.Context, rec dprovider.ConnectionRecord, activeID string) ConnectionView {
	view := ConnectionView{Record: rec, SecretStatus: string(security.SecretInaccessib), TestCurrent: rec.TestCurrent(), Active: rec.ConnectionID != "" && rec.ConnectionID == activeID}
	if store, ok := s.secretStatusStore(); ok {
		if handle, err := parseConnectionHandle(rec.SecretRef); err == nil {
			if status, err := store.Status(ctx, handle); err == nil {
				view.SecretStatus = string(status)
			}
		}
	}
	return view
}

// secretStatusStore returns the wired store's resolution face for the
// masked-status probes (the wired SecretWriter dep may carry the full
// store; a writer-only fake reports the typed inaccessible status, which
// is the honest answer such a wiring can give).
func (s *Slice) secretStatusStore() (security.SecretStore, bool) {
	if s.deps.Secrets == nil {
		return nil, false
	}
	if store, ok := s.deps.Secrets.(security.SecretStore); ok {
		return store, true
	}
	return nil, false
}

func (s *Slice) secretStoreOrUnavailable() security.SecretStore {
	if store, ok := s.secretStatusStore(); ok {
		return store
	}
	return security.UnavailableStore{}
}

func parseConnectionHandle(ref string) (security.Handle, error) {
	scope, name, ok := strings.Cut(ref, "/")
	if !ok || scope == "" || name == "" {
		return security.Handle{}, fmt.Errorf("app: secret reference must be scope/name")
	}
	return security.Handle{Scope: scope, Name: name}, nil
}

// ListConnectionsRequest is the connection-list query (the card-9 working
// candidate's list discipline). Limit zero selects the per-surface
// default; Cursor is the opaque continuation cursor (empty = first page).
type ListConnectionsRequest struct {
	Limit  int
	Cursor string
}

// ListConnectionsResult is one bounded page. NextCursor is empty on the
// final page — the cursorless page IS the end marker (the card-9
// working candidate; no has_more/total vocabulary exists to invent).
type ListConnectionsResult struct {
	Connections []ConnectionView
	NextCursor  string
}

// connectionListCursorPosition is the decoded form behind the opaque connection-list
// cursor: the keyset position (the created_at/connection_id pair), the
// IMPL-4 list-cursor pattern as a separate query-side mechanism.
type connectionListCursorPosition struct {
	CreatedAt    string `json:"created_at"`
	ConnectionID string `json:"connection_id"`
}

// ListConnections returns one bounded page of the connection records in
// the Core-defined order — created_at descending (newest first),
// connection_id ascending tiebreak — through the same decision port every
// read authorizes.
func (s *Slice) ListConnections(ctx context.Context, req ListConnectionsRequest) (ListConnectionsResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return ListConnectionsResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConfig}, security.ActionDataRead); err != nil {
		return ListConnectionsResult{}, err
	}
	limit := req.Limit
	if limit == 0 {
		limit = DefaultConnectionListLimit
	}
	if limit < 1 || limit > MaxConnectionListLimit {
		return ListConnectionsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "limit"})
	}
	q := dprovider.ListConnectionsQuery{Limit: limit}
	if req.Cursor != "" {
		pos, err := decodeConnectionListCursor(req.Cursor)
		if err != nil {
			return ListConnectionsResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "cursor"})
		}
		q.AfterCreatedAt, q.AfterConnectionID = pos.CreatedAt, pos.ConnectionID
	}
	page, err := s.deps.Connections.ListConnections(ctx, q)
	if err != nil {
		return ListConnectionsResult{}, err
	}
	activeID, err := s.deps.Connections.ActiveConnectionID(ctx)
	if err != nil {
		return ListConnectionsResult{}, err
	}
	res := ListConnectionsResult{Connections: make([]ConnectionView, 0, len(page.Connections))}
	for _, rec := range page.Connections {
		res.Connections = append(res.Connections, s.connectionView(ctx, rec, activeID))
	}
	if page.More {
		if len(page.Connections) == 0 {
			return ListConnectionsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		next, cerr := encodeConnectionListCursor(page.Connections[len(page.Connections)-1])
		if cerr != nil {
			// A cursor that cannot be minted must never silently truncate
			// the list: the caller sees a typed failure and re-queries.
			return ListConnectionsResult{}, envelopeErr(errs.CodeInternal, corr, nil)
		}
		res.NextCursor = next
	}
	return res, nil
}

// GetConnection returns one connection record's view (the masked status +
// tested-state truth included). An unknown id is the typed NOT_FOUND; the
// read creates nothing and mutates nothing.
func (s *Slice) GetConnection(ctx context.Context, connectionID string) (ConnectionView, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return ConnectionView{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: connectionID}, security.ActionDataRead); err != nil {
		return ConnectionView{}, err
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, connectionID)
	if err != nil {
		return ConnectionView{}, err
	}
	activeID, err := s.deps.Connections.ActiveConnectionID(ctx)
	if err != nil {
		return ConnectionView{}, err
	}
	return s.connectionView(ctx, rec, activeID), nil
}

// TestConnectionRequest is the read-only test command (M-25): either a
// SAVED connection (ConnectionID present — the credential resolves through
// the record's stored reference) or an UNSAVED safe draft (the typed
// secret rides transiently, sent only by this explicit action and dropped
// after the probe — never stored, never echoed).
type TestConnectionRequest struct {
	ConnectionID  string
	Provider      string
	BaseURL       string
	ModelID       string
	CustomHeaders []dprovider.ConnectionHeader
	Secret        []byte
}

// TestConnectionResult reports the sanitized test outcome (M-25): the
// verified endpoint/model + capability statuses on success; the sanitized
// bounded error + correlation id on failure. No secret echo is possible:
// the executor redacts resolved credential bytes before construction and
// this report never carries one. Record is set when the test rode a saved
// connection.
type TestConnectionResult struct {
	Report dprovider.ConnectionTestReport
	Record *dprovider.ConnectionRecord
}

// TestConnection runs the bounded, non-destructive probe (M-25). It is a
// READ by effect class — safe re-issue; a lost response proves nothing
// and no retry is owed. A saved connection's SUCCESSFUL test stamps the
// tested state (TC-1); every other outcome records nothing.
func (s *Slice) TestConnection(ctx context.Context, req TestConnectionRequest) (TestConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return TestConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConfig}, security.ActionDataRead); err != nil {
		return TestConnectionResult{}, err
	}
	var rec *dprovider.ConnectionRecord
	provider := req.Provider
	baseURL := req.BaseURL
	modelID := req.ModelID
	headers := req.CustomHeaders
	if err := dprovider.ValidateHeaders(headers); err != nil {
		return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "custom_headers"})
	}
	secretRef := ""
	var transient []byte
	if req.ConnectionID != "" {
		saved, err := s.deps.Connections.LoadConnection(ctx, req.ConnectionID)
		if err != nil {
			return TestConnectionResult{}, err
		}
		rec = &saved
		provider = saved.Provider
		baseURL = saved.BaseURL
		modelID = saved.ModelID
		headers = saved.CustomHeaders
		secretRef = saved.SecretRef
	} else {
		// The unsaved draft's typed secret is transient: it rides a
		// request-scoped store for the single probe and is dropped after.
		transient = req.Secret
	}
	executor, wired := s.connectionExecutorFor(provider, baseURL, modelID)
	if !wired {
		return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "provider", "reason": "provider is not wired for this core",
		})
	}
	if strings.TrimSpace(modelID) == "" {
		return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model_id"})
	}
	// The test plan mirrors the execution path's plan shape: the wired
	// executor's connection posture, the record's base-URL override, and
	// the exact Model ID the connection stores. The probe carries NO user
	// data; its traffic is labeled with the Core's own test-fixture class
	// (the recorded classification for the Core's verification traffic —
	// datahandling.go's ClassificationTestFixture), which the fail-closed
	// egress gate applies exactly as for every call: a provider whose
	// data-handling record does not allow it refuses typed (the recorded
	// classification-policy owner decision governs the real-provider
	// probe path; no label is ever invented here for a provider).
	conn := executor.Connection()
	if baseURL != "" {
		conn.BaseURL = baseURL
	}
	// The same base-URL field law the save carries: a connection posture
	// without a recorded default (the custom connection-protocol families)
	// requires the record's or the draft's own base URL — the probe has
	// nowhere to point otherwise.
	if conn.BaseURL == "" {
		return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_url"})
	}
	if conn.AuthKind != dprovider.AuthNone {
		if secretRef == "" && len(transient) == 0 {
			return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "secret"})
		}
		if secretRef == "" {
			secretRef = ConnectionHandle("test").String()
		}
	}
	conn.SecretRef = secretRef
	plan := &dprovider.RequestPlan{
		Operation:  dprovider.OperationChat,
		Profile:    executor.ProfileFor(modelID),
		Connection: conn,
		Metadata:   map[string]string{dprovider.ClassificationKey: dprovider.ClassificationTestFixture},
	}
	report := dprovider.ConnectionTestReport{
		Provider:      provider,
		ModelID:       modelID,
		BaseURL:       conn.BaseURL,
		CorrelationID: corr,
	}
	// The resolution store for the probe: the app's wired store (in the
	// composition root the same keyring store the executors carry). The
	// unsaved-draft path wraps it with the request-scoped transient store
	// that answers only the transient handle.
	store := s.secretStoreOrUnavailable()
	if len(transient) > 0 && conn.AuthKind != dprovider.AuthNone {
		handle, err := parseConnectionHandle(conn.SecretRef)
		if err != nil {
			return TestConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "secret"})
		}
		store = transientSecretStore{handle: handle, value: transient, base: store}
	}
	probeErr := executor.TestConnectionWith(ctx, store, plan)
	if probeErr != nil {
		// The sanitized failure: the typed provider error's bounded,
		// already-redacted detail and its stable error class — never an
		// upstream body, never a secret echo (P3.2 §3.1 data bounds;
		// P3.3 §8 invariant 11).
		report.OK = false
		if pe, ok := probeErr.(*dprovider.ProviderError); ok {
			report.ErrorKind = string(pe.Kind)
			report.ErrorDetail = pe.Detail
		} else {
			report.ErrorKind = string(dprovider.ErrUnknown)
			report.ErrorDetail = bounded(probeErr.Error())
		}
		return TestConnectionResult{Report: report}, nil
	}
	report.OK = true
	caps := executor.Capabilities(plan.Profile)
	report.Capabilities = &caps
	// TC-1: a SAVED connection's successful test records the verified
	// result and its time (a metadata write on the record; an unsaved
	// draft's test records nothing).
	if rec != nil {
		stamped, err := s.deps.Connections.RecordConnectionTest(ctx, rec.ConnectionID, s.deps.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return TestConnectionResult{}, err
		}
		rec = &stamped
	}
	return TestConnectionResult{Report: report, Record: rec}, nil
}

// RotateConnectionRequest is one credential-rotation command (M-26): the
// replacement key is a NEW write-only value; it is never the decrypted old
// key (the old value is never read for a rotation) and is never echoed.
type RotateConnectionRequest struct {
	ConnectionID string
	Secret       []byte
}

// RotateConnectionResult reports the Core-confirmed rotation: the updated
// record (the credential epoch bumped — the tested state now renders
// stale-truthfully until the next successful test) and the affected-
// models/routing + interruption statement (the P3.2 §3.3 [P] row as the
// working candidate: the affected set is the connection's own stored
// model; the interruption statement is the Core's own report that the
// next call resolves the new value and no in-flight call re-reads it).
type RotateConnectionResult struct {
	Record           dprovider.ConnectionRecord `json:"record"`
	AffectedModels   []string                   `json:"affected_models"`
	RoutingNote      string                     `json:"routing_note"`
	InterruptionNote string                     `json:"interruption_note"`
}

// RotateConnection swaps the credential under the SAME handle (the D-37
// atomic same-handle swap): the credential epoch bumps durably FIRST (a
// failed swap renders the tested state stale-truthfully — the safe
// direction), then the new value is stored under the record's own handle.
func (s *Slice) RotateConnection(ctx context.Context, req RotateConnectionRequest) (RotateConnectionResult, error) {
	return s.replaceConnectionCredential(ctx, req, "connection.rotate")
}

// ReauthenticateConnection is the credential re-entry path (M-27): a fresh
// write-only key under the same record — the rotate path's cousin. It
// rides the identical Core mechanics with its own audited action.
func (s *Slice) ReauthenticateConnection(ctx context.Context, req RotateConnectionRequest) (RotateConnectionResult, error) {
	return s.replaceConnectionCredential(ctx, req, "connection.reauthenticate")
}

func (s *Slice) replaceConnectionCredential(ctx context.Context, req RotateConnectionRequest, action string) (RotateConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return RotateConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: req.ConnectionID}, security.ActionDataWrite)
	if err != nil {
		return RotateConnectionResult{}, err
	}
	if strings.TrimSpace(req.ConnectionID) == "" {
		return RotateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "connection_id"})
	}
	if len(req.Secret) == 0 {
		return RotateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "secret"})
	}
	if s.deps.Secrets == nil {
		return RotateConnectionResult{}, &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: "connection:unknown/" + connectionSecretName}
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, req.ConnectionID)
	if err != nil {
		return RotateConnectionResult{}, err
	}
	// The audited record BEFORE the effect (fail closed, C2-15 §5); the
	// audit never carries the credential.
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          action,
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: rec.ConnectionID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      rec.Provider,
		Outcome:         "credential_replaced",
		CorrelationID:   corr,
	}); err != nil {
		return RotateConnectionResult{}, err
	}
	// The epoch bumps FIRST: a swap failure leaves the tested state
	// stale-truthfully (the safe direction), never falsely current.
	bumped, err := s.deps.Connections.BumpConnectionCredential(ctx, rec.ConnectionID)
	if err != nil {
		return RotateConnectionResult{}, err
	}
	handle, err := parseConnectionHandle(bumped.SecretRef)
	if err != nil {
		return RotateConnectionResult{}, envelopeErr(errs.CodeStateCorrupt, corr, map[string]any{"reason": "connection secret reference does not parse"})
	}
	// The D-37 same-handle swap: the next call boundary resolves the new
	// value; no in-flight call re-reads it.
	if err := s.deps.Secrets.Store(ctx, handle, req.Secret); err != nil {
		return RotateConnectionResult{}, err
	}
	return RotateConnectionResult{
		Record:           bumped,
		AffectedModels:   []string{bumped.ModelID},
		RoutingNote:      "connection_scoped_routing_only",
		InterruptionNote: "next_call_resolves_new_value_no_inflight_reread",
	}, nil
}

// RenameConnectionRequest is one connection-record rename command (the
// M-27 register row's rename portion, the adopted S8 supply's P-S8-01
// working candidate): the display name ONLY, fenced on the marker the
// caller read (the concurrent-edit guard).
type RenameConnectionRequest struct {
	ConnectionID      string
	DisplayName       string
	ExpectedUpdatedAt string
}

// RenameConnectionResult reports the Core-confirmed rename through its
// read view (the masked secret status + tested-state truth + active flag).
type RenameConnectionResult struct {
	Connection ConnectionView `json:"connection"`
}

// RenameConnection changes one connection record's display name (the M-27
// rename row as implemented): authorize → the request-shape law → the
// typed NOT_FOUND on an unknown id → audited (fail closed, before the
// effect) → the fenced store write. The write changes the record's display
// name ONLY — never the connection identity, never the Job identity, never
// the execution history (the P3.1 M-27 row's binding rule). A stale
// expected_updated_at marker (a concurrent editor moved the record) is the
// typed CONFLICT_STATE 409 whose data.currentState carries the record's
// current safe metadata — the concurrent-edit guard's recorded outcome.
// The operation is Open-class (P1.6 §4: no C1-07 key row exists for this
// record type): no idempotency key, no auto-retry anywhere; a lost
// response is reconciled by re-reading the record.
func (s *Slice) RenameConnection(ctx context.Context, req RenameConnectionRequest) (RenameConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return RenameConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: req.ConnectionID}, security.ActionDataWrite)
	if err != nil {
		return RenameConnectionResult{}, err
	}
	// The request-shape law: typed refusals, never silent coercion.
	if strings.TrimSpace(req.ConnectionID) == "" {
		return RenameConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "connection_id"})
	}
	if strings.TrimSpace(req.ExpectedUpdatedAt) == "" {
		return RenameConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "expected_updated_at"})
	}
	if err := dprovider.ValidateDisplayName(req.DisplayName); err != nil {
		return RenameConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "display_name"})
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, req.ConnectionID)
	if err != nil {
		return RenameConnectionResult{}, err
	}
	// The audited record BEFORE the effect (fail closed, C2-15 §5); the
	// audit never carries secret content.
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "connection.rename",
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: rec.ConnectionID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      rec.Provider,
		Outcome:         "rename",
		CorrelationID:   corr,
	}); err != nil {
		return RenameConnectionResult{}, err
	}
	renamed, err := s.deps.Connections.RenameConnection(ctx, req.ConnectionID, req.DisplayName, req.ExpectedUpdatedAt)
	if err != nil {
		if errors.Is(err, dprovider.ErrConnectionStaleMarker) {
			return RenameConnectionResult{}, s.connectionStaleConflict(ctx, corr, req.ConnectionID)
		}
		return RenameConnectionResult{}, err
	}
	view, err := s.connectionViewWithActive(ctx, renamed)
	if err != nil {
		return RenameConnectionResult{}, err
	}
	return RenameConnectionResult{Connection: view}, nil
}

// UpdateConnectionRequest is one non-secret field update (the adopted S8
// supply's P-S8-02 working candidate): EXACTLY the coded create shape's
// non-secret fields, fenced on the marker the caller read. NO secret field
// exists on this request — the secret path stays the coded
// rotate/re-authenticate contract (the replacement-key discipline; the
// epoch bump-first ordering untouched), and this milestone's law is that
// an update NEVER touches a credential.
type UpdateConnectionRequest struct {
	ConnectionID      string
	Provider          string
	BaseURL           string
	ModelID           string
	CustomHeaders     []dprovider.ConnectionHeader
	ExpectedUpdatedAt string
}

// UpdateConnectionResult reports the Core-confirmed update through its
// read view (the masked secret status + the tested-state truth — which any
// accepted update renders stale-truthfully via the coded inputs digest —
// plus the active flag).
type UpdateConnectionResult struct {
	Connection ConnectionView `json:"connection"`
}

// UpdateConnection replaces one connection record's non-secret fields (the
// coded create shape's non-secret field set): authorize → the request-shape
// law (the save's own law set: the wired-provider check, the bounds, the
// URL-embedded-credential refusal, the header restriction) → the typed
// NOT_FOUND on an unknown id → audited (fail closed, before the effect) →
// the fenced store write. The record's secret reference, credential epoch,
// and create fingerprint are carried over untouched: zero credential
// writes happen on this path, and the create-idempotency replay ruling
// stays keyed to the CREATE body. A stale expected_updated_at marker is
// the typed CONFLICT_STATE 409 with currentState (the concurrent-edit
// guard). Open-class per P1.6 §4: no idempotency key, no auto-retry
// anywhere; a lost response is reconciled by re-reading the record.
func (s *Slice) UpdateConnection(ctx context.Context, req UpdateConnectionRequest) (UpdateConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: req.ConnectionID}, security.ActionDataWrite)
	if err != nil {
		return UpdateConnectionResult{}, err
	}
	// The request-shape law: the save's law set applied to the four
	// non-secret fields; typed refusals naming their field, never silent
	// coercion. The secret path is not part of this law set: no secret
	// field exists on the request to validate.
	if strings.TrimSpace(req.ConnectionID) == "" {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "connection_id"})
	}
	if strings.TrimSpace(req.ExpectedUpdatedAt) == "" {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "expected_updated_at"})
	}
	if strings.TrimSpace(req.Provider) == "" {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "provider"})
	}
	if strings.TrimSpace(req.ModelID) == "" {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model_id"})
	}
	if len(req.ModelID) > 256 || len(req.BaseURL) > 2048 {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model_id"})
	}
	if strings.Contains(req.BaseURL, "@") {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_url"})
	}
	executor, wired := s.connectionExecutorFor(req.Provider, req.BaseURL, req.ModelID)
	if !wired {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "provider", "reason": "provider is not wired for this core",
		})
	}
	// The save's own base-URL field law, carried symmetrically: a posture
	// without a recorded default requires the updated record's base URL.
	if posture := executor.Connection(); posture.BaseURL == "" && req.BaseURL == "" {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "base_url"})
	}
	if err := dprovider.ValidateHeaders(req.CustomHeaders); err != nil {
		return UpdateConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "custom_headers"})
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, req.ConnectionID)
	if err != nil {
		return UpdateConnectionResult{}, err
	}
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "connection.update",
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: rec.ConnectionID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      rec.Provider,
		Outcome:         "update",
		CorrelationID:   corr,
	}); err != nil {
		return UpdateConnectionResult{}, err
	}
	updated, err := s.deps.Connections.UpdateConnection(ctx, req.ConnectionID, dprovider.UpdateConnectionFields{
		Provider:      req.Provider,
		BaseURL:       req.BaseURL,
		ModelID:       req.ModelID,
		CustomHeaders: req.CustomHeaders,
	}, req.ExpectedUpdatedAt)
	if err != nil {
		if errors.Is(err, dprovider.ErrConnectionStaleMarker) {
			return UpdateConnectionResult{}, s.connectionStaleConflict(ctx, corr, req.ConnectionID)
		}
		return UpdateConnectionResult{}, err
	}
	view, err := s.connectionViewWithActive(ctx, updated)
	if err != nil {
		return UpdateConnectionResult{}, err
	}
	return UpdateConnectionResult{Connection: view}, nil
}

// connectionStaleConflict converts the store's stale-marker fence signal
// into the typed CONFLICT_STATE envelope whose data carries the record's
// CURRENT state — the P1.6 §5 recorded typed-outcome class (CONFLICT_STATE
// 409 with data.currentState; the adopted S8 supply's chosen concurrent-
// edit candidate). The current state carries SAFE metadata only: the
// marker, the credential epoch, and the tested-state truth — never secret
// content (P1.6 §5's rule 9).
func (s *Slice) connectionStaleConflict(ctx context.Context, corr, connectionID string) error {
	rec, err := s.deps.Connections.LoadConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	return envelopeErr(errs.CodeConflictState, corr, map[string]any{
		"currentState": map[string]any{
			"updated_at":       rec.UpdatedAt,
			"credential_epoch": rec.CredentialEpoch,
			"test_current":     rec.TestCurrent(),
		},
	})
}

// connectionViewWithActive projects a record through the masked status and
// the tested-state truth with the REAL active pointer (the mutation
// results' view is the authoritative read the surface re-renders from).
func (s *Slice) connectionViewWithActive(ctx context.Context, rec dprovider.ConnectionRecord) (ConnectionView, error) {
	activeID, err := s.deps.Connections.ActiveConnectionID(ctx)
	if err != nil {
		return ConnectionView{}, err
	}
	return s.connectionView(ctx, rec, activeID), nil
}

// DeleteConnection deletes the record AND revokes its keyring entry (the
// D-37 fail-closed order: the revoke runs first — a typed revoke failure
// refuses the delete, so a live credential is never abandoned behind a
// deleted record; a missing entry is the idempotent no-op). Dependent
// surfaces then resolve the typed SECRET_UNAVAILABLE family and refuse
// protected calls — the existing secret-boundary machinery's own behavior.
func (s *Slice) DeleteConnection(ctx context.Context, connectionID string) error {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: connectionID}, security.ActionDataWrite)
	if err != nil {
		return err
	}
	if strings.TrimSpace(connectionID) == "" {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "connection_id"})
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "connection.delete",
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: rec.ConnectionID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      rec.Provider,
		Outcome:         "delete",
		CorrelationID:   corr,
	}); err != nil {
		return err
	}
	if s.deps.Secrets != nil {
		handle, err := parseConnectionHandle(rec.SecretRef)
		if err != nil {
			return envelopeErr(errs.CodeStateCorrupt, corr, map[string]any{"reason": "connection secret reference does not parse"})
		}
		// Revoke first (idempotent on a missing entry); a typed revoke
		// failure refuses the delete (fail closed).
		if err := s.deps.Secrets.Revoke(ctx, handle); err != nil {
			return err
		}
	}
	_, err = s.deps.Connections.DeleteConnection(ctx, connectionID)
	return err
}

// SwitchConnectionResult reports the Core-confirmed switch (the minimal
// M-34 working candidate): the active connection reference, now durable
// and queryable. No Job migrates; no cache mixes.
type SwitchConnectionResult struct {
	ActiveConnectionID string `json:"active_connection_id"`
	Replayed           bool   `json:"replayed"`
}

// SwitchConnection sets the active connection reference (M-34's minimal
// working candidate): authorize → audited (fail closed) → validated
// against a stored record → the durable pointer. Idempotent on the
// already-active connection (the recorded transition-collapse class: a
// repeat switch to the active connection is a no-op returning the same
// result).
func (s *Slice) SwitchConnection(ctx context.Context, connectionID string) (SwitchConnectionResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return SwitchConnectionResult{}, envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: connectionID}, security.ActionDataWrite)
	if err != nil {
		return SwitchConnectionResult{}, err
	}
	if strings.TrimSpace(connectionID) == "" {
		return SwitchConnectionResult{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "connection_id"})
	}
	rec, err := s.deps.Connections.LoadConnection(ctx, connectionID)
	if err != nil {
		return SwitchConnectionResult{}, err
	}
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "connection.switch",
		Scope:           (security.Resource{Type: security.ResourceConfig, ID: rec.ConnectionID}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		ProviderID:      rec.Provider,
		Outcome:         "switch",
		CorrelationID:   corr,
	}); err != nil {
		return SwitchConnectionResult{}, err
	}
	current, err := s.deps.Connections.ActiveConnectionID(ctx)
	if err != nil {
		return SwitchConnectionResult{}, err
	}
	if current == connectionID {
		return SwitchConnectionResult{ActiveConnectionID: connectionID, Replayed: true}, nil
	}
	active, err := s.deps.Connections.SetActiveConnection(ctx, connectionID)
	if err != nil {
		return SwitchConnectionResult{}, err
	}
	return SwitchConnectionResult{ActiveConnectionID: active}, nil
}

// ActiveConnection reads the stored active connection reference (empty
// when none was ever set) — the pure read behind the switch state.
func (s *Slice) ActiveConnection(ctx context.Context) (string, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Connections == nil {
		return "", envelopeErr(errs.CodeStorageUnavailable, corr, map[string]any{"reason": "connection store is not wired"})
	}
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConfig}, security.ActionDataRead); err != nil {
		return "", err
	}
	return s.deps.Connections.ActiveConnectionID(ctx)
}

// authorizeCapture runs the one decision port and returns the decision so
// the command's own audit record can carry the authorization reference
// (the approval-command pattern, applied to every connection write).
func (s *Slice) authorizeCapture(ctx context.Context, corr string, r security.Resource, a security.Action) (security.Decision, error) {
	if s.deps.Guard == nil {
		return security.Decision{}, envelopeErr(errs.CodeInternal, corr, nil)
	}
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: principalID(ctx)}
	pctx := security.PolicyContext{CorrelationID: corr, Credential: credential(ctx)}
	decision, auditErr := s.deps.Guard.Authorize(p, r, a, pctx)
	if auditErr != nil {
		return decision, auditErr // the effect is denied, not executed unrecorded (C2-15)
	}
	if !decision.Allow {
		return decision, envelopeErr(errs.CodeAuthzDenied, corr, map[string]any{
			"resource": r.String(), "action": string(a), "reason": decision.ReasonCode,
		})
	}
	return decision, nil
}

// transientSecretStore is the request-scoped resolution face for an
// UNSAVED draft's typed secret: it answers ONLY the transient handle the
// test plan carries and delegates everything else to the wired store. The
// value lives only in this struct for the single probe and is dropped
// after it — never stored, never logged, never echoed (the P3.2 §3.3
// transient-secret rule; P3.3 §6 rule 5).
type transientSecretStore struct {
	handle security.Handle
	value  []byte
	base   security.SecretStore
}

func (t transientSecretStore) Resolve(ctx context.Context, h security.Handle) ([]byte, error) {
	if h == t.handle {
		return t.value, nil
	}
	if t.base != nil {
		return t.base.Resolve(ctx, h)
	}
	return nil, &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
}

// Status delegates to the wired store; the transient handle reports
// present (the typed value was presented for this explicit action).
func (t transientSecretStore) Status(ctx context.Context, h security.Handle) (security.SecretStatus, error) {
	if h == t.handle {
		return security.SecretPresent, nil
	}
	if t.base != nil {
		return t.base.Status(ctx, h)
	}
	return security.SecretInaccessib, &security.SecretError{Code: errs.CodeStorageUnavailable, Reason: security.ReasonSecretInaccessible, Handle: h.String()}
}

// encodeConnectionListCursor / decodeConnectionListCursor are the opaque
// connection-list cursor codec (the IMPL-4 list-cursor pattern: a
// salt-prefixed hex-JSON envelope, never structured data on the wire).
func encodeConnectionListCursor(rec dprovider.ConnectionRecord) (string, error) {
	if rec.ConnectionID == "" || rec.CreatedAt == "" {
		return "", fmt.Errorf("app: connection list cursor position lacks its keyset values")
	}
	b, err := json.Marshal(connectionListCursorPosition{CreatedAt: rec.CreatedAt, ConnectionID: rec.ConnectionID})
	if err != nil {
		return "", err
	}
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt[:]) + "." + hex.EncodeToString(b), nil
}

func decodeConnectionListCursor(cursor string) (connectionListCursorPosition, error) {
	var pos connectionListCursorPosition
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return pos, fmt.Errorf("app: connection list cursor is malformed")
	}
	b, err := hex.DecodeString(parts[1])
	if err != nil {
		return pos, fmt.Errorf("app: connection list cursor is malformed")
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return pos, fmt.Errorf("app: connection list cursor is malformed")
	}
	if pos.ConnectionID == "" || pos.CreatedAt == "" {
		return pos, fmt.Errorf("app: connection list cursor is malformed")
	}
	return pos, nil
}
