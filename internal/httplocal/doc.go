// Package httplocal is the Core's local HTTP transport adapter (C2-14 §5
// "where a transport exists"): it exposes the app operation surface
// (Submit/Status/Events/Result/Cancel), the journal replay read, and the
// C-1 bootstrap/session-token exchange over a loopback-bound HTTP server.
//
// The security package's boundary predicates (internal/security/httpboundary)
// are the authoritative contract for this adapter and are enforced on every
// request: the Host-header allowlist with the loopback-only empty default
// (the DNS-rebinding defense), the Origin allowlist with the empty-Origin
// denial, the state-changing-GET prohibition (enforced at route
// registration — no mutating GET handler exists), CSRF enforcement for
// browser-originated mutations, and the credential-surface rule. The
// authorization decision itself stays the single AuthorizationPolicy port
// (C2-14 §2): this middleware rejects early; the app slice's Guard decides
// and records before every effect.
//
// The wire shapes are the adopted code-extraction facts (the S1 record):
// the app operation payloads, the event Record field set, the opaque
// cursor, and the D-35 four-field error envelope exactly as coded. This
// package owns NO canonical wire name: no published contract carries these
// paths, and the canonical-transport question stays its recorded open item.
// The CSRF-token issuance mechanics and the production Host/Origin
// allowlist content stay their recorded open items (C-2): browser-originated
// mutations are refused fail-closed until those close, and no browser
// session behavior is promised. The bootstrap credential is issued only
// through the in-process trusted-local channel (this package's
// IssueBootstrapCredential), never over HTTP; the browser-bootstrap UX and
// launch-mode choice belong to the UI track (D-26/D-39).
package httplocal
