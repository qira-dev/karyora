/**
 * Serial Prompt Submission Queue & Auto-Drain Lifecycle (`Queue (N)`) (UIR-1.4, Blueprint §7.3, §20.4, G-09).
 *
 * Every prompt submission passes through a serial per-session submission queue:
 * - When a turn is already running and `interactionBehavior === "queue"` (Screenshots #24–#25),
 *   additional prompts enter the visible Queue Drawer above the composer (`Queue (N)`).
 * - Queued items support:
 *   1. Drag-and-drop reorder (`reorder`)
 *   2. Inline edit (`editItem`)
 *   3. Immediate steer/interrupt (`promoteToSendNow`)
 *   4. Delete (`removeItem`)
 * - When the active turn completes with authoritative `S2`, the queue automatically drains
 *   and dispatches the next queued item (`onTurnCompletedS2`).
 * - When the active turn fails (`S3`), enters unknown outcome (`S4`), or pauses for human
 *   tool/Job approval (`pendingApproval`), auto-drain pauses and preserves the entire queue
 *   so the user can inspect, edit, or resume safely.
 */

import type { ComposerContextChip } from "./stateLifetimes";

export interface QueuedPromptItem {
  readonly queueItemId: string;
  readonly sessionId: string;
  readonly promptText: string;
  readonly chips: readonly ComposerContextChip[];
  readonly mode: "default" | "plan" | "auto";
  readonly modelId: string;
  readonly thoughtLevel: "low" | "medium" | "high";
  readonly idempotencyKey: string;
  readonly enqueuedAtMs: number;
}

export type SessionTurnExecutionStatus =
  | "idle"
  | "running"
  | "paused-pending-approval"
  | "paused-error-s3"
  | "paused-unknown-s4";

export interface SessionQueueSnapshot {
  readonly sessionId: string;
  readonly turnStatus: SessionTurnExecutionStatus;
  readonly activeItem: QueuedPromptItem | null;
  readonly queuedItems: readonly QueuedPromptItem[];
  readonly autoDrainPaused: boolean;
}

export class SubmissionQueueStore {
  private readonly sessions = new Map<string, SessionQueueSnapshot>();

  public getSnapshot(sessionId: string): SessionQueueSnapshot {
    const existing = this.sessions.get(sessionId);
    if (existing) {
      return existing;
    }
    const initial: SessionQueueSnapshot = {
      sessionId,
      turnStatus: "idle",
      activeItem: null,
      queuedItems: [],
      autoDrainPaused: false,
    };
    this.sessions.set(sessionId, initial);
    return initial;
  }

  /**
   * Submits or enqueues a prompt item.
   * - If the session is `idle` and auto-drain is not paused, the item immediately becomes
   *   `activeItem` (`turnStatus: "running"`, `dispatchedImmediately: true`).
   * - Otherwise, the item is appended to `queuedItems` (`dispatchedImmediately: false`).
   */
  public submitOrEnqueue(item: QueuedPromptItem): {
    readonly dispatchedImmediately: boolean;
    readonly snapshot: SessionQueueSnapshot;
  } {
    const current = this.getSnapshot(item.sessionId);

    if (current.turnStatus === "idle" && !current.autoDrainPaused) {
      const next: SessionQueueSnapshot = {
        ...current,
        turnStatus: "running",
        activeItem: item,
      };
      this.sessions.set(item.sessionId, next);
      return { dispatchedImmediately: true, snapshot: next };
    }

    const next: SessionQueueSnapshot = {
      ...current,
      queuedItems: [...current.queuedItems, item],
    };
    this.sessions.set(item.sessionId, next);
    return { dispatchedImmediately: false, snapshot: next };
  }

