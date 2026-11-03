import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type {
  WireCapabilitySet,
  WireReviewEffect,
  WireReviewPayload,
  WireSnapshotView,
  WireSubmitRequest,
  WireSubmitResult,
} from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./RunTaskPage.module.css";
import { jobStateChip } from "../jobs/jobStates";
import { SnapshotViewPanel } from "../jobs/JobDetailPage";

/**
 * `/jobs/new` — the Run task submission flow (IMPL-6; the owner's card-13
 * working candidate over the delivered transport surface).
 *
 * The laws this surface implements, exactly as the records bind them:
 * - X-1: Run ≠ Ask. This flow is reachable ONLY through the explicit
 *   `+ New → Run a task` entry; it creates ONE durable Job through Core
 *   validation, and the conversational Ask path (which has no surface
 *   here at all) never submits.
 * - The pre-submit review renders the Core's eight review fields EXACTLY
 *   as the Core returns them (`POST /v1/jobs/review` — a READ: the Core
 *   creates nothing). Missing information renders Not-available or the
 *   Core's own machine value verbatim — never zero, never guessed.
 * - The 202 law: an accepted-for-processing response is NEVER displayed
 *   as completion. The durable Job card renders from the Core-confirmed
 *   reference and links to the canonical `/jobs/:jobId` detail (R-1).
 * - The S0–S4 model (P1.6): duplicate submit is disabled while pending;
 *   a lost response is the truthful S4 unknown — NEVER an automatic
 *   retry. The one permitted re-issue is the explicit, user-initiated
 *   SAME-KEY re-send (the Supplied idempotency class: the Core returns
 *   the original job for the same key + same input, never a second job).
 * - The draft-safe rule: the draft lives in this component's memory for
 *   the page's lifetime only — never persisted, and no secret belongs in
 *   a task.
 * - IMPL-16 (the exact-only boundary — the owner's 2026-11-04 LIVE
 *   decision): the pre-send context-snapshot disclosure renders the
 *   truthful Not available / Not reported states for every fact the
 *   delivered Core does not supply before compile — no pre-compile
 *   preview, no approximation, no fabricated token count, no missing
 *   cost as $0; the `approximate` case stays the Core/API owner's
 *   recorded item. The post-submission snapshot region consumes ONLY
 *   the delivered exact snapshot read (IMPL-9's `jobSnapshot`) —
 *   explicit, user-initiated, read-sequence-guarded, never auto-retried;
 *   a Core NOT_FOUND is the honest not-yet-compiled state.
 * - Machine identifiers (effect vocabulary values, denial reasons,
 *   capability values, classification labels) render verbatim as the
 *   language-neutral identifiers they are; only section labels are
 *   localized copy.
 */

/** The coded provider-ID register (internal/domain/provider/contract.go,
 *  the twelve ids the code defines). The options are this coded
 *  vocabulary — the Core's own wired-set truth arrives in the review
 *  payload's `provider_model.wired` (an unwired choice is the Core's
 *  typed truth, never a client-side refusal). */
const CODED_PROVIDER_IDS: readonly string[] = [
  "openai",
  "anthropic",
  "google-gemini",
  "alibaba-qwen",
  "glm-zai",
  "kimi-moonshot",
  "minimax",
  "deepseek",
  "custom-openai",
  "custom-anthropic",
  "local",
  "mock",
];

/** The coded classification vocabulary (internal/domain/provider/
 *  datahandling.go): the empty label is the protected-unlabeled default
 *  that blocks every real provider (C3-19); the two coded labels are the
 *  v1 egress labels the code defines. */
const CODED_CLASSIFICATIONS: readonly { readonly value: string; readonly labelKey: "run.form.classificationUnlabeled" | "run.form.classificationNonPersonal" | "run.form.classificationTestFixture" }[] = [
  { value: "", labelKey: "run.form.classificationUnlabeled" },
  { value: "non-personal", labelKey: "run.form.classificationNonPersonal" },
  { value: "test-fixture", labelKey: "run.form.classificationTestFixture" },
];

type ReviewPhase = "idle" | "loading" | "data" | "failure" | "unknown";
type SubmitPhase = "idle" | "submitting" | "accepted" | "rejected" | "unknown";

/** The post-submission context-snapshot read (IMPL-16; the delivered
 *  exact read — IMPL-9's `jobSnapshot`): loading / the structured view /
 *  the honest not-yet-compiled state (a Core NOT_FOUND — the Core
 *  compiles per executed attempt) / the typed refusal / the truthful
 *  unknown. The same truthful-state shape the Job detail's Context
 *  Inspector read uses (JobDetailPage's SnapshotRead pattern). */
type SnapshotPreviewRead =
  | { readonly kind: "notLoaded" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly snapshot: WireSnapshotView; readonly confirmedAt: Date }
  | { readonly kind: "noSnapshot" }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

interface RunDraft {
  readonly task: string;
  readonly system: string;
  readonly provider: string;
  readonly model: string;
  readonly classification: string;
  readonly maxOutputTokens: string;
}

