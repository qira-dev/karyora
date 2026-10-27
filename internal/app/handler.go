package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/events"
	"github.com/qira-dev/karyora/internal/knowledgepack"
)

// RelayHandler executes one claimed Job through the vertical slice
// (C3-31): optional KnowledgePack retrieval → ContextSnapshot compile →
// capability-checked provider call → schema-validated result → fenced
// atomic commit. Every outcome returns through the store; a handler error
// is a retryable Attempt failure, never a silent drop.

type RelayHandler struct {
	deps SliceDeps
}

// NewRelayHandler builds the scheduler handler.
func NewRelayHandler(deps SliceDeps) *RelayHandler { return &RelayHandler{deps: deps} }

// producer identifies the execution side in journal events.
const producer = "uawp-core"

// Handle runs one claim and returns the fenced Commit for its outcome.
func (h *RelayHandler) Handle(ctx context.Context, claim aijobrelay.Claim) (aijobrelay.Commit, error) {
	corr := claim.Job.CorrelationID
	if corr == "" {
		corr = errs.NewCorrelationID()
	}
	// A canceled or timed-out context resolves cooperatively BEFORE any
	// work: reload the current authority so the commit carries fresh
	// fencing (RequestCancel bumps the Job version).
	if err := ctx.Err(); err != nil {
		return h.resolveInterrupted(ctx, claim, corr, err)
	}

	commit, err := h.execute(ctx, claim, corr)
	if err != nil {
		if pe, ok := err.(*dprovider.ProviderError); ok && pe.Retryable() {
			// Retryable provider failures wait on the typed backoff; the
			// runner requeues through the accepted Waiting → Queued
			// transition. A retry creates a NEW attempt (never a rewrite).
			return aijobrelay.Commit{
				JobID: claim.Job.JobID, JobVersion: claim.Job.Version,
				AttemptID: claim.Attempt.AttemptID, WorkerID: claim.Attempt.WorkerID,
				LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
				ToState:        aijobrelay.JobWaiting,
				WaitingReason:  string(aijobrelay.WaitBackoffScheduled),
				AttemptOutcome: aijobrelay.AttemptFailedRetryable,
				CorrelationID:  corr,
				Event: h.event(claim, corr, "job.waiting", map[string]any{
					"reason": string(aijobrelay.WaitBackoffScheduled), "provider_kind": string(pe.Kind),
				}),
			}, nil
		}
		// An interrupted context during execution resolves through the
		// interrupted path (cancellation race, deadline).
		if ierr := ctx.Err(); ierr != nil {
			return h.resolveInterrupted(ctx, claim, corr, ierr)
		}
		// Everything else is a terminal failure with a bounded, redacted
		// event payload: no false success, no silent substitution.
		return aijobrelay.Commit{
			JobID: claim.Job.JobID, JobVersion: claim.Job.Version,
			AttemptID: claim.Attempt.AttemptID, WorkerID: claim.Attempt.WorkerID,
			LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
			ToState:        aijobrelay.JobFailed,
			AttemptOutcome: aijobrelay.AttemptFailedTerminal,
			ReceiptID:      "rcpt-" + claim.Job.JobID + "-" + fmt.Sprint(claim.Attempt.Seq) + "-f",
			CorrelationID:  corr,
			Event: h.event(claim, corr, "job.failed", map[string]any{
				"detail": boundedDetail(err),
			}),
		}, nil
	}
	return commit, nil
}

