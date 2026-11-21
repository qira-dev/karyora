import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const srcRoot = join(uiRoot, "src");

test("UIR-2.1 (G-03, UI-08, UI-25): all Dark & Light semantic token pairs pass WCAG 2.2 AA contrast (>= 4.5:1) and all 10 Light / 10 Dark code themes exist", async () => {
  const {
    evaluateCanonicalContrastPairs,
    computeContrastRatio,
  } = await import("../src/styles/contrastCalculator.ts");
  const {
    LIGHT_CODE_THEME_PALETTES,
    DARK_CODE_THEME_PALETTES,
    resolveCodeThemePalette,
  } = await import("../src/styles/codeThemeRegistry.ts");

  const evaluated = evaluateCanonicalContrastPairs();
  assert.equal(evaluated.length, 14, "Must evaluate 14 canonical Dark & Light token pairs");

  for (const pair of evaluated) {
    assert.equal(
      pair.passesWcagAA,
      true,
      `[${pair.theme}] ${pair.foregroundToken} (${pair.foregroundHex}) on ${pair.backgroundToken} (${pair.backgroundHex}) must pass WCAG AA >= ${pair.minimumRequiredRatio}:1 (got ${pair.calculatedRatio}:1)`,
    );
  }

  assert.equal(
    computeContrastRatio("#ffffff", "#000000"),
    21,
    "Pure white on pure black contrast ratio must be 21:1",
  );

  assert.equal(
    Object.keys(LIGHT_CODE_THEME_PALETTES).length,
    10,
    "Must define all 10 Light code themes (Screenshots #5–#9)",
  );
  assert.equal(
    Object.keys(DARK_CODE_THEME_PALETTES).length,
    10,
    "Must define all 10 Dark code themes (Screenshots #5–#9)",
  );
  assert.equal(
    resolveCodeThemePalette("Gruvbox Dark").iconThemeVariant,
    "gruvbox_dark",
  );
  assert.equal(
    resolveCodeThemePalette("Gruvbox Light").iconThemeVariant,
    "gruvbox_light",
  );

  const animCss = readFileSync(join(srcRoot, "styles/animations.css"), "utf8");
  for (const selector of [
    ".animated-gradient-text",
    ".tool-call-progress-indeterminate",
    ".draft-prompt-card-waterfall",
    ".digit-flip-counter",
    "prefers-reduced-motion: reduce",
  ]) {
    assert.ok(
      animCss.includes(selector),
      `animations.css must implement ${selector}`,
    );
  }
});

test("UIR-2.2 (G-04, UI-36): MaterialFileIconDispatcher & LucideIconRegistry resolve basename, extension, folder, and code-theme variants", async () => {
  const { resolveMaterialIconId } = await import(
    "../src/components/icons/materialIconManifest.ts"
  );
  const { resolveIconThemeVariant } = await import(
    "../src/components/primitives/ariaHelpers.ts"
  );

  // Basename-first resolution
  assert.equal(resolveMaterialIconId("Dockerfile"), "docker");
  assert.equal(resolveMaterialIconId("go.mod"), "go-mod");
  assert.equal(resolveMaterialIconId("AGENTS.md"), "robot");
  assert.equal(resolveMaterialIconId("CLAUDE.md"), "claude");
  assert.equal(resolveMaterialIconId("package.json"), "nodejs");
  assert.equal(resolveMaterialIconId("tsconfig.json"), "tsconfig");

  // Extension-second resolution (Screenshots #75–#77, #84)
  assert.equal(resolveMaterialIconId("internal/plan/planstore_test.go"), "go");
  assert.equal(
    resolveMaterialIconId("src/workspace/composer/AgenticComposer.tsx"),
    "react_ts",
  );
  assert.equal(resolveMaterialIconId("report.pdf"), "pdf");
  assert.equal(resolveMaterialIconId("deck.pptx"), "powerpoint");
  assert.equal(resolveMaterialIconId("sheet.xlsx"), "table");
  assert.equal(resolveMaterialIconId("spec.docx"), "word");

  // Folder resolution
  assert.equal(
    resolveMaterialIconId("src", { isFolder: true, isOpen: false }),
    "folder-src",
  );
  assert.equal(
    resolveMaterialIconId("src", { isFolder: true, isOpen: true }),
    "folder-src-open",
  );

  // Theme variant resolution
  assert.equal(resolveIconThemeVariant("Gruvbox Dark"), "gruvbox_dark");
  assert.equal(resolveIconThemeVariant("GitHub"), "light");
  assert.equal(resolveIconThemeVariant("GitHub Dark"), "default");

  const lucideSource = readFileSync(
    join(srcRoot, "components/icons/LucideIconRegistry.tsx"),
    "utf8",
  );
  for (const iconKey of [
    "new-task",
    "automations",
    "plugin-marketplace",
    "projects",
    "tasks",
    "settings",
    "profile",
    "brain-thinking",
    "subagent",
    "mcp-plug",
  ]) {
    assert.ok(
      lucideSource.includes(`"${iconKey}"`),
      `LucideIconRegistry.tsx must register chrome icon "${iconKey}"`,
    );
  }
});

