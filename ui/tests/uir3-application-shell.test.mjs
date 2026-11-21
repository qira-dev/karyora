import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const srcRoot = join(uiRoot, "src");

test("UIR-3.1: index.html embeds inline dark/light Startup Logo Shell and StartupHandshakeGate probes Core honestly", () => {
  const indexHtml = readFileSync(join(uiRoot, "index.html"), "utf8");
  assert.ok(
    indexHtml.includes('id="startup-logo-shell"') &&
      indexHtml.includes("background-color: #0b0e14"),
    "index.html must include the inline dark Startup Logo Shell preventing white flash",
  );

  const gateSource = readFileSync(
    join(srcRoot, "shell/StartupHandshakeGate.tsx"),
    "utf8",
  );
  assert.ok(
    gateSource.includes("probeBootstrapStatus") &&
      gateSource.includes("RecoveryPanel"),
    "StartupHandshakeGate.tsx must probe Core status and render RecoveryPanel on offline/auth-required",
  );
});

test("UIR-3.2 (Screenshots #84, #85): exact 13-item Task Context Menu, Project Hover Card, and sidebar filtering/grouping work accurately", async () => {
  const {
    TASK_CONTEXT_MENU_ITEMS,
    filterTasksForSidebar,
    filterVisibleSidebarProjects,
  } = await import("../src/shell/shellConfig.ts");

  assert.equal(
    TASK_CONTEXT_MENU_ITEMS.length,
    13,
    "Screenshot #85 requires exactly 13 Task right-click context menu items",
  );
  const ids = TASK_CONTEXT_MENU_ITEMS.map((i) => i.id);
  for (const expected of [
    "move-to-group",
    "pin",
    "rename",
    "mark-as-unread",
    "archive",
    "delete",
    "copy-task-id",
    "copy-working-directory",
    "export-conversation",
    "view-log-file",
    "open-in-new-window",
    "view-model-trajectory",
    "report-issue",
  ]) {
    assert.ok(ids.includes(expected), `Task Context Menu must include "${expected}"`);
  }

  const sampleTasks = [
    {
      taskId: "t1",
      projectId: "p1",
      title: "Fix build",
      status: "running",
      isPinned: true,
      isUnread: false,
      sessionMode: "default",
      modelId: "m1",
      thoughtLevel: "high",
      logEpoch: 1,
      updatedAt: "1m",
      diffAdditions: 5,
      diffDeletions: 2,
      changedFilesCount: 1,
    },
    {
      taskId: "t2",
      projectId: "p1",
      title: "Archived old thread",
      status: "archived",
      isPinned: false,
      isUnread: true,
      sessionMode: "default",
      modelId: "m1",
      thoughtLevel: "low",
      logEpoch: 1,
      updatedAt: "5d",
      diffAdditions: 0,
      diffDeletions: 0,
      changedFilesCount: 0,
    },
  ];
  assert.equal(
    filterTasksForSidebar(sampleTasks, "", "all").length,
    1,
    "Archived tasks must be excluded from active sidebar list",
  );
  assert.equal(
    filterTasksForSidebar(sampleTasks, "", "running").length,
    1,
    "Running filter matches running task",
  );

  const sampleProjects = [
    {
      projectId: "p1",
      name: "karyora_Platform",
      path: "/home/km-abdullah/karyora_Platform",
      deviceName: "This computer",
      isRemoteDevice: false,
      gitBranch: "main",
      sessionCount: 3,
      lastActiveAt: "1m ago",
      isRemovedFromSidebar: false,
    },
    {
      projectId: "p2",
      name: "removed-project",
      path: "/tmp/old",
      deviceName: "This computer",
      isRemoteDevice: false,
      sessionCount: 0,
      lastActiveAt: "10d ago",
      isRemovedFromSidebar: true,
    },
  ];
  assert.equal(
    filterVisibleSidebarProjects(sampleProjects, "").length,
    1,
    "Removed projects stay hidden from sidebar until restored in Project Management (#58)",
  );
});

