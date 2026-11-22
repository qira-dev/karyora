/**
 * `Plugin Marketplace` Surface — Calibrated 1:1 to ZCode Screenshot #36.
 *
 * Compact 2-column icon+summary ZCode plugin cards, `Public` | `Community` | `Personal` tabs,
 * category pills, live search, and WASM SHA-256 governance drawer.
 */

import { useMemo, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Input } from "../components/primitives";
import {
  MARKETPLACE_CATEGORIES,
  MARKETPLACE_TABS,
  evaluatePluginInstallPolicy,
  filterMarketplacePlugins,
  type MarketplacePluginRecord,
  type MarketplaceTabId,
  type PluginCategory,
} from "./marketplaceConfig";
import { PluginGovernanceDrawer } from "./PluginGovernanceDrawer";

export interface PluginMarketplacePageProps {
  readonly plugins: readonly MarketplacePluginRecord[];
  readonly onInstallPlugin: (pluginId: string) => void;
  readonly onTogglePluginEnabled: (pluginId: string, enabled: boolean) => void;
  readonly onRemovePlugin: (pluginId: string) => void;
  readonly onRevokePlugin: (pluginId: string) => void;
  readonly onOpenPluginsSettings: () => void;
}

export function PluginMarketplacePage({
  plugins,
  onInstallPlugin,
  onTogglePluginEnabled,
  onRemovePlugin,
  onRevokePlugin,
  onOpenPluginsSettings,
}: PluginMarketplacePageProps): ReactElement {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<MarketplaceTabId>("public");
  const [activeCategory, setActiveCategory] = useState<PluginCategory>("All");
  const [searchQuery, setSearchQuery] = useState("");
  const [inspectingPluginId, setInspectingPluginId] = useState<string | null>(null);

  const visiblePlugins = useMemo(
    () => filterMarketplacePlugins(plugins, activeTab, activeCategory, searchQuery),
    [plugins, activeTab, activeCategory, searchQuery],
  );

  const inspectingPlugin = inspectingPluginId
    ? plugins.find((p) => p.pluginId === inspectingPluginId) ?? null
    : null;

  return (
    <section
      className="ky-domain-page"
      data-testid="plugin-marketplace-page"
      aria-label="Plugin Marketplace"
    >
      <header className="ky-domain-page__header">
        <div className="ky-domain-page__title-group">
          <h2 className="ky-domain-page__title">
            {t("ui.surfaces.marketplaceTitle", { defaultValue: "Plugin Marketplace" })}
          </h2>
          <p className="ky-domain-page__subtitle">
            {t("ui.surfaces.marketplaceSubtitle", {
              defaultValue:
                "Discover WASM-sandboxed app plugins and developer extensions verified by Core SHA-256 digests.",
            })}
          </p>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <div className="ky-segmented-tabs" role="tablist" aria-label="Marketplace catalogs">
            {MARKETPLACE_TABS.map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.id}
                className="ky-segmented-tabs__btn"
                data-active={activeTab === tab.id ? "true" : "false"}
                onClick={() => setActiveTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <Button size="sm" variant="secondary" onClick={onOpenPluginsSettings}>
            <ChromeIcon name="settings" size={14} />
            <span>
              {t("ui.surfaces.manageInstalled", { defaultValue: "Manage installed" })}
            </span>
          </Button>
        </div>
      </header>

      <div className="ky-marketplace-filters">
        <div className="ky-category-pills" role="group" aria-label="Plugin categories">
          {MARKETPLACE_CATEGORIES.map((category) => (
            <button
              key={category}
              type="button"
              className="ky-category-pill"
              data-active={activeCategory === category ? "true" : "false"}
              onClick={() => setActiveCategory(category)}
            >
              {category}
            </button>
          ))}
        </div>

        <div style={{ minWidth: "240px" }}>
          <Input
            aria-label="Search plugins"
            placeholder={t("ui.surfaces.searchPlugins", {
              defaultValue: "Search plugins...",
            })}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-plugin-grid" data-testid="plugin-marketplace-grid">
        {visiblePlugins.map((plugin) => {
          const policy = evaluatePluginInstallPolicy(plugin);
          const isInstalled =
            plugin.lifecycleState === "installed-enabled" ||
            plugin.lifecycleState === "installed-disabled";
          return (
            <article
              key={plugin.pluginId}
              className="ky-plugin-card"
              data-testid={`plugin-card-${plugin.pluginId}`}
            >
              <div className="ky-plugin-card__head-row">
                <span className="ky-plugin-card__icon-box">
                  <ChromeIcon name="plugin-marketplace" size={16} />
                </span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="ky-plugin-card__top">
                    <h3 className="ky-plugin-card__title">{plugin.name}</h3>
                    <span className="ky-plugin-card__publisher">v{plugin.version}</span>
                  </div>
                  <p className="ky-plugin-card__summary">{plugin.summary}</p>
                </div>
              </div>

              <div className="ky-plugin-card__footer">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setInspectingPluginId(plugin.pluginId)}
                >
                  <ChromeIcon name="shield-security" size={14} />
                  <span>{t("ui.surfaces.governance", { defaultValue: "Governance" })}</span>
                </Button>

                {plugin.lifecycleState === "revoked" ? (
                  <Badge tone="danger" label="Revoked" />
                ) : isInstalled ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => setInspectingPluginId(plugin.pluginId)}
                  >
                    {plugin.lifecycleState === "installed-enabled"
                      ? t("ui.surfaces.open", { defaultValue: "Open" })
                      : "Disabled"}
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="primary"
                    disabled={!policy.allowed}
                    onClick={() => {
                      if (policy.requiresExplicitTrustReview) {
                        setInspectingPluginId(plugin.pluginId);
                      } else {
                        onInstallPlugin(plugin.pluginId);
                      }
                    }}
                  >
                    <span>
                      {policy.requiresExplicitTrustReview
                        ? "Review & Install"
                        : t("ui.surfaces.install", { defaultValue: "Install" })}
                    </span>
                  </Button>
                )}
              </div>
            </article>
          );
        })}
      </div>

      <PluginGovernanceDrawer
        plugin={inspectingPlugin}
        onInstall={onInstallPlugin}
        onToggleEnabled={onTogglePluginEnabled}
        onRemove={onRemovePlugin}
        onRevoke={onRevokePlugin}
        onClose={() => setInspectingPluginId(null)}
      />
    </section>
  );
}
