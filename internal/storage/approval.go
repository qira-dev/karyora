package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/events"
)

// The durable approval request store (the card-14 working candidate's
// implementation, IMPL-7): the Core-issued identity is minted when a Job
// durably enters Waiting{reason: approval} (CommitAttempt's same-transaction
// hook); the accept/reject decision commands apply through the fenced
// single-writer path. A replayed decision never double-applies, and the
// decision's scope is exactly the presented approval identity bound to its
// own Job.

const approvalColumns = `approval_id, job_id, state, decision, decided_by, correlation_id, created_at, decided_at`

func scanApproval(row rowScanner) (aijobrelay.Approval, error) {
	var a aijobrelay.Approval
	var createdAt, decidedAt string
	err := row.Scan(&a.ApprovalID, &a.JobID, &a.State, &a.Decision, &a.DecidedBy,
		&a.CorrelationID, &createdAt, &decidedAt)
	if err != nil {
		return a, err
	}
	if a.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return a, errStoragef(errs.CodeStateCorrupt, "approval created_at does not parse: %v", err)
	}
	if decidedAt != "" {
		if a.DecidedAt, err = time.Parse(time.RFC3339Nano, decidedAt); err != nil {
			return a, errStoragef(errs.CodeStateCorrupt, "approval decided_at does not parse: %v", err)
		}
	}
	return a, nil
}

// newApprovalID mints the Core-issued approval identity (the app-layer id
// grammar: a typed prefix plus random hex). A mint without a unique id must
// never be silently reused — entropy failure aborts, exactly like the
// app-layer id mint.
func newApprovalID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("storage: entropy unavailable: " + err.Error())
	}
	return "appr-" + hex.EncodeToString(b[:])
}

// mintPendingApprovalTx inserts the pending approval row for one raised
// request inside the caller's transaction (the Waiting{approval} commit's
// same-transaction hook).
func mintPendingApprovalTx(tx *sqlTx, jobID string) error {
	_, err := tx.Exec(`INSERT INTO approvals
		(approval_id, job_id, state, decision, decided_by, correlation_id, created_at, decided_at)
		VALUES (?, ?, ?, '', '', '', ?, '')`,
		newApprovalID(), jobID, aijobrelay.ApprovalPending, time.Now().UTC().Format(time.RFC3339Nano))
	return mapVendorError(err)
}

// LoadApproval implements aijobrelay.RelayStore: one approval by its
// Core-issued id; an unknown id is the typed NOT_FOUND.
func (s *Store) LoadApproval(ctx context.Context, approvalID string) (aijobrelay.Approval, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+approvalColumns+` FROM approvals WHERE approval_id = ?`, approvalID)
	approval, err := scanApproval(row)
	if err == sql.ErrNoRows {
		return aijobrelay.Approval{}, errStoragef(errs.CodeNotFound, "approval %q is not stored", approvalID)
	}
	return approval, err
}

// LoadLatestApprovalForJob implements aijobrelay.RelayStore: the Job's
// latest raised approval request (created_at descending); a Job with no
// approval request is the typed NOT_FOUND.
func (s *Store) LoadLatestApprovalForJob(ctx context.Context, jobID string) (aijobrelay.Approval, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	row := s.readers.QueryRowContext(cctx, `SELECT `+approvalColumns+` FROM approvals
		WHERE job_id = ? ORDER BY created_at DESC, approval_id DESC LIMIT 1`, jobID)
	approval, err := scanApproval(row)
	if err == sql.ErrNoRows {
		return aijobrelay.Approval{}, errStoragef(errs.CodeNotFound, "job %q has no approval request", jobID)
	}
	return approval, err
}

