/**
 * Generated-types drift check (P1.5 §2.3 + §5; PP-4).
 *
 * The UI's TypeScript schema types are DERIVED artifacts: they are
 * regenerated from the versioned contract source and must match the
 * committed snapshot byte-for-byte, or a drift is reported.
 *
 * Honest scope (recorded, not hidden): the only contract document in the
 * repository today is `contracts/openapi.yaml`, the explicitly
 * NON-CANONICAL `0.1.0-eval` evaluation surface (one indicative
 * `submitJob` operation). The committed snapshot under
 * src/coreclient/generated-eval-surface/ is the drift-check baseline for
 * THIS chain proof only; it is consumed by no component, and no exact
 * wire name is treated as canonical (P2.1 O-1 keeps every wire name Open
 * with the Core/API owner). When a canonical contract surface is
 * published, this script's target switches to it.
 *
 * Run: node scripts/check-eval-surface-drift.mjs (wired into CI's ui job).
 */
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..");
const contract = join(uiRoot, "..", "contracts", "openapi.yaml");
const committedSnapshot = join(
  uiRoot,
  "src",
  "coreclient",
  "generated-eval-surface",
  "eval-surface.d.ts",
);

if (!existsSync(contract)) {
  console.error(`Drift check failed: the contract document is missing at ${contract}.`);
  process.exit(1);
}
if (!existsSync(committedSnapshot)) {
  console.error(
    `Drift check failed: the committed generated snapshot is missing at ${committedSnapshot}. ` +
      `Run "npm run types:generate" and commit the result.`,
  );
  process.exit(1);
}

const tempDir = mkdtempSync(join(tmpdir(), "karyora-ui-drift-"));
const generated = join(tempDir, "eval-surface.d.ts");

try {
  execFileSync(
    "node",
    [
      join(uiRoot, "node_modules", "openapi-typescript", "bin", "cli.js"),
      contract,
      "--output",
      generated,
    ],
    { stdio: "inherit", cwd: uiRoot },
  );

  const committed = readFileSync(committedSnapshot, "utf8");
  const fresh = readFileSync(generated, "utf8");

  if (committed !== fresh) {
    console.error(
      "Drift check failed: the committed generated snapshot differs from the contract source. " +
        'Run "npm run types:generate" and review the diff before committing.',
    );
    process.exit(1);
  }

  console.log(
    "Generated-types drift check passed: the committed snapshot matches the contract source exactly.",
  );
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}
