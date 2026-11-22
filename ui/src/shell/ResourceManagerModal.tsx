/**
 * Resource Manager Modal (1:1 ZCode v3.18.2 `resource-manager.html` / `resourceManager.*` parity).
 *
 * Inspects local app Processes (CPU / Memory by category), Network request log,
 * and Local Storage footprint without exposing secrets (G-11).
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Input } from "../components/primitives";

export interface ResourceManagerModalProps {
  readonly isOpen: boolean;
  readonly onClose: () => void;
}

interface ProcessMetricRow {
  readonly pid: number;
  readonly name: string;
  readonly category: "Base services" | "Built-in plugins" | "Task processes";
  readonly cpuPercent: string;
  readonly memoryMb: number;
}

const SAMPLE_PROCESSES: readonly ProcessMetricRow[] = [
  { pid: 14012, name: "Karyora Core Daemon (Go)", category: "Base services", cpuPercent: "0.8%", memoryMb: 42 },
  { pid: 14028, name: "Karyora UI Renderer (SPA)", category: "Base services", cpuPercent: "1.4%", memoryMb: 118 },
  { pid: 14041, name: "Sandboxed Document Preview Worker", category: "Built-in plugins", cpuPercent: "0.1%", memoryMb: 24 },
  { pid: 14055, name: "Diff & AST Highlight Worker", category: "Built-in plugins", cpuPercent: "0.2%", memoryMb: 19 },
  { pid: 14102, name: "Active Agent Task Session", category: "Task processes", cpuPercent: "0.4%", memoryMb: 31 },
];

const SAMPLE_STORAGE = [
  { category: "Settings, credentials & workspace", sizeLabel: "1.4 MB", note: "Keyring-referenced; zero plaintext secrets" },
  { category: "Session transcripts & SQLite WAL", sizeLabel: "18.6 MB", note: "Compactable via Archive management" },
  { category: "KnowledgePack verified caches", sizeLabel: "64.2 MB", note: "SHA-256 content-addressed" },
  { category: "Backups", sizeLabel: "9.8 MB", note: "Encrypted snapshot archive" },
] as const;

export function ResourceManagerModal({
  isOpen,
  onClose,
}: ResourceManagerModalProps): ReactElement | null {
  const [activeTab, setActiveTab] = useState<"processes" | "network" | "storage">("processes");
  const [filter, setFilter] = useState("");

  if (!isOpen) return null;

  const totalMemoryMb = SAMPLE_PROCESSES.reduce((sum, p) => sum + p.memoryMb, 0);
  const filteredProcesses = SAMPLE_PROCESSES.filter(
    (p) =>
      !filter.trim() ||
      `${p.name} ${p.pid} ${p.category}`.toLowerCase().includes(filter.trim().toLowerCase()),
  );

  return (
    <div
      className="ky-modal-backdrop"
      role="presentation"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Resource Manager"
        data-testid="resource-manager-modal"
        className="ky-modal-dialog"
        style={{ maxWidth: "660px", display: "flex", flexDirection: "column", gap: "14px" }}
      >
        <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
            <ChromeIcon name="gauge" size={18} />
            <h2 style={{ margin: 0, fontSize: "1rem", fontWeight: 600 }}>
              Resource Manager
            </h2>
            <Badge tone="neutral" label={`${SAMPLE_PROCESSES.length} processes · ${totalMemoryMb} MB`} />
          </div>
          <Button size="sm" variant="ghost" aria-label="Close" onClick={onClose}>
            <ChromeIcon name="close" size={16} />
          </Button>
        </header>

        <div style={{ display: "flex", gap: "6px", borderBottom: "1px solid var(--border-subtle)", paddingBottom: "8px" }}>
          {(["processes", "network", "storage"] as const).map((tab) => (
            <Button
              key={tab}
              size="sm"
              variant={activeTab === tab ? "secondary" : "ghost"}
              selected={activeTab === tab}
              onClick={() => setActiveTab(tab)}
            >
              {tab === "processes" ? "CPU & Memory" : tab === "network" ? "Network" : "Storage"}
            </Button>
          ))}
        </div>

        {activeTab === "processes" && (
          <>
            <Input
              aria-label="Filter processes"
              placeholder="Filter by process name, PID, or category..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
            <div className="ky-setting-group">
              {filteredProcesses.map((proc) => (
                <div key={proc.pid} className="ky-setting-row" style={{ padding: "10px 14px" }}>
                  <div className="ky-setting-row__info">
                    <span className="ky-setting-row__title">{proc.name}</span>
                    <span className="ky-setting-row__desc">
                      PID {proc.pid} · {proc.category}
                    </span>
                  </div>
                  <div style={{ display: "inline-flex", gap: "14px", fontFamily: "var(--font-mono)", fontSize: "0.78rem" }}>
                    <span>CPU {proc.cpuPercent}</span>
                    <span>{proc.memoryMb} MB</span>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}

        {activeTab === "network" && (
          <div className="ky-setting-group">
            <div className="ky-setting-row">
              <div className="ky-setting-row__info">
                <span className="ky-setting-row__title">Local Core Loopback Channel</span>
                <span className="ky-setting-row__desc">
                  Records local app requests while this window is open (keeps latest 1,000; credentials stripped).
                </span>
              </div>
              <Badge tone="success" dot label="127.0.0.1" />
            </div>
          </div>
        )}

        {activeTab === "storage" && (
          <div className="ky-setting-group">
            {SAMPLE_STORAGE.map((item) => (
              <div key={item.category} className="ky-setting-row" style={{ padding: "10px 14px" }}>
                <div className="ky-setting-row__info">
                  <span className="ky-setting-row__title">{item.category}</span>
                  <span className="ky-setting-row__desc">{item.note}</span>
                </div>
                <strong style={{ fontSize: "0.82rem", fontVariantNumeric: "tabular-nums" }}>
                  {item.sizeLabel}
                </strong>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default ResourceManagerModal;
