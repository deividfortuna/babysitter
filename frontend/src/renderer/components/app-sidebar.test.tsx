import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import type { ComponentProps } from "react";
import { buildNotification, buildRateLimit, buildRepo, buildWatch } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import { serveApi } from "@test/msw";
import { bridge } from "@/lib/bridge";
import { AppSidebar } from "./app-sidebar";

function renderSidebar(props: Partial<ComponentProps<typeof AppSidebar>> = {}) {
  renderWithProviders(
    <AppSidebar
      enabled
      view={{ kind: "watching" }}
      onNavigate={vi.fn()}
      onWatchPR={vi.fn()}
      onAddRepo={vi.fn()}
      onOpenSettings={vi.fn()}
      status={{ state: "ready", connection: { id: "local", kind: "local", name: "This Mac" } }}
      onPair={vi.fn()}
      width={256}
      onResize={vi.fn()}
      onResetWidth={vi.fn()}
      {...props}
    />,
    { withSidebar: true },
  );
}

test("offers the restart in the footer when a new version of the app is ready", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });
  const getStatus = vi
    .spyOn(bridge.updates, "getStatus")
    .mockResolvedValue({ state: "downloaded", currentVersion: "0.1.0", version: "0.2.0" });

  renderSidebar();

  expect(await screen.findByRole("button", { name: "Restart to update" })).toBeVisible();
  getStatus.mockRestore();
});

test("leaves the sidebar toggle to the content header", () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });

  renderSidebar();

  expect(screen.queryByRole("button", { name: "Toggle Sidebar" })).not.toBeInTheDocument();
});

test("navigates to the stopped watches from the sidebar", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });
  const onNavigate = vi.fn();
  const user = userEvent.setup();

  renderSidebar({ onNavigate });
  await user.click(screen.getByRole("button", { name: "Stopped" }));

  expect(onNavigate).toHaveBeenCalledWith({ kind: "stopped" });
});

test("navigates to the notifications from the sidebar and shows the unread count", async () => {
  serveApi({
    watches: [],
    repos: [],
    pullRequests: [],
    notifications: [buildNotification({ id: 1 }), buildNotification({ id: 2, readAt: "2026-09-21T12:30:00Z" })],
  });
  const onNavigate = vi.fn();
  const user = userEvent.setup();

  renderSidebar({ onNavigate });
  expect(await screen.findByTitle("1 unread notification")).toHaveTextContent("1");
  await user.click(screen.getByRole("button", { name: "Notifications" }));

  expect(onNavigate).toHaveBeenCalledWith({ kind: "notifications" });
});

test("opens the settings from the account row", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });
  const onOpenSettings = vi.fn();
  const user = userEvent.setup();

  renderSidebar({ onOpenSettings });
  await user.click(screen.getByRole("button", { name: "Settings" }));

  expect(onOpenSettings).toHaveBeenCalledWith();
});

test("keeps the actions of a repository shown while its menu is open", async () => {
  serveApi({ watches: [], repos: [buildRepo()], pullRequests: [] });
  const user = userEvent.setup();

  renderSidebar();
  const more = await screen.findByRole("button", { name: "More" });
  await user.click(more);

  expect(await screen.findByRole("menuitem", { name: "Sync now" })).toBeVisible();
  expect(more).toHaveAttribute("data-state", "open");
});

test("names the actions of a repository in a tooltip on hover", async () => {
  serveApi({ watches: [], repos: [buildRepo()], pullRequests: [] });
  const user = userEvent.setup();

  renderSidebar();
  await user.hover(await screen.findByRole("button", { name: "More" }));

  expect(await screen.findByRole("tooltip")).toHaveTextContent("More actions");
});

test("opens the Polling settings to poll less often when the rate limit runs low", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [], rateLimit: buildRateLimit({ state: "low", remaining: 312 }) });
  const onOpenSettings = vi.fn();
  const user = userEvent.setup();

  renderSidebar({ onOpenSettings });
  await user.click(await screen.findByRole("button", { name: "Poll less often" }));

  expect(onOpenSettings).toHaveBeenCalledWith("polling");
});

test("highlights how many pull requests are watched", async () => {
  serveApi({
    watches: [buildWatch({ id: 1, number: 1 }), buildWatch({ id: 2, number: 2 })],
    repos: [],
    pullRequests: [],
  });

  renderSidebar();

  const badge = await screen.findByTitle("2 watched pull requests");
  expect(badge).toHaveTextContent("2");
  expect(badge).toHaveClass("bg-attention");
});

test("counts the watches that wait on you when there are any", async () => {
  const base = buildWatch();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, session: { ...base.session, state: "waiting_input" } }),
      buildWatch({ id: 2, number: 2 }),
    ],
    repos: [],
    pullRequests: [],
  });

  renderSidebar();

  const badge = await screen.findByTitle("1 watched pull request needs you");
  expect(badge).toHaveTextContent("1");
  expect(badge).toHaveClass("bg-attention");
});

test("shows no number beside Watching when nothing is watched", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });

  renderSidebar();
  await screen.findByRole("button", { name: "Watching" });

  expect(screen.queryByTitle(/watched pull request/)).toBeNull();
});

test("shows the account the daemon acts as, and what sits beside it", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });

  renderSidebar();

  expect(await screen.findByText("Deivid Fortuna")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Report an issue" })).toHaveAttribute(
    "href",
    "https://github.com/deividfortuna/babysitter/issues",
  );

  expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
});

test("names the account as missing when the daemon cannot reach GitHub", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [], viewer: null });

  renderSidebar();

  expect(await screen.findByText("No account")).toBeInTheDocument();
});

test("names the daemon switcher with its state and the count of daemons found", async () => {
  serveApi({ watches: [], repos: [], pullRequests: [] });
  const discover = vi.spyOn(bridge.connections, "discover").mockResolvedValue([
    {
      name: "studio",
      host: "studio",
      address: "192.168.1.20",
      port: 7420,
      version: "",
      url: "http://192.168.1.20:7420",
    },
  ]);

  renderSidebar();

  expect(await screen.findByRole("button", { name: /This Mac.*Local daemon.*1 new/ })).toBeVisible();
  discover.mockRestore();
});
