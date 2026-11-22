/**
 * Multi-State Agentic Composer — Calibrated 1:1 to ZCode Screenshots #68, #75, #82.
 *
 * Clean rounded-2xl charcoal input shell, circular `+` button, borderless mode/model/thinking
 * controls, and crisp circular monochrome Send (`↑`) / Stop (`■`) button.
 */

import { useState, type KeyboardEvent, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Badge } from "../../components/primitives";
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
  const { t } = useTranslation();
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
    <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
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
            {goalModeEnabled && <Badge tone="accent" label={t("ui.composer.goal", { defaultValue: "Goal" })} />}
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
          placeholder={
            isRunning
              ? t("ui.composer.placeholderQueue", {
                  defaultValue: "Keep typing to queue follow-up changes...",
                })
              : t("ui.composer.placeholder", {
                  defaultValue:
                    "Ask Karyora anything, @ to add context, / for commands or capabilities",
                })
          }
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

        {/* ZCode Bottom Composer Bar: Left (+ & Mode) | Right (Model, Thinking & Circular Send/Stop) */}
        <div className="ky-composer-toolbar">
          <div style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
            <button
              type="button"
              className="ky-composer-plus-btn"
              aria-label={t("ui.composer.addContext", { defaultValue: "Add context" })}
              title={t("ui.composer.addContext", { defaultValue: "Add context" })}
              onClick={() => setAddMenuOpen((prev) => !prev)}
            >
              <ChromeIcon name="plus" size={16} />
            </button>

            <div
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: "4px",
                color: mode === "auto" ? "#f59e0b" : "var(--text-secondary)",
              }}
            >
              <ChromeIcon name="shield-security" size={14} />
              <select
                aria-label="Session mode"
                className="ky-composer-inline-select"
                style={{
                  color: mode === "auto" ? "#f59e0b" : "var(--text-secondary)",
                }}
                value={mode}
                onChange={(e) => onChangeMode(e.target.value as ComposerSessionMode)}
              >
                <option value="default">
                  {t("ui.composer.modeDefault", { defaultValue: "Default" })}
                </option>
                <option value="plan">
                  {t("ui.composer.modePlan", { defaultValue: "Plan" })}
                </option>
                <option value="auto">
                  {t("ui.composer.modeAuto", { defaultValue: "Full access" })}
                </option>
              </select>
            </div>
          </div>

          <div style={{ display: "inline-flex", alignItems: "center", gap: "6px" }}>
            <select
              aria-label="Active model"
              className="ky-composer-inline-select"
              value={modelId}
              onChange={(e) => onChangeModel(e.target.value)}
            >
              {availableModels.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>

            <select
              aria-label="Thought level"
              className="ky-composer-inline-select"
              style={{ color: "var(--text-muted)" }}
              value={thoughtLevel}
              onChange={(e) =>
                onChangeThoughtLevel(e.target.value as ComposerThoughtLevel)
              }
            >
              <option value="low">
                {t("ui.composer.thinkingLow", { defaultValue: "Low" })}
              </option>
              <option value="medium">
                {t("ui.composer.thinkingMedium", { defaultValue: "Medium" })}
              </option>
              <option value="high">
                {t("ui.composer.thinkingHigh", { defaultValue: "High" })}
              </option>
            </select>

            {isRunning ? (
              <button
                type="button"
                className="ky-composer-send-btn"
                aria-label={t("ui.composer.stop", { defaultValue: "Stop" })}
                title={t("ui.composer.stop", { defaultValue: "Stop" })}
                onClick={onStopTurn}
              >
                <ChromeIcon name="stop" size={14} />
              </button>
            ) : (
              <button
                type="button"
                className="ky-composer-send-btn"
                disabled={!canSubmit}
                aria-label={t("ui.composer.send", { defaultValue: "Send" })}
                title={
                  isCoreOffline
                    ? "Core is offline — draft preserved safely"
                    : t("ui.composer.send", { defaultValue: "Send" })
                }
                onClick={onSubmitPrompt}
              >
                <ChromeIcon name="send" size={16} />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
