/**
 * Generated-types drift check (P1.5 §2.3 + §5; PP-4).
 *
 * The UI's TypeScript schema types are DERIVED artifacts: they are
 * regenerated from the versioned contract source and must match the
 * committed snapshot byte-for-byte, or a drift is reported.
 *
 * Honest scope (recorded, not hidden): the contract document in the
 * repository is `contracts/openapi.yaml`. It carried the explicitly
 * NON-CANONICAL `0.1.0-eval` evaluation surface (one indicative
 * `submitJob` operation) until the owner's recorded contract publication
 * act (2026-11-03) switched it to the canonical `0.1.0` published set of
 * the frozen contract families (F1/F3/F5/F6/F7/F8/F10) — this script's
 * own recorded switch condition. The committed snapshot under
 * src/coreclient/generated-eval-surface/ is the drift-check baseline for
 * that published set: it is regenerated from the contract source and must
 * match it byte-for-byte. The family-specific wire shapes the UI
 * components consume remain their hand-typed typed boundaries; this
 * generated snapshot is the contract-derived check surface.
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
