package httplocal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/security"
)

// maxBodyBytes is the adapter's transport-level request-body bound: well
// above the slice's own task/system validation bounds (a 1<<20-rune task is
// at most ~4 MiB of UTF-8) and below the CAS object default, so the
// use-case layer's own bounds remain the authoritative ones.
const maxBodyBytes = 32 << 20

// maxReplayLimit is the adapter's transport-level replay-read bound (the
// Replay port's limit parameter). It is a transport safeguard, not a
// contract value: no list-pagination default exists in code.
const maxReplayLimit = 1000

// maxListCursorLen bounds a Jobs-list cursor query value (the opaque list
// cursor is a salt-hex envelope of a small JSON keyset — well under this
// bound; anything longer is refused before decoding). A transport
// safeguard, not a contract value.
const maxListCursorLen = 512

// maxSearchQueryLen bounds a conversation search-term query value (a
// transport safeguard; the domain's own content bound is the authoritative
// one).
const maxSearchQueryLen = 1 << 16

// maxPathID bounds a Job id or stream name parsed from a path/query value.
const maxPathID = 128

// ReplayReader is the journal replay port (C2-13 §5): the coded replay read
// with its explicit expiry/gap outcomes. The composition root wires the
// storage adapter; this package never imports it.
type ReplayReader interface {
	Replay(ctx context.Context, stream string, cursor string, limit int) ([]events.Record, events.ReplayOutcome, error)
}

// Deps wires the transport. Every dependency is required except Log; a
// missing required one fails construction (fail closed).
type Deps struct {
	// Slice is the app operation surface (the same functions every
	// consumer calls; CLI and transport produce identical state/events).
	Slice *app.Slice
	// Sessions is the local session service (C2-14 §3): the exchange
	// consumes its one-time bootstrap credentials exactly as issued.
	Sessions *security.SessionService
	// Guard is the one decision port + audit (C2-14 §2/§6) used for the
	// transport-level replay-read decision; the app ops decide inside the
	// slice through the same Guard.
	Guard *security.Guard
	// Replay is the journal replay read port.
	Replay ReplayReader
	// Log receives safe metadata only (reason codes — never credential
	// material); nil disables it.
	Log func(ev security.Event)
}

// Server is the local HTTP transport adapter.
type Server struct {
	deps Deps

	// issued holds the bootstrap credentials this process issued for
	// exchange (hash -> the issued record). The wire exchange resolves the
	// presented bearer secret against this registry so the service's
	// expiry/origin checks run on the ISSUED values, never on
	// client-presented metadata. The service's own consumption bookkeeping
	// keeps the one-time replay rule; records here are dropped after a
	// successful exchange and evicted on expiry.
	issuedMu sync.Mutex
	issued   map[string]security.BootstrapCredential
}

// New validates the dependencies and builds the server. Route registration
// applies the state-changing-GET prohibition (C2-14 §5): a mutating GET
// handler cannot be constructed at all.
func New(deps Deps) (*Server, error) {
	if deps.Slice == nil || deps.Sessions == nil || deps.Guard == nil || deps.Replay == nil {
		return nil, errors.New("httplocal: slice, sessions, guard, and replay ports are required")
	}
	if err := validateRoutes(routes()); err != nil {
		return nil, err
	}
	return &Server{deps: deps, issued: map[string]security.BootstrapCredential{}}, nil
}

// IssueBootstrapCredential mints the one-time bootstrap credential through
// the in-process trusted-local channel (same OS user, same data directory)
// and records it for exchange. It is a Go method, never an HTTP endpoint:
// how the credential reaches a client is the trusted-local bootstrap
// channel's concern (the browser-bootstrap UX and launch-mode choice belong
// to the UI track, D-26/D-39). The origin binds the exchange: empty binds
// to the local process boundary; non-empty binds it to that HTTP origin.
func (s *Server) IssueBootstrapCredential(origin string) (security.BootstrapCredential, error) {
	cred, err := s.deps.Sessions.IssueBootstrapCredential(origin)
	if err != nil {
		return security.BootstrapCredential{}, err
	}
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	now := time.Now()
	// Bounded bookkeeping: evict expired records first and deny when
	// genuinely full (the session store's own fail-closed pattern, bound
	// by the same coded MaxSessions constant).
	for h, old := range s.issued {
		if now.After(old.Expiry) {
			delete(s.issued, h)
		}
	}
	if len(s.issued) >= security.MaxSessions {
		return security.BootstrapCredential{}, errors.New("httplocal: bootstrap issue bookkeeping exhausted")
	}
	s.issued[hashCredential(cred.Secret)] = cred
	return cred, nil
}

// ValidateBind applies the boundary's bind rule with NO non-loopback
// opt-in: the adapter binds loopback only (a non-loopback bind owes
// authentication and TLS, neither of which this adapter offers).
func ValidateBind(host string) error {
	return security.ValidateBindAddress(host, false)
}

// route is one registered endpoint. mutates drives the state-changing-GET
// prohibition and the CSRF path. noSession marks the exchange route: its
// credential check is the presented one-time bootstrap credential, not a
// session token.
type route struct {
	method    string
	prefix    string // literal path prefix; the remainder is the job id where present
	suffix    string // optional literal suffix after the id
	hasID     bool
	mutates   bool
	noSession bool
	handler   func(s *Server, w http.ResponseWriter, r *http.Request, token, jobID string)
}

