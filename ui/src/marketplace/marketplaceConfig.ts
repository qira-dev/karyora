/**
 * Plugin Marketplace Catalog & Core WASM `pluginhost` Governance Helpers
 * (UIR-6.3, Screenshots #34–#36, Blueprint §11, Quality Gate G-12, E2E-12).
 *
 * Pure TypeScript module (import-type only) so both Vite and Node 26 native tests import it directly.
 */

export type MarketplaceTabId = "public" | "community" | "personal";

export type PluginCategory =
  | "All"
  | "App plugins"
  | "Developer Tools"
  | "Security & Audit"
  | "Knowledge & Docs";

export type PluginSignatureTrust =
  | "verified-official"
  | "verified-community"
  | "unsigned-review-required"
  | "local-personal";

export type PluginLifecycleState =
  | "available"
  | "installed-enabled"
  | "installed-disabled"
  | "revoked";

export interface PluginPermissionGrant {
  readonly scope: string;
  readonly risk: "low" | "medium" | "high";
  readonly description: string;
}

export interface PluginGovernanceMetadata {
  readonly manifestSha256: string;
  readonly signatureTrust: PluginSignatureTrust;
  readonly abiVersion: string;
  readonly abiCompatible: boolean;
  readonly runtimeSandbox: "wazero-wasm-isolated";
  readonly invocationSupportedByCore: boolean;
  readonly cpuBudgetMs: number;
  readonly memoryCeilingMiB: number;
  readonly crashCount: number;
  readonly permissions: readonly PluginPermissionGrant[];
  readonly diagnosticLogs: readonly string[];
}

export interface MarketplacePluginRecord {
  readonly pluginId: string;
  readonly name: string;
  readonly version: string;
  readonly publisher: string;
  readonly summary: string;
  readonly catalogTab: MarketplaceTabId;
  readonly category: Exclude<PluginCategory, "All">;
  readonly isBuiltIn: boolean;
  readonly lifecycleState: PluginLifecycleState;
  readonly governance: PluginGovernanceMetadata;
}

export interface PluginInstallPolicyVerdict {
  readonly allowed: boolean;
  readonly requiresExplicitTrustReview: boolean;
  readonly blockReason?: string;
}

export const MARKETPLACE_TABS: readonly {
  readonly id: MarketplaceTabId;
  readonly label: string;
}[] = [
  { id: "public", label: "Public" },
  { id: "community", label: "Community" },
  { id: "personal", label: "Personal" },
];

export const MARKETPLACE_CATEGORIES: readonly PluginCategory[] = [
  "All",
  "App plugins",
  "Developer Tools",
  "Security & Audit",
  "Knowledge & Docs",
];

/**
 * Authoritative Plugin Marketplace entries matching Screenshots #34–#36 & Phase 4 Plan §3.1.
 */
