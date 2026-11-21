/**
 * 13-Locale Key, Placeholder & RFC 9457 Parity Check (UIR-9.1, G-10, UI-18).
 *
 * Mechanical rules:
 * 1. All 13 locked locales (`en`, `bn`, `zh-CN`, `es`, `pt-BR`, `ja`, `ru`, `ko`,
 *    `de`, `fr`, `id`, `tr`, `vi`) MUST exist under `src/i18n/locales/`.
 * 2. Every locale directory MUST produce the EXACT same merged key set (16 namespace
 *    files) as the English (`en`) source.
 * 3. Every string value in every locale MUST be non-empty and preserve the EXACT
 *    set of `{{placeholder}}` interpolation tokens present in `en`.
 * 4. All 39 RFC 9457 machine-error i18n keys (`errors.*`) MUST exist in all 13 locales.
 *
 * Run: node scripts/check-locale-parity.mjs (wired into CI's `ui` job).
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(fileURLToPath(new URL(".", import.meta.url)), "..", "src", "i18n", "locales");
const SOURCE_LOCALE = "en";
const REQUIRED_LOCALES = [
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

const REQUIRED_RFC9457_KEYS = [
  "errors.common.draftPreservedSafely",
  "errors.common.correlationIdLabel",
  "errors.common.errorCodeLabel",
  "errors.actions.retry",
  "errors.actions.reconcileS4",
  "errors.actions.resolveConflict",
  "errors.actions.reauthenticate",
  "errors.actions.switchCoreConnection",
  "errors.actions.openModelSettings",
  "errors.actions.dismiss",
  "errors.validationFailed.title",
  "errors.validationFailed.explanation",
  "errors.configConflict.title",
  "errors.configConflict.explanation",
  "errors.authzDenied.title",
  "errors.authzDenied.explanation",
  "errors.notFound.title",
  "errors.notFound.explanation",
  "errors.conflictState.title",
  "errors.conflictState.explanation",
  "errors.conflictIdempotencyKey.title",
  "errors.conflictIdempotencyKey.explanation",
  "errors.singleInstanceConflict.title",
  "errors.singleInstanceConflict.explanation",
  "errors.storageUnavailable.title",
  "errors.storageUnavailable.explanation",
  "errors.migrationFailed.title",
  "errors.migrationFailed.explanation",
  "errors.stateCorrupt.title",
  "errors.stateCorrupt.explanation",
  "errors.recoveryRequired.title",
  "errors.recoveryRequired.explanation",
  "errors.rateLimited.title",
  "errors.rateLimited.explanation",
  "errors.internal.title",
  "errors.internal.explanation",
  "errors.unknownCode.title",
  "errors.unknownCode.explanation",
  "errors.unknownOutcomeS4.title",
  "errors.unknownOutcomeS4.explanation",
];

/** Deep-merge resource objects (same semantics as src/i18n/mergeBundle.ts). */
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

/** Collect flat dotted leaf map `{ "a.b": "value" }`. */
function flattenLeaves(obj, prefix = "", out = new Map()) {
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      flattenLeaves(value, path, out);
    } else {
      out.set(path, value);
    }
  }
  return out;
}

/** Extract sorted unique `{{placeholder}}` tokens from a string. */
function extractPlaceholders(value) {
  if (typeof value !== "string") return [];
  const matches = value.match(/\{\{[^}]+\}\}/g) ?? [];
  return [...new Set(matches)].sort();
}

function loadLocaleMerged(localeDir) {
  const files = readdirSync(localeDir)
    .filter((f) => f.endsWith(".json"))
    .sort();
  if (files.length === 0) {
    throw new Error(`${localeDir}: no JSON resource files`);
  }
  let merged = {};
  for (const file of files) {
    const raw = readFileSync(join(localeDir, file), "utf8");
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (error) {
      throw new Error(`${join(localeDir, file)}: invalid JSON (${error.message})`);
    }
    merged = deepMerge(merged, parsed);
  }
  const leaves = flattenLeaves(merged);
  return {
    files,
    leaves,
    keys: [...leaves.keys()].sort(),
  };
}

const localeDirs = readdirSync(root)
  .filter((name) => statSync(join(root, name)).isDirectory())
  .sort();

let failures = 0;

for (const reqLocale of REQUIRED_LOCALES) {
  if (!localeDirs.includes(reqLocale)) {
    console.error(`Locale parity check failed: required locale directory "${reqLocale}/" is missing.`);
    failures += 1;
  }
}

if (!localeDirs.includes(SOURCE_LOCALE)) {
  console.error(`Locale parity check failed: source locale "${SOURCE_LOCALE}/" is missing.`);
  process.exit(1);
}

const source = loadLocaleMerged(join(root, SOURCE_LOCALE));
const sourceKeySet = new Set(source.keys);

// Verify source itself includes all required RFC 9457 keys and non-empty strings
for (const rfcKey of REQUIRED_RFC9457_KEYS) {
  if (!sourceKeySet.has(rfcKey)) {
    console.error(`Locale parity check failed: ${SOURCE_LOCALE} is missing required RFC 9457 key "${rfcKey}".`);
    failures += 1;
  }
}

for (const [key, val] of source.leaves.entries()) {
  if (typeof val === "string" && val.trim().length === 0) {
    console.error(`Locale parity check failed: ${SOURCE_LOCALE} has empty string at "${key}".`);
    failures += 1;
  }
}

for (const locale of localeDirs) {
  if (locale === SOURCE_LOCALE) continue;
  let actual;
  try {
    actual = loadLocaleMerged(join(root, locale));
  } catch (error) {
    console.error(`Locale parity check failed: ${error.message}`);
    failures += 1;
    continue;
  }

  for (const key of source.keys) {
    if (!actual.leaves.has(key)) {
      console.error(`Locale parity check failed: ${locale} is missing key "${key}".`);
      failures += 1;
      continue;
    }
    const srcVal = source.leaves.get(key);
    const actVal = actual.leaves.get(key);

    if (typeof srcVal === "string") {
      if (typeof actVal !== "string" || actVal.trim().length === 0) {
        console.error(`Locale parity check failed: ${locale} has empty/non-string value at "${key}".`);
        failures += 1;
        continue;
      }
      const expectedVars = extractPlaceholders(srcVal);
      const actualVars = extractPlaceholders(actVal);
      if (expectedVars.join(",") !== actualVars.join(",")) {
        console.error(
          `Locale parity check failed: ${locale} placeholder mismatch at "${key}" (expected [${expectedVars.join(", ")}], got [${actualVars.join(", ")}]).`,
        );
        failures += 1;
      }
    }
  }

  for (const key of actual.keys) {
    if (!sourceKeySet.has(key)) {
      console.error(`Locale parity check failed: ${locale} has extra key "${key}".`);
      failures += 1;
    }
  }
}

if (failures > 0) {
  console.error(`Locale parity check failed with ${failures} problem(s).`);
  process.exit(1);
}

console.log(
  `Locale parity check passed: ${localeDirs.length} locale(s) (${localeDirs.join(", ")}), ` +
    `${source.files.length} namespace file(s) merged, ${source.keys.length} keys, ` +
    `100% key + placeholder + RFC 9457 parity with ${SOURCE_LOCALE}.`,
);
