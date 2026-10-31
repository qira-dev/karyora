import type { ReactNode } from "react";

import { PreferencesProvider } from "./preferences";
import { useApplyPreferences } from "./useApplyPreferences";

/**
 * App-level providers. The CoreClient layer is instantiated in the shell
 * surfaces that consume it (the unavailable instance in Phase 2 — see
 * src/coreclient/); no global store is introduced (§31.3: state stays
 * close to where it is used).
 */

function PreferenceEffects() {
  useApplyPreferences();
  return null;
}

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <PreferencesProvider>
      <PreferenceEffects />
      {children}
    </PreferencesProvider>
  );
}
