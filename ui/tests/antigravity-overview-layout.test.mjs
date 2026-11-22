/**
 * Antigravity-Style Layout Verification Tests:
 * - Right-Side `Overview` Panel & `⋮ Customize Sections` checklist (`overviewConfig.ts`, `WorkspaceOverviewRail.tsx`)
 * - Clean Antigravity Left Sidebar (zero Core clutter dumped in left nav)
 * - Settings Overlay Hub embedding Karyora Core views (`Jobs`, `Knowledge`, `Context`, `Activity & Audit`, `Automations`)
 */

import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const UI_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

test("Antigravity Overview Panel: Customize Sections checklist, defaults, toggling & JSON persistence", async () => {
  const {
    OVERVIEW_SECTION_DEFINITIONS,
    createDefaultOverviewVisibility,
    toggleOverviewSectionVisibility,
    parseOverviewVisibilityJson,
  } = await import("../src/side-pane/overviewConfig.ts");

  assert.equal(OVERVIEW_SECTION_DEFINITIONS.length, 13);

  const ids = OVERVIEW_SECTION_DEFINITIONS.map((d) => d.id);
  for (const expected of [
    "subagents",
    "background-tasks",
    "goals",
    "documents",
    "artifacts",
    "links",
    "modified-files",
    "uploads",
    "skills-used",
    "knowledge",
    "memory",
    "context",
    "activity",
  ]) {
    assert.ok(ids.includes(expected), `Overview Customize Sections must include "${expected}"`);
  }

  const defaults = createDefaultOverviewVisibility();
  assert.equal(defaults.subagents, true);
  assert.equal(defaults["background-tasks"], true);
  assert.equal(defaults.artifacts, true);
  assert.equal(defaults["modified-files"], true);
  assert.equal(defaults.uploads, true);
  assert.equal(defaults.goals, false);
  assert.equal(defaults.knowledge, false);

  const toggled = toggleOverviewSectionVisibility(defaults, "knowledge");
  assert.equal(toggled.knowledge, true);
  assert.equal(defaults.knowledge, false, "Toggle must be pure/immutable");

  const restored = parseOverviewVisibilityJson(JSON.stringify(toggled));
  assert.equal(restored.knowledge, true);
  assert.equal(restored.subagents, true);

  // Resilient fallback on corrupted storage JSON
  const fallback = parseOverviewVisibilityJson("{bad-json");
  assert.deepEqual(fallback, defaults);
});

test("Antigravity Layout Parity: Clean Left Sidebar, Right Overview Rail & Settings-Embedded Core Views", () => {
  const leftSidebarSource = fs.readFileSync(
    path.join(UI_ROOT, "src/shell/LeftSidebar.tsx"),
    "utf8",
  );
  assert.doesNotMatch(
    leftSidebarSource,
    /onSelectSurface\("jobs"\)/,
    "LeftSidebar must stay clean like Antigravity (Core surfaces live in Settings & Right Overview)",
  );

  const overviewSource = fs.readFileSync(
    path.join(UI_ROOT, "src/side-pane/WorkspaceOverviewRail.tsx"),
    "utf8",
  );
  assert.match(overviewSource, /overview-customize-btn/);
  assert.match(overviewSource, /overview-customize-menu/);
  assert.match(overviewSource, /overview-tool-browser/);
  assert.match(overviewSource, /overview-tool-review/);
  assert.match(overviewSource, /overview-tool-files/);
  assert.match(overviewSource, /overview-tool-terminal/);
  assert.match(overviewSource, /overview-tool-extensions/);

  const settingsHubSource = fs.readFileSync(
    path.join(UI_ROOT, "src/settings/SettingsOverlayHub.tsx"),
    "utf8",
  );
  assert.match(settingsHubSource, /settings-core-nav-/);
  assert.match(settingsHubSource, /coreSurfaceViews/);
});
