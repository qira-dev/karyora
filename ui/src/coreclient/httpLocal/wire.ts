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

/** The pre-submit review request (`reviewRequest` in
 *  `internal/httplocal/server.go`): exactly the submit request's shape
 *  keys minus the idempotency key — a review never carries one (the key
 *  is minted at submit time, never at review time). */
export interface WireReviewRequest {
  readonly task: string;
  readonly system?: string;
  readonly schema?: unknown;
  readonly pack_ref?: string;
  readonly provider: string;
  readonly model?: string;
  readonly max_output_tokens?: number;
  readonly classification?: string;
  readonly base_url?: string;
}

/** Field 2 — the selected sources (the coded KnowledgePack-reference
 *  resolution; `state` is `none` | `resolved` | `unresolved` |
 *  `unavailable`). */
export interface WireReviewSources {
  readonly state: string;
  readonly pack_ref?: string;
  readonly pack_id?: string;
  readonly version?: number;
  readonly status?: string;
  readonly reason?: string;
  readonly scope?: string;
}

/** The coded capability facts of one profile
 *  (`internal/domain/provider.CapabilitySet`): supported/unsupported/
 *  unknown per axis — unknown means unsupported wherever it gates a call,
 *  rendered by the UI as its not-verified state, never a granted badge. */
export interface WireCapabilitySet {
  readonly chat: string;
  readonly streaming: string;
  readonly tools: string;
  readonly parallel_tool_calls: string;
  readonly structured_output: string;
  readonly vision: string;
  readonly audio: string;
  readonly embeddings: string;
  readonly batch: string;
  readonly context_caching: string;
  readonly conversation_continuation: string;
  readonly max_context_tokens?: number;
  readonly max_output_tokens?: number;
  readonly token_estimator_ref?: string;
  readonly notes?: string;
}

/** Field 3 — the provider/model: the wired truth, the effective profile,
 *  and the coded capability facts (absent when the provider is unwired —
 *  never invented). */
export interface WireReviewProviderModel {
  readonly provider: string;
  readonly wired: boolean;
  readonly model?: string;
  readonly profile?: {
    readonly provider: string;
    readonly endpoint: string;
    readonly model: string;
    readonly region?: string;
    readonly api_version?: string;
  };
  readonly capabilities?: WireCapabilitySet;
}

/** Field 4 — one projected effect of the v1 task execution path, in the
 *  workflow effect vocabulary (`class`: none|local|external; `retry`:
 *  safe|scoped|never; `unknown_outcome`: fail|needs_reconciliation). The
 *  class value `unknown` is the projection's honest fail-closed value:
 *  undetermined means unknown, never a guessed class. */
export interface WireReviewEffect {
  readonly effect: string;
  readonly class: string;
  readonly retry: string;
  readonly idempotency_scope?: string;
  readonly unknown_outcome: string;
}

/** Field 5 — the permission state the Core reports (the pure decision-port
 *  outcome for the submit-class check; UI visibility is not enforcement —
 *  the Submit path's own Guard decision stays authoritative). */
export interface WireReviewPermission {
  readonly allowed: boolean;
  readonly reason: string;
  readonly action: string;
}

/** Field 6 — the policy snapshot the Core applies: the data-handling
 *  record's coded facts verbatim (unknown facts carry the coded
 *  `unknown`), the record's review status, and the fail-closed egress
 *  decision with its typed blocking reason. */
export interface WireReviewPolicy {
  readonly profile_known: boolean;
  readonly review?: string;
  readonly deployment?: string;
  readonly training_use?: string;
  readonly abuse_retention?: string;
  readonly application_state_ttl?: string;
  readonly subprocessors?: string;
  readonly deletion_eligibility?: string;
  readonly egress_allowed: boolean;
  readonly egress_reason?: string;
}

/** Field 7 — the approval need: the Core's report of its own coded
 *  posture, never an assumption either way. */
export interface WireReviewApprovalNeed {
  readonly required: boolean;
  readonly basis: string;
}

