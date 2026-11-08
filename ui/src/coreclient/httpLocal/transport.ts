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
  WireConnectionRenameRequest,
  WireConnectionRenameResult,
  WireConnectionRotateResult,
  WireConnectionSaveRequest,
  WireConnectionSaveResult,
  WireConnectionSwitchResult,
  WireConnectionTestRequest,
  WireConnectionTestResult,
  WireConnectionUpdateRequest,
  WireConnectionUpdateResult,
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
  WireAttachmentRegistryResult,
  WireConversationAttachment,
  WireConversationAttachmentDeleteRequest,
  WireConversationAttachmentDeleteResult,
  WireConversationAttachmentListResult,
  WireConversationAttachmentResult,
  WireConversationAttachmentUploadRequest,
  WireEventRecord,
  WireCapabilityDescriptorResult,
  WireRetryCommandRequest,
  WireRetryCommandResult,
  WireRetryEligibilityReadResult,
  WireEventsResult,
  WireExchangeRequest,
  WireJob,
  WireJobsListParams,
  WireJobsListResult,
  WireJobSnapshotResult,
  WireNotificationCountsResult,
  WireNotificationGroupEntry,
  WireNotificationMutationRequest,
  WireNotificationMutationResult,
  WireNotificationsListParams,
  WireAuditEntry,
  WireAuditExportResult,
  WireAuditListResult,
  WireAuditRecord,
  WireNotificationsListResult,
  WireReplayResult,
  WireReviewRequest,
  WireReviewResult,
  WireSessionLogoutResult,
  WireSessionRevokeAllResult,
  WireSessionResult,
  WireStatusResult,
  WireSubmitRequest,
  WireSubmitResult,
  KnowledgeExportRequest,
  KnowledgeImportRequest,
  KnowledgeImportSourceSpec,
  KnowledgeOperation,
  KnowledgeOperationSubmitResult,
  KnowledgePack,
  KnowledgePackVersionServed,
  MemoryAddRequest,
  MemoryEditRequest,
  MemoryExportRequest,
  MemoryInclusionRequest,
  MemoryItemServed,
  MemoryOperation,
  MemoryPinRequest,
  MemoryReviewRequest,
  WireKnowledgeOperationResult,
  WireKnowledgePackListResult,
  WireKnowledgePackResult,
  WireKnowledgeRegistryResult,
  WireKnowledgeSubmitResult,
  WireKnowledgeVersionListResult,
  WireKnowledgeVersionResult,
  WireMemoryDeleteResult,
  WireMemoryExportResult,
  WireMemoryInclusionResult,
  WireMemoryListResult,
  WireMemoryMutationResult,
  WireMemoryOperationResult,
  ContextInclusionRequest,
  ContextOperation,
  ContextSnapshotCompareRequest,
  ContextSnapshotSaveRequest,
  WireContextInclusionResult,
  WireContextOperationResult,
  WireContextSnapshotCompareResult,
  WireContextSnapshotSaveResult,
  WorkflowCreateRequest,
  WorkflowDefinition,
  WorkflowDefinitionState,
  WorkflowRevision,
  WorkflowRevisionSaveRequest,
  WorkflowRunHistoryEntry,
  WorkflowValidateRequest,
  WireWorkflowCreateResult,
  WireWorkflowInspectResult,
  WireWorkflowListResult,
  WireWorkflowRunHistoryResult,
  WireWorkflowSaveRevisionResult,
  WireWorkflowStateResult,
  WireWorkflowValidateResult,
  Plugin,
  PluginEnableRequest,
  PluginInstallRequest,
  PluginManifest,
  PluginMutationRequest,
  PluginPackage,
  PluginState,
  PluginUpdateRequest,
  WirePluginGetResult,
  WirePluginInstallResult,
  WirePluginListResult,
  WirePluginMutationResult,
  WirePluginStateResult,
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

function isWireAuditRecord(value: unknown): value is WireAuditRecord {
  if (value === null || typeof value !== "object") return false;
  const r = value as Record<string, unknown>;
  return (
    typeof r.audit_id === "string" &&
    typeof r.schema_version === "number" &&
    typeof r.time === "string" &&
    typeof r.actor === "string" &&
    typeof r.action === "string" &&
    typeof r.scope === "string" &&
    typeof r.policy_decision === "string" &&
    typeof r.policy_reference === "string" &&
    typeof r.outcome === "string" &&
    typeof r.correlation_id === "string"
  );
}

function isWireAuditEntry(value: unknown): value is WireAuditEntry {
  if (value === null || typeof value !== "object") return false;
  const e = value as Record<string, unknown>;
  return typeof e.sequence === "number" && isWireAuditRecord(e.record);
}

function isWireAuditListResult(value: unknown): value is WireAuditListResult {
  if (value === null || typeof value !== "object") return false;
  const r = value as Record<string, unknown>;
  return Array.isArray(r.entries) && r.entries.every((row) => isWireAuditEntry(row));
}

function isWireAuditExportResult(value: unknown): value is WireAuditExportResult {
  if (value === null || typeof value !== "object") return false;
  const r = value as Record<string, unknown>;
  return (
    Array.isArray(r.records) &&
    r.records.every((row) => isWireAuditEntry(row)) &&
    typeof r.count === "number" &&
    typeof r.generated_at === "string" &&
    typeof r.truncated === "boolean"
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

/** The rename/update response guard (IMPL-17): the response must carry
 *  the Core-confirmed view's distinguishing coded fields to be presented
 *  as data; anything else stays unknown — never reinterpreted. */
function isWireConnectionRecordResult<T extends { readonly connection: WireConnectionView }>(value: unknown): value is T {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return shaped.connection !== null && typeof shaped.connection === "object" && isWireConnectionView(shaped.connection);
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

/** The coded attachment guard (IMPL-25): the response must carry the
 *  distinguishing coded fields of `conversation.Attachment` to be
 *  presented as data; anything else stays unknown — never reinterpreted,
 *  never fabricated. */
function isWireConversationAttachment(value: unknown): value is WireConversationAttachment {
  if (value === null || typeof value !== "object") return false;
  const attachment = value as Record<string, unknown>;
  return (
    typeof attachment.attachment_id === "string" &&
    typeof attachment.conversation_id === "string" &&
    typeof attachment.upload_id === "string" &&
    typeof attachment.file_name === "string" &&
    typeof attachment.content_type === "string" &&
    typeof attachment.size_bytes === "number" &&
    typeof attachment.digest === "string" &&
    typeof attachment.untrusted_label === "string" &&
    typeof attachment.input_revision === "string" &&
    typeof attachment.state === "string" &&
    typeof attachment.created_at === "string"
  );
}

/** The registry-member guard (IMPL-25): one coded type/bound pair. */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/** The coded notification guards (IMPL-14): each response must carry its
 *  distinguishing coded fields to be presented as data; anything else
 *  stays unknown — never reinterpreted, never fabricated. */
function isWireNotificationGroupEntry(value: unknown): value is WireNotificationGroupEntry {
  if (value === null || typeof value !== "object") return false;
  const entry = value as Record<string, unknown>;
  if (
    typeof entry.notification_id !== "string" ||
    typeof entry.category !== "string" ||
    typeof entry.state_class !== "string" ||
    typeof entry.related_object_type !== "string" ||
    typeof entry.related_object_id !== "string" ||
    typeof entry.created_at !== "string" ||
    typeof entry.unread_count !== "number" ||
    typeof entry.repeat_count !== "number" ||
    typeof entry.presentation !== "string" ||
    !Array.isArray(entry.collapsed)
  ) {
    return false;
  }
  if (entry.object_state !== undefined && typeof entry.object_state !== "string") return false;
  return entry.collapsed.every((block) => {
    if (block === null || typeof block !== "object") return false;
    const shaped = block as Record<string, unknown>;
    return (
      typeof shaped.state_class === "string" &&
      typeof shaped.count === "number" &&
      typeof shaped.unread_count === "number" &&
      typeof shaped.newest_at === "string"
    );
  });
}

function isWireNotificationCounts(value: unknown): value is WireNotificationCountsResult {
  if (value === null || typeof value !== "object") return false;
  const counts = value as Record<string, unknown>;
  if (typeof counts.total !== "number") return false;
  if (counts.by_category === null || counts.by_category === undefined) return true;
  if (typeof counts.by_category !== "object") return false;
  return Object.values(counts.by_category).every((value) => typeof value === "number");
}

function isWireNotificationMutationResult(value: unknown): value is WireNotificationMutationResult {
  if (value === null || typeof value !== "object") return false;
  const result = value as Record<string, unknown>;
  return typeof result.updated === "number" && typeof result.unchanged === "number";
}

/** The coded knowledge guards (B1-U1): each response must carry its
 *  distinguishing published members to be presented as data; anything
 *  else stays unknown — never reinterpreted, never fabricated. The
 *  optional members are checked only where their presence distinguishes
 *  the shape; the generated types carry the full member set. */
function isWireKnowledgePack(value: unknown): value is KnowledgePack {
  if (value === null || typeof value !== "object") return false;
  const pack = value as Record<string, unknown>;
  return (
    typeof pack.pack_id === "string" &&
    typeof pack.schema_version === "number" &&
    typeof pack.title === "string" &&
    typeof pack.created_at === "string" &&
    typeof pack.updated_at === "string" &&
    typeof pack.current_version_id === "string"
  );
}

function isWireKnowledgePackList(value: unknown): value is WireKnowledgePackListResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.packs)) return false;
  // The cursor key is absent on the final page: absence is data (the end
  // marker), never an error.
  return shaped.cursor === undefined || typeof shaped.cursor === "string";
}

