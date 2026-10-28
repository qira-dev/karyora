// Package teamauth is the Team-mode authorization adapter (CR-4.4 C4-30):
// the D-36-accepted model v1 behind the ONE security.Policy port. It
// replaces only the adapter slot — the accepted C2-14 local adapter and
// Local mode stay byte-for-byte unchanged. Human members decide through
// the accepted role matrix scoped to their workspace; service workers
// decide ONLY through their current claimed Job delegation (submission
// mints no standing grant and every privileged step re-checks); plugins
// are hard-denied while D-14 keeps execution disabled.
//
// Status boundary (sub-milestone A of the CR-4.4 implementation): the
// adapter is real code with its decision tests executed (matrix allow/deny
// sweep, workspace isolation, membership revocation, own-only ownership,
// and the TM-DEL-01..04 delegated-revocation cases). It is NOT wired into
// any product surface and no remote session surface exists yet —
// authenticating the presented principal remains the remote session
// surface's responsibility (spec §3; TM-AUTH/TM-WEB stay planned), and the
// queued/running/retry RESOLUTION behaviors of the C4-31 revocation table
// belong to the relay integration that consumes these denials.
package teamauth

import (
	"errors"

	"github.com/qira-dev/karyora/internal/security"
)

// Role is the owner-accepted role vocabulary (D-36: names approved
// 2026-10-26; the matrix accepted 2026-10-28). A role attaches to
// workspace membership only — no role crosses workspaces.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// MemberState is the CURRENT membership state; only Active members decide.
// Revocation simply flips the state — the next decision denies.
type MemberState string

const (
	MemberActive  MemberState = "active"
	MemberRevoked MemberState = "revoked"
)

// Team resource types (closed set; anything else denies). Create-class
// actions target the workspace/org resource; read/execute/admin target the
// concrete resource.
const (
	ResourceWorkspace  = "workspace"
	ResourceOrg        = "org"
	ResourceSession    = "session"
	ResourceJob        = "job"
	ResourceRun        = "run"
	ResourceKnowledge  = "knowledgepack"
	ResourceArtifact   = "artifact"
	ResourceSecret     = "secret"
	ResourceConnection = "connection"
	ResourceObject     = "object"
	ResourcePlugin     = "plugin"
)

// Team actions (closed set; unknown denies).
const (
	ActionSessionCreate   security.Action = "session.create"
	ActionSessionRead     security.Action = "session.read"
	ActionSessionAdmin    security.Action = "session.admin"
	ActionJobCreate       security.Action = "job.create"
	ActionJobRead         security.Action = "job.read"
	ActionJobCancel       security.Action = "job.cancel"
	ActionRunRead         security.Action = "run.read"
	ActionKnowledgeCreate security.Action = "knowledge.create"
	ActionKnowledgeRead   security.Action = "knowledge.read"
	ActionSecretCreate    security.Action = "secret.create"
	ActionSecretRead      security.Action = "secret.read"
	ActionSecretExecute   security.Action = "secret.execute"
	ActionSecretAdmin     security.Action = "secret.admin"
	ActionConnCreate      security.Action = "connection.create"
	ActionConnRead        security.Action = "connection.read"
	ActionConnAdmin       security.Action = "connection.admin"
	ActionObjectCreate    security.Action = "object.create"
	ActionObjectRead      security.Action = "object.read"
	ActionWorkspaceAdmin  security.Action = "workspace.admin"
	ActionWorkspaceRead   security.Action = "workspace.read"
	ActionOrgDelete       security.Action = "org.delete"
	ActionOrgTransfer     security.Action = "org.transfer"
	ActionAuditRead       security.Action = "audit.read"
)

// Team-specific stable decision reason codes (bounded English, no prose).
const (
	ReasonMembershipRevoked      = "MEMBERSHIP_REVOKED"
	ReasonRoleForbidden          = "ROLE_FORBIDDEN"
	ReasonWorkspaceMismatch      = "WORKSPACE_MISMATCH"
	ReasonOwnershipRequired      = "OWNERSHIP_REQUIRED"
	ReasonDelegationMissing      = "DELEGATION_MISSING"
	ReasonDelegationScopeInvalid = "DELEGATION_SCOPE_INVALID"
	ReasonPluginDisabled         = "PLUGIN_DISABLED"
)

