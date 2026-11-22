/**
 * Settings Section 8: Personalization — Calibrated 1:1 to ZCode Screenshot #23.
 *
 * Clean subtitle, full-width custom instructions textarea with `X KB / 100 KB` counter,
 * and `Save` / `Cancel` controls with explicit dirty-form state tracking.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Button, Textarea } from "../../components/primitives";

export interface PersonalizationSettingsSectionProps {
  readonly onDirtyChange?: (dirty: boolean) => void;
  readonly onNotify: (msg: string) => void;
}

const MAX_PERSONALIZATION_BYTES = 100 * 1024;

export function PersonalizationSettingsSection({
  onDirtyChange,
  onNotify,
}: PersonalizationSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [savedValue, setSavedValue] = useState(
    "Respond concisely, verify every code edit with automated tests, and never print secrets.",
  );
  const [draft, setDraft] = useState(savedValue);

  const isDirty = draft !== savedValue;

  return (
    <div className="ky-settings-content" data-testid="settings-section-personalization">
      <p className="ky-settings-section-sub">
        {t("ui.settings.personalization.subtitle", {
          defaultValue:
            "Add custom instructions to personalize Agent responses. Instructions apply to future conversations of the local Agent.",
        })}
      </p>

      <Textarea
        aria-label="Custom instructions"
        placeholder={t("ui.settings.personalization.placeholder", {
          defaultValue: "Add custom instructions...",
        })}
        maxBytes={MAX_PERSONALIZATION_BYTES}
        rows={10}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          onDirtyChange?.(e.target.value !== savedValue);
        }}
      />

      <div style={{ display: "flex", justifyContent: "flex-end", gap: "8px" }}>
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
          {t("ui.settings.personalization.save", { defaultValue: "Save" })}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          disabled={!isDirty}
          onClick={() => {
            setDraft(savedValue);
            onDirtyChange?.(false);
          }}
        >
          {t("ui.settings.personalization.cancel", { defaultValue: "Cancel" })}
        </Button>
      </div>
    </div>
  );
}
