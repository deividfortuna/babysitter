import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildNotification, buildWatch } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi, type Decision } from "@test/msw";
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

test("a Dependabot update that waits on a review offers Approve and merge", async () => {
  const decisions: Decision[] = [];
  const readNotifications: { ids?: number[] }[] = [];
  const onNavigate = vi.fn();
  serveApi({
    watches: [buildWatch({ id: 42 })],
    notifications: [
      buildNotification({
        id: 9,
        kind: "auto",
        watchId: 42,
        action: "approve_merge",
        title: "Bump x/net waits on your review",
        body: "Minor update, build green. Only a review is missing.",
      }),
    ],
    decisions,
    readNotifications,
  });
  const user = userEvent.setup();

  renderWithProviders(<NotificationsView enabled onNavigate={onNavigate} />);
  await user.click(await screen.findByRole("button", { name: "Approve and merge" }));

  await waitFor(() => expect(decisions).toEqual([{ route: "merge", watch: 42, body: { method: "", approve: true } }]));
  await waitFor(() => expect(readNotifications).toEqual([{ ids: [9] }]));
  expect(onNavigate).not.toHaveBeenCalled();

  await user.click(screen.getByRole("button", { name: "Open the watch" }));
  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 42 });
});

test("Approve and merge shows why the daemon refused it", async () => {
  serveApi({
    watches: [buildWatch({ id: 42 })],
    notifications: [buildNotification({ id: 9, kind: "auto", watchId: 42, action: "approve_merge" })],
  });
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/merge"), () =>
      HttpResponse.json(
        { error: { code: "approve_refused", message: "the update is outside the merge scope of the repository" } },
        { status: 422 },
      ),
    ),
  );
  const user = userEvent.setup();

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Approve and merge" }));

  expect(await screen.findByText("the update is outside the merge scope of the repository")).toBeVisible();
});

test("a notification whose watch stopped offers no Approve and merge", async () => {
  serveApi({
    watches: [buildWatch({ id: 43 })],
    notifications: [
      buildNotification({ id: 9, kind: "auto", watchId: 42, action: "approve_merge", body: "stopped since" }),
      buildNotification({ id: 10, kind: "auto", watchId: 43, action: "approve_merge", body: "still watched" }),
    ],
  });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByRole("button", { name: "Approve and merge" })).toBeVisible();
  expect(screen.getAllByRole("button", { name: "Approve and merge" })).toHaveLength(1);
  expect(screen.getByText("still watched").closest('[role="listitem"]')).toContainElement(
    screen.getByRole("button", { name: "Approve and merge" }),
  );
});

test("groups the notifications into today and earlier, and marks the unread ones", async () => {
  serveApi({
    notifications: [
      buildNotification({ id: 3, kind: "agent", title: "Agent asks", createdAt: new Date().toISOString() }),
      buildNotification({ id: 2, kind: "merge", title: "Merged one", createdAt: new Date().toISOString() }),
      buildNotification({ id: 1, title: "Old one", readAt: "2026-09-21T11:00:00Z" }),
    ],
  });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  const today = await screen.findByRole("region", { name: "Today" });
  const asks = within(today).getByRole("button", { name: /Agent asks/ });
  expect(within(asks).getByText("unread")).toBeInTheDocument();
  const merged = within(today).getByRole("button", { name: /Merged one/ });
  expect(within(merged).getByText("unread")).toBeInTheDocument();
  const earlier = screen.getByRole("region", { name: "Earlier" });
  const old = within(earlier).getByRole("button", { name: /Old one/ });
  expect(within(old).queryByText("unread")).toBeNull();
});

test("only an unread notification that waits on you asks for attention", async () => {
  serveApi({
    watches: [buildWatch({ id: 42 })],
    notifications: [
      buildNotification({ id: 5, kind: "auto", watchId: 41, action: "approve_merge", title: "Watch stopped" }),
      buildNotification({ id: 4, kind: "agent", title: "Agent asks" }),
      buildNotification({ id: 3, kind: "auto", watchId: 42, action: "approve_merge", title: "Review waits" }),
      buildNotification({ id: 2, kind: "merge", title: "Merged one" }),
      buildNotification({ id: 1, kind: "agent", title: "Answered", readAt: "2026-09-21T11:00:00Z" }),
    ],
  });

  renderWithProviders(<NotificationsView enabled onNavigate={vi.fn()} />);

  const rowOf = async (name: RegExp) => (await screen.findByRole("button", { name })).closest('[role="listitem"]');
  expect(await rowOf(/Agent asks/)).toHaveAttribute("data-attention");
  expect(await rowOf(/Review waits/)).toHaveAttribute("data-attention");
  expect(await rowOf(/Merged one/)).not.toHaveAttribute("data-attention");
  expect(await rowOf(/Answered/)).not.toHaveAttribute("data-attention");
  expect(await rowOf(/Watch stopped/)).not.toHaveAttribute("data-attention");
});
