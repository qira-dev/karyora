import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const srcRoot = join(uiRoot, "src");

test("UIR-1.1 (G-02, G-08, G-10): CoreClient, RFC 9457 machine error mapper, and monotonic SSE cursor-gap refetch manager exist and enforce rules", () => {
  const requiredFiles = [
    "coreclient/types.ts",
    "coreclient/rfc9457.ts",
    "coreclient/sseManager.ts",
    "coreclient/CoreClient.ts",
    "coreclient/index.ts",
  ];
  for (const file of requiredFiles) {
    assert.equal(
      existsSync(join(srcRoot, file)),
      true,
      `Required UIR-1.1 module src/${file} must exist`,
    );
  }

  const rfcSource = readFileSync(join(srcRoot, "coreclient/rfc9457.ts"), "utf8");
  assert.ok(
    rfcSource.includes("parseCoreErrorEnvelope") &&
      rfcSource.includes("toRecoveryPanelDescriptor") &&
      rfcSource.includes("toUnknownOutcomeS4RecoveryDescriptor"),
    "rfc9457.ts must export parseCoreErrorEnvelope, toRecoveryPanelDescriptor, and toUnknownOutcomeS4RecoveryDescriptor",
  );
  // Must strip raw prose and secrets
  assert.ok(
    rfcSource.includes('lower === "detail"') &&
      rfcSource.includes('lower === "message"') &&
      rfcSource.includes('lower.includes("secret")'),
    "rfc9457.ts must strip raw backend prose and secret keys from error envelopes",
  );

  const sseSource = readFileSync(join(srcRoot, "coreclient/sseManager.ts"), "utf8");
  assert.ok(
    sseSource.includes("refetching-gap") &&
      sseSource.includes("onGapRefetchRequired") &&
      sseSource.includes("duplicate-ignored"),
    "sseManager.ts must implement idempotent duplicate protection and cursor-gap REST snapshot refetch",
  );
});

test("UIR-1.2 (UI-03, UI-16, UI-24): 4-platform PlatformBridge supports web, windows, macos, linux, window-close job guard, and URL protocol validation", () => {
  const bridgePath = join(srcRoot, "platform-bridge/platformBridge.ts");
  assert.equal(existsSync(bridgePath), true, "platformBridge.ts must exist");

  const bridgeSource = readFileSync(bridgePath, "utf8");
  for (const fn of [
    "detectRuntimePlatform",
    "getPlatformCapabilities",
    "validateExternalUrlForOpen",
    "evaluateWindowCloseGuard",
    "createPlatformBridge",
  ]) {
    assert.ok(
      bridgeSource.includes(`export function ${fn}`),
      `platformBridge.ts must export ${fn}`,
    );
  }
  assert.ok(
    bridgeSource.includes("confirm-active-jobs") &&
      bridgeSource.includes("hide-to-tray"),
    "platformBridge.ts must enforce UI-24 window-close active-job protection",
  );
  assert.ok(
    bridgeSource.includes('parsed.protocol !== "https:" && parsed.protocol !== "http:"'),
    "platformBridge.ts must block non-HTTP(S) external URL schemes",
  );
});

test("UIR-1.3 (Blueprint §23.1, G-06, G-11): 5 state-lifetime stores enforce connection cache purge, draft preservation, focus return, and zero-secret storage guard", () => {
  const lifetimesPath = join(srcRoot, "workspace/state/stateLifetimes.ts");
  assert.equal(existsSync(lifetimesPath), true, "stateLifetimes.ts must exist");

  const source = readFileSync(lifetimesPath, "utf8");
  for (const cls of [
    "DomainStateCache",
    "WorkspaceSessionStore",
    "DraftAndMutationStore",
    "LocalPreferencesStore",
    "EphemeralUiStore",
  ]) {
    assert.ok(
      source.includes(`export class ${cls}`),
      `stateLifetimes.ts must implement ${cls}`,
    );
  }
  assert.ok(
    source.includes("G-11 Secret Hygiene Violation") &&
      source.includes("assertZeroSecretContent"),
    "stateLifetimes.ts must enforce G-11 zero-secret assertion on client preferences and drafts",
  );
  assert.ok(
    source.includes("workspaceInert") && source.includes("focusReturnElementId"),
    "EphemeralUiStore must track workspaceInert and deterministic focusReturnElementId (G-06)",
  );
});

