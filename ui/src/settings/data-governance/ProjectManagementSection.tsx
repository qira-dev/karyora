/**
 * Settings Section 19: Project Management (UIR-8.3, Screenshot #58, E2E-20).
 *
 * Active projects table with search/filter/sort, multi-select removal (never touches
 * files on disk!), and collapsible `Removed projects` with `Restore`.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Input } from "../../components/primitives";
import type { WorkspaceProjectSummary } from "../../coreclient";

export interface ProjectManagementSectionProps {
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly onNotify: (msg: string) => void;
}

export function ProjectManagementSection({
  projects,
  onNotify,
}: ProjectManagementSectionProps): ReactElement {
  const [search, setSearch] = useState("");
  const [removedIds, setRemovedIds] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(() => new Set());
  const [showRemoved, setShowRemoved] = useState(true);

  const activeProjects = projects.filter(
    (p) =>
      !removedIds.has(p.projectId) &&
      (p.name.toLowerCase().includes(search.trim().toLowerCase()) ||
        p.path.toLowerCase().includes(search.trim().toLowerCase())),
  );
  const removedProjects = projects.filter((p) => removedIds.has(p.projectId));

  const handleRemoveSelected = () => {
    setRemovedIds((prev) => {
      const next = new Set(prev);
      for (const id of selectedIds) next.add(id);
      return next;
    });
    setSelectedIds(new Set());
    onNotify("Removed selected project(s) from sidebar only — files on disk untouched.");
  };

  return (
    <div className="ky-settings-content" data-testid="settings-section-project-management">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <Input
            aria-label="Search managed projects"
            placeholder="Search projects..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button
            size="sm"
            variant="secondary"
            disabled={selectedIds.size === 0}
            onClick={handleRemoveSelected}
          >
            Remove selected ({selectedIds.size}) — keeps files on disk
          </Button>
        </div>

        <table className="ky-step-table">
          <thead>
            <tr>
              <th />
              <th>Project</th>
              <th>Path</th>
              <th>Sessions</th>
            </tr>
          </thead>
          <tbody>
            {activeProjects.map((proj) => (
              <tr key={proj.projectId}>
                <td>
                  <input
                    type="checkbox"
                    aria-label={`Select ${proj.name}`}
                    checked={selectedIds.has(proj.projectId)}
                    onChange={(e) =>
                      setSelectedIds((prev) => {
                        const next = new Set(prev);
                        if (e.target.checked) next.add(proj.projectId);
                        else next.delete(proj.projectId);
                        return next;
                      })
                    }
                  />
                </td>
                <td>
                  <strong>{proj.name}</strong>
                </td>
                <td style={{ fontFamily: "var(--font-mono)", fontSize: "0.74rem" }}>{proj.path}</td>
                <td>
                  <Badge tone="neutral" label={`${proj.sessionCount} sessions`} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">
            Removed projects ({removedProjects.length})
          </h3>
          <Button size="sm" variant="ghost" onClick={() => setShowRemoved((v) => !v)}>
            {showRemoved ? "Collapse" : "Expand"}
          </Button>
        </div>

        {showRemoved &&
          (removedProjects.length === 0 ? (
            <p style={{ margin: 0, fontSize: "0.8rem", color: "var(--text-muted)" }}>
              No removed projects.
            </p>
          ) : (
            removedProjects.map((proj) => (
              <div key={proj.projectId} className="ky-gov-perm-row">
                <span>
                  <strong>{proj.name}</strong> — <code>{proj.path}</code>
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    setRemovedIds((prev) => {
                      const next = new Set(prev);
                      next.delete(proj.projectId);
                      return next;
                    });
                    onNotify(`Restored project "${proj.name}".`);
                  }}
                >
                  ↻ Restore
                </Button>
              </div>
            ))
          ))}
      </div>
    </div>
  );
}
