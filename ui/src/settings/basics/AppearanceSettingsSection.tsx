/**
 * Settings Section 2: Appearance — Calibrated 1:1 to ZCode Screenshots #5–#9.
 *
 * Horizontal grouped rows for App theme, UI font size, Code settings (10 Light &
 * 10 Dark code themes, Show line numbers, Wrap long lines, Code font size), and
 * side-by-side live Light & Dark Code preview cards.
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Badge, Toggle } from "../../components/primitives";
import { resolveCodeThemePalette } from "../../styles/codeThemeRegistry";
import {
  DARK_CODE_THEMES,
  LIGHT_CODE_THEMES,
  type AppTheme,
  type DarkCodeTheme,
  type LightCodeTheme,
} from "../../types/decisions";

export interface AppearanceSettingsSectionProps {
  readonly appTheme: AppTheme;
  readonly lightCodeTheme: LightCodeTheme;
  readonly darkCodeTheme: DarkCodeTheme;
  readonly onChangeAppTheme: (theme: AppTheme) => void;
  readonly onChangeLightCodeTheme: (theme: LightCodeTheme) => void;
  readonly onChangeDarkCodeTheme: (theme: DarkCodeTheme) => void;
}

const SAMPLE_SNIPPET = `func VerifyReceipt(digest string) bool {
  // Deterministic SHA-256 check
  return len(digest) == 71
}`;

export function AppearanceSettingsSection({
  appTheme,
  lightCodeTheme,
  darkCodeTheme,
  onChangeAppTheme,
  onChangeLightCodeTheme,
  onChangeDarkCodeTheme,
}: AppearanceSettingsSectionProps): ReactElement {
  const { t } = useTranslation();
  const [uiFontSize, setUiFontSize] = useState("14");
  const [codeFontSize, setCodeFontSize] = useState("12");
  const [showLineNumbers, setShowLineNumbers] = useState(true);
  const [wrapLines, setWrapLines] = useState(false);

  const lightPalette = resolveCodeThemePalette(lightCodeTheme);
  const darkPalette = resolveCodeThemePalette(darkCodeTheme);

  const renderPreview = (
    title: string,
    palette: ReturnType<typeof resolveCodeThemePalette>,
    isActive: boolean,
  ) => (
    <div
      className="ky-theme-preview-card"
      data-active={isActive ? "true" : "false"}
      data-testid={`appearance-preview-${palette.mode}`}
    >
      <div
        className="ky-theme-preview-card__head"
        style={{ backgroundColor: palette.lineHighlight, color: palette.foreground }}
      >
        <span>
          {title} — {palette.id}
        </span>
        {isActive && (
          <Badge
            tone="success"
            label={t("ui.settings.appearance.activeBadge", { defaultValue: "Active" })}
          />
        )}
      </div>
      <pre
        className="ky-theme-preview-card__code"
        style={{
          backgroundColor: palette.background,
          color: palette.foreground,
          fontSize: `${codeFontSize}px`,
          whiteSpace: wrapLines ? "pre-wrap" : "pre",
        }}
      >
        {SAMPLE_SNIPPET.split("\n").map((line, i) => (
          <div key={i}>
            {showLineNumbers && (
              <span style={{ color: palette.gutterForeground, marginInlineEnd: "12px" }}>
                {i + 1}
              </span>
            )}
            <span>{line}</span>
          </div>
        ))}
      </pre>
    </div>
  );

  return (
    <div className="ky-settings-content" data-testid="settings-section-appearance">
      <p className="ky-settings-section-sub">
        {t("ui.settings.appearance.subtitle", {
          defaultValue: "Choose the app theme and interface text size.",
        })}
      </p>

      {/* Group 1: App Theme & UI Font Size (#5 top card) */}
      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.appThemeTitle", { defaultValue: "App theme" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.appThemeDesc", {
                defaultValue: "Choose light, dark, or follow the system theme.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <select
              aria-label="App theme"
              className="ky-select"
              value={appTheme}
              onChange={(e) => onChangeAppTheme(e.target.value as AppTheme)}
            >
              <option value="dark">
                {t("ui.profile.themeDark", { defaultValue: "Dark" })}
              </option>
              <option value="light">
                {t("ui.profile.themeLight", { defaultValue: "Light" })}
              </option>
              <option value="system">
                {t("ui.profile.themeSystem", { defaultValue: "System" })}
              </option>
            </select>
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.uiFontSizeTitle", { defaultValue: "UI font size" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.uiFontSizeDesc", {
                defaultValue: "Adjust interface text without changing icons or layout dimensions.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <input
              type="number"
              className="ky-input"
              aria-label="UI font size (px)"
              min={12}
              max={20}
              style={{ width: "86px", textAlign: "end" }}
              value={uiFontSize}
              onChange={(e) => {
                setUiFontSize(e.target.value);
                const parsed = Number(e.target.value);
                if (Number.isFinite(parsed) && parsed >= 12 && parsed <= 20) {
                  document.documentElement.style.setProperty("--ui-font-size", `${parsed}px`);
                }
              }}
            />
            <span style={{ fontSize: "0.8rem", color: "var(--text-muted)" }}>px</span>
          </div>
        </div>
      </div>

      {/* Group 2: Code Settings (#5 middle section) */}
      <div>
        <h3 className="ky-settings-section-heading">
          {t("ui.settings.appearance.codeSettingsHeading", { defaultValue: "Code settings" })}
        </h3>
        <p className="ky-settings-section-sub">
          {t("ui.settings.appearance.codeSettingsDesc", {
            defaultValue:
              "Choose code themes, font size, and display options independently from the interface font size.",
          })}
        </p>
      </div>

      <div className="ky-setting-group">
        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.lightCodeThemeTitle", { defaultValue: "Light code theme" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.lightCodeThemeDesc", {
                defaultValue: "Highlighting theme used for code content in the light interface.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <select
              aria-label="Light code theme"
              className="ky-select"
              value={lightCodeTheme}
              onChange={(e) => onChangeLightCodeTheme(e.target.value as LightCodeTheme)}
            >
              {LIGHT_CODE_THEMES.map((theme) => (
                <option key={theme} value={theme}>
                  {theme}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.darkCodeThemeTitle", { defaultValue: "Dark code theme" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.darkCodeThemeDesc", {
                defaultValue: "Highlighting theme used for code content in the dark interface.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control">
            <select
              aria-label="Dark code theme"
              className="ky-select"
              value={darkCodeTheme}
              onChange={(e) => onChangeDarkCodeTheme(e.target.value as DarkCodeTheme)}
            >
              {DARK_CODE_THEMES.map((theme) => (
                <option key={theme} value={theme}>
                  {theme}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.lineNumbersTitle", { defaultValue: "Show line numbers" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.lineNumbersDesc", {
                defaultValue: "Display line numbers in code and diff views.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle
              label=""
              aria-label="Show line numbers"
              checked={showLineNumbers}
              onChange={setShowLineNumbers}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.wrapLinesTitle", { defaultValue: "Wrap long lines" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.wrapLinesDesc", {
                defaultValue: "Wrap long code lines automatically.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <Toggle
              label=""
              aria-label="Wrap long lines"
              checked={wrapLines}
              onChange={setWrapLines}
            />
          </div>
        </div>

        <div className="ky-setting-row">
          <div className="ky-setting-row__info">
            <h3 className="ky-setting-row__title">
              {t("ui.settings.appearance.codeFontSizeTitle", { defaultValue: "Code font size" })}
            </h3>
            <p className="ky-setting-row__desc">
              {t("ui.settings.appearance.codeFontSizeDesc", {
                defaultValue:
                  "Adjust the default font size for code blocks, file previews, and diff views.",
              })}
            </p>
          </div>
          <div className="ky-setting-row__control ky-setting-row__control--compact">
            <input
              type="number"
              className="ky-input"
              aria-label="Code font size (px)"
              min={10}
              max={18}
              style={{ width: "86px", textAlign: "end" }}
              value={codeFontSize}
              onChange={(e) => setCodeFontSize(e.target.value)}
            />
            <span style={{ fontSize: "0.8rem", color: "var(--text-muted)" }}>px</span>
          </div>
        </div>
      </div>

      {/* Group 3: Side-by-Side Live Code Previews (#5 bottom) */}
      <div>
        <h3 className="ky-settings-section-heading">
          {t("ui.settings.appearance.codePreviewHeading", { defaultValue: "Code preview" })}
        </h3>
        <p className="ky-settings-section-sub">
          {t("ui.settings.appearance.codePreviewDesc", {
            defaultValue:
              "Preview the light and dark code themes together. The theme used by the current interface is marked as active.",
          })}
        </p>
      </div>

      <div className="ky-settings-grid-2">
        {renderPreview(
          t("ui.settings.appearance.lightPreview", { defaultValue: "Light preview" }),
          lightPalette,
          appTheme === "light",
        )}
        {renderPreview(
          t("ui.settings.appearance.darkPreview", { defaultValue: "Dark preview" }),
          darkPalette,
          appTheme !== "light",
        )}
      </div>
    </div>
  );
}
