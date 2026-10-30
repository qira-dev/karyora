package updater

// The lifecycle rows (C5-19 §6) — the C5-18 §7 recovery decision table R1–R15
// exercised in-process, bound by the §8.1 fixture identifiers. The crash
// windows are simulated by writing the exact durable residue a crash leaves
// (journal phase + files) and then running the startup reconciliation — the
// same reconciliation the SIGKILL-injection discipline would exercise against
// a real process (C5-19 §8.1).

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"
)

// runFullUpdate drives one complete successful cycle so a test starts from a
// stable updated state (S10: lkg advanced, journal cleared).
func runFullUpdate(t *testing.T, h *hHarness) Outcome {
	t.Helper()
	h.serve(bundleOpts{})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatalf("seed update: %v", err)
	}
	if out.State != StateUpdatedStable {
		t.Fatalf("seed update: outcome = %+v, want S10", out)
	}
	return out
}

// L1 (CLM-UA-034): interrupted download — the staging residue of a crash is
// discarded UNCONDITIONALLY at startup and the machine returns to S0; a
// half-downloaded artifact can never be a candidate (T4's disposable rule).
func TestUpdateAdversarialInterruptedDownloadStagingDiscarded(t *testing.T) {
	h := newHarness(t)
	// The crash residue: a partial staging file, NO journal (the journal is
	// written only after digest verification — a mid-download crash leaves
	// exactly this).
	if err := writeFile(h.layout.StagingDir+"/candidate.part", []byte(artifactMagic+"partial")); err != nil {
		t.Fatal(err)
	}
	rec, err := h.engine.ReconcileAtStartup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != "clean" {
		t.Fatalf("reconcile = %+v, want clean S0", rec)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not discarded: %v", entries)
	}
	j, _ := loadJournal(h.layout)
	if j != nil {
		t.Fatalf("no journal may exist: %+v", j)
	}
	// The machine then works normally: a full cycle completes.
	h.serve(bundleOpts{})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.State != StateUpdatedStable {
		t.Fatalf("post-cleanup cycle: outcome = %+v err = %v", out, err)
	}
}

// L2 (CLM-UA-035): the designed worst case — crash between the swap and the
// health check (T8): the journal's ACTIVATION_INTENT + the current binary's
// digest decide complete-and-verify vs revert-to-prior; a half-swapped state
// is never accepted as final.
func TestUpdateAdversarialCrashBetweenSwapAndHealth(t *testing.T) {
	oldBytes := []byte(artifactMagic + "v" + installedVersion)
	newBytes := []byte(artifactMagic + "v" + newVersion)

	t.Run("crash after the swap: complete-and-verify", func(t *testing.T) {
		h := newHarness(t)
		if err := writeFile(h.layout.InstallPath, newBytes); err != nil { // the swap landed
			t.Fatal(err)
		}
		if err := writeFile(h.layout.RetainPath, oldBytes); err != nil { // the retain landed
			t.Fatal(err)
		}
		if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseIntent,
			OldVersion: installedVersion, NewVersion: newVersion,
			CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/candidate",
			RetainedDigest: sha256Hex(oldBytes)}); err != nil {
			t.Fatal(err)
		}
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "resolved-updated" {
			t.Fatalf("reconcile = %+v, want resolved-updated (proceed to S8)", rec)
		}
		if h.health.calls == 0 {
			t.Fatal("the health check must re-run on the reconciled activation")
		}
		st, _ := loadState(h.layout)
		if st.LastKnownGood.Version != newVersion {
			t.Fatalf("last-known-good = %+v, want advanced to %s", st.LastKnownGood, newVersion)
		}
	})

	t.Run("crash before the swap: complete from staging", func(t *testing.T) {
		h := newHarness(t)
		// Install still the OLD binary; the verified candidate still staged.
		if err := writeFile(h.layout.StagingDir+"/candidate", newBytes); err != nil {
			t.Fatal(err)
		}
		if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseIntent,
			OldVersion: installedVersion, NewVersion: newVersion,
			CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/candidate",
			RetainedDigest: sha256Hex(oldBytes)}); err != nil {
			t.Fatal(err)
		}
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "resolved-updated" {
			t.Fatalf("reconcile = %+v, want the activation completed from staging then S8", rec)
		}
		current, _ := readFile(h.layout.InstallPath)
		if string(current) != string(newBytes) {
			t.Fatalf("the activation was not completed: install = %q", current)
		}
	})

	t.Run("half-swapped state: revert-to-prior", func(t *testing.T) {
		h := newHarness(t)
		// The install path carries NEITHER the old nor the new bytes — the
		// half-swapped corruption case.
		if err := writeFile(h.layout.InstallPath, []byte(artifactMagic+"HALF-SWAPPED")); err != nil {
			t.Fatal(err)
		}
		if err := writeFile(h.layout.RetainPath, oldBytes); err != nil {
			t.Fatal(err)
		}
		if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseIntent,
			OldVersion: installedVersion, NewVersion: newVersion,
			CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/gone",
			RetainedDigest: sha256Hex(oldBytes)}); err != nil {
			t.Fatal(err)
		}
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "resolved-rollback" {
			t.Fatalf("reconcile = %+v, want resolved-rollback (revert-to-prior)", rec)
		}
		current, _ := readFile(h.layout.InstallPath)
		if string(current) != string(oldBytes) {
			t.Fatalf("the prior binary was not restored: install = %q", current)
		}
		st, _ := loadState(h.layout)
		if st.RefusedThrough != newVersion {
			t.Fatalf("refused-through = %q, want %s", st.RefusedThrough, newVersion)
		}
	})
}

