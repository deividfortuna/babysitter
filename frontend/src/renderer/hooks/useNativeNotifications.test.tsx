import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { buildNotification, buildSettings } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import type { Settings } from "@/hooks/useSettings";
import { createQueryClientForTests } from "@test/test-utils";
import type { Notification } from "@/hooks/useNotifications";
import { bridge } from "@/lib/bridge";
import { notificationsQueryKey, settingsQueryKey } from "@/lib/query-keys";
import { useNativeNotifications } from "./useNativeNotifications";

function failSettings() {
  server.use(
    http.get(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { message: "read settings: database is locked" } }, { status: 500 }),
    ),
  );
}

function harness(notifications: Notification[], settings: Settings = buildSettings()) {
  serveApi({ notifications, settings });
  const queryClient = createQueryClientForTests();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const show = vi.spyOn(bridge.notifications, "show").mockResolvedValue(undefined);
  const setBadge = vi.spyOn(bridge.notifications, "setBadge").mockResolvedValue(undefined);
  return { queryClient, wrapper, show, setBadge };
}

test("the first load is history: the badge is set and nothing is shown", async () => {
  const { wrapper, show, setBadge } = harness([buildNotification({ id: 3 }), buildNotification({ id: 2 })]);

  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });

  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(2));
  expect(show).not.toHaveBeenCalled();
});

test("a row that arrives while the app runs is shown once", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3, body: "old" })]);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({
    notifications: [
      buildNotification({ id: 4, watchId: 42, title: "PR #7", body: "build failed" }),
      buildNotification({ id: 3, body: "old" }),
    ],
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledTimes(1));
  expect(show).toHaveBeenCalledWith(
    expect.objectContaining({ id: 4, title: "PR #7", body: "build failed", watchId: 42 }),
  );
});

test("the row carries its kind, so the dock knows how loud to be", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({ notifications: [buildNotification({ id: 4, kind: "agent" }), buildNotification({ id: 3 })] });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledWith(expect.objectContaining({ kind: "agent", silent: false })));
});

test("the sound setting of the daemon silences what the app shows", async () => {
  const { queryClient, wrapper, show } = harness(
    [buildNotification({ id: 3 })],
    buildSettings({ notificationSound: false }),
  );
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({
    notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })],
    settings: buildSettings({ notificationSound: false }),
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledWith(expect.objectContaining({ silent: true })));
});

test("a row that asked for no sound is silent while the sound is on", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({ notifications: [buildNotification({ id: 4, silent: true }), buildNotification({ id: 3 })] });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 4, silent: true })));
});

test("the notifications turned off show nothing, and the badge still counts", async () => {
  const off = buildSettings({ notificationsEnabled: false });
  const { queryClient, wrapper, show, setBadge } = harness([buildNotification({ id: 3 })], off);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(1));

  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })], settings: off });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(2));
  expect(show).not.toHaveBeenCalled();
});

test("a row the daemon already marked as seen is not shown", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({
    notifications: [buildNotification({ id: 4, readAt: "2026-09-21T12:30:00Z" }), buildNotification({ id: 3 })],
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());
  expect(show).not.toHaveBeenCalled();
});

test("the menu bar item opens the notification screen", async () => {
  const { wrapper } = harness([]);
  const onNavigate = vi.fn();
  const opens: (() => void)[] = [];
  vi.spyOn(bridge.notifications, "onOpen").mockImplementation((listener) => {
    opens.push(listener);
    return () => undefined;
  });

  renderHook(() => useNativeNotifications(true, onNavigate, 3), { wrapper });
  opens[0]();

  expect(onNavigate).toHaveBeenCalledWith({ kind: "notifications" });
});

test("a click on a notification opens the watch it belongs to", async () => {
  const { wrapper } = harness([]);
  const onNavigate = vi.fn();
  const clicks: ((c: { id: number; watchId?: number }) => void)[] = [];
  vi.spyOn(bridge.notifications, "onClick").mockImplementation((listener) => {
    clicks.push(listener);
    return () => undefined;
  });

  renderHook(() => useNativeNotifications(true, onNavigate, 3), { wrapper });
  clicks[0]({ id: 4, watchId: 42 });

  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 42 });
});

