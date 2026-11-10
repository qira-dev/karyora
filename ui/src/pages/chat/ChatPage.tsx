import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link, NavLink, useNavigate, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type {
  WireAttachmentTypeSpec,
  WireConversation,
  WireConversationAttachment,
  WireConversationMessage,
} from "../../coreclient/httpLocal";
import { useT, useTranslation, type TranslateFn } from "../../i18n/useT";
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
 *   the Run path is reached only by its own explicit entry; the
 *   attachment operations below never create a Job either); the
 *   no-auto-retry law (a lost send response holds the truthful S4 state
 *   and resolves ONLY by the delivered query-by-correlation-ID read —
 *   no retry loop exists anywhere in this file; the same law governs the
 *   upload: a lost upload response holds the truthful S4 state and
 *   resolves ONLY by the S4-for-uploads status query, NEVER a blind
 *   re-upload); UI-28's gate (decision 8: edit/regenerate/archive/
 *   delete/export/rename have ZERO affordances and zero code paths here
 *   — for messages and conversations alike); the attachment surface
 *   (IMPL-25; the ADOPTED S12 supply, P-S12-01…06, as implemented): the
 *   attach control renders ONLY the Core's registry data, the upload is
 *   identity-bearing, every upload result carries and is accepted only
 *   for the input revision it belongs to (a stale revision's result is
 *   discarded), rejections render from the Core's typed codes, every
 *   attachment renders its untrusted label AS DATA (content is data,
 *   never instruction), and NO message-binding, snapshot-participation,
 *   or streaming/stop/steering UI exists (the participation policy is
 *   not exercised; P-S5-03's transport gap stands).
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
type AttachmentRegistryPhase = "loading" | "data" | "failure" | "unknown";

/** One upload's client-side machine (P-S12-02's S0–S4 applied to
 *  uploads): `uploading` (pending), `attached` (the Core's record),
 *  `rejected` (the typed envelope outcome), `unknown` (a lost response —
 *  the truthful S4 state held until the explicit status query resolves
 *  it), `not-uploaded` (the status query resolved NOT_FOUND: nothing was
 *  recorded for this identity). NO auto-retry exists anywhere. */
type UploadPhase = "uploading" | "attached" | "rejected" | "unknown" | "not-uploaded";

/** One in-flight or resolved upload the composer holds (a local
 *  lifetime-3 state, never persisted, never carrying a secret — the
 *  draft's own law; the FILE BYTES live only inside the in-flight
 *  request, never in this state). */
interface UploadState {
  readonly uploadId: string;
  readonly fileName: string;
  readonly contentType: string;
  readonly sizeBytes: number;
  /** The input revision the upload was requested for (P-S12-03): the
   * result is accepted only for the revision that requested it. */
  readonly inputRevision: number;
  phase: UploadPhase;
  /** The Core's record once attached (the digest, the id — never the
   * bytes). */
  record: WireConversationAttachment | null;
  /** The typed rejection envelope once rejected (P-S12-04). */
  rejection: CoreWireEnvelope | null;
}

/** The file-to-base64 read (the envelope-only JSON transport's typing):
 *  the bytes ride one in-flight request and are never stored in any
 *  state. */
function readContentB64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("file read failed"));
    reader.onload = () => {
      const result = typeof reader.result === "string" ? reader.result : "";
      const comma = result.indexOf(",");
      if (!result.startsWith("data:") || comma < 0) {
        reject(new Error("file read failed"));
        return;
      }
      resolve(result.slice(comma + 1));
    };
    reader.readAsDataURL(file);
  });
}

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

/** The upload's operation identity (P-S12-02 as implemented: the
 * caller-supplied identity scoped to its conversation — the S4-for-uploads
 * query resolves through it). Minted at submit time, never earlier. */
