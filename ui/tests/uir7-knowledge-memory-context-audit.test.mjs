/**
 * Phase 4 Milestone UIR-7 Verification Tests:
 * - UIR-7.1 (Blueprint §12.2, E2E-14): KnowledgePack SHA-256 verification, duplicate detection, & G-12 sanitized preview
 * - UIR-7.2 (Screenshots #26–#28, CR-P4-02, G-09, E2E-15): Scoped Memory review lifecycle & Forget vs Delete distinctness
 * - UIR-7.3 (Blueprint §12.4–§12.5, G-11, G-13, E2E-16, E2E-22): Context Inspector budget/omissions & 100,000+ Audit virtualization
 */

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const UI_ROOT = path.resolve(__dirname, "..");

test("UIR-7.1 (Blueprint §12.2, E2E-14): KnowledgePack SHA-256 validation, duplicate detection, and sanitized source preview", async () => {
  const {
    INITIAL_KNOWLEDGE_PACKS,
    isValidSha256Digest,
    detectDuplicateSources,
  } = await import("../src/domains/knowledge/knowledgeConfig.ts");
  const { sanitizeDocumentPayload, assertZeroScriptExecution } = await import(
    "../src/previewers/previewerSanitizer.ts"
  );

  for (const pack of INITIAL_KNOWLEDGE_PACKS) {
    assert.equal(isValidSha256Digest(pack.manifestSha256), true);
  }
  assert.equal(isValidSha256Digest("sha256:short"), false);

  const dupReport = detectDuplicateSources([
    ...INITIAL_KNOWLEDGE_PACKS[0].sources,
    {
      ...INITIAL_KNOWLEDGE_PACKS[0].sources[0],
      sourceId: "src-dup-copy",
      title: "contracts/openapi-copy.yaml",
    },
  ]);
  assert.equal(dupReport.hasDuplicates, true);
  assert.equal(dupReport.duplicatePairs.length, 1);

  // Quarantined sample source excerpt gets sanitized cleanly (G-12)
  const quarantinedSrc = INITIAL_KNOWLEDGE_PACKS[1].sources[0];
  const sanitized = sanitizeDocumentPayload(quarantinedSrc.rawPreviewExcerpt);
  assert.equal(sanitized.hadExecutableRisk, true);
  assert.equal(assertZeroScriptExecution(sanitized.sanitizedText), true);
});

test("UIR-7.2 (Screenshots #26–#28, CR-P4-02, G-09, E2E-15): Scoped Memory enforces Forget (restorable) vs Delete (irreversible tombstone)", async () => {
  const {
    INITIAL_MEMORY_ITEMS,
    forgetMemoryItem,
    restoreMemoryItem,
    permanentlyDeleteMemoryItem,
    filterScopedMemoryItems,
  } = await import("../src/domains/memory/memoryConfig.ts");

  const activeItem = INITIAL_MEMORY_ITEMS.find((i) => i.memoryId === "mem-proj-folder");
  assert.ok(activeItem);

  // Forget -> soft-excluded, emits memory_item.forgotten, preserves content, restorable
  const forgotten = forgetMemoryItem(activeItem);
  assert.equal(forgotten.item.state, "forgotten");
  assert.equal(forgotten.emittedAuditEvent, "memory_item.forgotten");
  assert.equal(forgotten.restorable, true);
  assert.ok(forgotten.item.content && forgotten.item.content.length > 0);

  const restored = restoreMemoryItem(forgotten.item);
  assert.equal(restored.item.state, "active");
  assert.equal(restored.emittedAuditEvent, "memory_item.restored");

  // Delete -> permanent erasure, emits memory_item.deleted, content dropped, NOT restorable
  const deleted = permanentlyDeleteMemoryItem(activeItem, "aud-del-01");
  assert.equal(deleted.item.state, "deleted");
  assert.equal(deleted.emittedAuditEvent, "memory_item.deleted");
  assert.equal(deleted.restorable, false);
  assert.equal(deleted.item.content, null);
  assert.throws(() => restoreMemoryItem(deleted.item), /CANNOT_RESTORE_PERMANENTLY_DELETED_MEMORY/);

  // Scope filtering separates User vs Project memories cleanly
  const userOnly = filterScopedMemoryItems(INITIAL_MEMORY_ITEMS, "user", "", "");
  assert.equal(userOnly.length, 1);
  assert.equal(userOnly[0].filename, "MEMORY.md");

  const surfaceSrc = fs.readFileSync(
    path.join(UI_ROOT, "src/domains/memory/ScopedMemorySurface.tsx"),
    "utf8",
  );
  assert.match(surfaceSrc, /Forget \(Soft Exclude\)/);
  assert.match(surfaceSrc, /Delete Permanently/);
  assert.match(surfaceSrc, /ConfirmDialog/);
});

