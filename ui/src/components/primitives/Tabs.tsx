/**
 * W3C WAI-ARIA APG 1.2 Accessible Tabs Primitive (UIR-2.3, Blueprint §8.8, §20.5, G-05, G-06).
 *
 * Implements `role="tablist"`, `role="tab"`, and `role="tabpanel"` with:
 * - Roving `tabIndex` (`0` on active tab, `-1` on inactive tabs)
 * - Horizontal (`ArrowLeft` / `ArrowRight`) and Vertical (`ArrowUp` / `ArrowDown`) keyboard traversal
 * - `Home` and `End` jump navigation
 * - Explicit `aria-selected` and `aria-controls` / `aria-labelledby` wiring
 */

import {
  useId,
  useRef,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from "react";

export interface TabDefinition {
  readonly id: string;
  readonly label: string;
  readonly icon?: ReactNode;
  readonly badgeCount?: number;
  readonly disabled?: boolean;
  readonly closable?: boolean;
}

export interface TabsProps {
  readonly tabs: readonly TabDefinition[];
  readonly activeTabId: string;
  readonly onSelectTab: (tabId: string) => void;
  readonly onCloseTab?: (tabId: string) => void;
  readonly orientation?: "horizontal" | "vertical";
  readonly ariaLabel: string;
  readonly children?: ReactNode;
}

export function computeNextTabIndex(
  tabs: readonly TabDefinition[],
  currentIndex: number,
  key: string,
  orientation: "horizontal" | "vertical",
): number {
  const enabledIndices = tabs
    .map((t, idx) => (t.disabled ? -1 : idx))
    .filter((idx) => idx >= 0);

  if (enabledIndices.length === 0) {
    return currentIndex;
  }

  const forwardKey = orientation === "horizontal" ? "ArrowRight" : "ArrowDown";
  const backwardKey = orientation === "horizontal" ? "ArrowLeft" : "ArrowUp";

  if (key === "Home") {
    return enabledIndices[0];
  }
  if (key === "End") {
    return enabledIndices[enabledIndices.length - 1];
  }

  const pos = enabledIndices.indexOf(currentIndex);
  if (pos === -1) {
    return enabledIndices[0];
  }

  if (key === forwardKey) {
    return enabledIndices[(pos + 1) % enabledIndices.length];
  }
  if (key === backwardKey) {
    return enabledIndices[
      (pos - 1 + enabledIndices.length) % enabledIndices.length
    ];
  }
  return currentIndex;
}

export function Tabs({
  tabs,
  activeTabId,
  onSelectTab,
  onCloseTab,
  orientation = "horizontal",
  ariaLabel,
  children,
}: TabsProps): ReactElement {
  const baseId = useId();
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const handleKeyDown = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const nextIndex = computeNextTabIndex(tabs, index, e.key, orientation);
    if (nextIndex !== index) {
      e.preventDefault();
      const targetTab = tabs[nextIndex];
      if (targetTab && !targetTab.disabled) {
        onSelectTab(targetTab.id);
        tabRefs.current[nextIndex]?.focus();
      }
    }
  };

  return (
    <div
      style={{
        display: "flex",
        flexDirection: orientation === "vertical" ? "row" : "column",
        width: "100%",
      }}
    >
      <div
        role="tablist"
        aria-label={ariaLabel}
        aria-orientation={orientation}
        style={{
          display: "flex",
          flexDirection: orientation === "vertical" ? "column" : "row",
          gap: "var(--space-1)",
          borderBottom:
            orientation === "horizontal"
              ? "1px solid var(--border-subtle)"
              : undefined,
          borderInlineEnd:
            orientation === "vertical"
              ? "1px solid var(--border-subtle)"
              : undefined,
        }}
      >
        {tabs.map((tab, index) => {
          const isSelected = tab.id === activeTabId;
          const tabDomId = `${baseId}-tab-${tab.id}`;
          const panelDomId = `${baseId}-panel-${tab.id}`;

          return (
            <div
              key={tab.id}
              style={{ display: "inline-flex", alignItems: "center" }}
            >
              <button
                ref={(el) => {
                  tabRefs.current[index] = el;
                }}
                id={tabDomId}
                type="button"
                role="tab"
                aria-selected={isSelected ? "true" : "false"}
                aria-controls={panelDomId}
                aria-disabled={tab.disabled ? "true" : undefined}
                disabled={tab.disabled}
                tabIndex={isSelected ? 0 : -1}
                className={`ky-btn ky-btn--ghost ky-btn--sm ${
                  isSelected ? "ky-btn--selected" : ""
                }`}
                onClick={() => {
                  if (!tab.disabled) {
                    onSelectTab(tab.id);
                  }
                }}
                onKeyDown={(e) => handleKeyDown(e, index)}
              >
                {tab.icon}
                <span>{tab.label}</span>
                {typeof tab.badgeCount === "number" && (
                  <span className="ky-badge">{tab.badgeCount}</span>
                )}
              </button>
              {tab.closable && onCloseTab && (
                <button
                  type="button"
                  aria-label={`Close ${tab.label} tab`}
                  className="ky-btn ky-btn--ghost ky-btn--sm"
                  onClick={() => onCloseTab(tab.id)}
                >
                  ×
                </button>
              )}
            </div>
          );
        })}
      </div>
      <div
        id={`${baseId}-panel-${activeTabId}`}
        role="tabpanel"
        aria-labelledby={`${baseId}-tab-${activeTabId}`}
        tabIndex={0}
        style={{ flex: 1, minWidth: 0 }}
      >
        {children}
      </div>
    </div>
  );
}