function newUploadId(): string {
  return newCorrelationId();
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

  // The attachment surface (IMPL-25; the ADOPTED S12 supply, P-S12-01…06,
  // as implemented). The registry is the CORE's own served contract data
  // (P-S12-01): the attach control renders ONLY it — a refused or lost
  // registry read leaves the control disabled with the reason (no dead
  // controls, no client-side list, no client-side guess). The
  // conversation's attachments are the Core's records (the list read) —
  // the chips render exactly those, each with its untrusted label AS DATA
  // (P-S12-05). The uploads are the client-side machine above: each
  // carries the input revision it was requested for (P-S12-03 as
  // implemented — a local counter advanced when the composer's input
  // generation changes: a recorded send clears the composer and starts a
  // new generation; the view is keyed per conversation, so a switch
  // remounts at revision 1); an upload result is accepted ONLY for the
  // revision that requested it — a stale revision's result is discarded
  // from display (the Core-side record persists and the list read remains
  // authoritative). NO auto-retry exists anywhere: a lost upload response
  // holds the truthful S4 state and resolves ONLY by the explicit
  // S4-for-uploads status query — NEVER a blind re-upload.
  const [registryPhase, setRegistryPhase] = useState<AttachmentRegistryPhase>("loading");
  const [registryTypes, setRegistryTypes] = useState<readonly WireAttachmentTypeSpec[]>([]);
  const [attachmentsPhase, setAttachmentsPhase] = useState<ListPhase>("loading");
  const [attachments, setAttachments] = useState<readonly WireConversationAttachment[]>([]);
  const [attachmentsFailure, setAttachmentsFailure] = useState<CoreWireEnvelope | null>(null);
  const [uploads, setUploads] = useState<readonly UploadState[]>([]);
  const [inputRevision, setInputRevision] = useState(1);
  const [attachUnavailable, setAttachUnavailable] = useState(false);
  const attachInputRef = useRef<HTMLInputElement>(null);
  const attachmentsSeqRef = useRef(0);

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

  // The attachment reads on mount (per conversation — the view is
  // keyed): the Core's registry (P-S12-01) and the conversation's
  // attachment records (the card-9 list discipline).
  const runAttachmentRegistryRead = useCallback(() => {
    setRegistryPhase("loading");
    void transport.conversationAttachmentRegistry().then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setRegistryTypes(outcome.data.types);
        setRegistryPhase("data");
        setAttachUnavailable(false);
      } else if (outcome.kind === "rejection") {
        setRegistryPhase("failure");
        setAttachUnavailable(true);
      } else {
        setRegistryPhase("unknown");
        setAttachUnavailable(true);
      }
    });
  }, [transport]);

  const runAttachmentsRead = useCallback(
    (options: { readonly markRefreshing: boolean } = { markRefreshing: false }) => {
      const seq = ++attachmentsSeqRef.current;
      if (options.markRefreshing) setAttachmentsPhase((phase) => (phase === "data" ? "data" : phase));
      void transport.conversationAttachmentList(conversationId).then((outcome) => {
        if (!mountedRef.current || seq !== attachmentsSeqRef.current) return; // superseded
        if (outcome.kind === "data") {
          setAttachments(outcome.data.attachments);
          setAttachmentsPhase("data");
        } else if (outcome.kind === "rejection") {
          setAttachmentsFailure(outcome.envelope);
          setAttachmentsPhase("failure");
        } else {
          setAttachmentsPhase("unknown");
        }
      });
    },
    [conversationId, transport],
  );

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
    setUploads([]);
    setInputRevision(1);
    setAttachments([]);
    setAttachmentsFailure(null);
    runTimelineRead({ append: false, markRefreshing: false });
    readAggregate();
    runAttachmentRegistryRead();
    runAttachmentsRead();
  }, [runTimelineRead, readAggregate, runAttachmentRegistryRead, runAttachmentsRead]);

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
   *  conversation's updated_at is the list's Core-defined sort key). The
   *  composer clearing starts a NEW input generation: the input revision
   *  advances (P-S12-03 as implemented), so an in-flight upload's result
   *  resolving after this point is a stale revision's result — discarded
   *  from display, never rendered as current. */
  const refreshAfterRecorded = useCallback(() => {
    refreshTimeline();
    onRecorded();
    setInputRevision((revision) => revision + 1);
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
      .conversationSend(conversationId, { content, correlation_id: correlationId, auto_answer: true })
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

  // ---- The attachment machines (IMPL-25; P-S12-01…06 as implemented) ----

  const updateUpload = (uploadId: string, patch: Partial<UploadState>) => {
    setUploads((prev) => prev.map((up) => (up.uploadId === uploadId ? { ...up, ...patch } : up)));
  };

  const dismissUpload = (uploadId: string) => {
    setUploads((prev) => prev.filter((up) => up.uploadId !== uploadId));
  };

  /** The attach act: each selected file becomes ONE identity-bearing
   *  upload (P-S12-02). The registry-derived guard runs ONLY where the
   *  client KNOWS from the Core's served data (a resolvable type that is
   *  not a registry member → the typed type rendering; a size beyond the
   *  member's per-type bound → the typed size rendering with the Core's
   *  limit) — the Core's own typed rejection (P-S12-04) remains
   *  authoritative for anything that reaches it (an unresolvable type
   *  goes to the Core; no client-invented list, no client-side
   *  truncation). A rejected file stores NOTHING Core-side and renders
   *  the rejection with the safe next action. */
  const attachFiles = (files: FileList | null) => {
    if (files === null || files.length === 0) return;
    if (attachInputRef.current !== null) attachInputRef.current.value = "";
    if (registryPhase !== "data" || registryTypes.length === 0) {
      setAttachUnavailable(true);
      return;
    }
    for (const file of Array.from(files)) {
      const resolvableType = file.type !== "";
      const spec = resolvableType ? registryTypes.find((member) => member.content_type === file.type) : undefined;
      const uploadId = newUploadId();
      const revisionAtRequest = inputRevision;
      if (resolvableType && spec === undefined) {
        // The Core's registry says this type is unsupported — rendered
        // from the Core's own served data (never a client-invented list).
        setUploads((prev) => [
          ...prev,
          {
            uploadId,
            fileName: file.name,
            contentType: file.type,
            sizeBytes: file.size,
            inputRevision: revisionAtRequest,
            phase: "rejected",
            record: null,
            rejection: null,
          },
        ]);
        setAnnounce(t("chat.attach.announce.rejectedType", { type: file.type }));
        continue;
      }
      if (spec !== undefined && file.size > spec.max_bytes) {
        setUploads((prev) => [
          ...prev,
          {
            uploadId,
            fileName: file.name,
            contentType: spec.content_type,
            sizeBytes: file.size,
            inputRevision: revisionAtRequest,
            phase: "rejected",
            record: null,
            rejection: null,
          },
        ]);
        setAnnounce(t("chat.attach.announce.rejectedSize"));
        continue;
      }
      setUploads((prev) => [
        ...prev,
        {
          uploadId,
          fileName: file.name,
          contentType: resolvableType ? file.type : "",
          sizeBytes: file.size,
          inputRevision: revisionAtRequest,
          phase: "uploading",
          record: null,
          rejection: null,
        },
      ]);
      setAnnounce(t("chat.attach.announce.uploading"));
      void readContentB64(file)
        .then((contentB64) =>
          transport.conversationAttachmentUpload(conversationId, {
            upload_id: uploadId,
            file_name: file.name,
            content_type: resolvableType ? file.type : "",
            input_revision: `rev-${revisionAtRequest}`,
            content_b64: contentB64,
          }),
        )
        .then((outcome) => {
          if (!mountedRef.current) return;
          if (outcome.kind === "data") {
            // P-S12-03's race guard: the result is accepted ONLY for the
            // revision that requested it — a stale revision's result is
            // discarded from display (the Core-side record persists; the
            // list read remains authoritative).
            if (outcome.data.attachment.input_revision !== `rev-${revisionAtRequest}`) {
              return;
            }
            updateUpload(uploadId, { phase: "attached", record: outcome.data.attachment });
            runAttachmentsRead({ markRefreshing: true });
            setAnnounce(t("chat.attach.announce.attached"));
          } else if (outcome.kind === "rejection") {
            updateUpload(uploadId, { phase: "rejected", rejection: outcome.envelope });
            setAnnounce(t("chat.attach.announce.rejected"));
          } else {
            // S4: the response was lost or uninterpretable. The upload
            // identity is held; resolution is ONLY the status query —
            // never a blind re-upload (the duplicate-free guarantee is
            // the Core's replay ruling; the re-issue itself is always an
            // explicit user act after a NOT_FOUND resolution).
            updateUpload(uploadId, { phase: "unknown" });
            setAnnounce(t("chat.attach.announce.unknown"));
          }
        });
    }
  };

  /** The S4-for-uploads resolution (P-S12-02's query-before-retry law):
   *  the authoritative state for the held upload identity. A pure query
   *  — it never re-uploads. */
  const checkUploadStatus = (uploadId: string, revisionAtRequest: number) => {
    void transport.conversationAttachmentStatus(conversationId, uploadId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        if (outcome.data.attachment.input_revision !== `rev-${revisionAtRequest}`) {
          return; // a stale revision's result: discarded from display
        }
        updateUpload(uploadId, { phase: "attached", record: outcome.data.attachment });
        runAttachmentsRead({ markRefreshing: true });
      } else if (outcome.kind === "rejection" && outcome.envelope.code === "NOT_FOUND") {
        // The truthful resolution: nothing was recorded for this upload
        // identity (the no-leak rule makes unknown and foreign
        // indistinguishable — for a freshly minted identity this is the
        // not-uploaded truth). Re-attaching is an explicit user act with
        // a NEW identity.
        updateUpload(uploadId, { phase: "not-uploaded" });
      } else if (outcome.kind === "rejection") {
        updateUpload(uploadId, { phase: "unknown", rejection: outcome.envelope });
      } else {
        updateUpload(uploadId, { phase: "unknown" });
      }
    });
  };

  /** The draft-stage delete: the body names exactly the attachment. A
   *  lost response is NEVER an assumed success — the list re-read learns
   *  the truth. */
  const removeAttachment = (record: WireConversationAttachment) => {
    void transport.conversationAttachmentDelete(conversationId, { attachment_id: record.attachment_id }).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        runAttachmentsRead({ markRefreshing: true });
        setAnnounce(t("chat.attach.announce.removed"));
      } else if (outcome.kind === "rejection") {
        setAttachmentsFailure(outcome.envelope);
        setAnnounce(t("chat.attach.announce.removeRejected", { code: outcome.envelope.code }));
      } else {
        runAttachmentsRead({ markRefreshing: true });
        setAnnounce(t("chat.attach.announce.removeUnknown"));
      }
    });
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
            <p className={styles.help}>{t("chat.timeline.noAnswerNote")}</p>
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
        <AttachmentTray
          registryPhase={registryPhase}
          registryTypes={registryTypes}
          attachmentsPhase={attachmentsPhase}
          attachments={attachments}
          attachmentsFailure={attachmentsFailure}
          uploads={uploads}
          attachUnavailable={attachUnavailable}
          inputRevision={inputRevision}
          onRetryRegistry={runAttachmentRegistryRead}
          onRetryList={() => runAttachmentsRead({ markRefreshing: true })}
          onAttach={attachFiles}
          onCheckUpload={checkUploadStatus}
          onDismissUpload={dismissUpload}
          onRemoveAttachment={removeAttachment}
          attachInputRef={attachInputRef}
        />
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
                : resolvedExchange.state === "answered"
                ? `${t("chat.timeline.answeredState")}`
                : resolvedExchange.state === "failed"
                ? `${t("chat.timeline.failedState")}`
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

