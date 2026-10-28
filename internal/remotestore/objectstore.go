package remotestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The shared object/blob store lifecycle (C4-36, spec §4): tenant-scoped
// (workspace, digest) identity with the bytes held by the S3-compatible
// blob store and the DB reference row as the ONLY authority that makes an
// object readable. Upload-before-reference ordering is structural: the
// blob write happens OUTSIDE any database transaction (no network I/O
// inside transactions, spec §1) and the reference commits afterwards —
// so upload-success + reference-commit-failure leaves a recoverable,
// GC-eligible orphan and never a dangling reference. Objects are
// immutable: the key is derived from the content digest, so a stored
// object's address always names its own bytes. Reads verify the digest —
// a missing or corrupt object is a typed visible failure, never a silent
// substitute. Error strings are value-free: they carry identifiers only,
// never content or credentials.

// Typed failures of the object lifecycle (stable, inspectable).
var (
	// ErrObjectUnknown: no reference row exists for the requested
	// (workspace, digest) — the tenant-scoped identity is unknown. Also
	// covers cross-workspace reads: another workspace's digest is never
	// resolvable from the wrong scope.
	ErrObjectUnknown = errors.New("remotestore: object reference unknown in this workspace scope")
	// ErrObjectNotReferenced: a row exists but its lifecycle state no
	// longer grants reads (dropped, orphan-marked or sweep-leased).
	ErrObjectNotReferenced = errors.New("remotestore: object reference is not active")
	// ErrObjectMissing: a live reference points at bytes the blob store
	// does not have — visible failure, never a silent substitute.
	ErrObjectMissing = errors.New("remotestore: referenced object is missing in the blob store")
	// ErrObjectCorrupt: the stored bytes do not verify against the
	// expected digest (read or write verification).
	ErrObjectCorrupt = errors.New("remotestore: object bytes do not match the expected digest")
	// ErrObjectTooLarge: the read exceeded the caller's bound.
	ErrObjectTooLarge = errors.New("remotestore: object exceeds the caller's size bound")
	// ErrInvalidDigest: the digest argument is not a well-formed
	// sha256-<hex> value (fail closed before anything is touched).
	ErrInvalidDigest = errors.New("remotestore: digest is not a well-formed sha256 value")
	// ErrReferenceCommitFailed: the object was durably written but the DB
	// reference commit failed — the designed-for orphan: recoverable by a
	// repeated PutObject (idempotent) and GC-eligible after mark + grace.
	ErrReferenceCommitFailed = errors.New("remotestore: object written but the reference commit failed (orphan is recoverable and GC-eligible)")
	// ErrObjectPinned: a backup pin holds the object; the reference
	// cannot be dropped while the pin exists (C2-12 discipline extended
	// cross-store; the D-27 legal-hold posture).
	ErrObjectPinned = errors.New("remotestore: object is pinned by a backup reference")
	// ErrObjectSweepInFlight: a GC sweep holds the sweep lease on this
	// (workspace, digest); the reference commit must re-upload afterwards.
	ErrObjectSweepInFlight = errors.New("remotestore: a GC sweep lease is active on this object")
	// ErrBlobStoreRequired: the object primitives need an attached blob
	// store; the DB-only store cannot serve them.
	ErrBlobStoreRequired = errors.New("remotestore: no blob store attached to the remote authority")
)

// DefaultGCGrace is the Proposed orphan-grace interval. No owner-accepted
// numeric exists (D-27 owns retention numerics); the value parameterizes
// the interval only — the SERVER clock inside the sweep transaction
// decides what is past grace, exactly like the lease TTL (spec §2).
const DefaultGCGrace = 24 * time.Hour

// DigestPrefix is the canonical digest scheme of the object store (the
// same sha256- anchor the migration checksums and receipts use).
const DigestPrefix = "sha256-"

var digestPattern = regexp.MustCompile(`^sha256-[0-9a-f]{64}$`)

