import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type { WireNotificationGroupEntry } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { CopyIdButton, CODED_JOB_STATES, jobStateChip } from "../jobs/jobStates";
import {
  CODED_KNOWN_PRESENTATIONS,
  CODED_KNOWN_STATE_CLASSES,
  CODED_NOTIFICATION_CATEGORIES,
  CODED_NOTIFICATION_READ_STATES,
  CODED_RELATED_OBJECT_JOB,
  notificationCategoryLabelKey,
  notificationPresentationChip,
  notificationStateClassChip,
} from "./notificationRegisters";
import styles from "./NotificationList.module.css";

/**
 * The notification list surface (IMPL-14; the ADOPTED S6 supply,
 * P-S6-01/02 — the working candidates they are, never finalized). One
 * component, two containers: the topbar notification-center popover
 * (`variant="popover"`) and the `/settings/notifications` page
 * (`variant="page"`).
 *
 * What this surface consumes (exactly as the Core delivered it, IMPL-13):
 * - `GET /v1/notifications?category=&read_state=&cursor=` (the grouped
 *   list read, the card-9 working candidate): no client-side limit (the
 *   Core's per-surface default applies); the filter values are ONLY the
 *   coded closed-register members (filter options come from the Core's
 *   vocabulary, never invented client-side); the opaque cursor rides
 *   back verbatim and a cursorless response IS the final page (no
 *   has_more/total is invented); an empty page IS the honest
 *   no-notification answer.
 * - `GET /v1/notifications/counts` — the Core-supplied unread counts.
 *   Unread counts render ONLY from this read — never locally counted
 *   (P3.8 §5.6).
 * - `POST /v1/notifications/read|unread|clear` — the delivered O-6
 *   mutation set, exactly: the body names EXACTLY the notification
 *   identities to mutate; NO mark-all-read exists (not in the P-S6-02
 *   named set); a mutation is all-or-nothing on the Core side. The
 *   typed outcome is never rendered as the confirmation by itself — the
 *   surface re-reads the authoritative list and counts. A lost response
 *   is `unknown` (P1.6 S4) and is NEVER re-sent: the recovery is the
 *   query (the refresh re-read), never a resend, and NO auto-retry
 *   exists anywhere. NO UI-local "seen" hint exists — the unread state
 *   renders only from the Core-supplied presentation state (P3.8 §5.6's
 *   default answer: absent until the owner roles approve it).
 * - The non-mutation law (P-S6-02; P3.8 §9): a notification read/dismiss
 *   NEVER changes the underlying Job, Activity, approval, or security
 *   record; the footer states it and the copy never presents a
 *   read/cleared item as handled, resolved, approved, or completed.
 *
 * The recorded behavior, rendered:
 * - The four-way state distinction (unread | read | stale |
 *   permission_revoked) is NON-COLOR: icon + text per the §8.6 registry
 *   discipline; the tone class supplements, never carries (P3.8 §5.4).
 * - The grouping display: one entry per related-object group — the
 *   representative (newest) identity, the newest timestamp, the group's
 *   unread count and total, and the collapsed blocks (every meaningful
 *   state transition visible; the collapse never hides one).
 * - The stale binding: a stale entry's `object_state` is empty (the Core
 *   supplies none) and renders as Unavailable — never invented; the
 *   whole display carries the last-confirmed line (P1.7 §5.3), and a
 *   refresh in flight keeps the prior rows visible and labeled.
 * - The canonical destination: a `job` object's entry links to
 *   `/jobs/:jobId` (R-1). The access recheck on open IS the
 *   destination's own authoritative read (`GET /v1/jobs/{id}` — the
 *   stale UI grant is never the authorization), and a
 *   denied/revoked/unresolvable object lands on that page's truthful
 *   NOT_FOUND/unknown state — an explained fallback, never a dead link.
 *   An unknown future object type renders NO link (no destination route
 *   exists; the R-2/R-3 families ship with their owning domains).
 * - The no-leak class: only wire fields render; no invented titles; an
 *   inaccessible object's existence is never revealed beyond the Core's
 *   own payload.
 * - Truthful unavailability (P-S6-07's own terms): no session, transport
 *   refused, and Core-unavailable states render as the distinct
 *   unavailable panels below — never a fake empty list; the honest empty
 *   state is split from them (no notifications at all vs no matches for
 *   the active filter).
 *
 * FORBIDDEN here (the recorded gates): no OS-permission request, no OS
 * delivery, no quiet hours (decision 6 — P-S6-05); no stream transport
 * (P-S6-04); no mark-all-read; no preference store (the settings page
 * states its absence — gap (11)); nothing publishes (Software Rule 13).
 */

