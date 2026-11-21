/**
 * 13-Item Task Right-Click Context Menu (UIR-3.2, Screenshot #85, Blueprint §4.3 (4), UI-28).
 *
 * Implements all 13 canonical task actions (`Move to group`, `Pin`, `Rename`,
 * `Mark as unread`, `Archive`, `Delete`, `Copy Task ID`, `Copy working directory`,
 * `Export conversation`, `View log file`, `Open in new window`, `View model trajectory`,
 * `Report issue`) with a high-risk `ConfirmDialog` guard on `Delete`.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
import { ConfirmDialog, ContextMenu } from "../components/primitives";
import type { TaskSessionSummary } from "../coreclient/types";
import {
  TASK_CONTEXT_MENU_ITEMS,
  type TaskContextMenuActionDef,
} from "./shellConfig";

export type TaskContextActionId = TaskContextMenuActionDef["id"];

export interface TaskContextMenuProps {
  readonly task: TaskSessionSummary;
  readonly isOpen: boolean;
  readonly onAction: (actionId: TaskContextActionId, task: TaskSessionSummary) => void;
  readonly onClose: () => void;
}

const ACTION_ICONS: Readonly<Record<TaskContextActionId, ChromeIconName>> = {
  "move-to-group": "folder-open" as ChromeIconName,
  "pin": "pin",
  "rename": "edit",
  "mark-as-unread": "activity",
  "archive": "archive",
  "delete": "trash",
  "copy-task-id": "copy",
  "copy-working-directory": "copy",
  "export-conversation": "external-link",
  "view-log-file": "file-text",
  "open-in-new-window": "maximize",
  "view-model-trajectory": "brain-thinking",
  "report-issue": "alert-triangle",
};

export function TaskContextMenu({
  task,
  isOpen,
  onAction,
  onClose,
}: TaskContextMenuProps): ReactElement | null {
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);

  if (!isOpen && !confirmDeleteOpen) {
    return null;
  }

  return (
    <>
      {isOpen && (
        <div
          data-testid={`task-context-menu-${task.taskId}`}
          className="ky-popover-surface"
          style={{ top: "100%", insetInlineStart: "var(--space-2)" }}
        >
          <ContextMenu
            ariaLabel={`Actions for ${task.title}`}
            items={TASK_CONTEXT_MENU_ITEMS.map((item) => ({
              id: item.id,
              label:
                item.id === "pin" && task.isPinned
                  ? "Unpin"
                  : item.id === "mark-as-unread" && task.isUnread
                    ? "Mark as read"
                    : item.label,
              danger: item.danger,
              separatorBefore: item.separatorBefore,
              icon: (
                <ChromeIcon
                  name={
                    item.id === "move-to-group"
                      ? "project-open"
                      : ACTION_ICONS[item.id]
                  }
                  size={14}
                />
              ),
              shortcutHint: item.hasSubmenu ? "▸" : undefined,
            }))}
            onSelectItem={(actionId) => {
              const typedAction = actionId as TaskContextActionId;
              if (typedAction === "delete") {
                setConfirmDeleteOpen(true);
                return;
              }
              onAction(typedAction, task);
            }}
            onClose={onClose}
          />
        </div>
      )}

      <ConfirmDialog
        isOpen={confirmDeleteOpen}
        title={`Delete task "${task.title}"?`}
        explanation="Deleting this task removes its conversation transcript from your active history. Workspace source files on disk are never touched."
        confirmLabel="Delete task"
        isDestructive
        onConfirm={() => {
          setConfirmDeleteOpen(false);
          onAction("delete", task);
        }}
        onCancel={() => setConfirmDeleteOpen(false)}
      />
    </>
  );
}
