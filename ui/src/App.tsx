/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry
 * (Phase 1 UIR-0/UIR-1, Phase 2 UIR-2/UIR-3 & Phase 3 UIR-4/UIR-5 Complete).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */

import { Suspense, lazy, useEffect, useMemo, useState, type ReactElement } from "react";
import i18next from "i18next";
import { activateLocale } from "./i18n";
import {
  AutomationsAndWorkflowsPage,
  INITIAL_SCHEDULED_TASKS,
  INITIAL_WORKFLOWS,
  type ScheduledTaskRecord,
  type WorkflowDefinitionRecord,
} from "./automations";
import type { ToastMessage } from "./components/primitives";
import {
  createIdempotencyKey,
  createV3CoreClient,
  type TaskSessionSummary,
  type WorkspaceProjectSummary,
} from "./coreclient";
import {
  ActivityAuditPage,
  INITIAL_AUDIT_EVENTS,
  createAppendOnlyCorrectionEvent,
  type AuditEventRecord,
} from "./domains/activity";
import {
  ContextInspectorView,
  INITIAL_CONTEXT_SNAPSHOTS,
} from "./domains/context";
import { JobsPage } from "./domains/jobs";
import {
  INITIAL_KNOWLEDGE_PACKS,
  KnowledgePackPage,
  type KnowledgePackRecord,
} from "./domains/knowledge";
import {
  INITIAL_MEMORY_ITEMS,
  ScopedMemorySurface,
  forgetMemoryItem,
  permanentlyDeleteMemoryItem,
  restoreMemoryItem,
  type ScopedMemoryItem,
} from "./domains/memory";
import {
  INITIAL_MARKETPLACE_PLUGINS,
  PluginMarketplacePage,
  type MarketplacePluginRecord,
} from "./marketplace";
import { createPlatformBridge } from "./platform-bridge";
import {
  ApplicationShell,
  StartupHandshakeGate,
  deduplicateNotifications,
  type ActiveMainSurface,
  type CoreNotificationItem,
} from "./shell";
import {
  FloatingMinimizedWorkingBar,
  INITIAL_OVERVIEW_ARTIFACTS,
  INITIAL_OVERVIEW_UPLOADS,
  MultiTabSidePane,
  WorkspaceOverviewRail,
  closeSideTab,
  createDefaultOverviewVisibility,
  openOrFocusSideTab,
  parseOverviewVisibilityJson,
  resetOriginPermissions,
  setOriginPermission,
  toggleOverviewSectionVisibility,
  type NewTabLauncherItem,
  type OverviewSectionId,
  type OverviewSectionVisibilityMap,
  type ReviewDiffFileRecord,
  type SidePaneTab,
  type SitePermissionsStoreMap,
  type SubagentRunRecord,
  type WorkspaceFileNode,
} from "./side-pane";
import { DockedTerminalPanel } from "./terminal";
import type {
  AppTheme,
  InterfaceMode,
  SettingsSectionId,
  SupportedLocale,
} from "./types/decisions";

const LazySettingsOverlayHub = lazy(() => import("./settings/SettingsOverlayHub"));
import {
  AgenticComposer,
  ConversationTimeline,
  GreetingEmptyState,
  type ComposerSessionMode,
  type ComposerThoughtLevel,
  type ConversationTurnRecord,
  type DurableJobRecord,
  type TimelineProgressStep,
  type TimelineSubagentBadge,
  type TriggerSuggestionItem,
} from "./workspace";
import {
  DEFAULT_LOCAL_UI_PREFERENCES,
  EphemeralUiStore,
  LocalPreferencesStore,
  SubmissionQueueStore,
  type ComposerContextChip,
} from "./workspace/state";

const INITIAL_PROJECTS: readonly WorkspaceProjectSummary[] = [
  {
    projectId: "proj-karyora",
    name: "karyora_Platform",
    path: "/home/km-abdullah/karyora_Platform",
    deviceName: "This computer",
    isRemoteDevice: false,
    gitBranch: "main",
    sessionCount: 2,
    lastActiveAt: "2m ago",
    isRemovedFromSidebar: false,
  },
];

const INITIAL_TASKS: readonly TaskSessionSummary[] = [
  {
    taskId: "task-v3-shell",
    projectId: "proj-karyora",
    title: "Rebuild v3.0 Agentic Frontend Workspace",
    status: "idle",
    isPinned: true,
    isUnread: false,
    sessionMode: "default",
    modelId: "configured-default",
    thoughtLevel: "high",
    logEpoch: 1,
    updatedAt: "2m",
    diffAdditions: 57,
    diffDeletions: 3,
    changedFilesCount: 2,
  },
];

const INITIAL_TURNS: readonly ConversationTurnRecord[] = [
  {
    turnId: "turn-1",
    role: "user",
    timestamp: "Just now",
    text: "Inspect planstore_test.go and verify the UIR-4 timeline, Side Pane diff viewer, and sandboxed previewers.",
  },
  {
    turnId: "turn-2",
    role: "assistant",
    timestamp: "Just now",
    thinkingText:
      "Checking workspace files, resolving Material SVG language icons, and preparing tool execution summary.",
    toolRows: [
      {
        toolCallId: "tc-1",
        toolName: "edit_file",
        actionVerb: "Edited",
        state: "success",
        targetFilePath: "internal/plan/planstore_test.go",
        diffAdditions: 57,
        diffDeletions: 3,
        startedAt: "23:50:01",
        completedAt: "23:50:02",
        outputPreview: "Updated planstore_test.go (+57 -3)",
      },
      {
        toolCallId: "tc-2",
        toolName: "scan_secrets",
        actionVerb: "MCP Tool",
        mcpServerName: "Security",
        state: "success",
        startedAt: "23:50:03",
        completedAt: "23:50:04",
        outputPreview: "1.gitleaks: PASS",
      },
    ],
    text: "Verified `planstore_test.go` and ran the Security MCP check cleanly.",
  },
];

const INITIAL_PROGRESS_STEPS: readonly TimelineProgressStep[] = [
  { stepId: "s1", label: "Wire Multi-State Composer (@ / # $ +)", completed: true },
  { stepId: "s2", label: "Render 6-State Tool & MCP Rows", completed: true },
  { stepId: "s3", label: "Mount Multi-Tab Side Pane, Previewers & PTY Terminal", completed: true },
];

const INITIAL_SUBAGENT_RECORDS: readonly SubagentRunRecord[] = [
  {
    subagentId: "sub-1",
    name: "Codebase Researcher",
    role: "Architecture & Contract Auditor",
    status: "ended",
    elapsedLabel: "Ran for 2m 13s",
    toolCallsCount: 14,
    summaryMarkdown:
      "Audited OpenAPI contract surface, verified 1,147 Material SVG assets, and confirmed zero drift.",
    trajectorySteps: [
      {
        stepId: "st-1",
        action: "Searched",
        detail: "contracts/openapi.yaml (76 routes verified)",
        status: "success",
      },
      {
        stepId: "st-2",
        action: "Viewed",
        detail: "ui/src/components/icons/materialIconManifest.ts",
        status: "success",
      },
    ],
  },
  {
    subagentId: "sub-2",
    name: "Security Verifier",
    role: "G-12 Sandbox & Secret Gate",
    status: "running",
    elapsedLabel: "Running 18s",
    toolCallsCount: 4,
    summaryMarkdown:
      "Verifying zero-script enforcement across PDF, DOCX, XLSX, and PPTX previewers and checking Gitleaks/Semgrep/Trivy gates.",
    trajectorySteps: [
      {
        stepId: "st-3",
        action: "Checked",
        detail: "ui/src/previewers/previewerSanitizer.ts (G-12 pass)",
        status: "success",
      },
      {
        stepId: "st-4",
        action: "Scanning",
        detail: "Running static dependency & code checks...",
        status: "running",
      },
    ],
  },
];

const INITIAL_SUBAGENT_BADGES: readonly TimelineSubagentBadge[] =
  INITIAL_SUBAGENT_RECORDS.map((s) => ({
    subagentId: s.subagentId,
    name: s.name,
    status: s.status,
    elapsedLabel: s.elapsedLabel,
  }));

