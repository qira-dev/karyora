import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, MemoryItemServed } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import { MemoryAddDialog } from "./MemoryAddDialog";
import { MemoryExportDialog } from "./MemoryExportDialog";
import styles from "./MemoryLibraryPage.module.css";
import {
  CODED_MEMORY_LIFECYCLE_STATES,
  CODED_MEMORY_SCOPES,
  CopyIdButton,
  memoryLifecycleChip,
  memoryOriginLabelKey,
  memoryScopeLabelKey,
} from "./memoryStates";

/**
 * `/memory` — the Memory list (B2-U1; the DE-0 queue row 4, extending
 * P4.1 M1–M6 against the now-live B2 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/memories?limit=&scope=&state=&cursor=` (the published list
 *   operation's serving). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented). Removed items are not served by the
 *   Core, so nothing renders for them anywhere.
 * - The SEL-8 scope filter and the pending-review filter (M1) operate
 *   ONLY over the Core's own closed registers: the filter options are
 *   the published register values, the filters are applied Core-side,
 *   and an unknown Core-refused value surfaces verbatim — the UI never
 *   invents a scope name and never maps an unknown reported scope to a
 *   known one (P4.5 §3.2 SC-4; the scope never silently broadens).
 * - The columns render Core-reported values only: the state renders as
 *   the Core's lifecycle register reports it (pending review is a
 *   review state, NEVER a trust claim; expiry/forgotten/deleted stay
 *   distinct); the pin flag renders as the Core returns it (a flag —
 *   B2-A1 §2.6 item 3); missing optional members render Unavailable —
 *   never zero, never inferred (the P4.5 §3.3 map's law).
 * - Row identity is the Core-owned item ID (X-4); row select navigates
 *   the ONE canonical detail route (M4, X-2). Memory content is
 *   sensitive personal/conversational data — it is never leaked into
 *   URLs (the identity rides the path; the content never does).
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed D-35 refusal surfaced verbatim, or the truthful
 *   unknown (a lost or uninterpretable response — never shown as an
 *   empty list). No fabricated rows, counts, totals, or optimistic
 *   states exist here.
 * - The add entry (M2) opens the add dialog; the export entry (the
 *   family's served export member) opens the export dialog; a completed
 *   add/refresh re-reads the Core's confirmed state — never a local
 *   assumption about what the operation did.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function MemoryLibraryPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly MemoryItemServed[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [scopeFilter, setScopeFilter] = useState("");
  const [stateFilter, setStateFilter] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a refresh supersedes an in-flight read instead of being
  // dropped; nothing is ever applied twice.
  const readSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const addTriggerRef = useRef<HTMLButtonElement | null>(null);
  const exportTriggerRef = useRef<HTMLButtonElement | null>(null);

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

      const params: { scope?: string; state?: string; cursor?: string } = {};
      if (scopeFilter !== "") params.scope = scopeFilter;
      if (stateFilter !== "") params.state = stateFilter;
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.memoryList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.items] : outcome.data.items));
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
    [scopeFilter, stateFilter, transport],
  );

  // The first read on mount and every filter change: the filters are
  // Core-side query parameters — a change re-reads the FIRST page.
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

  const pendingOnly = stateFilter === "pending_review";

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("memory.list.loading");
      case "failure":
        return t("memory.list.failureTitle");
      case "unknown":
        return t("memory.list.unknownTitle");
      default:
        return rows.length === 0
          ? pendingOnly
            ? t("memory.list.emptyPending")
            : t("memory.list.emptyAll")
          : t("memory.list.heading");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("memory.list.heading")}</PageHeading>
      <p className={styles.description}>{t("memory.list.description")}</p>
      <div className={styles.toolbar}>
        <button
          type="button"
          ref={addTriggerRef}
          className={styles.primaryControl}
          onClick={() => setAddOpen(true)}
        >
          {t("memory.list.addItem")}
        </button>
        <button
          type="button"
          ref={exportTriggerRef}
          className={styles.control}
          onClick={() => setExportOpen(true)}
          disabled={listPhase !== "data"}
        >
          {t("memory.list.exportItems")}
        </button>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("memory.list.refreshing") : t("memory.list.refresh")}
        </button>
        {/* M1's SEL-8 scope filter and the pending-review filter: their
            option sets are the Core's own closed registers, applied
            Core-side — the UI invents no scope name and never maps an
            unknown reported scope (P4.5 §3.2). */}
        <label className={styles.filterLabel} htmlFor="memory-scope-filter">
          {t("memory.list.scopeFilterLabel")}
        </label>
        <select
          id="memory-scope-filter"
          className={styles.filterSelect}
          value={scopeFilter}
          onChange={(event) => setScopeFilter(event.target.value)}
        >
          <option value="">{t("memory.list.scopeFilterAll")}</option>
          {CODED_MEMORY_SCOPES.map((value) => {
            const labelKey = memoryScopeLabelKey(value);
            return (
              <option key={value} value={value}>
                {labelKey !== null ? t(labelKey) : value}
              </option>
            );
          })}
        </select>
        <label className={styles.filterLabel} htmlFor="memory-state-filter">
          {t("memory.list.stateFilterLabel")}
        </label>
        <select
          id="memory-state-filter"
          className={styles.filterSelect}
          value={stateFilter}
          onChange={(event) => setStateFilter(event.target.value)}
        >
          <option value="">{t("memory.list.stateFilterAll")}</option>
          {CODED_MEMORY_LIFECYCLE_STATES.map((value) => (
            <option key={value} value={value}>
              {t(memoryLifecycleChip(value).labelKey)}
            </option>
          ))}
        </select>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("memory.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("memory.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("memory.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>
              {pendingOnly ? t("memory.list.emptyPending") : t("memory.list.emptyAll")}
            </p>
            <p>
              {pendingOnly ? t("memory.list.emptyPendingBody") : t("memory.list.emptyAllBody")}
            </p>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th scope="col">{t("memory.list.columnContent")}</th>
                  <th scope="col">{t("memory.list.columnItemId")}</th>
                  <th scope="col">{t("memory.list.columnScope")}</th>
                  <th scope="col">{t("memory.list.columnOrigin")}</th>
                  <th scope="col">{t("memory.list.columnState")}</th>
                  <th scope="col">{t("memory.list.columnPinned")}</th>
                  <th scope="col">{t("memory.list.columnUpdated")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((item) => (
                  <MemoryRow key={item.memory_item_id} item={item} />
                ))}
              </tbody>
            </table>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? t("memory.list.loadingMore") : t("memory.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("memory.list.failureTitle")}
            body={t("memory.list.failureBody")}
            note={t("memory.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("memory.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("memory.list.unknownTitle")}
          body={t("memory.list.unknownBody")}
          note={t("memory.list.connectionHint")}
          actions={[{ label: t("memory.list.refresh"), onAction: refresh }]}
        />
      ) : null}

      <MemoryAddDialog
        open={addOpen}
        onClose={() => {
          setAddOpen(false);
          // The dialog's close returns focus to its trigger (the Dialog
          // does this); the list re-reads so a completed add appears from
          // the Core's own confirmed state — never from a local
          // assumption about what the add did.
          refresh();
        }}
      />
      <MemoryExportDialog
        open={exportOpen}
        onClose={() => {
          setExportOpen(false);
          exportTriggerRef.current?.focus();
        }}
      />
    </PageLayout>
  );
}

