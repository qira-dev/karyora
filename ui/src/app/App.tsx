import { BrowserRouter } from "react-router-dom";

import { AppProviders } from "./providers";
import { AppRoutes } from "./routes";

/**
 * The application root (Blueprint §31.6 app/). One app for all targets
 * (UI-03): the web shell runs the same React/TypeScript app the desktop
 * candidate will embed; the desktop transport adapter is a later, gated
 * contract (O-4) and nothing here assumes it.
 */
export function App() {
  return (
    <AppProviders>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </AppProviders>
  );
}
