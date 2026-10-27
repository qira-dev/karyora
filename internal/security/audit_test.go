package security

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// memAuditStore is the in-memory AuditStore used by the security package's
// own tests. It mirrors the durable contract: append-only, fork-refusing,
// checkpointed, with an integrity-broken flag that stops writes.
type memAuditStore struct {
	records  []ChainedRecord
	anchors  []Anchor
	broken   bool
	failNext bool // transient pressure injection
}

func (m *memAuditStore) Tail() (TailState, error) {
	if m.failNext {
		return TailState{}, errors.New("store unavailable")
	}
	if len(m.records) == 0 {
		return TailState{}, nil
	}
	last := m.records[len(m.records)-1]
	return TailState{Sequence: last.Sequence, RecHash: last.RecHash}, nil
}

func (m *memAuditStore) Append(rec ChainedRecord) error {
	if m.broken {
		return errors.New("integrity broken")
	}
	if m.failNext {
		return errors.New("store unavailable")
	}
	want := uint64(len(m.records) + 1)
	var prev string
	if len(m.records) > 0 {
		prev = m.records[len(m.records)-1].RecHash
	}
	if rec.Sequence != want || rec.PrevHash != prev {
		return fmt.Errorf("fork refused at %d", rec.Sequence)
	}
	m.records = append(m.records, rec)
	return nil
}

func (m *memAuditStore) Records() ([]ChainedRecord, error) { return m.records, nil }

func (m *memAuditStore) Anchor(a Anchor) error {
	m.anchors = append(m.anchors, a)
	return nil
}

func (m *memAuditStore) Anchors() ([]Anchor, error) { return m.anchors, nil }

func (m *memAuditStore) SetIntegrityBroken(b bool) error { m.broken = b; return nil }

func (m *memAuditStore) IntegrityBroken() (bool, error) { return m.broken, nil }

func sampleRecord(n int) AuditRecord {
	return AuditRecord{
		SchemaVersion:   AuditSchemaVersion,
		Actor:           "local-user:local",
		Action:          string(ActionLifecycleStop),
		Scope:           "core:foreground",
		PolicyDecision:  "allow",
		PolicyReference: ReasonOK,
		Outcome:         "allow",
		CorrelationID:   fmt.Sprintf("corr-%d", n),
	}
}

