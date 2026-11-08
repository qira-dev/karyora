import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, Plugin, PluginState } from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./PluginDetailPage.module.css";
import { CopyIdButton, pluginLifecycleChip } from "./pluginStates";

/**
 * `/plugins/:pluginId` — the ONE canonical plugin-detail route (B5-U1;
 * the DE-0 queue row 9, the P4.1 G1–G10 rows against the now-live B5
 * adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/plugins/{id}` (the inspect member, B5-A1 §2.1 #3): the
 *   installed plugin record with its Core-returned manifest and package
 *   verification states. THE DUTY SPLIT (binding, carried): REDACTION IS
 *   THE CORE'S DUTY — the page displays only Core-returned sanitized
 *   content and never exposes, requests, or fabricates an unredacted
 *   channel (the may-never list: no secret values — a named
 *   secret-handle's NAME only; no raw logs, runtime internals, stack
 *   traces, or memory contents; no raw plugin output or raw provider
 *   payloads as diagnostics; no diagnostic the Core did not return).
 * - `GET /v1/plugins/{id}/state` (the authoritative state endpoint,
 *   B5-A1 §2.1 #8): a pure query — never a retry; the reconciliation
 *   anchor for every lost lifecycle outcome (the S0–S4 law). Events are
 *   never the source of truth; this read is.
 * - THE VERDICT LAW (binding): every verification state
 *   (signature/trust/rotation/revocation) renders ONLY as the Core
 *   returns it — never computed UI-side; an absent verification state is
 *   never rendered as a verified one, and `Unknown` is the honest trust
 *   rendering (P4.1 P1's law). On this Core the served package carries
 *   `package_ref` alone (no verification machinery exists while the MB
 *   §10.2 gate stands — B5-A1 §2.4 gap 3), so the verification members
 *   render their honest absence.
 * - THE MANIFEST HONESTY (B5-A1 §2.4 gap 6): the manifest member is
 *   OPTIONAL on the published `Plugin` schema and is ABSENT on this Core
 *   (no manifest-examination path exists) — the Manifest tab renders that
 *   absence honestly; manifest text is UNTRUSTED DATA — rendered, never
 *   executed.
 * - THE DISTINCTNESS LAWS (binding, carried): DISABLED (the user's
 *   action) ≠ GRANTS-REVOKED (the user's revocation of capabilities) ≠
 *   TRUST-REVOKED (the publisher/trust-root action) — each renders its
 *   own chip, never merged, never milder; INVOCATION TERMINATION ≠
 *   PLUGIN DISABLE/REVOCATION.
 * - THE LIFECYCLE CONTROLS (each at its recorded gate state, the B2
 *   review-actions' honest-disabled pattern): the enable/disable toggle
 *   (G4) and removal (G6) are UI-29-withheld — the controls render
 *   DISABLED with their accurate reasons, never dead stubs; the update
 *   (G5) has no servable input (no catalog exists — the no-catalog law;
 *   no served surface supplies a target version reference; the
 *   fresh-review producer's honest gap stands); grant revocation (G7) is
 *   the user's explicit high-risk act whose mutation caller is not wired
 *   in this milestone's read-only transport. THE REMOVE-VS-REVOKE
 *   DISTINCTNESS IS RECORDED OPEN: no copy here asserts removal and grant
 *   revocation are the same or distinct.
 * - NO invocation/execution surface exists anywhere (the D-13/D-14 gate
 *   and the Core Master Blueprint §10.2 runtime proof): no invocation
 *   control, no invocation state, and no invocation-derived failure
 *   record is rendered or invented.
 * - The unknown identity and the out-of-scope identity are the SAME
 *   typed 404 (the no-leak rule): the page states exactly that and never
 *   infers existence from a refusal.
 */

type ReadPhase = "loading" | "data" | "failure" | "unknown";

type TabId = "overview" | "manifest" | "capabilities" | "compatibility" | "digest" | "logs" | "audit";

