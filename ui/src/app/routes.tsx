import { Navigate, Route, Routes } from "react-router-dom";
import { Fragment } from "react";
import type { ReactElement } from "react";

import { AppShell } from "./shell/AppShell";
import { HomePage } from "../pages/home/HomePage";
import { JobsListPage } from "../pages/jobs/JobsListPage";
import { JobDetailPage } from "../pages/jobs/JobDetailPage";
import { RunTaskPage } from "../pages/run/RunTaskPage";
import { ChatPage } from "../pages/chat/ChatPage";
import { NotificationsPage } from "../pages/notifications/NotificationsPage";
import { SecurityPage } from "../pages/settings/SecurityPage";
import { ActivityPage } from "../pages/activity/ActivityPage";
import { SettingsLayout } from "../pages/settings/SettingsLayout";
import { SettingsHub } from "../pages/settings/SettingsHub";
import { AppearancePage } from "../pages/settings/AppearancePage";
import { ShortcutsPage } from "../pages/settings/ShortcutsPage";
import { AboutPage } from "../pages/settings/AboutPage";
import { EntryStatePage } from "../pages/settings/EntryStatePage";
import { ConnectionsPage } from "../pages/settings/ConnectionsPage";
import { KnowledgeLibraryPage } from "../pages/knowledge/KnowledgeLibraryPage";
import { KnowledgePackDetailPage } from "../pages/knowledge/KnowledgePackDetailPage";
import { KnowledgeVersionPage } from "../pages/knowledge/KnowledgeVersionPage";

/**
 * The frozen Phase 2 route set (P2.1 §2.0; Blueprint §31.5): `/` plus the
 * ten accepted settings routes. P2.7 added NO route: the first-run wizard
 * rides `/` exactly as C-06 records ("entered at `/` on first use"), and
 * the connection surfaces live on the already-frozen
 * `/settings/connections` route (C-14).
 *
 * IMPL-5 adds the Jobs surface's routes exactly as the records name
 * them: `/jobs` (the server-paginated list) and `/jobs/:jobId` (the ONE
 * canonical Job detail route — every Job link anywhere opens it, R-1).
 *
 * IMPL-6 adds the Run submission flow's route: `/jobs/new` — the
 * submission entry inside the Jobs family, reached ONLY through the
 * explicit `+ New → Run a task` act (X-1: a Run requires intentional
 * entry; Ask never submits). The static segment outranks the dynamic
 * `jobs/:jobId` in the router's ranking, so it can never be captured as
 * a job id. The DURABLE outcome still lives at `/jobs/:jobId` (R-1) —
 * the submission flow itself owns no durable state. The remaining §31.5
 * feature routes are still NOT surfaces — their screens are the owning
 * packages' deliverables (UIR-4, UIR-6–UIR-8), so no route, no stub, and
 * no placeholder screen exists for them here (the no-dead-stub rule).
 * Unknown URLs resolve to `/` (no dead route).
 *
 * IMPL-11 adds the conversation/Ask surface's routes exactly as the
 * records name them (P1.1 #3/#4; P3.5 §2's scope): `/chat` (no
 * conversation selected — the landing) and `/chat/:conversationId` (the
 * selected conversation's authoritative timeline + composer). The two
 * render one surface component: the id selects the conversation; a
 * selection is navigation, never a mutation. The Chat surface stays the
 * Ask surface only — it never submits a Run (X-1: the Run path keeps its
 * own explicit entry at `/jobs/new`), and no Job card renders from any
 * conversation send.
 *
 * IMPL-14 WIRES the already-frozen `/settings/notifications` route to
 * the delivered Core notification machinery (the ADOPTED S6 supply,
 * P-S6-07's own terms): the entry-state page is replaced by the real
 * notification surface (the same component the topbar
 * notification-center popover renders). NO new route is added, and the
 * preference controls are NOT here (no preference store exists in the
 * delivered Core contract — the page states the absence truthfully).
 *
 * IMPL-24 WIRES the recorded `/activity` feature route (P1.1 §2.7.1's
 * route row; the P2.1 §2.0 feature-route NAME) to the delivered F12
 * audit/activity read surface — the disabled OPERATE-group nav item the
 * S11 §3.5 extraction recorded is resolved by the surface's own
 * delivery. The surface is READ-ONLY: no dismiss, no delete, and no
 * correction control exists anywhere on it.
 *
 * B1-U1 (the DE-0 queue row 2) adds the Knowledge surface's routes
 * exactly as the records name them (P1.1 #7/#8/#9; P4.1 §3.1/§3.2):
 * `/knowledge` (the library list), `/knowledge/packs/:packId` (the ONE
 * canonical pack detail route — every pack link opens it, X-2), and
 * `/knowledge/packs/:packId/versions/:versionId` (the canonical
 * immutable-version deep link). The surfaces consume the now-live B1
 * `/knowledgepacks` adapter family only (B1-A1 §2.1) and render nothing
 * the adapter does not serve (B1-A1 §2.6's honesty list). The remaining
 * UIR-6/7 feature routes (Memory, Context, Workflows, Plugins) are still
 * NOT surfaces — no route, no stub, and no placeholder screen exists for
 * them (the no-dead-stub rule).
 *
 * The route ELEMENTS are exported so the app can build a data router
 * from them (createBrowserRouter + createRoutesFromElements — required
 * by the J-8 dirty-form navigation guard's useBlocker); `AppRoutes`
 * renders the same elements in declarative mode.
 */