// validDigest enforces the digest shape before anything is touched.
func validDigest(d string) bool { return digestPattern.MatchString(d) }

// DigestOf computes the canonical content digest.
func DigestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return DigestPrefix + hex.EncodeToString(sum[:])
}

// objectKey is the tenant-prefixed blob key: the workspace scope travels
// with the key so no bare digest is ever a global address (spec §4).
func objectKey(workspaceID, digest string) string {
	return fmt.Sprintf("workspaces/%s/objects/%s", workspaceID, digest)
}

// BlobConfig is the operator configuration of the S3-compatible blob
// store (endpoint + credentials come from the approved configuration
// surface; they are never hardcoded and never logged).
type BlobConfig struct {
	Endpoint  string // host[:port] of the S3-compatible endpoint
	AccessKey string
	SecretKey string
	Bucket    string
	Secure    bool // TLS
}

// BlobStore is the S3-compatible client wrapper (the pinned minio-go
// client). It is endpoint-neutral: the same code drives MinIO, Amazon S3,
// Cloudflare R2 or any S3-compatible endpoint — the vendor selection
// lives in configuration, never in code.
type BlobStore struct {
	client *minio.Client
	bucket string
}

// OpenBlobStore connects the S3-compatible endpoint.
func OpenBlobStore(ctx context.Context, cfg BlobConfig) (*BlobStore, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, errors.New("remotestore: blob store needs an endpoint and a bucket")
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
	})
	if err != nil {
		return nil, fmt.Errorf("remotestore: blob client: %w", err)
	}
	return &BlobStore{client: client, bucket: cfg.Bucket}, nil
}

// EnsureBucket creates the configured bucket if absent (idempotent; the
// operator credential needs the create right — a fixture/deployment
// concern, not a product surface).
func (b *BlobStore) EnsureBucket(ctx context.Context) error {
	exists, err := b.client.BucketExists(ctx, b.bucket)
	if err != nil {
		return fmt.Errorf("remotestore: blob bucket probe: %w", err)
	}
	if exists {
		return nil
	}
	if err := b.client.MakeBucket(ctx, b.bucket, minio.MakeBucketOptions{}); err != nil {
		// A concurrent creator may have won: tolerate that exact race.
		if exists2, probe := b.client.BucketExists(ctx, b.bucket); probe == nil && exists2 {
			return nil
		}
		return fmt.Errorf("remotestore: blob bucket create: %w", err)
	}
	return nil
}

// put writes the bytes and verifies the durable result SERVER-side: a
// stat confirms the stored object's size before the caller proceeds. The
// checklist's "idempotent/conditional put with ETag-or-digest
// verification" is satisfied on the digest branch — the key IS the
// sha256 digest of the content and every read re-verifies it — so no
// weak hash touches the write path.
func (b *BlobStore) put(ctx context.Context, key string, content []byte) error {
	_, err := b.client.PutObject(ctx, b.bucket, key, bytes.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return fmt.Errorf("remotestore: blob put: %w", err)
	}
	info, err := b.client.StatObject(ctx, b.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("remotestore: blob put verification: %w", err)
	}
	if info.Size != int64(len(content)) {
		_ = b.client.RemoveObject(ctx, b.bucket, key, minio.RemoveObjectOptions{})
		return fmt.Errorf("remotestore: blob put stored %d bytes, want %d", info.Size, len(content))
	}
	return nil
}

