//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveFamily implements the macOS row of the C2-06 path map through the
// platform-directory APIs exposed by the Go standard library:
// os.UserConfigDir (~/Library/Application Support, backed by
// NSSearchPathForDirectoriesInDomains), os.UserCacheDir (~/Library/Caches),
// and os.TempDir (the per-user temp directory derived from
// confstr(_CS_DARWIN_USER_TEMP_DIR)).
func resolveFamily() (DirSet, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return DirSet{}, fmt.Errorf("platform: config directory: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return DirSet{}, fmt.Errorf("platform: cache directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return DirSet{}, fmt.Errorf("platform: user home: %w", err)
	}
	temp := os.TempDir()
	return DirSet{
		Data:    filepath.Join(config, "Karyora", "data"),
		Config:  filepath.Join(config, "Karyora", "config"),
		Cache:   filepath.Join(cache, "Karyora"),
		State:   filepath.Join(home, "Library", "Logs", "Karyora"),
		Temp:    filepath.Join(temp, "Karyora", "tmp"),
		Runtime: filepath.Join(temp, "Karyora", "runtime"),
	}, nil
}
