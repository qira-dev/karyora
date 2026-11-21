/**
 * Multi-Tab Docked & Side-Pane PTY Terminal (UIR-5.5, UI-40, Screenshot #81, Blueprint §9).
 *
 * Mounts identically in the Bottom Docked Terminal (`Ctrl+J`) and inside the Multi-Tab Side Pane.
 * Features:
 * - Multi-tab header (`>_ Terminal zsh`, `+` add tab, `⤢` maximize, `×` close)
 * - Active Light/Dark Code Theme synchronized colors (`resolveCodeThemePalette`)
 * - Powerline prompt rendering (`karyora ~/karyora_Platform/karyora (main) ❯`)
 * - Honest Web-mode Core policy fallback banner when remote PTY shell is disabled (`UI-40`)
 */

import { useState, type KeyboardEvent, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button } from "../components/primitives";
import { resolveCodeThemePalette } from "../styles/codeThemeRegistry";
import type { DarkCodeTheme, LightCodeTheme, TargetPlatform } from "../types/decisions";
import {
  createTerminalTabSession,
  evaluateTerminalPolicy,
  formatPowerlinePrompt,
  type TerminalTabSession,
} from "./terminalConfig";

export interface DockedTerminalPanelProps {
  readonly runtimeTarget: TargetPlatform;
  readonly remoteShellEnabledByCore: boolean;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly placement: "bottom-dock" | "side-pane";
  readonly isMaximized?: boolean;
  readonly onToggleMaximize?: () => void;
  readonly onClose?: () => void;
  readonly onRunPtyCommand?: (terminalId: string, command: string) => Promise<string> | string;
}

