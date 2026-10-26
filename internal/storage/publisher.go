package storage

import (
	"context"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/events"
)

// Consumer is one registered in-process delivery target (C2-13 §3).
// Consumers must persist their deduplication state before acknowledging —
// see DedupConsumer for the ledger-backed wrapper.
type Consumer interface {
	Name() string
	Consume(ctx context.Context, rec events.Record) error
}

// DedupConsumer wraps a Consumer with the durable delivered-event ledger:
// the event_id is recorded BEFORE the consumer's application is
// acknowledged, and duplicates are dropped by event_id (C2-13 §3).
type DedupConsumer struct {
	store *Store
	next  Consumer
}

// NewDedupConsumer builds the ledger-backed consumer wrapper.
func NewDedupConsumer(store *Store, next Consumer) *DedupConsumer {
	return &DedupConsumer{store: store, next: next}
}

// Name returns the wrapped consumer's stable name.
func (d *DedupConsumer) Name() string { return d.next.Name() }

// Consume drops duplicates by event_id and applies once.
func (d *DedupConsumer) Consume(ctx context.Context, rec events.Record) error {
	delivered, err := d.store.Delivered(ctx, d.next.Name(), rec.EventID)
	if err != nil {
		return err
	}
	if delivered {
		return nil
	}
	// Persist the dedup state BEFORE acknowledging application.
	if err := d.store.RecordDelivery(ctx, d.next.Name(), rec.EventID); err != nil {
		return err
	}
	return d.next.Consume(ctx, rec)
}

// Publisher is the single local publisher (C2-13 §3): a bounded poller
// delivering committed outbox entries in per-aggregate sequence order to
// registered in-process consumers. Durability comes from the transaction,
// not the publisher; restart resumes unpolled rows; retries are bounded
// with backoff — no unbounded hot loops. No external broker (locked D-11);
// no exactly-once promise anywhere.
type Publisher struct {
	store    *Store
	interval time.Duration
	batch    int

	mu        sync.Mutex
	consumers []Consumer
	stop      chan struct{}
	stopped   sync.Once
	wg        sync.WaitGroup
}

// NewPublisher builds the bounded poller (Proposed interval 1 s, batch 256).
func NewPublisher(store *Store, interval time.Duration, batch int) *Publisher {
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 256
	}
	return &Publisher{store: store, interval: interval, batch: batch, stop: make(chan struct{})}
}

// Subscribe registers one consumer (duplicates by name are ignored).
func (p *Publisher) Subscribe(c Consumer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, existing := range p.consumers {
		if existing.Name() == c.Name() {
			return
		}
	}
	p.consumers = append(p.consumers, c)
}

// Start runs the poll loop until Stop.
func (p *Publisher) Start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), p.interval*5)
				_ = p.deliverPass(ctx)
				cancel()
			}
		}
	}()
}

// Stop ends the poll loop and waits for the in-flight pass.
func (p *Publisher) Stop() {
	p.stopped.Do(func() { close(p.stop) })
	p.wg.Wait()
}

// deliverPass runs one bounded delivery pass: fetch → deliver to every
// consumer in per-aggregate sequence order → mark published or schedule
// the bounded retry.
func (p *Publisher) deliverPass(ctx context.Context) error {
	recs, err := p.store.dueRows(ctx, p.batch)
	if err != nil {
		return err
	}
	events.SortBySequence(recs)
	for _, rec := range recs {
		p.mu.Lock()
		consumers := append([]Consumer(nil), p.consumers...)
		p.mu.Unlock()
		failed := false
		var firstErr error
		for _, c := range consumers {
			if err := c.Consume(ctx, rec); err != nil {
				failed = true
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		if failed {
			var attempts int
			_ = p.store.readers.QueryRowContext(ctx,
				`SELECT attempts FROM events_outbox WHERE event_id = ?`, rec.EventID).Scan(&attempts)
			_ = p.store.scheduleRetry(ctx, rec.EventID, attempts)
			_ = firstErr
			continue
		}
		if err := p.store.markPublished(ctx, rec.EventID); err != nil {
			return err
		}
	}
	return nil
}
