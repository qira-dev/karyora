package security

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

// Diagnostics (C2-15 §3–§5): structured, bounded, sanitized operational
// events — a separate class from audit with its own schema and degrade
// behavior. Every record is size-bounded, every category rate-bounded, and
// the queue is bounded with lowest-severity dropping plus dropped
// counters; logging loss never deadlocks the Core and never disables a
// security decision. Untrusted input is sanitized and redaction runs at
// the emit boundary — before formatting or export — so log injection
// cannot forge entries and excluded material never reaches a sink.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	default:
		return "error"
	}
}

// severity order used by the drop-lowest policy.
func (l Level) less(o Level) bool { return l < o }

// Event is one structured diagnostic record. Stable English keys; Fields
// carry bounded sanitized values only.
type Event struct {
	Time          time.Time
	Level         Level
	Category      string
	MessageKey    string
	CorrelationID string
	Fields        map[string]string
}

// RedactMarker replaces every excluded value at the emit boundary.
const RedactMarker = "[REDACTED]"

// excludedKeySubstrings is the excluded-by-default list (C2-15 §4):
// provider credentials, session tokens, key material, raw prompts, raw
// model output, file contents, and sensitive headers. Matching is
// case-insensitive over field keys; a matched key is emitted redacted.
var excludedKeySubstrings = []string{
	"authorization", "cookie", "token", "secret", "password", "passwd",
	"credential", "api_key", "api-key", "apikey", "private_key",
	"private-key", "signing_key", "key_material", "prompt", "model_output",
	"file_content", "document_body", "set_cookie", "session_key",
}

