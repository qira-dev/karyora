/**
 * Scoped `Memory` Surface (UIR-7.2, Screenshots #26–#28, Blueprint §12.3, CR-P4-02, E2E-15).
 *
 * Shared component mounted both on the workspace `/memory` surface and inside
 * `Settings → Memory` (#26–#28). Enforces explicit `Forget` (soft exclude, restorable,
 * `memory_item.forgotten`) vs `Delete` (permanent erasure with ConfirmDialog,
 * identity tombstone, `memory_item.deleted`).
 */

import { useMemo, useState, type ReactElement } from "react";
import { ChromeIcon, MaterialFileIcon } from "../../components/icons";
import {
  Badge,
  Button,
  ConfirmDialog,
  Input,
  Select,
  Toggle,
} from "../../components/primitives";
import type { DarkCodeTheme, LightCodeTheme } from "../../types/decisions";
import {
  filterScopedMemoryItems,
  type MemoryScope,
  type ScopedMemoryItem,
} from "./memoryConfig";

export interface ScopedMemorySurfaceProps {
  readonly items: readonly ScopedMemoryItem[];
  readonly workspaceMemoryEnabled: boolean;
  readonly availableProjects: readonly string[];
  readonly activeCodeTheme?: LightCodeTheme | DarkCodeTheme;
  readonly onToggleWorkspaceMemory: (enabled: boolean) => void;
  readonly onApproveItem: (memoryId: string) => void;
  readonly onRejectItem: (memoryId: string) => void;
  readonly onEditItemContent: (memoryId: string, nextContent: string) => void;
  readonly onTogglePinItem: (memoryId: string) => void;
  readonly onForgetItem: (memoryId: string) => void;
  readonly onRestoreItem: (memoryId: string) => void;
  readonly onPermanentDeleteItem: (memoryId: string) => void;
}

