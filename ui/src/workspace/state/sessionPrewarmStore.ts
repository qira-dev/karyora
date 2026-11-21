/**
 * Draft Session Prewarming & Generation-Counter Race Guard (`createSession`) (UIR-1.4, Blueprint §20.4, G-09).
 *
 * Opening a `New task` prewarms a draft session scoped to:
 * `(workspacePath, paneId, transportConnectionId, generationCounter)`.
 *
 * Switching projects, switching Local/Remote Core connections, or navigating to
 * another task increments `generationCounter` and invalidates any in-flight
 * prewarm response from an older generation so late network replies NEVER
 * overwrite the active workspace.
 */

export interface PrewarmContextScope {
  readonly workspacePath: string;
  readonly paneId: string;
  readonly transportConnectionId: string;
}

export interface PrewarmSessionTicket extends PrewarmContextScope {
  readonly generation: number;
  readonly startedAtMs: number;
}

export interface PrewarmedDraftSession extends PrewarmSessionTicket {
  readonly sessionId: string;
  readonly logEpoch: number;
}

export class SessionPrewarmStore {
  private generationCounter = 0;
  private activeScope: PrewarmContextScope | null = null;
  private activeSession: PrewarmedDraftSession | null = null;

  public getCurrentGeneration(): number {
    return this.generationCounter;
  }

  public getActiveSession(): PrewarmedDraftSession | null {
    return this.activeSession;
  }

  /**
   * Starts a new prewarm cycle, incrementing the generation counter and invalidating
   * any older in-flight prewarm tickets.
   */
  public beginPrewarm(scope: PrewarmContextScope): PrewarmSessionTicket {
    this.generationCounter += 1;
    this.activeScope = { ...scope };
    this.activeSession = null;

    return {
      ...scope,
      generation: this.generationCounter,
      startedAtMs: Date.now(),
    };
  }

  /**
   * Attempts to commit a completed prewarm response.
   * Returns `{ committed: false, reason: "STALE_GENERATION" | "SCOPE_MISMATCH" }`
   * if the user switched workspace, pane, or Core connection while the request was in flight.
   */
  public commitPrewarmResult(
    ticket: PrewarmSessionTicket,
    sessionId: string,
    initialLogEpoch = 0,
  ):
    | { readonly committed: true; readonly session: PrewarmedDraftSession }
    | {
        readonly committed: false;
        readonly reason: "STALE_GENERATION" | "SCOPE_MISMATCH";
      } {
    if (ticket.generation !== this.generationCounter) {
      return { committed: false, reason: "STALE_GENERATION" };
    }
    if (
      !this.activeScope ||
      this.activeScope.workspacePath !== ticket.workspacePath ||
      this.activeScope.paneId !== ticket.paneId ||
      this.activeScope.transportConnectionId !== ticket.transportConnectionId
    ) {
      return { committed: false, reason: "SCOPE_MISMATCH" };
    }

    const session: PrewarmedDraftSession = {
      ...ticket,
      sessionId,
      logEpoch: initialLogEpoch,
    };
    this.activeSession = session;
    return { committed: true, session };
  }

  /**
   * Explicitly invalidates the current generation (e.g., on connection switch or task change).
   */
  public invalidate(): number {
    this.generationCounter += 1;
    this.activeScope = null;
    this.activeSession = null;
    return this.generationCounter;
  }
}
