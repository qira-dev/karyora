/**
 * Settings Section 16: Hooks — Calibrated 1:1 to ZCode Screenshots #49–#53.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Input, Toggle } from "../../components/primitives";
import {
  HookEditorForm,
  type ConfiguredHookRecord,
} from "./HookEditorForm";

const INITIAL_HOOKS: readonly ConfiguredHookRecord[] = [
  {
    id: "hk-1",
    event: "PreToolUse",
    runner: "process",
    scope: "Project",
    matcher: "Write|Edit",
    command: "gitleaks",
    args: "detect --no-git",
    timeoutMs: 5000,
    enabled: true,
  },
];

export interface HooksSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function HooksSettingsSection({
  onNotify,
}: HooksSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [hooks, setHooks] = useState<readonly ConfiguredHookRecord[]>(INITIAL_HOOKS);
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);

  if (creating) {
    return (
      <div className="ky-settings-content" data-testid="settings-section-hooks">
        <div className="ky-settings-breadcrumb">
          <button
            type="button"
            className="ky-settings-breadcrumb__link"
            onClick={() => setCreating(false)}
          >
            {t("ui.settings.sections.hooks", { defaultValue: "Hooks" })}
          </button>
          <span>›</span>
          <span>{t("ui.settings.capabilities.newHook", { defaultValue: "New hook" })}</span>
        </div>
        <HookEditorForm
          onSave={(hk) => {
            setHooks((prev) => [...prev, hk]);
            setCreating(false);
            onNotify(`Hook on ${hk.event} saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      </div>
    );
  }

  const q = search.trim().toLowerCase();
  const filtered = hooks.filter(
    (h) =>
      !q ||
      h.event.toLowerCase().includes(q) ||
      h.command.toLowerCase().includes(q) ||
      h.matcher.toLowerCase().includes(q),
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-hooks">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <span className="ky-settings-pill-counter">
            <span>{t("ui.settings.sections.hooks", { defaultValue: "Hooks" })}</span>
            <strong>{hooks.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search hooks"
            placeholder={t("ui.settings.capabilities.searchHooks", {
              defaultValue: "Search hooks...",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{filtered.length}</span>
        </h3>
        <Button size="sm" variant="secondary" onClick={() => setCreating(true)}>
          <ChromeIcon name="plus" size={14} />
          <span>{t("ui.settings.capabilities.newHook", { defaultValue: "New hook" })}</span>
        </Button>
      </div>

      {filtered.length === 0 ? (
        <div className="ky-settings-empty">
          <p className="ky-settings-empty__title">No hooks installed</p>
          <p className="ky-settings-empty__desc">
            Create a hook to run commands during task lifecycle events.
          </p>
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            <ChromeIcon name="plus" size={14} />
            <span>{t("ui.settings.capabilities.newHook", { defaultValue: "New hook" })}</span>
          </Button>
        </div>
      ) : (
        <div className="ky-settings-list">
          {filtered.map((hk) => (
            <div key={hk.id} className="ky-settings-list-row">
              <div className="ky-settings-list-row__main">
                <span className="ky-settings-list-row__icon">
                  <ChromeIcon name="hooks" size={16} />
                </span>
                <div className="ky-settings-list-row__copy">
                  <span className="ky-settings-list-row__title">
                    <span>{hk.event}</span>
                    <Badge tone="neutral" label={hk.runner} />
                    <Badge tone="neutral" label={hk.scope} />
                  </span>
                  <span className="ky-settings-list-row__desc" style={{ fontFamily: "var(--font-mono)" }}>
                    {hk.matcher ? `[${hk.matcher}] ` : ""}
                    {hk.command} {hk.args}
                  </span>
                </div>
              </div>
              <div className="ky-settings-list-row__right">
                <Toggle
                  label=""
                  aria-label={`Enable hook ${hk.event}`}
                  checked={hk.enabled}
                  onChange={(enabled) =>
                    setHooks((prev) =>
                      prev.map((item) => (item.id === hk.id ? { ...item, enabled } : item)),
                    )
                  }
                />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
