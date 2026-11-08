import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, WorkflowDefinition } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./WorkflowLibraryPage.module.css";
import { CopyIdButton, fieldOrUnavailable } from "./workflowStates";

/**
 * `/workflows` — the Workflow list (B4-U1; the DE-0 queue row 7, the
 * P4.1 W1–W6 rows against the now-live B4 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/workflows?limit=&cursor=` (the published list member's
 *   serving, B4-A1 §2.1 #1). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented).
 * - The columns render Core-reported values only (P4.5 §3.3's map law):
 *   the name renders as the Core's display_name where one exists (an
 *   untitled definition is stated honestly, never renamed); the current
 *   revision renders as the Core's current_revision_ref where one exists
 *   (a definition with no saved revision is stated as exactly that —
 *   never inferred); the recorded W1 run/status cells render Unavailable
 *   with the page's note — the definition record carries no status
 *   member, and no run data exists while the D-40 gate withholds Run
 *   execution (never zero, never inferred).
 * - W2 (`New workflow`), W3 (`Import`), and every creation/editing
 *   control are HIDDEN ENTIRELY: the UI-30 interaction gate is open (the
 *   Roadmap decision row; P4.1 §4's UI-30 gate row withholds W2), the
 *   Core serves no workflow-import member (W3 is shown only if
 *   supported), and no dead stub renders anywhere (§31.4). The gate note
 *   (W6) states the accurate recorded reason.
 * - W4 row select navigates the ONE canonical detail route (X-2); row
 *   identity is the Core-minted definition ID. A definition's content is
 *   user-authored data — the identity rides the path; the content never
 *   does.
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed refusal surfaced verbatim, or the truthful unknown (a
 *   lost or uninterpretable response — never shown as an empty list). No
 *   fabricated rows, counts, totals, or optimistic states exist here.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function WorkflowLibraryPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly WorkflowDefinition[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a refresh supersedes an in-flight read instead of being
  // dropped; nothing is ever applied twice.
  const readSeqRef = useRef(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const runRead = useCallback(
    (options: { readonly append: boolean; readonly cursor?: string; readonly markRefreshing: boolean }) => {
      const seq = ++readSeqRef.current;
      if (options.markRefreshing) setRefreshing(true);
      if (!options.append && !options.markRefreshing) setListPhase("loading");

      const params: { cursor?: string } = {};
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.workflowList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) =>
            options.append ? [...prev, ...outcome.data.definitions] : outcome.data.definitions,
          );
          // A response without the cursor key IS the final page: null
          // ends pagination (the coded end marker — no has_more exists).
          setNextCursor(outcome.data.next_cursor ?? null);
          setLastConfirmedAt(new Date());
          setListPhase("data");
        } else if (outcome.kind === "rejection") {
          setFailure(outcome.envelope);
          setListPhase("failure");
        } else {
          setListPhase("unknown");
        }
      });
    },
    [transport],
  );

  // The first read on mount.
  useEffect(() => {
    setRows([]);
    setNextCursor(null);
    setFailure(null);
    setLastConfirmedAt(null);
    runRead({ append: false, markRefreshing: false });
  }, [runRead]);

  const loadMore = () => {
    if (nextCursor === null || loadingMore || refreshing) return;
    setLoadingMore(true);
    runRead({ append: true, cursor: nextCursor, markRefreshing: false });
  };

  const refresh = useCallback(() => {
    if (refreshing || loadingMore) return;
    setFailure(null);
    // A refresh keeps the confirmed rows rendered (stale-labeled by the
    // last-confirmed line) and re-reads the FIRST page.
    runRead({ append: false, markRefreshing: true });
  }, [loadingMore, refreshing, runRead]);

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("workflow.list.loading");
      case "failure":
        return t("workflow.list.failureTitle");
      case "unknown":
        return t("workflow.list.unknownTitle");
      default:
        return rows.length === 0 ? t("workflow.list.emptyAll") : t("workflow.list.heading");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("workflow.list.heading")}</PageHeading>
      <p className={styles.description}>{t("workflow.list.description")}</p>
      {/* W6 — the surface gate state (the accurate recorded reason; the
          gate rows' "no dead stub" law): the reads render; the
          creation/editing controls are hidden, not disabled look-alikes. */}
      <p className={styles.gateNote}>{t("workflow.list.gateNote")}</p>
      <div className={styles.toolbar}>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("workflow.list.refreshing") : t("workflow.list.refresh")}
        </button>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("workflow.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("workflow.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>{t("workflow.list.emptyAll")}</p>
            <p>{t("workflow.list.emptyAllBody")}</p>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <caption className="visually-hidden">{t("workflow.list.description")}</caption>
              <thead>
                <tr>
                  <th scope="col">{t("workflow.list.columnDefinition")}</th>
                  <th scope="col">{t("workflow.list.columnDefinitionId")}</th>
                  <th scope="col">{t("workflow.list.columnRevision")}</th>
                  <th scope="col">{t("workflow.list.columnLastRun")}</th>
                  <th scope="col">{t("workflow.list.columnStatus")}</th>
                  <th scope="col">{t("workflow.list.columnCreated")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((definition) => (
                  <WorkflowRow key={definition.definition_id} definition={definition} />
                ))}
              </tbody>
            </table>
            <p className={styles.tableNote}>{t("workflow.list.runStatusNote")}</p>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? t("workflow.list.loadingMore") : t("workflow.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("workflow.list.failureTitle")}
            body={t("workflow.list.failureBody")}
            note={t("workflow.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("workflow.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("workflow.list.unknownTitle")}
          body={t("workflow.list.unknownBody")}
          note={t("workflow.list.connectionHint")}
          actions={[{ label: t("workflow.list.refresh"), onAction: refresh }]}
        />
      ) : null}
    </PageLayout>
  );
}

