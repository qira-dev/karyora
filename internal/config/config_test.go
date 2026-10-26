package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

func TestDefaultsLoad(t *testing.T) {
	cfg, err := Load("c1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		PoolMaxOpen:             4,
		WriterQueueDepth:        64,
		WriterDeadlineMS:        10000,
		BusyTimeoutMS:           5000,
		WALAutocheckpointPages:  1000,
		DurabilitySynchronous:   SynchronousNormal,
		CASMaxObjectBytes:       268435456,
		CASMaxObjects:           1000000,
		CASGCGraceHours:         24,
		EventsPublishIntervalMS: 1000,
		EventsPublishBatch:      256,
		EventsRetentionDays:     30,
	}
	if cfg != want {
		t.Fatalf("defaults mismatch:\n got %+v\nwant %+v", cfg, want)
	}
}

func TestPrecedenceOverrideWins(t *testing.T) {
	cfg, err := Load("c1", map[string]any{"pool.max_open": 8}, func(string) string { return "" }, map[string]any{"pool.max_open": 16})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PoolMaxOpen != 16 {
		t.Fatalf("override must win over file: %+v", cfg)
	}
}

func TestEnvLayerApplies(t *testing.T) {
	env := func(k string) string {
		if k == "KARYORA_BUSY_TIMEOUT_MS" {
			return "2500"
		}
		return ""
	}
	cfg, err := Load("c1", nil, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BusyTimeoutMS != 2500 {
		t.Fatalf("env value must apply: %+v", cfg)
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	_, err := Load("c1", map[string]any{"max_jobs_per_tenant": 5}, nil, nil)
	if err == nil {
		t.Fatal("unknown key must fail closed")
	}
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error, got %T", err)
	}
	if ce.Envelope.Code != errs.CodeValidationFailed {
		t.Fatalf("code = %s, want VALIDATION_FAILED", ce.Envelope.Code)
	}
	b, _ := json.Marshal(ce.Envelope.Data)
	if !strings.Contains(string(b), "UNKNOWN_KEY") || !strings.Contains(string(b), "max_jobs_per_tenant") {
		t.Fatalf("problem not reported: %s", b)
	}
}

func TestRangeValidation(t *testing.T) {
	_, err := Load("c1", map[string]any{"pool.max_open": 999}, nil, nil)
	if err == nil {
		t.Fatal("out-of-range value must fail")
	}
	ce := err.(*Error)
	b, _ := json.Marshal(ce.Envelope.Data)
	if !strings.Contains(string(b), "OUT_OF_RANGE") {
		t.Fatalf("range reason missing: %s", b)
	}
}

func TestPolicyConflict(t *testing.T) {
	env := func(k string) string {
		if k == "KARYORA_DURABILITY_SYNCHRONOUS" {
			return "FULL"
		}
		return ""
	}
	_, err := Load("c1", map[string]any{"durability.synchronous": "NORMAL"}, env, nil)
	if err == nil {
		t.Fatal("policy conflict must fail")
	}
	ce := err.(*Error)
	if ce.Envelope.Code != errs.CodeConfigConflict {
		t.Fatalf("code = %s, want CONFIG_CONFLICT", ce.Envelope.Code)
	}
	b, _ := json.Marshal(ce.Envelope.Data)
	if !strings.Contains(string(b), "durability.synchronous") || !strings.Contains(string(b), "environment") {
		t.Fatalf("conflict must name key and sources: %s", b)
	}
}

func TestEqualPolicyValuesNoConflict(t *testing.T) {
	env := func(k string) string {
		if k == "KARYORA_DURABILITY_SYNCHRONOUS" {
			return "NORMAL"
		}
		return ""
	}
	cfg, err := Load("c1", map[string]any{"durability.synchronous": "normal"}, env, nil)
	if err != nil {
		t.Fatalf("equal policy values must not conflict: %v", err)
	}
	if cfg.DurabilitySynchronous != SynchronousNormal {
		t.Fatalf("normalized value mismatch: %+v", cfg)
	}
}

// TestErrorEnvelopeMachineFields pins the envelope hygiene: the owner-
// accepted field names only, no title/detail/prose members.
func TestErrorEnvelopeMachineFields(t *testing.T) {
	_, err := Load("c1", map[string]any{"pool.max_open": "abc"}, nil, nil)
	if err == nil {
		t.Fatal("invalid type must fail")
	}
	ce := err.(*Error)
	b, _ := json.Marshal(ce.Envelope)
	for _, banned := range []string{`"title"`, `"detail"`, `"message"`} {
		if strings.Contains(string(b), banned) {
			t.Fatalf("banned field %s present in %s", banned, b)
		}
	}
}
