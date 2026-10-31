import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { DEFAULT_LOCALE, type AcceptedLocale } from "../i18n";

/**
 * User preferences (Blueprint §31.3 lifetime 4): theme, motion, language,
 * and sidebar state are LOCAL per-user/device preferences. They persist
 * in browser localStorage under a namespaced key — never shared domain
 * state, never a secret (X-6: no secret ever reaches any storage). The
 * desktop-shell storage policy is a later boundary (P2.8); the web shell
 * uses localStorage only for these four preference values.
 */

const STORAGE_KEY = "karyora.ui.preferences.v1";

export type ThemePreference = "light" | "dark" | "system";
export type MotionPreference = "system" | "reduce" | "no-preference";

interface StoredPreferences {
  theme: ThemePreference;
  motion: MotionPreference;
  locale: AcceptedLocale;
  sidebarExpanded: boolean;
}

const DEFAULTS: StoredPreferences = {
  theme: "light", // Light is the first-run default (UI-08 LOCKED).
  motion: "system", // Follow system + override (C-13).
  locale: DEFAULT_LOCALE, // English source/default (UI-18 LOCKED).
  sidebarExpanded: true, // C-01 local preference; expanded default.
};

function loadStored(): StoredPreferences {
  if (typeof window === "undefined") return DEFAULTS;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS;
    const parsed: unknown = JSON.parse(raw);
    if (parsed === null || typeof parsed !== "object") return DEFAULTS;
    const candidate = parsed as Partial<StoredPreferences>;
    return {
      theme: isTheme(candidate.theme) ? candidate.theme : DEFAULTS.theme,
      motion: isMotion(candidate.motion) ? candidate.motion : DEFAULTS.motion,
      locale: isLocale(candidate.locale) ? candidate.locale : DEFAULTS.locale,
      sidebarExpanded:
        typeof candidate.sidebarExpanded === "boolean"
          ? candidate.sidebarExpanded
          : DEFAULTS.sidebarExpanded,
    };
  } catch {
    // Corrupt or unavailable storage: fall back to defaults, never fail.
    return DEFAULTS;
  }
}

function isTheme(value: unknown): value is ThemePreference {
  return value === "light" || value === "dark" || value === "system";
}
function isMotion(value: unknown): value is MotionPreference {
  return value === "system" || value === "reduce" || value === "no-preference";
}
function isLocale(value: unknown): value is AcceptedLocale {
  return typeof value === "string" && value.length > 0;
}

interface PreferencesContextValue {
  readonly theme: ThemePreference;
  readonly motion: MotionPreference;
  readonly locale: AcceptedLocale;
  readonly sidebarExpanded: boolean;
  setTheme: (theme: ThemePreference) => void;
  setMotion: (motion: MotionPreference) => void;
  setLocale: (locale: AcceptedLocale) => void;
  setSidebarExpanded: (expanded: boolean) => void;
  resetToDefaults: () => void;
}

const PreferencesContext = createContext<PreferencesContextValue | undefined>(undefined);

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState<StoredPreferences>(loadStored);

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(preferences));
    } catch {
      // Storage may be unavailable (private mode); preferences still work
      // for the session. Nothing sensitive is stored, so failing silently
      // to in-memory state is the honest degradation.
    }
  }, [preferences]);

  const setTheme = useCallback((theme: ThemePreference) => {
    setPreferences((prev) => ({ ...prev, theme }));
  }, []);
  const setMotion = useCallback((motion: MotionPreference) => {
    setPreferences((prev) => ({ ...prev, motion }));
  }, []);
  const setLocale = useCallback((locale: AcceptedLocale) => {
    setPreferences((prev) => ({ ...prev, locale }));
  }, []);
  const setSidebarExpanded = useCallback((sidebarExpanded: boolean) => {
    setPreferences((prev) => ({ ...prev, sidebarExpanded }));
  }, []);
  const resetToDefaults = useCallback(() => {
    setPreferences({ ...DEFAULTS });
  }, []);

  const value = useMemo<PreferencesContextValue>(
    () => ({
      ...preferences,
      setTheme,
      setMotion,
      setLocale,
      setSidebarExpanded,
      resetToDefaults,
    }),
    [preferences, setTheme, setMotion, setLocale, setSidebarExpanded, resetToDefaults],
  );

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>;
}

export function usePreferences(): PreferencesContextValue {
  const context = useContext(PreferencesContext);
  if (!context) throw new Error("usePreferences must be used inside PreferencesProvider");
  return context;
}