/** The recorded G1 tab set (§19.2's order): Overview, Manifest,
 *  Capabilities, Compatibility, Versions/Digest, Safe logs, Audit
 *  events. Every tab renders its honest served state — a section the
 *  Core does not serve renders its absence (G1's missing-section law),
 *  never invented content. */
const TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "overview", labelKey: "plugin.detail.tabsOverview" },
  { id: "manifest", labelKey: "plugin.detail.tabsManifest" },
  { id: "capabilities", labelKey: "plugin.detail.tabsCapabilities" },
  { id: "compatibility", labelKey: "plugin.detail.tabsCompatibility" },
  { id: "digest", labelKey: "plugin.detail.tabsDigest" },
  { id: "logs", labelKey: "plugin.detail.tabsLogs" },
  { id: "audit", labelKey: "plugin.detail.tabsAudit" },
];

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function PluginDetailPage() {
  const { pluginId = "" } = useParams<{ pluginId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [plugin, setPlugin] = useState<Plugin | null>(null);
  const [state, setState] = useState<PluginState | null>(null);
  const [inspectPhase, setInspectPhase] = useState<ReadPhase>("loading");
  const [statePhase, setStatePhase] = useState<ReadPhase>("loading");
  const [inspectFailure, setInspectFailure] = useState<CoreWireEnvelope | null>(null);
  const [stateFailure, setStateFailure] = useState<CoreWireEnvelope | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [notFound, setNotFound] = useState(false);

  const tabListRef = useRef<HTMLDivElement>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const readAll = useCallback(() => {
    setInspectPhase("loading");
    setInspectFailure(null);
    setStatePhase("loading");
    setStateFailure(null);
    setNotFound(false);
    setLastConfirmedAt(null);
    void transport.pluginGet(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setPlugin(outcome.data);
        setInspectPhase("data");
        setLastConfirmedAt(new Date());
      } else if (outcome.kind === "rejection") {
        setInspectFailure(outcome.envelope);
        setNotFound(outcome.envelope.code === "NOT_FOUND");
        setInspectPhase("failure");
      } else {
        setInspectPhase("unknown");
      }
    });
    void transport.pluginState(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data);
        setStatePhase("data");
      } else if (outcome.kind === "rejection") {
        setStateFailure(outcome.envelope);
        setStatePhase("failure");
      } else {
        setStatePhase("unknown");
      }
    });
  }, [transport, pluginId]);

  useEffect(() => {
    readAll();
  }, [readAll]);

  // The tablist keyboard pattern (§8.8): Arrow keys move among the tabs;
  // every tab renders its honest served state (G1's missing-section law),
  // so none is excluded from the roving focus.
  const onTabListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
    event.preventDefault();
    const index = TABS.findIndex((tab) => tab.id === activeTab);
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = TABS[(index + delta + TABS.length) % TABS.length];
    setActiveTab(next.id);
    const nextButton = tabListRef.current?.querySelector<HTMLButtonElement>(`[data-tab-id="${next.id}"]`);
    nextButton?.focus();
  };

  if (inspectPhase === "loading") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        <p className={styles.note}>{t("plugin.list.loading")}</p>
      </PageLayout>
    );
  }

  if (inspectPhase === "failure" && inspectFailure !== null) {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        {notFound ? (
          <RecoveryPanel
            title={t("plugin.detail.notFoundTitle")}
            body={t("plugin.detail.notFoundBody")}
            note={t("plugin.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[
              { label: t("plugin.detail.back"), onAction: () => window.history.back() },
            ]}
          />
        ) : (
          <RecoveryPanel
            title={t("plugin.detail.failureTitle")}
            body={t("plugin.detail.failureBody")}
            note={t("plugin.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
          />
        )}
        <p className={styles.code}>
          <span className="visually-hidden">Code: </span>
          <code>{inspectFailure.code}</code>
        </p>
      </PageLayout>
    );
  }

  if (inspectPhase === "unknown") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("plugin.detail.unknownTitle")}
          body={t("plugin.detail.unknownBody")}
          note={t("plugin.detail.connectionHint")}
          actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
        />
      </PageLayout>
    );
  }

  // inspectPhase === "data" here; plugin is non-null by construction.
  const shownPlugin = plugin as Plugin;
  // The served record carries no display-name member; the Core-minted
  // identity is the honest page title (never renamed, never invented).
  const shownName = shownPlugin.plugin_id;

  const lifecycleChip = pluginLifecycleChip(shownPlugin.lifecycle_state);
  const LifecycleIcon = lifecycleChip.icon;

  const stateChip = state === null ? null : pluginLifecycleChip(state.lifecycle_state);
  const StateIcon = stateChip === null ? null : stateChip.icon;

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to="/plugins">{t("plugin.detail.back")}</Link>
      </p>
      <PageHeading>{shownName}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("plugin.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}
      <div className={styles.toolbar}>
        <button type="button" className={styles.control} onClick={readAll}>
          {t("plugin.detail.refresh")}
        </button>
      </div>
      {/* G10 — the detail surface's gate state (the accurate recorded
          reason): the catalog/install/activation/removal controls stay
          withheld while UI-29 is open; the surface renders Core-owned
          reads only. */}
      <p className={styles.gateNote}>{t("plugin.detail.gateNote")}</p>
      {/* The execution gate's honest state: NO invocation/execution
          surface exists (D-13/D-14 + the Core MB §10.2 runtime proof);
          nothing is invented for it. */}
      <p className={styles.gateNote}>{t("plugin.detail.executionGateNote")}</p>

      <section className={styles.factsPanel} aria-label={t("plugin.detail.factsHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.factsHeading")}</h2>
        <dl className={styles.facts}>
          <div>
            <dt>{t("plugin.detail.factsPluginId")}</dt>
            <dd>
              <CopyIdButton
                id={shownPlugin.plugin_id}
                label={t("plugin.detail.copyPluginId")}
                copiedLabel={t("plugin.list.copied")}
                failedLabel={t("plugin.list.copyFailed")}
                className={styles.copyId}
              />
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsLifecycleState")}</dt>
            <dd>
              <span className={`${styles.chip} ${styles[lifecycleChip.tone]}`}>
                <LifecycleIcon aria-hidden="true" size={14} />
                {t(lifecycleChip.labelKey)}
              </span>
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsInstallRecordRef")}</dt>
            <dd>
              {shownPlugin.install_record_ref === undefined || shownPlugin.install_record_ref === "" ? (
                <span className={styles.muted}>{unavailable}</span>
              ) : (
                <code>{shownPlugin.install_record_ref}</code>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsInstalledAt")}</dt>
            <dd>
              {shownPlugin.installed_at === undefined ? (
                unavailable
              ) : (
                <time dateTime={shownPlugin.installed_at}>
                  {formatTimestamp(new Date(shownPlugin.installed_at), locale)}
                </time>
              )}
            </dd>
          </div>
        </dl>
      </section>

      {/* The authoritative state endpoint's read (a pure query — never a
          retry; the reconciliation anchor for every lost lifecycle
          outcome; events are never the source of truth). */}
      <section className={styles.factsPanel} aria-label={t("plugin.detail.stateHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.stateHeading")}</h2>
        <p className={styles.note}>{t("plugin.detail.stateNote")}</p>
        {statePhase === "loading" ? <p className={styles.note}>{t("plugin.list.loading")}</p> : null}
        {statePhase === "failure" && stateFailure !== null ? (
          <>
            <RecoveryPanel
              title={t("plugin.detail.failureTitle")}
              body={t("plugin.detail.failureBody")}
              note={t("plugin.detail.connectionHint")}
              referenceId={stateFailure.correlationId}
              actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{stateFailure.code}</code>
            </p>
          </>
        ) : null}
        {statePhase === "unknown" ? (
          <RecoveryPanel
            title={t("plugin.detail.unknownTitle")}
            body={t("plugin.detail.unknownBody")}
            note={t("plugin.detail.connectionHint")}
            actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
          />
        ) : null}
        {statePhase === "data" && state !== null && stateChip !== null && StateIcon !== null ? (
          <dl className={styles.facts}>
            <div>
              <dt>{t("plugin.detail.stateLifecycleState")}</dt>
              <dd>
                <span className={`${styles.chip} ${styles[stateChip.tone]}`}>
                  <StateIcon aria-hidden="true" size={14} />
                  {t(stateChip.labelKey)}
                </span>
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.statePackageRef")}</dt>
              <dd>
                {state.package_ref === undefined || state.package_ref === "" ? (
                  <span className={styles.muted}>{unavailable}</span>
                ) : (
                  <code>{state.package_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.stateInstallRecordRef")}</dt>
              <dd>
                {state.install_record_ref === undefined || state.install_record_ref === "" ? (
                  <span className={styles.muted}>{unavailable}</span>
                ) : (
                  <code>{state.install_record_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.stateUpdatedAt")}</dt>
              <dd>
                {state.updated_at === undefined ? (
                  unavailable
                ) : (
                  <time dateTime={state.updated_at}>{formatTimestamp(new Date(state.updated_at), locale)}</time>
                )}
              </dd>
            </div>
          </dl>
        ) : null}
      </section>

      {/* The lifecycle controls at their recorded gate states (the B2
          review-actions' honest-disabled pattern — never dead stubs). The
          remove-vs-revoke distinctness is RECORDED OPEN: the copy asserts
          neither. */}
      <section className={styles.factsPanel} aria-label={t("plugin.detail.actionsHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.actionsHeading")}</h2>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="plugin-toggle-disabled-reason"
          >
            {t("plugin.detail.toggleControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="plugin-update-disabled-reason"
          >
            {t("plugin.detail.updateControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="plugin-remove-disabled-reason"
          >
            {t("plugin.detail.removeControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="plugin-revoke-disabled-reason"
          >
            {t("plugin.detail.revokeControl")}
          </button>
        </div>
        <p id="plugin-toggle-disabled-reason" className={styles.actionReason}>
          {t("plugin.detail.toggleDisabledReason")}
        </p>
        <p className={styles.actionReason}>{t("plugin.detail.toggleShapeNote")}</p>
        <p id="plugin-update-disabled-reason" className={styles.actionReason}>
          {t("plugin.detail.updateDisabledReason")}
        </p>
        <p id="plugin-remove-disabled-reason" className={styles.actionReason}>
          {t("plugin.detail.removeDisabledReason")}
        </p>
        <p className={styles.actionReason}>{t("plugin.detail.removeRevokeOpenNote")}</p>
        <p id="plugin-revoke-disabled-reason" className={styles.actionReason}>
          {t("plugin.detail.revokeDisabledReason")}
        </p>
      </section>

      <div
        className={styles.tabList}
        role="tablist"
        aria-label={shownName}
        ref={tabListRef}
        onKeyDown={onTabListKeyDown}
      >
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`plugin-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`plugin-panel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={activeTab === tab.id ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(tab.id)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>

      {activeTab === "overview" ? (
        <section
          role="tabpanel"
          id="plugin-panel-overview"
          aria-labelledby="plugin-tab-overview"
          className={styles.panel}
        >
          <p className={styles.note}>{t("plugin.detail.overviewDistinctNote")}</p>
          <p className={styles.note}>{t("plugin.detail.overviewTrustNote")}</p>
          <p className={styles.note}>{t("plugin.detail.overviewFailureNote")}</p>
        </section>
      ) : null}

      {activeTab === "manifest" ? (
        <section
          role="tabpanel"
          id="plugin-panel-manifest"
          aria-labelledby="plugin-tab-manifest"
          className={styles.panel}
        >
          {/* The manifest member is ABSENT on this Core (B5-A1 §2.4 gap
              6): the honest absence renders — never a fabricated manifest. */}
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.manifestAbsentTitle")}</p>
            <p>{t("plugin.detail.manifestAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.manifestUntrustedNote")}</p>
          <p className={styles.note}>{t("plugin.detail.manifestResourceNote")}</p>
        </section>
      ) : null}

      {activeTab === "capabilities" ? (
        <section
          role="tabpanel"
          id="plugin-panel-capabilities"
          aria-labelledby="plugin-tab-capabilities"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.capabilitiesAbsentTitle")}</p>
            <p>{t("plugin.detail.capabilitiesAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.capabilitiesNeverGrantNote")}</p>
          <p className={styles.note}>{t("plugin.detail.capabilitiesReviewNote")}</p>
        </section>
      ) : null}

      {activeTab === "compatibility" ? (
        <section
          role="tabpanel"
          id="plugin-panel-compatibility"
          aria-labelledby="plugin-tab-compatibility"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.compatibilityAbsentTitle")}</p>
            <p>{t("plugin.detail.compatibilityAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.compatibilityAbiNote")}</p>
        </section>
      ) : null}

      {activeTab === "digest" ? (
        <section
          role="tabpanel"
          id="plugin-panel-digest"
          aria-labelledby="plugin-tab-digest"
          className={styles.panel}
        >
          {/* The package record renders as the Core returns it (the
              verdict law); each verification member renders only where the
              Core returns it — on this Core the served package carries
              `package_ref` alone (B5-A1 §2.4 gap 3), so the verification
              members render their honest absence. */}
          {shownPlugin.package === undefined ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyTitle}>{t("plugin.detail.digestNoPackageTitle")}</p>
              <p>{t("plugin.detail.digestNoPackageBody")}</p>
            </div>
          ) : (
            <dl className={styles.facts}>
              <div>
                <dt>{t("plugin.detail.digestPackageRef")}</dt>
                <dd>
                  <code>{shownPlugin.package.package_ref}</code>
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestArtifactDigest")}</dt>
                <dd>
                  {shownPlugin.package.artifact_digest === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.artifact_digest}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestSignatureResult")}</dt>
                <dd>
                  {shownPlugin.package.signature_verification_result === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.signature_verification_result}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestTrustRootRef")}</dt>
                <dd>
                  {shownPlugin.package.trust_root_ref === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.trust_root_ref}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestRotationStatus")}</dt>
                <dd>
                  {shownPlugin.package.rotation_status === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.rotation_status}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestRevocationStatus")}</dt>
                <dd>
                  {shownPlugin.package.revocation_status === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.revocation_status}</code>
                  )}
                </dd>
              </div>
            </dl>
          )}
          <p className={styles.note}>{t("plugin.detail.digestVerificationNote")}</p>
          <p className={styles.note}>{t("plugin.detail.digestTrustWordNote")}</p>
        </section>
      ) : null}

      {activeTab === "logs" ? (
        <section
          role="tabpanel"
          id="plugin-panel-logs"
          aria-labelledby="plugin-tab-logs"
          className={styles.panel}
        >
          {/* G9's honest absence: no published plugin-log read exists in
              the served family; missing/unavailable logs are labeled
              honestly; the redaction duty-split rides the note. */}
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.logsAbsentTitle")}</p>
            <p>{t("plugin.detail.logsAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.logsRedactionNote")}</p>
        </section>
      ) : null}

      {activeTab === "audit" ? (
        <section
          role="tabpanel"
          id="plugin-panel-audit"
          aria-labelledby="plugin-tab-audit"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.auditAbsentTitle")}</p>
            <p>{t("plugin.detail.auditAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.auditSourceNote")}</p>
        </section>
      ) : null}
    </PageLayout>
  );
}
