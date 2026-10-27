// Package storage is the SQLite + CAS adapter layer (dependency-map rule 4:
// database drivers are imported only here). It implements the C2-10
// connection/durability/writer-admission policy, the C2-11 migration
// policy, the C2-12 CAS store, and the C2-13 journal/outbox, and exposes
// the startup/drain ports the runtime machine consumes. Vendor errors are
// mapped inward to the stable C2-05 codes at this boundary; the mapping
// table is part of this adapter's contract.
package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Options carries the validated C2-10 policy values. The composition root
// builds them from the validated configuration; this package never reads
// configuration itself. Every numeric default in the configuration remains
// Proposed until recorded review.
type Options struct {
	Path                   string
	PoolMaxOpen            int
	WriterQueueDepth       int
	WriterDeadline         time.Duration
	BusyTimeout            time.Duration
	WALAutocheckpointPages int
	Synchronous            string // "NORMAL" | "FULL" (C2-10 §3 durability policy)
	// ReadBudget bounds every read transaction/statement (Proposed max 5 s,
	// C2-10 §4).
	ReadBudget time.Duration
	// MaxObjectBytes and MaxObjects are the CAS write bounds (C2-12 §5).
	MaxObjectBytes int64
	MaxObjects     int64
	// GCGracePeriod is the collection grace (Proposed 24 h, C2-12 §4).
	GCGracePeriod time.Duration
	// PublishInterval and PublishBatch are the outbox poller bounds
	// (Proposed 1 s / 256, C2-13 §3).
	PublishInterval time.Duration
	PublishBatch    int
	// Retention is the replay window (Proposed 30 days, C1-08).
	Retention time.Duration
	// NetworkFS checks whether path sits on a network filesystem; the
	// composition wires the platform adapter. A live WAL on a network
	// mount is refused at open (C2-10 §6, locked D-08).
	NetworkFS func(path string) (bool, string, error)
}

// Synchronous pragmas map to their numeric SQLite values.
const (
	synchronousNormal = 1
	synchronousFull   = 2
)

func synchronousValue(mode string) (int, bool) {
	switch strings.ToUpper(mode) {
	case "NORMAL":
		return synchronousNormal, true
	case "FULL":
		return synchronousFull, true
	default:
		return 0, false
	}
}

// Store is the durable local store: SQLite (policies per C2-10) plus the
// CAS (C2-12) and the journal/outbox (C2-13). It implements the runtime
// startup/drain ports. Exactly one Store exists per data directory — the
// runner's kernel lock guarantees it.
type Store struct {
	opts Options
	// readers is the bounded reader pool; the writer path goes through the
	// admission gate to keep pool and writer admission separate and
	// measurable (C2-10 §2).
	readers *sql.DB
	// writers is a dedicated single-connection handle behind the gate.
	writers      *sql.DB
	gate         *writerGate
	cas          *CAS
	pub          *Publisher
	m            *Metrics
	instanceUUID string
}

// Metrics carries the C2-05 §3 storage/WAL/queue signals exported to health.
// Every counter is bounded-safe for the report model.
type Metrics struct {
	PoolInUse          int64  `json:"poolInUse"`
	PoolIdle           int64  `json:"poolIdle"`
	QueueDepth         int64  `json:"writerQueueDepth"`
	QueueRejections    int64  `json:"writerQueueRejections"`
	WaitMS             int64  `json:"writerWaitMs"`
	WALSizeBytes       uint64 `json:"walSizeBytes"`
	CheckpointStarving bool   `json:"walStarving"`
}

func newMetrics() *Metrics { return &Metrics{} }

// Metrics reports a snapshot of the storage signals.
func (s *Store) Metrics() Metrics {
	return Metrics{
		PoolInUse:          int64(s.readers.Stats().InUse),
		PoolIdle:           int64(s.readers.Stats().Idle),
		QueueDepth:         s.gate.queueDepth(),
		QueueRejections:    s.gate.rejections(),
		WaitMS:             s.gate.lastWaitMS(),
		WALSizeBytes:       s.walSize(),
		CheckpointStarving: s.starving(),
	}
}

