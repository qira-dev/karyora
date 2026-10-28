package remotestore

import (
	"context"

	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/security/teamauth"
)

// StoreDirectory is the CURRENT-authority identity view behind the
// teamauth.Directory port (CR-4.4 identity sub-milestone, C4-30/C4-32):
// membership role/state, resource tenancy and the live worker delegation
// are read FRESH from the store on every call — a membership revocation,
// a lease expiry or a submitter revocation is visible on the next
// decision, never cached. Every failure (missing row, ambiguous tenancy,
// store error, malformed input) resolves as ABSENT — the team adapter
// denies, fail closed; the port carries no error return by design.
//
// Tenant identity comes only from the store's scope columns, never from a
// client-supplied field (Tenant Isolation Matrix rule 1). Planes without
// a remote v1 table (sessions, knowledge, secret handles, connections,
// runs, org authority) resolve absent: their surfaces land together with
// their rows in future milestones. Nothing here is wired into a product
// surface yet — remote mode does not exist (isolation rule; Local mode
// has zero dependency on this package).
type StoreDirectory struct {
	store *Store
}

var _ teamauth.Directory = (*StoreDirectory)(nil)

// Directory returns the teamauth.Directory view resolved against this
// store. Calls run on the pool with its configured DSN timeouts; the
// decision latency is bounded by the pool, never unbounded.
func (s *Store) Directory() *StoreDirectory { return &StoreDirectory{store: s} }

// maxIDLen bounds every identifier crossing the Directory (mirrors the
// security and teamauth package bounds).
const maxIDLen = 128

func boundedID(s string) bool { return s != "" && len(s) <= maxIDLen }

// RoleOf resolves the member's CURRENT role and state in one tenant scope
// (the memberships table; the org for org-scoped resources). Role and
// state values outside the accepted closed sets resolve absent (fail
// closed) even if a future migration widened the CHECK constraints.
func (d *StoreDirectory) RoleOf(principalID, tenantID string) (teamauth.Role, teamauth.MemberState, bool) {
	if d.store == nil || !boundedID(principalID) || !boundedID(tenantID) {
		return "", "", false
	}
	var roleStr, stateStr string
	err := d.store.pool.QueryRow(context.Background(),
		`SELECT role, state FROM memberships WHERE principal_id = $1 AND workspace_id = $2`,
		principalID, tenantID,
	).Scan(&roleStr, &stateStr)
	if err != nil {
		return "", "", false
	}
	role, state := teamauth.Role(roleStr), teamauth.MemberState(stateStr)
	switch role {
	case teamauth.RoleOwner, teamauth.RoleAdmin, teamauth.RoleMember, teamauth.RoleViewer:
	default:
		return "", "", false
	}
	switch state {
	case teamauth.MemberActive, teamauth.MemberRevoked:
	default:
		return "", "", false
	}
	return role, state, true
}

// WorkspaceOfResource resolves the tenant owning a protected resource from
// the store's scope columns. Resolved in schema v1: the workspace itself,
// the org id (present while any workspace carries it), Jobs (the job row's
// workspace column) and object references (object_refs). A bare digest
// referenced by MORE THAN ONE workspace is ambiguous tenancy and resolves
// absent — a bare content digest is never a global key (storage spec §4);
// the object surface carries the workspace scope with it once it exists.
func (d *StoreDirectory) WorkspaceOfResource(r security.Resource) (string, bool) {
	if d.store == nil || !boundedID(r.ID) {
		return "", false
	}
	ctx := context.Background()
	switch r.Type {
	case teamauth.ResourceWorkspace:
		var exists bool
		if err := d.store.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = $1)`, r.ID,
		).Scan(&exists); err != nil || !exists {
			return "", false
		}
		return r.ID, true
	case teamauth.ResourceOrg:
		var exists bool
		if err := d.store.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM workspaces WHERE org_id = $1)`, r.ID,
		).Scan(&exists); err != nil || !exists {
			return "", false
		}
		return r.ID, true
	case teamauth.ResourceJob:
		var ws string
		if err := d.store.pool.QueryRow(ctx,
			`SELECT workspace_id FROM jobs WHERE id = $1`, r.ID,
		).Scan(&ws); err != nil {
			return "", false
		}
		return ws, true
	case teamauth.ResourceObject:
		rows, err := d.store.pool.Query(ctx,
			`SELECT workspace_id FROM object_refs WHERE digest = $1`, r.ID)
		if err != nil {
			return "", false
		}
		defer rows.Close()
		ws, count := "", 0
		for rows.Next() {
			if err := rows.Scan(&ws); err != nil {
				return "", false
			}
			count++
			if count > 1 {
				break // ambiguous tenancy — fail closed below
			}
		}
		if err := rows.Err(); err != nil || count != 1 {
			return "", false
		}
		return ws, true
	default:
		// Sessions, knowledge, artifacts, secret handles, connections and
		// runs have no remote v1 table: absent, deny.
		return "", false
	}
}

