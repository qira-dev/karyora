/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry
 * (Phase 1 UIR-0/UIR-1 & Phase 2 UIR-2/UIR-3 Complete).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */

import { useMemo, useState, type ReactElement } from "react";
import { EmptyStateCard } from "./components/feedback";
import {
  MATERIAL_ICON_TOTAL_COUNT,
  MaterialFileIcon,
} from "./components/icons";
import { Badge, Button, ModalDialog, type ToastMessage } from "./components/primitives";
import {
  createV3CoreClient,
  type TaskSessionSummary,
  type WorkspaceProjectSummary,
} from "./coreclient";
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
  DEFAULT_LOCAL_UI_PREFERENCES,
  EphemeralUiStore,
  LocalPreferencesStore,
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
    title: "Rebuild v3.0 Agentic Frontend Shell",
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

export function App(): ReactElement {
  const coreClient = useMemo(() => createV3CoreClient(), []);
  const platformBridge = useMemo(() => createPlatformBridge(), []);
  const preferencesStore = useMemo(() => new LocalPreferencesStore(), []);
  const overlayStore = useMemo(() => new EphemeralUiStore(), []);

  const [prefs, setPrefs] = useState(DEFAULT_LOCAL_UI_PREFERENCES);
  const [activeSurface, setActiveSurface] = useState<ActiveMainSurface>("workspace");
  const [projects] = useState<readonly WorkspaceProjectSummary[]>(INITIAL_PROJECTS);
  const [tasks, setTasks] = useState<readonly TaskSessionSummary[]>(INITIAL_TASKS);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(
    INITIAL_TASKS[0]?.taskId ?? null,
  );
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
        eventId: "evt-shell-ready",
        category: "core-health",
        title: "Phase 2 Application Shell Ready",
        privacySafeSummary:
          "Left Sidebar, Top Bar, Command Center (Ctrl+K), and 6-step Migration Guide are active.",
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

  const activeTask =
    tasks.find((t) => t.taskId === activeTaskId) ?? null;

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
            pushToast(
              "Add project folder",
              platformBridge.capabilities.nativeFolderPicker
                ? "Native OS folder picker invoked."
                : "Web mode: select or enter a Core-authorized workspace directory.",
            );
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
              pushToast("Task archived", `${task.title} moved to Archive management.`);
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
          <div
            style={{
              maxWidth: "820px",
              margin: "0 auto",
              display: "flex",
              flexDirection: "column",
              gap: "var(--space-4)",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
              <Badge
                tone="success"
                dot
                label={`Phase 2 Shell Active (${platformBridge.runtimeTarget})`}
              />
              <Badge
                tone="accent"
                label={`${MATERIAL_ICON_TOTAL_COUNT} Material SVG Icons`}
              />
            </div>

            <EmptyStateCard
              title={
                activeSurface === "workspace"
                  ? activeTask
                    ? activeTask.title
                    : "Ready for a new task"
                  : `Surface: ${activeSurface}`
              }
              description="Phase 2 Application Shell (Left Sidebar, Top Bar, Command Center Ctrl+K, Notifications, Onboarding & 6-Step Migration Guide) is live. Phase 3 mounts the Multi-State Composer, Live Stream, Side Pane, Previewers, and PTY Terminal into this shell."
              primaryActionLabel="Open First-Run Setup"
              onPrimaryAction={() => setOnboardingMode("onboarding")}
            />

            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                gap: "var(--space-3)",
                padding: "var(--space-3)",
                borderRadius: "var(--radius-md)",
                border: "1px solid var(--border-subtle)",
                backgroundColor: "var(--bg-surface-1)",
              }}
            >
              <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                <MaterialFileIcon filename="planstore_test.go" codeTheme={activeCodeTheme} />
                <code>planstore_test.go</code>
              </span>
              <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                <MaterialFileIcon filename="ApplicationShell.tsx" codeTheme={activeCodeTheme} />
                <code>ApplicationShell.tsx</code>
              </span>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setOnboardingMode("migration")}
              >
                Open 6-Step Migration Guide (#55)
              </Button>
            </div>
          </div>
        </ApplicationShell>
      )}
    </StartupHandshakeGate>
  );
}
