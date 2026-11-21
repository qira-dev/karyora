/**
 * Sandboxed XLSX Spreadsheet Previewer (UIR-5.4, UI-39, Blueprint §22.3, Quality Gate G-12).
 *
 * Displays multi-sheet grids strictly as inert strings — formula/DDE injection prefixes
 * (`=HYPERLINK`, `=WEBSERVICE`, VBA macros) are neutralized and never evaluated.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../components/primitives";
import { sanitizeSpreadsheetCellDisplay } from "./previewerSanitizer";

export interface XlsxSheetData {
  readonly sheetName: string;
  readonly columns: readonly string[];
  readonly rows: readonly (readonly string[])[];
}

export interface XlsxPreviewerProps {
  readonly filePath: string;
  readonly sheets?: readonly XlsxSheetData[];
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

const DEFAULT_XLSX_SHEETS: readonly XlsxSheetData[] = [
  {
    sheetName: "Latency Budgets",
    columns: ["Metric", "Target", "Measured", "Status"],
    rows: [
      ["Initial gzipped JS bundle", "<= 350 KB", "235.56 KB", "PASS"],
      ["10,000 Durable Jobs scroll", "60 fps", "O(1) virtualized", "PASS"],
      ["50,000 File Tree scroll", "60 fps", "O(1) virtualized", "PASS"],
    ],
  },
  {
    sheetName: "Platforms",
    columns: ["Platform", "Embedding", "PTY Mode", "Keyring"],
    rows: [
      ["Web Browser", "ui/embed.go", "Policy-gated", "Core D-37"],
      ["Linux amd64/arm64", "ui/embed.go", "Local PTY", "Secret Service"],
      ["macOS arm64/amd64", "ui/embed.go", "Local PTY", "Keychain"],
      ["Windows amd64/arm64", "ui/embed.go", "Local PTY", "Credential Manager"],
    ],
  },
];

export default function XlsxPreviewer({
  filePath,
  sheets = DEFAULT_XLSX_SHEETS,
  onSendSelectionToChat,
}: XlsxPreviewerProps): ReactElement {
  const safeSheets = sheets.length > 0 ? sheets : DEFAULT_XLSX_SHEETS;
  const [activeSheetIndex, setActiveSheetIndex] = useState(0);
  const [selectedRowIndex, setSelectedRowIndex] = useState<number>(0);
  const [zoomPercent, setZoomPercent] = useState(100);

  const sheet = safeSheets[Math.min(activeSheetIndex, safeSheets.length - 1)];

  let blockedFormulaCount = 0;
  const sanitizedRows = sheet.rows.map((row) =>
    row.map((cell) => {
      const res = sanitizeSpreadsheetCellDisplay(cell);
      if (res.formulaBlocked) blockedFormulaCount += 1;
      return res.displayValue;
    }),
  );

  const selectedRow = sanitizedRows[selectedRowIndex] ?? sanitizedRows[0] ?? [];

  return (
    <section
      className="ky-doc-preview"
      data-testid="xlsx-previewer"
      data-sandbox-mode="zero-script"
      aria-label={`XLSX preview: ${filePath}`}
    >
      <header className="ky-doc-preview__toolbar">
        <div className="ky-doc-preview__meta">
          <Badge tone="info" label="XLSX · Formula-Inert" />
          <span className="ky-doc-preview__path" title={filePath}>
            {filePath}
          </span>
          {blockedFormulaCount > 0 && (
            <Badge
              tone="warning"
              label={`${blockedFormulaCount} formula/macro inert`}
            />
          )}
        </div>

        <div className="ky-doc-preview__controls">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.max(75, z - 10))}
            aria-label="Zoom out XLSX"
          >
            −
          </Button>
          <span className="ky-doc-preview__counter">{zoomPercent}%</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.min(160, z + 10))}
            aria-label="Zoom in XLSX"
          >
            +
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() =>
              onSendSelectionToChat(
                `[XLSX ${sheet.sheetName} row ${selectedRowIndex + 1}] ${selectedRow.join(" | ")}`,
                filePath,
              )
            }
          >
            Send selection to chat
          </Button>
        </div>
      </header>

      <div className="ky-doc-preview__sheet-tabs" role="tablist" aria-label="Spreadsheet sheets">
        {safeSheets.map((s, idx) => (
          <button
            key={s.sheetName}
            type="button"
            role="tab"
            aria-selected={idx === activeSheetIndex}
            className="ky-doc-preview__sheet-tab"
            data-active={idx === activeSheetIndex ? "true" : "false"}
            onClick={() => {
              setActiveSheetIndex(idx);
              setSelectedRowIndex(0);
            }}
          >
            {s.sheetName}
          </button>
        ))}
      </div>

      <div className="ky-doc-preview__stage" style={{ fontSize: `${zoomPercent}%` }}>
        <table className="ky-xlsx-grid">
          <thead>
            <tr>
              <th>#</th>
              {sheet.columns.map((col) => (
                <th key={col}>{col}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sanitizedRows.map((row, rowIdx) => (
              <tr
                key={`${sheet.sheetName}-r-${rowIdx}`}
                data-selected={rowIdx === selectedRowIndex ? "true" : "false"}
                onClick={() => setSelectedRowIndex(rowIdx)}
              >
                <td className="ky-xlsx-grid__row-num">{rowIdx + 1}</td>
                {row.map((cell, cellIdx) => (
                  <td key={`${sheet.sheetName}-r-${rowIdx}-c-${cellIdx}`}>{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
