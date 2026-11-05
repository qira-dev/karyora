package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// fixtureAuditReader is the in-memory AuditReader fixture: it applies the
// closed-register filter, the DESC keyset, and the limit/More semantics
// honestly over its rows (the storage-level query semantics — keyset,
// range, orders — are proven against the real store in the storage and
// real-listener registers). The prior rows are read-only: the fixture
// exposes them for the never-mutated pin.
type fixtureAuditReader struct {
	rows []AuditRow
	// decisions parallels rows (the closed-register filter's input).
	decisions []string
}

func (f *fixtureAuditReader) ListAuditRows(ctx context.Context, q AuditRowsQuery) (AuditRowsPage, error) {
	selected := make([]AuditRow, 0, len(f.rows))
	for i, row := range f.rows {
		if q.PolicyDecision != "" && f.decisions[i] != q.PolicyDecision {
			continue
		}
		if q.OrderDesc && q.AfterSequence != 0 && row.Sequence >= q.AfterSequence {
			continue
		}
		selected = append(selected, row)
	}
	if q.OrderDesc {
		// newest (highest sequence) first
		for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
			selected[i], selected[j] = selected[j], selected[i]
		}
	}
	more := len(selected) > q.Limit
	if more {
		selected = selected[:q.Limit]
	}
	return AuditRowsPage{Rows: selected, More: more}, nil
}

// auditTestEnv carries the slice wired with the fixture reader and the
// session fixture's real Guard/audit machinery (the decision records the
// reads' own authorizations produce are real).
type auditTestEnv struct {
	*sessionTestEnv
	fixture *fixtureAuditReader
}

func newAuditTestEnv(t *testing.T) *auditTestEnv {
	t.Helper()
	base := newSessionTestEnv(t)
	fixture := &fixtureAuditReader{}
	base.slice.deps.AuditReader = fixture
	return &auditTestEnv{sessionTestEnv: base, fixture: fixture}
}

func (e *auditTestEnv) seed(t *testing.T, id string, decision string, at time.Time) {
	t.Helper()
	rec := security.AuditRecord{
		AuditID: id, SchemaVersion: security.AuditSchemaVersion,
		Time: at.UTC().Format(time.RFC3339Nano), Actor: "local-user:test",
		Action: "data.write", Scope: "core", PolicyDecision: decision,
		PolicyReference: "ok", Outcome: decision, CorrelationID: "c-" + id,
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	e.fixture.rows = append(e.fixture.rows, AuditRow{Sequence: uint64(len(e.fixture.rows) + 1), Payload: payload})
	e.fixture.decisions = append(e.fixture.decisions, decision)
}

func (e *auditTestEnv) ctxWith(token string) context.Context {
	return WithCaller(context.Background(), "local", token)
}

// TestAuditListPageAndCursor pins the card-9 class over the audit read:
// the Core-defined newest-first order, the bounded pages, the opaque
// keyset cursor, and the closed-register filter — and the read-only pin:
// the read mutates nothing (the fixture's rows are byte-identical
// before/after, and the read-only surface offers no correction path at
// all).
func TestAuditListPageAndCursor(t *testing.T) {
	env := newAuditTestEnv(t)
	token := env.sessionToken(t)
	base := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)
	env.seed(t, "r1", "allow", base)
	env.seed(t, "r2", "allow", base.Add(time.Minute))
	env.seed(t, "r3", "deny", base.Add(2*time.Minute))
	env.seed(t, "r4", "allow", base.Add(3*time.Minute))
	env.seed(t, "r5", "allow", base.Add(4*time.Minute))
	before := make([]string, 0, len(env.fixture.rows))
	for _, r := range env.fixture.rows {
		before = append(before, string(r.Payload))
	}

	res, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Entries) != 5 || res.NextCursor != "" {
		t.Fatalf("the default page must carry all rows, got %d entries cursor=%q", len(res.Entries), res.NextCursor)
	}
	if res.Entries[0].Sequence != 5 || res.Entries[4].Sequence != 1 {
		t.Fatalf("the list must read newest-first, got %d..%d", res.Entries[0].Sequence, res.Entries[4].Sequence)
	}
	if res.Entries[0].Record.AuditID != "r5" || res.Entries[0].Record.SchemaVersion != security.AuditSchemaVersion {
		t.Fatalf("the record schema must ride verbatim, got %+v", res.Entries[0].Record)
	}

	// The bounded page + the opaque continuation: [5,4] → [3,2] → [1].
	page1, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Entries) != 2 || page1.Entries[0].Sequence != 5 || page1.NextCursor == "" {
		t.Fatalf("page1 = %d entries cursor=%q; want 2 + a cursor", len(page1.Entries), page1.NextCursor)
	}
	page2, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Entries) != 2 || page2.Entries[0].Sequence != 3 || page2.NextCursor == "" {
		t.Fatalf("page2 = %d entries cursor=%q; want the 3,2 continuation", len(page2.Entries), page2.NextCursor)
	}
	page3, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{Limit: 2, Cursor: page2.NextCursor})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3.Entries) != 1 || page3.Entries[0].Sequence != 1 || page3.NextCursor != "" {
		t.Fatalf("the final page must be the cursorless last row, got %+v", page3.Entries)
	}

	// The closed-register filter: the Core's own allow/deny vocabulary.
	denied, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{PolicyDecision: "deny"})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(denied.Entries) != 1 || denied.Entries[0].Record.AuditID != "r3" {
		t.Fatalf("the deny filter must select only r3, got %+v", denied.Entries)
	}

	// THE READ-ONLY PIN (the S11 §4 P-S11-02 law): the prior records are
	// NEVER mutated — the rows are byte-identical after every read above.
	for i, r := range env.fixture.rows {
		if string(r.Payload) != before[i] {
			t.Fatalf("the read surface must never mutate a prior record, row %d changed", r.Sequence)
		}
	}
	if len(env.fixture.rows) != 5 {
		t.Fatal("the read surface must never append, delete, or rewrite a record")
	}
}

