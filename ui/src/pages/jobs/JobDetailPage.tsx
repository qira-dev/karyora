import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type { WireAttempt, WireEventRecord, WireJob } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./JobDetailPage.module.css";
import {
  CODED_WAITING_REASON_APPROVAL,
  CORE_CANCEL_ELIGIBLE_STATES,
  CopyIdButton,
  jobStateChip,
} from "./jobStates";

/**
 * `/jobs/:jobId` — the canonical Job detail (IMPL-5; the UIR-5 Job
 * detail over the delivered local transport surface).
 *
 * Source-of-truth law (MB §14.2, carried by P3.7 §4): the authoritative
 * state endpoint (`GET /v1/jobs/{id}`) is the contract — the event read
 * (`GET /v1/jobs/{id}/events`) renders the projection and NEVER confirms
 * a command outcome by itself. Every displayed field is a wire field of
 * the coded shapes (the Job/Attempt/Record sets, wire.ts) — nothing is
 * invented, nothing is renamed, missing optional fields render "Not set"
 * (the Not-available discipline). Machine identifiers (state values,
 * waiting reasons, attempt outcomes, digests, codes) render verbatim —
 * they are language-neutral by the localization boundary (P2.4 §4.4).
 *
 * The cancellation command (`POST /v1/jobs/{id}/cancel`, the coded
 * Cancel operation):
 * - The control renders ONLY while the authoritative state is in the
 *   Core's cancel-accepting set (queued/running/waiting — the coded
 *   transition table; P3.7 §6.3: eligibility is the Core's report, never
 *   a client guess). Any other state: no control, with the truthful note.
 * - The request is explicit (dialog, scope stated); duplicate requests
 *   are prevented while one is in flight (C1-10 §2); a 202 is "accepted
 *   for processing, not confirmation" (the 202 law) — the page re-reads
 *   the authoritative state and the state chip shows what the Core
 *   reports (CancelRequested → "Cancelling"; "Cancelled" only from
 *   authoritative state, P3.7 §6.1).
 * - A lost response is `unknown` (S4): never proof of cancellation or of
 *   failure; the page offers a status re-check and never retries on its
 *   own. The typed refusal is surfaced verbatim.
 * - Escape never cancels a durable Job (P2.3 §4.8): the dialog's Escape
 *   (and every navigation path) only closes the surface.
 *
 * The approval `Waiting` display (card 17 as delivered in IMPL-4): a job
 * in `waiting` with the coded `approval` reason renders the P3.7 §5.2
 * approval label and the truthful note that no approval action exists in
 * this surface — there is NO accept/reject affordance anywhere (card
 * 14's approval commands are a later milestone).
 *
 * The result read (`GET /v1/jobs/{id}/result`) is fetched ONLY on
 * explicit request — large outputs are never fetched unbounded
 * (P3.7 §4.1).
 */

type StatusRead =
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly job: WireJob; readonly attempt: WireAttempt; readonly confirmedAt: Date }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

type EventsRead =
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly events: readonly WireEventRecord[] }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

type CancelState =
  | { readonly kind: "idle" }
  | { readonly kind: "pending" }
  | { readonly kind: "accepted" }
  | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

type ResultState =
  | { readonly kind: "notLoaded" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly body: unknown }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(value);
}

/** Renders the Core's own timestamp when it parses; otherwise the
 * Core's own string, unmodified — never a fabricated time. */
function formatWireTimestamp(value: string, locale: string | undefined): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return formatTimestamp(parsed, locale);
}

function renderJsonValue(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}

/** Consumers dedupe by event_id (C1-08; P1.7): first occurrence wins,
 * order preserved as the Core returned it. */
function dedupeByEventId(events: readonly WireEventRecord[]): readonly WireEventRecord[] {
  const seen = new Set<string>();
  const kept: WireEventRecord[] = [];
  for (const event of events) {
    if (seen.has(event.event_id)) continue;
    seen.add(event.event_id);
    kept.push(event);
  }
  return kept;
}

