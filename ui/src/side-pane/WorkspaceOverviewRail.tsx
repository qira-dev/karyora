/**
 * Antigravity-Style Right-Side `Overview` Panel with `⋮ Customize Sections` Popover Menu.
 *
 * Matches the Antigravity Workspace right-side Overview layout:
 * - Header: `Overview` + `⋮` three-dot button opening `Customize Sections` checklist
 * - Top Quick Actions: `Review`, `Files`, `Browser`, `Terminal`, `Extensions >`
 * - Customizable Live Sections: `Subagents`, `Background Tasks`, `Goals`, `Documents`,
 *   `Artifacts`, `Links`, `Modified Files` (with Material SVG icons), `Uploads`,
 *   `Skills Used`, plus Karyora Core sections (`Knowledge`, `Memory`, `Context`, `Activity & Audit`)
 *   with live count badges and `See all` drill-down actions.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon, MaterialFileIcon } from "../components/icons";
import type { AuditEventRecord } from "../domains/activity";
import type { ContextSnapshotRecord } from "../domains/context";
import type { KnowledgePackRecord } from "../domains/knowledge";
import type { ScopedMemoryItem } from "../domains/memory";
import type { MarketplacePluginRecord } from "../marketplace";
import type { DarkCodeTheme, LightCodeTheme } from "../types/decisions";
import type {
  DurableJobRecord,
  TimelineProgressStep,
} from "../workspace/workspaceConfig";
import { DEFAULT_RECENT_VISITS } from "./browserConfig";
import {
  OVERVIEW_SECTION_DEFINITIONS,
  type OverviewArtifactItem,
  type OverviewSectionId,
  type OverviewSectionVisibilityMap,
  type OverviewUploadItem,
} from "./overviewConfig";
import type {
  ReviewDiffFileRecord,
  SubagentRunRecord,
  WorkspaceFileNode,
} from "./sidePaneConfig";

export interface WorkspaceOverviewRailProps {
  readonly visibility: OverviewSectionVisibilityMap;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly subagents: readonly SubagentRunRecord[];
  readonly backgroundJobs: readonly DurableJobRecord[];
  readonly goalSteps: readonly TimelineProgressStep[];
  readonly workspaceFiles: readonly WorkspaceFileNode[];
  readonly artifacts: readonly OverviewArtifactItem[];
  readonly modifiedFiles: readonly ReviewDiffFileRecord[];
  readonly uploads: readonly OverviewUploadItem[];
  readonly plugins: readonly MarketplacePluginRecord[];
  readonly knowledgePacks: readonly KnowledgePackRecord[];
  readonly memoryItems: readonly ScopedMemoryItem[];
  readonly contextSnapshots: readonly ContextSnapshotRecord[];
  readonly auditEvents: readonly AuditEventRecord[];
  readonly onToggleSection: (sectionId: OverviewSectionId) => void;
  readonly onOpenReview: (filePath?: string) => void;
  readonly onOpenFiles: (filePath?: string) => void;
  readonly onOpenBrowser: (url?: string) => void;
  readonly onOpenTerminal: () => void;
  readonly onOpenExtensions: () => void;
  readonly onInspectSubagent: (subagent: SubagentRunRecord) => void;
  readonly onOpenSubagentsDirectory: () => void;
  readonly onOpenJobsSurface: () => void;
  readonly onOpenDocumentPreview: (filePath: string, title: string) => void;
  readonly onOpenKnowledgeSurface: () => void;
  readonly onOpenMemorySurface: () => void;
  readonly onOpenContextSurface: () => void;
  readonly onOpenActivitySurface: () => void;
}

function splitFileDisplay(filePath: string): { readonly name: string; readonly dir: string } {
  const idx = filePath.lastIndexOf("/");
  if (idx === -1) return { name: filePath, dir: "" };
  return { name: filePath.slice(idx + 1), dir: filePath.slice(0, idx) };
}

export function WorkspaceOverviewRail({
  visibility,
  activeCodeTheme,
  subagents,
  backgroundJobs,
  goalSteps,
  workspaceFiles,
  artifacts,
  modifiedFiles,
  uploads,
  plugins,
  knowledgePacks,
  memoryItems,
  contextSnapshots,
  auditEvents,
  onToggleSection,
  onOpenReview,
  onOpenFiles,
  onOpenBrowser,
  onOpenTerminal,
  onOpenExtensions,
  onInspectSubagent,
  onOpenSubagentsDirectory,
  onOpenJobsSurface,
  onOpenDocumentPreview,
  onOpenKnowledgeSurface,
  onOpenMemorySurface,
  onOpenContextSurface,
  onOpenActivitySurface,
}: WorkspaceOverviewRailProps): ReactElement {
  const { t } = useTranslation();
  const [customizeOpen, setCustomizeOpen] = useState(false);

  const documentNodes = workspaceFiles.filter(
    (f) => !f.isDirectory && /\.(pdf|docx|xlsx|pptx)$/i.test(f.path),
  );
  const activePlugins = plugins.filter((p) => p.lifecycleState === "installed-enabled");
  const activeMemories = memoryItems.filter((m) => m.state === "active");

  return (
    <aside
      aria-label={t("ui.overview.title", { defaultValue: "Overview" })}
      data-testid="workspace-overview-rail"
      className="ky-overview-rail"
    >
      {/* Top Header: `Overview` + `⋮` Three-Dot Customize Sections Menu */}
      <header className="ky-overview-rail__header">
        <span className="ky-overview-rail__title">
          {t("ui.overview.title", { defaultValue: "Overview" })}
        </span>
        <div style={{ position: "relative" }}>
          <button
            type="button"
            className="ky-overview-rail__more-btn"
            aria-haspopup="menu"
            aria-expanded={customizeOpen}
            aria-label={t("ui.overview.customizeSections", {
              defaultValue: "Customize Sections",
            })}
            title={t("ui.overview.customizeSections", {
              defaultValue: "Customize Sections",
            })}
            data-testid="overview-customize-btn"
            onClick={() => setCustomizeOpen((prev) => !prev)}
          >
            <ChromeIcon name="more-vertical" size={14} />
          </button>

          {customizeOpen && (
            <div
              role="menu"
              aria-label="Customize overview sections"
              data-testid="overview-customize-menu"
              className="ky-popover-surface ky-overview-customize-menu"
            >
              <div className="ky-overview-customize-menu__heading">
                {t("ui.overview.customizeSections", {
                  defaultValue: "Customize Sections",
                })}
              </div>

              {OVERVIEW_SECTION_DEFINITIONS.filter((d) => d.group === "workspace").map(
                (def) => {
                  const checked = visibility[def.id];
                  return (
                    <button
                      key={def.id}
                      type="button"
                      role="menuitemcheckbox"
                      aria-checked={checked}
                      className="ky-overview-customize-menu__item"
                      data-testid={`overview-toggle-${def.id}`}
                      onClick={() => onToggleSection(def.id)}
                    >
                      <span>{def.label}</span>
                      {checked && <ChromeIcon name="check" size={14} />}
                    </button>
                  );
                },
              )}

              <hr
                role="separator"
                style={{
                  border: "none",
                  borderTop: "1px solid var(--border-subtle)",
                  margin: "4px 0",
                }}
              />

              <div className="ky-overview-customize-menu__heading">
                {t("ui.overview.coreSections", { defaultValue: "Core & Governance" })}
              </div>

              {OVERVIEW_SECTION_DEFINITIONS.filter((d) => d.group === "core").map((def) => {
                const checked = visibility[def.id];
                return (
                  <button
                    key={def.id}
                    type="button"
                    role="menuitemcheckbox"
                    aria-checked={checked}
                    className="ky-overview-customize-menu__item"
                    data-testid={`overview-toggle-${def.id}`}
                    onClick={() => onToggleSection(def.id)}
                  >
                    <span>{def.label}</span>
                    {checked && <ChromeIcon name="check" size={14} />}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      </header>

      <div className="ky-overview-rail__scroll">
        {/* Top Quick-Launch Tool Rows (`Review`, `Files`, `Browser`, `Terminal`, `Extensions >`) */}
        <nav aria-label="Workspace quick tools" className="ky-overview-rail__tools">
          <button
            type="button"
            className="ky-overview-rail__tool-row"
            data-testid="overview-tool-review"
            onClick={() => onOpenReview()}
          >
            <ChromeIcon name="file-diff" size={14} />
            <span>{t("ui.overview.review", { defaultValue: "Review" })}</span>
          </button>
          <button
            type="button"
            className="ky-overview-rail__tool-row"
            data-testid="overview-tool-files"
            onClick={() => onOpenFiles()}
          >
            <ChromeIcon name="file-text" size={14} />
            <span>{t("ui.overview.files", { defaultValue: "Files" })}</span>
          </button>
          <button
            type="button"
            className="ky-overview-rail__tool-row"
            data-testid="overview-tool-browser"
            onClick={() => onOpenBrowser()}
          >
            <ChromeIcon name="browser-globe" size={14} />
            <span>{t("ui.browser.title", { defaultValue: "Browser" })}</span>
          </button>
          <button
            type="button"
            className="ky-overview-rail__tool-row"
            data-testid="overview-tool-terminal"
            onClick={onOpenTerminal}
          >
            <ChromeIcon name="terminal" size={14} />
            <span>{t("ui.overview.terminal", { defaultValue: "Terminal" })}</span>
          </button>
          <button
            type="button"
            className="ky-overview-rail__tool-row ky-overview-rail__tool-row--split"
            data-testid="overview-tool-extensions"
            onClick={onOpenExtensions}
          >
            <span style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
              <ChromeIcon name="plugin-marketplace" size={14} />
              <span>{t("ui.overview.extensions", { defaultValue: "Extensions" })}</span>
            </span>
            <ChromeIcon name="chevron-right" size={14} />
          </button>
        </nav>

        {/* Subagents */}
        {visibility.subagents && (
          <section className="ky-overview-section" data-testid="overview-section-subagents">
            <button
              type="button"
              className="ky-overview-section__head"
              onClick={onOpenSubagentsDirectory}
            >
              <span>{t("ui.overview.subagents", { defaultValue: "Subagents" })}</span>
              <span className="ky-overview-section__count">{subagents.length}</span>
            </button>
            <div className="ky-overview-section__list">
              {subagents.slice(0, 3).map((sub) => (
                <button
                  key={sub.subagentId}
                  type="button"
                  className="ky-overview-item"
                  onClick={() => onInspectSubagent(sub)}
                >
                  <span
                    className="ky-overview-dot"
                    data-status={sub.status}
                    aria-hidden="true"
                  />
                  <span className="ky-overview-item__primary">{sub.name}</span>
                </button>
              ))}
            </div>
          </section>
        )}

        {/* Background Tasks (Durable Jobs) */}
        {visibility["background-tasks"] && (
          <section
            className="ky-overview-section"
            data-testid="overview-section-background-tasks"
          >
            <div className="ky-overview-section__head-static">
              <span>
                {t("ui.overview.backgroundTasks", { defaultValue: "Background Tasks" })}
              </span>
              <span className="ky-overview-section__count">{backgroundJobs.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {backgroundJobs.slice(0, 2).map((job) => (
                <button
                  key={job.jobId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenJobsSurface}
                  title={job.title}
                >
                  <ChromeIcon name="terminal" size={14} />
                  <span className="ky-overview-item__primary">{job.title}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={onOpenJobsSurface}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {/* Goals */}
        {visibility.goals && (
          <section className="ky-overview-section" data-testid="overview-section-goals">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.goals", { defaultValue: "Goals" })}</span>
              <span className="ky-overview-section__count">{goalSteps.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {goalSteps.map((step) => (
                <div key={step.stepId} className="ky-overview-item ky-overview-item--static">
                  <ChromeIcon name={step.completed ? "check" : "target"} size={14} />
                  <span className="ky-overview-item__primary">{step.label}</span>
                </div>
              ))}
            </div>
          </section>
        )}

        {/* Documents */}
        {visibility.documents && (
          <section className="ky-overview-section" data-testid="overview-section-documents">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.documents", { defaultValue: "Documents" })}</span>
              <span className="ky-overview-section__count">{documentNodes.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {documentNodes.slice(0, 4).map((doc) => (
                <button
                  key={doc.path}
                  type="button"
                  className="ky-overview-item"
                  onClick={() => onOpenDocumentPreview(doc.path, doc.name)}
                >
                  <MaterialFileIcon
                    filename={doc.path}
                    codeTheme={activeCodeTheme}
                    size={16}
                  />
                  <span className="ky-overview-item__primary">{doc.name}</span>
                </button>
              ))}
            </div>
          </section>
        )}

        {/* Artifacts */}
        {visibility.artifacts && (
          <section className="ky-overview-section" data-testid="overview-section-artifacts">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.artifacts", { defaultValue: "Artifacts" })}</span>
              <span className="ky-overview-section__count">{artifacts.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {artifacts.slice(0, 3).map((art) => (
                <button
                  key={art.id}
                  type="button"
                  className="ky-overview-item"
                  onClick={() => onOpenDocumentPreview(art.filePath, art.title)}
                >
                  <ChromeIcon name="file-text" size={14} />
                  <span className="ky-overview-item__primary">{art.title}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={() => onOpenFiles()}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {/* Links (Opens directly inside Embedded Browser) */}
        {visibility.links && (
          <section className="ky-overview-section" data-testid="overview-section-links">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.links", { defaultValue: "Links" })}</span>
              <span className="ky-overview-section__count">
                {DEFAULT_RECENT_VISITS.length}
              </span>
            </div>
            <div className="ky-overview-section__list">
              {DEFAULT_RECENT_VISITS.map((link) => (
                <button
                  key={link.url}
                  type="button"
                  className="ky-overview-item"
                  onClick={() => onOpenBrowser(link.url)}
                >
                  <ChromeIcon name="link" size={14} />
                  <span className="ky-overview-item__primary">{link.title}</span>
                </button>
              ))}
            </div>
          </section>
        )}

        {/* Modified Files (with Material SVG language icons + muted directory path) */}
        {visibility["modified-files"] && (
          <section
            className="ky-overview-section"
            data-testid="overview-section-modified-files"
          >
            <div className="ky-overview-section__head-static">
              <span>
                {t("ui.overview.modifiedFiles", { defaultValue: "Modified Files" })}
              </span>
              <span className="ky-overview-section__count">{modifiedFiles.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {modifiedFiles.slice(0, 4).map((mf) => {
                const { name, dir } = splitFileDisplay(mf.filePath);
                return (
                  <button
                    key={mf.filePath}
                    type="button"
                    className="ky-overview-item"
                    onClick={() => onOpenReview(mf.filePath)}
                    title={mf.filePath}
                  >
                    <MaterialFileIcon
                      filename={mf.filePath}
                      codeTheme={activeCodeTheme}
                      size={16}
                    />
                    <span className="ky-overview-item__primary">{name}</span>
                    {dir && <span className="ky-overview-item__secondary">{dir}</span>}
                  </button>
                );
              })}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={() => onOpenReview()}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {/* Uploads */}
        {visibility.uploads && (
          <section className="ky-overview-section" data-testid="overview-section-uploads">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.uploads", { defaultValue: "Uploads" })}</span>
              <span className="ky-overview-section__count">{uploads.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {uploads.slice(0, 4).map((up) => (
                <button
                  key={up.id}
                  type="button"
                  className="ky-overview-item"
                  onClick={() => onOpenFiles()}
                >
                  <ChromeIcon name="upload" size={14} />
                  <span className="ky-overview-item__primary">{up.label}</span>
                  <span className="ky-overview-item__secondary">{up.timestamp}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={() => onOpenFiles()}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {/* Skills Used */}
        {visibility["skills-used"] && (
          <section className="ky-overview-section" data-testid="overview-section-skills-used">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.overview.skillsUsed", { defaultValue: "Skills Used" })}</span>
              <span className="ky-overview-section__count">{activePlugins.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {activePlugins.slice(0, 4).map((p) => (
                <button
                  key={p.pluginId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenExtensions}
                >
                  <ChromeIcon name="skills" size={14} />
                  <span className="ky-overview-item__primary">{p.name}</span>
                </button>
              ))}
            </div>
          </section>
        )}

        {/* Karyora Core Optional Sections (Toggleable via `⋮ Customize Sections`) */}
        {visibility.knowledge && (
          <section className="ky-overview-section" data-testid="overview-section-knowledge">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.sidebar.knowledge", { defaultValue: "Knowledge" })}</span>
              <span className="ky-overview-section__count">{knowledgePacks.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {knowledgePacks.slice(0, 3).map((kp) => (
                <button
                  key={kp.packId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenKnowledgeSurface}
                >
                  <ChromeIcon name="database" size={14} />
                  <span className="ky-overview-item__primary">{kp.name}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={onOpenKnowledgeSurface}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {visibility.memory && (
          <section className="ky-overview-section" data-testid="overview-section-memory">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.sidebar.memory", { defaultValue: "Memory" })}</span>
              <span className="ky-overview-section__count">{activeMemories.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {activeMemories.slice(0, 3).map((mem) => (
                <button
                  key={mem.memoryId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenMemorySurface}
                >
                  <ChromeIcon name="brain-thinking" size={14} />
                  <span className="ky-overview-item__primary">{mem.title}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={onOpenMemorySurface}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {visibility.context && (
          <section className="ky-overview-section" data-testid="overview-section-context">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.sidebar.context", { defaultValue: "Context" })}</span>
              <span className="ky-overview-section__count">{contextSnapshots.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {contextSnapshots.slice(0, 2).map((snap) => (
                <button
                  key={snap.snapshotId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenContextSurface}
                >
                  <ChromeIcon name="sliders" size={14} />
                  <span className="ky-overview-item__primary">
                    {snap.turnLabel} ({snap.usedTokens.toLocaleString()} tok)
                  </span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={onOpenContextSurface}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}

        {visibility.activity && (
          <section className="ky-overview-section" data-testid="overview-section-activity">
            <div className="ky-overview-section__head-static">
              <span>{t("ui.sidebar.activityAudit", { defaultValue: "Activity & Audit" })}</span>
              <span className="ky-overview-section__count">{auditEvents.length}</span>
            </div>
            <div className="ky-overview-section__list">
              {auditEvents.slice(0, 3).map((evt) => (
                <button
                  key={evt.eventId}
                  type="button"
                  className="ky-overview-item"
                  onClick={onOpenActivitySurface}
                >
                  <ChromeIcon name="activity" size={14} />
                  <span className="ky-overview-item__primary">{evt.action}</span>
                </button>
              ))}
            </div>
            <button
              type="button"
              className="ky-overview-see-all"
              onClick={onOpenActivitySurface}
            >
              {t("ui.overview.seeAll", { defaultValue: "See all" })}
            </button>
          </section>
        )}
      </div>
    </aside>
  );
}
