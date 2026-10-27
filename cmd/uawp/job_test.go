package main

import (
	"encoding/json"
	"strings"
	"testing"
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

// TestJobSubmitUnwiredProviderRefused proves the provider boundary through
// the CLI: only the recorded fixture provider is wired while D-37 is open.
func TestJobSubmitUnwiredProviderRefused(t *testing.T) {
	h := newHarness(t)
	code, out := h.run("job", "submit", "--task", "x", "--idempotency-key", "cli-p", "--provider", "openai", "--json")
	if code == 0 {
		t.Fatal("a non-fixture provider must be refused while D-37 is open")
	}
	if !strings.Contains(out, "VALIDATION_FAILED") {
		t.Fatalf("refusal must be typed, got %s", out)
	}
}
