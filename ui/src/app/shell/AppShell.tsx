import { useCallback, useEffect, useRef, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";

import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { ShortcutHelpDialog } from "./ShortcutHelpDialog";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { useT } from "../../i18n/useT";
import { usePreferences } from "../preferences";
import { useMediaQuery } from "../useMediaQuery";
import styles from "./AppShell.module.css";

/**
 * The application shell (Blueprint §5; P2.6).
 *
 * Composition: sidebar (nav rail / narrow drawer) + top bar (page title,
 * search entry, `+ New`, notification entry) + main content region.
 *
 * Global shortcuts (§29): `Ctrl/Cmd+K` opens search when not typing in an
 * editable field or composing with an IME; `Ctrl/Cmd+,` opens Settings;
 * `?` opens shortcut help with the same guards. Each has a visible
 * alternative. Escape is handled by the overlays themselves and never
 * cancels anything durable.
 *
 * Below the compact breakpoint the sidebar becomes a drawer (§28.1);
 * navigation and keyboard behavior are identical in both variants.
 */
export function AppShell() {
  const t = useT();
  const location = useLocation();
  const navigate = useNavigate();
  const { sidebarExpanded } = usePreferences();
  const isCompact = useMediaQuery("(max-width: 1023px)");

  const [searchOpen, setSearchOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const searchTriggerRef = useRef<HTMLButtonElement>(null);
  const helpTriggerRef = useRef<HTMLButtonElement>(null);
  const drawerTriggerRef = useRef<HTMLButtonElement>(null);
  const drawerLabelId = useDialogLabelId();

  // Route change: close the narrow drawer. (Focus moves to the page's
  // main heading via PageHeading's own focus-on-mount — Blueprint §7.4.)
  useEffect(() => {
    setDrawerOpen(false);
  }, [location.pathname]);

  const openSettings = useCallback(() => {
    navigate("/settings");
  }, [navigate]);

  // Global keyboard shortcuts, guarded (§29 + P2.3 §4.7).
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target;
      const editable =
        target instanceof HTMLElement &&
        (target.isContentEditable ||
          ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
      const composing = event.isComposing || event.keyCode === 229;
      if (editable || composing) return;

      const meta = event.ctrlKey || event.metaKey;
      if (meta && (event.key === "k" || event.key === "K")) {
        event.preventDefault();
        setSearchOpen(true);
        return;
      }
      if (meta && event.key === ",") {
        event.preventDefault();
        openSettings();
        return;
      }
      if (event.key === "?") {
        event.preventDefault();
        setHelpOpen(true);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [openSettings]);

  return (
    <div
      className={styles.shell}
      data-sidebar-collapsed={!isCompact && !sidebarExpanded ? "true" : "false"}
    >
      <a href="#main-content" className={styles.skipLink}>
        {t("skipToContent")}
      </a>
      {!isCompact ? (
        <div
          className={styles.sidebar}
          style={{ width: sidebarExpanded ? "var(--shell-sidebar-expanded)" : "var(--shell-sidebar-collapsed)" }}
        >
          <Sidebar variant="rail" />
        </div>
      ) : null}
      <div className={styles.mainArea}>
        <Topbar
          onOpenDrawer={() => setDrawerOpen(true)}
          drawerTriggerRef={drawerTriggerRef}
          searchOpen={searchOpen}
          onSearchOpenChange={setSearchOpen}
          searchTriggerRef={searchTriggerRef}
          onOpenHelp={() => setHelpOpen(true)}
          helpTriggerRef={helpTriggerRef}
        />
        <main id="main-content" className={styles.main}>
          <Outlet />
        </main>
      </div>

      <ShortcutHelpDialog
        open={helpOpen}
        onClose={() => setHelpOpen(false)}
        returnFocusRef={helpTriggerRef}
      />

      {isCompact ? (
        <Dialog
          labelId={drawerLabelId}
          title={t("groups.main")}
          open={drawerOpen}
          onClose={() => setDrawerOpen(false)}
          returnFocusRef={drawerTriggerRef}
        >
          <div className={styles.drawerBody}>
            <Sidebar variant="drawer" onNavigate={() => setDrawerOpen(false)} />
          </div>
        </Dialog>
      ) : null}
    </div>
  );
}
