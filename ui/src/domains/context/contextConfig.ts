/**
 * Context Inspector Domain Helpers: Exact ContextSnapshot Items, Omission Reasons,
 * Visual Token Budget Breakdown & Honest Cost Display (UIR-7.3, Blueprint §12.4, E2E-16).
 *
 * Pure TypeScript module (import-type only) for direct Vite & Node 26 testing.
 */

export type ContextOmissionReason =
  | "Token budget exceeded"
  | "Scope excluded"
  | "Policy filtered"
  | "Forgotten memory";

export type CostReportingStatus = "Reported" | "Estimated" | "Not reported";

export interface ContextIncludedItem {
  readonly itemId: string;
  readonly category: "system" | "knowledge" | "memory" | "workspace-file" | "history";
  readonly label: string;
  readonly trustZone: "trusted-system" | "verified-knowledge" | "user-workspace";
  readonly tokens: number;
}

export interface ContextOmittedItem {
  readonly itemId: string;
  readonly label: string;
  readonly estimatedTokens: number;
  readonly reason: ContextOmissionReason;
  readonly detail: string;
}

export interface ContextSnapshotRecord {
  readonly snapshotId: string;
  readonly turnLabel: string;
  readonly assembledDigest: string;
  readonly modelId: string;
  readonly totalTokenBudget: number;
  readonly usedTokens: number;
  readonly costStatus: CostReportingStatus;
  readonly costDisplayValue: string;
  readonly includedItems: readonly ContextIncludedItem[];
  readonly omittedItems: readonly ContextOmittedItem[];
}

export interface TokenBudgetCategoryShare {
  readonly category: ContextIncludedItem["category"] | "headroom";
  readonly label: string;
  readonly tokens: number;
  readonly percent: number;
}

export const INITIAL_CONTEXT_SNAPSHOTS: readonly ContextSnapshotRecord[] = [
  {
    snapshotId: "snap-turn-2",
    turnLabel: "Turn #2 — Inspect planstore_test.go & Security MCP",
    assembledDigest:
      "sha256:6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
    modelId: "configured-default",
    totalTokenBudget: 128_000,
    usedTokens: 18_420,
    costStatus: "Not reported",
    costDisplayValue: "Not reported by provider adapter",
    includedItems: [
      {
        itemId: "ctx-sys",
        category: "system",
        label: "Karyora Policy & Tool Schema System Prompt",
        trustZone: "trusted-system",
        tokens: 4_200,
      },
      {
        itemId: "ctx-kp",
        category: "knowledge",
        label: "KnowledgePack: Karyora Core Architecture & Contracts (v3)",
        trustZone: "verified-knowledge",
        tokens: 6_150,
      },
      {
        itemId: "ctx-mem",
        category: "memory",
        label: "Scoped Memory: MEMORY.md & session-folder-selection.md",
        trustZone: "user-workspace",
        tokens: 920,
      },
      {
        itemId: "ctx-file",
        category: "workspace-file",
        label: "internal/plan/planstore_test.go",
        trustZone: "user-workspace",
        tokens: 3_650,
      },
      {
        itemId: "ctx-hist",
        category: "history",
        label: "Conversation History (Turns 1–2)",
        trustZone: "user-workspace",
        tokens: 3_500,
      },
    ],
    omittedItems: [
      {
        itemId: "omit-1",
        label: "legacy-v2-prototype-note.md",
        estimatedTokens: 480,
        reason: "Forgotten memory",
        detail: "Soft-excluded via Scoped Memory Forget action (restorable).",
      },
      {
        itemId: "omit-2",
        label: "Untrusted External Archive (Sample)",
        estimatedTokens: 3_100,
        reason: "Policy filtered",
        detail: "Quarantined KnowledgePack excluded fail-closed by Core verifier.",
      },
      {
        itemId: "omit-3",
        label: "Other Workspace: Marketing Site Draft",
        estimatedTokens: 5_200,
        reason: "Scope excluded",
        detail: "Outside active project scope (karyora_Platform).",
      },
      {
        itemId: "omit-4",
        label: "Historical Archive Turns #1–#40",
        estimatedTokens: 112_400,
        reason: "Token budget exceeded",
        detail: "Dropped deterministically to stay within 128,000-token context window.",
      },
    ],
  },
];

/**
 * Computes exact per-category token shares + remaining headroom for the visual budget bar.
 */
export function computeTokenBudgetBreakdown(
  snapshot: ContextSnapshotRecord,
): readonly TokenBudgetCategoryShare[] {
  const budget = Math.max(1, snapshot.totalTokenBudget);
  const totals: Record<ContextIncludedItem["category"], number> = {
    system: 0,
    knowledge: 0,
    memory: 0,
    "workspace-file": 0,
    history: 0,
  };
  for (const item of snapshot.includedItems) {
    totals[item.category] += item.tokens;
  }
  const used = Object.values(totals).reduce((sum, n) => sum + n, 0);
  const headroom = Math.max(0, budget - used);

  const labels: Record<ContextIncludedItem["category"] | "headroom", string> = {
    system: "System & Tools",
    knowledge: "KnowledgePack",
    memory: "Scoped Memory",
    "workspace-file": "Workspace Files",
    history: "Conversation",
    headroom: "Free Headroom",
  };

  return (
    [
      "system",
      "knowledge",
      "memory",
      "workspace-file",
      "history",
      "headroom",
    ] as const
  ).map((cat) => {
    const tokens = cat === "headroom" ? headroom : totals[cat];
    return {
      category: cat,
      label: labels[cat],
      tokens,
      percent: Math.round((tokens / budget) * 1000) / 10,
    };
  });
}