export function DockedTerminalPanel({
  runtimeTarget,
  remoteShellEnabledByCore,
  activeCodeTheme,
  placement,
  isMaximized = false,
  onToggleMaximize,
  onClose,
  onRunPtyCommand,
}: DockedTerminalPanelProps): ReactElement {
  const [sessions, setSessions] = useState<readonly TerminalTabSession[]>(() => [
    createTerminalTabSession(1, runtimeTarget === "windows" ? "powershell" : "zsh"),
  ]);
  const [activeTerminalId, setActiveTerminalId] = useState<string>(
    () => sessions[0]?.terminalId ?? "",
  );
  const [inputValue, setInputValue] = useState("");
  const [historyCursor, setHistoryCursor] = useState<number>(-1);

  const policy = evaluateTerminalPolicy(runtimeTarget, remoteShellEnabledByCore);
  const palette = resolveCodeThemePalette(activeCodeTheme);

  const activeSession =
    sessions.find((s) => s.terminalId === activeTerminalId) ?? sessions[0];

  const handleAddTab = () => {
    const nextIndex = sessions.length + 1;
    const shell = runtimeTarget === "windows" ? "powershell" : "zsh";
    const created = createTerminalTabSession(nextIndex, shell);
    setSessions((prev) => [...prev, created]);
    setActiveTerminalId(created.terminalId);
  };

  const handleCloseTab = (terminalId: string) => {
    if (sessions.length <= 1) {
      onClose?.();
      return;
    }
    const idx = sessions.findIndex((s) => s.terminalId === terminalId);
    const next = sessions.filter((s) => s.terminalId !== terminalId);
    setSessions(next);
    if (activeTerminalId === terminalId) {
      setActiveTerminalId(next[Math.min(idx, next.length - 1)]?.terminalId ?? "");
    }
  };

  const handleExecuteCommand = async () => {
    const trimmed = inputValue.trim();
    if (!trimmed || !activeSession) return;

    if (!policy.ptyAllowed) {
      setSessions((prev) =>
        prev.map((s) =>
          s.terminalId === activeSession.terminalId
            ? {
                ...s,
                lines: [
                  ...s.lines,
                  {
                    lineId: `pol-${Date.now()}`,
                    kind: "policy",
                    text: `[POLICY BLOCKED] ${policy.policyBannerText ?? "PTY disabled by policy."}`,
                  },
                ],
              }
            : s,
        ),
      );
      setInputValue("");
      return;
    }

    if (trimmed === "clear") {
      setSessions((prev) =>
        prev.map((s) =>
          s.terminalId === activeSession.terminalId ? { ...s, lines: [] } : s,
        ),
      );
      setInputValue("");
      return;
    }

    const promptText = `${formatPowerlinePrompt(activeSession)} ${trimmed}`;
    const resultText = onRunPtyCommand
      ? await onRunPtyCommand(activeSession.terminalId, trimmed)
      : `Dispatched to Core PTY (${policy.modeLabel}): ${trimmed}`;

    setSessions((prev) =>
      prev.map((s) =>
        s.terminalId === activeSession.terminalId
          ? {
              ...s,
              commandHistory: [...s.commandHistory, trimmed],
              lines: [
                ...s.lines,
                { lineId: `p-${Date.now()}`, kind: "prompt", text: promptText },
                { lineId: `o-${Date.now() + 1}`, kind: "stdout", text: resultText },
              ],
            }
          : s,
      ),
    );
    setInputValue("");
    setHistoryCursor(-1);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void handleExecuteCommand();
    } else if (e.key === "ArrowUp" && activeSession) {
      e.preventDefault();
      const history = activeSession.commandHistory;
      if (history.length === 0) return;
      const nextCursor =
        historyCursor === -1 ? history.length - 1 : Math.max(0, historyCursor - 1);
      setHistoryCursor(nextCursor);
      setInputValue(history[nextCursor] ?? "");
    } else if (e.key === "ArrowDown" && activeSession) {
      e.preventDefault();
      const history = activeSession.commandHistory;
      if (historyCursor === -1) return;
      const nextCursor = historyCursor + 1;
      if (nextCursor >= history.length) {
        setHistoryCursor(-1);
        setInputValue("");
      } else {
        setHistoryCursor(nextCursor);
        setInputValue(history[nextCursor] ?? "");
      }
    } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "l" && activeSession) {
      e.preventDefault();
      setSessions((prev) =>
        prev.map((s) =>
          s.terminalId === activeSession.terminalId ? { ...s, lines: [] } : s,
        ),
      );
    }
  };

  return (
    <section
      className="ky-terminal-panel"
      data-testid={placement === "bottom-dock" ? "docked-terminal-panel" : "side-pane-terminal-panel"}
      data-placement={placement}
      data-maximized={isMaximized ? "true" : "false"}
      data-pty-allowed={policy.ptyAllowed ? "true" : "false"}
      style={{
        backgroundColor: palette.background,
        color: palette.foreground,
      }}
      aria-label="Multi-tab PTY Terminal"
    >
      <header className="ky-terminal-panel__header">
        <div className="ky-terminal-panel__tabs" role="tablist" aria-label="Terminal sessions">
          {sessions.map((session) => {
            const active = session.terminalId === activeSession?.terminalId;
            return (
              <div
                key={session.terminalId}
                className="ky-terminal-panel__tab"
                data-active={active ? "true" : "false"}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={active}
                  className="ky-terminal-panel__tab-btn"
                  onClick={() => setActiveTerminalId(session.terminalId)}
                >
                  <ChromeIcon name="terminal" size={14} />
                  <span>{session.title}</span>
                </button>
                <button
                  type="button"
                  className="ky-terminal-panel__tab-close"
                  aria-label={`Close ${session.title}`}
                  onClick={() => handleCloseTab(session.terminalId)}
                >
                  <ChromeIcon name="close" size={14} />
                </button>
              </div>
            );
          })}
          <Button
            size="sm"
            variant="ghost"
            onClick={handleAddTab}
            aria-label="New terminal tab"
            title="New terminal tab"
          >
            <ChromeIcon name="plus" size={14} />
          </Button>
        </div>

        <div className="ky-terminal-panel__actions">
          <Badge
            tone={policy.ptyAllowed ? "success" : "warning"}
            label={policy.modeLabel}
          />
          {onToggleMaximize && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onToggleMaximize}
              aria-label={isMaximized ? "Restore terminal height" : "Maximize terminal"}
              title={isMaximized ? "Restore terminal" : "Maximize terminal"}
            >
              <ChromeIcon name={isMaximized ? "minimize" : "maximize"} size={14} />
            </Button>
          )}
          {onClose && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onClose}
              aria-label="Close terminal panel"
              title="Close terminal (Ctrl+J)"
            >
              <ChromeIcon name="close" size={14} />
            </Button>
          )}
        </div>
      </header>

      {!policy.ptyAllowed && policy.policyBannerText && (
        <div
          className="ky-terminal-panel__policy-banner"
          role="status"
          data-testid="terminal-web-policy-fallback"
        >
          <ChromeIcon name="shield-security" size={14} />
          <span>{policy.policyBannerText}</span>
        </div>
      )}

      <div className="ky-terminal-panel__viewport" role="log" aria-live="polite">
        {activeSession?.lines.map((line) => (
          <div
            key={line.lineId}
            className="ky-terminal-panel__line"
            data-kind={line.kind}
            style={{
              color:
                line.kind === "prompt"
                  ? palette.functionName
                  : line.kind === "policy"
                    ? palette.typeName
                    : line.kind === "stderr"
                      ? palette.keyword
                      : palette.foreground,
            }}
          >
            {line.text}
          </div>
        ))}

        {activeSession && (
          <div className="ky-terminal-panel__input-row">
            <span
              className="ky-terminal-panel__prompt"
              style={{ color: palette.stringLiteral }}
            >
              {formatPowerlinePrompt(activeSession)}
            </span>
            <input
              type="text"
              className="ky-terminal-panel__input"
              aria-label="Terminal command input"
              value={inputValue}
              disabled={!policy.ptyAllowed}
              placeholder={
                policy.ptyAllowed
                  ? "Type a command (e.g. git status, go test ./ui/...)"
                  : "Shell input disabled by Web policy"
              }
              style={{ color: palette.foreground }}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={handleKeyDown}
            />
          </div>
        )}
      </div>
    </section>
  );
}