export function JobDetailPage() {
  const { jobId = "" } = useParams<{ jobId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const dialogLabelId = `jobs-cancel-label-${useId()}`;

  const [statusRead, setStatusRead] = useState<StatusRead>({ kind: "loading" });
  const [eventsRead, setEventsRead] = useState<EventsRead>({ kind: "loading" });
  const [resultState, setResultState] = useState<ResultState>({ kind: "notLoaded" });
  const [cancelState, setCancelState] = useState<CancelState>({ kind: "idle" });
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [stateAnnouncement, setStateAnnouncement] = useState("");
  const cancelTriggerRef = useRef<HTMLButtonElement>(null);

  // Read sequencing: only the latest issued read of each kind applies,
  // so a refresh supersedes an in-flight read and nothing applies twice.
  const statusSeqRef = useRef(0);
  const eventsSeqRef = useRef(0);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const readStatus = useCallback(
    (options: { readonly markRefreshing?: boolean } = {}) => {
      if (jobId === "") return;
      const seq = ++statusSeqRef.current;
      setStatusRead((prev) =>
        prev.kind === "data" && options.markRefreshing === true
          ? prev // keep the confirmed view while re-checking (stale-labeled)
          : { kind: "loading" },
      );
      void transport.status(jobId).then((outcome) => {
        if (!mountedRef.current || seq !== statusSeqRef.current) return;
        if (outcome.kind === "data") {
          setStatusRead({
            kind: "data",
            job: outcome.data.job,
            attempt: outcome.data.attempt,
            confirmedAt: new Date(),
          });
        } else if (outcome.kind === "rejection") {
          setStatusRead({ kind: "failure", envelope: outcome.envelope });
        } else {
          setStatusRead({ kind: "unknown" });
        }
      });
    },
    [jobId, transport],
  );

  const readEvents = useCallback(() => {
    if (jobId === "") return;
    const seq = ++eventsSeqRef.current;
    setEventsRead({ kind: "loading" });
    void transport.events(jobId).then((outcome) => {
      if (!mountedRef.current || seq !== eventsSeqRef.current) return;
      if (outcome.kind === "data") {
        setEventsRead({ kind: "data", events: outcome.data.events });
      } else if (outcome.kind === "rejection") {
        setEventsRead({ kind: "failure", envelope: outcome.envelope });
      } else {
        setEventsRead({ kind: "unknown" });
      }
    });
  }, [jobId, transport]);

  useEffect(() => {
    readStatus();
    readEvents();
  }, [readStatus, readEvents]);

  const job = statusRead.kind === "data" ? statusRead.job : null;
  const isApprovalWaiting =
    job !== null &&
    job.state === "waiting" &&
    job.waiting_reason === CODED_WAITING_REASON_APPROVAL;
  const cancelEligible = job !== null && CORE_CANCEL_ELIGIBLE_STATES.includes(job.state);

  // Cancel: one explicit request at a time. A 202 re-reads the
  // authoritative state — the request is never displayed as the
  // confirmation (P3.7 §6.1).
  const requestCancel = useCallback(() => {
    if (jobId === "" || cancelState.kind === "pending") return;
    setCancelState({ kind: "pending" });
    setCancelDialogOpen(false);
    void transport.cancel(jobId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setCancelState({ kind: "accepted" });
        readStatus({ markRefreshing: true });
      } else if (outcome.kind === "rejection") {
        setCancelState({ kind: "refused", envelope: outcome.envelope });
      } else {
        setCancelState({ kind: "unknown" });
      }
    });
  }, [cancelState.kind, jobId, readStatus, transport]);

  const loadResult = useCallback(() => {
    if (jobId === "" || resultState.kind === "loading") return;
    setResultState({ kind: "loading" });
    void transport.result(jobId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setResultState({ kind: "data", body: outcome.data });
      } else if (outcome.kind === "rejection") {
        setResultState({ kind: "failure", envelope: outcome.envelope });
      } else {
        setResultState({ kind: "unknown" });
      }
    });
  }, [jobId, resultState.kind, transport]);

  // Job lifecycle transitions announce politely at meaningful changes
  // only (P2.4 §2.4, the C-C row) — never per refetch.
  const announcedStateRef = useRef<string | null>(null);
  useEffect(() => {
    if (job === null) return;
    if (announcedStateRef.current !== job.state) {
      announcedStateRef.current = job.state;
      setStateAnnouncement(
        `${t("jobs.detail.fields.state")}: ${t(jobStateChip(job.state).labelKey)}`,
      );
    }
  }, [job, t]);

  if (jobId === "") {
    return (
      <PageLayout>
        <PageHeading>{t("jobs.title")}</PageHeading>
      </PageLayout>
    );
  }

  const cancelPending = cancelState.kind === "pending";

  return (
    <PageLayout>
      <p className={styles.backRow}>
        <Link to="/jobs" className={styles.backLink}>
          {t("jobs.detail.back")}
        </Link>
      </p>
      <PageHeading>
        <span className={styles.headingRow}>
          <code className={styles.headingId}>{jobId}</code>
          <CopyIdButton
            id={jobId}
            label={t("jobs.list.copyJobId")}
            copiedLabel={t("jobs.list.copied")}
            failedLabel={t("jobs.list.copyFailed")}
            className={styles.headingCopy}
          />
        </span>
      </PageHeading>

      {statusRead.kind === "loading" ? <p className={styles.note}>{t("jobs.list.loading")}</p> : null}

      {statusRead.kind === "data" ? (
        <>
          <div className={styles.statusRow}>
            <span className={`${styles.chip} ${styles[jobStateChip(statusRead.job.state).tone]}`}>
              {(() => {
                const Icon = jobStateChip(statusRead.job.state).icon;
                return <Icon aria-hidden="true" size={14} />;
              })()}
              {isApprovalWaiting
                ? t("jobs.state.waitingApproval")
                : t(jobStateChip(statusRead.job.state).labelKey)}
            </span>
            <button
              type="button"
              className={styles.control}
              onClick={() => {
                setCancelState({ kind: "idle" });
                readStatus({ markRefreshing: true });
              }}
              disabled={cancelPending}
            >
              {cancelPending ? t("jobs.detail.refreshing") : t("jobs.detail.refresh")}
            </button>
          </div>
          <p className={styles.confirmed}>
            {t("jobs.detail.lastConfirmed", { time: formatTimestamp(statusRead.confirmedAt, locale) })}
          </p>
          {cancelState.kind === "accepted" ? (
            <p className={styles.outcomeNote} role="status">
              {t("jobs.detail.cancel.accepted")}
            </p>
          ) : null}
          {cancelPending ? (
            <p className={styles.outcomeNote} role="status">
              {t("jobs.detail.cancel.pending")}
            </p>
          ) : null}
          {isApprovalWaiting ? (
            <div className={styles.approvalPanel}>
              <p>{t("jobs.detail.approvalWaiting.body")}</p>
            </div>
          ) : null}
          {cancelEligible ? (
            <div className={styles.actionsRow}>
              <button
                type="button"
                ref={cancelTriggerRef}
                className={styles.control}
                onClick={() => setCancelDialogOpen(true)}
                disabled={cancelPending}
              >
                {t("jobs.detail.cancel.button")}
              </button>
            </div>
          ) : (
            <p className={styles.note}>{t("jobs.detail.cancel.notOffered")}</p>
          )}
        </>
      ) : null}

      {statusRead.kind === "failure" ? (
        <>
          <RecoveryPanel
            title={t("jobs.detail.failureTitle")}
            body={t("jobs.detail.failureBody")}
            referenceId={statusRead.envelope.correlationId}
            actions={[{ label: t("jobs.detail.refresh"), onAction: () => readStatus() }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{statusRead.envelope.code}</code>
          </p>
        </>
      ) : null}

      {statusRead.kind === "unknown" ? (
        <RecoveryPanel
          title={t("jobs.detail.unknownTitle")}
          body={t("jobs.detail.unknownBody")}
          actions={[{ label: t("jobs.detail.refresh"), onAction: () => readStatus() }]}
        />
      ) : null}

      {statusRead.kind === "data" && job !== null ? (
        <>
          <PageSection>
            <h2 className={styles.sectionTitle}>{t("jobs.detail.summaryHeading")}</h2>
            <JobField label={t("jobs.detail.fields.jobId")} value={job.job_id} mono />
            <JobField label={t("jobs.detail.fields.idempotencyKey")} value={job.idempotency_key} mono />
            <JobField label={t("jobs.detail.fields.inputDigest")} value={job.input_digest} mono />
            <JobField label={t("jobs.detail.fields.correlationId")} value={job.correlation_id} mono />
            <JobField label={t("jobs.detail.fields.state")} value={job.state} mono />
            {job.state === "waiting" ? (
              <JobField
                label={t("jobs.detail.fields.waitingReason")}
                value={job.waiting_reason}
                mono
              />
            ) : null}
            <JobField label={t("jobs.detail.fields.version")} value={String(job.version)} numeric />
            <JobField
              label={t("jobs.detail.fields.attemptCount")}
              value={String(job.attempt_count)}
              numeric
            />
            <JobField label={t("jobs.detail.fields.provider")} value={job.requirements.provider_id} />
            <JobField
              label={t("jobs.detail.fields.minMemory")}
              value={
                job.requirements.min_memory_mb === undefined
                  ? undefined
                  : t("jobs.detail.fields.minMemoryValue", { value: job.requirements.min_memory_mb })
              }
              numeric
            />
            <JobField
              label={t("jobs.detail.fields.deadline")}
              value={
                job.deadline_secs === undefined
                  ? undefined
                  : t("jobs.detail.fields.deadlineValue", { value: job.deadline_secs })
              }
              numeric
            />
            <JobField label={t("jobs.detail.fields.resultDigest")} value={job.result_digest} mono />
            <JobField label={t("jobs.detail.fields.receiptId")} value={job.receipt_id} mono />
            <JobField
              label={t("jobs.detail.fields.createdAt")}
              value={formatWireTimestamp(job.created_at, locale)}
            />
            <JobField
              label={t("jobs.detail.fields.updatedAt")}
              value={formatWireTimestamp(job.updated_at, locale)}
            />
          </PageSection>

          <PageSection>
            <h2 className={styles.sectionTitle}>{t("jobs.detail.attemptHeading")}</h2>
            <AttemptView attempt={statusRead.attempt} />
          </PageSection>
        </>
      ) : null}

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("jobs.detail.eventsHeading")}</h2>
        {eventsRead.kind === "loading" ? (
          <p className={styles.note}>{t("jobs.detail.events.loading")}</p>
        ) : null}
        {eventsRead.kind === "data" ? (
          dedupeByEventId(eventsRead.events).length === 0 ? (
            <p className={styles.note}>{t("jobs.detail.events.empty")}</p>
          ) : (
            <>
              <p className={styles.note}>{t("jobs.detail.events.projectionNote")}</p>
              <ul className={styles.events}>
                {dedupeByEventId(eventsRead.events).map((event) => (
                  <EventItem key={event.event_id} event={event} />
                ))}
              </ul>
            </>
          )
        ) : null}
        {eventsRead.kind === "failure" ? (
          <>
            <RecoveryPanel
              title={t("jobs.detail.events.failureTitle")}
              body={t("jobs.detail.events.failureBody")}
              referenceId={eventsRead.envelope.correlationId}
              actions={[{ label: t("jobs.detail.refresh"), onAction: readEvents }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{eventsRead.envelope.code}</code>
            </p>
          </>
        ) : null}
        {eventsRead.kind === "unknown" ? (
          <RecoveryPanel
            title={t("jobs.detail.events.unknownTitle")}
            body={t("jobs.detail.events.unknownBody")}
            actions={[{ label: t("jobs.detail.refresh"), onAction: readEvents }]}
          />
        ) : null}
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("jobs.detail.resultHeading")}</h2>
        {resultState.kind === "notLoaded" ? (
          <>
            <p className={styles.note}>{t("jobs.detail.result.notLoaded")}</p>
            <div className={styles.actionsRow}>
              <button type="button" className={styles.control} onClick={loadResult}>
                {t("jobs.detail.result.load")}
              </button>
            </div>
          </>
        ) : null}
        {resultState.kind === "loading" ? (
          <p className={styles.note}>{t("jobs.detail.result.loading")}</p>
        ) : null}
        {resultState.kind === "data" ? (
          resultState.body === null ? (
            <p className={styles.note}>{t("jobs.detail.result.empty")}</p>
          ) : (
            <pre className={styles.payload}>{renderJsonValue(resultState.body)}</pre>
          )
        ) : null}
        {resultState.kind === "failure" ? (
          <>
            <RecoveryPanel
              title={t("jobs.detail.result.failureTitle")}
              body={t("jobs.detail.result.failureBody")}
              referenceId={resultState.envelope.correlationId}
              actions={[{ label: t("jobs.detail.result.load"), onAction: loadResult }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{resultState.envelope.code}</code>
            </p>
          </>
        ) : null}
        {resultState.kind === "unknown" ? (
          <RecoveryPanel
            title={t("jobs.detail.result.unknownTitle")}
            body={t("jobs.detail.result.unknownBody")}
            actions={[{ label: t("jobs.detail.result.load"), onAction: loadResult }]}
          />
        ) : null}
      </PageSection>

      {cancelState.kind === "refused" ? (
        <>
          <RecoveryPanel
            title={t("jobs.detail.cancel.refusedTitle")}
            body={t("jobs.detail.cancel.refusedBody")}
            referenceId={cancelState.envelope.correlationId}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{cancelState.envelope.code}</code>
          </p>
        </>
      ) : null}
      {cancelState.kind === "unknown" ? (
        <RecoveryPanel
          title={t("jobs.detail.cancel.unknownTitle")}
          body={t("jobs.detail.cancel.unknownBody")}
          actions={[
            {
              label: t("jobs.detail.cancel.checkStatus"),
              onAction: () => {
                setCancelState({ kind: "idle" });
                readStatus({ markRefreshing: true });
              },
            },
          ]}
        />
      ) : null}

      <Dialog
        labelId={dialogLabelId}
        title={t("jobs.detail.cancel.dialogTitle")}
        open={cancelDialogOpen}
        onClose={() => setCancelDialogOpen(false)}
        returnFocusRef={cancelTriggerRef}
      >
        <p>{t("jobs.detail.cancel.dialogBody", { jobId })}</p>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.primaryControl}
            onClick={requestCancel}
            disabled={cancelPending}
          >
            {t("jobs.detail.cancel.confirm")}
          </button>
          <button type="button" className={styles.control} onClick={() => setCancelDialogOpen(false)}>
            {t("actions.close")}
          </button>
        </div>
      </Dialog>

      <p role="status" className="visually-hidden">
        {stateAnnouncement}
      </p>
    </PageLayout>
  );
}

function JobField({
  label,
  value,
  mono,
  numeric,
}: {
  label: string;
  value: string | undefined;
  mono?: boolean;
  numeric?: boolean;
}) {
  const t = useT();
  const shown = value === undefined || value === "" ? t("jobs.detail.fields.notSet") : value;
  return (
    <div className={styles.fieldRow}>
      <span className={styles.fieldLabel}>{label}</span>
      <span
        className={`${styles.fieldValue} ${mono === true ? styles.fieldMono : ""} ${
          numeric === true ? styles.fieldNumeric : ""
        }`}
      >
        {shown}
      </span>
    </div>
  );
}

function AttemptView({ attempt }: { attempt: WireAttempt }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  // The Core returns the zero Attempt when no attempt exists (an empty
  // attempt_id): the truthful "no attempt recorded" line — never a
  // fabricated attempt.
  if (attempt.attempt_id === "") {
    return <p className={styles.note}>{t("jobs.detail.attempt.noAttempt")}</p>;
  }

  return (
    <div className={styles.fieldList}>
      <JobField label={t("jobs.detail.attempt.attemptId")} value={attempt.attempt_id} mono />
      <JobField label={t("jobs.detail.attempt.seq")} value={String(attempt.seq)} numeric />
      <JobField label={t("jobs.detail.attempt.worker")} value={attempt.worker_id} mono />
      <JobField label={t("jobs.detail.attempt.lease")} value={attempt.lease_id} mono />
      <JobField label={t("jobs.detail.attempt.leaseEpoch")} value={String(attempt.lease_epoch)} numeric />
      {/* The outcome is the Core's machine identifier, rendered verbatim
          (language-neutral; P2.4 §4.4) — no invented label mapping. */}
      <JobField
        label={t("jobs.detail.attempt.outcome")}
        value={attempt.outcome === "" ? t("jobs.detail.attempt.noOutcome") : attempt.outcome}
        mono
      />
      <JobField
        label={t("jobs.detail.attempt.startedAt")}
        value={formatWireTimestamp(attempt.started_at, locale)}
      />
      <JobField
        label={t("jobs.detail.attempt.endsAt")}
        value={attempt.ends_at === undefined ? undefined : formatWireTimestamp(attempt.ends_at, locale)}
      />
    </div>
  );
}

function EventItem({ event }: { event: WireEventRecord }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  return (
    <li className={styles.eventItem}>
      <div className={styles.eventHead}>
        <code className={styles.eventType}>{event.event_type}</code>
        <span className={styles.eventSeq}>
          <span className="visually-hidden">{t("jobs.detail.events.eventSequence")}: </span>
          {event.sequence}
        </span>
        <time dateTime={event.occurred_at} className={styles.eventTime}>
          {formatWireTimestamp(event.occurred_at, locale)}
        </time>
      </div>
      <div className={styles.eventMeta}>
        <span>
          <span className="visually-hidden">{t("jobs.detail.events.eventId")}: </span>
          <code>{event.event_id}</code>
        </span>
        <span>
          <span className="visually-hidden">{t("jobs.detail.events.eventProducer")}: </span>
          {event.producer}
        </span>
        {event.blob_ref !== undefined ? (
          <span>
            <span className="visually-hidden">{t("jobs.detail.events.blobRef")}: </span>
            <code>{event.blob_ref}</code>
          </span>
        ) : null}
      </div>
      {event.inline_payload === undefined ? (
        <p className={styles.eventNoPayload}>{t("jobs.detail.events.noPayload")}</p>
      ) : (
        <pre className={styles.payload}>{renderJsonValue(event.inline_payload)}</pre>
      )}
    </li>
  );
}
