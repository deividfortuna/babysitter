import type { DaemonStatus } from "../shared/daemon-status";
import { watchFolder, type WatchFolderSource } from "../shared/open-in";
import { isRecord } from "../shared/values";

const WATCH_TIMEOUT_MS = 4_000;

type Fetch = typeof fetch;

export async function localWatchFolder(status: DaemonStatus, watchId: number, fetcher: Fetch = fetch) {
  const localBaseUrl = status.connection?.kind === "local" ? status.baseUrl : undefined;
  if (!localBaseUrl) return null;
  try {
    const response = await fetcher(`${localBaseUrl}/watches/${watchId}`, {
      signal: AbortSignal.timeout(WATCH_TIMEOUT_MS),
    });
    if (!response.ok) return null;
    const watch: unknown = await response.json();
    return isWatchFolderSource(watch) ? watchFolder(watch) : null;
  } catch {
    return null;
  }
}

function isWatchFolderSource(value: unknown): value is WatchFolderSource {
  if (!isRecord(value)) return false;
  return [value.worktreeDir, value.sourceDir, value.provider].every((field) => typeof field === "string");
}
