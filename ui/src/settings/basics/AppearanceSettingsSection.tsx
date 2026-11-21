/**
 * Settings Section 2: Appearance (UIR-8.1, Screenshots #5–#9, Blueprint §13.1 (2), E2E-21).
 *
 * App theme (`System`/`Dark`/`Light`), UI font size (`14px`), 10 Light & 10 Dark code themes,
 * Line numbers, Wrap lines, Code font size (`12px`), and live side-by-side Light Preview &
 * Dark Preview cards with `Active` badge.
 */

import { useState, type ReactElement } from "react";
import { Badge, Input, Select, Toggle } from "../../components/primitives";
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
        {isActive && <Badge tone="success" label="Active" />}
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
      <div className="ky-settings-card">
        <h3 className="ky-settings-card__title">Theme & Typography</h3>
        <div className="ky-settings-grid-2">
          <Select
            label="App theme"
            value={appTheme}
            options={[
              { value: "dark", label: "Dark" },
              { value: "light", label: "Light" },
              { value: "system", label: "System" },
            ]}
            onChange={(e) => onChangeAppTheme(e.target.value as AppTheme)}
          />
          <Input
            label="UI font size (px)"
            type="number"
            min={12}
            max={20}
            value={uiFontSize}
            onChange={(e) => setUiFontSize(e.target.value)}
          />
          <Select
            label="Light code theme (10 themes)"
            value={lightCodeTheme}
            options={LIGHT_CODE_THEMES.map((t) => ({ value: t, label: t }))}
            onChange={(e) => onChangeLightCodeTheme(e.target.value as LightCodeTheme)}
          />
          <Select
            label="Dark code theme (10 themes)"
            value={darkCodeTheme}
            options={DARK_CODE_THEMES.map((t) => ({ value: t, label: t }))}
            onChange={(e) => onChangeDarkCodeTheme(e.target.value as DarkCodeTheme)}
          />
          <Input
            label="Code font size (px)"
            type="number"
            min={10}
            max={20}
            value={codeFontSize}
            onChange={(e) => setCodeFontSize(e.target.value)}
          />
        </div>
        <Toggle label="Show line numbers" checked={showLineNumbers} onChange={setShowLineNumbers} />
        <Toggle label="Wrap long code lines" checked={wrapLines} onChange={setWrapLines} />
      </div>

      <div className="ky-settings-grid-2">
        {renderPreview("Light preview", lightPalette, appTheme === "light")}
        {renderPreview("Dark preview", darkPalette, appTheme !== "light")}
      </div>
    </div>
  );
}