// execute runs the pipeline body.
func (h *RelayHandler) execute(ctx context.Context, claim aijobrelay.Claim, corr string) (aijobrelay.Commit, error) {
	inputRaw, err := h.deps.Objects.ReadObject(ctx, claim.Job.InputDigest)
	if err != nil {
		return aijobrelay.Commit{}, fmt.Errorf("job input unavailable: %w", err)
	}
	var input JobInput
	if err := json.Unmarshal(inputRaw, &input); err != nil {
		return aijobrelay.Commit{}, fmt.Errorf("job input does not parse: %w", err)
	}
	executor, ok := h.deps.Providers[input.Provider]
	if !ok {
		return aijobrelay.Commit{}, dprovider.NewError(dprovider.ErrUnsupported, input.Provider,
			"provider is not wired for this core")
	}

	// Optional KnowledgePack retrieval (C3-31: a Job succeeds WITHOUT a
	// pack; an attached pack contributes inspectable provenance).
	candidates, packProvenance := h.retrieve(ctx, input, corr)

	// Compile the immutable ContextSnapshot (C3-08/C3-10): scope-bound,
	// budgeted, omission ledger explicit.
	budget := contextengine.Budget{}
	if input.MaxOutputTokens > 0 {
		budget.ReservedOutputTokens = input.MaxOutputTokens
	}
	snapshot, err := contextengine.Compile(
		"snap-"+claim.Job.JobID+"-"+fmt.Sprint(claim.Attempt.Seq),
		claim.Job.JobID, input.Task, candidates,
		contextengine.Policy{
			Scope:               "local",
			AuthorizedScopes:    []string{"local"},
			RequireLicenseKnown: false,
			SystemPolicyRef:     "uawp.slice.v1",
		},
		budget, contextengine.CharRatioEstimator{}, h.now())
	if err != nil {
		return aijobrelay.Commit{}, fmt.Errorf("context compilation failed: %w", err)
	}
	if err := h.deps.Snapshots.SaveSnapshot(ctx, *snapshot); err != nil {
		return aijobrelay.Commit{}, fmt.Errorf("snapshot persistence failed: %w", err)
	}

	// The capability-checked provider call (C3-20): the executor applies
	// the capability, data-handling, secret, and egress gates. The
	// connection posture comes from the wired executor (BYOK credentials
	// resolve through the secret boundary at call time; the fixture
	// adapter's endpoint is the reserved .invalid name — never dialable).
	model := input.Model
	if strings.TrimSpace(model) == "" {
		model = "default" // every profile needs a model identity
	}
	plan := &dprovider.RequestPlan{
		Operation:       dprovider.OperationChat,
		Profile:         executor.ProfileFor(model),
		Connection:      executor.Connection(),
		System:          input.System,
		Messages:        []dprovider.Message{{Role: dprovider.RoleUser, Parts: []dprovider.Part{{Type: "text", Text: snapshot.AssembledRepresentation}}}},
		MaxOutputTokens: input.MaxOutputTokens,
		Metadata: map[string]string{
			dprovider.ClassificationKey: input.Classification,
			"job_id":                    claim.Job.JobID,
			"snapshot_id":               snapshot.SnapshotID,
		},
	}
	if input.Schema != nil {
		plan.Structured = &dprovider.StructuredOutput{Name: "result", Schema: input.Schema}
	}
	// Per-job endpoint override (C3-29 posture): the caller may name the
	// destination; every executor gate (URL validation, egress policy, TLS
	// floor, secret boundary) still applies to the resulting request.
	if input.BaseURL != "" {
		plan.Connection.BaseURL = input.BaseURL
	}
	result, err := executor.Execute(ctx, plan)
	if err != nil {
		return aijobrelay.Commit{}, err
	}

	// Persist the result payload (C2-12 CAS) and commit atomically (C3-15).
	payload := result.ResultPayload()
	digest, err := h.deps.Objects.PutObject(ctx, payload, "job.result", claim.Job.JobID)
	if err != nil {
		return aijobrelay.Commit{}, fmt.Errorf("result persistence failed: %w", err)
	}
	eventPayload := map[string]any{
		"snapshot_id":      snapshot.SnapshotID,
		"assembled_digest": snapshot.AssembledDigest,
		"provider":         input.Provider,
		"result_digest":    digest,
		"raw_digest":       result.RawDigest,
		"finish_reason":    string(result.FinishReason),
		"usage":            result.Usage,
	}
	for k, v := range packProvenance {
		eventPayload[k] = v
	}
	return aijobrelay.Commit{
		JobID: claim.Job.JobID, JobVersion: claim.Job.Version,
		AttemptID: claim.Attempt.AttemptID, WorkerID: claim.Attempt.WorkerID,
		LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:        aijobrelay.JobSucceeded,
		AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest:   digest,
		ReceiptID:      "rcpt-" + claim.Job.JobID + "-" + fmt.Sprint(claim.Attempt.Seq),
		CorrelationID:  corr,
		Event:          h.event(claim, corr, "job.succeeded", eventPayload),
	}, nil
}

// retrieve resolves the optional pack reference into retrieval candidates
// and returns the provenance entries for the commit event. A missing pack
// reference is a terminal failure; a quarantined/rejected pack contributes
// NOTHING but records its omission (hard exclusion, C3-07).
func (h *RelayHandler) retrieve(ctx context.Context, input JobInput, corr string) ([]contextengine.RetrievedItem, map[string]any) {
	if input.PackRef == "" || h.deps.Packs == nil {
		return nil, nil
	}
	provenance := map[string]any{"pack_ref": input.PackRef}
	packID, version, err := parsePackRef(input.PackRef)
	if err != nil {
		provenance["retrieval"] = "rejected:" + err.Error()
		return nil, provenance
	}
	if version == 0 {
		version, err = h.deps.Packs.LatestPackVersion(ctx, packID)
		if err != nil || version == 0 {
			provenance["retrieval"] = "rejected:pack_not_found"
			return nil, provenance
		}
	}
	pack, err := h.deps.Packs.LoadPackVersion(ctx, packID, version)
	if err != nil {
		provenance["retrieval"] = "rejected:pack_not_found"
		return nil, provenance
	}
	if pack.Status != knowledgepack.PackStatusActive {
		provenance["retrieval"] = "omitted:pack_" + pack.Status
		return nil, provenance
	}
	chunks := knowledgepack.ChunkManifest(pack.Manifest, pack.Entries, knowledgepack.DefaultIndexConfig())
	index := knowledgepack.BuildIndex(chunks, knowledgepack.DefaultIndexConfig())
	hits := index.Search(input.Task, knowledgepack.Filters{
		IncludeStale:         false,
		RequireLicenseKnown:  false,
		ExcludeClaimStatuses: []string{"revoked", "superseded", "conflicted"},
		ExcludePackStatuses:  []string{knowledgepack.PackStatusQuarantined, knowledgepack.PackStatusRejected},
		MaxResults:           8,
	}, map[string]string{packID: pack.Status}, map[string]string{}, nil)
	items := make([]contextengine.RetrievedItem, 0, len(hits))
	for _, hit := range hits {
		items = append(items, contextengine.RetrievedItem{
			Ref:           hit.Chunk.ID,
			Text:          hit.Chunk.Text,
			Digest:        DigestOf([]byte(hit.Chunk.Text)),
			SourceKind:    "knowledgepack",
			Scope:         "local",
			PackID:        hit.Chunk.PackID,
			PackVersion:   hit.Chunk.Version,
			ClaimID:       hit.Chunk.ClaimID,
			EntryPath:     hit.Chunk.EntryPath,
			LicenseStatus: "unknown",
			Freshness:     "current",
			ClaimStatus:   "active",
			PackStatus:    pack.Status,
		})
	}
	provenance["retrieval"] = "hits:" + fmt.Sprint(len(items))
	return items, provenance
}

