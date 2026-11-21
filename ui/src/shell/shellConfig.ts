/**
 * Authoritative v3.0 Application Shell Configuration & Pure Helpers
 * (UIR-3.2, UIR-3.3, UIR-3.4, Screenshots #55, #61–#65, #68, #79–#80, #84–#85).
 *
 * Pure TypeScript module (`import type` only) so both React shell components
 * and Node's native ESM test runner (`node --test`) verify these contracts directly.
 */

import type {
  TaskSessionSummary,
  WorkspaceProjectSummary,
} from "../coreclient/types";
import type {
  AppTheme,
  InterfaceMode,
  SettingsSectionId,
  SupportedLocale,
} from "../types/decisions";

/**
 * Screenshot #85: Exact 13-Item Task Right-Click Context Menu.
 */
export interface TaskContextMenuActionDef {
  readonly id:
    | "move-to-group"
    | "pin"
    | "rename"
    | "mark-as-unread"
    | "archive"
    | "delete"
    | "copy-task-id"
    | "copy-working-directory"
    | "export-conversation"
    | "view-log-file"
    | "open-in-new-window"
    | "view-model-trajectory"
    | "report-issue";
  readonly label: string;
  readonly danger?: boolean;
  readonly separatorBefore?: boolean;
  readonly hasSubmenu?: boolean;
}

export const TASK_CONTEXT_MENU_ITEMS: readonly TaskContextMenuActionDef[] = [
  { id: "move-to-group", label: "Move to group", hasSubmenu: true },
  { id: "pin", label: "Pin" },
  { id: "rename", label: "Rename" },
  { id: "mark-as-unread", label: "Mark as unread" },
  { id: "archive", label: "Archive" },
  { id: "delete", label: "Delete", danger: true },
  { id: "copy-task-id", label: "Copy Task ID", separatorBefore: true },
  { id: "copy-working-directory", label: "Copy working directory" },
  { id: "export-conversation", label: "Export conversation" },
  { id: "view-log-file", label: "View log file" },
  { id: "open-in-new-window", label: "Open in new window" },
  { id: "view-model-trajectory", label: "View model trajectory", separatorBefore: true },
  { id: "report-issue", label: "Report issue" },
] as const;

/**
 * Screenshot #80: Exact 7-Item Top-Bar `?` Help Menu.
 */
export interface HelpMenuEntry {
  readonly id:
    | "product-docs"
    | "best-practices"
    | "changelog"
    | "tutorial-videos"
    | "feedback"
    | "community"
    | "about";
  readonly label: string;
  readonly externalUrl?: string;
  readonly separatorBefore?: boolean;
}

export const HELP_MENU_ITEMS: readonly HelpMenuEntry[] = [
  { id: "product-docs", label: "Product docs", externalUrl: "https://github.com/qira-dev/karyora" },
  { id: "best-practices", label: "Best practices", externalUrl: "https://github.com/qira-dev/karyora" },
  { id: "changelog", label: "Changelog", externalUrl: "https://github.com/qira-dev/karyora/releases" },
  { id: "tutorial-videos", label: "Tutorial videos", externalUrl: "https://github.com/qira-dev/karyora" },
  { id: "feedback", label: "Feedback", separatorBefore: true, externalUrl: "https://github.com/qira-dev/karyora/issues" },
  { id: "community", label: "Community", externalUrl: "https://github.com/qira-dev/karyora/discussions" },
  { id: "about", label: "About", separatorBefore: true },
] as const;

/**
 * Screenshot #55: Exact 6-Step Migration Guide Modal.
 */
export interface MigrationGuideStep {
  readonly stepNumber: 1 | 2 | 3 | 4 | 5 | 6;
  readonly id:
    | "sessions"
    | "skills"
    | "mcp-servers"
    | "commands"
    | "agents-md"
    | "migration";
  readonly label: string;
  readonly summary: string;
}

