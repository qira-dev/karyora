package security

import (
	"errors"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// PrincipalKind is the stable principal class of one actor (C2-14 §1).
// service-worker and plugin are declared now and carry no grants in
// Phase 2: workers arrive with durable Jobs in Phase 3, plugins behind
// D-14. Declaring them keeps the port shape stable without inventing
// grants.
type PrincipalKind string

const (
	// PrincipalLocalUser is the single OS user running the Core — the only
	// human principal in local mode.
	PrincipalLocalUser PrincipalKind = "local-user"
	// PrincipalServiceWorker is reserved for Phase 3 durable-Job workers.
	PrincipalServiceWorker PrincipalKind = "service-worker"
	// PrincipalPlugin is reserved behind D-14.
	PrincipalPlugin PrincipalKind = "plugin"
)

// Principal is one authenticated actor.
type Principal struct {
	Kind PrincipalKind
	// ID is the stable, bounded principal identifier (e.g. the local
	// session's token-hash prefix). It is an identifier only — never a
	// credential value.
	ID string
}

func (p Principal) validate() error {
	if p.ID == "" || len(p.ID) > maxIDLen {
		return errors.New("security: principal id is missing or unbounded")
	}
	switch p.Kind {
	case PrincipalLocalUser, PrincipalServiceWorker, PrincipalPlugin:
		return nil
	default:
		return errors.New("security: unknown principal kind")
	}
}

// Resource identifies the protected target as `type:id`. The type set is
// the closed Phase 2 local scope; unknown types deny (fail closed).
type Resource struct {
	Type string
	ID   string
}

func (r Resource) String() string {
	if r.ID == "" {
		return r.Type
	}
	return r.Type + ":" + r.ID
}

// Resource types of the Phase 2 local slice (C2-14 §2 "in-scope local
// resources").
const (
	ResourceCore        = "core"        // lifecycle surface (status/stop)
	ResourceConfig      = "config"      // validated configuration reads
	ResourceStorage     = "storage"     // SQLite/CAS/journal surfaces
	ResourceAudit       = "audit"       // audit records and verification
	ResourceDiagnostics = "diagnostics" // diagnostic logs and exports
	ResourceBackup      = "backup"      // backup snapshots (C2-16)
	ResourceRestore     = "restore"     // controlled restore (C2-17)
	ResourceSecret      = "secret"      // secret handle administration
)

var localResourceTypes = map[string]bool{
	ResourceCore:        true,
	ResourceConfig:      true,
	ResourceStorage:     true,
	ResourceAudit:       true,
	ResourceDiagnostics: true,
	ResourceBackup:      true,
	ResourceRestore:     true,
	ResourceSecret:      true,
}

func (r Resource) validate() error {
	if !localResourceTypes[r.Type] {
		return errors.New("security: resource type is outside the local scope")
	}
	if len(r.ID) > maxIDLen {
		return errors.New("security: resource id is unbounded")
	}
	return nil
}

// Action is one bounded operation identifier (`domain.verb`). The required
// operation-class scope derives from the action table below.
type Action string

// Actions of the Phase 2 local slice.
const (
	ActionStatusRead    Action = "status.read"    // lifecycle/status surface
	ActionLifecycleStop Action = "lifecycle.stop" // graceful stop
	ActionLifecycleView Action = "lifecycle.view" // doctor/diagnostics reads
	ActionConfigRead    Action = "config.read"
	ActionDataRead      Action = "data.read"
	ActionDataWrite     Action = "data.write"
	ActionBackupCreate  Action = "backup.create"
	ActionRestoreExec   Action = "restore.execute"
	ActionAuditRead     Action = "audit.read"
	ActionSecretAdmin   Action = "secret.admin"
)

// requiredScope maps every action to the operation-class scope its session
// must carry (C2-14 §3: tokens carry an operation-class scope —
// lifecycle/status vs data operations).
var requiredScope = map[Action]Scope{
	ActionStatusRead:    ScopeLifecycle,
	ActionLifecycleStop: ScopeLifecycle,
	ActionLifecycleView: ScopeLifecycle,
	ActionConfigRead:    ScopeLifecycle,
	ActionDataRead:      ScopeData,
	ActionDataWrite:     ScopeData,
	ActionBackupCreate:  ScopeData,
	ActionRestoreExec:   ScopeData,
	ActionAuditRead:     ScopeData,
	ActionSecretAdmin:   ScopeData,
}

func (a Action) validate() error {
	if _, ok := requiredScope[a]; !ok {
		return errors.New("security: unknown action")
	}
	return nil
}

// StateChanging reports whether the action can change Core state. The
// audit fail-closed rule (C2-15 §5) denies a state-changing effect whose
// audit record cannot be written; read-only actions keep reads available
// and surface the audit failure through diagnostics instead.
func (a Action) StateChanging() bool {
	switch a {
	case ActionLifecycleStop, ActionDataWrite, ActionBackupCreate,
		ActionRestoreExec, ActionSecretAdmin:
		return true
	default:
		return false
	}
}

// Decision is the outcome of the one decision port: allow|deny plus a
// stable, bounded English reason code. Reasons never carry prose, secrets,
// or vendor detail.
type Decision struct {
	Allow      bool
	ReasonCode string
}

// Stable decision reason codes.
const (
	ReasonOK                 = "OK"
	ReasonDenyDefault        = "DENY_DEFAULT"
	ReasonUnknownPrincipal   = "UNKNOWN_PRINCIPAL"
	ReasonMalformedContext   = "MALFORMED_CONTEXT"
	ReasonPolicyUnavailable  = "POLICY_UNAVAILABLE"
	ReasonSessionInvalid     = "SESSION_INVALID"
	ReasonSessionExpired     = "SESSION_EXPIRED"
	ReasonSessionRevoked     = "SESSION_REVOKED"
	ReasonScopeMismatch      = "SCOPE_MISMATCH"
	ReasonResourceOutOfScope = "RESOURCE_OUT_OF_SCOPE"
	ReasonAuditUnavailable   = "AUDIT_UNAVAILABLE"
)

func allow() Decision { return Decision{Allow: true, ReasonCode: ReasonOK} }

func deny(code string) Decision { return Decision{Allow: false, ReasonCode: code} }

// DeniedEnvelope builds the machine error for a denial (AUTHZ_DENIED with
// bounded typed data: the reason code, resource, and action — identifiers
// only).
func (d Decision) DeniedEnvelope(correlationID string, r Resource, a Action) (errs.Envelope, error) {
	return errs.New(errs.CodeAuthzDenied, correlationID, map[string]any{
		"reason":   d.ReasonCode,
		"resource": boundedString(r.String(), maxIDLen*2+1),
		"action":   boundedString(string(a), maxIDLen),
	})
}

// maxIDLen bounds every identifier crossing this package.
const maxIDLen = 128

func boundedString(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

// PolicyContext carries the request-scoped authorization inputs. It holds
// identifiers and the presented credential reference only — never secret
// values or raw content.
type PolicyContext struct {
	// CorrelationID threads the decision into the audit record (C2-14 §6).
	CorrelationID string
	// Credential is the presented credential REFERENCE: a session token
	// string, or the CredentialIPCCapability sentinel when the platform IPC
	// capability authentication (C2-08 §5) already proved the same-OS-user
	// caller. It is validated, never logged.
	Credential string
	// Origin is the validated request origin for surfaces that have one
	// (local HTTP); empty for CLI/IPC.
	Origin string
}

// CredentialIPCCapability is the credential-class sentinel for the local
// IPC lifecycle surface: the capability token authentication in
// platform.ServeIPC is the credential proof; the handler-side decision port
// applies the principal/scope/resource matrix and the audit record on top.
// It is accepted only for lifecycle-scope actions on the core resource —
// never for data operations.
const CredentialIPCCapability = "ipc-capability"

func (c PolicyContext) validate() error {
	if c.CorrelationID == "" || len(c.CorrelationID) > maxIDLen {
		return errors.New("security: policy context lacks a correlation id")
	}
	if len(c.Credential) > 4096 {
		return errors.New("security: credential reference is unbounded")
	}
	if len(c.Origin) > 253 {
		return errors.New("security: origin is unbounded")
	}
	return nil
}

// PolicySnapshot is the opaque policy reference an asynchronous submitting
// context carries (C2-14 §2): each privileged step re-checks revocable
// permissions through the port before its effect. Phase 2 declares the
// type; resolution lands with durable Jobs (Phase 3) — submitting never
// mints a broader or permanent grant.
type PolicySnapshot struct {
	Ref string
}

func (s PolicySnapshot) Valid() bool {
	return s.Ref != "" && len(s.Ref) <= maxIDLen && !strings.ContainsAny(s.Ref, " \t\n")
}
