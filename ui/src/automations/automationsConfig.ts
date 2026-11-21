/**
 * Automations & Versioned Workflows Pure Domain Helpers
 * (UIR-6.1 & UIR-6.2, Screenshots #66–#67, Blueprint §10, §20.5, E2E-11).
 *
 * Pure TypeScript module (import-type only) so both Vite and Node 26 native tests import it directly.
 */

export type AutomationFrequency =
  | "daily"
  | "weekly"
  | "monthly"
  | "custom"
  | "when-idle";

export type AutomationSessionMode = "default" | "plan" | "auto";

export interface ScheduledTaskRunRecord {
  readonly runId: string;
  readonly jobId: string;
  readonly startedAt: string;
  readonly durationLabel: string;
  readonly status: "succeeded" | "running" | "failed" | "cancelled";
  readonly summary: string;
}

export interface ScheduledTaskRecord {
  readonly taskId: string;
  readonly name: string;
  readonly active: boolean;
  readonly frequency: AutomationFrequency;
  readonly scheduleTime: string;
  readonly customCron?: string;
  readonly instructions: string;
  readonly projectFolder: string;
  readonly sessionMode: AutomationSessionMode;
  readonly modelId: string;
  readonly lastRunLabel: string;
  readonly history: readonly ScheduledTaskRunRecord[];
}

export interface AutomationStarterTemplate {
  readonly templateId: string;
  readonly title: string;
  readonly description: string;
  readonly defaultFrequency: AutomationFrequency;
  readonly defaultTime: string;
  readonly defaultMode: AutomationSessionMode;
  readonly defaultInstructions: string;
}

export type WorkflowNodeKind =
  | "trigger"
  | "agent-step"
  | "tool-call"
  | "approval-gate"
  | "condition"
  | "output";

export interface WorkflowNode {
  readonly nodeId: string;
  readonly title: string;
  readonly kind: WorkflowNodeKind;
  readonly description: string;
  readonly configSummary: string;
  readonly x: number;
  readonly y: number;
}

export interface WorkflowEdge {
  readonly edgeId: string;
  readonly fromNodeId: string;
  readonly toNodeId: string;
  readonly conditionLabel?: string;
}

export interface WorkflowRevisionSnapshot {
  readonly revisionId: string;
  readonly versionNumber: number;
  readonly savedAt: string;
  readonly summary: string;
  readonly nodes: readonly WorkflowNode[];
  readonly edges: readonly WorkflowEdge[];
}

export interface WorkflowDefinitionRecord {
  readonly workflowId: string;
  readonly name: string;
  readonly description: string;
  readonly currentRevision: string;
  readonly versionNumber: number;
  readonly nodes: readonly WorkflowNode[];
  readonly edges: readonly WorkflowEdge[];
  readonly revisions: readonly WorkflowRevisionSnapshot[];
  readonly lastJobId?: string;
}

export interface WorkflowValidationIssue {
  readonly severity: "error" | "warning";
  readonly code:
    | "EMPTY_WORKFLOW"
    | "DUPLICATE_NODE_ID"
    | "EMPTY_NODE_TITLE"
    | "BROKEN_EDGE_REF"
    | "SELF_LOOP"
    | "CYCLE_DETECTED"
    | "NO_TRIGGER_NODE";
  readonly message: string;
  readonly nodeId?: string;
}

export interface WorkflowValidationResult {
  readonly valid: boolean;
  readonly issues: readonly WorkflowValidationIssue[];
  readonly topologicalOrder: readonly string[];
}

export interface WorkflowCasSaveResult {
  readonly status: "saved" | "conflict_409";
  readonly newRevision?: string;
  readonly serverRevision?: string;
  readonly conflictMessage?: string;
}

/**
 * Exact 4 Starter Automation Templates (Screenshot #66, Phase 4 Plan §3.1).
 */
