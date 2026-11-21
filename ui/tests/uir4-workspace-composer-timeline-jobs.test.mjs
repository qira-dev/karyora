import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const srcRoot = join(uiRoot, "src");

test("UIR-4.1 (Screenshot #68, E2E-03): Greeting, 4 quick chips, 4 + Add Context plugins, and @ / # $ inline trigger detection", async () => {
  const {
    resolveTimeOfDayGreeting,
    STARTER_QUICK_CHIPS,
    ADD_CONTEXT_PLUGINS,
    detectComposerTrigger,
    COMPOSER_SESSION_MODES,
    COMPOSER_THOUGHT_LEVELS,
    cycleValue,
  } = await import("../src/workspace/workspaceConfig.ts");

  assert.equal(resolveTimeOfDayGreeting(9), "Good morning");
  assert.equal(resolveTimeOfDayGreeting(14), "Good afternoon");
  assert.equal(resolveTimeOfDayGreeting(21), "Good evening");

  assert.equal(STARTER_QUICK_CHIPS.length, 4);
  assert.deepEqual(
    STARTER_QUICK_CHIPS.map((c) => c.label),
    [
      "Quick Start",
      "Create a Presentation",
      "Build a Good Website",
      "Scheduled Tasks",
    ],
  );

  assert.equal(ADD_CONTEXT_PLUGINS.length, 4);
  assert.deepEqual(
    ADD_CONTEXT_PLUGINS.map((p) => p.label),
    ["Documents", "PDF", "Presentations", "Spreadsheets"],
  );

  assert.deepEqual(detectComposerTrigger("Check @plan", 11), {
    trigger: "@",
    query: "plan",
    startIndex: 6,
    endIndex: 11,
  });
  assert.deepEqual(detectComposerTrigger("/review", 7), {
    trigger: "/",
    query: "review",
    startIndex: 0,
    endIndex: 7,
  });
  assert.deepEqual(detectComposerTrigger("Link #task-1", 12), {
    trigger: "#",
    query: "task-1",
    startIndex: 5,
    endIndex: 12,
  });
  assert.deepEqual(detectComposerTrigger("Use $sec", 8), {
    trigger: "$",
    query: "sec",
    startIndex: 4,
    endIndex: 8,
  });
  assert.equal(detectComposerTrigger("email@example.com", 17), null);

  assert.equal(cycleValue(COMPOSER_SESSION_MODES, "default"), "plan");
  assert.equal(cycleValue(COMPOSER_THOUGHT_LEVELS, "high"), "low");
});

test("UIR-4.2 (Screenshots #69–#77, #79, #84, E2E-04..06): 6-state tool row machine and timeline components exist and validate transitions", async () => {
  const { isValidToolRowTransition } = await import(
    "../src/workspace/workspaceConfig.ts"
  );

  assert.equal(isValidToolRowTransition("inputStreaming", "pendingApproval"), true);
  assert.equal(isValidToolRowTransition("pendingApproval", "running"), true);
  assert.equal(isValidToolRowTransition("running", "success"), true);
  assert.equal(isValidToolRowTransition("running", "error"), true);
  assert.equal(isValidToolRowTransition("running", "cancelled"), true);
  assert.equal(isValidToolRowTransition("success", "running"), false);
  assert.equal(isValidToolRowTransition("cancelled", "success"), false);

  const requiredTimelineFiles = [
    "workspace/workspace.css",
    "workspace/workspaceConfig.ts",
    "workspace/GreetingEmptyState.tsx",
    "workspace/composer/AddContextMenu.tsx",
    "workspace/composer/TriggerPopovers.tsx",
    "workspace/composer/SubmissionQueueDrawer.tsx",
    "workspace/composer/AgenticComposer.tsx",
    "workspace/timeline/ThinkingRow.tsx",
    "workspace/timeline/ToolExecutionRow.tsx",
    "workspace/timeline/McpToolRow.tsx",
    "workspace/timeline/ApprovalBanner.tsx",
    "workspace/timeline/FloatingProgressCard.tsx",
    "workspace/timeline/ConversationTimeline.tsx",
    "workspace/index.ts",
  ];
  for (const rel of requiredTimelineFiles) {
    assert.equal(
      existsSync(join(srcRoot, rel)),
      true,
      `Required UIR-4 module src/${rel} must exist`,
    );
  }
});

test("UIR-4.3 (Blueprint §12.1, G-13, E2E-13): Durable Jobs (AIJobRelay) filter and O(1) virtualization scale cleanly to 10,000+ rows", async () => {
  const { filterDurableJobs, computeVirtualWindow } = await import(
    "../src/workspace/workspaceConfig.ts"
  );

  const syntheticJobs = Array.from({ length: 10000 }, (_, i) => ({
    jobId: `job-${String(i + 1).padStart(5, "0")}`,
    title: i % 2 === 0 ? "Agent run" : "KnowledgePack ingest",
    jobType: i % 2 === 0 ? "agent-task" : "knowledge-ingest",
    projectName: "karyora_Platform",
    status: i === 5000 ? "reconciling_s4" : i % 3 === 0 ? "running" : "succeeded",
    attemptCount: 1,
    checkpointLabel: `ckpt-${i}`,
    updatedAt: "1m ago",
    idempotencyKey: `idem-${i}`,
    correlationId: `corr-${i}`,
    events: [],
  }));

  const s4Matches = filterDurableJobs(syntheticJobs, "", "reconciling_s4", "all");
  assert.equal(s4Matches.length, 1);
  assert.equal(s4Matches[0].jobId, "job-05001");

  const slice = computeVirtualWindow(10000, 220000, 44, 420, 6);
  assert.equal(slice.totalHeightPx, 440000);
  assert.ok(slice.startIndex >= 4990 && slice.startIndex <= 5000);
  assert.ok(
    slice.endIndex - slice.startIndex <= 24,
    "Virtualized window must render only a bounded DOM slice regardless of 10k+ dataset",
  );

  for (const rel of [
    "domains/jobs/JobsPage.tsx",
    "domains/jobs/JobDetailDrawer.tsx",
    "domains/jobs/JobReceiptModal.tsx",
    "domains/jobs/index.ts",
  ]) {
    assert.equal(
      existsSync(join(srcRoot, rel)),
      true,
      `Required UIR-4.3 Jobs file src/${rel} must exist`,
    );
  }

  const drawerSource = readFileSync(
    join(srcRoot, "domains/jobs/JobDetailDrawer.tsx"),
    "utf8",
  );
  assert.ok(
    drawerSource.includes("Reconciling Job state (S4)") &&
      drawerSource.includes("RecoveryPanel"),
    "JobDetailDrawer must render RecoveryPanel on S4 reconciliation state",
  );
});
