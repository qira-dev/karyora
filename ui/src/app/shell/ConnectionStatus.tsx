import { useCallback, useRef, useState } from "react";
import { CircleAlert, CircleCheck, CircleDashed, RefreshCw, Unplug } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { useCoreConnection, type CoreConnectionPhase } from "../coreConnectionState";
import { SwitchConnectionDialog } from "./SwitchConnectionDialog";
import { Popover } from "../../components/Popover";
import { useT, type TranslationKey } from "../../i18n/useT";
import styles from "./shellControls.module.css";

/**
 * Connection indicator + popover (C-02; Blueprint §3.2/§4.1 BOTTOM).
 *
 * Truthful states only, now driven by REAL transport outcomes (IMPL-2):
 * the CoreConnectionProvider probes the Core's local transport with one
 * real replay read and the phase derives exclusively from what actually
 * answered. When no transport answers this page's origin (the Core's
 * local transport is opt-in and disabled by default; a static
 * deployment has none), the phase is Unknown and the display is exactly
 * the P2.6/P2.7 truthful-unavailable state — never a connected, stale,
 * or populated state, and no state is invented.
 *
 * The designed `stale` state has no real trigger in this milestone (no
 * data surface consumes events yet), so it is not rendered and nothing
 * fabricates it.
 *
 * P2.7: the `Switch connection` entry still opens the C-09/C-10 switch
 * flow (a dedicated flow, not a silent selector — §21.10). The flow
 * renders its honest states; nothing switches, because the switch is a
 * Core-mediated context change and no connection-list surface exists in
 * the delivered transport.
 *
 * Accessibility (C-02): keyboard-accessible trigger; popover focus is
 * contained; Escape closes with focus return; never hover-only; status
 * is icon + text — never color alone.
 */

/** The per-phase icon: a distinct glyph per state so the state never
 *  rides on color alone (C-02). The glyphs are static — no animation
 *  exists without its written motion contract and reduced-motion
 *  handling (the project's motion rule). */
const PHASE_ICONS: Record<CoreConnectionPhase, LucideIcon> = {
  connecting: CircleDashed,
  unknown: CircleDashed,
  disconnected: Unplug,
  connected: CircleCheck,
  reconnecting: RefreshCw,
  "session-expired": CircleAlert,
};

const PHASE_LABEL_KEYS: Record<CoreConnectionPhase, TranslationKey> = {
  connecting: "connection.stateConnecting",
  unknown: "connection.stateUnknown",
  disconnected: "connection.stateDisconnected",
  connected: "connection.stateConnected",
  reconnecting: "connection.stateReconnecting",
  "session-expired": "connection.stateSessionExpired",
};

const PHASE_EXPLANATION_KEYS: Record<CoreConnectionPhase, TranslationKey> = {
  connecting: "connection.connectingExplanation",
  unknown: "connection.unknownExplanation",
  disconnected: "connection.disconnectedExplanation",
  connected: "connection.connectedExplanation",
  reconnecting: "connection.reconnectingExplanation",
  "session-expired": "connection.sessionExpiredExplanation",
};

export function ConnectionStatus({ expanded }: { expanded: boolean }) {
  const t = useT();
  const { phase } = useCoreConnection();
  const Icon = PHASE_ICONS[phase];
  const label = t(PHASE_LABEL_KEYS[phase]);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [switchOpen, setSwitchOpen] = useState(false);
  const openSwitch = useCallback(() => {
    setSwitchOpen(true);
  }, []);

  return (
    <>
      <Popover
        label={t("connection.popoverTitle")}
        alignment="start"
        trigger={({ open, toggle, describedById }) => (
          <button
            ref={triggerRef}
            type="button"
            className={styles.trigger}
            aria-haspopup="dialog"
            aria-expanded={open}
            aria-describedby={describedById}
            title={t("connection.label")}
            onClick={toggle}
          >
            <Icon aria-hidden="true" size={20} className={styles.stateIconUnknown} />
            <span className={expanded ? styles.triggerLabel : "visually-hidden"}>
              {t("connection.label")}: {label}
            </span>
          </button>
        )}
      >
        {(close) => (
          <div>
            <div className={styles.popoverHeaderRow}>
              <span className={styles.statusChip} data-state={phase}>
                <Icon aria-hidden="true" size={16} />
                {label}
              </span>
            </div>
            <p className={styles.popoverBody}>{t(PHASE_EXPLANATION_KEYS[phase])}</p>
            {phase === "unknown" ? <p className={styles.popoverBody}>{t("connection.listUnavailable")}</p> : null}
            <div className={styles.popoverActions}>
              <button
                type="button"
                className={styles.popoverClose}
                onClick={() => {
                  close();
                  openSwitch();
                }}
              >
                {t("connection.switch.title")}
              </button>
              <button type="button" className={styles.popoverClose} onClick={close}>
                {t("actions.close")}
              </button>
            </div>
          </div>
        )}
      </Popover>
      <SwitchConnectionDialog
        open={switchOpen}
        onClose={() => setSwitchOpen(false)}
        returnFocusRef={triggerRef}
      />
    </>
  );
}
