/**
 * Workspace Top Title Bar — Calibrated 1:1 to ZCode Screenshots #70–#85, #79 Share Popover, #80 Help Menu.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, DiffCountPill, Input } from "../components/primitives";
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
  const { t } = useTranslation();
  const [shareOpen, setShareOpen] = useState(false);
  const [shareTitleDraft, setShareTitleDraft] = useState("");
  const [shareCheckedNoSensitive, setShareCheckedNoSensitive] = useState(false);
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
            fontSize: "0.9rem",
            fontWeight: 600,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {activeTask ? activeTask.title : surfaceTitle}
        </h2>
        {activeTask && activeTask.status === "running" && (
          <Badge
            tone="info"
            dot
            label={t("ui.topbar.working", { defaultValue: "Working" })}
          />
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

        {/* Share Conversation Preflight Popover (Exact Match to Screenshot #79) */}
        <div style={{ position: "relative" }}>
          <Button
            variant="ghost"
            size="sm"
            aria-haspopup="dialog"
            aria-expanded={shareOpen ? "true" : "false"}
            aria-label={t("ui.topbar.share", { defaultValue: "Share conversation" })}
            title={t("ui.topbar.share", { defaultValue: "Share conversation" })}
            disabled={!activeTask}
            disabledReason="Select an active task to create a share link"
            onClick={() => {
              setHelpOpen(false);
              setShareTitleDraft(activeTask?.title ?? "");
              setShareCheckedNoSensitive(false);
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
                width: "320px",
                padding: "16px",
                display: "flex",
                flexDirection: "column",
                gap: "12px",
              }}
            >
              <div style={{ fontWeight: 600, fontSize: "0.9rem" }}>
                {t("ui.topbar.share", { defaultValue: "Share conversation" })}
              </div>

              <Input
                label={t("ui.surfaces.shareTitleLabel", { defaultValue: "Share title" })}
                value={shareTitleDraft}
                onChange={(e) => setShareTitleDraft(e.target.value)}
              />

              <div style={{ display: "flex", flexDirection: "column", gap: "4px" }}>
                <span style={{ fontSize: "0.75rem", color: "var(--text-muted)", fontWeight: 500 }}>
                  {t("ui.surfaces.shareAccessLabel", { defaultValue: "Access" })}
                </span>
                <div
                  style={{
                    padding: "8px 10px",
                    borderRadius: "8px",
                    border: "1px solid var(--border-subtle)",
                    backgroundColor: "var(--bg-surface-2)",
                    fontSize: "0.8rem",
                    color: "var(--text-primary)",
                  }}
                >
                  {t("ui.surfaces.shareAccessValue", {
                    defaultValue: "Link: read-only snapshot",
                  })}
                </div>
              </div>

              <label
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "8px",
                  fontSize: "0.78rem",
                  color: "var(--text-secondary)",
                  cursor: "pointer",
                }}
              >
                <input
                  type="checkbox"
                  checked={shareCheckedNoSensitive}
                  onChange={(e) => setShareCheckedNoSensitive(e.target.checked)}
                />
                <span>
                  {t("ui.surfaces.shareCheckLabel", {
                    defaultValue: "I've checked: no sensitive info.",
                  })}
                </span>
              </label>

              <div
                style={{
                  display: "flex",
                  justifyContent: "flex-end",
                  gap: "8px",
                }}
              >
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setShareOpen(false)}
                >
                  {t("actions.cancel", { defaultValue: "Cancel" })}
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  disabled={!shareCheckedNoSensitive}
                  onClick={() => {
                    onCreateShareLink(activeTask.taskId);
                    setShareOpen(false);
                  }}
                >
                  {t("ui.surfaces.createShareLink", {
                    defaultValue: "Create share link",
                  })}
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
          aria-label={t("ui.topbar.terminal", { defaultValue: "Toggle terminal" })}
          title={`${t("ui.topbar.terminal", { defaultValue: "Toggle terminal" })} (Ctrl+J)`}
          onClick={onToggleTerminal}
        >
          <ChromeIcon name="terminal" size={16} />
        </Button>

        {/* Multi-Tab Side Pane Toggle (Ctrl+Alt+B, Screenshots #74, #78, #82) */}
        <Button
          variant="ghost"
          size="sm"
          selected={sidePaneOpen}
          aria-label={t("ui.topbar.sidePane", { defaultValue: "Toggle side pane" })}
          title={`${t("ui.topbar.sidePane", { defaultValue: "Toggle side pane" })} (Ctrl+Alt+B)`}
          onClick={onToggleSidePane}
        >
          <ChromeIcon name="side-pane-right" size={16} />
        </Button>

        {/* Notification Center Drawer Trigger */}
        <Button
          variant="ghost"
          size="sm"
          aria-label={`${t("ui.topbar.notifications", { defaultValue: "Notifications" })} (${unreadNotificationsCount})`}
          title={t("ui.topbar.notifications", { defaultValue: "Notifications" })}
          onClick={onToggleNotifications}
        >
          <ChromeIcon name="activity" size={16} />
          {unreadNotificationsCount > 0 && (
            <Badge tone="accent" label={String(unreadNotificationsCount)} />
          )}
        </Button>

        {/* `?` Help Menu (Screenshot #80) */}
        <div style={{ position: "relative" }}>
          <Button
            variant="ghost"
            size="sm"
            aria-haspopup="menu"
            aria-expanded={helpOpen ? "true" : "false"}
            aria-label={t("ui.topbar.help", { defaultValue: "Help" })}
            title={t("ui.topbar.help", { defaultValue: "Help" })}
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
                width: "228px",
                padding: "6px",
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
                        margin: "4px 0",
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
                  margin: "4px 0",
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
                {t("ui.surfaces.migrationGuide", { defaultValue: "Migration Guide" })}
              </Button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