// L3 (CLM-UA-036): crash during migration — the machine re-enters S8
// IDEMPOTENTLY (no state advances without a PASS); a failed health re-run
// settles exactly like a live failure (T10).
func TestUpdateAdversarialCrashDuringMigrationIdempotent(t *testing.T) {
	newBytes := []byte(artifactMagic + "v" + newVersion)
	oldBytes := []byte(artifactMagic + "v" + installedVersion)

	t.Run("re-enters S8 idempotently and settles on a PASS", func(t *testing.T) {
		h := newHarness(t)
		if err := writeFile(h.layout.InstallPath, newBytes); err != nil {
			t.Fatal(err)
		}
		if err := writeFile(h.layout.RetainPath, oldBytes); err != nil {
			t.Fatal(err)
		}
		if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseActivated,
			OldVersion: installedVersion, NewVersion: newVersion,
			CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/candidate",
			RetainedDigest: sha256Hex(oldBytes), MigrationFrom: 1, MigrationTo: 2,
			BackupReference: "backup-ref-1"}); err != nil {
			t.Fatal(err)
		}
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "resolved-updated" || h.health.calls != 1 {
			t.Fatalf("reconcile = %+v health calls = %d, want exactly one idempotent S8 re-entry", rec, h.health.calls)
		}
		if h.health.last.MigrationFrom != 1 || h.health.last.MigrationTo != 2 {
			t.Fatalf("health spec = %+v, want the journaled migration range", h.health.last)
		}
	})

	t.Run("a failed migration re-run rolls back transactionally-settled data untouched", func(t *testing.T) {
		h := newHarness(t)
		if err := writeFile(h.layout.InstallPath, newBytes); err != nil {
			t.Fatal(err)
		}
		if err := writeFile(h.layout.RetainPath, oldBytes); err != nil {
			t.Fatal(err)
		}
		if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseActivated,
			OldVersion: installedVersion, NewVersion: newVersion,
			CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/candidate",
			RetainedDigest: sha256Hex(oldBytes), MigrationFrom: 1, MigrationTo: 2,
			BackupReference: "backup-ref-2"}); err != nil {
			t.Fatal(err)
		}
		h.health.pass = false
		h.health.reason = string(ReasonHealthFailed)
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "resolved-rollback" {
			t.Fatalf("reconcile = %+v, want resolved-rollback", rec)
		}
		current, _ := readFile(h.layout.InstallPath)
		if string(current) != string(oldBytes) {
			t.Fatalf("the prior binary was not restored: %q", current)
		}
		st, _ := loadState(h.layout)
		if st.RefusedThrough != newVersion {
			t.Fatalf("refused-through = %q, want %s", st.RefusedThrough, newVersion)
		}
	})
}

