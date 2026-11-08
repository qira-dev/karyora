package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/knowledgepack"
	"github.com/qira-dev/karyora/internal/security"
)

// The durable vertical slice use cases (C3-31): submit with idempotency →
// claim → optional KnowledgePack retrieval → ContextSnapshot compile →
// capability-checked provider call → result/receipt/event atomic commit →
// event inspection. This package IS the Core API surface for the slice —
// the CLI (cmd/uawp) and any future transport call the SAME functions, so
// both produce semantically identical state and events. A Job succeeds
// with neither Work nor KnowledgePack (D-12: Work stays excluded).

// ObjectStore is the CAS-backed durable object port (C2-12): job inputs
// and results are content-addressed and reference-registered.
type ObjectStore interface {
	PutObject(ctx context.Context, data []byte, ownerType, ownerID string) (string, error)
	ReadObject(ctx context.Context, digest string) ([]byte, error)
}

// EventReader is the journal inspection port (C2-13): one aggregate's
// events in per-aggregate sequence order.
type EventReader interface {
	JobEvents(ctx context.Context, jobID string) ([]events.Record, error)
}

// ProviderExecutor is the capability-checked provider port (C3-20): the
// concrete executor applies the full gate set (capability, data-handling,
// secret boundary, egress) before any wire moves.
type ProviderExecutor interface {
	Execute(ctx context.Context, plan *dprovider.RequestPlan) (*dprovider.Result, error)
	// ProfileFor reports the profile identity the executor's gates key on
	// (capability facts and the data-handling registry attach to profiles,
	// never to providers in general).
	ProfileFor(model string) dprovider.ProfileID
	// Connection returns the wired connection posture (base URL, credential
	// class, secret reference) — never invented by the use-case layer.
	Connection() dprovider.Connection
	// Capabilities reports the coded capability facts for one profile (the
	// pre-submit review's fact source; the same tested claims the gate
	// consults — unknown means unsupported).
	Capabilities(profile dprovider.ProfileID) dprovider.CapabilitySet
	// DataHandling reports the governing per-profile data-handling record
	// for one feature scope (the review's policy source; the fail-closed
	// egress gate's own input). False when no record governs — the caller
	// fails closed.
	DataHandling(profile dprovider.ProfileID, feature dprovider.Feature) (dprovider.DataHandlingProfile, bool)
	// TestConnection runs the bounded, non-destructive readiness probe
	// (M-25; C3-18) with the executor's own wired resolution store.
	TestConnection(ctx context.Context, plan *dprovider.RequestPlan) error
	// TestConnectionWith is the store-parameterized form: the connection
	// record surface resolves saved credentials through the slice's wired
	// store and an unsaved draft's typed secret through a request-scoped
	// transient store. The full gate set applies either way.
	TestConnectionWith(ctx context.Context, secrets security.SecretStore, plan *dprovider.RequestPlan) error
}

// BackoffRequeue moves Waiting backoff jobs back to Queued (the accepted
// Waiting → Queued transition); the storage adapter implements it.
type BackoffRequeue interface {
	RequeueBackoff(ctx context.Context, now time.Time) ([]string, error)
}

