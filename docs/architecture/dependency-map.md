# Core Dependency Map (Phase 2, C2-02)

Contract version 1.1 (2026-10-27): adds `internal/config`, `internal/health`, and the
domain `errs` subpackage while implementing the C2-05 contract set as code; no rule
changed. Version 1 derived from Core Master Blueprint v0.9 §1–§2 and the Phase 1
authority/module boundary map. Substantive changes require a version bump and an ADR.

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
| `internal/config` | Validated configuration contract (C2-05): layered sources, ranges, policy conflicts | domain |
| `internal/health` | Liveness/readiness report model (C2-05); five signal groups | (none) |
| `internal/app` | Use cases and orchestration over ports | domain, module packages, events, security |
| `internal/contractadapter` | Runtime adapters derived from `/contracts` | domain |
| `internal/storage` | SQLite + CAS adapters (D-08/D-09; DB drivers here only) | domain, events, module packages via ports |
| `internal/providers` | Provider capability model and adapters (D-15/D-16/D-17) | domain, security |
| `internal/plugins` | Plugin ABI host contracts (D-13/D-14; execution disabled) | domain, security |
| `internal/runtime` | Process modes, lifecycle, startup/shutdown/recovery wiring | app, security, platform |
| `internal/platform` | OS paths and lifecycle adapters (OS APIs here only) | (ports) |
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
