package updater

// The ports of the machine (C5-18 §1.2: "any OS-specific swap implementation
// → implementation under its own Go"; the confinement rules of
// dependency-map rule 4). Every environment-touching step is a port so this
// package uses no os/exec, no syscall, no database driver, and no network:
// the composition root (cmd/uawp) implements them per target. A REQUIRED
// port that is not wired makes the machine refuse fail-closed
// (ReasonPortUnavailable / the step's own reason) — a missing port is never
// a bypass.

import (
	"context"
	"crypto/ed25519"
	"time"
)

// Layout names the machine's filesystem locations (C5-15 §5.0's binding
// data-directory contract: the staging and state locations are the
// executable/update-cache and update-state classes — NEVER the user-data
// classes, and the install location is app-owned, never a package area).
type Layout struct {
	InstallPath string // the app-owned current binary (the swap target)
	RetainPath  string // the retained prior binary (the last-known-good anchor)
	StagingDir  string // the DISPOSABLE staging area (C5-18 S3)
	StateDir    string // the update-state class: journal + client state
}

// Locker is the single-instance data-directory lock (C5-18 §5.2 step 1:
// attach-or-refuse — a second process never updates anything). Production
// implements it over internal/platform's lock; the composition root wires it.
type Locker interface {
	Acquire(ctx context.Context) error
}

// MetadataFetcher fetches the published metadata bundle from the official
// endpoint (C5-18 T1). It returns the exact served bytes; the machine owns
// every verification. Production implements the endpoint fetch; no network
// code lives in this package.
type MetadataFetcher interface {
	Fetch(ctx context.Context) (MetadataBundle, error)
}

// ArtifactDownloader streams one artifact into the staging area (C5-18 S3).
// The machine computes the digest over the completed file and enforces the
// manifest's declared size — the downloader's contract is "write the bytes,
// report the failure"; any partial result is discarded by the disposable-
// staging rule (T4/R8: staging is never authoritative).
type ArtifactDownloader interface {
	Download(ctx context.Context, entry ArtifactEntry, destPath string) error
}

// Backupper is the C5-09-ACCEPTED local backup path (C5-18 S5): triggered by
// the update flow, committing before the machine may proceed. It creates no
// new guarantee, no schedule, no RPO/RTO claim — the reference it returns is
// the delivered backup manifest identity, journaled by the machine (T6).
type Backupper interface {
	CommitBackup(ctx context.Context) (reference string, err error)
}

// SupplyChainVerifier resolves the manifest's supply_chain[] pointers to
// their C5-16 binding-record digests (C5-18 S4; C5-19 A5: the records must
// exist and match). Production implements it over the C5-16 records.
type SupplyChainVerifier interface {
	Resolve(ctx context.Context, ptr SupplyChainPtr) (bindingRecordSHA256 string, err error)
}

// ArtifactSignatureVerifier is the C5-13 §3.2 artifact-signature layer (the
// per-target platform trust path — C5-15 §6). REQUIRED: until the signing
// stage exists (C5-13 §9.2) no production implementation can pass, so every
// update refuses — the honest blocked state. The stub implementations in the
// test suite use generated TEST KEYS only.
type ArtifactSignatureVerifier interface {
	Verify(ctx context.Context, artifactPath string, sig *Signature, targetID string) error
}

// ArtifactKindValidator checks the staged artifact is structurally valid for
// its declared kind (C5-19 A2's malformed-artifact refusal). OPTIONAL: nil
// means the machine performs no kind-specific structural parse (the binary
// kind needs none — the digest and the signature layer govern it); a wired
// validator's refusal is fail-closed.
type ArtifactKindValidator interface {
	Validate(ctx context.Context, artifactPath, kind string) error
}

// HealthCheckSpec names what S8 verifies (the minimal v1 check set,
// Proposed, recorded in the dependency-map entry: the data-directory lock,
// the database opens, migrations apply transactionally, the schema lands in
// the declared range, core diagnostics respond).
type HealthCheckSpec struct {
	Version         string
	MigrationFrom   int
	MigrationTo     int
	CandidateDigest string
}

// HealthChecker runs the S8 check set for the newly activated version
// (C5-18 S8). Production implements it by starting the new binary under
// observation — process-level work belongs to the composition root (os/exec
// confinement, dependency-map rule 4), never to this package.
type HealthChecker interface {
	Check(ctx context.Context, spec HealthCheckSpec) (passed bool, reason string)
}

// SwapRequest is the atomic-activation contract (C5-18 S7, Mode A): retain
// the current binary at RetainPath (the last-known-good anchor — its digest
// is already journaled), then move the staged candidate onto InstallPath
// atomically. The per-OS mechanism is the port's implementation: the
// rename-based Unix implementation ships here (swap_unix.go); the
// Windows/macOS implementations are the composition root's recorded
// follow-up (their semantics differ — e.g. a running Windows binary cannot
// be renamed over).
type SwapRequest struct {
	StagedPath        string
	InstallPath       string
	RetainPath        string
	ExpectedNewDigest string // the staged candidate's verified digest (post-swap sanity)
}

// Activator is the S7 atomic-activation port (Mode A). Restore is the S9
// binary-rollback mechanism: the retained prior binary goes back onto
// InstallPath — BINARY ONLY (C5-18 §5.3's separation rule: this machine
// never reverts user data).
type Activator interface {
	Swap(ctx context.Context, req SwapRequest) error
	Restore(ctx context.Context, retainPath, installPath string) error
}

// Event is one audit record (C5-18 §4.2's event names; the never-key-material
// rule — C5-13 §9.1 — applies to every field).
type Event struct {
	Name    string    `json:"name"`
	Reason  string    `json:"reason,omitempty"`
	Version string    `json:"version,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

// Auditor receives the machine's audit events. Production wires the
// delivered audit chain (internal/security) at the composition root; the
// test suite captures them for the C5-19 event-cell assertions.
type Auditor interface {
	Record(ctx context.Context, e Event)
}

// auditEventName are the C5-18 §4.2 event names.
const (
	EventUpdateCheck             = "update_check"
	EventUpdatePaused            = "update_paused"
	EventUpdateMetadataAccepted  = "update_metadata_accepted"
	EventUpdateSelected          = "update_selected"
	EventUpdateDownload          = "update_download"
	EventUpdateArtifactVerified  = "update_artifact_verified"
	EventUpdateBackupCommitted   = "update_backup_committed"
	EventUpdateValidated         = "update_validated"
	EventUpdateActivated         = "update_activated"
	EventUpdateHealth            = "update_health"
	EventUpdateStable            = "update_stable"
	EventUpdateRollback          = "update_rollback"
	EventUpdateRollbackFailed    = "update_rollback_failed"
	EventUpdateExternalReconcile = "update_external_reconcile"
)

// Clock returns the machine's wall-clock reading. Production injects
// time.Now; the tests inject a controllable clock (M4/L8's time-advance and
// discontinuity scenarios). The clock-quarantine rule (§5.2 step 2, the
// C5-05 FI-R2 class) uses the persisted last-validated time against it.
type Clock func() time.Time

// PinnedRoot is the client's out-of-band trust anchor (C5-17 §4.1: the root
// public key whose fingerprint the canonical anchor page publishes). It is
// CONSTRUCTION input, never served metadata — trusting a served key would
// erase the two-key structure's recovery property.
type PinnedRoot struct {
	Key ed25519.PublicKey
}
