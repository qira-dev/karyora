//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// resolveFamily implements the Linux row of the C2-06 path map through the
// XDG Base Directory Specification: XDG_DATA_HOME / XDG_CONFIG_HOME /
// XDG_CACHE_HOME / XDG_STATE_HOME / XDG_RUNTIME_DIR with their documented
// defaults. The runtime class falls back to a bounded per-user directory
// under the system temp path when XDG_RUNTIME_DIR is unset.
func resolveFamily() (DirSet, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DirSet{}, fmt.Errorf("platform: user home: %w", err)
	}
	dataHome := envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	configHome := envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cacheHome := envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	stateHome := envOr("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		// Documented fallback (C2-06 Linux row): a bounded runtime
		// directory per user outside the per-boot tmpfs location.
		runtime = filepath.Join(os.TempDir(), "karyora-runtime-"+strconv.Itoa(os.Getuid()))
	}
	return DirSet{
		Data:    filepath.Join(dataHome, "karyora", "data"),
		Config:  filepath.Join(configHome, "karyora"),
		Cache:   filepath.Join(cacheHome, "karyora"),
		State:   filepath.Join(stateHome, "karyora"),
		Temp:    filepath.Join(cacheHome, "karyora", "tmp"),
		Runtime: filepath.Join(runtime, "karyora"),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
