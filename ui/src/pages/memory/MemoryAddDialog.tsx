import { useEffect, useRef, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, MemoryItemServed } from "../../coreclient/httpLocal";
import { useT } from "../../i18n/useT";
import {
  CODED_MEMORY_SCOPES,
  memoryFailureCodeKey,
  memoryFailureRecoveryKey,
  memoryLifecycleChip,
  memoryScopeLabelKey,
  mintMemoryAddKey,
  mintMemoryCorrelationId,
} from "./memoryStates";
import styles from "./memoryDialogs.module.css";
import listStyles from "./MemoryLibraryPage.module.css";

/**
 * The add-item dialog (B2-U1; P4.1 M2 — the M-17 creation class against
 * the served POST /v1/memories member).
 *
 * The laws this component carries, exactly:
 * - **The Core mints the identity** — the body never carries one; the
 *   scope is STATED at creation (the scope selector is the closed
 *   register's own values) and is never broadened afterward (P4.5 §3.2).
 * - **The origin is honest, never a client claim** (B2-A1 §2.5's
 *   origin-labeling law): a user-created item is `user-entered` — the
 *   form offers NO origin choice; the body carries origin `user-entered`
 *   exactly. `model-extracted` can only be set by the Core's own
 *   extraction path, and `imported`/`approved-decision` are never client
 *   claims.
 * - **The secrets law (BINDING)**: the body must never attempt secret
 *   storage; the Core refuses any attempt (MEMORY_SECRETS_REJECTED) and
 *   the surface renders that refusal as the Core returns it — no secret
 *   value is ever echoed (the matched material is never echoed into the
 *   envelope, so no reason value renders).
 * - **Only a confirmed 200 carries the outcome truth** (the F11 law) —
 *   the Core-confirmed item's `lifecycle_state` is the entry-state truth
 *   (the response's lifecycle_state carries it, never an assumption;
 *   B2-A1 §2.6 item 2). The request is never displayed as the
 *   confirmation (the J-5 rule).
 * - **A lost response is unknown — never proof of success or failure**
 *   (the S0–S4 law): the state offers the M-17 keyed re-issue (the SAME
 *   minted key + correlation replays the recorded item — the reconcile
 *   path, never a blind re-add) and the list re-read; nothing is
 *   optimistically inserted anywhere.
 * - The pending-review truth: this surface never fabricates a
 *   pending-review entry state — the add lands whatever lifecycle_state
 *   the Core confirms (this Core's entry-state policy lands `active` for
 *   a user-entered add; `pending_review` is reserved for the Core's own
 *   extraction path, which has no producer here — B2-A1 §2.6 item 4).
 */

type AddPhase = "editing" | "submitting" | "done" | "unknown";

const MEMORY_SCOPES: readonly string[] = CODED_MEMORY_SCOPES;

export interface MemoryAddDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
}

