package providers

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/domain/jsonschema"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

// Adapter is one provider integration behind the neutral contract
// (C3-17/C3-21–C3-30). SDK types never escape an adapter; everything
// crossing this interface is the neutral domain/provider vocabulary.
type Adapter interface {
	// ID is the stable provider identifier (provider.ProfileID.Provider).
	ID() string
	// DefaultProfile returns this integration's profile identity for one
	// model (refreshable metadata carried by the profile, not the code).
	DefaultProfile(model string) dprovider.ProfileID
	// Capabilities returns the capability facts for one profile (C3-17):
	// tested claims only; unknown means unsupported.
	Capabilities(p dprovider.ProfileID) dprovider.CapabilitySet
	// AuthHeaderName is the credential header the family uses; the VALUE
	// is resolved at call time and attached only through SetCredentialHeader.
	AuthHeaderName(p dprovider.ProfileID) string
	// SeedDataHandling is the dated official-source profile record
	// (C3-19): direct integrations carry the facts their vendor policies
	// state as of the workspace's dated review round (unknown facts stay
	// unknown and block); custom/local integrations seed ReviewUnknown;
	// the mock fixture seeds its own recorded test classification. Every
	// record's AllowedClassifications is empty — the classification policy
	// is a separate owner decision.
	SeedDataHandling(p dprovider.ProfileID) dprovider.DataHandlingProfile
	// NewRequest builds the wire request for one plan. Adapters validate
	// their URL through ValidateDestinationURL and their headers through
	// ValidateExtraHeader; the credential is attached by the executor.
	NewRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error)
	// DecodeResponse maps a complete non-stream response to the neutral
	// result (protocol errors map to typed provider errors).
	DecodeResponse(status int, body []byte, plan *dprovider.RequestPlan) (*dprovider.Result, error)
	// NewStreamDecoder builds the family's SSE decoder (nil when the
	// profile does not support streaming).
	NewStreamDecoder(p dprovider.ProfileID) StreamDecoder
	// TestConnectionRequest builds the bounded, non-destructive readiness
	// probe (C3-18): a small GET or a validation POST, never a generation.
	TestConnectionRequest(ctx context.Context, e *Executor, plan *dprovider.RequestPlan, secret string) (*http.Request, error)
	// InterpretProbe decides what a probe status means for this adapter.
	InterpretProbe(status int) error
	// DefaultConnection returns the adapter's own connection posture: the
	// default base URL, the credential class, and the secret reference.
	// Per-connection records (C3-29) override these at the composition
	// root; the slice never invents a connection.
	DefaultConnection() dprovider.Connection
}

// StreamDelta is one neutral streaming step.
type StreamDelta struct {
	Text      string
	ToolCalls []dprovider.ToolCall
	Finish    dprovider.FinishReason // set when the provider finished
	Usage     dprovider.Usage        // set when the provider reported usage
}

// StreamDecoder consumes SSE lines and emits neutral deltas. Feed receives
// EVERY line of the stream (data:, event:, comments, blanks); decoders
// filter. A provider error frame returns a typed error (never a fake
// success).
type StreamDecoder interface {
	Feed(line string) ([]StreamDelta, error)
}

// DefaultCallTimeout bounds one provider call when the plan carries none
// (Proposed).
const DefaultCallTimeout = 120 * time.Second

// TestConnectionTimeout bounds the readiness probe (C3-18: bounded and
// non-destructive) (Proposed).
const TestConnectionTimeout = 10 * time.Second

// Executor executes provider calls for ONE adapter under the full gate set:
// plan validation, capability check, per-profile data-handling gate, secret
// resolution through the boundary, the egress guard, size caps, and the
// schema validation of structured output before any result escapes
// (C3-17–C3-20).
type Executor struct {
	Adapter  Adapter
	Policy   EgressPolicy
	Secrets  security.SecretStore
	Registry *dprovider.Registry
	Client   *http.Client
}

