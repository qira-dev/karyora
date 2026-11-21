import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const srcRoot = join(uiRoot, "src");

test("UIR-0.1 & UIR-0.2: superseded legacy prototype directories are removed and v3.0 clean-slate folders exist", () => {
  const removedLegacyDirs = ["app", "pages", "zui"];
  for (const dir of removedLegacyDirs) {
    assert.equal(
      existsSync(join(srcRoot, dir)),
      false,
      `Legacy directory src/${dir} must not exist in v3.0 clean-slate UI`,
    );
  }

  const requiredV3Dirs = [
    "assets/material-icons",
    "types",
    "styles",
    "coreclient",
    "platform-bridge",
    "i18n/locales",
    "components/primitives",
    "components/icons",
    "components/feedback",
    "shell",
    "workspace/composer",
    "workspace/timeline",
    "workspace/state",
    "side-pane",
    "previewers",
    "terminal",
    "automations",
    "marketplace",
    "domains/jobs",
    "domains/knowledge",
    "domains/memory",
    "domains/context",
    "domains/activity",
    "settings/basics",
    "settings/capabilities",
    "settings/data-governance",
  ];

  for (const relDir of requiredV3Dirs) {
    assert.equal(
      existsSync(join(srcRoot, relDir)),
      true,
      `Required v3.0 directory src/${relDir} must exist`,
    );
  }
});

test("UIR-0.3 & UIR-0.4 (G-04): all 1,147 Material SVG icons exist, are valid SVGs, and match materialIconManifest.ts", () => {
  const iconsDir = join(srcRoot, "assets", "material-icons");
  const svgFiles = readdirSync(iconsDir)
    .filter((f) => f.endsWith(".svg"))
    .sort();

  assert.equal(svgFiles.length, 1147, "Must contain all 1,147 Material SVG icons");

  const manifestSource = readFileSync(
    join(srcRoot, "components", "icons", "materialIconManifest.ts"),
    "utf8",
  );

  for (const file of svgFiles) {
    const content = readFileSync(join(iconsDir, file), "utf8");
    assert.ok(content.includes("<svg"), `${file} must contain a valid <svg> element`);
    const iconId = file.slice(0, -4);
    assert.ok(
      manifestSource.includes(`"${iconId}"`),
      `materialIconManifest.ts must register icon ID "${iconId}"`,
    );
  }
});

test("UIR-0.3: decisions.ts locks 4 platforms, 13 locales, 21 settings sections, 40 UI decisions, and BYOK presets without Perplexity", () => {
  const decisionsSource = readFileSync(
    join(srcRoot, "types", "decisions.ts"),
    "utf8",
  );

  for (const platform of ["web", "windows", "macos", "linux"]) {
    assert.ok(
      decisionsSource.includes(`"${platform}"`),
      `decisions.ts must include target platform "${platform}"`,
    );
  }

  for (const locale of [
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
  ]) {
    assert.ok(
      decisionsSource.includes(`"${locale}"`),
      `decisions.ts must lock locale "${locale}"`,
    );
  }

  assert.equal(
    decisionsSource.toLowerCase().includes('"perplexity"'),
    false,
    "UI-11: Perplexity must not be included in Locked BYOK Provider presets",
  );

  for (let i = 1; i <= 40; i++) {
    const id = `UI-${String(i).padStart(2, "0")}`;
    assert.ok(
      decisionsSource.includes(`"${id}"`),
      `decisions.ts must lock decision ${id}`,
    );
  }
});
