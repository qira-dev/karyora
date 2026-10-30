//go:build windows

package updater

// The Windows implementation of the S7 atomic-activation port (C5-18 S7,
// Mode A) and the S9 binary-rollback restore. A running Windows binary can
// be RENAMED but not replaced or deleted, so the swap moves the running
// executable aside first (rename-aside is permitted while the process runs),
// then renames the staged candidate onto the install path; a failure of the
// second rename renames the original back (the atomicity contract: the
// install path holds EITHER the old OR the new bytes — the T8 journal
// reconciliation decides by digest if a crash lands between the two
// renames). The best-effort removal of the renamed-aside copy usually fails
// while the old process still runs (its image stays mapped) and simply
// remains until a later swap — a harmless leftover in the app-owned install
// location, never in the user-data classes (C5-15 §5.0).
//
// Plain standard-library os.Rename/os.Remove calls only (the same boundary
// rationale recorded on swap_unix.go: portable stdlib calls, not the
// confined syscall/exec surface — dependency-map rule 4).

import (
	"context"
	"fmt"
	"os"
)

// DefaultActivator returns the Windows Mode-A activator; the composition
// root wires it on this platform.
func DefaultActivator() Activator { return ReplaceActivator{} }

// ReplaceActivator is the Windows Mode-A activator.
type ReplaceActivator struct{}

// Swap implements Activator.Swap.
func (ReplaceActivator) Swap(_ context.Context, req SwapRequest) error {
	if err := retainCurrent(req.InstallPath, req.RetainPath); err != nil {
		return fmt.Errorf("updater: retain the current binary: %w", err)
	}
	aside := req.InstallPath + ".replacing"
	if err := os.Rename(req.InstallPath, aside); err != nil {
		return fmt.Errorf("updater: move the running binary aside: %w", err)
	}
	if err := os.Rename(req.StagedPath, req.InstallPath); err != nil {
		// The candidate could not take the install path: put the original
		// back — the mechanism's own atomicity contract.
		if back := os.Rename(aside, req.InstallPath); back != nil {
			return fmt.Errorf("updater: activate the staged binary: %v (and the rename-back also failed: %v — the journal drives startup reconciliation)", err, back)
		}
		return fmt.Errorf("updater: activate the staged binary: %w", err)
	}
	// Post-swap sanity: the installed bytes must be the verified candidate
	// (the machine's own digest check is the real gate; this is the
	// mechanism's atomicity contract).
	ok, err := fileSHA256(req.InstallPath)
	if err != nil {
		return fmt.Errorf("updater: post-swap digest: %w", err)
	}
	if ok != req.ExpectedNewDigest {
		return fmt.Errorf("updater: post-swap digest mismatch (the journal will drive the revert)")
	}
	// Best-effort cleanup; a locked (still-running) image simply remains
	// until a later swap removes it.
	_ = os.Remove(aside)
	return nil
}

// Restore implements Activator.Restore: the retained prior binary goes back
// onto the install path — BINARY ONLY (C5-18 §5.3's separation rule; user
// data is never touched by this machine). The installed candidate is not
// running at rollback time (the health check failed and the prior process
// never switched), so a direct replace normally works; if the target is
// somehow locked, the candidate is renamed aside first (rename-aside is
// permitted on a locked-for-replacement file) and the retained binary takes
// the path.
func (ReplaceActivator) Restore(_ context.Context, retainPath, installPath string) error {
	if _, err := os.Stat(retainPath); err != nil {
		return fmt.Errorf("updater: the retained prior binary is unavailable: %w", err)
	}
	err := os.Rename(retainPath, installPath)
	if err == nil {
		return nil
	}
	aside := installPath + ".replacing"
	if moveErr := os.Rename(installPath, aside); moveErr != nil {
		return fmt.Errorf("updater: restore the prior binary: %v (and the installed candidate could not be moved aside: %v)", err, moveErr)
	}
	if err := os.Rename(retainPath, installPath); err != nil {
		// Put the candidate back rather than leave the path empty.
		if back := os.Rename(aside, installPath); back != nil {
			return fmt.Errorf("updater: restore the prior binary: %v (and the candidate could not be put back: %v — the journal drives startup reconciliation)", err, back)
		}
		return fmt.Errorf("updater: restore the prior binary: %w", err)
	}
	_ = os.Remove(aside)
	return nil
}
