/**
 * Side-Pane `Review` Diff Tab (UIR-5.2, Blueprint §8.3, Quality Gate G-13).
 *
 * Displays aggregated changed files (`+A -R` DiffCountPill + Material SVG icons) and
 * computes unified or side-by-side split word-level diffs off the main thread via
 * `diffs.worker.ts` (with clean synchronous fallback in environments without Web Workers).
 */

import { useEffect, useMemo, useState, type ReactElement } from "react";
import { MaterialFileIcon } from "../components/icons";
import { Badge, Button, DiffCountPill } from "../components/primitives";
import {
  computeLineAndWordDiff,
  type ComputedFileDiff,
  type WordDiffSegment,
} from "../previewers/previewerSanitizer";
import { resolveCodeThemePalette } from "../styles/codeThemeRegistry";
import type { DarkCodeTheme, LightCodeTheme } from "../types/decisions";
import type { ReviewDiffFileRecord } from "./sidePaneConfig";

export interface ReviewDiffTabProps {
  readonly files: readonly ReviewDiffFileRecord[];
  readonly initialSelectedPath?: string | null;
  readonly activeCodeTheme: LightCodeTheme | DarkCodeTheme;
  readonly onSendHunkToChat?: (filePath: string, summary: string) => void;
}

function renderSegments(
  segments: readonly WordDiffSegment[] | undefined,
  fallbackText: string,
): ReactElement {
  if (!segments || segments.length === 0) {
    return <span>{fallbackText || " "}</span>;
  }
  return (
    <span>
      {segments.map((seg, idx) => (
        <span
          key={`seg-${idx}`}
          className={
            seg.kind === "added"
              ? "ky-diff-word--added"
              : seg.kind === "removed"
                ? "ky-diff-word--removed"
                : undefined
          }
        >
          {seg.text}
        </span>
      ))}
    </span>
  );
}

