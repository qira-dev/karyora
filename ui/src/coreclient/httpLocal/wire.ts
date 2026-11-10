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
 * The knowledge section (B1-U1) is the ONE exception to the by-hand
 * typing, and it is deliberate: the B1 Knowledge* shapes ride the
 * GENERATED canonical types (the `components` members of
 * `src/coreclient/generated-eval-surface/eval-surface.d.ts`, the
 * drift-checked snapshot of `contracts/openapi.yaml`) — consumed
 * verbatim, never hand-restyped. The adapter's knowledge JSON tags carry
 * the published member names exactly (B1-A1 §2.5), so the generated
 * types ARE the served shapes; only the adapter's wrapper keys and the
 * `/v1` result shapes are typed here (the coded-name rule).
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

/** The generated canonical contract types (see the header note): the
 *  knowledge section consumes them verbatim. */
import type { components } from "../generated-eval-surface/eval-surface";

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

/** `app.CapabilityDescriptor` — the F2 family's capability descriptor
 *  (IMPL-22, the adopted S10 supply's P-S10-01 class as implemented): the
 *  Core's OWN coded capability state for one wired provider identity —
 *  the coded CapabilitySet verbatim plus the identity members the read
 *  keys on. The descriptor carries FACTS, never promises (C3-29): every
 *  unoverridden axis stays unknown; nothing beyond what the Core's coded
 *  Capabilities() supplies rides it (each matrix field's presence stays
 *  Unverified — the P3.2 §5.1 verdict). */
export interface WireCapabilityDescriptor {
  readonly provider: string;
  readonly profile: {
    readonly provider: string;
    readonly endpoint: string;
    readonly model: string;
    readonly region?: string;
    readonly api_version?: string;
  };
  readonly capabilities: WireCapabilitySet;
}

/** `GET /v1/providers/{id}/capabilities` success (200): the descriptor
 *  inside the `capability_descriptor` wrapper. The standing discovery/
 *  status read (P-S10-02 as implemented): it runs NO network probe and is
 *  ADDITIVE to the M-25 test (the probe-bearing discovery surface stays
 *  exactly as coded). A provider outside the wired set is the typed
 *  NOT_FOUND — never an invented descriptor, never a name-inferred
 *  capability. */