// routes is the fixed endpoint table. The paths are this adapter's own: no
// published contract carries them (the canonical-transport question stays
// its recorded open item).
func routes() []route {
	return []route{
		{method: http.MethodPost, prefix: "/v1/session/exchange", mutates: true, noSession: true, handler: (*Server).handleExchange},
		{method: http.MethodPost, prefix: "/v1/jobs/review", mutates: true, handler: (*Server).handleReview},
		{method: http.MethodPost, prefix: "/v1/jobs", mutates: true, handler: (*Server).handleSubmit},
		{method: http.MethodGet, prefix: "/v1/jobs", handler: (*Server).handleList},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/events", handler: (*Server).handleEvents},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/result", handler: (*Server).handleResult},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/snapshot", handler: (*Server).handleJobSnapshot},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/approval", handler: (*Server).handleJobApproval},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, handler: (*Server).handleStatus},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/cancel", mutates: true, handler: (*Server).handleCancel},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/approvals/accept", mutates: true, handler: (*Server).handleApprovalAccept},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/approvals/reject", mutates: true, handler: (*Server).handleApprovalReject},
		{method: http.MethodPost, prefix: "/v1/connections/test", mutates: true, handler: (*Server).handleConnectionTest},
		{method: http.MethodPost, prefix: "/v1/connections", mutates: true, handler: (*Server).handleConnectionSave},
		{method: http.MethodGet, prefix: "/v1/connections", handler: (*Server).handleConnectionList},
		{method: http.MethodGet, prefix: "/v1/connections/", hasID: true, handler: (*Server).handleConnectionGet},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/rotate", mutates: true, handler: (*Server).handleConnectionRotate},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/reauthenticate", mutates: true, handler: (*Server).handleConnectionReauthenticate},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/delete", mutates: true, handler: (*Server).handleConnectionDelete},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/switch", mutates: true, handler: (*Server).handleConnectionSwitch},
		// The conversation surface (the adopted S5 mapping, P-S5-01…05): the
		// aggregate create, the list/search query, the aggregate read, the
		// messages read, the Ask send, and the S4 exchange-status query.
		// The paths are this adapter's own (the IMPL-4/6/7/8/9 convention);
		// no streaming, stop, or steering endpoint exists anywhere (the
		// stream-transport mechanism is the named owner gap, P-S5-03).
		{method: http.MethodPost, prefix: "/v1/conversations", mutates: true, handler: (*Server).handleConversationCreate},
		{method: http.MethodGet, prefix: "/v1/conversations", handler: (*Server).handleConversationList},
		{method: http.MethodGet, prefix: "/v1/conversations/", hasID: true, suffix: "/messages", handler: (*Server).handleConversationMessages},
		{method: http.MethodGet, prefix: "/v1/conversations/", hasID: true, suffix: "/exchange", handler: (*Server).handleConversationExchange},
		{method: http.MethodPost, prefix: "/v1/conversations/", hasID: true, suffix: "/send", mutates: true, handler: (*Server).handleConversationSend},
		{method: http.MethodGet, prefix: "/v1/conversations/", hasID: true, handler: (*Server).handleConversationGet},
		{method: http.MethodGet, prefix: "/v1/events/replay", handler: (*Server).handleReplay},
	}
}

// validateRoutes applies the state-changing-GET prohibition at registration
// time: GET/HEAD/OPTIONS routes can never mutate (C2-14 §5).
func validateRoutes(rs []route) error {
	for _, rt := range rs {
		if err := security.CheckStateChangingGET(rt.method, rt.mutates); err != nil {
			return err
		}
	}
	return nil
}

// match resolves the route for method+path and returns the job id when the
// route carries one. Method mismatches on known paths resolve to NOT_FOUND
// (the register's stable vocabulary; no method-not-allowed code exists).
func match(method, path string) (route, string, bool) {
	for _, rt := range routes() {
		if rt.method != method {
			continue
		}
		rest, ok := strings.CutPrefix(path, rt.prefix)
		if !ok {
			continue
		}
		if rt.hasID {
			id, ok := cutID(rest, rt.suffix)
			if !ok {
				continue
			}
			return rt, id, true
		}
		if rt.suffix != "" || rest != "" {
			continue
		}
		return rt, "", true
	}
	return route{}, "", false
}

func cutID(rest, suffix string) (string, bool) {
	if suffix != "" {
		s, ok := strings.CutSuffix(rest, suffix)
		if !ok {
			return "", false
		}
		rest = s
	}
	if rest == "" || strings.Contains(rest, "/") || len(rest) > maxPathID {
		return "", false
	}
	return rest, true
}

// Handler returns the boundary-enforcing HTTP handler.
func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. The Host-header allowlist (the DNS-rebinding defense): the empty
	// allowlist accepts loopback forms only (the coded default; the
	// production allowlist content stays its recorded open item, C-2).
	if !security.HostHeaderAllowed(r.Host, nil) {
		s.refuse(w, security.ReasonHostNotAllowed)
		return
	}
	// 2. The credential-surface rule: a presented credential never travels
	// in the request URL or Referer (C2-14 §5 / R-3, R-11).
	token := bearerToken(r)
	if err := security.CredentialSurfaceRule(token, []string{r.URL.String()}, []string{r.Referer()}); err != nil {
		s.refuse(w, boundaryReason(err))
		return
	}
	// 3. Route resolution.
	rt, jobID, ok := match(r.Method, r.URL.Path)
	if !ok {
		s.writeEnvelope(w, http.StatusNotFound, envelope(errs.CodeNotFound, errs.NewCorrelationID(),
			map[string]any{"reason": "no such local endpoint"}))
		return
	}
	// 4. CSRF for mutations (reads no-op inside the predicate): a
	// non-browser local client whose session credential validates passes
	// the explicit same-origin carve-out; every other mutation needs a
	// valid Origin AND the anti-CSRF token — and with no token issuance
	// (the recorded open item, C-2) that path refuses fail-closed.
	if rt.mutates && !rt.noSession {
		sameOrigin := token != "" && s.deps.Sessions.Validate(token, security.ScopeData)
		if err := security.CSRFCheck(r.Method, r.Header.Get("Origin"), r.Header.Get("X-CSRF-Token"), "", sameOrigin); err != nil {
			s.refuse(w, boundaryReason(err))
			return
		}
	}
	// 5. The handler runs the use case; the Guard's decision inside the
	// slice (or the replay guard below) stays the authoritative check.
	rt.handler(s, w, r, token, jobID)
}

