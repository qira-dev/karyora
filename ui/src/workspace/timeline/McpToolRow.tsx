/**
 * MCP Tool Invocation Row (UIR-4.2, Screenshots #69–#71, Blueprint §8.1).
 *
 * Displays the connected MCP server badge (`MCP Desktop-UI-MCP`, `Obsidian`, `9Scan`, `Security`),
 * tool name, execution status, and expandable structured result panel.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge } from "../../components/primitives";
import type { TimelineToolExecutionRow } from "../../coreclient/types";

export interface McpToolRowProps {
  readonly row: TimelineToolExecutionRow;
}

export function McpToolRow({ row }: McpToolRowProps): ReactElement {
  const [expanded, setExpanded] = useState(false);

  return (
    <div
      className="ky-tool-row"
      data-testid={`mcp-tool-row-${row.toolCallId}`}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <button
          type="button"
          className="ky-btn ky-btn--ghost ky-btn--sm"
          aria-expanded={expanded ? "true" : "false"}
          onClick={() => setExpanded((prev) => !prev)}
          style={{ flex: 1, justifyContent: "flex-start" }}
        >
          <ChromeIcon name="mcp-plug" size={14} />
          <Badge tone="accent" label={`MCP ${row.mcpServerName ?? "Server"}`} />
          <code style={{ fontFamily: "var(--font-mono)", fontSize: "0.8125rem" }}>
            {row.toolName}
          </code>
          <ChromeIcon
            name={expanded ? "chevron-down" : "chevron-right"}
            size={14}
          />
        </button>

        <Badge
          tone={
            row.state === "success"
              ? "success"
              : row.state === "error"
                ? "danger"
                : "info"
          }
          label={row.state}
        />
      </div>

      {row.state === "running" && (
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
