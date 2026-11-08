import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  KnowledgePack,
  KnowledgePackVersion,
  KnowledgePackVersionServed,
} from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./KnowledgePackDetailPage.module.css";
import { KnowledgeExportDialog } from "./KnowledgeExportDialog";
import { KnowledgeImportWizard } from "./KnowledgeImportWizard";
import { CopyIdButton, countOrUnavailable, digestOrUnavailable } from "./knowledgeStates";

/**
 * `/knowledge/packs/:packId` — the canonical pack detail (B1-U1; P4.1
 * D1–D14 against the served B1 family).
 *
 * The tab law (P4.3 §3.4) binds this page: a tab is PRESENT only where
 * the Core actually supplies its data; where unsupported, the tab is
 * absent or disabled with the accurate reason — never a placeholder.
 * Applied to the served family:
 * - **Overview (D1)** renders the pack read's members (identity, schema
 *   version, timestamps, current-version reference) plus the current
 *   version's served facts; the language member is not served and
 *   renders `Unavailable` — never invented.
 * - **Sources (D2), Claims & evidence (D3), Provenance (D5)** are
 *   DISABLED tabs: the served family carries no source, claim/evidence,
 *   or provenance read (B1-A1 §2.6 item 1), so they render no invented
 *   content. The reason is stated accessibly beneath the tab row.
 * - **Permissions (D7)** is ABSENT entirely (its own recorded law: the
 *   tab exists only where the access model supports pack-level display;
 *   no access model is served, so no placeholder exists).
 * - **Versions (D4)** renders the version list exactly as the Core lists
 *   it; each row links the canonical immutable-version route; the
 *   "current" badge derives ONLY from the pack read's current_version_id
 *   (the Core's own identification — the UI never computes "latest").
 * - **Integrity (D6)** renders digest/counts/license only from the
 *   served version read; no `Verified` label can ever appear (no
 *   verification surface is served), and no signature/trust state is
 *   inferred (the honest-absence statement renders instead).
 * - The mutation controls that have NO served operation (Create pack,
 *   Compare versions, Resolve conflict, Delete pack, Verify integrity)
 *   are ABSENT — never stubs (§31.5). The one served version-creating
 *   path (import with a destination pack) is the detail page's
 *   `Import new version` entry.
 */

type PackPhase = "loading" | "data" | "failure" | "unknown";
type VersionsPhase = "loading" | "data" | "failure" | "unknown";
type VersionReadPhase = "idle" | "loading" | "data" | "failure" | "unknown";

type TabId = "overview" | "sources" | "claims" | "versions" | "provenance" | "integrity";

const ENABLED_TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "overview", labelKey: "knowledge.detail.tabOverview" },
  { id: "versions", labelKey: "knowledge.detail.tabVersions" },
  { id: "integrity", labelKey: "knowledge.detail.tabIntegrity" },
];

