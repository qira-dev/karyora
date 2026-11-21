/**
 * Settings Section 20: Archive Management (UIR-8.3, Screenshot #59, E2E-20).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button, ConfirmDialog, Input, Select, Toggle } from "../../components/primitives";

interface ArchivedSessionItem {
  readonly id: string;
  readonly title: string;
  readonly archivedAt: string;
}

const INITIAL_ARCHIVED: readonly ArchivedSessionItem[] = [
  { id: "arch-1", title: "Phase 1 OpenAPI Contract Drift Audit", archivedAt: "2 days ago" },
  { id: "arch-2", title: "Material SVG Manifest Verification", archivedAt: "4 days ago" },
];

export interface ArchiveManagementSectionProps {
  readonly onNotify: (msg: string) => void;
}

export function ArchiveManagementSection({
  onNotify,
}: ArchiveManagementSectionProps): ReactElement {
  const [autoArchive, setAutoArchive] = useState(true);
  const [retentionDays, setRetentionDays] = useState("30");
  const [search, setSearch] = useState("");
  const [archived, setArchived] = useState<readonly ArchivedSessionItem[]>(INITIAL_ARCHIVED);
  const [confirmPurgeOpen, setConfirmPurgeOpen] = useState(false);

  const visible = archived.filter((a) =>
    a.title.toLowerCase().includes(search.trim().toLowerCase()),
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-archive-management">
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Archive Policy & Retention</h3>
        <Toggle
          label="Auto-archive inactive sessions"
          checked={autoArchive}
          onChange={setAutoArchive}
        />
        <Select
          label="Retention period"
          value={retentionDays}
          options={[
            { value: "7", label: "Archive after 7 days" },
            { value: "30", label: "Archive after 30 days" },
            { value: "90", label: "Archive after 90 days" },
          ]}
          onChange={(e) => setRetentionDays(e.target.value)}
        />
        <div style={{ display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-2)" }}>
          <Input
            aria-label="Search archived sessions"
            placeholder="Search archived sessions..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button
            size="sm"
            variant="danger"
            disabled={archived.length === 0}
            onClick={() => setConfirmPurgeOpen(true)}
          >
            Permanently delete all ({archived.length})
          </Button>
        </div>

        {visible.length === 0 ? (
          <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-muted)" }}>
            No archived sessions.
          </p>
        ) : (
          visible.map((item) => (
            <div key={item.id} className="ky-gov-perm-row">
              <span>
                <strong>{item.title}</strong> · <Badge tone="neutral" label={item.archivedAt} />
              </span>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  setArchived((prev) => prev.filter((a) => a.id !== item.id));
                  onNotify(`Restored archived session "${item.title}".`);
                }}
              >
                Restore
              </Button>
            </div>
          ))
        )}
      </div>

      <ConfirmDialog
        isOpen={confirmPurgeOpen}
        title="Permanently delete all archived sessions?"
        explanation="This irreversibly removes all archived transcripts from local storage. Active tasks and Activity Audit entries remain intact."
        confirmLabel="Permanently delete all"
        onConfirm={() => {
          setArchived([]);
          setConfirmPurgeOpen(false);
          onNotify("All archived sessions permanently deleted.");
        }}
        onCancel={() => setConfirmPurgeOpen(false)}
      />
    </div>
  );
}