// handleExchange consumes a one-time bootstrap credential exactly once and
// returns the session token (C-1, the P-S3-01 recorded candidate, on the
// existing security.SessionService). The CSRF check runs here because the
// route's credential check IS the presented bootstrap credential: a
// cross-site page cannot present the 256-bit one-time secret.
func (s *Server) handleExchange(w http.ResponseWriter, r *http.Request, _ string, _ string) {
	var req exchangeRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	cred := s.resolveIssued(req.Credential)
	// The credential-presented carve-out: only a request presenting a
	// bootstrap credential this process actually issued passes the
	// same-origin check; every other mutation-shaped request is refused
	// per the predicate (empty/disallowed Origin -> ORIGIN_NOT_ALLOWED, a
	// loopback Origin without a token -> MISSING_CSRF_TOKEN).
	if err := security.CSRFCheck(r.Method, r.Header.Get("Origin"), r.Header.Get("X-CSRF-Token"), "", cred != nil); err != nil {
		s.refuse(w, boundaryReason(err))
		return
	}
	if cred == nil {
		// An invalid bootstrap token is rejected; safe metadata only (S-3).
		s.writeEnvelope(w, http.StatusForbidden, envelope(errs.CodeAuthzDenied, errs.NewCorrelationID(),
			map[string]any{"reason": security.ReasonSessionInvalid}))
		return
	}
	scopes, invalid := parseScopes(req.Scopes)
	if invalid {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "scopes"}))
		return
	}
	// The service checks expiry and the origin binding (the presented
	// Origin header against the issued origin) and enforces the one-time
	// replay rule: a replayed credential is rejected AND the session the
	// first exchange minted is revoked (C2-14 §3). The issued record stays
	// in the registry until expiry so a replay reaches the service's
	// consumption check instead of looking like an unknown credential.
	tok, err := s.deps.Sessions.Exchange(*cred, r.Header.Get("Origin"), scopes)
	if err != nil {
		s.writeEnvelope(w, http.StatusForbidden, envelope(errs.CodeAuthzDenied, errs.NewCorrelationID(),
			map[string]any{"reason": security.ReasonSessionInvalid}))
		return
	}
	s.writeJSON(w, http.StatusOK, sessionResult{
		Token:   tok.Secret,
		ID:      tok.ID,
		Scopes:  scopeStrings(tok.Scopes),
		Expires: tok.Expires,
	})
}

// resolveIssued resolves a presented bootstrap-credential secret to the
// record this process issued (nil when unknown or past its issue-time
// expiry). The service re-checks expiry and origin on the returned struct.
func (s *Server) resolveIssued(presented string) *security.BootstrapCredential {
	if !strings.HasPrefix(presented, "bootstrap-") || len(presented) < 8 || len(presented) > 256 {
		return nil
	}
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	cred, ok := s.issued[hashCredential(presented)]
	if !ok {
		return nil
	}
	if time.Now().After(cred.Expiry) {
		delete(s.issued, hashCredential(presented))
		return nil
	}
	return &cred
}

// parseScopes maps the requested scope strings onto the coded register;
// unknown or empty requests fail closed.
func parseScopes(raw []string) ([]security.Scope, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	out := make([]security.Scope, 0, len(raw))
	for _, s := range raw {
		switch security.Scope(s) {
		case security.ScopeData:
			out = append(out, security.ScopeData)
		case security.ScopeLifecycle:
			out = append(out, security.ScopeLifecycle)
		default:
			return nil, true
		}
	}
	return out, false
}

func scopeStrings(sc []security.Scope) []string {
	out := make([]string, 0, len(sc))
	for _, s := range sc {
		out = append(out, string(s))
	}
	return out
}

// handleSubmit exposes the durable Job submission (the same idempotency
// semantics the storage layer enforces: same key + same input replays; the
// same key with different input is a typed CONFLICT_STATE).
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req submitRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.Submit(s.callCtx(r, token), app.SubmitRequest{
		IdempotencyKey:  req.IdempotencyKey,
		Task:            req.Task,
		System:          req.System,
		Schema:          req.Schema,
		PackRef:         req.PackRef,
		Provider:        req.Provider,
		Model:           req.Model,
		MaxOutputTokens: req.MaxOutputTokens,
		DeadlineSecs:    req.DeadlineSecs,
		Classification:  req.Classification,
		BaseURL:         req.BaseURL,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, submitResult{Job: res.Job, Replayed: res.Replayed})
}

// handleReview exposes the pre-submit review read (the owner's card-13
// working candidate): the eight review fields the Core projects from its
// own gates and execution path. The route rides the POST class only
// because the draft submission travels in a body (a GET URL cannot carry
// a 1<<20-rune task); its effect class is a READ — it creates no durable
// state — and the slice authorizes it through the same decision port
// every read decides through.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req reviewRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	payload, err := s.deps.Slice.Review(s.callCtx(r, token), app.ReviewRequest{
		Task:            req.Task,
		System:          req.System,
		Schema:          req.Schema,
		PackRef:         req.PackRef,
		Provider:        req.Provider,
		Model:           req.Model,
		MaxOutputTokens: req.MaxOutputTokens,
		Classification:  req.Classification,
		BaseURL:         req.BaseURL,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, reviewResult{Review: *payload})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, token, jobID string) {
	job, attempt, err := s.deps.Slice.Status(s.callCtx(r, token), jobID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, statusResult{Job: job, Attempt: attempt})
}

