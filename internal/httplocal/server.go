package httplocal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/app"
	appcontext "github.com/qira-dev/karyora/internal/domain/context"
	"github.com/qira-dev/karyora/internal/domain/conversation"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/knowledge"
	"github.com/qira-dev/karyora/internal/domain/memory"
	"github.com/qira-dev/karyora/internal/domain/notification"
	plugindomain "github.com/qira-dev/karyora/internal/domain/plugin"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	workflowdomain "github.com/qira-dev/karyora/internal/domain/workflow"
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
	// StaticFS is the optional filesystem serving the Web UI SPA static files.
	// When non-nil, non-API GET/HEAD requests serve static assets or index.html.
	StaticFS fs.FS
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
// session token. midPath extends the registered shapes with the two-ID form
// (the B1 immutable-version read: prefix + id + midPath + secondId); every
// pre-existing route leaves it empty and the single-ID shape is unchanged.
type route struct {
	method    string
	prefix    string // literal path prefix; the remainder is the job id where present
	suffix    string // optional literal suffix after the id
	hasID     bool
	mutates   bool
	noSession bool
	handler   func(s *Server, w http.ResponseWriter, r *http.Request, token, jobID string)
	// midPath + handler2 register the two-ID path shape; handler2 receives
	// both resolved ids.
	midPath  string
	handler2 func(s *Server, w http.ResponseWriter, r *http.Request, token, id1, id2 string)
}

