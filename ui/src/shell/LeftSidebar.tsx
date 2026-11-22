/**
 * Left Sidebar Navigation (`260 px` ZCode Chrome, Collapsible via `Ctrl+B`)
 * (Screenshots #61–#65, #68, #75, #82, #84–#85, Blueprint §4.3).
 */

import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Button } from "../components/primitives";
import type {
  TaskSessionSummary,
  WorkspaceProjectSummary,
} from "../coreclient/types";
import type {
  AppTheme,
  InterfaceMode,
  SettingsSectionId,
  SupportedLocale,
} from "../types/decisions";
import { ProfilePopoverMenu } from "./ProfilePopoverMenu";
import { ProjectTaskTree } from "./ProjectTaskTree";
import type { ActiveMainSurface } from "./shellConfig";
import type { TaskContextActionId } from "./TaskContextMenu";

export interface LeftSidebarProps {
  readonly collapsed: boolean;
  readonly widthPx?: number;
  readonly canGoBack?: boolean;
  readonly canGoForward?: boolean;
  readonly activeSurface: ActiveMainSurface;
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly tasks: readonly TaskSessionSummary[];
  readonly activeTaskId: string | null;
  readonly activeLocale: SupportedLocale;
  readonly activeTheme: AppTheme;
  readonly activeInterfaceMode: InterfaceMode;
  readonly activeZoomPercent: number;
  readonly onToggleCollapse: () => void;
  readonly onGoBack?: () => void;
  readonly onGoForward?: () => void;
  readonly onNewTask: () => void;
  readonly onSelectSurface: (surface: ActiveMainSurface) => void;
  readonly onSelectTask: (task: TaskSessionSummary) => void;
  readonly onAddProject: () => void;
  readonly onShowProjectFiles?: (project: WorkspaceProjectSummary) => void;
  readonly onTaskContextAction: (
    actionId: TaskContextActionId,
    task: TaskSessionSummary,
  ) => void;
  readonly onOpenMobileRemoteControl?: () => void;
  readonly onOpenResourceManager?: () => void;
  readonly onOpenSettings: (section?: SettingsSectionId, triggerId?: string) => void;
  readonly onOpenCommandCenter: (triggerId?: string) => void;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeTheme: (theme: AppTheme) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onChangeZoomPercent: (zoomPercent: number) => void;
}

