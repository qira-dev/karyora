/**
 * Main Workspace Greeting & 4 Starter Quick Chips (UIR-4.1, Screenshot #68, Blueprint §7.2).
 */

import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon, type ChromeIconName } from "../components/icons";
import {
  resolveTimeOfDayGreeting,
  STARTER_QUICK_CHIPS,
  type StarterQuickChip,
} from "./workspaceConfig";

export interface GreetingEmptyStateProps {
  readonly currentHour24?: number;
  readonly onSelectQuickChip: (chip: StarterQuickChip) => void;
}

const CHIP_ICON_MAP: Readonly<Record<string, ChromeIconName>> = {
  "quick-start": "sparkles",
  "create-presentation": "file-text",
  "build-website": "code",
  "scheduled-tasks": "automations",
};

const CHIP_I18N_KEY_MAP: Readonly<Record<string, string>> = {
  "quick-start": "ui.greeting.chipQuickStart",
  "create-presentation": "ui.greeting.chipPresentation",
  "build-website": "ui.greeting.chipWebsite",
  "scheduled-tasks": "ui.greeting.chipScheduled",
};

export function GreetingEmptyState({
  currentHour24 = new Date().getHours(),
  onSelectQuickChip,
}: GreetingEmptyStateProps): ReactElement {
  const { t } = useTranslation();
  const fallbackGreeting = resolveTimeOfDayGreeting(currentHour24);
  const greetingKey =
    currentHour24 < 12
      ? "ui.greeting.morning"
      : currentHour24 < 18
        ? "ui.greeting.afternoon"
        : "ui.greeting.evening";

  return (
    <section
      aria-label="New task greeting"
      data-testid="workspace-greeting-empty-state"
      className="ky-greeting-hero"
    >
      <h1
        style={{
          margin: 0,
          fontSize: "1.75rem",
          fontWeight: 500,
          letterSpacing: "-0.02em",
          color: "var(--text-primary)",
        }}
      >
        {t(greetingKey, { defaultValue: fallbackGreeting })}
      </h1>
      <p style={{ margin: 0, color: "var(--text-muted)", fontSize: "0.875rem" }}>
        {t("ui.greeting.subtitle", {
          defaultValue: "What would you like to build, inspect, or automate today?",
        })}
      </p>

      <div className="ky-quick-chips-row">
        {STARTER_QUICK_CHIPS.map((chip, idx) => (
          <div
            key={chip.id}
            className="draft-prompt-card-waterfall"
            style={{ ["--card-index" as string]: idx }}
          >
            <button
              type="button"
              className="ky-quick-chip-btn"
              onClick={() => onSelectQuickChip(chip)}
            >
              <ChromeIcon name={CHIP_ICON_MAP[chip.id] ?? "sparkles"} size={14} />
              <span>
                {t(CHIP_I18N_KEY_MAP[chip.id] ?? "ui.greeting.chipQuickStart", {
                  defaultValue: chip.label,
                })}
              </span>
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
