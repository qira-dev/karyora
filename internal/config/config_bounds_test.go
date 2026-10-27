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
