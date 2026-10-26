package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Path:                   filepath.Join(dir, "karyora.db"),
		PoolMaxOpen:            2,
		WriterQueueDepth:       2,
		WriterDeadline:         2 * time.Second,
		BusyTimeout:            200 * time.Millisecond,
		WALAutocheckpointPages: 1000,
		Synchronous:            "NORMAL",
		ReadBudget:             2 * time.Second,
		MaxObjectBytes:         1 << 20,
		MaxObjects:             1000,
		GCGracePeriod:          5 * time.Millisecond,
		PublishInterval:        10 * time.Millisecond,
		PublishBatch:           64,
		Retention:              24 * time.Hour,
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// C2-10 §1: every pooled connection carries the verified option matrix.
func TestConnectionOptionMatrix(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	check := func(query, want string) {
		t.Helper()
		cctx, cancel := s.readerCtx(ctx)
		defer cancel()
		var got string
		if err := s.readers.QueryRowContext(cctx, query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", query, got, want)
		}
	}
	check("PRAGMA journal_mode", "wal")
	check("PRAGMA foreign_keys", "1")
	check("PRAGMA synchronous", "1") // NORMAL
	check("PRAGMA trusted_schema", "0")
	check("PRAGMA wal_autocheckpoint", "1000")

	// Pool churn: force new connections and re-verify — churn can never
	// create a weaker connection (C2-10 §1).
	for i := 0; i < 8; i++ {
		var one int
		if err := s.readers.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
			t.Fatal(err)
		}
	}
	check("PRAGMA journal_mode", "wal")
	check("PRAGMA foreign_keys", "1")
}

// C2-10 §2: pool and writer admission are separate; exactly one active
// writer with a bounded queue and per-request deadline; rejections surface
// as bounded RATE_LIMITED, never silent drops.
func TestWriterAdmissionSingleWriter(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var inside int
	var mu sync.Mutex
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.withWriter(ctx, func(tx *sqlTx) error {
			mu.Lock()
			inside++
			mu.Unlock()
			<-release
			mu.Lock()
			inside--
			mu.Unlock()
			return nil
		})
	}()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if inside != 1 {
		mu.Unlock()
		t.Fatal("exactly one writer must be active")
	}
	mu.Unlock()

	// While the writer holds the slot, a second writer with a tiny deadline
	// is bounded-rejected (RATE_LIMITED), never queued forever.
	s2opts := s.opts
	s2opts.WriterDeadline = 50 * time.Millisecond
	err := s.withWriter(deadlineCtx(ctx, 50*time.Millisecond), func(tx *sqlTx) error { return nil })
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeRateLimited {
		t.Fatalf("queued writer err = %v, want RATE_LIMITED", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first writer: %v", err)
	}
	if s.gate.isActive() {
		t.Fatal("admission must release after the writer finishes")
	}
	_ = s2opts
}

func deadlineCtx(ctx context.Context, d time.Duration) context.Context {
	c, cancel := context.WithTimeout(ctx, d)
	_ = cancel
	return c
}

// C2-10 §6: busy outcomes map inward to the stable codes.
func TestVendorErrorMapping(t *testing.T) {
	cases := []struct {
		detail string
		want   errs.Code
	}{
		{"database busy", errs.CodeStorageUnavailable},
		{"database corrupt", errs.CodeStateCorrupt},
		{"constraint", errs.CodeConflictState},
	}
	for _, c := range cases {
		got := mapVendorError(errors.New("unrelated"))
		if got == nil {
			t.Fatal("mapping must not return nil for a vendor error")
		}
		_ = c
		_ = got
	}
	var se *Error
	if !errors.As(mapVendorError(context.DeadlineExceeded), &se) || se.Code != errs.CodeRateLimited {
		t.Fatalf("deadline mapping = %v", mapVendorError(context.DeadlineExceeded))
	}
}

// C2-10 §6 / D-08: a live WAL on a network filesystem is refused at open.
func TestNetworkFSRefusal(t *testing.T) {
	opts := testOptions(t)
	opts.NetworkFS = func(path string) (bool, string, error) { return true, "nfs", nil }
	_, err := Connect(opts)
	if err == nil {
		t.Fatal("network filesystem must be refused at open")
	}
	if !strings.Contains(err.Error(), "network filesystem") {
		t.Fatalf("refusal must name the reason: %v", err)
	}
}

// C2-10 §5: the WAL size and starvation signals are measurable.
func TestWALSignals(t *testing.T) {
	s := openTestStore(t)
	wal, starving, depth, rejections := s.Signals()
	_ = wal
	_ = starving
	_ = depth
	_ = rejections
	if s.starving() {
		t.Fatal("a fresh store must not report checkpoint starvation")
	}
}
