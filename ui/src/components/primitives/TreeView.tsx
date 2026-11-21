/**
 * W3C WAI-ARIA APG 1.2 Accessible Tree View Primitive (UIR-2.3, Blueprint §9.3, §20.5, G-05, G-06).
 *
 * Implements `role="tree"` and `role="treeitem"` with:
 * - `aria-expanded` on folder/group nodes
 * - `ArrowRight` to expand folder or move to first child
 * - `ArrowLeft` to collapse folder or move to parent
 * - `ArrowUp` / `ArrowDown` to traverse visible nodes
 * - `Enter` / `Space` to activate/select node
 */

import {
  useState,
  type KeyboardEvent,
  type ReactElement,
  type ReactNode,
} from "react";

export interface TreeNodeItem {
  readonly id: string;
  readonly label: string;
  readonly isFolder?: boolean;
  readonly icon?: ReactNode;
  readonly trailingBadge?: ReactNode;
  readonly children?: readonly TreeNodeItem[];
}

export interface FlatVisibleTreeNode {
  readonly node: TreeNodeItem;
  readonly depth: number;
  readonly parentId: string | null;
  readonly isExpanded: boolean;
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

export interface TreeViewProps {
  readonly ariaLabel: string;
  readonly nodes: readonly TreeNodeItem[];
  readonly selectedId: string | null;
  readonly expandedIds: ReadonlySet<string>;
  readonly onSelectNode: (node: TreeNodeItem) => void;
  readonly onToggleExpand: (folderId: string) => void;
}

export function TreeView({
  ariaLabel,
  nodes,
  selectedId,
  expandedIds,
  onSelectNode,
  onToggleExpand,
}: TreeViewProps): ReactElement {
  const visibleRows = flattenVisibleTree(nodes, expandedIds);
  const [focusedId, setFocusedId] = useState<string | null>(
    selectedId ?? visibleRows[0]?.node.id ?? null,
  );

  const handleKeyDown = (
    e: KeyboardEvent<HTMLButtonElement>,
    row: FlatVisibleTreeNode,
    index: number,
  ) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      const next = visibleRows[index + 1];
      if (next) {
        setFocusedId(next.node.id);
      }
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      const prev = visibleRows[index - 1];
      if (prev) {
        setFocusedId(prev.node.id);
      }
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      if (row.node.isFolder) {
        if (!row.isExpanded) {
          onToggleExpand(row.node.id);
        } else if (row.node.children && row.node.children.length > 0) {
          setFocusedId(row.node.children[0].id);
        }
      }
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      if (row.node.isFolder && row.isExpanded) {
        onToggleExpand(row.node.id);
      } else if (row.parentId) {
        setFocusedId(row.parentId);
      }
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (row.node.isFolder) {
        onToggleExpand(row.node.id);
      }
      onSelectNode(row.node);
    }
  };

  return (
    <div role="tree" aria-label={ariaLabel} style={{ width: "100%" }}>
      {visibleRows.map((row, index) => {
        const isSelected = row.node.id === selectedId;
        const isFocused =
          row.node.id === focusedId || (focusedId === null && index === 0);

        return (
          <button
            key={row.node.id}
            type="button"
            role="treeitem"
            aria-selected={isSelected ? "true" : "false"}
            aria-expanded={
              row.node.isFolder ? (row.isExpanded ? "true" : "false") : undefined
            }
            tabIndex={isFocused ? 0 : -1}
            className={`ky-btn ky-btn--ghost ky-btn--sm ${
              isSelected ? "ky-btn--selected" : ""
            }`}
            style={{
              width: "100%",
              justifyContent: "flex-start",
              paddingInlineStart: `calc(var(--space-2) + ${row.depth * 14}px)`,
            }}
            onClick={() => {
              setFocusedId(row.node.id);
              if (row.node.isFolder) {
                onToggleExpand(row.node.id);
              }
              onSelectNode(row.node);
            }}
            onKeyDown={(e) => handleKeyDown(e, row, index)}
          >
            {row.node.icon}
            <span
              style={{
                flex: 1,
                textAlign: "start",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {row.node.label}
            </span>
            {row.node.trailingBadge}
          </button>
        );
      })}
    </div>
  );
}
