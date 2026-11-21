/**
 * 1,147 Material SVG Icon Integrity Verification Script (UIR-0.4, G-04).
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const iconsDir = join(uiRoot, "src", "assets", "material-icons");
const manifestPath = join(
  uiRoot,
  "src",
  "components",
  "icons",
  "materialIconManifest.ts",
);

const EXPECTED_COUNT = 1147;

const svgFiles = readdirSync(iconsDir)
  .filter((f) => f.endsWith(".svg"))
  .sort();

if (svgFiles.length !== EXPECTED_COUNT) {
  console.error(
    `Material icon count mismatch: expected ${EXPECTED_COUNT}, found ${svgFiles.length}`,
  );
  process.exit(1);
}

for (const file of svgFiles) {
  const fullPath = join(iconsDir, file);
  const size = statSync(fullPath).size;
  if (size === 0) {
    console.error(`Empty SVG file detected: ${fullPath}`);
    process.exit(1);
  }
  const content = readFileSync(fullPath, "utf8");
  if (!content.includes("<svg")) {
    console.error(`Invalid SVG content (missing <svg tag): ${fullPath}`);
    process.exit(1);
  }
}

const manifestText = readFileSync(manifestPath, "utf8");
for (const file of svgFiles) {
  const iconId = file.slice(0, -4);
  if (!manifestText.includes(`"${iconId}"`)) {
    console.error(`Manifest missing icon ID "${iconId}" from ${file}`);
    process.exit(1);
  }
}

console.log(
  `Material SVG icon verification passed: ${svgFiles.length}/${EXPECTED_COUNT} valid SVG icons verified against materialIconManifest.ts.`,
);
