/**
 * `New command` Editor Form (UIR-8.2, Screenshot #47, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Button, Input, Textarea } from "../../components/primitives";

export interface CustomCommandRecord {
  readonly id: string;
  readonly slashName: string;
  readonly source: "installed" | "plugin";
  readonly description: string;
  readonly template: string;
}

export interface CommandEditorFormProps {
  readonly onSave: (cmd: CustomCommandRecord) => void;
  readonly onCancel: () => void;
}

export function CommandEditorForm({
  onSave,
  onCancel,
}: CommandEditorFormProps): ReactElement {
  const [slashName, setSlashName] = useState("/verify-release");
  const [description, setDescription] = useState("");
  const [template, setTemplate] = useState("");

  return (
    <div className="ky-settings-card" data-testid="command-editor-form">
      <h4 className="ky-settings-card__title">New command</h4>
      <div className="ky-settings-grid-2">
        <Input
          label="Slash command"
          value={slashName}
          onChange={(e) => setSlashName(e.target.value)}
        />
        <Input
          label="Description"
          placeholder="What does this command trigger?"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>
      <Textarea
        label="Prompt template"
        rows={4}
        placeholder="Write the reusable slash command instructions..."
        value={template}
        onChange={(e) => setTemplate(e.target.value)}
      />
      <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          size="sm"
          variant="primary"
          disabled={!slashName.trim()}
          onClick={() =>
            onSave({
              id: `cmd-${Date.now()}`,
              slashName: slashName.startsWith("/") ? slashName.trim() : `/${slashName.trim()}`,
              source: "installed",
              description: description.trim() || "Custom slash command",
              template,
            })
          }
        >
          Save command
        </Button>
      </div>
    </div>
  );
}
