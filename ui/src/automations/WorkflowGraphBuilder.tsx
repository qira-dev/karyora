/**
 * Versioned Workflow DAG Graph Builder (UIR-6.2, Blueprint §10.2, §20.5, E2E-11).
 *
 * Features:
 * - Interactive SVG DAG Canvas synchronized with keyboard-accessible `WorkflowStepListInspector`
 * - Pre-execution Core schema validation (fails closed on cycles, broken edges, empty titles)
 * - Version history selector (`v1`, `v2`, ...)
 * - CAS `baseRevision` optimistic concurrency guard (`409 Conflict` preserves draft!)
 * - One-click launch creating a durable `AIJobRelay` Job
 */

import { useMemo, useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Select } from "../components/primitives";
import {
  evaluateWorkflowCasSave,
  reorderWorkflowNodes,
  validateWorkflowDag,
  type WorkflowDefinitionRecord,
  type WorkflowEdge,
  type WorkflowNode,
} from "./automationsConfig";
import { WorkflowStepListInspector } from "./WorkflowStepListInspector";

export interface WorkflowGraphBuilderProps {
  readonly workflow: WorkflowDefinitionRecord;
  readonly onSaveRevision: (updated: WorkflowDefinitionRecord) => void;
  readonly onLaunchWorkflowJob: (workflow: WorkflowDefinitionRecord) => void;
}

