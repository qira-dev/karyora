/**
 * 3-Region Application Shell (UIR-3, Blueprint §4.2–4.4, §14, §22.2, §23.4).
 *
 * Coordinates:
 * - Left Sidebar (`248 px`, collapsible `Ctrl+B`, Projects & Tasks `#84–#85`, Profile `#61–#65`)
 * - Center Workspace (`WorkspaceTopBar` `#79–#80` + Active Surface + Bottom Terminal slot `#81`)
 * - Right Multi-Tab Side Pane slot (`Ctrl+Alt+B`, `#74`, `#78`, `#82–#83`)
 * - Overlays: Global Command Center (`Ctrl/Cmd+K`), Notification Drawer, First-Run Onboarding,
 *   6-Step Migration Guide (`#55`), and 21-Section Overlay Settings (`Ctrl+,`) keeping the
 *   underlying workspace mounted & `inert` (`G-06`).
 */

import { useEffect, type ReactElement, type ReactNode } from "react";
import { ToastRegion, type ToastMessage } from "../components/primitives";
import type {
  CoreBootstrapStatus,
  TaskSessionSummary,
  WorkspaceProjectSummary,
} from "../coreclient/types";
import type {
  AppTheme,
  DarkCodeTheme,
  InterfaceMode,
  LightCodeTheme,
  SettingsSectionId,
  SupportedLocale,
  TargetPlatform,
} from "../types/decisions";
import { CommandCenterModal } from "./CommandCenterModal";
import { LeftSidebar } from "./LeftSidebar";
import { NotificationDrawer } from "./NotificationDrawer";
import { OnboardingAndMigrationModal } from "./OnboardingAndMigrationModal";
import {
  clampInterfaceZoom,
  INTERFACE_ZOOM_STEP,
  type ActiveMainSurface,
  type CoreNotificationItem,
} from "./shellConfig";
import type { TaskContextActionId } from "./TaskContextMenu";
import { WorkspaceTopBar } from "./WorkspaceTopBar";

export interface ApplicationShellProps {
  readonly runtimeTarget: TargetPlatform;
  readonly bootstrapStatus: CoreBootstrapStatus | null;
  readonly activeTheme: AppTheme;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly activeLocale: SupportedLocale;
  readonly activeInterfaceMode: InterfaceMode;
  readonly activeZoomPercent: number;
  readonly sidebarCollapsed: boolean;
  readonly terminalOpen: boolean;
  readonly terminalMaximized?: boolean;
  readonly sidePaneOpen: boolean;
  readonly sidePaneFullView?: boolean;
  readonly activeSurface: ActiveMainSurface;
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly tasks: readonly TaskSessionSummary[];
  readonly activeTask: TaskSessionSummary | null;
  readonly notifications: readonly CoreNotificationItem[];
  readonly osNotificationsEnabled: boolean;
  readonly commandCenterOpen: boolean;
  readonly notificationDrawerOpen: boolean;
  readonly onboardingMode: "closed" | "onboarding" | "migration";
  readonly settingsOverlayOpen: boolean;
  readonly toasts: readonly ToastMessage[];
  readonly canWrite: boolean;
  readonly onToggleSidebar: () => void;
  readonly onToggleTerminal: () => void;
  readonly onToggleSidePane: () => void;
  readonly onToggleSidePaneFullView?: () => void;
  readonly onOpenReviewDiffTab?: () => void;
  readonly onNewTask: () => void;
  readonly onSelectSurface: (surface: ActiveMainSurface) => void;
  readonly onSelectTask: (task: TaskSessionSummary) => void;
  readonly onAddProject: () => void;
  readonly onTaskContextAction: (
    actionId: TaskContextActionId,
    task: TaskSessionSummary,
  ) => void;
  readonly onCreateShareLink: (taskId: string) => void;
  readonly onOpenSettings: (section?: SettingsSectionId, triggerId?: string) => void;
  readonly onOpenCommandCenter: (triggerId?: string) => void;
  readonly onCloseCommandCenter: () => void;
  readonly onToggleNotifications: () => void;
  readonly onMarkNotificationRead: (eventId: string) => void;
  readonly onDismissNotification: (eventId: string) => void;
  readonly onToggleOsNotifications: (enabled: boolean) => void;
  readonly onOpenOnboardingMode: (mode: "closed" | "onboarding" | "migration") => void;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeTheme: (theme: AppTheme) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onChangeZoomPercent: (zoomPercent: number) => void;
  readonly onDismissToast: (toastId: string) => void;
  readonly sidePaneContent?: ReactNode;
  readonly terminalContent?: ReactNode;
  readonly floatingWorkingBar?: ReactNode;
  readonly settingsOverlayContent?: ReactNode;
  readonly children: ReactNode;
}

const SURFACE_TITLES: Readonly<Record<ActiveMainSurface, string>> = {
  workspace: "New task",
  automations: "Automations | Workflows",
  marketplace: "Plugin Marketplace",
  jobs: "Durable Jobs (AIJobRelay)",
  knowledge: "KnowledgePack Library",
  memory: "Scoped Memory",
  context: "Context Inspector",
  activity: "Activity & Audit",
};

