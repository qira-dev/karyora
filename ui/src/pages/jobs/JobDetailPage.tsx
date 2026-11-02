import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope } from "../../coreclient/httpLocal";
import type {
  WireApproval,
  WireAttempt,
  WireEventRecord,
  WireJob,
  WireSnapshotItem,
  WireSnapshotOmission,
  WireSnapshotView,
} from "../../coreclient/httpLocal";
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
 * The approval `Waiting` display and its decision commands (card 14's
 * working candidate as delivered in IMPL-7 over the card-17 display of
 * IMPL-4/IMPL-5): a job in `waiting` with the coded `approval` reason
 * renders the P3.7 §5.2 approval label, the approval-request inspection
 * (the Core-issued identity — `GET /v1/jobs/{id}/approval`), and the
 * accept/reject affordances (`POST /v1/jobs/{id}/approvals/accept|reject`,
 * the coded decision commands). The inspection precedes the decision: no
 * affordance renders without the Core's approval record. Each decision is
 * explicit (dialog, scope stated — the job and the approval identity being
 * decided) and follows the cancel command's four-outcome discipline:
 * - The Core-confirmed result is NEVER rendered from the response alone —
 *   the page immediately re-reads the authoritative state, and the state
 *   chip shows what the Core reports (accepted → re-queued; rejected → the
 *   terminal disposition).
 * - The typed refusal is surfaced verbatim; the honest `replayed` flag is
 *   rendered as the Core's own replay report.
 * - A lost response is `unknown` (S4): never proof the decision was
 *   recorded or not, never labeled decided, never auto-retried — the
 *   recovery is the explicit status re-check (the Open-class
 *   reconciliation: a QUERY by the approval identity, never a blind
 *   re-decision).
 * - A notification read/dismiss never implies approval (requirement (f)):
 *   viewing this job, its events, or this page records no decision — the
 *   copy states it where the decision is made.
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

type ApprovalRead =
  | { readonly kind: "idle" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly approval: WireApproval }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

