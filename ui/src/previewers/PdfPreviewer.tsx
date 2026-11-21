/**
 * Sandboxed PDF Document Previewer (UIR-5.4, UI-39, Blueprint §22.3, Quality Gate G-12).
 *
 * Renders extracted PDF pages purely as sanitized text/layout blocks — zero PDF JavaScript
 * actions, zero embedded launch URIs, zero script execution (`G-12`).
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";
import { sanitizeDocumentPayload } from "./previewerSanitizer";

export interface PdfPreviewPage {
  readonly pageNumber: number;
  readonly heading: string;
  readonly body: string;
}

export interface PdfPreviewerProps {
  readonly filePath: string;
  readonly pages?: readonly PdfPreviewPage[];
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

const DEFAULT_PDF_PAGES: readonly PdfPreviewPage[] = [
  {
    pageNumber: 1,
    heading: "Karyora Security & Architecture Brief (Page 1)",
    body: "All provider BYOK credentials remain write-only in the OS keyring (D-37). Loopback API requests require strict Origin, Host, and CSRF header checks.",
  },
  {
    pageNumber: 2,
    heading: "Sandboxed Document Preview Boundary (Page 2)",
    body: "PDF JavaScript actions, embedded form submissions, and external launch links are stripped prior to render (Quality Gate G-12).",
  },
  {
    pageNumber: 3,
    heading: "Durable AIJobRelay & Audit Provenance (Page 3)",
    body: "Every durable Job step writes an immutable SHA-256 receipt linked to its ContextSnapshot and approval audit trail.",
  },
];

export default function PdfPreviewer({
  filePath,
  pages = DEFAULT_PDF_PAGES,
  onSendSelectionToChat,
}: PdfPreviewerProps): ReactElement {
  const [pageIndex, setPageIndex] = useState(0);
  const [zoomPercent, setZoomPercent] = useState(100);

  const safePages = pages.length > 0 ? pages : DEFAULT_PDF_PAGES;
  const clampedIndex = Math.min(Math.max(0, pageIndex), safePages.length - 1);
  const currentPage = safePages[clampedIndex];

  const headingReport = sanitizeDocumentPayload(currentPage.heading);
  const bodyReport = sanitizeDocumentPayload(currentPage.body);
  const blockedAny = headingReport.hadExecutableRisk || bodyReport.hadExecutableRisk;

  return (
    <section
      className="ky-doc-preview"
      data-testid="pdf-previewer"
      data-sandbox-mode="zero-script"
      aria-label={`PDF preview: ${filePath}`}
    >
      <header className="ky-doc-preview__toolbar">
        <div className="ky-doc-preview__meta">
          <Badge tone="info" label="PDF · Sandboxed" />
          <span className="ky-doc-preview__path" title={filePath}>
            {filePath}
          </span>
          {blockedAny && <Badge tone="warning" label="Active script stripped" />}
        </div>

        <div className="ky-doc-preview__controls">
          <Button
            size="sm"
            variant="ghost"
            disabled={clampedIndex === 0}
            onClick={() => setPageIndex((p) => Math.max(0, p - 1))}
            aria-label="Previous PDF page"
          >
            <ChromeIcon name="chevron-left" size={14} />
          </Button>
          <span className="ky-doc-preview__counter" data-testid="pdf-page-indicator">
            Page {clampedIndex + 1} / {safePages.length}
          </span>
          <Button
            size="sm"
            variant="ghost"
            disabled={clampedIndex >= safePages.length - 1}
            onClick={() => setPageIndex((p) => Math.min(safePages.length - 1, p + 1))}
            aria-label="Next PDF page"
          >
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.max(50, z - 10))}
            aria-label="Zoom out PDF"
          >
            −
          </Button>
          <span className="ky-doc-preview__counter">{zoomPercent}%</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.min(200, z + 10))}
            aria-label="Zoom in PDF"
          >
            +
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() =>
              onSendSelectionToChat(
                `[PDF p.${clampedIndex + 1}] ${headingReport.sanitizedText}: ${bodyReport.sanitizedText}`,
                filePath,
              )
            }
          >
            Send selection to chat
          </Button>
        </div>
      </header>

      <div className="ky-doc-preview__stage">
        <article
          className="ky-doc-preview__sheet"
          style={{ fontSize: `${zoomPercent}%` }}
        >
          <h3 className="ky-doc-preview__heading">{headingReport.sanitizedText}</h3>
          <p className="ky-doc-preview__body">{bodyReport.sanitizedText}</p>
        </article>
      </div>
    </section>
  );
}
