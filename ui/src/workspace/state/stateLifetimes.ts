/**
 * Five State-Lifetime Stores (UIR-1.3, Blueprint §23.1, G-06, G-11).
 *
 * Implements the 5 distinct state-lifetime categories:
 * 1. Authoritative Domain State Cache (`DomainStateCache` — connection-scoped)
 * 2. Session-Scoped Workspace State (`WorkspaceSessionStore` — `(projectId, taskId)`-scoped)
 * 3. Draft & In-Flight Mutation State (`DraftAndMutationStore` — preserved until Core S2 ack)
 * 4. Persistent Local UI Preferences (`LocalPreferencesStore` — strict G-11 zero-secret guard)
 * 5. Ephemeral View & Focus State (`EphemeralUiStore` — overlay inertness & focus-return ref)
 */

import type {
  AppTheme,
  DarkCodeTheme,
  InterfaceMode,
  LightCodeTheme,
  SettingsSectionId,
  SidePaneTabKind,
  SupportedLocale,
} from "../../types/decisions";

const SECRET_KEY_PATTERN =
  /(secret|token|password|passwd|apikey|api_key|bearer|private_key|passphrase|credential)/i;
const SECRET_VALUE_PATTERN =
  /^(sk-[A-Za-z0-9_-]{12,}|Bearer\s+[A-Za-z0-9._~+/-]+=*|ghp_[A-Za-z0-9]{20,}|AIza[0-9A-Za-z_-]{20,})$/;

/**
 * G-11 Secret Hygiene Assertion: throws immediately if any key or string value
 * looks like a credential, API key, or bearer token.
 */
export function assertZeroSecretContent(key: string, value: unknown): void {
  if (SECRET_KEY_PATTERN.test(key)) {
    throw new Error(
      `G-11 Secret Hygiene Violation: key "${key}" is prohibited in client preference/draft storage`,
    );
  }
  if (typeof value === "string" && SECRET_VALUE_PATTERN.test(value.trim())) {
    throw new Error(
      `G-11 Secret Hygiene Violation: value for "${key}" matches a credential pattern and cannot be stored in client state`,
    );
  }
}

/**
 * Category 1: Authoritative Domain State Cache (scoped to active `connectionId`).
 * Switching Local ↔ Remote Core connections clears the cache so data never mixes
 * across connections (Blueprint §3.3, §23.1).
 */
export interface DomainCacheEntry<T> {
  readonly data: T;
  readonly revision: string;
  readonly updatedAtMs: number;
}

export class DomainStateCache {
  private activeConnectionId: string;
  private readonly entries = new Map<string, DomainCacheEntry<unknown>>();

  public constructor(initialConnectionId = "local-core") {
    this.activeConnectionId = initialConnectionId;
  }

  public getConnectionId(): string {
    return this.activeConnectionId;
  }

  public switchConnection(newConnectionId: string): void {
    if (this.activeConnectionId !== newConnectionId) {
      this.entries.clear();
      this.activeConnectionId = newConnectionId;
    }
  }

  public set<T>(resourceKey: string, data: T, revision: string): void {
    this.entries.set(resourceKey, {
      data,
      revision,
      updatedAtMs: Date.now(),
    });
  }

  public get<T>(resourceKey: string): DomainCacheEntry<T> | undefined {
    return this.entries.get(resourceKey) as DomainCacheEntry<T> | undefined;
  }

  public clear(): void {
    this.entries.clear();
  }
}

/**
 * Category 2: Session-Scoped Workspace State (indexed by `projectId::taskId`).
 * Preserves open Side-Pane tabs, Full-View mode (#83), bottom Terminal visibility (#81),
 * expanded file tree directories (#78), and diff mode per open task.
 */
export interface SidePaneTabInstance {
  readonly tabId: string;
  readonly kind: SidePaneTabKind;
  readonly title: string;
  readonly targetPathOrId?: string;
}

export interface WorkspaceSessionViewState {
  readonly projectId: string;
  readonly taskId: string;
  readonly sidePaneOpen: boolean;
  readonly fullViewMode: boolean;
  readonly bottomTerminalOpen: boolean;
  readonly openTabs: readonly SidePaneTabInstance[];
  readonly activeTabId: string | null;
  readonly expandedFolders: readonly string[];
  readonly diffViewMode: "unified" | "split";
}

