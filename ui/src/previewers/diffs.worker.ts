/**
 * Off-Main-Thread Diff Web Worker (UIR-5.2, Blueprint §8.3, Quality Gate G-13).
 *
 * Computes line-level and word-level unified/split diffs outside the UI thread
 * so 60 fps scroll and 100+ tok/s conversation streaming never jank.
 */

import {
  computeLineAndWordDiff,
  type ComputedFileDiff,
} from "./previewerSanitizer";

export interface DiffWorkerRequest {
  readonly requestId: string;
  readonly filePath: string;
  readonly oldText: string;
  readonly newText: string;
}

export interface DiffWorkerResponse {
  readonly requestId: string;
  readonly diff: ComputedFileDiff;
  readonly durationMs: number;
}

if (
  typeof self !== "undefined" &&
  typeof (self as unknown as { postMessage?: unknown }).postMessage === "function" &&
  typeof window === "undefined"
) {
  self.onmessage = (event: MessageEvent<DiffWorkerRequest>) => {
    const start = performance.now();
    const { requestId, filePath, oldText, newText } = event.data;
    const diff = computeLineAndWordDiff(filePath, oldText, newText);
    const durationMs = Math.round((performance.now() - start) * 100) / 100;
    const response: DiffWorkerResponse = {
      requestId,
      diff,
      durationMs,
    };
    self.postMessage(response);
  };
}