// handleList exposes the Jobs-list operation (the card-9 working
// candidate): server-side cursor pagination, the Core-defined created-at
// descending order, and the optional registered-state filter. The limit is
// optional (the slice's per-surface default applies); the cursor is opaque
// and its absence on a response marks the final page. Authorization stays
// the slice's decision port — the same Guard every read decides through.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request, token, _ string) {
	q := r.URL.Query()
	limit, lerr := parseListLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	cursor := q.Get("cursor")
	if len(cursor) > maxListCursorLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "cursor"}))
		return
	}
	res, err := s.deps.Slice.List(s.callCtx(r, token), app.ListRequest{
		Limit:  limit,
		Status: q.Get("status"),
		Cursor: cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, listResult{Jobs: res.Jobs, Cursor: res.NextCursor})
}

// parseListLimit parses the OPTIONAL list limit: an absent value selects
// the slice's per-surface default (0 passes through for that purpose); a
// present value must be a positive integer — the page bound is the slice's
// typed validation.
func parseListLimit(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errors.New("httplocal: limit must be a positive integer")
	}
	return n, nil
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, token, jobID string) {
	recs, err := s.deps.Slice.Events(s.callCtx(r, token), jobID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, eventsResult{Events: recs})
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request, token, jobID string) {
	payload, err := s.deps.Slice.Result(s.callCtx(r, token), jobID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// handleJobSnapshot exposes the context-snapshot read (the owner's card-11
// working candidate): one stored snapshot's STRUCTURE per the P3.2 §3.5
// Supplied semantics — the included/omitted lists with their Core-supplied
// reasons, the budget pair, the exact-vs-approximate label, the identity.
// The compiled CONTENT (the assembled representation) never rides this
// read — the view shape excludes it by construction. The optional
// `attempt` query value selects a specific attempt's snapshot; its absence
// resolves the Job's current attempt. Authorization stays the slice's
// decision port; a missing snapshot is the typed NOT_FOUND.
func (s *Server) handleJobSnapshot(w http.ResponseWriter, r *http.Request, token, jobID string) {
	attempt := 0
	if raw := r.URL.Query().Get("attempt"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
				map[string]any{"field": "attempt"}))
			return
		}
		attempt = n
	}
	view, err := s.deps.Slice.JobContextSnapshot(s.callCtx(r, token), jobID, attempt)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, snapshotResult{Snapshot: *view})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request, token, jobID string) {
	if err := s.deps.Slice.Cancel(s.callCtx(r, token), jobID); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, map[string]any{})
}

// handleJobApproval exposes the Job's approval-request read (the card-14
// working candidate's queryable identity): the latest raised approval for
// the Job, authorized through the same decision port every read decides
// through. The read creates nothing and mutates nothing — a notification
// read/dismiss never implies approval.
func (s *Server) handleJobApproval(w http.ResponseWriter, r *http.Request, token, jobID string) {
	approval, err := s.deps.Slice.JobApproval(s.callCtx(r, token), jobID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, approvalResult{Approval: approval})
}

// handleApprovalDecision exposes one approval decision command (the card-14
// working candidate): the Core-issued approval identity rides the body; the
// response is the Core-confirmed post-decision state (requirement (d) —
// the transition is durable when the response returns). The replayed flag
// reports the honest replay (the same decision re-presented returns the
// recorded outcome without re-applying).
func (s *Server) handleApprovalDecision(w http.ResponseWriter, r *http.Request, token, jobID, decision string) {
	var req approvalDecisionRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.DecideApproval(s.callCtx(r, token), app.ApprovalDecisionRequest{
		JobID:      jobID,
		ApprovalID: req.ApprovalID,
		Decision:   decision,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, approvalDecisionResult{Approval: res.Approval, Job: res.Job, Replayed: res.Replayed})
}

func (s *Server) handleApprovalAccept(w http.ResponseWriter, r *http.Request, token, jobID string) {
	s.handleApprovalDecision(w, r, token, jobID, aijobrelay.DecisionAccept)
}

func (s *Server) handleApprovalReject(w http.ResponseWriter, r *http.Request, token, jobID string) {
	s.handleApprovalDecision(w, r, token, jobID, aijobrelay.DecisionReject)
}

// handleReplay exposes the coded journal replay read: the opaque cursor,
// the explicit expiry/gap outcomes, and authorization applied at
// read/replay time (C2-13 §5). The decision port decides (and records)
// before the read; a denial resolves to the coded forbidden outcome so a
// client is never stranded in a false permanent status.
// handleConnectionSave exposes the provider-connection create command
// (M-24; the card-10 working candidate): the Supplied create idempotency —
// duplicate returns the existing connection reference (replayed=true);
// same key + different body is the typed CONFLICT_IDEMPOTENCY_KEY
// (fingerprintMatch:false). The credential VALUE rides the body once —
// the Core stores it through the D-37 SecretWriter and the response
// carries the record with the handle REFERENCE only.
func (s *Server) handleConnectionSave(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req connectionSaveRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SaveConnection(s.callCtx(r, token), app.SaveConnectionRequest{
		IdempotencyKey: req.IdempotencyKey,
		Provider:       req.Provider,
		BaseURL:        req.BaseURL,
		ModelID:        req.ModelID,
		CustomHeaders:  req.CustomHeaders,
		Secret:         []byte(req.Secret),
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionSaveResult{Connection: res.Connection, Replayed: res.Replayed})
}

// handleConnectionList exposes the connection-list read (the card-9 list
// discipline over the connection records): the optional limit, the opaque
// cursor, the cursorless final page. Authorization stays the slice's
// decision port.
func (s *Server) handleConnectionList(w http.ResponseWriter, r *http.Request, token, _ string) {
	q := r.URL.Query()
	limit, lerr := parseListLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	cursor := q.Get("cursor")
	if len(cursor) > maxListCursorLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "cursor"}))
		return
	}
	res, err := s.deps.Slice.ListConnections(s.callCtx(r, token), app.ListConnectionsRequest{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionListResult{Connections: res.Connections, Cursor: res.NextCursor})
}

