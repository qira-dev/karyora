/**
 * The browser HTTP transport for the typed CoreClient boundary
 * (IMPL-2; Blueprint §31.2 "browser clients use the approved HTTP/API
 * transport").
 *
 * Consumes the Core's local HTTP transport adapter EXACTLY as delivered
 * in IMPL-1 (`internal/httplocal/server.go` at commit 79cc26c): the five
 * app operations, the journal replay read, and the C-1 session exchange.
 * The paths are that adapter's own — non-canonical (see wire.ts).
 *
 * Truthfulness rules this transport enforces:
 * - A network failure is the `unknown` outcome (P1.6 S4; MB §18): a lost
 *   response is NEVER proof of success, failure, or safe retry. No retry
 *   loop exists anywhere in this module — a retry is a new, explicit
 *   user action on the surface that owns it.
 * - A response that does not parse as its coded shape is `unknown` —
 *   never reinterpreted, never fabricated into data.
 * - The D-35 envelope (code/retryable/correlationId/data) is surfaced
 *   verbatim as `CoreRejection` — codes are stable, language-neutral
 *   machine identifiers; presentation copy resolves against locale
 *   resources upstream, with the stable code shown as the fallback.
 * - The bearer session token travels ONLY in the Authorization header —
 *   never in a URL, never in a Referer (R-3/R-11). Bootstrap credentials
 *   travel ONLY in the exchange POST body. `referrerPolicy: "no-referrer"`
 *   is the defense-in-depth belt on the Referer surface; the Core's
 *   boundary predicates remain the authoritative checks.
 * - Same-origin only: every call targets this page's own origin. How
 *   that origin reaches the loopback-bound Core is the serving story's
 *   concern (in development, the repo's Vite dev-server proxy) — this
 *   module hardcodes no host, no port, and no environment branch.
 */

import type { CoreRejection, CoreUnknown } from "../CoreClient";
import {
  discardHeldSecret,
  holdSession,
  heldSecret,
  type CoreSessionMetadata,
} from "./session";
import type {
  CoreWireEnvelope,
  WireAttempt,
  WireEventRecord,
  WireEventsResult,
  WireExchangeRequest,
  WireJob,
  WireJobsListParams,
  WireJobsListResult,
  WireReplayResult,
  WireSessionResult,
  WireStatusResult,
  WireSubmitRequest,
  WireSubmitResult,
} from "./wire";

/** The outcome of one transport call: typed data, the verbatim D-35
 *  envelope, or the truthful unknown (lost/uninterpretable response). */
export type TransportOutcome<TData> =
  | { readonly kind: "data"; readonly data: TData }
  | CoreRejection
  | CoreUnknown;

/** The exchange result: the held session's safe metadata on success. */
export type ExchangeOutcome = TransportOutcome<CoreSessionMetadata>;

/** The liveness probe result: whether a Core transport answered this
 *  page's origin, and (when it answered) the coded replay outcome it
 *  reported for the probe read. */
export type ProbeResult =
  | { readonly kind: "answered"; readonly outcome: string }
  | { readonly kind: "unreachable" };

/** The coded replay stream the journal's job events carry (the aggregate
 *  type used by the adapter's own tests and the events package). */
const JOB_STREAM = "job";

/** The coded operation-class scopes requested for a browser session: the
 *  full coded register (lifecycle + data), since the session serves both
 *  operation classes the adapter's surface spans. */
const REQUESTED_SCOPES: readonly string[] = ["data", "lifecycle"];

function isEnvelope(value: unknown): value is CoreWireEnvelope {
  if (value === null || typeof value !== "object") return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.code === "string" &&
    typeof candidate.retryable === "boolean" &&
    typeof candidate.correlationId === "string"
  );
}

function rejection(envelope: CoreWireEnvelope): CoreRejection {
  return { kind: "rejection", envelope };
}

function unknown(correlationId: string | undefined): CoreUnknown {
  return { kind: "unknown", correlationId };
}

/** Minimal coded-shape guards: a response must carry its distinguishing
 *  fields to be presented as data; anything else stays unknown. */