export const STARTER_AUTOMATION_TEMPLATES: readonly AutomationStarterTemplate[] = [
  {
    templateId: "tpl-progress-brief",
    title: "Project progress brief",
    description:
      "Summarize recent commits, open milestones, and blockers into a concise daily brief.",
    defaultFrequency: "daily",
    defaultTime: "09:00",
    defaultMode: "plan",
    defaultInstructions:
      "Review repository commits from the last 24 hours, summarize milestone progress, and list open risks.",
  },
  {
    templateId: "tpl-code-health",
    title: "Code health sweep",
    description:
      "Scan modified modules for lint warnings, test regressions, and dead imports.",
    defaultFrequency: "weekly",
    defaultTime: "08:30",
    defaultMode: "default",
    defaultInstructions:
      "Run typecheck, unit tests, and static health diagnostics on active packages and prepare a prioritized fix list.",
  },
  {
    templateId: "tpl-dep-security",
    title: "Dependency security review",
    description:
      "Audit third-party packages and lockfiles for CVEs, secrets, and supply-chain drift.",
    defaultFrequency: "weekly",
    defaultTime: "07:00",
    defaultMode: "plan",
    defaultInstructions:
      "Check Go and npm dependencies with OSV/Trivy, inspect secret hygiene, and flag any HIGH or CRITICAL findings.",
  },
  {
    templateId: "tpl-idle-docs",
    title: "Idle-time doc refresh",
    description:
      "Detect outdated README/architecture notes during system idle windows and draft updates.",
    defaultFrequency: "when-idle",
    defaultTime: "Idle 15m",
    defaultMode: "plan",
    defaultInstructions:
      "Compare recent API and UI module changes against docs/ and draft a non-destructive documentation refresh patch.",
  },
];

export const SCHEDULE_FREQUENCIES: readonly {
  readonly value: AutomationFrequency;
  readonly label: string;
}[] = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "custom", label: "Custom" },
  { value: "when-idle", label: "When idle" },
];

export const INITIAL_SCHEDULED_TASKS: readonly ScheduledTaskRecord[] = [
  {
    taskId: "auto-1",
    name: "Daily UI Contract & Asset Drift Check",
    active: true,
    frequency: "daily",
    scheduleTime: "09:00",
    instructions:
      "Run assets:verify, i18n:parity, types:check, and report any OpenAPI or Material SVG drift.",
    projectFolder: "/home/km-abdullah/karyora_Platform/karyora",
    sessionMode: "default",
    modelId: "inherit-default",
    lastRunLabel: "Succeeded 2h ago",
    history: [
      {
        runId: "run-a1",
        jobId: "job-0003",
        startedAt: "Today 09:00",
        durationLabel: "14s",
        status: "succeeded",
        summary: "1,147 SVGs verified, 2,017 i18n keys matched, 0 OpenAPI drift.",
      },
    ],
  },
];

