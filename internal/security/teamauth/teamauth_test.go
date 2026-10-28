package teamauth

import (
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/security"
)

// Decision fixtures for the accepted D-36 matrix and the C4-31 revocation
// table: every allowed AND denied side is asserted (workstream rule 5:
// prove both cases for any implemented Team capability). Synthetic
// principals only.

var testCtx = security.PolicyContext{CorrelationID: "test-corr"}

type fakeMember struct {
	role  Role
	state MemberState
}

type fakeDirectory struct {
	members  map[string]map[string]fakeMember // principal → tenant → membership
	tenants  map[string]string                // "type:id" → tenant
	owners   map[string]string                // "type:id" → owning principal
	orgOwner map[string]string                // org id → owner
	grants   map[string]DelegatedGrant        // worker → current grant
}

func (f *fakeDirectory) RoleOf(pid, tenant string) (Role, MemberState, bool) {
	m, ok := f.members[pid][tenant]
	return m.role, m.state, ok
}

func (f *fakeDirectory) WorkspaceOfResource(r security.Resource) (string, bool) {
	t, ok := f.tenants[r.Type+":"+r.ID]
	return t, ok
}

func (f *fakeDirectory) ResourceOwner(r security.Resource) (string, bool) {
	o, ok := f.owners[r.Type+":"+r.ID]
	return o, ok
}

func (f *fakeDirectory) OrganizationOwner(org string) (string, bool) {
	o, ok := f.orgOwner[org]
	return o, ok
}

func (f *fakeDirectory) ActiveDelegation(w string) (DelegatedGrant, bool) {
	g, ok := f.grants[w]
	return g, ok
}

func testDir() *fakeDirectory {
	return &fakeDirectory{
		members: map[string]map[string]fakeMember{
			"alice": {"ws1": {RoleOwner, MemberActive}, "org1": {RoleOwner, MemberActive}, "ws2": {RoleOwner, MemberActive}},
			"bob":   {"ws1": {RoleAdmin, MemberActive}, "org1": {RoleAdmin, MemberActive}},
			"carol": {"ws1": {RoleMember, MemberActive}},
			"dave":  {"ws1": {RoleViewer, MemberActive}},
			"eve":   {"ws2": {RoleMember, MemberRevoked}},
		},
		tenants: map[string]string{
			"workspace:ws1": "ws1", "workspace:ws2": "ws2",
			"org:org1": "org1",
			"job:j1":   "ws1", "job:j2": "ws2", "job:j3": "ws1",
			"run:r1":            "ws1",
			"knowledgepack:kp1": "ws1", "artifact:a1": "ws1",
			"secret:s1": "ws1", "secret:s2": "ws1",
			"connection:c1": "ws1", "connection:c2": "ws1",
			"object:o1":     "ws1",
			"session:sess1": "ws1", "session:sess2": "ws1",
			"plugin:p1": "ws1",
		},
		owners: map[string]string{
			"secret:s1": "carol", "secret:s2": "alice",
			"connection:c1": "carol", "connection:c2": "alice",
			"session:sess1": "carol", "session:sess2": "alice",
		},
		orgOwner: map[string]string{"org1": "alice"},
		grants: map[string]DelegatedGrant{
			"w1": {JobID: "j1", WorkspaceID: "ws1", SubmitterID: "carol",
				Capabilities: []security.Action{ActionJobRead, ActionJobCancel, ActionSecretExecute, ActionObjectCreate}},
		},
	}
}

func decide(t *testing.T, dir Directory, kind security.PrincipalKind, actor, rtype, rid string, a security.Action) security.Decision {
	t.Helper()
	tp := &TeamPolicy{Directory: dir}
	return tp.Decide(security.Principal{Kind: kind, ID: actor}, security.Resource{Type: rtype, ID: rid}, a, testCtx)
}

func wantDecision(t *testing.T, got security.Decision, allow bool, reason string) {
	t.Helper()
	if got.Allow != allow || got.ReasonCode != reason {
		t.Fatalf("decision = allow:%v reason:%q, want allow:%v reason:%q", got.Allow, got.ReasonCode, allow, reason)
	}
}

