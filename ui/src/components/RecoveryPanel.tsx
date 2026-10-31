import { useEffect, useRef } from "react";

import styles from "./RecoveryPanel.module.css";

/**
 * The shared recovery panel (P2.3 §3.2; Blueprint §25.3): a clear state
 * title, a short explanation, whether saved state is preserved, safe
 * next actions, and an optional safe reference ID for diagnostics.
 *
 * This panel is the shared S4/failure surface — it never claims a
 * success and never invents a reference. All copy is passed in already
 * localized by the calling surface; actions are rendered as buttons.
 */
export interface RecoveryAction {
  readonly label: string;
  readonly onAction: () => void;
}

export interface RecoveryPanelProps {
  readonly title: string;
  readonly body: string;
  /** One line stating what is preserved (or what was cleared) — §25.3. */
  readonly note?: string;
  readonly actions?: readonly RecoveryAction[];
  /** A safe diagnostics reference (correlation ID) — never secret content. */
  readonly referenceId?: string;
}

export function RecoveryPanel({ title, body, note, actions, referenceId }: RecoveryPanelProps) {
  const panelRef = useRef<HTMLDivElement>(null);

  // Focus the panel on mount so keyboard and screen-reader users land on
  // the outcome (§7.4 focus behavior for replaced content regions).
  useEffect(() => {
    panelRef.current?.focus();
  }, []);

  return (
    <div ref={panelRef} className={styles.panel} role="group" aria-label={title} tabIndex={-1}>
      <p className={styles.title}>{title}</p>
      <p className={styles.body}>{body}</p>
      {note ? <p className={styles.note}>{note}</p> : null}
      {referenceId ? (
        <p className={styles.note}>
          <span className="visually-hidden">Reference: </span>
          <code className={styles.reference}>{referenceId}</code>
        </p>
      ) : null}
      {actions && actions.length > 0 ? (
        <div className={styles.actions}>
          {actions.map((action) => (
            <button key={action.label} type="button" className={styles.action} onClick={action.onAction}>
              {action.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