  /**
   * Drag-and-drop reorder inside the Queue Drawer (`fromIndex` -> `toIndex`).
   */
  public reorder(
    sessionId: string,
    fromIndex: number,
    toIndex: number,
  ): SessionQueueSnapshot {
    const current = this.getSnapshot(sessionId);
    const list = [...current.queuedItems];
    if (
      fromIndex < 0 ||
      fromIndex >= list.length ||
      toIndex < 0 ||
      toIndex >= list.length ||
      fromIndex === toIndex
    ) {
      return current;
    }
    const [moved] = list.splice(fromIndex, 1);
    list.splice(toIndex, 0, moved);
    const next: SessionQueueSnapshot = {
      ...current,
      queuedItems: list,
    };
    this.sessions.set(sessionId, next);
    return next;
  }

  /**
   * Inline edit of a queued prompt item's text or context chips.
   */
  public editItem(
    sessionId: string,
    queueItemId: string,
    patch: Partial<Pick<QueuedPromptItem, "promptText" | "chips" | "mode" | "modelId" | "thoughtLevel">>,
  ): SessionQueueSnapshot {
    const current = this.getSnapshot(sessionId);
    const next: SessionQueueSnapshot = {
      ...current,
      queuedItems: current.queuedItems.map((item) =>
        item.queueItemId === queueItemId ? { ...item, ...patch } : item,
      ),
    };
    this.sessions.set(sessionId, next);
    return next;
  }

  /**
   * Removes an item from the queue.
   */
  public removeItem(
    sessionId: string,
    queueItemId: string,
  ): SessionQueueSnapshot {
    const current = this.getSnapshot(sessionId);
    const next: SessionQueueSnapshot = {
      ...current,
      queuedItems: current.queuedItems.filter(
        (item) => item.queueItemId !== queueItemId,
      ),
    };
    this.sessions.set(sessionId, next);
    return next;
  }

  /**
   * Promotes a queued item to the front and dispatches it immediately (steer/send-now).
   */
  public promoteToSendNow(
    sessionId: string,
    queueItemId: string,
  ): {
    readonly promotedItem: QueuedPromptItem | null;
    readonly snapshot: SessionQueueSnapshot;
  } {
    const current = this.getSnapshot(sessionId);
    const target = current.queuedItems.find(
      (item) => item.queueItemId === queueItemId,
    );
    if (!target) {
      return { promotedItem: null, snapshot: current };
    }
    const remaining = current.queuedItems.filter(
      (item) => item.queueItemId !== queueItemId,
    );
    const next: SessionQueueSnapshot = {
      ...current,
      turnStatus: "running",
      activeItem: target,
      queuedItems: remaining,
      autoDrainPaused: false,
    };
    this.sessions.set(sessionId, next);
    return { promotedItem: target, snapshot: next };
  }

  /**
   * Called when the active turn completes with authoritative S2 success.
   * Automatically pops and dispatches the next queued item if `autoDrainPaused === false`.
   */
  public onTurnCompletedS2(sessionId: string): {
    readonly nextDispatchedItem: QueuedPromptItem | null;
    readonly snapshot: SessionQueueSnapshot;
  } {
    const current = this.getSnapshot(sessionId);
    if (current.autoDrainPaused || current.queuedItems.length === 0) {
      const next: SessionQueueSnapshot = {
        ...current,
        turnStatus: "idle",
        activeItem: null,
      };
      this.sessions.set(sessionId, next);
      return { nextDispatchedItem: null, snapshot: next };
    }

    const [nextItem, ...rest] = current.queuedItems;
    const next: SessionQueueSnapshot = {
      ...current,
      turnStatus: "running",
      activeItem: nextItem,
      queuedItems: rest,
    };
    this.sessions.set(sessionId, next);
    return { nextDispatchedItem: nextItem, snapshot: next };
  }

  /**
   * Called when the active turn pauses for `pendingApproval`, fails (`S3`), or enters `S4`.
   * Pauses auto-drain and preserves all queued items untouched (Blueprint §20.4).
   */
  public onTurnPausedOrFailed(
    sessionId: string,
    reason: "paused-pending-approval" | "paused-error-s3" | "paused-unknown-s4",
  ): SessionQueueSnapshot {
    const current = this.getSnapshot(sessionId);
    const next: SessionQueueSnapshot = {
      ...current,
      turnStatus: reason,
      autoDrainPaused: true,
    };
    this.sessions.set(sessionId, next);
    return next;
  }
}