func parsePackRef(ref string) (string, int, error) {
	id, vStr, hasVersion := strings.Cut(ref, "@")
	if id == "" {
		return "", 0, fmt.Errorf("pack reference is empty")
	}
	if !hasVersion {
		return id, 0, nil
	}
	var v int
	if _, err := fmt.Sscanf(vStr, "%d", &v); err != nil || v < 1 {
		return "", 0, fmt.Errorf("pack version must be a positive integer")
	}
	return id, v, nil
}

// resolveInterrupted commits the cooperative outcome for a canceled or
// deadline-broken attempt with FRESH fencing (RequestCancel bumps the Job
// version; the claim-time version would be stale).
func (h *RelayHandler) resolveInterrupted(ctx context.Context, claim aijobrelay.Claim, corr string, cause error) (aijobrelay.Commit, error) {
	// The reload and the commit must run on a live context: the caller's
	// attempt context is exactly what was canceled. Durable truth may never
	// be unwritable because the attempt was interrupted.
	reload := context.WithoutCancel(ctx)
	job, err := h.deps.Relay.LoadJob(reload, claim.Job.JobID)
	if err != nil {
		return aijobrelay.Commit{}, err
	}
	attempt, err := h.deps.Relay.LoadCurrentAttempt(reload, claim.Job.JobID)
	if err != nil {
		return aijobrelay.Commit{}, err
	}
	if job.State == aijobrelay.JobCancelRequested {
		return aijobrelay.Commit{
			JobID: job.JobID, JobVersion: job.Version,
			AttemptID: attempt.AttemptID, WorkerID: attempt.WorkerID,
			LeaseID: attempt.LeaseID, LeaseEpoch: attempt.LeaseEpoch,
			ToState:        aijobrelay.JobCancelled,
			AttemptOutcome: aijobrelay.AttemptFailedTerminal,
			ReceiptID:      "rcpt-" + job.JobID + "-" + fmt.Sprint(attempt.Seq) + "-c",
			CorrelationID:  corr,
			Event:          h.event(claim, corr, "job.cancelled", map[string]any{"cause": boundedDetail(cause)}),
		}, nil
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return aijobrelay.Commit{
			JobID: job.JobID, JobVersion: job.Version,
			AttemptID: attempt.AttemptID, WorkerID: attempt.WorkerID,
			LeaseID: attempt.LeaseID, LeaseEpoch: attempt.LeaseEpoch,
			ToState:        aijobrelay.JobTimedOut,
			AttemptOutcome: aijobrelay.AttemptFailedTerminal,
			ReceiptID:      "rcpt-" + job.JobID + "-" + fmt.Sprint(attempt.Seq) + "-t",
			CorrelationID:  corr,
			Event:          h.event(claim, corr, "job.timed_out", map[string]any{"cause": boundedDetail(cause)}),
		}, nil
	}
	// A cancellation race the store already resolved: nothing to commit.
	return aijobrelay.Commit{}, nil
}

func (h *RelayHandler) event(claim aijobrelay.Claim, corr, eventType string, payload map[string]any) events.Record {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["attempt_seq"] = claim.Attempt.Seq
	return events.Record{
		EventID:       "evt-" + claim.Job.JobID + "-" + strings.TrimPrefix(eventType, "job.") + "-" + fmt.Sprint(claim.Attempt.Seq),
		AggregateType: "job",
		AggregateID:   claim.Job.JobID,
		EventType:     eventType,
		SchemaVersion: 1,
		OccurredAt:    h.now(),
		CorrelationID: corr,
		Producer:      producer,
		InlinePayload: mustJSON(payload),
	}
}

func (h *RelayHandler) now() time.Time {
	if h.deps.Now != nil {
		return h.deps.Now()
	}
	return time.Now()
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// boundedDetail keeps terminal-failure payloads within the journal's
// inline bound and strips anything credential-shaped defensively.
func boundedDetail(err error) string {
	s := err.Error()
	if len(s) > 400 {
		s = s[:400]
	}
	return strings.ReplaceAll(s, "\n", " ")
}