export const MIGRATION_GUIDE_STEPS: readonly MigrationGuideStep[] = [
  {
    stepNumber: 1,
    id: "sessions",
    label: "Sessions",
    summary: "Import historical task transcripts and workspace folder associations without mutating files on disk.",
  },
  {
    stepNumber: 2,
    id: "skills",
    label: "Skills",
    summary: "Review and import reusable SKILL.md instruction bundles into User or Project scope.",
  },
  {
    stepNumber: 3,
    id: "mcp-servers",
    label: "MCP servers",
    summary: "Migrate stdio, HTTP, and SSE MCP server definitions with write-only secret handling (D-37).",
  },
  {
    stepNumber: 4,
    id: "commands",
    label: "Commands",
    summary: "Import custom slash commands and argument hints into Settings → Commands.",
  },
  {
    stepNumber: 5,
    id: "agents-md",
    label: "AGENTS.md",
    summary: "Inspect and link repository AGENTS.md rulebooks for Subagent context injection.",
  },
  {
    stepNumber: 6,
    id: "migration",
    label: "Migration",
    summary: "Review checksum summary and execute an atomic, reversible local migration.",
  },
] as const;

/**
 * Screenshots #61–#65: Profile Popover 4 Submenus Configuration.
 */
export const PROFILE_LOCALE_OPTIONS: ReadonlyArray<{
  readonly code: "system" | SupportedLocale;
  readonly label: string;
}> = [
  { code: "system", label: "System default" },
  { code: "en", label: "English (en)" },
  { code: "bn", label: "বাংলা (bn)" },
  { code: "zh-CN", label: "简体中文 (zh-CN)" },
  { code: "es", label: "Español (es)" },
  { code: "pt-BR", label: "Português (pt-BR)" },
  { code: "ja", label: "日本語 (ja)" },
  { code: "ru", label: "Русский (ru)" },
  { code: "ko", label: "한국어 (ko)" },
  { code: "de", label: "Deutsch (de)" },
  { code: "fr", label: "Français (fr)" },
  { code: "id", label: "Bahasa Indonesia (id)" },
  { code: "tr", label: "Türkçe (tr)" },
  { code: "vi", label: "Tiếng Việt (vi)" },
] as const;

export const PROFILE_THEME_OPTIONS: ReadonlyArray<{
  readonly value: AppTheme;
  readonly label: string;
}> = [
  { value: "dark", label: "Dark" },
  { value: "light", label: "Light" },
  { value: "system", label: "System" },
] as const;

export const PROFILE_INTERFACE_MODE_OPTIONS: ReadonlyArray<{
  readonly value: InterfaceMode;
  readonly label: string;
  readonly description: string;
}> = [
  {
    value: "coding",
    label: "Coding mode",
    description: "Full IDE workspace chrome, diffs, PTY terminal, and git metadata",
  },
  {
    value: "office",
    label: "Office mode",
    description: "Streamlined document/presentation workflow with proactive task suggestions",
  },
] as const;

export const INTERFACE_ZOOM_MIN = 75;
export const INTERFACE_ZOOM_MAX = 150;
export const INTERFACE_ZOOM_STEP = 10;

export function clampInterfaceZoom(zoomPercent: number): number {
  return Math.max(
    INTERFACE_ZOOM_MIN,
    Math.min(INTERFACE_ZOOM_MAX, Math.round(zoomPercent)),
  );
}

/**
 * Primary Workspace Navigation Surfaces (Blueprint §4.3, §23.4).
 */
export type ActiveMainSurface =
  | "workspace"
  | "automations"
  | "marketplace"
  | "jobs"
  | "knowledge"
  | "memory"
  | "context"
  | "activity";

export type ProjectTaskFilterMode = "all" | "active" | "pinned" | "unread" | "running";
export type ProjectTaskGroupMode = "project" | "flat";

/**
 * Pure filtering and grouping helper for Left Sidebar Projects & Tasks (UIR-3.2).
 */