// maxIDLen bounds every identifier crossing this adapter (mirrors the
// security package's bound).
const maxIDLen = 128

func allow() security.Decision { return security.Decision{Allow: true, ReasonCode: security.ReasonOK} }

func deny(code string) security.Decision {
	return security.Decision{Allow: false, ReasonCode: code}
}

// DelegatedGrant is a worker's CURRENT claimed Job-scoped authority. It is
// resolved fresh on every decision — submission mints no standing grant,
// and revocation simply removes it (spec §4).
type DelegatedGrant struct {
	JobID       string
	WorkspaceID string
	// SubmitterID is the Job's submitter principal (spec §4 submission
	// identity record): a worker's secret resolves stay scoped to handles
	// the submitter owns.
	SubmitterID string
	// Capabilities are the Job-scoped actions this worker may perform for
	// the claimed Job (capability matching per the storage spec).
	Capabilities []security.Action
}

func (g DelegatedGrant) validate() error {
	if g.JobID == "" || len(g.JobID) > maxIDLen || g.WorkspaceID == "" || len(g.WorkspaceID) > maxIDLen ||
		g.SubmitterID == "" || len(g.SubmitterID) > maxIDLen {
		return errors.New("teamauth: delegation lacks a bounded job/workspace/submitter id")
	}
	if len(g.Capabilities) == 0 {
		return errors.New("teamauth: delegation carries no capabilities")
	}
	for _, c := range g.Capabilities {
		if !teamActionSet[c] {
			return errors.New("teamauth: delegation capability is outside the team action set")
		}
	}
	return nil
}

// Directory is the CURRENT-authority view the adapter decides against. It
// is a port: the durable remote store implements it (sub-milestone B/C);
// tests provide an in-memory fake. Every method resolves CURRENT state —
// a revocation is visible on the next call, never cached here.
type Directory interface {
	// RoleOf resolves the member's CURRENT role and state in one tenant
	// scope (a workspace, or the org for org-scoped resources). Absence
	// means "not a member here".
	RoleOf(principalID, tenantID string) (Role, MemberState, bool)
	// WorkspaceOfResource resolves the tenant (workspace id, or the org id
	// for org-scoped resources) that owns a protected resource.
	WorkspaceOfResource(r security.Resource) (string, bool)
	// ResourceOwner resolves the owning principal for own-only matrix
	// cells (own secret handles, connections, sessions).
	ResourceOwner(r security.Resource) (string, bool)
	// OrganizationOwner resolves the org owner for Owner-only org actions.
	OrganizationOwner(orgID string) (string, bool)
	// ActiveDelegation resolves a service worker's CURRENT claimed
	// Job-scoped grant; absence covers revocation-before-claim, mid-run
	// revocation and retry-after-revocation alike (spec §4).
	ActiveDelegation(workerPrincipalID string) (DelegatedGrant, bool)
}

// matrixCell encodes one accepted D-36 matrix cell: base roles decide
// outright; ownOnly roles decide only when they own the concrete resource
// ("own handles only" cells). Every cell stays subject to workspace
// scoping — enforced before the matrix is consulted.
type matrixCell struct {
	base    []Role
	ownOnly []Role
}

var (
	roleAll    = []Role{RoleOwner, RoleAdmin, RoleMember, RoleViewer}
	roleNoView = []Role{RoleOwner, RoleAdmin, RoleMember}
	roleAdmins = []Role{RoleOwner, RoleAdmin}
)

