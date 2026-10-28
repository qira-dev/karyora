package remotestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// runStamp makes every fixture id unique per run, so the suite re-runs
// against a persistent database without colliding with a previous run's
// lease-carrying rows (CI's postgres service is fresh anyway).
var runStamp = time.Now().UnixNano()

func uid(name string) string { return fmt.Sprintf("%s-%d", name, runStamp) }

// Conformance fixtures for spec §2 (C4-34), executed against a LIVE
// PostgreSQL server: the competing-claim race, stale-epoch commit
// refusals, authority-clock lease expiry, transaction rollback
// cleanliness, and duplicate-event apply-once. The DSN comes from
// REMOTESTORE_TEST_DSN; when unset the DB fixtures SKIP (recorded as Not
// Run for that run, never a pass).

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("REMOTESTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("REMOTESTORE_TEST_DSN not set — PostgreSQL fixtures Not Run in this run")
	}
	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.VerifyAhead(context.Background()); err != nil {
		t.Fatalf("verify ahead: %v", err)
	}
	return store
}

func seedWorkspace(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, org_id) VALUES ($1, $1) ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
}

func seedJob(t *testing.T, s *Store, ws, id, submitter, state string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO jobs (id, workspace_id, state, submitter_id, idempotency_key, request_fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, job_version = 1`,
		id, ws, state, submitter, id+"-idem", id+"-fingerprint"); err != nil {
		t.Fatalf("seed job: %v", err)
	}
}

func TestMigrationsApplyIdempotentlyAndVerify(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	// A second run is a no-op; the checksum gate holds.
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	if err := s.VerifyAhead(ctx); err != nil {
		t.Fatalf("verify ahead: %v", err)
	}
	// A drifted applied checksum refuses (C2-11 integrity anchor).
	if _, err := s.pool.Exec(ctx,
		`UPDATE schema_migrations SET checksum = 'sha256-tampered' WHERE version = 1`); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := s.Migrate(ctx); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("drifted checksum must refuse, got %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE schema_migrations SET checksum = $1 WHERE version = 1`, migrationsV1[0].checksum()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	// A database claiming a newer version refuses (forward-fix only).
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, state) VALUES (99, 'future', 'sha256-future', 'applied')
		 ON CONFLICT (version) DO NOTHING`); err != nil {
		t.Fatalf("seed future version: %v", err)
	}
	if err := s.VerifyAhead(ctx); !errors.Is(err, ErrSchemaAhead) {
		t.Fatalf("newer schema must refuse, got %v", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = 99`); err != nil {
		t.Fatalf("cleanup future version: %v", err)
	}
	if err := s.VerifyAhead(ctx); err != nil {
		t.Fatalf("verify after cleanup: %v", err)
	}
}

