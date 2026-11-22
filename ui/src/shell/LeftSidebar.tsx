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
  readonly activeSurface: ActiveMainSurface;
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly tasks: readonly TaskSessionSummary[];
  readonly activeTaskId: string | null;
  readonly activeLocale: SupportedLocale;
  readonly activeTheme: AppTheme;
  readonly activeInterfaceMode: InterfaceMode;
  readonly activeZoomPercent: number;
  readonly onToggleCollapse: () => void;
  readonly onNewTask: () => void;
  readonly onSelectSurface: (surface: ActiveMainSurface) => void;
  readonly onSelectTask: (task: TaskSessionSummary) => void;
  readonly onAddProject: () => void;
  readonly onTaskContextAction: (
    actionId: TaskContextActionId,
    task: TaskSessionSummary,
  ) => void;
  readonly onOpenSettings: (section?: SettingsSectionId, triggerId?: string) => void;
  readonly onOpenCommandCenter: (triggerId?: string) => void;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeTheme: (theme: AppTheme) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onChangeZoomPercent: (zoomPercent: number) => void;
}

export function LeftSidebar({
  collapsed,
  activeSurface,
  projects,
  tasks,
  activeTaskId,
  activeLocale,
  activeTheme,
  activeInterfaceMode,
  activeZoomPercent,
  onToggleCollapse,
  onNewTask,
  onSelectSurface,
  onSelectTask,
  onAddProject,
  onTaskContextAction,
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
    >
      {/* Top Window Brand Mark & Subtle Navigation Arrows (#68, #75, #82) */}
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

      {/* Scrollable Projects & Tasks Tree + Karyora Core Domain Links */}
      <div className="ky-sidebar__scroll-region">
        <ProjectTaskTree
          projects={projects}
          tasks={tasks}
          activeTaskId={activeTaskId}
          onSelectTask={onSelectTask}
          onAddProject={onAddProject}
          onTaskContextAction={onTaskContextAction}
        />

        <div className="ky-sidebar__divider" style={{ margin: "6px 2px" }} />

        <div style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
          <div className="ky-sidebar__section-header">
            <span>{t("ui.sidebar.coreSection", { defaultValue: "Karyora Core" })}</span>
          </div>
          <button
            type="button"
            className="ky-sidebar__nav-item"
            data-active={activeSurface === "jobs" ? "true" : "false"}
            onClick={() => onSelectSurface("jobs")}
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="cpu" size={14} />
              <span>{t("ui.sidebar.jobs", { defaultValue: "Jobs" })}</span>
            </span>
          </button>
          <button
            type="button"
            className="ky-sidebar__nav-item"
            data-active={activeSurface === "knowledge" ? "true" : "false"}
            onClick={() => onSelectSurface("knowledge")}
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="database" size={14} />
              <span>{t("ui.sidebar.knowledge", { defaultValue: "Knowledge" })}</span>
            </span>
          </button>
          <button
            type="button"
            className="ky-sidebar__nav-item"
            data-active={activeSurface === "memory" ? "true" : "false"}
            onClick={() => onSelectSurface("memory")}
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="brain-thinking" size={14} />
              <span>{t("ui.sidebar.memory", { defaultValue: "Memory" })}</span>
            </span>
          </button>
          <button
            type="button"
            className="ky-sidebar__nav-item"
            data-active={activeSurface === "context" ? "true" : "false"}
            onClick={() => onSelectSurface("context")}
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="sliders" size={14} />
              <span>{t("ui.sidebar.context", { defaultValue: "Context" })}</span>
            </span>
          </button>
          <button
            type="button"
            className="ky-sidebar__nav-item"
            data-active={activeSurface === "activity" ? "true" : "false"}
            onClick={() => onSelectSurface("activity")}
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="activity" size={14} />
              <span>{t("ui.sidebar.activityAudit", { defaultValue: "Activity & Audit" })}</span>
            </span>
          </button>
        </div>
      </div>

      {/* Bottom Footer: Profile Popover (#61–#65) & Settings Trigger (Ctrl+,) */}
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
      </footer>
    </aside>
  );
}
