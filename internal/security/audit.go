package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
)

// Audit (C2-15): security-decision records with tamper evidence, a
// separate class from diagnostics. The schema carries identifiers, policy
// outcomes, and content DIGESTS/references only — prompts, full documents,
// secret values, and session tokens are structurally absent (there is no
// free-text content field to hold them). Tamper-evidence is a chain over
// the canonical representation with periodic anchored checkpoints; it
// proves integrity, not actor identity or business truth, and its signing
// strength follows the open D-37 decisions — the system never claims more
// than detection.
const (
	// AuditSchemaVersion is the current record schema version.
	AuditSchemaVersion = 1
	// CheckpointInterval is the periodic anchored-checkpoint cadence
	// (Proposed 1000 records, C2-15 §2; owner/technical acceptance
	// pending). It is overridable at construction for tests.
	CheckpointInterval = 1000
	// chainDomain separates audit-chain hashes from every other SHA-256
	// use in the Core.
	chainDomain = "karyora-audit-chain-v1"
)

// AuditRecord is one audit record. Stable English field names; the zero
// values of job/attempt/provider/plugin fields are schema-ready and
// populate as those subsystems arrive.
type AuditRecord struct {
	AuditID         string   `json:"audit_id"`
	SchemaVersion   int      `json:"schema_version"`
	Time            string   `json:"time"` // RFC3339Nano UTC
	Actor           string   `json:"actor"`
	Action          string   `json:"action"`
	Scope           string   `json:"scope"`
	PolicyDecision  string   `json:"policy_decision"`
	PolicyReference string   `json:"policy_reference"`
	JobID           string   `json:"job_id,omitempty"`
	AttemptID       string   `json:"attempt_id,omitempty"`
	ProviderID      string   `json:"provider_id,omitempty"`
	PluginID        string   `json:"plugin_id,omitempty"`
	Outcome         string   `json:"outcome"`
	ContentDigests  []string `json:"content_digests,omitempty"`
	ContentRefs     []string `json:"content_refs,omitempty"`
	CorrelationID   string   `json:"correlation_id"`
}

func (r AuditRecord) validate() error {
	if r.AuditID == "" || len(r.AuditID) > maxIDLen {
		return errors.New("security: audit_id is missing or unbounded")
	}
	if r.SchemaVersion != AuditSchemaVersion {
		return errors.New("security: audit schema version mismatch")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.Time); err != nil {
		return errors.New("security: audit time is not RFC3339")
	}
	for _, f := range []string{r.Actor, r.Action, r.Scope, r.PolicyDecision, r.PolicyReference, r.Outcome, r.CorrelationID,
		r.JobID, r.AttemptID, r.ProviderID, r.PluginID} {
		if len(f) > maxIDLen {
			return errors.New("security: audit identifier is unbounded")
		}
	}
	for _, d := range append(append([]string(nil), r.ContentDigests...), r.ContentRefs...) {
		if !digestGrammar(d) {
			return errors.New("security: audit content reference does not match the digest grammar")
		}
		if len(r.ContentDigests)+len(r.ContentRefs) > 64 {
			return errors.New("security: audit content reference set is unbounded")
		}
	}
	return nil
}

// canonical renders the deterministic record payload: fixed field order,
// RFC3339Nano UTC time, no chain fields (the chain covers this payload).
func (r AuditRecord) canonical() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(r)
}

// digestGrammar mirrors the additive CAS naming scheme locally so the
// package stays within its dependency rule (domain only).
func digestGrammar(s string) bool {
	const prefix = "sha256:"
	if len(s) != len(prefix)+64 || len(s) < 7 {
		return false
	}
	if s[:7] != prefix {
		return false
	}
	for _, c := range s[7:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ChainHash computes the record hash over the canonical payload plus the
// chain position. Length-delimited fields keep the composition unambiguous.
func ChainHash(prevHash string, sequence uint64, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(chainDomain))
	h.Write([]byte{0})
	h.Write([]byte(prevHash))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatUint(sequence, 10)))
	h.Write([]byte{0})
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(len(payload) >> (8 * (7 - i)))
	}
	h.Write(b)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// TailState is the durable chain tail.
type TailState struct {
	Sequence uint64
	RecHash  string
}

// ChainedRecord is one durably stored record: the canonical payload plus
// its chain position.
type ChainedRecord struct {
	Sequence uint64
	PrevHash string
	RecHash  string
	Payload  []byte
}

// Anchor is one periodic checkpoint anchor (hash of the chain at a
// sequence). Signing/trust anchors follow D-37 (open); Phase 2 anchors are
// detection values only.
type Anchor struct {
	Sequence uint64 `json:"sequence"`
	Hash     string `json:"hash"`
}

