package httplocal

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// The F12 audit/activity wire tests (IMPL-24, the adopted S11 classes
// over the wire): the permission-filtered, server-side, paginated list
// and the M-32-class read-with-export — the read-only law, the
// closed-register filter, the opaque cursor, the typed refusals, and the
// honest truncation, all against the real listener and the real storage
// adapter. The reads' own authorizations produce real decision records
// (the coded Guard/audit coupling), so the assertions are shape- and
// order-based, never exact-count-based.

// storageAuditStore adapts the test store to the security.AuditStore port
// (the composition root's lazyAuditStore twin) so the audit tests mirror
// the REAL wiring: the Guard's decision records land in the durable
// audit_records table the read surface serves — not in a test sink.
type storageAuditStore struct{ st *storage.Store }

func (a storageAuditStore) Tail() (security.TailState, error) {
	seq, hash, err := a.st.AuditTail()
	return security.TailState{Sequence: seq, RecHash: hash}, err
}

func (a storageAuditStore) Append(rec security.ChainedRecord) error {
	return a.st.AuditAppend(rec.Sequence, rec.PrevHash, rec.RecHash, rec.Payload)
}

func (a storageAuditStore) Records() ([]security.ChainedRecord, error) {
	rows, err := a.st.AuditRecords()
	if err != nil {
		return nil, err
	}
	out := make([]security.ChainedRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, security.ChainedRecord{Sequence: r.Sequence, PrevHash: r.PrevHash, RecHash: r.RecHash, Payload: r.Payload})
	}
	return out, nil
}

func (a storageAuditStore) Anchor(an security.Anchor) error {
	return a.st.AuditAnchor(an.Sequence, an.Hash)
}

func (a storageAuditStore) Anchors() ([]security.Anchor, error) {
	rows, err := a.st.AuditAnchors()
	if err != nil {
		return nil, err
	}
	out := make([]security.Anchor, 0, len(rows))
	for _, r := range rows {
		out = append(out, security.Anchor{Sequence: r.Sequence, Hash: r.Hash})
	}
	return out, nil
}

func (a storageAuditStore) SetIntegrityBroken(broken bool) error {
	return a.st.SetAuditIntegrityBroken(broken)
}

func (a storageAuditStore) IntegrityBroken() (bool, error) {
	return a.st.AuditIntegrityBroken()
}

// wireRealAuditStore re-wires the shared env's Guard audit sink to the
// real durable store (the composition root's own wiring shape), so the
// reads below serve the decision records the surface itself produces.
func wireRealAuditStore(env *testEnv) *storage.Store {
	st := env.deps.AuditReader.(auditTestReader).st
	env.deps.Guard.Audit = security.NewAuditService(storageAuditStore{st: st}, 64, 1000)
	return st
}

// seedAuditRows appends canonical chain frames directly (bounded,
// schema-valid payloads the surface then serves).
func seedAuditRows(t *testing.T, st *storage.Store, n int) {
	t.Helper()
	h := strings.Repeat("a", 64)
	for i := 0; i < n; i++ {
		rec := map[string]any{
			"audit_id": "seed-" + string(rune('a'+i)), "schema_version": 1,
			"time":  time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			"actor": "local-user:test", "action": "data.write", "scope": "core",
			"policy_decision": "allow", "policy_reference": "ok", "outcome": "allow",
			"correlation_id": "c-seed-" + string(rune('a'+i)),
		}
		payload, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var prev string
		if i > 0 {
			prev = h
		}
		if err := st.AuditAppend(uint64(i+1), prev, h, payload); err != nil {
			t.Fatalf("seed append %d: %v", i+1, err)
		}
	}
}

