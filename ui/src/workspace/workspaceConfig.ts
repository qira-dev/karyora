/**
 * Pure TypeScript Agentic Workspace, Composer, Timeline & Durable Jobs Helpers
 * (UIR-4.1, UIR-4.2, UIR-4.3, Screenshots #68–#77, #79, #84, Blueprint §7–§8, §12.1).
 *
 * Uses type-only imports (`import type`) so both Vite/React components and
 * Node's native ESM TypeScript test runner (`node --test`) execute directly.
 */

import type {
  TimelineToolExecutionRow,
  ToolRowState,
} from "../coreclient/types";

/**
 * Screenshot #68: Time-of-day Greeting Helper.
 */
export function resolveTimeOfDayGreeting(hour24: number): string {
  const normalized = ((Math.floor(hour24) % 24) + 24) % 24;
  if (normalized < 12) return "Good morning";
  if (normalized < 18) return "Good afternoon";
  return "Good evening";
}

/**
 * Screenshot #68: Exact 4 Starter Quick Chips in New-Task Greeting State.
 */
export interface StarterQuickChip {
  readonly id:
    | "quick-start"
    | "create-presentation"
    | "build-website"
    | "scheduled-tasks";
  readonly label: string;
  readonly promptTemplate: string;
}

export const STARTER_QUICK_CHIPS: readonly StarterQuickChip[] = [
  {
    id: "quick-start",
    label: "Quick Start",
    promptTemplate: "Inspect this workspace, summarize architecture, and propose the next milestone.",
  },
  {
    id: "create-presentation",
    label: "Create a Presentation",
    promptTemplate: "Draft an executive slide deck outline and export PPTX preview artifacts.",
  },
  {
    id: "build-website",
    label: "Build a Good Website",
    promptTemplate: "Design and build a responsive, WCAG 2.2 AA web interface with live preview.",
  },
  {
    id: "scheduled-tasks",
    label: "Scheduled Tasks",
    promptTemplate: "Configure an automated scheduled code health and dependency security review.",
  },
] as const;

/**
 * Screenshot #68: `+` Add Context Menu Built-In Plugin Toggles.
 */
export const ADD_CONTEXT_PLUGINS = [
  { id: "plugin-documents", label: "Documents" },
  { id: "plugin-pdf", label: "PDF" },
  { id: "plugin-presentations", label: "Presentations" },
  { id: "plugin-spreadsheets", label: "Spreadsheets" },
] as const;

export type ComposerTriggerChar = "@" | "/" | "#" | "$";

export interface DetectedComposerTrigger {
  readonly trigger: ComposerTriggerChar;
  readonly query: string;
  readonly startIndex: number;
  readonly endIndex: number;
}

/**
 * Detects an active inline trigger (`@`, `/`, `#`, `$`) at the caret position in the Composer.
 */
