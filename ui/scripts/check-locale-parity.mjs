/**
 * Locale key parity check (G-10; P1.9 §6.5; P2.4 §4.3).
 *
 * Mechanical rule: every locale directory under src/i18n/locales must
 * produce exactly the same MERGED key set as the English source (the
 * namespace files merge into one translation tree at load time —
 * src/i18n/index.ts — so parity is compared on the merged tree).
 * The 12-locale set is LOCKED (UI-18); only `en` exists today because no
 * translation content exists yet — the check is written so that any
 * locale file added later must match en exactly or CI fails.
 *
 * Run: node scripts/check-locale-parity.mjs (wired into CI's ui job).
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(fileURLToPath(new URL(".", import.meta.url)), "..", "src", "i18n", "locales");
const SOURCE_LOCALE = "en";

/** Deep-merge resource objects (same semantics as src/i18n/index.ts). */
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

/** Recursively collect dotted key paths of a JSON object. */
function keyPaths(obj, prefix = "") {
  const keys = [];
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      keys.push(...keyPaths(value, path));
    } else {
      keys.push(path);
    }
  }
  return keys.sort();
}

function mergedKeySet(localeDir) {
  const files = readdirSync(localeDir).filter((f) => f.endsWith(".json"));
  if (files.length === 0) {
    throw new Error(`${localeDir}: no JSON resource files`);
  }
  let merged = {};
  const perFile = [];
  for (const file of files) {
    const raw = readFileSync(join(localeDir, file), "utf8");
    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch (error) {
      throw new Error(`${join(localeDir, file)}: invalid JSON (${error.message})`);
    }
    merged = deepMerge(merged, parsed);
    perFile.push(file);
  }
  return { keys: keyPaths(merged), files: perFile };
}

const localeDirs = readdirSync(root).filter((name) =>
  statSync(join(root, name)).isDirectory(),
);

if (!localeDirs.includes(SOURCE_LOCALE)) {
  console.error(`Locale parity check failed: the source locale directory ${SOURCE_LOCALE}/ is missing.`);
  process.exit(1);
}

const source = mergedKeySet(join(root, SOURCE_LOCALE));
const sourceKeySet = new Set(source.keys);

let failures = 0;

for (const locale of localeDirs) {
  if (locale === SOURCE_LOCALE) continue;
  let actual;
  try {
    actual = mergedKeySet(join(root, locale));
  } catch (error) {
    console.error(`Locale parity check failed: ${error.message}`);
    failures += 1;
    continue;
  }
  for (const key of source.keys.filter((k) => !actual.keys.includes(k))) {
    console.error(`Locale parity check failed: ${locale} is missing key "${key}".`);
    failures += 1;
  }
  for (const key of actual.keys.filter((k) => !sourceKeySet.has(k))) {
    console.error(`Locale parity check failed: ${locale} has extra key "${key}".`);
    failures += 1;
  }
}

if (failures > 0) {
  console.error(`Locale parity check failed with ${failures} problem(s).`);
  process.exit(1);
}

console.log(
  `Locale parity check passed: ${localeDirs.length} locale(s), ${source.files.length} namespace file(s) merged, ` +
    `${source.keys.length} keys, all key sets equal to the ${SOURCE_LOCALE} source.`,
);
