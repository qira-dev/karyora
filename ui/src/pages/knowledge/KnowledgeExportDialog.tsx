import { useCallback, useEffect, useRef, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, KnowledgeOperation, KnowledgePack, KnowledgePackVersion } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import styles from "./KnowledgeLibraryPage.module.css";
import wizardStyles from "./KnowledgeImportWizard.module.css";
import { knowledgeOperationChip, mintExportCorrelationId } from "./knowledgeStates";

/**
 * The export dialog (B1-U1; P4.1 I7, P4.4 R8 — the §24.2 cross-screen
 * export contract applied to the served B1 export family).
 *
 * The laws this component carries, exactly:
 * - **The exact scope and destination are shown before generation**
 *   (R8): the pack, the exact version (the served request's
 *   `version_id` absent = the pack's current version — rendered as that
 *   explicit choice, never assumed), and the user-entered destination
 *   descriptor are all confirmed on the review view before the submit.
 * - **Secrets are excluded by default — a prohibition, not a
 *   preference**: the standing statement renders on the review view
 *   before generation.
 * - **Integrity is stated only where verification was actually
 *   performed**: the completed result's digest renders as the artifact's
 *   own content-identity digest — never a "verified" label (no
 *   verification claim exists on this surface).
 * - **The 202 is never the confirmation** (J-5): the tracking view is
 *   driven by the authoritative state endpoint's reads only.
 * - **No fabricated progress** (R4): the state/step chips only; no
 *   percentage exists anywhere.
 * - **An uncertain export reconciles by its operation ID BEFORE another
 *   copy** (R7/S0–S4): the reconcile query is offered; retry controls
 *   stay disabled until reconciliation resolves the authoritative state.
 * - **No download surface is invented** (B1-A1 §2.6 item 4): the
 *   completed result states the artifact's reference and digest as the
 *   Core-confirmed record and honestly states that the served family
 *   carries no artifact download route.
 */

type SubmitPhase = "idle" | "submitting" | "polling" | "done";

const POLL_INTERVAL_MS = 1000;

const EXPORT_TERMINAL_STATES: readonly string[] = [
  "completed",
  "rejected",
  "failed",
  "cancelled",
  "outcome_unknown",
];

function isLiveExportState(state: string): boolean {
  return !EXPORT_TERMINAL_STATES.includes(state);
}

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export interface KnowledgeExportDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly pack: KnowledgePack;
  readonly versions: readonly KnowledgePackVersion[];
}

