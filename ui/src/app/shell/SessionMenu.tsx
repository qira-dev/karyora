import { CircleUser } from "lucide-react";

import { useCoreConnection } from "../coreConnectionState";
import { Popover } from "../../components/Popover";
import { useT } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * User / session menu with the session-state display (§4.1 BOTTOM;
 * C-11; P2.7), now driven by real transport outcomes (IMPL-2).
 *
 * Truthful only: the display reflects what the Core's transport actually
 * answered. When a session is established through the C-1 exchange, the
 * safe metadata is shown (the Core's hash-prefix session id, the scopes,
 * the absolute expiry) — the token itself is held in memory only and is
 * never displayed, never persisted (§5.4: credentials are never stored
 * to preserve convenience). When the session no longer authorizes, the
 * display says exactly that and the token has been discarded from
 * memory. When no transport answers this origin, the honest unavailable
 * state renders exactly as before. No fabricated identity, no fake
 * sign-out.
 *
 * The re-authentication path stays the recorded open item (O-3 — the
 * Core/security owners own the re-auth mechanism): the button stays
 * disabled with its reason. Recovery from a session-expired state runs
 * through the existing setup path (the first-run wizard's interim
 * bootstrap panel), which is the one exchange flow this milestone
 * delivers; nothing here decides the dedicated re-auth flow's design.
 */
export function SessionMenu({ expanded }: { expanded: boolean }) {
  const t = useT();
  const { phase, session } = useCoreConnection();
  const hasSession = session !== null && (phase === "connected" || phase === "session-expired");

  return (
    <Popover
      label={t("session.popoverTitle")}
      alignment="start"
      trigger={({ open, toggle, describedById }) => (
        <button
          type="button"
          className={styles.trigger}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-describedby={describedById}
          title={t("session.label")}
          onClick={toggle}
        >
          <CircleUser aria-hidden="true" size={20} />
          <span className={expanded ? styles.triggerLabel : "visually-hidden"}>
            {t("session.label")}
          </span>
        </button>
      )}
    >
      {(close) => (
        <div>
          {hasSession && session !== null ? (
            <>
              <p className={styles.popoverBody}>
                {phase === "connected" ? t("session.stateActiveBody") : t("session.stateExpiredBody")}
              </p>
              <p className={styles.popoverNote}>
                <span className="visually-hidden">{t("session.sessionIdLabel")}: </span>
                <code>{session.id}</code>
              </p>
              <p className={styles.popoverNote}>
                {t("session.sessionScopesLabel")}: {session.scopes.join(", ")}
              </p>
              <p className={styles.popoverNote}>
                {t("session.sessionExpiresLabel")}: {formatExpiry(session.expires)}
              </p>
            </>
          ) : (
            <>
              <p className={styles.popoverBody}>{t("session.stateUnavailable")}</p>
              <p className={styles.popoverBody}>{t("session.stateRule")}</p>
            </>
          )}
          <div className={styles.sessionReauth}>
            <button type="button" className={styles.popoverClose} disabled aria-disabled="true">
              {t("session.reauthLabel")}
            </button>
            <p className={styles.popoverNote} id="session-reauth-reason">
              {t("session.reauthUnavailable")}
            </p>
            <p className={styles.popoverNote}>{t("session.reauthNote")}</p>
          </div>
          <button type="button" className={styles.popoverClose} onClick={close}>
            {t("actions.close")}
          </button>
        </div>
      )}
    </Popover>
  );
}

/** Renders a Core-set RFC 3339 expiry in the viewer's locale, or the
 *  raw value when it cannot be parsed (never a fabricated fallback). */
function formatExpiry(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
