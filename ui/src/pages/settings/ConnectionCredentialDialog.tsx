import { useEffect, useRef, useState } from "react";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import type { WireConnectionView } from "../../coreclient/httpLocal/wire";
import styles from "./ConnectionsPage.module.css";

/**
 * The credential re-entry dialog (M-26 rotate / M-27 re-authenticate —
 * the rotate path's cousin): the replacement key is a NEW empty
 * write-only field — never pre-filled, never the stored key, never
 * re-displayed after it is submitted (P3.3 §4.4's replacement-key
 * discipline; the P1.3 lifetime-6 rule dominates every other state).
 *
 * The confirmation states the scope and the effects BEFORE the command
 * (§4.4: rotation confirms the affected model and the possible
 * interruption of in-flight requests, exactly as the Core reports them
 * — no invented "seamless rotation"). Escape closes and mutates nothing;
 * focus returns to the trigger.
 */
export interface ConnectionCredentialDialogProps {
  readonly open: boolean;
  /** `rotate` (M-26) or `reauthenticate` (M-27) — the dialog copy states
   *  which act is confirmed. */
  readonly kind: "rotate" | "reauthenticate";
  readonly connection: WireConnectionView | null;
  readonly onConfirm: (connectionID: string, secret: string) => void;
  readonly onClose: () => void;
  readonly returnFocusRef?: React.RefObject<HTMLElement | null>;
  /** True while the command is in flight (the dialog's confirm stays
   *  disabled — no duplicate submit). */
  readonly pending: boolean;
}

export function ConnectionCredentialDialog({
  open,
  kind,
  connection,
  onConfirm,
  onClose,
  returnFocusRef,
  pending,
}: ConnectionCredentialDialogProps) {
  const t = useT();
  const labelId = useDialogLabelId();
  // The replacement key lives ONLY in this dialog's local state while it
  // is open (X-6): it never enters app state, storage, URLs, or logs, and
  // the dialog discards it on close.
  const [secret, setSecret] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      // A fresh empty write-only field per opening — never a restored
      // value (the value is never stored anywhere).
      setSecret("");
      inputRef.current?.focus();
    }
  }, [open]);

  const confirm = () => {
    if (secret.length === 0 || connection === null || pending) return;
    onConfirm(connection.record.connection_id, secret);
    setSecret("");
  };

  const titleKey = kind === "rotate" ? "connections.rotateTitle" : "connections.reauthTitle";
  const bodyKey = kind === "rotate" ? "connections.rotateBody" : "connections.reauthBody";

  return (
    <Dialog labelId={labelId} title={t(titleKey)} open={open} onClose={onClose} returnFocusRef={returnFocusRef}>
      <form
        className={styles.dialogBody}
        onSubmit={(event) => {
          event.preventDefault();
          confirm();
        }}
      >
        <p className={styles.dialogBodyText}>{t(bodyKey, { connectionId: connection?.record.connection_id ?? "" })}</p>
        <p className={styles.dialogEffects}>
          {kind === "rotate"
            ? t("connections.rotateEffects", { model: connection?.record.model_id ?? "" })
            : t("connections.reauthEffects", { model: connection?.record.model_id ?? "" })}
        </p>
        <div className={styles.dialogField}>
          <label htmlFor="connection-credential-input" className={styles.dialogLabel}>
            {t("connections.credentialLabel")}
          </label>
          <input
            ref={inputRef}
            id="connection-credential-input"
            className={styles.dialogInput}
            type="password"
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
          <p className={styles.dialogHint}>{t("connections.credentialHint")}</p>
        </div>
        <div className={styles.dialogActions}>
          <button
            type="submit"
            className={styles.primaryButton}
            disabled={secret.length === 0 || pending}
            aria-disabled={secret.length === 0 || pending ? "true" : undefined}
          >
            {kind === "rotate" ? t("connections.rotateConfirm") : t("connections.reauthConfirm")}
          </button>
          <button type="button" className={styles.secondaryButton} onClick={onClose}>
            {t("connections.cancel")}
          </button>
        </div>
        <p className={styles.dialogHint}>{t("connections.dialogDurableNote")}</p>
      </form>
    </Dialog>
  );
}