// AuditStore is the durable append-only audit storage port. The
// implementation (storage package adapter wired by the composition root)
// must be append-only and refuse forks.
type AuditStore interface {
	// Tail returns the current chain tail (zero state when empty).
	Tail() (TailState, error)
	// Append durably appends one completed record. A record whose sequence
	// is not tail+1 or whose prev hash is not the tail hash MUST be
	// refused (the chain can never fork).
	Append(rec ChainedRecord) error
	// Records returns every stored record in sequence order (bounded
	// verification reads).
	Records() ([]ChainedRecord, error)
	// Anchor records one checkpoint anchor.
	Anchor(a Anchor) error
	// Anchors returns all recorded anchors in sequence order.
	Anchors() ([]Anchor, error)
	// SetIntegrityBroken marks the integrity-broken health signal; while
	// set, the store refuses further appends — writes to the affected
	// segment stop (C2-15 §2). Clearing is an explicit incident-process
	// action, never automatic.
	SetIntegrityBroken(broken bool) error
	// IntegrityBroken reports the signal.
	IntegrityBroken() (bool, error)
}

// AuditService owns the audit write path (C2-15 §5): audit is on the
// critical path — a protected effect whose record cannot be durably
// appended is denied (AUDIT_UNAVAILABLE family). A bounded reserved-quota
// buffer absorbs short storage pressure before denial begins; reads remain
// available. The service never drops audit records to make room.
type AuditService struct {
	store              AuditStore
	clock              func() time.Time
	quota              int
	checkpointInterval uint64

	mu      sync.Mutex
	pending []ChainedRecord
}

// NewAuditService builds the service. quota is the reserved-quota bound
// (Proposed 128, configuration key audit.pending_max); interval is the
// checkpoint cadence (Proposed 1000).
func NewAuditService(store AuditStore, quota int, interval uint64) *AuditService {
	if quota < 1 {
		quota = 1
	}
	if interval < 1 {
		interval = CheckpointInterval
	}
	return &AuditService{
		store:              store,
		clock:              time.Now,
		quota:              quota,
		checkpointInterval: interval,
	}
}

// ErrAuditUnavailable is the typed critical-path failure: the effect must
// be denied rather than executed unrecorded.
type ErrAuditUnavailable struct{ Detail string }

func (e *ErrAuditUnavailable) Error() string {
	return "security: audit unavailable: " + e.Detail
}

// Envelope maps the failure onto the closed register (STORAGE_UNAVAILABLE
// with the typed reason code).
func (e *ErrAuditUnavailable) Envelope(correlationID string) (errs.Envelope, error) {
	return errs.New(errs.CodeStorageUnavailable, correlationID, map[string]any{"reason": ReasonAuditUnavailable})
}

func errAuditUnavailable(detail string) error {
	return &ErrAuditUnavailable{Detail: detail}
}

// Record appends one audit record and reports whether the effect may
// proceed. The record is validated (bounded identifiers, digest grammar),
// canonicalized, and chained onto the durable tail. On a transient store
// failure the record enters the reserved-quota buffer and the caller may
// proceed; when the buffer is exhausted, denial begins (C2-15 §5).
func (s *AuditService) Record(rec AuditRecord) error {
	if s.store == nil {
		return errAuditUnavailable("audit store is not wired")
	}
	if rec.AuditID == "" {
		id, err := randomHex(8)
		if err != nil {
			return errAuditUnavailable("entropy unavailable")
		}
		rec.AuditID = id
	}
	if rec.Time == "" {
		rec.Time = s.now().UTC().Format(time.RFC3339Nano)
	}
	payload, err := rec.canonical()
	if err != nil {
		// A contract-violating record is refused — never stored malformed.
		return errAuditUnavailable(err.Error())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Drain the reserved-quota buffer first (FIFO; order is preserved).
	if err := s.drainLocked(); err != nil {
		return err
	}
	if len(s.pending) >= s.quota {
		// Reserved quota exhausted: denial begins.
		return errAuditUnavailable("audit reserved quota exhausted")
	}
	tail, err := s.store.Tail()
	if err != nil {
		// Transient pressure: absorb into the quota if room remains.
		return s.absorbLocked(tail, payload, err)
	}
	if err := s.appendOne(tail, payload); err != nil {
		return s.absorbLocked(tail, payload, err)
	}
	return nil
}

// drainLocked flushes buffered records head-first; the first failure stops
// the drain (the buffer keeps order).
func (s *AuditService) drainLocked() error {
	for len(s.pending) > 0 {
		head := s.pending[0]
		if err := s.store.Append(head); err != nil {
			if broken, berr := s.store.IntegrityBroken(); berr == nil && broken {
				return errAuditUnavailable("audit chain integrity broken; writes stopped")
			}
			return nil // store still under pressure; keep buffering
		}
		s.pending = s.pending[1:]
		if head.Sequence%s.checkpointInterval == 0 {
			_ = s.store.Anchor(Anchor{Sequence: head.Sequence, Hash: head.RecHash})
		}
	}
	return nil
}

// absorbLocked buffers one record inside the reserved quota; a full quota
// denies.
func (s *AuditService) absorbLocked(tail TailState, payload []byte, cause error) error {
	if broken, berr := s.store.IntegrityBroken(); berr == nil && broken {
		return errAuditUnavailable("audit chain integrity broken; writes stopped")
	}
	if len(s.pending) >= s.quota {
		return errAuditUnavailable("audit reserved quota exhausted: " + cause.Error())
	}
	seq := tail.Sequence + uint64(len(s.pending)) + 1
	prev := tail.RecHash
	if len(s.pending) > 0 {
		last := s.pending[len(s.pending)-1]
		seq = last.Sequence + 1
		prev = last.RecHash
	}
	s.pending = append(s.pending, ChainedRecord{
		Sequence: seq,
		PrevHash: prev,
		RecHash:  ChainHash(prev, seq, payload),
		Payload:  append([]byte(nil), payload...),
	})
	return nil
}

func (s *AuditService) appendOne(tail TailState, payload []byte) error {
	rec := ChainedRecord{
		Sequence: tail.Sequence + 1,
		PrevHash: tail.RecHash,
		RecHash:  ChainHash(tail.RecHash, tail.Sequence+1, payload),
		Payload:  payload,
	}
	if err := s.store.Append(rec); err != nil {
		return err
	}
	if rec.Sequence%s.checkpointInterval == 0 {
		_ = s.store.Anchor(Anchor{Sequence: rec.Sequence, Hash: rec.RecHash})
	}
	return nil
}

// Flush drains the reserved-quota buffer durably (drain step: journal,
// outbox, audit — C2-07 §2 safe order). A failed flush surfaces: buffered
// records remain buffered and the caller reports the outcome; the next
// start's reconcile re-attempts nothing for them (they were not durable)
// — the bounded quota is the recorded trade-off.
func (s *AuditService) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.drainLocked(); err != nil {
		return err
	}
	if len(s.pending) > 0 {
		return errAuditUnavailable(fmt.Sprintf("%d audit records remain buffered", len(s.pending)))
	}
	return nil
}