export interface WireCapabilityDescriptorResult {
  readonly capability_descriptor: WireCapabilityDescriptor;
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

/** `internal/aijobrelay.RetryEligibility` — the F9 family's frozen
 *  typed-member set exactly (IMPL-20; the coded JSON tags): the Core's
 *  own eligibility verdict, the side-effect risk statement in the
 *  recorded exact terms, the idempotency guarantee class (the closed
 *  three-value register `supplied_keyed`/`open`/`none` — the Core
 *  reports `supplied_keyed` since the M-7 command's keying act), and the
 *  reconciliation requirement. The verdict is the Core's report, never a
 *  client-side inference (P-S7-02): the M-7 control renders from THIS
 *  read or not at all. */
export interface WireRetryEligibility {
  readonly eligible: boolean;
  readonly risk_statement: string;
  readonly idempotency_class: string;
  readonly reconciliation_required: boolean;
}

/** `GET /v1/jobs/{id}/retry-eligibility` success (200): the four typed
 *  members inside the `retry_eligibility` wrapper. The read mutates
 *  nothing and executes nothing — an allow verdict authorizes no
 *  execution; only the retry command applies one. */
export interface WireRetryEligibilityReadResult {
  readonly retry_eligibility: WireRetryEligibility;
}

/** The M-7 retry command body (`retryCommandRequest` in
 *  `internal/httplocal/server.go`): the Supplied-keyed idempotency
 *  reference the retried operation carries — the same reference returns
 *  the original result, replayed, never regenerated; a contradicting
 *  re-issue is the typed CONFLICT_STATE. */
export interface WireRetryCommandRequest {
  readonly idempotency_reference: string;
}

/** `POST /v1/jobs/{id}/retry` success (200): the Core-confirmed
 *  post-command Job and the honest `replayed` flag. A retry is a NEW
 *  Attempt of the SAME Job — never a new Job — and does not undo the
 *  prior Attempt's effects (retry ≠ reversal). */
export interface WireRetryCommandResult {
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

/** `POST /v1/session/logout` success (200): the C-4 per-token logout
 *  command's confirmation (the adopted P-S3-03 candidate point 1; the
 *  S9-adopted P-S9-02 class as implemented — the coded per-token `Revoke`
 *  served through the one decision port). The canonical wire name joins
 *  the published vocabulary at the publication act (O-1 stays Open; no
 *  contracts/ edit). */
export interface WireSessionLogoutResult {
  readonly revoked: boolean;
}

/** `POST /v1/session/revoke-all` success (200): the C-4 credential-change
 *  revocation's outcome — every live session of the instance invalidated
 *  (the coded `RevokeAll` class), with the post-effect live-session count
 *  as the honest safe-metadata evidence (counts only, never token
 *  material). */
export interface WireSessionRevokeAllResult {
  readonly live_sessions: number;
}

/** `internal/security.AuditRecord` — the coded JSON tags exactly (the
 *  adopted [S] audit-record schema, carried exactly: digests and CAS
 *  references only — never prompt or document content, never secret
 *  values, never session tokens). */
export interface WireAuditRecord {
  readonly audit_id: string;
  readonly schema_version: number;
  readonly time: string;
  readonly actor: string;
  readonly action: string;
  readonly scope: string;
  readonly policy_decision: string;
  readonly policy_reference: string;
  readonly job_id?: string;
  readonly attempt_id?: string;
  readonly provider_id?: string;
  readonly plugin_id?: string;
  readonly outcome: string;
  readonly content_digests?: readonly string[];
  readonly content_refs?: readonly string[];
  readonly correlation_id: string;
}

/** One audit list/export entry (IMPL-24): the record's chain position
 *  and the record itself — the coded schema carried exactly, never
 *  extended. */
export interface WireAuditEntry {
  readonly sequence: number;
  readonly record: WireAuditRecord;
}

/** `GET /v1/audit` success (200) — the F12 permission-filtered audit
 *  list (the card-9 working candidate's class): the bounded page + the
 *  opaque continuation cursor, omitted on the final page (the coded end
 *  marker — no has_more/total is invented). The filter values ride the
 *  Core's own coded allow/deny register only. */
export interface WireAuditListResult {
  readonly entries: readonly WireAuditEntry[];
  readonly cursor?: string;
}

/** `GET /v1/audit/export` success (200) — the M-32-class read-with-export
 *  (the adopted P-S11-03 class as implemented): the chronological export
 *  document, the honest count, the echoed confirmed range, the generation
 *  instant, and the honest truncation flag. The export reads records,
 *  mutates nothing, and adds no tamper-resistance. */
export interface WireAuditExportResult {
  readonly records: readonly WireAuditEntry[];
  readonly count: number;
  readonly from?: string;
  readonly to?: string;
  readonly generated_at: string;
  readonly truncated: boolean;
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
  /** The record's optional user-facing display label (the M-27 rename
   *  row as implemented — a rename changes ONLY this field). Empty means
   *  untitled: the surfaces render the provider/model identity as before. */
  readonly display_name?: string;
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
 *  verified endpoint/model + the coded capability statuses (the
 *  `CapabilitySet` shape, typed as `WireCapabilitySet`) on success; the
 *  typed error class + the bounded redacted detail on failure. The
 *  correlation id is the only request detail a surface needs. */
export interface WireConnectionTestReport {
  readonly ok: boolean;
  readonly provider: string;
  readonly model_id: string;
  readonly base_url?: string;
  readonly capabilities?: WireCapabilitySet;
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

/** The connection-record rename and the non-secret field update (IMPL-17;
 *  the adopted S8 supply's P-S8-01/P-S8-02 working candidates — the M-27
 *  register row's rename portion and the coded create shape's non-secret
 *  field set). The honest boundary (the same class as every family
 *  above):
 * - The `/v1/connections...` paths are the adapter's OWN non-canonical
 *   paths — no published contract carries them.
 * - `expected_updated_at` is the marker the caller read the record under:
 *   the concurrent-edit guard's input (the adopted S8 supply's CHOSEN
 *   class, disclosed at implementation — the P3.3-recorded typed
 *   CONFLICT_STATE guard with `currentState`, the P1.6 §5 recorded
 *   typed-outcome class). A stale marker is the typed 409 CONFLICT_STATE;
 *   the surface renders it verbatim and performs NO overwrite and NO
 *   merge (the P1.6 §5 interim rule's rendering duty).
 * - NO secret field exists on either request: the secret path stays the
 *   coded rotate/re-authenticate flows — an update NEVER touches a
 *   credential, and the rename touches ONLY the display name (never the
 *   connection identity, never the Job identity, never the execution
 *   history). */

/** `POST /v1/connections/{id}/rename` request body: the display name ONLY
 *  (trimmed and bounded by the Core's law) plus the concurrency marker. */
export interface WireConnectionRenameRequest {
  readonly display_name: string;
  readonly expected_updated_at: string;
}

/** `POST /v1/connections/{id}/rename` success (200): the Core-confirmed
 *  view. */
export interface WireConnectionRenameResult {
  readonly connection: WireConnectionView;
}

/** `POST /v1/connections/{id}/update` request body: EXACTLY the coded
 *  create shape's non-secret field set plus the concurrency marker. */
export interface WireConnectionUpdateRequest {
  readonly provider: string;
  readonly base_url?: string;
  readonly model_id: string;
  readonly custom_headers?: readonly WireConnectionHeader[];
  readonly expected_updated_at: string;
}

/** `POST /v1/connections/{id}/update` success (200): the Core-confirmed
 *  view; any accepted update renders the tested state stale-truthfully
 *  (the Core's coded inputs digest). */
export interface WireConnectionUpdateResult {
  readonly connection: WireConnectionView;
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
// - The attachment contract (P-S5-06…11 / P-S12-01…06, the ADOPTED S12
//   supply) is implemented as its own section below — the conversation
//   and message shapes here still carry NO attachment member: the
//   message-binding shape, if any, belongs to the answering/production
//   milestone and nothing here invents one. NO lifecycle member exists
//   (UI-28 gated, the owner's decision 8) — no rename, archive, delete,
//   export, edit, or regenerate shape on messages or conversations.
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
  readonly auto_answer?: boolean;
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
  readonly answer?: WireConversationMessage;
}

// The conversation attachment surface (IMPL-25; the ADOPTED S12 supply,
// P-S12-01…06 — the owner's LIVE S12 approval of 2026-11-05 — as
// implemented: the working candidates they are, never finalized). Every
// key is a coded name typed BY HAND against the coded JSON tags of
// `internal/domain/conversation/attachment.go`, the app registry shapes
// (`internal/app/conversation_attachment.go`), and the coded adapter
// structs of `internal/httplocal/server.go`
// (`conversationAttachmentUploadRequest`/`conversationAttachmentDeleteRequest`/
// `conversationAttachmentRegistryResult`/`conversationAttachmentResult`/
// `conversationAttachmentListResult`/`conversationAttachmentDeleteResult`).
// Boundaries carried exactly (the honest typings the implementation
// records, never claimed canonical):
// - The paths are the adapter's OWN non-canonical paths; the canonical
//   names join the published vocabulary at the publication act.
// - P-S12-01: the registry below is the CORE's own served contract data —
//   the UI renders ONLY it, invents no list, no guess, no default
//   allowance; its contents (the types, the sizes, the absence of any
//   per-scope difference) are the implementation's typing.
// - P-S12-02: the upload is identity-bearing (`upload_id` — the
//   caller-supplied operation identity scoped to its conversation; the
//   identity's class stays its owner's decision). A lost upload response
//   resolves ONLY by the status query — NEVER a blind re-upload; the
//   Core's replay ruling returns the existing upload's state on (same
//   identity, same digest) and refuses a different digest
//   (CONFLICT_STATE).
// - P-S12-03: every upload carries and echoes the opaque input revision
//   it belongs to; the stale-revision discard is the UI's race guard.
// - P-S12-04: rejection is the typed envelope (VALIDATION_FAILED with
//   data.cause ∈ {type, size, policy}) — never a silent client-side
//   truncation, never a raw upstream error.
// - P-S12-05: `untrusted_label` rides the record AS DATA (`untrusted` is
//   the only coded value — every imported attachment is untrusted);
//   attachment content is data, never instruction.
// - P-S12-06: NO snapshot-participation member, NO message-binding
//   member, and NO stream/stop/steering member exists anywhere in these
//   shapes — the participation policy is not exercised and the answering
//   production path is the named owner gap; the UI never assumes
//   inclusion or exclusion and never invents a reference.

/** The attachment registry member (P-S12-01 as implemented): one
 *  supported content type with its per-type size bound in bytes — the
 *  Core's own values, the implementation's typing (the owners' supply act
 *  canonicalizes them). */
export interface WireAttachmentTypeSpec {
  readonly content_type: string;
  readonly max_bytes: number;
}

/** `GET /v1/conversations/attachments/registry` success (200):
 *  `{types}` — the Core's own supported attachment types and size limits
 *  as queryable contract data. No per-scope difference exists in this
 *  implementation — the absence is carried honestly rather than dressed
 *  as a member. */
export interface WireAttachmentRegistryResult {
  readonly types: readonly WireAttachmentTypeSpec[];
}

/** `internal/domain/conversation.Attachment` — the coded JSON tags
 *  exactly: the Core-issued stable attachment identity, the conversation
 *  scope, the caller-supplied upload identity (the S4-for-uploads query
 *  key), the display file name (a label, never a path), the registry
 *  content type, the CAS-verified byte count, the CAS content digest
 *  (`sha256:<hex>` — the exact canonical reference form this
 *  implementation carries), the untrusted-content label AS DATA (the
 *  closed register's `untrusted`), the echoed opaque input revision, the
 *  closed state register's `stored`, and the created instant. Safe
 *  metadata only (X-6); the bytes NEVER ride this shape — they live in
 *  the CAS behind the digest. */
export interface WireConversationAttachment {
  readonly attachment_id: string;
  readonly conversation_id: string;
  readonly upload_id: string;
  readonly file_name: string;
  readonly content_type: string;
  readonly size_bytes: number;
  readonly digest: string;
  readonly untrusted_label: string;
  readonly input_revision: string;
  readonly state: string;
  readonly created_at: string;
}

/** `POST /v1/conversations/{id}/attachments` request body
 *  (`conversationAttachmentUploadRequest`): the caller-supplied upload
 *  identity, the display file name, the registry content type, the opaque
 *  input revision, and the base64 (standard, padded) content — the
 *  envelope-only JSON transport's typing (no multipart parser exists on
 *  this surface and none is introduced). */
export interface WireConversationAttachmentUploadRequest {
  readonly upload_id: string;
  readonly file_name: string;
  readonly content_type: string;
  readonly input_revision: string;
  readonly content_b64: string;
}

/** `POST /v1/conversations/{id}/attachments` and
 *  `GET /v1/conversations/{id}/attachments/status?upload_id=` success
 *  (200): `{attachment}` — the upload result and the S4-for-uploads
 *  status query's shape. A typed rejection (the per-type bound, the type
 *  membership, the shape law) is the D-35 envelope surfaced verbatim. */
export interface WireConversationAttachmentResult {
  readonly attachment: WireConversationAttachment;
}

/** `GET /v1/conversations/{id}/attachments?limit=&cursor=` success
 *  (200): `{attachments, cursor?}` — the coded
 *  `conversationAttachmentListResult` shape: one bounded chronological
 *  page; the cursorless page IS the final page. */
export interface WireConversationAttachmentListResult {
  readonly attachments: readonly WireConversationAttachment[];
  readonly cursor?: string;
}

/** `POST /v1/conversations/{id}/attachments/delete` request body
 *  (`conversationAttachmentDeleteRequest`): the body names exactly the
 *  attachment to delete — no implicit target. */
export interface WireConversationAttachmentDeleteRequest {
  readonly attachment_id: string;
}

/** `POST /v1/conversations/{id}/attachments/delete` success (200):
 *  `{}` — the confirmed 200 IS the outcome; no tombstone exists. */
export type WireConversationAttachmentDeleteResult = Record<string, never>;

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

// The knowledge surface (B1-U1; the DE-0 queue row 2). The typed shapes
// are the GENERATED canonical contract types consumed verbatim (see the
// header note) — the adapter's knowledge JSON tags carry the published
// member names exactly (B1-A1 §2.5), so the generated types ARE the
// served shapes. Honest boundary (the same class as every family above):
// - The `/v1/knowledgepacks...` paths are the adapter's own serving of
//   the published `/knowledgepacks` family (B1-A1 §2.1) — the published
//   members verbatim under the adapter's `/v1` prefix.
// - NO claims/evidence content is served (B1-A1 §2.6 item 1: counts ride
//   the version as 0 and no claim/evidence member exists on the served
//   version), NO source/entry content read exists, and NO artifact
//   download route exists — none of it is invented anywhere downstream.

/** The published KnowledgePack schema (generated, consumed verbatim). */
export type KnowledgePack = components["schemas"]["KnowledgePack"];

/** The published KnowledgePackVersion schema (generated, consumed
 *  verbatim). */
export type KnowledgePackVersion = components["schemas"]["KnowledgePackVersion"];

/** The published KnowledgePackList schema (generated, consumed
 *  verbatim). */
export type KnowledgePackList = components["schemas"]["KnowledgePackList"];

/** The published KnowledgePackVersionList schema (generated, consumed
 *  verbatim). */
export type KnowledgePackVersionList = components["schemas"]["KnowledgePackVersionList"];

/** The served version read's `created_at` member. HONEST GAP NOTE (the
 *  DE-0 §5 law, recorded in B1-U1 §6 — never a silent contract edit):
 *  the published schema REQUIRES `created_at` on every
 *  KnowledgePackVersion but does not DEFINE the property, so the
 *  generated type structurally omits it. The adapter serves the member
 *  (the coded JSON tag in internal/domain/knowledge), and the UI
 *  consumes it through this narrow served-shape extension — never a
 *  hand-restatement of the contract type itself. */
export type KnowledgePackVersionServed = KnowledgePackVersion & { readonly created_at: string };

/** The published KnowledgeImportSourceSpec schema (generated, consumed
 *  verbatim): one served registry member — the Core's own supported
 *  kinds and per-kind bounds; the served contents are the canonical
 *  values, and the UI renders EXACTLY that list. */
export type KnowledgeImportSourceSpec = components["schemas"]["KnowledgeImportSourceSpec"];

/** The published KnowledgeImportRequest schema (generated, consumed
 *  verbatim): the import submit body (the M-9 digest-keyed idempotency
 *  class). */
export type KnowledgeImportRequest = components["schemas"]["KnowledgeImportRequest"];

/** The published KnowledgeExportRequest schema (generated, consumed
 *  verbatim): the export submit body (the read-with-output class; the
 *  scope/destination shown before generation). */
export type KnowledgeExportRequest = components["schemas"]["KnowledgeExportRequest"];

/** The published KnowledgeOperation schema (generated, consumed
 *  verbatim): the state endpoint's authoritative record. No percentage
 *  member exists anywhere on it (the no-fabricated-percentage law). */
export type KnowledgeOperation = components["schemas"]["KnowledgeOperation"];

/** The published KnowledgeOperationSubmitResult schema (generated,
 *  consumed verbatim): `{operation, replayed}` — `replayed` is the honest
 *  M-9 replay flag, and the 202 is accepted-for-processing, never
 *  completion (the J-5 rule: the request is never the confirmation). */
export type KnowledgeOperationSubmitResult = components["schemas"]["KnowledgeOperationSubmitResult"];

/** `GET /v1/knowledgepacks?limit=&cursor=` success (200): the published
 *  `KnowledgePackList` — the keyset page; `cursor` is OMITTED on the
 *  final page: the cursorless page IS the end marker (no has_more/total
 *  vocabulary exists). */
export type WireKnowledgePackListResult = KnowledgePackList;

/** `GET /v1/knowledgepacks/{id}` success (200): `{pack}` (the
 *  coded-name rule: the operation's own name lowercased). */
export interface WireKnowledgePackResult {
  readonly pack: KnowledgePack;
}

/** `GET /v1/knowledgepacks/{id}/versions?limit=&cursor=` success (200):
 *  the published `KnowledgePackVersionList`'s page shape with every
 *  served version carrying its served `created_at` member (see
 *  KnowledgePackVersionServed's honest gap note); the cursorless page
 *  IS the final page. */
export interface WireKnowledgeVersionListResult {
  readonly versions: readonly KnowledgePackVersionServed[];
  readonly cursor?: string;
}

/** `GET /v1/knowledgepacks/{id}/versions/{version_id}` success (200):
 *  `{version}` — one immutable version's structural members (the
 *  served shape with `created_at`; see the gap note). */
export interface WireKnowledgeVersionResult {
  readonly version: KnowledgePackVersionServed;
}

/** `GET /v1/knowledgepacks/import/registry` success (200): `{sources}` —
 *  the Core's own served supported-source registry; the read creates
 *  nothing and probes nothing. */
export interface WireKnowledgeRegistryResult {
  readonly sources: readonly KnowledgeImportSourceSpec[];
}

/** `POST /v1/knowledgepacks/import` and `POST /v1/knowledgepacks/export`
 *  success (202): the published `KnowledgeOperationSubmitResult` —
 *  `{operation, replayed}`. */
export type WireKnowledgeSubmitResult = KnowledgeOperationSubmitResult;

/** `GET /v1/knowledgepacks/operations/{id}` (the family's authoritative
 *  state endpoint — a pure query, never a retry) and
 *  `POST /v1/knowledgepacks/operations/{id}/cancel` (202 = accepted for
 *  processing) success: `{operation}`. */
export interface WireKnowledgeOperationResult {
  readonly operation: KnowledgeOperation;
}

// The memory surface (B2-U1; the DE-0 queue row 4). The typed shapes are
// the GENERATED canonical contract types consumed verbatim (see the
// header note) — the adapter's memory JSON tags carry the published
// member names exactly (B2-A1 §2), so the generated types ARE the served
// shapes. Honest boundary (the same class as every family above):
// - The `/v1/memories...` paths are the adapter's own serving of the
//   published `/memories` family (B2-A1 §2.1) — the published members
//   verbatim under the adapter's `/v1` prefix.
// - The one additional served member is the item's `pinned` flag (B2-A1
//   §2.6 item 3 — this Core's served definition of the recorded
//   pin-register gap; the published MemoryItem schema carries no
//   additionalProperties:false): consumed through the narrow served-shape
//   extension below, never a hand-restatement of the contract type.
// - NO approve/reject positive path is reachable through the public API
//   (B2-A1 §2.6 item 4 — no pending_review producer exists), no
//   `memory_candidate.extracted`/`expired` event feed exists, no TTL
//   sweep surface exists, and NO export-artifact download route exists —
//   none of it is invented anywhere downstream.

/** The published MemoryItem schema (generated, consumed verbatim). */
export type MemoryItem = components["schemas"]["MemoryItem"];

/** The served item's `pinned` flag. HONEST SERVED-SHAPE NOTE (the DE-0
 *  §5 law, recorded in B2-A1 §2.6 item 3 — never a silent contract edit):
 *  the adapter serves `pinned` on every item as this Core's own served
 *  definition of the recorded pin-register gap (a FLAG, never a lifecycle
 *  state). The generated type structurally omits it; the UI consumes the
 *  served member through this narrow served-shape extension — never a
 *  hand-restatement of the contract type itself. */
export type MemoryItemServed = MemoryItem & { readonly pinned: boolean };

/** The published MemoryItemList schema (generated, consumed verbatim):
 *  the keyset page; `cursor` is OMITTED on the final page — the absence
 *  is the end marker. */
export type MemoryItemList = components["schemas"]["MemoryItemList"];

/** The published request bodies (generated, consumed verbatim):
 *  MemoryAddRequest (the M-17 creation identity), MemoryEditRequest (the
 *  M-16 revision identity + the prepared-against state reference),
 *  MemoryReviewRequest (approve/reject), MemoryPinRequest (one member
 *  covering both directions), MemoryInclusionRequest (the M-18
 *  request-scoped decision), MemoryExportRequest (the M-32 read-with-
 *  output scope). */
export type MemoryAddRequest = components["schemas"]["MemoryAddRequest"];
export type MemoryEditRequest = components["schemas"]["MemoryEditRequest"];
export type MemoryReviewRequest = components["schemas"]["MemoryReviewRequest"];
export type MemoryPinRequest = components["schemas"]["MemoryPinRequest"];
export type MemoryInclusionRequest = components["schemas"]["MemoryInclusionRequest"];
export type MemoryExportRequest = components["schemas"]["MemoryExportRequest"];

/** The published results (generated, consumed verbatim):
 *  MemoryMutationResult (the confirmed post-state item + the Core-minted
 *  operation reference), MemoryDeleteResult (the deletion truth + the
 *  Core-confirmed retention caveat), MemoryInclusionResult (the decision
 *  record — never an item-state change), MemoryExportResult (the honest
 *  count + generation instant + truncation flag), MemoryOperation (the
 *  state endpoint's authoritative record). */
export type MemoryMutationResult = components["schemas"]["MemoryMutationResult"];
export type MemoryDeleteResult = components["schemas"]["MemoryDeleteResult"];
export type MemoryInclusionResult = components["schemas"]["MemoryInclusionResult"];
export type MemoryExportResult = components["schemas"]["MemoryExportResult"];
export type MemoryOperation = components["schemas"]["MemoryOperation"];

/** The delete/forget members' OPTIONAL keying body (B2-A1 §2.6 item 1 —
 *  the recorded carrier gap: the published operations declare no
 *  requestBody while their descriptions name the M-16 keying members).
 *  The adapter accepts the members through this OPTIONAL body and serves
 *  the full M-16 ruling when keyed; an unkeyed call records a
 *  non-replayable destructive operation. The UI sends the keyed form (the
 *  caller-minted key + correlation), so a lost destructive response's
 *  keyed retry reconciles through the M-16 replay (the S0–S4 law). */
export interface WireMemoryDestructiveRequest {
  readonly idempotency_key: string;
  readonly correlation_id: string;
}

/** `GET /v1/memories?limit=&scope=&state=&cursor=` success (200): the
 *  published `MemoryItemList` page shape with every served item carrying
 *  its served `pinned` member (see MemoryItemServed's honest note — the
 *  same class as the B1 version-list's served extension); the cursorless
 *  page IS the final page (no has_more/total vocabulary exists). Removed
 *  items are not served. */
export interface WireMemoryListResult {
  readonly items: readonly MemoryItemServed[];
  readonly cursor?: string;
}

/** `POST /v1/memories` (add) and `POST /v1/memories/{id}` (edit) and the
 *  approve/reject/forget/pin members success (200): `{memory_item,
 *  operation_id}` — the Core-confirmed post-state item and the operation
 *  reference a lost response reconciles through (the S0–S4 law). */
export interface WireMemoryMutationResult {
  readonly memory_item: MemoryItemServed;
  readonly operation_id: string;
}

/** `GET /v1/memories/{id}` success (200): `{memory_item}` — the inspect
 *  read (a pure query, never a mutation). The unknown item and the
 *  out-of-scope item are the SAME typed 404 (the no-leak rule). */
export interface WireMemoryItemResult {
  readonly memory_item: MemoryItemServed;
}

/** `POST /v1/memories/{id}/delete` success (200): the published
 *  `MemoryDeleteResult` — the Core-confirmed deletion truth and its
 *  retention caveat (never a total-erasure promise). */
export type WireMemoryDeleteResult = MemoryDeleteResult;

/** `POST /v1/memories/{id}/inclusion` success (200): the published
 *  `MemoryInclusionResult` — the decision record. */
export type WireMemoryInclusionResult = MemoryInclusionResult;

/** `POST /v1/memories/export` success (200): the published
 *  `MemoryExportResult` — the read-with-output document (mutates
 *  nothing); the honest count, generation instant, and truncation flag. */
export type WireMemoryExportResult = MemoryExportResult;

/** `GET /v1/memories/operations/{id}` success (200): `{operation}` — the
 *  family's authoritative state endpoint (a pure query, never a retry). */
export interface WireMemoryOperationResult {
  readonly operation: MemoryOperation;
}

// The context surface (B3-U1; the DE-0 queue row 5). The typed shapes are
// the GENERATED canonical contract types consumed verbatim (see the
// header note) — the adapter's context JSON tags carry the published
// member names exactly (B3-A1 §2), so the generated types ARE the served
// shapes. Honest boundary (the same class as every family above):
// - The `/v1/context...` paths are the adapter's own serving of the
//   published `/context` family (B3-A1 §2.1) — the published members
//   verbatim under the adapter's `/v1` prefix.
// - THE CONDITIONAL LAW (BINDING): the save-snapshot command (the M-19
//   class) is CONDITIONAL on durable snapshot storage existing and the
//   compare (the M-20 class) on stable Core comparison — both conditions
//   are UNMET (the owner's PROPOSED decisions, unresolved), so BOTH
//   surfaces answer their typed unmet-condition refusals and NEITHER is
//   ever a fake success. The UI hides both controls ENTIRELY while the
//   conditions stand (never dead stubs — GC-3/GC-4); the transport
//   methods below exist so the typed refusal renders honestly if a
//   direct path ever reaches them.
// - The M-18 inclusion decision is request-scoped only: no item-state
//   change, no Memory action row, no scope broadening; the result's
//   reason members are the Core's alone — absence is the absence (the
//   Core-provided-only reason law; no reason producer exists on this
//   surface, so reason is never set — B3-A1 §2.6).
// - The metadata extension (ContextSnapshotMetadataExtension) has NO
//   serving source today (no standalone compiled-context read route was
//   published — the only snapshot route is F5's): nothing renders from
//   it anywhere downstream.

/** The published ContextSnapshotSaveRequest schema (generated, consumed
 *  verbatim). */
export type ContextSnapshotSaveRequest = components["schemas"]["ContextSnapshotSaveRequest"];

/** The published ContextSnapshotCompareRequest schema (generated,
 *  consumed verbatim). */
export type ContextSnapshotCompareRequest = components["schemas"]["ContextSnapshotCompareRequest"];

/** The published ContextInclusionRequest schema (generated, consumed
 *  verbatim). */
export type ContextInclusionRequest = components["schemas"]["ContextInclusionRequest"];

/** The published results (generated, consumed verbatim):
 *  ContextSnapshotSaveResult (the accepted outcome — no instance exists
 *  while the durable-storage gate stands), ContextSnapshotCompareResult
 *  (no instance exists while the comparison gate stands),
 *  ContextInclusionResult (the decision record for the stated request
 *  ONLY), ContextOperation (the state endpoint's authoritative record). */
export type ContextSnapshotSaveResult = components["schemas"]["ContextSnapshotSaveResult"];
export type ContextSnapshotCompareResult = components["schemas"]["ContextSnapshotCompareResult"];
export type ContextInclusionResult = components["schemas"]["ContextInclusionResult"];
export type ContextOperation = components["schemas"]["ContextOperation"];

/** `POST /v1/context/snapshots` success (202 — UNREACHABLE while the
 *  durable-storage gate stands; the typed unmet-condition refusal is the
 *  served behavior): the published `ContextSnapshotSaveResult`. */
export type WireContextSnapshotSaveResult = ContextSnapshotSaveResult;

/** `POST /v1/context/snapshots/compare` success (200 — UNREACHABLE while
 *  the comparison gate stands; the typed CONTEXT_COMPARE_UNSUPPORTED
 *  refusal is the served behavior): the published
 *  `ContextSnapshotCompareResult`. */
export type WireContextSnapshotCompareResult = ContextSnapshotCompareResult;

/** `POST /v1/context/inclusion` success (200): the published
 *  `ContextInclusionResult` — the decision record for the stated request
 *  ONLY (never an item-state change, never a Memory action row). */
export type WireContextInclusionResult = ContextInclusionResult;

/** `GET /v1/context/operations/{id}` success (200): `{operation}` — the
 *  family's authoritative state endpoint (a pure query, never a retry);
 *  the unknown identity is the SAME typed 404
 *  (CONTEXT_SNAPSHOT_NOT_FOUND — the no-leak rule). */
export interface WireContextOperationResult {
  readonly operation: ContextOperation;
}

// The workflow surface (B4-U1 reads; B4-U2 writes; the DE-0 queue row 7 /
// the GR-0 queue row 4). The typed shapes are the GENERATED canonical
// contract types consumed verbatim (see the header note) — the adapter's
// workflow JSON tags carry the published member names exactly (B4-A1 §2),
// so the generated types ARE the served shapes. Honest boundary (the same
// class as every family above):
// - The `/v1/workflows...` paths are the adapter's own serving of the
//   published `/workflows` family (B4-A1 §2.1) — the published members
//   verbatim under the adapter's `/v1` prefix.
// - THE UI-30 RESOLUTION (GR-C2, the owner's LIVE 2026-11-09 decision):
//   the B4-U2 act (GR-0 queue row 4) wires the family's create member
//   (the aggregate-create class — the Core mints the identity, the 72 h
//   keyed replay), the validate member (the read-like Core-authoritative
//   typed validation — no silent save), and the save-revision member (the
//   M-21 class — INSERT-only, the new ref stated, the 1 h keyed replay,
//   the interim conflict rule). The AI-proposal member
//   (proposeWorkflowAIDiff) is served by the Core in its honest
//   UNMET-PRODUCER state (the typed refusal — B4-A1 §2.4 gap 1) and
//   carries NO method here and NO caller anywhere: the producer is
//   unwired (the provider/model selection and the data-handling/egress
//   review surface the published request schema does NOT carry are
//   UNSUPPLIED), and no proposal surface is fabricated.
// - THE D-40 GATE (carried exactly): NO run-execution method exists
//   anywhere — the run-execution operation does not exist server-side
//   (B4-A1 §2.3's scripted absence checks) and none is typed here; the
//   only `run`-named surface is the run-history LINKAGE read, whose
//   entries are LINKAGE records (run_ref / source_revision /
//   linkage_state `linked | unavailable` / created_at) carrying NO
//   lifecycle, NO state register, and NO Run/Job cardinality claim. A
//   missing linkage renders `unavailable` — never an inferred link.
// - NO If-Match/ETag/precondition member exists anywhere — the clause is
//   prose-only (B4-A1 §2.2's scripted zero-member verification); the save
//   carries the client's known base as `base_revision_ref`, which asserts
//   no protocol.
// - THE KEYED MINTING (B4-U2): the create (72 h) and save-revision (1 h)
//   classes' idempotency key + correlation ID are CALLER-MINTED per new
//   attempt (the retention classes' windows govern the keyed replays);
//   the validate member is the none-required read class (correlation ID
//   only); the reads mint neither.

/** The published WorkflowDefinition schema (generated, consumed verbatim). */
export type WorkflowDefinition = components["schemas"]["WorkflowDefinition"];

/** The published WorkflowDefinitionList schema (generated, consumed
 *  verbatim): the keyset page; `next_cursor` is OMITTED on the final page
 *  — the absence is the end marker. */
export type WorkflowDefinitionList = components["schemas"]["WorkflowDefinitionList"];

/** The published WorkflowRevision schema (generated, consumed verbatim):
 *  the immutable revision's identity record (revision_ref + created_at).
 *  The family publishes NO revision-content read (B4-A1 §2.4 gap 7), so a
 *  stored revision renders ONLY this identity record — the draft content
 *  types below serve the WRITE path (the draft the save carries), never a
 *  revision-content rendering. */
export type WorkflowRevision = components["schemas"]["WorkflowRevision"];

/** The published WorkflowRunHistoryEntry schema (generated, consumed
 *  verbatim): a LINKAGE record, NOT the D-40-blocked Run schema. */
export type WorkflowRunHistoryEntry = components["schemas"]["WorkflowRunHistoryEntry"];

/** The published WorkflowRunHistory schema (generated, consumed verbatim). */
export type WorkflowRunHistory = components["schemas"]["WorkflowRunHistory"];

/** The published WorkflowDefinitionState schema (generated, consumed
 *  verbatim): the family's authoritative state endpoint's record. */
export type WorkflowDefinitionState = components["schemas"]["WorkflowDefinitionState"];

/** The published WorkflowValidationResult schema (generated, consumed
 *  verbatim): the Core-authoritative typed validation output (the
 *  client-checks-advisory law binds the DISPLAY, never this type). */
export type WorkflowValidationResult = components["schemas"]["WorkflowValidationResult"];

/** The published WorkflowValidationIssue schema (generated, consumed
 *  verbatim). */
export type WorkflowValidationIssue = components["schemas"]["WorkflowValidationIssue"];

/** `GET /v1/workflows?limit=&cursor=` success (200): the published
 *  `WorkflowDefinitionList` — the keyset page; the cursorless page IS the
 *  final page (no has_more/total vocabulary exists). */
export type WireWorkflowListResult = WorkflowDefinitionList;

/** `GET /v1/workflows/{id}` success (200): `{definition, revisions}` —
 *  the inspect member; each revision immutable and reachable. The unknown
 *  identity and the out-of-scope identity are the SAME typed 404 (the
 *  no-leak rule). */
export interface WireWorkflowInspectResult {
  readonly definition: WorkflowDefinition;
  readonly revisions: readonly WorkflowRevision[];
}

/** `GET /v1/workflows/{id}/run-history` success (200): the published
 *  `WorkflowRunHistory` — the honest empty page while the D-40 gate
 *  stands (the linkage table's producer is the GATED run execution). */
export type WireWorkflowRunHistoryResult = WorkflowRunHistory;

/** `GET /v1/workflows/{id}/state?revision_id=` success (200):
 *  `{definition_state}` — the family's authoritative state endpoint (a
 *  pure query — never a retry). */
export interface WireWorkflowStateResult {
  readonly definition_state: WorkflowDefinitionState;
}

// The B4-U2 write-path types (the UI-30 resolution's wired members; the
// GR-0 queue row 4). All generated, consumed verbatim — never hand-restyped.

/** The published WorkflowDraftNode schema (generated, consumed verbatim):
 *  one draft node — the client-local authoring unit. `node_type` is the
 *  type AS THE DRAFT DECLARES it (the Core validates against its own
 *  catalogue — WORKFLOW_UNKNOWN_NODE_TYPE for an uncontracted type); no
 *  node catalogue is published (the vocabulary is a CLASS only, P4.7
 *  §3.1), so no palette entry or type list is invented anywhere
 *  downstream. `config` is open-typed because the per-node schema is
 *  UNSUPPLIED [B4]. */
export type WorkflowDraftNode = components["schemas"]["WorkflowDraftNode"];

/** The published WorkflowDraftEdge schema (generated, consumed verbatim):
 *  one draft edge — the client-local connection between two draft nodes. */
export type WorkflowDraftEdge = components["schemas"]["WorkflowDraftEdge"];

/** The published WorkflowDraftContent schema (generated, consumed
 *  verbatim): the draft's graph content — the client-local authoring
 *  state. The draft is NEVER Core state until a save-revision confirms
 *  one (the no-silent-save law); validation and run submission never
 *  silently save it. */
export type WorkflowDraftContent = components["schemas"]["WorkflowDraftContent"];

/** The published WorkflowCreateRequest schema (generated, consumed
 *  verbatim): the aggregate-create body — the caller mints the key +
 *  correlation (the 72 h class); `display_name` is required; the body
 *  NEVER carries a definition identity (the Core-mints law). */
export type WorkflowCreateRequest = components["schemas"]["WorkflowCreateRequest"];

/** The published WorkflowCreateResult schema (generated, consumed
 *  verbatim): the Core-confirmed created definition and the honest
 *  `replayed` (the keyed replay returns the SAME Core-minted identity,
 *  never a second one). */
export type WorkflowCreateResult = components["schemas"]["WorkflowCreateResult"];

/** The published WorkflowValidateRequest schema (generated, consumed
 *  verbatim): the read-like validation body — correlation ID only (the
 *  none-required read class; no key minted); the optional
 *  `definition_id` states the definition the draft edits (absent for an
 *  unsaved new definition's draft). */
export type WorkflowValidateRequest = components["schemas"]["WorkflowValidateRequest"];

/** The published WorkflowRevisionSaveRequest schema (generated, consumed
 *  verbatim): the M-21 save body — the caller mints the key + correlation
 *  (the 1 h class); `base_revision_ref` is the client's known base (the
 *  fingerprint's other half — asserts NO protocol; the empty string is
 *  the client's known base for a definition with no revisions, the
 *  adapter's recorded reading, B4-A1 §2.4 gap 8); `draft` is the saved
 *  graph content. NO If-Match/ETag/precondition member exists. */
export type WorkflowRevisionSaveRequest = components["schemas"]["WorkflowRevisionSaveRequest"];

/** The published WorkflowRevisionSaveResult schema (generated, consumed
 *  verbatim): the Core-confirmed save outcome — the NEW immutable
 *  revision's reference STATED on save and the honest `replayed`. The
 *  draft's dirty state clears only on this confirmation (never a
 *  pre-emptive "saved" label). */
export type WorkflowRevisionSaveResult = components["schemas"]["WorkflowRevisionSaveResult"];

/** `POST /v1/workflows` success (200): the published
 *  `WorkflowCreateResult` — the aggregate-create outcome. A lost response
 *  is `unknown` and reconciles by the minted correlation ID through the
 *  keyed replay (the SAME key + the SAME request identity replays the
 *  recorded create — never a duplicate create) BEFORE any retry (the
 *  S0–S4 law). */
export type WireWorkflowCreateResult = WorkflowCreateResult;

/** `POST /v1/workflows/validate` success (200): the published
 *  `WorkflowValidationResult` — the Core-authoritative typed validation
 *  for the exact evaluated object (the `evaluated_object_ref` label).
 *  Read-like: it mutates NO stored revision and NEVER silently saves the
 *  draft. */
export type WireWorkflowValidateResult = WorkflowValidationResult;

/** `POST /v1/workflows/{definition_id}/revisions` success (200): the
 *  published `WorkflowRevisionSaveResult` — the M-21 outcome (the new
 *  reference stated; the old revision preserved immutable). A lost
 *  response reconciles by the authoritative definition/revision-state
 *  read BEFORE any retry (VRC-LOST-1; the S0–S4 law) — never a blind
 *  retry, never an update-in-place. The concurrent-revision outcome is
 *  the typed 409 `WORKFLOW_REVISION_CONFLICT` whose envelope `data`
 *  carries the stored side's REFERENCE only (`current_revision_ref`; the
 *  redaction law — the interim conflict rule governs). */
export type WireWorkflowSaveRevisionResult = WorkflowRevisionSaveResult;

// The plugin surface (B5-U1 reads; B5-U2 mutations; the DE-0 queue rows
// 9/GR-0 row 2). The typed shapes are the GENERATED canonical contract
// types consumed verbatim (see the header note) — the adapter's plugin
// JSON tags carry the published member names exactly (B5-A1 §2), so the
// generated types ARE the served shapes. Honest boundary (the same class
// as every family above):
// - The `/v1/plugins...` paths are the adapter's own serving of the
//   published `/plugins` family (B5-A1 §2.1) — the published members
//   verbatim under the adapter's `/v1` prefix.
// - THE UI-29 RESOLUTION (GR-C1, the owner's LIVE 2026-11-09 decision):
//   the B5-U2 act (GR-0 queue row 2) wires the family's mutation members
//   — install (the C1-07 class), enable/disable (the M-29 class),
//   update (the M-30 class), remove (the M-31 class), and revoke-grants
//   (the G7 class) — against the served adapter, with the §8.9
//   confirmations, the OC-1–OC-8 reconciliation, and the keyed
//   idempotency-key/correlation minting per the published retention
//   classes (install 72 h; update/remove/revoke 1 h). The D-13/D-14 +
//   the Core MB §10.2 runtime proof are untouched: no invocation surface
//   appears (the boundary below carries).
// - THE EXECUTION GATE (carried exactly): NO invocation/execution method
//   exists anywhere — the invocation/execution operation does not exist
//   server-side (B5-A1 §2.3's scripted absence checks: no invocation
//   route, no invocation port member, no invocation table, and the
//   `InvocationSupported` gate constant `false`) and none is typed here.
// - THE NO-CATALOG LAW (carried exactly): USER-INSTALLED ≠
//   CATALOG-LISTED, always — the list IS the user-installed list; no
//   catalog contract, no marketplace, no trust badge, and no one-click
//   install exists anywhere. The wired install surface consumes an
//   EXPLICIT user-supplied package reference (the contract's own single
//   body member) — never a catalog listing, never a recommendation; the
//   admission verdict is the Core's alone. The local-package import
//   member (P3) has NO served registry/upload shape in the published
//   family and P4.10 C-1's validation+quarantine precondition is unmet —
//   no import method is typed and no import control is fabricated.
// - THE ENABLE CARRIER GAP (carried honestly, B5-A1 §2.4 gap 1): the
//   published `PluginEnableRequest` body carries exactly ONE member
//   (`enabled`) — the keyed-class members its description references are
//   UNSUPPLIED on the published body, so the client sends exactly that
//   one member; the served toggle is the Core-recorded state-checked
//   mutation whose reconcile law is the authoritative state query.
// - THE VERDICT LAW (binding): every verification state
//   (signature/trust/rotation/revocation) is Core-RETURNED — the UI
//   computes none; each optional member renders only where the Core
//   returns it, and an absent verification state is never rendered as a
//   verified one. THE MAY-NEVER LIST rides the consuming surfaces: no
//   secret values (a named secret-handle's NAME only), no raw logs,
//   runtime internals, stack traces, or memory contents; no raw plugin
//   output or raw provider payloads as diagnostics; no diagnostic the
//   Core did not return.

/** The published PluginManifest schema (generated, consumed verbatim):
 *  the manifest's declared fields. The manifest member is OPTIONAL on the
 *  served `Plugin` record and is ABSENT on this Core (no
 *  manifest-examination path exists — B5-A1 §2.4 gap 6); an absent
 *  manifest renders its honest absence — never a fabricated one. Manifest
 *  text is UNTRUSTED DATA — rendered, never executed. */
export type PluginManifest = components["schemas"]["PluginManifest"];

/** The published PluginPackage schema (generated, consumed verbatim): the
 *  package record's Core-returned verification states. Each verification
 *  member renders ONLY as the Core returns it; on this Core the served
 *  package carries `package_ref` alone (no verification machinery exists
 *  while the MB §10.2 gate stands — B5-A1 §2.4 gap 3), so the others
 *  render their honest absence. */
export type PluginPackage = components["schemas"]["PluginPackage"];

/** The published Plugin schema (generated, consumed verbatim): one
 *  installed plugin record. `lifecycle_state` is the Core-returned
 *  authoritative state — the distinctness laws bind its rendering
 *  (DISABLED ≠ GRANTS-REVOKED ≠ TRUST-REVOKED; INVOCATION TERMINATION ≠
 *  PLUGIN DISABLE/REVOCATION). The Core's own served state values are
 *  `installed | enabled | disabled | grants_revoked | trust_revoked`
 *  (B5-A1 §2.4 gap 4 — the served-definition class); an unknown reported
 *  value renders verbatim — never mapped onto a known one. */
export type Plugin = components["schemas"]["Plugin"];

/** The published PluginList schema (generated, consumed verbatim): the
 *  keyset page; `next_cursor` is OMITTED on the final page — the absence
 *  is the end marker. */
export type PluginList = components["schemas"]["PluginList"];

/** The published PluginState schema (generated, consumed verbatim): the
 *  family's authoritative state endpoint's record. */
export type PluginState = components["schemas"]["PluginState"];

/** The published request bodies (generated, consumed verbatim):
 *  PluginInstallRequest (the C1-07 install identity — the Core mints the
 *  plugin/package/install-record/grant identities; the body never carries
 *  one), PluginUpdateRequest (the M-30 keyed identity + the target
 *  version reference), PluginMutationRequest (the keyed body shared by
 *  remove and revoke-grants), PluginEnableRequest (the M-29 toggle body —
 *  exactly ONE member; the carrier gap stands). */
export type PluginInstallRequest = components["schemas"]["PluginInstallRequest"];
export type PluginUpdateRequest = components["schemas"]["PluginUpdateRequest"];
export type PluginMutationRequest = components["schemas"]["PluginMutationRequest"];
export type PluginEnableRequest = components["schemas"]["PluginEnableRequest"];

/** The published results (generated, consumed verbatim):
 *  PluginInstallResult (`install_record_ref` + the honest `replayed` —
 *  the C1-07 replay returns the SAME install-record reference, never a
 *  second one) and PluginMutationResult (`plugin_id` + the Core-minted
 *  `operation_ref` + the honest `replayed`). */
export type PluginInstallResult = components["schemas"]["PluginInstallResult"];
export type PluginMutationResult = components["schemas"]["PluginMutationResult"];

/** `GET /v1/plugins?limit=&cursor=` success (200): the published
 *  `PluginList` — the keyset page; the cursorless page IS the final page
 *  (no has_more/total vocabulary exists). */
export type WirePluginListResult = PluginList;

/** `GET /v1/plugins/{plugin_id}` success (200): the published `Plugin` —
 *  the inspect member (a pure read). The unknown identity and the
 *  out-of-scope identity are the SAME typed 404 riding the envelope
 *  register's `NOT_FOUND` (no Plugin not-found domain code exists in the
 *  published register — the no-leak rule). */
export type WirePluginGetResult = Plugin;

/** `GET /v1/plugins/{plugin_id}/state` success (200): the published
 *  `PluginState` — the family's authoritative state endpoint (a pure
 *  query — never a retry); the reconciliation anchor for every lost
 *  lifecycle outcome (the S0–S4 law). The unknown identity is the SAME
 *  typed 404 (the no-leak rule). */
export type WirePluginStateResult = PluginState;

/** `POST /v1/plugins` success (200): the published `PluginInstallResult`
 *  — the C1-07 install outcome (installed, NOT-yet-enabled; activation
 *  separate through the permission review). A lost response reconciles by
 *  the authoritative package ID/version through the family's state read
 *  BEFORE any retry (the S0–S4 law) — never an optimistic "Installed". */
export type WirePluginInstallResult = PluginInstallResult;

/** The enable/disable, update, remove, and revoke-grants members' shared
 *  success (200): the published `PluginMutationResult` — the Core-minted
 *  operation reference is the lost-response reconcile anchor (never a
 *  timeout-inferred state). */
export type WirePluginMutationResult = PluginMutationResult;
