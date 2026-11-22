/**
 * Settings Section 9: Behavior and Tools — Calibrated 1:1 to ZCode Screenshots #24–#25.
 *
 * Exact 9 horizontal ZCode setting rows inside `.ky-setting-group` cards with zero
 * spec or screenshot reference clutter.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Select, Toggle } from "../../components/primitives";

export function BehaviorAndToolsSettingsSection(): ReactElement {
  const { t } = useTranslation();
  const [toolSearch, setToolSearch] = useState("auto");
  const [enhancedFindGrep, setEnhancedFindGrep] = useState(true);
  const [interactionBehavior, setInteractionBehavior] = useState("queue");
  const [autoContinue, setAutoContinue] = useState(false);
  const [pluginRecs, setPluginRecs] = useState(true);
  const [dynamicWorkflows, setDynamicWorkflows] = useState("on-command");
  const [keepCompleteIo, setKeepCompleteIo] = useState(true);
  const [showReasoning, setShowReasoning] = useState(true);
  const [showTodos, setShowTodos] = useState(true);

  return (
    <div className="ky-settings-content" data-testid="settings-section-behavior-and-tools">
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.toolSearch", { defaultValue: "Tool search" })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.toolSearchDesc", {
                defaultValue:
                  "Auto selects native or local search for the model. Local loads tools on demand. Off sends the full tool catalog.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Tool search"
              value={toolSearch}
              options={[
                { value: "off", label: "Off" },
                { value: "auto", label: "Auto" },
                { value: "local", label: "Local" },
              ]}
              onChange={(e) => setToolSearch(e.target.value)}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.enhancedFind", {
                defaultValue: "Enhanced Find and Grep",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.enhancedFindDesc", {
                defaultValue:
                  "Use enhanced Find and Grep in new sessions and sessions restored after restart.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Enhanced Find and Grep"
              checked={enhancedFindGrep}
              onChange={setEnhancedFindGrep}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.interactionBehavior", {
                defaultValue: "Interaction behavior",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.interactionBehaviorDesc", {
                defaultValue:
                  "While Karyora is running, add follow-up actions to the queue or guide them after the next tool call.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Interaction behavior"
              value={interactionBehavior}
              options={[
                { value: "queue", label: "Queue" },
                { value: "guide", label: "Guide" },
              ]}
              onChange={(e) => setInteractionBehavior(e.target.value)}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.autoContinue", {
                defaultValue: "Automatically continue questions",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.autoContinueDesc", {
                defaultValue:
                  "When enabled, agent questions automatically continue after 5 minutes without an answer.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Automatically continue questions"
              checked={autoContinue}
              onChange={setAutoContinue}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.pluginRecs", {
                defaultValue: "Automatic plugin recommendations",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.pluginRecsDesc", {
                defaultValue:
                  "Let the agent find and recommend plugins in conversations. Installation always requires confirmation.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Automatic plugin recommendations"
              checked={pluginRecs}
              onChange={setPluginRecs}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.dynamicWorkflows", {
                defaultValue: "Dynamic workflows",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.dynamicWorkflowsDesc", {
                defaultValue:
                  "Let the model orchestrate parallel subagents on complex tasks.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Dynamic workflows"
              value={dynamicWorkflows}
              options={[
                { value: "on-command", label: "On command" },
                { value: "auto", label: "Automatic" },
                { value: "off", label: "Off" },
              ]}
              onChange={(e) => setDynamicWorkflows(e.target.value)}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.keepCompleteIo", {
                defaultValue: "Keep complete model I/O",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.keepCompleteIoDesc", {
                defaultValue:
                  "Keep complete model requests and responses without compression or automatic deletion.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Keep complete model I/O"
              checked={keepCompleteIo}
              onChange={setKeepCompleteIo}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.showReasoning", {
                defaultValue: "Show reasoning",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.showReasoningDesc", {
                defaultValue: "Show full reasoning inside the message stream.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Show reasoning"
              checked={showReasoning}
              onChange={setShowReasoning}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.behavior.showTodos", { defaultValue: "Show todos" })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.behavior.showTodosDesc", {
                defaultValue:
                  "Show progress checklists and Todo cards inside the message stream.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Show todos"
              checked={showTodos}
              onChange={setShowTodos}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
