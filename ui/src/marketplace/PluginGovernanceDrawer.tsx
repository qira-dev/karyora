/**
 * Core WASM `pluginhost` Governance & Trust Drawer
 * (UIR-6.3, Screenshots #34–#35, Blueprint §11, Quality Gate G-12, E2E-12).
 *
 * Displays:
 * - SHA-256 manifest digest & signature trust badge
 * - WASM ABI version & sandbox isolation (`wazero-wasm-isolated`)
 * - Honest `D-13/D-14` Core invocation boundary status
 * - Requested permission chips with risk badges
 * - Quota/timeout health (CPU ms, memory MiB, crash count) & failure diagnostic logs
 * - Explicit `Disable` / `Enable`, `Remove` (uninstall) vs `Revoke` (trust revocation with ConfirmDialog)
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, ConfirmDialog, ModalDialog } from "../components/primitives";
import {
  evaluatePluginInstallPolicy,
  type MarketplacePluginRecord,
} from "./marketplaceConfig";

export interface PluginGovernanceDrawerProps {
  readonly plugin: MarketplacePluginRecord | null;
  readonly onInstall: (pluginId: string) => void;
  readonly onToggleEnabled: (pluginId: string, enabled: boolean) => void;
  readonly onRemove: (pluginId: string) => void;
  readonly onRevoke: (pluginId: string) => void;
  readonly onClose: () => void;
}

export function PluginGovernanceDrawer({
  plugin,
  onInstall,
  onToggleEnabled,
  onRemove,
  onRevoke,
  onClose,
}: PluginGovernanceDrawerProps): ReactElement | null {
  const [confirmRevokeOpen, setConfirmRevokeOpen] = useState(false);

  if (!plugin) return null;

  const policy = evaluatePluginInstallPolicy(plugin);
  const gov = plugin.governance;

  const trustBadgeLabel =
    gov.signatureTrust === "verified-official"
      ? "Verified Official"
      : gov.signatureTrust === "verified-community"
        ? "Verified Community"
        : gov.signatureTrust === "local-personal"
          ? "Local Personal"
          : "Unsigned — Review Required";

  const trustTone =
    gov.signatureTrust === "verified-official" ||
    gov.signatureTrust === "verified-community"
      ? "success"
      : gov.signatureTrust === "local-personal"
        ? "info"
        : "warning";

  return (
    <>
      <ModalDialog
        isOpen={Boolean(plugin)}
        title={`Plugin Governance — ${plugin.name} v${plugin.version}`}
        description={`Publisher: ${plugin.publisher} · Runtime: ${gov.runtimeSandbox}`}
        onClose={onClose}
      >
        <div
          style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}
          data-testid="plugin-governance-drawer"
        >
          <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
            <Badge tone={trustTone} label={trustBadgeLabel} />
            <Badge
              tone={gov.abiCompatible ? "success" : "danger"}
              label={`ABI: ${gov.abiVersion}`}
            />
            <Badge
              tone={gov.invocationSupportedByCore ? "success" : "warning"}
              label={
                gov.invocationSupportedByCore
                  ? "WASM Invocation Enabled"
                  : "D-14 Invocation Gated (Inspect-Only)"
              }
            />
            <Badge tone="neutral" label={`State: ${plugin.lifecycleState}`} />
          </div>

          <div className="ky-gov-section">
            <strong>Manifest SHA-256 Digest</strong>
            <code className="ky-gov-digest">{gov.manifestSha256}</code>
          </div>

          <div className="ky-gov-section">
            <strong>Requested Permissions ({gov.permissions.length})</strong>
            <div className="ky-gov-perm-list">
              {gov.permissions.map((perm) => (
                <div key={perm.scope} className="ky-gov-perm-row">
                  <span>
                    <code>{perm.scope}</code> — {perm.description}
                  </span>
                  <Badge
                    tone={
                      perm.risk === "high"
                        ? "danger"
                        : perm.risk === "medium"
                          ? "warning"
                          : "success"
                    }
                    label={`${perm.risk} risk`}
                  />
                </div>
              ))}
            </div>
          </div>

          <div className="ky-gov-section">
            <strong>Sandbox Quota & Isolation Health</strong>
            <div style={{ display: "flex", gap: "var(--space-3)", flexWrap: "wrap", fontSize: "0.8rem" }}>
              <span>CPU budget: {gov.cpuBudgetMs} ms</span>
              <span>Memory ceiling: {gov.memoryCeilingMiB} MiB</span>
              <span>Isolated crashes: {gov.crashCount}</span>
            </div>
          </div>

          <div className="ky-gov-section">
            <strong>Failure & Diagnostic Logs</strong>
            {gov.diagnosticLogs.map((log, idx) => (
              <div
                key={`log-${idx}`}
                style={{ fontFamily: "var(--font-mono)", fontSize: "0.75rem", color: "var(--text-secondary)" }}
              >
                {log}
              </div>
            ))}
          </div>

          {!policy.allowed && policy.blockReason && (
            <div className="ky-workflow-validation-banner" data-valid="false" role="alert">
              <span>Install blocked by Core G-12 policy: {policy.blockReason}</span>
            </div>
          )}

          <div className="ky-modal-form__footer">
            <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
              {plugin.lifecycleState !== "available" &&
                plugin.lifecycleState !== "revoked" &&
                !plugin.isBuiltIn && (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => {
                      onRemove(plugin.pluginId);
                      onClose();
                    }}
                  >
                    Remove
                  </Button>
                )}
              {plugin.lifecycleState !== "revoked" && !plugin.isBuiltIn && (
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => setConfirmRevokeOpen(true)}
                >
                  <ChromeIcon name="shield-security" size={14} />
                  <span>Revoke trust</span>
                </Button>
              )}
            </div>

            <div style={{ display: "flex", gap: "var(--space-2)" }}>
              {plugin.lifecycleState === "available" ? (
                <Button
                  size="sm"
                  variant="primary"
                  disabled={!policy.allowed}
                  onClick={() => {
                    onInstall(plugin.pluginId);
                    onClose();
                  }}
                >
                  {policy.requiresExplicitTrustReview
                    ? "Acknowledge & Install"
                    : "+ Install plugin"}
                </Button>
              ) : plugin.lifecycleState === "installed-enabled" ? (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => onToggleEnabled(plugin.pluginId, false)}
                >
                  Disable
                </Button>
              ) : plugin.lifecycleState === "installed-disabled" ? (
                <Button
                  size="sm"
                  variant="primary"
                  onClick={() => onToggleEnabled(plugin.pluginId, true)}
                >
                  Enable
                </Button>
              ) : null}
              <Button size="sm" variant="ghost" onClick={onClose}>
                Close
              </Button>
            </div>
          </div>
        </div>
      </ModalDialog>

      <ConfirmDialog
        isOpen={confirmRevokeOpen}
        title={`Revoke trust for ${plugin.name}?`}
        explanation="Revoking trust permanently blocks this plugin digest from re-installing or executing (distinct from simple Remove/Uninstall) and records an entry in Activity Audit."
        confirmLabel="Revoke trust"
        onConfirm={() => {
          onRevoke(plugin.pluginId);
          setConfirmRevokeOpen(false);
          onClose();
        }}
        onCancel={() => setConfirmRevokeOpen(false)}
      />
    </>
  );
}