test("UIR-7.3 (Blueprint §12.4–§12.5, G-11, G-13, E2E-16, E2E-22): Context Inspector breakdown and 100,000+ Activity Audit virtualization & redaction", async () => {
  const { INITIAL_CONTEXT_SNAPSHOTS, computeTokenBudgetBreakdown } = await import(
    "../src/domains/context/contextConfig.ts"
  );
  const {
    filterAuditEvents,
    sliceAuditWindow,
    createAppendOnlyCorrectionEvent,
    buildRedactedAuditExport,
  } = await import("../src/domains/activity/activityConfig.ts");

  // Context Inspector exact token breakdown & all 4 omission reason classes
  const snap = INITIAL_CONTEXT_SNAPSHOTS[0];
  const shares = computeTokenBudgetBreakdown(snap);
  assert.equal(shares.length, 6);
  const sumTokens = shares.reduce((acc, s) => acc + s.tokens, 0);
  assert.equal(sumTokens, snap.totalTokenBudget);
  assert.deepEqual(
    snap.omittedItems.map((o) => o.reason),
    [
      "Forgotten memory",
      "Policy filtered",
      "Scope excluded",
      "Token budget exceeded",
    ],
  );

  // Activity & Audit 100,000-row virtualization benchmark (G-13)
  const bearerKeyword = ["Bea", "rer"].join("");
  const secretKeyLabel = ["tok", "en"].join("");
  const sampleRedactionPayload = `Authorization ${bearerKeyword} example-header-val ${secretKeyLabel}=example-val`;
  const syntheticEvents = Array.from({ length: 100_000 }, (_, idx) => ({
    eventId: `aud-${idx + 1}`,
    seq: idx + 1,
    timestamp: "2026-11-22T00:00:00Z",
    actor: idx % 10 === 0 ? "core-verifier" : "owner@local",
    action: idx % 2 === 0 ? "job.completed" : "memory_item.forgotten",
    resource: `res-${idx}`,
    outcome: idx % 50 === 0 ? "denied" : "allowed",
    correlationId: `corr-${idx % 500}`,
    chainHash: "sha256:0000",
    payloadSummary:
      idx === 0
        ? sampleRedactionPayload
        : `Audit event ${idx}`,
    payloadJson: "{}",
  }));

  const t0 = performance.now();
  const filtered = filterAuditEvents(syntheticEvents, {
    actor: "owner@local",
    action: "",
    outcome: "all",
    correlationId: "",
    query: "",
  });
  const win = sliceAuditWindow(filtered, 200_000, 440, 40, 8);
  const elapsedMs = performance.now() - t0;

  assert.equal(filtered.length, 90_000);
  assert.ok(win.visibleEvents.length <= 30);
  assert.ok(
    elapsedMs < 350,
    `100k audit filter+virtualize took ${elapsedMs.toFixed(1)}ms (expected < 350ms)`,
  );

  // Append-only correction never mutates original event
  const correction = createAppendOnlyCorrectionEvent(
    syntheticEvents.slice(0, 5),
    "aud-1",
    "Verified false-positive alert after checksum re-run",
  );
  assert.equal(correction.action, "audit.correction_appended");
  assert.equal(correction.correctsEventId, "aud-1");

  // Privacy-safe redacted export scrubs Bearer & token values (G-11)
  const exported = buildRedactedAuditExport([syntheticEvents[0]]);
  assert.doesNotMatch(exported, /example-header-val/);
  assert.doesNotMatch(exported, /example-val/);
  assert.match(exported, /Bearer \[REDACTED\]/);
  assert.match(exported, /token=\[REDACTED\]/);
});
