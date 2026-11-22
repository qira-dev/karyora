/**
 * Settings Section 15: Commands — Calibrated 1:1 to ZCode Screenshots #46–#48.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, Input } from "../../components/primitives";
import {
  CommandEditorForm,
  type CustomCommandRecord,
} from "./CommandEditorForm";

const INITIAL_COMMANDS: readonly CustomCommandRecord[] = [
  { id: "c-1", slashName: "/revise-agents-md", source: "plugin", description: "Update project instructions with learnings from this session", template: "Review session diffs and update AGENTS.md." },
  { id: "c-2", slashName: "/verify-build", source: "plugin", description: "Run full typecheck, unit tests, and security checks", template: "Verify workspace toolchain and setup hooks." },
  { id: "c-3", slashName: "/aside", source: "installed", description: "Answer a quick side question without losing context from the current task", template: "Answer concisely without altering task state." },
];

export interface CommandsSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function CommandsSettingsSection({
  onNotify,
}: CommandsSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [commands, setCommands] = useState<readonly CustomCommandRecord[]>(INITIAL_COMMANDS);
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);

  if (creating) {
    return (
      <div className="ky-settings-content" data-testid="settings-section-commands">
        <div className="ky-settings-breadcrumb">
          <button
            type="button"
            className="ky-settings-breadcrumb__link"
            onClick={() => setCreating(false)}
          >
            {t("ui.settings.sections.commands", { defaultValue: "Commands" })}
          </button>
          <span>›</span>
          <span>
            {t("ui.settings.capabilities.newCommand", { defaultValue: "New command" })}
          </span>
        </div>
        <CommandEditorForm
          onSave={(cmd) => {
            setCommands((prev) => [...prev, cmd]);
            setCreating(false);
            onNotify(`Command ${cmd.slashName} saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      </div>
    );
  }

  const q = search.trim().toLowerCase();
  const filtered = commands.filter(
    (c) => !q || c.slashName.toLowerCase().includes(q) || c.description.toLowerCase().includes(q),
  );
  const installed = filtered.filter((c) => c.source === "installed");
  const pluginCmds = filtered.filter((c) => c.source !== "installed");

  const renderRow = (cmd: CustomCommandRecord) => (
    <div key={cmd.id} className="ky-settings-list-row">
      <div className="ky-settings-list-row__main">
        <span className="ky-settings-list-row__icon">
          <ChromeIcon name="commands" size={16} />
        </span>
        <div className="ky-settings-list-row__copy">
          <span className="ky-settings-list-row__title" style={{ fontFamily: "var(--font-mono)" }}>
            {cmd.slashName}
          </span>
          <span className="ky-settings-list-row__desc">{cmd.description}</span>
        </div>
      </div>
      <div className="ky-settings-list-row__right">
        {cmd.source === "installed" && (
          <Button
            size="sm"
            variant="ghost"
            aria-label={`Delete ${cmd.slashName}`}
            onClick={() => setCommands((prev) => prev.filter((c) => c.id !== cmd.id))}
          >
            <ChromeIcon name="trash" size={14} />
          </Button>
        )}
      </div>
    </div>
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-commands">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <span className="ky-settings-pill-counter">
            <span>{t("ui.settings.sections.commands", { defaultValue: "Commands" })}</span>
            <strong>{commands.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search commands"
            placeholder={t("ui.settings.capabilities.searchCommands", {
              defaultValue: "Search commands...",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{installed.length}</span>
        </h3>
        <div style={{ display: "flex", gap: "8px" }}>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onNotify("Import command markdown ready.")}
          >
            {t("ui.settings.capabilities.import", { defaultValue: "Import" })}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setCreating(true)}>
            <ChromeIcon name="plus" size={14} />
            <span>
              {t("ui.settings.capabilities.newCommand", { defaultValue: "New command" })}
            </span>
          </Button>
        </div>
      </div>

      {installed.length === 0 ? (
        <div className="ky-settings-empty">
          <p className="ky-settings-empty__title">No commands installed</p>
          <p className="ky-settings-empty__desc">
            Create a command or import one from an external agent.
          </p>
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            <ChromeIcon name="plus" size={14} />
            <span>
              {t("ui.settings.capabilities.newCommand", { defaultValue: "New command" })}
            </span>
          </Button>
        </div>
      ) : (
        <div className="ky-settings-list">{installed.map(renderRow)}</div>
      )}

      {pluginCmds.length > 0 && (
        <>
          <div className="ky-settings-group-header">
            <h3 className="ky-settings-group-header__title">
              <span>Plugin Commands</span>
              <span className="ky-settings-group-header__count">{pluginCmds.length}</span>
            </h3>
          </div>
          <div className="ky-settings-list">{pluginCmds.map(renderRow)}</div>
        </>
      )}
    </div>
  );
}