// SliceDeps wires the slice. Every port is an interface — the adapters
// (storage, providers) are composed at cmd/uawp.
type SliceDeps struct {
	Relay       aijobrelay.RelayStore
	Snapshots   contextengineStore
	Objects     ObjectStore
	Events      EventReader
	Packs       knowledgepackStore // optional: nil disables retrieval
	Guard       *security.Guard
	Providers   map[string]ProviderExecutor
	Backoff     BackoffRequeue  // optional
	Connections ConnectionStore // optional: nil disables the connection surface (typed refusals)
	// Conversations is the durable conversation-aggregate port (the adopted
	// S5 mapping, P-S5-01). Optional: nil disables the conversation surface
	// (typed refusals, fail closed).
	Conversations ConversationStore
	// ConversationAttachments is the durable conversation-attachment port
	// (the adopted S12 supply, P-S12-01…06 — the F4 attachment-contract
	// working candidates; IMPL-25). Optional: nil disables the attachment
	// surface (typed refusals, fail closed) — every other surface is
	// unchanged.
	ConversationAttachments ConversationAttachmentStore
	// Notifications is the durable notification-record port (the adopted
	// S6 supply, P-S6-03: the identity store is new Core state). Optional:
	// nil disables the notification surface (typed refusals, fail closed).
	Notifications NotificationStore
	// Knowledge is the B1 KnowledgePack family's durable surface port (the
	// published /knowledgepacks route family; DE-0 queue row 1). Optional:
	// nil disables the knowledge surface (typed refusals, fail closed) —
	// every other surface is unchanged.
	Knowledge KnowledgeStore
	// Secrets is the D-37 SecretWriter face the connection operations use
	// (the keyring store implements both faces). Nil disables credential
	// persistence — the connection writes then refuse with the typed
	// SECRET_UNAVAILABLE family (fail closed).
	Secrets security.SecretWriter
	// ConnectionExecutors constructs the per-connection executors of the
	// custom connection-protocol families (C3-29/C3-30) — the composition
	// root implements the port over the providers package (the app layer
	// cannot construct executors: the dependency map's own rule). Nil
	// keeps the recorded unwired posture for those providers (typed
	// refusals, fail closed).
	ConnectionExecutors ConnectionExecutorFactory
	// Sessions is the local session service (C2-14 §3) the C-4 logout and
	// credential-change revocation commands serve (IMPL-23: the adopted
	// P-S3-03 session/credential half; the coded Revoke/RevokeAll classes
	// served verbatim, never re-implemented). Optional: nil disables the
	// session-command surface (typed refusals, fail closed) — every other
	// surface is unchanged.
	Sessions *security.SessionService
	// AuditReader is the audit read port the F12 audit list and export
	// reads serve (IMPL-24: the adopted S11 classes as implemented — a
	// pure read face over the audit chain frames; no write, no delete,
	// and no correction path exists on the port). Optional: nil disables
	// the audit read surface (typed refusals, fail closed).
	AuditReader AuditReader
	Now         func() time.Time
}

// SubmitRequest is the API submission contract (C3-31 acceptance: the same
// idempotency key plus the same canonical input returns the original Job).
type SubmitRequest struct {
	IdempotencyKey  string
	Task            string
	System          string
	Schema          json.RawMessage
	PackRef         string // "packID" or "packID@version"; empty = no retrieval
	Provider        string // adapter id from the wired provider set
	Model           string
	MaxOutputTokens int
	DeadlineSecs    int64
	// Classification is the workspace data-classification label consulted
	// by the fail-closed egress gate (C3-19). Empty is treated as
	// protected-unlabeled and blocks every real provider.
	Classification string
	// BaseURL is the optional caller-provided endpoint override (C3-29
	// posture: the caller names the destination; the executor's URL
	// validation, egress policy, and TLS floor still apply). Empty keeps
	// the adapter's recorded default base.
	BaseURL string
}

// SubmitResult reports the submitted (or replayed) Job.
type SubmitResult struct {
	Job      aijobrelay.Job
	Replayed bool
}

