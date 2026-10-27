// Package security owns the identity, authorization, secret, and audit
// contracts (C2-14/C2-15): the single AuthorizationPolicy decision port
// every protected use case must consult before its first effect, local
// sessions and one-time bootstrap credentials, credential/secret handle
// references resolved only through the (open, D-37) store boundary, the
// tamper-evident audit chain with periodic anchored checkpoints, the local
// HTTP control-surface predicates (loopback/Host/Origin/CSRF and the
// state-changing-GET prohibition), and bounded sanitized diagnostics with
// source redaction. Local mode is single-user and role-free: LocalPolicy
// is the only Phase 2 implementation slot, and Phase 4 Team role
// resolution (D-36) replaces it behind the same port. Secret values never
// cross this package's outputs — only opaque handles and safe metadata.
// The package imports internal/domain only.
package security
