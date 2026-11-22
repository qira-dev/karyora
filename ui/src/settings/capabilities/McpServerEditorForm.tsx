/**
 * `New MCP server` Editor with Bidirectional `Form` | `JSON` Switch
 * (UIR-8.2, Screenshots #37–#43, Blueprint §13.2 (13), E2E-18).
 *
 * Supports `stdio`, `http` (with optional write-only auth header), and `sse`.
 */

import { useState, type ReactElement } from "react";
import { Button, Input, Select, Textarea } from "../../components/primitives";
import {
  parseMcpServerJson,
  serializeMcpServerToJson,
  type McpServerConfigRecord,
  type McpTransportType,
} from "../settingsConfig";

export interface McpServerEditorFormProps {
  readonly onSave: (server: McpServerConfigRecord) => void;
  readonly onCancel: () => void;
}

export function McpServerEditorForm({
  onSave,
  onCancel,
}: McpServerEditorFormProps): ReactElement {
  const [editorMode, setEditorMode] = useState<"form" | "json">("form");
  const [name, setName] = useState("");
  const [transport, setTransport] = useState<McpTransportType>("stdio");
  const [commandOrUrl, setCommandOrUrl] = useState("");
  const [argsText, setArgsText] = useState("");
  const [authHeaderDraft, setAuthHeaderDraft] = useState("");
  const [jsonBuffer, setJsonBuffer] = useState("");
  const [jsonError, setJsonError] = useState<string | null>(null);

  const switchToJson = () => {
    setJsonBuffer(
      serializeMcpServerToJson({
        name: name || "custom-mcp",
        transport,
        commandOrUrl,
        args: argsText
          .split(/\s+/)
          .map((s) => s.trim())
          .filter(Boolean),
      }),
    );
    setJsonError(null);
    setEditorMode("json");
  };

  const switchToForm = () => {
    const parsed = parseMcpServerJson(jsonBuffer);
    if (!parsed.ok) {
      setJsonError(parsed.error);
      return;
    }
    setName(parsed.name);
    setTransport(parsed.transport);
    setCommandOrUrl(parsed.commandOrUrl);
    setArgsText(parsed.args.join(" "));
    setJsonError(null);
    setEditorMode("form");
  };

  const handleSave = () => {
    if (editorMode === "json") {
      const parsed = parseMcpServerJson(jsonBuffer);
      if (!parsed.ok) {
        setJsonError(parsed.error);
        return;
      }
      onSave({
        id: `mcp-${Date.now()}`,
        name: parsed.name,
        source: "installed",
        transport: parsed.transport,
        commandOrUrl: parsed.commandOrUrl,
        args: parsed.args,
        enabled: true,
        hasAuthHeaderSecret: authHeaderDraft.trim().length > 0,
      });
      setAuthHeaderDraft("");
      return;
    }

    if (!name.trim() || !commandOrUrl.trim()) return;
    onSave({
      id: `mcp-${Date.now()}`,
      name: name.trim(),
      source: "installed",
      transport,
      commandOrUrl: commandOrUrl.trim(),
      args: argsText
        .split(/\s+/)
        .map((s) => s.trim())
        .filter(Boolean),
      enabled: true,
      hasAuthHeaderSecret: authHeaderDraft.trim().length > 0,
    });
    setAuthHeaderDraft("");
  };

  return (
    <div className="ky-settings-card" data-testid="mcp-server-editor-form">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: "var(--space-2)" }}>
        <h4 className="ky-settings-card__title">New MCP server</h4>
        <div className="ky-segmented-tabs" role="tablist" aria-label="MCP editor mode">
          <button
            type="button"
            role="tab"
            aria-selected={editorMode === "form"}
            className="ky-segmented-tabs__btn"
            data-active={editorMode === "form" ? "true" : "false"}
            onClick={() => (editorMode === "json" ? switchToForm() : undefined)}
          >
            Form
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={editorMode === "json"}
            className="ky-segmented-tabs__btn"
            data-active={editorMode === "json" ? "true" : "false"}
            onClick={() => (editorMode === "form" ? switchToJson() : undefined)}
          >
            JSON
          </button>
        </div>
      </div>

      {editorMode === "form" ? (
        <>
          <div className="ky-settings-grid-2">
            <Input
              label="Server name"
              placeholder="e.g. Postgres MCP"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <Select
              label="Transport"
              value={transport}
              options={[
                { value: "stdio", label: "stdio (Local process)" },
                { value: "http", label: "HTTP (Streamable HTTP)" },
                { value: "sse", label: "SSE (Server-Sent Events)" },
              ]}
              onChange={(e) => setTransport(e.target.value as McpTransportType)}
            />
          </div>
          <Input
            label={transport === "stdio" ? "Executable command" : "Server endpoint URL"}
            placeholder={
              transport === "stdio"
                ? "e.g. npx @modelcontextprotocol/server-filesystem"
                : "https://mcp.example.com/v1"
            }
            value={commandOrUrl}
            onChange={(e) => setCommandOrUrl(e.target.value)}
          />
          {transport === "stdio" ? (
            <Input
              label="Arguments (space-separated)"
              placeholder="--stdio --readonly"
              value={argsText}
              onChange={(e) => setArgsText(e.target.value)}
            />
          ) : (
            <Input
              label="Authorization Header Secret (OS Keyring)"
              writeOnlySecret={true}
              placeholder="Optional bearer credential..."
              value={authHeaderDraft}
              onChange={(e) => setAuthHeaderDraft(e.target.value)}
            />
          )}
        </>
      ) : (
        <Textarea
          label="MCP Server JSON Configuration"
          rows={8}
          value={jsonBuffer}
          errorText={jsonError ?? undefined}
          onChange={(e) => {
            setJsonBuffer(e.target.value);
            setJsonError(null);
          }}
        />
      )}

      <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" variant="primary" onClick={handleSave}>
          Save MCP server
        </Button>
      </div>
    </div>
  );
}
