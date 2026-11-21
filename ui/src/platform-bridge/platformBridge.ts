/**
 * Four-Platform Runtime Bridge (`web` | `windows` | `macos` | `linux`) (UIR-1.2, UI-03, UI-16, UI-19, UI-24).
 *
 * Enables one shared React 19 + TypeScript + Vite SPA (`karyora/ui/dist` embedded via `ui/embed.go`)
 * to run seamlessly across Web Browser, Windows, macOS, and Linux while adapting host-specific
 * capabilities honestly (Blueprint §3.1, §3.2, §23.2).
 */

import type { TargetPlatform } from "../types/decisions";

export interface PlatformCapabilities {
  readonly nativeFolderPicker: boolean;
  readonly systemTray: boolean;
  readonly keepAwake: boolean;
  readonly osNotifications: boolean;
  readonly localPtyTerminal: boolean;
  readonly computerUse: boolean;
  readonly osKeyringAvailable: boolean;
}

export interface PickerOptions {
  readonly mode: "folder" | "file" | "multiple-files";
  readonly title?: string;
  readonly defaultPath?: string;
  readonly acceptExtensions?: readonly string[];
}

export interface PickedPath {
  readonly name: string;
  readonly path: string;
  readonly isDirectory: boolean;
}

export type WindowCloseDisposition =
  | { readonly action: "allow-close" }
  | { readonly action: "hide-to-tray"; readonly activeRunningJobsCount: number }
  | {
      readonly action: "confirm-active-jobs";
      readonly activeRunningJobsCount: number;
    };

export type ExternalUrlValidationResult =
  | { readonly allowed: true; readonly normalizedUrl: string; readonly domain: string }
  | { readonly allowed: false; readonly reason: string };

export interface PlatformBridge {
  readonly runtimeTarget: TargetPlatform;
  readonly isDesktop: boolean;
  readonly capabilities: Readonly<PlatformCapabilities>;
  pickFolderOrFile(options: PickerOptions): Promise<readonly PickedPath[] | null>;
  setWindowTitle(title: string): Promise<void>;
  setKeepAwake(enabled: boolean): Promise<boolean>;
  setHideToTrayOnClose(enabled: boolean): Promise<boolean>;
  evaluateWindowCloseGuard(
    activeRunningJobsCount: number,
    hideToTrayEnabled: boolean,
  ): WindowCloseDisposition;
  validateExternalUrlForOpen(rawUrl: string): ExternalUrlValidationResult;
}

export interface RuntimeEnvironmentHint {
  readonly desktopPlatform?: "windows" | "macos" | "linux";
  readonly isDesktopShell?: boolean;
  readonly userAgent?: string;
  readonly computerUseSupported?: boolean;
}

/**
 * Deterministically detects the current target platform (`web`, `windows`, `macos`, `linux`).
 */
export function detectRuntimePlatform(
  hint?: RuntimeEnvironmentHint,
): TargetPlatform {
  if (hint?.desktopPlatform) {
    return hint.desktopPlatform;
  }

  const globalAny = globalThis as Record<string, unknown>;
  const injectedPlatform = globalAny.__KARYORA_DESKTOP_PLATFORM__;
  if (
    injectedPlatform === "windows" ||
    injectedPlatform === "macos" ||
    injectedPlatform === "linux"
  ) {
    return injectedPlatform;
  }

  const isDesktopShell =
    Boolean(hint?.isDesktopShell) ||
    Boolean(globalAny.__WAILS__) ||
    Boolean(globalAny.wails);

  if (!isDesktopShell) {
    return "web";
  }

  const ua = (
    hint?.userAgent ??
    (typeof navigator !== "undefined" ? navigator.userAgent : "")
  ).toLowerCase();

  if (ua.includes("win")) {
    return "windows";
  }
  if (ua.includes("mac") || ua.includes("darwin")) {
    return "macos";
  }
  return "linux";
}

/**
 * Resolves the authoritative capability matrix for the detected target platform
 * (Blueprint §3.2, Screenshot #20 Computer Use boundary).
 */
export function getPlatformCapabilities(
  target: TargetPlatform,
  hint?: RuntimeEnvironmentHint,
): Readonly<PlatformCapabilities> {
  if (target === "web") {
    return {
      nativeFolderPicker: false,
      systemTray: false,
      keepAwake: false,
      osNotifications: true,
      localPtyTerminal: true, // Mediated by Core PTY policy
      computerUse: false,
      osKeyringAvailable: true, // Mediated by Core D-37 keyring boundary
    };
  }

  return {
    nativeFolderPicker: true,
    systemTray: true,
    keepAwake: true,
    osNotifications: true,
    localPtyTerminal: true,
    // Screenshot #20: Computer Use availability is honestly reported per OS/host
    computerUse:
      typeof hint?.computerUseSupported === "boolean"
        ? hint.computerUseSupported
        : target === "macos" || target === "windows",
    osKeyringAvailable: true,
  };
}

/**
 * Validates an external URL before opening, blocking dangerous schemes (`javascript:`,
 * `data:`, `file:`, `vbscript:`) and extracting the target domain for user confirmation
 * (Blueprint §22.3, §23.2).
 */
export function validateExternalUrlForOpen(
  rawUrl: string,
): ExternalUrlValidationResult {
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

/**
 * Enforces UI-24 Desktop Window Close & Active Job Lifecycle Safety:
 * Closing a desktop window never silently corrupts or kills active running Jobs.
 */
export function evaluateWindowCloseGuard(
  target: TargetPlatform,
  activeRunningJobsCount: number,
  hideToTrayEnabled: boolean,
): WindowCloseDisposition {
  if (target !== "web" && hideToTrayEnabled) {
    return {
      action: "hide-to-tray",
      activeRunningJobsCount,
    };
  }
  if (activeRunningJobsCount > 0) {
    return {
      action: "confirm-active-jobs",
      activeRunningJobsCount,
    };
  }
  return { action: "allow-close" };
}

export function createPlatformBridge(
  hint?: RuntimeEnvironmentHint,
): PlatformBridge {
  const runtimeTarget = detectRuntimePlatform(hint);
  const capabilities = getPlatformCapabilities(runtimeTarget, hint);
  const isDesktop = runtimeTarget !== "web";

  let keepAwakeActive = false;
  let hideToTrayActive = isDesktop;

  return {
    runtimeTarget,
    isDesktop,
    capabilities,
    async pickFolderOrFile(_options: PickerOptions): Promise<readonly PickedPath[] | null> {
      if (!capabilities.nativeFolderPicker) {
        return null;
      }
      return [];
    },
    async setWindowTitle(title: string): Promise<void> {
      if (typeof document !== "undefined") {
        document.title = title ? `${title} — Karyora` : "Karyora";
      }
    },
    async setKeepAwake(enabled: boolean): Promise<boolean> {
      if (!capabilities.keepAwake) {
        return false;
      }
      keepAwakeActive = enabled;
      return keepAwakeActive;
    },
    async setHideToTrayOnClose(enabled: boolean): Promise<boolean> {
      if (!capabilities.systemTray) {
        return false;
      }
      hideToTrayActive = enabled;
      return hideToTrayActive;
    },
    evaluateWindowCloseGuard(
      activeRunningJobsCount: number,
      hideToTrayEnabled = hideToTrayActive,
    ): WindowCloseDisposition {
      return evaluateWindowCloseGuard(
        runtimeTarget,
        activeRunningJobsCount,
        hideToTrayEnabled,
      );
    },
    validateExternalUrlForOpen,
  };
}
