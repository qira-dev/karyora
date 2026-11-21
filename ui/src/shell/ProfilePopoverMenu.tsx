/**
 * Left Sidebar Bottom User Profile Popover & 4 Flyout Submenus
 * (UIR-3.3, Screenshots #61–#65, Blueprint §4.3 (6)).
 *
 * Implements:
 * - Avatar button + masked user/workspace ID + one-click Copy ID
 * - `Language ▸` (`System default` + all 13 locked locales including `bn` #62)
 * - `App theme ▸` (`Dark`, `Light`, `System` #63)
 * - `Interface mode ▸` (`Coding mode` vs `Office mode` #64)
 * - `Interface zoom ▸` (`100%`, `Zoom In Ctrl+=`, `Zoom Out Ctrl+-`, `Reset Zoom Ctrl+0` #65)
 */

import { useState, type ReactElement } from "react";
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
            width: "22px",
            height: "22px",
            borderRadius: "var(--radius-full)",
            backgroundColor: "var(--accent-primary)",
            color: "#ffffff",
            fontSize: "0.75rem",
            fontWeight: 600,
          }}
        >
          {initials}
        </span>
        <span style={{ fontSize: "0.8125rem", fontWeight: 500 }}>
          {userDisplayName}
        </span>
      </Button>

      {open && (
        <div
          role="menu"
          aria-label="User profile and quick appearance menu"
          data-testid="profile-popover-menu"
          className="ky-popover-surface"
          style={{
            bottom: "calc(100% + 8px)",
            insetInlineStart: 0,
            width: "256px",
            padding: "var(--space-2)",
            display: "flex",
            flexDirection: "column",
            gap: "4px",
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              setOpen(false);
            }
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              padding: "var(--space-2)",
              borderBottom: "1px solid var(--border-subtle)",
            }}
          >
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: "0.8125rem", fontWeight: 600 }}>
                {userDisplayName}
              </div>
              <div
                style={{
                  fontSize: "0.7rem",
                  color: "var(--text-muted)",
                  fontFamily: "var(--font-mono)",
                }}
              >
                ID: {maskedUserId}
              </div>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label="Copy User ID"
              onClick={() => {
                if (
                  typeof navigator !== "undefined" &&
                  typeof navigator.clipboard?.writeText === "function"
                ) {
                  void navigator.clipboard.writeText(maskedUserId);
                }
                setCopiedId(true);
              }}
            >
              <ChromeIcon name={copiedId ? "check" : "copy"} size={14} />
            </Button>
          </div>

          <Button
            variant="ghost"
            size="sm"
            selected={submenu === "language"}
            style={{ justifyContent: "space-between", width: "100%" }}
            onClick={() =>
              setSubmenu((prev) => (prev === "language" ? "none" : "language"))
            }
          >
            <span>Language ({activeLocale})</span>
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          {submenu === "language" && (
            <div
              data-testid="profile-submenu-language"
              style={{
                maxHeight: "180px",
                overflowY: "auto",
                paddingInlineStart: "var(--space-2)",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_LOCALE_OPTIONS.map((opt) => (
                <Button
                  key={opt.code}
                  variant="ghost"
                  size="sm"
                  selected={opt.code === activeLocale}
                  style={{ justifyContent: "flex-start", width: "100%" }}
                  onClick={() => {
                    onChangeLocale(opt.code === "system" ? "en" : opt.code);
                    setOpen(false);
                  }}
                >
                  {opt.label}
                </Button>
              ))}
            </div>
          )}

          <Button
            variant="ghost"
            size="sm"
            selected={submenu === "theme"}
            style={{ justifyContent: "space-between", width: "100%" }}
            onClick={() =>
              setSubmenu((prev) => (prev === "theme" ? "none" : "theme"))
            }
          >
            <span>App theme ({activeTheme})</span>
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          {submenu === "theme" && (
            <div
              data-testid="profile-submenu-theme"
              style={{
                paddingInlineStart: "var(--space-2)",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_THEME_OPTIONS.map((opt) => (
                <Button
                  key={opt.value}
                  variant="ghost"
                  size="sm"
                  selected={opt.value === activeTheme}
                  style={{ justifyContent: "flex-start", width: "100%" }}
                  onClick={() => {
                    onChangeTheme(opt.value);
                    setOpen(false);
                  }}
                >
                  {opt.label}
                </Button>
              ))}
            </div>
          )}

          <Button
            variant="ghost"
            size="sm"
            selected={submenu === "mode"}
            style={{ justifyContent: "space-between", width: "100%" }}
            onClick={() =>
              setSubmenu((prev) => (prev === "mode" ? "none" : "mode"))
            }
          >
            <span>Interface mode ({activeInterfaceMode})</span>
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          {submenu === "mode" && (
            <div
              data-testid="profile-submenu-mode"
              style={{
                paddingInlineStart: "var(--space-2)",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              {PROFILE_INTERFACE_MODE_OPTIONS.map((opt) => (
                <Button
                  key={opt.value}
                  variant="ghost"
                  size="sm"
                  selected={opt.value === activeInterfaceMode}
                  style={{ justifyContent: "flex-start", width: "100%" }}
                  onClick={() => {
                    onChangeInterfaceMode(opt.value);
                    setOpen(false);
                  }}
                >
                  {opt.label}
                </Button>
              ))}
            </div>
          )}

          <Button
            variant="ghost"
            size="sm"
            selected={submenu === "zoom"}
            style={{ justifyContent: "space-between", width: "100%" }}
            onClick={() =>
              setSubmenu((prev) => (prev === "zoom" ? "none" : "zoom"))
            }
          >
            <span>Interface zoom ({activeZoomPercent}%)</span>
            <ChromeIcon name="chevron-right" size={14} />
          </Button>
          {submenu === "zoom" && (
            <div
              data-testid="profile-submenu-zoom"
              style={{
                paddingInlineStart: "var(--space-2)",
                display: "flex",
                flexDirection: "column",
                gap: "2px",
              }}
            >
              <Button
                variant="ghost"
                size="sm"
                style={{ justifyContent: "space-between", width: "100%" }}
                onClick={() =>
                  onChangeZoomPercent(
                    clampInterfaceZoom(activeZoomPercent + INTERFACE_ZOOM_STEP),
                  )
                }
              >
                <span>Zoom In</span>
                <kbd>Ctrl+=</kbd>
              </Button>
              <Button
                variant="ghost"
                size="sm"
                style={{ justifyContent: "space-between", width: "100%" }}
                onClick={() =>
                  onChangeZoomPercent(
                    clampInterfaceZoom(activeZoomPercent - INTERFACE_ZOOM_STEP),
                  )
                }
              >
                <span>Zoom Out</span>
                <kbd>Ctrl+-</kbd>
              </Button>
              <Button
                variant="ghost"
                size="sm"
                style={{ justifyContent: "space-between", width: "100%" }}
                onClick={() => onChangeZoomPercent(100)}
              >
                <span>Reset Zoom (100%)</span>
                <kbd>Ctrl+0</kbd>
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
