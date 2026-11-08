import {
  Ban,
  Circle,
  CircleCheck,
  CircleX,
  FileSearch,
  FileWarning,
  FolderSearch,
  Play,
  RefreshCcwDot,
  ScanSearch,
  TriangleAlert,
  Upload,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Knowledge surface's coded-state display mapping (B1-U1; the DE-0
 * queue row 2). Every value below is the Core's OWN closed register,
 * carried exactly as the published contract reports it — nothing is
 * added, renamed, or collapsed:
 *
 * - The knowledge-operation state register is the published closed
 *   eleven-value register (submitted | uploading | extracting |
 *   validating | indexing | generating | completed | rejected | failed |
 *   cancelled | outcome_unknown). The import stage vocabulary renders
 *   ONLY as the Core's state field reports it — progress is a state/step,
 *   NEVER a fabricated percentage (the no-fabricated-percentage law; no
 *   percentage exists anywhere on the served operation shape).
 * - An unknown future state value renders a NEUTRAL unknown chip (UI
 *   §31.4) — never mapped onto success or failure.
 * - The `outcome_unknown` state is the S0–S4 surface: it renders as its
 *   own state, never as success, failure, or cancellation (a lost
 *   response is never proof of anything — reconciliation goes through
 *   the authoritative state endpoint).
 * - The rejection/failure codes render as DISTINCT states (P4.4 R6),
 *   each with its own explanation and recovery action; an unlisted code
 *   surfaces verbatim as the stable language-neutral identifier it is.
 * - Every chip is icon + text (§8.6/§7.2): color is supplemental, never
 *   the only state signal.
 */

/** The closed knowledge-operation state register (the published contract
 *  verbatim). This set drives the step display only — eligibility for
 *  anything (cancellation included) is the Core's reported state, never a
 *  client-side guess. */
export const CODED_KNOWLEDGE_OPERATION_STATES: readonly string[] = [
  "submitted",
  "uploading",
  "extracting",
  "validating",
  "indexing",
  "generating",
  "completed",
  "rejected",
  "failed",
  "cancelled",
  "outcome_unknown",
];

/** The knowledge-operation kind register (import | export), carried
 *  verbatim; unknown kind values render verbatim. */
export const CODED_KNOWLEDGE_OPERATION_KINDS: readonly string[] = ["import", "export"];

/** The states in which the Core-confirmed cancellation can still apply:
 *  the non-terminal live states. The cancel control renders ONLY while
 *  the authoritative state read is in this set — a terminal operation is
 *  the Core's 409 CONFLICT_STATE refusal (the Core-confirmed state
 *  decides, never the client). */
export const KNOWLEDGE_LIVE_STATES: readonly string[] = [
  "submitted",
  "uploading",
  "extracting",
  "validating",
  "indexing",
  "generating",
];

export interface KnowledgeStateChip {
  /** The machine state value the Core reports (unknown values pass
   *  through unchanged — never rewritten). */
  readonly value: string;
  /** The locale key of the user-facing label. */
  readonly labelKey: TranslationKey;
  /** A static decorative glyph; the text carries the state (§7.2). */
  readonly icon: LucideIcon;
  /** The semantic color token class supplement — never the only signal. */
  readonly tone: "neutral" | "info" | "success" | "warning" | "danger";
}

/** Maps one Core-reported operation state to the display chip. Unknown
 *  state values render the neutral unknown chip; `outcome_unknown`
 *  renders as ITS OWN state (never success, failure, or cancellation —
 *  the S0–S4 law). */
