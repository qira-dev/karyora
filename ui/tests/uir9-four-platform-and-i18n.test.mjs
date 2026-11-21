/**
 * Phase 5 Milestone UIR-9 Automated Verification Suite:
 *  - UIR-9.1 (G-10, UI-18): 13-Locale Internationalization, Placeholder Parity & RFC 9457 Error Map
 *  - UIR-9.2 (G-07, WCAG 1.4.4 / 1.4.10): 320 CSS px Reflow & 200% Text Zoom Hardening across 13 Locales
 *  - UIR-9.3 (G-15, UI-03, UI-16, UI-24): 4-Platform Single-SPA Parity (Web, Linux, macOS, Windows) & Window-Close Safety
 */

import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import i18next from "i18next";

const UI_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const LOCALES_ROOT = path.join(UI_ROOT, "src", "i18n", "locales");

const ALL_13_LOCALES = [
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
];

function deepMerge(target, ...sources) {
  const result = { ...target };
  for (const source of sources) {
    for (const [key, value] of Object.entries(source)) {
      const existing = result[key];
      if (
        existing !== null &&
        value !== null &&
        typeof existing === "object" &&
        typeof value === "object" &&
        !Array.isArray(existing) &&
        !Array.isArray(value)
      ) {
        result[key] = deepMerge(existing, value);
      } else {
        result[key] = value;
      }
    }
  }
  return result;
}

function flattenLeaves(obj, prefix = "", out = new Map()) {
  for (const [k, v] of Object.entries(obj)) {
    const p = prefix ? `${prefix}.${k}` : k;
    if (v !== null && typeof v === "object" && !Array.isArray(v)) {
      flattenLeaves(v, p, out);
    } else {
      out.set(p, v);
    }
  }
  return out;
}

function loadLocaleTree(locale) {
  const dir = path.join(LOCALES_ROOT, locale);
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith(".json"))
    .sort();
  let merged = {};
  for (const file of files) {
    merged = deepMerge(merged, JSON.parse(fs.readFileSync(path.join(dir, file), "utf8")));
  }
  return { files, merged, leaves: flattenLeaves(merged) };
}

function extractPlaceholders(str) {
  return [...new Set(String(str).match(/\{\{[^}]+\}\}/g) ?? [])].sort();
}

