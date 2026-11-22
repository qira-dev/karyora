/**
 * Settings Section 21: Shares & Governance — Calibrated 1:1 to ZCode Screenshot #60.
 *
 * Implements:
 * - Active conversation share links management (#60)
 * - Privacy & Redacted Diagnostics bundle preview/export
 * - Updates & Official Maintenance with Apache-2.0 Open-Source Non-Bricking Covenant
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Badge, Button, Select, Toggle } from "../../components/primitives";
import type { TargetPlatform } from "../../types/decisions";
import { buildRedactedDiagnosticBundle } from "../settingsConfig";

export interface SharesAndGovernanceSectionProps {
  readonly runtimeTarget: TargetPlatform;
  readonly onNotify: (msg: string) => void;
}

export function SharesAndGovernanceSection({
  runtimeTarget,
  onNotify,
}: SharesAndGovernanceSectionProps): ReactElement {
  const { t } = useTranslation();
  const [shares, setShares] = useState([
    { id: "sh-1", title: "v3.0 Architecture Walkthrough", visibility: "Redacted snapshot", createdAt: "Today" },
  ]);
  const [optInTelemetry, setOptInTelemetry] = useState(false);
  const [entitlementState, setEntitlementState] = useState<
    "Not linked" | "Active" | "Expired" | "Offline"
  >("Not linked");
  const [diagnosticPreview, setDiagnosticPreview] = useState<string | null>(null);

  return (
    <div className="ky-settings-content" data-testid="settings-section-shares-and-governance">
      {shares.length === 0 ? (
        <div className="ky-settings-empty">
          <p className="ky-settings-empty__title">
            {t("ui.settings.dataGov.noActiveShares", { defaultValue: "No active shares" })}
          </p>
          <p className="ky-settings-empty__desc">
            {t("ui.settings.dataGov.noActiveSharesDesc", {
              defaultValue:
                "Conversations you share appear here. Expired shares are not shown.",
            })}
          </p>
        </div>
      ) : (
        <div className="ky-settings-list">
          {shares.map((sh) => (
            <div key={sh.id} className="ky-settings-list-row">
              <div className="ky-settings-list-row__copy">
                <span className="ky-settings-list-row__title">
                  <span>{sh.title}</span>
                  <Badge tone="neutral" label={sh.visibility} />
                </span>
                <span className="ky-settings-list-row__desc">{sh.createdAt}</span>
              </div>
              <div className="ky-settings-list-row__right">
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => {
                    setShares((prev) => prev.filter((item) => item.id !== sh.id));
                    onNotify(`Revoked share link for "${sh.title}".`);
                  }}
                >
                  {t("ui.settings.dataGov.revokeLink", { defaultValue: "Revoke link" })}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.dataGov.privacyTitle", {
          defaultValue: "Privacy & Redacted Diagnostics",
        })}
      </h3>
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">Anonymous crash telemetry</div>
            <div className="ky-setting-row__desc">
              Opt-in crash metadata only. Prompts, file contents, and credentials are never collected.
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Opt-in anonymous crash telemetry"
              checked={optInTelemetry}
              onChange={setOptInTelemetry}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">Redacted diagnostic bundle</div>
            <div className="ky-setting-row__desc">
              Inspect and export scrubbed runtime diagnostics with all tokens and secrets stripped.
            </div>
          </div>
          <div className="ky-setting-row__control" style={{ display: "flex", gap: "8px" }}>
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                setDiagnosticPreview(
                  buildRedactedDiagnosticBundle({
                    runtimeTarget,
                    appVersion: "v3.0.0-clean-slate",
                    coreConnected: true,
                  }),
                )
              }
            >
              Preview bundle
            </Button>
            {diagnosticPreview && (
              <Button
                size="sm"
                variant="primary"
                onClick={() => onNotify("Redacted diagnostic bundle exported cleanly.")}
              >
                Export JSON
              </Button>
            )}
          </div>
        </div>
      </div>

      {diagnosticPreview && (
        <pre className="ky-audit-json" data-testid="redacted-diagnostic-preview">
          {diagnosticPreview}
        </pre>
      )}

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.dataGov.maintenanceTitle", {
          defaultValue: "Updates & Official Maintenance",
        })}
      </h3>
      <div className="ky-setting-group" data-testid="official-maintenance-panel">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              <span>Apache-2.0 Open-Source Covenant</span>
              <span style={{ marginInlineStart: "8px" }}>
                <Badge
                  tone={entitlementState === "Active" ? "success" : "neutral"}
                  label={`Maintenance: ${entitlementState}`}
                />
              </span>
            </div>
            <div className="ky-setting-row__desc">
              Karyora operates 100% locally and offline with zero kill switches, zero DRM, and zero
              in-app checkout regardless of maintenance plan status.
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Entitlement state preview"
              value={entitlementState}
              options={[
                { value: "Not linked", label: "Not linked" },
                { value: "Active", label: "Active" },
                { value: "Expired", label: "Expired (App remains usable)" },
                { value: "Offline", label: "Unknown (Offline)" },
              ]}
              onChange={(e) =>
                setEntitlementState(
                  e.target.value as "Not linked" | "Active" | "Expired" | "Offline",
                )
              }
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">Official maintenance portal</div>
            <div className="ky-setting-row__desc">
              Opens in your external system browser. Never blocks local builds or offline usage.
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                onNotify("Opens official maintenance portal in external system browser.")
              }
            >
              Manage plan
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
