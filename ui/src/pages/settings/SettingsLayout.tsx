import { NavLink, Outlet } from "react-router-dom";

import { useT } from "../../i18n/useT";
import { SETTINGS_GROUPS } from "./settingsGroups";
import styles from "./SettingsLayout.module.css";

/**
 * `/settings` layout (C-12): the vertical settings-nav pattern — the
 * group nav scrolls vertically and is keyboard/tabs accessible; the hub
 * renders as the index; each group is a route (no dead link: every
 * rendered group has an entry route in the frozen route set).
 */
export function SettingsLayout() {
  const t = useT();

  return (
    <div className={styles.layout}>
      <nav aria-label={t("hub.groupsNavLabel")} className={styles.groupNav}>
        <ul className={styles.groupList}>
          {SETTINGS_GROUPS.map((group) => {
            const Icon = group.icon;
            return (
              <li key={group.id}>
                <NavLink
                  to={group.route}
                  end
                  className={({ isActive }) =>
                    `${styles.groupLink} ${isActive ? styles.groupLinkActive : ""}`
                  }
                >
                  <Icon aria-hidden="true" size={18} />
                  <span>{t(group.labelKey)}</span>
                </NavLink>
              </li>
            );
          })}
        </ul>
      </nav>
      <div className={styles.groupPanel}>
        <Outlet />
      </div>
    </div>
  );
}
