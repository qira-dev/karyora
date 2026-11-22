/**
 * Complete 21-Section Overlay Settings Hub — Calibrated 1:1 to ZCode Screenshots #1–#60.
 *
 * Full-viewport overlay (`Ctrl+,`) mounted above the live (`inert`) workspace:
 * - Top-left `← Back to app` pill + live `Search settings` filter
 * - 3-Group Left Navigation (`Basics` 7, `Agent capabilities` 9, `Data and statistics` 5 = 21 sections)
 *   with crisp icons and live 13-locale i18next translations
 * - Dirty-form navigation guard (`ConfirmDialog`)
 */

import { useEffect, useState, type ReactElement, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
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

const SECTION_ICON_MAP: Readonly<Record<SettingsSectionId, ChromeIconName>> = {
  general: "sliders",
  appearance: "appearance",
  "model-settings": "model-cube",
  "browser-use": "browser-globe",
  "remote-devices": "remote-server",
  "computer-use": "computer-monitor",
  "keyboard-shortcuts": "keyboard",
  personalization: "profile",
  "behavior-and-tools": "settings",
  memory: "brain-thinking",
  subagents: "subagent",
  plugins: "plugin-marketplace",
  "mcp-servers": "mcp-plug",
  skills: "skills",
  commands: "commands",
  hooks: "hooks",
  "data-sync": "database",
  "usage-stats": "usage-chart",
  "project-management": "projects",
  "archive-management": "archive",
  "shares-and-governance": "shield-security",
};

const GROUP_I18N_KEY_MAP: Readonly<Record<string, string>> = {
  basics: "ui.settings.groups.basics",
  capabilities: "ui.settings.groups.agentCapabilities",
  "data-governance": "ui.settings.groups.dataAndStatistics",
};

export type SettingsEmbeddedCoreSurfaceId =
  | "jobs"
  | "knowledge"
  | "context"
  | "activity"
  | "automations";

const CORE_SETTINGS_ITEMS: readonly {
  readonly id: SettingsEmbeddedCoreSurfaceId;
  readonly labelKey: string;
  readonly defaultLabel: string;
  readonly icon: ChromeIconName;
}[] = [
  { id: "jobs", labelKey: "ui.sidebar.jobs", defaultLabel: "Jobs", icon: "cpu" },
  { id: "knowledge", labelKey: "ui.sidebar.knowledge", defaultLabel: "Knowledge", icon: "database" },
  { id: "context", labelKey: "ui.sidebar.context", defaultLabel: "Context", icon: "sliders" },
  { id: "activity", labelKey: "ui.sidebar.activityAudit", defaultLabel: "Activity & Audit", icon: "activity" },
  { id: "automations", labelKey: "ui.sidebar.automations", defaultLabel: "Automations", icon: "automations" },
];

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
  readonly coreSurfaceViews?: Readonly<Partial<Record<SettingsEmbeddedCoreSurfaceId, ReactNode>>>;
  readonly onOpenCoreSurfaceInWorkspace?: (surface: SettingsEmbeddedCoreSurfaceId) => void;
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
  coreSurfaceViews,
  onOpenCoreSurfaceInWorkspace,
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
  const { t } = useTranslation();
  const [hasDirtyForm, setHasDirtyForm] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [activeCoreSurface, setActiveCoreSurface] =
    useState<SettingsEmbeddedCoreSurfaceId | null>(null);
  const [pendingNavTarget, setPendingNavTarget] = useState<
    { readonly kind: "close" } | { readonly kind: "section"; readonly id: SettingsSectionId } | null
  >(null);

  const requestClose = () => {
    if (hasDirtyForm) {
      setPendingNavTarget({ kind: "close" });
    } else {
      setActiveCoreSurface(null);
      onClose();
    }
  };

  const requestSection = (id: SettingsSectionId) => {
    setActiveCoreSurface(null);
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
  const normalizedSearch = searchQuery.trim().toLowerCase();

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
            <button
              type="button"
              className="ky-settings-back-btn"
              onClick={requestClose}
              data-testid="settings-back-to-app"
            >
              <ChromeIcon name="arrow-left" size={14} />
              <span>{t("ui.settings.backToApp", { defaultValue: "Back to app" })}</span>
              {hasDirtyForm && (
                <span style={{ marginInlineStart: "auto" }}>
                  <Badge
                    tone="warning"
                    label={t("ui.settings.unsaved", { defaultValue: "Unsaved" })}
                  />
                </span>
              )}
            </button>

            <div className="ky-settings-search-wrap">
              <span
                aria-hidden="true"
                style={{
                  position: "absolute",
                  insetInlineStart: "10px",
                  color: "var(--text-muted)",
                  display: "inline-flex",
                }}
              >
                <ChromeIcon name="search" size={14} />
              </span>
              <input
                type="search"
                className="ky-settings-search-input"
                aria-label={t("ui.settings.searchSettings", { defaultValue: "Search settings" })}
                placeholder={t("ui.settings.searchSettings", { defaultValue: "Search settings" })}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>

          <div className="ky-settings-nav__scroll">
            {SETTINGS_GROUPS.map((group) => {
              const groupSections = SETTINGS_SECTIONS.filter((sec) => {
                if (sec.group !== group.id) return false;
                if (!normalizedSearch) return true;
                const localizedLabel = t(`ui.settings.sections.${sec.id}`, {
                  defaultValue: sec.label,
                });
                return (
                  localizedLabel.toLowerCase().includes(normalizedSearch) ||
                  sec.label.toLowerCase().includes(normalizedSearch)
                );
              });
              if (groupSections.length === 0) return null;
              return (
                <div key={group.id}>
                  <h4 className="ky-settings-nav__group-title">
                    {t(GROUP_I18N_KEY_MAP[group.id] ?? "ui.settings.groups.basics", {
                      defaultValue: group.label,
                    })}
                  </h4>
                  {groupSections.map((sec) => (
                    <button
                      key={sec.id}
                      type="button"
                      className="ky-settings-nav__btn"
                      data-testid={`settings-nav-${sec.id}`}
                      data-active={
                        activeCoreSurface === null && sec.id === activeSection
                          ? "true"
                          : "false"
                      }
                      onClick={() => requestSection(sec.id)}
                    >
                      <ChromeIcon name={SECTION_ICON_MAP[sec.id]} size={16} />
                      <span>
                        {t(`ui.settings.sections.${sec.id}`, {
                          defaultValue: sec.label,
                        })}
                      </span>
                    </button>
                  ))}
                </div>
              );
            })}

            {/* Karyora Core Surfaces Directly Accessible Inside Settings (Clean Left Sidebar) */}
            {(() => {
              const filteredCore = CORE_SETTINGS_ITEMS.filter((item) => {
                if (!normalizedSearch) return true;
                const locLabel = t(item.labelKey, { defaultValue: item.defaultLabel });
                return (
                  locLabel.toLowerCase().includes(normalizedSearch) ||
                  item.defaultLabel.toLowerCase().includes(normalizedSearch)
                );
              });
              if (filteredCore.length === 0) return null;
              return (
                <div>
                  <h4 className="ky-settings-nav__group-title">
                    {t("ui.sidebar.coreSection", { defaultValue: "Karyora Core" })}
                  </h4>
                  {filteredCore.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className="ky-settings-nav__btn"
                      data-testid={`settings-core-nav-${item.id}`}
                      data-active={activeCoreSurface === item.id ? "true" : "false"}
                      onClick={() => setActiveCoreSurface(item.id)}
                    >
                      <ChromeIcon name={item.icon} size={16} />
                      <span>
                        {t(item.labelKey, { defaultValue: item.defaultLabel })}
                      </span>
                    </button>
                  ))}
                </div>
              );
            })()}
          </div>
        </nav>

        <main className="ky-settings-main">
          {activeCoreSurface && coreSurfaceViews?.[activeCoreSurface] ? (
            <div style={{ display: "flex", flexDirection: "column", gap: "14px", width: "100%" }}>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "12px",
                }}
              >
                <h2 className="ky-settings-page-title">
                  {t(
                    CORE_SETTINGS_ITEMS.find((i) => i.id === activeCoreSurface)?.labelKey ??
                      "ui.sidebar.coreSection",
                    {
                      defaultValue:
                        CORE_SETTINGS_ITEMS.find((i) => i.id === activeCoreSurface)
                          ?.defaultLabel ?? "Workspace",
                    },
                  )}
                </h2>
                {onOpenCoreSurfaceInWorkspace && (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => {
                      const target = activeCoreSurface;
                      setActiveCoreSurface(null);
                      onClose();
                      onOpenCoreSurfaceInWorkspace(target);
                    }}
                  >
                    <ChromeIcon name="external-link" size={14} />
                    <span>Open full view</span>
                  </Button>
                )}
              </div>
              {coreSurfaceViews[activeCoreSurface]}
            </div>
          ) : (
            <>
              <div className="ky-settings-content" style={{ marginBottom: "18px" }}>
                <h2 className="ky-settings-page-title">
                  {t(`ui.settings.sections.${activeMeta.id}`, {
                    defaultValue: activeMeta.label,
                  })}
                </h2>
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
            </>
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
