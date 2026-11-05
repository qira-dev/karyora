import { useCallback, useEffect, useRef, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, WireAuditEntry } from "../../coreclient/httpLocal";
import { CopyIdButton } from "../jobs/jobStates";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./ActivityPage.module.css";

/**
 * `/activity` — the Activity & Audit surface (IMPL-24; the UIR-5 Activity
 * half, the S4 §20 F12 freeze act's frozen content + the adopted S11
 * supply's classes as implemented).
 *
 * What this page consumes (exactly as the Core delivered it):
 * - `GET /v1/audit?limit=&cursor=&policy_decision=` — the
 *   permission-filtered, server-side, paginated audit list (the card-9
 *   working candidate's class). The filter values are ONLY the Core's
 *   coded allow/deny register (filter options come from the Core, never
 *   invented client-side); the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page.
 * - `GET /v1/audit/export?from=&to=&limit=` — the M-32-class
 *   read-with-export (the adopted P-S11-03 class as implemented). The
 *   scope/count/date-range confirmation runs BEFORE the request (P1.6's
 *   precondition); the export is a point-in-time read that mutates
 *   nothing and adds no tamper-resistance — the honesty note renders
 *   with every result; no integrity receipt is claimed anywhere.
 * - The READ-ONLY law binds the surface (the [S] rules): no dismiss,
 *   no delete, and NO correction control exists anywhere — corrections
 *   are new records only if the Core supplies a correction contract, and
 *   the honest absence is stated on the page.
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed D-35 refusal surfaced verbatim, or the truthful
 *   unknown (a lost response is never shown as an empty list). Stale
 *   labeling (P1.7 §5.3): the last-confirmed line states when the
 *   displayed data was last confirmed with the Core.
 * - Raw IDs are selectable/copyable (P1.1 §2.7.1); the records carry
 *   digests and identifiers only — never content, never secret values.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

/** The Core's coded policy_decision register — the filter's closed
 *  vocabulary (filter options come from the Core only). */
const CODED_POLICY_DECISIONS = ["allow", "deny"] as const;

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

/** Renders a Core-set RFC 3339 record time in the viewer's locale, or the
 *  raw value when it cannot be parsed (never a fabricated fallback). */
function formatRecordTime(value: string, locale: string | undefined): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return formatTimestamp(date, locale);
}

type ExportOutcome =
  | { readonly kind: "confirmed"; readonly count: number; readonly truncated: boolean }
  | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" }
  | null;

