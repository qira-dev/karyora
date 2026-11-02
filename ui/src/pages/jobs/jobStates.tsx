import { useState } from "react";
import {
  Ban,
  Circle,
  CircleAlert,
  CircleCheck,
  CircleDashed,
  CircleX,
  Clock,
  Hourglass,
  Pause,
  Play,
  RefreshCcwDot,
  Timer,
  TriangleAlert,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Jobs surface's lifecycle-state display mapping (IMPL-5; the P3.7
 * §5.2 lifecycle table consumed exactly).
 *
 * The twelve values below are the CODED Job-state registry
 * (internal/aijobrelay/states.go `jobTransitions` — the same closed
 * registry the Core's list-status filter validates against, IMPL-4).
 * Nothing is added, renamed, or collapsed:
 * - Core lifecycle names are preserved; each maps to ONE user-facing
 *   label without collapsing distinct states (CancelRequested stays
 *   distinct from CancellationPending; NeedsReconciliation is never
 *   rendered as Failed/Cancelled).
 * - The waiting reason is the Core's typed machine identifier
 *   (`internal/aijobrelay.WaitingReason`), displayed verbatim — the
 *   values are language-neutral machine identifiers (the localization
 *   boundary; P2.4 §4.4). The coded approval reason
 *   (`WaitApproval = "approval"`, the owner's card-17 register
 *   extension, IMPL-4) gets the P3.7 §5.2 approval label.
 * - An unknown future state value renders a NEUTRAL unknown chip
 *   (UI §31.4) — never mapped onto success or failure.
 *
 * The exact English strings follow the P3.7 §5.2 carried labels; their
 * final wording stays that record's review item — the non-collapse
 * discipline is what binds here.
 *
 * Every chip is icon + text (§8.6/§7.2): color is supplemental, never
 * the only state signal. The icons are static glyphs — no animation
 * without a written motion contract (P2.4 §2.5).
 */

/** The coded Job-state registry, exactly as the Core codes it
 * (internal/aijobrelay/states.go). This closed set is the list status
 * filter's vocabulary — filter options come from the Core only. */
export const CODED_JOB_STATES: readonly string[] = [
  "created",
  "queued",
  "running",
  "waiting",
  "paused",
  "cancel_requested",
  "cancellation_pending",
  "needs_reconciliation",
  "succeeded",
  "failed",
  "cancelled",
  "timed_out",
];

/** The coded WaitingReason register (internal/aijobrelay/states.go), of
 * which `approval` is the card-17 extension (IMPL-4). The register is
 * the Core's; this constant exists only so the approval reason can be
 * recognized for its P3.7 §5.2 label. The reason VALUE shown to the user
 * is always the Core's own string. */
export const CODED_WAITING_REASON_APPROVAL = "approval";

/** The states from which the Core's coded cancellation accepts a request:
 * `RequestCancel` moves a Queued Job to Cancelled directly and a
 * Running/Waiting Job to CancelRequested (internal/aijobrelay/port.go;
 * the transition table refuses every other from-state with the coded
 * CONFLICT_STATE). The Cancel control renders ONLY while the
 * authoritative state is in this set — eligibility is the Core's
 * reported state, never a client-side guess (P3.7 §6.3). */
export const CORE_CANCEL_ELIGIBLE_STATES: readonly string[] = [
  "queued",
  "running",
  "waiting",
];

export interface JobStateChip {
  /** The machine state value the Core reports (unknown values pass
   * through unchanged — never rewritten). */
  readonly value: string;
  /** The locale key of the user-facing label. */
  readonly labelKey: TranslationKey;
  /** A static decorative glyph; the text carries the state (§7.2). */
  readonly icon: LucideIcon;
  /** The semantic color token class supplement — never the only signal. */
  readonly tone: "neutral" | "info" | "success" | "warning" | "danger";
}

/** Maps one Core-reported state (and, for `waiting`, its typed reason)
 * to the display chip. Unknown state values render the neutral unknown
 * chip; unknown waiting reasons still render the Waiting chip with the
 * Core's verbatim reason shown separately. */
export function jobStateChip(state: string): JobStateChip {
  switch (state) {
    case "created":
      return { value: state, labelKey: "jobs.state.created", icon: Circle, tone: "neutral" };
    case "queued":
      return { value: state, labelKey: "jobs.state.queued", icon: Clock, tone: "info" };
    case "running":
      return { value: state, labelKey: "jobs.state.running", icon: Play, tone: "info" };
    case "waiting":
      return { value: state, labelKey: "jobs.state.waiting", icon: Hourglass, tone: "warning" };
    case "paused":
      return { value: state, labelKey: "jobs.state.paused", icon: Pause, tone: "neutral" };
    case "cancel_requested":
      // The REQUEST is never displayed as the CONFIRMATION (P3.7 §6.1):
      // this label says the request is recorded, nothing more.
      return {
        value: state,
        labelKey: "jobs.state.cancelRequested",
        icon: CircleDashed,
        tone: "warning",
      };
    case "cancellation_pending":
      // Kept DISTINCT from the request state (P3.7 §5.2): a protected
      // side effect may still be active or unknown.
      return {
        value: state,
        labelKey: "jobs.state.cancellationPending",
        icon: CircleAlert,
        tone: "warning",
      };
    case "needs_reconciliation":
      // Never labeled Failed or Cancelled merely because a connection
      // dropped (§25.1).
      return {
        value: state,
        labelKey: "jobs.state.needsReconciliation",
        icon: TriangleAlert,
        tone: "warning",
      };
    case "succeeded":
      return {
        value: state,
        labelKey: "jobs.state.succeeded",
        icon: CircleCheck,
        tone: "success",
      };
    case "failed":
      return { value: state, labelKey: "jobs.state.failed", icon: CircleX, tone: "danger" };
    case "cancelled":
      return { value: state, labelKey: "jobs.state.cancelled", icon: Ban, tone: "neutral" };
    case "timed_out":
      return { value: state, labelKey: "jobs.state.timedOut", icon: Timer, tone: "danger" };
    default:
      // Unknown future state values render a neutral unknown — never
      // success, never failure (UI §31.4).
      return {
        value: state,
        labelKey: "jobs.state.unknown",
        icon: RefreshCcwDot,
        tone: "neutral",
      };
  }
}

/** The terminal coded states (internal/aijobrelay `terminalSet`): a
 * terminal Job accepts no further transitions, so no cancel control can
 * ever be valid there. Used only as a display hint, never as authority —
 * the Core's state report is the only eligibility source. */
export function isCodedTerminalState(state: string): boolean {
  return (
    state === "succeeded" || state === "failed" || state === "cancelled" || state === "timed_out"
  );
}

/** The copy control for long machine identifiers (§8.7's copyable long
 * IDs): a real clipboard write, visible text feedback announced through
 * a polite live region (§7.5 — batched, non-focus-stealing), an
 * accessible name. The copied value is the identifier itself — never
 * secret material. All copy is passed in already localized. */
export function CopyIdButton({
  id,
  label,
  copiedLabel,
  failedLabel,
  className,
}: {
  id: string;
  label: string;
  copiedLabel: string;
  failedLabel: string;
  className?: string;
}) {
  const [feedback, setFeedback] = useState<string | null>(null);

  const copy = () => {
    setFeedback(null);
    navigator.clipboard
      .writeText(id)
      .then(() => setFeedback(copiedLabel))
      .catch(() => setFeedback(failedLabel));
  };

  return (
    <span className={className}>
      <button type="button" onClick={copy} title={label} aria-label={`${label}: ${id}`}>
        {id}
      </button>
      <span role="status">{feedback}</span>
    </span>
  );
}