function MemoryRow({ item }: { item: MemoryItemServed }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const lifecycleChip = memoryLifecycleChip(item.lifecycle_state);
  const LifecycleIcon = lifecycleChip.icon;
  const scopeKey = memoryScopeLabelKey(item.scope);
  const originKey = memoryOriginLabelKey(item.origin);

  return (
    <tr className={styles.row}>
      <td className={styles.cellItem}>
        <Link to={`/memory/${encodeURIComponent(item.memory_item_id)}`} className={styles.rowLink}>
          <span className={styles.contentExcerpt}>{item.content}</span>
          <span className="visually-hidden">{t("memory.list.openItem", { itemId: item.memory_item_id })}</span>
        </Link>
      </td>
      <td className={styles.cellId}>
        <CopyIdButton
          id={item.memory_item_id}
          label={t("memory.list.copyItemId")}
          copiedLabel={t("memory.list.copied")}
          failedLabel={t("memory.list.copyFailed")}
          className={styles.copyId}
        />
      </td>
      <td className={styles.cell}>
        {/* A scope value renders ONLY as the Core reports it — an unknown
            reported scope renders verbatim, never mapped (P4.5 §3.2 SC-4). */}
        {scopeKey !== null ? t(scopeKey) : item.scope}
      </td>
      <td className={styles.cell}>
        {/* The origin is labeled honestly as the Core reports it — never
            relabeled silently (P4.5 §3.3 row 3). */}
        {originKey !== null ? t(originKey) : item.origin}
      </td>
      <td className={styles.cell}>
        <span className={`${styles.chip} ${styles[lifecycleChip.tone]}`}>
          <LifecycleIcon aria-hidden="true" size={14} />
          {t(lifecycleChip.labelKey)}
        </span>
      </td>
      <td className={styles.cell}>
        {/* The pin flag renders as the Core returns it — this Core's
            served pin-register position (B2-A1 §2.6 item 3); never a
            lifecycle state and never an invented badge. */}
        {item.pinned ? t("memory.list.pinnedTrue") : t("memory.list.pinnedFalse")}
      </td>
      <td className={styles.cell}>
        <time dateTime={item.updated_at}>{formatTimestamp(new Date(item.updated_at), locale)}</time>
      </td>
    </tr>
  );
}
