/**
 * Docked & Side-Pane Multi-Tab PTY Terminal Configuration & Policy Engine
 * (UIR-5.5, UI-40, Screenshot #81, Blueprint §9).
 *
 * Pure TypeScript module (import-type only) so both Vite and Node 26 native tests import it directly.
 */

import type { TargetPlatform } from "../types/decisions";

export interface TerminalTabSession {
  readonly terminalId: string;
  readonly title: string;
  readonly shellName: "zsh" | "bash" | "powershell";
  readonly cwd: string;
  readonly gitBranch: string;
  readonly lines: readonly TerminalOutputLine[];
  readonly commandHistory: readonly string[];
}

export interface TerminalOutputLine {
  readonly lineId: string;
  readonly kind: "prompt" | "stdout" | "stderr" | "policy";
  readonly text: string;
}

export interface TerminalPolicyState {
  readonly ptyAllowed: boolean;
  readonly modeLabel: string;
  readonly reasonCode: "DESKTOP_LOCAL_PTY" | "WEB_CORE_PTY_ENABLED" | "WEB_REMOTE_SHELL_DISABLED";
  readonly policyBannerText: string | null;
}

/**
 * Evaluates honest PTY availability across Web Browser, Linux, macOS, and Windows (UI-40).
 * When running in Web Browser mode with remote shell disabled by Core policy, returns
 * an explicit policy explanation instead of faking shell execution.
 */
export function evaluateTerminalPolicy(
  runtimeTarget: TargetPlatform,
  remoteShellEnabledByCore: boolean,
): TerminalPolicyState {
  if (runtimeTarget !== "web") {
    return {
      ptyAllowed: true,
      modeLabel: `Local PTY (${runtimeTarget})`,
      reasonCode: "DESKTOP_LOCAL_PTY",
      policyBannerText: null,
    };
  }
  if (remoteShellEnabledByCore) {
    return {
      ptyAllowed: true,
      modeLabel: "Core PTY Stream (Web)",
      reasonCode: "WEB_CORE_PTY_ENABLED",
      policyBannerText: null,
    };
  }
  return {
    ptyAllowed: false,
    modeLabel: "Web Policy Read-Only",
    reasonCode: "WEB_REMOTE_SHELL_DISABLED",
    policyBannerText:
      "Interactive PTY shell execution is disabled by Core security policy in Web Browser mode. Enable Core PTY relay in Settings → Security or use the Desktop app.",
  };
}

/**
 * Creates a fresh powerline-ready terminal tab session (`Screenshot #81`).
 */
export function createTerminalTabSession(
  index: number,
  shellName: "zsh" | "bash" | "powershell" = "zsh",
  cwd = "~/karyora_Platform/karyora",
  gitBranch = "main",
): TerminalTabSession {
  return {
    terminalId: `term-${index}-${Date.now()}`,
    title: index === 1 ? `Terminal ${shellName}` : `Terminal ${shellName} (${index})`,
    shellName,
    cwd,
    gitBranch,
    lines: [
      {
        lineId: `welcome-${index}`,
        kind: "stdout",
        text: `Karyora PTY Session (${shellName}) — connected to ${cwd} [${gitBranch}]`,
      },
    ],
    commandHistory: [],
  };
}

/**
 * Formats a ZCode-style powerline prompt string (`Screenshot #81`).
 */
export function formatPowerlinePrompt(session: TerminalTabSession): string {
  return `karyora ${session.cwd} (${session.gitBranch}) ❯`;
}
