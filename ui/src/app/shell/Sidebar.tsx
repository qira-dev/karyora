import { NavLink } from "react-router-dom";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

import { useT } from "../../i18n/useT";
import { BOTTOM_ITEMS, NAV_GROUPS } from "./navItems";
import { ConnectionStatus } from "./ConnectionStatus";
import { SessionMenu } from "./SessionMenu";
import { usePreferences } from "../preferences";
import styles from "./Sidebar.module.css";

/**
 * The app navigation rail (C-01; Blueprint §4.1/§4.3/§5.2).
 *
 * - Expanded: visible text labels. Collapsed: accessible name + tooltip
 *   per item; the decorative redundant icon inside a labeled control is
 *   hidden from assistive technology (aria-hidden) while the accessible
 *   name stays on the control.
 * - The active item has a visible indicator bar AND a text treatment
 *   (font weight) AND aria-current="page" — color is not the only signal.
 * - Keyboard-operable throughout (semantic links/buttons); focus-visible
 *   ring from the global token.
 * - Collapsed/expanded state persists per user/device (local preference).
 * - Disabled items (later packages) carry an inline explanation — never a
 *   tooltip alone; no dead controls.
 */

type NavItem = (typeof NAV_GROUPS)[number]["items"][number];

export function Sidebar({
  variant,
  onNavigate,
}: {
  variant: "rail" | "drawer";
  onNavigate?: () => void;
}) {
  const { sidebarExpanded, setSidebarExpanded } = usePreferences();
  const t = useT();
  const expanded = variant === "drawer" ? true : sidebarExpanded;

  return (
    <nav aria-label={t("groups.main")} className={styles.nav}>
      {variant === "rail" ? (
        <div className={styles.topCluster}>
          <button
            type="button"
            className={styles.collapseToggle}
            onClick={() => setSidebarExpanded(!sidebarExpanded)}
            title={sidebarExpanded ? t("collapse") : t("expand")}
          >
            {sidebarExpanded ? (
              <PanelLeftClose aria-hidden="true" size={20} />
            ) : (
              <PanelLeftOpen aria-hidden="true" size={20} />
            )}
            <span className={expanded ? styles.controlLabel : "visually-hidden"}>
              {sidebarExpanded ? t("collapse") : t("expand")}
            </span>
          </button>
        </div>
      ) : null}
      <div className={styles.groups}>
        {NAV_GROUPS.map((group) => (
          <div key={group.id} className={styles.group}>
            <h2 className={expanded ? styles.groupLabel : "visually-hidden"}>
              {t(group.labelKey)}
            </h2>
            <ul className={styles.items}>
              {group.items.map((item) => (
                <li key={item.id}>
                  <NavItemControl item={item} expanded={expanded} onNavigate={onNavigate} />
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className={styles.bottomCluster}>
        <ConnectionStatus expanded={expanded} />
        {BOTTOM_ITEMS.map((item) => (
          <NavItemControl key={item.id} item={item} expanded={expanded} onNavigate={onNavigate} />
        ))}
        <SessionMenu expanded={expanded} />
      </div>
    </nav>
  );
}

function NavItemControl({
  item,
  expanded,
  onNavigate,
}: {
  item: NavItem;
  expanded: boolean;
  onNavigate?: () => void;
}) {
  const t = useT();
  const Icon = item.icon;
  const label = t(item.labelKey);

  if (item.disabled) {
    return (
      <button
        type="button"
        className={styles.item}
        data-disabled="true"
        aria-disabled="true"
        title={`${label} ${t("unavailableSuffix")}`}
      >
        <Icon aria-hidden="true" size={20} className={styles.itemIcon} />
        <span className={expanded ? styles.itemLabel : "visually-hidden"}>{label}</span>
        {!expanded ? null : (
          <span className={styles.disabledNote}>{t("unavailableReason")}</span>
        )}
        <span className="visually-hidden">{t("unavailableReason")}</span>
      </button>
    );
  }

  // Active destinations (this milestone: Settings).
  return (
    <NavLink
      to={item.route ?? "/"}
      className={({ isActive }) => `${styles.item} ${isActive ? styles.itemActive : ""}`}
      onClick={onNavigate}
      title={expanded ? undefined : label}
    >
      <Icon aria-hidden="true" size={20} className={styles.itemIcon} />
      <span className={expanded ? styles.itemLabel : "visually-hidden"}>{label}</span>
    </NavLink>
  );
}
