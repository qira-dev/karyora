/**
 * Composer `+` Add Context Popup — Calibrated 1:1 to ZCode Screenshot #68.
 *
 * Implements `ADD` (`Attachments`, `Goal`, `Workflow`) and `PLUGINS` quick toggles,
 * fully wired to live 13-locale i18next translations.
 */

import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();

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
        width: "284px",
        padding: "10px",
        display: "flex",
        flexDirection: "column",
        gap: "6px",
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          onClose();
        }
      }}
    >
      <div
        style={{
          paddingInline: "8px",
          fontSize: "0.68rem",
          fontWeight: 600,
          color: "var(--text-muted)",
          letterSpacing: "0.05em",
        }}
      >
        {t("ui.surfaces.addSection", { defaultValue: "ADD" })}
      </div>

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
        {t("ui.composer.attachments", { defaultValue: "Attachments" })}
      </Button>

      <div style={{ paddingInline: "8px", paddingBlock: "2px" }}>
        <Toggle
          checked={goalModeEnabled}
          onChange={onToggleGoalMode}
          label={t("ui.composer.goal", { defaultValue: "Goal" })}
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
        {t("ui.composer.workflow", { defaultValue: "Workflow" })}
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
          paddingInline: "8px",
          fontSize: "0.68rem",
          fontWeight: 600,
          color: "var(--text-muted)",
          letterSpacing: "0.05em",
        }}
      >
        {t("ui.surfaces.pluginsSection", { defaultValue: "PLUGINS" })}
      </div>

      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "6px",
          paddingInline: "8px",
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
