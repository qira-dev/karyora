/**
 * Developer Tools Side-Pane Tab (1:1 ZCode v3.18.2 `developer-tools` side-pane tab).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, Input } from "../components/primitives";

export interface DevRequestLogRow {
  readonly id: string;
  readonly time: string;
  readonly method: "GET" | "POST" | "SSE";
  readonly url: string;
  readonly status: string;
  readonly durationMs: number;
}

const INITIAL_DEV_LOGS: readonly DevRequestLogRow[] = [
  {
    id: "req-1",
    time: "Now",
    method: "GET",
    url: "/api/v3/connections",
    status: "200 OK (local)",
    durationMs: 4,
  },
  {
    id: "req-2",
    time: "Now",
    method: "GET",
    url: "/api/v3/conversations",
    status: "200 OK",
    durationMs: 7,
  },
  {
    id: "req-3",
    time: "Now",
    method: "SSE",
    url: "/api/v3/jobs/job-0001/events",
    status: "STREAM OPEN",
    durationMs: 12,
  },
  {
    id: "req-4",
    time: "Now",
    method: "POST",
    url: "/api/v3/context/snapshots",
    status: "200 OK",
    durationMs: 18,
  },
];

export function DeveloperToolsTab(): ReactElement {
  const [filterText, setFilterText] = useState("");
  const [logs, setLogs] = useState<readonly DevRequestLogRow[]>(INITIAL_DEV_LOGS);

  const visible = logs.filter((row) => {
    const q = filterText.trim().toLowerCase();
    if (!q) return true;
    return `${row.method} ${row.url} ${row.status}`.toLowerCase().includes(q);
  });

  return (
    <section
      className="ky-side-tool-page"
      data-testid="developer-tools-tab"
      aria-label="Developer tools inspector"
    >
      <div className="ky-side-tool-page__inner">
        <div className="ky-side-tool-page__head-row">
          <div>
            <h3 className="ky-side-tool-page__subhead">Developer Tools</h3>
            <p className="ky-setting-row__desc">
              Local Core HTTP/SSE request trace and runtime diagnostics (zero credentials logged).
            </p>
          </div>
          <div style={{ display: "inline-flex", gap: "6px" }}>
            <Badge tone="neutral" label={`${visible.length} entries`} />
            <Button size="sm" variant="secondary" onClick={() => setLogs([])}>
              Clear
            </Button>
          </div>
        </div>

        <Input
          aria-label="Filter developer network trace"
          placeholder="Filter by URL, method, or status..."
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
        />

        <div className="ky-setting-group">
          {visible.length === 0 ? (
            <div className="ky-setting-row">
              <span className="ky-setting-row__desc">
                Waiting for new local Core requests…
              </span>
            </div>
          ) : (
            visible.map((row) => (
              <div key={row.id} className="ky-setting-row" style={{ padding: "10px 14px" }}>
                <div style={{ display: "flex", alignItems: "center", gap: "10px", minWidth: 0 }}>
                  <Badge
                    tone={row.method === "SSE" ? "info" : row.method === "POST" ? "warning" : "neutral"}
                    label={row.method}
                  />
                  <code
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: "0.78rem",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {row.url}
                  </code>
                </div>
                <div style={{ display: "inline-flex", alignItems: "center", gap: "10px", flexShrink: 0 }}>
                  <span style={{ fontSize: "0.75rem", color: "var(--success-text)" }}>
                    {row.status}
                  </span>
                  <span style={{ fontSize: "0.72rem", color: "var(--text-muted)", fontVariantNumeric: "tabular-nums" }}>
                    {row.durationMs}ms
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </section>
  );
}

export default DeveloperToolsTab;
