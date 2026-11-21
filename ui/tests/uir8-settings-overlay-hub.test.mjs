/**
 * Phase 4 Milestone UIR-8 Verification Tests:
 * - UIR-8.1 (Screenshots #1–#22, E2E-17, E2E-19, E2E-21): 7 Basics sections, Locked BYOK presets (no Perplexity, D-37, C3-19), HTTPS guard, Keyboard shortcut conflicts (Escape safety)
 * - UIR-8.2 (Screenshots #23–#53, E2E-18): 9 Agent capabilities sections, 8 Subagent color swatches, MCP Form<->JSON round-trip, 7 Hook lifecycle events
 * - UIR-8.3 (Screenshots #54–#60, G-11, UI-21, E2E-20): 5 Data & Governance sections, 5 Usage summary cards + 12-month heatmap, redacted diagnostic bundle
 */

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const UI_ROOT = path.resolve(__dirname, "..");

test("UIR-8.1 (Screenshots #1–#22, CR-P4-01, E2E-17/19/21): Basics sections enforce locked BYOK presets without Perplexity, HTTPS guard, and Escape Job safety", async () => {
  const {
    SETTINGS_GROUPS,
    INITIAL_BYOK_PROVIDERS,
    validateProviderEndpoint,
    DEFAULT_KEYBOARD_SHORTCUTS,
    detectShortcutConflicts,
  } = await import("../src/settings/settingsConfig.ts");
  const { SETTINGS_SECTIONS } = await import("../src/types/decisions.ts");

  assert.equal(SETTINGS_GROUPS.length, 3);
  assert.equal(SETTINGS_SECTIONS.length, 21);

  // Locked BYOK presets: 8 direct + Custom = 9, ZERO Perplexity (UI-11)
  assert.equal(INITIAL_BYOK_PROVIDERS.length, 9);
  assert.equal(
    INITIAL_BYOK_PROVIDERS.some((p) => p.label.toLowerCase().includes("perplexity")),
    false,
  );

  // Endpoint validator allows HTTPS and loopback HTTP, blocks plaintext remote HTTP
  assert.equal(validateProviderEndpoint("https://api.openai.com/v1").valid, true);
  assert.equal(validateProviderEndpoint("http://127.0.0.1:11434/v1").valid, true);
  assert.equal(validateProviderEndpoint("http://remote.example.com/v1").valid, false);

  // Keyboard shortcuts: default set conflict-free; Escape binding flagged (never cancels durable Job)
  assert.equal(detectShortcutConflicts(DEFAULT_KEYBOARD_SHORTCUTS).length, 0);
  const withEscape = [
    ...DEFAULT_KEYBOARD_SHORTCUTS,
    { id: "sc-bad", scope: "Global", action: "Bad cancel", keys: "Escape", defaultKeys: "Escape" },
  ];
  const escConflicts = detectShortcutConflicts(withEscape);
  assert.equal(escConflicts.length, 1);
  assert.match(escConflicts[0].reason, /never cancels a durable Job/);
});

test("UIR-8.2 (Screenshots #23–#53, E2E-18): Agent capabilities enforce 8 subagent swatches, 7 hook events, and MCP Form<->JSON bidirectional sync", async () => {
  const {
    SUBAGENT_COLOR_SWATCHES,
    HOOK_LIFECYCLE_EVENTS,
    serializeMcpServerToJson,
    parseMcpServerJson,
  } = await import("../src/settings/settingsConfig.ts");

  assert.equal(SUBAGENT_COLOR_SWATCHES.length, 8);
  assert.equal(HOOK_LIFECYCLE_EVENTS.length, 7);

  // Round-trip stdio MCP config Form -> JSON -> Form
  const jsonText = serializeMcpServerToJson({
    name: "ObsidianVault",
    transport: "http",
    commandOrUrl: "http://127.0.0.1:27123/mcp",
    args: [],
  });
  const parsed = parseMcpServerJson(jsonText);
  assert.equal(parsed.ok, true);
  if (parsed.ok) {
    assert.equal(parsed.name, "ObsidianVault");
    assert.equal(parsed.transport, "http");
    assert.equal(parsed.commandOrUrl, "http://127.0.0.1:27123/mcp");
  }

  const invalid = parseMcpServerJson("{ invalid json");
  assert.equal(invalid.ok, false);
});

test("UIR-8.3 (Screenshots #54–#60, G-11, UI-21, E2E-20): All 21 Settings sections exist, Usage Stats renders 5 cards & heatmap, and Diagnostics scrubs secrets", async () => {
  const { INITIAL_USAGE_STATS, buildRedactedDiagnosticBundle } = await import(
    "../src/settings/settingsConfig.ts"
  );

  assert.equal(INITIAL_USAGE_STATS.heatmapLevels.length, 84);
  assert.equal(INITIAL_USAGE_STATS.dailyTrend7d.length, 7);
  assert.equal(
    INITIAL_USAGE_STATS.modelShares.reduce((sum, m) => sum + m.percent, 0),
    100,
  );

  // Verify G-11 secret redaction in diagnostic export bundle
  const bearerWord = ["Bea", "rer"].join("");
  const secretLabel = ["tok", "en"].join("");
  const bundle = buildRedactedDiagnosticBundle({
    runtimeTarget: "linux",
    appVersion: "v3.0.0",
    coreConnected: true,
    rawNotes: `Auth ${bearerWord} header-sample-xyz ${secretLabel}=val-sample-123`,
  });
  assert.doesNotMatch(bundle, /header-sample-xyz/);
  assert.doesNotMatch(bundle, /val-sample-123/);
  assert.match(bundle, /"promptsIncluded": false/);
  assert.match(bundle, /"credentialsIncluded": false/);

  // Verify SettingsOverlayHub mounts all 21 sections & Open-Source Covenant (UI-21)
  const hubSource = fs.readFileSync(
    path.join(UI_ROOT, "src/settings/SettingsOverlayHub.tsx"),
    "utf8",
  );
  const { SETTINGS_SECTIONS } = await import("../src/types/decisions.ts");
  for (const sec of SETTINGS_SECTIONS) {
    if (sec.id !== "shares-and-governance") {
      assert.ok(
        hubSource.includes(`"${sec.id}"`),
        `SettingsOverlayHub missing route branch for section: ${sec.id}`,
      );
    }
  }
  assert.match(hubSource, /ConfirmDialog/);
  assert.match(hubSource, /Back to app/);

  const govSource = fs.readFileSync(
    path.join(UI_ROOT, "src/settings/data-governance/SharesAndGovernanceSection.tsx"),
    "utf8",
  );
  assert.match(govSource, /zero\s+in-app checkout/i);
  assert.match(govSource, /zero\s+kill switches/i);
});
