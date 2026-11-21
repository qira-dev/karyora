/**
 * Activity & Audit Log Domain Helpers: 100,000+ Virtualized Cursor Stream,
 * Append-Only Correction Notes & Privacy-Safe Redacted Export
 * (UIR-7.3, Blueprint §12.5, Quality Gate G-11 & G-13, E2E-22).
 *
 * Pure TypeScript module (import-type only) for direct Vite & Node 26 testing.
 */

export type AuditOutcome = "allowed" | "denied" | "error" | "reconciled";

export interface AuditEventRecord {
  readonly eventId: string;
  readonly seq: number;
  readonly timestamp: string;
  readonly actor: string;
  readonly action: string;
  readonly resource: string;
  readonly outcome: AuditOutcome;
  readonly correlationId: string;
  readonly chainHash: string;
  readonly correctsEventId?: string;
  readonly payloadSummary: string;
  readonly payloadJson: string;
}

export interface AuditFilterState {
  readonly actor: string;
  readonly action: string;
  readonly outcome: AuditOutcome | "all";
  readonly correlationId: string;
  readonly query: string;
}

export interface AuditVirtualWindow {
  readonly startIndex: number;
  readonly endIndex: number;
  readonly topSpacerPx: number;
  readonly bottomSpacerPx: number;
  readonly visibleEvents: readonly AuditEventRecord[];
}

export const INITIAL_AUDIT_EVENTS: readonly AuditEventRecord[] = [
  {
    eventId: "aud-100003",
    seq: 100_003,
    timestamp: "2026-11-22T00:44:10Z",
    actor: "owner@local",
    action: "plugin.trust.checked",
    resource: "plugin-git-commit",
    outcome: "allowed",
    correlationId: "corr-plugin-01",
    chainHash: "sha256:90a1b2c3d4e5f60718293a4b5c6d7e8f",
    payloadSummary: "Verified WASM ABI karyora-wasm-abi-v1 and flagged unsigned review",
    payloadJson: JSON.stringify(
      { pluginId: "plugin-git-commit", abi: "karyora-wasm-abi-v1", reviewRequired: true },
      null,
      2,
    ),
  },
  {
    eventId: "aud-100002",
    seq: 100_002,
    timestamp: "2026-11-22T00:35:00Z",
    actor: "owner@local",
    action: "memory_item.forgotten",
    resource: "mem-proj-forgotten",
    outcome: "allowed",
    correlationId: "corr-mem-02",
    chainHash: "sha256:70f1e2d3c4b5a69788796a5b4c3d2e1f",
    payloadSummary: "Soft-excluded legacy-v2-prototype-note.md (restorable)",
    payloadJson: JSON.stringify(
      { memoryId: "mem-proj-forgotten", restorable: true },
      null,
      2,
    ),
  },
  {
    eventId: "aud-100001",
    seq: 100_001,
    timestamp: "2026-11-22T00:15:12Z",
    actor: "core-verifier",
    action: "knowledgepack.quarantined",
    resource: "kp-quarantined-sample",
    outcome: "denied",
    correlationId: "corr-kp-99",
    chainHash: "sha256:445566778899aabbccddeeff00112233",
    payloadSummary: "Blocked untrusted pack due to SHA-256 checksum mismatch",
    payloadJson: JSON.stringify(
      { packId: "kp-quarantined-sample", reason: "SHA256_MISMATCH" },
      null,
      2,
    ),
  },
];

/**
 * Filters an audit stream (up to 100,000+ rows) in a single linear pass.
 */
export function filterAuditEvents(
  events: readonly AuditEventRecord[],
  filter: AuditFilterState,
): readonly AuditEventRecord[] {
  const actorQ = filter.actor.trim().toLowerCase();
  const actionQ = filter.action.trim().toLowerCase();
  const corrQ = filter.correlationId.trim().toLowerCase();
  const textQ = filter.query.trim().toLowerCase();

  const result: AuditEventRecord[] = [];
  for (let i = 0; i < events.length; i += 1) {
    const ev = events[i];
    if (filter.outcome !== "all" && ev.outcome !== filter.outcome) continue;
    if (actorQ && !ev.actor.toLowerCase().includes(actorQ)) continue;
    if (actionQ && !ev.action.toLowerCase().includes(actionQ)) continue;
    if (corrQ && !ev.correlationId.toLowerCase().includes(corrQ)) continue;
    if (
      textQ &&
      !ev.payloadSummary.toLowerCase().includes(textQ) &&
      !ev.resource.toLowerCase().includes(textQ) &&
      !ev.eventId.toLowerCase().includes(textQ)
    ) {
      continue;
    }
    result.push(ev);
  }
  return result;
}