export function filterTasksForSidebar(
  tasks: readonly TaskSessionSummary[],
  searchQuery: string,
  filterMode: ProjectTaskFilterMode,
): readonly TaskSessionSummary[] {
  const normalizedQuery = searchQuery.trim().toLowerCase();

  return tasks.filter((task) => {
    if (task.status === "archived") {
      return false;
    }
    if (filterMode === "pinned" && !task.isPinned) return false;
    if (filterMode === "unread" && !task.isUnread) return false;
    if (filterMode === "running" && task.status !== "running") return false;
    if (
      filterMode === "active" &&
      task.status !== "running" &&
      task.status !== "waiting-approval"
    ) {
      return false;
    }
    if (normalizedQuery.length > 0) {
      const hay = `${task.title} ${task.taskId} ${task.modelId}`.toLowerCase();
      return hay.includes(normalizedQuery);
    }
    return true;
  });
}

export function filterVisibleSidebarProjects(
  projects: readonly WorkspaceProjectSummary[],
  searchQuery: string,
): readonly WorkspaceProjectSummary[] {
  const normalizedQuery = searchQuery.trim().toLowerCase();
  return projects.filter((proj) => {
    if (proj.isRemovedFromSidebar) {
      return false;
    }
    if (!normalizedQuery) {
      return true;
    }
    const hay = `${proj.name} ${proj.path} ${proj.gitBranch ?? ""}`.toLowerCase();
    return hay.includes(normalizedQuery);
  });
}

/**
 * Global Command Center (`Ctrl/Cmd+K`) Entry Model & Permission-Filtered Search (Blueprint §14).
 */
export type CommandCenterCategory =
  | "workspace-action"
  | "project"
  | "task"
  | "job"
  | "knowledge"
  | "memory"
  | "workflow"
  | "provider"
  | "plugin"
  | "settings";

export interface CommandCenterEntry {
  readonly id: string;
  readonly category: CommandCenterCategory;
  readonly title: string;
  readonly subtitle?: string;
  readonly shortcutHint?: string;
  readonly requiresWritePermission?: boolean;
  readonly requiresConfirmation?: boolean;
  readonly targetSurface?: ActiveMainSurface;
  readonly targetSettingsSection?: SettingsSectionId;
}

export function filterCommandCenterEntries(
  entries: readonly CommandCenterEntry[],
  query: string,
  canWrite: boolean,
): readonly CommandCenterEntry[] {
  const normalized = query.trim().toLowerCase();
  return entries.filter((entry) => {
    if (entry.requiresWritePermission && !canWrite) {
      return false;
    }
    if (!normalized) {
      return true;
    }
    const hay = `${entry.title} ${entry.subtitle ?? ""} ${entry.category}`.toLowerCase();
    return hay.includes(normalized);
  });
}

/**
 * In-App & OS Notification Center Deduplication by Stable Core Event ID (Blueprint §14, UI-34).
 */
export interface CoreNotificationItem {
  readonly eventId: string;
  readonly category:
    | "job"
    | "approval"
    | "provider"
    | "knowledge"
    | "plugin"
    | "core-health";
  readonly title: string;
  readonly privacySafeSummary: string;
  readonly occurredAt: string;
  readonly read: boolean;
  readonly dismissed: boolean;
  readonly targetSurface?: ActiveMainSurface;
  readonly targetEntityId?: string;
}

/**
 * Deduplicates incoming Core notification events by stable `eventId` while
 * preserving existing user `read` / `dismissed` flags.
 */
export function deduplicateNotifications(
  existing: readonly CoreNotificationItem[],
  incoming: readonly CoreNotificationItem[],
): readonly CoreNotificationItem[] {
  const byId = new Map<string, CoreNotificationItem>();
  for (const item of existing) {
    byId.set(item.eventId, item);
  }
  for (const next of incoming) {
    const prev = byId.get(next.eventId);
    if (prev) {
      byId.set(next.eventId, {
        ...next,
        read: prev.read || next.read,
        dismissed: prev.dismissed || next.dismissed,
      });
    } else {
      byId.set(next.eventId, next);
    }
  }
  return Array.from(byId.values()).sort((a, b) =>
    b.occurredAt.localeCompare(a.occurredAt),
  );
}
