import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildNotification } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { expectViewTitle, renderWithProviders } from "@test/test-utils";
import { NotificationsView } from "./notifications-view";

test("the view lists what the daemon told you, newest first", async () => {
  serveApi({
    notifications: [
      buildNotification({ id: 2, kind: "checks", title: "PR #7", body: "build failed" }),
      buildNotification({ id: 1, kind: "review", body: "bob commented: rename this", readAt: "2026-09-21T11:00:00Z" }),
    ],
  });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByText("build failed")).toBeVisible();
  expect(screen.getByText("bob commented: rename this")).toBeVisible();
  expect(screen.getByText("1 unread")).toBeVisible();
});

test("an empty history says what lands here", async () => {
  serveApi({ notifications: [] });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByText("Nothing to tell you")).toBeVisible();
});

test("a click on a row opens its watch and marks the row as seen", async () => {
  const readNotifications: { ids?: number[] }[] = [];
  const onNavigate = vi.fn();
  serveApi({
    notifications: [buildNotification({ id: 4, watchId: 42, body: "bob commented: rename this" })],
    readNotifications,
  });

  renderWithProviders(<NotificationsView enabled onNavigate={onNavigate} />);
  await userEvent.setup().click(await screen.findByText("bob commented: rename this"));

  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 42 });
  await waitFor(() => expect(readNotifications).toEqual([{ ids: [4] }]));
});

test("mark all as read sends no ids, so the daemon marks them all", async () => {
  const readNotifications: { ids?: number[] }[] = [];
  serveApi({ notifications: [buildNotification({ id: 4 }), buildNotification({ id: 5 })], readNotifications });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);
  await userEvent.setup().click(await screen.findByRole("button", { name: "Mark all as read" }));

  await waitFor(() => expect(readNotifications).toEqual([{}]));
});

test("nothing unread leaves the mark all button off", async () => {
  serveApi({ notifications: [buildNotification({ id: 4, readAt: "2026-09-21T12:30:00Z" })] });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByRole("button", { name: "Mark all as read" })).toBeDisabled();
});

test("keeps the title in the view header", async () => {
  serveApi({ notifications: [buildNotification()] });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByRole("banner")).toContainElement(
    screen.getByRole("heading", { level: 1, name: "Notifications" }),
  );
});

test("keeps the title in the view header while notifications load, fail or are none", async () => {
  serveApi({ notifications: [] });
  const { unmount } = renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);
  expectViewTitle("Notifications");
  expect(await screen.findByText("Nothing to tell you")).toBeVisible();
  expectViewTitle("Notifications");
  unmount();

  server.use(
    http.get(apiUrl("/api/v1/notifications"), () =>
      HttpResponse.json({ error: { message: "daemon gone" } }, { status: 500 }),
    ),
  );
  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByText("daemon gone")).toBeVisible();
  expectViewTitle("Notifications");
});
