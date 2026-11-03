package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// The provider-connection record (the owner's card-10 working candidate of
// 2026-11-02 — the P3.2 §3.3 basis implemented by the connection-record
// milestone): one durable BYOK connection over a WIRED adapter. The record
// stores the secret REFERENCE only (the keyring Handle string form —
// handle-only on disk per the accepted D-37 policy); a secret VALUE never
// enters the record, the wire, a log, an error, or a test. The tested
// state rides the tested-at inputs digest (the P3.3 §4.3 [P] row, the
// working candidate recorded for the freeze) so "tested, but changed
// since" renders truthfully without ever digesting the secret VALUE — the
// credential change is tracked by the credential epoch counter.

// ConnectionHeader is one optional restricted custom header (the UI-12
// LOCKED Custom API field set). Values are stored on the record; a
// secret-bearing header value follows the same write-only rules as the
// credential (it is never echoed back by any read).
type ConnectionHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ConnectionRecord is one durable provider-connection record. Instants are
// RFC 3339 UTC strings as the storage layer writes them; an empty
// tested_at means the connection has never passed a test (Saved-untested —
// a first-class state per P3.3 §4.3). The fingerprint carries the
// connection-create idempotency digest (a one-way sha256 over the request
// form INCLUDING the credential bytes — never a value, never reversible)
// and is internal: it is never marshaled to the wire.
type ConnectionRecord struct {
	ConnectionID   string             `json:"connection_id"`
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
	Provider       string             `json:"provider"`
	BaseURL        string             `json:"base_url,omitempty"`
	ModelID        string             `json:"model_id"`
	CustomHeaders  []ConnectionHeader `json:"custom_headers,omitempty"`
	// DisplayName is the record's optional user-facing display label (the
	// M-27 register row's rename portion as implemented — the adopted S8
	// supply's working candidate). A rename changes ONLY this field: never
	// the connection identity, never the Job identity, never the execution
	// history (the P3.1 M-27 row's binding rule). Empty means untitled —
	// the surfaces then render the provider/model identity as before.
	DisplayName     string `json:"display_name,omitempty"`
	SecretRef       string `json:"secret_ref"`
	CredentialEpoch int    `json:"credential_epoch"`
	Fingerprint     string `json:"-"`
	TestedAt        string `json:"tested_at,omitempty"`
	TestedDigest    string `json:"tested_digest,omitempty"`
	TestedEpoch     int    `json:"tested_epoch,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// Tested reports whether a successful bounded test exists for the record.
func (r ConnectionRecord) Tested() bool { return r.TestedAt != "" }

// inputsDigest computes the tested-at inputs digest (the P3.3 §4.3 [P]
// row's working candidate): a one-way digest over the connection's
// non-secret inputs — provider, base URL, model id, custom headers — in
// their canonical field order. The credential value is NEVER an input
// (D-37: no value-derived material on disk); a credential change is
// tracked by the credential epoch instead.
func (r ConnectionRecord) InputsDigest() string {
	h := sha256.New()
	h.Write([]byte("karyora-connection-inputs-v1\n"))
	h.Write([]byte(r.Provider + "\n"))
	h.Write([]byte(r.BaseURL + "\n"))
	h.Write([]byte(r.ModelID + "\n"))
	for _, hd := range r.CustomHeaders {
		h.Write([]byte(hd.Name + "\x00" + hd.Value + "\n"))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// TestCurrent reports whether the record's tested result still describes
// its current inputs: tested, AND the tested-at digest matches the current
// non-secret inputs, AND the tested credential epoch matches the current
// one. A rotation/re-authentication bumps the credential epoch, so a
// formerly verified test renders stale-truthfully (P3.3 §4.3
// "Untested (after change)").
func (r ConnectionRecord) TestCurrent() bool {
	if !r.Tested() {
		return false
	}
	return r.TestedDigest == r.InputsDigest() && r.TestedEpoch == r.CredentialEpoch
}

// FingerprintOf computes the connection-create idempotency fingerprint:
// a one-way sha256 over the request form including the credential bytes.
// The digest is stored; the credential bytes are not — they exist only for
// the computation and the single SecretWriter store that follows.
func FingerprintOf(provider, baseURL, modelID string, headers []ConnectionHeader, secret []byte) string {
	h := sha256.New()
	h.Write([]byte("karyora-connection-save-v1\n"))
	h.Write([]byte(provider + "\n"))
	h.Write([]byte(baseURL + "\n"))
	h.Write([]byte(modelID + "\n"))
	for _, hd := range headers {
		h.Write([]byte(hd.Name + "\x00" + hd.Value + "\n"))
	}
	h.Write(secret)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// forbiddenConnectionHeaders are the header names a CONNECTION record may
// never set (C3-18): the Host override, hop-by-hop machinery, the proxy/
// forwarded/internal-routing family, and every credential-bearing header
// the adapter itself must own. Comparison is case-insensitive.
var forbiddenConnectionHeaders = map[string]bool{
	"host": true, "content-length": true, "connection": true,
	"transfer-encoding": true, "upgrade": true, "te": true, "trailer": true,
	"keep-alive": true, "proxy-connection": true, "proxy-authorization": true,
	"proxy-authenticate": true, "expect": true, "authorization": true,
	"x-api-key": true, "x-goog-api-key": true,
	"x-forwarded-for": true, "x-forwarded-host": true, "x-forwarded-proto": true,
	"x-forwarded-port": true, "x-forwarded-server": true, "x-real-ip": true,
	"forwarded": true, "via": true, "cookie": true, "set-cookie": true,
}

// ValidateConnectionHeader refuses a connection-supplied header that is
// forbidden or malformed. Header VALUE hygiene (no newlines) is enforced
// here too — header injection never passes (C3-18 evidence). The rule
// lives in the domain package because the connection-record contract (the
// UI-12 optional custom headers, MB §12.5's restriction) owns it; the
// providers adapter delegates to this single rule.
func ValidateConnectionHeader(name, value string) error {
	if name == "" || strings.ContainsAny(name, " \t\r\n:") {
		return NewError(ErrInvalidRequest, "egress", "malformed header name")
	}
	if forbiddenConnectionHeaders[strings.ToLower(name)] {
		return NewError(ErrEgressPolicy, "egress",
			"connection header "+name+" is forbidden (Host/hop-by-hop/authorization-override/internal-routing)")
	}
	if strings.ContainsAny(value, "\r\n") || len(value) > 1024 {
		return NewError(ErrInvalidRequest, "egress", "header value for "+name+" is malformed or oversized")
	}
	return nil
}

// ValidateHeaders applies the Core-side restriction of the optional custom
// headers (MB §12.5: custom header names/values restricted): the header
// grammar is the code's own connection-header rule (a forbidden or
// malformed header never passes; header injection never passes), the set
// is bounded, and every name/value is bounded.
func ValidateHeaders(headers []ConnectionHeader) error {
	const maxHeaders = 16
	if len(headers) > maxHeaders {
		return fmt.Errorf("provider: connection headers exceed the %d-entry bound", maxHeaders)
	}
	for _, hd := range headers {
		if err := ValidateConnectionHeader(hd.Name, hd.Value); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRecord applies the connection-record shape rules every write
// enforces (the request-shape law: typed refusals, never silent coercion).
func ValidateRecord(r ConnectionRecord) error {
	if strings.TrimSpace(r.ConnectionID) == "" {
		return fmt.Errorf("provider: connection record requires its Core-issued id")
	}
	if strings.TrimSpace(r.Provider) == "" {
		return fmt.Errorf("provider: connection record requires its provider id")
	}
	if strings.ContainsAny(r.Provider, " \t\n\r") {
		return fmt.Errorf("provider: connection provider id is malformed")
	}
	if strings.Contains(r.BaseURL, "@") {
		return fmt.Errorf("provider: connection base URL must not embed credentials")
	}
	if strings.TrimSpace(r.ModelID) == "" {
		return fmt.Errorf("provider: connection record requires the exact Model ID")
	}
	if len(r.ModelID) > 256 {
		return fmt.Errorf("provider: connection model id is unbounded")
	}
	if len(r.BaseURL) > 2048 {
		return fmt.Errorf("provider: connection base URL is unbounded")
	}
	if strings.TrimSpace(r.SecretRef) == "" {
		return fmt.Errorf("provider: connection record requires its secret reference")
	}
	return ValidateHeaders(r.CustomHeaders)
}

// ValidateDisplayName applies the display-name law the M-27 rename
// enforces (the adopted S8 supply's working candidate; the bound class the
// record's other bounded fields ride): a display name is optional, is
// trimmed before it is stored, and is bounded. An empty TRIMMED name
// refuses — a rename that clears the label is a clearing, not a rename,
// and the typed refusal names it so. The connection identity and the
// execution history are never touched by this law's caller (the M-27
// rename row's binding rule).
func ValidateDisplayName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("provider: connection display name must not be empty")
	}
	if len(trimmed) > 256 {
		return fmt.Errorf("provider: connection display name is unbounded (max 256)")
	}
	return nil
}

// ErrConnectionStaleMarker is the concurrent-edit guard's store-side fence
// signal (the adopted S8 supply's working candidate — the chosen class is
// the P3.3-recorded typed `CONFLICT_STATE` guard with `currentState`, the
// P1.6 §5 recorded typed-outcome class): a rename/update's conditional
// write matched no row because the record's `updated_at` marker moved
// since the caller read it. The store separates not-found from the stale
// marker by re-reading; the use-case layer converts this signal into the
// typed CONFLICT_STATE envelope whose data carries the record's current
// state (safe metadata only — never secret content, P1.6 §5's rule 9).
var ErrConnectionStaleMarker = errors.New("provider: connection record changed since the caller read it")

// ConnectionTestReport is the read-only test-connection report (M-25; the
// P3.2 §3.3 [S] rules): the verified endpoint/model + the capability
// statuses the wired executor reports + the sanitized bounded error +
// the correlation ID. No secret value can appear in any field: the
// executor redacts resolved credential bytes out of error details before
// construction (C3-18), and this report never carries a credential.
type ConnectionTestReport struct {
	OK            bool           `json:"ok"`
	Provider      string         `json:"provider"`
	ModelID       string         `json:"model_id"`
	BaseURL       string         `json:"base_url,omitempty"`
	Capabilities  *CapabilitySet `json:"capabilities,omitempty"`
	ErrorKind     string         `json:"error_kind,omitempty"`
	ErrorDetail   string         `json:"error_detail,omitempty"`
	CorrelationID string         `json:"correlation_id"`
}

// ListConnectionsQuery is the server-side connection-list query (the
// card-9 working candidate's list discipline: a Core-defined sort, a
// bounded page, an opaque keyset position). It lives in the domain
// package so the storage adapter and the use-case layer share one shape.
type ListConnectionsQuery struct {
	Limit             int
	AfterCreatedAt    string
	AfterConnectionID string
}

// ListConnectionsPage is one bounded page; More marks a further page.
type ListConnectionsPage struct {
	Connections []ConnectionRecord
	More        bool
}

// UpdateConnectionFields is one non-secret field update's field set (the
// adopted S8 supply's P-S8-02 working candidate): EXACTLY the coded create
// shape's non-secret fields. No secret field exists on this shape — the
// secret path stays the coded rotate/re-authenticate contract, and the
// record's secret reference, credential epoch, and create fingerprint are
// carried over untouched by any updater. It lives in the domain package so
// the storage adapter and the use-case layer share one shape (the
// ListConnectionsQuery precedent).
type UpdateConnectionFields struct {
	Provider      string
	BaseURL       string
	ModelID       string
	CustomHeaders []ConnectionHeader
}