export function KnowledgeExportDialog({ open, onClose, pack, versions }: KnowledgeExportDialogProps) {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const labelId = useDialogLabelId();

  // "current" = the request's version_id stays ABSENT (the pack's
  // current version as the Core reports it — the explicit, displayed
  // choice); otherwise the exact Core-owned version identity.
  const [scopeVersion, setScopeVersion] = useState<string>("current");
  const [destination, setDestination] = useState("");
  const [submitPhase, setSubmitPhase] = useState<SubmitPhase>("idle");
  const [submitFailure, setSubmitFailure] = useState<CoreWireEnvelope | null>(null);
  const [submitUnknownCorrelation, setSubmitUnknownCorrelation] = useState<string | null>(null);
  const [operation, setOperation] = useState<KnowledgeOperation | null>(null);
  const [retried, setRetried] = useState(false);
  const pollTimerRef = useRef<number | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current !== null) {
        window.clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (open) {
      setScopeVersion("current");
      setDestination("");
      setSubmitPhase("idle");
      setSubmitFailure(null);
      setSubmitUnknownCorrelation(null);
      setOperation(null);
      setRetried(false);
    }
  }, [open]);

  const pollOperation = useCallback(
    (operationID: string) => {
      void transport.knowledgeOperationStatus(operationID).then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          const op = outcome.data.operation;
          setOperation(op);
          if (EXPORT_TERMINAL_STATES.includes(op.state)) {
            setSubmitPhase("done");
          }
        }
        // A lost/ambiguous poll: the last confirmed state stays rendered
        // and the next tick re-queries — never a timeout-inferred state.
      });
    },
    [transport],
  );

  // The poll loop while the tracked operation is live.
  useEffect(() => {
    if (operation === null || !isLiveExportState(operation.state)) return;
    pollTimerRef.current = window.setTimeout(() => {
      pollTimerRef.current = null;
      pollOperation(operation.operation_id);
    }, POLL_INTERVAL_MS);
    return () => {
      if (pollTimerRef.current !== null) {
        window.clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [operation, pollOperation]);

  const submitExport = () => {
    const correlationId = mintExportCorrelationId();
    setSubmitFailure(null);
    setSubmitUnknownCorrelation(null);
    setSubmitPhase("submitting");
    const payload: {
      correlation_id: string;
      pack_id: string;
      destination: string;
      version_id?: string;
    } = {
      correlation_id: correlationId,
      pack_id: pack.pack_id,
      destination: destination,
    };
    if (scopeVersion !== "current" && scopeVersion !== "") payload.version_id = scopeVersion;
    void transport.knowledgeExportSubmit(payload).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        // 202 = accepted for processing, never completion (J-5).
        setOperation(outcome.data.operation);
        setSubmitPhase("polling");
      } else if (outcome.kind === "rejection") {
        setSubmitFailure(outcome.envelope);
        setSubmitPhase("idle");
      } else {
        // A lost export submit: never proof of success or failure —
        // reconcile by the operation ID before another copy (R7). The
        // correlation ID is the explained state's reference; this
        // dialog has no operation ID yet, so the state offers the
        // correlation-based note and no auto-retry.
        setSubmitUnknownCorrelation(correlationId);
        setSubmitPhase("idle");
      }
    });
  };

  const tracking = submitPhase === "polling" || submitPhase === "done";
  const op = operation;
  const live = op !== null && isLiveExportState(op.state);
  const reviewReady = destination.trim() !== "";

  return (
    <Dialog labelId={labelId} title={t("knowledge.export.title")} open={open} onClose={onClose}>
      {!tracking ? (
        <div>
          <p className={styles.description}>{t("knowledge.export.scopeHeading")}</p>
          <dl className={wizardStyles.reviewFacts}>
            <div>
              <dt>{t("knowledge.export.scopePackLabel")}</dt>
              <dd>
                {pack.title} (<code>{pack.pack_id}</code>)
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.export.scopeVersionLabel")}</dt>
              <dd>
                <select
                  className={wizardStyles.destinationPackSelect}
                  value={scopeVersion}
                  onChange={(event) => setScopeVersion(event.target.value)}
                  aria-label={t("knowledge.export.scopeVersionLabel")}
                >
                  <option value="current">{t("knowledge.export.scopeVersionCurrent")}</option>
                  {versions.map((version) => (
                    <option key={version.version_id} value={version.version_id}>
                      {t("knowledge.export.scopeVersionExact")}: {version.version_id}
                    </option>
                  ))}
                </select>
              </dd>
            </div>
          </dl>
          <label className={wizardStyles.destinationPackLabel} htmlFor="knowledge-export-destination">
            {t("knowledge.export.destinationLabel")}
          </label>
          <input
            id="knowledge-export-destination"
            type="text"
            className={wizardStyles.destinationPackSelect}
            value={destination}
            onChange={(event) => setDestination(event.target.value)}
            placeholder={t("knowledge.export.destinationPlaceholder")}
          />
          <p className={styles.note}>{t("knowledge.export.destinationNote")}</p>
          <section className={wizardStyles.outcomeBlock}>
            <p className={wizardStyles.outcomeTitle}>{t("knowledge.export.secretsHeading")}</p>
            <p>{t("knowledge.export.secretsValue")}</p>
          </section>
          <section className={wizardStyles.outcomeBlock}>
            <p className={wizardStyles.outcomeTitle}>{t("knowledge.export.integrityHeading")}</p>
            <p>{t("knowledge.export.integrityValue")}</p>
          </section>
          {submitFailure !== null ? (
            <div>
              <RecoveryPanel
                title={t("knowledge.export.submitFailureTitle")}
                body={t("knowledge.export.submitFailureBody")}
                referenceId={submitFailure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{submitFailure.code}</code>
              </p>
            </div>
          ) : null}
          {submitUnknownCorrelation !== null ? (
            <RecoveryPanel
              title={t("knowledge.export.submitUnknownTitle")}
              body={t("knowledge.export.submitUnknownBody", { correlationId: submitUnknownCorrelation })}
            />
          ) : null}
          <div className={wizardStyles.actions}>
            <button
              type="button"
              className={styles.primaryControl}
              onClick={submitExport}
              disabled={!reviewReady || submitPhase === "submitting"}
            >
              {submitPhase === "submitting" ? t("knowledge.export.submitting") : t("knowledge.export.confirm")}
            </button>
          </div>
        </div>
      ) : null}

      {tracking && op !== null ? (
        <section className={wizardStyles.trackingPanel}>
          <h3 className={wizardStyles.subHeading}>{t("knowledge.operation.trackingHeading")}</h3>
          <p className={styles.note}>{t("knowledge.operation.acceptedNote")}</p>
          <p className={wizardStyles.operationStateRow}>
            <ExportChip state={op.state} />
            <span className="visually-hidden">{t("knowledge.operation.noPercentageNote")}</span>
          </p>
          <dl className={wizardStyles.reviewFacts}>
            <div>
              <dt>{t("knowledge.operation.operationId")}</dt>
              <dd>
                <code>{op.operation_id}</code>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.operation.correlationId")}</dt>
              <dd>
                <code>{op.correlation_id}</code>
              </dd>
            </div>
            <div>
              <dt>{t("knowledge.operation.lastUpdated")}</dt>
              <dd>
                <time dateTime={op.updated_at}>{formatTimestamp(new Date(op.updated_at), locale)}</time>
              </dd>
            </div>
          </dl>
          {live ? (
            <div className={wizardStyles.actions}>
              <button type="button" className={styles.control} onClick={() => pollOperation(op.operation_id)}>
                {t("knowledge.operation.reconcile")}
              </button>
            </div>
          ) : null}
          {op.state === "completed" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>{t("knowledge.operation.completedExportNote")}</p>
              {op.result !== undefined && op.result !== null ? (
                <dl className={wizardStyles.reviewFacts}>
                  {op.result.export_ref !== undefined && op.result.export_ref !== "" ? (
                    <div>
                      <dt>{t("knowledge.operation.resultExportRef")}</dt>
                      <dd>
                        <code>{op.result.export_ref}</code>
                      </dd>
                    </div>
                  ) : null}
                  {op.result.digest !== undefined && op.result.digest !== "" ? (
                    <div>
                      <dt>{t("knowledge.operation.resultDigest")}</dt>
                      <dd>
                        <code>{op.result.digest}</code>
                      </dd>
                    </div>
                  ) : null}
                </dl>
              ) : null}
              <p className={styles.note}>{t("knowledge.operation.resultNoDownload")}</p>
            </section>
          ) : null}
          {op.state === "rejected" || op.state === "failed" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>
                {op.state === "rejected" ? t("knowledge.states.rejected") : t("knowledge.states.failed")}
              </p>
              {op.failure_reason !== undefined && op.failure_reason !== "" ? (
                <p className={styles.note}>
                  {t("knowledge.operation.failureReason")}: {op.failure_reason}
                </p>
              ) : null}
            </section>
          ) : null}
          {op.state === "cancelled" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>{t("knowledge.states.cancelled")}</p>
              <p>{t("knowledge.operation.cancelledNote")}</p>
            </section>
          ) : null}
          {op.state === "outcome_unknown" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>{t("knowledge.states.outcome_unknown")}</p>
              <p>{t("knowledge.operation.outcomeUnknownNote")}</p>
              <div className={wizardStyles.actions}>
                <button type="button" className={styles.control} onClick={() => pollOperation(op.operation_id)}>
                  {t("knowledge.operation.reconcile")}
                </button>
              </div>
              <p className={styles.note}>{t("knowledge.operation.reconcileNote")}</p>
            </section>
          ) : null}
          {/* The export re-issue control (the read-with-output class is
              safe to re-issue): rendered only after the outcome is
              resolved (R7 — never before reconciliation). */}
          {!live && retried === false && op.state !== "outcome_unknown" ? (
            <div className={wizardStyles.actions}>
              <button
                type="button"
                className={styles.control}
                onClick={() => {
                  setRetried(true);
                  setSubmitPhase("idle");
                  setOperation(null);
                  setSubmitFailure(null);
                  setSubmitUnknownCorrelation(null);
                }}
              >
                {t("knowledge.export.confirm")}
              </button>
            </div>
          ) : null}
        </section>
      ) : null}
    </Dialog>
  );
}

function ExportChip({ state }: { state: string }) {
  const t = useT();
  const chip = knowledgeOperationChip(state);
  const Icon = chip.icon;
  return (
    <span className={`${wizardStyles.chip} ${wizardStyles[chip.tone]}`}>
      <Icon aria-hidden="true" size={14} />
      {t(chip.labelKey)}
    </span>
  );
}
