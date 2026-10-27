// Package config implements the validated configuration contract (C2-05):
// layered sources (defaults → file → environment → explicit overrides),
// unknown-key rejection, range validation, and policy-conflict detection.
// Values are operator-tunable only; a secret is always a reference resolved
// through the secret boundary, never a configuration value. Environment
// lookup consults only the registered KARYORA_* names, so unknown
// environment keys cannot enter through this package.
package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Synchronous is the SQLite durability mode (C2-10 policy). NORMAL is the
// scaffold default and remains a Proposed value: the production default
// requires the recorded owner/technical review before it is fixed.
type Synchronous string

const (
	SynchronousNormal Synchronous = "NORMAL"
	SynchronousFull   Synchronous = "FULL"
)

// Config is the fully resolved, validated configuration. Every field always
// carries a value because every registered key has a compiled-in default.
type Config struct {
	PoolMaxOpen             int
	WriterQueueDepth        int
	WriterDeadlineMS        int
	BusyTimeoutMS           int
	WALAutocheckpointPages  int
	DurabilitySynchronous   Synchronous
	CASMaxObjectBytes       int
	CASMaxObjects           int
	CASGCGraceHours         int
	EventsPublishIntervalMS int
	EventsPublishBatch      int
	EventsRetentionDays     int
	// Audit and diagnostics bounds (C2-15 §3/§5; every default is Proposed
	// pending recorded review).
	AuditPendingMax         int
	AuditCheckpointInterval int
	DiagnosticsQueueDepth   int
	DiagnosticsMaxRecBytes  int
	DiagnosticsRatePerMin   int
}

// defaults are the compiled-in baseline (the lowest-precedence source).
var defaults = Config{
	PoolMaxOpen:             4,
	WriterQueueDepth:        64,
	WriterDeadlineMS:        10000,
	BusyTimeoutMS:           5000,
	WALAutocheckpointPages:  1000,
	DurabilitySynchronous:   SynchronousNormal,
	CASMaxObjectBytes:       268435456, // Proposed 256 MiB
	CASMaxObjects:           1000000,   // Proposed
	CASGCGraceHours:         24,        // Proposed (C2-12 grace period)
	EventsPublishIntervalMS: 1000,      // Proposed (C2-13 poller)
	EventsPublishBatch:      256,       // Proposed (C2-13 batch)
	EventsRetentionDays:     30,        // Proposed (C1-08 replay window)
	AuditPendingMax:         128,       // Proposed (C2-15 §5 reserved quota)
	AuditCheckpointInterval: 1000,      // Proposed (C2-15 §2 checkpoints)
	DiagnosticsQueueDepth:   256,       // Proposed (C2-15 §3 queue bound)
	DiagnosticsMaxRecBytes:  4096,      // Proposed (C2-15 §3 record bound)
	DiagnosticsRatePerMin:   120,       // Proposed (C2-15 §3 category rate)
}

type valueKind int

const (
	kindInt valueKind = iota
	kindSynchronous
)

type keySpec struct {
	env      string
	kind     valueKind
	min, max int
	isPolicy bool // differing values across sources are a hard conflict (C2-05)
}

