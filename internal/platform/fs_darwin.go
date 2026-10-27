//go:build darwin

package platform

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// IsNetworkFilesystem reports whether path lives on a detected network
// filesystem: statfs flags without MNT_LOCAL (C2-06 §5, locked D-08). The
// flag comes from x/sys/unix because the standard-library syscall package
// for darwin does not export MNT_LOCAL.
func IsNetworkFilesystem(path string) (bool, string, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, "", fmt.Errorf("platform: statfs %s: %w", path, err)
	}
	if st.Flags&unix.MNT_LOCAL == 0 {
		return true, "remote-mount", nil
	}
	return false, "", nil
}
