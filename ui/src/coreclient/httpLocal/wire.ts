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
  | "CONFLICT_IDEMPOTENCY_KEY"
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

/** The context-snapshot view (the card-11 working candidate as the Core
 *  projects it — the app-layer `SnapshotView` JSON tags): the STRUCTURE of
 *  one compilation per the P3.2 §3.5 Supplied semantics. The compiled
 *  CONTENT (the assembled representation) is absent from the shape by
 *  construction — the read returns references, digests, labels, reasons,
 *  and budget facts, never a bulk content dump. */
export interface WireSnapshotItem {
  readonly ref: string;
  readonly digest: string;
  readonly trust_label: string;
  readonly reason: string;
}

export interface WireSnapshotOmission {
  readonly ref: string;
  readonly reason: string;
}

/** The requested bounds (the coded `BudgetRequested`). The Core's zero
 *  means "no bound of this kind" — such a bound is ABSENT on the wire,
 *  never zero (the UI-22 label discipline). */
export interface WireSnapshotBudgetRequested {
  readonly max_input_tokens?: number;
  readonly reserved_output_tokens?: number;
  readonly max_items?: number;
}

/** The budget pair: the requested bounds vs what the Core reports (its
 *  compiled estimate, labeled by the estimator version it names). No
 *  provider-reported usage and no cost member exist — missing renders
 *  Not-reported upstream, never zero, never an invented price. */
export interface WireSnapshotBudget {
  readonly requested: WireSnapshotBudgetRequested;
  readonly estimated_input_tokens: number;
  readonly estimator_version: string;
}

export interface WireSnapshotView {
  readonly schema_version: number;
  readonly snapshot_id: string;
  readonly task_ref: string;
  readonly scope: string;
  readonly user_input_digest: string;
  readonly assembled_digest: string;
  readonly created_at: string;
  readonly included: readonly WireSnapshotItem[];
  readonly omitted: readonly WireSnapshotOmission[];
  readonly budget: WireSnapshotBudget;
  readonly estimator_version: string;
  readonly renderer_version: string;
  readonly compression_method: string;
  readonly retrieval_config_version: string;
  readonly exact_vs_approximate: string;
}

/** `GET /v1/jobs/{id}/snapshot` success (200): `{snapshot}` — the
 *  job-scoped read (the optional `attempt` query value selects a specific
 *  attempt; its absence resolves the Job's current attempt). */