export function ScopedMemorySurface({
  items,
  workspaceMemoryEnabled,
  availableProjects,
  activeCodeTheme,
  onToggleWorkspaceMemory,
  onApproveItem,
  onRejectItem,
  onEditItemContent,
  onTogglePinItem,
  onForgetItem,
  onRestoreItem,
  onPermanentDeleteItem,
}: ScopedMemorySurfaceProps): ReactElement {
  const [scope, setScope] = useState<MemoryScope>("project");
  const [selectedProject, setSelectedProject] = useState<string>(
    availableProjects[0] ?? "karyora_Platform",
  );
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedMemoryId, setSelectedMemoryId] = useState<string | null>(
    items[0]?.memoryId ?? null,
  );
  const [editingDraft, setEditingDraft] = useState<string | null>(null);
  const [confirmDeleteItem, setConfirmDeleteItem] = useState<ScopedMemoryItem | null>(null);

  const visibleItems = useMemo(
    () => filterScopedMemoryItems(items, scope, selectedProject, searchQuery),
    [items, scope, selectedProject, searchQuery],
  );

  const selectedItem =
    visibleItems.find((i) => i.memoryId === selectedMemoryId) ??
    visibleItems[0] ??
    null;

  const projectCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of items) {
      if (item.scope === "project" && item.projectName) {
        counts.set(item.projectName, (counts.get(item.projectName) ?? 0) + 1);
      }
    }
    return counts;
  }, [items]);

  return (
    <section
      className="ky-domain-page"
      data-testid="scoped-memory-surface"
      aria-label="Scoped Memory Manager"
    >
      <header className="ky-domain-page__header">
        <div className="ky-domain-page__title-group">
          <h2 className="ky-domain-page__title">Scoped Memory</h2>
          <p className="ky-domain-page__subtitle">
            Review, pin, soft-exclude (Forget), or permanently erase (Delete) User and Project memory items.
          </p>
        </div>

        <Toggle
          label="Workspace Memory"
          description="Include approved active memories in Context assembly"
          checked={workspaceMemoryEnabled}
          onChange={onToggleWorkspaceMemory}
        />
      </header>

      <div className="ky-memory-controls">
        <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap", alignItems: "center" }}>
          <Select
            aria-label="Memory scope"
            value={scope}
            options={[
              { value: "user", label: "This computer · User" },
              { value: "project", label: "This computer · Projects" },
            ]}
            onChange={(e) => {
              setScope(e.target.value as MemoryScope);
              setEditingDraft(null);
            }}
          />

          {scope === "project" && (
            <Select
              aria-label="Memory project filter"
              value={selectedProject}
              options={availableProjects.map((proj) => ({
                value: proj,
                label: `${proj} (${projectCounts.get(proj) ?? 0})`,
              }))}
              onChange={(e) => setSelectedProject(e.target.value)}
            />
          )}
        </div>

        <div style={{ minWidth: "240px" }}>
          <Input
            aria-label="Search memory files"
            placeholder="Search memory files..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      <div className="ky-memory-split">
        <div className="ky-memory-list" role="listbox" aria-label="Memory items">
          {visibleItems.length === 0 ? (
            <p style={{ color: "var(--text-muted)", fontSize: "0.82rem", padding: "var(--space-3)" }}>
              No memory files match the active scope and filter.
            </p>
          ) : (
            visibleItems.map((item) => (
              <button
                key={item.memoryId}
                type="button"
                role="option"
                aria-selected={item.memoryId === selectedItem?.memoryId}
                className="ky-memory-row"
                data-selected={item.memoryId === selectedItem?.memoryId ? "true" : "false"}
                onClick={() => {
                  setSelectedMemoryId(item.memoryId);
                  setEditingDraft(null);
                }}
              >
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "6px" }}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "6px", fontWeight: 600, fontSize: "0.82rem" }}>
                    <MaterialFileIcon
                      filename={item.filename}
                      codeTheme={activeCodeTheme}
                      size={16}
                    />
                    <span>{item.filename}</span>
                  </span>
                  <Badge
                    tone={
                      item.state === "active"
                        ? "success"
                        : item.state === "pending_review"
                          ? "warning"
                          : item.state === "deleted"
                            ? "danger"
                            : "neutral"
                    }
                    label={item.state.replace("_", " ")}
                  />
                </div>
                <span style={{ fontSize: "0.76rem", color: "var(--text-secondary)" }}>
                  {item.title}
                </span>
              </button>
            ))
          )}
        </div>

        <div className="ky-memory-detail" data-testid="memory-detail-panel">
          {selectedItem ? (
            <>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: "var(--space-2)", flexWrap: "wrap" }}>
                <div>
                  <h3 style={{ margin: "0 0 4px 0", fontSize: "1rem" }}>
                    {selectedItem.title}
                  </h3>
                  <div style={{ display: "flex", gap: "6px", flexWrap: "wrap" }}>
                    <Badge tone="info" label={selectedItem.filename} />
                    <Badge tone="neutral" label={`Rev: ${selectedItem.revision}`} />
                    {selectedItem.pinned && <Badge tone="accent" label="Pinned" />}
                  </div>
                </div>

                {selectedItem.state !== "deleted" && (
                  <div style={{ display: "flex", gap: "6px", flexWrap: "wrap" }}>
                    <Button
                      size="sm"
                      variant={selectedItem.pinned ? "secondary" : "ghost"}
                      onClick={() => onTogglePinItem(selectedItem.memoryId)}
                    >
                      <ChromeIcon name="pin" size={14} />
                      <span>{selectedItem.pinned ? "Unpin" : "Pin"}</span>
                    </Button>

                    {selectedItem.state === "forgotten" ? (
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => onRestoreItem(selectedItem.memoryId)}
                      >
                        <ChromeIcon name="refresh" size={14} />
                        <span>Restore</span>
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => onForgetItem(selectedItem.memoryId)}
                        data-testid="memory-forget-btn"
                      >
                        <span>Forget (Soft Exclude)</span>
                      </Button>
                    )}

                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setConfirmDeleteItem(selectedItem)}
                      data-testid="memory-delete-btn"
                    >
                      <ChromeIcon name="trash" size={14} />
                      <span>Delete Permanently</span>
                    </Button>
                  </div>
                )}
              </div>

              {selectedItem.state === "pending_review" && (
                <div className="ky-memory-pending-bar" data-testid="memory-pending-review-bar">
                  <span>Pending operator review before entering Context assembly.</span>
                  <div style={{ display: "flex", gap: "6px" }}>
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => onApproveItem(selectedItem.memoryId)}
                    >
                      Approve
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => setEditingDraft(selectedItem.content ?? "")}
                    >
                      Edit
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => onRejectItem(selectedItem.memoryId)}
                    >
                      Reject
                    </Button>
                  </div>
                </div>
              )}

              {selectedItem.state === "deleted" ? (
                <div className="ky-memory-tombstone" data-testid="memory-deleted-tombstone">
                  <strong>Permanently Deleted Tombstone (Irreversible)</strong>
                  <p style={{ margin: "6px 0 0 0" }}>
                    Content was permanently erased (`memory_item.deleted`). Identity tombstone
                    preserved for audit integrity ({selectedItem.tombstoneAuditRef ?? "audit-logged"}).
                  </p>
                </div>
              ) : editingDraft !== null ? (
                <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
                  <textarea
                    className="ky-modal-form__textarea"
                    value={editingDraft}
                    onChange={(e) => setEditingDraft(e.target.value)}
                  />
                  <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-2)" }}>
                    <Button size="sm" variant="ghost" onClick={() => setEditingDraft(null)}>
                      Cancel
                    </Button>
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => {
                        onEditItemContent(selectedItem.memoryId, editingDraft);
                        setEditingDraft(null);
                      }}
                    >
                      Save edit
                    </Button>
                  </div>
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
                  <p style={{ margin: 0, lineHeight: 1.6, fontSize: "0.88rem" }}>
                    {selectedItem.content}
                  </p>
                  {selectedItem.state === "active" && (
                    <div>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditingDraft(selectedItem.content ?? "")}
                      >
                        <ChromeIcon name="edit" size={14} />
                        <span>Edit memory text</span>
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </>
          ) : (
            <p style={{ color: "var(--text-muted)", fontSize: "0.84rem" }}>
              Select a memory file from the list to inspect, approve, forget, or permanently delete.
            </p>
          )}
        </div>
      </div>

      <ConfirmDialog
        isOpen={Boolean(confirmDeleteItem)}
        title={`Permanently delete ${confirmDeleteItem?.filename ?? "memory"}?`}
        explanation="Delete permanently erases memory content (`memory_item.deleted`) and cannot be restored. Use Forget instead if you only want to soft-exclude it from prompts."
        confirmLabel="Permanently Delete"
        onConfirm={() => {
          if (confirmDeleteItem) {
            onPermanentDeleteItem(confirmDeleteItem.memoryId);
          }
          setConfirmDeleteItem(null);
        }}
        onCancel={() => setConfirmDeleteItem(null)}
      />
    </section>
  );
}
