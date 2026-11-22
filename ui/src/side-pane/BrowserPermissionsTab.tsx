/**
 * Per-Origin Site Permissions Tab (1:1 ZCode v3.18.2 `vYn` / `browser-permissions` parity).
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
import { Button } from "../components/primitives";
import {
  SITE_PERMISSION_CAPABILITIES,
  type OriginPermissionsState,
  type SitePermissionDecision,
} from "./browserConfig";

export interface BrowserPermissionsTabProps {
  readonly origin: string;
  readonly permissions: OriginPermissionsState;
  readonly onSetPermission: (
    origin: string,
    capabilityKey: string,
    decision: SitePermissionDecision,
  ) => void;
  readonly onResetPermissions: (origin: string) => void;
}

export function BrowserPermissionsTab({
  origin,
  permissions,
  onSetPermission,
  onResetPermissions,
}: BrowserPermissionsTabProps): ReactElement {
  const { t } = useTranslation();
  const [statusMessage, setStatusMessage] = useState<string | null>(null);

  return (
    <section
      className="ky-side-tool-page"
      data-testid="browser-site-permissions"
      aria-label={`Site permissions for ${origin}`}
    >
      <div className="ky-side-tool-page__inner">
        <h2 className="ky-side-tool-page__origin">{origin}</h2>

        <div className="ky-side-tool-page__head-row">
          <h3 className="ky-side-tool-page__subhead">
            {t("ui.browser.sitePermissions", { defaultValue: "Site permissions" })}
          </h3>
          <Button
            size="sm"
            variant="secondary"
            data-testid="browser-permission-reset"
            onClick={() => {
              onResetPermissions(origin);
              setStatusMessage(
                t("ui.browser.permissionsResetDone", {
                  defaultValue:
                    "Permissions reset. Reload the page to request access again.",
                }),
              );
            }}
          >
            {t("ui.browser.resetPermissions", {
              defaultValue: "Reset site permissions",
            })}
          </Button>
        </div>

        <div className="ky-setting-group">
          {SITE_PERMISSION_CAPABILITIES.map((cap) => {
            const current: SitePermissionDecision = permissions[cap.key] ?? "ask";
            return (
              <div
                key={cap.key}
                className="ky-setting-row"
                data-permission-key={cap.key}
              >
                <div className="ky-setting-row__info">
                  <div style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
                    <ChromeIcon name={cap.iconName as ChromeIconName} size={14} />
                    <span className="ky-setting-row__title">{cap.label}</span>
                  </div>
                  <p className="ky-setting-row__desc">{cap.description}</p>
                </div>

                <div className="ky-setting-row__control ky-setting-row__control--compact">
                  <select
                    aria-label={cap.label}
                    className="ky-select"
                    style={{ width: "140px" }}
                    value={current}
                    data-testid={`site-permission-setting-${cap.key}`}
                    onChange={(e) => {
                      const next = e.target.value as SitePermissionDecision;
                      onSetPermission(origin, cap.key, next);
                      setStatusMessage(
                        t("ui.browser.permissionsSaved", {
                          defaultValue:
                            "Permissions updated. Reload the page to apply changes to active resources.",
                        }),
                      );
                    }}
                  >
                    <option value="ask">Ask</option>
                    <option value="allow">Allow</option>
                    <option value="block">Block</option>
                  </select>
                </div>
              </div>
            );
          })}
        </div>

        {statusMessage && (
          <p role="status" className="ky-side-tool-page__status">
            {statusMessage}
          </p>
        )}
      </div>
    </section>
  );
}

export default BrowserPermissionsTab;
