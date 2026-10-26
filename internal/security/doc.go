// Package security owns the identity, authorization, secret, and audit
// contracts: the single AuthorizationPolicy decision port every protected
// use case must consult, credential/secret handle abstraction, and
// tamper-evident audit metadata. It may import internal/domain only. Local
// mode is role-free; Team role resolution arrives behind the same port at
// the Phase 4 trigger (D-36).
package security