// routes is the fixed endpoint table. The paths are this adapter's own: no
// published contract carries them (the canonical-transport question stays
// its recorded open item).
func routes() []route {
	return []route{
		{method: http.MethodPost, prefix: "/v1/session/exchange", mutates: true, noSession: true, handler: (*Server).handleExchange},
		// The C-4 logout/revocation wire commands (IMPL-23, the S9-adopted
		// P-S9-02 class as implemented; the S4 §19 F11 freeze act's frozen
		// content): the per-token logout (the coded Revoke class) and the
		// credential-change revocation (the coded RevokeAll class), each a
		// WRITE through the one decision port inside the slice. The paths
		// are this adapter's own — non-canonical (the IMPL-4/6/7/8
		// convention); the canonical names join the published vocabulary at
		// the publication act per card 1's additive discipline — NO
		// contracts/ edit. Both carry the mutating-route CSRF/Origin
		// posture and require the presented session (the boundary's
		// mutates gate applies, per the adopted class).
		{method: http.MethodPost, prefix: "/v1/session/logout", mutates: true, handler: (*Server).handleSessionLogout},
		{method: http.MethodPost, prefix: "/v1/session/revoke-all", mutates: true, handler: (*Server).handleSessionRevokeAll},
		// The F12 audit/activity read surface (IMPL-24, the adopted S11
		// classes as implemented; the S4 §20 freeze act's frozen content):
		// the permission-filtered, server-side, paginated audit list (the
		// card-9 working candidate's class) and the M-32-class
		// read-with-export operation. Both are READS — non-mutating GETs
		// (the state-changing-GET prohibition holds: neither mutates
		// anything); the read-only law binds — no dismiss/delete/correction
		// of source records exists anywhere on the surface, and the export
		// adds no tamper-resistance. The paths are this adapter's own —
		// non-canonical (the IMPL-4/6/7/8 convention); the canonical names
		// join the published vocabulary at the publication act per card 1's
		// additive discipline — NO contracts/ edit.
		{method: http.MethodGet, prefix: "/v1/audit/export", handler: (*Server).handleAuditExport},
		{method: http.MethodGet, prefix: "/v1/audit", handler: (*Server).handleAuditList},
		{method: http.MethodPost, prefix: "/v1/jobs/review", mutates: true, handler: (*Server).handleReview},
		{method: http.MethodPost, prefix: "/v1/jobs", mutates: true, handler: (*Server).handleSubmit},
		{method: http.MethodGet, prefix: "/v1/jobs", handler: (*Server).handleList},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/events", handler: (*Server).handleEvents},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/result", handler: (*Server).handleResult},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/snapshot", handler: (*Server).handleJobSnapshot},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/retry-eligibility", handler: (*Server).handleRetryEligibility},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, suffix: "/approval", handler: (*Server).handleJobApproval},
		{method: http.MethodGet, prefix: "/v1/jobs/", hasID: true, handler: (*Server).handleStatus},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/cancel", mutates: true, handler: (*Server).handleCancel},
		// The M-7 retry command (IMPL-21, the F9 family's frozen semantics):
		// a NEW Attempt of the SAME Job — never a new Job — applied only
		// where the Core reports eligibility, carrying its Supplied-keyed
		// idempotency reference. A WRITE through the one decision port; the
		// mutating route carries the same CSRF/Origin posture every
		// mutating route carries. The path is this adapter's own (the
		// canonical name joins the published vocabulary at the publication
		// act per card 1's additive discipline — NO contracts/ edit).
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/retry", mutates: true, handler: (*Server).handleRetryCommand},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/approvals/accept", mutates: true, handler: (*Server).handleApprovalAccept},
		{method: http.MethodPost, prefix: "/v1/jobs/", hasID: true, suffix: "/approvals/reject", mutates: true, handler: (*Server).handleApprovalReject},
		{method: http.MethodPost, prefix: "/v1/connections/test", mutates: true, handler: (*Server).handleConnectionTest},
		{method: http.MethodPost, prefix: "/v1/connections", mutates: true, handler: (*Server).handleConnectionSave},
		{method: http.MethodGet, prefix: "/v1/connections", handler: (*Server).handleConnectionList},
		{method: http.MethodGet, prefix: "/v1/connections/", hasID: true, handler: (*Server).handleConnectionGet},
		// The F2 capability discovery/status read (IMPL-22, the adopted
		// S10 supply's P-S10-02 class as implemented): the standing
		// per-provider capability descriptor — a non-mutating GET that
		// runs no network probe, ADDITIVE to the implemented M-25 test
		// (the probe-bearing discovery surface stays exactly as coded).
		// The path is this adapter's own (the canonical name joins the
		// published vocabulary at the publication act per card 1's
		// additive discipline — NO contracts/ edit).
		{method: http.MethodGet, prefix: "/v1/providers/", hasID: true, suffix: "/capabilities", handler: (*Server).handleProviderCapabilities},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/rotate", mutates: true, handler: (*Server).handleConnectionRotate},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/reauthenticate", mutates: true, handler: (*Server).handleConnectionReauthenticate},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/delete", mutates: true, handler: (*Server).handleConnectionDelete},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/switch", mutates: true, handler: (*Server).handleConnectionSwitch},
		// The connection-record rename and the non-secret field update (the
		// adopted S8 supply's P-S8-01/P-S8-02 working candidates, the M-27
		// register row's rename portion + the coded create shape's
		// non-secret field set). The concurrent-edit guard rides both: the
		// request carries the updated_at marker the caller read, and a
		// stale marker is the typed CONFLICT_STATE 409 with
		// data.currentState. NO secret field exists on either request —
		// the secret path stays the coded rotate/re-authenticate routes
		// above, byte-identical. The paths are this adapter's own (the
		// IMPL-4/6/7/8 convention); no published contract carries them.
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/rename", mutates: true, handler: (*Server).handleConnectionRename},
		{method: http.MethodPost, prefix: "/v1/connections/", hasID: true, suffix: "/update", mutates: true, handler: (*Server).handleConnectionUpdate},
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
		// The conversation attachment surface (IMPL-25; the ADOPTED S12
		// supply, P-S12-01…06, as implemented): the registry read (the
		// Core's own supported types/sizes as queryable contract data —
		// P-S12-01), the identity-bearing upload (P-S12-02's duplicate-free
		// replay ruling inside the store; P-S12-04's typed rejections), the
		// S4-for-uploads status query (P-S12-02's query-before-retry law),
		// the conversation-scoped list, and the draft-stage delete. The
		// upload carries base64 content over the JSON envelope transport
		// (the implementation's typing — no multipart parser exists here
		// and none is introduced). NO message-binding, NO snapshot
		// participation, and NO stream/stop/steering endpoint exists
		// anywhere (P-S12-06's policy is not exercised; the answering path
		// is the named owner gap). Every operation rides the one decision
		// port (ResourceConversation + the data actions) and NEVER creates
		// a Job (X-1). The paths are this adapter's own (the IMPL-4..11
		// convention); the canonical names join the published vocabulary
		// at the publication act per card 1's additive discipline — NO
		// contracts/ edit.
		{method: http.MethodGet, prefix: "/v1/conversations/attachments/registry", handler: (*Server).handleConversationAttachmentRegistry},
		{method: http.MethodPost, prefix: "/v1/conversations/", hasID: true, suffix: "/attachments", mutates: true, handler: (*Server).handleConversationAttachmentUpload},
		{method: http.MethodGet, prefix: "/v1/conversations/", hasID: true, suffix: "/attachments/status", handler: (*Server).handleConversationAttachmentStatus},
		{method: http.MethodGet, prefix: "/v1/conversations/", hasID: true, suffix: "/attachments", handler: (*Server).handleConversationAttachmentList},
		{method: http.MethodPost, prefix: "/v1/conversations/", hasID: true, suffix: "/attachments/delete", mutates: true, handler: (*Server).handleConversationAttachmentDelete},
		// The notification surface (the adopted S6 supply, P-S6-01/02): the
		// grouped list read, the Core-supplied unread counts, and the O-6
		// mutation set's typed outcomes (mark read, mark unread, clear —
		// "filter" is the list read's query parameters). The paths are this
		// adapter's own (the IMPL-4/6/7/8/9/10 convention). NO OS-delivery,
		// OS-permission, quiet-hours, or mark-all-read endpoint exists
		// anywhere (decision 6: in-app ON + OS OFF by default; the UI-34
		// FINAL freeze stays its own Checkpoint-C act; the P-S6-02 named
		// set is exactly mark read/unread/filter/clear) — the route
		// register is pinned so none can silently appear.
		{method: http.MethodGet, prefix: "/v1/notifications/counts", handler: (*Server).handleNotificationCounts},
		{method: http.MethodGet, prefix: "/v1/notifications", handler: (*Server).handleNotificationList},
		{method: http.MethodPost, prefix: "/v1/notifications/read", mutates: true, handler: (*Server).handleNotificationMarkRead},
		{method: http.MethodPost, prefix: "/v1/notifications/unread", mutates: true, handler: (*Server).handleNotificationMarkUnread},
		{method: http.MethodPost, prefix: "/v1/notifications/clear", mutates: true, handler: (*Server).handleNotificationClear},
		// The B1 KnowledgePack family (DE-0 queue row 1): the published
		// /knowledgepacks route family as this adapter serves it (the /v1
		// adapter-prefix convention; the canonical members live verbatim in
		// contracts/openapi.yaml — never restated). Every operation rides
		// the slice's one decision port (the ResourceKnowledge register
		// extension; Local mode stays role-free); the submit routes are
		// 202-class (accepted for processing — the authoritative
		// operation-state read confirms every outcome); the state endpoint
		// is the family's authoritative source of truth; cancellation is
		// the Core-confirmed operation only (the pre-submit queue removal
		// is a local act with NO Core meaning). Literal routes register
		// before the generic {id} reads so match() resolves them first.
		{method: http.MethodGet, prefix: "/v1/knowledgepacks/import/registry", handler: (*Server).handleKnowledgeImportRegistry},
		{method: http.MethodPost, prefix: "/v1/knowledgepacks/import", mutates: true, handler: (*Server).handleKnowledgeImportSubmit},
		{method: http.MethodPost, prefix: "/v1/knowledgepacks/export", mutates: true, handler: (*Server).handleKnowledgeExportSubmit},
		{method: http.MethodPost, prefix: "/v1/knowledgepacks/operations/", hasID: true, suffix: "/cancel", mutates: true, handler: (*Server).handleKnowledgeOperationCancel},
		{method: http.MethodGet, prefix: "/v1/knowledgepacks/operations/", hasID: true, handler: (*Server).handleKnowledgeOperationStatus},
		// The immutable-version read (the two-ID shape): prefix + pack id +
		// midPath + version id — the canonical deep link of the family.
		{method: http.MethodGet, prefix: "/v1/knowledgepacks/", hasID: true, midPath: "/versions/", handler2: (*Server).handleKnowledgeVersionGet},
		{method: http.MethodGet, prefix: "/v1/knowledgepacks/", hasID: true, suffix: "/versions", handler: (*Server).handleKnowledgeVersionList},
		{method: http.MethodGet, prefix: "/v1/knowledgepacks", handler: (*Server).handleKnowledgePackList},
		{method: http.MethodGet, prefix: "/v1/knowledgepacks/", hasID: true, handler: (*Server).handleKnowledgePackGet},
		// The B2 Memory family (DE-0 queue row 3): the published /memories
		// route family as this adapter serves it (the /v1 adapter-prefix
		// convention; the canonical members live verbatim in
		// contracts/openapi.yaml — never restated). Every operation rides
		// the slice's one decision port (the ResourceMemory register
		// extension; Local mode stays role-free); the mutation answers are
		// 200-class Core confirmations (the F11 law: only a confirmed 200
		// carries the outcome truth); the state endpoint is the family's
		// authoritative source of truth; the list/inspect reads are the
		// M-1 correlation class. THE SECRETS LAW (BINDING): a write
		// attempting secret storage is refused (MEMORY_SECRETS_REJECTED);
		// the law extends to exports, logs, URLs, and error messages.
		// Literal routes register before the generic {id} reads so match()
		// resolves them first (GET /v1/memories/export resolves the generic
		// item read — an item named "export"; the same typed 404).
		{method: http.MethodPost, prefix: "/v1/memories/export", mutates: true, handler: (*Server).handleMemoryExport},
		{method: http.MethodGet, prefix: "/v1/memories/operations/", hasID: true, handler: (*Server).handleMemoryOperationStatus},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/delete", mutates: true, handler: (*Server).handleMemoryDelete},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/forget", mutates: true, handler: (*Server).handleMemoryForget},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/approve", mutates: true, handler: (*Server).handleMemoryApprove},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/reject", mutates: true, handler: (*Server).handleMemoryReject},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/pin", mutates: true, handler: (*Server).handleMemoryPin},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, suffix: "/inclusion", mutates: true, handler: (*Server).handleMemoryInclusion},
		{method: http.MethodGet, prefix: "/v1/memories", handler: (*Server).handleMemoryList},
		{method: http.MethodPost, prefix: "/v1/memories", mutates: true, handler: (*Server).handleMemoryAdd},
		{method: http.MethodGet, prefix: "/v1/memories/", hasID: true, handler: (*Server).handleMemoryItemGet},
		{method: http.MethodPost, prefix: "/v1/memories/", hasID: true, mutates: true, handler: (*Server).handleMemoryEdit},
		// The B3 ContextSnapshot family (DE-0 queue row 5): the published
		// /context route family as this adapter serves it (the /v1
		// adapter-prefix convention; the canonical members live verbatim in
		// contracts/openapi.yaml — never restated). Every operation rides
		// the slice's one decision port (the ResourceContext register
		// extension; Local mode stays role-free). THE CONDITIONAL LAW
		// (carried exactly): the save-snapshot command is CONDITIONAL on
		// durable snapshot storage existing and the compare on stable Core
		// comparison — both conditions are UNMET (the owner's PROPOSED
		// decisions, unresolved), so both routes serve their typed
		// unmet-condition refusals and NEITHER is ever a fake success;
		// the M-18 inclusion decision is UNCONDITIONAL and implemented
		// fully; the state endpoint is the family's authoritative source
		// of truth (a pure query — never a retry). The compare route is
		// the M-20 READ-ONLY class (it mutates nothing — no operation row,
		// no diff — so it rides no CSRF mutation gate; the slice's
		// authorization still answers every call); the inclusion route is
		// a real mutation (the decision rows) and carries the CSRF gate.
		{method: http.MethodPost, prefix: "/v1/context/snapshots/compare", handler: (*Server).handleContextSnapshotCompare},
		{method: http.MethodPost, prefix: "/v1/context/snapshots", mutates: true, handler: (*Server).handleContextSnapshotSave},
		{method: http.MethodPost, prefix: "/v1/context/inclusion", mutates: true, handler: (*Server).handleContextInclusion},
		{method: http.MethodGet, prefix: "/v1/context/operations/", hasID: true, handler: (*Server).handleContextOperationStatus},
		// The B4 Workflow authoring family (DE-0 queue row 6): the published
		// /workflows route family as this adapter serves it (the /v1
		// adapter-prefix convention; the canonical members live verbatim in
		// contracts/openapi.yaml — never restated). Every operation rides
		// the slice's one decision port (the ResourceWorkflow register
		// extension; Local mode stays role-free). THE D-40 GATE (carried
		// exactly): THE RUN-EXECUTION OPERATION DOES NOT EXIST — no Run
		// submission, no Run start/resume/cancel, no Run schema, no Run/step
		// lifecycle event, no second run journal; the run-history route is
		// the LINKAGE read only. THE IF-MATCH CLAUSE: prose-only — no
		// precondition member exists anywhere; the interim conflict rule
		// governs. The create and save routes are real mutations (the CSRF
		// gate); the validate route is the read-like class (it mutates no
		// stored revision and never silently saves — no mutation gate); the
		// ai-proposals route is the draft-local generation class (nothing
		// stored — no mutation gate). Literal routes register before the
		// generic {definition_id} reads so match() resolves them first.
		{method: http.MethodGet, prefix: "/v1/workflows", handler: (*Server).handleWorkflowList},
		{method: http.MethodPost, prefix: "/v1/workflows", mutates: true, handler: (*Server).handleWorkflowCreate},
		{method: http.MethodPost, prefix: "/v1/workflows/validate", handler: (*Server).handleWorkflowValidate},
		{method: http.MethodPost, prefix: "/v1/workflows/ai-proposals", handler: (*Server).handleWorkflowAIDiffPropose},
		{method: http.MethodGet, prefix: "/v1/workflows/", hasID: true, suffix: "/state", handler: (*Server).handleWorkflowDefinitionState},
		{method: http.MethodGet, prefix: "/v1/workflows/", hasID: true, suffix: "/run-history", handler: (*Server).handleWorkflowRunHistory},
		{method: http.MethodPost, prefix: "/v1/workflows/", hasID: true, suffix: "/revisions", mutates: true, handler: (*Server).handleWorkflowRevisionSave},
		{method: http.MethodGet, prefix: "/v1/workflows/", hasID: true, handler: (*Server).handleWorkflowDefinitionGet},
		// The B5 Plugin lifecycle family (DE-0 queue row 8): the published
		// /plugins route family as this adapter serves it (the /v1
		// adapter-prefix convention; the canonical members live verbatim in
		// contracts/openapi.yaml — never restated). Every operation rides
		// the slice's one decision port (the ResourcePlugin register
		// extension; Local mode stays role-free). THE GATED EXECUTION ROWS
		// (carried exactly): THE INVOCATION/EXECUTION OPERATION DOES NOT
		// EXIST — no invocation route exists in this table, no
		// invocation/in-flight state schema exists, no invocation-status
		// read, no invocation events, no quota-value reporting, no
		// steering/stop controls; every plausible invocation path resolves
		// the router's NOT_FOUND (the register's stable vocabulary). THE
		// NEVER-GRANT LAW: no route on this family grants a capability —
		// the user grants/denies; the Core mediates and enforces. THE
		// DISTINCTNESS LAWS: the M-29 toggle is ONE member covering both
		// directions via the body's target state; the revoked states are
		// never exitable through it. THE REMOVE-VS-REVOKE DISTINCTNESS
		// (recorded at the 2026-11-09 resolution act, binding on this
		// family): removal and grant revocation are DISTINCT Core
		// semantics — removing a plugin does not revoke its grants;
		// revoking grants does not remove the artifact; each acts only on
		// its own named object. All five
		// mutation routes are real mutations (the CSRF gate); the list,
		// inspect, and state routes are pure reads. Literal suffix routes
		// register before the generic {plugin_id} read so match() resolves
		// them first.
		{method: http.MethodGet, prefix: "/v1/plugins", handler: (*Server).handlePluginList},
		{method: http.MethodPost, prefix: "/v1/plugins", mutates: true, handler: (*Server).handlePluginInstall},
		{method: http.MethodGet, prefix: "/v1/plugins/", hasID: true, suffix: "/state", handler: (*Server).handlePluginState},
		{method: http.MethodPost, prefix: "/v1/plugins/", hasID: true, suffix: "/enable", mutates: true, handler: (*Server).handlePluginEnable},
		{method: http.MethodPost, prefix: "/v1/plugins/", hasID: true, suffix: "/update", mutates: true, handler: (*Server).handlePluginUpdate},
		{method: http.MethodPost, prefix: "/v1/plugins/", hasID: true, suffix: "/remove", mutates: true, handler: (*Server).handlePluginRemove},
		{method: http.MethodPost, prefix: "/v1/plugins/", hasID: true, suffix: "/grants/revoke", mutates: true, handler: (*Server).handlePluginRevokeGrants},
		{method: http.MethodGet, prefix: "/v1/plugins/", hasID: true, handler: (*Server).handlePluginGet},
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

// match resolves the route for method+path and returns the resolved path
// ids (one for the single-ID shape; two for the two-ID shape; none for the
// literal routes). Method mismatches on known paths resolve to NOT_FOUND
// (the register's stable vocabulary; no method-not-allowed code exists).
func match(method, path string) (route, string, string, bool) {
	for _, rt := range routes() {
		if rt.method != method {
			continue
		}
		rest, ok := strings.CutPrefix(path, rt.prefix)
		if !ok {
			continue
		}
		if rt.midPath != "" {
			id1, id2, ok := cutIDPair(rest, rt.midPath)
			if !ok {
				continue
			}
			return rt, id1, id2, true
		}
		if rt.hasID {
			id, ok := cutID(rest, rt.suffix)
			if !ok {
				continue
			}
			return rt, id, "", true
		}
		if rt.suffix != "" || rest != "" {
			continue
		}
		return rt, "", "", true
	}
	return route{}, "", "", false
}

