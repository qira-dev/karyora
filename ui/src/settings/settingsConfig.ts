/**
 * Complete 21-Section Overlay Settings Hub Pure Helpers, Schemas & Validators
 * (UIR-8, Screenshots #1–#60, Blueprint §13, CR-P4-01, G-11, E2E-17..E2E-21).
 *
 * Pure TypeScript module (import-type only) so both Vite and Node 26 native tests import it directly.
 */

import type {
  LockedByokProviderId,
  SettingsGroupId,
  SupportedApiFormat,
} from "../types/decisions";

export interface SettingsGroupMeta {
  readonly id: SettingsGroupId;
  readonly label: string;
}

export const SETTINGS_GROUPS: readonly SettingsGroupMeta[] = [
  { id: "basics", label: "Basics" },
  { id: "agent-capabilities", label: "Agent capabilities" },
  { id: "data-and-statistics", label: "Data and statistics" },
];

export interface ShortcutBinding {
  readonly id: string;
  readonly scope: "Global" | "Composer";
  readonly action: string;
  readonly keys: string;
  readonly defaultKeys: string;
}

export const DEFAULT_KEYBOARD_SHORTCUTS: readonly ShortcutBinding[] = [
  { id: "sc-cmd", scope: "Global", action: "Open Command Center", keys: "Ctrl+K", defaultKeys: "Ctrl+K" },
  { id: "sc-sidebar", scope: "Global", action: "Toggle Left Sidebar", keys: "Ctrl+B", defaultKeys: "Ctrl+B" },
  { id: "sc-sidepane", scope: "Global", action: "Toggle Multi-Tab Side Pane", keys: "Ctrl+Alt+B", defaultKeys: "Ctrl+Alt+B" },
  { id: "sc-fullview", scope: "Global", action: "Toggle Side-Pane Full-View Mode", keys: "Ctrl+Shift+F", defaultKeys: "Ctrl+Shift+F" },
  { id: "sc-terminal", scope: "Global", action: "Toggle Docked PTY Terminal", keys: "Ctrl+J", defaultKeys: "Ctrl+J" },
  { id: "sc-settings", scope: "Global", action: "Open Settings Overlay", keys: "Ctrl+,", defaultKeys: "Ctrl+," },
  { id: "sc-theme", scope: "Global", action: "Toggle Light / Dark Theme", keys: "Ctrl+Shift+L", defaultKeys: "Ctrl+Shift+L" },
  { id: "sc-newtask", scope: "Global", action: "New Task", keys: "Ctrl+N", defaultKeys: "Ctrl+N" },
  { id: "sc-mode", scope: "Composer", action: "Cycle Session Mode (Default / Plan / Auto)", keys: "Ctrl+Shift+M", defaultKeys: "Ctrl+Shift+M" },
  { id: "sc-model", scope: "Composer", action: "Open Model Selector", keys: "Ctrl+M", defaultKeys: "Ctrl+M" },
  { id: "sc-thought", scope: "Composer", action: "Cycle Thought Level", keys: "Ctrl+T", defaultKeys: "Ctrl+T" },
];

/**
 * Detects duplicate keybinding conflicts and enforces that `Escape` can never be bound
 * to cancel a durable Job (`Blueprint §13.1 (7)`).
 */
export function detectShortcutConflicts(
  bindings: readonly ShortcutBinding[],
): readonly { readonly bindingId: string; readonly reason: string }[] {
  const conflicts: { bindingId: string; reason: string }[] = [];
  const seen = new Map<string, string>();

  for (const b of bindings) {
    const normalized = b.keys.trim().toLowerCase();
    if (!normalized) continue;
    if (normalized === "escape" || normalized === "esc") {
      conflicts.push({
        bindingId: b.id,
        reason: "Escape is reserved for closing overlays and never cancels a durable Job.",
      });
      continue;
    }
    const key = `${b.scope}:${normalized}`;
    const existing = seen.get(key);
    if (existing) {
      conflicts.push({
        bindingId: b.id,
        reason: `Conflicts with binding "${existing}" (${b.keys}).`,
      });
    } else {
      seen.set(key, b.action);
    }
  }
  return conflicts;
}