// JobInput is the canonical durable payload stored in the CAS under the
// Job's input digest. It carries no secrets and no prompt-adjacent state
// beyond the task itself.
type JobInput struct {
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

// Slice bounds (Proposed, pending the recorded owner review).
const (
	MaxTaskRunes   = 1 << 20
	MaxSystemRunes = 1 << 16
)

// Port aliases: the slice consumes the module packages' own persistence
// contracts directly (the dependency map allows app → module packages).
type (
	contextengineStore = contextengine.SnapshotStore
	knowledgepackStore = knowledgepack.StorePort
)

// EnvelopeError carries a machine error envelope (D-35) as an error value.
type EnvelopeError struct{ Envelope errs.Envelope }

func (e *EnvelopeError) Error() string {
	return string(e.Envelope.Code) + " (" + e.Envelope.CorrelationID + ")"
}

// envelopeErr builds a bounded D-35 envelope error; the data maps used by
// the slice are fixed-shape and bounded, so construction cannot fail — a
// construction failure degrades to a plain error, never to silence.
func envelopeErr(code errs.Code, corr string, data map[string]any) error {
	env, err := errs.New(code, corr, data)
	if err != nil {
		return fmt.Errorf("%s: %v", code, data)
	}
	return &EnvelopeError{Envelope: env}
}

// Slice is the API surface over the wired ports.
type Slice struct {
	deps SliceDeps
}

// NewSlice builds the slice; every dependency except Packs/Backoff is
// required.
func NewSlice(deps SliceDeps) (*Slice, error) {
	if deps.Relay == nil || deps.Snapshots == nil || deps.Objects == nil || deps.Events == nil {
		return nil, fmt.Errorf("app: slice requires relay, snapshot, object, and event ports")
	}
	if deps.Guard == nil {
		return nil, fmt.Errorf("app: slice requires the authorization guard (C2-14: no effect without a decision)")
	}
	if len(deps.Providers) == 0 {
		return nil, fmt.Errorf("app: slice requires at least one wired provider executor")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Slice{deps: deps}, nil
}

// authorize runs the one decision port before any effect (C2-14) and maps
// a denial to the D-35 envelope.
func (s *Slice) authorize(ctx context.Context, corr string, r security.Resource, a security.Action) error {
	if s.deps.Guard == nil {
		return fmt.Errorf("app: guard is not wired")
	}
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: principalID(ctx)}
	pctx := security.PolicyContext{CorrelationID: corr, Credential: credential(ctx)}
	decision, auditErr := s.deps.Guard.Authorize(p, r, a, pctx)
	if auditErr != nil && a.StateChanging() {
		return auditErr // the effect is denied, not executed unrecorded (C2-15)
	}
	if !decision.Allow {
		return envelopeErr(errs.CodeAuthzDenied, corr, map[string]any{
			"resource": r.String(), "action": string(a), "reason": decision.ReasonCode,
		})
	}
	return nil
}

func principalID(ctx context.Context) string {
	if v, ok := ctx.Value(callerKey{}).(string); ok && v != "" {
		return v
	}
	return "local"
}

func credential(ctx context.Context) string {
	if v, ok := ctx.Value(credentialKey{}).(string); ok {
		return v
	}
	return security.CredentialIPCCapability
}

type callerKey struct{}

type credentialKey struct{}

// WithCaller records the caller identity and credential for the guard.
func WithCaller(ctx context.Context, principalID, credential string) context.Context {
	ctx = context.WithValue(ctx, callerKey{}, principalID)
	return context.WithValue(ctx, credentialKey{}, credential)
}

// Submit validates, canonicalizes, and durably submits one Job (C3-31):
// same key + same canonical input replays the original Job; same key +
// different input is a CONFLICT, never a second Job.
func (s *Slice) Submit(ctx context.Context, req SubmitRequest) (*SubmitResult, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob}, security.ActionDataWrite); err != nil {
		return nil, err
	}
	if err := validateSubmit(req, corr); err != nil {
		return nil, err
	}
	if _, wired := s.deps.Providers[req.Provider]; !wired {
		// Fail fast at the API boundary: an unwired provider would sit in
		// Queued forever (no capable worker can claim it).
		return nil, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{
			"field": "provider", "reason": "provider is not wired for this core",
		})
	}
	input := JobInput{
		Task: req.Task, System: req.System, Schema: req.Schema, PackRef: req.PackRef,
		Provider: req.Provider, Model: req.Model, MaxOutputTokens: req.MaxOutputTokens,
		Classification: req.Classification, BaseURL: req.BaseURL,
	}
	canonical, err := events.CanonicalJSON(input)
	if err != nil {
		return nil, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "input"})
	}
	digest := DigestOf(canonical)
	if _, err := s.deps.Objects.PutObject(ctx, canonical, "job.input", digest); err != nil {
		return nil, err
	}
	job := aijobrelay.Job{
		JobID:          "job-" + randomID(),
		IdempotencyKey: req.IdempotencyKey,
		InputDigest:    digest,
		State:          aijobrelay.JobCreated,
		Version:        1,
		Requirements:   aijobrelay.Requirements{ProviderID: req.Provider},
		DeadlineSecs:   req.DeadlineSecs,
		CorrelationID:  corr,
	}
	if err := aijobrelay.ValidateSubmit(&job); err != nil {
		return nil, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"reason": err.Error()})
	}
	saved, err := s.deps.Relay.SubmitJob(ctx, job)
	if err != nil {
		// The storage adapter enforces the C3-02 idempotency semantics; the
		// conflict arrives typed and must not be softened.
		return nil, err
	}
	// Replayed reports the coded SubmitResult flag truthfully: the storage
	// adapter either stored the freshly minted Job (our JobID comes back)
	// or returned the original Job under the same idempotency key (a
	// different JobID — the C3-02 replay). The previous signal rode the
	// stored created_at/updated_at discrepancy of an unstamped creation
	// instant; stamping created_at at the insert (the Jobs-list sort key's
	// repair) removed that artifact, so the detection reads the one signal
	// that cannot lie.
	replayed := saved.JobID != job.JobID
	return &SubmitResult{Job: *saved, Replayed: replayed}, nil
}

