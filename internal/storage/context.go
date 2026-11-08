package storage

import (
	stdcontext "context"
	"database/sql"
	"encoding/json"
	"time"

	contextdomain "github.com/qira-dev/karyora/internal/domain/context"
	"github.com/qira-dev/karyora/internal/domain/errs"
)

// The Store implements the B3 ContextSnapshot family's durable surface
// (DE-0 queue row 5): the M-18 request-scoped inclusion ruling runs
// transactionally under the single-writer gate, and the authoritative
// operation-record read serves the family's state endpoint. THE
// CONDITIONAL LAW (BINDING, carried exactly): the M-19 save-snapshot
// command is CONDITIONAL on durable snapshot storage existing — the
// owner's PROPOSED decision, unresolved — so NO save path and NO durable
// snapshot storage is built here; while the gate stands no operation row
// can exist and `snapshot.saved` is never emitted. The M-20 compare is
// read-only and CONDITIONAL on stable Core comparison — the family
// answers the typed CONTEXT_COMPARE_UNSUPPORTED and this store implements
// no comparison. The M-18 inclusion decision is UNCONDITIONAL and
// implemented fully: the decision rows' own (request_ref, memory_item_id)
// scope IS the key carrier (the published law: the inclusion decision
// carries NO durable operation row), the fingerprint is the request's
// correlation ID + the decided item IDs, and the retention is
// request-lifetime at most (the decision row's own lifetime — recorded
// honestly). Events are never the source of truth — the authoritative
// state endpoint is.

// ApplyContextInclusion applies the M-18 request-scoped ruling for the
// stated request's decision set: the same (request_ref, item) row under
// the SAME correlation replays the recorded decision; a different
// correlation under the same request is the caller's NEW decision record
// (supersedes within the request — the request-scoped semantics); the
// decision changes no Memory item's lifecycle state, creates no Memory
// action row, and never widens any scope. Every decision in one call is
// one transaction (the F11 law: only a confirmed outcome carries the
// truth). The result's reason members stay unset — the Core-provided-only
// reason law: no reason producer exists on this surface, and absence is
// the absence, never a synthesized reason.
func (s *Store) ApplyContextInclusion(ctx stdcontext.Context, intent contextdomain.InclusionIntent) (contextdomain.InclusionOutcome, error) {
	if intent.CorrelationID == "" || intent.RequestRef == "" || len(intent.Decisions) == 0 {
		return contextdomain.InclusionOutcome{}, errStoragef(errs.CodeValidationFailed, "context inclusion intent lacks its identity members")
	}
	var outcome contextdomain.InclusionOutcome
	err := s.CommitStateChange(ctx, intent.CorrelationID, func(tx *sqlTx) error {
		confirmed := make([]contextdomain.InclusionDecisionResult, 0, len(intent.Decisions))
		now := s.now().UTC().Format(time.RFC3339Nano)
		for _, decision := range intent.Decisions {
			var corr string
			var included int64
			row := tx.QueryRow(`SELECT correlation_id, included FROM context_inclusions
				WHERE request_ref = ? AND memory_item_id = ?`, intent.RequestRef, decision.MemoryItemID)
			err := row.Scan(&corr, &included)
			switch {
			case err == nil && corr == intent.CorrelationID:
				// The same request identity: replay the recorded decision.
				confirmed = append(confirmed, contextdomain.InclusionDecisionResult{
					MemoryItemID: decision.MemoryItemID,
					Included:     included == 1,
				})
				continue
			case err == nil:
				// A different correlation under the same request: the
				// caller's new decision record — supersede below.
			case err == sql.ErrNoRows:
				// A fresh decision for this request/item pair.
			default:
				return mapVendorError(err)
			}
			if _, err := tx.Exec(`INSERT INTO context_inclusions
				(request_ref, memory_item_id, included, correlation_id, updated_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(request_ref, memory_item_id) DO UPDATE SET
					included = excluded.included, correlation_id = excluded.correlation_id,
					updated_at = excluded.updated_at`,
				intent.RequestRef, decision.MemoryItemID, boolToInt(decision.Included), intent.CorrelationID, now); err != nil {
				return mapVendorError(err)
			}
			confirmed = append(confirmed, contextdomain.InclusionDecisionResult{
				MemoryItemID: decision.MemoryItemID,
				Included:     decision.Included,
			})
		}
		outcome = contextdomain.InclusionOutcome{
			Result: contextdomain.InclusionResult{
				RequestRef: intent.RequestRef,
				Decisions:  confirmed,
			},
		}
		return nil
	})
	if err != nil {
		return contextdomain.InclusionOutcome{}, err
	}
	return outcome, nil
}

// LoadContextOperation resolves the family's authoritative state read (a
// pure query — never a retry): the unknown identity and the out-of-scope
// identity are the SAME typed CONTEXT_SNAPSHOT_NOT_FOUND 404 (the
// no-leak rule). The family's only durable-operation producer is the
// M-19 save command, which is CONDITIONAL on the unmet durable-storage
// decision — no operation row can exist while the gate stands, so the
// endpoint serves the typed 404 for every identity today; the machinery
// is the authoritative read for the day the condition resolves, never a
// fabricated record.
func (s *Store) LoadContextOperation(ctx stdcontext.Context, opID string) (contextdomain.Operation, error) {
	var kind, state, corr, sourceJobID, resultJSON, failureCode, failureReason, createdAt, updatedAt string
	row := s.readers.QueryRowContext(ctx, `
SELECT kind, state, correlation_id, source_job_id, result_json, failure_code, failure_reason, created_at, updated_at
FROM context_operations WHERE operation_id = ?`, opID)
	err := row.Scan(&kind, &state, &corr, &sourceJobID, &resultJSON, &failureCode, &failureReason, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return contextdomain.Operation{}, errStoragef(errs.CodeContextSnapshotNotFound, "no such context operation")
	}
	if err != nil {
		return contextdomain.Operation{}, mapVendorError(err)
	}
	op := contextdomain.Operation{
		OperationID:   opID,
		Kind:          kind,
		State:         state,
		CorrelationID: corr,
		SourceJobID:   sourceJobID,
		FailureCode:   failureCode,
		FailureReason: failureReason,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
	if resultJSON != "" {
		var result contextdomain.SavedSnapshotResult
		if jerr := json.Unmarshal([]byte(resultJSON), &result); jerr != nil {
			return contextdomain.Operation{}, errStoragef(errs.CodeStateCorrupt, "recorded result of operation %q does not parse", opID)
		}
		op.Result = &result
	}
	return op, nil
}
