import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import { createI18n } from "./i18n";
import "./styles/tokens.css";
import "./styles/global.css";

// Localization first: every user-facing string resolves through the
// locale resources (G-10); English is the source and default.
createI18n();

const container = document.getElementById("root");
if (!container) {
  throw new Error("Root container missing");
}

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