func TestAuditChainAndVerification(t *testing.T) {
	store := &memAuditStore{}
	svc := NewAuditService(store, 16, 2)
	for i := 1; i <= 5; i++ {
		if err := svc.Record(sampleRecord(i)); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	if len(store.records) != 5 {
		t.Fatalf("stored %d records, want 5", len(store.records))
	}
	// Checkpoint cadence (interval 2): anchors at sequences 2 and 4.
	if len(store.anchors) != 2 || store.anchors[0].Sequence != 2 || store.anchors[1].Sequence != 4 {
		t.Fatalf("anchors = %+v, want checkpoints at 2 and 4", store.anchors)
	}
	report, err := svc.Verify()
	if err != nil || !report.OK {
		t.Fatalf("verify: %+v %v", report, err)
	}
	if report.Records != 5 || report.Anchors != 2 {
		t.Fatalf("report = %+v", report)
	}

	// Tamper detection: rewrite one stored payload — the chain breaks at
	// that sequence, the segment is marked integrity-broken, and writes
	// stop (C2-15 §2).
	store.records[2].Payload = []byte(`{"tampered":true}`)
	report, err = svc.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.BrokenAt != 3 {
		t.Fatalf("tampered chain must break at 3: %+v", report)
	}
	if broken, _ := svc.Broken(); !broken {
		t.Fatal("detected tampering must set the visible integrity signal")
	}
	if err := svc.Record(sampleRecord(6)); err == nil {
		t.Fatal("writes must stop while the segment is integrity-broken")
	}
}

func TestAuditCheckpointMismatchDetected(t *testing.T) {
	store := &memAuditStore{}
	svc := NewAuditService(store, 16, 2)
	for i := 1; i <= 4; i++ {
		if err := svc.Record(sampleRecord(i)); err != nil {
			t.Fatal(err)
		}
	}
	// Forge a checkpoint anchor that disagrees with the chain.
	store.anchors[0].Hash = strings.Repeat("0", 64)
	report, err := svc.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Reason != "checkpoint hash mismatch" {
		t.Fatalf("checkpoint mismatch must be detected: %+v", report)
	}
	// A missing checkpoint is also detected.
	store2 := &memAuditStore{}
	svc2 := NewAuditService(store2, 16, 2)
	for i := 1; i <= 4; i++ {
		_ = svc2.Record(sampleRecord(i))
	}
	store2.anchors = nil
	report, _ = svc2.Verify()
	if report.OK || report.Reason != "missing checkpoint" {
		t.Fatalf("missing checkpoint must be detected: %+v", report)
	}
}

func TestAuditReservedQuotaDeniesWhenExhausted(t *testing.T) {
	store := &memAuditStore{}
	svc := NewAuditService(store, 2, 1000)
	store.failNext = true // every append fails (transient pressure)
	// Two records absorb into the reserved quota (effects may proceed);
	// the third denial begins (C2-15 §5).
	if err := svc.Record(sampleRecord(1)); err != nil {
		t.Fatalf("quota must absorb the first record: %v", err)
	}
	if err := svc.Record(sampleRecord(2)); err != nil {
		t.Fatalf("quota must absorb the second record: %v", err)
	}
	err := svc.Record(sampleRecord(3))
	var unavail *ErrAuditUnavailable
	if !errors.As(err, &unavail) {
		t.Fatalf("quota exhaustion must deny with AUDIT_UNAVAILABLE, got %v", err)
	}
	env, envErr := unavail.Envelope("corr")
	if envErr != nil || env.Code != "STORAGE_UNAVAILABLE" || env.Data["reason"] != ReasonAuditUnavailable {
		t.Fatalf("typed envelope: %+v %v", env, envErr)
	}
	// Pressure clears: the drain flushes the two buffered records durably,
	// in order, with an intact chain. The third record was DENIED (never
	// buffered) — its effect never happened, so nothing of it persists.
	store.failNext = false
	if err := svc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if len(store.records) != 2 {
		t.Fatalf("flushed %d records, want the 2 accepted ones", len(store.records))
	}
	report, err := svc.Verify()
	if err != nil || !report.OK {
		t.Fatalf("chain must verify after flush: %+v %v", report, err)
	}
}

func TestAuditRecordValidationRefusesBadReferences(t *testing.T) {
	store := &memAuditStore{}
	svc := NewAuditService(store, 4, 1000)
	rec := sampleRecord(1)
	rec.ContentRefs = []string{"not-a-digest"}
	if err := svc.Record(rec); err == nil {
		t.Fatal("a malformed content reference must be refused")
	}
	rec = sampleRecord(2)
	rec.ContentDigests = []string{"sha256:" + strings.Repeat("ab", 32)}
	if err := svc.Record(rec); err != nil {
		t.Fatalf("digest-grammar references must record: %v", err)
	}
}

func TestAuditCanonicalIsDeterministic(t *testing.T) {
	a := sampleRecord(1)
	a.AuditID = "id"
	a.Time = "2026-10-27T00:00:00Z"
	b := sampleRecord(1)
	b.AuditID = "id"
	b.Time = "2026-10-27T00:00:00Z"
	pa, err := a.canonical()
	if err != nil {
		t.Fatal(err)
	}
	pb, err := b.canonical()
	if err != nil {
		t.Fatal(err)
	}
	if string(pa) != string(pb) {
		t.Fatal("canonical form must be deterministic for identical records")
	}
	if ChainHash("", 1, pa) == ChainHash("", 2, pa) {
		t.Fatal("chain position must change the record hash")
	}
}
