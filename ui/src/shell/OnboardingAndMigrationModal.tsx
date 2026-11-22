/**
 * First-Run Setup & 6-Step Migration Guide Modal
 * (UIR-3.4, Screenshot #55, Blueprint §7.1, E2E-01).
 *
 * Implements:
 * - First-Run Onboarding (Core health status, optional BYOK Provider link,
 *   workspace folder picker, always-reachable `Skip for now`).
 * - 6-Step Migration Guide (`#55`):
 *   `1 Sessions` → `2 Skills` → `3 MCP servers` → `4 Commands` → `5 AGENTS.md` → `6 Migration`
 *   with `Skip`, `Back`, and `Next →` controls.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, ModalDialog } from "../components/primitives";
import type { CoreBootstrapStatus } from "../coreclient/types";
import { MIGRATION_GUIDE_STEPS } from "./shellConfig";

export interface OnboardingAndMigrationModalProps {
  readonly mode: "closed" | "onboarding" | "migration";
  readonly bootstrapStatus: CoreBootstrapStatus | null;
  readonly onClose: () => void;
  readonly onOpenModelSettings: () => void;
  readonly onPickWorkspaceFolder: () => void;
  readonly onCompleteMigration?: () => void;
}

export function OnboardingAndMigrationModal({
  mode,
  bootstrapStatus,
  onClose,
  onOpenModelSettings,
  onPickWorkspaceFolder,
  onCompleteMigration,
}: OnboardingAndMigrationModalProps): ReactElement | null {
  const [migrationStepIndex, setMigrationStepIndex] = useState(0);

  if (mode === "closed") {
    return null;
  }

  if (mode === "onboarding") {
    return (
      <ModalDialog
        isOpen
        title="Welcome to Karyora — First-Run Setup"
        description="Verify your local or remote Go Core connection, optionally link a BYOK model provider into your OS keyring, and select your first workspace."
        onClose={onClose}
        footer={
          <>
            <Button variant="ghost" onClick={onClose}>
              Skip for now
            </Button>
            <Button variant="primary" onClick={onClose}>
              Enter Workspace
            </Button>
          </>
        }
      >
        <div
          data-testid="first-run-onboarding-modal"
          style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}
        >
          <div
            style={{
              padding: "var(--space-3)",
              borderRadius: "var(--radius-md)",
              border: "1px solid var(--border-subtle)",
              backgroundColor: "var(--bg-surface-2)",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <div>
              <div style={{ fontWeight: 600, fontSize: "0.875rem" }}>
                1. Core Connection Health
              </div>
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                Active target: {bootstrapStatus?.connectionId ?? "local-core"}
              </div>
            </div>
            <Badge
              tone={
                bootstrapStatus?.status === "connected" ? "success" : "warning"
              }
              dot
              label={bootstrapStatus?.status ?? "probing"}
            />
          </div>

          <div
            style={{
              padding: "var(--space-3)",
              borderRadius: "var(--radius-md)",
              border: "1px solid var(--border-subtle)",
              backgroundColor: "var(--bg-surface-2)",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <div>
              <div style={{ fontWeight: 600, fontSize: "0.875rem" }}>
                2. BYOK Provider &amp; Models (Optional)
              </div>
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                Your API key / your provider billing stored exclusively in OS keyring.
              </div>
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                onClose();
                onOpenModelSettings();
              }}
            >
              Configure Provider
            </Button>
          </div>

          <div
            style={{
              padding: "var(--space-3)",
              borderRadius: "var(--radius-md)",
              border: "1px solid var(--border-subtle)",
              backgroundColor: "var(--bg-surface-2)",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <div>
              <div style={{ fontWeight: 600, fontSize: "0.875rem" }}>
                3. Open Workspace Folder
              </div>
              <div style={{ fontSize: "0.75rem", color: "var(--text-secondary)" }}>
                Attach a local folder or remote SSH/Docker workspace.
              </div>
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={onPickWorkspaceFolder}
            >
              Choose folder
            </Button>
          </div>
        </div>
      </ModalDialog>
    );
  }

  // Screenshot #55: 6-Step Migration Guide Modal
  const currentStep = MIGRATION_GUIDE_STEPS[migrationStepIndex];
  const isLastStep = migrationStepIndex === MIGRATION_GUIDE_STEPS.length - 1;

  return (
    <ModalDialog
      isOpen
      title="Migration Guide"
      description="Migrate existing sessions, skills, MCP servers, commands, and AGENTS.md into your Karyora workspace."
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Skip
          </Button>
          {migrationStepIndex > 0 && (
            <Button
              variant="secondary"
              onClick={() => setMigrationStepIndex((prev) => Math.max(0, prev - 1))}
            >
              Back
            </Button>
          )}
          <Button
            variant="primary"
            onClick={() => {
              if (isLastStep) {
                onCompleteMigration?.();
                setMigrationStepIndex(0);
                onClose();
              } else {
                setMigrationStepIndex((prev) =>
                  Math.min(MIGRATION_GUIDE_STEPS.length - 1, prev + 1),
                );
              }
            }}
          >
            {isLastStep ? "Finish migration" : "Next →"}
          </Button>
        </>
      }
    >
      <div
        data-testid="migration-guide-6-step-modal"
        style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
      >
        <ol
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--space-2)",
            listStyle: "none",
            margin: 0,
            padding: 0,
          }}
        >
          {MIGRATION_GUIDE_STEPS.map((step, idx) => (
            <li key={step.id}>
              <Badge
                tone={
                  idx === migrationStepIndex
                    ? "accent"
                    : idx < migrationStepIndex
                      ? "success"
                      : "neutral"
                }
                label={`${step.stepNumber} ${step.label}`}
              />
            </li>
          ))}
        </ol>

        <div
          style={{
            padding: "var(--space-4)",
            borderRadius: "var(--radius-md)",
            border: "1px solid var(--border-subtle)",
            backgroundColor: "var(--bg-surface-2)",
          }}
        >
          <h3 style={{ margin: "0 0 var(--space-2) 0", fontSize: "0.9375rem" }}>
            Step {currentStep.stepNumber}: {currentStep.label}
          </h3>
          <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--text-secondary)" }}>
            {currentStep.summary}
          </p>
        </div>
      </div>
    </ModalDialog>
  );
}
