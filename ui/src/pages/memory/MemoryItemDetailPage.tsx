import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Pin, PinOff } from "lucide-react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  MemoryDeleteResult,
  MemoryInclusionResult,
  MemoryItemServed,
  TransportOutcome,
  WireMemoryDeleteResult,
  WireMemoryMutationResult,
} from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import { MemoryExportDialog } from "./MemoryExportDialog";
import styles from "./MemoryItemDetailPage.module.css";
import dialogStyles from "./memoryDialogs.module.css";
import {
  CopyIdButton,
  fieldOrUnavailable,
  memoryFailureCodeKey,
  memoryFailureRecoveryKey,
  memoryItemStateRef,
  memoryLifecycleChip,
  memoryOriginLabelKey,
  memoryScopeLabelKey,
  mintMemoryCorrelationId,
  mintMemoryOperationKey,
} from "./memoryStates";

/**
 * `/memory/:memoryItemId` — the ONE canonical memory-item detail route
 * (B2-U1; P4.1 R1–R8 against the now-live B2 adapter, the M-16/M-18
 * register rows at their recorded matrix positions).
 *
 * The laws this page carries, exactly:
 * - **R1 — the detail read**: every field renders from the served item
 *   only; missing/empty members render Unavailable or "none reported" —
 *   never zero, never invented (the P4.5 §3.3 map's law). Trust state is
 *   DISTINCT from pending-review: the lifecycle_state carries the truth
 *   (the entry-state honesty law) and no trust register or badge is
 *   invented anywhere. Conflict/freshness warnings render ONLY as the
 *   Core returns them (`conflict_note`; empty = none reported). The
 *   source event (originating_refs) is shown, never rewritten.
 * - **R2 — Approve/Reject** render at their matrix position as DISABLED
 *   controls with the accurate served-behavior reason (the honest
 *   disabled state, never a dead stub): the Core serves approve/reject
 *   for items in its pending-review state, but no pending_review item is
 *   producible through the public API (no extraction producer exists on
 *   this surface — B2-A1 §2.6 item 4), so no item is currently
 *   reviewable here; the Core's typed review-state refusal
 *   (MEMORY_REVIEW_STATE_CONFLICT) governs any direct attempt. Nothing
 *   is fabricated to produce a pending item, and no invented success
 *   path exists.
 * - **R3 — Edit/correct** renders where the Core's served eligibility
 *   class puts it (B2-A1 §2.3: the active/updated lifecycle states — the
 *   states whose register carries the correction path; eligibility is
 *   Core-owned and the typed MEMORY_NOT_EDITABLE refusal stays
 *   authoritative). The edit is prepared against the item's current
 *   state reference (this Core's served `lifecycleState@updatedAt`
 *   typing — B2-A1 §2.3); a stale reference is the typed MEMORY_CONFLICT
 *   and the recovery is the re-read — never an overwrite. Provenance is
 *   NEVER silently rewritten; the origin label is preserved; the edit
 *   alone never constitutes review approval.
 * - **R4 — Pin/Unpin**: one served member covering both directions; the
 *   control's label renders from the Core-reported `pinned` flag (the
 *   served pin-register position — a flag, never a lifecycle state; the
 *   confirmed flag rides the response's item). Unknown → reconcile by
 *   the item's authoritative read BEFORE a repeat.
 * - **R5/R6 — Forget/Delete**: the served OPEN-distinctness members.
 *   Each renders its own §8.9 confirmation stating the destructive scope
 *   exactly as the Core's contract defines it (the active store entry,
 *   derived references, sync state, backup retention as policy defines)
 *   with NO immediate-total-erasure promise — the Core-confirmed
 *   retention caveat (`retention_note`; empty = none reported) is the
 *   only retention statement ever shown. The forgotten/deleted
 *   distinctness is rendered as OPEN (the Core's confirmed outcome
 *   carries the truth; no label asserts the semantics either way). A
 *   lost destructive response is NEVER assumed to have destroyed — the
 *   keyed re-issue (the same minted M-16 identity) reconciles through
 *   the M-16 replay before any retry (the S0–S4 law). Neither action
 *   rides any other surface's deletion flow (the UI-28 boundary).
 * - **M3 — Per-request inclusion**: the served M-18 member renders as an
 *   explicit decision form (the stated request/scope reference + the
 *   explicit include/exclude labels); the confirmed decision record
 *   renders as a decision record — it changes no item state, applies
 *   ONLY to the stated request, and never widens any scope.
 * - **R7 — Export item**: the served export member, item-scoped (the
 *   exact subset + the item's own scope — the scope never broadens).
 * - After every confirmed mutation the page re-reads the item's
 *   authoritative state (the inspect read — a pure query, never a
 *   mutation); a removed item's re-read is the SAME typed 404 the Core
 *   serves for unknown items (the no-leak rule), rendered honestly with
 *   the back-to-list recovery.
 */

type ItemPhase = "loading" | "data" | "failure" | "unknown";

/** The served eligibility class (B2-A1 §2.3): the lifecycle states whose
 *  register carries the correction path. Eligibility is Core-owned —
 *  this set only positions the Edit affordance; the Core's typed
 *  MEMORY_NOT_EDITABLE refusal stays authoritative either way. */