// handleConnectionGet exposes one connection record's read (the masked
// secret status + the tested-state truth included; an unknown id is the
// typed NOT_FOUND).
func (s *Server) handleConnectionGet(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	view, err := s.deps.Slice.GetConnection(s.callCtx(r, token), connectionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionGetResult{Connection: view})
}

// handleConnectionTest exposes the read-only test command (M-25): either a
// saved connection (the id in the body) or an unsaved safe draft (the
// typed secret rides the body once — sent only by this explicit action,
// dropped after the probe, never stored, never echoed). The route rides
// the POST class only because the draft (and its transient credential)
// travels in a body; its effect class is a READ — safe re-issue, a lost
// response proves nothing — and the slice authorizes it through the same
// decision port every read decides through.
func (s *Server) handleConnectionTest(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req connectionTestRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.TestConnection(s.callCtx(r, token), app.TestConnectionRequest{
		ConnectionID:  req.ConnectionID,
		Provider:      req.Provider,
		BaseURL:       req.BaseURL,
		ModelID:       req.ModelID,
		CustomHeaders: req.CustomHeaders,
		Secret:        []byte(req.Secret),
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionTestResult{Report: res.Report, Record: res.Record})
}

// handleConnectionRotate exposes the credential-rotation command (M-26):
// the replacement key is a NEW write-only value stored under the SAME
// handle atomically (the D-37 same-handle swap); the response data carries
// the affected-models/routing + interruption statement (the P3.2 §3.3 [P]
// row as the working candidate).
func (s *Server) handleConnectionRotate(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	s.handleConnectionCredentialReplace(w, r, token, connectionID, s.deps.Slice.RotateConnection)
}

// handleConnectionReauthenticate exposes the credential re-entry command
// (M-27): a fresh write-only key under the same record — the rotate path's
// cousin, its own audited action.
func (s *Server) handleConnectionReauthenticate(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	s.handleConnectionCredentialReplace(w, r, token, connectionID, s.deps.Slice.ReauthenticateConnection)
}

func (s *Server) handleConnectionCredentialReplace(w http.ResponseWriter, r *http.Request, token, connectionID string, op func(context.Context, app.RotateConnectionRequest) (app.RotateConnectionResult, error)) {
	var req connectionCredentialRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := op(s.callCtx(r, token), app.RotateConnectionRequest{ConnectionID: connectionID, Secret: []byte(req.Secret)})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleConnectionDelete exposes the delete/revoke command: the Core
// deletes the record AND revokes the keyring entry (the D-37 fail-closed
// order — a typed revoke failure refuses the delete). The high-risk
// confirmation is the UI's.
func (s *Server) handleConnectionDelete(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	if err := s.deps.Slice.DeleteConnection(s.callCtx(r, token), connectionID); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{})
}

// handleConnectionSwitch exposes the switch command (M-34's minimal
// working candidate): the durable active-connection pointer, idempotent on
// the already-active connection (replayed=true). No Job migrates and no
// cache mixes.
func (s *Server) handleConnectionSwitch(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	res, err := s.deps.Slice.SwitchConnection(s.callCtx(r, token), connectionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// The conversation surface handlers (the adopted S5 mapping, P-S5-01…05).
// Every handler rides the existing boundary middleware (the Host allowlist,
// the credential-surface rule, CSRF for mutations, the loopback-only bind);
// the slice's decision port is the authoritative check inside. The paths
// are this adapter's own — no published contract carries them, and no
// streaming, stop, or steering endpoint exists anywhere (P-S5-03's
// transport mechanism is the named owner gap).

// handleConversationCreate exposes the aggregate-create operation: the Core
// mints the stable conversation identity; the body carries the optional
// bounded title only.
func (s *Server) handleConversationCreate(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req conversationCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	conv, err := s.deps.Slice.CreateConversation(s.callCtx(r, token), app.ConversationRecordRequest{Title: req.Title})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationResult{Conversation: conv})
}

// handleConversationList exposes the server-side list/search query (the
// card-9 working candidate): server-side cursor pagination, the
// Core-defined updated-at descending order, the optional `q` search term.
// The limit is optional (the slice's per-surface default applies); the
// cursor is opaque and its absence on a response marks the final page. An
// empty page IS the honest no-match answer for a search — never an error
// shape that invents one.
func (s *Server) handleConversationList(w http.ResponseWriter, r *http.Request, token, _ string) {
	q := r.URL.Query()
	limit, lerr := parseListLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	cursor := q.Get("cursor")
	if len(cursor) > maxListCursorLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "cursor"}))
		return
	}
	query := q.Get("q")
	if len(query) > maxSearchQueryLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "q"}))
		return
	}
	res, err := s.deps.Slice.ListConversations(s.callCtx(r, token), app.ListConversationsRequest{
		Limit:  limit,
		Query:  query,
		Cursor: cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationListResult{Conversations: res.Conversations, Cursor: res.NextCursor})
}

// handleConversationGet exposes one conversation aggregate's read (safe
// metadata only); an unknown id is the typed NOT_FOUND.
func (s *Server) handleConversationGet(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	conv, err := s.deps.Slice.Conversation(s.callCtx(r, token), conversationID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationResult{Conversation: conv})
}

// handleConversationMessages exposes one conversation's authoritative
// messages read (P-S5-02) in timeline order, one bounded page at a time.
func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	q := r.URL.Query()
	limit, lerr := parseListLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	cursor := q.Get("cursor")
	if len(cursor) > maxListCursorLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "cursor"}))
		return
	}
	res, err := s.deps.Slice.ConversationMessages(s.callCtx(r, token), conversationID, app.ConversationMessagePageRequest{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationMessagesResult{Messages: res.Messages, Cursor: res.NextCursor})
}

