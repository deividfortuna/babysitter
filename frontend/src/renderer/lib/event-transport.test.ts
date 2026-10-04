import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import { FakeEventSource } from "@test/fake-event-source";
import { setApiBaseUrl } from "./api-client";
import { connectEventTransport, type ConnectionListener } from "./event-transport";
import {
  authQueryKey,
  logLevelQueryKey,
  notificationsQueryKey,
  providersQueryKey,
  rateLimitQueryKey,
  repoConfigQueryKey,
  reposQueryKey,
  settingsMutationKey,
  settingsQueryKey,
  viewerQueryKey,
  watchesQueryKey,
} from "./query-keys";
import { deferred } from "@test/test-utils";

afterEach(() => {
  FakeEventSource.instances = [];
  vi.useRealTimers();
});

function connect(queryClient = new QueryClient(), onConnection: ConnectionListener = () => undefined) {
  setApiBaseUrl("http://localhost:1234");
  const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
  const dispose = connectEventTransport(queryClient, onConnection, { present: true });
  return { invalidateQueries, dispose, es: FakeEventSource.instances.at(-1)! };
}

describe("connectEventTransport", () => {
  it.each([
    {
      event: "ready",
      reads: "the provider catalog and the account",
      keys: [providersQueryKey, viewerQueryKey],
      repoConfigInvalidated: true,
    },
    { event: "log_level_changed", reads: "the log level", keys: [logLevelQueryKey], repoConfigInvalidated: false },
    {
      event: "auth_changed",
      reads: "the GitHub access and the account",
      keys: [authQueryKey, viewerQueryKey],
      repoConfigInvalidated: false,
    },
    { event: "settings_changed", reads: "the settings", keys: [settingsQueryKey], repoConfigInvalidated: false },
    {
      event: "notification_added",
      reads: "the notification history",
      keys: [notificationsQueryKey],
      repoConfigInvalidated: false,
    },
    { event: "watch_ready", reads: "the watches", keys: [watchesQueryKey], repoConfigInvalidated: true },
    { event: "watch_proposal", reads: "the watches", keys: [watchesQueryKey], repoConfigInvalidated: true },
    {
      event: "watch_changed",
      reads: "the watches and the rate limit",
      keys: [watchesQueryKey, rateLimitQueryKey],
      repoConfigInvalidated: true,
    },
    { event: "repo_synced", reads: "the rate limit", keys: [rateLimitQueryKey], repoConfigInvalidated: true },
    { event: "watch_activity", reads: "the rate limit", keys: [rateLimitQueryKey], repoConfigInvalidated: true },
    {
      event: "repo_changed",
      reads: "the repositories and the configuration of each repository",
      keys: [reposQueryKey],
      repoConfigInvalidated: true,
    },
  ])("$event reads $reads again", ({ event, keys, repoConfigInvalidated }) => {
    vi.useFakeTimers();
    const queryClient = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } });
    queryClient.setQueryData(repoConfigQueryKey(3), { checkoutDir: "" });
    const { invalidateQueries, dispose, es } = connect(queryClient);

    es.dispatch(event);
    vi.runAllTimers();

    for (const queryKey of keys) expect(invalidateQueries).toHaveBeenCalledWith({ queryKey });
    expect(queryClient.getQueryState(repoConfigQueryKey(3))?.isInvalidated).toBe(repoConfigInvalidated);

    dispose();
  });

  it("holds the settings while a save of them runs, then reads them once", async () => {
    const queryClient = new QueryClient();
    const held = deferred();
    const saving = queryClient.getMutationCache().build(queryClient, {
      mutationKey: settingsMutationKey,
      mutationFn: () => held.promise,
    });
    const saved = saving.execute(undefined);

    const { invalidateQueries, dispose, es } = connect(queryClient);
    es.dispatch("settings_changed");
    es.dispatch("settings_changed");
    const settingsReads = () =>
      invalidateQueries.mock.calls.filter(([filters]) => filters?.queryKey === settingsQueryKey).length;

    expect(settingsReads()).toBe(0);
    held.resolve();
    await saved;
    expect(settingsReads()).toBe(1);

    dispose();
  });

  it("tells the daemon that the app shows the notifications itself", () => {
    const { es, dispose } = connect();

    expect(es.url).toBe("http://localhost:1234/events?present=1");

    dispose();
  });

  it("leaves the showing to the daemon outside the app", () => {
    setApiBaseUrl("http://localhost:1234");

    const dispose = connectEventTransport(new QueryClient());

    expect(FakeEventSource.instances.at(-1)!.url).toBe("http://localhost:1234/events");

    dispose();
  });
});

describe("the ready frame", () => {
  it.each([
    {
      name: "hands over the newest notification the daemon holds",
      payload: { seq: 12, lastNotificationId: 7 },
      lastNotificationId: 7,
    },
    {
      name: "names no row when the daemon kept the showing",
      payload: { seq: 12, lastNotificationId: null },
      lastNotificationId: null,
    },
    { name: "names no row when the frame cannot be read", payload: undefined, lastNotificationId: null },
  ])("$name", ({ payload, lastNotificationId }) => {
    const onConnection = vi.fn();
    const { es, dispose } = connect(new QueryClient(), onConnection);

    es.dispatch("ready", payload);

    expect(onConnection).toHaveBeenLastCalledWith("open", { lastNotificationId });

    dispose();
  });
});
