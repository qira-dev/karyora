/**
 * Side-Pane `New tab` Categorized Launcher (UIR-5.1, Screenshots #74, #82, Blueprint §8.1).
 *
 * Displays two clear categories:
 * 1. Top Workspace Tools (`Files`, `Browser`, `Terminal`, `Review`, `Sites`,
 *    `Side conversation`, `Subagents`, `Context Inspector`)
 * 2. `App plugins` (`Documents`, `PDF`, `Presentations`, `Spreadsheets`)
 */

import type { ReactElement } from "react";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
import {
  NEW_TAB_APP_PLUGINS,
  NEW_TAB_TOOLS,
  type NewTabLauncherItem,
} from "./sidePaneConfig";

export interface NewTabLauncherProps {
  readonly onLaunchItem: (item: NewTabLauncherItem) => void;
}

export function NewTabLauncher({ onLaunchItem }: NewTabLauncherProps): ReactElement {
  return (
    <section
      className="ky-new-tab-launcher"
      data-testid="side-pane-new-tab-launcher"
      aria-label="New side pane tab launcher"
    >
      <div className="ky-new-tab-launcher__group">
        <h3 className="ky-new-tab-launcher__heading">Workspace Tools</h3>
        <div className="ky-new-tab-launcher__grid">
          {NEW_TAB_TOOLS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="ky-new-tab-launcher__card"
              data-testid={`launcher-item-${item.id}`}
              onClick={() => onLaunchItem(item)}
            >
              <span className="ky-new-tab-launcher__icon">
                <ChromeIcon name={item.iconName as ChromeIconName} size={18} />
              </span>
              <span className="ky-new-tab-launcher__copy">
                <span className="ky-new-tab-launcher__title">{item.label}</span>
                <span className="ky-new-tab-launcher__desc">{item.description}</span>
              </span>
            </button>
          ))}
        </div>
      </div>

      <div className="ky-new-tab-launcher__group">
        <h3 className="ky-new-tab-launcher__heading">App plugins</h3>
        <div className="ky-new-tab-launcher__grid">
          {NEW_TAB_APP_PLUGINS.map((item) => (
            <button
              key={item.id}
              type="button"
              className="ky-new-tab-launcher__card"
              data-testid={`launcher-item-${item.id}`}
              onClick={() => onLaunchItem(item)}
            >
              <span className="ky-new-tab-launcher__icon">
                <ChromeIcon name={item.iconName as ChromeIconName} size={18} />
              </span>
              <span className="ky-new-tab-launcher__copy">
                <span className="ky-new-tab-launcher__title">{item.label}</span>
                <span className="ky-new-tab-launcher__desc">{item.description}</span>
              </span>
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