type ApprovalDecisionState =
  | { readonly kind: "idle" }
  | { readonly kind: "pending" }
  | { readonly kind: "recorded"; readonly replayed: boolean }
  | { readonly kind: "refused"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

type ApprovalDialog = "accept" | "reject" | null;

type ResultState =
  | { readonly kind: "notLoaded" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly body: unknown }
  | { readonly kind: "failure"; readonly envelope: CoreWireEnvelope }
  | { readonly kind: "unknown" };

/** The context-snapshot read (the card-11 working candidate's Inspector
 *  read): loading / the structured view / the typed refusal / the
 *  truthful unknown. A Core NOT_FOUND is its own honest state (no
 *  snapshot stored for the attempt), distinct from other refusals. */
type SnapshotRead =
  | { readonly kind: "notLoaded" }
  | { readonly kind: "loading" }
  | { readonly kind: "data"; readonly snapshot: WireSnapshotView; readonly confirmedAt: Date }
  | { readonly kind: "noSnapshot" }
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
  const approvalAcceptLabelId = `jobs-approval-accept-label-${useId()}`;
  const approvalRejectLabelId = `jobs-approval-reject-label-${useId()}`;

  const [statusRead, setStatusRead] = useState<StatusRead>({ kind: "loading" });
  const [eventsRead, setEventsRead] = useState<EventsRead>({ kind: "loading" });
  const [resultState, setResultState] = useState<ResultState>({ kind: "notLoaded" });
  const [snapshotRead, setSnapshotRead] = useState<SnapshotRead>({ kind: "notLoaded" });
  const [cancelState, setCancelState] = useState<CancelState>({ kind: "idle" });
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [approvalRead, setApprovalRead] = useState<ApprovalRead>({ kind: "idle" });
  const [decisionState, setDecisionState] = useState<ApprovalDecisionState>({ kind: "idle" });
  const [approvalDialog, setApprovalDialog] = useState<ApprovalDialog>(null);
  const [stateAnnouncement, setStateAnnouncement] = useState("");
  const cancelTriggerRef = useRef<HTMLButtonElement>(null);
  const approvalTriggerRef = useRef<HTMLButtonElement>(null);

  // Read sequencing: only the latest issued read of each kind applies,
  // so a refresh supersedes an in-flight read and nothing applies twice.
  const statusSeqRef = useRef(0);
  const eventsSeqRef = useRef(0);
  const approvalSeqRef = useRef(0);
  const snapshotSeqRef = useRef(0);
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

  // The approval-request inspection (the queryable Core-issued identity):
  // issued ONLY while the authoritative status read reports the approval
  // wait; the inspection precedes the decision (P3.6 §6.1), so no
  // affordance renders without it. The read mutates nothing — a read never
  // implies approval.
  const readApproval = useCallback(() => {
    if (jobId === "") return;
    const seq = ++approvalSeqRef.current;
    setApprovalRead({ kind: "loading" });
    void transport.jobApproval(jobId).then((outcome) => {
      if (!mountedRef.current || seq !== approvalSeqRef.current) return;
      if (outcome.kind === "data") {
        setApprovalRead({ kind: "data", approval: outcome.data.approval });
      } else if (outcome.kind === "rejection") {
        setApprovalRead({ kind: "failure", envelope: outcome.envelope });
      } else {
        setApprovalRead({ kind: "unknown" });
      }
    });
  }, [jobId, transport]);

  const approvalWaitingNow =
    statusRead.kind === "data" &&
    statusRead.job.state === "waiting" &&
    statusRead.job.waiting_reason === CODED_WAITING_REASON_APPROVAL;

  // Rising edge: a fresh approval wait — reset any prior decision outcome
  // and read the raised identity. Falling edge: the wait resolved (the
  // Core-confirmed transition re-read from the authoritative status) — the
  // panel goes away while the decision outcome note persists above.
  useEffect(() => {
    if (approvalWaitingNow) {
      setDecisionState({ kind: "idle" });
      readApproval();
    } else {
      setApprovalRead({ kind: "idle" });
    }
  }, [approvalWaitingNow, readApproval]);

  // The approval decision command: one explicit decision at a time; the
  // Core-confirmed result is never rendered from the response alone — the
  // page immediately re-reads the authoritative status and the raised
  // approval. A lost response is `unknown` (S4): never labeled decided,
  // never retried on its own — the reconciliation is the explicit status
  // re-check (a QUERY by the approval identity, never a blind
  // re-decision).
  const decideApproval = useCallback(
    (decision: "accept" | "reject") => {
      if (
        jobId === "" ||
        approvalRead.kind !== "data" ||
        decisionState.kind === "pending"
      ) {
        return;
      }
      setDecisionState({ kind: "pending" });
      setApprovalDialog(null);
      void transport
        .decideApproval(jobId, decision, approvalRead.approval.approval_id)
        .then((outcome) => {
          if (!mountedRef.current) return;
          if (outcome.kind === "data") {
            setDecisionState({ kind: "recorded", replayed: outcome.data.replayed });
            readStatus({ markRefreshing: true });
            readApproval();
          } else if (outcome.kind === "rejection") {
            setDecisionState({ kind: "refused", envelope: outcome.envelope });
          } else {
            setDecisionState({ kind: "unknown" });
          }
        });
    },
    [approvalRead, decisionState.kind, jobId, readApproval, readStatus, transport],
  );

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

  // The context-snapshot read (the card-11 working candidate's
  // Inspector read): an explicit, user-initiated QUERY — server-side,
  // read-sequence-guarded, and mutating nothing. The Core's NOT_FOUND is
  // its own honest state (no snapshot stored for this attempt), never an
  // empty structure and never an error panel.
  const readSnapshot = useCallback(() => {
    if (jobId === "" || snapshotRead.kind === "loading") return;
    const seq = ++snapshotSeqRef.current;
    setSnapshotRead({ kind: "loading" });
    void transport.jobSnapshot(jobId).then((outcome) => {
      if (!mountedRef.current || seq !== snapshotSeqRef.current) return;
      if (outcome.kind === "data") {
        setSnapshotRead({ kind: "data", snapshot: outcome.data.snapshot, confirmedAt: new Date() });
      } else if (outcome.kind === "rejection") {
        setSnapshotRead(
          outcome.envelope.code === "NOT_FOUND"
            ? { kind: "noSnapshot" }
            : { kind: "failure", envelope: outcome.envelope },
        );
      } else {
        setSnapshotRead({ kind: "unknown" });
      }
    });
  }, [jobId, snapshotRead.kind, transport]);

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
                setDecisionState({ kind: "idle" });
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
          {decisionState.kind === "recorded" ? (
            <p className={styles.outcomeNote} role="status">
              {t("jobs.detail.approval.recorded")}
              {decisionState.replayed ? ` ${t("jobs.detail.approval.replayed")}` : ""}
            </p>
          ) : null}
          {decisionState.kind === "pending" ? (
            <p className={styles.outcomeNote} role="status">
              {t("jobs.detail.approval.pending")}
            </p>
          ) : null}
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
              <h2 className={styles.approvalHeading}>{t("jobs.detail.approval.heading")}</h2>
              <p>{t("jobs.detail.approval.body")}</p>
              {approvalRead.kind === "loading" ? (
                <p className={styles.approvalMeta}>{t("jobs.list.loading")}</p>
              ) : null}
              {approvalRead.kind === "failure" ? (
                approvalRead.envelope.code === "NOT_FOUND" ? (
                  <p className={styles.approvalMeta}>{t("jobs.detail.approval.noApprovalRecord")}</p>
                ) : (
                  <>
                    <RecoveryPanel
                      title={t("jobs.detail.approval.readFailureTitle")}
                      body={t("jobs.detail.approval.readFailureBody")}
                      referenceId={approvalRead.envelope.correlationId}
                      actions={[
                        { label: t("jobs.detail.refresh"), onAction: readApproval },
                      ]}
                    />
                    <p className={styles.code}>
                      <span className="visually-hidden">Code: </span>
                      <code>{approvalRead.envelope.code}</code>
                    </p>
                  </>
                )
              ) : null}
              {approvalRead.kind === "unknown" ? (
                <RecoveryPanel
                  title={t("jobs.detail.approval.readUnknownTitle")}
                  body={t("jobs.detail.approval.readUnknownBody")}
                  actions={[{ label: t("jobs.detail.refresh"), onAction: readApproval }]}
                />
              ) : null}
              {approvalRead.kind === "data" ? (
                <>
                  <div className={styles.approvalMeta}>
                    <span className={styles.fieldLabel}>{t("jobs.detail.approval.approvalId")}</span>
                    <CopyIdButton
                      id={approvalRead.approval.approval_id}
                      label={t("jobs.detail.approval.copyApprovalId")}
                      copiedLabel={t("jobs.list.copied")}
                      failedLabel={t("jobs.list.copyFailed")}
                      className={styles.approvalCopy}
                    />
                  </div>
                  <p className={styles.approvalMeta}>
                    {t("jobs.detail.approval.requestedAt", {
                      time: formatWireTimestamp(approvalRead.approval.created_at, locale),
                    })}
                  </p>
                  <p className={styles.approvalMeta}>
                    {t("jobs.detail.approval.scope", { jobId })}
                  </p>
                  {approvalRead.approval.state === "pending" ? (
                    <div className={styles.actionsRow}>
                      <button
                        type="button"
                        ref={approvalTriggerRef}
                        className={styles.primaryControl}
                        onClick={() => setApprovalDialog("accept")}
                        disabled={decisionState.kind === "pending"}
                      >
                        {t("jobs.detail.approval.acceptButton")}
                      </button>
                      <button
                        type="button"
                        className={styles.control}
                        onClick={() => setApprovalDialog("reject")}
                        disabled={decisionState.kind === "pending"}
                      >
                        {t("jobs.detail.approval.rejectButton")}
                      </button>
                    </div>
                  ) : (
                    <p className={styles.approvalMeta}>
                      {t("jobs.detail.approval.alreadyDecided", {
                        decision: approvalRead.approval.decision ?? "",
                        state: approvalRead.approval.state,
                      })}
                    </p>
                  )}
                </>
              ) : null}
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

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("jobs.detail.context.heading")}</h2>
        <p className={styles.note}>{t("jobs.detail.context.description")}</p>
        {snapshotRead.kind === "notLoaded" ? (
          <div className={styles.actionsRow}>
            <button type="button" className={styles.control} onClick={readSnapshot}>
              {t("jobs.detail.context.load")}
            </button>
          </div>
        ) : null}
        {snapshotRead.kind === "notLoaded" ? (
          <p className={styles.note}>{t("jobs.detail.context.notLoaded")}</p>
        ) : null}
        {snapshotRead.kind === "loading" ? (
          <p className={styles.note}>{t("jobs.detail.context.loading")}</p>
        ) : null}
        {snapshotRead.kind === "noSnapshot" ? (
          <>
            <p className={styles.note}>{t("jobs.detail.context.noSnapshot")}</p>
            <div className={styles.actionsRow}>
              <button type="button" className={styles.control} onClick={readSnapshot}>
                {t("jobs.detail.context.refresh")}
              </button>
            </div>
          </>
        ) : null}
        {snapshotRead.kind === "failure" ? (
          <>
            <RecoveryPanel
              title={t("jobs.detail.context.failureTitle")}
              body={t("jobs.detail.context.failureBody")}
              referenceId={snapshotRead.envelope.correlationId}
              actions={[{ label: t("jobs.detail.context.refresh"), onAction: readSnapshot }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{snapshotRead.envelope.code}</code>
            </p>
          </>
        ) : null}
        {snapshotRead.kind === "unknown" ? (
          <RecoveryPanel
            title={t("jobs.detail.context.unknownTitle")}
            body={t("jobs.detail.context.unknownBody")}
            actions={[{ label: t("jobs.detail.context.refresh"), onAction: readSnapshot }]}
          />
        ) : null}
        {snapshotRead.kind === "data" ? (
          <SnapshotViewPanel snapshot={snapshotRead.snapshot} confirmedAt={snapshotRead.confirmedAt} locale={locale} />
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

      {decisionState.kind === "refused" ? (
        <>
          <RecoveryPanel
            title={t("jobs.detail.approval.refusedTitle")}
            body={t("jobs.detail.approval.refusedBody")}
            referenceId={decisionState.envelope.correlationId}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{decisionState.envelope.code}</code>
          </p>
        </>
      ) : null}
      {decisionState.kind === "unknown" ? (
        <RecoveryPanel
          title={t("jobs.detail.approval.unknownTitle")}
          body={t("jobs.detail.approval.unknownBody")}
          actions={[
            {
              label: t("jobs.detail.cancel.checkStatus"),
              onAction: () => {
                setDecisionState({ kind: "idle" });
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

      <Dialog
        labelId={approvalAcceptLabelId}
        title={t("jobs.detail.approval.acceptDialogTitle")}
        open={approvalDialog === "accept"}
        onClose={() => setApprovalDialog(null)}
        returnFocusRef={approvalTriggerRef}
      >
        <p>
          {t("jobs.detail.approval.acceptDialogBody", {
            approvalId: approvalRead.kind === "data" ? approvalRead.approval.approval_id : "",
            jobId,
          })}
        </p>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.primaryControl}
            onClick={() => decideApproval("accept")}
            disabled={decisionState.kind === "pending"}
          >
            {t("jobs.detail.approval.confirmAccept")}
          </button>
          <button type="button" className={styles.control} onClick={() => setApprovalDialog(null)}>
            {t("actions.close")}
          </button>
        </div>
      </Dialog>

      <Dialog
        labelId={approvalRejectLabelId}
        title={t("jobs.detail.approval.rejectDialogTitle")}
        open={approvalDialog === "reject"}
        onClose={() => setApprovalDialog(null)}
        returnFocusRef={approvalTriggerRef}
      >
        <p>
          {t("jobs.detail.approval.rejectDialogBody", {
            approvalId: approvalRead.kind === "data" ? approvalRead.approval.approval_id : "",
            jobId,
          })}
        </p>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.primaryControl}
            onClick={() => decideApproval("reject")}
            disabled={decisionState.kind === "pending"}
          >
            {t("jobs.detail.approval.confirmReject")}
          </button>
          <button type="button" className={styles.control} onClick={() => setApprovalDialog(null)}>
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

/** The Context Inspector's structured snapshot view (the card-11 working
 *  candidate; P3.2 §3.5 + P3.5 §6): the included/omitted lists with their
 *  Core-supplied reasons, the budget pair, and the exact-vs-approximate
 *  label — rendered EXACTLY as the Core returns them. Machine identifiers
 *  (refs, digests, trust labels, omission reasons, estimator/renderer
 *  versions, the exact_vs_approximate value) render verbatim as the
 *  language-neutral identifiers they are. Missing facts render their
 *  honest Not-set/Not-reported state — never zero, never invented
 *  coverage; the compiled content itself is never part of this shape. */
function SnapshotViewPanel({
  snapshot,
  confirmedAt,
  locale,
}: {
  snapshot: WireSnapshotView;
  confirmedAt: Date;
  locale: string | undefined;
}) {
  const t = useT();
  const coreProduced = snapshot.exact_vs_approximate === "core_produced";

  return (
    <div className={styles.snapshotPanel}>
      <p className={styles.confirmed}>
        {t("jobs.detail.lastConfirmed", { time: formatTimestamp(confirmedAt, locale) })}
      </p>

      <h3 className={styles.snapshotSubheading}>{t("jobs.detail.context.identityHeading")}</h3>
      <div className={styles.fieldList}>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.fields.snapshotId")}</span>
          <span className={`${styles.fieldValue} ${styles.fieldMono}`}>
            <code>{snapshot.snapshot_id}</code>
          </span>
        </div>
        <JobField label={t("jobs.detail.context.fields.taskRef")} value={snapshot.task_ref} mono />
        <JobField label={t("jobs.detail.context.fields.scope")} value={snapshot.scope} mono />
        <JobField
          label={t("jobs.detail.context.fields.schemaVersion")}
          value={String(snapshot.schema_version)}
          numeric
        />
        <JobField
          label={t("jobs.detail.context.fields.createdAt")}
          value={formatWireTimestamp(snapshot.created_at, locale)}
        />
        <JobField
          label={t("jobs.detail.context.fields.assembledDigest")}
          value={snapshot.assembled_digest}
          mono
        />
        <JobField
          label={t("jobs.detail.context.fields.userInputDigest")}
          value={snapshot.user_input_digest}
          mono
        />
        <JobField
          label={t("jobs.detail.context.fields.rendererVersion")}
          value={snapshot.renderer_version}
          mono
        />
        <JobField
          label={t("jobs.detail.context.fields.compressionMethod")}
          value={snapshot.compression_method}
          mono
        />
        <JobField
          label={t("jobs.detail.context.fields.retrievalConfigVersion")}
          value={snapshot.retrieval_config_version}
          mono
        />
      </div>

      <h3 className={styles.snapshotSubheading}>{t("jobs.detail.context.exactnessHeading")}</h3>
      <div className={styles.fieldList}>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.exactnessHeading")}</span>
          <span className={`${styles.fieldValue} ${styles.fieldMono}`}>
            {coreProduced ? t("jobs.detail.context.exactCoreProduced") : snapshot.exact_vs_approximate}
          </span>
        </div>
        {coreProduced ? <p className={styles.note}>{t("jobs.detail.context.exactCoreProducedNote")}</p> : null}
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.estimateLabel")}</span>
          <span className={`${styles.fieldValue} ${styles.fieldNumeric}`}>{snapshot.budget.estimated_input_tokens}</span>
        </div>
        <p className={styles.note}>
          {t("jobs.detail.context.estimateNote", { estimator: snapshot.budget.estimator_version })}
        </p>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.providerActualLabel")}</span>
          <span className={`${styles.fieldValue} ${styles.fieldMono}`}>
            {t("jobs.detail.context.providerActualMissing")}
          </span>
        </div>
        <p className={styles.note}>{t("jobs.detail.context.providerActualNote")}</p>
      </div>

      <h3 className={styles.snapshotSubheading}>{t("jobs.detail.context.budgetHeading")}</h3>
      <div className={styles.fieldList}>
        <p className={styles.snapshotRequestedLabel}>
          {t("jobs.detail.context.budgetRequestedHeading")}
        </p>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.budgetMaxInput")}</span>
          <span className={styles.fieldValue}>
            {snapshot.budget.requested.max_input_tokens === undefined
              ? t("jobs.detail.context.budgetNotReported")
              : snapshot.budget.requested.max_input_tokens}
          </span>
        </div>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.budgetReservedOutput")}</span>
          <span className={styles.fieldValue}>
            {snapshot.budget.requested.reserved_output_tokens === undefined
              ? t("jobs.detail.context.budgetNotReported")
              : snapshot.budget.requested.reserved_output_tokens}
          </span>
        </div>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("jobs.detail.context.budgetMaxItems")}</span>
          <span className={styles.fieldValue}>
            {snapshot.budget.requested.max_items === undefined
              ? t("jobs.detail.context.budgetNotReported")
              : snapshot.budget.requested.max_items}
          </span>
        </div>
        <p className={styles.note}>{t("jobs.detail.context.budgetNote")}</p>
      </div>

      <h3 className={styles.snapshotSubheading}>{t("jobs.detail.context.includedHeading")}</h3>
      {snapshot.included.length === 0 ? (
        <p className={styles.note}>{t("jobs.detail.context.includedEmpty")}</p>
      ) : (
        <ul className={styles.snapshotList}>
          {snapshot.included.map((item: WireSnapshotItem) => (
            <li key={item.ref} className={styles.snapshotItem}>
              <div className={styles.snapshotItemHead}>
                <code className={styles.snapshotRef}>{item.ref}</code>
                <span className={styles.snapshotBadge}>
                  <span className="visually-hidden">
                    {t("jobs.detail.context.includedTrustLabel")}:{" "}
                  </span>
                  {item.trust_label}
                </span>
              </div>
              <div className={styles.snapshotItemMeta}>
                <code>{item.digest}</code>
              </div>
              <div className={styles.snapshotItemMeta}>
                <span className={styles.fieldLabel}>{t("jobs.detail.context.includedReasonLabel")}: </span>
                <code>{item.reason}</code>
              </div>
            </li>
          ))}
        </ul>
      )}

      <h3 className={styles.snapshotSubheading}>{t("jobs.detail.context.omittedHeading")}</h3>
      {snapshot.omitted.length === 0 ? (
        <p className={styles.note}>{t("jobs.detail.context.omittedEmpty")}</p>
      ) : (
        <ul className={styles.snapshotList}>
          {snapshot.omitted.map((omission: WireSnapshotOmission) => (
            <li key={omission.ref} className={styles.snapshotItem}>
              <div className={styles.snapshotItemHead}>
                <code className={styles.snapshotRef}>{omission.ref}</code>
                <span className={`${styles.snapshotBadge} ${styles.snapshotBadgeOmitted}`}>
                  {omission.reason}
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}
      <p className={styles.note}>{t("jobs.detail.context.omittedNote")}</p>
    </div>
  );
}
