/**
 * Multi-Tab Side Pane Configuration, 50k+ Virtualized File Tree & Full-View Helpers
 * (UIR-5.1..UIR-5.3, Screenshots #71–#74, #78, #82–#83, Blueprint §8, Quality Gate G-13).
 *
 * Pure TypeScript module (import-type only) so both Vite and Node 26 native tests import it directly.
 */

export type SidePaneToolKind =
  | "new-tab"
  | "files"
  | "browser"
  | "browser-permissions"
  | "terminal"
  | "review"
  | "sites"
  | "side-conversation"
  | "subagents"
  | "context-inspector"
  | "document-preview"
  | "subagent-report"
  | "treemapping"
  | "whiteboard"
  | "model-trajectory"
  | "developer-tools";

export type AppPluginLauncherId =
  | "plugin-documents"
  | "plugin-pdf"
  | "plugin-presentations"
  | "plugin-spreadsheets";

export interface NewTabLauncherItem {
  readonly id: string;
  readonly kind: SidePaneToolKind;
  readonly category: "tool" | "advanced-tool" | "app-plugin";
  readonly label: string;
  readonly description: string;
  readonly iconName: string;
  readonly sampleDocumentPath?: string;
}

export interface SidePaneTab {
  readonly tabId: string;
  readonly kind: SidePaneToolKind;
  readonly title: string;
  readonly filePath?: string;
  readonly subagentId?: string;
  readonly browserUrl?: string;
  readonly browserOrigin?: string;
  readonly htmlSourceContent?: string;
  readonly pinned?: boolean;
  readonly closedAt?: number;
}

export type GitFileStatus = "M" | "U" | "A" | "D" | null;

export interface WorkspaceFileNode {
  readonly path: string;
  readonly name: string;
  readonly isDirectory: boolean;
  readonly depth: number;
  readonly parentPath: string | null;
  readonly gitStatus?: GitFileStatus;
  readonly content?: string;
}

export interface VirtualRowSlice<T> {
  readonly startIndex: number;
  readonly endIndex: number;
  readonly topSpacerPx: number;
  readonly bottomSpacerPx: number;
  readonly visibleItems: readonly T[];
}

export interface SubagentRunRecord {
  readonly subagentId: string;
  readonly name: string;
  readonly role: string;
  readonly status: "running" | "ended";
  readonly elapsedLabel: string;
  readonly toolCallsCount: number;
  readonly summaryMarkdown: string;
  readonly trajectorySteps: readonly {
    readonly stepId: string;
    readonly action: string;
    readonly detail: string;
    readonly status: "success" | "running" | "error";
  }[];
}

export interface ReviewDiffFileRecord {
  readonly filePath: string;
  readonly gitStatus: Exclude<GitFileStatus, null>;
  readonly oldText: string;
  readonly newText: string;
}

export interface SideConversationMessage {
  readonly messageId: string;
  readonly role: "user" | "assistant";
  readonly text: string;
  readonly timestamp: string;
}

/**
 * Authoritative `New tab` Launcher Tools (Screenshots #74, #82, Blueprint §8.1).
 */
export const NEW_TAB_TOOLS: readonly NewTabLauncherItem[] = [
  {
    id: "tool-files",
    kind: "files",
    category: "tool",
    label: "Files",
    description: "Virtualized workspace tree & syntax-highlighted code preview",
    iconName: "projects",
  },
  {
    id: "tool-browser",
    kind: "browser",
    category: "tool",
    label: "Browser",
    description: "Governed sandboxed browser preview & generative UI host",
    iconName: "browser-globe",
  },
  {
    id: "tool-terminal",
    kind: "terminal",
    category: "tool",
    label: "Terminal",
    description: "Embedded multi-tab PTY terminal inside the Side Pane",
    iconName: "terminal",
  },
  {
    id: "tool-review",
    kind: "review",
    category: "tool",
    label: "Review",
    description: "Task diff summary with off-main-thread Unified & Split word diffs",
    iconName: "git-branch",
  },
  {
    id: "tool-sites",
    kind: "sites",
    category: "tool",
    label: "Sites",
    description: "Local dev servers & sandboxed HTML artifact preview",
    iconName: "external-link",
  },
  {
    id: "tool-side-conversation",
    kind: "side-conversation",
    category: "tool",
    label: "Side conversation",
    description: "Isolated workspace thread without polluting primary task history",
    iconName: "sparkles",
  },
  {
    id: "tool-subagents",
    kind: "subagents",
    category: "tool",
    label: "Subagents",
    description: "Inspect Running & Ended subagents and full trajectory reports",
    iconName: "subagent",
  },
  {
    id: "tool-context-inspector",
    kind: "context-inspector",
    category: "tool",
    label: "Context Inspector",
    description: "Inspect exact assembled ContextSnapshot tokens & omissions",
    iconName: "cpu",
  },
];

