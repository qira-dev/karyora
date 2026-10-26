// Package platform is the only package allowed to touch OS-specific APIs:
// per-OS-family path resolution (C2-06), the data-directory ownership lock
// and owner record (C2-08), and the local IPC channel (C2-08 §5). Domain and
// application code never composes these paths; they receive them resolved.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirSet carries the C2-06 storage-class roots for one user. Every class is
// resolved through the family adapter; no caller composes these paths from
// home-directory strings.
type DirSet struct {
	Data    string // user data: DB/WAL/SHM, CAS, backups
	Config  string // configuration files
	Cache   string // rebuildable cache and update staging
	State   string // logs and append-only state
	Temp    string // ephemeral work files
	Runtime string // ephemeral lock/IPC namespace (per boot)
}

// Overrides let the composition root (and tests) redirect the data class and
// the ephemeral runtime class explicitly; every other class stays at its
// platform-resolved location. An override is an operator-level explicit
// choice (C2-05 "explicit CLI flags" layer), never silently derived.
type Overrides struct {
	DataDir    string
	RuntimeDir string
}

// Stable file names inside the runtime namespace. The kernel lock, the IPC
// endpoint, and the capability token live only here (C2-06 runtime class).
const (
	// LockFileName is the dedicated kernel-lock target. It is created once
	// and never renamed or replaced: the lock lives on this file's open
	// file description for the owner's lifetime, so an atomic record
	// rewrite must never replace this inode.
	LockFileName = "core.lock"
	// OwnerRecordName is the owner record (machine JSON) rewritten
	// atomically (temp + rename) under the held lock (C2-08 §3/§4).
	OwnerRecordName = "core.owner.json"
	IPCSocketName   = "core.sock"
	// CapabilityTokenID is the per-instance IPC capability reference.
	CapabilityTokenID = "core.cap"
)

// Resolve returns the storage-class roots for the running OS family. The
// family adapters are in the build-tagged files of this package.
func Resolve(over *Overrides) (DirSet, error) {
	ds, err := resolveFamily()
	if err != nil {
		return DirSet{}, err
	}
	if over != nil {
		if over.DataDir != "" {
			abs, err := filepath.Abs(over.DataDir)
			if err != nil {
				return DirSet{}, fmt.Errorf("platform: data-dir override: %w", err)
			}
			ds.Data = abs
		}
		if over.RuntimeDir != "" {
			abs, err := filepath.Abs(over.RuntimeDir)
			if err != nil {
				return DirSet{}, fmt.Errorf("platform: runtime-dir override: %w", err)
			}
			ds.Runtime = abs
		}
	}
	if strings.TrimSpace(ds.Data) == "" || strings.TrimSpace(ds.Runtime) == "" {
		return DirSet{}, fmt.Errorf("platform: data and runtime classes must resolve")
	}
	return ds, nil
}

// EnsureDir creates dir with the explicit mode, creating intermediate
// components itself with no-replace, no-follow semantics (C2-06 §4): an
// existing component that is a symlink (or any non-directory) is refused,
// never followed. The umask is never relied on for sensitive paths.
func EnsureDir(dir string, mode os.FileMode) error {
	if dir == "" {
		return fmt.Errorf("platform: empty directory path")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("platform: resolve %s: %w", dir, err)
	}
	parent := abs
	var create []string
	for {
		if parent == filepath.Clean(parent+string(filepath.Separator)+"..") || parent == string(filepath.Separator) {
			break
		}
		st, err := os.Lstat(parent)
		if err == nil {
			if !st.IsDir() {
				return fmt.Errorf("platform: %s exists and is not a directory", parent)
			}
			if st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("platform: %s is a symlink; refusing sensitive path", parent)
			}
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("platform: inspect %s: %w", parent, err)
		}
		create = append(create, parent)
		up := filepath.Dir(parent)
		if up == parent {
			break
		}
		parent = up
	}
	for i := len(create) - 1; i >= 0; i-- {
		if err := os.Mkdir(create[i], mode); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("platform: %s appeared during creation", create[i])
			}
			return fmt.Errorf("platform: create %s: %w", create[i], err)
		}
	}
	// Read the mode back and enforce it: on POSIX the explicit mode already
	// applies to the directories this call created; a pre-existing directory
	// keeps its mode (renaming it is not this function's business), but the
	// final component of a fresh creation is verified.
	if len(create) > 0 {
		st, err := os.Stat(create[0])
		if err != nil {
			return fmt.Errorf("platform: verify %s: %w", create[0], err)
		}
		if st.Mode().Perm()&0o077 != 0 && st.Mode().Perm() == mode.Perm() {
			return fmt.Errorf("platform: %s has broader permissions than %o", create[0], mode)
		}
	}
	return nil
}

// EnsureLayout creates the class directories a Phase 2 Core needs with the
// least-privilege posture (C2-06 §3): data/config/state/runtime 0700.
func EnsureLayout(ds DirSet) error {
	for _, d := range []struct {
		path string
		mode os.FileMode
	}{
		{ds.Data, 0o700},
		{ds.Config, 0o700},
		{ds.State, 0o700},
		{ds.Runtime, 0o700},
	} {
		if err := EnsureDir(d.path, d.mode); err != nil {
			return err
		}
	}
	return nil
}
