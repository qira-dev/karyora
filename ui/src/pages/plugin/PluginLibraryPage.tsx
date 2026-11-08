import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { useCoreConnection } from "../../app/coreConnectionState";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import type { CoreWireEnvelope, Plugin } from "../../coreclient/httpLocal";
import { useT, useTranslation } from "../../i18n/useT";
import { PageHeading, PageLayout } from "../PageLayout";
import styles from "./PluginLibraryPage.module.css";
import { CopyIdButton, fieldOrUnavailable, pluginLifecycleChip } from "./pluginStates";

/**
 * `/plugins` — the Plugin list (B5-U1; the DE-0 queue row 9, the P4.1
 * P1–P6 rows against the now-live B5 adapter).
 *
 * What this page consumes (exactly as the Core adapter serves it):
 * - `GET /v1/plugins?limit=&cursor=` (the published list member's
 *   serving, B5-A1 §2.1 #1). No client-side limit is sent — the Core's
 *   default applies; the opaque cursor rides back verbatim and a
 *   cursorless response IS the final page (the coded end marker — no
 *   has_more/total is invented).
 * - THE NO-CATALOG LAW (carried, binding): USER-INSTALLED ≠
 *   CATALOG-LISTED, always — this list IS the user-installed list. No
 *   catalog surface, no install control (P2 — UI-29-withheld), no
 *   local-package import (P3), and no catalog refresh (P4) exists
 *   anywhere here: a catalog exists ONLY when the Core supplies one
 *   (none is published), and no dead stub renders (§31.4). The gate note
 *   (P6) states the accurate recorded reason.
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
    </PageLayout>
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