test("UIR-9.1 (G-10, UI-18): all 13 locked locales have 100% key, placeholder, script, and RFC 9457 error parity", async () => {
  const { RFC9457_REQUIRED_I18N_KEYS, resolveLocalizedRecoveryCopy } = await import(
    "../src/i18n/rfc9457ErrorMap.ts"
  );
  const {
    parseCoreErrorEnvelope,
    toRecoveryPanelDescriptor,
    toUnknownOutcomeS4RecoveryDescriptor,
  } = await import("../src/coreclient/rfc9457.ts");

  const en = loadLocaleTree("en");
  assert.equal(en.files.length, 16, "en source must have 16 namespace files");
  assert.ok(en.leaves.size >= 2050, `expected >= 2050 keys including RFC 9457 errors, got ${en.leaves.size}`);

  const scriptAssertions = {
    bn: /[\u0980-\u09FF]/,
    "zh-CN": /[\u4E00-\u9FFF]/,
    ja: /[\u3040-\u30FF\u4E00-\u9FFF]/,
    ru: /[\u0400-\u04FF]/,
    ko: /[\uAC00-\uD7AF]/,
    vi: /[ăâđêôơưàáảãạèéẻẽẹìíỉĩịòóỏõọùúủũụỳýỷỹỵ]/i,
  };

  const resources = {};

  for (const locale of ALL_13_LOCALES) {
    const loc = loadLocaleTree(locale);
    assert.deepEqual(
      loc.files,
      en.files,
      `locale ${locale} must have the exact same 16 namespace files as en`,
    );
    assert.equal(
      loc.leaves.size,
      en.leaves.size,
      `locale ${locale} key count mismatch (${loc.leaves.size} vs ${en.leaves.size})`,
    );

    let scriptHits = 0;
    for (const [key, enVal] of en.leaves.entries()) {
      assert.ok(loc.leaves.has(key), `locale ${locale} missing key ${key}`);
      const locVal = loc.leaves.get(key);
      assert.equal(typeof locVal, "string", `locale ${locale} key ${key} must be string`);
      assert.ok(locVal.trim().length > 0, `locale ${locale} key ${key} must be non-empty`);
      assert.deepEqual(
        extractPlaceholders(locVal),
        extractPlaceholders(enVal),
        `locale ${locale} placeholder mismatch on key ${key}`,
      );
      if (scriptAssertions[locale] && scriptAssertions[locale].test(locVal)) {
        scriptHits += 1;
      }
    }

    if (scriptAssertions[locale]) {
      const ratio = scriptHits / loc.leaves.size;
      assert.ok(
        ratio >= 0.7,
        `locale ${locale} expected >= 70% native script coverage, got ${(ratio * 100).toFixed(1)}%`,
      );
    }

    for (const reqKey of RFC9457_REQUIRED_I18N_KEYS) {
      assert.ok(loc.leaves.has(reqKey), `locale ${locale} missing RFC 9457 key ${reqKey}`);
    }

    resources[locale] = { translation: loc.merged };
  }

  // Live i18next runtime verification across all 13 locales + RFC 9457 resolution
  const instance = i18next.createInstance();
  await instance.init({
    lng: "en",
    fallbackLng: "en",
    supportedLngs: ALL_13_LOCALES,
    returnEmptyString: false,
    returnNull: false,
    defaultNS: "translation",
    nsSeparator: false,
    resources,
    interpolation: { escapeValue: false },
  });

  // Raw Go/backend prose must be stripped before localization
  const rawEnvelope = parseCoreErrorEnvelope({
    error: {
      code: "CONFIG_CONFLICT",
      retryable: false,
      correlationId: "corr-uir9-test",
      detail: "RAW_GO_PANIC_NEVER_SHOW",
      data: { expectedRevision: 4, detail: "LEAKED_BACKEND_TEXT" },
    },
  });
  assert.ok(rawEnvelope);
  const conflictDesc = toRecoveryPanelDescriptor(rawEnvelope, 409);
  const s4Desc = toUnknownOutcomeS4RecoveryDescriptor("corr-s4-test");

  const resolvedConflictTitles = new Set();
  for (const locale of ALL_13_LOCALES) {
    await instance.changeLanguage(locale);
    const t = (key, opts) => instance.t(key, opts);
    const conflictCopy = resolveLocalizedRecoveryCopy(t, conflictDesc);
    const s4Copy = resolveLocalizedRecoveryCopy(t, s4Desc);

    assert.ok(conflictCopy.title.length > 0 && !conflictCopy.title.startsWith("errors."));
    assert.ok(conflictCopy.explanation.length > 0 && !conflictCopy.explanation.startsWith("errors."));
    assert.ok(conflictCopy.draftSafetyNotice.length > 0);
    assert.match(conflictCopy.errorCodeLabel, /CONFIG_CONFLICT/);
    assert.match(conflictCopy.correlationIdLabel, /corr-uir9-test/);
    assert.doesNotMatch(JSON.stringify(conflictCopy), /RAW_GO_PANIC|LEAKED_BACKEND_TEXT/);

    assert.ok(s4Copy.title.length > 0 && !s4Copy.title.startsWith("errors."));
    assert.match(s4Copy.correlationIdLabel, /corr-s4-test/);
    resolvedConflictTitles.add(conflictCopy.title);
  }
  assert.ok(
    resolvedConflictTitles.size >= 10,
    `expected distinct localized RFC 9457 titles across 13 locales, got ${resolvedConflictTitles.size}`,
  );
});

