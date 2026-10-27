package security

import (
	"net"
	"strings"
)

// Local HTTP control surface (C2-14 §5, where a transport exists): the
// predicates below are the authoritative contract for any local HTTP
// adapter. They are pure functions so adapters cannot drift from the
// contract. The browser-bootstrap UX and launch-mode choice belong to the
// UI track (D-26/D-39) — nothing here invents either.

// ErrBoundary is the stable boundary refusal with a machine reason.
type ErrBoundary struct{ Reason string }

func (e *ErrBoundary) Error() string { return "security: boundary refusal: " + e.Reason }

// Stable boundary reason codes.
const (
	ReasonNonLoopbackBind     = "NON_LOOPBACK_BIND"
	ReasonHostNotAllowed      = "HOST_NOT_ALLOWED"
	ReasonOriginNotAllowed    = "ORIGIN_NOT_ALLOWED"
	ReasonStateChangingGET    = "STATE_CHANGING_GET"
	ReasonCSRFCheckFailed     = "CSRF_CHECK_FAILED"
	ReasonCredentialInURL     = "CREDENTIAL_IN_URL"
	ReasonCredentialInReferer = "CREDENTIAL_IN_REFERER"
	ReasonMissingCSRFToken    = "MISSING_CSRF_TOKEN"
)

// normalizeHost strips any port/bracket suffix and lowercases, returning
// the bare host name or IP.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(host, "[") {
		if h, _, ok := strings.Cut(host, "]"); ok {
			return h[1:]
		}
		return strings.Trim(host, "[]")
	}
	// An unbracketed IPv6 address contains several colons; only a single
	// colon separates a host from its port.
	if strings.Count(host, ":") == 1 {
		host, _, _ = strings.Cut(host, ":")
	}
	return host
}

// IsLoopbackHost reports whether host is a loopback name or address. A
// local control surface binds loopback only by default; any non-loopback
// bind requires explicit operator opt-in plus authentication and TLS — no
// unauthenticated LAN exposure exists in any mode (C2-14 §5).
func IsLoopbackHost(host string) bool {
	host = normalizeHost(host)
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	parsed := net.ParseIP(host)
	return parsed != nil && parsed.IsLoopback()
}

// ValidateBindAddress checks a bind host: loopback is the default; a
// non-loopback bind is refused unless the operator explicitly opted in
// (and the adapter then owes authentication and TLS — the caller must have
// both configured; this predicate records the boundary decision only).
func ValidateBindAddress(host string, nonLoopbackOptIn bool) error {
	if IsLoopbackHost(host) {
		return nil
	}
	if nonLoopbackOptIn {
		// Explicit opt-in: the adapter MUST pair this with authentication
		// and TLS (§12.2 law 6). The predicate marks the choice; the
		// adapter's TLS/auth wiring is separately required.
		return nil
	}
	return &ErrBoundary{Reason: ReasonNonLoopbackBind}
}

// HostHeaderAllowed validates the Host header against the allowlist
// (DNS-rebinding defense, [R34]). An empty allowlist accepts only
// loopback forms. Comparison is exact and case-insensitive; a port
// suffix is stripped before comparison.
func HostHeaderAllowed(hostHeader string, allowlist []string) bool {
	host := normalizeHost(hostHeader)
	if host == "" {
		return false
	}
	if IsLoopbackHost(host) && len(allowlist) == 0 {
		return true
	}
	for _, a := range allowlist {
		if strings.ToLower(strings.TrimSpace(a)) == host {
			return true
		}
	}
	return false
}

// OriginAllowed validates an Origin header (scheme://host[:port]) against
// the allowlist. An empty allowlist accepts only loopback origins. Empty
// origins (non-browser clients) are DENIED for state-changing requests by
// the CSRF path below; reads may use OriginAllowed with an explicit
// allowlist.
func OriginAllowed(origin string, allowlist []string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == "" {
		return false
	}
	if len(allowlist) == 0 {
		rest, ok := strings.CutPrefix(origin, "http://")
		if !ok {
			rest, ok = strings.CutPrefix(origin, "https://")
		}
		if !ok {
			return false
		}
		return IsLoopbackHost(rest)
	}
	for _, a := range allowlist {
		if strings.ToLower(strings.TrimSpace(a)) == origin {
			return true
		}
	}
	return false
}

// mutatingMethods are the methods that may change state. GET, HEAD, and
// OPTIONS must never be state-changing: a state-changing GET is prohibited
// outright (C2-14 §5) — no adapter may register a mutating GET handler.
var mutatingMethods = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// CheckStateChangingGET fails any request whose method cannot change state
// but whose handler would mutate (GET/HEAD/OPTIONS are never state
// changing).
func CheckStateChangingGET(method string, handlerMutates bool) error {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD", "OPTIONS":
		if handlerMutates {
			return &ErrBoundary{Reason: ReasonStateChangingGET}
		}
	}
	return nil
}

// CSRFCheck enforces the state-changing-request protections: origin
// enforcement plus the anti-CSRF token (C2-14 §5). Non-browser same-origin
// local clients (CLI/IPC) pass an explicit sameOrigin=true from their own
// credential check; browser-originated mutations must present a valid
// Origin and the anti-CSRF token.
func CSRFCheck(method, origin, csrfToken, expectedToken string, sameOriginClient bool) error {
	if !mutatingMethods[strings.ToUpper(strings.TrimSpace(method))] {
		return nil // reads are not CSRF-protected mutations
	}
	if sameOriginClient {
		// Authenticated non-browser local client (capability/session
		// credential already proven): origin enforcement does not apply.
		return nil
	}
	if !OriginAllowed(origin, nil) {
		return &ErrBoundary{Reason: ReasonOriginNotAllowed}
	}
	if csrfToken == "" || expectedToken == "" {
		return &ErrBoundary{Reason: ReasonMissingCSRFToken}
	}
	if !credentialMatches(csrfToken, expectedToken) {
		return &ErrBoundary{Reason: ReasonCSRFCheckFailed}
	}
	return nil
}

// CredentialSurfaceRule enforces the bootstrap-credential contract: the
// credential never travels in URLs or Referer values (C2-14 §5). Callers
// pass every URL/Referer surface they are about to emit; any leak is a
// typed refusal.
func CredentialSurfaceRule(credential string, urls []string, referers []string) error {
	if credential == "" {
		return nil
	}
	for _, u := range urls {
		if u != "" && strings.Contains(u, credential) {
			return &ErrBoundary{Reason: ReasonCredentialInURL}
		}
	}
	for _, r := range referers {
		if r != "" && strings.Contains(r, credential) {
			return &ErrBoundary{Reason: ReasonCredentialInReferer}
		}
	}
	return nil
}

// ScrubCredential removes a credential value from arbitrary text (a
// defense-in-depth helper for surfaces that must render messages without
// credential material).
func ScrubCredential(text, credential string) string {
	if credential == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, credential, RedactMarker)
}
