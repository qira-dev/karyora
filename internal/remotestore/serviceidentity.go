package remotestore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The workload-scoped service-identity lifecycle (C4-38 implementation
// remainder; Remote Operations Specification §1): each worker deployment
// receives its own service principal credential, scoped to named
// workspaces only — workers never hold human sessions, secret-handle
// values, or cross-workspace scopes.
//
// Lifecycle: issue → bind (the issuance workspaces; the capability set is
// the ACCEPTED operation-to-policy map's fixed worker row — job.read,
// job.cancel, secret.execute, object.read — which claiming never widens)
// → rotate (versioned credentials; the Proposed 90-day numeric is NOT
// enforced here — rotation is operator-driven and the numeric stays
// Proposed, D-28) → revoke (typed, immediate at the claim boundary).
//
// The credential plaintext is generated server-side, returned ONCE at
// issue/rotate, and stored ONLY as a SHA-256 digest. It never enters an
// error string, a log line, or a journal payload. Revoked identities and
// credentials are refused at every authoritative boundary: claim
// (credential + binding verification inside the claim transaction),
// renewal and commit (identity-state check inside the transaction), so a
// running Attempt under a revoked worker resolves through the fencing
// rules — its lease expires normally into reconciliation and a fresh
// claim with a bumped epoch takes the Job over.
//
// Issuance/rotation authorization (Owner/Admin per the operation map)
// stays with the future remote session surface — the same boundary as the
// teamauth adapter's presented principal. Nothing here is wired into any
// product surface; Local mode has zero dependency on this package.

// Typed failures of the service-identity boundary (stable, inspectable).
var (
	// ErrServiceIdentityRefused: the presenting worker's identity is
	// unknown, revoked, or the credential does not verify against the
	// current version — the claim/renew/commit boundary refuses.
	ErrServiceIdentityRefused = errors.New("remotestore: service identity refused")
	// ErrWorkspaceNotBound: at least one requested workspace is not bound
	// to this identity — workers never hold cross-workspace scopes, so
	// the whole claim refuses (fail closed, nothing claimed).
	ErrWorkspaceNotBound = errors.New("remotestore: workspace not bound to this service identity")
)

// credentialEntropyBytes is the credential token length (256 bits).
const credentialEntropyBytes = 32

// hashCredential derives the stored digest of a credential token. Only
// the digest is persisted; the plaintext never crosses this boundary
// again.
func hashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

// IssueServiceIdentity creates the service principal (state active),
// binds it to the named workspaces, and issues credential version 1. The
// plaintext credential is returned ONCE — the caller delivers it to the
// worker deployment through the approved secret mechanism; the store
// keeps only its digest. Duplicate identity ids and empty workspace sets
// fail closed.
func (s *Store) IssueServiceIdentity(ctx context.Context, identityID string, workspaceIDs []string) (string, error) {
	if !boundedID(identityID) {
		return "", fmt.Errorf("remotestore: identity id is missing or unbounded: %w", ErrServiceIdentityRefused)
	}
	if len(workspaceIDs) == 0 {
		return "", fmt.Errorf("remotestore: issue needs at least one bound workspace: %w", ErrWorkspaceNotBound)
	}
	for _, ws := range workspaceIDs {
		if !boundedID(ws) {
			return "", fmt.Errorf("remotestore: workspace id is missing or unbounded: %w", ErrWorkspaceNotBound)
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var exists int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM service_identities WHERE id = $1`, identityID,
	).Scan(&exists); err != nil {
		return "", err
	}
	if exists != 0 {
		return "", fmt.Errorf("remotestore: service identity %s already exists: %w", identityID, ErrServiceIdentityRefused)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO service_identities (id, state) VALUES ($1, 'active')`, identityID,
	); err != nil {
		return "", err
	}
	for _, ws := range workspaceIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO service_identity_bindings (identity_id, workspace_id)
			VALUES ($1, $2)`, identityID, ws,
		); err != nil {
			return "", fmt.Errorf("remotestore: bind %s: %w", ws, err)
		}
	}
	credential, err := issueCredential(ctx, tx, identityID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return credential, nil
}

// RotateServiceIdentity issues the next credential version and revokes
// every previous version immediately (Remote Operations §1: rotation; the
// Proposed 90-day cadence is an operator schedule, never enforced here).
// The old credential stops verifying at the next authoritative boundary.
func (s *Store) RotateServiceIdentity(ctx context.Context, identityID string) (string, error) {
	if !boundedID(identityID) {
		return "", fmt.Errorf("remotestore: identity id is missing or unbounded: %w", ErrServiceIdentityRefused)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var state string
	err = tx.QueryRow(ctx,
		`SELECT state FROM service_identities WHERE id = $1 FOR UPDATE`, identityID,
	).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("remotestore: service identity %s is unknown: %w", identityID, ErrServiceIdentityRefused)
	}
	if err != nil {
		return "", err
	}
	if state != "active" {
		return "", fmt.Errorf("remotestore: service identity %s is %s: %w", identityID, state, ErrServiceIdentityRefused)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE service_identity_credentials SET revoked_at = now()
		WHERE identity_id = $1 AND revoked_at IS NULL`, identityID,
	); err != nil {
		return "", err
	}
	credential, err := issueCredential(ctx, tx, identityID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return credential, nil
}

// RevokeServiceIdentity flips the identity to revoked — terminal,
// effective at the next authoritative boundary: claims refuse the
// credential, and any running Attempt's renewal and authoritative commit
// refuse, so its lease expires normally into reconciliation (Remote
// Operations §1). Revoking an already-revoked identity is idempotent.
func (s *Store) RevokeServiceIdentity(ctx context.Context, identityID string) error {
	if !boundedID(identityID) {
		return fmt.Errorf("remotestore: identity id is missing or unbounded: %w", ErrServiceIdentityRefused)
	}
	return RetryTransaction(ctx, 3, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)

		var state string
		err = tx.QueryRow(ctx,
			`SELECT state FROM service_identities WHERE id = $1 FOR UPDATE`, identityID,
		).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("remotestore: service identity %s is unknown: %w", identityID, ErrServiceIdentityRefused)
		}
		if err != nil {
			return err
		}
		if state == "revoked" {
			return nil // idempotent: the revoked state is the goal
		}
		if _, err := tx.Exec(ctx, `
			UPDATE service_identities SET state = 'revoked', revoked_at = now()
			WHERE id = $1`, identityID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE service_identity_credentials SET revoked_at = now()
			WHERE identity_id = $1 AND revoked_at IS NULL`, identityID,
		); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

// issueCredential generates a fresh one-time credential (256 bits of CSPRNG
// entropy), persists its digest as the next version, and returns the
// plaintext exactly once. The caller's transaction owns the write so the
// credential version lands atomically with the lifecycle operation.
func issueCredential(ctx context.Context, tx pgx.Tx, identityID string) (string, error) {
	raw := make([]byte, credentialEntropyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("remotestore: credential entropy: %w", err)
	}
	credential := base64.RawURLEncoding.EncodeToString(raw)
	var version int
	if err := tx.QueryRow(ctx, `
		INSERT INTO service_identity_credentials (identity_id, version, credential_hash)
		VALUES ($1, COALESCE((SELECT max(version) FROM service_identity_credentials WHERE identity_id = $2), 0) + 1, $3)
		RETURNING version`, identityID, identityID, hashCredential(credential),
	).Scan(&version); err != nil {
		return "", fmt.Errorf("remotestore: record credential version: %w", err)
	}
	return credential, nil
}

// verifyServiceIdentityCredential authenticates the presenting worker at
// the claim boundary, INSIDE the claim transaction: the identity must
// exist and be ACTIVE, the credential must verify (constant time) against
// the identity's CURRENT unrevoked version, and EVERY requested workspace
// must be bound to the identity — one unbound scope refuses the whole
// claim (fail closed; workers never hold cross-workspace scopes).
func verifyServiceIdentityCredential(ctx context.Context, tx pgx.Tx, workerID, credential string, workspaces []string) error {
	if !boundedID(workerID) || credential == "" {
		return ErrServiceIdentityRefused
	}
	var state string
	err := tx.QueryRow(ctx,
		`SELECT state FROM service_identities WHERE id = $1`, workerID,
	).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrServiceIdentityRefused
	}
	if err != nil {
		return err
	}
	if state != "active" {
		return ErrServiceIdentityRefused
	}
	var currentHash string
	err = tx.QueryRow(ctx, `
		SELECT credential_hash FROM service_identity_credentials
		WHERE identity_id = $1 AND revoked_at IS NULL
		ORDER BY version DESC
		LIMIT 1`, workerID,
	).Scan(&currentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrServiceIdentityRefused
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(currentHash), []byte(hashCredential(credential))) != 1 {
		return ErrServiceIdentityRefused
	}
	var bound int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM service_identity_bindings
		WHERE identity_id = $1 AND workspace_id = ANY($2)`, workerID, workspaces,
	).Scan(&bound); err != nil {
		return err
	}
	if bound != len(workspaces) {
		return ErrWorkspaceNotBound
	}
	return nil
}

// requireActiveServiceIdentity refuses renewal and authoritative commit
// for a worker whose identity is unknown or revoked (Remote Operations
// §1: the running Attempt resolves through the fencing rules — the next
// authoritative boundary refuses and the lease expires normally into
// reconciliation). Runs inside the caller's transaction so a revocation
// is visible to every boundary that starts after it commits.
func requireActiveServiceIdentity(ctx context.Context, tx pgx.Tx, workerID string) error {
	if !boundedID(workerID) {
		return ErrServiceIdentityRefused
	}
	var state string
	err := tx.QueryRow(ctx,
		`SELECT state FROM service_identities WHERE id = $1`, workerID,
	).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrServiceIdentityRefused
	}
	if err != nil {
		return err
	}
	if state != "active" {
		return ErrServiceIdentityRefused
	}
	return nil
}
