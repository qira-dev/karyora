import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, Plugin, PluginInstallResult } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./PluginLibraryPage.module.css";
import {
  CopyIdButton,
  fieldOrUnavailable,
  mintPluginCorrelationId,
  mintPluginInstallKey,
  pluginLifecycleChip,
} from "./pluginStates";

/**
 * `/plugins` — the Plugin list (B5-U1 reads; B5-U2 install; the DE-0
 * queue row 9 / GR-0 queue row 2, the P4.1 P1–P6 rows against the
 * now-live B5 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/plugins?limit=&cursor=` (the published list member's
 *   serving, B5-A1 §2.1 #1). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented).
 * - THE UI-29 RESOLUTION (GR-C1, the owner's LIVE 2026-11-09 decision;
 *   B5-U2, GR-0 queue row 2): the P2 install surface is LIVE through the
 *   C1-07 install class exactly — the Core mints the plugin/package/
 *   install-record/grant identities (the body never carries one); the
 *   client mints the idempotency key + correlation ID per attempt (the
 *   72 h replay window); the three replay outcomes ride the Core's
 *   keyed ruling (same key + same fingerprint → the SAME install-record
 *   reference, replayed; same key + different fingerprint → 409
 *   CONFLICT_IDEMPOTENCY_KEY; past the window → 409 CONFLICT_STATE); a
 *   confirmed install lands the plugin in its installed, NOT-yet-enabled
 *   state — activation is separate, on the detail page through the
 *   permission review. The §8.9 confirmation governs (two-step: the
 *   dialog's scope statement, then the explicit confirm action). OC-1:
 *   a lost install response renders the explained uncertain state with
 *   the request reference and reconciles through the authoritative list
 *   read — NO blind retry is ever offered.
 * - THE NO-CATALOG LAW (carried, binding): USER-INSTALLED ≠
 *   CATALOG-LISTED, always — this list IS the user-installed list. The
 *   install path is EXPLICIT: the package reference is one the user
 *   supplies (the contract's own single body member), never a catalog
 *   listing, never a recommendation; the admission verdict is the
 *   Core's alone (the verdict law). The local-package import (P3) has
 *   NO served registry/upload shape in the published family and P4.10
 *   C-1's validation+quarantine precondition is unmet — no import
 *   control exists (the absence is the honest state, never a placeholder
 *   form). No catalog refresh (P4) exists — no catalog exists.
 * - The columns render Core-reported values only (the P4.5 §3.3 map law's
 *   class): the row's primary text is the Core-minted plugin identity
 *   (the served record carries no display-name member — the identity is
 *   the honest primary, never renamed); the version and requested
 *   capabilities render from the manifest where the Core serves one (the
 *   manifest member is ABSENT on this Core — B5-A1 §2.4 gap 6 — so they
 *   render Unavailable honestly); the trust cell renders the
 *   Core-returned verification states verbatim — where the Core returns
 *   no verification state it renders `Unknown`, NEVER `Verified` (P4.1
 *   P1's binding law; the trust-word discipline), and where no package
 *   record exists it renders Unavailable.
 * - The status cell renders the Core's own served lifecycle-state values
 *   through the distinctness chips (DISABLED ≠ GRANTS-REVOKED ≠
 *   TRUST-REVOKED — each its own chip, never merged, never milder).
 * - P5 row select navigates the ONE canonical detail route (X-2); row
 *   identity is the Core-minted plugin ID. A plugin's content is
 *   untrusted data — the identity rides the path; nothing here executes
 *   any of it.
 * - Every state below is driven by a real transport outcome: loading,
 *   data, the typed refusal surfaced verbatim, or the truthful unknown (a
 *   lost or uninterpretable response — never shown as an empty list). No
 *   fabricated rows, counts, totals, or optimistic states exist here.
 */

type ListPhase = "loading" | "data" | "failure" | "unknown";

/** The install flow's phases (the B2 destructive-dialog idiom): the
 *  dialog's confirming step, the in-flight submission, the Core-confirmed
 *  outcome (the honest `replayed` flag rendered where the Core reports
 *  it), the explained uncertain outcome (OC-1), and the typed refusal. */
type InstallPhase = "confirming" | "submitting" | "confirmed" | "unknown" | "failure";

function formatTimestamp(value: Date, locale: string | undefined): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(value);
}

/** The Core-returned package verification states, rendered verbatim as
 *  the Core returns them (the verdict law): the returned members' values
 *  in the published schema's own order. Where the Core returns NONE, the
 *  trust state is not determinable by the UI — the caller renders
 *  `Unknown`, never `Verified`. */