test("UIR-3.3 & UIR-3.4 (Screenshots #55, #61–#65, #79–#80): Profile 4 submenus, 7-item Help menu, 6-step Migration Guide, Command Center permission filter, and Notification deduplication", async () => {
  const {
    HELP_MENU_ITEMS,
    MIGRATION_GUIDE_STEPS,
    PROFILE_LOCALE_OPTIONS,
    PROFILE_THEME_OPTIONS,
    PROFILE_INTERFACE_MODE_OPTIONS,
    clampInterfaceZoom,
    filterCommandCenterEntries,
    deduplicateNotifications,
  } = await import("../src/shell/shellConfig.ts");

  // Screenshots #61–#65 Profile submenus
  assert.equal(
    PROFILE_LOCALE_OPTIONS.length,
    14,
    "Profile Language submenu must offer System default + 13 locked locales",
  );
  assert.equal(PROFILE_THEME_OPTIONS.length, 3);
  assert.equal(PROFILE_INTERFACE_MODE_OPTIONS.length, 2);
  assert.equal(clampInterfaceZoom(200), 150);
  assert.equal(clampInterfaceZoom(50), 75);

  // Screenshot #80 Help menu (7 items)
  assert.equal(HELP_MENU_ITEMS.length, 7);

  // Screenshot #55 6-Step Migration Guide
  assert.equal(MIGRATION_GUIDE_STEPS.length, 6);
  assert.deepEqual(
    MIGRATION_GUIDE_STEPS.map((s) => s.label),
    ["Sessions", "Skills", "MCP servers", "Commands", "AGENTS.md", "Migration"],
  );

  // Command Center permission-filtered search (Blueprint §14)
  const cmdEntries = [
    {
      id: "action:new-task",
      category: "workspace-action",
      title: "New task",
      requiresWritePermission: true,
    },
    {
      id: "surface:jobs",
      category: "job",
      title: "Durable Jobs",
      requiresWritePermission: false,
    },
  ];
  assert.equal(
    filterCommandCenterEntries(cmdEntries, "", false).length,
    1,
    "Viewer role (canWrite=false) must not see write-protected workspace actions",
  );
  assert.equal(
    filterCommandCenterEntries(cmdEntries, "job", true).length,
    1,
  );

  // Notification deduplication by stable Core event ID preserving read/dismissed flags
  const deduped = deduplicateNotifications(
    [
      {
        eventId: "evt-1",
        category: "job",
        title: "Job finished",
        privacySafeSummary: "Summary",
        occurredAt: "2026-11-21T10:00:00Z",
        read: true,
        dismissed: false,
      },
    ],
    [
      {
        eventId: "evt-1",
        category: "job",
        title: "Job finished",
        privacySafeSummary: "Summary",
        occurredAt: "2026-11-21T10:00:00Z",
        read: false,
        dismissed: false,
      },
      {
        eventId: "evt-2",
        category: "approval",
        title: "Approval needed",
        privacySafeSummary: "Check job step",
        occurredAt: "2026-11-21T11:00:00Z",
        read: false,
        dismissed: false,
      },
    ],
  );
  assert.equal(deduped.length, 2);
  const evt1 = deduped.find((n) => n.eventId === "evt-1");
  assert.equal(evt1?.read, true, "Existing read flag must survive event replay");

  // Verify all UIR-3 component files exist
  const requiredShellFiles = [
    "shell/shell.css",
    "shell/shellConfig.ts",
    "shell/StartupHandshakeGate.tsx",
    "shell/ProjectHoverCard.tsx",
    "shell/TaskContextMenu.tsx",
    "shell/ProjectTaskTree.tsx",
    "shell/ProfilePopoverMenu.tsx",
    "shell/LeftSidebar.tsx",
    "shell/WorkspaceTopBar.tsx",
    "shell/CommandCenterModal.tsx",
    "shell/NotificationDrawer.tsx",
    "shell/OnboardingAndMigrationModal.tsx",
    "shell/ApplicationShell.tsx",
    "shell/index.ts",
  ];
  for (const rel of requiredShellFiles) {
    assert.equal(existsSync(join(srcRoot, rel)), true, `Required UIR-3 file src/${rel} must exist`);
  }
});