export interface ByokProviderProfile {
  readonly id: LockedByokProviderId;
  readonly label: string;
  readonly baseUrl: string;
  readonly apiFormat: SupportedApiFormat;
  readonly hasStoredKeyringSecret: boolean;
  readonly c319DataHandlingStatus:
    | "Eligible (Non-Personal Data)"
    | "Fail-Closed Review Required";
  readonly c319Note: string;
  readonly models: readonly CustomModelSpec[];
}

export interface CustomModelSpec {
  readonly modelId: string;
  readonly displayName: string;
  readonly contextWindow: number;
  readonly maxOutputTokens: number;
  readonly inputTypes: readonly ("text" | "image" | "pdf" | "audio")[];
  readonly capabilityStatus: "Verified" | "Not verified";
  readonly reasoningLevels: readonly ("low" | "medium" | "high")[];
}

export const INITIAL_BYOK_PROVIDERS: readonly ByokProviderProfile[] = [
  {
    id: "openai",
    label: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    apiFormat: "responses",
    hasStoredKeyringSecret: true,
    c319DataHandlingStatus: "Eligible (Non-Personal Data)",
    c319Note: "Dated C3-19 API review passed for non-personal egress; secret stored in OS keyring (D-37).",
    models: [
      {
        modelId: "gpt-5",
        displayName: "GPT-5",
        contextWindow: 256_000,
        maxOutputTokens: 16_384,
        inputTypes: ["text", "image"],
        capabilityStatus: "Verified",
        reasoningLevels: ["low", "medium", "high"],
      },
    ],
  },
  {
    id: "anthropic",
    label: "Anthropic",
    baseUrl: "https://api.anthropic.com/v1",
    apiFormat: "anthropic-messages",
    hasStoredKeyringSecret: true,
    c319DataHandlingStatus: "Eligible (Non-Personal Data)",
    c319Note: "Dated C3-19 commercial API review passed; D-37 OS keyring write-only.",
    models: [
      {
        modelId: "claude-sonnet",
        displayName: "Claude Sonnet",
        contextWindow: 200_000,
        maxOutputTokens: 16_384,
        inputTypes: ["text", "image", "pdf"],
        capabilityStatus: "Verified",
        reasoningLevels: ["low", "medium", "high"],
      },
    ],
  },
  {
    id: "gemini",
    label: "Google Gemini",
    baseUrl: "https://generativelanguage.googleapis.com/v1beta",
    apiFormat: "gemini-api",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Protected-data egress stays fail-closed until dated regional review is confirmed.",
    models: [
      {
        modelId: "gemini-pro",
        displayName: "Gemini Pro",
        contextWindow: 1_000_000,
        maxOutputTokens: 32_768,
        inputTypes: ["text", "image", "pdf", "audio"],
        capabilityStatus: "Not verified",
        reasoningLevels: ["low", "medium", "high"],
      },
    ],
  },
  {
    id: "qwen",
    label: "Alibaba Qwen / DashScope",
    baseUrl: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Protected-data egress blocked fail-closed by C3-19 policy.",
    models: [],
  },
  {
    id: "glm",
    label: "GLM / Z.AI",
    baseUrl: "https://open.bigmodel.cn/api/paas/v4",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Eligible (Non-Personal Data)",
    c319Note: "Non-personal egress eligible under dated C3-19 review.",
    models: [],
  },
  {
    id: "kimi",
    label: "Kimi",
    baseUrl: "https://api.moonshot.ai/v1",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Unknown review status; protected-data egress blocked by design.",
    models: [],
  },
  {
    id: "minimax",
    label: "MiniMax",
    baseUrl: "https://api.minimax.io/v1",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Unknown review status; protected-data egress blocked by design.",
    models: [],
  },
  {
    id: "deepseek",
    label: "DeepSeek",
    baseUrl: "https://api.deepseek.com/v1",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Unknown review status; protected-data egress blocked by design.",
    models: [],
  },
  {
    id: "custom",
    label: "Custom API",
    baseUrl: "https://api.example.com/v1",
    apiFormat: "chat-completions",
    hasStoredKeyringSecret: false,
    c319DataHandlingStatus: "Fail-Closed Review Required",
    c319Note: "Custom endpoints require explicit HTTPS or loopback validation.",
    models: [],
  },
];