type BlockReason = "connecting" | "noCore" | "noSession" | "sessionExpired";
type ListPhase = "loading" | "data" | "failure" | "unknown";
type CountsPhase = "loading" | "data" | "failure" | "unknown";
type MutationAction = "read" | "unread" | "clear";
type MutationState =
  | { readonly kind: "idle" }
  | { readonly kind: "pending"; readonly notificationId: string; readonly action: MutationAction }
  | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope; readonly notificationId: string; readonly action: MutationAction }
  | { readonly kind: "unknown"; readonly notificationId: string; readonly action: MutationAction };

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function NotificationList({
  variant,
  onEntryNavigate,
}: {
  variant: "popover" | "page";
  /** Called when an entry's canonical-destination link is followed —
   * the popover container closes itself on navigation. */
  onEntryNavigate?: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const { phase, transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly WireNotificationGroupEntry[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [countsPhase, setCountsPhase] = useState<CountsPhase>("loading");
  const [countsTotal, setCountsTotal] = useState<number | null>(null);
  const [categoryFilter, setCategoryFilter] = useState<string>("");
  const [readStateFilter, setReadStateFilter] = useState<string>("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [mutationState, setMutationState] = useState<MutationState>({ kind: "idle" });
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

      const params: { category?: string; read_state?: string; cursor?: string } = {};
      if (categoryFilter !== "") params.category = categoryFilter;
      if (readStateFilter !== "") params.read_state = readStateFilter;
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.notificationCounts().then((countsOutcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return;
        if (countsOutcome.kind === "data") {
          setCountsTotal(countsOutcome.data.total);
          setCountsPhase("data");
        } else if (countsOutcome.kind === "rejection") {
          setCountsPhase("failure");
        } else {
          setCountsPhase("unknown");
        }
      });

      void transport.notificationList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.entries] : outcome.data.entries));
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
    [categoryFilter, readStateFilter, transport],
  );

  // The truthful unavailability gates: the reads run ONLY while a Core
  // session is connected. No session, a refused transport, an expired
  // session, and a connection check in flight each render their own
  // truthful unavailable state — never a fake empty list (P-S6-07's own
  // terms: the unavailable state remains for the genuine cases).
  const blockReason: BlockReason | null =
    phase === "connected"
      ? null
      : phase === "unknown"
        ? "noCore"
        : phase === "disconnected"
          ? "noSession"
          : phase === "session-expired"
            ? "sessionExpired"
            : "connecting";

  useEffect(() => {
    if (blockReason !== null) return;
    setRows([]);
    setNextCursor(null);
    setFailure(null);
    setLastConfirmedAt(null);
    runRead({ append: false, markRefreshing: false });
  }, [blockReason, runRead]);

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

  const mutationPending = mutationState.kind === "pending";

  // The O-6 mutation: ONE explicit call naming exactly the notification
  // identity, then the authoritative re-read. A lost response is the S4
  // unknown state — the recovery is the query (refresh), never a resend,
  // and no auto-retry exists anywhere (P1.6; the convergent no-op is
  // state-convergence by nature, NOT an idempotency class).
  const runMutation = useCallback(
    (notificationId: string, action: MutationAction) => {
      if (mutationPending) return;
      setMutationState({ kind: "pending", notificationId, action });
      const payload = { notification_ids: [notificationId] };
      const call =
        action === "read"
          ? transport.notificationMarkRead(payload)
          : action === "unread"
            ? transport.notificationMarkUnread(payload)
            : transport.notificationClear(payload);
      void call.then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The Core-confirmed outcome is never rendered from the
          // response alone: the re-read is the display authority.
          setMutationState({ kind: "idle" });
          runRead({ append: false, markRefreshing: true });
        } else if (outcome.kind === "rejection") {
          setMutationState({ kind: "refused", envelope: outcome.envelope, notificationId, action });
        } else {
          setMutationState({ kind: "unknown", notificationId, action });
        }
      });
    },
    [mutationPending, runRead, transport],
  );

  const announce = (() => {
    if (blockReason !== null) return t(`notifications.blocked.${blockReason}`);
    switch (listPhase) {
      case "loading":
        return t("notifications.loading");
      case "failure":
        return t("notifications.failureTitle");
      case "unknown":
        return t("notifications.unknownTitle");
      default:
        return rows.length === 0
          ? categoryFilter === "" && readStateFilter === ""
            ? t("notifications.emptyAll")
            : t("notifications.emptyFiltered")
          : t("notifications.announceData", { count: rows.length });
    }
  })();

  return (
    <div className={variant === "popover" ? styles.popoverSurface : styles.pageSurface}>
      <div className={styles.toolbar}>
        <label className={styles.filterLabel} htmlFor="notifications-category-filter">
          {t("notifications.filterCategory")}
        </label>
        <select
          id="notifications-category-filter"
          className={styles.filterSelect}
          value={categoryFilter}
          onChange={(event) => setCategoryFilter(event.target.value)}
        >
          <option value="">{t("notifications.filterAll")}</option>
          {CODED_NOTIFICATION_CATEGORIES.map((category) => (
            <option key={category} value={category}>
              {t(notificationCategoryLabelKey(category))} ({category})
            </option>
          ))}
        </select>
        <label className={styles.filterLabel} htmlFor="notifications-readstate-filter">
          {t("notifications.filterReadState")}
        </label>
        <select
          id="notifications-readstate-filter"
          className={styles.filterSelect}
          value={readStateFilter}
          onChange={(event) => setReadStateFilter(event.target.value)}
        >
          <option value="">{t("notifications.filterAll")}</option>
          {CODED_NOTIFICATION_READ_STATES.map((state) => (
            <option key={state} value={state}>
              {t(notificationPresentationChip(state).labelKey)} ({state})
            </option>
          ))}
        </select>
        {categoryFilter !== "" || readStateFilter !== "" ? (
          <button
            type="button"
            className={styles.control}
            onClick={() => {
              setCategoryFilter("");
              setReadStateFilter("");
            }}
          >
            {t("notifications.clearFilter")}
          </button>
        ) : null}
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading" || blockReason !== null}
        >
          {refreshing ? t("notifications.refreshing") : t("notifications.refresh")}
        </button>
      </div>

      {blockReason === null && lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("notifications.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("notifications.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {blockReason !== null ? (
        <div className={styles.empty}>
          <p className={styles.emptyTitle}>{t(`notifications.blocked.${blockReason}`)}</p>
          <p className={styles.note}>{t("notifications.noOsDelivery")}</p>
        </div>
      ) : null}

      {blockReason === null && listPhase === "loading" ? (
        <p className={styles.note}>{t("notifications.loading")}</p>
      ) : null}

      {blockReason === null && listPhase === "data" ? (
        rows.length === 0 ? (
          categoryFilter === "" && readStateFilter === "" ? (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("notifications.emptyAll")}</p>
              <p>{t("notifications.emptyAllBody")}</p>
            </div>
          ) : (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("notifications.emptyFiltered")}</p>
              <p>{t("notifications.emptyFilteredBody")}</p>
              <button
                type="button"
                className={styles.control}
                onClick={() => {
                  setCategoryFilter("");
                  setReadStateFilter("");
                }}
              >
                {t("notifications.clearFilter")}
              </button>
            </div>
          )
        ) : (
          <>
            <p className={styles.counts}>
              {countsPhase === "data" && countsTotal !== null
                ? t("notifications.countsLine", { total: countsTotal })
                : t("notifications.countsUnavailable")}
            </p>
            <ul className={styles.rows}>
              {rows.map((entry) => (
                <NotificationEntry
                  key={entry.notification_id}
                  entry={entry}
                  onNavigate={onEntryNavigate}
                  mutationPending={mutationPending}
                  onMutate={runMutation}
                />
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
                  {loadingMore ? t("notifications.loadingMore") : t("notifications.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {blockReason === null && listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("notifications.failureTitle")}
            body={t("notifications.failureBody")}
            referenceId={failure.correlationId}
            actions={[{ label: t("notifications.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {blockReason === null && listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("notifications.unknownTitle")}
          body={t("notifications.unknownBody")}
          actions={[{ label: t("notifications.refresh"), onAction: refresh }]}
        />
      ) : null}

      {blockReason === null && mutationState.kind === "refused" ? (
        <>
          <RecoveryPanel
            title={t("notifications.actions.refusedTitle")}
            body={t("notifications.actions.refusedBody")}
            referenceId={mutationState.envelope.correlationId}
            actions={[
              {
                label: t("notifications.actions.tryAgain"),
                onAction: () => runMutation(mutationState.notificationId, mutationState.action),
              },
            ]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{mutationState.envelope.code}</code>
          </p>
        </>
      ) : null}

      {blockReason === null && mutationState.kind === "unknown" ? (
        <RecoveryPanel
          title={t("notifications.actions.unknownTitle")}
          body={t("notifications.actions.unknownBody")}
          actions={[{ label: t("notifications.actions.checkByRefresh"), onAction: refresh }]}
        />
      ) : null}

      {blockReason === null ? (
        <footer className={styles.footer}>
          <p>{t("notifications.actions.scopeNote")}</p>
          <p>{t("notifications.lawNote")}</p>
          <p>{t("notifications.noOsDelivery")}</p>
        </footer>
      ) : null}
    </div>
  );
}

/** One grouped entry: the four-way presentation chip (icon + text),
 *  the representative's state class, the collapsed blocks (every
 *  meaningful transition visible), the related object's canonical
 *  destination, and the delivered per-notification actions. */
function NotificationEntry({
  entry,
  onNavigate,
  mutationPending,
  onMutate,
}: {
  entry: WireNotificationGroupEntry;
  onNavigate?: () => void;
  mutationPending: boolean;
  onMutate: (notificationId: string, action: MutationAction) => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const presentation = notificationPresentationChip(entry.presentation);
  const PresentationIcon = presentation.icon;
  const presentationIsKnown = CODED_KNOWN_PRESENTATIONS.has(entry.presentation);
  const stateClass = notificationStateClassChip(entry.state_class);
  const StateClassIcon = stateClass.icon;
  const stateClassIsKnown = CODED_KNOWN_STATE_CLASSES.has(entry.state_class);
  const categoryIsKnown = CODED_NOTIFICATION_CATEGORIES.includes(entry.category);
  const isJobObject = entry.related_object_type === CODED_RELATED_OBJECT_JOB;
  const objectState = entry.object_state ?? "";
  const hasObjectState = objectState !== "";
  const stateKnown = hasObjectState && CODED_JOB_STATES.includes(objectState);
  const newestAt = new Date(entry.created_at);
  const newestAtValid = !Number.isNaN(newestAt.getTime());

  return (
    <li className={styles.row}>
      <div className={styles.rowHead}>
        <span className={`${styles.chip} ${styles[presentation.tone]}`}>
          <PresentationIcon aria-hidden="true" size={14} />
          {t(presentation.labelKey)}
          {presentationIsKnown ? null : ` (${entry.presentation})`}
        </span>
        <span className={`${styles.chip} ${styles[stateClass.tone]}`}>
          <StateClassIcon aria-hidden="true" size={14} />
          {t(stateClass.labelKey)}
          {stateClassIsKnown ? null : ` (${entry.state_class})`}
        </span>
        <span className={styles.category}>
          {t(notificationCategoryLabelKey(entry.category))}
          {categoryIsKnown ? null : ` (${entry.category})`}
        </span>
        {newestAtValid ? (
          <time className={styles.rowTime} dateTime={entry.created_at}>
            {formatTimestamp(newestAt, locale)}
          </time>
        ) : null}
      </div>

      <div className={styles.objectRow}>
        <span className={styles.fieldLabel}>
          {isJobObject ? t("notifications.entry.objectJob") : t("notifications.entry.objectUnknownType")}
        </span>
        {isJobObject ? (
          <Link
            to={`/jobs/${encodeURIComponent(entry.related_object_id)}`}
            className={styles.objectLink}
            onClick={onNavigate}
          >
            <code className={styles.objectId}>{entry.related_object_id}</code>
          </Link>
        ) : (
          <code className={styles.objectId}>{entry.related_object_id}</code>
        )}
        {hasObjectState ? (
          <span className={`${styles.chip} ${styles[jobStateChip(objectState).tone]}`}>
            {(() => {
              const Icon = jobStateChip(objectState).icon;
              return <Icon aria-hidden="true" size={14} />;
            })()}
            {stateKnown ? t(jobStateChip(objectState).labelKey) : objectState}
          </span>
        ) : (
          // The stale binding: the Core supplies no canonical state for an
          // unresolvable object — rendered as Unavailable, never invented.
          <span className={styles.stateUnavailable}>{t("notifications.entry.stateUnavailable")}</span>
        )}
        {isJobObject ? null : (
          // An unknown future object type has no delivered destination
          // route: NO link is rendered (never a dead link), and the
          // truthful reason states it.
          <span className={styles.stateUnavailable}>{t("notifications.entry.noDestination")}</span>
        )}
      </div>

      {entry.repeat_count > 1 ? (
        <p className={styles.meta}>
          {t("notifications.entry.updatesCount", { count: entry.repeat_count })}
          {entry.unread_count > 0
            ? ` — ${t("notifications.entry.unreadInGroup", { count: entry.unread_count })}`
            : ""}
        </p>
      ) : null}

      {entry.collapsed.length > 0 ? (
        <ul className={styles.blocks}>
          {entry.collapsed.map((block, index) => {
            const blockClass = notificationStateClassChip(block.state_class);
            const BlockIcon = blockClass.icon;
            const blockNewest = new Date(block.newest_at);
            const blockNewestValid = !Number.isNaN(blockNewest.getTime());
            return (
              <li key={`${block.state_class}-${block.newest_at}-${index}`} className={styles.block}>
                <span className={`${styles.chip} ${styles[blockClass.tone]}`}>
                  <BlockIcon aria-hidden="true" size={14} />
                  {t(blockClass.labelKey)}
                  {CODED_KNOWN_STATE_CLASSES.has(block.state_class) ? null : ` (${block.state_class})`}
                </span>
                <span className={styles.blockCount}>{`× ${block.count}`}</span>
                {block.unread_count > 0 ? (
                  <span className={styles.blockUnread}>
                    {t("notifications.entry.blockUnread", { count: block.unread_count })}
                  </span>
                ) : null}
                {blockNewestValid ? (
                  <span className={styles.rowTime}>
                    {t("notifications.entry.blockNewest", { time: formatTimestamp(blockNewest, locale) })}
                  </span>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}

      <div className={styles.actionsRow}>
        <span className={styles.mutationTarget}>
          <span className={styles.fieldLabel}>{t("notifications.entry.copyId")}</span>
          <CopyIdButton
            id={entry.notification_id}
            label={entry.notification_id}
            copiedLabel={t("jobs.list.copied")}
            failedLabel={t("jobs.list.copyFailed")}
          />
        </span>
        {entry.presentation === "read" ? (
          <button
            type="button"
            className={styles.control}
            disabled={mutationPending}
            onClick={() => onMutate(entry.notification_id, "unread")}
          >
            {mutationPending ? t("notifications.actions.pending") : t("notifications.actions.markUnread")}
          </button>
        ) : (
          <button
            type="button"
            className={styles.control}
            disabled={mutationPending}
            onClick={() => onMutate(entry.notification_id, "read")}
          >
            {mutationPending ? t("notifications.actions.pending") : t("notifications.actions.markRead")}
          </button>
        )}
        <button
          type="button"
          className={styles.control}
          disabled={mutationPending}
          onClick={() => onMutate(entry.notification_id, "clear")}
        >
          {mutationPending ? t("notifications.actions.pending") : t("notifications.actions.clear")}
        </button>
      </div>
    </li>
  );
}
