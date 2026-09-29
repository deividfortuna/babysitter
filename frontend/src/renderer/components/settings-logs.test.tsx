import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { LogsPanel } from "./settings-logs";
import { SettingsDialog } from "./settings-dialog";
import type { LogLevel } from "../../shared/logs";

test("the Logs page holds the debug switch and the viewer", async () => {
  serveApi();
  renderWithProviders(<SettingsDialog open category="logs" onOpenChange={vi.fn()} />);

  expect(await screen.findByRole("switch", { name: "Debug logs" })).toBeVisible();
  expect(screen.getByRole("log", { name: "Log records" })).toBeVisible();
  expect(screen.getByText("babysitter daemon logs")).toBeVisible();
});

test("the debug switch makes the daemon record at debug level", async () => {
  const savedLogLevels: LogLevel[] = [];
  serveApi({ logLevel: "info", savedLogLevels });
  const user = userEvent.setup();
  renderWithProviders(<LogsPanel />);

  const toggle = screen.getByRole("switch", { name: "Debug logs" });
  await waitFor(() => expect(toggle).toBeEnabled());
  expect(toggle).not.toBeChecked();

  await user.click(toggle);

  await waitFor(() => expect(toggle).toBeChecked());
  expect(savedLogLevels).toEqual(["debug"]);
});

test("the debug switch shows the level the daemon runs at", async () => {
  serveApi({ logLevel: "debug" });
  renderWithProviders(<LogsPanel />);

  await waitFor(() => expect(screen.getByRole("switch", { name: "Debug logs" })).toBeChecked());
});

test("the debug switch waits for the daemon", () => {
  renderWithProviders(<LogsPanel />);

  expect(screen.getByRole("switch", { name: "Debug logs" })).toBeDisabled();
});

test("a level the daemon refuses shows what the daemon answered", async () => {
  serveApi({ logLevel: "info" });
  server.use(
    http.put(apiUrl("/api/v1/logs/level"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "the level is locked" } }, { status: 400 }),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<SettingsDialog open category="logs" onOpenChange={vi.fn()} />);

  const toggle = await screen.findByRole("switch", { name: "Debug logs" });
  await waitFor(() => expect(toggle).toBeEnabled());
  await user.click(toggle);

  expect(await screen.findByText("the level is locked")).toBeVisible();
});
