import { useEffect, useState } from "react";

import { usePreferences } from "./preferences";

/**
 * Applies the preference state to the document (theme tokens, motion
 * tokens, document language) and keeps the System theme live-following
 * the OS appearance (P2.2 §3.4: System resolves to the active OS
 * appearance and switches the rendered token set live when the OS
 * appearance changes).
 */
export function useApplyPreferences(): void {
  const { theme, motion, locale } = usePreferences();
  const systemDark = useSystemPrefersDark();

  // System is a third state of the SAME selector (UI-08): it renders the
  // OS-matched member of the same two token sets — no separate theme.
  const renderedTheme: "light" | "dark" =
    theme === "system" ? (systemDark ? "dark" : "light") : theme;

  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = renderedTheme;
    root.dataset.motion = motion;
    root.lang = locale;
  }, [renderedTheme, motion, locale]);
}

/** The OS appearance preference, live. */
function useSystemPrefersDark(): boolean {
  const [systemDark, setSystemDark] = useState<boolean>(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return false;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches;
  });

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return undefined;
    }
    const query = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = (event: MediaQueryListEvent) => setSystemDark(event.matches);
    query.addEventListener("change", listener);
    return () => query.removeEventListener("change", listener);
  }, []);

  return systemDark;
}
