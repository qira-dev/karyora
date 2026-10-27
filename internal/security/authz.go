package security

// The one AuthorizationPolicy decision port (C2-14 §2): a single decision
// point with one implementation slot per mode. Every protected or
// state-changing Core use case calls Decide before its first effect,
// records the decision to the audit contract (C2-15), and denies by
// default whenever authorization cannot be established. Adapter
// middleware (CLI, local HTTP, IPC) may reject early for UX but is never
// the sole or authoritative check; handlers never duplicate role/policy
// logic.

// Policy is the decision port.
type Policy interface {
	Decide(p Principal, r Resource, a Action, ctx PolicyContext) Decision
}

// LocalPolicy is the Phase 2 local adapter: it allows the local user's
// valid session for in-scope local resources and nothing else. Phase 4
// Team/role resolution (D-36) replaces only this adapter behind the same
// port. Local mode stays role-free: no role names, no permission grants —
// only the operation-class scope check (C2-14 §3).
type LocalPolicy struct {
	// Sessions validates presented session tokens; nil denies everything
	// (fail closed — an unavailable session path is a denial).
	Sessions *SessionService
	// IPC capability-credential acceptance can be disabled (tests); the
	// production surface keeps it enabled for the lifecycle scope only.
	AcceptIPCCredential bool
}

// NewLocalPolicy builds the adapter with IPC capability-credential
// acceptance enabled.
func NewLocalPolicy(sessions *SessionService) *LocalPolicy {
	return &LocalPolicy{Sessions: sessions, AcceptIPCCredential: true}
}

// Decide applies the default-deny matrix. Fail-closed paths: unknown
// principal kind, malformed context, unavailable session path, invalid /
// expired / revoked credential, scope mismatch, out-of-scope resource.
func (lp *LocalPolicy) Decide(p Principal, r Resource, a Action, ctx PolicyContext) Decision {
	if err := p.validate(); err != nil || p.Kind != PrincipalLocalUser {
		return deny(ReasonUnknownPrincipal)
	}
	if err := ctx.validate(); err != nil {
		return deny(ReasonMalformedContext)
	}
	if err := r.validate(); err != nil {
		return deny(ReasonResourceOutOfScope)
	}
	if err := a.validate(); err != nil {
		return deny(ReasonDenyDefault)
	}
	required, ok := requiredScope[a]
	if !ok {
		return deny(ReasonDenyDefault)
	}
	// The IPC capability credential class is accepted only for the local
	// lifecycle surface: the capability authentication (C2-08 §5) proved
	// the same-OS-user caller; the port still applies the scope/resource
	// matrix and records the decision. Data operations never accept it.
	if ctx.Credential == CredentialIPCCapability {
		if lp.AcceptIPCCredential && required == ScopeLifecycle && r.Type == ResourceCore {
			return allow()
		}
		return deny(ReasonScopeMismatch)
	}
	if lp.Sessions == nil {
		return deny(ReasonPolicyUnavailable)
	}
	s, check := lp.Sessions.validateToken(ctx.Credential, required)
	switch check {
	case tokenValid:
		// Binding check: a token never grants authority outside its own
		// instance and data directory (C2-14 §3).
		if s.Instance != lp.Sessions.InstanceID || s.DataDir != lp.Sessions.DataDirID {
			return deny(ReasonSessionInvalid)
		}
		return allow()
	case tokenRevoked:
		return deny(ReasonSessionRevoked)
	case tokenExpired:
		return deny(ReasonSessionExpired)
	default:
		return deny(ReasonSessionInvalid)
	}
}

// Guard couples the decision port with the audit contract (C2-14 §6): the
// decision is recorded before the caller effects anything, and
// authorization and audit fail together rather than silently diverging
// (C2-15 §5: an allowed effect whose audit record cannot be written is
// denied — AUDIT_UNAVAILABLE).
type Guard struct {
	Policy Policy
	Audit  *AuditService
}

// Authorize decides, records the audit outcome, and enforces the audit
// fail-closed rule. The returned decision is authoritative; a non-nil
// error carries the audit failure alongside a deny.
func (g *Guard) Authorize(p Principal, r Resource, a Action, ctx PolicyContext) (Decision, error) {
	var d Decision
	if g.Policy == nil {
		d = deny(ReasonPolicyUnavailable)
	} else {
		d = g.Policy.Decide(p, r, a, ctx)
	}
	auditErr := g.recordDecision(p, r, a, ctx, d)
	if d.Allow && auditErr != nil {
		// The effect would execute unrecorded: deny (C2-15 §5).
		return deny(ReasonAuditUnavailable), auditErr
	}
	return d, auditErr
}

func (g *Guard) recordDecision(p Principal, r Resource, a Action, ctx PolicyContext, d Decision) error {
	if g.Audit == nil {
		// No audit sink wired: the authorization outcome cannot be recorded,
		// so an allow must not proceed (same fail-closed rule).
		if d.Allow {
			return errAuditUnavailable("audit sink is not wired")
		}
		return nil
	}
	outcome := "deny"
	if d.Allow {
		outcome = "allow"
	}
	rec := AuditRecord{
		SchemaVersion:   AuditSchemaVersion,
		Actor:           string(p.Kind) + ":" + p.ID,
		Action:          string(a),
		Scope:           r.String(),
		PolicyDecision:  mapDecision(d),
		PolicyReference: d.ReasonCode,
		Outcome:         outcome,
		CorrelationID:   ctx.CorrelationID,
	}
	return g.Audit.Record(rec)
}

func mapDecision(d Decision) string {
	if d.Allow {
		return "allow"
	}
	return "deny"
}
