package platform

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Ownership errors map to the C2-05/C2-08 taxonomy at the runtime boundary:
// ErrLockHeld means another authority owns the data directory
// (SINGLE_INSTANCE_CONFLICT); ErrLockInfrastructure means the lock/IPC
// substrate itself failed (EXIT_AUTHORITY). File existence, PID, or file age
// is never liveness proof — the kernel lock is the only authority (C2-08).
var (
	ErrLockHeld           = errors.New("platform: another process holds the data-directory lock")
	ErrLockInfrastructure = errors.New("platform: lock/IPC infrastructure failure")
	ErrOwnerRecordMissing = errors.New("platform: no owner record present")
)

// OwnerRecord is the machine-readable owner diagnostic written into the
// ownership file (stable English JSON keys). Process start time and boot ID
// defeat PID reuse during diagnostics; the kernel lock stays the liveness
// authority. Fields a family cannot obtain are recorded as empty strings so
// the schema stays stable across targets.
type OwnerRecord struct {
	InstanceUUID      string `json:"instanceUUID"`
	PID               int    `json:"pid"`
	ProcessStartTime  string `json:"processStartTime"`
	BootID            string `json:"bootID"`
	CoreVersion       string `json:"coreVersion"`
	DataDirIdentity   string `json:"dataDirIdentity"`
	IPCCapabilityFile string `json:"ipcCapabilityFile"`
	AcquiredAt        string `json:"acquiredAt"`
}

// DirIdentity is the canonical data-directory identity: POSIX (device,
// inode) or Windows (volume serial, file index). Alias spellings, symlinks,
// and case variants of one directory resolve to one identity; two genuinely
// different directories never collide (C2-08 §1).
type DirIdentity struct {
	Dev uint64
	Ino uint64
}

// Digest returns the stable identity digest recorded in the owner record.
func (d DirIdentity) Digest() string {
	return fmt.Sprintf("%d:%d", d.Dev, d.Ino)
}

// Ownership is a held exclusive kernel lock on the canonical data directory
// plus the owner record written into the runtime namespace.
type Ownership struct {
	runtime    DirSet
	file       *os.File
	Record     OwnerRecord
	identity   DirIdentity
	capability []byte
	released   bool
}

// AcquireOwnership takes the exclusive kernel lock for dataDir, writing the
// owner record into the runtime namespace. tookOver reports that an owner
// record was present while the kernel lock was free — a stale takeover that
// the caller must treat as a recovery event (C2-08 §4). The lock is held for
// the process lifetime; liveness is the lock itself.
func AcquireOwnership(ds DirSet, dataDir, coreVersion string) (own *Ownership, tookOver bool, err error) {
	if err := EnsureDir(ds.Runtime, 0o700); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	// First start creates the data directory itself with no-follow
	// semantics (C2-06 §4) before the canonical identity is computed.
	if err := EnsureDir(dataDir, 0o700); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	identity, err := dataDirIdentity(dataDir)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	// The lock is taken on the dedicated core.lock file, which is never
	// renamed or replaced while the protocol runs; the owner record is a
	// separate file rewritten atomically under the held lock (C2-08 §2–§4).
	lockPath := filepath.Join(ds.Runtime, LockFileName)
	prior, _ := ReadOwnerRecord(ds.Runtime)
	file, err := acquireFamilyLock(lockPath)
	if err != nil {
		if errors.Is(err, ErrLockHeld) {
			return nil, false, ErrLockHeld
		}
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	own = &Ownership{runtime: ds, file: file, identity: identity}
	token, err := randomToken(32)
	if err != nil {
		_ = own.releaseFamilyLock()
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	own.capability = token
	tokenPath := filepath.Join(ds.Runtime, CapabilityTokenID)
	if err := writeFileAtomic(tokenPath, []byte(hex.EncodeToString(token)), 0o600); err != nil {
		_ = own.releaseFamilyLock()
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	start, boot := hostIdentity()
	instance, err := newInstanceUUID()
	if err != nil {
		_ = own.releaseFamilyLock()
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	own.Record = OwnerRecord{
		InstanceUUID:      instance,
		PID:               os.Getpid(),
		ProcessStartTime:  start,
		BootID:            boot,
		CoreVersion:       coreVersion,
		DataDirIdentity:   identity.Digest(),
		IPCCapabilityFile: CapabilityTokenID,
		AcquiredAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := own.writeRecord(); err != nil {
		_ = own.releaseFamilyLock()
		return nil, false, fmt.Errorf("%w: %v", ErrLockInfrastructure, err)
	}
	tookOver = prior != nil
	return own, tookOver, nil
}

// writeRecord rewrites the owner record atomically (temp + rename inside the
// runtime directory), used at acquisition and at takeover. The record file is
// separate from the lock file so the rewrite can never drop the kernel lock.
func (o *Ownership) writeRecord() error {
	path := filepath.Join(o.runtime.Runtime, OwnerRecordName)
	b, err := json.Marshal(o.Record)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

// Capability returns the per-instance IPC capability token.
func (o *Ownership) Capability() []byte { return o.capability }

// Release lets go of the kernel lock and removes the per-instance capability
// file and owner record. The runtime namespace is per-boot ephemeral; removal
// keeps diagnostics honest for the next start.
func (o *Ownership) Release() error {
	if o.released {
		return nil
	}
	o.released = true
	_ = os.Remove(filepath.Join(o.runtime.Runtime, CapabilityTokenID))
	_ = os.Remove(filepath.Join(o.runtime.Runtime, OwnerRecordName))
	// core.lock stays on disk: file existence means nothing; the released
	// kernel lock is what makes the authority free (C2-08 §2).
	return o.releaseFamilyLock()
}

// ReadOwnerRecord returns the recorded owner diagnostics, or
// ErrOwnerRecordMissing when none is present. Record presence alone never
// proves liveness (C2-08 §3).
func ReadOwnerRecord(runtimeDir string) (*OwnerRecord, error) {
	b, err := os.ReadFile(filepath.Join(runtimeDir, OwnerRecordName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrOwnerRecordMissing
		}
		return nil, err
	}
	var rec OwnerRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, fmt.Errorf("platform: owner record unreadable: %w", err)
	}
	return &rec, nil
}

func newInstanceUUID() (string, error) {
	b, err := randomToken(16)
	if err != nil {
		// Fail closed: a record without an instance identity is not written.
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomToken(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// writeFileAtomic writes a file via temp + rename in the same directory with
// the explicit mode (C2-08 §4 atomic rewrite rule).
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".core-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