export function WorkflowGraphBuilder({
  workflow,
  onSaveRevision,
  onLaunchWorkflowJob,
}: WorkflowGraphBuilderProps): ReactElement {
  const [nodes, setNodes] = useState<readonly WorkflowNode[]>(workflow.nodes);
  const [edges, setEdges] = useState<readonly WorkflowEdge[]>(workflow.edges);
  const [baseRevision, setBaseRevision] = useState<string>(workflow.currentRevision);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(
    workflow.nodes[0]?.nodeId ?? null,
  );
  const [conflictMessage, setConflictMessage] = useState<string | null>(null);

  const validation = useMemo(() => validateWorkflowDag(nodes, edges), [nodes, edges]);

  const nodeMap = useMemo(() => {
    const map = new Map<string, WorkflowNode>();
    for (const n of nodes) map.set(n.nodeId, n);
    return map;
  }, [nodes]);

  const handleAddStep = () => {
    const lastNode = nodes[nodes.length - 1];
    const nextId = `n-step-${Date.now()}`;
    const newNode: WorkflowNode = {
      nodeId: nextId,
      title: `Step ${nodes.length + 1}`,
      kind: "agent-step",
      description: "New governed workflow step",
      configSummary: "timeout=60s",
      x: 36 + nodes.length * 210,
      y: 80,
    };
    setNodes((prev) => [...prev, newNode]);
    if (lastNode) {
      setEdges((prev) => [
        ...prev,
        {
          edgeId: `e-${Date.now()}`,
          fromNodeId: lastNode.nodeId,
          toNodeId: nextId,
        },
      ]);
    }
    setSelectedNodeId(nextId);
  };

  const handleSave = () => {
    if (!validation.valid) return;
    const nextVersion = workflow.versionNumber + 1;
    const cas = evaluateWorkflowCasSave(
      baseRevision,
      workflow.currentRevision,
      nextVersion,
    );
    if (cas.status === "conflict_409") {
      setConflictMessage(cas.conflictMessage ?? "409 Conflict detected.");
      return;
    }

    const newRev = cas.newRevision ?? `rev-00${nextVersion}`;
    setConflictMessage(null);
    setBaseRevision(newRev);
    onSaveRevision({
      ...workflow,
      currentRevision: newRev,
      versionNumber: nextVersion,
      nodes,
      edges,
      revisions: [
        ...workflow.revisions,
        {
          revisionId: newRev,
          versionNumber: nextVersion,
          savedAt: "Just now",
          summary: `Saved ${nodes.length} steps (${edges.length} edges)`,
          nodes,
          edges,
        },
      ],
    });
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
      <div className="ky-workflow-toolbar" data-testid="workflow-graph-builder">
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <ChromeIcon name="workflow" size={18} />
          <strong>{workflow.name}</strong>
          <Badge tone="info" label={`v${workflow.versionNumber} · ${workflow.currentRevision}`} />
          <Select
            aria-label="Workflow version history"
            value={baseRevision}
            options={workflow.revisions.map((r) => ({
              value: r.revisionId,
              label: `v${r.versionNumber} (${r.revisionId})`,
            }))}
            onChange={(e) => {
              const target = workflow.revisions.find((r) => r.revisionId === e.target.value);
              if (target) {
                setNodes(target.nodes);
                setEdges(target.edges);
                setBaseRevision(target.revisionId);
                setConflictMessage(null);
              }
            }}
          />
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Button size="sm" variant="secondary" onClick={handleAddStep}>
            <ChromeIcon name="plus" size={14} />
            <span>Add step</span>
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={!validation.valid}
            onClick={handleSave}
          >
            Save revision
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!validation.valid}
            onClick={() =>
              onLaunchWorkflowJob({
                ...workflow,
                nodes,
                edges,
              })
            }
          >
            <ChromeIcon name="run" size={14} />
            <span>Run as Durable Job</span>
          </Button>
        </div>
      </div>

      {conflictMessage && (
        <div
          className="ky-workflow-conflict-banner"
          role="alert"
          data-testid="workflow-cas-conflict-banner"
        >
          <span>{conflictMessage}</span>
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setNodes(workflow.nodes);
                setEdges(workflow.edges);
                setBaseRevision(workflow.currentRevision);
                setConflictMessage(null);
              }}
            >
              Reload latest revision
            </Button>
            <Button
              size="sm"
              variant="primary"
              onClick={() => {
                setBaseRevision(workflow.currentRevision);
                setConflictMessage(null);
              }}
            >
              Keep draft & rebase
            </Button>
          </div>
        </div>
      )}

      <div
        className="ky-workflow-validation-banner"
        data-valid={validation.valid ? "true" : "false"}
        data-testid="workflow-validation-banner"
      >
        <span>
          {validation.valid
            ? `Core DAG schema valid · Topological order: ${validation.topologicalOrder.join(" → ")}`
            : `Validation failed (${validation.issues.length} issue(s)): ${validation.issues
                .map((i) => i.message)
                .join(" | ")}`}
        </span>
        <Badge
          tone={validation.valid ? "success" : "danger"}
          label={validation.valid ? "DAG Valid" : "Execution Blocked"}
        />
      </div>

      {/* Interactive SVG DAG Canvas */}
      <div className="ky-workflow-canvas-wrap" aria-label="Workflow DAG visual canvas">
        <svg
          className="ky-workflow-svg"
          viewBox={`0 0 ${Math.max(820, nodes.length * 220 + 60)} 210`}
          role="img"
          aria-label={`Workflow graph for ${workflow.name}`}
        >
          <defs>
            <marker
              id="ky-arrow"
              markerWidth="8"
              markerHeight="8"
              refX="7"
              refY="4"
              orient="auto"
            >
              <path d="M 0 0 L 8 4 L 0 8 z" fill="currentColor" />
            </marker>
          </defs>

          {edges.map((edge) => {
            const from = nodeMap.get(edge.fromNodeId);
            const to = nodeMap.get(edge.toNodeId);
            if (!from || !to) return null;
            const x1 = from.x + 170;
            const y1 = from.y + 34;
            const x2 = to.x;
            const y2 = to.y + 34;
            return (
              <g key={edge.edgeId} style={{ color: "var(--text-muted)" }}>
                <line
                  x1={x1}
                  y1={y1}
                  x2={x2}
                  y2={y2}
                  stroke="currentColor"
                  strokeWidth="2"
                  markerEnd="url(#ky-arrow)"
                />
                {edge.conditionLabel && (
                  <text
                    x={(x1 + x2) / 2}
                    y={y1 - 8}
                    textAnchor="middle"
                    fontSize="11"
                    fill="currentColor"
                  >
                    {edge.conditionLabel}
                  </text>
                )}
              </g>
            );
          })}

          {nodes.map((node) => {
            const selected = node.nodeId === selectedNodeId;
            return (
              <g
                key={node.nodeId}
                transform={`translate(${node.x}, ${node.y})`}
                style={{ cursor: "pointer" }}
                onClick={() => setSelectedNodeId(node.nodeId)}
              >
                <rect
                  width="170"
                  height="68"
                  rx="10"
                  fill="var(--bg-canvas)"
                  stroke={selected ? "var(--accent-primary)" : "var(--border-default)"}
                  strokeWidth={selected ? "2" : "1"}
                />
                <text
                  x="12"
                  y="24"
                  fontSize="12"
                  fontWeight="700"
                  fill="var(--text-primary)"
                >
                  {node.title.slice(0, 20)}
                </text>
                <text x="12" y="42" fontSize="10" fill="var(--text-muted)">
                  {node.kind} · {node.configSummary.slice(0, 18)}
                </text>
              </g>
            );
          })}
        </svg>
      </div>

      {/* Synchronized Keyboard-Accessible Step List Inspector (§20.5) */}
      <WorkflowStepListInspector
        nodes={nodes}
        selectedNodeId={selectedNodeId}
        onSelectNode={setSelectedNodeId}
        onMoveNode={(from, to) => setNodes((prev) => reorderWorkflowNodes(prev, from, to))}
        onUpdateNode={(nodeId, patch) =>
          setNodes((prev) =>
            prev.map((n) => (n.nodeId === nodeId ? { ...n, ...patch } : n)),
          )
        }
        onDeleteNode={(nodeId) => {
          setNodes((prev) => prev.filter((n) => n.nodeId !== nodeId));
          setEdges((prev) =>
            prev.filter((e) => e.fromNodeId !== nodeId && e.toNodeId !== nodeId),
          );
        }}
      />
    </div>
  );
}
