/**
 * Settings Section 19: Project Management — Calibrated 1:1 to ZCode Screenshot #58.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, Input } from "../../components/primitives";
import type { WorkspaceProjectSummary } from "../../coreclient";

export interface ProjectManagementSectionProps {
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly onNotify: (msg: string) => void;
}

export function ProjectManagementSection({
  projects,
  onNotify,
}: ProjectManagementSectionProps): ReactElement {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [removedIds, setRemovedIds] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(() => new Set());
  const [showRemoved, setShowRemoved] = useState(true);

  const q = search.trim().toLowerCase();
  const activeProjects = projects.filter(
    (p) =>
      !removedIds.has(p.projectId) &&
      (!q || p.name.toLowerCase().includes(q) || p.path.toLowerCase().includes(q)),
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
      <p className="ky-settings-section-sub">
        {t("ui.settings.dataGov.projectsSubtitle", {
          defaultValue:
            "Removing a project takes it out of the sidebar and the project picker. Its sessions stay in Recent and search, and nothing on disk is touched.",
        })}
      </p>

      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left" style={{ flex: 1, minWidth: "220px" }}>
          <Input
            aria-label="Search managed projects"
            placeholder={t("ui.settings.dataGov.searchProjects", {
              defaultValue: "Search projects, folders or devices",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="ky-settings-toolbar__right">
          {selectedIds.size > 0 && (
            <Button size="sm" variant="secondary" onClick={handleRemoveSelected}>
              Remove ({selectedIds.size})
            </Button>
          )}
        </div>
      </div>

      <div className="ky-settings-list">
        <table className="ky-settings-table">
          <thead>
            <tr>
              <th style={{ width: "36px" }} />
              <th>Project</th>
              <th>Device</th>
              <th>Sessions</th>
              <th>Last active</th>
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
                  <div style={{ display: "flex", flexDirection: "column", gap: "2px" }}>
                    <strong>{proj.name}</strong>
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: "0.73rem",
                        color: "var(--text-muted)",
                      }}
                    >
                      {proj.path}
                    </span>
                  </div>
                </td>
                <td style={{ color: "var(--text-secondary)", fontSize: "0.8rem" }}>
                  This computer
                </td>
                <td style={{ color: "var(--text-secondary)" }}>{proj.sessionCount}</td>
                <td style={{ color: "var(--text-muted)", fontSize: "0.8rem" }}>
                  {proj.lastActiveAt}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="ky-settings-group-header">
        <button
          type="button"
          className="ky-settings-breadcrumb__link"
          style={{ display: "inline-flex", alignItems: "center", gap: "6px", fontWeight: 600 }}
          onClick={() => setShowRemoved((v) => !v)}
        >
          <ChromeIcon name={showRemoved ? "chevron-down" : "chevron-right"} size={14} />
          <span>
            {t("ui.settings.dataGov.removedProjects", { defaultValue: "Removed projects" })}
          </span>
        </button>
        <span className="ky-settings-group-header__count">
          {removedProjects.length} {removedProjects.length === 1 ? "project" : "projects"}
        </span>
      </div>

      {showRemoved && removedProjects.length > 0 && (
        <div className="ky-settings-list">
          {removedProjects.map((proj) => (
            <div key={proj.projectId} className="ky-settings-list-row">
              <div className="ky-settings-list-row__copy">
                <span className="ky-settings-list-row__title">{proj.name}</span>
                <span
                  className="ky-settings-list-row__desc"
                  style={{ fontFamily: "var(--font-mono)" }}
                >
                  {proj.path}
                </span>
              </div>
              <div className="ky-settings-list-row__right">
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
                  <ChromeIcon name="refresh" size={14} />
                  <span>{t("ui.settings.dataGov.restore", { defaultValue: "Restore" })}</span>
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