function isWireJob(value: unknown): value is WireJob {
  if (value === null || typeof value !== "object") return false;
  const job = value as Record<string, unknown>;
  return typeof job.job_id === "string" && typeof job.state === "string";
}

function isWireAttempt(value: unknown): value is WireAttempt {
  if (value === null || typeof value !== "object") return false;
  const attempt = value as Record<string, unknown>;
  return typeof attempt.attempt_id === "string" && typeof attempt.seq === "number";
}

function isWireEventList(value: unknown): value is readonly WireEventRecord[] {
  return Array.isArray(value);
}

function isWireReplayResult(value: unknown): value is WireReplayResult {
  if (value === null || typeof value !== "object") return false;
  const result = value as Record<string, unknown>;
  return typeof result.outcome === "string";
}

function isWireSessionResult(value: unknown): value is WireSessionResult {
  if (value === null || typeof value !== "object") return false;
  const session = value as Record<string, unknown>;
  return (
    typeof session.token === "string" &&
    typeof session.id === "string" &&
    Array.isArray(session.scopes) &&
    typeof session.expires === "string"
  );
}

/** One HTTP round trip: fetch, parse once, map honestly. Never retries. */
async function request(
  path: string,
  init: RequestInit | undefined,
): Promise<{ status: number; body: unknown; lost: boolean }> {
  try {
    const response = await fetch(path, init);
    let body: unknown = null;
    try {
      const text = await response.text();
      if (text !== "") body = JSON.parse(text);
    } catch {
      body = null; // a non-JSON body stays uninterpreted
    }
    return { status: response.status, body, lost: false };
  } catch {
    return { status: 0, body: null, lost: true };
  }
}

function headers(): Record<string, string> {
  const token = heldSecret();
  const base: Record<string, string> = { Accept: "application/json" };
  if (token !== null) {
    base.Authorization = `Bearer ${token}`;
  }
  return base;
}

async function call<TData>(
  path: string,
  init: RequestInit,
  guard: (body: unknown) => boolean,
): Promise<TransportOutcome<TData>> {
  const result = await request(path, init);
  if (result.lost) {
    return unknown(undefined);
  }
  if (isEnvelope(result.body)) {
    return rejection(result.body);
  }
  if (result.status >= 200 && result.status < 300 && guard(result.body)) {
    return { kind: "data", data: result.body as TData };
  }
  // A 2xx whose body is not the coded shape, or a non-2xx without the
  // envelope: the response cannot be interpreted — never fabricated.
  return unknown(undefined);
}

function post(body: string): RequestInit {
  return {
    method: "POST",
    headers: headers(),
    body,
    referrerPolicy: "no-referrer",
  };
}

function get(): RequestInit {
  return {
    method: "GET",
    headers: headers(),
    referrerPolicy: "no-referrer",
  };
}

/**
 * The browser transport over the Core's local HTTP adapter. One instance
 * per app; the session holder is module-scoped memory (session.ts).
 */
