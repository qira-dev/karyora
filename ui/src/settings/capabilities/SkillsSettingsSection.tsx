/**
 * Settings Section 14: Skills (UIR-8.2, Screenshots #44–#45, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Input, Toggle } from "../../components/primitives";

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
  const [skills, setSkills] = useState<readonly SkillItem[]>(INITIAL_SKILLS);
  const [search, setSearch] = useState("");
  const [newSkillName, setNewSkillName] = useState("");
  const [creating, setCreating] = useState(false);

  const visible = skills.filter((s) =>
    s.name.toLowerCase().includes(search.trim().toLowerCase()) ||
    s.description.toLowerCase().includes(search.trim().toLowerCase()),
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-skills">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
            <h3 className="ky-settings-card__title">Installed Skills</h3>
            <Badge tone="info" label="357 catalog / 4 active" />
          </div>
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Input
              aria-label="Search skills"
              placeholder="Search skills..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Button size="sm" variant="secondary" onClick={() => onNotify("Import skill bundle (.md) ready.")}>
              … Import
            </Button>
            <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
              + New
            </Button>
          </div>
        </div>

        {creating && (
          <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "end" }}>
            <Input
              label="Skill slug"
              placeholder="e.g. api-contract-audit"
              value={newSkillName}
              onChange={(e) => setNewSkillName(e.target.value)}
            />
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
              Add
            </Button>
          </div>
        )}

        {visible.map((skill) => (
          <div key={skill.id} className="ky-gov-perm-row">
            <div>
              <strong>${skill.name}</strong>
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                {skill.description}
              </div>
            </div>
            <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
              <Toggle
                label={`Enable ${skill.name}`}
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
                onClick={() => setSkills((prev) => prev.filter((item) => item.id !== skill.id))}
              >
                Delete
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
