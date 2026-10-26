//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveFamily implements the Windows row of the C2-06 path map through the
// platform-directory APIs exposed by the Go standard library:
// os.UserConfigDir (%RoamingAppData%, backed by SHGetKnownFolderPath with
// FOLDERID_RoamingAppData) and %LocalAppData% (FOLDERID_LocalAppData) for
// cache/logs/temp/runtime, with os.TempDir as the GetTempPath fallback.
func resolveFamily() (DirSet, error) {
	roaming, err := os.UserConfigDir()
	if err != nil {
		return DirSet{}, fmt.Errorf("platform: roaming app-data directory: %w", err)
	}
	local := os.Getenv("LocalAppData")
	if local == "" {
		return DirSet{}, fmt.Errorf("platform: LocalAppData is not set")
	}
	return DirSet{
		Data:    filepath.Join(roaming, "Karyora", "data"),
		Config:  filepath.Join(roaming, "Karyora", "config"),
		Cache:   filepath.Join(local, "Karyora", "cache"),
		State:   filepath.Join(local, "Karyora", "logs"),
		Temp:    filepath.Join(local, "Karyora", "tmp"),
		Runtime: filepath.Join(local, "Karyora", "runtime"),
	}, nil
}
