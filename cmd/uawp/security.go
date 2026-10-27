package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
)

// Security wiring (C2-14/C2-15): the composition root adapts the storage
// audit rows to the security package's AuditStore port, builds the identity
// bundle (sessions, local policy, guard, audit service, diagnostics
// logger), and wires the runner's authoritative IPC guard. The security
// package never imports storage; the dependency direction runs through
// this root only.

// lazyAuditStore adapts the lazily opened store to security.AuditStore.
// While the store is closed (pre-open startup window) every access fails —
// the audit service absorbs records into its reserved quota and effects
// deny when that quota exhausts (C2-15 §5 fail-closed).
type lazyAuditStore struct{ l *lazyStore }

func (lz lazyAuditStore) store() (*storage.Store, error) { return lz.l.get() }

func (lz lazyAuditStore) Tail() (security.TailState, error) {
	s, err := lz.store()
	if err != nil {
		return security.TailState{}, err
	}
	seq, hash, err := s.AuditTail()
	if err != nil {
		return security.TailState{}, err
	}
	return security.TailState{Sequence: seq, RecHash: hash}, nil
}

func (lz lazyAuditStore) Append(rec security.ChainedRecord) error {
	s, err := lz.store()
	if err != nil {
		return err
	}
	return s.AuditAppend(rec.Sequence, rec.PrevHash, rec.RecHash, rec.Payload)
}

func (lz lazyAuditStore) Records() ([]security.ChainedRecord, error) {
	s, err := lz.store()
	if err != nil {
		return nil, err
	}
	rows, err := s.AuditRecords()
	if err != nil {
		return nil, err
	}
	out := make([]security.ChainedRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, security.ChainedRecord{
			Sequence: r.Sequence, PrevHash: r.PrevHash, RecHash: r.RecHash, Payload: r.Payload,
		})
	}
	return out, nil
}

func (lz lazyAuditStore) Anchor(a security.Anchor) error {
	s, err := lz.store()
	if err != nil {
		return err
	}
	return s.AuditAnchor(a.Sequence, a.Hash)
}

func (lz lazyAuditStore) Anchors() ([]security.Anchor, error) {
	s, err := lz.store()
	if err != nil {
		return nil, err
	}
	rows, err := s.AuditAnchors()
	if err != nil {
		return nil, err
	}
	out := make([]security.Anchor, 0, len(rows))
	for _, r := range rows {
		out = append(out, security.Anchor{Sequence: r.Sequence, Hash: r.Hash})
	}
	return out, nil
}

func (lz lazyAuditStore) SetIntegrityBroken(broken bool) error {
	s, err := lz.store()
	if err != nil {
		return err
	}
	return s.SetAuditIntegrityBroken(broken)
}

func (lz lazyAuditStore) IntegrityBroken() (bool, error) {
	s, err := lz.store()
	if err != nil {
		return false, err
	}
	return s.AuditIntegrityBroken()
}

// securityBundle carries the wired identity/audit/diagnostics instances.
type securityBundle struct {
	sessions *security.SessionService
	policy   *security.LocalPolicy
	guard    *security.Guard
	audit    *security.AuditService
	logger   *security.Logger
	instance string
}

// newSecurityBundle builds the local identity slice (C2-14): the session
// service bound to this process's data directory, the default-deny local
// policy, the guard with audit linkage, the audit service over the lazy
// store, and the bounded diagnostics logger writing to the state class.
// No client transport presents session tokens in Phase 2 (the surfaces are
// CLI/IPC, whose credential class is the per-instance capability); the
// session machinery is contract-complete and exercised by its tests.
func newSecurityBundle(l *lazyStore, cfg config.Config, dataDir, stateDir, correlationID string) *securityBundle {
	instance := fmt.Sprintf("inst-%d", os.Getpid())
	b := &securityBundle{instance: instance}
	b.sessions = security.NewSessionService(instance, dataDir)
	b.policy = security.NewLocalPolicy(b.sessions)
	b.audit = security.NewAuditService(lazyAuditStore{l: l}, cfg.AuditPendingMax, uint64(cfg.AuditCheckpointInterval))
	b.guard = &security.Guard{Policy: b.policy, Audit: b.audit}
	b.logger = security.NewLogger(cfg.DiagnosticsQueueDepth, cfg.DiagnosticsMaxRecBytes, cfg.DiagnosticsRatePerMin,
		diagnosticsFileSink(stateDir))
	return b
}

// guardFunc builds the runner's authoritative IPC check (C2-14 §2): the
// one decision port + audit record before any effect. State-changing
// effects deny when the audit record cannot be written (AUDIT_UNAVAILABLE);
// read-only actions keep reads available and surface the failure through
// diagnostics (C2-15 §5).
func (b *securityBundle) guardFunc(correlationID string) func(ctx context.Context, action string) error {
	return func(ctx context.Context, action string) error {
		p := security.Principal{Kind: security.PrincipalLocalUser, ID: b.instance}
		r := security.Resource{Type: security.ResourceCore}
		a := security.Action(action)
		pctx := security.PolicyContext{
			CorrelationID: correlationID,
			Credential:    security.CredentialIPCCapability,
		}
		decision, auditErr := b.guard.Authorize(p, r, a, pctx)
		if auditErr != nil {
			if a.StateChanging() {
				return auditErr // the effect is denied, not executed unrecorded
			}
			b.logger.Emit(security.Event{
				Level:         security.LevelWarn,
				Category:      "security.audit",
				MessageKey:    "AUDIT_RECORD_FAILED_READ_PATH",
				CorrelationID: correlationID,
				Fields:        map[string]string{"detail": boundedDetail(auditErr)},
			})
		}
		if !decision.Allow {
			env, err := decision.DeniedEnvelope(correlationID, r, a)
			if err != nil {
				return err
			}
			return &authzError{env: env}
		}
		return nil
	}
}

