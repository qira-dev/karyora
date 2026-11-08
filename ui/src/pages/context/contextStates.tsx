import { CircleCheck, CircleX, Clock, HelpCircle } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Context surface's coded-state display mapping (B3-U1; the DE-0
 * queue row 5). Every register below is the Core's OWN closed register,
 * carried exactly as the published contract reports it — nothing is
 * added, renamed, or collapsed:
 *
 * - The closed context-operation kind register (save_snapshot) renders
 *   verbatim; an unknown reported value renders verbatim, never mapped.
 * - The closed context-operation state register (accepted | saved |
 *   failed | outcome_unknown) renders as icon + text chips (§8.6
 *   non-color). `outcome_unknown` renders as ITS OWN state, never
 *   success/failure (the S0–S4 law); `accepted` is the submit
 *   confirmation, never completion (the F6 law); `failed` carries the
 *   explicit failure members (never a suppressed warning).
 * - The Context failure codes render as DISTINCT states (each with its
 *   own explanation and recovery); an unlisted code surfaces verbatim as
 *   the stable language-neutral identifier it is.
 */

/** The closed context-operation kind register (the published contract
 *  verbatim): save_snapshot. The compare is the read-with-output class
 *  and the inclusion decision is request-scoped — neither carries a
 *  durable operation row. */
export const CODED_CONTEXT_OPERATION_KINDS: readonly string[] = ["save_snapshot"];

/** The closed context-operation state register (the published contract
 *  verbatim): accepted | saved | failed | outcome_unknown. */
export const CODED_CONTEXT_OPERATION_STATES: readonly string[] = [
  "accepted",
  "saved",
  "failed",
  "outcome_unknown",
];

export interface ContextStateChip {
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

/** Maps one Core-reported operation kind to the display label. Unknown
 *  reported values render verbatim downstream — never mapped. */
export function contextOperationKindLabelKey(kind: string): TranslationKey | null {
  switch (kind) {
    case "save_snapshot":
      return "context.state.kind.save_snapshot";
    default:
      return null;
  }
}

/** Maps one Core-reported operation state to the display chip. Unknown
 *  state values render the neutral unknown chip; `outcome_unknown`
 *  renders as ITS OWN state (never success or failure — the S0–S4 law);
 *  `accepted` is the submit confirmation (202 = accepted for processing,
 *  never completion — the F6 law). */
export function contextOperationStateChip(state: string): ContextStateChip {
  switch (state) {
    case "accepted":
      return { value: state, labelKey: "context.state.opState.accepted", icon: Clock, tone: "info" };
    case "saved":
      return { value: state, labelKey: "context.state.opState.saved", icon: CircleCheck, tone: "success" };
    case "failed":
      return { value: state, labelKey: "context.state.opState.failed", icon: CircleX, tone: "danger" };
    case "outcome_unknown":
      return { value: state, labelKey: "context.state.opState.outcome_unknown", icon: HelpCircle, tone: "warning" };
    default:
      return { value: state, labelKey: "context.state.opState.outcome_unknown", icon: HelpCircle, tone: "neutral" };
  }
}

/** The Context failure-code register → distinct per-code explanations
 *  (the family's five published codes + the published conflicts + the
 *  envelope codes this surface can meet). An unlisted code surfaces
 *  verbatim (the stable language-neutral identifier). */
export function contextFailureCodeKey(code: string): TranslationKey {
  switch (code) {
    case "CONTEXT_BUDGET_EXHAUSTED":
      return "context.state.failure.CONTEXT_BUDGET_EXHAUSTED";
    case "CONTEXT_OMISSION_REASON_UNAVAILABLE":
      return "context.state.failure.CONTEXT_OMISSION_REASON_UNAVAILABLE";
    case "CONTEXT_COMPILATION_UNAVAILABLE":
      return "context.state.failure.CONTEXT_COMPILATION_UNAVAILABLE";
    case "CONTEXT_COMPARE_UNSUPPORTED":
      return "context.state.failure.CONTEXT_COMPARE_UNSUPPORTED";
    case "CONTEXT_SNAPSHOT_NOT_FOUND":
      return "context.state.failure.CONTEXT_SNAPSHOT_NOT_FOUND";
    case "CONFLICT_IDEMPOTENCY_KEY":
      return "context.state.failure.CONFLICT_IDEMPOTENCY_KEY";
    case "CONFLICT_STATE":
      return "context.state.failure.CONFLICT_STATE";
    case "MEMORY_ITEM_NOT_FOUND":
      return "context.state.failure.MEMORY_ITEM_NOT_FOUND";
    case "NOT_FOUND":
      return "context.state.failure.NOT_FOUND";
    case "AUTHZ_DENIED":
      return "context.state.failure.AUTHZ_DENIED";
    case "INTERNAL":
      return "context.state.failure.INTERNAL";
    default:
      return "context.state.failure.unlisted";
  }
}

/** The Context failure codes → distinct per-code recoveries. */
export function contextFailureRecoveryKey(code: string): TranslationKey {
  switch (code) {
    case "CONTEXT_BUDGET_EXHAUSTED":
      return "context.state.recovery.CONTEXT_BUDGET_EXHAUSTED";
    case "CONTEXT_OMISSION_REASON_UNAVAILABLE":
      return "context.state.recovery.CONTEXT_OMISSION_REASON_UNAVAILABLE";
    case "CONTEXT_COMPILATION_UNAVAILABLE":
      return "context.state.recovery.CONTEXT_COMPILATION_UNAVAILABLE";
    case "CONTEXT_COMPARE_UNSUPPORTED":
      return "context.state.recovery.CONTEXT_COMPARE_UNSUPPORTED";
    case "CONTEXT_SNAPSHOT_NOT_FOUND":
      return "context.state.recovery.CONTEXT_SNAPSHOT_NOT_FOUND";
    case "CONFLICT_IDEMPOTENCY_KEY":
      return "context.state.recovery.CONFLICT_IDEMPOTENCY_KEY";
    case "CONFLICT_STATE":
      return "context.state.recovery.CONFLICT_STATE";
    case "MEMORY_ITEM_NOT_FOUND":
      return "context.state.recovery.MEMORY_ITEM_NOT_FOUND";
    case "NOT_FOUND":
      return "context.state.recovery.NOT_FOUND";
    case "AUTHZ_DENIED":
      return "context.state.recovery.AUTHZ_DENIED";
    case "INTERNAL":
      return "context.state.recovery.INTERNAL";
    default:
      return "context.state.recovery.unlisted";
  }
}

/** Mints a caller-side decision correlation ID (the client-minted
 *  identity the M-18 replay reconciles through — the same minting class
 *  the Memory surface's keyed members carry). */
export function mintContextCorrelationId(): string {
  return `ctx-decision-${crypto.randomUUID()}`;
}

/** Renders one optional string field as its value or the unavailable
 *  label — missing renders Unavailable, never zero, never inferred (the
 *  P4.6 C7 law). */
export function contextFieldOrUnavailable(
  value: string | undefined,
  unavailableLabel: string,
): string {
  if (value === undefined || value === "") {
    return unavailableLabel;
  }
  return value;
}

/** The chip tone's CSS class name (the semantic token class supplement —
 *  never the only signal; §8.6 non-color). */
export function chipToneClass(tone: "neutral" | "info" | "success" | "warning" | "danger"): string {
  switch (tone) {
    case "info":
      return "chipInfo";
    case "success":
      return "chipSuccess";
    case "warning":
      return "chipWarning";
    case "danger":
      return "chipDanger";
    default:
      return "chipNeutral";
  }
}
