import { useCallback, useEffect, useId, useRef } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from "react";
import { createPortal } from "react-dom";

import { containTab, focusFirstIn } from "./overlayFocus";
import styles from "./Dialog.module.css";
import { useT } from "../i18n/useT";

/**
 * Modal dialog (Blueprint §8.4 + §8.15, via P2.3 §2.5): accessible name
 * and dialog role; modal semantics with focus containment while open;
 * focus moves inside on open and returns to the trigger on close; Escape
 * supported; a visible close/cancel control always present; near
 * full-screen on compact windows with visible close (§28.2).
 */
export interface DialogProps {
  labelId: string;
  title: string;
  open: boolean;
  onClose: () => void;
  /** The element to return focus to on close (usually the trigger). */
  returnFocusRef?: React.RefObject<HTMLElement | null>;
  children: ReactNode;
}

export function Dialog({ labelId, title, open, onClose, returnFocusRef, children }: DialogProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);
  const t = useT();
  const closeLabel = t("actions.close");

  useEffect(() => {
    if (!open) return;
    previouslyFocused.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const panel = panelRef.current;
    if (panel) focusFirstIn(panel);
    return () => {
      const target = returnFocusRef?.current ?? previouslyFocused.current;
      target?.focus();
    };
  }, [open, returnFocusRef]);

  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onClose();
        return;
      }
      const panel = panelRef.current;
      if (panel) {
        containTab(event.nativeEvent, panel);
      }
    },
    [onClose],
  );

  if (!open) return null;

  return createPortal(
    <div className={styles.backdrop}>
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelId}
        className={styles.panel}
        onKeyDown={onKeyDown}
        tabIndex={-1}
      >
        <div className={styles.header}>
          <h2 id={labelId} className={styles.title}>
            {title}
          </h2>
          <button type="button" className={styles.closeButton} onClick={onClose}>
            {closeLabel}
          </button>
        </div>
        <div className={styles.body}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}

/** Convenience hook giving a stable label id for a dialog. */
export function useDialogLabelId(): string {
  const id = useId();
  return `dialog-label-${id}`;
}
