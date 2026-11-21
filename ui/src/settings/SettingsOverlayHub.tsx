/**
 * Complete 21-Section Overlay Settings Hub (UIR-8, Screenshots #1–#60, Blueprint §13).
 *
 * Full-viewport overlay (`Ctrl+,`) mounted above the live (`inert`) 3-region workspace:
 * - Top-left `← Back to app` button + `Escape` close handler
 * - 3-Group Left Navigation (`Basics` 7, `Agent capabilities` 9, `Data and statistics` 5 = 21 sections)
 * - Dirty-form navigation guard (`ConfirmDialog`) preventing accidental loss of unsaved edits
 */

import { useEffect, useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, ConfirmDialog } from "../components/primitives";
import type { WorkspaceProjectSummary } from "../coreclient";
import type { ScopedMemorySurfaceProps } from "../domains/memory";
import type { MarketplacePluginRecord } from "../marketplace";
import {
  SETTINGS_SECTIONS,
  type AppTheme,
  type DarkCodeTheme,
  type InterfaceMode,
  type LightCodeTheme,
  type SettingsSectionId,
  type SupportedLocale,
  type TargetPlatform,
} from "../types/decisions";
import {
  AppearanceSettingsSection,
  BrowserUseSettingsSection,
  ComputerUseSettingsSection,
  GeneralSettingsSection,
  KeyboardShortcutsSettingsSection,
  ModelSettingsSection,
  RemoteDevicesSettingsSection,
} from "./basics";
import {
  BehaviorAndToolsSettingsSection,
  CommandsSettingsSection,
  HooksSettingsSection,
  McpServersSettingsSection,
  MemorySettingsSection,
  PersonalizationSettingsSection,
  PluginsSettingsSection,
  SkillsSettingsSection,
  SubagentsSettingsSection,
} from "./capabilities";
import {
  ArchiveManagementSection,
  DataSyncAndBackupsSection,
  ProjectManagementSection,
  SharesAndGovernanceSection,
  UsageStatsSection,
} from "./data-governance";
import { SETTINGS_GROUPS } from "./settingsConfig";

export interface SettingsOverlayHubProps {
  readonly isOpen: boolean;
  readonly activeSection: SettingsSectionId;
  readonly runtimeTarget: TargetPlatform;
  readonly computerUseSupported: boolean;
  readonly appTheme: AppTheme;
  readonly lightCodeTheme: LightCodeTheme;
  readonly darkCodeTheme: DarkCodeTheme;
  readonly locale: SupportedLocale;
  readonly interfaceMode: InterfaceMode;
  readonly browserUseEnabled: boolean;
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly plugins: readonly MarketplacePluginRecord[];
  readonly memoryProps: ScopedMemorySurfaceProps;
  readonly onSelectSection: (section: SettingsSectionId) => void;
  readonly onChangeAppTheme: (theme: AppTheme) => void;
  readonly onChangeLightCodeTheme: (theme: LightCodeTheme) => void;
  readonly onChangeDarkCodeTheme: (theme: DarkCodeTheme) => void;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onToggleBrowserUse: (enabled: boolean) => void;
  readonly onTogglePlugin: (pluginId: string, enabled: boolean) => void;
  readonly onBrowseMarketplace: () => void;
  readonly onSwitchCoreConnection: (mode: "local" | "remote") => void;
  readonly onOpenOnboarding: () => void;
  readonly onOpenMigrationGuide: () => void;
  readonly onNotify: (msg: string) => void;
  readonly onClose: () => void;
}