/**
 * Authoritative ZCode v3.18.2 Advanced Workspace Tools (`VQn` side-pane tabs).
 */
export const NEW_TAB_ADVANCED_TOOLS: readonly NewTabLauncherItem[] = [
  {
    id: "tool-treemapping",
    kind: "treemapping",
    category: "advanced-tool",
    label: "Treemapping",
    description: "Visual map of files read, written, modified, or deleted this turn",
    iconName: "layout-dashboard",
  },
  {
    id: "tool-whiteboard",
    kind: "whiteboard",
    category: "advanced-tool",
    label: "Whiteboard",
    description: "Interactive drawing canvas with Pen, Eraser, Undo/Redo & Add to chat",
    iconName: "pen-tool",
  },
  {
    id: "tool-model-trajectory",
    kind: "model-trajectory",
    category: "advanced-tool",
    label: "Model Trajectory",
    description: "Inspect raw model prompts, reasoning blocks, tool calls, and results",
    iconName: "activity",
  },
  {
    id: "tool-developer-tools",
    kind: "developer-tools",
    category: "advanced-tool",
    label: "Developer Tools",
    description: "Inspect live local requests, SSE streams, and runtime telemetry",
    iconName: "wrench",
  },
];

/**
 * Authoritative `App plugins` Launcher Group (Screenshots #74, #82, UI-39, Blueprint §22.3).
 */
export const NEW_TAB_APP_PLUGINS: readonly NewTabLauncherItem[] = [
  {
    id: "plugin-documents",
    kind: "document-preview",
    category: "app-plugin",
    label: "Documents",
    description: "Sandboxed DOCX viewer (zero scripts, zero macros)",
    iconName: "file-text",
    sampleDocumentPath: "docs/architecture/Karyora-Platform-Spec.docx",
  },
  {
    id: "plugin-pdf",
    kind: "document-preview",
    category: "app-plugin",
    label: "PDF",
    description: "Sandboxed PDF page viewer with zoom & selection-to-chat",
    iconName: "docs",
    sampleDocumentPath: "docs/release/Karyora-Security-Brief.pdf",
  },
  {
    id: "plugin-presentations",
    kind: "document-preview",
    category: "app-plugin",
    label: "Presentations",
    description: "Sandboxed PPTX slide deck viewer with speaker notes",
    iconName: "tasks",
    sampleDocumentPath: "docs/decks/Karyora-v3-Architecture.pptx",
  },
  {
    id: "plugin-spreadsheets",
    kind: "document-preview",
    category: "app-plugin",
    label: "Spreadsheets",
    description: "Sandboxed XLSX multi-sheet grid (formula/macro inert)",
    iconName: "usage-chart",
    sampleDocumentPath: "docs/benchmarks/Karyora-Latency-Matrix.xlsx",
  },
];

/**
 * Filters a flat pre-order workspace file list by search query and collapsed folders.
 * Designed for O(N) single-pass traversal over 50,000+ workspace entries (`G-13`).
 */
