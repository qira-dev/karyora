package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestJobSubmitVerticalSlice is the C3-31 CLI leg: the same app.SliceService
// functions the API tests exercise run behind the CLI flags, the Job
// succeeds WITHOUT KnowledgePack or Work (D-12), and the report carries the
// result, receipt, and event sequence. The status probe then reads the SAME
// durable state back read-only.
func TestJobSubmitVerticalSlice(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "Explain the recorded fixture slice.",
		"--idempotency-key", "cli-1", "--json")
	if code != 0 {
		t.Fatalf("job submit exited %d: %s", code, out)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, out)
	}
	if report["state"] != "succeeded" {
		t.Fatalf("job did not succeed: %v", report)
	}
	if report["receipt_id"] == "" || report["result_digest"] == "" {
		t.Fatalf("report lacks the receipt/result provenance: %v", report)
	}
	events, _ := report["events"].([]any)
	if len(events) < 3 {
		t.Fatalf("event sequence too short: %v", events)
	}
	joined := ""
	for _, e := range events {
		joined += e.(string) + ","
	}
	for _, want := range []string{"job.submitted", "job.claimed", "job.succeeded"} {
		if !strings.Contains(joined, want) {
			t.Errorf("event sequence missing %q: %v", want, events)
		}
	}

	// Idempotent resubmission replays the SAME job (C3-02 through the CLI).
	code2, out2 := h.run("job", "submit", "--task", "Explain the recorded fixture slice.",
		"--idempotency-key", "cli-1", "--json")
	if code2 != 0 {
		t.Fatalf("replay exited %d: %s", code2, out2)
	}
	var replay map[string]any
	_ = json.Unmarshal([]byte(out2), &replay)
	if replay["job_id"] != report["job_id"] {
		t.Fatalf("same key + same input must replay the original job: %v vs %v", replay["job_id"], report["job_id"])
	}

	// Read-only status probe reads the SAME durable state back.
	code3, out3 := h.run("job", "status", report["job_id"].(string), "--json")
	if code3 != 0 {
		t.Fatalf("status exited %d: %s", code3, out3)
	}
	var probe struct {
		Job struct {
			State        string `json:"state"`
			ReceiptID    string `json:"receipt_id"`
			ResultDigest string `json:"result_digest"`
		} `json:"job"`
		Events []struct {
			EventType string `json:"event_type"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out3), &probe); err != nil {
		t.Fatalf("status report is not JSON: %v\n%s", err, out3)
	}
	if probe.Job.State != "succeeded" || probe.Job.ReceiptID != report["receipt_id"] {
		t.Fatalf("status probe disagrees with the submit report: %+v vs %v", probe.Job, report)
	}
	if len(probe.Events) != len(events) {
		t.Fatalf("status probe sees %d events, submit reported %d", len(probe.Events), len(events))
	}

	// Job events probe reads only the journal events array.
	code4, out4 := h.run("job", "events", report["job_id"].(string), "--json")
	if code4 != 0 {
		t.Fatalf("events exited %d: %s", code4, out4)
	}
	var eventsOnly []struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal([]byte(out4), &eventsOnly); err != nil {
		t.Fatalf("events report is not JSON array: %v\n%s", err, out4)
	}
	if len(eventsOnly) != len(events) {
		t.Fatalf("events command returned %d events, expected %d", len(eventsOnly), len(events))
	}
}

// TestJobSubmitConflictIsTyped proves the idempotency conflict through the
// CLI: the same key with a different task is refused (CONFLICT_STATE), and
// the process exits non-zero without producing a second job.
func TestJobSubmitConflictIsTyped(t *testing.T) {
	h := newHarness(t)
	if code, out := h.run("job", "submit", "--task", "One task.", "--idempotency-key", "cli-conflict", "--json"); code != 0 {
		t.Fatalf("first submit exited %d: %s", code, out)
	}
	code, out := h.run("job", "submit", "--task", "A different task.", "--idempotency-key", "cli-conflict", "--json")
	if code == 0 {
		t.Fatal("a same-key different-input submit must exit non-zero")
	}
	if !strings.Contains(out, "CONFLICT_STATE") {
		t.Fatalf("conflict must be typed, got %s", out)
	}
}

// TestJobSubmitUnknownProviderRefused proves the provider boundary through
// the CLI: a provider string that matches NO wired adapter is refused as
// unwired. The eight direct integrations ARE wired (2026-10-28 wiring
// milestone); the gate and credential boundary decide their submissions.
func TestJobSubmitUnknownProviderRefused(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-p", "--provider", "no-such-provider", "--json")
	if code == 0 {
		t.Fatal("a provider with no wired adapter must be refused")
	}
	if !strings.Contains(out, "VALIDATION_FAILED") {
		t.Fatalf("refusal must be typed, got %s", out)
	}
}

// TestJobSubmitRealProviderUnlabeledBlocksEgress proves the accepted
// egress-classification policy through the CLI: a real provider submission
// WITHOUT a classification label is protected-unlabeled and fails the C3-19
// gate terminally — even though the provider adapter is wired.
func TestJobSubmitRealProviderUnlabeledBlocksEgress(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-egress", "--provider", "openai", "--json")
	if code == 0 {
		t.Fatal("an unlabeled real-provider submission must fail")
	}
	if !strings.Contains(out, "egress") {
		t.Fatalf("the failure must name the egress policy gate, got %s", out)
	}
}

// TestJobSubmitZaiNonPersonalPassesGateFailsOnCredential proves BOTH halves
// of the wiring: the activated Z.AI profile passes the C3-19 gate for
// `non-personal` (the sole eligible provider/class pair), and the run then
// fails typed at the credential boundary because nothing is stored in the
// OS keyring — no live call is possible without a credential (D-37).
func TestJobSubmitZaiNonPersonalPassesGateFailsOnCredential(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-zai",
		"--provider", "glm-zai", "--classification", "non-personal", "--json")
	if code == 0 {
		t.Fatal("without a stored credential the run must fail")
	}
	if strings.Contains(out, "egress") {
		t.Fatalf("the egress gate must PASS for Z.AI x non-personal, got %s", out)
	}
	if !strings.Contains(out, "credential unavailable") {
		t.Fatalf("the failure must come from the credential boundary, got %s", out)
	}
}

// TestJobSubmitZaiPersonalNeverEgresses proves the personal-data rule: even
// on the only eligible provider, a `personal` label is refused by the gate
// (v1: personal data never egresses to a third-party cloud).
func TestJobSubmitZaiPersonalNeverEgresses(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-personal",
		"--provider", "glm-zai", "--classification", "personal", "--json")
	if code == 0 {
		t.Fatal("personal data must never egress in v1")
	}
	if !strings.Contains(out, "egress") {
		t.Fatalf("the failure must name the egress policy gate, got %s", out)
	}
}

// TestJobSubmitWithRunningCore verifies that uawp job submit succeeds even when
// a core daemon instance is already holding the platform data lock.
func TestJobSubmitWithRunningCore(t *testing.T) {
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

	code, out := h.run("job", "submit", "--task", "Explain the running daemon vertical slice.",
		"--idempotency-key", "cli-daemon-1", "--provider", "mock", "--json")
	if code != 0 {
		t.Fatalf("job submit with running core exited %d: %s", code, out)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, out)
	}
	if report["state"] != "succeeded" {
		t.Fatalf("job state = %v, want succeeded", report["state"])
	}

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("start did not exit after cancel")
	}
}
