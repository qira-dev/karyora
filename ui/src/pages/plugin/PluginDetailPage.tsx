import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type {
  CoreWireEnvelope,
  Plugin,
  PluginMutationResult,
  PluginState,
} from "../../coreclient/httpLocal";
import type { TranslationKey } from "../../i18n/keys";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./PluginDetailPage.module.css";
import {
  CopyIdButton,
  mintPluginCorrelationId,
  mintPluginMutationKey,
  pluginLifecycleChip,
  pluginToggleTargetable,
} from "./pluginStates";

/**
 * `/plugins/:pluginId` — the ONE canonical plugin-detail route (B5-U1
 * reads; B5-U2 lifecycle wiring; the DE-0 queue row 9 / GR-0 queue row 2,
 * the P4.1 G1–G10 rows against the now-live B5 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/plugins/{id}` (the inspect member, B5-A1 §2.1 #3): the
 *   installed plugin record with its Core-returned manifest and package
 *   verification states. THE DUTY SPLIT (binding, carried): REDACTION IS
 *   THE CORE'S DUTY — the page displays only Core-returned sanitized
 *   content and never exposes, requests, or fabricates an unredacted
 *   channel (the may-never list: no secret values — a named
 *   secret-handle's NAME only; no raw logs, runtime internals, stack
 *   traces, or memory contents; no raw plugin output or raw provider
 *   payloads as diagnostics; no diagnostic the Core did not return).
 * - `GET /v1/plugins/{id}/state` (the authoritative state endpoint,
 *   B5-A1 §2.1 #8): a pure query — never a retry; the reconciliation
 *   anchor for every lost lifecycle outcome (the S0–S4 law). Events are
 *   never the source of truth; this read is.
 * - THE UI-29 RESOLUTION (GR-C1, the owner's LIVE 2026-11-09 decision;
 *   B5-U2, GR-0 queue row 2): the four lifecycle controls are LIVE
 *   behind their §8.9 two-step confirmations — enable/disable (the M-29
 *   toggle; the carrier gap stands: the body carries exactly one
 *   member), update (the M-30 class; the target version reference is an
 *   EXPLICIT user-supplied input — no served surface lists available
 *   targets, and the note says so; the Core verifies the target), remove
 *   (the M-31 class; the remove-vs-revoke distinctness stays RECORDED
 *   OPEN — no copy asserts either), and revoke-grants (the G7 class; the
 *   never-grant law binds — a retry never re-grants). The permission
 *   review (G3, P4.10 §3.3) rides activation: the enable confirmation
 *   carries the review's laws and its honest absence (no
 *   requested-capability record is served on this Core — the manifest
 *   member is absent), never a fabricated review item; Escape/Cancel
 *   cancels with no implied grant and no implied deny.
 * - THE KEYED MINTING (the caller's duty): update/remove/revoke mint a
 *   FRESH idempotency key + correlation ID per NEW attempt (the 1 h
 *   replay window class); the toggle carries NO key members (the
 *   carrier gap) and reconciles by the state query.
 * - THE OC RECONCILIATION (P4.11 §3.3): a lost mutation response renders
 *   the explained uncertain state with its request reference and
 *   reconciles ONLY through the authoritative state read (OC-2/3/4/5) —
 *   no blind retry is ever offered; a reconcile read that itself fails
   keeps the explained state with the re-query affordance (OC-8); while
 *   a mutation is pending the other controls stay disabled (OC-7's
 *   duplicate-submit discipline, the §8.11/§8.12 class).
 * - THE VERDICT LAW (binding): every verification state
 *   (signature/trust/rotation/revocation) renders ONLY as the Core
 *   returns it — never computed UI-side; an absent verification state is
 *   never rendered as a verified one, and `Unknown` is the honest trust
 *   rendering (P4.1 P1's law). On this Core the served package carries
 *   `package_ref` alone (no verification machinery exists while the MB
 *   §10.2 gate stands — B5-A1 §2.4 gap 3), so the verification members
 *   render their honest absence.
 * - THE MANIFEST HONESTY (B5-A1 §2.4 gap 6): the manifest member is
 *   OPTIONAL on the published `Plugin` schema and is ABSENT on this Core
 *   (no manifest-examination path exists) — the Manifest tab renders that
 *   absence honestly; manifest text is UNTRUSTED DATA — rendered, never
 *   executed.
 * - THE DISTINCTNESS LAWS (binding, carried): DISABLED (the user's
 *   action) ≠ GRANTS-REVOKED (the user's revocation of capabilities) ≠
 *   TRUST-REVOKED (the publisher/trust-root action) — each renders its
 *   own chip, never merged, never milder; INVOCATION TERMINATION ≠
 *   PLUGIN DISABLE/REVOCATION. The toggle operates on the
 *   installed/enabled/disabled class only (a revoked state is never
 *   exitable through it — the Core refuses).
 * - NO invocation/execution surface exists anywhere (the D-13/D-14 gate
 *   and the Core Master Blueprint §10.2 runtime proof): no invocation
 *   control, no invocation state, and no invocation-derived failure
 *   record is rendered or invented.
 * - The unknown identity and the out-of-scope identity are the SAME
 *   typed 404 (the no-leak rule): the page states exactly that and never
 *   infers existence from a refusal.
 */