export function ApplicationShell({
  runtimeTarget,
  bootstrapStatus,
  activeTheme,
  activeCodeTheme,
  activeLocale,
  activeInterfaceMode,
  activeZoomPercent,
  sidebarCollapsed,
  terminalOpen,
  terminalMaximized = false,
  sidePaneOpen,
  sidePaneFullView = false,
  activeSurface,
  projects,
  tasks,
  activeTask,
  notifications,
  osNotificationsEnabled,
  commandCenterOpen,
  notificationDrawerOpen,
  onboardingMode,
  settingsOverlayOpen,
  toasts,
  canWrite,
  onToggleSidebar,
  onToggleTerminal,
  onToggleSidePane,
  onToggleSidePaneFullView,
  onOpenReviewDiffTab,
  onNewTask,
  onSelectSurface,
  onSelectTask,
  onAddProject,
  onTaskContextAction,
  onCreateShareLink,
  onOpenSettings,
  onOpenCommandCenter,
  onCloseCommandCenter,
  onToggleNotifications,
  onMarkNotificationRead,
  onDismissNotification,
  onToggleOsNotifications,
  onOpenOnboardingMode,
  onChangeLocale,
  onChangeTheme,
  onChangeInterfaceMode,
  onChangeZoomPercent,
  onDismissToast,
  sidePaneContent,
  terminalContent,
  floatingWorkingBar,
  settingsOverlayContent,
  children,
}: ApplicationShellProps): ReactElement {
  const workspaceInert =
    commandCenterOpen || settingsOverlayOpen || onboardingMode !== "closed";
  const unreadNotificationsCount = notifications.filter(
    (n) => !n.read && !n.dismissed,
  ).length;

  // Global keyboard shortcuts (Blueprint §13.1 (7), Screenshots #21–#22)
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      if (!mod) return;

      const lower = e.key.toLowerCase();
      if (lower === "k" || (e.shiftKey && lower === "p")) {
        e.preventDefault();
        onOpenCommandCenter();
      } else if (e.shiftKey && lower === "f" && onToggleSidePaneFullView) {
        e.preventDefault();
        onToggleSidePaneFullView();
      } else if (lower === "b" && e.altKey) {
        e.preventDefault();
        onToggleSidePane();
      } else if (lower === "b" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        onToggleSidebar();
      } else if (lower === "j" && !e.shiftKey) {
        e.preventDefault();
        onToggleTerminal();
      } else if (e.key === ",") {
        e.preventDefault();
        onOpenSettings("general");
      } else if (e.shiftKey && lower === "l") {
        e.preventDefault();
        onChangeTheme(activeTheme === "dark" ? "light" : "dark");
      } else if (lower === "n" && !e.shiftKey) {
        e.preventDefault();
        onNewTask();
      } else if (e.key === "=" || e.key === "+") {
        e.preventDefault();
        onChangeZoomPercent(
          clampInterfaceZoom(activeZoomPercent + INTERFACE_ZOOM_STEP),
        );
      } else if (e.key === "-") {
        e.preventDefault();
        onChangeZoomPercent(
          clampInterfaceZoom(activeZoomPercent - INTERFACE_ZOOM_STEP),
        );
      } else if (e.key === "0") {
        e.preventDefault();
        onChangeZoomPercent(100);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    activeTheme,
    activeZoomPercent,
    onChangeTheme,
    onChangeZoomPercent,
    onNewTask,
    onOpenCommandCenter,
    onOpenSettings,
    onToggleSidePane,
    onToggleSidePaneFullView,
    onToggleSidebar,
    onToggleTerminal,
  ]);

  const resolvedThemeClass = activeTheme === "light" ? "" : "dark";
  const resolvedDataTheme = activeTheme === "light" ? "light" : "dark";

  return (
    <div
      className={`karyora-shell-root ${resolvedThemeClass}`}
      data-theme={resolvedDataTheme}
      data-code-theme={activeCodeTheme}
      data-interface-mode={activeInterfaceMode}
      data-platform={runtimeTarget}
      data-locale={activeLocale}
      data-testid="karyora-v3-root"
      style={{
        fontSize: `${(14 * activeZoomPercent) / 100}px`,
      }}
    >
      {/* Underlying 3-Region Workspace stays mounted & inert while Overlays are open (G-06) */}
      <div
        className="ky-app-shell"
        data-inert={workspaceInert ? "true" : "false"}
        aria-hidden={workspaceInert ? "true" : undefined}
      >
        <LeftSidebar
          collapsed={sidebarCollapsed}
          activeSurface={activeSurface}
          projects={projects}
          tasks={tasks}
          activeTaskId={activeTask?.taskId ?? null}
          activeLocale={activeLocale}
          activeTheme={activeTheme}
          activeInterfaceMode={activeInterfaceMode}
          activeZoomPercent={activeZoomPercent}
          onToggleCollapse={onToggleSidebar}
          onNewTask={onNewTask}
          onSelectSurface={onSelectSurface}
          onSelectTask={onSelectTask}
          onAddProject={onAddProject}
          onTaskContextAction={onTaskContextAction}
          onOpenSettings={onOpenSettings}
          onOpenCommandCenter={onOpenCommandCenter}
          onChangeLocale={onChangeLocale}
          onChangeTheme={onChangeTheme}
          onChangeInterfaceMode={onChangeInterfaceMode}
          onChangeZoomPercent={onChangeZoomPercent}
        />

        <div className="ky-main-column">
          <WorkspaceTopBar
            sidebarCollapsed={sidebarCollapsed}
            surfaceTitle={SURFACE_TITLES[activeSurface]}
            activeTask={activeSurface === "workspace" ? activeTask : null}
            terminalOpen={terminalOpen}
            sidePaneOpen={sidePaneOpen}
            unreadNotificationsCount={unreadNotificationsCount}
            onExpandSidebar={onToggleSidebar}
            onOpenReviewDiffTab={() => {
              if (onOpenReviewDiffTab) {
                onOpenReviewDiffTab();
              } else if (!sidePaneOpen) {
                onToggleSidePane();
              }
            }}
            onCreateShareLink={onCreateShareLink}
            onToggleTerminal={onToggleTerminal}
            onToggleSidePane={onToggleSidePane}
            onToggleNotifications={onToggleNotifications}
            onOpenAboutSettings={() => onOpenSettings("shares-and-governance")}
            onOpenMigrationGuide={() => onOpenOnboardingMode("migration")}
          />

          <div className="ky-workspace-body">
            <div
              className="ky-center-pane"
              data-hidden-for-full-view={
                sidePaneOpen && sidePaneFullView ? "true" : "false"
              }
            >
              <main className="ky-center-scroll">{children}</main>
              {terminalOpen && (
                <section
                  aria-label="Docked PTY Terminal"
                  data-testid="bottom-terminal-slot"
                  data-maximized={terminalMaximized ? "true" : "false"}
                  className="ky-bottom-terminal-slot"
                >
                  {terminalContent ?? (
                    <div
                      style={{
                        padding: "var(--space-3)",
                        fontFamily: "var(--font-mono)",
                        fontSize: "0.75rem",
                      }}
                    >
                      Terminal zsh — ready.
                    </div>
                  )}
                </section>
              )}
            </div>

            {sidePaneOpen && (
              <aside
                aria-label="Multi-Tab Side Pane"
                data-testid="multi-tab-side-pane-slot"
                data-full-view={sidePaneFullView ? "true" : "false"}
                className="ky-side-pane-slot"
              >
                {sidePaneContent ?? (
                  <div
                    style={{
                      padding: "var(--space-4)",
                      fontSize: "0.875rem",
                      color: "var(--text-secondary)",
                    }}
                  >
                    Multi-Tab Side Pane ready.
                  </div>
                )}
              </aside>
            )}
          </div>
          {floatingWorkingBar}
        </div>
      </div>

      {/* Global Overlays */}
      <CommandCenterModal
        isOpen={commandCenterOpen}
        canWrite={canWrite}
        projects={projects}
        tasks={tasks}
        onClose={onCloseCommandCenter}
        onExecuteWorkspaceAction={(actionId) => {
          if (actionId === "new-task") onNewTask();
          else if (actionId === "toggle-sidebar") onToggleSidebar();
          else if (actionId === "toggle-terminal") onToggleTerminal();
          else if (actionId === "toggle-side-pane") onToggleSidePane();
          else if (actionId === "toggle-theme") {
            onChangeTheme(activeTheme === "dark" ? "light" : "dark");
          } else if (actionId === "open-migration") {
            onOpenOnboardingMode("migration");
          }
        }}
        onNavigateSurface={onSelectSurface}
        onSelectTask={onSelectTask}
        onOpenSettingsSection={(section) => onOpenSettings(section)}
      />

      <NotificationDrawer
        isOpen={notificationDrawerOpen}
        notifications={notifications}
        osNotificationsEnabled={osNotificationsEnabled}
        onClose={onToggleNotifications}
        onMarkRead={onMarkNotificationRead}
        onDismiss={onDismissNotification}
        onToggleOsNotifications={onToggleOsNotifications}
        onOpenNotificationTarget={(item) => {
          if (item.targetSurface) {
            onSelectSurface(item.targetSurface);
          }
          onToggleNotifications();
        }}
      />

      <OnboardingAndMigrationModal
        mode={onboardingMode}
        bootstrapStatus={bootstrapStatus}
        onClose={() => onOpenOnboardingMode("closed")}
        onOpenModelSettings={() => onOpenSettings("model-settings")}
        onPickWorkspaceFolder={onAddProject}
      />

      {settingsOverlayOpen && settingsOverlayContent}

      <ToastRegion toasts={toasts} onDismissToast={onDismissToast} />
    </div>
  );
}
