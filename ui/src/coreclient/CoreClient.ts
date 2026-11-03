/**
 * CoreClient contract layer (UI Phase 2, P2.6 shell; P2.7 first-run and
 * connection onboarding).
 *
 * Construction authority: UI Blueprint §31.2 and the P1.5 wire-contract
 * specification. What is implemented here is the typed boundary — the
 * interface shape (queries and commands), the D-35 error envelope, and
 * the unavailable state — consumed by the shell's truthful status
 * surfaces (C-02/C-04/C-11) and the P2.7 onboarding surfaces
 * (C-06/C-07/C-08/C-09/C-10).
 *
 * What is deliberately NOT here (carried Open, never invented):
 * - No exact wire names, paths, or per-operation payload schemas. Every
 *   exact wire name is Open with the Core/API owner (P2.1 §2.0 O-1); the
 *   `0.1.0-eval` contract surface in `contracts/` is explicitly
 *   non-canonical.
 * - No transport implementation. The browser HTTP/event transport and
 *   the desktop transport adapter await their recorded approvals
 *   (P1.5 §2.2 verified-vs-open split; P2.1 §2.0 O-4). This milestone
 *   wires no live Core.
 * - No fabricated state: an operation whose source is not connected
 *   reports `unavailable`, and the shell renders Unknown/unavailable —
 *   never a connected, populated, or successful state.
 */

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

/** Object-level operation kinds the Phase 2 shell consumes. These are UI
 *  interface names at the P1.5 §3 object/authority level — not wire
 *  names. Each is marked Open where its exact wire semantics are Open. */
export type CoreOperationKind =
  /** The active connection's Core-reported connectivity state
   *  (C-02; the six Core-reported states). Wire name: Open (O-1). */
  | "connection-state"
  /** The Core connection records with safe display labels
   *  (C-10/C-14; SEL-13). Wire semantics: Open (O-1/O-2/O-6). */
  | "connection-list"
  /** The session/permission state projection (C-11).
   *  Wire name: Open (O-1/O-3). */
  | "session-state"
  /** The notification records list (C-04; IMPL-14 wires the surface to
   *  the delivered Core notification machinery over the adapter's own
   *  non-canonical paths). The CANONICAL wire semantics stay Open
   *  (O-2/O-6) — the adapter's paths bind nothing. */
  | "notification-list"
  /** Permission-filtered server-side search (C-03; the full palette is
   *  a later package). Wire semantics: Open (O-1/O-2). */
  | "search";

/** The result of a query the shell makes. `unavailable` is the truthful
 *  state of this milestone: the transport layer is not configured, so
 *  the operation has no source. The shell renders it as
 *  Unknown/unavailable and never as a populated or connected state. */
export type CoreQueryResult<TData> =
  | { readonly kind: "data"; readonly data: TData }
  | { readonly kind: "unavailable" }
  | CoreRejection
  | CoreUnknown;

/** Command-level operation kinds the P2.7 first-run and connection
 *  surfaces consume. These are UI interface names at the P1.5 §3
 *  object/authority level — not wire names. Each maps to a recorded P1.1
 *  mutation or contract operation, and its exact wire name stays Open
 *  (O-1). No command here may be implemented client-side: every one is
 *  Core-owned durable authority. */
export type CoreCommandKind =
  /** `Test connection` (M-25; C-07): read-only endpoint/model/capability
   *  verification with a sanitized result. Wire name: Open (O-1). */
  | "provider-connection-test"
  /** `Save securely` (M-24; C-07): store provider metadata + the secret
   *  reference through the Core credential boundary. Wire name: Open. */
  | "provider-connection-save"
  /** Connection rename (C-08/M-27): safe display label only. */
  | "connection-rename"
  /** Connection remove (C-08/M-27): destructive; §8.9 confirmation binds. */
  | "connection-remove"
  /** Connection switch (M-34; C-09): a Core-mediated context change that
   *  never migrates a Job and never mixes caches/cursors/credentials. */
  | "connection-switch"
  /** Session re-authentication (C-11): the dedicated re-auth flow.
   *  Mechanism: Open (O-3 — Core/security owners). */
  | "session-reauthenticate"
  /** First-run setup completion (C-06): recorded ONLY after
   *  authoritative Core success — never invented client-side. */
  | "first-run-complete";

/** The result of a command attempt. `unavailable` is the truthful state
 *  of this milestone: no transport is configured, so the command was
 *  NEVER SUBMITTED — no correlation ID exists, nothing reached a Core,
 *  and no retry is owed (this is S0-with-reason in the P1.6 §2 model,
 *  not S1, S3, or S4). `accepted` is the future S1 surface for a live
 *  transport; no code path can produce it today. */
export type CoreCommandResult =
  | { readonly kind: "accepted"; readonly correlationId: string }
  | { readonly kind: "unavailable" }
  | CoreRejection
  | CoreUnknown;

/** The typed client boundary (Blueprint §31.2). Phase 2 provides the
 *  interface and the unavailable instance; live transports arrive only
 *  through their recorded approval path. */
export interface CoreClient {
  /** Run one object-level query. */
  query<TData>(operation: CoreOperationKind): Promise<CoreQueryResult<TData>>;
  /** Attempt one Core-owned command. */
  command(operation: CoreCommandKind): Promise<CoreCommandResult>;
}

/** The single Phase 2 instance: every query and command reports
 *  `unavailable`. This is the honest no-transport state — not a mock,
 *  not a fake success, not fabricated data. */
export function createUnavailableCoreClient(): CoreClient {
  return {
    query<TData>(_operation: CoreOperationKind): Promise<CoreQueryResult<TData>> {
      return Promise.resolve({ kind: "unavailable" });
    },
    command(_operation: CoreCommandKind): Promise<CoreCommandResult> {
      return Promise.resolve({ kind: "unavailable" });
    },
  };
}
