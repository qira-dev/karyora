/**
 * `KnowledgePack` Library & Import/Create Surface (UIR-7.1, Blueprint §12.2, E2E-14).
 *
 * Manages SHA-256-verified KnowledgePacks, Quarantine disclosure, Version inspection,
 * and Import/Create modal backed by durable `knowledge-ingest` AIJobRelay Jobs.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../../components/icons";
import { Badge, Button, Input, ModalDialog } from "../../components/primitives";
import {
  detectDuplicateSources,
  type KnowledgePackRecord,
} from "./knowledgeConfig";
import { KnowledgePackDetailDrawer } from "./KnowledgePackDetailDrawer";

export interface KnowledgePackPageProps {
  readonly packs: readonly KnowledgePackRecord[];
  readonly onCreatePackAndIngest: (name: string, sourceUri: string) => void;
  readonly onExportManifest: (pack: KnowledgePackRecord) => void;
  readonly onInspectIngestionJob?: (jobId: string) => void;
}

export function KnowledgePackPage({
  packs,
  onCreatePackAndIngest,
  onExportManifest,
  onInspectIngestionJob,
}: KnowledgePackPageProps): ReactElement {
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedPackId, setSelectedPackId] = useState<string | null>(null);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [newPackName, setNewPackName] = useState("");
  const [newSourceUri, setNewSourceUri] = useState("workspace://docs/");

  const filtered = packs.filter((p) => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return true;
    return (
      p.name.toLowerCase().includes(q) ||
      p.description.toLowerCase().includes(q) ||
      p.manifestSha256.toLowerCase().includes(q)
    );
  });

  const selectedPack = selectedPackId
    ? packs.find((p) => p.packId === selectedPackId) ?? null
    : null;

  return (
    <section
      className="ky-domain-page"
      data-testid="knowledgepack-page"
      aria-label="KnowledgePack Library"
    >
      <header className="ky-domain-page__header">
        <div className="ky-domain-page__title-group">
          <h2 className="ky-domain-page__title">KnowledgePack Library</h2>
          <p className="ky-domain-page__subtitle">
            SHA-256 provenance-verified knowledge bundles (`uawp-kp-internal-digest-v1`). Quarantined packs are excluded fail-closed.
          </p>
        </div>

        <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "center" }}>
          <Input
            aria-label="Search KnowledgePacks"
            placeholder="Search packs or SHA-256..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
          <Button
            size="sm"
            variant="primary"
            onClick={() => setImportModalOpen(true)}
          >
            <ChromeIcon name="plus" size={14} />
            <span>Import / Create Pack</span>
          </Button>
        </div>
      </header>

      <div className="ky-kp-grid">
        {filtered.map((pack) => {
          const dupes = detectDuplicateSources(pack.sources);
          return (
            <article
              key={pack.packId}
              className="ky-kp-card"
              data-integrity={pack.integrityState}
              data-testid={`kp-card-${pack.packId}`}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
                <div style={{ display: "flex", justifyContent: "space-between", gap: "var(--space-2)" }}>
                  <strong>
                    {pack.name} (v{pack.versionNumber})
                  </strong>
                  <Badge
                    tone={
                      pack.integrityState === "verified"
                        ? "success"
                        : pack.integrityState === "quarantined"
                          ? "danger"
                          : "warning"
                    }
                    label={pack.integrityState}
                  />
                </div>
                <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
                  {pack.description}
                </p>
                <code className="ky-kp-digest">{pack.manifestSha256}</code>
                <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
                  <Badge tone="neutral" label={`${pack.sources.length} sources`} />
                  <Badge tone="info" label={pack.signatureStatus} />
                  {dupes.hasDuplicates && (
                    <Badge tone="warning" label="Duplicate source flagged" />
                  )}
                </div>
              </div>

              <div className="ky-plugin-card__footer">
                <span style={{ fontSize: "0.76rem", color: "var(--text-muted)" }}>
                  Updated {pack.updatedAt}
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setSelectedPackId(pack.packId)}
                >
                  Inspect Provenance
                </Button>
              </div>
            </article>
          );
        })}
      </div>

      <KnowledgePackDetailDrawer
        pack={selectedPack}
        onExportManifest={onExportManifest}
        onInspectIngestionJob={onInspectIngestionJob}
        onClose={() => setSelectedPackId(null)}
      />

      <ModalDialog
        isOpen={importModalOpen}
        title="Import / Create KnowledgePack"
        description="Launches a durable AIJobRelay knowledge-ingest Job with SHA-256 verification and quarantine enforcement."
        onClose={() => setImportModalOpen(false)}
      >
        <div className="ky-modal-form">
          <Input
            label="KnowledgePack name"
            placeholder="e.g. Engineering Runbooks"
            value={newPackName}
            onChange={(e) => setNewPackName(e.target.value)}
          />
          <Input
            label="Source folder or archive URI"
            placeholder="workspace://docs/runbooks"
            value={newSourceUri}
            onChange={(e) => setNewSourceUri(e.target.value)}
          />
          <div className="ky-modal-form__footer">
            <span style={{ fontSize: "0.75rem", color: "var(--text-muted)" }}>
              Profile: uawp-kp-internal-digest-v1 (D-41)
            </span>
            <div style={{ display: "flex", gap: "var(--space-2)" }}>
              <Button size="sm" variant="ghost" onClick={() => setImportModalOpen(false)}>
                Cancel
              </Button>
              <Button
                size="sm"
                variant="primary"
                disabled={!newPackName.trim() || !newSourceUri.trim()}
                onClick={() => {
                  onCreatePackAndIngest(newPackName.trim(), newSourceUri.trim());
                  setNewPackName("");
                  setImportModalOpen(false);
                }}
              >
                Create & Start Ingestion Job
              </Button>
            </div>
          </div>
        </div>
      </ModalDialog>
    </section>
  );
}
