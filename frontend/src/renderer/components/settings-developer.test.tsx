import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildRateLimit } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { RateLimitCard } from "./rate-limit-card";
import { DeveloperPanel } from "./settings-developer";
import type { LogLevel } from "../../shared/logs";

function renderPanelAndCard() {
  serveApi({ rateLimit: buildRateLimit({ state: "ok", remaining: 4212 }) });
  renderWithProviders(
    <>
      <DeveloperPanel />
      <RateLimitCard enabled onPollLessOften={vi.fn()} />
    </>,
  );
}

test("always showing the rate limit puts the card in the sidebar at once", async () => {
  const user = userEvent.setup();
  renderPanelAndCard();

  const toggle = screen.getByRole("switch", { name: "Always show the GitHub rate limit" });
  expect(toggle).not.toBeChecked();

  await user.click(toggle);

  expect(toggle).toBeChecked();
  expect(await screen.findByRole("region", { name: "GitHub rate limit" })).toBeVisible();
});

test("turning the setting off hides a budget that is not relevant again", async () => {
  window.localStorage.setItem("always_show_rate_limit", "true");
  const user = userEvent.setup();
  renderPanelAndCard();

  await screen.findByRole("region", { name: "GitHub rate limit" });
  await user.click(screen.getByRole("switch", { name: "Always show the GitHub rate limit" }));

  expect(screen.queryByRole("region", { name: "GitHub rate limit" })).not.toBeInTheDocument();
});

test("keeps the choice for the next start of the app", async () => {
  const user = userEvent.setup();
  renderWithProviders(<DeveloperPanel />);

  await user.click(screen.getByRole("switch", { name: "Always show the GitHub rate limit" }));

  expect(window.localStorage.getItem("always_show_rate_limit")).toBe("true");
});

test("the debug switch makes the daemon record at debug level", async () => {
  const savedLogLevels: LogLevel[] = [];
  serveApi({ logLevel: "info", savedLogLevels });
  const user = userEvent.setup();
  renderWithProviders(<DeveloperPanel />);

  const toggle = screen.getByRole("switch", { name: "Debug logs" });
  await waitFor(() => expect(toggle).toBeEnabled());
  expect(toggle).not.toBeChecked();

  await user.click(toggle);

  await waitFor(() => expect(toggle).toBeChecked());
  expect(savedLogLevels).toEqual(["debug"]);
});

test("the debug switch shows the level the daemon runs at", async () => {
  serveApi({ logLevel: "debug" });
  renderWithProviders(<DeveloperPanel />);

  await waitFor(() => expect(screen.getByRole("switch", { name: "Debug logs" })).toBeChecked());
});

test("the debug switch waits for the daemon", () => {
  renderWithProviders(<DeveloperPanel />);

  expect(screen.getByRole("switch", { name: "Debug logs" })).toBeDisabled();
});
