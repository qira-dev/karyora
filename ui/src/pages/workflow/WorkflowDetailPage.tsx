import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { useDirtyNavigationGuard } from "../../app/useDirtyNavigationGuard";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  WorkflowDefinition,
  WorkflowDefinitionState,
  WorkflowDraftEdge,
  WorkflowDraftNode,
  WorkflowRevision,
  WorkflowRunHistoryEntry,
  WorkflowValidationIssue,
  WorkflowValidationResult,
} from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./WorkflowDetailPage.module.css";
import {
  CopyIdButton,
  mintWorkflowCorrelationId,
  mintWorkflowSaveKey,
  workflowIssueClassChip,
  workflowLinkageChip,
} from "./workflowStates";

/**
 * `/workflows/:workflowId` — the ONE canonical workflow-detail route
 * (B4-U1; the DE-0 queue row 7, the P4.1 B1/B17/B19/B20 rows against the
 * now-live B4 adapter; the Builder/editing surface wired by B4-U2 per the
 * UI-30 gate resolution, GR-C2 — the GR-0 queue row 4).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/workflows/{id}` (the inspect member, B4-A1 §2.1 #5): the
 *   definition plus its immutable revision records — each revision
 *   renders as the identity record the Core serves (reference and
 *   creation time). The family publishes NO revision-content read
 *   (B4-A1 §2.4 gap 7), so a stored revision renders its identity record
 *   only — never invented content.
 * - `GET /v1/workflows/{id}/state` (the authoritative state endpoint,
 *   B4-A1 §2.1 #8): a pure query — never a retry; the lost/uncertain
 *   save outcome's reconcile anchor (VRC-LOST-1).
 * - `GET /v1/workflows/{id}/run-history` (the LINKAGE read, B4-A1 §2.1
 *   #7): each entry renders beside its Core-reported source revision; a
 *   missing linkage renders Unavailable — NEVER an inferred link
 *   (RUN-LINK-3). The honest empty page stands while the D-40 gate
 *   withholds the run producer — no run record is fabricated.
 * - THE BUILDER (B4-U2; P4.8's laws): the draft is CLIENT-LOCAL authoring
 *   state (WorkflowDraftContent — never Core state until a Core-confirmed
 *   save). The three states stay distinct and labeled (the stored
 *   revision; the draft's dirty state; each run's own linked revision).
 *   Every action carries a keyboard route AND a pointer route, separately
 *   (P4.8 §3.1/§3.2): selection (arrow keys + Enter; click), connect (the
 *   explicit source→target dialog without any drag; keyboard-operable),
 *   select-edge (the edge list), delete (the Delete key on a focused row;
 *   the pointer control), undo/redo (the platform modifiers; the toolbar
 *   controls — editor-local, bounded, draft-only). The palette's ONLY
 *   source is the Core-contracted node vocabulary (P4.7 §3.3): the Core
 *   publishes no supported node-type set, so the palette renders its
 *   honest empty state and the add action is disabled-with-reason — no
 *   type list is invented, no free-text type entry exists (RUN-LINK-11's
 *   no-generic-no-code law). Reorder, zoom/fit, and clipboard/duplicate
 *   are OMITTED entirely (the served shape carries no ordering member and
 *   no spatial presentation exists; the omission law — never a disabled
 *   stub implying support).
 * - THE VALIDATION (B4-U2; P4.9 §3.2): the Validate control is LIVE — its
 *   precondition (a draft) is mettable now. The Core's typed result is
 *   the authoritative truth and renders with its exact-object label
 *   (`evaluated_object_ref`, RUN-LINK-2); each issue links to its
 *   offending node/edge (the validation navigation, B19); a result whose
 *   draft has since changed is labeled stale — one object's result is
 *   never presented as another's. Validation writes nothing (the
 *   no-silent-save law); a lost validation is the explained state and a
 *   re-validate is a FRESH validation, never a retry of a mutation.
 * - THE SAVE (B4-U2; P4.9 §3.3, the M-21 class): one explicit command
 *   creates a NEW immutable revision, the new reference is STATED, the
 *   old revision stays immutable and reachable — no update-in-place ever.
 *   The caller mints the keyed pair (the 1 h replay window); the base is
 *   the client's known base (the empty string for a definition with no
 *   revisions — the adapter's recorded reading). The draft's dirty state
 *   clears ONLY on the Core-confirmed confirmation — never a pre-emptive
 *   "saved" label. A typed rejection (400 WORKFLOW_VALIDATION_FAILED)
 *   stores nothing and the draft survives byte-for-byte, visibly. A lost
 *   response reconciles through the authoritative state read BEFORE any
 *   retry (the S0–S4 law) — the keyed re-issue re-uses the SAME pair and
 *   never mints a second revision.
 * - THE CONFLICT (B4-U2; P4.9 §3.4, the interim rule): the typed 409
 *   WORKFLOW_REVISION_CONFLICT renders BOTH sides separately and labeled
 *   (the stored side by the reference the envelope's datum carries; the
 *   local draft preserved client-side). The conflict persists until the
 *   user resolves it — never auto-resolved, never timed out, never
 *   resolved by a refresh. The offered choices are exactly the
 *   Core-supported set: reload-to-remote (explicit, confirmed, never
 *   default). Compare and merge are NOT rendered — the contract defines
 *   neither, and an unsupported choice's absence is the correct rendering
 *   (never a disabled look-alike).
 * - THE GUARD (B4-U2; P4.9 §3.3, the J-8/X-3 shared class): leaving the
 *   surface with a dirty draft prompts ONCE — Keep editing / Discard
 *   draft / Save revision (offered only when no current blocking
 *   validation stands). Navigation itself never saves, cancels, or
 *   mutates; unchanged data is never re-prompted.
 * - THE BOUNDARY (carried exactly): NO Run control exists anywhere (the
 *   D-40 gate); NO If-Match/ETag/precondition member exists anywhere (the
 *   clause is prose-only — the interim conflict rule governs); the
 *   AI-proposal producer stays unwired (the typed refusal is the served
 *   state — no proposal surface is fabricated, B4-A1 §2.4 gap 1).
 */

type ReadPhase = "loading" | "data" | "failure" | "unknown";

type TabId = "builder" | "validation" | "versions" | "runs" | "permissions";

/** The enabled tabs in the recorded B1 order. The Builder tab is LIVE per
 *  the UI-30 resolution (GR-C2) at its recorded first position; the
 *  Permissions & side effects tab renders its honest absence (the
 *  per-node contract fields render only where the Core supplies them —
 *  P4.7 §3.2's display law). */
const ENABLED_TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "builder", labelKey: "workflow.detail.tabsBuilder" },
  { id: "validation", labelKey: "workflow.detail.tabsValidation" },
  { id: "versions", labelKey: "workflow.detail.tabsVersions" },
  { id: "runs", labelKey: "workflow.detail.tabsRuns" },
  { id: "permissions", labelKey: "workflow.detail.tabsPermissions" },
];

/** The client-local draft state (WorkflowDraftContent's shape). The draft
 *  is never Core state (the no-silent-save law) — it lives only here and
 *  rides the validate/save bodies exactly as the published schemas carry
 *  it. */
interface BuilderDraft {
  readonly nodes: readonly WorkflowDraftNode[];
  readonly edges: readonly WorkflowDraftEdge[];
}

const EMPTY_DRAFT: BuilderDraft = { nodes: [], edges: [] };

/** The undo/redo history bound (the honest bounded-history note's number
 *  — A-10's "history-limited undo explained honestly"). */
const UNDO_HISTORY_LIMIT = 50;

type ValidatePhase = "idle" | "pending" | "unknown" | "failure";

/** The save flow's phases: idle, the in-flight submission, the
 *  Core-confirmed outcome (the new reference stated; the honest
 *  `replayed`), the typed validation refusal (400 — the draft survives
 *  visibly), the typed conflict (409 — the interim rule), the typed
 *  refusal, and the explained uncertain outcome (the keyed pair retained;
 *  the reconcile through the authoritative state read). */
type SavePhase = "idle" | "submitting" | "confirmed" | "validationRefused" | "conflict" | "failure" | "unknown";

/** The save reconcile's outcomes: the state read found the save landed
 *  (the stated revision adopted) or found no recorded save (the draft
 *  intact; the keyed re-issue available). */
type SaveReconcileState = "landed" | "unchanged" | null;

/** The connect dialog's input-error register (draft-local structural
 *  hygiene — the Core re-checks authoritatively at validate/save). */
type ConnectError = "none" | "sameNode" | "duplicate";

interface SaveUnknownState {
  readonly idempotencyKey: string;
  readonly correlationId: string;
  readonly baseRevisionRef: string;
}

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

/** The draft-equality check (the dirty law's backbone): the draft is
 *  dirty exactly when it differs from the last Core-confirmed state, so
 *  undoing back to the saved state is honestly "no unsaved changes" and
 *  the guard never re-prompts unchanged data (X-3). */
