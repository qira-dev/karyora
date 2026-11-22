/**
 * Settings Section 20: Archive Management — Calibrated 1:1 to ZCode Screenshot #59.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../../components/icons";
import { Button, ConfirmDialog, Input, Select, Toggle } from "../../components/primitives";

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
  const { t } = useTranslation();
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
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.dataGov.autoArchive", {
                defaultValue: "Auto-archive old sessions",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.dataGov.autoArchiveDesc", {
                defaultValue:
                  "Periodically archive sessions whose last activity is older than the retention window; pinned and running sessions are never affected.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Toggle
              label=""
              aria-label="Auto-archive old sessions"
              checked={autoArchive}
              onChange={setAutoArchive}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <div className="ky-setting-row__title">
              {t("ui.settings.dataGov.archiveRetention", {
                defaultValue: "Archive retention",
              })}
            </div>
            <div className="ky-setting-row__desc">
              {t("ui.settings.dataGov.archiveRetentionDesc", {
                defaultValue:
                  "A session becomes eligible for auto-archive only after its last activity is older than this window.",
              })}
            </div>
          </div>
          <div className="ky-setting-row__control">
            <Select
              aria-label="Archive retention"
              value={retentionDays}
              options={[
                { value: "7", label: "Archive after 7 days" },
                { value: "30", label: "Archive after 30 days" },
                { value: "90", label: "Archive after 90 days" },
              ]}
              onChange={(e) => setRetentionDays(e.target.value)}
            />
          </div>
        </div>
      </div>

      <div className="ky-settings-group-header">
        <h3 className="ky-settings-group-header__title">
          <span>
            {t("ui.settings.dataGov.archivedSessions", {
              defaultValue: "Archived sessions",
            })}
          </span>
          <span className="ky-settings-group-header__count">{archived.length}</span>
        </h3>
        <Button
          size="sm"
          variant="danger"
          disabled={archived.length === 0}
          onClick={() => setConfirmPurgeOpen(true)}
        >
          <ChromeIcon name="trash" size={14} />
          <span>
            {t("ui.settings.dataGov.deleteAllArchived", {
              defaultValue: "Permanently delete all",
            })}
          </span>
        </Button>
      </div>

      <div className="ky-settings-toolbar">
        <div className="ky-settings-toolbar__left" style={{ flex: 1 }}>
          <Input
            aria-label="Search archived sessions"
            placeholder={t("ui.settings.dataGov.searchArchived", {
              defaultValue: "Search archived sessions",
            })}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      {visible.length === 0 ? (
        <div className="ky-settings-empty">
          <p className="ky-settings-empty__title">No archived sessions</p>
        </div>
      ) : (
        <div className="ky-settings-list">
          {visible.map((item) => (
            <div key={item.id} className="ky-settings-list-row">
              <div className="ky-settings-list-row__copy">
                <span className="ky-settings-list-row__title">{item.title}</span>
                <span className="ky-settings-list-row__desc">{item.archivedAt}</span>
              </div>
              <div className="ky-settings-list-row__right">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    setArchived((prev) => prev.filter((a) => a.id !== item.id));
                    onNotify(`Restored archived session "${item.title}".`);
                  }}
                >
                  {t("ui.settings.dataGov.restore", { defaultValue: "Restore" })}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <ConfirmDialog
        isOpen={confirmPurgeOpen}
        title="Permanently delete all archived sessions?"
        explanation="This irreversibly removes all archived transcripts from local storage. Active tasks and Activity Audit entries remain intact."
        confirmLabel="Permanently delete all"
        isDestructive
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
