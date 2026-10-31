import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from "react";
import { createPortal } from "react-dom";

import { focusFirstIn } from "./overlayFocus";
import styles from "./Menu.module.css";

/**
 * Menu-button pattern (Blueprint §8.3 + §8.15, via P2.3 §2.4): Enter/Space
 * opens and exposes the expanded state; focus moves to the first item;
 * arrow keys move among items; Enter activates; Escape closes and focus
 * returns to the trigger. Destructive options would render in a separated
 * group (none exists in Phase 2).
 */

export interface MenuItem {
  readonly id: string;
  readonly label: string;
  readonly disabled?: boolean;
  /** Inline disabled explanation (§8.1: never a tooltip alone). */
  readonly disabledReason?: string;
  readonly onSelect?: () => void;
}

export interface MenuProps {
  /** Render the trigger. Receives open state + toggle + aria wiring. */
  trigger: (state: {
    open: boolean;
    toggle: () => void;
    ariaHasPopup: true;
    ariaExpanded: boolean;
    ariaControls?: string;
  }) => ReactNode;
  /** Accessible name for the menu. */
  label: string;
  items: readonly MenuItem[];
}

const ITEM_SELECTOR = "[role='menuitem']";

export function Menu({ trigger, label, items }: MenuProps) {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLSpanElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

  const close = useCallback(() => setOpen(false), []);
  const toggle = useCallback(() => setOpen((value) => !value), []);

  useEffect(() => {
    if (!open) return undefined;
    const previouslyFocused =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;

    const menu = menuRef.current;
    if (menu) focusFirstIn(menu);

    const onDocumentKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        setOpen(false);
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (menuRef.current?.contains(target) || anchorRef.current?.contains(target)) {
        return;
      }
      setOpen(false);
    };

    document.addEventListener("keydown", onDocumentKeyDown, true);
    document.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      document.removeEventListener("keydown", onDocumentKeyDown, true);
      document.removeEventListener("pointerdown", onPointerDown, true);
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

  const onMenuKeyDown = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const menu = menuRef.current;
    if (!menu) return;
    const items = Array.from(menu.querySelectorAll<HTMLElement>(ITEM_SELECTOR));
    if (items.length === 0) return;
    const currentIndex = items.findIndex((item) => item === document.activeElement);

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        items[(currentIndex + 1 + items.length) % items.length].focus();
        break;
      case "ArrowUp":
        event.preventDefault();
        items[(currentIndex - 1 + items.length) % items.length].focus();
        break;
      case "Home":
        event.preventDefault();
        items[0].focus();
        break;
      case "End":
        event.preventDefault();
        items[items.length - 1].focus();
        break;
      case "Tab":
        // Tab closes the menu and moves on (menu pattern).
        setOpen(false);
        break;
      default:
        break;
    }
  }, []);

  return (
    <span className={styles.anchor} ref={anchorRef}>
      {trigger({ open, toggle, ariaHasPopup: true, ariaExpanded: open, ariaControls: open ? menuId : undefined })}
      {open
        ? createPortal(
            <div
              ref={menuRef}
              id={menuId}
              role="menu"
              aria-label={label}
              tabIndex={-1}
              className={styles.menu}
              onKeyDown={onMenuKeyDown}
            >
              {items.map((item) =>
                item.disabled ? (
                  <div
                    key={item.id}
                    role="menuitem"
                    aria-disabled="true"
                    tabIndex={-1}
                    className={`${styles.item} ${styles.itemDisabled}`}
                  >
                    <span className={styles.itemLabel}>{item.label}</span>
                    {item.disabledReason ? (
                      <span className={styles.disabledReason}>{item.disabledReason}</span>
                    ) : null}
                  </div>
                ) : (
                  <button
                    key={item.id}
                    type="button"
                    role="menuitem"
                    tabIndex={-1}
                    className={styles.item}
                    onClick={() => {
                      item.onSelect?.();
                      close();
                    }}
                  >
                    {item.label}
                  </button>
                ),
              )}
            </div>,
            document.body,
          )
        : null}
    </span>
  );
}
