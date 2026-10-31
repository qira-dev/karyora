import { useRef, useState } from "react";

import { CONNECTION_UNKNOWN_ICON } from "./navItems";
import { SwitchConnectionDialog } from "./SwitchConnectionDialog";
import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * Connection indicator + popover (C-02; Blueprint §3.2/§4.1 BOTTOM).
 *
 * Truthful states only: the six Core-reported states include Unknown, and
 * Unknown is shown as Unknown — never as connected. In this milestone no
 * Core connection is wired, so the truthful display is Unknown with the
 * explanation, and the connection list is unavailable.
 *
 * P2.7: the `Switch connection` entry now opens the C-09/C-10 switch
 * flow (a dedicated flow, not a silent selector — §21.10). The flow
 * itself renders its honest states: the data-separation warning, the
 * selector's unavailable option source, and a confirm control disabled
 * with its reason. Nothing switches, because nothing can: the switch is
 * a Core-mediated context change and the connection contracts are not
 * wired.
 *
 * Accessibility (C-02): keyboard-accessible trigger; popover focus is
 * contained; Escape closes with focus return; never hover-only; status is
 * icon + text — never color alone.
 */
export function ConnectionStatus({ expanded }: { expanded: boolean }) {
  const t = useT();
  const Icon = CONNECTION_UNKNOWN_ICON;
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [switchOpen, setSwitchOpen] = useState(false);

  return (
    <>
      <Popover
        label={t("connection.popoverTitle")}
        alignment="start"
        trigger={({ open, toggle, describedById }) => (
          <button
            ref={triggerRef}
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
            <div className={styles.popoverActions}>
              <button
                type="button"
                className={styles.popoverClose}
                onClick={() => {
                  close();
                  setSwitchOpen(true);
                }}
              >
                {t("connection.switch.title")}
              </button>
              <button type="button" className={styles.popoverClose} onClick={close}>
                {t("actions.close")}
              </button>
            </div>
          </div>
        )}
      </Popover>
      <SwitchConnectionDialog
        open={switchOpen}
        onClose={() => setSwitchOpen(false)}
        returnFocusRef={triggerRef}
      />
    </>
  );
}
