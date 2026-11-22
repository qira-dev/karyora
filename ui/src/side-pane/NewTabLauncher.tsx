/**
 * Side-Pane `New tab` Categorized Launcher — Calibrated 1:1 to ZCode Screenshots #74, #82, #83.
 *
 * Renders clean horizontal tool rows (`Icon` | `Title + Description` | right-aligned `Open` pill)
 * under `New tab` and `App plugins` (with `View all plugins ›`), fully localized via i18next.
 */

import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
import {
  NEW_TAB_ADVANCED_TOOLS,
  NEW_TAB_APP_PLUGINS,
  NEW_TAB_TOOLS,
  type NewTabLauncherItem,
} from "./sidePaneConfig";

export interface NewTabLauncherProps {
  readonly onLaunchItem: (item: NewTabLauncherItem) => void;
  readonly onOpenMarketplace?: () => void;
}

export function NewTabLauncher({
  onLaunchItem,
  onOpenMarketplace,
}: NewTabLauncherProps): ReactElement {
  const { t } = useTranslation();
  const openLabel = t("ui.sidePane.open", { defaultValue: "Open" });

  return (
    <section
      className="ky-new-tab-launcher"
      data-testid="side-pane-new-tab-launcher"
      aria-label="New side pane tab launcher"
    >
      <div className="ky-new-tab-launcher__group">
        <div className="ky-new-tab-launcher__head-row">
          <h3 className="ky-new-tab-launcher__heading">
            {t("ui.sidePane.newTab", { defaultValue: "New tab" })}
          </h3>
        </div>
        <div className="ky-new-tab-launcher__grid">
          {NEW_TAB_TOOLS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="ky-new-tab-launcher__card"
              data-testid={`launcher-item-${item.id}`}
              onClick={() => onLaunchItem(item)}
            >
              <span className="ky-new-tab-launcher__left">
                <span className="ky-new-tab-launcher__icon">
                  <ChromeIcon name={item.iconName as ChromeIconName} size={16} />
                </span>
                <span className="ky-new-tab-launcher__copy">
                  <span className="ky-new-tab-launcher__title">{item.label}</span>
                  <span className="ky-new-tab-launcher__desc">{item.description}</span>
                </span>
              </span>
              <span className="ky-new-tab-launcher__open-pill">{openLabel}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="ky-new-tab-launcher__group">
        <div className="ky-new-tab-launcher__head-row">
          <h3 className="ky-new-tab-launcher__heading">
            {t("ui.sidePane.workspaceTools", { defaultValue: "Workspace tools" })}
          </h3>
        </div>
        <div className="ky-new-tab-launcher__grid">
          {NEW_TAB_ADVANCED_TOOLS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="ky-new-tab-launcher__card"
              data-testid={`launcher-item-${item.id}`}
              onClick={() => onLaunchItem(item)}
            >
              <span className="ky-new-tab-launcher__left">
                <span className="ky-new-tab-launcher__icon">
                  <ChromeIcon name={item.iconName as ChromeIconName} size={16} />
                </span>
                <span className="ky-new-tab-launcher__copy">
                  <span className="ky-new-tab-launcher__title">{item.label}</span>
                  <span className="ky-new-tab-launcher__desc">{item.description}</span>
                </span>
              </span>
              <span className="ky-new-tab-launcher__open-pill">{openLabel}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="ky-new-tab-launcher__group">
        <div className="ky-new-tab-launcher__head-row">
          <h3 className="ky-new-tab-launcher__heading">
            {t("ui.sidePane.appPlugins", { defaultValue: "App plugins" })}
          </h3>
          {onOpenMarketplace && (
            <button
              type="button"
              className="ky-new-tab-launcher__link"
              onClick={onOpenMarketplace}
            >
              {t("ui.sidePane.viewAllPlugins", { defaultValue: "View all plugins" })} ›
            </button>
          )}
        </div>
        <div className="ky-new-tab-launcher__grid">
          {NEW_TAB_APP_PLUGINS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="ky-new-tab-launcher__card"
              data-testid={`launcher-item-${item.id}`}
              onClick={() => onLaunchItem(item)}
            >
              <span className="ky-new-tab-launcher__left">
                <span className="ky-new-tab-launcher__icon">
                  <ChromeIcon name={item.iconName as ChromeIconName} size={16} />
                </span>
                <span className="ky-new-tab-launcher__copy">
                  <span className="ky-new-tab-launcher__title">{item.label}</span>
                  <span className="ky-new-tab-launcher__desc">{item.description}</span>
                </span>
              </span>
              <span className="ky-new-tab-launcher__open-pill">{openLabel}</span>
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
