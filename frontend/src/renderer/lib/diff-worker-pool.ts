import { useSyncExternalStore } from "react";
import type { HighlighterTypes } from "@pierre/diffs";
import {
  getOrCreateWorkerPoolSingleton,
  terminateWorkerPoolSingleton,
  type WorkerPoolManager,
} from "@pierre/diffs/worker";

export const DIFF_THEMES = { light: "github-light-default", dark: "github-dark-default" } as const;

export const PREFERRED_HIGHLIGHTER: HighlighterTypes = "shiki-wasm";

export const TOKENIZE_MAX_LINE_LENGTH = 1000;

const KEEP_ALIVE_MS = 30_000;

let users = 0;
let stopTimer: ReturnType<typeof setTimeout> | undefined;

function poolSize(): number {
  const cores = typeof navigator === "undefined" ? 4 : navigator.hardwareConcurrency || 4;
  return Math.min(6, Math.max(2, Math.floor(cores / 2)));
}

function pool(): WorkerPoolManager {
  return getOrCreateWorkerPoolSingleton({
    poolOptions: {
      workerFactory: () => new Worker(new URL("@pierre/diffs/worker/worker.js", import.meta.url), { type: "module" }),
      poolSize: poolSize(),
      totalASTLRUCacheSize: 240,
    },
    highlighterOptions: {
      theme: DIFF_THEMES,
      preferredHighlighter: PREFERRED_HIGHLIGHTER,
      tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
    },
  });
}

function subscribe(): () => void {
  clearTimeout(stopTimer);
  users++;
  return () => {
    users--;
    if (users > 0) return;
    stopTimer = setTimeout(terminateWorkerPoolSingleton, KEEP_ALIVE_MS);
  };
}

function noServerPool(): undefined {
  return undefined;
}

export function useDiffWorkerPool(): WorkerPoolManager | undefined {
  return useSyncExternalStore(subscribe, pool, noServerPool);
}