// cutIDPair resolves the two-ID shape: rest = id1 + mid + id2, both ids
// bounded exactly like the single-ID one.
func cutIDPair(rest, mid string) (string, string, bool) {
	i := strings.Index(rest, mid)
	if i <= 0 || i+len(mid) >= len(rest) {
		return "", "", false
	}
	id1, id2 := rest[:i], rest[i+len(mid):]
	if id1 == "" || id2 == "" || strings.Contains(id1, "/") || strings.Contains(id2, "/") ||
		len(id1) > maxPathID || len(id2) > maxPathID {
		return "", "", false
	}
	return id1, id2, true
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
	rt, id1, id2, ok := match(r.Method, r.URL.Path)
	if !ok {
		if s.deps.StaticFS != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.HasPrefix(r.URL.Path, "/v1/") {
			s.serveStatic(w, r)
			return
		}
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
	if rt.handler2 != nil {
		rt.handler2(s, w, r, token, id1, id2)
		return
	}
	rt.handler(s, w, r, token, id1)
}

// serveStatic serves embedded SPA static assets or falls back to index.html for client routing.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "." || cleanPath == "" {
		cleanPath = "index.html"
	}
	f, err := s.deps.StaticFS.Open(cleanPath)
	if err != nil {
		if !strings.HasPrefix(cleanPath, "assets/") {
			cleanPath = "index.html"
			f, err = s.deps.StaticFS.Open(cleanPath)
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		cleanPath = "index.html"
		if f2, err2 := s.deps.StaticFS.Open(cleanPath); err2 == nil {
			defer f2.Close()
			if stat2, err3 := f2.Stat(); err3 == nil && !stat2.IsDir() {
				http.ServeFileFS(w, r, s.deps.StaticFS, cleanPath)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, s.deps.StaticFS, cleanPath)
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

// The C-4 logout/revocation wire commands (IMPL-23, the S9-adopted
// P-S9-02 class as implemented): the per-token logout (the coded Revoke
// class) and the credential-change revocation (the coded RevokeAll
// class), each served through the slice — the one decision port + the
// fail-closed audit record run INSIDE the command, exactly like every
// command this adapter exposes. The request carries NO body: the
// presented session credential (already validated by the boundary's
// mutates gate) identifies the acted-on session half. The failure law
// binds the surfaces: a typed refusal is rendered verbatim on the D-35
// envelope and the client never assumes the session died because the
// command failed (P1.8 §6.2) — only a confirmed 200 carries the
// revocation truth. The paths are this adapter's own — non-canonical
// (the IMPL-4/6/7/8 convention); no published contract carries them.

// handleSessionLogout exposes the per-token logout command (the adopted
// P-S3-03 candidate point 1; R-13's dedicated flow's Core command — the
// UI surface is Settings → Security, never an ordinary preference
// control).
func (s *Server) handleSessionLogout(w http.ResponseWriter, r *http.Request, token, _ string) {
	res, err := s.deps.Slice.SessionLogout(s.callCtx(r, token))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleSessionRevokeAll exposes the credential-change revocation
// command (the adopted P-S3-03 candidate point 2; the coded C2-08
// lock-takeover + credential-change class): every live session of this
// instance is invalidated, including the presented one.
func (s *Server) handleSessionRevokeAll(w http.ResponseWriter, r *http.Request, token, _ string) {
	res, err := s.deps.Slice.SessionRevokeAll(s.callCtx(r, token))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
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

// handleRetryEligibility exposes the F9-frozen retry-eligibility read
// (the P3.2 §3.9 [P] typed-member set as frozen at the S4 §15 act — the
// adopted P-S7-01…06 supply): the Core's own per-Job verdict plus the
// three recorded typed members, authorized through the same decision
// port every read decides through. The read mutates nothing and is NOT
// the retry command — the M-7 command's identity stays its recorded Open
// item (the S7 §5 gap 2; O-1), so no eligibility response authorizes any
// execution. The path is this adapter's own (the canonical-transport
// question stays its recorded open item); the canonical name joins the
// published vocabulary at the publication act per card 1's additive
// discipline.
func (s *Server) handleRetryEligibility(w http.ResponseWriter, r *http.Request, token, jobID string) {
	res, err := s.deps.Slice.RetryEligibility(s.callCtx(r, token), jobID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, retryEligibilityResult{RetryEligibility: res})
}

// handleRetryCommand exposes the M-7 retry command (IMPL-21, the F9
// family's frozen semantics): a NEW Attempt of the SAME Job — never a new
// Job — applied only where the Core reports eligibility (the same verdict
// the eligibility read reports — one eligibility truth, never a fork), and
// carrying its Supplied-keyed idempotency reference: the same reference
// re-presented returns the original result (replayed, never regenerated);
// a contradicting re-issue is the typed CONFLICT_STATE. The response is
// the Core-confirmed post-command Job plus the honest replayed flag; the
// UI still renders only from the authoritative re-read (the 202-class
// law's consumption side). The eligibility read's GET surface executes
// nothing — execution exists only here, behind the command's own
// ActionDataWrite decision.
func (s *Server) handleRetryCommand(w http.ResponseWriter, r *http.Request, token, jobID string) {
	var req retryCommandRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.RetryJob(s.callCtx(r, token), jobID, req.IdempotencyReference)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, retryCommandResult{Job: res.Job, Replayed: res.Replayed})
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

// handleProviderCapabilities exposes the F2 capability discovery/status
// read (IMPL-22, the adopted S10 supply's P-S10-02 class as implemented):
// the standing per-provider capability descriptor — the Core's OWN coded
// capability state, permission-filtered through the same decision port
// every read decides through, WITHOUT running any network probe. The read
// is ADDITIVE to the implemented M-25 test (the probe-bearing discovery
// surface stays exactly as coded, never replaced); the descriptor carries
// FACTS, never promises (C3-29's frozen boundary), and nothing beyond
// what the Core's coded Capabilities() supplies (each matrix field's
// presence stays Unverified — the P3.2 §5.1 verdict). The optional model
// query value resolves the effective profile (the coded ProfileFor
// resolution — the review field 3's own pattern; none → the registered
// default profile). The path is this adapter's own; the canonical name
// joins the published vocabulary at the publication act per card 1.
func (s *Server) handleProviderCapabilities(w http.ResponseWriter, r *http.Request, token, providerID string) {
	res, err := s.deps.Slice.ProviderCapabilities(s.callCtx(r, token), providerID, r.URL.Query().Get("model"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, capabilityDescriptorResult{Descriptor: res})
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

// handleConnectionRename exposes the connection-record rename command (the
// M-27 register row's rename portion as implemented — the adopted S8
// supply's P-S8-01 working candidate): the display name ONLY, fenced on
// the updated_at marker the caller read (the concurrent-edit guard; a
// stale marker is the typed CONFLICT_STATE 409 with data.currentState).
// The request carries NO secret field — the secret path stays the coded
// rotate/re-authenticate routes.
func (s *Server) handleConnectionRename(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	var req connectionRenameRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.RenameConnection(s.callCtx(r, token), app.RenameConnectionRequest{
		ConnectionID:      connectionID,
		DisplayName:       req.DisplayName,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionRenameResult{Connection: res.Connection})
}

// handleConnectionUpdate exposes the non-secret field update command (the
// adopted S8 supply's P-S8-02 working candidate): EXACTLY the coded create
// shape's non-secret fields, fenced on the updated_at marker the caller
// read (the same concurrent-edit guard the rename rides). The request
// carries NO secret field — a credential change stays the coded
// rotate/re-authenticate routes, and no update path ever writes a secret.
func (s *Server) handleConnectionUpdate(w http.ResponseWriter, r *http.Request, token, connectionID string) {
	var req connectionUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.UpdateConnection(s.callCtx(r, token), app.UpdateConnectionRequest{
		ConnectionID:      connectionID,
		Provider:          req.Provider,
		BaseURL:           req.BaseURL,
		ModelID:           req.ModelID,
		CustomHeaders:     req.CustomHeaders,
		ExpectedUpdatedAt: req.ExpectedUpdatedAt,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, connectionUpdateResult{Connection: res.Connection})
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
	autoAnswer := req.AutoAnswer || r.URL.Query().Get("answer") == "true"
	res, err := s.deps.Slice.Send(s.callCtx(r, token), app.ConversationSendRequest{
		ConversationID: conversationID,
		Content:        req.Content,
		CorrelationID:  req.CorrelationID,
		AutoAnswer:     autoAnswer,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationExchangeResult{Exchange: res.Exchange, Message: res.Message, Answer: res.Answer})
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

// The conversation attachment surface handlers (IMPL-25; the ADOPTED S12
// supply, P-S12-01…06, as implemented). Every handler rides the existing
// boundary middleware (the Host allowlist, the credential-surface rule,
// CSRF for mutations, the loopback-only bind); the slice's decision port
// is the authoritative check inside. The paths are this adapter's own —
// no published contract carries them.

// handleConversationAttachmentRegistry exposes the attachment registry
// read (P-S12-01): the Core's own supported attachment types and size
// limits as queryable contract data — the read creates nothing and probes
// nothing.
func (s *Server) handleConversationAttachmentRegistry(w http.ResponseWriter, r *http.Request, token, _ string) {
	res, err := s.deps.Slice.ConversationAttachmentRegistry(s.callCtx(r, token))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationAttachmentRegistryResult{Types: res.Types})
}

// handleConversationAttachmentUpload exposes the identity-bearing upload
// (P-S12-02): the body carries the upload identity, the display file
// name, the registry content type, the opaque input revision, and the
// base64 content. The typed rejections (P-S12-04: type membership, then
// the per-type size bound) ride the envelope BEFORE any byte is stored;
// the store's replay ruling returns the existing upload's state on
// (same identity, same digest) and refuses a different digest. A lost
// response resolves ONLY by the status query — never a blind re-upload.
func (s *Server) handleConversationAttachmentUpload(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	var req conversationAttachmentUploadRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.UploadConversationAttachment(s.callCtx(r, token), app.ConversationAttachmentUploadRequest{
		ConversationID: conversationID,
		UploadID:       req.UploadID,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		InputRevision:  req.InputRevision,
		ContentB64:     req.ContentB64,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationAttachmentResult{Attachment: res.Attachment})
}

// handleConversationAttachmentStatus exposes the S4-for-uploads read
// (P-S12-02): the authoritative attachment state for one upload identity,
// resolved by (conversation identity, upload identity). A pure query — it
// never re-uploads, never mutates. An upload identity bound to a
// different conversation is the SAME typed NOT_FOUND as an unknown id
// (the no-leak rule).
func (s *Server) handleConversationAttachmentStatus(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	upload := r.URL.Query().Get("upload_id")
	if upload == "" || len(upload) > maxPathID {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "upload_id"}))
		return
	}
	att, err := s.deps.Slice.ConversationAttachmentStatus(s.callCtx(r, token), conversationID, upload)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationAttachmentResult{Attachment: att})
}

// handleConversationAttachmentList exposes the conversation's attachments
// read (the card-9 list discipline): server-side, bounded, keyset
// pagination, chronological; the cursorless page IS the final page.
func (s *Server) handleConversationAttachmentList(w http.ResponseWriter, r *http.Request, token, conversationID string) {
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
	res, err := s.deps.Slice.ConversationAttachments(s.callCtx(r, token), conversationID, app.ConversationAttachmentListRequest{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationAttachmentListResult{Attachments: res.Attachments, Cursor: res.NextCursor})
}

// handleConversationAttachmentDelete exposes the draft-stage delete: the
// body names exactly the attachment to delete — no implicit target. The
// record row and its cas_references row are removed inside the store; the
// CAS object becomes GC-collectible through the store's normal barrier
// discipline. NEVER touches the Job machinery (X-1).
func (s *Server) handleConversationAttachmentDelete(w http.ResponseWriter, r *http.Request, token, conversationID string) {
	var req conversationAttachmentDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	if err := s.deps.Slice.DeleteConversationAttachment(s.callCtx(r, token), app.ConversationAttachmentDeleteRequest{
		ConversationID: conversationID,
		AttachmentID:   req.AttachmentID,
	}); err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, conversationAttachmentDeleteResult{})
}

// maxNotificationFilterLen bounds a notification filter query value (a
// closed-register value; anything longer cannot be a member — the same
// transport-safeguard class as maxSearchQueryLen).
const maxNotificationFilterLen = 64

// The F12 audit/activity read-surface handlers (IMPL-24, the adopted S11
// classes as implemented). Both ride the existing boundary middleware
// (the Host allowlist, the credential-surface rule, the loopback-only
// bind); the slice's decision port is the authoritative check inside —
// the coded audit.read action over the coded audit resource. Both are
// pure reads over the append-only chain: the handlers decode no payload
// and mutate nothing; the read-only law binds.

// handleAuditList exposes the permission-filtered, server-side, paginated
// audit list (the card-9 working candidate's class): the optional limit
// (the slice's per-surface default applies), the opaque continuation
// cursor (its absence on a response marks the final page), and the
// optional closed-register policy_decision filter (the decision port's
// own allow/deny vocabulary — filter options come from the Core only;
// anything else is the slice's typed validation).
func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	policyDecision := q.Get("policy_decision")
	if len(policyDecision) > maxNotificationFilterLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "policy_decision"}))
		return
	}
	res, err := s.deps.Slice.ListAudit(s.callCtx(r, token), app.AuditListRequest{
		Limit:          limit,
		PolicyDecision: policyDecision,
		Cursor:         cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleAuditExport exposes the M-32-class read-with-export operation
// (the adopted P-S11-03 class as implemented): the optional RFC3339
// date-range bounds and the finite record cap ride the query; the
// scope/count/date-range confirmation is the calling surface's duty
// BEFORE the request (P1.6's precondition). The response carries the
// records, the honest count, the echoed range, the generation instant,
// and the honest truncation flag — the export reads records, mutates
// nothing, and adds no tamper-resistance.
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request, token, _ string) {
	q := r.URL.Query()
	limit, lerr := parseListLimit(q.Get("limit"))
	if lerr != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "limit"}))
		return
	}
	from, to := q.Get("from"), q.Get("to")
	if len(from) > maxListCursorLen || len(to) > maxListCursorLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "from"}))
		return
	}
	res, err := s.deps.Slice.ExportAudit(s.callCtx(r, token), app.AuditExportRequest{
		FromTime: from,
		ToTime:   to,
		Limit:    limit,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleNotificationList exposes the grouped list read (the adopted S6
// supply, P-S6-01; the card-9 working candidate): server-side cursor
// pagination, the Core-defined newest-first order, the grouped collapse
// (the recorded rule's working-candidate mechanics), the optional
// category/read_state filter parameters over the closed registers. The
// limit is optional (the slice's per-surface default applies); the cursor
// is opaque and its absence on a response marks the final page. An empty
// page IS the honest no-notification answer — never an error shape that
// invents one. A read — it mutates nothing.
func (s *Server) handleNotificationList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	category := q.Get("category")
	if len(category) > maxNotificationFilterLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "category"}))
		return
	}
	readState := q.Get("read_state")
	if len(readState) > maxNotificationFilterLen {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "read_state"}))
		return
	}
	res, err := s.deps.Slice.ListNotifications(s.callCtx(r, token), app.ListNotificationsRequest{
		Limit:     limit,
		Category:  category,
		ReadState: readState,
		Cursor:    cursor,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, notificationListResult{Entries: res.Entries, Cursor: res.NextCursor})
}

