import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { ContextOperation, CoreWireEnvelope, WireSnapshotView } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import { ContextInclusionDialog } from "./ContextInclusionDialog";
import styles from "./ContextInspectorPage.module.css";
import {
  contextFailureCodeKey,
  contextFailureRecoveryKey,
  contextOperationKindLabelKey,
  contextOperationStateChip,
  chipToneClass,
} from "./contextStates";

/**
 * `/context` — the Context Inspector (B3-U1; the DE-0 queue row 5,
 * extending P4.1 §3.6 C1–C7 against the now-live B3 adapter and the
 * served F5 snapshot read).
 *
 * What this page consumes (exactly as the Core serves it):
 * - `GET /v1/jobs/{id}/snapshot` (the served F5 Job-scoped read through
 *   the existing transport method) — the Inspector's data source. THE
 *   F5 NON-DUPLICATION LAW (BINDING): the Jobs flow's own snapshot
 *   presentation (JobDetailPage's SnapshotViewPanel) is untouched — this
 *   surface renders the P4.6 Inspector's two-region inspection view of
 *   the SAME served data, never a re-serving or alteration of the
 *   Jobs-flow presentation (P4.6 §1's law). The compiled content is
 *   never carried anywhere (the F5 law).
 * - `POST /v1/context/inclusion` (the M-18 per-request memory-inclusion
 *   decision, UNCONDITIONAL) through the inclusion dialog.
 * - `GET /v1/context/operations/{id}` (the family's authoritative state
 *   endpoint, a pure query — never a retry) through the operation-status
 *   surface.
 *
 * The conditional controls (C4 save snapshot / C5 compare) are HIDDEN
 * ENTIRELY (GC-3/GC-4): their un-gate preconditions — durable snapshot
 * storage existing and stable Core comparison existing — are the owner's
 * PROPOSED decisions, unresolved. Never a dead stub, never a
 * disabled-with-tooltip fake (UI §31.5): no save button and no compare
 * control exists anywhere on this surface while the conditions stand; the
 * recorded gate-state note (C7) renders the accurate reason visibly.
 *
 * The binding laws carried, exactly (P4.6 §3.3):
 * - No-silent-truncation: every Core-reported omission renders with its
 *   Core-provided reason (ST-2) or the honest reason-unavailable state
 *   (ST-3) — never discarded, never synthesized.
 * - No-invented-rationale: the UI never invents why a source was chosen
 *   or omitted — the reasons are the Core's alone.
 * - Estimate/actual distinctness (ST-5/ST-6): the estimate renders with
 *   its estimator version; the used budget renders "not yet reported" —
 *   the provider/Core usage-reporting fields are UNSUPPLIED (the owner's
 *   PROPOSED decision); actual usage renders only when the provider
 *   reports it, never an extrapolated or computed value.
 * - Exact vs approximate (ST-7/ST-8): the label renders ONLY from the
 *   Core-reported state (the served `exact_vs_approximate` value); exact
 *   never implies deterministic model output; the UI never claims
 *   exactness itself.
 * - Non-canonical (F-6): the preview is explicitly NON-CANONICAL
 *   knowledge and never implies deterministic model output; the
 *   reconstruction-limit statement says exactly what is reconstructable.
 * - Identifier law: snapshot/compilation/digest identifiers are
 *   Core-owned; the UI never mints, guesses, or rewrites one (X-4).
 * - Data sensitivity: sources render as references, digests, and
 *   reasons — content is never rendered wholesale; nothing leaks into
 *   URLs beyond the job reference the user names (the identity rides the
 *   query; the content never does).
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed D-35 refusal surfaced verbatim, or the truthful
 *   unknown (a lost or uninterpretable response — a timeout is never
 *   presented as any outcome; the authoritative read is re-queried
 *   BEFORE any retry, the S0–S4 law).
 */

type ReadPhase = "idle" | "loading" | "data" | "failure" | "unknown" | "notFound";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

function formatWireTimestamp(value: string, locale: string | undefined): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatTimestamp(parsed, locale);
}