export class WorkspaceSessionStore {
  private readonly states = new Map<string, WorkspaceSessionViewState>();

  private key(projectId: string, taskId: string): string {
    return `${projectId}::${taskId}`;
  }

  public getOrCreate(
    projectId: string,
    taskId: string,
  ): WorkspaceSessionViewState {
    const k = this.key(projectId, taskId);
    const existing = this.states.get(k);
    if (existing) {
      return existing;
    }
    const created: WorkspaceSessionViewState = {
      projectId,
      taskId,
      sidePaneOpen: false,
      fullViewMode: false,
      bottomTerminalOpen: false,
      openTabs: [],
      activeTabId: null,
      expandedFolders: [],
      diffViewMode: "unified",
    };
    this.states.set(k, created);
    return created;
  }

  public update(
    projectId: string,
    taskId: string,
    patch: Partial<Omit<WorkspaceSessionViewState, "projectId" | "taskId">>,
  ): WorkspaceSessionViewState {
    const current = this.getOrCreate(projectId, taskId);
    const next: WorkspaceSessionViewState = {
      ...current,
      ...patch,
    };
    this.states.set(this.key(projectId, taskId), next);
    return next;
  }
}

/**
 * Category 3: Draft & In-Flight Mutation Store (Blueprint §23.1).
 * Preserves unsent Composer prompts, context chips (`@`, `/`, `#`, `$`, `+`),
 * and dirty form buffers until Core S2 acknowledgment or explicit user discard.
 */
export interface ComposerContextChip {
  readonly chipId: string;
  readonly trigger: "@" | "/" | "#" | "$" | "+";
  readonly label: string;
  readonly targetIdOrPath: string;
}

export interface ComposerDraft {
  readonly sessionKey: string;
  readonly promptText: string;
  readonly chips: readonly ComposerContextChip[];
  readonly goalModeEnabled: boolean;
  readonly updatedAtMs: number;
}

export class DraftAndMutationStore {
  private readonly composerDrafts = new Map<string, ComposerDraft>();
  private readonly dirtyForms = new Map<string, Readonly<Record<string, unknown>>>();

  public saveComposerDraft(
    sessionKey: string,
    promptText: string,
    chips: readonly ComposerContextChip[] = [],
    goalModeEnabled = false,
  ): ComposerDraft {
    const draft: ComposerDraft = {
      sessionKey,
      promptText,
      chips: [...chips],
      goalModeEnabled,
      updatedAtMs: Date.now(),
    };
    this.composerDrafts.set(sessionKey, draft);
    return draft;
  }

  public getComposerDraft(sessionKey: string): ComposerDraft | undefined {
    return this.composerDrafts.get(sessionKey);
  }

  /**
   * Called ONLY after authoritative Core S2 turn creation acknowledgment
   * or explicit user discard (Blueprint §7.3, §20.3 B).
   */
  public clearComposerDraftOnCoreAck(sessionKey: string): void {
    this.composerDrafts.delete(sessionKey);
  }

  public setDirtyFormDraft(
    formId: string,
    values: Readonly<Record<string, unknown>>,
  ): void {
    for (const [k, v] of Object.entries(values)) {
      assertZeroSecretContent(k, v);
    }
    this.dirtyForms.set(formId, { ...values });
  }

  public getDirtyFormDraft(
    formId: string,
  ): Readonly<Record<string, unknown>> | undefined {
    return this.dirtyForms.get(formId);
  }

  public isFormDirty(formId: string): boolean {
    return this.dirtyForms.has(formId);
  }

  public clearDirtyFormDraft(formId: string): void {
    this.dirtyForms.delete(formId);
  }
}

/**
 * Category 4: Persistent Local UI Preferences Store (Screenshots #1–#9, #21–#22, #61–#65).
 * Enforces G-11 secret hygiene so credentials can never be written into preferences.
 */
export interface LocalUiPreferences {
  readonly appTheme: AppTheme;
  readonly interfaceMode: InterfaceMode;
  readonly locale: SupportedLocale;
  readonly uiFontSizePx: number;
  readonly codeFontSizePx: number;
  readonly lightCodeTheme: LightCodeTheme;
  readonly darkCodeTheme: DarkCodeTheme;
  readonly showLineNumbers: boolean;
  readonly wrapLongLines: boolean;
  readonly sidebarCollapsed: boolean;
  readonly interfaceZoomPercent: number;
  readonly interactionBehavior: "queue" | "guide";
}