// Connect opens the pooled connections with the verified option matrix —
// C2-09 startup step 3 happens through Migrate below, never before the
// runner owns the data directory.
func Connect(opts Options) (*Store, error) {
	return openConnections(opts)
}

// Open applies the full startup storage sequence for tests and direct use:
// open with the verified option matrix, run the integrity check, and apply
// migrations. The runtime ports compose these steps in the strict C2-09
// order instead.
func Open(ctx context.Context, opts Options) (*Store, error) {
	s, err := Connect(opts)
	if err != nil {
		return nil, err
	}
	if err := s.Migrate(ctx, "test"); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// verifiedConnector wraps the pinned go-sqlite3 driver with the C2-10 §1
// connect hook: every new pooled connection applies the required settings
// and reads them back; any mismatch fails the connection, so pool churn can
// never create a weaker connection.
type verifiedConnector struct {
	opts Options
}

func (c verifiedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	d := &sqlite3.SQLiteDriver{}
	conn, err := d.Open(c.opts.Path)
	if err != nil {
		return nil, err
	}
	sc, ok := conn.(*sqlite3.SQLiteConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("storage: unexpected driver connection type")
	}
	if err := applyAndVerify(ctx, sc, c.opts); err != nil {
		conn.Close()
		return nil, fmt.Errorf("storage: connection verification failed: %w", err)
	}
	return conn, nil
}

func (c verifiedConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

// applyAndVerify sets and reads back the full option matrix (C2-10 §1).
func applyAndVerify(ctx context.Context, conn *sqlite3.SQLiteConn, opts Options) error {
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL", nil); err != nil {
		return fmt.Errorf("journal_mode: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON", nil); err != nil {
		return fmt.Errorf("foreign_keys: %w", err)
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", opts.BusyTimeout.Milliseconds()), nil); err != nil {
		return fmt.Errorf("busy_timeout: %w", err)
	}
	syncVal, ok := synchronousValue(opts.Synchronous)
	if !ok {
		return fmt.Errorf("unsupported synchronous mode %q", opts.Synchronous)
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA synchronous=%d", syncVal), nil); err != nil {
		return fmt.Errorf("synchronous: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA trusted_schema=OFF", nil); err != nil {
		return fmt.Errorf("trusted_schema: %w", err)
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA wal_autocheckpoint=%d", opts.WALAutocheckpointPages), nil); err != nil {
		return fmt.Errorf("wal_autocheckpoint: %w", err)
	}
	return verifyMatrix(ctx, conn, opts, syncVal)
}

// verifyMatrix reads back every required setting; a mismatch fails closed.
func verifyMatrix(ctx context.Context, conn *sqlite3.SQLiteConn, opts Options, syncVal int) error {
	check := func(query, want, name string) error {
		rows, err := conn.QueryContext(ctx, query, nil)
		if err != nil {
			return fmt.Errorf("%s readback: %w", name, err)
		}
		defer rows.Close()
		dest := make([]driver.Value, 1)
		if err := rows.Next(dest); err != nil {
			return fmt.Errorf("%s readback: %w", name, err)
		}
		got := dest[0]
		gotStr := ""
		switch v := got.(type) {
		case string:
			gotStr = v
		case int64:
			gotStr = strconv.FormatInt(v, 10)
		}
		if gotStr != want {
			return fmt.Errorf("%s = %q, want %q", name, gotStr, want)
		}
		return nil
	}
	if err := check("PRAGMA journal_mode", "wal", "journal_mode"); err != nil {
		return err
	}
	if err := check("PRAGMA foreign_keys", "1", "foreign_keys"); err != nil {
		return err
	}
	if err := check("PRAGMA busy_timeout", strconv.FormatInt(opts.BusyTimeout.Milliseconds(), 10), "busy_timeout"); err != nil {
		return err
	}
	if err := check("PRAGMA synchronous", strconv.Itoa(syncVal), "synchronous"); err != nil {
		return err
	}
	if err := check("PRAGMA trusted_schema", "0", "trusted_schema"); err != nil {
		return err
	}
	return check("PRAGMA wal_autocheckpoint", strconv.Itoa(opts.WALAutocheckpointPages), "wal_autocheckpoint")
}

func openConnections(opts Options) (*Store, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("storage: path is required")
	}
	if opts.NetworkFS != nil {
		net, kind, err := opts.NetworkFS(opts.Path)
		if err != nil {
			return nil, mapOpenError(err)
		}
		if net {
			// Locked D-08: a live WAL/SHM on a network filesystem is
			// refused with a clear, stable error naming the reason.
			return nil, fmt.Errorf("storage: %s lives on network filesystem %q; WAL is local-only (D-08)", opts.Path, kind)
		}
	}
	s := &Store{opts: opts, m: newMetrics()}
	s.gate = newWriterGate(opts.WriterQueueDepth, opts.WriterDeadline)
	readers := sql.OpenDB(verifiedConnector{opts: opts})
	readers.SetMaxOpenConns(opts.PoolMaxOpen)
	writers := sql.OpenDB(verifiedConnector{opts: opts})
	writers.SetMaxOpenConns(1)
	s.readers = readers
	s.writers = writers
	uuid, err := newInstanceUUID()
	if err != nil {
		s.Close()
		return nil, err
	}
	s.instanceUUID = uuid
	s.cas = NewCAS(casRoot(opts.Path), opts.MaxObjectBytes, opts.MaxObjects, opts.GCGracePeriod)
	// Reachability roots are constructor wiring, not a startup step: the
	// GC barrier (backup pins, journal payload references, durable
	// references) must hold for every Store instance from the first Collect,
	// independent of whether Reconcile has run (C2-12 §4, C2-16 §2).
	s.registerReferenceRoots()
	s.pub = NewPublisher(s, opts.PublishInterval, opts.PublishBatch)
	return s, nil
}

func casRoot(dbPath string) string {
	// dataDir/cas — the DB path is <data>/karyora.db, so the CAS root is a
	// sibling directory under the same data class (C2-06 §1).
	return strings.TrimSuffix(dbPath, "/") + ".cas"
}

func newInstanceUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("storage: instance uuid: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Close releases the pools and stops the publisher.
func (s *Store) Close() error {
	var firstErr error
	if s.pub != nil {
		s.pub.Stop()
	}
	if s.writers != nil {
		if err := s.writers.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.readers != nil {
		if err := s.readers.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// mapVendorError implements the adapter's error-mapping table (C2-05):
// vendor types never cross the boundary; the stable codes do.
func mapVendorError(err error) error {
	if err == nil {
		return nil
	}
	var serr sqlite3.Error
	if errors.As(err, &serr) {
		switch serr.Code {
		case sqlite3.ErrBusy, sqlite3.ErrLocked:
			return errStoragef(errs.CodeStorageUnavailable, "database busy")
		case sqlite3.ErrFull, sqlite3.ErrIoErr, sqlite3.ErrNomem:
			return errStoragef(errs.CodeStorageUnavailable, "storage io/full")
		case sqlite3.ErrCorrupt, sqlite3.ErrNotADB:
			return errStoragef(errs.CodeStateCorrupt, "database corrupt")
		case sqlite3.ErrConstraint:
			return errStoragef(errs.CodeConflictState, "constraint")
		default:
			return errStoragef(errs.CodeStorageUnavailable, "sqlite error")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errStoragef(errs.CodeRateLimited, "deadline exceeded")
	}
	return errStoragef(errs.CodeStorageUnavailable, "storage failure")
}

func mapOpenError(err error) error { return mapVendorError(err) }

// walSize reads the WAL size for the health signal (C2-10 §5) from the
// -wal sibling file. A read failure reports 0 — the health layer pairs the
// value with the storage signal, never a silent claim.
func (s *Store) walSize() uint64 {
	st, err := os.Stat(s.opts.Path + "-wal")
	if err != nil {
		return 0
	}
	return uint64(st.Size())
}

// starving reports whether a checkpoint is starved by a long reader
// (C2-10 §5). Phase 2 computes it from the WAL size relative to the
// autocheckpoint threshold; the maintenance response (read-drain) lands
// with the Phase 2 benchmark evidence.
func (s *Store) starving() bool {
	const starveThresholdBytes = 64 * 1024 * 1024 // Proposed bound: 64 MiB WAL
	return s.walSize() > starveThresholdBytes
}