// NewExecutor builds an executor; a nil registry is an error — the
// data-handling gate is never optional.
func NewExecutor(a Adapter, policy EgressPolicy, secrets security.SecretStore, reg *dprovider.Registry) (*Executor, error) {
	if a == nil {
		return nil, fmt.Errorf("providers: executor requires an adapter")
	}
	if reg == nil {
		return nil, fmt.Errorf("providers: executor requires a data-handling registry (fail-closed gate)")
	}
	if secrets == nil {
		secrets = security.UnavailableStore{}
	}
	return &Executor{
		Adapter:  a,
		Policy:   policy,
		Secrets:  secrets,
		Registry: reg,
		Client:   NewClient(policy),
	}, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// ProfileFor reports the adapter's profile identity for one model (the
// identity the capability and data-handling gates key on).
func (e *Executor) ProfileFor(model string) dprovider.ProfileID {
	return e.Adapter.DefaultProfile(model)
}

// Connection returns the adapter's default connection posture.
func (e *Executor) Connection() dprovider.Connection {
	return e.Adapter.DefaultConnection()
}

// Execute performs one bounded, capability-checked, gated provider call.
func (e *Executor) Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error) {
	if plan == nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "nil plan")
	}
	if err := plan.Validate(); err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), err.Error())
	}
	secret, err := e.gate(ctx, plan)
	if err != nil {
		return nil, err
	}
	req, err := e.Adapter.NewRequest(ctx, e, plan, secret)
	if err != nil {
		return nil, e.redactErr(err, secret)
	}
	if req == nil {
		// Fixture adapters (no network) return a nil request: the result
		// comes from the decoder path with every gate already applied.
		result, derr := e.Adapter.DecodeResponse(http.StatusOK, nil, plan)
		if derr != nil {
			return nil, e.redactErr(derr, secret)
		}
		return e.finalize(plan, result, nil)
	}
	callCtx, cancel := e.callDeadline(ctx, plan)
	defer cancel()
	resp, err := e.Client.Do(req.WithContext(callCtx))
	if err != nil {
		return nil, e.redactErr(classifyTransportError(err), secret)
	}
	defer resp.Body.Close()
	body, rerr := cappedBody(resp.Body, e.Policy)
	if rerr != nil {
		return nil, e.redactErr(rerr, secret)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, e.redactErr(mapStatusError(e.Adapter.ID(), resp.StatusCode, body, retryAfterFrom(resp.Header)), secret)
	}
	result, err := e.Adapter.DecodeResponse(resp.StatusCode, body, plan)
	if err != nil {
		return nil, e.redactErr(err, secret)
	}
	return e.finalize(plan, result, body)
}

// finalize stamps the digests and enforces the pre-commit structured-output
// validation (C3-20 acceptance). Stream paths compute RawDigest themselves;
// the body-based path passes the raw response bytes here.
func (e *Executor) finalize(plan *dprovider.RequestPlan, result *dprovider.Result, rawBody []byte) (*dprovider.Result, error) {
	if result.RawDigest == "" {
		result.RawDigest = sha256Hex(rawBody)
	}
	result.ResultDigest = sha256Hex(result.ResultPayload())
	// Structured output is validated against the selected schema BEFORE the
	// result escapes to any commit path. A schema failure is a non-retryable
	// protocol failure: the provider answered, the answer just does not
	// satisfy the contract.
	if plan.Structured != nil {
		if len(result.StructuredJSON) == 0 {
			return nil, dprovider.NewError(dprovider.ErrProtocol, e.Adapter.ID(),
				"structured output requested but the response carries no JSON document")
		}
		if err := jsonschema.Validate(result.StructuredJSON, plan.Structured.Schema); err != nil {
			return nil, dprovider.NewError(dprovider.ErrProtocol, e.Adapter.ID(),
				"generated output failed the selected output schema: "+bounded(err.Error()))
		}
	}
	return result, nil
}

