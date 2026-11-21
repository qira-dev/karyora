/**
 * Workspace Top Title Bar (UIR-3.3, Screenshots #70–#85, #79 Share Popover, #80 Help Menu).
 *
 * Implements:
 * - Left: Sidebar reopen button (when collapsed) + active task/surface title
 * - Right: Clickable `+A -R` diff pill, `Share conversation` preflight popover (#79),
 *   Bottom Terminal toggle (`Ctrl+J`), Side-Pane toggle (`Ctrl+Alt+B`),
 *   Notifications drawer toggle, and 7-item `?` Help menu (#80)
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, DiffCountPill } from "../components/primitives";
import type { TaskSessionSummary } from "../coreclient/types";
import { validateExternalUrlForOpen } from "../platform-bridge";
import { HELP_MENU_ITEMS } from "./shellConfig";

export interface WorkspaceTopBarProps {
  readonly sidebarCollapsed: boolean;
  readonly surfaceTitle: string;
  readonly activeTask: TaskSessionSummary | null;
  readonly terminalOpen: boolean;
  readonly sidePaneOpen: boolean;
  readonly unreadNotificationsCount: number;
  readonly onExpandSidebar: () => void;
  readonly onOpenReviewDiffTab: () => void;
  readonly onCreateShareLink: (taskId: string) => void;
  readonly onToggleTerminal: () => void;
  readonly onToggleSidePane: () => void;
  readonly onToggleNotifications: () => void;
  readonly onOpenAboutSettings: () => void;
  readonly onOpenMigrationGuide: () => void;
}

export function WorkspaceTopBar({
  sidebarCollapsed,
  surfaceTitle,
  activeTask,
  terminalOpen,
  sidePaneOpen,
  unreadNotificationsCount,
  onExpandSidebar,
  onOpenReviewDiffTab,
  onCreateShareLink,
  onToggleTerminal,
  onToggleSidePane,
  onToggleNotifications,
  onOpenAboutSettings,
  onOpenMigrationGuide,
}: WorkspaceTopBarProps): ReactElement {
  const [shareOpen, setShareOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);

  return (
    <header
      aria-label="Workspace top title bar"
      data-testid="karyora-workspace-topbar"
      className="ky-topbar"
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--space-2)",
          minWidth: 0,
        }}
      >
        {sidebarCollapsed && (
          <Button
            variant="ghost"
            size="sm"
            aria-label="Expand sidebar (Ctrl+B)"
            title="Expand Sidebar (Ctrl+B)"
            onClick={onExpandSidebar}
          >
            <ChromeIcon name="sidebar-left" size={16} />
          </Button>
        )}
        <h2
          style={{
            margin: 0,
            fontSize: "0.9375rem",
            fontWeight: 600,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {activeTask ? activeTask.title : surfaceTitle}
        </h2>
        {activeTask && activeTask.status === "running" && (
          <Badge tone="info" dot label="Working" />
        )}
      </div>

      <div
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: "var(--space-1)",
          flexShrink: 0,
        }}
      >
        {activeTask && (
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            aria-label="Open task file diff review"
            onClick={onOpenReviewDiffTab}
          >
            <DiffCountPill
              additions={activeTask.diffAdditions}
              deletions={activeTask.diffDeletions}
              filesCount={activeTask.changedFilesCount}
            />
          </button>
        )}

        {/* Share Conversation Preflight Popover (Screenshot #79) */}
        <div style={{ position: "relative" }}>
          <Button
            variant="ghost"
            size="sm"
            aria-haspopup="dialog"
            aria-expanded={shareOpen ? "true" : "false"}
            aria-label="Share conversation"
            title="Share conversation"
            disabled={!activeTask}
            disabledReason="Select an active task to create a share link"
            onClick={() => {
              setHelpOpen(false);
              setShareOpen((prev) => !prev);
            }}
          >
            <ChromeIcon name="share" size={16} />
          </Button>

          {shareOpen && activeTask && (
            <div
              role="dialog"
              aria-label="Share conversation preflight"
              data-testid="share-conversation-popover"
              className="ky-popover-surface"
              style={{
                top: "calc(100% + 6px)",
                insetInlineEnd: 0,
                width: "300px",
                padding: "var(--space-4)",
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-3)",
              }}
            >
              <div style={{ fontWeight: 600, fontSize: "0.875rem" }}>
                Share conversation
              </div>
              <p
                style={{
                  margin: 0,
                  fontSize: "0.75rem",
                  color: "var(--text-secondary)",
                }}
              >
                Creates a read-only snapshot link governed by your Core share policy.
                Secrets and OS-keyring credentials are never included. You can revoke
                active links anytime in Settings → Shares.
              </p>
              <div
                style={{
                  display: "flex",
                  justifyContent: "flex-end",
                  gap: "var(--space-2)",
                }}
              >
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setShareOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => {
                    onCreateShareLink(activeTask.taskId);
                    setShareOpen(false);
                  }}
                >
                  Create share link
                </Button>
              </div>
            </div>
          )}
        </div>

        {/* Bottom Terminal Toggle (Ctrl+J, Screenshot #81) */}
        <Button
          variant="ghost"
          size="sm"
          selected={terminalOpen}
          aria-label="Toggle Terminal (Ctrl+J)"
          title="Toggle Terminal (Ctrl+J)"
          onClick={onToggleTerminal}
        >
          <ChromeIcon name="terminal" size={16} />
        </Button>

        {/* Multi-Tab Side Pane Toggle (Ctrl+Alt+B, Screenshots #74, #78, #82) */}
        <Button
          variant="ghost"
          size="sm"
          selected={sidePaneOpen}
          aria-label="Toggle Side Pane (Ctrl+Alt+B)"
          title="Toggle Side Pane (Ctrl+Alt+B)"
          onClick={onToggleSidePane}
        >
          <ChromeIcon name="side-pane-right" size={16} />
        </Button>

        {/* Notification Center Drawer Trigger */}
        <Button
          variant="ghost"
          size="sm"
          aria-label={`Notifications (${unreadNotificationsCount} unread)`}
          title="Notifications"
          onClick={onToggleNotifications}
        >
          <ChromeIcon name="activity" size={16} />
          {unreadNotificationsCount > 0 && (
            <Badge tone="accent" label={String(unreadNotificationsCount)} />
          )}
        </Button>

        {/* `?` Help Menu (Screenshot #80 — All 7 Canonical Items) */}
        <div style={{ position: "relative" }}>
          <Button
            variant="ghost"
            size="sm"
            aria-haspopup="menu"
            aria-expanded={helpOpen ? "true" : "false"}
            aria-label="Help and resources"
            title="Help"
            onClick={() => {
              setShareOpen(false);
              setHelpOpen((prev) => !prev);
            }}
          >
            <ChromeIcon name="help" size={16} />
          </Button>

          {helpOpen && (
            <div
              role="menu"
              aria-label="Help menu"
              data-testid="topbar-help-menu"
              className="ky-popover-surface"
              style={{
                top: "calc(100% + 6px)",
                insetInlineEnd: 0,
                width: "220px",
                padding: "var(--space-1)",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.preventDefault();
                  setHelpOpen(false);
                }
              }}
            >
              {HELP_MENU_ITEMS.map((item) => (
                <div key={item.id}>
                  {item.separatorBefore && (
                    <hr
                      role="separator"
                      style={{
                        border: "none",
                        borderTop: "1px solid var(--border-subtle)",
                        margin: "var(--space-1) 0",
                      }}
                    />
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    role="menuitem"
                    style={{ width: "100%", justifyContent: "space-between" }}
                    onClick={() => {
                      setHelpOpen(false);
                      if (item.id === "about") {
                        onOpenAboutSettings();
                      } else if (item.externalUrl) {
                        const checked = validateExternalUrlForOpen(item.externalUrl);
                        if (checked.allowed && typeof window !== "undefined") {
                          window.open(
                            checked.normalizedUrl,
                            "_blank",
                            "noopener,noreferrer",
                          );
                        }
                      }
                    }}
                  >
                    <span>{item.label}</span>
                    {item.externalUrl && (
                      <ChromeIcon name="external-link" size={14} />
                    )}
                  </Button>
                </div>
              ))}
              <hr
                role="separator"
                style={{
                  border: "none",
                  borderTop: "1px solid var(--border-subtle)",
                  margin: "var(--space-1) 0",
                }}
              />
              <Button
                variant="ghost"
                size="sm"
                role="menuitem"
                style={{ width: "100%", justifyContent: "flex-start" }}
                onClick={() => {
                  setHelpOpen(false);
                  onOpenMigrationGuide();
                }}
              >
                Migration Guide (6 steps)
              </Button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
