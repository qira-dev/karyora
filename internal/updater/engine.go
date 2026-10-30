package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// The machine's update flow: C5-18 §4.2's T1–T10 with the C5-17 §7 binding
// check order at the front door — metadata validity (parse/bounds → root →
// manifest signature) → freshness (expiry + monotonic version, with the
// clock-quarantine rule) → THEN, and only then, the artifact path (digest →
// artifact signature → supply chain). A6's invariant: no code path reaches
// the artifact layer while any earlier check is unresolved.

// fileSHA256 computes the sha256 over a file's bytes in bounded chunks.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// quarantined implements §5.2 step 2 (the C5-05 FI-R2 class): a persisted
// last-validated time in the future relative to the current clock means the
// wall clock jumped backwards — freshness decisions are quarantined for this
// run (the machine refuses to treat the persisted state as either fresh or
// expired on quarantined evidence).
func (e *Engine) quarantined(st ClientState) bool {
	if st.LastValidated.IsZero() {
		return false
	}
	return e.Clock().Before(st.LastValidated)
}

// RunUpdate executes one update check-and-apply cycle (T1–T10): the timing
// policy and the explicit-consent gate are the caller's (C5-18 §9.3's
// delegated default — check on demand; never a silent install; the
// composition root asks the user before calling this when consent is
// required). Any failure is a typed Outcome; nothing is ever partially
// accepted.
func (e *Engine) RunUpdate(ctx context.Context) (Outcome, error) {
	st, manifest, out, ok, err := e.metadataPhase(ctx)
	if err != nil || !ok {
		return out, err
	}

	// The explicit-consent gate is the caller's responsibility (§9.3);
	// reaching here means the caller authorized this check-and-apply cycle.
	e.audit(ctx, EventUpdateSelected, "", manifest.ReleaseVersion, "update selected ("+e.InstalledVersion+" → "+manifest.ReleaseVersion+")")

	// --- S3 STAGED_DOWNLOAD (T3/T4) ---
	entry, found := artifactForTarget(manifest, e.TargetID)
	if !found {
		return Outcome{State: StateRefused, Reason: ReasonNoArtifact, Version: manifest.ReleaseVersion,
			Detail: "the manifest carries no artifact for target " + e.TargetID}, nil
	}
	if err := e.resetStaging(); err != nil {
		return Outcome{}, err
	}
	stagedPath := filepath.Join(e.Layout.StagingDir, "candidate")
	if e.Downloader == nil {
		return Outcome{State: StateRefused, Reason: ReasonPortUnavailable,
			Detail: "the artifact downloader port is not wired"}, nil
	}
	if err := e.Downloader.Download(ctx, entry, stagedPath); err != nil {
		_ = e.resetStaging()
		e.audit(ctx, EventUpdateDownload, "failed", manifest.ReleaseVersion, err.Error())
		return Outcome{State: StateRefused, Reason: ReasonFetchFailed, Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
	}
	info, err := os.Stat(stagedPath)
	if err != nil {
		_ = e.resetStaging()
		return Outcome{State: StateRefused, Reason: ReasonFetchFailed, Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
	}
	if info.Size() != entry.Size {
		// R5/A1 family: the delivered bytes do not match the signed
		// declaration — staging DELETED, typed refusal.
		_ = e.resetStaging()
		e.audit(ctx, EventUpdateDownload, string(ReasonDigestMismatch), manifest.ReleaseVersion, "staged size mismatch")
		return Outcome{State: StateRefused, Reason: ReasonDigestMismatch, Version: manifest.ReleaseVersion,
			Detail: "the staged artifact's size does not match the manifest"}, nil
	}
	digest, err := fileSHA256(stagedPath)
	if err != nil {
		_ = e.resetStaging()
		return Outcome{State: StateRefused, Reason: ReasonFetchFailed, Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
	}
	if digest != entry.SHA256 {
		// A1: mismatched artifact digest → staging DELETED → X1.
		_ = e.resetStaging()
		e.audit(ctx, EventUpdateDownload, string(ReasonDigestMismatch), manifest.ReleaseVersion, "staged digest mismatch")
		return Outcome{State: StateRefused, Reason: ReasonDigestMismatch, Version: manifest.ReleaseVersion,
			Detail: "the staged artifact's sha256 does not match the manifest"}, nil
	}
	j := &Journal{
		JournalVersion:  journalVer,
		Phase:           PhaseStaged,
		OldVersion:      e.InstalledVersion,
		NewVersion:      manifest.ReleaseVersion,
		CandidateDigest: digest,
		CandidatePath:   stagedPath,
		MigrationFrom:   manifest.Migrations.FromSchema,
		MigrationTo:     manifest.Migrations.ToSchema,
	}
	if err := saveJournal(e.Layout, j); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateDownload, "ok", manifest.ReleaseVersion, digest[:12])

	// --- S4 ARTIFACT_VERIFICATION (T5; the C5-13 §3.2 order: digest FIRST —
	// done above — THEN the artifact signature, THEN the supply chain) ---
	if e.ArtifactSig == nil {
		// The C5-13 platform trust layer is REQUIRED: without it no artifact
		// is verifiable and every update refuses (the honest blocked state —
		// C5-13 §9.2; never a digest-only downgrade).
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		return Outcome{State: StateRefused, Reason: ReasonArtifactSignature,
			Detail: "the artifact-signature verifier is not wired (the C5-13 signing stage is blocked)"}, nil
	}
	if e.KindValid != nil {
		if err := e.KindValid.Validate(ctx, stagedPath, entry.Kind); err != nil {
			_ = e.resetStaging()
			_ = saveJournal(e.Layout, nil)
			e.audit(ctx, EventUpdateArtifactVerified, string(ReasonArtifactMalformed), manifest.ReleaseVersion, err.Error())
			return Outcome{State: StateRefused, Reason: ReasonArtifactMalformed, Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
		}
	}
	if entry.Signature == nil {
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		e.audit(ctx, EventUpdateArtifactVerified, string(ReasonArtifactSignature), manifest.ReleaseVersion, "signature absent")
		return Outcome{State: StateRefused, Reason: ReasonArtifactSignature, Version: manifest.ReleaseVersion,
			Detail: "the artifact carries no signature (no digest-only downgrade)"}, nil
	}
	if err := e.ArtifactSig.Verify(ctx, stagedPath, entry.Signature, entry.TargetID); err != nil {
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		e.audit(ctx, EventUpdateArtifactVerified, string(ReasonArtifactSignature), manifest.ReleaseVersion, err.Error())
		return Outcome{State: StateRefused, Reason: ReasonArtifactSignature, Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
	}
	for _, ptr := range manifest.SupplyChain {
		if ptr.ArtifactSHA256 != entry.SHA256 {
			continue
		}
		if e.SupplyChain == nil {
			_ = e.resetStaging()
			_ = saveJournal(e.Layout, nil)
			return Outcome{State: StateRefused, Reason: ReasonSupplyChain, Version: manifest.ReleaseVersion,
				Detail: "the manifest carries supply-chain pointers but the resolver is not wired"}, nil
		}
		got, err := e.SupplyChain.Resolve(ctx, ptr)
		if err != nil || got != ptr.BindingRecordSHA256 {
			_ = e.resetStaging()
			_ = saveJournal(e.Layout, nil)
			e.audit(ctx, EventUpdateArtifactVerified, string(ReasonSupplyChain), manifest.ReleaseVersion, "supply-chain pointer unresolved/mismatched")
			return Outcome{State: StateRefused, Reason: ReasonSupplyChain, Version: manifest.ReleaseVersion,
				Detail: "a supply-chain pointer did not resolve to a matching C5-16 binding record"}, nil
		}
	}
	j.Phase = PhaseVerified
	if err := saveJournal(e.Layout, j); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateArtifactVerified, "ok", manifest.ReleaseVersion, "digest, signature, and supply-chain checks passed")

	// --- S5 PRE_UPDATE_BACKUP (T6; the C5-09 commitment consumed literally:
	// NO data migration proceeds without a committed backup) ---
	if e.Backupper == nil {
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		return Outcome{State: StateRefused, Reason: ReasonPortUnavailable,
			Detail: "the backup port is not wired (the C5-09 commitment requires a committed backup)"}, nil
	}
	ref, err := e.Backupper.CommitBackup(ctx)
	if err != nil {
		// R9/L6: refuse; last-known-good keeps running; zero data touch.
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		e.audit(ctx, EventUpdateBackupCommitted, string(ReasonBackupFailed), manifest.ReleaseVersion, err.Error())
		return Outcome{State: StateRefused, Reason: ReasonBackupFailed, Version: manifest.ReleaseVersion,
			Detail: "the update cannot proceed without a committed backup"}, nil
	}
	j.Phase = PhaseBackedUp
	j.BackupReference = ref
	if err := saveJournal(e.Layout, j); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateBackupCommitted, "ok", manifest.ReleaseVersion, ref)

	// --- S6 MIGRATION_VALIDATION (T7; both §6 gates BEFORE any data touch
	// and BEFORE activation) ---
	if err := ValidateMigrations(manifest, e.InstalledVersion); err != nil {
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		e.audit(ctx, EventUpdateValidated, string(reasonOf(err)), manifest.ReleaseVersion, err.Error())
		return Outcome{State: StateRefused, Reason: reasonOf(err), Version: manifest.ReleaseVersion, Detail: err.Error()}, nil
	}

	// T7's action: write the ACTIVATION_INTENT journal — durable BEFORE the
	// swap. The retained-binary digest is recorded now (the T8 recovery
	// decision datum).
	retainedDigest, err := fileSHA256(e.Layout.InstallPath)
	if err != nil {
		_ = e.resetStaging()
		_ = saveJournal(e.Layout, nil)
		return Outcome{State: StateRefused, Reason: ReasonPortUnavailable, Version: manifest.ReleaseVersion,
			Detail: "the installed binary could not be read to record the rollback anchor"}, nil
	}
	j.Phase = PhaseIntent
	j.RetainedDigest = retainedDigest
	if err := saveJournal(e.Layout, j); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateValidated, "ok", manifest.ReleaseVersion, "migration gates passed; activation intent journaled")

	// --- S7 ATOMIC_ACTIVATION (T8) ---
	if e.Activator == nil {
		// The intent stands: startup reconciliation completes or reverts —
		// the designed fail-closed behavior, never a half-state accepted.
		return Outcome{State: StateRefused, Reason: ReasonPortUnavailable, Version: manifest.ReleaseVersion,
			Detail: "the activator port is not wired; the journaled intent drives startup reconciliation"}, nil
	}
	if err := e.Activator.Swap(ctx, SwapRequest{
		StagedPath:        stagedPath,
		InstallPath:       e.Layout.InstallPath,
		RetainPath:        e.Layout.RetainPath,
		ExpectedNewDigest: digest,
	}); err != nil {
		// T8's fail-closed column: the mechanism's own atomicity governs; the
		// journal's intent entry drives startup reconciliation (§5.2). The
		// machine does NOT resolve it here.
		e.audit(ctx, EventUpdateActivated, string(ReasonSwapFailed), manifest.ReleaseVersion, err.Error())
		return Outcome{State: StateRefused, Reason: ReasonSwapFailed, Version: manifest.ReleaseVersion,
			Detail: "activation failed; the journaled intent drives startup reconciliation"}, nil
	}
	j.Phase = PhaseActivated
	if err := saveJournal(e.Layout, j); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateActivated, "ok", manifest.ReleaseVersion, "the staged binary is active; health check follows")

	// --- S8/S10 or S9 (T9/T10) ---
	return e.healthGate(ctx, st, j)
}

