//go:build linux

package platform

import (
	"fmt"
	"os"
	"strings"
)

// hostIdentity returns process start time and boot identifier for the owner
// record (C2-08 §3): Linux reads procfs. These defeat PID reuse during
// diagnostics; the kernel lock remains the liveness authority.
func hostIdentity() (startTime, bootID string) {
	if s, err := procStartTime(os.Getpid()); err == nil {
		startTime = s
	}
	if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		bootID = strings.TrimSpace(string(b))
	}
	return startTime, bootID
}

// procStartTime parses field 22 (starttime) of /proc/<pid>/stat: the clock
// ticks since boot at which the process started.
func procStartTime(pid int) (string, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	s := string(b)
	idx := strings.LastIndexByte(s, ')')
	if idx < 0 || idx+2 > len(s) {
		return "", fmt.Errorf("proc stat: unexpected format")
	}
	fields := strings.Fields(s[idx+2:])
	// fields[0] is state (field 3); starttime is field 22 → index 19.
	if len(fields) < 20 {
		return "", fmt.Errorf("proc stat: too few fields")
	}
	return fields[19], nil
}
