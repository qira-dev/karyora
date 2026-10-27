package storage

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/events"
)

// C2-13 §2: the state change, journal row, and outbox row commit in ONE
// transaction with the full event identity and per-aggregate sequence.
func TestCommitStateChangeAtomic(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	err := s.CommitStateChange(ctx, "c1", func(tx *sqlTx) error {
		_, err := tx.Exec(`INSERT INTO cas_references (blob_ref, owner_type, owner_id, registered_at) VALUES ('sha256:aa', 't', '1', 'now')`)
		return err
	},
		events.Record{EventID: "e1", AggregateType: "job", AggregateID: "j1", EventType: "job.created", InlinePayload: []byte(`{}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	var seq int
	if err := s.readers.QueryRow(`SELECT sequence FROM events_journal WHERE event_id = 'e1'`).Scan(&seq); err != nil {
		t.Fatalf("journal row must exist: %v", err)
	}
	if seq != 1 {
		t.Fatalf("sequence = %d, want 1", seq)
	}
	var published int
	if err := s.readers.QueryRow(`SELECT published FROM events_outbox WHERE event_id = 'e1'`).Scan(&published); err != nil {
		t.Fatalf("outbox row must exist atomically: %v", err)
	}
	if published != 0 {
		t.Fatal("a fresh outbox row is unpublished")
	}
	// A failed state write rolls back the journal+outbox rows too.
	err = s.CommitStateChange(ctx, "c1", func(tx *sqlTx) error {
		return errors.New("state write failed")
	}, events.Record{EventID: "e2", AggregateType: "job", AggregateID: "j1", EventType: "job.created", InlinePayload: []byte(`{}`)})
	if err == nil {
		t.Fatal("state failure must roll back the whole commit")
	}
	var count int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM events_journal WHERE event_id = 'e2'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("a rolled-back commit must not leave a journal row")
	}
}

// C2-13 §4: sequences are monotonic per aggregate; global ordering is not
// implied.
func TestPerAggregateSequences(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mk := func(id, eid string) events.Record {
		return events.Record{EventID: eid, AggregateType: "job", AggregateID: id, EventType: "job.created", InlinePayload: []byte(`{}`)}
	}
	if err := s.CommitStateChange(ctx, "c1", nil, mk("a", "ea1")); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitStateChange(ctx, "c1", nil, mk("b", "eb1")); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitStateChange(ctx, "c1", nil, mk("a", "ea2")); err != nil {
		t.Fatal(err)
	}
	var seqA2, seqB1 int
	if err := s.readers.QueryRow(`SELECT sequence FROM events_journal WHERE event_id = 'ea2'`).Scan(&seqA2); err != nil {
		t.Fatal(err)
	}
	if err := s.readers.QueryRow(`SELECT sequence FROM events_journal WHERE event_id = 'eb1'`).Scan(&seqB1); err != nil {
		t.Fatal(err)
	}
	if seqA2 != 2 || seqB1 != 1 {
		t.Fatalf("per-aggregate sequences: a2=%d b1=%d, want 2 and 1", seqA2, seqB1)
	}
}

// C2-13 §3: at-least-once delivery with a deduplicating consumer; the
// outbox row is marked published only after consumers accept.
func TestPublisherDeliveryAndDedup(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.CommitStateChange(ctx, "c1", nil,
		events.Record{EventID: "e1", AggregateType: "job", AggregateID: "j1", EventType: "job.created", InlinePayload: []byte(`{}`)},
		events.Record{EventID: "e2", AggregateType: "job", AggregateID: "j1", EventType: "job.updated", InlinePayload: []byte(`{}`)},
	); err != nil {
		t.Fatal(err)
	}
	collected := make([]string, 0, 4)
	var mu sync.Mutex
	appendCollected := func(id string) {
		mu.Lock()
		collected = append(collected, id)
		mu.Unlock()
	}
	countCollected := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(collected)
	}
	tc := &testConsumer{name: "test", onConsume: appendCollected}
	dedup := NewDedupConsumer(s, tc)
	p := NewPublisher(s, 10*time.Millisecond, 16)
	p.Subscribe(dedup)
	p.Start()
	t.Cleanup(p.Stop)
	deadline := time.Now().Add(3 * time.Second)
	for countCollected() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if countCollected() != 2 {
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("delivered %v, want both events", collected)
	}
	var unpublished int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM events_outbox WHERE published = 0`).Scan(&unpublished); err != nil {
		t.Fatal(err)
	}
	if unpublished != 0 {
		t.Fatalf("%d rows still unpublished after delivery", unpublished)
	}
	// Duplicate delivery is dropped by event_id (at-least-once + idempotent
	// consumer is the entire delivery contract).
	rec, err := fetchOne(ctx, s, "e1")
	if err != nil {
		t.Fatal(err)
	}
	before := countCollected()
	if err := dedup.Consume(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if countCollected() != before {
		t.Fatal("duplicate delivery must be dropped by event_id")
	}
}

func fetchOne(ctx context.Context, s *Store, id string) (events.Record, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT event_id, aggregate_type, aggregate_id, sequence, event_type, schema_version, occurred_at, causation_id, correlation_id, producer, payload, payload_digest, blob_ref FROM events_journal WHERE event_id = ?`, id)
	if err != nil {
		return events.Record{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return events.Record{}, errors.New("event not found")
	}
	var r events.Record
	var occurred string
	var payload []byte
	if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.Sequence, &r.EventType, &r.SchemaVersion, &occurred, &r.CausationID, &r.CorrelationID, &r.Producer, &payload, &r.PayloadDigest, &r.BlobRef); err != nil {
		return events.Record{}, err
	}
	r.InlinePayload = payload
	return r, nil
}

type testConsumer struct {
	name      string
	onConsume func(string)
}

func (c *testConsumer) Name() string { return c.name }

func (c *testConsumer) Consume(ctx context.Context, rec events.Record) error {
	c.onConsume(rec.EventID)
	return nil
}

// C2-13 §6: retention cannot erase undelivered outbox entries — delivery
// completion, not age, is the precondition.
func TestRetentionKeepsUndelivered(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.CommitStateChange(ctx, "c1", nil,
		events.Record{EventID: "delivered", AggregateType: "job", AggregateID: "j1", EventType: "e", InlinePayload: []byte(`{}`)},
		events.Record{EventID: "pending", AggregateType: "job", AggregateID: "j1", EventType: "e", InlinePayload: []byte(`{}`)},
	); err != nil {
		t.Fatal(err)
	}
	if err := s.markPublished(ctx, "delivered"); err != nil {
		t.Fatal(err)
	}
	n, err := s.ApplyRetention(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("retention removed %d rows, want only the delivered one", n)
	}
	var count int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM events_journal WHERE event_id = 'pending'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("retention must never erase an undelivered event")
	}
}

// C2-13 §5: replay gap resolution — a cursor behind a sequence gap gets the
// explicit replay_gap outcome and the refetch path, never a false permanent
// status.
func TestReplayGapAndCursor(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.CommitStateChange(ctx, "c1", nil,
		events.Record{EventID: "g1", AggregateType: "job", AggregateID: "j1", EventType: "e", InlinePayload: []byte(`{}`)},
		events.Record{EventID: "g2", AggregateType: "job", AggregateID: "j1", EventType: "e", InlinePayload: []byte(`{}`)},
	); err != nil {
		t.Fatal(err)
	}
	// Simulate retention removing the first event (and its outbox row).
	if _, err := s.writers.Exec(`DELETE FROM events_outbox WHERE event_id = 'g1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writers.Exec(`DELETE FROM events_journal WHERE event_id = 'g1'`); err != nil {
		t.Fatal(err)
	}
	cursor, err := events.EncodeCursor("job", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	recs, outcome, err := s.Replay(ctx, "job", cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != events.ReplayGap {
		t.Fatalf("outcome = %s, want replay_gap", outcome)
	}
	if recs != nil {
		t.Fatal("a gap response carries no historical data")
	}
	// A cursor at the current head returns an empty ok page.
	cursor, err = events.EncodeCursor("job", 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	recs, outcome, err = s.Replay(ctx, "job", cursor, 10)
	if err != nil || outcome != events.ReplayOK {
		t.Fatalf("head cursor: %v %s", err, outcome)
	}
	if len(recs) != 0 {
		t.Fatalf("head cursor must return no rows, got %d", len(recs))
	}
	// A malformed cursor is expired, never a crash.
	_, outcome, err = s.Replay(ctx, "job", "not-a-cursor", 10)
	if err != nil || outcome != events.ReplayCursorExpired {
		t.Fatalf("malformed cursor: %v %s", err, outcome)
	}
}

// C2-09 step 4 / C2-13: reconciliation is idempotent; an inconsistent
// journal/outbox pair is STATE_CORRUPT, never silently repaired.
func TestReconcileOutboxDetectsInconsistency(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.ReconcileOutbox(ctx); err != nil {
		t.Fatalf("consistent store reconciles cleanly: %v", err)
	}
	if err := tamperOrphanOutbox(s.opts.Path); err != nil {
		t.Fatal(err)
	}
	err := s.ReconcileOutbox(ctx)
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeStateCorrupt {
		t.Fatalf("inconsistency err = %v, want STATE_CORRUPT", err)
	}
}

// tamperOrphanOutbox simulates a tampered/corrupt database: a raw
// connection without the verified hook has foreign keys off, so an orphan
// outbox row can be planted the way a corrupted or hand-edited database
// would carry one. The consistency check must detect it (STATE_CORRUPT).
func tamperOrphanOutbox(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`PRAGMA foreign_keys=OFF; INSERT INTO events_outbox (event_id, published) VALUES ('orphan', 0)`)
	return err
}

// The full startup port sequence: migrate → reconcile → bump → fence.
func TestStartupPortSequence(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.Migrate(ctx, "test-version"); err != nil {
		t.Fatalf("migrate step: %v", err)
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile step: %v", err)
	}
	defer s.StopPublisher()
	gen, err := s.BumpGeneration(ctx)
	if err != nil {
		t.Fatalf("generation step: %v", err)
	}
	if gen != 1 {
		t.Fatalf("first generation = %d, want 1", gen)
	}
	if err := s.FencePrior(ctx, "c1", gen); err != nil {
		t.Fatalf("fence step: %v", err)
	}
	// The fence is recorded in the journal (visible, never silent).
	var count int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM events_journal WHERE event_type = 'core.fence.completed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("fence event count = %d, want 1", count)
	}
}
