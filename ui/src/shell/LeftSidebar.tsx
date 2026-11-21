/**
 * Left Sidebar Navigation (`248 px` Expanded / Collapsible via `Ctrl+B`)
 * (UIR-3.2, Screenshots #61–#65, #68, #84–#85, Blueprint §4.3).
 */

import type { ReactElement } from "react";
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
  if (collapsed) {
    return null;
  }

  return (
    <aside
      aria-label="Primary workspace navigation"
      data-testid="karyora-left-sidebar"
      className="ky-sidebar"
    >
      {/* Top Window Chrome & Collapse Toggle */}
      <div className="ky-sidebar__top-chrome">
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          <ChromeIcon name="sparkles" size={16} />
          <span style={{ fontWeight: 600, fontSize: "0.875rem" }}>Karyora</span>
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
            <ChromeIcon name="search" size={16} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Collapse sidebar (Ctrl+B)"
            title="Toggle Sidebar (Ctrl+B)"
            onClick={onToggleCollapse}
          >
            <ChromeIcon name="sidebar-left" size={16} />
          </Button>
        </div>
      </div>

      {/* ZCode Primary Navigation Entries (#68) */}
      <nav aria-label="Workspace tools" className="ky-sidebar__primary-nav">
        <Button
          variant="ghost"
          size="sm"
          selected={activeSurface === "workspace" && activeTaskId === null}
          leadingIcon={<ChromeIcon name="new-task" size={16} />}
          style={{ justifyContent: "flex-start", width: "100%" }}
          onClick={onNewTask}
        >
          New task
        </Button>
        <Button
          variant="ghost"
          size="sm"
          selected={activeSurface === "automations"}
          leadingIcon={<ChromeIcon name="automations" size={16} />}
          style={{ justifyContent: "flex-start", width: "100%" }}
          onClick={() => onSelectSurface("automations")}
        >
          Automations
        </Button>
        <Button
          variant="ghost"
          size="sm"
          selected={activeSurface === "marketplace"}
          leadingIcon={<ChromeIcon name="plugin-marketplace" size={16} />}
          style={{ justifyContent: "flex-start", width: "100%" }}
          onClick={() => onSelectSurface("marketplace")}
        >
          Plugin Marketplace
        </Button>
      </nav>

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

        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "2px",
            paddingTop: "var(--space-2)",
            borderTop: "1px solid var(--border-subtle)",
          }}
        >
          <div className="ky-sidebar__section-header">
            <span>Karyora Core</span>
          </div>
          <Button
            variant="ghost"
            size="sm"
            selected={activeSurface === "jobs"}
            leadingIcon={<ChromeIcon name="cpu" size={14} />}
            style={{ justifyContent: "flex-start", width: "100%" }}
            onClick={() => onSelectSurface("jobs")}
          >
            Durable Jobs
          </Button>
          <Button
            variant="ghost"
            size="sm"
            selected={activeSurface === "knowledge"}
            leadingIcon={<ChromeIcon name="database" size={14} />}
            style={{ justifyContent: "flex-start", width: "100%" }}
            onClick={() => onSelectSurface("knowledge")}
          >
            KnowledgePack
          </Button>
          <Button
            variant="ghost"
            size="sm"
            selected={activeSurface === "memory"}
            leadingIcon={<ChromeIcon name="brain-thinking" size={14} />}
            style={{ justifyContent: "flex-start", width: "100%" }}
            onClick={() => onSelectSurface("memory")}
          >
            Scoped Memory
          </Button>
          <Button
            variant="ghost"
            size="sm"
            selected={activeSurface === "context"}
            leadingIcon={<ChromeIcon name="sliders" size={14} />}
            style={{ justifyContent: "flex-start", width: "100%" }}
            onClick={() => onSelectSurface("context")}
          >
            Context Inspector
          </Button>
          <Button
            variant="ghost"
            size="sm"
            selected={activeSurface === "activity"}
            leadingIcon={<ChromeIcon name="activity" size={14} />}
            style={{ justifyContent: "flex-start", width: "100%" }}
            onClick={() => onSelectSurface("activity")}
          >
            Activity &amp; Audit
          </Button>
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
          aria-label="Open Settings (Ctrl+,)"
          title="Settings (Ctrl+,)"
          onClick={() => onOpenSettings("general", "sidebar-settings-btn")}
        >
          <ChromeIcon name="settings" size={16} />
        </Button>
      </footer>
    </aside>
  );
}