export const INITIAL_MARKETPLACE_PLUGINS: readonly MarketplacePluginRecord[] = [
  {
    pluginId: "plugin-documents",
    name: "Documents",
    version: "1.4.0",
    publisher: "Karyora Core",
    summary: "Sandboxed DOCX viewer & outline navigator with zero macro execution.",
    catalogTab: "public",
    category: "App plugins",
    isBuiltIn: true,
    lifecycleState: "installed-enabled",
    governance: {
      manifestSha256: "sha256:4a7f9021bc8e31d05c11a8b42f19002e4b7799aa2c3e10ff90123456789abc01",
      signatureTrust: "verified-official",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 1500,
      memoryCeilingMiB: 64,
      crashCount: 0,
      permissions: [
        { scope: "workspace:read-selected", risk: "low", description: "Read user-opened DOCX files only" },
        { scope: "net:none", risk: "low", description: "Zero network egress permitted" },
      ],
      diagnosticLogs: ["[00:10:02] Manifest SHA-256 verified against Core pin."],
    },
  },
  {
    pluginId: "plugin-pdf",
    name: "PDF",
    version: "1.4.0",
    publisher: "Karyora Core",
    summary: "Zero-script PDF page inspector with zoom and selection-to-chat.",
    catalogTab: "public",
    category: "App plugins",
    isBuiltIn: true,
    lifecycleState: "installed-enabled",
    governance: {
      manifestSha256: "sha256:88c1e451de7a2204f9910ac21b552310aa1290cd44332211ffeeddccbbaa0012",
      signatureTrust: "verified-official",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 2000,
      memoryCeilingMiB: 96,
      crashCount: 0,
      permissions: [
        { scope: "workspace:read-selected", risk: "low", description: "Read user-opened PDF files only" },
        { scope: "script:disabled", risk: "low", description: "PDF JS actions stripped (G-12)" },
      ],
      diagnosticLogs: ["[00:10:03] Zero-script policy active."],
    },
  },
  {
    pluginId: "plugin-presentations",
    name: "Presentations",
    version: "1.3.2",
    publisher: "Karyora Core",
    summary: "Sandboxed PPTX 16:9 slide deck viewer with speaker notes.",
    catalogTab: "public",
    category: "App plugins",
    isBuiltIn: true,
    lifecycleState: "installed-enabled",
    governance: {
      manifestSha256: "sha256:9912bc7788aabbccddeeff00112233445566778899aabbccddeeff0011223344",
      signatureTrust: "verified-official",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 1500,
      memoryCeilingMiB: 64,
      crashCount: 0,
      permissions: [
        { scope: "workspace:read-selected", risk: "low", description: "Read user-opened PPTX decks only" },
      ],
      diagnosticLogs: ["[00:10:04] Sandbox quota healthy."],
    },
  },
  {
    pluginId: "plugin-spreadsheets",
    name: "Spreadsheets",
    version: "1.3.2",
    publisher: "Karyora Core",
    summary: "Formula-inert XLSX grid previewer blocking DDE and macro evaluation.",
    catalogTab: "public",
    category: "App plugins",
    isBuiltIn: true,
    lifecycleState: "installed-enabled",
    governance: {
      manifestSha256: "sha256:5523aa11bb22cc33dd44ee55ff6677889900aabbccddeeff1122334455667788",
      signatureTrust: "verified-official",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 1500,
      memoryCeilingMiB: 64,
      crashCount: 0,
      permissions: [
        { scope: "workspace:read-selected", risk: "low", description: "Read user-opened XLSX workbooks" },
        { scope: "formula:inert", risk: "low", description: "Macros and external formulas neutralized" },
      ],
      diagnosticLogs: ["[00:10:05] Formula sanitizer verified."],
    },
  },
  {
    pluginId: "plugin-dev-team",
    name: "Dev Team",
    version: "2.1.0",
    publisher: "Karyora Official",
    summary: "Multi-agent subagent presets for architecture review, TDD, and PR triage.",
    catalogTab: "public",
    category: "Developer Tools",
    isBuiltIn: false,
    lifecycleState: "available",
    governance: {
      manifestSha256: "sha256:11223344556677889900aabbccddeeff00112233445566778899aabbccddeeff",
      signatureTrust: "verified-official",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 3000,
      memoryCeilingMiB: 128,
      crashCount: 0,
      permissions: [
        { scope: "subagents:register-presets", risk: "medium", description: "Registers review subagent presets" },
        { scope: "workspace:read", risk: "medium", description: "Read active workspace source files" },
      ],
      diagnosticLogs: ["[ready] Signed manifest verified."],
    },
  },
  {
    pluginId: "plugin-compound-engineering",
    name: "Compound Engineering",
    version: "1.8.4",
    publisher: "Community Verified",
    summary: "Structured plan-build-verify workflows, checklists, and architecture decision helpers.",
    catalogTab: "community",
    category: "Developer Tools",
    isBuiltIn: false,
    lifecycleState: "available",
    governance: {
      manifestSha256: "sha256:aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
      signatureTrust: "verified-community",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 2500,
      memoryCeilingMiB: 96,
      crashCount: 0,
      permissions: [
        { scope: "workflows:templates", risk: "low", description: "Provides versioned workflow DAG templates" },
      ],
      diagnosticLogs: ["[ready] Community signature verified."],
    },
  },
  {
    pluginId: "plugin-git-commit",
    name: "Git Commit",
    version: "1.2.1",
    publisher: "Community",
    summary: "Conventional commit message generator and diff pre-commit checklist helper.",
    catalogTab: "community",
    category: "Developer Tools",
    isBuiltIn: false,
    lifecycleState: "available",
    governance: {
      manifestSha256: "sha256:ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100",
      signatureTrust: "unsigned-review-required",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 1000,
      memoryCeilingMiB: 48,
      crashCount: 0,
      permissions: [
        { scope: "git:diff-read", risk: "medium", description: "Reads staged git diff metadata" },
      ],
      diagnosticLogs: ["[notice] Unsigned community package — explicit review required before install."],
    },
  },
  {
    pluginId: "plugin-local-audit-pack",
    name: "Local Audit Formatter",
    version: "0.9.0-local",
    publisher: "This computer (Personal)",
    summary: "Personal workspace plugin formatting redacted audit bundles for internal review.",
    catalogTab: "personal",
    category: "Security & Audit",
    isBuiltIn: false,
    lifecycleState: "installed-disabled",
    governance: {
      manifestSha256: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      signatureTrust: "local-personal",
      abiVersion: "karyora-wasm-abi-v1",
      abiCompatible: true,
      runtimeSandbox: "wazero-wasm-isolated",
      invocationSupportedByCore: false,
      cpuBudgetMs: 1000,
      memoryCeilingMiB: 64,
      crashCount: 1,
      permissions: [
        { scope: "audit:read-redacted", risk: "low", description: "Reads redacted audit summaries only" },
      ],
      diagnosticLogs: [
        "[warn] Previous sandbox invocation hit 1000ms CPU ceiling and was cleanly isolated.",
      ],
    },
  },
];

