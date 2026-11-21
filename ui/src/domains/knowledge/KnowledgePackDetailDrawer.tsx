/**
 * KnowledgePack Detail, SHA-256 Provenance, Duplicate Detection & Sanitized Preview Drawer
 * (UIR-7.1, Blueprint §12.2, G-12 zero-script, E2E-14).
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, ModalDialog } from "../../components/primitives";
import { sanitizeDocumentPayload } from "../../previewers/previewerSanitizer";
import {
  detectDuplicateSources,
  isValidSha256Digest,
  type KnowledgePackRecord,
} from "./knowledgeConfig";

export interface KnowledgePackDetailDrawerProps {
  readonly pack: KnowledgePackRecord | null;
  readonly onExportManifest: (pack: KnowledgePackRecord) => void;
  readonly onInspectIngestionJob?: (jobId: string) => void;
  readonly onClose: () => void;
}

export function KnowledgePackDetailDrawer({
  pack,
  onExportManifest,
  onInspectIngestionJob,
  onClose,
}: KnowledgePackDetailDrawerProps): ReactElement | null {
  const [selectedSourceId, setSelectedSourceId] = useState<string | null>(null);

  if (!pack) return null;

  const duplicateReport = detectDuplicateSources(pack.sources);
  const digestValid = isValidSha256Digest(pack.manifestSha256);
  const selectedSource =
    pack.sources.find((s) => s.sourceId === selectedSourceId) ?? pack.sources[0] ?? null;
  const sanitizedPreview = selectedSource
    ? sanitizeDocumentPayload(selectedSource.rawPreviewExcerpt)
    : null;

  return (
    <ModalDialog
      isOpen={Boolean(pack)}
      title={`KnowledgePack — ${pack.name} (v${pack.versionNumber})`}
      description={`Profile: ${pack.profile} (D-41 local/internal digest format)`}
      onClose={onClose}
    >
      <div
        style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}
        data-testid="knowledgepack-detail-drawer"
      >
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Badge
            tone={
              pack.integrityState === "verified"
                ? "success"
                : pack.integrityState === "quarantined"
                  ? "danger"
                  : "warning"
            }
            label={`Integrity: ${pack.integrityState}`}
          />
          <Badge
            tone={digestValid ? "info" : "danger"}
            label={digestValid ? "SHA-256 Valid" : "Invalid SHA-256"}
          />
          <Badge tone="neutral" label={`Signature: ${pack.signatureStatus}`} />
          {duplicateReport.hasDuplicates && (
            <Badge
              tone="warning"
              label={`${duplicateReport.duplicatePairs.length} duplicate(s) detected`}
            />
          )}
        </div>

        <div className="ky-gov-section">
          <strong>Manifest Checksum (SHA-256)</strong>
          <code className="ky-kp-digest">{pack.manifestSha256}</code>
        </div>

        <div className="ky-gov-section">
          <strong>Source Provenance ({pack.sources.length})</strong>
          <table className="ky-step-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>Origin</th>
                <th>SHA-256</th>
                <th>Size</th>
              </tr>
            </thead>
            <tbody>
              {pack.sources.map((src) => (
                <tr
                  key={src.sourceId}
                  data-selected={src.sourceId === selectedSource?.sourceId ? "true" : "false"}
                  onClick={() => setSelectedSourceId(src.sourceId)}
                >
                  <td>{src.title}</td>
                  <td style={{ fontFamily: "var(--font-mono)", fontSize: "0.72rem" }}>
                    {src.originUri}
                  </td>
                  <td style={{ fontFamily: "var(--font-mono)", fontSize: "0.7rem" }}>
                    {src.sha256.slice(0, 19)}…
                  </td>
                  <td>{Math.max(1, Math.round(src.byteSize / 1024))} KB</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {selectedSource && sanitizedPreview && (
          <div className="ky-gov-section" data-testid="knowledgepack-sanitized-preview">
            <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
              <strong>Sanitized Source Preview — {selectedSource.title}</strong>
              {sanitizedPreview.hadExecutableRisk && (
                <Badge tone="warning" label="Active scripts stripped (G-12)" />
              )}
            </div>
            <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
              {sanitizedPreview.sanitizedText}
            </p>
          </div>
        )}

        <div className="ky-gov-section">
          <strong>Version History ({pack.versions.length})</strong>
          {pack.versions.map((ver) => (
            <div
              key={ver.versionNumber}
              style={{ display: "flex", justifyContent: "space-between", fontSize: "0.78rem" }}
            >
              <span>
                <strong>v{ver.versionNumber}</strong> — {ver.summary} ({ver.sourceCount} sources)
              </span>
              <span style={{ color: "var(--text-muted)" }}>{ver.createdAt}</span>
            </div>
          ))}
        </div>

        <div className="ky-modal-form__footer">
          <div style={{ display: "flex", gap: "var(--space-2)" }}>
            {onInspectIngestionJob && (
              <Button
                size="sm"
                variant="secondary"
                onClick={() => onInspectIngestionJob(pack.lastIngestionJobId)}
              >
                Inspect Ingestion Job ({pack.lastIngestionJobId})
              </Button>
            )}
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onExportManifest(pack)}
            >
              <ChromeIcon name="copy" size={14} />
              <span>Export Manifest JSON</span>
            </Button>
          </div>
          <Button size="sm" variant="ghost" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </ModalDialog>
  );
}
