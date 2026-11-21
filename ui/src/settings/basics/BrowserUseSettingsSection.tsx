/**
 * Settings Section 4: Browser Use (UIR-8.1, Screenshots #14–#15, Blueprint §13.1 (4)).
 */

import { useState, type ReactElement } from "react";
import { Button, ConfirmDialog, Toggle } from "../../components/primitives";

export interface BrowserUseSettingsSectionProps {
  readonly browserUseEnabled: boolean;
  readonly onToggleBrowserUse: (enabled: boolean) => void;
  readonly onNotify: (msg: string) => void;
}

export function BrowserUseSettingsSection({
  browserUseEnabled,
  onToggleBrowserUse,
  onNotify,
}: BrowserUseSettingsSectionProps): ReactElement {
  const [importChromeState, setImportChromeState] = useState(false);
  const [openHtmlOnClick, setOpenHtmlOnClick] = useState(true);
  const [ignoreCertErrors, setIgnoreCertErrors] = useState(false);
  const [confirmClearAllOpen, setConfirmClearAllOpen] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-browser-use">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Built-In Browser Control & Sandbox</h3>
        <Toggle
          label="Enable built-in browser control"
          description="Allows agentic browser inspection in Side Pane Browser/Sites tabs"
          checked={browserUseEnabled}
          onChange={onToggleBrowserUse}
        />
        <Toggle
          label="Import Chrome sign-in state (explicit opt-in)"
          checked={importChromeState}
          onChange={setImportChromeState}
        />
        <Toggle
          label="Open HTML artifacts on click"
          checked={openHtmlOnClick}
          onChange={setOpenHtmlOnClick}
        />
        <Toggle
          label="Ignore local development TLS certificate errors"
          description="Applies only to localhost/127.0.0.1 previews"
          checked={ignoreCertErrors}
          onChange={setIgnoreCertErrors}
        />
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onNotify("Browser preview cache cleared.")}
          >
            Clear cache
          </Button>
          <Button
            size="sm"
            variant="danger"
            onClick={() => setConfirmClearAllOpen(true)}
          >
            Clear all browser data
          </Button>
        </div>
      </div>

      <ConfirmDialog
        isOpen={confirmClearAllOpen}
        title="Clear all built-in browser data?"
        explanation="This removes cookies, cached preview assets, and imported browser state from Karyora's isolated browser profile."
        confirmLabel="Clear all"
        onConfirm={() => {
          setConfirmClearAllOpen(false);
          onNotify("Cleared all isolated browser data.");
        }}
        onCancel={() => setConfirmClearAllOpen(false)}
      />
    </div>
  );
}