func isExcludedKey(key string) bool {
	k := strings.ToLower(key)
	for _, sub := range excludedKeySubstrings {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

// Sanitize normalizes untrusted text so structure is always injected by
// the logger and never by data: control characters are escaped, other
// non-printables are dropped, and the result is length-bounded. This is
// what makes forged log entries impossible (C2-15 §3).
func Sanitize(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\\':
			b.WriteString(`\\`)
		case unicode.IsControl(r) || (r >= 0x7f && r <= 0x9f):
			b.WriteString(`\x`)
			const hexdigits = "0123456789abcdef"
			b.WriteByte(hexdigits[(r>>4)&0xf])
			b.WriteByte(hexdigits[r&0xf])
		case r == unicode.ReplacementChar:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
		if b.Len() >= max {
			break
		}
	}
	out := b.String()
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// Redact applies the excluded-by-default rules to a field set. It runs at
// the emit boundary (before formatting/export) — never as post-filtering.
func Redact(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if isExcludedKey(k) {
			out[k] = RedactMarker
			continue
		}
		out[k] = v
	}
	return out
}

// maxEventFields bounds one record's field count (record size is bounded
// separately).
const maxEventFields = 16

// rateWindow is the per-category rate-bound window.
const rateWindow = time.Minute

type rateState struct {
	count int
	reset time.Time
}

// Logger is the bounded diagnostic emitter. Emit never blocks and never
// panics; on a full queue the lowest-severity record is dropped with a
// dropped counter; a category over its rate bound is dropped with a
// counter. Delivery happens on one drain goroutine through the sink.
type Logger struct {
	mu      sync.Mutex
	buf     []Event
	maxBuf  int
	maxRec  int
	ratePM  int
	rates   map[string]*rateState
	dropped [4]uint64 // per severity

	sink    func(Event)
	wake    chan struct{}
	done    chan struct{}
	stopped bool
}

// NewLogger builds the bounded logger. queueDepth is the buffer bound
// (Proposed 256, diagnostics.queue_depth); maxRecordBytes bounds one record
// (Proposed 4096, diagnostics.max_record_bytes); ratePerMin bounds each
// category per minute (Proposed 120, diagnostics.category_rate_per_min).
// sink receives sanitized+redacted records; nil sinks record drops only.
func NewLogger(queueDepth, maxRecordBytes, ratePerMin int, sink func(Event)) *Logger {
	if queueDepth < 1 {
		queueDepth = 1
	}
	if maxRecordBytes < 1 {
		maxRecordBytes = 4096
	}
	if ratePerMin < 1 {
		ratePerMin = 120
	}
	l := &Logger{
		maxBuf: queueDepth,
		maxRec: maxRecordBytes,
		ratePM: ratePerMin,
		rates:  map[string]*rateState{},
		sink:   sink,
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go l.drain()
	return l
}

// Emit enqueues one record (non-blocking). Bounds are enforced at the
// emit boundary: sanitize every value, redact excluded keys, cap the
// record size, rate-bound the category.
func (l *Logger) Emit(ev Event) {
	if ev.Category == "" || ev.MessageKey == "" {
		l.drop(ev.Level)
		return
	}
	now := time.Now()
	if ev.Time.IsZero() {
		ev.Time = now
	}
	if !l.allowRate(ev.Category, now) {
		l.drop(ev.Level)
		return
	}
	fields := Redact(ev.Fields)
	if len(fields) > maxEventFields {
		fields = truncateFields(fields, maxEventFields)
	}
	budget := l.maxRec
	ev.Fields = make(map[string]string, len(fields))
	total := 0
	for k, v := range fields {
		key := Sanitize(k, 64)
		val := Sanitize(v, 256)
		total += len(key) + len(val) + 4
		if total > budget-len(ev.Category)-len(ev.MessageKey)-len(ev.CorrelationID)-32 {
			l.drop(ev.Level)
			return
		}
		ev.Fields[key] = val
	}
	ev.Category = Sanitize(ev.Category, 64)
	ev.MessageKey = Sanitize(ev.MessageKey, 128)
	ev.CorrelationID = Sanitize(ev.CorrelationID, maxIDLen)

	l.mu.Lock()
	if len(l.buf) >= l.maxBuf {
		// Drop the lowest-severity queued record to make room; a record
		// lower than the newcomer is dropped instead. Counters make the
		// loss visible (C2-15 §5).
		victim := -1
		for i, q := range l.buf {
			if q.Level.less(ev.Level) {
				if victim == -1 || l.buf[i].Level.less(l.buf[victim].Level) {
					victim = i
				}
			}
		}
		if victim == -1 {
			l.dropLocked(ev.Level)
			l.mu.Unlock()
			return
		}
		l.dropLocked(l.buf[victim].Level)
		l.buf = append(l.buf[:victim], l.buf[victim+1:]...)
	}
	l.buf = append(l.buf, ev)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func truncateFields(fields map[string]string, max int) map[string]string {
	out := make(map[string]string, max)
	n := 0
	// Deterministic-ish truncation: keep the first N keys by insertion is
	// not stable for maps; sort for determinism.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		if n >= max {
			break
		}
		out[k] = fields[k]
		n++
	}
	return out
}

func (l *Logger) allowRate(category string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.rates[category]
	if !ok || now.Sub(r.reset) >= rateWindow {
		l.rates[category] = &rateState{count: 1, reset: now}
		return true
	}
	if r.count >= l.ratePM {
		return false
	}
	r.count++
	return true
}

func (l *Logger) drop(lv Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropLocked(lv)
}

func (l *Logger) dropLocked(lv Level) {
	if lv < LevelDebug || lv > LevelError {
		lv = LevelDebug
	}
	l.dropped[lv]++
}

// Dropped reports the per-severity dropped counters (health signal).
func (l *Logger) Dropped() [4]uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

// drain delivers queued records until Close. Delivery failures are
// reported by the sink's own error path; the logger never retries
// unbounded and never blocks the Core.
func (l *Logger) drain() {
	defer close(l.done)
	for {
		l.mu.Lock()
		if len(l.buf) == 0 && l.stopped {
			l.mu.Unlock()
			return
		}
		var ev Event
		if len(l.buf) > 0 {
			ev = l.buf[0]
			l.buf = l.buf[1:]
		}
		l.mu.Unlock()
		if ev.Category != "" && l.sink != nil {
			l.sink(ev)
		}
		if ev.Category == "" {
			<-l.wake // buffer empty: wait for the next emit or close signal
		}
	}
}

// Flush waits (bounded) until the queue is empty; used in the drain safe
// order. It never deadlocks: the wait is bounded by ctx.
func (l *Logger) Flush(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		l.mu.Lock()
		n := len(l.buf)
		l.mu.Unlock()
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Close stops the drain goroutine after delivering what is queued.
func (l *Logger) Close() {
	l.mu.Lock()
	l.stopped = true
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
	<-l.done
}

// ExportBundle renders records for export through the same redaction and
// sanitization rules (C2-15 §4: exported bundles inherit the emit-boundary
// rules) — it never bypasses them by taking raw input.
func ExportBundle(evs []Event, maxRecordBytes int) []Event {
	if maxRecordBytes < 1 {
		maxRecordBytes = 4096
	}
	out := make([]Event, 0, len(evs))
	for _, ev := range evs {
		ev.Fields = Redact(ev.Fields)
		for k, v := range ev.Fields {
			ev.Fields[k] = Sanitize(v, 256)
		}
		ev.Category = Sanitize(ev.Category, 64)
		ev.MessageKey = Sanitize(ev.MessageKey, 128)
		out = append(out, ev)
	}
	return out
}
