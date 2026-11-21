/**
 * `New scheduled task` / Edit Automation Modal (UIR-6.1, Screenshot #67, Blueprint §10.1).
 *
 * Implements:
 * - Segmented `Settings` | `History` tabs
 * - `Task name`, `Active` toggle, `Frequency` (`Daily`/`Weekly`/`Monthly`/`Custom`/`When idle`), `Time`
 * - `Instructions` prompt box
 * - Bottom selector pills (`Select a project folder`, `Default`/`Plan`/`Auto` mode, `Inherit default` model)
 * - `Delete` (danger + ConfirmDialog), `Cancel`, `Save`
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import {
  Badge,
  Button,
  ConfirmDialog,
  Input,
  ModalDialog,
  Select,
  Toggle,
} from "../components/primitives";
import {
  SCHEDULE_FREQUENCIES,
  type AutomationFrequency,
  type AutomationSessionMode,
  type ScheduledTaskRecord,
} from "./automationsConfig";

export interface ScheduledTaskModalProps {
  readonly isOpen: boolean;
  readonly initialTask: ScheduledTaskRecord | null;
  readonly availableModels: readonly string[];
  readonly onSave: (task: ScheduledTaskRecord) => void;
  readonly onDelete?: (taskId: string) => void;
  readonly onPickProjectFolder: () => void;
  readonly onOpenJobHistory?: (jobId: string) => void;
  readonly onClose: () => void;
}

export function ScheduledTaskModal({
  isOpen,
  initialTask,
  availableModels,
  onSave,
  onDelete,
  onPickProjectFolder,
  onOpenJobHistory,
  onClose,
}: ScheduledTaskModalProps): ReactElement | null {
  const [modalTab, setModalTab] = useState<"settings" | "history">("settings");
  const [name, setName] = useState(initialTask?.name ?? "");
  const [active, setActive] = useState(initialTask?.active ?? true);
  const [frequency, setFrequency] = useState<AutomationFrequency>(
    initialTask?.frequency ?? "daily",
  );
  const [scheduleTime, setScheduleTime] = useState(initialTask?.scheduleTime ?? "09:00");
  const [customCron, setCustomCron] = useState(initialTask?.customCron ?? "0 9 * * 1-5");
  const [instructions, setInstructions] = useState(initialTask?.instructions ?? "");
  const [projectFolder, setProjectFolder] = useState(
    initialTask?.projectFolder ?? "/home/km-abdullah/karyora_Platform/karyora",
  );
  const [sessionMode, setSessionMode] = useState<AutomationSessionMode>(
    initialTask?.sessionMode ?? "default",
  );
  const [modelId, setModelId] = useState(initialTask?.modelId ?? "inherit-default");
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);

  if (!isOpen) return null;

  const isEditing = Boolean(initialTask?.taskId && !initialTask.taskId.startsWith("draft-"));

  const handleSave = () => {
    const trimmedName = name.trim();
    if (!trimmedName || !instructions.trim()) return;
    onSave({
      taskId: initialTask?.taskId && isEditing ? initialTask.taskId : `auto-${Date.now()}`,
      name: trimmedName,
      active,
      frequency,
      scheduleTime,
      customCron: frequency === "custom" ? customCron : undefined,
      instructions: instructions.trim(),
      projectFolder,
      sessionMode,
      modelId,
      lastRunLabel: initialTask?.lastRunLabel ?? "Never run yet",
      history: initialTask?.history ?? [],
    });
    onClose();
  };

  return (
    <>
      <ModalDialog
        isOpen={isOpen}
        title={isEditing ? "Edit scheduled task" : "New scheduled task"}
        description="Configure recurring or idle-time autonomous tasks backed by Karyora AIJobRelay."
        onClose={onClose}
      >
        <div className="ky-modal-form" data-testid="scheduled-task-modal">
          <div className="ky-segmented-tabs" role="tablist" aria-label="Scheduled task modal tabs">
            <button
              type="button"
              role="tab"
              aria-selected={modalTab === "settings"}
              className="ky-segmented-tabs__btn"
              data-active={modalTab === "settings" ? "true" : "false"}
              onClick={() => setModalTab("settings")}
            >
              Settings
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={modalTab === "history"}
              className="ky-segmented-tabs__btn"
              data-active={modalTab === "history" ? "true" : "false"}
              onClick={() => setModalTab("history")}
            >
              History ({initialTask?.history.length ?? 0})
            </button>
          </div>

          {modalTab === "settings" ? (
            <>
              <div className="ky-modal-form__row">
                <Input
                  label="Task name"
                  placeholder="e.g. Daily dependency audit"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
                <div style={{ alignSelf: "end" }}>
                  <Toggle
                    label="Active"
                    description="Run automatically on schedule"
                    checked={active}
                    onChange={setActive}
                  />
                </div>
              </div>

              <div className="ky-modal-form__row">
                <Select
                  label="Frequency"
                  value={frequency}
                  options={SCHEDULE_FREQUENCIES}
                  onChange={(e) => setFrequency(e.target.value as AutomationFrequency)}
                />
                {frequency === "custom" ? (
                  <Input
                    label="Cron expression"
                    value={customCron}
                    onChange={(e) => setCustomCron(e.target.value)}
                  />
                ) : (
                  <Input
                    label={frequency === "when-idle" ? "Idle threshold" : "Time"}
                    value={scheduleTime}
                    onChange={(e) => setScheduleTime(e.target.value)}
                  />
                )}
              </div>

              <label style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                <span style={{ fontSize: "0.82rem", fontWeight: 500 }}>Instructions</span>
                <textarea
                  className="ky-modal-form__textarea"
                  placeholder="Describe the exact autonomous instructions for this scheduled task..."
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                />
              </label>

              <div className="ky-modal-form__pills">
                <Button size="sm" variant="secondary" onClick={onPickProjectFolder}>
                  <ChromeIcon name="projects" size={14} />
                  <span>{projectFolder.split("/").pop() || "Select a project folder"}</span>
                </Button>
                <Select
                  aria-label="Session mode"
                  value={sessionMode}
                  options={[
                    { value: "default", label: "Default" },
                    { value: "plan", label: "Plan" },
                    { value: "auto", label: "Auto" },
                  ]}
                  onChange={(e) =>
                    setSessionMode(e.target.value as AutomationSessionMode)
                  }
                />
                <Select
                  aria-label="Model selector"
                  value={modelId}
                  options={[
                    { value: "inherit-default", label: "Inherit default" },
                    ...availableModels.map((m) => ({ value: m, label: m })),
                  ]}
                  onChange={(e) => setModelId(e.target.value)}
                />
                <Input
                  aria-label="Project folder path"
                  value={projectFolder}
                  onChange={(e) => setProjectFolder(e.target.value)}
                />
              </div>

              <div className="ky-modal-form__footer">
                <div>
                  {isEditing && onDelete && (
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setConfirmDeleteOpen(true)}
                    >
                      <ChromeIcon name="trash" size={14} />
                      <span>Delete</span>
                    </Button>
                  )}
                </div>
                <div style={{ display: "flex", gap: "var(--space-2)" }}>
                  <Button size="sm" variant="ghost" onClick={onClose}>
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    variant="primary"
                    disabled={!name.trim() || !instructions.trim()}
                    onClick={handleSave}
                  >
                    Save
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <div className="ky-auto-list" data-testid="scheduled-task-history-tab">
              {(initialTask?.history ?? []).length === 0 ? (
                <p style={{ color: "var(--text-muted)", fontSize: "0.84rem" }}>
                  No execution history yet. Runs launch durable AIJobRelay Jobs.
                </p>
              ) : (
                initialTask?.history.map((run) => (
                  <div key={run.runId} className="ky-auto-card">
                    <div className="ky-auto-card__info">
                      <strong>
                        {run.startedAt} · {run.durationLabel}
                      </strong>
                      <span style={{ fontSize: "0.8rem", color: "var(--text-secondary)" }}>
                        {run.summary}
                      </span>
                    </div>
                    <div className="ky-auto-card__actions">
                      <Badge
                        tone={
                          run.status === "succeeded"
                            ? "success"
                            : run.status === "failed"
                              ? "danger"
                              : "info"
                        }
                        label={run.status}
                      />
                      {onOpenJobHistory && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => onOpenJobHistory(run.jobId)}
                        >
                          Inspect Job {run.jobId}
                        </Button>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </ModalDialog>

      <ConfirmDialog
        isOpen={confirmDeleteOpen}
        title="Delete scheduled task?"
        explanation={`This removes "${name || "scheduled task"}" from future runs. Past durable Job receipts remain in Jobs and Activity Audit.`}
        confirmLabel="Delete task"
        onConfirm={() => {
          if (initialTask && onDelete) {
            onDelete(initialTask.taskId);
          }
          setConfirmDeleteOpen(false);
          onClose();
        }}
        onCancel={() => setConfirmDeleteOpen(false)}
      />
    </>
  );
}
