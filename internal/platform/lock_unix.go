//go:build linux || darwin

package platform

import (
	"fmt"
	"os"
	"syscall"
)

// acquireFamilyLock takes the advisory kernel lock (flock LOCK_EX|LOCK_NB)
// on the ownership file for the process lifetime (C2-08 §2). EWOULDBLOCK
// maps to ErrLockHeld; any other failure is infrastructure.
func acquireFamilyLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN || err == syscall.EACCES {
			return nil, ErrLockHeld
		}
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return f, nil
}

func (o *Ownership) releaseFamilyLock() error {
	if o.file == nil {
		return nil
	}
	_ = syscall.Flock(int(o.file.Fd()), syscall.LOCK_UN)
	return o.file.Close()
}

// dataDirIdentity canonicalizes through an opened descriptor: the identity
// is (device, inode) from fstat, never a path string (C2-08 §1).
func dataDirIdentity(dir string) (DirIdentity, error) {
	f, err := os.Open(dir)
	if err != nil {
		return DirIdentity{}, fmt.Errorf("open data directory %s: %w", dir, err)
	}
	defer f.Close()
	var st syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		return DirIdentity{}, fmt.Errorf("fstat %s: %w", dir, err)
	}
	return DirIdentity{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
}