// handleNotificationCounts exposes the Core-supplied unread counts
// (P-S6-01: unread counts render only from Core-supplied state — never
// locally counted by the UI). A pure read.
func (s *Server) handleNotificationCounts(w http.ResponseWriter, r *http.Request, token, _ string) {
	counts, err := s.deps.Slice.NotificationUnreadCounts(s.callCtx(r, token))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, notificationCountsResult{
		Total:      counts.Total,
		ByCategory: counts.ByCategory,
	})
}

// handleNotificationMutation is the shared O-6 mutation frame (P-S6-02):
// the body names exactly the notification identities to mutate — no
// mark-all-read exists (the named set is mark read/unread/filter/clear).
// The outcome is the typed Core outcome; the refusals ride the D-35
// envelope. A notification read/dismiss NEVER changes the underlying Job,
// Activity, approval, or security record (the non-mutation law — the
// mutations write ONLY notification rows; pinned by test).
func (s *Server) handleNotificationMutation(w http.ResponseWriter, r *http.Request, token string, apply func(context.Context, app.NotificationMutationRequest) (app.NotificationMutationResult, error)) {
	var req notificationMutationRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := apply(s.callCtx(r, token), app.NotificationMutationRequest{NotificationIDs: req.NotificationIDs})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, notificationMutationResult{Updated: res.Updated, Unchanged: res.Unchanged})
}

func (s *Server) handleNotificationMarkRead(w http.ResponseWriter, r *http.Request, token, _ string) {
	s.handleNotificationMutation(w, r, token, s.deps.Slice.MarkNotificationsRead)
}

func (s *Server) handleNotificationMarkUnread(w http.ResponseWriter, r *http.Request, token, _ string) {
	s.handleNotificationMutation(w, r, token, s.deps.Slice.MarkNotificationsUnread)
}

func (s *Server) handleNotificationClear(w http.ResponseWriter, r *http.Request, token, _ string) {
	s.handleNotificationMutation(w, r, token, s.deps.Slice.ClearNotifications)
}

// The B1 KnowledgePack family handlers (DE-0 queue row 1): the published
// /knowledgepacks operations over the slice's knowledge surface. Every
// handler runs the use case through the one decision port inside the slice
// — an unauthenticated request reaches the typed AUTHZ_DENIED/SESSION_INVALID
// envelope, never the data. The submit routes answer 202 (accepted for
// processing — the authoritative state read confirms every outcome); the
// reads answer 200; the error register rides the published envelope exactly
// (writeError).

// handleKnowledgeImportRegistry exposes the supported-source registry read
// (the Core-authoritative read class): the read creates nothing and probes
// nothing — the served contents ARE the canonical values.
func (s *Server) handleKnowledgeImportRegistry(w http.ResponseWriter, r *http.Request, token, _ string) {
	sources, err := s.deps.Slice.KnowledgeImportRegistry(s.callCtx(r, token))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgeRegistryResult{Sources: sources})
}

// handleKnowledgeImportSubmit exposes the M-9 digest-keyed import submit:
// 202 = accepted for processing, never completion.
func (s *Server) handleKnowledgeImportSubmit(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req knowledgeImportRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SubmitKnowledgeImport(s.callCtx(r, token), app.KnowledgeImportRequest{
		IdempotencyKey:            req.IdempotencyKey,
		SourceName:                req.SourceName,
		SourceKind:                req.SourceKind,
		ContentB64:                req.ContentB64,
		DestinationPackID:         req.DestinationPackID,
		DuplicateMode:             req.DuplicateMode,
		ExternalTransferConfirmed: req.ExternalTransferConfirmed,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, knowledgeSubmitResult{Operation: res.Operation, Replayed: res.Replayed})
}

// handleKnowledgeExportSubmit exposes the export submit (the
// read-with-output class — none-required idempotency, safe re-issue).
func (s *Server) handleKnowledgeExportSubmit(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req knowledgeExportRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SubmitKnowledgeExport(s.callCtx(r, token), app.KnowledgeExportRequest{
		CorrelationID:             req.CorrelationID,
		PackID:                    req.PackID,
		VersionID:                 req.VersionID,
		Destination:               req.Destination,
		ExternalTransferConfirmed: req.ExternalTransferConfirmed,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, knowledgeSubmitResult{Operation: res.Operation, Replayed: res.Replayed})
}

// handleKnowledgeOperationStatus exposes the family's authoritative state
// endpoint (a pure query — never a retry).
func (s *Server) handleKnowledgeOperationStatus(w http.ResponseWriter, r *http.Request, token, opID string) {
	op, err := s.deps.Slice.KnowledgeOperationStatus(s.callCtx(r, token), opID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgeOperationResult{Operation: op})
}

// handleKnowledgeOperationCancel exposes the Core-confirmed cancellation
// request: the request is never displayed as the confirmation — the
// authoritative state read confirms the terminal cancelled state.
func (s *Server) handleKnowledgeOperationCancel(w http.ResponseWriter, r *http.Request, token, opID string) {
	op, err := s.deps.Slice.CancelKnowledgeOperation(s.callCtx(r, token), opID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, knowledgeOperationResult{Operation: op})
}

// handleKnowledgePackList exposes the workspace-scoped pack list (the
// keyset list semantics; the cursor's absence marks the final page).
func (s *Server) handleKnowledgePackList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	res, err := s.deps.Slice.KnowledgePacks(s.callCtx(r, token), app.KnowledgeListRequest{Limit: limit, Cursor: cursor})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgePackListResult{Packs: res.Packs, Cursor: res.Cursor})
}

// handleKnowledgePackGet exposes one pack's authoritative state read.
func (s *Server) handleKnowledgePackGet(w http.ResponseWriter, r *http.Request, token, packID string) {
	pack, err := s.deps.Slice.KnowledgePack(s.callCtx(r, token), packID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgePackResult{Pack: pack})
}

// handleKnowledgeVersionList exposes one pack's version list.
func (s *Server) handleKnowledgeVersionList(w http.ResponseWriter, r *http.Request, token, packID string) {
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
	res, err := s.deps.Slice.KnowledgeVersions(s.callCtx(r, token), packID, app.KnowledgeListRequest{Limit: limit, Cursor: cursor})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgeVersionListResult{Versions: res.Versions, Cursor: res.Cursor})
}

