import { useT } from "../../i18n/useT";
import { usePreferences } from "../../app/preferences";
import type { MotionPreference, ThemePreference } from "../../app/preferences";
import { ACCEPTED_LOCALES } from "../../i18n";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./AppearancePage.module.css";

/**
 * `/settings/appearance` (C-13; UI-08 LOCKED; UI-31 immediate-apply).
 *
 * Local presentation preferences ONLY — no Core object, identical in all
 * modes. Immediate-apply reversible local preferences (the UI-31 rule):
 * no save confirmation is implied. Reset restores the documented defaults
 * (§21.2): Light theme, motion following the system, English.
 *
 * - Theme set = the UI-08 LOCKED trio; Light is the first-run default;
 *   System resolves to the active OS appearance and switches live.
 * - Reduced motion = follows system + override (C-13).
 * - Language = only the UI-18 LOCKED set. No translation content exists
 *   yet (the human linguistic review is the owner's standing item), so
 *   the help text says selecting a non-English locale renders English.
 * - Density is WITHHELD (SEL-10 conditional): no compact option renders
 *   until target-size/accessibility checks pass.
 */
export function AppearancePage() {
  const t = useT();
  const { theme, motion, locale, setTheme, setMotion, setLocale, resetToDefaults } =
    usePreferences();

  return (
    <PageLayout>
      <PageHeading>{t("groups.appearance.title")}</PageHeading>
      <PageSection>
        <form className={styles.form} onSubmit={(event) => event.preventDefault()}>
          <div className={styles.field}>
            <label htmlFor="appearance-theme" className={styles.label}>
              {t("appearance.theme")}
            </label>
            <select
              id="appearance-theme"
              className={styles.select}
              value={theme}
              onChange={(event) => setTheme(event.target.value as ThemePreference)}
            >
              <option value="light">{t("appearance.themeLight")}</option>
              <option value="dark">{t("appearance.themeDark")}</option>
              <option value="system">{t("appearance.themeSystem")}</option>
            </select>
            <p className={styles.help}>{t("appearance.themeHelp")}</p>
          </div>

          <div className={styles.field}>
            <label htmlFor="appearance-motion" className={styles.label}>
              {t("appearance.reducedMotion")}
            </label>
            <select
              id="appearance-motion"
              className={styles.select}
              value={motion}
              onChange={(event) => setMotion(event.target.value as MotionPreference)}
            >
              <option value="system">{t("appearance.reducedMotionSystem")}</option>
              <option value="reduce">{t("appearance.reducedMotionReduce")}</option>
              <option value="no-preference">
                {t("appearance.reducedMotionFull")}
              </option>
            </select>
            <p className={styles.help}>{t("appearance.reducedMotionHelp")}</p>
          </div>

          <div className={styles.field}>
            <label htmlFor="appearance-language" className={styles.label}>
              {t("appearance.language")}
            </label>
            <select
              id="appearance-language"
              className={styles.select}
              value={locale}
              onChange={(event) => setLocale(event.target.value as (typeof ACCEPTED_LOCALES)[number])}
            >
              {ACCEPTED_LOCALES.map((value) => (
                <option key={value} value={value}>
                  {LOCALE_LABELS[value] ?? value}
                </option>
              ))}
            </select>
            <p className={styles.help}>{t("appearance.languageHelp")}</p>
          </div>

          <p className={styles.help}>{t("appearance.applied")}</p>
          <p className={styles.help}>{t("appearance.densityWithheld")}</p>

          <button type="button" className={styles.resetButton} onClick={resetToDefaults}>
            {t("appearance.resetConfirm")}
          </button>
        </form>
      </PageSection>
    </PageLayout>
  );
}

/**
 * Display names for the LOCKED locale set. Language names are conventionally
 * shown in their own language; these are fixed identifiers of the accepted
 * set (not user-facing copy requiring translation).
 */
const LOCALE_LABELS: Record<string, string> = {
  en: "English",
  bn: "বাংলা (Bengali)",
  "zh-CN": "中文（简体）",
  es: "Español",
  "pt-BR": "Português (Brasil)",
  ja: "日本語",
  ru: "Русский",
  ko: "한국어",
  de: "Deutsch",
  fr: "Français",
  id: "Bahasa Indonesia",
  tr: "Türkçe",
  vi: "Tiếng Việt",
};
