import { Bell } from "lucide-react";

import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * In-app notification center entry (C-04; Blueprint §23.1–23.4).
 *
 * Phase 2 freezes the ENTRY with only Core/OS-supported categories and
 * truthful state. No Core notification source is wired in this milestone,
 * so the panel shows the unavailable state — no fabricated events, no
 * mark-read/clear actions (O-6 retention/mutation semantics Open with
 * Core/API). No OS notification permission is requested here and the
 * panel must not imply desktop delivery (UI-34 stays gated).
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
          <p className={styles.popoverBody}>{t("notifications.sourceUnavailable")}</p>
          <p className={styles.popoverBody}>{t("notifications.noOsDelivery")}</p>
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}