// handleKnowledgeVersionGet exposes one immutable pack version read (the
// family's canonical deep link; the two-ID route shape).
func (s *Server) handleKnowledgeVersionGet(w http.ResponseWriter, r *http.Request, token, packID, versionID string) {
	version, err := s.deps.Slice.KnowledgePackVersion(s.callCtx(r, token), packID, versionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, knowledgeVersionResult{Version: version})
}

// The B2 Memory family handlers (DE-0 queue row 3): the published /memories
// operations over the slice's memory surface. Every handler runs the use
// case through the one decision port inside the slice — an unauthenticated
// request reaches the typed AUTHZ_DENIED/SESSION_INVALID envelope, never
// the data. The mutation answers are 200-class Core confirmations (the F11
// law); the reads answer 200; the error register rides the published
// envelope exactly (writeError).

// handleMemoryList exposes the scope- and review-filtered list read (the
// keyset list semantics; the cursor's absence marks the final page; the
// M-1 correlation class).
func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	res, err := s.deps.Slice.MemoryItems(s.callCtx(r, token), app.MemoryListRequest{
		Limit: limit, Cursor: cursor, Scope: q.Get("scope"), State: q.Get("state"),
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryItemListResult{Items: res.Items, Cursor: res.Cursor})
}

// handleMemoryAdd exposes the M-17 creation-class add: the Core mints the
// memory_item_id — the body never carries one.
func (s *Server) handleMemoryAdd(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req memoryAddRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.AddMemoryItem(s.callCtx(r, token), app.MemoryAddRequest{
		IdempotencyKey:  req.IdempotencyKey,
		CorrelationID:   req.CorrelationID,
		Scope:           req.Scope,
		Origin:          req.Origin,
		Content:         req.Content,
		OriginatingRefs: req.OriginatingRefs,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryItemGet exposes the inspect read (a pure query — never a
// mutation; the item-state half of the unknown-outcome lookup).
func (s *Server) handleMemoryItemGet(w http.ResponseWriter, r *http.Request, token, itemID string) {
	item, err := s.deps.Slice.MemoryItem(s.callCtx(r, token), itemID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryItemResult{MemoryItem: item})
}

// handleMemoryEdit exposes the M-16 revision-class edit/correct member
// (provenance-preserving; the eligibility and state-reference laws are the
// Core's).
func (s *Server) handleMemoryEdit(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryEditRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.EditMemoryItem(s.callCtx(r, token), itemID, app.MemoryEditRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		ItemStateRef:   req.ItemStateRef,
		Content:        req.Content,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryApprove exposes the explicit review approval (the J-5 rule:
// the request is never displayed as the confirmation — the Core-confirmed
// item state is the truth).
func (s *Server) handleMemoryApprove(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryReviewRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.ApproveMemoryItem(s.callCtx(r, token), itemID, app.MemoryReviewRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryReject exposes the explicit review rejection.
func (s *Server) handleMemoryReject(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryReviewRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.RejectMemoryItem(s.callCtx(r, token), itemID, app.MemoryReviewRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryPin exposes the pin/unpin member (one member operation
// covering both directions via the body's pinned).
func (s *Server) handleMemoryPin(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryPinRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SetMemoryItemPin(s.callCtx(r, token), itemID, app.MemoryPinRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		Pinned:         req.Pinned,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryDelete exposes the destructive delete member. THE PUBLISHED
// KEYING CARRIER (the B2-CA amendment): the published operation declares
// the OPTIONAL MemoryDestructiveRequest keying body carrying the M-16
// keying members — the keying members ride an OPTIONAL body here
// (empty body → the unkeyed record). Only a confirmed
// 200 carries the deletion truth (the F11 law).
func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryDestructiveRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.DeleteMemoryItem(s.callCtx(r, token), itemID, app.MemoryReviewCommand{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryDeleteResult{
		MemoryItemID:  res.MemoryItemID,
		OutcomeState:  res.OutcomeState,
		RetentionNote: res.RetentionNote,
		OperationID:   res.OperationID,
	})
}

// handleMemoryForget exposes the forget member (THE FORGET/DELETE
// DISTINCTNESS recorded at the 2026-11-09 resolution act, binding on
// this family: forget and delete are DISTINCT Core semantics at the
// register level; the M-16 destructive discipline governs regardless).
func (s *Server) handleMemoryForget(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryDestructiveRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.ForgetMemoryItem(s.callCtx(r, token), itemID, app.MemoryReviewCommand{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryMutationResult{MemoryItem: res.MemoryItem, OperationID: res.OperationID})
}

// handleMemoryInclusion exposes the M-18 request-scoped inclusion decision
// (no item-state change, no global effect).
func (s *Server) handleMemoryInclusion(w http.ResponseWriter, r *http.Request, token, itemID string) {
	var req memoryInclusionRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SetMemoryItemInclusion(s.callCtx(r, token), itemID, app.MemoryInclusionRequest{
		CorrelationID: req.CorrelationID,
		RequestRef:    req.RequestRef,
		Included:      req.Included,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryInclusionResult{
		MemoryItemID: res.MemoryItemID,
		RequestRef:   res.RequestRef,
		Included:     res.Included,
		OperationID:  res.OperationID,
	})
}

// handleMemoryExport exposes the M-32 read-with-export member (mutates
// nothing; honest truncation; SECRETS EXCLUDED BY DEFAULT).
func (s *Server) handleMemoryExport(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req memoryExportRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.ExportMemoryItems(s.callCtx(r, token), app.MemoryExportRequest{
		CorrelationID: req.CorrelationID,
		Scope:         req.Scope,
		ItemIDs:       req.ItemIDs,
		Destination:   req.Destination,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryExportResult{
		ExportOperationID: res.ExportOperationID,
		Count:             res.Count,
		GeneratedAt:       res.GeneratedAt,
		Truncated:         res.Truncated,
		Items:             res.Items,
	})
}

// handleMemoryOperationStatus exposes the family's authoritative state
// endpoint (a pure query — never a retry; the operation-ID half of the
// unknown-outcome lookup).
func (s *Server) handleMemoryOperationStatus(w http.ResponseWriter, r *http.Request, token, opID string) {
	op, err := s.deps.Slice.MemoryOperationStatus(s.callCtx(r, token), opID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, memoryOperationResult{Operation: op})
}

// The B3 ContextSnapshot family handlers (DE-0 queue row 5): the published
// /context operations over the slice's context surface. Every handler runs
// the use case through the one decision port inside the slice — an
// unauthenticated request reaches the typed AUTHZ_DENIED/SESSION_INVALID
// envelope, never the data. THE CONDITIONAL LAW (carried exactly): the
// save and compare handlers NEVER answer a fabricated success — their
// typed unmet-condition refusals are the served behavior while the owner's
// PROPOSED decisions stand unresolved; the inclusion handler serves the
// M-18 decision class fully; the state handler is the authoritative pure
// query whose unknown identity is the SAME typed 404.

// handleContextSnapshotSave exposes the M-19 save-snapshot command in its
// CONDITIONAL unmet state: the typed unmet-condition refusal (never a 202,
// never a fabricated operation row, never a fake success path).
func (s *Server) handleContextSnapshotSave(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req contextSnapshotSaveRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	_, err := s.deps.Slice.SaveContextSnapshot(s.callCtx(r, token), app.ContextSaveRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		SourceJobID:    req.SourceJobID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	// UNREACHABLE while the durable-storage gate stands (the slice refuses
	// before any success path exists); the compile-time shape is kept for
	// the un-gated producer's day.
	s.writeJSON(w, http.StatusAccepted, contextSnapshotSaveResult{
		Operation: appcontext.Operation{}, Replayed: false,
	})
}

// handleContextSnapshotCompare exposes the M-20 snapshot compare in its
// CONDITIONAL unmet state: the typed CONTEXT_COMPARE_UNSUPPORTED refusal
// (the comparison-unavailable state renders honestly and no diff is ever
// fabricated; the read-only class mutates nothing).
func (s *Server) handleContextSnapshotCompare(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req contextSnapshotCompareRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	_, err := s.deps.Slice.CompareContextSnapshots(s.callCtx(r, token), app.ContextCompareRequest{
		BaselineSnapshotID:  req.BaselineSnapshotID,
		CandidateSnapshotID: req.CandidateSnapshotID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	// UNREACHABLE while the comparison gate stands (the slice refuses
	// before any success path exists); the compile-time shape is kept for
	// the un-gated producer's day.
	s.writeJSON(w, http.StatusOK, contextSnapshotCompareResult{})
}

// handleContextInclusion exposes the M-18 request-scoped inclusion
// decision (UNCONDITIONAL — implemented fully; no item-state change, no
// Memory action row, no scope broadening).
func (s *Server) handleContextInclusion(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req contextInclusionRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	decisions := make([]appcontext.InclusionDecision, 0, len(req.Decisions))
	for _, d := range req.Decisions {
		decisions = append(decisions, appcontext.InclusionDecision{
			MemoryItemID: d.MemoryItemID,
			Included:     d.Included,
		})
	}
	res, err := s.deps.Slice.SetContextInclusion(s.callCtx(r, token), app.ContextInclusionRequest{
		CorrelationID: req.CorrelationID,
		RequestRef:    req.RequestRef,
		Decisions:     decisions,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	out := make([]appcontext.InclusionDecisionResult, 0, len(res.Decisions))
	out = append(out, res.Decisions...)
	s.writeJSON(w, http.StatusOK, contextInclusionResult{
		RequestRef: res.RequestRef,
		Decisions:  out,
	})
}

// handleContextOperationStatus exposes the family's authoritative state
// endpoint (a pure query — never a retry; the operation-ID half of the
// unknown-outcome lookup; the unknown identity is the SAME typed
// CONTEXT_SNAPSHOT_NOT_FOUND 404).
func (s *Server) handleContextOperationStatus(w http.ResponseWriter, r *http.Request, token, opID string) {
	op, err := s.deps.Slice.ContextOperationStatus(s.callCtx(r, token), opID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, contextOperationResult{Operation: op})
}

// The B4 Workflow authoring family handlers (DE-0 queue row 6): the
// published /workflows operations over the slice's workflow surface. Every
// handler runs the use case through the one decision port inside the slice
// — an unauthenticated request reaches the typed
// AUTHZ_DENIED/SESSION_INVALID envelope, never the data. THE D-40 GATE
// (carried exactly): NO run-execution handler exists — the only `run`
// surface is the run-history LINKAGE read; every plausible run-execution
// path resolves the router's NOT_FOUND (the register's stable vocabulary).
// THE IF-MATCH CLAUSE: prose-only — no handler reads, sets, or echoes any
// If-Match/ETag/precondition member; the interim conflict rule governs.

// handleWorkflowList exposes the keyset list read (the cursor's absence
// marks the final page; the M-1 correlation class).
func (s *Server) handleWorkflowList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	res, err := s.deps.Slice.WorkflowDefinitions(s.callCtx(r, token), app.WorkflowListRequest{Limit: limit, Cursor: cursor})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowListResult{Definitions: res.Definitions, NextCursor: res.Cursor})
}

// handleWorkflowCreate exposes the aggregate-create member: the Core mints
// the definition identity — the body never carries one (the Core-mints
// law); the keyed replay returns the recorded identity, never a second one.
func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req workflowCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.CreateWorkflowDefinition(s.callCtx(r, token), app.WorkflowCreateRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		DisplayName:    req.DisplayName,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowCreateResult{Definition: res.Definition, Replayed: res.Replayed})
}

// handleWorkflowValidate exposes the Core-authoritative typed validation
// (a read-like operation: it mutates no stored revision and never silently
// saves the draft — the no-silent-save law; the result labels exactly
// which draft it evaluated).
func (s *Server) handleWorkflowValidate(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req workflowValidateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.ValidateWorkflowDraft(s.callCtx(r, token), app.WorkflowValidateRequest{
		CorrelationID: req.CorrelationID,
		DefinitionID:  req.DefinitionID,
		Draft:         req.Draft,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleWorkflowAIDiffPropose exposes the M-23 AI-diff propose member in
// its honest unmet-producer state: the typed unwired-producer refusal (the
// AI-PROPOSAL LAW carried exactly — no proposal is ever fabricated, nothing
// is stored, the approval is draft-local and carries no Core write).
func (s *Server) handleWorkflowAIDiffPropose(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req workflowAIDiffProposeRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	_, err := s.deps.Slice.ProposeWorkflowAIDiff(s.callCtx(r, token), app.WorkflowAIDiffProposeRequest{
		CorrelationID: req.CorrelationID,
		DefinitionID:  req.DefinitionID,
		Draft:         req.Draft,
		Instruction:   req.Instruction,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	// UNREACHABLE while the producer's condition stands (the slice refuses
	// before any success path exists); the compile-time shape is kept for
	// the wired producer's day.
	s.writeJSON(w, http.StatusOK, workflowdomain.AIDiffProposal{})
}

// handleWorkflowDefinitionGet exposes the inspect member: the definition
// and its revision records — each immutable and reachable (the unknown
// identity is the SAME typed 404).
func (s *Server) handleWorkflowDefinitionGet(w http.ResponseWriter, r *http.Request, token, definitionID string) {
	res, err := s.deps.Slice.WorkflowDefinitionInspect(s.callCtx(r, token), definitionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowDefinitionResult{Definition: res.Definition, Revisions: res.Revisions})
}

// handleWorkflowRevisionSave exposes the M-21 save member: the NEW
// immutable revision reference STATED on save; no update-in-place; the
// interim conflict rule governs the concurrent-revision outcome.
func (s *Server) handleWorkflowRevisionSave(w http.ResponseWriter, r *http.Request, token, definitionID string) {
	var req workflowRevisionSaveRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SaveWorkflowRevision(s.callCtx(r, token), definitionID, app.WorkflowRevisionSaveRequest{
		IdempotencyKey:  req.IdempotencyKey,
		CorrelationID:   req.CorrelationID,
		BaseRevisionRef: req.BaseRevisionRef,
		Draft:           req.Draft,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowRevisionSaveResult{RevisionRef: res.RevisionRef, Replayed: res.Replayed})
}

// handleWorkflowRunHistory exposes the run-history LINKAGE read (a pure
// read; the D-40 gate stands — the honest empty page while no Run producer
// exists; missing linkage renders `unavailable`, never an inferred link).
func (s *Server) handleWorkflowRunHistory(w http.ResponseWriter, r *http.Request, token, definitionID string) {
	res, err := s.deps.Slice.WorkflowRunHistory(s.callCtx(r, token), definitionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowRunHistoryResult{Entries: res.Entries})
}

// handleWorkflowDefinitionState exposes the family's authoritative state
// endpoint (a pure query — never a retry; the reconcile anchor after a
// lost write response; the unknown identity is the SAME typed 404).
func (s *Server) handleWorkflowDefinitionState(w http.ResponseWriter, r *http.Request, token, definitionID string) {
	revisionID := r.URL.Query().Get("revision_id")
	res, err := s.deps.Slice.WorkflowDefinitionState(s.callCtx(r, token), definitionID, revisionID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, workflowStateResult{DefinitionState: res})
}

// The B5 Plugin lifecycle family handlers (DE-0 queue row 8): the published
// /plugins operations over the slice's plugin surface. Every handler runs
// the use case through the one decision port inside the slice — an
// unauthenticated request reaches the typed AUTHZ_DENIED/SESSION_INVALID
// envelope, never the data. THE GATED EXECUTION ROWS (carried exactly): NO
// invocation/execution handler exists — no invocation operation, no
// invocation/in-flight state schema, no invocation-status read, no
// invocation events, no quota-value reporting, no steering/stop control;
// every plausible invocation path resolves the router's NOT_FOUND (the
// register's stable vocabulary). THE NEVER-GRANT LAW: no handler grants a
// capability — the user grants/denies; the Core mediates and enforces.
// THE DISTINCTNESS LAWS: the toggle is ONE member covering both directions
// via the body's target state; the revoked states are never exitable
// through it. REDACTION IS THE CORE'S DUTY — the handlers return
// Core-returned content only, never a computed diagnostic.

// handlePluginList exposes the keyset list read (the cursor's absence marks
// the final page; the M-1 correlation class — safe to re-issue). The list
// IS the user-installed list — USER-INSTALLED ≠ CATALOG-LISTED, always; a
// catalog exists ONLY when the Core supplies one (none is published).
func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request, token, _ string) {
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
	res, err := s.deps.Slice.Plugins(s.callCtx(r, token), app.PluginListRequest{Limit: limit, Cursor: cursor})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginListResult{Plugins: res.Plugins, NextCursor: res.Cursor})
}

// handlePluginInstall exposes the C1-07 install member: the Core mints the
// plugin, package, install-record, and grant identities — the body never
// carries one; a duplicate with the same key returns the SAME install-record
// reference, never a second one; the install lands the plugin in its
// installed NOT-yet-enabled state. THE ADMISSION-PIPELINE STRUCTURE, THE
// VERDICT LAW, and THE AUDIT-RECEIPT LAW ride the published description and
// are honored in the slice — the verdict is the CORE's output, the receipt
// rides the Core's audit layer, never a UI-side journal.
func (s *Server) handlePluginInstall(w http.ResponseWriter, r *http.Request, token, _ string) {
	var req pluginInstallRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.InstallPlugin(s.callCtx(r, token), app.PluginInstallRequest{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		PackageRef:     req.PackageRef,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginInstallResult{InstallRecordRef: res.InstallRecordRef, Replayed: res.Replayed})
}

// handlePluginGet exposes the inspect member (a pure read; no state
// change). THE DUTY SPLIT and THE MAY-NEVER LIST carried: the record
// renders ONLY Core-returned verification states — never a computed one.
// The unknown identity and the out-of-scope identity are the SAME typed 404
// (the no-leak rule).
func (s *Server) handlePluginGet(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	rec, err := s.deps.Slice.PluginInspect(s.callCtx(r, token), pluginID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, rec)
}

// handlePluginEnable exposes the M-29 state-toggle member — ONE member
// covering both directions via the body's target state (the published pin
// precedent's shape). A revoked state is never exitable through it (the
// distinctness laws — the typed refusals ride the published register codes
// the states carry).
func (s *Server) handlePluginEnable(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	var req pluginEnableRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.SetPluginEnabled(s.callCtx(r, token), pluginID, app.PluginEnableRequestBody{Enabled: req.Enabled})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginMutationResult{PluginID: res.PluginID, OperationRef: res.OperationRef, Replayed: res.Replayed})
}

// handlePluginUpdate exposes the M-30 update member: the fresh-review law
// gates completion past an un-reviewed permission change (the typed
// refusal stores nothing and consumes no key — a retry never grants); the
// atomic upgrade is the Core-confirmed switch (the prior record intact
// until it commits).
func (s *Server) handlePluginUpdate(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	var req pluginUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.UpdatePlugin(s.callCtx(r, token), pluginID, app.PluginUpdateRequest{
		IdempotencyKey:   req.IdempotencyKey,
		CorrelationID:    req.CorrelationID,
		TargetVersionRef: req.TargetVersionRef,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginMutationResult{PluginID: res.PluginID, OperationRef: res.OperationRef, Replayed: res.Replayed})
}

// handlePluginRemove exposes the M-31 remove member: Core-confirmed
// removal; disposal follows the Core's retention — no erasure claim beyond
// the Core-confirmed scope. THE REMOVE-VS-REVOKE DISTINCTNESS (recorded at
// the 2026-11-09 resolution act, binding on this family): removal and
// grant revocation are DISTINCT Core semantics — removing a plugin does
// not revoke its grants; revoking grants does not remove the artifact;
// each acts only on its own named object. This member implements its own
// published behavior under that law.
func (s *Server) handlePluginRemove(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	var req pluginMutationRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.RemovePlugin(s.callCtx(r, token), pluginID, app.PluginMutationRequestBody{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginMutationResult{PluginID: res.PluginID, OperationRef: res.OperationRef, Replayed: res.Replayed})
}

// handlePluginRevokeGrants exposes the G7 revoke-grants member: revocation
// blocks FUTURE invocations; the current invocation's behavior is
// Core-reported — never guessed (on this Core no invocation path exists
// while the execution rows are gated, so there is no current invocation to
// report — the honest absence, never a fabricated one). A retry never
// re-grants and a denial stands until the user's own explicit act.
func (s *Server) handlePluginRevokeGrants(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	var req pluginMutationRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeEnvelope(w, http.StatusBadRequest, envelope(errs.CodeValidationFailed, errs.NewCorrelationID(),
			map[string]any{"field": "body"}))
		return
	}
	res, err := s.deps.Slice.RevokePluginGrants(s.callCtx(r, token), pluginID, app.PluginMutationRequestBody{
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, pluginMutationResult{PluginID: res.PluginID, OperationRef: res.OperationRef, Replayed: res.Replayed})
}

// handlePluginState exposes the family's authoritative state endpoint (a
// pure query — never a retry; the S0–S4 reconcile anchor after a lost write
// response; the unknown identity is the SAME typed 404). Events are never
// the source of truth; this endpoint is.
func (s *Server) handlePluginState(w http.ResponseWriter, r *http.Request, token, pluginID string) {
	st, err := s.deps.Slice.PluginState(s.callCtx(r, token), pluginID)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}

// decodeOptionalJSON decodes a request body against the published OPTIONAL
// MemoryDestructiveRequest keying carrier (the B2-CA amendment — the
// published delete/forget operations declare this OPTIONAL body): an
// empty body decodes to the zero value; a present body must be one JSON
// object.
func decodeOptionalJSON(r *http.Request, v any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	if r.Body == nil {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
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
	// The Memory family's domain codes (the additive register extension):
	// the scope violation rides 403 (the read/write crossing the forbidden
	// cross-scope default — an authorization-class boundary); the secrets
	// refusal rides 400 (the body's content is unacceptable); the typed
	// item 404 rides 404 (the no-leak rule); the review-state, conflict,
	// and eligibility refusals ride 409 (state/eligibility conflicts with
	// the request).
	case errs.CodeMemoryScopeViolation:
		return http.StatusForbidden
	case errs.CodeMemorySecretsRejected:
		return http.StatusBadRequest
	case errs.CodeMemoryItemNotFound:
		return http.StatusNotFound
	case errs.CodeMemoryReviewStateConflict, errs.CodeMemoryConflict, errs.CodeMemoryNotEditable:
		return http.StatusConflict
	// The Context family's domain codes (the additive register extension):
	// the typed snapshot/operation 404 rides 404 (the no-leak rule — the
	// SAME code for unknown and out-of-scope); the honest unavailable
	// states (the compilation/preview and the comparison-unavailable
	// carrier) ride 503 (the state cannot be served right now; the
	// register's safe-retry class); the budget-exhaustion outcome (the
	// explicit omission-record state, never produced on this surface
	// today) rides 409 (the stated budget conflicts with the
	// compilation's capacity); the reason-unavailable carrier (never
	// produced on this surface today) rides 503 (the fully-reasoned state
	// cannot be served; the absence is displayed as the absence).
	case errs.CodeContextSnapshotNotFound:
		return http.StatusNotFound
	case errs.CodeContextCompilationUnavailable, errs.CodeContextOmissionReasonUnavailable:
		return http.StatusServiceUnavailable
	case errs.CodeContextBudgetExhausted:
		return http.StatusConflict
	case errs.CodeContextCompareUnsupported:
		return http.StatusServiceUnavailable
	// The Workflow family's domain codes (the additive register extension):
	// the typed blocking-error outcome (the save's authoritative re-check
	// refusing a draft carrying blocking findings) rides 400 (the body's
	// content is unacceptable — the typed rejection preserves the draft
	// byte-for-byte client-side); the concurrent-revision outcome, the
	// acknowledgment law's refusal, and the not-saved law's refusal ride
	// 409 (state/eligibility conflicts with the request); the unknown-node
	// code rides 400 (issue-level in validation results; the envelope
	// mapping kept for completeness); the node-permission code rides 403
	// (an authorization-class boundary; the Core re-checks
	// authoritatively).
	case errs.CodeWorkflowValidationFailed, errs.CodeWorkflowUnknownNodeType:
		return http.StatusBadRequest
	case errs.CodeWorkflowRevisionConflict, errs.CodeWorkflowWarningUnacknowledged, errs.CodeWorkflowNotSaved:
		return http.StatusConflict
	case errs.CodeWorkflowNodePermissionDenied:
		return http.StatusForbidden
	// The Plugin family's domain codes (the additive register extension):
	// the three typed INVALID outcomes (a fact about the submitted bytes/
	// manifest) ride 400 (the body's content is unacceptable); the
	// UNKNOWN state's outcome (the state can change only Core-side;
	// reconcile), the INCOMPATIBLE state's outcome, and the REVOKED
	// state's outcome (the operation conflicts with the revoked state)
	// ride 409 (state conflicts with the request); the DENIED state's
	// outcome (the denied grant — an authorization-class boundary; a
	// retry never grants) rides 403.
	case errs.CodePluginSignatureInvalid, errs.CodePluginDigestMismatch, errs.CodePluginManifestInvalid:
		return http.StatusBadRequest
	case errs.CodePluginTrustUnknown, errs.CodePluginABIIncompatible, errs.CodePluginTrustRevoked:
		return http.StatusConflict
	case errs.CodePluginCapabilityDenied:
		return http.StatusForbidden
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

// The retry-eligibility read shape (the F9 family's frozen contract, the
// P3.2 §3.9 [P] set as frozen at the S4 §15 act): the aijobrelay
// .RetryEligibility coded JSON tags inside the retry_eligibility wrapper
// (the coded-name rule: the coded Go name lowercased). Exactly the four
// recorded typed members ride the shape — the eligibility verdict, the
// side-effect risk statement in the recorded exact terms, the
// idempotency guarantee class (the closed three-value register; `open`
// until the Core/API owner's own recorded act defines the retry
// command's keying), and the reconciliation requirement — and nothing
// beyond them; no ineligible-reason field exists (the S7 §5 gap 5 stays
// its recorded owner's).
type retryEligibilityResult struct {
	RetryEligibility aijobrelay.RetryEligibility `json:"retry_eligibility"`
}

// The F2 capability descriptor's wire shape (IMPL-22): the app-layer
// CapabilityDescriptor's coded JSON tags inside the capability_descriptor
// wrapper (the coded-name rule: the coded Go name lowercased). The
// capabilities member is the coded CapabilitySet verbatim — the eleven
// axes as the three-state facts, the limits members (0 = unknown), the
// estimator ref, and the notes law. No source/verification-state/
// freshness member exists in the shape (the coded vocabulary does not
// carry them — the S10 §5 gap 3 stays its recorded owner's; each matrix
// field's presence stays Unverified), and no capability promise exists
// anywhere in it (C3-29's frozen boundary).
type capabilityDescriptorResult struct {
	Descriptor app.CapabilityDescriptor `json:"capability_descriptor"`
}

// The M-7 retry command's wire shapes (IMPL-21): idempotency_reference is
// the Supplied-keyed idempotency reference the retried operation carries
// (the frozen law — the same reference returns the original result,
// replayed, never regenerated); job carries the coded Job struct tags and
// replayed is the coded RetryOutcome field lowercased (the coded-name
// rule). No exactly-once vocabulary exists anywhere in these shapes (the
// exact-terms law).
type retryCommandRequest struct {
	IdempotencyReference string `json:"idempotency_reference"`
}

type retryCommandResult struct {
	Job      aijobrelay.Job `json:"job"`
	Replayed bool           `json:"replayed"`
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
	AutoAnswer    bool   `json:"auto_answer,omitempty"`
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
	Exchange conversation.Exchange       `json:"exchange"`
	Message  conversation.Message        `json:"message"`
	Answer   *conversation.Message       `json:"answer,omitempty"`
}

// The conversation attachment wire shapes (IMPL-25; the ADOPTED S12
// supply, P-S12-01…06, as implemented). The JSON tags are the coded tags
// of the domain attachment contract
// (internal/domain/conversation/attachment.go) and the app registry
// shapes (internal/app/conversation_attachment.go); the wrapper keys are
// the coded struct names lowercased (the coded-name rule). Honest
// boundary: the paths are this adapter's own (non-canonical); NO
// message-binding member, NO snapshot-reference member, and NO
// stream/stop/steering member exists anywhere in these shapes — the
// participation policy is not exercised (P-S12-06) and the answering
// production path is the named owner gap.

// conversationAttachmentUploadRequest is the identity-bearing upload body
// (P-S12-02 as implemented): upload_id is the caller-supplied operation
// identity the S4-for-uploads query resolves through (NOT a canonical
// idempotency key — the identity class stays its owner's decision);
// content_b64 is the base64 (standard, padded) attachment bytes; the
// typed rejections (type membership, then the per-type size bound) ride
// the envelope before any byte is stored.
type conversationAttachmentUploadRequest struct {
	UploadID      string `json:"upload_id"`
	FileName      string `json:"file_name"`
	ContentType   string `json:"content_type"`
	InputRevision string `json:"input_revision"`
	ContentB64    string `json:"content_b64"`
}

// conversationAttachmentDeleteRequest names exactly the attachment to
// delete — no implicit target.
type conversationAttachmentDeleteRequest struct {
	AttachmentID string `json:"attachment_id"`
}

// conversationAttachmentRegistryResult carries the registry read
// (P-S12-01): the Core's own supported types and per-type size limits.
type conversationAttachmentRegistryResult struct {
	Types []app.AttachmentTypeSpec `json:"types"`
}

// conversationAttachmentResult carries one attachment record
// (conversation.Attachment coded tags: attachment_id, conversation_id,
// upload_id, file_name, content_type, size_bytes, digest,
// untrusted_label, input_revision, state, created_at) — the upload
// result and the S4-for-uploads status query's shape.
type conversationAttachmentResult struct {
	Attachment conversation.Attachment `json:"attachment"`
}

// conversationAttachmentListResult carries one bounded attachments page
// (chronological); cursor is OMITTED on the final page — the cursorless
// page IS the end marker.
type conversationAttachmentListResult struct {
	Attachments []conversation.Attachment `json:"attachments"`
	Cursor      string                    `json:"cursor,omitempty"`
}

// conversationAttachmentDeleteResult is the confirmed delete's empty
// success shape (the confirmed-200 IS the outcome; no tombstone exists).
type conversationAttachmentDeleteResult struct{}

// The notification wire shapes (the adopted S6 supply, P-S6-01/02). Every
// key is a coded name: the group-entry JSON tags are the coded
// notification.GroupEntry tags (notification_id, category, state_class,
// related_object_type, related_object_id, created_at, unread_count,
// repeat_count, collapsed, object_state?, presentation); the wrapper keys
// are the coded struct names lowercased (the coded-name rule). The
// four-way presentation state rides the payload non-color; no prompt,
// file path, token, generated output, or private model data exists in any
// shape (the P-S6-06 prohibition list).

type notificationMutationRequest struct {
	// notification_ids names exactly the notifications to mutate — no
	// implicit target, no mark-all-read (the P-S6-02 named set is mark
	// read/unread/filter/clear).
	NotificationIDs []string `json:"notification_ids"`
}

type notificationListResult struct {
	// entries is the grouped list (one entry per related-object group);
	// cursor is the opaque list cursor — omitted on the final page (the
	// cursorless page IS the end marker; no has_more/total vocabulary).
	Entries []notification.GroupEntry `json:"entries"`
	Cursor  string                    `json:"cursor,omitempty"`
}

type notificationCountsResult struct {
	// total is the Core-supplied unread total; by_category the per-category
	// counts (cleared rows excluded). Unread counts render only from
	// Core-supplied state (P-S6-01) — never locally counted by the UI.
	Total      int            `json:"total"`
	ByCategory map[string]int `json:"by_category"`
}

type notificationMutationResult struct {
	// updated is the count of rows that changed; unchanged the count of
	// rows already in the requested state (the convergent no-op — recorded
	// honestly: state-convergent, not a documented idempotency class).
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
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

type connectionRenameRequest struct {
	// display_name is the record's display label (the M-27 rename portion
	// as implemented — the display name ONLY, never the connection
	// identity, never the execution history); expected_updated_at is the
	// marker the caller read the record under — the concurrent-edit
	// guard's input (a stale marker is the typed CONFLICT_STATE 409 with
	// data.currentState). NO secret field exists on this request.
	DisplayName       string `json:"display_name"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

type connectionRenameResult struct {
	// connection is the Core-confirmed view (the record + the masked
	// secret status + the tested-state truth + the active flag).
	Connection app.ConnectionView `json:"connection"`
}

type connectionUpdateRequest struct {
	// provider/base_url/model_id/custom_headers are EXACTLY the coded
	// create shape's non-secret field set (P-S8-02); expected_updated_at
	// is the concurrent-edit guard's marker. NO secret field exists on
	// this request — no update path ever writes a credential.
	Provider          string                       `json:"provider"`
	BaseURL           string                       `json:"base_url,omitempty"`
	ModelID           string                       `json:"model_id"`
	CustomHeaders     []dprovider.ConnectionHeader `json:"custom_headers,omitempty"`
	ExpectedUpdatedAt string                       `json:"expected_updated_at"`
}

type connectionUpdateResult struct {
	// connection is the Core-confirmed view; any accepted update renders
	// the tested state stale-truthfully (the coded inputs digest).
	Connection app.ConnectionView `json:"connection"`
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

// The B1 KnowledgePack family's wire shapes (DE-0 queue row 1): the JSON
// tags are the canonical contract's member names verbatim (the
// KnowledgeImportRequest/KnowledgeExportRequest bodies; the
// KnowledgeOperationSubmitResult/KnowledgePackList/KnowledgePackVersionList
// responses and their {pack}/{version}/{operation}/{sources} wrappers);
// the record members ride the domain structs' contract tags
// (internal/domain/knowledge). No member outside the published schemas
// exists anywhere in these shapes.
type knowledgeImportRequest struct {
	IdempotencyKey            string `json:"idempotency_key"`
	SourceName                string `json:"source_name"`
	SourceKind                string `json:"source_kind"`
	ContentB64                string `json:"content_b64"`
	DestinationPackID         string `json:"destination_pack_id,omitempty"`
	DuplicateMode             string `json:"duplicate_mode,omitempty"`
	ExternalTransferConfirmed bool   `json:"external_transfer_confirmed,omitempty"`
}

type knowledgeExportRequest struct {
	CorrelationID             string `json:"correlation_id"`
	PackID                    string `json:"pack_id"`
	VersionID                 string `json:"version_id,omitempty"`
	Destination               string `json:"destination"`
	ExternalTransferConfirmed bool   `json:"external_transfer_confirmed,omitempty"`
}

type knowledgeSubmitResult struct {
	Operation knowledge.Operation `json:"operation"`
	Replayed  bool                `json:"replayed"`
}

type knowledgeOperationResult struct {
	Operation knowledge.Operation `json:"operation"`
}

type knowledgePackResult struct {
	Pack knowledge.Pack `json:"pack"`
}

type knowledgeVersionResult struct {
	Version knowledge.PackVersion `json:"version"`
}

type knowledgePackListResult struct {
	Packs  []knowledge.Pack `json:"packs"`
	Cursor string           `json:"cursor,omitempty"`
}

type knowledgeVersionListResult struct {
	Versions []knowledge.PackVersion `json:"versions"`
	Cursor   string                  `json:"cursor,omitempty"`
}

type knowledgeRegistryResult struct {
	Sources []knowledge.ImportSourceSpec `json:"sources"`
}

// The B2 Memory family's wire shapes (DE-0 queue row 3): the JSON tags are
// the canonical contract's member names verbatim (the MemoryAddRequest/
// MemoryEditRequest/MemoryReviewRequest/MemoryPinRequest/
// MemoryInclusionRequest/MemoryExportRequest bodies; the MemoryMutation/
// Delete/Inclusion/ExportResult responses and their {memory_item}/
// {operation}/{items} wrappers); the record members ride the domain
// structs' contract tags (internal/domain/memory). No member outside the
// published schemas exists anywhere in these shapes — the one exception is
// the item's `pinned` flag, this Core's served definition of the recorded
// pin-register gap (the published MemoryItem schema carries no
// additionalProperties:false; the pin/unpin member's description requires
// the confirmed pin state to ride the response's item).
type memoryAddRequest struct {
	IdempotencyKey  string   `json:"idempotency_key"`
	CorrelationID   string   `json:"correlation_id"`
	Scope           string   `json:"scope"`
	Origin          string   `json:"origin"`
	Content         string   `json:"content"`
	OriginatingRefs []string `json:"originating_refs,omitempty"`
}

type memoryEditRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	ItemStateRef   string `json:"item_state_ref"`
	Content        string `json:"content"`
}

type memoryReviewRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
}

// memoryDestructiveRequest is the delete/forget members' OPTIONAL keying
// body (the published MemoryDestructiveRequest carrier — the B2-CA
// amendment; an empty body is the unkeyed record).
type memoryDestructiveRequest struct {
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

type memoryPinRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	Pinned         bool   `json:"pinned"`
}

type memoryInclusionRequest struct {
	CorrelationID string `json:"correlation_id"`
	RequestRef    string `json:"request_ref"`
	Included      bool   `json:"included"`
}

type memoryExportRequest struct {
	CorrelationID string   `json:"correlation_id"`
	Scope         string   `json:"scope"`
	ItemIDs       []string `json:"item_ids,omitempty"`
	Destination   string   `json:"destination,omitempty"`
}

type memoryMutationResult struct {
	MemoryItem  memory.Item `json:"memory_item"`
	OperationID string      `json:"operation_id"`
}

type memoryDeleteResult struct {
	MemoryItemID  string `json:"memory_item_id"`
	OutcomeState  string `json:"outcome_state"`
	RetentionNote string `json:"retention_note,omitempty"`
	OperationID   string `json:"operation_id"`
}

type memoryInclusionResult struct {
	MemoryItemID string `json:"memory_item_id"`
	RequestRef   string `json:"request_ref"`
	Included     bool   `json:"included"`
	OperationID  string `json:"operation_id"`
}

type memoryExportResult struct {
	ExportOperationID string        `json:"export_operation_id"`
	Count             int           `json:"count"`
	GeneratedAt       string        `json:"generated_at"`
	Truncated         bool          `json:"truncated"`
	Items             []memory.Item `json:"items"`
}

type memoryItemResult struct {
	MemoryItem memory.Item `json:"memory_item"`
}

type memoryOperationResult struct {
	Operation memory.Operation `json:"operation"`
}

type memoryItemListResult struct {
	Items  []memory.Item `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
}

// The B3 ContextSnapshot family's wire shapes (DE-0 queue row 5): the JSON
// tags are the canonical contract's member names verbatim (the
// ContextSnapshotSaveRequest / ContextSnapshotCompareRequest /
// ContextInclusionRequest bodies; the ContextSnapshotSaveResult /
// ContextSnapshotCompareResult / ContextInclusionResult responses and the
// {operation} state wrapper); the record members ride the domain structs'
// contract tags (internal/contextdomain — the published Context* schemas
// carried verbatim). No member outside the published schemas exists
// anywhere in these shapes. THE CONDITIONAL LAW rides the save/compare
// result shapes: no instance of either result is ever produced while the
// durable-storage and comparison gates stand (the typed refusals are the
// served behavior; the shapes exist for the un-gated producer's day).
type contextSnapshotSaveRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	SourceJobID    string `json:"source_job_id"`
}

type contextSnapshotSaveResult struct {
	Operation appcontext.Operation `json:"operation"`
	Replayed  bool                 `json:"replayed"`
}

type contextSnapshotCompareRequest struct {
	BaselineSnapshotID  string `json:"baseline_snapshot_id"`
	CandidateSnapshotID string `json:"candidate_snapshot_id"`
}

type contextSnapshotCompareResult struct {
	BaselineSnapshotID  string         `json:"baseline_snapshot_id"`
	CandidateSnapshotID string         `json:"candidate_snapshot_id"`
	Comparison          map[string]any `json:"comparison"`
}

type contextInclusionRequest struct {
	CorrelationID string `json:"correlation_id"`
	RequestRef    string `json:"request_ref"`
	Decisions     []struct {
		MemoryItemID string `json:"memory_item_id"`
		Included     bool   `json:"included"`
	} `json:"decisions"`
}

type contextInclusionResult struct {
	RequestRef string                               `json:"request_ref"`
	Decisions  []appcontext.InclusionDecisionResult `json:"decisions"`
}

type contextOperationResult struct {
	Operation appcontext.Operation `json:"operation"`
}

// The B4 Workflow family's wire shapes (the published /workflows members'
// bodies as this adapter decodes and answers them; every key is a coded
// name mirroring the canonical contract member names verbatim — never a
// restatement).

type workflowCreateRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	DisplayName    string `json:"display_name"`
}

type workflowCreateResult struct {
	Definition workflowdomain.Definition `json:"definition"`
	Replayed   bool                      `json:"replayed"`
}

type workflowListResult struct {
	Definitions []workflowdomain.Definition `json:"definitions"`
	NextCursor  string                      `json:"next_cursor,omitempty"`
}

type workflowValidateRequest struct {
	CorrelationID string                      `json:"correlation_id"`
	DefinitionID  string                      `json:"definition_id,omitempty"`
	Draft         workflowdomain.DraftContent `json:"draft"`
}

type workflowAIDiffProposeRequest struct {
	CorrelationID string                      `json:"correlation_id"`
	DefinitionID  string                      `json:"definition_id,omitempty"`
	Draft         workflowdomain.DraftContent `json:"draft"`
	Instruction   string                      `json:"instruction"`
}

type workflowDefinitionResult struct {
	Definition workflowdomain.Definition `json:"definition"`
	Revisions  []workflowdomain.Revision `json:"revisions"`
}

type workflowRevisionSaveRequest struct {
	IdempotencyKey  string                      `json:"idempotency_key"`
	CorrelationID   string                      `json:"correlation_id"`
	BaseRevisionRef string                      `json:"base_revision_ref"`
	Draft           workflowdomain.DraftContent `json:"draft"`
}

type workflowRevisionSaveResult struct {
	RevisionRef string `json:"revision_ref"`
	Replayed    bool   `json:"replayed"`
}

type workflowRunHistoryResult struct {
	Entries []workflowdomain.RunHistoryEntry `json:"entries"`
}

type workflowStateResult struct {
	DefinitionState workflowdomain.DefinitionState `json:"definition_state"`
}

// The B5 Plugin family's wire shapes (the published /plugins members'
// bodies as this adapter decodes and answers them; every key mirrors the
// canonical contract member names verbatim — never a restatement). The
// inspect and state reads answer the domain types directly — their JSON
// tags ARE the contract member names (plugindomain.Plugin,
// plugindomain.State).

type pluginInstallRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
	PackageRef     string `json:"package_ref"`
}

type pluginInstallResult struct {
	InstallRecordRef string `json:"install_record_ref"`
	Replayed         bool   `json:"replayed"`
}

type pluginListResult struct {
	Plugins    []plugindomain.Plugin `json:"plugins"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type pluginEnableRequest struct {
	Enabled bool `json:"enabled"`
}

type pluginUpdateRequest struct {
	IdempotencyKey   string `json:"idempotency_key"`
	CorrelationID    string `json:"correlation_id"`
	TargetVersionRef string `json:"target_version_ref"`
}

type pluginMutationRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID  string `json:"correlation_id"`
}

type pluginMutationResult struct {
	PluginID     string `json:"plugin_id"`
	OperationRef string `json:"operation_ref"`
	Replayed     bool   `json:"replayed"`
}

// compile-time interface check.
var _ http.Handler = (*Server)(nil)