// handleConversationSend exposes the Ask send (P-S5-05): the body carries
// the message content only — no idempotency key exists (the class stays
// Open). The response is the recorded exchange with its Core-issued
// correlation ID (the identity the S4 query resolves through) plus the
// persisted user message. The send NEVER creates a Job, and the response
// carries NO answer — no answering production path exists (the named owner
// gap).
func (s *Server) handleConversationSend(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	var req conversationSendRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.Send(s.callCtx(r, token), app.ConversationSendRequest{
		ConversationID: conversationID,
		Content:        req.Content,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationExchangeResult{Exchange: res.Exchange, Message: res.Message})
}

// handleConversationExchange exposes the S4 query-by-correlation-ID read
// (P-S5-05): the authoritative exchange state for one send, resolved by the
// correlation ID. A pure query — it never re-sends, never mutates. A
// correlation ID bound to a different conversation is the SAME typed
// NOT_FOUND as an unknown id (the no-leak rule).
func (s *Server) handleConversationExchange(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	corr := r.URL.Query().Get("correlation_id")
	if corr == "" || len(corr) > maxPathID {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "correlation_id"}))
		return
	}
	exchange, msg, err := s.deps.Slice.ConversationExchangeStatus(s.callCtx(r, token), conversationID, corr)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationExchangeResult{Exchange: exchange, Message: msg})
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request, token, _ string) {
	q := r.URL.Query()
	stream := q.Get("stream")
	cursor := q.Get("cursor")
	limit, lerr := parseLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	if stream == "" || len(stream) > maxPathID {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "stream"}))
		return
	}
	corr := errs.NewCorrelationID()
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: callerID(token)}
	pctx := security.PolicyContext{CorrelationID: corr, Credential: token}
	decision, auditErr := s.deps.Guard.Authorize(p, security.Resource{Type: security.ResourceStorage}, security.ActionDataRead, pctx)
	if auditErr != nil || !decision.Allow {
		s.writeJSON(w, http.StatusOK, replayResult{Outcome: events.ReplayForbidden})
		return
	}
	recs, outcome, err := s.deps.Replay.Replay(r.Context(), stream, cursor, limit)
	if err != nil {
		s.writeError(w, err)
		return
	}
	res := replayResult{Outcome: outcome, Events: recs}
	if outcome == events.ReplayOK && len(recs) > 0 {
		if next, cerr := events.EncodeCursor(stream, recs[len(recs)-1].Sequence, time.Now()); cerr == nil {
			res.Cursor = next
		}
	}
	s.writeJSON(w, http.StatusOK, res)
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 0, errors.New("httplocal: limit is required")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxReplayLimit {
		return 0, errors.New("httplocal: limit out of the transport bound")
	}
	return n, nil
}

// callCtx stamps the caller identity and the presented session credential
// for the Guard (the token-hash prefix is the safe identifier, never the
// credential value itself). An unauthenticated request still reaches the
// decision port, which denies it with the stable reason code.
func (s *Server) callCtx(r *http.Request, token string) context.Context {
	return app.WithCaller(r.Context(), callerID(token), token)
}

// callerID derives the bounded log/audit identifier from the presented
// token: the SHA-256 hex prefix, the same derivation the session service
// uses for its own token IDs. An empty token keeps the bounded local
// default so the decision port still sees a well-formed principal.
func callerID(token string) string {
	if token == "" {
		return "local"
	}
	return hashCredential(token)[:16]
}

func hashCredential(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	scheme, rest, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(rest)
}

// refuse writes the typed boundary refusal: the stable reason code rides
// the D-35 envelope's bounded data (the register's AUTHZ_DENIED code; the
// eight boundary reasons are the only refusal vocabulary). Safe metadata
// goes to the optional log — never credential material (S-1).
func (s *Server) refuse(w http.ResponseWriter, reason string) {
	if s.deps.Log != nil {
		s.deps.Log(security.Event{
			Level:      security.LevelWarn,
			Category:   "security.boundary",
			MessageKey: "LOCAL_HTTP_BOUNDARY_REFUSAL",
			Fields:     map[string]string{"reason": reason},
		})
	}
	s.writeEnvelope(w, http.StatusForbidden, envelope(errs.CodeAuthzDenied, errs.NewCorrelationID(),
		map[string]any{"reason": reason}))
}

func boundaryReason(err error) string {
	var b *security.ErrBoundary
	if errors.As(err, &b) {
		return b.Reason
	}
	return ""
}

// envelopeCarrier is any adapter error that converts itself into the wire
// envelope with its stable coded register semantics (the storage boundary
// error does; the use-case EnvelopeError below is the app-layer one).
type envelopeCarrier interface {
	Envelope(correlationID string) (errs.Envelope, error)
}

// writeError maps a use-case failure onto the D-35 envelope: typed
// envelope errors pass through with their coded semantics; anything else
// is the bounded INTERNAL envelope (never raw internals).
func (s *Server) writeError(w http.ResponseWriter, err error) {
	var ee *app.EnvelopeError
	if errors.As(err, &ee) {
		s.writeEnvelope(w, statusFor(ee.Envelope.Code), ee.Envelope)
		return
	}
	var ec envelopeCarrier
	if errors.As(err, &ec) {
		env, cerr := ec.Envelope(errs.NewCorrelationID())
		if cerr == nil {
			s.writeEnvelope(w, statusFor(env.Code), env)
			return
		}
	}
	env, cerr := errs.New(errs.CodeInternal, errs.NewCorrelationID(), nil)
	if cerr != nil {
		return
	}
	s.writeEnvelope(w, http.StatusInternalServerError, env)
}