/** Field 8 — the estimated budget: only determinable facts are present;
 *  an ABSENT field renders Not-available, never zero (no cost source
 *  exists in code, so no cost value is ever invented). */
export interface WireReviewBudget {
  readonly reserved_output_tokens?: number;
}

/** The eight-field pre-submit review payload, exactly the recorded field
 *  set (P3.6 §4.1) as the Core projects it. */
export interface WireReviewPayload {
  readonly intent: {
    readonly task: string;
    readonly system?: string;
    readonly schema_provided: boolean;
  };
  readonly sources: WireReviewSources;
  readonly provider_model: WireReviewProviderModel;
  readonly tool_effects: readonly WireReviewEffect[];
  readonly permission: WireReviewPermission;
  readonly policy: WireReviewPolicy;
  readonly approval_need: WireReviewApprovalNeed;
  readonly budget: WireReviewBudget;
}

/** `POST /v1/jobs/review` success (200): `{review}` — the review
 *  operation's own name lowercased (the coded-name rule). */
export interface WireReviewResult {
  readonly review: WireReviewPayload;
}

/** `GET /v1/jobs/{id}` success (200): `{job, attempt}`. */
export interface WireStatusResult {
  readonly job: WireJob;
  readonly attempt: WireAttempt;
}

/** `internal/aijobrelay.Approval` — the coded JSON tags exactly: the
 *  Core-issued, stable, queryable approval identity bound to the raised
 *  request (the card-14 working candidate's approval-request object
 *  schema). The `state` values are the coded approval register
 *  (`pending` / `accepted` / `rejected`); the decision values are the
 *  coded accept/reject command vocabulary. */
export interface WireApproval {
  readonly approval_id: string;
  readonly job_id: string;
  readonly state: string;
  readonly decision?: string;
  readonly decided_by?: string;
  readonly correlation_id?: string;
  readonly created_at: string;
  readonly decided_at?: string;
}

/** `GET /v1/jobs/{id}/approval` success (200): `{approval}` — the Job's
 *  latest raised approval request. The read mutates nothing: a read never
 *  implies approval. */
export interface WireApprovalReadResult {
  readonly approval: WireApproval;
}

/** The approval decision command body (`approvalDecisionRequest` in
 *  `internal/httplocal/server.go`): the Core-issued approval identity the
 *  decision carries — the decision's scope is exactly this presented
 *  identity, never broader. */
export interface WireApprovalDecisionRequest {
  readonly approval_id: string;
}

/** `POST /v1/jobs/{id}/approvals/accept|reject` success (200): the
 *  Core-confirmed post-decision state — the recorded approval, the
 *  post-decision Job, and the honest `replayed` flag (the same decision
 *  re-presented returns the recorded outcome without re-applying). */
export interface WireApprovalDecisionResult {
  readonly approval: WireApproval;
  readonly job: WireJob;
  readonly replayed: boolean;
}

/** The `GET /v1/jobs` query parameters (the card-9 working candidate's
 *  list semantics, as delivered in IMPL-4). Every field is optional: an
 *  omitted `limit` selects the Core's per-surface default (50 — the
 *  client invents no limit of its own); `status` is the closed coded
 *  Job-state registry (an unknown value is the Core's typed refusal, not
 *  a client-side guess); `cursor` is the opaque continuation cursor. */
export interface WireJobsListParams {
  readonly limit?: number;
  readonly status?: string;
  readonly cursor?: string;
}

/** `GET /v1/jobs` success (200): `{jobs, cursor?}` — the coded
 *  `listResult` shape (internal/httplocal/server.go). `cursor` is the
 *  opaque list cursor and is OMITTED on the final page: the cursorless
 *  response IS the end marker (no has_more/total vocabulary exists —
 *  the working candidate finalizes at the freeze). The empty list is an
 *  empty `jobs` array, never null. */
export interface WireJobsListResult {
  readonly jobs: readonly WireJob[];
  readonly cursor?: string;
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
