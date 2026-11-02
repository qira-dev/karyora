/**
 * The provider preset list (C-07 / SEL-3; UI-11 LOCKED).
 *
 * The list is a FROZEN option-set decision (the same class as the
 * accepted navigation labels): these are the only presets, in this
 * order, with no additions and no removals — Perplexity is deliberately
 * absent. The display names are fixed product identifiers of the named
 * providers (proper nouns, like the locale names in the appearance
 * settings), not translatable copy; the "Custom API" option label IS
 * user-facing copy and resolves through the locale resources.
 *
 * No endpoint URL, no API-format enum, and no capability is attached to
 * any preset: endpoint details and adapter support come from the Core
 * (O-1), and capability stays Unknown/Not verified until a real
 * verification exists (UI-13).
 *
 * The preset ids are the coded adapter register's ids
 * (internal/domain/provider contract.go) — the value the save command
 * sends as the connection's provider. The Kimi preset's id aligns with
 * the coded `kimi-moonshot` adapter id (the display name is unchanged);
 * the alignment is the coded-name rule applied to the wire value.
 */
export interface ProviderPreset {
  readonly id: string;
  /** Fixed display name (proper noun; not locale copy). */
  readonly name: string;
}

export const PROVIDER_PRESETS: readonly ProviderPreset[] = [
  { id: "openai", name: "OpenAI" },
  { id: "anthropic", name: "Anthropic" },
  { id: "google-gemini", name: "Google Gemini" },
  { id: "alibaba-qwen", name: "Alibaba Qwen" },
  { id: "glm-zai", name: "GLM/Z.AI" },
  { id: "kimi-moonshot", name: "Kimi" },
  { id: "minimax", name: "MiniMax" },
  { id: "deepseek", name: "DeepSeek" },
];

/** The Custom API sentinel id (its label is locale copy, not a preset name). */
export const CUSTOM_API_PRESET_ID = "custom";
