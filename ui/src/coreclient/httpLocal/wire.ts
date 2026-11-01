/**
 * The IMPL-2 browser HTTP transport wire types.
 *
 * Typed BY HAND against the coded JSON shapes of the Core's local HTTP
 * transport adapter exactly as delivered in IMPL-1 (commit
 * `79cc26cb05ca64d172433ebe4a6980b22f0fc125`): the route table and the
 * wire structs of `internal/httplocal/server.go`, the coded JSON tags of
 * the Job/Attempt/Record structs, the D-35 error envelope, and the
 * session service's exchange result.
 *
 * Honest boundary (recorded, never hidden):
 * - The `/v1/...` paths and the wrapper keys are the Core adapter's OWN
 *   paths — no published contract carries them, and no canonical
 *   wire-name claim is made anywhere (the canonical-transport question
 *   stays its recorded PROPOSED state; the publish act stays its own act).
 * - `contracts/openapi.yaml` is the explicitly NON-canonical
 *   `0.1.0-eval` evaluation surface; it was NOT consumed to produce these
 *   types, and the generated-eval-surface drift-check baseline is
 *   untouched by this milestone.
 * - Per the P1.5 §4.1 compatibility policy, unknown future values (error
 *   codes, boundary reasons, Job/Attempt states, event types) render
 *   safely as neutral unknowns — the string unions here document the
 *   CODED vocabularies but the client never fails on an unlisted value.
 * - No wire name, path, header, or field is invented beyond the coded
 *   shapes cited above.
 */

/** The coded Phase 2 error-code register (internal/domain/errs). This is
 *  the documented vocabulary, not a closed gate: an unlisted code is
 *  surfaced verbatim as the stable language-neutral identifier it is. */
export type CoreWireCode =
  | "VALIDATION_FAILED"
  | "CONFIG_CONFLICT"
  | "AUTHZ_DENIED"
  | "NOT_FOUND"
  | "CONFLICT_STATE"
  | "SINGLE_INSTANCE_CONFLICT"
  | "STORAGE_UNAVAILABLE"
  | "MIGRATION_FAILED"
  | "STATE_CORRUPT"
  | "RECOVERY_REQUIRED"
  | "RATE_LIMITED"
  | "INTERNAL";

/** The coded retry classes (the reserved `data.retryClass` key on the
 *  envelope, D-35). */
export type CoreWireRetryClass = "never" | "safe-retry" | "backoff" | "reconcile";

/** The eight coded boundary reason codes (internal/security/httpboundary):
 *  the only refusal vocabulary the Core's boundary emits. */
export type CoreWireBoundaryReason =
  | "NON_LOOPBACK_BIND"
  | "HOST_NOT_ALLOWED"
  | "ORIGIN_NOT_ALLOWED"
  | "STATE_CHANGING_GET"
  | "CSRF_CHECK_FAILED"
  | "CREDENTIAL_IN_URL"
  | "CREDENTIAL_IN_REFERER"
  | "MISSING_CSRF_TOKEN";

/** The coded session denial reasons the decision port can report through
 *  an AUTHZ_DENIED envelope (internal/security). */
export type CoreWireSessionReason = "SESSION_INVALID" | "SESSION_EXPIRED" | "SESSION_REVOKED";

/** The D-35 four-field machine error envelope, exactly as coded
 *  (`internal/domain/errs`): stable language-neutral `code`, `retryable`,
 *  server-generated `correlationId`, bounded typed `data`. No prose. */
export interface CoreWireEnvelope {
  readonly code: string;
  readonly retryable: boolean;
  readonly correlationId: string;
  readonly data: Readonly<Record<string, unknown>>;
}

/** `internal/aijobrelay.Job` — the coded JSON tags exactly. */
export interface WireJob {
  readonly job_id: string;
  readonly idempotency_key: string;
  readonly input_digest: string;
  readonly state: string;
  readonly version: number;
  readonly attempt_count: number;
  readonly waiting_reason?: string;
  readonly requirements: WireJobRequirements;
  readonly deadline_secs?: number;
  readonly result_digest?: string;
  readonly receipt_id?: string;
  readonly correlation_id?: string;
  readonly created_at: string;
  readonly updated_at: string;
}

/** `internal/aijobrelay.Requirements` — the coded JSON tags. */
export interface WireJobRequirements {
  readonly provider_id?: string;
  readonly min_memory_mb?: number;
}

/** `internal/aijobrelay.Attempt` — the coded JSON tags exactly. */
export interface WireAttempt {
  readonly attempt_id: string;
  readonly job_id: string;
  readonly seq: number;
  readonly worker_id: string;
  readonly lease_id: string;
  readonly lease_epoch: number;
  readonly outcome: string;
  readonly started_at: string;
  readonly ends_at?: string;
}

/** `internal/events.Record` — the coded JSON tags exactly. */
export interface WireEventRecord {
  readonly event_id: string;
  readonly aggregate_type: string;
  readonly aggregate_id: string;
  readonly sequence: number;
  readonly event_type: string;
  readonly schema_version: number;
  readonly occurred_at: string;
  readonly causation_id?: string;
  readonly correlation_id?: string;
  readonly producer: string;
  readonly inline_payload?: unknown;
  readonly payload_digest?: string;
  readonly blob_ref?: string;
}

/** The adapter's submit request (`submitRequest` in
 *  `internal/httplocal/server.go`): the coded JobInput tags plus the Job
 *  record's coded `idempotency_key`/`deadline_secs` tags. */
export interface WireSubmitRequest {
  readonly idempotency_key: string;
  readonly task: string;
  readonly system?: string;
  readonly schema?: unknown;
  readonly pack_ref?: string;
  readonly provider: string;
  readonly model?: string;
  readonly max_output_tokens?: number;
  readonly deadline_secs: number;
  readonly classification?: string;
  readonly base_url?: string;
}

/** `POST /v1/jobs` success (202): `{job, replayed}`. */
export interface WireSubmitResult {
  readonly job: WireJob;
  readonly replayed: boolean;
}

/** `GET /v1/jobs/{id}` success (200): `{job, attempt}`. */
export interface WireStatusResult {
  readonly job: WireJob;
  readonly attempt: WireAttempt;
}

/** `GET /v1/jobs/{id}/events` success (200): `{events}`. */
export interface WireEventsResult {
  readonly events: readonly WireEventRecord[];
}

/** The coded replay outcome values (internal/events): the explicit
 *  expiry/gap/forbidden outcomes — a client is never stranded. */
export type WireReplayOutcome = "ok" | "cursor_expired" | "replay_gap" | "forbidden";

/** `GET /v1/events/replay` success (200): `{outcome, events?, cursor?}`.
 *  The cursor is the opaque encoded cursor — never structured data. */
export interface WireReplayResult {
  readonly outcome: string;
  readonly events?: readonly WireEventRecord[];
  readonly cursor?: string;
}

/** `POST /v1/session/exchange` request body: the one-time bootstrap
 *  credential and the requested operation-class scopes (the coded
 *  register: `lifecycle` / `data`). */
export interface WireExchangeRequest {
  readonly credential: string;
  readonly scopes?: readonly string[];
}

/** `POST /v1/session/exchange` success (200): `{token, id, scopes,
 *  expires}` — the SessionToken field names lowercased. The token value
 *  is the bearer secret; the client holds it IN MEMORY ONLY (the
 *  client-side persistence rule stays its recorded open item). */
export interface WireSessionResult {
  readonly token: string;
  readonly id: string;
  readonly scopes: readonly string[];
  readonly expires: string;
}