type ReadPhase = "loading" | "data" | "failure" | "unknown";

/** The lifecycle mutation actions (B5-U2). The toggle's two directions
 *  are the ONE M-29 member's two target states, rendered as explicit
 *  intents. */
type MutationAction = "enable" | "disable" | "update" | "remove" | "revoke";

/** The mutation dialog's phases (the B2 destructive-dialog idiom): the
 *  confirming step (the §8.9 first step — the scope statement), the
 *  in-flight submission, the Core-confirmed outcome, the
 *  reconciliation-resolved outcome (the authoritative state read landed),
 *  the explained uncertain outcome (OC-2/3/4/5), and the typed refusal. */
type MutationPhase = "confirming" | "submitting" | "confirmed" | "reconciled" | "unknown" | "failure";

type TabId = "overview" | "manifest" | "capabilities" | "compatibility" | "digest" | "logs" | "audit";

/** The recorded G1 tab set (§19.2's order): Overview, Manifest,
 *  Capabilities, Compatibility, Versions/Digest, Safe logs, Audit
 *  events. Every tab renders its honest served state — a section the
 *  Core does not serve renders its absence (G1's missing-section law),
 *  never invented content. */
const TABS: readonly { readonly id: TabId; readonly labelKey: TranslationKey }[] = [
  { id: "overview", labelKey: "plugin.detail.tabsOverview" },
  { id: "manifest", labelKey: "plugin.detail.tabsManifest" },
  { id: "capabilities", labelKey: "plugin.detail.tabsCapabilities" },
  { id: "compatibility", labelKey: "plugin.detail.tabsCompatibility" },
  { id: "digest", labelKey: "plugin.detail.tabsDigest" },
  { id: "logs", labelKey: "plugin.detail.tabsLogs" },
  { id: "audit", labelKey: "plugin.detail.tabsAudit" },
];

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

