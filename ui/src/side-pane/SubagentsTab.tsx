/**
 * Side-Pane `Subagents` Directory & Trajectory Report Inspector
 * (UIR-5.3, Screenshots #71–#73, Blueprint §8.4, E2E-06).
 *
 * Displays Running and Ended subagent cards with elapsed timers (`Ran for 2m 13s`)
 * and opens full markdown + tool-trajectory inspection views on click.
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";
import type { SubagentRunRecord } from "./sidePaneConfig";

export interface SubagentsTabProps {
  readonly subagents: readonly SubagentRunRecord[];
  readonly inspectingSubagentId?: string | null;
  readonly onInspectSubagent: (subagent: SubagentRunRecord) => void;
  readonly onBackToDirectory?: () => void;
}

export function SubagentsTab({
  subagents,
  inspectingSubagentId,
  onInspectSubagent,
  onBackToDirectory,
}: SubagentsTabProps): ReactElement {
  const inspected = inspectingSubagentId
    ? subagents.find((s) => s.subagentId === inspectingSubagentId) ?? null
    : null;

  if (inspected) {
    return (
      <section
        className="ky-subagent-report"
        data-testid={`subagent-report-${inspected.subagentId}`}
        aria-label={`Subagent report: ${inspected.name}`}
      >
        <header className="ky-subagent-report__header">
          <div className="ky-subagent-report__title-row">
            {onBackToDirectory && (
              <Button size="sm" variant="ghost" onClick={onBackToDirectory}>
                <ChromeIcon name="arrow-left" size={14} />
                <span>Subagents</span>
              </Button>
            )}
            <ChromeIcon name="subagent" size={16} />
            <h3>{inspected.name}</h3>
            <Badge
              tone={inspected.status === "running" ? "info" : "success"}
              label={inspected.status === "running" ? "Running" : "Ended"}
            />
          </div>
          <span className="ky-subagent-report__elapsed">{inspected.elapsedLabel}</span>
        </header>

        <div className="ky-subagent-report__summary">
          <p>{inspected.summaryMarkdown}</p>
        </div>

        <div className="ky-subagent-report__trajectory">
          <h4>Tool Trajectory ({inspected.trajectorySteps.length} steps)</h4>
          {inspected.trajectorySteps.map((step) => (
            <div
              key={step.stepId}
              className="ky-subagent-report__step"
              data-step-status={step.status}
            >
              <Badge
                tone={
                  step.status === "success"
                    ? "success"
                    : step.status === "running"
                      ? "info"
                      : "danger"
                }
                label={step.action}
              />
              <span>{step.detail}</span>
            </div>
          ))}
        </div>
      </section>
    );
  }

  const running = subagents.filter((s) => s.status === "running");
  const ended = subagents.filter((s) => s.status === "ended");

  const renderGroup = (title: string, list: readonly SubagentRunRecord[]) => (
    <div className="ky-subagents-dir__group">
      <h4 className="ky-subagents-dir__heading">
        {title} ({list.length})
      </h4>
      {list.length === 0 ? (
        <p className="ky-subagents-dir__empty">No {title.toLowerCase()} subagents.</p>
      ) : (
        <div className="ky-subagents-dir__list">
          {list.map((agent) => (
            <button
              key={agent.subagentId}
              type="button"
              className="ky-subagents-dir__card"
              data-testid={`subagent-card-${agent.subagentId}`}
              onClick={() => onInspectSubagent(agent)}
            >
              <div className="ky-subagents-dir__card-top">
                <span className="ky-subagents-dir__card-name">
                  <ChromeIcon name="subagent" size={16} />
                  <strong>{agent.name}</strong>
                </span>
                <Badge
                  tone={agent.status === "running" ? "info" : "neutral"}
                  label={agent.elapsedLabel}
                />
              </div>
              <div className="ky-subagents-dir__card-meta">
                <span>{agent.role}</span>
                <span>{agent.toolCallsCount} tool calls</span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );

  return (
    <section
      className="ky-subagents-dir"
      data-testid="subagents-tab"
      aria-label="Subagents directory"
    >
      {renderGroup("Running", running)}
      {renderGroup("Ended", ended)}
    </section>
  );
}
