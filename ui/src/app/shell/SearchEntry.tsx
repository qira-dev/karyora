import { useEffect, useRef } from "react";
import { Search } from "lucide-react";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import styles from "./SearchEntry.module.css";

/**
 * Global command-palette / search ENTRY (C-03; Blueprint §4.3/§22).
 *
 * The Phase 2 freeze is the shell search entry: a top-bar control plus
 * `Ctrl/Cmd+K` when not typing in an editable field. The full six-group
 * palette is a later package (UIR-8) and needs the permission-filtered
 * Core search contract, which is not wired in this milestone — so the
 * opened entry renders its honest unavailable state. No results are ever
 * fabricated; query text is not persisted anywhere (no URLs, storage, or
 * telemetry).
 */
export function SearchEntry({
  open,
  onOpenChange,
  triggerRef,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const inputRef = useRef<HTMLInputElement>(null);

  // Focus the input on open (C-03: open the entry, focus the input).
  useEffect(() => {
    if (open) {
      // The dialog moves focus to its first focusable; that is the close
      // button, so re-focus the input right after mount.
      const timer = window.setTimeout(() => inputRef.current?.focus(), 0);
      return () => window.clearTimeout(timer);
    }
    return undefined;
  }, [open]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={styles.searchTrigger}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
        title={t("topbar.openSearch")}
      >
        <Search aria-hidden="true" size={20} />
        <span className={styles.searchTriggerLabel}>{t("topbar.search")}</span>
        <kbd className={styles.kbd} aria-hidden="true">
          Ctrl K
        </kbd>
      </button>
      <Dialog
        labelId={labelId}
        title={t("search.title")}
        open={open}
        onClose={() => onOpenChange(false)}
        returnFocusRef={triggerRef}
      >
        <div className={styles.body}>
          <label className={styles.label} htmlFor="search-entry-input">
            {t("search.inputLabel")}
          </label>
          <input
            id="search-entry-input"
            ref={inputRef}
            type="text"
            className={styles.input}
            disabled
          />
          <p className={styles.unavailable}>{t("search.unavailable")}</p>
          <p className={styles.hint}>{t("search.shortcutHint")}</p>
        </div>
      </Dialog>
    </>
  );
}
