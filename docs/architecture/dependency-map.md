# Core Dependency Map (Phase 2, C2-02)

Contract version 1.17 (2026-10-29): the CR-4.4 C4-38 implementation
milestone (the workload-scoped service-identity lifecycle, Remote
Operations Specification §1) lands inside `internal/remotestore` — schema
migration v2 (service-identity bindings + versioned credentials) and the
lifecycle store primitives with the claim/renew/commit boundary
verification. NO import-rule change: the store's allowed imports stay
`security` (the new code uses stdlib crypto only and the pgx import stays
confined to the package); the graph, its acyclicity, and every
confinement rule are unchanged. The lifecycle is NOT wired into any
product surface, remote mode does not exist, and Local mode keeps zero
dependency on the package (isolation rule).
Contract version 1.16 (2026-10-29): the CR-4.5 C4-41 PROVE-half milestone
(the owner-accepted D-38 device-key lifecycle) adds
`internal/security/devicekey` — the lifecycle state machine, the server
side registry semantics, and the key-state verification rules behind
typed denials. Rules: the package imports `internal/security` and the
`internal/domain` error register only; it defines the
`devicekey.PrivateKeySource` storage port, which
`internal/security/keyring` implements structurally (the go-keyring
import stays confined to keyring; the adapter→port direction means
keyring imports devicekey's port package and devicekey never imports
keyring — the graph stays acyclic and no forbidden direction opens). The
lifecycle is NOT wired into any product surface, no remote session
surface exists (the registration ceremony's credential validation stays
with TM-AUTH), the two-party transfer protocol stays D-42-gated and
unimplemented, and Local mode is byte-for-byte unchanged.
Contract version 1.15 (2026-10-29): the CR-4.4 identity sub-milestone
implements the `teamauth.Directory` port inside `internal/remotestore`
(`StoreDirectory`): the store now imports `internal/security` (the
`security.Resource`/`security.Action` port types) and
`internal/security/teamauth` (the Directory port and the role vocabulary
it resolves), replacing the "no internal imports in v1" rule for this
package. The direction stays adapter→port — teamauth never imports the
store; the graph remains acyclic and no forbidden direction opens. The
store is still NOT wired into any product surface, remote mode does not
exist, and Local mode keeps zero dependency on the package (isolation
rule); the object-store half stays an owner item (vendor selection open).
Contract version 1.14 (2026-10-28): the CR-4.4 sub-milestone B (D-10 pin)
adds `internal/remotestore` — the PostgreSQL remote/team authority adapter
on the pinned `github.com/jackc/pgx/v5` v5.11.0 (D-10 baseline; MIT; the
pgx import is confined here by the executable rule check, rule 4). The
package carries the spec §1 schema-area map (Work excluded), the C2-11
migration discipline, the atomic claim (SKIP LOCKED + monotonic fencing
epoch + server-clock lease inside one transaction), the fenced commit, and
the journal/outbox append. Rules: no internal imports in v1; the store is
NOT wired into any product surface and local mode has zero dependency on
it (isolation rule); remote mode does not exist yet. The teamauth.Directory
implementation over these tables lands with the identity sub-milestone. No
forbidden direction opened; the graph stays acyclic. Contract version 1.13 (2026-10-28): the CR-4.4 sub-milestone A (D-36
identity wiring) adds `internal/security/teamauth` — the Team-mode
authorization adapter implementing the owner-accepted D-36 model v1
(roles/matrix/delegation/revocation) behind the unchanged
`security.Policy` port, plus the new `security.PrincipalTeamUser`
principal kind declaration. Rules: the subpackage imports
`internal/security` only; the accepted C2-14 local adapter and Local mode
stay byte-for-byte unchanged (isolation rule); the adapter is NOT wired
into any product surface and no remote session surface exists yet. The
durable remote store (sub-milestone B/C) implements the teamauth.Directory
port. No forbidden direction opened; the graph stays acyclic. Contract version 1.12 (2026-10-28): the CR-4.3 C4-25 runtime-enforcement
milestone adds `internal/pluginhost` — the enforced plugin host adapter
that wraps the pinned WASM runtime (`github.com/tetratelabs/wazero`
v1.12.0, Apache-2.0) around the `internal/plugins` contract model so the
manifest quota envelope, per-call authorization, cancellation, and fault
containment are measured at a product boundary. Rules: the subpackage
imports `internal/plugins` only; the wazero import is confined to this
package by the executable rule check (rule 4: the WASM runtime only
behind plugin host adapters). Untrusted plugin execution stays DISABLED:
nothing outside the package's own fixtures instantiates a plugin, no
CLI/API surface reaches it, and the D-14 gate remains open
(cross-target runtime execution and the publisher-signature profile are
unproven; no instruction fuel claim is made). No forbidden direction
opened; the graph stays acyclic. Contract version 1.11 (2026-10-28): the Secret Store milestone (realizing the
accepted D-37 decision record of 2026-10-28) adds `internal/security/keyring`
— the concrete OS-keyring store (Windows Credential Manager, macOS Keychain,
Linux Secret Service, via `github.com/zalando/go-keyring`) behind the
unchanged `security.SecretStore` and the new `security.SecretWriter` ports.
Rules: the subpackage imports `internal/security` only; the keyring import is
confined to the subpackage by the executable rule check; `cmd/uawp` (the
composition root) wires it where the job slice previously hardcoded
`security.UnavailableStore{}` (the executor nil-default stays
`UnavailableStore` — fail closed). `internal/security` gains the lifecycle
boundary types with no new internal imports. No forbidden direction opened;
the graph stays acyclic. Contract version 1.10 (2026-10-27): the CR-3.5 (C3-31) implementation makes
`internal/app` real code — the durable vertical slice use cases (submit with
idempotency, status/events/result inspection, cooperative cancel, the relay
handler with retrieval → snapshot → capability-checked provider call →
atomic commit, and the bounded settlement loop) — under its unchanged rule
(imports domain, events, security, and the module packages only; every
adapter is an interface wired at cmd/uawp). `cmd/uawp` gains the `job`
command group (foreground submit session over the same SliceService plus
the read-only status/events probe). `internal/storage` gains the object /
journal-read / backoff-requeue adapter methods and the read-only job probe
(no import change); `internal/security` gains the closed `job` resource
type (no import change); `internal/providers` gains the ProfileFor/
Connection accessor methods on the existing Executor (no import change);
`internal/aijobrelay`'s scheduler now commits resolved outcomes on a live
context (defect found by CR-3.5 verification, no import change). No
forbidden direction opened; the graph stays acyclic. Contract version 1.9 (2026-10-27): the CR-3.4 (C3-17–C3-30) implementation
makes `internal/providers` real code — the wire codecs, the egress guard
(C3-18), the BYOK secret resolution with redaction, the per-profile
data-handling registry seeding (C3-19), the conformance harness (C3-20),
and the eight direct plus custom/local adapters (C3-21–C3-30) — under its
unchanged rule (imports `internal/domain` and `internal/security` only;
no provider SDKs, stdlib `net/http` only). Two pure domain subpackages
carry the neutral contract: `internal/domain/provider` (C3-17 request/
result/capability/error/connection types and the C3-19 records with the
fail-closed gate) and `internal/domain/jsonschema` (the bounded output-
schema validator used pre-commit; subpackages inherit the domain rule:
no internal imports). No forbidden direction opened; the graph stays
acyclic. Contract version 1.8 (2026-10-27): the CR-3.3 (C3-12–C3-16) implementation
makes `internal/aijobrelay` real code — the separate Job/Attempt/Lease/
Checkpoint/SideEffectIntent/Receipt state machines under the accepted D-34
transition package, fenced atomic claims and commits, and the bounded
scheduler — under its unchanged rule (imports `internal/domain` and
`internal/events` only). `internal/storage` exercises its map-allowed
`internal/aijobrelay` import through the relay adapter (migration v6:
`jobs`/`attempts`/`side_effect_intents`/`job_receipts`; no Work tables —
D-12/D-33 exclusion held). Contract version 1.7 (2026-10-27): the CR-3.2
(C3-08–C3-11) implementation makes `internal/contextengine` real code —
the policy-aware compilation pipeline, immutable ContextSnapshots, and the
scoped Memory lifecycle — under its unchanged rule (imports
`internal/domain` and `internal/events` only); the canonical JSON authority
moved to `internal/events` (events.CanonicalJSON) with knowledgepack
delegating to it; `internal/storage` exercises its map-allowed
`internal/contextengine` import through the snapshot/memory adapter
(migration v5: `context_snapshots`/`memory_records`). No forbidden
direction opened; the graph stays acyclic. Contract version 1.6 (2026-10-27): the CR-3.1 (C3-03–C3-07) implementation
makes `internal/knowledgepack` real code — the versioned pack manifest,
claims/evidence/provenance records, the D-41 internal digest/signature
profile, bounded untrusted import, verification/quarantine, and lexical
retrieval — under its unchanged rule (imports `internal/domain` and
`internal/events` only). `internal/storage` exercises its map-allowed
`internal/knowledgepack` import through the pack-store adapter
(migration v4: `kp_packs`/`kp_versions`/`kp_entries`), and `internal/config`
extends the closed key registry with the five Proposed knowledge
import/retrieval bounds. CAS reachability roots are now registered in the
Store constructor (was Reconcile-only), so the GC barrier holds for every
instance. No forbidden direction opened; the graph stays acyclic. Contract
version 1.5 (2026-10-27): the C2-16/C2-17 implementation adds the
backup/restore machinery to `internal/storage` (online snapshot capture,
manifest protocol, durable backup pins as CAS reachability roots, the
controlled staged restore with generation fencing) with no import change —
storage keeps `domain, events, platform` plus the pinned driver, and no new
package exists. Contract version 1.4 (2026-10-27): the C2-14/C2-15
implementation makes
`internal/security` real code — the AuthorizationPolicy decision port,
local sessions, secret handles, the tamper-evident audit chain, and
bounded diagnostics — under its unchanged rule (imports `internal/domain`
only). `internal/runtime` now exercises its map-allowed `internal/security`
import through the runner's authoritative IPC guard hook and doctor's audit
verification check. `internal/storage` gains audit persistence (ordered
durable rows, checkpoint anchors, the integrity-broken flag) with no import
change — the security-package port is adapted to those rows only by the
`cmd/uawp` composition root, which may import anything. `internal/config`
extends the closed key registry with the five audit/diagnostics bounds
(Proposed defaults), no import change. No forbidden direction opened; the
graph stays acyclic. Version 1
derived from Core Master Blueprint v0.9 §1–§2 and the Phase 1
authority/module boundary map; v1.1 added `internal/config`,
`internal/health`, and the domain `errs` subpackage. Substantive changes
require a version bump and an ADR.

