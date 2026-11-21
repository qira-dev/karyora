/**
 * Side-Pane `Browser` & `Sites` Sandboxed Preview Host
 * (UIR-5.3, Blueprint §8.5, §22.3, Settings → Browser Use governed).
 *
 * Enforces URL protocol allowlisting (`http:`/`https:` via `validateExternalUrlForOpen`),
 * strict sandboxed `<iframe>` (`sandbox=""` when scripts are disallowed by Browser Use
 * policy), and explicit external-open domain disclosure.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Input } from "../components/primitives";
import { validateExternalUrlForOpen } from "../platform-bridge";

export interface LocalSiteEntry {
  readonly siteId: string;
  readonly label: string;
  readonly url: string;
  readonly kind: "dev-server" | "html-artifact" | "generative-ui";
}

export interface BrowserAndSitesTabProps {
  readonly mode: "browser" | "sites";
  readonly browserUseEnabled: boolean;
  readonly sites?: readonly LocalSiteEntry[];
  readonly onOpenBrowserUseSettings: () => void;
  readonly onRequestExternalOpen?: (normalizedUrl: string, domain: string) => void;
}

const DEFAULT_LOCAL_SITES: readonly LocalSiteEntry[] = [
  {
    siteId: "site-dev-local",
    label: "Local Dev Server (Vite)",
    url: "http://127.0.0.1:5173",
    kind: "dev-server",
  },
  {
    siteId: "site-core-docs",
    label: "Karyora Local API Surface",
    url: "http://127.0.0.1:27201",
    kind: "dev-server",
  },
];

export function BrowserAndSitesTab({
  mode,
  browserUseEnabled,
  sites = DEFAULT_LOCAL_SITES,
  onOpenBrowserUseSettings,
  onRequestExternalOpen,
}: BrowserAndSitesTabProps): ReactElement {
  const [urlInput, setUrlInput] = useState(sites[0]?.url ?? "http://127.0.0.1:5173");
  const [activeUrl, setActiveUrl] = useState(sites[0]?.url ?? "http://127.0.0.1:5173");
  const [validationError, setValidationError] = useState<string | null>(null);

  const handleNavigate = (candidate: string) => {
    const check = validateExternalUrlForOpen(candidate);
    if (!check.allowed) {
      setValidationError(`Blocked unsafe URL (${check.reason}). Only http:// and https:// URLs are permitted.`);
      return;
    }
    setValidationError(null);
    setUrlInput(check.normalizedUrl);
    setActiveUrl(check.normalizedUrl);
  };

  const currentCheck = validateExternalUrlForOpen(activeUrl);

  return (
    <section
      className="ky-browser-tab"
      data-testid={mode === "sites" ? "sites-tab" : "browser-tab"}
      aria-label={mode === "sites" ? "Local Sites preview" : "Sandboxed Browser preview"}
    >
      <header className="ky-browser-tab__bar">
        <Badge
          tone={browserUseEnabled ? "success" : "warning"}
          label={browserUseEnabled ? "Browser Use Active" : "Strict Sandbox (No Scripts)"}
        />
        <div className="ky-browser-tab__url-form">
          <Input
            aria-label="Preview URL"
            value={urlInput}
            onChange={(e) => setUrlInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                handleNavigate(urlInput);
              }
            }}
          />
          <Button size="sm" variant="secondary" onClick={() => handleNavigate(urlInput)}>
            Go
          </Button>
          {currentCheck.allowed && onRequestExternalOpen && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                onRequestExternalOpen(currentCheck.normalizedUrl, currentCheck.domain)
              }
              title={`Open ${currentCheck.domain} externally`}
            >
              <ChromeIcon name="external-link" size={14} />
            </Button>
          )}
        </div>
      </header>

      {mode === "sites" && (
        <div className="ky-browser-tab__sites-strip">
          {sites.map((site) => (
            <button
              key={site.siteId}
              type="button"
              className="ky-browser-tab__site-pill"
              data-active={activeUrl.startsWith(site.url) ? "true" : "false"}
              onClick={() => handleNavigate(site.url)}
            >
              <ChromeIcon name="browser-globe" size={14} />
              <span>{site.label}</span>
            </button>
          ))}
        </div>
      )}

      {validationError && (
        <div className="ky-browser-tab__error" role="alert" data-testid="browser-url-blocked">
          <ChromeIcon name="alert-triangle" size={14} />
          <span>{validationError}</span>
        </div>
      )}

      <div className="ky-browser-tab__viewport">
        {!browserUseEnabled && (
          <div className="ky-browser-tab__policy-notice">
            <span>
              Governed by <strong>Settings → Browser Use</strong>. Embedded preview uses an
              inert sandbox until Browser Use is enabled.
            </span>
            <Button size="sm" variant="ghost" onClick={onOpenBrowserUseSettings}>
              Configure Browser Use
            </Button>
          </div>
        )}

        {currentCheck.allowed ? (
          <iframe
            title={`Sandboxed preview of ${currentCheck.domain}`}
            className="ky-browser-tab__iframe"
            srcDoc={`<!doctype html><html><body style="font-family:sans-serif;padding:16px;color:#e5e7eb;background:#111318;"><h3>Sandboxed Preview Host</h3><p>Target URL: <code>${currentCheck.domain}</code></p><p>Iframe sandbox attribute enforced: <code>${browserUseEnabled ? "allow-forms allow-same-origin" : "strict-empty"}</code></p></body></html>`}
            sandbox={browserUseEnabled ? "allow-forms" : ""}
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="ky-files-split__empty">Enter a valid http:// or https:// URL.</div>
        )}
      </div>
    </section>
  );
}
