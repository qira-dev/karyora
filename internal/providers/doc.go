// Package providers owns the provider-neutral capability model and the
// adapter surface (C3-17–C3-30): the neutral contract types live in
// internal/domain/provider, the wire codecs and HTTP machinery live here
// behind the Adapter interface. Every call passes the full gate set — plan
// validation, capability check (unknown means unsupported), the per-profile
// data-handling registry (unknown/stale profiles block protected data
// egress, fail closed), credential resolution through the security secret
// boundary with error-surface redaction, and the egress guard (https,
// dial-time address verification against prohibited ranges for remote
// cores, redirects refused, header policy, size caps, TLS floor, no
// proxy). The conformance harness runs recorded fixture cases per profile;
// live checks stay Not Run while D-37 is open — no credential persistence,
// no live provider calls, no real API keys anywhere.
package providers
