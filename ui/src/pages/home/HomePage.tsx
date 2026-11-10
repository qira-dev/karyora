import { Link } from "react-router-dom";
import { MessageCircle, ListTodo, Network, BookOpen, History, SlidersHorizontal } from "lucide-react";

import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./HomePage.module.css";
import { FirstRunWizard } from "../firstRun/FirstRunWizard";

export function HomePage() {
  const t = useT();
  const { status, resumeSetup } = useFirstRun();

  if (status === "pending") {
    return <FirstRunWizard />;
  }

  return (
    <PageLayout>
      <PageHeading>{t("items.overview")}</PageHeading>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("shellStatus")}</h2>
        <p>{t("shellStatusBody")}</p>
        {status !== "completed" && (
          <button type="button" className={styles.resumeButton} onClick={resumeSetup}>
            {t("wizard.resume")}
          </button>
        )}
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("nextActions")}</h2>
        <div className={styles.cardGrid}>
          <Link to="/chat" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <MessageCircle size={18} style={{ color: "var(--color-primary)" }} />
              {t("items.chat")}
            </span>
            <p className={styles.actionCardDesc}>
              Interactive conversational AI assistant and inquiries.
            </p>
          </Link>

          <Link to="/jobs/new" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <ListTodo size={18} style={{ color: "var(--color-info)" }} />
              {t("items.jobs")}
            </span>
            <p className={styles.actionCardDesc}>
              Submit and monitor durable background execution tasks.
            </p>
          </Link>

          <Link to="/providers" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <Network size={18} style={{ color: "var(--color-warning)" }} />
              {t("items.providersAndModels")}
            </span>
            <p className={styles.actionCardDesc}>
              Inspect foundation model codecs, capabilities, and active connections.
            </p>
          </Link>

          <Link to="/knowledge" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <BookOpen size={18} style={{ color: "var(--color-success)" }} />
              {t("items.knowledge")}
            </span>
            <p className={styles.actionCardDesc}>
              Browse and inspect curated knowledge packs and versioned artifacts.
            </p>
          </Link>

          <Link to="/activity" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <History size={18} style={{ color: "var(--color-text-secondary)" }} />
              {t("items.activityAndAudit")}
            </span>
            <p className={styles.actionCardDesc}>
              Review immutable audit trail, security events, and execution records.
            </p>
          </Link>

          <Link to="/settings" className={styles.actionCard}>
            <span className={styles.actionCardTitle}>
              <SlidersHorizontal size={18} style={{ color: "var(--color-text-primary)" }} />
              {t("items.settings")}
            </span>
            <p className={styles.actionCardDesc}>
              Configure appearance, API credentials, privacy, and system diagnostics.
            </p>
          </Link>
        </div>
      </PageSection>
    </PageLayout>
  );
}
