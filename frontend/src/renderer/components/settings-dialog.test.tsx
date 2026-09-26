import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { SettingsDialog } from "./settings-dialog";

test("opens on General and shows the theme setting", () => {
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);

  expect(screen.getByRole("button", { name: "General" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("heading", { name: "General" })).toBeVisible();
  expect(screen.getByRole("group", { name: "Theme" })).toBeVisible();
});

test("the theme control in the General panel takes a choice", async () => {
  window.localStorage.setItem("theme", "light");
  const user = userEvent.setup();

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Dark theme" }));

  expect(screen.getByRole("button", { name: "Dark theme" })).toHaveAttribute("aria-pressed", "true");
});

test("opens on the category it is asked for", async () => {
  serveApi();

  renderWithProviders(<SettingsDialog open category="watching" onOpenChange={vi.fn()} />);

  expect(screen.getByRole("button", { name: "Watching" })).toHaveAttribute("aria-current", "page");
  expect(await screen.findByRole("heading", { name: "Watching" })).toBeVisible();
});

test("opens again on the category it is asked for after another pane was chosen", async () => {
  serveApi();
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(<SettingsDialog open category="watching" onOpenChange={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "General" }));

  rerender(<SettingsDialog open={false} category="watching" onOpenChange={vi.fn()} />);
  rerender(<SettingsDialog open category="watching" onOpenChange={vi.fn()} />);

  expect(screen.getByRole("button", { name: "Watching" })).toHaveAttribute("aria-current", "page");
  expect(await screen.findByRole("heading", { name: "Watching" })).toBeVisible();
});

test("has a pane for the updates of the app", async () => {
  renderWithProviders(<SettingsDialog open category="updates" onOpenChange={vi.fn()} />);

  expect(screen.getByRole("button", { name: "Updates" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("heading", { name: "Updates" })).toBeVisible();
  expect(await screen.findByText(/This build does not update itself/)).toBeVisible();
});

test("stays shut when it is not open", () => {
  renderWithProviders(<SettingsDialog open={false} onOpenChange={vi.fn()} />);

  expect(screen.queryByRole("heading", { name: "General" })).not.toBeInTheDocument();
});
