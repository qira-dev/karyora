import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { useFirstRun } from "../../app/firstRun/firstRunState";
import { useDirtyNavigationGuard } from "../../app/useDirtyNavigationGuard";
import { useCoreConnection } from "../../app/coreConnectionState";
import { SwitchConnectionDialog } from "../../app/shell/SwitchConnectionDialog";
import { Dialog, useDialogLabelId } from "../../components/Dialog";
import type { CoreWireEnvelope, WireConnectionView } from "../../coreclient/httpLocal/wire";
import { useT } from "../../i18n/useT";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import { PageHeading, PageLayout, PageSection } from "../PageLayout";
import styles from "./ConnectionsPage.module.css";
import { ProviderSetupForm, type ProviderSetupFormHandle } from "../firstRun/ProviderSetupForm";
import { ConnectionCredentialDialog } from "./ConnectionCredentialDialog";
import { ConnectionEditDialog, ConnectionRenameDialog } from "./ConnectionRecordDialogs";

/**
 * `/settings/connections` (C-14) — the Settings home of the connection
 * surfaces (C-08 add/rename/remove/test/re-authenticate, C-09 switch,
 * C-10 selector), per its frozen rule: "EXACTLY C-08/C-09/C-10's
 * contracts — cited, not restated."
 *
 * IMPL-8: the connection list is the Core's real connection-list read
 * (the card-9 list discipline — server-side cursor pagination, the
 * cursorless final page), and the per-connection commands are the Core's
 * real operations (test/rotate/re-authenticate/delete/switch — the
 * card-10 working candidate). The wired provider setup form (C-07) is
 * the same component the first-run wizard uses; its Test/Save commands
 * are live.
 *
 * Contract rules carried here:
 * - The C-10 empty rule (no connections → route to setup) renders as the
 *   setup entry below; a loading/unavailable/typed-refused list renders
 *   its honest state — never invented rows.
 * - Every state-changing command (rotate/re-auth/delete/switch) opens
 *   its explicit confirmation with the scope/effects stated (§8.9's
 *   dialog discipline; rotation confirms the affected model + possible
 *   interruption BEFORE the command per P3.3 §4.4). Escape mutates
 *   nothing.
 * - The four-outcome discipline binds every command (the IMPL-7
 *   pattern): the Core-confirmed result (the page immediately re-reads
 *   the list — the authoritative state, never the response alone), the
 *   typed refusal verbatim (the stable code + the correlation id), and
 *   a lost response = S4 unknown with the explicit status re-check —
 *   never auto-retried. Rotate/re-auth/delete/switch are Open-class
 *   operations (P1.6 §4): no idempotency key, so the recovery is the
 *   QUERY (re-reading the list), never a blind re-send.
 * - Secrets (X-6): no stored key is ever displayed — a saved key is
 *   NEVER re-rendered; rotate/re-auth open a fresh empty write-only
 *   field (the dialog owns it in local state only); the masked secret
 *   status is the only credential fact any row renders; nothing here
 *   writes to browser storage, URLs, logs, or telemetry.
 * - The tested/untested states render per P3.3 §4.3: Tested with the
 *   verified time; Saved-untested visibly labeled; Untested-after-change
 *   rendered stale-truthfully via the Core's tested-state truth
 *   (test_current=false with a tested instant = tested, changed since).
 * - Rename (IMPL-17 — the adopted S8 supply's P-S8-01 working candidate,
 *   the M-27 register row's rename portion as implemented) is a real
 *   per-row command over the Core's rename operation: the display name
 *   ONLY — never the connection identity, never the Job identity, never
 *   the execution history (the P3.1 M-27 row's binding rule). The row
 *   renders the record's display name when the record carries one.
 * - Edit (IMPL-17 — the adopted S8 supply's P-S8-02 working candidate)
 *   rides the Core's non-secret field update: EXACTLY the coded create
 *   shape's non-secret fields. The secret is NEVER re-displayed and NO
 *   secret field exists on the edit flow — a key change rides Replace
 *   key / Re-enter key (the coded rotate/re-authenticate contract). Any
 *   accepted edit renders the tested state stale-truthfully (the Core's
 *   coded inputs digest).
 * - The concurrent-edit guard (the adopted S8 supply's CHOSEN class,
 *   disclosed at implementation): both commands send the `updated_at`
 *   marker the row was read under; a stale marker is the Core's typed
 *   CONFLICT_STATE — rendered verbatim with the truthful no-overwrite
 *   note (the P1.6 §5 interim rule's rendering duty: the Core's typed
 *   outcome renders; the UI performs NO overwrite and NO merge; the
 *   recovery is the explicit list re-read, never a resend).
 * - The C-09 Core-switch flow (the dialog below) is its own
 *   contract and rides unchanged.
 */
