/**
 * Unified v3.0 CoreClient Domain & Wire Contract Types (UIR-1.1, UI-26, G-02).
 *
 * Bridges the canonical OpenAPI schema snapshot (`./generated-eval-surface/eval-surface.d.ts`)
 * and the Go Core local HTTP adapter wire types (`./httpLocal/wire.ts`) with the v3.0
 * 4-platform agentic workspace domain contracts (Blueprint §23.2).
 */

import type { components } from "./generated-eval-surface/eval-surface";
import type {
  CommandOutcome,
  InterfaceMode,
  LockedByokProviderId,
  SettingsSectionId,
  SidePaneTabKind,
  SupportedApiFormat,
  SupportedLocale,
  TargetPlatform,
  ToolRowState,
} from "../types/decisions";
import type {
  CoreWireEnvelope,
  KnowledgePack,
  MemoryItemServed,
  Plugin,
  WireAuditRecord,
  WireConnectionView,
  WireConversation,
  WireEventRecord,
  WireJob,
  WireSnapshotView,
  WorkflowDefinition,
} from "./httpLocal/wire";

export type OpenApiComponents = components;

/**
 * Canonical 5-Outcome Command State Wrapper (Blueprint §6.2: S0–S4).
 */
export type CommandState<TData> =
  | { readonly outcome: "S0"; readonly reason?: string }
  | {
      readonly outcome: "S1";
      readonly idempotencyKey: string;
      readonly startedAtMs: number;
    }
  | {
      readonly outcome: "S2";
      readonly data: TData;
      readonly correlationId?: string;
    }
  | {
      readonly outcome: "S3";
      readonly envelope: CoreWireEnvelope;
      readonly recovery: RecoveryPanelDescriptor;
    }
  | {
      readonly outcome: "S4";
      readonly correlationId?: string;
      readonly idempotencyKey?: string;
      readonly recovery: RecoveryPanelDescriptor;
    };

/**
 * Standardized 5-Element RecoveryPanel Descriptor (Blueprint §21.2).
 */
export interface RecoveryPanelDescriptor {
  readonly stateKind:
    | "error"
    | "offline"
    | "auth-required"
    | "permission-denied"
    | "rate-limited"
    | "conflict"
    | "unknown-outcome-s4";
  readonly titleI18nKey: string;
  readonly explanationI18nKey: string;
  readonly draftSafetyI18nKey: string;
  readonly primaryAction:
    | "retry"
    | "reconcile-s4"
    | "open-model-settings"
    | "switch-core-connection"
    | "reauthenticate"
    | "resolve-conflict"
    | "dismiss";
  readonly errorCode: string;
  readonly retryable: boolean;
  readonly correlationId: string;
  readonly httpStatus?: number;
}

/**
 * Core Bootstrap & Connection Health Status (Blueprint §3.3, §4.1).
 */
export interface CoreBootstrapStatus {
  readonly connectionId: string;
  readonly mode: "local" | "remote";
  readonly status: "connected" | "degraded" | "offline" | "auth-required";
  readonly coreVersion: string;
  readonly sessionAuthenticated: boolean;
  readonly activeRole?: "owner" | "admin" | "member" | "viewer";
}

/**
 * Workspace Project & Task Session Summary (Left Sidebar #68, #84, #85).
 */
export interface WorkspaceProjectSummary {
  readonly projectId: string;
  readonly name: string;
  readonly path: string;
  readonly deviceName: string;
  readonly isRemoteDevice: boolean;
  readonly gitBranch?: string;
  readonly sessionCount: number;
  readonly lastActiveAt: string;
  readonly isRemovedFromSidebar: boolean;
}

export interface TaskSessionSummary {
  readonly taskId: string;
  readonly projectId: string;
  readonly title: string;
  readonly status: "idle" | "running" | "waiting-approval" | "error" | "archived";
  readonly isPinned: boolean;
  readonly isUnread: boolean;
  readonly sessionMode: "default" | "plan" | "auto";
  readonly modelId: string;
  readonly thoughtLevel: "low" | "medium" | "high";
  readonly logEpoch: number;
  readonly updatedAt: string;
  readonly diffAdditions: number;
  readonly diffDeletions: number;
  readonly changedFilesCount: number;
}

/**
 * Live Conversation Timeline Turn & 6-State Tool Execution Row (Blueprint §8.1, §20.4).
 */
export interface TimelineToolExecutionRow {
  readonly toolCallId: string;
  readonly toolName: string;
  readonly actionVerb:
    | "Viewed"
    | "Edited"
    | "Created"
    | "Changed"
    | "Deleted"
    | "Ran command"
    | "Searched"
    | "MCP Tool";
  readonly state: ToolRowState;
  readonly targetFilePath?: string;
  readonly diffAdditions?: number;
  readonly diffDeletions?: number;
  readonly mcpServerName?: string;
  readonly commandLine?: string;
  readonly outputPreview?: string;
  readonly startedAt: string;
  readonly completedAt?: string;
}

/**
 * Agent Capabilities Configuration Item (Settings Group 2 #23–#53).
 */
export type AgentCapabilityKind =
  | "subagents"
  | "plugins"
  | "mcp-servers"
  | "skills"
  | "commands"
  | "hooks";

export interface AgentCapabilityItem {
  readonly id: string;
  readonly kind: AgentCapabilityKind;
  readonly name: string;
  readonly scope: "user" | "project";
  readonly enabled: boolean;
  readonly sourcePlugin?: string;
  readonly revision: string;
  readonly config: Readonly<Record<string, unknown>>;
}

export type {
  CommandOutcome,
  CoreWireEnvelope,
  InterfaceMode,
  KnowledgePack,
  LockedByokProviderId,
  MemoryItemServed,
  Plugin,
  SettingsSectionId,
  SidePaneTabKind,
  SupportedApiFormat,
  SupportedLocale,
  TargetPlatform,
  ToolRowState,
  WireAuditRecord,
  WireConnectionView,
  WireConversation,
  WireEventRecord,
  WireJob,
  WireSnapshotView,
  WorkflowDefinition,
};
