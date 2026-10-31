import { CircleDashed, SlidersHorizontal } from "lucide-react";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT, type TranslationKey } from "../../i18n/useT";
import styles from "./ShortcutHelpDialog.module.css";

/**
 * Shortcut help / discovery (Blueprint §29; P2.4 §6; C-01/P2.6 work list).
 *
 * The help panel documents the shortcuts that are IMPLEMENTED in the
 * current shell (visible alternatives everywhere — no control depends on
 * a shortcut as its only activation path). The full §29 registry and the
 * remap/disable mechanics are a later package (CO-6); the single-key `?`
 * shortcut is scoped so it never fires in editable fields or IME
 * composition. Escape never cancels a durable Job — the note states it.
 */
export const IMPLEMENTED_SHORTCUTS: ReadonlyArray<{ keys: string; actionKey: TranslationKey }> = [
  { keys: "Ctrl / Cmd + K", actionKey: "shortcuts.openSearch" },
  { keys: "Ctrl / Cmd + ,", actionKey: "shortcuts.openSettings" },
  { keys: "?", actionKey: "shortcuts.openShortcutHelp" },
  { keys: "Escape", actionKey: "shortcuts.closeSurface" },
];

export function ShortcutHelpDialog({
  open,
  onClose,
  returnFocusRef,
}: {
  open: boolean;
  onClose: () => void;
  returnFocusRef: React.RefObject<HTMLElement | null>;
}) {
  const t = useT();
  const labelId = useDialogLabelId();

  return (
    <Dialog
      labelId={labelId}
      title={t("shortcutHelp.title")}
      open={open}
      onClose={onClose}
      returnFocusRef={returnFocusRef}
    >
      <div>
        <p className={styles.note}>{t("shortcutHelp.implemented")}</p>
        <table className={styles.table}>
          <thead>
            <tr>
              <th scope="col">{t("shortcutHelp.shortcutColumn")}</th>
              <th scope="col">{t("shortcutHelp.actionColumn")}</th>
            </tr>
          </thead>
          <tbody>
            {IMPLEMENTED_SHORTCUTS.map((shortcut) => (
              <tr key={shortcut.keys}>
                <td>
                  <kbd className={styles.kbd}>{shortcut.keys}</kbd>
                </td>
                <td>{t(shortcut.actionKey)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className={styles.note}>
          <CircleDashed aria-hidden="true" size={16} className={styles.noteIcon} />
          {t("shortcutHelp.remapNote")}
        </p>
        <p className={styles.note}>
          <SlidersHorizontal aria-hidden="true" size={16} className={styles.noteIcon} />
          {t("shortcutHelp.escNote")}
        </p>
      </div>
    </Dialog>
  );
}
