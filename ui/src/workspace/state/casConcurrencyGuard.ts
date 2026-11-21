/**
 * Compare-And-Swap (`CAS`) Optimistic Concurrency & Transcript Log-Epoch Guard
 * (UIR-1.4, Blueprint §20.4, Screenshot #85, G-09).
 *
 * Enforces two strict concurrency laws:
 * 1. Configuration / Entity Mutation CAS (`baseRevision` / `ETag`):
 *    Every edit to a Workflow graph, Provider config, Subagent/MCP/Hook definition,
 *    Memory item, or Personalization instructions sends `baseRevision`. On a `409 Conflict`
 *    (`CONFLICT_STATE` / `CONFIG_CONFLICT`), the guard preserves the user's unsaved draft
 *    side-by-side with the newer server state instead of silently overwriting either side.
 * 2. Transcript Branch / Edit Log-Epoch Guard (`targetTurnId` + `baseLogEpoch`):
 *    Every `Fork from here` or `Edit message` action (Screenshot #85) validates that the
 *    session's authoritative `logEpoch` has not mutated since the user opened the action,
 *    preventing truncation or branching against a stale transcript history.
 */

export interface CasVersionedEntity<TData> {
  readonly entityId: string;
  readonly revision: string;
  readonly data: TData;
}

export type CasMutationResult<TData> =
  | {
      readonly status: "applied";
      readonly entity: CasVersionedEntity<TData>;
    }
  | {
      readonly status: "conflict-preserved";
      readonly entityId: string;
      readonly attemptedBaseRevision: string;
      readonly unsavedClientDraft: TData;
      readonly latestServerEntity: CasVersionedEntity<TData>;
    };

export interface TranscriptBranchRequest {
  readonly sessionId: string;
  readonly action: "fork-from-here" | "edit-message";
  readonly targetTurnId: string;
  readonly baseLogEpoch: number;
}

export type TranscriptEpochValidationResult =
  | {
      readonly valid: true;
      readonly nextLogEpoch: number;
    }
  | {
      readonly valid: false;
      readonly reason: "STALE_LOG_EPOCH" | "MISSING_TARGET_TURN";
      readonly expectedLogEpoch: number;
      readonly actualLogEpoch: number;
    };

export class CasConcurrencyGuard {
  /**
   * Evaluates a Compare-And-Swap mutation against the authoritative server revision.
   * On revision mismatch (`409 Conflict`), preserves both `unsavedClientDraft` and
   * `latestServerEntity` for side-by-side resolution (Blueprint §20.4).
   */
  public evaluateCasMutation<TData>(
    attemptedBaseRevision: string,
    unsavedClientDraft: TData,
    latestServerEntity: CasVersionedEntity<TData>,
    nextRevisionOnSuccess: string,
  ): CasMutationResult<TData> {
    if (attemptedBaseRevision !== latestServerEntity.revision) {
      return {
        status: "conflict-preserved",
        entityId: latestServerEntity.entityId,
        attemptedBaseRevision,
        unsavedClientDraft,
        latestServerEntity,
      };
    }

    return {
      status: "applied",
      entity: {
        entityId: latestServerEntity.entityId,
        revision: nextRevisionOnSuccess,
        data: unsavedClientDraft,
      },
    };
  }

  /**
   * Validates that a conversation `Fork from here` or `Edit message` request
   * matches the current authoritative transcript `logEpoch` and contains `targetTurnId`.
   */
  public validateTranscriptBranch(
    request: TranscriptBranchRequest,
    authoritativeLogEpoch: number,
    existingTurnIds: ReadonlySet<string>,
  ): TranscriptEpochValidationResult {
    if (request.baseLogEpoch !== authoritativeLogEpoch) {
      return {
        valid: false,
        reason: "STALE_LOG_EPOCH",
        expectedLogEpoch: request.baseLogEpoch,
        actualLogEpoch: authoritativeLogEpoch,
      };
    }
    if (!existingTurnIds.has(request.targetTurnId)) {
      return {
        valid: false,
        reason: "MISSING_TARGET_TURN",
        expectedLogEpoch: request.baseLogEpoch,
        actualLogEpoch: authoritativeLogEpoch,
      };
    }
    return {
      valid: true,
      nextLogEpoch: authoritativeLogEpoch + 1,
    };
  }
}
