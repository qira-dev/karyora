/**
 * Pure TypeScript ARIA & Design-System Helper Functions (UIR-2.2, UIR-2.3, G-04, G-05, G-06).
 *
 * Uses type-only imports (`import type`) so both `tsc -b` (Vite bundler resolution)
 * and Node's native ESM TypeScript test runner (`node --test`) execute it directly.
 */

import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import type { ResolveMaterialIconOptions } from "../icons/materialIconManifest";
import type { TabDefinition } from "./Tabs";
import type { FlatVisibleTreeNode, TreeNodeItem } from "./TreeView";

const LIGHT_THEME_SET: ReadonlySet<string> = new Set<string>([
  "GitHub",
  "One Light",
  "Solarized Light",
  "Catppuccin Latte",
  "Atom One Light",
  "Quiet Light",
  "Night Owl Light",
  "Vitesse Light",
  "Min Light",
]);

export function resolveIconThemeVariant(
  codeTheme?: LightCodeTheme | DarkCodeTheme,
  explicitVariant?: ResolveMaterialIconOptions["themeVariant"],
): ResolveMaterialIconOptions["themeVariant"] {
  if (explicitVariant) {
    return explicitVariant;
  }
  if (!codeTheme) {
    return "default";
  }
  if (codeTheme === "Gruvbox Dark") {
    return "gruvbox_dark";
  }
  if (codeTheme === "Gruvbox Light") {
    return "gruvbox_light";
  }
  if (LIGHT_THEME_SET.has(codeTheme)) {
    return "light";
  }
  return "default";
}

export function formatByteCounter(currentBytes: number, maxBytes: number): string {
  const currentKb = (currentBytes / 1024).toFixed(currentBytes === 0 ? 0 : 1);
  const maxKb = Math.round(maxBytes / 1024);
  return `${currentKb} KB / ${maxKb} KB`;
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

export function flattenVisibleTree(
  nodes: readonly TreeNodeItem[],
  expandedIds: ReadonlySet<string>,
  depth = 0,
  parentId: string | null = null,
): readonly FlatVisibleTreeNode[] {
  const result: FlatVisibleTreeNode[] = [];
  for (const node of nodes) {
    const isExpanded = Boolean(node.isFolder && expandedIds.has(node.id));
    result.push({
      node,
      depth,
      parentId,
      isExpanded,
    });
    if (node.isFolder && isExpanded && node.children && node.children.length > 0) {
      result.push(
        ...flattenVisibleTree(node.children, expandedIds, depth + 1, node.id),
      );
    }
  }
  return result;
}
