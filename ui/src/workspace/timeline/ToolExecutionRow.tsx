/**
 * Canonical 6-State Tool Execution Row with 1,147 Material File/Language SVG Icons & `+A -R` Diff Counters
 * (UIR-4.2, Screenshots #75–#77, #84, Blueprint §8.1, §20.4).
 *
 * Strictly reflects all 6 tool row states:
 * `inputStreaming` -> `pendingApproval` -> `running` -> `success` | `error` | `cancelled`
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../../components/icons";
import { Badge, DiffCountPill } from "../../components/primitives";
import type { TimelineToolExecutionRow } from "../../coreclient/types";
import type { DarkCodeTheme, LightCodeTheme, ToolRowState } from "../../types/decisions";

export interface ToolExecutionRowProps {
  readonly row: TimelineToolExecutionRow;
  readonly codeTheme: LightCodeTheme | DarkCodeTheme;
  readonly onInspectFile?: (filePath: string) => void;
}

const STATE_BADGE_MAP: Readonly<
  Record<ToolRowState, { label: string; tone: "neutral" | "info" | "warning" | "success" | "danger" }>
> = {
  inputStreaming: { label: "Preparing", tone: "info" },
  pendingApproval: { label: "Approval required", tone: "warning" },
  running: { label: "Running", tone: "info" },
  success: { label: "Done", tone: "success" },
  error: { label: "Error", tone: "danger" },
  cancelled: { label: "Stopped", tone: "neutral" },
};

export function ToolExecutionRow({
  row,
  codeTheme,
  onInspectFile,
}: ToolExecutionRowProps): ReactElement {
  const [expanded, setExpanded] = useState(row.state === "error");
  const statusMeta = STATE_BADGE_MAP[row.state];
  const basename = row.targetFilePath
    ? row.targetFilePath.split("/").pop() ?? row.targetFilePath
    : row.commandLine ?? row.toolName;

  const hasDiff =
    typeof row.diffAdditions === "number" ||
    typeof row.diffDeletions === "number";

  return (
    <div
      className="ky-tool-row"
      data-testid={`tool-row-${row.toolCallId}`}
      data-tool-state={row.state}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--space-2)",
          minWidth: 0,
        }}
      >
        <button
          type="button"
          className="ky-btn ky-btn--ghost ky-btn--sm"
          style={{
            flex: 1,
            minWidth: 0,
            justifyContent: "flex-start",
            paddingInline: "var(--space-1)",
          }}
          title={row.targetFilePath ?? row.commandLine ?? row.toolName}
          onClick={() => {
            if (row.targetFilePath && onInspectFile) {
              onInspectFile(row.targetFilePath);
            } else {
              setExpanded((prev) => !prev);
            }
          }}
        >
          <span
            className={
              row.state === "inputStreaming" || row.state === "running"
                ? "animated-gradient-text"
                : ""
            }
            style={{ color: "var(--text-secondary)", fontWeight: 500 }}
          >
            {row.actionVerb}
          </span>

          {row.targetFilePath ? (
            <MaterialFileIcon
              filename={row.targetFilePath}
              codeTheme={codeTheme}
              size={16}
            />
          ) : (
            <ChromeIcon name="terminal" size={14} />
          )}

          <code
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "0.8125rem",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {basename}
          </code>

          {hasDiff && (
            <DiffCountPill
              additions={row.diffAdditions ?? 0}
              deletions={row.diffDeletions ?? 0}
            />
          )}
        </button>

        <Badge tone={statusMeta.tone} label={statusMeta.label} />

        {row.outputPreview && (
          <button
            type="button"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            aria-expanded={expanded ? "true" : "false"}
            aria-label={`Toggle output for ${basename}`}
            onClick={() => setExpanded((prev) => !prev)}
          >
            <ChromeIcon name={expanded ? "chevron-down" : "chevron-right"} size={14} />
          </button>
        )}
      </div>

      {(row.state === "running" || row.state === "inputStreaming") && (
        <div className="ky-tool-row__progress-track" aria-hidden="true">
          <div className="ky-tool-row__progress-bar tool-call-progress-indeterminate" />
        </div>
      )}

      {expanded && row.outputPreview && (
        <pre
          style={{
            margin: "var(--space-1) 0 0 0",
            padding: "var(--space-2)",
            borderRadius: "var(--radius-xs)",
            backgroundColor: "var(--code-bg)",
            color: "var(--code-fg)",
            fontFamily: "var(--font-mono)",
            fontSize: "0.75rem",
            overflowX: "auto",
          }}
        >
          {row.outputPreview}
        </pre>
      )}
    </div>
  );
}