// statusFor maps the coded register onto transport statuses. It is this
// adapter's own mapping: the RFC 9457 problem-details mapping stays its
// recorded future act; the body is the four-field envelope exactly as
// coded.
func statusFor(code errs.Code) int {
	switch code {
	case errs.CodeValidationFailed:
		return http.StatusBadRequest
	case errs.CodeAuthzDenied:
		return http.StatusForbidden
	case errs.CodeNotFound:
		return http.StatusNotFound
	case errs.CodeConflictState, errs.CodeConfigConflict, errs.CodeSingleInstanceConflict,
		errs.CodeConflictIdempotencyKey:
		return http.StatusConflict
	case errs.CodeRateLimited:
		return http.StatusTooManyRequests
	case errs.CodeStorageUnavailable, errs.CodeRecoveryRequired, errs.CodeStateCorrupt, errs.CodeMigrationFailed:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func envelope(code errs.Code, corr string, data map[string]any) errs.Envelope {
	env, err := errs.New(code, corr, data)
	if err != nil {
		// The fixed-shape data maps used here cannot fail construction; a
		// failure degrades to a bounded generic envelope, never to silence.
		env, _ = errs.New(errs.CodeInternal, corr, nil)
		return env
	}
	return env
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.writeEnvelope(w, http.StatusInternalServerError, envelope(errs.CodeInternal, errs.NewCorrelationID(), nil))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func (s *Server) writeEnvelope(w http.ResponseWriter, status int, env errs.Envelope) {
	b, err := json.Marshal(env)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	if r.Body == nil {
		return errors.New("httplocal: empty body")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// The wire shapes. Every key is a coded name: the JSON tags mirror the
// coded tags of the shapes the adopted extraction records (JobInput, Job,
// Attempt, the event Record, the D-35 envelope, the read-only probe's
// job/events keys); where a coded struct carries no JSON tag, the coded Go
// field name lowercased is used (replayed, token). No key invents a name
// outside the codebase's own vocabulary.

type submitRequest struct {
	// task/system/schema/pack_ref/provider/model/max_output_tokens/
	// classification/base_url are the coded JobInput tags for the same
	// fields; idempotency_key and deadline_secs are the Job record's coded
	// tags for the same fields.
	IdempotencyKey  string          `json:"idempotency_key"`
	Task            string          `json:"task"`
	System          string          `json:"system,omitempty"`
	Schema          json.RawMessage `json:"schema,omitempty"`
	PackRef         string          `json:"pack_ref,omitempty"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	DeadlineSecs    int64           `json:"deadline_secs"`
	Classification  string          `json:"classification,omitempty"`
	BaseURL         string          `json:"base_url,omitempty"`
}

type submitResult struct {
	// job is the read-only probe's coded key for the Job record; replayed
	// is the SubmitResult field's coded name lowercased.
	Job      aijobrelay.Job `json:"job"`
	Replayed bool           `json:"replayed"`
}

type statusResult struct {
	Job     aijobrelay.Job     `json:"job"`
	Attempt aijobrelay.Attempt `json:"attempt"`
}

// The context-snapshot read shape (the card-11 working candidate): the
// app-layer SnapshotView's coded JSON tags inside the snapshot wrapper
// (the coded-name rule: the coded Go name lowercased). The compiled
// content is absent from the shape by construction — the view carries the
// structure only.
type snapshotResult struct {
	Snapshot app.SnapshotView `json:"snapshot"`
}

type approvalResult struct {
	// approval carries the aijobrelay.Approval coded JSON tags (the card-14
	// working candidate's approval-request object schema: approval_id,
	// job_id, state, decision?, decided_by?, correlation_id?, created_at,
	// decided_at?).
	Approval aijobrelay.Approval `json:"approval"`
}

type approvalDecisionRequest struct {
	// approval_id is the Core-issued approval identity the decision command
	// carries (requirement (a) of the card-14 working candidate — the
	// decision's scope is exactly this presented identity, never broader).
	ApprovalID string `json:"approval_id"`
}

type approvalDecisionResult struct {
	// approval/job are the coded struct tags; replayed is the coded
	// ApprovalDecisionResult field lowercased (the coded-name rule).
	Approval aijobrelay.Approval `json:"approval"`
	Job      aijobrelay.Job      `json:"job"`
	Replayed bool                `json:"replayed"`
}

type reviewRequest struct {
	// task/system/schema/pack_ref/provider/model/max_output_tokens/
	// classification/base_url are the coded JobInput tags — exactly the
	// submit request's shape keys minus the idempotency key, which a
	// pre-submit review never carries (the key is minted at submit time).
	Task            string          `json:"task"`
	System          string          `json:"system,omitempty"`
	Schema          json.RawMessage `json:"schema,omitempty"`
	PackRef         string          `json:"pack_ref,omitempty"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Classification  string          `json:"classification,omitempty"`
	BaseURL         string          `json:"base_url,omitempty"`
}

type reviewResult struct {
	// review is the review operation's own name lowercased (the coded-name
	// rule: the coded Go name lowercased); the payload inside is the
	// app-layer ReviewPayload's coded JSON tags (the eight recorded review
	// fields).
	Review app.ReviewPayload `json:"review"`
}

type listResult struct {
	// jobs is the read-only probe's coded `job` key, pluralized for the
	// list surface; cursor is the coded replay cursor's key carrying the
	// opaque list cursor — omitted on the final page. No has_more/total
	// vocabulary is invented (the card-9 working candidate finalizes at the
	// freeze).
	Jobs   []aijobrelay.Job `json:"jobs"`
	Cursor string           `json:"cursor,omitempty"`
}

// The conversation wire shapes (the adopted S5 mapping, P-S5-01…05): the
// JSON tags are the coded tags of the domain conversation contract
// (internal/domain/conversation — the Core's own durable message shape,
// never the provider-layer request-plan `Message`); the wrapper keys are
// the coded struct names lowercased (the coded-name rule). No answer,
// stream, stop, or steering member exists anywhere in these shapes — the
// answering/stream production path is the named owner gap (P-S5-03).

type conversationCreateRequest struct {
	// title is the optional bounded display title (the domain's own shape
	// law bounds it; the Core mints the identity, the body never carries
	// one).
	Title string `json:"title"`
}

type conversationSendRequest struct {
	// content is the user's message content; correlation_id is the send's
	// request identity (the P1.6 §3.9 M-1 register row) — the identity the
	// S4 query resolves through. It is NOT an idempotency key (the class
	// stays Open): a duplicate identity on the same conversation refuses —
	// never a duplicate exchange, never a replay claim.
	Content       string `json:"content"`
	CorrelationID string `json:"correlation_id"`
}

type conversationResult struct {
	// conversation carries the conversation.Conversation coded tags
	// (conversation_id, title, created_at, updated_at).
	Conversation conversation.Conversation `json:"conversation"`
}

type conversationListResult struct {
	// conversations carries the coded tags; cursor is the coded replay
	// cursor's key carrying the opaque list cursor — omitted on the final
	// page (the cursorless page IS the end marker; no has_more/total
	// vocabulary exists). An empty page IS the honest no-match answer.
	Conversations []conversation.Conversation `json:"conversations"`
	Cursor        string                      `json:"cursor,omitempty"`
}

type conversationMessagesResult struct {
	// messages carries the conversation.Message coded tags (message_id,
	// conversation_id, seq, role, content, correlation_id?, created_at) —
	// the authoritative Core state inside the aggregate (P-S5-02), in
	// timeline order (Seq ascending).
	Messages []conversation.Message `json:"messages"`
	Cursor   string                 `json:"cursor,omitempty"`
}

type conversationExchangeResult struct {
	// exchange carries the conversation.Exchange coded tags
	// (correlation_id, conversation_id, user_message_id, state, created_at)
	// — the S4 query's authoritative exchange state (P-S5-05); message is
	// the send's persisted user message.
	Exchange conversation.Exchange `json:"exchange"`
	Message  conversation.Message  `json:"message"`
}

// The connection wire shapes. Every key is a coded name: the record/view
// JSON tags are the coded tags of the card-10 working candidate's object
// (internal/domain/provider connections.go) and the app-layer view; the
// wrapper keys are the coded struct names lowercased (the coded-name
// rule). The credential VALUE appears in a REQUEST body only — no
// response, log, error, or test ever carries one.

type connectionSaveRequest struct {
	// idempotency_key/provider/model_id are the record's coded tags;
	// base_url/custom_headers are the coded Custom API field set (UI-12);
	// secret is the write-only credential (transient entry).
	IdempotencyKey string                       `json:"idempotency_key"`
	Provider       string                       `json:"provider"`
	BaseURL        string                       `json:"base_url,omitempty"`
	ModelID        string                       `json:"model_id"`
	CustomHeaders  []dprovider.ConnectionHeader `json:"custom_headers,omitempty"`
	Secret         string                       `json:"secret,omitempty"`
}

type connectionSaveResult struct {
	// connection is the app view's coded shape (the record + the masked
	// secret status + the tested-state truth + the active flag); replayed
	// is the honest replay flag (the duplicate save returned the existing
	// reference).
	Connection app.ConnectionView `json:"connection"`
	Replayed   bool               `json:"replayed"`
}

type connectionListResult struct {
	// connections is the coded ConnectionView list; cursor is the opaque
	// list cursor — omitted on the final page (the card-9 discipline).
	Connections []app.ConnectionView `json:"connections"`
	Cursor      string               `json:"cursor,omitempty"`
}

type connectionGetResult struct {
	Connection app.ConnectionView `json:"connection"`
}

type connectionTestRequest struct {
	// connection_id selects a SAVED connection (its stored reference
	// resolves); its absence makes the request an unsaved safe draft whose
	// typed secret rides transiently (provider/base_url/model_id/
	// custom_headers are the draft's fields — the coded record tags).
	ConnectionID  string                       `json:"connection_id,omitempty"`
	Provider      string                       `json:"provider,omitempty"`
	BaseURL       string                       `json:"base_url,omitempty"`
	ModelID       string                       `json:"model_id,omitempty"`
	CustomHeaders []dprovider.ConnectionHeader `json:"custom_headers,omitempty"`
	Secret        string                       `json:"secret,omitempty"`
}

type connectionTestResult struct {
	// report is the sanitized M-25 report (the coded ConnectionTestReport
	// tags); record is set only when the test rode a saved connection (the
	// TC-1 tested-state stamp).
	Report dprovider.ConnectionTestReport `json:"report"`
	Record *dprovider.ConnectionRecord    `json:"record,omitempty"`
}

type connectionCredentialRequest struct {
	// secret is the NEW write-only replacement key (M-26/M-27): never the
	// decrypted old key, never echoed.
	Secret string `json:"secret"`
}

type eventsResult struct {
	Events []events.Record `json:"events"`
}

type replayResult struct {
	// outcome carries the coded ReplayOutcome value set; cursor is the
	// opaque encoded cursor (never structured data on the wire, C2-13 §5).
	Outcome events.ReplayOutcome `json:"outcome"`
	Events  []events.Record      `json:"events"`
	Cursor  string               `json:"cursor,omitempty"`
}

type exchangeRequest struct {
	Credential string   `json:"credential"`
	Scopes     []string `json:"scopes,omitempty"`
}

type sessionResult struct {
	// token/id/scopes are the SessionToken field names lowercased. The
	// token value is the bearer secret the holder persists client-side
	// (the client-persistence rule stays its recorded open item); it never
	// travels in a URL, log, or Referer (the credential-surface rule
	// enforces the server side; R-11).
	Token  string   `json:"token"`
	ID     string   `json:"id"`
	Scopes []string `json:"scopes"`
	// Expires carries the absolute TTL as coded (the RFC 3339 encoding
	// time.Time already produces); the value is a Proposed constant the
	// owner has not finalized.
	Expires time.Time `json:"expires"`
}

// compile-time interface check.
var _ http.Handler = (*Server)(nil)
