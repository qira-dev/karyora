import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type { KnowledgePack } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./KnowledgeLibraryPage.module.css";
import { CopyIdButton } from "./knowledgeStates";
import { KnowledgeImportWizard } from "./KnowledgeImportWizard";

/**
 * `/knowledge` — the Knowledge library list (B1-U1; the DE-0 queue row 2,
 * extending P4.1 K1–K6 against the now-live B1 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/knowledgepacks?limit=&cursor=` (the published list
 *   operation's serving). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented).
 * - The columns are the P4.1 K1 set (Pack / Version / Sources / Updated /
 *   Integrity-freshness) rendered per the P4.3 K1 law: every value is
 *   Core-reported; the served pack carries no Sources member and no
 *   integrity/freshness member, so those cells render `Unavailable` —
 *   never zero, never inferred.
 * - Search/filter/sort (K2): the served list declares no searchable,
 *   facet, or sortable fields, so NO speculative facet, sort, or search
 *   control is shown as working — the control state renders the accurate
 *   unavailable reason (P4.3 K2 rule 2; UI §14.6).
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed D-35 refusal surfaced verbatim, or the truthful
 *   unknown (a lost or uninterpretable response — never shown as an
 *   empty list). No fabricated rows, counts, totals, or optimistic
 *   states exist here.
 * - The import entry (I1's toolbar pointer row) opens the W-2 wizard; a
 *   completed import refreshes the confirmed list.
 * - Stale labeling (P1.7 §5.3): the last-confirmed line states when the
 *   displayed data was last confirmed with the Core.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function KnowledgeLibraryPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly KnowledgePack[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);
  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a refresh supersedes an in-flight read instead of being
  // dropped; nothing is ever applied twice.
  const readSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const importTriggerRef = useRef<HTMLButtonElement | null>(null);

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

      void transport.knowledgePackList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.packs] : outcome.data.packs));
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
        return t("knowledge.list.loading");
      case "failure":
        return t("knowledge.list.failureTitle");
      case "unknown":
        return t("knowledge.list.unknownTitle");
      default:
        return rows.length === 0 ? t("knowledge.list.emptyAll") : t("knowledge.list.heading");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("knowledge.list.heading")}</PageHeading>
      <p className={styles.description}>{t("knowledge.list.description")}</p>
      <div className={styles.toolbar}>
        <button
          type="button"
          ref={importTriggerRef}
          className={styles.primaryControl}
          onClick={() => setWizardOpen(true)}
        >
          {t("knowledge.list.importSource")}
        </button>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("knowledge.list.refreshing") : t("knowledge.list.refresh")}
        </button>
        {/* K2: the served list declares no searchable/facet/sort fields,
            so no speculative search, facet, or sort control is offered —
            the control's absence is the accurate unavailable state, and
            this note states the reason accessibly (§14.6). */}
        <p className={styles.filterNote}>{t("knowledge.list.searchFilterUnavailable")}</p>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("knowledge.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("knowledge.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("knowledge.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>{t("knowledge.list.emptyAll")}</p>
            <p>{t("knowledge.list.emptyAllBody")}</p>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th scope="col">{t("knowledge.list.columnPack")}</th>
                  <th scope="col">{t("knowledge.list.columnPackId")}</th>
                  <th scope="col">{t("knowledge.list.columnVersion")}</th>
                  <th scope="col">{t("knowledge.list.columnSources")}</th>
                  <th scope="col">{t("knowledge.list.columnUpdated")}</th>
                  <th scope="col">{t("knowledge.list.columnIntegrityFreshness")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((pack) => (
                  <PackRow key={pack.pack_id} pack={pack} />
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
                  {loadingMore ? t("knowledge.list.loadingMore") : t("knowledge.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("knowledge.list.failureTitle")}
            body={t("knowledge.list.failureBody")}
            note={t("knowledge.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("knowledge.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("knowledge.list.unknownTitle")}
          body={t("knowledge.list.unknownBody")}
          note={t("knowledge.list.connectionHint")}
          actions={[{ label: t("knowledge.list.refresh"), onAction: refresh }]}
        />
      ) : null}

      <KnowledgeImportWizard
        open={wizardOpen}
        onClose={() => {
          setWizardOpen(false);
          // The wizard's close returns focus to its trigger (the Dialog
          // does this); the list re-reads so a completed import's pack
          // appears from the Core's own confirmed state — never from a
          // local assumption about what the import did.
          refresh();
        }}
      />
    </PageLayout>
  );
}

function PackRow({ pack }: { pack: KnowledgePack }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  return (
    <tr className={styles.row}>
      <td className={styles.cellPack}>
        <Link to={`/knowledge/packs/${encodeURIComponent(pack.pack_id)}`} className={styles.rowLink}>
          {pack.title}
          <span className="visually-hidden">{t("knowledge.list.openPack", { packId: pack.pack_id })}</span>
        </Link>
      </td>
      <td className={styles.cellId}>
        <CopyIdButton
          id={pack.pack_id}
          label={t("knowledge.list.copyPackId")}
          copiedLabel={t("knowledge.list.copied")}
          failedLabel={t("knowledge.list.copyFailed")}
          className={styles.copyId}
        />
      </td>
      <td className={styles.cell}>
        {pack.current_version_id === "" ? (
          <span className={styles.unavailable}>{t("knowledge.list.noVersionStoredYet")}</span>
        ) : (
          <code className={styles.versionId}>{pack.current_version_id}</code>
        )}
      </td>
      {/* The served pack carries no Sources member: the cell renders
          Unavailable — never zero, never inferred (the P4.3 K1 law). */}
      <td className={styles.cell}>
        <span className={styles.unavailable}>{unavailable}</span>
      </td>
      <td className={styles.cell}>
        <time dateTime={pack.updated_at}>{formatTimestamp(new Date(pack.updated_at), locale)}</time>
      </td>
      {/* The served pack carries no integrity/freshness member: the cell
          renders Unavailable — never zero, never inferred. */}
      <td className={styles.cell}>
        <span className={styles.unavailable}>{unavailable}</span>
      </td>
    </tr>
  );
}
