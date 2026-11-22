/**
 * ZCode v3.18.2 Full-Stack Parity Verification Tests:
 * - Embedded Browser (`browserConfig.ts`, `BrowserAndSitesTab.tsx`, `BrowserPermissionsTab.tsx`)
 * - Address normalization & unsafe URL scheme blocking (`javascript:`, `vbscript:`, `data:`, `file:`)
 * - Back/Forward history stack, Recent Visits, Free-Size Responsive Viewport presets & bounds
 * - Per-Origin Site Permissions (`gYn` 11 capabilities) & Reset
 * - Advanced Side-Pane Tools (`Treemapping`, `Whiteboard`, `Model Trajectory`, `Developer Tools`)
 * - Shell Parity (`Toggle browser` TopBar, Resizable handles, Mobile Remote Control, Resource Manager,
 *   YOLO confirmation dialog, Context usage ring, Voice input, `Worked for Xs` disclosure)
 */

import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const UI_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

test("ZCode Embedded Browser: smart URL normalization, protocol allowlisting, history stack & recent visits", async () => {
  const {
    normalizeBrowserAddressInput,
    createBrowserHistory,
    pushBrowserHistory,
    stepBrowserBack,
    stepBrowserForward,
    upsertRecentVisit,
    removeRecentVisit,
  } = await import("../src/side-pane/browserConfig.ts");

  // Smart normalization
  const bare = normalizeBrowserAddressInput("example.com");
  assert.equal(bare.allowed, true);
  if (bare.allowed) {
    assert.equal(bare.normalizedUrl, "https://example.com/");
    assert.equal(bare.domain, "example.com");
  }

  const localDev = normalizeBrowserAddressInput("127.0.0.1:5173/dashboard");
  assert.equal(localDev.allowed, true);
  if (localDev.allowed) {
    assert.equal(localDev.normalizedUrl, "http://127.0.0.1:5173/dashboard");
  }

  // Strict unsafe scheme blocking (G-12)
  for (const unsafe of [
    "javascript:alert(1)",
    "vbscript:msgbox(1)",
    "data:text/html,<script>alert(1)</script>",
    "file:///etc/passwd",
  ]) {
    const res = normalizeBrowserAddressInput(unsafe);
    assert.equal(res.allowed, false, `Unsafe URL "${unsafe}" must be blocked`);
  }

  // History stack navigation
  let hist = createBrowserHistory("https://example.com/");
  hist = pushBrowserHistory(hist, "http://127.0.0.1:5173/");
  hist = pushBrowserHistory(hist, "https://developer.mozilla.org/");
  assert.equal(hist.index, 2);
  hist = stepBrowserBack(hist);
  assert.equal(hist.entries[hist.index], "http://127.0.0.1:5173/");
  hist = stepBrowserForward(hist);
  assert.equal(hist.entries[hist.index], "https://developer.mozilla.org/");

  // Recent visits dedup & remove
  const v1 = upsertRecentVisit([], {
    url: "https://example.com/",
    title: "Example",
    domain: "example.com",
    visitedAt: "now",
  });
  const v2 = upsertRecentVisit(v1, {
    url: "https://example.com/",
    title: "Example Updated",
    domain: "example.com",
    visitedAt: "now",
  });
  assert.equal(v2.length, 1);
  assert.equal(v2[0].title, "Example Updated");
  assert.equal(removeRecentVisit(v2, "https://example.com/").length, 0);
});

test("ZCode Embedded Browser: Free-size Responsive Viewport presets, bounds validation & 11-capability Site Permissions", async () => {
  const {
    BROWSER_VIEWPORT_PRESETS,
    SITE_PERMISSION_CAPABILITIES,
    matchViewportPreset,
    isValidViewportDimension,
    setOriginPermission,
    resetOriginPermissions,
  } = await import("../src/side-pane/browserConfig.ts");

  assert.deepEqual(
    BROWSER_VIEWPORT_PRESETS.map((p) => p.id),
    ["desktop", "tablet", "phone", "slide"],
  );
  assert.equal(matchViewportPreset({ width: 393, height: 852 }), "phone");
  assert.equal(matchViewportPreset({ width: 1000, height: 700 }), "custom");
  assert.equal(isValidViewportDimension("width", 1280), true);
  assert.equal(isValidViewportDimension("width", 50), false);
  assert.equal(isValidViewportDimension("height", 5000), false);

  assert.equal(SITE_PERMISSION_CAPABILITIES.length, 11);

  let perms = setOriginPermission({}, "https://example.com", "notifications", "allow");
  perms = setOriginPermission(perms, "https://example.com", "media", "block");
  assert.equal(perms["https://example.com"]?.notifications, "allow");
  assert.equal(perms["https://example.com"]?.media, "block");
  const cleared = resetOriginPermissions(perms, "https://example.com");
  assert.equal(cleared["https://example.com"], undefined);
});

test("ZCode Full UI Parity: real live browser iframe, advanced side-pane tools, TopBar browser toggle & ZCode modals exist cleanly", () => {
  const browserSource = fs.readFileSync(
    path.join(UI_ROOT, "src/side-pane/BrowserAndSitesTab.tsx"),
    "utf8",
  );
  assert.match(browserSource, /src=\{currentUrl\}/, "BrowserAndSitesTab must load real URLs in iframe src");
  assert.doesNotMatch(browserSource, /dangerouslySetInnerHTML/);
  assert.match(browserSource, /browser-responsive-bar/);
  assert.match(browserSource, /browser-element-picker-bar/);
  assert.match(browserSource, /browser-start-page/);

  const topbarSource = fs.readFileSync(
    path.join(UI_ROOT, "src/shell/WorkspaceTopBar.tsx"),
    "utf8",
  );
  assert.match(topbarSource, /topbar-toggle-browser-btn/);
  assert.match(topbarSource, /topbar-title-more-menu/);

  const composerSource = fs.readFileSync(
    path.join(UI_ROOT, "src/workspace/composer/AgenticComposer.tsx"),
    "utf8",
  );
  assert.match(composerSource, /composer-yolo-confirm-modal/);
  assert.match(composerSource, /composer-context-usage-btn/);
  assert.match(composerSource, /composer-voice-btn/);

  const timelineSource = fs.readFileSync(
    path.join(UI_ROOT, "src/workspace/timeline/ConversationTimeline.tsx"),
    "utf8",
  );
  assert.match(timelineSource, /Worked for /);

  for (const rel of [
    "src/side-pane/BrowserPermissionsTab.tsx",
    "src/side-pane/TreemappingTab.tsx",
    "src/side-pane/WhiteboardTab.tsx",
    "src/side-pane/ModelTrajectoryTab.tsx",
    "src/side-pane/DeveloperToolsTab.tsx",
    "src/shell/MobileRemoteControlModal.tsx",
    "src/shell/ResourceManagerModal.tsx",
  ]) {
    assert.equal(
      fs.existsSync(path.join(UI_ROOT, rel)),
      true,
      `Expected ZCode parity component ${rel} to exist`,
    );
  }
});
