/**
 * Phase 3 Milestone UIR-5 Verification Tests:
 * - UIR-5.1: Multi-Tab Side Pane, New Tab Launcher (#74/#82), Full-View Mode & Floating Bar (#83)
 * - UIR-5.2: 50,000+ Virtualized File Tree (#78, G-13) & Off-Main-Thread Unified/Split Word Diffs
 * - UIR-5.3: Subagents (#71–#73), Browser/Sites Sandbox, & Isolated Side Conversation
 * - UIR-5.4: Lazy-Loaded Sandboxed PDF/DOCX/XLSX/PPTX Previewers & Zero-Script Gate (G-12)
 * - UIR-5.5: Multi-Tab Docked PTY Terminal (#81, UI-40) & Honest Web-Mode Policy Fallback
 */

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const UI_ROOT = path.resolve(__dirname, "..");

test("UIR-5.1 (Screenshots #74, #82, #83, E2E-07/08): New tab launcher, tab lifecycle, and Full-View floating working bar", async () => {
  const {
    NEW_TAB_TOOLS,
    NEW_TAB_APP_PLUGINS,
    openOrFocusSideTab,
    closeSideTab,
    formatWorkingDuration,
  } = await import("../src/side-pane/sidePaneConfig.ts");

  assert.equal(NEW_TAB_TOOLS.length, 8);
  assert.deepEqual(
    NEW_TAB_TOOLS.map((t) => t.label),
    [
      "Files",
      "Browser",
      "Terminal",
      "Review",
      "Sites",
      "Side conversation",
      "Subagents",
      "Context Inspector",
    ],
  );
  assert.deepEqual(
    NEW_TAB_APP_PLUGINS.map((p) => p.label),
    ["Documents", "PDF", "Presentations", "Spreadsheets"],
  );

  // Tab open/focus/close determinism
  const opened1 = openOrFocusSideTab([], {
    tabId: "tab-files",
    kind: "files",
    title: "Files",
  });
  const opened2 = openOrFocusSideTab(opened1.tabs, {
    tabId: "tab-review",
    kind: "review",
    title: "Review",
  });
  const refocused = openOrFocusSideTab(opened2.tabs, {
    tabId: "tab-files",
    kind: "files",
    title: "Files",
  });
  assert.equal(refocused.tabs.length, 2);
  assert.equal(refocused.activeTabId, "tab-files");

  const closed = closeSideTab(refocused.tabs, refocused.activeTabId, "tab-files");
  assert.equal(closed.tabs.length, 1);
  assert.equal(closed.activeTabId, "tab-review");

  // Screenshot #83 duration formatting (`26m 37s`)
  assert.equal(formatWorkingDuration(26 * 60 + 37), "26m 37s");
  assert.equal(formatWorkingDuration(3661), "1h 1m 1s");

  // Verify Full-View Ctrl+Shift+F & floating working bar exist in source
  const shellSource = fs.readFileSync(
    path.join(UI_ROOT, "src/shell/ApplicationShell.tsx"),
    "utf8",
  );
  assert.match(shellSource, /e\.shiftKey && lower === "f"/);
  assert.match(shellSource, /data-full-view/);
  assert.ok(
    fs.existsSync(path.join(UI_ROOT, "src/side-pane/FloatingMinimizedWorkingBar.tsx")),
  );
});

test("UIR-5.2 (Screenshot #78, G-13, E2E-07): 50,000+ file tree virtualization and word-level unified/split diffs", async () => {
  const { filterVisibleFileNodes, sliceVirtualRows } = await import(
    "../src/side-pane/sidePaneConfig.ts"
  );
  const { computeLineAndWordDiff } = await import(
    "../src/previewers/previewerSanitizer.ts"
  );

  // Construct 50,000 workspace entries and benchmark O(N) filter + O(1) viewport slice
  const nodes = [];
  for (let d = 0; d < 100; d += 1) {
    const dirPath = `pkg-${d}`;
    nodes.push({
      path: dirPath,
      name: dirPath,
      isDirectory: true,
      depth: 0,
      parentPath: null,
    });
    for (let f = 0; f < 500; f += 1) {
      nodes.push({
        path: `${dirPath}/module_${f}.ts`,
        name: `module_${f}.ts`,
        isDirectory: false,
        depth: 1,
        parentPath: dirPath,
        gitStatus: f === 0 ? "M" : null,
      });
    }
  }
  assert.equal(nodes.length, 50_100);

  const expanded = new Set(["pkg-0", "pkg-1"]);
  const t0 = performance.now();
  const visible = filterVisibleFileNodes(nodes, expanded, "");
  const windowSlice = sliceVirtualRows(visible, 280, 420, 28, 6);
  const searchFiltered = filterVisibleFileNodes(nodes, expanded, "pkg-99/module_499");
  const elapsedMs = performance.now() - t0;

  assert.equal(visible.length, 100 + 1000);
  assert.ok(windowSlice.visibleItems.length <= 30);
  assert.equal(searchFiltered.length, 1);
  assert.equal(searchFiltered[0].path, "pkg-99/module_499.ts");
  assert.ok(
    elapsedMs < 250,
    `50k file tree filter+virtualize took ${elapsedMs.toFixed(1)}ms (expected < 250ms)`,
  );

  // Verify word-level diff calculation & Web Worker file presence
  const diff = computeLineAndWordDiff(
    "internal/plan/planstore_test.go",
    "const status = 'draft';\nreturn true;",
    "const status = 'verified';\nreturn true;\n// checkpoint",
  );
  assert.equal(diff.additions, 2);
  assert.equal(diff.deletions, 1);
  const removedLine = diff.lines.find((l) => l.kind === "removed");
  const addedLine = diff.lines.find((l) => l.kind === "added");
  assert.ok(removedLine?.wordSegments?.some((s) => s.kind === "removed"));
  assert.ok(addedLine?.wordSegments?.some((s) => s.kind === "added"));
  assert.ok(fs.existsSync(path.join(UI_ROOT, "src/previewers/diffs.worker.ts")));
});

