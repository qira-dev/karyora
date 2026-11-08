import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  KnowledgeImportSourceSpec,
  KnowledgeOperation,
  KnowledgePack,
} from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import styles from "./KnowledgeLibraryPage.module.css";
import wizardStyles from "./KnowledgeImportWizard.module.css";
import {
  knowledgeFailureCodeKey,
  knowledgeFailureRecoveryKey,
  knowledgeOperationChip,
  mintIdempotencyKey,
} from "./knowledgeStates";

/**
 * The W-2 import wizard (B1-U1; P4.1 I1–I6, P4.4 DF-1/R1–R7 applied to
 * the served B1 import family).
 *
 * The laws this component carries, exactly:
 * - **Source types are the served registry EXACTLY** (I1; UI §14.6): the
 *   kinds, size bounds, and notes render only as
 *   `GET /v1/knowledgepacks/import/registry` serves them. No URL kind,
 *   no connector, no speculative format is ever displayed — the served
 *   registry carries none (B1-A1 §2.4). A registry read that fails or
 *   cannot be confirmed shows the unavailable state with the accurate
 *   reason; no format is guessed.
 * - **Client checks are advisory only** (DF-1/T3): the locally read file
 *   size against the served bound renders as a clearly-labeled ADVISORY
 *   early check — never a validation result, never authoritative. No
 *   client MIME/extension filter is applied (the served registry carries
 *   no extension/MIME mapping and none is invented).
 * - **Pre-submit queue removal is a local act** (I3/R1): "Remove from
 *   queue" has NO Core meaning, creates no event, and needs no
 *   confirmation; focus moves to the next queued item's control.
 * - **Pre-submit review shows the facts before submission** (I2): the
 *   destination, the Core-declared limit, the source origin, the
 *   permission note, and the boundary fact — on this local surface no
 *   provider/remote-Core crossing exists, and the review says so
 *   honestly (never a fabricated crossing warning, never a hidden
 *   boundary). No submission happens before this step is completed.
 * - **The 202 is never the confirmation** (J-5; I4): the submit response
 *   states accepted-for-processing; the authoritative state endpoint
 *   confirms every outcome. `replayed` renders as the Core's honest
 *   replay flag (the M-9 same-digest class).
 * - **Progress is the Core's state/step — never a percentage** (R4): the
 *   state register renders as step chips from the state endpoint's real
 *   reads; no percentage exists anywhere in this component.
 * - **Cancellation is Core-confirmed only** (I5/R2): the cancel request
 *   (202) is never displayed as the confirmation — the terminal
 *   `cancelled` state is; a refused cancel surfaces the typed envelope.
 * - **Distinct rejections** (R6): the Core's failure_code + reason
 *   render as the code's own distinct state with its own recovery; the
 *   same-key-different-bytes conflict is its own distinct state.
 * - **Unknown outcomes reconcile by the operation ID** (R7/S0–S4): a
 *   lost poll, a lost submit, or an `outcome_unknown` state shows the
 *   explained state with the reconcile query — never a blind retry,
 *   never a timeout-inferred outcome. A same-key same-bytes
 *   re-submission is the M-9-safe path and is offered explicitly.
 */

type RegistryPhase = "loading" | "data" | "failure" | "unknown";
type PackListPhase = "idle" | "loading" | "data" | "failure" | "unknown";
type WizardStep = "source" | "queue" | "review" | "tracking";
type SubmitPhase = "idle" | "submitting" | "polling";

interface QueueItem {
  /** The caller-minted M-9 idempotency key (stable for the item's
   *  lifetime; the same key + same bytes reconciles a re-submission). */
  readonly idempotencyKey: string;
  readonly file: File;
  readonly kind: string;
  readonly destinationMode: "new" | "existing";
  readonly destinationPackId: string;
}

interface SubmitOutcomeRow {
  /** The item's stable key (unique per queued item). */
  readonly itemKey: string;
  readonly fileName: string;
  readonly operation: KnowledgeOperation;
}

const POLL_INTERVAL_MS = 1000;

const KNOWLEDGE_TERMINAL_STATES: readonly string[] = [
  "completed",
  "rejected",
  "failed",
  "cancelled",
  "outcome_unknown",
];

function formatBytes(bytes: number, locale: string | undefined): string {
  return new Intl.NumberFormat(locale, { style: "unit", unit: "byte", unitDisplay: "long" }).format(bytes);
}

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

/** The submitted bytes as standard padded base64 (the JSON envelope
 *  transport's typing — the F4 attachment class). Read in one pass; the
 *  served kinds bound the size well under browser limits. */
function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const bytes = new Uint8Array(reader.result as ArrayBuffer);
      let binary = "";
      const chunk = 0x8000;
      for (let offset = 0; offset < bytes.length; offset += chunk) {
        binary += String.fromCharCode(...bytes.subarray(offset, offset + chunk));
      }
      resolve(btoa(binary));
    };
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.readAsArrayBuffer(file);
  });
}

export interface KnowledgeImportWizardProps {
  readonly open: boolean;
  readonly onClose: () => void;
  /** The pack-detail entry (I1's pack-detail pointer): new files queue
   *  with this pack preselected as the destination — the version-create
   *  identity operation on the named pack. The user can still change the
   *  destination per item; the preset is a convenience, never a lock. */
  readonly presetDestinationPackId?: string;
}

