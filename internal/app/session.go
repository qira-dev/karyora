package app

import (
	"context"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The C-4 session/credential commands (IMPL-23): the wire transport for
// the revocation machinery the coded SessionService has carried since its
// own milestone — the adopted P-S3-03 candidate's session/credential half
// and the S9-adopted P-S9-02 command classes (the S4 §19 F11 freeze act's
// frozen content). The coded classes are served VERBATIM — nothing here
// re-implements revocation:
//
//   - the per-token logout class → security.SessionService.Revoke (the
//     coded explicit logout path, C2-14 §3);
//   - the credential-change class → security.SessionService.RevokeAll
//     (the coded lock-takeover + credential-change revocation, C2-08);
//   - the Core/data-dir switch posture needs NO command at all: the
//     binding check refuses any token outside its own instance and data
//     directory (SESSION_INVALID) by construction on every decision —
//     this command surface changes nothing about it.
//
// A revocation is a WRITE, so each command decides through the one
// decision port with the coded ActionDataWrite posture every command
// decides through. The typed resource is honest per the coded registers:
// the session machine is instance-scoped security state (the binding
// check ties every session to its own instance and data directory), so
// the decision acts on ResourceCore — the coded instance-surface type —
// with no id (the instance itself, never a named record). The
// operation-specific audit record is written fail-closed BEFORE the
// effect (the coded command pattern), so a revocation can never execute
// unrecorded (C2-15).
//
// The failure law binds every command (P1.8 §6.2; the S-5 expected
// behavior): a refusal is rendered truthfully by the caller — the client
// NEVER assumes the session died because the command failed. Only a
// confirmed allow + effect ends the session; a dead or absent credential
// refuses at the decision port with the typed session reason (the coded
// register: SESSION_REVOKED / SESSION_EXPIRED / SESSION_INVALID) and the
// effect does not run.

// SessionLogoutResult reports the per-token logout command's outcome.
// The shape carries the revocation confirmation only — no session
// material, no distinguishing metadata beyond the fact of the
// revocation.
type SessionLogoutResult struct {
	Revoked bool `json:"revoked"`
}

// SessionRevokeAllResult reports the credential-change revocation's
// outcome: the live-session count AFTER the effect — the coded
// diagnostics class (counts only, never token material; the store's own
// bounded in-memory posture). Every live session of this instance is
// invalidated by the effect, so the count reports the honest post-state.
type SessionRevokeAllResult struct {
	LiveSessions int `json:"live_sessions"`
}

// SessionLogout is the C-4 per-token logout command (the adopted
// P-S3-03 candidate point 1; the P-S9-02 per-token logout class): the
// dedicated revocation flow's Core command — never an ordinary
// preference control (R-13); the UI surface is Settings → Security. The
// presented session credential (the caller context's own) is the token
// revoked: the decision port validates it first, the operation-specific
// audit record is written fail-closed BEFORE the effect, and only then
// does the coded per-token Revoke run (C2-14 §3's explicit logout path).
//
// Idempotency/replay posture, typed honestly per the recorded laws: the
// EFFECT is idempotent at the Core (re-revoking an already-revoked token
// is the store's no-op), but the WIRE surface's refusal class stays the
// typed AUTHZ_DENIED envelope for a dead credential (the coded session
// reason register) — a second logout with the same token refuses rather
// than reporting a fresh success. No auto-retry exists anywhere on this
// path (the Open-idempotency class law, P1.6 §4 rule 2): a lost response
// is the S4 unknown — the client holds it and re-derives state from
// authoritative outcomes, never a re-issue. The canonical wire name
// joins the published vocabulary at the publication act (O-1 stays
// Open; no contracts/ edit).
func (s *Slice) SessionLogout(ctx context.Context) (SessionLogoutResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Sessions == nil {
		return SessionLogoutResult{}, envelopeErr(errs.CodeStorageUnavailable, corr,
			map[string]any{"reason": "session service is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceCore}, security.ActionDataWrite)
	if err != nil {
		return SessionLogoutResult{}, err
	}
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "session.logout",
		Scope:           (security.Resource{Type: security.ResourceCore}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		Outcome:         "logout",
		CorrelationID:   corr,
	}); err != nil {
		return SessionLogoutResult{}, err
	}
	// The coded per-token logout path (C2-14 §3) — served, never
	// re-implemented. The token revoked is the caller context's own
	// presented credential.
	s.deps.Sessions.Revoke(credential(ctx))
	return SessionLogoutResult{Revoked: true}, nil
}

// SessionRevokeAll is the C-4 credential-change revocation command (the
// adopted P-S3-03 candidate point 2; the P-S9-02 credential-change
// class): the coded RevokeAll served as a decision-ported, audited
// command — every live session of this instance is invalidated (C2-08
// lock takeover and credential-change revocation), so the next decision
// for any held token denies with the typed reason code (SESSION_REVOKED,
// the coded register). The caller's own presented session is among the
// invalidated ones: the decision port validates it first, the audited
// record is written fail-closed BEFORE the effect, and the post-effect
// live count rides the result as the honest safe-metadata evidence.
//
// No retention value, no idempotency-key class, and no tenant surface
// exists anywhere on this command — each stays its recorded owner's item
// (the S9 record §5; the tenant half behind D-36/the Team gates). No
// auto-retry (the Open-idempotency law); a lost response is the S4
// unknown, never a re-issue.
func (s *Slice) SessionRevokeAll(ctx context.Context) (SessionRevokeAllResult, error) {
	corr := errs.NewCorrelationID()
	if s.deps.Sessions == nil {
		return SessionRevokeAllResult{}, envelopeErr(errs.CodeStorageUnavailable, corr,
			map[string]any{"reason": "session service is not wired"})
	}
	decision, err := s.authorizeCapture(ctx, corr, security.Resource{Type: security.ResourceCore}, security.ActionDataWrite)
	if err != nil {
		return SessionRevokeAllResult{}, err
	}
	actor := string(security.PrincipalLocalUser) + ":" + principalID(ctx)
	if err := s.deps.Guard.Audit.Record(security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           actor,
		Action:          "session.revoke_all",
		Scope:           (security.Resource{Type: security.ResourceCore}).String(),
		PolicyDecision:  "allow",
		PolicyReference: decision.ReasonCode,
		Outcome:         "revoke_all",
		CorrelationID:   corr,
	}); err != nil {
		return SessionRevokeAllResult{}, err
	}
	s.deps.Sessions.RevokeAll()
	return SessionRevokeAllResult{LiveSessions: s.deps.Sessions.LiveSessions()}, nil
}
