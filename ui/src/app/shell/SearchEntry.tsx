import { useEffect, useRef, useState } from "react";
import { Search } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import styles from "./SearchEntry.module.css";

/**
 * Global command-palette / search ENTRY (C-03; Blueprint §4.3/§22).
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
  const navigate = useNavigate();
  const [query, setQuery] = useState("");

  const destinations = [
    { path: "/", title: t("items.overview"), category: "Main" },
    { path: "/chat", title: t("items.chat"), category: "Main" },
    { path: "/jobs", title: t("items.jobs"), category: "Main" },
    { path: "/knowledge", title: t("items.knowledge"), category: "Main" },
    { path: "/memory", title: t("items.memory"), category: "Main" },
    { path: "/context", title: t("items.context"), category: "Main" },
    { path: "/workflows", title: t("items.workflows"), category: "Main" },
    { path: "/providers", title: t("items.providersAndModels"), category: "Build & connect" },
    { path: "/plugins", title: t("items.plugins"), category: "Extensions" },
    { path: "/activity", title: t("items.activityAndAudit"), category: "Operate" },
    { path: "/help", title: t("items.helpAndDocumentation"), category: "More" },
    { path: "/settings", title: t("items.settings"), category: "Operate" },
  ];

  const filtered = destinations.filter((d) =>
    query === ""
      ? true
      : d.title.toLowerCase().includes(query.toLowerCase()) ||
        d.path.toLowerCase().includes(query.toLowerCase())
  );

  // Focus the input on open (C-03: open the entry, focus the input).
  useEffect(() => {
    if (open) {
      setQuery("");
      const timer = window.setTimeout(() => inputRef.current?.focus(), 0);
      return () => window.clearTimeout(timer);
    }
    return undefined;
  }, [open]);

  const selectDestination = (path: string) => {
    onOpenChange(false);
    navigate(path);
  };

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
            placeholder="Jump to destination..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && filtered.length > 0) {
                selectDestination(filtered[0].path);
              }
            }}
          />
          <div className={styles.results} role="listbox">
            {filtered.map((item) => (
              <button
                key={item.path}
                type="button"
                className={styles.resultItem}
                onClick={() => selectDestination(item.path)}
              >
                <span>{item.title}</span>
                <span className={styles.categoryTag}>{item.category} ({item.path})</span>
              </button>
            ))}
          </div>
          <p className={styles.hint}>{t("search.shortcutHint")}</p>
        </div>
      </Dialog>
    </>
  );
}
