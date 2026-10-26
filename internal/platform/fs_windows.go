//go:build windows

package platform

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procGetDriveTypeW = kernel32.NewProc("GetDriveTypeW")

const driveRemote = 4 // DRIVE_REMOTE

// IsNetworkFilesystem reports whether path lives on a network drive:
// GetDriveTypeW == DRIVE_REMOTE (C2-06 §5, locked D-08).
func IsNetworkFilesystem(path string) (bool, string, error) {
	root := volumeRoot(path)
	ptr, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return false, "", fmt.Errorf("platform: path %s: %w", path, err)
	}
	r1, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(ptr)))
	if r1 == uintptr(driveRemote) {
		return true, "remote-drive", nil
	}
	return false, "", nil
}

func volumeRoot(path string) string {
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return path[:3]
	}
	if len(path) >= 2 && path[1] == ':' {
		return path[:2] + `\`
	}
	return path
}
