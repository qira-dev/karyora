package remotestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Typed failures of the remote authority (stable, inspectable).
var (
	// ErrNotClaimed: the claim found no eligible queued Job for this
	// worker's authorized workspaces.
	ErrNotClaimed = errors.New("remotestore: no eligible job to claim")
	// ErrStaleFencing: the commit/renew carried a missing, mismatched or
	// superseded fencing tuple — a stale worker can never commit.
	ErrStaleFencing = errors.New("remotestore: stale fencing tuple")
	// ErrLeaseExpired: the SERVER clock (read inside the transaction)
	// shows the lease no longer valid.
	ErrLeaseExpired = errors.New("remotestore: lease expired by the authority clock")
	// ErrFencedRefused: the job is not in a state that accepts this
	// authoritative commit.
	ErrFencedRefused = errors.New("remotestore: authoritative commit refused")
	// ErrSchemaAhead / ErrChecksumMismatch: C2-11 migration discipline.
	ErrSchemaAhead      = errors.New("remotestore: database schema is newer than this build")
	ErrChecksumMismatch = errors.New("remotestore: applied migration checksum drifted")
)

// Store is the PostgreSQL remote authority. Open a pool per process; the
// zero value is unusable.
type Store struct {
	pool *pgxpool.Pool
	// LeaseTTL is the Proposed claim-lease duration. The expiry is ALWAYS
	// computed by the SERVER clock inside the claim/renew transaction —
	// this value only parameterizes the interval (spec §2 authority-clock
	// rule). Default DefaultLeaseTTL; the numeric value stays Proposed.
	LeaseTTL time.Duration
	// Blob is the S3-compatible shared object store (C4-36, spec §4).
	// Attached when the deployment configures one; the object lifecycle
	// primitives refuse with ErrBlobStoreRequired otherwise. The DB
	// reference rows remain the ONLY authority that makes an object
	// readable — the blob store holds bytes, never authority.
	Blob *BlobStore
	// GCGrace is the Proposed orphan-grace interval (DefaultGCGrace); the
	// SERVER clock inside the sweep transaction decides what is past
	// grace, exactly like the lease expiry.
	GCGrace time.Duration
}

// DefaultLeaseTTL is the Proposed lease interval (no owner-accepted
// numeric exists; D-28 owns performance targets).
const DefaultLeaseTTL = 30 * time.Second

// Open connects the pool. The DSN is operator configuration (never a
// credential store — secrets stay in the accepted mechanisms).
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("remotestore: connect: %w", err)
	}
	return &Store{pool: pool, LeaseTTL: DefaultLeaseTTL, GCGrace: DefaultGCGrace}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// ClaimedJob is one successful atomic claim: the full authority tuple the
// worker must carry on every subsequent commit.
type ClaimedJob struct {
	JobID        string
	WorkspaceID  string
	AttemptID    string
	AttemptNo    int64
	WorkerID     string
	LeaseID      string
	FencingEpoch int64
	// LeaseExpiresAt is the SERVER-side expiry read inside the claim
	// transaction; the worker uses it for scheduling only, never as the
	// authority.
	LeaseExpiresAt time.Time
}

