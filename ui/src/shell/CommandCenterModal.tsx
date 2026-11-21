/**
 * Global Command Center (`Ctrl/Cmd+K` / `Ctrl+Shift+P`) (UIR-3.4, Blueprint §14 (1), E2E-23).
 *
 * Permission-filtered search & command palette unifying:
 * - Workspace actions (`New task`, `Toggle sidebar`, `Toggle terminal`, `Toggle side pane`, `Open settings`)
 * - Projects & Tasks
 * - Karyora Core surfaces (`Jobs`, `KnowledgePack`, `Memory`, `Context Inspector`, `Workflows`, `Activity & Audit`)
 * - All 21 Settings sections (`#1–#60`)
 */

import { useMemo, useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, ComboboxPopover, ModalDialog } from "../components/primitives";
import type {
  TaskSessionSummary,
  WorkspaceProjectSummary,
} from "../coreclient/types";
import { SETTINGS_SECTIONS, type SettingsSectionId } from "../types/decisions";
import {
  filterCommandCenterEntries,
  type ActiveMainSurface,
  type CommandCenterEntry,
} from "./shellConfig";

export interface CommandCenterModalProps {
  readonly isOpen: boolean;
  readonly canWrite: boolean;
  readonly projects: readonly WorkspaceProjectSummary[];
  readonly tasks: readonly TaskSessionSummary[];
  readonly onClose: () => void;
  readonly onExecuteWorkspaceAction: (actionId: string) => void;
  readonly onNavigateSurface: (surface: ActiveMainSurface) => void;
  readonly onSelectTask: (task: TaskSessionSummary) => void;
  readonly onOpenSettingsSection: (section: SettingsSectionId) => void;
}

export function CommandCenterModal({
  isOpen,
  canWrite,
  projects,
  tasks,
  onClose,
  onExecuteWorkspaceAction,
  onNavigateSurface,
  onSelectTask,
  onOpenSettingsSection,
}: CommandCenterModalProps): ReactElement | null {
  const [query, setQuery] = useState("");

  const allEntries = useMemo<readonly CommandCenterEntry[]>(() => {
    const coreActions: CommandCenterEntry[] = [
      {
        id: "action:new-task",
        category: "workspace-action",
        title: "New task",
        subtitle: "Start a fresh agentic workspace session",
        shortcutHint: "Ctrl+N",
        requiresWritePermission: true,
      },
      {
        id: "action:toggle-sidebar",
        category: "workspace-action",
        title: "Toggle Sidebar",
        shortcutHint: "Ctrl+B",
      },
      {
        id: "action:toggle-terminal",
        category: "workspace-action",
        title: "Toggle Terminal",
        shortcutHint: "Ctrl+J",
      },
      {
        id: "action:toggle-side-pane",
        category: "workspace-action",
        title: "Toggle Side Pane",
        shortcutHint: "Ctrl+Alt+B",
      },
      {
        id: "action:toggle-theme",
        category: "workspace-action",
        title: "Toggle Light/Dark Theme",
        shortcutHint: "Ctrl+Shift+L",
      },
      {
        id: "action:open-migration",
        category: "workspace-action",
        title: "Open 6-Step Migration Guide",
        subtitle: "Sessions, Skills, MCP servers, Commands, AGENTS.md",
      },
      {
        id: "surface:automations",
        category: "workflow",
        title: "Automations & Versioned Workflows",
        targetSurface: "automations",
      },
      {
        id: "surface:marketplace",
        category: "plugin",
        title: "Plugin Marketplace",
        targetSurface: "marketplace",
      },
      {
        id: "surface:jobs",
        category: "job",
        title: "Durable Jobs (AIJobRelay)",
        targetSurface: "jobs",
      },
      {
        id: "surface:knowledge",
        category: "knowledge",
        title: "KnowledgePack Library",
        targetSurface: "knowledge",
      },
      {
        id: "surface:memory",
        category: "memory",
        title: "Scoped Memory Review",
        targetSurface: "memory",
      },
      {
        id: "surface:context",
        category: "workspace-action",
        title: "Context Inspector",
        targetSurface: "context",
      },
      {
        id: "surface:activity",
        category: "workspace-action",
        title: "Activity & Audit Log",
        targetSurface: "activity",
      },
    ];

    const projectEntries: CommandCenterEntry[] = projects.map((p) => ({
      id: `project:${p.projectId}`,
      category: "project",
      title: p.name,
      subtitle: p.path,
    }));

    const taskEntries: CommandCenterEntry[] = tasks.map((t) => ({
      id: `task:${t.taskId}`,
      category: "task",
      title: t.title,
      subtitle: `${t.modelId} · ${t.updatedAt}`,
    }));

    const settingsEntries: CommandCenterEntry[] = SETTINGS_SECTIONS.map((s) => ({
      id: `settings:${s.id}`,
      category: "settings",
      title: `Settings → ${s.label}`,
      subtitle: `Group: ${s.group} (${s.screenshots})`,
      targetSettingsSection: s.id,
    }));

    return [...coreActions, ...projectEntries, ...taskEntries, ...settingsEntries];
  }, [projects, tasks]);

  const filtered = filterCommandCenterEntries(allEntries, query, canWrite);

  return (
    <ModalDialog
      isOpen={isOpen}
      title="Command Center (Ctrl+K)"
      description="Search workspace commands, projects, tasks, Jobs, KnowledgePacks, Workflows, Plugins, and all 21 Settings sections."
      onClose={() => {
        setQuery("");
        onClose();
      }}
    >
      <div data-testid="command-center-palette">
        <ComboboxPopover
          ariaLabel="Search commands and workspace items"
          isOpen
          query={query}
          placeholder="Type a command, task, project, or setting..."
          options={filtered.map((entry) => ({
            id: entry.id,
            label: entry.title,
            description: entry.shortcutHint ?? entry.subtitle,
            icon: (
              <Badge
                tone="neutral"
                label={entry.category}
              />
            ),
          }))}
          onQueryChange={setQuery}
          onDismiss={() => {
            setQuery("");
            onClose();
          }}
          onSelectOption={(opt) => {
            const matched = filtered.find((e) => e.id === opt.id);
            setQuery("");
            onClose();
            if (!matched) return;
            if (matched.targetSettingsSection) {
              onOpenSettingsSection(matched.targetSettingsSection);
            } else if (matched.targetSurface) {
              onNavigateSurface(matched.targetSurface);
            } else if (matched.id.startsWith("task:")) {
              const tId = matched.id.slice("task:".length);
              const targetTask = tasks.find((t) => t.taskId === tId);
              if (targetTask) {
                onSelectTask(targetTask);
              }
            } else if (matched.id.startsWith("action:")) {
              onExecuteWorkspaceAction(matched.id.slice("action:".length));
            }
          }}
        />
        <div
          style={{
            marginTop: "var(--space-2)",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            fontSize: "0.75rem",
            color: "var(--text-muted)",
          }}
        >
          <span style={{ display: "inline-flex", alignItems: "center", gap: "4px" }}>
            <ChromeIcon name="keyboard" size={14} />
            ↑/↓ Navigate · Enter Open · Esc Close
          </span>
          <span>{filtered.length} matching items</span>
        </div>
      </div>
    </ModalDialog>
  );
}