function isWireKnowledgeVersion(value: unknown): value is KnowledgePackVersionServed {
  if (value === null || typeof value !== "object") return false;
  const version = value as Record<string, unknown>;
  return (
    typeof version.pack_id === "string" &&
    typeof version.version_id === "string" &&
    typeof version.schema_version === "number" &&
    typeof version.pack_root_digest === "string" &&
    // The served `created_at` member (see wire.ts's honest gap note).
    typeof version.created_at === "string"
  );
}

function isWireKnowledgeVersionList(value: unknown): value is WireKnowledgeVersionListResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.versions)) return false;
  if (!shaped.versions.every((row) => isWireKnowledgeVersion(row))) return false;
  return shaped.cursor === undefined || typeof shaped.cursor === "string";
}

function isWireKnowledgeImportSourceSpec(value: unknown): value is KnowledgeImportSourceSpec {
  if (value === null || typeof value !== "object") return false;
  const spec = value as Record<string, unknown>;
  return (
    typeof spec.source_kind === "string" && typeof spec.max_bytes === "number"
  );
}

function isWireKnowledgeRegistry(value: unknown): value is WireKnowledgeRegistryResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return Array.isArray(shaped.sources) && shaped.sources.every((row) => isWireKnowledgeImportSourceSpec(row));
}

function isWireKnowledgeOperation(value: unknown): value is KnowledgeOperation {
  if (value === null || typeof value !== "object") return false;
  const op = value as Record<string, unknown>;
  return (
    typeof op.operation_id === "string" &&
    typeof op.kind === "string" &&
    typeof op.state === "string" &&
    typeof op.correlation_id === "string" &&
    typeof op.created_at === "string" &&
    typeof op.updated_at === "string"
  );
}

function isWireKnowledgeSubmitResult(value: unknown): value is KnowledgeOperationSubmitResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return typeof shaped.replayed === "boolean" && isWireKnowledgeOperation(shaped.operation);
}

function isWireKnowledgeOperationResult(value: unknown): value is WireKnowledgeOperationResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return shaped.operation !== null && typeof shaped.operation === "object" && isWireKnowledgeOperation(shaped.operation);
}

/** The coded memory guards (B2-U1): each response must carry its
 *  distinguishing published members to be presented as data; anything
 *  else stays unknown — never reinterpreted, never fabricated. The
 *  optional members are checked only where their presence distinguishes
 *  the shape; the generated types carry the full member set. The served
 *  item's `pinned` flag rides every served item (see wire.ts's served-
 *  shape note — B2-A1 §2.6 item 3). */
function isWireMemoryItem(value: unknown): value is MemoryItemServed {
  if (value === null || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return (
    typeof item.memory_item_id === "string" &&
    typeof item.scope === "string" &&
    typeof item.origin === "string" &&
    typeof item.lifecycle_state === "string" &&
    typeof item.content === "string" &&
    // The served `pinned` flag (this Core's served pin-register position).
    typeof item.pinned === "boolean" &&
    typeof item.created_at === "string" &&
    typeof item.updated_at === "string"
  );
}

function isWireMemoryList(value: unknown): value is WireMemoryListResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.items)) return false;
  if (!shaped.items.every((row) => isWireMemoryItem(row))) return false;
  // The cursor key is absent on the final page: absence is data (the end
  // marker), never an error.
  return shaped.cursor === undefined || typeof shaped.cursor === "string";
}

function isWireMemoryMutationResult(value: unknown): value is WireMemoryMutationResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.memory_item !== null &&
    typeof shaped.memory_item === "object" &&
    isWireMemoryItem(shaped.memory_item) &&
    typeof shaped.operation_id === "string"
  );
}

function isWireMemoryItemResult(value: unknown): value is { readonly memory_item: MemoryItemServed } {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.memory_item !== null &&
    typeof shaped.memory_item === "object" &&
    isWireMemoryItem(shaped.memory_item)
  );
}

function isWireMemoryDeleteResult(value: unknown): value is WireMemoryDeleteResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    typeof shaped.memory_item_id === "string" &&
    typeof shaped.outcome_state === "string" &&
    typeof shaped.operation_id === "string"
  );
}

function isWireMemoryInclusionResult(value: unknown): value is WireMemoryInclusionResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    typeof shaped.memory_item_id === "string" &&
    typeof shaped.request_ref === "string" &&
    typeof shaped.included === "boolean" &&
    typeof shaped.operation_id === "string"
  );
}

function isWireMemoryExportResult(value: unknown): value is WireMemoryExportResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    typeof shaped.export_operation_id === "string" &&
    typeof shaped.count === "number" &&
    typeof shaped.generated_at === "string" &&
    typeof shaped.truncated === "boolean" &&
    Array.isArray(shaped.items) &&
    shaped.items.every((row: unknown) => isWireMemoryItem(row))
  );
}

function isWireMemoryOperation(value: unknown): value is MemoryOperation {
  if (value === null || typeof value !== "object") return false;
  const op = value as Record<string, unknown>;
  return (
    typeof op.operation_id === "string" &&
    typeof op.kind === "string" &&
    typeof op.state === "string" &&
    typeof op.created_at === "string" &&
    typeof op.updated_at === "string"
  );
}

function isWireMemoryOperationResult(value: unknown): value is WireMemoryOperationResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.operation !== null && typeof shaped.operation === "object" && isWireMemoryOperation(shaped.operation)
  );
}

/** The coded context guards (B3-U1): each response must carry its
 *  published shape's required members (the closed registers' string
 *  values ride as strings; an unknown reported register value renders
 *  verbatim downstream — never mapped). The save/compare result guards
 *  exist for the un-gated producer's day; no instance can be served while
 *  the conditional gates stand. */
function isWireContextOperation(value: unknown): value is ContextOperation {
  if (value === null || typeof value !== "object") return false;
  const op = value as Record<string, unknown>;
  return (
    typeof op.operation_id === "string" &&
    typeof op.kind === "string" &&
    typeof op.state === "string" &&
    typeof op.correlation_id === "string" &&
    typeof op.created_at === "string" &&
    typeof op.updated_at === "string"
  );
}

function isWireContextOperationResult(value: unknown): value is WireContextOperationResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.operation !== null && typeof shaped.operation === "object" && isWireContextOperation(shaped.operation)
  );
}

function isWireContextInclusionResult(value: unknown): value is WireContextInclusionResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (typeof shaped.request_ref !== "string" || !Array.isArray(shaped.decisions)) return false;
  return shaped.decisions.every(
    (row: unknown) =>
      row !== null &&
      typeof row === "object" &&
      typeof (row as Record<string, unknown>).memory_item_id === "string" &&
      typeof (row as Record<string, unknown>).included === "boolean",
  );
}

function isWireContextSnapshotSaveResult(value: unknown): value is WireContextSnapshotSaveResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.operation !== null && typeof shaped.operation === "object" && isWireContextOperation(shaped.operation) &&
    typeof shaped.replayed === "boolean"
  );
}

function isWireContextSnapshotCompareResult(value: unknown): value is WireContextSnapshotCompareResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    typeof shaped.baseline_snapshot_id === "string" &&
    typeof shaped.candidate_snapshot_id === "string" &&
    shaped.comparison !== null &&
    typeof shaped.comparison === "object"
  );
}

/** The coded workflow guards (B4-U1): each response must carry its
 *  distinguishing published members to be presented as data; anything
 *  else stays unknown — never reinterpreted, never fabricated. The
 *  optional members are checked only where their presence distinguishes
 *  the shape; the generated types carry the full member set. */
function isWireWorkflowDefinition(value: unknown): value is WorkflowDefinition {
  if (value === null || typeof value !== "object") return false;
  const definition = value as Record<string, unknown>;
  return (
    typeof definition.definition_id === "string" && typeof definition.created_at === "string"
  );
}

function isWireWorkflowRevision(value: unknown): value is WorkflowRevision {
  if (value === null || typeof value !== "object") return false;
  const revision = value as Record<string, unknown>;
  return typeof revision.revision_ref === "string" && typeof revision.created_at === "string";
}

function isWireWorkflowList(value: unknown): value is WireWorkflowListResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.definitions)) return false;
  if (!shaped.definitions.every((row) => isWireWorkflowDefinition(row))) return false;
  // The cursor key is absent on the final page: absence is data (the end
  // marker), never an error.
  return shaped.next_cursor === undefined || typeof shaped.next_cursor === "string";
}

function isWireWorkflowInspectResult(value: unknown): value is WireWorkflowInspectResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (
    shaped.definition === null ||
    typeof shaped.definition !== "object" ||
    !isWireWorkflowDefinition(shaped.definition)
  ) {
    return false;
  }
  if (!Array.isArray(shaped.revisions)) return false;
  return shaped.revisions.every((row) => isWireWorkflowRevision(row));
}

function isWireWorkflowRunHistoryEntry(value: unknown): value is WorkflowRunHistoryEntry {
  if (value === null || typeof value !== "object") return false;
  const entry = value as Record<string, unknown>;
  // `linkage_state` is the closed register `linked | unavailable`; an
  // unknown reported value renders verbatim downstream — never mapped.
  return typeof entry.run_ref === "string" && typeof entry.linkage_state === "string";
}

function isWireWorkflowRunHistory(value: unknown): value is WireWorkflowRunHistoryResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.entries)) return false;
  return shaped.entries.every((row) => isWireWorkflowRunHistoryEntry(row));
}

function isWireWorkflowState(value: unknown): value is WorkflowDefinitionState {
  if (value === null || typeof value !== "object") return false;
  const state = value as Record<string, unknown>;
  return typeof state.definition_id === "string";
}

