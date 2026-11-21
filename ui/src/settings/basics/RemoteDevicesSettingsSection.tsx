/**
 * Settings Section 5: Remote Devices & Local/Remote Core Connection Switcher
 * (UIR-8.1, Screenshots #16–#19, Blueprint §3.3, §13.1 (5), E2E-19).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Select } from "../../components/primitives";
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
  const [coreMode, setCoreMode] = useState<"local" | "remote">("local");
  const [connections, setConnections] = useState<readonly RemoteDeviceConnection[]>([
    {
      id: "conn-local",
      kind: "ssh",
      label: "This computer (Local Loopback)",
      target: "127.0.0.1:27201",
      authMethod: "OS session token",
      status: "connected",
    },
  ]);
  const [modalOpen, setModalOpen] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-remote-devices">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Core Connection Authority (Local ↔ Remote)</h3>
        <Select
          label="Active Core endpoint"
          value={coreMode}
          options={[
            { value: "local", label: "Local Core (127.0.0.1:27201 — Single-user)" },
            { value: "remote", label: "Remote Core (Team / Explicit Switch — Purges Local Cache)" },
          ]}
          onChange={(e) => {
            const next = e.target.value as "local" | "remote";
            setCoreMode(next);
            onSwitchCoreConnection(next);
          }}
        />
        <p style={{ margin: 0, fontSize: "0.78rem", color: "var(--text-secondary)" }}>
          Switching between Local and Remote Core immediately purges `DomainStateCache` and
          cancels in-flight streams — zero silent cross-connection data sync.
        </p>
      </div>

      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">Remote SSH & Docker Devices</h3>
          <Button size="sm" variant="primary" onClick={() => setModalOpen(true)}>
            + New connection
          </Button>
        </div>

        {connections.map((conn) => (
          <div key={conn.id} className="ky-gov-perm-row">
            <span>
              <strong>{conn.label}</strong> · <code>{conn.target}</code> ({conn.kind.toUpperCase()} / {conn.authMethod})
            </span>
            <Badge
              tone={conn.status === "connected" ? "success" : "info"}
              label={conn.status}
            />
          </div>
        ))}
      </div>

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