/**
 * Validates BYOK provider endpoint & blocks disallowed/non-HTTPS remote endpoints (CR-P4-01).
 */
export function validateProviderEndpoint(baseUrl: string): {
  readonly valid: boolean;
  readonly error?: string;
} {
  const trimmed = baseUrl.trim();
  if (!trimmed) return { valid: false, error: "Base URL is required." };
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return { valid: false, error: "Base URL must be a valid URL." };
  }
  const isLoopback =
    parsed.hostname === "127.0.0.1" || parsed.hostname === "localhost";
  if (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && isLoopback)) {
    return {
      valid: false,
      error: "Remote provider endpoints require HTTPS (HTTP allowed only for loopback 127.0.0.1/localhost).",
    };
  }
  return { valid: true };
}

/**
 * Exact 8 Subagent Color Swatches (Screenshots #29–#33).
 */
export const SUBAGENT_COLOR_SWATCHES: readonly { readonly id: string; readonly hex: string }[] = [
  { id: "indigo", hex: "#6366f1" },
  { id: "blue", hex: "#3b82f6" },
  { id: "emerald", hex: "#10b981" },
  { id: "amber", hex: "#f59e0b" },
  { id: "rose", hex: "#f43f5e" },
  { id: "violet", hex: "#8b5cf6" },
  { id: "cyan", hex: "#06b6d4" },
  { id: "slate", hex: "#64748b" },
];

/**
 * Exact 7 Hook Lifecycle Events (Screenshots #49–#53).
 */
export const HOOK_LIFECYCLE_EVENTS = [
  "PreToolUse",
  "PostToolUse",
  "SessionStart",
  "SessionStop",
  "UserPromptSubmit",
  "SubagentStop",
  "Notification",
] as const;
export type HookLifecycleEvent = (typeof HOOK_LIFECYCLE_EVENTS)[number];

export type McpTransportType = "stdio" | "http" | "sse";

export interface McpServerConfigRecord {
  readonly id: string;
  readonly name: string;
  readonly source: "installed" | "plugin";
  readonly transport: McpTransportType;
  readonly commandOrUrl: string;
  readonly args: readonly string[];
  readonly enabled: boolean;
  readonly hasAuthHeaderSecret: boolean;
}

export const INITIAL_MCP_SERVERS: readonly McpServerConfigRecord[] = [
  { id: "mcp-9scan", name: "9Scan", source: "installed", transport: "stdio", commandOrUrl: "9scan-mcp", args: ["--stdio"], enabled: true, hasAuthHeaderSecret: false },
  { id: "mcp-obsidian", name: "Obsidian", source: "installed", transport: "http", commandOrUrl: "http://127.0.0.1:27123/mcp", args: [], enabled: true, hasAuthHeaderSecret: true },
  { id: "mcp-security", name: "Security", source: "installed", transport: "stdio", commandOrUrl: "security-scanner-mcp", args: [], enabled: true, hasAuthHeaderSecret: false },
  { id: "mcp-desktop", name: "Desktop-UI-MCP", source: "plugin", transport: "sse", commandOrUrl: "http://127.0.0.1:27299/sse", args: [], enabled: false, hasAuthHeaderSecret: false },
];

/**
 * Bidirectional `Form` <-> `JSON` MCP Server parser/serializer (Screenshots #37–#43).
 */
export function serializeMcpServerToJson(cfg: {
  readonly name: string;
  readonly transport: McpTransportType;
  readonly commandOrUrl: string;
  readonly args: readonly string[];
}): string {
  const payload =
    cfg.transport === "stdio"
      ? {
          mcpServers: {
            [cfg.name || "new-server"]: {
              transport: "stdio",
              command: cfg.commandOrUrl,
              args: cfg.args,
            },
          },
        }
      : {
          mcpServers: {
            [cfg.name || "new-server"]: {
              transport: cfg.transport,
              url: cfg.commandOrUrl,
            },
          },
        };
  return JSON.stringify(payload, null, 2);
}

