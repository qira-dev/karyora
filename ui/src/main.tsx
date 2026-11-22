import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { I18nextProvider } from "react-i18next";

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
import "./styles/responsive-reflow.css";

const i18nInstance = createI18n();

// Apply dark class synchronously before first paint unless light theme was explicitly saved
if (typeof document !== "undefined") {
  try {
    const savedTheme = window.localStorage.getItem("karyora:ui:theme");
    const isLight =
      savedTheme === "light" ||
      (savedTheme === "system" &&
        window.matchMedia?.("(prefers-color-scheme: light)").matches);
    document.documentElement.classList.toggle("dark", !isLight);
    document.documentElement.dataset.theme = isLight ? "light" : "dark";
  } catch {
    document.documentElement.classList.add("dark");
    document.documentElement.dataset.theme = "dark";
  }
}

const container = document.getElementById("root");
if (!container) {
  throw new Error("Root container missing");
}

createRoot(container).render(
  <StrictMode>
    <I18nextProvider i18n={i18nInstance}>
      <App />
    </I18nextProvider>
  </StrictMode>,
);
