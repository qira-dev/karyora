import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";

import { containTab, focusFirstIn } from "./overlayFocus";
import styles from "./Popover.module.css";

/**
 * Non-blocking popover (P2.3 §2.5 applied to C-02): keyboard-accessible
 * trigger; focus is contained while open (C-02's recorded field); Escape
 * closes and focus returns to the trigger (R-6); never hover-only; an
 * accessible name; elevation level 2; menus/popovers never cover a focus
 * ring (elevation tokens; §6.10).
 */
export interface PopoverProps {
  /** Render the trigger. Receives open state + handlers. */
  trigger: (state: { open: boolean; toggle: () => void; describedById: string }) => ReactNode;
  /** Accessible name for the popover panel. */
  label: string;
  /** Panel content. Receives a close callback. */
  children: (close: () => void) => ReactNode;
  alignment?: "start" | "end";
}

export function Popover({ trigger, label, children, alignment = "start" }: PopoverProps) {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLSpanElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const panelId = useId();
  const describedById = useId();

  const close = useCallback(() => setOpen(false), []);
  const toggle = useCallback(() => setOpen((value) => !value), []);

  // Escape closes; focus returns to the trigger (handled by restoring
  // focus below); click-outside closes.
  useEffect(() => {
    if (!open) return undefined;
    const previouslyFocused =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;

    const panel = panelRef.current;
    if (panel) focusFirstIn(panel);

    const onDocumentKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        setOpen(false);
        return;
      }
      if (panel) containTab(event, panel);
    };
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (
        panelRef.current?.contains(target) ||
        anchorRef.current?.contains(target)
      ) {
        return;
      }
      setOpen(false);
    };

    document.addEventListener("keydown", onDocumentKeyDown, true);
    document.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      document.removeEventListener("keydown", onDocumentKeyDown, true);
      document.removeEventListener("pointerdown", onPointerDown, true);
      // Focus returns to the trigger on close (R-6), unless focus already
      // moved back into the anchor.
      const anchor = anchorRef.current;
      const active = document.activeElement;
      if (
        previouslyFocused &&
        anchor &&
        !(active instanceof HTMLElement && (anchor.contains(active) || active === anchor))
      ) {
        previouslyFocused.focus();
      }
    };
  }, [open]);

  return (
    <span className={styles.anchor} ref={anchorRef}>
      {trigger({ open, toggle, describedById })}
      {open
        ? createPortal(
            <div
              ref={panelRef}
              id={panelId}
              role="dialog"
              aria-modal="false"
              aria-label={label}
              aria-describedby={describedById}
              tabIndex={-1}
              className={`${styles.panel} ${alignment === "end" ? styles.alignEnd : ""}`}
            >
              {children(close)}
            </div>,
            document.body,
          )
        : null}
    </span>
  );
}