// ExecuteStream performs one bounded streaming call with the delta
// callback. The full gate set applies before any byte moves.
func (e *Executor) ExecuteStream(ctx context.Context, plan *dprovider.RequestPlan, onDelta func(StreamDelta) error) (*dprovider.Result, error) {
	if plan == nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "nil plan")
	}
	if err := plan.Validate(); err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), err.Error())
	}
	return runStream(e, ctx, plan, onDelta)
}

// callDeadline derives the bounded call context: the plan timeout, else the
// default; never longer than the caller's own deadline.
func (e *Executor) callDeadline(ctx context.Context, plan *dprovider.RequestPlan) (context.Context, context.CancelFunc) {
	timeout := plan.Timeout
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// gate runs the pre-call gate set and returns the resolved credential
// (empty when AuthNone). Order: validate → capability → data-handling →
// secret resolution (C3-17/C3-18/C3-19/C3-20).
func (e *Executor) gate(ctx context.Context, plan *dprovider.RequestPlan) (string, error) {
	feature := dprovider.FeatureForPlan(plan)
	if !e.Adapter.Capabilities(plan.Profile).Supported(feature) {
		return "", dprovider.NewError(dprovider.ErrUnsupported, e.Adapter.ID(),
			"capability "+string(feature)+" is not supported (unknown means unsupported)")
	}
	if err := e.Registry.EgressAllowed(plan.Profile, feature, plan.Metadata[dprovider.ClassificationKey]); err != nil {
		if pe, ok := err.(*dprovider.ProviderError); ok {
			pe.Provider = e.Adapter.ID()
		}
		return "", err
	}
	if plan.Connection.AuthKind == dprovider.AuthNone {
		return "", nil
	}
	handle, err := parseSecretRef(plan.Connection.SecretRef)
	if err != nil {
		return "", dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "secret reference does not parse")
	}
	secretBytes, err := security.ResolveForUse(ctx, e.Secrets, handle)
	if err != nil {
		return "", dprovider.NewError(dprovider.ErrAuth, e.Adapter.ID(), "credential unavailable through the secret boundary")
	}
	return string(secretBytes), nil
}

// parseSecretRef splits the "scope/name" handle reference form. It is a
// local helper: the security package owns the Handle type, this package
// only re-forms it from the connection record.
func parseSecretRef(ref string) (security.Handle, error) {
	scope, name, ok := strings.Cut(ref, "/")
	if !ok || scope == "" || name == "" {
		return security.Handle{}, fmt.Errorf("providers: secret reference must be scope/name")
	}
	return security.Handle{Scope: scope, Name: name}, nil
}

// redactErr scrubs resolved credential bytes out of an error's surfaces.
func (e *Executor) redactErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	if len(secrets) == 0 || secrets[0] == "" {
		return err
	}
	var pe *dprovider.ProviderError
	if errors.As(err, &pe) {
		pe.Detail = redact(pe.Detail, secrets...)
		return pe
	}
	return dprovider.NewError(dprovider.ErrUnknown, e.Adapter.ID(), redact(err.Error(), secrets...))
}

// redact replaces every occurrence of each secret with a marker.
func redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec == "" {
			continue
		}
		s = strings.ReplaceAll(s, sec, "[REDACTED]")
	}
	return s
}

