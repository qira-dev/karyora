/**
 * File Activity & Diff Treemapping Tab (1:1 ZCode v3.18.2 `treemapping` side-pane tab).
 */

import type { ReactElement } from "react";
import { MaterialFileIcon } from "../components/icons";
import { Badge } from "../components/primitives";
import type { DarkCodeTheme, LightCodeTheme } from "../types/decisions";
import type { ReviewDiffFileRecord, WorkspaceFileNode } from "./sidePaneConfig";

export interface TreemappingTabProps {
  readonly files: readonly WorkspaceFileNode[];
  readonly diffFiles: readonly ReviewDiffFileRecord[];
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly onOpenFile: (filePath: string) => void;
}

type ActivityKind = "modified" | "written" | "deleted" | "viewed";

interface TreemapTile {
  readonly path: string;
  readonly name: string;
  readonly kind: ActivityKind;
  readonly weight: number;
  readonly additions: number;
  readonly deletions: number;
}

export function TreemappingTab({
  files,
  diffFiles,
  activeCodeTheme,
  onOpenFile,
}: TreemappingTabProps): ReactElement {
  const diffByPath = new Map(diffFiles.map((d) => [d.filePath, d]));
  const leafFiles = files.filter((f) => !f.isDirectory);

  const tiles: TreemapTile[] = leafFiles.map((file) => {
    const diff = diffByPath.get(file.path);
    const additions = diff ? diff.newText.split("\n").length : 0;
    const deletions = diff && diff.oldText ? diff.oldText.split("\n").length : 0;
    const kind: ActivityKind =
      file.gitStatus === "A"
        ? "written"
        : file.gitStatus === "D"
          ? "deleted"
          : file.gitStatus === "M" || diff
            ? "modified"
            : "viewed";
    const weight = Math.max(1, additions + deletions + (file.content?.split("\n").length ?? 4));
    return {
      path: file.path,
      name: file.name,
      kind,
      weight,
      additions,
      deletions,
    };
  });

  const modifiedCount = tiles.filter((t) => t.kind === "modified" || t.kind === "written").length;
  const totalDiffLines = tiles.reduce((acc, t) => acc + t.additions + t.deletions, 0);

  return (
    <section
      className="ky-side-tool-page"
      data-testid="treemapping-tab"
      aria-label="Turn file activity treemapping"
    >
      <div className="ky-side-tool-page__inner">
        <div className="ky-side-tool-page__head-row">
          <div>
            <h3 className="ky-side-tool-page__subhead">Treemapping</h3>
            <p className="ky-setting-row__desc">
              Current turn tool activity across inspected and changed workspace files.
            </p>
          </div>
          <Badge tone="info" dot label="Live" />
        </div>

        <div className="ky-treemap-summary">
          <div className="ky-treemap-stat">
            <span className="ky-treemap-stat__label">Files</span>
            <strong className="ky-treemap-stat__val">{tiles.length}</strong>
          </div>
          <div className="ky-treemap-stat">
            <span className="ky-treemap-stat__label">Changed</span>
            <strong className="ky-treemap-stat__val">{modifiedCount}</strong>
          </div>
          <div className="ky-treemap-stat">
            <span className="ky-treemap-stat__label">Diff lines</span>
            <strong className="ky-treemap-stat__val">{totalDiffLines}</strong>
          </div>
        </div>

        <div className="ky-treemap-grid">
          {tiles.map((tile) => (
            <button
              key={tile.path}
              type="button"
              className="ky-treemap-tile"
              data-activity={tile.kind}
              style={{ flexGrow: Math.min(6, Math.max(1, Math.round(tile.weight / 4))) }}
              onClick={() => onOpenFile(tile.path)}
              title={tile.path}
            >
              <div className="ky-treemap-tile__top">
                <MaterialFileIcon
                  filename={tile.path}
                  codeTheme={activeCodeTheme}
                  size={16}
                />
                <span className="ky-treemap-tile__name">{tile.name}</span>
              </div>
              <span className="ky-treemap-tile__path">{tile.path}</span>
              <div className="ky-treemap-tile__foot">
                <span className="ky-treemap-tile__badge">{tile.kind}</span>
                {(tile.additions > 0 || tile.deletions > 0) && (
                  <span className="ky-diff-pill">
                    <span className="ky-diff-pill__added">+{tile.additions}</span>
                    <span className="ky-diff-pill__removed">-{tile.deletions}</span>
                  </span>
                )}
              </div>
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}

export default TreemappingTab;
