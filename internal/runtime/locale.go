package runtime

import (
	"strings"
)

// Locale resolves human-facing CLI copy from the accepted 12-locale resource
// set (UI-18): English is the source/default; a requested locale falls back
// to English for any missing key (C2-07 §5). Command identifiers, exit
// identifiers, JSON keys, and fixed log fields stay English in every locale;
// the Phase 2 repository ships the English source catalog, and the
// additional locale catalogs arrive with the UI locale resources (C2-18
// matrix checks all 12 keys and the fallback).
type Locale struct {
	name  string
	table map[string]string
}

// supportedLocales are the UI-18 identifiers; every unresolved one falls
// back to English. The non-English catalogs are UI-track resources; asking
// for one today resolves English (visible fallback, never invented copy).
var supportedLocales = map[string]bool{
	"en": true, "zh-CN": true, "es": true, "pt-BR": true, "ja": true,
	"ru": true, "ko": true, "de": true, "fr": true, "id": true,
	"tr": true, "vi": true,
}

// Catalog keys (stable English identifiers).
const (
	KeyHelpUsage         = "help.usage"
	KeyCmdStart          = "cmd.start"
	KeyCmdStop           = "cmd.stop"
	KeyCmdStatus         = "cmd.status"
	KeyCmdDoctor         = "cmd.doctor"
	KeyStatusLive        = "status.live"
	KeyStatusNotRunning  = "status.not_running"
	KeyStatusOwner       = "status.owner"
	KeyStatusTakeover    = "status.takeover"
	KeyStatusState       = "status.state"
	KeyStatusLock        = "status.lock_held"
	KeyStatusMigrations  = "status.migrations_applied"
	KeyStatusRecovery    = "status.recovery_complete"
	KeyStatusStorage     = "status.storage_ok"
	KeyStatusWALStarving = "status.wal_starving"
	KeyWordYes           = "word.yes"
	KeyWordNo            = "word.no"
	KeyDoctorOverall     = "doctor.overall"
	KeyStopCompleted     = "stop.completed"
	KeyStopIncomplete    = "stop.incomplete"
	KeyErrGeneric        = "error.generic"
)

// enCatalog is the English source catalog: the only copy the CLI renders
// until the UI locale resources supply the other eleven locales.
var enCatalog = map[string]string{
	KeyHelpUsage:         "usage: uawp <command> [--json] [--data-dir DIR]\ncommands: start, stop, status, doctor, version",
	KeyCmdStart:          "start the Core in the foreground",
	KeyCmdStop:           "request a graceful stop of the running Core",
	KeyCmdStatus:         "print the read-only status report",
	KeyCmdDoctor:         "run read-only local diagnostics (never repairs)",
	KeyStatusLive:        "core is running",
	KeyStatusNotRunning:  "core is not running",
	KeyStatusOwner:       "owner",
	KeyStatusTakeover:    "(stale-owner takeover)",
	KeyStatusState:       "state",
	KeyStatusLock:        "lock held",
	KeyStatusMigrations:  "migrations applied",
	KeyStatusRecovery:    "recovery complete",
	KeyStatusStorage:     "storage writable",
	KeyStatusWALStarving: "warning: WAL checkpoint starvation detected",
	KeyWordYes:           "yes",
	KeyWordNo:            "no",
	KeyDoctorOverall:     "overall",
	KeyStopCompleted:     "core stopped gracefully",
	KeyStopIncomplete:    "stop did not complete within the budget; the next start reconciles via crash recovery",
	KeyErrGeneric:        "command failed: code {{code}} (correlationId {{correlationId}})",
}

// NewLocale returns the locale for name, falling back to English for any
// unsupported name or missing key (never localized prose in JSON mode).
func NewLocale(name string) Locale {
	name = strings.TrimSpace(name)
	if name == "" || !supportedLocales[name] {
		name = "en"
	}
	// The English catalog is the only table in the Phase 2 repository; when
	// the UI locale resources land, per-locale tables load here and the
	// lookup falls back key-by-key to English.
	return Locale{name: name, table: enCatalog}
}

// Name reports the resolved locale identifier.
func (l Locale) Name() string { return l.name }

// Text resolves one key with English fallback.
func (l Locale) Text(key string) string {
	if s, ok := l.table[key]; ok {
		return s
	}
	return enCatalog[key]
}

// ErrorText maps a machine error envelope to bounded human copy, resolving
// from code + typed data (data is never rendered raw as prose, C2-07 §5).
func (l Locale) ErrorText(code, correlationID string) string {
	t := l.Text(KeyErrGeneric)
	t = strings.ReplaceAll(t, "{{code}}", code)
	t = strings.ReplaceAll(t, "{{correlationId}}", correlationID)
	return t
}
