import { useEffect, useRef, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { ContextInclusionResult, CoreWireEnvelope } from "../../coreclient/httpLocal";
import { useT } from "../../i18n/useT";
import {
  contextFailureCodeKey,
  contextFailureRecoveryKey,
  mintContextCorrelationId,
} from "./contextStates";
import styles from "./ContextInspectorPage.module.css";
import dialogStyles from "./contextDialog.module.css";

/**
 * The per-request memory-inclusion dialog (B3-U1; P4.1 §3.6 / P4.6 §4 —
 * the M-18 request-scoped decision against the served POST
 * /v1/context/inclusion member).
 *
 * The laws this component carries, exactly:
 * - **REQUEST-SCOPED ONLY** (UI §15.5): the decision applies ONLY to the
 *   stated request reference — it changes no memory item's lifecycle
 *   state, creates no Memory action row (the P4.6 §4 boundary), and
 *   never widens any scope (the laws stated in the dialog copy). The
 *   confirmed record renders as a decision record — never an item-state
 *   change.
 * - **The Core mints nothing here that the client claims**: the caller
 *   mints the correlation ID (the replay identity — the reconcile path
 *   for a lost response: the SAME request reference + the SAME
 *   correlation ID re-issued explicitly replays the recorded decision —
 *   the M-18 replay ruling, never a blind repeat).
 * - **The Core-provided-only reason law**: a reason renders ONLY where
 *   the Core reports one — absence is the absence (no reason producer
 *   exists on this surface, so no reason ever renders here); never a
 *   synthesized rationale.
 * - **Only a confirmed 200 carries the outcome truth** (the F11 law);
 *   a lost response is unknown — never proof of success or failure
 *   (the S0–S4 law).
 * - The typed refusals surface verbatim (the D-35 envelope); the decided
 *   item's existence resolves Core-side (an unknown item is the SAME
 *   typed 404 the Memory family's inspect read serves — the no-leak
 *   rule).
 */

type InclusionPhase = "editing" | "submitting" | "done" | "unknown";

interface DecisionRow {
  readonly key: string;
  memoryItemId: string;
  included: boolean;
}

export interface ContextInclusionDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
}

