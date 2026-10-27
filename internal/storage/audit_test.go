package storage

import (
	"strings"
	"testing"
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