const EMPTY_DRAFT: RunDraft = {
  task: "",
  system: "",
  provider: "mock",
  model: "",
  classification: "",
  maxOutputTokens: "",
};

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(value);
}

/** The idempotency key is minted at submit time, never at review time.
 * A submission without a unique key must never be sent. */
function newIdempotencyKey(): string {
  if (typeof crypto === "undefined" || typeof crypto.randomUUID !== "function") {
    throw new Error("idempotency key source unavailable");
  }
  return crypto.randomUUID();
}

function wireRequestFromDraft(draft: RunDraft): Omit<WireSubmitRequest, "idempotency_key"> {
  const maxOutput = Number.parseInt(draft.maxOutputTokens, 10);
  return {
    task: draft.task,
    ...(draft.system.trim() !== "" ? { system: draft.system } : {}),
    provider: draft.provider,
    ...(draft.model.trim() !== "" ? { model: draft.model } : {}),
    ...(Number.isFinite(maxOutput) && maxOutput > 0 ? { max_output_tokens: maxOutput } : {}),
    ...(draft.classification !== "" ? { classification: draft.classification } : {}),
    deadline_secs: 0,
  };
}

export function RunTaskPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const { transport } = useCoreConnection();

  const [draft, setDraft] = useState<RunDraft>(EMPTY_DRAFT);
  const [reviewPhase, setReviewPhase] = useState<ReviewPhase>("idle");
  const [review, setReview] = useState<WireReviewPayload | null>(null);
  const [reviewFailure, setReviewFailure] = useState<CoreWireEnvelope | null>(null);
  const [reviewLoading, setReviewLoading] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);

  const [submitPhase, setSubmitPhase] = useState<SubmitPhase>("idle");
  const [submitResult, setSubmitResult] = useState<WireSubmitResult | null>(null);
  const [submitFailure, setSubmitFailure] = useState<CoreWireEnvelope | null>(null);
  // The post-submission snapshot read (IMPL-16): reset on every new
  // submission — a fresh durable job id invalidates any prior read.
  const [snapshotPreview, setSnapshotPreview] = useState<SnapshotPreviewRead>({ kind: "notLoaded" });
  // The pending submission's identity: the idempotency key and the EXACT
  // wire body it rode. The S4 re-send re-posts this snapshot verbatim —
  // the same key AND the same input, which is what makes the Core's
  // replay return the original job instead of a conflict or a duplicate.
  const [pendingKey, setPendingKey] = useState<string | null>(null);
  const [pendingBody, setPendingBody] = useState<Omit<WireSubmitRequest, "idempotency_key"> | null>(null);
  const [localError, setLocalError] = useState<string | null>(null);
  const [announce, setAnnounce] = useState("");

  // The read-sequence guard (the Jobs-list pattern): only the LATEST
  // issued review applies, so a draft change supersedes an in-flight
  // review instead of racing it.
  const reviewSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const reviewRegionRef = useRef<HTMLDivElement>(null);
  const snapshotPreviewSeqRef = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const reviewable = draft.task.trim() !== "" && draft.provider !== "";

  const requestReview = useCallback(
    (options: { readonly markRefreshing: boolean; readonly focusOnArrival: boolean }) => {
      const seq = ++reviewSeqRef.current;
      if (options.markRefreshing) setReviewLoading(true);
      else setReviewPhase("loading");
      setReviewFailure(null);

      const maxOutput = Number.parseInt(draft.maxOutputTokens, 10);
      void transport
        .review({
          task: draft.task,
          ...(draft.system.trim() !== "" ? { system: draft.system } : {}),
          provider: draft.provider,
          ...(draft.model.trim() !== "" ? { model: draft.model } : {}),
          ...(Number.isFinite(maxOutput) && maxOutput > 0 ? { max_output_tokens: maxOutput } : {}),
          ...(draft.classification !== "" ? { classification: draft.classification } : {}),
        })
        .then((outcome) => {
          if (!mountedRef.current || seq !== reviewSeqRef.current) return; // superseded
          setReviewLoading(false);
          if (outcome.kind === "data") {
            setReview(outcome.data.review);
            setLastConfirmedAt(new Date());
            setReviewPhase("data");
            setAnnounce(t("run.announce.reviewLoaded"));
            if (options.focusOnArrival) reviewRegionRef.current?.focus();
          } else if (outcome.kind === "rejection") {
            setReviewFailure(outcome.envelope);
            setReviewPhase("failure");
          } else {
            setReviewPhase("unknown");
          }
        });
    },
    // The review re-runs when the draft's reviewable content changes —
    // the same server-driven pattern the Jobs list uses for its filter.
    [draft.task, draft.system, draft.provider, draft.model, draft.classification, draft.maxOutputTokens, transport, t],
  );

  useEffect(() => {
    if (!reviewable) {
      reviewSeqRef.current += 1; // supersede any in-flight review
      setReviewPhase("idle");
      setReview(null);
      setReviewFailure(null);
      setLastConfirmedAt(null);
      return;
    }
    requestReview({ markRefreshing: false, focusOnArrival: false });
  }, [reviewable, requestReview]);

  // The submission-blocking rule (P3.6 §4.2): the gaps the Core itself
  // reports block submission, each with its reason. An undetermined
  // provider-call effect class blocks too — the user must be able to
  // tell BEFORE submission whether external side effects may occur.
  const blockers: readonly string[] = useMemo(() => {
    if (reviewPhase !== "data" || review === null) return [];
    const gaps: string[] = [];
    if (!review.provider_model.wired) gaps.push(t("run.review.blockProviderUnwired"));
    if (!review.permission.allowed) gaps.push(t("run.review.blockPermission"));
    if (!review.policy.egress_allowed) gaps.push(t("run.review.blockEgress"));
    const call = review.tool_effects.find((effect: WireReviewEffect) => effect.effect === "provider_call");
    if (call !== undefined && call.class === "unknown") gaps.push(t("run.review.blockUnknownEffect"));
    return gaps;
  }, [reviewPhase, review, t]);

  const submitDisabled =
    !reviewable || reviewPhase !== "data" || blockers.length > 0 || submitPhase === "submitting";

  // The post-submission context-snapshot read (IMPL-16; the delivered
  // exact read — IMPL-9's `jobSnapshot`): an explicit, user-initiated
  // QUERY, read-sequence-guarded, mutating nothing. A Core NOT_FOUND is
  // the honest not-yet-compiled state (the Core compiles the exact
  // snapshot per executed attempt), never an empty structure and never
  // an error panel. No retry exists anywhere: a lost response stays the
  // truthful unknown until the user reads again.
  const readSnapshotPreview = useCallback(() => {
    if (submitResult === null || snapshotPreview.kind === "loading") return;
    const seq = ++snapshotPreviewSeqRef.current;
    setSnapshotPreview({ kind: "loading" });
    void transport.jobSnapshot(submitResult.job.job_id).then((outcome) => {
      if (!mountedRef.current || seq !== snapshotPreviewSeqRef.current) return;
      if (outcome.kind === "data") {
        setSnapshotPreview({ kind: "data", snapshot: outcome.data.snapshot, confirmedAt: new Date() });
        setAnnounce(t("run.snapshot.preview.announceData"));
      } else if (outcome.kind === "rejection") {
        const notFound = outcome.envelope.code === "NOT_FOUND";
        setSnapshotPreview(
          notFound ? { kind: "noSnapshot" } : { kind: "failure", envelope: outcome.envelope },
        );
        setAnnounce(
          notFound
            ? t("run.snapshot.preview.announceNoSnapshot")
            : t("run.snapshot.preview.announceFailure"),
        );
      } else {
        setSnapshotPreview({ kind: "unknown" });
        setAnnounce(t("run.snapshot.preview.announceUnknown"));
      }
    });
  }, [submitResult, snapshotPreview.kind, transport, t]);

  const submit = () => {
    if (submitDisabled) return;
    let key: string;
    try {
      key = newIdempotencyKey();
    } catch {
      setLocalError(t("run.submit.keyUnavailable"));
      return;
    }
    const body = wireRequestFromDraft(draft);
    setLocalError(null);
    setSubmitResult(null);
    setSubmitFailure(null);
    setSnapshotPreview({ kind: "notLoaded" });
    setSubmitPhase("submitting");
    setPendingKey(key);
    setPendingBody(body);
    setAnnounce(t("run.announce.submitting"));
    void transport.submit({ ...body, idempotency_key: key }).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setSubmitResult(outcome.data);
        setSubmitPhase("accepted");
        setPendingKey(null);
        setPendingBody(null);
        setAnnounce(t("run.announce.accepted"));
      } else if (outcome.kind === "rejection") {
        setSubmitFailure(outcome.envelope);
        setSubmitPhase("rejected");
        setPendingKey(null);
        setPendingBody(null);
        setAnnounce(t("run.announce.rejected"));
      } else {
        // S4: the response was lost or uninterpretable. The key and the
        // exact body stay held so the explicit same-key re-send resolves
        // the Core's replay — never an automatic retry.
        setSubmitPhase("unknown");
        setAnnounce(t("run.announce.unknown"));
      }
    });
  };

  // The Supplied-idempotency recovery: an explicit, user-initiated
  // re-issue of the SAME key and the EXACT same input. The Core's coded
  // semantics return the original job — never a second one.
  const resendSameKey = () => {
    if (pendingKey === null || pendingBody === null || submitPhase === "submitting") return;
    setSubmitPhase("submitting");
    setSnapshotPreview({ kind: "notLoaded" });
    setAnnounce(t("run.announce.submitting"));
    void transport.submit({ ...pendingBody, idempotency_key: pendingKey }).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setSubmitResult(outcome.data);
        setSubmitPhase("accepted");
        setPendingKey(null);
        setPendingBody(null);
        setAnnounce(t("run.announce.accepted"));
      } else if (outcome.kind === "rejection") {
        setSubmitFailure(outcome.envelope);
        setSubmitPhase("rejected");
        setPendingKey(null);
        setPendingBody(null);
        setAnnounce(t("run.announce.rejected"));
      } else {
        setSubmitPhase("unknown");
        setAnnounce(t("run.announce.unknown"));
      }
    });
  };

  const updateDraft = (patch: Partial<RunDraft>) => {
    setDraft((prev) => ({ ...prev, ...patch }));
    // Editing the draft supersedes a prior outcome display: the next
    // submission is a new intentional act with a fresh idempotency key.
    if (submitPhase === "rejected") {
      setSubmitPhase("idle");
      setSubmitFailure(null);
    }
  };

  return (
    <PageLayout>
      <PageHeading>{t("run.title")}</PageHeading>
      <p className={styles.description}>{t("run.description")}</p>
      <p className={styles.note}>{t("run.intentionalEntry")}</p>
      <p className={styles.note}>{t("run.draftNote")}</p>

      <PageSection>
        <div className={styles.form}>
          <label className={styles.label} htmlFor="run-task">
            {t("run.form.taskLabel")}
          </label>
          <textarea
            id="run-task"
            className={styles.textarea}
            value={draft.task}
            onChange={(event) => updateDraft({ task: event.target.value })}
            rows={5}
          />
          <p className={styles.help}>{t("run.form.taskHelp")}</p>

          <label className={styles.label} htmlFor="run-system">
            {t("run.form.systemLabel")}
          </label>
          <textarea
            id="run-system"
            className={styles.textarea}
            value={draft.system}
            onChange={(event) => updateDraft({ system: event.target.value })}
            rows={3}
          />
          <p className={styles.help}>{t("run.form.systemHelp")}</p>

          <label className={styles.label} htmlFor="run-provider">
            {t("run.form.providerLabel")}
          </label>
          <select
            id="run-provider"
            className={styles.select}
            value={draft.provider}
            onChange={(event) => updateDraft({ provider: event.target.value })}
          >
            {CODED_PROVIDER_IDS.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
          <p className={styles.help}>{t("run.form.providerHelp")}</p>

          <label className={styles.label} htmlFor="run-model">
            {t("run.form.modelLabel")}
          </label>
          <input
            id="run-model"
            className={styles.input}
            type="text"
            value={draft.model}
            onChange={(event) => updateDraft({ model: event.target.value })}
          />
          <p className={styles.help}>{t("run.form.modelHelp")}</p>

          <label className={styles.label} htmlFor="run-classification">
            {t("run.form.classificationLabel")}
          </label>
          <select
            id="run-classification"
            className={styles.select}
            value={draft.classification}
            onChange={(event) => updateDraft({ classification: event.target.value })}
          >
            {CODED_CLASSIFICATIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {t(option.labelKey)}
                {option.value !== "" ? ` (${option.value})` : ""}
              </option>
            ))}
          </select>
          <p className={styles.help}>{t("run.form.classificationHelp")}</p>

          <label className={styles.label} htmlFor="run-max-output">
            {t("run.form.maxOutputLabel")}
          </label>
          <input
            id="run-max-output"
            className={styles.input}
            type="number"
            min={1}
            value={draft.maxOutputTokens}
            onChange={(event) => updateDraft({ maxOutputTokens: event.target.value })}
          />
          <p className={styles.help}>{t("run.form.maxOutputHelp")}</p>
        </div>
      </PageSection>

      <PageSection>
        <div
          ref={reviewRegionRef}
          tabIndex={-1}
          className={styles.reviewRegion}
          aria-label={t("run.review.heading")}
        >
          <h2 className={styles.sectionHeading}>{t("run.review.heading")}</h2>
          {!reviewable ? (
            <p className={styles.note}>{t("run.review.empty")}</p>
          ) : reviewPhase === "loading" ? (
            <p className={styles.note}>{t("run.review.loading")}</p>
          ) : reviewPhase === "failure" && reviewFailure !== null ? (
            <>
              <RecoveryPanel
                title={t("run.review.failureTitle")}
                body={t("run.review.failureBody")}
                referenceId={reviewFailure.correlationId}
                actions={[
                  {
                    label: t("run.review.refresh"),
                    onAction: () => requestReview({ markRefreshing: true, focusOnArrival: true }),
                  },
                ]}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{reviewFailure.code}</code>
              </p>
            </>
          ) : reviewPhase === "unknown" ? (
            <RecoveryPanel
              title={t("run.review.unknownTitle")}
              body={t("run.review.unknownBody")}
              actions={[
                {
                  label: t("run.review.refresh"),
                  onAction: () => requestReview({ markRefreshing: true, focusOnArrival: true }),
                },
              ]}
            />
          ) : reviewPhase === "data" && review !== null ? (
            <>
              {lastConfirmedAt !== null ? (
                <p className={styles.confirmed}>
                  {t("run.review.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
                  {reviewLoading ? ` — ${t("run.review.refreshing")}` : ""}
                </p>
              ) : null}
              <ReviewPayloadView payload={review} />
              <div className={styles.blockers}>
                {blockers.length > 0 ? (
                  <>
                    <p className={styles.blockedHeading}>{t("run.review.blockedHeading")}</p>
                    <ul className={styles.blockList}>
                      {blockers.map((gap) => (
                        <li key={gap}>{gap}</li>
                      ))}
                    </ul>
                  </>
                ) : (
                  <p className={styles.note}>{t("run.review.notBlocked")}</p>
                )}
              </div>
            </>
          ) : null}
        </div>
      </PageSection>

      <PageSection>
        <SnapshotDisclosure />
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionHeading}>{t("run.submit.heading")}</h2>
        {reviewPhase === "data" && review !== null ? <PreSendDisclosureLine review={review} /> : null}
        <div className={styles.submitRow}>
          <button type="button" className={styles.primary} onClick={submit} disabled={submitDisabled}>
            {submitPhase === "submitting" ? t("run.submit.submitting") : t("run.submit.action")}
          </button>
          {submitPhase === "submitting" ? (
            <p className={styles.pending} role="status">
              {t("run.submit.pendingNote")}
            </p>
          ) : null}
        </div>
        {localError !== null ? <p className={styles.help}>{localError}</p> : null}

        {submitPhase === "accepted" && submitResult !== null ? (
          <>
            <AcceptedCard result={submitResult} />
            <SnapshotPreviewRegion read={snapshotPreview} onRead={readSnapshotPreview} />
          </>
        ) : null}

        {submitPhase === "rejected" && submitFailure !== null ? (
          <>
            <RecoveryPanel
              title={t("run.submit.rejectedTitle")}
              body={t("run.submit.rejectedBody")}
              referenceId={submitFailure.correlationId}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{submitFailure.code}</code>
            </p>
          </>
        ) : null}

        {submitPhase === "unknown" ? (
          <RecoveryPanel
            title={t("run.submit.unknownTitle")}
            body={t("run.submit.unknownBody")}
            note={t("run.submit.unknownResendNote")}
            actions={[{ label: t("run.submit.unknownResend"), onAction: resendSameKey }]}
          />
        ) : null}
        {submitPhase === "unknown" && pendingKey !== null ? (
          <p className={styles.code}>
            <span className={styles.keyLabel}>{t("run.submit.idempotencyKeyLabel")}: </span>
            <code>{pendingKey}</code>
          </p>
        ) : null}

        {submitPhase === "unknown" ? (
          <p className={styles.linkRow}>
            <Link to="/jobs">{t("run.submit.openJobsList")}</Link>
          </p>
        ) : null}
      </PageSection>

      <p role="status" className="visually-hidden">
        {announce}
      </p>
    </PageLayout>
  );
}