// ClaimJob claims ONE queued job for the worker, atomically (spec §2):
// the presenting credential is verified INSIDE the claim transaction
// (active identity + current credential version + every requested
// workspace bound — Remote Operations §1; a revoked identity, a stale
// credential or an unbound scope refuses the whole claim) + SELECT … FOR
// UPDATE SKIP LOCKED over the worker's bound workspaces + precondition
// checks (still queued, no active attempt) + monotonic fencing-epoch bump
// + attempt row + lease row (server-clock expiry computed inside this
// transaction) + job → running + journal + outbox, all in ONE
// transaction. Two concurrent workers can never claim the same job; the
// loser sees no rows.
func (s *Store) ClaimJob(ctx context.Context, workerID, credential string, workspaces []string) (ClaimedJob, error) {
	if workerID == "" || credential == "" || len(workspaces) == 0 {
		return ClaimedJob{}, errors.New("remotestore: claim needs a worker id, a credential, and authorized workspaces")
	}
	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	var claimed ClaimedJob
	err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		// Service-identity boundary (Remote Operations §1): revoked
		// credentials are refused at claim, immediately, inside this
		// transaction — before any job state is touched.
		if err := verifyServiceIdentityCredential(ctx, tx, workerID, credential, workspaces); err != nil {
			return err
		}

		var jobID, workspaceID string
		var jobVersion int64
		err = tx.QueryRow(ctx, `
			SELECT id, workspace_id, job_version FROM jobs
			WHERE state = 'queued' AND workspace_id = ANY($1)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1`, workspaces,
		).Scan(&jobID, &workspaceID, &jobVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotClaimed
		}
		if err != nil {
			return err
		}

		// Precondition: no LIVE attempt may exist. Liveness is decided by
		// the SERVER clock through the attempt's lease (spec §3): an
		// expired lease is dead authority — its attempt never revives, and
		// a fresh claim with a bumped epoch takes the job over.
		var live int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM attempts a
			JOIN leases l ON l.attempt_id = a.id
			WHERE a.job_id = $1 AND a.state IN ('claimed','running')
			  AND l.expires_at > clock_timestamp()`, jobID,
		).Scan(&live); err != nil {
			return err
		}
		if live != 0 {
			return fmt.Errorf("remotestore: job %s has a live attempt: %w", jobID, ErrFencedRefused)
		}

		// Monotonic fencing epoch, bumped inside the same transaction.
		var epoch int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO job_fencing (job_id, fencing_epoch) VALUES ($1, 1)
			ON CONFLICT (job_id) DO UPDATE SET fencing_epoch = job_fencing.fencing_epoch + 1
			RETURNING fencing_epoch`, jobID,
		).Scan(&epoch); err != nil {
			return err
		}

		var attemptNo int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(max(attempt_no), 0) + 1 FROM attempts WHERE job_id = $1`, jobID,
		).Scan(&attemptNo); err != nil {
			return err
		}

		// Server-side identity fields: the lease expiry is computed by the
		// SERVER clock inside this transaction (spec §2 authority-clock
		// rule) — never by the worker host.
		attemptID := fmt.Sprintf("%s-attempt-%d", jobID, attemptNo)
		leaseID := fmt.Sprintf("%s-lease-%d", jobID, epoch)
		if _, err := tx.Exec(ctx, `
			INSERT INTO attempts (id, job_id, workspace_id, worker_id, attempt_no, state)
			VALUES ($1, $2, $3, $4, $5, 'claimed')`,
			attemptID, jobID, workspaceID, workerID, attemptNo,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO leases (id, job_id, attempt_id, worker_id, fencing_epoch, expires_at)
			VALUES ($1, $2, $3, $4, $5, clock_timestamp() + make_interval(secs => $6))`,
			leaseID, jobID, attemptID, workerID, epoch, ttl.Seconds(),
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET state = 'running', job_version = job_version + 1, updated_at = now()
			WHERE id = $1`, jobID,
		); err != nil {
			return err
		}

		claimed = ClaimedJob{
			JobID: jobID, WorkspaceID: workspaceID,
			AttemptID: attemptID, AttemptNo: attemptNo, WorkerID: workerID,
			LeaseID: leaseID, FencingEpoch: epoch,
		}
		if err := appendEvent(ctx, tx, workspaceID, fmt.Sprintf("job.claimed:%s:%d", jobID, epoch),
			"job", jobID, "job.claimed", map[string]any{
				"job_id": jobID, "attempt_id": attemptID, "worker_id": workerID, "fencing_epoch": epoch,
			}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return ClaimedJob{}, err
	}
	// Read the durable expiry for the worker's scheduler (a second query —
	// the authority for validity stays the in-transaction server clock).
	err = s.pool.QueryRow(ctx,
		`SELECT expires_at FROM leases WHERE id = $1`, claimed.LeaseID,
	).Scan(&claimed.LeaseExpiresAt)
	if err != nil {
		return ClaimedJob{}, fmt.Errorf("remotestore: read lease expiry: %w", err)
	}
	return claimed, nil
}

// RenewLease extends a live lease by the store TTL. Validity is decided by
// the SERVER clock inside the transaction; a stale epoch refuses before
// anything else, an expired lease refuses renewal (a fresh claim with a
// bumped epoch is the only recovery, spec §3), and a worker whose service
// identity is unknown or revoked refuses (Remote Operations §1 — the lease
// expires normally into reconciliation instead of being extended by a
// revoked worker).
func (s *Store) RenewLease(ctx context.Context, jobID, attemptID, workerID string, fencingEpoch int64) (time.Time, error) {
	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	var expires time.Time
	err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		// Service-identity boundary (Remote Operations §1): a revoked or
		// unknown worker identity never extends a lease.
		if err := requireActiveServiceIdentity(ctx, tx, workerID); err != nil {
			return err
		}

		var rowEpoch int64
		var rowExpires time.Time
		err = tx.QueryRow(ctx, `
			SELECT fencing_epoch, expires_at FROM leases
			WHERE attempt_id = $1 AND worker_id = $2 AND job_id = $3
			FOR UPDATE`, attemptID, workerID, jobID,
		).Scan(&rowEpoch, &rowExpires)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStaleFencing
		}
		if err != nil {
			return err
		}
		if rowEpoch != fencingEpoch {
			return ErrStaleFencing
		}
		// Authority clock, inside this transaction.
		var expired bool
		if err := tx.QueryRow(ctx,
			`SELECT clock_timestamp() > $1`, rowExpires,
		).Scan(&expired); err != nil {
			return err
		}
		if expired {
			return ErrLeaseExpired
		}
		if err := tx.QueryRow(ctx, `
			UPDATE leases SET expires_at = clock_timestamp() + make_interval(secs => $1)
			WHERE attempt_id = $2 RETURNING expires_at`, ttl.Seconds(), attemptID,
		).Scan(&expires); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	return expires, err
}

// AttemptCommit is the full fencing tuple of one authoritative attempt
// commit (spec §2). Every field is verified inside the commit transaction;
// anything missing/stale/mismatched is a typed refusal.
type AttemptCommit struct {
	JobID       string
	WorkspaceID string
	AttemptID   string
	WorkerID    string
	LeaseID     string
	// FencingEpoch must equal the claim epoch; epochs only increase.
	FencingEpoch int64
	// ExpectedJobVersion is optimistic-concurrency evidence: the caller
	// must have read the job version it saw.
	ExpectedJobVersion int64
	// JobState is the terminal job state this commit lands: succeeded,
	// failed or needs_reconciliation.
	JobState string
	// Receipt carries the durable audit line (digest-addressed; no secret
	// or raw content).
	ReceiptID    string
	ResultDigest string
	Outcome      string
}

// CommitAttempt lands one attempt's terminal outcome under the full
// fencing tuple, atomically: lease verified against the SERVER clock, job
// version verified, attempt + job rows landed, receipt written, journal +
// outbox appended — one transaction. A stale worker can never commit.
func (s *Store) CommitAttempt(ctx context.Context, c AttemptCommit) error {
	switch c.JobState {
	case "succeeded", "failed", "needs_reconciliation":
	default:
		return fmt.Errorf("remotestore: terminal job state %q is not committable: %w", c.JobState, ErrFencedRefused)
	}
	return RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		// Service-identity boundary (Remote Operations §1): the running
		// Attempt of a revoked worker resolves through the fencing rules —
		// its next authoritative commit refuses here and the lease expires
		// normally into reconciliation (fresh-epoch recovery takes over).
		if err := requireActiveServiceIdentity(ctx, tx, c.WorkerID); err != nil {
			return err
		}

		var epoch int64
		var expires time.Time
		err = tx.QueryRow(ctx, `
			SELECT fencing_epoch, expires_at FROM leases
			WHERE id = $1 AND attempt_id = $2 AND worker_id = $3 AND job_id = $4
			FOR UPDATE`, c.LeaseID, c.AttemptID, c.WorkerID, c.JobID,
		).Scan(&epoch, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStaleFencing
		}
		if err != nil {
			return err
		}
		if epoch != c.FencingEpoch {
			return ErrStaleFencing
		}
		// Authority clock inside the transaction: an expired lease never
		// commits (a fresh claim with a bumped epoch is the only recovery).
		var expired bool
		if err := tx.QueryRow(ctx,
			`SELECT clock_timestamp() > $1`, expires,
		).Scan(&expired); err != nil {
			return err
		}
		if expired {
			return ErrLeaseExpired
		}

		var state string
		var version int64
		if err := tx.QueryRow(ctx,
			`SELECT state, job_version FROM jobs WHERE id = $1 FOR UPDATE`, c.JobID,
		).Scan(&state, &version); err != nil {
			return err
		}
		if state != "running" {
			return fmt.Errorf("remotestore: job %s is %q: %w", c.JobID, state, ErrFencedRefused)
		}
		if version != c.ExpectedJobVersion {
			return fmt.Errorf("remotestore: job %s version moved (%d != %d): %w",
				c.JobID, version, c.ExpectedJobVersion, ErrStaleFencing)
		}

		if _, err := tx.Exec(ctx,
			`UPDATE attempts SET state = $1 WHERE id = $2`, c.JobState, c.AttemptID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET state = $1, job_version = job_version + 1, updated_at = now()
			WHERE id = $2`, c.JobState, c.JobID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_receipts (id, workspace_id, job_id, attempt_id, outcome, result_digest)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ReceiptID, c.WorkspaceID, c.JobID, c.AttemptID, c.Outcome, c.ResultDigest,
		); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, c.WorkspaceID,
			fmt.Sprintf("job.completed:%s:%d", c.JobID, c.FencingEpoch),
			"job", c.JobID, "job."+c.JobState, map[string]any{
				"job_id": c.JobID, "attempt_id": c.AttemptID, "receipt_id": c.ReceiptID,
				"result_digest": c.ResultDigest, "fencing_epoch": c.FencingEpoch,
			}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// appendEvent writes journal + outbox in the CALLER's transaction (state