// classifyTransportError maps client/transport failures to the neutral
// taxonomy (C3-17/C3-18 evidence classes).
func classifyTransportError(err error) *dprovider.ProviderError {
	var pe *dprovider.ProviderError
	if errors.As(err, &pe) {
		return pe
	}
	if errors.Is(err, errRedirectRefused) {
		return dprovider.NewError(dprovider.ErrEgressPolicy, "egress", errRedirectRefused.Error())
	}
	if errors.Is(err, context.Canceled) {
		return dprovider.NewError(dprovider.ErrCanceled, "egress", "call canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return dprovider.NewError(dprovider.ErrTimeout, "egress", "call deadline exceeded")
	}
	var tlsErr *tlsHandshakeError
	if errors.As(err, &tlsErr) {
		return dprovider.NewError(dprovider.ErrTLS, "egress", "TLS handshake failed: "+bounded(err.Error()))
	}
	return dprovider.NewError(dprovider.ErrUnreachable, "egress", bounded(err.Error()))
}

type tlsHandshakeError struct{ msg string }

func (e *tlsHandshakeError) Error() string { return e.msg }

// bounded caps a detail string.
func bounded(s string) string {
	const max = 512
	if len(s) > max {
		return s[:max]
	}
	return s
}

// mapStatusError maps a non-2xx HTTP status plus body to the neutral error
// taxonomy. The body contributes only bounded, redacted detail.
func mapStatusError(providerID string, status int, body []byte, retryAfter time.Duration) *dprovider.ProviderError {
	kind := dprovider.ErrUnreachable
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		kind = dprovider.ErrAuth
	case status == http.StatusNotFound:
		kind = dprovider.ErrInvalidRequest
	case status == http.StatusRequestTimeout:
		kind = dprovider.ErrTimeout
	case status == http.StatusTooManyRequests:
		kind = dprovider.ErrRateLimited
	case status >= 400 && status < 500:
		kind = dprovider.ErrInvalidRequest
	case status >= 500:
		kind = dprovider.ErrUnreachable
	}
	pe := dprovider.NewError(kind, providerID, fmt.Sprintf("provider returned status %d: %s", status, bounded(strings.TrimSpace(string(body)))))
	pe.Status = status
	if retryAfter > 0 {
		pe.RetryAfter = retryAfter
	}
	return pe
}

// retryAfterFrom parses a Retry-After header (seconds form only; the HTTP
// date form is treated as absent rather than guessed).
func retryAfterFrom(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

// sseLines streams capped lines from a response body.
func sseLines(body io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), DefaultMaxStreamLineBytes)
	return sc
}

