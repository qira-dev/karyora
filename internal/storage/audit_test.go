package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAuditAppendAndTail(t *testing.T) {
	s := openTestStore(t)
	if _, hash, err := s.AuditTail(); err != nil || hash != "" {
		t.Fatalf("empty tail: %v %q", err, hash)
	}
	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("b", 64)
	if err := s.AuditAppend(1, "", h1, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := s.AuditAppend(2, h1, h2, []byte(`{"n":2}`)); err != nil {
		t.Fatalf("second append: %v", err)
	}
	seq, tail, err := s.AuditTail()
	if err != nil || seq != 2 || tail != h2 {
		t.Fatalf("tail = %d %q %v", seq, tail, err)
	}
	// The chain can never fork: wrong prev hash or wrong sequence is
	// refused at the durable boundary, whatever the caller computed.
	if err := s.AuditAppend(3, strings.Repeat("c", 64), strings.Repeat("d", 64), []byte(`{}`)); err == nil {
		t.Fatal("a forked prev hash must be refused")
	}
	if err := s.AuditAppend(5, h2, strings.Repeat("d", 64), []byte(`{}`)); err == nil {
		t.Fatal("a skipped sequence must be refused")
	}
	records, err := s.AuditRecords()
	if err != nil || len(records) != 2 {
		t.Fatalf("records round trip: %d %v", len(records), err)
	}
	if string(records[0].Payload) != `{"n":1}` || records[0].PrevHash != "" || records[0].RecHash != h1 {
		t.Fatalf("record row mismatch: %+v", records[0])
	}
}

func TestAuditIntegrityBrokenStopsWrites(t *testing.T) {
	s := openTestStore(t)
	if err := s.AuditAppend(1, "", strings.Repeat("a", 64), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	// The incident process marks the segment; writes stop (C2-15 §2).
	if err := s.SetAuditIntegrityBroken(true); err != nil {
		t.Fatal(err)
	}
	broken, err := s.AuditIntegrityBroken()
	if err != nil || !broken {
		t.Fatalf("integrity flag: %v %v", broken, err)
	}
	if err := s.AuditAppend(2, strings.Repeat("a", 64), strings.Repeat("b", 64), []byte(`{}`)); err == nil {
		t.Fatal("appends must be refused while integrity is broken")
	}
	// Clearing is an explicit action (C1-12), after which the chain
	// continues normally.
	if err := s.SetAuditIntegrityBroken(false); err != nil {
		t.Fatal(err)
	}
	if err := s.AuditAppend(2, strings.Repeat("a", 64), strings.Repeat("b", 64), []byte(`{}`)); err != nil {
		t.Fatalf("append after explicit clear: %v", err)
	}
}

func TestAuditAnchors(t *testing.T) {
	s := openTestStore(t)
	h := strings.Repeat("e", 64)
	if err := s.AuditAnchor(10, h); err != nil {
		t.Fatalf("anchor: %v", err)
	}
	// Idempotent per sequence.
	if err := s.AuditAnchor(10, h); err != nil {
		t.Fatalf("anchor idempotency: %v", err)
	}
	anchors, err := s.AuditAnchors()
	if err != nil || len(anchors) != 1 || anchors[0].Sequence != 10 || anchors[0].Hash != h {
		t.Fatalf("anchors round trip: %+v %v", anchors, err)
	}
	if err := s.AuditAnchor(0, h); err == nil {
		t.Fatal("sequence 0 anchor must be refused")
	}
	if err := s.AuditAnchor(3, "short"); err == nil {
		t.Fatal("malformed anchor hash must be refused")
	}
}

func TestAuditListRowsQuery(t *testing.T) {
	s := openTestStore(t)
	// Three canonical chain frames: two older allow records, one newer
	// deny record — the sequence order IS the time order (append-only).
	mk := func(id, decision string, at time.Time) []byte {
		return []byte(`{"audit_id":"` + id + `","schema_version":1,"time":"` + at.UTC().Format(time.RFC3339Nano) + `","actor":"local-user:x","action":"data.write","scope":"core","policy_decision":"` + decision + `","policy_reference":"ok","outcome":"` + decision + `","correlation_id":"c-` + id + `"}`)
	}
	base := time.Date(2026, 11, 6, 2, 0, 0, 0, time.UTC)
	h1, h2, h3 := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	if err := s.AuditAppend(1, "", h1, mk("r1", "allow", base)); err != nil {
		t.Fatal(err)
	}
	if err := s.AuditAppend(2, h1, h2, mk("r2", "allow", base.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := s.AuditAppend(3, h2, h3, mk("r3", "deny", base.Add(2*time.Minute))); err != nil {
		t.Fatal(err)
	}

	// DESC newest-first, the page bound, and the More sentinel.
	page, err := s.AuditListRows(context.Background(), AuditListQuery{Limit: 2, OrderDesc: true})
	if err != nil {
		t.Fatalf("desc page: %v", err)
	}
	if len(page.Rows) != 2 || !page.More {
		t.Fatalf("desc page = %d rows, more=%v; want 2+more", len(page.Rows), page.More)
	}
	if page.Rows[0].Sequence != 3 || page.Rows[1].Sequence != 2 {
		t.Fatalf("desc order must be newest-first, got %d,%d", page.Rows[0].Sequence, page.Rows[1].Sequence)
	}

	// The keyset continuation: rows with sequence < 2.
	page, err = s.AuditListRows(context.Background(), AuditListQuery{Limit: 10, AfterSequence: 2, OrderDesc: true})
	if err != nil {
		t.Fatalf("keyset page: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Sequence != 1 || page.More {
		t.Fatalf("keyset page = %+v", page.Rows)
	}

	// The closed-register policy filter.
	page, err = s.AuditListRows(context.Background(), AuditListQuery{Limit: 10, PolicyDecision: "deny", OrderDesc: true})
	if err != nil {
		t.Fatalf("filter page: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Sequence != 3 {
		t.Fatalf("the deny filter must select only r3, got %+v", page.Rows)
	}

	// The temporal range (julianday — a real comparison, not a string
	// order): the first minute only.
	page, err = s.AuditListRows(context.Background(), AuditListQuery{
		Limit:    10,
		FromTime: base.Format(time.RFC3339Nano),
		ToTime:   base.Add(time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("range page: %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[0].Sequence != 1 || page.Rows[1].Sequence != 2 {
		t.Fatalf("the range must select r1,r2 chronologically, got %+v", page.Rows)
	}

	// The ASC export order reads chronological.
	page, err = s.AuditListRows(context.Background(), AuditListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("asc page: %v", err)
	}
	if len(page.Rows) != 3 || page.Rows[0].Sequence != 1 || page.Rows[2].Sequence != 3 {
		t.Fatalf("the export order must be chronological, got %+v", page.Rows)
	}

	// A non-positive limit is the typed validation failure.
	if _, err := s.AuditListRows(context.Background(), AuditListQuery{Limit: 0}); err == nil {
		t.Fatal("a non-positive limit must be refused")
	}
}