// TestAuditListRefusals pins the typed refusal register: the decision
// port's denials and the typed validations — every refusal carries the
// D-35 envelope, and no refusal mutates anything.
func TestAuditListRefusals(t *testing.T) {
	env := newAuditTestEnv(t)
	token := env.sessionToken(t)
	env.seed(t, "r1", "allow", time.Now())

	cases := []struct {
		name      string
		ctx       context.Context
		req       AuditListRequest
		wantCode  errs.Code
		wantField string
	}{
		{name: "no_credential", ctx: context.Background(), req: AuditListRequest{}, wantCode: errs.CodeAuthzDenied},
		{name: "garbage_credential", ctx: env.ctxWith("kry1-not-a-real-token-000000000000"), req: AuditListRequest{}, wantCode: errs.CodeAuthzDenied},
		{name: "limit_over_bound", ctx: env.ctxWith(token), req: AuditListRequest{Limit: MaxAuditListLimit + 1}, wantCode: errs.CodeValidationFailed, wantField: "limit"},
		{name: "limit_negative", ctx: env.ctxWith(token), req: AuditListRequest{Limit: -1}, wantCode: errs.CodeValidationFailed, wantField: "limit"},
		{name: "filter_outside_register", ctx: env.ctxWith(token), req: AuditListRequest{PolicyDecision: "maybe"}, wantCode: errs.CodeValidationFailed, wantField: "policy_decision"},
		{name: "cursor_malformed", ctx: env.ctxWith(token), req: AuditListRequest{Cursor: "not-a-cursor"}, wantCode: errs.CodeValidationFailed, wantField: "cursor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.slice.ListAudit(tc.ctx, tc.req)
			if err == nil {
				t.Fatalf("%s must refuse", tc.name)
			}
			var ee *EnvelopeError
			if !errors.As(err, &ee) {
				t.Fatalf("%s: expected a typed D-35 envelope, got %v", tc.name, err)
			}
			if ee.Envelope.Code != tc.wantCode {
				t.Fatalf("%s: expected %s, got %s", tc.name, tc.wantCode, ee.Envelope.Code)
			}
			if tc.wantField != "" {
				if field, _ := ee.Envelope.Data["field"].(string); field != tc.wantField {
					t.Fatalf("%s: the field must ride the envelope data, got %v", tc.name, ee.Envelope.Data)
				}
			}
		})
	}

	// The unwired posture: the nil port refuses typed (fail closed).
	env.slice.deps.AuditReader = nil
	if _, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{}); err == nil {
		t.Fatal("the unwired audit surface must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeStorageUnavailable {
		t.Fatalf("the unwired refusal must be STORAGE_UNAVAILABLE, got %q", env.Code)
	}
}

