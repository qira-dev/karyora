package remotestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/security/teamauth"
)

// Conformance fixtures for the identity sub-milestone (C4-30/C4-31/C4-32):
// the StoreDirectory resolves the teamauth.Directory CURRENT authority
// over the live membership/resource tables, and the TeamPolicy decisions
// carry the Tenant Isolation Matrix's negative rows that have a delivered
// remote plane (TI-02 job state, TI-07 journal scoping, TI-08 object
// tenancy, TI-11 plugin hard-deny, TI-12 secret fail-closed absence,
// TI-13 worker scopes, TI-14 audit read). Planes without a remote v1
// table stay Not Run (recorded in the matrix register). The DSN comes
// from REMOTESTORE_TEST_DSN; when unset the fixtures SKIP (Not Run for
// that run, never a pass).

func seedMembership(t *testing.T, s *Store, ws, principal, role, state string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO memberships (workspace_id, principal_id, role, state)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, principal_id)
		DO UPDATE SET role = EXCLUDED.role, state = EXCLUDED.state, updated_at = now()`,
		ws, principal, role, state); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

func seedObjectRef(t *testing.T, s *Store, ws, digest string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO object_refs (workspace_id, digest, size_bytes, state)
		VALUES ($1, $2, 128, 'referenced')
		ON CONFLICT (workspace_id, digest) DO NOTHING`, ws, digest); err != nil {
		t.Fatalf("seed object ref: %v", err)
	}
}

// team decides through the delivered adapter with the store-backed
// Directory — the exact composition a remote surface would wire.
func team(s *Store) *teamauth.TeamPolicy {
	return &teamauth.TeamPolicy{Directory: s.Directory()}
}

func decideMember(p *teamauth.TeamPolicy, actor, rtype, rid string, a security.Action) security.Decision {
	return p.Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: actor},
		security.Resource{Type: rtype, ID: rid}, a,
		security.PolicyContext{CorrelationID: "corr-" + actor})
}

func decideWorker(p *teamauth.TeamPolicy, actor, rtype, rid string, a security.Action) security.Decision {
	return p.Decide(security.Principal{Kind: security.PrincipalServiceWorker, ID: actor},
		security.Resource{Type: rtype, ID: rid}, a,
		security.PolicyContext{CorrelationID: "corr-" + actor})
}

func TestDirectoryRoleOfResolvesCurrentState(t *testing.T) {
	s := testStore(t)
	ws := uid("ws-role")
	seedWorkspace(t, s, ws)
	seedMembership(t, s, ws, "alice", "member", "active")

	role, state, ok := s.Directory().RoleOf("alice", ws)
	if !ok || role != teamauth.RoleMember || state != teamauth.MemberActive {
		t.Fatalf("RoleOf = (%v, %v, %v), want (member, active, true)", role, state, ok)
	}
	// CURRENT authority: the revocation is visible on the next call.
	seedMembership(t, s, ws, "alice", "member", "revoked")
	_, state, ok = s.Directory().RoleOf("alice", ws)
	if !ok || state != teamauth.MemberRevoked {
		t.Fatalf("revocation must be visible immediately, got state=%v ok=%v", state, ok)
	}
	// Absence in every not-a-member-here shape.
	if _, _, ok := s.Directory().RoleOf("stranger", ws); ok {
		t.Fatal("a non-member must resolve absent")
	}
	if _, _, ok := s.Directory().RoleOf("alice", uid("ws-nowhere")); ok {
		t.Fatal("a foreign workspace must resolve absent")
	}
	if _, _, ok := s.Directory().RoleOf("", ws); ok {
		t.Fatal("a malformed id must resolve absent")
	}
}

