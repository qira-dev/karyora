/**
 * Unified v3.0 CoreClient Contract & Implementation Layer (UIR-1.1, UI-26, G-02, G-08).
 *
 * Wraps the Go Core local HTTP transport adapter (`./httpLocal/transport.ts`),
 * the RFC 9457 / D-35 machine error envelope mapper (`./rfc9457.ts`), and the
 * monotonic SSE cursor-gap refetch manager (`./sseManager.ts`) into one
 * authoritative client contract across Web Browser, Windows, macOS, and Linux.
 */

import {
  createHttpLocalTransport,
  type HttpLocalTransport,
  type TransportOutcome,
} from "./httpLocal/transport";
import {
  toRecoveryPanelDescriptor,
  toUnknownOutcomeS4RecoveryDescriptor,
} from "./rfc9457";
import { SseStreamManager } from "./sseManager";
import type { CommandState, CoreBootstrapStatus } from "./types";

/** The D-35 machine error envelope (P1.5 §2.4): stable namespaced `code`,
 *  `retryable`, server-generated `correlationId`, bounded typed `data`.
 *  No prose travels in the envelope; the UI resolves localized copy from
 *  `code`/`data` against locale resources. */
export interface CoreErrorEnvelope {
  readonly code: string;
  readonly retryable: boolean;
  readonly correlationId: string;
  readonly data: Readonly<Record<string, unknown>>;
}

/** The typed denial surface (P1.6 outcome S3). */
export type CoreRejection = {
  readonly kind: "rejection";
  readonly envelope: CoreErrorEnvelope;
};

/** The unknown-outcome surface (P1.6 outcome S4): a lost response is
 *  never proof of success, failure, or safe retry (MB §18). */
export type CoreUnknown = {
  readonly kind: "unknown";
  readonly correlationId: string | undefined;
};

export type CoreOperationKind =
  | "connection-state"
  | "connection-list"
  | "session-state"
  | "notification-list"
  | "search";

export type CoreQueryResult<TData> =
  | { readonly kind: "data"; readonly data: TData }
  | { readonly kind: "unavailable" }
  | CoreRejection
  | CoreUnknown;

export type CoreCommandKind =
  | "provider-connection-test"
  | "provider-connection-save"
  | "connection-rename"
  | "connection-remove"
  | "connection-switch"
  | "session-reauthenticate"
  | "first-run-complete";

export type CoreCommandResult =
  | { readonly kind: "accepted"; readonly correlationId: string }
  | { readonly kind: "unavailable" }
  | CoreRejection
  | CoreUnknown;

export interface CoreClient {
  query<TData = never>(operation: CoreOperationKind): Promise<CoreQueryResult<TData>>;
  command(
    operation: CoreCommandKind,
    payload?: Readonly<Record<string, unknown>>,
  ): Promise<CoreCommandResult>;
}

export function createUnavailableCoreClient(): CoreClient {
  return {
    async query<TData = never>(
      _operation: CoreOperationKind,
    ): Promise<CoreQueryResult<TData>> {
      return { kind: "unavailable" };
    },
    async command(
      _operation: CoreCommandKind,
      _payload?: Readonly<Record<string, unknown>>,
    ): Promise<CoreCommandResult> {
      return { kind: "unavailable" };
    },
  };
}

let idempotencyCounter = 0;

/**
 * Generates a deterministic-format unique Idempotency-Key for mutating operations.
 */
export function createIdempotencyKey(prefix = "ui-cmd"): string {
  idempotencyCounter += 1;
  const randomPart =
    typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID()
      : `${Date.now().toString(36)}-${idempotencyCounter.toString(36)}`;
  return `${prefix}-${randomPart}`;
}

/**
 * Converts a low-level `TransportOutcome<T>` from `httpLocal` into the canonical
 * 5-outcome `CommandState<T>` (`S0`–`S4`, Blueprint §6.2).
 */
export function transportOutcomeToCommandState<T>(
  outcome: TransportOutcome<T>,
  idempotencyKey?: string,
): CommandState<T> {
  if (outcome.kind === "data") {
    return {
      outcome: "S2",
      data: outcome.data,
    };
  }
  if (outcome.kind === "rejection") {
    return {
      outcome: "S3",
      envelope: outcome.envelope,
      recovery: toRecoveryPanelDescriptor(outcome.envelope),
    };
  }
  return {
    outcome: "S4",
    correlationId: outcome.correlationId,
    idempotencyKey,
    recovery: toUnknownOutcomeS4RecoveryDescriptor(
      outcome.correlationId ?? "unreported",
    ),
  };
}

export interface V3CoreClient {
  readonly transport: HttpLocalTransport;
  readonly sse: SseStreamManager;
  probeBootstrapStatus(): Promise<CoreBootstrapStatus>;
  createIdempotencyKey(prefix?: string): string;
}

export function createV3CoreClient(
  customTransport?: HttpLocalTransport,
): V3CoreClient {
  const transport = customTransport ?? createHttpLocalTransport();
  const sse = new SseStreamManager();

  return {
    transport,
    sse,
    createIdempotencyKey,
    async probeBootstrapStatus(): Promise<CoreBootstrapStatus> {
      const probeResult = await transport.probe();
      if (probeResult.kind === "answered") {
        if (probeResult.outcome === "forbidden") {
          return {
            connectionId: "local-core",
            mode: "local",
            status: "auth-required",
            coreVersion: "0.1.0",
            sessionAuthenticated: false,
          };
        }
        return {
          connectionId: "local-core",
          mode: "local",
          status: "connected",
          coreVersion: "0.1.0",
          sessionAuthenticated: true,
        };
      }
      return {
        connectionId: "local-core",
        mode: "local",
        status: "offline",
        coreVersion: "0.1.0",
        sessionAuthenticated: false,
      };
    },
  };
}
