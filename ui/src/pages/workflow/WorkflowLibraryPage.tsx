import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, WorkflowCreateResult, WorkflowDefinition } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./WorkflowLibraryPage.module.css";
import { CopyIdButton, fieldOrUnavailable, mintWorkflowCorrelationId, mintWorkflowCreateKey } from "./workflowStates";

/**
 * `/workflows` — the Workflow list (B4-U1; the DE-0 queue row 7, the
 * P4.1 W1–W6 rows against the now-live B4 adapter; the creation control
 * wired by B4-U2 per the UI-30 gate resolution, GR-C2 — the GR-0 queue
 * row 4).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/workflows?limit=&cursor=` (the published list member's
 *   serving, B4-A1 §2.1 #1). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented).
 * - The columns render Core-reported values only (P4.5 §3.3's map law):
 *   the name renders as the Core's display_name where one exists (an
 *   untitled definition is stated honestly, never renamed); the current
 *   revision renders as the Core's current_revision_ref where one exists
 *   (a definition with no saved revision is stated as exactly that —
 *   never inferred); the recorded W1 run/status cells render Unavailable
 *   with the page's note — the definition record carries no status
 *   member, and no run data exists while the D-40 gate withholds Run
 *   execution (never zero, never inferred).
 * - W2 (`New workflow`) is LIVE per the UI-30 resolution (GR-C2): the
 *   aggregate-create class (B4-A1 §2.1 #2) — the §8.9-style two-step
 *   dialog states the scope and the keyed-replay law, then the explicit
 *   confirm action submits the keyed body (`idempotency_key` +
 *   `correlation_id` + `display_name`; a fresh pair per NEW attempt, the
 *   72 h replay window). THE CORE-MINTS LAW: the body never carries a
 *   definition identity — the Core mints it, and the confirmed state
 *   renders the minted identity as received with the honest `replayed`
 *   datum. A lost response is the explained uncertain state with the
 *   retained request reference; reconciliation goes ONLY through the
 *   authoritative list read (the S0–S4 law) — never a blind retry, never
 *   a duplicate create.
 * - W3 (`Import`) stays ABSENT: the Core serves no workflow-import
 *   member (the row is "shown only if supported") — the absence is the
 *   honest state, never a placeholder form.
 * - W4 row select navigates the ONE canonical detail route (X-2); row
 *   identity is the Core-minted definition ID. A definition's content is
 *   user-authored data — the identity rides the path; the content never
 *   does.
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed refusal surfaced verbatim, or the truthful unknown (a
 *   lost or uninterpretable response — never shown as an empty list). No
 *   fabricated rows, counts, totals, or optimistic states exist here.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

/** The create flow's phases (the B2 destructive-dialog idiom, the B5-U2
 *  install class): the naming step, the §8.9-style confirm step, the
 *  in-flight submission, the Core-confirmed outcome (the honest
 *  `replayed` flag rendered where the Core reports it), the explained
 *  uncertain outcome (OC-1), and the typed refusal. */