/**
 * Filters marketplace catalog plugins by active tab, category pill, and search string.
 */
export function filterMarketplacePlugins(
  plugins: readonly MarketplacePluginRecord[],
  tab: MarketplaceTabId,
  category: PluginCategory,
  searchQuery: string,
): readonly MarketplacePluginRecord[] {
  const q = searchQuery.trim().toLowerCase();
  return plugins.filter((p) => {
    if (p.catalogTab !== tab) return false;
    if (category !== "All" && p.category !== category) return false;
    if (!q) return true;
    return (
      p.name.toLowerCase().includes(q) ||
      p.summary.toLowerCase().includes(q) ||
      p.publisher.toLowerCase().includes(q)
    );
  });
}

/**
 * Enforces G-12 WASM `pluginhost` install and trust boundaries before allowing install.
 * Blocks incompatible ABI, invalid SHA-256 digests, and revoked plugins.
 */
export function evaluatePluginInstallPolicy(
  plugin: MarketplacePluginRecord,
): PluginInstallPolicyVerdict {
  if (plugin.lifecycleState === "revoked") {
    return {
      allowed: false,
      requiresExplicitTrustReview: true,
      blockReason: "PLUGIN_REVOKED_BY_TRUST_POLICY",
    };
  }
  if (!plugin.governance.abiCompatible) {
    return {
      allowed: false,
      requiresExplicitTrustReview: true,
      blockReason: `INCOMPATIBLE_WASM_ABI_${plugin.governance.abiVersion}`,
    };
  }
  if (!/^sha256:[a-f0-9]{64}$/i.test(plugin.governance.manifestSha256)) {
    return {
      allowed: false,
      requiresExplicitTrustReview: true,
      blockReason: "INVALID_MANIFEST_SHA256_DIGEST",
    };
  }
  const requiresReview =
    plugin.governance.signatureTrust === "unsigned-review-required" ||
    plugin.governance.permissions.some((perm) => perm.risk === "high");

  return {
    allowed: true,
    requiresExplicitTrustReview: requiresReview,
  };
}
