/**
 * Settings Section 13: MCP Servers (UIR-8.2, Screenshots #37–#43, E2E-18).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Toggle } from "../../components/primitives";
import {
  INITIAL_MCP_SERVERS,
  type McpServerConfigRecord,
} from "../settingsConfig";
import { McpServerEditorForm } from "./McpServerEditorForm";

export interface McpServersSettingsSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function McpServersSettingsSection({
  onNotify,
}: McpServersSettingsSectionProps): ReactElement {
  const [servers, setServers] = useState<readonly McpServerConfigRecord[]>(
    INITIAL_MCP_SERVERS,
  );
  const [creating, setCreating] = useState(false);

  return (
    <div className="ky-settings-content" data-testid="settings-section-mcp-servers">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">
            Installed & Plugin MCP Servers ({servers.length})
          </h3>
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            + New MCP server
          </Button>
        </div>

        {servers.map((srv) => (
          <div key={srv.id} className="ky-gov-perm-row">
            <div>
              <strong>{srv.name}</strong>{" "}
              <Badge tone="info" label={srv.transport.toUpperCase()} />{" "}
              <Badge tone="neutral" label={srv.source} />
              <div style={{ fontFamily: "var(--font-mono)", fontSize: "0.73rem", color: "var(--text-secondary)" }}>
                {srv.commandOrUrl} {srv.args.join(" ")}
              </div>
            </div>
            <Toggle
              label={`Enable ${srv.name}`}
              checked={srv.enabled}
              onChange={(enabled) =>
                setServers((prev) =>
                  prev.map((item) => (item.id === srv.id ? { ...item, enabled } : item)),
                )
              }
            />
          </div>
        ))}
      </div>

      {creating && (
        <McpServerEditorForm
          onSave={(srv) => {
            setServers((prev) => [...prev, srv]);
            setCreating(false);
            onNotify(`MCP server "${srv.name}" configured.`);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}
