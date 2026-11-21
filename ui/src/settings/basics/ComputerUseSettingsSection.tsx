/**
 * Settings Section 6: Computer Use (UIR-8.1, Screenshot #20, Blueprint §3.2, §13.1 (6)).
 *
 * Honestly reports OS and Web-mode capability boundaries from `PlatformBridge`.
 */

import type { ReactElement } from "react";
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
  return (
    <div className="ky-settings-content" data-testid="settings-section-computer-use">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">Computer Use & OS Permission Boundary</h3>
          <Badge
            tone={computerUseSupported ? "success" : "warning"}
            label={
              computerUseSupported
                ? `Available on ${runtimeTarget}`
                : `Unavailable in ${runtimeTarget} mode`
            }
          />
        </div>
        <p style={{ margin: 0, fontSize: "0.84rem", color: "var(--text-secondary)", lineHeight: 1.5 }}>
          {computerUseSupported
            ? "Desktop Computer Use requires explicit OS Accessibility / Screen Recording permission and per-action operator approval."
            : "Computer Use is truthfully disabled in Web Browser mode and unsupported Linux headless hosts. Run the native macOS or Windows desktop shell to request OS desktop automation permissions."}
        </p>
      </div>
    </div>
  );
}