export function KnowledgeImportWizard({
  open,
  onClose,
  presetDestinationPackId,
}: KnowledgeImportWizardProps) {
  const t = useT();
  const { transport } = useCoreConnection();
  const labelId = useDialogLabelId();

  const [step, setStep] = useState<WizardStep>("source");
  const [registryPhase, setRegistryPhase] = useState<RegistryPhase>("loading");
  const [sources, setSources] = useState<readonly KnowledgeImportSourceSpec[]>([]);
  const [registryFailure, setRegistryFailure] = useState<CoreWireEnvelope | null>(null);
  const [selectedKind, setSelectedKind] = useState<string>("");

  const [queue, setQueue] = useState<readonly QueueItem[]>([]);
  const [packs, setPacks] = useState<readonly KnowledgePack[]>([]);
  const [packsCursor, setPacksCursor] = useState<string | null>(null);
  const [packListPhase, setPackListPhase] = useState<PackListPhase>("idle");
  const [packListFailure, setPackListFailure] = useState<CoreWireEnvelope | null>(null);

  const [submitPhase, setSubmitPhase] = useState<SubmitPhase>("idle");
  const [currentItemKey, setCurrentItemKey] = useState<string | null>(null);
  const [currentOperation, setCurrentOperation] = useState<KnowledgeOperation | null>(null);
  /** The submit response's honest `replayed` flag (the M-9 same-digest
   *  replay) — it rides the SUBMIT result, never the operation record. */
  const [currentReplayed, setCurrentReplayed] = useState<boolean>(false);
  const [submitFailure, setSubmitFailure] = useState<CoreWireEnvelope | null>(null);
  const [submitUnknownKey, setSubmitUnknownKey] = useState<string | null>(null);
  const [results, setResults] = useState<readonly SubmitOutcomeRow[]>([]);

  const mountedRef = useRef(true);
  const pollTimerRef = useRef<number | null>(null);
  const submitSeqRef = useRef(0);

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

  const readRegistry = useCallback(() => {
    setRegistryPhase("loading");
    setRegistryFailure(null);
    void transport.knowledgeImportRegistry().then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setSources(outcome.data.sources);
        setRegistryPhase("data");
        if (outcome.data.sources.length === 0) {
          // The Core serves no kinds: nothing speculative is offered and
          // no kind can be selected (the no-speculative-path law).
          setSelectedKind("");
        } else {
          setSelectedKind((prev) => (prev === "" ? outcome.data.sources[0].source_kind : prev));
        }
      } else if (outcome.kind === "rejection") {
        setRegistryFailure(outcome.envelope);
        setRegistryPhase("failure");
      } else {
        setRegistryPhase("unknown");
      }
    });
  }, [transport]);

  const readPacks = useCallback(
    (cursor?: string) => {
      setPackListPhase("loading");
      setPackListFailure(null);
      const params: { cursor?: string } = {};
      if (cursor !== undefined) params.cursor = cursor;
      void transport.knowledgePackList(params).then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          setPacks((prev) => (cursor === undefined ? outcome.data.packs : [...prev, ...outcome.data.packs]));
          setPacksCursor(outcome.data.cursor ?? null);
          setPackListPhase("data");
        } else if (outcome.kind === "rejection") {
          setPackListFailure(outcome.envelope);
          setPackListPhase("failure");
        } else {
          setPackListPhase("unknown");
        }
      });
    },
    [transport],
  );

  // The registry read on every open — the Core's own list, freshly
  // confirmed each time the wizard opens.
  useEffect(() => {
    if (open) {
      setStep("source");
      setQueue([]);
      setCurrentItemKey(null);
      setCurrentOperation(null);
      setCurrentReplayed(false);
      setSubmitFailure(null);
      setSubmitUnknownKey(null);
      setResults([]);
      setSubmitPhase("idle");
      setSelectedKind("");
      setPackListPhase("idle");
      readRegistry();
    }
  }, [open, readRegistry]);

  const currentItem = queue.find((item) => item.idempotencyKey === currentItemKey) ?? null;
  const selectedSpec = sources.find((spec) => spec.source_kind === selectedKind) ?? null;

  const addFiles = (files: FileList | null) => {
    if (files === null || selectedKind === "") return;
    const items: QueueItem[] = [];
    for (const file of files) {
      items.push({
        idempotencyKey: mintIdempotencyKey(),
        file,
        kind: selectedKind,
        destinationMode: presetDestinationPackId !== undefined && presetDestinationPackId !== "" ? "existing" : "new",
        destinationPackId: presetDestinationPackId !== undefined && presetDestinationPackId !== "" ? presetDestinationPackId : "",
      });
    }
    setQueue((prev) => [...prev, ...items]);
    if (packListPhase === "idle") readPacks();
  };

  const removeItem = (key: string) => {
    // I3/R1: a local act with no Core meaning — no confirmation, no
    // event. The removal control moves focus to the next queued item's
    // control (handled in QueueStep's effect).
    setQueue((prev) => prev.filter((item) => item.idempotencyKey !== key));
  };

  const setItemDestination = (key: string, mode: "new" | "existing", packId?: string) => {
    setQueue((prev) =>
      prev.map((item) =>
        item.idempotencyKey === key
          ? {
              ...item,
              destinationMode: mode,
              destinationPackId: mode === "existing" ? (packId ?? item.destinationPackId) : "",
            }
          : item,
      ),
    );
  };

  // The review gate (§14.3): every item needs a kind and a decided
  // destination; an existing-pack destination needs a selected pack. No
  // invalid/missing required field passes.
  const reviewReady =
    queue.length > 0 &&
    queue.every(
      (item) =>
        item.kind !== "" &&
        (item.destinationMode === "new" ||
          (item.destinationMode === "existing" && item.destinationPackId !== "")),
    );

  const startTracking = useCallback(
    (item: QueueItem) => {
      const seq = ++submitSeqRef.current;
      setCurrentItemKey(item.idempotencyKey);
      setCurrentOperation(null);
      setCurrentReplayed(false);
      setSubmitFailure(null);
      setSubmitUnknownKey(null);
      setStep("tracking");
      setSubmitPhase("submitting");
      void (async () => {
        let contentB64: string;
        try {
          contentB64 = await fileToBase64(item.file);
        } catch {
          if (!mountedRef.current || seq !== submitSeqRef.current) return;
          setSubmitPhase("idle");
          return;
        }
        if (!mountedRef.current || seq !== submitSeqRef.current) return;
        const payload: {
          idempotency_key: string;
          source_name: string;
          source_kind: string;
          content_b64: string;
          destination_pack_id?: string;
        } = {
          idempotency_key: item.idempotencyKey,
          source_name: item.file.name,
          source_kind: item.kind,
          content_b64: contentB64,
        };
        if (item.destinationMode === "existing") payload.destination_pack_id = item.destinationPackId;
        const outcome = await transport.knowledgeImportSubmit(payload);
        if (!mountedRef.current || seq !== submitSeqRef.current) return;
        if (outcome.kind === "data") {
          // 202 = accepted for processing, never completion (J-5): the
          // tracking display moves to the state endpoint's reads. The
          // honest `replayed` flag rides this result and is rendered on
          // the completed outcome (the M-9 same-digest replay).
          setCurrentOperation(outcome.data.operation);
          setCurrentReplayed(outcome.data.replayed);
          setSubmitPhase("polling");
        } else if (outcome.kind === "rejection") {
          setSubmitFailure(outcome.envelope);
          setSubmitPhase("idle");
        } else {
          // A lost submit response: never proof of success or failure —
          // the explained state carries the item's idempotency key, and
          // a same-key same-bytes re-submission is safely reconciled by
          // the Core (the M-9 class); nothing is auto-retried here.
          setSubmitUnknownKey(item.idempotencyKey);
          setSubmitPhase("idle");
        }
      })();
    },
    [transport],
  );

  const submitQueueFromReview = () => {
    if (!reviewReady) return;
    const first = queue[0];
    if (first !== undefined) startTracking(first);
  };

  const recordResult = useCallback(
    (operation: KnowledgeOperation) => {
      if (currentItemKey === null) return;
      const item = queue.find((entry) => entry.idempotencyKey === currentItemKey);
      if (item === undefined) return;
      setResults((prev) =>
        prev.some((row) => row.itemKey === item.idempotencyKey)
          ? prev
          : [...prev, { itemKey: item.idempotencyKey, fileName: item.file.name, operation }],
      );
    },
    [currentItemKey, queue],
  );

  // The state-endpoint poll: the authoritative reconciliation read (a
  // pure query — never a retry). One real read per tick; a lost read
  // keeps the last confirmed state visible and the next tick re-queries
  // — never a timeout-inferred state.
  const pollOperation = useCallback(
    (operationID: string) => {
      void transport.knowledgeOperationStatus(operationID).then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          const op = outcome.data.operation;
          setCurrentOperation(op);
          if (KNOWLEDGE_TERMINAL_STATES.includes(op.state)) {
            setSubmitPhase("idle");
            recordResult(op);
          }
        }
      });
    },
    [recordResult, transport],
  );

  // The poll loop while a tracked operation is live.
  useEffect(() => {
    if (step !== "tracking" || currentOperation === null) return;
    if (!isLiveState(currentOperation.state)) return;
    pollTimerRef.current = window.setTimeout(() => {
      pollTimerRef.current = null;
      pollOperation(currentOperation.operation_id);
    }, POLL_INTERVAL_MS);
    return () => {
      if (pollTimerRef.current !== null) {
        window.clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [step, currentOperation, pollOperation]);

  const requestCancel = () => {
    if (currentOperation === null) return;
    void transport.knowledgeOperationCancel(currentOperation.operation_id).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        // 202 = the request is accepted for processing — never the
        // confirmation (J-5). The poll loop continues; the terminal
        // `cancelled` state IS the confirmation.
        setCurrentOperation(outcome.data.operation);
      } else if (outcome.kind === "rejection") {
        // Terminal operations are the typed CONFLICT_STATE refusal: the
        // envelope surfaces verbatim and the state read stays
        // authoritative.
        setSubmitFailure(outcome.envelope);
        setSubmitPhase("idle");
      }
      // A lost cancel request is never proof of cancellation — the poll
      // continues and the state endpoint remains the source of truth.
    });
  };

  const continueAfterCurrent = () => {
    const remaining = queue.filter(
      (item) => !results.some((row) => row.itemKey === item.idempotencyKey),
    );
    const next = remaining[0];
    if (next !== undefined) {
      startTracking(next);
    } else {
      setCurrentItemKey(null);
      setCurrentOperation(null);
      setSubmitPhase("idle");
    }
  };

  const dialogOpen = open;

  return (
    <Dialog
      labelId={labelId}
      title={t("knowledge.import.title")}
      open={dialogOpen}
      onClose={onClose}
    >
      <ol className={wizardStyles.steps}>
        <li className={step === "source" ? wizardStyles.stepCurrent : wizardStyles.step}>
          {t("knowledge.import.stepSource")}
        </li>
        <li className={step === "queue" ? wizardStyles.stepCurrent : wizardStyles.step}>
          {t("knowledge.import.stepQueue")}
        </li>
        <li className={step === "review" ? wizardStyles.stepCurrent : wizardStyles.step}>
          {t("knowledge.import.stepReview")}
        </li>
        <li className={step === "tracking" ? wizardStyles.stepCurrent : wizardStyles.step}>
          {t("knowledge.import.stepTracking")}
        </li>
      </ol>

      {step === "source" ? (
        <SourceStep
          registryPhase={registryPhase}
          sources={sources}
          registryFailure={registryFailure}
          selectedKind={selectedKind}
          onSelectKind={setSelectedKind}
          onRetry={readRegistry}
          onContinue={() => {
            setStep("queue");
            if (packListPhase === "idle") readPacks();
          }}
        />
      ) : null}

      {step === "queue" ? (
        <QueueStep
          queue={queue}
          selectedSpec={selectedSpec}
          packs={packs}
          packsCursor={packsCursor}
          packListPhase={packListPhase}
          packListFailure={packListFailure}
          onFiles={addFiles}
          onRemove={removeItem}
          onDestination={setItemDestination}
          onLoadMorePacks={() => {
            if (packsCursor !== null) readPacks(packsCursor);
          }}
          onBack={() => setStep("source")}
          onContinue={() => setStep("review")}
        />
      ) : null}

      {step === "review" ? (
        <ReviewStep
          queue={queue}
          sources={sources}
          reviewReady={reviewReady}
          onBack={() => setStep("queue")}
          onConfirm={submitQueueFromReview}
        />
      ) : null}

      {step === "tracking" ? (
        <TrackingStep
          currentItem={currentItem}
          submitPhase={submitPhase}
          currentOperation={currentOperation}
          currentReplayed={currentReplayed}
          submitFailure={submitFailure}
          submitUnknownKey={submitUnknownKey}
          results={results}
          queue={queue}
          onCancel={requestCancel}
          onReconcile={() => {
            if (currentOperation !== null) pollOperation(currentOperation.operation_id);
          }}
          onResubmitSameKey={() => {
            if (currentItem !== null) startTracking(currentItem);
          }}
          onContinueAfterCurrent={continueAfterCurrent}
          onBackToReview={() => {
            setStep("review");
            setCurrentItemKey(null);
            setCurrentOperation(null);
            setSubmitPhase("idle");
          }}
        />
      ) : null}
    </Dialog>
  );
}