// teamMatrix encodes the accepted D-36 permission matrix (proposal §2.4)
// and the operation-to-policy map (spec §2). ResourcePlugin carries NO
// entries: plugin operations are denied before the matrix (D-14).
var teamMatrix = map[string]map[security.Action]matrixCell{
	ResourceWorkspace: {
		ActionSessionCreate:   {base: roleAll},
		ActionJobCreate:       {base: roleNoView},
		ActionKnowledgeCreate: {base: roleNoView},
		ActionObjectCreate:    {base: roleNoView},
		ActionSecretCreate:    {base: roleNoView},
		ActionConnCreate:      {base: roleNoView},
		ActionWorkspaceAdmin:  {base: roleAdmins},
		ActionWorkspaceRead:   {base: roleAdmins},
		ActionAuditRead:       {base: roleAdmins},
	},
	ResourceOrg: {
		ActionOrgDelete:      {base: []Role{RoleOwner}},
		ActionOrgTransfer:    {base: []Role{RoleOwner}},
		ActionWorkspaceAdmin: {base: roleAdmins},
		ActionWorkspaceRead:  {base: roleAdmins},
	},
	ResourceSession: {
		ActionSessionRead:  {ownOnly: roleAll},
		ActionSessionAdmin: {ownOnly: roleAll},
	},
	ResourceJob: {
		ActionJobRead:   {base: roleAll}, // Viewer read returns redacted metadata (surface duty)
		ActionJobCancel: {base: roleNoView},
	},
	ResourceRun: {
		ActionRunRead: {base: roleNoView},
	},
	ResourceKnowledge: {
		ActionKnowledgeRead: {base: roleAll},
	},
	ResourceArtifact: {
		ActionKnowledgeRead: {base: roleAll},
	},
	ResourceSecret: {
		ActionSecretRead:    {base: roleAdmins, ownOnly: []Role{RoleMember}},
		ActionSecretExecute: {base: roleAdmins, ownOnly: []Role{RoleMember}},
		ActionSecretAdmin:   {base: roleAdmins, ownOnly: []Role{RoleMember}},
	},
	ResourceConnection: {
		ActionConnRead:  {base: roleAdmins, ownOnly: []Role{RoleMember}},
		ActionConnAdmin: {base: roleAdmins, ownOnly: []Role{RoleMember}},
	},
	ResourceObject: {
		ActionObjectRead: {base: roleNoView},
	},
}

var teamResourceSet = func() map[string]bool {
	set := map[string]bool{}
	for t := range teamMatrix {
		set[t] = true
	}
	set[ResourcePlugin] = true
	return set
}()

var teamActionSet = func() map[security.Action]bool {
	set := map[security.Action]bool{}
	for _, perType := range teamMatrix {
		for a := range perType {
			set[a] = true
		}
	}
	return set
}()

// TeamPolicy is the Team adapter behind the one security.Policy port.
// Directory nil denies everything (fail closed).
type TeamPolicy struct {
	Directory Directory
}

var _ security.Policy = (*TeamPolicy)(nil)

// Decide applies the accepted matrix, deny-by-default at every step:
// unknown principal kind, malformed context, out-of-scope resource,
// missing membership, revoked membership, forbidden role, ownership
// failure, plugin operations, and any worker request outside its current
// delegation all deny. The presented principal is assumed AUTHENTICATED —
// credential validation belongs to the remote session surface (spec §3).
func (tp *TeamPolicy) Decide(p security.Principal, r security.Resource, a security.Action, ctx security.PolicyContext) security.Decision {
	if tp == nil || tp.Directory == nil {
		return deny(security.ReasonPolicyUnavailable)
	}
	if err := validatePrincipal(p); err != nil {
		return deny(security.ReasonUnknownPrincipal)
	}
	if err := validateContext(ctx); err != nil {
		return deny(security.ReasonMalformedContext)
	}
	if err := validateResource(r); err != nil {
		return deny(security.ReasonResourceOutOfScope)
	}
	if !teamActionSet[a] {
		return deny(security.ReasonDenyDefault)
	}
	// Plugins stay hard-denied while D-14 keeps execution disabled —
	// before any membership or matrix logic.
	if p.Kind == security.PrincipalPlugin || r.Type == ResourcePlugin {
		return deny(ReasonPluginDisabled)
	}
	// Service workers never act as members: they decide only through their
	// current claimed Job delegation (spec §4).
	if p.Kind == security.PrincipalServiceWorker {
		return tp.decideWorker(p, r, a)
	}
	return tp.decideMember(p, r, a)
}

