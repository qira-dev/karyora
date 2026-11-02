import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link, NavLink, useNavigate, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type { WireConversation, WireConversationMessage } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./ChatPage.module.css";

/**
 * `/chat` and `/chat/:conversationId` — the conversation and Ask surface
 * (IMPL-11; the Chat surface per P3.5 over the delivered IMPL-10 Core
 * conversation surface).
 *
 * What this surface consumes (exactly as the Core delivered it, IMPL-10):
 * - `POST /v1/conversations` (create; the Core mints the identity, the
 *   body carries the optional bounded title), `GET /v1/conversations
 *   ?limit=&q=&cursor=` (the server-side list/search), `GET
 *   /v1/conversations/{id}` (the aggregate read), `GET
 *   /v1/conversations/{id}/messages?limit=&cursor=` (the authoritative
 *   timeline, seq ascending), `POST /v1/conversations/{id}/send` (the
 *   Ask send — the caller-presented correlation ID is the P1.6 §3.9 M-1
 *   request identity, NOT an idempotency key), and `GET
 *   /v1/conversations/{id}/exchange?correlation_id=` (the S4
 *   query-by-correlation-ID resolution).
 * - The laws this surface implements, exactly as the records bind them:
 *   X-1 (Ask never creates a durable Job, never opens the Run path; the
 *   [No durable Job created] marker rides every send-recorded message;
 *   the Run path is reached only by its own explicit entry); the
 *   no-auto-retry law (a lost send response holds the truthful S4 state
 *   and resolves ONLY by the delivered query-by-correlation-ID read —
 *   no retry loop exists anywhere in this file); UI-28's gate (decision
 *   8: edit/regenerate/archive/delete/export/rename have ZERO
 *   affordances and zero code paths here — for messages and
 *   conversations alike); the attachment gate (P-S5-06…11 unsupplied —
 *   zero attachment affordance); no streaming/stop/steering UI (none
 *   exists in the Core — P-S5-03's transport gap stands).
 * - The honest no-answer truth: no answering/production path exists in
 *   the Core (IMPL-10 §7). The timeline ends at the recorded exchange.
 *   There is NO fake streaming, NO typing/answer indicator, NO spinner
 *   promising an answer that cannot arrive, NO partial-rendering
 *   machinery, and NO completion confirmation — each absence is the
 *   truthful state, rendered as such.
 * - Every state below is driven by a real transport outcome (the IMPL-5
 *   pattern): loading, data, the typed D-35 refusal surfaced verbatim,
 *   or the truthful unknown. No fabricated rows, counts, titles, or
 *   optimistic states exist here. The stale-query race guard (P1.7
 *   §6.2) is the read-sequence guard: an older query's results never
 *   replace a newer query's. The no-history / no-search-matches /
 *   Unavailable distinction renders exactly (a failed search never
 *   renders a fake "no results").
 * - The draft is a local lifetime-3 state (the RunTaskPage precedent):
 *   this component's memory for the mounted conversation view's
 *   lifetime — never persisted, never carrying secrets.
 * - Machine identifiers (role and exchange-state register values, error
 *   codes) render verbatim as the language-neutral identifiers they are.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";
type TimelinePhase = "idle" | "loading" | "data" | "failure" | "unknown";
type SendPhase = "idle" | "sending" | "unknown" | "resolved-recorded" | "not-recorded" | "rejected";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

/** The send's request identity (P1.6 §3.9 M-1: correlation ID — the
 * caller carries it). Minted at submit time, never earlier; a submission
 * without an identity source is never sent. */
function newCorrelationId(): string {
  if (typeof crypto === "undefined" || typeof crypto.randomUUID !== "function") {
    throw new Error("correlation id source unavailable");
  }
  return crypto.randomUUID();
}

