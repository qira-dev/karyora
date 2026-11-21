/**
 * Authoritative v3.0 Architectural Decisions & Domain Constants (UI-01 through UI-40).
 *
 * Governing Sources:
 * - Unified AI Work Platform/Software/Blueprints/UNIFIED_AI_WORK_PLATFORM_UI_MASTER_BLUEPRINT.md (v3.0)
 * - Unified AI Work Platform/Software/Roadmaps/UI Roadmap.md (v3.0)
 * - Unified AI Work Platform/Software/Planning/UI/UI Implementation Plan — Phase 1 (v3.0, UIR-0)
 */

/**
 * UI-03 / UI-16: One Shared React 19 + TypeScript + Vite SPA Across Four Target Platforms.
 */
export const TARGET_PLATFORMS = ["web", "windows", "macos", "linux"] as const;
export type TargetPlatform = (typeof TARGET_PLATFORMS)[number];

/**
 * UI-18 / G-10: 13 Locked End-User UI Locales (English source + 11 UI-18 locales + Bengali bn).
 */
export const SUPPORTED_LOCALES = [
  "en",
  "bn",
  "zh-CN",
  "es",
  "pt-BR",
  "ja",
  "ru",
  "ko",
  "de",
  "fr",
  "id",
  "tr",
  "vi",
] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

/**
 * UI-11 / UI-12 / UI-22: Locked BYOK Provider Presets (Perplexity is explicitly excluded).
 */
export const LOCKED_BYOK_PROVIDER_PRESETS = [
  {
    id: "openai",
    label: "OpenAI",
    defaultBaseUrl: "https://api.openai.com/v1",
    defaultApiFormat: "responses",
  },
  {
    id: "anthropic",
    label: "Anthropic",
    defaultBaseUrl: "https://api.anthropic.com/v1",
    defaultApiFormat: "anthropic-messages",
  },
  {
    id: "gemini",
    label: "Google Gemini",
    defaultBaseUrl: "https://generativelanguage.googleapis.com/v1beta",
    defaultApiFormat: "gemini-api",
  },
  {
    id: "qwen",
    label: "Alibaba Qwen / DashScope",
    defaultBaseUrl: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
    defaultApiFormat: "chat-completions",
  },
  {
    id: "glm",
    label: "GLM / Z.AI",
    defaultBaseUrl: "https://open.bigmodel.cn/api/paas/v4",
    defaultApiFormat: "chat-completions",
  },
  {
    id: "kimi",
    label: "Kimi",
    defaultBaseUrl: "https://api.moonshot.ai/v1",
    defaultApiFormat: "chat-completions",
  },
  {
    id: "minimax",
    label: "MiniMax",
    defaultBaseUrl: "https://api.minimax.io/v1",
    defaultApiFormat: "chat-completions",
  },
  {
    id: "deepseek",
    label: "DeepSeek",
    defaultBaseUrl: "https://api.deepseek.com/v1",
    defaultApiFormat: "chat-completions",
  },
  {
    id: "custom",
    label: "Custom API",
    defaultBaseUrl: "https://api.example.com/v1",
    defaultApiFormat: "chat-completions",
  },
] as const;

export type LockedByokProviderId =
  (typeof LOCKED_BYOK_PROVIDER_PRESETS)[number]["id"];

export const SUPPORTED_API_FORMATS = [
  "anthropic-messages",
  "chat-completions",
  "responses",
  "gemini-api",
] as const;
export type SupportedApiFormat = (typeof SUPPORTED_API_FORMATS)[number];

/**
 * UI-08: App Theme Modes and 10 Light / 10 Dark Code Themes (Screenshots #5–#9).
 */
export const APP_THEMES = ["dark", "light", "system"] as const;
export type AppTheme = (typeof APP_THEMES)[number];

export const INTERFACE_MODES = ["coding", "office"] as const;
export type InterfaceMode = (typeof INTERFACE_MODES)[number];

export const LIGHT_CODE_THEMES = [
  "GitHub",
  "One Light",
  "Solarized Light",
  "Catppuccin Latte",
  "Gruvbox Light",
  "Atom One Light",
  "Quiet Light",
  "Night Owl Light",
  "Vitesse Light",
  "Min Light",
] as const;
export type LightCodeTheme = (typeof LIGHT_CODE_THEMES)[number];

export const DARK_CODE_THEMES = [
  "GitHub Dark",
  "One Dark Pro",
  "Dracula",
  "Catppuccin Mocha",
  "Gruvbox Dark",
  "Tokyo Night",
  "Nord",
  "Monokai Pro",
  "Night Owl",
  "Vitesse Dark",
] as const;
export type DarkCodeTheme = (typeof DARK_CODE_THEMES)[number];

/**
 * UI-31 / UI-38: Complete 21-Section Overlay Settings Architecture (Screenshots #1–#60).
 */
