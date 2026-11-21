/**
 * `Automations | Workflows` Dual Surface (UIR-6.1 & UIR-6.2, Screenshots #66–#67, Blueprint §10).
 *
 * Implements:
 * - Dual segmented header (`Automations` | `Workflows`) + primary `+ New scheduled task` / `+ New workflow`
 * - `Automations` list + exact 4 starter templates (`Project progress brief`, `Code health sweep`,
 *   `Dependency security review`, `Idle-time doc refresh`)
 * - `ScheduledTaskModal` (`#67`)
 * - `Workflows` DAG builder + keyboard-accessible `WorkflowStepListInspector`
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Toggle } from "../components/primitives";
import {
  STARTER_AUTOMATION_TEMPLATES,
  type AutomationStarterTemplate,
  type ScheduledTaskRecord,
  type WorkflowDefinitionRecord,
} from "./automationsConfig";
import { ScheduledTaskModal } from "./ScheduledTaskModal";
import { WorkflowGraphBuilder } from "./WorkflowGraphBuilder";

export interface AutomationsAndWorkflowsPageProps {
  readonly scheduledTasks: readonly ScheduledTaskRecord[];
  readonly workflows: readonly WorkflowDefinitionRecord[];
  readonly availableModels: readonly string[];
  readonly onSaveScheduledTask: (task: ScheduledTaskRecord) => void;
  readonly onToggleScheduledTaskActive: (taskId: string, active: boolean) => void;
  readonly onDeleteScheduledTask: (taskId: string) => void;
  readonly onRunScheduledTaskNow: (task: ScheduledTaskRecord) => void;
  readonly onSaveWorkflowRevision: (updated: WorkflowDefinitionRecord) => void;
  readonly onCreateWorkflow: () => void;
  readonly onLaunchWorkflowJob: (workflow: WorkflowDefinitionRecord) => void;
  readonly onPickProjectFolder: () => void;
  readonly onOpenJobHistory?: (jobId: string) => void;
}

export function AutomationsAndWorkflowsPage({
  scheduledTasks,
  workflows,
  availableModels,
  onSaveScheduledTask,
  onToggleScheduledTaskActive,
  onDeleteScheduledTask,
  onRunScheduledTaskNow,
  onSaveWorkflowRevision,
  onCreateWorkflow,
  onLaunchWorkflowJob,
  onPickProjectFolder,
  onOpenJobHistory,
}: AutomationsAndWorkflowsPageProps): ReactElement {
  const [activeTab, setActiveTab] = useState<"automations" | "workflows">("automations");
  const [modalOpen, setModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<ScheduledTaskRecord | null>(null);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string>(
    workflows[0]?.workflowId ?? "",
  );

  const selectedWorkflow =
    workflows.find((w) => w.workflowId === selectedWorkflowId) ?? workflows[0] ?? null;

  const openFromTemplate = (tpl: AutomationStarterTemplate) => {
    setEditingTask({
      taskId: `draft-${tpl.templateId}`,
      name: tpl.title,
      active: true,
      frequency: tpl.defaultFrequency,
      scheduleTime: tpl.defaultTime,
      instructions: tpl.defaultInstructions,
      projectFolder: "/home/km-abdullah/karyora_Platform/karyora",
      sessionMode: tpl.defaultMode,
      modelId: "inherit-default",
      lastRunLabel: "Template draft",
      history: [],
    });
    setModalOpen(true);
  };

  return (
    <section
      className="ky-domain-page"
      data-testid="automations-and-workflows-page"
      aria-label="Automations and Workflows"
    >
      <header className="ky-domain-page__header">
        <div className="ky-domain-page__title-group">
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
            <div className="ky-segmented-tabs" role="tablist" aria-label="Automations or Workflows">
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === "automations"}
                className="ky-segmented-tabs__btn"
                data-active={activeTab === "automations" ? "true" : "false"}
                onClick={() => setActiveTab("automations")}
              >
                Automations
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === "workflows"}
                className="ky-segmented-tabs__btn"
                data-active={activeTab === "workflows" ? "true" : "false"}
                onClick={() => setActiveTab("workflows")}
              >
                Workflows
              </button>
            </div>
          </div>
          <p className="ky-domain-page__subtitle">
            {activeTab === "automations"
              ? "Schedule recurring or idle-time tasks executed as durable Karyora Jobs."
              : "Author Core-validated versioned DAG workflows with full keyboard step parity."}
          </p>
        </div>

        <div>
          {activeTab === "automations" ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setEditingTask(null);
                setModalOpen(true);
              }}
            >
              <ChromeIcon name="plus" size={14} />
              <span>New scheduled task</span>
            </Button>
          ) : (
            <Button variant="primary" size="sm" onClick={onCreateWorkflow}>
              <ChromeIcon name="plus" size={14} />
              <span>New workflow</span>
            </Button>
          )}
        </div>
      </header>

      {activeTab === "automations" ? (
        <>
          <div className="ky-auto-list" data-testid="scheduled-tasks-list">
            {scheduledTasks.map((task) => (
              <article key={task.taskId} className="ky-auto-card">
                <div className="ky-auto-card__info">
                  <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                    <strong>{task.name}</strong>
                    <Badge
                      tone={task.active ? "success" : "neutral"}
                      label={task.active ? "Active" : "Paused"}
                    />
                  </div>
                  <div className="ky-auto-card__meta">
                    <span>
                      {task.frequency} · {task.customCron ?? task.scheduleTime}
                    </span>
                    <span>Mode: {task.sessionMode}</span>
                    <span>Model: {task.modelId}</span>
                    <span>{task.lastRunLabel}</span>
                  </div>
                </div>

                <div className="ky-auto-card__actions">
                  <Toggle
                    label="Active"
                    checked={task.active}
                    onChange={(next) => onToggleScheduledTaskActive(task.taskId, next)}
                  />
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => onRunScheduledTaskNow(task)}
                  >
                    <ChromeIcon name="run" size={14} />
                    <span>Run now</span>
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setEditingTask(task);
                      setModalOpen(true);
                    }}
                  >
                    <ChromeIcon name="edit" size={14} />
                    <span>Edit</span>
                  </Button>
                </div>
              </article>
            ))}
          </div>

          <div>
            <h3 style={{ margin: "0 0 var(--space-2) 0", fontSize: "0.9rem" }}>
              Starter Templates
            </h3>
            <div className="ky-template-grid" data-testid="automation-starter-templates">
              {STARTER_AUTOMATION_TEMPLATES.map((tpl) => (
                <button
                  key={tpl.templateId}
                  type="button"
                  className="ky-template-card"
                  onClick={() => openFromTemplate(tpl)}
                >
                  <span className="ky-template-card__title">{tpl.title}</span>
                  <span className="ky-template-card__desc">{tpl.description}</span>
                  <Badge
                    tone="neutral"
                    label={`${tpl.defaultFrequency} · ${tpl.defaultTime}`}
                  />
                </button>
              ))}
            </div>
          </div>
        </>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          {workflows.length > 1 && (
            <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
              {workflows.map((wf) => (
                <Button
                  key={wf.workflowId}
                  size="sm"
                  variant={wf.workflowId === selectedWorkflow?.workflowId ? "primary" : "secondary"}
                  onClick={() => setSelectedWorkflowId(wf.workflowId)}
                >
                  {wf.name} (v{wf.versionNumber})
                </Button>
              ))}
            </div>
          )}

          {selectedWorkflow && (
            <WorkflowGraphBuilder
              key={selectedWorkflow.workflowId}
              workflow={selectedWorkflow}
              onSaveRevision={onSaveWorkflowRevision}
              onLaunchWorkflowJob={onLaunchWorkflowJob}
            />
          )}
        </div>
      )}

      {modalOpen && (
        <ScheduledTaskModal
          isOpen={modalOpen}
          initialTask={editingTask}
          availableModels={availableModels}
          onSave={onSaveScheduledTask}
          onDelete={onDeleteScheduledTask}
          onPickProjectFolder={onPickProjectFolder}
          onOpenJobHistory={onOpenJobHistory}
          onClose={() => setModalOpen(false)}
        />
      )}
    </section>
  );
}
