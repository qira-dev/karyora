import { CONNECTION_UNKNOWN_ICON } from "./navItems";
import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * Connection indicator + popover (C-02; Blueprint §3.2/§4.1 BOTTOM).
 *
 * Truthful states only: the six Core-reported states include Unknown, and
 * Unknown is shown as Unknown — never as connected. In this milestone no
 * Core connection is wired, so the truthful display is Unknown with the
 * explanation, the connection list is unavailable, and no Switch control
 * is offered (it would be a dead control; switching rides C-08/C-09's
 * unwired contracts).
 *
 * Accessibility (C-02): keyboard-accessible trigger; popover focus is
 * contained; Escape closes with focus return; never hover-only; status is
 * icon + text — never color alone.
 */
export function ConnectionStatus({ expanded }: { expanded: boolean }) {
  const t = useT();
  const Icon = CONNECTION_UNKNOWN_ICON;

  return (
    <Popover
      label={t("connection.popoverTitle")}
      alignment="start"
      trigger={({ open, toggle, describedById }) => (
        <button
          type="button"
          className={styles.trigger}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-describedby={describedById}
          title={t("connection.label")}
          onClick={toggle}
        >
          <Icon aria-hidden="true" size={20} className={styles.stateIconUnknown} />
          <span className={expanded ? styles.triggerLabel : "visually-hidden"}>
            {t("connection.label")}: {t("connection.stateUnknown")}
          </span>
        </button>
      )}
    >
      {(close) => (
        <div>
          <div className={styles.popoverHeaderRow}>
            <span className={styles.statusChip} data-state="unknown">
              <Icon aria-hidden="true" size={16} />
              {t("connection.stateUnknown")}
            </span>
          </div>
          <p className={styles.popoverBody}>{t("connection.unknownExplanation")}</p>
          <p className={styles.popoverBody}>{t("connection.listUnavailable")}</p>
          <p className={styles.popoverBody}>{t("connection.switchUnavailable")}</p>
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}