const INITIAL_WORKSPACE_FILES: readonly WorkspaceFileNode[] = [
  { path: "internal", name: "internal", isDirectory: true, depth: 0, parentPath: null },
  { path: "internal/plan", name: "plan", isDirectory: true, depth: 1, parentPath: "internal" },
  {
    path: "internal/plan/planstore_test.go",
    name: "planstore_test.go",
    isDirectory: false,
    depth: 2,
    parentPath: "internal/plan",
    gitStatus: "M",
    content: [
      "package plan",
      "",
      'import "testing"',
      "",
      "func TestPlanStoreDurability(t *testing.T) {",
      '  t.Log("verified deterministic WAL checkpoint and recovery fence")',
      "}",
    ].join("\n"),
  },
  { path: "ui", name: "ui", isDirectory: true, depth: 0, parentPath: null },
  { path: "ui/src", name: "src", isDirectory: true, depth: 1, parentPath: "ui" },
  {
    path: "ui/src/side-pane/MultiTabSidePane.tsx",
    name: "MultiTabSidePane.tsx",
    isDirectory: false,
    depth: 2,
    parentPath: "ui/src",
    gitStatus: "A",
    content: [
      "// Multi-Tab Side Pane (UIR-5.1)",
      "export const SIDE_PANE_SHORTCUT = 'Ctrl+Alt+B';",
      "export const FULL_VIEW_SHORTCUT = 'Ctrl+Shift+F';",
    ].join("\n"),
  },
  { path: "docs", name: "docs", isDirectory: true, depth: 0, parentPath: null },
  {
    path: "docs/release/Karyora-Security-Brief.pdf",
    name: "Karyora-Security-Brief.pdf",
    isDirectory: false,
    depth: 1,
    parentPath: "docs",
    gitStatus: null,
  },
  {
    path: "docs/architecture/Karyora-Platform-Spec.docx",
    name: "Karyora-Platform-Spec.docx",
    isDirectory: false,
    depth: 1,
    parentPath: "docs",
    gitStatus: null,
  },
  {
    path: "docs/benchmarks/Karyora-Latency-Matrix.xlsx",
    name: "Karyora-Latency-Matrix.xlsx",
    isDirectory: false,
    depth: 1,
    parentPath: "docs",
    gitStatus: null,
  },
  {
    path: "docs/decks/Karyora-v3-Architecture.pptx",
    name: "Karyora-v3-Architecture.pptx",
    isDirectory: false,
    depth: 1,
    parentPath: "docs",
    gitStatus: null,
  },
  { path: "public", name: "public", isDirectory: true, depth: 0, parentPath: null },
  {
    path: "public/preview.html",
    name: "preview.html",
    isDirectory: false,
    depth: 1,
    parentPath: "public",
    gitStatus: "A",
    content: [
      "<!doctype html>",
      '<html lang="en">',
      "  <head>",
      '    <meta charset="utf-8" />',
      "    <title>Karyora Built-In Browser Preview</title>",
      "    <style>",
      "      body { font-family: system-ui, sans-serif; margin: 0; padding: 32px; background: #171717; color: #f0f0f0; }",
      "      .card { max-width: 560px; margin: 0 auto; padding: 24px; border-radius: 14px; border: 1px solid rgba(255,255,255,.12); background: #232323; }",
      "      h1 { margin: 0 0 8px; font-size: 20px; }",
      "      p { color: #b5b5b5; line-height: 1.6; margin: 0; }",
      "    </style>",
      "  </head>",
      "  <body>",
      '    <div class="card">',
      "      <h1>Karyora Built-In Browser Live Preview</h1>",
      "      <p>Workspace HTML files open directly inside the embedded browser with responsive viewport presets (Desktop, Tablet, Phone, Slide) and element selection for chat.</p>",
      "    </div>",
      "  </body>",
      "</html>",
    ].join("\n"),
  },
];

const INITIAL_REVIEW_DIFFS: readonly ReviewDiffFileRecord[] = [
  {
    filePath: "internal/plan/planstore_test.go",
    gitStatus: "M",
    oldText: [
      "package plan",
      "",
      'import "testing"',
      "",
      "func TestPlanStoreDurability(t *testing.T) {",
      '  t.Log("basic planstore test")',
      "}",
    ].join("\n"),
    newText: [
      "package plan",
      "",
      'import "testing"',
      "",
      "func TestPlanStoreDurability(t *testing.T) {",
      '  t.Log("verified deterministic WAL checkpoint and recovery fence")',
      "}",
    ].join("\n"),
  },
  {
    filePath: "ui/src/side-pane/MultiTabSidePane.tsx",
    gitStatus: "A",
    oldText: "",
    newText: [
      "// Multi-Tab Side Pane (UIR-5.1)",
      "export const SIDE_PANE_SHORTCUT = 'Ctrl+Alt+B';",
      "export const FULL_VIEW_SHORTCUT = 'Ctrl+Shift+F';",
    ].join("\n"),
  },
];

const INITIAL_SIDE_TABS: readonly SidePaneTab[] = [
  {
    tabId: "tab-browser",
    kind: "browser",
    title: "Browser",
  },
  {
    tabId: "tab-files",
    kind: "files",
    title: "Files",
    filePath: "internal/plan/planstore_test.go",
  },
  { tabId: "tab-review", kind: "review", title: "Review" },
  { tabId: "tab-subagents", kind: "subagents", title: "Subagents" },
];

const INITIAL_DURABLE_JOBS: readonly DurableJobRecord[] = [
  {
    jobId: "job-0001",
    title: "Rebuild v3.0 UI Asset & Contract Verification",
    jobType: "agent-task",
    projectName: "karyora_Platform",
    status: "running",
    attemptCount: 1,
    checkpointLabel: "ckpt-step-3-verified",
    updatedAt: "1m ago",
    idempotencyKey: "idem-job-0001",
    correlationId: "corr-job-0001",
    events: [
      { seq: 1, timestamp: "23:50:00Z", kind: "job.created", summary: "Submitted to Core AIJobRelay" },
      { seq: 2, timestamp: "23:50:05Z", kind: "checkpoint.saved", summary: "Saved ckpt-step-3-verified" },
    ],
  },
  {
    jobId: "job-0002",
    title: "KnowledgePack SHA-256 Provenance Ingestion",
    jobType: "knowledge-ingest",
    projectName: "karyora_Platform",
    status: "waiting_approval",
    attemptCount: 1,
    checkpointLabel: "ckpt-awaiting-human-approval",
    updatedAt: "4m ago",
    idempotencyKey: "idem-job-0002",
    correlationId: "corr-job-0002",
    events: [
      { seq: 1, timestamp: "23:46:00Z", kind: "approval.requested", summary: "Waiting for human scope approval" },
    ],
  },
  {
    jobId: "job-0003",
    title: "Scheduled Dependency & Security Audit",
    jobType: "automation",
    projectName: "karyora_Platform",
    status: "succeeded",
    attemptCount: 1,
    checkpointLabel: "ckpt-terminal-receipt",
    updatedAt: "15m ago",
    idempotencyKey: "idem-job-0003",
    correlationId: "corr-job-0003",
    receiptDigest: "sha256:9f86d081884c7d659a2feaa0c55ad015",
    events: [
      { seq: 1, timestamp: "23:35:00Z", kind: "job.completed", summary: "Verified receipt committed" },
    ],
  },
];

const COMPOSER_SUGGESTIONS: readonly TriggerSuggestionItem[] = [
  {
    id: "file-planstore",
    trigger: "@",
    label: "planstore_test.go",
    subtitle: "internal/plan/planstore_test.go",
    filePathForIcon: "internal/plan/planstore_test.go",
  },
  {
    id: "file-composer",
    trigger: "@",
    label: "AgenticComposer.tsx",
    subtitle: "ui/src/workspace/composer/AgenticComposer.tsx",
    filePathForIcon: "ui/src/workspace/composer/AgenticComposer.tsx",
  },
  {
    id: "cmd-plan",
    trigger: "/",
    label: "plan",
    subtitle: "Generate an implementation plan before editing",
  },
  {
    id: "sess-shell",
    trigger: "#",
    label: "task-v3-shell",
    subtitle: "Rebuild v3.0 Agentic Frontend Workspace",
  },
  {
    id: "skill-security",
    trigger: "$",
    label: "security-review",
    subtitle: "Run security checklist & OWASP validation",
  },
];