type CreatePhase = "naming" | "confirming" | "submitting" | "confirmed" | "unknown" | "failure";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function WorkflowLibraryPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const navigate = useNavigate();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly WorkflowDefinition[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  // The create flow (B4-U2; the aggregate-create class per the UI-30
  // resolution): the display name is the user's own input (the body's
  // single authored member); the §8.9-style confirmation is two-step (the
  // scope statement, then the explicit confirm action); the OC-1 unknown
  // outcome carries the minted request reference and reconciles through
  // the authoritative list read — never a blind retry, never a duplicate
  // create.
  const [createOpen, setCreateOpen] = useState(false);
  const [createPhase, setCreatePhase] = useState<CreatePhase>("naming");
  const [createName, setCreateName] = useState("");
  const [createInputError, setCreateInputError] = useState(false);
  const [createResult, setCreateResult] = useState<WorkflowCreateResult | null>(null);
  const [createFailure, setCreateFailure] = useState<CoreWireEnvelope | null>(null);
  const [createUnknown, setCreateUnknown] = useState<{
    readonly displayName: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null>(null);
  const [createReconcileFailed, setCreateReconcileFailed] = useState(false);
  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a refresh supersedes an in-flight read instead of being
  // dropped; nothing is ever applied twice.
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

      const params: { cursor?: string } = {};
      if (options.cursor !== undefined) params.cursor = options.cursor;

      void transport.workflowList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) =>
            options.append ? [...prev, ...outcome.data.definitions] : outcome.data.definitions,
          );
          // A response without the cursor key IS the final page: null
          // ends pagination (the coded end marker — no has_more exists).
          setNextCursor(outcome.data.next_cursor ?? null);
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
    [transport],
  );

  // The first read on mount.
  useEffect(() => {
    setRows([]);
    setNextCursor(null);
    setFailure(null);
    setLastConfirmedAt(null);
    runRead({ append: false, markRefreshing: false });
  }, [runRead]);

  const loadMore = () => {
    if (nextCursor === null || loadingMore || refreshing) return;
    setLoadingMore(true);
    runRead({ append: true, cursor: nextCursor, markRefreshing: false });
  };

  const refresh = useCallback(() => {
    if (refreshing || loadingMore) return;
    setFailure(null);
    // A refresh keeps the confirmed rows rendered (stale-labeled by the
    // last-confirmed line) and re-reads the FIRST page.
    runRead({ append: false, markRefreshing: true });
  }, [loadingMore, refreshing, runRead]);

  // The create flow's steps. The §8.9-style first step (the name form)
  // opens the dialog; the second step (the scope confirm) mints the keyed
  // pair HERE — a fresh identity per NEW attempt (the Core mints the
  // definition identity; the body never carries one). A lost response is
  // `unknown`: the minted pair is retained as the request reference and
  // the reconcile action goes through the authoritative list read — NO
  // blind retry is ever offered (the S0–S4 law).
  const openCreate = () => {
    setCreateName("");
    setCreateInputError(false);
    setCreateResult(null);
    setCreateFailure(null);
    setCreateUnknown(null);
    setCreateReconcileFailed(false);
    setCreatePhase("naming");
    setCreateOpen(true);
  };

  const openCreateConfirm = () => {
    if (createName.trim() === "") {
      setCreateInputError(true);
      return;
    }
    setCreateInputError(false);
    setCreateResult(null);
    setCreateFailure(null);
    setCreateUnknown(null);
    setCreateReconcileFailed(false);
    setCreatePhase("confirming");
  };

  const closeCreate = useCallback(() => {
    // Escape/Close cancels the flow — no implied create (the review-flow
    // escape law's class).
    setCreateOpen(false);
    setCreatePhase("naming");
    setCreateResult(null);
    setCreateFailure(null);
    setCreateUnknown(null);
    setCreateReconcileFailed(false);
  }, []);

  const confirmCreate = useCallback(() => {
    const displayName = createName.trim();
    if (displayName === "") {
      setCreateInputError(true);
      return;
    }
    // The keyed pair (the 72 h replay class): a fresh identity per NEW
    // attempt; a lost outcome's reconcile re-issues the SAME pair (the
    // keyed replay returns the recorded create — never a duplicate).
    const idempotencyKey = mintWorkflowCreateKey();
    const correlationId = mintWorkflowCorrelationId("workflow-create");
    setCreatePhase("submitting");
    void transport
      .workflowCreate({ idempotency_key: idempotencyKey, correlation_id: correlationId, display_name: displayName })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The Core-confirmed create: the definition is the Core-minted
          // record rendered as received; the honest `replayed` datum
          // renders where the Core reports it.
          setCreateResult(outcome.data);
          setCreatePhase("confirmed");
          // The authoritative re-read: the new definition appears in the
          // list as the Core reports it.
          runRead({ append: false, markRefreshing: false });
        } else if (outcome.kind === "rejection") {
          setCreateFailure(outcome.envelope);
          setCreatePhase("failure");
        } else {
          // OC-1: the uncertain outcome renders with the request
          // reference; reconcile through the authoritative read — never
          // a blind resubmit.
          setCreateUnknown({ displayName, idempotencyKey, correlationId });
          setCreatePhase("unknown");
        }
      });
  }, [createName, runRead, transport]);

  // OC-1's reconciliation: the served reconcile surface here is the
  // authoritative user-facing list read (the created definition's
  // identity resolves through it). A usable answer applies the fresh list
  // (the truth the user sees) and closes the uncertain state; a read that
  // did not land a usable answer keeps the explained state with the
  // re-query affordance (OC-8) — never a blind resubmit of the create.
  const reconcileCreate = useCallback(() => {
    setCreateReconcileFailed(false);
    const seq = ++readSeqRef.current;
    setRefreshing(true);
    void transport.workflowList({}).then((outcome) => {
      if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
      setRefreshing(false);
      setLoadingMore(false);
      if (outcome.kind === "data") {
        setRows(outcome.data.definitions);
        setNextCursor(outcome.data.next_cursor ?? null);
        setLastConfirmedAt(new Date());
        setListPhase("data");
        closeCreate();
      } else {
        setCreateReconcileFailed(true);
      }
    });
  }, [closeCreate, transport]);

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("workflow.list.loading");
      case "failure":
        return t("workflow.list.failureTitle");
      case "unknown":
        return t("workflow.list.unknownTitle");
      default:
        return rows.length === 0 ? t("workflow.list.emptyAll") : t("workflow.list.heading");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("workflow.list.heading")}</PageHeading>
      <p className={styles.description}>{t("workflow.list.description")}</p>
      {/* W6 — the surface gate state (the accurate recorded reason): the
          reads and the creation control render; the editing surface waits
          for the Core's node-type catalogue; no import member exists; no
          Run execution exists (the D-40 remainder). */}
      <p className={styles.gateNote}>{t("workflow.list.gateNote")}</p>
      <div className={styles.toolbar}>
        {/* W2 — the creation control (LIVE per the UI-30 resolution,
            GR-C2): the §8.9-style two-step dialog follows. */}
        <button type="button" className={styles.control} onClick={openCreate}>
          {t("workflow.list.create.control")}
        </button>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("workflow.list.refreshing") : t("workflow.list.refresh")}
        </button>
      </div>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("workflow.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("workflow.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>{t("workflow.list.emptyAll")}</p>
            <p>{t("workflow.list.emptyAllBody")}</p>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <caption className="visually-hidden">{t("workflow.list.description")}</caption>
              <thead>
                <tr>
                  <th scope="col">{t("workflow.list.columnDefinition")}</th>
                  <th scope="col">{t("workflow.list.columnDefinitionId")}</th>
                  <th scope="col">{t("workflow.list.columnRevision")}</th>
                  <th scope="col">{t("workflow.list.columnLastRun")}</th>
                  <th scope="col">{t("workflow.list.columnStatus")}</th>
                  <th scope="col">{t("workflow.list.columnCreated")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((definition) => (
                  <WorkflowRow key={definition.definition_id} definition={definition} />
                ))}
              </tbody>
            </table>
            <p className={styles.tableNote}>{t("workflow.list.runStatusNote")}</p>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? t("workflow.list.loadingMore") : t("workflow.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("workflow.list.failureTitle")}
            body={t("workflow.list.failureBody")}
            note={t("workflow.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("workflow.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("workflow.list.unknownTitle")}
          body={t("workflow.list.unknownBody")}
          note={t("workflow.list.connectionHint")}
          actions={[{ label: t("workflow.list.refresh"), onAction: refresh }]}
        />
      ) : null}

      <CreateDialog
        open={createOpen}
        phase={createPhase}
        displayName={createName.trim()}
        inputError={createInputError}
        result={createResult}
        failure={createFailure}
        unknown={createUnknown}
        reconcileFailed={createReconcileFailed}
        onClose={closeCreate}
        onNameChange={setCreateName}
        onConfirmStep={openCreateConfirm}
        onConfirm={confirmCreate}
        onReconcile={reconcileCreate}
        onOpenCreated={(definitionId) => {
          closeCreate();
          navigate(`/workflows/${encodeURIComponent(definitionId)}`);
        }}
      />
    </PageLayout>
  );
}

