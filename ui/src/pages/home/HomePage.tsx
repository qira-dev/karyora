import { Link } from "react-router-dom";

import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./HomePage.module.css";
import { FirstRunWizard } from "../firstRun/FirstRunWizard";

/**
 * `/` — the application entry route (C-06: first-run is entered here on
 * first use and returns to the operational home).
 *
 * Two truthful states:
 * - First use (setup pending): the First-Run Setup wizard IS the
 *   first-use state of this route (C-06). It forces nothing and can be
 *   skipped at any step.
 * - After skip: the honest shell state — no data, no metrics, no
 *   composer, no seeded history, no fabricated capability — with the
 *   first safe action discoverable (resume setup, Settings) and no
 *   tutorial carousel. The operational home (Overview) is a later
 *   package (UIR-4).
 */
export function HomePage() {
  const t = useT();
  const { status, resumeSetup } = useFirstRun();

  if (status === "pending") {
    return <FirstRunWizard />;
  }

  return (
    <PageLayout>
      <PageHeading>{t("title")}</PageHeading>
      <PageSection>
        <h2 className={styles.sectionTitle}>{t("home.heading")}</h2>
        <p>{status === "completed" ? t("wizard.completedDone") : t("wizard.skipDone")}</p>
        <button type="button" className={styles.resumeButton} onClick={resumeSetup}>
          {t("wizard.resume")}
        </button>
      </PageSection>
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