// specs is the closed key registry: unknown keys are rejected, never
// ignored. All numeric defaults and ranges mirror the Phase 2
// specifications' Proposed values.
var specs = map[string]keySpec{
	"pool.max_open":              {env: "KARYORA_POOL_MAX_OPEN", kind: kindInt, min: 1, max: 64},
	"writer.queue_depth":         {env: "KARYORA_WRITER_QUEUE_DEPTH", kind: kindInt, min: 1, max: 10000},
	"writer.deadline_ms":         {env: "KARYORA_WRITER_DEADLINE_MS", kind: kindInt, min: 100, max: 600000},
	"busy_timeout_ms":            {env: "KARYORA_BUSY_TIMEOUT_MS", kind: kindInt, min: 100, max: 60000},
	"wal.autocheckpoint_pages":   {env: "KARYORA_WAL_AUTOCHECKPOINT_PAGES", kind: kindInt, min: 100, max: 100000},
	"durability.synchronous":     {env: "KARYORA_DURABILITY_SYNCHRONOUS", kind: kindSynchronous, isPolicy: true},
	"cas.max_object_bytes":       {env: "KARYORA_CAS_MAX_OBJECT_BYTES", kind: kindInt, min: 1048576, max: 8589934592},
	"cas.max_objects":            {env: "KARYORA_CAS_MAX_OBJECTS", kind: kindInt, min: 1000, max: 100000000},
	"cas.gc_grace_hours":         {env: "KARYORA_CAS_GC_GRACE_HOURS", kind: kindInt, min: 1, max: 720},
	"events.publish_interval_ms": {env: "KARYORA_EVENTS_PUBLISH_INTERVAL_MS", kind: kindInt, min: 100, max: 60000},
	"events.publish_batch":       {env: "KARYORA_EVENTS_PUBLISH_BATCH", kind: kindInt, min: 1, max: 10000},
	"events.retention_days":      {env: "KARYORA_EVENTS_RETENTION_DAYS", kind: kindInt, min: 1, max: 3650},
	"audit.pending_max":          {env: "KARYORA_AUDIT_PENDING_MAX", kind: kindInt, min: 1, max: 4096},
	"audit.checkpoint_interval":  {env: "KARYORA_AUDIT_CHECKPOINT_INTERVAL", kind: kindInt, min: 10, max: 100000},
	"diagnostics.queue_depth":    {env: "KARYORA_DIAGNOSTICS_QUEUE_DEPTH", kind: kindInt, min: 16, max: 10000},
	"diagnostics.max_record_bytes": {
		env: "KARYORA_DIAGNOSTICS_MAX_RECORD_BYTES", kind: kindInt, min: 256, max: 65536,
	},
	"diagnostics.category_rate_per_min": {
		env: "KARYORA_DIAGNOSTICS_CATEGORY_RATE_PER_MIN", kind: kindInt, min: 1, max: 10000,
	},
}

// field is one bounded validation problem reported in the error envelope.
type field struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Source string `json:"source"`
}

// Error carries the machine envelope as an error value. Its message states
// machine meaning only; human copy is resolved by the presentation layer
// from code + data (C2-05).
type Error struct{ Envelope errs.Envelope }

func (e *Error) Error() string {
	return fmt.Sprintf("config: %s (correlationId=%s)", e.Envelope.Code, e.Envelope.CorrelationID)
}

// Load resolves the layered configuration. file is the parsed configuration
// document (dotted key names), env resolves registered environment
// variables (return "" when unset), and overrides are explicit CLI-flag
// values. correlationID stamps the returned error envelope.
//
// Precedence: override > environment > file > default. A policy key set to
// different values by two or more sources is a CONFIG_CONFLICT naming the
// sources; equal values across sources are accepted. Validation problems
// and conflicts fail closed — the defaults are never partially applied.
func Load(correlationID string, file map[string]any, env func(string) string, overrides map[string]any) (Config, error) {
	if correlationID == "" {
		return Config{}, fmt.Errorf("config: correlationId is required")
	}
	var problems []field
	unknownKeys(file, overrides, &problems)
	conflictKey := ""
	var conflictSources []string

	out := defaults
	for name, spec := range specs {
		type observed struct {
			value any
			src   string
		}
		var seen []observed
		if file != nil {
			if v, ok := file[name]; ok {
				seen = append(seen, observed{value: v, src: "file"})
			}
		}
		if env != nil && spec.env != "" {
			if s := env(spec.env); s != "" {
				seen = append(seen, observed{value: s, src: "environment"})
			}
		}
		if overrides != nil {
			if v, ok := overrides[name]; ok {
				seen = append(seen, observed{value: v, src: "override"})
			}
		}
		normalized := make([]string, 0, len(seen))
		var effective any
		for _, o := range seen {
			v, ok := coerceValidate(name, spec, o.value, o.src, &problems)
			if !ok {
				continue
			}
			normalized = append(normalized, normalize(spec, v))
			effective = v
		}
		if spec.isPolicy && len(normalized) > 1 {
			distinct := map[string]bool{}
			for _, n := range normalized {
				distinct[n] = true
			}
			if len(distinct) > 1 {
				conflictKey = name
				conflictSources = conflictSources[:0]
				for _, o := range seen {
					conflictSources = append(conflictSources, o.src)
				}
			}
		}
		if effective != nil {
			assign(&out, name, effective)
		}
	}

	if conflictKey != "" {
		return fail(correlationID, errs.CodeConfigConflict, map[string]any{
			"key":     conflictKey,
			"sources": conflictSources,
		})
	}
	if len(problems) > 0 {
		return fail(correlationID, errs.CodeValidationFailed, map[string]any{
			"fields": problems,
		})
	}
	return out, nil
}