func TestCompetingClaimRaceSingleWinner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-race"))
	const jobs = 8
	for i := 0; i < jobs; i++ {
		seedJob(t, s, uid("ws-race"), uid(fmt.Sprintf("j-race-%d", i)), "alice", "queued")
	}

	var mu sync.Mutex
	claimed := map[string]string{} // job id → worker id
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			worker := fmt.Sprintf("worker-%d", n)
			for {
				c, err := s.ClaimJob(ctx, worker, []string{uid("ws-race")})
				if errors.Is(err, ErrNotClaimed) {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				mu.Lock()
				if prev, dup := claimed[c.JobID]; dup {
					t.Errorf("job %s claimed twice (%s and %s)", c.JobID, prev, worker)
				}
				claimed[c.JobID] = worker
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	if len(claimed) != jobs {
		t.Fatalf("claimed %d jobs, want exactly %d (each exactly once)", len(claimed), jobs)
	}
}

func TestStaleFencingCommitRefused(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-fence"))
	seedJob(t, s, uid("ws-fence"), uid("j-fence"), "carol", "queued")

	c, err := s.ClaimJob(ctx, "worker-a", []string{uid("ws-fence")})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	commit := func(epoch, version int64, worker, lease, receipt string) error {
		return s.CommitAttempt(ctx, AttemptCommit{
			JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
			WorkerID: worker, LeaseID: lease, FencingEpoch: epoch,
			ExpectedJobVersion: version, JobState: "succeeded",
			ReceiptID: receipt, ResultDigest: "sha256-result", Outcome: "ok",
		})
	}
	// A stale epoch (lower than the claim) never commits.
	if err := commit(c.FencingEpoch-1, 2, c.WorkerID, c.LeaseID, uid("rc-stale")); !errors.Is(err, ErrStaleFencing) {
		t.Fatalf("stale epoch must refuse, got %v", err)
	}
	// A forged (higher) epoch refuses — the lease carries the real one.
	if err := commit(c.FencingEpoch+1, 2, c.WorkerID, c.LeaseID, uid("rc-forged")); !errors.Is(err, ErrStaleFencing) {
		t.Fatalf("forged epoch must refuse, got %v", err)
	}
	// A different worker with the same lease refuses.
	if err := commit(c.FencingEpoch, 2, "worker-b", c.LeaseID, uid("rc-foreign")); !errors.Is(err, ErrStaleFencing) {
		t.Fatalf("foreign worker must refuse, got %v", err)
	}
	// A stale job version (moved underneath the caller) refuses.
	if err := commit(c.FencingEpoch, 1, c.WorkerID, c.LeaseID, uid("rc-stalever")); !errors.Is(err, ErrStaleFencing) {
		t.Fatalf("stale job version must refuse, got %v", err)
	}
	// The genuine tuple commits exactly once.
	if err := commit(c.FencingEpoch, 2, c.WorkerID, c.LeaseID, uid("rc-fence-genuine")); err != nil {
		t.Fatalf("genuine commit: %v", err)
	}
	if err := commit(c.FencingEpoch, 3, c.WorkerID, c.LeaseID, uid("rc-fence-twice")); !errors.Is(err, ErrFencedRefused) {
		t.Fatalf("second terminal commit must refuse, got %v", err)
	}
}

func TestExpiredLeaseNeverCommits(t *testing.T) {
	s := testStore(t)
	s.LeaseTTL = 5 * time.Millisecond // Proposed interval; the SERVER clock decides
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-expiry"))
	seedJob(t, s, uid("ws-expiry"), uid("j-expiry"), "carol", "queued")

	c, err := s.ClaimJob(ctx, "worker-a", []string{uid("ws-expiry")})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // the SERVER clock expires the lease
	if _, err := s.RenewLease(ctx, c.JobID, c.AttemptID, c.WorkerID, c.FencingEpoch); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired lease renewal must refuse, got %v", err)
	}
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-expired"), ResultDigest: "sha256-expired", Outcome: "ok",
	}); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired lease commit must refuse, got %v", err)
	}
	// Recovery is a FRESH claim with a bumped epoch; the stale worker's
	// tuple can never follow it. The recovering worker runs with the
	// default lease interval (the 5 ms TTL above exists only to let the
	// SERVER clock expire the first claim).
	recovered := &Store{pool: s.pool, LeaseTTL: DefaultLeaseTTL}
	seedJob(t, s, uid("ws-expiry"), uid("j-expiry"), "carol", "queued")
	fresh, err := recovered.ClaimJob(ctx, "worker-b", []string{uid("ws-expiry")})
	if err != nil {
		t.Fatalf("fresh claim: %v", err)
	}
	if fresh.FencingEpoch != c.FencingEpoch+1 {
		t.Fatalf("fresh claim epoch = %d, want the bumped %d", fresh.FencingEpoch, c.FencingEpoch+1)
	}
	if err := recovered.CommitAttempt(ctx, AttemptCommit{
		JobID: fresh.JobID, WorkspaceID: fresh.WorkspaceID, AttemptID: fresh.AttemptID,
		WorkerID: fresh.WorkerID, LeaseID: fresh.LeaseID, FencingEpoch: fresh.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-expiry-fresh"), ResultDigest: "sha256-fresh", Outcome: "ok",
	}); err != nil {
		t.Fatalf("fresh-epoch commit: %v", err)
	}
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-expired-2"), ResultDigest: "sha256-expired-2", Outcome: "ok",
	}); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("stale worker must refuse after recovery (its lease expired by the authority clock), got %v", err)
	}
}

