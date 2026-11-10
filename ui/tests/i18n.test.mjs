import test from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import i18next from "i18next";

const localesRoot = join(
  fileURLToPath(new URL(".", import.meta.url)),
  "..",
  "src",
  "i18n",
  "locales",
);

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

function loadLocale(locale) {
  const dir = join(localesRoot, locale);
  const files = readdirSync(dir).filter((f) => f.endsWith(".json"));
  let merged = {};
  for (const file of files) {
    const raw = readFileSync(join(dir, file), "utf8");
    merged = deepMerge(merged, JSON.parse(raw));
  }
  return { files, resources: merged };
}

test("i18n: en and bn locale files exist with all 16 namespaces", () => {
  const en = loadLocale("en");
  const bn = loadLocale("bn");

  assert.equal(en.files.length, 16, "en should contain 16 namespace files");
  assert.equal(bn.files.length, 16, "bn should contain 16 namespace files");
  assert.deepEqual(en.files.sort(), bn.files.sort(), "both locales must have matching file lists");
});

test("i18n: bn strings contain valid Bengali characters and non-empty values", () => {
  const bn = loadLocale("bn");
  const bengaliRegex = /[\u0980-\u09FF]/;

  let totalKeys = 0;
  let bengaliKeys = 0;

  function traverse(obj) {
    for (const [key, val] of Object.entries(obj)) {
      if (val !== null && typeof val === "object" && !Array.isArray(val)) {
        traverse(val);
      } else if (typeof val === "string") {
        totalKeys++;
        assert.ok(val.length > 0, `Key ${key} in bn must not be empty`);
        if (bengaliRegex.test(val)) {
          bengaliKeys++;
        }
      }
    }
  }

  traverse(bn.resources);
  assert.ok(totalKeys >= 2000, `Expected at least 2000 translation keys, got ${totalKeys}`);
  assert.ok(
    bengaliKeys / totalKeys > 0.85,
    `At least 85% of values should contain Bengali script (got ${bengaliKeys}/${totalKeys})`,
  );
});

test("i18n: i18next resolves both English and Bengali translations correctly", async () => {
  const en = loadLocale("en");
  const bn = loadLocale("bn");

  const instance = i18next.createInstance();
  await instance.init({
    lng: "en",
    fallbackLng: "en",
    returnEmptyString: false,
    defaultNS: "translation",
    nsSeparator: false,
    resources: {
      en: { translation: en.resources },
      bn: { translation: bn.resources },
    },
    interpolation: { escapeValue: false },
  });

  // Verify English
  assert.equal(instance.t("appName"), "Karyora");
  assert.equal(instance.t("items.chat"), "Chat");
  assert.equal(instance.t("items.jobs"), "Jobs");
  assert.equal(instance.t("topbar.search"), "Search");
  assert.equal(instance.t("chat.title"), "Chat");

  // Switch to Bengali
  await instance.changeLanguage("bn");
  assert.equal(instance.t("appName"), "ক্যারিওরা");
  assert.equal(instance.t("items.chat"), "চ্যাট");
  assert.equal(instance.t("items.jobs"), "জবস");
  assert.equal(instance.t("topbar.search"), "অনুসন্ধান");
  assert.equal(instance.t("chat.title"), "চ্যাট");
});
