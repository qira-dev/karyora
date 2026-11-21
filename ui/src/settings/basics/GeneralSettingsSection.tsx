/**
 * Settings Section 1: General (UIR-8.1, Screenshots #1–#4, Blueprint §13.1 (1)).
 *
 * Implements all 15 controls: Language (13 locales), Interface mode (Coding/Office),
 * Proactive suggestions, Terminal profile & font, HTTP/No Proxy, Custom cert, Hardware accel,
 * Preview/Auto updates, Task/Sound/Mobile notifications, Tray, Keep awake, Data storage path,
 * and `Open onboarding`.
 */

import { useState, type ReactElement } from "react";
import { Button, Input, Select, Toggle } from "../../components/primitives";
import {
  SUPPORTED_LOCALES,
  type InterfaceMode,
  type SupportedLocale,
} from "../../types/decisions";

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
  const [proactiveSuggestions, setProactiveSuggestions] = useState(true);
  const [terminalProfile, setTerminalProfile] = useState("zsh");
  const [terminalFont, setTerminalFont] = useState("JetBrains Mono");
  const [httpProxy, setHttpProxy] = useState("");
  const [noProxy, setNoProxy] = useState("127.0.0.1,localhost");
  const [customCertPath, setCustomCertPath] = useState("");
  const [hardwareAccel, setHardwareAccel] = useState(true);
  const [previewChannel, setPreviewChannel] = useState(false);
  const [autoUpdateCheck, setAutoUpdateCheck] = useState(true);
  const [taskNotifications, setTaskNotifications] = useState(true);
  const [soundAlerts, setSoundAlerts] = useState(false);
  const [mobilePush, setMobilePush] = useState(false);
  const [hideToTray, setHideToTray] = useState(true);
  const [keepAwake, setKeepAwake] = useState(true);
  const [dataStoragePath] = useState("~/.karyora");

  return (
    <div className="ky-settings-content" data-testid="settings-section-general">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Language, Interface & Assistant</h3>
        <div className="ky-settings-grid-2">
          <Select
            label="Language (13 Locales)"
            value={locale}
            options={SUPPORTED_LOCALES.map((loc) => ({ value: loc, label: loc }))}
            onChange={(e) => onChangeLocale(e.target.value as SupportedLocale)}
          />
          <Select
            label="Interface mode"
            value={interfaceMode}
            options={[
              { value: "coding", label: "Coding mode" },
              { value: "office", label: "Office mode" },
            ]}
            onChange={(e) => onChangeInterfaceMode(e.target.value as InterfaceMode)}
          />
        </div>
        <Toggle
          label="Proactive suggestions"
          description="Offer follow-up prompt suggestions and workspace context hints"
          checked={proactiveSuggestions}
          onChange={setProactiveSuggestions}
        />
      </div>

      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Terminal & Network Proxy</h3>
        <div className="ky-settings-grid-2">
          <Select
            label="Default terminal profile"
            value={terminalProfile}
            options={[
              { value: "zsh", label: "zsh" },
              { value: "bash", label: "bash" },
              { value: "powershell", label: "PowerShell" },
            ]}
            onChange={(e) => setTerminalProfile(e.target.value)}
          />
          <Input
            label="Terminal font family"
            value={terminalFont}
            onChange={(e) => setTerminalFont(e.target.value)}
          />
          <Input
            label="HTTP / HTTPS Proxy"
            placeholder="http://proxy.internal:8080"
            value={httpProxy}
            onChange={(e) => setHttpProxy(e.target.value)}
          />
          <Input
            label="No Proxy hosts"
            value={noProxy}
            onChange={(e) => setNoProxy(e.target.value)}
          />
        </div>
        <Input
          label="Custom CA Certificate path"
          placeholder="/etc/ssl/certs/corp-root.pem"
          value={customCertPath}
          onChange={(e) => setCustomCertPath(e.target.value)}
        />
      </div>

      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Notifications, Runtime & Data Storage</h3>
        <Toggle label="Hardware acceleration" checked={hardwareAccel} onChange={setHardwareAccel} />
        <Toggle label="Preview release channel" checked={previewChannel} onChange={setPreviewChannel} />
        <Toggle label="Automatic update checks (non-forced)" checked={autoUpdateCheck} onChange={setAutoUpdateCheck} />
        <Toggle label="Task completion notifications" checked={taskNotifications} onChange={setTaskNotifications} />
        <Toggle label="Notification sound" checked={soundAlerts} onChange={setSoundAlerts} />
        <Toggle label="Mobile push notifications (opt-in)" checked={mobilePush} onChange={setMobilePush} />
        <Toggle label="Hide to system tray on window close (UI-24)" checked={hideToTray} onChange={setHideToTray} />
        <Toggle label="Keep awake while durable Jobs are running" checked={keepAwake} onChange={setKeepAwake} />
        <Input label="Local Core data storage path" value={dataStoragePath} readOnly />
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Button size="sm" variant="secondary" onClick={onOpenOnboarding}>
            Open onboarding
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() => onNotify("General settings applied.")}
          >
            Save General Settings
          </Button>
        </div>
      </div>
    </div>
  );
}