// healthGate is S8 + the T9/T10 branch: the check set runs; PASS advances
// the last-known-good pointer (never regressingly) and settles the update;
// FAIL restores the retained prior binary — BINARY ONLY.
func (e *Engine) healthGate(ctx context.Context, st ClientState, j *Journal) (Outcome, error) {
	if e.Health == nil {
		// Fail-closed: an unwired health check cannot PASS, so the machine
		// rolls back (never advances the pointer on unknown health).
		j.Phase = PhaseRollingBack
		_ = saveJournal(e.Layout, j)
		return e.rollback(ctx, j, string(ReasonPortUnavailable), "the health-check port is not wired")
	}
	passed, reason := e.Health.Check(ctx, HealthCheckSpec{
		Version:         j.NewVersion,
		MigrationFrom:   j.MigrationFrom,
		MigrationTo:     j.MigrationTo,
		CandidateDigest: j.CandidateDigest,
	})
	e.audit(ctx, EventUpdateHealth, boolWord(passed, reason), j.NewVersion, reason)
	if !passed {
		j.Phase = PhaseRollingBack
		if err := saveJournal(e.Layout, j); err != nil {
			return Outcome{}, err
		}
		return e.rollback(ctx, j, firstNonEmpty(reason, string(ReasonHealthFailed)), "the health check failed; the prior binary is restored")
	}
	// T9 → S10: advance the last-known-good pointer (ONLY on a PASS), settle
	// the journal, release the staging. The retained prior binary stays
	// (one-prior retention — the implementation constant recorded in the
	// dependency-map entry; a future update's retain step overwrites it).
	st.LastKnownGood = LastKnownGood{Version: j.NewVersion, SHA256: j.CandidateDigest}
	st.Paused = false
	st.PauseReason = ""
	if err := saveState(e.Layout, st); err != nil {
		return Outcome{}, err
	}
	if err := saveJournal(e.Layout, nil); err != nil {
		return Outcome{}, err
	}
	if err := e.resetStaging(); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateStable, "", j.NewVersion, "the update is stable; the last-known-good pointer advanced")
	return Outcome{State: StateUpdatedStable, Version: j.NewVersion}, nil
}

