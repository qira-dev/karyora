/**
 * Embedded Browser, Free-Size Responsive Viewport & Per-Origin Site Permissions Config
 * (1:1 ZCode v3.18.2 `mYn`/`iqn`/`UJn`/`vYn`/`gYn` parity, Blueprint §8.5, §22.3, G-12).
 *
 * Pure TypeScript module (`import type` only) so both Vite React components and
 * Node's native ESM test runner (`node --test`) verify browser behavior directly.
 */

import type { ExternalUrlValidationResult } from "../platform-bridge/platformBridge";

function validateHttpOrHttpsUrl(rawUrl: string): ExternalUrlValidationResult {
  const trimmed = rawUrl.trim();
  if (!trimmed) {
    return { allowed: false, reason: "EMPTY_URL" };
  }
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return { allowed: false, reason: "INVALID_URL" };
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    return {
      allowed: false,
      reason: `DISALLOWED_PROTOCOL_${parsed.protocol.replace(":", "").toUpperCase()}`,
    };
  }
  if (!parsed.hostname) {
    return { allowed: false, reason: "MISSING_HOSTNAME" };
  }
  return {
    allowed: true,
    normalizedUrl: parsed.toString(),
    domain: parsed.hostname,
  };
}

export type BrowserViewportPresetId = "desktop" | "tablet" | "phone" | "slide" | "custom";

export interface BrowserViewportPreset {
  readonly id: Exclude<BrowserViewportPresetId, "custom">;
  readonly label: string;
  readonly width: number;
  readonly height: number;
}

export interface BrowserViewportSize {
  readonly width: number;
  readonly height: number;
}

export const BROWSER_VIEWPORT_BOUNDS = {
  minWidth: 280,
  maxWidth: 3840,
  minHeight: 240,
  maxHeight: 2160,
} as const;

export const BROWSER_VIEWPORT_PRESETS: readonly BrowserViewportPreset[] = [
  { id: "desktop", label: "Desktop", width: 1280, height: 800 },
  { id: "tablet", label: "Tablet", width: 768, height: 1024 },
  { id: "phone", label: "Phone", width: 393, height: 852 },
  { id: "slide", label: "Slide", width: 1280, height: 720 },
] as const;

export const BROWSER_ZOOM_OPTIONS = ["fit", "50", "75", "100", "125", "150"] as const;
export type BrowserZoomOption = (typeof BROWSER_ZOOM_OPTIONS)[number];

export function matchViewportPreset(size: BrowserViewportSize): BrowserViewportPresetId {
  const matched = BROWSER_VIEWPORT_PRESETS.find(
    (p) => p.width === size.width && p.height === size.height,
  );
  return matched ? matched.id : "custom";
}

export function clampViewportDimension(
  axis: "width" | "height",
  value: number,
): number {
  const min = axis === "width" ? BROWSER_VIEWPORT_BOUNDS.minWidth : BROWSER_VIEWPORT_BOUNDS.minHeight;
  const max = axis === "width" ? BROWSER_VIEWPORT_BOUNDS.maxWidth : BROWSER_VIEWPORT_BOUNDS.maxHeight;
  if (!Number.isFinite(value)) return min;
  return Math.min(max, Math.max(min, Math.round(value)));
}

export function isValidViewportDimension(
  axis: "width" | "height",
  value: number,
): boolean {
  if (!Number.isInteger(value)) return false;
  if (axis === "width") {
    return value >= BROWSER_VIEWPORT_BOUNDS.minWidth && value <= BROWSER_VIEWPORT_BOUNDS.maxWidth;
  }
  return value >= BROWSER_VIEWPORT_BOUNDS.minHeight && value <= BROWSER_VIEWPORT_BOUNDS.maxHeight;
}

/**
 * Normalizes human-typed browser address bar input (bare domains like `example.com`
 * or `localhost:5173`) while strictly rejecting unsafe URL schemes (`javascript:`,
 * `data:`, `vbscript:`, `file:`) through `validateExternalUrlForOpen`.
 */
export function normalizeBrowserAddressInput(
  rawInput: string,
): ExternalUrlValidationResult {
  const trimmed = rawInput.trim();
  if (!trimmed) {
    return { allowed: false, reason: "EMPTY_URL" };
  }

  // If the user explicitly supplied a scheme (`foo:...`), validate as-is (do NOT rewrite `javascript:`!)
  const hasExplicitScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed);
  const looksLikeLocalhostPort = /^(localhost|127\.0\.0\.1|\[::1\])(:\d+)?(\/.*)?$/i.test(
    trimmed,
  );

  let candidate = trimmed;
  if (looksLikeLocalhostPort) {
    candidate = `http://${trimmed}`;
  } else if (!hasExplicitScheme) {
    if (trimmed.includes(" ") || !trimmed.includes(".")) {
      return { allowed: false, reason: "INVALID_URL" };
    }
    candidate = `https://${trimmed}`;
  }

  return validateHttpOrHttpsUrl(candidate);
}

export function extractOriginSafe(rawUrl: string): string | null {
  try {
    const parsed = new URL(rawUrl);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return parsed.origin;
    }
  } catch {
    // ignore invalid URL
  }
  return null;
}

export interface BrowserHistoryState {
  readonly entries: readonly string[];
  readonly index: number;
}

