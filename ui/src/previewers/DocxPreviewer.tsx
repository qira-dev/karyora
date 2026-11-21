/**
 * Sandboxed DOCX Document Previewer (UIR-5.4, UI-39, Blueprint §22.3, Quality Gate G-12).
 *
 * Renders structured Word document sections with zero ActiveX, VBA macro, or script execution.
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../components/primitives";
import { sanitizeDocumentPayload } from "./previewerSanitizer";

export interface DocxSectionBlock {
  readonly sectionId: string;
  readonly heading: string;
  readonly paragraphs: readonly string[];
}

export interface DocxPreviewerProps {
  readonly filePath: string;
  readonly sections?: readonly DocxSectionBlock[];
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

const DEFAULT_DOCX_SECTIONS: readonly DocxSectionBlock[] = [
  {
    sectionId: "sec-1",
    heading: "1. Executive Summary",
    paragraphs: [
      "Karyora serves Web Browser, Windows, macOS, and Linux from a single React 19 + TypeScript + Vite SPA embedded into the Go Core binary.",
    ],
  },
  {
    sectionId: "sec-2",
    heading: "2. Sandboxed Office Document Policy",
    paragraphs: [
      "Embedded Word VBA macros, external XML template fetches, and raw HTML script tags are stripped prior to rendering (G-12).",
    ],
  },
];

export default function DocxPreviewer({
  filePath,
  sections = DEFAULT_DOCX_SECTIONS,
  onSendSelectionToChat,
}: DocxPreviewerProps): ReactElement {
  const [selectedSectionId, setSelectedSectionId] = useState<string>(
    (sections[0] ?? DEFAULT_DOCX_SECTIONS[0]).sectionId,
  );
  const [zoomPercent, setZoomPercent] = useState(100);

  const safeSections = sections.length > 0 ? sections : DEFAULT_DOCX_SECTIONS;
  const sanitizedSections = safeSections.map((sec) => {
    const h = sanitizeDocumentPayload(sec.heading);
    const paras = sec.paragraphs.map((p) => sanitizeDocumentPayload(p));
    return {
      sectionId: sec.sectionId,
      heading: h.sanitizedText,
      paragraphs: paras.map((p) => p.sanitizedText),
      hadRisk: h.hadExecutableRisk || paras.some((p) => p.hadExecutableRisk),
    };
  });

  const activeSection =
    sanitizedSections.find((s) => s.sectionId === selectedSectionId) ??
    sanitizedSections[0];
  const anyRiskStripped = sanitizedSections.some((s) => s.hadRisk);

  return (
    <section
      className="ky-doc-preview"
      data-testid="docx-previewer"
      data-sandbox-mode="zero-script"
      aria-label={`DOCX preview: ${filePath}`}
    >
      <header className="ky-doc-preview__toolbar">
        <div className="ky-doc-preview__meta">
          <Badge tone="info" label="DOCX · Macro-Inert" />
          <span className="ky-doc-preview__path" title={filePath}>
            {filePath}
          </span>
          {anyRiskStripped && <Badge tone="warning" label="Macros / scripts stripped" />}
        </div>

        <div className="ky-doc-preview__controls">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.max(70, z - 10))}
            aria-label="Zoom out DOCX"
          >
            −
          </Button>
          <span className="ky-doc-preview__counter">{zoomPercent}%</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.min(180, z + 10))}
            aria-label="Zoom in DOCX"
          >
            +
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() =>
              onSendSelectionToChat(
                `[DOCX ${activeSection.heading}] ${activeSection.paragraphs.join(" ")}`,
                filePath,
              )
            }
          >
            Send selection to chat
          </Button>
        </div>
      </header>

      <div className="ky-doc-preview__split-body">
        <nav className="ky-doc-preview__outline" aria-label="Document outline">
          {sanitizedSections.map((sec) => (
            <button
              key={sec.sectionId}
              type="button"
              className="ky-doc-preview__outline-btn"
              data-active={sec.sectionId === activeSection.sectionId ? "true" : "false"}
              onClick={() => setSelectedSectionId(sec.sectionId)}
            >
              {sec.heading}
            </button>
          ))}
        </nav>

        <div className="ky-doc-preview__stage">
          <article
            className="ky-doc-preview__sheet"
            style={{ fontSize: `${zoomPercent}%` }}
          >
            {sanitizedSections.map((sec) => (
              <div
                key={sec.sectionId}
                className="ky-doc-preview__section"
                data-selected={sec.sectionId === activeSection.sectionId ? "true" : "false"}
                onClick={() => setSelectedSectionId(sec.sectionId)}
              >
                <h3 className="ky-doc-preview__heading">{sec.heading}</h3>
                {sec.paragraphs.map((p, idx) => (
                  <p key={`${sec.sectionId}-p-${idx}`} className="ky-doc-preview__body">
                    {p}
                  </p>
                ))}
              </div>
            ))}
          </article>
        </div>
      </div>
    </section>
  );
}
