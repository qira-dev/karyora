//go:build windows

package platform

import (
	"net"
	"os"
	"path/filepath"
)

// peerAuthorized is nil on Windows: SO_PEERCRED is unavailable. The user
// boundary is enforced by the socket file's 0600 mode inside the per-user
// runtime class, and every request still carries the per-instance
// capability token (C2-08 §5) — the same trust shape as the darwin family.
var peerAuthorized func(net.Conn) bool

// The endpoint is a REAL AF_UNIX socket file in the runtime class, NOT a
// \\.\pipe\ named-pipe address: Windows AF_UNIX (afunix.sys) binds
// filesystem paths, and binding the pipe namespace fails with WSAENETDEAD
// ("a socket operation encountered a dead network") — the C5-20 lifecycle
// evidence caught exactly that on the hosted Windows runners. The runtime
// class is the per-boot, per-user namespace (C2-06's class map), which
// scopes the endpoint to this user exactly as the pipe namespace did; the
// tightening to a pipe-ACL form remains recorded per-target evidence work
// (D-06 matrix), not a runtime claim.
//
// ListenIPC binds the domain socket in the runtime namespace. A stale
// socket file from a dead prior owner is removed first: the kernel lock is
// held at this point, so no live owner can own that name (C2-08 §5).
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
