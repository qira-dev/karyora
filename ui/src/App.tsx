/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry
 * (Phase 1 UIR-0/UIR-1, Phase 2 UIR-2/UIR-3 & Phase 3 UIR-4 Active).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */

import { useMemo, useState, type ReactElement } from "react";
import { EmptyStateCard } from "./components/feedback";
import { Button, ModalDialog, type ToastMessage } from "./components/primitives";
import {
  createIdempotencyKey,
  createV3CoreClient,
  type TaskSessionSummary,
  type WorkspaceProjectSummary,
} from "./coreclient";
import { JobsPage } from "./domains/jobs";
import { createPlatformBridge } from "./platform-bridge";
import {
  ApplicationShell,
  StartupHandshakeGate,
  deduplicateNotifications,
  type ActiveMainSurface,
  type CoreNotificationItem,
} from "./shell";
import {
  SETTINGS_SECTIONS,
  type AppTheme,
  type InterfaceMode,
  type SettingsSectionId,
  type SupportedLocale,
} from "./types/decisions";
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
    text: "Inspect planstore_test.go and verify the UIR-4 timeline and MCP tool rows.",
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
  { stepId: "s3", label: "Verify 10k+ Durable Jobs Virtualization", completed: false },
];

const INITIAL_SUBAGENTS: readonly TimelineSubagentBadge[] = [
  {
    subagentId: "sub-1",
    name: "Codebase Researcher",
    status: "ended",
    elapsedLabel: "Ran for 42s",
  },
  {
    subagentId: "sub-2",
    name: "Security Verifier",
    status: "running",
    elapsedLabel: "Running 18s",
  },
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

  // Shell overlay & panel toggles
  const [terminalOpen, setTerminalOpen] = useState(false);
  const [sidePaneOpen, setSidePaneOpen] = useState(false);
  const [commandCenterOpen, setCommandCenterOpen] = useState(false);
  const [notificationDrawerOpen, setNotificationDrawerOpen] = useState(false);
  const [osNotificationsEnabled, setOsNotificationsEnabled] = useState(false);
  const [onboardingMode, setOnboardingMode] = useState<
    "closed" | "onboarding" | "migration"
  >("closed");
  const [settingsOverlayOpen, setSettingsOverlayOpen] = useState(false);
  const [activeSettingsSection, setActiveSettingsSection] =
    useState<SettingsSectionId>("general");
  const [notifications, setNotifications] = useState<
    readonly CoreNotificationItem[]
  >(() =>
    deduplicateNotifications([], [
      {
        eventId: "evt-uir4-ready",
        category: "job",
        title: "Phase 3 Agentic Workspace & Durable Jobs Ready",
        privacySafeSummary:
          "Multi-State Composer, Live Conversation Stream, and AIJobRelay Jobs view are active.",
        occurredAt: "Just now",
        read: false,
        dismissed: false,
        targetSurface: "jobs",
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
  };

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
          terminalOpen={terminalOpen}
          sidePaneOpen={sidePaneOpen}
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
          toasts={toasts}
          canWrite={true}
          onToggleSidebar={() =>
            updatePref("sidebarCollapsed", !prefs.sidebarCollapsed)
          }
          onToggleTerminal={() => setTerminalOpen((prev) => !prev)}
          onToggleSidePane={() => setSidePaneOpen((prev) => !prev)}
          onNewTask={() => {
            setActiveSurface("workspace");
            setActiveTaskId(null);
          }}
          onSelectSurface={setActiveSurface}
          onSelectTask={(task) => {
            setActiveSurface("workspace");
            setActiveTaskId(task.taskId);
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
          settingsOverlayContent={
            <ModalDialog
              isOpen={settingsOverlayOpen}
              title={`Settings — ${
                SETTINGS_SECTIONS.find((s) => s.id === activeSettingsSection)
                  ?.label ?? "General"
              }`}
              description="Full 21-section Overlay Settings Hub mounts in Phase 4 (UIR-8); workspace stays mounted and inert beneath this overlay."
              onClose={closeSettingsOverlay}
            >
              <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-1)" }}>
                {SETTINGS_SECTIONS.map((sec) => (
                  <Button
                    key={sec.id}
                    variant={
                      sec.id === activeSettingsSection ? "primary" : "secondary"
                    }
                    size="sm"
                    onClick={() => setActiveSettingsSection(sec.id)}
                  >
                    {sec.label}
                  </Button>
                ))}
              </div>
            </ModalDialog>
          }
        >
          {activeSurface === "workspace" ? (
            <div className="ky-workspace-stage">
              {activeTask === null ? (
                <GreetingEmptyState
                  onSelectQuickChip={(chip) => setPromptText(chip.promptTemplate)}
                />
              ) : (
                <ConversationTimeline
                  turns={turns}
                  progressSteps={INITIAL_PROGRESS_STEPS}
                  subagents={INITIAL_SUBAGENTS}
                  codeTheme={activeCodeTheme}
                  onInspectFile={(filePath) => {
                    setSidePaneOpen(true);
                    pushToast("Inspecting file in Side Pane", filePath);
                  }}
                  onApprovalDecision={(approvalId, decision) => {
                    pushToast("Approval recorded", `${approvalId}: ${decision}`);
                  }}
                  onOpenSubagentsSideTab={() => {
                    setSidePaneOpen(true);
                    pushToast("Side Pane", "Opened Subagents directory");
                  }}
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
          ) : (
            <div style={{ maxWidth: "820px", margin: "0 auto" }}>
              <EmptyStateCard
                title={`Surface: ${activeSurface}`}
                description="This enterprise surface mounts in its assigned Roadmap milestone (UIR-6 / UIR-7)."
                primaryActionLabel="Return to Agentic Workspace"
                onPrimaryAction={() => setActiveSurface("workspace")}
              />
            </div>
          )}
        </ApplicationShell>
      )}
    </StartupHandshakeGate>
  );
}