// auditMirror mirrors one lifecycle transition into the audit chain
// (C2-15 §1: the Phase 2 audit slice populates lifecycle and
// authorization events). The record carries identifiers only; the full
// transition detail lives in the journal event (C2-07 §1). A mirror
// failure is surfaced through diagnostics, never silent.
func (b *securityBundle) auditMirror(ev runtime.Event, correlationID string) {
	rec := security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           "local-user:" + b.instance,
		Action:          "lifecycle.transition",
		Scope:           "core:foreground",
		PolicyDecision:  "allow",
		PolicyReference: "lifecycle-machine:" + string(ev.From) + "->" + string(ev.To),
		Outcome:         "success",
		CorrelationID:   correlationID,
	}
	if err := b.audit.Record(rec); err != nil {
		b.logger.Emit(security.Event{
			Level:         security.LevelWarn,
			Category:      "security.audit",
			MessageKey:    "AUDIT_RECORD_FAILED",
			CorrelationID: correlationID,
			Fields:        map[string]string{"detail": boundedDetail(err)},
		})
	}
}

// diagnosticsFileSink returns the bounded delivery sink writing JSON lines
// into the state class (append-only, 0600). Delivery failures are silent
// at the sink (the logger's dropped counters are the health signal) — the
// sink never blocks the Core.
func diagnosticsFileSink(stateDir string) func(security.Event) {
	return func(ev security.Event) {
		path := filepath.Join(stateDir, "diagnostics.jsonl")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		type sinkRecord struct {
			Time          string            `json:"time"`
			Level         string            `json:"level"`
			Category      string            `json:"category"`
			MessageKey    string            `json:"message_key"`
			CorrelationID string            `json:"correlation_id,omitempty"`
			Fields        map[string]string `json:"fields,omitempty"`
		}
		b, err := json.Marshal(sinkRecord{
			Time:          ev.Time.UTC().Format(time.RFC3339Nano),
			Level:         ev.Level.String(),
			Category:      ev.Category,
			MessageKey:    ev.MessageKey,
			CorrelationID: ev.CorrelationID,
			Fields:        ev.Fields,
		})
		if err != nil {
			return
		}
		_, _ = f.Write(append(b, '\n'))
	}
}

// authzError carries a machine denial envelope to the CLI boundary.
type authzError struct{ env errs.Envelope }

func (e *authzError) Error() string {
	return "security: " + string(e.env.Code) + " (" + e.env.CorrelationID + ")"
}

// auditProbe is doctor's read-only audit verification path: it opens the
// existing database read-concurrent (never migrating or repairing), runs
// the chain verification, and closes. doctor never creates anything.
func auditProbe(ctx context.Context, opts storage.Options, cfg config.Config) runtime.DoctorCheck {
	if _, err := os.Stat(opts.Path); os.IsNotExist(err) {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorUnavailable, Detail: "database not initialized (run start once)"}
	}
	s, err := storage.Connect(opts)
	if err != nil {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorUnavailable, Detail: bounded(err)}
	}
	defer s.Close()
	l := &lazyStore{opts: opts}
	l.mu.Lock()
	l.store = s
	l.mu.Unlock()
	audit := security.NewAuditService(lazyAuditStore{l: l}, cfg.AuditPendingMax, uint64(cfg.AuditCheckpointInterval))
	b := &securityBundle{audit: audit, logger: security.NewLogger(16, 4096, 120, nil)}
	defer b.logger.Close()
	return doctorAuditCheck(ctx, b)
}

func boundedDetail(err error) string {
	const max = 160
	s := err.Error()
	if len(s) > max {
		return s[:max]
	}
	return s
}

// doctorAuditCheck is doctor's read-only audit verification (C2-15 §2): the
// chain is re-computed and every checkpoint checked; an integrity-broken
// chain is a visible FAIL. doctor never repairs.
func doctorAuditCheck(ctx context.Context, b *securityBundle) runtime.DoctorCheck {
	broken, err := b.audit.Broken()
	if err != nil {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorUnavailable, Detail: boundedDetail(err)}
	}
	if broken {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorFail, Detail: "audit chain integrity broken; writes stopped (incident process)"}
	}
	report, err := b.audit.Verify()
	if err != nil {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorUnavailable, Detail: boundedDetail(err)}
	}
	if !report.OK {
		return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorFail, Detail: fmt.Sprintf("chain broken at sequence %d: %s", report.BrokenAt, report.Reason)}
	}
	return runtime.DoctorCheck{Name: "audit", Status: runtime.DoctorPass, Detail: fmt.Sprintf("chain verified: %d records, %d anchors", report.Records, report.Anchors)}
}