// L4 (CLM-UA-037): health-check failure → the BINARY-ONLY rollback: the
// prior binary restored, the pointer NOT advanced, the failed version
// refused until a new release, the user data untouched.
func TestUpdateAdversarialHealthFailureRollsBackBinaryOnly(t *testing.T) {
	h := newHarness(t)
	sentinel := h.dir + "/userdata/sentinel"
	if err := writeFile(sentinel, []byte("USER-DATA-INTACT")); err != nil {
		t.Fatal(err)
	}
	oldBytes := []byte(artifactMagic + "v" + installedVersion)
	h.health.pass = false
	h.health.reason = string(ReasonHealthFailed)
	h.serve(bundleOpts{})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonHealthFailed {
		t.Fatalf("outcome = %+v, want the S9 health-failure rollback", out)
	}
	current, _ := readFile(h.layout.InstallPath)
	if string(current) != string(oldBytes) {
		t.Fatalf("the prior binary was not restored: %q", current)
	}
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastKnownGood.Version != "" && st.LastKnownGood.Version == newVersion {
		t.Fatal("the last-known-good pointer must NOT advance on a failed update")
	}
	if st.RefusedThrough != newVersion {
		t.Fatalf("refused-through = %q, want %s", st.RefusedThrough, newVersion)
	}
	if st.Paused {
		t.Fatal("a health-failure rollback is not an X2 pause")
	}
	if h.auditor.count(EventUpdateRollback) == 0 {
		t.Error("missing update_rollback audit event")
	}
	// The failed version is refused until a NEW release exists; the user
	// data sentinel is untouched (the separation rule — §5.3).
	h.health.pass = true
	h.serve(bundleOpts{metadataVer: 3, releaseVersion: newVersion})
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonRefusedVersion {
		t.Fatalf("re-run outcome = %+v, want refused_version until a new release", out)
	}
	got, _ := readFile(sentinel)
	if string(got) != "USER-DATA-INTACT" {
		t.Fatalf("user data touched: %q", got)
	}
}

// L5 (CLM-UA-038): the rollback itself fails — the honest
// unrecoverable-update-state WITHOUT touching data, naming the documented
// C5-09 restore path of the S5-committed backup; startup reconciliation
// completes the revert once the mechanism works again.
func TestUpdateAdversarialRollbackFailureRestorePath(t *testing.T) {
	h := newHarness(t)
	newBytes := []byte(artifactMagic + "v" + newVersion)
	oldBytes := []byte(artifactMagic + "v" + installedVersion)
	if err := writeFile(h.layout.InstallPath, newBytes); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(h.layout.RetainPath, oldBytes); err != nil {
		t.Fatal(err)
	}
	if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseActivated,
		OldVersion: installedVersion, NewVersion: newVersion,
		CandidateDigest: sha256Hex(newBytes), CandidatePath: h.layout.StagingDir + "/candidate",
		RetainedDigest: sha256Hex(oldBytes), BackupReference: "backup-ref-worst-case"}); err != nil {
		t.Fatal(err)
	}
	h.health.pass = false
	h.health.reason = string(ReasonHealthFailed)
	h.engine.Activator = failingRestoreActivator{}
	rec, err := h.engine.ReconcileAtStartup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != "unrecoverable" || rec.Reason != ReasonRollbackFailed {
		t.Fatalf("reconcile = %+v, want the honest unrecoverable-update-state", rec)
	}
	if h.auditor.count(EventUpdateRollbackFailed) == 0 {
		t.Error("missing update_rollback_failed audit event")
	}
	// The journal STAYS rolling-back: the machine retries at the next
	// startup; the committed backup reference is named as the recovery.
	j, _ := loadJournal(h.layout)
	if j == nil || j.Phase != PhaseRollingBack {
		t.Fatalf("journal = %+v, want the retained ROLLING_BACK phase", j)
	}
	// The mechanism recovers; the next startup completes the revert.
	h.engine.Activator = RenameActivator{}
	rec, err = h.engine.ReconcileAtStartup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != "resolved-rollback" {
		t.Fatalf("second reconcile = %+v, want the revert completed", rec)
	}
	current, _ := readFile(h.layout.InstallPath)
	if string(current) != string(oldBytes) {
		t.Fatalf("the prior binary was not restored: %q", current)
	}
}