export function LeftSidebar({
  collapsed,
  widthPx,
  canGoBack = false,
  canGoForward = false,
  activeSurface,
  projects,
  tasks,
  activeTaskId,
  activeLocale,
  activeTheme,
  activeInterfaceMode,
  activeZoomPercent,
  onToggleCollapse,
  onGoBack,
  onGoForward,
  onNewTask,
  onSelectSurface,
  onSelectTask,
  onAddProject,
  onShowProjectFiles,
  onTaskContextAction,
  onOpenMobileRemoteControl,
  onOpenResourceManager,
  onOpenSettings,
  onOpenCommandCenter,
  onChangeLocale,
  onChangeTheme,
  onChangeInterfaceMode,
  onChangeZoomPercent,
}: LeftSidebarProps): ReactElement | null {
  const { t } = useTranslation();

  if (collapsed) {
    return null;
  }

  return (
    <aside
      aria-label="Primary workspace navigation"
      data-testid="karyora-left-sidebar"
      className="ky-sidebar"
      style={
        widthPx
          ? { width: `${widthPx}px`, minWidth: `${widthPx}px`, maxWidth: `${widthPx}px` }
          : undefined
      }
    >
      {/* Top Window Brand Mark & Back/Forward Navigation Arrows (`00-main-window.aria.yaml`, #68, #75, #82) */}
      <div className="ky-sidebar__top-chrome">
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <span className="ky-sidebar__brand-mark" aria-hidden="true">
            K
          </span>
          <span style={{ fontWeight: 600, fontSize: "0.875rem", color: "var(--text-primary)" }}>
            {t("appName", { defaultValue: "Karyora" })}
          </span>
        </div>
        <div style={{ display: "inline-flex", alignItems: "center", gap: "2px" }}>
          <Button
            variant="ghost"
            size="sm"
            disabled={!canGoBack}
            aria-label={t("ui.sidebar.goBack", { defaultValue: "Go back" })}
            title={t("ui.sidebar.goBack", { defaultValue: "Go back" })}
            onClick={onGoBack}
          >
            <ChromeIcon name="chevron-left" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={!canGoForward}
            aria-label={t("ui.sidebar.goForward", { defaultValue: "Go forward" })}
            title={t("ui.sidebar.goForward", { defaultValue: "Go forward" })}
            onClick={onGoForward}
          >
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          <Button
            id="sidebar-cmd-center-btn"
            variant="ghost"
            size="sm"
            aria-label="Open Command Center (Ctrl+K)"
            title="Command Center (Ctrl+K)"
            onClick={() => onOpenCommandCenter("sidebar-cmd-center-btn")}
          >
            <ChromeIcon name="search" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label={t("ui.sidebar.collapseSidebar", { defaultValue: "Collapse sidebar" })}
            title="Toggle Sidebar (Ctrl+B)"
            onClick={onToggleCollapse}
          >
            <ChromeIcon name="sidebar-left" size={14} />
          </Button>
        </div>
      </div>

      {/* ZCode Primary 3 Navigation Items (#68, #75, #82) */}
      <nav aria-label="Workspace tools" className="ky-sidebar__primary-nav">
        <button
          type="button"
          className="ky-sidebar__nav-item"
          data-active={activeSurface === "workspace" && activeTaskId === null ? "true" : "false"}
          onClick={onNewTask}
        >
          <span className="ky-sidebar__nav-left">
            <ChromeIcon name="new-task" size={16} />
            <span>{t("ui.sidebar.newTask", { defaultValue: "New task" })}</span>
          </span>
          <span className="ky-sidebar__shortcut-hint">Ctrl+N</span>
        </button>

        <button
          type="button"
          className="ky-sidebar__nav-item"
          data-active={activeSurface === "automations" ? "true" : "false"}
          onClick={() => onSelectSurface("automations")}
        >
          <span className="ky-sidebar__nav-left">
            <ChromeIcon name="automations" size={16} />
            <span>{t("ui.sidebar.automations", { defaultValue: "Automations" })}</span>
          </span>
        </button>

        <button
          type="button"
          className="ky-sidebar__nav-item"
          data-active={activeSurface === "marketplace" ? "true" : "false"}
          onClick={() => onSelectSurface("marketplace")}
        >
          <span className="ky-sidebar__nav-left">
            <ChromeIcon name="plugin-marketplace" size={16} />
            <span>{t("ui.sidebar.pluginMarketplace", { defaultValue: "Plugin Marketplace" })}</span>
          </span>
        </button>
      </nav>

      <div className="ky-sidebar__divider" />

      {/* Scrollable Projects & Tasks Tree (Clean Antigravity Left Sidebar — Core Surfaces live in Settings & Right Overview) */}
      <div className="ky-sidebar__scroll-region">
        <ProjectTaskTree
          projects={projects}
          tasks={tasks}
          activeTaskId={activeTaskId}
          onSelectTask={onSelectTask}
          onAddProject={onAddProject}
          onNewTask={onNewTask}
          onShowProjectFiles={onShowProjectFiles}
          onTaskContextAction={onTaskContextAction}
        />
      </div>

      {/* Bottom Footer: Profile Popover (#61–#65), Mobile Remote Control, Resource Manager & Settings (Ctrl+,) */}
      <footer className="ky-sidebar__footer">
        <ProfilePopoverMenu
          userDisplayName="Local Owner"
          maskedUserId="local-core-01"
          activeLocale={activeLocale}
          activeTheme={activeTheme}
          activeInterfaceMode={activeInterfaceMode}
          activeZoomPercent={activeZoomPercent}
          onChangeLocale={onChangeLocale}
          onChangeTheme={onChangeTheme}
          onChangeInterfaceMode={onChangeInterfaceMode}
          onChangeZoomPercent={onChangeZoomPercent}
        />
        <div style={{ display: "inline-flex", alignItems: "center", gap: "2px" }}>
          {onOpenMobileRemoteControl && (
            <Button
              variant="ghost"
              size="sm"
              aria-label={t("ui.remoteControl.title", {
                defaultValue: "Mobile remote control",
              })}
              title={t("ui.remoteControl.title", {
                defaultValue: "Mobile remote control",
              })}
              data-testid="sidebar-mobile-remote-btn"
              onClick={onOpenMobileRemoteControl}
            >
              <ChromeIcon name="qr-code" size={16} />
            </Button>
          )}
          {onOpenResourceManager && (
            <Button
              variant="ghost"
              size="sm"
              aria-label="Resource Manager"
              title="Resource Manager"
              data-testid="sidebar-resource-manager-btn"
              onClick={onOpenResourceManager}
            >
              <ChromeIcon name="gauge" size={16} />
            </Button>
          )}
          <Button
            id="sidebar-settings-btn"
            variant="ghost"
            size="sm"
            aria-label={t("ui.sidebar.settings", { defaultValue: "Settings" })}
            title={`${t("ui.sidebar.settings", { defaultValue: "Settings" })} (Ctrl+,)`}
            onClick={() => onOpenSettings("general", "sidebar-settings-btn")}
          >
            <ChromeIcon name="settings" size={16} />
          </Button>
        </div>
      </footer>
    </aside>
  );
}
