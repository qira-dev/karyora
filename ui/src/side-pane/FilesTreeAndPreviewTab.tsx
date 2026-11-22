/**
 * Side-Pane `Files` Split View: 50,000+ Virtualized File Tree + Line-Numbered Code /
 * Sandboxed Document Previewer (UIR-5.2, Screenshot #78, Blueprint §8.2, G-04, G-12, G-13).
 */

import { useMemo, useState, type ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../components/icons";
import { Badge, Button, Input } from "../components/primitives";
import {
  SandboxedDocumentPreviewHost,
  detectDocumentPreviewKind,
} from "../previewers";
import { resolveCodeThemePalette } from "../styles/codeThemeRegistry";
import type { DarkCodeTheme, LightCodeTheme } from "../types/decisions";
import {
  filterVisibleFileNodes,
  sliceVirtualRows,
  type WorkspaceFileNode,
} from "./sidePaneConfig";

export interface FilesTreeAndPreviewTabProps {
  readonly files: readonly WorkspaceFileNode[];
  readonly selectedFilePath: string | null;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly onSelectFile: (filePath: string) => void;
  readonly onOpenExternalFile?: (filePath: string) => void;
  readonly onOpenHtmlInBrowser?: (filePath: string, htmlContent: string) => void;
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

const FILE_ROW_HEIGHT_PX = 28;
const VIEWPORT_HEIGHT_PX = 420;

export function FilesTreeAndPreviewTab({
  files,
  selectedFilePath,
  activeCodeTheme,
  onSelectFile,
  onOpenExternalFile,
  onOpenHtmlInBrowser,
  onSendSelectionToChat,
}: FilesTreeAndPreviewTabProps): ReactElement {
  const [searchQuery, setSearchQuery] = useState("");
  const [scrollTopPx, setScrollTopPx] = useState(0);
  const [sourcePreviewMode, setSourcePreviewMode] = useState(true);
  const [expandedDirs, setExpandedDirs] = useState<ReadonlySet<string>>(() => {
    const initial = new Set<string>();
    for (const f of files) {
      if (f.isDirectory && f.depth <= 1) {
        initial.add(f.path);
      }
    }
    return initial;
  });

  const palette = resolveCodeThemePalette(activeCodeTheme);

  const visibleNodes = useMemo(
    () => filterVisibleFileNodes(files, expandedDirs, searchQuery),
    [files, expandedDirs, searchQuery],
  );

  const virtualSlice = useMemo(
    () =>
      sliceVirtualRows(
        visibleNodes,
        scrollTopPx,
        VIEWPORT_HEIGHT_PX,
        FILE_ROW_HEIGHT_PX,
        8,
      ),
    [visibleNodes, scrollTopPx],
  );

  const selectedNode = useMemo(
    () =>
      files.find((f) => f.path === selectedFilePath && !f.isDirectory) ??
      files.find((f) => !f.isDirectory) ??
      null,
    [files, selectedFilePath],
  );

  const toggleDir = (dirPath: string) => {
    setExpandedDirs((prev) => {
      const next = new Set(prev);
      if (next.has(dirPath)) next.delete(dirPath);
      else next.add(dirPath);
      return next;
    });
  };

  const previewKind = selectedNode
    ? detectDocumentPreviewKind(selectedNode.path)
    : "code";

  const codeLines = (selectedNode?.content ?? "// Empty file").split("\n");

  return (
    <section
      className="ky-files-split"
      data-testid="files-tree-and-preview-tab"
      aria-label="Workspace files tree and code preview"
    >
      {/* Left Column: Virtualized 50k+ Workspace File Tree */}
      <div className="ky-files-split__tree-col">
        <div className="ky-files-split__tree-header">
          <Input
            aria-label="Search files"
            placeholder="Search files..."
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value);
              setScrollTopPx(0);
            }}
          />
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setExpandedDirs(new Set())}
            aria-label="Collapse all folders"
            title="Collapse all folders"
          >
            <ChromeIcon name="minimize" size={14} />
          </Button>
        </div>

        <div
          className="ky-files-split__tree-scroll"
          role="tree"
          aria-label="Workspace files"
          onScroll={(e) => setScrollTopPx(e.currentTarget.scrollTop)}
        >
          <div style={{ height: `${virtualSlice.topSpacerPx}px` }} />
          {virtualSlice.visibleItems.map((node) => {
            const isExpanded = node.isDirectory && expandedDirs.has(node.path);
            const isSelected = !node.isDirectory && node.path === selectedNode?.path;
            return (
              <button
                key={node.path}
                type="button"
                role="treeitem"
                aria-expanded={node.isDirectory ? isExpanded : undefined}
                aria-selected={isSelected}
                className="ky-files-split__row"
                data-selected={isSelected ? "true" : "false"}
                style={{
                  height: `${FILE_ROW_HEIGHT_PX}px`,
                  paddingInlineStart: `${8 + node.depth * 14}px`,
                }}
                onClick={() => {
                  if (node.isDirectory) toggleDir(node.path);
                  else onSelectFile(node.path);
                }}
                title={node.path}
              >
                {node.isDirectory ? (
                  <ChromeIcon
                    name={isExpanded ? "chevron-down" : "chevron-right"}
                    size={14}
                  />
                ) : (
                  <span style={{ width: "14px", flexShrink: 0 }} />
                )}
                <MaterialFileIcon
                  filename={node.path}
                  isFolder={node.isDirectory}
                  isOpen={isExpanded}
                  codeTheme={activeCodeTheme}
                  size={16}
                />
                <span className="ky-files-split__name">{node.name}</span>
                {node.gitStatus && (
                  <span
                    className="ky-files-split__git-badge"
                    data-git-status={node.gitStatus}
                  >
                    {node.gitStatus}
                  </span>
                )}
              </button>
            );
          })}
          <div style={{ height: `${virtualSlice.bottomSpacerPx}px` }} />
        </div>
      </div>

      {/* Right Column: Line-Numbered Code Viewer OR Sandboxed Document Previewer */}
      <div className="ky-files-split__viewer-col">
        {selectedNode ? (
          <>
            <header className="ky-files-split__viewer-header">
              <div className="ky-files-split__viewer-title">
                <MaterialFileIcon
                  filename={selectedNode.path}
                  codeTheme={activeCodeTheme}
                  size={16}
                />
                <span className="ky-files-split__viewer-path" title={selectedNode.path}>
                  {selectedNode.path}
                </span>
                {selectedNode.gitStatus && (
                  <Badge tone="info" label={selectedNode.gitStatus} />
                )}
              </div>

              <div className="ky-files-split__viewer-actions">
                {onOpenHtmlInBrowser &&
                  /\.(html?|svg)$/i.test(selectedNode.path) &&
                  selectedNode.content && (
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() =>
                        onOpenHtmlInBrowser(selectedNode.path, selectedNode.content ?? "")
                      }
                    >
                      <ChromeIcon name="browser-globe" size={14} />
                      <span>Preview in Browser</span>
                    </Button>
                  )}
                {previewKind === "code" && (
                  <Button
                    size="sm"
                    variant={sourcePreviewMode ? "secondary" : "ghost"}
                    onClick={() => setSourcePreviewMode((v) => !v)}
                  >
                    {sourcePreviewMode ? "Source preview" : "Raw text"}
                  </Button>
                )}
                {onOpenExternalFile && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => onOpenExternalFile(selectedNode.path)}
                  >
                    <ChromeIcon name="external-link" size={14} />
                    <span>Open</span>
                  </Button>
                )}
              </div>
            </header>

            {previewKind !== "code" ? (
              <SandboxedDocumentPreviewHost
                filePath={selectedNode.path}
                onSendSelectionToChat={onSendSelectionToChat}
              />
            ) : (
              <div
                className="ky-code-viewer"
                data-code-theme={activeCodeTheme}
                style={{
                  backgroundColor: palette.background,
                  color: palette.foreground,
                }}
              >
                {codeLines.map((line, index) => (
                  <div key={`${selectedNode.path}-L${index + 1}`} className="ky-code-viewer__line">
                    {sourcePreviewMode && (
                      <span
                        className="ky-code-viewer__gutter"
                        style={{ color: palette.gutterForeground }}
                      >
                        {index + 1}
                      </span>
                    )}
                    <code className="ky-code-viewer__text">{line || " "}</code>
                  </div>
                ))}
              </div>
            )}
          </>
        ) : (
          <div className="ky-files-split__empty">
            Select a file from the workspace tree to inspect source code or sandboxed previews.
          </div>
        )}
      </div>
    </section>
  );
}
