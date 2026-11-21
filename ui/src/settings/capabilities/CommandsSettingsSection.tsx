/**
 * Settings Section 15: Commands (UIR-8.2, Screenshots #46–#48, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../../components/primitives";
import {
  CommandEditorForm,
  type CustomCommandRecord,
} from "./CommandEditorForm";

const INITIAL_COMMANDS: readonly CustomCommandRecord[] = [
  { id: "c-1", slashName: "/revise-claude-md", source: "plugin", description: "Update project instructions after session learnings", template: "Review session diffs and update AGENTS.md." },
  { id: "c-2", slashName: "/maker-setup", source: "plugin", description: "Scaffold project configuration & CI checks", template: "Verify workspace toolchain and setup hooks." },
  { id: "c-3", slashName: "/aside", source: "installed", description: "Ask a side question without derailing the active task", template: "Answer concisely without altering task state." },
];

export interface CommandsSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function CommandsSettingsSection({
  onNotify,
}: CommandsSettingsSectionProps): ReactElement {
  const [commands, setCommands] = useState<readonly CustomCommandRecord[]>(INITIAL_COMMANDS);
  const [creating, setCreating] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-commands">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <h3 className="ky-settings-card__title">
            Installed & Plugin Commands ({commands.length})
          </h3>
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Button size="sm" variant="secondary" onClick={() => onNotify("Import command markdown ready.")}>
              … Import
            </Button>
            <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
              + New command
            </Button>
          </div>
        </div>

        {commands.map((cmd) => (
          <div key={cmd.id} className="ky-gov-perm-row">
            <div>
              <strong style={{ fontFamily: "var(--font-mono)" }}>{cmd.slashName}</strong>{" "}
              <Badge tone="neutral" label={cmd.source} />
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                {cmd.description}
              </div>
            </div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setCommands((prev) => prev.filter((c) => c.id !== cmd.id))}
            >
              Delete
            </Button>
          </div>
        ))}
      </div>

      {creating && (
        <CommandEditorForm
          onSave={(cmd) => {
            setCommands((prev) => [...prev, cmd]);
            setCreating(false);
            onNotify(`Command ${cmd.slashName} saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}
