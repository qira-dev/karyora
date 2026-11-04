import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

/**
 * First-run setup state (C-06; the P2.7 milestone).
 *
 * What persists, and why it is safe to persist (X-6 binding everywhere):
 * - `status` ("pending" | "skipped") and `connectionChoice` ("local" |
 *   "remote" | null) are NON-SECRET local per-user/device preference
 *   values (P1.3 §3 lifetime 4, the same class as the sidebar state).
 *   They live in localStorage under their own namespaced key. There is
 *   deliberately NO "completed" value: setup completion is recorded ONLY
 *   after authoritative Core confirmation (C-06 scope/authority), and no
 *   Core is wired in this milestone — the UI can never truthfully reach
 *   it, so the stored state can never claim it.
 * - `providerDraft` is the §8.13 back-preservation draft for the provider
 *   step (non-secret fields only: preset, base URL, model ID, custom
 *   headers). It is IN-MEMORY session state (P1.3 §3 lifetime 3) — never
 *   persisted anywhere, never secrets. The API key itself never enters
 *   this store at all: it lives only in the provider form's own local
 *   state while that form is mounted, so unmounting the form is what
 *   discards it (X-6; C-07 "secrets never in any UI persistence").
 *
 * Re-running setup preserves existing state (C-06/C-14): the recorded
 * choice and the session draft survive re-entry; nothing is reset.
 */

const STORAGE_KEY = "karyora.ui.firstrun.v1";

export type FirstRunStatus = "pending" | "skipped";
export type ConnectionChoice = "local" | "remote";

/** Non-secret provider-step draft (C-07 UI-12 fields minus the secret).
 *  `apiFormat` is the Custom API branch's coded connection protocol (the
 *  API-format selector's value — the wire provider id a Custom API
 *  connection save sends; empty until the format is chosen). */
export interface ProviderDraft {
  presetId: string;
  apiFormat: string;
  baseUrl: string;
  modelId: string;
  headers: string;
}

export const EMPTY_PROVIDER_DRAFT: ProviderDraft = {
  presetId: "",
  apiFormat: "",
  baseUrl: "",
  modelId: "",
  headers: "",
};

interface StoredFirstRun {
  status: FirstRunStatus;
  connectionChoice: ConnectionChoice | null;
}

const DEFAULTS: StoredFirstRun = {
  status: "pending", // First use: the wizard is the first-use state of `/`.
  connectionChoice: null,
};

function loadStored(): StoredFirstRun {
  if (typeof window === "undefined") return DEFAULTS;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS;
    const parsed: unknown = JSON.parse(raw);
    if (parsed === null || typeof parsed !== "object") return DEFAULTS;
    const candidate = parsed as Partial<StoredFirstRun>;
    return {
      status: candidate.status === "skipped" ? "skipped" : DEFAULTS.status,
      connectionChoice:
        candidate.connectionChoice === "local" || candidate.connectionChoice === "remote"
          ? candidate.connectionChoice
          : DEFAULTS.connectionChoice,
    };
  } catch {
    // Corrupt or unavailable storage: fall back to defaults, never fail.
    return DEFAULTS;
  }
}

interface FirstRunContextValue {
  readonly status: FirstRunStatus;
  readonly connectionChoice: ConnectionChoice | null;
  readonly providerDraft: ProviderDraft;
  setConnectionChoice: (choice: ConnectionChoice) => void;
  skipSetup: () => void;
  resumeSetup: () => void;
  setProviderDraft: (draft: ProviderDraft) => void;
  discardProviderDraft: () => void;
}

const FirstRunContext = createContext<FirstRunContextValue | undefined>(undefined);

export function FirstRunProvider({ children }: { children: ReactNode }) {
  const [stored, setStored] = useState<StoredFirstRun>(loadStored);
  const [providerDraft, setProviderDraftState] = useState<ProviderDraft>(EMPTY_PROVIDER_DRAFT);

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
    } catch {
      // Storage may be unavailable (private mode); the state still works
      // for the session. Nothing sensitive is stored here.
    }
  }, [stored]);

  const setConnectionChoice = useCallback((choice: ConnectionChoice) => {
    setStored((prev) => ({ ...prev, connectionChoice: choice }));
  }, []);
  const skipSetup = useCallback(() => {
    setStored((prev) => ({ ...prev, status: "skipped" }));
  }, []);
  const resumeSetup = useCallback(() => {
    setStored((prev) => ({ ...prev, status: "pending" }));
  }, []);
  const setProviderDraft = useCallback((draft: ProviderDraft) => {
    setProviderDraftState(draft);
  }, []);
  const discardProviderDraft = useCallback(() => {
    setProviderDraftState(EMPTY_PROVIDER_DRAFT);
  }, []);

  const value = useMemo<FirstRunContextValue>(
    () => ({
      status: stored.status,
      connectionChoice: stored.connectionChoice,
      providerDraft,
      setConnectionChoice,
      skipSetup,
      resumeSetup,
      setProviderDraft,
      discardProviderDraft,
    }),
    [
      stored.status,
      stored.connectionChoice,
      providerDraft,
      setConnectionChoice,
      skipSetup,
      resumeSetup,
      setProviderDraft,
      discardProviderDraft,
    ],
  );

  return <FirstRunContext.Provider value={value}>{children}</FirstRunContext.Provider>;
}

export function useFirstRun(): FirstRunContextValue {
  const context = useContext(FirstRunContext);
  if (!context) throw new Error("useFirstRun must be used inside FirstRunProvider");
  return context;
}
