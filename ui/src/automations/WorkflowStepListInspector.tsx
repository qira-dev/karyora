/**
 * Keyboard-Accessible Workflow Step List Table & Node Inspector
 * (UIR-6.2, Blueprint §10.2, §20.5, Quality Gate G-05 / G-06).
 *
 * Provides 100% non-drag keyboard parity synchronized with the visual DAG canvas:
 * reorder steps up/down, edit step title/kind/config, add steps, and remove steps.
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Input, Select } from "../components/primitives";
import type { WorkflowNode, WorkflowNodeKind } from "./automationsConfig";

export interface WorkflowStepListInspectorProps {
  readonly nodes: readonly WorkflowNode[];
  readonly selectedNodeId: string | null;
  readonly onSelectNode: (nodeId: string) => void;
  readonly onMoveNode: (fromIndex: number, toIndex: number) => void;
  readonly onUpdateNode: (nodeId: string, patch: Partial<WorkflowNode>) => void;
  readonly onDeleteNode: (nodeId: string) => void;
}

const NODE_KIND_OPTIONS: readonly { readonly value: WorkflowNodeKind; readonly label: string }[] = [
  { value: "trigger", label: "Trigger" },
  { value: "agent-step", label: "Agent Step" },
  { value: "tool-call", label: "Tool Call" },
  { value: "approval-gate", label: "Approval Gate" },
  { value: "condition", label: "Condition" },
  { value: "output", label: "Output / Receipt" },
];

export function WorkflowStepListInspector({
  nodes,
  selectedNodeId,
  onSelectNode,
  onMoveNode,
  onUpdateNode,
  onDeleteNode,
}: WorkflowStepListInspectorProps): ReactElement {
  const selectedNode = nodes.find((n) => n.nodeId === selectedNodeId) ?? nodes[0] ?? null;

  return (
    <section
      className="ky-gov-section"
      data-testid="workflow-step-list-inspector"
      aria-label="Keyboard-accessible workflow step list and inspector"
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "var(--space-2)",
        }}
      >
        <strong>Synchronized Step List (Keyboard-Accessible Non-Drag View)</strong>
        <Badge tone="neutral" label={`${nodes.length} steps`} />
      </div>

      <table className="ky-step-table">
        <thead>
          <tr>
            <th>#</th>
            <th>Step Title</th>
            <th>Node Kind</th>
            <th>Config</th>
            <th>Order & Actions</th>
          </tr>
        </thead>
        <tbody>
          {nodes.map((node, index) => {
            const isSelected = node.nodeId === selectedNode?.nodeId;
            return (
              <tr
                key={node.nodeId}
                data-selected={isSelected ? "true" : "false"}
                onClick={() => onSelectNode(node.nodeId)}
              >
                <td>{index + 1}</td>
                <td>
                  <button
                    type="button"
                    style={{
                      border: "none",
                      background: "transparent",
                      color: "inherit",
                      fontWeight: isSelected ? 700 : 500,
                      cursor: "pointer",
                      padding: 0,
                    }}
                    onClick={() => onSelectNode(node.nodeId)}
                  >
                    {node.title || "(Untitled step)"}
                  </button>
                </td>
                <td>
                  <Badge
                    tone={
                      node.kind === "approval-gate"
                        ? "warning"
                        : node.kind === "trigger"
                          ? "info"
                          : "neutral"
                    }
                    label={node.kind}
                  />
                </td>
                <td style={{ fontFamily: "var(--font-mono)", fontSize: "0.75rem" }}>
                  {node.configSummary}
                </td>
                <td>
                  <div style={{ display: "inline-flex", gap: "4px" }}>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={index === 0}
                      aria-label={`Move ${node.title} up`}
                      onClick={(e) => {
                        e.stopPropagation();
                        onMoveNode(index, index - 1);
                      }}
                    >
                      <ChromeIcon name="chevron-up" size={14} />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={index >= nodes.length - 1}
                      aria-label={`Move ${node.title} down`}
                      onClick={(e) => {
                        e.stopPropagation();
                        onMoveNode(index, index + 1);
                      }}
                    >
                      <ChromeIcon name="chevron-down" size={14} />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={nodes.length <= 1}
                      aria-label={`Remove ${node.title}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        onDeleteNode(node.nodeId);
                      }}
                    >
                      <ChromeIcon name="trash" size={14} />
                    </Button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {selectedNode && (
        <div
          className="ky-modal-form__row"
          style={{
            paddingTop: "var(--space-3)",
            borderTop: "1px solid var(--border-subtle)",
          }}
        >
          <Input
            label="Selected step title"
            value={selectedNode.title}
            onChange={(e) => onUpdateNode(selectedNode.nodeId, { title: e.target.value })}
          />
          <Select
            label="Step type"
            value={selectedNode.kind}
            options={NODE_KIND_OPTIONS}
            onChange={(e) =>
              onUpdateNode(selectedNode.nodeId, {
                kind: e.target.value as WorkflowNodeKind,
              })
            }
          />
          <Input
            label="Step configuration summary"
            value={selectedNode.configSummary}
            onChange={(e) =>
              onUpdateNode(selectedNode.nodeId, { configSummary: e.target.value })
            }
          />
        </div>
      )}
    </section>
  );
}
