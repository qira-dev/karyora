package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSessionCommandsWhenNotRunning(t *testing.T) {
	h := newHarness(t)

	// Status when not running
	code, out := h.run("session", "status", "--json")
	if code != 0 {
		t.Fatalf("session status exited %d: %s", code, out)
	}
	if !strings.Contains(out, `"running": false`) {
		t.Fatalf("expected running: false, got %s", out)
	}

	// Token when not running fails cleanly
	code, out = h.run("session", "token", "--json")
	if code == 0 {
		t.Fatalf("session token when not running should exit non-zero")
	}
	if !strings.Contains(out, "CORE_NOT_RUNNING") {
		t.Fatalf("expected CORE_NOT_RUNNING error, got %s", out)
	}
}

func TestSessionTokenWithRunningCore(t *testing.T) {
	h := newHarness(t)
	t.Setenv("KARYORA_TRANSPORT_LOCAL_HTTP_ENABLED", "1")
	t.Setenv("KARYORA_TRANSPORT_LOCAL_HTTP_PORT", "0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		code int
		out  string
	}
	done := make(chan result, 1)
	go func() {
		code, out := h.runCtx(ctx, "start", "--json")
		done <- result{code, out}
	}()

	// Wait for the Core to serve its IPC endpoint.
	deadline := time.Now().Add(15 * time.Second)
	live := false
	for time.Now().Before(deadline) {
		code, out := h.run("status", "--json")
		if code == 0 && strings.Contains(out, `"live": true`) {
			live = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !live {
		cancel()
		t.Fatal("start never became live")
	}

	code, out := h.run("session", "status", "--json")
	if code != 0 || !strings.Contains(out, `"running": true`) {
		t.Fatalf("session status with running core: code=%d, out=%s", code, out)
	}

	code, out = h.run("session", "token", "--json")
	if code != 0 {
		t.Fatalf("session token with running core failed: code=%d, out=%s", code, out)
	}
	if !strings.Contains(out, "bootstrap-") {
		t.Fatalf("expected bootstrap- token in json output, got: %s", out)
	}

	// Stop cleanly and wait for start goroutine to exit
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("start did not exit after cancel")
	}
}