function returnedVerificationStates(pkg: NonNullable<Plugin["package"]>): string[] {
  const returned: string[] = [];
  if (pkg.signature_verification_result !== undefined) returned.push(pkg.signature_verification_result);
  if (pkg.trust_root_ref !== undefined) returned.push(pkg.trust_root_ref);
  if (pkg.rotation_status !== undefined) returned.push(pkg.rotation_status);
  if (pkg.revocation_status !== undefined) returned.push(pkg.revocation_status);
  return returned;
}

export function PluginLibraryPage() {
  const t = useT();
  const { i18n } = useTranslation();
  const { transport } = useCoreConnection();
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const [rows, setRows] = useState<readonly Plugin[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [failure, setFailure] = useState<CoreWireEnvelope | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [lastConfirmedAt, setLastConfirmedAt] = useState<Date | null>(null);
  // The install flow (B5-U2; the C1-07 class): the explicit package
  // reference is the user's own input (no catalog exists); the §8.9
  // confirmation is two-step (the dialog's scope statement, then the
  // explicit confirm action); the OC-1 unknown outcome carries the minted
  // request reference and reconciles through the authoritative list read
  // — never a blind retry.
  const [installOpen, setInstallOpen] = useState(false);
  const [installPhase, setInstallPhase] = useState<InstallPhase>("confirming");
  const [installPackageRef, setInstallPackageRef] = useState("");
  const [installResult, setInstallResult] = useState<PluginInstallResult | null>(null);
  const [installFailure, setInstallFailure] = useState<CoreWireEnvelope | null>(null);
  const [installUnknown, setInstallUnknown] = useState<{
    readonly packageRef: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null>(null);
  const [installReconcileFailed, setInstallReconcileFailed] = useState(false);
  const [installInputError, setInstallInputError] = useState(false);
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

      void transport.pluginList(params).then((outcome) => {
        if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
        setRefreshing(false);
        setLoadingMore(false);
        if (outcome.kind === "data") {
          setRows((prev) =>
            options.append ? [...prev, ...outcome.data.plugins] : outcome.data.plugins,
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

  // The install confirm (the §8.9 second step): the caller mints the
  // C1-07 key + correlation HERE (a fresh identity per NEW attempt — the
  // Core mints every record identity; the body never carries one). A lost
  // response is `unknown`: the minted pair is retained as the request
  // reference and the reconcile action goes through the authoritative
  // list read — NO blind retry is ever offered (the S0–S4 law).
  const openInstallConfirm = () => {
    if (installPackageRef.trim() === "") {
      setInstallInputError(true);
      return;
    }
    setInstallInputError(false);
    setInstallResult(null);
    setInstallFailure(null);
    setInstallUnknown(null);
    setInstallReconcileFailed(false);
    setInstallPhase("confirming");
    setInstallOpen(true);
  };

  const closeInstall = useCallback(() => {
    // Escape/Close cancels the flow — no implied install, no implied
    // refusal (the review-flow escape law's class).
    setInstallOpen(false);
    setInstallPhase("confirming");
    setInstallResult(null);
    setInstallFailure(null);
    setInstallUnknown(null);
    setInstallReconcileFailed(false);
  }, []);

  const confirmInstall = useCallback(() => {
    const packageRef = installPackageRef.trim();
    if (packageRef === "") {
      setInstallInputError(true);
      return;
    }
    const idempotencyKey = mintPluginInstallKey();
    const correlationId = mintPluginCorrelationId("plugin-install");
    setInstallPhase("submitting");
    void transport
      .pluginInstall({ idempotency_key: idempotencyKey, correlation_id: correlationId, package_ref: packageRef })
      .then((outcome) => {
        if (!mountedRef.current) return;
        if (outcome.kind === "data") {
          // The Core-confirmed install outcome: the landed state is
          // installed, NOT-yet-enabled (activation is separate). The
          // honest `replayed` flag renders where the Core reports it.
          setInstallResult(outcome.data);
          setInstallPhase("confirmed");
          // The authoritative re-read: the freshly installed plugin
          // appears in the user-installed list as the Core reports it.
          runRead({ append: false, markRefreshing: false });
        } else if (outcome.kind === "rejection") {
          setInstallFailure(outcome.envelope);
          setInstallPhase("failure");
        } else {
          // OC-1: the uncertain outcome renders with the request
          // reference; reconcile through the authoritative read — never
          // a blind resubmit.
          setInstallUnknown({ packageRef, idempotencyKey, correlationId });
          setInstallPhase("unknown");
        }
      });
  }, [installPackageRef, runRead, transport]);

  // OC-1's reconciliation: the served install-status query class here is
  // the authoritative user-installed list read (the install's plugin/
  // package identity resolves through it). A usable answer applies the
  // fresh list (the truth the user sees) and closes the uncertain state;
  // a read that did not land a usable answer (a typed refusal or a lost
  // response) keeps the explained state with the re-query affordance
  // (OC-8) — never a blind resubmit of the install.
  const reconcileInstall = useCallback(() => {
    setInstallReconcileFailed(false);
    const seq = ++readSeqRef.current;
    setRefreshing(true);
    void transport.pluginList({}).then((outcome) => {
      if (!mountedRef.current || seq !== readSeqRef.current) return; // superseded
      setRefreshing(false);
      setLoadingMore(false);
      if (outcome.kind === "data") {
        setRows(outcome.data.plugins);
        setNextCursor(outcome.data.next_cursor ?? null);
        setLastConfirmedAt(new Date());
        setListPhase("data");
        closeInstall();
      } else {
        setInstallReconcileFailed(true);
      }
    });
  }, [closeInstall, transport]);

  const announce = (() => {
    switch (listPhase) {
      case "loading":
        return t("plugin.list.loading");
      case "failure":
        return t("plugin.list.failureTitle");
      case "unknown":
        return t("plugin.list.unknownTitle");
      default:
        return rows.length === 0 ? t("plugin.list.emptyAll") : t("plugin.list.heading");
    }
  })();

  return (
    <PageLayout>
      <PageHeading>{t("plugin.list.heading")}</PageHeading>
      <p className={styles.description}>{t("plugin.list.description")}</p>
      {/* P6 — the surface gate state (the accurate recorded reason; the
          gate rows' "no dead stub" law): the reads render; the
          catalog/install/import/activation controls do not exist here,
          not disabled look-alikes. */}
      <p className={styles.gateNote}>{t("plugin.list.gateNote")}</p>
      <div className={styles.toolbar}>
        <button
          type="button"
          className={styles.control}
          onClick={refresh}
          disabled={refreshing || listPhase === "loading"}
        >
          {refreshing ? t("plugin.list.refreshing") : t("plugin.list.refresh")}
        </button>
      </div>

      {/* P2 — the install surface (B5-U2; the C1-07 class): EXPLICIT and
          not a catalog — the package reference is the user's own input
          (the contract's own single body member); the admission verdict
          is the Core's alone; the §8.9 confirmation is two-step. No
          local-package import exists (no served registry shape; P4.10
          C-1's validation+quarantine precondition unmet) and no catalog
          refresh exists (no catalog exists) — the notes below state both
          honestly, never as placeholder forms. */}
      <section className={styles.installPanel} aria-label={t("plugin.install.heading")}>
        <h2 className={styles.installHeading}>{t("plugin.install.heading")}</h2>
        <label className={styles.fieldLabel} htmlFor="plugin-install-package-ref">
          {t("plugin.install.inputLabel")}
        </label>
        <input
          id="plugin-install-package-ref"
          className={styles.fieldInput}
          type="text"
          value={installPackageRef}
          onChange={(event) => {
            setInstallPackageRef(event.target.value);
            setInstallInputError(false);
          }}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={installInputError}
          aria-describedby={installInputError ? "plugin-install-input-error" : "plugin-install-input-note"}
        />
        <p id="plugin-install-input-note" className={styles.fieldNote}>
          {t("plugin.install.inputNote")}
        </p>
        {installInputError ? (
          <p id="plugin-install-input-error" className={styles.inputError} role="alert">
            {t("plugin.install.emptyInput")}
          </p>
        ) : null}
        <div className={styles.toolbar}>
          <button type="button" className={styles.control} onClick={openInstallConfirm}>
            {t("plugin.install.openConfirm")}
          </button>
        </div>
        <div className={styles.empty}>
          <p className={styles.emptyTitle}>{t("plugin.install.importAbsentTitle")}</p>
          <p>{t("plugin.install.importAbsentBody")}</p>
        </div>
        <p className={styles.note}>{t("plugin.install.catalogAbsentNote")}</p>
      </section>
      {lastConfirmedAt !== null ? (
        <p className={styles.confirmed}>
          {t("plugin.list.lastConfirmed", { time: formatTimestamp(lastConfirmedAt, locale) })}
          {refreshing ? ` — ${t("plugin.list.refreshing")}` : ""}
        </p>
      ) : null}
      <p role="status" className="visually-hidden">
        {announce}
      </p>

      {listPhase === "loading" ? <p className={styles.note}>{t("plugin.list.loading")}</p> : null}

      {listPhase === "data" ? (
        rows.length === 0 ? (
          <div className={styles.empty}>
            <p className={styles.emptyTitle}>{t("plugin.list.emptyAll")}</p>
            <p>{t("plugin.list.emptyAllBody")}</p>
          </div>
        ) : (
          <>
            <table className={styles.table}>
              <caption className="visually-hidden">{t("plugin.list.description")}</caption>
              <thead>
                <tr>
                  <th scope="col">{t("plugin.list.columnPlugin")}</th>
                  <th scope="col">{t("plugin.list.columnPluginId")}</th>
                  <th scope="col">{t("plugin.list.columnVersion")}</th>
                  <th scope="col">{t("plugin.list.columnTrust")}</th>
                  <th scope="col">{t("plugin.list.columnCapabilities")}</th>
                  <th scope="col">{t("plugin.list.columnStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((plugin) => (
                  <PluginRow key={plugin.plugin_id} plugin={plugin} />
                ))}
              </tbody>
            </table>
            <p className={styles.tableNote}>{t("plugin.list.trustNote")}</p>
            {nextCursor !== null ? (
              <p className={styles.moreRow}>
                <button
                  type="button"
                  className={styles.control}
                  onClick={loadMore}
                  disabled={loadingMore}
                >
                  {loadingMore ? t("plugin.list.loadingMore") : t("plugin.list.loadMore")}
                </button>
              </p>
            ) : null}
          </>
        )
      ) : null}

      {listPhase === "failure" && failure !== null ? (
        <>
          <RecoveryPanel
            title={t("plugin.list.failureTitle")}
            body={t("plugin.list.failureBody")}
            note={t("plugin.list.connectionHint")}
            referenceId={failure.correlationId}
            actions={[{ label: t("plugin.list.refresh"), onAction: refresh }]}
          />
          <p className={styles.code}>
            <span className="visually-hidden">Code: </span>
            <code>{failure.code}</code>
          </p>
        </>
      ) : null}

      {listPhase === "unknown" ? (
        <RecoveryPanel
          title={t("plugin.list.unknownTitle")}
          body={t("plugin.list.unknownBody")}
          note={t("plugin.list.connectionHint")}
          actions={[{ label: t("plugin.list.refresh"), onAction: refresh }]}
        />
      ) : null}

      <InstallDialog
        open={installOpen}
        phase={installPhase}
        packageRef={installPackageRef.trim()}
        result={installResult}
        failure={installFailure}
        unknown={installUnknown}
        reconcileFailed={installReconcileFailed}
        onClose={closeInstall}
        onConfirm={confirmInstall}
        onReconcile={reconcileInstall}
      />
    </PageLayout>
  );
}

/** The install confirmation dialog (the §8.9 two-step confirmation; the
 *  B2 destructive-dialog idiom): step one states the scope and the laws
 *  exactly (the Core verifies and admits — the verdict law; the
 *  never-grant law; the installed-NOT-enabled landing), step two is the
 *  explicit confirm action that submits the C1-07 keyed body. Escape and
 *  Close cancel with no implied install (the review-flow escape law's
 *  class). The uncertain outcome renders the explained state with the
 *  request reference and the reconcile affordance (OC-1/OC-8) — never a
 *  blind retry, never an optimistic "Installed". */
function InstallDialog(props: {
  readonly open: boolean;
  readonly phase: InstallPhase;
  readonly packageRef: string;
  readonly result: PluginInstallResult | null;
  readonly failure: CoreWireEnvelope | null;
  readonly unknown: {
    readonly packageRef: string;
    readonly idempotencyKey: string;
    readonly correlationId: string;
  } | null;
  readonly reconcileFailed: boolean;
  readonly onClose: () => void;
  readonly onConfirm: () => void;
  readonly onReconcile: () => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();

  return (
    <Dialog labelId={labelId} title={t("plugin.install.confirmTitle")} open={props.open} onClose={props.onClose}>
      {props.phase === "confirming" ? (
        <div>
          <section className={styles.confirmBlock}>
            <p className={styles.outcomeTitle}>{t("plugin.install.confirmTitle")}</p>
            <p>{t("plugin.install.confirmScope")}</p>
            <p>{t("plugin.install.confirmLaws")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("plugin.install.inputLabel")}</dt>
                <dd>
                  <code>{props.packageRef}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onConfirm}>
              {t("plugin.install.confirmAction")}
            </button>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.cancel")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "submitting" ? <p className={styles.fieldNote}>{t("plugin.install.submitting")}</p> : null}

      {props.phase === "confirmed" && props.result !== null ? (
        <div>
          <section className={styles.outcomeBlock}>
            <p className={styles.outcomeTitle}>{t("plugin.install.confirmedTitle")}</p>
            <p>{props.result.replayed ? t("plugin.install.replayedBody") : t("plugin.install.confirmedBody")}</p>
            <dl className={styles.reviewFacts}>
              <div>
                <dt>{t("plugin.install.confirmedRefLabel")}</dt>
                <dd>
                  <code>{props.result.install_record_ref}</code>
                </dd>
              </div>
            </dl>
          </section>
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onClose}>
              {t("actions.close")}
            </button>
          </div>
        </div>
      ) : null}

      {props.phase === "unknown" ? (
        <div>
          <RecoveryPanel
            title={t("plugin.install.unknownTitle")}
            body={t("plugin.install.unknownBody")}
            referenceId={props.unknown?.correlationId ?? ""}
          />
          <dl className={styles.reviewFacts}>
            <div>
              <dt>{t("plugin.install.unknownRefLabel")}</dt>
              <dd>
                <code>{props.unknown?.correlationId ?? ""}</code>
              </dd>
            </div>
          </dl>
          {props.reconcileFailed ? <p className={styles.fieldNote}>{t("plugin.install.reconcileFailed")}</p> : null}
          <div className={styles.toolbar}>
            <button type="button" className={styles.control} onClick={props.onReconcile}>
              {t("plugin.install.reconcile")}
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
            title={t("plugin.install.failureTitle")}
            body={t("plugin.list.failureBody")}
            note={t("plugin.list.connectionHint")}
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

function PluginRow({ plugin }: { plugin: Plugin }) {
  const t = useT();
  const unavailable = t("states.unavailable");

  const chip = pluginLifecycleChip(plugin.lifecycle_state);
  const ChipIcon = chip.icon;

  const version = plugin.manifest === undefined ? unavailable : fieldOrUnavailable(plugin.manifest.plugin_version, unavailable);

  // The trust cell: the Core-returned verification states verbatim; where
  // the Core returns none the state is not determinable — `Unknown`,
  // never `Verified` (P4.1 P1's law); where no package record exists,
  // Unavailable.
  const verification = plugin.package === undefined ? null : returnedVerificationStates(plugin.package);

  // The requested capabilities render from the manifest where the Core
  // serves one (the manifest is ABSENT on this Core — the honest
  // Unavailable stands); a declared name is DATA, never a grant.
  const capabilities = plugin.manifest === undefined
    ? null
    : plugin.manifest.requested_capabilities === undefined || plugin.manifest.requested_capabilities.length === 0
      ? null
      : plugin.manifest.requested_capabilities;

  return (
    <tr className={styles.row}>
      <td className={styles.cellName}>
        <Link to={`/plugins/${encodeURIComponent(plugin.plugin_id)}`} className={styles.rowLink}>
          <span className={styles.nameText}>{plugin.plugin_id}</span>
          <span className="visually-hidden">
            {t("plugin.list.openPlugin", { pluginId: plugin.plugin_id })}
          </span>
        </Link>
      </td>
      <td className={styles.cellId}>
        <CopyIdButton
          id={plugin.plugin_id}
          label={t("plugin.list.copyPluginId")}
          copiedLabel={t("plugin.list.copied")}
          failedLabel={t("plugin.list.copyFailed")}
          className={styles.copyId}
        />
      </td>
      <td className={styles.cell}>
        {/* The manifest's declared version renders where the Core serves
            a manifest; the manifest member is ABSENT on this Core, so the
            honest Unavailable stands (B5-A1 §2.4 gap 6). */}
        <span className={styles.mutedCell}>{version}</span>
      </td>
      <td className={styles.cell}>
        {plugin.package === undefined ? (
          <span className={styles.mutedCell}>{unavailable}</span>
        ) : verification !== null && verification.length > 0 ? (
          verification.map((state) => (
            <code key={state} className={styles.verificationValue}>
              {state}
            </code>
          ))
        ) : (
          // No verification state returned: trust is not determinable by
          // the UI — `Unknown`, NEVER `Verified` (the trust-word
          // discipline; P4.1 P1's binding law).
          <span className={styles.mutedCell}>{t("plugin.states.unknownTrust")}</span>
        )}
      </td>
      <td className={styles.cell}>
        {capabilities === null ? (
          <span className={styles.mutedCell}>{unavailable}</span>
        ) : (
          capabilities.map((capability) => (
            <code key={capability} className={styles.verificationValue}>
              {capability}
            </code>
          ))
        )}
      </td>
      <td className={styles.cell}>
        <span className={`${styles.chip} ${styles[chip.tone]}`}>
          <ChipIcon aria-hidden="true" size={14} />
          {t(chip.labelKey)}
        </span>
      </td>
    </tr>
  );
}