export function PluginDetailPage() {
  const { pluginId = "" } = useParams<{ pluginId: string }>();
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const unavailable = t("states.unavailable");

  const [plugin, setPlugin] = useState<Plugin | null>(null);
  const [state, setState] = useState<PluginState | null>(null);
  const [inspectPhase, setInspectPhase] = useState<ReadPhase>("loading");
  const [statePhase, setStatePhase] = useState<ReadPhase>("loading");
  const [inspectFailure, setInspectFailure] = useState<CoreWireEnvelope | null>(null);
  const [stateFailure, setStateFailure] = useState<CoreWireEnvelope | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  const [notFound, setNotFound] = useState(false);
  // The lifecycle mutation machinery (B5-U2): one pending mutation at a
  // time (OC-7's duplicate-submit discipline); the §8.9 dialog carries
  // each action's confirming/submitting/outcome phases; the uncertain
  // outcome carries the minted request reference (the keyed members) and
  // reconciles ONLY through the authoritative state read.
  const [dialogAction, setDialogAction] = useState<MutationAction | null>(null);
  const [dialogPhase, setDialogPhase] = useState<MutationPhase>("confirming");
  const [pendingAction, setPendingAction] = useState(false);
  const [mutationResult, setMutationResult] = useState<PluginMutationResult | null>(null);
  const [mutationFailure, setMutationFailure] = useState<CoreWireEnvelope | null>(null);
  const [unknownCorrelation, setUnknownCorrelation] = useState<string | null>(null);
  const [reconcileFailed, setReconcileFailed] = useState(false);
  const [reconciledState, setReconciledState] = useState<string | null>(null);
  const [updateTargetRef, setUpdateTargetRef] = useState("");
  const [updateTargetError, setUpdateTargetError] = useState(false);
  const navigate = useNavigate();

  const tabListRef = useRef<HTMLDivElement>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const readAll = useCallback(() => {
    setInspectPhase("loading");
    setInspectFailure(null);
    setStatePhase("loading");
    setStateFailure(null);
    setNotFound(false);
    setLastConfirmedAt(null);
    void transport.pluginGet(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setPlugin(outcome.data);
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
    void transport.pluginState(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data);
        setStatePhase("data");
      } else if (outcome.kind === "rejection") {
        setStateFailure(outcome.envelope);
        setStatePhase("failure");
      } else {
        setStatePhase("unknown");
      }
    });
  }, [transport, pluginId]);

  useEffect(() => {
    readAll();
  }, [readAll]);

  // The silent re-read behind a dialog (the B2 idiom's post-mutation
  // authoritative re-read): the current render stays up while the fresh
  // record/state land; no loading flash, no optimistic state — the
  // Core's confirmed truth replaces the rendered record when it arrives.
  const silentRefresh = useCallback(() => {
    void transport.pluginGet(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setPlugin(outcome.data);
        setInspectPhase("data");
        setInspectFailure(null);
        setNotFound(false);
        setLastConfirmedAt(new Date());
      }
    });
    void transport.pluginState(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data);
        setStatePhase("data");
        setStateFailure(null);
      }
    });
  }, [pluginId, transport]);

  const openMutationDialog = (action: MutationAction) => {
    setDialogAction(action);
    setDialogPhase("confirming");
    setMutationResult(null);
    setMutationFailure(null);
    setUnknownCorrelation(null);
    setReconcileFailed(false);
    setReconciledState(null);
    setUpdateTargetError(false);
  };

  const closeMutationDialog = useCallback(() => {
    const wasRemoveConfirmed = dialogAction === "remove" && dialogPhase === "confirmed";
    setDialogAction(null);
    setDialogPhase("confirming");
    setMutationResult(null);
    setMutationFailure(null);
    setUnknownCorrelation(null);
    setReconcileFailed(false);
    setReconciledState(null);
    if (wasRemoveConfirmed) {
      // The Core-confirmed removal took the artifact out of the
      // installed set: the honest landing is the list, not a dead detail
      // page (the removed panel carried the confirmed outcome).
      navigate("/plugins");
    }
  }, [dialogAction, dialogPhase, navigate]);

  // The §8.9 second step: the explicit confirm action submits. The keyed
  // members (update/remove/revoke) mint a FRESH key + correlation here (a
  // new attempt = a new identity; the 1 h replay window class); the
  // toggle carries NO key members (the carrier gap). A lost response is
  // `unknown`: the minted correlation is retained as the request
  // reference and the reconcile goes through the authoritative state
  // read — NO blind retry is ever offered (the S0–S4 law).
  const confirmMutation = useCallback(
    (action: MutationAction) => {
      if (pendingAction) return;
      setPendingAction(true);
      setDialogPhase("submitting");
      const finishPending = () => setPendingAction(false);

      if (action === "enable" || action === "disable") {
        // The M-29 state-toggle member: exactly ONE body member (the
        // carrier gap — no key members are minted or sent). The revoked
        // states refuse through the Core (the distinctness law).
        void transport.pluginSetEnabled(pluginId, { enabled: action === "enable" }).then((outcome) => {
          finishPending();
          if (!mountedRef.current) return;
          if (outcome.kind === "data") {
            setMutationResult(outcome.data);
            setDialogPhase("confirmed");
            silentRefresh();
          } else if (outcome.kind === "rejection") {
            setMutationFailure(outcome.envelope);
            setDialogPhase("failure");
          } else {
            setUnknownCorrelation(null);
            setDialogPhase("unknown");
          }
        });
        return;
      }

      const correlationId = mintPluginCorrelationId(`plugin-${action}`);
      if (action === "update") {
        const target = updateTargetRef.trim();
        if (target === "") {
          setPendingAction(false);
          setDialogPhase("confirming");
          setUpdateTargetError(true);
          return;
        }
        const idempotencyKey = mintPluginMutationKey("plugin-update");
        void transport
          .pluginUpdate(pluginId, {
            idempotency_key: idempotencyKey,
            correlation_id: correlationId,
            target_version_ref: target,
          })
          .then((outcome) => {
            finishPending();
            if (!mountedRef.current) return;
            if (outcome.kind === "data") {
              setMutationResult(outcome.data);
              setDialogPhase("confirmed");
              silentRefresh();
            } else if (outcome.kind === "rejection") {
              setMutationFailure(outcome.envelope);
              setDialogPhase("failure");
            } else {
              setUnknownCorrelation(correlationId);
              setDialogPhase("unknown");
            }
          });
        return;
      }

      const idempotencyKey = mintPluginMutationKey(`plugin-${action}`);
      const call =
        action === "remove"
          ? transport.pluginRemove(pluginId, { idempotency_key: idempotencyKey, correlation_id: correlationId })
          : transport.pluginRevokeGrants(pluginId, { idempotency_key: idempotencyKey, correlation_id: correlationId });
      void call.then((outcome) => {
        finishPending();
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          setMutationResult(outcome.data);
          setDialogPhase("confirmed");
          if (action !== "remove") silentRefresh();
        } else if (outcome.kind === "rejection") {
          setMutationFailure(outcome.envelope);
          setDialogPhase("failure");
        } else {
          setUnknownCorrelation(correlationId);
          setDialogPhase("unknown");
        }
      });
    },
    [pendingAction, pluginId, silentRefresh, transport, updateTargetRef],
  );

  // The OC-2/3/4/5 reconciliation: the authoritative state read (a pure
  // query — never a retry) resolves the uncertain outcome by the
  // plugin/package identity. A usable answer applies the Core's truth
  // (the state chip renders; the page re-reads behind the dialog); a
  // failed read keeps the explained state with the re-query affordance
  // (OC-8) — never a blind resubmit of the original mutation.
  const reconcileMutation = useCallback(() => {
    setReconcileFailed(false);
    void transport.pluginState(pluginId).then((outcome) => {
      if (!mountedRef.current) return;
      if (outcome.kind === "data") {
        setState(outcome.data);
        setStatePhase("data");
        setStateFailure(null);
        setReconciledState(outcome.data.lifecycle_state);
        setDialogPhase("reconciled");
        silentRefresh();
      } else {
        setReconcileFailed(true);
      }
    });
  }, [pluginId, silentRefresh, transport]);

  // The tablist keyboard pattern (§8.8): Arrow keys move among the tabs;
  // every tab renders its honest served state (G1's missing-section law),
  // so none is excluded from the roving focus.
  const onTabListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
    event.preventDefault();
    const index = TABS.findIndex((tab) => tab.id === activeTab);
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const next = TABS[(index + delta + TABS.length) % TABS.length];
    setActiveTab(next.id);
    const nextButton = tabListRef.current?.querySelector<HTMLButtonElement>(`[data-tab-id="${next.id}"]`);
    nextButton?.focus();
  };

  if (inspectPhase === "loading") {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        <p className={styles.note}>{t("plugin.list.loading")}</p>
      </PageLayout>
    );
  }

  if (inspectPhase === "failure" && inspectFailure !== null) {
    return (
      <PageLayout>
        <p className={styles.backLink}>
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        {notFound ? (
          <RecoveryPanel
            title={t("plugin.detail.notFoundTitle")}
            body={t("plugin.detail.notFoundBody")}
            note={t("plugin.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[
              { label: t("plugin.detail.back"), onAction: () => window.history.back() },
            ]}
          />
        ) : (
          <RecoveryPanel
            title={t("plugin.detail.failureTitle")}
            body={t("plugin.detail.failureBody")}
            note={t("plugin.detail.connectionHint")}
            referenceId={inspectFailure.correlationId}
            actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
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
          <Link to="/plugins">{t("plugin.detail.back")}</Link>
        </p>
        <PageHeading>{t("plugin.list.heading")}</PageHeading>
        <RecoveryPanel
          title={t("plugin.detail.unknownTitle")}
          body={t("plugin.detail.unknownBody")}
          note={t("plugin.detail.connectionHint")}
          actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
        />
      </PageLayout>
    );
  }

  // inspectPhase === "data" here; plugin is non-null by construction.
  const shownPlugin = plugin as Plugin;
  // The served record carries no display-name member; the Core-minted
  // identity is the honest page title (never renamed, never invented).
  const shownName = shownPlugin.plugin_id;

  const lifecycleChip = pluginLifecycleChip(shownPlugin.lifecycle_state);
  const LifecycleIcon = lifecycleChip.icon;

  // The toggle's intent resolves from the Core-reported state (the ONE
  // M-29 member's two target states, rendered as explicit intents); the
  // revoked states are not toggle-targetable (the distinctness law).
  const toggleTargetable = pluginToggleTargetable(shownPlugin.lifecycle_state);
  const toggleIntent: MutationAction = shownPlugin.lifecycle_state === "enabled" ? "disable" : "enable";

  const stateChip = state === null ? null : pluginLifecycleChip(state.lifecycle_state);
  const StateIcon = stateChip === null ? null : stateChip.icon;

  return (
    <PageLayout>
      <p className={styles.backLink}>
        <Link to="/plugins">{t("plugin.detail.back")}</Link>
      </p>
      <PageHeading>{shownName}</PageHeading>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("plugin.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
        </p>
      ) : null}
      <div className={styles.toolbar}>
        <button type="button" className={styles.control} onClick={readAll}>
          {t("plugin.detail.refresh")}
        </button>
      </div>
      {/* G10 — the detail surface's gate state (the accurate recorded
          reason): the catalog/install/activation/removal controls stay
          withheld while UI-29 is open; the surface renders Core-owned
          reads only. */}
      <p className={styles.gateNote}>{t("plugin.detail.gateNote")}</p>
      {/* The execution gate's honest state: NO invocation/execution
          surface exists (D-13/D-14 + the Core MB §10.2 runtime proof);
          nothing is invented for it. */}
      <p className={styles.gateNote}>{t("plugin.detail.executionGateNote")}</p>

      <section className={styles.factsPanel} aria-label={t("plugin.detail.factsHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.factsHeading")}</h2>
        <dl className={styles.facts}>
          <div>
            <dt>{t("plugin.detail.factsPluginId")}</dt>
            <dd>
              <CopyIdButton
                id={shownPlugin.plugin_id}
                label={t("plugin.detail.copyPluginId")}
                copiedLabel={t("plugin.list.copied")}
                failedLabel={t("plugin.list.copyFailed")}
                className={styles.copyId}
              />
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsLifecycleState")}</dt>
            <dd>
              <span className={`${styles.chip} ${styles[lifecycleChip.tone]}`}>
                <LifecycleIcon aria-hidden="true" size={14} />
                {t(lifecycleChip.labelKey)}
              </span>
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsInstallRecordRef")}</dt>
            <dd>
              {shownPlugin.install_record_ref === undefined || shownPlugin.install_record_ref === "" ? (
                <span className={styles.muted}>{unavailable}</span>
              ) : (
                <code>{shownPlugin.install_record_ref}</code>
              )}
            </dd>
          </div>
          <div>
            <dt>{t("plugin.detail.factsInstalledAt")}</dt>
            <dd>
              {shownPlugin.installed_at === undefined ? (
                unavailable
              ) : (
                <time dateTime={shownPlugin.installed_at}>
                  {formatTimestamp(new Date(shownPlugin.installed_at), locale)}
                </time>
              )}
            </dd>
          </div>
        </dl>
      </section>

      {/* The authoritative state endpoint's read (a pure query — never a
          retry; the reconciliation anchor for every lost lifecycle
          outcome; events are never the source of truth). */}
      <section className={styles.factsPanel} aria-label={t("plugin.detail.stateHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.stateHeading")}</h2>
        <p className={styles.note}>{t("plugin.detail.stateNote")}</p>
        {statePhase === "loading" ? <p className={styles.note}>{t("plugin.list.loading")}</p> : null}
        {statePhase === "failure" && stateFailure !== null ? (
          <>
            <RecoveryPanel
              title={t("plugin.detail.failureTitle")}
              body={t("plugin.detail.failureBody")}
              note={t("plugin.detail.connectionHint")}
              referenceId={stateFailure.correlationId}
              actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
            />
            <p className={styles.code}>
              <span className="visually-hidden">Code: </span>
              <code>{stateFailure.code}</code>
            </p>
          </>
        ) : null}
        {statePhase === "unknown" ? (
          <RecoveryPanel
            title={t("plugin.detail.unknownTitle")}
            body={t("plugin.detail.unknownBody")}
            note={t("plugin.detail.connectionHint")}
            actions={[{ label: t("plugin.detail.refresh"), onAction: readAll }]}
          />
        ) : null}
        {statePhase === "data" && state !== null && stateChip !== null && StateIcon !== null ? (
          <dl className={styles.facts}>
            <div>
              <dt>{t("plugin.detail.stateLifecycleState")}</dt>
              <dd>
                <span className={`${styles.chip} ${styles[stateChip.tone]}`}>
                  <StateIcon aria-hidden="true" size={14} />
                  {t(stateChip.labelKey)}
                </span>
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.statePackageRef")}</dt>
              <dd>
                {state.package_ref === undefined || state.package_ref === "" ? (
                  <span className={styles.muted}>{unavailable}</span>
                ) : (
                  <code>{state.package_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.stateInstallRecordRef")}</dt>
              <dd>
                {state.install_record_ref === undefined || state.install_record_ref === "" ? (
                  <span className={styles.muted}>{unavailable}</span>
                ) : (
                  <code>{state.install_record_ref}</code>
                )}
              </dd>
            </div>
            <div>
              <dt>{t("plugin.detail.stateUpdatedAt")}</dt>
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

      {/* The lifecycle controls (B5-U2; LIVE per the UI-29 resolution,
          each behind its §8.9 two-step confirmation — never dead stubs,
          never enabled look-alikes): the toggle operates on the
          installed/enabled/disabled class only (the Core refuses the
          revoked states); the update's target reference is an explicit
          user-supplied input with its honest no-served-source note; the
          remove-vs-revoke distinctness is RECORDED OPEN — the copy
          asserts neither. While a mutation is pending, every control
          stays disabled (OC-7's duplicate-submit discipline). */}
      <section className={styles.factsPanel} aria-label={t("plugin.detail.actionsHeading")}>
        <h2 className={styles.subHeading}>{t("plugin.detail.actionsHeading")}</h2>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.control}
            disabled={!toggleTargetable || pendingAction}
            aria-describedby={
              toggleTargetable ? "plugin-toggle-shape-note" : "plugin-toggle-targetability-note"
            }
            onClick={() => openMutationDialog(toggleIntent)}
          >
            {toggleTargetable
              ? toggleIntent === "enable"
                ? t("plugin.detail.enableConfirmAction")
                : t("plugin.detail.disableConfirmAction")
              : t("plugin.detail.toggleControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled={pendingAction}
            aria-describedby="plugin-update-target-note"
            onClick={() => openMutationDialog("update")}
          >
            {t("plugin.detail.updateControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled={pendingAction}
            onClick={() => openMutationDialog("remove")}
          >
            {t("plugin.detail.removeControl")}
          </button>
          <button
            type="button"
            className={styles.control}
            disabled={pendingAction}
            onClick={() => openMutationDialog("revoke")}
          >
            {t("plugin.detail.revokeControl")}
          </button>
        </div>
        {pendingAction ? (
          <p className={styles.actionReason} role="status">
            {t("plugin.detail.actionsPendingNote")}
          </p>
        ) : null}
        {toggleTargetable ? (
          <p id="plugin-toggle-shape-note" className={styles.actionReason}>
            {t("plugin.detail.toggleShapeNote")}
          </p>
        ) : (
          <p id="plugin-toggle-targetability-note" className={styles.actionReason}>
            {t("plugin.detail.toggleTargetableNote")}
          </p>
        )}
        <p id="plugin-update-target-note" className={styles.actionReason}>
          {t("plugin.detail.updateTargetNote")}
        </p>
        <p className={styles.actionReason}>{t("plugin.detail.removeConfirmOpenNote")}</p>
      </section>

      <div
        className={styles.tabList}
        role="tablist"
        aria-label={shownName}
        ref={tabListRef}
        onKeyDown={onTabListKeyDown}
      >
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            data-tab-id={tab.id}
            id={`plugin-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`plugin-panel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={activeTab === tab.id ? styles.tabActive : styles.tab}
            onClick={() => setActiveTab(tab.id)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>

      {activeTab === "overview" ? (
        <section
          role="tabpanel"
          id="plugin-panel-overview"
          aria-labelledby="plugin-tab-overview"
          className={styles.panel}
        >
          <p className={styles.note}>{t("plugin.detail.overviewDistinctNote")}</p>
          <p className={styles.note}>{t("plugin.detail.overviewTrustNote")}</p>
          <p className={styles.note}>{t("plugin.detail.overviewFailureNote")}</p>
        </section>
      ) : null}

      {activeTab === "manifest" ? (
        <section
          role="tabpanel"
          id="plugin-panel-manifest"
          aria-labelledby="plugin-tab-manifest"
          className={styles.panel}
        >
          {/* The manifest member is ABSENT on this Core (B5-A1 §2.4 gap
              6): the honest absence renders — never a fabricated manifest. */}
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.manifestAbsentTitle")}</p>
            <p>{t("plugin.detail.manifestAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.manifestUntrustedNote")}</p>
          <p className={styles.note}>{t("plugin.detail.manifestResourceNote")}</p>
        </section>
      ) : null}

      {activeTab === "capabilities" ? (
        <section
          role="tabpanel"
          id="plugin-panel-capabilities"
          aria-labelledby="plugin-tab-capabilities"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.capabilitiesAbsentTitle")}</p>
            <p>{t("plugin.detail.capabilitiesAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.capabilitiesNeverGrantNote")}</p>
          <p className={styles.note}>{t("plugin.detail.capabilitiesReviewNote")}</p>
        </section>
      ) : null}

      {activeTab === "compatibility" ? (
        <section
          role="tabpanel"
          id="plugin-panel-compatibility"
          aria-labelledby="plugin-tab-compatibility"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.compatibilityAbsentTitle")}</p>
            <p>{t("plugin.detail.compatibilityAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.compatibilityAbiNote")}</p>
        </section>
      ) : null}

      {activeTab === "digest" ? (
        <section
          role="tabpanel"
          id="plugin-panel-digest"
          aria-labelledby="plugin-tab-digest"
          className={styles.panel}
        >
          {/* The package record renders as the Core returns it (the
              verdict law); each verification member renders only where the
              Core returns it — on this Core the served package carries
              `package_ref` alone (B5-A1 §2.4 gap 3), so the verification
              members render their honest absence. */}
          {shownPlugin.package === undefined ? (
            <div className={styles.emptyState}>
              <p className={styles.emptyTitle}>{t("plugin.detail.digestNoPackageTitle")}</p>
              <p>{t("plugin.detail.digestNoPackageBody")}</p>
            </div>
          ) : (
            <dl className={styles.facts}>
              <div>
                <dt>{t("plugin.detail.digestPackageRef")}</dt>
                <dd>
                  <code>{shownPlugin.package.package_ref}</code>
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestArtifactDigest")}</dt>
                <dd>
                  {shownPlugin.package.artifact_digest === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.artifact_digest}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestSignatureResult")}</dt>
                <dd>
                  {shownPlugin.package.signature_verification_result === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.signature_verification_result}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestTrustRootRef")}</dt>
                <dd>
                  {shownPlugin.package.trust_root_ref === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.trust_root_ref}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestRotationStatus")}</dt>
                <dd>
                  {shownPlugin.package.rotation_status === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.rotation_status}</code>
                  )}
                </dd>
              </div>
              <div>
                <dt>{t("plugin.detail.digestRevocationStatus")}</dt>
                <dd>
                  {shownPlugin.package.revocation_status === undefined ? (
                    <span className={styles.muted}>{unavailable}</span>
                  ) : (
                    <code>{shownPlugin.package.revocation_status}</code>
                  )}
                </dd>
              </div>
            </dl>
          )}
          <p className={styles.note}>{t("plugin.detail.digestVerificationNote")}</p>
          <p className={styles.note}>{t("plugin.detail.digestTrustWordNote")}</p>
        </section>
      ) : null}

      {activeTab === "logs" ? (
        <section
          role="tabpanel"
          id="plugin-panel-logs"
          aria-labelledby="plugin-tab-logs"
          className={styles.panel}
        >
          {/* G9's honest absence: no published plugin-log read exists in
              the served family; missing/unavailable logs are labeled
              honestly; the redaction duty-split rides the note. */}
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.logsAbsentTitle")}</p>
            <p>{t("plugin.detail.logsAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.logsRedactionNote")}</p>
        </section>
      ) : null}

      {activeTab === "audit" ? (
        <section
          role="tabpanel"
          id="plugin-panel-audit"
          aria-labelledby="plugin-tab-audit"
          className={styles.panel}
        >
          <div className={styles.emptyState}>
            <p className={styles.emptyTitle}>{t("plugin.detail.auditAbsentTitle")}</p>
            <p>{t("plugin.detail.auditAbsentBody")}</p>
          </div>
          <p className={styles.note}>{t("plugin.detail.auditSourceNote")}</p>
        </section>
      ) : null}

      <MutationDialog
        action={dialogAction}
        phase={dialogPhase}
        updateTargetRef={updateTargetRef}
        updateTargetError={updateTargetError}
        onUpdateTargetRefChange={(value) => {
          setUpdateTargetRef(value);
          setUpdateTargetError(false);
        }}
        result={mutationResult}
        failure={mutationFailure}
        unknownCorrelation={unknownCorrelation}
        reconcileFailed={reconcileFailed}
        reconciledState={reconciledState}
        onClose={closeMutationDialog}
        onConfirm={confirmMutation}
        onReconcile={reconcileMutation}
      />
    </PageLayout>
  );
}

/** The lifecycle mutation confirmation dialog (the §8.9 two-step
 *  confirmation for EACH lifecycle mutation; the B2 destructive-dialog
 *  idiom): step one states the action's scope and laws exactly; step two
 *  is the explicit confirm action. Escape and Close cancel with no
 *  implied grant, no implied deny, and no implied mutation (the
 *  review-flow escape law's class). The enable confirmation carries the
 *  permission-review flow (G3, P4.10 §3.3): its laws render, and the
 *  honest absence of served per-permission items is stated — never a
 *  fabricated review item. The uncertain outcome renders the explained
 *  state with the request reference and reconciles ONLY through the
 *  authoritative state read (OC-2/3/4/5; OC-8's re-query affordance) —
 *  never a blind retry, never an optimistic confirmation. */
function MutationDialog(props: {
  readonly action: MutationAction | null;
  readonly phase: MutationPhase;
  readonly updateTargetRef: string;
  readonly updateTargetError: boolean;
  readonly onUpdateTargetRefChange: (value: string) => void;
  readonly result: PluginMutationResult | null;
  readonly failure: CoreWireEnvelope | null;
  readonly unknownCorrelation: string | null;
  readonly reconcileFailed: boolean;
  readonly reconciledState: string | null;
  readonly onClose: () => void;
  readonly onConfirm: (action: MutationAction) => void;
  readonly onReconcile: () => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();

  // A local const of the action: the narrowing into the JSX branches and
  // their click callbacks holds on a const where a property access would
  // not.
  const action = props.action;

  const titleKey: TranslationKey | null =
    props.action === null
      ? null
      : props.action === "enable"
        ? "plugin.detail.enableConfirmTitle"
        : props.action === "disable"
          ? "plugin.detail.disableConfirmTitle"
          : props.action === "update"
            ? "plugin.detail.updateConfirmTitle"
            : props.action === "remove"
              ? "plugin.detail.removeConfirmTitle"
              : "plugin.detail.revokeConfirmTitle";

  const chip = props.reconciledState === null ? null : pluginLifecycleChip(props.reconciledState);
  const ChipIcon = chip?.icon;

  return (
    <Dialog
      labelId={labelId}
      title={titleKey === null ? "" : t(titleKey)}
      open={props.action !== null}
      onClose={props.onClose}
    >
      {action !== null && props.phase === "confirming" ? (
        <div>
          {action === "enable" ? (
            <section className={styles.confirmBlock}>
              <p>{t("plugin.detail.enableConfirmScope")}</p>
              <p className={styles.outcomeTitle}>{t("plugin.detail.enableConfirmReviewHeading")}</p>
              <p>{t("plugin.detail.enableConfirmReviewLaws")}</p>
              <p>{t("plugin.detail.enableConfirmReviewAbsent")}</p>
            </section>
          ) : null}
          {action === "disable" ? (
            <section className={styles.confirmBlock}>
              <p>{t("plugin.detail.disableConfirmScope")}</p>
            </section>
          ) : null}
          {action === "update" ? (
            <section className={styles.confirmBlock}>
              <label className={styles.fieldLabel} htmlFor="plugin-update-target-ref">
                {t("plugin.detail.updateTargetLabel")}
              </label>
              <input
                id="plugin-update-target-ref"
                className={styles.fieldInput}
                type="text"
                value={props.updateTargetRef}
                onChange={(event) => props.onUpdateTargetRefChange(event.target.value)}
                autoComplete="off"
                spellCheck={false}
                aria-invalid={props.updateTargetError}
                aria-describedby={props.updateTargetError ? "plugin-update-target-error" : "plugin-update-target-note"}
              />
              <p id="plugin-update-target-note" className={styles.fieldNote}>
                {t("plugin.detail.updateTargetNote")}
              </p>
              {props.updateTargetError ? (
                <p id="plugin-update-target-error" className={styles.inputError} role="alert">
                  {t("plugin.detail.emptyInput")}
                </p>
              ) : null}
              <p>{t("plugin.detail.updateConfirmScope")}</p>
              <p>{t("plugin.detail.updateConfirmFreshReview")}</p>
            </section>
          ) : null}
          {action === "remove" ? (
            <section className={styles.confirmBlock}>
              <p>{t("plugin.detail.removeConfirmScope")}</p>
              <p>{t("plugin.detail.removeConfirmOpenNote")}</p>
            </section>
          ) : null}
          {action === "revoke" ? (
            <section className={styles.confirmBlock}>
              <p>{t("plugin.detail.revokeConfirmScope")}</p>
              <p>{t("plugin.detail.revokeConfirmLaw")}</p>
            </section>
          ) : null}
          <div className={styles.actions}>
            <button type="button" className={styles.control} onClick={() => props.onConfirm(action)}>
              {props.action === "enable"
                ? t("plugin.detail.enableConfirmAction")
                : props.action === "disable"
                  ? t("plugin.detail.disableConfirmAction")
                  : props.action === "update"
                    ? t("plugin.detail.updateConfirmAction")
                    : props.action === "remove"
                      ? t("plugin.detail.removeConfirmAction")
                      : t("plugin.detail.revokeConfirmAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      ) : null}

      {action !== null && props.phase === "submitting" ? (
        <p className={styles.fieldNote}>{t("plugin.detail.submitting")}</p>
      ) : null}

      {action !== null && props.phase === "confirmed" && props.result !== null ? (
        <div>
          {action === "remove" ? (
            <section className={styles.outcomeBlock}>
              <p className={styles.outcomeTitle}>{t("plugin.detail.removedTitle")}</p>
              <p>{t("plugin.detail.removedBody")}</p>
            </section>
          ) : (
            <section className={styles.outcomeBlock}>
              <p className={styles.outcomeTitle}>{t("plugin.detail.confirmedTitle")}</p>
              {props.result.replayed ? <p>{t("plugin.detail.replayedNote")}</p> : null}
            </section>
          )}
          <dl className={styles.facts}>
            <div>
              <dt>{t("plugin.detail.confirmedRefLabel")}</dt>
              <dd>
                <code>{props.result.operation_ref}</code>
              </dd>
            </div>
          </dl>
          <div className={styles.actions}>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {action !== null && props.phase === "reconciled" && chip !== null && ChipIcon !== undefined ? (
        <div>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("plugin.detail.stateHeading")}</p>
            <dl className={styles.facts}>
              <div>
                <dt>{t("plugin.detail.stateLifecycleState")}</dt>
                <dd>
                  <span className={`${styles.chip} ${styles[chip.tone]}`}>
                    <ChipIcon aria-hidden="true" size={14} />
                    {t(chip.labelKey)}
                  </span>
                </dd>
              </div>
            </dl>
          </section>
          <div className={styles.actions}>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {action !== null && props.phase === "unknown" ? (
        <div>
          <RecoveryPanel
            title={t("plugin.detail.unknownTitle")}
            body={t("plugin.detail.unknownBody")}
            referenceId={props.unknownCorrelation ?? ""}
          />
          <dl className={styles.facts}>
            <div>
              <dt>{t("plugin.detail.unknownRefLabel")}</dt>
              <dd>
                {props.unknownCorrelation === null ? (
                  <span className={styles.muted}>{t("plugin.detail.unknownNoKeyNote")}</span>
                ) : (
                  <code>{props.unknownCorrelation}</code>
                )}
              </dd>
            </div>
          </dl>
          {props.reconcileFailed ? <p className={styles.fieldNote}>{t("plugin.detail.reconcileFailed")}</p> : null}
          <div className={styles.actions}>
            <button type="button" className={styles.control} onClick={props.onReconcile}>
              {t("plugin.detail.reconcile")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {action !== null && props.phase === "failure" && props.failure !== null ? (
        <div>
          <RecoveryPanel
            title={t("plugin.detail.failureTitle")}
            body={t("plugin.detail.actionFailureBody")}
            note={t("plugin.detail.connectionHint")}
            referenceId={props.failure.correlationId}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{props.failure.code}</code>
          </p>
          <div className={styles.actions}>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
