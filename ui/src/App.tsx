/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry (UIR-0).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */
import type { ReactElement } from "react";
import {
  MATERIAL_ICON_TOTAL_COUNT,
  resolveMaterialIconId,
} from "./components/icons/materialIconManifest";
import {
  LOCKED_BYOK_PROVIDER_PRESETS,
  SETTINGS_SECTIONS,
  SUPPORTED_LOCALES,
  TARGET_PLATFORMS,
  UI_DECISION_REGISTER_IDS,
} from "./types/decisions";

export function App(): ReactElement {
  const sampleGoIcon = resolveMaterialIconId("planstore_test.go");
  const sampleTsxIcon = resolveMaterialIconId("AgenticComposer.tsx");

  return (
    <div className="karyora-shell-root dark" data-testid="karyora-v3-root">
      <main
        style={{
          margin: "auto",
          maxWidth: "720px",
          padding: "var(--space-6)",
          borderRadius: "var(--radius-lg)",
          backgroundColor: "var(--bg-surface-1)",
          border: "1px solid var(--border-default)",
        }}
      >
        <h1 style={{ margin: "0 0 var(--space-2) 0", fontSize: "1.25rem" }}>
          Karyora — Unified AI Work Platform (v3.0 Clean-Slate Baseline)
        </h1>
        <p style={{ margin: "0 0 var(--space-4) 0", color: "var(--text-secondary)" }}>
          Phase 1 Milestone UIR-0: Clean-slate directory architecture,{" "}
          {MATERIAL_ICON_TOTAL_COUNT} Material file/language SVG icons, and{" "}
          {UI_DECISION_REGISTER_IDS.length} locked architectural decisions active across{" "}
          {TARGET_PLATFORMS.join(", ")}.
        </p>
        <ul
          style={{
            margin: 0,
            paddingInlineStart: "var(--space-5)",
            color: "var(--text-muted)",
          }}
        >
          <li>
            Material SVG Icon Dispatcher: <code>{sampleGoIcon}.svg</code> /{" "}
            <code>{sampleTsxIcon}.svg</code> ({MATERIAL_ICON_TOTAL_COUNT} icons verified)
          </li>
          <li>
            Overlay Settings Sections: {SETTINGS_SECTIONS.length} sections · Locked BYOK
            Providers: {LOCKED_BYOK_PROVIDER_PRESETS.length} presets · Locales:{" "}
            {SUPPORTED_LOCALES.length}
          </li>
        </ul>
      </main>
    </div>
  );
}
