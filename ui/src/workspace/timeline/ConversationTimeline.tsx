/**
 * Live Conversation Stream Timeline with Left-Margin Turn Navigation Rail
 * (UIR-4.2, Screenshots #69–#77, #79, #84, Blueprint §8.1–8.2).
 */

import { useState, type ReactElement } from "react";
import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import type {
  ConversationTurnRecord,
  TimelineProgressStep,
  TimelineSubagentBadge,
} from "../workspaceConfig";
import { ApprovalBanner } from "./ApprovalBanner";
import { FloatingProgressCard } from "./FloatingProgressCard";
import { McpToolRow } from "./McpToolRow";
import { ThinkingRow } from "./ThinkingRow";
import { ToolExecutionRow } from "./ToolExecutionRow";

export interface ConversationTimelineProps {
  readonly turns: readonly ConversationTurnRecord[];
  readonly progressSteps: readonly TimelineProgressStep[];
  readonly subagents: readonly TimelineSubagentBadge[];
  readonly codeTheme: LightCodeTheme | DarkCodeTheme;
  readonly onInspectFile: (filePath: string) => void;
  readonly onApprovalDecision: (
    approvalId: string,
    decision: "approve" | "approve-session" | "reject",
  ) => void;
  readonly onOpenSubagentsSideTab: () => void;
}

export function ConversationTimeline({
  turns,
  progressSteps,
  subagents,
  codeTheme,
  onInspectFile,
  onApprovalDecision,
  onOpenSubagentsSideTab,
}: ConversationTimelineProps): ReactElement {
  const [focusedTurnId, setFocusedTurnId] = useState<string | null>(
    turns[turns.length - 1]?.turnId ?? null,
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)", flex: 1 }}>
      <div className="ky-timeline-layout" data-testid="conversation-timeline">
        {/* Left-Margin Turn Navigation Rail (Screenshots #75–#77) */}
        <nav
          aria-label="Conversation turns navigation rail"
          className="ky-turn-rail"
        >
          {turns.map((turn, idx) => (
            <button
              key={turn.turnId}
              type="button"
              className="ky-turn-rail__dash"
              data-active={turn.turnId === focusedTurnId ? "true" : "false"}
              aria-label={`Jump to turn ${idx + 1} (${turn.role})`}
              title={`Turn ${idx + 1}: ${turn.role}`}
              onClick={() => {
                setFocusedTurnId(turn.turnId);
                document.getElementById(`turn-${turn.turnId}`)?.scrollIntoView({
                  behavior: "smooth",
                  block: "start",
                });
              }}
            />
          ))}
        </nav>

        <div className="ky-timeline-stream">
          {turns.map((turn) => (
            <article
              key={turn.turnId}
              id={`turn-${turn.turnId}`}
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-2)",
                padding:
                  turn.role === "user" ? "var(--space-3)" : "var(--space-1) 0",
                borderRadius: turn.role === "user" ? "var(--radius-md)" : undefined,
                backgroundColor:
                  turn.role === "user" ? "var(--bg-surface-2)" : "transparent",
                border:
                  turn.role === "user"
                    ? "1px solid var(--border-subtle)"
                    : undefined,
              }}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  fontSize: "0.75rem",
                  color: "var(--text-muted)",
                }}
              >
                <strong style={{ color: "var(--text-secondary)" }}>
                  {turn.role === "user" ? "You" : "Karyora Agent"}
                </strong>
                <span>{turn.timestamp}</span>
              </div>

              {turn.thinkingText && (
                <ThinkingRow
                  thinkingText={turn.thinkingText}
                  isStreaming={turn.isThinkingStreaming}
                />
              )}

              {turn.toolRows && turn.toolRows.length > 0 && (
                <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                  {turn.toolRows.map((row) =>
                    row.actionVerb === "MCP Tool" ? (
                      <McpToolRow key={row.toolCallId} row={row} />
                    ) : (
                      <ToolExecutionRow
                        key={row.toolCallId}
                        row={row}
                        codeTheme={codeTheme}
                        onInspectFile={onInspectFile}
                      />
                    ),
                  )}
                </div>
              )}

              {turn.pendingApproval && (
                <ApprovalBanner
                  approvalId={turn.pendingApproval.approvalId}
                  commandOrTarget={turn.pendingApproval.commandOrTarget}
                  scopeLabel={turn.pendingApproval.scopeLabel}
                  riskLevel={turn.pendingApproval.riskLevel}
                  diffPreview={turn.pendingApproval.diffPreview}
                  status={turn.pendingApproval.status}
                  onDecision={onApprovalDecision}
                />
              )}

              {turn.text && (
                <div
                  style={{
                    fontSize: "0.9375rem",
                    lineHeight: 1.6,
                    whiteSpace: "pre-wrap",
                  }}
                >
                  {turn.text}
                </div>
              )}
            </article>
          ))}
        </div>
      </div>

      <FloatingProgressCard
        steps={progressSteps}
        subagents={subagents}
        onOpenSubagentsSideTab={onOpenSubagentsSideTab}
      />
    </div>
  );
}
