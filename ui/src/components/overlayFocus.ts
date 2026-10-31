/**
 * Overlay focus utilities (Blueprint §8.15 patterns; P2.4 §2.2).
 *
 * - Focus moves inside the overlay when it opens and returns to the
 *   trigger when it closes (R-6).
 * - Escape closes the overlay; Escape NEVER cancels a durable Job (no
 *   Phase 2 surface has one, and the rule is structural: these helpers
 *   only close the overlay surface — P2.3 §4.8).
 * - Tab/Shift+Tab stay contained while the overlay is open.
 */

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (element) => element.offsetParent !== null || element === document.activeElement,
  );
}

/** Move focus to the first focusable element inside the overlay. */
export function focusFirstIn(container: HTMLElement): void {
  const focusable = getFocusableElements(container);
  (focusable[0] ?? container).focus();
}

/** Contain Tab inside the overlay's focusables. */
export function containTab(event: KeyboardEvent, container: HTMLElement): void {
  if (event.key !== "Tab") return;
  const focusable = getFocusableElements(container);
  if (focusable.length === 0) {
    event.preventDefault();
    return;
  }
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  const active = document.activeElement;
  if (event.shiftKey && (active === first || !container.contains(active))) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (active === last || !container.contains(active))) {
    event.preventDefault();
    first.focus();
  }
}

/** Is the event target inside an editable field or an IME composition?
 *  Shortcut guards (§29; P2.3 §4.7): never intercept editable fields,
 *  IME composition, or assistive-technology text entry. */
export function isInEditableContext(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  return false;
}

/** Is an IME composition currently in progress? */
export function isComposing(event: KeyboardEvent): boolean {
  return event.isComposing || event.keyCode === 229;
}
