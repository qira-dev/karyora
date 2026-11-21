/**
 * Scoped Memory Domain Helpers: User vs Project Scopes, Review Lifecycle, and
 * Explicit `Forget` vs `Delete` Distinctness (UIR-7.2, Screenshots #26–#28,
 * Blueprint §12.3, CR-P4-02, G-09, E2E-15).
 *
 * Pure TypeScript module (import-type only) for direct Vite & Node 26 testing.
 */

export type MemoryScope = "user" | "project";

export type MemoryLifecycleState =
  | "pending_review"
  | "active"
  | "forgotten"
  | "deleted";

export interface ScopedMemoryItem {
  readonly memoryId: string;
  readonly filename: string;
  readonly title: string;
  readonly scope: MemoryScope;
  readonly projectName?: string;
  readonly state: MemoryLifecycleState;
  readonly pinned: boolean;
  readonly revision: string;
  readonly updatedAt: string;
  readonly content: string | null; // null after permanent Delete (tombstone only)
  readonly tombstoneAuditRef?: string;
}

export interface MemoryTransitionOutcome {
  readonly item: ScopedMemoryItem;
  readonly emittedAuditEvent:
    | "memory_item.approved"
    | "memory_item.edited"
    | "memory_item.rejected"
    | "memory_item.forgotten"
    | "memory_item.restored"
    | "memory_item.deleted";
  readonly restorable: boolean;
}

export const INITIAL_MEMORY_ITEMS: readonly ScopedMemoryItem[] = [
  {
    memoryId: "mem-user-prefs",
    filename: "MEMORY.md",
    title: "Owner Communication & Delivery Preferences",
    scope: "user",
    state: "active",
    pinned: true,
    revision: "mrev-004",
    updatedAt: "5m ago",
    content:
      "Owner communicates in Bengali and prefers concise milestone status summaries while keeping full technical proof in Obsidian.",
  },
  {
    memoryId: "mem-proj-folder",
    filename: "session-folder-selection.md",
    title: "Workspace Repository & Go Embed Boundary",
    scope: "project",
    projectName: "karyora_Platform",
    state: "active",
    pinned: true,
    revision: "mrev-002",
    updatedAt: "18m ago",
    content:
      "Active repository is /home/km-abdullah/karyora_Platform/karyora (origin qira-dev/karyora). Single ui/dist embedded via ui/embed.go.",
  },
  {
    memoryId: "mem-proj-pending",
    filename: "pending-ci-note.md",
    title: "Node 26 Native TS Test Runner Rule",
    scope: "project",
    projectName: "karyora_Platform",
    state: "pending_review",
    pinned: false,
    revision: "mrev-001",
    updatedAt: "2m ago",
    content:
      "Keep pure helper modules (.ts) free of extensionless runtime imports so node --test imports them directly.",
  },
  {
    memoryId: "mem-proj-forgotten",
    filename: "legacy-v2-prototype-note.md",
    title: "Archived v2.0 UI Reference",
    scope: "project",
    projectName: "karyora_Platform",
    state: "forgotten",
    pinned: false,
    revision: "mrev-003",
    updatedAt: "2h ago",
    content:
      "Old v2.0 prototype paths were archived prior to the clean-slate v3.0 rebuild.",
  },
];

/**
 * Soft-excludes a memory item from context assembly (`memory_item.forgotten`).
 * Content is preserved so the operator can restore it later (`Forget` != `Delete`, G-09).
 */
export function forgetMemoryItem(item: ScopedMemoryItem): MemoryTransitionOutcome {
  return {
    item: {
      ...item,
      state: "forgotten",
      pinned: false,
      updatedAt: "Just now",
    },
    emittedAuditEvent: "memory_item.forgotten",
    restorable: true,
  };
}

/**
 * Restores a previously forgotten memory item back to active inclusion.
 * Permanently deleted items CANNOT be restored.
 */
export function restoreMemoryItem(item: ScopedMemoryItem): MemoryTransitionOutcome {
  if (item.state === "deleted") {
    throw new Error("CANNOT_RESTORE_PERMANENTLY_DELETED_MEMORY");
  }
  return {
    item: {
      ...item,
      state: "active",
      updatedAt: "Just now",
    },
    emittedAuditEvent: "memory_item.restored",
    restorable: true,
  };
}

/**
 * Permanently erases memory item content (`memory_item.deleted`) while leaving only
 * an irreversible identity/audit tombstone (`M-16` removal discipline, G-09).
 */
export function permanentlyDeleteMemoryItem(
  item: ScopedMemoryItem,
  auditRef: string,
): MemoryTransitionOutcome {
  return {
    item: {
      ...item,
      state: "deleted",
      pinned: false,
      content: null,
      tombstoneAuditRef: auditRef,
      updatedAt: "Just now",
    },
    emittedAuditEvent: "memory_item.deleted",
    restorable: false,
  };
}

/**
 * Filters scoped memory items by scope (`user` vs `project`), project name, and search query.
 */
export function filterScopedMemoryItems(
  items: readonly ScopedMemoryItem[],
  scope: MemoryScope,
  projectName: string,
  searchQuery: string,
): readonly ScopedMemoryItem[] {
  const q = searchQuery.trim().toLowerCase();
  return items.filter((item) => {
    if (item.scope !== scope) return false;
    if (scope === "project" && projectName && item.projectName !== projectName) {
      return false;
    }
    if (!q) return true;
    return (
      item.filename.toLowerCase().includes(q) ||
      item.title.toLowerCase().includes(q) ||
      (item.content ?? "").toLowerCase().includes(q)
    );
  });
}