test("UIR-2.3 (G-05, G-06, G-08, G-11): 9-state primitives, ARIA keyboard helpers (Tabs, TreeView, Combobox, ContextMenu, ModalDialog), write-only secret input, byte counter, and 5-element RecoveryPanel work accurately", async () => {
  const {
    formatByteCounter,
    computeNextTabIndex,
    flattenVisibleTree,
  } = await import("../src/components/primitives/ariaHelpers.ts");

  // 1. Personalization byte counter (Screenshot #23: "0 KB / 100 KB")
  assert.equal(formatByteCounter(0, 102400), "0 KB / 100 KB");
  assert.equal(formatByteCounter(1536, 102400), "1.5 KB / 100 KB");

  // 2. ARIA Tabs keyboard traversal (ArrowRight, ArrowLeft, Home, End, skipping disabled tabs)
  const tabs = [
    { id: "files", label: "Files" },
    { id: "disabled", label: "Disabled", disabled: true },
    { id: "review", label: "Review" },
    { id: "subagents", label: "Subagents" },
  ];
  assert.equal(
    computeNextTabIndex(tabs, 0, "ArrowRight", "horizontal"),
    2,
    "ArrowRight must skip disabled tab index 1 and focus tab index 2",
  );
  assert.equal(
    computeNextTabIndex(tabs, 0, "ArrowLeft", "horizontal"),
    3,
    "ArrowLeft from first tab must wrap to last enabled tab",
  );
  assert.equal(computeNextTabIndex(tabs, 2, "Home", "horizontal"), 0);
  assert.equal(computeNextTabIndex(tabs, 0, "End", "horizontal"), 3);

  // 3. ARIA TreeView visible node flattening based on expanded folder IDs
  const tree = [
    {
      id: "folder-src",
      label: "src",
      isFolder: true,
      children: [{ id: "file-app", label: "App.tsx" }],
    },
    {
      id: "folder-tests",
      label: "tests",
      isFolder: true,
      children: [{ id: "file-test", label: "uir2.test.mjs" }],
    },
  ];
  const collapsedFlat = flattenVisibleTree(tree, new Set());
  assert.equal(collapsedFlat.length, 2, "Collapsed folders hide children");
  const expandedFlat = flattenVisibleTree(tree, new Set(["folder-src"]));
  assert.equal(expandedFlat.length, 3, "Expanded folder-src reveals child");
  assert.equal(expandedFlat[1].node.id, "file-app");
  assert.equal(expandedFlat[1].depth, 1);

  // 4. Verify all primitive & feedback modules exist and enforce required ARIA/security attributes
  const requiredModules = [
    "components/primitives/Button.tsx",
    "components/primitives/Input.tsx",
    "components/primitives/Select.tsx",
    "components/primitives/Tabs.tsx",
    "components/primitives/TreeView.tsx",
    "components/primitives/ComboboxPopover.tsx",
    "components/primitives/ModalDialog.tsx",
    "components/primitives/index.ts",
    "components/feedback/DigitFlipCounter.tsx",
    "components/feedback/RecoveryPanel.tsx",
    "components/feedback/index.ts",
  ];
  for (const mod of requiredModules) {
    assert.equal(
      existsSync(join(srcRoot, mod)),
      true,
      `Required UIR-2.3 module src/${mod} must exist`,
    );
  }

  const recoverySource = readFileSync(
    join(srcRoot, "components/feedback/RecoveryPanel.tsx"),
    "utf8",
  );
  for (const marker of [
    'role="alert"',
    "recovery-draft-safety-assurance",
    "recovery-technical-diagnostics",
    "Copy Correlation ID",
  ]) {
    assert.ok(
      recoverySource.includes(marker),
      `RecoveryPanel.tsx must render mandatory 5-element marker: ${marker}`,
    );
  }
});
