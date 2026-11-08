import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  WorkflowDefinition,
  WorkflowDefinitionState,
  WorkflowRevision,
  WorkflowRunHistoryEntry,
} from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./WorkflowDetailPage.module.css";
import { CopyIdButton, workflowLinkageChip } from "./workflowStates";

/**
 * `/workflows/:workflowId` — the ONE canonical workflow-detail route
 * (B4-U1; the DE-0 queue row 7, the P4.1 B1/B17/B19/B20 rows against the
 * now-live B4 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/workflows/{id}` (the inspect member, B4-A1 §2.1 #5): the
 *   definition plus its immutable revision records — each revision
 *   renders as the identity record the Core serves (reference and
 *   creation time). The family publishes NO revision-content read
 *   (B4-A1 §2.4 gap 7), so no node/edge listing renders anywhere: the
 *   structured graph representation renders where the Core provides the
 *   data, never invented. Every prior revision stays immutable and
 *   reachable; nothing on this surface can rewrite history.
 * - `GET /v1/workflows/{id}/state` (the authoritative state endpoint,
 *   B4-A1 §2.1 #8): a pure query — never a retry. Events are never the
 *   source of truth; this read is.
 * - `GET /v1/workflows/{id}/run-history` (the LINKAGE read, B4-A1 §2.1
 *   #7): each entry renders beside its Core-reported source revision;
 *   the linkage state renders as the Core's closed register reports it
 *   (`linked | unavailable`; an unknown reported value verbatim); a
 *   missing linkage renders Unavailable — NEVER an inferred link
 *   (RUN-LINK-3). The honest empty page stands while the D-40 gate
 *   withholds the run producer — no run record is fabricated.
 * - The Builder tab renders DISABLED with the accurate reason (the UI-30
 *   interaction gate is open — the Roadmap decision row; P4.1 §4's UI-30
 *   gate row withholds the editing/revision controls): the builder, node
 *   palette, and revision-saving controls stay hidden, and no dead
 *   builder renders (§31.4). NO Run control exists anywhere (the D-40
 *   gate: no Run submission/start/resume/cancel exists or is invented).
 *   NO If-Match/concurrency-protocol control exists (the clause is
 *   prose-only — the interim conflict rule governs; no conflict surface
 *   can arise from a read-only surface).
 * - The validation display (B19's state row): the Core's typed
 *   validation is the authoritative truth and renders where a
 *   Core-confirmed result exists; while the editing gate withholds the
 *   builder, no draft can exist, so the panel states that absence
 *   honestly (P4.1 B19's "pending/absent states honest"). The Validate
 *   control renders its honest disabled state with the accurate reason
 *   (the B2 review-actions' disabled-with-reason idiom): validation
 *   evaluates a draft, and no draft can exist while the gate stands. No
 *   client-side check exists anywhere to label advisory — the labeling
 *   law is stated, and the absence is the honest state.
 * - The unknown identity and the out-of-scope identity are the SAME
 *   typed 404 (the no-leak rule): the page states exactly that and never
 *   infers existence from a refusal.
 */

type ReadPhase = "loading" | "data" | "failure" | "unknown";

type TabId = "builder" | "validation" | "versions" | "runs" | "permissions";

/** The enabled tabs in the recorded B1 order minus the withheld Builder
 *  tab (which renders disabled at its recorded first position). The
 *  Permissions & side effects tab renders its honest absence: the
 *  per-node contract fields render only where the Core supplies them
 *  (P4.7 §3.2's display law), and no per-node contract is supplied yet. */