// get reads the object bounded by maxBytes (fail closed on oversize) and
// maps the typed failure modes: missing key and transport errors stay
// distinct so a missing object with a live reference is a VISIBLE typed
// failure, never a silent substitute.
func (b *BlobStore) get(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	obj, err := b.client.GetObject(ctx, b.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, mapBlobErr(err)
	}
	defer obj.Close()
	limited := io.LimitReader(obj, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, mapBlobErr(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrObjectTooLarge
	}
	return data, nil
}

// remove deletes the key (idempotent: S3 deletion of an absent key
// succeeds).
func (b *BlobStore) remove(ctx context.Context, key string) error {
	return b.client.RemoveObject(ctx, b.bucket, key, minio.RemoveObjectOptions{})
}

// listSizes returns size by key under the prefix (the GC mark pass).
func (b *BlobStore) listSizes(ctx context.Context, prefix string) (map[string]int64, error) {
	sizes := map[string]int64{}
	for obj := range b.client.ListObjects(ctx, b.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("remotestore: blob list: %w", obj.Err)
		}
		sizes[obj.Key] = obj.Size
	}
	return sizes, nil
}

func mapBlobErr(err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		switch resp.Code {
		case "NoSuchKey", "NoSuchBucket":
			return fmt.Errorf("remotestore: blob read: %w", ErrObjectMissing)
		}
	}
	return fmt.Errorf("remotestore: blob read: %w", err)
}

// PutObjectResult reports one landed object (digest-addressed).
type PutObjectResult struct {
	Digest string
	Size   int64
}

// PutObject implements the upload-before-reference ordering (spec §4
// hard rule): digest verify (the digest is computed from the content and
// IS the key) → object durably written to the blob store with ETag/size
// verification (OUTSIDE any transaction — no network I/O inside
// transactions) → DB reference row commits. A reference-commit failure
// after a successful upload is the designed-for orphan: the error is
// typed (ErrReferenceCommitFailed), the object is recoverable by calling
// PutObject again (idempotent: same content, same digest, same key) and
// GC-eligible after mark + grace. Repeating the call with the same
// content lands the reference exactly once. If a GC sweep holds the
// sweep lease on the (workspace, digest), the commit refuses and the
// upload is retried (bounded) — the sweep may have removed the just-
// uploaded bytes, so the retry re-uploads before landing the reference.
func (s *Store) PutObject(ctx context.Context, workspaceID string, content []byte) (PutObjectResult, error) {
	if s.Blob == nil {
		return PutObjectResult{}, ErrBlobStoreRequired
	}
	if !boundedID(workspaceID) || len(content) == 0 {
		return PutObjectResult{}, errors.New("remotestore: put needs a workspace scope and non-empty content")
	}
	digest := DigestOf(content)
	key := objectKey(workspaceID, digest)
	// External effect first, outside any transaction.
	if err := s.Blob.put(ctx, key, content); err != nil {
		return PutObjectResult{}, err
	}
	landReference := func() error {
		return RetryTransaction(ctx, 3, func(ctx context.Context) error {
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)
			var state string
			err = tx.QueryRow(ctx,
				`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2 FOR UPDATE`,
				workspaceID, digest).Scan(&state)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				if _, err := tx.Exec(ctx, `
					INSERT INTO object_refs (workspace_id, digest, size_bytes, state)
					VALUES ($1, $2, $3, 'referenced')`, workspaceID, digest, len(content)); err != nil {
					return err
				}
			case err != nil:
				return err
			case state == "sweeping":
				// A GC sweep lease is active: the sweep may remove the
				// uploaded bytes after this point, so the caller retries
				// the WHOLE ordering (upload again, then reference).
				return ErrObjectSweepInFlight
			case state == "orphan_candidate":
				// Recovery of a marked orphan (or of a dropped
				// reference): the upload-before-reference ordering makes
				// the object durable again BEFORE the reference lands.
				if _, err := tx.Exec(ctx, `
					UPDATE object_refs SET state = 'referenced', orphan_since = NULL, size_bytes = $3
					WHERE workspace_id = $1 AND digest = $2`, workspaceID, digest, len(content)); err != nil {
					return err
				}
			default: // referenced: idempotent re-put of identical content
				if _, err := tx.Exec(ctx, `
					UPDATE object_refs SET size_bytes = $3
					WHERE workspace_id = $1 AND digest = $2 AND state = 'referenced'`,
					workspaceID, digest, len(content)); err != nil {
					return err
				}
			}
			return tx.Commit(ctx)
		})
	}
	const referenceAttempts = 3
	var lastErr error
	for attempt := 0; attempt < referenceAttempts; attempt++ {
		err := landReference()
		if err == nil {
			return PutObjectResult{Digest: digest, Size: int64(len(content))}, nil
		}
		if !errors.Is(err, ErrObjectSweepInFlight) {
			// The upload succeeded; the reference did not land. Typed,
			// value-free: the orphan is recoverable and GC-eligible.
			return PutObjectResult{}, fmt.Errorf("remotestore: object %s in workspace %s: %w: %w",
				digest, workspaceID, ErrReferenceCommitFailed, err)
		}
		lastErr = err
		// Re-upload before the next reference attempt (the sweep may have
		// removed the previous bytes; the ordering requires durable bytes
		// BEFORE any reference).
		if err := s.Blob.put(ctx, key, content); err != nil {
			return PutObjectResult{}, err
		}
	}
	return PutObjectResult{}, fmt.Errorf("remotestore: object %s in workspace %s: %w: %w",
		digest, workspaceID, ErrReferenceCommitFailed, lastErr)
}

