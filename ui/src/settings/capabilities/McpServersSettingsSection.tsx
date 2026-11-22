/**
 * Settings Section 13: MCP Servers — Calibrated 1:1 to ZCode Screenshots #37–#43.
 *
 * Counter pill + search bar, `Installed` & plugin-bundled MCP sections with live
 * status dot + transport URL subtitle + toggle switch, and inline `MCP Servers > New MCP server` view.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, Input, Toggle } from "../../components/primitives";
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
  const { t } = useTranslation();
  const [servers, setServers] = useState<readonly McpServerConfigRecord[]>(
    INITIAL_MCP_SERVERS,
  );
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);

  if (creating) {
    return (
      <div className="ky-settings-content" data-testid="settings-section-mcp-servers">
        <div className="ky-settings-breadcrumb">
          <button
            type="button"
            className="ky-settings-breadcrumb__link"
            onClick={() => setCreating(false)}
          >
            {t("ui.settings.sections.mcp-servers", { defaultValue: "MCP Servers" })}
          </button>
          <span>›</span>
          <span>{t("ui.settings.capabilities.newMcp", { defaultValue: "New MCP server" })}</span>
        </div>
        <McpServerEditorForm
          onSave={(srv) => {
            setServers((prev) => [...prev, srv]);
            setCreating(false);
            onNotify(`MCP server "${srv.name}" configured.`);
          }}
          onCancel={() => setCreating(false)}
        />
      </div>
    );
  }

  const q = search.trim().toLowerCase();
  const visible = servers.filter(
    (s) =>
      !q ||
      s.name.toLowerCase().includes(q) ||
      s.commandOrUrl.toLowerCase().includes(q),
  );
  const installed = visible.filter((s) => s.source === "installed");
  const pluginServers = visible.filter((s) => s.source !== "installed");

  const renderRow = (srv: McpServerConfigRecord) => (
    <div key={srv.id} className="ky-settings-list-row">
      <div className="ky-settings-list-row__main">
        <span className="ky-settings-list-row__icon">
          <ChromeIcon name="mcp-plug" size={16} />
        </span>
        <div className="ky-settings-list-row__copy">
          <span className="ky-settings-list-row__title">{srv.name}</span>
          <span
            className="ky-settings-list-row__desc"
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: "6px",
              fontFamily: "var(--font-mono)",
            }}
          >
            <span
              className={`ky-status-dot ${
                srv.enabled ? "ky-status-dot--ok" : "ky-status-dot--muted"
              }`}
            />
            <span>
              {srv.transport} · {srv.commandOrUrl} {srv.args.join(" ")}
            </span>
          </span>
        </div>
      </div>
      <div className="ky-settings-list-row__right">
        <Toggle
          label=""
          aria-label={`Enable ${srv.name}`}
          checked={srv.enabled}
          onChange={(enabled) =>
            setServers((prev) =>
              prev.map((item) => (item.id === srv.id ? { ...item, enabled } : item)),
            )
          }
        />
      </div>
    </div>
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-mcp-servers">
      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left">
          <span className="ky-settings-pill-counter">
            <span>MCP</span>
            <strong>{servers.length}</strong>
          </span>
        </div>
        <div className="ky-settings-toolbar__right">
          <Input
            aria-label="Search MCP servers"
            placeholder={t("ui.settings.capabilities.searchMcp", {
              defaultValue: "Search MCP servers...",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>{t("ui.settings.capabilities.installed", { defaultValue: "Installed" })}</span>
          <span className="ky-settings-group-header__count">{installed.length}</span>
        </h3>
        <Button size="sm" variant="secondary" onClick={() => setCreating(true)}>
          <ChromeIcon name="plus" size={14} />
          <span>{t("ui.settings.capabilities.newMcp", { defaultValue: "New MCP server" })}</span>
        </Button>
      </div>

      <div className="ky-settings-list">{installed.map(renderRow)}</div>

      {pluginServers.length > 0 && (
        <>
          <div className="ky-settings-group-header">
            <h3 className="ky-settings-group-header__title">
              <span>Plugin MCP Servers</span>
              <span className="ky-settings-group-header__count">{pluginServers.length}</span>
            </h3>
          </div>
          <div className="ky-settings-list">{pluginServers.map(renderRow)}</div>
        </>
      )}
    </div>
  );
}
