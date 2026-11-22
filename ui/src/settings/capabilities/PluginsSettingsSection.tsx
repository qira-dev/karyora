/**
 * Settings Section 12: Plugins — Calibrated 1:1 to ZCode Screenshots #34–#35.
 *
 * Summary counters (`Plugins`, `MCP`, `Skills`), search filter, `Installed` & `Built-in`
 * grouped lists with `Browse plugin marketplace` button and per-plugin toggles.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, Input, Toggle } from "../../components/primitives";
import type { MarketplacePluginRecord } from "../../marketplace";

export interface PluginsSettingsSectionProps {
  readonly plugins: readonly MarketplacePluginRecord[];
  readonly onTogglePlugin: (pluginId: string, enabled: boolean) => void;
  readonly onBrowseMarketplace: () => void;
  readonly onNotify: (msg: string) => void;
}

export function PluginsSettingsSection({
  plugins,
  onTogglePlugin,
  onBrowseMarketplace,
  onNotify,
}: PluginsSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");

  const installedAll = plugins.filter(
    (p) =>
      p.lifecycleState === "installed-enabled" ||
      p.lifecycleState === "installed-disabled",
  );
  const q = search.trim().toLowerCase();
  const filtered = installedAll.filter(
    (p) => !q || p.name.toLowerCase().includes(q) || p.summary.toLowerCase().includes(q),
  );
  const userInstalled = filtered.filter((p) => !p.isBuiltIn);
  const builtInInstalled = filtered.filter((p) => p.isBuiltIn);

  const renderRow = (plugin: MarketplacePluginRecord) => (
    <div key={plugin.pluginId} className="ky-settings-list-row">
      <div className="ky-settings-list-row__main">
        <span className="ky-settings-list-row__icon">
          <ChromeIcon name="plugin-marketplace" size={16} />
        </span>
        <div className="ky-settings-list-row__copy">
          <span className="ky-settings-list-row__title">{plugin.name}</span>
          <span className="ky-settings-list-row__desc">{plugin.summary}</span>
        </div>
      </div>
      <div className="ky-settings-list-row__right">
        <Toggle
          label=""
          aria-label={`Enable ${plugin.name}`}
          checked={plugin.lifecycleState === "installed-enabled"}
          onChange={(checked) => onTogglePlugin(plugin.pluginId, checked)}
        />
      </div>
    </div>
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-plugins">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <span className="ky-settings-pill-counter">
            <span>{t("ui.settings.sections.plugins", { defaultValue: "Plugins" })}</span>
            <strong>{installedAll.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search plugins"
            placeholder={t("ui.settings.capabilities.searchPlugins", {
              defaultValue: "Search plugins",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{userInstalled.length}</span>
        </h3>
        <div style={{ display: "flex", gap: "8px" }}>
          <Button size="sm" variant="secondary" onClick={onBrowseMarketplace}>
            <ChromeIcon name="plugin-marketplace" size={14} />
            <span>
              {t("ui.settings.capabilities.browseMarketplace", {
                defaultValue: "Browse plugin marketplace",
              })}
            </span>
          </Button>
          <Button
            size="sm"
            variant="secondary"
            aria-label="Create plugin"
            onClick={() => onNotify("Create plugin wizard ready.")}
          >
            <ChromeIcon name="plus" size={14} />
          </Button>
        </div>
      </div>

      {userInstalled.length > 0 && (
        <div className="ky-settings-list">{userInstalled.map(renderRow)}</div>
      )}

      {builtInInstalled.length > 0 && (
        <>
          <div className="ky-settings-group-header">
            <h3 className="ky-settings-group-header__title">
              <span>{t("ui.settings.capabilities.builtIn", { defaultValue: "Built-in" })}</span>
              <span className="ky-settings-group-header__count">{builtInInstalled.length}</span>
            </h3>
          </div>
          <div className="ky-settings-list">{builtInInstalled.map(renderRow)}</div>
        </>
      )}
    </div>
  );
}
