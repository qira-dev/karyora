import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useDirtyNavigationGuard } from "../../app/useDirtyNavigationGuard";
import { createUnavailableCoreClient } from "../../coreclient";
import { SwitchConnectionDialog } from "../../app/shell/SwitchConnectionDialog";
import { useT } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./ConnectionsPage.module.css";
import { ProviderSetupForm, type ProviderSetupFormHandle } from "../firstRun/ProviderSetupForm";

/**
 * `/settings/connections` (C-14) — the Settings home of the connection
 * surfaces (C-08 add/rename/remove/test/re-authenticate, C-09 switch,
 * C-10 selector), per its frozen rule: "EXACTLY C-08/C-09/C-10's
 * contracts — cited, not restated."
 *
 * Honest states in this no-transport milestone:
 * - The connection list (C-10/C-02's option source) queries the Core
 *   connection contracts and truthfully reports `unavailable` — no
 *   connection records can be listed, and none are invented. The C-10
 *   empty rule (no connections → route to setup) renders as the setup
 *   entry below.
 * - Provider connection setup (C-07) is the one connection type whose
 *   setup fields are frozen (UI-11/UI-12); the form below is the same
 *   component the first-run wizard uses, so re-running setup preserves
 *   existing state (C-06/C-14) and never resets it.
 * - The C-08 operations render their contract-defined availability:
 *   Add opens the provider setup (the frozen field set); Rename and
 *   Remove apply to a connection in the list — with no listable
 *   connection they are disabled with that exact reason (§4.3: no dead
 *   controls; removal stays §8.9-confirmed when it can ever run); Test
 *   of a SAVED connection is likewise disabled while the form's own
 *   `Test connection` (M-25) covers entered values; Re-authenticate
 *   rides the Core session contracts (O-3) and states so.
 * - The switch flow (C-09/C-10) opens from this page exactly as it does
 *   from the connection popover (C-08's two entry points).
 *
 * Secrets (X-6): the form holds the API key only in memory while it is
 * mounted; nothing here writes to browser storage, URLs, logs, or
 * telemetry, and no error surface can echo a secret (all copy is static
 * locale text).
 */
export function ConnectionsPage() {
  const t = useT();
  const navigate = useNavigate();
  const { resumeSetup } = useFirstRun();
  const formRef = useRef<ProviderSetupFormHandle>(null);
  const providerSectionRef = useRef<HTMLDivElement>(null);
  const switchTriggerRef = useRef<HTMLButtonElement>(null);
  const [switchOpen, setSwitchOpen] = useState(false);

  const isDirty = useCallback(() => formRef.current?.isDirty() ?? false, []);
  const guard = useDirtyNavigationGuard({
    isDirty,
    onSave: () => formRef.current?.attemptSave(),
    onDiscard: () => formRef.current?.discard(),
  });

  // The connection list query (C-10/C-14; the exact list semantics are
  // Open — O-1/O-2/O-6). The result renders its honest state; `data`
  // (which no transport can produce today) would render safe display
  // names only — never tokens.
  const [listState, setListState] = useState<"loading" | "unavailable" | "empty" | "records">(
    "loading",
  );
  useEffect(() => {
    let active = true;
    const client = createUnavailableCoreClient();
    void client.query<readonly string[]>("connection-list").then((result) => {
      if (!active) return;
      if (result.kind === "data") {
        setListState(result.data.length === 0 ? "empty" : "records");
      } else {
        setListState("unavailable");
      }
    });
    return () => {
      active = false;
    };
  }, []);

  const openProviderSetup = useCallback(() => {
    providerSectionRef.current?.scrollIntoView({ block: "start" });
    providerSectionRef.current?.focus();
  }, []);

  const resumeFirstRun = useCallback(() => {
    resumeSetup();
    navigate("/");
  }, [resumeSetup, navigate]);

  return (
    <PageLayout>
      <PageHeading>{t("groups.connections.title")}</PageHeading>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.listTitle")}</h2>
        <p className={styles.help}>{t("connections.listIntro")}</p>
        {listState === "loading" ? <p className={styles.help}>{t("states.unavailable")}</p> : null}
        {listState === "unavailable" ? (
          <div className={styles.stateBlock} role="note">
            <p className={styles.stateTitle}>{t("states.unavailable")}</p>
            <p className={styles.stateBody}>{t("connections.listUnavailable")}</p>
          </div>
        ) : null}
        {listState === "empty" ? (
          <p className={styles.help}>{t("provider.emptyState")}</p>
        ) : null}
        {listState === "records" ? (
          // Unreachable without a live transport; the C-10 rule when it
          // ever renders: safe display names only, each row's switch rides
          // the C-09 flow — never a silent switch.
          <p className={styles.help}>{t("connections.listUnavailable")}</p>
        ) : null}
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.operationsTitle")}</h2>
        <p className={styles.help}>{t("connections.operationsIntro")}</p>
        <ul className={styles.operationList}>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} onClick={openProviderSetup}>
              {t("connections.opAddGo")}
            </button>
            <p className={styles.operationReason}>
              <span className={styles.operationName}>{t("connections.opAdd")}: </span>
              {t("connections.opAddNote")}
            </p>
          </li>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} disabled aria-disabled="true">
              {t("connections.opRename")}
            </button>
            <p className={styles.operationReason}>{t("connections.opRenameUnavailable")}</p>
          </li>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} disabled aria-disabled="true">
              {t("connections.opRemove")}
            </button>
            <p className={styles.operationReason}>{t("connections.opRemoveUnavailable")}</p>
          </li>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} disabled aria-disabled="true">
              {t("connections.opTest")}
            </button>
            <p className={styles.operationReason}>{t("connections.opTestUnavailable")}</p>
          </li>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} disabled aria-disabled="true">
              {t("connections.opReauth")}
            </button>
            <p className={styles.operationReason}>{t("connections.opReauthUnavailable")}</p>
          </li>
          <li className={styles.operationItem}>
            <button
              ref={switchTriggerRef}
              type="button"
              className={styles.operationButton}
              onClick={() => setSwitchOpen(true)}
            >
              {t("connections.switchTitle")}
            </button>
            <p className={styles.operationReason}>{t("connections.switchIntro")}</p>
          </li>
        </ul>
      </PageSection>

      <PageSection>
        <div ref={providerSectionRef} tabIndex={-1} className={styles.providerAnchor}>
          <h2 className={styles.sectionTitle}>{t("connections.providerSectionTitle")}</h2>
        </div>
        <p className={styles.help}>{t("connections.providerIntro")}</p>
        <ProviderSetupForm ref={formRef} />
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.firstRunTitle")}</h2>
        <p className={styles.help}>{t("connections.firstRunBody")}</p>
        <button type="button" className={styles.resumeButton} onClick={resumeFirstRun}>
          {t("connections.resumeSetup")}
        </button>
      </PageSection>

      <SwitchConnectionDialog
        open={switchOpen}
        onClose={() => setSwitchOpen(false)}
        returnFocusRef={switchTriggerRef}
      />
      {guard.guardDialog}
    </PageLayout>
  );
}