// ResourceOwner resolves the owning principal for the matrix's own-only
// cells (own secret handles, connections, sessions). Schema v1 carries no
// remote table for those planes, so every resolution is ABSENT — the
// own-only matrix cells and the worker's submitter-scoped secret path
// stay fail-closed DENY until the owning surface lands with its rows
// (secret handles stay local per the accepted D-37 posture).
func (d *StoreDirectory) ResourceOwner(security.Resource) (string, bool) {
	return "", false
}

// OrganizationOwner resolves the org owner for Owner-only org actions.
// Schema v1 has no org authority table (workspaces carry org_id only), so
// resolution is ABSENT — org.delete/org.transfer deny until the org
// administration surface lands (D-36 administration; C4-33 org model).
func (d *StoreDirectory) OrganizationOwner(string) (string, bool) {
	return "", false
}

// workerJobCapabilities is the accepted operation-to-policy map's worker
// row (auth spec §2): a claimed Job's worker may read the Job, cancel it,
// read tenant-scoped object references, and resolve the Job submitter's
// own secret handles at the call boundary — nothing else. The set is
// fixed by the accepted map; claiming never widens it. object.create
// lands with the object-store surface (its vendor profile is an owner
// item), not here.
var workerJobCapabilities = []security.Action{
	teamauth.ActionJobRead,
	teamauth.ActionJobCancel,
	teamauth.ActionSecretExecute,
	teamauth.ActionObjectRead,
}

// ActiveDelegation resolves the service worker's CURRENT claimed
// Job-scoped grant from the live lease join — attempts still in
// claimed/running whose lease the SERVER clock has not expired (the same
// liveness rule the atomic claim uses, spec §2/§3). Absence covers
// revocation-before-claim, mid-run revocation, retry-after-revocation and
// lease expiry alike. Two accepted rules are part of the resolution:
// the grant survives only while the Job's SUBMITTER membership is
// currently active (auth spec §4 revocation table — a revoked submitter's
// Job loses its delegation at the next decision and never restarts work),
// and a worker holding MORE THAN ONE live lease is outside the
// one-grant invariant and resolves absent (fail closed).
func (d *StoreDirectory) ActiveDelegation(workerPrincipalID string) (teamauth.DelegatedGrant, bool) {
	if d.store == nil || !boundedID(workerPrincipalID) {
		return teamauth.DelegatedGrant{}, false
	}
	rows, err := d.store.pool.Query(context.Background(), `
		SELECT a.job_id, j.workspace_id, j.submitter_id
		FROM attempts a
		JOIN leases l ON l.attempt_id = a.id
		JOIN jobs j ON j.id = a.job_id
		JOIN memberships m
		     ON m.principal_id = j.submitter_id AND m.workspace_id = j.workspace_id
		WHERE a.worker_id = $1 AND a.state IN ('claimed','running')
		  AND m.state = 'active'
		  AND l.expires_at > clock_timestamp()`, workerPrincipalID)
	if err != nil {
		return teamauth.DelegatedGrant{}, false
	}
	defer rows.Close()
	var jobID, ws, submitter string
	count := 0
	for rows.Next() {
		if err := rows.Scan(&jobID, &ws, &submitter); err != nil {
			return teamauth.DelegatedGrant{}, false
		}
		count++
		if count > 1 {
			break // multiple live leases — fail closed below
		}
	}
	if err := rows.Err(); err != nil || count != 1 {
		return teamauth.DelegatedGrant{}, false
	}
	return teamauth.DelegatedGrant{
		JobID:        jobID,
		WorkspaceID:  ws,
		SubmitterID:  submitter,
		Capabilities: workerJobCapabilities,
	}, true
}