// Pending reports the buffered count (diagnostics).
func (s *AuditService) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func (s *AuditService) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock()
}

// IntegrityReport is the verification outcome (C2-15 §2): chain breaks and
// checkpoint mismatches are detected and localized; the report never
// repairs anything.
type IntegrityReport struct {
	OK       bool   `json:"ok"`
	Records  int    `json:"records"`
	Anchors  int    `json:"anchors"`
	BrokenAt uint64 `json:"broken_at,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Verify re-computes the whole chain and checks every recorded anchor. A
// detected break marks the segment integrity-broken (visible health
// signal; writes stop) and enters the incident process — history is never
// silently rewritten to "fix" the chain.
func (s *AuditService) Verify() (IntegrityReport, error) {
	report := IntegrityReport{}
	if s.store == nil {
		return report, errAuditUnavailable("audit store is not wired")
	}
	recs, err := s.store.Records()
	if err != nil {
		return report, err
	}
	report.Records = len(recs)
	prev := TailState{}
	for i, r := range recs {
		if r.Sequence != prev.Sequence+1 {
			return s.fail(report, r.Sequence, "sequence gap"), nil
		}
		if r.PrevHash != prev.RecHash {
			return s.fail(report, r.Sequence, "prev-hash mismatch"), nil
		}
		if ChainHash(r.PrevHash, r.Sequence, r.Payload) != r.RecHash {
			return s.fail(report, r.Sequence, "record hash mismatch"), nil
		}
		prev = TailState{Sequence: r.Sequence, RecHash: r.RecHash}
		_ = i
	}
	anchors, err := s.store.Anchors()
	if err != nil {
		return report, err
	}
	report.Anchors = len(anchors)
	bySeq := map[uint64]string{}
	for _, a := range anchors {
		bySeq[a.Sequence] = a.Hash
	}
	// Recompute the expected anchor for every stored checkpoint: a
	// mismatch is a checkpoint integrity failure.
	for seq, hash := range bySeq {
		if seq == 0 || int(seq) > len(recs) {
			return s.fail(report, seq, "checkpoint references an unknown sequence"), nil
		}
		if recs[seq-1].RecHash != hash {
			return s.fail(report, seq, "checkpoint hash mismatch"), nil
		}
	}
	// Every checkpoint interval within the stored range must have an anchor.
	for n := uint64(s.checkpointInterval); n <= prev.Sequence; n += s.checkpointInterval {
		if _, ok := bySeq[n]; !ok {
			return s.fail(report, n, "missing checkpoint"), nil
		}
	}
	report.OK = true
	return report, nil
}

func (s *AuditService) fail(report IntegrityReport, at uint64, reason string) IntegrityReport {
	report.OK = false
	report.BrokenAt = at
	report.Reason = reason
	// Visible health signal + writes stop; the incident enters the
	// vulnerability/incident process (C1-12). The mark is best-effort: a
	// store that cannot even record the flag still returns the report.
	_ = s.store.SetIntegrityBroken(true)
	return report
}

// Broken reports the durable integrity-broken signal.
func (s *AuditService) Broken() (bool, error) {
	if s.store == nil {
		return false, errAuditUnavailable("audit store is not wired")
	}
	return s.store.IntegrityBroken()
}