const DISABLED_TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "sources", labelKey: "knowledge.detail.tabSources" },
  { id: "claims", labelKey: "knowledge.detail.tabClaimsEvidence" },
  { id: "provenance", labelKey: "knowledge.detail.tabProvenance" },
];

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function KnowledgePackDetailPage() {
  const { packId = "" } = useParams<{ packId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [pack, setPack] = useState<KnowledgePack | null>(null);
  const [packPhase, setPackPhase] = useState<PackPhase>("loading");
  const [packFailure, setPackFailure] = useState<CoreWireEnvelope | null>(null);

  const [versions, setVersions] = useState<readonly KnowledgePackVersionServed[]>([]);
  const [versionsPhase, setVersionsPhase] = useState<VersionsPhase>("loading");
  const [versionsFailure, setVersionsFailure] = useState<CoreWireEnvelope | null>(null);

  const [currentVersion, setCurrentVersion] = useState<KnowledgePackVersionServed | null>(null);
  const [currentVersionPhase, setCurrentVersionPhase] = useState<VersionReadPhase>("idle");
  const [currentVersionFailure, setCurrentVersionFailure] = useState<CoreWireEnvelope | null>(null);

  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [wizardOpen, setWizardOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);

  const tabListRef = useRef<HTMLDivElement>(null);
  const importTriggerRef = useRef<HTMLButtonElement | null>(null);
  const exportTriggerRef = useRef<HTMLButtonElement | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const readAll = useCallback(() => {
    setPackPhase("loading");
    setPackFailure(null);
    setVersionsPhase("loading");
    setVersionsFailure(null);
    setLastConfirmedAt(null);
    void transport.knowledgePackGet(packId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setPack(outcome.data.pack);
        setPackPhase("data");
        setLastConfirmedAt(new Date());
      } else if (outcome.kind === "rejection") {
        setPackFailure(outcome.envelope);
        setPackPhase("failure");
      } else {
        setPackPhase("unknown");
      }
    });
    void transport.knowledgeVersionList(packId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setVersions(outcome.data.versions);
        setVersionsPhase("data");
      } else if (outcome.kind === "rejection") {
        setVersionsFailure(outcome.envelope);
        setVersionsPhase("failure");
      } else {
        setVersionsPhase("unknown");
      }
    });
  }, [packId, transport]);

  useEffect(() => {
    readAll();
  }, [readAll]);

  // The current version's read (the pack's own current_version_id is the
  // Core's identification — the UI never computes "latest").
  useEffect(() => {
    if (pack === null || pack.current_version_id === "") {
      setCurrentVersion(null);
      setCurrentVersionPhase("idle");
      setCurrentVersionFailure(null);
      return;
    }
    setCurrentVersionPhase("loading");
    setCurrentVersionFailure(null);
    void transport.knowledgeVersionGet(packId, pack.current_version_id).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setCurrentVersion(outcome.data.version);
        setCurrentVersionPhase("data");
      } else if (outcome.kind === "rejection") {
        setCurrentVersionFailure(outcome.envelope);
        setCurrentVersionPhase("failure");
      } else {
        setCurrentVersionPhase("unknown");
      }
    });
  }, [pack, packId, transport]);

  // The tablist keyboard pattern (§8.8): Arrow keys move among the
  // enabled tabs; the disabled tabs stay out of the roving focus.
  const onTabListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
    event.preventDefault();
    const index = ENABLED_TABS.findIndex((tab) => tab.id === activeTab);
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = ENABLED_TABS[(index + delta + ENABLED_TABS.length) % ENABLED_TABS.length];
    setActiveTab(next.id);
    const nextButton = tabListRef.current?.querySelector<HTMLButtonElement>(`[data-tab-id="${next.id}"]`);
    nextButton?.focus();
  };

  if (packPhase === "loading") {
    return (
      <PageLayout>
        <PageHeading>{t("knowledge.list.heading")}</PageHeading>
        <p className={styles.note}>{t("knowledge.list.loading")}</p>
      </PageLayout>
    );
  }

  if (packPhase === "failure" && packFailure !== null) {
    return (
      <PageLayout>
        <PageHeading>{t("knowledge.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("knowledge.detail.failureTitle")}
          body={t("knowledge.detail.failureBody")}
          note={t("knowledge.list.connectionHint")}
          referenceId={packFailure.correlationId}
          actions={[{ label: t("knowledge.detail.back"), onAction: () => window.history.back() }]}
        />
        <p className={styles.code}>
          <span className="visually-hidden">Code: </span>
          <code>{packFailure.code}</code>
        </p>
      </PageLayout>
    );
  }

  if (packPhase === "unknown") {
    return (
      <PageLayout>
        <PageHeading>{t("knowledge.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("knowledge.detail.unknownTitle")}
          body={t("knowledge.detail.unknownBody")}
          note={t("knowledge.list.connectionHint")}
          actions={[
            { label: t("knowledge.detail.refresh"), onAction: readAll },
            { label: t("knowledge.detail.back"), onAction: () => window.history.back() },
          ]}
        />
      </PageLayout>
    );
  }

  // packPhase === "data" here; pack is non-null by construction.
  const shownPack = pack as KnowledgePack;

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to="/knowledge">{t("knowledge.detail.back")}</Link>
      </p>
      <PageHeading>{shownPack.title}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("knowledge.detail.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}
      <div className={styles.toolbar}>
        <button
          type="button"
          ref={importTriggerRef}
          className={styles.control}
          onClick={() => setWizardOpen(true)}
        >
          {t("knowledge.detail.versions.importNewVersion")}
        </button>
        <button
          type="button"
          ref={exportTriggerRef}
          className={styles.control}
          onClick={() => setExportOpen(true)}
        >
          {t("knowledge.detail.export.open")}
        </button>
        <button type="button" className={styles.control} onClick={readAll}>
          {t("knowledge.detail.refresh")}
        </button>
      </div>
      <p className={styles.note}>{t("knowledge.detail.versions.importNewVersionNote")}</p>

      <div
        className={styles.tabList}
        role="tablist"
        aria-label={shownPack.title}
        ref={tabListRef}
        onKeyDown={onTabListKeyDown}
      >
        {ENABLED_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`knowledge-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`knowledge-panel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={activeTab === tab.id ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(tab.id)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
        {DISABLED_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`knowledge-tab-${tab.id}`}
            aria-selected={false}
            aria-disabled="true"
            tabIndex={-1}
            className={styles.tabDisabled}
            onClick={(event) => event.preventDefault()}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>
      {/* The disabled tabs' accurate reason (D14's law: never a dead
          stub, never a placeholder). */}
      <p className={styles.tabNote}>{t("knowledge.detail.tabsUnavailableNote")}</p>

      {activeTab === "overview" ? (
        <section role="tabpanel" id="knowledge-panel-overview" aria-labelledby="knowledge-tab-overview" className={styles.panel}>
          <dl className={styles.facts}>
            <div>
              <dt>{t("knowledge.detail.overview.packId")}</dt>
              <dd>
                <CopyIdButton
                  id={shownPack.pack_id}
                  label={t("knowledge.list.copyPackId")}
                  copiedLabel={t("knowledge.list.copied")}
                  failedLabel={t("knowledge.list.copyFailed")}
                  className={styles.copyId}
                />
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.detail.overview.schemaVersion")}</dt>
              <dd>{shownPack.schema_version}</dd>
            </div>
            <div>
              <dt>{t("knowledge.detail.overview.description")}</dt>
              <dd>{shownPack.description === undefined || shownPack.description === "" ? unavailable : shownPack.description}</dd>
            </div>
            {/* The language member is not served by the pack read: the
                row renders Unavailable — never invented (P4.3 D1). */}
            <div>
              <dt>{t("knowledge.detail.overview.language")}</dt>
              <dd>{unavailable}</dd>
            </div>
            <div>
              <dt>{t("knowledge.detail.overview.createdAt")}</dt>
              <dd>
                <time dateTime={shownPack.created_at}>{formatTimestamp(new Date(shownPack.created_at), locale)}</time>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.detail.overview.updatedAt")}</dt>
              <dd>
                <time dateTime={shownPack.updated_at}>{formatTimestamp(new Date(shownPack.updated_at), locale)}</time>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.detail.overview.currentVersion")}</dt>
              <dd>
                {shownPack.current_version_id === "" ? (
                  t("knowledge.detail.overview.noVersionStoredYet")
                ) : (
                  <Link to={`/knowledge/packs/${encodeURIComponent(shownPack.pack_id)}/versions/${encodeURIComponent(shownPack.current_version_id)}`} className={styles.link}>
                    <code>{shownPack.current_version_id}</code>
                  </Link>
                )}
              </dd>
            </div>
          </dl>

          {shownPack.current_version_id !== "" ? (
            <section className={styles.currentVersionBlock}>
              <h2 className={styles.subHeading}>{t("knowledge.detail.overview.currentVersionBlock")}</h2>
              {currentVersionPhase === "loading" ? <p className={styles.note}>{t("knowledge.list.loading")}</p> : null}
              {currentVersionPhase === "failure" && currentVersionFailure !== null ? (
                <p className={styles.code}>
                  <code>{currentVersionFailure.code}</code>
                </p>
              ) : null}
              {currentVersionPhase === "unknown" ? <p className={styles.note}>{unavailable}</p> : null}
              {currentVersionPhase === "data" && currentVersion !== null ? (
                <CurrentVersionFacts version={currentVersion} />
              ) : null}
            </section>
          ) : null}
        </section>
      ) : null}

      {activeTab === "versions" ? (
        <section role="tabpanel" id="knowledge-panel-versions" aria-labelledby="knowledge-tab-versions" className={styles.panel}>
          <p className={styles.description}>{t("knowledge.detail.versions.description")}</p>
          {versionsPhase === "loading" ? <p className={styles.note}>{t("knowledge.list.loading")}</p> : null}
          {versionsPhase === "failure" && versionsFailure !== null ? (
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{versionsFailure.code}</code>
            </p>
          ) : null}
          {versionsPhase === "unknown" ? <p className={styles.note}>{unavailable}</p> : null}
          {versionsPhase === "data" ? (
            versions.length === 0 ? (
              <p className={styles.note}>{t("knowledge.detail.versions.empty")}</p>
            ) : (
              <table className={styles.versionsTable}>
                <thead>
                  <tr>
                    <th scope="col">{t("knowledge.detail.versions.columnVersion")}</th>
                    <th scope="col">{t("knowledge.detail.versions.columnCreatedAt")}</th>
                    <th scope="col">{t("knowledge.detail.versions.columnEntries")}</th>
                    <th scope="col">{t("knowledge.detail.versions.columnLicense")}</th>
                    <th scope="col">{t("knowledge.detail.versions.columnLicenseStatus")}</th>
                  </tr>
                </thead>
                <tbody>
                  {versions.map((version) => (
                    <tr key={version.version_id}>
                      <td>
                        <Link
                          to={`/knowledge/packs/${encodeURIComponent(version.pack_id)}/versions/${encodeURIComponent(version.version_id)}`}
                          className={styles.link}
                        >
                          <code>{version.version_id}</code>
                          <span className="visually-hidden">
                            {t("knowledge.detail.versions.openVersion", { versionId: version.version_id })}
                          </span>
                        </Link>
                        {version.version_id === shownPack.current_version_id ? (
                          <span className={styles.currentBadge}>{t("knowledge.detail.versions.currentBadge")}</span>
                        ) : null}
                        <VersionSupersession version={version} />
                      </td>
                      <td>
                        <time dateTime={version.created_at}>{formatTimestamp(new Date(version.created_at), locale)}</time>
                      </td>
                      <td>{countOrUnavailable(version.entry_count, unavailable)}</td>
                      <td>{version.license === undefined || version.license === "" ? unavailable : version.license}</td>
                      <td>
                        {version.license_status === undefined || version.license_status === ""
                          ? unavailable
                          : version.license_status}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )
          ) : null}
        </section>
      ) : null}

      {activeTab === "integrity" ? (
        <section role="tabpanel" id="knowledge-panel-integrity" aria-labelledby="knowledge-tab-integrity" className={styles.panel}>
          <p className={styles.description}>{t("knowledge.detail.integrity.description")}</p>
          {shownPack.current_version_id === "" ? (
            <p className={styles.note}>{t("knowledge.detail.overview.noVersionStoredYet")}</p>
          ) : (
            <>
              {currentVersionPhase === "loading" ? <p className={styles.note}>{t("knowledge.list.loading")}</p> : null}
              {currentVersionPhase === "failure" && currentVersionFailure !== null ? (
                <p className={styles.code}>
                  <span className="visually-hidden">Code: </span>
                  <code>{currentVersionFailure.code}</code>
                </p>
              ) : null}
              {currentVersionPhase === "unknown" ? <p className={styles.note}>{unavailable}</p> : null}
              {currentVersionPhase === "data" && currentVersion !== null ? (
                <dl className={styles.facts}>
                  <div>
                    <dt>{t("knowledge.detail.integrity.digest")}</dt>
                    <dd>
                      <code>{digestOrUnavailable(currentVersion.pack_root_digest, unavailable)}</code>
                      <p className={styles.note}>{t("knowledge.detail.integrity.digestNote")}</p>
                    </dd>
                  </div>
                  <div>
                    <dt>{t("knowledge.detail.integrity.entryCount")}</dt>
                    <dd>{countOrUnavailable(currentVersion.entry_count, unavailable)}</dd>
                  </div>
                  <div>
                    <dt>{t("knowledge.detail.integrity.claimCount")}</dt>
                    <dd>{countOrUnavailable(currentVersion.claim_count, unavailable)}</dd>
                  </div>
                  <div>
                    <dt>{t("knowledge.detail.integrity.evidenceCount")}</dt>
                    <dd>{countOrUnavailable(currentVersion.evidence_count, unavailable)}</dd>
                  </div>
                  <div>
                    <dt>{t("knowledge.detail.integrity.license")}</dt>
                    <dd>
                      {currentVersion.license === undefined || currentVersion.license === "" ? unavailable : currentVersion.license}
                    </dd>
                  </div>
                  <div>
                    <dt>{t("knowledge.detail.integrity.licenseStatus")}</dt>
                    <dd>
                      {currentVersion.license_status === undefined || currentVersion.license_status === "" ? (
                        unavailable
                      ) : (
                        <>
                          {currentVersion.license_status}
                          {currentVersion.license_status === "unknown" ? (
                            <span className={styles.note}> — {t("knowledge.detail.integrity.licenseStatusUnknownNote")}</span>
                          ) : null}
                        </>
                      )}
                    </dd>
                  </div>
                </dl>
              ) : null}
              <p className={styles.note}>{t("knowledge.detail.integrity.signatureTrustNote")}</p>
            </>
          )}
        </section>
      ) : null}

      <KnowledgeImportWizard
        open={wizardOpen}
        presetDestinationPackId={shownPack.pack_id}
        onClose={() => {
          setWizardOpen(false);
          // The wizard's close re-reads so a completed version-create
          // appears from the Core's own confirmed state.
          readAll();
        }}
      />
      <KnowledgeExportDialog
        open={exportOpen}
        pack={shownPack}
        versions={versions}
        onClose={() => {
          setExportOpen(false);
          if (exportTriggerRef.current !== null) exportTriggerRef.current.focus();
        }}
      />
    </PageLayout>
  );
}

function CurrentVersionFacts({ version }: { version: KnowledgePackVersion }) {
  const t = useT();
  const unavailable = t("states.unavailable");
  return (
    <dl className={styles.facts}>
      <div>
        <dt>{t("knowledge.detail.overview.digest")}</dt>
        <dd>
          <code>{digestOrUnavailable(version.pack_root_digest, unavailable)}</code>
        </dd>
      </div>
      <div>
        <dt>{t("knowledge.detail.overview.entryCount")}</dt>
        <dd>{countOrUnavailable(version.entry_count, unavailable)}</dd>
      </div>
      <div>
        <dt>{t("knowledge.detail.overview.claimCount")}</dt>
        <dd>{countOrUnavailable(version.claim_count, unavailable)}</dd>
      </div>
      <div>
        <dt>{t("knowledge.detail.overview.evidenceCount")}</dt>
        <dd>{countOrUnavailable(version.evidence_count, unavailable)}</dd>
      </div>
      <div>
        <dt>{t("knowledge.detail.overview.license")}</dt>
        <dd>{version.license === undefined || version.license === "" ? unavailable : version.license}</dd>
      </div>
      <div>
        <dt>{t("knowledge.detail.overview.licenseStatus")}</dt>
        <dd>
          {version.license_status === undefined || version.license_status === "" ? (
            unavailable
          ) : (
            <>
              {version.license_status}
              {version.license_status === "unknown" ? (
                <span className={styles.note}> — {t("knowledge.detail.overview.licenseStatusUnknown")}</span>
              ) : null}
            </>
          )}
        </dd>
      </div>
    </dl>
  );
}

function VersionSupersession({ version }: { version: KnowledgePackVersion }) {
  const t = useT();
  const supersedes = version.supersedes ?? [];
  const supersededBy = version.superseded_by ?? "";
  if (supersedes.length === 0 && supersededBy === "") {
    return <p className={styles.supersession}>{t("knowledge.detail.versions.noSupersession")}</p>;
  }
  return (
    <div className={styles.supersession}>
      {supersedes.length > 0 ? (
        <p>
          {t("knowledge.detail.versions.supersedes", { values: supersedes.join(", ") })}
        </p>
      ) : null}
      {supersededBy !== "" ? (
        <p>{t("knowledge.detail.versions.supersededBy", { value: supersededBy })}</p>
      ) : null}
    </div>
  );
}