export function detectComposerTrigger(
  text: string,
  caretIndex: number,
): DetectedComposerTrigger | null {
  const safeCaret = Math.max(0, Math.min(caretIndex, text.length));
  const beforeCaret = text.slice(0, safeCaret);
  const match = /(?:^|\s)([@/#$])([^\s]*)$/.exec(beforeCaret);
  if (!match) {
    return null;
  }
  const trigger = match[1] as ComposerTriggerChar;
  const query = match[2] ?? "";
  const triggerPos = beforeCaret.length - query.length - 1;
  return {
    trigger,
    query,
    startIndex: triggerPos,
    endIndex: safeCaret,
  };
}

export const COMPOSER_SESSION_MODES = ["default", "plan", "auto"] as const;
export type ComposerSessionMode = (typeof COMPOSER_SESSION_MODES)[number];

export const COMPOSER_THOUGHT_LEVELS = ["low", "medium", "high"] as const;
export type ComposerThoughtLevel = (typeof COMPOSER_THOUGHT_LEVELS)[number];

export function cycleValue<T>(list: readonly T[], current: T): T {
  const idx = list.indexOf(current);
  return list[(idx + 1) % list.length];
}

/**
 * Blueprint §20.4: Valid Transitions of the 6-State Tool Execution Row Machine.
 */
const VALID_TOOL_TRANSITIONS: Readonly<Record<ToolRowState, readonly ToolRowState[]>> = {
  inputStreaming: ["pendingApproval", "running", "error", "cancelled"],
  pendingApproval: ["running", "cancelled", "error"],
  running: ["success", "error", "cancelled"],
  success: [],
  error: [],
  cancelled: [],
};

export function isValidToolRowTransition(
  from: ToolRowState,
  to: ToolRowState,
): boolean {
  return VALID_TOOL_TRANSITIONS[from].includes(to);
}

/**
 * Conversation Timeline Turn Data Model (Screenshots #69–#85).
 */
export interface TimelineProgressStep {
  readonly stepId: string;
  readonly label: string;
  readonly completed: boolean;
}

export interface TimelineSubagentBadge {
  readonly subagentId: string;
  readonly name: string;
  readonly status: "running" | "ended" | "error";
  readonly elapsedLabel: string;
}

export interface ConversationTurnRecord {
  readonly turnId: string;
  readonly role: "user" | "assistant";
  readonly timestamp: string;
  readonly text: string;
  readonly thinkingText?: string;
  readonly isThinkingStreaming?: boolean;
  readonly toolRows?: readonly TimelineToolExecutionRow[];
  readonly pendingApproval?: {
    readonly approvalId: string;
    readonly toolCallId: string;
    readonly commandOrTarget: string;
    readonly scopeLabel: string;
    readonly riskLevel: "low" | "medium" | "high";
    readonly diffPreview?: string;
    readonly status: "pending" | "approving" | "approved" | "rejected";
  };
}

/**
 * Durable Jobs (`AIJobRelay` — Blueprint §12.1) Record, Filter & Virtualization Window.
 */
export type DurableJobStatus =
  | "queued"
  | "running"
  | "waiting_approval"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "reconciling_s4";

export interface DurableJobRecord {
  readonly jobId: string;
  readonly title: string;
  readonly jobType: "agent-task" | "workflow-run" | "knowledge-ingest" | "automation";
  readonly projectName: string;
  readonly status: DurableJobStatus;
  readonly attemptCount: number;
  readonly checkpointLabel: string;
  readonly updatedAt: string;
  readonly idempotencyKey: string;
  readonly correlationId: string;
  readonly receiptDigest?: string;
  readonly events: readonly {
    readonly seq: number;
    readonly timestamp: string;
    readonly kind: string;
    readonly summary: string;
  }[];
}

export function filterDurableJobs(
  jobs: readonly DurableJobRecord[],
  searchQuery: string,
  statusFilter: "all" | DurableJobStatus,
  typeFilter: "all" | DurableJobRecord["jobType"],
): readonly DurableJobRecord[] {
  const q = searchQuery.trim().toLowerCase();
  return jobs.filter((job) => {
    if (statusFilter !== "all" && job.status !== statusFilter) return false;
    if (typeFilter !== "all" && job.jobType !== typeFilter) return false;
    if (q.length > 0) {
      const hay = `${job.jobId} ${job.title} ${job.projectName} ${job.correlationId}`.toLowerCase();
      return hay.includes(q);
    }
    return true;
  });
}

export interface VirtualWindowSlice {
  readonly startIndex: number;
  readonly endIndex: number;
  readonly offsetTopPx: number;
  readonly totalHeightPx: number;
}

/**
 * Pure O(1) virtualized window calculator for 10,000+ Durable Jobs and 100,000+ Audit rows (G-13).
 */
export function computeVirtualWindow(
  totalCount: number,
  scrollTopPx: number,
  rowHeightPx: number,
  viewportHeightPx: number,
  overscan = 6,
): VirtualWindowSlice {
  if (totalCount <= 0 || rowHeightPx <= 0) {
    return { startIndex: 0, endIndex: 0, offsetTopPx: 0, totalHeightPx: 0 };
  }
  const visibleCount = Math.ceil(viewportHeightPx / rowHeightPx);
  const rawStart = Math.floor(Math.max(0, scrollTopPx) / rowHeightPx);
  const startIndex = Math.max(0, rawStart - overscan);
  const endIndex = Math.min(totalCount, rawStart + visibleCount + overscan);
  return {
    startIndex,
    endIndex,
    offsetTopPx: startIndex * rowHeightPx,
    totalHeightPx: totalCount * rowHeightPx,
  };
}