export function ChatPage() {
  const { conversationId } = useParams<{ conversationId: string }>();
  const t = useT();
  // The history pane's refresh signal: a send confirmed recorded bumps it
  // (the conversation's updated_at is the list's Core-defined sort key),
  // and the pane re-reads through its own markRefreshing read — the
  // prior rows stay visible and stale-labeled while it runs.
  const [historyRefreshSignal, setHistoryRefreshSignal] = useState(0);

  return (
    <PageLayout>
      <PageHeading>{t("chat.title")}</PageHeading>
      <p className={styles.description}>{t("chat.description")}</p>
      <div className={styles.surface}>
        <HistoryPane refreshSignal={historyRefreshSignal} />
        {conversationId !== undefined && conversationId !== "" ? (
          // Keyed by the conversation id: switching conversations remounts
          // the view — each conversation's composer starts empty (the draft
          // is memory-only for one mounted conversation view, never
          // persisted across conversations).
          <ConversationView
            key={conversationId}
            conversationId={conversationId}
            onRecorded={() => setHistoryRefreshSignal((signal) => signal + 1)}
          />
        ) : (
          <ConversationLanding />
        )}
      </div>
    </PageLayout>
  );
}

/** The history pane: the Core's server-side list + search (P-S5-04), the
 *  create affordance (P-S5-01), and selection-as-navigation. NO rename,
 *  archive, delete, or export affordance exists anywhere (UI-28 gated —
 *  decision 8; the Core supplies no such operation). */
