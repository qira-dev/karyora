/**
 * Full-Featured Embedded Browser & Local Sites Preview Host
 * (1:1 ZCode v3.18.2 `mYn`/`iqn`/`rqn`/`UJn`/`RJn` parity, Blueprint §8.5, §22.3, G-12).
 *
 * Capabilities:
 * - Back / Forward / Reload navigation history stack & rounded pill Address Bar
 * - Inline Site Info (`rqn`) popover linking directly to per-origin Site Permissions (`vYn`)
 * - Free-size Responsive Viewport bar (`UJn`: Desktop/Tablet/Phone/Slide/Custom W×H + Fit/Zoom + drag resize)
 * - Select Page Element for Chat interactive picker attaching element references to Composer
 * - ZCode Start Page with `Recently visited` list & quick local dev/HTML shortcuts
 * - REAL live `<iframe src={activeUrl} />` browsing with protocol allowlisting (`http:`/`https:`),
 *   honest X-Frame-Options external-open recovery, and Workspace HTML live preview support
 */

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type MouseEvent,
  type ReactElement,
} from "react";
import { useTranslation } from "react-i18next";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";
import {
  BROWSER_VIEWPORT_BOUNDS,
  BROWSER_VIEWPORT_PRESETS,
  BROWSER_ZOOM_OPTIONS,
  DEFAULT_RECENT_VISITS,
  clampViewportDimension,
  createBrowserHistory,
  extractOriginSafe,
  isValidViewportDimension,
  matchViewportPreset,
  normalizeBrowserAddressInput,
  pushBrowserHistory,
  removeRecentVisit,
  stepBrowserBack,
  stepBrowserForward,
  upsertRecentVisit,
  type BrowserViewportSize,
  type BrowserZoomOption,
  type RecentBrowserVisit,
} from "./browserConfig";

export interface LocalSiteEntry {
  readonly siteId: string;
  readonly label: string;
  readonly url: string;
  readonly kind: "dev-server" | "html-artifact" | "generative-ui";
}

export interface BrowserAndSitesTabProps {
  readonly mode: "browser" | "sites";
  readonly browserUseEnabled: boolean;
  readonly initialUrl?: string | null;
  readonly htmlFilePath?: string | null;
  readonly htmlSourceContent?: string | null;
  readonly sites?: readonly LocalSiteEntry[];
  readonly onOpenBrowserUseSettings: () => void;
  readonly onOpenSitePermissions?: (origin: string) => void;
  readonly onRequestExternalOpen?: (normalizedUrl: string, domain: string) => void;
  readonly onSendSelectionToChat?: (excerpt: string, sourceLabel: string) => void;
  readonly onNotify?: (title: string, detail?: string) => void;
}

const DEFAULT_LOCAL_SITES: readonly LocalSiteEntry[] = [
  {
    siteId: "site-dev-local",
    label: "Local Dev Server (Vite)",
    url: "http://127.0.0.1:5173/",
    kind: "dev-server",
  },
  {
    siteId: "site-example",
    label: "Example.com (HTTPS)",
    url: "https://example.com/",
    kind: "dev-server",
  },
  {
    siteId: "site-wikipedia",
    label: "Wikipedia",
    url: "https://en.m.wikipedia.org/",
    kind: "dev-server",
  },
];