export const SETTINGS_SECTIONS = [
  // Group 1: Basics (7 sections, Screenshots #1–#22)
  { id: "general", group: "basics", label: "General", screenshots: "#1–#4" },
  { id: "appearance", group: "basics", label: "Appearance", screenshots: "#5–#9" },
  { id: "model-settings", group: "basics", label: "Model settings", screenshots: "#10–#13" },
  { id: "browser-use", group: "basics", label: "Browser Use", screenshots: "#14–#15" },
  { id: "remote-devices", group: "basics", label: "Remote devices", screenshots: "#16–#19" },
  { id: "computer-use", group: "basics", label: "Computer Use", screenshots: "#20" },
  { id: "keyboard-shortcuts", group: "basics", label: "Keyboard Shortcuts", screenshots: "#21–#22" },

  // Group 2: Agent capabilities (9 sections, Screenshots #23–#53)
  { id: "personalization", group: "agent-capabilities", label: "Personalization", screenshots: "#23" },
  { id: "behavior-and-tools", group: "agent-capabilities", label: "Behavior and tools", screenshots: "#24–#25" },
  { id: "memory", group: "agent-capabilities", label: "Memory", screenshots: "#26–#28" },
  { id: "subagents", group: "agent-capabilities", label: "Subagents", screenshots: "#29–#33" },
  { id: "plugins", group: "agent-capabilities", label: "Plugins", screenshots: "#34–#35" },
  { id: "mcp-servers", group: "agent-capabilities", label: "MCP Servers", screenshots: "#37–#43" },
  { id: "skills", group: "agent-capabilities", label: "Skills", screenshots: "#44–#45" },
  { id: "commands", group: "agent-capabilities", label: "Commands", screenshots: "#46–#48" },
  { id: "hooks", group: "agent-capabilities", label: "Hooks", screenshots: "#49–#53" },

  // Group 3: Data and statistics & Karyora Governance (5 sections, Screenshots #54–#60)
  { id: "data-sync", group: "data-and-statistics", label: "Data sync", screenshots: "#54–#55" },
  { id: "usage-stats", group: "data-and-statistics", label: "Usage stats", screenshots: "#56–#57" },
  { id: "project-management", group: "data-and-statistics", label: "Project Management", screenshots: "#58" },
  { id: "archive-management", group: "data-and-statistics", label: "Archive management", screenshots: "#59" },
  { id: "shares-and-governance", group: "data-and-statistics", label: "Shares & Governance", screenshots: "#60" },
] as const;

export type SettingsSectionId = (typeof SETTINGS_SECTIONS)[number]["id"];
export type SettingsGroupId = (typeof SETTINGS_SECTIONS)[number]["group"];

/**
 * UI-37 / UI-39: Multi-Tab Side Pane Tab Types (Screenshots #71–#74, #78, #82–#83).
 */
export const SIDE_PANE_TAB_KINDS = [
  "files",
  "browser",
  "terminal",
  "review",
  "sites",
  "side-conversation",
  "subagents",
  "context-inspector",
  "plugin-documents",
  "plugin-pdf",
  "plugin-presentations",
  "plugin-spreadsheets",
] as const;
export type SidePaneTabKind = (typeof SIDE_PANE_TAB_KINDS)[number];

/**
 * Blueprint §20.4: Canonical 6-State Tool Execution Row Machine.
 */
export const TOOL_ROW_STATES = [
  "inputStreaming",
  "pendingApproval",
  "running",
  "success",
  "error",
  "cancelled",
] as const;
export type ToolRowState = (typeof TOOL_ROW_STATES)[number];

/**
 * Blueprint §6.2: Canonical 5-Outcome Command Semantics (S0–S4).
 */
export const COMMAND_OUTCOMES = ["S0", "S1", "S2", "S3", "S4"] as const;
export type CommandOutcome = (typeof COMMAND_OUTCOMES)[number];

/**
 * Blueprint §18: Complete 40-Decision Architectural Register (UI-01..UI-40).
 */
export const UI_DECISION_REGISTER_IDS = [
  "UI-01", "UI-02", "UI-03", "UI-04", "UI-05", "UI-06", "UI-07", "UI-08",
  "UI-09", "UI-10", "UI-11", "UI-12", "UI-13", "UI-14", "UI-15", "UI-16",
  "UI-17", "UI-18", "UI-19", "UI-20", "UI-21", "UI-22", "UI-23", "UI-24",
  "UI-25", "UI-26", "UI-27", "UI-28", "UI-29", "UI-30", "UI-31", "UI-32",
  "UI-33", "UI-34", "UI-35", "UI-36", "UI-37", "UI-38", "UI-39", "UI-40",
] as const;
export type UiDecisionId = (typeof UI_DECISION_REGISTER_IDS)[number];
