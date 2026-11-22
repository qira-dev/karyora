/**
 * Multi-Tab Side Pane Container (UIR-5.1..UIR-5.5, 1:1 ZCode v3.18.2 `VQn` & `KQn` parity).
 *
 * Features:
 * - `Search tabs` + `Recently closed tabs` reopen popover (`KQn`)
 * - Dynamic scrollable tab bar (`Files`, `Review`, `Subagents`, `Browser`, `Browser Permissions`,
 *   `Terminal`, `Sites`, `Side conversation`, `Context`, `Treemapping`, `Whiteboard`,
 *   `Model Trajectory`, `Developer Tools`, active document/subagent report tabs)
 * - `+` `New tab` launcher button (`#74`, `#82`)
 * - Full-View Mode toggle (`⤢` `Ctrl+Shift+F`) paired with `FloatingMinimizedWorkingBar` (`#83`)
 * - Close (`×`) button & resizable drag edge
 */

import { Suspense, lazy, useState, type ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../components/icons";
import { Button } from "../components/primitives";
import {
  ContextInspectorView,
  INITIAL_CONTEXT_SNAPSHOTS,
} from "../domains/context";
import { SandboxedDocumentPreviewHost } from "../previewers";
import { DockedTerminalPanel } from "../terminal";
import type { DarkCodeTheme, LightCodeTheme, TargetPlatform } from "../types/decisions";
import type { ConversationTurnRecord } from "../workspace/workspaceConfig";
import { BrowserAndSitesTab } from "./BrowserAndSitesTab";
import type {
  SitePermissionDecision,
  SitePermissionsStoreMap,
} from "./browserConfig";
import { FilesTreeAndPreviewTab } from "./FilesTreeAndPreviewTab";
import { NewTabLauncher } from "./NewTabLauncher";
import { ReviewDiffTab } from "./ReviewDiffTab";
import { SideConversationTab } from "./SideConversationTab";
import { SubagentsTab } from "./SubagentsTab";
import type {
  NewTabLauncherItem,
  ReviewDiffFileRecord,
  SidePaneTab,
  SubagentRunRecord,
  WorkspaceFileNode,
} from "./sidePaneConfig";

const LazyBrowserPermissionsTab = lazy(() => import("./BrowserPermissionsTab"));
const LazyTreemappingTab = lazy(() => import("./TreemappingTab"));
const LazyWhiteboardTab = lazy(() => import("./WhiteboardTab"));
const LazyModelTrajectoryTab = lazy(() => import("./ModelTrajectoryTab"));
const LazyDeveloperToolsTab = lazy(() => import("./DeveloperToolsTab"));

export interface MultiTabSidePaneProps {
  readonly tabs: readonly SidePaneTab[];
  readonly recentlyClosedTabs?: readonly SidePaneTab[];
  readonly activeTabId: string | null;
  readonly isFullView: boolean;
  readonly runtimeTarget: TargetPlatform;
  readonly remoteShellEnabledByCore: boolean;
  readonly browserUseEnabled: boolean;
  readonly sitePermissionsStore?: SitePermissionsStoreMap;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly workspaceName: string;
  readonly activeTaskTitle?: string;
  readonly conversationTurns?: readonly ConversationTurnRecord[];
  readonly workspaceFiles: readonly WorkspaceFileNode[];
  readonly selectedFilePath: string | null;
  readonly reviewDiffFiles: readonly ReviewDiffFileRecord[];
  readonly subagents: readonly SubagentRunRecord[];
  readonly onSelectTab: (tabId: string) => void;
  readonly onCloseTab: (tabId: string) => void;
  readonly onReopenClosedTab?: (tab: SidePaneTab) => void;
  readonly onOpenNewTabLauncher: () => void;
  readonly onLaunchToolOrPlugin: (item: NewTabLauncherItem) => void;
  readonly onOpenMarketplace?: () => void;
  readonly onSelectFile: (filePath: string) => void;
  readonly onOpenHtmlInBrowser?: (filePath: string, htmlContent: string) => void;
  readonly onOpenSitePermissions?: (origin: string) => void;
  readonly onSetSitePermission?: (
    origin: string,
    capabilityKey: string,
    decision: SitePermissionDecision,
  ) => void;
  readonly onResetSitePermissions?: (origin: string) => void;
  readonly onInspectSubagent: (subagent: SubagentRunRecord) => void;
  readonly onToggleFullView: () => void;
  readonly onClosePane: () => void;
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
  readonly onOpenBrowserUseSettings: () => void;
  readonly onOpenExternalUrl?: (normalizedUrl: string, domain: string) => void;
  readonly onOpenContextInspectorPage?: () => void;
  readonly onNotify?: (title: string, detail?: string) => void;
}

function resolveTabIcon(kind: SidePaneTab["kind"]) {
  switch (kind) {
    case "files":
      return "projects";
    case "review":
      return "git-branch";
    case "subagents":
    case "subagent-report":
      return "subagent";
    case "terminal":
      return "terminal";
    case "browser":
    case "sites":
      return "browser-globe";
    case "browser-permissions":
      return "shield-security";
    case "side-conversation":
      return "sparkles";
    case "context-inspector":
      return "cpu";
    case "treemapping":
      return "layout-dashboard";
    case "whiteboard":
      return "pen-tool";
    case "model-trajectory":
      return "activity";
    case "developer-tools":
      return "wrench";
    default:
      return "plus";
  }
}

export function MultiTabSidePane({
  tabs,
  recentlyClosedTabs = [],
  activeTabId,
  isFullView,
  runtimeTarget,
  remoteShellEnabledByCore,
  browserUseEnabled,
  sitePermissionsStore = {},
  activeCodeTheme,
  workspaceName,
  activeTaskTitle = "Workspace session",
  conversationTurns = [],
  workspaceFiles,
  selectedFilePath,
  reviewDiffFiles,
  subagents,
  onSelectTab,
  onCloseTab,
  onReopenClosedTab,
  onOpenNewTabLauncher,
  onLaunchToolOrPlugin,
  onOpenMarketplace,
  onSelectFile,
  onOpenHtmlInBrowser,
  onOpenSitePermissions,
  onSetSitePermission,
  onResetSitePermissions,
  onInspectSubagent,
  onToggleFullView,
  onClosePane,
  onSendSelectionToChat,
  onOpenBrowserUseSettings,
  onOpenExternalUrl,
  onOpenContextInspectorPage,
  onNotify,
}: MultiTabSidePaneProps): ReactElement {
  const [tabSearchOpen, setTabSearchOpen] = useState(false);
  const [tabSearchQuery, setTabSearchQuery] = useState("");
  const activeTab = tabs.find((t) => t.tabId === activeTabId) ?? tabs[0] ?? null;

  const q = tabSearchQuery.trim().toLowerCase();
  const filteredOpenTabs = tabs.filter(
    (t) => !q || t.title.toLowerCase().includes(q) || t.kind.toLowerCase().includes(q),
  );
  const filteredClosedTabs = recentlyClosedTabs.filter(
    (t) => !q || t.title.toLowerCase().includes(q) || t.kind.toLowerCase().includes(q),
  );

  const lazyFallback = (
    <div className="ky-files-split__empty" role="status">
      Loading workspace tool...
    </div>
  );

  return (
    <div
      className="ky-side-pane"
      data-testid="multi-tab-side-pane"
      data-full-view={isFullView ? "true" : "false"}
    >
      <header className="ky-side-pane__header">
        {/* ZCode `KQn` Search Tabs + Recently Closed Popover Trigger */}
        <div style={{ position: "relative", flexShrink: 0 }}>
          <Button
            size="sm"
            variant={tabSearchOpen ? "secondary" : "ghost"}
            aria-label="Search tabs"
            title="Search open & recently closed tabs"
            data-testid="side-pane-search-tabs-btn"
            onClick={() => setTabSearchOpen((prev) => !prev)}
          >
            <ChromeIcon name="search" size={14} />
          </Button>

          {tabSearchOpen && (
            <div
              role="dialog"
              aria-label="Search side pane tabs"
              data-testid="side-pane-search-tabs-popover"
              className="ky-popover-surface ky-tab-search-popover"
            >
              <input
                type="text"
                aria-label="Filter tabs"
                placeholder="Search tabs..."
                className="ky-input"
                style={{ minHeight: "30px", fontSize: "0.8rem" }}
                value={tabSearchQuery}
                onChange={(e) => setTabSearchQuery(e.target.value)}
              />

              <div className="ky-tab-search-popover__section">Open tabs ({filteredOpenTabs.length})</div>
              <div className="ky-tab-search-popover__list">
                {filteredOpenTabs.map((t) => (
                  <button
                    key={t.tabId}
                    type="button"
                    className="ky-tab-search-popover__item"
                    data-active={t.tabId === activeTab?.tabId ? "true" : "false"}
                    onClick={() => {
                      onSelectTab(t.tabId);
                      setTabSearchOpen(false);
                    }}
                  >
                    <ChromeIcon name={resolveTabIcon(t.kind)} size={14} />
                    <span className="ky-tab-search-popover__label">{t.title}</span>
                  </button>
                ))}
              </div>

              {filteredClosedTabs.length > 0 && onReopenClosedTab && (
                <>
                  <div className="ky-tab-search-popover__section">
                    Recently closed ({filteredClosedTabs.length})
                  </div>
                  <div className="ky-tab-search-popover__list">
                    {filteredClosedTabs.map((t) => (
                      <button
                        key={`closed-${t.tabId}-${t.closedAt ?? 0}`}
                        type="button"
                        className="ky-tab-search-popover__item"
                        onClick={() => {
                          onReopenClosedTab(t);
                          setTabSearchOpen(false);
                        }}
                      >
                        <ChromeIcon name="history" size={14} />
                        <span className="ky-tab-search-popover__label">{t.title}</span>
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}
        </div>

        <div className="ky-side-pane__tab-strip" role="tablist" aria-label="Side pane tabs">
          {tabs.map((tab) => {
            const isSelected = tab.tabId === activeTab?.tabId;
            return (
              <div
                key={tab.tabId}
                className="ky-side-pane__tab"
                data-active={isSelected ? "true" : "false"}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={isSelected}
                  className="ky-side-pane__tab-btn"
                  data-testid={`side-pane-tab-${tab.tabId}`}
                  onClick={() => onSelectTab(tab.tabId)}
                >
                  {tab.filePath && tab.kind !== "browser" ? (
                    <MaterialFileIcon
                      filename={tab.filePath}
                      codeTheme={activeCodeTheme}
                      size={16}
                    />
                  ) : (
                    <ChromeIcon name={resolveTabIcon(tab.kind)} size={14} />
                  )}
                  <span className="ky-side-pane__tab-title">{tab.title}</span>
                </button>
                <button
                  type="button"
                  className="ky-side-pane__tab-close"
                  aria-label={`Close tab ${tab.title}`}
                  onClick={() => onCloseTab(tab.tabId)}
                >
                  <ChromeIcon name="close" size={14} />
                </button>
              </div>
            );
          })}

          <Button
            size="sm"
            variant="ghost"
            onClick={onOpenNewTabLauncher}
            aria-label="Open new side pane tab"
            title="New tab"
            data-testid="side-pane-new-tab-btn"
          >
            <ChromeIcon name="plus" size={14} />
          </Button>
        </div>

        <div className="ky-side-pane__chrome-actions">
          <Button
            size="sm"
            variant={isFullView ? "secondary" : "ghost"}
            onClick={onToggleFullView}
            aria-label={isFullView ? "Exit Full-View Mode" : "Enter Full-View Mode"}
            title={isFullView ? "Exit Full-View (Ctrl+Shift+F)" : "Full-View Mode (Ctrl+Shift+F)"}
            data-testid="side-pane-full-view-toggle"
          >
            <ChromeIcon name={isFullView ? "minimize" : "maximize"} size={14} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={onClosePane}
            aria-label="Close Side Pane"
            title="Close Side Pane (Ctrl+Alt+B)"
          >
            <ChromeIcon name="close" size={14} />
          </Button>
        </div>
      </header>

      <div className="ky-side-pane__body">
        {!activeTab || activeTab.kind === "new-tab" ? (
          <NewTabLauncher
            onLaunchItem={onLaunchToolOrPlugin}
            onOpenMarketplace={onOpenMarketplace}
          />
        ) : activeTab.kind === "files" ? (
          <FilesTreeAndPreviewTab
            files={workspaceFiles}
            selectedFilePath={activeTab.filePath ?? selectedFilePath}
            activeCodeTheme={activeCodeTheme}
            onSelectFile={onSelectFile}
            onOpenHtmlInBrowser={onOpenHtmlInBrowser}
            onSendSelectionToChat={onSendSelectionToChat}
          />
        ) : activeTab.kind === "review" ? (
          <ReviewDiffTab
            files={reviewDiffFiles}
            initialSelectedPath={activeTab.filePath ?? null}
            activeCodeTheme={activeCodeTheme}
            onSendHunkToChat={(fp, summary) => onSendSelectionToChat(summary, fp)}
          />
        ) : activeTab.kind === "subagents" ? (
          <SubagentsTab
            subagents={subagents}
            onInspectSubagent={onInspectSubagent}
          />
        ) : activeTab.kind === "subagent-report" ? (
          <SubagentsTab
            subagents={subagents}
            inspectingSubagentId={activeTab.subagentId ?? null}
            onInspectSubagent={onInspectSubagent}
            onBackToDirectory={() =>
              onLaunchToolOrPlugin({
                id: "tool-subagents",
                kind: "subagents",
                category: "tool",
                label: "Subagents",
                description: "",
                iconName: "subagent",
              })
            }
          />
        ) : activeTab.kind === "browser" || activeTab.kind === "sites" ? (
          <BrowserAndSitesTab
            mode={activeTab.kind}
            browserUseEnabled={browserUseEnabled}
            initialUrl={activeTab.browserUrl ?? null}
            htmlFilePath={activeTab.filePath ?? null}
            htmlSourceContent={activeTab.htmlSourceContent ?? null}
            onOpenBrowserUseSettings={onOpenBrowserUseSettings}
            onOpenSitePermissions={onOpenSitePermissions}
            onRequestExternalOpen={onOpenExternalUrl}
            onSendSelectionToChat={onSendSelectionToChat}
            onNotify={onNotify}
          />
        ) : activeTab.kind === "browser-permissions" ? (
          <Suspense fallback={lazyFallback}>
            <LazyBrowserPermissionsTab
              origin={activeTab.browserOrigin ?? "http://127.0.0.1:5173"}
              permissions={
                sitePermissionsStore[activeTab.browserOrigin ?? "http://127.0.0.1:5173"] ?? {}
              }
              onSetPermission={(origin, key, dec) =>
                onSetSitePermission?.(origin, key, dec)
              }
              onResetPermissions={(origin) => onResetSitePermissions?.(origin)}
            />
          </Suspense>
        ) : activeTab.kind === "treemapping" ? (
          <Suspense fallback={lazyFallback}>
            <LazyTreemappingTab
              files={workspaceFiles}
              diffFiles={reviewDiffFiles}
              activeCodeTheme={activeCodeTheme}
              onOpenFile={(fp) => {
                onSelectFile(fp);
                onLaunchToolOrPlugin({
                  id: "tool-files",
                  kind: "files",
                  category: "tool",
                  label: "Files",
                  description: "",
                  iconName: "projects",
                });
              }}
            />
          </Suspense>
        ) : activeTab.kind === "whiteboard" ? (
          <Suspense fallback={lazyFallback}>
            <LazyWhiteboardTab onAddToChat={onSendSelectionToChat} />
          </Suspense>
        ) : activeTab.kind === "model-trajectory" ? (
          <Suspense fallback={lazyFallback}>
            <LazyModelTrajectoryTab
              taskTitle={activeTaskTitle}
              turns={conversationTurns}
            />
          </Suspense>
        ) : activeTab.kind === "developer-tools" ? (
          <Suspense fallback={lazyFallback}>
            <LazyDeveloperToolsTab />
          </Suspense>
        ) : activeTab.kind === "side-conversation" ? (
          <SideConversationTab
            workspaceName={workspaceName}
            onInsertToMainComposer={(text) =>
              onSendSelectionToChat(text, "side-conversation")
            }
          />
        ) : activeTab.kind === "terminal" ? (
          <DockedTerminalPanel
            runtimeTarget={runtimeTarget}
            remoteShellEnabledByCore={remoteShellEnabledByCore}
            activeCodeTheme={activeCodeTheme}
            placement="side-pane"
          />
        ) : activeTab.kind === "document-preview" ? (
          <SandboxedDocumentPreviewHost
            filePath={activeTab.filePath ?? "docs/release/Karyora-Security-Brief.pdf"}
            onSendSelectionToChat={onSendSelectionToChat}
          />
        ) : (
          <section className="ky-context-side-summary" data-testid="side-context-inspector-tab">
            <ContextInspectorView snapshots={INITIAL_CONTEXT_SNAPSHOTS} compact={true} />
            {onOpenContextInspectorPage && (
              <div>
                <Button size="sm" variant="secondary" onClick={onOpenContextInspectorPage}>
                  Open full Context Inspector surface
                </Button>
              </div>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
