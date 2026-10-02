import type { QueryClient } from "@tanstack/react-query";
import { getApiBaseUrl, subscribeApiBaseUrl } from "./api-client";
import {
  authQueryKey,
  logLevelQueryKey,
  notificationsQueryKey,
  providersQueryKey,
  pullsQueryKey,
  rateLimitQueryKey,
  reposQueryKey,
  settingsMutationKey,
  settingsQueryKey,
  viewerQueryKey,
  watchesQueryKey,
} from "./query-keys";

const EVENT_TYPES = [
  "repo_added",
  "repo_removed",
  "repo_synced",
  "repo_changed",
  "pull_updated",
  "watch_started",
  "watch_stopped",
  "watch_activity",
  "watch_session",
  "watch_ready",
  "watch_proposal",
  "watch_changed",
] as const;

const INVALIDATE_WINDOW_MS = 150;

type QueryKey = readonly unknown[];

function invalidationHeldWhileSaving(queryClient: QueryClient, queryKey: QueryKey, mutationKey: QueryKey) {
  let held = false;
  const idle = () => queryClient.isMutating({ mutationKey }) === 0;
  const invalidate = () => void queryClient.invalidateQueries({ queryKey });
  const unsubscribe = queryClient.getMutationCache().subscribe(() => {
    if (!held || !idle()) return;
    held = false;
    invalidate();
  });
  function request() {
    if (idle()) invalidate();
    else held = true;
  }
  return { request, dispose: unsubscribe };
}

function readyFrame(data: unknown): ReadyFrame {
  try {
    const parsed: unknown = typeof data === "string" ? JSON.parse(data) : data;
    const id = (parsed as { lastNotificationId?: unknown } | null)?.lastNotificationId;
    return { lastNotificationId: typeof id === "number" ? id : null };
  } catch {
    return { lastNotificationId: null };
  }
}
const RETRY_BASE_MS = 500;
const RETRY_MAX_MS = 10_000;

export type EventsConnection = "connecting" | "open" | "closed";

export type ReadyFrame = {
  lastNotificationId: number | null;
};

export type ConnectionListener = (state: EventsConnection, ready?: ReadyFrame) => void;

export type EventTransportOptions = {
  present?: boolean;
};

export function connectEventTransport(
  queryClient: QueryClient,
  onConnection: ConnectionListener = () => undefined,
  options: EventTransportOptions = {},
): () => void {
  let source: EventSource | null = null;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  let attempt = 0;
  let disposed = false;
  const settings = invalidationHeldWhileSaving(queryClient, settingsQueryKey, settingsMutationKey);

  function scheduleRefetch() {
    if (refreshTimer) return;
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      void queryClient.invalidateQueries({ queryKey: reposQueryKey });
      void queryClient.invalidateQueries({ queryKey: pullsQueryKey });
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
      void queryClient.invalidateQueries({ queryKey: rateLimitQueryKey });
    }, INVALIDATE_WINDOW_MS);
  }

  function close() {
    if (retryTimer) {
      clearTimeout(retryTimer);
      retryTimer = null;
    }
    source?.close();
    source = null;
  }

  function scheduleRetry() {
    if (disposed || retryTimer) return;
    const delay = Math.min(RETRY_BASE_MS * 2 ** attempt, RETRY_MAX_MS);
    attempt += 1;
    retryTimer = setTimeout(() => {
      retryTimer = null;
      open();
    }, delay);
  }

  function open() {
    close();
    const base = getApiBaseUrl();
    if (!base || disposed) {
      onConnection("closed");
      return;
    }
    onConnection("connecting");
    const es = new EventSource(`${base}/events${options.present ? "?present=1" : ""}`);
    source = es;
    es.addEventListener("ready", (event: MessageEvent) => {
      attempt = 0;
      onConnection("open", readyFrame(event.data));
      scheduleRefetch();
      void queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
      void queryClient.invalidateQueries({ queryKey: providersQueryKey });
      void queryClient.invalidateQueries({ queryKey: viewerQueryKey });
      void queryClient.invalidateQueries({ queryKey: authQueryKey });
    });
    for (const type of EVENT_TYPES) {
      es.addEventListener(type, scheduleRefetch);
    }
    es.addEventListener("settings_changed", settings.request);
    es.addEventListener("auth_changed", () => {
      void queryClient.invalidateQueries({ queryKey: authQueryKey });
      void queryClient.invalidateQueries({ queryKey: viewerQueryKey });
    });
    es.addEventListener("log_level_changed", () => {
      void queryClient.invalidateQueries({ queryKey: logLevelQueryKey });
    });
    for (const type of ["notification_added", "notifications_read"] as const) {
      es.addEventListener(type, () => {
        void queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
      });
    }
    es.onerror = () => {
      if (source !== es) return;
      onConnection("closed");
      close();
      scheduleRetry();
    };
  }

  const unsubscribe = subscribeApiBaseUrl(() => {
    attempt = 0;
    open();
  });
  open();

  return () => {
    disposed = true;
    unsubscribe();
    settings.dispose();
    close();
    if (refreshTimer) clearTimeout(refreshTimer);
    onConnection("closed");
  };
}