test("UIR-1.4 (Blueprint §20.4, G-09): SessionPrewarmStore, SubmissionQueueStore, and CasConcurrencyGuard implement generation counters, serial queue auto-drain/pause, and CAS 409/logEpoch guards", () => {
  const prewarmSource = readFileSync(
    join(srcRoot, "workspace/state/sessionPrewarmStore.ts"),
    "utf8",
  );
  assert.ok(
    prewarmSource.includes("STALE_GENERATION") &&
      prewarmSource.includes("SCOPE_MISMATCH"),
    "SessionPrewarmStore must reject stale generation and mismatched workspace scope responses",
  );

  const queueSource = readFileSync(
    join(srcRoot, "workspace/state/submissionQueueStore.ts"),
    "utf8",
  );
  for (const method of [
    "submitOrEnqueue",
    "reorder",
    "editItem",
    "removeItem",
    "promoteToSendNow",
    "onTurnCompletedS2",
    "onTurnPausedOrFailed",
  ]) {
    assert.ok(
      queueSource.includes(method),
      `SubmissionQueueStore must implement ${method}`,
    );
  }

  const casSource = readFileSync(
    join(srcRoot, "workspace/state/casConcurrencyGuard.ts"),
    "utf8",
  );
  assert.ok(
    casSource.includes("conflict-preserved") &&
      casSource.includes("unsavedClientDraft") &&
      casSource.includes("latestServerEntity") &&
      casSource.includes("STALE_LOG_EPOCH"),
    "CasConcurrencyGuard must preserve unsaved drafts side-by-side on 409 Conflict and validate transcript baseLogEpoch",
  );
});

