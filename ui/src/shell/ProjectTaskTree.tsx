/**
 * Left Sidebar Projects & Tasks Tree (UIR-3.2, Screenshots #68, #84, #85, Blueprint §4.3 (4)).
 *
 * Includes:
 * - `Projects` section controls: `+` Add project, `🔍` inline search, `Filter` mode, `Group by` (`Project` | `Flat`)
 * - Collapsible project groups with `ProjectHoverCard` (#84) on hover/focus
 * - Nested task rows with running shimmer indicator, pinned/unread dots, relative time,
 *   and right-click / `...` 13-item `TaskContextMenu` (#85)
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Button, Input } from "../components/primitives";
import type {
  TaskSessionSummary,
  WorkspaceProjectSummary,
} from "../coreclient/types";
import { ProjectHoverCard } from "./ProjectHoverCard";
import {
  filterTasksForSidebar,
  filterVisibleSidebarProjects,
  type ProjectTaskFilterMode,
  type ProjectTaskGroupMode,
} from "./shellConfig";
import { TaskContextMenu, type TaskContextActionId } from "./TaskContextMenu";

export interface ProjectTaskTreeProps {
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly tasks: readonly TaskSessionSummary[];
  readonly activeTaskId: string | null;
  readonly onSelectTask: (task: TaskSessionSummary) => void;
  readonly onAddProject: () => void;
  readonly onNewTask?: () => void;
  readonly onShowProjectFiles?: (project: WorkspaceProjectSummary) => void;
  readonly onTaskContextAction: (
    actionId: TaskContextActionId,
    task: TaskSessionSummary,
  ) => void;
}

export function ProjectTaskTree({
  projects,
  tasks,
  activeTaskId,
  onSelectTask,
  onAddProject,
  onNewTask,
  onShowProjectFiles,
  onTaskContextAction,
}: ProjectTaskTreeProps): ReactElement {
  const { t } = useTranslation();
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [filterMode, setFilterMode] = useState<ProjectTaskFilterMode>("all");
  const [groupMode, setGroupMode] = useState<ProjectTaskGroupMode>("project");
  const [expandedProjectIds, setExpandedProjectIds] = useState<ReadonlySet<string>>(
    () => new Set(projects.map((p) => p.projectId)),
  );
  const [hoveredProjectId, setHoveredProjectId] = useState<string | null>(null);
  const [openContextTaskId, setOpenContextTaskId] = useState<string | null>(null);

  const visibleProjects = filterVisibleSidebarProjects(projects, searchQuery);
  const visibleTasks = filterTasksForSidebar(tasks, searchQuery, filterMode);

  const toggleProjectExpanded = (projectId: string) => {
    setExpandedProjectIds((prev) => {
      const next = new Set(prev);
      if (next.has(projectId)) {
        next.delete(projectId);
      } else {
        next.add(projectId);
      }
      return next;
    });
  };

  const cycleFilterMode = () => {
    const order: readonly ProjectTaskFilterMode[] = [
      "all",
      "active",
      "pinned",
      "unread",
      "running",
    ];
    const nextIdx = (order.indexOf(filterMode) + 1) % order.length;
    setFilterMode(order[nextIdx]);
  };

  const renderTaskRow = (task: TaskSessionSummary) => {
    const isSelected = task.taskId === activeTaskId;
    const isContextOpen = openContextTaskId === task.taskId;

    return (
      <div
        key={task.taskId}
        style={{ position: "relative", display: "flex", alignItems: "center" }}
        onContextMenu={(e) => {
          e.preventDefault();
          setOpenContextTaskId(task.taskId);
        }}
      >
        <button
          type="button"
          className={`ky-btn ky-btn--ghost ky-btn--sm ${
            isSelected ? "ky-btn--selected" : ""
          }`}
          onClick={() => onSelectTask(task)}
          style={{
            flex: 1,
            minWidth: 0,
            justifyContent: "flex-start",
            gap: "6px",
          }}
        >
          {task.status === "running" ? (
            <span
              className="animated-gradient-text"
              aria-label="Running"
              style={{ fontSize: "0.75rem", fontWeight: 700 }}
            >
              ●
            </span>
          ) : task.isPinned ? (
            <ChromeIcon name="pin" size={14} />
          ) : (
            <ChromeIcon name="file-text" size={14} />
          )}
          <span
            style={{
              flex: 1,
              textAlign: "start",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              fontWeight: task.isUnread ? 600 : 400,
            }}
          >
            {task.title}
          </span>
          <span
            style={{
              fontSize: "0.7rem",
              color: "var(--text-muted)",
              fontVariantNumeric: "tabular-nums",
            }}
          >
            {task.updatedAt}
          </span>
        </button>

        <Button
          variant="ghost"
          size="sm"
          aria-label={`More actions for ${task.title}`}
          onClick={() =>
            setOpenContextTaskId((prev) =>
              prev === task.taskId ? null : task.taskId,
            )
          }
        >
          <ChromeIcon name="more" size={14} />
        </Button>

        <TaskContextMenu
          task={task}
          isOpen={isContextOpen}
          onAction={onTaskContextAction}
          onClose={() => setOpenContextTaskId(null)}
        />
      </div>
    );
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
      <div className="ky-sidebar__section-header">
        <span>{t("ui.sidebar.taskList", { defaultValue: "Task list" })}</span>
        <div style={{ display: "inline-flex", alignItems: "center", gap: "2px" }}>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Collapse all projects"
            title="Collapse all projects"
            onClick={() =>
              setExpandedProjectIds((prev) =>
                prev.size > 0
                  ? new Set()
                  : new Set(projects.map((p) => p.projectId)),
              )
            }
          >
            <ChromeIcon name="minimize" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Add project folder"
            title="Add project"
            onClick={onAddProject}
          >
            <ChromeIcon name="plus" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label="Search projects and tasks"
            title="Search"
            selected={searchOpen}
            onClick={() => setSearchOpen((prev) => !prev)}
          >
            <ChromeIcon name="search" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label={`Filter tasks: ${filterMode}`}
            title={`Filter: ${filterMode}`}
            selected={filterMode !== "all"}
            onClick={cycleFilterMode}
          >
            <ChromeIcon name="filter" size={14} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label={`Group by: ${groupMode}`}
            title={`Group by: ${groupMode}`}
            onClick={() =>
              setGroupMode((prev) => (prev === "project" ? "flat" : "project"))
            }
          >
            <ChromeIcon name="group-by" size={14} />
          </Button>
        </div>
      </div>

      {searchOpen && (
        <Input
          aria-label="Search projects and tasks"
          placeholder="Filter projects or tasks..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      )}

      {groupMode === "project" ? (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
          {visibleProjects.length === 0 ? (
            <div
              style={{
                padding: "var(--space-2)",
                fontSize: "0.75rem",
                color: "var(--text-muted)",
              }}
            >
              No projects match current filter.
            </div>
          ) : (
            visibleProjects.map((project) => {
              const isExpanded = expandedProjectIds.has(project.projectId);
              const projectTasks = visibleTasks.filter(
                (t) => t.projectId === project.projectId,
              );

              return (
                <div
                  key={project.projectId}
                  style={{ position: "relative" }}
                  onMouseEnter={() => setHoveredProjectId(project.projectId)}
                  onMouseLeave={() =>
                    setHoveredProjectId((prev) =>
                      prev === project.projectId ? null : prev,
                    )
                  }
                >
                  <div className="ky-project-row-wrap">
                    <button
                      type="button"
                      aria-expanded={isExpanded ? "true" : "false"}
                      className="ky-btn ky-btn--ghost ky-btn--sm"
                      style={{
                        flex: 1,
                        minWidth: 0,
                        justifyContent: "flex-start",
                        fontWeight: 500,
                      }}
                      onClick={() => toggleProjectExpanded(project.projectId)}
                    >
                      <ChromeIcon
                        name={isExpanded ? "chevron-down" : "chevron-right"}
                        size={14}
                      />
                      <ChromeIcon
                        name={isExpanded ? "project-open" : "projects"}
                        size={14}
                      />
                      <span
                        style={{
                          flex: 1,
                          textAlign: "start",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {project.name}
                      </span>
                      <span
                        style={{
                          fontSize: "0.7rem",
                          color: "var(--text-muted)",
                        }}
                      >
                        {projectTasks.length}
                      </span>
                    </button>
                    <div className="ky-project-row-actions">
                      {onShowProjectFiles && (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`Show files for ${project.name}`}
                          title="Show files"
                          onClick={() => onShowProjectFiles(project)}
                        >
                          <ChromeIcon name="file-code" size={14} />
                        </Button>
                      )}
                      {onNewTask && (
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`New task in ${project.name}`}
                          title="New task"
                          onClick={onNewTask}
                        >
                          <ChromeIcon name="plus" size={14} />
                        </Button>
                      )}
                    </div>
                  </div>

                  {hoveredProjectId === project.projectId && (
                    <div
                      style={{
                        position: "absolute",
                        top: 0,
                        insetInlineStart: "calc(100% + 6px)",
                        zIndex: 45,
                      }}
                    >
                      <ProjectHoverCard project={project} />
                    </div>
                  )}

                  {isExpanded && (
                    <div
                      style={{
                        paddingInlineStart: "var(--space-3)",
                        display: "flex",
                        flexDirection: "column",
                        gap: "2px",
                      }}
                    >
                      {projectTasks.map(renderTaskRow)}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
          <div className="ky-sidebar__section-header">
            <span>Tasks ({visibleTasks.length})</span>
          </div>
          {visibleTasks.map(renderTaskRow)}
        </div>
      )}
    </div>
  );
}
