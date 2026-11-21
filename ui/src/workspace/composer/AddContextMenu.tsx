/**
 * Composer `+` Add Context Popup (UIR-4.1, Screenshot #68, Blueprint §7.3).
 *
 * Implements:
 * - `Attachments` (upload files/folders)
 * - `Goal` toggle
 * - `Workflow ▸` selector
 * - `PLUGINS` quick toggles (`Documents`, `PDF`, `Presentations`, `Spreadsheets`)
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Button, Toggle } from "../../components/primitives";
import { ADD_CONTEXT_PLUGINS } from "../workspaceConfig";

export interface AddContextMenuProps {
  readonly isOpen: boolean;
  readonly goalModeEnabled: boolean;
  readonly enabledPluginIds: ReadonlySet<string>;
  readonly onPickAttachment: () => void;
  readonly onToggleGoalMode: (enabled: boolean) => void;
  readonly onSelectWorkflow: () => void;
  readonly onTogglePlugin: (pluginId: string) => void;
  readonly onClose: () => void;
}

export function AddContextMenu({
  isOpen,
  goalModeEnabled,
  enabledPluginIds,
  onPickAttachment,
  onToggleGoalMode,
  onSelectWorkflow,
  onTogglePlugin,
  onClose,
}: AddContextMenuProps): ReactElement | null {
  if (!isOpen) {
    return null;
  }

  return (
    <div
      role="dialog"
      aria-label="Add context menu"
      data-testid="composer-add-context-menu"
      className="ky-popover-surface"
      style={{
        bottom: "calc(100% + 8px)",
        insetInlineStart: 0,
        width: "272px",
        padding: "var(--space-2)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-2)",
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          onClose();
        }
      }}
    >
      <Button
        variant="ghost"
        size="sm"
        leadingIcon={<ChromeIcon name="attachment" size={14} />}
        style={{ width: "100%", justifyContent: "flex-start" }}
        onClick={() => {
          onPickAttachment();
          onClose();
        }}
      >
        Attachments
      </Button>

      <div style={{ paddingInline: "var(--space-2)" }}>
        <Toggle
          checked={goalModeEnabled}
          onChange={onToggleGoalMode}
          label="Goal"
          description="Autonomous goal completion mode"
        />
      </div>

      <Button
        variant="ghost"
        size="sm"
        leadingIcon={<ChromeIcon name="workflow" size={14} />}
        trailingIcon={<ChromeIcon name="chevron-right" size={14} />}
        style={{ width: "100%", justifyContent: "space-between" }}
        onClick={() => {
          onSelectWorkflow();
          onClose();
        }}
      >
        Workflow
      </Button>

      <hr
        role="separator"
        style={{
          border: "none",
          borderTop: "1px solid var(--border-subtle)",
          margin: "2px 0",
        }}
      />

      <div
        style={{
          paddingInline: "var(--space-2)",
          fontSize: "0.7rem",
          fontWeight: 600,
          color: "var(--text-muted)",
          letterSpacing: "0.04em",
        }}
      >
        PLUGINS
      </div>

      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "var(--space-2)",
          paddingInline: "var(--space-2)",
        }}
      >
        {ADD_CONTEXT_PLUGINS.map((plugin) => (
          <Toggle
            key={plugin.id}
            checked={enabledPluginIds.has(plugin.id)}
            onChange={() => onTogglePlugin(plugin.id)}
            label={plugin.label}
          />
        ))}
      </div>
    </div>
  );
}
