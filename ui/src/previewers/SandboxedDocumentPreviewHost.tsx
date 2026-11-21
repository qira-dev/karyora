/**
 * Lazy-Loaded Sandboxed Document Preview Host (UIR-5.4, UI-39, Blueprint §22.3, G-12).
 *
 * Uses dynamic `import()` via `React.lazy` so PDF, DOCX, XLSX, and PPTX previewers
 * are code-split out of the initial JS bundle and mounted strictly on demand.
 */

import { Suspense, lazy, type ReactElement } from "react";
import { detectDocumentPreviewKind } from "./previewerSanitizer";

const LazyPdfPreviewer = lazy(() => import("./PdfPreviewer"));
const LazyDocxPreviewer = lazy(() => import("./DocxPreviewer"));
const LazyXlsxPreviewer = lazy(() => import("./XlsxPreviewer"));
const LazyPptxPreviewer = lazy(() => import("./PptxPreviewer"));

export interface SandboxedDocumentPreviewHostProps {
  readonly filePath: string;
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

export function SandboxedDocumentPreviewHost({
  filePath,
  onSendSelectionToChat,
}: SandboxedDocumentPreviewHostProps): ReactElement {
  const kind = detectDocumentPreviewKind(filePath);

  return (
    <Suspense
      fallback={
        <div className="ky-doc-preview__loading" role="status">
          Loading sandboxed {kind.toUpperCase()} previewer...
        </div>
      }
    >
      {kind === "pdf" ? (
        <LazyPdfPreviewer
          filePath={filePath}
          onSendSelectionToChat={onSendSelectionToChat}
        />
      ) : kind === "docx" ? (
        <LazyDocxPreviewer
          filePath={filePath}
          onSendSelectionToChat={onSendSelectionToChat}
        />
      ) : kind === "xlsx" ? (
        <LazyXlsxPreviewer
          filePath={filePath}
          onSendSelectionToChat={onSendSelectionToChat}
        />
      ) : (
        <LazyPptxPreviewer
          filePath={filePath}
          onSendSelectionToChat={onSendSelectionToChat}
        />
      )}
    </Suspense>
  );
}
