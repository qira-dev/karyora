package platform

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local IPC (C2-08 §5): the endpoint exists only while the ownership lock is
// held, lives in the per-user runtime namespace, and authenticates every
// request with the per-instance capability token; the POSIX server
// additionally verifies the peer OS user. The IPC surface carries only
// lifecycle/status/diagnostic operations — it is never a data-plane API.
// This file holds the transport-neutral protocol; the family adapters are in
// the build-tagged files.

// IPC request/response schema: stable English keys, bounded sizes.
const (
	IPCOpStatus = "status"
	IPCOpStop   = "stop"

	// ipcDeadline is the bounded per-request budget (Proposed, C2-05
	// finite-bounds rule); a request that exceeds it fails closed.
	ipcDeadline = 5 * time.Second

	// maxIPCMessageBytes bounds one newline-delimited message.
	maxIPCMessageBytes = 64 * 1024

	// stopResponseGrace extends the connection deadline past the drain
	// budget for stop requests.
	stopResponseGrace = 30 * time.Second
)

// IPCRequest is one authenticated lifecycle request.
type IPCRequest struct {
	Op           string `json:"op"`
	InstanceUUID string `json:"instanceUUID"`
	Token        string `json:"token"`
}

// IPCResponse carries one bounded machine result. Body is pre-encoded JSON
// from the caller (a status report or a drain outcome); Code is a stable
// machine error identifier for the denied/failed cases.
type IPCResponse struct {
	OK   bool            `json:"ok"`
	Code string          `json:"code,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

// ErrIPCUnreachable reports that no live owner answered the endpoint (the
// stable basis for the idempotent "not running" outcome); other errors are
// infrastructure failures (EXIT_AUTHORITY at the runtime boundary).
var ErrIPCUnreachable = errors.New("platform: no live Core answered the IPC endpoint")

// ServeIPC serves lifecycle requests until the listener closes. verify
// applies any additional per-operation authorization and returns a stable
// denial code; handle executes one authorized operation.
func ServeIPC(ln net.Listener, rec OwnerRecord, token []byte, verify func(req IPCRequest) (allowed bool, code string), handle func(req IPCRequest) IPCResponse) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(ipcDeadline))
			if !peerAllowed(c) {
				writeIPCResponse(c, IPCResponse{OK: false, Code: "AUTHZ_DENIED"})
				return
			}
			req, err := readIPCRequest(c)
			if err != nil {
				writeIPCResponse(c, IPCResponse{OK: false, Code: "VALIDATION_FAILED"})
				return
			}
			if req.Op == IPCOpStop {
				// A graceful stop waits for the bounded drain; the request
				// deadline extends past the drain budget so the outcome can
				// still reach the client (C2-07 §2).
				_ = c.SetDeadline(time.Now().Add(ipcDeadline + stopResponseGrace))
			}
			if !authorizePayload(c, rec, token, req, verify) {
				return
			}
			writeIPCResponse(c, handle(req))
		}(conn)
	}
}

// peerAllowed runs the family-specific peer credential check before any
// protocol byte is accepted (POSIX SO_PEERCRED; on Windows the check is nil
// and the per-instance token below is the client verification).
func peerAllowed(c net.Conn) bool {
	if peerAuthorized == nil {
		return true
	}
	return peerAuthorized(c)
}

// authorizePayload checks the shared per-instance capability: the instance
// UUID and token must both match the live owner; anything else is
// AUTHZ_DENIED — untrusted local clients cannot claim authority, query
// secrets, or stop the Core (C2-08 §5).
func authorizePayload(c net.Conn, rec OwnerRecord, token []byte, req IPCRequest, verify func(IPCRequest) (bool, string)) bool {
	// The capability travels hex-encoded, matching the 0600 capability file
	// contents clients read (C2-08 §5).
	if req.InstanceUUID != rec.InstanceUUID || req.Token == "" || req.Token != hex.EncodeToString(token) {
		writeIPCResponse(c, IPCResponse{OK: false, Code: "AUTHZ_DENIED"})
		return false
	}
	if verify != nil {
		if allowed, code := verify(req); !allowed {
			writeIPCResponse(c, IPCResponse{OK: false, Code: code})
			return false
		}
	}
	return true
}

func readIPCRequest(c net.Conn) (IPCRequest, error) {
	var req IPCRequest
	r := bufio.NewReaderSize(c, maxIPCMessageBytes)
	line, err := readBoundedLine(r, maxIPCMessageBytes)
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return req, fmt.Errorf("platform: ipc request: %w", err)
	}
	return req, nil
}

func readBoundedLine(r *bufio.Reader, limit int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > limit {
			return nil, fmt.Errorf("platform: ipc message exceeds %d bytes", limit)
		}
		if err == nil {
			break
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return nil, err
	}
	return out, nil
}

func writeIPCResponse(c net.Conn, resp IPCResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = c.Write(append(b, '\n'))
}

// CallIPC sends one authenticated request to the live owner and returns the
// bounded response. No live endpoint answers with ErrIPCUnreachable.
func CallIPC(runtimeDir, op, instanceUUID, token string) (IPCResponse, error) {
	if instanceUUID == "" || token == "" {
		return IPCResponse{}, ErrIPCUnreachable
	}
	conn, err := dialFamily(runtimeDir)
	if err != nil {
		return IPCResponse{}, ErrIPCUnreachable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ipcDeadline))
	req, err := json.Marshal(IPCRequest{Op: op, InstanceUUID: instanceUUID, Token: token})
	if err != nil {
		return IPCResponse{}, err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return IPCResponse{}, fmt.Errorf("platform: ipc write: %w", err)
	}
	line, err := readBoundedLine(bufio.NewReaderSize(conn, maxIPCMessageBytes), maxIPCMessageBytes)
	if err != nil {
		return IPCResponse{}, fmt.Errorf("platform: ipc read: %w", err)
	}
	var resp IPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return IPCResponse{}, fmt.Errorf("platform: ipc response: %w", err)
	}
	return resp, nil
}

// OwnerCredentials loads the recorded owner identity and capability token
// from the runtime namespace for an IPC client. A missing record or token
// means no live owner is knowable — callers treat it as "not running".
func OwnerCredentials(runtimeDir string) (rec OwnerRecord, token string, err error) {
	r, err := ReadOwnerRecord(runtimeDir)
	if err != nil {
		return OwnerRecord{}, "", err
	}
	b, err := os.ReadFile(filepath.Join(runtimeDir, r.IPCCapabilityFile))
	if err != nil {
		return *r, "", err
	}
	return *r, strings.TrimSpace(string(b)), nil
}