function readCalls(): number[][] {
  const calls: number[][] = [];
  server.use(
    http.post(apiUrl("/api/v1/notifications/read"), async ({ request }) => {
      const body = (await request.json()) as { ids?: number[] };
      calls.push(body.ids ?? []);
      return HttpResponse.json({ unreadCount: 0 });
    }),
  );
  return calls;
}

function clickListeners(): ((click: { id: number; watchId?: number }) => void)[] {
  const clicks: ((click: { id: number; watchId?: number }) => void)[] = [];
  vi.spyOn(bridge.notifications, "onClick").mockImplementation((listener) => {
    clicks.push(listener);
    return () => undefined;
  });
  return clicks;
}

test("a click on a banner marks that notification as seen", async () => {
  const { wrapper } = harness([buildNotification({ id: 4, watchId: 42 })]);
  const marks = readCalls();
  const clicks = clickListeners();

  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  clicks[0]({ id: 4, watchId: 42 });

  await waitFor(() => expect(marks).toEqual([[4]]));
});

test("a click on a banner of no watch marks it as seen just the same", async () => {
  const { wrapper } = harness([buildNotification({ id: 7 })]);
  const marks = readCalls();
  const clicks = clickListeners();

  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  clicks[0]({ id: 7 });

  await waitFor(() => expect(marks).toEqual([[7]]));
});

test("a kind the settings mute shows nothing, and the badge still counts", async () => {
  const muted = buildSettings({ mutedNotificationKinds: ["review"] });
  const { queryClient, wrapper, show, setBadge } = harness([buildNotification({ id: 3 })], muted);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(1));

  serveApi({
    notifications: [buildNotification({ id: 4, kind: "review" }), buildNotification({ id: 3 })],
    settings: muted,
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(2));
  expect(show).not.toHaveBeenCalled();
});

test("settings the app cannot read show nothing, so a muted kind is never shown by accident", async () => {
  const { queryClient, wrapper, show, setBadge } = harness([buildNotification({ id: 3 })]);
  failSettings();
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(1));

  serveApi({ notifications: [buildNotification({ id: 4, kind: "review" }), buildNotification({ id: 3 })] });
  failSettings();
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(2));
  expect(show).not.toHaveBeenCalled();
});

test("a kind nobody muted is shown while another one is off", async () => {
  const muted = buildSettings({ mutedNotificationKinds: ["review"] });
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })], muted);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  serveApi({
    notifications: [buildNotification({ id: 4, kind: "merge" }), buildNotification({ id: 3 })],
    settings: muted,
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 4, kind: "merge" })));
});

test("an app that does not present shows nothing, and the badge still counts", async () => {
  const { queryClient, wrapper, show, setBadge } = harness([buildNotification({ id: 3 })]);
  queryClient.setQueryData(settingsQueryKey, buildSettings());
  queryClient.setQueryData(notificationsQueryKey, { notifications: [buildNotification({ id: 3 })], unreadCount: 1 });

  renderHook(() => useNativeNotifications(false, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(1));

  queryClient.setQueryData(notificationsQueryKey, {
    notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })],
    unreadCount: 2,
  });

  await waitFor(() => expect(setBadge).toHaveBeenCalledWith(2));
  expect(show).not.toHaveBeenCalled();
});

test("the rows the daemon showed while the feed was down are not shown again", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  const { rerender } = renderHook(({ presented }) => useNativeNotifications(true, vi.fn(), presented), {
    wrapper,
    initialProps: { presented: 3 as number | null },
  });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  rerender({ presented: null });
  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })] });
  rerender({ presented: 4 });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toMatchObject({ unreadCount: 2 }));

  expect(show).not.toHaveBeenCalled();
});

