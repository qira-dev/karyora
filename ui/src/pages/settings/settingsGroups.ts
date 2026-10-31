import { Link2 } from "lucide-react";
import { Bell, CircleHelp, History, Info, KeyRound, Palette, RefreshCw, ScrollText } from "lucide-react";
import type { LucideIcon } from "lucide-react";

/**
 * The settings group registry (C-12; the accepted composition minus the
 * withheld groups).
 *
 * - The two withheld groups are ABSENT (Team & Access, UI-23; Storage,
 *   Core-conditional — P2.1 §5): no dead link, no placeholder route.
 * - Providers & Models and Memory SHORTCUTS to canonical routes are NOT
 *   rendered here: their canonical routes do not ship in this milestone
 *   (UIR-4/UIR-6), so a shortcut would be a dead control (C-22).
 * - The vertical settings-nav pattern (C-12): keyboard/tabs accessible;
 *   the active state is non-color (aria-current + weight).
 */

export type SettingsGroupLabelKey =
  | "appearance"
  | "connections"
  | "privacy"
  | "security"
  | "notifications"
  | "shortcuts"
  | "diagnostics"
  | "updates"
  | "about";

export interface SettingsGroup {
  readonly id: SettingsGroupLabelKey;
  readonly route: string;
  readonly labelKey: `groups.${SettingsGroupLabelKey}.label`;
  readonly descriptionKey: `groups.${SettingsGroupLabelKey}.description`;
  readonly icon: LucideIcon;
}

export const SETTINGS_GROUPS: readonly SettingsGroup[] = [
  {
    id: "appearance",
    route: "/settings/appearance",
    labelKey: "groups.appearance.label",
    descriptionKey: "groups.appearance.description",
    icon: Palette,
  },
  {
    id: "connections",
    route: "/settings/connections",
    labelKey: "groups.connections.label",
    descriptionKey: "groups.connections.description",
    icon: Link2,
  },
  {
    id: "privacy",
    route: "/settings/privacy",
    labelKey: "groups.privacy.label",
    descriptionKey: "groups.privacy.description",
    icon: ScrollText,
  },
  {
    id: "security",
    route: "/settings/security",
    labelKey: "groups.security.label",
    descriptionKey: "groups.security.description",
    icon: KeyRound,
  },
  {
    id: "notifications",
    route: "/settings/notifications",
    labelKey: "groups.notifications.label",
    descriptionKey: "groups.notifications.description",
    icon: Bell,
  },
  {
    id: "shortcuts",
    route: "/settings/shortcuts",
    labelKey: "groups.shortcuts.label",
    descriptionKey: "groups.shortcuts.description",
    icon: CircleHelp,
  },
  {
    id: "diagnostics",
    route: "/settings/diagnostics",
    labelKey: "groups.diagnostics.label",
    descriptionKey: "groups.diagnostics.description",
    icon: History,
  },
  {
    id: "updates",
    route: "/settings/updates",
    labelKey: "groups.updates.label",
    descriptionKey: "groups.updates.description",
    icon: RefreshCw,
  },
  {
    id: "about",
    route: "/settings/about",
    labelKey: "groups.about.label",
    descriptionKey: "groups.about.description",
    icon: Info,
  },
];
