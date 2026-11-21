/**
 * Visible Serial Prompt Submission Queue Drawer (`Queue (N)`)
 * (UIR-4.1, Blueprint §7.3, §20.4, E2E-03).
 *
 * Displays queued prompts above the Agentic Composer with:
 * - Reorder controls (move up / move down)
 * - Inline edit (`Edit` / `Save`)
 * - Immediate interrupt / steer (`Send now`)
 * - Delete (`×`)
 * - Honest paused notice when active turn pauses for `pendingApproval`, `S3`, or `S4`
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Input } from "../../components/primitives";
import type { SessionQueueSnapshot } from "../state";

export interface SubmissionQueueDrawerProps {
  readonly snapshot: SessionQueueSnapshot;
  readonly onReorder: (fromIndex: number, toIndex: number) => void;
  readonly onEditPrompt: (queueItemId: string, nextPromptText: string) => void;
  readonly onSendNow: (queueItemId: string) => void;
  readonly onRemoveItem: (queueItemId: string) => void;
}

export function SubmissionQueueDrawer({
  snapshot,
  onReorder,
  onEditPrompt,
  onSendNow,
  onRemoveItem,
}: SubmissionQueueDrawerProps): ReactElement | null {
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editBuffer, setEditBuffer] = useState("");

  if (snapshot.queuedItems.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Prompt submission queue"
      data-testid="composer-submission-queue-drawer"
      className="ky-queue-drawer"
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <div style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)" }}>
          <span style={{ fontSize: "0.8125rem", fontWeight: 600 }}>
            Queue ({snapshot.queuedItems.length})
          </span>
          {snapshot.autoDrainPaused && (
            <Badge
              tone="warning"
              label={`Auto-drain paused (${snapshot.turnStatus})`}
            />
          )}
        </div>
        <span style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
          Serial dispatch on turn completion
        </span>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
        {snapshot.queuedItems.map((item, idx) => {
          const isEditing = editingId === item.queueItemId;
          return (
            <div
              key={item.queueItemId}
              style={{
                display: "flex",
                alignItems: "center",
                gap: "var(--space-2)",
                padding: "var(--space-2)",
                borderRadius: "var(--radius-sm)",
                backgroundColor: "var(--bg-surface-1)",
                border: "1px solid var(--border-subtle)",
              }}
            >
              <span
                style={{
                  fontSize: "0.75rem",
                  color: "var(--text-muted)",
                  fontFamily: "var(--font-mono)",
                }}
              >
                #{idx + 1}
              </span>

              {isEditing ? (
                <div style={{ flex: 1, display: "flex", gap: "var(--space-1)" }}>
                  <Input
                    aria-label="Edit queued prompt"
                    value={editBuffer}
                    onChange={(e) => setEditBuffer(e.target.value)}
                  />
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      onEditPrompt(item.queueItemId, editBuffer);
                      setEditingId(null);
                    }}
                  >
                    Save
                  </Button>
                </div>
              ) : (
                <span
                  style={{
                    flex: 1,
                    fontSize: "0.8125rem",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {item.promptText}
                </span>
              )}

              <div style={{ display: "inline-flex", gap: "2px", flexShrink: 0 }}>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={idx === 0}
                  aria-label="Move up"
                  onClick={() => onReorder(idx, idx - 1)}
                >
                  <ChromeIcon name="chevron-up" size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={idx === snapshot.queuedItems.length - 1}
                  aria-label="Move down"
                  onClick={() => onReorder(idx, idx + 1)}
                >
                  <ChromeIcon name="chevron-down" size={14} />
                </Button>
                {!isEditing && (
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label="Edit queued prompt"
                    onClick={() => {
                      setEditingId(item.queueItemId);
                      setEditBuffer(item.promptText);
                    }}
                  >
                    <ChromeIcon name="edit" size={14} />
                  </Button>
                )}
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onSendNow(item.queueItemId)}
                >
                  Send now
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Remove queued item"
                  onClick={() => onRemoveItem(item.queueItemId)}
                >
                  <ChromeIcon name="close" size={14} />
                </Button>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
