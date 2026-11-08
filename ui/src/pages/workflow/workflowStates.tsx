import {
  CircleCheck,
  CircleDashed,
  CircleSlash,
  CircleX,
  Layers,
  NotebookPen,
  RefreshCcwDot,
  ShieldAlert,
  TriangleAlert,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Workflow surface's coded-state display mapping (B4-U1; the DE-0
 * queue row 7). Every register below is the Core's OWN closed register,
 * carried exactly as the published contract reports it — nothing is
 * added, renamed, or collapsed:
 *
 * - The closed linkage register (`linked | unavailable`) renders as icon
 *   + text chips (§8.6 non-color). `unavailable` is the honest state for
 *   a missing Run ↔ source-revision linkage — NEVER an inferred link
 *   (RUN-LINK-3); an unknown future value renders the neutral unknown
 *   chip (UI §31.4) — never mapped onto linked.
 * - The closed validation issue-class register (`blocking_error |
 *   non_blocking_warning | permission_gap | schema_compatibility`) drives
 *   the display chips only; the UI invents no error category, warning
 *   class, or count (P4.1 B19) — the Core's typed output is the truth.
 * - NO run-state register exists anywhere in this mapping: the D-40 gate
 *   withholds the Run execution, the run-history entries are LINKAGE
 *   records carrying no lifecycle vocabulary, and no lifecycle chip is
 *   invented for them (B4-A1 §2.3).
 * - The node vocabulary is NOT mapped here: no node catalogue is
 *   published (the P4.7 §3.1 catalogue is the vocabulary CLASS, every
 *   concrete type UNSUPPLIED [B4]), so no node-type label or palette
 *   entry is invented — a node type renders only as the Core's own
 *   catalogue responses report it, which no served read carries yet.
 */

/** The closed linkage register (the published contract verbatim). Drives
 *  display only — the link's truth is the Core's report, never a
 *  client-side inference. */
export const CODED_WORKFLOW_LINKAGE_STATES: readonly string[] = ["linked", "unavailable"];

/** The closed validation issue-class register (the published contract
 *  verbatim). Drives display only. */
export const CODED_WORKFLOW_ISSUE_CLASSES: readonly string[] = [
  "blocking_error",
  "non_blocking_warning",
  "permission_gap",
  "schema_compatibility",
];

export interface WorkflowStateChip {
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

/** Maps one Core-reported linkage state to the display chip. Unknown
 *  state values render the neutral unknown chip — never mapped onto
 *  `linked` (a missing linkage is the honest `unavailable` state,
 *  RUN-LINK-3; the no-invented-link law binds the display too). */
export function workflowLinkageChip(state: string): WorkflowStateChip {
  switch (state) {
    case "linked":
      // Rendered only as the Core reports the link (RUN-LINK-3) — never
      // inferred from run/revision proximity.
      return { value: state, labelKey: "workflow.states.linked", icon: CircleCheck, tone: "success" };
    case "unavailable":
      // The honest missing-linkage state — never an inferred link
      // (P4.1 B17; RUN-LINK-3).
      return { value: state, labelKey: "workflow.states.unavailable", icon: CircleSlash, tone: "neutral" };
    default:
      // Unknown future state values render a neutral unknown — never
      // mapped onto linked (UI §31.4).
      return { value: state, labelKey: "workflow.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** Maps one Core-reported validation issue class to the display chip.
 *  Unknown class values render the neutral unknown chip — the UI invents
 *  no error category or warning class (P4.1 B19). */
export function workflowIssueClassChip(issueClass: string): WorkflowStateChip {
  switch (issueClass) {
    case "blocking_error":
      // A blocking error blocks Save/Run per the Core's own law (P4.9
      // §3.2) — the chip is display only.
      return {
        value: issueClass,
        labelKey: "workflow.states.issueBlockingError",
        icon: CircleX,
        tone: "danger",
      };
    case "non_blocking_warning":
      // A non-blocking warning's acknowledgment is the user's explicit
      // act, never a timeout or a dismissed toast (VRC-VAL-2) — the chip
      // carries no acknowledgment state of its own.
      return {
        value: issueClass,
        labelKey: "workflow.states.issueNonBlockingWarning",
        icon: TriangleAlert,
        tone: "warning",
      };
    case "permission_gap":
      // The Core re-checks authoritatively; client-side permission
      // filtering is presentation, not enforcement (RUN-LINK-8).
      return {
        value: issueClass,
        labelKey: "workflow.states.issuePermissionGap",
        icon: ShieldAlert,
        tone: "warning",
      };
    case "schema_compatibility":
      return {
        value: issueClass,
        labelKey: "workflow.states.issueSchemaCompatibility",
        icon: Layers,
        tone: "info",
      };
    default:
      return { value: issueClass, labelKey: "workflow.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** The workflow-surface placeholder glyphs for the honest gap notes —
 *  decorative only; the text carries the state (§7.2). Exported so the
 *  pages never invent a glyph inline. */
export const WORKFLOW_GAP_ICONS = {
  /** The UI-30 interaction gate (the editing/revision surfaces withheld). */
  editingGate: NotebookPen,
  /** The D-40 remainder (no run execution). */
  runGate: CircleDashed,
  /** The unwired AI-proposal producer (B4-A1 §2.4 gap 1). */
  proposalGate: CircleX,
} as const;

/** The field render law (the P4.5 §3.3 map's class, applied to the
 *  Workflow members): a member the Core carries renders as reported; an
 *  ABSENT/empty member renders Unavailable — never zero, never invented. */
export function fieldOrUnavailable(value: string | undefined, unavailableLabel: string): string {
  return value === undefined || value === "" ? unavailableLabel : value;
}

/** The copy control for the Core-minted identifiers (§8.7's copyable
 *  long IDs): the Jobs surface's CopyIdButton re-exported under one
 *  workflow-side import path. The copied value is the identifier itself —
 *  never secret material. */
export { CopyIdButton } from "../jobs/jobStates";
