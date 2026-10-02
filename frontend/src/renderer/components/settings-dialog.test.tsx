import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildSettings } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { deferred, renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import { SettingsDialog } from "./settings-dialog";

function navigation() {
  return screen.getByRole("navigation", { name: "Settings categories" });
}

function pageButton(name: string) {
  return within(navigation()).getByRole("button", { name });
}

test("the pages sit in three groups: App, New watches and Daemon", () => {
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);

  const pagesOf = (group: string) =>
    within(within(navigation()).getByRole("group", { name: group }))
      .getAllByRole("button")
      .map((button) => button.textContent);
  expect(pagesOf("App")).toEqual(["Appearance", "Notifications", "Updates"]);
  expect(pagesOf("New watches")).toEqual(["Agent", "Review and merge"]);
  expect(pagesOf("Daemon")).toEqual(["GitHub access", "Polling", "Logs"]);
});

test("opens on Appearance and shows the theme setting", () => {
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);

  expect(pageButton("Appearance")).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("heading", { name: "Appearance" })).toBeVisible();
  expect(screen.getByRole("group", { name: "Theme" })).toBeVisible();
});

test("the theme control on the Appearance page takes a choice", async () => {
  window.localStorage.setItem("theme", "light");
  const user = userEvent.setup();

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Dark theme" }));

  expect(screen.getByRole("button", { name: "Dark theme" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByText("saved")).toBeVisible();
});

test("opens on the category it is asked for", async () => {
  serveApi();

  renderWithProviders(<SettingsDialog open category="polling" onOpenChange={vi.fn()} />);

  expect(pageButton("Polling")).toHaveAttribute("aria-current", "page");
  expect(await screen.findByRole("heading", { name: "Polling" })).toBeVisible();
});

test("opens again on the category it is asked for after another page was chosen", async () => {
  serveApi();
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);
  await user.click(pageButton("Appearance"));

  rerender(<SettingsDialog open={false} category="agent" onOpenChange={vi.fn()} />);
  rerender(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);

  expect(pageButton("Agent")).toHaveAttribute("aria-current", "page");
  expect(await screen.findByRole("heading", { name: "Agent" })).toBeVisible();
});

test("has a page for the updates of the app", async () => {
  renderWithProviders(<SettingsDialog open category="updates" onOpenChange={vi.fn()} />);

  expect(pageButton("Updates")).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("heading", { name: "Updates" })).toBeVisible();
  expect(await screen.findByText(/This build does not update itself/)).toBeVisible();
});

test("the version of the app sits under the pages", async () => {
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);

  expect(await within(navigation()).findByText(/^babysitter \S+$/)).toBeVisible();
});

test("a change on a page shows the saved mark, and another page starts without it", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  const own = await screen.findByRole("switch", { name: "Report my own comments" });
  expect(screen.queryByText("saved")).not.toBeInTheDocument();
  await user.click(own);

  expect(await screen.findByText("saved")).toBeVisible();
  expect(savedSettings).toHaveLength(1);

  await user.click(pageButton("Agent"));

  expect(screen.queryByText("saved")).not.toBeInTheDocument();
});

test("the saved mark does not show when the daemon refuses the change", async () => {
  serveApi({ settings: buildSettings() });
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "no" } }, { status: 400 }),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));

  expect(await screen.findByText("no")).toBeVisible();
  expect(screen.queryByText("saved")).not.toBeInTheDocument();
});

async function typeWatchIntervalThenOpenLogs(user: ReturnType<typeof userEvent.setup>) {
  const field = await screen.findByLabelText("Watch poll interval", { exact: true });
  await user.clear(field);
  await user.type(field, "90");
  await user.click(pageButton("Logs"));
}

test("a number still waiting when you leave its page is saved, and the navigation says so", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ pollIntervalSeconds: 45 }), savedSettings });
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="polling" onOpenChange={vi.fn()} />);

  await typeWatchIntervalThenOpenLogs(user);

  expect(await within(navigation()).findByText("Polling saved")).toBeVisible();
  expect(savedSettings).toHaveLength(1);
  expect(savedSettings[0]).toMatchObject({ watchIntervalSeconds: 90 });
  expect(screen.getByRole("heading", { name: "Logs" })).toBeVisible();
  expect(screen.queryByText("saved")).not.toBeInTheDocument();
});

test("a number the daemon refuses after you left its page shows why under the navigation", async () => {
  serveApi({ settings: buildSettings({ pollIntervalSeconds: 45 }) });
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "the interval is too short" } }, { status: 400 }),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="polling" onOpenChange={vi.fn()} />);

  await typeWatchIntervalThenOpenLogs(user);

  expect(await within(navigation()).findByText("Polling: the interval is too short")).toBeVisible();
});

test("a save still running when you leave its page keeps its line until it ends", async () => {
  serveApi({ settings: buildSettings() });
  const held = deferred();
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = await request.json();
      await held.promise;
      return HttpResponse.json(body as Settings);
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));
  await user.click(pageButton("Agent"));

  expect(await within(navigation()).findByText("Review and merge saving")).toBeVisible();
  held.resolve();
  expect(await within(navigation()).findByText("Review and merge saved")).toBeVisible();
});

function refuseWhen(refused: (body: Settings) => boolean, message: string) {
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      if (refused(body)) return HttpResponse.json({ error: { code: "bad_request", message } }, { status: 400 });
      return HttpResponse.json(body);
    }),
  );
}

test("a refused save stays on its page when a save queued behind it goes through", async () => {
  serveApi({ settings: buildSettings() });
  const held = deferred();
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      if (!body.includeOwn) return HttpResponse.json(body);
      await held.promise;
      return HttpResponse.json({ error: { code: "bad_request", message: "own comments are locked" } }, { status: 400 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));
  await user.click(screen.getByRole("switch", { name: "Report the review items that already exist" }));
  held.resolve();

  expect(await screen.findByText("own comments are locked")).toBeVisible();
  expect(await screen.findByText("saved")).toBeVisible();
  expect(screen.getByText("own comments are locked")).toBeVisible();
});

test("a refused save of another page stays under the navigation after a save here goes through", async () => {
  serveApi({ settings: buildSettings() });
  refuseWhen((body) => body.includeOwn, "own comments are locked");
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));
  expect(await screen.findByText("own comments are locked")).toBeVisible();
  await user.click(pageButton("Agent"));
  await user.click(await screen.findByRole("switch", { name: "Keep the worktree when a watch stops" }));

  expect(await screen.findByText("saved")).toBeVisible();
  expect(within(navigation()).getByText("Review and merge: own comments are locked")).toBeVisible();
});

test("a save that goes through after the refusal clears it from the page", async () => {
  serveApi({ settings: buildSettings() });
  refuseWhen((body) => body.includeOwn, "own comments are locked");
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));
  expect(await screen.findByText("own comments are locked")).toBeVisible();
  await user.click(screen.getByRole("switch", { name: "Report the review items that already exist" }));

  await waitFor(() => expect(screen.queryByText("own comments are locked")).toBeNull());
});

test("closing the dialog tells the owner", async () => {
  const onOpenChange = vi.fn();
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open onOpenChange={onOpenChange} />);

  await user.click(screen.getByRole("button", { name: "Close" }));

  expect(onOpenChange).toHaveBeenCalledWith(false);
});

test("stays shut when it is not open", () => {
  renderWithProviders(<SettingsDialog open={false} onOpenChange={vi.fn()} />);

  expect(screen.queryByRole("heading", { name: "Appearance" })).not.toBeInTheDocument();
});
