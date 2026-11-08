import { BookOpen, Bookmark, CircleDashed, CircleHelp, CircleUser, Filter, History, LayoutDashboard, ListTodo, MessageCircle, Network, Puzzle, SlidersHorizontal, Workflow } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type nav from "../../i18n/locales/en/nav.json";

/**
 * The frozen navigation model (P2.1 §2.0/§3; the accepted §4.1
 * composition, UI-09 ACCEPTED; UI-35 LOCKED placements).
 *
 * Rules carried here:
 * - Items = the accepted §4.1 labels/order; nothing added, removed, or
 *   reordered. The two withheld items (Work / Projects, UI-10; Team &
 *   Access, UI-23) are ABSENT — their glyphs never render.
 * - Plugins sits ONLY under EXTENSIONS; Providers & Models under
 *   BUILD & CONNECT (UI-35).
 * - Feature destinations whose screens ship in a later package render as
 *   disabled items with an inline explanation (no dead controls,
 *   C-01/§4.3) — their routes do not exist yet. As a surface ships (the
 *   Jobs surface, IMPL-5), its item activates its route target.
 * - Help & Documentation renders disabled: its TARGET is the recorded
 *   deferred item (in-app help vs external docs — P2.1 §5 row 1); the
 *   §4.1 icon/label rule still binds.
 * - One icon family (lucide-react, ISC license — the recorded build-time
 *   family selection per O-5, substitutions recorded in the P2.6 record
 *   and ui/README.md); icon shape + text/state — never color alone —
 *   distinguishes a route.
 */

type NavItemLabel = keyof typeof nav["items"];

export type NavLabelKey = `items.${NavItemLabel}`;

export interface NavItem {
  readonly id: string;
  readonly labelKey: NavLabelKey;
  readonly icon: LucideIcon;
  /** The target route when the destination ships in this milestone. */
  readonly route?: string;
  /** True when the destination is a later package (disabled + reason). */
  readonly disabled?: boolean;
}

export interface NavGroup {
  readonly id: "main" | "buildAndConnect" | "extensions" | "operate";
  readonly labelKey: `groups.${NavGroupKey}`;
  readonly items: readonly NavItem[];
}

type NavGroupKey = "main" | "buildAndConnect" | "extensions" | "operate";

