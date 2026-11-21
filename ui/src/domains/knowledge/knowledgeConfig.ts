/**
 * KnowledgePack Domain Helpers: SHA-256 Provenance, Quarantine, & Duplicate Detection
 * (UIR-7.1, Blueprint §12.2, D-41 internal profile, E2E-14).
 *
 * Pure TypeScript module (import-type only) for direct Vite & Node 26 testing.
 */

export type KnowledgeIntegrityState = "verified" | "quarantined" | "pending-ingest";

export interface KnowledgeSourceItem {
  readonly sourceId: string;
  readonly title: string;
  readonly originUri: string;
  readonly sha256: string;
  readonly byteSize: number;
  readonly importedAt: string;
  readonly rawPreviewExcerpt: string;
  readonly isDuplicateOfSourceId?: string;
}

export interface KnowledgePackVersionRecord {
  readonly versionNumber: number;
  readonly createdAt: string;
  readonly manifestSha256: string;
  readonly sourceCount: number;
  readonly summary: string;
}

export interface KnowledgePackRecord {
  readonly packId: string;
  readonly name: string;
  readonly description: string;
  readonly versionNumber: number;
  readonly profile: "uawp-kp-internal-digest-v1";
  readonly integrityState: KnowledgeIntegrityState;
  readonly manifestSha256: string;
  readonly signatureStatus: "signed-local-key" | "unsigned-local" | "tamper-quarantined";
  readonly lastIngestionJobId: string;
  readonly updatedAt: string;
  readonly sources: readonly KnowledgeSourceItem[];
  readonly versions: readonly KnowledgePackVersionRecord[];
}

export interface DuplicateSourceCheckResult {
  readonly hasDuplicates: boolean;
  readonly duplicatePairs: readonly {
    readonly sourceId: string;
    readonly duplicateOfSourceId: string;
    readonly sha256: string;
  }[];
}

export const INITIAL_KNOWLEDGE_PACKS: readonly KnowledgePackRecord[] = [
  {
    packId: "kp-core-contracts",
    name: "Karyora Core Architecture & Contracts",
    description:
      "Authoritative OpenAPI contracts, D-37 OS-keyring spec, and AIJobRelay state invariants.",
    versionNumber: 3,
    profile: "uawp-kp-internal-digest-v1",
    integrityState: "verified",
    manifestSha256:
      "sha256:c4f81290ab7734de0123456789abcdef0123456789abcdef0123456789abcde1",
    signatureStatus: "signed-local-key",
    lastIngestionJobId: "job-0002",
    updatedAt: "12m ago",
    sources: [
      {
        sourceId: "src-openapi",
        title: "contracts/openapi.yaml",
        originUri: "workspace://contracts/openapi.yaml",
        sha256: "sha256:11aa22bb33cc44dd55ee66ff7788990011aa22bb33cc44dd55ee66ff77889900",
        byteSize: 84_210,
        importedAt: "2026-11-22 00:05",
        rawPreviewExcerpt:
          "openapi: 3.1.0 — 76 canonical Core HTTP/SSE endpoints with RFC 9457 error envelopes.",
      },
      {
        sourceId: "src-security-policy",
        title: "docs/security/d37-keyring.md",
        originUri: "workspace://docs/security/d37-keyring.md",
        sha256: "sha256:22bb33cc44dd55ee66ff7788990011aa22bb33cc44dd55ee66ff7788990011aa",
        byteSize: 12_400,
        importedAt: "2026-11-22 00:08",
        rawPreviewExcerpt:
          "D-37 Policy: Provider API keys persist strictly inside OS Keyring (Keychain / Secret Service / Credential Manager).",
      },
    ],
    versions: [
      {
        versionNumber: 2,
        createdAt: "2026-11-21 19:00",
        manifestSha256:
          "sha256:99887766554433221100ffeeddccbbaa99887766554433221100ffeeddccbbaa",
        sourceCount: 1,
        summary: "Imported OpenAPI 0.1.0 contract snapshot",
      },
      {
        versionNumber: 3,
        createdAt: "2026-11-22 00:08",
        manifestSha256:
          "sha256:c4f81290ab7734de0123456789abcdef0123456789abcdef0123456789abcde1",
        sourceCount: 2,
        summary: "Added D-37 OS-keyring security reference",
      },
    ],
  },
  {
    packId: "kp-quarantined-sample",
    name: "Untrusted External Archive (Sample)",
    description:
      "Demonstrates Core fail-closed quarantine when checksum verification detects a mismatch.",
    versionNumber: 1,
    profile: "uawp-kp-internal-digest-v1",
    integrityState: "quarantined",
    manifestSha256:
      "sha256:deadbeef0000111122223333444455556666777788889999aaaabbbbccccdddd",
    signatureStatus: "tamper-quarantined",
    lastIngestionJobId: "job-0002",
    updatedAt: "1h ago",
    sources: [
      {
        sourceId: "src-untrusted",
        title: "external-notes.html",
        originUri: "import://archive/external-notes.html",
        sha256: "sha256:0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff",
        byteSize: 4_120,
        importedAt: "2026-11-21 23:15",
        rawPreviewExcerpt:
          "<script>alert(1)</script>Checksum mismatch detected during untrusted archive import; excluded from Context Engine.",
      },
    ],
    versions: [
      {
        versionNumber: 1,
        createdAt: "2026-11-21 23:15",
        manifestSha256:
          "sha256:deadbeef0000111122223333444455556666777788889999aaaabbbbccccdddd",
        sourceCount: 1,
        summary: "Quarantined on SHA-256 verification failure",
      },
    ],
  },
];

/**
 * Validates SHA-256 digest format (`sha256:<64 hex chars>`).
 */
export function isValidSha256Digest(digest: string): boolean {
  return /^sha256:[a-f0-9]{64}$/i.test(digest.trim());
}

/**
 * Detects duplicate sources inside a KnowledgePack by SHA-256 content hash (`Blueprint §12.2`).
 */
export function detectDuplicateSources(
  sources: readonly KnowledgeSourceItem[],
): DuplicateSourceCheckResult {
  const seenByHash = new Map<string, string>();
  const duplicatePairs: {
    sourceId: string;
    duplicateOfSourceId: string;
    sha256: string;
  }[] = [];

  for (const src of sources) {
    const key = src.sha256.toLowerCase();
    const existingId = seenByHash.get(key);
    if (existingId) {
      duplicatePairs.push({
        sourceId: src.sourceId,
        duplicateOfSourceId: existingId,
        sha256: src.sha256,
      });
    } else {
      seenByHash.set(key, src.sourceId);
    }
  }

  return {
    hasDuplicates: duplicatePairs.length > 0,
    duplicatePairs,
  };
}
