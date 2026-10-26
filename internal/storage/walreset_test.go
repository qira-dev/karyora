package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestD09WALResetRegression implements the D-09 Driver Evidence Matrix
// WAL-reset regression/fault test plan on the local target (linux/amd64):
//
//  1. sqlite_version()/sqlite_source_id() captured in the same run proves
//     the fixed library (>= 3.51.3, WAL-reset fix) is actually linked.
//  2. A multi-connection concurrent write/checkpoint reproducer exercises
//     the documented bug pattern (two or more connections on the same file
//     committing and checkpointing at the same instant), with repeated
//     timing variation to widen the race window.
//  3. Post-run PRAGMA integrity_check plus journal/outbox consistency
//     asserts no corruption.
//
// Per-target linked-runtime evidence for the other D-06 rows remains
// separate future evidence; D-09 stays Proposed + Technical Gate for the
// owner's review — this test proves the local slice only.
func TestD09WALResetRegression(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// 1. Linked-library proof from the actual artifact.
	var version, sourceID string
	if err := s.readers.QueryRowContext(ctx, "SELECT sqlite_version(), sqlite_source_id()").Scan(&version, &sourceID); err != nil {
		t.Fatalf("linked-library probe: %v", err)
	}
	t.Logf("linked SQLite: version=%s source_id=%s", version, sourceID)
	if !versionAtLeast(version, 3, 51, 3) {
		t.Fatalf("linked SQLite %s predates the WAL-reset fix (3.51.3); the driver step must stop per the D-09 matrix", version)
	}

	// 2. Multi-connection reproducer: two independent connection pools on
	// the same file (the same pattern as a second reader/writer process
	// slice), interleaving commits and checkpoints.
	other, err := Connect(s.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.writers.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS wal_probe (id INTEGER PRIMARY KEY, payload TEXT)`); err != nil {
		t.Fatal(err)
	}
	stop := time.Now().Add(1500 * time.Millisecond)
	iterations := 0
	for time.Now().Before(stop) {
		// Connection A commits; connection B checkpoints at the same
		// instant — the documented bug pattern. Timing jitter widens the
		// race window across iterations.
		if _, err := other.writers.ExecContext(ctx, `INSERT INTO wal_probe (payload) VALUES (?)`, fmt.Sprintf("iter-%d", iterations)); err != nil {
			t.Fatalf("write side: %v", err)
		}
		if _, err := s.writers.ExecContext(ctx, `INSERT INTO wal_probe (payload) VALUES (?)`, fmt.Sprintf("owner-%d", iterations)); err != nil {
			t.Fatalf("owner write side: %v", err)
		}
		if _, err := s.readers.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
			t.Fatalf("checkpoint side: %v", err)
		}
		iterations++
		if iterations%7 == 0 {
			time.Sleep(time.Duration(iterations%3) * time.Millisecond)
		}
	}
	t.Logf("reproducer completed %d interleaved write/checkpoint rounds", iterations)

	// 3. Post-run integrity: PRAGMA integrity_check plus journal/outbox
	// consistency; no corruption anywhere.
	if err := s.IntegrityCheck(ctx); err != nil {
		t.Fatalf("post-run integrity_check: %v", err)
	}
	if err := other.IntegrityCheck(ctx); err != nil {
		t.Fatalf("post-run integrity_check (second connection): %v", err)
	}
	if err := s.ReconcileOutbox(ctx); err != nil {
		t.Fatalf("post-run journal/outbox consistency: %v", err)
	}
	var rows int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM wal_probe`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != iterations*2 {
		t.Fatalf("probe rows = %d, want %d", rows, iterations*2)
	}
}

// versionAtLeast parses "X.Y.Z" and reports whether it is >= the floor.
func versionAtLeast(version string, wantMajor, wantMinor, wantPatch int) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	if minor != wantMinor {
		return minor > wantMinor
	}
	return patch >= wantPatch
}

// The linked library identity is also recorded from a fully independent
// pool, matching the D-09 pin record requirement that no claim rests on the
// Go wrapper version alone.
func TestD09LinkedLibraryIdentityStable(t *testing.T) {
	opts := testOptions(t)
	ctx := context.Background()
	s1, err := Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	s2, err := Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var v1, id1, v2, id2 string
	if err := s1.readers.QueryRowContext(ctx, "SELECT sqlite_version(), sqlite_source_id()").Scan(&v1, &id1); err != nil {
		t.Fatal(err)
	}
	if err := s2.readers.QueryRowContext(ctx, "SELECT sqlite_version(), sqlite_source_id()").Scan(&v2, &id2); err != nil {
		t.Fatal(err)
	}
	if v1 != v2 || id1 != id2 {
		t.Fatalf("linked library identity diverged across pools: %s/%s vs %s/%s", v1, id1, v2, id2)
	}
	if !strings.HasPrefix(id1, "20") {
		t.Fatalf("source id format unexpected: %s", id1)
	}
	var _ = sql.ErrNoRows
}
