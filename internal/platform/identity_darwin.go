//go:build darwin

package platform

import "syscall"

// hostIdentity returns the owner-record process identity on darwin: the boot
// session comes from sysctl(kern.bootsessionuuid) when present. Process
// start time is not obtainable without the private proc_pidinfo API and is
// recorded empty — the kernel lock remains the liveness authority (C2-08).
func hostIdentity() (startTime, bootID string) {
	if b, err := syscall.Sysctl("kern.bootsessionuuid"); err == nil {
		bootID = b
	}
	return "", bootID
}
