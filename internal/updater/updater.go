// Package updater implements the owner-approved C5-18 staged-update state
// machine (Core CR-5.2 C5-18 §4) for the application-update lifecycle: the
// signed-metadata check (C5-17 §7's binding order — metadata validity, then
// expiry/version/freshness, then artifact digest, then artifact signature),
// the disposable staging area, the durable update journal with startup
// reconciliation (C5-18 §5), the migration compatibility gates (§6), atomic
// activation, the health check, and the last-known-good BINARY-only rollback
// (§5.3's separation rule: user-data recovery is the C5-09 restore path or a
// forward fix — never this machine).
//
// Trust parameters are the owner's standing-delegation values (C5-17 §14):
// a two-key structure (offline root delegating an online release signer),
// threshold 1 per role, per-release expiry carried inside the signed
// metadata itself, fail-closed user-visible pause (X2) past expiry, and no
// downgrades in v1. Metadata signatures are Ed25519 over the exact canonical
// payload bytes (crypto/ed25519 — the standard library; the C5-17 §4.2
// envelope/canonicalization choice fixed at implementation: the published
// document embeds its payload as an exact-byte JSON string and the signature
// covers those bytes, so the client never re-canonicalizes and a splice of
// documents from different releases fails byte-exact verification).
//
// The package imports NOTHING internal (dependency-map v1.19) and uses no
// os/exec and no OS-specific API (those are confined to internal/platform by
// rule 4): every environment-touching step is a port (§1.2 of C5-18 — the
// per-OS swap mechanics and process-level health checks are the composition
// root's implementations). No SLSA or benignity claim is made anywhere
// (C5-14 §5.4; C5-17 §7) and no artifact signature exists until the C5-13
// signing stage unblocks — the artifact-signature port is REQUIRED, so a
// machine without it refuses every update fail-closed.
package updater

import (
	"errors"
	"fmt"
)

// Reason is the typed refusal/failure catalog of the machine: every failure
// is one of these reasons, mapped from C5-17 §5's rows and C5-18 §4's
// fail-closed column and §7's recovery table. There is no "proceed anyway"
// path for any of them (C5-13 §3.3 binding).
type Reason string

const (
	ReasonFetchFailed        Reason = "fetch_failed"           // R1 (M9): the metadata endpoint is unreachable
	ReasonMalformed          Reason = "malformed"              // R2 (M8): schema-invalid structure or field-bound violation
	ReasonOversized          Reason = "oversized"              // R2 (M8): hard size/field bounds exceeded
	ReasonSignatureInvalid   Reason = "signature_invalid"      // R2 (M5/M6): signature missing, wrong key, or invalid
	ReasonThresholdShortfall Reason = "threshold_shortfall"    // R2 (M6): fewer valid signatures than the approved threshold
	ReasonExpired            Reason = "expired"                // R3 (M1/M4): signed metadata past its expires_at
	ReasonVersionRollback    Reason = "version_rollback"       // R4 (M2/M3): metadata_version below the persisted highest-seen
	ReasonVersionReuse       Reason = "version_reuse"          // R4 (M11): metadata_version equal to the highest-seen but different bytes
	ReasonInconsistent       Reason = "inconsistent"           // (M10): cross-release metadata combination (root/manifest version mismatch)
	ReasonClockQuarantine    Reason = "clock_quarantine"       // R15 (L8): wall-clock discontinuity quarantines freshness decisions
	ReasonDowngradeRefused   Reason = "downgrade_refused"      // (M3 variant b): P-DOWNGRADE — no downgrades in v1
	ReasonRefusedVersion     Reason = "refused_version"        // (L4): a release_version at/below a failed update's version
	ReasonNoArtifact         Reason = "no_artifact_for_target" // S3: the manifest carries no artifact for this target
	ReasonDigestMismatch     Reason = "digest_mismatch"        // R5 (A1): staged artifact's sha256 != the manifest's
	ReasonArtifactMalformed  Reason = "artifact_malformed"     // R5 (A2): the artifact is not valid for its declared kind
	ReasonArtifactSignature  Reason = "artifact_signature"     // R6 (A3): the C5-13 artifact-signature layer refused
	ReasonSupplyChain        Reason = "supply_chain"           // R7 (A5): supply-chain pointers unresolved/mismatched
	ReasonBackupFailed       Reason = "backup_failed"          // R9 (L6): the C5-09 backup did not commit
	ReasonMigrationGate      Reason = "migration_gate"         // R10 (L7): min_supported_client / declaration / rollback-compatibility
	ReasonSwapFailed         Reason = "swap_failed"            // (L2): the activation mechanism failed; the journal drives startup recovery
	ReasonHealthFailed       Reason = "health_failed"          // R12 (L4): the S8 check set failed
	ReasonRollbackFailed     Reason = "rollback_failed"        // R13 (L5): the rollback mechanism itself failed — the worst case
	ReasonJournalPending     Reason = "journal_pending"        // §5.2: an unresolved journal exists — reconciliation must run first
	ReasonPortUnavailable    Reason = "port_unavailable"       // fail-closed: a required port is not wired
	ReasonLockRefused        Reason = "lock_refused"           // §5.2 step 1: the data-directory lock is held elsewhere
	ReasonSelfCheckMismatch  Reason = "self_check_mismatch"    // §5.2 step 5: the running binary's digest is unexplained
)

// Error is the typed failure of the machine: a Reason plus the human-visible
// detail (never key material, never secret material — C5-13 §9.1).
type Error struct {
	Reason Reason
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("updater: %s", e.Reason)
	}
	return fmt.Sprintf("updater: %s: %s", e.Reason, e.Detail)
}

func fail(r Reason, format string, args ...any) *Error {
	return &Error{Reason: r, Detail: fmt.Sprintf(format, args...)}
}

// AsUpdaterError extracts the typed failure from an error chain.
func AsUpdaterError(err error) (*Error, bool) {
	var ue *Error
	if errors.As(err, &ue) {
		return ue, true
	}
	return nil, false
}

// Outcome is the terminal state of one machine run or reconciliation, mapped
// to the C5-18 §4.1 states (S0/S10/X1/X2/X3 and the recovery rows of §7).
type Outcome struct {
	// State names the C5-18 §4.1 state the machine settled in: "S0", "S10",
	// "X1", "X2", "X3" (X3's reconciled and mismatch forms are
	// distinguished by Reason).
	State   string
	Reason  Reason
	Version string // the release version the outcome concerns (when any)
	Detail  string
}

// Outcome states (C5-18 §4.1).
const (
	StateNoUpdate          = "S0"  // no applicable update; last-known-good running
	StateUpdatedStable     = "S10" // health PASS; the last-known-good pointer advanced
	StateRefused           = "X1"  // terminal typed refusal for this attempt
	StatePaused            = "X2"  // expired/frozen metadata — fail-closed visible pause
	StateExternallyManaged = "X3"  // Mode B: an externally changed install reconciled (Reason distinguishes reconciled vs mismatch)
	StateUnrecoverable     = "S9F" // R13: the rollback mechanism itself failed — the honest worst case
)
