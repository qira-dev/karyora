package platform

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ipcHarness starts a live ownership + IPC endpoint on temp directories.
func ipcHarness(t *testing.T) (DirSet, OwnerRecord, []byte) {
	t.Helper()
	tmp := t.TempDir()
	ds := DirSet{Data: tmp + "/data", Runtime: tmp + "/runtime"}
	if err := os.MkdirAll(ds.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	own, _, err := AcquireOwnership(ds, ds.Data, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { own.Release() })
	return ds, own.Record, own.Capability()
}

func TestIPCRoundTripAndDenial(t *testing.T) {
	ds, rec, token := ipcHarness(t)
	ln, err := ListenIPC(ds)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	served := make(chan error, 1)
	go func() {
		served <- ServeIPC(ln, rec, token, func(req IPCRequest) (bool, string) {
			if req.Op == IPCOpStatus || req.Op == IPCOpStop {
				return true, ""
			}
			return false, "NOT_FOUND"
		}, func(req IPCRequest) IPCResponse {
			body, _ := json.Marshal(map[string]string{"echo": req.Op})
			return IPCResponse{OK: true, Body: body}
		})
	}()

	// Authorized round trip (the token travels hex-encoded, matching the
	// capability file contents).
	tokenHex := hex.EncodeToString(token)
	resp, err := CallIPC(ds.Runtime, IPCOpStatus, rec.InstanceUUID, tokenHex)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("authorized status must pass: %+v", resp)
	}
	var body map[string]string
	if err := json.Unmarshal(resp.Body, &body); err != nil || body["echo"] != IPCOpStatus {
		t.Fatalf("echo body mismatch: %s (%v)", resp.Body, err)
	}

	// Wrong token: AUTHZ_DENIED — untrusted local clients cannot claim
	// authority (C2-08 §5).
	resp, err = CallIPC(ds.Runtime, IPCOpStatus, rec.InstanceUUID, "wrong-token")
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "AUTHZ_DENIED" {
		t.Fatalf("wrong token must be AUTHZ_DENIED: %+v", resp)
	}

	// Wrong instance UUID: AUTHZ_DENIED.
	resp, err = CallIPC(ds.Runtime, IPCOpStatus, "another-instance", tokenHex)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "AUTHZ_DENIED" {
		t.Fatalf("wrong instance must be AUTHZ_DENIED: %+v", resp)
	}

	// Unknown op: NOT_FOUND through verify.
	resp, err = CallIPC(ds.Runtime, "payload", rec.InstanceUUID, tokenHex)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "NOT_FOUND" {
		t.Fatalf("unknown op must be NOT_FOUND: %+v", resp)
	}

	// No live endpoint: ErrIPCUnreachable.
	if _, err := CallIPC(t.TempDir(), IPCOpStatus, rec.InstanceUUID, tokenHex); err != ErrIPCUnreachable {
		t.Fatalf("dead endpoint err = %v, want ErrIPCUnreachable", err)
	}
	_ = served
}

func TestOwnerCredentialsLoadsToken(t *testing.T) {
	ds, rec, token := ipcHarness(t)
	gotRec, gotToken, err := OwnerCredentials(ds.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if gotRec.InstanceUUID != rec.InstanceUUID {
		t.Fatalf("record mismatch: %+v vs %+v", gotRec, rec)
	}
	if gotToken != hex.EncodeToString(token) {
		t.Fatalf("token mismatch")
	}
	if strings.ContainsAny(gotToken, "\n\r") {
		t.Fatal("token must be a single hex line")
	}
}