export function ContextInspectorPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const [searchParams, setSearchParams] = useSearchParams();

  const [jobRef, setJobRef] = useState(searchParams.get("job") ?? "");
  const [phase, setPhase] = useState<ReadPhase>("idle");
  const [snapshot, setSnapshot] = useState<WireSnapshotView | null>(null);
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [confirmedAt, setConfirmedAt] = useState<Date | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [inclusionOpen, setInclusionOpen] = useState(false);
  const [opId, setOpId] = useState("");
  const [opPhase, setOpPhase] = useState<"idle" | "loading" | "data" | "failure" | "unknown">("idle");
  const [operation, setOperation] = useState<ContextOperation | null>(null);
  const [opFailure, setOpFailure] = useState<CoreWireEnvelope | null>(null);
  const [opNotFound, setOpNotFound] = useState(false);

  // The read-sequence guard: only the LATEST issued read applies its
  // result, so a refresh supersedes an in-flight read instead of being
  // dropped; nothing is ever applied twice.
  const readSeqRef = useRef(0);
  const opSeqRef = useRef(0);
  const mountedRef = useRef(true);
  const inclusionTriggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const runRead = useCallback(
    (options: { readonly jobRef: string; readonly markRefreshing: boolean }) => {
      const trimmed = options.jobRef.trim();
      if (trimmed === "") return;
      const seq = ++readSeqRef.current;
      if (options.markRefreshing) setRefreshing(true);
      else setPhase("loading");
      setFailure(null);
      void transport.jobSnapshot(trimmed).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        if (outcome.kind === "data") {
          setSnapshot(outcome.data.snapshot);
          setConfirmedAt(new Date());
          setPhase("data");
        } else if (outcome.kind === "rejection") {
          setSnapshot(null);
          setConfirmedAt(null);
          setFailure(outcome.envelope);
          // The typed "no snapshot stored for this job" refusal is its
          // own honest state (the surfaced code — never reinterpreted as
          // an empty structure).
          setPhase(outcome.envelope.code === "NOT_FOUND" ? "notFound" : "failure");
        } else {
          setSnapshot(null);
          setConfirmedAt(null);
          setPhase("unknown");
        }
      });
    },
    [transport],
  );

  // A deep link (?job=…) reads once on mount.
  useEffect(() => {
    const linked = searchParams.get("job");
    if (linked !== null && linked !== "") {
      runRead({ jobRef: linked, markRefreshing: false });
    }
  }, []);

  const inspect = () => {
    const trimmed = jobRef.trim();
    if (trimmed === "") return;
    setSearchParams({ job: trimmed }, { replace: true });
    runRead({ jobRef: trimmed, markRefreshing: false });
  };

  const refresh = () => {
    if (refreshing || phase === "loading") return;
    runRead({ jobRef: jobRef.trim(), markRefreshing: true });
  };

  const readOperation = () => {
    const trimmed = opId.trim();
    if (trimmed === "") return;
    const seq = ++opSeqRef.current;
    setOpPhase("loading");
    setOpFailure(null);
    setOpNotFound(false);
    void transport.contextOperationStatus(trimmed).then((outcome) => {
      if (!mountedRef.current || seq !== opSeqRef.current) return; // superseded
      if (outcome.kind === "data") {
        setOperation(outcome.data.operation);
        setOpPhase("data");
      } else if (outcome.kind === "rejection") {
        setOperation(null);
        setOpFailure(outcome.envelope);
        setOpNotFound(outcome.envelope.code === "CONTEXT_SNAPSHOT_NOT_FOUND");
        setOpPhase("failure");
      } else {
        setOperation(null);
        setOpPhase("unknown");
      }
    });
  };

  const announce = (() => {
    switch (phase) {
      case "loading":
        return t("context.inspector.loading");
      case "failure":
        return t("context.inspector.failureTitle");
      case "notFound":
        return t("context.inspector.notFoundTitle");
      case "unknown":
        return t("context.inspector.unknownTitle");
      default:
        return phase === "data" ? t("context.inspector.heading") : t("context.inspector.noJob");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("context.inspector.heading")}</PageHeading>
      <p className={styles.description}>{t("context.inspector.description")}</p>
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      <div className={styles.toolbar}>
        <label className={styles.fieldLabel} htmlFor="context-job-ref">
          {t("context.inspector.jobLabel")}
        </label>
        <input
          id="context-job-ref"
          className={styles.fieldInput}
          value={jobRef}
          placeholder={t("context.inspector.jobPlaceholder")}
          onChange={(event) => setJobRef(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") inspect();
          }}
        />
        <button
          type="button"
          className={styles.primaryControl}
          onClick={inspect}
          disabled={jobRef.trim() === "" || phase === "loading"}
        >
          {phase === "loading" ? t("context.inspector.inspecting") : t("context.inspector.inspect")}
        </button>
        {phase === "data" ? (
          <button
            type="button"
            className={styles.control}
            onClick={refresh}
            disabled={refreshing}
          >
            {refreshing ? t("context.inspector.refreshing") : t("context.inspector.refresh")}
          </button>
        ) : null}
        {/* The M-18 per-request memory-inclusion decision (the
            unconditional served member) opens here — an explicit decision
            form, never a global toggle. */}
        <button
          type="button"
          ref={inclusionTriggerRef}
          className={styles.control}
          onClick={() => setInclusionOpen(true)}
        >
          {t("context.inclusion.open")}
        </button>
      </div>

      {phase === "idle" ? <p className={styles.note}>{t("context.inspector.noJob")}</p> : null}
      {phase === "loading" ? <p className={styles.note}>{t("context.inspector.loading")}</p> : null}

      {phase === "notFound" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("context.inspector.notFoundTitle")}
            body={t("context.inspector.notFoundBody")}
            referenceId={failure.correlationId}
            actions={[{ label: t("context.inspector.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {phase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("context.inspector.failureTitle")}
            body={t("context.inspector.failureBody")}
            note={t("context.inspector.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("context.inspector.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {phase === "unknown" ? (
        <RecoveryPanel
          title={t("context.inspector.unknownTitle")}
          body={t("context.inspector.unknownBody")}
          note={t("context.inspector.connectionHint")}
          actions={[{ label: t("context.inspector.refresh"), onAction: refresh }]}
        />
      ) : null}

      {phase === "data" && snapshot !== null && confirmedAt !== null ? (
        <InspectorSnapshotView snapshot={snapshot} confirmedAt={confirmedAt} locale={locale} />
      ) : null}

      {/* THE CONDITIONAL GATE STATE (C7 — carried exactly): the save and
          compare controls stay HIDDEN ENTIRELY while their un-gate
          preconditions stand (the owner's PROPOSED decisions,
          unresolved); the accurate reason renders visibly,
          keyboard-reachable, and persists at all widths. Never a dead
          stub, never a disabled-with-tooltip fake (UI §31.5). */}
      <section className={styles.gateNote} aria-labelledby="context-gate-note-title">
        <h2 id="context-gate-note-title" className={styles.gateNoteTitle}>
          {t("context.inspector.gateNoteTitle")}
        </h2>
        <p>{t("context.inspector.gateNoteBody")}</p>
      </section>

      <section className={styles.operationsSection} aria-labelledby="context-operations-title">
        <h2 id="context-operations-title" className={styles.sectionTitle}>
          {t("context.operations.heading")}
        </h2>
        <p className={styles.note}>{t("context.operations.description")}</p>
        <div className={styles.toolbar}>
          <label className={styles.fieldLabel} htmlFor="context-op-id">
            {t("context.operations.opIdLabel")}
          </label>
          <input
            id="context-op-id"
            className={styles.fieldInput}
            value={opId}
            placeholder={t("context.operations.opIdPlaceholder")}
            onChange={(event) => setOpId(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") readOperation();
            }}
          />
          <button
            type="button"
            className={styles.control}
            onClick={readOperation}
            disabled={opId.trim() === "" || opPhase === "loading"}
          >
            {opPhase === "loading" ? t("context.operations.reading") : t("context.operations.read")}
          </button>
        </div>
        {opPhase === "loading" ? <p className={styles.note}>{t("context.operations.reading")}</p> : null}
        {opPhase === "failure" && opFailure !== null ? (
          <>
            {opNotFound ? (
              <RecoveryPanel
                title={t("context.operations.notFoundTitle")}
                body={t("context.operations.notFoundBody")}
                referenceId={opFailure.correlationId}
                actions={[{ label: t("context.operations.read"), onAction: readOperation }]}
              />
            ) : (
              <RecoveryPanel
                title={t("context.operations.failureTitle")}
                body={t("context.operations.failureBody")}
                note={t(contextFailureRecoveryKey(opFailure.code))}
                referenceId={opFailure.correlationId}
                actions={[{ label: t("context.operations.read"), onAction: readOperation }]}
              />
            )}
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{opFailure.code}</code>
            </p>
            <p className={styles.note}>{t(contextFailureCodeKey(opFailure.code))}</p>
          </>
        ) : null}
        {opPhase === "unknown" ? (
          <RecoveryPanel
            title={t("context.operations.unknownTitle")}
            body={t("context.operations.unknownBody")}
            actions={[{ label: t("context.operations.read"), onAction: readOperation }]}
          />
        ) : null}
        {opPhase === "data" && operation !== null ? <OperationView operation={operation} locale={locale} /> : null}
      </section>

      <ContextInclusionDialog
        open={inclusionOpen}
        onClose={() => {
          setInclusionOpen(false);
          inclusionTriggerRef.current?.focus();
        }}
      />
    </PageLayout>
  );
}

/** The two-region inspection view (P4.6 §3.5 C1; UI §16.2 — collapses on
 *  compact through the responsive layout): every value renders from the
 *  served F5 read only; a missing field renders Unavailable — never
 *  zero, never inferred (the C7 law). */
function InspectorSnapshotView({
  snapshot,
  confirmedAt,
  locale,
}: {
  snapshot: WireSnapshotView;
  confirmedAt: Date;
  locale: string | undefined;
}) {
  const t = useT();
  const exactness = snapshot.exact_vs_approximate;
  const coreProduced = exactness === "core_produced";
  const approximate = exactness === "approximate";

  return (
    <div className={styles.snapshotPanel}>
      <p className={styles.confirmed}>{t("context.inspector.lastConfirmed", { time: formatTimestamp(confirmedAt, locale) })}</p>

      {/* The non-canonical law (F-6) and the reconstruction-limit law:
          stated explicitly, never implied away. */}
      <p className={styles.lawNote}>{t("context.inspector.nonCanonicalNote")}</p>
      <p className={styles.lawNote}>{t("context.inspector.reconstructionNote")}</p>
      <p className={styles.lawNote}>{t("context.inspector.sensitivityNote")}</p>

      <h2 className={styles.sectionTitle}>{t("context.view.identityHeading")}</h2>
      <div className={styles.fieldList}>
        <FieldRow label={t("context.view.snapshotId")} mono>
          <code>{snapshot.snapshot_id}</code>
        </FieldRow>
        <FieldRow label={t("context.view.taskRef")} mono>
          <span className={styles.mono}>{snapshot.task_ref}</span>
        </FieldRow>
        <FieldRow label={t("context.view.scope")} mono>
          <span className={styles.mono}>{snapshot.scope}</span>
        </FieldRow>
        <FieldRow label={t("context.view.schemaVersion")}>{snapshot.schema_version}</FieldRow>
        <FieldRow label={t("context.view.createdAt")}>{formatWireTimestamp(snapshot.created_at, locale)}</FieldRow>
        <FieldRow label={t("context.view.userInputDigest")} mono>
          <span className={styles.mono}>{snapshot.user_input_digest}</span>
        </FieldRow>
        <FieldRow label={t("context.view.assembledDigest")} mono>
          <span className={styles.mono}>{snapshot.assembled_digest}</span>
        </FieldRow>
        <FieldRow label={t("context.view.rendererVersion")} mono>
          <span className={styles.mono}>{snapshot.renderer_version}</span>
        </FieldRow>
        <FieldRow label={t("context.view.compressionMethod")} mono>
          <span className={styles.mono}>{snapshot.compression_method}</span>
        </FieldRow>
        <FieldRow label={t("context.view.retrievalConfigVersion")} mono>
          <span className={styles.mono}>{snapshot.retrieval_config_version}</span>
        </FieldRow>
      </div>

      {/* The exactness label renders ONLY from the Core-reported state
          (ST-7/ST-8): exact never implies deterministic model output;
          approximate renders WITH the available provenance instead of
          "exact"; the UI never claims exactness itself. An unknown
          reported value renders verbatim (never mapped). */}
      <h2 className={styles.sectionTitle}>{t("context.view.exactnessHeading")}</h2>
      <div className={styles.fieldList}>
        <div className={styles.fieldRow}>
          <span className={styles.fieldLabel}>{t("context.view.exactnessHeading")}</span>
          <span className={`${styles.chip} ${coreProduced ? styles.chipSuccess : approximate ? styles.chipWarning : styles.chipNeutral}`}>
            {coreProduced
              ? t("context.view.exactCoreProduced")
              : approximate
                ? t("context.view.approximateLabel")
                : exactness !== ""
                  ? exactness
                  : t("context.view.exactnessUnavailable")}
          </span>
        </div>
        {coreProduced ? <p className={styles.note}>{t("context.view.exactCoreProducedNote")}</p> : null}
        {approximate ? <p className={styles.note}>{t("context.view.approximateNote")}</p> : null}
      </div>

      {/* The budget display (ST-5/ST-6): the requested bounds render as
          the Core exposes them (unbounded renders Unbounded — never zero);
          the estimate renders labeled with its estimator version; the
          used budget renders "not yet reported" — the usage-reporting
          fields are UNSUPPLIED (the owner's PROPOSED decision); actual
          usage is never extrapolated, carried over, or computed. */}
      <h2 className={styles.sectionTitle}>{t("context.view.budgetHeading")}</h2>
      <div className={styles.fieldList}>
        <p className={styles.subheading}>{t("context.view.budgetRequestedHeading")}</p>
        <FieldRow label={t("context.view.budgetMaxInput")}>
          {snapshot.budget.requested.max_input_tokens === undefined
            ? t("context.view.budgetUnbounded")
            : snapshot.budget.requested.max_input_tokens}
        </FieldRow>
        <FieldRow label={t("context.view.budgetReservedOutput")}>
          {snapshot.budget.requested.reserved_output_tokens === undefined
            ? t("context.view.budgetUnbounded")
            : snapshot.budget.requested.reserved_output_tokens}
        </FieldRow>
        <FieldRow label={t("context.view.budgetMaxItems")}>
          {snapshot.budget.requested.max_items === undefined
            ? t("context.view.budgetUnbounded")
            : snapshot.budget.requested.max_items}
        </FieldRow>
        <FieldRow label={t("context.view.estimateLabel")}>{snapshot.budget.estimated_input_tokens}</FieldRow>
        <p className={styles.note}>{t("context.view.estimateNote", { estimator: snapshot.budget.estimator_version })}</p>
        <FieldRow label={t("context.view.usedLabel")}>
          <span className={styles.notReported}>{t("context.view.usedNotReported")}</span>
        </FieldRow>
        <p className={styles.note}>{t("context.view.usedNote")}</p>
      </div>

      <h2 className={styles.sectionTitle}>{t("context.view.versionsHeading")}</h2>
      <div className={styles.fieldList}>
        <FieldRow label={t("context.view.estimatorVersion")} mono>
          <span className={styles.mono}>{snapshot.estimator_version}</span>
        </FieldRow>
      </div>

      {/* THE METADATA EXTENSION'S HONEST ABSENCE: the published
          ContextSnapshotMetadataExtension members (workflow/run refs, the
          source-digest classes, the used budget, the model target, the
          snapshot digest, the representation version) have NO serving
          source today — no standalone compiled-context read was
          published. The absence renders honestly; nothing is invented
          (the C7 law; the never-fabricate rule). */}
      <h2 className={styles.sectionTitle}>{t("context.view.metadataHeading")}</h2>
      <p className={styles.note}>{t("context.view.metadataNote")}</p>

      {/* The two-region included/omitted layout (UI §16.2): the omitted
          region renders EVERY Core-reported omission — the no-silent-
          truncation law; a reason renders as the Core provides it (ST-2)
          or the honest reason-unavailable state (ST-3) — never
          synthesized, never discarded. */}
      <h2 className={styles.sectionTitle}>{t("context.view.includedHeading", { count: snapshot.included.length })}</h2>
      {snapshot.included.length === 0 ? (
        <p className={styles.note}>{t("context.view.includedEmpty")}</p>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              <th scope="col">{t("context.view.columnRef")}</th>
              <th scope="col">{t("context.view.columnDigest")}</th>
              <th scope="col">{t("context.view.columnTrust")}</th>
              <th scope="col">{t("context.view.columnReason")}</th>
            </tr>
          </thead>
          <tbody>
            {snapshot.included.map((item) => (
              <tr key={item.ref}>
                <td className={styles.cellMono}>{item.ref}</td>
                <td className={styles.cellMono}>{item.digest}</td>
                <td>{item.trust_label}</td>
                <td>{item.reason === "" ? t("context.view.reasonUnavailable") : item.reason}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2 className={styles.sectionTitle}>{t("context.view.omittedHeading", { count: snapshot.omitted.length })}</h2>
      {snapshot.omitted.length === 0 ? (
        <p className={styles.note}>{t("context.view.omittedEmpty")}</p>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              <th scope="col">{t("context.view.columnRef")}</th>
              <th scope="col">{t("context.view.columnReason")}</th>
            </tr>
          </thead>
          <tbody>
            {snapshot.omitted.map((omission) => (
              <tr key={omission.ref}>
                <td className={styles.cellMono}>{omission.ref}</td>
                <td>
                  {omission.reason === "" ? (
                    <>
                      <span className={styles.reasonUnavailable}>{t("context.view.reasonUnavailable")}</span>
                      <span className={styles.noteBlock}>{t("context.view.reasonUnavailableNote")}</span>
                    </>
                  ) : (
                    omission.reason
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/** The authoritative operation record (the state endpoint's payload): the
 *  closed registers render as icon+text chips (unknown reported values
 *  render verbatim, never mapped); `outcome_unknown` renders as ITS OWN
 *  state (never success/failure — the S0–S4 law); the result and failure
 *  members render exactly as the Core returns them (empty = none
 *  reported). */
function OperationView({ operation, locale }: { operation: ContextOperation; locale: string | undefined }) {
  const t = useT();
  const stateChip = contextOperationStateChip(operation.state);
  const StateIcon = stateChip.icon;
  const kindKey = contextOperationKindLabelKey(operation.kind);

  return (
    <div className={styles.fieldList}>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>{t("context.view.snapshotId")}</span>
        <span className={styles.mono}>
          <code>{operation.operation_id}</code>
        </span>
      </div>
      <FieldRow label={t("context.operations.kindLabel")}>
        {kindKey !== null ? t(kindKey) : operation.kind}
      </FieldRow>
      <div className={styles.fieldRow}>
        <span className={styles.fieldLabel}>{t("context.operations.stateLabel")}</span>
        <span className={`${styles.chip} ${styles[chipToneClass(stateChip.tone)]}`}>
          <StateIcon aria-hidden="true" size={14} />
          {t(stateChip.labelKey)}
        </span>
      </div>
      <FieldRow label={t("context.operations.correlationLabel")} mono>
        <span className={styles.mono}>{operation.correlation_id}</span>
      </FieldRow>
      <FieldRow label={t("context.operations.sourceJobLabel")} mono>
        <span className={styles.mono}>{operation.source_job_id === undefined || operation.source_job_id === "" ? "—" : operation.source_job_id}</span>
      </FieldRow>
      {operation.result !== undefined && operation.result !== null ? (
        <>
          <p className={styles.subheading}>{t("context.operations.resultHeading")}</p>
          <FieldRow label={t("context.operations.snapshotIdLabel")} mono>
            <span className={styles.mono}>{operation.result.snapshot_id ?? "—"}</span>
          </FieldRow>
          <FieldRow label={t("context.operations.retentionLabel")}>
            {operation.result.retention_note === undefined || operation.result.retention_note === ""
              ? t("context.operations.retentionAbsent")
              : operation.result.retention_note}
          </FieldRow>
        </>
      ) : null}
      {operation.failure_code !== undefined && operation.failure_code !== "" ? (
        <>
          <FieldRow label={t("context.operations.failureCodeLabel")} mono>
            <span className={styles.mono}>{operation.failure_code}</span>
          </FieldRow>
          <p className={styles.note}>{t(contextFailureCodeKey(operation.failure_code))}</p>
        </>
      ) : null}
      {operation.failure_reason !== undefined && operation.failure_reason !== "" ? (
        <FieldRow label={t("context.operations.failureReasonLabel")}>{operation.failure_reason}</FieldRow>
      ) : null}
      <FieldRow label={t("context.operations.createdAtLabel")}>{formatWireTimestamp(operation.created_at, locale)}</FieldRow>
      <FieldRow label={t("context.operations.updatedAtLabel")}>{formatWireTimestamp(operation.updated_at, locale)}</FieldRow>
    </div>
  );
}

function FieldRow({
  label,
  mono,
  children,
}: {
  label: string;
  mono?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={styles.fieldRow}>
      <span className={styles.fieldLabel}>{label}</span>
      <span className={mono ? `${styles.fieldValue} ${styles.mono}` : styles.fieldValue}>{children}</span>
    </div>
  );
}
