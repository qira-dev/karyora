/**
 * Multi-State Agentic Composer (UIR-4.1, Screenshots #68, #70–#85, Blueprint §7.3, E2E-03).
 *
 * Implements:
 * - Context chips row (`@`, `/`, `#`, `$`, `+` attachments)
 * - Auto-expanding prompt input (`Enter` send, `Shift+Enter` newline)
 * - `+` Add context menu (`Attachments`, `Goal`, `Workflow`, `PLUGINS`)
 * - Inline trigger popovers (`@`, `/`, `#`, `$`)
 * - Mode (`Default` | `Plan` | `Auto` `Ctrl+Shift+M`), Model (`Ctrl+M`), Thought Level (`Ctrl+T`)
 * - Send (`↑`) / Stop (`■`) button and visible `SubmissionQueueDrawer` (`Queue (N)`)
 */

import { useState, type KeyboardEvent, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Select } from "../../components/primitives";
import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import type { ComposerContextChip, SessionQueueSnapshot } from "../state";
import {
  COMPOSER_SESSION_MODES,
  COMPOSER_THOUGHT_LEVELS,
  cycleValue,
  detectComposerTrigger,
  type ComposerSessionMode,
  type ComposerThoughtLevel,
} from "../workspaceConfig";
import { AddContextMenu } from "./AddContextMenu";
import { SubmissionQueueDrawer } from "./SubmissionQueueDrawer";
import {
  TriggerPopovers,
  type TriggerSuggestionItem,
} from "./TriggerPopovers";

export interface AgenticComposerProps {
  readonly promptText: string;
  readonly chips: readonly ComposerContextChip[];
  readonly mode: ComposerSessionMode;
  readonly modelId: string;
  readonly availableModels: readonly string[];
  readonly thoughtLevel: ComposerThoughtLevel;
  readonly goalModeEnabled: boolean;
  readonly enabledPluginIds: ReadonlySet<string>;
  readonly isRunning: boolean;
  readonly isCoreOffline: boolean;
  readonly codeTheme: LightCodeTheme | DarkCodeTheme;
  readonly queueSnapshot: SessionQueueSnapshot;
  readonly triggerSuggestions: readonly TriggerSuggestionItem[];
  readonly onPromptChange: (nextText: string) => void;
  readonly onAddChip: (chip: ComposerContextChip) => void;
  readonly onRemoveChip: (chipId: string) => void;
  readonly onChangeMode: (mode: ComposerSessionMode) => void;
  readonly onChangeModel: (modelId: string) => void;
  readonly onChangeThoughtLevel: (level: ComposerThoughtLevel) => void;
  readonly onToggleGoalMode: (enabled: boolean) => void;
  readonly onTogglePlugin: (pluginId: string) => void;
  readonly onPickAttachment: () => void;
  readonly onOpenWorkflowPicker: () => void;
  readonly onSubmitPrompt: () => void;
  readonly onStopTurn: () => void;
  readonly onQueueReorder: (fromIndex: number, toIndex: number) => void;
  readonly onQueueEdit: (queueItemId: string, nextPromptText: string) => void;
  readonly onQueueSendNow: (queueItemId: string) => void;
  readonly onQueueRemove: (queueItemId: string) => void;
}