// change + journal + outbox commit together, spec §2). The event id is the
// dedupe key: a replayed id is a no-op (consumers apply once). Callers
// must never perform external I/O inside the same transaction.
func appendEvent(ctx context.Context, tx pgx.Tx, workspaceID, eventID, aggregate, aggregateID, eventType string, payload map[string]any) error {
	var existing string
	err := tx.QueryRow(ctx,
		`SELECT event_id FROM event_journal WHERE event_id = $1`, eventID,
	).Scan(&existing)
	if err == nil {
		return nil // already applied (idempotent replay)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var seq int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO workspace_seq (workspace_id, last_seq) VALUES ($1, 1)
		ON CONFLICT (workspace_id) DO UPDATE SET last_seq = workspace_seq.last_seq + 1
		RETURNING last_seq`, workspaceID,
	).Scan(&seq); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO event_journal (workspace_id, seq, event_id, aggregate, aggregate_id, type, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		workspaceID, seq, eventID, aggregate, aggregateID, eventType, payload,
	); err != nil {
		// A concurrent writer landed the same event id first: the event is
		// applied exactly once either way (unique event_id). Treat as
		// applied — the retry helper re-runs only conflict classes.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox (event_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING`, eventID, workspaceID,
	); err != nil {
		return err
	}
	return nil
}

// RetryTransaction re-runs a transactional unit on PostgreSQL
// serialization-failure (40001) and deadlock (40P01) classes, bounded,
// with a small backoff — per spec §3 the TRANSACTION retries and never an
// external effect (effects live outside transactions).
func RetryTransaction(ctx context.Context, attempts int, unit func(ctx context.Context) error) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		err := unit(ctx)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || (pgErr.Code != "40001" && pgErr.Code != "40P01") {
			return err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 25 * time.Millisecond):
		}
	}
	return fmt.Errorf("remotestore: transaction did not settle after %d attempts: %w", attempts, lastErr)
}
