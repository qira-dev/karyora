package app

import (
	"context"
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/contextengine"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/security"
)

// The context-snapshot read (the owner's card-11 working candidate,
// 2026-11-02): the Core-side read that returns one stored ContextSnapshot
// per the P3.2 §3.5 Supplied semantics — the included/omitted lists each
// WITH their Core-supplied reasons (the coded omission-reason vocabulary,
// C3-09), the budget pair (the requested bounds vs what the Core reports;
// missing renders absent, never zero — the UI-22 label discipline), the
// exact-vs-approximate labeling, and the snapshot's identity/created-at.
//
// The store's conflict-on-duplicate is consumed AS CODED (C3-10: saving an
// id that already exists is a conflict, never an overwrite — the store's
// typed CONFLICT_STATE); this milestone changes no store behavior.
//
// The no-bulk-content rule (X-6; the P3.2 §3.5 content rules): the view
// returns the STRUCTURE of the compilation — references, digests, trust
// labels, reasons, budget facts, provenance versions — NEVER the compiled
// content itself. The snapshot's AssembledRepresentation (the raw prompt
// and retrieved knowledge text) and the provider handle (optimization
// metadata, never canonical context identity — C3-10) are deliberately
// absent from the view; the digests are one-way structural references the
// record itself carries.
//
// The read is a READ (ActionDataRead through the same decision port every
// read authorizes): it creates nothing, mutates nothing, and compiles
// nothing — this milestone changes no compile/selection logic.

// SnapshotItemView is one included item (the coded SelectedItem: the
// exact canonical reference, the source digest, the trust label, and the
// inclusion reason — verbatim coded values, no text content).
type SnapshotItemView struct {
	Ref        string `json:"ref"`
	Digest     string `json:"digest"`
	TrustLabel string `json:"trust_label"`
	Reason     string `json:"reason"`
}

// SnapshotOmissionView is one excluded candidate with its Core-supplied
// omission reason (the coded C3-09 vocabulary: budget_exhausted,
// item_cap, duplicate, stale_excluded, license_unknown, claim_revoked,
// claim_superseded, pack_quarantined, scope_denied). No silent
// truncation: every exclusion the record carries is reported.
type SnapshotOmissionView struct {
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
}

// SnapshotBudgetRequestedView carries the compilation's requested bounds
// (the coded BudgetRequested). The coded zero means "no bound of this
// kind" — a zero renders ABSENT on the wire, never zero (the UI-22
// label discipline: missing renders Not-reported, never a fabricated 0).
type SnapshotBudgetRequestedView struct {
	MaxInputTokens       *int `json:"max_input_tokens,omitempty"`
	ReservedOutputTokens *int `json:"reserved_output_tokens,omitempty"`
	MaxItems             *int `json:"max_items,omitempty"`
}

// SnapshotBudgetView is the budget pair per the P3.2 §3.5 [S] row: the
// requested bounds vs what the Core reports. What the Core reports for a
// stored snapshot is its own compiled estimate (EstimatedInputTokens,
// labeled an estimate by its coded name and by EstimatorVersion — the
// coded estimator is "a budgeting heuristic, never a tokenizer claim").
// The provider-reported actual usage has NO source in the snapshot
// record, so the member is absent entirely — never a zero, never a
// fabricated count; no cost source exists in code, so no cost member
// exists at all (missing cost is never $0 — UI-22).
type SnapshotBudgetView struct {
	Requested            SnapshotBudgetRequestedView `json:"requested"`
	EstimatedInputTokens int                         `json:"estimated_input_tokens"`
	EstimatorVersion     string                      `json:"estimator_version"`
}

// SnapshotView is the structural snapshot record the read returns: the
// identity, the included/omitted lists with their reasons, the budget
// pair, the exact-vs-approximate label, and the provenance versions. No
// compiled content rides this shape (the no-bulk-content rule above).
type SnapshotView struct {
	SchemaVersion          int                    `json:"schema_version"`
	SnapshotID             string                 `json:"snapshot_id"`
	TaskRef                string                 `json:"task_ref"`
	Scope                  string                 `json:"scope"`
	UserInputDigest        string                 `json:"user_input_digest"`
	AssembledDigest        string                 `json:"assembled_digest"`
	CreatedAt              string                 `json:"created_at"`
	Included               []SnapshotItemView     `json:"included"`
	Omitted                []SnapshotOmissionView `json:"omitted"`
	Budget                 SnapshotBudgetView     `json:"budget"`
	EstimatorVersion       string                 `json:"estimator_version"`
	RendererVersion        string                 `json:"renderer_version"`
	CompressionMethod      string                 `json:"compression_method"`
	RetrievalConfigVersion string                 `json:"retrieval_config_version"`
	// ExactVsApproximate is the P3.2 §3.5 exact-vs-approximate label as
	// the Core determines it for THIS read: the record is the Core's own
	// stored immutable snapshot (produced by its compiler and written
	// once — C3-10), so the value is the stable core_produced. The
	// recorded vocabulary's approximate case belongs to a pre-compile
	// preview read this milestone does not build (no compile/selection
	// changes); no exactness claim beyond this coded fact is made, and
	// the token count carries its own estimate labeling.
	ExactVsApproximate string `json:"exact_vs_approximate"`
}