export function AgenticComposer({
  promptText,
  chips,
  mode,
  modelId,
  availableModels,
  thoughtLevel,
  goalModeEnabled,
  enabledPluginIds,
  isRunning,
  isCoreOffline,
  codeTheme,
  queueSnapshot,
  triggerSuggestions,
  onPromptChange,
  onAddChip,
  onRemoveChip,
  onChangeMode,
  onChangeModel,
  onChangeThoughtLevel,
  onToggleGoalMode,
  onTogglePlugin,
  onPickAttachment,
  onOpenWorkflowPicker,
  onSubmitPrompt,
  onStopTurn,
  onQueueReorder,
  onQueueEdit,
  onQueueSendNow,
  onQueueRemove,
}: AgenticComposerProps): ReactElement {
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [caretPos, setCaretPos] = useState(promptText.length);
  const [dismissedTriggerAt, setDismissedTriggerAt] = useState<number | null>(null);

  const detected = detectComposerTrigger(promptText, caretPos);
  const activeTrigger =
    detected && detected.startIndex !== dismissedTriggerAt ? detected : null;

  const canSubmit =
    !isCoreOffline && (promptText.trim().length > 0 || chips.length > 0);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.shiftKey && e.key.toLowerCase() === "m") {
      e.preventDefault();
      onChangeMode(cycleValue(COMPOSER_SESSION_MODES, mode));
      return;
    }
    if (mod && !e.shiftKey && e.key.toLowerCase() === "t") {
      e.preventDefault();
      onChangeThoughtLevel(cycleValue(COMPOSER_THOUGHT_LEVELS, thoughtLevel));
      return;
    }
    if (e.key === "Escape" && activeTrigger) {
      e.preventDefault();
      setDismissedTriggerAt(activeTrigger.startIndex);
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (canSubmit) {
        onSubmitPrompt();
      }
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
      <SubmissionQueueDrawer
        snapshot={queueSnapshot}
        onReorder={onQueueReorder}
        onEditPrompt={onQueueEdit}
        onSendNow={onQueueSendNow}
        onRemoveItem={onQueueRemove}
      />

      <div className="ky-composer-shell" data-testid="agentic-composer">
        <AddContextMenu
          isOpen={addMenuOpen}
          goalModeEnabled={goalModeEnabled}
          enabledPluginIds={enabledPluginIds}
          onPickAttachment={onPickAttachment}
          onToggleGoalMode={onToggleGoalMode}
          onSelectWorkflow={onOpenWorkflowPicker}
          onTogglePlugin={onTogglePlugin}
          onClose={() => setAddMenuOpen(false)}
        />

        <TriggerPopovers
          activeTrigger={activeTrigger?.trigger ?? null}
          query={activeTrigger?.query ?? ""}
          codeTheme={codeTheme}
          suggestions={triggerSuggestions}
          onDismiss={() => {
            if (activeTrigger) {
              setDismissedTriggerAt(activeTrigger.startIndex);
            }
          }}
          onSelectSuggestion={(item) => {
            if (activeTrigger) {
              const updatedText =
                promptText.slice(0, activeTrigger.startIndex) +
                promptText.slice(activeTrigger.endIndex);
              onPromptChange(updatedText.trimStart());
            }
            onAddChip({
              chipId: `chip-${Date.now()}-${item.id}`,
              trigger: item.trigger,
              label: `${item.trigger}${item.label}`,
              targetIdOrPath: item.filePathForIcon ?? item.id,
            });
            setDismissedTriggerAt(null);
          }}
        />

        {(chips.length > 0 || goalModeEnabled) && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: "6px" }}>
            {goalModeEnabled && <Badge tone="accent" label="Goal Mode" />}
            {chips.map((chip) => (
              <span
                key={chip.chipId}
                className="ky-badge"
                style={{
                  backgroundColor: "var(--bg-surface-2)",
                  color: "var(--text-primary)",
                }}
              >
                <span>{chip.label}</span>
                <button
                  type="button"
                  aria-label={`Remove ${chip.label}`}
                  onClick={() => onRemoveChip(chip.chipId)}
                  style={{
                    border: "none",
                    background: "transparent",
                    color: "var(--text-muted)",
                    cursor: "pointer",
                    padding: 0,
                  }}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        )}

        <textarea
          aria-label="Prompt composer"
          className="ky-composer-textarea"
          placeholder="Ask anything... Use @ for files, / for commands, # for sessions, $ for skills"
          value={promptText}
          onChange={(e) => {
            setDismissedTriggerAt(null);
            setCaretPos(e.target.selectionStart ?? e.target.value.length);
            onPromptChange(e.target.value);
          }}
          onKeyUp={(e) =>
            setCaretPos(e.currentTarget.selectionStart ?? promptText.length)
          }
          onKeyDown={handleKeyDown}
        />

        <div className="ky-composer-toolbar">
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "var(--space-2)" }}>
            <Button
              variant="ghost"
              size="sm"
              aria-label="Add context (+)"
              title="Add context, Goal, Workflow, or Plugins"
              selected={addMenuOpen}
              onClick={() => setAddMenuOpen((prev) => !prev)}
            >
              <ChromeIcon name="plus" size={16} />
            </Button>

            <div style={{ width: "110px" }}>
              <Select
                aria-label="Session mode"
                value={mode}
                onChange={(e) => onChangeMode(e.target.value as ComposerSessionMode)}
                options={[
                  { value: "default", label: "Default" },
                  { value: "plan", label: "Plan" },
                  { value: "auto", label: "Auto" },
                ]}
              />
            </div>

            <div style={{ width: "170px" }}>
              <Select
                aria-label="Active model"
                value={modelId}
                onChange={(e) => onChangeModel(e.target.value)}
                options={availableModels.map((m) => ({ value: m, label: m }))}
              />
            </div>

            <div style={{ width: "120px" }}>
              <Select
                aria-label="Thought level"
                value={thoughtLevel}
                onChange={(e) =>
                  onChangeThoughtLevel(e.target.value as ComposerThoughtLevel)
                }
                options={[
                  { value: "low", label: "Thought: Low" },
                  { value: "medium", label: "Thought: Med" },
                  { value: "high", label: "Thought: High" },
                ]}
              />
            </div>
          </div>

          <div style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)" }}>
            {isRunning && (
              <Button
                variant="danger"
                size="sm"
                aria-label="Stop running turn"
                leadingIcon={<ChromeIcon name="stop" size={14} />}
                onClick={onStopTurn}
              >
                Stop
              </Button>
            )}
            <Button
              variant="primary"
              size="sm"
              disabled={!canSubmit}
              disabledReason={
                isCoreOffline
                  ? "Core is offline — draft preserved safely"
                  : "Enter a prompt or attach context"
              }
              leadingIcon={<ChromeIcon name="send" size={14} />}
              onClick={onSubmitPrompt}
            >
              {isRunning ? "Queue" : "Send"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
