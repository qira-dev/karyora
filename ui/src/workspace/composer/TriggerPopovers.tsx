/**
 * Composer Inline Trigger Popovers (`@` Files/Resources, `/` Commands, `#` Sessions, `$` Skills)
 * (UIR-4.1, Screenshot #68, Blueprint §7.3, §20.5).
 */

import type { ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../../components/icons";
import { Button } from "../../components/primitives";
import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import type { ComposerTriggerChar } from "../workspaceConfig";

export interface TriggerSuggestionItem {
  readonly id: string;
  readonly label: string;
  readonly subtitle: string;
  readonly trigger: ComposerTriggerChar;
  readonly filePathForIcon?: string;
}

export interface TriggerPopoversProps {
  readonly activeTrigger: ComposerTriggerChar | null;
  readonly query: string;
  readonly codeTheme: LightCodeTheme | DarkCodeTheme;
  readonly suggestions: readonly TriggerSuggestionItem[];
  readonly onSelectSuggestion: (item: TriggerSuggestionItem) => void;
  readonly onDismiss: () => void;
}

const TRIGGER_TITLES: Readonly<Record<ComposerTriggerChar, string>> = {
  "@": "Files, Folders & KnowledgePacks (@)",
  "/": "Slash Commands & Workflows (/)",
  "#": "Sessions & Tasks (#)",
  "$": "Skills ($)",
};

export function TriggerPopovers({
  activeTrigger,
  query,
  codeTheme,
  suggestions,
  onSelectSuggestion,
  onDismiss,
}: TriggerPopoversProps): ReactElement | null {
  if (!activeTrigger) {
    return null;
  }

  const q = query.toLowerCase();
  const filtered = suggestions.filter(
    (item) =>
      item.trigger === activeTrigger &&
      (q.length === 0 ||
        item.label.toLowerCase().includes(q) ||
        item.subtitle.toLowerCase().includes(q)),
  );

  return (
    <div
      role="listbox"
      aria-label={TRIGGER_TITLES[activeTrigger]}
      data-testid={`composer-trigger-popover-${activeTrigger}`}
      className="ky-popover-surface"
      style={{
        bottom: "calc(100% + 8px)",
        insetInlineStart: 0,
        width: "min(440px, 100%)",
        maxHeight: "240px",
        overflowY: "auto",
        padding: "var(--space-2)",
        display: "flex",
        flexDirection: "column",
        gap: "2px",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "var(--space-1) var(--space-2)",
          fontSize: "0.75rem",
          color: "var(--text-muted)",
        }}
      >
        <span>{TRIGGER_TITLES[activeTrigger]}</span>
        <Button variant="ghost" size="sm" onClick={onDismiss}>
          Esc
        </Button>
      </div>

      {filtered.length === 0 ? (
        <div
          style={{
            padding: "var(--space-2)",
            fontSize: "0.8125rem",
            color: "var(--text-muted)",
          }}
        >
          No matching items for &quot;{activeTrigger}
          {query}&quot;
        </div>
      ) : (
        filtered.map((item) => (
          <button
            key={item.id}
            type="button"
            role="option"
            aria-selected="false"
            className="ky-btn ky-btn--ghost ky-btn--sm"
            style={{ width: "100%", justifyContent: "flex-start" }}
            onClick={() => onSelectSuggestion(item)}
          >
            {item.filePathForIcon ? (
              <MaterialFileIcon
                filename={item.filePathForIcon}
                codeTheme={codeTheme}
                size={16}
              />
            ) : (
              <ChromeIcon
                name={
                  activeTrigger === "/"
                    ? "commands"
                    : activeTrigger === "$"
                      ? "skills"
                      : "tasks"
                }
                size={14}
              />
            )}
            <span style={{ fontWeight: 500 }}>{item.label}</span>
            <span
              style={{
                marginInlineStart: "auto",
                fontSize: "0.75rem",
                color: "var(--text-muted)",
              }}
            >
              {item.subtitle}
            </span>
          </button>
        ))
      )}
    </div>
  );
}
