import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { createI18n } from "./i18n";
import "./styles/tokens.css";
import "./styles/animations.css";
import "./styles/code-themes.css";
import "./styles/layout.css";
import "./shell/shell.css";
import "./workspace/workspace.css";
import "./side-pane/side-pane.css";
import "./automations/automations-marketplace.css";
import "./domains/enterprise-domains.css";
import "./settings/settings-hub.css";

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