// rollback is S9/T10: restore the retained prior binary — BINARY ONLY. User
// data is never touched (the schema is safe under T7's compatibility gate;
// user-data recovery is the C5-09 restore or a forward fix — §5.3).
func (e *Engine) rollback(ctx context.Context, j *Journal, reason, detail string) (Outcome, error) {
	if e.Activator == nil {
		return Outcome{State: StateUnrecoverable, Reason: ReasonPortUnavailable, Version: j.NewVersion,
			Detail: "the activator port is not wired; the rollback cannot run"}, nil
	}
	if err := e.Activator.Restore(ctx, e.Layout.RetainPath, e.Layout.InstallPath); err != nil {
		// R13/L5 — the worst case, stated honestly: the machine reports the
		// unrecoverable-update-state WITHOUT touching data and names the
		// documented recovery (the C5-09 restore of the S5-committed backup).
		// The journal stays ROLLING_BACK: startup reconciliation retries the
		// revert; the backup reference is the recovery anchor.
		e.audit(ctx, EventUpdateRollbackFailed, reason, j.NewVersion, "the rollback mechanism failed; the C5-09 restore of the committed backup ("+j.BackupReference+") is the documented recovery")
		return Outcome{State: StateUnrecoverable, Reason: ReasonRollbackFailed, Version: j.NewVersion,
			Detail: "the rollback failed without touching user data; restore the committed backup (" + j.BackupReference + ") or ship a forward fix"}, nil
	}
	// The rollback succeeded: record the refused version (until a NEW
	// release exists), clear the journal, release the staging.
	st, err := loadState(e.Layout)
	if err == nil {
		if CompareVersions(j.NewVersion, st.RefusedThrough) > 0 {
			st.RefusedThrough = j.NewVersion
		}
		st.Paused = false
		st.PauseReason = ""
		_ = saveState(e.Layout, st) // best-effort; the refusal record is durable-state hygiene, not a gate
	}
	_ = saveJournal(e.Layout, nil)
	_ = e.resetStaging()
	e.audit(ctx, EventUpdateRollback, reason, j.OldVersion, detail)
	return Outcome{State: StateRefused, Reason: Reason(reason), Version: j.NewVersion, Detail: detail + " (binary-only rollback complete; user data untouched)"}, nil
}

