import { useT } from "../../i18n/useT";
import { IMPLEMENTED_SHORTCUTS } from "../../app/shell/ShortcutHelpDialog";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./ShortcutsPage.module.css";

/**
 * `/settings/shortcuts` (C-18; §29): the shortcut DISCOVERY entry.
 *
 * The shortcuts documented here are the ones implemented in the current
 * shell — nothing aspirational. The full shortcut registry and the
 * remap/disable controls ship in a later package (CO-6 Open); the note
 * says so. Shortcut preferences are a local per-device preference
 * (lifetime 4).
 */
export function ShortcutsPage() {
  const t = useT();

  return (
    <PageLayout>
      <PageHeading>{t("groups.shortcuts.title")}</PageHeading>
      <PageSection>
        <p>{t("shortcuts.body")}</p>
        <table className={styles.table}>
          <thead>
            <tr>
              <th scope="col">{t("shortcutHelp.shortcutColumn")}</th>
              <th scope="col">{t("shortcutHelp.actionColumn")}</th>
            </tr>
          </thead>
          <tbody>
            {IMPLEMENTED_SHORTCUTS.map((shortcut) => (
              <tr key={shortcut.keys}>
                <td>
                  <kbd className={styles.kbd}>{shortcut.keys}</kbd>
                </td>
                <td>{t(shortcut.actionKey)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className={styles.note}>{t("shortcutHelp.remapNote")}</p>
        <p className={styles.note}>{t("shortcuts.localNote")}</p>
      </PageSection>
    </PageLayout>
  );
}