export function MemoryAddDialog({ open, onClose }: MemoryAddDialogProps) {
  const t = useT();
  const { transport } = useCoreConnection();
  const labelId = useDialogLabelId();

  const [scope, setScope] = useState<string>("user");
  const [content, setContent] = useState("");
  const [originatingRefsText, setOriginatingRefsText] = useState("");
  const [phase, setPhase] = useState<AddPhase>("editing");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [confirmedItem, setConfirmedItem] = useState<MemoryItemServed | null>(null);
  const [reissue, setReissue] = useState<{ idempotencyKey: string; correlationId: string; scope: string; content: string; refs: string[] } | null>(null);
  const submitTriggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (open) {
      setScope("user");
      setContent("");
      setOriginatingRefsText("");
      setPhase("editing");
      setFailure(null);
      setConfirmedItem(null);
      setReissue(null);
    }
  }, [open]);

  const refs = originatingRefsText
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");

  const submit = (payload: { idempotencyKey: string; correlationId: string; scope: string; content: string; refs: string[] }) => {
    setFailure(null);
    setPhase("submitting");
    const body = {
      idempotency_key: payload.idempotencyKey,
      correlation_id: payload.correlationId,
      scope: payload.scope,
      // The honest origin: a client add is user-entered — the Core
      // validates the origin claim and refuses any client claim of the
      // other origins (B2-A1 §2.5's request-shape law).
      origin: "user-entered",
      content: payload.content,
      ...(payload.refs.length > 0 ? { originating_refs: payload.refs } : {}),
    };
    void transport.memoryAdd(body).then((outcome) => {
      if (outcome.kind === "data") {
        // Only a confirmed 200 carries the outcome truth; the
        // Core-confirmed item (its lifecycle_state included) renders.
        setConfirmedItem(outcome.data.memory_item);
        setReissue(null);
        setPhase("done");
      } else if (outcome.kind === "rejection") {
        setFailure(outcome.envelope);
        setPhase("editing");
      } else {
        // A lost add response is unknown — never proof of success or
        // failure. The minted identity is kept for the explicit M-17
        // keyed re-issue (the reconcile path: the same key + correlation
        // replays the recorded item — never a blind re-add).
        setReissue(payload);
        setPhase("unknown");
      }
    });
  };

  const onSubmit = () => {
    const payload = {
      idempotencyKey: mintMemoryAddKey(),
      correlationId: mintMemoryCorrelationId("memory-add"),
      scope,
      content,
      refs,
    };
    submit(payload);
  };

  const item = confirmedItem;
  const lifecycleChip = item !== null ? memoryLifecycleChip(item.lifecycle_state) : null;
  const LifecycleIcon = lifecycleChip?.icon;

  return (
    <Dialog labelId={labelId} title={t("memory.add.title")} open={open} onClose={onClose}>
      {phase === "editing" ? (
        <div>
          <label className={styles.fieldLabel} htmlFor="memory-add-scope">
            {t("memory.add.scopeLabel")}
          </label>
          <select
            id="memory-add-scope"
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
          <p className={styles.fieldNote}>{t("memory.add.scopeNote")}</p>

          <label className={styles.fieldLabel} htmlFor="memory-add-content">
            {t("memory.add.contentLabel")}
          </label>
          <textarea
            id="memory-add-content"
            className={styles.fieldTextarea}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            required
          />

          <label className={styles.fieldLabel} htmlFor="memory-add-refs">
            {t("memory.add.refsLabel")}
          </label>
          <textarea
            id="memory-add-refs"
            className={styles.fieldTextarea}
            value={originatingRefsText}
            onChange={(event) => setOriginatingRefsText(event.target.value)}
          />
          <p className={styles.fieldNote}>{t("memory.add.refsNote")}</p>

          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("memory.add.originHeading")}</p>
            <p>{t("memory.add.originValue")}</p>
          </section>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("memory.add.secretsHeading")}</p>
            <p>{t("memory.add.secretsValue")}</p>
          </section>

          {failure !== null ? (
            <div>
              <RecoveryPanel
                title={t("memory.add.failureTitle")}
                body={t(memoryFailureCodeKey(failure.code))}
                note={t(memoryFailureRecoveryKey(failure.code))}
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
              onClick={onSubmit}
              disabled={content.trim() === ""}
            >
              {t("memory.add.submit")}
            </button>
          </div>
        </div>
      ) : null}

      {phase === "submitting" ? (
        <p className={styles.fieldNote} role="status">
          {t("memory.add.submitting")}
        </p>
      ) : null}

      {phase === "done" && item !== null && lifecycleChip !== null && LifecycleIcon !== undefined ? (
        <div>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("memory.add.confirmedTitle")}</p>
            <p className={styles.fieldNote}>{t("memory.add.confirmedEntryStateNote")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("memory.add.confirmedStateLabel")}</dt>
                <dd>
                  <span className={`${styles.chip} ${styles[lifecycleChip.tone]}`}>
                    <LifecycleIcon aria-hidden="true" size={14} />
                    {t(lifecycleChip.labelKey)}
                  </span>
                </dd>
              </div>
              <div>
                <dt>{t("memory.add.confirmedScopeLabel")}</dt>
                <dd>
                  {(() => {
                    const scopeKey = memoryScopeLabelKey(item.scope);
                    return scopeKey !== null ? t(scopeKey) : item.scope;
                  })()}
                </dd>
              </div>
              <div>
                <dt>{t("memory.add.confirmedItemIdLabel")}</dt>
                <dd>
                  <code>{item.memory_item_id}</code>
                </dd>
              </div>
            </dl>
          </section>
        </div>
      ) : null}

      {phase === "unknown" && reissue !== null ? (
        <div>
          <RecoveryPanel
            title={t("memory.add.unknownTitle")}
            body={t("memory.add.unknownBody", { correlationId: reissue.correlationId })}
          />
          <p className={styles.fieldNote}>{t("memory.add.unknownReconcileNote")}</p>
          <div className={styles.actions}>
            <button
              type="button"
              className={styles.primaryControl}
              onClick={() => submit(reissue)}
            >
              {t("memory.add.unknownReissue")}
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
