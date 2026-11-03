import {
  CircleCheck,
  CircleDashed,
  Hourglass,
  Mail,
  MailOpen,
  ShieldOff,
  TriangleAlert,
  Unlink,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { TranslationKey } from "../../i18n/keys";

/**
 * The Notifications surface's register display mappings (IMPL-14; the
 * ADOPTED S6 supply, P-S6-01…07 — the working candidates they are).
 *
 * The registers below are the CODED Core registers
 * (internal/domain/notification/notification.go), exactly as coded and
 * nothing more:
 * - `Category` carries ONLY the categories with a coded Phase 3 producer
 *   — exactly one (`job`, the AIJobRelay Job event contract); the
 *   provider/Core-health categories carry no coded producer and the
 *   Knowledge/Plugin categories are RESERVED (P3.8 §5.1). The register
 *   extends ADDITIVELY when a producer milestone implements it — never
 *   invented client-side.
 * - `ReadState` (the stored Core read state; the list read_state filter's
 *   vocabulary) and the four-way `PresentationState` register (unread |
 *   read | stale | permission_revoked — P3.8 §5.4) ride the payload.
 * - `StateClass` is the closed meaningful-transition register (result |
 *   failure | approval_needed) — never prompt/output content.
 *
 * Every chip is icon + text (§8.6/§7.2 — the four-way state distinction
 * is NON-COLOR: the text label always carries the state; the tone class
 * supplements with color only). An unknown future register value renders
 * a NEUTRAL unknown chip with the value verbatim — never mapped onto
 * success, failure, or any known state (UI §31.4; the machine
 * identifiers stay language-neutral, P2.4 §4.4).
 */

/** The coded notification Category register (internal/domain/notification
 * `Category`) — the closed set the category filter's vocabulary draws
 * from; filter options come from the Core only. */
export const CODED_NOTIFICATION_CATEGORIES: readonly string[] = ["job"];

/** The coded ReadState register (internal/domain/notification
 * `ReadState`) — the read-state filter's vocabulary. */
export const CODED_NOTIFICATION_READ_STATES: readonly string[] = ["unread", "read"];

/** The coded four-way PresentationState register
 * (internal/domain/notification `PresentationState`; P3.8 §5.4). The
 * fourth value (`permission_revoked`) is carried in the register and its
 * computation joins additively with a mode that has per-object
 * permissions (Team mode, D-36) — the single-user Local slice computes
 * no row to it (recorded honestly, never faked). */
export const CODED_NOTIFICATION_PRESENTATIONS: readonly string[] = [
  "unread",
  "read",
  "stale",
  "permission_revoked",
];

/** The coded StateClass register (internal/domain/notification
 * `StateClass`). */
export const CODED_NOTIFICATION_STATE_CLASSES: readonly string[] = [
  "result",
  "failure",
  "approval_needed",
];

/** The coded related-object register (internal/domain/notification):
 * the owning-domain object types the Phase 3 producers emit — exactly
 * `job` (the R-1 family's canonical destination `/jobs/:jobId`). */
export const CODED_RELATED_OBJECT_JOB = "job";

/** Membership sets for the verbatim-unknown rendering: a value inside
 * the coded register renders its mapped label; an unknown future value
 * renders the neutral unknown label plus the value verbatim. */
export const CODED_KNOWN_PRESENTATIONS: ReadonlySet<string> = new Set(CODED_NOTIFICATION_PRESENTATIONS);
export const CODED_KNOWN_STATE_CLASSES: ReadonlySet<string> = new Set(CODED_NOTIFICATION_STATE_CLASSES);

export interface NotificationPresentationChip {
  /** The machine presentation value (unknown values pass through
   * unchanged — never rewritten). */
  readonly value: string;
  /** The locale key of the user-facing label. */
  readonly labelKey: TranslationKey;
  /** A static decorative glyph; the text carries the state (§7.2). */
  readonly icon: LucideIcon;
  /** The semantic color token class supplement — never the only signal. */
  readonly tone: "neutral" | "info" | "success" | "warning" | "danger";
}

/** The four-way presentation chip (icon + text, non-color): unread and
 * read are the Core-supplied stored read state (the closed/open
 * envelope pair); stale is the unresolvable-object state (the broken
 * link); permission_revoked is the register's fourth value (the struck
 * shield) — rendered when the Core supplies it, never fabricated. */
export function notificationPresentationChip(presentation: string): NotificationPresentationChip {
  switch (presentation) {
    case "unread":
      return { value: presentation, labelKey: "notifications.presentation.unread", icon: Mail, tone: "info" };
    case "read":
      return { value: presentation, labelKey: "notifications.presentation.read", icon: MailOpen, tone: "neutral" };
    case "stale":
      return { value: presentation, labelKey: "notifications.presentation.stale", icon: Unlink, tone: "warning" };
    case "permission_revoked":
      return {
        value: presentation,
        labelKey: "notifications.presentation.permissionRevoked",
        icon: ShieldOff,
        tone: "danger",
      };
    default:
      // Unknown future presentation values render a neutral unknown —
      // never success, never failure (UI §31.4).
      return {
        value: presentation,
        labelKey: "notifications.presentation.unknown",
        icon: CircleDashed,
        tone: "neutral",
      };
  }
}

export interface NotificationStateClassChip {
  readonly value: string;
  readonly labelKey: TranslationKey;
  readonly icon: LucideIcon;
  readonly tone: "neutral" | "info" | "success" | "warning" | "danger";
}

/** The state-class chip: the meaningful transition class the Core
 * recorded (never prompt/output content). */
export function notificationStateClassChip(stateClass: string): NotificationStateClassChip {
  switch (stateClass) {
    case "result":
      return { value: stateClass, labelKey: "notifications.stateClass.result", icon: CircleCheck, tone: "success" };
    case "failure":
      return { value: stateClass, labelKey: "notifications.stateClass.failure", icon: TriangleAlert, tone: "danger" };
    case "approval_needed":
      return {
        value: stateClass,
        labelKey: "notifications.stateClass.approvalNeeded",
        icon: Hourglass,
        tone: "warning",
      };
    default:
      return {
        value: stateClass,
        labelKey: "notifications.stateClass.unknown",
        icon: CircleDashed,
        tone: "neutral",
      };
  }
}

/** The category chip label: the coded register maps to its label; an
 * unknown future category renders verbatim with the neutral unknown
 * note (the UI never synthesizes a category the Core did not emit). */
export function notificationCategoryLabelKey(category: string): TranslationKey {
  switch (category) {
    case "job":
      return "notifications.category.job";
    default:
      return "notifications.category.unknown";
  }
}