// pause enters X2 UPDATE_PAUSED (R3/M1/M4): the fail-closed, user-visible
// pause — nothing downloads, last-known-good keeps running, and the state
// clears only on a fresh valid check (the delegated freeze behavior).
func (e *Engine) pause(ctx context.Context, st ClientState, reason, detail string) (Outcome, error) {
	st.Paused = true
	st.PauseReason = reason
	if err := saveState(e.Layout, st); err != nil {
		return Outcome{}, err
	}
	e.audit(ctx, EventUpdateCheck, reason, "", detail)
	e.audit(ctx, EventUpdatePaused, reason, "", detail)
	return Outcome{State: StatePaused, Reason: Reason(reason), Detail: detail}, nil
}

func (e *Engine) audit(ctx context.Context, name, reason, version, detail string) {
	if e.Auditor == nil {
		return // audit-less runs are a composition-root wiring defect, not a machine failure
	}
	e.Auditor.Record(ctx, Event{Name: name, Reason: reason, Version: version, Detail: detail, At: e.Clock()})
}

func (e *Engine) resetStaging() error {
	if err := os.RemoveAll(e.Layout.StagingDir); err != nil {
		return fail(ReasonPortUnavailable, "reset staging: %v", err)
	}
	if err := os.MkdirAll(e.Layout.StagingDir, 0o755); err != nil {
		return fail(ReasonPortUnavailable, "create staging: %v", err)
	}
	return nil
}

func artifactForTarget(m ReleaseManifest, targetID string) (ArtifactEntry, bool) {
	for _, a := range m.Artifacts {
		if a.TargetID == targetID {
			return a, true
		}
	}
	return ArtifactEntry{}, false
}

// refusedThrough reports whether the served release version is at/below the
// persisted refused-through version — a failed update's version is refused
// until a NEW release exists (C5-18 S9; the L4 invariant).
func refusedThrough(st ClientState, releaseVersion string) bool {
	if st.RefusedThrough == "" {
		return false
	}
	return CompareVersions(releaseVersion, st.RefusedThrough) <= 0
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func reasonOf(err error) Reason {
	if ue, ok := AsUpdaterError(err); ok {
		return ue.Reason
	}
	return ReasonMalformed
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func boolWord(passed bool, reason string) string {
	if passed {
		return "pass"
	}
	return reason
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