export const INITIAL_WORKFLOWS: readonly WorkflowDefinitionRecord[] = [
  {
    workflowId: "wf-release-gate",
    name: "Milestone Security & CI Gate",
    description:
      "Core-validated DAG executing typecheck, static security scans, human approval gate, and release receipt.",
    currentRevision: "rev-002",
    versionNumber: 2,
    nodes: [
      {
        nodeId: "n-trigger",
        title: "Milestone Push Trigger",
        kind: "trigger",
        description: "Starts on manual invocation or workspace commit hook.",
        configSummary: "Manual / On-demand",
        x: 36,
        y: 80,
      },
      {
        nodeId: "n-verify",
        title: "Run Typecheck & Unit Suite",
        kind: "tool-call",
        description: "Executes tsc, node --test, and go test -race ./ui/...",
        configSummary: "timeout=120s",
        x: 246,
        y: 80,
      },
      {
        nodeId: "n-approval",
        title: "Human Approval Gate",
        kind: "approval-gate",
        description: "Requires explicit operator confirmation before durable publish.",
        configSummary: "role=Owner|Admin",
        x: 456,
        y: 80,
      },
      {
        nodeId: "n-receipt",
        title: "Emit Verified Receipt",
        kind: "output",
        description: "Persists SHA-256 execution receipt into AIJobRelay & Audit Log.",
        configSummary: "durable=true",
        x: 666,
        y: 80,
      },
    ],
    edges: [
      { edgeId: "e-1", fromNodeId: "n-trigger", toNodeId: "n-verify" },
      { edgeId: "e-2", fromNodeId: "n-verify", toNodeId: "n-approval", conditionLabel: "pass" },
      { edgeId: "e-3", fromNodeId: "n-approval", toNodeId: "n-receipt", conditionLabel: "approved" },
    ],
    revisions: [
      {
        revisionId: "rev-001",
        versionNumber: 1,
        savedAt: "2026-11-21 22:00",
        summary: "Initial 3-node verification graph",
        nodes: [
          {
            nodeId: "n-trigger",
            title: "Milestone Push Trigger",
            kind: "trigger",
            description: "Starts on manual invocation.",
            configSummary: "Manual",
            x: 36,
            y: 80,
          },
          {
            nodeId: "n-verify",
            title: "Run Typecheck & Unit Suite",
            kind: "tool-call",
            description: "Executes tsc and unit tests.",
            configSummary: "timeout=120s",
            x: 246,
            y: 80,
          },
          {
            nodeId: "n-receipt",
            title: "Emit Verified Receipt",
            kind: "output",
            description: "Persists execution receipt.",
            configSummary: "durable=true",
            x: 456,
            y: 80,
          },
        ],
        edges: [
          { edgeId: "e-1", fromNodeId: "n-trigger", toNodeId: "n-verify" },
          { edgeId: "e-2", fromNodeId: "n-verify", toNodeId: "n-receipt" },
        ],
      },
      {
        revisionId: "rev-002",
        versionNumber: 2,
        savedAt: "2026-11-22 00:10",
        summary: "Added explicit human approval gate before receipt",
        nodes: [
          {
            nodeId: "n-trigger",
            title: "Milestone Push Trigger",
            kind: "trigger",
            description: "Starts on manual invocation or workspace commit hook.",
            configSummary: "Manual / On-demand",
            x: 36,
            y: 80,
          },
          {
            nodeId: "n-verify",
            title: "Run Typecheck & Unit Suite",
            kind: "tool-call",
            description: "Executes tsc, node --test, and go test -race ./ui/...",
            configSummary: "timeout=120s",
            x: 246,
            y: 80,
          },
          {
            nodeId: "n-approval",
            title: "Human Approval Gate",
            kind: "approval-gate",
            description: "Requires explicit operator confirmation before durable publish.",
            configSummary: "role=Owner|Admin",
            x: 456,
            y: 80,
          },
          {
            nodeId: "n-receipt",
            title: "Emit Verified Receipt",
            kind: "output",
            description: "Persists SHA-256 execution receipt into AIJobRelay & Audit Log.",
            configSummary: "durable=true",
            x: 666,
            y: 80,
          },
        ],
        edges: [
          { edgeId: "e-1", fromNodeId: "n-trigger", toNodeId: "n-verify" },
          { edgeId: "e-2", fromNodeId: "n-verify", toNodeId: "n-approval", conditionLabel: "pass" },
          { edgeId: "e-3", fromNodeId: "n-approval", toNodeId: "n-receipt", conditionLabel: "approved" },
        ],
      },
    ],
  },
];

/**
 * Validates a workflow DAG prior to saving or launching a durable AIJobRelay Job
 * (Blueprint §10.2 — fails closed on cycles, broken edges, duplicate IDs, or empty titles).
 */
