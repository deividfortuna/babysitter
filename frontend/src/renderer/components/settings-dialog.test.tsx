import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildSettings } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
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
  expect(pagesOf("Daemon")).toEqual(["Polling", "Logs"]);
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