// failingRestoreActivator is the L5 fixture: Swap works, Restore never does.
type failingRestoreActivator struct{ RenameActivator }

func (failingRestoreActivator) Restore(_ context.Context, _, _ string) error {
	return errors.New("the rollback mechanism is broken (the injected worst case)")
}

// L6 (CLM-UA-039): backup failure → X1 refusal; NO data migration proceeds
// without a committed backup (the C5-09 commitment consumed literally).
func TestUpdateAdversarialBackupFailureRefusesUpdate(t *testing.T) {
	h := newHarness(t)
	sentinel := h.dir + "/userdata/sentinel"
	if err := writeFile(sentinel, []byte("USER-DATA-INTACT")); err != nil {
		t.Fatal(err)
	}
	h.backupper.err = errors.New("the backup could not commit")
	h.serve(bundleOpts{})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonBackupFailed {
		t.Fatalf("outcome = %+v, want X1/backup_failed", out)
	}
	if h.downloader.calls != 1 {
		t.Fatalf("downloads = %d, want exactly the refused attempt's download", h.downloader.calls)
	}
	j, _ := loadJournal(h.layout)
	if j != nil {
		t.Fatalf("no journal may survive a backup refusal: %+v", j)
	}
	got, _ := readFile(sentinel)
	if string(got) != "USER-DATA-INTACT" {
		t.Fatalf("user data touched: %q", got)
	}
	// The update re-runs S5 from the start once the backup condition is fixed
	// (a fresh metadata_version — the refusal never advanced the machine's
	// accepted-metadata state, but the served manifest was already consumed).
	h.backupper.err = nil
	h.serve(bundleOpts{metadataVer: 3})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.State != StateUpdatedStable {
		t.Fatalf("retry outcome = %+v err = %v, want S10", out, err)
	}
}

