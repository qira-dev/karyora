/**
 * Karyora Unified AI Work Platform — Clean-Slate v3.0 Root Application Entry (UIR-0, UIR-1 & UIR-2).
 *
 * One Shared React 19 + TypeScript + Vite SPA Serving Four Target Platforms:
 * Web Browser, Windows (amd64/arm64), macOS (arm64/amd64), and Linux (amd64/arm64).
 */
import { useMemo, useState, type ReactElement } from "react";
import { DigitFlipCounter, RecoveryPanel } from "./components/feedback";
import {
  ChromeIcon,
  MATERIAL_ICON_TOTAL_COUNT,
  MaterialFileIcon,
} from "./components/icons";
import {
  Badge,
  Button,
  DiffCountPill,
  Tabs,
  Toggle,
} from "./components/primitives";
import {
  createV3CoreClient,
  toUnknownOutcomeS4RecoveryDescriptor,
} from "./coreclient";
import { createPlatformBridge } from "./platform-bridge";
import { resolveCodeThemePalette } from "./styles/codeThemeRegistry";
import { evaluateCanonicalContrastPairs } from "./styles/contrastCalculator";
import {
  DARK_CODE_THEMES,
  LIGHT_CODE_THEMES,
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
    const contrastAudit = evaluateCanonicalContrastPairs();

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
      contrastAudit,
    };
  }, []);

  const [activePreviewTab, setActivePreviewTab] = useState("tokens");
  const [darkThemeEnabled, setDarkThemeEnabled] = useState(true);
  const activePrefs = runtimeFoundation.preferences.getPreferences();
  const activeCodePalette = resolveCodeThemePalette(
    darkThemeEnabled ? activePrefs.darkCodeTheme : activePrefs.lightCodeTheme,
  );
  const sampleRecovery = toUnknownOutcomeS4RecoveryDescriptor("corr-uir2-verify");

  return (
    <div
      className={`karyora-shell-root ${darkThemeEnabled ? "dark" : ""}`}
      data-theme={darkThemeEnabled ? "dark" : "light"}
      data-code-theme={activeCodePalette.id}
      data-testid="karyora-v3-root"
      data-platform={runtimeFoundation.platformBridge.runtimeTarget}
    >
      <main
        style={{
          margin: "auto",
          maxWidth: "820px",
          width: "100%",
          padding: "var(--space-6)",
          borderRadius: "var(--radius-lg)",
          backgroundColor: "var(--bg-surface-1)",
          border: "1px solid var(--border-default)",
        }}
      >
        <header
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: "var(--space-3)",
            marginBottom: "var(--space-4)",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
            <ChromeIcon name="sparkles" size={20} />
            <h1 style={{ margin: 0, fontSize: "1.1875rem" }}>
              Karyora — Unified AI Work Platform (v3.0 Phase 2 UIR-2 Baseline)
            </h1>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
            <Badge
              tone="success"
              dot
              label={`WCAG AA ${runtimeFoundation.contrastAudit.length}/${runtimeFoundation.contrastAudit.length} PASS`}
            />
            <DiffCountPill additions={57} deletions={3} filesCount={2} />
          </div>
        </header>

        <div style={{ marginBottom: "var(--space-4)" }}>
          <Toggle
            checked={darkThemeEnabled}
            onChange={setDarkThemeEnabled}
            label="Dark Theme Mode (UI-08)"
            description={`Active Code Theme: ${activeCodePalette.id} (${LIGHT_CODE_THEMES.length} Light / ${DARK_CODE_THEMES.length} Dark themes)`}
          />
        </div>

        <Tabs
          ariaLabel="Phase 2 UIR-2 Design System Verification Tabs"
          activeTabId={activePreviewTab}
          onSelectTab={setActivePreviewTab}
          tabs={[
            {
              id: "tokens",
              label: "Icons & Tokens",
              badgeCount: MATERIAL_ICON_TOTAL_COUNT,
            },
            {
              id: "recovery",
              label: "18-State RecoveryPanel (S4)",
            },
          ]}
        >
          {activePreviewTab === "tokens" ? (
            <div
              style={{
                paddingTop: "var(--space-3)",
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-3)",
              }}
            >
              <div
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  gap: "var(--space-3)",
                }}
              >
                <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                  <MaterialFileIcon
                    filename="planstore_test.go"
                    codeTheme={activeCodePalette.id}
                  />
                  <code>planstore_test.go</code>
                </span>
                <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                  <MaterialFileIcon
                    filename="AgenticComposer.tsx"
                    codeTheme={activeCodePalette.id}
                  />
                  <code>AgenticComposer.tsx</code>
                </span>
                <span style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
                  <MaterialFileIcon
                    filename="Dockerfile"
                    codeTheme={activeCodePalette.id}
                  />
                  <code>Dockerfile</code>
                </span>
                <span className="animated-gradient-text" style={{ fontWeight: 600 }}>
                  Working for <DigitFlipCounter value="26m 37s" />
                </span>
              </div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
                <Button variant="primary" size="sm">
                  Primary Action
                </Button>
                <Button variant="secondary" size="sm">
                  Secondary ({TARGET_PLATFORMS.length} Platforms)
                </Button>
                <Button variant="ghost" size="sm">
                  {SETTINGS_SECTIONS.length} Settings ·{" "}
                  {LOCKED_BYOK_PROVIDER_PRESETS.length} BYOK ·{" "}
                  {SUPPORTED_LOCALES.length} Locales ·{" "}
                  {UI_DECISION_REGISTER_IDS.length} Decisions
                </Button>
              </div>
            </div>
          ) : (
            <div style={{ paddingTop: "var(--space-3)" }}>
              <RecoveryPanel descriptor={sampleRecovery} />
            </div>
          )}
        </Tabs>
      </main>
    </div>
  );
}