export function filterVisibleFileNodes(
  allNodes: readonly WorkspaceFileNode[],
  expandedDirs: ReadonlySet<string>,
  searchQuery: string,
): readonly WorkspaceFileNode[] {
  const q = searchQuery.trim().toLowerCase();
  if (q.length > 0) {
    const matched: WorkspaceFileNode[] = [];
    for (let i = 0; i < allNodes.length; i += 1) {
      const node = allNodes[i];
      if (
        node.name.toLowerCase().includes(q) ||
        node.path.toLowerCase().includes(q)
      ) {
        matched.push(node);
      }
    }
    return matched;
  }

  const visible: WorkspaceFileNode[] = [];
  let hiddenPrefix: string | null = null;

  for (let i = 0; i < allNodes.length; i += 1) {
    const node = allNodes[i];
    if (hiddenPrefix !== null) {
      if (node.path.startsWith(hiddenPrefix)) {
        continue;
      }
      hiddenPrefix = null;
    }
    visible.push(node);
    if (node.isDirectory && !expandedDirs.has(node.path)) {
      hiddenPrefix = `${node.path}/`;
    }
  }
  return visible;
}

/**
 * O(1) virtual viewport row slicer for 50,000+ file trees & spreadsheet rows (`G-13`).
 */
export function sliceVirtualRows<T>(
  items: readonly T[],
  scrollTopPx: number,
  viewportHeightPx: number,
  rowHeightPx: number,
  overscanCount = 8,
): VirtualRowSlice<T> {
  const total = items.length;
  if (total === 0 || rowHeightPx <= 0) {
    return {
      startIndex: 0,
      endIndex: 0,
      topSpacerPx: 0,
      bottomSpacerPx: 0,
      visibleItems: [],
    };
  }
  const safeScroll = Math.max(0, scrollTopPx);
  const rawStart = Math.floor(safeScroll / rowHeightPx);
  const visibleCount = Math.ceil(Math.max(rowHeightPx, viewportHeightPx) / rowHeightPx);
  const startIndex = Math.max(0, rawStart - overscanCount);
  const endIndex = Math.min(total, rawStart + visibleCount + overscanCount);
  return {
    startIndex,
    endIndex,
    topSpacerPx: startIndex * rowHeightPx,
    bottomSpacerPx: Math.max(0, (total - endIndex) * rowHeightPx),
    visibleItems: items.slice(startIndex, endIndex),
  };
}

/**
 * Opens a Side-Pane tab or focuses an existing tab with the same `tabId`.
 */
export function openOrFocusSideTab(
  tabs: readonly SidePaneTab[],
  nextTab: SidePaneTab,
): { readonly tabs: readonly SidePaneTab[]; readonly activeTabId: string } {
  const existing = tabs.find((t) => t.tabId === nextTab.tabId);
  if (existing) {
    return { tabs, activeTabId: existing.tabId };
  }
  return {
    tabs: [...tabs, nextTab],
    activeTabId: nextTab.tabId,
  };
}

/**
 * Closes a Side-Pane tab and deterministically selects the nearest neighbor tab.
 */
export function closeSideTab(
  tabs: readonly SidePaneTab[],
  activeTabId: string | null,
  tabIdToClose: string,
): { readonly tabs: readonly SidePaneTab[]; readonly activeTabId: string | null } {
  const index = tabs.findIndex((t) => t.tabId === tabIdToClose);
  if (index === -1) {
    return { tabs, activeTabId };
  }
  const nextTabs = tabs.filter((t) => t.tabId !== tabIdToClose);
  if (nextTabs.length === 0) {
    return { tabs: nextTabs, activeTabId: null };
  }
  if (activeTabId !== tabIdToClose) {
    return { tabs: nextTabs, activeTabId };
  }
  const fallbackIndex = Math.min(index, nextTabs.length - 1);
  return {
    tabs: nextTabs,
    activeTabId: nextTabs[fallbackIndex]?.tabId ?? null,
  };
}

/**
 * Formats elapsed working seconds for the Full-View floating minimized working bar
 * (Screenshot #83 — e.g. `Working for 26m 37s`).
 */
export function formatWorkingDuration(elapsedSeconds: number): string {
  const safe = Math.max(0, Math.floor(elapsedSeconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const seconds = safe % 60;
  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}
