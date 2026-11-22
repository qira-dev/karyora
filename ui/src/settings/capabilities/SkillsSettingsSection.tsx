/**
 * Settings Section 14: Skills — Calibrated 1:1 to ZCode Screenshots #44–#45.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, Input, Toggle } from "../../components/primitives";

interface SkillItem {
  readonly id: string;
  readonly name: string;
  readonly description: string;
  readonly enabled: boolean;
}

const INITIAL_SKILLS: readonly SkillItem[] = [
  { id: "sk-sec", name: "security-review", description: "OWASP & secret hygiene verification checklist", enabled: true },
  { id: "sk-tdd", name: "tdd-workflow", description: "Test-first implementation and regression verification", enabled: true },
  { id: "sk-a11y", name: "frontend-a11y", description: "WCAG 2.2 AA semantics, focus traps & contrast", enabled: true },
  { id: "sk-go", name: "golang-patterns", description: "Idiomatic Go concurrency, error wrapping & race checks", enabled: true },
];

export interface SkillsSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function SkillsSettingsSection({
  onNotify,
}: SkillsSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [skills, setSkills] = useState<readonly SkillItem[]>(INITIAL_SKILLS);
  const [search, setSearch] = useState("");
  const [newSkillName, setNewSkillName] = useState("");
  const [creating, setCreating] = useState(false);

  const q = search.trim().toLowerCase();
  const visible = skills.filter(
    (s) => !q || s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q),
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-skills">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <span className="ky-settings-pill-counter">
            <span>{t("ui.settings.sections.skills", { defaultValue: "Skills" })}</span>
            <strong>{skills.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search skills"
            placeholder={t("ui.settings.capabilities.searchSkills", {
              defaultValue: "Search skills...",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{visible.length}</span>
        </h3>
        <div style={{ display: "flex", gap: "8px" }}>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onNotify("Import skill bundle (.md) ready.")}
          >
            {t("ui.settings.capabilities.import", { defaultValue: "Import" })}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setCreating((v) => !v)}>
            <ChromeIcon name="plus" size={14} />
          </Button>
        </div>
      </div>

      {creating && (
        <div className="ky-settings-card" style={{ flexDirection: "row", alignItems: "end", flexWrap: "wrap" }}>
          <div style={{ flex: 1, minWidth: "200px" }}>
            <Input
              label="Skill slug"
              placeholder="e.g. api-contract-audit"
              value={newSkillName}
              onChange={(e) => setNewSkillName(e.target.value)}
            />
          </div>
          <Button
            size="sm"
            variant="primary"
            disabled={!newSkillName.trim()}
            onClick={() => {
              setSkills((prev) => [
                {
                  id: `sk-${Date.now()}`,
                  name: newSkillName.trim(),
                  description: "Custom workspace skill",
                  enabled: true,
                },
                ...prev,
              ]);
              setNewSkillName("");
              setCreating(false);
              onNotify("Skill created.");
            }}
          >
            {t("ui.settings.capabilities.save", { defaultValue: "Save" })}
          </Button>
        </div>
      )}

      <div className="ky-settings-list">
        {visible.map((skill) => (
          <div key={skill.id} className="ky-settings-list-row">
            <div className="ky-settings-list-row__main">
              <span className="ky-settings-list-row__icon">
                <ChromeIcon name="skills" size={16} />
              </span>
              <div className="ky-settings-list-row__copy">
                <span className="ky-settings-list-row__title">{skill.name}</span>
                <span className="ky-settings-list-row__desc">{skill.description}</span>
              </div>
            </div>
            <div className="ky-settings-list-row__right">
              <Toggle
                label=""
                aria-label={`Enable ${skill.name}`}
                checked={skill.enabled}
                onChange={(enabled) =>
                  setSkills((prev) =>
                    prev.map((item) => (item.id === skill.id ? { ...item, enabled } : item)),
                  )
                }
              />
              <Button
                size="sm"
                variant="ghost"
                aria-label={`Delete ${skill.name}`}
                onClick={() => setSkills((prev) => prev.filter((item) => item.id !== skill.id))}
              >
                <ChromeIcon name="trash" size={14} />
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
