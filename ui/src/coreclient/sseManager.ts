/**
 * Monotonic SSE Event Cursor Manager & Cursor-Gap REST Snapshot Refetcher (UIR-1.1, Blueprint §21.2).
 *
 * Tracks monotonic event sequence numbers (`lastEventId` / `sequenceNumber`) per
 * stream topic (`session`, `job`, `notifications`). When an incoming event reveals
 * a sequence gap (or after a network drop / sleep-wake cycle / compacted cursor),
 * the manager triggers an authoritative REST snapshot refetch before applying
 * further incremental SSE deltas, preventing duplicate or missing tool rows.
 */

export type SseTopicKind = "session" | "job" | "notifications";

export interface SseStreamEvent<TPayload = unknown> {
  readonly eventId: string;
  readonly sequence: number;
  readonly topic: SseTopicKind;
  readonly entityId: string;
  readonly eventType: string;
  readonly payload: TPayload;
  readonly occurredAt: string;
}

export interface SseCursorState {
  readonly topic: SseTopicKind;
  readonly entityId: string;
  readonly lastEventId: string | null;
  readonly lastSequence: number;
  readonly status: "idle" | "streaming" | "reconnecting" | "refetching-gap" | "closed";
  readonly gapRefetchCount: number;
}

export interface SseSubscriptionHandlers<TPayload = unknown, TSnapshot = unknown> {
  readonly onEvent: (event: SseStreamEvent<TPayload>) => void;
  readonly onGapRefetchRequired: (
    cursor: SseCursorState,
    incomingSequence: number,
  ) => Promise<TSnapshot>;
  readonly onSnapshotReconciled?: (snapshot: TSnapshot, newSequence: number) => void;
  readonly onStatusChange?: (state: SseCursorState) => void;
}

export class SseStreamManager {
  private readonly cursors = new Map<string, SseCursorState>();

  private key(topic: SseTopicKind, entityId: string): string {
    return `${topic}:${entityId}`;
  }

  public getCursorState(topic: SseTopicKind, entityId: string): SseCursorState {
    const k = this.key(topic, entityId);
    const existing = this.cursors.get(k);
    if (existing) {
      return existing;
    }
    const initial: SseCursorState = {
      topic,
      entityId,
      lastEventId: null,
      lastSequence: 0,
      status: "idle",
      gapRefetchCount: 0,
    };
    this.cursors.set(k, initial);
    return initial;
  }

  public resetAllCursors(): void {
    this.cursors.clear();
  }

  /**
   * Processes an incoming SSE event.
   * - Duplicate/older sequence numbers (`event.sequence <= lastSequence`) are ignored idempotently.
   * - Contiguous sequence numbers (`event.sequence === lastSequence + 1` or first event) are applied immediately.
   * - A sequence gap (`event.sequence > lastSequence + 1` when `lastSequence > 0`) triggers
   *   `onGapRefetchRequired` to perform an authoritative REST snapshot refetch before advancing the cursor.
   */
  public async processIncomingEvent<TPayload, TSnapshot>(
    event: SseStreamEvent<TPayload>,
    handlers: SseSubscriptionHandlers<TPayload, TSnapshot>,
  ): Promise<{
    readonly disposition: "applied" | "duplicate-ignored" | "gap-refetched";
    readonly cursor: SseCursorState;
  }> {
    const k = this.key(event.topic, event.entityId);
    const current = this.getCursorState(event.topic, event.entityId);

    // 1. Idempotent duplicate protection
    if (current.lastSequence > 0 && event.sequence <= current.lastSequence) {
      return { disposition: "duplicate-ignored", cursor: current };
    }

    // 2. Cursor-gap detection (Blueprint §21.2)
    if (current.lastSequence > 0 && event.sequence > current.lastSequence + 1) {
      const refetchingState: SseCursorState = {
        ...current,
        status: "refetching-gap",
        gapRefetchCount: current.gapRefetchCount + 1,
      };
      this.cursors.set(k, refetchingState);
      handlers.onStatusChange?.(refetchingState);

      const snapshot = await handlers.onGapRefetchRequired(
        refetchingState,
        event.sequence,
      );
      handlers.onSnapshotReconciled?.(snapshot, event.sequence);

      const reconciledState: SseCursorState = {
        ...refetchingState,
        lastEventId: event.eventId,
        lastSequence: event.sequence,
        status: "streaming",
      };
      this.cursors.set(k, reconciledState);
      handlers.onStatusChange?.(reconciledState);
      return { disposition: "gap-refetched", cursor: reconciledState };
    }

    // 3. Contiguous event application
    handlers.onEvent(event);
    const updated: SseCursorState = {
      ...current,
      lastEventId: event.eventId,
      lastSequence: event.sequence,
      status: "streaming",
    };
    this.cursors.set(k, updated);
    handlers.onStatusChange?.(updated);
    return { disposition: "applied", cursor: updated };
  }
}