export function ReviewDiffTab({
  files,
  initialSelectedPath,
  activeCodeTheme,
  onSendHunkToChat,
}: ReviewDiffTabProps): ReactElement {
  const [selectedPath, setSelectedPath] = useState<string | null>(
    initialSelectedPath ?? files[0]?.filePath ?? null,
  );
  const [viewMode, setViewMode] = useState<"unified" | "split">("unified");
  const [computedDiff, setComputedDiff] = useState<ComputedFileDiff | null>(null);
  const [workerDurationMs, setWorkerDurationMs] = useState<number | null>(null);

  const palette = resolveCodeThemePalette(activeCodeTheme);
  const activeFile =
    files.find((f) => f.filePath === selectedPath) ?? files[0] ?? null;

  const perFileStats = useMemo(
    () =>
      files.map((f) => {
        const quick = computeLineAndWordDiff(f.filePath, f.oldText, f.newText);
        return {
          filePath: f.filePath,
          gitStatus: f.gitStatus,
          additions: quick.additions,
          deletions: quick.deletions,
        };
      }),
    [files],
  );

  // Off-main-thread Web Worker diff execution with deterministic fallback
  useEffect(() => {
    if (!activeFile) {
      setComputedDiff(null);
      return;
    }

    let cancelled = false;
    let worker: Worker | null = null;

    if (typeof Worker !== "undefined") {
      try {
        worker = new Worker(
          new URL("../previewers/diffs.worker.ts", import.meta.url),
          { type: "module" },
        );
        const requestId = `diff-${Date.now()}`;
        worker.onmessage = (event: MessageEvent<{ requestId: string; diff: ComputedFileDiff; durationMs: number }>) => {
          if (!cancelled && event.data.requestId === requestId) {
            setComputedDiff(event.data.diff);
            setWorkerDurationMs(event.data.durationMs);
          }
        };
        worker.onerror = () => {
          if (!cancelled) {
            setComputedDiff(
              computeLineAndWordDiff(
                activeFile.filePath,
                activeFile.oldText,
                activeFile.newText,
              ),
            );
          }
        };
        worker.postMessage({
          requestId,
          filePath: activeFile.filePath,
          oldText: activeFile.oldText,
          newText: activeFile.newText,
        });
        return () => {
          cancelled = true;
          worker?.terminate();
        };
      } catch {
        // Fallback below when worker instantiation is unavailable
      }
    }

    setComputedDiff(
      computeLineAndWordDiff(
        activeFile.filePath,
        activeFile.oldText,
        activeFile.newText,
      ),
    );
    return () => {
      cancelled = true;
    };
  }, [activeFile]);

  return (
    <section
      className="ky-review-diff"
      data-testid="review-diff-tab"
      aria-label="Task changes review and diff viewer"
    >
      <header className="ky-review-diff__header">
        <div className="ky-review-diff__file-pills">
          {perFileStats.map((stat) => (
            <button
              key={stat.filePath}
              type="button"
              className="ky-review-diff__file-btn"
              data-active={stat.filePath === activeFile?.filePath ? "true" : "false"}
              onClick={() => setSelectedPath(stat.filePath)}
            >
              <MaterialFileIcon
                filename={stat.filePath}
                codeTheme={activeCodeTheme}
                size={16}
              />
              <span>{stat.filePath.split("/").pop() ?? stat.filePath}</span>
              <DiffCountPill
                additions={stat.additions}
                deletions={stat.deletions}
              />
            </button>
          ))}
        </div>

        <div className="ky-review-diff__mode-bar">
          {workerDurationMs !== null && (
            <Badge tone="neutral" label={`Worker ${workerDurationMs}ms`} />
          )}
          <Button
            size="sm"
            variant={viewMode === "unified" ? "primary" : "ghost"}
            onClick={() => setViewMode("unified")}
          >
            Unified
          </Button>
          <Button
            size="sm"
            variant={viewMode === "split" ? "primary" : "ghost"}
            onClick={() => setViewMode("split")}
          >
            Split
          </Button>
          {activeFile && computedDiff && onSendHunkToChat && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                onSendHunkToChat(
                  activeFile.filePath,
                  `Review diff ${activeFile.filePath} (+${computedDiff.additions} -${computedDiff.deletions})`,
                )
              }
            >
              Discuss diff
            </Button>
          )}
        </div>
      </header>

      <div
        className="ky-review-diff__viewport"
        data-view-mode={viewMode}
        style={{
          backgroundColor: palette.background,
          color: palette.foreground,
        }}
      >
        {computedDiff ? (
          viewMode === "unified" ? (
            computedDiff.lines.map((row, idx) => (
              <div
                key={`u-${idx}`}
                className="ky-diff-row"
                data-diff-kind={row.kind}
              >
                <span className="ky-diff-row__num">{row.oldLineNumber ?? ""}</span>
                <span className="ky-diff-row__num">{row.newLineNumber ?? ""}</span>
                <span className="ky-diff-row__sign">
                  {row.kind === "added" ? "+" : row.kind === "removed" ? "-" : " "}
                </span>
                <code className="ky-diff-row__code">
                  {renderSegments(row.wordSegments, row.text)}
                </code>
              </div>
            ))
          ) : (
            <div className="ky-diff-split-grid">
              {computedDiff.lines.map((row, idx) => (
                <div key={`s-${idx}`} className="ky-diff-split-row">
                  <div
                    className="ky-diff-split-cell"
                    data-diff-kind={row.kind === "removed" ? "removed" : "context"}
                  >
                    <span className="ky-diff-row__num">{row.oldLineNumber ?? ""}</span>
                    <code className="ky-diff-row__code">
                      {row.kind !== "added"
                        ? renderSegments(row.wordSegments, row.text)
                        : ""}
                    </code>
                  </div>
                  <div
                    className="ky-diff-split-cell"
                    data-diff-kind={row.kind === "added" ? "added" : "context"}
                  >
                    <span className="ky-diff-row__num">{row.newLineNumber ?? ""}</span>
                    <code className="ky-diff-row__code">
                      {row.kind !== "removed"
                        ? renderSegments(row.wordSegments, row.text)
                        : ""}
                    </code>
                  </div>
                </div>
              ))}
            </div>
          )
        ) : (
          <div className="ky-files-split__empty">No modified files in this task yet.</div>
        )}
      </div>
    </section>
  );
}
