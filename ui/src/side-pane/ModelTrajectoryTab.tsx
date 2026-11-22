/**
 * Model Trajectory Inspector Tab (1:1 ZCode v3.18.2 `model-trajectory` side-pane tab).
 *
 * Uses ZCode's exact extracted trajectory color tokens (`--color-trajectory-user`,
 * `--color-trajectory-reasoning`, `--color-trajectory-assistant`, `--color-trajectory-tool-call`,
 * `--color-trajectory-tool-result`).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../components/primitives";
import type { ConversationTurnRecord } from "../workspace/workspaceConfig";

export interface ModelTrajectoryTabProps {
  readonly taskTitle: string;
  readonly turns: readonly ConversationTurnRecord[];
}

export function ModelTrajectoryTab({
  taskTitle,
  turns,
}: ModelTrajectoryTabProps): ReactElement {
  const [filterKind, setFilterKind] = useState<
    "all" | "user" | "reasoning" | "tool-call" | "assistant"
  >("all");

  const entries: Array<{
    readonly id: string;
    readonly kind: "user" | "reasoning" | "tool-call" | "tool-result" | "assistant";
    readonly label: string;
    readonly timestamp: string;
    readonly payload: string;
  }> = [];

  for (const turn of turns) {
    if (turn.role === "user") {
      entries.push({
        id: `${turn.turnId}-user`,
        kind: "user",
        label: "USER PROMPT",
        timestamp: turn.timestamp,
        payload: turn.text,
      });
    } else {
      if (turn.thinkingText) {
        entries.push({
          id: `${turn.turnId}-reasoning`,
          kind: "reasoning",
          label: "REASONING",
          timestamp: turn.timestamp,
          payload: turn.thinkingText,
        });
      }
      for (const row of turn.toolRows ?? []) {
        entries.push({
          id: `${row.toolCallId}-call`,
          kind: "tool-call",
          label: `TOOL CALL · ${row.toolName}`,
          timestamp: row.startedAt,
          payload: JSON.stringify(
            {
              tool: row.toolName,
              verb: row.actionVerb,
              target: row.targetFilePath ?? row.commandLine ?? row.mcpServerName,
              state: row.state,
            },
            null,
            2,
          ),
        });
        if (row.outputPreview) {
          entries.push({
            id: `${row.toolCallId}-res`,
            kind: "tool-result",
            label: `TOOL RESULT · ${row.toolName}`,
            timestamp: row.completedAt ?? row.startedAt,
            payload: row.outputPreview,
          });
        }
      }
      if (turn.text) {
        entries.push({
          id: `${turn.turnId}-assistant`,
          kind: "assistant",
          label: "ASSISTANT OUTPUT",
          timestamp: turn.timestamp,
          payload: turn.text,
        });
      }
    }
  }

  const visibleEntries = entries.filter((e) => {
    if (filterKind === "all") return true;
    if (filterKind === "tool-call") {
      return e.kind === "tool-call" || e.kind === "tool-result";
    }
    return e.kind === filterKind;
  });

  return (
    <section
      className="ky-side-tool-page"
      data-testid="model-trajectory-tab"
      aria-label="Model trajectory inspector"
    >
      <div className="ky-side-tool-page__inner">
        <div className="ky-side-tool-page__head-row">
          <div>
            <h3 className="ky-side-tool-page__subhead">Model Trajectory</h3>
            <p className="ky-setting-row__desc">{taskTitle}</p>
          </div>
          <Badge tone="neutral" label={`${entries.length} frames`} />
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px" }}>
          {(["all", "user", "reasoning", "tool-call", "assistant"] as const).map(
            (kind) => (
              <Button
                key={kind}
                size="sm"
                variant={filterKind === kind ? "secondary" : "ghost"}
                selected={filterKind === kind}
                onClick={() => setFilterKind(kind)}
              >
                {kind === "all"
                  ? "All frames"
                  : kind === "tool-call"
                    ? "Tools"
                    : kind.charAt(0).toUpperCase() + kind.slice(1)}
              </Button>
            ),
          )}
        </div>

        <div className="ky-trajectory-list">
          {visibleEntries.map((item) => (
            <article
              key={item.id}
              className="ky-trajectory-card"
              data-trajectory-kind={item.kind}
            >
              <header className="ky-trajectory-card__head">
                <span className="ky-trajectory-card__label">{item.label}</span>
                <span className="ky-trajectory-card__time">{item.timestamp}</span>
              </header>
              <pre className="ky-trajectory-card__body">{item.payload}</pre>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

export default ModelTrajectoryTab;