function WorkflowRow({ definition }: { definition: WorkflowDefinition }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const shownName = fieldOrUnavailable(definition.display_name, t("workflow.list.untitledDefinition"));

  return (
    <tr className={styles.row}>
      <td className={styles.cellName}>
        <Link to={`/workflows/${encodeURIComponent(definition.definition_id)}`} className={styles.rowLink}>
          <span className={styles.nameText}>{shownName}</span>
          <span className="visually-hidden">
            {t("workflow.list.openDefinition", { definitionId: definition.definition_id })}
          </span>
        </Link>
      </td>
      <td className={styles.cellId}>
        <CopyIdButton
          id={definition.definition_id}
          label={t("workflow.list.copyDefinitionId")}
          copiedLabel={t("workflow.list.copied")}
          failedLabel={t("workflow.list.copyFailed")}
          className={styles.copyId}
        />
      </td>
      <td className={styles.cell}>
        {/* The current revision renders as the Core reports it; a
            definition with no saved revision is stated as exactly that —
            never inferred (the P4.5 §3.3 map's law). */}
        {definition.current_revision_ref === undefined || definition.current_revision_ref === "" ? (
          <span className={styles.mutedCell}>{t("workflow.list.revisionNone")}</span>
        ) : (
          <code>{definition.current_revision_ref}</code>
        )}
      </td>
      {/* The recorded W1 run/status cells render Unavailable: the
          definition record carries no such member, and no run data exists
          while the D-40 gate withholds Run execution — never zero, never
          inferred. */}
      <td className={styles.cell}>
        <span className={styles.mutedCell}>{unavailable}</span>
      </td>
      <td className={styles.cell}>
        <span className={styles.mutedCell}>{unavailable}</span>
      </td>
      <td className={styles.cell}>
        <time dateTime={definition.created_at}>{formatTimestamp(new Date(definition.created_at), locale)}</time>
      </td>
    </tr>
  );
}