/** The create dialog (the §8.9-style two-step confirmation; the B2
 *  destructive-dialog idiom; the B5-U2 install-dialog class): step one
 *  takes the display name (the body's single authored member), step two
 *  states the scope and the keyed-replay law exactly, and the explicit
 *  confirm action submits the keyed body. Escape and Close cancel with no
 *  implied create. The uncertain outcome renders the explained state with
 *  the request reference and the reconcile affordance (OC-1/OC-8) — never
 *  a blind retry, never an optimistic "Created". The confirmed outcome
 *  renders the Core-minted identity as received (the Core-mints law) with
 *  the honest `replayed` datum. */
function CreateDialog(props: {
  readonly open: boolean;
  readonly phase: CreatePhase;
  readonly displayName: string;
  readonly inputError: boolean;
  readonly result: WorkflowCreateResult | null;
  readonly failure: CoreWireEnvelope | null;
  readonly unknown: {
    readonly displayName: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null;
  readonly reconcileFailed: boolean;
  readonly onClose: () => void;
  readonly onNameChange: (value: string) => void;
  readonly onConfirmStep: () => void;
  readonly onConfirm: () => void;
  readonly onReconcile: () => void;
  readonly onOpenCreated: (definitionId: string) => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const nameFieldId = "workflow-create-name";
  const nameErrorId = "workflow-create-name-error";

  return (
    <Dialog labelId={labelId} title={t("workflow.list.create.nameHeading")} open={props.open} onClose={props.onClose}>
      {props.phase === "naming" ? (
        <div>
          <div className={styles.field}>
            <label htmlFor={nameFieldId}>{t("workflow.list.create.nameLabel")}</label>
            <input
              id={nameFieldId}
              type="text"
              value={props.displayName}
              onChange={(event) => props.onNameChange(event.target.value)}
              aria-invalid={props.inputError}
              aria-describedby={props.inputError ? nameErrorId : undefined}
            />
            {props.inputError ? (
              <p id={nameErrorId} className={styles.actionReason}>
                {t("workflow.list.create.nameRequired")}
              </p>
            ) : null}
          </div>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onConfirmStep}>
              {t("workflow.list.create.nameStepAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "confirming" ? (
        <div>
          <section className={styles.confirmBlock}>
            <p className={styles.outcomeTitle}>{t("workflow.list.create.confirmTitle")}</p>
            <p>{t("workflow.list.create.confirmScope")}</p>
            <p>{t("workflow.list.create.confirmLaws")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("workflow.list.create.nameLabel")}</dt>
                <dd>
                  <code>{props.displayName}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onConfirm}>
              {t("workflow.list.create.confirmAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "submitting" ? <p className={styles.fieldNote}>{t("workflow.list.create.submitting")}</p> : null}

      {props.phase === "confirmed" && props.result !== null ? (
        <div>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("workflow.list.create.confirmedTitle")}</p>
            <p>{props.result.replayed ? t("workflow.list.create.replayedBody") : t("workflow.list.create.confirmedBody")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("workflow.list.create.confirmedIdLabel")}</dt>
                <dd>
                  <code>{props.result.definition.definition_id}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={styles.toolbar}>
            <button
              type="button"
              className={styles.control}
              onClick={() => props.onOpenCreated(props.result?.definition.definition_id ?? "")}
            >
              {t("workflow.list.create.openAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "unknown" ? (
        <div>
          <RecoveryPanel
            title={t("workflow.list.create.unknownTitle")}
            body={t("workflow.list.create.unknownBody")}
            referenceId={props.unknown?.correlationId ?? ""}
          />
          <dl className={styles.reviewFacts}>
            <div>
              <dt>{t("workflow.list.create.unknownRefLabel")}</dt>
              <dd>
                <code>{props.unknown?.correlationId ?? ""}</code>
              </dd>
            </div>
          </dl>
          {props.reconcileFailed ? <p className={styles.fieldNote}>{t("workflow.list.create.reconcileFailed")}</p> : null}
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onReconcile}>
              {t("workflow.list.create.reconcile")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "failure" && props.failure !== null ? (
        <div>
          <RecoveryPanel
            title={t("workflow.list.create.failureTitle")}
            body={t("workflow.list.create.failureBody")}
            note={t("workflow.list.connectionHint")}
            referenceId={props.failure.correlationId}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{props.failure.code}</code>
          </p>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}

function WorkflowRow({ definition }: { definition: WorkflowDefinition }) {
  const t = useT();
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const shownName = fieldOrUnavailable(definition.display_name, t("workflow.list.untitledDefinition"));

  return (
    <tr className={styles.row}>
      <td className={styles.cellName}>
        <Link to={`/workflows/${encodeURIComponent(definition.definition_id)}`} className={styles.rowLink}>
          <span className={styles.nameText}>{shownName}</span>
          <span className="visually-hidden">
            {t("workflow.list.openDefinition", { definitionId: definition.definition_id })}
          </span>
        </Link>
      </td>
      <td className={styles.cellId}>
        <CopyIdButton
          id={definition.definition_id}
          label={t("workflow.list.copyDefinitionId")}
          copiedLabel={t("workflow.list.copied")}
          failedLabel={t("workflow.list.copyFailed")}
          className={styles.copyId}
        />
      </td>
      <td className={styles.cell}>
        {/* The current revision renders as the Core reports it; a
            definition with no saved revision is stated as exactly that —
            never inferred (the P4.5 §3.3 map's law). */}
        {definition.current_revision_ref === undefined || definition.current_revision_ref === "" ? (
          <span className={styles.mutedCell}>{t("workflow.list.revisionNone")}</span>
        ) : (
          <code>{definition.current_revision_ref}</code>
        )}
      </td>
      {/* The recorded W1 run/status cells render Unavailable: the
          definition record carries no such member, and no run data exists
          while the D-40 gate withholds Run execution — never zero, never
          inferred. */}
      <td className={styles.cell}>
        <span className={styles.mutedCell}>{unavailable}</span>
      </td>
      <td className={styles.cell}>
        <span className={styles.mutedCell}>{unavailable}</span>
      </td>
      <td className={styles.cell}>
        <time dateTime={definition.created_at}>{formatTimestamp(new Date(definition.created_at), locale)}</time>
      </td>
    </tr>
  );
}