export function ContextInclusionDialog({ open, onClose }: ContextInclusionDialogProps) {
  const t = useT();
  const { transport } = useCoreConnection();
  const labelId = useDialogLabelId();

  const [requestRef, setRequestRef] = useState("");
  const [correlationId, setCorrelationId] = useState("");
  const [rows, setRows] = useState<readonly DecisionRow[]>([{ key: "row-1", memoryItemId: "", included: true }]);
  const [phase, setPhase] = useState<InclusionPhase>("editing");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [confirmed, setConfirmed] = useState<ContextInclusionResult | null>(null);
  const submitTriggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (open) {
      setRequestRef("");
      setCorrelationId(mintContextCorrelationId());
      setRows([{ key: "row-1", memoryItemId: "", included: true }]);
      setPhase("editing");
      setFailure(null);
      setConfirmed(null);
    }
  }, [open]);

  const usableRows = rows.filter((row) => row.memoryItemId.trim() !== "");

  const submit = () => {
    const trimmedRef = requestRef.trim();
    if (trimmedRef === "" || usableRows.length === 0) return;
    setFailure(null);
    setPhase("submitting");
    const mintedCorrelation = correlationId.trim() !== "" ? correlationId.trim() : mintContextCorrelationId();
    const issuedRows = rows;
    void transport
      .contextInclusion({
        correlation_id: mintedCorrelation,
        request_ref: trimmedRef,
        decisions: usableRows.map((row) => ({
          memory_item_id: row.memoryItemId.trim(),
          included: row.included,
        })),
      })
      .then((outcome) => {
        if (outcome.kind === "data") {
          // Only a confirmed 200 carries the outcome truth; the
          // Core-confirmed decision record renders (the reason members
          // render only where the Core reports one — absence is the
          // absence).
          setConfirmed(outcome.data);
          setPhase("done");
        } else if (outcome.kind === "rejection") {
          setFailure(outcome.envelope);
          setPhase("editing");
        } else {
          // The explained unknown: the SAME request reference + the SAME
          // correlation ID re-issued explicitly is the reconcile path
          // (the M-18 replay) — never a blind repeat.
          setPhase("unknown");
          setCorrelationId(mintedCorrelation);
          setRows(issuedRows);
        }
      });
  };

  const reissue = () => {
    // The explicit re-issue of the SAME decision (the same request
    // reference + the same minted correlation ID): the M-18 replay is
    // the reconcile path.
    setPhase("submitting");
    void transport
      .contextInclusion({
        correlation_id: correlationId.trim(),
        request_ref: requestRef.trim(),
        decisions: usableRows.map((row) => ({
          memory_item_id: row.memoryItemId.trim(),
          included: row.included,
        })),
      })
      .then((outcome) => {
        if (outcome.kind === "data") {
          setConfirmed(outcome.data);
          setPhase("done");
        } else if (outcome.kind === "rejection") {
          setFailure(outcome.envelope);
          setPhase("editing");
        } else {
          setPhase("unknown");
        }
      });
  };

  return (
    <Dialog labelId={labelId} title={t("context.inclusion.title")} open={open} onClose={onClose} returnFocusRef={submitTriggerRef}>
      <p className={dialogStyles.description}>{t("context.inclusion.description")}</p>

      {phase === "editing" || phase === "submitting" ? (
        <form
          className={dialogStyles.form}
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <label className={dialogStyles.fieldLabel} htmlFor="context-inclusion-request-ref">
            {t("context.inclusion.requestRefLabel")}
          </label>
          <input
            id="context-inclusion-request-ref"
            className={dialogStyles.fieldInput}
            value={requestRef}
            onChange={(event) => setRequestRef(event.target.value)}
            required
          />
          <p className={dialogStyles.hint}>{t("context.inclusion.requestRefHint")}</p>

          <label className={dialogStyles.fieldLabel} htmlFor="context-inclusion-correlation">
            {t("context.inclusion.correlationLabel")}
          </label>
          <input
            id="context-inclusion-correlation"
            className={dialogStyles.fieldInput}
            value={correlationId}
            onChange={(event) => setCorrelationId(event.target.value)}
            required
          />
          <p className={dialogStyles.hint}>{t("context.inclusion.correlationHint")}</p>

          <fieldset className={dialogStyles.rowsFieldset}>
            <legend className={dialogStyles.fieldLabel}>{t("context.inclusion.itemIdLabel")}</legend>
            {rows.map((row, index) => (
              <div key={row.key} className={dialogStyles.row}>
                <label className="visually-hidden" htmlFor={`context-decision-item-${index}`}>
                  {t("context.inclusion.itemIdLabel")}
                </label>
                <input
                  id={`context-decision-item-${index}`}
                  className={dialogStyles.fieldInput}
                  value={row.memoryItemId}
                  onChange={(event) =>
                    setRows((prev) =>
                      prev.map((r) => (r.key === row.key ? { ...r, memoryItemId: event.target.value } : r)),
                    )
                  }
                />
                <label className="visually-hidden" htmlFor={`context-decision-include-${index}`}>
                  {t("context.inclusion.includedLabel")}
                </label>
                <input
                  id={`context-decision-include-${index}`}
                  type="radio"
                  name={`context-decision-direction-${row.key}`}
                  checked={row.included}
                  onChange={() =>
                    setRows((prev) => prev.map((r) => (r.key === row.key ? { ...r, included: true } : r)))
                  }
                />
                <span className={dialogStyles.radioLabel}>{t("context.inclusion.includedLabel")}</span>
                <label className="visually-hidden" htmlFor={`context-decision-exclude-${index}`}>
                  {t("context.inclusion.excludedLabel")}
                </label>
                <input
                  id={`context-decision-exclude-${index}`}
                  type="radio"
                  name={`context-decision-direction-${row.key}`}
                  checked={!row.included}
                  onChange={() =>
                    setRows((prev) => prev.map((r) => (r.key === row.key ? { ...r, included: false } : r)))
                  }
                />
                <span className={dialogStyles.radioLabel}>{t("context.inclusion.excludedLabel")}</span>
                <button
                  type="button"
                  className={dialogStyles.rowRemove}
                  onClick={() => setRows((prev) => prev.filter((r) => r.key !== row.key))}
                  disabled={rows.length === 1}
                >
                  {t("context.inclusion.removeItem")}
                </button>
              </div>
            ))}
            <button
              type="button"
              className={dialogStyles.rowAdd}
              onClick={() =>
                setRows((prev) => [...prev, { key: `row-${prev.length + 1}-${Date.now()}`, memoryItemId: "", included: true }])
              }
            >
              {t("context.inclusion.addItem")}
            </button>
          </fieldset>

          {failure !== null ? (
            <>
              <RecoveryPanel
                title={t("context.inclusion.failureTitle")}
                body={t("context.inclusion.failureBody")}
                note={t(contextFailureRecoveryKey(failure.code))}
                referenceId={failure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{failure.code}</code>
              </p>
              <p className={styles.note}>{t(contextFailureCodeKey(failure.code))}</p>
            </>
          ) : null}

          <button
            type="submit"
            ref={submitTriggerRef}
            className={dialogStyles.submit}
            disabled={requestRef.trim() === "" || usableRows.length === 0 || phase === "submitting"}
          >
            {phase === "submitting" ? t("context.inclusion.submitting") : t("context.inclusion.submit")}
          </button>
        </form>
      ) : null}

      {phase === "done" && confirmed !== null ? (
        <div className={dialogStyles.confirmed}>
          <p className={dialogStyles.confirmedTitle}>{t("context.inclusion.confirmedTitle")}</p>
          <p className={dialogStyles.confirmedBody}>{t("context.inclusion.confirmedBody")}</p>
          <div className={dialogStyles.fieldList}>
            <div className={dialogStyles.fieldRow}>
              <span className={dialogStyles.fieldLabel}>{t("context.inclusion.requestRefLabel")}</span>
              <span className={dialogStyles.mono}>{confirmed.request_ref}</span>
            </div>
            {confirmed.decisions.map((decision) => (
              <div key={decision.memory_item_id} className={dialogStyles.fieldRow}>
                <span className={dialogStyles.fieldLabel}>
                  <span className={dialogStyles.mono}>{decision.memory_item_id}</span>
                </span>
                <span className={dialogStyles.fieldValue}>
                  {decision.included ? t("context.inclusion.includedLabel") : t("context.inclusion.excludedLabel")}
                  {/* The Core-provided-only reason law: a reason renders
                      ONLY where the Core reports one — absence is the
                      absence (never a synthesized rationale). */}
                  {decision.reason !== undefined && decision.reason !== "" ? (
                    <span className={dialogStyles.reasonLine}>
                      {t("context.inclusion.reasonHeading")}: {decision.reason}
                    </span>
                  ) : (
                    <span className={dialogStyles.reasonLine}>
                      {t("context.inclusion.reasonHeading")}: {t("context.inclusion.reasonAbsent")}
                    </span>
                  )}
                </span>
              </div>
            ))}
          </div>
          <button type="button" className={dialogStyles.submit} onClick={onClose}>
            {t("context.inclusion.done")}
          </button>
        </div>
      ) : null}

      {phase === "unknown" ? (
        <>
          <RecoveryPanel
            title={t("context.inclusion.unknownTitle")}
            body={t("context.inclusion.unknownBody")}
            actions={[{ label: t("context.inclusion.submit"), onAction: reissue }]}
          />
          <button type="button" className={dialogStyles.submit} onClick={onClose}>
            {t("context.inclusion.close")}
          </button>
        </>
      ) : null}
    </Dialog>
  );
}