export function ActivityPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly WireAuditEntry[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [decisionFilter, setDecisionFilter] = useState<string>("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  // The export dialog's own state: the confirmation flow, the range the
  // user confirms, and the outcome rendered after.
  const [exportOpen, setExportOpen] = useState(false);
  const [exportPending, setExportPending] = useState(false);
  const [exportFrom, setExportFrom] = useState("");
  const [exportTo, setExportTo] = useState("");
  const [exportOutcome, setExportOutcome] = useState<ExportOutcome>(null);
  const exportTriggerRef = useRef<HTMLButtonElement>(null);
  const exportLabelId = useDialogLabelId();

  // The read-sequence guard: only the LATEST issued read applies its
  // result; a filter change supersedes an in-flight read.
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

      const params: { policyDecision?: string; cursor?: string } = {};
      if (decisionFilter !== "") params.policyDecision = decisionFilter;
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.auditList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.entries] : outcome.data.entries));
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
    [decisionFilter, transport],
  );

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
    runRead({ append: false, markRefreshing: true });
  };

  const runExport = useCallback(() => {
    if (exportPending) return;
    setExportPending(true);
    const params: { from?: string; to?: string } = {};
    if (exportFrom.trim() !== "") params.from = exportFrom.trim();
    if (exportTo.trim() !== "") params.to = exportTo.trim();
    void transport
      .auditExport(params)
      .then((outcome) => {
        if (outcome.kind === "data") {
          // The confirmed 200 IS the export: the JSON document downloads
          // from the response the authorized session received (the
          // client-side secure download — no server-side artifact, no
          // extra copy, no URL carrying anything sensitive).
          const exportJson = JSON.stringify(outcome.data, null, 2);
          const blob = new Blob([exportJson], { type: "application/json" });
          const url = URL.createObjectURL(blob);
          const anchor = document.createElement("a");
          anchor.href = url;
          anchor.download = `karyora-audit-export-${outcome.data.generated_at.replace(/[:.]/g, "-")}.json`;
          document.body.appendChild(anchor);
          anchor.click();
          anchor.remove();
          URL.revokeObjectURL(url);
          setExportOpen(false);
          setExportOutcome({ kind: "confirmed", count: outcome.data.count, truncated: outcome.data.truncated });
        } else if (outcome.kind === "rejection") {
          setExportOpen(false);
          setExportOutcome({ kind: "refused", envelope: outcome.envelope });
        } else {
          setExportOpen(false);
          setExportOutcome({ kind: "unknown" });
        }
      })
      .finally(() => setExportPending(false));
  }, [exportPending, exportFrom, exportTo, transport]);

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("activity.list.loading");
      case "failure":
        return t("activity.list.failureTitle");
      case "unknown":
        return t("activity.list.unknownTitle");
      default:
        return rows.length === 0
          ? decisionFilter === ""
            ? t("activity.list.emptyAll")
            : t("activity.list.emptyFiltered")
          : t("activity.list.rowsCount", { count: rows.length });
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("activity.title")}</PageHeading>
      <div className={styles.toolbar}>
        <label className={styles.filterLabel} htmlFor="activity-decision-filter">
          {t("activity.list.filterLabel")}
        </label>
        <select
          id="activity-decision-filter"
          className={styles.filterSelect}
          value={decisionFilter}
          onChange={(event) => setDecisionFilter(event.target.value)}
        >
          <option value="">{t("activity.list.filterAll")}</option>
          {CODED_POLICY_DECISIONS.map((decision) => (
            <option key={decision} value={decision}>
              {decision === "allow" ? t("activity.list.filterAllow") : t("activity.list.filterDeny")} ({decision})
            </option>
          ))}
        </select>
        {decisionFilter !== "" ? (
          <button type="button" className={styles.control} onClick={() => setDecisionFilter("")}>
            {t("activity.list.clearFilter")}
          </button>
        ) : null}
        <button
          type="button"
          className={styles.control}
          ref={exportTriggerRef}
          disabled={exportPending}
          aria-disabled={exportPending}
          onClick={() => {
            setExportOutcome(null);
            setExportOpen(true);
          }}
        >
          {t("activity.export.action")}
        </button>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("activity.list.refreshing") : t("activity.list.refresh")}
        </button>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("activity.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("activity.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("activity.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          decisionFilter === "" ? (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("activity.list.emptyAll")}</p>
              <p>{t("activity.list.emptyAllBody")}</p>
            </div>
          ) : (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("activity.list.emptyFiltered")}</p>
              <p>{t("activity.list.emptyFilteredBody")}</p>
              <button type="button" className={styles.control} onClick={() => setDecisionFilter("")}>
                {t("activity.list.clearFilter")}
              </button>
            </div>
          )
        ) : (
          <>
            <ul className={styles.rows}>
              {rows.map((entry) => (
                <AuditRow key={entry.record.audit_id} entry={entry} locale={locale} />
              ))}
            </ul>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button type="button" className={styles.control} onClick={loadMore} disabled={loadingMore}>
                  {loadingMore ? t("activity.list.loadingMore") : t("activity.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("activity.list.failureTitle")}
            body={t("activity.list.failureBody", { code: failure.code })}
            note={t("activity.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("activity.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("activity.list.unknownTitle")}
          body={t("activity.list.unknownBody")}
          note={t("activity.list.connectionHint")}
          actions={[{ label: t("activity.list.refresh"), onAction: refresh }]}
        />
      ) : null}

      {exportOutcome?.kind === "confirmed" ? (
        <>
          <RecoveryPanel
            title={t("activity.export.confirmedTitle")}
            body={t("activity.export.confirmedBody", { count: exportOutcome.count })}
            note={
              exportOutcome.truncated ? t("activity.export.truncatedNote") : t("activity.export.integrityNote")
            }
          />
        </>
      ) : null}

      {exportOutcome?.kind === "refused" ? (
        <RecoveryPanel
          title={t("activity.export.refusedTitle")}
          body={t("activity.export.refusedBody", { code: exportOutcome.envelope.code })}
          note={t("activity.export.integrityNote")}
          referenceId={exportOutcome.envelope.correlationId}
        />
      ) : null}

      {exportOutcome?.kind === "unknown" ? (
        <RecoveryPanel
          title={t("activity.export.unknownTitle")}
          body={t("activity.export.unknownBody")}
        />
      ) : null}

      <p className={styles.readOnlyNote}>{t("activity.readOnlyNote")}</p>

      <Dialog
        labelId={exportLabelId}
        title={t("activity.export.dialogTitle")}
        open={exportOpen}
        onClose={() => setExportOpen(false)}
        returnFocusRef={exportTriggerRef}
      >
        <div className={styles.dialogBody}>
          <p>{t("activity.export.dialogScope")}</p>
          <div className={styles.rangeGrid}>
            <p className={styles.rangeLabel}>{t("activity.export.rangeLabel")}</p>
            <div className={styles.rangeRow}>
              <label className={styles.rangeLabel} htmlFor="activity-export-from">
                {t("activity.export.rangeFrom")}
              </label>
              <input
                id="activity-export-from"
                className={styles.rangeInput}
                type="text"
                value={exportFrom}
                onChange={(event) => setExportFrom(event.target.value)}
                spellCheck={false}
                autoComplete="off"
              />
            </div>
            <div className={styles.rangeRow}>
              <label className={styles.rangeLabel} htmlFor="activity-export-to">
                {t("activity.export.rangeTo")}
              </label>
              <input
                id="activity-export-to"
                className={styles.rangeInput}
                type="text"
                value={exportTo}
                onChange={(event) => setExportTo(event.target.value)}
                spellCheck={false}
                autoComplete="off"
              />
            </div>
          </div>
          <p>{t("activity.export.boundNote", { count: 1000 })}</p>
          <div className={styles.dialogActions}>
            <button
              type="button"
              className={styles.primaryButton}
              disabled={exportPending}
              aria-disabled={exportPending}
              onClick={runExport}
            >
              {t("activity.export.confirm")}
            </button>
            <button
              type="button"
              className={styles.ghostButton}
              disabled={exportPending}
              aria-disabled={exportPending}
              onClick={() => setExportOpen(false)}
            >
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      </Dialog>
    </PageLayout>
  );
}