export function parseMcpServerJson(rawJson: string): {
  readonly ok: true;
  readonly name: string;
  readonly transport: McpTransportType;
  readonly commandOrUrl: string;
  readonly args: readonly string[];
} | {
  readonly ok: false;
  readonly error: string;
} {
  try {
    const parsed = JSON.parse(rawJson) as Record<string, unknown>;
    const servers = parsed.mcpServers as Record<string, Record<string, unknown>> | undefined;
    if (!servers || typeof servers !== "object") {
      return { ok: false, error: 'JSON must include top-level "mcpServers" object.' };
    }
    const entries = Object.entries(servers);
    if (entries.length === 0) {
      return { ok: false, error: '"mcpServers" must contain at least one server definition.' };
    }
    const [name, def] = entries[0];
    const transport = (def.transport ?? (def.command ? "stdio" : "http")) as McpTransportType;
    if (transport !== "stdio" && transport !== "http" && transport !== "sse") {
      return { ok: false, error: `Unsupported transport: ${String(transport)}` };
    }
    const commandOrUrl =
      transport === "stdio"
        ? String(def.command ?? "")
        : String(def.url ?? "");
    const args = Array.isArray(def.args) ? def.args.map(String) : [];
    return { ok: true, name, transport, commandOrUrl, args };
  } catch (err) {
    return {
      ok: false,
      error: err instanceof Error ? err.message : "Invalid JSON syntax.",
    };
  }
}

/**
 * Usage Stats 5 Summary Cards, 12-Month Heatmap & Model Donut Data (Screenshots #56–#57).
 */
export interface UsageStatsSnapshot {
  readonly totalTokens: string;
  readonly peakTokens: string;
  readonly longestSession: string;
  readonly currentStreak: string;
  readonly longestStreak: string;
  readonly heatmapLevels: readonly number[]; // 84 cells (12 months x 7 slots)
  readonly dailyTrend7d: readonly { readonly day: string; readonly tokensK: number }[];
  readonly dailyTrend30d: readonly { readonly day: string; readonly tokensK: number }[];
  readonly modelShares: readonly { readonly model: string; readonly percent: number; readonly color: string }[];
}

export const INITIAL_USAGE_STATS: UsageStatsSnapshot = {
  totalTokens: "14.8M",
  peakTokens: "1.92M",
  longestSession: "2h 18m",
  currentStreak: "12 days",
  longestStreak: "27 days",
  heatmapLevels: Array.from({ length: 84 }, (_, idx) => (idx * 7 + 3) % 5),
  dailyTrend7d: [
    { day: "Mon", tokensK: 420 },
    { day: "Tue", tokensK: 680 },
    { day: "Wed", tokensK: 510 },
    { day: "Thu", tokensK: 890 },
    { day: "Fri", tokensK: 1120 },
    { day: "Sat", tokensK: 640 },
    { day: "Sun", tokensK: 940 },
  ],
  dailyTrend30d: Array.from({ length: 10 }, (_, idx) => ({
    day: `D${idx * 3 + 1}`,
    tokensK: 380 + ((idx * 173) % 860),
  })),
  modelShares: [
    { model: "Claude Sonnet", percent: 54, color: "#6366f1" },
    { model: "GPT-5", percent: 31, color: "#10b981" },
    { model: "Gemini Pro", percent: 15, color: "#f59e0b" },
  ],
};

/**
 * Builds a privacy-safe redacted Diagnostics Bundle (`#60` + `Blueprint §13.3`, `G-11`).
 * Strips credentials, bearer headers, and user prompt contents before preview/export.
 */
export function buildRedactedDiagnosticBundle(meta: {
  readonly runtimeTarget: string;
  readonly appVersion: string;
  readonly coreConnected: boolean;
  readonly rawNotes?: string;
}): string {
  const scrubbedNotes = (meta.rawNotes ?? "Clean runtime diagnostics")
    .replace(/\bBearer\s+[A-Za-z0-9._-]+/gi, "Bearer [REDACTED]")
    .replace(/\b(secret|token|password|key)\s*[:=]\s*[^\s,"}]+/gi, "$1=[REDACTED]");

  return JSON.stringify(
    {
      schema: "karyora-diagnostics-v1",
      generatedAt: new Date().toISOString(),
      redactionApplied: true,
      promptsIncluded: false,
      credentialsIncluded: false,
      runtimeTarget: meta.runtimeTarget,
      appVersion: meta.appVersion,
      coreConnected: meta.coreConnected,
      notes: scrubbedNotes,
    },
    null,
    2,
  );
}
