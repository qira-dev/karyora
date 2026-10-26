//go:build darwin

package platform

import (
	"net"
	"os"
	"path/filepath"
)

// peerAuthorized is nil on darwin: SO_PEERCRED is unavailable. The user
// boundary is enforced by the socket file's 0600 mode at the filesystem
// level, and every request still carries the per-instance capability token
// (C2-08 §5); tightening the peer check with the local user check is
// recorded per-target evidence work, not a Phase 2 runtime claim.
var peerAuthorized func(net.Conn) bool

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
