import { useCallback, useId, useState } from "react";

import { useCoreConnection } from "../../app/coreConnectionState";
import type { ExchangeOutcome } from "../../coreclient/httpLocal";
import { RecoveryPanel } from "../../components/RecoveryPanel";
import { useT, type TranslationKey } from "../../i18n/useT";
import styles from "./LocalBootstrapPanel.module.css";

/**
 * The interim manual bootstrap-exchange panel (C-1; the P-S3-01 recorded
 * candidate's client flow; IMPL-2).
 *
 * What this is, truthfully: the INTERIM delivery path for the C-1
 * bootstrap/exchange client flow — the user pastes the one-time
 * `"bootstrap-"` credential that the Core issued through its trusted-
 * local channel, and this panel exchanges it exactly once through the
 * Core's `POST /v1/session/exchange`. The FULL browser bootstrap UX
 * (automatic launch, deep-link hand-off) is a separately tracked owner
 * decision and is deliberately NOT built here; nothing in this panel
 * decides it. Issuance stays entirely on the Core's side: this panel
 * never mints, stores, or regenerates a credential.
 *
 * Truthful states only (no optimistic states, no retry loops):
 * - Pending disables the control once; there is no automatic retry — a
 *   new attempt is a new explicit user action.
 * - A typed Core refusal is surfaced verbatim (the D-35 envelope's code
 *   and bounded data through the shared recovery surface, with the
 *   server-generated correlation id as the safe reference). Browser-
 *   originated refusals the Core's boundary emits fail-closed
 *   (`MISSING_CSRF_TOKEN` / `ORIGIN_NOT_ALLOWED`) and are shown exactly
 *   as refused — never masked, never worked around.
 * - A lost or uninterpretable response is the unknown outcome: it is
 *   never proof of success or failure, because a consumed one-time
 *   credential cannot be un-consumed. The copy says exactly that.
 * - Success renders the safe metadata only (session id, scopes, expiry).
 *   The token itself is held in memory only and is never displayed,
 *   never persisted, and never leaves this page except in the
 *   Authorization header of calls to the user's own local Core.
 */

/** Renders a Core-set RFC 3339 expiry in the viewer's locale, or the
 *  raw value when it cannot be parsed (never a fabricated fallback). */
function formatExpiry(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

interface FailureCopy {
  readonly titleKey: TranslationKey;
  readonly bodyKey: TranslationKey;
  readonly values: Record<string, unknown>;
}

/** Maps a typed Core refusal onto stable locale keys — the envelope's
 *  code/data drive the copy; the stable code itself is always shown
 *  verbatim as the language-neutral fallback (never interpreted away). */
function describeFailure(outcome: Extract<ExchangeOutcome, { kind: "rejection" }>): FailureCopy {
  const envelope = outcome.envelope;
  const reason = envelope.data.reason;
  const field = envelope.data.field;
  if (envelope.code === "AUTHZ_DENIED" && typeof reason === "string") {
    if (reason === "SESSION_INVALID") {
      return {
        titleKey: "step1.bootstrap.failureSessionInvalidTitle",
        bodyKey: "step1.bootstrap.failureSessionInvalidBody",
        values: { code: envelope.code },
      };
    }
    return {
      titleKey: "step1.bootstrap.failureBoundaryTitle",
      bodyKey: "step1.bootstrap.failureBoundaryBody",
      values: { reason, code: envelope.code },
    };
  }
  if (envelope.code === "VALIDATION_FAILED") {
    return {
      titleKey: "step1.bootstrap.failureValidationTitle",
      bodyKey: "step1.bootstrap.failureValidationBody",
      values: { code: envelope.code, field: typeof field === "string" ? field : "" },
    };
  }
  return {
    titleKey: "step1.bootstrap.failureGenericTitle",
    bodyKey: "step1.bootstrap.failureGenericBody",
    values: { code: envelope.code },
  };
}

export function LocalBootstrapPanel() {
  const t = useT();
  const { phase, session, exchange } = useCoreConnection();
  const inputId = useId();
  const helpId = useId();
  const [credential, setCredential] = useState("");
  const [pending, setPending] = useState(false);
  const [outcome, setOutcome] = useState<ExchangeOutcome | null>(null);

  const connected = phase === "connected" && session !== null;

  const runExchange = useCallback(async () => {
    const trimmed = credential.trim();
    if (trimmed === "" || pending) return;
    setPending(true);
    try {
      const result = await exchange(trimmed);
      setOutcome(result);
      if (result.kind === "data") {
        // The one-time secret has served its single purpose; it never
        // lingers in the form's state (the provider-form rule, applied
        // to this credential too).
        setCredential("");
      }
    } finally {
      setPending(false);
    }
  }, [credential, pending, exchange]);

  return (
    <div className={styles.panel}>
      <h3 className={styles.panelHeading}>{t("step1.bootstrap.heading")}</h3>
      <p className={styles.panelHelp}>{t("step1.bootstrap.help")}</p>
      {phase === "session-expired" ? (
        <p className={styles.expiredNote} role="note">
          {t("step1.bootstrap.sessionExpiredNote")}
        </p>
      ) : null}
      {!connected ? (
        <>
          <label className="visually-hidden" htmlFor={inputId}>
            {t("step1.bootstrap.credentialLabel")}
          </label>
          <p className={styles.panelHelp} id={helpId}>
            {t("step1.bootstrap.credentialHelp")}
          </p>
          <form
            className={styles.credentialRow}
            onSubmit={(event) => {
              event.preventDefault();
              void runExchange();
            }}
          >
            <input
              id={inputId}
              type="password"
              className={styles.credentialInput}
              value={credential}
              autoComplete="off"
              spellCheck={false}
              aria-describedby={helpId}
              disabled={pending}
              placeholder={t("step1.bootstrap.credentialPlaceholder")}
              onChange={(event) => setCredential(event.target.value)}
            />
            <button
              type="submit"
              className={styles.exchangeButton}
              disabled={pending || credential.trim() === ""}
            >
              {pending ? t("step1.bootstrap.exchanging") : t("step1.bootstrap.exchangeButton")}
            </button>
          </form>
        </>
      ) : null}
      {connected && session !== null ? (
        <>
          <RecoveryPanel
            title={t("step1.bootstrap.successTitle")}
            body={t("step1.bootstrap.successBody")}
          />
          <ul className={styles.metaList}>
            <li>
              {t("session.sessionIdLabel")}: <code>{session.id}</code>
            </li>
            <li>
              {t("session.sessionScopesLabel")}: {session.scopes.join(", ")}
            </li>
            <li>
              {t("session.sessionExpiresLabel")}: {formatExpiry(session.expires)}
            </li>
          </ul>
        </>
      ) : null}
      {outcome !== null && outcome.kind === "rejection" && !connected ? (
        <RecoveryPanel
          title={t(describeFailure(outcome).titleKey)}
          body={t(describeFailure(outcome).bodyKey, describeFailure(outcome).values)}
          note={t("step1.bootstrap.failureNote")}
          referenceId={outcome.envelope.correlationId}
        />
      ) : null}
      {outcome !== null && outcome.kind === "unknown" ? (
        <RecoveryPanel
          title={t("step1.bootstrap.failureUnknownTitle")}
          body={t("step1.bootstrap.failureUnknownBody")}
          note={t("step1.bootstrap.failureNote")}
        />
      ) : null}
    </div>
  );
}
