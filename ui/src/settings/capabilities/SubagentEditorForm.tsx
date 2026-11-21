/**
 * `New subagent` Editor Form (UIR-8.2, Screenshots #29–#33, E2E-18).
 *
 * Implements Name, 8 Color swatches, Model menu, Description, Allowed tools, Scope menu,
 * System prompt, and `Inject AGENTS.md` toggle.
 */

import { useState, type ReactElement } from "react";
import { Button, Input, Select, Textarea, Toggle } from "../../components/primitives";
import { SUBAGENT_COLOR_SWATCHES } from "../settingsConfig";

export interface ConfiguredSubagentRecord {
  readonly id: string;
  readonly name: string;
  readonly group: "installed" | "Claude Security";
  readonly colorHex: string;
  readonly modelOverride: string;
  readonly description: string;
  readonly allowedTools: string;
  readonly scope: "User" | "Project";
  readonly systemPrompt: string;
  readonly injectAgentsMd: boolean;
}

export interface SubagentEditorFormProps {
  readonly onSave: (subagent: ConfiguredSubagentRecord) => void;
  readonly onCancel: () => void;
}

export function SubagentEditorForm({
  onSave,
  onCancel,
}: SubagentEditorFormProps): ReactElement {
  const [name, setName] = useState("");
  const [colorHex, setColorHex] = useState(SUBAGENT_COLOR_SWATCHES[0].hex);
  const [modelOverride, setModelOverride] = useState("inherit-default");
  const [description, setDescription] = useState("");
  const [allowedTools, setAllowedTools] = useState("read-only");
  const [scope, setScope] = useState<"User" | "Project">("Project");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [injectAgentsMd, setInjectAgentsMd] = useState(true);

  return (
    <div className="ky-settings-card" data-testid="subagent-editor-form">
      <h4 className="ky-settings-card__title">New subagent</h4>
      <div className="ky-settings-grid-2">
        <Input
          label="Name"
          placeholder="e.g. Security Auditor"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Select
          label="Model override"
          value={modelOverride}
          options={[
            { value: "inherit-default", label: "Inherit default" },
            { value: "claude-sonnet", label: "Claude Sonnet" },
            { value: "gpt-5", label: "GPT-5" },
            { value: "gemini-pro", label: "Gemini Pro" },
          ]}
          onChange={(e) => setModelOverride(e.target.value)}
        />
      </div>

      <div>
        <span style={{ fontSize: "0.8rem", fontWeight: 500, display: "block", marginBottom: "6px" }}>
          Accent Color (8 swatches)
        </span>
        <div className="ky-swatch-row">
          {SUBAGENT_COLOR_SWATCHES.map((sw) => (
            <button
              key={sw.id}
              type="button"
              className="ky-swatch-btn"
              aria-label={`Select color ${sw.id}`}
              data-active={colorHex === sw.hex ? "true" : "false"}
              style={{ backgroundColor: sw.hex }}
              onClick={() => setColorHex(sw.hex)}
            />
          ))}
        </div>
      </div>

      <Input
        label="Description"
        placeholder="When should the orchestrator delegate to this subagent?"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
      />

      <div className="ky-settings-grid-2">
        <Select
          label="Allowed tools"
          value={allowedTools}
          options={[
            { value: "read-only", label: "Read-only research tools" },
            { value: "workspace-edit", label: "Workspace read & edit" },
            { value: "security-mcp", label: "Security MCP + read-only" },
          ]}
          onChange={(e) => setAllowedTools(e.target.value)}
        />
        <Select
          label="Scope"
          value={scope}
          options={[
            { value: "Project", label: "Project" },
            { value: "User", label: "User (Global)" },
          ]}
          onChange={(e) => setScope(e.target.value as "User" | "Project")}
        />
      </div>

      <Textarea
        label="System prompt"
        rows={4}
        placeholder="Define the subagent's exact responsibilities and constraints..."
        value={systemPrompt}
        onChange={(e) => setSystemPrompt(e.target.value)}
      />

      <Toggle
        label="Inject AGENTS.md"
        description="Include workspace AGENTS.md governance rules in this subagent's prompt"
        checked={injectAgentsMd}
        onChange={setInjectAgentsMd}
      />

      <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          size="sm"
          variant="primary"
          disabled={!name.trim()}
          onClick={() =>
            onSave({
              id: `sub-${Date.now()}`,
              name: name.trim(),
              group: "installed",
              colorHex,
              modelOverride,
              description: description.trim() || "Custom workspace subagent",
              allowedTools,
              scope,
              systemPrompt,
              injectAgentsMd,
            })
          }
        >
          Save subagent
        </Button>
      </div>
    </div>
  );
}
