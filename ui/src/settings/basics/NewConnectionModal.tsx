/**
 * `+ New connection` Modal for Remote Devices (SSH & Docker)
 * Calibrated 1:1 to ZCode Screenshots #17–#19.
 */

import { useState, type ReactElement } from "react";
import { Button, Input, ModalDialog, Select } from "../../components/primitives";

export interface RemoteDeviceConnection {
  readonly id: string;
  readonly kind: "ssh" | "docker";
  readonly label: string;
  readonly target: string;
  readonly authMethod: string;
  readonly status: "connected" | "ready";
}

export interface NewConnectionModalProps {
  readonly isOpen: boolean;
  readonly onCreateConnection: (conn: RemoteDeviceConnection) => void;
  readonly onClose: () => void;
}

export function NewConnectionModal({
  isOpen,
  onCreateConnection,
  onClose,
}: NewConnectionModalProps): ReactElement | null {
  const [kind, setKind] = useState<"ssh" | "docker">("ssh");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [username, setUsername] = useState("");
  const [sshAuth, setSshAuth] = useState<"password" | "private-key">("password");
  const [sshKeyPath, setSshKeyPath] = useState("~/.ssh/id_ed25519");
  const [passphrase, setPassphrase] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [downloadMethod, setDownloadMethod] = useState("local-relay");
  const [dockerMode, setDockerMode] = useState<"running" | "manual">("running");
  const [containerId, setContainerId] = useState("");

  if (!isOpen) return null;

  const canSubmit =
    kind === "ssh"
      ? host.trim().length > 0 && username.trim().length > 0
      : containerId.trim().length > 0;

  return (
    <ModalDialog
      isOpen={isOpen}
      title="New connection"
      description={
        kind === "ssh"
          ? `${username || "user"}@${host || "host"}:${port || "22"}`
          : "Choose a running container or enter a container ID manually."
      }
      onClose={onClose}
    >
      <div className="ky-modal-form" data-testid="new-connection-modal">
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexWrap: "wrap",
            gap: "8px",
          }}
        >
          <div className="ky-segmented-tabs" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={kind === "ssh"}
              className="ky-segmented-tabs__btn"
              data-active={kind === "ssh" ? "true" : "false"}
              onClick={() => setKind("ssh")}
            >
              SSH
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={kind === "docker"}
              className="ky-segmented-tabs__btn"
              data-active={kind === "docker" ? "true" : "false"}
              onClick={() => setKind("docker")}
            >
              Docker
            </button>
          </div>

          {kind === "ssh" && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setHost("192.168.1.100");
                setUsername("dev");
                setPort("22");
                setSshAuth("private-key");
              }}
            >
              Import from SSH config
            </Button>
          )}
        </div>

        {kind === "ssh" ? (
          <>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 110px", gap: "10px" }}>
              <Input
                label="Host"
                placeholder="Enter host address or IP, e.g. 192.168.1.100"
                value={host}
                onChange={(e) => setHost(e.target.value)}
              />
              <Input
                label="Port"
                value={port}
                onChange={(e) => setPort(e.target.value)}
              />
            </div>

            <Input
              label="Username"
              placeholder="Enter username, e.g. root"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />

            <div style={{ display: "flex", alignItems: "center", gap: "12px", flexWrap: "wrap" }}>
              <span style={{ fontSize: "0.82rem", fontWeight: 500, color: "var(--text-primary)" }}>
                Authentication method
              </span>
              <div className="ky-segmented-tabs" role="tablist">
                <button
                  type="button"
                  role="tab"
                  aria-selected={sshAuth === "password"}
                  className="ky-segmented-tabs__btn"
                  data-active={sshAuth === "password" ? "true" : "false"}
                  onClick={() => setSshAuth("password")}
                >
                  Password
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={sshAuth === "private-key"}
                  className="ky-segmented-tabs__btn"
                  data-active={sshAuth === "private-key" ? "true" : "false"}
                  onClick={() => setSshAuth("private-key")}
                >
                  Private key
                </button>
              </div>
            </div>

            {sshAuth === "private-key" ? (
              <>
                <Input
                  label="Private key path"
                  placeholder="Select or enter a private key file path"
                  value={sshKeyPath}
                  onChange={(e) => setSshKeyPath(e.target.value)}
                />
                <Input
                  label="Private key passphrase (optional)"
                  placeholder="Enter your passphrase"
                  writeOnlySecret={true}
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                />
              </>
            ) : (
              <Input
                label="SSH Password"
                placeholder="Enter your SSH password"
                writeOnlySecret={true}
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
              />
            )}

            <div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setShowAdvanced((v) => !v)}
              >
                {showAdvanced ? "▾ Advanced options" : "▸ Advanced options"}
              </Button>
            </div>

            {showAdvanced && (
              <Select
                label="Resource download method"
                value={downloadMethod}
                options={[
                  { value: "local-relay", label: "Download locally, then upload" },
                  { value: "direct", label: "Download on remote server" },
                ]}
                onChange={(e) => setDownloadMethod(e.target.value)}
              />
            )}
          </>
        ) : (
          <>
            <div className="ky-segmented-tabs" role="tablist">
              <button
                type="button"
                role="tab"
                aria-selected={dockerMode === "running"}
                className="ky-segmented-tabs__btn"
                data-active={dockerMode === "running" ? "true" : "false"}
                onClick={() => setDockerMode("running")}
              >
                Select a running container
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={dockerMode === "manual"}
                className="ky-segmented-tabs__btn"
                data-active={dockerMode === "manual" ? "true" : "false"}
                onClick={() => setDockerMode("manual")}
              >
                Enter a container manually
              </button>
            </div>
            <Input
              label="Container"
              placeholder="Enter container name or ID, e.g. my-container"
              value={containerId}
              onChange={(e) => setContainerId(e.target.value)}
            />
          </>
        )}

        <div className="ky-modal-form__footer">
          <span />
          <div style={{ display: "flex", gap: "8px" }}>
            <Button size="sm" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant="primary"
              disabled={!canSubmit}
              onClick={() => {
                onCreateConnection({
                  id: `conn-${Date.now()}`,
                  kind,
                  label:
                    kind === "ssh"
                      ? `${username.trim()}@${host.trim()}`
                      : containerId.trim(),
                  target:
                    kind === "ssh"
                      ? `${username.trim()}@${host.trim()}:${port.trim() || "22"}`
                      : containerId.trim(),
                  authMethod: kind === "ssh" ? sshAuth : dockerMode,
                  status: "ready",
                });
                setPassphrase("");
                onClose();
              }}
            >
              Add and connect
            </Button>
          </div>
        </div>
      </div>
    </ModalDialog>
  );
}