/** The durable Job card (the S2-of-the-acknowledgment rendering): the
 * Core-confirmed reference, the 202 law stated in copy, the replay note
 * when the Core reports one, and the canonical-detail link (R-1). */
function AcceptedCard({ result }: { result: WireSubmitResult }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const chip = jobStateChip(result.job.state);

  return (
    <div className={styles.acceptedCard}>
      <p className={styles.acceptedTitle}>{t("run.submit.acceptedTitle")}</p>
      <p>{t("run.submit.acceptedBody")}</p>
      {result.replayed ? <p className={styles.note}>{t("run.submit.replayedNote")}</p> : null}
      <dl className={styles.facts}>
        <div className={styles.factRow}>
          <dt>{t("run.submit.jobIdLabel")}</dt>
          <dd>
            <Link to={`/jobs/${encodeURIComponent(result.job.job_id)}`} className={styles.jobLink}>
              <code>{result.job.job_id}</code>
              <span className="visually-hidden">{t("run.submit.openJob", { jobId: result.job.job_id })}</span>
            </Link>
          </dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.submit.stateLabel")}</dt>
          <dd>
            <span className={`${styles.chip} ${styles[chip.tone]}`}>
              <chip.icon aria-hidden="true" size={14} />
              {t(chip.labelKey)}
            </span>
          </dd>
        </div>
        {result.job.correlation_id !== undefined ? (
          <div className={styles.factRow}>
            <dt>{t("run.submit.correlationLabel")}</dt>
            <dd>
              <code>{result.job.correlation_id}</code>
            </dd>
          </div>
        ) : null}
        <div className={styles.factRow}>
          <dt>{t("jobs.list.columnCreated")}</dt>
          <dd>
            <time dateTime={result.job.created_at}>
              {formatTimestamp(new Date(result.job.created_at), locale)}
            </time>
          </dd>
        </div>
      </dl>
      <p className={styles.linkRow}>
        <Link to="/jobs">{t("run.submit.openJobsList")}</Link>
      </p>
    </div>
  );
}

