import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type { WireJob } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./JobsListPage.module.css";
import { CODED_JOB_STATES, CopyIdButton, jobStateChip } from "./jobStates";

/**
 * `/jobs` — the Jobs list (IMPL-5; the UIR-5 server-paginated list over
 * the delivered local transport surface).
 *
 * What this page consumes (exactly as the Core delivered it):
 * - `GET /v1/jobs?limit=&status=&cursor=` (the card-9 working
 *   candidate's list operation, IMPL-4). No client-side limit is sent —
 *   the Core's per-surface default applies; the status filter values are
 *   ONLY the coded Job-state registry (filter options come from the
 *   Core's vocabulary, never invented client-side, P3.7 §3.1); the
 *   opaque cursor rides back verbatim and a cursorless response IS the
 *   final page (the coded end marker — no has_more/total is invented).
 * - Every state below is driven by a real transport outcome: loading
 *   (read in flight), data (the confirmed page(s)), the typed D-35
 *   refusal surfaced verbatim, or the truthful unknown (a lost or
 *   uninterpretable response — never shown as an empty list). No
 *   fabricated rows, counts, totals, or optimistic states exist here.
 * - Empty is split exactly as the records bind (P3.7 §3.1): no jobs at
 *   all vs no matches for the active filter — each with its own
 *   recovery affordance (the filter's clear/reset).
 * - Stale labeling (P1.7 §5.3): the last-confirmed line states when the
 *   displayed data was last confirmed with the Core; a refresh in
 *   flight keeps the prior rows visible and labeled — never silently
 *   authoritative, never blanked mid-refresh.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function JobsListPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly WireJob[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a filter change supersedes an in-flight read instead of
  // being dropped; nothing is ever applied twice.
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

      const params: { status?: string; cursor?: string } = {};
      if (statusFilter !== "") params.status = statusFilter;
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.list(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.jobs] : outcome.data.jobs));
          // A response without the cursor key IS the final page: null
          // ends pagination (the coded end marker — no has_more exists).
          setNextCursor(outcome.data.cursor ?? null);
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
    [statusFilter, transport],
  );

  // The first read on mount and on every filter change (server-side
  // filtering — the client never filters rows itself).
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

  const refresh = () => {
    if (refreshing || loadingMore) return;
    setFailure(null);
    // A refresh keeps the confirmed rows rendered (stale-labeled by the
    // last-confirmed line) and re-reads the FIRST page — a later cursor
    // page is re-walked by explicit user action only.
    runRead({ append: false, markRefreshing: true });
  };

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("jobs.list.loading");
      case "failure":
        return t("jobs.list.failureTitle");
      case "unknown":
        return t("jobs.list.unknownTitle");
      default:
        return rows.length === 0
          ? statusFilter === ""
            ? t("jobs.list.emptyAll")
            : t("jobs.list.emptyFiltered")
          : t("jobs.list.rowsCount", { count: rows.length });
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("jobs.title")}</PageHeading>
      <div className={styles.toolbar}>
        <label className={styles.filterLabel} htmlFor="jobs-status-filter">
          {t("jobs.list.filterLabel")}
        </label>
        <select
          id="jobs-status-filter"
          className={styles.filterSelect}
          value={statusFilter}
          onChange={(event) => setStatusFilter(event.target.value)}
        >
          <option value="">{t("jobs.list.filterAll")}</option>
          {CODED_JOB_STATES.map((state) => (
            <option key={state} value={state}>
              {t(jobStateChip(state).labelKey)} ({state})
            </option>
          ))}
        </select>
        {statusFilter !== "" ? (
          <button type="button" className={styles.control} onClick={() => setStatusFilter("")}>
            {t("jobs.list.clearFilter")}
          </button>
        ) : null}
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("jobs.list.refreshing") : t("jobs.list.refresh")}
        </button>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("jobs.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("jobs.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("jobs.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          statusFilter === "" ? (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("jobs.list.emptyAll")}</p>
              <p>{t("jobs.list.emptyAllBody")}</p>
            </div>
          ) : (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("jobs.list.emptyFiltered")}</p>
              <p>{t("jobs.list.emptyFilteredBody")}</p>
              <button type="button" className={styles.control} onClick={() => setStatusFilter("")}>
                {t("jobs.list.clearFilter")}
              </button>
            </div>
          )
        ) : (
          <>
            <ul className={styles.rows}>
              {rows.map((job) => (
                <JobRow key={job.job_id} job={job} />
              ))}
            </ul>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? t("jobs.list.loadingMore") : t("jobs.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("jobs.list.failureTitle")}
            body={t("jobs.list.failureBody")}
            note={t("jobs.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("jobs.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("jobs.list.unknownTitle")}
          body={t("jobs.list.unknownBody")}
          note={t("jobs.list.connectionHint")}
          actions={[{ label: t("jobs.list.refresh"), onAction: refresh }]}
        />
      ) : null}
    </PageLayout>
  );
}

function JobRow({ job }: { job: WireJob }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const chip = jobStateChip(job.state);

  return (
    <li className={styles.row}>
      <div className={styles.rowId}>
        <Link to={`/jobs/${encodeURIComponent(job.job_id)}`} className={styles.rowLink}>
          <code className={styles.jobId}>{job.job_id}</code>
          <span className="visually-hidden">{t("jobs.list.openJob", { jobId: job.job_id })}</span>
        </Link>
        <CopyIdButton
          id={job.job_id}
          label={t("jobs.list.copyJobId")}
          copiedLabel={t("jobs.list.copied")}
          failedLabel={t("jobs.list.copyFailed")}
          className={styles.copyId}
        />
      </div>
      <span className={`${styles.chip} ${styles[chip.tone]}`}>
        <chip.icon aria-hidden="true" size={14} />
        {t(chip.labelKey)}
      </span>
      <span className={styles.rowTime}>
        <span className="visually-hidden">{t("jobs.list.columnCreated")}: </span>
        <time dateTime={job.created_at}>{formatTimestamp(new Date(job.created_at), locale)}</time>
      </span>
      <span className={styles.rowTime}>
        <span className="visually-hidden">{t("jobs.list.columnUpdated")}: </span>
        <time dateTime={job.updated_at}>{formatTimestamp(new Date(job.updated_at), locale)}</time>
      </span>
    </li>
  );
}
