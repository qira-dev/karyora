/**
 * Settings Section 21: Shares & Karyora Privacy / Security / Diagnostics / Updates & Maintenance
 * (UIR-8.3, Screenshot #60, Blueprint §13.3, UI-21, G-11, E2E-20).
 *
 * Enforces:
 * - Active conversation share links management (#60)
 * - Privacy & Data opt-in boundary (zero prompt harvesting)
 * - 3-stage Redacted Diagnostic Bundle Preview & Export (`buildRedactedDiagnosticBundle`)
 * - Official Maintenance signed entitlement status (`Active` / `Expired` / `Not linked` / `Offline`)
 *   with explicit Open-Source Non-Bricking Covenant (NEVER disables the installed app, ZERO in-app checkout per UI-21)
 */

import { useState, type ReactElement } from "react";
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
      {/* Shares (#60) */}
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Active Conversation Shares (#60)</h3>
        {shares.length === 0 ? (
          <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-muted)" }}>
            No active shared links.
          </p>
        ) : (
          shares.map((sh) => (
            <div key={sh.id} className="ky-gov-perm-row">
              <span>
                <strong>{sh.title}</strong> · <Badge tone="info" label={sh.visibility} />
              </span>
              <Button
                size="sm"
                variant="danger"
                onClick={() => {
                  setShares((prev) => prev.filter((item) => item.id !== sh.id));
                  onNotify(`Revoked share link for "${sh.title}".`);
                }}
              >
                Revoke link
              </Button>
            </div>
          ))
        )}
      </div>

      {/* Privacy, Security & Diagnostics Redaction Preview */}
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Privacy, Security & Redacted Diagnostics (G-11)</h3>
        <Toggle
          label="Opt-in anonymous crash telemetry (zero prompts or secrets)"
          checked={optInTelemetry}
          onChange={setOptInTelemetry}
        />
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
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
            Preview Redacted Diagnostic Bundle
          </Button>
          {diagnosticPreview && (
            <Button
              size="sm"
              variant="primary"
              onClick={() => onNotify("Redacted diagnostic bundle exported cleanly.")}
            >
              Export Bundle JSON
            </Button>
          )}
        </div>
        {diagnosticPreview && (
          <pre className="ky-audit-json" data-testid="redacted-diagnostic-preview">
            {diagnosticPreview}
          </pre>
        )}
      </div>

      {/* Updates & Official Maintenance (UI-21 Open-Source Non-Bricking Covenant) */}
      <div className="ky-settings-card" data-testid="official-maintenance-panel">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <h3 className="ky-settings-card__title">Updates & Official Maintenance (UI-21)</h3>
          <Badge
            tone={entitlementState === "Active" ? "success" : "neutral"}
            label={`Maintenance: ${entitlementState}`}
          />
        </div>
        <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
          Apache-2.0 Open-Source Covenant: Karyora operates 100% locally and offline with zero
          kill switches, zero DRM, and zero in-app checkout regardless of maintenance plan status.
        </p>
        <div className="ky-settings-grid-2">
          <Select
            label="Entitlement state preview"
            value={entitlementState}
            options={[
              { value: "Not linked", label: "Not linked" },
              { value: "Active", label: "Active" },
              { value: "Expired", label: "Expired (App remains 100% usable)" },
              { value: "Offline", label: "Unknown (Offline)" },
            ]}
            onChange={(e) =>
              setEntitlementState(
                e.target.value as "Not linked" | "Active" | "Expired" | "Offline",
              )
            }
          />
          <div style={{ alignSelf: "end" }}>
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                onNotify("Opens official maintenance portal in external system browser (no in-app checkout).")
              }
            >
              Manage maintenance plan (external browser)
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
