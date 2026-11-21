/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry (UIR-0 & UIR-1).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */
import { useMemo, type ReactElement } from "react";
import {
  MATERIAL_ICON_TOTAL_COUNT,
  resolveMaterialIconId,
} from "./components/icons/materialIconManifest";
import { createV3CoreClient } from "./coreclient";
import { createPlatformBridge } from "./platform-bridge";
import {
  LOCKED_BYOK_PROVIDER_PRESETS,
  SETTINGS_SECTIONS,
  SUPPORTED_LOCALES,
  TARGET_PLATFORMS,
  UI_DECISION_REGISTER_IDS,
} from "./types/decisions";
import {
  CasConcurrencyGuard,
  DomainStateCache,
  DraftAndMutationStore,
  EphemeralUiStore,
  LocalPreferencesStore,
  SessionPrewarmStore,
  SubmissionQueueStore,
  WorkspaceSessionStore,
} from "./workspace/state";

export function App(): ReactElement {
  const runtimeFoundation = useMemo(() => {
    const coreClient = createV3CoreClient();
    const platformBridge = createPlatformBridge();
    const domainCache = new DomainStateCache();
    const workspaceSessions = new WorkspaceSessionStore();
    const drafts = new DraftAndMutationStore();
    const preferences = new LocalPreferencesStore();
    const overlays = new EphemeralUiStore();
    const prewarm = new SessionPrewarmStore();
    const submissionQueue = new SubmissionQueueStore();
    const casGuard = new CasConcurrencyGuard();

    return {
      coreClient,
      platformBridge,
      domainCache,
      workspaceSessions,
      drafts,
      preferences,
      overlays,
      prewarm,
      submissionQueue,
      casGuard,
    };
  }, []);

  const sampleGoIcon = resolveMaterialIconId("planstore_test.go");
  const sampleTsxIcon = resolveMaterialIconId("AgenticComposer.tsx");
  const activePrefs = runtimeFoundation.preferences.getPreferences();

  return (
    <div
      className={`karyora-shell-root ${activePrefs.appTheme}`}
      data-testid="karyora-v3-root"
      data-platform={runtimeFoundation.platformBridge.runtimeTarget}
    >
      <main
        style={{
          margin: "auto",
          maxWidth: "760px",
          padding: "var(--space-6)",
          borderRadius: "var(--radius-lg)",
          backgroundColor: "var(--bg-surface-1)",
          border: "1px solid var(--border-default)",
        }}
      >
        <h1 style={{ margin: "0 0 var(--space-2) 0", fontSize: "1.25rem" }}>
          Karyora — Unified AI Work Platform (v3.0 Phase 1 Baseline)
        </h1>
        <p style={{ margin: "0 0 var(--space-4) 0", color: "var(--text-secondary)" }}>
          Phase 1 Complete (UIR-0 &amp; UIR-1): Clean-slate v3.0 architecture,{" "}
          {MATERIAL_ICON_TOTAL_COUNT} Material SVG icons, typed CoreClient + SSE
          cursor-gap recovery, 4-platform PlatformBridge (
          <code>{runtimeFoundation.platformBridge.runtimeTarget}</code>), 5
          state-lifetime stores, draft session prewarming, serial submission queue,
          and CAS concurrency guards active.
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
            Target Platforms: {TARGET_PLATFORMS.join(", ")} · Active Core Connection:{" "}
            <code>{runtimeFoundation.domainCache.getConnectionId()}</code>
          </li>
          <li>
            Overlay Settings Sections: {SETTINGS_SECTIONS.length} · Locked BYOK
            Providers: {LOCKED_BYOK_PROVIDER_PRESETS.length} · Locales:{" "}
            {SUPPORTED_LOCALES.length} · Decisions: {UI_DECISION_REGISTER_IDS.length}
          </li>
        </ul>
      </main>
    </div>
  );
}
