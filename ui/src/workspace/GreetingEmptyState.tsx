/**
 * Main Workspace Greeting & 4 Starter Quick Chips (UIR-4.1, Screenshot #68, Blueprint §7.2).
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Button } from "../components/primitives";
import {
  resolveTimeOfDayGreeting,
  STARTER_QUICK_CHIPS,
  type StarterQuickChip,
} from "./workspaceConfig";

export interface GreetingEmptyStateProps {
  readonly currentHour24?: number;
  readonly onSelectQuickChip: (chip: StarterQuickChip) => void;
}

export function GreetingEmptyState({
  currentHour24 = new Date().getHours(),
  onSelectQuickChip,
}: GreetingEmptyStateProps): ReactElement {
  const greeting = resolveTimeOfDayGreeting(currentHour24);

  return (
    <section
      aria-label="New task greeting"
      data-testid="workspace-greeting-empty-state"
      className="ky-greeting-hero"
    >
      <div style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)" }}>
        <ChromeIcon name="sparkles" size={24} />
        <h1 style={{ margin: 0, fontSize: "1.625rem", fontWeight: 600 }}>
          {greeting}
        </h1>
      </div>
      <p style={{ margin: 0, color: "var(--text-secondary)", fontSize: "0.9375rem" }}>
        What would you like to build, inspect, or automate today?
      </p>

      <div className="ky-quick-chips-row">
        {STARTER_QUICK_CHIPS.map((chip, idx) => (
          <div
            key={chip.id}
            className="draft-prompt-card-waterfall"
            style={{ ["--card-index" as string]: idx }}
          >
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onSelectQuickChip(chip)}
            >
              {chip.label}
            </Button>
          </div>
        ))}
      </div>
    </section>
  );
}