test("UIR-1 Runtime Behavioral Verification: RFC 9457, SSE gap refetch, 4-platform bridge, G-11 secret guard, prewarm generation race, serial queue auto-drain/pause, and CAS conflict preservation", async () => {
  const {
    parseCoreErrorEnvelope,
    toRecoveryPanelDescriptor,
    toUnknownOutcomeS4RecoveryDescriptor,
  } = await import("../src/coreclient/rfc9457.ts");
  const { SseStreamManager } = await import("../src/coreclient/sseManager.ts");
  const {
    detectRuntimePlatform,
    getPlatformCapabilities,
    validateExternalUrlForOpen,
    evaluateWindowCloseGuard,
  } = await import("../src/platform-bridge/platformBridge.ts");
  const {
    DomainStateCache,
    LocalPreferencesStore,
    EphemeralUiStore,
  } = await import("../src/workspace/state/stateLifetimes.ts");
  const { SessionPrewarmStore } = await import(
    "../src/workspace/state/sessionPrewarmStore.ts"
  );
  const { SubmissionQueueStore } = await import(
    "../src/workspace/state/submissionQueueStore.ts"
  );
  const { CasConcurrencyGuard } = await import(
    "../src/workspace/state/casConcurrencyGuard.ts"
  );

  // 1. RFC 9457 machine error parsing & raw prose/secret stripping
  const disallowedField = ["api", "Key"].join("");
  const parsed = parseCoreErrorEnvelope({
    code: "RATE_LIMITED",
    retryable: true,
    correlationId: "corr-123",
    detail: "Raw English backend prose that must be stripped",
    data: { retryAfterMs: 2500, [disallowedField]: "redacted" },
  });
  assert.ok(parsed);
  assert.equal(parsed.code, "RATE_LIMITED");
  assert.equal(parsed.correlationId, "corr-123");
  assert.equal(disallowedField in parsed.data, false);
  const recovery = toRecoveryPanelDescriptor(parsed, 429);
  assert.equal(recovery.stateKind, "rate-limited");
  assert.equal(recovery.titleI18nKey, "errors.rateLimited.title");
  assert.equal(
    toUnknownOutcomeS4RecoveryDescriptor("corr-s4").stateKind,
    "unknown-outcome-s4",
  );

  // 2. Monotonic SSE cursor-gap detection & snapshot refetch
  const sse = new SseStreamManager();
  let refetched = false;
  const r1 = await sse.processIncomingEvent(
    {
      eventId: "e1",
      sequence: 1,
      topic: "session",
      entityId: "s1",
      eventType: "turn.delta",
      payload: {},
      occurredAt: "2026-11-21T00:00:00Z",
    },
    {
      onEvent: () => {},
      onGapRefetchRequired: async () => ({ ok: true }),
    },
  );
  assert.equal(r1.disposition, "applied");

  // Duplicate sequence 1 ignored
  const rDup = await sse.processIncomingEvent(
    {
      eventId: "e1",
      sequence: 1,
      topic: "session",
      entityId: "s1",
      eventType: "turn.delta",
      payload: {},
      occurredAt: "2026-11-21T00:00:00Z",
    },
    {
      onEvent: () => {},
      onGapRefetchRequired: async () => ({ ok: true }),
    },
  );
  assert.equal(rDup.disposition, "duplicate-ignored");

  // Sequence 4 (> 1 + 1) triggers gap refetch
  const rGap = await sse.processIncomingEvent(
    {
      eventId: "e4",
      sequence: 4,
      topic: "session",
      entityId: "s1",
      eventType: "turn.delta",
      payload: {},
      occurredAt: "2026-11-21T00:00:01Z",
    },
    {
      onEvent: () => {},
      onGapRefetchRequired: async () => {
        refetched = true;
        return { snapshotSeq: 4 };
      },
    },
  );
  assert.equal(rGap.disposition, "gap-refetched");
  assert.equal(refetched, true);

  // 3. 4-Platform detection, URL safety & UI-24 window-close guard
  assert.equal(detectRuntimePlatform({ isDesktopShell: false }), "web");
  assert.equal(
    detectRuntimePlatform({ isDesktopShell: true, userAgent: "Linux x86_64" }),
    "linux",
  );
  assert.equal(getPlatformCapabilities("linux").computerUse, false);
  assert.equal(validateExternalUrlForOpen("javascript:alert(1)").allowed, false);
  assert.equal(
    validateExternalUrlForOpen("https://github.com/qira-dev/karyora").allowed,
    true,
  );
  assert.equal(
    evaluateWindowCloseGuard("linux", 2, false).action,
    "confirm-active-jobs",
  );

  // 4. State lifetimes & G-11 secret hygiene
  const cache = new DomainStateCache("local-core");
  cache.set("jobs", [1, 2], "rev-1");
  cache.switchConnection("remote-core");
  assert.equal(cache.get("jobs"), undefined);

  const prefs = new LocalPreferencesStore();
  assert.throws(() => {
    prefs.updatePreferences({ [disallowedField]: "redacted" });
  }, /G-11 Secret Hygiene Violation/);

  const overlays = new EphemeralUiStore();
  const opened = overlays.openSettings("model-settings", "sidebar-settings-btn");
  assert.equal(opened.workspaceInert, true);
  const closed = overlays.closeSettings();
  assert.equal(closed.state.workspaceInert, false);
  assert.equal(closed.restoreFocusTo, "sidebar-settings-btn");

  // 5. Draft session prewarm generation race guard
  const prewarm = new SessionPrewarmStore();
  const ticket1 = prewarm.beginPrewarm({
    workspacePath: "/proj/a",
    paneId: "main",
    transportConnectionId: "local",
  });
  const ticket2 = prewarm.beginPrewarm({
    workspacePath: "/proj/b",
    paneId: "main",
    transportConnectionId: "local",
  });
  assert.equal(
    prewarm.commitPrewarmResult(ticket1, "sess-old").committed,
    false,
  );
  assert.equal(
    prewarm.commitPrewarmResult(ticket2, "sess-new").committed,
    true,
  );

  // 6. Serial submission queue auto-drain & pause
  const queue = new SubmissionQueueStore();
  const first = queue.submitOrEnqueue({
    queueItemId: "q1",
    sessionId: "s1",
    promptText: "First",
    chips: [],
    mode: "default",
    modelId: "m1",
    thoughtLevel: "high",
    idempotencyKey: "k1",
    enqueuedAtMs: 1,
  });
  assert.equal(first.dispatchedImmediately, true);

  const second = queue.submitOrEnqueue({
    queueItemId: "q2",
    sessionId: "s1",
    promptText: "Second",
    chips: [],
    mode: "default",
    modelId: "m1",
    thoughtLevel: "high",
    idempotencyKey: "k2",
    enqueuedAtMs: 2,
  });
  assert.equal(second.dispatchedImmediately, false);
  assert.equal(second.snapshot.queuedItems.length, 1);

  const drained = queue.onTurnCompletedS2("s1");
  assert.equal(drained.nextDispatchedItem?.queueItemId, "q2");

  // 7. CAS 409 Conflict draft preservation & logEpoch guard
  const cas = new CasConcurrencyGuard();
  const conflict = cas.evaluateCasMutation(
    "rev-1",
    { name: "User Draft" },
    { entityId: "wf-1", revision: "rev-2", data: { name: "Server Newer" } },
    "rev-3",
  );
  assert.equal(conflict.status, "conflict-preserved");

  const staleBranch = cas.validateTranscriptBranch(
    {
      sessionId: "s1",
      action: "fork-from-here",
      targetTurnId: "t1",
      baseLogEpoch: 3,
    },
    4,
    new Set(["t1"]),
  );
  assert.equal(staleBranch.valid, false);
});
