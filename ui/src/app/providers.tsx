import type { ReactNode } from "react";

import { CoreConnectionProvider } from "./coreConnectionState";
import { FirstRunProvider } from "./firstRun/firstRunState";
import { PreferencesProvider } from "./preferences";
import { useApplyPreferences } from "./useApplyPreferences";

/**
 * App-level providers. The CoreClient layer is instantiated in the shell
 * surfaces that consume it (the unavailable instance for the generic
 * query/command kinds whose Core surfaces do not exist yet — see
 * src/coreclient/; the IMPL-2 browser HTTP transport for the delivered
 * `internal/httplocal` surface lives in the CoreConnectionProvider and
 * src/coreclient/httpLocal/). No global store is introduced (§31.3:
 * state stays close to where it is used).
 */

function PreferenceEffects() {
  useApplyPreferences();
  return null;
}

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <PreferencesProvider>
      <CoreConnectionProvider>
        <FirstRunProvider>
          <PreferenceEffects />
          {children}
        </FirstRunProvider>
      </CoreConnectionProvider>
    </PreferencesProvider>
  );
}
