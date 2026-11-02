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
  WireApprovalDecisionRequest,
  WireApprovalDecisionResult,
  WireApprovalReadResult,
  WireAttempt,
  WireConnectionGetResult,
  WireConnectionListResult,
  WireConnectionRotateResult,
  WireConnectionSaveRequest,
  WireConnectionSaveResult,
  WireConnectionSwitchResult,
  WireConnectionTestRequest,
  WireConnectionTestResult,
  WireConnectionView,
  WireConversation,
  WireConversationCreateRequest,
  WireConversationExchange,
  WireConversationExchangeResult,
  WireConversationListResult,
  WireConversationMessage,
  WireConversationMessagesResult,
  WireConversationResult,
  WireConversationsListParams,
  WireConversationSendRequest,
  WireEventRecord,
  WireEventsResult,
  WireExchangeRequest,
  WireJob,
  WireJobsListParams,
  WireJobsListResult,
  WireJobSnapshotResult,
  WireReplayResult,
  WireReviewRequest,
  WireReviewResult,
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

/** The coded connection-view guard: the record's distinguishing fields +
 *  the masked status + the tested-state truth + the active flag. */
function isWireConnectionView(value: unknown): value is WireConnectionView {
  if (value === null || typeof value !== "object") return false;
  const view = value as Record<string, unknown>;
  if (view.record === null || typeof view.record !== "object") return false;
  const record = view.record as Record<string, unknown>;
  return (
    typeof record.connection_id === "string" &&
    typeof record.provider === "string" &&
    typeof record.model_id === "string" &&
    typeof record.secret_ref === "string" &&
    typeof view.secret_status === "string" &&
    typeof view.test_current === "boolean" &&
    typeof view.active === "boolean"
  );
}

function isWireConnectionRotateResult(value: unknown): value is WireConnectionRotateResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (shaped.record === null || typeof shaped.record !== "object") return false;
  const record = shaped.record as Record<string, unknown>;
  return (
    typeof record.connection_id === "string" &&
    Array.isArray(shaped.affected_models) &&
    typeof shaped.routing_note === "string" &&
    typeof shaped.interruption_note === "string"
  );
}

/** The rotate/re-authenticate request-body shape (the wire type lives in
 *  wire.ts; this alias keeps the two methods' bodies uniform). */
type WireConnectionRotateRequestShape = { readonly secret: string };

/** The coded conversation guards: each response must carry its
 *  distinguishing coded fields to be presented as data; anything else
 *  stays unknown — never reinterpreted, never fabricated. */
function isWireConversation(value: unknown): value is WireConversation {
  if (value === null || typeof value !== "object") return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.conversation_id === "string" &&
    typeof record.title === "string" &&
    typeof record.created_at === "string" &&
    typeof record.updated_at === "string"
  );
}

function isWireConversationMessage(value: unknown): value is WireConversationMessage {
  if (value === null || typeof value !== "object") return false;
  const message = value as Record<string, unknown>;
  return (
    typeof message.message_id === "string" &&
    typeof message.conversation_id === "string" &&
    typeof message.seq === "number" &&
    typeof message.role === "string" &&
    typeof message.content === "string" &&
    typeof message.created_at === "string"
  );
}