export function createBrowserHistory(initialUrl?: string | null): BrowserHistoryState {
  if (initialUrl && initialUrl.trim().length > 0) {
    return { entries: [initialUrl], index: 0 };
  }
  return { entries: [], index: -1 };
}

export function pushBrowserHistory(
  state: BrowserHistoryState,
  nextUrl: string,
): BrowserHistoryState {
  const current = state.index >= 0 ? state.entries[state.index] : null;
  if (current === nextUrl) {
    return state;
  }
  const truncated = state.entries.slice(0, state.index + 1);
  const nextEntries = [...truncated, nextUrl].slice(-50);
  return {
    entries: nextEntries,
    index: nextEntries.length - 1,
  };
}

export function stepBrowserBack(state: BrowserHistoryState): BrowserHistoryState {
  if (state.index <= 0) return state;
  return { ...state, index: state.index - 1 };
}

export function stepBrowserForward(state: BrowserHistoryState): BrowserHistoryState {
  if (state.index < 0 || state.index >= state.entries.length - 1) return state;
  return { ...state, index: state.index + 1 };
}

export interface RecentBrowserVisit {
  readonly url: string;
  readonly title: string;
  readonly domain: string;
  readonly visitedAt: string;
}

export const DEFAULT_RECENT_VISITS: readonly RecentBrowserVisit[] = [
  {
    url: "http://127.0.0.1:5173/",
    title: "Karyora Local Workspace (Vite Dev)",
    domain: "127.0.0.1",
    visitedAt: "Just now",
  },
  {
    url: "https://example.com/",
    title: "Example Domain (Live HTTPS Test)",
    domain: "example.com",
    visitedAt: "Today",
  },
  {
    url: "https://developer.mozilla.org/",
    title: "MDN Web Docs",
    domain: "developer.mozilla.org",
    visitedAt: "Today",
  },
];

export function upsertRecentVisit(
  visits: readonly RecentBrowserVisit[],
  visit: RecentBrowserVisit,
  maxItems = 12,
): readonly RecentBrowserVisit[] {
  const filtered = visits.filter((v) => v.url !== visit.url);
  return [visit, ...filtered].slice(0, maxItems);
}

export function removeRecentVisit(
  visits: readonly RecentBrowserVisit[],
  url: string,
): readonly RecentBrowserVisit[] {
  return visits.filter((v) => v.url !== url);
}

/**
 * Exact ZCode v3.18.2 `gYn` Per-Origin Site Permissions Registry.
 */
export type SitePermissionDecision = "ask" | "allow" | "block";

export interface SitePermissionCapabilityDef {
  readonly key: string;
  readonly label: string;
  readonly description: string;
  readonly iconName: string;
}

export const SITE_PERMISSION_CAPABILITIES: readonly SitePermissionCapabilityDef[] = [
  {
    key: "media",
    label: "Camera and microphone",
    description: "Allow this site to request video or audio capture devices.",
    iconName: "mic",
  },
  {
    key: "clipboard-read",
    label: "Clipboard reading",
    description: "Read text or images copied to your system clipboard.",
    iconName: "copy",
  },
  {
    key: "geolocation",
    label: "Location",
    description: "Access approximate or precise geographic coordinates.",
    iconName: "browser-globe",
  },
  {
    key: "notifications",
    label: "Notifications",
    description: "Display alerts from this origin inside the desktop workspace.",
    iconName: "activity",
  },
  {
    key: "midi",
    label: "MIDI devices",
    description: "Communicate with connected MIDI instruments and controllers.",
    iconName: "sliders",
  },
  {
    key: "idle-detection",
    label: "Idle detection",
    description: "Detect when your keyboard or screen has been idle.",
    iconName: "automations",
  },
  {
    key: "speaker-selection",
    label: "Audio output selection",
    description: "Enumerate and route audio to connected speakers or headsets.",
    iconName: "sliders",
  },
  {
    key: "window-management",
    label: "Window management",
    description: "Inspect multi-display geometry and open positioned windows.",
    iconName: "computer-monitor",
  },
  {
    key: "storage-access",
    label: "Storage access",
    description: "Request partitioned cookie or local storage access in embeds.",
    iconName: "database",
  },
  {
    key: "keyboardLock",
    label: "Keyboard lock",
    description: "Capture reserved system shortcuts while focused.",
    iconName: "keyboard",
  },
  {
    key: "fileSystem",
    label: "File access",
    description: "Prompt to read or save files via the File System Access API.",
    iconName: "projects",
  },
] as const;

export type OriginPermissionsState = Readonly<Record<string, SitePermissionDecision>>;
export type SitePermissionsStoreMap = Readonly<Record<string, OriginPermissionsState>>;

export function setOriginPermission(
  store: SitePermissionsStoreMap,
  origin: string,
  capabilityKey: string,
  decision: SitePermissionDecision,
): SitePermissionsStoreMap {
  return {
    ...store,
    [origin]: {
      ...(store[origin] ?? {}),
      [capabilityKey]: decision,
    },
  };
}

export function resetOriginPermissions(
  store: SitePermissionsStoreMap,
  origin: string,
): SitePermissionsStoreMap {
  const next = { ...store };
  delete next[origin];
  return next;
}
