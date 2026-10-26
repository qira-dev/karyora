package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// C2-11 §1/§2: fresh apply through the single authority.
func TestMigrationsFreshApply(t *testing.T) {
	s := openTestStore(t)
	v, err := s.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersionMax {
		t.Fatalf("schema version = %d, want %d", v, schemaVersionMax)
	}
	state, err := s.migrationState(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if state != migStateApplied {
		t.Fatalf("migration 1 state = %s, want applied", state)
	}
}

// C2-11 §2: a modified or missing checksum record is STATE_CORRUPT — the
// quarantine path, never silently treated as current.
func TestMigrationChecksumTamperDetected(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.writers.Exec(`UPDATE schema_migrations SET checksum = 'tampered' WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	err := s.migrate(context.Background(), "test")
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeStateCorrupt {
		t.Fatalf("tampered checksum err = %v, want STATE_CORRUPT", err)
	}
}

// C2-11 §2: a database newer than the binary refuses to start.
func TestNewerSchemaRefused(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.writers.Exec(`INSERT INTO schema_migrations (version, name, checksum, state, step, applied_at, core_version)
		VALUES (99, '099-future', 'x', 'applied', 0, 'now', 'newer')`); err != nil {
		t.Fatal(err)
	}
	err := s.migrate(context.Background(), "test")
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeConflictState {
		t.Fatalf("newer schema err = %v, want CONFLICT_STATE", err)
	}
	if !strings.Contains(se.Detail, "newer") {
		t.Fatalf("refusal must be actionable: %s", se.Detail)
	}
}

// C2-11 §3: an interrupted migration resumes (in_progress → re-applied).
func TestInterruptedMigrationResumes(t *testing.T) {
	opts := testOptions(t)
	ctx := context.Background()

	// Simulate a crash after the in_progress marker, before the DDL commit.
	if err := ensureAuthorityForTest(ctx, opts); err != nil {
		t.Fatal(err)
	}
	s1, err := Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.writers.Exec(`INSERT INTO schema_migrations (version, name, checksum, state, step, applied_at, core_version)
		VALUES (1, '0001-initial-core-schema', ?, 'in_progress', 0, 'now', 'test')`, checksumOf(migrations[0])); err != nil {
		t.Fatal(err)
	}
	// The DDL never committed: the journal tables do not exist.
	var hasJournal int
	if err := s1.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='events_journal'`).Scan(&hasJournal); err != nil {
		t.Fatal(err)
	}
	if hasJournal != 0 {
		t.Fatal("precondition: the interrupted migration must not have produced tables")
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// The next start resumes and completes the migration.
	s2, err := Connect(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.Migrate(ctx, "test"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	state, err := s2.migrationState(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if state != migStateApplied {
		t.Fatalf("state after resume = %s, want applied", state)
	}
}

func ensureAuthorityForTest(ctx context.Context, opts Options) error {
	s, err := Connect(opts)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.ensureMigrationAuthority(ctx)
}

// C2-11 §4: a destructive migration refuses without a verified recovery
// point (fail-closed; the upgrade does not proceed on hope).
func TestDestructiveMigrationRequiresRecoveryPoint(t *testing.T) {
	s := openTestStore(t)
	err := s.applyTransactional(context.Background(), Migration{
		Version:     2,
		Name:        "0002-destructive-test",
		SQL:         "CREATE TABLE destructive_probe (id INTEGER)",
		Destructive: true,
	}, "test")
	var se *Error
	if !errors.As(err, &se) || se.Code != errs.CodeRecoveryRequired {
		t.Fatalf("destructive without recovery point err = %v, want RECOVERY_REQUIRED", err)
	}
	// The migration must not have run.
	var hasTable int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='destructive_probe'`).Scan(&hasTable); err != nil {
		t.Fatal(err)
	}
	if hasTable != 0 {
		t.Fatal("a refused destructive migration must not execute its DDL")
	}
}

// C2-11: integrity_check is part of startup; corruption is STATE_CORRUPT.
func TestIntegrityCheck(t *testing.T) {
	s := openTestStore(t)
	if err := s.IntegrityCheck(context.Background()); err != nil {
		t.Fatalf("fresh database must pass integrity_check: %v", err)
	}
}

// The recovery generation persists and increments monotonically across
// restarts (C2-09 §2 step 5).
func TestGenerationPersists(t *testing.T) {
	opts := testOptions(t)
	ctx := context.Background()
	s1, err := Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	g1, err := s1.BumpGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	g2, err := s2.BumpGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if g2 != g1+1 {
		t.Fatalf("generation must increment across restarts: %d then %d", g1, g2)
	}
	if _, err := s2.Generation(ctx); err != nil {
		t.Fatal(err)
	}
}

// The D-12/D-33 exclusion holds: no Work tables exist in the schema.
func TestNoWorkTables(t *testing.T) {
	s := openTestStore(t)
	rows, err := s.readers.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.ToLower(name), "work") || strings.Contains(strings.ToLower(name), "workflow_run") {
			t.Fatalf("Work-dependent table %q must not exist (D-12/D-33 exclusion)", name)
		}
	}
}

var _ = time.Now
var _ = filepath.Join