test("UIR-5.3 & UIR-5.4 (Screenshots #71–#73, UI-39, G-12, E2E-09): Lazy sandboxed PDF/DOCX/XLSX/PPTX previewers enforce zero script execution", async () => {
  const {
    detectDocumentPreviewKind,
    sanitizeDocumentPayload,
    assertZeroScriptExecution,
    sanitizeSpreadsheetCellDisplay,
  } = await import("../src/previewers/previewerSanitizer.ts");

  assert.equal(detectDocumentPreviewKind("report.PDF"), "pdf");
  assert.equal(detectDocumentPreviewKind("spec.docx"), "docx");
  assert.equal(detectDocumentPreviewKind("metrics.xlsx"), "xlsx");
  assert.equal(detectDocumentPreviewKind("deck.pptx"), "pptx");
  assert.equal(detectDocumentPreviewKind("main.go"), "code");

  const hostilePayload =
    '<script>alert("xss")</script><iframe src="javascript:alert(1)"></iframe><div onload="steal()">AutoOpen()</div>';
  const cleaned = sanitizeDocumentPayload(hostilePayload);
  assert.equal(cleaned.hadExecutableRisk, true);
  assert.ok(cleaned.strippedPatternsCount >= 4);
  assert.equal(assertZeroScriptExecution(cleaned.sanitizedText), true);
  assert.doesNotMatch(cleaned.sanitizedText, /<script|onload=|javascript:|<iframe|AutoOpen/i);

  const formulaCell = sanitizeSpreadsheetCellDisplay(
    '=HYPERLINK("http://evil.invalid","Click")',
  );
  assert.equal(formulaCell.formulaBlocked, true);
  assert.match(formulaCell.displayValue, /\[formula-inert\]/);

  // Verify lazy dynamic import() code splitting in SandboxedDocumentPreviewHost.tsx
  const hostSource = fs.readFileSync(
    path.join(UI_ROOT, "src/previewers/SandboxedDocumentPreviewHost.tsx"),
    "utf8",
  );
  assert.match(hostSource, /lazy\(\(\) => import\("\.\/PdfPreviewer"\)\)/);
  assert.match(hostSource, /lazy\(\(\) => import\("\.\/DocxPreviewer"\)\)/);
  assert.match(hostSource, /lazy\(\(\) => import\("\.\/XlsxPreviewer"\)\)/);
  assert.match(hostSource, /lazy\(\(\) => import\("\.\/PptxPreviewer"\)\)/);

  // Verify Subagents, Browser/Sites strict sandbox, and Side Conversation exist
  for (const rel of [
    "src/side-pane/SubagentsTab.tsx",
    "src/side-pane/BrowserAndSitesTab.tsx",
    "src/side-pane/SideConversationTab.tsx",
    "src/previewers/PdfPreviewer.tsx",
    "src/previewers/DocxPreviewer.tsx",
    "src/previewers/XlsxPreviewer.tsx",
    "src/previewers/PptxPreviewer.tsx",
  ]) {
    const content = fs.readFileSync(path.join(UI_ROOT, rel), "utf8");
    assert.doesNotMatch(content, /dangerouslySetInnerHTML/);
  }
});

test("UIR-5.5 (Screenshot #81, UI-40, E2E-10): Docked PTY Terminal multi-tab sessions and honest Web-mode policy fallback", async () => {
  const {
    evaluateTerminalPolicy,
    createTerminalTabSession,
    formatPowerlinePrompt,
  } = await import("../src/terminal/terminalConfig.ts");

  // Desktop platforms allow local PTY directly
  for (const desktopTarget of ["linux", "macos", "windows"]) {
    const pol = evaluateTerminalPolicy(desktopTarget, false);
    assert.equal(pol.ptyAllowed, true);
    assert.equal(pol.reasonCode, "DESKTOP_LOCAL_PTY");
    assert.equal(pol.policyBannerText, null);
  }

  // Web mode with Core remote shell disabled fails closed honestly
  const webDisabled = evaluateTerminalPolicy("web", false);
  assert.equal(webDisabled.ptyAllowed, false);
  assert.equal(webDisabled.reasonCode, "WEB_REMOTE_SHELL_DISABLED");
  assert.ok(webDisabled.policyBannerText && webDisabled.policyBannerText.length > 20);

  // Web mode with Core PTY explicitly enabled allows PTY relay
  const webEnabled = evaluateTerminalPolicy("web", true);
  assert.equal(webEnabled.ptyAllowed, true);
  assert.equal(webEnabled.reasonCode, "WEB_CORE_PTY_ENABLED");

  const session = createTerminalTabSession(1, "zsh", "~/karyora_Platform/karyora", "main");
  assert.equal(session.title, "Terminal zsh");
  assert.equal(
    formatPowerlinePrompt(session),
    "karyora ~/karyora_Platform/karyora (main) ❯",
  );
  assert.ok(
    fs.existsSync(path.join(UI_ROOT, "src/terminal/DockedTerminalPanel.tsx")),
  );
});
