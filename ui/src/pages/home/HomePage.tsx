import { Link } from "react-router-dom";

import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "../PageLayout.module.css";

/**
 * `/` — the application entry route, resolving to the operational home.
 *
 * The operational home (Overview) is a later package (UIR-4); first-run
 * setup is P2.7. Until those ship, this page renders the honest shell
 * state: no data, no metrics, no composer, no seeded history, no
 * fabricated capability — with the safe next action (Settings) reachable.
 */
export function HomePage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("title")}</PageHeading>
      <PageSection>
        <h2 className={styles.sectionTitle}>{t("shellStatus")}</h2>
        <p>{t("shellStatusBody")}</p>
      </PageSection>
      <PageSection>
        <h2 className={styles.sectionTitle}>{t("nextActions")}</h2>
        <p>
          {t("nextActionsBody")}{" "}
          <Link to="/settings">{t("items.settings")}</Link>
        </p>
      </PageSection>
    </PageLayout>
  );
}
