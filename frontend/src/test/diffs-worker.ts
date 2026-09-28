import type { WorkerPoolManager } from "@pierre/diffs/worker";

export type { WorkerPoolManager };

let pool: WorkerPoolManager | undefined;

export const created = { count: 0 };

export function getOrCreateWorkerPoolSingleton(): WorkerPoolManager {
  if (!pool) {
    created.count++;
    pool = {} as WorkerPoolManager;
  }
  return pool;
}

export function terminateWorkerPoolSingleton(): void {
  pool = undefined;
}