export default function SettingsOverlayHub({
  isOpen,
  activeSection,
  runtimeTarget,
  computerUseSupported,
  appTheme,
  lightCodeTheme,
  darkCodeTheme,
  locale,
  interfaceMode,
  browserUseEnabled,
  projects,
  plugins,
  memoryProps,
  onSelectSection,
  onChangeAppTheme,
  onChangeLightCodeTheme,
  onChangeDarkCodeTheme,
  onChangeLocale,
  onChangeInterfaceMode,
  onToggleBrowserUse,
  onTogglePlugin,
  onBrowseMarketplace,
  onSwitchCoreConnection,
  onOpenOnboarding,
  onOpenMigrationGuide,
  onNotify,
  onClose,
}: SettingsOverlayHubProps): ReactElement | null {
  const [hasDirtyForm, setHasDirtyForm] = useState(false);
  const [pendingNavTarget, setPendingNavTarget] = useState<
    { readonly kind: "close" } | { readonly kind: "section"; readonly id: SettingsSectionId } | null
  >(null);

  const requestClose = () => {
    if (hasDirtyForm) {
      setPendingNavTarget({ kind: "close" });
    } else {
      onClose();
    }
  };

  const requestSection = (id: SettingsSectionId) => {
    if (id === activeSection) return;
    if (hasDirtyForm) {
      setPendingNavTarget({ kind: "section", id });
    } else {
      onSelectSection(id);
    }
  };

  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        requestClose();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  if (!isOpen) return null;

  const activeMeta =
    SETTINGS_SECTIONS.find((s) => s.id === activeSection) ?? SETTINGS_SECTIONS[0];

  return (
    <>
      <div
        className="ky-settings-overlay"
        role="dialog"
        aria-modal="true"
        aria-label="Karyora 21-Section Overlay Settings Hub"
        data-testid="settings-overlay-hub"
      >
        <nav className="ky-settings-nav" aria-label="Settings sections">
          <div className="ky-settings-nav__top">
            <Button size="sm" variant="ghost" onClick={requestClose} data-testid="settings-back-to-app">
              <ChromeIcon name="arrow-left" size={14} />
              <span>Back to app</span>
            </Button>
            {hasDirtyForm && <Badge tone="warning" label="Unsaved" />}
          </div>

          <div className="ky-settings-nav__scroll">
            {SETTINGS_GROUPS.map((group) => {
              const groupSections = SETTINGS_SECTIONS.filter(
                (sec) => sec.group === group.id,
              );
              return (
                <div key={group.id}>
                  <h4 className="ky-settings-nav__group-title">{group.label}</h4>
                  {groupSections.map((sec) => (
                    <button
                      key={sec.id}
                      type="button"
                      className="ky-settings-nav__btn"
                      data-testid={`settings-nav-${sec.id}`}
                      data-active={sec.id === activeSection ? "true" : "false"}
                      onClick={() => requestSection(sec.id)}
                    >
                      <span>{sec.label}</span>
                      <span style={{ fontSize: "0.7rem", color: "var(--text-muted)" }}>
                        {sec.screenshots}
                      </span>
                    </button>
                  ))}
                </div>
              );
            })}
          </div>
        </nav>

        <main className="ky-settings-main">
          <div className="ky-settings-content" style={{ marginBottom: "var(--space-4)" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
              <h2 style={{ margin: 0, fontSize: "1.3rem" }}>{activeMeta.label}</h2>
              <Badge tone="neutral" label={`Reference ${activeMeta.screenshots}`} />
            </div>
          </div>

          {activeSection === "general" ? (
            <GeneralSettingsSection
              locale={locale}
              interfaceMode={interfaceMode}
              onChangeLocale={onChangeLocale}
              onChangeInterfaceMode={onChangeInterfaceMode}
              onOpenOnboarding={onOpenOnboarding}
              onNotify={onNotify}
            />
          ) : activeSection === "appearance" ? (
            <AppearanceSettingsSection
              appTheme={appTheme}
              lightCodeTheme={lightCodeTheme}
              darkCodeTheme={darkCodeTheme}
              onChangeAppTheme={onChangeAppTheme}
              onChangeLightCodeTheme={onChangeLightCodeTheme}
              onChangeDarkCodeTheme={onChangeDarkCodeTheme}
            />
          ) : activeSection === "model-settings" ? (
            <ModelSettingsSection
              onDirtyChange={setHasDirtyForm}
              onNotify={onNotify}
            />
          ) : activeSection === "browser-use" ? (
            <BrowserUseSettingsSection
              browserUseEnabled={browserUseEnabled}
              onToggleBrowserUse={onToggleBrowserUse}
              onNotify={onNotify}
            />
          ) : activeSection === "remote-devices" ? (
            <RemoteDevicesSettingsSection
              onSwitchCoreConnection={onSwitchCoreConnection}
              onNotify={onNotify}
            />
          ) : activeSection === "computer-use" ? (
            <ComputerUseSettingsSection
              runtimeTarget={runtimeTarget}
              computerUseSupported={computerUseSupported}
            />
          ) : activeSection === "keyboard-shortcuts" ? (
            <KeyboardShortcutsSettingsSection />
          ) : activeSection === "personalization" ? (
            <PersonalizationSettingsSection
              onDirtyChange={setHasDirtyForm}
              onNotify={onNotify}
            />
          ) : activeSection === "behavior-and-tools" ? (
            <BehaviorAndToolsSettingsSection />
          ) : activeSection === "memory" ? (
            <MemorySettingsSection {...memoryProps} />
          ) : activeSection === "subagents" ? (
            <SubagentsSettingsSection onNotify={onNotify} />
          ) : activeSection === "plugins" ? (
            <PluginsSettingsSection
              plugins={plugins}
              onTogglePlugin={onTogglePlugin}
              onBrowseMarketplace={onBrowseMarketplace}
              onNotify={onNotify}
            />
          ) : activeSection === "mcp-servers" ? (
            <McpServersSettingsSection onNotify={onNotify} />
          ) : activeSection === "skills" ? (
            <SkillsSettingsSection onNotify={onNotify} />
          ) : activeSection === "commands" ? (
            <CommandsSettingsSection onNotify={onNotify} />
          ) : activeSection === "hooks" ? (
            <HooksSettingsSection onNotify={onNotify} />
          ) : activeSection === "data-sync" ? (
            <DataSyncAndBackupsSection
              onOpenMigrationGuide={onOpenMigrationGuide}
              onNotify={onNotify}
            />
          ) : activeSection === "usage-stats" ? (
            <UsageStatsSection />
          ) : activeSection === "project-management" ? (
            <ProjectManagementSection projects={projects} onNotify={onNotify} />
          ) : activeSection === "archive-management" ? (
            <ArchiveManagementSection onNotify={onNotify} />
          ) : (
            <SharesAndGovernanceSection
              runtimeTarget={runtimeTarget}
              onNotify={onNotify}
            />
          )}
        </main>
      </div>

      <ConfirmDialog
        isOpen={Boolean(pendingNavTarget)}
        title="Discard unsaved settings changes?"
        explanation="You have unsaved edits in this Settings section. Leave anyway and discard those unsaved changes?"
        confirmLabel="Discard changes"
        onConfirm={() => {
          const target = pendingNavTarget;
          setHasDirtyForm(false);
          setPendingNavTarget(null);
          if (target?.kind === "close") {
            onClose();
          } else if (target?.kind === "section") {
            onSelectSection(target.id);
          }
        }}
        onCancel={() => setPendingNavTarget(null)}
      />
    </>
  );
}
