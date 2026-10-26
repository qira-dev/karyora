//go:build linux

package platform

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// peerAuthorized verifies the peer OS user equals this process's user via
// SO_PEERCRED before any protocol byte is accepted (C2-08 §5). The data
// directory is per-user; a cross-user client is AUTHZ_DENIED.
var peerAuthorized = func(c net.Conn) bool {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	allowed := false
	ctlErr := raw.Control(func(fd uintptr) {
		ucred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			return
		}
		allowed = ucred.Uid == uint32(os.Getuid())
	})
	return ctlErr == nil && allowed
}

// ListenIPC binds the domain socket in the runtime namespace. A stale socket
// file from a dead prior owner is removed first: the kernel lock is held at
// this point, so no live owner can own that name (C2-08 §5).
func ListenIPC(ds DirSet) (net.Listener, error) {
	path := filepath.Join(ds.Runtime, IPCSocketName)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Least privilege: only the owning user may connect at all.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// dialFamily connects to the live owner's endpoint.
func dialFamily(runtimeDir string) (net.Conn, error) {
	return net.DialTimeout("unix", filepath.Join(runtimeDir, IPCSocketName), ipcDeadline)
}