function draftEquals(a: BuilderDraft, b: BuilderDraft): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** The announcement summary for a validation result (the typed classes'
 *  counts — the Core's own output, never invented categories). */
function validationSummary(result: WorkflowValidationResult): string {
  return `${result.blocking_errors.length} blocking errors, ${result.warnings.length} non-blocking warnings, ${result.permission_gaps.length} permission gaps, ${result.schema_compatibility.length} schema-compatibility findings`;
}

/** Groups a validation result's issues by their draft-local link anchor
 *  (node_ref / edge_ref — the exact-object law's linkage anchors). */
function groupIssues(result: WorkflowValidationResult): {
  byNode: ReadonlyMap<string, readonly WorkflowValidationIssue[]>;
  byEdge: ReadonlyMap<string, readonly WorkflowValidationIssue[]>;
} {
  const byNode = new Map<string, WorkflowValidationIssue[]>();
  const byEdge = new Map<string, WorkflowValidationIssue[]>();
  const all = [
    ...result.blocking_errors,
    ...result.warnings,
    ...result.permission_gaps,
    ...result.schema_compatibility,
  ];
  for (const issue of all) {
    if (typeof issue.node_ref === "string" && issue.node_ref !== "") {
      const bucket = byNode.get(issue.node_ref);
      if (bucket === undefined) byNode.set(issue.node_ref, [issue]);
      else bucket.push(issue);
    }
    if (typeof issue.edge_ref === "string" && issue.edge_ref !== "") {
      const bucket = byEdge.get(issue.edge_ref);
      if (bucket === undefined) byEdge.set(issue.edge_ref, [issue]);
      else bucket.push(issue);
    }
  }
  return { byNode, byEdge };
}

/** Reads the conflict envelope's stored-side reference datum (the
 *  redaction law: the reference only — never content). */
function conflictRemoteRef(envelope: CoreWireEnvelope): string | undefined {
  const value = envelope.data["current_revision_ref"];
  return typeof value === "string" && value !== "" ? value : undefined;
}

export function WorkflowDetailPage() {
  const { workflowId = "" } = useParams<{ workflowId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [definition, setDefinition] = useState<WorkflowDefinition | null>(null);
  const [revisions, setRevisions] = useState<readonly WorkflowRevision[]>([]);
  const [state, setState] = useState<WorkflowDefinitionState | null>(null);
  const [runEntries, setRunEntries] = useState<readonly WorkflowRunHistoryEntry[]>([]);
  const [inspectPhase, setInspectPhase] = useState<ReadPhase>("loading");
  const [statePhase, setStatePhase] = useState<ReadPhase>("loading");
  const [runsPhase, setRunsPhase] = useState<ReadPhase>("loading");
  const [inspectFailure, setInspectFailure] = useState<CoreWireEnvelope | null>(null);
  const [stateFailure, setStateFailure] = useState<CoreWireEnvelope | null>(null);
  const [runsFailure, setRunsFailure] = useState<CoreWireEnvelope | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>("builder");
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [notFound, setNotFound] = useState(false);

  // ---- The builder's client-local draft state (B4-U2). The draft, its
  // history, and the selection live at PAGE level so a tab switch never
  // drops the user's work (the draft survives; the guard covers leaving
  // the ROUTE).
  const [draft, setDraft] = useState<BuilderDraft>(EMPTY_DRAFT);
  const [savedDraft, setSavedDraft] = useState<BuilderDraft>(EMPTY_DRAFT);
  const [past, setPast] = useState<readonly BuilderDraft[]>([]);
  const [future, setFuture] = useState<readonly BuilderDraft[]>([]);
  const [selectedNodeRef, setSelectedNodeRef] = useState<string | null>(null);
  const [selectedEdgeRef, setSelectedEdgeRef] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");

  // The validation flow (the read-like class — no key minted).
  const [validationResult, setValidationResult] = useState<WorkflowValidationResult | null>(null);
  const [validatedSnapshot, setValidatedSnapshot] = useState<BuilderDraft | null>(null);
  const [validatePhase, setValidatePhase] = useState<ValidatePhase>("idle");
  const [validateFailure, setValidateFailure] = useState<CoreWireEnvelope | null>(null);

  // The save flow (the M-21 class — the keyed pair per the 1 h class).
  const [savePhase, setSavePhase] = useState<SavePhase>("idle");
  const [saveResult, setSaveResult] = useState<{ readonly revisionRef: string; readonly replayed: boolean; readonly viaReconcile: boolean } | null>(null);
  const [saveFailure, setSaveFailure] = useState<CoreWireEnvelope | null>(null);
  const [saveUnknown, setSaveUnknown] = useState<SaveUnknownState | null>(null);
  const [saveReconcileState, setSaveReconcileState] = useState<SaveReconcileState>(null);
  const [saveReconcileFailed, setSaveReconcileFailed] = useState(false);
  const [conflictRemoteRevision, setConflictRemoteRevision] = useState<string | null>(null);
  const [conflictReloadConfirmOpen, setConflictReloadConfirmOpen] = useState(false);

  // The connect dialog (the explicit non-drag connect command).
  const [connectOpen, setConnectOpen] = useState(false);
  const [connectSource, setConnectSource] = useState("");
  const [connectTarget, setConnectTarget] = useState("");
  const [connectError, setConnectError] = useState<ConnectError>("none");

  const tabListRef = useRef<HTMLDivElement>(null);
  const validationPanelRef = useRef<HTMLElement>(null);
  const nodeListRef = useRef<HTMLUListElement>(null);
  const edgeListRef = useRef<HTMLUListElement>(null);
  const mountedRef = useRef(true);

  const draftDirty = !draftEquals(draft, savedDraft);
  const validationStale = validationResult !== null && validatedSnapshot !== null && !draftEquals(draft, validatedSnapshot);
  const currentBlocking =
    validationResult !== null && validatedSnapshot !== null && !validationStale && validationResult.blocking_errors.length > 0;
  const groupedIssues = useMemo(
    () => (validationResult === null ? null : groupIssues(validationResult)),
    [validationResult],
  );

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // The predicted-focus request (B8's "the next remaining element" and
  // A-03's "focus to the new edge"): applied once after the render that
  // realizes it, then cleared.
  const [focusRequest, setFocusRequest] = useState<
    { readonly kind: "node-row"; readonly ref: string } | { readonly kind: "edge-row"; readonly ref: string } | { readonly kind: "node-list" } | null
  >(null);
  useEffect(() => {
    if (focusRequest === null) return;
    if (focusRequest.kind === "node-row") {
      const row = nodeListRef.current?.querySelector<HTMLButtonElement>(`[data-node-ref="${CSS.escape(focusRequest.ref)}"]`);
      row?.focus();
    } else if (focusRequest.kind === "edge-row") {
      const row = edgeListRef.current?.querySelector<HTMLButtonElement>(`[data-edge-ref="${CSS.escape(focusRequest.ref)}"]`);
      row?.focus();
    } else {
      nodeListRef.current?.focus();
    }
    setFocusRequest(null);
  }, [focusRequest]);

  const readAll = useCallback(() => {
    setInspectPhase("loading");
    setInspectFailure(null);
    setStatePhase("loading");
    setStateFailure(null);
    setRunsPhase("loading");
    setRunsFailure(null);
    setNotFound(false);
    setLastConfirmedAt(null);
    void transport.workflowGet(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setDefinition(outcome.data.definition);
        setRevisions(outcome.data.revisions);
        setInspectPhase("data");
        setLastConfirmedAt(new Date());
      } else if (outcome.kind === "rejection") {
        setInspectFailure(outcome.envelope);
        setNotFound(outcome.envelope.code === "NOT_FOUND");
        setInspectPhase("failure");
      } else {
        setInspectPhase("unknown");
      }
    });
    void transport.workflowState(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data.definition_state);
        setStatePhase("data");
      } else if (outcome.kind === "rejection") {
        setStateFailure(outcome.envelope);
        setStatePhase("failure");
      } else {
        setStatePhase("unknown");
      }
    });
    void transport.workflowRunHistory(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setRunEntries(outcome.data.entries);
        setRunsPhase("data");
      } else if (outcome.kind === "rejection") {
        setRunsFailure(outcome.envelope);
        setRunsPhase("failure");
      } else {
        setRunsPhase("unknown");
      }
    });
  }, [transport, workflowId]);

  useEffect(() => {
    readAll();
  }, [readAll]);

  // ---- The draft operations. Each preserves the draft's structural
  // validity by construction (no dangling edges, no duplicate edge for a
  // node pair, no self-edge) — the Core's authoritative validation remains
  // the truth source at validate/save time.

  const applyDraftChange = useCallback(
    (next: BuilderDraft) => {
      setPast((prevPast) => {
        const withCurrent = [...prevPast, draft];
        return withCurrent.length > UNDO_HISTORY_LIMIT ? withCurrent.slice(withCurrent.length - UNDO_HISTORY_LIMIT) : withCurrent;
      });
      setFuture([]);
      setDraft(next);
    },
    [draft],
  );

  const undo = useCallback(() => {
    setPast((prevPast) => {
      if (prevPast.length === 0) return prevPast;
      const previous = prevPast[prevPast.length - 1];
      setFuture((prevFuture) => [draft, ...prevFuture].slice(0, UNDO_HISTORY_LIMIT));
      setDraft(previous);
      return prevPast.slice(0, prevPast.length - 1);
    });
  }, [draft]);

  const redo = useCallback(() => {
    setFuture((prevFuture) => {
      if (prevFuture.length === 0) return prevFuture;
      const [next, ...rest] = prevFuture;
      setPast((prevPast) => [...prevPast, draft].slice(0, UNDO_HISTORY_LIMIT));
      setDraft(next);
      return rest;
    });
  }, [draft]);

  const selectNode = useCallback((nodeRef: string) => {
    setSelectedNodeRef(nodeRef);
    setSelectedEdgeRef(null);
  }, []);

  const selectEdge = useCallback((edgeRef: string) => {
    setSelectedEdgeRef(edgeRef);
    setSelectedNodeRef(null);
  }, []);

  const openConnect = useCallback(() => {
    setConnectSource("");
    setConnectTarget("");
    setConnectError("none");
    setConnectOpen(true);
  }, []);

  const confirmConnect = useCallback(() => {
    if (connectSource === "" || connectTarget === "") return;
    if (connectSource === connectTarget) {
      setConnectError("sameNode");
      return;
    }
    const duplicate = draft.edges.some(
      (edge) =>
        (edge.from_node === connectSource && edge.to_node === connectTarget) ||
        (edge.from_node === connectTarget && edge.to_node === connectSource),
    );
    if (duplicate) {
      setConnectError("duplicate");
      return;
    }
    const edgeRef = `edge-${crypto.randomUUID()}`;
    const next: BuilderDraft = {
      nodes: draft.nodes,
      edges: [...draft.edges, { edge_ref: edgeRef, from_node: connectSource, to_node: connectTarget }],
    };
    applyDraftChange(next);
    setConnectOpen(false);
    selectEdge(edgeRef);
    setAnnouncement(
      t("workflow.detail.builder.announcementEdgeAdded", { fromNode: connectSource, toNode: connectTarget }),
    );
  }, [applyDraftChange, connectSource, connectTarget, draft, selectEdge, t]);

  /** Deletes one node by its draft-local ref — the node's incident edges
   *  go with it (no dangling edge by construction). Focus lands on the
   *  next remaining node predictably (B8); the deletion is announced. */
  const deleteNode = useCallback(
    (nodeRef: string) => {
      const nextNodes = draft.nodes.filter((node) => node.node_ref !== nodeRef);
      const nextEdges = draft.edges.filter((edge) => edge.from_node !== nodeRef && edge.to_node !== nodeRef);
      applyDraftChange({ nodes: nextNodes, edges: nextEdges });
      const nextSelection = nextNodes.length > 0 ? nextNodes[0].node_ref : null;
      setSelectedNodeRef(nextSelection);
      setSelectedEdgeRef(null);
      setAnnouncement(t("workflow.detail.builder.announcementNodeDeleted", { nodeRef }));
      setFocusRequest(nextSelection !== null ? { kind: "node-row", ref: nextSelection } : { kind: "node-list" });
    },
    [applyDraftChange, draft, t],
  );

  /** Deletes one edge by its draft-local ref. The deletion is announced. */
  const deleteEdge = useCallback(
    (edgeRef: string) => {
      const nextEdges = draft.edges.filter((edge) => edge.edge_ref !== edgeRef);
      applyDraftChange({ nodes: draft.nodes, edges: nextEdges });
      setSelectedEdgeRef(null);
      setAnnouncement(t("workflow.detail.builder.announcementEdgeDeleted", { edgeRef }));
    },
    [applyDraftChange, draft, t],
  );

  const deleteSelected = useCallback(() => {
    if (selectedNodeRef !== null) {
      deleteNode(selectedNodeRef);
      return;
    }
    if (selectedEdgeRef !== null) {
      deleteEdge(selectedEdgeRef);
    }
  }, [deleteEdge, deleteNode, selectedEdgeRef, selectedNodeRef]);

  // ---- The validation flow (the read-like class; the none-required
  // idempotency class — the correlation ID rides, no key is minted).

  const runValidation = useCallback(() => {
    setValidatePhase("pending");
    setValidateFailure(null);
    const correlationId = mintWorkflowCorrelationId("workflow-validate");
    void transport
      .workflowValidate({
        correlation_id: correlationId,
        definition_id: workflowId,
        draft: { nodes: [...draft.nodes], edges: [...draft.edges] },
      })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          setValidationResult(outcome.data);
          setValidatedSnapshot(draft);
          setValidatePhase("idle");
          // B12's focus law: the validation panel receives focus on
          // completion (the polite announcement rides the status region —
          // no focus theft beyond the recorded landing).
          setActiveTab("validation");
          setAnnouncement(
            t("workflow.detail.builder.announcementValidated", { summary: validationSummary(outcome.data) }),
          );
          window.requestAnimationFrame(() => {
            validationPanelRef.current?.focus();
          });
        } else if (outcome.kind === "rejection") {
          setValidateFailure(outcome.envelope);
          setValidatePhase("failure");
        } else {
          setValidatePhase("unknown");
        }
      });
  }, [draft, t, transport, workflowId]);

  // ---- The validation navigation (B19): an issue moves the user to the
  // offending element — the Builder tab's structured list selects it and
  // focus lands on its row predictably.
  const navigateToIssue = useCallback(
    (target: { readonly kind: "node" | "edge"; readonly ref: string }) => {
      setActiveTab("builder");
      if (target.kind === "node") {
        selectNode(target.ref);
        setFocusRequest({ kind: "node-row", ref: target.ref });
      } else {
        selectEdge(target.ref);
        setFocusRequest({ kind: "edge-row", ref: target.ref });
      }
    },
    [selectEdge, selectNode],
  );

  // ---- The save flow (the M-21 class; the keyed pair per the 1 h class;
  // the base is the client's known base — the empty string for a
  // revision-less definition).

  const submitSave = useCallback(
    (keyed?: SaveUnknownState) => {
      const idempotencyKey = keyed?.idempotencyKey ?? mintWorkflowSaveKey();
      const correlationId = keyed?.correlationId ?? mintWorkflowCorrelationId("workflow-save");
      const baseRevisionRef = keyed?.baseRevisionRef ?? state?.current_revision_ref ?? "";
      const payload = {
        idempotency_key: idempotencyKey,
        correlation_id: correlationId,
        base_revision_ref: baseRevisionRef,
        draft: { nodes: [...draft.nodes], edges: [...draft.edges] },
      };
      setSaveReconcileState(null);
      setSaveReconcileFailed(false);
      setSavePhase("submitting");
      void transport.workflowSaveRevision(workflowId, payload).then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The Core-confirmed save: the new revision is STATED; the dirty
          // state clears ONLY here (never a pre-emptive "saved" label).
          setSaveResult({ revisionRef: outcome.data.revision_ref, replayed: outcome.data.replayed, viaReconcile: keyed !== undefined });
          setSavePhase("confirmed");
          setSaveUnknown(null);
          setConflictRemoteRevision(null);
          setSavedDraft(draft);
          // The authoritative re-reads: the revision list and the state
          // pointer update as the Core reports them.
          void transport.workflowGet(workflowId).then((refresh) => {
            if (!mountedRef.current) return;
            if (refresh.kind === "data") {
              setDefinition(refresh.data.definition);
              setRevisions(refresh.data.revisions);
              setInspectPhase("data");
              setLastConfirmedAt(new Date());
            }
          });
          void transport.workflowState(workflowId).then((refresh) => {
            if (!mountedRef.current) return;
            if (refresh.kind === "data") setState(refresh.data.definition_state);
          });
          setAnnouncement(t("workflow.detail.builder.announcementSaved", { revisionRef: outcome.data.revision_ref }));
        } else if (outcome.kind === "rejection") {
          if (outcome.envelope.code === "WORKFLOW_REVISION_CONFLICT") {
            // The interim conflict rule: both sides preserved and shown;
            // the conflict persists until the user resolves it.
            setConflictRemoteRevision(conflictRemoteRef(outcome.envelope) ?? null);
            setSavePhase("conflict");
          } else if (outcome.envelope.code === "WORKFLOW_VALIDATION_FAILED") {
            // The save's authoritative re-check refused the draft; the
            // draft survives byte-for-byte, visibly (nothing was stored).
            setSaveFailure(outcome.envelope);
            setSavePhase("validationRefused");
            setActiveTab("validation");
            window.requestAnimationFrame(() => {
              validationPanelRef.current?.focus();
            });
          } else {
            setSaveFailure(outcome.envelope);
            setSavePhase("failure");
          }
        } else {
          // The S0–S4 law: the uncertain outcome retains the keyed pair;
          // reconcile through the authoritative state read BEFORE any
          // retry — never a blind retry, never a second revision.
          setSaveUnknown({ idempotencyKey, correlationId, baseRevisionRef });
          setSavePhase("unknown");
        }
      });
    },
    [draft, state, t, transport, workflowId],
  );

  // The reconcile action: the authoritative state read (a pure query —
  // never a retry). A usable answer decides: landed (the stated revision
  // adopted; the dirty state clears) or unchanged (no save recorded; the
  // keyed re-issue with the SAME pair becomes available — the replay
  // returns the recorded outcome, never a second revision). A read that
  // did not land a usable answer keeps the explained state (OC-8).
  const reconcileSave = useCallback(() => {
    setSaveReconcileFailed(false);
    void transport.workflowState(workflowId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data.definition_state);
        const remoteRef = outcome.data.definition_state.current_revision_ref;
        const base = saveUnknown?.baseRevisionRef ?? "";
        if (typeof remoteRef === "string" && remoteRef !== "" && remoteRef !== base) {
          setSaveResult({ revisionRef: remoteRef, replayed: false, viaReconcile: true });
          setSaveReconcileState("landed");
          setSavePhase("confirmed");
          setSaveUnknown(null);
          setSavedDraft(draft);
          void transport.workflowGet(workflowId).then((refresh) => {
            if (!mountedRef.current) return;
            if (refresh.kind === "data") {
              setDefinition(refresh.data.definition);
              setRevisions(refresh.data.revisions);
              setInspectPhase("data");
              setLastConfirmedAt(new Date());
            }
          });
          setAnnouncement(t("workflow.detail.builder.announcementSaved", { revisionRef: remoteRef }));
        } else {
          setSaveReconcileState("unchanged");
        }
      } else {
        setSaveReconcileFailed(true);
      }
    });
  }, [draft, saveUnknown, t, transport, workflowId]);

  // The conflict's only Core-supported resolution: reload-to-remote —
  // explicit, confirmed, never default. The draft is discarded; the
  // editor re-bases on the current stored revision (the restarted draft
  // begins empty — the Core serves no revision content to preload); the
  // authoritative reads refresh and the conflict clears.
  const resolveConflictByReload = useCallback(() => {
    setConflictReloadConfirmOpen(false);
    setConflictRemoteRevision(null);
    setSavePhase("idle");
    setSaveResult(null);
    setSaveFailure(null);
    setSaveUnknown(null);
    setSaveReconcileState(null);
    setDraft(EMPTY_DRAFT);
    setSavedDraft(EMPTY_DRAFT);
    setPast([]);
    setFuture([]);
    setSelectedNodeRef(null);
    setSelectedEdgeRef(null);
    setValidationResult(null);
    setValidatedSnapshot(null);
    setValidatePhase("idle");
    readAll();
  }, [readAll]);

  const keepConflict = useCallback(() => {
    // Keep editing: the conflict state PERSISTS (never auto-resolved);
    // the banner stays until the user resolves it.
    setConflictReloadConfirmOpen(false);
  }, []);

  // ---- The navigation guard (the shared J-8/X-3 hook with the
  // workflow's own truthful copy; the Save option is offered only when no
  // current blocking validation stands).
  const guard = useDirtyNavigationGuard({
    isDirty: () => draftDirty,
    onSave: () => {
      if (!currentBlocking) submitSave();
    },
    onDiscard: () => {
      // Discard the unsaved changes: revert to the last Core-confirmed
      // state (an empty draft where no revision is confirmed yet). The
      // stored revision is untouched; no run link changes.
      setDraft(savedDraft);
      setPast([]);
      setFuture([]);
      setSelectedNodeRef(null);
      setSelectedEdgeRef(null);
      setValidationResult(null);
      setValidatedSnapshot(null);
      setValidatePhase("idle");
    },
    saveAvailable: () => !currentBlocking,
    labels: {
      title: "workflow.detail.guard.title",
      body: "workflow.detail.guard.body",
      keepEditing: "workflow.detail.guard.keepEditing",
      discard: "workflow.detail.guard.discard",
      save: "workflow.detail.guard.save",
      saveNote: "workflow.detail.guard.saveNote",
    },
  });

  // ---- The tablist keyboard pattern (§8.8): Arrow keys move among the
  // enabled tabs.
  const onTabListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
    event.preventDefault();
    const index = ENABLED_TABS.findIndex((tab) => tab.id === activeTab);
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = ENABLED_TABS[(index + delta + ENABLED_TABS.length) % ENABLED_TABS.length];
    setActiveTab(next.id);
    const nextButton = tabListRef.current?.querySelector<HTMLButtonElement>(`[data-tab-id="${next.id}"]`);
    nextButton?.focus();
  };

  // The list arrow-key traversal (B5's keyboard-selection structure): the
  // rows are natural tab stops; the arrow keys additionally move focus
  // within the list.
  const onListKeyDown = (event: ReactKeyboardEvent<HTMLUListElement>) => {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    const rows = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("button[data-row]"));
    if (rows.length === 0) return;
    event.preventDefault();
    const index = rows.indexOf(document.activeElement as HTMLButtonElement);
    const delta = event.key === "ArrowDown" ? 1 : -1;
    const next = rows[(index + delta + rows.length) % rows.length];
    next?.focus();
  };

  // The undo/redo keyboard route (A-10; the §29 platform-modifier
  // convention — exactly the commands the keyboard contract documents).
  // The §4.9 shortcut safeguards: a focused text field/select keeps its
  // native editing semantics (the binding never fires there), and an
  // active IME composition is never intercepted.
  const onBuilderKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    const modifier = event.ctrlKey || event.metaKey;
    if (!modifier) return;
    const key = event.key.toLowerCase();
    const isUndo = key === "z" && !event.shiftKey;
    const isRedo = key === "z" && event.shiftKey;
    if (!isUndo && !isRedo) return;
    const target = event.target;
    if (
      target instanceof HTMLInputElement ||
      target instanceof HTMLSelectElement ||
      target instanceof HTMLTextAreaElement ||
      (target instanceof HTMLElement && target.isContentEditable)
    ) {
      return;
    }
    event.preventDefault();
    if (isUndo) undo();
    else redo();
  };

  if (inspectPhase === "loading") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        <p className={styles.note}>{t("workflow.list.loading")}</p>
      </PageLayout>
    );
  }

  if (inspectPhase === "failure" && inspectFailure !== null) {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        {notFound ? (
          <RecoveryPanel
            title={t("workflow.detail.notFoundTitle")}
            body={t("workflow.detail.notFoundBody")}
            note={t("workflow.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[
              { label: t("workflow.detail.back"), onAction: () => window.history.back() },
            ]}
          />
        ) : (
          <RecoveryPanel
            title={t("workflow.detail.failureTitle")}
            body={t("workflow.detail.failureBody")}
            note={t("workflow.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
          />
        )}
        <p className={styles.code}>
          <span className="visually-hidden">Code: </span>
          <code>{inspectFailure.code}</code>
        </p>
      </PageLayout>
    );
  }

  if (inspectPhase === "unknown") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/workflows">{t("workflow.detail.back")}</Link>
        </p>
        <PageHeading>{t("workflow.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("workflow.detail.unknownTitle")}
          body={t("workflow.detail.unknownBody")}
          note={t("workflow.detail.connectionHint")}
          actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
        />
      </PageLayout>
    );
  }

  // inspectPhase === "data" here; definition is non-null by construction.
  const shownDefinition = definition as WorkflowDefinition;
  const shownName =
    shownDefinition.display_name === undefined || shownDefinition.display_name === ""
      ? shownDefinition.definition_id
      : shownDefinition.display_name;
  const storedRevisionRef =
    shownDefinition.current_revision_ref === undefined || shownDefinition.current_revision_ref === ""
      ? null
      : shownDefinition.current_revision_ref;
  const selectedNode =
    selectedNodeRef !== null ? draft.nodes.find((node) => node.node_ref === selectedNodeRef) ?? null : null;
  const selectedEdge =
    selectedEdgeRef !== null ? draft.edges.find((edge) => edge.edge_ref === selectedEdgeRef) ?? null : null;
  const connectEligible = draft.nodes.length >= 2;

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to="/workflows">{t("workflow.detail.back")}</Link>
      </p>
      <PageHeading>{shownName}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("workflow.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}
      <div className={styles.toolbar}>
        <button type="button" className={styles.control} onClick={readAll}>
          {t("workflow.detail.refresh")}
        </button>
      </div>
      {/* B20 — the surface's recorded state after the UI-30 resolution:
          the editing/revision-saving controls are LIVE; no Run control
          exists; the AI-proposal producer stays unwired. */}
      <p className={styles.gateNote}>{t("workflow.detail.gateNote")}</p>
      {/* The AI-proposal member's honest unmet-producer state (B4-A1 §2.4
          gap 1): the typed refusal is the served behavior; no proposal
          surface is fabricated. */}
      <p className={styles.gateNote}>{t("workflow.detail.aiProposalNote")}</p>
      <p role="status" className="visually-hidden">
        {announcement}
      </p>

      <section className={styles.factsPanel} aria-label={t("workflow.detail.factsDefinitionId")}>
        <h2 className={styles.subHeading}>{t("workflow.list.heading")}</h2>
        <dl className={styles.facts}>
          <div>
            <dt>{t("workflow.detail.factsDefinitionId")}</dt>
            <dd>
              <CopyIdButton
                id={shownDefinition.definition_id}
                label={t("workflow.detail.copyDefinitionId")}
                copiedLabel={t("workflow.list.copied")}
                failedLabel={t("workflow.list.copyFailed")}
                className={styles.copyId}
              />
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsDisplayName")}</dt>
            <dd>
              {shownDefinition.display_name === undefined || shownDefinition.display_name === ""
                ? unavailable
                : shownDefinition.display_name}
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsCurrentRevision")}</dt>
            <dd>
              {storedRevisionRef === null ? (
                <span className={styles.muted}>{t("workflow.detail.factsNoRevision")}</span>
              ) : (
                <code>{storedRevisionRef}</code>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsCreatedAt")}</dt>
            <dd>
              <time dateTime={shownDefinition.created_at}>
                {formatTimestamp(new Date(shownDefinition.created_at), locale)}
              </time>
            </dd>
          </div>
          <div>
            <dt>{t("workflow.detail.factsUpdatedAt")}</dt>
            <dd>
              {shownDefinition.updated_at === undefined ? (
                unavailable
              ) : (
                <time dateTime={shownDefinition.updated_at}>
                  {formatTimestamp(new Date(shownDefinition.updated_at), locale)}
                </time>
              )}
            </dd>
          </div>
        </dl>
      </section>

      {/* The authoritative state endpoint's read (a pure query — never a
          retry; the reconcile anchor). */}
      <section className={styles.factsPanel} aria-label={t("workflow.detail.stateHeading")}>
        <h2 className={styles.subHeading}>{t("workflow.detail.stateHeading")}</h2>
        <p className={styles.note}>{t("workflow.detail.stateNote")}</p>
        {statePhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}
        {statePhase === "failure" && stateFailure !== null ? (
          <>
            <RecoveryPanel
              title={t("workflow.detail.failureTitle")}
              body={t("workflow.detail.failureBody")}
              note={t("workflow.detail.connectionHint")}
              referenceId={stateFailure.correlationId}
              actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{stateFailure.code}</code>
            </p>
          </>
        ) : null}
        {statePhase === "unknown" ? (
          <RecoveryPanel
            title={t("workflow.detail.unknownTitle")}
            body={t("workflow.detail.unknownBody")}
            note={t("workflow.detail.connectionHint")}
            actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
          />
        ) : null}
        {statePhase === "data" && state !== null ? (
          <dl className={styles.facts}>
            <div>
              <dt>{t("workflow.detail.stateCurrentRevision")}</dt>
              <dd>
                {state.current_revision_ref === undefined || state.current_revision_ref === "" ? (
                  <span className={styles.muted}>{t("workflow.detail.stateNoneStored")}</span>
                ) : (
                  <code>{state.current_revision_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("workflow.detail.stateUpdatedAt")}</dt>
              <dd>
                {state.updated_at === undefined ? (
                  unavailable
                ) : (
                  <time dateTime={state.updated_at}>{formatTimestamp(new Date(state.updated_at), locale)}</time>
                )}
              </dd>
            </div>
          </dl>
        ) : null}
      </section>

      <div
        className={styles.tabList}
        role="tablist"
        aria-label={shownName}
        ref={tabListRef}
        onKeyDown={onTabListKeyDown}
      >
        {ENABLED_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`workflow-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`workflow-panel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={activeTab === tab.id ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(tab.id)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>

      {activeTab === "builder" ? (
        <section
          role="tabpanel"
          id="workflow-panel-builder"
          aria-labelledby="workflow-tab-builder"
          className={styles.panel}
          onKeyDown={onBuilderKeyDown}
        >
          {/* The three-state spine (P4.9 §3.0): the stored revision, the
              draft's dirty state, and the run-revision distinctness —
              labeled text, never color alone. */}
          <p className={styles.note}>{t("workflow.detail.builder.helpNote")}</p>
          <p className={styles.note}>{t("workflow.detail.builder.threeStateNote")}</p>
          <dl className={styles.facts}>
            <div>
              <dt>{t("workflow.detail.builder.storedRevisionLabel")}</dt>
              <dd>
                {storedRevisionRef === null ? (
                  <span className={styles.muted}>{t("workflow.detail.stateNoneStored")}</span>
                ) : (
                  <code>{storedRevisionRef}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("workflow.detail.builder.draftStateLabel")}</dt>
              <dd>{draftDirty ? t("workflow.detail.builder.draftDirty") : t("workflow.detail.builder.draftClean")}</dd>
            </div>
          </dl>

          {/* The conflict banner (the interim rule: the conflict persists
              until the user resolves it — never auto-resolved). The
              resolution dialog opens from here. */}
          {savePhase === "conflict" ? (
            <div className={styles.conflictBanner}>
              <p className={styles.outcomeTitle}>{t("workflow.detail.builder.conflictTitle")}</p>
              <p>{t("workflow.detail.builder.conflictBody")}</p>
              <dl className={styles.facts}>
                <div>
                  <dt>{t("workflow.detail.builder.conflictRemoteLabel")}</dt>
                  <dd>
                    {conflictRemoteRevision === null ? (
                      <span className={styles.muted}>{unavailable}</span>
                    ) : (
                      <code>{conflictRemoteRevision}</code>
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t("workflow.detail.builder.conflictDraftLabel")}</dt>
                  <dd>{t("workflow.detail.builder.draftDirty")}</dd>
                </div>
              </dl>
              <p className={styles.note}>{t("workflow.detail.builder.conflictChoicesNote")}</p>
              <div className={styles.toolbar}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={() => setConflictReloadConfirmOpen(true)}
                >
                  {t("workflow.detail.builder.conflictReload")}
                </button>
                <button type="button" className={styles.control} onClick={keepConflict}>
                  {t("workflow.detail.builder.conflictKeepEditing")}
                </button>
              </div>
            </div>
          ) : null}

          {/* The save outcomes (inline; the Core-confirmed truth only).
              The confirmed state renders the STATED new reference; the
              typed refusals render verbatim with the draft visibly
              surviving; the uncertain state reconciles through the
              authoritative state read before any retry. */}
          {savePhase === "submitting" ? <p className={styles.fieldNote}>{t("workflow.detail.builder.savePending")}</p> : null}
          {savePhase === "confirmed" && saveResult !== null ? (
            <div className={styles.outcomeBlock}>
              <p className={styles.outcomeTitle}>{t("workflow.detail.builder.saveConfirmedTitle")}</p>
              <p>
                {saveResult.replayed
                  ? t("workflow.detail.builder.saveReplayedBody")
                  : saveResult.viaReconcile
                    ? t("workflow.detail.builder.saveReconcileLandedBody")
                    : t("workflow.detail.builder.saveConfirmedBody")}
              </p>
              <dl className={styles.facts}>
                <div>
                  <dt>{t("workflow.detail.builder.saveNewRefLabel")}</dt>
                  <dd>
                    <code>{saveResult.revisionRef}</code>
                  </dd>
                </div>
              </dl>
            </div>
          ) : null}
          {savePhase === "validationRefused" && saveFailure !== null ? (
            <div className={styles.outcomeBlock}>
              <p className={styles.outcomeTitle}>{t("workflow.detail.builder.saveValidationRefusedTitle")}</p>
              <p>{t("workflow.detail.builder.saveValidationRefusedBody")}</p>
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{saveFailure.code}</code>
              </p>
            </div>
          ) : null}
          {savePhase === "failure" && saveFailure !== null ? (
            <div className={styles.outcomeBlock}>
              <p className={styles.outcomeTitle}>{t("workflow.detail.builder.saveFailureTitle")}</p>
              <p>{t("workflow.detail.builder.saveFailureBody")}</p>
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{saveFailure.code}</code>
              </p>
            </div>
          ) : null}
          {savePhase === "unknown" && saveUnknown !== null ? (
            <div className={styles.outcomeBlock}>
              <RecoveryPanel
                title={t("workflow.detail.builder.saveUnknownTitle")}
                body={t("workflow.detail.builder.saveUnknownBody")}
                referenceId={saveUnknown.correlationId}
              />
              {saveReconcileState === "unchanged" ? (
                <p>{t("workflow.detail.builder.saveReconcileUnchangedBody")}</p>
              ) : null}
              {saveReconcileFailed ? <p className={styles.fieldNote}>{t("workflow.detail.builder.saveReconcileFailed")}</p> : null}
              <div className={styles.toolbar}>
                {saveReconcileState === "unchanged" ? (
                  <button type="button" className={styles.control} onClick={() => submitSave(saveUnknown)}>
                    {t("workflow.detail.builder.saveReissue")}
                  </button>
                ) : (
                  <button type="button" className={styles.control} onClick={reconcileSave}>
                    {t("workflow.detail.builder.saveReconcile")}
                  </button>
                )}
                <button type="button" className={styles.control} onClick={() => setSavePhase("idle")}>
                  {t("actions.close")}
                </button>
              </div>
            </div>
          ) : null}

          {/* Region 1 — the palette (P4.7 §3.3's source law): the ONLY
              source is the Core-contracted node vocabulary; none is
              published, so the honest empty state renders and the add
              action is disabled-with-reason. No type list is invented; no
              free-text type entry exists. */}
          <section className={styles.builderRegion} aria-labelledby="workflow-builder-palette-heading">
            <h3 id="workflow-builder-palette-heading" className={styles.subHeading}>
              {t("workflow.detail.builder.paletteHeading")}
            </h3>
            <p className={styles.note}>{t("workflow.detail.builder.paletteSourceNote")}</p>
            <button
              type="button"
              className={styles.control}
              disabled
              aria-describedby="workflow-builder-add-disabled-reason"
            >
              {t("workflow.detail.builder.addControl")}
            </button>
            <p id="workflow-builder-add-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.addDisabledReason")}
            </p>
          </section>

          {/* The toolbar (§17.7: only implemented controls). Reorder,
              zoom/fit, and clipboard/duplicate are omitted entirely (the
              omission law — the served shape carries no ordering member
              and no spatial presentation exists). */}
          <div className={styles.toolbar}>
            <button
              type="button"
              className={styles.control}
              onClick={openConnect}
              disabled={!connectEligible}
              aria-describedby={connectEligible ? undefined : "workflow-builder-connect-disabled-reason"}
            >
              {t("workflow.detail.builder.connectControl")}
            </button>
            <button
              type="button"
              className={styles.control}
              onClick={deleteSelected}
              disabled={selectedNodeRef === null && selectedEdgeRef === null}
              aria-describedby={selectedNodeRef === null && selectedEdgeRef === null ? "workflow-builder-delete-disabled-reason" : undefined}
            >
              {t("workflow.detail.builder.deleteControl")}
            </button>
            <button
              type="button"
              className={styles.control}
              onClick={undo}
              disabled={past.length === 0}
              aria-describedby={past.length === 0 ? "workflow-builder-undo-disabled-reason" : undefined}
            >
              {t("workflow.detail.builder.undoControl")}
            </button>
            <button
              type="button"
              className={styles.control}
              onClick={redo}
              disabled={future.length === 0}
              aria-describedby={future.length === 0 ? "workflow-builder-redo-disabled-reason" : undefined}
            >
              {t("workflow.detail.builder.redoControl")}
            </button>
            <button type="button" className={styles.control} onClick={runValidation} disabled={validatePhase === "pending"}>
              {validatePhase === "pending" ? t("workflow.detail.validatePending") : t("workflow.detail.validateControl")}
            </button>
            <button
              type="button"
              className={styles.control}
              onClick={() => submitSave()}
              disabled={savePhase === "submitting" || currentBlocking}
              aria-describedby={currentBlocking ? "workflow-builder-save-blocked-reason" : undefined}
            >
              {t("workflow.detail.builder.saveControl")}
            </button>
          </div>
          {!connectEligible ? (
            <p id="workflow-builder-connect-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.connectDisabledReason")}
            </p>
          ) : null}
          {selectedNodeRef === null && selectedEdgeRef === null ? (
            <p id="workflow-builder-delete-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.deleteDisabledReason")}
            </p>
          ) : null}
          {past.length === 0 ? (
            <p id="workflow-builder-undo-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.undoDisabledReason")}
            </p>
          ) : null}
          {future.length === 0 ? (
            <p id="workflow-builder-redo-disabled-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.redoDisabledReason")}
            </p>
          ) : null}
          {currentBlocking ? (
            <p id="workflow-builder-save-blocked-reason" className={styles.actionReason}>
              {t("workflow.detail.builder.saveBlockedReason")}
            </p>
          ) : null}
          <p className={styles.actionReason}>{t("workflow.detail.builder.undoNote")}</p>
          <p className={styles.actionReason}>{t("workflow.detail.builder.deleteNodeNote")}</p>

          {/* Regions 2+3 — the structured node and edge lists (P4.8 §3.5's
              AT-suitable core: every node and relationship addressable
              WITHOUT spatial position or color) and the inspector. One
              data, two views: the selection made here drives the
              inspector predictably. */}
          <div className={styles.builderColumns}>
            <div>
              <section aria-labelledby="workflow-builder-nodes-heading">
                <h3 id="workflow-builder-nodes-heading" className={styles.subHeading}>
                  {t("workflow.detail.builder.nodesHeading")}
                </h3>
                <p className={styles.actionReason}>{t("workflow.detail.builder.selectHint")}</p>
                {draft.nodes.length === 0 ? (
                  <div className={styles.emptyState}>
                    <p>{t("workflow.detail.builder.nodesEmpty")}</p>
                  </div>
                ) : (
                  <ul className={styles.rowList} ref={nodeListRef} onKeyDown={onListKeyDown} tabIndex={-1}>
                    {draft.nodes.map((node) => {
                      const findings = groupedIssues?.byNode.get(node.node_ref) ?? [];
                      return (
                        <li key={node.node_ref}>
                          <button
                            type="button"
                            data-row="node"
                            data-node-ref={node.node_ref}
                            className={styles.rowButton}
                            aria-pressed={selectedNodeRef === node.node_ref}
                            onClick={() => selectNode(node.node_ref)}
                            onKeyDown={(event) => {
                              if (event.key === "Delete" || event.key === "Backspace") {
                                event.preventDefault();
                                deleteNode(node.node_ref);
                              }
                            }}
                          >
                            <span className="visually-hidden">
                              {selectedNodeRef === node.node_ref ? `${t("workflow.detail.builder.draftStateLabel")}: ` : ""}
                            </span>
                            {t("workflow.detail.builder.kindNode")} <code>{node.node_ref}</code> —{" "}
                            {t("workflow.detail.builder.nodeTypeLabel")}: <code>{node.node_type}</code>
                            {findings.length > 0 ? (
                              <span className={styles.rowFindings}> ({findings.length})</span>
                            ) : null}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </section>
              <section aria-labelledby="workflow-builder-edges-heading">
                <h3 id="workflow-builder-edges-heading" className={styles.subHeading}>
                  {t("workflow.detail.builder.edgesHeading")}
                </h3>
                {draft.edges.length === 0 ? (
                  <div className={styles.emptyState}>
                    <p>{t("workflow.detail.builder.edgesEmpty")}</p>
                  </div>
                ) : (
                  <ul className={styles.rowList} ref={edgeListRef} onKeyDown={onListKeyDown}>
                    {draft.edges.map((edge) => {
                      const findings = groupedIssues?.byEdge.get(edge.edge_ref) ?? [];
                      return (
                        <li key={edge.edge_ref}>
                          <button
                            type="button"
                            data-row="edge"
                            data-edge-ref={edge.edge_ref}
                            className={styles.rowButton}
                            aria-pressed={selectedEdgeRef === edge.edge_ref}
                            onClick={() => selectEdge(edge.edge_ref)}
                            onKeyDown={(event) => {
                              if (event.key === "Delete" || event.key === "Backspace") {
                                event.preventDefault();
                                deleteEdge(edge.edge_ref);
                              }
                            }}
                          >
                            <span className="visually-hidden">
                              {selectedEdgeRef === edge.edge_ref ? `${t("workflow.detail.builder.draftStateLabel")}: ` : ""}
                            </span>
                            <code>{edge.from_node}</code> → <code>{edge.to_node}</code>
                            {findings.length > 0 ? (
                              <span className={styles.rowFindings}> ({findings.length})</span>
                            ) : null}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </section>
            </div>

            {/* The inspector (A-08): the per-node structured view — the
                fields render ONLY where the Core supplies them; the ten
                contract dimensions render their honest Unavailable (the
                display law); the validation findings associate with the
                selected element. */}
            <section className={styles.builderRegion} aria-labelledby="workflow-builder-inspector-heading">
              <h3 id="workflow-builder-inspector-heading" className={styles.subHeading}>
                {t("workflow.detail.builder.inspectorHeading")}
              </h3>
              {selectedNode === null && selectedEdge === null ? (
                <div className={styles.emptyState}>
                  <p>{t("workflow.detail.builder.inspectorEmpty")}</p>
                </div>
              ) : selectedNode !== null ? (
                <dl className={styles.facts}>
                  <div>
                    <dt>{t("workflow.detail.builder.nodeRefLabel")}</dt>
                    <dd>
                      <code>{selectedNode.node_ref}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t("workflow.detail.builder.nodeTypeLabel")}</dt>
                    <dd>
                      <code>{selectedNode.node_type}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t("workflow.detail.builder.configLabel")}</dt>
                    <dd>
                      {Object.keys(selectedNode.config).length === 0 ? (
                        <span className={styles.muted}>{t("workflow.detail.builder.configEmpty")}</span>
                      ) : (
                        <pre className={styles.configBlock}>{JSON.stringify(selectedNode.config, null, 2)}</pre>
                      )}
                    </dd>
                  </div>
                </dl>
              ) : selectedEdge !== null ? (
                <dl className={styles.facts}>
                  <div>
                    <dt>{t("workflow.detail.builder.edgeRefLabel")}</dt>
                    <dd>
                      <code>{selectedEdge.edge_ref}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t("workflow.detail.builder.edgeFromLabel")}</dt>
                    <dd>
                      <code>{selectedEdge.from_node}</code>
                    </dd>
                  </div>
                  <div>
                    <dt>{t("workflow.detail.builder.edgeToLabel")}</dt>
                    <dd>
                      <code>{selectedEdge.to_node}</code>
                    </dd>
                  </div>
                </dl>
              ) : null}
              {selectedNode !== null || selectedEdge !== null ? (
                <>
                  <p className={styles.actionReason}>{t("workflow.detail.builder.contractUnavailable")}</p>
                  {selectedNode !== null ? (
                    <>
                      <h4 className={styles.inspectorSubHeading}>{t("workflow.detail.builder.validationFindingsNode")}</h4>
                      <ValidationFindings
                        issues={groupedIssues?.byNode.get(selectedNode.node_ref) ?? []}
                        noneLabel={t("workflow.detail.builder.validationFindingsNone", { kind: t("workflow.detail.builder.kindNode") })}
                      />
                    </>
                  ) : selectedEdge !== null ? (
                    <>
                      <h4 className={styles.inspectorSubHeading}>{t("workflow.detail.builder.validationFindingsEdge")}</h4>
                      <ValidationFindings
                        issues={groupedIssues?.byEdge.get(selectedEdge.edge_ref) ?? []}
                        noneLabel={t("workflow.detail.builder.validationFindingsNone", { kind: t("workflow.detail.builder.kindEdge") })}
                      />
                    </>
                  ) : null}
                </>
              ) : null}
              {selectedNode !== null ? (
                <p className={styles.actionReason}>{t("workflow.detail.builder.nodeTypeNote")}</p>
              ) : null}
            </section>
          </div>

          {/* The keyboard contract (§17.7: documented beside the surface;
              it agrees with the actually-exposed control set). */}
          <section aria-labelledby="workflow-builder-commands-heading" className={styles.builderRegion}>
            <h3 id="workflow-builder-commands-heading" className={styles.subHeading}>
              {t("workflow.detail.builder.commandsHeading")}
            </h3>
            <ul className={styles.commandList}>
              <li>{t("workflow.detail.builder.kbdSelect")}</li>
              <li>{t("workflow.detail.builder.kbdDelete")}</li>
              <li>{t("workflow.detail.builder.kbdUndo")}</li>
              <li>{t("workflow.detail.builder.kbdRedo")}</li>
            </ul>
          </section>
        </section>
      ) : null}

      {activeTab === "validation" ? (
        <section
          role="tabpanel"
          id="workflow-panel-validation"
          aria-labelledby="workflow-tab-validation"
          className={styles.panel}
          ref={validationPanelRef}
          tabIndex={-1}
        >
          <p className={styles.note}>{t("workflow.detail.validationAuthoritativeNote")}</p>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={runValidation} disabled={validatePhase === "pending"}>
              {validatePhase === "pending" ? t("workflow.detail.validatePending") : t("workflow.detail.validateControl")}
            </button>
          </div>
          {validatePhase === "unknown" ? (
            <>
              <RecoveryPanel
                title={t("workflow.detail.validateUnknownTitle")}
                body={t("workflow.detail.validateUnknownBody")}
                actions={[{ label: t("workflow.detail.validateRevalidate"), onAction: runValidation }]}
              />
            </>
          ) : null}
          {validatePhase === "failure" && validateFailure !== null ? (
            <>
              <RecoveryPanel
                title={t("workflow.detail.validateFailureTitle")}
                body={t("workflow.detail.validateFailureBody")}
                note={t("workflow.detail.connectionHint")}
                referenceId={validateFailure.correlationId}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{validateFailure.code}</code>
              </p>
            </>
          ) : null}
          {validatePhase !== "unknown" && validatePhase !== "failure" && validationResult === null ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyTitle}>{t("workflow.detail.validationNoDraftTitle")}</p>
              <p>{t("workflow.detail.validationNoDraftBody")}</p>
            </div>
          ) : null}
          {validationResult !== null ? (
            <>
              {/* The exact-object label (RUN-LINK-2): the result labels
                  EXACTLY which draft it evaluated. */}
              <p className={styles.fieldNote}>
                {t("workflow.detail.validationEvaluatedLabel", { objectRef: validationResult.evaluated_object_ref })}
              </p>
              {validationStale ? <p className={styles.fieldNote}>{t("workflow.detail.validationStaleNote")}</p> : null}
              <ValidationClassSection
                headingKey="workflow.detail.validationBlockingHeading"
                issues={validationResult.blocking_errors}
                onNavigate={navigateToIssue}
              />
              <ValidationClassSection
                headingKey="workflow.detail.validationWarningsHeading"
                issues={validationResult.warnings}
                onNavigate={navigateToIssue}
              />
              <ValidationClassSection
                headingKey="workflow.detail.validationPermissionGapsHeading"
                issues={validationResult.permission_gaps}
                onNavigate={navigateToIssue}
              />
              <ValidationClassSection
                headingKey="workflow.detail.validationSchemaCompatibilityHeading"
                issues={validationResult.schema_compatibility}
                onNavigate={navigateToIssue}
              />
              {validationResult.blocking_errors.length === 0 ? (
                <div className={styles.emptyState}>
                  <p className={styles.emptyTitle}>{t("workflow.detail.validationPassTitle")}</p>
                  <p>{t("workflow.detail.validationPassBody")}</p>
                </div>
              ) : null}
            </>
          ) : null}
        </section>
      ) : null}

      {activeTab === "versions" ? (
        <section
          role="tabpanel"
          id="workflow-panel-versions"
          aria-labelledby="workflow-tab-versions"
          className={styles.panel}
        >
          <p className={styles.note}>{t("workflow.detail.versionsImmutableNote")}</p>
          <p className={styles.note}>{t("workflow.detail.versionsContentNote")}</p>
          {revisions.length === 0 ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyTitle}>{t("workflow.detail.versionsEmpty")}</p>
            </div>
          ) : (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th scope="col">{t("workflow.detail.versionsColumnRevision")}</th>
                  <th scope="col">{t("workflow.detail.versionsColumnCreatedAt")}</th>
                </tr>
              </thead>
              <tbody>
                {revisions.map((revision) => (
                  <tr key={revision.revision_ref}>
                    <td>
                      <CopyIdButton
                        id={revision.revision_ref}
                        label={t("workflow.detail.copyRevisionRef")}
                        copiedLabel={t("workflow.list.copied")}
                        failedLabel={t("workflow.list.copyFailed")}
                        className={styles.copyId}
                      />
                    </td>
                    <td>
                      <time dateTime={revision.created_at}>
                        {formatTimestamp(new Date(revision.created_at), locale)}
                      </time>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      ) : null}

      {activeTab === "runs" ? (
        <section
          role="tabpanel"
          id="workflow-panel-runs"
          aria-labelledby="workflow-tab-runs"
          className={styles.panel}
        >
          <p className={styles.note}>{t("workflow.detail.runsD40Note")}</p>
          {runsPhase === "loading" ? <p className={styles.note}>{t("workflow.list.loading")}</p> : null}
          {runsPhase === "failure" && runsFailure !== null ? (
            <>
              <RecoveryPanel
                title={t("workflow.detail.failureTitle")}
                body={t("workflow.detail.failureBody")}
                note={t("workflow.detail.connectionHint")}
                referenceId={runsFailure.correlationId}
                actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
              />
              <p className={styles.code}>
                <span className="visually-hidden">Code: </span>
                <code>{runsFailure.code}</code>
              </p>
            </>
          ) : null}
          {runsPhase === "unknown" ? (
            <RecoveryPanel
              title={t("workflow.detail.unknownTitle")}
              body={t("workflow.detail.unknownBody")}
              note={t("workflow.detail.connectionHint")}
              actions={[{ label: t("workflow.detail.refresh"), onAction: readAll }]}
            />
          ) : null}
          {runsPhase === "data" ? (
            runEntries.length === 0 ? (
              // The honest empty page while the D-40 gate stands: the
              // linkage read's producer is the GATED run execution (B4-A1
              // §2.1 #7) — no run record is fabricated.
              <div className={styles.emptyState}>
                <p className={styles.emptyTitle}>{t("workflow.detail.runsEmpty")}</p>
                <p>{t("workflow.detail.runsEmptyBody")}</p>
              </div>
            ) : (
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th scope="col">{t("workflow.detail.runsColumnRunRef")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnSourceRevision")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnLinkage")}</th>
                    <th scope="col">{t("workflow.detail.runsColumnCreatedAt")}</th>
                  </tr>
                </thead>
                <tbody>
                  {runEntries.map((entry) => {
                    const linkage = workflowLinkageChip(entry.linkage_state);
                    const LinkageIcon = linkage.icon;
                    return (
                      <tr key={entry.run_ref}>
                        <td>
                          <code>{entry.run_ref}</code>
                        </td>
                        <td>
                          {/* The link renders ONLY as the Core reports it;
                              missing linkage renders Unavailable — never an
                              inferred link (RUN-LINK-3). */}
                          {entry.source_revision === undefined || entry.source_revision === "" ? (
                            <span className={styles.muted}>{unavailable}</span>
                          ) : (
                            <code>{entry.source_revision}</code>
                          )}
                        </td>
                        <td>
                          <span className={`${styles.chip} ${styles[linkage.tone]}`}>
                            <LinkageIcon aria-hidden="true" size={14} />
                            {t(linkage.labelKey)}
                          </span>
                        </td>
                        <td>
                          {entry.created_at === undefined ? (
                            <span className={styles.muted}>{unavailable}</span>
                          ) : (
                            <time dateTime={entry.created_at}>
                              {formatTimestamp(new Date(entry.created_at), locale)}
                            </time>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )
          ) : null}
        </section>
      ) : null}

      {activeTab === "permissions" ? (
        <section
          role="tabpanel"
          id="workflow-panel-permissions"
          aria-labelledby="workflow-tab-permissions"
          className={styles.panel}
        >
          {/* P4.7 §3.2's display law: the node contract fields render ONLY
              where the Core supplies them; no per-node contract is
              supplied yet, so the honest absence renders — nothing is
              invented. */}
          <div className={styles.emptyState}>
            <p>{t("workflow.detail.permissionsNote")}</p>
          </div>
        </section>
      ) : null}

      {/* The connect dialog (A-03's explicit connect command: source →
          target selection WITHOUT any drag; keyboard-operable throughout).
          The guards prevent the self-edge and the duplicate edge by
          construction — the Core re-checks authoritatively. */}
      <ConnectDialog
        open={connectOpen}
        nodes={draft.nodes}
        source={connectSource}
        target={connectTarget}
        error={connectError}
        onClose={() => setConnectOpen(false)}
        onSourceChange={setConnectSource}
        onTargetChange={setConnectTarget}
        onConfirm={confirmConnect}
      />

      {/* The conflict resolution dialog (§8.9 class: the choice list is
          keyboard-comparable; the reload choice is explicit and confirmed,
          never default). */}
      <Dialog
        labelId="workflow-conflict-dialog-label"
        title={t("workflow.detail.builder.conflictReloadConfirm")}
        open={conflictReloadConfirmOpen}
        onClose={keepConflict}
      >
        <p>{t("workflow.detail.builder.conflictReloadConfirm")}</p>
        <div className={styles.toolbar}>
          <button type="button" className={styles.control} onClick={resolveConflictByReload}>
            {t("workflow.detail.builder.conflictReloadAction")}
          </button>
          <button type="button" className={styles.control} onClick={keepConflict}>
            {t("workflow.detail.builder.conflictKeepEditing")}
          </button>
        </div>
      </Dialog>

      {guard.guardDialog}
    </PageLayout>
  );
}

/** One typed validation class's section (the Core's own four-class
 *  register; the UI invents no category — unknown reported values render
 *  verbatim). Each issue links to its offending node/edge — the
 *  validation navigation (B19: the issue moves the user to the offending
 *  element with focus landing predictably). */
function ValidationClassSection(props: {
  readonly headingKey: TranslationKey;
  readonly issues: readonly WorkflowValidationIssue[];
  readonly onNavigate: (target: { readonly kind: "node" | "edge"; readonly ref: string }) => void;
}) {
  const t = useT();
  return (
    <section>
      <h3 className={styles.inspectorSubHeading}>
        {t(props.headingKey)} ({props.issues.length})
      </h3>
      {props.issues.length === 0 ? (
        <p className={styles.actionReason}>{t("workflow.detail.validationNone")}</p>
      ) : (
        <ul className={styles.commandList}>
          {props.issues.map((issue, index) => {
            const chip = workflowIssueClassChip(issue.issue_class);
            const ChipIcon = chip.icon;
            const linkRef = typeof issue.node_ref === "string" && issue.node_ref !== "" ? issue.node_ref : null;
            const edgeRef = typeof issue.edge_ref === "string" && issue.edge_ref !== "" ? issue.edge_ref : null;
            return (
              <li key={`${issue.issue_class}-${index}`}>
                <span className={`${styles.chip} ${styles[chip.tone]}`}>
                  <ChipIcon aria-hidden="true" size={14} />
                  {t(chip.labelKey)}
                </span>{" "}
                {issue.message}
                {typeof issue.code === "string" && issue.code !== "" ? (
                  <>
                    {" "}
                    <code>{issue.code}</code>
                  </>
                ) : null}
                {linkRef !== null || edgeRef !== null ? (
                  <button
                    type="button"
                    className={styles.linkButton}
                    onClick={() => {
                      if (linkRef !== null) props.onNavigate({ kind: "node", ref: linkRef });
                      else if (edgeRef !== null) props.onNavigate({ kind: "edge", ref: edgeRef });
                    }}
                  >
                    {linkRef !== null
                      ? t("workflow.detail.validationGoToNode", { nodeRef: linkRef })
                      : t("workflow.detail.validationGoToEdge", { edgeRef: edgeRef ?? "" })}
                  </button>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

/** The inspector's findings block for the selected node/edge. */
function ValidationFindings(props: { readonly issues: readonly WorkflowValidationIssue[]; readonly noneLabel: string }) {
  const t = useT();
  if (props.issues.length === 0) {
    return <p className={styles.actionReason}>{props.noneLabel}</p>;
  }
  return (
    <ul className={styles.commandList}>
      {props.issues.map((issue, index) => {
        const chip = workflowIssueClassChip(issue.issue_class);
        const ChipIcon = chip.icon;
        return (
          <li key={`${issue.issue_class}-${index}`}>
            <span className={`${styles.chip} ${styles[chip.tone]}`}>
              <ChipIcon aria-hidden="true" size={14} />
              {t(chip.labelKey)}
            </span>{" "}
            {issue.message}
          </li>
        );
      })}
    </ul>
  );
}

/** The connect dialog (A-03; the explicit non-drag connect command). The
 *  selects are native (keyboard-operable; the §8.2 form contract). */
function ConnectDialog(props: {
  readonly open: boolean;
  readonly nodes: readonly WorkflowDraftNode[];
  readonly source: string;
  readonly target: string;
  readonly error: ConnectError;
  readonly onClose: () => void;
  readonly onSourceChange: (value: string) => void;
  readonly onTargetChange: (value: string) => void;
  readonly onConfirm: () => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  const sourceId = "workflow-connect-source";
  const targetId = "workflow-connect-target";
  const errorId = "workflow-connect-error";

  return (
    <Dialog labelId={labelId} title={t("workflow.detail.builder.connectTitle")} open={props.open} onClose={props.onClose}>
      {props.nodes.length < 2 ? (
        <p className={styles.fieldNote}>{t("workflow.detail.builder.connectDisabledReason")}</p>
      ) : (
        <div>
          <div className={styles.field}>
            <label htmlFor={sourceId}>{t("workflow.detail.builder.connectSourceLabel")}</label>
            <select
              id={sourceId}
              value={props.source}
              onChange={(event) => props.onSourceChange(event.target.value)}
            >
              <option value="">—</option>
              {props.nodes.map((node) => (
                <option key={node.node_ref} value={node.node_ref}>
                  {node.node_ref} ({node.node_type})
                </option>
              ))}
            </select>
          </div>
          <div className={styles.field}>
            <label htmlFor={targetId}>{t("workflow.detail.builder.connectTargetLabel")}</label>
            <select
              id={targetId}
              value={props.target}
              onChange={(event) => props.onTargetChange(event.target.value)}
            >
              <option value="">—</option>
              {props.nodes.map((node) => (
                <option key={node.node_ref} value={node.node_ref}>
                  {node.node_ref} ({node.node_type})
                </option>
              ))}
            </select>
          </div>
          {props.error !== "none" ? (
            <p id={errorId} className={styles.actionReason} role="alert">
              {props.error === "sameNode"
                ? t("workflow.detail.builder.connectSameNodeError")
                : t("workflow.detail.builder.connectDuplicateError")}
            </p>
          ) : null}
          <div className={styles.toolbar}>
            <button
              type="button"
              className={styles.control}
              onClick={props.onConfirm}
              disabled={props.source === "" || props.target === ""}
            >
              {t("workflow.detail.builder.connectAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      )}
    </Dialog>
  );
}
