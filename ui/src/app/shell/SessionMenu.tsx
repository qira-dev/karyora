import { CircleUser } from "lucide-react";

import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * User / session menu (§4.1 BOTTOM; C-11 session-state display).
 *
 * Truthful only: the session state display reflects the Core's own
 * authorization state. No Core session contract is wired in this
 * milestone, so the menu shows the unavailable state and offers no
 * action — no fabricated identity, no fake sign-out (dead control), no
 * credential storage.
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
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}
