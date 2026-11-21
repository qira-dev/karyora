/**
 * Floating `Progress X/Y & Agents` Overlay Card
 * (UIR-4.2, Screenshots #70–#74, Blueprint §8.2, E2E-06).
 */

import { useState, type ReactElement } from "react";
import { DigitFlipCounter } from "../../components/feedback";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button } from "../../components/primitives";
import type {
  TimelineProgressStep,
  TimelineSubagentBadge,
} from "../workspaceConfig";

export interface FloatingProgressCardProps {
  readonly steps: readonly TimelineProgressStep[];
  readonly subagents: readonly TimelineSubagentBadge[];
  readonly onOpenSubagentsSideTab: () => void;
}

export function FloatingProgressCard({
  steps,
  subagents,
  onOpenSubagentsSideTab,
}: FloatingProgressCardProps): ReactElement | null {
  const [expanded, setExpanded] = useState(false);

  if (steps.length === 0 && subagents.length === 0) {
    return null;
  }

  const completedCount = steps.filter((s) => s.completed).length;
  const runningSubagents = subagents.filter((a) => a.status === "running").length;

  return (
    <div
      data-testid="floating-progress-agents-card"
      className="ky-floating-progress-card"
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-3)",
        }}
      >
        <button
          type="button"
          className="ky-btn ky-btn--ghost ky-btn--sm"
          aria-expanded={expanded ? "true" : "false"}
          onClick={() => setExpanded((prev) => !prev)}
        >
          <ChromeIcon name="check" size={14} />
          <span>
            Progress <DigitFlipCounter value={`${completedCount}/${steps.length}`} />
          </span>
          <ChromeIcon name={expanded ? "chevron-down" : "chevron-up"} size={14} />
        </button>

        {subagents.length > 0 && (
          <Button
            variant="secondary"
            size="sm"
            leadingIcon={<ChromeIcon name="subagent" size={14} />}
            onClick={onOpenSubagentsSideTab}
          >
            {subagents.length} Agents
            {runningSubagents > 0 ? ` (${runningSubagents} running)` : ""}
          </Button>
        )}
      </div>

      {expanded && steps.length > 0 && (
        <ul
          style={{
            margin: 0,
            padding: "var(--space-1) var(--space-2)",
            listStyle: "none",
            display: "flex",
            flexDirection: "column",
            gap: "4px",
            borderTop: "1px solid var(--border-subtle)",
          }}
        >
          {steps.map((step) => (
            <li
              key={step.stepId}
              style={{
                display: "flex",
                alignItems: "center",
                gap: "var(--space-2)",
                fontSize: "0.8125rem",
                color: step.completed
                  ? "var(--text-muted)"
                  : "var(--text-primary)",
                textDecoration: step.completed ? "line-through" : "none",
              }}
            >
              <Badge
                tone={step.completed ? "success" : "info"}
                label={step.completed ? "✓" : "•"}
              />
              <span>{step.label}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
