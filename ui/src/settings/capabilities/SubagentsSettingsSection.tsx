/**
 * Settings Section 11: Subagents — Calibrated 1:1 to ZCode Screenshots #29–#33.
 *
 * Scope filter + counter pill + search toolbar, grouped `Installed` & plugin lists
 * with per-subagent model override selector, and inline `Subagents > New subagent` view.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Input, Select } from "../../components/primitives";
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
    name: "codebase-researcher",
    group: "installed",
    colorHex: "#38bdf8",
    modelOverride: "inherit-default",
    description: "Read-only architecture & contract lookup subagent.",
    allowedTools: "read-only",
    scope: "Project",
    systemPrompt: "Inspect files without mutating workspace state.",
    injectAgentsMd: true,
  },
  {
    id: "sub-claude-sec",
    name: "security-reviewer",
    group: "Claude Security",
    colorHex: "#f43f5e",
    modelOverride: "inherit-default",
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
  const { t } = useTranslation();
  const [scopeFilter, setScopeFilter] = useState<"All" | "User" | "Project">("All");
  const [search, setSearch] = useState("");
  const [subagents, setSubagents] = useState<readonly ConfiguredSubagentRecord[]>(
    INITIAL_SUBAGENTS,
  );
  const [creating, setCreating] = useState(false);

  if (creating) {
    return (
      <div className="ky-settings-content" data-testid="settings-section-subagents">
        <div className="ky-settings-breadcrumb">
          <button
            type="button"
            className="ky-settings-breadcrumb__link"
            onClick={() => setCreating(false)}
          >
            {t("ui.settings.sections.subagents", { defaultValue: "Subagents" })}
          </button>
          <span>›</span>
          <span>
            {t("ui.settings.capabilities.newSubagent", { defaultValue: "New subagent" })}
          </span>
        </div>
        <SubagentEditorForm
          onSave={(sub) => {
            setSubagents((prev) => [...prev, sub]);
            setCreating(false);
            onNotify(`Subagent "${sub.name}" saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      </div>
    );
  }

  const visible = subagents.filter((s) => {
    if (scopeFilter !== "All" && s.scope !== scopeFilter) return false;
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q);
  });

  const installedItems = visible.filter((s) => s.group === "installed");
  const pluginItems = visible.filter((s) => s.group !== "installed");

  const renderRow = (sub: ConfiguredSubagentRecord) => (
    <div key={sub.id} className="ky-settings-list-row">
      <div className="ky-settings-list-row__main">
        <span className="ky-settings-list-row__icon">
          <span
            style={{
              width: "10px",
              height: "10px",
              borderRadius: "999px",
              backgroundColor: sub.colorHex,
              display: "inline-block",
            }}
          />
        </span>
        <div className="ky-settings-list-row__copy">
          <span className="ky-settings-list-row__title">
            <span>{sub.name}</span>
            <Badge tone="neutral" label={sub.scope} />
          </span>
          <span className="ky-settings-list-row__desc">{sub.description}</span>
        </div>
      </div>
      <div className="ky-settings-list-row__right">
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
    </div>
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-subagents">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <Select
            aria-label="Subagent scope filter"
            value={scopeFilter}
            options={[
              { value: "All", label: "This computer · All" },
              { value: "User", label: "This computer · User" },
              { value: "Project", label: "This computer · Project" },
            ]}
            onChange={(e) => setScopeFilter(e.target.value as "All" | "User" | "Project")}
          />
          <span className="ky-settings-pill-counter">
            <span>{t("ui.settings.sections.subagents", { defaultValue: "Subagents" })}</span>
            <strong>{subagents.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search subagents"
            placeholder={t("ui.settings.capabilities.searchSubagents", {
              defaultValue: "Search subagents...",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{installedItems.length}</span>
        </h3>
        <Button size="sm" variant="secondary" onClick={() => setCreating(true)}>
          <ChromeIcon name="plus" size={14} />
          <span>{t("ui.settings.capabilities.newSubagent", { defaultValue: "New subagent" })}</span>
        </Button>
      </div>

      {installedItems.length === 0 ? (
        <div className="ky-settings-empty">
          <p className="ky-settings-empty__title">No subagents found</p>
          <p className="ky-settings-empty__desc">
            Fill in the subagent name, tools, and system prompt, then save to return to the list.
          </p>
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            <ChromeIcon name="plus" size={14} />
            <span>{t("ui.settings.capabilities.newSubagent", { defaultValue: "New subagent" })}</span>
          </Button>
        </div>
      ) : (
        <div className="ky-settings-list">{installedItems.map(renderRow)}</div>
      )}

      {pluginItems.length > 0 && (
        <>
          <div className="ky-settings-group-header">
            <h3 className="ky-settings-group-header__title">
              <span>Built-in & Plugin Subagents</span>
              <span className="ky-settings-group-header__count">{pluginItems.length}</span>
            </h3>
          </div>
          <div className="ky-settings-list">{pluginItems.map(renderRow)}</div>
        </>
      )}
    </div>
  );
}