function HistoryPane({ refreshSignal }: { refreshSignal: number }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const { transport } = useCoreConnection();
  const navigate = useNavigate();

  const [rows, setRows] = useState<readonly WireConversation[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [searchInput, setSearchInput] = useState("");
  const [activeSearch, setActiveSearch] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);

  const [createTitle, setCreateTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [createFailure, setCreateFailure] = useState<CoreWireEnvelope | null>(null);
  const [createUnknown, setCreateUnknown] = useState(false);

  // The stale-query race guard (P1.7 §6.2): only the LATEST issued read
  // applies its result — an older query's results never replace a newer
  // query's (the IMPL-5 read-sequence pattern).
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

      const params: { q?: string; cursor?: string } = {};
      if (activeSearch !== "") params.q = activeSearch;
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.conversationList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) => (options.append ? [...prev, ...outcome.data.conversations] : outcome.data.conversations));
          // A response without the cursor key IS the final page: null ends
          // pagination (the coded end marker — no has_more exists).
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
    [activeSearch, transport],
  );

  // The first read on mount and on every committed search change
  // (server-side filtering — the client never filters rows itself).
  useEffect(() => {
    setRows([]);
    setNextCursor(null);
    setFailure(null);
    setLastConfirmedAt(null);
    runRead({ append: false, markRefreshing: false });
  }, [runRead]);

  // A recorded send bumps the refresh signal: the pane re-reads its
  // authoritative first page (markRefreshing — prior rows stay visible
  // and stale-labeled while the read runs). Signal 0 is the initial
  // state, already covered by the mount read above; the handled-signal
  // ref keeps a runRead identity change (a search edit) from re-firing
  // this effect — only a genuine signal bump refreshes here.
  const handledSignalRef = useRef(0);
  useEffect(() => {
    if (refreshSignal === handledSignalRef.current) return;
    handledSignalRef.current = refreshSignal;
    runRead({ append: false, markRefreshing: true });
  }, [refreshSignal, runRead]);

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

  const submitSearch = () => {
    // Search narrowing is a committed query (Enter or the search action):
    // the seq guard above supersedes any in-flight read, and a FAILED
    // search renders Unavailable — never a fake "no results".
    setActiveSearch(searchInput.trim());
  };

  const clearSearch = () => {
    setSearchInput("");
    setActiveSearch("");
  };

  const createConversation = () => {
    if (creating) return;
    setCreating(true);
    setCreateFailure(null);
    setCreateUnknown(false);
    // Creation-time titles only (the delivered create semantics). The
    // Core mints the identity; NO rename affordance exists anywhere.
    const body = { title: createTitle.trim() };
    void transport.conversationCreate(body).then((outcome) => {
      if (!mountedRef.current) return;
      setCreating(false);
      if (outcome.kind === "data") {
        setCreateTitle("");
        runRead({ append: false, markRefreshing: true });
        navigate(`/chat/${encodeURIComponent(outcome.data.conversation.conversation_id)}`);
      } else if (outcome.kind === "rejection") {
        setCreateFailure(outcome.envelope);
      } else {
        // The truthful create-unknown state: no conversation identity was
        // issued to this page — nothing is known to have been created,
        // and the refresh action is how the truth (if any) appears.
        setCreateUnknown(true);
      }
    });
  };

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("chat.history.loading");
      case "failure":
        return t("chat.history.failureTitle");
      case "unknown":
        return t("chat.history.unknownTitle");
      default:
        return rows.length === 0
          ? activeSearch === ""
            ? t("chat.history.emptyAllTitle")
            : t("chat.history.emptySearchTitle")
          : t("chat.history.loaded");
    }
  })();

  return (
    <aside className={styles.history} aria-label={t("chat.history.heading")}>
      <h2 className={styles.paneHeading}>{t("chat.history.heading")}</h2>
      <div className={styles.createRow}>
        <label className="visually-hidden" htmlFor="chat-new-title">
          {t("chat.history.titleLabel")}
        </label>
        <input
          id="chat-new-title"
          className={styles.createTitle}
          type="text"
          value={createTitle}
          placeholder={t("chat.history.titlePlaceholder")}
          onChange={(event) => setCreateTitle(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.nativeEvent.isComposing) {
              event.preventDefault();
              createConversation();
            }
          }}
        />
        <button type="button" className={styles.control} onClick={createConversation} disabled={creating}>
          {creating ? t("chat.history.creating") : t("chat.history.newButton")}
        </button>
      </div>
      {createFailure !== null ? (
        <>
          <RecoveryPanel
            title={t("chat.history.createFailed")}
            body={t("chat.history.failureBody")}
            note={t("chat.history.connectionHint")}
            referenceId={createFailure.correlationId}
            actions={[{ label: t("chat.history.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{createFailure.code}</code>
          </p>
        </>
      ) : null}
      {createUnknown ? (
        <RecoveryPanel
          title={t("chat.history.createFailed")}
          body={t("chat.history.createUnavailable")}
          note={t("chat.history.connectionHint")}
          actions={[{ label: t("chat.history.refresh"), onAction: refresh }]}
        />
      ) : null}

      <form
        className={styles.searchRow}
        onSubmit={(event) => {
          event.preventDefault();
          submitSearch();
        }}
      >
        <label className="visually-hidden" htmlFor="chat-search">
          {t("chat.history.searchLabel")}
        </label>
        <input
          id="chat-search"
          className={styles.searchInput}
          type="search"
          value={searchInput}
          placeholder={t("chat.history.searchPlaceholder")}
          onChange={(event) => setSearchInput(event.target.value)}
        />
        <button type="submit" className={styles.control}>
          {t("chat.history.searchLabel")}
        </button>
        {activeSearch !== "" ? (
          <button type="button" className={styles.control} onClick={clearSearch}>
            {t("chat.history.clearSearch")}
          </button>
        ) : null}
      </form>

      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("chat.history.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("chat.history.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("chat.history.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          activeSearch === "" ? (
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("chat.history.emptyAllTitle")}</p>
              <p>{t("chat.history.emptyAllBody")}</p>
            </div>
          ) : (
            // A successful server-backed search with zero rows — the
            // honest no-match state, with its filter-clear affordance.
            <div className={styles.empty}>
              <p className={styles.emptyTitle}>{t("chat.history.emptySearchTitle")}</p>
              <p>{t("chat.history.emptySearchBody")}</p>
              <button type="button" className={styles.control} onClick={clearSearch}>
                {t("chat.history.clearSearch")}
              </button>
            </div>
          )
        ) : (
          <>
            <ul className={styles.rows}>
              {rows.map((conversation) => (
                <li key={conversation.conversation_id}>
                  <NavLink
                    to={`/chat/${encodeURIComponent(conversation.conversation_id)}`}
                    className={({ isActive }) =>
                      isActive ? `${styles.rowLink} ${styles.rowLinkActive}` : styles.rowLink
                    }
                  >
                    <span className={styles.rowTitle}>
                      {conversation.title !== ""
                        ? conversation.title
                        : t("chat.history.untitledFallback", { conversationId: conversation.conversation_id })}
                    </span>
                    <span className={styles.rowTime}>
                      <time dateTime={conversation.updated_at}>
                        {formatTimestamp(new Date(conversation.updated_at), locale)}
                      </time>
                    </span>
                    <span className="visually-hidden">
                      {t("chat.history.openConversation", { conversationId: conversation.conversation_id })}
                    </span>
                  </NavLink>
                </li>
              ))}
            </ul>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button type="button" className={styles.control} onClick={loadMore} disabled={loadingMore}>
                  {loadingMore ? t("chat.history.loadingMore") : t("chat.history.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("chat.history.failureTitle")}
            body={t("chat.history.failureBody")}
            note={t("chat.history.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("chat.history.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("chat.history.unknownTitle")}
          body={t("chat.history.unknownBody")}
          note={t("chat.history.connectionHint")}
          actions={[{ label: t("chat.history.refresh"), onAction: refresh }]}
        />
      ) : null}
    </aside>
  );
}

/** The no-conversation-selected landing: the truthful empty state — the
 *  surface's Ask-only disclosure and the create/select affordances. No
 *  composer renders here: the delivered send wire addresses ONE
 *  conversation, and no conversation is selected — a landing composer
 *  would be a fake affordance (no silent conversation creation is
 *  invented either). */
function ConversationLanding() {
  const t = useT();
  return (
    <section className={styles.conversation} aria-label={t("chat.timeline.focusHeading")}>
      <div className={styles.empty}>
        <p className={styles.emptyTitle}>{t("chat.history.emptyAllTitle")}</p>
        <p>{t("chat.history.emptyAllBody")}</p>
        <p>{t("chat.timeline.noAnswerNote")}</p>
      </div>
      <ModeDisclosure />
    </section>
  );
}

/** The X-1 mode disclosure (visible on every render of this surface,
 *  before send): the surface IS the Ask mode — it never switches, never
 *  creates a durable Job, and the Run path is reached only by its own
 *  explicit entry. The provider/model and egress lines render the
 *  truthful Not-reported values — the delivered send wire carries no
 *  provider or model selection, so nothing reassuring is invented. */
function ModeDisclosure() {
  const t = useT();
  return (
    <div className={styles.disclosure}>
      <p className={styles.disclosureLine}>
        <strong>{t("chat.modeLabel")}:</strong> {t("chat.modeAsk")} — {t("chat.modeAskNote")}
      </p>
      <p className={styles.disclosureLine}>{t("chat.composer.providerLine")}</p>
      <p className={styles.disclosureLine}>{t("chat.composer.egressLine")}</p>
      <p className={styles.disclosureLine}>
        {t("chat.runPointer")}{" "}
        <Link to="/jobs/new" className={styles.runLink}>
          {t("chat.runPointerLink")}
        </Link>
      </p>
      <p className={styles.disclosureLine}>{t("chat.draftNote")}</p>
    </div>
  );
}

/** One selected conversation: the authoritative aggregate read, the
 *  authoritative timeline (seq ascending, cursor-paginated forward), and
 *  the composer + send machine (the S0–S4 discipline over the
 *  conversation plane). */
function ConversationView({
  conversationId,
  onRecorded,
}: {
  conversationId: string;
  onRecorded: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const { transport } = useCoreConnection();

  // The aggregate read (the heading's authoritative source).
  const [aggregate, setAggregate] = useState<WireConversation | null>(null);
  const [aggregatePhase, setAggregatePhase] = useState<TimelinePhase>("loading");
  const [aggregateFailure, setAggregateFailure] = useState<CoreWireEnvelope | null>(null);

  // The timeline read (the authoritative messages, seq ascending).
  const [messages, setMessages] = useState<readonly WireConversationMessage[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [timelinePhase, setTimelinePhase] = useState<TimelinePhase>("loading");
  const [timelineFailure, setTimelineFailure] = useState<CoreWireEnvelope | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);

  // The composer draft: a local lifetime-3 state — this component's
  // memory for the mounted view's lifetime, never persisted, never
  // carrying a secret. The view is keyed by conversation id, so a
  // conversation switch starts a fresh composer.
  const [draft, setDraft] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  // The send machine (the conversation plane's S0–S4): the correlation ID
  // is the request identity (P1.6 §3.9 M-1) — minted at submit time and
  // held while the outcome is unresolved, because the S4 resolution is
  // the query-by-correlation-ID read. NO idempotency key exists (the
  // class stays Open) and NO auto-retry exists anywhere: a lost response
  // holds S4 until the explicit status check resolves it.
  const [sendPhase, setSendPhase] = useState<SendPhase>("idle");
  const [pendingCorrelationId, setPendingCorrelationId] = useState<string | null>(null);
  const [sendFailure, setSendFailure] = useState<CoreWireEnvelope | null>(null);
  const [statusCheckFailure, setStatusCheckFailure] = useState<CoreWireEnvelope | null>(null);
  const [statusCheckLost, setStatusCheckLost] = useState(false);
  const [correlationUnavailable, setCorrelationUnavailable] = useState(false);
  const [resolvedExchange, setResolvedExchange] = useState<{ correlationId: string; state: string } | null>(null);
  const [announce, setAnnounce] = useState("");

  const readSeqRef = useRef(0);
  const aggregateSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const composerRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // After selection, focus moves to the conversation heading (the P3.5
  // §3 row 2 focus rule — selection is navigation, never a mutation).
  useEffect(() => {
    headingRef.current?.focus();
  }, [conversationId]);

  const runTimelineRead = useCallback(
    (options: { readonly append: boolean; readonly cursor?: string; readonly markRefreshing: boolean }) => {
      const seq = ++readSeqRef.current;
      if (options.markRefreshing) setRefreshing(true);
      if (!options.append && !options.markRefreshing) setTimelinePhase("loading");

      const params: { cursor?: string } = {};
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.conversationMessages(conversationId, params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setMessages((prev) => (options.append ? [...prev, ...outcome.data.messages] : outcome.data.messages));
          setNextCursor(outcome.data.cursor ?? null);
          setLastConfirmedAt(new Date());
          setTimelinePhase("data");
          setAnnounce(t("chat.announce.messagesLoaded"));
        } else if (outcome.kind === "rejection") {
          setTimelineFailure(outcome.envelope);
          setTimelinePhase("failure");
        } else {
          setTimelinePhase("unknown");
        }
      });
    },
    [conversationId, transport, t],
  );

  const readAggregate = useCallback(() => {
    const seq = ++aggregateSeqRef.current;
    setAggregatePhase("loading");
    setAggregateFailure(null);
    void transport.conversationGet(conversationId).then((outcome) => {
      if (!mountedRef.current || seq !== aggregateSeqRef.current) return; // superseded
      if (outcome.kind === "data") {
        setAggregate(outcome.data.conversation);
        setAggregatePhase("data");
      } else if (outcome.kind === "rejection") {
        setAggregateFailure(outcome.envelope);
        setAggregatePhase("failure");
      } else {
        setAggregatePhase("unknown");
      }
    });
  }, [conversationId, transport]);

  // The first reads on mount (per conversation — the view is keyed).
  useEffect(() => {
    setMessages([]);
    setNextCursor(null);
    setTimelineFailure(null);
    setLastConfirmedAt(null);
    setSendPhase("idle");
    setPendingCorrelationId(null);
    setSendFailure(null);
    setResolvedExchange(null);
    runTimelineRead({ append: false, markRefreshing: false });
    readAggregate();
  }, [runTimelineRead, readAggregate]);

  const loadMore = () => {
    if (nextCursor === null || loadingMore || refreshing) return;
    setLoadingMore(true);
    runTimelineRead({ append: true, cursor: nextCursor, markRefreshing: false });
  };

  const refreshTimeline = () => {
    if (refreshing || loadingMore) return;
    setTimelineFailure(null);
    // A refresh keeps the confirmed rows rendered (stale-labeled by the
    // last-confirmed line) and re-reads the FIRST page — the Core's
    // delivered read is seq-ascending, so the first page is the
    // timeline's head; a later cursor page is re-walked by explicit user
    // action only (the IMPL-5 refresh pattern, consumed exactly).
    runTimelineRead({ append: false, markRefreshing: true });
  };

  /** A send that the Core confirms recorded (directly or through the S4
   *  query) refreshes BOTH authoritative reads: the timeline (the new
   *  message's durable home, markRefreshing — prior rows stay visible)
   *  and the history list through the parent's refresh signal (the
   *  conversation's updated_at is the list's Core-defined sort key). */
  const refreshAfterRecorded = useCallback(() => {
    refreshTimeline();
    onRecorded();
  }, [refreshTimeline, onRecorded]);

  const send = () => {
    const content = draft.trim();
    if (content === "") {
      setLocalError(t("chat.composer.emptyError"));
      return;
    }
    if (sendPhase === "sending" || sendPhase === "unknown") return; // duplicate submit disabled while pending

    let correlationId: string;
    try {
      correlationId = newCorrelationId();
    } catch {
      // Nothing was sent: no request identity could be minted, so no S4
      // exists — the truthful note renders and the draft is preserved.
      setCorrelationUnavailable(true);
      return;
    }
    setCorrelationUnavailable(false);
    setLocalError(null);
    setSendFailure(null);
    setStatusCheckFailure(null);
    setStatusCheckLost(false);
    setSendPhase("sending");
    setPendingCorrelationId(correlationId);
    setAnnounce(t("chat.announce.sending"));
    void transport
      .conversationSend(conversationId, { content, correlation_id: correlationId })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // S2-equivalent on the conversation plane: the Core recorded the
          // exchange. The response carries NO answer — none exists to
          // carry. The authoritative reads render the recorded state.
          setResolvedExchange({
            correlationId: outcome.data.exchange.correlation_id,
            state: outcome.data.exchange.state,
          });
          setSendPhase("resolved-recorded");
          setPendingCorrelationId(null);
          setDraft("");
          setAnnounce(t("chat.announce.recorded"));
          refreshAfterRecorded();
        } else if (outcome.kind === "rejection") {
          // A typed refusal is a RESOLVED outcome (S3): the Core answered.
          // The draft is preserved — fix the message and send again.
          setSendFailure(outcome.envelope);
          setSendPhase("rejected");
          setPendingCorrelationId(null);
          setAnnounce(t("chat.announce.rejected"));
        } else {
          // S4: the response was lost or uninterpretable. The correlation
          // ID stays held; resolution is ONLY the query-by-correlation-ID
          // read — never a duplicate send (no idempotency key exists).
          setSendPhase("unknown");
          setAnnounce(t("chat.announce.unknown"));
        }
      });
  };

  /** The S4 resolution: the delivered query-by-correlation-ID read. It is
   *  a pure query — it never re-sends. */
  const checkStatus = () => {
    if (pendingCorrelationId === null || sendPhase !== "unknown") return;
    setStatusCheckFailure(null);
    setStatusCheckLost(false);
    void transport.conversationExchangeStatus(conversationId, pendingCorrelationId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        // The Core records the exchange: the send WAS recorded. The
        // timeline re-read renders the authoritative state.
        setResolvedExchange({
          correlationId: outcome.data.exchange.correlation_id,
          state: outcome.data.exchange.state,
        });
        setSendPhase("resolved-recorded");
        setPendingCorrelationId(null);
        setDraft("");
        setAnnounce(t("chat.announce.resolvedRecorded"));
        refreshAfterRecorded();
      } else if (outcome.kind === "rejection") {
        if (outcome.envelope.code === "NOT_FOUND") {
          // The truthful resolution: nothing was recorded for this request
          // identity (the Core's no-leak rule makes unknown and foreign
          // indistinguishable — for a freshly minted identity this is the
          // not-recorded truth). The draft is preserved; sending again is
          // a NEW request with a NEW identity — an explicit user act.
          setSendPhase("not-recorded");
          setPendingCorrelationId(null);
          setAnnounce(t("chat.announce.resolvedNotRecorded"));
        } else {
          // The status QUERY itself was refused: the send's outcome is
          // still unresolved — S4 holds, with the refusal surfaced.
          setStatusCheckFailure(outcome.envelope);
        }
      } else {
        // The status check itself was lost: S4 holds.
        setStatusCheckLost(true);
      }
    });
  };

  const newRequestAfterNotRecorded = () => {
    setSendPhase("idle");
    setSendFailure(null);
    setStatusCheckFailure(null);
    setStatusCheckLost(false);
    composerRef.current?.focus();
  };

  const composerDisabled = sendPhase === "sending" || sendPhase === "unknown";
  const sendDisabled = draft.trim() === "" || composerDisabled;

  const onComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    // Enter sends only when IME composition is accounted for (P3.5 §5):
    // a composition in progress commits per platform behavior and never
    // sends. Shift+Enter inserts a newline (the textarea default).
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      send();
    }
  };

  const title = aggregate !== null && aggregate.title !== "" ? aggregate.title : null;

  return (
    <section className={styles.conversation} aria-label={t("chat.timeline.focusHeading")}>
      <h2 ref={headingRef} tabIndex={-1} className={styles.conversationHeading}>
        {title !== null ? title : aggregate !== null ? (
          <>
            {t("chat.timeline.untitledHeading")}{" "}
            <code className={styles.identityCode}>{aggregate.conversation_id}</code>
          </>
        ) : (
          t("chat.timeline.focusHeading")
        )}
      </h2>

      {aggregatePhase === "loading" ? <p className={styles.note}>{t("chat.timeline.loading")}</p> : null}
      {aggregatePhase === "failure" && aggregateFailure !== null ? (
        <>
          <RecoveryPanel
            title={t("chat.timeline.notFoundTitle")}
            body={t("chat.timeline.notFoundBody")}
            referenceId={aggregateFailure.correlationId}
            actions={[{ label: t("chat.timeline.refresh"), onAction: readAggregate }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{aggregateFailure.code}</code>
          </p>
          <p className={styles.linkRow}>
            <Link to="/chat">{t("chat.timeline.backToList")}</Link>
          </p>
        </>
      ) : null}
      {aggregatePhase === "unknown" ? (
        <RecoveryPanel
          title={t("chat.timeline.unknownTitle")}
          body={t("chat.timeline.unknownBody")}
          note={t("chat.timeline.connectionHint")}
          actions={[{ label: t("chat.timeline.refresh"), onAction: readAggregate }]}
        />
      ) : null}

      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("chat.timeline.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("chat.timeline.refreshing")}` : ""}
        </p>
      ) : null}

      {timelinePhase === "loading" ? <p className={styles.note}>{t("chat.timeline.loading")}</p> : null}

      {timelinePhase === "data" ? (
        messages.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>{t("chat.timeline.emptyTitle")}</p>
            <p>{t("chat.timeline.emptyBody")}</p>
            <p>{t("chat.timeline.noAnswerNote")}</p>
          </div>
        ) : (
          <>
            <ol className={styles.messages}>
              {messages.map((message) => (
                <MessageRow key={message.message_id} message={message} />
              ))}
            </ol>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button type="button" className={styles.control} onClick={loadMore} disabled={loadingMore}>
                  {loadingMore ? t("chat.timeline.loadingMore") : t("chat.timeline.loadMore")}
                </button>
              </p>
            ) : null}
            <p className={styles.noAnswerNote}>{t("chat.timeline.noAnswerNote")}</p>
          </>
        )
      ) : null}

      {timelinePhase === "failure" && timelineFailure !== null ? (
        <>
          <RecoveryPanel
            title={t("chat.timeline.failureTitle")}
            body={t("chat.timeline.failureBody")}
            note={t("chat.timeline.connectionHint")}
            referenceId={timelineFailure.correlationId}
            actions={[{ label: t("chat.timeline.refresh"), onAction: refreshTimeline }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{timelineFailure.code}</code>
          </p>
        </>
      ) : null}

      {timelinePhase === "unknown" ? (
        <RecoveryPanel
          title={t("chat.timeline.unknownTitle")}
          body={t("chat.timeline.unknownBody")}
          note={t("chat.timeline.connectionHint")}
          actions={[{ label: t("chat.timeline.refresh"), onAction: refreshTimeline }]}
        />
      ) : null}

      <ModeDisclosure />

      <div className={styles.composer}>
        <label className={styles.composerLabel} htmlFor="chat-composer">
          {t("chat.composer.label")}
        </label>
        <textarea
          id="chat-composer"
          ref={composerRef}
          className={styles.composerInput}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            if (localError !== null) setLocalError(null);
            if (sendPhase === "rejected") {
              setSendPhase("idle");
              setSendFailure(null);
            }
          }}
          onKeyDown={onComposerKeyDown}
          disabled={composerDisabled}
          aria-busy={sendPhase === "sending"}
          rows={4}
        />
        <div className={styles.composerRow}>
          <button type="button" className={styles.primary} onClick={send} disabled={sendDisabled}>
            {sendPhase === "sending" ? t("chat.composer.sending") : t("chat.composer.send")}
          </button>
          <span className={styles.enterHint}>{t("chat.composer.enterHint")}</span>
        </div>
        {sendPhase === "sending" || sendPhase === "unknown" ? (
          <p className={styles.pending} role="status">
            {sendPhase === "sending" ? t("chat.composer.pendingNote") : t("chat.composer.pendingUnknownNote")}
          </p>
        ) : null}
        {localError !== null ? <p className={styles.help}>{localError}</p> : null}
        {correlationUnavailable ? <p className={styles.help}>{t("chat.send.unknownBodyNoId")}</p> : null}
      </div>

      {sendPhase === "resolved-recorded" && resolvedExchange !== null ? (
        <div className={styles.sendOutcome}>
          <p>{t("chat.send.resolvedRecorded")}</p>
          <p className={styles.sendFacts}>
            <span>
              {t("chat.send.correlationLabel")}: <code>{resolvedExchange.correlationId}</code>
            </span>
            <span>
              {resolvedExchange.state === "recorded"
                ? `${t("chat.timeline.recordedState")}`
                : t("chat.timeline.exchangeStateUnknown", { state: resolvedExchange.state })}
            </span>
          </p>
        </div>
      ) : null}

      {sendPhase === "rejected" && sendFailure !== null ? (
        <>
          <RecoveryPanel
            title={t("chat.send.rejectedTitle")}
            body={t("chat.send.rejectedBody")}
            referenceId={sendFailure.correlationId}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{sendFailure.code}</code>
          </p>
        </>
      ) : null}

      {sendPhase === "unknown" ? (
        <>
          <RecoveryPanel
            title={t("chat.send.unknownTitle")}
            body={t("chat.send.unknownBody")}
            actions={[{ label: t("chat.send.checkStatus"), onAction: checkStatus }]}
          />
          {statusCheckFailure !== null ? (
            <p className={styles.help}>
              {t("chat.send.statusCheckFailed", { code: statusCheckFailure.code })}
            </p>
          ) : null}
          {statusCheckLost ? <p className={styles.help}>{t("chat.send.statusCheckLost")}</p> : null}
          {pendingCorrelationId !== null ? (
            <p className={styles.code}>
              <span className={styles.keyLabel}>{t("chat.send.correlationLabel")}: </span>
              <code>{pendingCorrelationId}</code>
            </p>
          ) : null}
        </>
      ) : null}

      {sendPhase === "not-recorded" ? (
        <RecoveryPanel
          title={t("chat.send.notRecordedTitle")}
          body={t("chat.send.notRecordedBody")}
          actions={[{ label: t("chat.send.newRequest"), onAction: newRequestAfterNotRecorded }]}
        />
      ) : null}

      <p role="status" className="visually-hidden">
        {announce}
      </p>
    </section>
  );
}

