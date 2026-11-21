/**
 * Settings Section 8: Personalization (UIR-8.2, Screenshot #23, Blueprint §13.2 (8)).
 *
 * Custom instructions textarea with live `0 KB / 100 KB` byte counter, `Save` / `Cancel`,
 * and explicit dirty-form notification.
 */

import { useState, type ReactElement } from "react";
import { Button, Textarea } from "../../components/primitives";

export interface PersonalizationSettingsSectionProps {
  readonly onDirtyChange?: (dirty: boolean) => void;
  readonly onNotify: (msg: string) => void;
}

const MAX_PERSONALIZATION_BYTES = 100 * 1024; // 100 KB (#23)

export function PersonalizationSettingsSection({
  onDirtyChange,
  onNotify,
}: PersonalizationSettingsSectionProps): ReactElement {
  const [savedValue, setSavedValue] = useState(
    "Respond concisely, verify every code edit with automated tests, and never print secrets.",
  );
  const [draft, setDraft] = useState(savedValue);

  const isDirty = draft !== savedValue;

  return (
    <div className="ky-settings-content" data-testid="settings-section-personalization">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Custom Instructions</h3>
        <Textarea
          label="Global custom instructions"
          hintText="Applied across local workspace conversations up to 100 KB."
          maxBytes={MAX_PERSONALIZATION_BYTES}
          rows={8}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            onDirtyChange?.(e.target.value !== savedValue);
          }}
        />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
          <Button
            size="sm"
            variant="ghost"
            disabled={!isDirty}
            onClick={() => {
              setDraft(savedValue);
              onDirtyChange?.(false);
            }}
          >
            Cancel
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!isDirty}
            onClick={() => {
              setSavedValue(draft);
              onDirtyChange?.(false);
              onNotify("Personalization instructions saved.");
            }}
          >
            Save
          </Button>
        </div>
      </div>
    </div>
  );
}