func (tp *TeamPolicy) decideMember(p security.Principal, r security.Resource, a security.Action) security.Decision {
	tenant, ok := tp.Directory.WorkspaceOfResource(r)
	if !ok || tenant == "" {
		return deny(security.ReasonResourceOutOfScope)
	}
	role, state, ok := tp.Directory.RoleOf(p.ID, tenant)
	if !ok {
		return deny(ReasonWorkspaceMismatch)
	}
	if state != MemberActive {
		return deny(ReasonMembershipRevoked)
	}
	cell, ok := teamMatrix[r.Type][a]
	if !ok {
		return deny(security.ReasonDenyDefault)
	}
	for _, allowed := range cell.base {
		if allowed == role {
			return allow()
		}
	}
	for _, own := range cell.ownOnly {
		if own == role {
			owner, ok := tp.Directory.ResourceOwner(r)
			if !ok || owner != p.ID {
				return deny(ReasonOwnershipRequired)
			}
			return allow()
		}
	}
	return deny(ReasonRoleForbidden)
}

func (tp *TeamPolicy) decideWorker(p security.Principal, r security.Resource, a security.Action) security.Decision {
	grant, ok := tp.Directory.ActiveDelegation(p.ID)
	if !ok {
		// Covers revocation-before-claim, mid-run revocation and
		// retry-after-revocation alike: the current grant is gone.
		return deny(ReasonDelegationMissing)
	}
	if err := grant.validate(); err != nil {
		return deny(ReasonDelegationScopeInvalid)
	}
	// Tenant first: a cross-workspace target is a workspace violation
	// regardless of anything else.
	tenant, ok := tp.Directory.WorkspaceOfResource(r)
	if !ok || tenant != grant.WorkspaceID {
		return deny(ReasonWorkspaceMismatch)
	}
	switch r.Type {
	case ResourceJob:
		// A worker touches ONLY its claimed Job.
		if r.ID != grant.JobID {
			return deny(ReasonDelegationScopeInvalid)
		}
	case ResourceSecret:
		// A worker resolves ONLY handles the Job's submitter owns — the
		// grant is Job-scoped, never a workspace-wide grant.
		owner, ownerOK := tp.Directory.ResourceOwner(r)
		if !ownerOK || owner != grant.SubmitterID {
			return deny(ReasonDelegationScopeInvalid)
		}
	case ResourceObject:
		// Job-scoped call-boundary class: tenant match suffices.
	default:
		// Workers never reach administration, sessions, audits,
		// definitions or any other group.
		return deny(security.ReasonDenyDefault)
	}
	for _, c := range grant.Capabilities {
		if c == a {
			return allow()
		}
	}
	return deny(ReasonDelegationScopeInvalid)
}

func validatePrincipal(p security.Principal) error {
	if p.ID == "" || len(p.ID) > maxIDLen {
		return errors.New("teamauth: principal id is missing or unbounded")
	}
	switch p.Kind {
	case security.PrincipalTeamUser, security.PrincipalServiceWorker, security.PrincipalPlugin:
		return nil
	default:
		// A local principal is unknown in team mode — the adapters are
		// mode-specific and never cross.
		return errors.New("teamauth: principal kind is not a team principal")
	}
}

func validateContext(ctx security.PolicyContext) error {
	if ctx.CorrelationID == "" || len(ctx.CorrelationID) > maxIDLen {
		return errors.New("teamauth: policy context lacks a correlation id")
	}
	if len(ctx.Credential) > 4096 || len(ctx.Origin) > 253 {
		return errors.New("teamauth: policy context is unbounded")
	}
	return nil
}

func validateResource(r security.Resource) error {
	if !teamResourceSet[r.Type] {
		return errors.New("teamauth: resource type is outside the team scope")
	}
	if r.ID == "" || len(r.ID) > maxIDLen {
		return errors.New("teamauth: resource id is missing or unbounded")
	}
	return nil
}
