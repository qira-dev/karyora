//go:build unix

package updater

// The Unix (Linux/macOS) implementation of the S7 atomic-activation port
// (C5-18 S7, Mode A) and the S9 binary-rollback restore. The mechanism is
// rename-based within the app-owned install location — os.Rename replaces
// the target atomically on POSIX systems, so a crash leaves EITHER the old
// OR the new binary at InstallPath, never a mixture (the T8 reconciliation
// decides by digest). The Windows/macOS-specific mechanisms (a running
// Windows binary cannot be renamed over) are the composition root's recorded
// implementations of the same port — dependency-map v1.19 records this
// boundary and os/exec stays out of this package (rule 4: OS-specific APIs
// are confined to internal/platform; plain os.Rename/os.Link are portable
// standard-library calls, not the confined syscall/exec surface).

import (
	"context"
	"fmt"
	"io"
	"os"
)

// RenameActivator is the Unix Mode-A activator: copy the current binary to
// the retain path (the last-known-good anchor), fsync it, then rename the
// staged candidate onto the install path.
type RenameActivator struct{}

// Swap implements Activator.Swap.
func (RenameActivator) Swap(_ context.Context, req SwapRequest) error {
	if err := retainCurrent(req.InstallPath, req.RetainPath); err != nil {
		return fmt.Errorf("updater: retain the current binary: %w", err)
	}
	if err := os.Rename(req.StagedPath, req.InstallPath); err != nil {
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
	return nil
}

// Restore implements Activator.Restore: the retained prior binary goes back
// onto the install path — BINARY ONLY (C5-18 §5.3's separation rule; user
// data is never touched by this machine).
func (RenameActivator) Restore(_ context.Context, retainPath, installPath string) error {
	if _, err := os.Stat(retainPath); err != nil {
		return fmt.Errorf("updater: the retained prior binary is unavailable: %w", err)
	}
	if err := os.Rename(retainPath, installPath); err != nil {
		return fmt.Errorf("updater: restore the prior binary: %w", err)
	}
	return nil
}

// retainCurrent copies the current binary to the retain path with fsync (a
// plain copy — the anchor must be a durable, independent copy, not a link
// that a rename could move).
func retainCurrent(installPath, retainPath string) error {
	src, err := os.Open(installPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(retainPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return nil
}