export function ConnectionsPage() {
  const t = useT();
  const navigate = useNavigate();
  const { resumeSetup } = useFirstRun();
  const { transport, phase, refresh } = useCoreConnection();
  const formRef = useRef<ProviderSetupFormHandle>(null);
  const providerSectionRef = useRef<HTMLDivElement>(null);
  const switchTriggerRef = useRef<HTMLButtonElement>(null);
  const [switchOpen, setSwitchOpen] = useState(false);

  const isDirty = useCallback(() => formRef.current?.isDirty() ?? false, []);
  const guard = useDirtyNavigationGuard({
    isDirty,
    onSave: () => formRef.current?.attemptSave(),
    onDiscard: () => formRef.current?.discard(),
  });

  type ListPhase = "loading" | "unavailable" | "rejection" | "empty" | "records";
  const [listPhase, setListPhase] = useState<ListPhase>("loading");
  const [rows, setRows] = useState<readonly WireConnectionView[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [listFailure, setListFailure] = useState<CoreWireEnvelope | null>(null);
  // The read-sequence guard (the Jobs-list pattern): only the LATEST
  // issued list read applies, so a superseded read cannot race a newer one.
  const readSeqRef = useRef(0);

  const load = useCallback(
    (options: { readonly append: boolean; readonly markRefreshing: boolean }) => {
      if (phase !== "connected") {
        setListPhase("unavailable");
        return;
      }
      const seq = ++readSeqRef.current;
      if (options.markRefreshing) setListPhase("loading");
      const nextCursor = options.append ? cursor : null;
      void transport
        .connectionList({ ...(nextCursor ? { cursor: nextCursor } : {}), limit: 50 })
        .then((outcome) => {
          if (seq !== readSeqRef.current) return; // superseded
          if (outcome.kind === "data") {
            setRows((prev) => (options.append ? [...prev, ...outcome.data.connections] : outcome.data.connections));
            setCursor(outcome.data.cursor ?? null);
            setListPhase(outcome.data.connections.length === 0 && !options.append ? "empty" : "records");
          } else if (outcome.kind === "rejection") {
            setListFailure(outcome.envelope);
            setListPhase("rejection");
          } else if (outcome.kind === "unknown") {
            setListPhase("unavailable");
          } else {
            setListPhase("unavailable");
          }
        });
    },
    [phase, transport, cursor],
  );

  useEffect(() => {
    load({ append: false, markRefreshing: true });
  }, [load]);

  type PendingAction = { readonly connectionID: string; readonly action: "test" | "rotate" | "reauth" | "delete" | "switch" | "rename" | "update" } | null;

  // Static key lookups — typed keys (G-10) admit no dynamic keys.
  const SECRET_STATUS_KEYS: Record<string, "connections.secretStatus.present" | "connections.secretStatus.missing" | "connections.secretStatus.revoked" | "connections.secretStatus.corrupt" | "connections.secretStatus.inaccessible"> = {
    present: "connections.secretStatus.present",
    missing: "connections.secretStatus.missing",
    revoked: "connections.secretStatus.revoked",
    corrupt: "connections.secretStatus.corrupt",
    inaccessible: "connections.secretStatus.inaccessible",
  };
  const ACTION_KEYS: Record<"test" | "rotate" | "reauth" | "delete" | "switch" | "rename" | "update", "connections.action.test" | "connections.action.rotate" | "connections.action.reauth" | "connections.action.delete" | "connections.action.switch" | "connections.action.rename" | "connections.action.update"> = {
    test: "connections.action.test",
    rotate: "connections.action.rotate",
    reauth: "connections.action.reauth",
    delete: "connections.action.delete",
    switch: "connections.action.switch",
    rename: "connections.action.rename",
    update: "connections.action.update",
  };
  const [pending, setPending] = useState<PendingAction>(null);
  const [dialog, setDialog] = useState<
    | { readonly kind: "rotate" | "reauthenticate"; readonly connection: WireConnectionView }
    | { readonly kind: "delete"; readonly connection: WireConnectionView }
    | { readonly kind: "switch"; readonly connection: WireConnectionView }
    | { readonly kind: "rename"; readonly connection: WireConnectionView }
    | { readonly kind: "edit"; readonly connection: WireConnectionView }
    | null
  >(null);
  const deleteTriggerRef = useRef<HTMLButtonElement>(null);
  const rotateTriggerRef = useRef<HTMLButtonElement>(null);
  const reauthTriggerRef = useRef<HTMLButtonElement>(null);
  const switchConnectionTriggerRef = useRef<HTMLButtonElement>(null);
  const renameTriggerRef = useRef<HTMLButtonElement>(null);
  const editTriggerRef = useRef<HTMLButtonElement>(null);

  type ConnAction = "test" | "rotate" | "reauth" | "delete" | "switch" | "rename" | "update";
  type ActionOutcome =
    | { readonly kind: "confirmed"; readonly action: ConnAction; readonly connectionID: string; readonly detail: string }
    | { readonly kind: "refused"; readonly action: ConnAction; readonly envelope: CoreWireEnvelope }
    | { readonly kind: "unknown"; readonly action: ConnAction; readonly connectionID: string }
    | null;
  const [actionOutcome, setActionOutcome] = useState<ActionOutcome>(null);

  const handleOutcome = useCallback(
    (action: ConnAction, connectionID: string) => (outcome: { kind: "data" | "rejection" | "unknown" | "unavailable"; envelope?: CoreWireEnvelope; detail?: string }) => {
      if (outcome.kind === "data") {
        // The 200 IS the Core confirmation; the page re-reads the list —
        // the outcome renders from the authoritative state, never the
        // response alone.
        setActionOutcome({ kind: "confirmed", action, connectionID, detail: outcome.detail ?? "" });
        load({ append: false, markRefreshing: false });
      } else if (outcome.kind === "rejection" && outcome.envelope) {
        setActionOutcome({ kind: "refused", action, envelope: outcome.envelope });
      } else if (outcome.kind === "unknown") {
        setActionOutcome({ kind: "unknown", action, connectionID });
      } else {
        setListPhase("unavailable");
      }
    },
    [load],
  );

  const runTest = useCallback(
    (connection: WireConnectionView) => {
      if (pending !== null) return;
      setPending({ connectionID: connection.record.connection_id, action: "test" });
      void transport
        .connectionTest({ connection_id: connection.record.connection_id })
        .then((outcome) => {
          if (outcome.kind === "data") {
            if (outcome.data.report.ok) {
              setActionOutcome({
                kind: "confirmed",
                action: "test",
                connectionID: connection.record.connection_id,
                detail: outcome.data.report.correlation_id,
              });
              load({ append: false, markRefreshing: false });
            } else {
              // The sanitized failure is the report's typed outcome —
              // surfaced with its correlation id, no secret echo.
              setActionOutcome({
                kind: "refused",
                action: "test",
                envelope: {
                  code: outcome.data.report.error_kind ?? "INTERNAL",
                  retryable: false,
                  correlationId: outcome.data.report.correlation_id,
                  data: outcome.data.report.error_detail ? { reason: outcome.data.report.error_detail } : {},
                },
              });
            }
          } else if (outcome.kind === "rejection") {
            setActionOutcome({ kind: "refused", action: "test", envelope: outcome.envelope });
          } else if (outcome.kind === "unknown") {
            setActionOutcome({ kind: "unknown", action: "test", connectionID: connection.record.connection_id });
          } else {
            setListPhase("unavailable");
          }
        })
        .finally(() => setPending(null));
    },
    [pending, transport, load],
  );

  const runCredential = useCallback(
    (kind: "rotate" | "reauth", connectionID: string, secret: string) => {
      if (pending !== null) return;
      setPending({ connectionID, action: kind });
      const op = kind === "rotate" ? transport.connectionRotate : transport.connectionReauthenticate;
      void op(connectionID, secret)
        .then((outcome) => {
          handleOutcome(kind, connectionID)(outcome);
        })
        .finally(() => setPending(null));
    },
    [pending, transport, handleOutcome],
  );

  const runDelete = useCallback(
    (connectionID: string) => {
      if (pending !== null) return;
      setPending({ connectionID, action: "delete" });
      void transport
        .connectionDelete(connectionID)
        .then((outcome) => {
          handleOutcome("delete", connectionID)(outcome);
        })
        .finally(() => setPending(null));
    },
    [pending, transport, handleOutcome],
  );

  const runSwitch = useCallback(
    (connectionID: string) => {
      if (pending !== null) return;
      setPending({ connectionID, action: "switch" });
      void transport
        .connectionSwitch(connectionID)
        .then((outcome) => {
          handleOutcome("switch", connectionID)(outcome);
        })
        .finally(() => setPending(null));
    },
    [pending, transport, handleOutcome],
  );

  // The rename command (the M-27 rename portion as implemented): the
  // display name ONLY, carrying the marker the row was read under — a
  // stale marker is the Core's typed CONFLICT_STATE (the concurrent-edit
  // guard), rendered verbatim with the truthful no-overwrite note.
  const runRename = useCallback(
    (connectionID: string, displayName: string, expectedUpdatedAt: string) => {
      if (pending !== null) return;
      setPending({ connectionID, action: "rename" });
      void transport
        .connectionRename(connectionID, { display_name: displayName, expected_updated_at: expectedUpdatedAt })
        .then((outcome) => {
          handleOutcome("rename", connectionID)(outcome);
        })
        .finally(() => setPending(null));
    },
    [pending, transport, handleOutcome],
  );

  // The non-secret field update (the P-S8-02 working candidate): the
  // coded create shape's non-secret fields ONLY — no secret field exists
  // on this flow; the same concurrency marker and no-retry rule ride it.
  const runUpdate = useCallback(
    (connectionID: string, fields: { provider: string; base_url: string; model_id: string; custom_headers: { name: string; value: string }[] }, expectedUpdatedAt: string) => {
      if (pending !== null) return;
      setPending({ connectionID, action: "update" });
      void transport
        .connectionUpdate(connectionID, {
          provider: fields.provider,
          ...(fields.base_url ? { base_url: fields.base_url } : {}),
          model_id: fields.model_id,
          ...(fields.custom_headers.length > 0 ? { custom_headers: fields.custom_headers } : {}),
          expected_updated_at: expectedUpdatedAt,
        })
        .then((outcome) => {
          handleOutcome("update", connectionID)(outcome);
        })
        .finally(() => setPending(null));
    },
    [pending, transport, handleOutcome],
  );

  const openProviderSetup = useCallback(() => {
    providerSectionRef.current?.scrollIntoView({ block: "start" });
    providerSectionRef.current?.focus();
  }, []);

  const resumeFirstRun = useCallback(() => {
    resumeSetup();
    navigate("/");
  }, [resumeSetup, navigate]);

  return (
    <PageLayout>
      <PageHeading>{t("groups.connections.title")}</PageHeading>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.listTitle")}</h2>
        <p className={styles.help}>{t("connections.listIntro")}</p>
        {listPhase === "loading" ? <p className={styles.help}>{t("states.unavailable")}</p> : null}
        {listPhase === "unavailable" ? (
          <div className={styles.stateBlock} role="note">
            <p className={styles.stateTitle}>{t("states.unavailable")}</p>
            <p className={styles.stateBody}>{t("connections.listUnavailable")}</p>
          </div>
        ) : null}
        {listPhase === "rejection" && listFailure !== null ? (
          <RecoveryPanel
            title={t("connections.listRefusedTitle")}
            body={t("connections.listRefusedBody", { code: listFailure.code })}
            note={t("connections.listRefusedNote", { correlationId: listFailure.correlationId })}
            actions={[
              { label: t("connections.checkStatus"), onAction: () => load({ append: false, markRefreshing: true }) },
              { label: t("connections.dismiss"), onAction: () => setListPhase("unavailable") },
            ]}
          />
        ) : null}
        {listPhase === "empty" ? (
          <p className={styles.help}>{t("provider.emptyState")}</p>
        ) : null}
        {listPhase === "records" ? (
          <ul className={styles.connectionList}>
            {rows.map((connection) => (
              <li key={connection.record.connection_id} className={styles.connectionRow} data-active={connection.active ? "true" : undefined}>
                <div className={styles.connectionMeta}>
                  <p className={styles.connectionName}>
                    {connection.record.display_name ? (
                      <span className={styles.connectionDisplayName}>{connection.record.display_name}</span>
                    ) : null}
                    <span className={styles.connectionProvider}>{connection.record.provider}</span>
                    <span className={styles.connectionModel}>{connection.record.model_id}</span>
                    {connection.active ? <span className={styles.activeBadge}>{t("connections.activeBadge")}</span> : null}
                  </p>
                  {connection.record.base_url ? (
                    <p className={styles.connectionLine}>{connection.record.base_url}</p>
                  ) : null}
                  <p className={styles.connectionLine}>
                    <span className={styles.connectionLabel}>{t("connections.secretStatusLabel")}: </span>
                    <span className={styles.connectionValue}>
                      {t(SECRET_STATUS_KEYS[connection.secret_status] ?? "connections.secretStatus.inaccessible")}
                    </span>
                  </p>
                  <p className={styles.connectionLine}>
                    <span className={styles.connectionLabel}>{t("connections.testedLabel")}: </span>
                    <span className={styles.connectionValue}>
                      {connection.record.tested_at
                        ? connection.test_current
                          ? t("connections.testedCurrent", { time: connection.record.tested_at })
                          : t("connections.testedChanged")
                        : t("connections.testedNever")}
                    </span>
                  </p>
                  <p className={styles.connectionId}>{connection.record.connection_id}</p>
                </div>
                <div className={styles.connectionActions}>
                  <button
                    ref={renameTriggerRef}
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null}
                    onClick={() => setDialog({ kind: "rename", connection })}
                  >
                    {t("connections.opRename")}
                  </button>
                  <button
                    ref={editTriggerRef}
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null}
                    onClick={() => setDialog({ kind: "edit", connection })}
                  >
                    {t("connections.opEdit")}
                  </button>
                  <button
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null}
                    onClick={() => runTest(connection)}
                  >
                    {t("connections.opTest")}
                  </button>
                  <button
                    ref={rotateTriggerRef}
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null}
                    onClick={() => setDialog({ kind: "rotate", connection })}
                  >
                    {t("connections.opRotate")}
                  </button>
                  <button
                    ref={reauthTriggerRef}
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null}
                    onClick={() => setDialog({ kind: "reauthenticate", connection })}
                  >
                    {t("connections.opReauth")}
                  </button>
                  <button
                    ref={switchConnectionTriggerRef}
                    type="button"
                    className={styles.secondaryButton}
                    disabled={pending !== null || connection.active}
                    onClick={() => setDialog({ kind: "switch", connection })}
                  >
                    {t("connections.opSwitch")}
                  </button>
                  <button
                    ref={deleteTriggerRef}
                    type="button"
                    className={styles.dangerButton}
                    disabled={pending !== null}
                    onClick={() => setDialog({ kind: "delete", connection })}
                  >
                    {t("connections.opRemove")}
                  </button>
                </div>
              </li>
            ))}
          </ul>
        ) : null}
        {listPhase === "records" && cursor !== null ? (
          <button
            type="button"
            className={styles.secondaryButton}
            onClick={() => load({ append: true, markRefreshing: false })}
          >
            {t("connections.loadMore")}
          </button>
        ) : null}
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.operationsTitle")}</h2>
        <p className={styles.help}>{t("connections.operationsIntro")}</p>
        <ul className={styles.operationList}>
          <li className={styles.operationItem}>
            <button type="button" className={styles.operationButton} onClick={openProviderSetup}>
              {t("connections.opAddGo")}
            </button>
            <p className={styles.operationReason}>
              <span className={styles.operationName}>{t("connections.opAdd")}: </span>
              {t("connections.opAddNote")}
            </p>
          </li>
          <li className={styles.operationItem}>
            <button
              ref={switchTriggerRef}
              type="button"
              className={styles.operationButton}
              onClick={() => setSwitchOpen(true)}
            >
              {t("connections.switchTitle")}
            </button>
            <p className={styles.operationReason}>{t("connections.switchIntro")}</p>
          </li>
        </ul>
      </PageSection>

      <PageSection>
        <div ref={providerSectionRef} tabIndex={-1} className={styles.providerAnchor}>
          <h2 className={styles.sectionTitle}>{t("connections.providerSectionTitle")}</h2>
        </div>
        <p className={styles.help}>{t("connections.providerIntro")}</p>
        <ProviderSetupForm ref={formRef} />
      </PageSection>

      <PageSection>
        <h2 className={styles.sectionTitle}>{t("connections.firstRunTitle")}</h2>
        <p className={styles.help}>{t("connections.firstRunBody")}</p>
        <button type="button" className={styles.resumeButton} onClick={resumeFirstRun}>
          {t("connections.resumeSetup")}
        </button>
      </PageSection>

      <ConnectionCredentialDialog
        open={dialog?.kind === "rotate" || dialog?.kind === "reauthenticate"}
        kind={dialog?.kind === "reauthenticate" ? "reauthenticate" : "rotate"}
        connection={dialog?.kind === "rotate" || dialog?.kind === "reauthenticate" ? dialog.connection : null}
        pending={pending !== null && (pending.action === "rotate" || pending.action === "reauth")}
        onConfirm={(connectionID, secret) => runCredential(dialog?.kind === "reauthenticate" ? "reauth" : "rotate", connectionID, secret)}
        onClose={() => setDialog(null)}
        returnFocusRef={dialog?.kind === "reauthenticate" ? reauthTriggerRef : rotateTriggerRef}
      />

      <ConnectionRenameDialog
        open={dialog?.kind === "rename"}
        connection={dialog?.kind === "rename" ? dialog.connection : null}
        pending={pending?.action === "rename"}
        onConfirm={runRename}
        onClose={() => setDialog(null)}
        returnFocusRef={renameTriggerRef}
      />

      <ConnectionEditDialog
        open={dialog?.kind === "edit"}
        connection={dialog?.kind === "edit" ? dialog.connection : null}
        pending={pending?.action === "update"}
        onConfirm={runUpdate}
        onClose={() => setDialog(null)}
        returnFocusRef={editTriggerRef}
      />

      {dialog?.kind === "delete" ? (
        <ConfirmDialog
          open
          title={t("connections.deleteTitle")}
          body={t("connections.deleteBody", { connectionId: dialog.connection.record.connection_id })}
          effects={t("connections.deleteEffects", { provider: dialog.connection.record.provider })}
          confirmLabel={t("connections.deleteConfirm")}
          pending={pending?.action === "delete"}
          returnFocusRef={deleteTriggerRef}
          onConfirm={() => {
            runDelete(dialog.connection.record.connection_id);
            setDialog(null);
          }}
          onClose={() => setDialog(null)}
        />
      ) : null}
      {dialog?.kind === "switch" ? (
        <ConfirmDialog
          open
          title={t("connections.switchConnTitle")}
          body={t("connections.switchConnBody", { connectionId: dialog.connection.record.connection_id })}
          effects={t("connections.switchConnEffects")}
          confirmLabel={t("connections.switchConnConfirm")}
          pending={pending?.action === "switch"}
          returnFocusRef={switchConnectionTriggerRef}
          onConfirm={() => {
            runSwitch(dialog.connection.record.connection_id);
            setDialog(null);
          }}
          onClose={() => setDialog(null)}
        />
      ) : null}

      <SwitchConnectionDialog
        open={switchOpen}
        onClose={() => setSwitchOpen(false)}
        returnFocusRef={switchTriggerRef}
      />
      {guard.guardDialog}

      {actionOutcome?.kind === "confirmed" ? (
        <RecoveryPanel
          title={t("connections.outcomeConfirmedTitle")}
          body={t("connections.outcomeConfirmedBody", {
            action: t(ACTION_KEYS[actionOutcome.action]),
            connectionId: actionOutcome.connectionID,
          })}
          note={t("connections.outcomeConfirmedNote")}
          actions={[{ label: t("connections.dismiss"), onAction: () => setActionOutcome(null) }]}
        />
      ) : null}
      {actionOutcome?.kind === "refused" ? (
        <RecoveryPanel
          title={t("connections.outcomeRefusedTitle")}
          body={
            actionOutcome.envelope.code === "CONFLICT_STATE"
              ? // The concurrent-edit guard's typed refusal (the P1.6 §5
                // interim rule's rendering duty): the code verbatim PLUS the
                // truthful no-overwrite note — the Core's typed outcome
                // renders; the UI performs no overwrite and no merge; the
                // recovery is the explicit list re-read.
                `${t("connections.outcomeRefusedBody", {
                  action: t(ACTION_KEYS[actionOutcome.action]),
                  code: actionOutcome.envelope.code,
                })} ${t("connections.conflictNote")}`
              : t("connections.outcomeRefusedBody", {
                  action: t(ACTION_KEYS[actionOutcome.action]),
                  code: actionOutcome.envelope.code,
                })
          }
          note={t("connections.outcomeRefusedNote", { correlationId: actionOutcome.envelope.correlationId })}
          actions={[
            { label: t("connections.checkStatus"), onAction: () => load({ append: false, markRefreshing: false }) },
            { label: t("connections.dismiss"), onAction: () => setActionOutcome(null) },
          ]}
        />
      ) : null}
      {actionOutcome?.kind === "unknown" ? (
        <RecoveryPanel
          title={t("connections.outcomeUnknownTitle")}
          body={t("connections.outcomeUnknownBody", {
            action: t(ACTION_KEYS[actionOutcome.action]),
            connectionId: actionOutcome.connectionID,
          })}
          note={t("connections.outcomeUnknownNote")}
          actions={[
            { label: t("connections.checkStatus"), onAction: () => { setActionOutcome(null); load({ append: false, markRefreshing: true }); refresh(); } },
            { label: t("connections.dismiss"), onAction: () => setActionOutcome(null) },
          ]}
        />
      ) : null}
    </PageLayout>
  );
}

