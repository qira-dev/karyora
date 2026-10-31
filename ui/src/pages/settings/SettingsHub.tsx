import { Link } from "react-router-dom";

import { useT } from "../../i18n/useT";
import { SETTINGS_GROUPS } from "./settingsGroups";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./SettingsHub.module.css";

/**
 * `/settings` hub (C-12): organize the group entries; navigation only;
 * every rendered group has an entry route; the withheld groups (Team &
 * Access, Storage) are absent — not shown, not dead links.
 */
export function SettingsHub() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("hub.title")}</PageHeading>
      <PageSection>
        <p>{t("hub.subtitle")}</p>
      </PageSection>
      <ul className={styles.groupGrid}>
        {SETTINGS_GROUPS.map((group) => {
          const Icon = group.icon;
          return (
            <li key={group.id}>
              <Link to={group.route} className={styles.groupCard}>
                <span className={styles.groupCardIcon} aria-hidden="true">
                  <Icon size={20} />
                </span>
                <span className={styles.groupCardBody}>
                  <span className={styles.groupCardLabel}>{t(group.labelKey)}</span>
                  <span className={styles.groupCardDescription}>
                    {t(group.descriptionKey)}
                  </span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
    </PageLayout>
  );
}