const EDIT_ELIGIBLE_STATES: readonly string[] = ["active", "updated"];

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function MemoryItemDetailPage() {
  const { memoryItemId = "" } = useParams<{ memoryItemId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");
  const noneReported = t("memory.detail.noneReported");

  const [item, setItem] = useState<MemoryItemServed | null>(null);
  const [itemPhase, setItemPhase] = useState<ItemPhase>("loading");
  const [itemFailure, setItemFailure] = useState<CoreWireEnvelope | null>(null);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);

  // R3 edit dialog state.
  const [editOpen, setEditOpen] = useState(false);
  const [editPhase, setEditPhase] = useState<"editing" | "submitting" | "unknown">("editing");
  const [editFailure, setEditFailure] = useState<CoreWireEnvelope | null>(null);
  const [editUnknownCorrelation, setEditUnknownCorrelation] = useState<string | null>(null);

  // R4 pin state.
  const [pinPhase, setPinPhase] = useState<"idle" | "submitting">("idle");
  const [pinFailure, setPinFailure] = useState<CoreWireEnvelope | null>(null);
  const [pinUnknown, setPinUnknown] = useState(false);

  // R5/R6 destructive state.
  const [destructive, setDestructive] = useState<"forget" | "delete" | null>(null);
  const [destructivePhase, setDestructivePhase] = useState<
    "confirming" | "submitting" | "unknown" | "confirmed"
  >("confirming");
  const [destructiveFailure, setDestructiveFailure] = useState<CoreWireEnvelope | null>(null);
  const [destructiveUnknown, setDestructiveUnknown] = useState<{
    readonly mode: "forget" | "delete";
    readonly itemId: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null>(null);
  const [deleteResult, setDeleteResult] = useState<MemoryDeleteResult | null>(null);
  const [forgetConfirmed, setForgetConfirmed] = useState<{
    readonly lifecycleState: string;
    readonly operationId: string;
  } | null>(null);

  // M3 inclusion state.
  const [inclusionOpen, setInclusionOpen] = useState(false);
  const [inclusionPhase, setInclusionPhase] = useState<"editing" | "submitting" | "done">("editing");
  const [inclusionFailure, setInclusionFailure] = useState<CoreWireEnvelope | null>(null);
  const [inclusionResult, setInclusionResult] = useState<MemoryInclusionResult | null>(null);

  const [exportOpen, setExportOpen] = useState(false);
  const exportTriggerRef = useRef<HTMLButtonElement | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const readItem = useCallback(() => {
    setItemPhase("loading");
    setItemFailure(null);
    setLastConfirmedAt(null);
    void transport.memoryItemGet(memoryItemId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setItem(outcome.data.memory_item);
        setLastConfirmedAt(new Date());
        setItemPhase("data");
      } else if (outcome.kind === "rejection") {
        setItemFailure(outcome.envelope);
        setItemPhase("failure");
      } else {
        setItemPhase("unknown");
      }
    });
  }, [memoryItemId, transport]);

  useEffect(() => {
    readItem();
  }, [readItem]);

  // The authoritative re-read after every confirmed mutation (the
  // inspect read — a pure query). The displayed state updates from the
  // Core's own confirmed state, never from a local assumption.
  const reconcileRead = useCallback(() => {
    void transport.memoryItemGet(memoryItemId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setItem(outcome.data.memory_item);
        setLastConfirmedAt(new Date());
        setItemPhase("data");
      } else if (outcome.kind === "rejection") {
        setItemFailure(outcome.envelope);
        setItemPhase("failure");
      } else {
        setItemPhase("unknown");
      }
    });
  }, [memoryItemId, transport]);

  if (itemPhase === "loading") {
    return (
      <PageLayout>
        <PageHeading>{t("memory.detail.heading")}</PageHeading>
        <p className={styles.note}>{t("memory.list.loading")}</p>
      </PageLayout>
    );
  }

  if (itemPhase === "failure" && itemFailure !== null) {
    return (
      <PageLayout>
        <PageHeading>{t("memory.detail.heading")}</PageHeading>
        <RecoveryPanel
          title={t("memory.detail.failureTitle")}
          body={t(memoryFailureCodeKey(itemFailure.code))}
          note={t(memoryFailureRecoveryKey(itemFailure.code))}
          referenceId={itemFailure.correlationId}
          actions={[
            { label: t("memory.detail.backToList"), onAction: () => window.history.back() },
          ]}
        />
        <p className={styles.code}>
          <span className="visually-hidden">Code: </span>
          <code>{itemFailure.code}</code>
        </p>
      </PageLayout>
    );
  }

  if (itemPhase === "unknown") {
    return (
      <PageLayout>
        <PageHeading>{t("memory.detail.heading")}</PageHeading>
        <RecoveryPanel
          title={t("memory.detail.unknownTitle")}
          body={t("memory.detail.unknownBody")}
          note={t("memory.list.connectionHint")}
          actions={[
            { label: t("memory.detail.refresh"), onAction: readItem },
            { label: t("memory.detail.backToList"), onAction: () => window.history.back() },
          ]}
        />
      </PageLayout>
    );
  }

  // itemPhase === "data" here; item is non-null by construction.
  const shownItem = item as MemoryItemServed;
  const lifecycleChip = memoryLifecycleChip(shownItem.lifecycle_state);
  const LifecycleIcon = lifecycleChip.icon;
  const scopeKey = memoryScopeLabelKey(shownItem.scope);
  const originKey = memoryOriginLabelKey(shownItem.origin);
  const editEligible = EDIT_ELIGIBLE_STATES.includes(shownItem.lifecycle_state);

  const submitEdit = (content: string) => {
    setEditFailure(null);
    setEditUnknownCorrelation(null);
    setEditPhase("submitting");
    const idempotencyKey = mintMemoryOperationKey("memory-edit");
    const correlationId = mintMemoryCorrelationId("memory-edit");
    void transport
      .memoryEdit(memoryItemId, {
        idempotency_key: idempotencyKey,
        correlation_id: correlationId,
        // The prepared-against state reference: this Core's served
        // `lifecycleState@updatedAt` typing (B2-A1 §2.3) over the
        // Core-confirmed item's members; a mismatch is the Core's typed
        // MEMORY_CONFLICT (reconcile by re-reading, never overwrite).
        item_state_ref: memoryItemStateRef(shownItem),
        content,
      })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          setEditOpen(false);
          setEditPhase("editing");
          reconcileRead();
        } else if (outcome.kind === "rejection") {
          setEditFailure(outcome.envelope);
          setEditPhase("editing");
        } else {
          // A lost edit response is unknown — never proof of success or
          // failure; the item's authoritative read reconciles (the
          // item-state half of the lookup) before any retry.
          setEditUnknownCorrelation(correlationId);
          setEditPhase("unknown");
        }
      });
  };

  const submitPin = (pinned: boolean) => {
    setPinFailure(null);
    setPinUnknown(false);
    setPinPhase("submitting");
    void transport
      .memoryPin(memoryItemId, {
        idempotency_key: mintMemoryOperationKey("memory-pin"),
        correlation_id: mintMemoryCorrelationId("memory-pin"),
        pinned,
      })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The confirmed flag rides the response's item; the
          // authoritative re-read confirms it (never a local flip).
          reconcileRead();
          setPinPhase("idle");
        } else if (outcome.kind === "rejection") {
          setPinFailure(outcome.envelope);
          setPinPhase("idle");
        } else {
          // An unknown pin outcome: reconcile by the item's
          // authoritative read BEFORE a repeat (the contract's own law).
          setPinUnknown(true);
          setPinPhase("idle");
        }
      });
  };

  const submitDestructive = (mode: "forget" | "delete", payload: { idempotencyKey: string; correlationId: string }) => {
    setDestructiveFailure(null);
    setDestructivePhase("submitting");
    const body = { idempotency_key: payload.idempotencyKey, correlation_id: payload.correlationId };
    const applyOutcome = (outcome: TransportOutcome<WireMemoryDeleteResult | WireMemoryMutationResult>) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        if ("memory_item_id" in outcome.data) {
          // The delete member's confirmed result: the deletion truth +
          // the Core-confirmed retention caveat.
          setDeleteResult(outcome.data);
          setForgetConfirmed(null);
        } else {
          // The forget member confirms the tombstone post-state: the
          // Core-confirmed item's lifecycle_state is the truth.
          setForgetConfirmed({
            lifecycleState: outcome.data.memory_item.lifecycle_state,
            operationId: outcome.data.operation_id,
          });
          setDeleteResult(null);
        }
        setDestructivePhase("confirmed");
        setDestructiveUnknown(null);
        // The authoritative re-read: a confirmed destructive outcome's
        // item is removed/tombstoned — the re-read surfaces the Core's
        // own served state (a removed item is the SAME typed 404).
        reconcileRead();
      } else if (outcome.kind === "rejection") {
        setDestructiveFailure(outcome.envelope);
        setDestructivePhase("confirming");
      } else {
        // A lost destructive response is NEVER assumed to have
        // destroyed. The minted M-16 identity is kept: the keyed
        // re-issue (the same key + correlation) reconciles through the
        // M-16 replay BEFORE any retry (the S0–S4 law).
        setDestructiveUnknown({ mode, itemId: memoryItemId, idempotencyKey: payload.idempotencyKey, correlationId: payload.correlationId });
        setDestructivePhase("unknown");
      }
    };
    if (mode === "delete") {
      void transport.memoryDelete(memoryItemId, body).then(applyOutcome);
    } else {
      void transport.memoryForget(memoryItemId, body).then(applyOutcome);
    }
  };

  const submitInclusion = (requestRef: string, included: boolean) => {
    setInclusionFailure(null);
    setInclusionPhase("submitting");
    void transport
      .memoryInclusion(memoryItemId, {
        correlation_id: mintMemoryCorrelationId("memory-inclusion"),
        request_ref: requestRef,
        included,
      })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The decision record renders as a decision record — it
          // changes no item state, so no item re-read occurs.
          setInclusionResult(outcome.data);
          setInclusionPhase("done");
        } else if (outcome.kind === "rejection") {
          setInclusionFailure(outcome.envelope);
          setInclusionPhase("editing");
        } else {
          // A lost inclusion response: the decision's record reconciles
          // through the operation-status read; this dialog holds no
          // operation ID from a lost response, so the explained unknown
          // renders and the user re-decides explicitly (the same request
          // identity under a new correlation is the caller's NEW decision
          // record — never a silent duplicate).
          setInclusionFailure(null);
          setInclusionResult(null);
          setInclusionPhase("editing");
        }
      });
  };

  const destructiveMode = destructive;
  const destructiveConfirmed = destructivePhase === "confirmed";

  return (
    <PageLayout>
      <PageHeading>{t("memory.detail.heading")}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("memory.detail.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}

      <section className={styles.section}>
        <h2 className={styles.sectionHeading}>{t("memory.detail.identityHeading")}</h2>
        <dl className={styles.facts}>
          <div>
            <dt>{t("memory.detail.itemId")}</dt>
            <dd>
              <CopyIdButton
                id={shownItem.memory_item_id}
                label={t("memory.list.copyItemId")}
                copiedLabel={t("memory.list.copied")}
                failedLabel={t("memory.list.copyFailed")}
                className={styles.copyId}
              />
            </dd>
          </div>
          <div>
            <dt>{t("memory.list.columnScope")}</dt>
            {/* A scope value renders ONLY as the Core reports it — an
                unknown reported scope renders verbatim, never mapped. */}
            <dd>{scopeKey !== null ? t(scopeKey) : shownItem.scope}</dd>
          </div>
          <div>
            <dt>{t("memory.list.columnOrigin")}</dt>
            {/* The origin is labeled honestly as the Core reports it —
                never relabeled silently (the provenance law). */}
            <dd>{originKey !== null ? t(originKey) : shownItem.origin}</dd>
          </div>
          <div>
            <dt>{t("memory.list.columnState")}</dt>
            <dd>
              <span className={`${styles.chip} ${styles[lifecycleChip.tone]}`}>
                <LifecycleIcon aria-hidden="true" size={14} />
                {t(lifecycleChip.labelKey)}
              </span>
            </dd>
          </div>
          <div>
            <dt>{t("memory.list.columnPinned")}</dt>
            {/* The pin flag renders as the Core returns it — never a
                lifecycle state and never an invented badge. */}
            <dd>{shownItem.pinned ? t("memory.list.pinnedTrue") : t("memory.list.pinnedFalse")}</dd>
          </div>
          <div>
            <dt>{t("memory.detail.createdAt")}</dt>
            <dd>
              <time dateTime={shownItem.created_at}>{formatTimestamp(new Date(shownItem.created_at), locale)}</time>
            </dd>
          </div>
          <div>
            <dt>{t("memory.detail.updatedAt")}</dt>
            <dd>
              <time dateTime={shownItem.updated_at}>{formatTimestamp(new Date(shownItem.updated_at), locale)}</time>
            </dd>
          </div>
          <div>
            <dt>{t("memory.detail.lastUsedAt")}</dt>
            <dd>
              {shownItem.last_used_at !== undefined && shownItem.last_used_at !== "" ? (
                <time dateTime={shownItem.last_used_at}>
                  {formatTimestamp(new Date(shownItem.last_used_at), locale)}
                </time>
              ) : (
                <span className={styles.unavailable}>{unavailable}</span>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("memory.detail.lastConfirmedAt")}</dt>
            <dd>
              {shownItem.last_confirmed_at !== undefined && shownItem.last_confirmed_at !== "" ? (
                <time dateTime={shownItem.last_confirmed_at}>
                  {formatTimestamp(new Date(shownItem.last_confirmed_at), locale)}
                </time>
              ) : (
                <span className={styles.unavailable}>{unavailable}</span>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("memory.detail.creationMethod")}</dt>
            <dd>{fieldOrUnavailable(shownItem.creation_method, unavailable)}</dd>
          </div>
          <div>
            <dt>{t("memory.detail.confidence")}</dt>
            <dd>{fieldOrUnavailable(shownItem.confidence, unavailable)}</dd>
          </div>
          <div>
            <dt>{t("memory.detail.retentionTtlRef")}</dt>
            {/* The retention/TTL class renders as the Core reports it —
                expiry is never UI-computed and stays DISTINCT from
                forgotten and deleted. */}
            <dd>{fieldOrUnavailable(shownItem.retention_ttl_ref, unavailable)}</dd>
          </div>
          <div>
            <dt>{t("memory.detail.untrustedLabel")}</dt>
            {/* `untrusted_label` is never set on this surface (no
                imported-origin producer — B2-A1 §2.6 item 10); the
                absence renders Unavailable — no invented badge. */}
            <dd>{fieldOrUnavailable(shownItem.untrusted_label, unavailable)}</dd>
          </div>
        </dl>
      </section>

      <section className={styles.section}>
        <h2 className={styles.sectionHeading}>{t("memory.detail.contentHeading")}</h2>
        <p className={styles.contentBlock}>{shownItem.content}</p>
        <p className={styles.actionReason}>{t("memory.detail.secretsLawNote")}</p>
      </section>

      <section className={styles.section}>
        <h2 className={styles.sectionHeading}>{t("memory.detail.provenanceHeading")}</h2>
        {shownItem.originating_refs !== undefined && shownItem.originating_refs.length > 0 ? (
          <ul className={styles.refList}>
            {shownItem.originating_refs.map((ref) => (
              <li key={ref}>{ref}</li>
            ))}
          </ul>
        ) : (
          <p className={styles.note}>{noneReported}</p>
        )}
        {/* The source event is shown, NEVER rewritten; provenance is
            never silently rewritten by an edit (R3's law). */}
        <p className={styles.actionReason}>{t("memory.detail.provenanceNote")}</p>
      </section>

      {shownItem.conflict_note !== undefined && shownItem.conflict_note !== "" ? (
        <section className={styles.section}>
          <h2 className={styles.sectionHeading}>{t("memory.detail.conflictHeading")}</h2>
          {/* The conflict/freshness warning renders ONLY as the Core
              returns it — never invented (R1's law). */}
          <p>{shownItem.conflict_note}</p>
        </section>
      ) : null}

      <section className={styles.section}>
        <h2 className={styles.sectionHeading}>{t("memory.detail.actionsHeading")}</h2>
        <div className={styles.actions}>
          {/* R4 — Pin/Unpin: one served member covering both directions;
              the label renders from the Core-reported pinned flag. */}
          {shownItem.pinned ? (
            <button
              type="button"
              className={styles.control}
              onClick={() => submitPin(false)}
              disabled={pinPhase === "submitting"}
            >
              <PinOff aria-hidden="true" size={14} /> {t("memory.detail.unpin")}
            </button>
          ) : (
            <button
              type="button"
              className={styles.control}
              onClick={() => submitPin(true)}
              disabled={pinPhase === "submitting"}
            >
              <Pin aria-hidden="true" size={14} /> {t("memory.detail.pin")}
            </button>
          )}

          {/* R3 — Edit/correct: positioned by the Core's served
              eligibility class (active/updated); the Core's typed
              MEMORY_NOT_EDITABLE refusal stays authoritative. */}
          {editEligible ? (
            <button
              type="button"
              className={styles.control}
              onClick={() => {
                setEditOpen(true);
                setEditPhase("editing");
                setEditFailure(null);
                setEditUnknownCorrelation(null);
              }}
            >
              {t("memory.detail.edit")}
            </button>
          ) : null}

          {/* M3 — the per-request inclusion decision (the served M-18
              member): an explicit decision form, never a global toggle. */}
          <button
            type="button"
            className={styles.control}
            onClick={() => {
              setInclusionOpen(true);
              setInclusionPhase("editing");
              setInclusionFailure(null);
              setInclusionResult(null);
            }}
          >
            {t("memory.detail.inclusion")}
          </button>

          {/* R7 — Export item: the served export member, item-scoped. */}
          <button
            type="button"
            ref={exportTriggerRef}
            className={styles.control}
            onClick={() => setExportOpen(true)}
          >
            {t("memory.detail.exportItem")}
          </button>

          {/* R5/R6 — Forget/Delete: the served OPEN-distinctness members,
              each with its own §8.9 confirmation. */}
          <button
            type="button"
            className={styles.dangerControl}
            onClick={() => {
              setDestructive("forget");
              setDestructivePhase("confirming");
              setDestructiveFailure(null);
              setDestructiveUnknown(null);
              setDeleteResult(null);
            }}
            disabled={destructive !== null && destructivePhase === "submitting"}
          >
            {t("memory.detail.forget")}
          </button>
          <button
            type="button"
            className={styles.dangerControl}
            onClick={() => {
              setDestructive("delete");
              setDestructivePhase("confirming");
              setDestructiveFailure(null);
              setDestructiveUnknown(null);
              setDeleteResult(null);
            }}
            disabled={destructive !== null && destructivePhase === "submitting"}
          >
            {t("memory.detail.delete")}
          </button>
        </div>
        {pinFailure !== null ? (
          <div>
            <RecoveryPanel
              title={t("memory.detail.pinFailureTitle")}
              body={t(memoryFailureCodeKey(pinFailure.code))}
              note={t(memoryFailureRecoveryKey(pinFailure.code))}
              referenceId={pinFailure.correlationId}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{pinFailure.code}</code>
            </p>
          </div>
        ) : null}
        {pinUnknown ? (
          <>
            <RecoveryPanel
              title={t("memory.detail.pinUnknownTitle")}
              body={t("memory.detail.pinUnknownBody")}
            />
            <div className={styles.actions}>
              <button type="button" className={styles.control} onClick={reconcileRead}>
                {t("memory.detail.reconcileItem")}
              </button>
            </div>
            <p className={styles.actionReason}>{t("memory.detail.pinUnknownNote")}</p>
          </>
        ) : null}

        {/* R2 — Approve/Reject at their matrix position: the honest
            disabled state with the accurate served-behavior reason. The
            Core serves the operations; no pending_review item is
            producible through the public API (no extraction producer —
            B2-A1 §2.6 item 4), so no item is reviewable here; the Core's
            typed review-state refusal governs any direct attempt. */}
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="memory-review-disabled-reason"
          >
            {t("memory.detail.approve")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled
            aria-describedby="memory-review-disabled-reason"
          >
            {t("memory.detail.reject")}
          </button>
        </div>
        <p id="memory-review-disabled-reason" className={styles.actionReason}>
          {t("memory.detail.reviewDisabledReason")}
        </p>
        <p className={styles.actionReason}>{t("memory.detail.reviewDistinctNote")}</p>
      </section>

      <EditDialog
        open={editOpen}
        onClose={() => {
          setEditOpen(false);
          setEditPhase("editing");
        }}
        onSubmit={submitEdit}
        phase={editPhase}
        failure={editFailure}
        unknownCorrelation={editUnknownCorrelation}
        onReconcile={() => {
          setEditOpen(false);
          setEditPhase("editing");
          reconcileRead();
        }}
      />

      <DestructiveDialog
        mode={destructiveMode}
        phase={destructivePhase}
        onClose={() => {
          setDestructive(null);
          setDestructivePhase("confirming");
          setDestructiveFailure(null);
          setDestructiveUnknown(null);
          setDeleteResult(null);
          setForgetConfirmed(null);
        }}
        onConfirm={(mode, payload) => submitDestructive(mode, payload)}
        failure={destructiveFailure}
        unknown={destructiveUnknown}
        deleteResult={destructiveConfirmed ? deleteResult : null}
        forgetConfirmed={destructiveConfirmed ? forgetConfirmed : null}
      />

      <InclusionDialog
        open={inclusionOpen}
        onClose={() => setInclusionOpen(false)}
        phase={inclusionPhase}
        failure={inclusionFailure}
        result={inclusionResult}
        onSubmit={submitInclusion}
      />

      <MemoryExportDialog
        open={exportOpen}
        onClose={() => {
          setExportOpen(false);
          exportTriggerRef.current?.focus();
        }}
        presetScope={shownItem.scope}
        itemIds={[shownItem.memory_item_id]}
      />
    </PageLayout>
  );
}

/** The R3 edit dialog: the corrected content prepared against the item's
 *  current state reference; provenance-preserving by the Core's own
 *  semantics — the origin label is preserved, the originating references
 *  are never silently rewritten, and the edit alone never constitutes
 *  review approval. */
function EditDialog(props: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onSubmit: (content: string) => void;
  readonly phase: "editing" | "submitting" | "unknown";
  readonly failure: CoreWireEnvelope | null;
  readonly unknownCorrelation: string | null;
  readonly onReconcile: () => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const [content, setContent] = useState("");

  useEffect(() => {
    if (props.open) setContent("");
  }, [props.open]);

  return (
    <Dialog labelId={labelId} title={t("memory.edit.title")} open={props.open} onClose={props.onClose}>
      {props.phase === "editing" ? (
        <div>
          <label className={dialogStyles.fieldLabel} htmlFor="memory-edit-content">
            {t("memory.edit.contentLabel")}
          </label>
          <textarea
            id="memory-edit-content"
            className={dialogStyles.fieldTextarea}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            required
          />
          <section className={dialogStyles.outcomeBlock}>
            <p className={dialogStyles.outcomeTitle}>{t("memory.edit.preservesHeading")}</p>
            <p>{t("memory.edit.preservesValue")}</p>
          </section>
          <section className={dialogStyles.outcomeBlock}>
            <p className={dialogStyles.outcomeTitle}>{t("memory.edit.secretsHeading")}</p>
            <p>{t("memory.edit.secretsValue")}</p>
          </section>
          {props.failure !== null ? (
            <div>
              <RecoveryPanel
                title={t("memory.edit.failureTitle")}
                body={t(memoryFailureCodeKey(props.failure.code))}
                note={t(memoryFailureRecoveryKey(props.failure.code))}
                referenceId={props.failure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{props.failure.code}</code>
              </p>
            </div>
          ) : null}
          <div className={dialogStyles.actions}>
            <button
              type="button"
              className={dialogStyles.primaryControl}
              onClick={() => props.onSubmit(content)}
              disabled={content.trim() === ""}
            >
              {t("memory.edit.submit")}
            </button>
          </div>
        </div>
      ) : null}
      {props.phase === "submitting" ? (
        <p className={dialogStyles.fieldNote}>{t("memory.edit.submitting")}</p>
      ) : null}
      {props.phase === "unknown" ? (
        <div>
          <RecoveryPanel
            title={t("memory.edit.unknownTitle")}
            body={t("memory.edit.unknownBody", {
              correlationId: props.unknownCorrelation ?? "",
            })}
          />
          <p className={dialogStyles.fieldNote}>{t("memory.edit.unknownReconcileNote")}</p>
          <div className={dialogStyles.actions}>
            <button type="button" className={dialogStyles.primaryControl} onClick={props.onReconcile}>
              {t("memory.detail.reconcileItem")}
            </button>
          </div>
        </div>
      ) : null}
      <div className={dialogStyles.actions}>
        <button type="button" className={dialogStyles.control} onClick={props.onClose}>
          {t("actions.close")}
        </button>
      </div>
    </Dialog>
  );
}

/** The R5/R6 destructive confirmation dialog: the §8.9 confirmation
 *  stating the destructive scope exactly as the Core's contract defines
 *  it, the OPEN distinctness carried in copy, the keyed identity minted
 *  per confirmation, and the Core-confirmed result with its retention
 *  caveat (never a total-erasure promise). */
function DestructiveDialog(props: {
  readonly mode: "forget" | "delete" | null;
  readonly phase: "confirming" | "submitting" | "unknown" | "confirmed";
  readonly onClose: () => void;
  readonly onConfirm: (
    mode: "forget" | "delete",
    payload: { idempotencyKey: string; correlationId: string },
  ) => void;
  readonly failure: CoreWireEnvelope | null;
  readonly unknown: {
    readonly mode: "forget" | "delete";
    readonly itemId: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null;
  readonly deleteResult: MemoryDeleteResult | null;
  readonly forgetConfirmed: {
    readonly lifecycleState: string;
    readonly operationId: string;
  } | null;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const mode = props.mode;

  const outcomeChip = props.deleteResult !== null ? memoryLifecycleChip(props.deleteResult.outcome_state) : null;
  const OutcomeIcon = outcomeChip?.icon;
  const forgetChip = props.forgetConfirmed !== null ? memoryLifecycleChip(props.forgetConfirmed.lifecycleState) : null;
  const ForgetIcon = forgetChip?.icon;

  const confirm = () => {
    if (mode === null) return;
    props.onConfirm(mode, {
      idempotencyKey: mintMemoryOperationKey(`memory-${mode}`),
      correlationId: mintMemoryCorrelationId(`memory-${mode}`),
    });
  };

  return (
    <Dialog
      labelId={labelId}
      title={mode === "delete" ? t("memory.destructive.deleteTitle") : t("memory.destructive.forgetTitle")}
      open={mode !== null}
      onClose={props.onClose}
    >
      {mode !== null && props.phase === "confirming" ? (
        <div>
          <section className={dialogStyles.confirmBlock}>
            <p className={dialogStyles.confirmTitle}>
              {mode === "delete" ? t("memory.destructive.deleteConfirmTitle") : t("memory.destructive.forgetConfirmTitle")}
            </p>
            <p>{t("memory.destructive.scopeStatement")}</p>
            <p>{t("memory.destructive.retentionStance")}</p>
            <p>{t("memory.destructive.openDistinctnessNote")}</p>
          </section>
          {props.failure !== null ? (
            <div>
              <RecoveryPanel
                title={t("memory.destructive.failureTitle")}
                body={t(memoryFailureCodeKey(props.failure.code))}
                note={t(memoryFailureRecoveryKey(props.failure.code))}
                referenceId={props.failure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{props.failure.code}</code>
              </p>
            </div>
          ) : null}
          <div className={dialogStyles.actions}>
            <button type="button" className={dialogStyles.dangerControl} onClick={confirm}>
              {mode === "delete"
                ? t("memory.destructive.deleteConfirm")
                : t("memory.destructive.forgetConfirm")}
            </button>
            <button type="button" className={dialogStyles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      ) : null}
      {mode !== null && props.phase === "submitting" ? (
        <p className={dialogStyles.fieldNote}>{t("memory.destructive.submitting")}</p>
      ) : null}

      {mode === "delete" &&
      props.phase === "confirmed" &&
      props.deleteResult !== null &&
      outcomeChip !== null &&
      OutcomeIcon !== undefined ? (
        <div>
          <section className={dialogStyles.outcomeBlock}>
            <p className={dialogStyles.outcomeTitle}>{t("memory.destructive.deleteConfirmedTitle")}</p>
            <dl className={dialogStyles.reviewFacts}>
              <div>
                <dt>{t("memory.destructive.outcomeState")}</dt>
                <dd>
                  <span className={`${dialogStyles.chip} ${dialogStyles[outcomeChip.tone]}`}>
                    <OutcomeIcon aria-hidden="true" size={14} />
                    {t(outcomeChip.labelKey)}
                  </span>
                </dd>
              </div>
              <div>
                <dt>{t("memory.destructive.retentionNote")}</dt>
                {/* The Core-confirmed retention caveat renders as the
                    Core returns it — empty = none reported; never a
                    total-erasure promise. */}
                <dd>
                  {props.deleteResult.retention_note !== undefined && props.deleteResult.retention_note !== ""
                    ? props.deleteResult.retention_note
                    : t("memory.detail.noneReported")}
                </dd>
              </div>
              <div>
                <dt>{t("memory.destructive.operationId")}</dt>
                <dd>
                  <code>{props.deleteResult.operation_id}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={dialogStyles.actions}>
            <button type="button" className={dialogStyles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {mode === "forget" &&
      props.phase === "confirmed" &&
      props.forgetConfirmed !== null &&
      forgetChip !== null &&
      ForgetIcon !== undefined ? (
        <div>
          <section className={dialogStyles.outcomeBlock}>
            <p className={dialogStyles.outcomeTitle}>{t("memory.destructive.forgetConfirmedTitle")}</p>
            <dl className={dialogStyles.reviewFacts}>
              <div>
                <dt>{t("memory.destructive.outcomeState")}</dt>
                <dd>
                  <span className={`${dialogStyles.chip} ${dialogStyles[forgetChip.tone]}`}>
                    <ForgetIcon aria-hidden="true" size={14} />
                    {t(forgetChip.labelKey)}
                  </span>
                </dd>
              </div>
              <div>
                <dt>{t("memory.destructive.operationId")}</dt>
                <dd>
                  <code>{props.forgetConfirmed.operationId}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={dialogStyles.actions}>
            <button type="button" className={dialogStyles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "unknown" && props.unknown !== null ? (
        <div>
          <RecoveryPanel
            title={t("memory.destructive.unknownTitle")}
            body={t("memory.destructive.unknownBody", { correlationId: props.unknown.correlationId })}
          />
          <p className={dialogStyles.fieldNote}>{t("memory.destructive.unknownReconcileNote")}</p>
          <div className={dialogStyles.actions}>
            <button
              type="button"
              className={dialogStyles.primaryControl}
              onClick={() => {
                const u = props.unknown;
                if (u === null) return;
                props.onConfirm(u.mode, {
                  idempotencyKey: u.idempotencyKey,
                  correlationId: u.correlationId,
                });
              }}
            >
              {t("memory.destructive.unknownReissue")}
            </button>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}

/** The M3 inclusion dialog: the explicit per-request decision form — the
 *  stated request/scope reference + the explicit include/exclude labels;
 *  the confirmed record renders as a decision record (no item-state
 *  change, only the stated request's scope, no global effect). */
function InclusionDialog(props: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly phase: "editing" | "submitting" | "done";
  readonly failure: CoreWireEnvelope | null;
  readonly result: MemoryInclusionResult | null;
  readonly onSubmit: (requestRef: string, included: boolean) => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const [requestRef, setRequestRef] = useState("");
  const [included, setIncluded] = useState<"true" | "false">("true");

  useEffect(() => {
    if (props.open) {
      setRequestRef("");
      setIncluded("true");
    }
  }, [props.open]);

  return (
    <Dialog labelId={labelId} title={t("memory.inclusion.title")} open={props.open} onClose={props.onClose}>
      {props.phase === "editing" ? (
        <div>
          <label className={dialogStyles.fieldLabel} htmlFor="memory-inclusion-request-ref">
            {t("memory.inclusion.requestRefLabel")}
          </label>
          <input
            id="memory-inclusion-request-ref"
            type="text"
            className={dialogStyles.fieldInput}
            value={requestRef}
            onChange={(event) => setRequestRef(event.target.value)}
            required
          />
          <p className={dialogStyles.fieldNote}>{t("memory.inclusion.requestRefNote")}</p>

          <fieldset className={dialogStyles.radioGroup}>
            <legend className={dialogStyles.fieldLabel}>{t("memory.inclusion.includedLabel")}</legend>
            <label className={dialogStyles.radioOption} htmlFor="memory-inclusion-included-true">
              <input
                id="memory-inclusion-included-true"
                type="radio"
                name="memory-inclusion-included"
                checked={included === "true"}
                onChange={() => setIncluded("true")}
              />
              {t("memory.inclusion.includedTrue")}
            </label>
            <label className={dialogStyles.radioOption} htmlFor="memory-inclusion-included-false">
              <input
                id="memory-inclusion-included-false"
                type="radio"
                name="memory-inclusion-included"
                checked={included === "false"}
                onChange={() => setIncluded("false")}
              />
              {t("memory.inclusion.includedFalse")}
            </label>
          </fieldset>
          <p className={dialogStyles.fieldNote}>{t("memory.inclusion.scopeNote")}</p>

          {props.failure !== null ? (
            <div>
              <RecoveryPanel
                title={t("memory.inclusion.failureTitle")}
                body={t(memoryFailureCodeKey(props.failure.code))}
                note={t(memoryFailureRecoveryKey(props.failure.code))}
                referenceId={props.failure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{props.failure.code}</code>
              </p>
            </div>
          ) : null}

          <div className={dialogStyles.actions}>
            <button
              type="button"
              className={dialogStyles.primaryControl}
              onClick={() => props.onSubmit(requestRef.trim(), included === "true")}
              disabled={requestRef.trim() === ""}
            >
              {t("memory.inclusion.submit")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "submitting" ? (
        <p className={dialogStyles.fieldNote} role="status">
          {t("memory.inclusion.submitting")}
        </p>
      ) : null}

      {props.phase === "done" && props.result !== null ? (
        <div>
          <section className={dialogStyles.outcomeBlock}>
            <p className={dialogStyles.outcomeTitle}>{t("memory.inclusion.confirmedTitle")}</p>
            <dl className={dialogStyles.reviewFacts}>
              <div>
                <dt>{t("memory.inclusion.resultRequestRef")}</dt>
                <dd>
                  <code>{props.result.request_ref}</code>
                </dd>
              </div>
              <div>
                <dt>{t("memory.inclusion.resultIncluded")}</dt>
                <dd>{props.result.included ? t("memory.inclusion.includedTrue") : t("memory.inclusion.includedFalse")}</dd>
              </div>
              <div>
                <dt>{t("memory.destructive.operationId")}</dt>
                <dd>
                  <code>{props.result.operation_id}</code>
                </dd>
              </div>
            </dl>
            <p className={dialogStyles.fieldNote}>{t("memory.inclusion.confirmedNote")}</p>
          </section>
          <div className={dialogStyles.actions}>
            <button type="button" className={dialogStyles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