export function createHttpLocalTransport() {
  return {
    /** C-1 bootstrap/exchange (P-S3-01): consumes the one-time credential
     *  exactly once through the Core's `POST /v1/session/exchange`. On
     *  success the session is held in memory; the returned metadata is
     *  safe for display. A lost response is `unknown` and is NEVER
     *  treated as success or failure — the credential may or may not
     *  have been consumed, and only the Core knows. */
    async exchange(credential: string): Promise<ExchangeOutcome> {
      const body: WireExchangeRequest = { credential, scopes: REQUESTED_SCOPES };
      const result = await call<WireSessionResult>(
        "/v1/session/exchange",
        post(JSON.stringify(body)),
        isWireSessionResult,
      );
      if (result.kind === "data") {
        const metadata: CoreSessionMetadata = {
          id: result.data.id,
          scopes: result.data.scopes,
          expires: result.data.expires,
        };
        holdSession(result.data.token, metadata);
        return { kind: "data", data: metadata };
      }
      return result;
    },

    /** The liveness/session probe: one real replay read (`stream=job`,
     *  the coded aggregate type, limit 1). Outcomes:
     *  - unreachable: no transport answered this origin;
     *  - `forbidden`: answered, and the presented credential (if any)
     *    does not authorize the read — the decision port's coded denial
     *    outcome, by design never stranding a client;
     *  - any other coded outcome: answered, and the presented session
     *    authorized the read. */
    async probe(): Promise<ProbeResult> {
      const path = `/v1/events/replay?stream=${JOB_STREAM}&limit=1`;
      const result = await request(path, get());
      if (result.lost) return { kind: "unreachable" };
      if (isWireReplayResult(result.body)) {
        return { kind: "answered", outcome: result.body.outcome };
      }
      return { kind: "unreachable" };
    },

    /** `POST /v1/jobs` — the durable Job submission (202 = accepted for
     *  processing, never completion). */
    submit(payload: WireSubmitRequest): Promise<TransportOutcome<WireSubmitResult>> {
      return call<WireSubmitResult>("/v1/jobs", post(JSON.stringify(payload)), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        return typeof shaped.replayed === "boolean" && isWireJob(shaped.job);
      });
    },

    /** `GET /v1/jobs?limit=&status=&cursor=` — the Jobs list read (the
     *  card-9 working candidate's server-side cursor pagination as
     *  delivered in IMPL-4). Omitted parameters are not sent: the Core's
     *  per-surface default limit applies, no status filter selects all,
     *  and the cursorless first page is the list head. A response whose
     *  `cursor` is absent is the FINAL page — the cursorless page is the
     *  coded end marker; the client invents no has_more/total of its
     *  own. */
    list(params: WireJobsListParams = {}): Promise<TransportOutcome<WireJobsListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.status !== undefined && params.status !== "") query.set("status", params.status);
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireJobsListResult>(`/v1/jobs${suffix}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (!Array.isArray(shaped.jobs)) return false;
        // The cursor key is absent on the final page: absence is data
        // (the end marker), never an error.
        return shaped.cursor === undefined || typeof shaped.cursor === "string";
      });
    },

    /** `GET /v1/jobs/{id}` — the canonical Job status read. */
    status(jobID: string): Promise<TransportOutcome<WireStatusResult>> {
      return call<WireStatusResult>(`/v1/jobs/${encodeURIComponent(jobID)}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        return isWireJob(shaped.job) && isWireAttempt(shaped.attempt);
      });
    },

    /** `GET /v1/jobs/{id}/events` — the Job's event records. */
    events(jobID: string): Promise<TransportOutcome<WireEventsResult>> {
      return call<WireEventsResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/events`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          return isWireEventList((body as Record<string, unknown>).events);
        },
      );
    },

    /** `GET /v1/jobs/{id}/result` — the raw CAS result read. */
    result(jobID: string): Promise<TransportOutcome<unknown>> {
      return call<unknown>(
        `/v1/jobs/${encodeURIComponent(jobID)}/result`,
        get(),
        () => true, // the CAS bytes are the payload; their schema is the Job's own
      );
    },

    /** `POST /v1/jobs/{id}/cancel` — the cancellation intent (202;
     *  terminal-state cancels surface the coded CONFLICT_STATE). */
    cancel(jobID: string): Promise<TransportOutcome<Record<string, never>>> {
      return call<Record<string, never>>(
        `/v1/jobs/${encodeURIComponent(jobID)}/cancel`,
        post("{}"),
        (body) => body !== null && typeof body === "object",
      );
    },

    /** `GET /v1/events/replay` — the coded journal replay read with the
     *  explicit outcome set (ok / cursor_expired / replay_gap /
     *  forbidden). Consumers deduplicate by event_id and reconcile by
     *  sequence (C1-08); an expiry/gap directs authoritative refetch. */
    replay(stream: string, cursor: string, limit: number): Promise<TransportOutcome<WireReplayResult>> {
      const params = new URLSearchParams({ stream, limit: String(limit) });
      if (cursor !== "") params.set("cursor", cursor);
      return call<WireReplayResult>(`/v1/events/replay?${params.toString()}`, get(), isWireReplayResult);
    },

    /** Discards the held token from memory (the expiry/recovery path).
     *  Returns the safe metadata snapshot for the truthful display. */
    discardSession(): CoreSessionMetadata | null {
      return discardHeldSecret();
    },
  };
}

export type HttpLocalTransport = ReturnType<typeof createHttpLocalTransport>;
