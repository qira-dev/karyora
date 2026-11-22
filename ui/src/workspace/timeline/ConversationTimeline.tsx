/**
 * Live Conversation Stream Timeline with Left-Margin Turn Navigation Rail
 * (UIR-4.2, Screenshots #69–#77, #79, #84, Blueprint §8.1–8.2).
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
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
  readonly onEditUserTurn?: (text: string) => void;
  readonly onForkTurn?: (turnId: string) => void;
  readonly onOpenHooksSettings?: () => void;
  readonly onNotify?: (title: string, detail?: string) => void;
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
  onEditUserTurn,
  onForkTurn,
  onOpenHooksSettings,
  onNotify,
  onApprovalDecision,
  onOpenSubagentsSideTab,
}: ConversationTimelineProps): ReactElement {
  const [focusedTurnId, setFocusedTurnId] = useState<string | null>(
    turns[turns.length - 1]?.turnId ?? null,
  );
  const [expandedWorkedTurns, setExpandedWorkedTurns] = useState<ReadonlySet<string>>(
    () => new Set(turns.map((t) => t.turnId)),
  );
  const [feedbackByTurn, setFeedbackByTurn] = useState<
    Readonly<Record<string, "like" | "dislike" | null>>
  >({});

  const toggleWorkedExpanded = (turnId: string) => {
    setExpandedWorkedTurns((prev) => {
      const next = new Set(prev);
      if (next.has(turnId)) next.delete(turnId);
      else next.add(turnId);
      return next;
    });
  };

  const copyText = (text: string) => {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      void navigator.clipboard.writeText(text);
    }
    onNotify?.("Copied to clipboard");
  };

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
          {turns.map((turn) => {
            const hasWorkSteps =
              Boolean(turn.thinkingText) || (turn.toolRows && turn.toolRows.length > 0);
            const isWorkRunning = Boolean(
              turn.isThinkingStreaming ||
                turn.toolRows?.some(
                  (r) => r.state === "running" || r.state === "inputStreaming",
                ),
            );
            const isWorkExpanded = expandedWorkedTurns.has(turn.turnId);
            const fb = feedbackByTurn[turn.turnId] ?? null;

            return (
              <article
                key={turn.turnId}
                id={`turn-${turn.turnId}`}
                className={turn.role === "user" ? "ky-user-turn-wrap" : "ky-assistant-turn-wrap"}
              >
                {turn.role === "user" ? (
                  <>
                    <div className="ky-user-bubble">{turn.text}</div>
                    <div className="ky-turn-actions ky-turn-actions--user">
                      <button
                        type="button"
                        className="ky-turn-action-btn"
                        aria-label="Copy message"
                        title="Copy"
                        onClick={() => copyText(turn.text)}
                      >
                        <ChromeIcon name="copy" size={14} />
                      </button>
                      {onEditUserTurn && (
                        <button
                          type="button"
                          className="ky-turn-action-btn"
                          aria-label="Edit message"
                          title="Edit"
                          onClick={() => onEditUserTurn(turn.text)}
                        >
                          <ChromeIcon name="edit" size={14} />
                        </button>
                      )}
                    </div>
                  </>
                ) : (
                  <>
                    {/* ZCode Collapsible `Worked for Xs` / Shimmering `Working...` Disclosure (`17-session-reply.aria.yaml`) */}
                    {hasWorkSteps && (
                      <div className="ky-worked-group">
                        <button
                          type="button"
                          className="ky-worked-toggle"
                          aria-expanded={isWorkExpanded}
                          onClick={() => toggleWorkedExpanded(turn.turnId)}
                        >
                          <ChromeIcon
                            name={isWorkExpanded ? "chevron-down" : "chevron-right"}
                            size={14}
                          />
                          <span className={isWorkRunning ? "animated-gradient-text" : undefined}>
                            {isWorkRunning
                              ? "Working..."
                              : `Worked for ${Math.max(2, (turn.toolRows?.length ?? 1) * 4)}s`}
                          </span>
                        </button>

                        {isWorkExpanded && (
                          <div className="ky-worked-body">
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
                          </div>
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

                    {/* ZCode Assistant Reply Footer Bar (`Copy`, `Like`, `Dislike`, `Fork`, `Hooks` + Timestamp) */}
                    <footer className="ky-turn-actions ky-turn-actions--assistant">
                      <button
                        type="button"
                        className="ky-turn-action-btn"
                        aria-label="Copy"
                        title="Copy"
                        onClick={() => copyText(turn.text)}
                      >
                        <ChromeIcon name="copy" size={14} />
                      </button>
                      <button
                        type="button"
                        className="ky-turn-action-btn"
                        data-active={fb === "like" ? "true" : "false"}
                        aria-label="Like"
                        title="Like"
                        onClick={() =>
                          setFeedbackByTurn((prev) => ({
                            ...prev,
                            [turn.turnId]: prev[turn.turnId] === "like" ? null : "like",
                          }))
                        }
                      >
                        <ChromeIcon name="thumbs-up" size={14} />
                      </button>
                      <button
                        type="button"
                        className="ky-turn-action-btn"
                        data-active={fb === "dislike" ? "true" : "false"}
                        aria-label="Dislike"
                        title="Dislike"
                        onClick={() =>
                          setFeedbackByTurn((prev) => ({
                            ...prev,
                            [turn.turnId]: prev[turn.turnId] === "dislike" ? null : "dislike",
                          }))
                        }
                      >
                        <ChromeIcon name="thumbs-down" size={14} />
                      </button>
                      {onForkTurn && (
                        <button
                          type="button"
                          className="ky-turn-action-btn"
                          aria-label="Fork"
                          title="Fork conversation from here"
                          onClick={() => onForkTurn(turn.turnId)}
                        >
                          <ChromeIcon name="git-fork" size={14} />
                        </button>
                      )}
                      {onOpenHooksSettings && (
                        <button
                          type="button"
                          className="ky-turn-action-btn"
                          aria-label="Hooks"
                          title="Inspect session hooks"
                          onClick={onOpenHooksSettings}
                        >
                          <ChromeIcon name="hooks" size={14} />
                        </button>
                      )}
                      <span className="ky-turn-timestamp">{turn.timestamp}</span>
                    </footer>
                  </>
                )}
              </article>
            );
          })}
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
