import { Ban, CircleCheck, CircleDashed, CircleSlash, FileWarning, PackageOpen, Pause, RefreshCcwDot, ShieldAlert } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Plugin surface's coded-state display mapping (B5-U1; the DE-0
 * queue row 9). Every register below is the Core's OWN served register,
 * carried exactly as the served record reports it — nothing is added,
 * renamed, or collapsed:
 *
 * - The five served lifecycle-state values (`installed | enabled |
 *   disabled | grants_revoked | trust_revoked`) are this Core's OWN
 *   served values under the recorded gap (the state-machine wire values
 *   are UNSUPPLIED [B5] — no register is invented in the canonical set;
 *   B5-A1 §2.4 gap 4, the served-definition class). THE DISTINCTNESS
 *   LAWS (binding, carried): DISABLED (the user's action) ≠
 *   GRANTS-REVOKED (the user's revocation of capabilities) ≠
 *   TRUST-REVOKED (the publisher/trust-root action) — three DISTINCT
 *   states, each rendered with its own chip, never merged and never
 *   shown as a milder one; INVOCATION TERMINATION ≠ PLUGIN
 *   DISABLE/REVOCATION (no invocation surface exists while the
 *   D-13/D-14 + the Core MB §10.2 runtime-proof gates stand, so no
 *   termination state can be reported — the honest absence, never a
 *   fabricated one). An unknown reported value renders the neutral
 *   unknown chip (UI §31.4) — never mapped onto a known state.
 * - The package verification members (signature/trust/rotation/
 *   revocation) are NOT mapped here: each renders ONLY as the Core
 *  returns it (the verdict law — the UI computes none), and on this Core
 *  the served package carries `package_ref` alone (no verification
 *  machinery exists — B5-A1 §2.4 gap 3), so no verification chip is
 *  invented anywhere. `Unknown` is the honest trust rendering where the
 *  Core returns no verification state — NEVER `Verified` (P4.1 P1's
 *  binding law; the trust-word discipline).
 * - NO invocation/failure-state register exists in this mapping: the
 *   five typed admission outcomes (PLUGIN_TRUST_UNKNOWN,
 *   PLUGIN_SIGNATURE_INVALID, PLUGIN_DIGEST_MISMATCH,
 *   PLUGIN_MANIFEST_INVALID, PLUGIN_ABI_INCOMPATIBLE) have NO producer
 *   on this Core (B5-A1 §2.4 gap 3) and render only as the Core returns
 *   them — never computed, never fabricated.
 */

/** The Core's OWN served lifecycle-state values (the served-definition
 *  class, B5-A1 §2.4 gap 4). Drives display only — the state's truth is
 *  the Core's report, never a client-side inference. */
export const SERVED_PLUGIN_LIFECYCLE_STATES: readonly string[] = [
  "installed",
  "enabled",
  "disabled",
  "grants_revoked",
  "trust_revoked",
];

export interface PluginStateChip {
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

/** Maps one Core-reported lifecycle state to the display chip. THE
 *  DISTINCTNESS LAWS (binding): each of the five served values renders
 *  its OWN chip — a user disable is never shown as a revocation, a
 *  revocation is never shown as a disable, and neither is ever shown as
 *  the milder installed state. Unknown state values render the neutral
 *  unknown chip — never mapped onto a known state (UI §31.4). */
export function pluginLifecycleChip(state: string): PluginStateChip {
  switch (state) {
    case "installed":
      // The installed, NOT-yet-enabled state every Core-confirmed install
      // lands in (activation is separate — never implied by this chip).
      return { value: state, labelKey: "plugin.states.installed", icon: PackageOpen, tone: "info" };
    case "enabled":
      return { value: state, labelKey: "plugin.states.enabled", icon: CircleCheck, tone: "success" };
    case "disabled":
      // DISABLED (the user's action) — DISTINCT from either revocation
      // state, never merged with them.
      return { value: state, labelKey: "plugin.states.disabled", icon: Pause, tone: "neutral" };
    case "grants_revoked":
      // GRANTS-REVOKED (the user's revocation of capabilities) — DISTINCT
      // from disabled and from trust-revoked.
      return { value: state, labelKey: "plugin.states.grantsRevoked", icon: ShieldAlert, tone: "warning" };
    case "trust_revoked":
      // TRUST-REVOKED (the publisher/trust-root action) — DISTINCT from
      // disabled and from grants-revoked; re-admission is the Core
      // policy's decision alone.
      return { value: state, labelKey: "plugin.states.trustRevoked", icon: Ban, tone: "danger" };
    default:
      // Unknown future state values render a neutral unknown — never
      // mapped onto a known state (UI §31.4).
      return { value: state, labelKey: "plugin.states.unknown", icon: RefreshCcwDot, tone: "neutral" };
  }
}

/** The plugin-surface placeholder glyphs for the honest gap notes —
 *  decorative only; the text carries the state (§7.2). Exported so the
 *  pages never invent a glyph inline. */
export const PLUGIN_GAP_ICONS = {
  /** The UI-29 trust/package gate (catalog/install/activation/removal
   *  controls withheld). */
  trustGate: ShieldAlert,
  /** The D-13/D-14 + Core MB §10.2 execution gate (no invocation surface
   *  exists; nothing is invented for it). */
  executionGate: CircleDashed,
  /** The manifest-absent gap (the served record carries no manifest —
   *  B5-A1 §2.4 gap 6). */
  manifestAbsent: FileWarning,
  /** The no-producer verification gap (the package verification members
   *  are absent on the served record — B5-A1 §2.4 gap 3). */
  verificationAbsent: CircleSlash,
} as const;

/** The field render law (the P4.5 §3.3 map's class, applied to the
 *  Plugin members): a member the Core returns renders as reported; an
 *  ABSENT/empty member renders Unavailable — never zero, never invented,
 *  never rendered as a verified/trusted value. */
export function fieldOrUnavailable(value: string | undefined, unavailableLabel: string): string {
  return value === undefined || value === "" ? unavailableLabel : value;
}

/** The copy control for the Core-minted identifiers (§8.7's copyable
 *  long IDs): the Jobs surface's CopyIdButton re-exported under one
 *  plugin-side import path. The copied value is the identifier itself —
 *  never secret material. */
export { CopyIdButton } from "../jobs/jobStates";

/** The caller-minted C1-07 install key (B5-U2; the caller's own mutation
 *  identity — the Core mints the plugin/package/install-record/grant
 *  identities, the body never carries one). A fresh random identity per
 *  NEW install attempt; the SAME key + correlation is what makes a
 *  re-issued install the C1-07 replay (the reconcile path for a lost
 *  response — the replay window is 72 h post-terminal). */
export function mintPluginInstallKey(): string {
  return `plugin-install-${crypto.randomUUID()}`;
}

/** The caller-minted correlation ID (the request identity the keyed
 *  fingerprint's first half and the unknown-outcome reconcile anchor —
 *  the S0–S4 law). One per mutation attempt; a lost outcome's reconcile
 *  retry re-issues the SAME pair. */
export function mintPluginCorrelationId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

/** The caller-minted short-window mutation key (the M-30/M-31/G7 keyed
 *  classes — update/remove/revoke-grants; the replay window is 1 h
 *  post-terminal; the key scope is workspace + package/plugin identity
 *  Core-side). The enable/disable member is NOT keyed (the carrier gap —
 *  the published body carries exactly one member). */
export function mintPluginMutationKey(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

/** The toggle-targetability law (the distinctness laws, binding): the
 *  enable/disable toggle operates on the installed/enabled/disabled class
 *  only — a revoked state is NEVER exitable through it (the Core refuses
 *  grants-revoked with PLUGIN_CAPABILITY_DENIED and trust-revoked with
 *  PLUGIN_TRUST_REVOKED). An unknown reported state value is not
 *  toggle-targetable by the client — the Core's own verdict governs, and
 *  the client invents no eligibility the Core has not reported. */
export function pluginToggleTargetable(lifecycleState: string): boolean {
  return lifecycleState === "installed" || lifecycleState === "enabled" || lifecycleState === "disabled";
}
