import { Bell } from "lucide-react";

import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import { NotificationList } from "../../pages/notifications/NotificationList";
import styles from "./shellControls.module.css";

/**
 * In-app notification center entry (C-04; Blueprint §23.1–23.4) — the
 * WIRED surface (IMPL-14; the ADOPTED S6 supply, P-S6-07's own terms:
 * the delivered entry's truthful-unavailable state is replaced by the
 * surface wired to the delivered Core notification machinery, while the
 * unavailable state REMAINS for the genuine unavailability cases — no
 * session, transport refused, Core unavailable — each truthful, never a
 * fake empty list; see NotificationList's state machine).
 *
 * The panel consumes exactly the delivered in-app reads and mutations
 * (the five notification routes, IMPL-13): the grouped list, the
 * Core-supplied unread counts, and the mark read/unread/clear controls.
 * No topbar unread badge exists here — the counts render inside the
 * panel only from the Core counts read (a badge would add an ambient
 * read whose presentation is a later decision; none is invented).
 *
 * FORBIDDEN here (the recorded gates): no OS notification permission is
 * requested and the panel must not imply desktop delivery (decision 6 —
 * P-S6-05; the UI-34 FINAL freeze stays its own Checkpoint-C act); no
 * stream transport (P-S6-04); no mark-all-read; no preference store.
 */
export function NotificationCenter() {
  const t = useT();

  return (
    <Popover
      label={t("notifications.title")}
      alignment="end"
      trigger={({ open, toggle, describedById }) => (
        <button
          type="button"
          className={styles.iconTrigger}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-describedby={describedById}
          title={t("topbar.openNotifications")}
          onClick={toggle}
        >
          <Bell aria-hidden="true" size={20} />
          <span className="visually-hidden">{t("topbar.openNotifications")}</span>
        </button>
      )}
    >
      {(close) => (
        <div>
          <h2 className={styles.popoverTitle}>{t("notifications.title")}</h2>
          <NotificationList variant="popover" onEntryNavigate={close} />
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}