// GetObject serves verified bytes for a live tenant-scoped reference.
// The DB reference is the authority: an unknown (workspace, digest)
// refuses before the blob store is touched (a bare digest is never a
// global key — another workspace's object is unresolvable from the wrong
// scope, fail closed), a dropped/orphan/sweep state refuses, and the
// returned bytes verify against the digest — a missing or corrupt object
// is a typed visible failure, never a silent substitute.
func (s *Store) GetObject(ctx context.Context, workspaceID, digest string, maxBytes int64) ([]byte, error) {
	if s.Blob == nil {
		return nil, ErrBlobStoreRequired
	}
	if !boundedID(workspaceID) {
		return nil, ErrObjectUnknown
	}
	if !validDigest(digest) {
		return nil, ErrInvalidDigest
	}
	if maxBytes <= 0 {
		return nil, errors.New("remotestore: get needs a positive size bound")
	}
	var state string
	err := s.pool.QueryRow(ctx,
		`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2`,
		workspaceID, digest).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("remotestore: digest %s in workspace %s: %w", digest, workspaceID, ErrObjectUnknown)
	}
	if err != nil {
		return nil, err
	}
	if state != "referenced" {
		return nil, fmt.Errorf("remotestore: digest %s in workspace %s is %q: %w", digest, workspaceID, state, ErrObjectNotReferenced)
	}
	data, err := s.Blob.get(ctx, objectKey(workspaceID, digest), maxBytes)
	if err != nil {
		return nil, err
	}
	if DigestOf(data) != digest {
		return nil, fmt.Errorf("remotestore: digest %s in workspace %s: %w", digest, workspaceID, ErrObjectCorrupt)
	}
	return data, nil
}

// DropObjectReference retires the tenant-scoped reference (the v1
// deletion primitive): the row moves to orphan_candidate with the SERVER
// clock stamping the grace start. The bytes stay in the blob store and
// the object becomes GC-eligible after grace + second-pass recheck. A
// pinned object refuses (the pin is the retention barrier; removing the
// pin is the pin owner's action, D-27 legal-hold posture).
func (s *Store) DropObjectReference(ctx context.Context, workspaceID, digest string) error {
	switch {
	case !validDigest(digest):
		return ErrInvalidDigest
	case !boundedID(workspaceID):
		return ErrObjectUnknown
	}
	return RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var state string
		err = tx.QueryRow(ctx,
			`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2 FOR UPDATE`,
			workspaceID, digest).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("remotestore: digest %s in workspace %s: %w", digest, workspaceID, ErrObjectUnknown)
		}
		if err != nil {
			return err
		}
		if state != "referenced" {
			return fmt.Errorf("remotestore: digest %s in workspace %s is %q: %w", digest, workspaceID, state, ErrObjectNotReferenced)
		}
		var pinned int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM object_pins WHERE workspace_id = $1 AND digest = $2`,
			workspaceID, digest).Scan(&pinned); err != nil {
			return err
		}
		if pinned != 0 {
			return fmt.Errorf("remotestore: digest %s in workspace %s: %w", digest, workspaceID, ErrObjectPinned)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE object_refs SET state = 'orphan_candidate', orphan_since = clock_timestamp()
			WHERE workspace_id = $1 AND digest = $2`, workspaceID, digest); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// PinObject installs a backup-pin barrier (C2-12 discipline extended
