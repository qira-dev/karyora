// Package remotestore is the PostgreSQL remote/team authority (CR-4.4
// sub-milestone B, C4-33/C4-34): the pinned pgx driver (D-10) behind the
// schema-area map of the Remote PostgreSQL and Object Storage
// Specification — Work tables excluded (D-12/D-33), every mutable row
// carrying its workspace scope column. The atomic claim runs
// SELECT ... FOR UPDATE SKIP LOCKED inside one transaction together with
// the precondition checks, the monotonic fencing-epoch bump, the attempt +
// lease rows and the job state change, with the SERVER clock read inside
// that same transaction deciding lease validity — a worker-host clock
// never decides authority. Every authoritative commit verifies the full
// fencing tuple and refuses stale/missing/mismatched evidence.
//
// Status boundary: the claim/lease/fenced-commit/outbox core is real
// code proven against a live PostgreSQL 18 server (the conformance
// fixtures of spec §2 executed locally); the object-store half of the
// specification (C4-36, spec §4) is implemented in this package on the
// owner-selected MinIO S3-compatible profile (2026-10-29) — tenant-
// scoped (workspace, digest) identity, upload-before-reference ordering,
// digest-verified reads, the reference-aware GC with grace + second-pass
// recheck + backup pins, and the typed missing/corrupt failures — with
// its conformance fixtures executed against a live MinIO server and a
// live PostgreSQL. The store is NOT wired into any product surface,
// remote mode does not exist yet, and the failover/clock-discontinuity
// rows of spec §3 remain Not Run (they need a controlled failover
// environment); the restore-reconciliation fixture row is Not Run (no
// restore surface exists). Local mode has zero dependency on this
// package (isolation rule).
package remotestore
