/**
 * Settings Section 16: Hooks (UIR-8.2, Screenshots #49–#53, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Toggle } from "../../components/primitives";
import {
  HookEditorForm,
  type ConfiguredHookRecord,
} from "./HookEditorForm";

const INITIAL_HOOKS: readonly ConfiguredHookRecord[] = [
  {
    id: "hk-1",
    event: "PreToolUse",
    runner: "process",
    scope: "Project",
    matcher: "Write|Edit",
    command: "gitleaks",
    args: "detect --no-git",
    timeoutMs: 5000,
    enabled: true,
  },
];

export interface HooksSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function HooksSettingsSection({
  onNotify,
}: HooksSettingsSectionProps): ReactElement {
  const [hooks, setHooks] = useState<readonly ConfiguredHookRecord[]>(INITIAL_HOOKS);
  const [creating, setCreating] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-hooks">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">
            Lifecycle Hooks ({hooks.length})
          </h3>
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            + New hook
          </Button>
        </div>

        {hooks.map((hk) => (
          <div key={hk.id} className="ky-gov-perm-row">
            <div>
              <strong>{hk.event}</strong>{" "}
              <Badge tone="info" label={hk.runner} />{" "}
              <Badge tone="neutral" label={hk.scope} />
              <div style={{ fontFamily: "var(--font-mono)", fontSize: "0.73rem", color: "var(--text-secondary)" }}>
                [{hk.matcher}] → {hk.command} {hk.args} ({hk.timeoutMs}ms)
              </div>
            </div>
            <Toggle
              label={`Enable hook ${hk.event}`}
              checked={hk.enabled}
              onChange={(enabled) =>
                setHooks((prev) =>
                  prev.map((item) => (item.id === hk.id ? { ...item, enabled } : item)),
                )
              }
            />
          </div>
        ))}
      </div>

      {creating && (
        <HookEditorForm
          onSave={(hk) => {
            setHooks((prev) => [...prev, hk]);
            setCreating(false);
            onNotify(`Hook on ${hk.event} saved.`);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}