export function validateWorkflowDag(
  nodes: readonly WorkflowNode[],
  edges: readonly WorkflowEdge[],
): WorkflowValidationResult {
  const issues: WorkflowValidationIssue[] = [];

  if (nodes.length === 0) {
    issues.push({
      severity: "error",
      code: "EMPTY_WORKFLOW",
      message: "Workflow must contain at least one step node.",
    });
    return { valid: false, issues, topologicalOrder: [] };
  }

  const nodeIds = new Set<string>();
  let hasTrigger = false;

  for (const node of nodes) {
    if (nodeIds.has(node.nodeId)) {
      issues.push({
        severity: "error",
        code: "DUPLICATE_NODE_ID",
        nodeId: node.nodeId,
        message: `Duplicate node identifier: ${node.nodeId}`,
      });
    }
    nodeIds.add(node.nodeId);

    if (node.title.trim().length === 0) {
      issues.push({
        severity: "error",
        code: "EMPTY_NODE_TITLE",
        nodeId: node.nodeId,
        message: `Node ${node.nodeId} requires a non-empty title.`,
      });
    }
    if (node.kind === "trigger") {
      hasTrigger = true;
    }
  }

  if (!hasTrigger) {
    issues.push({
      severity: "warning",
      code: "NO_TRIGGER_NODE",
      message: "Workflow has no explicit trigger node; execution will start from root nodes.",
    });
  }

  const inDegree = new Map<string, number>();
  const adjacency = new Map<string, string[]>();
  for (const id of nodeIds) {
    inDegree.set(id, 0);
    adjacency.set(id, []);
  }

  for (const edge of edges) {
    if (!nodeIds.has(edge.fromNodeId) || !nodeIds.has(edge.toNodeId)) {
      issues.push({
        severity: "error",
        code: "BROKEN_EDGE_REF",
        message: `Edge ${edge.edgeId} references an unknown node (${edge.fromNodeId} -> ${edge.toNodeId}).`,
      });
      continue;
    }
    if (edge.fromNodeId === edge.toNodeId) {
      issues.push({
        severity: "error",
        code: "SELF_LOOP",
        nodeId: edge.fromNodeId,
        message: `Node ${edge.fromNodeId} cannot connect to itself.`,
      });
      continue;
    }
    adjacency.get(edge.fromNodeId)?.push(edge.toNodeId);
    inDegree.set(edge.toNodeId, (inDegree.get(edge.toNodeId) ?? 0) + 1);
  }

  // Kahn's topological sort for deterministic cycle detection
  const queue: string[] = [];
  for (const [id, deg] of inDegree.entries()) {
    if (deg === 0) queue.push(id);
  }

  const topologicalOrder: string[] = [];
  while (queue.length > 0) {
    const current = queue.shift()!;
    topologicalOrder.push(current);
    for (const neighbor of adjacency.get(current) ?? []) {
      const nextDeg = (inDegree.get(neighbor) ?? 1) - 1;
      inDegree.set(neighbor, nextDeg);
      if (nextDeg === 0) {
        queue.push(neighbor);
      }
    }
  }

  if (topologicalOrder.length !== nodeIds.size) {
    issues.push({
      severity: "error",
      code: "CYCLE_DETECTED",
      message:
        "Workflow graph contains a cycle. Remove cyclic edges before saving or executing.",
    });
  }

  const hasErrors = issues.some((i) => i.severity === "error");
  return {
    valid: !hasErrors,
    issues,
    topologicalOrder: hasErrors ? [] : topologicalOrder,
  };
}

/**
 * Evaluates CAS optimistic concurrency on workflow save (`409 Conflict` guard).
 * Never silently overwrites a newer server revision.
 */
export function evaluateWorkflowCasSave(
  baseRevision: string,
  currentServerRevision: string,
  nextVersionNumber: number,
): WorkflowCasSaveResult {
  if (baseRevision !== currentServerRevision) {
    return {
      status: "conflict_409",
      serverRevision: currentServerRevision,
      conflictMessage: `409 Conflict: workflow revision changed on Core (${baseRevision} -> ${currentServerRevision}). Your draft is preserved; reload or rebase before saving.`,
    };
  }
  return {
    status: "saved",
    newRevision: `rev-${String(nextVersionNumber).padStart(3, "0")}`,
  };
}

/**
 * Reorders workflow steps cleanly for keyboard/non-drag accessibility (`Blueprint §20.5`).
 */
export function reorderWorkflowNodes(
  nodes: readonly WorkflowNode[],
  fromIndex: number,
  toIndex: number,
): readonly WorkflowNode[] {
  if (
    fromIndex < 0 ||
    toIndex < 0 ||
    fromIndex >= nodes.length ||
    toIndex >= nodes.length ||
    fromIndex === toIndex
  ) {
    return nodes;
  }
  const copy = [...nodes];
  const [moved] = copy.splice(fromIndex, 1);
  copy.splice(toIndex, 0, moved);
  return copy.map((node, idx) => ({
    ...node,
    x: 36 + idx * 210,
  }));
}
