import { useEffect, useRef, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, MemoryOperation } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import {
  CODED_MEMORY_SCOPES,
  memoryScopeLabelKey,
  mintMemoryCorrelationId,
} from "./memoryStates";
import styles from "./memoryDialogs.module.css";
import listStyles from "./MemoryLibraryPage.module.css";

/**
 * The export dialog (B2-U1; P4.1 R7 — the M-32 read-with-output class
 * against the served POST /v1/memories/export member).
 *
 * The laws this component carries, exactly:
 * - **The exact export scope is shown before generation, never
 *   assumed** (the contract's own description): the scope (the closed
 *   register's own values) and the optional destination descriptor are
 *   confirmed on the review view before the submit; an item-scoped
 *   export carries the exact item subset and the item's own scope —
 *   the scope never silently broadens (the typed MEMORY_SCOPE_VIOLATION
 *   governs any crossing, surfaced verbatim).
 * - **Secrets are excluded by default — a prohibition, not a
 *   preference** (the binding secrets law extends to exports): the
 *   standing statement renders before generation.
 * - **The honest truncation flag**: `truncated` renders as the Core
 *   reports it — a range exceeding the finite bound truncates honestly,
 *   never silently.
 * - **The served export_ref renders as a content digest — NO download
 *   surface is invented** (B2-A1 §2.6 item 9): the completed result
 *   renders the served operation record's `export_ref` as the content
 *   identity and honestly states the family carries no download route;
 *   the document rode this response body.
 * - **The M-32 class**: mutates nothing, no item-state change,
 *   none-required idempotency (every re-issue mints a new operation);
 *   the coded retry class is safe-retry. A lost response is the
 *   explained unknown — never proof of success or failure — with the
 *   correlation ID reference; the re-issue control renders (the class's
 *   safe re-issue), never an auto-retry.
 */

type ExportPhase = "editing" | "submitting" | "done" | "unknown";

const MEMORY_SCOPES: readonly string[] = CODED_MEMORY_SCOPES;

export interface MemoryExportDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The preset scope (the item's own scope when item-scoped). */
  readonly presetScope?: string;
  /** The item subset when opened from an item (R7's item export). */
  readonly itemIds?: readonly string[];
}

