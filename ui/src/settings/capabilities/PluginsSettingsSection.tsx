/**
 * Settings Section 12: Plugins (UIR-8.2, Screenshots #34–#35, Blueprint §11 & §13.2 (12)).
 */

import type { ReactElement } from "react";
import { Badge, Button, Toggle } from "../../components/primitives";
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
  const installed = plugins.filter(
    (p) =>
      p.lifecycleState === "installed-enabled" ||
      p.lifecycleState === "installed-disabled",
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-plugins">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <h3 className="ky-settings-card__title">
            Installed & Built-in Plugins ({installed.length})
          </h3>
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Button size="sm" variant="secondary" onClick={onBrowseMarketplace}>
              Browse plugin marketplace
            </Button>
            <Button
              size="sm"
              variant="primary"
              onClick={() => onNotify("Create plugin / Add plugin marketplace wizard ready.")}
            >
              Add ▾
            </Button>
          </div>
        </div>

        {installed.map((plugin) => (
          <div key={plugin.pluginId} className="ky-gov-perm-row">
            <div>
              <strong>{plugin.name}</strong>{" "}
              <Badge tone="neutral" label={plugin.isBuiltIn ? "Built-in" : "Installed"} />
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                {plugin.summary}
              </div>
            </div>
            <Toggle
              label={`Enable ${plugin.name}`}
              checked={plugin.lifecycleState === "installed-enabled"}
              onChange={(checked) => onTogglePlugin(plugin.pluginId, checked)}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