// snapshotExactValue is the stable exact-vs-approximate value this read
// reports for a stored snapshot (pinned by test; recorded for the freeze).
const snapshotExactValue = "core_produced"

// SnapshotIDFor is the coded snapshot-identity grammar the execution path
// uses (handler's compile: "snap-" + jobID + "-" + attempt seq) — one
// grammar, shared by the write path and this read path, never two.
func SnapshotIDFor(jobID string, attemptSeq int) string {
	return "snap-" + jobID + "-" + fmt.Sprint(attemptSeq)
}

// JobContextSnapshot returns one stored snapshot's structural view for a
// Job's attempt. attemptSeq zero selects the Job's current attempt (the
// coded LoadCurrentAttempt read); an explicit positive seq reads that
// attempt's snapshot. The read authorizes through the decision port like
// every read (Resource job — the snapshot belongs to the durable Job
// slice the compile rides — ActionDataRead); a missing attempt or a
// missing stored snapshot is the typed NOT_FOUND, never a fabricated
// structure. It is a QUERY: nothing is created, mutated, or recompiled.
func (s *Slice) JobContextSnapshot(ctx context.Context, jobID string, attemptSeq int) (*SnapshotView, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceJob, ID: jobID}, security.ActionDataRead); err != nil {
		return nil, err
	}
	if jobID == "" {
		return nil, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "job_id"})
	}
	if attemptSeq <= 0 {
		attempt, err := s.deps.Relay.LoadCurrentAttempt(ctx, jobID)
		if err != nil || attempt.AttemptID == "" {
			// A never-claimed Job has no attempt and therefore no snapshot.
			return nil, envelopeErr(errs.CodeNotFound, corr, map[string]any{
				"reason": "no context snapshot stored for this job",
			})
		}
		attemptSeq = attempt.Seq
	}
	snap, err := s.deps.Snapshots.LoadSnapshot(ctx, SnapshotIDFor(jobID, attemptSeq))
	if err != nil {
		return nil, err
	}
	return snapshotViewOf(snap), nil
}

// snapshotViewOf projects the coded record onto the structural view. The
// projection is pure and lossless over the STRUCTURE: every included and
// omitted entry with its reason, the budget facts, the provenance
// versions, and the digests — and nothing else (no AssembledRepresentation,
// no provider handle, no candidate text: the no-bulk-content rule).
func snapshotViewOf(snap contextengine.ContextSnapshot) *SnapshotView {
	view := &SnapshotView{
		SchemaVersion:          snap.SchemaVersion,
		SnapshotID:             snap.SnapshotID,
		TaskRef:                snap.TaskRef,
		Scope:                  snap.Scope,
		UserInputDigest:        snap.UserInputDigest,
		AssembledDigest:        snap.AssembledDigest,
		CreatedAt:              snap.CreatedAt.UTC().Format(time.RFC3339Nano),
		Included:               []SnapshotItemView{},
		Omitted:                []SnapshotOmissionView{},
		Budget:                 snapshotBudgetView(snap),
		EstimatorVersion:       snap.EstimatorVersion,
		RendererVersion:        snap.RendererVersion,
		CompressionMethod:      snap.CompressionMethod,
		RetrievalConfigVersion: snap.RetrievalConfigVersion,
		ExactVsApproximate:     snapshotExactValue,
	}
	for _, item := range snap.Selected {
		view.Included = append(view.Included, SnapshotItemView{
			Ref:        item.Ref,
			Digest:     item.Digest,
			TrustLabel: string(item.TrustLabel),
			Reason:     item.Reason,
		})
	}
	for _, omission := range snap.Omitted {
		view.Omitted = append(view.Omitted, SnapshotOmissionView{
			Ref:    omission.Ref,
			Reason: omission.Reason,
		})
	}
	return view
}

// snapshotBudgetView projects the budget pair. The coded zero bound means
// "no bound of this kind" (C3-09) — it renders ABSENT, never zero; the
// estimate is reported as the Core compiled it, labeled by the estimator
// version it names.
func snapshotBudgetView(snap contextengine.ContextSnapshot) SnapshotBudgetView {
	view := SnapshotBudgetView{
		Requested:            SnapshotBudgetRequestedView{},
		EstimatedInputTokens: snap.EstimatedInputTokens,
		EstimatorVersion:     snap.EstimatorVersion,
	}
	if snap.BudgetRequested.MaxInputTokens > 0 {
		v := snap.BudgetRequested.MaxInputTokens
		view.Requested.MaxInputTokens = &v
	}
	if snap.BudgetRequested.ReservedOutputTokens > 0 {
		v := snap.BudgetRequested.ReservedOutputTokens
		view.Requested.ReservedOutputTokens = &v
	}
	if snap.BudgetRequested.MaxItems > 0 {
		v := snap.BudgetRequested.MaxItems
		view.Requested.MaxItems = &v
	}
	return view
}