// runStream drives one streaming call: gates, request, guarded HTTP, the
// family decoder, and the no-false-success rule — a stream that ends
// without a finish reason is a mid-stream failure (retryable), never a
// success with partial output presented as whole.
func runStream(e *Executor, ctx context.Context, plan *dprovider.RequestPlan, onDelta func(StreamDelta) error) (*dprovider.Result, error) {
	secret, err := e.gate(ctx, plan)
	if err != nil {
		return nil, err
	}
	dec := e.Adapter.NewStreamDecoder(plan.Profile)
	if dec == nil {
		return nil, dprovider.NewError(dprovider.ErrUnsupported, e.Adapter.ID(), "adapter has no stream decoder")
	}
	req, err := e.Adapter.NewRequest(ctx, e, plan, secret)
	if err != nil {
		return nil, e.redactErr(err, secret)
	}
	callCtx, cancel := e.callDeadline(ctx, plan)
	defer cancel()

	var (
		sc      *bufio.Scanner
		rawHash = sha256.New()
		resp    *http.Response
	)
	if req == nil {
		// Fixture adapters drive the decoder with one synthetic frame.
		sc = bufio.NewScanner(strings.NewReader("data: fixture-frame\n"))
	} else {
		resp, err = e.Client.Do(req.WithContext(callCtx))
		if err != nil {
			return nil, e.redactErr(classifyTransportError(err), secret)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			body, rerr := cappedBody(resp.Body, e.Policy)
			if rerr != nil {
				return nil, e.redactErr(rerr, secret)
			}
			return nil, e.redactErr(mapStatusError(e.Adapter.ID(), resp.StatusCode, body, retryAfterFrom(resp.Header)), secret)
		}
		sc = sseLines(resp.Body)
	}

	var (
		text       strings.Builder
		toolCalls  []dprovider.ToolCall
		finish     dprovider.FinishReason
		usage      dprovider.Usage
		finishSeen bool
	)
	for sc.Scan() {
		line := sc.Text()
		rawHash.Write([]byte(line))
		rawHash.Write([]byte{'\n'})
		if plan.Structured != nil {
			// Structured streaming is not part of the Phase 3 subset: the
			// schema can only be validated over a complete document.
			return nil, dprovider.NewError(dprovider.ErrUnsupported, e.Adapter.ID(),
				"streaming with structured output is unsupported (schema must be validated over a complete document)")
		}
		deltas, derr := dec.Feed(line)
		if derr != nil {
			return nil, e.redactErr(derr, secret)
		}
		for _, d := range deltas {
			if d.Text != "" {
				text.WriteString(d.Text)
			}
			if len(d.ToolCalls) > 0 {
				toolCalls = append(toolCalls, d.ToolCalls...)
			}
			if d.Finish != "" && d.Finish != dprovider.FinishUnknown {
				finish = d.Finish
				finishSeen = true
			}
			if d.Usage != (dprovider.Usage{}) {
				usage = d.Usage
			}
			if onDelta != nil {
				if err := onDelta(d); err != nil {
					return nil, e.redactErr(err, secret)
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, e.redactErr(classifyTransportError(err), secret)
			}
		}
	}
	if serr := sc.Err(); serr != nil {
		return nil, e.redactErr(classifyStreamReadError(serr), secret)
	}
	if !finishSeen {
		return nil, dprovider.NewError(dprovider.ErrMidStreamFailure, e.Adapter.ID(),
			"stream ended without a finish reason; partial output is not presented as success")
	}
	result := &dprovider.Result{
		Text:         text.String(),
		ToolCalls:    toolCalls,
		FinishReason: finish,
		Usage:        usage,
		RawDigest:    fmt.Sprintf("sha256:%s", hex.EncodeToString(rawHash.Sum(nil))),
	}
	return e.finalize(plan, result, nil)
}

func classifyStreamReadError(err error) *dprovider.ProviderError {
	if errors.Is(err, bufio.ErrTooLong) {
		return dprovider.NewError(dprovider.ErrResponseTooLarge, "egress", "stream line exceeds the bound")
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return dprovider.NewError(dprovider.ErrMidStreamFailure, "egress", "stream closed unexpectedly")
	}
	return dprovider.NewError(dprovider.ErrUnreachable, "egress", bounded(err.Error()))
}

// TestConnection performs the bounded, non-destructive readiness probe
// (C3-18 acceptance). It never generates content and never persists state.
func (e *Executor) TestConnection(ctx context.Context, plan *dprovider.RequestPlan) error {
	if plan == nil {
		return dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "nil plan")
	}
	if err := plan.Validate(); err != nil {
		return dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), err.Error())
	}
	if _, err := e.gate(ctx, plan); err != nil {
		return err
	}
	var secret string
	if plan.Connection.AuthKind != dprovider.AuthNone {
		handle, err := parseSecretRef(plan.Connection.SecretRef)
		if err != nil {
			return dprovider.NewError(dprovider.ErrInvalidRequest, e.Adapter.ID(), "secret reference does not parse")
		}
		b, err := security.ResolveForUse(ctx, e.Secrets, handle)
		if err != nil {
			return dprovider.NewError(dprovider.ErrAuth, e.Adapter.ID(), "credential unavailable through the secret boundary")
		}
		secret = string(b)
	}
	req, err := e.Adapter.TestConnectionRequest(ctx, e, plan, secret)
	if err != nil {
		return e.redactErr(err, secret)
	}
	if req == nil {
		// Fixture adapters have nothing to probe.
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, TestConnectionTimeout)
	defer cancel()
	resp, err := e.Client.Do(req.WithContext(probeCtx))
	if err != nil {
		return e.redactErr(classifyTransportError(err), secret)
	}
	defer resp.Body.Close()
	body, rerr := cappedBody(resp.Body, e.Policy)
	if rerr != nil {
		return e.redactErr(rerr, secret)
	}
	_ = body
	if err := e.Adapter.InterpretProbe(resp.StatusCode); err != nil {
		return e.redactErr(err, secret)
	}
	return nil
}