export function MemoryExportDialog({ open, onClose, presetScope, itemIds }: MemoryExportDialogProps) {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const labelId = useDialogLabelId();

  const itemScoped = itemIds !== undefined && itemIds.length > 0;
  const [scope, setScope] = useState<string>(presetScope ?? "user");
  const [destination, setDestination] = useState("");
  const [phase, setPhase] = useState<ExportPhase>("editing");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [result, setResult] = useState<{
    readonly export_operation_id: string;
    readonly count: number;
    readonly generated_at: string;
    readonly truncated: boolean;
  } | null>(null);
  const [operation, setOperation] = useState<MemoryOperation | null>(null);
  const [unknownCorrelation, setUnknownCorrelation] = useState<string | null>(null);
  const submitTriggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (open) {
      setScope(presetScope ?? "user");
      setDestination("");
      setPhase("editing");
      setFailure(null);
      setResult(null);
      setOperation(null);
      setUnknownCorrelation(null);
    }
  }, [open, presetScope]);

  const submit = () => {
    setFailure(null);
    setUnknownCorrelation(null);
    setPhase("submitting");
    const correlationId = mintMemoryCorrelationId("memory-export");
    const body = {
      correlation_id: correlationId,
      scope,
      ...(itemScoped ? { item_ids: [...itemIds] } : {}),
      ...(destination.trim() !== "" ? { destination: destination.trim() } : {}),
    };
    void transport.memoryExport(body).then((outcome) => {
      if (outcome.kind === "data") {
        setResult(outcome.data);
        setPhase("done");
        // The served export_ref rides the operation record (a content
        // digest; B2-A1 §2.6 item 9): the read renders it as served.
        void transport.memoryOperationStatus(outcome.data.export_operation_id).then((opOutcome) => {
          if (opOutcome.kind === "data") setOperation(opOutcome.data.operation);
        });
      } else if (outcome.kind === "rejection") {
        setFailure(outcome.envelope);
        setPhase("editing");
      } else {
        // A lost export response is unknown — never proof of success or
        // failure. The M-32 class mutates nothing and its coded retry
        // class is safe-retry (every re-issue mints a new operation);
        // the re-issue is an explicit user act, never an auto-retry.
        setUnknownCorrelation(correlationId);
        setPhase("unknown");
      }
    });
  };

  function formatTimestamp(value: Date): string {
    return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(value);
  }

  return (
    <Dialog labelId={labelId} title={t("memory.export.title")} open={open} onClose={onClose}>
      {phase === "editing" ? (
        <div>
          {!itemScoped ? (
            <>
              <label className={styles.fieldLabel} htmlFor="memory-export-scope">
                {t("memory.export.scopeLabel")}
              </label>
              <select
                id="memory-export-scope"
                className={styles.fieldSelect}
                value={scope}
                onChange={(event) => setScope(event.target.value)}
              >
                {MEMORY_SCOPES.map((value) => {
                  const labelKey = memoryScopeLabelKey(value);
                  return (
                    <option key={value} value={value}>
                      {labelKey !== null ? t(labelKey) : value}
                    </option>
                  );
                })}
              </select>
            </>
          ) : null}
          {itemScoped ? (
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("memory.export.scopeLabel")}</dt>
                <dd>
                  {(() => {
                    const labelKey = memoryScopeLabelKey(scope);
                    return labelKey !== null ? t(labelKey) : scope;
                  })()}
                </dd>
              </div>
              <div>
                <dt>{t("memory.export.itemsLabel")}</dt>
                <dd>
                  {itemIds?.map((id) => (
                    <code key={id} className={listStyles.code}>
                      {id}
                    </code>
                  ))}
                </dd>
              </div>
            </dl>
          ) : null}
          <p className={styles.fieldNote}>{t("memory.export.scopeNote")}</p>

          <label className={styles.fieldLabel} htmlFor="memory-export-destination">
            {t("memory.export.destinationLabel")}
          </label>
          <input
            id="memory-export-destination"
            type="text"
            className={styles.fieldInput}
            value={destination}
            onChange={(event) => setDestination(event.target.value)}
            placeholder={t("memory.export.destinationPlaceholder")}
          />
          <p className={styles.fieldNote}>{t("memory.export.destinationNote")}</p>

          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("memory.export.secretsHeading")}</p>
            <p>{t("memory.export.secretsValue")}</p>
          </section>

          {failure !== null ? (
            <div>
              <RecoveryPanel
                title={t("memory.export.submitFailureTitle")}
                body={t("memory.export.submitFailureBody")}
                referenceId={failure.correlationId}
              />
              <p className={listStyles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{failure.code}</code>
              </p>
            </div>
          ) : null}

          <div className={styles.actions}>
            <button
              type="button"
              ref={submitTriggerRef}
              className={styles.primaryControl}
              onClick={submit}
            >
              {t("memory.export.submit")}
            </button>
          </div>
        </div>
      ) : null}

      {phase === "submitting" ? (
        <p className={styles.fieldNote} role="status">
          {t("memory.export.submitting")}
        </p>
      ) : null}

      {phase === "done" && result !== null ? (
        <div>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("memory.export.completedTitle")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("memory.export.resultCount")}</dt>
                <dd>{result.count}</dd>
              </div>
              <div>
                <dt>{t("memory.export.resultGeneratedAt")}</dt>
                <dd>
                  <time dateTime={result.generated_at}>{formatTimestamp(new Date(result.generated_at))}</time>
                </dd>
              </div>
              <div>
                <dt>{t("memory.export.resultTruncated")}</dt>
                <dd>{result.truncated ? t("memory.export.truncatedTrue") : t("memory.export.truncatedFalse")}</dd>
              </div>
              <div>
                <dt>{t("memory.export.resultOperationId")}</dt>
                <dd>
                  <code>{result.export_operation_id}</code>
                </dd>
              </div>
            </dl>
            {result.truncated ? <p className={styles.fieldNote}>{t("memory.export.truncatedNote")}</p> : null}
            {operation !== null && operation.export_ref !== undefined && operation.export_ref !== "" ? (
              <dl className={styles.reviewFacts}>
                <div>
                  <dt>{t("memory.export.resultExportRef")}</dt>
                  <dd>
                    <code>{operation.export_ref}</code>
                  </dd>
                </div>
              </dl>
            ) : null}
            <p className={styles.fieldNote}>{t("memory.export.exportRefNote")}</p>
            <p className={styles.fieldNote}>{t("memory.export.noDownloadNote")}</p>
          </section>
          <div className={styles.actions}>
            <button
              type="button"
              className={styles.control}
              onClick={() => {
                setPhase("editing");
                setResult(null);
                setOperation(null);
              }}
            >
              {t("memory.export.reissue")}
            </button>
          </div>
        </div>
      ) : null}

      {phase === "unknown" && unknownCorrelation !== null ? (
        <div>
          <RecoveryPanel
            title={t("memory.export.unknownTitle")}
            body={t("memory.export.unknownBody", { correlationId: unknownCorrelation })}
          />
          <p className={styles.fieldNote}>{t("memory.export.unknownReissueNote")}</p>
          <div className={styles.actions}>
            <button type="button" className={styles.primaryControl} onClick={submit}>
              {t("memory.export.reissue")}
            </button>
          </div>
        </div>
      ) : null}

      <div className={styles.actions}>
        <button
          type="button"
          className={styles.control}
          onClick={() => {
            onClose();
            submitTriggerRef.current?.focus();
          }}
        >
          {t("actions.close")}
        </button>
      </div>
    </Dialog>
  );
}