/** One authoritative message row: the Core's stored state, rendered
 *  exactly. The closed role register carries `user` today; an unknown
 *  future value renders verbatim as the neutral unknown it is. Content
 *  renders as text — message content is data, never instruction (the
 *  untrusted-content rule). Every send-linked message carries the
 *  [No durable Job created] marker (the P3.5 §3 row 12 UI duty over the
 *  delivered read): the send created no durable Job and never will on
 *  this path. */
function MessageRow({ message }: { message: WireConversationMessage }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const roleLabel = message.role === "user" ? t("chat.timeline.roleYou") : null;

  return (
    <li className={styles.message}>
      <div className={styles.messageMeta}>
        {roleLabel !== null ? (
          <span className={styles.roleChip}>{roleLabel}</span>
        ) : (
          <span className={styles.roleChipUnknown}>
            <code>{message.role}</code>
            <span className="visually-hidden">{t("chat.timeline.roleUnknownNote")}</span>
          </span>
        )}
        <time dateTime={message.created_at} className={styles.messageTime}>
          {formatTimestamp(new Date(message.created_at), locale)}
        </time>
      </div>
      <p className={styles.messageContent}>{message.content}</p>
      {message.correlation_id !== undefined ? (
        <p className={styles.noJobMarker}>{t("chat.timeline.noJobMarker")}</p>
      ) : null}
    </li>
  );
}