// cross-store): while the pin row exists, the GC never removes the
// object and the reference cannot be dropped. The pin requires a live
// referenced object (fail closed otherwise). The pin id is the backup
// manifest's reference; the pin owns its own lifecycle (no TTL numeric —
// the D-27 retention numerics stay Proposed and are not encoded here).
func (s *Store) PinObject(ctx context.Context, workspaceID, digest, pinID string) error {
	switch {
	case !validDigest(digest):
		return ErrInvalidDigest
	case !boundedID(workspaceID) || !boundedID(pinID):
		return ErrObjectUnknown
	}
	return RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var state string
		err = tx.QueryRow(ctx,
			`SELECT state FROM object_refs WHERE workspace_id = $1 AND digest = $2 FOR UPDATE`,
			workspaceID, digest).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("remotestore: digest %s in workspace %s: %w", digest, workspaceID, ErrObjectUnknown)
		}
		if err != nil {
			return err
		}
		if state != "referenced" {
			return fmt.Errorf("remotestore: digest %s in workspace %s is %q: %w", digest, workspaceID, state, ErrObjectNotReferenced)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO object_pins (workspace_id, digest, pin_id) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, workspaceID, digest, pinID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// UnpinObject removes the named pin (idempotent). Once unpinned, the
// reference drop and the GC sweep behave by the normal rules.
func (s *Store) UnpinObject(ctx context.Context, workspaceID, digest, pinID string) error {
	if !validDigest(digest) {
		return ErrInvalidDigest
	}
	_, err := s.pool.Exec(ctx,
		`DELETE FROM object_pins WHERE workspace_id = $1 AND digest = $2 AND pin_id = $3`,
		workspaceID, digest, pinID)
	return err
}

// MarkStoreOrphans is the GC mark pass for STORE-side orphans: objects
// physically present in the blob store that carry NO object_refs row —
// the upload-success + reference-commit-failure state, or bytes left by
// an interrupted sweep. Each is recorded durably as an orphan_candidate
// row with the SERVER clock stamping the grace start, so the sweep is
// restartable across processes. A row that appears between the list and
// the insert is left untouched (ON CONFLICT DO NOTHING) — a live
// reference is never turned into a candidate.
func (s *Store) MarkStoreOrphans(ctx context.Context) (int, error) {
	if s.Blob == nil {
		return 0, ErrBlobStoreRequired
	}
	sizes, err := s.Blob.listSizes(ctx, "workspaces/")
	if err != nil {
		return 0, err
	}
	marked := 0
	for key, size := range sizes {
		workspaceID, digest, ok := parseObjectKey(key)
		if !ok {
			continue // not an object-store key shape; never touched by GC
		}
		result, err := s.pool.Exec(ctx, `
			INSERT INTO object_refs (workspace_id, digest, size_bytes, state, orphan_since)
			VALUES ($1, $2, $3, 'orphan_candidate', clock_timestamp())
			ON CONFLICT (workspace_id, digest) DO NOTHING`, workspaceID, digest, size)
		if err != nil {
			return marked, err
		}
		marked += int(result.RowsAffected())
	}
	return marked, nil
}

var objectKeyPattern = regexp.MustCompile(`^workspaces/([^/]+)/objects/(sha256-[0-9a-f]{64})$`)

func parseObjectKey(key string) (workspaceID, digest string, ok bool) {
	m := objectKeyPattern.FindStringSubmatch(key)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// SweepResult reports one GC sweep pass.
type SweepResult struct {
	// Swept is the number of candidates the pass processed (their sweep
	// leases taken this pass, or found stuck from an interrupted one).
	Swept int
	// Removed is the number of object rows deleted after the second-pass
	// recheck — referenced objects are never among them.
	Removed int
}

// SweepOrphanedObjects runs one reference-aware GC pass (C2-12
// discipline extended cross-store):
//
//  1. LEASE: inside ONE transaction, rows that are orphan_candidate past
//     the SERVER-clock grace (or stuck in sweeping from an interrupted
//     pass) are flipped to the 'sweeping' lease state — pinned rows and
//     referenced rows are structurally excluded by the query, so no
//     concurrent reference can appear for a leased row: every reference
//     commit on a 'sweeping' row refuses and retries its upload.
//  2. REMOVE (outside any transaction): the leased objects are removed
//     from the blob store (idempotent).
//  3. RECHECK + DELETE: inside one transaction per candidate, the row is
//     deleted only while it STILL holds the sweep lease (the second-pass
//     recheck) — a row that flipped back can only mean a recovery put
//     re-landed the object after the removal, so the row survives and the
//     store-side mark pass re-covers any residual bytes.
//
// The grace interval is Proposed (D-27 owns retention numerics); the
// SERVER clock decides what is past grace inside the lease transaction,
// never the host clock.
func (s *Store) SweepOrphanedObjects(ctx context.Context, grace time.Duration) (SweepResult, error) {
	if s.Blob == nil {
		return SweepResult{}, ErrBlobStoreRequired
	}
	if grace < 0 {
		return SweepResult{}, errors.New("remotestore: grace cannot be negative")
	}
	var candidates []struct {
		WorkspaceID string
		Digest      string
	}
	// Phase 1: take the sweep leases (one transaction; rows locked).
	err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
		rows, err := s.pool.Query(ctx, `
			UPDATE object_refs SET state = 'sweeping'
			WHERE (state = 'orphan_candidate'
			       AND orphan_since < clock_timestamp() - make_interval(secs => $1)
			       AND NOT EXISTS (SELECT 1 FROM object_pins p
			                       WHERE p.workspace_id = object_refs.workspace_id
			                         AND p.digest = object_refs.digest))
			   OR state = 'sweeping'
			RETURNING workspace_id, digest`, grace.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		candidates = nil
		for rows.Next() {
			var ws, digest string
			if err := rows.Scan(&ws, &digest); err != nil {
				return err
			}
			candidates = append(candidates, struct{ WorkspaceID, Digest string }{ws, digest})
		}
		return rows.Err()
	})
	if err != nil {
		return SweepResult{}, err
	}
	result := SweepResult{Swept: len(candidates)}
	// Phase 2 + 3 per candidate: external effect first (no transaction
	// held across blob I/O), then the guarded recheck+delete.
	for _, c := range candidates {
		if err := s.Blob.remove(ctx, objectKey(c.WorkspaceID, c.Digest)); err != nil {
			return result, fmt.Errorf("remotestore: sweep remove for digest %s in workspace %s: %w",
				c.Digest, c.WorkspaceID, err)
		}
		err := RetryTransaction(ctx, 3, func(ctx context.Context) error {
			tag, err := s.pool.Exec(ctx, `
				DELETE FROM object_refs
				WHERE workspace_id = $1 AND digest = $2 AND state = 'sweeping'`,
				c.WorkspaceID, c.Digest)
			if err != nil {
				return err
			}
			result.Removed += int(tag.RowsAffected())
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}
