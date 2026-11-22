/**
 * Left Sidebar Bottom User Profile Popover & 4 Flyout Submenus
 * (UIR-3.3, Screenshots #61–#65, Blueprint §4.3 (6)).
 */

import { useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Button } from "../components/primitives";
import type {
  AppTheme,
  InterfaceMode,
  SupportedLocale,
} from "../types/decisions";
import {
  clampInterfaceZoom,
  INTERFACE_ZOOM_STEP,
  PROFILE_INTERFACE_MODE_OPTIONS,
  PROFILE_LOCALE_OPTIONS,
  PROFILE_THEME_OPTIONS,
} from "./shellConfig";

export interface ProfilePopoverMenuProps {
  readonly userDisplayName: string;
  readonly maskedUserId: string;
  readonly activeLocale: SupportedLocale;
  readonly activeTheme: AppTheme;
  readonly activeInterfaceMode: InterfaceMode;
  readonly activeZoomPercent: number;
  readonly onChangeLocale: (locale: SupportedLocale) => void;
  readonly onChangeTheme: (theme: AppTheme) => void;
  readonly onChangeInterfaceMode: (mode: InterfaceMode) => void;
  readonly onChangeZoomPercent: (zoomPercent: number) => void;
}

type ActiveSubmenu = "none" | "language" | "theme" | "mode" | "zoom";