// L7 (CLM-UA-040/041/042): the three migration-validation gates — refusal
// BEFORE any data touch and BEFORE activation, with the failed gate named.
func TestUpdateAdversarialMigrationValidationGatesRefused(t *testing.T) {
	t.Run("min_supported_client gate", func(t *testing.T) {
		h := newHarness(t)
		h.serve(bundleOpts{minSupported: "2.0.0"}) // installed 1.0.0 < min 2.0.0
		out, err := h.engine.RunUpdate(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateRefused || out.Reason != ReasonMigrationGate {
			t.Fatalf("outcome = %+v, want X1/migration_gate", out)
		}
		if !contains(out.Detail, "2.0.0") || !contains(out.Detail, "intermediate") {
			t.Fatalf("the refusal must name the intermediate-upgrade instruction: %q", out.Detail)
		}
		assertNoDataTouch(t, h)
	})
	t.Run("rollback-compatibility gate (refuse-incompatible)", func(t *testing.T) {
		h := newHarness(t)
		false_ := false
		h.serve(bundleOpts{rollbackCompat: &false_})
		out, err := h.engine.RunUpdate(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if out.Reason != ReasonMigrationGate {
			t.Fatalf("outcome = %+v, want the refuse-incompatible refusal", out)
		}
		assertNoDataTouch(t, h)
	})
	t.Run("undeclared reversibility class", func(t *testing.T) {
		h := newHarness(t)
		h.serve(bundleOpts{stepClass: "maybe-reversible-who-knows"})
		out, err := h.engine.RunUpdate(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if out.Reason != ReasonMigrationGate {
			t.Fatalf("outcome = %+v, want the declaration refusal", out)
		}
		assertNoDataTouch(t, h)
	})
}

// assertNoDataTouch proves the pre-activation refusals touched nothing: no
// journal exists, the staging is released, and the installed binary is
// unchanged (the T7 structural guarantee, asserted).
func assertNoDataTouch(t *testing.T, h *hHarness) {
	t.Helper()
	j, _ := loadJournal(h.layout)
	if j != nil {
		t.Fatalf("a journal exists after a pre-activation refusal: %+v", j)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not released: %v", entries)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// L8 (CLM-UA-043): clock discontinuity — freshness decisions are quarantined
// (the C5-05 FI-R2 class); no install decision on quarantined evidence.
func TestUpdateAdversarialClockDiscontinuityQuarantine(t *testing.T) {
	h := newHarness(t)
	// A prior validated run stamps the clock; the wall clock then jumps
	// BACKWARDS (the persisted last-validated lands in the future).
	h.serve(bundleOpts{metadataVer: 2})
	st := ClientState{StateVersion: stateVersion, LastValidated: h.clock.now.Add(time.Hour),
		ReleaseSignerKey: h.signerKey.Public().(ed25519.PublicKey), RootVersionSeen: 1}
	if err := saveState(h.layout, st); err != nil {
		t.Fatal(err)
	}
	h.serve(bundleOpts{metadataVer: 3})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonClockQuarantine {
		t.Fatalf("run outcome = %+v, want the clock-quarantine refusal", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0 on quarantined evidence", h.downloader.calls)
	}
	rec, err := h.engine.ReconcileAtStartup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != "quarantined" {
		t.Fatalf("reconcile = %+v, want the quarantined state", rec)
	}
}

// L9 (CLM-UA-044): externally-managed version change (Mode B, X3) — the
// machine verifies what the mode allows, re-runs health, and advances the
// pointer ONLY on a full pass; otherwise the honest mismatch is shown.
func TestUpdateAdversarialExternallyManagedReconcile(t *testing.T) {
	oldBytes := []byte(artifactMagic + "v" + installedVersion)
	externalBytes := []byte(artifactMagic + "v2.0.0-packaged")

	t.Run("health pass: the pointer advances to the externally managed install", func(t *testing.T) {
		h := newHarness(t)
		if err := writeFile(h.layout.InstallPath, oldBytes); err != nil {
			t.Fatal(err)
		}
		st := ClientState{StateVersion: stateVersion,
			LastKnownGood: LastKnownGood{Version: installedVersion, SHA256: sha256Hex(oldBytes)}}
		if err := saveState(h.layout, st); err != nil {
			t.Fatal(err)
		}
		// The package transaction replaces the binary out-of-band.
		if err := writeFile(h.layout.InstallPath, externalBytes); err != nil {
			t.Fatal(err)
		}
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "external-reconciled" {
			t.Fatalf("reconcile = %+v, want X3 reconciled", rec)
		}
		if h.auditor.count(EventUpdateExternalReconcile) == 0 {
			t.Error("missing update_external_reconcile audit event")
		}
		after, _ := loadState(h.layout)
		if after.LastKnownGood.SHA256 != sha256Hex(externalBytes) {
			t.Fatalf("pointer = %+v, want the externally managed digest", after.LastKnownGood)
		}
	})

	t.Run("health fail: the honest mismatch state, the pointer NOT advanced", func(t *testing.T) {
		h := newHarness(t)
		if err := writeFile(h.layout.InstallPath, oldBytes); err != nil {
			t.Fatal(err)
		}
		st := ClientState{StateVersion: stateVersion,
			LastKnownGood: LastKnownGood{Version: installedVersion, SHA256: sha256Hex(oldBytes)}}
		if err := saveState(h.layout, st); err != nil {
			t.Fatal(err)
		}
		if err := writeFile(h.layout.InstallPath, externalBytes); err != nil {
			t.Fatal(err)
		}
		h.health.pass = false
		h.health.reason = "diagnostics unreachable"
		rec, err := h.engine.ReconcileAtStartup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != "external-mismatch" {
			t.Fatalf("reconcile = %+v, want the honest X3 mismatch", rec)
		}
		after, _ := loadState(h.layout)
		if after.LastKnownGood.SHA256 != sha256Hex(oldBytes) {
			t.Fatalf("the pointer must NOT advance on a failed verification: %+v", after.LastKnownGood)
		}
	})
}