function AuditRow({ entry, locale }: { entry: WireAuditEntry; locale: string | undefined }) {
  const t = useT();

  return (
    <li className={styles.row}>
      <div className={styles.rowMeta}>
        <span className={styles.rowTime}>
          <span className="visually-hidden">{t("activity.list.columnTime")}: </span>
          <time dateTime={entry.record.time}>{formatRecordTime(entry.record.time, locale)}</time>
        </span>
        <span className={styles.rowAction}>{entry.record.action}</span>
        <span className={styles.rowScope}>
          <span className="visually-hidden">{t("activity.list.columnScope")}: </span>
          {entry.record.scope}
        </span>
        <span className={styles.decision} data-decision={entry.record.policy_decision}>
          <span className="visually-hidden">{t("activity.list.columnDecision")}: </span>
          {entry.record.policy_decision}
        </span>
        <span className={styles.rowScope}>
          <span className="visually-hidden">{t("activity.list.columnOutcome")}: </span>
          {entry.record.outcome}
        </span>
      </div>
      <div className={styles.rowIds}>
        <span className={styles.rowTime}>
          <span className="visually-hidden">{t("activity.list.sequenceLabel")}: </span>
          #{entry.sequence}
        </span>
        <code className={styles.idCode}>{entry.record.audit_id}</code>
        <CopyIdButton
          id={entry.record.audit_id}
          label={t("activity.list.copyAuditId")}
          copiedLabel={t("activity.list.copied")}
          failedLabel={t("activity.list.copyFailed")}
          className={styles.copyId}
        />
        <code className={styles.idCode}>{entry.record.correlation_id}</code>
        <CopyIdButton
          id={entry.record.correlation_id}
          label={t("activity.list.copyCorrelationId")}
          copiedLabel={t("activity.list.copied")}
          failedLabel={t("activity.list.copyFailed")}
          className={styles.copyId}
        />
      </div>
    </li>
  );
}