func TestMatrixAllowDenySweep(t *testing.T) {
	dir := testDir()
	team := security.PrincipalTeamUser
	cases := []struct {
		name       string
		actor      string
		rtype, rid string
		action     security.Action
		allow      bool
		reason     string
	}{
		{"owner creates job", "alice", ResourceWorkspace, "ws1", ActionJobCreate, true, security.ReasonOK},
		{"member creates job", "carol", ResourceWorkspace, "ws1", ActionJobCreate, true, security.ReasonOK},
		{"viewer cannot create job", "dave", ResourceWorkspace, "ws1", ActionJobCreate, false, ReasonRoleForbidden},
		{"viewer reads job", "dave", ResourceJob, "j1", ActionJobRead, true, security.ReasonOK},
		{"member cancels job", "carol", ResourceJob, "j1", ActionJobCancel, true, security.ReasonOK},
		{"viewer cannot cancel job", "dave", ResourceJob, "j1", ActionJobCancel, false, ReasonRoleForbidden},
		{"viewer reads knowledge", "dave", ResourceKnowledge, "kp1", ActionKnowledgeRead, true, security.ReasonOK},
		{"viewer cannot create knowledge", "dave", ResourceWorkspace, "ws1", ActionKnowledgeCreate, false, ReasonRoleForbidden},
		{"admin reads any secret", "bob", ResourceSecret, "s1", ActionSecretRead, true, security.ReasonOK},
		{"member reads own secret", "carol", ResourceSecret, "s1", ActionSecretRead, true, security.ReasonOK},
		{"member cannot read foreign secret", "carol", ResourceSecret, "s2", ActionSecretRead, false, ReasonOwnershipRequired},
		{"member resolves own secret", "carol", ResourceSecret, "s1", ActionSecretExecute, true, security.ReasonOK},
		{"member cannot resolve foreign secret", "carol", ResourceSecret, "s2", ActionSecretExecute, false, ReasonOwnershipRequired},
		{"viewer never resolves secrets", "dave", ResourceSecret, "s1", ActionSecretExecute, false, ReasonRoleForbidden},
		{"member administers own secret", "carol", ResourceSecret, "s1", ActionSecretAdmin, true, security.ReasonOK},
		{"member administers own connection", "carol", ResourceConnection, "c1", ActionConnAdmin, true, security.ReasonOK},
		{"member cannot administer foreign connection", "carol", ResourceConnection, "c2", ActionConnAdmin, false, ReasonOwnershipRequired},
		{"member reads own connection", "carol", ResourceConnection, "c1", ActionConnRead, true, security.ReasonOK},
		{"admins administer workspace", "bob", ResourceWorkspace, "ws1", ActionWorkspaceAdmin, true, security.ReasonOK},
		{"member cannot administer workspace", "carol", ResourceWorkspace, "ws1", ActionWorkspaceAdmin, false, ReasonRoleForbidden},
		{"admin reads audit", "bob", ResourceWorkspace, "ws1", ActionAuditRead, true, security.ReasonOK},
		{"member cannot read audit", "carol", ResourceWorkspace, "ws1", ActionAuditRead, false, ReasonRoleForbidden},
		{"viewer creates session", "dave", ResourceWorkspace, "ws1", ActionSessionCreate, true, security.ReasonOK},
		{"member administers own session", "carol", ResourceSession, "sess1", ActionSessionAdmin, true, security.ReasonOK},
		{"admin cannot administer foreign session", "bob", ResourceSession, "sess1", ActionSessionAdmin, false, ReasonOwnershipRequired},
		{"member reads run", "carol", ResourceRun, "r1", ActionRunRead, true, security.ReasonOK},
		{"viewer cannot read run", "dave", ResourceRun, "r1", ActionRunRead, false, ReasonRoleForbidden},
		{"member reads object", "carol", ResourceObject, "o1", ActionObjectRead, true, security.ReasonOK},
		{"viewer cannot read object", "dave", ResourceObject, "o1", ActionObjectRead, false, ReasonRoleForbidden},
		{"org owner deletes org", "alice", ResourceOrg, "org1", ActionOrgDelete, true, security.ReasonOK},
		{"org admin cannot delete org", "bob", ResourceOrg, "org1", ActionOrgDelete, false, ReasonRoleForbidden},
		{"org admin administers org", "bob", ResourceOrg, "org1", ActionWorkspaceAdmin, true, security.ReasonOK},
		{"non-member of org denied", "carol", ResourceOrg, "org1", ActionWorkspaceRead, false, ReasonWorkspaceMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decide(t, dir, team, tc.actor, tc.rtype, tc.rid, tc.action)
			wantDecision(t, got, tc.allow, tc.reason)
		})
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	dir := testDir()
	team := security.PrincipalTeamUser
	// A member of ws1 has zero authority in ws2 — same role, other tenant.
	wantDecision(t, decide(t, dir, team, "carol", ResourceJob, "j2", ActionJobRead), false, ReasonWorkspaceMismatch)
	wantDecision(t, decide(t, dir, team, "carol", ResourceWorkspace, "ws2", ActionJobCreate), false, ReasonWorkspaceMismatch)
	// The owner of ws2 acts there normally.
	wantDecision(t, decide(t, dir, team, "alice", ResourceJob, "j2", ActionJobRead), true, security.ReasonOK)
}

func TestRevokedMembership(t *testing.T) {
	dir := testDir()
	team := security.PrincipalTeamUser
	wantDecision(t, decide(t, dir, team, "eve", ResourceJob, "j2", ActionJobRead), false, ReasonMembershipRevoked)
	wantDecision(t, decide(t, dir, team, "eve", ResourceWorkspace, "ws2", ActionJobCreate), false, ReasonMembershipRevoked)
}

func TestPluginHardDenied(t *testing.T) {
	dir := testDir()
	team := security.PrincipalTeamUser
	// No role reaches plugin operations while D-14 keeps execution disabled.
	for _, actor := range []string{"alice", "bob", "carol", "dave"} {
		wantDecision(t, decide(t, dir, team, actor, ResourcePlugin, "p1", ActionKnowledgeRead), false, ReasonPluginDisabled)
	}
	// A plugin principal is denied everywhere, before membership logic.
	wantDecision(t, decide(t, dir, security.PrincipalPlugin, "p1", ResourceJob, "j1", ActionJobRead), false, ReasonPluginDisabled)
}

