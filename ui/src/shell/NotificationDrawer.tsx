/**
 * In-App & Opt-In OS Notification Center Drawer (UIR-3.4, Blueprint §14 (2), UI-34).
 *
 * Non-negotiable rules:
 * - Deduplicates notifications by stable Core event ID (`deduplicateNotifications`).
 * - Marking read or dismissing a notification NEVER approves, cancels, or deletes
 *   the underlying durable Job or Activity Audit record.
 * - OS notifications are strictly opt-in and use privacy-safe previews (never leaking
 *   prompts, tokens, file paths, or model output on lock screens).
 */

import type { ReactElement } from "react";
import { EmptyStateCard } from "../components/feedback";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Toggle } from "../components/primitives";
import type { CoreNotificationItem } from "./shellConfig";

export interface NotificationDrawerProps {
  readonly isOpen: boolean;
  readonly notifications: readonly CoreNotificationItem[];
  readonly osNotificationsEnabled: boolean;
  readonly onClose: () => void;
  readonly onMarkRead: (eventId: string) => void;
  readonly onDismiss: (eventId: string) => void;
  readonly onToggleOsNotifications: (enabled: boolean) => void;
  readonly onOpenNotificationTarget: (item: CoreNotificationItem) => void;
}

export function NotificationDrawer({
  isOpen,
  notifications,
  osNotificationsEnabled,
  onClose,
  onMarkRead,
  onDismiss,
  onToggleOsNotifications,
  onOpenNotificationTarget,
}: NotificationDrawerProps): ReactElement | null {
  if (!isOpen) {
    return null;
  }

  const visibleItems = notifications.filter((n) => !n.dismissed);

  return (
    <aside
      role="dialog"
      aria-label="Notification Center"
      data-testid="karyora-notification-drawer"
      className="ky-drawer-right"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          onClose();
        }
      }}
    >
      <header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          borderBottom: "1px solid var(--border-subtle)",
          paddingBottom: "var(--space-2)",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
          <ChromeIcon name="activity" size={16} />
          <h3 style={{ margin: 0, fontSize: "0.9375rem" }}>Notifications</h3>
        </div>
        <Button
          variant="ghost"
          size="sm"
          aria-label="Close notifications"
          onClick={onClose}
        >
          <ChromeIcon name="close" size={16} />
        </Button>
      </header>

      <Toggle
        checked={osNotificationsEnabled}
        onChange={onToggleOsNotifications}
        label="OS task notifications (opt-in)"
        description="Privacy-safe previews only — never exposes prompts, secrets, or paths."
      />

      <div
        style={{
          flex: 1,
          overflowY: "auto",
          display: "flex",
          flexDirection: "column",
          gap: "var(--space-2)",
        }}
      >
        {visibleItems.length === 0 ? (
          <EmptyStateCard
            title="All caught up"
            description="Durable Job completions, human approvals, provider checks, and integrity notices will appear here."
          />
        ) : (
          visibleItems.map((item) => (
            <article
              key={item.eventId}
              style={{
                padding: "var(--space-3)",
                borderRadius: "var(--radius-md)",
                border: "1px solid var(--border-subtle)",
                backgroundColor: item.read
                  ? "var(--bg-surface-1)"
                  : "var(--bg-surface-2)",
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-1)",
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "var(--space-2)",
                }}
              >
                <Badge
                  tone={item.category === "approval" ? "warning" : "info"}
                  label={item.category}
                />
                <span style={{ fontSize: "0.7rem", color: "var(--text-muted)" }}>
                  {item.occurredAt}
                </span>
              </div>
              <div style={{ fontSize: "0.875rem", fontWeight: 600 }}>
                {item.title}
              </div>
              <p
                style={{
                  margin: 0,
                  fontSize: "0.75rem",
                  color: "var(--text-secondary)",
                }}
              >
                {item.privacySafeSummary}
              </p>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "var(--space-2)",
                  marginTop: "var(--space-1)",
                }}
              >
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onOpenNotificationTarget(item)}
                >
                  Inspect
                </Button>
                {!item.read && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onMarkRead(item.eventId)}
                  >
                    Mark read
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onDismiss(item.eventId)}
                >
                  Dismiss
                </Button>
              </div>
            </article>
          ))
        )}
      </div>

      <p style={{ margin: 0, fontSize: "0.7rem", color: "var(--text-muted)" }}>
        Dismissing a notification never approves, cancels, or deletes the underlying
        durable Job or Audit entry.
      </p>
    </aside>
  );
}
