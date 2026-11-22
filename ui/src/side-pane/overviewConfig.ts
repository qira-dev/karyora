/**
 * Antigravity-Style Right-Side `Overview` Panel & `Customize Sections` (`⋮`) Configuration
 *
 * Pure TypeScript module (`import type` only) so both React components and Node's
 * native ESM test runner (`node --test`) verify section visibility & toggle behavior directly.
 */

export type OverviewSectionId =
  | "subagents"
  | "background-tasks"
  | "goals"
  | "documents"
  | "artifacts"
  | "links"
  | "modified-files"
  | "uploads"
  | "skills-used"
  | "knowledge"
  | "memory"
  | "context"
  | "activity";

export interface OverviewSectionDefinition {
  readonly id: OverviewSectionId;
  readonly label: string;
  readonly defaultVisible: boolean;
  readonly group: "workspace" | "core";
}

/**
 * Exact Antigravity `Customize Sections` checklist (Screenshot #2) extended with
 * Karyora Core toggles (`Knowledge`, `Memory`, `Context`, `Activity & Audit`).
 */
export const OVERVIEW_SECTION_DEFINITIONS: readonly OverviewSectionDefinition[] = [
  { id: "subagents", label: "Subagents", defaultVisible: true, group: "workspace" },
  { id: "background-tasks", label: "Background Tasks", defaultVisible: true, group: "workspace" },
  { id: "goals", label: "Goals", defaultVisible: false, group: "workspace" },
  { id: "documents", label: "Documents", defaultVisible: false, group: "workspace" },
  { id: "artifacts", label: "Artifacts", defaultVisible: true, group: "workspace" },
  { id: "links", label: "Links", defaultVisible: false, group: "workspace" },
  { id: "modified-files", label: "Modified Files", defaultVisible: true, group: "workspace" },
  { id: "uploads", label: "Uploads", defaultVisible: true, group: "workspace" },
  { id: "skills-used", label: "Skills Used", defaultVisible: false, group: "workspace" },
  { id: "knowledge", label: "Knowledge", defaultVisible: false, group: "core" },
  { id: "memory", label: "Memory", defaultVisible: false, group: "core" },
  { id: "context", label: "Context", defaultVisible: false, group: "core" },
  { id: "activity", label: "Activity & Audit", defaultVisible: false, group: "core" },
] as const;

export type OverviewSectionVisibilityMap = Readonly<Record<OverviewSectionId, boolean>>;

export function createDefaultOverviewVisibility(): OverviewSectionVisibilityMap {
  const map = {} as Record<OverviewSectionId, boolean>;
  for (const def of OVERVIEW_SECTION_DEFINITIONS) {
    map[def.id] = def.defaultVisible;
  }
  return map;
}

export function toggleOverviewSectionVisibility(
  current: OverviewSectionVisibilityMap,
  sectionId: OverviewSectionId,
): OverviewSectionVisibilityMap {
  return {
    ...current,
    [sectionId]: !current[sectionId],
  };
}

export function parseOverviewVisibilityJson(
  raw: string | null,
): OverviewSectionVisibilityMap {
  const defaults = createDefaultOverviewVisibility();
  if (!raw) return defaults;
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const merged = { ...defaults } as Record<OverviewSectionId, boolean>;
    for (const def of OVERVIEW_SECTION_DEFINITIONS) {
      if (typeof parsed[def.id] === "boolean") {
        merged[def.id] = parsed[def.id] as boolean;
      }
    }
    return merged;
  } catch {
    return defaults;
  }
}

export interface OverviewArtifactItem {
  readonly id: string;
  readonly title: string;
  readonly subtitle: string;
  readonly filePath: string;
}

export interface OverviewUploadItem {
  readonly id: string;
  readonly label: string;
  readonly timestamp: string;
  readonly kind: "image" | "audio" | "file";
}

export const INITIAL_OVERVIEW_ARTIFACTS: readonly OverviewArtifactItem[] = [
  {
    id: "art-gap-audit",
    title: "ZCode Full Anatomy Gap Audit",
    subtitle: "Complete architecture & UI parity specification",
    filePath: "docs/architecture/Karyora-Platform-Spec.docx",
  },
  {
    id: "art-sec-brief",
    title: "Karyora Security Brief",
    subtitle: "G-11 / G-12 verification report",
    filePath: "docs/release/Karyora-Security-Brief.pdf",
  },
];

export const INITIAL_OVERVIEW_UPLOADS: readonly OverviewUploadItem[] = [
  {
    id: "up-1",
    label: "Workspace Preview HTML",
    timestamp: "Today",
    kind: "file",
  },
  {
    id: "up-2",
    label: "Architecture Deck Slide",
    timestamp: "Today",
    kind: "file",
  },
];
