//go:build darwin

package platform

import (
	"fmt"
	"syscall"
)

// IsNetworkFilesystem reports whether path lives on a detected network
// filesystem: statfs flags without MNT_LOCAL (C2-06 §5, locked D-08).
func IsNetworkFilesystem(path string) (bool, string, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, "", fmt.Errorf("platform: statfs %s: %w", path, err)
	}
	if st.Flags&syscall.MNT_LOCAL == 0 {
		return true, "remote-mount", nil
	}
	return false, "", nil
}
