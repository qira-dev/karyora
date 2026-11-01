package config

import "testing"

// The audit/diagnostics bounds (C2-15 §3/§5) are registered keys with
// Proposed defaults; unknown keys stay rejected and ranges stay enforced.
func TestAuditDiagnosticsBoundsKeys(t *testing.T) {
	cfg, err := Load("corr", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuditPendingMax != 128 {
		t.Fatalf("audit.pending_max default = %d, want 128 (Proposed)", cfg.AuditPendingMax)
	}
	if cfg.AuditCheckpointInterval != 1000 {
		t.Fatalf("audit.checkpoint_interval default = %d, want 1000 (Proposed)", cfg.AuditCheckpointInterval)
	}
	if cfg.DiagnosticsQueueDepth != 256 || cfg.DiagnosticsMaxRecBytes != 4096 || cfg.DiagnosticsRatePerMin != 120 {
		t.Fatalf("diagnostics defaults = %+v", cfg)
	}

	// Overrides apply and validate.
	cfg, err = Load("corr", map[string]any{
		"audit.pending_max":                 "64",
		"diagnostics.queue_depth":           32,
		"diagnostics.max_record_bytes":      2048,
		"diagnostics.category_rate_per_min": 60,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuditPendingMax != 64 || cfg.DiagnosticsQueueDepth != 32 || cfg.DiagnosticsMaxRecBytes != 2048 || cfg.DiagnosticsRatePerMin != 60 {
		t.Fatalf("overrides = %+v", cfg)
	}

	// Out-of-range values fail closed.
	for _, key := range []string{"audit.pending_max", "audit.checkpoint_interval", "diagnostics.queue_depth", "diagnostics.max_record_bytes", "diagnostics.category_rate_per_min"} {
		_, err := Load("corr", map[string]any{key: 0}, nil, nil)
		if err == nil {
			t.Fatalf("%s below the range must fail", key)
		}
	}
}

// The local HTTP transport keys (IMPL-1): opt-in (disabled by default),
// port bounded; neither value tunes any security property (the boundary
// predicates and session lifetimes are contract constants, not
// configuration).
func TestTransportLocalHTTPKeys(t *testing.T) {
	cfg, err := Load("corr", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TransportLocalHTTPEnabled != 0 || cfg.TransportLocalHTTPPort != 0 {
		t.Fatalf("transport defaults = %+v, want disabled with a kernel-assigned port", cfg)
	}

	cfg, err = Load("corr", map[string]any{
		"transport.local_http.enabled": 1,
		"transport.local_http.port":    8787,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TransportLocalHTTPEnabled != 1 || cfg.TransportLocalHTTPPort != 8787 {
		t.Fatalf("transport overrides = %+v", cfg)
	}

	// The enabled key is a strict 0/1 switch; the port is a bounded port.
	for _, tc := range []struct {
		key   string
		value int
	}{
		{"transport.local_http.enabled", 2},
		{"transport.local_http.enabled", -1},
		{"transport.local_http.port", 65536},
		{"transport.local_http.port", -5},
	} {
		if _, err := Load("corr", map[string]any{tc.key: tc.value}, nil, nil); err == nil {
			t.Fatalf("%s=%d must fail the range check", tc.key, tc.value)
		}
	}
}