/**
 * O(1) virtual viewport slicer for 100,000+ Activity & Audit rows (`G-13` 60 fps).
 */
export function sliceAuditWindow(
  events: readonly AuditEventRecord[],
  scrollTopPx: number,
  viewportHeightPx: number,
  rowHeightPx: number,
  overscan = 8,
): AuditVirtualWindow {
  const total = events.length;
  if (total === 0 || rowHeightPx <= 0) {
    return {
      startIndex: 0,
      endIndex: 0,
      topSpacerPx: 0,
      bottomSpacerPx: 0,
      visibleEvents: [],
    };
  }
  const safeScroll = Math.max(0, scrollTopPx);
  const rawStart = Math.floor(safeScroll / rowHeightPx);
  const visibleCount = Math.ceil(Math.max(rowHeightPx, viewportHeightPx) / rowHeightPx);
  const startIndex = Math.max(0, rawStart - overscan);
  const endIndex = Math.min(total, rawStart + visibleCount + overscan);
  return {
    startIndex,
    endIndex,
    topSpacerPx: startIndex * rowHeightPx,
    bottomSpacerPx: Math.max(0, (total - endIndex) * rowHeightPx),
    visibleEvents: events.slice(startIndex, endIndex),
  };
}

/**
 * Appends an immutable correction note linked to `targetEventId` (append-only audit law).
 * Never mutates or deletes the original audit record.
 */
export function createAppendOnlyCorrectionEvent(
  existingEvents: readonly AuditEventRecord[],
  targetEventId: string,
  correctionNote: string,
  actor = "owner@local",
): AuditEventRecord {
  const maxSeq = existingEvents.reduce((max, ev) => (ev.seq > max ? ev.seq : max), 100_000);
  const nextSeq = maxSeq + 1;
  return {
    eventId: `aud-corr-${nextSeq}`,
    seq: nextSeq,
    timestamp: new Date().toISOString(),
    actor,
    action: "audit.correction_appended",
    resource: targetEventId,
    outcome: "reconciled",
    correlationId: `corr-fix-${nextSeq}`,
    chainHash: `sha256:appendonly${String(nextSeq).padStart(54, "0")}`,
    correctsEventId: targetEventId,
    payloadSummary: `Correction for ${targetEventId}: ${correctionNote.trim()}`,
    payloadJson: JSON.stringify(
      {
        correctsEventId: targetEventId,
        note: correctionNote.trim(),
        appendOnly: true,
      },
      null,
      2,
    ),
  };
}

/**
 * Exports audit records with privacy-safe redaction (`G-11` — scrubs bearer tokens,
 * credential-like strings, and private prompt fields).
 */
export function buildRedactedAuditExport(
  events: readonly AuditEventRecord[],
): string {
  const scrub = (value: string): string =>
    value
      .replace(/\bBearer\s+[A-Za-z0-9._-]+/gi, "Bearer [REDACTED]")
      .replace(/\b(secret|token|password)\s*[:=]\s*[^\s,"}]+/gi, "$1=[REDACTED]");

  const redacted = events.map((ev) => ({
    eventId: ev.eventId,
    seq: ev.seq,
    timestamp: ev.timestamp,
    actor: ev.actor,
    action: ev.action,
    resource: ev.resource,
    outcome: ev.outcome,
    correlationId: ev.correlationId,
    chainHash: ev.chainHash,
    correctsEventId: ev.correctsEventId ?? null,
    payloadSummary: scrub(ev.payloadSummary),
  }));

  return JSON.stringify(
    {
      exportedAt: new Date().toISOString(),
      redactionApplied: true,
      totalRecords: redacted.length,
      events: redacted,
    },
    null,
    2,
  );
}