test("UIR-9.2 (G-07, WCAG 1.4.4 / 1.4.10): 320 CSS px reflow & 200% zoom CSS safeguards cover all surfaces and long locales", () => {
  const reflowCss = fs.readFileSync(
    path.join(UI_ROOT, "src", "styles", "responsive-reflow.css"),
    "utf8",
  );
  const mainSource = fs.readFileSync(path.join(UI_ROOT, "src", "main.tsx"), "utf8");

  assert.match(mainSource, /import\s+["']\.\/styles\/responsive-reflow\.css["']/);
  assert.match(reflowCss, /overflow-x:\s*clip/);
  assert.match(reflowCss, /min-width:\s*0/);
  assert.match(reflowCss, /overflow-wrap:\s*anywhere/);
  assert.match(reflowCss, /flex-wrap:\s*wrap/);
  assert.match(reflowCss, /@media\s*\(max-width:\s*640px\)/);
  assert.match(reflowCss, /@media\s*\(max-width:\s*400px\)/);
  assert.match(reflowCss, /min-height:\s*44px/);
  assert.match(reflowCss, /\.ky-settings-overlay/);
  assert.match(reflowCss, /\.ky-files-split/);
  assert.match(reflowCss, /\.ky-memory-split/);
  assert.match(reflowCss, /\.ky-context-columns/);

  // Stress-check longest labels in Bengali (bn), German (de), and Russian (ru) are bounded and non-empty
  for (const locale of ["bn", "de", "ru"]) {
    const { leaves } = loadLocaleTree(locale);
    const navKeys = [...leaves.entries()].filter(([k]) => k.startsWith("items."));
    assert.ok(navKeys.length >= 10, `${locale} must include nav items`);
    for (const [k, v] of navKeys) {
      assert.ok(
        typeof v === "string" && v.length > 0 && v.length <= 80,
        `${locale} nav label ${k} length (${String(v).length}) out of reflow bounds`,
      );
    }
  }
});

test("UIR-9.3 (G-15, UI-03, UI-16, UI-24): 4-platform runtime parity (Web, Linux, macOS, Windows), shortcut adaptation & window-close Job safety", async () => {
  const {
    PLATFORM_PARITY_PROFILES,
    createPlatformBridge,
    formatPlatformShortcut,
    getPlatformParityProfile,
  } = await import("../src/platform-bridge/platformBridge.ts");

  const targets = ["web", "linux", "macos", "windows"];
  assert.deepEqual(Object.keys(PLATFORM_PARITY_PROFILES).sort(), [...targets].sort());

  // Verify every target platform's capability matrix, shortcut modifier, and UI-24 window-close guard
  for (const target of targets) {
    const bridge = createPlatformBridge(
      target === "web"
        ? { isDesktopShell: false }
        : { isDesktopShell: true, desktopPlatform: target },
    );
    const profile = getPlatformParityProfile(target);

    assert.equal(bridge.runtimeTarget, target);
    assert.equal(bridge.isDesktop, target !== "web");
    assert.equal(bridge.capabilities.osKeyringAvailable, true);
    assert.equal(bridge.capabilities.localPtyTerminal, true);
    assert.deepEqual(
      [...profile.architectures].sort(),
      ["amd64", "arm64"],
      `${target} must declare amd64 + arm64 parity`,
    );

    // Shortcut modifier adaptation (Cmd on macOS, Ctrl on Web/Linux/Windows)
    const adapted = formatPlatformShortcut("Ctrl+K", target);
    assert.equal(adapted, target === "macos" ? "Cmd+K" : "Ctrl+K");

    // UI-24 Window Close & Active Job Lifecycle Safety
    const closeWithActiveJobsTrayOn = bridge.evaluateWindowCloseGuard(2, true);
    const closeWithActiveJobsTrayOff = bridge.evaluateWindowCloseGuard(2, false);
    const closeIdle = bridge.evaluateWindowCloseGuard(0, false);

    if (target === "web") {
      assert.equal(closeWithActiveJobsTrayOn.action, "confirm-active-jobs");
      assert.equal(bridge.capabilities.systemTray, false);
      assert.equal(bridge.capabilities.nativeFolderPicker, false);
    } else {
      assert.equal(closeWithActiveJobsTrayOn.action, "hide-to-tray");
      assert.equal(closeWithActiveJobsTrayOff.action, "confirm-active-jobs");
      assert.equal(bridge.capabilities.systemTray, true);
      assert.equal(bridge.capabilities.nativeFolderPicker, true);
    }
    assert.equal(closeIdle.action, "allow-close");

    // External URL protocol safety
    assert.equal(bridge.validateExternalUrlForOpen("javascript:alert(1)").allowed, false);
    assert.equal(bridge.validateExternalUrlForOpen("https://github.com/qira-dev/karyora").allowed, true);
  }

  // Verify single SPA Go embed contract file exists and uses //go:embed all:dist
  const embedGo = fs.readFileSync(path.join(UI_ROOT, "embed.go"), "utf8");
  assert.match(embedGo, /\/\/go:embed\s+all:dist/);

  // Verify all 12 lazy per-locale bundle loaders exist so initial bundle stays <= 350 KB gz
  for (const loc of ALL_13_LOCALES.filter((l) => l !== "en")) {
    const loaderPath = path.join(UI_ROOT, "src", "i18n", "bundles", `${loc}.ts`);
    assert.ok(fs.existsSync(loaderPath), `Missing lazy bundle loader: ${loaderPath}`);
  }
});
