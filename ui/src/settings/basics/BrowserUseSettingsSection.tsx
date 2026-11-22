/**
 * Settings Section 4: Browser Use — Calibrated 1:1 to ZCode Screenshots #14–#15.
 *
 * Exact 4-group layout:
 * 1. Main card: Enable built-in browser control (toggle) + Import Chrome sign-in state (action button)
 * 2. Opening files: Open HTML files in the built-in browser on click (toggle)
 * 3. Security: Ignore certificate errors (toggle)
 * 4. Browser data: Clear built-in browser cache (`Clear cache`) + Clear all browser data (`Clear all`)
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const [openHtmlOnClick, setOpenHtmlOnClick] = useState(true);
  const [ignoreCertErrors, setIgnoreCertErrors] = useState(false);
  const [confirmClearAllOpen, setConfirmClearAllOpen] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-browser-use">
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.enableTitle", {
                defaultValue: "Enable built-in browser control",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.enableDesc", {
                defaultValue:
                  "Allow sessions to inspect and control web pages inside the built-in browser.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Enable built-in browser control"
              checked={browserUseEnabled}
              onChange={onToggleBrowserUse}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.importTitle", {
                defaultValue: "Import Chrome sign-in state",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.importDesc", {
                defaultValue:
                  "Bring your Chrome sign-in state into the built-in browser once so the agent can open sites you already use.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onNotify("Chrome browser state import wizard opened.")}
            >
              {t("ui.settings.browserUse.importBtn", {
                defaultValue: "Import browser data",
              })}
            </Button>
          </div>
        </div>
      </div>

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.browserUse.openingFiles", { defaultValue: "Opening files" })}
      </h3>
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.openHtmlTitle", {
                defaultValue: "Open HTML files in the built-in browser on click",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.openHtmlDesc", {
                defaultValue:
                  "When on, clicking a local .html file in the file tree opens it in the built-in browser. When off, it opens the source preview.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Open HTML files in the built-in browser on click"
              checked={openHtmlOnClick}
              onChange={setOpenHtmlOnClick}
            />
          </div>
        </div>
      </div>

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.browserUse.security", { defaultValue: "Security" })}
      </h3>
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.ignoreCertTitle", {
                defaultValue: "Ignore certificate errors",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.ignoreCertDesc", {
                defaultValue:
                  "Stop verifying HTTPS certificates for local previews inside the built-in browser only.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Ignore certificate errors"
              checked={ignoreCertErrors}
              onChange={setIgnoreCertErrors}
            />
          </div>
        </div>
      </div>

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.browserUse.browserData", { defaultValue: "Browser data" })}
      </h3>
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.clearCacheTitle", {
                defaultValue: "Clear built-in browser cache",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.clearCacheDesc", {
                defaultValue:
                  "Clear HTTP cache, Cache Storage, and service workers while keeping cookies and local site data.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onNotify("Browser preview cache cleared.")}
            >
              {t("ui.settings.browserUse.clearCacheBtn", {
                defaultValue: "Clear cache",
              })}
            </Button>
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.browserUse.clearAllTitle", {
                defaultValue: "Clear all browser data",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.browserUse.clearAllDesc", {
                defaultValue:
                  "Delete cookies, site data, cache, and recent visits from the built-in browser. This cannot be undone.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Button
              size="sm"
              variant="danger"
              onClick={() => setConfirmClearAllOpen(true)}
            >
              {t("ui.settings.browserUse.clearAllBtn", { defaultValue: "Clear all" })}
            </Button>
          </div>
        </div>
      </div>

      <ConfirmDialog
        isOpen={confirmClearAllOpen}
        title="Clear all built-in browser data?"
        explanation="This removes cookies, cached preview assets, and imported browser state from Karyora's isolated browser profile."
        confirmLabel="Clear all"
        isDestructive
        onConfirm={() => {
          setConfirmClearAllOpen(false);
          onNotify("Cleared all isolated browser data.");
        }}
        onCancel={() => setConfirmClearAllOpen(false)}
      />
    </div>
  );
}