func TestRollbackLeavesNoPartialState(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-rollback"))
	seedJob(t, s, uid("ws-rollback"), uid("j-rollback"), "carol", "queued")

	c, err := s.ClaimJob(ctx, "worker-a", []string{uid("ws-rollback")})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Force a mid-transaction failure: the receipt primary key already
	// exists, so the commit transaction aborts after its writes.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO job_receipts (id, workspace_id, job_id, attempt_id, outcome, result_digest)
		VALUES ($4, $1, $2, $3, 'seed', 'sha256-seed')`,
		c.WorkspaceID, c.JobID, c.AttemptID, uid("rc-dup")); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-dup"), ResultDigest: "sha256-rollback", Outcome: "ok",
	}); err == nil {
		t.Fatal("a conflicting receipt must abort the commit transaction")
	}
	// Nothing partial survived: the job is still running and no completion
	// event leaked into the journal.
	var state string
	var completions int
	if err := s.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1`, c.JobID).Scan(&state); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if state != "running" {
		t.Fatalf("job state = %q, want running (transaction rolled back cleanly)", state)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM event_journal
		WHERE aggregate_id = $1 AND type <> 'job.claimed'`, c.JobID,
	).Scan(&completions); err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if completions != 0 {
		t.Fatalf("completion event leaked past a rolled-back transaction (%d rows)", completions)
	}
	// The failed transaction consumed nothing durable: the genuine commit
	// still lands with the same expected version.
	if err := s.CommitAttempt(ctx, AttemptCommit{
		JobID: c.JobID, WorkspaceID: c.WorkspaceID, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 2, JobState: "succeeded",
		ReceiptID: uid("rc-rollback-genuine"), ResultDigest: "sha256-genuine", Outcome: "ok",
	}); err != nil {
		t.Fatalf("genuine commit after rollback: %v", err)
	}
}

func TestOutboxApplyOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-outbox"))
	seedJob(t, s, uid("ws-outbox"), uid("j-outbox"), "carol", "queued")

	c, err := s.ClaimJob(ctx, "worker-a", []string{uid("ws-outbox")})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	var journal, outbox int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM event_journal WHERE workspace_id = $1`, uid("ws-outbox")).Scan(&journal); err != nil {
		t.Fatalf("journal count: %v", err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE workspace_id = $1`, uid("ws-outbox")).Scan(&outbox); err != nil {
		t.Fatalf("outbox count: %v", err)
	}
	if journal != 1 || outbox != 1 {
		t.Fatalf("journal=%d outbox=%d, want 1/1 after one claim", journal, outbox)
	}
	// A replayed event id is a no-op (consumers apply once).
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := appendEvent(ctx, tx, uid("ws-outbox"),
		fmt.Sprintf("job.claimed:%s:%d", c.JobID, c.FencingEpoch),
		"job", c.JobID, "job.claimed", map[string]any{"replay": true}); err != nil {
		t.Fatalf("replay append: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit replay: %v", err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM event_journal WHERE workspace_id = $1`, uid("ws-outbox")).Scan(&journal); err != nil {
		t.Fatalf("journal count after replay: %v", err)
	}
	if journal != 1 {
		t.Fatalf("replay duplicated the event (%d journal rows)", journal)
	}
}

func TestNotQueuedJobNeverClaims(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedWorkspace(t, s, uid("ws-states"))
	seedJob(t, s, uid("ws-states"), uid("j-running"), "carol", "running")
	seedJob(t, s, uid("ws-states"), uid("j-done"), "carol", "succeeded")
	if _, err := s.ClaimJob(ctx, "worker-a", []string{uid("ws-states")}); !errors.Is(err, ErrNotClaimed) {
		t.Fatalf("non-queued jobs must not claim, got %v", err)
	}
}

func TestRetryTransactionOnlyRetriesConflictClasses(t *testing.T) {
	ctx := context.Background()
	calls := 0
	err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return &pgconn.PgError{Code: "40001"} // serialization failure
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("conflict-class error must retry once and settle: calls=%d err=%v", calls, err)
	}
	other := errors.New("not a conflict")
	calls = 0
	if err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
		calls++
		return other
	}); !errors.Is(err, other) || calls != 1 {
		t.Fatalf("non-conflict errors must return immediately: calls=%d err=%v", calls, err)
	}
}