const ENABLED_TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "validation", labelKey: "workflow.detail.tabsValidation" },
  { id: "versions", labelKey: "workflow.detail.tabsVersions" },
  { id: "runs", labelKey: "workflow.detail.tabsRuns" },
  { id: "permissions", labelKey: "workflow.detail.tabsPermissions" },
];

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function WorkflowDetailPage() {
  const { workflowId = "" } = useParams<{ workflowId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [definition, setDefinition] = useState<WorkflowDefinition | null>(null);
  const [revisions, setRevisions] = useState<readonly WorkflowRevision[]>([]);
  const [state, setState] = useState<WorkflowDefinitionState | null>(null);
  const [runEntries, setRunEntries] = useState<readonly WorkflowRunHistoryEntry[]>([]);
  const [inspectPhase, setInspectPhase] = useState<ReadPhase>("loading");
  const [statePhase, setStatePhase] = useState<ReadPhase>("loading");
  const [runsPhase, setRunsPhase] = useState<ReadPhase>("loading");
  const [inspectFailure, setInspectFailure] = useState<CoreWireEnvelope | null>(null);
  const [stateFailure, setStateFailure] = useState<CoreWireEnvelope | null>(null);
  const [runsFailure, setRunsFailure] = useState<CoreWireEnvelope | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>("validation");
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
    setRunsPhase("loading");
    setRunsFailure(null);
    setNotFound(false);
    setLastConfirmedAt(null);
    void transport.workflowGet(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setDefinition(outcome.data.definition);
        setRevisions(outcome.data.revisions);
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
    void transport.workflowState(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data.definition_state);
        setStatePhase("data");
      } else if (outcome.kind === "rejection") {
        setStateFailure(outcome.envelope);
        setStatePhase("failure");
      } else {
        setStatePhase("unknown");
      }
    });
    void transport.workflowRunHistory(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setRunEntries(outcome.data.entries);
        setRunsPhase("data");
      } else if (outcome.kind === "rejection") {
        setRunsFailure(outcome.envelope);
        setRunsPhase("failure");
      } else {
        setRunsPhase("unknown");
      }
    });
  }, [transport, workflowId]);

  useEffect(() => {
    readAll();
  }, [readAll]);

  // The tablist keyboard pattern (§8.8): Arrow keys move among the
  // enabled tabs; the disabled Builder tab stays out of the roving focus.
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

  if (inspectPhase === "loading") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        <p className={styles.note}>{t("workflow.list.loading")}</p>
      </PageLayout>
    );
  }

  if (inspectPhase === "failure" && inspectFailure !== null) {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        {notFound ? (
          <RecoveryPanel
            title={t("workflow.detail.notFoundTitle")}
            body={t("workflow.detail.notFoundBody")}
            note={t("workflow.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[
              { label: t("workflow.detail.back"), onAction: () => window.history.back() },
            ]}
          />
        ) : (
          <RecoveryPanel
            title={t("workflow.detail.failureTitle")}
            body={t("workflow.detail.failureBody")}
            note={t("workflow.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
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
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("workflow.detail.unknownTitle")}
          body={t("workflow.detail.unknownBody")}
          note={t("workflow.detail.connectionHint")}
          actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
        />
      </PageLayout>
    );
  }

  // inspectPhase === "data" here; definition is non-null by construction.
  const shownDefinition = definition as WorkflowDefinition;
  const shownName =
    shownDefinition.display_name === undefined || shownDefinition.display_name === ""
      ? shownDefinition.definition_id
      : shownDefinition.display_name;

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to="/workflows">{t("workflow.detail.back")}</Link>
      </p>
      <PageHeading>{shownName}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("workflow.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}
      <div className={styles.toolbar}>
        <button type="button" className={styles.control} onClick={readAll}>
          {t("workflow.detail.refresh")}
        </button>
      </div>
      {/* B20 — the detail surface's gate state (the accurate recorded
          reason): the editing/revision controls are hidden; no Run
          control exists; the surface is read-only. */}
      <p className={styles.gateNote}>{t("workflow.detail.gateNote")}</p>
      {/* The AI-proposal member's honest unmet-producer state (B4-A1 §2.4
          gap 1): the typed refusal is the served behavior; no proposal
          surface is fabricated. */}
      <p className={styles.gateNote}>{t("workflow.detail.aiProposalNote")}</p>

      <section className={styles.factsPanel} aria-label={t("workflow.detail.factsDefinitionId")}>
        <h2 className={styles.subHeading}>{t("workflow.list.heading")}</h2>
        <dl className={styles.facts}>
          <div>
            <dt>{t("workflow.detail.factsDefinitionId")}</dt>
            <dd>
              <CopyIdButton
                id={shownDefinition.definition_id}
                label={t("workflow.detail.copyDefinitionId")}
                copiedLabel={t("workflow.list.copied")}
                failedLabel={t("workflow.list.copyFailed")}
                className={styles.copyId}
              />
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsDisplayName")}</dt>
            <dd>
              {shownDefinition.display_name === undefined || shownDefinition.display_name === ""
                ? unavailable
                : shownDefinition.display_name}
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsCurrentRevision")}</dt>
            <dd>
              {shownDefinition.current_revision_ref === undefined || shownDefinition.current_revision_ref === "" ? (
                <span className={styles.muted}>{t("workflow.detail.factsNoRevision")}</span>
              ) : (
                <code>{shownDefinition.current_revision_ref}</code>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsCreatedAt")}</dt>
            <dd>
              <time dateTime={shownDefinition.created_at}>
                {formatTimestamp(new Date(shownDefinition.created_at), locale)}
              </time>
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsUpdatedAt")}</dt>
            <dd>
              {shownDefinition.updated_at === undefined ? (
                unavailable
              ) : (
                <time dateTime={shownDefinition.updated_at}>
                  {formatTimestamp(new Date(shownDefinition.updated_at), locale)}
                </time>
              )}
            </dd>
          </div>
        </dl>
      </section>

      {/* The authoritative state endpoint's read (a pure query — never a
          retry; the reconcile anchor). */}
      <section className={styles.factsPanel} aria-label={t("workflow.detail.stateHeading")}>
        <h2 className={styles.subHeading}>{t("workflow.detail.stateHeading")}</h2>
        <p className={styles.note}>{t("workflow.detail.stateNote")}</p>
        {statePhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}
        {statePhase === "failure" && stateFailure !== null ? (
          <>
            <RecoveryPanel
              title={t("workflow.detail.failureTitle")}
              body={t("workflow.detail.failureBody")}
              note={t("workflow.detail.connectionHint")}
              referenceId={stateFailure.correlationId}
              actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{stateFailure.code}</code>
            </p>
          </>
        ) : null}
        {statePhase === "unknown" ? (
          <RecoveryPanel
            title={t("workflow.detail.unknownTitle")}
            body={t("workflow.detail.unknownBody")}
            note={t("workflow.detail.connectionHint")}
            actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
          />
        ) : null}
        {statePhase === "data" && state !== null ? (
          <dl className={styles.facts}>
            <div>
              <dt>{t("workflow.detail.stateCurrentRevision")}</dt>
              <dd>
                {state.current_revision_ref === undefined || state.current_revision_ref === "" ? (
                  <span className={styles.muted}>{t("workflow.detail.stateNoneStored")}</span>
                ) : (
                  <code>{state.current_revision_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("workflow.detail.stateUpdatedAt")}</dt>
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

      <div
        className={styles.tabList}
        role="tablist"
        aria-label={shownName}
        ref={tabListRef}
        onKeyDown={onTabListKeyDown}
      >
        {/* B20 — the Builder tab renders DISABLED with the accurate reason
            (the UI-30 interaction gate): never a dead builder, never an
            empty look-alike panel. */}
        <button
          type="button"
          role="tab"
          data-tab-id="builder"
          id="workflow-tab-builder"
          aria-selected={false}
          aria-disabled="true"
          tabIndex={-1}
          className={styles.tabDisabled}
          onClick={(event) => event.preventDefault()}
        >
          {t("workflow.detail.tabsBuilder")}
        </button>
        {ENABLED_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`workflow-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`workflow-panel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={activeTab === tab.id ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(tab.id)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>
      {/* The disabled Builder tab's accurate reason (B20's law: never a
          dead stub). */}
      <p className={styles.tabNote}>{t("workflow.detail.tabsUnavailableNote")}</p>

      {activeTab === "validation" ? (
        <section
          role="tabpanel"
          id="workflow-panel-validation"
          aria-labelledby="workflow-tab-validation"
          className={styles.panel}
        >
          <p className={styles.note}>{t("workflow.detail.validationAuthoritativeNote")}</p>
          {/* B19's honest absent state: while the editing gate withholds
              the builder, no draft can exist, so no Core validation can
              run from this surface — the panel states the absence; no
              fabricated result, count, or issue renders. */}
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("workflow.detail.validationAbsentTitle")}</p>
            <p>{t("workflow.detail.validationAbsentBody")}</p>
            <button
              type="button"
              className={styles.control}
              disabled
              aria-describedby="workflow-validate-disabled-reason"
            >
              {t("workflow.detail.validateControl")}
            </button>
            <p id="workflow-validate-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.validateDisabledReason")}
            </p>
          </div>
        </section>
      ) : null}

      {activeTab === "versions" ? (
        <section
          role="tabpanel"
          id="workflow-panel-versions"
          aria-labelledby="workflow-tab-versions"
          className={styles.panel}
        >
          <p className={styles.note}>{t("workflow.detail.versionsImmutableNote")}</p>
          <p className={styles.note}>{t("workflow.detail.versionsContentNote")}</p>
          {revisions.length === 0 ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyTitle}>{t("workflow.detail.versionsEmpty")}</p>
            </div>
          ) : (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th scope="col">{t("workflow.detail.versionsColumnRevision")}</th>
                  <th scope="col">{t("workflow.detail.versionsColumnCreatedAt")}</th>
                </tr>
              </thead>
              <tbody>
                {revisions.map((revision) => (
                  <tr key={revision.revision_ref}>
                    <td>
                      <CopyIdButton
                        id={revision.revision_ref}
                        label={t("workflow.detail.copyRevisionRef")}
                        copiedLabel={t("workflow.list.copied")}
                        failedLabel={t("workflow.list.copyFailed")}
                        className={styles.copyId}
                      />
                    </td>
                    <td>
                      <time dateTime={revision.created_at}>
                        {formatTimestamp(new Date(revision.created_at), locale)}
                      </time>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      ) : null}

      {activeTab === "runs" ? (
        <section
          role="tabpanel"
          id="workflow-panel-runs"
          aria-labelledby="workflow-tab-runs"
          className={styles.panel}
        >
          <p className={styles.note}>{t("workflow.detail.runsD40Note")}</p>
          {runsPhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}
          {runsPhase === "failure" && runsFailure !== null ? (
            <>
              <RecoveryPanel
                title={t("workflow.detail.failureTitle")}
                body={t("workflow.detail.failureBody")}
                note={t("workflow.detail.connectionHint")}
                referenceId={runsFailure.correlationId}
                actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{runsFailure.code}</code>
              </p>
            </>
          ) : null}
          {runsPhase === "unknown" ? (
            <RecoveryPanel
              title={t("workflow.detail.unknownTitle")}
              body={t("workflow.detail.unknownBody")}
              note={t("workflow.detail.connectionHint")}
              actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
            />
          ) : null}
          {runsPhase === "data" ? (
            runEntries.length === 0 ? (
              // The honest empty page while the D-40 gate stands: the
              // linkage read's producer is the GATED run execution (B4-A1
              // §2.1 #7) — no run record is fabricated.
              <div className={styles.emptyState}>
                <p className={styles.emptyTitle}>{t("workflow.detail.runsEmpty")}</p>
                <p>{t("workflow.detail.runsEmptyBody")}</p>
              </div>
            ) : (
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th scope="col">{t("workflow.detail.runsColumnRunRef")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnSourceRevision")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnLinkage")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnCreatedAt")}</th>
                  </tr>
                </thead>
                <tbody>
                  {runEntries.map((entry) => {
                    const linkage = workflowLinkageChip(entry.linkage_state);
                    const LinkageIcon = linkage.icon;
                    return (
                      <tr key={entry.run_ref}>
                        <td>
                          <code>{entry.run_ref}</code>
                        </td>
                        <td>
                          {/* The link renders ONLY as the Core reports it;
                              missing linkage renders Unavailable — never an
                              inferred link (RUN-LINK-3). */}
                          {entry.source_revision === undefined || entry.source_revision === "" ? (
                            <span className={styles.muted}>{unavailable}</span>
                          ) : (
                            <code>{entry.source_revision}</code>
                          )}
                        </td>
                        <td>
                          <span className={`${styles.chip} ${styles[linkage.tone]}`}>
                            <LinkageIcon aria-hidden="true" size={14} />
                            {t(linkage.labelKey)}
                          </span>
                        </td>
                        <td>
                          {entry.created_at === undefined ? (
                            <span className={styles.muted}>{unavailable}</span>
                          ) : (
                            <time dateTime={entry.created_at}>
                              {formatTimestamp(new Date(entry.created_at), locale)}
                            </time>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )
          ) : null}
        </section>
      ) : null}

      {activeTab === "permissions" ? (
        <section
          role="tabpanel"
          id="workflow-panel-permissions"
          aria-labelledby="workflow-tab-permissions"
          className={styles.panel}
        >
          {/* P4.7 §3.2's display law: the node contract fields render ONLY
              where the Core supplies them; no per-node contract is
              supplied yet, so the honest absence renders — nothing is
              invented. */}
          <div className={styles.emptyState}>
            <p>{t("workflow.detail.permissionsNote")}</p>
          </div>
        </section>
      ) : null}
    </PageLayout>
  );
}
