//go:build windows

package platform

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx     = kernel32.NewProc("LockFileEx")
	procUnlockFileEx   = kernel32.NewProc("UnlockFileEx")
	procGetFileInfByHd = kernel32.NewProc("GetFileInformationByHandle")
	procGetProcessTime = kernel32.NewProc("GetProcessTimes")
)

const (
	lockfileExFailImmediately = 0x1
	lockfileExExclusiveLock   = 0x2
)

// acquireFamilyLock takes the exclusive byte-range lock (LockFileEx) on the
// ownership file for the process lifetime (C2-08 §2). A held lock maps to
// ErrLockHeld; any other failure is infrastructure.
func acquireFamilyLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	h := syscall.Handle(f.Fd())
	var overlapped syscall.Overlapped
	r1, _, errNo := procLockFileEx.Call(
		uintptr(h),
		uintptr(lockfileExFailImmediately|lockfileExExclusiveLock),
		0,          // reserved
		0xFFFFFFFF, // bytes low
		0x7FFFFFFF, // bytes high
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		f.Close()
		const ERROR_LOCK_VIOLATION = 0x21
		if errNo == syscall.Errno(ERROR_LOCK_VIOLATION) {
			return nil, ErrLockHeld
		}
		return nil, fmt.Errorf("LockFileEx %s: %w", path, errNo)
	}
	return f, nil
}

func (o *Ownership) releaseFamilyLock() error {
	if o.file == nil {
		return nil
	}
	h := syscall.Handle(o.file.Fd())
	var overlapped syscall.Overlapped
	procUnlockFileEx.Call(uintptr(h), 0, 0xFFFFFFFF, 0x7FFFFFFF, uintptr(unsafe.Pointer(&overlapped)))
	return o.file.Close()
}

// dataDirIdentity canonicalizes through an opened handle: the identity is
// (volume serial number, file index) from GetFileInformationByHandle,
// never a path string (C2-08 §1).
func dataDirIdentity(dir string) (DirIdentity, error) {
	f, err := os.Open(dir)
	if err != nil {
		return DirIdentity{}, fmt.Errorf("open data directory %s: %w", dir, err)
	}
	defer f.Close()
	type fileInfo struct {
		VolumeSerialNumber uint32
		FileIndexHigh      uint32
		FileIndexLow       uint32
	}
	var fi fileInfo
	h := syscall.Handle(f.Fd())
	r1, _, errNo := procGetFileInfByHd.Call(uintptr(h), uintptr(unsafe.Pointer(&fi)))
	if r1 == 0 {
		return DirIdentity{}, fmt.Errorf("GetFileInformationByHandle %s: %w", dir, errNo)
	}
	ino := uint64(fi.FileIndexHigh)<<32 | uint64(fi.FileIndexLow)
	return DirIdentity{Dev: uint64(fi.VolumeSerialNumber), Ino: ino}, nil
}

// hostIdentity returns the process creation time from GetProcessTimes; the
// Windows boot identifier is not applicable (n/a per C2-08 §3) — the kernel
// lock covers liveness.
func hostIdentity() (startTime, bootID string) {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return "", ""
	}
	var creation, exit, kernel, user syscall.Filetime
	r1, _, _ := procGetProcessTime.Call(uintptr(h),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)))
	if r1 == 0 {
		return "", ""
	}
	return strconv.FormatInt(creation.Nanoseconds(), 10), ""
}
