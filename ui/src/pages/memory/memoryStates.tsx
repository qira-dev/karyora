import {
  Circle,
  CircleCheck,
  CircleX,
  Clock,
  Copy,
  Hourglass,
  PackageCheck,
  PackageOpen,
  Pencil,
  RefreshCcwDot,
  Trash2,
  TriangleAlert,
  Undo2,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { MemoryItemServed } from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";

/**
 * The Memory surface's coded-state display mapping (B2-U1; the DE-0
 * queue row 4). Every register below is the Core's OWN closed register,
 * carried exactly as the published contract reports it — nothing is
 * added, renamed, or collapsed:
 *
 * - The closed lifecycle-state register (captured | extracted |
 *   pending_review | approved | rejected | active | updated | expired |
 *   forgotten | deleted | outcome_unknown) renders as icon + text chips
 *   (§8.6 non-color). DISTINCT STATES STAY DISTINCT (P4.5 §3.4): pending
 *   review is a review state, NEVER a trust claim; expiry is DISTINCT
 *   from forgotten and deleted; `outcome_unknown` renders as ITS OWN
 *   state, never success/failure (the S0–S4 law). The forgotten/deleted
 *   OPEN distinctness is carried in the copy, never resolved by a chip.
 * - An unknown future state value renders a NEUTRAL unknown chip (UI
 *   §31.4) — never mapped onto success or failure.
 * - The scope and origin registers render AS THE CORE REPORTS them — an
 *   unknown reported value renders verbatim, never mapped to a known one
 *   (the scope-never-broadens/display laws; P4.5 §3.2 SC-4).
 * - The memory failure codes render as DISTINCT states (each with its own
 *   explanation and recovery); an unlisted code surfaces verbatim as the
 *   stable language-neutral identifier it is.
 */

/** The closed memory lifecycle-state register (the published contract
 *  verbatim). Drives display only — eligibility for anything is the
 *  Core's report, never a client-side guess. */
export const CODED_MEMORY_LIFECYCLE_STATES: readonly string[] = [
  "captured",
  "extracted",
  "pending_review",
  "approved",
  "rejected",
  "active",
  "updated",
  "expired",
  "forgotten",
  "deleted",
  "outcome_unknown",
];

/** The closed memory-scope register (the published contract verbatim).
 *  A reported scope value renders ONLY as the Core reports it; the set
 *  below is the register's own values for the filter's options and the
 *  labels' base — an unknown REPORTED value renders verbatim. */
export const CODED_MEMORY_SCOPES: readonly string[] = ["user", "workspace", "session", "policy-defined"];

/** The closed memory-origin register (the published contract verbatim):
 *  user-entered | approved-decision | imported | model-extracted. */
export const CODED_MEMORY_ORIGINS: readonly string[] = [
  "user-entered",
  "approved-decision",
  "imported",
  "model-extracted",
];

/** The closed memory-operation kind register (the published contract
 *  verbatim): add | edit | approve | reject | pin | unpin | forget |
 *  delete | export | inclusion. */
export const CODED_MEMORY_OPERATION_KINDS: readonly string[] = [
  "add",
  "edit",
  "approve",
  "reject",
  "pin",
  "unpin",
  "forget",
  "delete",
  "export",
  "inclusion",
];

/** The closed memory-operation state register (the published contract
 *  verbatim): recorded | outcome_unknown. */
export const CODED_MEMORY_OPERATION_STATES: readonly string[] = ["recorded", "outcome_unknown"];

export interface MemoryStateChip {
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

/** Maps one Core-reported lifecycle state to the display chip. Unknown
 *  state values render the neutral unknown chip; `outcome_unknown`
 *  renders as ITS OWN state (never success, failure, or deletion — the
 *  S0–S4 law). DISTINCT STATES STAY DISTINCT: pending_review is a review
 *  state (never a trust claim); expired is a retention outcome (never
 *  forgotten or deleted); forgotten and deleted each render their own
 *  register value — the chip never asserts their distinctness question
 *  either way. */
export function memoryLifecycleChip(state: string): MemoryStateChip {
  switch (state) {
    case "captured":
      return { value: state, labelKey: "memory.states.captured", icon: Copy, tone: "neutral" };
    case "extracted":
      // Model-extracted origin's entry state; rendered only as the Core
      // reports it — never a trust claim.
      return { value: state, labelKey: "memory.states.extracted", icon: PackageOpen, tone: "info" };
    case "pending_review":
      // A DISTINCT review state, NEVER a trust claim (P4.5 S1; UI §15.2).
      return { value: state, labelKey: "memory.states.pending_review", icon: Hourglass, tone: "warning" };
    case "approved":
      // Rendered only from Core confirmation of the explicit review
      // action (P4.5 S2) — never a UI-computed state.
      return { value: state, labelKey: "memory.states.approved", icon: CircleCheck, tone: "success" };
    case "rejected":
      // The Core-confirmed rejection outcome; the item's disposition
      // follows the Core's semantics — never a silent client-side
      // deletion (P4.5 S3).
      return { value: state, labelKey: "memory.states.rejected", icon: CircleX, tone: "danger" };
    case "active":
      return { value: state, labelKey: "memory.states.active", icon: Circle, tone: "success" };
    case "updated":
      // The Core-defined revision/update event's post-state (P4.5 S4);
      // provenance is never silently rewritten.
      return { value: state, labelKey: "memory.states.updated", icon: Pencil, tone: "info" };
    case "expired":
      // A retention outcome rendered as the Core reports it; DISTINCT
      // from forgotten and deleted (P4.5 S5) — never UI-computed.
      return { value: state, labelKey: "memory.states.expired", icon: Clock, tone: "warning" };
    case "forgotten":
      // Rendered as the Core returns it (P4.5 S6); the forgotten/deleted
      // distinctness is Core-defined and OPEN — this chip carries its own
      // register value and no conclusion either way.
      return { value: state, labelKey: "memory.states.forgotten", icon: Undo2, tone: "neutral" };
    case "deleted":
      // The Core-confirmed destructive outcome with its stated retention
      // caveat (P4.5 S7) — never displayed without Core confirmation.
      return { value: state, labelKey: "memory.states.deleted", icon: Trash2, tone: "danger" };
    case "outcome_unknown":
      // The S0–S4 surface: its own state, never success/failure/anything.
      return {
        value: state,
        labelKey: "memory.states.outcome_unknown",
        icon: TriangleAlert,
        tone: "warning",
      };
    default:
      // Unknown future state values render a neutral unknown — never
      // success, never failure (UI §31.4).
      return { value: state, labelKey: "memory.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** The scope label: the four register values carry their own labels; an
 *  unknown REPORTED scope renders verbatim as reported — never mapped to
 *  a known one (P4.5 §3.2 SC-4). `policy-defined` renders as the
 *  register's own value (a Core-reported policy scope, never renamed). */
export function memoryScopeLabelKey(scope: string): TranslationKey | null {
  switch (scope) {
    case "user":
      return "memory.scope.user";
    case "workspace":
      return "memory.scope.workspace";
    case "session":
      return "memory.scope.session";
    case "policy-defined":
      return "memory.scope.policyDefined";
    default:
      return null;
  }
}

/** The origin label: the four register values carry their own labels; an
 *  unknown reported origin renders verbatim (labeled honestly as the
 *  Core reports it — never relabeled silently; P4.5 §3.3 row 3). */
export function memoryOriginLabelKey(origin: string): TranslationKey | null {
  switch (origin) {
    case "user-entered":
      return "memory.origin.userEntered";
    case "approved-decision":
      return "memory.origin.approvedDecision";
    case "imported":
      return "memory.origin.imported";
    case "model-extracted":
      return "memory.origin.modelExtracted";
    default:
      return null;
  }
}

/** The memory-operation kind label (the closed register verbatim). */
export function memoryOperationKindLabelKey(kind: string): TranslationKey {
  switch (kind) {
    case "add":
      return "memory.opKind.add";
    case "edit":
      return "memory.opKind.edit";
    case "approve":
      return "memory.opKind.approve";
    case "reject":
      return "memory.opKind.reject";
    case "pin":
      return "memory.opKind.pin";
    case "unpin":
      return "memory.opKind.unpin";
    case "forget":
      return "memory.opKind.forget";
    case "delete":
      return "memory.opKind.delete";
    case "export":
      return "memory.opKind.export";
    case "inclusion":
      return "memory.opKind.inclusion";
    default:
      return "memory.states.unknown";
  }
}

/** The memory-operation state chip (the closed register: recorded |
 *  outcome_unknown; unknown values neutral). */
export function memoryOperationStateChip(state: string): MemoryStateChip {
  switch (state) {
    case "recorded":
      // A recorded, confirmed outcome — its result members carry the
      // truth (the Core's own register semantics).
      return { value: state, labelKey: "memory.opState.recorded", icon: PackageCheck, tone: "success" };
    case "outcome_unknown":
      // The S0–S4 surface: its own state, never success/failure.
      return { value: state, labelKey: "memory.states.outcome_unknown", icon: TriangleAlert, tone: "warning" };
    default:
      return { value: state, labelKey: "memory.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** The memory failure-code display mapping: each register code is its
 *  OWN distinct state with its own explanation and recovery; an unlisted
 *  code surfaces verbatim. The secrets code renders AS THE CORE RETURNS
 *  it (MEMORY_SECRETS_REJECTED — the matched material is never echoed
 *  anywhere, so no reason value is rendered from the envelope's data). */
export function memoryFailureCodeKey(code: string): TranslationKey {
  switch (code) {
    case "MEMORY_SCOPE_VIOLATION":
      return "memory.failure.MEMORY_SCOPE_VIOLATION";
    case "MEMORY_SECRETS_REJECTED":
      return "memory.failure.MEMORY_SECRETS_REJECTED";
    case "MEMORY_ITEM_NOT_FOUND":
      return "memory.failure.MEMORY_ITEM_NOT_FOUND";
    case "MEMORY_REVIEW_STATE_CONFLICT":
      return "memory.failure.MEMORY_REVIEW_STATE_CONFLICT";
    case "MEMORY_CONFLICT":
      return "memory.failure.MEMORY_CONFLICT";
    case "MEMORY_NOT_EDITABLE":
      return "memory.failure.MEMORY_NOT_EDITABLE";
    case "CONFLICT_IDEMPOTENCY_KEY":
      return "memory.failure.CONFLICT_IDEMPOTENCY_KEY";
    case "CONFLICT_STATE":
      return "memory.failure.CONFLICT_STATE";
    case "NOT_FOUND":
      return "memory.failure.NOT_FOUND";
    case "AUTHZ_DENIED":
      return "memory.failure.AUTHZ_DENIED";
    case "INTERNAL":
      return "memory.failure.INTERNAL";
    default:
      return "memory.failure.unknown";
  }
}

/** The per-code recovery line: the reconcile-class codes reconcile
 *  through the authoritative reads before any retry; the never-retry
 *  codes fix the input or the identity; the transient classes follow
 *  their published class. */
export function memoryFailureRecoveryKey(code: string): TranslationKey {
  switch (code) {
    case "MEMORY_CONFLICT":
      // Reconcile by re-reading the item's authoritative state, never
      // overwrite (the contract's own recovery).
      return "memory.failure.recoveryReconcileItem";
    case "MEMORY_ITEM_NOT_FOUND":
    case "NOT_FOUND":
      return "memory.failure.recoveryNotFound";
    case "MEMORY_SCOPE_VIOLATION":
    case "MEMORY_SECRETS_REJECTED":
    case "MEMORY_REVIEW_STATE_CONFLICT":
    case "MEMORY_NOT_EDITABLE":
    case "CONFLICT_IDEMPOTENCY_KEY":
    case "CONFLICT_STATE":
      return "memory.failure.recoveryFixInput";
    case "AUTHZ_DENIED":
      return "memory.failure.recoveryAuthz";
    case "INTERNAL":
      return "memory.failure.recoveryRetry";
    default:
      return "memory.failure.recoveryUnknown";
  }
}

/** The caller-minted M-17 add key (the caller's own creation identity;
 *  the Core mints the item identity — the body never carries one). A
 *  fresh random identity per add; the SAME key + correlation is what
 *  makes a re-issued add the M-17 replay (the reconcile path for a lost
 *  response) — never a duplicate. */
export function mintMemoryAddKey(): string {
  return `memory-add-${crypto.randomUUID()}`;
}

/** The caller-minted correlation ID (the request identity the M-17/M-16
 *  fingerprints key on; also the export's safe-retry identity). */
export function mintMemoryCorrelationId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

/** The caller-minted M-16 state-checked key (edit/approve/reject/pin/
 *  forget/delete — scoped user + item identity + scope Core-side). */
export function mintMemoryOperationKey(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

/** The item's current state reference (the M-16 edit fingerprint's state
 *  half). HONEST SERVED-TYPING NOTE (B2-A1 §2.3, recorded as this
 *  implementation's state-reference typing — never a contract member):
 *  this Core resolves the edit's prepared-against reference as the
 *  composite `lifecycleState@updatedAt` over the served item's members.
 *  The UI builds the reference from the Core-confirmed item exactly as
 *  served; a mismatch is the Core's typed MEMORY_CONFLICT, and the
 *  recovery is the re-read — never an overwrite. */
export function memoryItemStateRef(item: MemoryItemServed): string {
  return `${item.lifecycle_state}@${item.updated_at}`;
}

/** The field render law (the P4.5 §3.3 map): a member the Core carries
 *  renders as reported; an ABSENT/empty member renders Unavailable or
 *  "none reported" — never zero, never invented. */
export function fieldOrUnavailable(value: string | undefined, unavailableLabel: string): string {
  return value === undefined || value === "" ? unavailableLabel : value;
}

/** The copy control for the Core-minted identifiers (§8.7's copyable
 *  long IDs): the Jobs surface's CopyIdButton re-exported under one
 *  memory-side import path. The copied value is the identifier itself —
 *  never secret material. */
export { CopyIdButton } from "../jobs/jobStates";
