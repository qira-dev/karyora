import { Plus } from "lucide-react";

import { Menu } from "../../components/Menu";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * `+ New` creation menu (C-05; Blueprint §4.1 top bar / §10.2).
 *
 * Option source: ONLY Core-supported creation types — start Chat, run a
 * task, import a KnowledgePack/source (X-5; nothing invented). The menu
 * itself creates nothing; each item routes to its owning flow (UIR-4 /
 * UIR-6 packages). Those flows do not ship in this milestone, so each
 * item renders disabled with its inline explanation — no dead controls,
 * no fabricated flows. The §8.15 menu-button pattern is the Menu
 * primitive's keyboard contract.
 */
export function NewMenu() {
  const t = useT();
  const reason = t("newMenu.unavailableReason");

  return (
    <Menu
      label={t("topbar.openNewMenu")}
      items={[
        { id: "start-chat", label: t("newMenu.startChat"), disabled: true, disabledReason: reason },
        { id: "run-task", label: t("newMenu.runTask"), disabled: true, disabledReason: reason },
        {
          id: "import-knowledge",
          label: t("newMenu.importKnowledge"),
          disabled: true,
          disabledReason: reason,
        },
      ]}
      trigger={({ toggle, ariaExpanded, ariaControls }) => (
        <button
          type="button"
          className={styles.newTrigger}
          aria-haspopup="menu"
          aria-expanded={ariaExpanded}
          aria-controls={ariaControls}
          onClick={toggle}
        >
          <Plus aria-hidden="true" size={20} />
          <span>{t("topbar.newMenu")}</span>
        </button>
      )}
    />
  );
}