export interface WireJobSnapshotResult {
  readonly snapshot: WireSnapshotView;
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

/** The provider-connection record (the card-10 working candidate's coded
 *  object — the JSON tags of `dprovider.ConnectionRecord` in
 *  `internal/domain/provider/connections.go`): the Core-issued id, the
 *  wired adapter id, the exact Model ID, the optional restricted custom
 *  headers, the secret REFERENCE (handle-only on disk per D-37 — never a
 *  value), the credential epoch (the rotation marker), the tested state
 *  (the tested-at instant + inputs digest + epoch; the digest and epoch
 *  are what render "tested, but changed since" truthfully), and the
 *  created/updated instants. No field ever carries a secret value. */
export interface WireConnectionHeader {
  readonly name: string;
  readonly value: string;
}

export interface WireConnectionRecord {
  readonly connection_id: string;
  readonly idempotency_key?: string;
  readonly provider: string;
  readonly base_url?: string;
  readonly model_id: string;
  readonly custom_headers?: readonly WireConnectionHeader[];
  readonly secret_ref: string;
  readonly credential_epoch: number;
  readonly tested_at?: string;
  readonly tested_digest?: string;
  readonly tested_epoch?: number;
  readonly created_at: string;
  readonly updated_at: string;
}

/** The read view (the app-layer `ConnectionView`): the record plus the
 *  safe masked secret status (the D-37 masked-status vocabulary:
 *  present/missing/revoked/corrupt/inaccessible), the tested-state truth
 *  (a tested result that no longer matches the current inputs renders
 *  test_current=false — stale-truthfully), and the active flag (the M-34
 *  switch pointer). */
export interface WireConnectionView {
  readonly record: WireConnectionRecord;
  readonly secret_status: string;
  readonly test_current: boolean;
  readonly active: boolean;
}

/** `POST /v1/connections` request body (M-24): the Supplied create
 *  idempotency — duplicate → the existing reference; same key + different
 *  body → 409 CONFLICT_IDEMPOTENCY_KEY. The credential VALUE rides the
 *  body once (the explicit save action); the response never echoes it. */
export interface WireConnectionSaveRequest {
  readonly idempotency_key: string;
  readonly provider: string;
  readonly base_url?: string;
  readonly model_id: string;
  readonly custom_headers?: readonly WireConnectionHeader[];
  readonly secret?: string;
}

/** `POST /v1/connections` success (200): `{connection, replayed}`. */
export interface WireConnectionSaveResult {
  readonly connection: WireConnectionView;
  readonly replayed: boolean;
}

/** `GET /v1/connections` success (200): `{connections, cursor?}` — the
 *  cursorless page is the final page (the card-9 list discipline). */
export interface WireConnectionListResult {
  readonly connections: readonly WireConnectionView[];
  readonly cursor?: string;
}

/** `GET /v1/connections/{id}` success (200): `{connection}`. */
export interface WireConnectionGetResult {
  readonly connection: WireConnectionView;
}

/** `POST /v1/connections/test` request body (M-25): either a saved
 *  connection (`connection_id`) or an unsaved safe draft whose typed
 *  secret rides transiently — sent only by this explicit action, dropped
 *  after the probe, never stored, never echoed. */
export interface WireConnectionTestRequest {
  readonly connection_id?: string;
  readonly provider?: string;
  readonly base_url?: string;
  readonly model_id?: string;
  readonly custom_headers?: readonly WireConnectionHeader[];
  readonly secret?: string;
}

/** The sanitized M-25 report (the coded `ConnectionTestReport` tags): the
 *  verified endpoint/model + capability statuses on success; the typed
 *  error class + the bounded redacted detail on failure. The correlation
 *  id is the only request detail a surface needs. */
export interface WireConnectionTestReport {
  readonly ok: boolean;
  readonly provider: string;
  readonly model_id: string;
  readonly base_url?: string;
  readonly capabilities?: unknown;
  readonly error_kind?: string;
  readonly error_detail?: string;
  readonly correlation_id: string;
}

/** `POST /v1/connections/test` success (200): `{report, record?}` — the
 *  record is set only when the test rode a saved connection (the TC-1
 *  tested-state stamp). */
export interface WireConnectionTestResult {
  readonly report: WireConnectionTestReport;
  readonly record?: WireConnectionRecord;
}

/** `POST /v1/connections/{id}/rotate|reauthenticate` request body
 *  (M-26/M-27): the NEW write-only replacement key — never the decrypted
 *  old key, never echoed. */
export interface WireConnectionCredentialRequest {
  readonly secret: string;
}

/** `POST /v1/connections/{id}/rotate|reauthenticate` success (200): the
 *  updated record plus the working candidate's affected-models/routing +
 *  interruption statement (the P3.2 §3.3 [P] row as the working
 *  candidate). */
export interface WireConnectionRotateResult {
  readonly record: WireConnectionRecord;
  readonly affected_models: readonly string[];
  readonly routing_note: string;
  readonly interruption_note: string;
}

/** `POST /v1/connections/{id}/switch` success (200): the durable
 *  active-connection pointer (the minimal M-34 working candidate),
 *  idempotent on the already-active connection. */
export interface WireConnectionSwitchResult {
  readonly active_connection_id: string;
  readonly replayed: boolean;
}

// The conversation surface (IMPL-10; the adopted S5 mapping, P-S5-01…05
// as the working candidates). The JSON tags below are the coded tags of
// the domain conversation contract (internal/domain/conversation) and the
// adapter's coded wire structs (internal/httplocal/server.go): the
// wrapper keys are the coded struct names lowercased (the coded-name
// rule). Honest boundary (the same class as every family above):
// - The `/v1/conversations...` paths are the adapter's OWN non-canonical
//   paths — no published contract carries them.
// - NO answer, stream, stop, or steering member exists anywhere in these
//   shapes: the answering/stream production path is the named owner gap
//   (S5 §4 P-S5-03 gap (a)) — a send RECORDS the exchange and returns no
//   answer; the timeline honestly ends at the recorded message.
// - NO attachment field exists (P-S5-06…11 are their owners' supply) and
//   NO lifecycle member exists (UI-28 gated, the owner's decision 8) —
//   no rename, archive, delete, export, edit, or regenerate shape.
// - NO idempotency key exists on the send (the send-idempotency class
//   stays Open — no C1-07 row): the send carries the caller-presented
//   correlation ID (the P1.6 §3.9 M-1 request identity), which is NOT an
//   idempotency key; a duplicate identity on a recorded exchange is the
//   Core's typed CONFLICT_STATE refusal.

/** `internal/domain/conversation.Conversation` — the coded JSON tags
 *  exactly: the Core-issued stable conversation identity, the display
 *  title (empty when untitled — the identity renders then), and the
 *  created/updated instants (RFC 3339). Safe metadata only (X-6). */
export interface WireConversation {
  readonly conversation_id: string;
  readonly title: string;
  readonly created_at: string;
  readonly updated_at: string;
}

/** `internal/domain/conversation.Message` — the coded JSON tags exactly:
 *  the stable per-message identity, the intra-conversation ordering key
 *  (`seq` — 1-based, the timeline's authoritative order), the closed
 *  role register's value (`user` is the only coded value today — an
 *  unknown future value renders verbatim as the neutral unknown it is),
 *  the content, the correlation-ID linkage to the send that produced it
 *  (absent on a message with no send linkage), and the created instant.
 *  No revisions, no edit history, no answer member (UI-28 + the owner
 *  gap). */
export interface WireConversationMessage {
  readonly message_id: string;
  readonly conversation_id: string;
  readonly seq: number;
  readonly role: string;
  readonly content: string;
  readonly correlation_id?: string;
  readonly created_at: string;
}

/** `internal/domain/conversation.Exchange` — the coded JSON tags exactly:
 *  the send's durable linkage row bound to (conversation identity,
 *  correlation ID). The correlation ID is PRIMARY — the identity the S4
 *  query-before-retry read resolves through. `state` carries the closed
 *  exchange-state register (`recorded` is the only coded value today). */
export interface WireConversationExchange {
  readonly correlation_id: string;
  readonly conversation_id: string;
  readonly user_message_id: string;
  readonly state: string;
  readonly created_at: string;
}

/** `POST /v1/conversations` request body (`conversationCreateRequest`):
 *  the optional bounded display title only — the Core mints the stable
 *  conversation identity; the body never carries one. */
export interface WireConversationCreateRequest {
  readonly title: string;
}

/** `POST /v1/conversations/{id}/send` request body
 *  (`conversationSendRequest`): the message content plus the send's
 *  correlation ID — the request identity the caller carries (P1.6 §3.9
 *  M-1). It is NOT an idempotency key: the class stays Open, no replay
 *  semantics ride it, and no client-side retry loop exists anywhere. */
export interface WireConversationSendRequest {
  readonly content: string;
  readonly correlation_id: string;
}

/** `POST /v1/conversations` and `GET /v1/conversations/{id}` success
 *  (200): `{conversation}`. */
export interface WireConversationResult {
  readonly conversation: WireConversation;
}

/** The `GET /v1/conversations` query parameters (the card-9 working
 *  candidate's list semantics as delivered in IMPL-10): an omitted
 *  `limit` selects the Core's per-surface default (50 — the client
 *  invents no limit of its own); `q` is the optional server-side search
 *  term (empty = the plain list); `cursor` is the opaque continuation
 *  cursor. */
export interface WireConversationsListParams {
  readonly limit?: number;
  readonly q?: string;
  readonly cursor?: string;
}

/** `GET /v1/conversations` success (200): `{conversations, cursor?}` —
 *  the coded `conversationListResult` shape. The order is the Core's own
 *  (updated_at descending, conversation_id ascending tiebreak); `cursor`
 *  is OMITTED on the final page: the cursorless response IS the end
 *  marker (no has_more/total vocabulary exists). An empty page IS the
 *  honest no-match answer for a search — never an error. */
export interface WireConversationListResult {
  readonly conversations: readonly WireConversation[];
  readonly cursor?: string;
}

/** `GET /v1/conversations/{id}/messages` success (200):
 *  `{messages, cursor?}` — the coded `conversationMessagesResult`
 *  shape: the authoritative Core state in timeline order (seq
 *  ascending), one bounded page at a time; the cursorless page IS the
 *  final page. */
export interface WireConversationMessagesResult {
  readonly messages: readonly WireConversationMessage[];
  readonly cursor?: string;
}

/** `POST /v1/conversations/{id}/send` and
 *  `GET /v1/conversations/{id}/exchange?correlation_id=` success (200):
 *  `{exchange, message}` — the coded `conversationExchangeResult`
 *  shape. The send response carries NO answer — no answering production
 *  path exists. The exchange read is the S4 query-by-correlation-ID
 *  resolution: the authoritative exchange state for one send. */
export interface WireConversationExchangeResult {
  readonly exchange: WireConversationExchange;
  readonly message: WireConversationMessage;
}

// The notification surface (IMPL-14; the ADOPTED S6 supply, P-S6-01…07 —
// the owner's LIVE S6 approval gate of 2026-11-03, consumed as the
// working candidates they are, never finalized). Every key is a coded
// name typed BY HAND against the coded JSON tags of
// `internal/domain/notification/notification.go` and the coded adapter
// structs of `internal/httplocal/server.go`
// (`notificationListResult`/`notificationCountsResult`/
// `notificationMutationRequest`/`notificationMutationResult`).
// Boundaries carried exactly:
// - The `/v1/notifications...` paths are the adapter's OWN non-canonical
//   paths (the IMPL-4/6/7/8/9/10/11 convention); no published contract
//   carries them, and no wire name here is canonical anywhere.
// - The four-way presentation register (unread | read | stale |
//   permission_revoked) rides the payload as coded; the UI renders it
//   non-color (icon + text, never color alone).
// - The mutation set is EXACTLY the delivered three (mark read / mark
//   unread / clear over explicit `notification_ids`): NO mark-all-read
//   member exists (not in the P-S6-02 named set), and a mutation is
//   all-or-nothing on the Core side.
// - NO stream, OS-delivery, OS-permission, quiet-hours, or preference
//   member exists anywhere (P-S6-04/05; decision 6 — in-app ON + OS OFF
//   by default; the UI-34 FINAL freeze stays its own Checkpoint-C act).
// - NO prompt, file path, token, generated output, or private model data
//   exists in any shape (the P-S6-06 prohibition list): safe metadata
//   only.
// - Unread counts ride Core-supplied state only; the UI never counts
//   locally.

/** `internal/domain/notification.CollapsedBlock` — the coded JSON tags
 *  exactly: one collapsed block of repeated identical updates within a
 *  group (the same related object + category + state-class run collapsed
 *  to a count with its newest timestamp and its unread count). A
 *  meaningful state transition is a DIFFERENT block — never hidden by
 *  the collapse. */
export interface WireNotificationCollapsedBlock {
  readonly state_class: string;
  readonly count: number;
  readonly unread_count: number;
  readonly newest_at: string;
}

/** `internal/domain/notification.GroupEntry` — the coded JSON tags
 *  exactly: one list entry, one related object's group. The entry keeps
 *  the newest timestamp (the representative's), the group's unread
 *  count, the group's total, the collapsed blocks (every meaningful
 *  transition visible), the current canonical state where the owning
 *  domain supplies it (`object_state`; empty = the Core supplies none —
 *  rendered as Unavailable, never invented), and the four-way
 *  presentation state (`presentation`). `notification_id` is the
 *  group's representative — the group's newest notification's
 *  Core-issued identity, and the only member identity the list payload
 *  exposes. */
export interface WireNotificationGroupEntry {
  readonly notification_id: string;
  readonly category: string;
  readonly state_class: string;
  readonly related_object_type: string;
  readonly related_object_id: string;
  readonly created_at: string;
  readonly unread_count: number;
  readonly repeat_count: number;
  readonly collapsed: readonly WireNotificationCollapsedBlock[];
  readonly object_state?: string;
  readonly presentation: string;
}

/** `GET /v1/notifications` query parameters. No limit is sent by this
 *  client (the Core's per-surface default applies — the IMPL-5/11
 *  pattern); `category` and `read_state` are the "filter" control's
 *  working-candidate resolution (list-query parameters over the closed
 *  registers); `cursor` is the opaque continuation cursor. */
export interface WireNotificationsListParams {
  readonly category?: string;
  readonly read_state?: string;
  readonly cursor?: string;
}

/** `GET /v1/notifications` success (200): `{entries, cursor?}` — the
 *  coded `notificationListResult` shape. The order is the Core's own
 *  (created_at descending, notification_id ascending tiebreak);
 *  `cursor` is OMITTED on the final page: the cursorless response IS the
 *  end marker (no has_more/total vocabulary exists). An empty page IS
 *  the honest no-notification answer — never an error shape that
 *  invents one. */
export interface WireNotificationsListResult {
  readonly entries: readonly WireNotificationGroupEntry[];
  readonly cursor?: string;
}

/** `GET /v1/notifications/counts` success (200): `{total, by_category}`
 *  — the coded `notificationCountsResult` shape, the Core-supplied
 *  unread counts (cleared rows excluded). Unread counts render only
 *  from this Core-supplied state — never locally counted. */
export interface WireNotificationCountsResult {
  readonly total: number;
  readonly by_category: Readonly<Record<string, number>>;
}

/** `POST /v1/notifications/read|unread|clear` request body — the coded
 *  `notificationMutationRequest` shape: the body names EXACTLY the
 *  notification identities to mutate. No implicit target, no
 *  mark-all-read. */
export interface WireNotificationMutationRequest {
  readonly notification_ids: readonly string[];
}

/** `POST /v1/notifications/read|unread|clear` success (200):
 *  `{updated, unchanged}` — the coded `notificationMutationResult`
 *  shape: the typed Core outcome (all-or-nothing on the Core side;
 *  `unchanged` counts rows already in the requested state — the
 *  convergent no-op, recorded honestly as state-convergence by nature,
 *  NOT a documented idempotency class). The outcome is never rendered as
 *  the confirmation by itself: the surface re-reads the authoritative
 *  list and counts. */
export interface WireNotificationMutationResult {
  readonly updated: number;
  readonly unchanged: number;
}
