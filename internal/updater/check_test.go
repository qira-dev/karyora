package updater

// The on-demand check path (C5-18 §9.3's delegated default) bound to its
// design rows: a check verifies metadata and decides the version but NEVER
// downloads, stages, or swaps — the consent gate stays with the apply path.

import (
	"strings"
	"testing"
	"time"
)

func TestCheckForUpdateReportsAvailableWithoutStaging(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{})
	out, err := h.engine.CheckForUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateNoUpdate || out.Version != newVersion {
		t.Fatalf("outcome = %+v; want S0 reporting %s", out, newVersion)
	}
	if !strings.Contains(out.Detail, "available") {
		t.Fatalf("detail = %q; want the available-update wording", out.Detail)
	}
	// T2 persisted the monotonic state (the rollback protection survives any
	// later failure) exactly as the apply path persists it.
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.HighestSeenMetadataVersion != 2 || st.LastValidated.IsZero() {
		t.Fatalf("persisted state = %+v; want the accepted metadata version and last-validated stamp", st)
	}
	// The check path never touches the artifact layer: staging stays empty
	// and no journal exists (nothing was selected).
	entries, err := listDir(h.layout.StagingDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging = %v (%v); want empty", entries, err)
	}
	j, err := loadJournal(h.layout)
	if err != nil || j != nil {
		t.Fatalf("journal = %v (%v); want none", j, err)
	}
}

func TestCheckForUpdateExpiredPauses(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{expires: h.clock.now.Add(-time.Hour)})
	out, err := h.engine.CheckForUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StatePaused || out.Reason != ReasonExpired {
		t.Fatalf("outcome = %+v; want X2 expired", out)
	}
	st, err := loadState(h.layout)
	if err != nil || !st.Paused || st.PauseReason != string(ReasonExpired) {
		t.Fatalf("state = %+v (%v); want the visible pause", st, err)
	}
}

func TestCheckForUpdateReplayIsIdempotent(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{})
	if _, err := h.engine.CheckForUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	out, err := h.engine.CheckForUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The second check serves the identical manifest: the replay outcome is
	// "no applicable update", never a re-accept.
	if out.State != StateNoUpdate || out.Detail != "" {
		t.Fatalf("replay outcome = %+v; want the plain no-update replay", out)
	}
}

func TestCheckForUpdateRefusesMidFlightJournal(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{})
	if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseStaged,
		OldVersion: installedVersion, NewVersion: newVersion,
		CandidateDigest: sha256Hex([]byte("x")), CandidatePath: h.layout.StagingDir + "/candidate"}); err != nil {
		t.Fatal(err)
	}
	out, err := h.engine.CheckForUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonJournalPending {
		t.Fatalf("outcome = %+v; want the journal_pending refusal", out)
	}
}

func TestCheckForUpdateRefusesDowngrade(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{releaseVersion: "0.9.0"})
	out, err := h.engine.CheckForUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonDowngradeRefused {
		t.Fatalf("outcome = %+v; want the downgrade refusal", out)
	}
}
