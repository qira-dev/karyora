import { Navigate, Route, Routes } from "react-router-dom";

import { AppShell } from "./shell/AppShell";
import { HomePage } from "../pages/home/HomePage";
import { SettingsLayout } from "../pages/settings/SettingsLayout";
import { SettingsHub } from "../pages/settings/SettingsHub";
import { AppearancePage } from "../pages/settings/AppearancePage";
import { ShortcutsPage } from "../pages/settings/ShortcutsPage";
import { AboutPage } from "../pages/settings/AboutPage";
import { EntryStatePage } from "../pages/settings/EntryStatePage";

/**
 * The frozen Phase 2 route set (P2.1 §2.0; Blueprint §31.5): `/` plus the
 * ten accepted settings routes. The remaining §31.5 feature routes are
 * NOT Phase 2 surfaces — their screens are the owning packages'
 * deliverables (UIR-4–UIR-8), so no route, no stub, and no placeholder
 * screen exists for them here (the no-dead-stub rule). Unknown URLs
 * resolve to `/` (no dead route).
 */
export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<HomePage />} />
        <Route path="settings" element={<SettingsLayout />}>
          <Route index element={<SettingsHub />} />
          <Route path="appearance" element={<AppearancePage />} />
          <Route
            path="connections"
            element={
              <EntryStatePage
                titleKey="groups.connections.title"
                bodyKeys={["connections.body"]}
              />
            }
          />
          <Route
            path="privacy"
            element={
              <EntryStatePage titleKey="groups.privacy.title" bodyKeys={[]} />
            }
          />
          <Route
            path="security"
            element={
              <EntryStatePage titleKey="groups.security.title" bodyKeys={[]} />
            }
          />
          <Route
            path="notifications"
            element={
              <EntryStatePage titleKey="groups.notifications.title" bodyKeys={[]} />
            }
          />
          <Route path="shortcuts" element={<ShortcutsPage />} />
          <Route
            path="diagnostics"
            element={
              <EntryStatePage titleKey="groups.diagnostics.title" bodyKeys={[]} />
            }
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
    </Routes>
  );
}