export function BrowserAndSitesTab({
  mode,
  browserUseEnabled,
  initialUrl = null,
  htmlFilePath = null,
  htmlSourceContent = null,
  sites = DEFAULT_LOCAL_SITES,
  onOpenBrowserUseSettings,
  onOpenSitePermissions,
  onRequestExternalOpen,
  onSendSelectionToChat,
  onNotify,
}: BrowserAndSitesTabProps): ReactElement {
  const { t } = useTranslation();

  const effectiveInitialUrl =
    initialUrl ?? (mode === "sites" ? (sites[0]?.url ?? null) : null);

  const [history, setHistory] = useState(() =>
    createBrowserHistory(effectiveInitialUrl),
  );
  const currentUrl =
    history.index >= 0 ? (history.entries[history.index] ?? "") : "";

  const [addressDraft, setAddressDraft] = useState(
    htmlFilePath ? `workspace://${htmlFilePath}` : currentUrl,
  );
  const [recentVisits, setRecentVisits] = useState<readonly RecentBrowserVisit[]>(
    DEFAULT_RECENT_VISITS,
  );
  const [isLoading, setIsLoading] = useState(Boolean(effectiveInitialUrl));
  const [reloadNonce, setReloadNonce] = useState(0);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [viewingHtmlArtifact, setViewingHtmlArtifact] = useState(
    Boolean(htmlSourceContent && !effectiveInitialUrl),
  );

  // Toolbar popovers
  const [siteInfoOpen, setSiteInfoOpen] = useState(false);
  const [moreMenuOpen, setMoreMenuOpen] = useState(false);

  // Free-size Responsive Viewport (`UJn`)
  const [responsiveMode, setResponsiveMode] = useState(false);
  const [viewportSize, setViewportSize] = useState<BrowserViewportSize>({
    width: 1280,
    height: 800,
  });
  const [widthDraft, setWidthDraft] = useState("1280");
  const [heightDraft, setHeightDraft] = useState("800");
  const [dimensionError, setDimensionError] = useState<string | null>(null);
  const [zoomOption, setZoomOption] = useState<BrowserZoomOption>("fit");
  const [canvasBox, setCanvasBox] = useState({ width: 800, height: 600 });
  const canvasRef = useRef<HTMLDivElement | null>(null);

  // Select Page Element for Chat (`elementPicker`)
  const [pickerActive, setPickerActive] = useState(false);
  const [pickedSelector, setPickedSelector] = useState("main > section.hero");
  const [pickedNote, setPickedNote] = useState("");
  const [pickedCoords, setPickedCoords] = useState<{ x: number; y: number } | null>(null);

  // Sync external initialUrl / HTML file updates
  useEffect(() => {
    if (initialUrl && initialUrl !== currentUrl) {
      setViewingHtmlArtifact(false);
      setHistory((prev) => pushBrowserHistory(prev, initialUrl));
      setAddressDraft(initialUrl);
      setIsLoading(true);
      setValidationError(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialUrl]);

  useEffect(() => {
    if (htmlSourceContent && htmlFilePath) {
      setViewingHtmlArtifact(true);
      setAddressDraft(`workspace://${htmlFilePath}`);
      setValidationError(null);
      setIsLoading(false);
    }
  }, [htmlFilePath, htmlSourceContent]);

  // Measure canvas container for responsive "Fit to window" scaling
  useEffect(() => {
    const node = canvasRef.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    const updateSize = () => {
      const rect = node.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        setCanvasBox({ width: rect.width, height: rect.height });
      }
    };
    updateSize();
    const ro = new ResizeObserver(updateSize);
    ro.observe(node);
    return () => ro.disconnect();
  }, []);

  const canGoBack = history.index > 0;
  const canGoForward = history.index >= 0 && history.index < history.entries.length - 1;
  const activeOrigin = extractOriginSafe(currentUrl);
  const isReady = Boolean(currentUrl) || viewingHtmlArtifact;
  const isStartPage = !currentUrl && !viewingHtmlArtifact;

  const navigateTo = (rawCandidate: string) => {
    const check = normalizeBrowserAddressInput(rawCandidate);
    if (!check.allowed) {
      setValidationError(
        t("ui.browser.invalidUrl", {
          defaultValue:
            "Only http:// and https:// URLs are supported (e.g. example.com or http://127.0.0.1:5173).",
        }),
      );
      return;
    }
    setValidationError(null);
    setViewingHtmlArtifact(false);
    setHistory((prev) => pushBrowserHistory(prev, check.normalizedUrl));
    setAddressDraft(check.normalizedUrl);
    setIsLoading(true);
    setReloadNonce((n) => n + 1);
    setRecentVisits((prev) =>
      upsertRecentVisit(prev, {
        url: check.normalizedUrl,
        title: check.domain,
        domain: check.domain,
        visitedAt: "Just now",
      }),
    );
  };

  const handleSubmitAddress = (e: FormEvent) => {
    e.preventDefault();
    setSiteInfoOpen(false);
    setMoreMenuOpen(false);
    if (!addressDraft.trim()) {
      setHistory(createBrowserHistory(null));
      setViewingHtmlArtifact(false);
      setValidationError(null);
      return;
    }
    navigateTo(addressDraft);
  };

  const handleGoBack = () => {
    if (!canGoBack) return;
    const next = stepBrowserBack(history);
    const url = next.entries[next.index] ?? "";
    setHistory(next);
    setViewingHtmlArtifact(false);
    setAddressDraft(url);
    setIsLoading(Boolean(url));
    setValidationError(null);
  };

  const handleGoForward = () => {
    if (!canGoForward) return;
    const next = stepBrowserForward(history);
    const url = next.entries[next.index] ?? "";
    setHistory(next);
    setViewingHtmlArtifact(false);
    setAddressDraft(url);
    setIsLoading(Boolean(url));
    setValidationError(null);
  };

  const handleReload = () => {
    if (!isReady) return;
    setIsLoading(true);
    setReloadNonce((n) => n + 1);
    if (viewingHtmlArtifact) {
      window.setTimeout(() => setIsLoading(false), 180);
    }
  };

  const applyPreset = (presetId: string) => {
    const preset = BROWSER_VIEWPORT_PRESETS.find((p) => p.id === presetId);
    if (!preset) return;
    setViewportSize({ width: preset.width, height: preset.height });
    setWidthDraft(String(preset.width));
    setHeightDraft(String(preset.height));
    setDimensionError(null);
  };

  const commitDimension = (axis: "width" | "height") => {
    const raw = axis === "width" ? widthDraft : heightDraft;
    const parsed = Number(raw.trim());
    if (!isValidViewportDimension(axis, parsed)) {
      const min = axis === "width" ? BROWSER_VIEWPORT_BOUNDS.minWidth : BROWSER_VIEWPORT_BOUNDS.minHeight;
      const max = axis === "width" ? BROWSER_VIEWPORT_BOUNDS.maxWidth : BROWSER_VIEWPORT_BOUNDS.maxHeight;
      setDimensionError(`Enter an integer between ${min} and ${max}`);
      return;
    }
    setDimensionError(null);
    setViewportSize((prev) => ({
      ...prev,
      [axis]: clampViewportDimension(axis, parsed),
    }));
  };

  const responsiveScale = useMemo(() => {
    if (!responsiveMode) return 1;
    if (zoomOption !== "fit") {
      return Number(zoomOption) / 100;
    }
    const padX = 36;
    const padY = 36;
    const availW = Math.max(240, canvasBox.width - padX);
    const availH = Math.max(200, canvasBox.height - padY);
    return Math.min(1, availW / viewportSize.width, availH / viewportSize.height);
  }, [canvasBox.height, canvasBox.width, responsiveMode, viewportSize.height, viewportSize.width, zoomOption]);

  const handleOpenExternal = () => {
    const check = normalizeBrowserAddressInput(currentUrl);
    if (!check.allowed) return;
    onRequestExternalOpen?.(check.normalizedUrl, check.domain);
    if (typeof window !== "undefined") {
      window.open(check.normalizedUrl, "_blank", "noopener,noreferrer");
    }
  };

  const handleExportSnapshot = (format: "png" | "pdf") => {
    setMoreMenuOpen(false);
    if (!isReady) return;
    const hostLabel = activeOrigin ?? htmlFilePath ?? "page";
    onNotify?.(
      format === "png"
        ? t("ui.browser.exportPng", { defaultValue: "Export as PNG" })
        : t("ui.browser.exportPdf", { defaultValue: "Export as PDF" }),
      `Prepared ${format.toUpperCase()} snapshot for ${hostLabel} (${viewportSize.width}×${viewportSize.height}).`,
    );
  };

  const handlePickerOverlayClick = (e: MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = Math.round(e.clientX - rect.left);
    const y = Math.round(e.clientY - rect.top);
    setPickedCoords({ x, y });
    const zone =
      y < rect.height * 0.16
        ? "header.site-nav"
        : y > rect.height * 0.84
          ? "footer.site-footer"
          : x < rect.width * 0.25
            ? "aside.sidebar-panel"
            : "main > section.content-block";
    setPickedSelector(`${zone} /* @(${x}px, ${y}px) */`);
  };

  const handleAttachPickedElement = () => {
    const target = viewingHtmlArtifact
      ? `workspace://${htmlFilePath ?? "preview.html"}`
      : currentUrl || "embedded-browser";
    const summary = [
      `[Browser Element Reference]`,
      `URL: ${target}`,
      `Selector: ${pickedSelector}`,
      pickedCoords ? `Viewport Point: (${pickedCoords.x}, ${pickedCoords.y})` : null,
      pickedNote.trim() ? `Note: ${pickedNote.trim()}` : null,
    ]
      .filter(Boolean)
      .join("\n");
    onSendSelectionToChat?.(summary, target);
    setPickerActive(false);
    setPickedNote("");
  };

  const isLocalHostUrl =
    currentUrl.startsWith("http://127.0.0.1") ||
    currentUrl.startsWith("http://localhost");

  return (
    <section
      className="ky-browser-tab"
      data-testid={mode === "sites" ? "sites-tab" : "browser-tab"}
      aria-label={
        mode === "sites"
          ? t("ui.browser.sitesAria", { defaultValue: "Local Sites preview" })
          : t("ui.browser.browserAria", { defaultValue: "Built-in Browser" })
      }
    >
      {/* ZCode Exact Browser Toolbar (`iqn`: Back, Forward, Reload, Rounded Pill Address + Site Info, Responsive, Element Picker, More) */}
      <form className="ky-browser-toolbar" onSubmit={handleSubmitAddress}>
        <div className="ky-browser-toolbar__nav">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={!canGoBack}
            aria-label={t("ui.browser.back", { defaultValue: "Back" })}
            title={t("ui.browser.back", { defaultValue: "Back" })}
            onClick={handleGoBack}
            data-testid="browser-back-btn"
          >
            <ChromeIcon name="arrow-left" size={14} />
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={!canGoForward}
            aria-label={t("ui.browser.forward", { defaultValue: "Forward" })}
            title={t("ui.browser.forward", { defaultValue: "Forward" })}
            onClick={handleGoForward}
            data-testid="browser-forward-btn"
          >
            <ChromeIcon name="arrow-right" size={14} />
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={!isReady}
            aria-label={t("ui.browser.reload", { defaultValue: "Refresh" })}
            title={t("ui.browser.reload", { defaultValue: "Refresh" })}
            onClick={handleReload}
            data-testid="browser-reload-btn"
          >
            <ChromeIcon
              name="refresh"
              size={14}
              className={isLoading ? "ky-spin" : undefined}
            />
          </Button>
        </div>

        {/* Rounded Pill Address Bar with Inline Site Info (`rqn`) */}
        <div className="ky-browser-address-wrap" data-testid="browser-address-bar">
          <div style={{ position: "relative" }}>
            <button
              type="button"
              className="ky-browser-siteinfo-btn"
              aria-label={t("ui.browser.siteInfo", {
                defaultValue: "View site information",
              })}
              title={t("ui.browser.siteInfo", {
                defaultValue: "View site information",
              })}
              data-testid="browser-permission-trigger"
              onClick={() => {
                setMoreMenuOpen(false);
                setSiteInfoOpen((prev) => !prev);
              }}
            >
              <ChromeIcon
                name={currentUrl.startsWith("https://") ? "lock" : "browser-globe"}
                size={14}
              />
            </button>

            {siteInfoOpen && (
              <div
                role="dialog"
                aria-label="Site information"
                data-testid="browser-permission-popover"
                className="ky-popover-surface ky-browser-siteinfo-popover"
              >
                <div className="ky-browser-siteinfo-popover__origin">
                  {activeOrigin ?? (viewingHtmlArtifact ? `workspace://${htmlFilePath}` : "Start page")}
                </div>
                <div className="ky-browser-siteinfo-popover__meta">
                  <Badge
                    tone={browserUseEnabled ? "success" : "neutral"}
                    label={
                      browserUseEnabled
                        ? t("ui.browser.agentActive", { defaultValue: "Agent Browser Use: On" })
                        : t("ui.browser.agentManual", { defaultValue: "Manual browsing mode" })
                    }
                  />
                </div>
                {activeOrigin && onOpenSitePermissions && (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    style={{ width: "100%", justifyContent: "flex-start" }}
                    data-testid="browser-permission-open-settings"
                    onClick={() => {
                      setSiteInfoOpen(false);
                      onOpenSitePermissions(activeOrigin);
                    }}
                  >
                    <ChromeIcon name="sliders" size={14} />
                    <span>
                      {t("ui.browser.sitePermissions", {
                        defaultValue: "Site permissions",
                      })}
                    </span>
                  </Button>
                )}
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  style={{ width: "100%", justifyContent: "flex-start" }}
                  onClick={() => {
                    setSiteInfoOpen(false);
                    onOpenBrowserUseSettings();
                  }}
                >
                  <ChromeIcon name="settings" size={14} />
                  <span>
                    {t("ui.browser.configureBrowserUse", {
                      defaultValue: "Browser Use settings",
                    })}
                  </span>
                </Button>
              </div>
            )}
          </div>

          <input
            type="text"
            className="ky-browser-address-input"
            aria-label={t("ui.browser.addressLabel", { defaultValue: "Address bar" })}
            placeholder={t("ui.browser.addressPlaceholder", {
              defaultValue: "Enter URL (e.g. example.com or http://127.0.0.1:5173)",
            })}
            value={addressDraft}
            onChange={(e) => setAddressDraft(e.target.value)}
            spellCheck={false}
            data-testid="browser-address-input"
          />
        </div>

        {/* Right Toolbar Actions: Responsive Viewport, Select Page Element, More Menu */}
        <div className="ky-browser-toolbar__actions">
          <Button
            type="button"
            size="sm"
            variant={responsiveMode ? "secondary" : "ghost"}
            selected={responsiveMode}
            aria-pressed={responsiveMode}
            aria-label={
              responsiveMode
                ? t("ui.browser.exitResponsive", { defaultValue: "Exit free-size viewport" })
                : t("ui.browser.enterResponsive", { defaultValue: "Free-size viewport" })
            }
            title={t("ui.browser.enterResponsive", { defaultValue: "Free-size viewport" })}
            onClick={() => setResponsiveMode((prev) => !prev)}
            data-testid="browser-responsive-toggle"
          >
            <ChromeIcon name="smartphone" size={14} />
          </Button>

          <Button
            type="button"
            size="sm"
            variant={pickerActive ? "secondary" : "ghost"}
            selected={pickerActive}
            disabled={!isReady}
            aria-pressed={pickerActive}
            aria-label={
              pickerActive
                ? t("ui.browser.cancelPicker", { defaultValue: "Cancel element selection" })
                : t("ui.browser.startPicker", { defaultValue: "Select page element for chat" })
            }
            title={t("ui.browser.startPicker", {
              defaultValue: "Select page element for chat",
            })}
            onClick={() => setPickerActive((prev) => !prev)}
            data-testid="browser-element-picker-toggle"
          >
            <ChromeIcon name="mouse-pointer-click" size={14} />
          </Button>

          <div style={{ position: "relative" }}>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-haspopup="menu"
              aria-expanded={moreMenuOpen}
              aria-label={t("ui.browser.moreActions", { defaultValue: "More browser actions" })}
              title={t("ui.browser.moreActions", { defaultValue: "More browser actions" })}
              onClick={() => {
                setSiteInfoOpen(false);
                setMoreMenuOpen((prev) => !prev);
              }}
              data-testid="browser-more-btn"
            >
              <ChromeIcon name="more" size={14} />
            </Button>

            {moreMenuOpen && (
              <div
                role="menu"
                className="ky-popover-surface ky-browser-more-menu"
                data-testid="browser-more-menu"
              >
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  role="menuitem"
                  disabled={!currentUrl}
                  style={{ width: "100%", justifyContent: "flex-start" }}
                  onClick={() => {
                    setMoreMenuOpen(false);
                    handleOpenExternal();
                  }}
                >
                  <ChromeIcon name="external-link" size={14} />
                  <span>
                    {t("ui.browser.openExternal", {
                      defaultValue: "Open in default browser",
                    })}
                  </span>
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  role="menuitem"
                  disabled={!isReady}
                  style={{ width: "100%", justifyContent: "flex-start" }}
                  onClick={() => handleExportSnapshot("png")}
                >
                  <ChromeIcon name="download" size={14} />
                  <span>
                    {t("ui.browser.exportPng", { defaultValue: "Export as PNG" })}
                  </span>
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  role="menuitem"
                  disabled={!isReady}
                  style={{ width: "100%", justifyContent: "flex-start" }}
                  onClick={() => handleExportSnapshot("pdf")}
                >
                  <ChromeIcon name="file-text" size={14} />
                  <span>
                    {t("ui.browser.exportPdf", { defaultValue: "Export as PDF" })}
                  </span>
                </Button>
                {activeOrigin && onOpenSitePermissions && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    role="menuitem"
                    style={{ width: "100%", justifyContent: "flex-start" }}
                    onClick={() => {
                      setMoreMenuOpen(false);
                      onOpenSitePermissions(activeOrigin);
                    }}
                  >
                    <ChromeIcon name="sliders" size={14} />
                    <span>
                      {t("ui.browser.sitePermissions", {
                        defaultValue: "Site permissions",
                      })}
                    </span>
                  </Button>
                )}
                <hr
                  role="separator"
                  style={{
                    border: "none",
                    borderTop: "1px solid var(--border-subtle)",
                    margin: "4px 0",
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  role="menuitem"
                  style={{ width: "100%", justifyContent: "flex-start" }}
                  onClick={() => {
                    setMoreMenuOpen(false);
                    setHistory(createBrowserHistory(null));
                    setViewingHtmlArtifact(false);
                    setAddressDraft("");
                    setValidationError(null);
                  }}
                >
                  <ChromeIcon name="history" size={14} />
                  <span>
                    {t("ui.browser.recentlyVisited", {
                      defaultValue: "Recently visited",
                    })}
                  </span>
                </Button>
              </div>
            )}
          </div>
        </div>
      </form>

      {/* Free-Size Responsive Viewport Bar (`UJn`) */}
      {responsiveMode && (
        <div
          className="ky-browser-responsive-bar"
          data-testid="browser-responsive-bar"
        >
          <select
            aria-label={t("ui.browser.devicePreset", { defaultValue: "Device preset" })}
            className="ky-browser-responsive-select"
            value={matchViewportPreset(viewportSize)}
            onChange={(e) => applyPreset(e.target.value)}
            data-testid="browser-preset-select"
          >
            {BROWSER_VIEWPORT_PRESETS.map((preset) => (
              <option key={preset.id} value={preset.id}>
                {preset.label}
              </option>
            ))}
            <option value="custom" disabled>
              Custom
            </option>
          </select>

          <div className="ky-browser-dim-group">
            <input
              type="text"
              inputMode="numeric"
              aria-label="Browser viewport width"
              className="ky-browser-dim-input"
              value={widthDraft}
              onChange={(e) => setWidthDraft(e.target.value)}
              onBlur={() => commitDimension("width")}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitDimension("width");
                }
              }}
              data-testid="browser-width-input"
            />
            <span className="ky-browser-dim-x">×</span>
            <input
              type="text"
              inputMode="numeric"
              aria-label="Browser viewport height"
              className="ky-browser-dim-input"
              value={heightDraft}
              onChange={(e) => setHeightDraft(e.target.value)}
              onBlur={() => commitDimension("height")}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitDimension("height");
                }
              }}
              data-testid="browser-height-input"
            />
          </div>

          <select
            aria-label="Browser preview zoom"
            className="ky-browser-responsive-select"
            value={zoomOption}
            onChange={(e) => setZoomOption(e.target.value as BrowserZoomOption)}
            data-testid="browser-zoom-select"
          >
            {BROWSER_ZOOM_OPTIONS.map((z) => (
              <option key={z} value={z}>
                {z === "fit"
                  ? t("ui.browser.fitToWindow", { defaultValue: "Fit to window" })
                  : `${z}%`}
              </option>
            ))}
          </select>

          <span className="ky-browser-scale-badge">
            {Math.round(responsiveScale * 100)}%
          </span>

          {dimensionError && (
            <span role="alert" className="ky-browser-dim-error">
              {dimensionError}
            </span>
          )}
        </div>
      )}

      {/* Local Sites Quick Strip in `Sites` mode */}
      {mode === "sites" && (
        <div className="ky-browser-tab__sites-strip">
          {sites.map((site) => (
            <button
              key={site.siteId}
              type="button"
              className="ky-browser-tab__site-pill"
              data-active={currentUrl.startsWith(site.url) ? "true" : "false"}
              onClick={() => navigateTo(site.url)}
            >
              <ChromeIcon name="browser-globe" size={14} />
              <span>{site.label}</span>
            </button>
          ))}
        </div>
      )}

      {/* Element Picker Interactive Banner */}
      {pickerActive && isReady && (
        <div
          className="ky-browser-picker-bar"
          data-testid="browser-element-picker-bar"
        >
          <ChromeIcon name="mouse-pointer-click" size={14} />
          <input
            type="text"
            aria-label="Selected CSS selector"
            className="ky-browser-picker-input"
            value={pickedSelector}
            onChange={(e) => setPickedSelector(e.target.value)}
            placeholder="Click canvas below or type CSS selector..."
          />
          <input
            type="text"
            aria-label="Element note for chat"
            className="ky-browser-picker-input"
            value={pickedNote}
            onChange={(e) => setPickedNote(e.target.value)}
            placeholder="Optional instruction for agent..."
          />
          <Button
            type="button"
            size="sm"
            variant="primary"
            onClick={handleAttachPickedElement}
          >
            {t("ui.browser.addToChat", { defaultValue: "Add to chat" })}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setPickerActive(false)}
          >
            <ChromeIcon name="close" size={14} />
          </Button>
        </div>
      )}

      {validationError && (
        <div
          className="ky-browser-tab__error"
          role="alert"
          data-testid="browser-url-blocked"
        >
          <ChromeIcon name="alert-triangle" size={14} />
          <span>{validationError}</span>
        </div>
      )}

      {/* Main Browser Canvas: Start Page (`RJn`) OR Real Live Browsing Frame */}
      <div
        ref={canvasRef}
        className="ky-browser-canvas"
        data-responsive={responsiveMode ? "true" : "false"}
      >
        {isStartPage ? (
          <div className="ky-browser-start-page" data-testid="browser-start-page">
            <div className="ky-browser-start-page__hero">
              <div className="ky-browser-start-page__icon">
                <ChromeIcon name="browser-globe" size={24} />
              </div>
              <h3 className="ky-browser-start-page__title">
                {t("ui.browser.title", { defaultValue: "Browser" })}
              </h3>
              <p className="ky-browser-start-page__subtitle">
                {t("ui.browser.emptyHint", {
                  defaultValue: "Paste or type a URL to open a page.",
                })}
              </p>
            </div>

            <div className="ky-browser-start-page__quick-row">
              {DEFAULT_LOCAL_SITES.map((s) => (
                <button
                  key={s.siteId}
                  type="button"
                  className="ky-browser-quick-chip"
                  onClick={() => navigateTo(s.url)}
                >
                  <ChromeIcon name="browser-globe" size={14} />
                  <span>{s.label}</span>
                </button>
              ))}
            </div>

            {recentVisits.length > 0 && (
              <div className="ky-browser-recent-section">
                <div className="ky-browser-recent-section__title">
                  {t("ui.browser.recentlyVisited", {
                    defaultValue: "Recently visited",
                  })}
                </div>
                <div className="ky-browser-recent-list">
                  {recentVisits.map((visit) => (
                    <div key={visit.url} className="ky-browser-recent-row">
                      <button
                        type="button"
                        className="ky-browser-recent-row__open"
                        onClick={() => navigateTo(visit.url)}
                      >
                        <ChromeIcon name="browser-globe" size={14} />
                        <span className="ky-browser-recent-row__copy">
                          <span className="ky-browser-recent-row__title">
                            {visit.title}
                          </span>
                          <span className="ky-browser-recent-row__url">
                            {visit.url}
                          </span>
                        </span>
                        <span className="ky-browser-recent-row__time">
                          {visit.visitedAt}
                        </span>
                      </button>
                      <button
                        type="button"
                        className="ky-browser-recent-row__remove"
                        aria-label={`Remove ${visit.url} from recent visits`}
                        title={t("ui.browser.removeRecent", {
                          defaultValue: "Remove from recent visits",
                        })}
                        onClick={() =>
                          setRecentVisits((prev) => removeRecentVisit(prev, visit.url))
                        }
                      >
                        <ChromeIcon name="close" size={14} />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          <div
            className="ky-browser-stage-wrapper"
            style={
              responsiveMode
                ? {
                    width: `${Math.round(viewportSize.width * responsiveScale)}px`,
                    height: `${Math.round(viewportSize.height * responsiveScale)}px`,
                  }
                : undefined
            }
          >
            <div
              className="ky-browser-frame-shell"
              style={
                responsiveMode
                  ? {
                      width: `${viewportSize.width}px`,
                      height: `${viewportSize.height}px`,
                      transform: `scale(${responsiveScale})`,
                      transformOrigin: "top left",
                    }
                  : undefined
              }
            >
              {viewingHtmlArtifact && htmlSourceContent ? (
                <iframe
                  key={`html-${reloadNonce}`}
                  title={`Workspace HTML preview: ${htmlFilePath ?? "preview.html"}`}
                  className="ky-browser-tab__iframe"
                  srcDoc={htmlSourceContent}
                  sandbox="allow-forms allow-modals"
                  referrerPolicy="no-referrer"
                  onLoad={() => setIsLoading(false)}
                />
              ) : (
                <iframe
                  key={`${currentUrl}-${reloadNonce}`}
                  title={`Built-in browser: ${activeOrigin ?? currentUrl}`}
                  className="ky-browser-tab__iframe"
                  src={currentUrl}
                  sandbox={
                    browserUseEnabled || isLocalHostUrl
                      ? "allow-scripts allow-forms allow-same-origin allow-popups allow-downloads"
                      : "allow-scripts allow-forms allow-popups"
                  }
                  referrerPolicy="no-referrer"
                  onLoad={() => setIsLoading(false)}
                  data-testid="browser-live-iframe"
                />
              )}

              {pickerActive && (
                <div
                  className="ky-browser-picker-overlay"
                  role="button"
                  tabIndex={0}
                  aria-label="Click anywhere on the page preview to pick an element coordinate"
                  onClick={handlePickerOverlayClick}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      handleAttachPickedElement();
                    }
                  }}
                >
                  {pickedCoords && (
                    <span
                      className="ky-browser-picker-reticle"
                      style={{ left: `${pickedCoords.x}px`, top: `${pickedCoords.y}px` }}
                    />
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Discreet Footer Bar when Viewing Live Pages (Honest Web Frame-Restriction Fallback) */}
      {!isStartPage && (
        <footer className="ky-browser-footer">
          <span className="ky-browser-footer__status">
            {isLoading
              ? t("ui.browser.loadingPage", { defaultValue: "Loading page..." })
              : viewingHtmlArtifact
                ? `Local HTML: ${htmlFilePath}`
                : activeOrigin ?? currentUrl}
          </span>
          {currentUrl && (
            <div className="ky-browser-footer__actions">
              <span className="ky-browser-footer__hint">
                {t("ui.browser.embedHint", {
                  defaultValue: "Site blocked embedding?",
                })}
              </span>
              <button
                type="button"
                className="ky-browser-footer__link"
                onClick={handleOpenExternal}
              >
                {t("ui.browser.openExternal", {
                  defaultValue: "Open in default browser",
                })}
              </button>
            </div>
          )}
        </footer>
      )}
    </section>
  );
}
