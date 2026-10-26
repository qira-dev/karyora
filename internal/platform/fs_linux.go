//go:build linux

package platform

import (
	"fmt"
	"syscall"
)

// networkFsTypes are the Linux statfs f_type values treated as network
// filesystems (C2-06 §5, locked D-08: a live WAL/SHM on a network mount is
// refused at open, never degraded around).
var networkFsTypes = map[int64]string{
	0x6969:     "nfs",   // NFS_SUPER_MAGIC
	0xFF534D:   "cifs",  // CIFS_SUPER_MAGIC (SMB)
	0xFE534D:   "smb2",  // SMB2_SUPER_MAGIC
	0x5346544E: "ncpfs", // NCP_SUPER_MAGIC
	0x5346414F: "afs",   // AFS_SUPER_MAGIC
	0x01021997: "9p",    // V9FS_MAGIC
}

// IsNetworkFilesystem reports whether path lives on a detected network
// filesystem. The detection is a heuristic per C2-06 §5 (statfs f_type).
func IsNetworkFilesystem(path string) (bool, string, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, "", fmt.Errorf("platform: statfs %s: %w", path, err)
	}
	if name, ok := networkFsTypes[int64(st.Type)]; ok {
		return true, name, nil
	}
	return false, "", nil
}
