/**
 * Collapsible Inline `Thinking` Brain Row with Shimmer Animation
 * (UIR-4.2, Screenshot #79, Blueprint §8.1, §20.2).
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";

export interface ThinkingRowProps {
  readonly thinkingText: string;
  readonly isStreaming?: boolean;
  readonly defaultExpanded?: boolean;
}

export function ThinkingRow({
  thinkingText,
  isStreaming = false,
  defaultExpanded = false,
}: ThinkingRowProps): ReactElement {
  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <div
      data-testid="timeline-thinking-row"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-1)",
      }}
    >
      <button
        type="button"
        aria-expanded={expanded ? "true" : "false"}
        className="ky-btn ky-btn--ghost ky-btn--sm"
        style={{ justifyContent: "flex-start", alignSelf: "flex-start" }}
        onClick={() => setExpanded((prev) => !prev)}
      >
        <ChromeIcon name="brain-thinking" size={14} />
        <span className={isStreaming ? "animated-gradient-text" : ""}>
          {isStreaming ? "Thinking..." : "Thinking"}
        </span>
        <ChromeIcon name={expanded ? "chevron-down" : "chevron-right"} size={14} />
      </button>

      {expanded && (
        <div
          style={{
            padding: "var(--space-2) var(--space-3)",
            borderInlineStart: "2px solid var(--border-default)",
            marginInlineStart: "var(--space-2)",
            fontSize: "0.8125rem",
            color: "var(--text-secondary)",
            whiteSpace: "pre-wrap",
          }}
        >
          {thinkingText}
        </div>
      )}
    </div>
  );
}