/** The explicit confirmation dialog (the §8.9 discipline): the scope and
 *  the effects stated before the command; Escape never decides anything. */
function ConfirmDialog(props: {
  readonly open: boolean;
  readonly title: string;
  readonly body: string;
  readonly effects: string;
  readonly confirmLabel: string;
  readonly pending: boolean;
  readonly returnFocusRef?: React.RefObject<HTMLElement | null>;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}) {
  const t = useT();
  const labelId = useDialogLabelId();
  return (
    <Dialog labelId={labelId} title={props.title} open={props.open} onClose={props.onClose} returnFocusRef={props.returnFocusRef}>
      <div className={styles.dialogBody}>
        <p className={styles.dialogBodyText}>{props.body}</p>
        <p className={styles.dialogEffects}>{props.effects}</p>
        <div className={styles.dialogActions}>
          <button
            type="button"
            className={styles.dangerButton}
            disabled={props.pending}
            aria-disabled={props.pending ? "true" : undefined}
            onClick={props.onConfirm}
          >
            {props.confirmLabel}
          </button>
          <button type="button" className={styles.secondaryButton} onClick={props.onClose}>
            {t("connections.cancel")}
          </button>
        </div>
        <p className={styles.dialogHint}>{t("connections.dialogDurableNote")}</p>
      </div>
    </Dialog>
  );
}