func TestFailClosedPaths(t *testing.T) {
	// Nil directory and nil policy deny everything.
	wantDecision(t, (&TeamPolicy{}).Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: "alice"},
		security.Resource{Type: ResourceJob, ID: "j1"}, ActionJobRead, testCtx), false, security.ReasonPolicyUnavailable)
	var nilPolicy *TeamPolicy
	wantDecision(t, nilPolicy.Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: "alice"},
		security.Resource{Type: ResourceJob, ID: "j1"}, ActionJobRead, testCtx), false, security.ReasonPolicyUnavailable)

	dir := testDir()
	// A local principal is unknown in team mode — the adapters never cross.
	wantDecision(t, decide(t, dir, security.PrincipalLocalUser, "osuser", ResourceJob, "j1", ActionJobRead), false, security.ReasonUnknownPrincipal)
	// Unknown action, unknown resource type, malformed context, unbounded ids.
	wantDecision(t, decide(t, dir, security.PrincipalTeamUser, "alice", ResourceJob, "j1", "widget.make"), false, security.ReasonDenyDefault)
	wantDecision(t, decide(t, dir, security.PrincipalTeamUser, "alice", "widget", "w1", ActionJobRead), false, security.ReasonResourceOutOfScope)
	tp := &TeamPolicy{Directory: dir}
	wantDecision(t, tp.Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: "alice"},
		security.Resource{Type: ResourceJob, ID: "j1"}, ActionJobRead, security.PolicyContext{}), false, security.ReasonMalformedContext)
	wantDecision(t, tp.Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: "alice"},
		security.Resource{Type: ResourceJob, ID: strings.Repeat("x", maxIDLen+1)}, ActionJobRead, testCtx), false, security.ReasonResourceOutOfScope)
	wantDecision(t, tp.Decide(security.Principal{Kind: security.PrincipalTeamUser, ID: ""},
		security.Resource{Type: ResourceJob, ID: "j1"}, ActionJobRead, testCtx), false, security.ReasonUnknownPrincipal)
}

func TestDelegatedWorkerAndRevocationTable(t *testing.T) {
	dir := testDir()
	worker := security.PrincipalServiceWorker

	// The claimed worker decides through its current grant only.
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j1", ActionJobRead), true, security.ReasonOK)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j1", ActionJobCancel), true, security.ReasonOK)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceSecret, "s1", ActionSecretExecute), true, security.ReasonOK)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceObject, "o1", ActionObjectCreate), true, security.ReasonOK)

	// TM-DEL-04 — worker scope violations: another Job (same tenant and
	// cross-tenant), ungranted actions, and the human-only groups.
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j3", ActionJobRead), false, ReasonDelegationScopeInvalid)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j2", ActionJobRead), false, ReasonWorkspaceMismatch)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceSecret, "s1", ActionSecretAdmin), false, ReasonDelegationScopeInvalid)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceWorkspace, "ws1", ActionJobCreate), false, security.ReasonDenyDefault)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceWorkspace, "ws1", ActionAuditRead), false, security.ReasonDenyDefault)
	wantDecision(t, decide(t, dir, worker, "w1", ResourceWorkspace, "ws1", ActionSessionCreate), false, security.ReasonDenyDefault)
	// A job-scoped grant never reaches another member's secret handle,
	// even inside the same workspace.
	wantDecision(t, decide(t, dir, worker, "w1", ResourceSecret, "s2", ActionSecretExecute), false, ReasonDelegationScopeInvalid)
	// Cross-tenant targets deny even with a matching capability.
	wantDecision(t, decide(t, dir, worker, "w1", ResourceSecret, "s3", ActionSecretExecute), false, ReasonWorkspaceMismatch)

	// TM-DEL-01 — revocation before the next claim: the grant is gone.
	delete(dir.grants, "w1")
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j1", ActionJobRead), false, ReasonDelegationMissing)
	// TM-DEL-02 — mid-run revocation: the NEXT privileged step denies.
	// TM-DEL-03 — retry after revocation: still denied, never restarted.
	wantDecision(t, decide(t, dir, worker, "w1", ResourceJob, "j1", ActionJobCancel), false, ReasonDelegationMissing)
}

func TestDelegatedGrantValidation(t *testing.T) {
	dir := testDir()
	dir.grants["w2"] = DelegatedGrant{JobID: "j1", WorkspaceID: "ws1"} // no capabilities
	wantDecision(t, decide(t, dir, security.PrincipalServiceWorker, "w2", ResourceJob, "j1", ActionJobRead), false, ReasonDelegationScopeInvalid)
	dir.grants["w3"] = DelegatedGrant{JobID: "j1", WorkspaceID: "ws1", Capabilities: []security.Action{"widget.make"}}
	wantDecision(t, decide(t, dir, security.PrincipalServiceWorker, "w3", ResourceJob, "j1", ActionJobRead), false, ReasonDelegationScopeInvalid)
}