/** The attachment tray (IMPL-25; the ADOPTED S12 supply, P-S12-01…06, as
 *  implemented): the attach control rendering ONLY the Core's registry
 *  data (P-S12-01 — a refused or lost registry read leaves the control
 *  disabled with the reason; an empty registry leaves it absent — the
 *  Core supports nothing, and no dead control is invented), the
 *  conversation's attachment chips (the Core's records, each with its
 *  untrusted label AS DATA — P-S12-05), the in-flight upload states
 *  (P-S12-02's S0–S4: a lost response holds the truthful S4 state and
 *  resolves ONLY by the explicit status query), and the typed rejection
 *  rendering (P-S12-04 — the cause named, the safe next action shown, no
 *  raw internals). The stale-revision discard (P-S12-03) rides the upload
 *  machine above; the tray renders what the machines hold. NO
 *  message-binding, snapshot-participation, or streaming UI exists
 *  anywhere in this section. */
function AttachmentTray({
  registryPhase,
  registryTypes,
  attachmentsPhase,
  attachments,
  attachmentsFailure,
  uploads,
  attachUnavailable,
  inputRevision,
  onRetryRegistry,
  onRetryList,
  onAttach,
  onCheckUpload,
  onDismissUpload,
  onRemoveAttachment,
  attachInputRef,
}: {
  registryPhase: AttachmentRegistryPhase;
  registryTypes: readonly WireAttachmentTypeSpec[];
  attachmentsPhase: ListPhase;
  attachments: readonly WireConversationAttachment[];
  attachmentsFailure: CoreWireEnvelope | null;
  uploads: readonly UploadState[];
  attachUnavailable: boolean;
  inputRevision: number;
  onRetryRegistry: () => void;
  onRetryList: () => void;
  onAttach: (files: FileList | null) => void;
  onCheckUpload: (uploadId: string, revisionAtRequest: number) => void;
  onDismissUpload: (uploadId: string) => void;
  onRemoveAttachment: (record: WireConversationAttachment) => void;
  attachInputRef: React.RefObject<HTMLInputElement | null>;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const registryReady = registryPhase === "data" && registryTypes.length > 0;
  const registryEmpty = registryPhase === "data" && registryTypes.length === 0;
  const accept = registryReady ? registryTypes.map((member) => member.content_type).join(",") : undefined;
  // A chip renders once per Core record; an upload whose record the list
  // already carries does not render twice.
  const uploadChips = uploads.filter(
    (up) => up.phase !== "attached" || up.record === null || !attachments.some((a) => a.attachment_id === up.record?.attachment_id),
  );

  return (
    <div className={styles.attachSection}>
      <input
        ref={attachInputRef}
        type="file"
        className={styles.attachInput}
        accept={accept}
        multiple
        aria-hidden={attachInputRef.current === null}
        tabIndex={-1}
        onChange={(event) => onAttach(event.target.files)}
      />
      <div className={styles.attachControls}>
        {registryEmpty ? null : (
          <button
            type="button"
            className={styles.attachButton}
            onClick={() => attachInputRef.current?.click()}
            disabled={!registryReady}
          >
            {t("chat.attach.button")}
          </button>
        )}
        {registryPhase === "loading" ? <span className={styles.attachNote}>{t("chat.attach.registryLoading")}</span> : null}
        {registryPhase === "failure" || registryPhase === "unknown" || attachUnavailable ? (
          <>
            <span className={styles.attachNote}>{t("chat.attach.registryUnavailable")}</span>
            <button type="button" className={styles.attachRetry} onClick={onRetryRegistry}>
              {t("chat.attach.registryRetry")}
            </button>
          </>
        ) : null}
        {registryReady ? (
          <span className={styles.attachNote}>
            {t("chat.attach.registryNote", { types: registryTypes.map((member) => member.content_type).join(", ") })}
          </span>
        ) : null}
        {registryEmpty ? <span className={styles.attachNote}>{t("chat.attach.registryEmpty")}</span> : null}
      </div>

      {attachmentsPhase === "failure" && attachmentsFailure !== null ? (
        <p className={styles.attachFailure}>
          {t("chat.attach.listFailure", { code: attachmentsFailure.code })}{" "}
          <button type="button" className={styles.attachRetry} onClick={onRetryList}>
            {t("chat.attach.listRetry")}
          </button>
        </p>
      ) : null}
      {attachmentsPhase === "unknown" ? (
        <p className={styles.attachFailure}>
          {t("chat.attach.listUnknown")}{" "}
          <button type="button" className={styles.attachRetry} onClick={onRetryList}>
            {t("chat.attach.listRetry")}
          </button>
        </p>
      ) : null}

      {uploadChips.length === 0 && attachments.length === 0 && attachmentsPhase === "data" ? null : (
        <ul className={styles.attachChips}>
          {attachments.map((record) => (
            <AttachmentChip key={record.attachment_id} record={record} locale={locale} onRemove={onRemoveAttachment} />
          ))}
          {uploadChips.map((up) => (
            <UploadChip
              key={up.uploadId}
              upload={up}
              currentRevision={inputRevision}
              registryTypes={registryTypes}
              locale={locale}
              onCheck={() => onCheckUpload(up.uploadId, up.inputRevision)}
              onDismiss={() => onDismissUpload(up.uploadId)}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function formatBytes(value: number, locale: string | undefined): string {
  return new Intl.NumberFormat(locale).format(value);
}

/** One Core attachment record chip: the display name, the registry type,
 *  the CAS-verified size, and the untrusted label AS DATA (P-S12-05 —
 *  visually and semantically distinct, rendered as text, never
 *  color-alone). The remove control is the draft-stage delete affordance
 *  (the Core operation removes the record; a lost response is NEVER an
 *  assumed success). The chip invents no reference, no participation
 *  state, and no preview: content is data, never instruction. */
function AttachmentChip({
  record,
  locale,
  onRemove,
}: {
  record: WireConversationAttachment;
  locale: string | undefined;
  onRemove: (record: WireConversationAttachment) => void;
}) {
  const t = useT();
  return (
    <li className={styles.attachChip}>
      <span className={styles.attachChipName}>{record.file_name}</span>
      <span className={styles.attachChipMeta}>
        {record.content_type} · {t("chat.attach.bytes", { count: formatBytes(record.size_bytes, locale) })}
      </span>
      <span className={styles.attachChipUntrusted}>{t("chat.attach.untrustedLabel")}</span>
      <button type="button" className={styles.attachChipRemove} onClick={() => onRemove(record)}>
        {t("chat.attach.remove")}
      </button>
    </li>
  );
}

/** One client-side upload chip (the S0–S4 of the upload machine): the
 *  truthful state per phase — uploading (pending), unknown (the S4 state
 *  with the explicit Check status affordance — NEVER a blind re-upload),
 *  not-uploaded (the status query resolved NOT_FOUND: nothing was
 *  recorded; re-attaching is a new explicit act), rejected (the typed
 *  outcome — the Core's envelope cause when one exists, the Core
 *  registry's own served truth for the local guard — the safe next
 *  action shown, the code verbatim when present, no raw internals). */
function UploadChip({
  upload,
  currentRevision,
  registryTypes,
  locale,
  onCheck,
  onDismiss,
}: {
  upload: UploadState;
  currentRevision: number;
  registryTypes: readonly WireAttachmentTypeSpec[];
  locale: string | undefined;
  onCheck: () => void;
  onDismiss: () => void;
}) {
  const t = useT();
  const stale = upload.inputRevision !== currentRevision;
  const spec = registryTypes.find((member) => member.content_type === upload.contentType);
  return (
    <li className={`${styles.attachChip} ${styles.attachChipUpload}`}>
      <span className={styles.attachChipName}>{upload.fileName}</span>
      <span className={styles.attachChipMeta}>
        {upload.contentType !== "" ? `${upload.contentType} · ` : ""}
        {t("chat.attach.bytes", { count: formatBytes(upload.sizeBytes, locale) })}
      </span>
      {upload.phase === "uploading" ? (
        <span className={styles.attachChipState} role="status">
          {t("chat.attach.state.uploading")}
        </span>
      ) : null}
      {upload.phase === "unknown" ? (
        <>
          <span className={styles.attachChipState} role="status">
            {t("chat.attach.state.unknown")}
          </span>
          <button type="button" className={styles.attachRetry} onClick={onCheck}>
            {t("chat.attach.state.checkStatus")}
          </button>
        </>
      ) : null}
      {upload.phase === "not-uploaded" ? (
        <>
          <span className={styles.attachChipState}>{t("chat.attach.state.notUploaded")}</span>
          <button type="button" className={styles.attachRetry} onClick={onDismiss}>
            {t("chat.attach.state.dismiss")}
          </button>
        </>
      ) : null}
      {upload.phase === "rejected" ? (
        <>
          <span className={styles.attachChipRejected}>
            {upload.rejection !== null
              ? rejectionCopy(upload.rejection, t)
              : spec !== undefined
                ? t("chat.attach.rejection.size", { limit: formatBytes(spec.max_bytes, locale) })
                : t("chat.attach.rejection.type", { type: upload.contentType })}
          </span>
          {upload.rejection !== null ? (
            <span className={styles.attachChipCode}>
              <span className="visually-hidden">Code: </span>
              <code>{upload.rejection.code}</code>
            </span>
          ) : null}
          <button type="button" className={styles.attachRetry} onClick={onDismiss}>
            {t("chat.attach.state.dismiss")}
          </button>
        </>
      ) : null}
      {stale && upload.phase === "uploading" ? (
        <span className={styles.attachChipMeta}>{t("chat.attach.state.staleHint")}</span>
      ) : null}
    </li>
  );
}

/** The typed rejection copy (P-S12-04): the cause named from the
 *  envelope's bounded data, the safe next action, no raw internals — the
 *  stable code itself renders beside the copy (the machine identifier is
 *  language-neutral). */
function rejectionCopy(envelope: CoreWireEnvelope, t: TranslateFn): string {
  const cause = typeof envelope.data?.cause === "string" ? envelope.data.cause : "";
  if (cause === "type") return t("chat.attach.rejection.type", { type: "" });
  if (cause === "size") return t("chat.attach.rejection.size", { limit: "" });
  if (cause === "policy") return t("chat.attach.rejection.policy");
  return t("chat.attach.rejection.generic", { code: envelope.code });
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
  const isUser = message.role === "user";
  const isAssistant = message.role === "assistant";
  const roleLabel = isUser
    ? t("chat.timeline.roleYou")
    : isAssistant
    ? t("chat.timeline.roleAssistant")
    : null;

  return (
    <li className={`${styles.message} ${isAssistant ? styles.messageAssistant : ""}`}>
      <div className={styles.messageMeta}>
        {roleLabel !== null ? (
          <span className={isAssistant ? styles.roleChipAssistant : styles.roleChip}>{roleLabel}</span>
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
      {message.correlation_id !== undefined && isUser ? (
        <p className={styles.noJobMarker}>{t("chat.timeline.noJobMarker")}</p>
      ) : null}
    </li>
  );
}
