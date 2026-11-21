/**
 * Sandboxed Document Previewer Sanitizer & Off-Main-Thread Diff Engine
 * (UIR-5.2 & UIR-5.4, UI-39, Blueprint §22.3, Quality Gate G-12).
 *
 * Pure TypeScript module (zero runtime extensionless imports) so it executes identically
 * inside the browser bundle, Web Workers (`diffs.worker.ts`), and Node 26 unit tests.
 */

export type DocumentPreviewKind = "pdf" | "docx" | "xlsx" | "pptx" | "code";

export interface SanitizationReport {
  readonly sanitizedText: string;
  readonly strippedPatternsCount: number;
  readonly hadExecutableRisk: boolean;
}

export interface WordDiffSegment {
  readonly kind: "unchanged" | "added" | "removed";
  readonly text: string;
}

export interface DiffLineRow {
  readonly kind: "context" | "added" | "removed";
  readonly oldLineNumber: number | null;
  readonly newLineNumber: number | null;
  readonly text: string;
  readonly wordSegments?: readonly WordDiffSegment[];
}

export interface ComputedFileDiff {
  readonly filePath: string;
  readonly additions: number;
  readonly deletions: number;
  readonly lines: readonly DiffLineRow[];
}

const SCRIPT_TAG_RE = /<\s*script\b[^>]*>[\s\S]*?<\s*\/\s*script\s*>/gi;
const DANGEROUS_TAG_RE =
  /<\s*\/?\s*(iframe|object|embed|applet|meta|link|base|form|svg|math)\b[^>]*>/gi;
