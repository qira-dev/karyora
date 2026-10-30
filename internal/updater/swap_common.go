package updater

// The activator helpers shared by the per-OS implementations of the S7/S9
// ports (swap_unix.go, swap_windows.go): plain standard-library file calls
// only, per the same rationale recorded on swap_unix.go.

import (
	"io"
	"os"
)

// retainCurrent copies the current binary to the retain path with fsync (a
// plain copy — the last-known-good anchor must be a durable, independent
// copy, not a link that a rename could move).
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