export const NAV_GROUPS: readonly NavGroup[] = [
  {
    id: "main",
    labelKey: "groups.main",
    items: [
      { id: "overview", labelKey: "items.overview", icon: LayoutDashboard, disabled: true },
      // The Chat surface ships in this milestone (IMPL-11): the item
      // activates its accepted §4.1 route target — `/chat` (the landing)
      // and `/chat/:conversationId` (the selected conversation). The
      // surface is the Ask surface only (X-1): it never submits a Run,
      // and the Run path keeps its own explicit entry at `/jobs/new`.
      { id: "chat", labelKey: "items.chat", icon: MessageCircle, route: "/chat" },
      // The Jobs surface ships in this milestone (IMPL-5): the item
      // activates its accepted §4.1 route target — the list is `/jobs`,
      // and every Job link opens the ONE canonical detail route
      // `/jobs/:jobId` (R-1).
      { id: "jobs", labelKey: "items.jobs", icon: ListTodo, route: "/jobs" },
      // The Knowledge surface ships at B1-U1 (the DE-0 queue row 2): the
      // item activates its accepted §4.1 route target — the library list
      // is `/knowledge`, every pack link opens the ONE canonical detail
      // route `/knowledge/packs/:packId`, and every version link opens
      // the canonical immutable-version route (P1.1 #7/#8/#9; X-2/R-2).
      // The surface renders only what the live B1 adapter serves and
      // carries the recorded gate states for everything it does not
      // (B1-A1 §2.6's honesty list).
      { id: "knowledge", labelKey: "items.knowledge", icon: BookOpen, route: "/knowledge" },
      // The Memory surface ships at B2-U1 (the DE-0 queue row 4): the
      // item activates its accepted §4.1 route target — the list is
      // `/memory`, every item link opens the ONE canonical detail route
      // `/memory/:memoryItemId` (P1.1 #10/#11; X-2/R-3). The surface
      // renders only what the live B2 adapter serves and carries the
      // recorded honesty states for everything it does not (B2-A1 §2.6's
      // honesty list — the review actions render their honest disabled
      // state with reasons).
      { id: "memory", labelKey: "items.memory", icon: Bookmark, route: "/memory" },
      // The Context Inspector ships at B3-U1 (the DE-0 queue row 5): the
      // item activates its accepted §4.1 route target — `/context` (the
      // inspection view of the served F5 snapshot data; P1.1 #12; P4.1
      // §3.6). The conditional `/context/:snapshotId` detail route stays
      // UNSHIPPED while its condition is unmet (the GC-5 law), and the
      // save/compare controls stay HIDDEN ENTIRELY on the surface while
      // their un-gate preconditions stand (the owner's PROPOSED
      // decisions, unresolved — GC-3/GC-4; never dead stubs).
      { id: "context", labelKey: "items.context", icon: Filter, route: "/context" },
      // The Workflow surface ships at B4-U1 (the DE-0 queue row 7): the
      // item activates its accepted §4.1 route target — the list is
      // `/workflows`, and every definition link opens the ONE canonical
      // detail route `/workflows/:workflowId` (P1.1 #14/#15; P4.1 §3.7/
      // §3.8; X-2). The surface renders only what the live B4 adapter's
      // READ members serve and carries the recorded gate states for
      // everything the gates withhold (B4-A1 §2.4's honesty list): the
      // creation/editing/revision controls stay hidden while UI-30 is
      // open, NO Run control exists (the D-40 remainder), and the
      // AI-proposal producer's unwired state renders honestly.
      { id: "workflows", labelKey: "items.workflows", icon: Workflow, route: "/workflows" },
    ],
  },
  {
    id: "buildAndConnect",
    labelKey: "groups.buildAndConnect",
    items: [
      {
        id: "providers-and-models",
        labelKey: "items.providersAndModels",
        icon: Network,
        disabled: true,
      },
    ],
  },
  {
    // UI-35 LOCKED: Plugins only under the separate EXTENSIONS group. The
    // surface ships at B5-U1 (the DE-0 queue row 9): the item activates
    // its accepted §4.1 route target — the user-installed list is
    // `/plugins`, and every plugin link opens the ONE canonical detail
    // route `/plugins/:pluginId` (P1.1 #18/#19; P4.1 §3.9/§3.10; X-2).
    // THE DISCOVERABILITY-ONLY LAW (binding, P4.1 §3.11's N1): the route
    // and icon communicate discoverability ONLY — never trust, never
    // verification, never source approval, never a permission grant,
    // never catalog availability. The surface renders only what the live
    // B5 adapter's READ members serve and carries the recorded gate
    // states for everything the gates withhold (B5-A1 §2.4's honest
    // gaps): the UI-29 trust/package gate keeps catalog/install/
    // import/activation/removal controls unavailable, no catalog exists
    // (a catalog is supplied only by the Core — none is published), and
    // NO invocation/execution surface exists (the D-13/D-14 gate and the
    // Core MB §10.2 runtime proof).
    id: "extensions",
    labelKey: "groups.extensions",
    items: [{ id: "plugins", labelKey: "items.plugins", icon: Puzzle, route: "/plugins" }],
  },
  {
    id: "operate",
    labelKey: "groups.operate",
    items: [
      {
        // IMPL-24 wires the recorded item to its delivered surface: the
        // F12 audit/activity read surface ships at its recorded
        // `/activity` route (the P2.1 §2.0 feature-route name; the
        // disabled-item state the S11 §3.5 extraction recorded is
        // resolved by the surface's own delivery).
        id: "activity-and-audit",
        labelKey: "items.activityAndAudit",
        icon: History,
        route: "/activity",
      },
    ],
  },
];

export const BOTTOM_ITEMS: readonly NavItem[] = [
  // Help & Documentation: the target is deferred (P2.1 §5 row 1); the
  // item renders disabled with its reason. Keyboard-shortcut discovery
  // is reachable via `?` and Settings → Shortcuts.
  { id: "help", labelKey: "items.helpAndDocumentation", icon: CircleHelp, disabled: true },
  { id: "settings", labelKey: "items.settings", icon: SlidersHorizontal, route: "/settings" },
];

export const SESSION_ITEM = {
  id: "session",
  labelKey: "items.session" as NavLabelKey,
  icon: CircleUser,
};

/** The connection-status indicator icon: the truthful Unknown state
 *  glyph (never a connected glyph while nothing is wired — C-02). */
export const CONNECTION_UNKNOWN_ICON: LucideIcon = CircleDashed;

/** Sidebar widths (Blueprint §5.2). */
export const SIDEBAR_WIDTHS = { expanded: 248, collapsed: 72 } as const;
