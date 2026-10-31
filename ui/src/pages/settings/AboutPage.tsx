import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./AboutPage.module.css";

/**
 * `/settings/about` (C-21): version/license metadata as the local build
 * supplies it. Copy-only; no mutation. The product name Karyora stands
 * locked; the wordmark is DEFERRED (UI-33) — the name renders as a
 * neutral text placeholder with no logotype, glyph, or stylization. No
 * "newest/secure" claim is made (§21.9).
 */

declare const __APP_VERSION__: string;

export function AboutPage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("groups.about.title")}</PageHeading>
      <PageSection>
        <dl className={styles.metaList}>
          <div className={styles.metaRow}>
            <dt>{t("appName")}</dt>
            <dd>Karyora</dd>
          </div>
          <div className={styles.metaRow}>
            <dt>{t("about.appVersion")}</dt>
            <dd>
              <span className={styles.mono}>{__APP_VERSION__}</span>
            </dd>
          </div>
          <div className={styles.metaRow}>
            <dt>{t("about.license")}</dt>
            <dd>{t("about.licenseValue")}</dd>
          </div>
        </dl>
        <p>{t("about.body")}</p>
        <p className={styles.note}>{t("about.versionDisclaimer")}</p>
      </PageSection>
    </PageLayout>
  );
}
