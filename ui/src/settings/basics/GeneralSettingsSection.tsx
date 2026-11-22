/**
 * Settings Section 1: General — Calibrated 1:1 to ZCode Screenshots #1–#4.
 *
 * Horizontal grouped setting rows with live 13-locale i18next switching,
 * interface mode, terminal profile/font, proxy, hardware accel, notifications,
 * tray, keep awake, storage path, and onboarding/migration launcher.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Button, Toggle } from "../../components/primitives";
import { LOCALE_METADATA } from "../../i18n";
import type { InterfaceMode, SupportedLocale } from "../../types/decisions";

export interface GeneralSettingsSectionProps {
  readonly locale: SupportedLocale;
  readonly interfaceMode: InterfaceMode;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onOpenOnboarding: () => void;
  readonly onNotify: (msg: string) => void;
}

export function GeneralSettingsSection({
  locale,
  interfaceMode,
  onChangeLocale,
  onChangeInterfaceMode,
  onOpenOnboarding,
  onNotify,
}: GeneralSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [proactiveSuggestions, setProactiveSuggestions] = useState(false);
  const [inheritTerminalProfile, setInheritTerminalProfile] = useState(true);
  const [terminalFont, setTerminalFont] = useState("");
  const [httpProxy, setHttpProxy] = useState("");
  const [noProxy, setNoProxy] = useState("127.0.0.1,localhost");
  const [hardwareAccel, setHardwareAccel] = useState(true);
  const [autoUpdateCheck, setAutoUpdateCheck] = useState(true);
  const [taskNotifications, setTaskNotifications] = useState(true);
  const [hideToTray, setHideToTray] = useState(true);
  const [keepAwake, setKeepAwake] = useState(true);
  const [dataStoragePath] = useState("~/.karyora");

  return (
    <div className="ky-settings-content" data-testid="settings-section-general">
      {/* Group 1: Language (Screenshot #1 top card) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.languageTitle", { defaultValue: "Language" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.languageDesc", {
                defaultValue: "Choose the display language used by the application UI.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <select
              aria-label={t("ui.settings.general.languageTitle", { defaultValue: "Language" })}
              className="ky-select"
              value={locale}
              onChange={(e) => onChangeLocale(e.target.value as SupportedLocale)}
            >
              {LOCALE_METADATA.map((loc) => (
                <option key={loc.code} value={loc.code}>
                  {loc.nativeLabel} ({loc.code})
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* Group 2: Interface Mode & Proactive Suggestions (Screenshot #1 second card) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.interfaceModeTitle", { defaultValue: "Interface mode" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.interfaceModeDesc", {
                defaultValue:
                  "Office mode focuses on summaries and results. Coding mode shows commands, output, and code changes.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <select
              aria-label={t("ui.settings.general.interfaceModeTitle", {
                defaultValue: "Interface mode",
              })}
              className="ky-select"
              value={interfaceMode}
              onChange={(e) => onChangeInterfaceMode(e.target.value as InterfaceMode)}
            >
              <option value="coding">
                {t("ui.profile.codingMode", { defaultValue: "Coding mode" })}
              </option>
              <option value="office">
                {t("ui.profile.officeMode", { defaultValue: "Office mode" })}
              </option>
            </select>
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.proactiveTitle", {
                defaultValue: "Proactive task suggestions",
              })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.proactiveDesc", {
                defaultValue: "Offer contextual follow-up suggestions and starter workflows.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle
              label=""
              aria-label="Proactive task suggestions"
              checked={proactiveSuggestions}
              onChange={setProactiveSuggestions}
            />
          </div>
        </div>
      </div>

      {/* Group 3: Terminal Profile & Font (Screenshot #1 third card) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.terminalProfileTitle", {
                defaultValue: "Inherit system terminal profile",
              })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.terminalProfileDesc", {
                defaultValue:
                  "Inherit login shell environment, proxy, and local terminal settings when launching the built-in terminal.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle
              label=""
              aria-label="Inherit system terminal profile"
              checked={inheritTerminalProfile}
              onChange={setInheritTerminalProfile}
            />
          </div>
        </div>

        <div className="ky-setting-row ky-setting-row--stacked">
          <div className="ky-setting-row__head">
            <div className="ky-setting-row__info">
              <h3 className="ky-setting-row__title">
                {t("ui.settings.general.terminalFontTitle", { defaultValue: "Terminal font" })}
              </h3>
              <p className="ky-setting-row__desc">
                {t("ui.settings.general.terminalFontDesc", {
                  defaultValue:
                    "Leave blank to auto-detect system terminal settings or set a custom monospace font.",
                })}
              </p>
            </div>
            <Button
              size="sm"
              variant="primary"
              onClick={() => onNotify("Terminal font setting saved.")}
            >
              {t("ui.settings.save", { defaultValue: "Save" })}
            </Button>
          </div>
          <input
            type="text"
            className="ky-input"
            aria-label="Terminal font"
            placeholder="Leave blank to inherit, e.g. MesloLGS NF, monospace"
            value={terminalFont}
            onChange={(e) => setTerminalFont(e.target.value)}
          />
        </div>
      </div>

      {/* Group 4: HTTP Proxy & No Proxy (Screenshot #1–#2) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row ky-setting-row--stacked">
          <div className="ky-setting-row__head">
            <div className="ky-setting-row__info">
              <h3 className="ky-setting-row__title">
                {t("ui.settings.general.httpProxyTitle", { defaultValue: "HTTP Proxy" })}
              </h3>
              <p className="ky-setting-row__desc">
                {t("ui.settings.general.httpProxyDesc", {
                  defaultValue:
                    "Route model, MCP, and extension egress traffic through this proxy. Leave blank to connect directly.",
                })}
              </p>
            </div>
            <Button
              size="sm"
              variant="primary"
              onClick={() => onNotify("Network proxy settings saved.")}
            >
              {t("ui.settings.save", { defaultValue: "Save" })}
            </Button>
          </div>
          <input
            type="text"
            className="ky-input"
            aria-label="HTTP Proxy"
            placeholder="http://127.0.0.1:7890"
            value={httpProxy}
            onChange={(e) => setHttpProxy(e.target.value)}
          />
        </div>

        <div className="ky-setting-row ky-setting-row--stacked">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.noProxyTitle", { defaultValue: "No Proxy" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.noProxyDesc", {
                defaultValue: "Comma-separated hosts that bypass the configured proxy.",
              })}
            </p>
          </div>
          <input
            type="text"
            className="ky-input"
            aria-label="No Proxy"
            value={noProxy}
            onChange={(e) => setNoProxy(e.target.value)}
          />
        </div>
      </div>

      {/* Group 5: Runtime, Notifications & Desktop Lifecycle (#3–#4) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.hardwareAccelTitle", { defaultValue: "Hardware acceleration" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.hardwareAccelDesc", {
                defaultValue: "Use GPU compositing for smoother animations and scrolling.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle label="" aria-label="Hardware acceleration" checked={hardwareAccel} onChange={setHardwareAccel} />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.autoUpdateTitle", { defaultValue: "Automatic update checks" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.autoUpdateDesc", {
                defaultValue: "Check for signed Karyora Core and UI updates without disabling the installed app.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle label="" aria-label="Automatic update checks" checked={autoUpdateCheck} onChange={setAutoUpdateCheck} />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.notificationsTitle", { defaultValue: "Task notifications" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.notificationsDesc", {
                defaultValue: "Notify when background durable Jobs finish or require approval.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle label="" aria-label="Task notifications" checked={taskNotifications} onChange={setTaskNotifications} />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.trayTitle", { defaultValue: "Minimize to system tray on close" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.trayDesc", {
                defaultValue: "Keep active durable Jobs alive in the tray when closing desktop windows.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle label="" aria-label="Minimize to system tray on close" checked={hideToTray} onChange={setHideToTray} />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.keepAwakeTitle", { defaultValue: "Keep system awake during active runs" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.keepAwakeDesc", {
                defaultValue: "Prevent host sleep while agent tasks or workflows are running.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle label="" aria-label="Keep system awake" checked={keepAwake} onChange={setKeepAwake} />
          </div>
        </div>
      </div>

      {/* Group 6: Storage Path & Onboarding (#4) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.storagePathTitle", { defaultValue: "Data storage path" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.storagePathDesc", {
                defaultValue: "Local SQLite WAL workspace directory managed by Karyora Core.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <input
              type="text"
              className="ky-input"
              aria-label="Data storage path"
              value={dataStoragePath}
              readOnly
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.general.onboardingTitle", { defaultValue: "Onboarding & Migration" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.general.onboardingDesc", {
                defaultValue: "Reopen the first-run setup or 6-step data migration guide.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Button size="sm" variant="secondary" onClick={onOpenOnboarding}>
              {t("ui.settings.general.openOnboarding", { defaultValue: "Open onboarding" })}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
