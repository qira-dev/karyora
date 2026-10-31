import { useT, type TranslationKey } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";

/**
 * The honest entry state for a settings group whose screens and
 * Core-backed controls ship in a later package. Nothing is invented: no
 * fake data, no dead controls, no Core behavior implied. The page states
 * exactly what is (and is not) wired in this milestone.
 */
export function EntryStatePage({
  titleKey,
  bodyKeys,
}: {
  titleKey: TranslationKey;
  bodyKeys: readonly TranslationKey[];
}) {
  const t = useT();
  return (
    <PageLayout>
      <PageHeading>{t(titleKey)}</PageHeading>
      <PageSection>
        <p className="visually-hidden">{t("entryNotice.title")}</p>
        {bodyKeys.map((key) => (
          <p key={key}>{t(key)}</p>
        ))}
        <p>{t("entryNotice.body")}</p>
      </PageSection>
    </PageLayout>
  );
}