export const appRouteElements: ReactElement = (
  <Fragment>
    <Route element={<AppShell />}>
    <Route index element={<HomePage />} />
    <Route path="chat" element={<ChatPage />} />
    <Route path="chat/:conversationId" element={<ChatPage />} />
    <Route path="activity" element={<ActivityPage />} />
    <Route path="knowledge" element={<KnowledgeLibraryPage />} />
    <Route path="knowledge/packs/:packId" element={<KnowledgePackDetailPage />} />
    <Route path="knowledge/packs/:packId/versions/:versionId" element={<KnowledgeVersionPage />} />
    <Route path="jobs" element={<JobsListPage />} />
    <Route path="jobs/new" element={<RunTaskPage />} />
    <Route path="jobs/:jobId" element={<JobDetailPage />} />
    <Route path="settings" element={<SettingsLayout />}>
      <Route index element={<SettingsHub />} />
      <Route path="appearance" element={<AppearancePage />} />
      <Route path="connections" element={<ConnectionsPage />} />
      <Route
        path="privacy"
        element={<EntryStatePage titleKey="groups.privacy.title" bodyKeys={[]} />}
      />
      <Route
        path="security"
        element={
          /* IMPL-23 WIRES the already-frozen `/settings/security` route to
           * the delivered C-4 logout/revocation commands (the S9-adopted
           * P-S9-02 class's UI half per R-13): the entry-state page is
           * replaced by the dedicated logout flow's surface. NO new route
           * is added, and no control is invented for the credential-change
           * revocation (its recorded trigger classes are Core-side — the
           * honest absence is stated on the page). */
          <SecurityPage />
        }
      />
      <Route
        path="notifications"
        element={<NotificationsPage />}
      />
      <Route path="shortcuts" element={<ShortcutsPage />} />
      <Route
        path="diagnostics"
        element={<EntryStatePage titleKey="groups.diagnostics.title" bodyKeys={[]} />}
      />
      <Route
        path="updates"
        element={
          <EntryStatePage
            titleKey="groups.updates.title"
            bodyKeys={["updates.body"]}
          />
        }
      />
      <Route path="about" element={<AboutPage />} />
    </Route>
    <Route path="*" element={<Navigate to="/" replace />} />
  </Route>
  </Fragment>
);

export function AppRoutes() {
  return <Routes>{appRouteElements}</Routes>;
}