export function App(): ReactElement {
  const coreClient = useMemo(() => createV3CoreClient(), []);
  const platformBridge = useMemo(() => createPlatformBridge(), []);
  const preferencesStore = useMemo(() => new LocalPreferencesStore(), []);
  const overlayStore = useMemo(() => new EphemeralUiStore(), []);
  const queueStore = useMemo(() => new SubmissionQueueStore(), []);

  const [prefs, setPrefs] = useState(DEFAULT_LOCAL_UI_PREFERENCES);
  const [activeSurface, setActiveSurface] = useState<ActiveMainSurface>("workspace");
  const [projects] = useState<readonly WorkspaceProjectSummary[]>(INITIAL_PROJECTS);
  const [tasks, setTasks] = useState<readonly TaskSessionSummary[]>(INITIAL_TASKS);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(
    INITIAL_TASKS[0]?.taskId ?? null,
  );
  const [turns, setTurns] = useState<readonly ConversationTurnRecord[]>(INITIAL_TURNS);
  const [jobs, setJobs] = useState<readonly DurableJobRecord[]>(INITIAL_DURABLE_JOBS);

  // Multi-State Composer state
  const [promptText, setPromptText] = useState("");
  const [chips, setChips] = useState<readonly ComposerContextChip[]>([]);
  const [composerMode, setComposerMode] = useState<ComposerSessionMode>("default");
  const [composerModel, setComposerModel] = useState("configured-default");
  const [composerThought, setComposerThought] = useState<ComposerThoughtLevel>("high");
  const [goalModeEnabled, setGoalModeEnabled] = useState(false);
  const [enabledPluginIds, setEnabledPluginIds] = useState<ReadonlySet<string>>(
    () => new Set(["plugin-pdf", "plugin-documents"]),
  );
  const [, setQueueTick] = useState(0);

  // Multi-Tab Side Pane, Antigravity Right Overview Rail, Full-View Mode & Docked Terminal state (UIR-5)
  const [terminalOpen, setTerminalOpen] = useState(false);
  const [terminalMaximized, setTerminalMaximized] = useState(false);
  const [overviewRailOpen, setOverviewRailOpen] = useState(true);
  const [overviewVisibility, setOverviewVisibility] = useState<OverviewSectionVisibilityMap>(
    () => {
      if (typeof window === "undefined") return createDefaultOverviewVisibility();
      try {
        return parseOverviewVisibilityJson(
          window.localStorage.getItem("karyora:ui:overview-sections-v1"),
        );
      } catch {
        return createDefaultOverviewVisibility();
      }
    },
  );
  const [sidePaneOpen, setSidePaneOpen] = useState(false);
  const [sidePaneFullView, setSidePaneFullView] = useState(false);
  const [sideTabs, setSideTabs] = useState<readonly SidePaneTab[]>(INITIAL_SIDE_TABS);
  const [recentlyClosedTabs, setRecentlyClosedTabs] = useState<readonly SidePaneTab[]>([]);
  const [activeSideTabId, setActiveSideTabId] = useState<string | null>("tab-browser");
  const [sitePermissionsStore, setSitePermissionsStore] = useState<SitePermissionsStoreMap>({});
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(
    INITIAL_PROJECTS[0]?.projectId ?? null,
  );
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(
    "internal/plan/planstore_test.go",
  );
  const [navHistory, setNavHistory] = useState<{
    readonly entries: readonly { readonly surface: ActiveMainSurface; readonly taskId: string | null }[];
    readonly index: number;
  }>({
    entries: [{ surface: "workspace", taskId: INITIAL_TASKS[0]?.taskId ?? null }],
    index: 0,
  });

  // Automations, Workflows & Plugin Marketplace state (UIR-6)
  const [scheduledTasks, setScheduledTasks] = useState<readonly ScheduledTaskRecord[]>(
    INITIAL_SCHEDULED_TASKS,
  );
  const [workflows, setWorkflows] = useState<readonly WorkflowDefinitionRecord[]>(
    INITIAL_WORKFLOWS,
  );
  const [marketplacePlugins, setMarketplacePlugins] = useState<
    readonly MarketplacePluginRecord[]
  >(INITIAL_MARKETPLACE_PLUGINS);

  // KnowledgePack, Scoped Memory, Context Inspector & Activity Audit state (UIR-7)
  const [knowledgePacks, setKnowledgePacks] = useState<readonly KnowledgePackRecord[]>(
    INITIAL_KNOWLEDGE_PACKS,
  );
  const [memoryItems, setMemoryItems] = useState<readonly ScopedMemoryItem[]>(
    INITIAL_MEMORY_ITEMS,
  );
  const [workspaceMemoryEnabled, setWorkspaceMemoryEnabled] = useState(true);
  const [auditEvents, setAuditEvents] = useState<readonly AuditEventRecord[]>(
    INITIAL_AUDIT_EVENTS,
  );

  // Shell overlay & panel toggles
  const [commandCenterOpen, setCommandCenterOpen] = useState(false);
  const [notificationDrawerOpen, setNotificationDrawerOpen] = useState(false);
  const [osNotificationsEnabled, setOsNotificationsEnabled] = useState(false);
  const [onboardingMode, setOnboardingMode] = useState<
    "closed" | "onboarding" | "migration"
  >("closed");
  const [settingsOverlayOpen, setSettingsOverlayOpen] = useState(false);
  const [mobileRemoteOpen, setMobileRemoteOpen] = useState(false);
  const [resourceManagerOpen, setResourceManagerOpen] = useState(false);
  const [browserUseEnabled, setBrowserUseEnabled] = useState(true);
  const [activeSettingsSection, setActiveSettingsSection] =
    useState<SettingsSectionId>("general");
  const [notifications, setNotifications] = useState<
    readonly CoreNotificationItem[]
  >(() =>
    deduplicateNotifications([], [
      {
        eventId: "evt-uir5-ready",
        category: "job",
        title: "Phase 3 Complete — Side Pane, Sandboxed Previewers & PTY Terminal Active",
        privacySafeSummary:
          "Multi-Tab Side Pane, Full-View Mode, G-12 Document Previewers, and Docked Terminal are ready.",
        occurredAt: "Just now",
        read: false,
        dismissed: false,
        targetSurface: "workspace",
      },
    ]),
  );
  const [toasts, setToasts] = useState<readonly ToastMessage[]>([]);

  const pushToast = (title: string, description?: string) => {
    const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    setToasts((prev) => [...prev, { id, title, description }]);
  };

  const activeTask = tasks.find((t) => t.taskId === activeTaskId) ?? null;
  const activeSessionKey = activeTask?.taskId ?? "draft-new-task";
  const queueSnapshot = queueStore.getSnapshot(activeSessionKey);
  const isTurnRunning = queueSnapshot.turnStatus === "running";

  const updatePref = <K extends keyof typeof prefs>(
    key: K,
    value: (typeof prefs)[K],
  ) => {
    const updated = preferencesStore.updatePreferences({ [key]: value });
    setPrefs(updated);
    try {
      if (key === "appTheme") {
        window.localStorage.setItem("karyora:ui:theme", String(value));
      } else if (key === "locale") {
        window.localStorage.setItem("karyora:ui:locale", String(value));
      }
    } catch {
      // ignore storage errors in restricted contexts
    }
  };

  // Restore non-secret saved locale and theme on mount (G-11 safe: zero credentials)
  useEffect(() => {
    try {
      const savedLocale = window.localStorage.getItem("karyora:ui:locale");
      const savedTheme = window.localStorage.getItem("karyora:ui:theme");
      const patch: Record<string, unknown> = {};
      if (savedLocale && savedLocale !== prefs.locale) {
        patch.locale = savedLocale as SupportedLocale;
      }
      if (savedTheme === "dark" || savedTheme === "light" || savedTheme === "system") {
        patch.appTheme = savedTheme as AppTheme;
      }
      if (Object.keys(patch).length > 0) {
        setPrefs(preferencesStore.updatePreferences(patch));
      }
    } catch {
      // ignore
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Apply App Theme (dark / light / system) live to documentElement (Screenshots #5, #63)
  useEffect(() => {
    if (typeof document === "undefined") return;
    const applyTheme = () => {
      const systemDark =
        typeof window !== "undefined" &&
        window.matchMedia?.("(prefers-color-scheme: dark)").matches;
      const resolvedDark =
        prefs.appTheme === "dark" ||
        (prefs.appTheme === "system" && systemDark);
      document.documentElement.classList.toggle("dark", resolvedDark);
      document.documentElement.dataset.theme = resolvedDark ? "dark" : "light";
    };
    applyTheme();
    if (prefs.appTheme === "system" && typeof window !== "undefined" && window.matchMedia) {
      const mq = window.matchMedia("(prefers-color-scheme: dark)");
      mq.addEventListener("change", applyTheme);
      return () => mq.removeEventListener("change", applyTheme);
    }
    return undefined;
  }, [prefs.appTheme]);

  // Apply UI Font Size & Interface Zoom live to documentElement (#5, #65)
  useEffect(() => {
    if (typeof document === "undefined") return;
    const scaledPx = Math.round((prefs.uiFontSizePx * prefs.interfaceZoomPercent) / 100);
    document.documentElement.style.setProperty("--ui-font-size", `${scaledPx}px`);
    document.documentElement.style.setProperty("--code-font-size", `${prefs.codeFontSizePx}px`);
  }, [prefs.uiFontSizePx, prefs.interfaceZoomPercent, prefs.codeFontSizePx]);

  useEffect(() => {
    void activateLocale(i18next, prefs.locale);
  }, [prefs.locale]);

  useEffect(() => {
    if (typeof document !== "undefined" && document.documentElement) {
      document.documentElement.dataset.platform = platformBridge.runtimeTarget;
    }
  }, [platformBridge.runtimeTarget]);

  const openSettingsOverlay = (
    section: SettingsSectionId = "general",
    triggerId?: string,
  ) => {
    overlayStore.openSettings(section, triggerId ?? null);
    setActiveSettingsSection(section);
    setSettingsOverlayOpen(true);
  };

  const closeSettingsOverlay = () => {
    const { restoreFocusTo } = overlayStore.closeSettings();
    setSettingsOverlayOpen(false);
    if (restoreFocusTo && typeof document !== "undefined") {
      document.getElementById(restoreFocusTo)?.focus();
    }
  };

  const handleToggleOverviewSection = (sectionId: OverviewSectionId) => {
    setOverviewVisibility((prev) => {
      const next = toggleOverviewSectionVisibility(prev, sectionId);
      try {
        window.localStorage.setItem(
          "karyora:ui:overview-sections-v1",
          JSON.stringify(next),
        );
      } catch {
        // ignore restricted storage
      }
      return next;
    });
  };

  const openSideTab = (nextTab: SidePaneTab) => {
    const res = openOrFocusSideTab(sideTabs, nextTab);
    setSideTabs(res.tabs);
    setActiveSideTabId(res.activeTabId);
    setSidePaneOpen(true);
  };

  const pushNavHistory = (surface: ActiveMainSurface, taskId: string | null) => {
    setNavHistory((prev) => {
      const cur = prev.entries[prev.index];
      if (cur && cur.surface === surface && cur.taskId === taskId) {
        return prev;
      }
      const nextEntries = [...prev.entries.slice(0, prev.index + 1), { surface, taskId }].slice(-30);
      return { entries: nextEntries, index: nextEntries.length - 1 };
    });
  };

  const handleGoBack = () => {
    setNavHistory((prev) => {
      if (prev.index <= 0) return prev;
      const nextIdx = prev.index - 1;
      const entry = prev.entries[nextIdx];
      if (entry) {
        setActiveSurface(entry.surface);
        setActiveTaskId(entry.taskId);
      }
      return { ...prev, index: nextIdx };
    });
  };

  const handleGoForward = () => {
    setNavHistory((prev) => {
      if (prev.index >= prev.entries.length - 1) return prev;
      const nextIdx = prev.index + 1;
      const entry = prev.entries[nextIdx];
      if (entry) {
        setActiveSurface(entry.surface);
        setActiveTaskId(entry.taskId);
      }
      return { ...prev, index: nextIdx };
    });
  };

  const handleToggleBrowser = () => {
    const browserTab = sideTabs.find((t) => t.kind === "browser");
    if (sidePaneOpen && browserTab && activeSideTabId === browserTab.tabId) {
      setSidePaneFullView(false);
      setSidePaneOpen(false);
      return;
    }
    openSideTab(
      browserTab ?? {
        tabId: "tab-browser",
        kind: "browser",
        title: "Browser",
      },
    );
  };

  const handleTaskTopBarAction = (
    actionId:
      | "open-in-editor"
      | "open-in-file-manager"
      | "copy-path"
      | "copy-session-id"
      | "reload-session"
      | "view-model-trajectory"
      | "open-resource-manager"
      | "go-to-config",
  ) => {
    if (actionId === "view-model-trajectory") {
      openSideTab({
        tabId: `tab-model-trajectory-${activeTask?.taskId ?? "current"}`,
        kind: "model-trajectory",
        title: "Model Trajectory",
      });
    } else if (actionId === "open-resource-manager") {
      setResourceManagerOpen(true);
    } else if (actionId === "go-to-config") {
      openSettingsOverlay("general");
    } else if (actionId === "open-in-editor" || actionId === "open-in-file-manager") {
      openSideTab({ tabId: "tab-files", kind: "files", title: "Files" });
    } else if (actionId === "copy-path") {
      const path = projects[0]?.path ?? "/home/km-abdullah/karyora_Platform";
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        void navigator.clipboard.writeText(path);
      }
      pushToast("Copied workspace path", path);
    } else if (actionId === "copy-session-id") {
      const sid = activeTask?.taskId ?? "draft-session";
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        void navigator.clipboard.writeText(sid);
      }
      pushToast("Copied session ID", sid);
    } else {
      pushToast("Session reloaded", activeTask?.title ?? "Workspace");
    }
  };

  const handleLaunchToolOrPlugin = (item: NewTabLauncherItem) => {
    if (item.kind === "document-preview" && item.sampleDocumentPath) {
      openSideTab({
        tabId: `doc-${item.id}`,
        kind: "document-preview",
        title: item.label,
        filePath: item.sampleDocumentPath,
      });
      return;
    }
    openSideTab({
      tabId: `tab-${item.kind}`,
      kind: item.kind,
      title: item.label,
    });
  };

  const handleSendSelectionToChat = (excerpt: string, sourceFile: string) => {
    setChips((prev) => [
      ...prev,
      {
        chipId: `sel-${Date.now()}`,
        trigger: "@",
        label: sourceFile.split("/").pop() ?? sourceFile,
        targetIdOrPath: sourceFile,
      },
    ]);
    setPromptText((prev) => (prev ? `${prev}\n${excerpt}` : excerpt));
    if (sidePaneFullView) {
      setSidePaneFullView(false);
    }
    pushToast("Sent selection to chat", sourceFile);
  };

  const activeCodeTheme =
    prefs.appTheme === "light" ? prefs.lightCodeTheme : prefs.darkCodeTheme;

  const handleSubmitPrompt = () => {
    const trimmed = promptText.trim();
    if (!trimmed && chips.length === 0) return;

    const result = queueStore.submitOrEnqueue({
      queueItemId: `q-${Date.now()}`,
      sessionId: activeSessionKey,
      promptText: trimmed || chips.map((c) => c.label).join(" "),
      chips,
      mode: composerMode,
      modelId: composerModel,
      thoughtLevel: composerThought,
      idempotencyKey: createIdempotencyKey("turn"),
      enqueuedAtMs: Date.now(),
    });
    setQueueTick((n) => n + 1);

    if (result.dispatchedImmediately) {
      setTurns((prev) => [
        ...prev,
        {
          turnId: `turn-${Date.now()}`,
          role: "user",
          timestamp: "Just now",
          text: trimmed || chips.map((c) => c.label).join(" "),
        },
      ]);
      if (!activeTaskId) {
        setActiveTaskId(INITIAL_TASKS[0]?.taskId ?? null);
      }
    } else {
      pushToast("Queued prompt", `Position #${result.snapshot.queuedItems.length}`);
    }
    setPromptText("");
    setChips([]);
  };

  return (
    <StartupHandshakeGate
      coreClient={coreClient}
      onOpenConnectionSettings={() => openSettingsOverlay("remote-devices")}
    >
      {(bootstrapStatus) => (
        <ApplicationShell
          runtimeTarget={platformBridge.runtimeTarget}
          bootstrapStatus={bootstrapStatus}
          activeTheme={prefs.appTheme}
          activeCodeTheme={activeCodeTheme}
          activeLocale={prefs.locale}
          activeInterfaceMode={prefs.interfaceMode}
          activeZoomPercent={prefs.interfaceZoomPercent}
          sidebarCollapsed={prefs.sidebarCollapsed}
          canGoBack={navHistory.index > 0}
          canGoForward={navHistory.index < navHistory.entries.length - 1}
          terminalOpen={terminalOpen}
          terminalMaximized={terminalMaximized}
          browserOpen={
            sidePaneOpen &&
            sideTabs.some((t) => t.tabId === activeSideTabId && t.kind === "browser")
          }
          overviewRailOpen={overviewRailOpen}
          sidePaneOpen={sidePaneOpen}
          sidePaneFullView={sidePaneFullView}
          activeSurface={activeSurface}
          projects={projects}
          tasks={tasks}
          activeTask={activeTask}
          notifications={notifications}
          osNotificationsEnabled={osNotificationsEnabled}
          commandCenterOpen={commandCenterOpen}
          notificationDrawerOpen={notificationDrawerOpen}
          onboardingMode={onboardingMode}
          settingsOverlayOpen={settingsOverlayOpen}
          mobileRemoteOpen={mobileRemoteOpen}
          resourceManagerOpen={resourceManagerOpen}
          toasts={toasts}
          canWrite={true}
          onToggleSidebar={() =>
            updatePref("sidebarCollapsed", !prefs.sidebarCollapsed)
          }
          onGoBack={handleGoBack}
          onGoForward={handleGoForward}
          onToggleBrowser={handleToggleBrowser}
          onToggleTerminal={() => setTerminalOpen((prev) => !prev)}
          onToggleSidePane={() => {
            if (sidePaneOpen) {
              setSidePaneFullView(false);
              setSidePaneOpen(false);
              setOverviewRailOpen(true);
            } else if (overviewRailOpen) {
              setOverviewRailOpen(false);
            } else {
              setOverviewRailOpen(true);
            }
          }}
          onToggleSidePaneFullView={() => {
            if (!sidePaneOpen) {
              setSidePaneOpen(true);
              setSidePaneFullView(true);
            } else {
              setSidePaneFullView((prev) => !prev);
            }
          }}
          onOpenReviewDiffTab={() =>
            openSideTab({ tabId: "tab-review", kind: "review", title: "Review" })
          }
          onShowProjectFiles={() =>
            openSideTab({ tabId: "tab-files", kind: "files", title: "Files" })
          }
          onTaskTopBarAction={handleTaskTopBarAction}
          onOpenMobileRemoteControl={() => setMobileRemoteOpen(true)}
          onCloseMobileRemoteControl={() => setMobileRemoteOpen(false)}
          onOpenResourceManager={() => setResourceManagerOpen(true)}
          onCloseResourceManager={() => setResourceManagerOpen(false)}
          onNotify={pushToast}
          onNewTask={() => {
            setActiveSurface("workspace");
            setActiveTaskId(null);
            setSidePaneFullView(false);
            pushNavHistory("workspace", null);
          }}
          onSelectSurface={(surface) => {
            setActiveSurface(surface);
            setSidePaneFullView(false);
            pushNavHistory(surface, activeTaskId);
          }}
          onSelectTask={(task) => {
            setActiveSurface("workspace");
            setActiveTaskId(task.taskId);
            setSidePaneFullView(false);
            pushNavHistory("workspace", task.taskId);
          }}
          onAddProject={() => {
            void platformBridge.pickFolderOrFile({ mode: "folder" });
            pushToast("Add project folder", "Workspace folder picker triggered.");
          }}
          onTaskContextAction={(actionId, task) => {
            if (actionId === "pin") {
              setTasks((prev) =>
                prev.map((t) =>
                  t.taskId === task.taskId ? { ...t, isPinned: !t.isPinned } : t,
                ),
              );
            } else if (actionId === "mark-as-unread") {
              setTasks((prev) =>
                prev.map((t) =>
                  t.taskId === task.taskId ? { ...t, isUnread: !t.isUnread } : t,
                ),
              );
            } else if (actionId === "archive") {
              setTasks((prev) =>
                prev.map((t) =>
                  t.taskId === task.taskId ? { ...t, status: "archived" } : t,
                ),
              );
              pushToast("Task archived", task.title);
            } else if (actionId === "delete") {
              setTasks((prev) => prev.filter((t) => t.taskId !== task.taskId));
              if (activeTaskId === task.taskId) {
                setActiveTaskId(null);
              }
              pushToast("Task deleted", task.title);
            } else if (actionId === "view-model-trajectory") {
              openSideTab({
                tabId: `tab-model-trajectory-${task.taskId}`,
                kind: "model-trajectory",
                title: "Model Trajectory",
              });
            } else {
              pushToast(`Task action: ${actionId}`, task.title);
            }
          }}
          onCreateShareLink={(taskId) => {
            pushToast("Share preflight requested", `Task ${taskId}`);
          }}
          onOpenSettings={openSettingsOverlay}
          onOpenCommandCenter={(triggerId) => {
            overlayStore.openCommandCenter(triggerId ?? null);
            setCommandCenterOpen(true);
          }}
          onCloseCommandCenter={() => {
            const { restoreFocusTo } = overlayStore.closeCommandCenter();
            setCommandCenterOpen(false);
            if (restoreFocusTo && typeof document !== "undefined") {
              document.getElementById(restoreFocusTo)?.focus();
            }
          }}
          onToggleNotifications={() =>
            setNotificationDrawerOpen((prev) => !prev)
          }
          onMarkNotificationRead={(eventId) =>
            setNotifications((prev) =>
              prev.map((n) => (n.eventId === eventId ? { ...n, read: true } : n)),
            )
          }
          onDismissNotification={(eventId) =>
            setNotifications((prev) =>
              prev.map((n) =>
                n.eventId === eventId ? { ...n, dismissed: true } : n,
              ),
            )
          }
          onToggleOsNotifications={setOsNotificationsEnabled}
          onOpenOnboardingMode={setOnboardingMode}
          onChangeLocale={(locale: SupportedLocale) => updatePref("locale", locale)}
          onChangeTheme={(theme: AppTheme) => updatePref("appTheme", theme)}
          onChangeInterfaceMode={(mode: InterfaceMode) =>
            updatePref("interfaceMode", mode)
          }
          onChangeZoomPercent={(zoom: number) =>
            updatePref("interfaceZoomPercent", zoom)
          }
          onDismissToast={(id) =>
            setToasts((prev) => prev.filter((t) => t.id !== id))
          }
          terminalContent={
            <DockedTerminalPanel
              runtimeTarget={platformBridge.runtimeTarget}
              remoteShellEnabledByCore={platformBridge.isDesktop}
              activeCodeTheme={activeCodeTheme}
              placement="bottom-dock"
              isMaximized={terminalMaximized}
              onToggleMaximize={() => setTerminalMaximized((m) => !m)}
              onClose={() => {
                setTerminalMaximized(false);
                setTerminalOpen(false);
              }}
            />
          }
          overviewRailContent={
            <WorkspaceOverviewRail
              visibility={overviewVisibility}
              activeCodeTheme={activeCodeTheme}
              subagents={INITIAL_SUBAGENT_RECORDS}
              backgroundJobs={jobs}
              goalSteps={INITIAL_PROGRESS_STEPS}
              workspaceFiles={INITIAL_WORKSPACE_FILES}
              artifacts={INITIAL_OVERVIEW_ARTIFACTS}
              modifiedFiles={INITIAL_REVIEW_DIFFS}
              uploads={INITIAL_OVERVIEW_UPLOADS}
              plugins={marketplacePlugins}
              knowledgePacks={knowledgePacks}
              memoryItems={memoryItems}
              contextSnapshots={INITIAL_CONTEXT_SNAPSHOTS}
              auditEvents={auditEvents}
              onToggleSection={handleToggleOverviewSection}
              onOpenReview={(fp) =>
                openSideTab({
                  tabId: "tab-review",
                  kind: "review",
                  title: "Review",
                  filePath: fp,
                })
              }
              onOpenFiles={(fp) => {
                if (fp) setSelectedFilePath(fp);
                openSideTab({
                  tabId: "tab-files",
                  kind: "files",
                  title: "Files",
                  filePath: fp,
                });
              }}
              onOpenBrowser={(url) =>
                openSideTab({
                  tabId: "tab-browser",
                  kind: "browser",
                  title: "Browser",
                  browserUrl: url,
                })
              }
              onOpenTerminal={() => setTerminalOpen((prev) => !prev)}
              onOpenExtensions={() => {
                setActiveSurface("marketplace");
                pushNavHistory("marketplace", activeTaskId);
              }}
              onInspectSubagent={(sub) =>
                openSideTab({
                  tabId: `subagent-report-${sub.subagentId}`,
                  kind: "subagent-report",
                  title: sub.name,
                  subagentId: sub.subagentId,
                })
              }
              onOpenSubagentsDirectory={() =>
                openSideTab({
                  tabId: "tab-subagents",
                  kind: "subagents",
                  title: "Subagents",
                })
              }
              onOpenJobsSurface={() => {
                setActiveSurface("jobs");
                pushNavHistory("jobs", activeTaskId);
              }}
              onOpenDocumentPreview={(filePath, title) =>
                openSideTab({
                  tabId: `doc-${filePath}`,
                  kind: "document-preview",
                  title,
                  filePath,
                })
              }
              onOpenKnowledgeSurface={() => {
                setActiveSurface("knowledge");
                pushNavHistory("knowledge", activeTaskId);
              }}
              onOpenMemorySurface={() => {
                setActiveSurface("memory");
                pushNavHistory("memory", activeTaskId);
              }}
              onOpenContextSurface={() => {
                setActiveSurface("context");
                pushNavHistory("context", activeTaskId);
              }}
              onOpenActivitySurface={() => {
                setActiveSurface("activity");
                pushNavHistory("activity", activeTaskId);
              }}
            />
          }
          sidePaneContent={
            <MultiTabSidePane
              tabs={sideTabs}
              recentlyClosedTabs={recentlyClosedTabs}
              activeTabId={activeSideTabId}
              isFullView={sidePaneFullView}
              runtimeTarget={platformBridge.runtimeTarget}
              remoteShellEnabledByCore={platformBridge.isDesktop}
              browserUseEnabled={browserUseEnabled}
              sitePermissionsStore={sitePermissionsStore}
              activeCodeTheme={activeCodeTheme}
              workspaceName={projects[0]?.name ?? "karyora_Platform"}
              activeTaskTitle={activeTask?.title ?? "Workspace session"}
              conversationTurns={turns}
              workspaceFiles={INITIAL_WORKSPACE_FILES}
              selectedFilePath={selectedFilePath}
              reviewDiffFiles={INITIAL_REVIEW_DIFFS}
              subagents={INITIAL_SUBAGENT_RECORDS}
              onSelectTab={setActiveSideTabId}
              onCloseTab={(tabId) => {
                const closing = sideTabs.find((t) => t.tabId === tabId);
                if (closing && closing.kind !== "new-tab") {
                  setRecentlyClosedTabs((prev) => [
                    { ...closing, closedAt: Date.now() },
                    ...prev.filter((p) => p.tabId !== closing.tabId),
                  ].slice(0, 12));
                }
                const res = closeSideTab(sideTabs, activeSideTabId, tabId);
                setSideTabs(res.tabs);
                setActiveSideTabId(res.activeTabId);
                if (res.tabs.length === 0) {
                  setSidePaneFullView(false);
                  setSidePaneOpen(false);
                }
              }}
              onReopenClosedTab={(closedTab) => {
                setRecentlyClosedTabs((prev) =>
                  prev.filter((t) => t.tabId !== closedTab.tabId),
                );
                openSideTab(closedTab);
              }}
              onOpenNewTabLauncher={() =>
                openSideTab({
                  tabId: "tab-new-launcher",
                  kind: "new-tab",
                  title: "New tab",
                })
              }
              onLaunchToolOrPlugin={handleLaunchToolOrPlugin}
              onOpenMarketplace={() => {
                setSidePaneFullView(false);
                setActiveSurface("marketplace");
              }}
              onSelectFile={(fp) => setSelectedFilePath(fp)}
              onOpenHtmlInBrowser={(filePath, htmlContent) =>
                openSideTab({
                  tabId: `browser-html-${filePath}`,
                  kind: "browser",
                  title: filePath.split("/").pop() ?? "HTML Preview",
                  filePath,
                  htmlSourceContent: htmlContent,
                })
              }
              onOpenSitePermissions={(origin) =>
                openSideTab({
                  tabId: `browser-permissions-${origin}`,
                  kind: "browser-permissions",
                  title: "Site permissions",
                  browserOrigin: origin,
                })
              }
              onSetSitePermission={(origin, key, decision) =>
                setSitePermissionsStore((prev) =>
                  setOriginPermission(prev, origin, key, decision),
                )
              }
              onResetSitePermissions={(origin) =>
                setSitePermissionsStore((prev) =>
                  resetOriginPermissions(prev, origin),
                )
              }
              onInspectSubagent={(sub) =>
                openSideTab({
                  tabId: `subagent-report-${sub.subagentId}`,
                  kind: "subagent-report",
                  title: sub.name,
                  subagentId: sub.subagentId,
                })
              }
              onToggleFullView={() => setSidePaneFullView((prev) => !prev)}
              onClosePane={() => {
                setSidePaneFullView(false);
                setSidePaneOpen(false);
              }}
              onSendSelectionToChat={handleSendSelectionToChat}
              onOpenBrowserUseSettings={() => openSettingsOverlay("browser-use")}
              onOpenExternalUrl={(url, domain) =>
                pushToast(`Opened external URL (${domain})`, url)
              }
              onOpenContextInspectorPage={() => {
                setSidePaneFullView(false);
                setActiveSurface("context");
              }}
              onNotify={pushToast}
            />
          }
          floatingWorkingBar={
            <FloatingMinimizedWorkingBar
              isVisible={sidePaneOpen && sidePaneFullView}
              isWorking={isTurnRunning || INITIAL_SUBAGENT_RECORDS.some((s) => s.status === "running")}
              elapsedSeconds={1597}
              activeTaskTitle={activeTask?.title ?? "Workspace"}
              onRestoreSplitView={() => setSidePaneFullView(false)}
            />
          }
          settingsOverlayContent={
            <Suspense
              fallback={
                <div className="ky-settings-overlay" role="status">
                  <div style={{ margin: "auto", fontSize: "0.9rem" }}>
                    Loading 21-Section Settings Hub...
                  </div>
                </div>
              }
            >
              <LazySettingsOverlayHub
                isOpen={settingsOverlayOpen}
                activeSection={activeSettingsSection}
                runtimeTarget={platformBridge.runtimeTarget}
                computerUseSupported={platformBridge.capabilities.computerUse}
                appTheme={prefs.appTheme}
                lightCodeTheme={prefs.lightCodeTheme}
                darkCodeTheme={prefs.darkCodeTheme}
                locale={prefs.locale}
                interfaceMode={prefs.interfaceMode}
                browserUseEnabled={browserUseEnabled}
                projects={projects}
                plugins={marketplacePlugins}
                memoryProps={{
                  items: memoryItems,
                  workspaceMemoryEnabled,
                  availableProjects: projects.map((p) => p.name),
                  activeCodeTheme,
                  onToggleWorkspaceMemory: setWorkspaceMemoryEnabled,
                  onApproveItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId
                          ? { ...m, state: "active", updatedAt: "Just now" }
                          : m,
                      ),
                    ),
                  onRejectItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId ? forgetMemoryItem(m).item : m,
                      ),
                    ),
                  onEditItemContent: (memoryId, nextContent) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId
                          ? { ...m, content: nextContent, updatedAt: "Just now" }
                          : m,
                      ),
                    ),
                  onTogglePinItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId ? { ...m, pinned: !m.pinned } : m,
                      ),
                    ),
                  onForgetItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId ? forgetMemoryItem(m).item : m,
                      ),
                    ),
                  onRestoreItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId ? restoreMemoryItem(m).item : m,
                      ),
                    ),
                  onPermanentDeleteItem: (memoryId) =>
                    setMemoryItems((prev) =>
                      prev.map((m) =>
                        m.memoryId === memoryId
                          ? permanentlyDeleteMemoryItem(m, `aud-del-${Date.now()}`).item
                          : m,
                      ),
                    ),
                }}
                coreSurfaceViews={{
                  jobs: (
                    <JobsPage
                      jobs={jobs}
                      onCancelJob={(jobId) => {
                        setJobs((prev) =>
                          prev.map((j) =>
                            j.jobId === jobId ? { ...j, status: "cancelled" } : j,
                          ),
                        );
                        pushToast("Safe cancel dispatched", jobId);
                      }}
                      onRetryOrResumeJob={(jobId) => {
                        setJobs((prev) =>
                          prev.map((j) =>
                            j.jobId === jobId
                              ? { ...j, status: "running", attemptCount: j.attemptCount + 1 }
                              : j,
                          ),
                        );
                        pushToast("Job resumed", jobId);
                      }}
                      onReconcileS4Job={(jobId) => {
                        setJobs((prev) =>
                          prev.map((j) =>
                            j.jobId === jobId ? { ...j, status: "running" } : j,
                          ),
                        );
                        pushToast("Reconciled S4 Job state", jobId);
                      }}
                    />
                  ),
                  knowledge: (
                    <KnowledgePackPage
                      packs={knowledgePacks}
                      onCreatePackAndIngest={(name, sourceUri) => {
                        pushToast("KnowledgePack queued", `${name} (${sourceUri})`);
                      }}
                      onExportManifest={(pack) =>
                        pushToast("Exported KnowledgePack manifest", pack.name)
                      }
                      onInspectIngestionJob={() => {
                        closeSettingsOverlay();
                        setActiveSurface("jobs");
                      }}
                    />
                  ),
                  context: <ContextInspectorView snapshots={INITIAL_CONTEXT_SNAPSHOTS} />,
                  activity: (
                    <ActivityAuditPage
                      events={auditEvents}
                      onAppendCorrectionNote={(targetEventId, note) => {
                        const corr = createAppendOnlyCorrectionEvent(
                          auditEvents,
                          targetEventId,
                          note,
                        );
                        setAuditEvents((prev) => [corr, ...prev]);
                        pushToast("Correction appended", `${corr.eventId} -> ${targetEventId}`);
                      }}
                      onExportRedactedAudit={(_json, count) => {
                        pushToast("Redacted audit exported", `${count} sanitized event(s)`);
                      }}
                    />
                  ),
                  automations: (
                    <AutomationsAndWorkflowsPage
                      scheduledTasks={scheduledTasks}
                      workflows={workflows}
                      availableModels={["configured-default", "claude-sonnet", "gpt-5", "gemini-pro"]}
                      onSaveScheduledTask={(task) => {
                        setScheduledTasks((prev) => {
                          const exists = prev.some((t) => t.taskId === task.taskId);
                          return exists
                            ? prev.map((t) => (t.taskId === task.taskId ? task : t))
                            : [task, ...prev];
                        });
                        pushToast("Scheduled task saved", task.name);
                      }}
                      onToggleScheduledTaskActive={(taskId, active) =>
                        setScheduledTasks((prev) =>
                          prev.map((t) => (t.taskId === taskId ? { ...t, active } : t)),
                        )
                      }
                      onDeleteScheduledTask={(taskId) => {
                        setScheduledTasks((prev) => prev.filter((t) => t.taskId !== taskId));
                        pushToast("Scheduled task deleted", taskId);
                      }}
                      onRunScheduledTaskNow={(task) =>
                        pushToast("Automation dispatched", task.name)
                      }
                      onSaveWorkflowRevision={(updated) =>
                        setWorkflows((prev) =>
                          prev.map((w) => (w.workflowId === updated.workflowId ? updated : w)),
                        )
                      }
                      onCreateWorkflow={() => pushToast("Workflow created")}
                      onLaunchWorkflowJob={(wf) => pushToast("Workflow launched", wf.name)}
                      onPickProjectFolder={() => {
                        void platformBridge.pickFolderOrFile({ mode: "folder" });
                      }}
                      onOpenJobHistory={() => {
                        closeSettingsOverlay();
                        setActiveSurface("jobs");
                      }}
                    />
                  ),
                }}
                onOpenCoreSurfaceInWorkspace={(surface) => {
                  setActiveSurface(surface);
                  pushNavHistory(surface, activeTaskId);
                }}
                onSelectSection={setActiveSettingsSection}
                onChangeAppTheme={(theme) => updatePref("appTheme", theme)}
                onChangeLightCodeTheme={(theme) => updatePref("lightCodeTheme", theme)}
                onChangeDarkCodeTheme={(theme) => updatePref("darkCodeTheme", theme)}
                onChangeLocale={(loc) => updatePref("locale", loc)}
                onChangeInterfaceMode={(mode) => updatePref("interfaceMode", mode)}
                onToggleBrowserUse={setBrowserUseEnabled}
                onTogglePlugin={(pluginId, enabled) =>
                  setMarketplacePlugins((prev) =>
                    prev.map((p) =>
                      p.pluginId === pluginId
                        ? {
                            ...p,
                            lifecycleState: enabled
                              ? "installed-enabled"
                              : "installed-disabled",
                          }
                        : p,
                    ),
                  )
                }
                onBrowseMarketplace={() => {
                  closeSettingsOverlay();
                  setActiveSurface("marketplace");
                }}
                onSwitchCoreConnection={(mode) =>
                  pushToast(
                    `Switched Core connection (${mode})`,
                    "Purged DomainStateCache cleanly.",
                  )
                }
                onOpenOnboarding={() => {
                  closeSettingsOverlay();
                  setOnboardingMode("onboarding");
                }}
                onOpenMigrationGuide={() => {
                  closeSettingsOverlay();
                  setOnboardingMode("migration");
                }}
                onNotify={(msg) => pushToast("Settings", msg)}
                onClose={closeSettingsOverlay}
              />
            </Suspense>
          }
        >
          {activeSurface === "workspace" ? (
            <div
              className={
                activeTask === null
                  ? "ky-workspace-stage ky-workspace-stage--empty"
                  : "ky-workspace-stage"
              }
            >
              {activeTask === null ? (
                <GreetingEmptyState
                  projects={projects}
                  selectedProjectId={selectedProjectId}
                  onSelectProject={setSelectedProjectId}
                  onAddProject={() => {
                    void platformBridge.pickFolderOrFile({ mode: "folder" });
                    pushToast("Add project folder", "Workspace folder picker triggered.");
                  }}
                  onSelectQuickChip={(chip) => setPromptText(chip.promptTemplate)}
                />
              ) : (
                <ConversationTimeline
                  turns={turns}
                  progressSteps={INITIAL_PROGRESS_STEPS}
                  subagents={INITIAL_SUBAGENT_BADGES}
                  codeTheme={activeCodeTheme}
                  onInspectFile={(filePath) => {
                    setSelectedFilePath(filePath);
                    openSideTab({
                      tabId: "tab-files",
                      kind: "files",
                      title: "Files",
                      filePath,
                    });
                  }}
                  onEditUserTurn={(text) => {
                    setPromptText(text);
                    pushToast("Loaded prompt for editing");
                  }}
                  onForkTurn={(turnId) => {
                    const forkId = `task-fork-${Date.now().toString().slice(-4)}`;
                    setTasks((prev) => [
                      {
                        taskId: forkId,
                        projectId: projects[0]?.projectId ?? "proj-karyora",
                        title: `Fork: ${activeTask.title}`,
                        status: "idle",
                        isPinned: false,
                        isUnread: false,
                        sessionMode: composerMode,
                        modelId: composerModel,
                        thoughtLevel: composerThought,
                        logEpoch: 1,
                        updatedAt: "now",
                        diffAdditions: activeTask.diffAdditions,
                        diffDeletions: activeTask.diffDeletions,
                        changedFilesCount: activeTask.changedFilesCount,
                      },
                      ...prev,
                    ]);
                    setActiveTaskId(forkId);
                    pushToast("Conversation forked", `Forked from ${turnId}`);
                  }}
                  onOpenHooksSettings={() => openSettingsOverlay("hooks")}
                  onNotify={pushToast}
                  onApprovalDecision={(approvalId, decision) => {
                    pushToast("Approval recorded", `${approvalId}: ${decision}`);
                  }}
                  onOpenSubagentsSideTab={() =>
                    openSideTab({
                      tabId: "tab-subagents",
                      kind: "subagents",
                      title: "Subagents",
                    })
                  }
                />
              )}

              <AgenticComposer
                promptText={promptText}
                chips={chips}
                mode={composerMode}
                modelId={composerModel}
                availableModels={["configured-default", "claude-sonnet", "gpt-5", "gemini-pro"]}
                thoughtLevel={composerThought}
                goalModeEnabled={goalModeEnabled}
                enabledPluginIds={enabledPluginIds}
                isRunning={isTurnRunning}
                isCoreOffline={false}
                codeTheme={activeCodeTheme}
                queueSnapshot={queueSnapshot}
                triggerSuggestions={COMPOSER_SUGGESTIONS}
                onOpenContextInspector={() =>
                  openSideTab({
                    tabId: "tab-context-inspector",
                    kind: "context-inspector",
                    title: "Context Inspector",
                  })
                }
                onPromptChange={setPromptText}
                onAddChip={(chip) => setChips((prev) => [...prev, chip])}
                onRemoveChip={(chipId) =>
                  setChips((prev) => prev.filter((c) => c.chipId !== chipId))
                }
                onChangeMode={setComposerMode}
                onChangeModel={setComposerModel}
                onChangeThoughtLevel={setComposerThought}
                onToggleGoalMode={setGoalModeEnabled}
                onTogglePlugin={(pluginId) =>
                  setEnabledPluginIds((prev) => {
                    const next = new Set(prev);
                    if (next.has(pluginId)) next.delete(pluginId);
                    else next.add(pluginId);
                    return next;
                  })
                }
                onPickAttachment={() => {
                  void platformBridge.pickFolderOrFile({ mode: "multiple-files" });
                  pushToast("Attach files", "Select workspace files or documents.");
                }}
                onOpenWorkflowPicker={() => setActiveSurface("automations")}
                onSubmitPrompt={handleSubmitPrompt}
                onStopTurn={() => {
                  queueStore.onTurnCompletedS2(activeSessionKey);
                  setQueueTick((n) => n + 1);
                  pushToast("Turn stopped", "Stop requested cleanly.");
                }}
                onQueueReorder={(from, to) => {
                  queueStore.reorder(activeSessionKey, from, to);
                  setQueueTick((n) => n + 1);
                }}
                onQueueEdit={(id, text) => {
                  queueStore.editItem(activeSessionKey, id, { promptText: text });
                  setQueueTick((n) => n + 1);
                }}
                onQueueSendNow={(id) => {
                  queueStore.promoteToSendNow(activeSessionKey, id);
                  setQueueTick((n) => n + 1);
                }}
                onQueueRemove={(id) => {
                  queueStore.removeItem(activeSessionKey, id);
                  setQueueTick((n) => n + 1);
                }}
              />
            </div>
          ) : activeSurface === "jobs" ? (
            <JobsPage
              jobs={jobs}
              onCancelJob={(jobId) => {
                setJobs((prev) =>
                  prev.map((j) =>
                    j.jobId === jobId ? { ...j, status: "cancelled" } : j,
                  ),
                );
                pushToast("Safe cancel dispatched", jobId);
              }}
              onRetryOrResumeJob={(jobId) => {
                setJobs((prev) =>
                  prev.map((j) =>
                    j.jobId === jobId
                      ? { ...j, status: "running", attemptCount: j.attemptCount + 1 }
                      : j,
                  ),
                );
                pushToast("Job resumed", jobId);
              }}
              onReconcileS4Job={(jobId) => {
                setJobs((prev) =>
                  prev.map((j) =>
                    j.jobId === jobId ? { ...j, status: "running" } : j,
                  ),
                );
                pushToast("Reconciled S4 Job state", jobId);
              }}
            />
          ) : activeSurface === "automations" ? (
            <AutomationsAndWorkflowsPage
              scheduledTasks={scheduledTasks}
              workflows={workflows}
              availableModels={["configured-default", "claude-sonnet", "gpt-5", "gemini-pro"]}
              onSaveScheduledTask={(task) => {
                setScheduledTasks((prev) => {
                  const exists = prev.some((t) => t.taskId === task.taskId);
                  return exists
                    ? prev.map((t) => (t.taskId === task.taskId ? task : t))
                    : [task, ...prev];
                });
                pushToast("Scheduled task saved", task.name);
              }}
              onToggleScheduledTaskActive={(taskId, active) =>
                setScheduledTasks((prev) =>
                  prev.map((t) => (t.taskId === taskId ? { ...t, active } : t)),
                )
              }
              onDeleteScheduledTask={(taskId) => {
                setScheduledTasks((prev) => prev.filter((t) => t.taskId !== taskId));
                pushToast("Scheduled task deleted", taskId);
              }}
              onRunScheduledTaskNow={(task) => {
                const newJobId = `job-auto-${Date.now().toString().slice(-4)}`;
                setJobs((prev) => [
                  {
                    jobId: newJobId,
                    title: `Automation: ${task.name}`,
                    jobType: "automation",
                    projectName: projects[0]?.name ?? "karyora_Platform",
                    status: "running",
                    attemptCount: 1,
                    checkpointLabel: "ckpt-automation-dispatched",
                    updatedAt: "Just now",
                    idempotencyKey: createIdempotencyKey("auto"),
                    correlationId: `corr-${newJobId}`,
                    events: [
                      {
                        seq: 1,
                        timestamp: new Date().toISOString(),
                        kind: "job.created",
                        summary: `Triggered scheduled task "${task.name}"`,
                      },
                    ],
                  },
                  ...prev,
                ]);
                pushToast("Automation dispatched", `Created durable Job ${newJobId}`);
              }}
              onSaveWorkflowRevision={(updated) => {
                setWorkflows((prev) =>
                  prev.map((w) => (w.workflowId === updated.workflowId ? updated : w)),
                );
                pushToast("Workflow revision saved", `${updated.name} (${updated.currentRevision})`);
              }}
              onCreateWorkflow={() => {
                const nextId = `wf-${Date.now()}`;
                const fresh: WorkflowDefinitionRecord = {
                  workflowId: nextId,
                  name: `Workflow ${workflows.length + 1}`,
                  description: "New Core-validated workflow DAG",
                  currentRevision: "rev-001",
                  versionNumber: 1,
                  nodes: [
                    {
                      nodeId: `n-trig-${Date.now()}`,
                      title: "Manual Trigger",
                      kind: "trigger",
                      description: "Starts workflow execution",
                      configSummary: "Manual",
                      x: 36,
                      y: 80,
                    },
                  ],
                  edges: [],
                  revisions: [
                    {
                      revisionId: "rev-001",
                      versionNumber: 1,
                      savedAt: "Just now",
                      summary: "Initial workflow draft",
                      nodes: [
                        {
                          nodeId: `n-trig-${Date.now()}`,
                          title: "Manual Trigger",
                          kind: "trigger",
                          description: "Starts workflow execution",
                          configSummary: "Manual",
                          x: 36,
                          y: 80,
                        },
                      ],
                      edges: [],
                    },
                  ],
                };
                setWorkflows((prev) => [...prev, fresh]);
                pushToast("Created workflow", fresh.name);
              }}
              onLaunchWorkflowJob={(wf) => {
                const newJobId = `job-wf-${Date.now().toString().slice(-4)}`;
                setJobs((prev) => [
                  {
                    jobId: newJobId,
                    title: `Workflow: ${wf.name} (v${wf.versionNumber})`,
                    jobType: "workflow-run",
                    projectName: projects[0]?.name ?? "karyora_Platform",
                    status: "running",
                    attemptCount: 1,
                    checkpointLabel: `ckpt-${wf.currentRevision}`,
                    updatedAt: "Just now",
                    idempotencyKey: createIdempotencyKey("wf"),
                    correlationId: `corr-${newJobId}`,
                    events: [
                      {
                        seq: 1,
                        timestamp: new Date().toISOString(),
                        kind: "workflow.dispatched",
                        summary: `Launched ${wf.nodes.length}-step validated DAG (${wf.currentRevision})`,
                      },
                    ],
                  },
                  ...prev,
                ]);
                pushToast("Workflow launched", `Durable Job ${newJobId} started`);
              }}
              onPickProjectFolder={() => {
                void platformBridge.pickFolderOrFile({ mode: "folder" });
                pushToast("Select project folder", "Workspace folder picker triggered.");
              }}
              onOpenJobHistory={() => setActiveSurface("jobs")}
            />
          ) : activeSurface === "marketplace" ? (
            <PluginMarketplacePage
              plugins={marketplacePlugins}
              onInstallPlugin={(pluginId) => {
                setMarketplacePlugins((prev) =>
                  prev.map((p) =>
                    p.pluginId === pluginId
                      ? { ...p, lifecycleState: "installed-enabled" }
                      : p,
                  ),
                );
                pushToast("Plugin installed", pluginId);
              }}
              onTogglePluginEnabled={(pluginId, enabled) => {
                setMarketplacePlugins((prev) =>
                  prev.map((p) =>
                    p.pluginId === pluginId
                      ? {
                          ...p,
                          lifecycleState: enabled
                            ? "installed-enabled"
                            : "installed-disabled",
                        }
                      : p,
                  ),
                );
                pushToast(enabled ? "Plugin enabled" : "Plugin disabled", pluginId);
              }}
              onRemovePlugin={(pluginId) => {
                setMarketplacePlugins((prev) =>
                  prev.map((p) =>
                    p.pluginId === pluginId ? { ...p, lifecycleState: "available" } : p,
                  ),
                );
                pushToast("Plugin removed", pluginId);
              }}
              onRevokePlugin={(pluginId) => {
                setMarketplacePlugins((prev) =>
                  prev.map((p) =>
                    p.pluginId === pluginId ? { ...p, lifecycleState: "revoked" } : p,
                  ),
                );
                pushToast("Plugin trust revoked", pluginId);
              }}
              onOpenPluginsSettings={() => openSettingsOverlay("plugins")}
            />
          ) : activeSurface === "knowledge" ? (
            <KnowledgePackPage
              packs={knowledgePacks}
              onCreatePackAndIngest={(name, sourceUri) => {
                const newJobId = `job-kp-${Date.now().toString().slice(-4)}`;
                const packId = `kp-${Date.now()}`;
                setKnowledgePacks((prev) => [
                  {
                    packId,
                    name,
                    description: `Ingested from ${sourceUri}`,
                    versionNumber: 1,
                    profile: "uawp-kp-internal-digest-v1",
                    integrityState: "pending-ingest",
                    manifestSha256:
                      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
                    signatureStatus: "signed-local-key",
                    lastIngestionJobId: newJobId,
                    updatedAt: "Just now",
                    sources: [
                      {
                        sourceId: `src-${Date.now()}`,
                        title: sourceUri.split("/").pop() || name,
                        originUri: sourceUri,
                        sha256:
                          "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
                        byteSize: 4096,
                        importedAt: "Just now",
                        rawPreviewExcerpt: `Queued ingestion source from ${sourceUri}`,
                      },
                    ],
                    versions: [
                      {
                        versionNumber: 1,
                        createdAt: "Just now",
                        manifestSha256:
                          "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
                        sourceCount: 1,
                        summary: "Initial ingestion queued",
                      },
                    ],
                  },
                  ...prev,
                ]);
                setJobs((prev) => [
                  {
                    jobId: newJobId,
                    title: `KnowledgePack Ingest: ${name}`,
                    jobType: "knowledge-ingest",
                    projectName: projects[0]?.name ?? "karyora_Platform",
                    status: "running",
                    attemptCount: 1,
                    checkpointLabel: "ckpt-sha256-verifying",
                    updatedAt: "Just now",
                    idempotencyKey: createIdempotencyKey("kp"),
                    correlationId: `corr-${newJobId}`,
                    events: [
                      {
                        seq: 1,
                        timestamp: new Date().toISOString(),
                        kind: "knowledge.ingest.started",
                        summary: `Verifying ${sourceUri}`,
                      },
                    ],
                  },
                  ...prev,
                ]);
                pushToast("KnowledgePack ingestion started", `Job ${newJobId}`);
              }}
              onExportManifest={(pack) => {
                pushToast("Exported KnowledgePack manifest", `${pack.name} (v${pack.versionNumber})`);
              }}
              onInspectIngestionJob={() => setActiveSurface("jobs")}
            />
          ) : activeSurface === "memory" ? (
            <ScopedMemorySurface
              items={memoryItems}
              workspaceMemoryEnabled={workspaceMemoryEnabled}
              availableProjects={projects.map((p) => p.name)}
              activeCodeTheme={activeCodeTheme}
              onToggleWorkspaceMemory={setWorkspaceMemoryEnabled}
              onApproveItem={(memoryId) => {
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId ? { ...m, state: "active", updatedAt: "Just now" } : m,
                  ),
                );
                pushToast("Memory item approved", memoryId);
              }}
              onRejectItem={(memoryId) => {
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId ? forgetMemoryItem(m).item : m,
                  ),
                );
                pushToast("Pending memory rejected (soft-excluded)", memoryId);
              }}
              onEditItemContent={(memoryId, nextContent) => {
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId
                      ? { ...m, content: nextContent, updatedAt: "Just now" }
                      : m,
                  ),
                );
                pushToast("Memory updated", memoryId);
              }}
              onTogglePinItem={(memoryId) =>
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId ? { ...m, pinned: !m.pinned } : m,
                  ),
                )
              }
              onForgetItem={(memoryId) => {
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId ? forgetMemoryItem(m).item : m,
                  ),
                );
                setAuditEvents((prev) => [
                  {
                    eventId: `aud-mem-${Date.now()}`,
                    seq: (prev[0]?.seq ?? 100_000) + 1,
                    timestamp: new Date().toISOString(),
                    actor: "owner@local",
                    action: "memory_item.forgotten",
                    resource: memoryId,
                    outcome: "allowed",
                    correlationId: `corr-mem-${Date.now().toString().slice(-4)}`,
                    chainHash: "sha256:forgotten-soft-exclude",
                    payloadSummary: `Soft-excluded memory ${memoryId} (restorable)`,
                    payloadJson: JSON.stringify({ memoryId, restorable: true }, null, 2),
                  },
                  ...prev,
                ]);
                pushToast("Memory forgotten (soft-excluded)", memoryId);
              }}
              onRestoreItem={(memoryId) => {
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId ? restoreMemoryItem(m).item : m,
                  ),
                );
                pushToast("Memory restored", memoryId);
              }}
              onPermanentDeleteItem={(memoryId) => {
                const auditId = `aud-del-${Date.now()}`;
                setMemoryItems((prev) =>
                  prev.map((m) =>
                    m.memoryId === memoryId
                      ? permanentlyDeleteMemoryItem(m, auditId).item
                      : m,
                  ),
                );
                setAuditEvents((prev) => [
                  {
                    eventId: auditId,
                    seq: (prev[0]?.seq ?? 100_000) + 1,
                    timestamp: new Date().toISOString(),
                    actor: "owner@local",
                    action: "memory_item.deleted",
                    resource: memoryId,
                    outcome: "allowed",
                    correlationId: `corr-del-${Date.now().toString().slice(-4)}`,
                    chainHash: "sha256:permanent-erasure-tombstone",
                    payloadSummary: `Permanently erased memory ${memoryId} (tombstone retained)`,
                    payloadJson: JSON.stringify({ memoryId, irreversible: true }, null, 2),
                  },
                  ...prev,
                ]);
                pushToast("Memory permanently deleted", `Audit ${auditId}`);
              }}
            />
          ) : activeSurface === "context" ? (
            <ContextInspectorView snapshots={INITIAL_CONTEXT_SNAPSHOTS} />
          ) : (
            <ActivityAuditPage
              events={auditEvents}
              onAppendCorrectionNote={(targetEventId, note) => {
                const corr = createAppendOnlyCorrectionEvent(
                  auditEvents,
                  targetEventId,
                  note,
                );
                setAuditEvents((prev) => [corr, ...prev]);
                pushToast("Correction appended", `${corr.eventId} -> ${targetEventId}`);
              }}
              onExportRedactedAudit={(_json, count) => {
                pushToast("Redacted audit exported", `${count} sanitized event(s)`);
              }}
            />
          )}
        </ApplicationShell>
      )}
    </StartupHandshakeGate>
  );
}