/** The eight review fields, rendered EXACTLY as the Core returns them:
 * machine values verbatim, absent facts Not-available, coded `unknown`
 * values as the honest unknown they are. */
function ReviewPayloadView({ payload }: { payload: WireReviewPayload }) {
  const t = useT();

  const capabilityValue = (value: string): string => {
    switch (value) {
      case "supported":
        return t("run.review.capSupported");
      case "unsupported":
        return t("run.review.capUnsupported");
      case "unknown":
        return t("run.review.capUnknown");
      default:
        return value;
    }
  };

  return (
    <div className={styles.review}>
      <section className={styles.reviewField}>
        <h3>{t("run.review.intentHeading")}</h3>
        <p className={styles.taskEcho}>{payload.intent.task}</p>
        {payload.intent.system !== undefined && payload.intent.system !== "" ? (
          <p className={styles.systemEcho}>{payload.intent.system}</p>
        ) : null}
        <p className={styles.note}>
          {payload.intent.schema_provided ? t("run.review.intentSchemaProvided") : t("run.review.intentNoSchema")}
        </p>
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.sourcesHeading")}</h3>
        {payload.sources.state === "none" ? (
          <p>{t("run.review.sourcesNone")}</p>
        ) : payload.sources.state === "resolved" ? (
          <p>
            {t("run.review.sourcesResolved", {
              packId: payload.sources.pack_id ?? "",
              version: payload.sources.version ?? 0,
              status: payload.sources.status ?? "",
            })}
            {payload.sources.scope !== undefined ? (
              <>
                {" "}
                {t("run.review.sourcesScope", { scope: payload.sources.scope })}
              </>
            ) : null}
          </p>
        ) : payload.sources.state === "unresolved" ? (
          <p>{t("run.review.sourcesUnresolved", { reason: payload.sources.reason ?? "" })}</p>
        ) : (
          <p>{t("run.review.sourcesUnavailable")}</p>
        )}
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.providerHeading")}</h3>
        <p>
          <code>{payload.provider_model.provider}</code>
          {payload.provider_model.model !== undefined && payload.provider_model.model !== "" ? (
            <>
              {" / "}
              <code>{payload.provider_model.model}</code>
            </>
          ) : null}
          {" — "}
          {payload.provider_model.wired ? t("run.review.providerWired") : t("run.review.providerUnwired")}
        </p>
        {payload.provider_model.profile !== undefined ? (
          <p className={styles.note}>
            {t("run.review.profileHeading")}:{" "}
            <code>
              {payload.provider_model.profile.provider}/{payload.provider_model.profile.endpoint}/
              {payload.provider_model.profile.model}
            </code>
          </p>
        ) : null}
        {payload.provider_model.capabilities !== undefined ? (
          <CapabilityFacts capabilities={payload.provider_model.capabilities} valueLabel={capabilityValue} />
        ) : (
          <p className={styles.note}>{t("run.review.notAvailable")}</p>
        )}
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.effectsHeading")}</h3>
        <p className={styles.note}>{t("run.review.effectsIntro")}</p>
        <ul className={styles.effectList}>
          {payload.tool_effects.map((effect) => (
            <li key={effect.effect} className={styles.effectRow}>
              <span className={styles.effectName}>
                {effect.effect === "provider_call"
                  ? t("run.review.effectProviderCall")
                  : effect.effect === "durable_state"
                    ? t("run.review.effectDurableState")
                    : effect.effect}
              </span>
              <span className={styles.effectFacts}>
                <span>
                  {t("run.review.effectClass")}:{" "}
                  {effect.class === "unknown" ? t("run.review.classUnknown") : <code>{effect.class}</code>}
                </span>
                <span>
                  {t("run.review.effectRetry")}: <code>{effect.retry}</code>
                </span>
                {effect.idempotency_scope !== undefined ? (
                  <span>
                    {t("run.review.effectScope")}: <code>{effect.idempotency_scope}</code>
                  </span>
                ) : null}
                <span>
                  {t("run.review.effectUnknownOutcome")}: <code>{effect.unknown_outcome}</code>
                </span>
                {effect.unknown_outcome === "needs_reconciliation" ? (
                  <span className={styles.note}>{t("run.review.unknownNeedsReconciliationNote")}</span>
                ) : null}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.permissionHeading")}</h3>
        {payload.permission.allowed ? (
          <p>{t("run.review.permissionAllowed")}</p>
        ) : (
          <>
            <p>{t("run.review.permissionDenied")}</p>
            <p className={styles.note}>
              {t("run.review.permissionReason", { reason: payload.permission.reason })}
              {payload.permission.action !== "" ? (
                <>
                  {" "}
                  {t("run.review.permissionAction", { action: payload.permission.action })}
                </>
              ) : null}
            </p>
          </>
        )}
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.policyHeading")}</h3>
        {payload.policy.profile_known ? (
          <>
            <p className={styles.note}>
              {t("run.review.policyReview", { review: payload.policy.review ?? "" })}
            </p>
            <dl className={styles.factGrid}>
              <FactRow label={t("run.review.factDeployment")} value={payload.policy.deployment} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
              <FactRow label={t("run.review.factTrainingUse")} value={payload.policy.training_use} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
              <FactRow label={t("run.review.factAbuseRetention")} value={payload.policy.abuse_retention} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
              <FactRow label={t("run.review.factApplicationStateTtl")} value={payload.policy.application_state_ttl} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
              <FactRow label={t("run.review.factSubprocessors")} value={payload.policy.subprocessors} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
              <FactRow label={t("run.review.factDeletionEligibility")} value={payload.policy.deletion_eligibility} unknownLabel={t("run.review.factsUnknownValue")} notAvailableLabel={t("run.review.notAvailable")} />
            </dl>
          </>
        ) : (
          <p>{t("run.review.policyNoRecord", { reason: payload.policy.egress_reason ?? "" })}</p>
        )}
        <h4 className={styles.subHeading}>{t("run.review.egressHeading")}</h4>
        {payload.policy.egress_allowed ? (
          <p>{t("run.review.egressAllowed")}</p>
        ) : (
          <p>
            {t("run.review.egressBlocked")}{" "}
            <span className={styles.note}>
              {t("run.review.egressReason", { reason: payload.policy.egress_reason ?? "" })}
            </span>
          </p>
        )}
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.approvalHeading")}</h3>
        {payload.approval_need.required ? (
          <p>{t("run.review.approvalRequired")}</p>
        ) : (
          <p>{t("run.review.approvalNotRequired")}</p>
        )}
      </section>

      <section className={styles.reviewField}>
        <h3>{t("run.review.budgetHeading")}</h3>
        {payload.budget.reserved_output_tokens !== undefined ? (
          <p>{t("run.review.budgetReservedOutput", { value: payload.budget.reserved_output_tokens })}</p>
        ) : (
          <p className={styles.note}>{t("run.review.budgetAbsent")}</p>
        )}
      </section>
    </div>
  );
}

function FactRow({
  label,
  value,
  unknownLabel,
  notAvailableLabel,
}: {
  label: string;
  value: string | undefined;
  unknownLabel: string;
  notAvailableLabel: string;
}) {
  return (
    <div className={styles.factRow}>
      <dt>{label}</dt>
      <dd>
        {value === undefined || value === ""
          ? notAvailableLabel
          : value === "unknown"
            ? unknownLabel
            : (
                <code>{value}</code>
              )}
      </dd>
    </div>
  );
}

function CapabilityFacts({
  capabilities,
  valueLabel,
}: {
  capabilities: WireCapabilitySet;
  valueLabel: (value: string) => string;
}) {
  const t = useT();
  const axes: readonly { readonly label: string; readonly value: string }[] = [
    { label: "chat", value: capabilities.chat },
    { label: "streaming", value: capabilities.streaming },
    { label: "tools", value: capabilities.tools },
    { label: "parallel_tool_calls", value: capabilities.parallel_tool_calls },
    { label: "structured_output", value: capabilities.structured_output },
    { label: "vision", value: capabilities.vision },
    { label: "audio", value: capabilities.audio },
    { label: "embeddings", value: capabilities.embeddings },
    { label: "batch", value: capabilities.batch },
    { label: "context_caching", value: capabilities.context_caching },
    { label: "conversation_continuation", value: capabilities.conversation_continuation },
  ];
  return (
    <div>
      <h4 className={styles.subHeading}>{t("run.review.capabilitiesHeading")}</h4>
      <dl className={styles.factGrid}>
        {axes.map((axis) => (
          <div key={axis.label} className={styles.factRow}>
            <dt>
              <code>{axis.label}</code>
            </dt>
            <dd>{valueLabel(axis.value)}</dd>
          </div>
        ))}
        <div className={styles.factRow}>
          <dt>
            <code>max_context_tokens</code>
          </dt>
          <dd>
            {capabilities.max_context_tokens !== undefined && capabilities.max_context_tokens > 0
              ? t("run.review.capMaxContext", { value: capabilities.max_context_tokens })
              : t("run.review.capLimitUnknown")}
          </dd>
        </div>
        <div className={styles.factRow}>
          <dt>
            <code>max_output_tokens</code>
          </dt>
          <dd>
            {capabilities.max_output_tokens !== undefined && capabilities.max_output_tokens > 0
              ? t("run.review.capMaxOutput", { value: capabilities.max_output_tokens })
              : t("run.review.capLimitUnknown")}
          </dd>
        </div>
      </dl>
    </div>
  );
}

/** The pre-send context-snapshot disclosure (IMPL-16; P3.5 §6 rows
 *  2/3/5/6/7 rendered truthfully for the pre-compile region): every fact
 *  the delivered Core does NOT supply before compile renders its honest
 *  Not available / Not reported state — never a fabricated preview, never
 *  a fake token count, never a missing cost as $0, never an estimate
 *  displayed as an actual. The exact-only boundary is stated in copy: no
 *  approximation or pre-compile preview is displayed anywhere on this
 *  surface (the owner's 2026-11-04 exact-only decision; the
 *  `approximate` case stays the Core/API owner's recorded item). The one
 *  pre-send budget fact the Core DOES report (the reserved output bound)
 *  renders in the review's Estimated budget field above; the data-boundary
 *  facts render in the review's Policy field — this section never
 *  duplicates or re-invents them. */
function SnapshotDisclosure() {
  const t = useT();

  return (
    <div>
      <h2 className={styles.sectionHeading}>{t("run.snapshot.heading")}</h2>
      <p className={styles.description}>{t("run.snapshot.intro")}</p>
      <p className={styles.note}>{t("run.snapshot.exactOnlyNote")}</p>
      <dl className={styles.facts}>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.snapshotCompiledLabel")}</dt>
          <dd>{t("run.snapshot.snapshotCompiledMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.includedLabel")}</dt>
          <dd>{t("run.snapshot.includedMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.omittedLabel")}</dt>
          <dd>{t("run.snapshot.omittedMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.budgetEstimateLabel")}</dt>
          <dd>{t("run.snapshot.budgetEstimateMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.providerActualLabel")}</dt>
          <dd>{t("run.snapshot.providerActualMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.costLabel")}</dt>
          <dd>{t("run.snapshot.costMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.exactnessLabel")}</dt>
          <dd>{t("run.snapshot.exactnessMissing")}</dd>
        </div>
        <div className={styles.factRow}>
          <dt>{t("run.snapshot.whereExactLabel")}</dt>
          <dd>{t("run.snapshot.whereExactBody")}</dd>
        </div>
      </dl>
      <p className={styles.note}>{t("run.snapshot.boundaryPointer")}</p>
    </div>
  );
}

/** The compact pre-send disclosure line (IMPL-16; P3.5 §6 row 8 / §3
 *  row 8): the active provider/model and the relevant egress notice
 *  render BEFORE send — visible pre-send, keyboard-visible, one compact
 *  wrapping line that stays compact at narrow widths. Rendered ONLY from
 *  the Core's confirmed review payload (the egress decision is the Core's
 *  own gate decision — never a reassuring default); the blocked case
 *  carries the Core's typed reason. */
function PreSendDisclosureLine({ review }: { review: WireReviewPayload }) {
  const t = useT();

  return (
    <p className={styles.presendLine}>
      <strong>{t("run.snapshot.presendLabel")}</strong>{" "}
      <code>{review.provider_model.provider}</code>
      {review.provider_model.model !== undefined && review.provider_model.model !== "" ? (
        <>
          {"/"}
          <code>{review.provider_model.model}</code>
        </>
      ) : null}
      {" — "}
      {review.provider_model.wired ? t("run.review.providerWired") : t("run.review.providerUnwired")}
      {" · "}
      {t("run.snapshot.presendEgressLabel")}{" "}
      {review.policy.egress_allowed
        ? t("run.review.egressAllowed")
        : t("run.review.egressBlocked")}
      {!review.policy.egress_allowed &&
      review.policy.egress_reason !== undefined &&
      review.policy.egress_reason !== "" ? (
        <span className={styles.note}>
          {" "}
          {t("run.review.egressReason", { reason: review.policy.egress_reason })}
        </span>
      ) : null}
    </p>
  );
}

/** The post-submission context-snapshot region (IMPL-16): the ONE
 *  delivered exact snapshot read (IMPL-9's `jobSnapshot`) consumed in the
 *  Run flow — explicit, user-initiated, read-sequence-guarded (the read
 *  itself lives in the page's machine). The structured view is the shared
 *  SnapshotViewPanel (the Job detail's Context Inspector view — ONE
 *  rendering of the exact snapshot's structure, two containers). A Core
 *  NOT_FOUND is the honest not-yet-compiled state; no retry exists
 *  anywhere; the truthful unknown never renders as data. */
function SnapshotPreviewRegion({
  read,
  onRead,
}: {
  read: SnapshotPreviewRead;
  onRead: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  return (
    <div className={styles.snapshotPreview}>
      <h3 className={styles.acceptedTitle}>{t("run.snapshot.preview.heading")}</h3>
      {read.kind === "notLoaded" ? (
        <>
          <p className={styles.note}>{t("run.snapshot.preview.notLoadedIntro")}</p>
          <button type="button" className={styles.secondary} onClick={onRead}>
            {t("run.snapshot.preview.load")}
          </button>
        </>
      ) : read.kind === "loading" ? (
        <p className={styles.note}>{t("run.snapshot.preview.loading")}</p>
      ) : read.kind === "noSnapshot" ? (
        <>
          <p className={styles.note}>{t("run.snapshot.preview.noSnapshot")}</p>
          <button type="button" className={styles.secondary} onClick={onRead}>
            {t("run.snapshot.preview.refresh")}
          </button>
        </>
      ) : read.kind === "failure" ? (
        <>
          <RecoveryPanel
            title={t("run.snapshot.preview.failureTitle")}
            body={t("run.snapshot.preview.failureBody")}
            referenceId={read.envelope.correlationId}
            actions={[{ label: t("run.snapshot.preview.refresh"), onAction: onRead }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{read.envelope.code}</code>
          </p>
        </>
      ) : read.kind === "unknown" ? (
        <RecoveryPanel
          title={t("run.snapshot.preview.unknownTitle")}
          body={t("run.snapshot.preview.unknownBody")}
          actions={[{ label: t("run.snapshot.preview.refresh"), onAction: onRead }]}
        />
      ) : (
        <>
          <SnapshotViewPanel snapshot={read.snapshot} confirmedAt={read.confirmedAt} locale={locale} />
          <p className={styles.note}>{t("run.snapshot.preview.detailPointer")}</p>
        </>
      )}
    </div>
  );
}
