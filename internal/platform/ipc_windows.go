//go:build windows

package platform

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// peerAuthorized is nil on Windows: the per-instance capability token is the
// client verification (C2-08 §5 "client-token verification"); the per-user
// pipe namespace scopes the endpoint.
var peerAuthorized func(net.Conn) bool

// ipcPipeName is the named pipe in the per-user namespace
// (\\.\pipe\karyora\<user>\core). The pipe name is bounded and sanitized;
// tightening the pipe ACL to the owner SID is recorded per-target evidence
// work (D-06 matrix), not a Phase 2 runtime claim.
func ipcPipeName(runtimeDir string) string {
	user := sanitizePipeSegment(os.Getenv("USERNAME"))
	if user == "" {
		user = "default"
	}
	return fmt.Sprintf(`\\.\pipe\karyora\%s\core`, user)
}

func sanitizePipeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// ListenIPC creates the named pipe server via the standard library's
// unix-listener-over-named-pipes support on Windows.
func ListenIPC(ds DirSet) (net.Listener, error) {
	_ = filepath.Join // keep filepath import parity with the family files
	return net.Listen("unix", ipcPipeName(ds.Runtime))
}

// dialFamily connects to the live owner's named pipe.
func dialFamily(runtimeDir string) (net.Conn, error) {
	return net.DialTimeout("unix", ipcPipeName(runtimeDir), ipcDeadline)
}