function isWireWorkflowStateResult(value: unknown): value is WireWorkflowStateResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.definition_state !== null &&
    typeof shaped.definition_state === "object" &&
    isWireWorkflowState(shaped.definition_state)
  );
}

// The B4-U2 write-result guards (the UI-30 resolution's wired members):
// each response must carry its distinguishing published members to be
// presented as data; anything else stays unknown — never reinterpreted,
// never fabricated.
function isWireWorkflowCreateResult(value: unknown): value is WireWorkflowCreateResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    shaped.definition !== null &&
    typeof shaped.definition === "object" &&
    isWireWorkflowDefinition(shaped.definition) &&
    typeof shaped.replayed === "boolean"
  );
}

function isWireWorkflowValidationResult(value: unknown): value is WireWorkflowValidateResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  // The exact-object label + the four typed arrays (the closed issue-class
  // register rides each issue; unknown reported values render verbatim
  // downstream — never mapped, never invented).
  return (
    typeof shaped.evaluated_object_ref === "string" &&
    Array.isArray(shaped.blocking_errors) &&
    Array.isArray(shaped.warnings) &&
    Array.isArray(shaped.permission_gaps) &&
    Array.isArray(shaped.schema_compatibility)
  );
}

function isWireWorkflowSaveRevisionResult(
  value: unknown,
): value is WireWorkflowSaveRevisionResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return typeof shaped.revision_ref === "string" && typeof shaped.replayed === "boolean";
}

/** The coded plugin guards (B5-U1): each response must carry its
 *  distinguishing published members to be presented as data; anything
 *  else stays unknown — never reinterpreted, never fabricated. The
 *  optional members are checked only where their presence distinguishes
 *  the shape; the generated types carry the full member set. The manifest
 *  and package members are OPTIONAL on the served record and render their
 *  honest absence where the Core serves none (B5-A1 §2.4 gaps 3 and 6) —
 *  an absent member is never synthesized. */
function isWirePluginManifest(value: unknown): value is PluginManifest {
  if (value === null || typeof value !== "object") return false;
  const manifest = value as Record<string, unknown>;
  return (
    typeof manifest.manifest_version === "string" &&
    typeof manifest.plugin_id === "string" &&
    typeof manifest.plugin_version === "string"
  );
}

function isWirePluginPackage(value: unknown): value is PluginPackage {
  if (value === null || typeof value !== "object") return false;
  const pkg = value as Record<string, unknown>;
  // `package_ref` is the required member (the Core-minted package
  // identity); the verification members are optional and render only as
  // the Core returns them (the verdict law — never computed UI-side).
  return typeof pkg.package_ref === "string";
}

function isWirePlugin(value: unknown): value is Plugin {
  if (value === null || typeof value !== "object") return false;
  const plugin = value as Record<string, unknown>;
  // `lifecycle_state` is the Core-returned authoritative state; the
  // five served values are the Core's own selection (B5-A1 §2.4 gap 4) —
  // an unknown reported value renders verbatim downstream, never mapped.
  if (typeof plugin.plugin_id !== "string" || typeof plugin.lifecycle_state !== "string") return false;
  if (
    plugin.manifest !== undefined &&
    plugin.manifest !== null &&
    !isWirePluginManifest(plugin.manifest)
  ) {
    return false;
  }
  if (
    plugin.package !== undefined &&
    plugin.package !== null &&
    !isWirePluginPackage(plugin.package)
  ) {
    return false;
  }
  return true;
}

function isWirePluginList(value: unknown): value is WirePluginListResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  if (!Array.isArray(shaped.plugins)) return false;
  if (!shaped.plugins.every((row) => isWirePlugin(row))) return false;
  // The cursor key is absent on the final page: absence is data (the end
  // marker), never an error.
  return shaped.next_cursor === undefined || typeof shaped.next_cursor === "string";
}

function isWirePluginState(value: unknown): value is PluginState {
  if (value === null || typeof value !== "object") return false;
  const state = value as Record<string, unknown>;
  return typeof state.plugin_id === "string" && typeof state.lifecycle_state === "string";
}

function isWirePluginStateResult(value: unknown): value is WirePluginStateResult {
  // The state endpoint's payload IS the published `PluginState` record
  // (no wrapper member — the family's authoritative state read).
  return isWirePluginState(value);
}

/** The B5-U2 mutation-result guards: each response must carry its
 *  distinguishing published members to be presented as data (the
 *  coded-shape guard class). The honest `replayed` flag rides both
 *  results (the C1-07 class: a replay returns the SAME recorded outcome,
 *  never a second mutation). */
function isWirePluginInstallResult(value: unknown): value is WirePluginInstallResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return typeof shaped.install_record_ref === "string" && typeof shaped.replayed === "boolean";
}