func TestAuditListEndpoint(t *testing.T) {
	env := newTestEnv(t)
	st := wireRealAuditStore(env)
	seedAuditRows(t, st, 3)

	status, body := env.do(t, http.MethodGet, "/v1/audit?limit=50", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the audit list must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Entries []struct {
			Sequence uint64 `json:"sequence"`
			Record   struct {
				AuditID        string `json:"audit_id"`
				SchemaVersion  int    `json:"schema_version"`
				Time           string `json:"time"`
				Actor          string `json:"actor"`
				Action         string `json:"action"`
				Scope          string `json:"scope"`
				PolicyDecision string `json:"policy_decision"`
				Outcome        string `json:"outcome"`
				CorrelationID  string `json:"correlation_id"`
			} `json:"record"`
		} `json:"entries"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if len(res.Entries) == 0 {
		t.Fatalf("the list must serve the decision records the surface itself produces: %s", body)
	}
	// The Core-defined order: chain sequence descending (newest first).
	for i := 1; i < len(res.Entries); i++ {
		if res.Entries[i-1].Sequence <= res.Entries[i].Sequence {
			t.Fatalf("the list must read newest-first, got %d then %d", res.Entries[i-1].Sequence, res.Entries[i].Sequence)
		}
	}
	// The adopted [S] schema rides verbatim — every record carries the
	// coded members, never an invented field.
	for _, e := range res.Entries {
		if e.Record.AuditID == "" || e.Record.Time == "" || e.Record.Actor == "" || e.Record.CorrelationID == "" {
			t.Fatalf("the record schema must ride verbatim, got %+v", e.Record)
		}
		if e.Record.SchemaVersion != 1 {
			t.Fatalf("the schema version must be the coded 1, got %d", e.Record.SchemaVersion)
		}
		if _, err := time.Parse(time.RFC3339Nano, e.Record.Time); err != nil {
			t.Fatalf("the record time must parse RFC3339Nano, got %q", e.Record.Time)
		}
	}

	// The closed-register filter (the decision port's own allow/deny
	// vocabulary): a deny-only page carries only deny records.
	status, body = env.do(t, http.MethodGet, "/v1/audit?limit=50&policy_decision=deny", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the filtered list must answer 200, got %d: %s", status, body)
	}
	var filtered struct {
		Entries []struct {
			Record struct {
				PolicyDecision string `json:"policy_decision"`
			} `json:"record"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &filtered); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	for _, e := range filtered.Entries {
		if e.Record.PolicyDecision != "deny" {
			t.Fatalf("the deny filter must serve only deny records, got %q", e.Record.PolicyDecision)
		}
	}

	// The bounded page + the opaque continuation over the wire.
	status, body = env.do(t, http.MethodGet, "/v1/audit?limit=1", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the bounded page must answer 200, got %d: %s", status, body)
	}
	var page1 struct {
		Entries []struct {
			Sequence uint64 `json:"sequence"`
		} `json:"entries"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(body, &page1); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if len(page1.Entries) != 1 || page1.Cursor == "" {
		t.Fatalf("the bounded page must carry one row and a cursor, got %+v cursor=%q", page1.Entries, page1.Cursor)
	}
	status, body = env.do(t, http.MethodGet, "/v1/audit?limit=1&cursor="+page1.Cursor, "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the continuation must answer 200, got %d: %s", status, body)
	}
	var page2 struct {
		Entries []struct {
			Sequence uint64 `json:"sequence"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &page2); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if len(page2.Entries) != 1 || page2.Entries[0].Sequence >= page1.Entries[0].Sequence {
		t.Fatalf("the continuation must advance the keyset, got %+v after %d", page2.Entries, page1.Entries[0].Sequence)
	}
}

func TestAuditExportEndpoint(t *testing.T) {
	env := newTestEnv(t)
	st := wireRealAuditStore(env)
	seedAuditRows(t, st, 3)

	status, body := env.do(t, http.MethodGet, "/v1/audit/export", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the export must answer 200, got %d: %s", status, body)
	}
	var res struct {
		Records []struct {
			Sequence uint64 `json:"sequence"`
			Record   struct {
				AuditID string `json:"audit_id"`
			} `json:"record"`
		} `json:"records"`
		Count       int    `json:"count"`
		From        string `json:"from"`
		To          string `json:"to"`
		GeneratedAt string `json:"generated_at"`
		Truncated   bool   `json:"truncated"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if res.Count < 1 || len(res.Records) != res.Count {
		t.Fatalf("the export must carry the honest count, got count=%d records=%d", res.Count, len(res.Records))
	}
	for i := 1; i < len(res.Records); i++ {
		if res.Records[i-1].Sequence >= res.Records[i].Sequence {
			t.Fatalf("the export must read chronologically, got %d then %d", res.Records[i-1].Sequence, res.Records[i].Sequence)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, res.GeneratedAt); err != nil {
		t.Fatalf("the export must carry a parseable generation instant, got %q", res.GeneratedAt)
	}
	if res.Truncated {
		t.Fatal("the uncapped export must not report truncation")
	}

	// The finite bound truncates honestly over the wire.
	status, body = env.do(t, http.MethodGet, "/v1/audit/export?limit=1", "", env.token, nil)
	if status != http.StatusOK {
		t.Fatalf("the capped export must answer 200, got %d: %s", status, body)
	}
	var capped struct {
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(body, &capped); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if capped.Count != 1 || !capped.Truncated {
		t.Fatalf("the capped export must report the truncation honestly, got count=%d truncated=%v", capped.Count, capped.Truncated)
	}
}

func TestAuditReadSurfaceRefusals(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
		wantCode   string
	}{
		{name: "list_unauthenticated", method: http.MethodGet, path: "/v1/audit", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "export_unauthenticated", method: http.MethodGet, path: "/v1/audit/export", token: "", wantStatus: http.StatusForbidden, wantCode: "AUTHZ_DENIED"},
		{name: "list_limit_bound", method: http.MethodGet, path: "/v1/audit?limit=1001", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "list_filter_outside_register", method: http.MethodGet, path: "/v1/audit?policy_decision=maybe", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "list_cursor_malformed", method: http.MethodGet, path: "/v1/audit?cursor=zzz", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "export_from_malformed", method: http.MethodGet, path: "/v1/audit/export?from=not-a-time", token: env.token, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_FAILED"},
		{name: "list_post_is_no_route", method: http.MethodPost, path: "/v1/audit", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "export_post_is_no_route", method: http.MethodPost, path: "/v1/audit/export", token: env.token, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := env.do(t, tc.method, tc.path, "", tc.token, nil)
			if status != tc.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", tc.name, tc.wantStatus, status, body)
			}
			if !strings.Contains(string(body), tc.wantCode) {
				t.Fatalf("%s: the refusal must carry the typed %s code: %s", tc.name, tc.wantCode, body)
			}
		})
	}
}
