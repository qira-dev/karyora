/**
 * Settings Section 9: Behavior and Tools (UIR-8.2, Screenshots #24–#25, Blueprint §13.2 (9)).
 */

import { useState, type ReactElement } from "react";
import { Select, Toggle } from "../../components/primitives";

export function BehaviorAndToolsSettingsSection(): ReactElement {
  const [toolSearch, setToolSearch] = useState("auto");
  const [enhancedFindGrep, setEnhancedFindGrep] = useState(true);
  const [interactionBehavior, setInteractionBehavior] = useState("queue");
  const [autoContinue, setAutoContinue] = useState(false);
  const [pluginRecs, setPluginRecs] = useState(true);
  const [dynamicWorkflows, setDynamicWorkflows] = useState(true);
  const [keepCompleteIo, setKeepCompleteIo] = useState(true);
  const [showReasoning, setShowReasoning] = useState(true);
  const [showTodos, setShowTodos] = useState(true);

  return (
    <div className="ky-settings-content" data-testid="settings-section-behavior-and-tools">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Tool Discovery & Interaction Behavior</h3>
        <div className="ky-settings-grid-2">
          <Select
            label="Tool search mode"
            value={toolSearch}
            options={[
              { value: "off", label: "Off" },
              { value: "auto", label: "Auto (Recommended)" },
              { value: "local", label: "Local index only" },
            ]}
            onChange={(e) => setToolSearch(e.target.value)}
          />
          <Select
            label="Interaction behavior while running"
            value={interactionBehavior}
            options={[
              { value: "queue", label: "Queue follow-up prompts" },
              { value: "guide", label: "Guide active turn immediately" },
            ]}
            onChange={(e) => setInteractionBehavior(e.target.value)}
          />
        </div>
        <Toggle label="Enhanced Find & Grep tools" checked={enhancedFindGrep} onChange={setEnhancedFindGrep} />
        <Toggle label="Auto-continue multi-step plans" checked={autoContinue} onChange={setAutoContinue} />
        <Toggle label="Contextual plugin recommendations" checked={pluginRecs} onChange={setPluginRecs} />
        <Toggle label="Dynamic workflows" checked={dynamicWorkflows} onChange={setDynamicWorkflows} />
        <Toggle label="Keep complete model I/O in local history" checked={keepCompleteIo} onChange={setKeepCompleteIo} />
        <Toggle label="Show reasoning (Thinking rows #79)" checked={showReasoning} onChange={setShowReasoning} />
        <Toggle label="Show progress checklist & todos (#70–#74)" checked={showTodos} onChange={setShowTodos} />
      </div>
    </div>
  );
}