// Status reports one Job with its current attempt (ActionDataRead).
func (s *Slice) Status(ctx context.Context, jobID string) (aijobrelay.Job, aijobrelay.Attempt, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataRead); err != nil {
		return aijobrelay.Job{}, aijobrelay.Attempt{}, err
	}
	job, err := s.deps.Relay.LoadJob(ctx, jobID)
	if err != nil {
		return job, aijobrelay.Attempt{}, err
	}
	attempt, err := s.deps.Relay.LoadCurrentAttempt(ctx, jobID)
	if err != nil {
		return job, aijobrelay.Attempt{}, nil // a never-claimed Job has no attempt
	}
	return job, attempt, nil
}

// Events reads one Job's journal events (C3-31: inspect and replay).
func (s *Slice) Events(ctx context.Context, jobID string) ([]events.Record, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataRead); err != nil {
		return nil, err
	}
	return s.deps.Events.JobEvents(ctx, jobID)
}

// Result reads one succeeded Job's result payload from the CAS.
func (s *Slice) Result(ctx context.Context, jobID string) ([]byte, error) {
	job, _, err := s.Status(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.State != aijobrelay.JobSucceeded || job.ResultDigest == "" {
		return nil, envelopeErr(errs.CodeNotFound, job.CorrelationID, map[string]any{"reason": "no committed result"})
	}
	return s.deps.Objects.ReadObject(ctx, job.ResultDigest)
}

// Cancel requests cooperative cancellation (ActionDataWrite).
func (s *Slice) Cancel(ctx context.Context, jobID string) error {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataWrite); err != nil {
		return err
	}
	return s.deps.Relay.RequestCancel(ctx, jobID)
}

// validateSubmit maps every request-shape refusal to the typed D-35
// envelope with the request's correlation id (the adopted extraction
// records these as VALIDATION_FAILED envelopes with typed `field` data —
// an envelope requires its correlationId, so the request's id rides every
// refusal).
func validateSubmit(req SubmitRequest, corr string) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "idempotency_key"})
	}
	if req.DeadlineSecs < 0 {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "deadline_secs"})
	}
	return validateSubmissionShape(req.Task, req.System, req.Provider, req.Schema, corr)
}

// validateSubmissionShape is the request-shape rule set shared by the
// submit path and the pre-submit review read (the same field rules, the
// same typed envelopes — one shape law, never two).
func validateSubmissionShape(task, system, provider string, schema json.RawMessage, corr string) error {
	taskRunes := []rune(task)
	if len(taskRunes) == 0 || len(taskRunes) > MaxTaskRunes {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "task"})
	}
	if len([]rune(system)) > MaxSystemRunes {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "system"})
	}
	if provider == "" {
		return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "provider"})
	}
	if len(schema) > 0 {
		var probe any
		if err := json.Unmarshal(schema, &probe); err != nil {
			return envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "schema", "reason": "schema is not valid JSON"})
		}
	}
	return nil
}

// DigestOf is the shared content address: sha256 with the C2-12 grammar.
func DigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A submission without a unique id must never be silently reused.
		panic("app: entropy unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
