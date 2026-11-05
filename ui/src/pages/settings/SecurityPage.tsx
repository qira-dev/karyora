import { useCallback, useRef, useState } from "react";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import { useCoreConnection } from "../../app/coreConnectionState";
import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./SecurityPage.module.css";

/**
 * `/settings/security` — the security group's page (IMPL-23; the
 * S9-adopted P-S9-02 class's UI half per R-13): the already-frozen route's
 * entry-state page is replaced by the dedicated logout flow's surface —
 * a deliberate user flow, NEVER an ordinary preference control (R-13).
 *
 * The flow's own laws, exactly as the records carry them:
 * - ONE command per explicit confirmation (the §8.12-class dialog: the
 *   scope stated, what is cleared and what stays in view); the pending
 *   guard disables the controls while the command runs; no auto-retry
 *   anywhere (the Open-idempotency law) — a lost response is the S4
 *   unknown the user resolves explicitly.
 * - The failure law (P1.8 §6.2): a typed refusal renders verbatim on the
 *   D-35 envelope and the held token STAYS held — the client never
 *   assumes the session died because the command failed. Only the
 *   Core's confirmed 200 ends the session here; the token discard and
 *   the connection-state transition derive from that real outcome alone.
 * - The client keep/clear set is the recorded laws' set and nothing
 *   beyond (P-S9-03): CLEARED — the held token (in memory only) and the
 *   scoped server-state views (each surface re-queries authoritatively
 *   on its next entry and refuses typed until a session is
 *   re-established); KEPT — local preferences (their own lifetime) and
 *   unsaved, non-sensitive drafts. The enumeration beyond the recorded
 *   laws stays the recorded owners' item — nothing is invented here.
 * - The credential-change revocation (the coded RevokeAll class) has NO
 *   control on this page: its recorded trigger classes are Core-side
 *   (credential change, lock takeover) and no such surface exists — the
 *   honest absence is stated, no control is invented (the no-dead-control
 *   law; the S9 §5 gap 8 class keeps its recorded owner).
 */
export function SecurityPage() {
  const t = useT();
  const { phase, transport, signedOut } = useCoreConnection();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [outcome, setOutcome] = useState<
    { readonly kind: "confirmed" } | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope } | { readonly kind: "unknown" } | null
  >(null);
  const logoutTriggerRef = useRef<HTMLButtonElement>(null);
  const dialogLabelId = useDialogLabelId();

  const sessionHeld = phase === "connected";

  const runLogout = useCallback(() => {
    if (pending) return;
    setPending(true);
    void transport
      .sessionLogout()
      .then((result) => {
        if (result.kind === "data") {
          // The Core's confirmed 200 IS the real outcome: the token is
          // discarded (the holder's law) and the connection state renders
          // the honest no-session posture — derived from this response,
          // never from optimism.
          transport.discardSession();
          signedOut();
          setConfirmOpen(false);
          setOutcome({ kind: "confirmed" });
        } else if (result.kind === "rejection") {
          // The failure law: the typed refusal renders verbatim and the
          // held token stays held.
          setConfirmOpen(false);
          setOutcome({ kind: "refused", envelope: result.envelope });
        } else {
          // The S4 unknown: the Core never confirmed the sign-out — the
          // session is NOT assumed ended; no auto-retry exists.
          setConfirmOpen(false);
          setOutcome({ kind: "unknown" });
        }
      })
      .finally(() => setPending(false));
  }, [pending, transport, signedOut]);

  return (
    <PageLayout>
      <PageHeading>{t("groups.security.title")}</PageHeading>
      <PageSection>
        <h2 className={styles.sectionTitle}>{t("security.logoutSectionTitle")}</h2>
        <p className={styles.help}>{t("security.logoutSectionBody")}</p>

        {sessionHeld ? (
          <div className={styles.actionRow}>
            <button
              type="button"
              className={styles.logoutButton}
              disabled={pending}
              aria-disabled={pending}
              ref={logoutTriggerRef}
              onClick={() => {
                setOutcome(null);
                setConfirmOpen(true);
              }}
            >
              {t("security.logoutAction")}
            </button>
          </div>
        ) : (
          <div className={styles.stateBlock} role="note">
            <p>{t("security.postureNoSession")}</p>
            <p className={styles.actionReason}>
              {phase === "connecting" || phase === "reconnecting"
                ? t("security.postureRechecking")
                : phase === "unknown"
                  ? t("security.postureUnknown")
                  : t("security.postureNoSessionReason")}
            </p>
          </div>
        )}

        {outcome?.kind === "confirmed" ? (
          <RecoveryPanel
            title={t("security.confirmedTitle")}
            body={t("security.confirmedBody")}
            note={t("security.confirmedReestablish")}
          />
        ) : null}

        {outcome?.kind === "refused" ? (
          <RecoveryPanel
            title={t("security.refusedTitle")}
            body={t("security.refusedBody", { code: outcome.envelope.code })}
            note={t("security.refusedNote")}
            referenceId={outcome.envelope.correlationId}
          />
        ) : null}

        {outcome?.kind === "unknown" ? (
          <RecoveryPanel
            title={t("security.unknownTitle")}
            body={t("security.unknownBody")}
            note={t("security.unknownNote")}
          />
        ) : null}

        <p className={styles.help}>{t("security.revokeAllAbsent")}</p>
      </PageSection>

      <Dialog
        labelId={dialogLabelId}
        title={t("security.dialogTitle")}
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        returnFocusRef={logoutTriggerRef}
      >
        <div className={styles.dialogBody}>
          <p>{t("security.dialogScope")}</p>
          <p>{t("security.dialogKeep")}</p>
          <div className={styles.dialogActions}>
            <button
              type="button"
              className={styles.dangerButton}
              disabled={pending}
              aria-disabled={pending}
              onClick={runLogout}
            >
              {t("security.dialogConfirm")}
            </button>
            <button
              type="button"
              className={styles.ghostButton}
              disabled={pending}
              aria-disabled={pending}
              onClick={() => setConfirmOpen(false)}
            >
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      </Dialog>
    </PageLayout>
  );
}