// unknownKeys reports keys outside the closed registry; they are rejected,
// never silently ignored (C2-05).
func unknownKeys(file map[string]any, overrides map[string]any, problems *[]field) {
	for name := range file {
		if _, ok := specs[name]; !ok {
			*problems = append(*problems, field{Path: name, Reason: "UNKNOWN_KEY", Source: "file"})
		}
	}
	for name := range overrides {
		if _, ok := specs[name]; !ok {
			*problems = append(*problems, field{Path: name, Reason: "UNKNOWN_KEY", Source: "override"})
		}
	}
}

func coerceValidate(name string, spec keySpec, value any, src string, problems *[]field) (any, bool) {
	bad := func(reason string) (any, bool) {
		*problems = append(*problems, field{Path: name, Reason: reason, Source: src})
		return nil, false
	}
	switch spec.kind {
	case kindInt:
		var n int
		switch v := value.(type) {
		case int:
			n = v
		case int64:
			n = int(v)
		case float64:
			if v != float64(int(v)) {
				return bad("INVALID_TYPE")
			}
			n = int(v)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return bad("INVALID_TYPE")
			}
			n = parsed
		default:
			return bad("INVALID_TYPE")
		}
		if n < spec.min || n > spec.max {
			return bad("OUT_OF_RANGE")
		}
		return n, true
	case kindSynchronous:
		s, ok := value.(string)
		if !ok {
			if v, ok := value.(Synchronous); ok {
				s = string(v)
			} else {
				return bad("INVALID_TYPE")
			}
		}
		mode := Synchronous(strings.ToUpper(s))
		if mode != SynchronousNormal && mode != SynchronousFull {
			return bad("UNSUPPORTED_VALUE")
		}
		return mode, true
	}
	return bad("INVALID_TYPE")
}

func normalize(spec keySpec, value any) string {
	switch spec.kind {
	case kindSynchronous:
		return strings.ToUpper(string(value.(Synchronous)))
	default:
		return strconv.Itoa(value.(int))
	}
}

func assign(out *Config, name string, value any) {
	switch name {
	case "pool.max_open":
		out.PoolMaxOpen = value.(int)
	case "writer.queue_depth":
		out.WriterQueueDepth = value.(int)
	case "writer.deadline_ms":
		out.WriterDeadlineMS = value.(int)
	case "busy_timeout_ms":
		out.BusyTimeoutMS = value.(int)
	case "wal.autocheckpoint_pages":
		out.WALAutocheckpointPages = value.(int)
	case "durability.synchronous":
		out.DurabilitySynchronous = value.(Synchronous)
	case "cas.max_object_bytes":
		out.CASMaxObjectBytes = value.(int)
	case "cas.max_objects":
		out.CASMaxObjects = value.(int)
	case "cas.gc_grace_hours":
		out.CASGCGraceHours = value.(int)
	case "events.publish_interval_ms":
		out.EventsPublishIntervalMS = value.(int)
	case "events.publish_batch":
		out.EventsPublishBatch = value.(int)
	case "events.retention_days":
		out.EventsRetentionDays = value.(int)
	case "audit.pending_max":
		out.AuditPendingMax = value.(int)
	case "audit.checkpoint_interval":
		out.AuditCheckpointInterval = value.(int)
	case "diagnostics.queue_depth":
		out.DiagnosticsQueueDepth = value.(int)
	case "diagnostics.max_record_bytes":
		out.DiagnosticsMaxRecBytes = value.(int)
	case "diagnostics.category_rate_per_min":
		out.DiagnosticsRatePerMin = value.(int)
	}
}

func fail(correlationID string, code errs.Code, data map[string]any) (Config, error) {
	env, err := errs.New(code, correlationID, data)
	if err != nil {
		return Config{}, err
	}
	return Config{}, &Error{Envelope: env}
}
