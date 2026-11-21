/**
 * Left Sidebar Project Hover Card (UIR-3.2, Screenshot #84, Blueprint §4.3 (4)).
 *
 * Displays exact project metadata on hover/focus:
 * - Project name + device indicator
 * - Full filesystem path
 * - Relative `Last active` timestamp
 * - Current Git branch badge (`⎇ frontend-rebuild`)
 * - Session count
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge } from "../components/primitives";
import type { WorkspaceProjectSummary } from "../coreclient/types";

export interface ProjectHoverCardProps {
  readonly project: WorkspaceProjectSummary;
}

export function ProjectHoverCard({ project }: ProjectHoverCardProps): ReactElement {
  return (
    <div
      role="tooltip"
      data-testid={`project-hover-card-${project.projectId}`}
      className="ky-popover-surface ky-project-hover-card"
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-2)",
            fontWeight: 600,
            fontSize: "0.875rem",
          }}
        >
          <ChromeIcon name="projects" size={16} />
          <span>{project.name}</span>
        </div>
        <Badge
          tone={project.isRemoteDevice ? "info" : "neutral"}
          label={project.deviceName}
        />
      </div>

      <code
        style={{
          fontSize: "0.75rem",
          color: "var(--text-muted)",
          wordBreak: "break-all",
          fontFamily: "var(--font-mono)",
        }}
      >
        {project.path}
      </code>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
          fontSize: "0.75rem",
          color: "var(--text-secondary)",
        }}
      >
        <span>Last active: {project.lastActiveAt}</span>
        <span>{project.sessionCount} sessions</span>
      </div>

      {project.gitBranch && (
        <div
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "var(--space-1)",
            fontSize: "0.75rem",
            color: "var(--accent-text)",
            fontFamily: "var(--font-mono)",
          }}
        >
          <ChromeIcon name="git-branch" size={14} />
          <span>{project.gitBranch}</span>
        </div>
      )}
    </div>
  );
}
