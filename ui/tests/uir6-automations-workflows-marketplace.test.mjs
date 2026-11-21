/**
 * Phase 4 Milestone UIR-6 Verification Tests:
 * - UIR-6.1 (Screenshots #66–#67, E2E-11): Automations 4 starter templates, frequencies, & ScheduledTaskModal
 * - UIR-6.2 (Blueprint §10.2, §20.5, E2E-11): Workflow DAG cycle/schema validator, keyboard step reordering, & 409 CAS guard
 * - UIR-6.3 (Screenshots #34–#36, G-12, E2E-12): Plugin Marketplace tabs/categories & WASM pluginhost trust/revocation policy
 */

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const UI_ROOT = path.resolve(__dirname, "..");

test("UIR-6.1 (Screenshots #66–#67, E2E-11): Automations surface exposes exact 4 starter templates and 5 schedule frequencies", async () => {
  const { STARTER_AUTOMATION_TEMPLATES, SCHEDULE_FREQUENCIES } = await import(
    "../src/automations/automationsConfig.ts"
  );

  assert.deepEqual(
    STARTER_AUTOMATION_TEMPLATES.map((t) => t.title),
    [
      "Project progress brief",
      "Code health sweep",
      "Dependency security review",
      "Idle-time doc refresh",
    ],
  );
  assert.deepEqual(
    SCHEDULE_FREQUENCIES.map((f) => f.value),
    ["daily", "weekly", "monthly", "custom", "when-idle"],
  );

  const modalSrc = fs.readFileSync(
    path.join(UI_ROOT, "src/automations/ScheduledTaskModal.tsx"),
    "utf8",
  );
  assert.match(modalSrc, /Settings/);
  assert.match(modalSrc, /History/);
  assert.match(modalSrc, /ConfirmDialog/);
});

test("UIR-6.2 (Blueprint §10.2, §20.5, E2E-11): Workflow DAG validator fails closed on cycles, enforces 409 CAS, and supports keyboard reordering", async () => {
  const {
    INITIAL_WORKFLOWS,
    validateWorkflowDag,
    evaluateWorkflowCasSave,
    reorderWorkflowNodes,
  } = await import("../src/automations/automationsConfig.ts");

  const sample = INITIAL_WORKFLOWS[0];
  const validResult = validateWorkflowDag(sample.nodes, sample.edges);
  assert.equal(validResult.valid, true);
  assert.equal(validResult.topologicalOrder.length, sample.nodes.length);

  // Introducing a cycle MUST fail closed
  const cyclicEdges = [
    ...sample.edges,
    { edgeId: "e-cycle", fromNodeId: "n-receipt", toNodeId: "n-verify" },
  ];
  const cyclicResult = validateWorkflowDag(sample.nodes, cyclicEdges);
  assert.equal(cyclicResult.valid, false);
  assert.ok(cyclicResult.issues.some((i) => i.code === "CYCLE_DETECTED"));

  // Broken edge reference fails closed
  const brokenResult = validateWorkflowDag(sample.nodes, [
    ...sample.edges,
    { edgeId: "e-broken", fromNodeId: "n-verify", toNodeId: "non-existent" },
  ]);
  assert.equal(brokenResult.valid, false);
  assert.ok(brokenResult.issues.some((i) => i.code === "BROKEN_EDGE_REF"));

  // CAS 409 Conflict guard detects stale baseRevision cleanly
  const conflict = evaluateWorkflowCasSave("rev-001", "rev-002", 3);
  assert.equal(conflict.status, "conflict_409");
  assert.match(conflict.conflictMessage ?? "", /409 Conflict/);

  const okSave = evaluateWorkflowCasSave("rev-002", "rev-002", 3);
  assert.equal(okSave.status, "saved");
  assert.equal(okSave.newRevision, "rev-003");

  // Keyboard-accessible step reordering updates order deterministically
  const reordered = reorderWorkflowNodes(sample.nodes, 1, 2);
  assert.equal(reordered[1].nodeId, sample.nodes[2].nodeId);
  assert.equal(reordered[2].nodeId, sample.nodes[1].nodeId);
});

test("UIR-6.3 (Screenshots #34–#36, G-12, E2E-12): Plugin Marketplace tabs/categories and WASM pluginhost trust/revocation policy", async () => {
  const {
    MARKETPLACE_TABS,
    INITIAL_MARKETPLACE_PLUGINS,
    filterMarketplacePlugins,
    evaluatePluginInstallPolicy,
  } = await import("../src/marketplace/marketplaceConfig.ts");

  assert.deepEqual(
    MARKETPLACE_TABS.map((t) => t.id),
    ["public", "community", "personal"],
  );

  // Verify required built-in & catalog plugin cards (#34–#36)
  const names = INITIAL_MARKETPLACE_PLUGINS.map((p) => p.name);
  for (const required of [
    "Documents",
    "PDF",
    "Presentations",
    "Spreadsheets",
    "Dev Team",
    "Compound Engineering",
    "Git Commit",
  ]) {
    assert.ok(names.includes(required), `Missing required plugin card: ${required}`);
  }

  const publicApps = filterMarketplacePlugins(
    INITIAL_MARKETPLACE_PLUGINS,
    "public",
    "App plugins",
    "",
  );
  assert.equal(publicApps.length, 4);

  // Unsigned community plugin requires explicit trust review before install
  const gitCommit = INITIAL_MARKETPLACE_PLUGINS.find((p) => p.pluginId === "plugin-git-commit");
  assert.ok(gitCommit);
  const unsignedVerdict = evaluatePluginInstallPolicy(gitCommit);
  assert.equal(unsignedVerdict.allowed, true);
  assert.equal(unsignedVerdict.requiresExplicitTrustReview, true);

  // Revoked plugin is blocked from installing
  const revokedVerdict = evaluatePluginInstallPolicy({
    ...gitCommit,
    lifecycleState: "revoked",
  });
  assert.equal(revokedVerdict.allowed, false);
  assert.equal(revokedVerdict.blockReason, "PLUGIN_REVOKED_BY_TRUST_POLICY");

  // Incompatible WASM ABI is blocked (G-12)
  const badAbiVerdict = evaluatePluginInstallPolicy({
    ...gitCommit,
    governance: {
      ...gitCommit.governance,
      abiCompatible: false,
      abiVersion: "legacy-v0",
    },
  });
  assert.equal(badAbiVerdict.allowed, false);
  assert.match(badAbiVerdict.blockReason ?? "", /INCOMPATIBLE_WASM_ABI/);

  // Governance drawer enforces Remove vs Revoke trust distinctness with ConfirmDialog
  const drawerSrc = fs.readFileSync(
    path.join(UI_ROOT, "src/marketplace/PluginGovernanceDrawer.tsx"),
    "utf8",
  );
  assert.match(drawerSrc, /Revoke trust/);
  assert.match(drawerSrc, /Remove/);
  assert.match(drawerSrc, /ConfirmDialog/);
});