function isLiveState(state: string): boolean {
  // The live set is the complement of the terminal set within the
  // served register; written explicitly so an unknown future state
  // value reads as LIVE (the Core owns the terminal truth — the poll
  // continues until the Core reports a terminal state).
  return (
    state === "submitted" ||
    state === "uploading" ||
    state === "extracting" ||
    state === "validating" ||
    state === "indexing" ||
    state === "generating"
  );
}

function SourceStep(props: {
  registryPhase: RegistryPhase;
  sources: readonly KnowledgeImportSourceSpec[];
  registryFailure: CoreWireEnvelope | null;
  selectedKind: string;
  onSelectKind: (kind: string) => void;
  onRetry: () => void;
  onContinue: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  if (props.registryPhase === "loading") {
    return <p className={styles.note}>{t("knowledge.import.registryLoading")}</p>;
  }

  if (props.registryPhase === "failure" && props.registryFailure !== null) {
    return (
      <div>
        <RecoveryPanel
          title={t("knowledge.import.registryFailureTitle")}
          body={t("knowledge.import.registryFailureBody")}
          referenceId={props.registryFailure.correlationId}
          actions={[{ label: t("knowledge.import.registryRetry"), onAction: props.onRetry }]}
        />
        <p className={styles.code}>
          <span className="visually-hidden">Code: </span>
          <code>{props.registryFailure.code}</code>
        </p>
      </div>
    );
  }

  if (props.registryPhase === "unknown") {
    return (
      <RecoveryPanel
        title={t("knowledge.import.registryFailureTitle")}
        body={t("knowledge.import.registryUnknownBody")}
        actions={[{ label: t("knowledge.import.registryRetry"), onAction: props.onRetry }]}
      />
    );
  }

  return (
    <div>
      <p className={styles.description}>{t("knowledge.import.registryHeading")}</p>
      {props.sources.length === 0 ? (
        <p className={styles.note}>{t("knowledge.import.registryNoneServed")}</p>
      ) : (
        <fieldset className={wizardStyles.kindList}>
          <legend className="visually-hidden">{t("knowledge.import.selectKind")}</legend>
          {props.sources.map((spec) => (
            <label key={spec.source_kind} className={wizardStyles.kindRow}>
              <input
                type="radio"
                name="knowledge-import-kind"
                value={spec.source_kind}
                checked={props.selectedKind === spec.source_kind}
                onChange={() => props.onSelectKind(spec.source_kind)}
              />
              <span>
                <code>{spec.source_kind}</code>
                <span className={wizardStyles.kindMeta}>
                  {t("knowledge.import.kindSizeLimit", { limit: formatBytes(spec.max_bytes, locale) })}
                </span>
                {spec.notes !== undefined && spec.notes !== "" ? (
                  <span className={wizardStyles.kindMeta}>{spec.notes}</span>
                ) : null}
              </span>
            </label>
          ))}
        </fieldset>
      )}
      <div className={wizardStyles.actions}>
        <button
          type="button"
          className={styles.primaryControl}
          onClick={props.onContinue}
          disabled={props.selectedKind === ""}
        >
          {t("knowledge.import.continue")}
        </button>
      </div>
    </div>
  );
}

function QueueStep(props: {
  queue: readonly QueueItem[];
  selectedSpec: KnowledgeImportSourceSpec | null;
  packs: readonly KnowledgePack[];
  packsCursor: string | null;
  packListPhase: PackListPhase;
  packListFailure: CoreWireEnvelope | null;
  onFiles: (files: FileList | null) => void;
  onRemove: (key: string) => void;
  onDestination: (key: string, mode: "new" | "existing", packId?: string) => void;
  onLoadMorePacks: () => void;
  onBack: () => void;
  onContinue: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");
  const removedFocusRef = useRef<string | null>(null);

  // After a removal, focus moves to the next queued item's remove
  // control (I3's focus law).
  useEffect(() => {
    const removedKey = removedFocusRef.current;
    if (removedKey === null) return;
    removedFocusRef.current = null;
    const next = props.queue.find((item) => item.idempotencyKey !== removedKey);
    if (next === undefined) return;
    const nextButton = document.querySelector<HTMLButtonElement>(
      `[data-remove-key="${next.idempotencyKey}"]`,
    );
    nextButton?.focus();
  }, [props.queue]);

  return (
    <div>
      <p className={styles.description}>{t("knowledge.import.chooseFile")}</p>
      <input
        type="file"
        // No accept filter: the served registry carries no extension or
        // MIME mapping, so none is invented — the Core's content
        // validation is authoritative (T3's law).
        multiple
        onChange={(event) => {
          props.onFiles(event.target.files);
          event.target.value = "";
        }}
        className={wizardStyles.fileInput}
        aria-label={t("knowledge.import.chooseFile")}
      />
      {props.selectedSpec !== null ? (
        <p className={styles.note}>
          {t("knowledge.import.kindSizeLimit", { limit: formatBytes(props.selectedSpec.max_bytes, locale) })}
        </p>
      ) : null}

      <h3 className={wizardStyles.subHeading}>{t("knowledge.import.queueHeading")}</h3>
      <p className={styles.note}>{t("knowledge.import.queueRemoveNote")}</p>
      {props.queue.length === 0 ? (
        <p className={styles.note}>{t("knowledge.import.queueEmpty")}</p>
      ) : (
        <ul className={wizardStyles.queueList}>
          {props.queue.map((item) => {
            const spec = props.selectedSpec;
            const overLimit = spec !== null && item.file.size > spec.max_bytes;
            return (
              <li key={item.idempotencyKey} className={wizardStyles.queueItem}>
                <div className={wizardStyles.queueItemMain}>
                  <span className={wizardStyles.queueName}>{item.file.name}</span>
                  <span className={wizardStyles.queueMeta}>
                    {t("knowledge.import.fileSizeLabel")}: {formatBytes(item.file.size, locale)}
                  </span>
                  {overLimit ? (
                    <span className={wizardStyles.advisory}>{t("knowledge.import.sizeAdvisoryOver")}</span>
                  ) : (
                    <span className={wizardStyles.advisoryQuiet}>{t("knowledge.import.sizeAdvisory")}</span>
                  )}
                </div>
                <fieldset className={wizardStyles.destinationFieldset}>
                  <legend className={wizardStyles.destinationLegend}>
                    {t("knowledge.import.destinationHeading")}
                  </legend>
                  <label className={wizardStyles.destinationOption}>
                    <input
                      type="radio"
                      name={`dest-${item.idempotencyKey}`}
                      checked={item.destinationMode === "new"}
                      onChange={() => props.onDestination(item.idempotencyKey, "new")}
                    />
                    <span>
                      {t("knowledge.import.destinationNewPack")}
                      <span className={wizardStyles.kindMeta}>
                        {" — "}
                        {t("knowledge.import.destinationNewPackNote")}
                      </span>
                    </span>
                  </label>
                  <label className={wizardStyles.destinationOption}>
                    <input
                      type="radio"
                      name={`dest-${item.idempotencyKey}`}
                      checked={item.destinationMode === "existing"}
                      onChange={() => props.onDestination(item.idempotencyKey, "existing")}
                    />
                    <span>{t("knowledge.import.destinationExistingPack")}</span>
                  </label>
                  {item.destinationMode === "existing" ? (
                    <div>
                      <label
                        className={wizardStyles.destinationPackLabel}
                        htmlFor={`dest-pack-${item.idempotencyKey}`}
                      >
                        {t("knowledge.import.destinationPackLabel")}
                      </label>
                      <select
                        id={`dest-pack-${item.idempotencyKey}`}
                        className={wizardStyles.destinationPackSelect}
                        value={item.destinationPackId}
                        onChange={(event) =>
                          props.onDestination(item.idempotencyKey, "existing", event.target.value)
                        }
                      >
                        <option value="">{t("knowledge.import.destinationPackPlaceholder")}</option>
                        {props.packs.map((pack) => (
                          <option key={pack.pack_id} value={pack.pack_id}>
                            {pack.title} ({pack.pack_id})
                          </option>
                        ))}
                      </select>
                      {props.packsCursor !== null ? (
                        <button type="button" className={styles.control} onClick={props.onLoadMorePacks}>
                          {t("knowledge.list.loadMore")}
                        </button>
                      ) : null}
                      {props.packListPhase === "loading" ? (
                        <p className={styles.note}>{t("knowledge.import.destinationPackLoading")}</p>
                      ) : null}
                      {props.packListPhase === "failure" && props.packListFailure !== null ? (
                        <p className={styles.code}>
                          <code>{props.packListFailure.code}</code>
                        </p>
                      ) : null}
                      {props.packListPhase === "unknown" ? (
                        <p className={styles.note}>{unavailable}</p>
                      ) : null}
                    </div>
                  ) : null}
                </fieldset>
                <button
                  type="button"
                  className={styles.control}
                  data-remove-key={item.idempotencyKey}
                  onClick={() => {
                    removedFocusRef.current = item.idempotencyKey;
                    props.onRemove(item.idempotencyKey);
                  }}
                >
                  {t("knowledge.import.queueRemove")}
                </button>
              </li>
            );
          })}
        </ul>
      )}

      <div className={wizardStyles.actions}>
        <button type="button" className={styles.control} onClick={props.onBack}>
          {t("knowledge.import.back")}
        </button>
        <button
          type="button"
          className={styles.primaryControl}
          onClick={props.onContinue}
          disabled={props.queue.length === 0}
        >
          {t("knowledge.import.continue")}
        </button>
      </div>
    </div>
  );
}

function ReviewStep(props: {
  queue: readonly QueueItem[];
  sources: readonly KnowledgeImportSourceSpec[];
  reviewReady: boolean;
  onBack: () => void;
  onConfirm: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  return (
    <div>
      <p className={styles.description}>{t("knowledge.import.reviewHeading")}</p>
      <ol className={wizardStyles.reviewList}>
        {props.queue.map((item, index) => {
          const spec = props.sources.find((entry) => entry.source_kind === item.kind) ?? null;
          return (
            <li key={item.idempotencyKey} className={wizardStyles.reviewItem}>
              <p className={wizardStyles.reviewItemHeading}>
                {t("knowledge.import.reviewItemHeading", { index: index + 1 })}
              </p>
              <dl className={wizardStyles.reviewFacts}>
                <div>
                  <dt>{t("knowledge.import.reviewKind")}</dt>
                  <dd>
                    <code>{item.kind}</code>
                  </dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewFile")}</dt>
                  <dd>{item.file.name}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewSize")}</dt>
                  <dd>{formatBytes(item.file.size, locale)}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewLimit")}</dt>
                  <dd>{spec === null ? unavailable : formatBytes(spec.max_bytes, locale)}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewDestination")}</dt>
                  <dd>
                    {item.destinationMode === "new"
                      ? t("knowledge.import.reviewDestinationNew")
                      : item.destinationPackId === ""
                        ? unavailable
                        : t("knowledge.import.reviewDestinationExisting", {
                            packId: item.destinationPackId,
                          })}
                  </dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewOrigin")}</dt>
                  <dd>{t("knowledge.import.reviewOriginValue")}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewLicenseFreshness")}</dt>
                  <dd>{t("knowledge.import.reviewLicenseFreshnessNone")}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewPermission")}</dt>
                  <dd>{t("knowledge.import.reviewPermissionValue")}</dd>
                </div>
                <div>
                  <dt>{t("knowledge.import.reviewBoundary")}</dt>
                  <dd>{t("knowledge.import.reviewBoundaryValue")}</dd>
                </div>
              </dl>
            </li>
          );
        })}
      </ol>
      <p className={styles.note}>{t("knowledge.import.reviewUntrustedNote")}</p>
      <div className={wizardStyles.actions}>
        <button type="button" className={styles.control} onClick={props.onBack}>
          {t("knowledge.import.back")}
        </button>
        <button
          type="button"
          className={styles.primaryControl}
          onClick={props.onConfirm}
          disabled={!props.reviewReady}
        >
          {t("knowledge.import.reviewConfirm")}
        </button>
      </div>
    </div>
  );
}

function TrackingStep(props: {
  currentItem: QueueItem | null;
  submitPhase: SubmitPhase;
  currentOperation: KnowledgeOperation | null;
  currentReplayed: boolean;
  submitFailure: CoreWireEnvelope | null;
  submitUnknownKey: string | null;
  results: readonly SubmitOutcomeRow[];
  queue: readonly QueueItem[];
  onCancel: () => void;
  onReconcile: () => void;
  onResubmitSameKey: () => void;
  onContinueAfterCurrent: () => void;
  onBackToReview: () => void;
}) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const [confirmCancelOpen, setConfirmCancelOpen] = useState(false);
  const [cancelRequested, setCancelRequested] = useState(false);

  const op = props.currentOperation;
  const live = op !== null && isLiveState(op.state);
  const remaining = props.queue.filter(
    (item) => !props.results.some((row) => row.itemKey === item.idempotencyKey),
  );

  return (
    <div>
      {props.currentItem !== null ? (
        <p className={styles.note}>
          {t("knowledge.import.reviewFile")}: {props.currentItem.file.name}
        </p>
      ) : null}

      {props.submitPhase === "submitting" ? (
        <p className={styles.note}>{t("knowledge.import.submitting")}</p>
      ) : null}

      {props.submitUnknownKey !== null && op === null ? (
        <div>
          <RecoveryPanel
            title={t("knowledge.import.submitUnknownTitle")}
            body={t("knowledge.import.submitUnknownBody", { idempotencyKey: props.submitUnknownKey })}
            actions={[{ label: t("knowledge.import.reviewConfirm"), onAction: props.onResubmitSameKey }]}
          />
        </div>
      ) : null}

      {props.submitFailure !== null ? (
        <div>
          {props.submitFailure.code === "CONFLICT_IDEMPOTENCY_KEY" ? (
            <RecoveryPanel
              title={t("knowledge.import.submitFailureTitle")}
              body={t("knowledge.import.submitConflictSameKey")}
            />
          ) : (
            <RecoveryPanel
              title={t("knowledge.import.submitFailureTitle")}
              body={t("knowledge.import.submitFailureBody")}
              referenceId={props.submitFailure.correlationId}
            />
          )}
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{props.submitFailure.code}</code>
          </p>
        </div>
      ) : null}

      {op !== null ? (
        <section className={wizardStyles.trackingPanel}>
          <h3 className={wizardStyles.subHeading}>{t("knowledge.operation.trackingHeading")}</h3>
          <p className={styles.note}>{t("knowledge.operation.acceptedNote")}</p>
          <p className={wizardStyles.operationStateRow}>
            <OperationChip state={op.state} />
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
              <dt>{t("knowledge.operation.kind")}</dt>
              <dd>{op.kind}</dd>
            </div>
            <div>
              <dt>{t("knowledge.operation.lastUpdated")}</dt>
              <dd>
                <time dateTime={op.updated_at}>{formatTimestamp(new Date(op.updated_at), locale)}</time>
              </dd>
            </div>
            {op.failure_code !== undefined && op.failure_code !== "" ? (
              <div>
                <dt>{t("knowledge.operation.failureCode")}</dt>
                <dd>
                  <code>{op.failure_code}</code>
                </dd>
              </div>
            ) : null}
            {op.untrusted_label !== undefined && op.untrusted_label !== "" ? (
              <div>
                <dt>{t("knowledge.operation.untrustedLabel")}</dt>
                <dd>
                  <code>{op.untrusted_label}</code>
                </dd>
              </div>
            ) : null}
          </dl>
          {op.failure_reason !== undefined && op.failure_reason !== "" ? (
            <p className={styles.note}>
              {t("knowledge.operation.failureReason")}: {op.failure_reason}
            </p>
          ) : null}

          {live ? (
            <div>
              {cancelRequested ? (
                <p className={styles.note}>{t("knowledge.operation.cancelRequestAccepted")}</p>
              ) : null}
              {confirmCancelOpen ? (
                <div className={wizardStyles.confirmBlock}>
                  <p className={wizardStyles.confirmTitle}>{t("knowledge.operation.cancelConfirmTitle")}</p>
                  <p>{t("knowledge.operation.cancelConfirmBody")}</p>
                  <div className={wizardStyles.actions}>
                    <button type="button" className={styles.control} onClick={() => setConfirmCancelOpen(false)}>
                      {t("actions.cancel")}
                    </button>
                    <button
                      type="button"
                      className={styles.primaryControl}
                      onClick={() => {
                        setConfirmCancelOpen(false);
                        setCancelRequested(true);
                        props.onCancel();
                      }}
                    >
                      {t("knowledge.operation.cancelConfirm")}
                    </button>
                  </div>
                </div>
              ) : (
                <div className={wizardStyles.actions}>
                  <button
                    type="button"
                    className={styles.control}
                    onClick={() => {
                      setCancelRequested(false);
                      setConfirmCancelOpen(true);
                    }}
                  >
                    {t("knowledge.operation.cancel")}
                  </button>
                  <button type="button" className={styles.control} onClick={props.onReconcile}>
                    {t("knowledge.operation.reconcile")}
                  </button>
                </div>
              )}
              <p className={styles.note}>{t("knowledge.operation.closeDuringLiveNote")}</p>
            </div>
          ) : null}

          {op.state === "completed" ? <CompletedResult operation={op} replayed={props.currentReplayed} /> : null}
          {op.state === "rejected" || op.state === "failed" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>
                {op.state === "rejected" ? t("knowledge.states.rejected") : t("knowledge.states.failed")}
              </p>
              <p>
                {op.state === "rejected"
                  ? t("knowledge.operation.rejectedNote")
                  : t("knowledge.operation.failedNote")}
              </p>
              {op.failure_code !== undefined && op.failure_code !== "" ? (
                <div>
                  <p className={styles.note}>{t(knowledgeFailureCodeKey(op.failure_code))}</p>
                  <p className={styles.note}>{t(knowledgeFailureRecoveryKey(op.failure_code))}</p>
                </div>
              ) : null}
              <div className={wizardStyles.actions}>
                <button type="button" className={styles.primaryControl} onClick={props.onContinueAfterCurrent}>
                  {t("knowledge.import.continue")}
                </button>
              </div>
            </section>
          ) : null}
          {op.state === "cancelled" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>{t("knowledge.states.cancelled")}</p>
              <p>{t("knowledge.operation.cancelledNote")}</p>
              <div className={wizardStyles.actions}>
                <button type="button" className={styles.primaryControl} onClick={props.onContinueAfterCurrent}>
                  {t("knowledge.import.continue")}
                </button>
              </div>
            </section>
          ) : null}
          {op.state === "outcome_unknown" ? (
            <section className={wizardStyles.outcomeBlock}>
              <p className={wizardStyles.outcomeTitle}>{t("knowledge.states.outcome_unknown")}</p>
              <p>{t("knowledge.operation.outcomeUnknownNote")}</p>
              <div className={wizardStyles.actions}>
                <button type="button" className={styles.control} onClick={props.onReconcile}>
                  {t("knowledge.operation.reconcile")}
                </button>
              </div>
              <p className={styles.note}>{t("knowledge.operation.reconcileNote")}</p>
            </section>
          ) : null}
        </section>
      ) : null}

      {props.results.length > 0 ? (
        <section>
          <h3 className={wizardStyles.subHeading}>{t("knowledge.operation.resultHeading")}</h3>
          <ul className={wizardStyles.resultList}>
            {props.results.map((row) => (
              <li key={row.itemKey} className={wizardStyles.resultRow}>
                <span>{row.fileName}</span>
                <OperationChip state={row.operation.state} />
                {row.operation.state === "completed" ? (
                  <CompletedResult operation={row.operation} inline />
                ) : row.operation.failure_code !== undefined && row.operation.failure_code !== "" ? (
                  <span className={wizardStyles.kindMeta}>
                    {t(knowledgeFailureCodeKey(row.operation.failure_code))}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <div className={wizardStyles.actions}>
        {op !== null && !live && (op.state === "completed" || op.state === "rejected" || op.state === "failed") ? (
          <button
            type="button"
            className={styles.primaryControl}
            onClick={props.onContinueAfterCurrent}
            disabled={remaining.length === 0}
          >
            {t("knowledge.import.continue")}
          </button>
        ) : null}
        <button type="button" className={styles.control} onClick={props.onBackToReview}>
          {t("knowledge.import.back")}
        </button>
      </div>
    </div>
  );
}

function OperationChip({ state }: { state: string }) {
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

function CompletedResult({
  operation,
  replayed,
  inline,
}: {
  operation: KnowledgeOperation;
  /** The submit result's honest replay flag; only the tracking panel's
   *  non-inline render shows it (the M-9 same-digest replay note). */
  replayed?: boolean;
  inline?: boolean;
}) {
  const t = useT();
  const result = operation.result;

  if (inline) {
    return (
      <span className={wizardStyles.resultFacts}>
        {result !== undefined &&
        result !== null &&
        result.pack_id !== undefined &&
        result.version_id !== undefined ? (
          <Link to={`/knowledge/packs/${encodeURIComponent(result.pack_id)}`} className={styles.rowLink}>
            {result.pack_id} @ {result.version_id}
          </Link>
        ) : (
          <span>{t("states.unavailable")}</span>
        )}
      </span>
    );
  }

  return (
    <section className={wizardStyles.outcomeBlock}>
      <p className={wizardStyles.outcomeTitle}>{t("knowledge.operation.completedImportNote")}</p>
      {replayed === true ? <p className={styles.note}>{t("knowledge.operation.replayedNote")}</p> : null}
      {result !== undefined && result !== null ? (
        <dl className={wizardStyles.reviewFacts}>
          <div>
            <dt>{t("knowledge.operation.resultImportRef")}</dt>
            <dd>
              {result.pack_id !== undefined && result.version_id !== undefined ? (
                <Link to={`/knowledge/packs/${encodeURIComponent(result.pack_id)}`} className={styles.rowLink}>
                  {result.pack_id} @ {result.version_id}
                </Link>
              ) : (
                t("states.unavailable")
              )}
            </dd>
          </div>
          {result.digest !== undefined && result.digest !== "" ? (
            <div>
              <dt>{t("knowledge.operation.resultDigest")}</dt>
              <dd>
                <code>{result.digest}</code>
              </dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </section>
  );
}
