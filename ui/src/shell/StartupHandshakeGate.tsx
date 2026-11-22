/**
 * Startup Core Handshake Gate (UIR-3.1, Blueprint §4.1, §21.1 #1).
 *
 * Probes the Go Core via `V3CoreClient.probeBootstrapStatus()` on mount and exposes
 * an honest, non-intrusive Core status pill in the bottom-right corner with one-click
 * expandable `RecoveryPanel` details so the ZCode workspace shell remains spotless.
 */

import { useEffect, useState, type ReactElement, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { RecoveryPanel } from "../components/feedback";
import type { CoreBootstrapStatus, V3CoreClient } from "../coreclient";
import { toRecoveryPanelDescriptor } from "../coreclient";

export interface StartupHandshakeGateProps {
  readonly coreClient: V3CoreClient;
  readonly onBootstrapResolved?: (status: CoreBootstrapStatus) => void;
  readonly onOpenConnectionSettings?: () => void;
  readonly children: (status: CoreBootstrapStatus | null) => ReactNode;
}

export function StartupHandshakeGate({
  coreClient,
  onBootstrapResolved,
  onOpenConnectionSettings,
  children,
}: StartupHandshakeGateProps): ReactElement {
  const { t } = useTranslation();
  const [status, setStatus] = useState<CoreBootstrapStatus | null>(null);
  const [dismissedBanner, setDismissedBanner] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);

  const runProbe = async () => {
    try {
      const result = await coreClient.probeBootstrapStatus();
      setStatus(result);
      onBootstrapResolved?.(result);
    } catch {
      const offlineStatus: CoreBootstrapStatus = {
        connectionId: "local-core",
        mode: "local",
        status: "offline",
        coreVersion: "unknown",
        sessionAuthenticated: false,
      };
      setStatus(offlineStatus);
      onBootstrapResolved?.(offlineStatus);
    }
  };

  useEffect(() => {
    void runProbe();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [coreClient]);

  const showOfflineOrAuthNotice =
    !dismissedBanner &&
    status !== null &&
    (status.status === "offline" || status.status === "auth-required");

  const recoveryDescriptor =
    status?.status === "auth-required"
      ? toRecoveryPanelDescriptor({
          code: "AUTHZ_DENIED",
          retryable: false,
          correlationId: "bootstrap-probe",
          data: { sessionReason: "SESSION_INVALID" },
        })
      : toRecoveryPanelDescriptor({
          code: "STORAGE_UNAVAILABLE",
          retryable: true,
          correlationId: "bootstrap-probe",
          data: {},
        });

  return (
    <>
      {children(status)}

      {showOfflineOrAuthNotice && !detailsOpen && (
        <div
          data-testid="startup-handshake-banner"
          className="ky-core-status-pill"
          role="status"
        >
          <span
            aria-hidden="true"
            style={{
              width: "7px",
              height: "7px",
              borderRadius: "9999px",
              backgroundColor:
                status.status === "auth-required"
                  ? "var(--warning-text)"
                  : "var(--text-muted)",
            }}
          />
          <span>
            {status.status === "auth-required"
              ? t("ui.coreStatus.authRequiredPill", { defaultValue: "Core auth required" })
              : t("ui.coreStatus.offlinePill", { defaultValue: "Core offline (local preview)" })}
          </span>
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            style={{ minHeight: "22px", paddingInline: "6px", fontSize: "0.74rem" }}
            onClick={() => void runProbe()}
          >
            {t("ui.coreStatus.retryHandshake", { defaultValue: "Retry" })}
          </button>
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            style={{ minHeight: "22px", paddingInline: "5px", fontSize: "0.74rem" }}
            aria-label="Core connection details"
            onClick={() => setDetailsOpen(true)}
          >
            Details
          </button>
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            style={{ minHeight: "22px", paddingInline: "5px", fontSize: "0.74rem" }}
            aria-label="Dismiss status notice"
            onClick={() => setDismissedBanner(true)}
          >
            ×
          </button>
        </div>
      )}

      {showOfflineOrAuthNotice && detailsOpen && (
        <div className="ky-modal-backdrop" role="presentation">
          <div className="ky-modal-dialog">
            <RecoveryPanel
              descriptor={recoveryDescriptor}
              titleText={
                status.status === "auth-required"
                  ? "Core session authentication required"
                  : "Local Core is currently unreachable"
              }
              explanationText={
                status.status === "auth-required"
                  ? "The local Core answered, but requires an origin-bound session exchange before loading authoritative workspace data."
                  : "Karyora could not reach the local Go Core on this origin. Workspace chrome remains browsable in offline preview mode while drafts stay protected."
              }
              primaryActionLabel={
                status.status === "auth-required"
                  ? "Open Core Connection Settings"
                  : "Retry Core Handshake"
              }
              onPrimaryAction={() => {
                if (status.status === "auth-required" && onOpenConnectionSettings) {
                  setDetailsOpen(false);
                  onOpenConnectionSettings();
                } else {
                  void runProbe();
                }
              }}
              secondaryActions={
                <button
                  type="button"
                  className="ky-btn ky-btn--secondary ky-btn--sm"
                  onClick={() => setDetailsOpen(false)}
                >
                  Close
                </button>
              }
            />
          </div>
        </div>
      )}
    </>
  );
}
