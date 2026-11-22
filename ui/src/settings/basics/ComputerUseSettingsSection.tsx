/**
 * Settings Section 6: Computer Use — Calibrated 1:1 to ZCode Screenshot #20.
 *
 * Honestly reports OS and Web-mode capability boundaries from `PlatformBridge`.
 */

import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "../../components/primitives";
import type { TargetPlatform } from "../../types/decisions";

export interface ComputerUseSettingsSectionProps {
  readonly runtimeTarget: TargetPlatform;
  readonly computerUseSupported: boolean;
}

export function ComputerUseSettingsSection({
  runtimeTarget,
  computerUseSupported,
}: ComputerUseSettingsSectionProps): ReactElement {
  const { t } = useTranslation();

  return (
    <div className="ky-settings-content" data-testid="settings-section-computer-use">
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {computerUseSupported
                ? t("ui.settings.computerUse.availableTitle", {
                    defaultValue: "Computer Use is available",
                  })
                : t("ui.settings.computerUse.unavailableTitle", {
                    defaultValue: "Computer Use is unavailable here",
                  })}
            </div>
            <div className="ky-setting-row__desc">
              {computerUseSupported
                ? t("ui.settings.computerUse.availableDesc", {
                    defaultValue:
                      "Desktop Computer Use requires explicit OS Accessibility and Screen Recording permissions plus per-action approval.",
                  })
                : t("ui.settings.computerUse.unavailableDesc", {
                    defaultValue:
                      "Computer Use is not yet supported in Web Browser mode or unsupported Linux desktops. Switch to a native macOS or Windows workspace.",
                  })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Badge
              tone={computerUseSupported ? "success" : "neutral"}
              label={runtimeTarget}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