export const DEFAULT_LOCAL_UI_PREFERENCES: LocalUiPreferences = {
  appTheme: "dark",
  interfaceMode: "coding",
  locale: "en",
  uiFontSizePx: 14,
  codeFontSizePx: 12,
  lightCodeTheme: "GitHub",
  darkCodeTheme: "GitHub Dark",
  showLineNumbers: true,
  wrapLongLines: true,
  sidebarCollapsed: false,
  interfaceZoomPercent: 100,
  interactionBehavior: "queue",
};

export class LocalPreferencesStore {
  private prefs: LocalUiPreferences = { ...DEFAULT_LOCAL_UI_PREFERENCES };

  public getPreferences(): LocalUiPreferences {
    return this.prefs;
  }

  public updatePreferences(
    patch: Partial<LocalUiPreferences> & Readonly<Record<string, unknown>>,
  ): LocalUiPreferences {
    for (const [k, v] of Object.entries(patch)) {
      assertZeroSecretContent(k, v);
    }
    this.prefs = {
      ...this.prefs,
      ...patch,
    };
    return this.prefs;
  }
}

/**
 * Category 5: Ephemeral View & Focus Store (Blueprint §13, §14, §20.5, G-06).
 * Keeps the main workspace mounted and `inert` while Overlay Settings or
 * Command Center is open, and tracks `focusReturnElementId` for deterministic
 * focus restoration on close.
 */
export interface EphemeralOverlayState {
  readonly settingsOpen: boolean;
  readonly activeSettingsSection: SettingsSectionId;
  readonly commandCenterOpen: boolean;
  readonly notificationDrawerOpen: boolean;
  readonly onboardingModalOpen: boolean;
  readonly migrationGuideModalOpen: boolean;
  readonly workspaceInert: boolean;
  readonly focusReturnElementId: string | null;
}

export class EphemeralUiStore {
  private state: EphemeralOverlayState = {
    settingsOpen: false,
    activeSettingsSection: "general",
    commandCenterOpen: false,
    notificationDrawerOpen: false,
    onboardingModalOpen: false,
    migrationGuideModalOpen: false,
    workspaceInert: false,
    focusReturnElementId: null,
  };

  public getState(): EphemeralOverlayState {
    return this.state;
  }

  private recomputeInert(next: Omit<EphemeralOverlayState, "workspaceInert">): EphemeralOverlayState {
    const workspaceInert =
      next.settingsOpen ||
      next.commandCenterOpen ||
      next.onboardingModalOpen ||
      next.migrationGuideModalOpen;
    return {
      ...next,
      workspaceInert,
    };
  }

  public openSettings(
    section: SettingsSectionId = "general",
    invokingElementId: string | null = null,
  ): EphemeralOverlayState {
    this.state = this.recomputeInert({
      ...this.state,
      settingsOpen: true,
      activeSettingsSection: section,
      focusReturnElementId: invokingElementId ?? this.state.focusReturnElementId,
    });
    return this.state;
  }

  public closeSettings(): {
    readonly state: EphemeralOverlayState;
    readonly restoreFocusTo: string | null;
  } {
    const restoreFocusTo = this.state.focusReturnElementId;
    this.state = this.recomputeInert({
      ...this.state,
      settingsOpen: false,
      focusReturnElementId: null,
    });
    return { state: this.state, restoreFocusTo };
  }

  public openCommandCenter(
    invokingElementId: string | null = null,
  ): EphemeralOverlayState {
    this.state = this.recomputeInert({
      ...this.state,
      commandCenterOpen: true,
      focusReturnElementId: invokingElementId ?? this.state.focusReturnElementId,
    });
    return this.state;
  }

  public closeCommandCenter(): {
    readonly state: EphemeralOverlayState;
    readonly restoreFocusTo: string | null;
  } {
    const restoreFocusTo = this.state.focusReturnElementId;
    this.state = this.recomputeInert({
      ...this.state,
      commandCenterOpen: false,
      focusReturnElementId: null,
    });
    return { state: this.state, restoreFocusTo };
  }
}