// DecideApproval implements aijobrelay.RelayStore (the card-14 working
// candidate): the decision command applies under the fenced single-writer
// path. A pending approval resolves exactly once — accepted moves the Job
// Waiting → Queued (the re-queue; the waiting reason clears), rejected
// moves it Waiting → Failed (the card-14 register-consistent extension: the
// terminal disposition of a known policy refusal). The Job update is
// version-fenced and the approval update is state-fenced in ONE
// transaction, so a replayed decision can never double-apply: the same
// decision on a decided approval returns the recorded outcome unchanged
// (Replayed=true), and a contradicting decision is the typed
// CONFLICT_STATE. The decision's scope is exactly the presented approval
// identity bound to its own Job — an approval of another Job is
// indistinguishable from an unknown id (the no-existence-leak shape).
func (s *Store) DecideApproval(ctx context.Context, dec aijobrelay.ApprovalDecision) (aijobrelay.ApprovalDecisionResult, error) {
	if dec.JobID == "" || dec.ApprovalID == "" || dec.Actor == "" {
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeValidationFailed, "an approval decision requires its job scope, approval id, and actor")
	}
	if !aijobrelay.ValidApprovalDecision(dec.Decision) {
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeValidationFailed, "decision %q is not a registered approval decision", dec.Decision)
	}
	approval, err := s.LoadApproval(ctx, dec.ApprovalID)
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	// The scope-bound law (requirement (c)): the decision acts on exactly
	// the presented approval identity bound to its own Job. An approval
	// bound to a different Job is indistinguishable from an unknown id —
	// the same typed NOT_FOUND shape (an error never confirms an
	// inaccessible object).
	if approval.JobID != dec.JobID {
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeNotFound, "approval %q is not stored", dec.ApprovalID)
	}
	job, err := s.LoadJob(ctx, approval.JobID)
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	// The replay ruling (requirement (b), the coded command conventions): a
	// decided approval is immutable. The same decision re-presented returns
	// the recorded outcome without re-applying — no state change, no second
	// event. A contradicting decision is the coded CONFLICT_STATE, never a
	// re-decision.
	if approval.State != aijobrelay.ApprovalPending {
		if aijobrelay.DecisionForState(approval.State) == dec.Decision {
			return aijobrelay.ApprovalDecisionResult{Approval: approval, Job: job, Replayed: true}, nil
		}
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeConflictState,
			"approval %q was already decided (%s); a contradicting decision is refused", approval.ApprovalID, approval.State)
	}
	// The state gate (requirement (d)): a fresh decision resolves an
	// approval whose Job is still Waiting{approval}. Any other Job state is
	// the coded wrong-state refusal.
	if job.State != aijobrelay.JobWaiting || job.WaitingReason != string(aijobrelay.WaitApproval) {
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeConflictState,
			"job %q is not waiting for approval (state %s); the decision is refused", job.JobID, job.State)
	}
	toState := aijobrelay.JobQueued
	if dec.Decision == aijobrelay.DecisionReject {
		toState = aijobrelay.JobFailed
	}
	if !aijobrelay.CanTransition(job.State, toState) {
		// Defensive: the table above guarantees both exits; an unreachable
		// branch refuses rather than writing an illegal transition.
		return aijobrelay.ApprovalDecisionResult{}, errStoragef(errs.CodeConflictState,
			"illegal transition %s → %s; the decision is refused", job.State, toState)
	}
	toApprovalState := aijobrelay.ApprovalAccepted
	if dec.Decision == aijobrelay.DecisionReject {
		toApprovalState = aijobrelay.ApprovalRejected
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	corr := dec.CorrelationID
	if corr == "" {
		corr = job.CorrelationID
	}
	err = s.withWriter(ctx, func(tx *sqlTx) error {
		// The approval resolves first, state-fenced: a concurrent decision
		// (or a replay racing the apply) wins the fence and this command
		// refuses — the effect is applied exactly once.
		res, err := tx.Exec(`UPDATE approvals SET state = ?, decision = ?, decided_by = ?, correlation_id = ?, decided_at = ?
			WHERE approval_id = ? AND state = ?`,
			toApprovalState, dec.Decision, dec.Actor, dec.CorrelationID, now, dec.ApprovalID, aijobrelay.ApprovalPending)
		if err != nil {
			return mapVendorError(err)
		}
		if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
			return errStoragef(errs.CodeConflictState, "approval %q is no longer pending; the decision is refused", dec.ApprovalID)
		}
		// The Job transition, version-fenced (the coded fencing; a stale
		// writer can never move the Job).
		res, err = tx.Exec(`UPDATE jobs SET state = ?, version = version + 1, waiting_reason = '', updated_at = ?
			WHERE job_id = ? AND version = ? AND state = ?`,
			toState, now, job.JobID, job.Version, aijobrelay.JobWaiting)
		if err != nil {
			return mapVendorError(err)
		}
		if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
			return errStoragef(errs.CodeConflictState, "job %q changed state during the decision; the decision is refused", job.JobID)
		}
		return appendEventTx(tx, events.Record{
			EventID:       "evt-" + job.JobID + "-" + toState + "-" + fmt.Sprint(job.Version),
			AggregateType: "job",
			AggregateID:   job.JobID,
			EventType:     "job." + toState,
			CorrelationID: corr,
			Producer:      s.instanceUUID,
			InlinePayload: mustJSON(map[string]any{
				"approval_id": approval.ApprovalID,
				"decision":    dec.Decision,
				"reason":      "approval_decision",
			}),
		})
	})
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	decided, err := s.LoadApproval(ctx, dec.ApprovalID)
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	updated, err := s.LoadJob(ctx, job.JobID)
	if err != nil {
		return aijobrelay.ApprovalDecisionResult{}, err
	}
	return aijobrelay.ApprovalDecisionResult{Approval: decided, Job: updated, Replayed: false}, nil
}
