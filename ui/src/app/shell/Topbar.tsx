import type { RefObject } from "react";
import { useLocation } from "react-router-dom";
import { CircleHelp, Menu as MenuIcon } from "lucide-react";

import { useT, type TranslationKey } from "../../i18n/useT";
import { NewMenu } from "./NewMenu";
import { NotificationCenter } from "./NotificationCenter";
import { SearchEntry } from "./SearchEntry";
import styles from "./Topbar.module.css";

/**
 * The top bar (Blueprint §5.2/§5.3, 64 px): current page title, the
 * search entry, the `+ New` creation menu, the notification entry, and
 * the shortcut-help trigger (the visible alternative for the `?`
 * shortcut). The product name is a NEUTRAL TEXT placeholder of the locked
 * name — no wordmark or brand asset (UI-33 deferred).
 */

interface TopbarProps {
  onOpenDrawer: () => void;
  drawerTriggerRef: RefObject<HTMLButtonElement | null>;
  searchOpen: boolean;
  onSearchOpenChange: (open: boolean) => void;
  searchTriggerRef: RefObject<HTMLButtonElement | null>;
  onOpenHelp: () => void;
  helpTriggerRef: RefObject<HTMLButtonElement | null>;
}

/** Route → title resource key. */
function routeTitleKey(pathname: string): TranslationKey {
  if (pathname === "/" || pathname === "") return "title";
  const settingsGroup = pathname.match(/^\/settings\/([a-z-]+)$/);
  if (settingsGroup) {
    const group = settingsGroup[1];
    const keyByGroup: Record<string, TranslationKey> = {
      appearance: "groups.appearance.title",
      connections: "groups.connections.title",
      privacy: "groups.privacy.title",
      security: "groups.security.title",
      notifications: "groups.notifications.title",
      shortcuts: "groups.shortcuts.title",
      diagnostics: "groups.diagnostics.title",
      updates: "groups.updates.title",
      about: "groups.about.title",
    };
    const key = keyByGroup[group];
    if (key) return key;
  }
  if (pathname.startsWith("/settings")) return "hub.title";
  return "title";
}

export function Topbar({
  onOpenDrawer,
  drawerTriggerRef,
  searchOpen,
  onSearchOpenChange,
  searchTriggerRef,
  onOpenHelp,
  helpTriggerRef,
}: TopbarProps) {
  const t = useT();
  const location = useLocation();
  const title = t(routeTitleKey(location.pathname));

  return (
    <header className={styles.topbar}>
      <div className={styles.left}>
        <button
          ref={drawerTriggerRef}
          type="button"
          className={`${styles.iconButton} ${styles.drawerOnly}`}
          aria-haspopup="dialog"
          onClick={onOpenDrawer}
          title={t("openNavigation")}
        >
          <MenuIcon aria-hidden="true" size={20} />
          <span className="visually-hidden">{t("openNavigation")}</span>
        </button>
        <span className={styles.productName}>{t("appName")}</span>
        <span className={styles.separator} aria-hidden="true" />
        <span className={styles.pageTitle}>{title}</span>
      </div>
      <div className={styles.right}>
        <SearchEntry
          open={searchOpen}
          onOpenChange={onSearchOpenChange}
          triggerRef={searchTriggerRef}
        />
        <NewMenu />
        <NotificationCenter />
        <button
          ref={helpTriggerRef}
          type="button"
          className={styles.iconButton}
          onClick={onOpenHelp}
          title={t("shortcutHelp.openHelp")}
        >
          <CircleHelp aria-hidden="true" size={20} />
          <span className="visually-hidden">{t("shortcutHelp.openHelp")}</span>
        </button>
      </div>
    </header>
  );
}
