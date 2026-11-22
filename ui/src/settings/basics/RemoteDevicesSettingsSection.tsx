/**
 * Settings Section 5: Remote Devices — Calibrated 1:1 to ZCode Screenshots #16–#19.
 *
 * Includes ZCode's `Preferences` group (`Hide device names in project rows`,
 * `Reconnect automatically at startup`), Karyora Core Local/Remote Connection Authority,
 * and SSH/Docker device list with `+ New connection` modal.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Select, Toggle } from "../../components/primitives";
import {
  NewConnectionModal,
  type RemoteDeviceConnection,
} from "./NewConnectionModal";

export interface RemoteDevicesSettingsSectionProps {
  readonly onSwitchCoreConnection: (mode: "local" | "remote") => void;
  readonly onNotify: (msg: string) => void;
}

export function RemoteDevicesSettingsSection({
  onSwitchCoreConnection,
  onNotify,
}: RemoteDevicesSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [hideDeviceNames, setHideDeviceNames] = useState(false);
  const [autoReconnect, setAutoReconnect] = useState(true);
  const [coreMode, setCoreMode] = useState<"local" | "remote">("local");
  const [connections, setConnections] = useState<readonly RemoteDeviceConnection[]>([
    {
      id: "conn-local",
      kind: "ssh",
      label: "This computer",
      target: "127.0.0.1:27201",
      authMethod: "Local Core session",
      status: "connected",
    },
  ]);
  const [modalOpen, setModalOpen] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-remote-devices">
      <p className="ky-settings-section-sub">
        {t("ui.settings.remoteDevices.subtitle", {
          defaultValue: "Manage your servers, WSL distributions, and Docker containers.",
        })}
      </p>

      <h3 className="ky-settings-section-heading">
        {t("ui.settings.remoteDevices.preferences", { defaultValue: "Preferences" })}
      </h3>
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.remoteDevices.hideDeviceNames", {
                defaultValue: "Hide device names in project rows",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.remoteDevices.hideDeviceNamesDesc", {
                defaultValue:
                  "Hide remote device names in task list project rows. Hover to view them.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Hide device names in project rows"
              checked={hideDeviceNames}
              onChange={setHideDeviceNames}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.remoteDevices.autoReconnect", {
                defaultValue: "Reconnect automatically at startup",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.remoteDevices.autoReconnectDesc", {
                defaultValue:
                  "Reconnects devices seen online in the last 7 days at startup.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Reconnect automatically at startup"
              checked={autoReconnect}
              onChange={setAutoReconnect}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.remoteDevices.coreAuthority", {
                defaultValue: "Active Core endpoint",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.remoteDevices.coreAuthorityDesc", {
                defaultValue:
                  "Switching between Local and Remote Core immediately purges cached state and cancels in-flight streams.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Active Core endpoint"
              value={coreMode}
              options={[
                { value: "local", label: "Local Core (127.0.0.1:27201)" },
                { value: "remote", label: "Remote Core (Explicit switch)" },
              ]}
              onChange={(e) => {
                const next = e.target.value as "local" | "remote";
                setCoreMode(next);
                onSwitchCoreConnection(next);
              }}
            />
          </div>
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>
            {t("ui.settings.sections.remote-devices", { defaultValue: "Remote devices" })}
          </span>
          <span className="ky-settings-group-header__count">{connections.length}</span>
        </h3>
        <Button size="sm" variant="primary" onClick={() => setModalOpen(true)}>
          <ChromeIcon name="plus" size={14} />
          <span>
            {t("ui.settings.remoteDevices.addConnection", {
              defaultValue: "New connection",
            })}
          </span>
        </Button>
      </div>

      {connections.length === 0 ? (
        <div className="ky-settings-empty">
          <ChromeIcon name="remote-server" size={24} />
          <p className="ky-settings-empty__title">
            {t("ui.settings.remoteDevices.emptyTitle", {
              defaultValue: "No remote device connections yet.",
            })}
          </p>
          <Button size="sm" variant="secondary" onClick={() => setModalOpen(true)}>
            <ChromeIcon name="plus" size={14} />
            <span>
              {t("ui.settings.remoteDevices.addConnection", {
                defaultValue: "New connection",
              })}
            </span>
          </Button>
        </div>
      ) : (
        <div className="ky-settings-list">
          {connections.map((conn) => (
            <div key={conn.id} className="ky-settings-list-row">
              <div className="ky-settings-list-row__main">
                <span className="ky-settings-list-row__icon">
                  <ChromeIcon name="remote-server" size={16} />
                </span>
                <div className="ky-settings-list-row__copy">
                  <span className="ky-settings-list-row__title">
                    <span
                      className={`ky-status-dot ${
                        conn.status === "connected" ? "ky-status-dot--ok" : "ky-status-dot--muted"
                      }`}
                    />
                    <span>{conn.label}</span>
                  </span>
                  <span className="ky-settings-list-row__desc" style={{ fontFamily: "var(--font-mono)" }}>
                    {conn.kind.toUpperCase()} · {conn.target} · {conn.authMethod}
                  </span>
                </div>
              </div>
              <div className="ky-settings-list-row__right">
                <Badge
                  tone={conn.status === "connected" ? "success" : "neutral"}
                  label={conn.status}
                />
              </div>
            </div>
          ))}
        </div>
      )}

      <NewConnectionModal
        isOpen={modalOpen}
        onCreateConnection={(conn) => {
          setConnections((prev) => [...prev, conn]);
          onNotify(`Added ${conn.kind.toUpperCase()} device: ${conn.label}`);
        }}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
}
