import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useBlocker } from "react-router-dom";

import { Dialog, useDialogLabelId } from "../components/Dialog";
import { useT } from "../i18n/useT";
import styles from "./DirtyGuard.module.css";

/**
 * The dirty-form navigation guard (J-8 / X-3 / §8.13; P2.3 §4.2).
 *
 * Leaving a dirty form prompts ONCE with explicit Keep editing /
 * Discard entries / Save (when the user chooses Save, the underlying
 * save flow runs and renders its REAL outcome — here the truthful
 * not-submitted state — and the user stays on the page). Unchanged data
 * is never re-prompted: the guard exists only while the page reports
 * itself dirty.
 *
 * Two leave paths are covered:
 * - In-app ROUTE navigation, via the router blocker (data router is
 *   required for useBlocker — App uses createBrowserRouter).
 * - In-page exits that would drop the form (the wizard's skip/finish),
 *   via the imperative requestLeave(action).
 *
 * Escape in the guard dialog equals Keep editing — the safe default.
 * Nothing here cancels or mutates a durable Job (none exists in this
 * milestone, and the guard has no such path structurally).
 */

export interface DirtyNavigationGuardOptions {
  /** Live dirty check (stable callback; read at decision time). */
  isDirty: () => boolean;
  /** Run the underlying save flow (J-8's Save action). */
  onSave: () => void;
  /** Discard the unsaved entries (J-8's Discard action). */
  onDiscard: () => void;
}

export interface DirtyNavigationGuard {
  /** The guard dialog to render at page level. */
  guardDialog: ReactNode;
  /** Request an in-page exit; runs the guard first when dirty. */
  requestLeave: (leave: () => void) => void;
}

export function useDirtyNavigationGuard(options: DirtyNavigationGuardOptions): DirtyNavigationGuard {
  const t = useT();
  const [guardOpen, setGuardOpen] = useState(false);
  const labelId = useDialogLabelId();

  const dirtyGetter = useRef(options.isDirty);
  dirtyGetter.current = options.isDirty;
  const discardRef = useRef(options.onDiscard);
  discardRef.current = options.onDiscard;
  const saveRef = useRef(options.onSave);
  saveRef.current = options.onSave;

  // The leave action the guard will complete on Discard: either the
  // router's proceed (route navigation) or the page's own exit action.
  const pendingLeave = useRef<(() => void) | null>(null);
  const routerBlocked = useRef(false);

  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    return (
      dirtyGetter.current() && currentLocation.pathname !== nextLocation.pathname
    );
  });

  useEffect(() => {
    if (blocker.state === "blocked") {
      routerBlocked.current = true;
      pendingLeave.current = () => blocker.proceed();
      setGuardOpen(true);
    }
  }, [blocker]);

  const closeGuard = useCallback(() => {
    setGuardOpen(false);
    pendingLeave.current = null;
    routerBlocked.current = false;
  }, []);

  const keepEditing = useCallback(() => {
    if (routerBlocked.current && blocker.state === "blocked") {
      blocker.reset();
    }
    closeGuard();
  }, [blocker, closeGuard]);

  const discardAndContinue = useCallback(() => {
    discardRef.current();
    const leave = pendingLeave.current;
    closeGuard();
    leave?.();
  }, [closeGuard]);

  const saveAndStay = useCallback(() => {
    saveRef.current();
    // Stay on the page either way: the save flow renders its real
    // outcome in place (J-8: Save runs the underlying save semantics).
    if (routerBlocked.current && blocker.state === "blocked") {
      blocker.reset();
    }
    closeGuard();
  }, [blocker, closeGuard]);

  const requestLeave = useCallback(
    (leave: () => void) => {
      if (dirtyGetter.current()) {
        routerBlocked.current = false;
        pendingLeave.current = leave;
        setGuardOpen(true);
        return;
      }
      leave();
    },
    [],
  );

  const guardDialog = guardOpen ? (
    <Dialog labelId={labelId} title={t("wizard.leaveGuard.title")} open onClose={keepEditing}>
      <div className={styles.body}>
        <p>{t("wizard.leaveGuard.body")}</p>
        <p>{t("wizard.leaveGuard.saveNote")}</p>
        <div className={styles.actions}>
          <button type="button" className={styles.primary} onClick={keepEditing}>
            {t("wizard.leaveGuard.keepEditing")}
          </button>
          <button type="button" className={styles.secondary} onClick={discardAndContinue}>
            {t("wizard.leaveGuard.discard")}
          </button>
          <button type="button" className={styles.secondary} onClick={saveAndStay}>
            {t("wizard.leaveGuard.save")}
          </button>
        </div>
      </div>
    </Dialog>
  ) : null;

  return { guardDialog, requestLeave };
}