test("a read of the history inside the gap does not make the gap the app's", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  const { rerender } = renderHook(({ presented }) => useNativeNotifications(true, vi.fn(), presented), {
    wrapper,
    initialProps: { presented: 3 as number | null },
  });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  rerender({ presented: null });
  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })] });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toMatchObject({ unreadCount: 2 }));

  serveApi({
    notifications: [buildNotification({ id: 5 }), buildNotification({ id: 4 }), buildNotification({ id: 3 })],
  });
  rerender({ presented: 5 });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toMatchObject({ unreadCount: 3 }));

  expect(show).not.toHaveBeenCalled();
});

test("a row that arrives after the feed came back is shown", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  const { rerender } = renderHook(({ presented }) => useNativeNotifications(true, vi.fn(), presented), {
    wrapper,
    initialProps: { presented: 3 as number | null },
  });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  rerender({ presented: null });
  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })] });
  rerender({ presented: 4 });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toMatchObject({ unreadCount: 2 }));

  serveApi({
    notifications: [buildNotification({ id: 5 }), buildNotification({ id: 4 }), buildNotification({ id: 3 })],
  });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledTimes(1));
  expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 5 }));
});

test("a row that arrives after a quiet reconnect is shown", async () => {
  const { queryClient, wrapper, show, setBadge } = harness([buildNotification({ id: 3 })]);
  const { rerender } = renderHook(({ presented }) => useNativeNotifications(true, vi.fn(), presented), {
    wrapper,
    initialProps: { presented: 3 as number | null },
  });
  await waitFor(() => expect(setBadge).toHaveBeenCalledTimes(1));
  const before = queryClient.getQueryState(notificationsQueryKey)?.dataUpdatedAt ?? 0;

  rerender({ presented: null });
  rerender({ presented: 3 });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryState(notificationsQueryKey)?.dataUpdatedAt).toBeGreaterThan(before));

  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 3 })] });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledTimes(1));
  expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 4 }));
});

test("a read that changes no count sets no badge", async () => {
  const { queryClient, wrapper, setBadge } = harness([buildNotification({ id: 3 })]);
  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });
  await waitFor(() => expect(setBadge).toHaveBeenCalledTimes(1));
  const before = queryClient.getQueryState(notificationsQueryKey)?.dataUpdatedAt ?? 0;

  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
  await waitFor(() => expect(queryClient.getQueryState(notificationsQueryKey)?.dataUpdatedAt).toBeGreaterThan(before));

  expect(setBadge).toHaveBeenCalledTimes(1);
});

test("a row above the newest of the claim is shown, even on the first load", async () => {
  const { wrapper, show } = harness([
    buildNotification({ id: 4, title: "PR #7", body: "build failed" }),
    buildNotification({ id: 3 }),
  ]);

  renderHook(() => useNativeNotifications(true, vi.fn(), 3), { wrapper });

  await waitFor(() => expect(show).toHaveBeenCalledTimes(1));
  expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 4, title: "PR #7" }));
});

test("a row above the newest of the reconnect is shown, even in the first read after it", async () => {
  const { queryClient, wrapper, show } = harness([buildNotification({ id: 3 })]);
  const { rerender } = renderHook(({ presented }) => useNativeNotifications(true, vi.fn(), presented), {
    wrapper,
    initialProps: { presented: 3 as number | null },
  });
  await waitFor(() => expect(queryClient.getQueryData(notificationsQueryKey)).toBeDefined());

  rerender({ presented: null });
  serveApi({
    notifications: [buildNotification({ id: 5 }), buildNotification({ id: 4 }), buildNotification({ id: 3 })],
  });
  rerender({ presented: 4 });
  await queryClient.invalidateQueries({ queryKey: notificationsQueryKey });

  await waitFor(() => expect(show).toHaveBeenCalledTimes(1));
  expect(show).toHaveBeenCalledWith(expect.objectContaining({ id: 5 }));
});