func TestMemberCrossWorkspaceJobDenied(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-mem-a"), uid("ws-mem-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedMembership(t, s, wsB, "mallory", "member", "active")
	seedJob(t, s, wsA, uid("j-mem-a"), "alice", "queued")
	seedJob(t, s, wsB, uid("j-mem-b"), "mallory", "queued")
	p := team(s)

	if d := decideMember(p, "alice", teamauth.ResourceJob, uid("j-mem-a"), teamauth.ActionJobRead); !d.Allow {
		t.Fatalf("a member must read their own workspace's job, got %s", d.ReasonCode)
	}
	// TI-02: the cross-tenant read denies with the typed code and carries
	// nothing of the other tenant (the decision is a reason code only).
	if d := decideMember(p, "alice", teamauth.ResourceJob, uid("j-mem-b"), teamauth.ActionJobRead); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("cross-workspace job read must deny WORKSPACE_MISMATCH, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	// The matrix side rides the same resolution: a Viewer reads, never cancels.
	seedMembership(t, s, wsA, "vera", "viewer", "active")
	if d := decideMember(p, "vera", teamauth.ResourceJob, uid("j-mem-a"), teamauth.ActionJobRead); !d.Allow {
		t.Fatalf("viewer job.read must allow, got %s", d.ReasonCode)
	}
	if d := decideMember(p, "vera", teamauth.ResourceJob, uid("j-mem-a"), teamauth.ActionJobCancel); d.Allow || d.ReasonCode != teamauth.ReasonRoleForbidden {
		t.Fatalf("viewer job.cancel must deny ROLE_FORBIDDEN, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
}

func TestClaimPathScopedToAuthorizedWorkspaces(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-claim-a"), uid("ws-claim-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedJob(t, s, wsA, uid("j-claim-a"), "alice", "queued")
	seedJob(t, s, wsB, uid("j-claim-b"), "mallory", "queued")

	// The worker's assignment lists ONLY workspace A: B's queued job is
	// unreachable by construction (TI-02 claim path).
	c, err := s.ClaimJob(context.Background(), uid("worker-a"), []string{wsA})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if c.WorkspaceID != wsA {
		t.Fatalf("claimed workspace = %s, want %s (B was never authorized)", c.WorkspaceID, wsA)
	}
	// The A job's fencing tuple can never land on B's job row.
	if err := s.CommitAttempt(context.Background(), AttemptCommit{
		JobID: uid("j-claim-b"), WorkspaceID: wsB, AttemptID: c.AttemptID,
		WorkerID: c.WorkerID, LeaseID: c.LeaseID, FencingEpoch: c.FencingEpoch,
		ExpectedJobVersion: 1, JobState: "succeeded",
		ReceiptID: uid("rc-iso-b"), ResultDigest: "sha256-iso", Outcome: "ok",
	}); !errors.Is(err, ErrStaleFencing) {
		t.Fatalf("a foreign job must refuse a foreign tuple, got %v", err)
	}
}

func TestWorkerDelegationScopesAndSubmitterRevocation(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-work-a"), uid("ws-work-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedMembership(t, s, wsB, "mallory", "member", "active")
	seedJob(t, s, wsA, uid("j-work"), "alice", "queued")
	seedJob(t, s, wsA, uid("j-other"), "alice", "queued") // same workspace, never claimed by this worker
	seedJob(t, s, wsB, uid("j-foreign"), "mallory", "queued")

	c, err := s.ClaimJob(context.Background(), uid("worker-iso"), []string{wsA})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	dir := s.Directory()
	grant, ok := dir.ActiveDelegation(uid("worker-iso"))
	if !ok || grant.JobID != c.JobID || grant.WorkspaceID != wsA || grant.SubmitterID != "alice" {
		t.Fatalf("ActiveDelegation = (%+v, %v), want the claimed job's grant", grant, ok)
	}
	p := team(s)
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceJob, c.JobID, teamauth.ActionJobRead); !d.Allow {
		t.Fatalf("the worker must read its own claimed job, got %s", d.ReasonCode)
	}
	// TI-13: outside the assignment denies at every layer.
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceJob, uid("j-foreign"), teamauth.ActionJobRead); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("a foreign workspace's job must deny WORKSPACE_MISMATCH, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceJob, uid("j-other"), teamauth.ActionJobRead); d.Allow || d.ReasonCode != teamauth.ReasonDelegationScopeInvalid {
		t.Fatalf("an unclaimed same-workspace job must deny DELEGATION_SCOPE_INVALID, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	// TI-11: plugin operations are hard-denied before anything else.
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourcePlugin, uid("pl"), teamauth.ActionObjectRead); d.Allow || d.ReasonCode != teamauth.ReasonPluginDisabled {
		t.Fatalf("plugin operations must deny PLUGIN_DISABLED, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	// TI-12: no remote secret handles exist in schema v1. The worker's
	// tenancy check runs FIRST (fail closed): a handle owned by no remote
	// table resolves no tenant, so the resolve denies before the
	// submitter-ownership check is even reached.
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceSecret, uid("handle"), teamauth.ActionSecretExecute); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("secret resolve must stay fail-closed absent, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	// Workers never reach administration.
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceWorkspace, wsA, teamauth.ActionAuditRead); d.Allow || d.ReasonCode != security.ReasonDenyDefault {
		t.Fatalf("worker audit read must deny DENY_DEFAULT, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}

	// The accepted revocation table (auth spec §4): revoking the
	// SUBMITTER kills the delegation at the next decision — mid-run.
	seedMembership(t, s, wsA, "alice", "member", "revoked")
	if _, ok := dir.ActiveDelegation(uid("worker-iso")); ok {
		t.Fatal("a revoked submitter's grant must resolve absent")
	}
	if d := decideWorker(p, uid("worker-iso"), teamauth.ResourceJob, c.JobID, teamauth.ActionJobRead); d.Allow {
		t.Fatal("the worker must lose the claimed job after submitter revocation")
	}
	// CURRENT authority: restoring the membership restores the resolution.
	seedMembership(t, s, wsA, "alice", "member", "active")
	if _, ok := dir.ActiveDelegation(uid("worker-iso")); !ok {
		t.Fatal("the grant must resolve again while the lease lives")
	}
}

func TestMultipleLiveLeasesFailClosed(t *testing.T) {
	s := testStore(t)
	ws := uid("ws-two")
	seedWorkspace(t, s, ws)
	seedMembership(t, s, ws, "alice", "member", "active")
	seedJob(t, s, ws, uid("j-two-a"), "alice", "queued")
	seedJob(t, s, ws, uid("j-two-b"), "alice", "queued")

	if _, err := s.ClaimJob(context.Background(), uid("worker-two"), []string{ws}); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := s.ClaimJob(context.Background(), uid("worker-two"), []string{ws}); err != nil {
		t.Fatalf("second claim: %v", err)
	}
	// Two live leases are outside the one-grant invariant: absent, deny.
	if _, ok := s.Directory().ActiveDelegation(uid("worker-two")); ok {
		t.Fatal("a worker holding two live leases must resolve absent")
	}
}

func TestActiveDelegationEndsWithLeaseExpiry(t *testing.T) {
	s := testStore(t)
	s.LeaseTTL = 5 * time.Millisecond // the SERVER clock decides, never the host
	ws := uid("ws-expire")
	seedWorkspace(t, s, ws)
	seedMembership(t, s, ws, "alice", "member", "active")
	seedJob(t, s, ws, uid("j-expire"), "alice", "queued")

	if _, err := s.ClaimJob(context.Background(), uid("worker-exp"), []string{ws}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, ok := s.Directory().ActiveDelegation(uid("worker-exp")); ok {
		t.Fatal("an expired lease must end the delegation")
	}
}

func TestEventJournalWorkspaceScoping(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-jrn-a"), uid("ws-jrn-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedMembership(t, s, wsB, "mallory", "member", "active")
	seedJob(t, s, wsA, uid("j-jrn-a"), "alice", "queued")
	seedJob(t, s, wsB, uid("j-jrn-b"), "mallory", "queued")

	// One claim takes one job: the first lands in A, the second (A's job
	// is running now) lands in B.
	if _, err := s.ClaimJob(context.Background(), uid("worker-jrn"), []string{wsA, wsB}); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if _, err := s.ClaimJob(context.Background(), uid("worker-jrn"), []string{wsA, wsB}); err != nil {
		t.Fatalf("claim b: %v", err)
	}
	// TI-07 storage level: the journal and the seq counter are
	// workspace-scoped — the same seq number in each workspace resolves
	// ONLY that workspace's event, so a cursor can never cross.
	var evtA, evtB string
	var seqA, seqB int64
	if err := s.pool.QueryRow(context.Background(),
		`SELECT seq, event_id FROM event_journal WHERE workspace_id = $1 ORDER BY seq LIMIT 1`, wsA,
	).Scan(&seqA, &evtA); err != nil {
		t.Fatalf("journal a: %v", err)
	}
	if err := s.pool.QueryRow(context.Background(),
		`SELECT seq, event_id FROM event_journal WHERE workspace_id = $1 ORDER BY seq LIMIT 1`, wsB,
	).Scan(&seqB, &evtB); err != nil {
		t.Fatalf("journal b: %v", err)
	}
	if seqA != 1 || seqB != 1 {
		t.Fatalf("per-workspace sequences must both start at 1, got %d/%d", seqA, seqB)
	}
	var cross int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM event_journal WHERE workspace_id = $1 AND event_id = $2`, wsA, evtB,
	).Scan(&cross); err != nil {
		t.Fatalf("cross check: %v", err)
	}
	if cross != 0 {
		t.Fatal("workspace A's journal must never carry workspace B's event")
	}
}

func TestObjectReferenceTenancyAndAmbiguity(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-obj-a"), uid("ws-obj-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedMembership(t, s, wsB, "mallory", "member", "active")
	digest := uid("sha256-deadbeef")
	seedObjectRef(t, s, wsA, digest)
	p := team(s)

	if d := decideMember(p, "alice", teamauth.ResourceObject, digest, teamauth.ActionObjectRead); !d.Allow {
		t.Fatalf("a member must read their workspace's object, got %s", d.ReasonCode)
	}
	// TI-08: the other tenant's object denies; the bare digest is never a
	// global key.
	if d := decideMember(p, "mallory", teamauth.ResourceObject, digest, teamauth.ActionObjectRead); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("a foreign workspace's object must deny WORKSPACE_MISMATCH, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	// The same digest landing in TWO workspaces makes the tenancy
	// ambiguous — every resolution fails closed until a scoped surface
	// carries the workspace with it.
	seedObjectRef(t, s, wsB, digest)
	if d := decideMember(p, "alice", teamauth.ResourceObject, digest, teamauth.ActionObjectRead); d.Allow {
		t.Fatal("an ambiguous digest must fail closed for every principal")
	}
	if d := decideMember(p, "mallory", teamauth.ResourceObject, digest, teamauth.ActionObjectRead); d.Allow {
		t.Fatal("an ambiguous digest must fail closed for the second tenant too")
	}
}

func TestWorkspaceAuditReadAdminOnly(t *testing.T) {
	s := testStore(t)
	wsA, wsB := uid("ws-aud-a"), uid("ws-aud-b")
	seedWorkspace(t, s, wsA)
	seedWorkspace(t, s, wsB)
	seedMembership(t, s, wsA, "dave", "admin", "active")
	seedMembership(t, s, wsA, "alice", "member", "active")
	seedMembership(t, s, wsB, "mallory", "admin", "active")
	p := team(s)

	if d := decideMember(p, "dave", teamauth.ResourceWorkspace, wsA, teamauth.ActionAuditRead); !d.Allow {
		t.Fatalf("a workspace admin must read the audit plane, got %s", d.ReasonCode)
	}
	// TI-14: the member role is forbidden, the foreign admin is a tenant
	// violation, and an unknown workspace is out of scope — never a leak.
	if d := decideMember(p, "alice", teamauth.ResourceWorkspace, wsA, teamauth.ActionAuditRead); d.Allow || d.ReasonCode != teamauth.ReasonRoleForbidden {
		t.Fatalf("member audit read must deny ROLE_FORBIDDEN, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	if d := decideMember(p, "mallory", teamauth.ResourceWorkspace, wsA, teamauth.ActionAuditRead); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("foreign admin audit read must deny WORKSPACE_MISMATCH, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	if d := decideMember(p, "dave", teamauth.ResourceWorkspace, uid("ws-nowhere"), teamauth.ActionAuditRead); d.Allow || d.ReasonCode != security.ReasonResourceOutOfScope {
		t.Fatalf("an unknown workspace must deny RESOURCE_OUT_OF_SCOPE, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
}

func TestOrgActionsFailClosedInSchemaV1(t *testing.T) {
	s := testStore(t)
	org, ws := uid("org-v1"), uid("ws-org")
	seedWorkspace(t, s, ws)
	// The workspace carries the org id; memberships stay workspace-scoped
	// and schema v1 has no org authority table.
	if _, err := s.pool.Exec(context.Background(),
		`UPDATE workspaces SET org_id = $1 WHERE id = $2`, org, ws); err != nil {
		t.Fatalf("set org: %v", err)
	}
	seedMembership(t, s, ws, "ownerann", "owner", "active")
	p := team(s)

	// The org tenancy resolves (a workspace carries it), but the
	// org-scoped membership does not exist — and OrganizationOwner is
	// absent. Double fail-closed: the Owner-only org actions deny.
	if d := decideMember(p, "ownerann", teamauth.ResourceOrg, org, teamauth.ActionOrgDelete); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("org.delete must fail closed in schema v1, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
	if d := decideMember(p, "ownerann", teamauth.ResourceOrg, org, teamauth.ActionOrgTransfer); d.Allow || d.ReasonCode != teamauth.ReasonWorkspaceMismatch {
		t.Fatalf("org.transfer must fail closed in schema v1, got allow=%v code=%s", d.Allow, d.ReasonCode)
	}
}
