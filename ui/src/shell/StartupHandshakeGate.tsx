/**
 * Startup Core Handshake Gate (UIR-3.1, Blueprint §4.1, §21.1 #1).
 *
 * Probes the Go Core via `V3CoreClient.probeBootstrapStatus()` on mount, removes the
 * static `#startup-logo-shell` once React mounts, and displays a truthful connectivity
 * status pill or `RecoveryPanel` if Core is offline or requires session authentication —
 * never fabricating a connected state.
 */

import { useEffect, useState, type ReactElement, type ReactNode } from "react";
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
  const [status, setStatus] = useState<CoreBootstrapStatus | null>(null);
  const [dismissedBanner, setDismissedBanner] = useState(false);

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
      {showOfflineOrAuthNotice && (
        <div
          data-testid="startup-handshake-banner"
          style={{
            padding: "var(--space-2) var(--space-4)",
            borderBottom: "1px solid var(--border-subtle)",
            backgroundColor: "var(--bg-surface-1)",
          }}
        >
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
              if (status.status === "auth-required") {
                onOpenConnectionSettings?.();
              } else {
                void runProbe();
              }
            }}
            secondaryActions={
              <button
                type="button"
                className="ky-btn ky-btn--ghost ky-btn--sm"
                onClick={() => setDismissedBanner(true)}
              >
                Continue offline
              </button>
            }
          />
        </div>
      )}
      {children(status)}
    </>
  );
}
