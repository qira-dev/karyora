import { CircleUser } from "lucide-react";

import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * User / session menu with the session-state display (§4.1 BOTTOM;
 * C-11; P2.7).
 *
 * Truthful only: the session state display reflects the Core's own
 * authorization state — UI visibility is NOT enforcement (MB §12.3).
 * Only Core-reported states are ever shown here (Authentication
 * required / Permission limited / Unknown) and Unknown is never shown
 * as connected. No Core session contract is wired in this milestone
 * (O-3), so the display renders the honest unavailable state and the
 * re-authentication path — a dedicated flow, never an ordinary
 * preference control (§21.10) — stays disabled with its reason: there
 * is no Core-reported session state to act on. No fabricated identity,
 * no fake sign-out, no credential storage (§5.4: credentials are never
 * stored to preserve convenience).
 */
export function SessionMenu({ expanded }: { expanded: boolean }) {
  const t = useT();

  return (
    <Popover
      label={t("session.popoverTitle")}
      alignment="start"
      trigger={({ open, toggle, describedById }) => (
        <button
          type="button"
          className={styles.trigger}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-describedby={describedById}
          title={t("session.label")}
          onClick={toggle}
        >
          <CircleUser aria-hidden="true" size={20} />
          <span className={expanded ? styles.triggerLabel : "visually-hidden"}>
            {t("session.label")}
          </span>
        </button>
      )}
    >
      {(close) => (
        <div>
          <p className={styles.popoverBody}>{t("session.stateUnavailable")}</p>
          <p className={styles.popoverBody}>{t("session.stateRule")}</p>
          <div className={styles.sessionReauth}>
            <button type="button" className={styles.popoverClose} disabled aria-disabled="true">
              {t("session.reauthLabel")}
            </button>
            <p className={styles.popoverNote} id="session-reauth-reason">
              {t("session.reauthUnavailable")}
            </p>
            <p className={styles.popoverNote}>{t("session.reauthNote")}</p>
          </div>
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}
