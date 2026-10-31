import { useEffect, useState } from "react";

import { createUnavailableCoreClient } from "../../coreclient";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import { Link } from "react-router-dom";
import styles from "./SwitchConnectionDialog.module.css";

/**
 * The connection switch flow (C-09 + C-10; §3.3; §21.3).
 *
 * The data-separation warning IS the confirmation (C-09): switching
 * changes which Core, workspace, data, and permissions are available;
 * it never migrates a Job, never copies data between Cores, and never
 * mixes caches, cursors, credentials, or events. The connection
 * selector (C-10) is the chooser — it never switches silently; the
 * Core-mediated switch command runs only through C-09's flow.
 *
 * Honest state in this milestone: the connection list (the selector's
 * option source) comes from the Core connection contracts, which are
 * not wired, so the selector renders its honest unavailable state and
 * the confirm control stays disabled with that reason adjacent — no
 * invented connections, no silent switch, no partial application. The
 * C-10 empty rule (no connections → route to setup) renders as the
 * setup link. Escape cancels (safe exit); focus returns to the trigger.
 */
export interface SwitchConnectionDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly returnFocusRef?: React.RefObject<HTMLElement | null>;
}

export function SwitchConnectionDialog({ open, onClose, returnFocusRef }: SwitchConnectionDialogProps) {
  const t = useT();
  const labelId = useDialogLabelId();
  // The selector's option source: the Core connection records (SEL-13).
  // `null` means the records cannot be listed (loading or unavailable —
  // the only reachable state in this milestone). Safe display names
  // only, never tokens; with records present (a live transport, never
  // today) the selector enables and selecting still only initiates the
  // C-09 flow — it never switches silently.
  const [records, setRecords] = useState<readonly string[] | null>(null);

  useEffect(() => {
    if (!open) return;
    let active = true;
    const client = createUnavailableCoreClient();
    void client.query<readonly string[]>("connection-list").then((result) => {
      if (active) setRecords(result.kind === "data" ? result.data : null);
    });
    return () => {
      active = false;
    };
  }, [open]);

  return (
    <Dialog labelId={labelId} title={t("connection.switch.title")} open={open} onClose={onClose} returnFocusRef={returnFocusRef}>
      <div className={styles.body}>
        <h3 className={styles.warningHeading}>{t("connection.switch.warningHeading")}</h3>
        <p className={styles.warningBody}>{t("connection.switch.warningBody")}</p>

        <div className={styles.field}>
          <label htmlFor="switch-connection-select" className={styles.label}>
            {t("connection.switch.selectorLabel")}
          </label>
          <select
            id="switch-connection-select"
            className={styles.select}
            disabled={records === null}
            aria-disabled={records === null ? "true" : undefined}
            aria-describedby="switch-connection-select-reason"
          >
            {records === null ? (
              <option>{t("states.unavailable")}</option>
            ) : (
              records.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))
            )}
          </select>
          <p id="switch-connection-select-reason" className={styles.reason}>
            {t("connection.switch.selectorUnavailable")}
          </p>
        </div>

        <div className={styles.confirmRow}>
          <button
            type="button"
            className={styles.primaryButton}
            disabled={records === null || records.length === 0}
            aria-disabled={records === null || records.length === 0 ? "true" : undefined}
          >
            {t("connection.switch.confirm")}
          </button>
          <p className={styles.reason} role="note">
            {t("connection.switch.confirmUnavailableReason")}
          </p>
        </div>

        <p className={styles.hint}>
          <Link to="/settings/connections" onClick={onClose}>
            {t("connection.switch.setupHint")}
          </Link>
        </p>
      </div>
    </Dialog>
  );
}