export function knowledgeOperationChip(state: string): KnowledgeStateChip {
  switch (state) {
    case "submitted":
      return { value: state, labelKey: "knowledge.states.submitted", icon: Circle, tone: "neutral" };
    case "uploading":
      return { value: state, labelKey: "knowledge.states.uploading", icon: Upload, tone: "info" };
    case "extracting":
      return { value: state, labelKey: "knowledge.states.extracting", icon: FolderSearch, tone: "info" };
    case "validating":
      return { value: state, labelKey: "knowledge.states.validating", icon: ScanSearch, tone: "info" };
    case "indexing":
      return { value: state, labelKey: "knowledge.states.indexing", icon: FileSearch, tone: "info" };
    case "generating":
      return { value: state, labelKey: "knowledge.states.generating", icon: Play, tone: "info" };
    case "completed":
      return { value: state, labelKey: "knowledge.states.completed", icon: CircleCheck, tone: "success" };
    case "rejected":
      // The Core's typed rejection: its own distinct state (P4.4 R6),
      // never a generic failure.
      return { value: state, labelKey: "knowledge.states.rejected", icon: FileWarning, tone: "danger" };
    case "failed":
      return { value: state, labelKey: "knowledge.states.failed", icon: CircleX, tone: "danger" };
    case "cancelled":
      // Core-confirmed only (the state endpoint's terminal state is the
      // confirmation — never the 202 request).
      return { value: state, labelKey: "knowledge.states.cancelled", icon: Ban, tone: "neutral" };
    case "outcome_unknown":
      // The S0–S4 surface: its own state, never success/failure/cancel.
      return {
        value: state,
        labelKey: "knowledge.states.outcome_unknown",
        icon: TriangleAlert,
        tone: "warning",
      };
    default:
      // Unknown future state values render a neutral unknown — never
      // success, never failure (UI §31.4).
      return { value: state, labelKey: "knowledge.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** True while the authoritative state read reports a non-terminal live
 *  state — the display-only gate for the cancel control and the
 *  close-during-live note. The Core's state decides cancellation
 *  authority, never this set. */
export function isKnowledgeLiveState(state: string): boolean {
  return KNOWLEDGE_LIVE_STATES.includes(state);
}

/** The rejection/failure-code display mapping (P4.4 R6): each recorded
 *  code is its OWN distinct state with its own explanation; the recovery
 *  line is per-code class (the M-9 reconcile class vs the transient
 *  backoff class). An unlisted code surfaces verbatim. */
export function knowledgeFailureCodeKey(code: string): TranslationKey {
  switch (code) {
    case "IMPORT_TYPE_UNSUPPORTED":
      return "knowledge.failure.IMPORT_TYPE_UNSUPPORTED";
    case "IMPORT_SIZE_OVER_LIMIT":
      return "knowledge.failure.IMPORT_SIZE_OVER_LIMIT";
    case "IMPORT_MALFORMED":
      return "knowledge.failure.IMPORT_MALFORMED";
    case "IMPORT_DUPLICATE_DIGEST":
      return "knowledge.failure.IMPORT_DUPLICATE_DIGEST";
    case "IMPORT_PERMISSION_DENIED":
      return "knowledge.failure.IMPORT_PERMISSION_DENIED";
    case "IMPORT_POLICY_BLOCKED":
      return "knowledge.failure.IMPORT_POLICY_BLOCKED";
    case "IMPORT_INTEGRITY_SIGNATURE_FAILURE":
      return "knowledge.failure.IMPORT_INTEGRITY_SIGNATURE_FAILURE";
    case "IMPORT_NETWORK_INTERRUPTED":
      return "knowledge.failure.IMPORT_NETWORK_INTERRUPTED";
    case "INTERNAL":
      return "knowledge.failure.INTERNAL";
    default:
      return "knowledge.failure.unknown";
  }
}

/** The per-code recovery line: the M-9 reconcile class for the explicit
 *  failures, the recorded backoff path for the transient interruption. */
export function knowledgeFailureRecoveryKey(code: string): TranslationKey {
  return code === "IMPORT_NETWORK_INTERRUPTED"
    ? "knowledge.failure.networkRecovery"
    : "knowledge.failure.recovery";
}

/** The digest render law (P4.3 §3.6 law 4; the D-41 value-FORMAT law):
 *  a digest is a content-identity value rendered verbatim — it is NEVER
 *  displayed as truth, safety, authenticity, or verification anywhere on
 *  these surfaces. An absent digest renders Unavailable — never zero,
 *  never a placeholder. */
export function digestOrUnavailable(digest: string | undefined, unavailableLabel: string): string {
  return digest === undefined || digest === "" ? unavailableLabel : digest;
}

/** The count render law (P4.1 K1's carried discipline): a served count
 *  renders as the Core reports it (0 is a real Core-reported zero); an
 *  ABSENT member renders Unavailable — never an inferred value. */
export function countOrUnavailable(count: number | undefined, unavailableLabel: string): string {
  return count === undefined ? unavailableLabel : String(count);
}

/** The caller-minted M-9 idempotency key (the caller's own operation
 *  identity; the Core mints pack/version/operation identities, the
 *  caller supplies this key — the published request's required member).
 *  A fresh random identity per queued item; the SAME key + the SAME
 *  bytes is what makes a re-submission safely reconciled. */
export function mintIdempotencyKey(): string {
  return `knowledge-import-${crypto.randomUUID()}`;
}

/** The caller-minted export correlation ID (the read-with-output class's
 *  request identity; safe to re-issue). */
export function mintExportCorrelationId(): string {
  return `knowledge-export-${crypto.randomUUID()}`;
}

/** The copy control for the Core-minted identifiers (§8.7's copyable
 *  long IDs): the Jobs surface's CopyIdButton re-exported under one
 *  knowledge-side import path. The copied value is the identifier
 *  itself — never secret material. */
export { CopyIdButton } from "../jobs/jobStates";
