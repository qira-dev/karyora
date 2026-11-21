/**
 * Full-View Mode Floating Minimized Working Bar (UIR-5.1, Screenshot #83, Blueprint §8.6, E2E-08).
 *
 * Appears docked at bottom-right while the Side Pane is expanded in Full-View Mode
 * (`Ctrl+Shift+F`). Displays `+ Working for 26m 37s` and restores the split view on click.
 */

import type { ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { formatWorkingDuration } from "./sidePaneConfig";

export interface FloatingMinimizedWorkingBarProps {
  readonly isVisible: boolean;
  readonly isWorking: boolean;
  readonly elapsedSeconds: number;
  readonly activeTaskTitle: string;
  readonly onRestoreSplitView: () => void;
}

export function FloatingMinimizedWorkingBar({
  isVisible,
  isWorking,
  elapsedSeconds,
  activeTaskTitle,
  onRestoreSplitView,
}: FloatingMinimizedWorkingBarProps): ReactElement | null {
  if (!isVisible) return null;

  const statusText = isWorking
    ? `Working for ${formatWorkingDuration(elapsedSeconds)}`
    : `Paused · ${activeTaskTitle}`;

  return (
    <button
      type="button"
      className="ky-floating-working-bar"
      data-testid="floating-minimized-working-bar"
      onClick={onRestoreSplitView}
      aria-label={`Restore split workspace view (${statusText})`}
      title="Restore split view (Ctrl+Shift+F)"
    >
      <span className="ky-floating-working-bar__plus">
        <ChromeIcon name="plus" size={14} />
      </span>
      <span
        className={`ky-floating-working-bar__label ${isWorking ? "animated-gradient-text" : ""}`}
      >
        {statusText}
      </span>
    </button>
  );
}