// TestAuditExportRangeAndBound pins the M-32 class as implemented: the
// chronological export document, the honest count/generated-at, the
// finite bound with the honest truncation flag, the range validation,
// and the safe re-issue (two identical requests return the identical
// record set — no idempotency key exists, none is needed).
func TestAuditExportRangeAndBound(t *testing.T) {
	env := newAuditTestEnv(t)
	token := env.sessionToken(t)
	base := time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC)
	env.seed(t, "e1", "allow", base)
	env.seed(t, "e2", "deny", base.Add(time.Minute))
	env.seed(t, "e3", "allow", base.Add(2*time.Minute))
	env.seed(t, "e4", "allow", base.Add(3*time.Minute))

	res, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.Count != 4 || len(res.Records) != 4 || res.Truncated {
		t.Fatalf("the full bounded export must carry all rows, got count=%d truncated=%v", res.Count, res.Truncated)
	}
	if res.Records[0].Sequence != 1 || res.Records[3].Sequence != 4 {
		t.Fatalf("the export must read chronologically, got %d..%d", res.Records[0].Sequence, res.Records[3].Sequence)
	}
	if _, err := time.Parse(time.RFC3339Nano, res.GeneratedAt); err != nil {
		t.Fatalf("the export must carry a parseable generation instant, got %q", res.GeneratedAt)
	}

	// The finite bound: a range exceeding it truncates honestly — never
	// silently.
	capped, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{Limit: 2})
	if err != nil {
		t.Fatalf("capped export: %v", err)
	}
	if capped.Count != 2 || !capped.Truncated {
		t.Fatalf("the capped export must report the truncation honestly, got count=%d truncated=%v", capped.Count, capped.Truncated)
	}

	// The date-range validation and the honest echo.
	ranged, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{
		FromTime: base.Format(time.RFC3339Nano),
		ToTime:   base.Add(time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("ranged export: %v", err)
	}
	if ranged.From == "" || ranged.To == "" {
		t.Fatal("the export must echo the confirmed range")
	}
	if _, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{FromTime: "not-a-time"}); err == nil {
		t.Fatal("a malformed from bound must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeValidationFailed {
		t.Fatalf("the malformed bound must be VALIDATION_FAILED, got %q", env.Code)
	}
	if _, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{
		FromTime: base.Add(time.Hour).Format(time.RFC3339Nano),
		ToTime:   base.Format(time.RFC3339Nano),
	}); err == nil {
		t.Fatal("an inverted range must refuse")
	}

	// The safe re-issue: the same request twice returns the identical
	// record set (no idempotency key, no side effect).
	again, err := env.slice.ExportAudit(env.ctxWith(token), AuditExportRequest{})
	if err != nil {
		t.Fatalf("re-issue: %v", err)
	}
	if len(again.Records) != len(res.Records) {
		t.Fatal("the safe re-issue must return the identical record set")
	}
	for i := range res.Records {
		if again.Records[i].Record.AuditID != res.Records[i].Record.AuditID {
			t.Fatalf("the re-issue diverged at %d", i)
		}
	}

	// The failure law: the export refuses a dead credential like every
	// read, typed, and mutates nothing.
	if _, err := env.slice.ExportAudit(env.ctxWith("kry1-not-a-real-token-000000000000"), AuditExportRequest{}); err == nil {
		t.Fatal("a garbage-credential export must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeAuthzDenied {
		t.Fatalf("the refusal must be AUTHZ_DENIED, got %q", env.Code)
	}
}

// TestAuditDecodeCorruptionPinned pins the honesty rule for undecodable
// payloads: the typed STATE_CORRUPT — never a skipped row, never an
// invented record.
func TestAuditDecodeCorruptionPinned(t *testing.T) {
	env := newAuditTestEnv(t)
	token := env.sessionToken(t)
	env.fixture.rows = append(env.fixture.rows, AuditRow{Sequence: 1, Payload: []byte("{not json")})
	env.fixture.decisions = append(env.fixture.decisions, "allow")

	if _, err := env.slice.ListAudit(env.ctxWith(token), AuditListRequest{}); err == nil {
		t.Fatal("an undecodable payload must refuse")
	} else if env := envelopeOf(t, err); env.Code != errs.CodeStateCorrupt {
		t.Fatalf("the corruption must be typed STATE_CORRUPT, got %q", env.Code)
	}
}
