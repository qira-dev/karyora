/**
 * `New hook` Editor Form (UIR-8.2, Screenshots #49–#53, E2E-18).
 *
 * Covers all 7 lifecycle events, `Process` / `Shell command` runner, Scope, Matcher,
 * Command, Arguments, and expandable `Advanced` section (timeout, status message, custom JSON).
 */

import { useState, type ReactElement } from "react";
import { Button, Input, Select, Textarea } from "../../components/primitives";
import {
  HOOK_LIFECYCLE_EVENTS,
  type HookLifecycleEvent,
} from "../settingsConfig";

export interface ConfiguredHookRecord {
  readonly id: string;
  readonly event: HookLifecycleEvent;
  readonly runner: "process" | "shell";
  readonly scope: "User" | "Project";
  readonly matcher: string;
  readonly command: string;
  readonly args: string;
  readonly timeoutMs: number;
  readonly enabled: boolean;
}

export interface HookEditorFormProps {
  readonly onSave: (hook: ConfiguredHookRecord) => void;
  readonly onCancel: () => void;
}

export function HookEditorForm({
  onSave,
  onCancel,
}: HookEditorFormProps): ReactElement {
  const [event, setEvent] = useState<HookLifecycleEvent>("PreToolUse");
  const [runner, setRunner] = useState<"process" | "shell">("process");
  const [scope, setScope] = useState<"User" | "Project">("Project");
  const [matcher, setMatcher] = useState("Edit|Write");
  const [command, setCommand] = useState("node");
  const [args, setArgs] = useState("scripts/check-hook.mjs");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [timeoutMs, setTimeoutMs] = useState("5000");
  const [statusMessage, setStatusMessage] = useState("Running pre-tool verification hook...");
  const [customJson, setCustomJson] = useState("{}");

  return (
    <div className="ky-settings-card" data-testid="hook-editor-form">
      <h4 className="ky-settings-card__title">New lifecycle hook</h4>
      <div className="ky-settings-grid-2">
        <Select
          label="Lifecycle event (7 events)"
          value={event}
          options={HOOK_LIFECYCLE_EVENTS.map((ev) => ({ value: ev, label: ev }))}
          onChange={(e) => setEvent(e.target.value as HookLifecycleEvent)}
        />
        <Select
          label="Runner type"
          value={runner}
          options={[
            { value: "process", label: "Process (Direct argv, safer)" },
            { value: "shell", label: "Shell command" },
          ]}
          onChange={(e) => setRunner(e.target.value as "process" | "shell")}
        />
        <Select
          label="Scope"
          value={scope}
          options={[
            { value: "Project", label: "Project" },
            { value: "User", label: "User" },
          ]}
          onChange={(e) => setScope(e.target.value as "User" | "Project")}
        />
        <Input
          label="Tool / Target Matcher"
          value={matcher}
          onChange={(e) => setMatcher(e.target.value)}
        />
        <Input
          label="Command"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
        />
        <Input
          label="Arguments"
          value={args}
          onChange={(e) => setArgs(e.target.value)}
        />
      </div>

      <div>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowAdvanced((v) => !v)}
        >
          {showAdvanced ? "Hide Advanced" : "Show Advanced (timeout / status / custom JSON)"}
        </Button>
      </div>

      {showAdvanced && (
        <div className="ky-settings-grid-2">
          <Input
            label="Timeout (ms)"
            type="number"
            value={timeoutMs}
            onChange={(e) => setTimeoutMs(e.target.value)}
          />
          <Input
            label="Status message"
            value={statusMessage}
            onChange={(e) => setStatusMessage(e.target.value)}
          />
          <Textarea
            label="Custom JSON metadata"
            rows={3}
            value={customJson}
            onChange={(e) => setCustomJson(e.target.value)}
          />
        </div>
      )}

      <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          size="sm"
          variant="primary"
          disabled={!command.trim()}
          onClick={() =>
            onSave({
              id: `hook-${Date.now()}`,
              event,
              runner,
              scope,
              matcher,
              command: command.trim(),
              args: args.trim(),
              timeoutMs: Number(timeoutMs) || 5000,
              enabled: true,
            })
          }
        >
          Save hook
        </Button>
      </div>
    </div>
  );
}