const EVENT_HANDLER_RE = /\bon[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi;
const DANGEROUS_URI_RE = /\b(?:javascript|vbscript|data\s*:\s*text\/html)\s*:/gi;
const VBA_MACRO_RE = /\b(?:AutoOpen|Auto_Open|Document_Open|Workbook_Open|Shell\s*\(|CreateObject\s*\()\b/gi;

/**
 * Classifies a workspace file path into a sandboxed Office/PDF previewer or code viewer.
 */
export function detectDocumentPreviewKind(filePath: string): DocumentPreviewKind {
  const lower = filePath.trim().toLowerCase();
  if (lower.endsWith(".pdf")) return "pdf";
  if (lower.endsWith(".docx") || lower.endsWith(".doc")) return "docx";
  if (lower.endsWith(".xlsx") || lower.endsWith(".xls") || lower.endsWith(".csv")) {
    return "xlsx";
  }
  if (lower.endsWith(".pptx") || lower.endsWith(".ppt")) return "pptx";
  return "code";
}

/**
 * Enforces G-12 zero-script and zero-macro execution for untrusted document text.
 * Strips script tags, embedded active elements, inline event handlers, dangerous URI
 * schemes, and Office macro triggers before text enters any sandboxed viewer.
 */
export function sanitizeDocumentPayload(raw: string): SanitizationReport {
  let current = raw;
  let strippedCount = 0;

  const applyStrip = (regex: RegExp, replacement: string) => {
    const matches = current.match(regex);
    if (matches && matches.length > 0) {
      strippedCount += matches.length;
      current = current.replace(regex, replacement);
    }
  };

  applyStrip(SCRIPT_TAG_RE, "[script-blocked]");
  applyStrip(DANGEROUS_TAG_RE, "");
  applyStrip(EVENT_HANDLER_RE, "");
  applyStrip(DANGEROUS_URI_RE, "blocked-uri:");
  applyStrip(VBA_MACRO_RE, "[macro-blocked]");

  return {
    sanitizedText: current,
    strippedPatternsCount: strippedCount,
    hadExecutableRisk: strippedCount > 0,
  };
}

/**
 * Verifies that rendered previewer text contains zero executable script or URI payloads
 * (Quality Gate G-12 assertion helper).
 */
export function assertZeroScriptExecution(renderedContent: string): boolean {
  const checks = [
    /<\s*script\b/i,
    /\bon(?:load|error|click|mouseover|focus)\s*=/i,
    /\bjavascript\s*:/i,
    /\bvbscript\s*:/i,
    /<\s*iframe\b/i,
  ];
  return checks.every((pattern) => !pattern.test(renderedContent));
}

/**
 * Neutralizes potentially dangerous spreadsheet formula/DDE injection prefixes
 * (`=`, `+`, `-`, `@`) when displaying or exporting untrusted XLSX cell content.
 */
export function sanitizeSpreadsheetCellDisplay(rawCell: string): {
  readonly displayValue: string;
  readonly formulaBlocked: boolean;
} {
  const trimmed = rawCell.trim();
  const isFormulaOrDde =
    /^[=+\-@]/.test(trimmed) &&
    /(?:HYPERLINK|WEBSERVICE|IMPORTXML|CMD|POWERSHELL|EXEC|DDE|\()/i.test(trimmed);

  const baseSanitized = sanitizeDocumentPayload(rawCell);
  if (isFormulaOrDde) {
    return {
      displayValue: `'${baseSanitized.sanitizedText} [formula-inert]`,
      formulaBlocked: true,
    };
  }
  return {
    displayValue: baseSanitized.sanitizedText,
    formulaBlocked: baseSanitized.hadExecutableRisk,
  };
}

/**
 * Computes word-level diff segments between two adjacent modified lines.
 */
export function computeWordDiffSegments(
  oldLine: string,
  newLine: string,
): {
  readonly oldSegments: readonly WordDiffSegment[];
  readonly newSegments: readonly WordDiffSegment[];
} {
  const oldTokens = oldLine.split(/(\s+)/);
  const newTokens = newLine.split(/(\s+)/);

  let prefix = 0;
  while (
    prefix < oldTokens.length &&
    prefix < newTokens.length &&
    oldTokens[prefix] === newTokens[prefix]
  ) {
    prefix += 1;
  }

  let oldEnd = oldTokens.length - 1;
  let newEnd = newTokens.length - 1;
  while (
    oldEnd >= prefix &&
    newEnd >= prefix &&
    oldTokens[oldEnd] === newTokens[newEnd]
  ) {
    oldEnd -= 1;
    newEnd -= 1;
  }

  const oldSegments: WordDiffSegment[] = [];
  const newSegments: WordDiffSegment[] = [];

  if (prefix > 0) {
    const sharedPrefix = oldTokens.slice(0, prefix).join("");
    oldSegments.push({ kind: "unchanged", text: sharedPrefix });
    newSegments.push({ kind: "unchanged", text: sharedPrefix });
  }

  if (oldEnd >= prefix) {
    oldSegments.push({
      kind: "removed",
      text: oldTokens.slice(prefix, oldEnd + 1).join(""),
    });
  }

  if (newEnd >= prefix) {
    newSegments.push({
      kind: "added",
      text: newTokens.slice(prefix, newEnd + 1).join(""),
    });
  }

  if (oldEnd + 1 < oldTokens.length) {
    const sharedSuffix = oldTokens.slice(oldEnd + 1).join("");
    oldSegments.push({ kind: "unchanged", text: sharedSuffix });
    newSegments.push({ kind: "unchanged", text: sharedSuffix });
  }

  return { oldSegments, newSegments };
}

/**
 * Computes deterministic line-level and inline word-level diffs for Unified & Split
 * review modes (`ReviewDiffTab.tsx` and `diffs.worker.ts`).
 */
export function computeLineAndWordDiff(
  filePath: string,
  oldText: string,
  newText: string,
): ComputedFileDiff {
  const oldLines = oldText.length === 0 ? [] : oldText.split("\n");
  const newLines = newText.length === 0 ? [] : newText.split("\n");

  const m = oldLines.length;
  const n = newLines.length;

  // Bounded LCS DP (fast for files up to several thousand lines; fallback linear scan beyond)
  if (m * n <= 250_000) {
    const dp: Uint32Array[] = Array.from({ length: m + 1 }, () => new Uint32Array(n + 1));
    for (let i = m - 1; i >= 0; i -= 1) {
      for (let j = n - 1; j >= 0; j -= 1) {
        if (oldLines[i] === newLines[j]) {
          dp[i][j] = dp[i + 1][j + 1] + 1;
        } else {
          dp[i][j] = dp[i + 1][j] >= dp[i][j + 1] ? dp[i + 1][j] : dp[i][j + 1];
        }
      }
    }

    const rawRows: DiffLineRow[] = [];
    let i = 0;
    let j = 0;
    let additions = 0;
    let deletions = 0;

    while (i < m && j < n) {
      if (oldLines[i] === newLines[j]) {
        rawRows.push({
          kind: "context",
          oldLineNumber: i + 1,
          newLineNumber: j + 1,
          text: oldLines[i],
        });
        i += 1;
        j += 1;
      } else if (dp[i + 1][j] >= dp[i][j + 1]) {
        rawRows.push({
          kind: "removed",
          oldLineNumber: i + 1,
          newLineNumber: null,
          text: oldLines[i],
        });
        deletions += 1;
        i += 1;
      } else {
        rawRows.push({
          kind: "added",
          oldLineNumber: null,
          newLineNumber: j + 1,
          text: newLines[j],
        });
        additions += 1;
        j += 1;
      }
    }

    while (i < m) {
      rawRows.push({
        kind: "removed",
        oldLineNumber: i + 1,
        newLineNumber: null,
        text: oldLines[i],
      });
      deletions += 1;
      i += 1;
    }

    while (j < n) {
      rawRows.push({
        kind: "added",
        oldLineNumber: null,
        newLineNumber: j + 1,
        text: newLines[j],
      });
      additions += 1;
      j += 1;
    }

    // Enrich adjacent removed + added pairs with word-level segments
    const enriched: DiffLineRow[] = [...rawRows];
    for (let k = 0; k < enriched.length - 1; k += 1) {
      const curr = enriched[k];
      const next = enriched[k + 1];
      if (curr.kind === "removed" && next.kind === "added") {
        const { oldSegments, newSegments } = computeWordDiffSegments(
          curr.text,
          next.text,
        );
        enriched[k] = { ...curr, wordSegments: oldSegments };
        enriched[k + 1] = { ...next, wordSegments: newSegments };
        k += 1;
      }
    }

    return {
      filePath,
      additions,
      deletions,
      lines: enriched,
    };
  }

  // O(max(m, n)) fallback for massive files
  const lines: DiffLineRow[] = [];
  const maxLen = Math.max(m, n);
  let additions = 0;
  let deletions = 0;
  for (let idx = 0; idx < maxLen; idx += 1) {
    const o = oldLines[idx];
    const nw = newLines[idx];
    if (o === nw && o !== undefined) {
      lines.push({
        kind: "context",
        oldLineNumber: idx + 1,
        newLineNumber: idx + 1,
        text: o,
      });
    } else {
      if (o !== undefined) {
        lines.push({
          kind: "removed",
          oldLineNumber: idx + 1,
          newLineNumber: null,
          text: o,
        });
        deletions += 1;
      }
      if (nw !== undefined) {
        lines.push({
          kind: "added",
          oldLineNumber: null,
          newLineNumber: idx + 1,
          text: nw,
        });
        additions += 1;
      }
    }
  }
  return { filePath, additions, deletions, lines };
}
