/**
 * Sandboxed PPTX Presentation Previewer (UIR-5.4, UI-39, Blueprint §22.3, Quality Gate G-12).
 *
 * Renders slides in a 16:9 canvas with thumbnail rail, speaker notes, zoom, and zero script
 * or macro execution.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";
import { sanitizeDocumentPayload } from "./previewerSanitizer";

export interface PptxSlideData {
  readonly slideNumber: number;
  readonly title: string;
  readonly bullets: readonly string[];
  readonly speakerNotes: string;
}

export interface PptxPreviewerProps {
  readonly filePath: string;
  readonly slides?: readonly PptxSlideData[];
  readonly onSendSelectionToChat: (excerpt: string, sourceFile: string) => void;
}

const DEFAULT_PPTX_SLIDES: readonly PptxSlideData[] = [
  {
    slideNumber: 1,
    title: "Karyora v3.0 — One SPA, Four Platforms",
    bullets: [
      "Single React 19 + TypeScript + Vite SPA embedded via Go ui/embed.go",
      "Serves Web Browser, Linux, macOS, and Windows with identical CoreClient contracts",
      "1,147 Material SVG icons + 20 Light/Dark code syntax themes",
    ],
    speakerNotes: "Highlight zero divergence between web and desktop frontends.",
  },
  {
    slideNumber: 2,
    title: "Zero-Script Sandboxed Previews & AIJobRelay",
    bullets: [
      "PDF, DOCX, XLSX, and PPTX render with scripts and Office macros stripped",
      "10k+ durable Jobs and 50k+ Workspace files use O(1) virtualization",
      "Human-in-the-loop approvals remain auditable and fail-closed",
    ],
    speakerNotes: "Call out G-12 sandbox enforcement and G-13 frame-rate budgets.",
  },
];

export default function PptxPreviewer({
  filePath,
  slides = DEFAULT_PPTX_SLIDES,
  onSendSelectionToChat,
}: PptxPreviewerProps): ReactElement {
  const safeSlides = slides.length > 0 ? slides : DEFAULT_PPTX_SLIDES;
  const [slideIndex, setSlideIndex] = useState(0);
  const [zoomPercent, setZoomPercent] = useState(100);

  const clamped = Math.min(Math.max(0, slideIndex), safeSlides.length - 1);
  const current = safeSlides[clamped];

  const titleRep = sanitizeDocumentPayload(current.title);
  const bulletReps = current.bullets.map((b) => sanitizeDocumentPayload(b));
  const notesRep = sanitizeDocumentPayload(current.speakerNotes);
  const hadRisk =
    titleRep.hadExecutableRisk ||
    notesRep.hadExecutableRisk ||
    bulletReps.some((b) => b.hadExecutableRisk);

  return (
    <section
      className="ky-doc-preview"
      data-testid="pptx-previewer"
      data-sandbox-mode="zero-script"
      aria-label={`PPTX preview: ${filePath}`}
    >
      <header className="ky-doc-preview__toolbar">
        <div className="ky-doc-preview__meta">
          <Badge tone="info" label="PPTX · Sandboxed" />
          <span className="ky-doc-preview__path" title={filePath}>
            {filePath}
          </span>
          {hadRisk && <Badge tone="warning" label="Active content stripped" />}
        </div>

        <div className="ky-doc-preview__controls">
          <Button
            size="sm"
            variant="ghost"
            disabled={clamped === 0}
            onClick={() => setSlideIndex((i) => Math.max(0, i - 1))}
            aria-label="Previous slide"
          >
            <ChromeIcon name="chevron-left" size={14} />
          </Button>
          <span className="ky-doc-preview__counter" data-testid="pptx-slide-indicator">
            Slide {clamped + 1} / {safeSlides.length}
          </span>
          <Button
            size="sm"
            variant="ghost"
            disabled={clamped >= safeSlides.length - 1}
            onClick={() => setSlideIndex((i) => Math.min(safeSlides.length - 1, i + 1))}
            aria-label="Next slide"
          >
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.max(60, z - 10))}
            aria-label="Zoom out PPTX"
          >
            −
          </Button>
          <span className="ky-doc-preview__counter">{zoomPercent}%</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setZoomPercent((z) => Math.min(160, z + 10))}
            aria-label="Zoom in PPTX"
          >
            +
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() =>
              onSendSelectionToChat(
                `[PPTX Slide ${clamped + 1}] ${titleRep.sanitizedText}: ${bulletReps.map((b) => b.sanitizedText).join("; ")}`,
                filePath,
              )
            }
          >
            Send selection to chat
          </Button>
        </div>
      </header>

      <div className="ky-doc-preview__split-body">
        <aside className="ky-doc-preview__outline" aria-label="Slide thumbnails">
          {safeSlides.map((s, idx) => (
            <button
              key={s.slideNumber}
              type="button"
              className="ky-doc-preview__outline-btn"
              data-active={idx === clamped ? "true" : "false"}
              onClick={() => setSlideIndex(idx)}
            >
              {idx + 1}. {sanitizeDocumentPayload(s.title).sanitizedText}
            </button>
          ))}
        </aside>

        <div className="ky-doc-preview__stage">
          <div
            className="ky-pptx-canvas"
            style={{ fontSize: `${zoomPercent}%` }}
          >
            <h3 className="ky-doc-preview__heading">{titleRep.sanitizedText}</h3>
            <ul className="ky-pptx-bullets">
              {bulletReps.map((b, idx) => (
                <li key={`bullet-${idx}`}>{b.sanitizedText}</li>
              ))}
            </ul>
          </div>

          <div className="ky-pptx-notes">
            <strong>Speaker notes:</strong> {notesRep.sanitizedText}
          </div>
        </div>
      </div>
    </section>
  );
}
