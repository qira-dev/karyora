/**
 * `+ New connection` Modal for Remote Devices (SSH & Docker)
 * (UIR-8.1, Screenshots #16–#19, E2E-19).
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
  const [label, setLabel] = useState("");
  const [sshHost, setSshHost] = useState("dev@10.0.0.12");
  const [sshAuth, setSshAuth] = useState("private-key");
  const [sshKeyPath, setSshKeyPath] = useState("~/.ssh/id_ed25519");
  const [passphrase, setPassphrase] = useState("");
  const [downloadMethod, setDownloadMethod] = useState("direct");
  const [dockerMode, setDockerMode] = useState("running");
  const [containerId, setContainerId] = useState("karyora-dev-container");

  if (!isOpen) return null;

  return (
    <ModalDialog
      isOpen={isOpen}
      title="New remote device connection"
      description="Add an SSH host or Docker container target. Passphrases are write-only (never stored in browser storage)."
      onClose={onClose}
    >
      <div className="ky-modal-form" data-testid="new-connection-modal">
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

        <Input
          label="Connection label"
          placeholder={kind === "ssh" ? "e.g. Build Server EU" : "e.g. Go 1.25 Dev Container"}
          value={label}
          onChange={(e) => setLabel(e.target.value)}
        />

        {kind === "ssh" ? (
          <>
            <Input
              label="SSH Host (`user@host`)"
              value={sshHost}
              onChange={(e) => setSshHost(e.target.value)}
            />
            <Select
              label="Authentication method"
              value={sshAuth}
              options={[
                { value: "private-key", label: "Private key + passphrase" },
                { value: "password", label: "Password (OS keyring D-37)" },
                { value: "ssh-config", label: "Import from ~/.ssh/config" },
              ]}
              onChange={(e) => setSshAuth(e.target.value)}
            />
            {sshAuth === "private-key" && (
              <Input
                label="Private key path"
                value={sshKeyPath}
                onChange={(e) => setSshKeyPath(e.target.value)}
              />
            )}
            <Input
              label={sshAuth === "password" ? "SSH Password" : "Key Passphrase (optional)"}
              writeOnlySecret={true}
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
            />
            <Select
              label="Resource download method"
              value={downloadMethod}
              options={[
                { value: "direct", label: "Direct download on remote host" },
                { value: "local-relay", label: "Relay through local machine" },
              ]}
              onChange={(e) => setDownloadMethod(e.target.value)}
            />
          </>
        ) : (
          <>
            <Select
              label="Container selection mode"
              value={dockerMode}
              options={[
                { value: "running", label: "Attach to running container" },
                { value: "manual-id", label: "Manual container ID" },
              ]}
              onChange={(e) => setDockerMode(e.target.value)}
            />
            <Input
              label="Container name or ID"
              value={containerId}
              onChange={(e) => setContainerId(e.target.value)}
            />
          </>
        )}

        <div className="ky-modal-form__footer">
          <span />
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            <Button size="sm" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant="primary"
              disabled={!label.trim()}
              onClick={() => {
                onCreateConnection({
                  id: `conn-${Date.now()}`,
                  kind,
                  label: label.trim(),
                  target: kind === "ssh" ? sshHost : containerId,
                  authMethod: kind === "ssh" ? sshAuth : dockerMode,
                  status: "ready",
                });
                setPassphrase("");
                onClose();
              }}
            >
              Add connection
            </Button>
          </div>
        </div>
      </div>
    </ModalDialog>
  );
}
