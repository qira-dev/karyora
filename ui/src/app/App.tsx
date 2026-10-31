import { createBrowserRouter, createRoutesFromElements, RouterProvider } from "react-router-dom";

import { appRouteElements } from "./routes";
import { AppProviders } from "./providers";

/**
 * The application root (Blueprint §31.6 app/). One app for all targets
 * (UI-03): the web shell runs the same React/TypeScript app the desktop
 * candidate will embed; the desktop transport adapter is a later, gated
 * contract (O-4) and nothing here assumes it.
 *
 * P2.7: the router is a DATA router (createBrowserRouter over the frozen
 * route elements). The data router is what makes the J-8 dirty-form
 * navigation guard (useBlocker) available to the wizard and the
 * connection surfaces; in a declarative <BrowserRouter> the blocker hook
 * is unavailable. In-app behavior is unchanged: the same routes, the
 * same components, the same resolution of unknown URLs to `/`.
 */
const router = createBrowserRouter(createRoutesFromElements(appRouteElements));

export function App() {
  return (
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  );
}