function isWireConversationExchange(value: unknown): value is WireConversationExchange {
  if (value === null || typeof value !== "object") return false;
  const exchange = value as Record<string, unknown>;
  return (
    typeof exchange.correlation_id === "string" &&
    typeof exchange.conversation_id === "string" &&
    typeof exchange.user_message_id === "string" &&
    typeof exchange.state === "string" &&
    typeof exchange.created_at === "string"
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

    /** `POST /v1/jobs/review` — the pre-submit review read (the owner's
     *  card-13 working candidate). It is a READ despite the POST class:
     *  the draft travels in a body (a GET URL cannot carry a large task),
     *  and the Core creates nothing — the eight review fields project
     *  what its own gates determine. The route rides the same POST
     *  boundary posture as every mutation (same-origin carve-out with a
     *  validated session), never weakening it. */
    review(payload: WireReviewRequest): Promise<TransportOutcome<WireReviewResult>> {
      return call<WireReviewResult>("/v1/jobs/review", post(JSON.stringify(payload)), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (shaped.review === null || typeof shaped.review !== "object") return false;
        const review = shaped.review as Record<string, unknown>;
        return (
          review.intent !== null &&
          typeof review.intent === "object" &&
          review.sources !== null &&
          typeof review.sources === "object" &&
          review.provider_model !== null &&
          typeof review.provider_model === "object" &&
          Array.isArray(review.tool_effects) &&
          review.permission !== null &&
          typeof review.permission === "object" &&
          review.policy !== null &&
          typeof review.policy === "object" &&
          review.approval_need !== null &&
          typeof review.approval_need === "object" &&
          review.budget !== null &&
          typeof review.budget === "object"
        );
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

    /** `GET /v1/jobs/{id}/snapshot` — the context-snapshot read (the
     *  card-11 working candidate): one stored snapshot's STRUCTURE per
     *  the P3.2 §3.5 Supplied semantics. The optional `attempt` selects a
     *  specific attempt's snapshot; its absence resolves the Job's
     *  current attempt. A Core NOT_FOUND (no stored snapshot for the
     *  attempt) is the typed refusal surfaced verbatim — never
     *  reinterpreted as an empty structure. */
    jobSnapshot(jobID: string, attempt?: number): Promise<TransportOutcome<WireJobSnapshotResult>> {
      const query = new URLSearchParams();
      if (attempt !== undefined) query.set("attempt", String(attempt));
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireJobSnapshotResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/snapshot${suffix}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (shaped.snapshot === null || typeof shaped.snapshot !== "object") return false;
          const snapshot = shaped.snapshot as Record<string, unknown>;
          return (
            typeof snapshot.snapshot_id === "string" &&
            typeof snapshot.task_ref === "string" &&
            typeof snapshot.scope === "string" &&
            typeof snapshot.created_at === "string" &&
            Array.isArray(snapshot.included) &&
            Array.isArray(snapshot.omitted) &&
            snapshot.budget !== null &&
            typeof snapshot.budget === "object" &&
            typeof snapshot.exact_vs_approximate === "string"
          );
        },
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

    /** `GET /v1/jobs/{id}/approval` — the Job's raised approval request
     *  (the card-14 working candidate's queryable identity). The read
     *  mutates nothing: a notification read/dismiss never implies
     *  approval. A Core NOT_FOUND (no approval record for this job) is
     *  the typed refusal surfaced verbatim — never reinterpreted. */
    jobApproval(jobID: string): Promise<TransportOutcome<WireApprovalReadResult>> {
      return call<WireApprovalReadResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/approval`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (shaped.approval === null || typeof shaped.approval !== "object") return false;
          const approval = shaped.approval as Record<string, unknown>;
          return (
            typeof approval.approval_id === "string" &&
            typeof approval.job_id === "string" &&
            typeof approval.state === "string" &&
            typeof approval.created_at === "string"
          );
        },
      );
    },

    /** `POST /v1/jobs/{id}/approvals/{decision}` — one approval decision
     *  command (the card-14 working candidate): the Core-issued approval
     *  identity rides the body; the 200 response is the Core-CONFIRMED
     *  post-decision state (the transition is durable when it returns) —
     *  still rendered only from the authoritative re-read. The `replayed`
     *  flag reports the honest replay: the same decision re-presented
     *  returns the recorded outcome without re-applying. A lost response
     *  is `unknown` — NEVER proof the decision was recorded or not; the
     *  Open-class reconciliation is a QUERY by the approval identity, so
     *  this method never retries on its own. */
    decideApproval(
      jobID: string,
      decision: "accept" | "reject",
      approvalID: string,
    ): Promise<TransportOutcome<WireApprovalDecisionResult>> {
      const body: WireApprovalDecisionRequest = { approval_id: approvalID };
      return call<WireApprovalDecisionResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/approvals/${decision}`,
        post(JSON.stringify(body)),
        (result) => {
          if (result === null || typeof result !== "object") return false;
          const shaped = result as Record<string, unknown>;
          if (
            shaped.approval === null ||
            typeof shaped.approval !== "object" ||
            !isWireJob(shaped.job) ||
            typeof shaped.replayed !== "boolean"
          ) {
            return false;
          }
          const approval = shaped.approval as Record<string, unknown>;
          return (
            typeof approval.approval_id === "string" &&
            typeof approval.job_id === "string" &&
            typeof approval.state === "string"
          );
        },
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

    // The provider-connection surface (IMPL-8; the card-10 working
    // candidate). The paths are the Core adapter's own — non-canonical
    // (wire.ts's honest boundary). The truthfulness rules above apply
    // unchanged: a lost response is `unknown`, never a save/test/rotation
    // result; no method retries on its own (the Open-class reconciliation
    // is the explicit re-read the owning surface performs).

    /** `POST /v1/connections` — the M-24 create with its Supplied
     *  idempotency: duplicate → the existing reference (replayed=true);
     *  same key + different body → the typed CONFLICT_IDEMPOTENCY_KEY.
     *  The credential VALUE rides this one body (the explicit save
     *  action); no response ever echoes it. */
    connectionSave(payload: WireConnectionSaveRequest): Promise<TransportOutcome<WireConnectionSaveResult>> {
      return call<WireConnectionSaveResult>("/v1/connections", post(JSON.stringify(payload)), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (shaped.connection === null || typeof shaped.connection !== "object") return false;
        return isWireConnectionView(shaped.connection) && typeof shaped.replayed === "boolean";
      });
    },

    /** `GET /v1/connections?limit=&cursor=` — the connection-list read
     *  (the card-9 list discipline): the cursorless page is the final
     *  page; rows carry the masked secret status, never a value. */
    connectionList(params: { limit?: number; cursor?: string } = {}): Promise<TransportOutcome<WireConnectionListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireConnectionListResult>(`/v1/connections${suffix}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (!Array.isArray(shaped.connections)) return false;
        return shaped.connections.every((row) => isWireConnectionView(row));
      });
    },

    /** `GET /v1/connections/{id}` — one connection's view (the masked
     *  status + tested-state truth). A Core NOT_FOUND is the typed
     *  refusal surfaced verbatim. */
    connectionGet(connectionID: string): Promise<TransportOutcome<WireConnectionGetResult>> {
      return call<WireConnectionGetResult>(`/v1/connections/${encodeURIComponent(connectionID)}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        return shaped.connection !== null && typeof shaped.connection === "object" && isWireConnectionView(shaped.connection);
      });
    },

    /** `POST /v1/connections/test` — the read-only M-25 probe: either a
     *  saved connection (the id) or an unsaved safe draft whose typed
     *  secret rides THIS body once and is dropped after the probe. The
     *  response is the sanitized report; a lost response proves nothing
     *  and this method never re-issues on its own. */
    connectionTest(payload: WireConnectionTestRequest): Promise<TransportOutcome<WireConnectionTestResult>> {
      return call<WireConnectionTestResult>("/v1/connections/test", post(JSON.stringify(payload)), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (shaped.report === null || typeof shaped.report !== "object") return false;
        const report = shaped.report as Record<string, unknown>;
        return typeof report.ok === "boolean" && typeof report.provider === "string" && typeof report.correlation_id === "string";
      });
    },

    /** `POST /v1/connections/{id}/rotate` — the M-26 rotation: the NEW
     *  write-only key under the SAME handle (the D-37 same-handle swap).
     *  The response carries the affected-models/routing + interruption
     *  statement; the response never echoes the key. */
    connectionRotate(connectionID: string, secret: string): Promise<TransportOutcome<WireConnectionRotateResult>> {
      const body: WireConnectionRotateRequestShape = { secret };
      return call<WireConnectionRotateResult>(
        `/v1/connections/${encodeURIComponent(connectionID)}/rotate`,
        post(JSON.stringify(body)),
        isWireConnectionRotateResult,
      );
    },

    /** `POST /v1/connections/{id}/reauthenticate` — the M-27 credential
     *  re-entry (the rotate path's cousin): a fresh write-only key under
     *  the same record, its own audited action. */
    connectionReauthenticate(connectionID: string, secret: string): Promise<TransportOutcome<WireConnectionRotateResult>> {
      const body: WireConnectionRotateRequestShape = { secret };
      return call<WireConnectionRotateResult>(
        `/v1/connections/${encodeURIComponent(connectionID)}/reauthenticate`,
        post(JSON.stringify(body)),
        isWireConnectionRotateResult,
      );
    },

    /** `POST /v1/connections/{id}/delete` — the delete/revoke command:
     *  the record is deleted AND the keyring entry revoked (the D-37
     *  fail-closed order; the high-risk confirmation is the surface's). */
    connectionDelete(connectionID: string): Promise<TransportOutcome<Record<string, never>>> {
      return call<Record<string, never>>(
        `/v1/connections/${encodeURIComponent(connectionID)}/delete`,
        post("{}"),
        (body) => body !== null && typeof body === "object",
      );
    },

    /** `POST /v1/connections/{id}/switch` — the minimal M-34 working
     *  candidate: the durable active-connection pointer, idempotent on
     *  the already-active connection (replayed=true). No Job migrates
     *  and no cache mixes. */
    connectionSwitch(connectionID: string): Promise<TransportOutcome<WireConnectionSwitchResult>> {
      return call<WireConnectionSwitchResult>(
        `/v1/connections/${encodeURIComponent(connectionID)}/switch`,
        post("{}"),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return typeof shaped.active_connection_id === "string" && typeof shaped.replayed === "boolean";
        },
      );
    },

    /** The conversation surface (IMPL-10; the adopted S5 mapping, P-S5-01…05
     *  as the working candidates). The paths are the Core adapter's own —
     *  non-canonical (wire.ts's honest boundary). The truthfulness rules
     *  above apply unchanged: a lost response is `unknown`, never a
     *  create/send result; no method retries on its own — the S4
     *  resolution is the explicit query-by-correlation-ID read the owning
     *  surface performs (the send-idempotency class stays Open — no
     *  auto-retry anywhere in the conversation client path). */

    /** `POST /v1/conversations` — the aggregate create (P-S5-01): the Core
     *  mints the stable conversation identity; the body carries the
     *  optional bounded title only. */
    conversationCreate(payload: WireConversationCreateRequest): Promise<TransportOutcome<WireConversationResult>> {
      return call<WireConversationResult>("/v1/conversations", post(JSON.stringify(payload)), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        return isWireConversation(shaped.conversation);
      });
    },

    /** `GET /v1/conversations?limit=&q=&cursor=` — the server-side
     *  list/search read (P-S5-04; the card-9 working candidate). The
     *  Core-defined order returns as served; the cursorless page IS the
     *  final page; an empty page IS the honest no-match answer. */
    conversationList(params: WireConversationsListParams = {}): Promise<TransportOutcome<WireConversationListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.q !== undefined && params.q !== "") query.set("q", params.q);
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireConversationListResult>(`/v1/conversations${suffix}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (!Array.isArray(shaped.conversations)) return false;
        return (
          shaped.conversations.every((row) => isWireConversation(row)) &&
          (shaped.cursor === undefined || typeof shaped.cursor === "string")
        );
      });
    },

    /** `GET /v1/conversations/{id}` — one conversation aggregate's read
     *  (safe metadata only). A Core NOT_FOUND is the typed refusal
     *  surfaced verbatim — never reinterpreted (the no-leak rule is the
     *  Core's; the UI never fabricates a count or a title). */
    conversationGet(conversationID: string): Promise<TransportOutcome<WireConversationResult>> {
      return call<WireConversationResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return isWireConversation(shaped.conversation);
        },
      );
    },

    /** `GET /v1/conversations/{id}/messages?limit=&cursor=` — the
     *  authoritative timeline read (P-S5-02): the Core's stored state in
     *  seq order, one bounded page at a time; the cursorless page IS the
     *  final page. */
    conversationMessages(
      conversationID: string,
      params: { limit?: number; cursor?: string } = {},
    ): Promise<TransportOutcome<WireConversationMessagesResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireConversationMessagesResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/messages${suffix}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (!Array.isArray(shaped.messages)) return false;
          return (
            shaped.messages.every((row) => isWireConversationMessage(row)) &&
            (shaped.cursor === undefined || typeof shaped.cursor === "string")
          );
        },
      );
    },

    /** `POST /v1/conversations/{id}/send` — the Ask send (P-S5-05): the
     *  caller-presented correlation ID rides the body (the P1.6 §3.9 M-1
     *  request identity — the caller carries it; this method never mints
     *  a second identity). The response is the recorded exchange plus the
     *  persisted user message — NO answer (no answering production path
     *  exists). A lost response is `unknown` and this method NEVER
     *  re-issues on its own: the resolution is the explicit S4 query
     *  below, never a duplicate send. */
    conversationSend(
      conversationID: string,
      payload: WireConversationSendRequest,
    ): Promise<TransportOutcome<WireConversationExchangeResult>> {
      return call<WireConversationExchangeResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/send`,
        post(JSON.stringify(payload)),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return isWireConversationExchange(shaped.exchange) && isWireConversationMessage(shaped.message);
        },
      );
    },

    /** `GET /v1/conversations/{id}/exchange?correlation_id=` — the S4
     *  query-before-retry read (P-S5-05): the authoritative exchange
     *  state for one send, resolved by the correlation ID. A pure query —
     *  it never re-sends, never mutates. A Core NOT_FOUND (including the
     *  no-leak foreign-binding case) is the typed refusal surfaced
     *  verbatim — never reinterpreted. */
    conversationExchangeStatus(
      conversationID: string,
      correlationID: string,
    ): Promise<TransportOutcome<WireConversationExchangeResult>> {
      const query = new URLSearchParams({ correlation_id: correlationID });
      return call<WireConversationExchangeResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/exchange?${query.toString()}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return isWireConversationExchange(shaped.exchange) && isWireConversationMessage(shaped.message);
        },
      );
    },

    /** Discards the held token from memory (the expiry/recovery path).
     * Returns the safe metadata snapshot for the truthful display. */
    discardSession(): CoreSessionMetadata | null {
      return discardHeldSecret();
    },
  };
}

export type HttpLocalTransport = ReturnType<typeof createHttpLocalTransport>;