export function ProfilePopoverMenu({
  userDisplayName,
  maskedUserId,
  activeLocale,
  activeTheme,
  activeInterfaceMode,
  activeZoomPercent,
  onChangeLocale,
  onChangeTheme,
  onChangeInterfaceMode,
  onChangeZoomPercent,
}: ProfilePopoverMenuProps): ReactElement {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [submenu, setSubmenu] = useState<ActiveSubmenu>("none");
  const [copiedId, setCopiedId] = useState(false);

  const initials = userDisplayName.trim().slice(0, 1).toUpperCase() || "K";

  return (
    <div style={{ position: "relative" }}>
      <Button
        variant="ghost"
        size="sm"
        aria-haspopup="menu"
        aria-expanded={open ? "true" : "false"}
        data-testid="sidebar-profile-trigger"
        onClick={() => {
          setOpen((prev) => !prev);
          setSubmenu("none");
        }}
      >
        <span
          aria-hidden="true"
          style={{
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            width: "24px",
            height: "24px",
            borderRadius: "var(--radius-full)",
            backgroundColor: "#7c3aed",
            color: "#ffffff",
            fontSize: "0.75rem",
            fontWeight: 600,
          }}
        >
          {initials}
        </span>
        <span style={{ fontSize: "0.82rem", color: "var(--text-primary)", fontWeight: 500 }}>
          {userDisplayName}
        </span>
      </Button>

      {open && (
        <div
          role="menu"
          aria-label="User profile and preferences menu"
          data-testid="profile-popover-menu"
          className="ky-popover-surface"
          style={{
            bottom: "calc(100% + 8px)",
            insetInlineStart: 0,
            width: "248px",
            padding: "8px",
            display: "flex",
            flexDirection: "column",
            gap: "4px",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              padding: "6px 8px",
              borderBottom: "1px solid var(--border-subtle)",
              marginBottom: "4px",
            }}
          >
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: "0.82rem", fontWeight: 600 }}>
                {userDisplayName}
              </div>
              <div
                style={{
                  fontSize: "0.72rem",
                  color: "var(--text-muted)",
                  fontFamily: "var(--font-mono)",
                }}
              >
                {maskedUserId}
              </div>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label={t("ui.profile.copyId", { defaultValue: "Copy ID" })}
              onClick={() => {
                void navigator.clipboard?.writeText(maskedUserId);
                setCopiedId(true);
                window.setTimeout(() => setCopiedId(false), 1200);
              }}
            >
              <ChromeIcon name={copiedId ? "check" : "copy"} size={14} />
            </Button>
          </div>

          <button
            type="button"
            role="menuitem"
            className="ky-sidebar__nav-item"
            data-active={submenu === "language" ? "true" : "false"}
            onClick={() =>
              setSubmenu((prev) => (prev === "language" ? "none" : "language"))
            }
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="browser-globe" size={14} />
              <span>{t("ui.profile.language", { defaultValue: "Language" })}</span>
            </span>
            <ChromeIcon name="chevron-right" size={14} />
          </button>

          <button
            type="button"
            role="menuitem"
            className="ky-sidebar__nav-item"
            data-active={submenu === "theme" ? "true" : "false"}
            onClick={() =>
              setSubmenu((prev) => (prev === "theme" ? "none" : "theme"))
            }
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="appearance" size={14} />
              <span>{t("ui.profile.appTheme", { defaultValue: "App theme" })}</span>
            </span>
            <ChromeIcon name="chevron-right" size={14} />
          </button>

          <button
            type="button"
            role="menuitem"
            className="ky-sidebar__nav-item"
            data-active={submenu === "mode" ? "true" : "false"}
            onClick={() =>
              setSubmenu((prev) => (prev === "mode" ? "none" : "mode"))
            }
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="sliders" size={14} />
              <span>{t("ui.profile.interfaceMode", { defaultValue: "Interface mode" })}</span>
            </span>
            <ChromeIcon name="chevron-right" size={14} />
          </button>

          <button
            type="button"
            role="menuitem"
            className="ky-sidebar__nav-item"
            data-active={submenu === "zoom" ? "true" : "false"}
            onClick={() =>
              setSubmenu((prev) => (prev === "zoom" ? "none" : "zoom"))
            }
          >
            <span className="ky-sidebar__nav-left">
              <ChromeIcon name="maximize" size={14} />
              <span>
                {t("ui.profile.interfaceZoom", { defaultValue: "Interface zoom" })} ({activeZoomPercent}%)
              </span>
            </span>
            <ChromeIcon name="chevron-right" size={14} />
          </button>

          {submenu === "language" && (
            <div
              role="menu"
              aria-label="Language options"
              className="ky-popover-surface"
              style={{
                bottom: 0,
                insetInlineStart: "calc(100% + 6px)",
                width: "216px",
                maxHeight: "340px",
                overflowY: "auto",
                padding: "6px",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_LOCALE_OPTIONS.map((opt) => (
                <button
                  key={opt.code}
                  type="button"
                  role="menuitemradio"
                  aria-checked={opt.code === activeLocale ? "true" : "false"}
                  className="ky-sidebar__nav-item"
                  data-active={opt.code === activeLocale ? "true" : "false"}
                  onClick={() => {
                    const resolvedLocale: SupportedLocale =
                      opt.code === "system" ? "en" : opt.code;
                    onChangeLocale(resolvedLocale);
                    setOpen(false);
                  }}
                >
                  <span>{opt.label}</span>
                  {opt.code === activeLocale && <ChromeIcon name="check" size={14} />}
                </button>
              ))}
            </div>
          )}

          {submenu === "theme" && (
            <div
              role="menu"
              aria-label="App theme options"
              className="ky-popover-surface"
              style={{
                bottom: 0,
                insetInlineStart: "calc(100% + 6px)",
                width: "164px",
                padding: "6px",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_THEME_OPTIONS.map((opt) => {
                const labelKey =
                  opt.value === "dark"
                    ? "ui.profile.themeDark"
                    : opt.value === "light"
                      ? "ui.profile.themeLight"
                      : "ui.profile.themeSystem";
                return (
                  <button
                    key={opt.value}
                    type="button"
                    role="menuitemradio"
                    aria-checked={opt.value === activeTheme ? "true" : "false"}
                    className="ky-sidebar__nav-item"
                    data-active={opt.value === activeTheme ? "true" : "false"}
                    onClick={() => {
                      onChangeTheme(opt.value);
                      setOpen(false);
                    }}
                  >
                    <span>{t(labelKey, { defaultValue: opt.label })}</span>
                    {opt.value === activeTheme && <ChromeIcon name="check" size={14} />}
                  </button>
                );
              })}
            </div>
          )}

          {submenu === "mode" && (
            <div
              role="menu"
              aria-label="Interface mode options"
              className="ky-popover-surface"
              style={{
                bottom: 0,
                insetInlineStart: "calc(100% + 6px)",
                width: "216px",
                padding: "6px",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_INTERFACE_MODE_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  role="menuitemradio"
                  aria-checked={opt.value === activeInterfaceMode ? "true" : "false"}
                  className="ky-sidebar__nav-item"
                  data-active={opt.value === activeInterfaceMode ? "true" : "false"}
                  onClick={() => {
                    onChangeInterfaceMode(opt.value);
                    setOpen(false);
                  }}
                >
                  <span>
                    {t(
                      opt.value === "coding"
                        ? "ui.profile.codingMode"
                        : "ui.profile.officeMode",
                      { defaultValue: opt.label },
                    )}
                  </span>
                  {opt.value === activeInterfaceMode && (
                    <ChromeIcon name="check" size={14} />
                  )}
                </button>
              ))}
            </div>
          )}

          {submenu === "zoom" && (
            <div
              role="menu"
              aria-label="Interface zoom controls"
              className="ky-popover-surface"
              style={{
                bottom: 0,
                insetInlineStart: "calc(100% + 6px)",
                width: "190px",
                padding: "6px",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              <button
                type="button"
                role="menuitem"
                className="ky-sidebar__nav-item"
                onClick={() =>
                  onChangeZoomPercent(
                    clampInterfaceZoom(activeZoomPercent + INTERFACE_ZOOM_STEP),
                  )
                }
              >
                <span>Zoom In</span>
                <span className="ky-sidebar__shortcut-hint">Ctrl+=</span>
              </button>
              <button
                type="button"
                role="menuitem"
                className="ky-sidebar__nav-item"
                onClick={() =>
                  onChangeZoomPercent(
                    clampInterfaceZoom(activeZoomPercent - INTERFACE_ZOOM_STEP),
                  )
                }
              >
                <span>Zoom Out</span>
                <span className="ky-sidebar__shortcut-hint">Ctrl+-</span>
              </button>
              <button
                type="button"
                role="menuitem"
                className="ky-sidebar__nav-item"
                onClick={() => onChangeZoomPercent(100)}
              >
                <span>Reset Zoom</span>
                <span className="ky-sidebar__shortcut-hint">Ctrl+0</span>
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
