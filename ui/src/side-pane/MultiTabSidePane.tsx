/**
 * Multi-Tab Side Pane Container (UIR-5.1..UIR-5.5, Screenshots #71–#74, #78, #81–#83, Blueprint §8).
 *
 * Features:
 * - Dynamic scrollable tab bar (`Files`, `Review`, `Subagents`, `Browser`, `Terminal`,
 *   `Sites`, `Side conversation`, `Context`, active document/subagent report tabs)
 * - `+` `New tab` launcher button (`#74`, `#82`)
 * - Full-View Mode toggle (`⤢` `Ctrl+Shift+F`) paired with `FloatingMinimizedWorkingBar` (`#83`)
 * - Close (`×`) button & resizable drag edge
 */

import type { ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../components/icons";
import { Button } from "../components/primitives";
import {
  ContextInspectorView,
  INITIAL_CONTEXT_SNAPSHOTS,
} from "../domains/context";
import { SandboxedDocumentPreviewHost } from "../previewers";
import { DockedTerminalPanel } from "../terminal";
import type { DarkCodeTheme, LightCodeTheme, TargetPlatform } from "../types/decisions";
import { BrowserAndSitesTab } from "./BrowserAndSitesTab";
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

export interface MultiTabSidePaneProps {
  readonly tabs: readonly SidePaneTab[];
  readonly activeTabId: string | null;
  readonly isFullView: boolean;
  readonly runtimeTarget: TargetPlatform;
  readonly remoteShellEnabledByCore: boolean;
  readonly browserUseEnabled: boolean;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly workspaceName: string;
  readonly workspaceFiles: readonly WorkspaceFileNode[];
  readonly selectedFilePath: string | null;
  readonly reviewDiffFiles: readonly ReviewDiffFileRecord[];
  readonly subagents: readonly SubagentRunRecord[];
  readonly onSelectTab: (tabId: string) => void;
  readonly onCloseTab: (tabId: string) => void;
  readonly onOpenNewTabLauncher: () => void;
  readonly onLaunchToolOrPlugin: (item: NewTabLauncherItem) => void;
  readonly onSelectFile: (filePath: string) => void;
  readonly onInspectSubagent: (subagent: SubagentRunRecord) => void;
  readonly onToggleFullView: () => void;
  readonly onClosePane: () => void;
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
  readonly onOpenBrowserUseSettings: () => void;
  readonly onOpenExternalUrl?: (normalizedUrl: string, domain: string) => void;
  readonly onOpenContextInspectorPage?: () => void;
}

export function MultiTabSidePane({
  tabs,
  activeTabId,
  isFullView,
  runtimeTarget,
  remoteShellEnabledByCore,
  browserUseEnabled,
  activeCodeTheme,
  workspaceName,
  workspaceFiles,
  selectedFilePath,
  reviewDiffFiles,
  subagents,
  onSelectTab,
  onCloseTab,
  onOpenNewTabLauncher,
  onLaunchToolOrPlugin,
  onSelectFile,
  onInspectSubagent,
  onToggleFullView,
  onClosePane,
  onSendSelectionToChat,
  onOpenBrowserUseSettings,
  onOpenExternalUrl,
  onOpenContextInspectorPage,
}: MultiTabSidePaneProps): ReactElement {
  const activeTab = tabs.find((t) => t.tabId === activeTabId) ?? tabs[0] ?? null;

  return (
    <div
      className="ky-side-pane"
      data-testid="multi-tab-side-pane"
      data-full-view={isFullView ? "true" : "false"}
    >
      <header className="ky-side-pane__header">
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
                  {tab.filePath ? (
                    <MaterialFileIcon
                      filename={tab.filePath}
                      codeTheme={activeCodeTheme}
                      size={16}
                    />
                  ) : (
                    <ChromeIcon
                      name={
                        tab.kind === "files"
                          ? "projects"
                          : tab.kind === "review"
                            ? "git-branch"
                            : tab.kind === "subagents" || tab.kind === "subagent-report"
                              ? "subagent"
                              : tab.kind === "terminal"
                                ? "terminal"
                                : tab.kind === "browser" || tab.kind === "sites"
                                  ? "browser-globe"
                                  : tab.kind === "side-conversation"
                                    ? "sparkles"
                                    : tab.kind === "context-inspector"
                                      ? "cpu"
                                      : "plus"
                      }
                      size={14}
                    />
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
          <NewTabLauncher onLaunchItem={onLaunchToolOrPlugin} />
        ) : activeTab.kind === "files" ? (
          <FilesTreeAndPreviewTab
            files={workspaceFiles}
            selectedFilePath={activeTab.filePath ?? selectedFilePath}
            activeCodeTheme={activeCodeTheme}
            onSelectFile={onSelectFile}
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
            onOpenBrowserUseSettings={onOpenBrowserUseSettings}
            onRequestExternalOpen={onOpenExternalUrl}
          />
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
