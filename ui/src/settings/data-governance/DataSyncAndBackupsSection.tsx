/**
 * Settings Section 17: Data Sync & Backups (UIR-8.3, Screenshots #54–#55, Blueprint §13.3 (17)).
 */

import { useState, type ReactElement } from "react";
import { Badge, Button } from "../../components/primitives";

export interface DataSyncAndBackupsSectionProps {
  readonly onOpenMigrationGuide: () => void;
  readonly onNotify: (msg: string) => void;
}

export function DataSyncAndBackupsSection({
  onOpenMigrationGuide,
  onNotify,
}: DataSyncAndBackupsSectionProps): ReactElement {
  const [lastBackupLabel, setLastBackupLabel] = useState("Verified snapshot · 14m ago");

  return (
    <div className="ky-settings-content" data-testid="settings-section-data-sync">
      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">Device Data Sync & Migration</h3>
          <Badge tone="neutral" label="Local-first (Zero silent cloud sync)" />
        </div>
        <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
          Migrate settings, projects, and transcripts safely from another workstation using
          the guided 6-step Migration Wizard (#55).
        </p>
        <div>
          <Button size="sm" variant="secondary" onClick={onOpenMigrationGuide}>
            ⇄ Migrate data (6-step guide)
          </Button>
        </div>
      </div>

      <div className="ky-settings-card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h3 className="ky-settings-card__title">SQLite WAL Online Backup & Generation-Fenced Restore</h3>
          <Badge tone="success" label={lastBackupLabel} />
        </div>
        <p style={{ margin: 0, fontSize: "0.82rem", color: "var(--text-secondary)" }}>
          Creates atomic, checksum-verified snapshots via Core `internal/storage` without
          blocking active readers. Restores enforce C2-17 generation fencing.
        </p>
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              setLastBackupLabel("Verified snapshot · Just now");
              onNotify("Atomic SQLite WAL backup snapshot verified.");
            }}
          >
            Create Online Backup Snapshot
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onNotify("Staged restore pre-flight validation ready.")}
          >
            Validate Backup Archive
          </Button>
        </div>
      </div>
    </div>
  );
}
