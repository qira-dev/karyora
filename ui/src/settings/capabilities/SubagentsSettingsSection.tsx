/**
 * Settings Section 11: Subagents (UIR-8.2, Screenshots #29–#33, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Select } from "../../components/primitives";
import {
  SubagentEditorForm,
  type ConfiguredSubagentRecord,
} from "./SubagentEditorForm";

export interface SubagentsSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

const INITIAL_SUBAGENTS: readonly ConfiguredSubagentRecord[] = [
  {
    id: "sub-researcher",
    name: "Codebase Researcher",
    group: "installed",
    colorHex: "#6366f1",
    modelOverride: "inherit-default",
    description: "Read-only architecture & contract lookup subagent.",
    allowedTools: "read-only",
    scope: "Project",
    systemPrompt: "Inspect files without mutating workspace state.",
    injectAgentsMd: true,
  },
  {
    id: "sub-claude-sec",
    name: "Security Review Guard",
    group: "Claude Security",
    colorHex: "#f43f5e",
    modelOverride: "claude-sonnet",
    description: "Audits OWASP Top 10, secret hygiene, and sandbox rules.",
    allowedTools: "security-mcp",
    scope: "User",
    systemPrompt: "Audit diffs for security regressions.",
    injectAgentsMd: true,
  },
];

export function SubagentsSettingsSection({
  onNotify,
}: SubagentsSettingsSectionProps): ReactElement {
  const [scopeFilter, setScopeFilter] = useState<"All" | "User" | "Project">("All");
  const [subagents, setSubagents] = useState<readonly ConfiguredSubagentRecord[]>(
    INITIAL_SUBAGENTS,
  );
  const [creating, setCreating] = useState(false);

  const visible = subagents.filter(
    (s) => scopeFilter === "All" || s.scope === scopeFilter,
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-subagents">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <Select
            aria-label="Subagent scope filter"
            value={scopeFilter}
            options={[
              { value: "All", label: "All scopes" },
              { value: "User", label: "User scope" },
              { value: "Project", label: "Project scope" },
            ]}
            onChange={(e) => setScopeFilter(e.target.value as "All" | "User" | "Project")}
          />
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            + New subagent
          </Button>
        </div>

        {visible.map((sub) => (
          <div key={sub.id} className="ky-gov-perm-row">
            <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <span
                style={{
                  width: "10px",
                  height: "10px",
                  borderRadius: "999px",
                  backgroundColor: sub.colorHex,
                  display: "inline-block",
                }}
              />
              <div>
                <strong>{sub.name}</strong>{" "}
                <Badge tone="neutral" label={sub.group} />
                <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                  {sub.description}
                </div>
              </div>
            </div>
            <Select
              aria-label={`Model override for ${sub.name}`}
              value={sub.modelOverride}
              options={[
                { value: "inherit-default", label: "Inherit default" },
                { value: "claude-sonnet", label: "Claude Sonnet" },
                { value: "gpt-5", label: "GPT-5" },
                { value: "gemini-pro", label: "Gemini Pro" },
              ]}
              onChange={(e) =>
                setSubagents((prev) =>
                  prev.map((item) =>
                    item.id === sub.id ? { ...item, modelOverride: e.target.value } : item,
                  ),
                )
              }
            />
          </div>
        ))}
      </div>

      {creating && (
        <SubagentEditorForm
          onSave={(sub) => {
            setSubagents((prev) => [...prev, sub]);
            setCreating(false);
            onNotify(`Subagent "${sub.name}" saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}