## Rules

1. One Go module (`github.com/qira-dev/karyora`). Splitting into multiple modules
   requires scale evidence plus an ADR.
2. Dependency direction is inward-only: adapters depend on ports; ports and domain
   never depend on adapters. The graph must stay acyclic.
3. `internal/domain` imports nothing internal. `internal/app` and the module packages
   never import `internal/storage`, `internal/providers`, `internal/plugins`,
   `internal/platform`, database drivers, HTTP frameworks, provider SDKs, or
   OS-specific APIs.
4. Database drivers are imported only inside `internal/storage`; OS-specific APIs only
   inside `internal/platform`; provider SDKs only behind provider adapters; the WASM
   runtime only behind plugin host adapters (execution disabled until D-14 passes).
5. Every protected use case consults the single `AuthorizationPolicy` decision port
   (`internal/security`) before effects; middleware is early rejection only.
6. Canonical wire schemas live only in `/contracts`; `internal/contractadapter` holds
   derived runtime adapters and never forks a schema.

## Package ownership and allowed imports

| Package | Owns (source of truth) | Allowed internal imports |
|---|---|---|
| `internal/domain` | Canonical domain types and invariants | (none) |
| `internal/knowledgepack` | Knowledge truth: packs, claims, evidence, provenance (D-41 gated for portability) | domain, events |
| `internal/aijobrelay` | Execution truth: Job/Attempt/Lease/Checkpoint/side-effect/receipt/transfer machines (Phase 1 contract v1; transfer gated by D-38/D-42; Runs gated by D-40) | domain, events |
| `internal/contextengine` | Compiled context truth: pipeline + ContextSnapshot records | domain, events |
| `internal/workflow` | Workflow definitions and immutable graph revisions | domain, events |
| `internal/events` | Event record, journal/outbox semantics, opaque cursors | domain |
| `internal/security` | AuthorizationPolicy decision port, secret handles, audit metadata | domain |
| `internal/security/keyring` | Concrete OS-keyring secret store (D-37 accepted; keyring import confined here) | security, domain (error register) |
| `internal/security/teamauth` | Team-mode authorization adapter (D-36 accepted matrix + delegated-Job revocation; not wired into any product surface) | security |
| `internal/security/devicekey` | D-38 device-key identity lifecycle (accepted model v1): registry state machine, key-state verification denials, `PrivateKeySource` port; not wired into any product surface; transfer protocol stays D-42-gated | security, domain (error register) |
| `internal/config` | Validated configuration contract (C2-05): layered sources, ranges, policy conflicts | domain |
| `internal/health` | Liveness/readiness report model (C2-05); five signal groups | (none) |
| `internal/app` | Use cases and orchestration over ports | domain, module packages, events, security |
| `internal/contractadapter` | Runtime adapters derived from `/contracts` | domain |
| `internal/storage` | SQLite + CAS adapters (D-08/D-09; DB drivers here only) | domain, events, platform (OS-bound primitives per rule 4), module packages via ports |
| `internal/providers` | Provider capability model and adapters (D-15/D-16/D-17) | domain, security |
| `internal/plugins` | Plugin ABI host contracts (D-13/D-14; execution disabled) | domain, security |
| `internal/pluginhost` | Enforced WASM plugin host adapter (C4-25; wazero confined here; execution disabled — package fixtures only, D-14 open) | plugins |
| `internal/runtime` | Process modes, lifecycle, startup/shutdown/recovery wiring | app, security, platform, config, health, domain |
| `internal/platform` | OS paths and lifecycle adapters (OS APIs here only) | (ports) |
| `internal/remotestore` | PostgreSQL remote/team authority (D-10; pgx confined here; claim/fencing/outbox core + the `teamauth.Directory` identity resolution; not wired into any surface) | security (incl. teamauth: the Directory port types) |
| `cmd/uawp` | Composition root | any internal |

## Static dependency-rule check (specified; executable in C2-03 CI)

A Go test walks the package import graph (via `go list -deps`/`go/packages`) and fails
the build when any rule in this document is violated, including: forbidden imports for
`domain`, `app`, and the module packages; DB-driver imports outside `storage`;
OS-specific imports outside `platform`; and any import cycle. A `depguard`-style linter
configuration may mirror the same rules. The check is a build gate, not a security proof.

## Exception/ADR process

Any deviation from a rule above requires an ADR under `docs/adr/` stating the deviation,
alternatives, and rollback, reviewed by the Product owner. The register starts empty
(ADR-000 record kept in the Phase 1 traceability package); deviations are never silent.