function isWirePluginMutationResult(value: unknown): value is WirePluginMutationResult {
  if (value === null || typeof value !== "object") return false;
  const shaped = value as Record<string, unknown>;
  return (
    typeof shaped.plugin_id === "string" &&
    typeof shaped.operation_ref === "string" &&
    typeof shaped.replayed === "boolean"
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

    // The C-4 logout/revocation commands (IMPL-23; the S9-adopted P-S9-02
    // class as implemented). The truthfulness rules above apply unchanged:
    // a lost response is `unknown` — NEVER proof that the session ended
    // (the P1.8 §6.2 failure law: the client never assumes the session
    // died because the command failed), never a re-issue, never an
    // auto-retry (the Open-idempotency law). A typed refusal is surfaced
    // verbatim and the held token stays held. Only the surface that owns
    // the dedicated logout flow (R-13: Settings → Security) discards the
    // token, and only on a confirmed data outcome. The paths are the
    // adapter's own — non-canonical (wire.ts's honest boundary).

    /** `POST /v1/session/logout` — the per-token logout command (the
     *  dedicated flow's Core command; R-13). The presented session
     *  credential is the token revoked; the typed refusal carries the
     *  coded session reason. No body. */
    sessionLogout(): Promise<TransportOutcome<WireSessionLogoutResult>> {
      return call<WireSessionLogoutResult>("/v1/session/logout", post(""), (body) => {
        if (body === null || typeof body !== "object") return false;
        return (body as Record<string, unknown>).revoked === true;
      });
    },

    /** `POST /v1/session/revoke-all` — the credential-change revocation
     *  command (the coded C2-08 lock-takeover + credential-change class):
     *  every live session of the instance invalidated, including the
     *  presented one; the response carries the post-effect live count
     *  (safe metadata, never token material). No UI control consumes this
     *  yet — its recorded trigger classes are Core-side (credential
     *  change, lock takeover) and no such surface exists; the command is
     *  delivered for the frozen contract, never wired to an invented
     *  control. No body. */
    sessionRevokeAll(): Promise<TransportOutcome<WireSessionRevokeAllResult>> {
      return call<WireSessionRevokeAllResult>("/v1/session/revoke-all", post(""), (body) => {
        if (body === null || typeof body !== "object") return false;
        return typeof (body as Record<string, unknown>).live_sessions === "number";
      });
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

    /** `GET /v1/jobs/{id}/retry-eligibility` — the F9 family's frozen
     *  retry-eligibility read (IMPL-20): exactly the four typed members
     *  inside the retry_eligibility wrapper. The verdict is the Core's
     *  own report — the M-7 control renders from THIS read or not at
     *  all, never from a client-side guess (P-S7-02). The read mutates
     *  nothing and executes nothing: an allow verdict authorizes no
     *  execution. A lost response is `unknown` — the control stays
     *  ABSENT while the read is unavailable (the truthful state, the
     *  M-5 pattern); this method never retries on its own. */
    jobRetryEligibility(jobID: string): Promise<TransportOutcome<WireRetryEligibilityReadResult>> {
      return call<WireRetryEligibilityReadResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/retry-eligibility`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (shaped.retry_eligibility === null || typeof shaped.retry_eligibility !== "object") {
            return false;
          }
          const read = shaped.retry_eligibility as Record<string, unknown>;
          return (
            typeof read.eligible === "boolean" &&
            typeof read.risk_statement === "string" &&
            typeof read.idempotency_class === "string" &&
            typeof read.reconciliation_required === "boolean"
          );
        },
      );
    },

    /** `POST /v1/jobs/{id}/retry` — the M-7 retry command (IMPL-21, the
     *  F9 family's frozen semantics): a NEW Attempt of the SAME Job —
     *  never a new Job — carrying its Supplied-keyed idempotency
     *  reference. The 200 response is the Core-confirmed post-command
     *  state plus the honest `replayed` flag (the same reference
     *  re-presented returns the recorded outcome without re-applying —
     *  never regenerated; a contradicting re-issue is the typed
     *  CONFLICT_STATE). A lost response is `unknown` — S4:
     *  query-before-retry, NEVER an automatic re-issue (the same
     *  reference may be re-presented explicitly after the authoritative
     *  status query, which is the recorded replay path, not a blind
     *  retry). Retry ≠ reversal: the response never implies the prior
     *  Attempt's effects were undone. */
    retryJob(jobID: string, idempotencyReference: string): Promise<TransportOutcome<WireRetryCommandResult>> {
      const body: WireRetryCommandRequest = { idempotency_reference: idempotencyReference };
      return call<WireRetryCommandResult>(
        `/v1/jobs/${encodeURIComponent(jobID)}/retry`,
        post(JSON.stringify(body)),
        (result) => {
          if (result === null || typeof result !== "object") return false;
          const shaped = result as Record<string, unknown>;
          return typeof shaped.replayed === "boolean" && isWireJob(shaped.job);
        },
      );
    },

    /** `GET /v1/providers/{id}/capabilities` — the F2 capability
     *  discovery/status read (IMPL-22, the adopted S10 supply's
     *  P-S10-02 class as implemented): the standing per-provider
     *  capability descriptor — the Core's OWN coded capability state,
     *  without running any network probe. ADDITIVE to the M-25 test
     *  (the probe-bearing discovery surface stays exactly as coded,
     *  never replaced). The optional model value resolves the effective
     *  profile (the Core's coded ProfileFor resolution). A provider
     *  outside the wired set is the typed NOT_FOUND — never an invented
     *  descriptor, never a name-inferred capability (UI-13). A lost
     *  response is `unknown` — the descriptor is re-read explicitly,
     *  never assumed; this method never retries on its own. */
    providerCapabilities(providerID: string, model?: string): Promise<TransportOutcome<WireCapabilityDescriptorResult>> {
      const query = new URLSearchParams();
      if (model !== undefined && model !== "") query.set("model", model);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireCapabilityDescriptorResult>(
        `/v1/providers/${encodeURIComponent(providerID)}/capabilities${suffix}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (shaped.capability_descriptor === null || typeof shaped.capability_descriptor !== "object") return false;
          const descriptor = shaped.capability_descriptor as Record<string, unknown>;
          return (
            typeof descriptor.provider === "string" &&
            descriptor.profile !== null &&
            typeof descriptor.profile === "object" &&
            descriptor.capabilities !== null &&
            typeof descriptor.capabilities === "object"
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

    /** `POST /v1/connections/{id}/rename` — the connection-record rename
     *  (IMPL-17; the adopted S8 supply's P-S8-01 working candidate): the
     *  display name ONLY — never the connection identity, never the Job
     *  identity, never the execution history. `expected_updated_at` is
     *  the concurrency marker the caller read the record under: a stale
     *  marker is the typed 409 CONFLICT_STATE (the concurrent-edit
     *  guard); no method retries on its own — the recovery is the
     *  explicit re-read, never a blind re-send. NO secret field exists
     *  on this request. */
    connectionRename(connectionID: string, payload: WireConnectionRenameRequest): Promise<TransportOutcome<WireConnectionRenameResult>> {
      return call<WireConnectionRenameResult>(
        `/v1/connections/${encodeURIComponent(connectionID)}/rename`,
        post(JSON.stringify(payload)),
        isWireConnectionRecordResult,
      );
    },

    /** `POST /v1/connections/{id}/update` — the non-secret field update
     *  (IMPL-17; the adopted S8 supply's P-S8-02 working candidate):
     *  EXACTLY the coded create shape's non-secret fields. NO secret
     *  field exists on this request — a credential change stays the
     *  rotate/re-authenticate flows, and no update path ever writes a
     *  credential. The same concurrency marker and the same no-retry
     *  rule ride this method. */
    connectionUpdate(connectionID: string, payload: WireConnectionUpdateRequest): Promise<TransportOutcome<WireConnectionUpdateResult>> {
      return call<WireConnectionUpdateResult>(
        `/v1/connections/${encodeURIComponent(connectionID)}/update`,
        post(JSON.stringify(payload)),
        isWireConnectionRecordResult,
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

    /**
     * The conversation attachment surface (IMPL-25; the ADOPTED S12
     * supply, P-S12-01…06, as implemented — the working candidates they
     * are, never finalized). Exactly the five delivered attachment
     * operations, each ONE `call` — NO retry loop exists anywhere in this
     * section: a lost upload response is `unknown` and the resolution is
     * the owning surface's explicit S4-for-uploads status query (NEVER a
     * blind re-upload — the Core's replay ruling makes the
     * identity-bearing re-issue duplicate-free, but the re-issue itself
     * is always an explicit user act). The paths are the adapter's own
     * non-canonical paths (wire.ts's honest boundary). The truthfulness
     * rules above apply unchanged.
     */

    /** `GET /v1/conversations/attachments/registry` — the attachment
     *  registry read (P-S12-01): the Core's own supported types/sizes as
     *  queryable contract data. The UI renders ONLY this data; a refusal
     *  or an unavailable Core leaves the attach affordance absent or
     *  disabled-with-reason (no dead controls, no client-side list). */
    conversationAttachmentRegistry(): Promise<TransportOutcome<WireAttachmentRegistryResult>> {
      return call<WireAttachmentRegistryResult>("/v1/conversations/attachments/registry", get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (!Array.isArray(shaped.types)) return false;
        return shaped.types.every(
          (row) =>
            isPlainObject(row) &&
            typeof row.content_type === "string" &&
            typeof row.max_bytes === "number" &&
            Number.isFinite(row.max_bytes),
        );
      });
    },

    /** `POST /v1/conversations/{id}/attachments` — the identity-bearing
     *  upload (P-S12-02): the caller-supplied upload identity rides the
     *  body (this method never mints a second identity and never
     *  re-issues on its own); the input revision is echoed back on the
     *  record (P-S12-03). The typed rejections (type membership, then the
     *  per-type size bound — P-S12-04) surface as the envelope
     *  verbatim. */
    conversationAttachmentUpload(
      conversationID: string,
      payload: WireConversationAttachmentUploadRequest,
    ): Promise<TransportOutcome<WireConversationAttachmentResult>> {
      return call<WireConversationAttachmentResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/attachments`,
        post(JSON.stringify(payload)),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return isWireConversationAttachment(shaped.attachment);
        },
      );
    },

    /** `GET /v1/conversations/{id}/attachments/status?upload_id=` — the
     *  S4-for-uploads read (P-S12-02): the authoritative attachment state
     *  for one upload identity. A pure query — it never re-uploads, never
     *  mutates. A Core NOT_FOUND (including the no-leak foreign-binding
     *  case) is the typed refusal surfaced verbatim — never
     *  reinterpreted. */
    conversationAttachmentStatus(
      conversationID: string,
      uploadID: string,
    ): Promise<TransportOutcome<WireConversationAttachmentResult>> {
      const query = new URLSearchParams({ upload_id: uploadID });
      return call<WireConversationAttachmentResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/attachments/status?${query.toString()}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return isWireConversationAttachment(shaped.attachment);
        },
      );
    },

    /** `GET /v1/conversations/{id}/attachments?limit=&cursor=` — the
     *  conversation's attachments read (the card-9 list discipline): the
     *  Core's stored records in chronological order, one bounded page at
     *  a time; the cursorless page IS the final page. */
    conversationAttachmentList(
      conversationID: string,
      params: { limit?: number; cursor?: string } = {},
    ): Promise<TransportOutcome<WireConversationAttachmentListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireConversationAttachmentListResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/attachments${suffix}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          if (!Array.isArray(shaped.attachments)) return false;
          return (
            shaped.attachments.every((row) => isWireConversationAttachment(row)) &&
            (shaped.cursor === undefined || typeof shaped.cursor === "string")
          );
        },
      );
    },

    /** `POST /v1/conversations/{id}/attachments/delete` — the
     *  draft-stage delete: the body names exactly the attachment to
     *  delete (no implicit target). The confirmed 200 is the outcome; a
     *  lost response is `unknown` and the owning surface re-reads the
     *  list to learn the truth — never an assumed success. */
    conversationAttachmentDelete(
      conversationID: string,
      payload: WireConversationAttachmentDeleteRequest,
    ): Promise<TransportOutcome<WireConversationAttachmentDeleteResult>> {
      return call<WireConversationAttachmentDeleteResult>(
        `/v1/conversations/${encodeURIComponent(conversationID)}/attachments/delete`,
        post(JSON.stringify(payload)),
        (body) => body !== null && typeof body === "object",
      );
    },

    /**
     * The notification surface (IMPL-14; the ADOPTED S6 supply,
     * P-S6-01/02 — the working candidates they are, never finalized).
     * Exactly the five delivered notification operations, each ONE
     * `call` — NO retry loop exists anywhere in this section (a lost
     * response is `unknown`; the recovery is the surface's explicit
     * query — re-reading the list and counts — never a resend, and NO
     * auto-retry anywhere, P1.6). The paths are the adapter's own
     * non-canonical paths. A notification read/dismiss NEVER changes the
     * underlying Job, Activity, approval, or security record (the
     * non-mutation law, P-S6-02/P3.8 §9 — the Core pins it structurally;
     * this client only ever names notification identities). NO
     * mark-all-read method exists (not in the P-S6-02 named set), and
     * NO stream, OS-delivery, OS-permission, quiet-hours, or preference
     * member exists here (P-S6-04/05; decision 6).
     */

    // The F12 audit/activity read surface (IMPL-24; the adopted S11
    // classes as implemented). Both are READS over the append-only audit
    // chain — the truthfulness rules above apply unchanged: a lost
    // response is `unknown`, never an empty list, never a document; no
    // method retries on its own. No dismiss/delete/correction operation
    // exists anywhere on this surface (the read-only law).

    /** `GET /v1/audit?limit=&cursor=&policy_decision=` — the
     *  permission-filtered audit list (the card-9 working candidate's
     *  class). The filter values are ONLY the Core's coded allow/deny
     *  register (filter options come from the Core, never invented
     *  client-side); the cursor rides back verbatim and a cursorless
     *  response IS the final page. */
    auditList(
      params: { limit?: number; cursor?: string; policyDecision?: string } = {},
    ): Promise<TransportOutcome<WireAuditListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      if (params.policyDecision !== undefined && params.policyDecision !== "")
        query.set("policy_decision", params.policyDecision);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireAuditListResult>(`/v1/audit${suffix}`, get(), isWireAuditListResult);
    },

    /** `GET /v1/audit/export?from=&to=&limit=` — the M-32-class
     *  read-with-export (the adopted P-S11-03 class as implemented): a
     *  point-in-time read that mutates nothing and adds no
     *  tamper-resistance. The confirmed scope/range/bound ride the query
     *  — the calling surface confirms BEFORE the request (P1.6's
     *  precondition). */
    auditExport(
      params: { from?: string; to?: string; limit?: number } = {},
    ): Promise<TransportOutcome<WireAuditExportResult>> {
      const query = new URLSearchParams();
      if (params.from !== undefined && params.from !== "") query.set("from", params.from);
      if (params.to !== undefined && params.to !== "") query.set("to", params.to);
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireAuditExportResult>(`/v1/audit/export${suffix}`, get(), isWireAuditExportResult);
    },

    /** `GET /v1/notifications?category=&read_state=&cursor=` — the
     * server-side grouped list read (P-S6-01; the card-9 working
     * candidate). No limit is sent (the Core's per-surface default
     * applies); the filter values are ONLY the Core's closed-register
     * members (the surface never invents a value); the opaque cursor
     * rides back verbatim and a cursorless response IS the final page.
     * An empty page IS the honest no-notification answer. A pure read. */
    notificationList(
      params: WireNotificationsListParams = {},
    ): Promise<TransportOutcome<WireNotificationsListResult>> {
      const query = new URLSearchParams();
      if (params.category !== undefined && params.category !== "") query.set("category", params.category);
      if (params.read_state !== undefined && params.read_state !== "") query.set("read_state", params.read_state);
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size > 0 ? `?${query.toString()}` : "";
      return call<WireNotificationsListResult>(`/v1/notifications${suffix}`, get(), (body) => {
        if (body === null || typeof body !== "object") return false;
        const shaped = body as Record<string, unknown>;
        if (!Array.isArray(shaped.entries)) return false;
        return (
          shaped.entries.every((row) => isWireNotificationGroupEntry(row)) &&
          (shaped.cursor === undefined || typeof shaped.cursor === "string")
        );
      });
    },

    /** `GET /v1/notifications/counts` — the Core-supplied unread counts
     * read (P-S6-01: unread counts render only from Core-supplied state
     * — never locally counted). A pure read. */
    notificationCounts(): Promise<TransportOutcome<WireNotificationCountsResult>> {
      return call<WireNotificationCountsResult>("/v1/notifications/counts", get(), isWireNotificationCounts);
    },

    /** `POST /v1/notifications/read` — the mark-read mutation (P-S6-02):
     * the body names EXACTLY the notification identities to mutate. The
     * Core applies the set all-or-nothing and answers the typed outcome;
     * the outcome is never rendered as the confirmation by itself — the
     * surface re-reads the authoritative list and counts. A lost
     * response is `unknown` and is NEVER re-sent on its own. */
    notificationMarkRead(
      payload: WireNotificationMutationRequest,
    ): Promise<TransportOutcome<WireNotificationMutationResult>> {
      return call<WireNotificationMutationResult>(
        "/v1/notifications/read",
        post(JSON.stringify(payload)),
        isWireNotificationMutationResult,
      );
    },

    /** `POST /v1/notifications/unread` — the mark-unread mutation
     * (P-S6-02): the same discipline as mark-read exactly. */
    notificationMarkUnread(
      payload: WireNotificationMutationRequest,
    ): Promise<TransportOutcome<WireNotificationMutationResult>> {
      return call<WireNotificationMutationResult>(
        "/v1/notifications/unread",
        post(JSON.stringify(payload)),
        isWireNotificationMutationResult,
      );
    },

    /** `POST /v1/notifications/clear` — the clear mutation (P-S6-02; the
     * soft-hide working candidate: the Core retains the rows and excludes
     * them from the list and the counts — the non-mutation law bounds
     * either reading). The body names EXACTLY the notification
     * identities to clear; a lost response is `unknown` and is NEVER
     * re-sent on its own. */
    notificationClear(
      payload: WireNotificationMutationRequest,
    ): Promise<TransportOutcome<WireNotificationMutationResult>> {
      return call<WireNotificationMutationResult>(
        "/v1/notifications/clear",
        post(JSON.stringify(payload)),
        isWireNotificationMutationResult,
      );
    },

    // The knowledge surface (B1-U1; the DE-0 queue row 2): the adapter's
    // serving of the published `/knowledgepacks` family (B1-A1 §2.1) —
    // nine operations, the M-9 digest-keyed idempotency class, and the
    // family's authoritative state endpoint. The truthfulness rules above
    // apply unchanged: a lost response is `unknown` and is NEVER proof of
    // success, cancellation, or safe retry (the S0–S4 law) — an import or
    // export outcome reconciles ONLY through `knowledgeOperationStatus`
    // (a pure query — never a retry) before any resubmission.

    /** `GET /v1/knowledgepacks?limit=&cursor=` — the workspace-scoped
     *  keyset pack list. Omitted parameters are not sent (the Core's
     *  default limit applies); the cursorless page IS the final page. */
    knowledgePackList(
      params: { limit?: number; cursor?: string } = {},
    ): Promise<TransportOutcome<WireKnowledgePackListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireKnowledgePackListResult>(`/v1/knowledgepacks${suffix}`, get(), isWireKnowledgePackList);
    },

    /** `GET /v1/knowledgepacks/{id}` — one pack's authoritative state.
     *  An unknown pack is the typed NOT_FOUND surfaced verbatim — the
     *  no-leak envelope (one identical shape for every unknown target). */
    knowledgePackGet(packID: string): Promise<TransportOutcome<WireKnowledgePackResult>> {
      return call<WireKnowledgePackResult>(
        `/v1/knowledgepacks/${encodeURIComponent(packID)}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return shaped.pack !== null && typeof shaped.pack === "object" && isWireKnowledgePack(shaped.pack);
        },
      );
    },

    /** `GET /v1/knowledgepacks/{id}/versions?limit=&cursor=` — one pack's
     *  keyset version page. An unknown pack is the SAME typed NOT_FOUND. */
    knowledgeVersionList(
      packID: string,
      params: { limit?: number; cursor?: string } = {},
    ): Promise<TransportOutcome<WireKnowledgeVersionListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireKnowledgeVersionListResult>(
        `/v1/knowledgepacks/${encodeURIComponent(packID)}/versions${suffix}`,
        get(),
        isWireKnowledgeVersionList,
      );
    },

    /** `GET /v1/knowledgepacks/{id}/versions/{version_id}` — one
     *  immutable version's structural read (the canonical deep link's
     *  data). Unknown pack/version: the SAME typed NOT_FOUND. */
    knowledgeVersionGet(packID: string, versionID: string): Promise<TransportOutcome<WireKnowledgeVersionResult>> {
      return call<WireKnowledgeVersionResult>(
        `/v1/knowledgepacks/${encodeURIComponent(packID)}/versions/${encodeURIComponent(versionID)}`,
        get(),
        (body) => {
          if (body === null || typeof body !== "object") return false;
          const shaped = body as Record<string, unknown>;
          return (
            shaped.version !== null &&
            typeof shaped.version === "object" &&
            isWireKnowledgeVersion(shaped.version)
          );
        },
      );
    },

    /** `GET /v1/knowledgepacks/import/registry` — the Core's own served
     *  supported-source registry as queryable data; the read creates
     *  nothing and probes nothing. The served contents ARE the canonical
     *  values — no speculative format or connector is invented anywhere
     *  downstream (UI §14.6). */
    knowledgeImportRegistry(): Promise<TransportOutcome<WireKnowledgeRegistryResult>> {
      return call<WireKnowledgeRegistryResult>("/v1/knowledgepacks/import/registry", get(), isWireKnowledgeRegistry);
    },

    /** `POST /v1/knowledgepacks/import` — the import submit (202 =
     *  accepted for processing, never completion; the authoritative state
     *  read confirms every outcome). The M-9 class rides the body's
     *  caller-supplied idempotency key: same key + same source digest
     *  replays (`replayed` true), same key + different bytes is the 409
     *  CONFLICT_IDEMPOTENCY_KEY envelope surfaced verbatim. */
    knowledgeImportSubmit(payload: KnowledgeImportRequest): Promise<TransportOutcome<WireKnowledgeSubmitResult>> {
      return call<WireKnowledgeSubmitResult>(
        "/v1/knowledgepacks/import",
        post(JSON.stringify(payload)),
        isWireKnowledgeSubmitResult,
      );
    },

    /** `POST /v1/knowledgepacks/export` — the export submit (202 =
     *  accepted for processing). The read-with-output class: no
     *  idempotency key, safe to re-issue (each submit a new operation);
     *  an uncertain outcome reconciles by the operation ID BEFORE another
     *  copy (the S0–S4 law). */
    knowledgeExportSubmit(payload: KnowledgeExportRequest): Promise<TransportOutcome<WireKnowledgeSubmitResult>> {
      return call<WireKnowledgeSubmitResult>(
        "/v1/knowledgepacks/export",
        post(JSON.stringify(payload)),
        isWireKnowledgeSubmitResult,
      );
    },

    /** `GET /v1/knowledgepacks/operations/{id}` — the family's
     *  authoritative state endpoint (a pure query — never a retry): the
     *  lost/uncertain outcome lookup (the S0–S4 surface). Events are
     *  never the source of truth; this endpoint is. */
    knowledgeOperationStatus(operationID: string): Promise<TransportOutcome<WireKnowledgeOperationResult>> {
      return call<WireKnowledgeOperationResult>(
        `/v1/knowledgepacks/operations/${encodeURIComponent(operationID)}`,
        get(),
        isWireKnowledgeOperationResult,
      );
    },

    /** `POST /v1/knowledgepacks/operations/{id}/cancel` — the
     *  cancellation REQUEST (202 = accepted for processing; the J-5 rule:
     *  the request is never displayed as the confirmation — the state
     *  endpoint's terminal `cancelled` state is). A terminal operation is
     *  the coded 409 CONFLICT_STATE refusal surfaced verbatim. */
    knowledgeOperationCancel(operationID: string): Promise<TransportOutcome<WireKnowledgeOperationResult>> {
      return call<WireKnowledgeOperationResult>(
        `/v1/knowledgepacks/operations/${encodeURIComponent(operationID)}/cancel`,
        post("{}"),
        isWireKnowledgeOperationResult,
      );
    },

    // The memory surface (B2-U1; the DE-0 queue row 4): the adapter's
    // serving of the published `/memories` family (B2-A1 §2.1) — twelve
    // operations, the M-17/M-16 idempotency classes, the M-18
    // request-scoped decision, the M-32 read-with-output export, and the
    // family's authoritative state endpoint. The truthfulness rules above
    // apply unchanged: a lost response is `unknown` and is NEVER proof of
    // success, failure, or safe retry (the S0–S4 law) — a lost mutation
    // reconciles ONLY through `memoryOperationStatus` (a pure query) by
    // the operation ID, or the item's inspect read by the item identity,
    // BEFORE any retry.

    /** `GET /v1/memories?limit=&scope=&state=&cursor=` — the scope- and
     *  review-filtered keyset item list. The `scope`/`state` filter
     *  values are the Core's closed registers (unknown values are the
     *  Core's typed refusal, never mapped); the cursorless page IS the
     *  final page; removed items are not served. */
    memoryList(
      params: { limit?: number; scope?: string; state?: string; cursor?: string } = {},
    ): Promise<TransportOutcome<WireMemoryListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.scope !== undefined && params.scope !== "") query.set("scope", params.scope);
      if (params.state !== undefined && params.state !== "") query.set("state", params.state);
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireMemoryListResult>(`/v1/memories${suffix}`, get(), isWireMemoryList);
    },

    /** `POST /v1/memories` — the add member (the M-17 creation class).
     *  The Core mints the memory_item_id — the body never carries one.
     *  The BINDING secrets law applies Core-side: a body attempting
     *  secret storage is refused MEMORY_SECRETS_REJECTED before any
     *  accepted operation. A confirmed 200 carries the Core-confirmed
     *  recorded item; a lost response is NEVER proof of either — it
     *  reconciles through the M-17 keyed replay (the SAME key +
     *  correlation re-issued explicitly) or the authoritative reads,
     *  never a blind re-add. */
    memoryAdd(payload: MemoryAddRequest): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>("/v1/memories", post(JSON.stringify(payload)), isWireMemoryMutationResult);
    },

    /** `GET /v1/memories/{id}` — the inspect read (a pure query, never a
     *  mutation): the item's authoritative state and the item-state half
     *  of the unknown-outcome lookup. Unknown and out-of-scope items are
     *  the SAME typed 404 surfaced verbatim (the no-leak rule). */
    memoryItemGet(itemID: string): Promise<TransportOutcome<{ readonly memory_item: MemoryItemServed }>> {
      return call<{ readonly memory_item: MemoryItemServed }>(
        `/v1/memories/${encodeURIComponent(itemID)}`,
        get(),
        isWireMemoryItemResult,
      );
    },

    /** `POST /v1/memories/{id}` — the edit/correct member (the M-16
     *  revision class, provenance-preserving): the corrected content
     *  prepared against the item's current state reference; a stale
     *  reference is 409 MEMORY_CONFLICT (reconcile by re-reading the
     *  item's authoritative state, never overwrite); a non-eligible item
     *  is 409 MEMORY_NOT_EDITABLE (the Core owns eligibility). */
    memoryEdit(itemID: string, payload: MemoryEditRequest): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>(
        `/v1/memories/${encodeURIComponent(itemID)}`,
        post(JSON.stringify(payload)),
        isWireMemoryMutationResult,
      );
    },

    /** `POST /v1/memories/{id}/delete` — the delete member (the M-16
     *  destructive class). The keying body is OPTIONAL on the served
     *  carrier (B2-A1 §2.6 item 1); this client sends the keyed form so a
     *  lost destructive response's keyed retry reconciles through the
     *  M-16 replay. Only a confirmed 200 carries the deletion truth
     *  (the F11 law) with its Core-confirmed retention caveat. */
    memoryDelete(
      itemID: string,
      payload: { idempotency_key: string; correlation_id: string },
    ): Promise<TransportOutcome<WireMemoryDeleteResult>> {
      return call<WireMemoryDeleteResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/delete`,
        post(JSON.stringify(payload)),
        isWireMemoryDeleteResult,
      );
    },

    /** `POST /v1/memories/{id}/forget` — the forget member (the
     *  OPEN-distinctness member; the M-16 destructive discipline governs
     *  regardless). The same optional-body carrier; the keyed form is
     *  sent for the same reconcile path. The forget/delete distinctness
     *  is Core-defined and OPEN — the confirmed outcome carries the
     *  truth, never a client assertion. */
    memoryForget(
      itemID: string,
      payload: { idempotency_key: string; correlation_id: string },
    ): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/forget`,
        post(JSON.stringify(payload)),
        isWireMemoryMutationResult,
      );
    },

    /** `POST /v1/memories/{id}/approve` — the explicit user review action.
     *  The request is never the confirmation (the J-5 rule); the response's
     *  Core-confirmed item state is the truth. A non-reviewable item is
     *  409 MEMORY_REVIEW_STATE_CONFLICT surfaced verbatim. */
    memoryApprove(itemID: string, payload: MemoryReviewRequest): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/approve`,
        post(JSON.stringify(payload)),
        isWireMemoryMutationResult,
      );
    },

    /** `POST /v1/memories/{id}/reject` — the explicit user review action;
     *  the rejected item's disposition follows the Core's semantics —
     *  never a silent client-side deletion. */
    memoryReject(itemID: string, payload: MemoryReviewRequest): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/reject`,
        post(JSON.stringify(payload)),
        isWireMemoryMutationResult,
      );
    },

    /** `POST /v1/memories/{id}/pin` — the pin/unpin member (one member
     *  operation covering both directions via the body's `pinned`); the
     *  Core-confirmed pin state rides the response's item, never a
     *  client assumption. */
    memoryPin(itemID: string, payload: MemoryPinRequest): Promise<TransportOutcome<WireMemoryMutationResult>> {
      return call<WireMemoryMutationResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/pin`,
        post(JSON.stringify(payload)),
        isWireMemoryMutationResult,
      );
    },

    /** `POST /v1/memories/{id}/inclusion` — the per-request-inclusion
     *  member (the M-18 class): a per-request decision, NOT an item-state
     *  change, with no global effect; it applies ONLY to the stated
     *  request/scope carried by request_ref. */
    memoryInclusion(
      itemID: string,
      payload: MemoryInclusionRequest,
    ): Promise<TransportOutcome<WireMemoryInclusionResult>> {
      return call<WireMemoryInclusionResult>(
        `/v1/memories/${encodeURIComponent(itemID)}/inclusion`,
        post(JSON.stringify(payload)),
        isWireMemoryInclusionResult,
      );
    },

    /** `POST /v1/memories/export` — the export member (the M-32
     *  read-with-output class): mutates nothing, no item-state change,
     *  none-required idempotency (every re-issue mints a new operation);
     *  the coded retry class is safe-retry. Secrets are excluded by
     *  default Core-side (the binding law); the honest truncation flag
     *  rides the result. */
    memoryExport(payload: MemoryExportRequest): Promise<TransportOutcome<WireMemoryExportResult>> {
      return call<WireMemoryExportResult>("/v1/memories/export", post(JSON.stringify(payload)), isWireMemoryExportResult);
    },

    /** `GET /v1/memories/operations/{id}` — the family's authoritative
     *  state endpoint (a pure query — never a retry): the operation-ID
     *  half of the unknown-outcome lookup. Events are never the source of
     *  truth; this endpoint is. The unknown operation rides the register's
     *  typed NOT_FOUND (no Memory operation-not-found domain code exists
     *  in the published register — B2-A1 §2.6 item 12). */
    memoryOperationStatus(operationID: string): Promise<TransportOutcome<WireMemoryOperationResult>> {
      return call<WireMemoryOperationResult>(
        `/v1/memories/operations/${encodeURIComponent(operationID)}`,
        get(),
        isWireMemoryOperationResult,
      );
    },

    // The context surface (B3-U1; the DE-0 queue row 5): the adapter's
    // serving of the published `/context` family (B3-A1 §2.1) — the M-18
    // request-scoped inclusion decision (unconditional), the two
    // CONDITIONAL commands (both serving their typed unmet-condition
    // refusals while their un-gate preconditions stand — the owner's
    // PROPOSED decisions, unresolved), and the family's authoritative
    // state endpoint. The truthfulness rules above apply unchanged: a
    // lost response is `unknown` and is NEVER proof of success or safe
    // retry (the S0–S4 law) — reconcile through the state endpoint by the
    // operation identity BEFORE any retry.

    /** `POST /v1/context/snapshots` — the save-snapshot member (the M-19
     *  class, CONDITIONAL on durable snapshot storage existing — the GC-3
     *  un-gate precondition, the owner's PROPOSED decision, unresolved).
     *  The typed unmet-condition refusal is the served behavior: NEVER a
     *  202, never a fabricated operation, never a fake success path. The
     *  UI hides this control ENTIRELY while the condition stands (never a
     *  dead stub — GC-3); this method exists so a typed refusal renders
     *  honestly if a direct path ever reaches it. */
    contextSnapshotSave(payload: ContextSnapshotSaveRequest): Promise<TransportOutcome<WireContextSnapshotSaveResult>> {
      return call<WireContextSnapshotSaveResult>(
        "/v1/context/snapshots",
        post(JSON.stringify(payload)),
        isWireContextSnapshotSaveResult,
      );
    },

    /** `POST /v1/context/snapshots/compare` — the snapshot-compare member
     *  (the M-20 class, READ-ONLY and CONDITIONAL on stable Core
     *  comparison — the GC-4 un-gate precondition). Where stable
     *  comparison does not exist the family answers the typed
     *  CONTEXT_COMPARE_UNSUPPORTED — the comparison-unavailable state
     *  renders honestly and no diff is ever fabricated. The UI hides this
     *  control ENTIRELY while the condition stands (never a dead stub —
     *  GC-4). */
    contextSnapshotCompare(payload: ContextSnapshotCompareRequest): Promise<TransportOutcome<WireContextSnapshotCompareResult>> {
      return call<WireContextSnapshotCompareResult>(
        "/v1/context/snapshots/compare",
        post(JSON.stringify(payload)),
        isWireContextSnapshotCompareResult,
      );
    },

    /** `POST /v1/context/inclusion` — the per-request memory-inclusion
     *  decision (the M-18 class, UNCONDITIONAL): request-scoped ONLY, no
     *  item-state change, no Memory action row, no scope broadening. The
     *  Core-confirmed decision record carries the per-item outcomes; the
     *  reason members are the Core's alone — absence is the absence (the
     *  Core-provided-only reason law), never a synthesized reason. */
    contextInclusion(payload: ContextInclusionRequest): Promise<TransportOutcome<WireContextInclusionResult>> {
      return call<WireContextInclusionResult>("/v1/context/inclusion", post(JSON.stringify(payload)), isWireContextInclusionResult);
    },

    /** `GET /v1/context/operations/{id}` — the family's authoritative
     *  state endpoint (a pure query — never a retry): the operation-ID
     *  half of the unknown-outcome lookup. Events are never the source of
     *  truth; this endpoint is. The unknown identity and the out-of-scope
     *  identity are the SAME typed 404 (CONTEXT_SNAPSHOT_NOT_FOUND — the
     *  no-leak rule). */
    contextOperationStatus(operationID: string): Promise<TransportOutcome<WireContextOperationResult>> {
      return call<WireContextOperationResult>(
        `/v1/context/operations/${encodeURIComponent(operationID)}`,
        get(),
        isWireContextOperationResult,
      );
    },

    // The workflow surface (B4-U1 reads; B4-U2 writes; the DE-0 queue row
    // 7 / GR-0 queue row 4): the adapter's serving of the published
    // `/workflows` family (B4-A1 §2.1) — the keyset list, the inspect
    // member, the run-history LINKAGE read, the family's authoritative
    // state endpoint, and (from the UI-30 resolution, GR-C2) the create
    // member (the aggregate-create class), the validate member (the
    // read-like Core-authoritative class), and the save-revision member
    // (the M-21 class). The truthfulness rules above apply unchanged: a
    // lost response is `unknown` and is NEVER proof of success or safe
    // retry (the S0–S4 law) — reconcile through the authoritative state
    // endpoint (a pure query — never a retry) by the definition/revision
    // identity BEFORE any retry. THE KEYED MINTING (the caller's duty, per
    // the published retention classes): the caller mints a FRESH
    // idempotency key + correlation ID per new write attempt and re-issues
    // the SAME pair on a lost outcome's reconcile retry — create 72 h;
    // save-revision 1 h; the validate member is the none-required read
    // class (correlation ID only, no key); the reads mint neither. THE
    // AI-PROPOSAL MEMBER: served by the Core in its honest UNMET-PRODUCER
    // state (the typed refusal — B4-A1 §2.4 gap 1) and carried by NO
    // method here and NO caller anywhere — the producer is unwired and no
    // proposal surface is fabricated. THE D-40 GATE: NO run-execution
    // method exists anywhere — the operation does not exist server-side
    // (B4-A1 §2.3's scripted absence checks) and none is typed here.

    /** `GET /v1/workflows?limit=&cursor=` — the workspace-scoped keyset
     *  definition list. Omitted parameters are not sent (the Core's
     *  default limit applies); the cursorless page IS the final page. */
    workflowList(params: { limit?: number; cursor?: string } = {}): Promise<TransportOutcome<WireWorkflowListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireWorkflowListResult>(`/v1/workflows${suffix}`, get(), isWireWorkflowList);
    },

    /** `GET /v1/workflows/{id}` — the inspect member: the definition and
     *  its immutable revision records (each reachable; the Core's history
     *  order). Unknown and out-of-scope definitions are the SAME typed 404
     *  surfaced verbatim (the no-leak rule). */
    workflowGet(definitionID: string): Promise<TransportOutcome<WireWorkflowInspectResult>> {
      return call<WireWorkflowInspectResult>(
        `/v1/workflows/${encodeURIComponent(definitionID)}`,
        get(),
        isWireWorkflowInspectResult,
      );
    },

    /** `GET /v1/workflows/{id}/run-history` — the run-history LINKAGE read
     *  (a pure read; the family's only `run`-named surface). Each entry is
     *  a LINKAGE record beside its Core-reported source revision; the
     *  honest empty page stands while the D-40 gate holds (no run
     *  producer); missing linkage renders `unavailable` — never an
     *  inferred link. The unknown identity is the SAME typed 404. */
    workflowRunHistory(definitionID: string): Promise<TransportOutcome<WireWorkflowRunHistoryResult>> {
      return call<WireWorkflowRunHistoryResult>(
        `/v1/workflows/${encodeURIComponent(definitionID)}/run-history`,
        get(),
        isWireWorkflowRunHistory,
      );
    },

    /** `GET /v1/workflows/{id}/state?revision_id=` — the family's
     *  authoritative state endpoint (a pure query — never a retry): the
     *  lost/uncertain-outcome reconcile anchor. Where the optional
     *  revision query rides, the state of that specific revision is
     *  returned; the unknown identity and the out-of-scope revision are
     *  the SAME typed 404 (the no-leak rule). Events are never the source
     *  of truth; this endpoint is. */
    workflowState(definitionID: string, revisionID?: string): Promise<TransportOutcome<WireWorkflowStateResult>> {
      const query = new URLSearchParams();
      if (revisionID !== undefined && revisionID !== "") query.set("revision_id", revisionID);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WireWorkflowStateResult>(
        `/v1/workflows/${encodeURIComponent(definitionID)}/state${suffix}`,
        get(),
        isWireWorkflowStateResult,
      );
    },

    /** `POST /v1/workflows` — the aggregate-create member (B4-U2; the
     *  UI-30 resolution, GR-C2): the caller mints the idempotency key +
     *  correlation ID (the 72 h replay window; the request identity is
     *  the fingerprint). THE CORE-MINTS LAW: the Core mints the stable
     *  definition identity — the body never carries one. The keyed
     *  ruling: the same key + the same request identity replays the
     *  recorded create (the SAME Core-minted identity, never a second
     *  one); the same key with a different request identity is 409
     *  CONFLICT_IDEMPOTENCY_KEY; a replay past the window is 409
     *  CONFLICT_STATE. A lost response is `unknown` and reconciles by the
     *  correlation ID through the keyed replay (the SAME pair re-issued —
     *  never a duplicate create) BEFORE any retry (the S0–S4 law). */
    workflowCreate(payload: WorkflowCreateRequest): Promise<TransportOutcome<WireWorkflowCreateResult>> {
      return call<WireWorkflowCreateResult>("/v1/workflows", post(JSON.stringify(payload)), isWireWorkflowCreateResult);
    },

    /** `POST /v1/workflows/validate` — the read-like validation member
     *  (B4-U2): Core-authoritative typed validation over the draft. THE
     *  NO-SILENT-SAVE LAW: it mutates NO stored revision and NEVER saves
     *  the draft. THE NONE-REQUIRED CLASS: no idempotency key is minted
     *  or sent; the correlation ID rides as the request identity (the
     *  result-reference read class; a validation that never returns is an
     *  unknown outcome — the query-before-retry law binds the uncertain
     *  state, and a re-validate is a FRESH validation, never a retry of a
     *  possibly-succeeded mutation). The result labels EXACTLY which
     *  draft it evaluated (`evaluated_object_ref` — the exact-object
     *  labeling law, RUN-LINK-2). */
    workflowValidate(payload: WorkflowValidateRequest): Promise<TransportOutcome<WireWorkflowValidateResult>> {
      return call<WireWorkflowValidateResult>(
        "/v1/workflows/validate",
        post(JSON.stringify(payload)),
        isWireWorkflowValidationResult,
      );
    },

    /** `POST /v1/workflows/{definition_id}/revisions` — the save-revision
     *  member (B4-U2; the M-21 class): the save creates a NEW immutable
     *  revision, states the new reference, and preserves the old — NO
     *  update-in-place ever. The caller mints the idempotency key +
     *  correlation ID (the 1 h replay window); the fingerprint is the
     *  correlation ID + the client's known base (`base_revision_ref`,
     *  which asserts NO protocol — the empty string is the client's known
     *  base for a definition with no revisions). The replay precedes the
     *  concurrent-revision check so a lost save's keyed retry reconciles
     *  (VRC-LOST-1). The concurrent-revision outcome is the typed 409
     *  WORKFLOW_REVISION_CONFLICT (the interim conflict rule governs —
     *  the envelope's `data.current_revision_ref` datum carries the
     *  stored side's REFERENCE only; the local draft survives client-side
     *  byte-for-byte). The save's authoritative re-check refuses a draft
     *  carrying blocking findings (400 WORKFLOW_VALIDATION_FAILED — the
     *  typed rejection stores nothing). A lost response reconciles by the
     *  authoritative definition/revision-state read BEFORE any retry —
     *  never a blind retry, never a second revision. */
    workflowSaveRevision(
      definitionID: string,
      payload: WorkflowRevisionSaveRequest,
    ): Promise<TransportOutcome<WireWorkflowSaveRevisionResult>> {
      return call<WireWorkflowSaveRevisionResult>(
        `/v1/workflows/${encodeURIComponent(definitionID)}/revisions`,
        post(JSON.stringify(payload)),
        isWireWorkflowSaveRevisionResult,
      );
    },

    // The plugin surface (B5-U1 reads; B5-U2 mutations; the DE-0 queue
    // row 9 / GR-0 queue row 2): the adapter's serving of the published
    // `/plugins` family (B5-A1 §2.1) — the keyset list, the inspect
    // member, the family's authoritative state endpoint, and (from the
    // UI-29 resolution, GR-C1) the five mutation members: install (the
    // C1-07 class), enable/disable (the M-29 class), update (the M-30
    // class), remove (the M-31 class), and revoke-grants (the G7 class).
    // The truthfulness rules above apply unchanged: a lost response is
    // `unknown` and is NEVER proof of success or safe retry (the S0–S4
    // law) — reconcile through the authoritative state endpoint (a pure
    // query — never a retry) by the plugin/package identity BEFORE any
    // retry. THE KEYED MINTING (the caller's duty, per the published
    // retention classes): the caller mints a FRESH idempotency key +
    // correlation ID per new mutation attempt and re-issues the SAME
    // pair on a lost outcome's reconcile retry — install 72 h;
    // update/remove/revoke 1 h; the enable member carries NO key members
    // (the carrier gap, B5-A1 §2.4 gap 1) and reconciles by the state
    // query. THE NO-CATALOG LAW: the install method consumes the
    // EXPLICIT user-supplied package reference (the contract's own
    // single body member) — never a catalog listing (none exists); no
    // local-package import method is typed (no served registry/upload
    // shape exists in the published family; P4.10 C-1's
    // validation+quarantine precondition is unmet). THE EXECUTION GATE:
    // NO invocation/execution method exists anywhere — the operation
    // does not exist server-side (B5-A1 §2.3's scripted absence checks)
    // and none is typed here.

    /** `GET /v1/plugins?limit=&cursor=` — the user-installed keyset
     *  plugin list (USER-INSTALLED ≠ CATALOG-LISTED, always — the list IS
     *  the user-installed list; no catalog exists). Omitted parameters
     *  are not sent (the Core's default limit applies); the cursorless
     *  page IS the final page. */
    pluginList(params: { limit?: number; cursor?: string } = {}): Promise<TransportOutcome<WirePluginListResult>> {
      const query = new URLSearchParams();
      if (params.limit !== undefined) query.set("limit", String(params.limit));
      if (params.cursor !== undefined && params.cursor !== "") query.set("cursor", params.cursor);
      const suffix = query.size === 0 ? "" : `?${query.toString()}`;
      return call<WirePluginListResult>(`/v1/plugins${suffix}`, get(), isWirePluginList);
    },

    /** `GET /v1/plugins/{plugin_id}` — the inspect member (a pure read):
     *  the installed plugin record with its Core-returned manifest and
     *  package verification states, each rendered only as the Core
     *  returns it (the verdict law — never computed UI-side). Unknown and
     *  out-of-scope plugins are the SAME typed 404 surfaced verbatim (the
     *  no-leak rule). */
    pluginGet(pluginID: string): Promise<TransportOutcome<WirePluginGetResult>> {
      return call<WirePluginGetResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}`,
        get(),
        isWirePlugin,
      );
    },

    /** `GET /v1/plugins/{plugin_id}/state` — the family's authoritative
     *  state endpoint (a pure query — never a retry): the lost/uncertain
     *  lifecycle outcome's reconcile anchor (install and update by
     *  package ID/version; remove by the package/plugin identity; enable
     *  by the plugin state; revoke by the grant/operation reference).
     *  Events are never the source of truth; this endpoint is. The
     *  unknown identity is the SAME typed 404 (the no-leak rule). */
    pluginState(pluginID: string): Promise<TransportOutcome<WirePluginStateResult>> {
      return call<WirePluginStateResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}/state`,
        get(),
        isWirePluginStateResult,
      );
    },

    /** `POST /v1/plugins` — the install member (the C1-07 class; B5-U2):
     *  the Core mints the plugin, package, install-record, and grant
     *  identities — the body never carries one. The caller mints the
     *  idempotency key + correlation ID (the 72 h replay window); the
     *  same key + the same fingerprint replays the recorded install (the
     *  SAME install-record reference, never a second one); the same key +
     *  a different fingerprint is 409 CONFLICT_IDEMPOTENCY_KEY; a replay
     *  past the window is 409 CONFLICT_STATE. A fresh install lands the
     *  plugin in its installed, NOT-yet-enabled state (activation is
     *  separate — never implied). A lost response is `unknown` and
     *  reconciles by the authoritative package ID/version through the
     *  family's state read BEFORE any retry (the S0–S4 law) — never an
     *  optimistic "Installed". The admission verdict is the Core's alone
     *  (the verdict law). */
    pluginInstall(payload: PluginInstallRequest): Promise<TransportOutcome<WirePluginInstallResult>> {
      return call<WirePluginInstallResult>("/v1/plugins", post(JSON.stringify(payload)), isWirePluginInstallResult);
    },

    /** `POST /v1/plugins/{plugin_id}/enable` — the state-toggle member
     *  (the M-29 class; B5-U2): ONE member covering both directions via
     *  the body's target state. THE CARRIER GAP (B5-A1 §2.4 gap 1): the
     *  published body carries exactly ONE member (`enabled`) — the
     *  keyed-class members its description references are UNSUPPLIED, so
     *  this client sends exactly that one member (no key is minted or
     *  sent for this member). A lost response reconciles through the
     *  authoritative state read BEFORE any retry (the Core's own
     *  reconcile law for this member) — never a local flip. The revoked
     *  states refuse through the toggle: grants-revoked rides
     *  PLUGIN_CAPABILITY_DENIED; trust-revoked rides PLUGIN_TRUST_REVOKED
     *  (the distinctness law — a revoked state is never exitable through
     *  the toggle). */
    pluginSetEnabled(pluginID: string, payload: PluginEnableRequest): Promise<TransportOutcome<WirePluginMutationResult>> {
      return call<WirePluginMutationResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}/enable`,
        post(JSON.stringify(payload)),
        isWirePluginMutationResult,
      );
    },

    /** `POST /v1/plugins/{plugin_id}/update` — the update member (the
     *  M-30 class; B5-U2): the caller mints the idempotency key +
     *  correlation ID (the 1 h replay window); the target version
     *  reference is the fingerprint's second half and the reconciliation
     *  key. THE FRESH-REVIEW LAW (the Core enforces it): ANY permission
     *  change triggers a fresh review of the changed set — the typed
     *  PLUGIN_CAPABILITY_DENIED refusal stores nothing and consumes no
     *  key (a retry never grants). THE ATOMIC UPGRADE: the prior version
     *  keeps running until the Core confirms the switch. A lost response
     *  reconciles by the authoritative package ID/version through the
     *  family's state read BEFORE any retry — never a blind retry, never
     *  an optimistic "Updated". */
    pluginUpdate(pluginID: string, payload: PluginUpdateRequest): Promise<TransportOutcome<WirePluginMutationResult>> {
      return call<WirePluginMutationResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}/update`,
        post(JSON.stringify(payload)),
        isWirePluginMutationResult,
      );
    },

    /** `POST /v1/plugins/{plugin_id}/remove` — the remove member (the
     *  M-31 class; B5-U2): the caller mints the idempotency key +
     *  correlation ID (the 1 h replay window). Core-confirmed removal;
     *  disposal follows the Core's retention — no erasure claim beyond
     *  the Core-confirmed scope. THE REMOVE-VS-REVOKE QUESTION STAYS
     *  RECORDED OPEN — no surface may assert removal and grant revocation
     *  are the same or distinct. A lost response reconciles by the
     *  authoritative package/plugin identity through the family's state
     *  read BEFORE any retry — removal is never assumed complete. */
    pluginRemove(pluginID: string, payload: PluginMutationRequest): Promise<TransportOutcome<WirePluginMutationResult>> {
      return call<WirePluginMutationResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}/remove`,
        post(JSON.stringify(payload)),
        isWirePluginMutationResult,
      );
    },

    /** `POST /v1/plugins/{plugin_id}/grants/revoke` — the revoke-grants
     *  member (the G7 class; B5-U2): the caller mints the idempotency key
     *  + correlation ID (the 1 h replay window; the grant-scope component
     *  is Core-side). Revocation blocks FUTURE invocations; the current
     *  invocation's behavior is Core-reported (no invocation path exists
     *  on this Core while the execution rows are gated — the honest
     *  absence). THE NEVER-GRANT LAW: a retry never re-grants and a
     *  denial stands until the user's own explicit act. A lost response
     *  reconciles through the family's state read BEFORE any retry — no
     *  assumption about the grant state. */
    pluginRevokeGrants(pluginID: string, payload: PluginMutationRequest): Promise<TransportOutcome<WirePluginMutationResult>> {
      return call<WirePluginMutationResult>(
        `/v1/plugins/${encodeURIComponent(pluginID)}/grants/revoke`,
        post(JSON.stringify(payload)),
        isWirePluginMutationResult,
      );
    },

    /** Discards the held token from memory (the expiry/recovery path).
     Returns the safe metadata snapshot for the truthful display. */
    discardSession(): CoreSessionMetadata | null {
      return discardHeldSecret();
    },
  };
}

export type HttpLocalTransport = ReturnType<typeof createHttpLocalTransport>;
